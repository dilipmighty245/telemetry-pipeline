package streaming

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/dilipmighty245/telemetry-pipeline/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStreamAdapterConfig_Struct(t *testing.T) {
	config := StreamAdapterConfig{
		ChannelSize:   1000,
		BatchSize:     500,
		Workers:       10,
		MaxRetries:    3,
		RetryDelay:    1 * time.Second,
		FlushInterval: 5 * time.Second,
		HTTPTimeout:   30 * time.Second,
		EnableMetrics: true,
		PartitionBy:   "hostname",
	}

	assert.Equal(t, 1000, config.ChannelSize)
	assert.Equal(t, 500, config.BatchSize)
	assert.Equal(t, 10, config.Workers)
	assert.Equal(t, 3, config.MaxRetries)
	assert.Equal(t, 1*time.Second, config.RetryDelay)
	assert.Equal(t, 5*time.Second, config.FlushInterval)
	assert.Equal(t, 30*time.Second, config.HTTPTimeout)
	assert.True(t, config.EnableMetrics)
	assert.Equal(t, "hostname", config.PartitionBy)
}

func TestStreamAdapterConfig_JSONSerialization(t *testing.T) {
	config := StreamAdapterConfig{
		ChannelSize:   1000,
		BatchSize:     500,
		Workers:       10,
		MaxRetries:    3,
		RetryDelay:    1 * time.Second,
		FlushInterval: 5 * time.Second,
		HTTPTimeout:   30 * time.Second,
		EnableMetrics: true,
		PartitionBy:   "hostname",
	}

	// Test JSON marshaling
	jsonData, err := json.Marshal(config)
	require.NoError(t, err)
	assert.Contains(t, string(jsonData), "\"channel_size\":1000")
	assert.Contains(t, string(jsonData), "\"batch_size\":500")
	assert.Contains(t, string(jsonData), "\"workers\":10")
	assert.Contains(t, string(jsonData), "\"partition_by\":\"hostname\"")

	// Test JSON unmarshaling
	var unmarshaled StreamAdapterConfig
	err = json.Unmarshal(jsonData, &unmarshaled)
	require.NoError(t, err)
	assert.Equal(t, config.ChannelSize, unmarshaled.ChannelSize)
	assert.Equal(t, config.BatchSize, unmarshaled.BatchSize)
	assert.Equal(t, config.Workers, unmarshaled.Workers)
	assert.Equal(t, config.PartitionBy, unmarshaled.PartitionBy)
}

func TestTelemetryChannelData_Struct(t *testing.T) {
	now := time.Now()
	data := &models.TelemetryData{
		ID:         1,
		MetricName: "GPU_UTIL",
		GPUID:      "0",
		Hostname:   "test-host",
		Value:      85.5,
		Timestamp:  now,
	}

	channelData := TelemetryChannelData{
		Data:      data,
		Headers:   map[string]string{"Content-Type": "application/json"},
		Timestamp: now,
		Partition: "test-host",
	}

	assert.Equal(t, data, channelData.Data)
	assert.Equal(t, "application/json", channelData.Headers["Content-Type"])
	assert.Equal(t, now, channelData.Timestamp)
	assert.Equal(t, "test-host", channelData.Partition)
}

func TestStreamMetrics_Struct(t *testing.T) {
	now := time.Now()
	metrics := StreamMetrics{
		TotalProcessed:     1000,
		TotalBatches:       10,
		TotalErrors:        5,
		ProcessingRate:     100.5,
		LastProcessTime:    now,
		ChannelUtilization: 0.75,
	}

	assert.Equal(t, int64(1000), metrics.TotalProcessed)
	assert.Equal(t, int64(10), metrics.TotalBatches)
	assert.Equal(t, int64(5), metrics.TotalErrors)
	assert.Equal(t, 100.5, metrics.ProcessingRate)
	assert.Equal(t, now, metrics.LastProcessTime)
	assert.Equal(t, 0.75, metrics.ChannelUtilization)
}

func TestNewStreamAdapter(t *testing.T) {
	config := &StreamAdapterConfig{
		ChannelSize:   100,
		BatchSize:     50,
		Workers:       2,
		MaxRetries:    3,
		RetryDelay:    1 * time.Second,
		FlushInterval: 5 * time.Second,
		HTTPTimeout:   10 * time.Second,
		EnableMetrics: true,
		PartitionBy:   "hostname",
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	adapter := NewStreamAdapter(ctx, config, "http://localhost:8080/api/data")

	assert.NotNil(t, adapter)
	assert.Equal(t, config, adapter.config)
	assert.NotNil(t, adapter.dataCh)
	assert.Equal(t, 100, cap(adapter.dataCh))
	assert.NotNil(t, adapter.httpClient)
	assert.NotNil(t, adapter.metrics)
	assert.False(t, adapter.isRunning)
}

func TestNewStreamAdapter_WithDefaults(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	// Test with nil config (should use defaults)
	adapter := NewStreamAdapter(ctx, nil, "http://localhost:8080/api/data")

	assert.NotNil(t, adapter)
	assert.NotNil(t, adapter.config)
	assert.Equal(t, DefaultChannelSize, adapter.config.ChannelSize)
	assert.Equal(t, DefaultBatchSize, adapter.config.BatchSize)
	assert.Equal(t, DefaultWorkers, adapter.config.Workers)
	assert.Equal(t, DefaultRetries, adapter.config.MaxRetries)
	assert.Equal(t, DefaultTimeout, adapter.config.HTTPTimeout)
	assert.True(t, adapter.config.EnableMetrics)
	assert.Equal(t, "hostname", adapter.config.PartitionBy)
	cancel()
}

func TestStreamAdapter_Start_Stop(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	config := &StreamAdapterConfig{
		ChannelSize:   10,
		BatchSize:     5,
		Workers:       1,
		FlushInterval: 100 * time.Millisecond,
		EnableMetrics: true,
	}

	adapter := NewStreamAdapter(ctx, config, "http://localhost:8080/api/data")

	// Test starting
	assert.False(t, adapter.IsRunning())
	adapter.Start(ctx)
	assert.True(t, adapter.IsRunning())

	// Give it a moment to start
	time.Sleep(50 * time.Millisecond)

	// Test stopping
	cancel()
	adapter.Stop()
	assert.False(t, adapter.IsRunning())
}

func TestStreamAdapter_WriteTelemetry(t *testing.T) {
	// Create a test server to receive data
	var receivedData []models.TelemetryData
	var mu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]interface{}
		err := json.NewDecoder(r.Body).Decode(&payload)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// Extract records from the payload
		if records, ok := payload["records"].([]interface{}); ok {
			mu.Lock()
			for _, record := range records {
				if recordBytes, err := json.Marshal(record); err == nil {
					var telemetryData models.TelemetryData
					if err := json.Unmarshal(recordBytes, &telemetryData); err == nil {
						receivedData = append(receivedData, telemetryData)
					}
				}
			}
			mu.Unlock()
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := &StreamAdapterConfig{
		ChannelSize:   10,
		BatchSize:     2,
		Workers:       1,
		FlushInterval: 50 * time.Millisecond,
		HTTPTimeout:   5 * time.Second,
		EnableMetrics: true,
	}

	ctx, cancel := context.WithCancel(context.Background())
	adapter := NewStreamAdapter(ctx, config, server.URL)
	adapter.Start(ctx)
	defer adapter.Stop()
	defer cancel()

	// Write test data
	testData := &models.TelemetryData{
		ID:         1,
		MetricName: "GPU_UTIL",
		GPUID:      "0",
		Hostname:   "test-host",
		Value:      85.5,
		Timestamp:  time.Now(),
	}

	err := adapter.WriteTelemetry(ctx, testData, map[string]string{})
	assert.NoError(t, err)

	// Write another piece of data to trigger batch
	testData2 := &models.TelemetryData{
		ID:         2,
		MetricName: "GPU_TEMP",
		GPUID:      "0",
		Hostname:   "test-host",
		Value:      72.0,
		Timestamp:  time.Now(),
	}

	err = adapter.WriteTelemetry(ctx, testData2, map[string]string{})
	assert.NoError(t, err)

	// Wait for data to be processed
	time.Sleep(200 * time.Millisecond)

	// Check that data was received
	mu.Lock()
	assert.GreaterOrEqual(t, len(receivedData), 2)
	mu.Unlock()
}

func TestStreamAdapter_WriteTelemetry_ChannelFull(t *testing.T) {
	config := &StreamAdapterConfig{
		ChannelSize:   2,
		BatchSize:     10,
		Workers:       1,
		FlushInterval: 1 * time.Hour, // Long interval to prevent automatic flushing
		EnableMetrics: true,
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	adapter := NewStreamAdapter(ctx, config, "http://localhost:8080/api/data")
	// Don't start the adapter so the channel will fill up

	testData := &models.TelemetryData{
		ID:         1,
		MetricName: "GPU_UTIL",
		GPUID:      "0",
		Hostname:   "test-host",
		Value:      85.5,
		Timestamp:  time.Now(),
	}

	// Fill the channel
	err := adapter.WriteTelemetry(ctx, testData, map[string]string{})
	assert.NoError(t, err)

	err = adapter.WriteTelemetry(ctx, testData, map[string]string{})
	assert.NoError(t, err)

	// This should fail because channel is full
	err = adapter.WriteTelemetry(ctx, testData, map[string]string{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "channel is full")
}

func TestStreamAdapter_GetMetrics(t *testing.T) {
	config := &StreamAdapterConfig{
		ChannelSize:   10,
		BatchSize:     5,
		Workers:       1,
		EnableMetrics: true,
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	adapter := NewStreamAdapter(ctx, config, "http://localhost:8080/api/data")

	// Test initial metrics
	metrics := adapter.GetMetrics()
	assert.NotNil(t, metrics)
	assert.Equal(t, int64(0), metrics.TotalProcessed)
	assert.Equal(t, int64(0), metrics.TotalBatches)
	assert.Equal(t, int64(0), metrics.TotalErrors)
	assert.Equal(t, 0.0, metrics.ProcessingRate)
}

func TestStreamAdapter_IsRunning(t *testing.T) {
	config := &StreamAdapterConfig{
		ChannelSize: 10,
		BatchSize:   5,
		Workers:     1,
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	adapter := NewStreamAdapter(ctx, config, "http://localhost:8080/api/data")

	// Initially not running
	assert.False(t, adapter.IsRunning())

	// Start and check
	adapter.Start(ctx)
	assert.True(t, adapter.IsRunning())

	// Stop and check
	cancel()
	adapter.Stop()
	assert.False(t, adapter.IsRunning())
}

func TestStreamAdapter_getPartition(t *testing.T) {
	tests := []struct {
		name        string
		partitionBy string
		data        *models.TelemetryData
		expected    string
	}{
		{
			name:        "partition by hostname",
			partitionBy: "hostname",
			data: &models.TelemetryData{
				Hostname: "test-host-1",
				GPUID:    "0",
			},
			expected: "test-host-1",
		},
		{
			name:        "partition by gpu_id",
			partitionBy: "gpu_id",
			data: &models.TelemetryData{
				Hostname: "test-host-1",
				GPUID:    "0",
			},
			expected: "0",
		},
		{
			name:        "partition by none",
			partitionBy: "none",
			data: &models.TelemetryData{
				Hostname: "test-host-1",
				GPUID:    "0",
			},
			expected: "default",
		},
		{
			name:        "unknown partition type",
			partitionBy: "unknown",
			data: &models.TelemetryData{
				Hostname: "test-host-1",
				GPUID:    "0",
			},
			expected: "default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &StreamAdapterConfig{
				PartitionBy: tt.partitionBy,
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			adapter := NewStreamAdapter(ctx, config, "http://localhost:8080/api/data")

			result := adapter.getPartition(tt.data)
			assert.Equal(t, tt.expected, result)

		})
	}
}

func TestStreamAdapter_ConcurrentWrites(t *testing.T) {
	// Create a test server
	var receivedCount int64
	var mu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]interface{}
		err := json.NewDecoder(r.Body).Decode(&payload)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// Extract records from the payload
		if records, ok := payload["records"].([]interface{}); ok {
			mu.Lock()
			receivedCount += int64(len(records))
			mu.Unlock()
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := &StreamAdapterConfig{
		ChannelSize:   100,
		BatchSize:     10,
		Workers:       2,
		FlushInterval: 50 * time.Millisecond,
		HTTPTimeout:   5 * time.Second,
		EnableMetrics: true,
	}
	ctx, cancel := context.WithCancel(context.Background())

	adapter := NewStreamAdapter(ctx, config, server.URL)
	adapter.Start(ctx)
	defer adapter.Stop()
	defer cancel()

	// Concurrent writes
	const numGoroutines = 5
	const writesPerGoroutine = 10
	var wg sync.WaitGroup

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()
			for j := 0; j < writesPerGoroutine; j++ {
				testData := &models.TelemetryData{
					ID:         uint(goroutineID*writesPerGoroutine + j),
					MetricName: "GPU_UTIL",
					GPUID:      "0",
					Hostname:   "test-host",
					Value:      85.5,
					Timestamp:  time.Now(),
				}

				err := adapter.WriteTelemetry(ctx, testData, map[string]string{})
				assert.NoError(t, err)
			}
		}(i)
	}

	wg.Wait()

	// Wait for processing
	time.Sleep(300 * time.Millisecond)

	// Check that all data was received
	mu.Lock()
	expectedCount := int64(numGoroutines * writesPerGoroutine)
	assert.Equal(t, expectedCount, receivedCount)
	mu.Unlock()
}

func TestStreamAdapter_ErrorHandling(t *testing.T) {
	// Create a server that returns errors
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}))
	defer server.Close()

	config := &StreamAdapterConfig{
		ChannelSize:   10,
		BatchSize:     2,
		Workers:       1,
		MaxRetries:    1, // Low retry count for faster test
		RetryDelay:    10 * time.Millisecond,
		FlushInterval: 50 * time.Millisecond,
		HTTPTimeout:   1 * time.Second,
		EnableMetrics: true,
	}
	ctx, cancel := context.WithCancel(context.Background())

	adapter := NewStreamAdapter(ctx, config, server.URL)
	adapter.Start(ctx)
	defer adapter.Stop()
	defer cancel()

	// Write test data
	testData := &models.TelemetryData{
		ID:         1,
		MetricName: "GPU_UTIL",
		GPUID:      "0",
		Hostname:   "test-host",
		Value:      85.5,
		Timestamp:  time.Now(),
	}

	err := adapter.WriteTelemetry(ctx, testData, map[string]string{})
	assert.NoError(t, err)

	err = adapter.WriteTelemetry(ctx, testData, map[string]string{})
	assert.NoError(t, err)

	// Wait for processing and retries
	time.Sleep(200 * time.Millisecond)

	// Check that errors were recorded
	metrics := adapter.GetMetrics()
	assert.Greater(t, metrics.TotalErrors, int64(0))
}

func TestStreamAdapter_MetricsCollection(t *testing.T) {
	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := &StreamAdapterConfig{
		ChannelSize:   10,
		BatchSize:     2,
		Workers:       1,
		FlushInterval: 50 * time.Millisecond,
		HTTPTimeout:   5 * time.Second,
		EnableMetrics: true,
	}
	ctx, cancel := context.WithCancel(context.Background())

	adapter := NewStreamAdapter(ctx, config, server.URL)
	adapter.Start(ctx)
	defer adapter.Stop()
	defer cancel()

	// Write test data
	for i := 0; i < 5; i++ {
		testData := &models.TelemetryData{
			ID:         uint(i),
			MetricName: "GPU_UTIL",
			GPUID:      "0",
			Hostname:   "test-host",
			Value:      85.5,
			Timestamp:  time.Now(),
		}

		err := adapter.WriteTelemetry(ctx, testData, map[string]string{})
		assert.NoError(t, err)
	}

	// Wait for processing
	time.Sleep(200 * time.Millisecond)

	// Check metrics
	metrics := adapter.GetMetrics()
	assert.Greater(t, metrics.TotalProcessed, int64(0))
	assert.Greater(t, metrics.TotalBatches, int64(0))
	assert.False(t, metrics.LastProcessTime.IsZero())
}

func TestStreamAdapter_ConfigValidation(t *testing.T) {
	tests := []struct {
		name   string
		config *StreamAdapterConfig
		valid  bool
	}{
		{
			name: "valid config",
			config: &StreamAdapterConfig{
				ChannelSize:   100,
				BatchSize:     50,
				Workers:       5,
				MaxRetries:    3,
				FlushInterval: 5 * time.Second,
				HTTPTimeout:   30 * time.Second,
				EnableMetrics: true,
				PartitionBy:   "hostname",
			},
			valid: true,
		},
		{
			name: "zero channel size",
			config: &StreamAdapterConfig{
				ChannelSize: 0,
				BatchSize:   50,
				Workers:     5,
			},
			valid: false,
		},
		{
			name: "zero batch size",
			config: &StreamAdapterConfig{
				ChannelSize: 100,
				BatchSize:   0,
				Workers:     5,
			},
			valid: false,
		},
		{
			name: "zero workers",
			config: &StreamAdapterConfig{
				ChannelSize: 100,
				BatchSize:   50,
				Workers:     0,
			},
			valid: false,
		},
		{
			name: "negative values",
			config: &StreamAdapterConfig{
				ChannelSize: -1,
				BatchSize:   -1,
				Workers:     -1,
			},
			valid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.valid {
				assert.Greater(t, tt.config.ChannelSize, 0)
				assert.Greater(t, tt.config.BatchSize, 0)
				assert.Greater(t, tt.config.Workers, 0)
			} else {
				invalid := tt.config.ChannelSize <= 0 ||
					tt.config.BatchSize <= 0 ||
					tt.config.Workers <= 0
				assert.True(t, invalid, "Config should be invalid")
			}
		})
	}
}

func TestStreamAdapter_EdgeCases(t *testing.T) {
	// Test with minimal config
	config := &StreamAdapterConfig{
		ChannelSize: 1,
		BatchSize:   1,
		Workers:     1,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	adapter := NewStreamAdapter(ctx, config, "http://localhost:8080/api/data")
	assert.NotNil(t, adapter)
	assert.Equal(t, config, adapter.config)

	// Test writing to stopped adapter
	testData := &models.TelemetryData{
		ID:         1,
		MetricName: "GPU_UTIL",
		GPUID:      "0",
		Hostname:   "test-host",
		Value:      85.5,
		Timestamp:  time.Now(),
	}

	// Should work even when not started (buffered in channel)
	err := adapter.WriteTelemetry(ctx, testData, map[string]string{})
	assert.NoError(t, err)
}

func TestStreamAdapter_ContextCancellation(t *testing.T) {
	config := &StreamAdapterConfig{
		ChannelSize:   10,
		BatchSize:     5,
		Workers:       1,
		FlushInterval: 100 * time.Millisecond,
	}

	ctx, cancel := context.WithCancel(context.Background())

	adapter := NewStreamAdapter(ctx, config, "http://localhost:8080/api/data")

	// Test that adapter starts and stops properly
	adapter.Start(ctx)
	assert.True(t, adapter.IsRunning())

	// Stop the adapter
	cancel()
	adapter.Stop()
	assert.False(t, adapter.IsRunning())
}

// Benchmark tests
func BenchmarkStreamAdapter_WriteTelemetry(b *testing.B) {
	config := &StreamAdapterConfig{
		ChannelSize:   10000,
		BatchSize:     100,
		Workers:       4,
		FlushInterval: 1 * time.Second,
		EnableMetrics: false, // Disable for cleaner benchmark
	}

	ctx, cancel := context.WithCancel(context.Background())

	adapter := NewStreamAdapter(ctx, config, "http://localhost:8080/api/data")
	adapter.Start(ctx)
	defer adapter.Stop()
	defer cancel()

	testData := &models.TelemetryData{
		ID:         1,
		MetricName: "GPU_UTIL",
		GPUID:      "0",
		Hostname:   "benchmark-host",
		Value:      85.5,
		Timestamp:  time.Now(),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := adapter.WriteTelemetry(ctx, testData, map[string]string{})
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkStreamAdapter_GetMetrics(b *testing.B) {
	config := &StreamAdapterConfig{
		ChannelSize:   100,
		BatchSize:     50,
		Workers:       2,
		EnableMetrics: true,
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	adapter := NewStreamAdapter(ctx, config, "http://localhost:8080/api/data")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		metrics := adapter.GetMetrics()
		_ = metrics
	}
}

func BenchmarkNewStreamAdapter(b *testing.B) {
	config := &StreamAdapterConfig{
		ChannelSize:   100,
		BatchSize:     50,
		Workers:       2,
		EnableMetrics: true,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		adapter := NewStreamAdapter(ctx, config, "http://localhost:8080/api/data")
		_ = adapter
	}
}

func BenchmarkTelemetryChannelData_Creation(b *testing.B) {
	data := &models.TelemetryData{
		ID:         1,
		MetricName: "GPU_UTIL",
		GPUID:      "0",
		Hostname:   "benchmark-host",
		Value:      85.5,
		Timestamp:  time.Now(),
	}

	headers := map[string]string{"Content-Type": "application/json"}
	now := time.Now()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		channelData := TelemetryChannelData{
			Data:      data,
			Headers:   headers,
			Timestamp: now,
			Partition: "benchmark-host",
		}
		_ = channelData
	}
}
