package streaming

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/dilipmighty245/telemetry-pipeline/pkg/logging"
	"github.com/dilipmighty245/telemetry-pipeline/pkg/models"
)

const (
	DefaultChannelSize = 1000 // Increased from prtc's 20 for high throughput
	DefaultBatchSize   = 500  // Increased from prtc's 100 for telemetry data
	DefaultWorkers     = 10   // Multiple workers for parallel processing
	DefaultRetries     = 3
	DefaultTimeout     = 30 * time.Second
)

// StreamAdapterConfig holds configuration for the streaming adapter
type StreamAdapterConfig struct {
	ChannelSize   int           `json:"channel_size"`
	BatchSize     int           `json:"batch_size"`
	Workers       int           `json:"workers"`
	MaxRetries    int           `json:"max_retries"`
	RetryDelay    time.Duration `json:"retry_delay"`
	FlushInterval time.Duration `json:"flush_interval"`
	HTTPTimeout   time.Duration `json:"http_timeout"`
	EnableMetrics bool          `json:"enable_metrics"`
	PartitionBy   string        `json:"partition_by"` // "hostname", "gpu_id", "none"
}

// TelemetryChannelData represents data flowing through the channel
type TelemetryChannelData struct {
	Data      *models.TelemetryData
	Headers   map[string]string
	Timestamp time.Time
	Partition string // For partitioning (hostname, gpu_id, etc.)
}

// StreamAdapter provides high-throughput streaming for telemetry data
type StreamAdapter struct {
	config         *StreamAdapterConfig
	dataCh         chan TelemetryChannelData
	wg             sync.WaitGroup
	ctx            context.Context
	cancel         context.CancelFunc
	httpClient     *http.Client
	metrics        *StreamMetrics
	mu             sync.RWMutex
	isRunning      bool
	destinationURL string
}

// StreamMetrics tracks adapter performance
type StreamMetrics struct {
	TotalProcessed     int64     `json:"total_processed"`
	TotalBatches       int64     `json:"total_batches"`
	TotalErrors        int64     `json:"total_errors"`
	ProcessingRate     float64   `json:"processing_rate"`
	LastProcessTime    time.Time `json:"last_process_time"`
	ChannelUtilization float64   `json:"channel_utilization"`
	mu                 sync.RWMutex
}

// NewStreamAdapter creates a new high-performance streaming adapter
func NewStreamAdapter(config *StreamAdapterConfig, destinationURL string) *StreamAdapter {
	// Handle nil config by creating default config
	if config == nil {
		config = &StreamAdapterConfig{}
		config.EnableMetrics = true
	}

	// Set defaults
	if config.ChannelSize <= 0 {
		config.ChannelSize = DefaultChannelSize
	}
	if config.BatchSize <= 0 {
		config.BatchSize = DefaultBatchSize
	}
	if config.Workers <= 0 {
		config.Workers = DefaultWorkers
	}
	if config.MaxRetries <= 0 {
		config.MaxRetries = DefaultRetries
	}
	if config.RetryDelay <= 0 {
		config.RetryDelay = time.Second
	}
	if config.FlushInterval <= 0 {
		config.FlushInterval = 5 * time.Second
	}
	if config.HTTPTimeout <= 0 {
		config.HTTPTimeout = DefaultTimeout
	}
	if config.PartitionBy == "" {
		config.PartitionBy = "hostname"
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Create optimized HTTP client similar to prtc
	httpClient := &http.Client{
		Transport: &http.Transport{
			MaxIdleConns:        100, // Connection pool
			MaxIdleConnsPerHost: 20,
			IdleConnTimeout:     90 * time.Second,
			TLSHandshakeTimeout: 10 * time.Second,
			DisableKeepAlives:   false, // Reuse connections
		},
		Timeout: config.HTTPTimeout,
	}

	adapter := &StreamAdapter{
		config:         config,
		dataCh:         make(chan TelemetryChannelData, config.ChannelSize),
		ctx:            ctx,
		cancel:         cancel,
		httpClient:     httpClient,
		destinationURL: destinationURL,
		metrics: &StreamMetrics{
			LastProcessTime: time.Now(),
		},
	}

	return adapter
}

// Start starts the streaming adapter with multiple workers
func (sa *StreamAdapter) Start() error {
	sa.mu.Lock()
	defer sa.mu.Unlock()

	if sa.isRunning {
		return nil
	}

	sa.isRunning = true

	// Start multiple workers for parallel processing
	for i := 0; i < sa.config.Workers; i++ {
		sa.wg.Add(1)
		go sa.streamWorker(i)
	}

	// Start metrics collector if enabled
	if sa.config.EnableMetrics {
		sa.wg.Add(1)
		go sa.metricsCollector()
	}

	logging.Infof("Started stream adapter with %d workers", sa.config.Workers)
	return nil
}

// Stop gracefully stops the streaming adapter
func (sa *StreamAdapter) Stop() error {
	sa.mu.Lock()
	defer sa.mu.Unlock()

	if !sa.isRunning {
		return nil
	}

	sa.isRunning = false
	close(sa.dataCh)
	sa.cancel()
	sa.wg.Wait()

	logging.Infof("Stopped stream adapter")
	return nil
}

// WriteTelemetry writes telemetry data to the streaming channel
func (sa *StreamAdapter) WriteTelemetry(data *models.TelemetryData, headers map[string]string) error {
	partition := sa.getPartition(data)

	select {
	case sa.dataCh <- TelemetryChannelData{
		Data:      data,
		Headers:   headers,
		Timestamp: time.Now(),
		Partition: partition,
	}:
		return nil
	case <-sa.ctx.Done():
		return fmt.Errorf("stream adapter is shutting down")
	default:
		// Channel is full, could implement backpressure here
		return fmt.Errorf("channel is full")
	}
}

// getPartition determines partition key for the data
func (sa *StreamAdapter) getPartition(data *models.TelemetryData) string {
	switch sa.config.PartitionBy {
	case "hostname":
		return data.Hostname
	case "gpu_id":
		return data.GPUID
	case "none":
		return "default"
	default:
		return "default"
	}
}

// streamWorker processes data in batches (similar to prtc's streamBatch)
func (sa *StreamAdapter) streamWorker(workerID int) {
	defer sa.wg.Done()

	logging.Infof("Started stream worker %d", workerID)

	// Partition-based batching (similar to prtc's cluster-based batching)
	recordMap := make(map[string][]*models.TelemetryData)
	ticker := time.NewTicker(sa.config.FlushInterval)
	defer ticker.Stop()

	for {
		select {
		case data, ok := <-sa.dataCh:
			if !ok {
				// Channel closed, flush remaining data
				sa.flushAllPartitions(recordMap, workerID)
				logging.Infof("Stream worker %d stopped", workerID)
				return
			}

			partition := data.Partition
			if _, exists := recordMap[partition]; !exists {
				recordMap[partition] = make([]*models.TelemetryData, 0, sa.config.BatchSize)
			}

			recordMap[partition] = append(recordMap[partition], data.Data)

			// Check if batch is ready for this partition
			if len(recordMap[partition]) >= sa.config.BatchSize {
				err := sa.sendBatch(partition, recordMap[partition], workerID)
				if err == nil {
					delete(recordMap, partition)
				} else {
					logging.Errorf("Worker %d failed to send batch for partition %s: %v", workerID, partition, err)
				}
			}

		case <-ticker.C:
			// Periodic flush of all partitions
			sa.flushAllPartitions(recordMap, workerID)

		case <-sa.ctx.Done():
			sa.flushAllPartitions(recordMap, workerID)
			logging.Infof("Stream worker %d stopped", workerID)
			return
		}
	}
}

// flushAllPartitions flushes all partitions with pending data
func (sa *StreamAdapter) flushAllPartitions(recordMap map[string][]*models.TelemetryData, workerID int) {
	for partition, records := range recordMap {
		if len(records) > 0 {
			err := sa.sendBatch(partition, records, workerID)
			if err == nil {
				delete(recordMap, partition)
			} else {
				logging.Errorf("Worker %d failed to flush partition %s: %v", workerID, partition, err)
			}
		}
	}
}

// sendBatch sends a batch of telemetry data (with retry logic from prtc)
func (sa *StreamAdapter) sendBatch(partition string, records []*models.TelemetryData, workerID int) error {
	if len(records) == 0 {
		return nil
	}

	// Convert to JSON payload
	payload, err := json.Marshal(map[string]interface{}{
		"partition": partition,
		"records":   records,
		"timestamp": time.Now().Unix(),
		"worker_id": workerID,
	})
	if err != nil {
		sa.updateMetrics(0, 0, 1)
		return fmt.Errorf("failed to marshal batch: %v", err)
	}

	// Retry logic similar to prtc's postRequest
	var lastErr error
	for attempt := 1; attempt <= sa.config.MaxRetries; attempt++ {
		resp, err := sa.httpClient.Post(
			sa.destinationURL,
			"application/json",
			bytes.NewReader(payload),
		)

		if err == nil && resp != nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				sa.updateMetrics(int64(len(records)), 1, 0)
				logging.Debugf("Worker %d sent batch of %d records for partition %s", workerID, len(records), partition)
				return nil
			}
			lastErr = fmt.Errorf("HTTP error: %d", resp.StatusCode)
		} else {
			lastErr = err
		}

		if attempt < sa.config.MaxRetries {
			time.Sleep(time.Duration(attempt) * sa.config.RetryDelay)
		}
	}

	sa.updateMetrics(0, 0, 1)
	return fmt.Errorf("failed after %d attempts: %v", sa.config.MaxRetries, lastErr)
}

// updateMetrics updates performance metrics
func (sa *StreamAdapter) updateMetrics(recordsProcessed, batchesProcessed, errors int64) {
	sa.metrics.mu.Lock()
	defer sa.metrics.mu.Unlock()

	sa.metrics.TotalProcessed += recordsProcessed
	sa.metrics.TotalBatches += batchesProcessed
	sa.metrics.TotalErrors += errors
	sa.metrics.LastProcessTime = time.Now()
}

// metricsCollector periodically calculates performance metrics
func (sa *StreamAdapter) metricsCollector() {
	defer sa.wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	startTime := time.Now()

	for {
		select {
		case <-ticker.C:
			sa.metrics.mu.Lock()
			elapsed := time.Since(startTime).Seconds()
			if elapsed > 0 {
				sa.metrics.ProcessingRate = float64(sa.metrics.TotalProcessed) / elapsed
			}
			sa.metrics.ChannelUtilization = float64(len(sa.dataCh)) / float64(sa.config.ChannelSize) * 100
			sa.metrics.mu.Unlock()

		case <-sa.ctx.Done():
			return
		}
	}
}

// GetMetrics returns current performance metrics
func (sa *StreamAdapter) GetMetrics() *StreamMetrics {
	sa.metrics.mu.RLock()
	defer sa.metrics.mu.RUnlock()

	return &StreamMetrics{
		TotalProcessed:     sa.metrics.TotalProcessed,
		TotalBatches:       sa.metrics.TotalBatches,
		TotalErrors:        sa.metrics.TotalErrors,
		ProcessingRate:     sa.metrics.ProcessingRate,
		LastProcessTime:    sa.metrics.LastProcessTime,
		ChannelUtilization: sa.metrics.ChannelUtilization,
	}
}

// IsRunning returns whether the adapter is currently running
func (sa *StreamAdapter) IsRunning() bool {
	sa.mu.RLock()
	defer sa.mu.RUnlock()
	return sa.isRunning
}
