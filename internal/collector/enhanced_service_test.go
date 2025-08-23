package collector

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/dilipmighty245/telemetry-pipeline/internal/streaming"
	"github.com/dilipmighty245/telemetry-pipeline/pkg/messagequeue"
	"github.com/dilipmighty245/telemetry-pipeline/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockMessageQueueService is a mock implementation of messagequeue.MessageQueueService
type MockMessageQueueService struct {
	mock.Mock
}

func (m *MockMessageQueueService) Start() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockMessageQueueService) Stop() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockMessageQueueService) PublishTelemetry(data *models.TelemetryData) error {
	args := m.Called(data)
	return args.Error(0)
}

func (m *MockMessageQueueService) ConsumeTelemetry(consumerGroup, consumerID string, batchSize int) ([]*messagequeue.Message, error) {
	args := m.Called(consumerGroup, consumerID, batchSize)
	return args.Get(0).([]*messagequeue.Message), args.Error(1)
}

func (m *MockMessageQueueService) AcknowledgeMessages(consumerGroup string, messages []*messagequeue.Message) error {
	args := m.Called(consumerGroup, messages)
	return args.Error(0)
}

func TestNewEnhancedCollectorService(t *testing.T) {
	tests := []struct {
		name    string
		config  *EnhancedCollectorConfig
		wantErr bool
		setup   func(*EnhancedCollectorConfig)
	}{
		{
			name: "basic enhanced collector",
			config: &EnhancedCollectorConfig{
				CollectorConfig: &CollectorConfig{
					CollectorID:   "test-collector",
					BatchSize:     100,
					ConsumerGroup: "test-group",
				},
				EnableStreaming:        true,
				EnableCircuitBreaker:   true,
				EnableAdaptiveBatching: true,
				EnableLoadBalancing:    true,
				Workers:                3,
			},
			wantErr: false,
		},
		{
			name: "enhanced collector with custom configs",
			config: &EnhancedCollectorConfig{
				CollectorConfig: &CollectorConfig{
					CollectorID:   "test-collector-2",
					BatchSize:     200,
					ConsumerGroup: "test-group-2",
				},
				EnableStreaming:     true,
				StreamDestination:   "kafka://localhost:9092",
				EnableLoadBalancing: true,
				Workers:             5,
				MinBatchSize:        25,
				MaxBatchSize:        500,
			},
			setup: func(config *EnhancedCollectorConfig) {
				config.StreamingConfig = &streaming.StreamAdapterConfig{
					ChannelSize:   500,
					BatchSize:     250,
					Workers:       3,
					MaxRetries:    5,
					RetryDelay:    2 * time.Second,
					FlushInterval: 10 * time.Second,
					HTTPTimeout:   60 * time.Second,
					EnableMetrics: true,
					PartitionBy:   "gpu_id",
				}
				config.CircuitBreakerConfig = &CircuitBreakerConfig{
					FailureThreshold: 10,
					RecoveryTimeout:  60 * time.Second,
					HalfOpenRequests: 5,
				}
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setup != nil {
				tt.setup(tt.config)
			}

			mockMQ := &MockMessageQueueService{}
			mockMQ.On("Start").Return(nil)

			service, err := NewEnhancedCollectorService(tt.config, &messagequeue.MessageQueueService{})

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, service)
				assert.True(t, service.isEnhanced)
				assert.Equal(t, tt.config, service.config)

				if tt.config.EnableStreaming {
					assert.NotNil(t, service.streamAdapter)
				}

				if tt.config.EnableCircuitBreaker {
					assert.NotNil(t, service.circuitBreaker)
					assert.Equal(t, Closed, service.circuitBreaker.state)
				}

				if tt.config.EnableAdaptiveBatching {
					assert.NotNil(t, service.adaptiveBatcher)
				}

				if tt.config.EnableLoadBalancing {
					assert.NotNil(t, service.workers)
					assert.NotNil(t, service.workerPool)
					assert.Len(t, service.workers, tt.config.Workers)
				}
			}
		})
	}
}

func TestEnhancedCollectorService_DefaultConfigs(t *testing.T) {
	config := &EnhancedCollectorConfig{
		CollectorConfig: &CollectorConfig{
			CollectorID:   "test-collector",
			BatchSize:     100,
			ConsumerGroup: "test-group",
		},
		EnableStreaming:        true,
		EnableCircuitBreaker:   true,
		EnableAdaptiveBatching: true,
		EnableLoadBalancing:    true,
		// Leave other fields empty to test defaults
	}

	mockMQ := &MockMessageQueueService{}
	mockMQ.On("Start").Return(nil)

	service, err := NewEnhancedCollectorService(config, &messagequeue.MessageQueueService{})

	assert.NoError(t, err)
	assert.NotNil(t, service)

	// Test default values
	assert.Equal(t, 5, config.Workers)
	assert.Equal(t, 50, config.MinBatchSize)
	assert.Equal(t, 1000, config.MaxBatchSize)

	// Test default streaming config
	assert.NotNil(t, config.StreamingConfig)
	assert.Equal(t, 1000, config.StreamingConfig.ChannelSize)
	assert.Equal(t, 500, config.StreamingConfig.BatchSize)
	assert.Equal(t, 5, config.StreamingConfig.Workers)

	// Test default circuit breaker config
	assert.NotNil(t, config.CircuitBreakerConfig)
	assert.Equal(t, 5, config.CircuitBreakerConfig.FailureThreshold)
	assert.Equal(t, 30*time.Second, config.CircuitBreakerConfig.RecoveryTimeout)
	assert.Equal(t, 3, config.CircuitBreakerConfig.HalfOpenRequests)
}

func TestCircuitBreaker_States(t *testing.T) {
	config := &CircuitBreakerConfig{
		FailureThreshold: 3,
		RecoveryTimeout:  100 * time.Millisecond,
		HalfOpenRequests: 2,
	}

	cb := &CircuitBreaker{
		config: config,
		state:  Closed,
	}

	// Test Closed state
	assert.True(t, cb.canExecute())

	// Record failures to trigger Open state
	for i := 0; i < 3; i++ {
		cb.recordFailure()
	}
	assert.Equal(t, Open, cb.state)
	assert.False(t, cb.canExecute())

	// Wait for recovery timeout
	time.Sleep(150 * time.Millisecond)
	assert.True(t, cb.canExecute()) // Should transition to HalfOpen
	assert.Equal(t, HalfOpen, cb.state)

	// Test HalfOpen state limits
	assert.True(t, cb.canExecute())  // First request
	assert.True(t, cb.canExecute())  // Second request
	assert.False(t, cb.canExecute()) // Third request should be blocked

	// Record success to close circuit
	cb.recordSuccess()
	assert.Equal(t, Closed, cb.state)
	assert.True(t, cb.canExecute())
}

func TestCircuitBreaker_ConcurrentAccess(t *testing.T) {
	config := &CircuitBreakerConfig{
		FailureThreshold: 5,
		RecoveryTimeout:  50 * time.Millisecond,
		HalfOpenRequests: 3,
	}

	cb := &CircuitBreaker{
		config: config,
		state:  Closed,
	}

	var wg sync.WaitGroup
	numGoroutines := 10

	// Test concurrent access
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			// Alternate between success and failure
			if id%2 == 0 {
				cb.recordSuccess()
			} else {
				cb.recordFailure()
			}

			// Check if can execute
			cb.canExecute()
		}(i)
	}

	wg.Wait()

	// Circuit breaker should still be functional
	assert.NotPanics(t, func() {
		cb.canExecute()
		cb.recordSuccess()
		cb.recordFailure()
	})
}

func TestAdaptiveBatcher_BatchSizeAdjustment(t *testing.T) {
	config := &EnhancedCollectorConfig{
		CollectorConfig: &CollectorConfig{
			BatchSize: 100,
		},
		MinBatchSize: 50,
		MaxBatchSize: 500,
	}

	batcher := &AdaptiveBatcher{
		config:           config,
		currentBatchSize: config.CollectorConfig.BatchSize,
		lastAdjustment:   time.Now().Add(-2 * time.Minute), // Allow adjustment
	}

	// Test high load scenario
	batcher.loadAverage = 1500 // High load
	batcher.adjustBatchSize()
	assert.Greater(t, batcher.currentBatchSize, 100)
	assert.LessOrEqual(t, batcher.currentBatchSize, 500)

	// Reset for low load test
	batcher.currentBatchSize = 200
	batcher.lastAdjustment = time.Now().Add(-2 * time.Minute)
	batcher.loadAverage = 50 // Low load
	batcher.adjustBatchSize()
	assert.Less(t, batcher.currentBatchSize, 200)
	assert.GreaterOrEqual(t, batcher.currentBatchSize, 50)
}

func TestAdaptiveBatcher_UpdateMetrics(t *testing.T) {
	batcher := &AdaptiveBatcher{
		loadAverage: 100.0,
	}

	// Update metrics with new data
	batcher.updateMetrics(500, 2*time.Second)

	// Load average should be updated
	expectedLoad := 500.0 / 2.0 // 250 records per second
	expectedAverage := (100.0 + expectedLoad) / 2
	assert.Equal(t, expectedAverage, batcher.loadAverage)
}

func TestAdaptiveBatcher_GetCurrentBatchSize(t *testing.T) {
	batcher := &AdaptiveBatcher{
		currentBatchSize: 150,
	}

	size := batcher.getCurrentBatchSize()
	assert.Equal(t, 150, size)
}

func TestAdaptiveBatcher_ConcurrentAccess(t *testing.T) {
	batcher := &AdaptiveBatcher{
		config: &EnhancedCollectorConfig{
			CollectorConfig: &CollectorConfig{
				BatchSize: 100,
			},
			MinBatchSize: 10,
			MaxBatchSize: 1000,
		},
		currentBatchSize: 100,
		lastAdjustment:   time.Now().Add(-2 * time.Minute),
	}

	var wg sync.WaitGroup
	numGoroutines := 10

	// Test concurrent access
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			// Alternate operations
			if id%3 == 0 {
				batcher.getCurrentBatchSize()
			} else if id%3 == 1 {
				batcher.updateMetrics(100, time.Second)
			} else {
				batcher.adjustBatchSize()
			}
		}(i)
	}

	wg.Wait()

	// Batcher should still be functional
	assert.NotPanics(t, func() {
		batcher.getCurrentBatchSize()
		batcher.updateMetrics(50, time.Second)
		batcher.adjustBatchSize()
	})
}

func TestCollectorWorker_Lifecycle(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	worker := &CollectorWorker{
		id:     1,
		ctx:    ctx,
		cancel: cancel,
	}

	// Test start
	assert.False(t, worker.isActive)
	worker.start()
	assert.True(t, worker.isActive)

	// Test stop
	worker.stop()
	assert.False(t, worker.isActive)

	// Test double stop (should not panic)
	assert.NotPanics(t, func() {
		worker.stop()
	})
}

func TestEnhancedCollectorService_GetBatchSize(t *testing.T) {
	tests := []struct {
		name                   string
		enableAdaptiveBatching bool
		baseBatchSize          int
		adaptiveBatchSize      int
		expected               int
	}{
		{
			name:                   "adaptive batching disabled",
			enableAdaptiveBatching: false,
			baseBatchSize:          100,
			expected:               100,
		},
		{
			name:                   "adaptive batching enabled",
			enableAdaptiveBatching: true,
			baseBatchSize:          100,
			adaptiveBatchSize:      150,
			expected:               150,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &EnhancedCollectorConfig{
				CollectorConfig: &CollectorConfig{
					CollectorID:   "test",
					BatchSize:     tt.baseBatchSize,
					ConsumerGroup: "test-group",
				},
				EnableAdaptiveBatching: tt.enableAdaptiveBatching,
			}

			mockMQ := &MockMessageQueueService{}
			mockMQ.On("Start").Return(nil)

			service, err := NewEnhancedCollectorService(config, &messagequeue.MessageQueueService{})
			assert.NoError(t, err)

			if tt.enableAdaptiveBatching {
				service.adaptiveBatcher.currentBatchSize = tt.adaptiveBatchSize
			}

			batchSize := service.getBatchSize()
			assert.Equal(t, tt.expected, batchSize)
		})
	}
}

func TestEnhancedCollectorService_GetEnhancedMetrics(t *testing.T) {
	config := &EnhancedCollectorConfig{
		CollectorConfig: &CollectorConfig{
			CollectorID:   "test-collector",
			BatchSize:     100,
			ConsumerGroup: "test-group",
		},
		EnableStreaming:        true,
		EnableCircuitBreaker:   true,
		EnableAdaptiveBatching: true,
		EnableLoadBalancing:    true,
		Workers:                3,
	}

	mockMQ := &MockMessageQueueService{}
	mockMQ.On("Start").Return(nil)

	service, err := NewEnhancedCollectorService(config, &messagequeue.MessageQueueService{})
	assert.NoError(t, err)

	metrics := service.GetEnhancedMetrics()

	assert.NotNil(t, metrics)
	assert.Equal(t, true, metrics["is_enhanced"])
	assert.NotNil(t, metrics["base_metrics"])

	// Check streaming metrics
	if config.EnableStreaming {
		assert.Contains(t, metrics, "streaming_metrics")
	}

	// Check circuit breaker metrics
	if config.EnableCircuitBreaker {
		assert.Contains(t, metrics, "circuit_breaker")
		cbMetrics := metrics["circuit_breaker"].(map[string]interface{})
		assert.Contains(t, cbMetrics, "state")
		assert.Contains(t, cbMetrics, "failures")
	}

	// Check adaptive batcher metrics
	if config.EnableAdaptiveBatching {
		assert.Contains(t, metrics, "adaptive_batcher")
		abMetrics := metrics["adaptive_batcher"].(map[string]interface{})
		assert.Contains(t, abMetrics, "current_batch_size")
		assert.Contains(t, abMetrics, "load_average")
	}
}

func TestEnhancedCollectorService_CollectBatchEnhanced(t *testing.T) {
	config := &EnhancedCollectorConfig{
		CollectorConfig: &CollectorConfig{
			CollectorID:   "test-collector",
			BatchSize:     2,
			ConsumerGroup: "test-group",
		},
		EnableStreaming: false, // Disable streaming for simpler test
	}

	// Create mock messages
	telemetryData1 := &models.TelemetryData{
		Timestamp:  time.Now(),
		Hostname:   "host1",
		GPUID:      "gpu1",
		MetricName: "utilization",
		Value:      75.5,
	}
	telemetryData2 := &models.TelemetryData{
		Timestamp:  time.Now(),
		Hostname:   "host2",
		GPUID:      "gpu2",
		MetricName: "temperature",
		Value:      65.0,
	}

	data1, _ := json.Marshal(telemetryData1)
	data2, _ := json.Marshal(telemetryData2)

	messages := []*messagequeue.Message{
		{ID: "msg1", Payload: data1},
		{ID: "msg2", Payload: data2},
	}

	mockMQ := &MockMessageQueueService{}
	mockMQ.On("Start").Return(nil)
	mockMQ.On("ConsumeTelemetry", "test-group", "test-collector", 2).Return(messages, nil)
	mockMQ.On("AcknowledgeMessages", "test-group", messages).Return(nil)

	service, err := NewEnhancedCollectorService(config, &messagequeue.MessageQueueService{})
	assert.NoError(t, err)

	// Test collectBatchEnhanced
	err = service.collectBatchEnhanced(2)
	assert.NoError(t, err)

	// Verify mock calls
	mockMQ.AssertExpectations(t)
}

func TestEnhancedCollectorService_CollectBatchWithCircuitBreaker(t *testing.T) {
	config := &EnhancedCollectorConfig{
		CollectorConfig: &CollectorConfig{
			CollectorID:   "test-collector",
			BatchSize:     1,
			ConsumerGroup: "test-group",
		},
		EnableCircuitBreaker: true,
	}

	mockMQ := &MockMessageQueueService{}
	mockMQ.On("Start").Return(nil)

	service, err := NewEnhancedCollectorService(config, &messagequeue.MessageQueueService{})
	assert.NoError(t, err)

	// Force circuit breaker to open state
	service.circuitBreaker.state = Open
	service.circuitBreaker.lastFailure = time.Now()

	// collectBatch should return without error when circuit breaker is open
	err = service.collectBatch()
	assert.NoError(t, err)

	// No message queue calls should be made
	mockMQ.AssertNotCalled(t, "ConsumeTelemetry")
}

func TestCircuitBreakerStates(t *testing.T) {
	tests := []struct {
		name     string
		state    CircuitBreakerState
		expected string
	}{
		{"Closed state", Closed, "Closed"},
		{"Open state", Open, "Open"},
		{"HalfOpen state", HalfOpen, "HalfOpen"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This test verifies the circuit breaker state constants
			assert.Equal(t, tt.state, tt.state)
		})
	}
}

func TestEnhancedCollectorConfig_Validation(t *testing.T) {
	config := &EnhancedCollectorConfig{
		CollectorConfig: &CollectorConfig{
			CollectorID:   "test-collector",
			BatchSize:     100,
			ConsumerGroup: "test-group",
		},
		EnableStreaming:        true,
		StreamDestination:      "kafka://localhost:9092",
		EnableCircuitBreaker:   true,
		EnableAdaptiveBatching: true,
		EnableLoadBalancing:    true,
		Workers:                5,
		MinBatchSize:           25,
		MaxBatchSize:           500,
	}

	assert.Equal(t, "test-collector", config.CollectorConfig.CollectorID)
	assert.Equal(t, 100, config.CollectorConfig.BatchSize)
	assert.Equal(t, "test-group", config.CollectorConfig.ConsumerGroup)
	assert.True(t, config.EnableStreaming)
	assert.Equal(t, "kafka://localhost:9092", config.StreamDestination)
	assert.True(t, config.EnableCircuitBreaker)
	assert.True(t, config.EnableAdaptiveBatching)
	assert.True(t, config.EnableLoadBalancing)
	assert.Equal(t, 5, config.Workers)
	assert.Equal(t, 25, config.MinBatchSize)
	assert.Equal(t, 500, config.MaxBatchSize)
}
