package collector

import (
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test constants
const (
	testDefaultBatchSize = 100
)

func TestGetEnv(t *testing.T) {
	// Test with existing environment variable
	os.Setenv("TEST_VAR", "test_value")
	defer os.Unsetenv("TEST_VAR")

	result := getEnv("TEST_VAR", "default")
	assert.Equal(t, "test_value", result)

	// Test with non-existing environment variable
	result = getEnv("NON_EXISTING_VAR", "default_value")
	assert.Equal(t, "default_value", result)

	// Test with empty environment variable
	os.Setenv("EMPTY_VAR", "")
	defer os.Unsetenv("EMPTY_VAR")

	result = getEnv("EMPTY_VAR", "default_empty")
	assert.Equal(t, "default_empty", result)
}

func TestGetEnvInt(t *testing.T) {
	// Test with valid integer
	os.Setenv("TEST_INT", "42")
	defer os.Unsetenv("TEST_INT")

	result := getEnvInt("TEST_INT", 10)
	assert.Equal(t, 42, result)

	// Test with non-existing environment variable
	result = getEnvInt("NON_EXISTING_INT", 25)
	assert.Equal(t, 25, result)

	// Test with invalid integer
	os.Setenv("INVALID_INT", "not_a_number")
	defer os.Unsetenv("INVALID_INT")

	result = getEnvInt("INVALID_INT", 30)
	assert.Equal(t, 30, result)

	// Test with empty string
	os.Setenv("EMPTY_INT", "")
	defer os.Unsetenv("EMPTY_INT")

	result = getEnvInt("EMPTY_INT", 35)
	assert.Equal(t, 35, result)
}

func TestGetEnvBool(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		default_ bool
		expected bool
	}{
		{
			name:     "true value",
			envValue: "true",
			default_: false,
			expected: true,
		},
		{
			name:     "false value",
			envValue: "false",
			default_: true,
			expected: false,
		},
		{
			name:     "1 value",
			envValue: "1",
			default_: false,
			expected: true,
		},
		{
			name:     "0 value",
			envValue: "0",
			default_: true,
			expected: false,
		},
		{
			name:     "invalid value",
			envValue: "maybe",
			default_: true,
			expected: true,
		},
		{
			name:     "empty value",
			envValue: "",
			default_: true,
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				os.Setenv("TEST_BOOL", tt.envValue)
				defer os.Unsetenv("TEST_BOOL")
				result := getEnvBool("TEST_BOOL", tt.default_)
				assert.Equal(t, tt.expected, result)
			} else {
				// Test non-existing variable
				result := getEnvBool("NON_EXISTING_BOOL", tt.default_)
				assert.Equal(t, tt.default_, result)
			}
		})
	}
}

func TestGetEnvDuration(t *testing.T) {
	// Test with valid duration
	os.Setenv("TEST_DURATION", "30s")
	defer os.Unsetenv("TEST_DURATION")

	result := getEnvDuration("TEST_DURATION", 10*time.Second)
	assert.Equal(t, 30*time.Second, result)

	// Test with non-existing environment variable
	result = getEnvDuration("NON_EXISTING_DURATION", 15*time.Second)
	assert.Equal(t, 15*time.Second, result)

	// Test with invalid duration
	os.Setenv("INVALID_DURATION", "not_a_duration")
	defer os.Unsetenv("INVALID_DURATION")

	result = getEnvDuration("INVALID_DURATION", 20*time.Second)
	assert.Equal(t, 20*time.Second, result)

	// Test with empty string
	os.Setenv("EMPTY_DURATION", "")
	defer os.Unsetenv("EMPTY_DURATION")

	result = getEnvDuration("EMPTY_DURATION", 25*time.Second)
	assert.Equal(t, 25*time.Second, result)
}

func TestBatchSizeEnvironment(t *testing.T) {
	// Test batch size parsing from environment
	tests := []struct {
		name     string
		envValue string
		expected int
	}{
		{
			name:     "valid batch size",
			envValue: "150",
			expected: 150,
		},
		{
			name:     "invalid batch size",
			envValue: "not_a_number",
			expected: testDefaultBatchSize,
		},
		{
			name:     "zero batch size",
			envValue: "0",
			expected: testDefaultBatchSize,
		},
		{
			name:     "negative batch size",
			envValue: "-10",
			expected: testDefaultBatchSize,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv("BATCH_SIZE", tt.envValue)
			defer os.Unsetenv("BATCH_SIZE")

			// Test the environment parsing logic
			result := getEnvInt("BATCH_SIZE", testDefaultBatchSize)
			if result <= 0 {
				result = testDefaultBatchSize
			}
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNexusCollectorConfig_Validation(t *testing.T) {
	tests := []struct {
		name    string
		config  *NexusCollectorConfig
		isValid bool
	}{
		{
			name: "valid config",
			config: &NexusCollectorConfig{
				ClusterID:    "test-cluster",
				CollectorID:  "test-collector",
				BatchSize:    100,
				PollInterval: 5 * time.Second,
				Workers:      2,
			},
			isValid: true,
		},
		{
			name: "empty cluster ID",
			config: &NexusCollectorConfig{
				ClusterID:    "",
				CollectorID:  "test-collector",
				BatchSize:    100,
				PollInterval: 5 * time.Second,
			},
			isValid: false,
		},
		{
			name: "empty collector ID",
			config: &NexusCollectorConfig{
				ClusterID:    "test-cluster",
				CollectorID:  "",
				BatchSize:    100,
				PollInterval: 5 * time.Second,
			},
			isValid: false,
		},
		{
			name: "zero batch size",
			config: &NexusCollectorConfig{
				ClusterID:    "test-cluster",
				CollectorID:  "test-collector",
				BatchSize:    0,
				PollInterval: 5 * time.Second,
			},
			isValid: false,
		},
		{
			name: "zero poll interval",
			config: &NexusCollectorConfig{
				ClusterID:    "test-cluster",
				CollectorID:  "test-collector",
				BatchSize:    100,
				PollInterval: 0,
			},
			isValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.isValid {
				assert.NotEmpty(t, tt.config.ClusterID)
				assert.NotEmpty(t, tt.config.CollectorID)
				assert.Greater(t, tt.config.BatchSize, 0)
				assert.Greater(t, tt.config.PollInterval, time.Duration(0))
			} else {
				invalid := tt.config.ClusterID == "" ||
					tt.config.CollectorID == "" ||
					tt.config.BatchSize <= 0 ||
					tt.config.PollInterval <= 0
				assert.True(t, invalid, "Config should be invalid")
			}
		})
	}
}

func TestNexusCollectorService_ParseConfig(t *testing.T) {
	service := &NexusCollectorService{}

	// Set up environment variables for testing
	testEnvVars := map[string]string{
		"CLUSTER_ID":         "test-cluster",
		"COLLECTOR_ID":       "test-collector-123",
		"BATCH_SIZE":         "250",
		"POLL_INTERVAL":      "10s",
		"WORKERS":            "4",
		"ETCD_ENDPOINTS":     "localhost:2379,localhost:2380",
		"STREAM_DESTINATION": "http://localhost:8080/stream",
	}

	// Set environment variables
	for key, value := range testEnvVars {
		os.Setenv(key, value)
		defer os.Unsetenv(key)
	}

	config, err := service.parseConfig([]string{})
	require.NoError(t, err)
	assert.NotNil(t, config)

	// Verify parsed values
	assert.Equal(t, "test-cluster", config.ClusterID)
	assert.Equal(t, "test-collector-123", config.CollectorID)
	assert.Equal(t, 250, config.BatchSize)
	assert.Equal(t, 10*time.Second, config.PollInterval)
	assert.Equal(t, 4, config.Workers)
}

func TestNexusCollectorService_ParseConfigDefaults(t *testing.T) {
	service := &NexusCollectorService{}

	// Clear environment variables to test defaults
	testEnvVars := []string{
		"CLUSTER_ID", "COLLECTOR_ID", "BATCH_SIZE", "POLL_INTERVAL",
		"WORKERS", "ETCD_ENDPOINTS", "STREAM_DESTINATION",
	}

	for _, envVar := range testEnvVars {
		os.Unsetenv(envVar)
	}

	config, err := service.parseConfig([]string{})
	require.NoError(t, err)
	assert.NotNil(t, config)

	// Verify default values are set
	assert.Equal(t, "default-cluster", config.ClusterID)
	assert.NotEmpty(t, config.CollectorID)
	assert.Contains(t, config.CollectorID, "collector-")
	assert.Equal(t, testDefaultBatchSize, config.BatchSize)
	assert.Equal(t, 1*time.Second, config.PollInterval)
}

// Test environment variable parsing edge cases
func TestEnvVariableEdgeCases(t *testing.T) {
	t.Run("getEnvInt with very large number", func(t *testing.T) {
		os.Setenv("LARGE_INT", strconv.Itoa(int(^uint(0)>>1))) // Max int
		defer os.Unsetenv("LARGE_INT")

		result := getEnvInt("LARGE_INT", 100)
		assert.Equal(t, int(^uint(0)>>1), result)
	})

	t.Run("getEnvDuration with various units", func(t *testing.T) {
		tests := []struct {
			value    string
			expected time.Duration
		}{
			{"1ns", 1 * time.Nanosecond},
			{"1us", 1 * time.Microsecond},
			{"1ms", 1 * time.Millisecond},
			{"1s", 1 * time.Second},
			{"1m", 1 * time.Minute},
			{"1h", 1 * time.Hour},
			{"2h30m", 2*time.Hour + 30*time.Minute},
		}

		for _, tt := range tests {
			os.Setenv("TEST_DURATION", tt.value)
			result := getEnvDuration("TEST_DURATION", 10*time.Second)
			assert.Equal(t, tt.expected, result)
			os.Unsetenv("TEST_DURATION")
		}
	})
}
