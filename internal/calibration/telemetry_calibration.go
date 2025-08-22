package calibration

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/dilipmighty245/telemetry-pipeline/internal/nexus"
	"github.com/labstack/echo/v4"
	log "github.com/sirupsen/logrus"
)

// CalibrationService provides performance testing and calibration for telemetry pipeline
type CalibrationService struct {
	nexusService *nexus.TelemetryService
	testResults  map[string]*TestResult
	mu           sync.RWMutex
}

// TestConfig defines a calibration test scenario
type TestConfig struct {
	Name        string   `json:"name"`
	Concurrency int      `json:"concurrency"`
	OpsCount    int      `json:"ops_count"`
	SampleRate  float32  `json:"sample_rate"`
	GraphQL     []string `json:"graphql"` // List of GraphQL queries to run
	Rest        []string `json:"rest"`    // List of REST endpoints to test
}

// TestResult stores the results of a calibration test
type TestResult struct {
	Name        string    `json:"name"`
	StartTime   time.Time `json:"start_time"`
	EndTime     time.Time `json:"end_time"`
	Duration    string    `json:"duration"`
	TotalOps    int       `json:"total_ops"`
	Concurrency int       `json:"concurrency"`
	OpsPerSec   float64   `json:"ops_per_sec"`
	Success     int       `json:"success"`
	Errors      int       `json:"errors"`
	AvgLatency  string    `json:"avg_latency"`
	Status      string    `json:"status"`
}

// CalibrationRequest defines a calibration run request
type CalibrationRequest struct {
	Tests []TestConfig `json:"tests"`
}

// NewCalibrationService creates a new calibration service
func NewCalibrationService(nexusService *nexus.TelemetryService) *CalibrationService {
	return &CalibrationService{
		nexusService: nexusService,
		testResults:  make(map[string]*TestResult),
	}
}

// RunTest executes a single calibration test
func (cs *CalibrationService) RunTest(config TestConfig) (*TestResult, error) {
	log.Infof("Starting calibration test: %s", config.Name)

	result := &TestResult{
		Name:        config.Name,
		StartTime:   time.Now(),
		Concurrency: config.Concurrency,
		TotalOps:    config.Concurrency * config.OpsCount,
		Status:      "running",
	}

	cs.mu.Lock()
	cs.testResults[config.Name] = result
	cs.mu.Unlock()

	var wg sync.WaitGroup
	var mu sync.Mutex
	successCount := 0
	errorCount := 0
	var totalLatency time.Duration

	// Run concurrent workers
	for i := 0; i < config.Concurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			for j := 0; j < config.OpsCount; j++ {
				startTime := time.Now()

				// Execute GraphQL tests
				for _, queryName := range config.GraphQL {
					err := cs.executeGraphQLTest(queryName)
					mu.Lock()
					if err != nil {
						errorCount++
						log.Debugf("GraphQL test %s failed: %v", queryName, err)
					} else {
						successCount++
					}
					mu.Unlock()
				}

				// Execute REST tests
				for _, restEndpoint := range config.Rest {
					err := cs.executeRestTest(restEndpoint)
					mu.Lock()
					if err != nil {
						errorCount++
						log.Debugf("REST test %s failed: %v", restEndpoint, err)
					} else {
						successCount++
					}
					mu.Unlock()
				}

				latency := time.Since(startTime)
				mu.Lock()
				totalLatency += latency
				mu.Unlock()

				// Apply sample rate delay
				if config.SampleRate > 0 {
					time.Sleep(time.Duration(1000/config.SampleRate) * time.Millisecond)
				}
			}
		}(i)
	}

	wg.Wait()

	// Calculate results
	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime).String()
	result.Success = successCount
	result.Errors = errorCount
	result.OpsPerSec = float64(successCount) / result.EndTime.Sub(result.StartTime).Seconds()
	if successCount > 0 {
		result.AvgLatency = (totalLatency / time.Duration(successCount)).String()
	}
	result.Status = "completed"

	cs.mu.Lock()
	cs.testResults[config.Name] = result
	cs.mu.Unlock()

	log.Infof("Calibration test %s completed: %d success, %d errors, %.2f ops/sec",
		config.Name, successCount, errorCount, result.OpsPerSec)

	return result, nil
}

// executeGraphQLTest executes a GraphQL test query
func (cs *CalibrationService) executeGraphQLTest(queryName string) error {
	// Define some common GraphQL queries for testing
	queries := map[string]string{
		"get_clusters": `
			query {
				clusters {
					id
					name
					totalHosts
					totalGPUs
				}
			}
		`,
		"get_gpus": `
			query {
				gpus {
					id
					uuid
					hostname
					modelName
					status
				}
			}
		`,
		"get_telemetry": `
			query {
				telemetry(limit: 10) {
					timestamp
					hostname
					gpuId
					gpuUtilization
					memoryUtilization
					temperature
				}
			}
		`,
		"get_dashboard": `
			query {
				dashboard {
					timestamp
					hostname
					gpuId
					gpuUtilization
					temperature
					powerDraw
				}
			}
		`,
	}

	query, exists := queries[queryName]
	if !exists {
		return fmt.Errorf("unknown GraphQL query: %s", queryName)
	}

	// Simulate GraphQL execution (in real implementation, this would call the GraphQL service)
	_ = query

	// Add small delay to simulate processing
	time.Sleep(time.Duration(10+len(query)/100) * time.Millisecond)

	return nil
}

// executeRestTest executes a REST endpoint test
func (cs *CalibrationService) executeRestTest(endpoint string) error {
	// Define REST endpoints for testing
	endpoints := map[string]string{
		"health":           "/health",
		"list_gpus":        "/api/v1/gpus",
		"list_clusters":    "/api/v1/clusters",
		"list_hosts":       "/api/v1/hosts",
		"get_telemetry":    "/api/v1/telemetry?host_id=test&gpu_id=0&limit=10",
		"latest_telemetry": "/api/v1/telemetry/latest",
	}

	path, exists := endpoints[endpoint]
	if !exists {
		return fmt.Errorf("unknown REST endpoint: %s", endpoint)
	}

	// Simulate REST API call (in real implementation, this would make HTTP requests)
	_ = path

	// Add small delay to simulate network and processing
	time.Sleep(time.Duration(50+len(path)) * time.Millisecond)

	return nil
}

// GetTestResult returns the result of a specific test
func (cs *CalibrationService) GetTestResult(testName string) (*TestResult, bool) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	result, exists := cs.testResults[testName]
	return result, exists
}

// GetAllTestResults returns all test results
func (cs *CalibrationService) GetAllTestResults() map[string]*TestResult {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	results := make(map[string]*TestResult)
	for k, v := range cs.testResults {
		results[k] = v
	}
	return results
}

// SetupCalibrationRoutes adds calibration endpoints to the API Gateway
func (cs *CalibrationService) SetupCalibrationRoutes(e *echo.Echo) {
	calibration := e.Group("/api/v1/calibration")

	// Start a calibration test
	calibration.POST("/tests", cs.startTestHandler)

	// Get test result
	calibration.GET("/tests/:testName", cs.getTestResultHandler)

	// List all test results
	calibration.GET("/tests", cs.listTestResultsHandler)

	// Get predefined test configurations
	calibration.GET("/configs", cs.getTestConfigsHandler)
}

// HTTP Handlers

func (cs *CalibrationService) startTestHandler(c echo.Context) error {
	var req CalibrationRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Invalid request format",
		})
	}

	if len(req.Tests) == 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "No tests specified",
		})
	}

	// Run tests asynchronously
	go func() {
		for _, test := range req.Tests {
			if _, err := cs.RunTest(test); err != nil {
				log.Errorf("Test %s failed: %v", test.Name, err)
			}
		}
	}()

	return c.JSON(http.StatusAccepted, map[string]interface{}{
		"message": "Calibration tests started",
		"tests":   len(req.Tests),
	})
}

func (cs *CalibrationService) getTestResultHandler(c echo.Context) error {
	testName := c.Param("testName")

	result, exists := cs.GetTestResult(testName)
	if !exists {
		return c.JSON(http.StatusNotFound, map[string]string{
			"error": "Test not found",
		})
	}

	return c.JSON(http.StatusOK, result)
}

func (cs *CalibrationService) listTestResultsHandler(c echo.Context) error {
	results := cs.GetAllTestResults()

	return c.JSON(http.StatusOK, map[string]interface{}{
		"results": results,
		"count":   len(results),
	})
}

func (cs *CalibrationService) getTestConfigsHandler(c echo.Context) error {
	// Return predefined test configurations
	configs := []TestConfig{
		{
			Name:        "basic_load_test",
			Concurrency: 5,
			OpsCount:    20,
			SampleRate:  1.0,
			GraphQL:     []string{"get_clusters", "get_gpus"},
			Rest:        []string{"health", "list_gpus"},
		},
		{
			Name:        "high_load_test",
			Concurrency: 20,
			OpsCount:    50,
			SampleRate:  2.0,
			GraphQL:     []string{"get_telemetry", "get_dashboard"},
			Rest:        []string{"get_telemetry", "latest_telemetry"},
		},
		{
			Name:        "stress_test",
			Concurrency: 50,
			OpsCount:    100,
			SampleRate:  5.0,
			GraphQL:     []string{"get_clusters", "get_gpus", "get_telemetry", "get_dashboard"},
			Rest:        []string{"health", "list_gpus", "get_telemetry", "latest_telemetry"},
		},
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"configs": configs,
		"count":   len(configs),
	})
}
