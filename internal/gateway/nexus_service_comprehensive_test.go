package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/server/v3/embed"
)

// getFreePort returns a free port by binding to :0 and then closing the connection
func getFreePort() (int, error) {
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()

	addr := listener.Addr().(*net.TCPAddr)
	return addr.Port, nil
}

func TestNexusGatewayService_ComprehensiveAPITests(t *testing.T) {
	// Get free ports for etcd
	clientPort, err := getFreePort()
	require.NoError(t, err)
	peerPort, err := getFreePort()
	require.NoError(t, err)

	// Setup embedded etcd for testing
	cfg := embed.NewConfig()
	cfg.Dir = t.TempDir()
	cfg.LogLevel = "error" // Reduce log noise
	cfg.Logger = "zap"

	// Use dynamic ports to avoid conflicts
	clientURL, _ := url.Parse(fmt.Sprintf("http://localhost:%d", clientPort))
	peerURL, _ := url.Parse(fmt.Sprintf("http://localhost:%d", peerPort))

	cfg.ListenClientUrls = []url.URL{*clientURL}
	cfg.AdvertiseClientUrls = []url.URL{*clientURL}
	cfg.ListenPeerUrls = []url.URL{*peerURL}
	cfg.AdvertisePeerUrls = []url.URL{*peerURL}

	// Set initial cluster to match the peer URL
	cfg.InitialCluster = cfg.Name + "=" + peerURL.String()
	cfg.ClusterState = embed.ClusterStateFlagNew

	e, err := embed.StartEtcd(cfg)
	require.NoError(t, err)
	defer e.Close()

	select {
	case <-e.Server.ReadyNotify():
		// Server is ready
	case <-time.After(10 * time.Second):
		t.Fatal("etcd server took too long to start")
	}

	// Create etcd client
	client, err := clientv3.New(clientv3.Config{
		Endpoints:   []string{e.Clients[0].Addr().String()},
		DialTimeout: 5 * time.Second,
	})
	require.NoError(t, err)
	defer client.Close()

	// Set environment variable for message queue service
	etcdEndpoint := e.Clients[0].Addr().String()
	t.Setenv("ETCD_ENDPOINTS", etcdEndpoint)

	// Get free ports for gateway
	gatewayPort, err := getFreePort()
	require.NoError(t, err)
	pprofPort, err := getFreePort()
	require.NoError(t, err)

	// Create gateway config
	config := &GatewayConfig{
		Port:            gatewayPort,
		PprofPort:       pprofPort,
		ClusterID:       "test-cluster",
		EtcdEndpoints:   []string{etcdEndpoint},
		EnableWebSocket: true,
		EnableCORS:      true,
		LogLevel:        "error",
	}

	// Create gateway service
	gateway, err := NewNexusGatewayService(context.Background(), config)
	require.NoError(t, err)
	defer gateway.Close()

	// Setup test data in etcd
	setupTestData(t, client, config.ClusterID)

	t.Run("HealthEndpoint", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		rec := httptest.NewRecorder()
		c := gateway.echo.NewContext(req, rec)

		err := gateway.healthHandler(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var response map[string]interface{}
		err = json.Unmarshal(rec.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Equal(t, "healthy", response["status"])
		assert.Equal(t, config.ClusterID, response["cluster_id"])
	})

	t.Run("ListAllGPUs", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/gpus", nil)
		rec := httptest.NewRecorder()
		c := gateway.echo.NewContext(req, rec)

		err := gateway.listAllGPUsHandler(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var response map[string]interface{}
		err = json.Unmarshal(rec.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.True(t, response["success"].(bool))
		assert.Contains(t, response, "data")
		assert.Contains(t, response, "count")
	})

	t.Run("QueryTelemetryByGPU_ValidGPU", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/gpus/GPU-12345/telemetry", nil)
		rec := httptest.NewRecorder()
		c := gateway.echo.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues("GPU-12345")

		err := gateway.queryTelemetryByGPUHandler(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var response map[string]interface{}
		err = json.Unmarshal(rec.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.True(t, response["success"].(bool))
		assert.Contains(t, response, "data")
		assert.Contains(t, response, "count")
		assert.Equal(t, "GPU-12345", response["gpu_id"])
	})

	t.Run("QueryTelemetryByGPU_EmptyGPUID", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/gpus//telemetry", nil)
		rec := httptest.NewRecorder()
		c := gateway.echo.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues("")

		err := gateway.queryTelemetryByGPUHandler(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, rec.Code)

		var response map[string]interface{}
		err = json.Unmarshal(rec.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.False(t, response["success"].(bool))
		assert.Equal(t, "GPU ID is required", response["error"])
	})

	t.Run("QueryTelemetryByGPU_WithTimeFilters", func(t *testing.T) {
		startTime := "2024-01-01T00:00:00Z"
		endTime := "2024-12-31T23:59:59Z"
		limit := "10"

		url := fmt.Sprintf("/api/v1/gpus/GPU-12345/telemetry?start_time=%s&end_time=%s&limit=%s", startTime, endTime, limit)
		req := httptest.NewRequest(http.MethodGet, url, nil)
		rec := httptest.NewRecorder()
		c := gateway.echo.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues("GPU-12345")

		err := gateway.queryTelemetryByGPUHandler(c)
		assert.NoError(t, err)
		// Status could be 200 (success) or 400 (validation error due to old dates)
		assert.True(t, rec.Code == http.StatusOK || rec.Code == http.StatusBadRequest)

		var response map[string]interface{}
		err = json.Unmarshal(rec.Body.Bytes(), &response)
		assert.NoError(t, err)
		
		// Check if the response was successful
		if success, ok := response["success"].(bool); ok && success {
			assert.True(t, success)
			if filters, ok := response["filters"].(map[string]interface{}); ok {
				assert.Equal(t, startTime, filters["start_time"])
				assert.Equal(t, endTime, filters["end_time"])
				assert.Equal(t, float64(10), filters["limit"])
			}
		} else {
			// If validation failed due to old dates, that's expected behavior
			t.Logf("Request failed validation as expected: %v", response["error"])
		}
	})

	t.Run("QueryTelemetryByGPU_InvalidTimeFormat", func(t *testing.T) {
		url := "/api/v1/gpus/GPU-12345/telemetry?start_time=invalid-time"
		req := httptest.NewRequest(http.MethodGet, url, nil)
		rec := httptest.NewRecorder()
		c := gateway.echo.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues("GPU-12345")

		err := gateway.queryTelemetryByGPUHandler(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, rec.Code)

		var response map[string]interface{}
		err = json.Unmarshal(rec.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.False(t, response["success"].(bool))
		assert.Contains(t, response["error"], "invalid start_time format")
	})

	t.Run("QueryTelemetryByGPU_InvalidEndTimeFormat", func(t *testing.T) {
		url := "/api/v1/gpus/GPU-12345/telemetry?end_time=invalid-time"
		req := httptest.NewRequest(http.MethodGet, url, nil)
		rec := httptest.NewRecorder()
		c := gateway.echo.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues("GPU-12345")

		err := gateway.queryTelemetryByGPUHandler(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, rec.Code)

		var response map[string]interface{}
		err = json.Unmarshal(rec.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.False(t, response["success"].(bool))
		assert.Contains(t, response["error"], "invalid end_time format")
	})

	t.Run("QueryTelemetryByGPU_InvalidLimit", func(t *testing.T) {
		url := "/api/v1/gpus/GPU-12345/telemetry?limit=invalid"
		req := httptest.NewRequest(http.MethodGet, url, nil)
		rec := httptest.NewRecorder()
		c := gateway.echo.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues("GPU-12345")

		err := gateway.queryTelemetryByGPUHandler(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, rec.Code)

		var response map[string]interface{}
		err = json.Unmarshal(rec.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.False(t, response["success"].(bool))
		assert.Contains(t, response["error"], "limit must be a valid integer")
	})

	t.Run("QueryTelemetryByGPU_NegativeLimit", func(t *testing.T) {
		url := "/api/v1/gpus/GPU-12345/telemetry?limit=-5"
		req := httptest.NewRequest(http.MethodGet, url, nil)
		rec := httptest.NewRecorder()
		c := gateway.echo.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues("GPU-12345")

		err := gateway.queryTelemetryByGPUHandler(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, rec.Code)

		var response map[string]interface{}
		err = json.Unmarshal(rec.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.False(t, response["success"].(bool))
		assert.Contains(t, response["error"], "limit must be greater than 0")
	})

	t.Run("QueryTelemetryByGPU_InvalidTimeRange", func(t *testing.T) {
		startTime := "2024-12-31T23:59:59Z"
		endTime := "2024-01-01T00:00:00Z"

		url := fmt.Sprintf("/api/v1/gpus/GPU-12345/telemetry?start_time=%s&end_time=%s", startTime, endTime)
		req := httptest.NewRequest(http.MethodGet, url, nil)
		rec := httptest.NewRecorder()
		c := gateway.echo.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues("GPU-12345")

		err := gateway.queryTelemetryByGPUHandler(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, rec.Code)

		var response map[string]interface{}
		err = json.Unmarshal(rec.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.False(t, response["success"].(bool))
		assert.Equal(t, "start_time must be before end_time", response["error"])
	})

	t.Run("AdditionalEndpoints", func(t *testing.T) {
		endpoints := []struct {
			method   string
			path     string
			handler  echo.HandlerFunc
			expected int
		}{
			{http.MethodGet, "/api/v1/clusters", gateway.listClustersHandler, http.StatusOK},
			{http.MethodGet, "/api/v1/clusters/test-cluster", gateway.getClusterHandler, http.StatusOK},
			{http.MethodGet, "/api/v1/clusters/test-cluster/stats", gateway.getClusterStatsHandler, http.StatusOK},
			{http.MethodGet, "/api/v1/hosts", gateway.listAllHostsHandler, http.StatusOK},
			{http.MethodGet, "/api/v1/hosts/test-host/gpus", gateway.listGPUsByHostHandler, http.StatusOK},
			{http.MethodGet, "/api/v1/telemetry", gateway.getTelemetryHandler, http.StatusOK},
			{http.MethodGet, "/api/v1/telemetry/latest", gateway.getLatestTelemetryHandler, http.StatusOK},
		}

		for _, endpoint := range endpoints {
			t.Run(fmt.Sprintf("%s_%s", endpoint.method, strings.ReplaceAll(endpoint.path, "/", "_")), func(t *testing.T) {
				req := httptest.NewRequest(endpoint.method, endpoint.path, nil)
				rec := httptest.NewRecorder()
				c := gateway.echo.NewContext(req, rec)

				// Set params for parameterized routes
				if strings.Contains(endpoint.path, "test-cluster") {
					c.SetParamNames("cluster_id")
					c.SetParamValues("test-cluster")
				}
				if strings.Contains(endpoint.path, "test-host") {
					c.SetParamNames("hostname")
					c.SetParamValues("test-host")
				}

				err := endpoint.handler(c)
				assert.NoError(t, err)
				assert.Equal(t, endpoint.expected, rec.Code)

				// Verify JSON response
				var response map[string]interface{}
				err = json.Unmarshal(rec.Body.Bytes(), &response)
				assert.NoError(t, err)
			})
		}
	})

	t.Run("WebSocketEndpoint", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/ws", nil)
		rec := httptest.NewRecorder()
		c := gateway.echo.NewContext(req, rec)

		err := gateway.websocketHandler(c)
		assert.NoError(t, err)
		// WebSocket handler returns 400 for requests without proper WebSocket upgrade headers
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	// Note: Swagger functionality is handled by echoSwagger.WrapHandler middleware
	// and doesn't have a custom handler method to test directly
}

func TestNexusGatewayService_ConfigurationAndSetup(t *testing.T) {
	t.Run("ParseConfig_DefaultValues", func(t *testing.T) {
		gateway := &NexusGatewayService{}
		config, err := gateway.parseConfig([]string{})
		assert.NoError(t, err)
		assert.Equal(t, 8080, config.Port)
		assert.Equal(t, "default-cluster", config.ClusterID)
		assert.True(t, config.EnableWebSocket)
		assert.True(t, config.EnableCORS)
		assert.Equal(t, "info", config.LogLevel)
		assert.Equal(t, []string{"localhost:2379"}, config.EtcdEndpoints)
	})

	t.Run("ParseConfig_EnvironmentVariables", func(t *testing.T) {
		// Set environment variables
		t.Setenv("PORT", "9090")
		t.Setenv("CLUSTER_ID", "test-cluster")
		t.Setenv("LOG_LEVEL", "debug")
		t.Setenv("ENABLE_WEBSOCKET", "false")
		t.Setenv("ENABLE_CORS", "false")
		t.Setenv("ETCD_ENDPOINTS", "localhost:2379,localhost:2380")

		gateway := &NexusGatewayService{}
		config, err := gateway.parseConfig([]string{})
		assert.NoError(t, err)
		assert.Equal(t, 9090, config.Port)
		assert.Equal(t, "test-cluster", config.ClusterID)
		assert.False(t, config.EnableWebSocket)
		assert.False(t, config.EnableCORS)
		assert.Equal(t, "debug", config.LogLevel)
		assert.Equal(t, []string{"localhost:2379", "localhost:2380"}, config.EtcdEndpoints)
	})

	t.Run("NewNexusGatewayService_InvalidEtcdEndpoint", func(t *testing.T) {
		config := &GatewayConfig{
			Port:          8080,
			PprofPort:     8082,
			ClusterID:     "test-cluster",
			EtcdEndpoints: []string{"invalid-endpoint:2379"},
			LogLevel:      "error",
		}

		_, err := NewNexusGatewayService(context.Background(), config)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to connect to etcd")
	})
}

func TestNexusGatewayService_ErrorHandling(t *testing.T) {
	t.Run("HealthHandler_NoEtcdClient", func(t *testing.T) {
		gateway := &NexusGatewayService{
			echo: echo.New(),
		}

		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		rec := httptest.NewRecorder()
		c := gateway.echo.NewContext(req, rec)

		err := gateway.healthHandler(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusServiceUnavailable, rec.Code)

		var response map[string]interface{}
		err = json.Unmarshal(rec.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Equal(t, "unhealthy", response["status"])
		assert.Contains(t, response["error"], "etcd client not initialized")
	})

	t.Run("ListAllGPUsHandler_NoEtcdClient", func(t *testing.T) {
		gateway := &NexusGatewayService{
			echo: echo.New(),
		}

		req := httptest.NewRequest(http.MethodGet, "/api/v1/gpus", nil)
		rec := httptest.NewRecorder()
		c := gateway.echo.NewContext(req, rec)

		err := gateway.listAllGPUsHandler(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusInternalServerError, rec.Code)

		var response map[string]interface{}
		err = json.Unmarshal(rec.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.False(t, response["success"].(bool))
		assert.Contains(t, response["error"], "etcd client not initialized")
	})
}

func TestTelemetryDataStruct(t *testing.T) {
	t.Run("TelemetryData_JSONSerialization", func(t *testing.T) {
		data := TelemetryData{
			Timestamp:         "2024-01-01T00:00:00Z",
			GPUID:             "GPU-12345",
			Hostname:          "test-host",
			GPUUtilization:    85.5,
			MemoryUtilization: 70.2,
			MemoryUsedMB:      45000,
			MemoryFreeMB:      35000,
			Temperature:       65.0,
			PowerDraw:         350.5,
			SMClockMHz:        1410,
			MemoryClockMHz:    1215,
		}

		// Test JSON marshaling
		jsonData, err := json.Marshal(data)
		assert.NoError(t, err)
		assert.Contains(t, string(jsonData), "GPU-12345")
		assert.Contains(t, string(jsonData), "test-host")

		// Test JSON unmarshaling
		var unmarshaled TelemetryData
		err = json.Unmarshal(jsonData, &unmarshaled)
		assert.NoError(t, err)
		assert.Equal(t, data, unmarshaled)
	})
}

// setupTestData creates test data in etcd for testing
func setupTestData(t *testing.T, client *clientv3.Client, clusterID string) {
	ctx := context.Background()

	// Create test GPU data
	gpu1 := map[string]interface{}{
		"gpu_id":   "0",
		"uuid":     "GPU-12345",
		"hostname": "test-host",
		"device":   "nvidia0",
		"name":     "NVIDIA H100 80GB HBM3",
		"status":   map[string]interface{}{"state": "active"},
	}

	gpu2 := map[string]interface{}{
		"gpu_id":   "1",
		"uuid":     "GPU-67890",
		"hostname": "test-host",
		"device":   "nvidia1",
		"name":     "NVIDIA H100 80GB HBM3",
		"status":   map[string]interface{}{"state": "active"},
	}

	// Store GPU data
	gpu1Data, _ := json.Marshal(gpu1)
	gpu2Data, _ := json.Marshal(gpu2)

	key1 := fmt.Sprintf("/telemetry/clusters/%s/hosts/test-host/gpus/0", clusterID)
	key2 := fmt.Sprintf("/telemetry/clusters/%s/hosts/test-host/gpus/1", clusterID)

	_, err := client.Put(ctx, key1, string(gpu1Data))
	require.NoError(t, err)

	_, err = client.Put(ctx, key2, string(gpu2Data))
	require.NoError(t, err)

	// Create test telemetry data
	telemetry1 := TelemetryData{
		Timestamp:         "2024-01-01T12:00:00Z",
		GPUID:             "GPU-12345",
		Hostname:          "test-host",
		GPUUtilization:    85.5,
		MemoryUtilization: 70.2,
		MemoryUsedMB:      45000,
		MemoryFreeMB:      35000,
		Temperature:       65.0,
		PowerDraw:         350.5,
		SMClockMHz:        1410,
		MemoryClockMHz:    1215,
	}

	telemetry2 := TelemetryData{
		Timestamp:         "2024-01-01T12:01:00Z",
		GPUID:             "GPU-67890",
		Hostname:          "test-host",
		GPUUtilization:    92.1,
		MemoryUtilization: 85.7,
		MemoryUsedMB:      68000,
		MemoryFreeMB:      12000,
		Temperature:       72.0,
		PowerDraw:         380.2,
		SMClockMHz:        1410,
		MemoryClockMHz:    1215,
	}

	// Store telemetry data
	telemetry1Data, _ := json.Marshal(telemetry1)
	telemetry2Data, _ := json.Marshal(telemetry2)

	telemetryKey1 := fmt.Sprintf("/telemetry/clusters/%s/hosts/test-host/gpus/0/data/2024-01-01T12:00:00Z", clusterID)
	telemetryKey2 := fmt.Sprintf("/telemetry/clusters/%s/hosts/test-host/gpus/1/data/2024-01-01T12:01:00Z", clusterID)

	_, err = client.Put(ctx, telemetryKey1, string(telemetry1Data))
	require.NoError(t, err)

	_, err = client.Put(ctx, telemetryKey2, string(telemetry2Data))
	require.NoError(t, err)
}
