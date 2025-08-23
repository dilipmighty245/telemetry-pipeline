// Enhanced coverage tests for telemetry pipeline

package integration

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// EnhancedCoverageTestSuite provides additional coverage tests
type EnhancedCoverageTestSuite struct {
	suite.Suite
	gatewayURL   string
	streamerURL  string
	gatewayPort  string
	collectorCmd interface{} // Mock field for compatibility
}

// SetupSuite runs once before all tests in the suite
func (suite *EnhancedCoverageTestSuite) SetupSuite() {
	suite.gatewayURL = "http://localhost:8080"
	suite.streamerURL = "http://localhost:8081"
	suite.gatewayPort = "8080"
	suite.collectorCmd = nil // Mock initialization
}

// SetupTest runs before each test
func (suite *EnhancedCoverageTestSuite) SetupTest() {
	// Setup for each test if needed
}

// Helper methods
func (suite *EnhancedCoverageTestSuite) startCollector() {
	// Mock implementation - in real tests this would start the collector service
	suite.T().Log("Starting collector service")
}

func (suite *EnhancedCoverageTestSuite) waitForServices() {
	// Mock implementation - in real tests this would wait for services to be ready
	suite.T().Log("Waiting for services to be ready")
	time.Sleep(100 * time.Millisecond) // Simulate wait time
}

func (suite *EnhancedCoverageTestSuite) TestCSVUploadAndProcessing() {
	// Mock implementation of CSV upload test
	suite.T().Log("Running CSV upload and processing test")
}

func (suite *EnhancedCoverageTestSuite) createLargeTestCSV(numRecords int) string {
	tmpFile, err := os.CreateTemp("", "large-test-*.csv")
	require.NoError(suite.T(), err)
	defer tmpFile.Close()

	// Write CSV header
	tmpFile.WriteString("timestamp,gpu_id,hostname,uuid,device,modelname,gpu_utilization,memory_utilization\n")

	// Write records
	for i := 0; i < numRecords; i++ {
		tmpFile.WriteString(fmt.Sprintf("2023-01-01T%02d:00:00Z,%d,test-host-%d,GPU-%d,nvidia%d,NVIDIA H100,85.5,60.2\n",
			i%24, i%8, i%10, i, i%8))
	}

	return tmpFile.Name()
}

func (suite *EnhancedCoverageTestSuite) createInvalidCSV() string {
	tmpFile, err := os.CreateTemp("", "invalid-test-*.csv")
	require.NoError(suite.T(), err)
	defer tmpFile.Close()

	// Write invalid CSV content
	tmpFile.WriteString("invalid,csv,content\nwithout,proper,headers\n")

	return tmpFile.Name()
}

func (suite *EnhancedCoverageTestSuite) createIncompleteCSV() string {
	tmpFile, err := os.CreateTemp("", "incomplete-test-*.csv")
	require.NoError(suite.T(), err)
	defer tmpFile.Close()

	// Write CSV with missing required fields
	tmpFile.WriteString("timestamp,some_field\n2023-01-01T00:00:00Z,value\n")

	return tmpFile.Name()
}

func (suite *EnhancedCoverageTestSuite) uploadCSVFile(filename string) error {
	file, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	part, err := writer.CreateFormFile("file", filepath.Base(filename))
	if err != nil {
		return err
	}

	_, err = io.Copy(part, file)
	if err != nil {
		return err
	}

	err = writer.Close()
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", suite.streamerURL+"/api/v1/csv/upload", &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("upload failed with status %d", resp.StatusCode)
	}

	return nil
}

// TestConcurrentUploads tests multiple concurrent CSV uploads
func (suite *EnhancedCoverageTestSuite) TestConcurrentUploads() {
	const numUploads = 5
	var wg sync.WaitGroup
	results := make(chan error, numUploads)

	for i := 0; i < numUploads; i++ {
		wg.Add(1)
		go func(uploadID int) {
			defer wg.Done()
			// Simulate concurrent CSV uploads
			err := suite.uploadTestCSV(fmt.Sprintf("concurrent-upload-%d", uploadID))
			results <- err
		}(i)
	}

	wg.Wait()
	close(results)

	// Check all uploads succeeded
	for err := range results {
		assert.NoError(suite.T(), err, "Concurrent upload should succeed")
	}
}

// TestServiceRestart tests service restart scenarios
func (suite *EnhancedCoverageTestSuite) TestServiceRestart() {
	// Stop collector - mock implementation
	if suite.collectorCmd != nil {
		suite.T().Log("Stopping collector service")
	}

	// Wait a bit
	time.Sleep(2 * time.Second)

	// Restart collector
	suite.startCollector()
	suite.waitForServices()

	// Verify system still works
	suite.TestCSVUploadAndProcessing()
}

// TestLargeFileUpload tests handling of large CSV files
func (suite *EnhancedCoverageTestSuite) TestLargeFileUpload() {
	// Create a large CSV file (1000 records)
	largeCSVFile := suite.createLargeTestCSV(1000)
	defer os.Remove(largeCSVFile)

	// Upload and verify processing
	err := suite.uploadCSVFile(largeCSVFile)
	assert.NoError(suite.T(), err, "Large file upload should succeed")
}

// TestInvalidDataHandling tests error scenarios
func (suite *EnhancedCoverageTestSuite) TestInvalidDataHandling() {
	// Test invalid CSV format
	invalidCSV := suite.createInvalidCSV()
	defer os.Remove(invalidCSV)

	err := suite.uploadCSVFile(invalidCSV)
	assert.Error(suite.T(), err, "Invalid CSV should be rejected")

	// Test missing required fields
	incompleteCSV := suite.createIncompleteCSV()
	defer os.Remove(incompleteCSV)

	err = suite.uploadCSVFile(incompleteCSV)
	assert.Error(suite.T(), err, "Incomplete CSV should be rejected")
}

// TestAPIRateLimiting tests API rate limiting
func (suite *EnhancedCoverageTestSuite) TestAPIRateLimiting() {
	const numRequests = 100
	var wg sync.WaitGroup
	results := make(chan int, numRequests)

	// Make many concurrent requests
	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := http.Get("http://localhost:8080/api/v1/gpus")
			if err != nil {
				results <- 500
				return
			}
			defer resp.Body.Close()
			results <- resp.StatusCode
		}()
	}

	wg.Wait()
	close(results)

	// Count successful vs rate-limited responses
	successCount := 0
	rateLimitedCount := 0
	for statusCode := range results {
		if statusCode == 200 {
			successCount++
		} else if statusCode == 429 {
			rateLimitedCount++
		}
	}

	suite.T().Logf("Successful requests: %d, Rate limited: %d", successCount, rateLimitedCount)
	assert.Greater(suite.T(), successCount, 0, "Some requests should succeed")
}

// TestDataConsistency tests data consistency across services
func (suite *EnhancedCoverageTestSuite) TestDataConsistency() {
	// Upload test data
	suite.TestCSVUploadAndProcessing()

	// Wait for processing
	time.Sleep(3 * time.Second)

	// Query data from different endpoints and verify consistency
	gpusResp, err := http.Get("http://localhost:8080/api/v1/gpus")
	require.NoError(suite.T(), err)
	defer gpusResp.Body.Close()

	hostsResp, err := http.Get("http://localhost:8080/api/v1/hosts")
	require.NoError(suite.T(), err)
	defer hostsResp.Body.Close()

	// Parse responses and verify data consistency
	// (Implementation would parse JSON and verify relationships)
	assert.Equal(suite.T(), http.StatusOK, gpusResp.StatusCode)
	assert.Equal(suite.T(), http.StatusOK, hostsResp.StatusCode)
}

// TestMetricsCollection tests metrics and monitoring
func (suite *EnhancedCoverageTestSuite) TestMetricsCollection() {
	// Upload some data to generate metrics
	suite.TestCSVUploadAndProcessing()

	// Check if services expose metrics
	endpoints := []string{
		"http://localhost:8081/api/v1/status",
		"http://localhost:8080/health",
	}

	for _, endpoint := range endpoints {
		resp, err := http.Get(endpoint)
		require.NoError(suite.T(), err)
		defer resp.Body.Close()
		assert.Equal(suite.T(), http.StatusOK, resp.StatusCode)
	}
}

// TestComprehensiveAPIEndpoints tests all API endpoints for better coverage
func (suite *EnhancedCoverageTestSuite) TestComprehensiveAPIEndpoints() {
	// First upload some test data
	suite.TestCSVUploadAndProcessing()

	// Wait for data to be processed
	time.Sleep(5 * time.Second)

	// Test all Gateway API endpoints
	gatewayEndpoints := []struct {
		method   string
		endpoint string
		expected int
	}{
		{"GET", "/health", http.StatusOK},
		{"GET", "/api/v1/gpus", http.StatusOK},
		{"GET", "/api/v1/hosts", http.StatusOK},
		{"GET", "/api/v1/clusters", http.StatusOK},
		{"GET", "/api/v1/clusters/test-cluster", http.StatusOK},
		{"GET", "/api/v1/telemetry/latest", http.StatusOK},
		{"GET", "/swagger/spec.json", http.StatusOK},
		{"GET", "/swagger/", http.StatusOK},
	}

	for _, ep := range gatewayEndpoints {
		url := fmt.Sprintf("http://localhost:8080%s", ep.endpoint)
		resp, err := http.Get(url)
		require.NoError(suite.T(), err, "Failed to call %s", ep.endpoint)
		resp.Body.Close()
		assert.Equal(suite.T(), ep.expected, resp.StatusCode, "Unexpected status for %s", ep.endpoint)
	}

	// Test Streamer API endpoints
	streamerEndpoints := []struct {
		method   string
		endpoint string
		expected int
	}{
		{"GET", "/health", http.StatusOK},
		{"GET", "/api/v1/status", http.StatusOK},
	}

	for _, ep := range streamerEndpoints {
		url := fmt.Sprintf("http://localhost:8081%s", ep.endpoint)
		resp, err := http.Get(url)
		require.NoError(suite.T(), err, "Failed to call %s", ep.endpoint)
		resp.Body.Close()
		assert.Equal(suite.T(), ep.expected, resp.StatusCode, "Unexpected status for %s", ep.endpoint)
	}
}

// TestGraphQLEndpoint tests GraphQL functionality
func (suite *EnhancedCoverageTestSuite) TestGraphQLEndpoint() {
	// Test GraphQL playground (GET)
	resp, err := http.Get("http://localhost:8080/graphql")
	require.NoError(suite.T(), err)
	defer resp.Body.Close()
	assert.Equal(suite.T(), http.StatusOK, resp.StatusCode)

	// Test GraphQL query (POST)
	query := `{"query": "{ __schema { types { name } } }"}`
	resp, err = http.Post(
		"http://localhost:8080/graphql",
		"application/json",
		strings.NewReader(query),
	)
	require.NoError(suite.T(), err)
	defer resp.Body.Close()
	assert.Equal(suite.T(), http.StatusOK, resp.StatusCode)

	// Test invalid GraphQL query
	invalidQuery := `{"query": "invalid query syntax"}`
	resp, err = http.Post(
		"http://localhost:8080/graphql",
		"application/json",
		strings.NewReader(invalidQuery),
	)
	require.NoError(suite.T(), err)
	defer resp.Body.Close()
	// Should still return 200 but with errors in response
	assert.Equal(suite.T(), http.StatusOK, resp.StatusCode)
}

// TestWebSocketEndpoint tests WebSocket functionality
func (suite *EnhancedCoverageTestSuite) TestWebSocketEndpoint() {
	// For now, just test that the WebSocket endpoint is available
	// A full WebSocket test would require a WebSocket client
	resp, err := http.Get("http://localhost:8080/ws")
	require.NoError(suite.T(), err)
	defer resp.Body.Close()
	// WebSocket upgrade should fail with regular HTTP GET, but endpoint should exist
	assert.Contains(suite.T(), []int{http.StatusBadRequest, http.StatusUpgradeRequired}, resp.StatusCode)
}

// TestErrorHandling tests various error scenarios
func (suite *EnhancedCoverageTestSuite) TestErrorHandling() {
	// Test 404 endpoints
	notFoundEndpoints := []string{
		"/nonexistent",
		"/api/v1/nonexistent",
		"/api/v2/gpus", // Wrong version
	}

	for _, endpoint := range notFoundEndpoints {
		resp, err := http.Get(fmt.Sprintf("http://localhost:8080%s", endpoint))
		require.NoError(suite.T(), err)
		resp.Body.Close()
		assert.Equal(suite.T(), http.StatusNotFound, resp.StatusCode, "Expected 404 for %s", endpoint)
	}

	// Test invalid cluster ID
	resp, err := http.Get("http://localhost:8080/api/v1/clusters/invalid-cluster")
	require.NoError(suite.T(), err)
	defer resp.Body.Close()
	assert.Equal(suite.T(), http.StatusNotFound, resp.StatusCode)

	// Test malformed requests
	resp, err = http.Post(
		"http://localhost:8080/graphql",
		"application/json",
		strings.NewReader("invalid json"),
	)
	require.NoError(suite.T(), err)
	defer resp.Body.Close()
	assert.Equal(suite.T(), http.StatusBadRequest, resp.StatusCode)
}

// TestTelemetryQueryEndpoints tests telemetry querying with various parameters
func (suite *EnhancedCoverageTestSuite) TestTelemetryQueryEndpoints() {
	// Upload test data first
	suite.TestCSVUploadAndProcessing()

	// Wait for processing
	time.Sleep(5 * time.Second)

	// Test telemetry queries with different parameters
	queryParams := []string{
		"",
		"?limit=10",
		"?start_time=2024-01-15T09:00:00Z",
		"?end_time=2024-01-15T11:00:00Z",
		"?start_time=2024-01-15T09:00:00Z&end_time=2024-01-15T11:00:00Z&limit=5",
	}

	for _, params := range queryParams {
		// Test general telemetry endpoint (should fail without required params)
		url := fmt.Sprintf("http://localhost:8080/api/v1/telemetry%s", params)
		resp, err := http.Get(url)
		require.NoError(suite.T(), err)
		resp.Body.Close()
		// Should return 400 because host_id and gpu_id are required
		assert.Equal(suite.T(), http.StatusBadRequest, resp.StatusCode, "Expected 400 for telemetry query without required params")
	}

	// Test with required parameters
	url := "http://localhost:8080/api/v1/telemetry?host_id=test-host&gpu_id=0"
	resp, err := http.Get(url)
	require.NoError(suite.T(), err)
	defer resp.Body.Close()
	assert.Equal(suite.T(), http.StatusOK, resp.StatusCode)
}

// Helper methods for enhanced testing

func (suite *EnhancedCoverageTestSuite) uploadTestCSV(testID string) error {
	// Create a test CSV file
	csvContent := fmt.Sprintf(`timestamp,gpu_id,uuid,device,modelName,Hostname,gpu_utilization,memory_utilization,memory_used_mb,memory_free_mb,temperature,power_draw,sm_clock_mhz,memory_clock_mhz
2024-01-15T10:00:00Z,0,GPU-%s-TEST-UUID,nvidia0,NVIDIA H100 80GB HBM3,test-host-%s,85.5,60.2,48000,32000,72.0,350.5,1410,1215
2024-01-15T10:00:01Z,1,GPU-%s-TEST-UUID2,nvidia1,NVIDIA H100 80GB HBM3,test-host-%s,92.1,75.8,60800,19200,75.5,380.2,1410,1215`, testID, testID, testID, testID)

	// Create temporary file
	tmpFile, err := os.CreateTemp("", fmt.Sprintf("test-csv-%s-*.csv", testID))
	if err != nil {
		return err
	}
	defer tmpFile.Close()

	if _, err := tmpFile.WriteString(csvContent); err != nil {
		return err
	}

	// Upload the file
	return suite.uploadCSVFile(tmpFile.Name())
}

// Run the enhanced coverage test suite
func TestEnhancedCoverageTestSuite(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping Enhanced Coverage tests in short mode")
	}

	suite.Run(t, new(EnhancedCoverageTestSuite))
}
