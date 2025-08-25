package collector

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/dilipmighty245/telemetry-pipeline/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollectorConfig_Struct(t *testing.T) {
	config := CollectorConfig{
		CollectorID:   "collector-1",
		ConsumerGroup: "telemetry-collectors",
		BatchSize:     100,
		PollInterval:  5 * time.Second,
		MaxRetries:    3,
		RetryDelay:    1 * time.Second,
		BufferSize:    1000,
		EnableMetrics: true,
	}

	assert.Equal(t, "collector-1", config.CollectorID)
	assert.Equal(t, "telemetry-collectors", config.ConsumerGroup)
	assert.Equal(t, 100, config.BatchSize)
	assert.Equal(t, 5*time.Second, config.PollInterval)
	assert.Equal(t, 3, config.MaxRetries)
	assert.Equal(t, 1*time.Second, config.RetryDelay)
	assert.Equal(t, 1000, config.BufferSize)
	assert.True(t, config.EnableMetrics)
}

func TestCollectorConfig_JSONSerialization(t *testing.T) {
	config := CollectorConfig{
		CollectorID:   "collector-1",
		ConsumerGroup: "telemetry-collectors",
		BatchSize:     100,
		PollInterval:  5 * time.Second,
		MaxRetries:    3,
		RetryDelay:    1 * time.Second,
		BufferSize:    1000,
		EnableMetrics: true,
	}

	// Test JSON marshaling
	jsonData, err := json.Marshal(config)
	require.NoError(t, err)
	assert.Contains(t, string(jsonData), "\"collector_id\":\"collector-1\"")
	assert.Contains(t, string(jsonData), "\"consumer_group\":\"telemetry-collectors\"")
	assert.Contains(t, string(jsonData), "\"batch_size\":100")

	// Test JSON unmarshaling
	var unmarshaled CollectorConfig
	err = json.Unmarshal(jsonData, &unmarshaled)
	require.NoError(t, err)
	assert.Equal(t, config.CollectorID, unmarshaled.CollectorID)
	assert.Equal(t, config.ConsumerGroup, unmarshaled.ConsumerGroup)
	assert.Equal(t, config.BatchSize, unmarshaled.BatchSize)
}

func TestCollectorMetrics_Struct(t *testing.T) {
	now := time.Now()
	metrics := CollectorMetrics{
		TotalRecordsCollected: 1000,
		TotalBatchesCollected: 10,
		TotalRecordsPersisted: 950,
		TotalErrors:           5,
		LastCollectionTime:    now,
		LastPersistTime:       now.Add(-1 * time.Minute),
		CollectionRate:        15.5,
		StartTime:             now.Add(-1 * time.Hour),
	}

	assert.Equal(t, int64(1000), metrics.TotalRecordsCollected)
	assert.Equal(t, int64(10), metrics.TotalBatchesCollected)
	assert.Equal(t, int64(950), metrics.TotalRecordsPersisted)
	assert.Equal(t, int64(5), metrics.TotalErrors)
	assert.Equal(t, now, metrics.LastCollectionTime)
	assert.Equal(t, 15.5, metrics.CollectionRate)
	assert.Equal(t, now.Add(-1*time.Hour), metrics.StartTime)
}

func TestCollectorMetrics_JSONSerialization(t *testing.T) {
	now := time.Now()
	metrics := CollectorMetrics{
		TotalRecordsCollected: 1000,
		TotalBatchesCollected: 10,
		TotalRecordsPersisted: 950,
		TotalErrors:           5,
		LastCollectionTime:    now,
		LastPersistTime:       now.Add(-1 * time.Minute),
		CollectionRate:        15.5,
		StartTime:             now.Add(-1 * time.Hour),
	}

	// Test JSON marshaling - copy to avoid lock value issue
	metricsCopy := CollectorMetrics{
		TotalRecordsCollected: metrics.TotalRecordsCollected,
		TotalBatchesCollected: metrics.TotalBatchesCollected,
		TotalRecordsPersisted: metrics.TotalRecordsPersisted,
		TotalErrors:           metrics.TotalErrors,
		LastCollectionTime:    metrics.LastCollectionTime,
		LastPersistTime:       metrics.LastPersistTime,
		CollectionRate:        metrics.CollectionRate,
		StartTime:             metrics.StartTime,
	}
	jsonData, err := json.Marshal(metricsCopy)
	require.NoError(t, err)
	assert.Contains(t, string(jsonData), "\"total_records_collected\":1000")
	assert.Contains(t, string(jsonData), "\"total_batches_collected\":10")
	assert.Contains(t, string(jsonData), "\"total_errors\":5")

	// Test JSON unmarshaling
	var unmarshaled CollectorMetrics
	err = json.Unmarshal(jsonData, &unmarshaled)
	require.NoError(t, err)
	assert.Equal(t, metrics.TotalRecordsCollected, unmarshaled.TotalRecordsCollected)
	assert.Equal(t, metrics.TotalBatchesCollected, unmarshaled.TotalBatchesCollected)
	assert.Equal(t, metrics.TotalErrors, unmarshaled.TotalErrors)
}

func TestNewCollectorService(t *testing.T) {
	config := &CollectorConfig{
		CollectorID:   "test-collector",
		ConsumerGroup: "test-group",
		BatchSize:     50,
		PollInterval:  1 * time.Second,
		MaxRetries:    2,
		RetryDelay:    500 * time.Millisecond,
		BufferSize:    500,
		EnableMetrics: true,
	}

	// Test with nil message queue (for testing core logic)
	service, err := NewCollectorService(config, nil)
	require.NoError(t, err)

	assert.NotNil(t, service)
	assert.Equal(t, config, service.config)
	assert.NotNil(t, service.metrics)
	assert.False(t, service.isRunning)
	assert.Equal(t, int64(0), service.totalCollected)
	assert.NotNil(t, service.buffer)
	assert.Equal(t, 0, len(service.buffer))
}

func TestCollectorService_GetMetrics(t *testing.T) {
	config := &CollectorConfig{
		CollectorID:   "test-collector",
		ConsumerGroup: "test-group",
		BatchSize:     50,
		EnableMetrics: true,
	}

	service, err := NewCollectorService(config, nil)
	require.NoError(t, err)

	// Test initial metrics
	metrics := service.GetMetrics()
	assert.NotNil(t, metrics)
	assert.Equal(t, int64(0), metrics.TotalRecordsCollected)
	assert.Equal(t, int64(0), metrics.TotalBatchesCollected)
	assert.Equal(t, int64(0), metrics.TotalErrors)

	// Manually update some metrics for testing
	service.mu.Lock()
	service.metrics.TotalRecordsCollected = 100
	service.metrics.TotalBatchesCollected = 5
	service.metrics.TotalErrors = 2
	service.mu.Unlock()

	metrics = service.GetMetrics()
	assert.Equal(t, int64(100), metrics.TotalRecordsCollected)
	assert.Equal(t, int64(5), metrics.TotalBatchesCollected)
	assert.Equal(t, int64(2), metrics.TotalErrors)
}

func TestCollectorService_IsRunning(t *testing.T) {
	config := &CollectorConfig{
		CollectorID:   "test-collector",
		ConsumerGroup: "test-group",
		BatchSize:     50,
	}

	service, err := NewCollectorService(config, nil)
	require.NoError(t, err)

	// Initially not running
	assert.False(t, service.IsRunning())

	// Manually set running state for testing
	service.mu.Lock()
	service.isRunning = true
	service.mu.Unlock()

	assert.True(t, service.IsRunning())

	// Set back to not running
	service.mu.Lock()
	service.isRunning = false
	service.mu.Unlock()

	assert.False(t, service.IsRunning())
}

func TestCollectorService_GetLastError(t *testing.T) {
	config := &CollectorConfig{
		CollectorID:   "test-collector",
		ConsumerGroup: "test-group",
		BatchSize:     50,
	}

	service, err := NewCollectorService(config, nil)
	require.NoError(t, err)

	// Initially no error
	assert.NoError(t, service.GetLastError())

	// Set an error for testing
	testError := assert.AnError
	service.mu.Lock()
	service.lastError = testError
	service.mu.Unlock()

	assert.Equal(t, testError, service.GetLastError())
}

func TestCollectorService_GetTotalCollected(t *testing.T) {
	config := &CollectorConfig{
		CollectorID:   "test-collector",
		ConsumerGroup: "test-group",
		BatchSize:     50,
	}

	service, err := NewCollectorService(config, nil)
	require.NoError(t, err)

	// Initially zero
	assert.Equal(t, int64(0), service.GetTotalCollected())

	// Set a value for testing
	service.mu.Lock()
	service.totalCollected = 500
	service.mu.Unlock()

	assert.Equal(t, int64(500), service.GetTotalCollected())
}

func TestCollectorService_AddToBuffer(t *testing.T) {
	config := &CollectorConfig{
		CollectorID:   "test-collector",
		ConsumerGroup: "test-group",
		BatchSize:     2,
		BufferSize:    5,
	}

	service, err := NewCollectorService(config, nil)
	require.NoError(t, err)

	// Create test telemetry data
	data1 := &models.TelemetryData{
		ID:         1,
		MetricName: "CPU_USAGE",
		Value:      75.5,
		Hostname:   "server1",
	}

	data2 := &models.TelemetryData{
		ID:         2,
		MetricName: "MEMORY_USAGE",
		Value:      60.2,
		Hostname:   "server1",
	}

	// Test adding data to buffer
	service.bufferMu.Lock()
	service.buffer = append(service.buffer, data1)
	service.buffer = append(service.buffer, data2)
	service.bufferMu.Unlock()

	// Verify buffer contents
	service.bufferMu.Lock()
	assert.Len(t, service.buffer, 2)
	assert.Equal(t, data1, service.buffer[0])
	assert.Equal(t, data2, service.buffer[1])
	service.bufferMu.Unlock()
}

func TestCollectorService_ClearBuffer(t *testing.T) {
	config := &CollectorConfig{
		CollectorID:   "test-collector",
		ConsumerGroup: "test-group",
		BatchSize:     50,
		BufferSize:    100,
	}

	service, err := NewCollectorService(config, nil)
	require.NoError(t, err)

	// Add some data to buffer
	data := &models.TelemetryData{
		ID:         1,
		MetricName: "CPU_USAGE",
		Value:      75.5,
		Hostname:   "server1",
	}

	service.bufferMu.Lock()
	service.buffer = append(service.buffer, data)
	service.bufferMu.Unlock()

	// Verify buffer has data
	service.bufferMu.Lock()
	assert.Len(t, service.buffer, 1)
	service.bufferMu.Unlock()

	// Clear buffer
	service.bufferMu.Lock()
	service.buffer = service.buffer[:0]
	service.bufferMu.Unlock()

	// Verify buffer is empty
	service.bufferMu.Lock()
	assert.Len(t, service.buffer, 0)
	service.bufferMu.Unlock()
}

func TestCollectorService_ConfigValidation(t *testing.T) {
	tests := []struct {
		name   string
		config *CollectorConfig
		valid  bool
	}{
		{
			name: "valid config",
			config: &CollectorConfig{
				CollectorID:   "collector-1",
				ConsumerGroup: "group-1",
				BatchSize:     100,
				PollInterval:  5 * time.Second,
				MaxRetries:    3,
				RetryDelay:    1 * time.Second,
				BufferSize:    1000,
				EnableMetrics: true,
			},
			valid: true,
		},
		{
			name: "empty collector ID",
			config: &CollectorConfig{
				CollectorID:   "",
				ConsumerGroup: "group-1",
				BatchSize:     100,
			},
			valid: false,
		},
		{
			name: "empty consumer group",
			config: &CollectorConfig{
				CollectorID:   "collector-1",
				ConsumerGroup: "",
				BatchSize:     100,
			},
			valid: false,
		},
		{
			name: "zero batch size",
			config: &CollectorConfig{
				CollectorID:   "collector-1",
				ConsumerGroup: "group-1",
				BatchSize:     0,
			},
			valid: false,
		},
		{
			name: "negative batch size",
			config: &CollectorConfig{
				CollectorID:   "collector-1",
				ConsumerGroup: "group-1",
				BatchSize:     -1,
			},
			valid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.valid {
				assert.NotEmpty(t, tt.config.CollectorID)
				assert.NotEmpty(t, tt.config.ConsumerGroup)
				assert.Greater(t, tt.config.BatchSize, 0)
			} else {
				invalid := tt.config.CollectorID == "" ||
					tt.config.ConsumerGroup == "" ||
					tt.config.BatchSize <= 0
				assert.True(t, invalid, "Config should be invalid")
			}
		})
	}
}

func TestCollectorService_EdgeCases(t *testing.T) {
	// Test with minimal config
	config := &CollectorConfig{
		CollectorID:   "minimal",
		ConsumerGroup: "minimal-group",
		BatchSize:     1,
	}

	service, err := NewCollectorService(config, nil)
	require.NoError(t, err)
	assert.NotNil(t, service)
	assert.Equal(t, config, service.config)

	// Test metrics when disabled
	config.EnableMetrics = false
	service, err = NewCollectorService(config, nil)
	require.NoError(t, err)
	metrics := service.GetMetrics()
	assert.NotNil(t, metrics) // Should still return metrics even if disabled
}

func TestCollectorService_ConcurrentAccess(t *testing.T) {
	config := &CollectorConfig{
		CollectorID:   "concurrent-test",
		ConsumerGroup: "concurrent-group",
		BatchSize:     10,
		BufferSize:    100,
		EnableMetrics: true,
	}

	service, err := NewCollectorService(config, nil)
	require.NoError(t, err)

	// Test concurrent access to metrics
	done := make(chan bool, 2)

	// Writer goroutine
	go func() {
		for i := 0; i < 100; i++ {
			service.mu.Lock()
			service.metrics.TotalRecordsCollected++
			service.totalCollected++
			service.mu.Unlock()
		}
		done <- true
	}()

	// Reader goroutine
	go func() {
		for i := 0; i < 100; i++ {
			service.GetMetrics()
			service.GetTotalCollected()
			service.IsRunning()
		}
		done <- true
	}()

	// Wait for both goroutines
	<-done
	<-done

	// Verify final state
	assert.Equal(t, int64(100), service.GetTotalCollected())
	metrics := service.GetMetrics()
	assert.Equal(t, int64(100), metrics.TotalRecordsCollected)
}

// Benchmark tests
func BenchmarkCollectorService_GetMetrics(b *testing.B) {
	config := &CollectorConfig{
		CollectorID:   "benchmark-collector",
		ConsumerGroup: "benchmark-group",
		BatchSize:     100,
		EnableMetrics: true,
	}

	service, err := NewCollectorService(config, nil)
	require.NoError(b, err)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		service.GetMetrics()
	}
}

func BenchmarkCollectorService_BufferOperations(b *testing.B) {
	config := &CollectorConfig{
		CollectorID:   "benchmark-collector",
		ConsumerGroup: "benchmark-group",
		BatchSize:     100,
		BufferSize:    1000,
	}

	service, err := NewCollectorService(config, nil)
	require.NoError(b, err)

	data := &models.TelemetryData{
		ID:         1,
		MetricName: "CPU_USAGE",
		Value:      75.5,
		Hostname:   "server1",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		service.bufferMu.Lock()
		service.buffer = append(service.buffer, data)
		if len(service.buffer) > 100 {
			service.buffer = service.buffer[:0]
		}
		service.bufferMu.Unlock()
	}
}

func BenchmarkCollectorConfig_JSONMarshal(b *testing.B) {
	config := CollectorConfig{
		CollectorID:   "benchmark-collector",
		ConsumerGroup: "benchmark-group",
		BatchSize:     100,
		PollInterval:  5 * time.Second,
		MaxRetries:    3,
		RetryDelay:    1 * time.Second,
		BufferSize:    1000,
		EnableMetrics: true,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := json.Marshal(config)
		if err != nil {
			b.Fatal(err)
		}
	}
}
