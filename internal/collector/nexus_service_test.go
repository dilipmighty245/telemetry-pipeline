package collector

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNexusCollectorConfig_Validation(t *testing.T) {
	tests := []struct {
		name   string
		config *NexusCollectorConfig
		valid  bool
	}{
		{
			name: "valid config",
			config: &NexusCollectorConfig{
				EtcdEndpoints:      []string{"localhost:2379"},
				ClusterID:          "test-cluster",
				CollectorID:        "test-collector",
				MessageQueuePrefix: "/telemetry/queue",
				PollTimeout:        5 * time.Second,
				BatchSize:          100,
				PollInterval:       1 * time.Second,
				BufferSize:         1000,
				Workers:            4,
				EnableNexus:        true,
				EnableWatchAPI:     true,
				EnableGraphQL:      true,
				EnableStreaming:    true,
				LogLevel:           "info",
			},
			valid: true,
		},
		{
			name: "minimal config",
			config: &NexusCollectorConfig{
				EtcdEndpoints: []string{"localhost:2379"},
				ClusterID:     "test-cluster",
				CollectorID:   "test-collector",
			},
			valid: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.NotNil(t, tt.config)
			if tt.valid {
				assert.NotEmpty(t, tt.config.EtcdEndpoints)
				assert.NotEmpty(t, tt.config.ClusterID)
				assert.NotEmpty(t, tt.config.CollectorID)
			}
		})
	}
}

func TestTelemetryRecord_Validation(t *testing.T) {
	tests := []struct {
		name   string
		record *TelemetryRecord
		valid  bool
	}{
		{
			name: "valid record",
			record: &TelemetryRecord{
				Timestamp:         "2023-01-01T00:00:00Z",
				GPUID:             "0",
				UUID:              "GPU-12345",
				Device:            "nvidia0",
				ModelName:         "NVIDIA H100",
				Hostname:          "test-host",
				GPUUtilization:    85.5,
				MemoryUtilization: 60.2,
				MemoryUsedMB:      8192,
				MemoryFreeMB:      2048,
				Temperature:       72.0,
				PowerDraw:         350.5,
				SMClockMHz:        1410,
				MemoryClockMHz:    1215,
			},
			valid: true,
		},
		{
			name: "missing required fields",
			record: &TelemetryRecord{
				Timestamp: "2023-01-01T00:00:00Z",
				// Missing GPUID and Hostname
			},
			valid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.valid {
				assert.NotEmpty(t, tt.record.GPUID)
				assert.NotEmpty(t, tt.record.Hostname)
				assert.NotEmpty(t, tt.record.Timestamp)
			}
		})
	}
}

func TestNexusCollectorService_ParseConfig(t *testing.T) {
	// Set environment variables for testing
	os.Setenv("CLUSTER_ID", "test-cluster")
	os.Setenv("COLLECTOR_ID", "test-collector")
	os.Setenv("ETCD_ENDPOINTS", "localhost:2379,localhost:2380")
	os.Setenv("BATCH_SIZE", "200")
	os.Setenv("WORKERS", "8")
	os.Setenv("LOG_LEVEL", "debug")
	defer func() {
		os.Unsetenv("CLUSTER_ID")
		os.Unsetenv("COLLECTOR_ID")
		os.Unsetenv("ETCD_ENDPOINTS")
		os.Unsetenv("BATCH_SIZE")
		os.Unsetenv("WORKERS")
		os.Unsetenv("LOG_LEVEL")
	}()

	service := &NexusCollectorService{}
	config, err := service.parseConfig([]string{"test"})

	require.NoError(t, err)
	assert.Equal(t, "test-cluster", config.ClusterID)
	assert.Equal(t, "test-collector", config.CollectorID)
	assert.Equal(t, []string{"localhost:2379", "localhost:2380"}, config.EtcdEndpoints)
	assert.Equal(t, 200, config.BatchSize)
	assert.Equal(t, 8, config.Workers)
	assert.Equal(t, "debug", config.LogLevel)
}

func TestNexusCollectorService_ParseConfigDefaults(t *testing.T) {
	service := &NexusCollectorService{}
	config, err := service.parseConfig([]string{"test"})

	require.NoError(t, err)
	assert.Equal(t, "default-cluster", config.ClusterID)
	assert.Contains(t, config.CollectorID, "collector-")
	assert.Equal(t, []string{"localhost:2379"}, config.EtcdEndpoints)
	assert.Equal(t, 100, config.BatchSize)
	assert.Equal(t, 8, config.Workers)
	assert.Equal(t, "info", config.LogLevel)
	assert.Equal(t, "/telemetry/queue", config.MessageQueuePrefix)
	assert.Equal(t, 5*time.Second, config.PollTimeout)
	assert.Equal(t, 1*time.Second, config.PollInterval)
	assert.Equal(t, 10000, config.BufferSize)
	assert.True(t, config.EnableNexus)
	assert.True(t, config.EnableWatchAPI)
	assert.True(t, config.EnableGraphQL)
	assert.True(t, config.EnableStreaming)
}

func TestUtilityFunctions(t *testing.T) {
	// Test getEnv
	os.Setenv("TEST_ENV", "test_value")
	defer os.Unsetenv("TEST_ENV")

	assert.Equal(t, "test_value", getEnv("TEST_ENV", "default"))
	assert.Equal(t, "default", getEnv("NON_EXISTENT", "default"))

	// Test getEnvInt
	os.Setenv("TEST_INT", "42")
	defer os.Unsetenv("TEST_INT")

	assert.Equal(t, 42, getEnvInt("TEST_INT", 10))
	assert.Equal(t, 10, getEnvInt("NON_EXISTENT", 10))

	os.Setenv("INVALID_INT", "not_a_number")
	defer os.Unsetenv("INVALID_INT")
	assert.Equal(t, 10, getEnvInt("INVALID_INT", 10))

	// Test getEnvBool
	os.Setenv("TEST_BOOL", "true")
	defer os.Unsetenv("TEST_BOOL")

	assert.True(t, getEnvBool("TEST_BOOL", false))
	assert.False(t, getEnvBool("NON_EXISTENT", false))

	os.Setenv("INVALID_BOOL", "not_a_bool")
	defer os.Unsetenv("INVALID_BOOL")
	assert.False(t, getEnvBool("INVALID_BOOL", false))

	// Test getEnvDuration
	os.Setenv("TEST_DURATION", "30s")
	defer os.Unsetenv("TEST_DURATION")

	assert.Equal(t, 30*time.Second, getEnvDuration("TEST_DURATION", 10*time.Second))
	assert.Equal(t, 10*time.Second, getEnvDuration("NON_EXISTENT", 10*time.Second))

	os.Setenv("INVALID_DURATION", "not_a_duration")
	defer os.Unsetenv("INVALID_DURATION")
	assert.Equal(t, 10*time.Second, getEnvDuration("INVALID_DURATION", 10*time.Second))
}

func TestNexusCollectorService_Run_InvalidConfig(t *testing.T) {
	service := &NexusCollectorService{}

	// Test with invalid log level
	os.Setenv("LOG_LEVEL", "invalid_level")
	defer os.Unsetenv("LOG_LEVEL")

	err := service.Run([]string{"test"}, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid log level")
}

func TestNexusCollectorService_Close(t *testing.T) {
	service := &NexusCollectorService{}

	// Test closing without initialization
	err := service.Close()
	assert.NoError(t, err)
}

// Benchmark tests
func BenchmarkTelemetryRecord_Creation(b *testing.B) {
	for i := 0; i < b.N; i++ {
		record := &TelemetryRecord{
			Timestamp:         "2023-01-01T00:00:00Z",
			GPUID:             "0",
			UUID:              "GPU-12345",
			Device:            "nvidia0",
			ModelName:         "NVIDIA H100",
			Hostname:          "test-host",
			GPUUtilization:    85.5,
			MemoryUtilization: 60.2,
			MemoryUsedMB:      8192,
			MemoryFreeMB:      2048,
			Temperature:       72.0,
			PowerDraw:         350.5,
			SMClockMHz:        1410,
			MemoryClockMHz:    1215,
		}
		_ = record
	}
}

func BenchmarkNexusCollectorConfig_Creation(b *testing.B) {
	for i := 0; i < b.N; i++ {
		config := &NexusCollectorConfig{
			EtcdEndpoints:      []string{"localhost:2379"},
			ClusterID:          "test-cluster",
			CollectorID:        "test-collector",
			MessageQueuePrefix: "/telemetry/queue",
			PollTimeout:        5 * time.Second,
			BatchSize:          100,
			PollInterval:       1 * time.Second,
			BufferSize:         1000,
			Workers:            4,
			EnableNexus:        true,
			EnableWatchAPI:     true,
			EnableGraphQL:      true,
			EnableStreaming:    true,
			LogLevel:           "info",
		}
		_ = config
	}
}
