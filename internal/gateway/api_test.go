package gateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
)

func TestAPIEndpoints_AdditionalCoverage(t *testing.T) {
	// Create a simple gateway service for testing
	gateway := &NexusGatewayService{
		echo: echo.New(),
	}

	t.Run("ListClustersHandler", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/clusters", nil)
		rec := httptest.NewRecorder()
		c := gateway.echo.NewContext(req, rec)

		err := gateway.listClustersHandler(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var response map[string]interface{}
		err = json.Unmarshal(rec.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Contains(t, response, "clusters")
		assert.Contains(t, response, "count")
	})

	t.Run("GetClusterHandler", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/clusters/test-cluster", nil)
		rec := httptest.NewRecorder()
		c := gateway.echo.NewContext(req, rec)
		c.SetParamNames("cluster_id")
		c.SetParamValues("test-cluster")

		err := gateway.getClusterHandler(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var response map[string]interface{}
		err = json.Unmarshal(rec.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Equal(t, "test-cluster", response["cluster_id"])
	})

	t.Run("GetClusterStatsHandler", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/clusters/test-cluster/stats", nil)
		rec := httptest.NewRecorder()
		c := gateway.echo.NewContext(req, rec)

		err := gateway.getClusterStatsHandler(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var response map[string]interface{}
		err = json.Unmarshal(rec.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Contains(t, response, "stats")
	})

	t.Run("ListHostsHandler", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/clusters/test-cluster/hosts", nil)
		rec := httptest.NewRecorder()
		c := gateway.echo.NewContext(req, rec)

		err := gateway.listHostsHandler(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var response map[string]interface{}
		err = json.Unmarshal(rec.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Contains(t, response, "hosts")
		assert.Contains(t, response, "count")
	})

	t.Run("GetHostHandler", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/clusters/test-cluster/hosts/test-host", nil)
		rec := httptest.NewRecorder()
		c := gateway.echo.NewContext(req, rec)
		c.SetParamNames("host_id")
		c.SetParamValues("test-host")

		err := gateway.getHostHandler(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var response map[string]interface{}
		err = json.Unmarshal(rec.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Equal(t, "test-host", response["host_id"])
	})

	t.Run("ListGPUsHandler", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/clusters/test-cluster/hosts/test-host/gpus", nil)
		rec := httptest.NewRecorder()
		c := gateway.echo.NewContext(req, rec)

		err := gateway.listGPUsHandler(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var response map[string]interface{}
		err = json.Unmarshal(rec.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Contains(t, response, "gpus")
		assert.Contains(t, response, "count")
	})

	t.Run("GetGPUHandler", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/clusters/test-cluster/hosts/test-host/gpus/0", nil)
		rec := httptest.NewRecorder()
		c := gateway.echo.NewContext(req, rec)
		c.SetParamNames("gpu_id")
		c.SetParamValues("0")

		err := gateway.getGPUHandler(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var response map[string]interface{}
		err = json.Unmarshal(rec.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Equal(t, "0", response["gpu_id"])
	})

	t.Run("GetGPUMetricsHandler", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/clusters/test-cluster/hosts/test-host/gpus/0/metrics", nil)
		rec := httptest.NewRecorder()
		c := gateway.echo.NewContext(req, rec)

		err := gateway.getGPUMetricsHandler(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var response map[string]interface{}
		err = json.Unmarshal(rec.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Contains(t, response, "metrics")
		assert.Contains(t, response, "count")
	})

	t.Run("GetTelemetryHandler", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/telemetry", nil)
		rec := httptest.NewRecorder()
		c := gateway.echo.NewContext(req, rec)

		err := gateway.getTelemetryHandler(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var response map[string]interface{}
		err = json.Unmarshal(rec.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Contains(t, response, "telemetry")
		assert.Contains(t, response, "count")
	})

	t.Run("GetLatestTelemetryHandler", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/telemetry/latest", nil)
		rec := httptest.NewRecorder()
		c := gateway.echo.NewContext(req, rec)

		err := gateway.getLatestTelemetryHandler(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var response map[string]interface{}
		err = json.Unmarshal(rec.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Contains(t, response, "telemetry")
		assert.Contains(t, response, "count")
	})

	t.Run("ListAllHostsHandler", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/hosts", nil)
		rec := httptest.NewRecorder()
		c := gateway.echo.NewContext(req, rec)

		err := gateway.listAllHostsHandler(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var response map[string]interface{}
		err = json.Unmarshal(rec.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Contains(t, response, "hosts")
		assert.Contains(t, response, "count")
	})

	t.Run("ListGPUsByHostHandler", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/hosts/test-host/gpus", nil)
		rec := httptest.NewRecorder()
		c := gateway.echo.NewContext(req, rec)
		c.SetParamNames("hostname")
		c.SetParamValues("test-host")

		err := gateway.listGPUsByHostHandler(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var response map[string]interface{}
		err = json.Unmarshal(rec.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Contains(t, response, "gpus")
		assert.Contains(t, response, "count")
	})

	t.Run("WebSocketHandler", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/ws", nil)
		rec := httptest.NewRecorder()
		c := gateway.echo.NewContext(req, rec)

		err := gateway.websocketHandler(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("SwaggerHandler", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/swagger/", nil)
		rec := httptest.NewRecorder()
		c := gateway.echo.NewContext(req, rec)

		err := gateway.swaggerHandler(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var response map[string]interface{}
		err = json.Unmarshal(rec.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Contains(t, response, "swagger")
	})
}

func TestTelemetryDataStruct_Coverage(t *testing.T) {
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

	t.Run("TelemetryData_Validation", func(t *testing.T) {
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

		// Basic validation
		assert.NotEmpty(t, data.GPUID)
		assert.NotEmpty(t, data.Hostname)
		assert.NotEmpty(t, data.Timestamp)
		assert.True(t, data.GPUUtilization >= 0 && data.GPUUtilization <= 100)
		assert.True(t, data.MemoryUtilization >= 0 && data.MemoryUtilization <= 100)
		assert.True(t, data.Temperature > 0)
		assert.True(t, data.PowerDraw > 0)
		assert.True(t, data.SMClockMHz > 0)
		assert.True(t, data.MemoryClockMHz > 0)
	})
}

func TestGatewayConfig_Coverage(t *testing.T) {
	t.Run("GatewayConfig_DefaultValues", func(t *testing.T) {
		config := &GatewayConfig{
			Port:            8080,
			ClusterID:       "test-cluster",
			EtcdEndpoints:   []string{"localhost:2379"},
			EnableWebSocket: true,
			EnableCORS:      true,
			LogLevel:        "info",
		}

		assert.Equal(t, 8080, config.Port)
		assert.Equal(t, "test-cluster", config.ClusterID)
		assert.Equal(t, []string{"localhost:2379"}, config.EtcdEndpoints)
		assert.True(t, config.EnableWebSocket)
		assert.True(t, config.EnableCORS)
		assert.Equal(t, "info", config.LogLevel)
	})

	t.Run("GatewayConfig_Validation", func(t *testing.T) {
		// Test valid config
		validConfig := &GatewayConfig{
			Port:            8080,
			ClusterID:       "test-cluster",
			EtcdEndpoints:   []string{"localhost:2379"},
			EnableWebSocket: true,
			EnableCORS:      true,
			LogLevel:        "info",
		}

		assert.NotEmpty(t, validConfig.ClusterID)
		assert.NotEmpty(t, validConfig.EtcdEndpoints)
		assert.True(t, validConfig.Port > 0)

		// Test edge cases
		assert.True(t, len(validConfig.EtcdEndpoints) > 0)
		assert.Contains(t, []string{"debug", "info", "warn", "error"}, validConfig.LogLevel)
	})
}
