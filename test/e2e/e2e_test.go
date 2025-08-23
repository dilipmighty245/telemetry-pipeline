package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	etcdPort     = "2379"
	gatewayPort  = "8080"
	streamerPort = "8081"
	testTimeout  = 30 * time.Second
)

// TestE2EServices tests the complete end-to-end flow
func TestE2EServices(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E tests in short mode")
	}

	// Start etcd for testing
	etcdCmd := startEtcd(t)
	defer stopService(etcdCmd)

	// Wait for etcd to be ready
	waitForService(t, "http://localhost:2379/health", 10*time.Second)

	// Start services
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	// Start services in background
	gatewayCmd := startGateway(t, ctx)
	defer stopService(gatewayCmd)

	streamerCmd := startStreamer(t, ctx)
	defer stopService(streamerCmd)

	collectorCmd := startCollector(t, ctx)
	defer stopService(collectorCmd)

	// Wait for services to be ready
	waitForService(t, "http://localhost:8080/health", 10*time.Second)
	waitForService(t, "http://localhost:8081/health", 10*time.Second)

	// Run E2E tests
	t.Run("HealthChecks", testHealthChecks)
	t.Run("CSVUpload", testCSVUpload)
	t.Run("TelemetryQuery", testTelemetryQuery)
	t.Run("ServiceIntegration", testServiceIntegration)
}

func TestE2EServicesWithCoverage(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E coverage tests in short mode")
	}

	// Build services with coverage
	buildServiceWithCoverage(t, "nexus-collector")
	buildServiceWithCoverage(t, "nexus-gateway")
	buildServiceWithCoverage(t, "nexus-streamer")

	// Start etcd for testing
	etcdCmd := startEtcd(t)
	defer stopService(etcdCmd)

	// Wait for etcd to be ready
	waitForService(t, "http://localhost:2379/health", 10*time.Second)

	// Start services with coverage
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	gatewayCmd := startServiceWithCoverage(t, ctx, "nexus-gateway", []string{})
	defer stopService(gatewayCmd)

	streamerCmd := startServiceWithCoverage(t, ctx, "nexus-streamer", []string{})
	defer stopService(streamerCmd)

	collectorCmd := startServiceWithCoverage(t, ctx, "nexus-collector", []string{})
	defer stopService(collectorCmd)

	// Wait for services to be ready
	waitForService(t, "http://localhost:8080/health", 10*time.Second)
	waitForService(t, "http://localhost:8081/health", 10*time.Second)

	// Run comprehensive tests
	t.Run("HealthChecks", testHealthChecks)
	t.Run("CSVUpload", testCSVUpload)
	t.Run("TelemetryQuery", testTelemetryQuery)
	t.Run("ServiceIntegration", testServiceIntegration)
	t.Run("ErrorHandling", testErrorHandling)
	t.Run("LoadTesting", testLoadTesting)

	// Collect coverage data
	collectCoverageData(t)
}

func testHealthChecks(t *testing.T) {
	// Test gateway health
	resp, err := http.Get("http://localhost:8080/health")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var healthResp map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&healthResp)
	require.NoError(t, err)
	assert.Equal(t, "healthy", healthResp["status"])

	// Test streamer health
	resp, err = http.Get("http://localhost:8081/health")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	err = json.NewDecoder(resp.Body).Decode(&healthResp)
	require.NoError(t, err)
	assert.Equal(t, "healthy", healthResp["status"])
}

func testCSVUpload(t *testing.T) {
	// Create test CSV content
	csvContent := `timestamp,gpu_id,hostname,uuid,device,modelname,gpu_utilization,memory_utilization,memory_used_mb,memory_free_mb,temperature,power_draw,sm_clock_mhz,memory_clock_mhz
2023-01-01T00:00:00Z,0,test-host,GPU-12345,nvidia0,NVIDIA H100,85.5,60.2,8192,2048,72.0,350.5,1410,1215
2023-01-01T00:01:00Z,1,test-host,GPU-67890,nvidia1,NVIDIA H100,90.0,65.0,9000,1240,75.0,360.0,1420,1220`

	// Create multipart form
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	part, err := writer.CreateFormFile("file", "test.csv")
	require.NoError(t, err)

	_, err = part.Write([]byte(csvContent))
	require.NoError(t, err)

	err = writer.Close()
	require.NoError(t, err)

	// Upload CSV
	req, err := http.NewRequest("POST", "http://localhost:8081/api/v1/csv/upload", &buf)
	require.NoError(t, err)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var uploadResp map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&uploadResp)
	require.NoError(t, err)

	assert.True(t, uploadResp["success"].(bool))
	assert.Equal(t, "test.csv", uploadResp["filename"])
	assert.Greater(t, uploadResp["record_count"].(float64), 0.0)
}

func testTelemetryQuery(t *testing.T) {
	// Wait a bit for data to be processed
	time.Sleep(2 * time.Second)

	// Query all GPUs
	resp, err := http.Get("http://localhost:8080/api/v1/gpus")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var gpusResp map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&gpusResp)
	require.NoError(t, err)

	assert.True(t, gpusResp["success"].(bool))

	// Query telemetry by GPU (should return empty for now since we need etcd integration)
	resp, err = http.Get("http://localhost:8080/api/v1/gpus/GPU-12345/telemetry")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func testServiceIntegration(t *testing.T) {
	// Test that services can communicate

	// Check streamer status
	resp, err := http.Get("http://localhost:8081/api/v1/status")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var statusResp map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&statusResp)
	require.NoError(t, err)
	assert.Equal(t, "nexus-streamer", statusResp["service"])

	// Test gateway endpoints
	endpoints := []string{
		"/api/v1/clusters",
		"/api/v1/hosts",
		"/api/v1/telemetry/latest",
	}

	for _, endpoint := range endpoints {
		resp, err := http.Get("http://localhost:8080" + endpoint)
		require.NoError(t, err)
		resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	}
}

func testErrorHandling(t *testing.T) {
	// Test invalid CSV upload
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	part, err := writer.CreateFormFile("file", "invalid.txt")
	require.NoError(t, err)
	part.Write([]byte("not a csv file"))
	writer.Close()

	req, err := http.NewRequest("POST", "http://localhost:8081/api/v1/csv/upload", &buf)
	require.NoError(t, err)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	// Test invalid GPU query
	resp, err = http.Get("http://localhost:8080/api/v1/gpus//telemetry")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func testLoadTesting(t *testing.T) {
	// Simple load test - upload multiple CSV files concurrently
	csvContent := `timestamp,gpu_id,hostname,uuid,device,modelname,gpu_utilization,memory_utilization
2023-01-01T00:00:00Z,0,load-test-host,GPU-LOAD-TEST,nvidia0,NVIDIA H100,85.5,60.2`

	const numRequests = 5
	done := make(chan bool, numRequests)

	for i := 0; i < numRequests; i++ {
		go func(id int) {
			var buf bytes.Buffer
			writer := multipart.NewWriter(&buf)

			part, _ := writer.CreateFormFile("file", fmt.Sprintf("load-test-%d.csv", id))
			part.Write([]byte(csvContent))
			writer.Close()

			req, _ := http.NewRequest("POST", "http://localhost:8081/api/v1/csv/upload", &buf)
			req.Header.Set("Content-Type", writer.FormDataContentType())

			client := &http.Client{Timeout: 10 * time.Second}
			resp, err := client.Do(req)
			if err == nil {
				resp.Body.Close()
			}

			done <- true
		}(i)
	}

	// Wait for all requests to complete
	for i := 0; i < numRequests; i++ {
		select {
		case <-done:
			// Request completed
		case <-time.After(15 * time.Second):
			t.Fatal("Load test timed out")
		}
	}
}

// Helper functions

func startEtcd(t *testing.T) *exec.Cmd {
	// Check if etcd is available
	if _, err := exec.LookPath("etcd"); err != nil {
		t.Skip("etcd not available, skipping E2E tests")
	}

	cmd := exec.Command("etcd",
		"--data-dir", "/tmp/etcd-test",
		"--listen-client-urls", "http://localhost:2379",
		"--advertise-client-urls", "http://localhost:2379",
		"--listen-peer-urls", "http://localhost:2380",
		"--initial-advertise-peer-urls", "http://localhost:2380",
		"--initial-cluster", "default=http://localhost:2380",
	)

	err := cmd.Start()
	require.NoError(t, err)

	return cmd
}

func startGateway(t *testing.T, ctx context.Context) *exec.Cmd {
	return startServiceInBackground(t, ctx, "gateway", []string{})
}

func startStreamer(t *testing.T, ctx context.Context) *exec.Cmd {
	return startServiceInBackground(t, ctx, "streamer", []string{})
}

func startCollector(t *testing.T, ctx context.Context) *exec.Cmd {
	return startServiceInBackground(t, ctx, "collector", []string{})
}

func startServiceInBackground(t *testing.T, ctx context.Context, service string, args []string) *exec.Cmd {
	// Build and start the service as external process
	binaryPath := fmt.Sprintf("./cmd/nexus-%s/nexus-%s", service, service)

	// Build the service first
	buildCmd := exec.Command("go", "build", "-o", binaryPath, fmt.Sprintf("./cmd/nexus-%s", service))
	err := buildCmd.Run()
	require.NoError(t, err)

	// Start the service
	cmd := exec.CommandContext(ctx, binaryPath, args...)
	cmd.Env = os.Environ()

	err = cmd.Start()
	require.NoError(t, err)

	return cmd
}

func buildServiceWithCoverage(t *testing.T, service string) {
	cmd := exec.Command("go", "build", "-cover", "-o", service+"-coverage", "./cmd/nexus-"+service)
	err := cmd.Run()
	require.NoError(t, err)
}

func startServiceWithCoverage(t *testing.T, ctx context.Context, service string, args []string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "./"+service+"-coverage", args...)
	cmd.Env = append(os.Environ(), "GOCOVERDIR=./coverage")

	err := cmd.Start()
	require.NoError(t, err)

	return cmd
}

func collectCoverageData(t *testing.T) {
	// Merge coverage data
	cmd := exec.Command("go", "tool", "covdata", "textfmt", "-i=./coverage", "-o=e2e-coverage.out")
	err := cmd.Run()
	if err != nil {
		t.Logf("Failed to collect coverage data: %v", err)
	}
}

func waitForService(t *testing.T, url string, timeout time.Duration) {
	client := &http.Client{Timeout: 1 * time.Second}
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(500 * time.Millisecond)
	}

	t.Fatalf("Service at %s not ready within %v", url, timeout)
}

func stopService(cmd *exec.Cmd) {
	if cmd != nil && cmd.Process != nil {
		cmd.Process.Kill()
		cmd.Wait()
	}
}
