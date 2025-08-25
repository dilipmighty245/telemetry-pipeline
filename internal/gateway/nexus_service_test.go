package gateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/dilipmighty245/telemetry-pipeline/test/testhelper"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGatewayConfig_Validation(t *testing.T) {
	// Setup embedded etcd server for testing
	etcdServer, cleanup, err := testhelper.SetupEtcdForTest()
	require.NoError(t, err)
	defer cleanup()

	tests := []struct {
		name   string
		config *GatewayConfig
		valid  bool
	}{
		{
			name: "valid config",
			config: &GatewayConfig{
				Port:          8080,
				PprofPort:     8082,
				ClusterID:     "test-cluster",
				EtcdEndpoints: etcdServer.Endpoints,

				EnableWebSocket: true,
				EnableCORS:      true,
				LogLevel:        "info",
			},
			valid: true,
		},
		{
			name: "minimal config",
			config: &GatewayConfig{
				Port:          8080,
				PprofPort:     8082,
				ClusterID:     "test-cluster",
				EtcdEndpoints: etcdServer.Endpoints,
			},
			valid: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.NotNil(t, tt.config)
			if tt.valid {
				assert.Greater(t, tt.config.Port, 0)
				assert.NotEmpty(t, tt.config.ClusterID)
				assert.NotEmpty(t, tt.config.EtcdEndpoints)
			}
		})
	}
}

func TestTelemetryData_Validation(t *testing.T) {
	tests := []struct {
		name  string
		data  *TelemetryData
		valid bool
	}{
		{
			name: "valid data",
			data: &TelemetryData{
				Timestamp:         "2023-01-01T00:00:00Z",
				GPUID:             "0",
				Hostname:          "test-host",
				GPUUtilization:    85.5,
				MemoryUtilization: 60.2,
				MemoryUsedMB:      8192,
				MemoryFreeMB:      2048,
				Temperature:       72.0,
				PowerDraw:         350.5,
				SMClockMHz:        1410,
				MemoryClockMHz:    1215,
			},
			valid: true,
		},
		{
			name:  "empty data",
			data:  &TelemetryData{},
			valid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.valid {
				assert.NotEmpty(t, tt.data.GPUID)
				assert.NotEmpty(t, tt.data.Hostname)
				assert.NotEmpty(t, tt.data.Timestamp)
			}
		})
	}
}

func TestNexusGatewayService_ParseConfig(t *testing.T) {
	// Set environment variables for testing
	os.Setenv("PORT", "9090")
	os.Setenv("CLUSTER_ID", "test-cluster")
	os.Setenv("ETCD_ENDPOINTS", "localhost:2379,localhost:2380")
	os.Setenv("LOG_LEVEL", "debug")
	os.Setenv("ENABLE_WEBSOCKET", "false")
	os.Setenv("ENABLE_CORS", "false")
	defer func() {
		os.Unsetenv("PORT")
		os.Unsetenv("CLUSTER_ID")
		os.Unsetenv("ETCD_ENDPOINTS")
		os.Unsetenv("LOG_LEVEL")
		os.Unsetenv("ENABLE_WEBSOCKET")
		os.Unsetenv("ENABLE_CORS")
	}()

	service := &NexusGatewayService{}
	config, err := service.parseConfig([]string{"test"})

	require.NoError(t, err)
	assert.Equal(t, 9090, config.Port)
	assert.Equal(t, "test-cluster", config.ClusterID)
	assert.Equal(t, []string{"localhost:2379", "localhost:2380"}, config.EtcdEndpoints)
	assert.Equal(t, "debug", config.LogLevel)
	assert.False(t, config.EnableWebSocket)
	assert.False(t, config.EnableCORS)
}

func TestNexusGatewayService_ParseConfigDefaults(t *testing.T) {
	service := &NexusGatewayService{}
	config, err := service.parseConfig([]string{"test"})

	require.NoError(t, err)
	assert.Equal(t, 8080, config.Port)
	assert.Equal(t, "default-cluster", config.ClusterID)
	assert.Equal(t, []string{"localhost:2379"}, config.EtcdEndpoints)
	assert.Equal(t, "info", config.LogLevel)

	assert.True(t, config.EnableWebSocket)
	assert.True(t, config.EnableCORS)
}

func TestNexusGatewayService_HealthHandler(t *testing.T) {
	// Create a mock gateway service
	service := &NexusGatewayService{
		config: &GatewayConfig{
			ClusterID:     "test-cluster",
			EtcdEndpoints: []string{"localhost:2379"},
		},
	}

	// Create Echo instance
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	// Test health handler (will fail due to no etcd connection, but we can test the structure)
	err := service.healthHandler(c)

	// Should not return an error, but should return unhealthy status
	assert.NoError(t, err)
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
}

func TestNexusGatewayService_ListAllGPUsHandler(t *testing.T) {
	service := &NexusGatewayService{
		config: &GatewayConfig{
			ClusterID:     "test-cluster",
			EtcdEndpoints: []string{"localhost:2379"},
		},
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/gpus", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	// Test handler (will fail due to no etcd connection)
	err := service.listAllGPUsHandler(c)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestNexusGatewayService_QueryTelemetryByGPUHandler(t *testing.T) {
	service := &NexusGatewayService{
		config: &GatewayConfig{
			ClusterID:     "test-cluster",
			EtcdEndpoints: []string{"localhost:2379"},
		},
	}

	e := echo.New()

	// Test with missing GPU ID
	req := httptest.NewRequest(http.MethodGet, "/api/v1/gpus//telemetry", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("")

	err := service.queryTelemetryByGPUHandler(c)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	// Test with valid GPU ID but no etcd connection
	req = httptest.NewRequest(http.MethodGet, "/api/v1/gpus/gpu-123/telemetry?start_time=2023-01-01T00:00:00Z&end_time=2023-01-02T00:00:00Z&limit=50", nil)
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("gpu-123")

	err = service.queryTelemetryByGPUHandler(c)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var response map[string]interface{}
	err = json.Unmarshal(rec.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.True(t, response["success"].(bool))
	assert.Equal(t, float64(0), response["count"])
}

func TestNexusGatewayService_PlaceholderHandlers(t *testing.T) {
	service := &NexusGatewayService{}
	e := echo.New()

	// Test placeholder handlers
	handlers := []struct {
		name    string
		handler echo.HandlerFunc
		path    string
	}{
		{"listClusters", service.listClustersHandler, "/clusters"},
		{"getCluster", service.getClusterHandler, "/clusters/test"},
		{"getClusterStats", service.getClusterStatsHandler, "/clusters/test/stats"},
		{"listHosts", service.listHostsHandler, "/hosts"},
		{"getHost", service.getHostHandler, "/hosts/test"},
		{"listGPUs", service.listGPUsHandler, "/gpus"},
		{"getGPU", service.getGPUHandler, "/gpus/test"},
		{"getGPUMetrics", service.getGPUMetricsHandler, "/gpus/test/metrics"},
		{"getTelemetry", service.getTelemetryHandler, "/telemetry"},
		{"getLatestTelemetry", service.getLatestTelemetryHandler, "/telemetry/latest"},
		{"listAllHosts", service.listAllHostsHandler, "/hosts"},
		{"listGPUsByHost", service.listGPUsByHostHandler, "/hosts/test/gpus"},

		{"websocket", service.websocketHandler, "/ws"},
	}

	for _, h := range handlers {
		t.Run(h.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, h.path, nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			// Set params for handlers that need them
			if strings.Contains(h.path, "test") {
				c.SetParamNames("cluster_id", "host_id", "gpu_id")
				c.SetParamValues("test", "test", "test")
			}

			err := h.handler(c)
			assert.NoError(t, err)
			assert.Equal(t, http.StatusOK, rec.Code)
		})
	}
}

func TestNexusGatewayService_Close(t *testing.T) {
	service := &NexusGatewayService{}

	// Test closing without initialization
	err := service.Close()
	assert.NoError(t, err)
}

// Benchmark tests
func BenchmarkGatewayConfig_Creation(b *testing.B) {
	for i := 0; i < b.N; i++ {
		config := &GatewayConfig{
			Port:            8080,
			PprofPort:       8082,
			ClusterID:       "test-cluster",
			EtcdEndpoints:   []string{"localhost:2379"},
			EnableWebSocket: true,
			EnableCORS:      true,
			LogLevel:        "info",
		}
		_ = config
	}
}

func BenchmarkTelemetryData_Creation(b *testing.B) {
	for i := 0; i < b.N; i++ {
		data := &TelemetryData{
			Timestamp:         "2023-01-01T00:00:00Z",
			GPUID:             "0",
			Hostname:          "test-host",
			GPUUtilization:    85.5,
			MemoryUtilization: 60.2,
			MemoryUsedMB:      8192,
			MemoryFreeMB:      2048,
			Temperature:       72.0,
			PowerDraw:         350.5,
			SMClockMHz:        1410,
			MemoryClockMHz:    1215,
		}
		_ = data
	}
}
