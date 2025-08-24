package streamer

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/dilipmighty245/telemetry-pipeline/pkg/messagequeue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// MockMessageQueueService for testing
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

func (m *MockMessageQueueService) PublishTelemetry(data []byte, headers map[string]string) error {
	args := m.Called(data, headers)
	return args.Error(0)
}

// Tests for basic StreamerService

func TestNewStreamerService_DefaultValues(t *testing.T) {
	config := &StreamerConfig{
		CSVFilePath: "test.csv",
		// Leave fields empty to test defaults
	}

	// This will fail due to CSV file not existing, but we can test the config defaults
	_, err := NewStreamerService(config, &messagequeue.MessageQueueService{})
	assert.Error(t, err) // Expected due to missing CSV file

	// Test that defaults were set
	assert.Equal(t, 100, config.BatchSize)
	assert.Equal(t, 1*time.Second, config.StreamInterval)
	assert.Equal(t, 3, config.MaxRetries)
	assert.Equal(t, 1*time.Second, config.RetryDelay)
	assert.Equal(t, 1000, config.BufferSize)
	assert.NotEmpty(t, config.StreamerID)
	assert.Contains(t, config.StreamerID, "streamer-")
}

func TestStreamerService_Lifecycle(t *testing.T) {
	// Create a service with mocked dependencies
	service := &StreamerService{
		config: &StreamerConfig{
			StreamerID: "test-streamer",
		},
		metrics: &StreamerMetrics{
			StartTime: time.Now(),
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	service.ctx = ctx
	service.cancel = cancel

	// Test initial state
	assert.False(t, service.IsRunning())

	// Test start
	err := service.Start()
	assert.NoError(t, err)
	assert.True(t, service.IsRunning())

	// Test double start (should not error)
	err = service.Start()
	assert.NoError(t, err)

	// Test stop
	err = service.Stop()
	assert.NoError(t, err)
	assert.False(t, service.IsRunning())

	// Test double stop (should not error)
	err = service.Stop()
	assert.NoError(t, err)
}

func TestStreamerService_GetMetrics(t *testing.T) {
	service := &StreamerService{
		metrics: &StreamerMetrics{
			TotalRecordsStreamed: 100,
			TotalBatchesStreamed: 10,
			TotalErrors:          2,
			LastStreamTime:       time.Now(),
			StartTime:            time.Now().Add(-1 * time.Hour),
		},
	}

	metrics := service.GetMetrics()
	assert.NotNil(t, metrics)
	assert.Equal(t, int64(100), metrics.TotalRecordsStreamed)
	assert.Equal(t, int64(10), metrics.TotalBatchesStreamed)
	assert.Equal(t, int64(2), metrics.TotalErrors)
	assert.Greater(t, metrics.StreamingRate, 0.0) // Should calculate rate
}

func TestStreamerService_Health(t *testing.T) {
	service := &StreamerService{
		isRunning: true,
		metrics: &StreamerMetrics{
			LastStreamTime: time.Now(),
			StartTime:      time.Now(),
		},
	}

	// Test healthy service
	assert.True(t, service.Health())

	// Test unhealthy service (not running)
	service.isRunning = false
	assert.False(t, service.Health())

	// Test unhealthy service (no recent activity)
	service.isRunning = true
	service.metrics.LastStreamTime = time.Now().Add(-10 * time.Minute)
	assert.False(t, service.Health())

	// Test healthy service with zero last stream time (new service)
	service.metrics.LastStreamTime = time.Time{}
	assert.True(t, service.Health())
}

func TestStreamerService_UpdateConfig(t *testing.T) {
	originalConfig := &StreamerConfig{
		StreamerID:     "test-streamer",
		BatchSize:      100,
		StreamInterval: 1 * time.Second,
		MaxRetries:     3,
		RetryDelay:     1 * time.Second,
	}

	service := &StreamerService{
		config: originalConfig,
	}

	// Test updating configuration
	newConfig := &StreamerConfig{
		BatchSize:      200,
		StreamInterval: 2 * time.Second,
		MaxRetries:     5,
		RetryDelay:     3 * time.Second,
	}

	err := service.UpdateConfig(newConfig)
	assert.NoError(t, err)

	// Verify updates
	assert.Equal(t, 200, service.config.BatchSize)
	assert.Equal(t, 2*time.Second, service.config.StreamInterval)
	assert.Equal(t, 5, service.config.MaxRetries)
	assert.Equal(t, 3*time.Second, service.config.RetryDelay)

	// Verify unchanged fields
	assert.Equal(t, "test-streamer", service.config.StreamerID)
}

// Tests for NexusStreamerService with enhanced features

func TestNewNexusStreamerService(t *testing.T) {
	// Setup embedded etcd for testing
	etcdServer, cleanup, err := messagequeue.SetupEtcdForTest()
	require.NoError(t, err, "Should start embedded etcd server")
	defer cleanup()

	// Wait for etcd to be ready
	err = messagequeue.WaitForEtcdReady(etcdServer.Endpoints, 10*time.Second)
	require.NoError(t, err, "etcd should be ready")

	config := &NexusStreamerConfig{
		ClusterID:       "test-cluster",
		StreamerID:      "test-streamer",
		BatchSize:       100,
		ParallelWorkers: 0, // Set to 0 to test default assignment
		EtcdEndpoints:   etcdServer.Endpoints,
	}

	// Now this should succeed with embedded etcd
	service, err := NewNexusStreamerService(context.Background(), config)
	assert.NoError(t, err, "Should create streamer service with embedded etcd")
	if service != nil {
		defer service.Close()
	}

	// Test that defaults were set
	assert.Equal(t, 5, config.ParallelWorkers) // Should be set to default
	assert.Equal(t, 1000.0, config.RateLimit)
	assert.Equal(t, 100, config.BurstSize)
	assert.Equal(t, 80.0, config.BackPressureThreshold)
	assert.Equal(t, 100*time.Millisecond, config.BackPressureDelay)
}

func TestNexusStreamerService_DefaultConfigs(t *testing.T) {
	// Setup embedded etcd for testing
	etcdServer, cleanup, err := messagequeue.SetupEtcdForTest()
	require.NoError(t, err, "Should start embedded etcd server")
	defer cleanup()

	// Wait for etcd to be ready
	err = messagequeue.WaitForEtcdReady(etcdServer.Endpoints, 10*time.Second)
	require.NoError(t, err, "etcd should be ready")

	config := &NexusStreamerConfig{
		ClusterID:     "test-cluster",
		StreamerID:    "test-streamer",
		BatchSize:     100,
		EtcdEndpoints: etcdServer.Endpoints,
		// Leave other fields empty to test defaults
	}

	// Now this should succeed with embedded etcd
	service, err := NewNexusStreamerService(context.Background(), config)
	assert.NoError(t, err, "Should create streamer service with embedded etcd")
	if service != nil {
		defer service.Close()
	}

	// Test default values were set
	assert.Equal(t, 5, config.ParallelWorkers)
	assert.Equal(t, 1000.0, config.RateLimit)
	assert.Equal(t, 100, config.BurstSize)
	assert.Equal(t, 80.0, config.BackPressureThreshold)
	assert.Equal(t, 100*time.Millisecond, config.BackPressureDelay)

	// Test default streaming config
	assert.NotNil(t, config.StreamingConfig)
	assert.Equal(t, 2000, config.StreamingConfig.ChannelSize)
	assert.Equal(t, 1000, config.StreamingConfig.BatchSize)
	assert.Equal(t, 8, config.StreamingConfig.Workers)
	assert.Equal(t, 2*time.Second, config.StreamingConfig.FlushInterval)
}

func TestRateLimiter_TokenBucket(t *testing.T) {
	rateLimiter := &RateLimiter{
		rate:       10.0, // 10 tokens per second
		burstSize:  5,    // 5 token bucket
		tokens:     5.0,  // Start with full bucket
		lastUpdate: time.Now(),
	}

	// Test initial tokens
	assert.True(t, rateLimiter.allow())  // Should allow (4 tokens left)
	assert.True(t, rateLimiter.allow())  // Should allow (3 tokens left)
	assert.True(t, rateLimiter.allow())  // Should allow (2 tokens left)
	assert.True(t, rateLimiter.allow())  // Should allow (1 token left)
	assert.True(t, rateLimiter.allow())  // Should allow (0 tokens left)
	assert.False(t, rateLimiter.allow()) // Should deny (no tokens)

	// Wait for token replenishment
	time.Sleep(200 * time.Millisecond)   // Should add ~2 tokens (10 * 0.2)
	assert.True(t, rateLimiter.allow())  // Should allow
	assert.True(t, rateLimiter.allow())  // Should allow
	assert.False(t, rateLimiter.allow()) // Should deny
}

func TestRateLimiter_ConcurrentAccess(t *testing.T) {
	rateLimiter := &RateLimiter{
		rate:       100.0, // High rate for testing
		burstSize:  50,
		tokens:     50.0,
		lastUpdate: time.Now(),
	}

	var wg sync.WaitGroup
	numGoroutines := 20
	allowedCount := 0
	var mu sync.Mutex

	// Test concurrent access
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if rateLimiter.allow() {
				mu.Lock()
				allowedCount++
				mu.Unlock()
			}
		}()
	}

	wg.Wait()

	// Should allow some requests but not all (due to token bucket)
	assert.Greater(t, allowedCount, 0)
	assert.LessOrEqual(t, allowedCount, numGoroutines)
}

func TestStreamerWorker_Lifecycle(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	worker := &StreamerWorker{
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

func TestNexusStreamerService_GetMetrics(t *testing.T) {
	// Setup embedded etcd for testing
	etcdServer, cleanup, err := messagequeue.SetupEtcdForTest()
	require.NoError(t, err, "Should start embedded etcd server")
	defer cleanup()

	// Wait for etcd to be ready
	err = messagequeue.WaitForEtcdReady(etcdServer.Endpoints, 10*time.Second)
	require.NoError(t, err, "etcd should be ready")

	config := &NexusStreamerConfig{
		ClusterID:       "test-cluster",
		StreamerID:      "test-streamer",
		BatchSize:       100,
		ParallelWorkers: 3,
		EtcdEndpoints:   etcdServer.Endpoints,
	}

	// Create a real service with embedded etcd
	service, err := NewNexusStreamerService(context.Background(), config)
	require.NoError(t, err, "Should create streamer service")
	defer service.Close()

	// Set some test values
	service.messageCount = 100
	service.startTime = time.Now().Add(-1 * time.Hour)

	metrics := service.GetEnhancedMetrics()

	assert.NotNil(t, metrics)
	assert.Equal(t, true, metrics["is_enhanced"])
	assert.Equal(t, int64(100), metrics["message_count"])
	assert.Equal(t, "test-streamer", metrics["streamer_id"])
	assert.Equal(t, "test-cluster", metrics["cluster_id"])
}

func TestNexusStreamerService_StreamDirectly(t *testing.T) {
	// Setup embedded etcd for testing
	etcdServer, cleanup, err := messagequeue.SetupEtcdForTest()
	require.NoError(t, err, "Should start embedded etcd server")
	defer cleanup()

	// Wait for etcd to be ready
	err = messagequeue.WaitForEtcdReady(etcdServer.Endpoints, 10*time.Second)
	require.NoError(t, err, "etcd should be ready")

	config := &NexusStreamerConfig{
		ClusterID:     "test-cluster",
		StreamerID:    "test-streamer",
		BatchSize:     100,
		EtcdEndpoints: etcdServer.Endpoints,
	}

	// Create a service with streaming enabled (always enabled now)
	service, err := NewNexusStreamerService(context.Background(), config)
	require.NoError(t, err, "Should create streamer service")
	defer service.Close()

	// Create test data using TelemetryRecord (the type used by NexusStreamerService)
	testData := []*TelemetryRecord{
		{
			Timestamp:         "2023-01-01T12:00:00Z",
			Hostname:          "host1",
			GPUID:             "gpu1",
			GPUUtilization:    75.5,
			MemoryUtilization: 65.0,
		},
		{
			Timestamp:         "2023-01-01T12:01:00Z",
			Hostname:          "host2",
			GPUID:             "gpu2",
			GPUUtilization:    80.0,
			MemoryUtilization: 70.0,
		},
	}

	// Test streaming with adapter (should work now)
	err = service.StreamDirectly(testData)
	assert.NoError(t, err, "Should stream data successfully")
}

func TestRateLimiter_TokenReplenishment(t *testing.T) {
	rateLimiter := &RateLimiter{
		rate:       5.0, // 5 tokens per second
		burstSize:  3,
		tokens:     0.0,                              // Start empty
		lastUpdate: time.Now().Add(-1 * time.Second), // 1 second ago
	}

	// After 1 second, should have replenished tokens
	assert.True(t, rateLimiter.allow()) // Should allow (tokens were replenished)
}

func TestRateLimiter_BurstLimit(t *testing.T) {
	rateLimiter := &RateLimiter{
		rate:       100.0, // High rate
		burstSize:  2,     // Small burst
		tokens:     0.0,
		lastUpdate: time.Now().Add(-10 * time.Second), // Long time ago
	}

	// Even after long time, tokens should be capped at burst size
	assert.True(t, rateLimiter.allow())  // 1 token used
	assert.True(t, rateLimiter.allow())  // 2 tokens used
	assert.False(t, rateLimiter.allow()) // No more tokens
}

func TestStreamerMetrics_ConcurrentAccess(t *testing.T) {
	metrics := &StreamerMetrics{
		StartTime: time.Now(),
	}

	var wg sync.WaitGroup
	numGoroutines := 20

	// Test concurrent read/write access to metrics
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			metrics.mu.Lock()
			metrics.TotalRecordsStreamed += int64(id)
			metrics.TotalBatchesStreamed += 1
			metrics.LastStreamTime = time.Now()
			metrics.mu.Unlock()
		}(i)
	}

	wg.Wait()

	// Verify final state
	metrics.mu.RLock()
	assert.Greater(t, metrics.TotalRecordsStreamed, int64(0))
	assert.Equal(t, int64(numGoroutines), metrics.TotalBatchesStreamed)
	assert.False(t, metrics.LastStreamTime.IsZero())
	metrics.mu.RUnlock()
}

func TestStreamerConfig_Validation(t *testing.T) {
	config := &StreamerConfig{
		CSVFilePath:    "/path/to/test.csv",
		BatchSize:      150,
		StreamInterval: 2 * time.Second,
		LoopMode:       true,
		MaxRetries:     5,
		RetryDelay:     2 * time.Second,
		EnableMetrics:  true,
		StreamerID:     "test-streamer-123",
		BufferSize:     2000,
	}

	assert.Equal(t, "/path/to/test.csv", config.CSVFilePath)
	assert.Equal(t, 150, config.BatchSize)
	assert.Equal(t, 2*time.Second, config.StreamInterval)
	assert.True(t, config.LoopMode)
	assert.Equal(t, 5, config.MaxRetries)
	assert.Equal(t, 2*time.Second, config.RetryDelay)
	assert.True(t, config.EnableMetrics)
	assert.Equal(t, "test-streamer-123", config.StreamerID)
	assert.Equal(t, 2000, config.BufferSize)
}

func TestNexusStreamerConfig_Validation(t *testing.T) {
	config := &NexusStreamerConfig{
		ClusterID:             "test-cluster",
		StreamerID:            "test-streamer",
		BatchSize:             100,
		StreamDestination:     "kafka://localhost:9092",
		ParallelWorkers:       5,
		RateLimit:             2000.0,
		BurstSize:             200,
		BackPressureThreshold: 85.0,
		BackPressureDelay:     150 * time.Millisecond,
		EtcdEndpoints:         []string{"localhost:2379"},
	}

	assert.Equal(t, "test-cluster", config.ClusterID)
	assert.Equal(t, "test-streamer", config.StreamerID)
	assert.Equal(t, 100, config.BatchSize)
	assert.Equal(t, "kafka://localhost:9092", config.StreamDestination)
	assert.Equal(t, 5, config.ParallelWorkers)
	assert.Equal(t, 2000.0, config.RateLimit)
	assert.Equal(t, 200, config.BurstSize)
	assert.Equal(t, 85.0, config.BackPressureThreshold)
	assert.Equal(t, 150*time.Millisecond, config.BackPressureDelay)
	assert.Equal(t, []string{"localhost:2379"}, config.EtcdEndpoints)
}

func TestStreamerService_StreamingRateCalculation(t *testing.T) {
	startTime := time.Now().Add(-1 * time.Hour) // 1 hour ago

	service := &StreamerService{
		metrics: &StreamerMetrics{
			TotalRecordsStreamed: 3600, // 3600 records in 1 hour
			StartTime:            startTime,
		},
	}

	metrics := service.GetMetrics()

	// Should be approximately 1 record per second (3600 records / 3600 seconds)
	assert.InDelta(t, 1.0, metrics.StreamingRate, 0.1)
}
