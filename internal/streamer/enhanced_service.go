package streamer

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/dilipmighty245/telemetry-pipeline/internal/streaming"
	"github.com/dilipmighty245/telemetry-pipeline/pkg/logging"
	"github.com/dilipmighty245/telemetry-pipeline/pkg/messagequeue"
	"github.com/dilipmighty245/telemetry-pipeline/pkg/models"
)

// EnhancedStreamerConfig extends StreamerConfig with high-throughput features
type EnhancedStreamerConfig struct {
	*StreamerConfig
	StreamingConfig   *streaming.StreamAdapterConfig `json:"streaming_config"`
	EnableStreaming   bool                           `json:"enable_streaming"`
	StreamDestination string                         `json:"stream_destination"`

	// Parallel streaming
	EnableParallelStreaming bool `json:"enable_parallel_streaming"`
	ParallelWorkers         int  `json:"parallel_workers"`

	// Rate limiting
	EnableRateLimit bool    `json:"enable_rate_limit"`
	RateLimit       float64 `json:"rate_limit"` // records per second
	BurstSize       int     `json:"burst_size"`

	// Back pressure handling
	EnableBackPressure    bool          `json:"enable_back_pressure"`
	BackPressureThreshold float64       `json:"back_pressure_threshold"` // channel utilization %
	BackPressureDelay     time.Duration `json:"back_pressure_delay"`
}

// EnhancedStreamerService provides high-throughput streaming with advanced features
type EnhancedStreamerService struct {
	*StreamerService
	config        *EnhancedStreamerConfig
	streamAdapter *streaming.StreamAdapter
	rateLimiter   *RateLimiter
	workers       []*StreamerWorker
	workerPool    chan *StreamerWorker
	isEnhanced    bool
}

// StreamerWorker handles parallel streaming
type StreamerWorker struct {
	id       int
	service  *EnhancedStreamerService
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	isActive bool
	mu       sync.RWMutex
}

// RateLimiter implements token bucket rate limiting
type RateLimiter struct {
	rate       float64
	burstSize  int
	tokens     float64
	lastUpdate time.Time
	mu         sync.Mutex
}

// NewEnhancedStreamerService creates a new enhanced streamer service
func NewEnhancedStreamerService(config *EnhancedStreamerConfig, mqService *messagequeue.MessageQueueService) (*EnhancedStreamerService, error) {
	// Create base streamer service
	baseService, err := NewStreamerService(config.StreamerConfig, mqService)
	if err != nil {
		return nil, err
	}

	// Set defaults for enhanced features
	if config.StreamingConfig == nil && config.EnableStreaming {
		config.StreamingConfig = &streaming.StreamAdapterConfig{
			ChannelSize:   2000, // Higher for streaming
			BatchSize:     1000, // Larger batches for streaming
			Workers:       8,    // More workers for streaming
			MaxRetries:    3,
			RetryDelay:    time.Second,
			FlushInterval: 2 * time.Second, // Faster flush for streaming
			HTTPTimeout:   30 * time.Second,
			EnableMetrics: true,
			PartitionBy:   "hostname",
		}
	}

	if config.ParallelWorkers <= 0 {
		config.ParallelWorkers = 5
	}

	if config.RateLimit <= 0 {
		config.RateLimit = 1000 // 1000 records per second default
	}

	if config.BurstSize <= 0 {
		config.BurstSize = 100
	}

	if config.BackPressureThreshold <= 0 {
		config.BackPressureThreshold = 80.0 // 80% channel utilization
	}

	if config.BackPressureDelay <= 0 {
		config.BackPressureDelay = 100 * time.Millisecond
	}

	enhanced := &EnhancedStreamerService{
		StreamerService: baseService,
		config:          config,
		isEnhanced:      true,
	}

	// Initialize streaming adapter if enabled
	if config.EnableStreaming {
		enhanced.streamAdapter = streaming.NewStreamAdapter(config.StreamingConfig, config.StreamDestination)
	}

	// Initialize rate limiter if enabled
	if config.EnableRateLimit {
		enhanced.rateLimiter = &RateLimiter{
			rate:       config.RateLimit,
			burstSize:  config.BurstSize,
			tokens:     float64(config.BurstSize),
			lastUpdate: time.Now(),
		}
	}

	// Initialize worker pool if parallel streaming is enabled
	if config.EnableParallelStreaming {
		enhanced.workerPool = make(chan *StreamerWorker, config.ParallelWorkers)
		enhanced.workers = make([]*StreamerWorker, config.ParallelWorkers)

		for i := 0; i < config.ParallelWorkers; i++ {
			ctx, cancel := context.WithCancel(context.Background())
			worker := &StreamerWorker{
				id:      i,
				service: enhanced,
				ctx:     ctx,
				cancel:  cancel,
			}
			enhanced.workers[i] = worker
			enhanced.workerPool <- worker
		}
	}

	logging.Infof("Created enhanced streamer service with streaming=%v, parallel=%v, rate_limit=%v, back_pressure=%v",
		config.EnableStreaming, config.EnableParallelStreaming, config.EnableRateLimit, config.EnableBackPressure)

	return enhanced, nil
}

// Start starts the enhanced streamer service
func (ess *EnhancedStreamerService) Start() error {
	// Start base service
	err := ess.StreamerService.Start()
	if err != nil {
		return err
	}

	// Start streaming adapter
	if ess.streamAdapter != nil {
		err = ess.streamAdapter.Start()
		if err != nil {
			logging.Errorf("Failed to start stream adapter: %v", err)
			return err
		}
	}

	// Start worker pool
	if ess.config.EnableParallelStreaming {
		for _, worker := range ess.workers {
			go worker.start()
		}
	}

	logging.Infof("Started enhanced streamer service")
	return nil
}

// Stop stops the enhanced streamer service
func (ess *EnhancedStreamerService) Stop() error {
	// Stop streaming adapter
	if ess.streamAdapter != nil {
		err := ess.streamAdapter.Stop()
		if err != nil {
			logging.Errorf("Failed to stop stream adapter: %v", err)
		}
	}

	// Stop workers
	if ess.config.EnableParallelStreaming {
		for _, worker := range ess.workers {
			worker.stop()
		}
	}

	// Stop base service
	return ess.StreamerService.Stop()
}

// streamBatch overrides base streamBatch with enhanced features
func (ess *EnhancedStreamerService) streamBatch() error {
	// Check back pressure
	if ess.config.EnableBackPressure && ess.streamAdapter != nil {
		metrics := ess.streamAdapter.GetMetrics()
		if metrics.ChannelUtilization > ess.config.BackPressureThreshold {
			logging.Warnf("Back pressure detected (%.1f%% utilization), applying delay", metrics.ChannelUtilization)
			time.Sleep(ess.config.BackPressureDelay)
		}
	}

	// Use worker pool if enabled
	if ess.config.EnableParallelStreaming {
		return ess.streamBatchWithWorkers()
	}

	// Fallback to enhanced single-threaded streaming
	return ess.streamBatchEnhanced()
}

// streamBatchWithWorkers distributes work across worker pool
func (ess *EnhancedStreamerService) streamBatchWithWorkers() error {
	select {
	case worker := <-ess.workerPool:
		go func() {
			defer func() {
				ess.workerPool <- worker // Return worker to pool
			}()

			err := worker.streamBatch()
			if err != nil {
				logging.Errorf("Worker %d streaming failed: %v", worker.id, err)
			}
		}()
		return nil
	default:
		// No workers available, fallback to direct processing
		return ess.streamBatchEnhanced()
	}
}

// streamBatchEnhanced performs enhanced streaming with rate limiting
func (ess *EnhancedStreamerService) streamBatchEnhanced() error {
	// Read batch from CSV
	telemetryData, err := ess.csvReader.ReadBatch(ess.config.BatchSize)
	if err != nil && err != io.EOF {
		logging.Errorf("Failed to read batch from CSV: %v", err)
		return err
	}

	if len(telemetryData) == 0 {
		return err // Return EOF or other error
	}

	// Apply rate limiting
	if ess.rateLimiter != nil {
		for range telemetryData {
			if !ess.rateLimiter.allow() {
				// Rate limit exceeded, wait
				time.Sleep(10 * time.Millisecond)
			}
		}
	}

	// Stream data using streaming adapter if available
	if ess.streamAdapter != nil {
		for _, data := range telemetryData {
			headers := map[string]string{
				"streamer_id": ess.config.StreamerID,
				"gpu_id":      data.GPUID,
				"hostname":    data.Hostname,
				"timestamp":   data.Timestamp.Format(time.RFC3339),
				"metric_name": data.MetricName,
			}

			err := ess.streamAdapter.WriteTelemetry(data, headers)
			if err != nil {
				logging.Errorf("Failed to stream telemetry data: %v", err)
				// Fallback to message queue
				err = ess.publishTelemetryData(data)
				if err != nil {
					logging.Errorf("Failed to publish telemetry data to message queue: %v", err)
					return err
				}
			}
		}
	} else {
		// Fallback to message queue publishing
		for _, data := range telemetryData {
			err = ess.publishTelemetryData(data)
			if err != nil {
				logging.Errorf("Failed to publish telemetry data: %v", err)
				return err
			}
		}
	}

	// Update metrics
	ess.updateMetrics(int64(len(telemetryData)), 1, 0)
	ess.totalStreamed += int64(len(telemetryData))

	logging.Debugf("Enhanced streamed batch of %d records from service %s", len(telemetryData), ess.config.StreamerID)
	return err
}

// StreamerWorker methods

func (sw *StreamerWorker) start() {
	sw.mu.Lock()
	sw.isActive = true
	sw.mu.Unlock()

	logging.Infof("Started streamer worker %d", sw.id)
}

func (sw *StreamerWorker) stop() {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	if !sw.isActive {
		return
	}

	sw.isActive = false
	sw.cancel()
	sw.wg.Wait()

	logging.Infof("Stopped streamer worker %d", sw.id)
}

func (sw *StreamerWorker) streamBatch() error {
	// Delegate to enhanced service's streaming logic
	return sw.service.streamBatchEnhanced()
}

// RateLimiter methods (Token bucket algorithm)

func (rl *RateLimiter) allow() bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(rl.lastUpdate).Seconds()
	rl.lastUpdate = now

	// Add tokens based on elapsed time
	rl.tokens += elapsed * rl.rate
	if rl.tokens > float64(rl.burstSize) {
		rl.tokens = float64(rl.burstSize)
	}

	// Check if we have tokens available
	if rl.tokens >= 1.0 {
		rl.tokens -= 1.0
		return true
	}

	return false
}

// GetEnhancedMetrics returns comprehensive metrics including streaming
func (ess *EnhancedStreamerService) GetEnhancedMetrics() map[string]interface{} {
	baseMetrics := ess.GetMetrics()
	enhanced := map[string]interface{}{
		"base_metrics": baseMetrics,
		"is_enhanced":  ess.isEnhanced,
	}

	if ess.streamAdapter != nil {
		enhanced["streaming_metrics"] = ess.streamAdapter.GetMetrics()
	}

	if ess.rateLimiter != nil {
		ess.rateLimiter.mu.Lock()
		enhanced["rate_limiter"] = map[string]interface{}{
			"rate":       ess.rateLimiter.rate,
			"burst_size": ess.rateLimiter.burstSize,
			"tokens":     ess.rateLimiter.tokens,
		}
		ess.rateLimiter.mu.Unlock()
	}

	if ess.config.EnableParallelStreaming {
		activeWorkers := 0
		for _, worker := range ess.workers {
			worker.mu.RLock()
			if worker.isActive {
				activeWorkers++
			}
			worker.mu.RUnlock()
		}
		enhanced["worker_pool"] = map[string]interface{}{
			"total_workers":     len(ess.workers),
			"active_workers":    activeWorkers,
			"available_workers": len(ess.workerPool),
		}
	}

	return enhanced
}

// StreamDirectly streams data directly without going through CSV
func (ess *EnhancedStreamerService) StreamDirectly(data []*models.TelemetryData) error {
	if !ess.IsRunning() {
		return fmt.Errorf("streamer service is not running")
	}

	// Apply rate limiting
	if ess.rateLimiter != nil {
		for range data {
			if !ess.rateLimiter.allow() {
				time.Sleep(10 * time.Millisecond)
			}
		}
	}

	// Stream data using streaming adapter if available
	if ess.streamAdapter != nil {
		for _, telemetryData := range data {
			headers := map[string]string{
				"streamer_id":   ess.config.StreamerID,
				"gpu_id":        telemetryData.GPUID,
				"hostname":      telemetryData.Hostname,
				"timestamp":     telemetryData.Timestamp.Format(time.RFC3339),
				"metric_name":   telemetryData.MetricName,
				"direct_stream": "true",
			}

			err := ess.streamAdapter.WriteTelemetry(telemetryData, headers)
			if err != nil {
				logging.Errorf("Failed to stream telemetry data directly: %v", err)
				return err
			}
		}
	} else {
		return fmt.Errorf("streaming adapter not available")
	}

	// Update metrics
	ess.updateMetrics(int64(len(data)), 1, 0)
	ess.totalStreamed += int64(len(data))

	logging.Debugf("Direct streamed %d records", len(data))
	return nil
}
