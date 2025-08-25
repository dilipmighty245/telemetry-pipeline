package streamer

import (
	"context"
	"encoding/json"
	"io"
	"sync"
	"time"

	"github.com/dilipmighty245/telemetry-pipeline/pkg/logging"
	"github.com/dilipmighty245/telemetry-pipeline/pkg/messagequeue"
	"github.com/dilipmighty245/telemetry-pipeline/pkg/models"
)

// StreamerConfig represents configuration for the streamer service
type StreamerConfig struct {
	CSVFilePath    string        `json:"csv_file_path"`
	BatchSize      int           `json:"batch_size"`
	StreamInterval time.Duration `json:"stream_interval"`
	LoopMode       bool          `json:"loop_mode"`
	MaxRetries     int           `json:"max_retries"`
	RetryDelay     time.Duration `json:"retry_delay"`
	EnableMetrics  bool          `json:"enable_metrics"`
	StreamerID     string        `json:"streamer_id"`
	BufferSize     int           `json:"buffer_size"`
}

// StreamerService handles streaming telemetry data from CSV to message queue
type StreamerService struct {
	config        *StreamerConfig
	csvReader     *CSVReader
	messageQueue  *messagequeue.MessageQueueService
	ctx           context.Context
	cancel        context.CancelFunc
	wg            sync.WaitGroup
	isRunning     bool
	mu            sync.RWMutex
	metrics       *StreamerMetrics
	lastError     error
	totalStreamed int64
}

// StreamerMetrics tracks streaming metrics
type StreamerMetrics struct {
	TotalRecordsStreamed int64     `json:"total_records_streamed"`
	TotalBatchesStreamed int64     `json:"total_batches_streamed"`
	TotalErrors          int64     `json:"total_errors"`
	LastStreamTime       time.Time `json:"last_stream_time"`
	StreamingRate        float64   `json:"streaming_rate"` // records per second
	StartTime            time.Time `json:"start_time"`
	mu                   sync.RWMutex
}

// NewStreamerService creates a new streamer service
func NewStreamerService(config *StreamerConfig, mqService *messagequeue.MessageQueueService) (*StreamerService, error) {
	// Set default values
	if config.BatchSize <= 0 {
		config.BatchSize = 100
	}
	if config.StreamInterval <= 0 {
		config.StreamInterval = 1 * time.Second
	}
	if config.MaxRetries <= 0 {
		config.MaxRetries = 3
	}
	if config.RetryDelay <= 0 {
		config.RetryDelay = 1 * time.Second
	}
	if config.StreamerID == "" {
		config.StreamerID = "streamer-" + time.Now().Format("20060102150405")
	}
	if config.BufferSize <= 0 {
		config.BufferSize = 1000
	}

	csvReader, err := NewCSVReader(config.CSVFilePath, config.LoopMode)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())

	service := &StreamerService{
		config:       config,
		csvReader:    csvReader,
		messageQueue: mqService,
		ctx:          ctx,
		cancel:       cancel,
		metrics: &StreamerMetrics{
			StartTime: time.Now(),
		},
	}

	logging.Infof("Created streamer service %s for file %s", config.StreamerID, config.CSVFilePath)
	return service, nil
}

// Start starts the streaming service
func (ss *StreamerService) Start(ctx context.Context) error {
	ss.mu.Lock()
	defer ss.mu.Unlock()

	if ss.isRunning {
		return nil
	}

	ss.isRunning = true
	ss.wg.Add(1)

	go ss.streamLoop(ctx)

	logging.Infof("Started streamer service %s", ss.config.StreamerID)
	return nil
}

// Stop stops the streaming service
func (ss *StreamerService) Stop() error {
	ss.mu.Lock()
	defer ss.mu.Unlock()

	if !ss.isRunning {
		return nil
	}

	ss.isRunning = false
	ss.cancel()

	// Wait for streaming loop to finish
	ss.wg.Wait()

	// Close CSV reader
	if ss.csvReader != nil {
		ss.csvReader.Close()
	}

	logging.Infof("Stopped streamer service %s", ss.config.StreamerID)
	return nil
}

// IsRunning returns whether the service is currently running
func (ss *StreamerService) IsRunning() bool {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	return ss.isRunning
}

// GetMetrics returns current streaming metrics
func (ss *StreamerService) GetMetrics() *StreamerMetrics {
	ss.metrics.mu.RLock()
	defer ss.metrics.mu.RUnlock()

	// Calculate streaming rate
	elapsed := time.Since(ss.metrics.StartTime).Seconds()
	if elapsed > 0 {
		ss.metrics.StreamingRate = float64(ss.metrics.TotalRecordsStreamed) / elapsed
	}

	// Create a copy of metrics
	return &StreamerMetrics{
		TotalRecordsStreamed: ss.metrics.TotalRecordsStreamed,
		TotalBatchesStreamed: ss.metrics.TotalBatchesStreamed,
		TotalErrors:          ss.metrics.TotalErrors,
		LastStreamTime:       ss.metrics.LastStreamTime,
		StreamingRate:        ss.metrics.StreamingRate,
		StartTime:            ss.metrics.StartTime,
	}
}

// GetLastError returns the last error encountered
func (ss *StreamerService) GetLastError() error {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	return ss.lastError
}

// GetTotalStreamed returns total number of records streamed
func (ss *StreamerService) GetTotalStreamed() int64 {
	return ss.totalStreamed
}

// streamLoop is the main streaming loop
func (ss *StreamerService) streamLoop(ctx context.Context) {
	defer ss.wg.Done()
	defer func() {
		if r := recover(); r != nil {
			logging.Errorf("Streamer service %s panic: %v", ss.config.StreamerID, r)
		}
	}()

	ticker := time.NewTicker(ss.config.StreamInterval)
	defer ticker.Stop()

	logging.Infof("Started streaming loop for service %s", ss.config.StreamerID)

	for {
		select {
		case <-ctx.Done():
			logging.Infof("Streaming loop stopped for service %s", ss.config.StreamerID)
			return
		case <-ticker.C:
			// Check context again before processing to ensure quick shutdown
			if ss.ctx.Err() != nil {
				logging.Infof("Context cancelled, stopping streaming loop for service %s", ss.config.StreamerID)
				return
			}

			err := ss.streamBatch(ctx)
			if err != nil {
				ss.mu.Lock()
				ss.lastError = err
				ss.mu.Unlock()

				ss.updateMetrics(0, 0, 1)

				if err == io.EOF && !ss.config.LoopMode {
					logging.Infof("Reached end of CSV file, stopping streamer %s", ss.config.StreamerID)
					// Don't call Stop() from within the loop to avoid deadlock
					// Just return and let the main Stop() method handle cleanup
					return
				}
			}
		}
	}
}

// streamBatch streams a batch of telemetry data
func (ss *StreamerService) streamBatch(ctx context.Context) error {
	// Read batch from CSV
	telemetryData, err := ss.csvReader.ReadBatch(ss.config.BatchSize)
	if err != nil && err != io.EOF {
		logging.Errorf("Failed to read batch from CSV: %v", err)
		return err
	}

	if len(telemetryData) == 0 {
		return err // Return EOF or other error
	}

	// Convert to JSON and publish to message queue
	for _, data := range telemetryData {
		// Check context before processing each record for quick shutdown
		if ss.ctx.Err() != nil {
			logging.Infof("Context cancelled during batch processing, stopping")
			return ss.ctx.Err()
		}

		err = ss.publishTelemetryData(ctx, data)
		if err != nil {
			logging.Errorf("Failed to publish telemetry data: %v", err)

			// Retry logic with context checking
			for retry := 0; retry < ss.config.MaxRetries; retry++ {
				// Check context before retry delay
				if ss.ctx.Err() != nil {
					logging.Infof("Context cancelled during retry, stopping")
					return ss.ctx.Err()
				}

				// Use context-aware sleep
				select {
				case <-ss.ctx.Done():
					logging.Infof("Context cancelled during retry delay, stopping")
					return ss.ctx.Err()
				case <-time.After(ss.config.RetryDelay):
					// Continue with retry
				}

				err = ss.publishTelemetryData(ctx, data)
				if err == nil {
					break
				}
				logging.Warnf("Retry %d failed for telemetry data: %v", retry+1, err)
			}

			if err != nil {
				logging.Errorf("Failed to publish telemetry data after %d retries", ss.config.MaxRetries)
				return err
			}
		}
	}

	// Update metrics
	ss.updateMetrics(int64(len(telemetryData)), 1, 0)
	ss.totalStreamed += int64(len(telemetryData))

	logging.Debugf("Streamed batch of %d records from service %s", len(telemetryData), ss.config.StreamerID)
	return err
}

// publishTelemetryData publishes a single telemetry data point to the message queue
func (ss *StreamerService) publishTelemetryData(ctx context.Context, data *models.TelemetryData) error {
	// Convert to JSON
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}

	// Create headers
	headers := map[string]string{
		"streamer_id": ss.config.StreamerID,
		"gpu_id":      data.GPUID,
		"hostname":    data.Hostname,
		"timestamp":   data.Timestamp.Format(time.RFC3339),
		"metric_name": data.MetricName,
	}

	// Publish to message queue
	return ss.messageQueue.PublishTelemetry(ctx, jsonData, headers)
}

// updateMetrics updates streaming metrics
func (ss *StreamerService) updateMetrics(recordsStreamed, batchesStreamed, errors int64) {
	ss.metrics.mu.Lock()
	defer ss.metrics.mu.Unlock()

	ss.metrics.TotalRecordsStreamed += recordsStreamed
	ss.metrics.TotalBatchesStreamed += batchesStreamed
	ss.metrics.TotalErrors += errors
	ss.metrics.LastStreamTime = time.Now()
}

// Health returns the health status of the streamer service
func (ss *StreamerService) Health() bool {
	ss.mu.RLock()
	defer ss.mu.RUnlock()

	// Service is healthy if it's running and no recent errors
	if !ss.isRunning {
		return false
	}

	// Check if we have recent activity (within last 5 minutes)
	ss.metrics.mu.RLock()
	lastActivity := ss.metrics.LastStreamTime
	ss.metrics.mu.RUnlock()

	if !lastActivity.IsZero() && time.Since(lastActivity) > 5*time.Minute {
		return false
	}

	return true
}

// GetConfig returns the streamer configuration
func (ss *StreamerService) GetConfig() *StreamerConfig {
	return ss.config
}

// UpdateConfig updates the streamer configuration (some fields only)
func (ss *StreamerService) UpdateConfig(newConfig *StreamerConfig) error {
	ss.mu.Lock()
	defer ss.mu.Unlock()

	// Only update safe fields while running
	if newConfig.StreamInterval > 0 {
		ss.config.StreamInterval = newConfig.StreamInterval
	}
	if newConfig.BatchSize > 0 {
		ss.config.BatchSize = newConfig.BatchSize
	}
	if newConfig.MaxRetries > 0 {
		ss.config.MaxRetries = newConfig.MaxRetries
	}
	if newConfig.RetryDelay > 0 {
		ss.config.RetryDelay = newConfig.RetryDelay
	}

	logging.Infof("Updated configuration for streamer service %s", ss.config.StreamerID)
	return nil
}
