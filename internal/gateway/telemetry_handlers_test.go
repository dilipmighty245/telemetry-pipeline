package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dilipmighty245/telemetry-pipeline/pkg/validation"
	"github.com/dilipmighty245/telemetry-pipeline/test/testhelper"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	clientv3 "go.etcd.io/etcd/client/v3"
)

// setupTestEtcd creates and starts an embedded etcd server for testing
func setupTestEtcd(t *testing.T) (*testhelper.EtcdTestServer, func()) {
	etcdServer, err := testhelper.StartEtcdTestServer()
	require.NoError(t, err)
	
	cleanup := func() {
		if etcdServer != nil {
			etcdServer.Server.Close()
		}
	}
	
	return etcdServer, cleanup
}

func TestNexusGatewayService_parseAndValidateTelemetryQuery(t *testing.T) {
	now := time.Now()
	validStartTime := now.Add(-time.Hour).Format(time.RFC3339)
	validEndTime := now.Format(time.RFC3339)
	
	tests := []struct {
		name           string
		pathParam      string
		queryParams    map[string]string
		expectedGPUID  string
		expectedLimit  int
		expectedError  bool
		errorContains  string
	}{
		{
			name:          "valid basic query",
			pathParam:     "gpu-0",
			queryParams:   map[string]string{},
			expectedGPUID: "gpu-0",
			expectedLimit: 1000, // default
			expectedError: false,
		},
		{
			name:      "valid query with all parameters",
			pathParam: "gpu-1",
			queryParams: map[string]string{
				"start_time": validStartTime,
				"end_time":   validEndTime,
				"limit":      "100",
			},
			expectedGPUID: "gpu-1",
			expectedLimit: 100,
			expectedError: false,
		},
		{
			name:          "empty GPU ID",
			pathParam:     "",
			queryParams:   map[string]string{},
			expectedError: true,
			errorContains: "GPU ID is required",
		},
		{
			name:          "invalid GPU ID with special characters",
			pathParam:     "gpu@0",
			queryParams:   map[string]string{},
			expectedError: true,
			errorContains: "GPU ID contains invalid characters",
		},
		{
			name:      "invalid start time format",
			pathParam: "gpu-0",
			queryParams: map[string]string{
				"start_time": "invalid-time",
			},
			expectedError: true,
			errorContains: "invalid start_time format",
		},
		{
			name:      "invalid limit",
			pathParam: "gpu-0",
			queryParams: map[string]string{
				"limit": "abc",
			},
			expectedError: true,
			errorContains: "limit must be a valid integer",
		},
		{
			name:      "limit exceeds maximum",
			pathParam: "gpu-0",
			queryParams: map[string]string{
				"limit": "2000",
			},
			expectedError: true,
			errorContains: "limit cannot exceed 1000",
		},
		{
			name:      "start time after end time",
			pathParam: "gpu-0",
			queryParams: map[string]string{
				"start_time": validEndTime,
				"end_time":   validStartTime,
			},
			expectedError: true,
			errorContains: "start_time must be before end_time",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			e := echo.New()
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			// Set path parameter
			c.SetParamNames("id")
			c.SetParamValues(tt.pathParam)

			// Set query parameters
			q := req.URL.Query()
			for key, value := range tt.queryParams {
				q.Add(key, value)
			}
			req.URL.RawQuery = q.Encode()

			// Create gateway service with validator
			gateway := &NexusGatewayService{
				validator: validation.NewValidator(),
			}

			// Execute
			params, err := gateway.parseAndValidateTelemetryQuery(c)

			// Verify
			if tt.expectedError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorContains)
				assert.Nil(t, params)
			} else {
				assert.NoError(t, err)
				require.NotNil(t, params)
				assert.Equal(t, tt.expectedGPUID, params.GPUID)
				assert.Equal(t, tt.expectedLimit, params.Limit)
			}
		})
	}
}

func TestNexusGatewayService_isValidTelemetryKey(t *testing.T) {
	gateway := &NexusGatewayService{}

	tests := []struct {
		name   string
		key    string
		gpuID  string
		expect bool
	}{
		{
			name:   "valid telemetry data key",
			key:    "/telemetry/clusters/test/hosts/node1/gpus/gpu-0/data/2024-01-01T00:00:00Z",
			gpuID:  "gpu-0",
			expect: true,
		},
		{
			name:   "valid telemetry data key with gpu_ prefix",
			key:    "/telemetry/clusters/test/hosts/node1/gpus/gpu_0/data/2024-01-01T00:00:00Z",
			gpuID:  "0",
			expect: true,
		},
		{
			name:   "GPU metadata key (not data)",
			key:    "/telemetry/clusters/test/hosts/node1/gpus/gpu-0",
			gpuID:  "gpu-0",
			expect: false,
		},
		{
			name:   "different GPU ID",
			key:    "/telemetry/clusters/test/hosts/node1/gpus/gpu-1/data/2024-01-01T00:00:00Z",
			gpuID:  "gpu-0",
			expect: false,
		},
		{
			name:   "no data path",
			key:    "/telemetry/clusters/test/hosts/node1/gpus/gpu-0/metadata",
			gpuID:  "gpu-0",
			expect: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := gateway.isValidTelemetryKey(tt.key, tt.gpuID)
			assert.Equal(t, tt.expect, result)
		})
	}
}

func TestNexusGatewayService_matchesGPUID(t *testing.T) {
	gateway := &NexusGatewayService{}

	tests := []struct {
		name        string
		dataGPUID   string
		requestedID string
		expect      bool
	}{
		{
			name:        "exact match",
			dataGPUID:   "gpu-0",
			requestedID: "gpu-0",
			expect:      true,
		},
		{
			name:        "partial match",
			dataGPUID:   "gpu-0-uuid-12345",
			requestedID: "gpu-0",
			expect:      true,
		},
		{
			name:        "no match",
			dataGPUID:   "gpu-1",
			requestedID: "gpu-0",
			expect:      false,
		},
		{
			name:        "empty data GPU ID",
			dataGPUID:   "",
			requestedID: "gpu-0",
			expect:      false,
		},
		{
			name:        "case sensitive",
			dataGPUID:   "GPU-0",
			requestedID: "gpu-0",
			expect:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := gateway.matchesGPUID(tt.dataGPUID, tt.requestedID)
			assert.Equal(t, tt.expect, result)
		})
	}
}

func TestNexusGatewayService_isWithinTimeRange(t *testing.T) {
	gateway := &NexusGatewayService{}

	baseTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	startTime := baseTime.Add(-time.Hour)
	endTime := baseTime.Add(time.Hour)

	tests := []struct {
		name         string
		timestampStr string
		startTime    *time.Time
		endTime      *time.Time
		expect       bool
	}{
		{
			name:         "within range",
			timestampStr: baseTime.Format(time.RFC3339),
			startTime:    &startTime,
			endTime:      &endTime,
			expect:       true,
		},
		{
			name:         "before start time",
			timestampStr: baseTime.Add(-2 * time.Hour).Format(time.RFC3339),
			startTime:    &startTime,
			endTime:      &endTime,
			expect:       false,
		},
		{
			name:         "after end time",
			timestampStr: baseTime.Add(2 * time.Hour).Format(time.RFC3339),
			startTime:    &startTime,
			endTime:      &endTime,
			expect:       false,
		},
		{
			name:         "no time constraints",
			timestampStr: baseTime.Format(time.RFC3339),
			startTime:    nil,
			endTime:      nil,
			expect:       true,
		},
		{
			name:         "only start time constraint",
			timestampStr: baseTime.Format(time.RFC3339),
			startTime:    &startTime,
			endTime:      nil,
			expect:       true,
		},
		{
			name:         "only end time constraint",
			timestampStr: baseTime.Format(time.RFC3339),
			startTime:    nil,
			endTime:      &endTime,
			expect:       true,
		},
		{
			name:         "invalid timestamp format",
			timestampStr: "invalid-timestamp",
			startTime:    &startTime,
			endTime:      &endTime,
			expect:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := gateway.isWithinTimeRange(tt.timestampStr, tt.startTime, tt.endTime)
			assert.Equal(t, tt.expect, result)
		})
	}
}

func TestNexusGatewayService_sortAndLimitTelemetryData(t *testing.T) {
	gateway := &NexusGatewayService{}

	// Create test data with different timestamps
	testData := []TelemetryData{
		{Timestamp: "2024-01-01T10:00:00Z", GPUID: "gpu-0"},
		{Timestamp: "2024-01-01T12:00:00Z", GPUID: "gpu-0"}, // Most recent
		{Timestamp: "2024-01-01T08:00:00Z", GPUID: "gpu-0"}, // Oldest
		{Timestamp: "2024-01-01T11:00:00Z", GPUID: "gpu-0"},
	}

	tests := []struct {
		name           string
		data           []TelemetryData
		limit          int
		expectedCount  int
		expectedFirst  string // timestamp of first element (most recent)
		expectedLast   string // timestamp of last element (oldest or limited)
	}{
		{
			name:          "sort and no limit needed",
			data:          testData,
			limit:         10,
			expectedCount: 4,
			expectedFirst: "2024-01-01T12:00:00Z",
			expectedLast:  "2024-01-01T08:00:00Z",
		},
		{
			name:          "sort and apply limit",
			data:          testData,
			limit:         2,
			expectedCount: 2,
			expectedFirst: "2024-01-01T12:00:00Z",
			expectedLast:  "2024-01-01T11:00:00Z",
		},
		{
			name:          "empty data",
			data:          []TelemetryData{},
			limit:         10,
			expectedCount: 0,
		},
		{
			name: "single item",
			data: []TelemetryData{
				{Timestamp: "2024-01-01T10:00:00Z", GPUID: "gpu-0"},
			},
			limit:          10,
			expectedCount:  1,
			expectedFirst:  "2024-01-01T10:00:00Z",
			expectedLast:   "2024-01-01T10:00:00Z",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := gateway.sortAndLimitTelemetryData(tt.data, tt.limit)
			assert.Equal(t, tt.expectedCount, len(result))

			if tt.expectedCount > 0 {
				assert.Equal(t, tt.expectedFirst, result[0].Timestamp)
				assert.Equal(t, tt.expectedLast, result[len(result)-1].Timestamp)
			}
		})
	}
}

func TestNexusGatewayService_formatValidationError(t *testing.T) {
	gateway := &NexusGatewayService{}

	err := &validation.ValidationError{
		Field:   "test_field",
		Value:   "test_value",
		Message: "test error message",
	}

	result := gateway.formatValidationError(err)

	assert.Equal(t, false, result["success"])
	assert.Contains(t, result["error"], "test_field")
	assert.Contains(t, result["error"], "test error message")
}

func TestNexusGatewayService_queryTelemetryByGPUHandler_Integration(t *testing.T) {
	etcdServer, cleanup := setupTestEtcd(t)
	defer cleanup()

	tests := []struct {
		name           string
		gpuID          string
		queryParams    map[string]string
		setupData      func(*clientv3.Client)
		expectedStatus int
		expectedCount  int
		expectSuccess  bool
	}{
		{
			name:  "successful query with data",
			gpuID: "gpu-0",
			setupData: func(client *clientv3.Client) {
				ctx := context.Background()
				data := TelemetryData{
					Timestamp: time.Now().Format(time.RFC3339),
					GPUID:     "gpu-0",
				}
				jsonData, _ := json.Marshal(data)
				key := "/telemetry/clusters/test/hosts/node1/gpus/gpu-0/data/" + time.Now().Format(time.RFC3339)
				_, _ = client.Put(ctx, key, string(jsonData))
			},
			expectedStatus: http.StatusOK,
			expectedCount:  1,
			expectSuccess:  true,
		},
		{
			name:           "invalid GPU ID",
			gpuID:          "invalid@gpu",
			setupData:      func(client *clientv3.Client) {},
			expectedStatus: http.StatusBadRequest,
			expectSuccess:  false,
		},
		{
			name:  "no matching data",
			gpuID: "gpu-1",
			setupData: func(client *clientv3.Client) {
				ctx := context.Background()
				data := TelemetryData{
					Timestamp: time.Now().Format(time.RFC3339),
					GPUID:     "gpu-0", // Different GPU ID
				}
				jsonData, _ := json.Marshal(data)
				key := "/telemetry/clusters/test/hosts/node1/gpus/gpu-0/data/" + time.Now().Format(time.RFC3339)
				_, _ = client.Put(ctx, key, string(jsonData))
			},
			expectedStatus: http.StatusOK,
			expectedCount:  0,
			expectSuccess:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup etcd data
			tt.setupData(etcdServer.Client)

			// Setup HTTP request
			e := echo.New()
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			
			// Add query parameters
			if tt.queryParams != nil {
				q := req.URL.Query()
				for key, value := range tt.queryParams {
					q.Add(key, value)
				}
				req.URL.RawQuery = q.Encode()
			}
			
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.SetParamNames("id")
			c.SetParamValues(tt.gpuID)

			// Create gateway service with real etcd client
			gateway := &NexusGatewayService{
				etcdClient: etcdServer.Client,
				config: &GatewayConfig{
					ClusterID: "test",
				},
				validator: validation.NewValidator(),
			}

			// Execute
			err := gateway.queryTelemetryByGPUHandler(c)

			// Verify
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, rec.Code)

			// Parse response
			var response map[string]interface{}
			err = json.Unmarshal(rec.Body.Bytes(), &response)
			require.NoError(t, err)

			assert.Equal(t, tt.expectSuccess, response["success"])

			if tt.expectSuccess && tt.expectedCount >= 0 {
				if dataInterface, ok := response["data"]; ok && dataInterface != nil {
					data := dataInterface.([]interface{})
					assert.Equal(t, tt.expectedCount, len(data))
				} else {
					assert.Equal(t, 0, tt.expectedCount, "expected no data but expectedCount > 0")
				}
				assert.Equal(t, float64(tt.expectedCount), response["count"])
			}
		})
	}
}

func TestNexusGatewayService_queryTelemetryByGPUHandler_ServiceUnavailable(t *testing.T) {
	// Setup
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("gpu-0")

	// Create gateway service without etcd client
	gateway := &NexusGatewayService{
		etcdClient: nil,
		config:     nil,
		validator:  validation.NewValidator(),
	}

	// Execute
	err := gateway.queryTelemetryByGPUHandler(c)

	// Verify
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var response map[string]interface{}
	err = json.Unmarshal(rec.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, true, response["success"])
	assert.Equal(t, float64(0), response["count"])
	assert.Equal(t, 0, len(response["data"].([]interface{})))
}

// Benchmark tests
func BenchmarkNexusGatewayService_parseAndValidateTelemetryQuery(b *testing.B) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/?limit=100&start_time=2024-01-01T00:00:00Z", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("gpu-0")

	gateway := &NexusGatewayService{
		validator: validation.NewValidator(),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = gateway.parseAndValidateTelemetryQuery(c)
	}
}

func BenchmarkNexusGatewayService_sortAndLimitTelemetryData(b *testing.B) {
	gateway := &NexusGatewayService{}
	
	// Create test data
	testData := make([]TelemetryData, 1000)
	for i := 0; i < 1000; i++ {
		testData[i] = TelemetryData{
			Timestamp: time.Now().Add(time.Duration(i) * time.Second).Format(time.RFC3339),
			GPUID:     "gpu-0",
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = gateway.sortAndLimitTelemetryData(testData, 100)
	}
}