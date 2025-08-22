package collector

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/dilipmighty245/telemetry-pipeline/pkg/config"
	"github.com/dilipmighty245/telemetry-pipeline/pkg/logging"
	"github.com/dilipmighty245/telemetry-pipeline/pkg/messagequeue"
	"github.com/dilipmighty245/telemetry-pipeline/pkg/models"
	clientv3 "go.etcd.io/etcd/client/v3"
)

// EnhancedCollector provides exactly-once processing with graceful shutdown
type EnhancedCollector struct {
	config         *config.CentralizedConfig
	queue          *messagequeue.EnhancedQueue
	etcdClient     *clientv3.Client
	ctx            context.Context
	cancel         context.CancelFunc
	wg             sync.WaitGroup
	metrics        *EnhancedCollectorMetrics
	isRunning      bool
	mu             sync.RWMutex
	workers        []*EnhancedCollectorWorker
	batchProcessor *BatchProcessor
	shutdownCh     chan struct{}
}

// EnhancedCollectorMetrics tracks enhanced collection metrics
type EnhancedCollectorMetrics struct {
	MessagesConsumed    int64     `json:"messages_consumed"`
	MessagesProcessed   int64     `json:"messages_processed"`
	MessagesDuplicated  int64     `json:"messages_duplicated"`
	MessagesInvalid     int64     `json:"messages_invalid"`
	BatchesProcessed    int64     `json:"batches_processed"`
	ProcessingErrors    int64     `json:"processing_errors"`
	DatabaseErrors      int64     `json:"database_errors"`
	ProcessingLatencyMs int64     `json:"processing_latency_ms"`
	QueueLag            int64     `json:"queue_lag_ms"`
	LastProcessedAt     time.Time `json:"last_processed_at"`
	StartTime           time.Time `json:"start_time"`
	mu                  sync.RWMutex
}

// EnhancedCollectorWorker represents a worker that processes messages
type EnhancedCollectorWorker struct {
	id        int
	collector *EnhancedCollector
	ctx       context.Context
	cancel    context.CancelFunc
}

// BatchProcessor handles batch processing and etcd storage operations
type BatchProcessor struct {
	collector *EnhancedCollector
	batchCh   chan []*models.VersionedTelemetryData
	ctx       context.Context
	cancel    context.CancelFunc
	mu        sync.Mutex
}

// NewEnhancedCollector creates a new enhanced collector
func NewEnhancedCollector(cfg *config.CentralizedConfig) (*EnhancedCollector, error) {
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

	// Create etcd client for data storage
	etcdClient, err := clientv3.New(clientv3.Config{
		Endpoints:   cfg.Etcd.Endpoints,
		DialTimeout: cfg.Etcd.DialTimeout,
	})
	if err != nil {
		queue.Close()
		return nil, fmt.Errorf("failed to connect to etcd: %w", err)
	}

	// Test etcd connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	_, err = etcdClient.Status(ctx, cfg.Etcd.Endpoints[0])
	cancel()
	if err != nil {
		queue.Close()
		etcdClient.Close()
		return nil, fmt.Errorf("failed to connect to etcd: %w", err)
	}

	ctx, cancel = context.WithCancel(context.Background())

	collector := &EnhancedCollector{
		config:     cfg,
		queue:      queue,
		etcdClient: etcdClient,
		ctx:        ctx,
		cancel:     cancel,
		metrics:    &EnhancedCollectorMetrics{StartTime: time.Now()},
		shutdownCh: make(chan struct{}),
	}

	// Create batch processor
	batchCtx, batchCancel := context.WithCancel(ctx)
	collector.batchProcessor = &BatchProcessor{
		collector: collector,
		batchCh:   make(chan []*models.VersionedTelemetryData, 100),
		ctx:       batchCtx,
		cancel:    batchCancel,
	}

	logging.Infof("Enhanced collector initialized with %d workers", cfg.Collector.WorkerCount)
	return collector, nil
}

// Start starts the enhanced collector with graceful shutdown support
func (ec *EnhancedCollector) Start() error {
	ec.mu.Lock()
	defer ec.mu.Unlock()

	if ec.isRunning {
		return fmt.Errorf("collector is already running")
	}

	// Set up signal handling for graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)

	// Start batch processor
	ec.wg.Add(1)
	go ec.batchProcessor.start()

	// Start worker goroutines
	ec.workers = make([]*EnhancedCollectorWorker, ec.config.Collector.WorkerCount)
	for i := 0; i < ec.config.Collector.WorkerCount; i++ {
		worker := &EnhancedCollectorWorker{
			id:        i,
			collector: ec,
		}
		worker.ctx, worker.cancel = context.WithCancel(ec.ctx)
		ec.workers[i] = worker

		ec.wg.Add(1)
		go worker.run()
	}

	// Start metrics updater
	ec.wg.Add(1)
	go ec.metricsUpdater()

	// Start orphaned work recovery
	ec.wg.Add(1)
	go ec.orphanedWorkRecovery()

	ec.isRunning = true

	// Handle graceful shutdown
	go func() {
		<-sigCh
		logging.Infof("Received shutdown signal, starting graceful shutdown...")
		ec.Stop()
	}()

	logging.Infof("Enhanced collector started with %d workers", len(ec.workers))
	return nil
}

// Stop stops the collector gracefully
func (ec *EnhancedCollector) Stop() error {
	ec.mu.Lock()
	defer ec.mu.Unlock()

	if !ec.isRunning {
		return nil
	}

	logging.Infof("Stopping enhanced collector...")

	// Cancel context to stop all operations
	ec.cancel()

	// Stop all workers
	for _, worker := range ec.workers {
		if worker.cancel != nil {
			worker.cancel()
		}
	}

	// Stop batch processor
	if ec.batchProcessor != nil {
		ec.batchProcessor.cancel()
	}

	// Wait for all goroutines to finish with timeout
	done := make(chan struct{})
	go func() {
		ec.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		logging.Infof("All collector workers stopped gracefully")
	case <-time.After(30 * time.Second):
		logging.Warnf("Timeout waiting for workers to stop")
	}

	// Close resources
	if ec.queue != nil {
		ec.queue.Close()
	}

	if ec.etcdClient != nil {
		ec.etcdClient.Close()
	}

	ec.isRunning = false
	close(ec.shutdownCh)

	logging.Infof("Enhanced collector stopped")
	return nil
}

// run runs a collector worker
func (cw *EnhancedCollectorWorker) run() {
	defer cw.collector.wg.Done()

	logging.Infof("Collector worker %d started", cw.id)

	ticker := time.NewTicker(cw.collector.config.Collector.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-cw.ctx.Done():
			logging.Infof("Collector worker %d stopped", cw.id)
			return
		case <-ticker.C:
			cw.processBatch()
		}
	}
}

// processBatch processes a batch of messages
func (cw *EnhancedCollectorWorker) processBatch() {
	ctx := cw.ctx

	startTime := time.Now()

	// Consume messages from queue
	messages, err := cw.collector.queue.ConsumeMessages(ctx, "telemetry", cw.collector.config.Collector.ID, cw.collector.config.Collector.BatchSize)
	if err != nil {
		logging.Errorf("Worker %d failed to consume messages: %v", cw.id, err)
		cw.collector.updateMetrics(0, 0, 0, 0, 0, 1, 0, 0)
		return
	}

	if len(messages) == 0 {
		return // No messages to process
	}

	logging.Debugf("Worker %d processing %d messages", cw.id, len(messages))

	// Process and validate messages
	var validMessages []*models.VersionedTelemetryData
	var processedMessages []*messagequeue.EnhancedMessage
	duplicateCount := int64(0)
	invalidCount := int64(0)

	for _, msg := range messages {
		// Parse message payload
		var rawData map[string]interface{}
		if err := json.Unmarshal(msg.Payload, &rawData); err != nil {
			logging.Errorf("Failed to unmarshal message payload: %v", err)
			invalidCount++
			continue
		}

		// Create versioned telemetry data
		versionedData := models.NewVersionedTelemetryData(msg.ID, msg.TraceID, rawData)

		// Validate against schema
		validationResult := versionedData.Validate()
		if !validationResult.Valid {
			logging.Warnf("Invalid message %s: %v", msg.ID, validationResult.Errors)
			invalidCount++
			// Still acknowledge invalid messages to prevent reprocessing
			processedMessages = append(processedMessages, msg)
			continue
		}

		validMessages = append(validMessages, versionedData)
		processedMessages = append(processedMessages, msg)
	}

	// Send valid messages for batch processing
	if len(validMessages) > 0 {
		select {
		case cw.collector.batchProcessor.batchCh <- validMessages:
		case <-ctx.Done():
			return
		}
	}

	// Acknowledge all processed messages (valid and invalid)
	if len(processedMessages) > 0 {
		if err := cw.collector.queue.AcknowledgeMessages(ctx, cw.collector.config.Collector.ID, processedMessages); err != nil {
			logging.Errorf("Failed to acknowledge messages: %v", err)
			cw.collector.updateMetrics(0, 0, duplicateCount, invalidCount, 0, 1, 0, 0)
			return
		}
	}

	// Update metrics
	processingLatency := time.Since(startTime).Milliseconds()
	cw.collector.updateMetrics(int64(len(messages)), int64(len(validMessages)), duplicateCount, invalidCount, 0, 0, 0, processingLatency)

	logging.Debugf("Worker %d processed %d messages (%d valid, %d invalid) in %dms",
		cw.id, len(messages), len(validMessages), invalidCount, processingLatency)
}

// start starts the batch processor
func (bp *BatchProcessor) start() {
	defer bp.collector.wg.Done()

	logging.Infof("Batch processor started")

	batchTimer := time.NewTicker(bp.collector.config.Performance.BatchCommit.Timeout)
	defer batchTimer.Stop()

	var currentBatch []*models.VersionedTelemetryData
	maxBatchSize := bp.collector.config.Performance.BatchCommit.Size

	for {
		select {
		case <-bp.ctx.Done():
			// Process remaining batch before stopping
			if len(currentBatch) > 0 {
				bp.processBatch(currentBatch)
			}
			logging.Infof("Batch processor stopped")
			return

		case batch := <-bp.batchCh:
			currentBatch = append(currentBatch, batch...)

			// Process batch if it reaches max size
			if len(currentBatch) >= maxBatchSize {
				bp.processBatch(currentBatch)
				currentBatch = nil
				batchTimer.Reset(bp.collector.config.Performance.BatchCommit.Timeout)
			}

		case <-batchTimer.C:
			// Process batch on timeout
			if len(currentBatch) > 0 {
				bp.processBatch(currentBatch)
				currentBatch = nil
			}
		}
	}
}

// processBatch processes a batch of validated telemetry data
func (bp *BatchProcessor) processBatch(batch []*models.VersionedTelemetryData) {
	ctx := bp.ctx

	startTime := time.Now()

	logging.Debugf("Processing batch of %d telemetry records", len(batch))

	// Convert to V1 format for etcd storage
	var etcdOps []clientv3.Op
	for _, vData := range batch {
		v1Data, err := vData.ConvertToV1()
		if err != nil {
			logging.Errorf("Failed to convert to V1 format: %v", err)
			continue
		}

		// Create etcd key for telemetry data
		// Format: /telemetry/data/{hostname}/{gpu_id}/{timestamp_unix_nano}_{message_id}
		key := fmt.Sprintf("/telemetry/data/%s/%s/%d_%s",
			v1Data.Hostname, v1Data.GPUID, v1Data.Timestamp.UnixNano(), v1Data.MessageID)

		// Serialize data to JSON
		data, err := json.Marshal(v1Data)
		if err != nil {
			logging.Errorf("Failed to marshal telemetry data: %v", err)
			continue
		}

		// Create lease for data retention
		lease, err := bp.collector.etcdClient.Grant(ctx, int64(bp.collector.config.Database.Retention.EtcdRetention.Seconds()))
		if err != nil {
			logging.Errorf("Failed to create lease for telemetry data: %v", err)
			continue
		}

		etcdOps = append(etcdOps, clientv3.OpPut(key, string(data), clientv3.WithLease(lease.ID)))
	}

	if len(etcdOps) == 0 {
		return
	}

	// Batch store in etcd using transaction
	_, err := bp.collector.etcdClient.Txn(ctx).Then(etcdOps...).Commit()
	if err != nil {
		logging.Errorf("Failed to store batch in etcd: %v", err)
		bp.collector.updateMetrics(0, 0, 0, 0, 0, 0, 1, 0)
		return
	}

	// Update metrics
	processingLatency := time.Since(startTime).Milliseconds()
	bp.collector.updateMetrics(0, 0, 0, 0, 1, 0, 0, processingLatency)

	logging.Debugf("Batch processor stored %d records in etcd in %dms", len(etcdOps), processingLatency)
}

// metricsUpdater periodically updates queue metrics
func (ec *EnhancedCollector) metricsUpdater() {
	defer ec.wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ec.ctx.Done():
			return
		case <-ticker.C:
			// Update queue depth metrics
			depth, err := ec.queue.GetQueueDepth(ec.ctx, "telemetry")
			if err != nil {
				logging.Errorf("Failed to get queue depth: %v", err)
				continue
			}

			ec.metrics.mu.Lock()
			// Calculate queue lag (simplified - could be more sophisticated)
			if depth > 0 {
				ec.metrics.QueueLag = time.Since(ec.metrics.LastProcessedAt).Milliseconds()
			} else {
				ec.metrics.QueueLag = 0
			}
			ec.metrics.mu.Unlock()
		}
	}
}

// orphanedWorkRecovery periodically recovers orphaned work
func (ec *EnhancedCollector) orphanedWorkRecovery() {
	defer ec.wg.Done()

	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ec.ctx.Done():
			return
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(ec.ctx, time.Minute)
			if err := ec.queue.RecoverOrphanedWork(ctx); err != nil {
				logging.Errorf("Failed to recover orphaned work: %v", err)
			}
			cancel()
		}
	}
}

// updateMetrics updates collector metrics
func (ec *EnhancedCollector) updateMetrics(consumed, processed, duplicated, invalid, batches, processingErrors, dbErrors, latency int64) {
	ec.metrics.mu.Lock()
	defer ec.metrics.mu.Unlock()

	ec.metrics.MessagesConsumed += consumed
	ec.metrics.MessagesProcessed += processed
	ec.metrics.MessagesDuplicated += duplicated
	ec.metrics.MessagesInvalid += invalid
	ec.metrics.BatchesProcessed += batches
	ec.metrics.ProcessingErrors += processingErrors
	ec.metrics.DatabaseErrors += dbErrors

	if latency > 0 {
		// Simple moving average for latency
		if ec.metrics.ProcessingLatencyMs == 0 {
			ec.metrics.ProcessingLatencyMs = latency
		} else {
			ec.metrics.ProcessingLatencyMs = (ec.metrics.ProcessingLatencyMs + latency) / 2
		}
	}

	if processed > 0 {
		ec.metrics.LastProcessedAt = time.Now()
	}
}

// GetMetrics returns current collector metrics
func (ec *EnhancedCollector) GetMetrics() *EnhancedCollectorMetrics {
	ec.metrics.mu.RLock()
	defer ec.metrics.mu.RUnlock()

	return &EnhancedCollectorMetrics{
		MessagesConsumed:    ec.metrics.MessagesConsumed,
		MessagesProcessed:   ec.metrics.MessagesProcessed,
		MessagesDuplicated:  ec.metrics.MessagesDuplicated,
		MessagesInvalid:     ec.metrics.MessagesInvalid,
		BatchesProcessed:    ec.metrics.BatchesProcessed,
		ProcessingErrors:    ec.metrics.ProcessingErrors,
		DatabaseErrors:      ec.metrics.DatabaseErrors,
		ProcessingLatencyMs: ec.metrics.ProcessingLatencyMs,
		QueueLag:            ec.metrics.QueueLag,
		LastProcessedAt:     ec.metrics.LastProcessedAt,
		StartTime:           ec.metrics.StartTime,
	}
}

// IsRunning returns whether the collector is running
func (ec *EnhancedCollector) IsRunning() bool {
	ec.mu.RLock()
	defer ec.mu.RUnlock()
	return ec.isRunning
}

// Health returns the health status of the collector
func (ec *EnhancedCollector) Health() bool {
	if !ec.IsRunning() {
		return false
	}

	// Check etcd connectivity
	if ec.etcdClient != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_, err := ec.etcdClient.Status(ctx, ec.config.Etcd.Endpoints[0])
		cancel()
		if err != nil {
			return false
		}
	}

	// Check if we have recent activity (within last 5 minutes)
	ec.metrics.mu.RLock()
	lastActivity := ec.metrics.LastProcessedAt
	ec.metrics.mu.RUnlock()

	if !lastActivity.IsZero() && time.Since(lastActivity) > 5*time.Minute {
		return false
	}

	return true
}

// WaitForShutdown waits for the collector to shutdown
func (ec *EnhancedCollector) WaitForShutdown() {
	<-ec.shutdownCh
}
