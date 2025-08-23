package streamer

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/dilipmighty245/telemetry-pipeline/pkg/messagequeue"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNexusStreamerConfig_Validation(t *testing.T) {
	// Setup embedded etcd server for testing
	etcdServer, cleanup, err := messagequeue.SetupEtcdForTest()
	require.NoError(t, err)
	defer cleanup()

	tests := []struct {
		name   string
		config *NexusStreamerConfig
		valid  bool
	}{
		{
			name: "valid config",
			config: &NexusStreamerConfig{
				EtcdEndpoints:      etcdServer.Endpoints,
				ClusterID:          "test-cluster",
				StreamerID:         "test-streamer",
				MessageQueuePrefix: "/telemetry/queue",
				BatchSize:          100,
				StreamInterval:     3 * time.Second,
				HTTPPort:           8081,
				EnableHTTP:         true,
				UploadDir:          "/tmp/uploads",
				MaxUploadSize:      100 * 1024 * 1024,
				MaxMemory:          32 * 1024 * 1024,
				LogLevel:           "info",
			},
			valid: true,
		},
		{
			name: "minimal config",
			config: &NexusStreamerConfig{
				EtcdEndpoints: etcdServer.Endpoints,
				ClusterID:     "test-cluster",
				StreamerID:    "test-streamer",
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
				assert.NotEmpty(t, tt.config.StreamerID)
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

func TestCSVUploadRequest_Validation(t *testing.T) {
	request := &CSVUploadRequest{
		Description: "Test upload",
		Tags:        []string{"test", "gpu"},
		Metadata:    map[string]string{"source": "test"},
	}

	assert.Equal(t, "Test upload", request.Description)
	assert.Contains(t, request.Tags, "test")
	assert.Equal(t, "test", request.Metadata["source"])
}

func TestCSVUploadResponse_Validation(t *testing.T) {
	now := time.Now()
	response := &CSVUploadResponse{
		Success:     true,
		FileID:      "test-file-123",
		Filename:    "test.csv",
		Size:        1024,
		MD5Hash:     "d41d8cd98f00b204e9800998ecf8427e",
		RecordCount: 100,
		Headers:     []string{"timestamp", "gpu_id", "hostname"},
		UploadedAt:  now,
		ProcessedAt: &now,
		Status:      "processed",
		Metadata:    map[string]string{"processing_time": "1s"},
	}

	assert.True(t, response.Success)
	assert.Equal(t, "test-file-123", response.FileID)
	assert.Equal(t, "test.csv", response.Filename)
	assert.Equal(t, int64(1024), response.Size)
	assert.Equal(t, 100, response.RecordCount)
	assert.Equal(t, "processed", response.Status)
	assert.NotNil(t, response.ProcessedAt)
}

func TestNexusStreamerService_ParseConfig(t *testing.T) {
	// Set environment variables for testing
	os.Setenv("CLUSTER_ID", "test-cluster")
	os.Setenv("STREAMER_ID", "test-streamer")
	os.Setenv("ETCD_ENDPOINTS", "localhost:2379,localhost:2380")
	os.Setenv("BATCH_SIZE", "200")
	os.Setenv("HTTP_PORT", "9081")
	os.Setenv("LOG_LEVEL", "debug")
	defer func() {
		os.Unsetenv("CLUSTER_ID")
		os.Unsetenv("STREAMER_ID")
		os.Unsetenv("ETCD_ENDPOINTS")
		os.Unsetenv("BATCH_SIZE")
		os.Unsetenv("HTTP_PORT")
		os.Unsetenv("LOG_LEVEL")
	}()

	service := &NexusStreamerService{}
	config, err := service.parseConfig([]string{"test"})

	require.NoError(t, err)
	assert.Equal(t, "test-cluster", config.ClusterID)
	assert.Equal(t, "test-streamer", config.StreamerID)
	assert.Equal(t, []string{"localhost:2379", "localhost:2380"}, config.EtcdEndpoints)
	assert.Equal(t, 200, config.BatchSize)
	assert.Equal(t, 9081, config.HTTPPort)
	assert.Equal(t, "debug", config.LogLevel)
}

func TestNexusStreamerService_ParseConfigDefaults(t *testing.T) {
	service := &NexusStreamerService{}
	config, err := service.parseConfig([]string{"test"})

	require.NoError(t, err)
	assert.Equal(t, "default-cluster", config.ClusterID)
	assert.Contains(t, config.StreamerID, "streamer-")
	assert.Equal(t, []string{"localhost:2379"}, config.EtcdEndpoints)
	assert.Equal(t, 100, config.BatchSize)
	assert.Equal(t, 8081, config.HTTPPort)
	assert.Equal(t, "info", config.LogLevel)
	assert.Equal(t, "/telemetry/queue", config.MessageQueuePrefix)
	assert.Equal(t, 3*time.Second, config.StreamInterval)
	assert.True(t, config.EnableHTTP)
	assert.Equal(t, "/tmp/telemetry-uploads", config.UploadDir)
	assert.Equal(t, int64(100*1024*1024), config.MaxUploadSize)
	assert.Equal(t, int64(32*1024*1024), config.MaxMemory)
}

func TestNexusStreamerService_ParseCSVRecord(t *testing.T) {
	service := &NexusStreamerService{}

	headers := []string{"timestamp", "gpu_id", "hostname", "uuid", "device", "modelname", "gpu_utilization", "memory_utilization"}
	columnMap := make(map[string]int)
	for i, header := range headers {
		columnMap[strings.ToLower(header)] = i
	}

	tests := []struct {
		name    string
		record  []string
		wantErr bool
	}{
		{
			name: "valid record",
			record: []string{
				"2023-01-01T00:00:00Z",
				"0",
				"test-host",
				"GPU-12345",
				"nvidia0",
				"NVIDIA H100",
				"85.5",
				"60.2",
			},
			wantErr: false,
		},
		{
			name: "missing required fields",
			record: []string{
				"2023-01-01T00:00:00Z",
				"", // missing gpu_id
				"", // missing hostname
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := service.parseCSVRecord(tt.record, columnMap)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.NotEmpty(t, result.GPUID)
				assert.NotEmpty(t, result.Hostname)
				assert.NotEmpty(t, result.Timestamp)
			}
		})
	}
}

func TestNexusStreamerService_ParseCSVRecord_TimestampHandling(t *testing.T) {
	service := &NexusStreamerService{}

	columnMap := map[string]int{
		"timestamp": 0,
		"gpu_id":    1,
		"hostname":  2,
	}

	tests := []struct {
		name      string
		timestamp string
		wantErr   bool
	}{
		{
			name:      "valid RFC3339 timestamp",
			timestamp: "2023-01-01T00:00:00Z",
			wantErr:   false,
		},
		{
			name:      "invalid timestamp format",
			timestamp: "invalid-timestamp",
			wantErr:   false, // Should not error, but use current time
		},
		{
			name:      "empty timestamp",
			timestamp: "",
			wantErr:   false, // Should not error, but use current time
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			record := []string{tt.timestamp, "0", "test-host"}
			result, err := service.parseCSVRecord(record, columnMap)

			assert.NoError(t, err)
			assert.NotNil(t, result)
			assert.NotEmpty(t, result.Timestamp)

			// Verify timestamp is in RFC3339 format
			_, parseErr := time.Parse(time.RFC3339, result.Timestamp)
			assert.NoError(t, parseErr)
		})
	}
}

func TestNexusStreamerService_HealthHandler(t *testing.T) {
	service := &NexusStreamerService{
		config: &NexusStreamerConfig{
			StreamerID: "test-streamer",
			ClusterID:  "test-cluster",
		},
		startTime: time.Now().Add(-1 * time.Hour),
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := service.healthHandler(c)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var response map[string]interface{}
	err = json.Unmarshal(rec.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "healthy", response["status"])
	assert.Equal(t, "nexus-streamer", response["service"])
	assert.Equal(t, "test-streamer", response["streamer_id"])
	assert.Equal(t, "test-cluster", response["cluster_id"])
}

func TestNexusStreamerService_StatusHandler(t *testing.T) {
	service := &NexusStreamerService{
		config: &NexusStreamerConfig{
			StreamerID:     "test-streamer",
			ClusterID:      "test-cluster",
			BatchSize:      100,
			StreamInterval: 3 * time.Second,
			HTTPPort:       8081,
		},
		messageCount: 1000,
		startTime:    time.Now().Add(-1 * time.Hour),
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := service.statusHandler(c)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var response map[string]interface{}
	err = json.Unmarshal(rec.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "nexus-streamer", response["service"])
	assert.Equal(t, "test-streamer", response["streamer_id"])
	assert.Equal(t, "test-cluster", response["cluster_id"])
	assert.Equal(t, float64(1000), response["message_count"])
}

func TestNexusStreamerService_UploadCSVHandler_InvalidRequests(t *testing.T) {
	service := &NexusStreamerService{
		config: &NexusStreamerConfig{
			MaxMemory:     32 * 1024 * 1024,
			MaxUploadSize: 100 * 1024 * 1024,
		},
	}

	e := echo.New()

	// Test with no file
	req := httptest.NewRequest(http.MethodPost, "/api/v1/csv/upload", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := service.uploadCSVHandler(c)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	// Test with non-CSV file
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("file", "test.txt")
	part.Write([]byte("test content"))
	writer.Close()

	req = httptest.NewRequest(http.MethodPost, "/api/v1/csv/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)

	err = service.uploadCSVHandler(c)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestNexusStreamerService_PrintStats(t *testing.T) {
	service := &NexusStreamerService{
		messageCount: 1000,
		startTime:    time.Now().Add(-1 * time.Hour),
	}

	// This should not panic
	assert.NotPanics(t, func() {
		service.PrintStats()
	})
}

func TestNexusStreamerService_Close(t *testing.T) {
	service := &NexusStreamerService{}

	// Test closing without initialization
	err := service.Close()
	assert.NoError(t, err)
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

// Benchmark tests
func BenchmarkNexusStreamerConfig_Creation(b *testing.B) {
	for i := 0; i < b.N; i++ {
		config := &NexusStreamerConfig{
			EtcdEndpoints:      []string{"localhost:2379"},
			ClusterID:          "test-cluster",
			StreamerID:         "test-streamer",
			MessageQueuePrefix: "/telemetry/queue",
			BatchSize:          100,
			StreamInterval:     3 * time.Second,
			HTTPPort:           8081,
			EnableHTTP:         true,
			UploadDir:          "/tmp/uploads",
			MaxUploadSize:      100 * 1024 * 1024,
			MaxMemory:          32 * 1024 * 1024,
			LogLevel:           "info",
		}
		_ = config
	}
}

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

func BenchmarkCSVUploadResponse_Creation(b *testing.B) {
	now := time.Now()
	for i := 0; i < b.N; i++ {
		response := &CSVUploadResponse{
			Success:     true,
			FileID:      "test-file-123",
			Filename:    "test.csv",
			Size:        1024,
			MD5Hash:     "d41d8cd98f00b204e9800998ecf8427e",
			RecordCount: 100,
			Headers:     []string{"timestamp", "gpu_id", "hostname"},
			UploadedAt:  now,
			ProcessedAt: &now,
			Status:      "processed",
			Metadata:    map[string]string{"processing_time": "1s"},
		}
		_ = response
	}
}
