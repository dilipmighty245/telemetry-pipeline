package collector

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/cf/telemetry-pipeline/internal/streaming"
	"github.com/cf/telemetry-pipeline/pkg/logging"
	"github.com/cf/telemetry-pipeline/pkg/messagequeue"
	"github.com/cf/telemetry-pipeline/pkg/models"
)

// EnhancedCollectorConfig extends CollectorConfig with streaming capabilities
type EnhancedCollectorConfig struct {
	*CollectorConfig
	StreamingConfig   *streaming.StreamAdapterConfig `json:"streaming_config"`
	EnableStreaming   bool                           `json:"enable_streaming"`
	StreamDestination string                         `json:"stream_destination"`

	// Advanced features
	EnableCircuitBreaker bool                  `json:"enable_circuit_breaker"`
	CircuitBreakerConfig *CircuitBreakerConfig `json:"circuit_breaker_config"`

	// Adaptive batching
	EnableAdaptiveBatching bool `json:"enable_adaptive_batching"`
	MinBatchSize           int  `json:"min_batch_size"`
	MaxBatchSize           int  `json:"max_batch_size"`

	// Load balancing
	EnableLoadBalancing bool `json:"enable_load_balancing"`
	Workers             int  `json:"workers"`
}

// CircuitBreakerConfig configures circuit breaker behavior
type CircuitBreakerConfig struct {
	FailureThreshold int           `json:"failure_threshold"`
	RecoveryTimeout  time.Duration `json:"recovery_timeout"`
	HalfOpenRequests int           `json:"half_open_requests"`
}

// CircuitBreakerState represents circuit breaker states
type CircuitBreakerState int

const (
	Closed CircuitBreakerState = iota
	Open
	HalfOpen
)

// CircuitBreaker implements circuit breaker pattern for fault tolerance
type CircuitBreaker struct {
	config       *CircuitBreakerConfig
	state        CircuitBreakerState
	failures     int
	lastFailure  time.Time
	halfOpenReqs int
	mu           sync.RWMutex
}

// EnhancedCollectorService provides high-throughput collection with streaming
type EnhancedCollectorService struct {
	*CollectorService
	config          *EnhancedCollectorConfig
	streamAdapter   *streaming.StreamAdapter
	circuitBreaker  *CircuitBreaker
	adaptiveBatcher *AdaptiveBatcher
	workers         []*CollectorWorker
	workerPool      chan *CollectorWorker
	isEnhanced      bool
}

// CollectorWorker handles parallel processing
type CollectorWorker struct {
	id       int
	service  *EnhancedCollectorService
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	isActive bool
	mu       sync.RWMutex
}

// AdaptiveBatcher dynamically adjusts batch sizes based on load
type AdaptiveBatcher struct {
	config           *EnhancedCollectorConfig
	currentBatchSize int
	loadAverage      float64
	lastAdjustment   time.Time
	mu               sync.RWMutex
}

// NewEnhancedCollectorService creates a new enhanced collector service
func NewEnhancedCollectorService(config *EnhancedCollectorConfig, mqService *messagequeue.MessageQueueService) (*EnhancedCollectorService, error) {
	// Create base collector service
	baseService, err := NewCollectorService(config.CollectorConfig, mqService)
	if err != nil {
		return nil, err
	}

	// Set defaults for enhanced features
	if config.StreamingConfig == nil && config.EnableStreaming {
		config.StreamingConfig = &streaming.StreamAdapterConfig{
			ChannelSize:   1000,
			BatchSize:     500,
			Workers:       5,
			MaxRetries:    3,
			RetryDelay:    time.Second,
			FlushInterval: 5 * time.Second,
			HTTPTimeout:   30 * time.Second,
			EnableMetrics: true,
			PartitionBy:   "hostname",
		}
	}

	if config.CircuitBreakerConfig == nil && config.EnableCircuitBreaker {
		config.CircuitBreakerConfig = &CircuitBreakerConfig{
			FailureThreshold: 5,
			RecoveryTimeout:  30 * time.Second,
			HalfOpenRequests: 3,
		}
	}

	if config.Workers <= 0 {
		config.Workers = 5
	}

	if config.MinBatchSize <= 0 {
		config.MinBatchSize = 50
	}
	if config.MaxBatchSize <= 0 {
		config.MaxBatchSize = 1000
	}

	enhanced := &EnhancedCollectorService{
		CollectorService: baseService,
		config:           config,
		isEnhanced:       true,
	}

	// Initialize streaming adapter if enabled
	if config.EnableStreaming {
		enhanced.streamAdapter = streaming.NewStreamAdapter(config.StreamingConfig, config.StreamDestination)
	}

	// Initialize circuit breaker if enabled
	if config.EnableCircuitBreaker {
		enhanced.circuitBreaker = &CircuitBreaker{
			config: config.CircuitBreakerConfig,
			state:  Closed,
		}
	}

	// Initialize adaptive batcher if enabled
	if config.EnableAdaptiveBatching {
		enhanced.adaptiveBatcher = &AdaptiveBatcher{
			config:           config,
			currentBatchSize: config.BatchSize,
			lastAdjustment:   time.Now(),
		}
	}

	// Initialize worker pool if load balancing is enabled
	if config.EnableLoadBalancing {
		enhanced.workerPool = make(chan *CollectorWorker, config.Workers)
		enhanced.workers = make([]*CollectorWorker, config.Workers)

		for i := 0; i < config.Workers; i++ {
			ctx, cancel := context.WithCancel(context.Background())
			worker := &CollectorWorker{
				id:      i,
				service: enhanced,
				ctx:     ctx,
				cancel:  cancel,
			}
			enhanced.workers[i] = worker
			enhanced.workerPool <- worker
		}
	}

	logging.Infof("Created enhanced collector service with streaming=%v, circuit_breaker=%v, adaptive_batching=%v, load_balancing=%v",
		config.EnableStreaming, config.EnableCircuitBreaker, config.EnableAdaptiveBatching, config.EnableLoadBalancing)

	return enhanced, nil
}

// Start starts the enhanced collector service
func (ecs *EnhancedCollectorService) Start() error {
	// Start base service
	err := ecs.CollectorService.Start()
	if err != nil {
		return err
	}

	// Start streaming adapter
	if ecs.streamAdapter != nil {
		err = ecs.streamAdapter.Start()
		if err != nil {
			logging.Errorf("Failed to start stream adapter: %v", err)
			return err
		}
	}

	// Start worker pool
	if ecs.config.EnableLoadBalancing {
		for _, worker := range ecs.workers {
			go worker.start()
		}
	}

	// Start adaptive batcher monitor
	if ecs.adaptiveBatcher != nil {
		go ecs.adaptiveBatcherMonitor()
	}

	logging.Infof("Started enhanced collector service")
	return nil
}

// Stop stops the enhanced collector service
func (ecs *EnhancedCollectorService) Stop() error {
	// Stop streaming adapter
	if ecs.streamAdapter != nil {
		err := ecs.streamAdapter.Stop()
		if err != nil {
			logging.Errorf("Failed to stop stream adapter: %v", err)
		}
	}

	// Stop workers
	if ecs.config.EnableLoadBalancing {
		for _, worker := range ecs.workers {
			worker.stop()
		}
	}

	// Stop base service
	return ecs.CollectorService.Stop()
}

// collectBatch overrides base collectBatch with enhanced features
func (ecs *EnhancedCollectorService) collectBatch() error {
	// Check circuit breaker
	if ecs.circuitBreaker != nil && !ecs.circuitBreaker.canExecute() {
		logging.Warnf("Circuit breaker is open, skipping collection")
		return nil
	}

	// Get adaptive batch size
	batchSize := ecs.getBatchSize()

	// Use worker pool if enabled
	if ecs.config.EnableLoadBalancing {
		return ecs.collectBatchWithWorkers(batchSize)
	}

	// Fallback to enhanced single-threaded collection
	return ecs.collectBatchEnhanced(batchSize)
}

// collectBatchWithWorkers distributes work across worker pool
func (ecs *EnhancedCollectorService) collectBatchWithWorkers(batchSize int) error {
	select {
	case worker := <-ecs.workerPool:
		go func() {
			defer func() {
				ecs.workerPool <- worker // Return worker to pool
			}()

			err := worker.collectBatch(batchSize)
			if err != nil {
				if ecs.circuitBreaker != nil {
					ecs.circuitBreaker.recordFailure()
				}
				logging.Errorf("Worker %d collection failed: %v", worker.id, err)
			} else {
				if ecs.circuitBreaker != nil {
					ecs.circuitBreaker.recordSuccess()
				}
			}
		}()
		return nil
	default:
		// No workers available, fallback to direct processing
		return ecs.collectBatchEnhanced(batchSize)
	}
}

// collectBatchEnhanced performs enhanced collection with streaming
func (ecs *EnhancedCollectorService) collectBatchEnhanced(batchSize int) error {
	// Consume messages from message queue
	messages, err := ecs.messageQueue.ConsumeTelemetry(
		ecs.config.ConsumerGroup,
		ecs.config.CollectorID,
		batchSize,
	)
	if err != nil {
		if ecs.circuitBreaker != nil {
			ecs.circuitBreaker.recordFailure()
		}
		return err
	}

	if len(messages) == 0 {
		return nil
	}

	var telemetryData []*models.TelemetryData
	var messageIDs []string

	// Parse messages
	for _, msg := range messages {
		var data models.TelemetryData
		err := json.Unmarshal(msg.Payload, &data)
		if err != nil {
			logging.Warnf("Failed to unmarshal telemetry data: %v", err)
			continue
		}

		telemetryData = append(telemetryData, &data)
		messageIDs = append(messageIDs, msg.ID)
	}

	if len(telemetryData) == 0 {
		return nil
	}

	// Stream data if streaming is enabled
	if ecs.streamAdapter != nil {
		for _, data := range telemetryData {
			headers := map[string]string{
				"collector_id": ecs.config.CollectorID,
				"batch_size":   fmt.Sprintf("%d", len(telemetryData)),
			}

			err := ecs.streamAdapter.WriteTelemetry(data, headers)
			if err != nil {
				logging.Warnf("Failed to stream telemetry data: %v", err)
				// Continue with database storage as fallback
			}
		}
	}

	// Add to buffer (existing functionality)
	ecs.addToBuffer(telemetryData)

	// Acknowledge processed messages
	err = ecs.messageQueue.AcknowledgeMessages(ecs.config.CollectorID, messageIDs)
	if err != nil {
		logging.Errorf("Failed to acknowledge messages: %v", err)
	}

	// Update adaptive batcher metrics
	if ecs.adaptiveBatcher != nil {
		ecs.adaptiveBatcher.updateMetrics(len(telemetryData), time.Since(time.Now()))
	}

	// Record success in circuit breaker
	if ecs.circuitBreaker != nil {
		ecs.circuitBreaker.recordSuccess()
	}

	// Update metrics
	ecs.updateMetrics(int64(len(telemetryData)), 1, 0, 0)
	ecs.totalCollected += int64(len(telemetryData))

	return nil
}

// getBatchSize returns the current batch size (adaptive or fixed)
func (ecs *EnhancedCollectorService) getBatchSize() int {
	if ecs.adaptiveBatcher != nil {
		return ecs.adaptiveBatcher.getCurrentBatchSize()
	}
	return ecs.config.BatchSize
}

// adaptiveBatcherMonitor adjusts batch sizes based on system load
func (ecs *EnhancedCollectorService) adaptiveBatcherMonitor() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ecs.ctx.Done():
			return
		case <-ticker.C:
			if ecs.adaptiveBatcher != nil {
				ecs.adaptiveBatcher.adjustBatchSize()
			}
		}
	}
}

// CollectorWorker methods

func (cw *CollectorWorker) start() {
	cw.mu.Lock()
	cw.isActive = true
	cw.mu.Unlock()

	logging.Infof("Started collector worker %d", cw.id)
}

func (cw *CollectorWorker) stop() {
	cw.mu.Lock()
	defer cw.mu.Unlock()

	if !cw.isActive {
		return
	}

	cw.isActive = false
	cw.cancel()
	cw.wg.Wait()

	logging.Infof("Stopped collector worker %d", cw.id)
}

func (cw *CollectorWorker) collectBatch(batchSize int) error {
	// Delegate to enhanced service's collection logic
	return cw.service.collectBatchEnhanced(batchSize)
}

// Circuit breaker methods

func (cb *CircuitBreaker) canExecute() bool {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	switch cb.state {
	case Closed:
		return true
	case Open:
		if time.Since(cb.lastFailure) > cb.config.RecoveryTimeout {
			cb.state = HalfOpen
			cb.halfOpenReqs = 0
			return true
		}
		return false
	case HalfOpen:
		return cb.halfOpenReqs < cb.config.HalfOpenRequests
	default:
		return false
	}
}

func (cb *CircuitBreaker) recordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failures = 0
	if cb.state == HalfOpen {
		cb.state = Closed
	}
}

func (cb *CircuitBreaker) recordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failures++
	cb.lastFailure = time.Now()

	if cb.state == HalfOpen {
		cb.state = Open
	} else if cb.failures >= cb.config.FailureThreshold {
		cb.state = Open
	}
}

// Adaptive batcher methods

func (ab *AdaptiveBatcher) getCurrentBatchSize() int {
	ab.mu.RLock()
	defer ab.mu.RUnlock()
	return ab.currentBatchSize
}

func (ab *AdaptiveBatcher) updateMetrics(recordsProcessed int, processingTime time.Duration) {
	ab.mu.Lock()
	defer ab.mu.Unlock()

	// Simple load average calculation
	newLoad := float64(recordsProcessed) / processingTime.Seconds()
	ab.loadAverage = (ab.loadAverage + newLoad) / 2
}

func (ab *AdaptiveBatcher) adjustBatchSize() {
	ab.mu.Lock()
	defer ab.mu.Unlock()

	if time.Since(ab.lastAdjustment) < 60*time.Second {
		return // Don't adjust too frequently
	}

	// Increase batch size if load is high, decrease if low
	if ab.loadAverage > 1000 { // High load
		newSize := int(float64(ab.currentBatchSize) * 1.2)
		if newSize <= ab.config.MaxBatchSize {
			ab.currentBatchSize = newSize
		}
	} else if ab.loadAverage < 100 { // Low load
		newSize := int(float64(ab.currentBatchSize) * 0.8)
		if newSize >= ab.config.MinBatchSize {
			ab.currentBatchSize = newSize
		}
	}

	ab.lastAdjustment = time.Now()
	logging.Infof("Adjusted batch size to %d (load average: %.2f)", ab.currentBatchSize, ab.loadAverage)
}

// GetEnhancedMetrics returns comprehensive metrics including streaming
func (ecs *EnhancedCollectorService) GetEnhancedMetrics() map[string]interface{} {
	baseMetrics := ecs.GetMetrics()
	enhanced := map[string]interface{}{
		"base_metrics": baseMetrics,
		"is_enhanced":  ecs.isEnhanced,
	}

	if ecs.streamAdapter != nil {
		enhanced["streaming_metrics"] = ecs.streamAdapter.GetMetrics()
	}

	if ecs.circuitBreaker != nil {
		ecs.circuitBreaker.mu.RLock()
		enhanced["circuit_breaker"] = map[string]interface{}{
			"state":    ecs.circuitBreaker.state,
			"failures": ecs.circuitBreaker.failures,
		}
		ecs.circuitBreaker.mu.RUnlock()
	}

	if ecs.adaptiveBatcher != nil {
		ecs.adaptiveBatcher.mu.RLock()
		enhanced["adaptive_batcher"] = map[string]interface{}{
			"current_batch_size": ecs.adaptiveBatcher.currentBatchSize,
			"load_average":       ecs.adaptiveBatcher.loadAverage,
		}
		ecs.adaptiveBatcher.mu.RUnlock()
	}

	return enhanced
}
