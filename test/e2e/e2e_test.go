package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net"
	"net/http"
	"os"
	"strconv"

	"sync"
	"testing"
	"time"

	"github.com/dilipmighty245/telemetry-pipeline/internal/collector"
	"github.com/dilipmighty245/telemetry-pipeline/internal/gateway"
	"github.com/dilipmighty245/telemetry-pipeline/internal/streamer"
	"github.com/dilipmighty245/telemetry-pipeline/test/testhelper"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testTimeout = 60 * time.Second
)

// getAvailablePort returns an available port on the local machine
func getAvailablePort() (int, error) {
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()

	addr := listener.Addr().(*net.TCPAddr)
	return addr.Port, nil
}

func enforcePortRelease(t *testing.T, ports ...int) {
	t.Helper()

	t.Cleanup(func() {
		// Give services a moment to fully shutdown before checking
		time.Sleep(200 * time.Millisecond)

		for _, port := range ports {
			addr := fmt.Sprintf(":%d", port)
			ln, err := net.Listen("tcp", addr)
			if err != nil {
				t.Fatalf("port %d is still in use after test cleanup", port)
			}
			_ = ln.Close()
		}
	})
}

// TestE2EServices tests the complete end-to-end flow
func TestE2EServices(t *testing.T) {
	// Get available ports for services
	gatewayPort, err := getAvailablePort()
	require.NoError(t, err, "Should get available port for gateway")

	streamerPort, err := getAvailablePort()
	require.NoError(t, err, "Should get available port for streamer")

	pprofPort, err := getAvailablePort()
	require.NoError(t, err, "Should get available port for pprof")

	// Start embedded etcd server for testing
	etcdServer, cleanup, err := testhelper.SetupEtcdForTest()
	require.NoError(t, err, "Should start embedded etcd server")
	defer cleanup()

	// Wait for etcd to be ready
	err = testhelper.WaitForEtcdReady(etcdServer.Endpoints, 10*time.Second)
	require.NoError(t, err, "etcd should be ready")

	// Start services in background using goroutines
	var wg sync.WaitGroup
	// serviceCancel := make(chan struct{})
	// defer close(serviceCancel)

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	// Ensure all services stop at test end
	t.Cleanup(func() {
		cancel()
		done := make(chan struct{})
		go func() {
			wg.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Log("Services did not shut down in time")
		}
	})

	// Start services
	start := func(name string, fn func(context.Context) error) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := fn(ctx); err != nil && err != context.Canceled {
				t.Logf("%s exited with error: %v", name, err)
			}
		}()
	}

	os.Setenv("PPROF_PORT", strconv.Itoa(pprofPort))

	start("collector", func(ctx context.Context) error {
		cl := &collector.NexusCollectorService{}
		return cl.Run(ctx, []string{}, os.Stdout)
	})
	start("gateway", func(ctx context.Context) error {
		gw := &gateway.NexusGatewayService{}
		return gw.Run(ctx, []string{"--port=" + strconv.Itoa(gatewayPort)}, os.Stdout)
	})

	start("streamer", func(ctx context.Context) error {
		st := &streamer.NexusStreamerService{}
		return st.Run(ctx, []string{"--port=" + strconv.Itoa(streamerPort)}, os.Stdout)
	})

	// Wait for services to become ready
	waitForService(t, fmt.Sprintf("http://localhost:%d/health", gatewayPort), 10*time.Second)
	waitForService(t, fmt.Sprintf("http://localhost:%d/health", streamerPort), 10*time.Second)

	// Run E2E tests with dynamic ports
	t.Run("HealthChecks", func(t *testing.T) { testHealthChecks(t, gatewayPort, streamerPort) })
	t.Run("CSVUpload", func(t *testing.T) { testCSVUpload(t, streamerPort) })
	t.Run("TelemetryQuery", func(t *testing.T) { testTelemetryQuery(t, gatewayPort) })
	t.Run("ServiceIntegration", func(t *testing.T) { testServiceIntegration(t, gatewayPort, streamerPort) })
}

func testHealthChecks(t *testing.T, gatewayPort, streamerPort int) {
	// Test gateway health
	gatewayURL := fmt.Sprintf("http://localhost:%d/health", gatewayPort)
	resp, err := http.Get(gatewayURL)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var healthResp map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&healthResp)
	require.NoError(t, err)
	assert.Equal(t, "healthy", healthResp["status"])

	// Test streamer health
	streamerURL := fmt.Sprintf("http://localhost:%d/health", streamerPort)
	resp, err = http.Get(streamerURL)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	err = json.NewDecoder(resp.Body).Decode(&healthResp)
	require.NoError(t, err)
	assert.Equal(t, "healthy", healthResp["status"])
}

func testCSVUpload(t *testing.T, streamerPort int) {
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
	uploadURL := fmt.Sprintf("http://localhost:%d/api/v1/csv/upload", streamerPort)
	req, err := http.NewRequest("POST", uploadURL, &buf)
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

func testTelemetryQuery(t *testing.T, gatewayPort int) {
	// Wait a bit for data to be processed
	time.Sleep(2 * time.Second)

	// Query all GPUs
	gpusURL := fmt.Sprintf("http://localhost:%d/api/v1/gpus", gatewayPort)
	resp, err := http.Get(gpusURL)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var gpusResp map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&gpusResp)
	require.NoError(t, err)

	assert.True(t, gpusResp["success"].(bool))

	// Query telemetry by GPU (should return empty for now since we need etcd integration)
	telemetryURL := fmt.Sprintf("http://localhost:%d/api/v1/gpus/GPU-12345/telemetry", gatewayPort)
	resp, err = http.Get(telemetryURL)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func testServiceIntegration(t *testing.T, gatewayPort, streamerPort int) {
	// Test that services can communicate

	// Check streamer status
	statusURL := fmt.Sprintf("http://localhost:%d/api/v1/status", streamerPort)
	resp, err := http.Get(statusURL)
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
		endpointURL := fmt.Sprintf("http://localhost:%d%s", gatewayPort, endpoint)
		resp, err := http.Get(endpointURL)
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
	_, err = part.Write([]byte("not a csv file"))
	require.NoError(t, err)
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
			_, _ = part.Write([]byte(csvContent))
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

func runService(t *testing.T, start func() (*http.Server, error)) (stop func()) {
	t.Helper()
	srv, err := start()
	require.NoError(t, err)
	return func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx) // best-effort shutdown
	}
}
