//go:build integration
// +build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	clientv3 "go.etcd.io/etcd/client/v3"
)

// E2ETestSuite contains all end-to-end tests for the telemetry pipeline
type E2ETestSuite struct {
	suite.Suite
	etcdClient      *clientv3.Client
	streamerCmd     *exec.Cmd
	collectorCmd    *exec.Cmd
	gatewayCmd      *exec.Cmd
	testDataDir     string
	streamerPort    int
	gatewayPort     int
	etcdEndpoints   []string
	clusterID       string
	testCSVFile     string
	dockerComposeUp bool
}

// SetupSuite runs before all tests in the suite
func (suite *E2ETestSuite) SetupSuite() {
	suite.streamerPort = 8081
	suite.gatewayPort = 8080
	suite.etcdEndpoints = []string{"localhost:12379"}
	suite.clusterID = "test-cluster"

	// Create test data directory
	var err error
	suite.testDataDir, err = os.MkdirTemp("", "telemetry-e2e-test-*")
	require.NoError(suite.T(), err)

	// Start etcd using docker-compose
	suite.startEtcd()

	// Wait for etcd to be ready
	suite.waitForEtcd()

	// Create test CSV file
	suite.createTestCSVFile()

	// Start telemetry services
	suite.startTelemetryServices()

	// Wait for services to be ready
	suite.waitForServices()
}

// TearDownSuite runs after all tests in the suite
func (suite *E2ETestSuite) TearDownSuite() {
	// Stop telemetry services
	suite.stopTelemetryServices()

	// Stop etcd
	suite.stopEtcd()

	// Clean up test data
	if suite.testDataDir != "" {
		os.RemoveAll(suite.testDataDir)
	}

	// Close etcd client
	if suite.etcdClient != nil {
		suite.etcdClient.Close()
	}
}

// startEtcd starts etcd using docker-compose
func (suite *E2ETestSuite) startEtcd() {
	// Get the directory containing the docker-compose file
	testDir := filepath.Dir(suite.getTestFilePath())
	composeFile := filepath.Join(testDir, "docker-compose.test.yml")

	// Stop any existing containers
	stopCmd := exec.Command("docker-compose", "-f", composeFile, "down", "-v")
	stopCmd.Run() // Ignore errors

	// Start etcd
	cmd := exec.Command("docker-compose", "-f", composeFile, "up", "-d", "etcd")
	cmd.Dir = testDir
	output, err := cmd.CombinedOutput()
	require.NoError(suite.T(), err, "Failed to start etcd: %s", string(output))

	suite.dockerComposeUp = true
	suite.T().Log("Started etcd container")
}

// stopEtcd stops etcd using docker-compose
func (suite *E2ETestSuite) stopEtcd() {
	if !suite.dockerComposeUp {
		return
	}

	testDir := filepath.Dir(suite.getTestFilePath())
	composeFile := filepath.Join(testDir, "docker-compose.test.yml")

	cmd := exec.Command("docker-compose", "-f", composeFile, "down", "-v")
	cmd.Dir = testDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		suite.T().Logf("Warning: Failed to stop etcd: %s", string(output))
	} else {
		suite.T().Log("Stopped etcd container")
	}
}

// waitForEtcd waits for etcd to be ready
func (suite *E2ETestSuite) waitForEtcd() {
	suite.T().Log("Waiting for etcd to be ready...")

	// Wait up to 60 seconds for etcd to be ready
	timeout := time.After(60 * time.Second)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			suite.T().Fatal("Timeout waiting for etcd to be ready")
		case <-ticker.C:
			// Try to connect to etcd
			client, err := clientv3.New(clientv3.Config{
				Endpoints:   suite.etcdEndpoints,
				DialTimeout: 5 * time.Second,
			})
			if err != nil {
				continue
			}

			// Test connection
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			_, err = client.Status(ctx, suite.etcdEndpoints[0])
			cancel()

			if err == nil {
				suite.etcdClient = client
				suite.T().Log("etcd is ready")
				return
			}
			client.Close()
		}
	}
}

// createTestCSVFile creates a test CSV file with sample telemetry data
func (suite *E2ETestSuite) createTestCSVFile() {
	csvContent := `timestamp,gpu_id,uuid,device,modelName,Hostname,gpu_utilization,memory_utilization,memory_used_mb,memory_free_mb,temperature,power_draw,sm_clock_mhz,memory_clock_mhz
2024-01-15T10:00:00Z,0,GPU-12345-67890-ABCDE,nvidia0,NVIDIA H100 80GB HBM3,test-host-1,85.5,60.2,48000,32000,72.0,350.5,1410,1215
2024-01-15T10:00:01Z,1,GPU-12345-67890-ABCDF,nvidia1,NVIDIA H100 80GB HBM3,test-host-1,92.1,75.8,60800,19200,75.5,380.2,1410,1215
2024-01-15T10:00:02Z,0,GPU-FEDCB-09876-54321,nvidia0,NVIDIA H100 80GB HBM3,test-host-2,78.3,55.1,44080,35920,70.2,340.8,1410,1215
2024-01-15T10:00:03Z,1,GPU-FEDCB-09876-54322,nvidia1,NVIDIA H100 80GB HBM3,test-host-2,88.7,68.9,55120,24880,73.8,365.1,1410,1215
2024-01-15T10:00:04Z,2,GPU-ABCDE-12345-FGHIJ,nvidia2,NVIDIA H100 80GB HBM3,test-host-1,95.2,82.4,65920,14080,77.1,395.6,1410,1215
2024-01-15T10:00:05Z,0,GPU-12345-67890-ABCDE,nvidia0,NVIDIA H100 80GB HBM3,test-host-1,87.8,62.5,50000,30000,72.8,355.3,1410,1215
2024-01-15T10:00:06Z,1,GPU-12345-67890-ABCDF,nvidia1,NVIDIA H100 80GB HBM3,test-host-1,90.4,73.2,58560,21440,74.9,375.8,1410,1215
2024-01-15T10:00:07Z,0,GPU-FEDCB-09876-54321,nvidia0,NVIDIA H100 80GB HBM3,test-host-2,80.1,57.6,46080,33920,71.0,345.2,1410,1215
2024-01-15T10:00:08Z,1,GPU-FEDCB-09876-54322,nvidia1,NVIDIA H100 80GB HBM3,test-host-2,86.9,66.8,53440,26560,73.2,362.4,1410,1215
2024-01-15T10:00:09Z,2,GPU-ABCDE-12345-FGHIJ,nvidia2,NVIDIA H100 80GB HBM3,test-host-1,93.5,80.1,64080,15920,76.5,390.9,1410,1215`

	suite.testCSVFile = filepath.Join(suite.testDataDir, "test_telemetry.csv")
	err := os.WriteFile(suite.testCSVFile, []byte(csvContent), 0644)
	require.NoError(suite.T(), err)

	suite.T().Logf("Created test CSV file: %s", suite.testCSVFile)
}

// startTelemetryServices starts all telemetry services
func (suite *E2ETestSuite) startTelemetryServices() {
	// Build the services first
	suite.buildServices()

	// Start streamer
	suite.startStreamer()

	// Start collector
	suite.startCollector()

	// Start gateway
	suite.startGateway()
}

// buildServices builds all the telemetry services
func (suite *E2ETestSuite) buildServices() {
	suite.T().Log("Building telemetry services...")

	projectRoot := suite.getProjectRoot()

	// Build streamer
	cmd := exec.Command("go", "build", "-o", filepath.Join(suite.testDataDir, "nexus-streamer"), "./cmd/nexus-streamer")
	cmd.Dir = projectRoot
	output, err := cmd.CombinedOutput()
	require.NoError(suite.T(), err, "Failed to build streamer: %s", string(output))

	// Build collector
	cmd = exec.Command("go", "build", "-o", filepath.Join(suite.testDataDir, "nexus-collector"), "./cmd/nexus-collector")
	cmd.Dir = projectRoot
	output, err = cmd.CombinedOutput()
	require.NoError(suite.T(), err, "Failed to build collector: %s", string(output))

	// Build gateway
	cmd = exec.Command("go", "build", "-o", filepath.Join(suite.testDataDir, "nexus-gateway"), "./cmd/nexus-gateway")
	cmd.Dir = projectRoot
	output, err = cmd.CombinedOutput()
	require.NoError(suite.T(), err, "Failed to build gateway: %s", string(output))

	suite.T().Log("Successfully built all services")
}

// startStreamer starts the nexus-streamer service
func (suite *E2ETestSuite) startStreamer() {
	streamerBinary := filepath.Join(suite.testDataDir, "nexus-streamer")

	suite.streamerCmd = exec.Command(streamerBinary,
		"-cluster-id", suite.clusterID,
		"-streamer-id", "test-streamer-1",
		"-http-port", fmt.Sprintf("%d", suite.streamerPort),
		"-batch-size", "5",
		"-stream-interval", "1s",
		"-log-level", "info",
	)

	suite.streamerCmd.Env = append(os.Environ(),
		fmt.Sprintf("ETCD_ENDPOINTS=%s", strings.Join(suite.etcdEndpoints, ",")),
		"UPLOAD_DIR="+filepath.Join(suite.testDataDir, "uploads"),
	)

	// Capture output for debugging
	suite.streamerCmd.Stdout = os.Stdout
	suite.streamerCmd.Stderr = os.Stderr

	err := suite.streamerCmd.Start()
	require.NoError(suite.T(), err, "Failed to start streamer")

	suite.T().Logf("Started streamer on port %d", suite.streamerPort)
}

// startCollector starts the nexus-collector service
func (suite *E2ETestSuite) startCollector() {
	collectorBinary := filepath.Join(suite.testDataDir, "nexus-collector")

	suite.collectorCmd = exec.Command(collectorBinary,
		"-cluster-id", suite.clusterID,
		"-collector-id", "test-collector-1",
		"-batch-size", "5",
		"-poll-interval", "1s",
		"-workers", "2",
		"-log-level", "info",
	)

	suite.collectorCmd.Env = append(os.Environ(),
		fmt.Sprintf("ETCD_ENDPOINTS=%s", strings.Join(suite.etcdEndpoints, ",")),
	)

	// Capture output for debugging
	suite.collectorCmd.Stdout = os.Stdout
	suite.collectorCmd.Stderr = os.Stderr

	err := suite.collectorCmd.Start()
	require.NoError(suite.T(), err, "Failed to start collector")

	suite.T().Log("Started collector")
}

// startGateway starts the nexus-gateway service
func (suite *E2ETestSuite) startGateway() {
	gatewayBinary := filepath.Join(suite.testDataDir, "nexus-gateway")

	suite.gatewayCmd = exec.Command(gatewayBinary,
		"-port", fmt.Sprintf("%d", suite.gatewayPort),
		"-cluster-id", suite.clusterID,
		"-log-level", "info",
	)

	suite.gatewayCmd.Env = append(os.Environ(),
		fmt.Sprintf("ETCD_ENDPOINTS=%s", strings.Join(suite.etcdEndpoints, ",")),
	)

	// Capture output for debugging
	suite.gatewayCmd.Stdout = os.Stdout
	suite.gatewayCmd.Stderr = os.Stderr

	err := suite.gatewayCmd.Start()
	require.NoError(suite.T(), err, "Failed to start gateway")

	suite.T().Logf("Started gateway on port %d", suite.gatewayPort)
}

// waitForServices waits for all services to be ready
func (suite *E2ETestSuite) waitForServices() {
	suite.T().Log("Waiting for services to be ready...")

	// Wait for streamer
	suite.waitForHTTPService(fmt.Sprintf("http://localhost:%d/health", suite.streamerPort), "streamer")

	// Wait for gateway
	suite.waitForHTTPService(fmt.Sprintf("http://localhost:%d/health", suite.gatewayPort), "gateway")

	suite.T().Log("All services are ready")
}

// waitForHTTPService waits for an HTTP service to be ready
func (suite *E2ETestSuite) waitForHTTPService(url, serviceName string) {
	timeout := time.After(30 * time.Second)
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			suite.T().Fatalf("Timeout waiting for %s to be ready", serviceName)
		case <-ticker.C:
			resp, err := http.Get(url)
			if err == nil && resp.StatusCode == http.StatusOK {
				resp.Body.Close()
				suite.T().Logf("%s is ready", serviceName)
				return
			}
			if resp != nil {
				resp.Body.Close()
			}
		}
	}
}

// stopTelemetryServices stops all telemetry services
func (suite *E2ETestSuite) stopTelemetryServices() {
	if suite.gatewayCmd != nil && suite.gatewayCmd.Process != nil {
		suite.gatewayCmd.Process.Kill()
		suite.gatewayCmd.Wait()
		suite.T().Log("Stopped gateway")
	}

	if suite.collectorCmd != nil && suite.collectorCmd.Process != nil {
		suite.collectorCmd.Process.Kill()
		suite.collectorCmd.Wait()
		suite.T().Log("Stopped collector")
	}

	if suite.streamerCmd != nil && suite.streamerCmd.Process != nil {
		suite.streamerCmd.Process.Kill()
		suite.streamerCmd.Wait()
		suite.T().Log("Stopped streamer")
	}
}

// Helper methods

func (suite *E2ETestSuite) getTestFilePath() string {
	_, filename, _, _ := runtime.Caller(0)
	return filename
}

func (suite *E2ETestSuite) getProjectRoot() string {
	testFile := suite.getTestFilePath()
	// Go up from test/integration/e2e_test.go to project root
	return filepath.Join(filepath.Dir(filepath.Dir(filepath.Dir(testFile))))
}

// Test methods

// TestHealthEndpoints tests health endpoints of all services
func (suite *E2ETestSuite) TestHealthEndpoints() {
	// Test streamer health
	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/health", suite.streamerPort))
	require.NoError(suite.T(), err)
	defer resp.Body.Close()
	assert.Equal(suite.T(), http.StatusOK, resp.StatusCode)

	var streamerHealth map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&streamerHealth)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), "healthy", streamerHealth["status"])

	// Test gateway health
	resp, err = http.Get(fmt.Sprintf("http://localhost:%d/health", suite.gatewayPort))
	require.NoError(suite.T(), err)
	defer resp.Body.Close()
	assert.Equal(suite.T(), http.StatusOK, resp.StatusCode)

	var gatewayHealth map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&gatewayHealth)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), "healthy", gatewayHealth["status"])
}

// TestCSVUploadAndProcessing tests CSV upload and processing pipeline
func (suite *E2ETestSuite) TestCSVUploadAndProcessing() {
	// Upload CSV file to streamer
	uploadURL := fmt.Sprintf("http://localhost:%d/api/v1/csv/upload", suite.streamerPort)

	// Create multipart form
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	// Add file
	file, err := os.Open(suite.testCSVFile)
	require.NoError(suite.T(), err)
	defer file.Close()

	part, err := writer.CreateFormFile("file", "test_telemetry.csv")
	require.NoError(suite.T(), err)

	_, err = io.Copy(part, file)
	require.NoError(suite.T(), err)

	err = writer.Close()
	require.NoError(suite.T(), err)

	// Send upload request
	req, err := http.NewRequest("POST", uploadURL, &buf)
	require.NoError(suite.T(), err)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	require.NoError(suite.T(), err)
	defer resp.Body.Close()

	assert.Equal(suite.T(), http.StatusOK, resp.StatusCode)

	var uploadResponse map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&uploadResponse)
	require.NoError(suite.T(), err)

	assert.True(suite.T(), uploadResponse["success"].(bool))
	assert.Equal(suite.T(), "processed", uploadResponse["status"])
	assert.Greater(suite.T(), uploadResponse["record_count"].(float64), 0.0)

	suite.T().Logf("Successfully uploaded and processed %v records", uploadResponse["record_count"])

	// Wait for data to be processed by collector
	time.Sleep(5 * time.Second)
}

// TestGatewayAPIEndpoints tests all Gateway API endpoints
func (suite *E2ETestSuite) TestGatewayAPIEndpoints() {
	baseURL := fmt.Sprintf("http://localhost:%d", suite.gatewayPort)

	// First upload some data
	suite.TestCSVUploadAndProcessing()

	// Wait for data to be available
	time.Sleep(3 * time.Second)

	// Test /api/v1/gpus endpoint
	resp, err := http.Get(baseURL + "/api/v1/gpus")
	require.NoError(suite.T(), err)
	defer resp.Body.Close()
	assert.Equal(suite.T(), http.StatusOK, resp.StatusCode)

	var gpusResponse map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&gpusResponse)
	require.NoError(suite.T(), err)
	assert.True(suite.T(), gpusResponse["success"].(bool))
	assert.Greater(suite.T(), gpusResponse["count"].(float64), 0.0)

	gpus := gpusResponse["data"].([]interface{})
	suite.T().Logf("Found %d GPUs", len(gpus))

	// Test /api/v1/hosts endpoint
	resp, err = http.Get(baseURL + "/api/v1/hosts")
	require.NoError(suite.T(), err)
	defer resp.Body.Close()
	assert.Equal(suite.T(), http.StatusOK, resp.StatusCode)

	var hostsResponse map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&hostsResponse)
	require.NoError(suite.T(), err)
	assert.True(suite.T(), hostsResponse["success"].(bool))

	// Test /api/v1/clusters endpoint
	resp, err = http.Get(baseURL + "/api/v1/clusters")
	require.NoError(suite.T(), err)
	defer resp.Body.Close()
	assert.Equal(suite.T(), http.StatusOK, resp.StatusCode)

	var clustersResponse map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&clustersResponse)
	require.NoError(suite.T(), err)
	assert.Greater(suite.T(), clustersResponse["count"].(float64), 0.0)

	// Test specific cluster endpoint
	resp, err = http.Get(baseURL + "/api/v1/clusters/" + suite.clusterID)
	require.NoError(suite.T(), err)
	defer resp.Body.Close()
	assert.Equal(suite.T(), http.StatusOK, resp.StatusCode)

	// Test telemetry by GPU (if we have GPUs)
	if len(gpus) > 0 {
		gpu := gpus[0].(map[string]interface{})
		gpuUUID := gpu["uuid"].(string)

		resp, err = http.Get(baseURL + "/api/v1/gpus/" + gpuUUID + "/telemetry?limit=5")
		require.NoError(suite.T(), err)
		defer resp.Body.Close()
		assert.Equal(suite.T(), http.StatusOK, resp.StatusCode)

		var telemetryResponse map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&telemetryResponse)
		require.NoError(suite.T(), err)
		assert.True(suite.T(), telemetryResponse["success"].(bool))

		suite.T().Logf("Retrieved telemetry for GPU %s: %v records", gpuUUID, telemetryResponse["count"])
	}
}

// TestGraphQLEndpoint tests the GraphQL endpoint
func (suite *E2ETestSuite) TestGraphQLEndpoint() {
	baseURL := fmt.Sprintf("http://localhost:%d", suite.gatewayPort)

	// First upload some data
	suite.TestCSVUploadAndProcessing()

	// Wait for data to be available
	time.Sleep(3 * time.Second)

	// Test GraphQL query
	query := `{
		clusters {
			id
			name
			totalHosts
			totalGPUs
		}
	}`

	requestBody := map[string]interface{}{
		"query": query,
	}

	jsonData, err := json.Marshal(requestBody)
	require.NoError(suite.T(), err)

	resp, err := http.Post(baseURL+"/graphql", "application/json", bytes.NewBuffer(jsonData))
	require.NoError(suite.T(), err)
	defer resp.Body.Close()

	assert.Equal(suite.T(), http.StatusOK, resp.StatusCode)

	var graphqlResponse map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&graphqlResponse)
	require.NoError(suite.T(), err)

	// Check if we have data or errors
	if data, exists := graphqlResponse["data"]; exists {
		suite.T().Logf("GraphQL response data: %+v", data)
	}
	if errors, exists := graphqlResponse["errors"]; exists {
		suite.T().Logf("GraphQL response errors: %+v", errors)
	}
}

// TestWebSocketEndpoint tests the WebSocket endpoint
func (suite *E2ETestSuite) TestWebSocketEndpoint() {
	// This is a basic test to ensure the WebSocket endpoint is available
	// A full WebSocket test would require a WebSocket client library
	baseURL := fmt.Sprintf("http://localhost:%d", suite.gatewayPort)

	// Try to connect to WebSocket endpoint (this will fail but should return proper error)
	resp, err := http.Get(baseURL + "/ws")
	require.NoError(suite.T(), err)
	defer resp.Body.Close()

	// WebSocket upgrade should fail with regular HTTP request, but endpoint should exist
	assert.Equal(suite.T(), http.StatusBadRequest, resp.StatusCode)
}

// TestSwaggerDocumentation tests the Swagger documentation endpoint
func (suite *E2ETestSuite) TestSwaggerDocumentation() {
	baseURL := fmt.Sprintf("http://localhost:%d", suite.gatewayPort)

	// Test Swagger UI
	resp, err := http.Get(baseURL + "/swagger/")
	require.NoError(suite.T(), err)
	defer resp.Body.Close()
	assert.Equal(suite.T(), http.StatusOK, resp.StatusCode)

	// Test Swagger spec
	resp, err = http.Get(baseURL + "/swagger/spec.json")
	require.NoError(suite.T(), err)
	defer resp.Body.Close()
	assert.Equal(suite.T(), http.StatusOK, resp.StatusCode)

	var spec map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&spec)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), "3.0.0", spec["openapi"])
}

// TestEtcdDataPersistence tests that data is properly stored in etcd
func (suite *E2ETestSuite) TestEtcdDataPersistence() {
	// First upload some data
	suite.TestCSVUploadAndProcessing()

	// Wait for data to be processed
	time.Sleep(5 * time.Second)

	// Check etcd for cluster data
	clusterKey := fmt.Sprintf("/telemetry/clusters/%s", suite.clusterID)
	resp, err := suite.etcdClient.Get(context.Background(), clusterKey)
	require.NoError(suite.T(), err)
	assert.Greater(suite.T(), len(resp.Kvs), 0, "Cluster data should exist in etcd")

	// Check for host data
	hostsKey := fmt.Sprintf("/telemetry/clusters/%s/hosts/", suite.clusterID)
	resp, err = suite.etcdClient.Get(context.Background(), hostsKey, clientv3.WithPrefix())
	require.NoError(suite.T(), err)
	assert.Greater(suite.T(), len(resp.Kvs), 0, "Host data should exist in etcd")

	// Check for GPU data
	gpusKey := fmt.Sprintf("/telemetry/clusters/%s/hosts/", suite.clusterID)
	resp, err = suite.etcdClient.Get(context.Background(), gpusKey, clientv3.WithPrefix())
	require.NoError(suite.T(), err)

	// Count different types of keys
	hostCount := 0
	gpuCount := 0
	telemetryCount := 0

	for _, kv := range resp.Kvs {
		key := string(kv.Key)
		if strings.Contains(key, "/gpus/") && strings.Contains(key, "/data/") {
			telemetryCount++
		} else if strings.Contains(key, "/gpus/") {
			gpuCount++
		} else if !strings.Contains(key, "/gpus/") {
			hostCount++
		}
	}

	suite.T().Logf("Found in etcd: %d hosts, %d GPUs, %d telemetry records", hostCount, gpuCount, telemetryCount)
	assert.Greater(suite.T(), hostCount, 0, "Should have host data")
	assert.Greater(suite.T(), gpuCount, 0, "Should have GPU data")
	assert.Greater(suite.T(), telemetryCount, 0, "Should have telemetry data")
}

// TestServiceStatus tests the status endpoints of services
func (suite *E2ETestSuite) TestServiceStatus() {
	// Test streamer status
	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/api/v1/status", suite.streamerPort))
	require.NoError(suite.T(), err)
	defer resp.Body.Close()
	assert.Equal(suite.T(), http.StatusOK, resp.StatusCode)

	var streamerStatus map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&streamerStatus)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), "nexus-streamer", streamerStatus["service"])
	assert.Equal(suite.T(), suite.clusterID, streamerStatus["cluster_id"])
}

// TestErrorHandling tests error handling scenarios
func (suite *E2ETestSuite) TestErrorHandling() {
	baseURL := fmt.Sprintf("http://localhost:%d", suite.gatewayPort)

	// Test non-existent GPU
	resp, err := http.Get(baseURL + "/api/v1/gpus/non-existent-gpu/telemetry")
	require.NoError(suite.T(), err)
	defer resp.Body.Close()
	assert.Equal(suite.T(), http.StatusOK, resp.StatusCode) // Should return empty result, not error

	var response map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), 0.0, response["count"])

	// Test non-existent cluster
	resp, err = http.Get(baseURL + "/api/v1/clusters/non-existent-cluster")
	require.NoError(suite.T(), err)
	defer resp.Body.Close()
	assert.Equal(suite.T(), http.StatusNotFound, resp.StatusCode)

	// Test invalid GraphQL query
	invalidQuery := `{
		invalidField {
			nonExistentField
		}
	}`

	requestBody := map[string]interface{}{
		"query": invalidQuery,
	}

	jsonData, err := json.Marshal(requestBody)
	require.NoError(suite.T(), err)

	resp, err = http.Post(baseURL+"/graphql", "application/json", bytes.NewBuffer(jsonData))
	require.NoError(suite.T(), err)
	defer resp.Body.Close()

	var graphqlResponse map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&graphqlResponse)
	require.NoError(suite.T(), err)

	// Should have errors for invalid query
	if errors, exists := graphqlResponse["errors"]; exists {
		assert.Greater(suite.T(), len(errors.([]interface{})), 0)
	}
}

// TestConcurrentRequests tests handling of concurrent requests
func (suite *E2ETestSuite) TestConcurrentRequests() {
	baseURL := fmt.Sprintf("http://localhost:%d", suite.gatewayPort)

	// First upload some data
	suite.TestCSVUploadAndProcessing()

	// Wait for data to be available
	time.Sleep(3 * time.Second)

	// Make concurrent requests
	const numRequests = 10
	results := make(chan error, numRequests)

	for i := 0; i < numRequests; i++ {
		go func() {
			resp, err := http.Get(baseURL + "/api/v1/gpus")
			if err != nil {
				results <- err
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				results <- fmt.Errorf("unexpected status code: %d", resp.StatusCode)
				return
			}

			results <- nil
		}()
	}

	// Check all results
	for i := 0; i < numRequests; i++ {
		err := <-results
		assert.NoError(suite.T(), err, "Concurrent request %d failed", i)
	}
}

// Run the test suite
func TestE2ETestSuite(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E tests in short mode")
	}

	// Check if Docker is available
	if !isDockerAvailable() {
		t.Skip("Docker is not available, skipping E2E tests")
	}

	suite.Run(t, new(E2ETestSuite))
}

// isDockerAvailable checks if Docker is available
func isDockerAvailable() bool {
	cmd := exec.Command("docker", "version")
	return cmd.Run() == nil
}
