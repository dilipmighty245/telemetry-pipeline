package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"os"

	"sync"
	"testing"
	"time"

	"github.com/dilipmighty245/telemetry-pipeline/pkg/messagequeue"

	// Import service packages for in-process testing
	"github.com/dilipmighty245/telemetry-pipeline/internal/collector"
	"github.com/dilipmighty245/telemetry-pipeline/internal/gateway"
	"github.com/dilipmighty245/telemetry-pipeline/internal/streamer"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testTimeout = 30 * time.Second
)

// TestE2EServices tests the complete end-to-end flow
func TestE2EServices(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E tests in short mode")
	}

	// Start embedded etcd server for testing
	etcdServer, cleanup, err := messagequeue.SetupEtcdForTest()
	require.NoError(t, err, "Should start embedded etcd server")
	defer cleanup()

	// Wait for etcd to be ready
	err = messagequeue.WaitForEtcdReady(etcdServer.Endpoints, 10*time.Second)
	require.NoError(t, err, "etcd should be ready")

	// Start services in background using goroutines
	var wg sync.WaitGroup
	serviceCancel := make(chan struct{})
	defer close(serviceCancel)

	// Start gateway service
	wg.Add(1)
	go func() {
		defer wg.Done()
		gatewayService := &gateway.NexusGatewayService{}
		if err := gatewayService.Run([]string{"--port=8080"}, os.Stdout); err != nil && err != context.Canceled {
			t.Logf("Gateway service error: %v", err)
		}
	}()

	// Start streamer service
	wg.Add(1)
	go func() {
		defer wg.Done()
		streamerService := &streamer.NexusStreamerService{}
		if err := streamerService.Run([]string{"--port=8081"}, os.Stdout); err != nil && err != context.Canceled {
			t.Logf("Streamer service error: %v", err)
		}
	}()

	// Start collector service
	wg.Add(1)
	go func() {
		defer wg.Done()
		collectorService := &collector.NexusCollectorService{}
		if err := collectorService.Run([]string{}, os.Stdout); err != nil && err != context.Canceled {
			t.Logf("Collector service error: %v", err)
		}
	}()

	// Ensure services are stopped when test completes
	defer func() {
		// Give services a moment to shut down gracefully
		done := make(chan struct{})
		go func() {
			wg.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Log("Services did not shut down within 5 seconds")
		}
	}()

	// Wait for services to be ready
	waitForService(t, "http://localhost:8080/health", 10*time.Second)
	waitForService(t, "http://localhost:8081/health", 10*time.Second)

	// Run E2E tests
	t.Run("HealthChecks", testHealthChecks)
	t.Run("CSVUpload", testCSVUpload)
	t.Run("TelemetryQuery", testTelemetryQuery)
	t.Run("ServiceIntegration", testServiceIntegration)
}

// func TestE2EServicesWithCoverage(t *testing.T) {
// 	if testing.Short() {
// 		t.Skip("Skipping E2E coverage tests in short mode")
// 	}

// 	// Start embedded etcd server for testing
// 	etcdServer, cleanup, err := messagequeue.SetupEtcdForTest()
// 	require.NoError(t, err, "Should start embedded etcd server")
// 	defer cleanup()

// 	// Wait for etcd to be ready
// 	err = messagequeue.WaitForEtcdReady(etcdServer.Endpoints, 10*time.Second)
// 	require.NoError(t, err, "etcd should be ready")

// 	// Start services in background using goroutines
// 	var wg sync.WaitGroup
// 	serviceCancel := make(chan struct{})
// 	defer close(serviceCancel)

// 	// Start gateway service
// 	wg.Add(1)
// 	go func() {
// 		defer wg.Done()
// 		gatewayService := &gateway.NexusGatewayService{}
// 		if err := gatewayService.Run([]string{"--port=8080"}, os.Stdout); err != nil && err != context.Canceled {
// 			t.Logf("Gateway service error: %v", err)
// 		}
// 	}()

// 	// Start streamer service
// 	wg.Add(1)
// 	go func() {
// 		defer wg.Done()
// 		streamerService := &streamer.NexusStreamerService{}
// 		if err := streamerService.Run([]string{"--port=8081"}, os.Stdout); err != nil && err != context.Canceled {
// 			t.Logf("Streamer service error: %v", err)
// 		}
// 	}()

// 	// Start collector service
// 	wg.Add(1)
// 	go func() {
// 		defer wg.Done()
// 		collectorService := &collector.NexusCollectorService{}
// 		if err := collectorService.Run([]string{}, os.Stdout); err != nil && err != context.Canceled {
// 			t.Logf("Collector service error: %v", err)
// 		}
// 	}()

// 	// Ensure services are stopped when test completes
// 	defer func() {
// 		// Give services a moment to shut down gracefully
// 		done := make(chan struct{})
// 		go func() {
// 			wg.Wait()
// 			close(done)
// 		}()
// 		select {
// 		case <-done:
// 		case <-time.After(5 * time.Second):
// 			t.Log("Services did not shut down within 5 seconds")
// 		}
// 	}()

// 	// Wait for services to be ready
// 	waitForService(t, "http://localhost:8080/health", 10*time.Second)
// 	waitForService(t, "http://localhost:8081/health", 10*time.Second)

// 	// Run comprehensive tests
// 	t.Run("HealthChecks", testHealthChecks)
// 	t.Run("CSVUpload", testCSVUpload)
// 	t.Run("TelemetryQuery", testTelemetryQuery)
// 	t.Run("ServiceIntegration", testServiceIntegration)
// 	t.Run("ErrorHandling", testErrorHandling)
// 	t.Run("LoadTesting", testLoadTesting)
// }

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
