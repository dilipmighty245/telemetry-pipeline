package calibration

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dilipmighty245/telemetry-pipeline/internal/nexus"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockNexusService is a mock implementation of nexus.TelemetryService
type MockNexusService struct {
	mock.Mock
}

func (m *MockNexusService) Start() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockNexusService) Stop() error {
	args := m.Called()
	return args.Error(0)
}

func TestNewCalibrationService(t *testing.T) {
	mockNexus := &MockNexusService{}

	service := NewCalibrationService(&nexus.TelemetryService{})

	assert.NotNil(t, service)
	assert.NotNil(t, service.testResults)
	assert.Equal(t, mockNexus, service.nexusService)
}

func TestCalibrationService_RunTest(t *testing.T) {
	tests := []struct {
		name     string
		config   TestConfig
		wantErr  bool
		validate func(*testing.T, *TestResult)
	}{
		{
			name: "basic test with GraphQL queries",
			config: TestConfig{
				Name:        "test1",
				Concurrency: 2,
				OpsCount:    3,
				SampleRate:  0.1,
				GraphQL:     []string{"get_clusters", "get_gpus"},
				Rest:        []string{},
			},
			wantErr: false,
			validate: func(t *testing.T, result *TestResult) {
				assert.Equal(t, "test1", result.Name)
				assert.Equal(t, 2, result.Concurrency)
				assert.Equal(t, 6, result.TotalOps) // 2 * 3
				assert.Equal(t, "completed", result.Status)
				assert.True(t, result.Success > 0)
				assert.True(t, result.OpsPerSec > 0)
			},
		},
		{
			name: "test with REST endpoints",
			config: TestConfig{
				Name:        "test2",
				Concurrency: 1,
				OpsCount:    2,
				SampleRate:  0.0,
				GraphQL:     []string{},
				Rest:        []string{"health", "list_gpus"},
			},
			wantErr: false,
			validate: func(t *testing.T, result *TestResult) {
				assert.Equal(t, "test2", result.Name)
				assert.Equal(t, 1, result.Concurrency)
				assert.Equal(t, 2, result.TotalOps)
				assert.Equal(t, "completed", result.Status)
			},
		},
		{
			name: "test with mixed queries",
			config: TestConfig{
				Name:        "test3",
				Concurrency: 3,
				OpsCount:    1,
				SampleRate:  0.0,
				GraphQL:     []string{"get_telemetry"},
				Rest:        []string{"health"},
			},
			wantErr: false,
			validate: func(t *testing.T, result *TestResult) {
				assert.Equal(t, "test3", result.Name)
				assert.Equal(t, 3, result.Concurrency)
				assert.Equal(t, 3, result.TotalOps)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := NewCalibrationService(&nexus.TelemetryService{})

			result, err := service.RunTest(tt.config)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				tt.validate(t, result)
			}
		})
	}
}

func TestCalibrationService_ExecuteGraphQLTest(t *testing.T) {
	tests := []struct {
		name      string
		queryName string
		wantErr   bool
	}{
		{
			name:      "valid GraphQL query - get_clusters",
			queryName: "get_clusters",
			wantErr:   false,
		},
		{
			name:      "valid GraphQL query - get_gpus",
			queryName: "get_gpus",
			wantErr:   false,
		},
		{
			name:      "valid GraphQL query - get_telemetry",
			queryName: "get_telemetry",
			wantErr:   false,
		},
		{
			name:      "valid GraphQL query - get_dashboard",
			queryName: "get_dashboard",
			wantErr:   false,
		},
		{
			name:      "invalid GraphQL query",
			queryName: "invalid_query",
			wantErr:   true,
		},
	}

	service := NewCalibrationService(&nexus.TelemetryService{})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := service.executeGraphQLTest(tt.queryName)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "unknown GraphQL query")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestCalibrationService_ExecuteRestTest(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
		wantErr  bool
	}{
		{
			name:     "valid REST endpoint - health",
			endpoint: "health",
			wantErr:  false,
		},
		{
			name:     "valid REST endpoint - list_gpus",
			endpoint: "list_gpus",
			wantErr:  false,
		},
		{
			name:     "valid REST endpoint - list_clusters",
			endpoint: "list_clusters",
			wantErr:  false,
		},
		{
			name:     "valid REST endpoint - get_telemetry",
			endpoint: "get_telemetry",
			wantErr:  false,
		},
		{
			name:     "invalid REST endpoint",
			endpoint: "invalid_endpoint",
			wantErr:  true,
		},
	}

	service := NewCalibrationService(&nexus.TelemetryService{})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := service.executeRestTest(tt.endpoint)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "unknown REST endpoint")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestCalibrationService_GetTestResult(t *testing.T) {
	service := NewCalibrationService(&nexus.TelemetryService{})

	// Test getting non-existent result
	result, exists := service.GetTestResult("non-existent")
	assert.False(t, exists)
	assert.Nil(t, result)

	// Add a test result
	testResult := &TestResult{
		Name:   "test1",
		Status: "completed",
	}
	service.testResults["test1"] = testResult

	// Test getting existing result
	result, exists = service.GetTestResult("test1")
	assert.True(t, exists)
	assert.NotNil(t, result)
	assert.Equal(t, "test1", result.Name)
	assert.Equal(t, "completed", result.Status)
}

func TestCalibrationService_GetAllTestResults(t *testing.T) {
	service := NewCalibrationService(&nexus.TelemetryService{})

	// Test empty results
	results := service.GetAllTestResults()
	assert.NotNil(t, results)
	assert.Len(t, results, 0)

	// Add test results
	testResult1 := &TestResult{Name: "test1", Status: "completed"}
	testResult2 := &TestResult{Name: "test2", Status: "running"}

	service.testResults["test1"] = testResult1
	service.testResults["test2"] = testResult2

	// Test getting all results
	results = service.GetAllTestResults()
	assert.Len(t, results, 2)
	assert.Contains(t, results, "test1")
	assert.Contains(t, results, "test2")
	assert.Equal(t, "completed", results["test1"].Status)
	assert.Equal(t, "running", results["test2"].Status)
}

func TestCalibrationService_ConcurrentAccess(t *testing.T) {
	service := NewCalibrationService(&nexus.TelemetryService{})

	var wg sync.WaitGroup
	numGoroutines := 10

	// Test concurrent writes
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			config := TestConfig{
				Name:        string(rune('A' + id)),
				Concurrency: 1,
				OpsCount:    1,
				GraphQL:     []string{"get_clusters"},
			}
			_, err := service.RunTest(config)
			assert.NoError(t, err)
		}(i)
	}

	// Test concurrent reads
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results := service.GetAllTestResults()
			assert.NotNil(t, results)
		}()
	}

	wg.Wait()

	// Verify all tests were stored
	results := service.GetAllTestResults()
	assert.Len(t, results, numGoroutines)
}

// HTTP Handler Tests

func TestCalibrationService_StartTestHandler(t *testing.T) {
	tests := []struct {
		name           string
		requestBody    string
		expectedStatus int
		expectedError  string
	}{
		{
			name: "valid request with single test",
			requestBody: `{
				"tests": [
					{
						"name": "test1",
						"concurrency": 2,
						"ops_count": 5,
						"sample_rate": 1.0,
						"graphql": ["get_clusters"],
						"rest": ["health"]
					}
				]
			}`,
			expectedStatus: http.StatusAccepted,
		},
		{
			name: "valid request with multiple tests",
			requestBody: `{
				"tests": [
					{
						"name": "test1",
						"concurrency": 1,
						"ops_count": 2,
						"graphql": ["get_gpus"]
					},
					{
						"name": "test2",
						"concurrency": 3,
						"ops_count": 1,
						"rest": ["list_gpus"]
					}
				]
			}`,
			expectedStatus: http.StatusAccepted,
		},
		{
			name:           "invalid JSON",
			requestBody:    `{"invalid": json}`,
			expectedStatus: http.StatusBadRequest,
			expectedError:  "Invalid request format",
		},
		{
			name:           "empty tests array",
			requestBody:    `{"tests": []}`,
			expectedStatus: http.StatusBadRequest,
			expectedError:  "No tests specified",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := NewCalibrationService(&nexus.TelemetryService{})

			e := echo.New()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/calibration/tests", strings.NewReader(tt.requestBody))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			err := service.startTestHandler(c)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, rec.Code)

			if tt.expectedError != "" {
				var response map[string]string
				err := json.Unmarshal(rec.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.Contains(t, response["error"], tt.expectedError)
			}
		})
	}
}

func TestCalibrationService_GetTestResultHandler(t *testing.T) {
	service := NewCalibrationService(&nexus.TelemetryService{})

	// Add a test result
	testResult := &TestResult{
		Name:   "test1",
		Status: "completed",
	}
	service.testResults["test1"] = testResult

	tests := []struct {
		name           string
		testName       string
		expectedStatus int
		expectedError  string
	}{
		{
			name:           "existing test",
			testName:       "test1",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "non-existent test",
			testName:       "non-existent",
			expectedStatus: http.StatusNotFound,
			expectedError:  "Test not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := echo.New()
			req := httptest.NewRequest(http.MethodGet, "/api/v1/calibration/tests/"+tt.testName, nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.SetParamNames("testName")
			c.SetParamValues(tt.testName)

			err := service.getTestResultHandler(c)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, rec.Code)

			if tt.expectedError != "" {
				var response map[string]string
				err := json.Unmarshal(rec.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.Contains(t, response["error"], tt.expectedError)
			} else if tt.expectedStatus == http.StatusOK {
				var response TestResult
				err := json.Unmarshal(rec.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.Equal(t, "test1", response.Name)
			}
		})
	}
}

func TestCalibrationService_ListTestResultsHandler(t *testing.T) {
	service := NewCalibrationService(&nexus.TelemetryService{})

	// Add test results
	service.testResults["test1"] = &TestResult{Name: "test1", Status: "completed"}
	service.testResults["test2"] = &TestResult{Name: "test2", Status: "running"}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/calibration/tests", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := service.listTestResultsHandler(c)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var response map[string]interface{}
	err = json.Unmarshal(rec.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, float64(2), response["count"])
	assert.NotNil(t, response["results"])
}

func TestCalibrationService_GetTestConfigsHandler(t *testing.T) {
	service := NewCalibrationService(&nexus.TelemetryService{})

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/calibration/configs", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := service.getTestConfigsHandler(c)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var response map[string]interface{}
	err = json.Unmarshal(rec.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, float64(3), response["count"]) // 3 predefined configs
	assert.NotNil(t, response["configs"])

	// Verify configs structure
	configs, ok := response["configs"].([]interface{})
	assert.True(t, ok)
	assert.Len(t, configs, 3)

	// Check first config
	firstConfig := configs[0].(map[string]interface{})
	assert.Equal(t, "basic_load_test", firstConfig["name"])
	assert.Equal(t, float64(5), firstConfig["concurrency"])
}

func TestCalibrationService_SetupCalibrationRoutes(t *testing.T) {
	service := NewCalibrationService(&nexus.TelemetryService{})

	e := echo.New()
	service.SetupCalibrationRoutes(e)

	// Test that routes are registered (this is a basic test)
	routes := e.Routes()
	assert.NotEmpty(t, routes)

	// Find calibration routes
	calibrationRoutes := 0
	for _, route := range routes {
		if strings.Contains(route.Path, "/api/v1/calibration") {
			calibrationRoutes++
		}
	}
	assert.Equal(t, 4, calibrationRoutes) // POST tests, GET tests/:testName, GET tests, GET configs
}

func TestTestResult_Duration(t *testing.T) {
	start := time.Now()
	end := start.Add(5 * time.Second)

	result := &TestResult{
		StartTime: start,
		EndTime:   end,
		Duration:  end.Sub(start).String(),
	}

	assert.Equal(t, "5s", result.Duration)
}

func TestTestConfig_Validation(t *testing.T) {
	config := TestConfig{
		Name:        "test",
		Concurrency: 5,
		OpsCount:    10,
		SampleRate:  2.0,
		GraphQL:     []string{"get_clusters", "get_gpus"},
		Rest:        []string{"health", "list_gpus"},
	}

	assert.Equal(t, "test", config.Name)
	assert.Equal(t, 5, config.Concurrency)
	assert.Equal(t, 10, config.OpsCount)
	assert.Equal(t, float32(2.0), config.SampleRate)
	assert.Len(t, config.GraphQL, 2)
	assert.Len(t, config.Rest, 2)
}
