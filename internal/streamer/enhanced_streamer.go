package streamer

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/dilipmighty245/telemetry-pipeline/pkg/config"
	"github.com/dilipmighty245/telemetry-pipeline/pkg/logging"
	"github.com/dilipmighty245/telemetry-pipeline/pkg/messagequeue"
)

// EnhancedStreamer provides streaming with backpressure and throttling
type EnhancedStreamer struct {
	config     *config.CentralizedConfig
	queue      *messagequeue.EnhancedQueue
	csvReader  *CSVReader
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	metrics    *EnhancedStreamerMetrics
	isRunning  bool
	mu         sync.RWMutex
	throttler  *Throttler
	shutdownCh chan struct{}
}

// EnhancedStreamerMetrics tracks enhanced streaming metrics
type EnhancedStreamerMetrics struct {
	MessagesPublished int64     `json:"messages_published"`
	MessagesThrottled int64     `json:"messages_throttled"`
	PublishErrors     int64     `json:"publish_errors"`
	CSVReadErrors     int64     `json:"csv_read_errors"`
	BatchesProcessed  int64     `json:"batches_processed"`
	PublishLatencyMs  int64     `json:"publish_latency_ms"`
	ThrottleRate      float64   `json:"throttle_rate"`
	QueueDepth        int64     `json:"queue_depth"`
	LastPublishedAt   time.Time `json:"last_published_at"`
	StartTime         time.Time `json:"start_time"`
	mu                sync.RWMutex
}

// Throttler implements token bucket throttling for backpressure
type Throttler struct {
	config      *config.ThrottleConfig
	queue       *messagequeue.EnhancedQueue
	tokens      chan struct{}
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
	currentRate float64
	mu          sync.RWMutex
}

// NewEnhancedStreamer creates a new enhanced streamer
func NewEnhancedStreamer(cfg *config.CentralizedConfig) (*EnhancedStreamer, error) {
	// Create enhanced message queue
	queueConfig := &messagequeue.EnhancedQueueConfig{
		EtcdEndpoints:     cfg.Etcd.Endpoints,
		QueuePrefix:       cfg.MessageQueue.QueuePrefix,
		ProcessingPrefix:  cfg.MessageQueue.ProcessingPrefix,
		ProcessedPrefix:   cfg.MessageQueue.ProcessedPrefix,
		DefaultTTL:        cfg.MessageQueue.DefaultTTL,
		ProcessingTimeout: cfg.MessageQueue.ProcessingTimeout,
		BatchSize:         cfg.MessageQueue.BatchSize,
		MaxRetries:        cfg.MessageQueue.MaxRetries,
		CompactionWindow:  cfg.MessageQueue.CompactionWindow,
	}

	queue, err := messagequeue.NewEnhancedQueue(queueConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create enhanced queue: %w", err)
	}

	// Create CSV reader
	csvReader, err := NewCSVReader(cfg.Streamer.CSVFilePath, cfg.Streamer.LoopMode)
	if err != nil {
		queue.Close()
		return nil, fmt.Errorf("failed to create CSV reader: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	streamer := &EnhancedStreamer{
		config:     cfg,
		queue:      queue,
		csvReader:  csvReader,
		ctx:        ctx,
		cancel:     cancel,
		metrics:    &EnhancedStreamerMetrics{StartTime: time.Now()},
		shutdownCh: make(chan struct{}),
	}

	// Create throttler if enabled
	if cfg.Streamer.ThrottleConfig.Enabled {
		throttler, err := NewThrottler(&cfg.Streamer.ThrottleConfig, queue, ctx)
		if err != nil {
			queue.Close()
			csvReader.Close()
			cancel()
			return nil, fmt.Errorf("failed to create throttler: %w", err)
		}
		streamer.throttler = throttler
	}

	logging.Infof("Enhanced streamer initialized for file: %s", cfg.Streamer.CSVFilePath)
	return streamer, nil
}

// NewThrottler creates a new throttler with token bucket algorithm
func NewThrottler(config *config.ThrottleConfig, queue *messagequeue.EnhancedQueue, parentCtx context.Context) (*Throttler, error) {
	ctx, cancel := context.WithCancel(parentCtx)

	throttler := &Throttler{
		config:      config,
		queue:       queue,
		tokens:      make(chan struct{}, config.TokenBucketSize),
		ctx:         ctx,
		cancel:      cancel,
		currentRate: 1.0, // Start at full rate
	}

	// Fill initial token bucket
	for i := 0; i < config.TokenBucketSize; i++ {
		select {
		case throttler.tokens <- struct{}{}:
		default:
			break
		}
	}

	// Start background token refill and monitoring
	throttler.wg.Add(2)
	go throttler.tokenRefiller()
	go throttler.queueMonitor()

	logging.Infof("Throttler initialized with bucket size %d, max queue depth %d",
		config.TokenBucketSize, config.MaxQueueDepth)
	return throttler, nil
}

// Start starts the enhanced streamer
func (es *EnhancedStreamer) Start() error {
	es.mu.Lock()
	defer es.mu.Unlock()

	if es.isRunning {
		return fmt.Errorf("streamer is already running")
	}

	// Set up signal handling for graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)

	// Start throttler if enabled
	if es.throttler != nil {
		if err := es.throttler.Start(); err != nil {
			return fmt.Errorf("failed to start throttler: %w", err)
		}
	}

	// Start streaming loop
	es.wg.Add(1)
	go es.streamLoop()

	// Start metrics updater
	es.wg.Add(1)
	go es.metricsUpdater()

	es.isRunning = true

	// Handle graceful shutdown
	go func() {
		<-sigCh
		logging.Infof("Received shutdown signal, starting graceful shutdown...")
		es.Stop()
	}()

	logging.Infof("Enhanced streamer started")
	return nil
}

// Stop stops the streamer gracefully
func (es *EnhancedStreamer) Stop() error {
	es.mu.Lock()
	defer es.mu.Unlock()

	if !es.isRunning {
		return nil
	}

	logging.Infof("Stopping enhanced streamer...")

	// Cancel context to stop all operations
	es.cancel()

	// Stop throttler
	if es.throttler != nil {
		es.throttler.Stop()
	}

	// Wait for all goroutines to finish with timeout
	done := make(chan struct{})
	go func() {
		es.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		logging.Infof("All streamer workers stopped gracefully")
	case <-time.After(30 * time.Second):
		logging.Warnf("Timeout waiting for streamer workers to stop")
	}

	// Close resources
	if es.queue != nil {
		es.queue.Close()
	}

	if es.csvReader != nil {
		es.csvReader.Close()
	}

	es.isRunning = false
	close(es.shutdownCh)

	logging.Infof("Enhanced streamer stopped")
	return nil
}

// streamLoop is the main streaming loop
func (es *EnhancedStreamer) streamLoop() {
	defer es.wg.Done()

	logging.Infof("Streaming loop started")

	ticker := time.NewTicker(es.config.Streamer.StreamInterval)
	defer ticker.Stop()

	for {
		select {
		case <-es.ctx.Done():
			logging.Infof("Streaming loop stopped")
			return
		case <-ticker.C:
			es.streamBatch()
		}
	}
}

// streamBatch streams a batch of telemetry data
func (es *EnhancedStreamer) streamBatch() {
	startTime := time.Now()

	// Read batch from CSV
	telemetryData, err := es.csvReader.ReadBatch(es.config.Streamer.BatchSize)
	if err != nil && err != io.EOF {
		logging.Errorf("Failed to read batch from CSV: %v", err)
		es.updateMetrics(0, 0, 0, 1, 0)
		return
	}

	if len(telemetryData) == 0 {
		if err == io.EOF && !es.config.Streamer.LoopMode {
			logging.Infof("Reached end of CSV file, stopping streamer")
			go es.Stop()
		}
		return
	}

	logging.Debugf("Streaming batch of %d records", len(telemetryData))

	published := int64(0)
	throttled := int64(0)
	errors := int64(0)

	// Process each record in the batch
	for _, data := range telemetryData {
		// Check if we should stop
		select {
		case <-es.ctx.Done():
			return
		default:
		}

		// Apply throttling if enabled
		if es.throttler != nil {
			if !es.throttler.AllowRequest() {
				throttled++
				continue
			}
		}

		// Create headers
		headers := map[string]string{
			"streamer_id": es.config.Streamer.ID,
			"gpu_id":      data.GPUID,
			"hostname":    data.Hostname,
			"timestamp":   data.Timestamp.Format(time.RFC3339),
			"metric_name": data.MetricName,
		}

		// Convert to JSON
		jsonData, err := json.Marshal(data)
		if err != nil {
			logging.Errorf("Failed to marshal telemetry data: %v", err)
			errors++
			continue
		}

		// Publish to queue with retry logic
		err = es.publishWithRetry(jsonData, headers)
		if err != nil {
			logging.Errorf("Failed to publish telemetry data after retries: %v", err)
			errors++
			continue
		}

		published++
	}

	// Update metrics
	processingLatency := time.Since(startTime).Milliseconds()
	es.updateMetrics(published, throttled, errors, 0, processingLatency)

	if published > 0 {
		logging.Debugf("Streamed batch: %d published, %d throttled, %d errors in %dms",
			published, throttled, errors, processingLatency)
	}
}

// publishWithRetry publishes data with exponential backoff retry
func (es *EnhancedStreamer) publishWithRetry(data []byte, headers map[string]string) error {
	var lastErr error

	for attempt := 0; attempt < es.config.Streamer.MaxRetries; attempt++ {
		// Check if we should stop
		select {
		case <-es.ctx.Done():
			return es.ctx.Err()
		default:
		}

		err := es.queue.PublishMessage(es.ctx, "telemetry", data, headers)
		if err == nil {
			return nil // Success
		}

		lastErr = err
		if attempt < es.config.Streamer.MaxRetries-1 {
			// Exponential backoff
			backoff := time.Duration(attempt+1) * es.config.Streamer.RetryDelay
			select {
			case <-es.ctx.Done():
				return es.ctx.Err()
			case <-time.After(backoff):
				// Continue with retry
			}
		}
	}

	return lastErr
}

// metricsUpdater periodically updates queue metrics
func (es *EnhancedStreamer) metricsUpdater() {
	defer es.wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-es.ctx.Done():
			return
		case <-ticker.C:
			// Update queue depth metrics
			depth, err := es.queue.GetQueueDepth(es.ctx, "telemetry")
			if err != nil {
				logging.Errorf("Failed to get queue depth: %v", err)
				continue
			}

			es.metrics.mu.Lock()
			es.metrics.QueueDepth = depth
			if es.throttler != nil {
				es.metrics.ThrottleRate = es.throttler.GetCurrentRate()
			}
			es.metrics.mu.Unlock()
		}
	}
}

// Start starts the throttler
func (t *Throttler) Start() error {
	logging.Infof("Throttler started")
	return nil
}

// Stop stops the throttler
func (t *Throttler) Stop() {
	t.cancel()
	t.wg.Wait()
	logging.Infof("Throttler stopped")
}

// AllowRequest checks if a request should be allowed (token bucket)
func (t *Throttler) AllowRequest() bool {
	select {
	case <-t.tokens:
		return true
	default:
		return false
	}
}

// GetCurrentRate returns the current throttle rate
func (t *Throttler) GetCurrentRate() float64 {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.currentRate
}

// tokenRefiller refills the token bucket at the configured rate
func (t *Throttler) tokenRefiller() {
	defer t.wg.Done()

	// Calculate refill interval based on current rate
	refillInterval := time.Duration(float64(time.Second) / float64(t.config.TokenBucketSize))
	ticker := time.NewTicker(refillInterval)
	defer ticker.Stop()

	for {
		select {
		case <-t.ctx.Done():
			return
		case <-ticker.C:
			// Add token if bucket not full and rate allows
			t.mu.RLock()
			currentRate := t.currentRate
			t.mu.RUnlock()

			if currentRate > 0 {
				select {
				case t.tokens <- struct{}{}:
					// Token added
				default:
					// Bucket full
				}
			}

			// Adjust ticker based on current rate
			newInterval := time.Duration(float64(time.Second) / (float64(t.config.TokenBucketSize) * currentRate))
			ticker.Reset(newInterval)
		}
	}
}

// queueMonitor monitors queue depth and adjusts throttle rate
func (t *Throttler) queueMonitor() {
	defer t.wg.Done()

	ticker := time.NewTicker(t.config.CheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-t.ctx.Done():
			return
		case <-ticker.C:
			// Get current queue depth
			depth, err := t.queue.GetQueueDepth(t.ctx, "telemetry")
			if err != nil {
				logging.Errorf("Failed to get queue depth for throttling: %v", err)
				continue
			}

			// Calculate new throttle rate based on queue depth
			newRate := t.calculateThrottleRate(depth)

			t.mu.Lock()
			if newRate != t.currentRate {
				logging.Debugf("Adjusting throttle rate from %.2f to %.2f (queue depth: %d)",
					t.currentRate, newRate, depth)
				t.currentRate = newRate
			}
			t.mu.Unlock()
		}
	}
}

// calculateThrottleRate calculates throttle rate based on queue depth
func (t *Throttler) calculateThrottleRate(queueDepth int64) float64 {
	if queueDepth <= t.config.MaxQueueDepth/2 {
		return 1.0 // Full rate when queue is less than half full
	}

	if queueDepth >= t.config.MaxQueueDepth {
		return t.config.ThrottleRate // Minimum rate when queue is full
	}

	// Linear interpolation between half-full and full
	ratio := float64(queueDepth-t.config.MaxQueueDepth/2) / float64(t.config.MaxQueueDepth/2)
	return 1.0 - (1.0-t.config.ThrottleRate)*ratio
}

// updateMetrics updates streamer metrics
func (es *EnhancedStreamer) updateMetrics(published, throttled, publishErrors, csvErrors int64, latency int64) {
	es.metrics.mu.Lock()
	defer es.metrics.mu.Unlock()

	es.metrics.MessagesPublished += published
	es.metrics.MessagesThrottled += throttled
	es.metrics.PublishErrors += publishErrors
	es.metrics.CSVReadErrors += csvErrors

	if published > 0 {
		es.metrics.BatchesProcessed++
		es.metrics.LastPublishedAt = time.Now()
	}

	if latency > 0 {
		// Simple moving average for latency
		if es.metrics.PublishLatencyMs == 0 {
			es.metrics.PublishLatencyMs = latency
		} else {
			es.metrics.PublishLatencyMs = (es.metrics.PublishLatencyMs + latency) / 2
		}
	}
}

// GetMetrics returns current streamer metrics
func (es *EnhancedStreamer) GetMetrics() *EnhancedStreamerMetrics {
	es.metrics.mu.RLock()
	defer es.metrics.mu.RUnlock()

	return &EnhancedStreamerMetrics{
		MessagesPublished: es.metrics.MessagesPublished,
		MessagesThrottled: es.metrics.MessagesThrottled,
		PublishErrors:     es.metrics.PublishErrors,
		CSVReadErrors:     es.metrics.CSVReadErrors,
		BatchesProcessed:  es.metrics.BatchesProcessed,
		PublishLatencyMs:  es.metrics.PublishLatencyMs,
		ThrottleRate:      es.metrics.ThrottleRate,
		QueueDepth:        es.metrics.QueueDepth,
		LastPublishedAt:   es.metrics.LastPublishedAt,
		StartTime:         es.metrics.StartTime,
	}
}

// IsRunning returns whether the streamer is running
func (es *EnhancedStreamer) IsRunning() bool {
	es.mu.RLock()
	defer es.mu.RUnlock()
	return es.isRunning
}

// Health returns the health status of the streamer
func (es *EnhancedStreamer) Health() bool {
	if !es.IsRunning() {
		return false
	}

	// Check if we have recent activity (within last 5 minutes)
	es.metrics.mu.RLock()
	lastActivity := es.metrics.LastPublishedAt
	es.metrics.mu.RUnlock()

	if !lastActivity.IsZero() && time.Since(lastActivity) > 5*time.Minute {
		return false
	}

	return true
}

// WaitForShutdown waits for the streamer to shutdown
func (es *EnhancedStreamer) WaitForShutdown() {
	<-es.shutdownCh
}
