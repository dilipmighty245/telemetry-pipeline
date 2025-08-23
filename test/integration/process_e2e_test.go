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
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	clientv3 "go.etcd.io/etcd/client/v3"
)

// ProcessE2ETestSuite runs E2E tests using direct process execution
type ProcessE2ETestSuite struct {
	suite.Suite

	// Infrastructure
	etcdClient      *clientv3.Client
	testDataDir     string
	dockerComposeUp bool

	// Service contexts and cancellation
	streamerCtx     context.Context
	collectorCtx    context.Context
	gatewayCtx      context.Context
	streamerCancel  context.CancelFunc
	collectorCancel context.CancelFunc
	gatewayCancel   context.CancelFunc

	// Service wait groups
	servicesWG sync.WaitGroup

	// Configuration
	streamerPort  int
	gatewayPort   int
	etcdEndpoints []string
	clusterID     string
	testCSVFile   string

	// Test data
	expectedRecords int
	testHosts       []string
	testGPUs        []string
}

// SetupSuite runs before all tests in the suite
func (suite *ProcessE2ETestSuite) SetupSuite() {
	suite.streamerPort = 8091
	suite.gatewayPort = 8090
	suite.etcdEndpoints = []string{"localhost:12379"}
	suite.clusterID = "process-test-cluster"
	suite.expectedRecords = 10
	suite.testHosts = []string{"test-host-1", "test-host-2"}
	suite.testGPUs = []string{"GPU-12345-67890-ABCDE", "GPU-12345-67890-ABCDF", "GPU-FEDCB-09876-54321"}

	// Create test data directory
	var err error
	suite.testDataDir, err = os.MkdirTemp("", "telemetry-process-e2e-test-*")
	require.NoError(suite.T(), err)

	suite.T().Logf("Test data directory: %s", suite.testDataDir)

	// Start etcd using docker-compose
	suite.startEtcd()

	// Wait for etcd to be ready
	suite.waitForEtcd()

	// Create test CSV file
	suite.createTestCSVFile()

	// Start telemetry services as processes
	suite.startTelemetryProcesses()

	// Wait for services to be ready
	suite.waitForServices()
}

// TearDownSuite runs after all tests in the suite
func (suite *ProcessE2ETestSuite) TearDownSuite() {
	// Stop telemetry processes
	suite.stopTelemetryProcesses()

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
func (suite *ProcessE2ETestSuite) startEtcd() {
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
func (suite *ProcessE2ETestSuite) stopEtcd() {
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
func (suite *ProcessE2ETestSuite) waitForEtcd() {
	suite.T().Log("Waiting for etcd to be ready...")

	timeout := time.After(60 * time.Second)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			suite.T().Fatal("Timeout waiting for etcd to be ready")
		case <-ticker.C:
			client, err := clientv3.New(clientv3.Config{
				Endpoints:   suite.etcdEndpoints,
				DialTimeout: 5 * time.Second,
			})
			if err != nil {
				continue
			}

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
func (suite *ProcessE2ETestSuite) createTestCSVFile() {
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

// startTelemetryProcesses starts all telemetry services as processes
func (suite *ProcessE2ETestSuite) startTelemetryProcesses() {
	suite.T().Log("Starting telemetry services as processes...")

	// Start streamer process
	suite.startStreamerProcess()

	// Start collector process
	suite.startCollectorProcess()

	// Start gateway process
	suite.startGatewayProcess()
}

// startStreamerProcess starts the streamer service as a process
func (suite *ProcessE2ETestSuite) startStreamerProcess() {
	suite.streamerCtx, suite.streamerCancel = context.WithCancel(context.Background())

	suite.servicesWG.Add(1)
	go func() {
		defer suite.servicesWG.Done()

		// Import the streamer main package
		// We'll call the run function directly
		args := []string{
			"nexus-streamer",
			"-cluster-id", suite.clusterID,
			"-streamer-id", "process-test-streamer-1",
			"-http-port", fmt.Sprintf("%d", suite.streamerPort),
			"-batch-size", "5",
			"-stream-interval", "1s",
			"-log-level", "info",
		}

		// Set environment variables
		os.Setenv("ETCD_ENDPOINTS", strings.Join(suite.etcdEndpoints, ","))
		os.Setenv("UPLOAD_DIR", filepath.Join(suite.testDataDir, "uploads"))

		// Call the streamer run function
		if err := suite.runStreamer(suite.streamerCtx, args); err != nil && err != context.Canceled {
			suite.T().Logf("Streamer process error: %v", err)
		}
	}()

	suite.T().Logf("Started streamer process on port %d", suite.streamerPort)
}

// startCollectorProcess starts the collector service as a process
func (suite *ProcessE2ETestSuite) startCollectorProcess() {
	suite.collectorCtx, suite.collectorCancel = context.WithCancel(context.Background())

	suite.servicesWG.Add(1)
	go func() {
		defer suite.servicesWG.Done()

		args := []string{
			"nexus-collector",
			"-cluster-id", suite.clusterID,
			"-collector-id", "process-test-collector-1",
			"-batch-size", "5",
			"-poll-interval", "1s",
			"-workers", "2",
			"-log-level", "info",
		}

		// Set environment variables
		os.Setenv("ETCD_ENDPOINTS", strings.Join(suite.etcdEndpoints, ","))

		// Call the collector run function
		if err := suite.runCollector(suite.collectorCtx, args); err != nil && err != context.Canceled {
			suite.T().Logf("Collector process error: %v", err)
		}
	}()

	suite.T().Log("Started collector process")
}

// startGatewayProcess starts the gateway service as a process
func (suite *ProcessE2ETestSuite) startGatewayProcess() {
	suite.gatewayCtx, suite.gatewayCancel = context.WithCancel(context.Background())

	suite.servicesWG.Add(1)
	go func() {
		defer suite.servicesWG.Done()

		args := []string{
			"nexus-gateway",
			"-port", fmt.Sprintf("%d", suite.gatewayPort),
			"-cluster-id", suite.clusterID,
			"-log-level", "info",
		}

		// Set environment variables
		os.Setenv("ETCD_ENDPOINTS", strings.Join(suite.etcdEndpoints, ","))

		// Call the gateway run function
		if err := suite.runGateway(suite.gatewayCtx, args); err != nil && err != context.Canceled {
			suite.T().Logf("Gateway process error: %v", err)
		}
	}()

	suite.T().Logf("Started gateway process on port %d", suite.gatewayPort)
}

// waitForServices waits for all services to be ready
func (suite *ProcessE2ETestSuite) waitForServices() {
	suite.T().Log("Waiting for services to be ready...")

	// Wait for streamer
	suite.waitForHTTPService(fmt.Sprintf("http://localhost:%d/health", suite.streamerPort), "streamer")

	// Wait for gateway
	suite.waitForHTTPService(fmt.Sprintf("http://localhost:%d/health", suite.gatewayPort), "gateway")

	suite.T().Log("All services are ready")
}

// waitForHTTPService waits for an HTTP service to be ready
func (suite *ProcessE2ETestSuite) waitForHTTPService(url, serviceName string) {
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

// stopTelemetryProcesses stops all telemetry processes
func (suite *ProcessE2ETestSuite) stopTelemetryProcesses() {
	suite.T().Log("Stopping telemetry processes...")

	// Cancel all contexts
	if suite.gatewayCancel != nil {
		suite.gatewayCancel()
	}
	if suite.collectorCancel != nil {
		suite.collectorCancel()
	}
	if suite.streamerCancel != nil {
		suite.streamerCancel()
	}

	// Wait for all services to stop
	done := make(chan struct{})
	go func() {
		suite.servicesWG.Wait()
		close(done)
	}()

	select {
	case <-done:
		suite.T().Log("All services stopped gracefully")
	case <-time.After(10 * time.Second):
		suite.T().Log("Timeout waiting for services to stop")
	}
}

// Helper methods

func (suite *ProcessE2ETestSuite) getTestFilePath() string {
	_, filename, _, _ := runtime.Caller(0)
	return filename
}

func (suite *ProcessE2ETestSuite) getProjectRoot() string {
	testFile := suite.getTestFilePath()
	return filepath.Join(filepath.Dir(filepath.Dir(filepath.Dir(testFile))))
}

// Service runner methods - these call the actual service implementations

func (suite *ProcessE2ETestSuite) runStreamer(ctx context.Context, args []string) error {
	wrapper := NewServiceWrapper()
	return wrapper.RunStreamer(ctx, args)
}

func (suite *ProcessE2ETestSuite) runCollector(ctx context.Context, args []string) error {
	wrapper := NewServiceWrapper()
	return wrapper.RunCollector(ctx, args)
}

func (suite *ProcessE2ETestSuite) runGateway(ctx context.Context, args []string) error {
	wrapper := NewServiceWrapper()
	return wrapper.RunGateway(ctx, args)
}

// Test methods

// TestProcessHealthEndpoints tests health endpoints of all services
func (suite *ProcessE2ETestSuite) TestProcessHealthEndpoints() {
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

// TestProcessCSVUploadAndProcessing tests the complete E2E flow
func (suite *ProcessE2ETestSuite) TestProcessCSVUploadAndProcessing() {
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

// TestProcessGatewayAPIEndpoints tests all Gateway API endpoints
func (suite *ProcessE2ETestSuite) TestProcessGatewayAPIEndpoints() {
	baseURL := fmt.Sprintf("http://localhost:%d", suite.gatewayPort)

	// First upload some data
	suite.TestProcessCSVUploadAndProcessing()

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

	// Test /api/v1/hosts endpoint
	resp, err = http.Get(baseURL + "/api/v1/hosts")
	require.NoError(suite.T(), err)
	defer resp.Body.Close()
	assert.Equal(suite.T(), http.StatusOK, resp.StatusCode)

	// Test /api/v1/clusters endpoint
	resp, err = http.Get(baseURL + "/api/v1/clusters")
	require.NoError(suite.T(), err)
	defer resp.Body.Close()
	assert.Equal(suite.T(), http.StatusOK, resp.StatusCode)
}

// TestProcessDataPersistence tests that data flows through the entire pipeline
func (suite *ProcessE2ETestSuite) TestProcessDataPersistence() {
	// Upload test data
	suite.TestProcessCSVUploadAndProcessing()

	// Wait for processing
	time.Sleep(5 * time.Second)

	// Check etcd for data
	clusterKey := fmt.Sprintf("/telemetry/clusters/%s", suite.clusterID)
	resp, err := suite.etcdClient.Get(context.Background(), clusterKey)
	require.NoError(suite.T(), err)

	// We should have some data in etcd
	hostsKey := fmt.Sprintf("/telemetry/clusters/%s/hosts/", suite.clusterID)
	resp, err = suite.etcdClient.Get(context.Background(), hostsKey, clientv3.WithPrefix())
	require.NoError(suite.T(), err)

	suite.T().Logf("Found %d keys in etcd under hosts", len(resp.Kvs))

	// Verify we have the expected hosts and GPUs
	hostCount := make(map[string]int)
	gpuCount := make(map[string]int)

	for _, kv := range resp.Kvs {
		key := string(kv.Key)
		parts := strings.Split(key, "/")

		if len(parts) >= 6 {
			hostname := parts[5]
			hostCount[hostname]++

			if strings.Contains(key, "/gpus/") && len(parts) >= 8 {
				gpuID := parts[7]
				gpuCount[gpuID]++
			}
		}
	}

	suite.T().Logf("Hosts found: %v", hostCount)
	suite.T().Logf("GPUs found: %v", gpuCount)

	// We should have data for our test hosts
	assert.Greater(suite.T(), len(hostCount), 0, "Should have host data")
	assert.Greater(suite.T(), len(gpuCount), 0, "Should have GPU data")
}

// Run the test suite
func TestProcessE2ETestSuite(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping Process E2E tests in short mode")
	}

	// Check if Docker is available
	if !isDockerAvailable() {
		t.Skip("Docker is not available, skipping Process E2E tests")
	}

	suite.Run(t, new(ProcessE2ETestSuite))
}
