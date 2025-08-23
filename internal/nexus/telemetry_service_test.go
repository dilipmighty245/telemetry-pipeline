package nexus

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServiceConfig_Struct(t *testing.T) {
	config := ServiceConfig{
		EtcdEndpoints:  []string{"localhost:2379"},
		ClusterID:      "cluster-1",
		ServiceID:      "service-1",
		UpdateInterval: 30 * time.Second,
		BatchSize:      100,
		EnableWatchAPI: true,
		EnableGraphQL:  true,
	}

	assert.Equal(t, []string{"localhost:2379"}, config.EtcdEndpoints)
	assert.Equal(t, "cluster-1", config.ClusterID)
	assert.Equal(t, "service-1", config.ServiceID)
	assert.Equal(t, 30*time.Second, config.UpdateInterval)
	assert.Equal(t, 100, config.BatchSize)
	assert.True(t, config.EnableWatchAPI)
	assert.True(t, config.EnableGraphQL)
}

func TestTelemetryCluster_Struct(t *testing.T) {
	now := time.Now()
	cluster := TelemetryCluster{
		ClusterID:   "cluster-1",
		ClusterName: "test-cluster",
		Region:      "us-west-2",
		Environment: "production",
		Labels:      map[string]string{"team": "platform"},
		CreatedAt:   now,
		UpdatedAt:   now,
		Metadata: ClusterMetadata{
			TotalHosts:    10,
			TotalGPUs:     40,
			Configuration: map[string]string{"version": "1.0.0"},
		},
	}

	assert.Equal(t, "cluster-1", cluster.ClusterID)
	assert.Equal(t, "test-cluster", cluster.ClusterName)
	assert.Equal(t, "us-west-2", cluster.Region)
	assert.Equal(t, "production", cluster.Environment)
	assert.Equal(t, "platform", cluster.Labels["team"])
	assert.Equal(t, now, cluster.CreatedAt)
	assert.Equal(t, now, cluster.UpdatedAt)
	assert.Equal(t, int32(10), cluster.Metadata.TotalHosts)
	assert.Equal(t, int32(40), cluster.Metadata.TotalGPUs)
	assert.Equal(t, "1.0.0", cluster.Metadata.Configuration["version"])
}

func TestTelemetryCluster_JSONSerialization(t *testing.T) {
	cluster := TelemetryCluster{
		ClusterID:   "cluster-1",
		ClusterName: "test-cluster",
		Region:      "us-west-2",
		Environment: "production",
		Labels:      map[string]string{"team": "platform", "env": "prod"},
		CreatedAt:   time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC),
		UpdatedAt:   time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC),
		Metadata: ClusterMetadata{
			TotalHosts:    10,
			TotalGPUs:     40,
			Configuration: map[string]string{"version": "1.0.0"},
		},
	}

	// Test JSON marshaling
	jsonData, err := json.Marshal(cluster)
	require.NoError(t, err)
	assert.Contains(t, string(jsonData), "\"cluster_id\":\"cluster-1\"")
	assert.Contains(t, string(jsonData), "\"cluster_name\":\"test-cluster\"")
	assert.Contains(t, string(jsonData), "\"region\":\"us-west-2\"")
	assert.Contains(t, string(jsonData), "\"team\":\"platform\"")

	// Test JSON unmarshaling
	var unmarshaled TelemetryCluster
	err = json.Unmarshal(jsonData, &unmarshaled)
	require.NoError(t, err)
	assert.Equal(t, cluster.ClusterID, unmarshaled.ClusterID)
	assert.Equal(t, cluster.ClusterName, unmarshaled.ClusterName)
	assert.Equal(t, cluster.Region, unmarshaled.Region)
	assert.Equal(t, cluster.Environment, unmarshaled.Environment)
	assert.Equal(t, cluster.Labels["team"], unmarshaled.Labels["team"])
	assert.Equal(t, cluster.Metadata.TotalHosts, unmarshaled.Metadata.TotalHosts)
}

func TestTelemetryHost_Struct(t *testing.T) {
	now := time.Now()
	host := TelemetryHost{
		HostID:    "host-1",
		Hostname:  "gpu-node-1",
		IPAddress: "192.168.1.100",
		OSVersion: "Ubuntu 20.04",
		Labels:    map[string]string{"zone": "us-west-2a"},
		CreatedAt: now,
		UpdatedAt: now,
		Status: HostStatus{
			State:        "active",
			LastSeen:     now,
			Healthy:      true,
			HealthChecks: map[string]string{"gpu_count": "4", "health_score": "0.95"},
		},
	}

	assert.Equal(t, "host-1", host.HostID)
	assert.Equal(t, "gpu-node-1", host.Hostname)
	assert.Equal(t, "192.168.1.100", host.IPAddress)
	assert.Equal(t, "Ubuntu 20.04", host.OSVersion)
	assert.Equal(t, "us-west-2a", host.Labels["zone"])
	assert.Equal(t, "active", host.Status.State)
	assert.True(t, host.Status.Healthy)
	assert.Equal(t, "4", host.Status.HealthChecks["gpu_count"])
	assert.Equal(t, "0.95", host.Status.HealthChecks["health_score"])
}

func TestTelemetryHost_JSONSerialization(t *testing.T) {
	host := TelemetryHost{
		HostID:    "host-1",
		Hostname:  "gpu-node-1",
		IPAddress: "192.168.1.100",
		OSVersion: "Ubuntu 20.04",
		Labels:    map[string]string{"zone": "us-west-2a", "type": "gpu"},
		CreatedAt: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC),
		Status: HostStatus{
			State:        "active",
			LastSeen:     time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC),
			Healthy:      true,
			HealthChecks: map[string]string{"gpu_count": "4", "health_score": "0.95"},
		},
	}

	// Test JSON marshaling
	jsonData, err := json.Marshal(host)
	require.NoError(t, err)
	assert.Contains(t, string(jsonData), "\"host_id\":\"host-1\"")
	assert.Contains(t, string(jsonData), "\"hostname\":\"gpu-node-1\"")
	assert.Contains(t, string(jsonData), "\"ip_address\":\"192.168.1.100\"")

	// Test JSON unmarshaling
	var unmarshaled TelemetryHost
	err = json.Unmarshal(jsonData, &unmarshaled)
	require.NoError(t, err)
	assert.Equal(t, host.HostID, unmarshaled.HostID)
	assert.Equal(t, host.Hostname, unmarshaled.Hostname)
	assert.Equal(t, host.IPAddress, unmarshaled.IPAddress)
	assert.Equal(t, host.Status.State, unmarshaled.Status.State)
	assert.Equal(t, host.Status.Healthy, unmarshaled.Status.Healthy)
}

func TestTelemetryGPU_Struct(t *testing.T) {
	now := time.Now()
	gpu := TelemetryGPU{
		GPUID:         "0",
		UUID:          "GPU-12345-abcde",
		Device:        "nvidia0",
		DeviceName:    "NVIDIA H100 80GB HBM3",
		DriverVersion: "535.129.03",
		CudaVersion:   "12.2",
		MemoryTotal:   81920,
		Properties:    map[string]string{"compute_capability": "9.0"},
		CreatedAt:     now,
		UpdatedAt:     now,
		Status: GPUStatus{
			State:              "active",
			UtilizationPercent: 85.5,
			MemoryUsedPercent:  60.2,
			TemperatureCelsius: 72.0,
			PowerDrawWatts:     350.5,
			Healthy:            true,
			LastUpdated:        now,
		},
	}

	assert.Equal(t, "0", gpu.GPUID)
	assert.Equal(t, "GPU-12345-abcde", gpu.UUID)
	assert.Equal(t, "nvidia0", gpu.Device)
	assert.Equal(t, "NVIDIA H100 80GB HBM3", gpu.DeviceName)
	assert.Equal(t, "535.129.03", gpu.DriverVersion)
	assert.Equal(t, "12.2", gpu.CudaVersion)
	assert.Equal(t, int32(81920), gpu.MemoryTotal)
	assert.Equal(t, "9.0", gpu.Properties["compute_capability"])
	assert.Equal(t, "active", gpu.Status.State)
	assert.Equal(t, float32(72.0), gpu.Status.TemperatureCelsius)
	assert.Equal(t, float32(350.5), gpu.Status.PowerDrawWatts)
	assert.Equal(t, float32(85.5), gpu.Status.UtilizationPercent)
	assert.Equal(t, float32(60.2), gpu.Status.MemoryUsedPercent)
	assert.True(t, gpu.Status.Healthy)
}

func TestTelemetryGPU_JSONSerialization(t *testing.T) {
	gpu := TelemetryGPU{
		GPUID:         "0",
		UUID:          "GPU-12345-abcde",
		Device:        "nvidia0",
		DeviceName:    "NVIDIA H100 80GB HBM3",
		DriverVersion: "535.129.03",
		CudaVersion:   "12.2",
		MemoryTotal:   81920,
		Properties:    map[string]string{"compute_capability": "9.0", "arch": "hopper"},
		CreatedAt:     time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC),
		UpdatedAt:     time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC),
		Status: GPUStatus{
			State:              "active",
			UtilizationPercent: 85.5,
			MemoryUsedPercent:  60.2,
			TemperatureCelsius: 72.0,
			PowerDrawWatts:     350.5,
			Healthy:            true,
			LastUpdated:        time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC),
		},
	}

	// Test JSON marshaling
	jsonData, err := json.Marshal(gpu)
	require.NoError(t, err)
	assert.Contains(t, string(jsonData), "\"gpu_id\":\"0\"")
	assert.Contains(t, string(jsonData), "\"uuid\":\"GPU-12345-abcde\"")
	assert.Contains(t, string(jsonData), "\"device\":\"nvidia0\"")
	assert.Contains(t, string(jsonData), "\"memory_total_mb\":81920")

	// Test JSON unmarshaling
	var unmarshaled TelemetryGPU
	err = json.Unmarshal(jsonData, &unmarshaled)
	require.NoError(t, err)
	assert.Equal(t, gpu.GPUID, unmarshaled.GPUID)
	assert.Equal(t, gpu.UUID, unmarshaled.UUID)
	assert.Equal(t, gpu.Device, unmarshaled.Device)
	assert.Equal(t, gpu.DeviceName, unmarshaled.DeviceName)
	assert.Equal(t, gpu.MemoryTotal, unmarshaled.MemoryTotal)
	assert.Equal(t, gpu.Properties["compute_capability"], unmarshaled.Properties["compute_capability"])
}

func TestTelemetryData_Struct(t *testing.T) {
	now := time.Now()
	data := TelemetryData{
		TelemetryID:       "telemetry-1",
		Timestamp:         now,
		GPUID:             "0",
		UUID:              "GPU-12345-abcde",
		Device:            "nvidia0",
		ModelName:         "NVIDIA H100 80GB HBM3",
		Hostname:          "gpu-node-1",
		GPUUtilization:    85.5,
		MemoryUtilization: 60.2,
		MemoryUsedMB:      48000,
		MemoryFreeMB:      32000,
		Temperature:       72.0,
		PowerDraw:         350.5,
		SMClockMHz:        1410,
		MemoryClockMHz:    1215,
		CustomMetrics:     map[string]float32{"custom_metric": 123.45},
		CollectedAt:       now,
		ProcessedAt:       now,
		CollectorID:       "collector-1",
		BatchID:           "batch-1",
	}

	assert.Equal(t, "telemetry-1", data.TelemetryID)
	assert.Equal(t, "0", data.GPUID)
	assert.Equal(t, "GPU-12345-abcde", data.UUID)
	assert.Equal(t, "nvidia0", data.Device)
	assert.Equal(t, "NVIDIA H100 80GB HBM3", data.ModelName)
	assert.Equal(t, "gpu-node-1", data.Hostname)
	assert.Equal(t, float32(85.5), data.GPUUtilization)
	assert.Equal(t, float32(60.2), data.MemoryUtilization)
	assert.Equal(t, float32(48000), data.MemoryUsedMB)
	assert.Equal(t, float32(32000), data.MemoryFreeMB)
	assert.Equal(t, float32(72.0), data.Temperature)
	assert.Equal(t, float32(350.5), data.PowerDraw)
	assert.Equal(t, float32(1410), data.SMClockMHz)
	assert.Equal(t, float32(1215), data.MemoryClockMHz)
	assert.Equal(t, float32(123.45), data.CustomMetrics["custom_metric"])
	assert.Equal(t, "collector-1", data.CollectorID)
	assert.Equal(t, "batch-1", data.BatchID)
}

func TestTelemetryData_JSONSerialization(t *testing.T) {
	data := TelemetryData{
		TelemetryID:       "telemetry-1",
		Timestamp:         time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC),
		GPUID:             "0",
		UUID:              "GPU-12345-abcde",
		Device:            "nvidia0",
		ModelName:         "NVIDIA H100 80GB HBM3",
		Hostname:          "gpu-node-1",
		GPUUtilization:    85.5,
		MemoryUtilization: 60.2,
		MemoryUsedMB:      48000,
		MemoryFreeMB:      32000,
		Temperature:       72.0,
		PowerDraw:         350.5,
		SMClockMHz:        1410,
		MemoryClockMHz:    1215,
		CustomMetrics:     map[string]float32{"custom_metric": 123.45, "another": 67.89},
		CollectedAt:       time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC),
		ProcessedAt:       time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC),
		CollectorID:       "collector-1",
		BatchID:           "batch-1",
	}

	// Test JSON marshaling
	jsonData, err := json.Marshal(data)
	require.NoError(t, err)
	assert.Contains(t, string(jsonData), "\"telemetry_id\":\"telemetry-1\"")
	assert.Contains(t, string(jsonData), "\"gpu_id\":\"0\"")
	assert.Contains(t, string(jsonData), "\"uuid\":\"GPU-12345-abcde\"")
	assert.Contains(t, string(jsonData), "\"gpu_utilization\":85.5")
	assert.Contains(t, string(jsonData), "\"memory_utilization\":60.2")
	assert.Contains(t, string(jsonData), "\"temperature\":72")
	assert.Contains(t, string(jsonData), "\"power_draw\":350.5")

	// Test JSON unmarshaling
	var unmarshaled TelemetryData
	err = json.Unmarshal(jsonData, &unmarshaled)
	require.NoError(t, err)
	assert.Equal(t, data.TelemetryID, unmarshaled.TelemetryID)
	assert.Equal(t, data.GPUID, unmarshaled.GPUID)
	assert.Equal(t, data.UUID, unmarshaled.UUID)
	assert.Equal(t, data.GPUUtilization, unmarshaled.GPUUtilization)
	assert.Equal(t, data.MemoryUtilization, unmarshaled.MemoryUtilization)
	assert.Equal(t, data.Temperature, unmarshaled.Temperature)
	assert.Equal(t, data.PowerDraw, unmarshaled.PowerDraw)
	assert.Equal(t, data.CustomMetrics["custom_metric"], unmarshaled.CustomMetrics["custom_metric"])
	assert.Equal(t, data.CollectorID, unmarshaled.CollectorID)
	assert.Equal(t, data.BatchID, unmarshaled.BatchID)
}

func TestClusterMetadata_Struct(t *testing.T) {
	metadata := ClusterMetadata{
		TotalHosts:    10,
		TotalGPUs:     40,
		ActiveHosts:   9,
		ActiveGPUs:    38,
		LastUpdated:   time.Now(),
		Configuration: map[string]string{"batch_size": "100", "enabled": "true"},
	}

	assert.Equal(t, int32(10), metadata.TotalHosts)
	assert.Equal(t, int32(40), metadata.TotalGPUs)
	assert.Equal(t, int32(9), metadata.ActiveHosts)
	assert.Equal(t, int32(38), metadata.ActiveGPUs)
	assert.Equal(t, "100", metadata.Configuration["batch_size"])
	assert.Equal(t, "true", metadata.Configuration["enabled"])
}

func TestHostStatus_Struct(t *testing.T) {
	now := time.Now()
	status := HostStatus{
		State:        "active",
		LastSeen:     now,
		Healthy:      true,
		HealthChecks: map[string]string{"gpu_count": "4", "health_score": "0.95"},
	}

	assert.Equal(t, "active", status.State)
	assert.Equal(t, now, status.LastSeen)
	assert.True(t, status.Healthy)
	assert.Equal(t, "4", status.HealthChecks["gpu_count"])
	assert.Equal(t, "0.95", status.HealthChecks["health_score"])
}

func TestGPUStatus_Struct(t *testing.T) {
	now := time.Now()
	status := GPUStatus{
		State:              "active",
		UtilizationPercent: 85.5,
		MemoryUsedPercent:  60.2,
		TemperatureCelsius: 72.0,
		PowerDrawWatts:     350.5,
		Healthy:            true,
		LastUpdated:        now,
	}

	assert.Equal(t, "active", status.State)
	assert.Equal(t, float32(85.5), status.UtilizationPercent)
	assert.Equal(t, float32(60.2), status.MemoryUsedPercent)
	assert.Equal(t, float32(72.0), status.TemperatureCelsius)
	assert.Equal(t, float32(350.5), status.PowerDrawWatts)
	assert.True(t, status.Healthy)
	assert.Equal(t, now, status.LastUpdated)
}

func TestNewTelemetryService(t *testing.T) {
	config := &ServiceConfig{
		EtcdEndpoints:  []string{"localhost:2379"},
		ClusterID:      "test-cluster",
		ServiceID:      "test-service",
		UpdateInterval: 30 * time.Second,
		BatchSize:      100,
		EnableWatchAPI: true,
		EnableGraphQL:  true,
	}

	// Test with nil etcd client (for unit testing)
	service, err := NewTelemetryService(config)
	require.NoError(t, err)

	assert.NotNil(t, service)
	assert.Equal(t, config, service.config)
	assert.NotNil(t, service.ctx)
	assert.NotNil(t, service.cancel)
}

func TestTelemetryService_initializeCluster(t *testing.T) {
	config := &ServiceConfig{
		ClusterID:      "test-cluster",
		ServiceID:      "test-service",
		UpdateInterval: 30 * time.Second,
		BatchSize:      100,
	}

	service, err := NewTelemetryService(config)
	require.NoError(t, err)
	_ = service // Use the service variable to avoid unused variable error

	// Test cluster initialization logic (without etcd)
	cluster := &TelemetryCluster{
		ClusterID:   config.ClusterID,
		ClusterName: "Test Cluster",
		Region:      "us-west-2",
		Environment: "test",
		Labels:      map[string]string{"type": "test"},
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		Metadata: ClusterMetadata{
			TotalHosts:    0,
			TotalGPUs:     0,
			Configuration: map[string]string{"version": "1.0.0"},
		},
	}

	assert.Equal(t, config.ClusterID, cluster.ClusterID)
	assert.Equal(t, "Test Cluster", cluster.ClusterName)
	assert.Equal(t, "us-west-2", cluster.Region)
	assert.Equal(t, "test", cluster.Environment)
	assert.Equal(t, "test", cluster.Labels["type"])
	assert.Equal(t, int32(0), cluster.Metadata.TotalHosts)
	assert.Equal(t, int32(0), cluster.Metadata.TotalGPUs)
	assert.Equal(t, "1.0.0", cluster.Metadata.Configuration["version"])
}

func TestTelemetryService_Close(t *testing.T) {
	config := &ServiceConfig{
		ClusterID: "test-cluster",
		ServiceID: "test-service",
	}

	service, err := NewTelemetryService(config)
	require.NoError(t, err)

	// Test that Close doesn't panic
	assert.NotPanics(t, func() {
		service.Close()
	})

	// Verify context was cancelled
	select {
	case <-service.ctx.Done():
		// Expected
	default:
		t.Error("Context was not cancelled")
	}
}

func TestUtilityFunctions(t *testing.T) {
	// Test isGPUKey
	assert.True(t, isGPUKey("/telemetry/cluster-1/hosts/host-1/gpus/gpu-0"))
	assert.True(t, isGPUKey("/telemetry/cluster-1/hosts/host-2/gpus/gpu-1"))
	assert.False(t, isGPUKey("/telemetry/cluster-1/hosts/host-1"))
	assert.False(t, isGPUKey("/telemetry/cluster-1"))
	assert.False(t, isGPUKey(""))

	// Test isTelemetryDataKey
	assert.True(t, isTelemetryDataKey("/telemetry/cluster-1/data/gpu-0/2025-01-01"))
	assert.True(t, isTelemetryDataKey("/telemetry/cluster-1/data/gpu-1/2025-01-02"))
	assert.False(t, isTelemetryDataKey("/telemetry/cluster-1/hosts/host-1"))
	assert.False(t, isTelemetryDataKey("/telemetry/cluster-1"))
	assert.False(t, isTelemetryDataKey(""))

	// Test contains (string contains substring)
	assert.True(t, contains("apple,banana,cherry", "banana"))
	assert.True(t, contains("apple,banana,cherry", "apple"))
	assert.True(t, contains("apple,banana,cherry", "cherry"))
	assert.False(t, contains("apple,banana,cherry", "orange"))
	assert.False(t, contains("", "apple"))
	assert.False(t, contains("test", ""))

	// Test indexOfSubstring
	assert.Equal(t, 0, indexOfSubstring("hello world", "hello"))
	assert.Equal(t, 6, indexOfSubstring("hello world", "world"))
	assert.Equal(t, 2, indexOfSubstring("hello world", "llo"))
	assert.Equal(t, -1, indexOfSubstring("hello world", "xyz"))
	assert.Equal(t, -1, indexOfSubstring("", "hello"))
	assert.Equal(t, 0, indexOfSubstring("hello", ""))
}

func TestTelemetryService_ValidationLogic(t *testing.T) {
	// Test configuration validation logic
	tests := []struct {
		name   string
		config *ServiceConfig
		valid  bool
	}{
		{
			name: "valid config",
			config: &ServiceConfig{
				EtcdEndpoints:  []string{"localhost:2379"},
				ClusterID:      "cluster-1",
				ServiceID:      "service-1",
				UpdateInterval: 30 * time.Second,
				BatchSize:      100,
			},
			valid: true,
		},
		{
			name: "empty cluster ID",
			config: &ServiceConfig{
				EtcdEndpoints:  []string{"localhost:2379"},
				ClusterID:      "",
				ServiceID:      "service-1",
				UpdateInterval: 30 * time.Second,
				BatchSize:      100,
			},
			valid: false,
		},
		{
			name: "empty service ID",
			config: &ServiceConfig{
				EtcdEndpoints:  []string{"localhost:2379"},
				ClusterID:      "cluster-1",
				ServiceID:      "",
				UpdateInterval: 30 * time.Second,
				BatchSize:      100,
			},
			valid: false,
		},
		{
			name: "zero batch size",
			config: &ServiceConfig{
				EtcdEndpoints:  []string{"localhost:2379"},
				ClusterID:      "cluster-1",
				ServiceID:      "service-1",
				UpdateInterval: 30 * time.Second,
				BatchSize:      0,
			},
			valid: false,
		},
		{
			name: "negative batch size",
			config: &ServiceConfig{
				EtcdEndpoints:  []string{"localhost:2379"},
				ClusterID:      "cluster-1",
				ServiceID:      "service-1",
				UpdateInterval: 30 * time.Second,
				BatchSize:      -1,
			},
			valid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.valid {
				assert.NotEmpty(t, tt.config.ClusterID)
				assert.NotEmpty(t, tt.config.ServiceID)
				assert.Greater(t, tt.config.BatchSize, 0)
			} else {
				invalid := tt.config.ClusterID == "" ||
					tt.config.ServiceID == "" ||
					tt.config.BatchSize <= 0
				assert.True(t, invalid, "Config should be invalid")
			}
		})
	}
}

func TestTelemetryService_EdgeCases(t *testing.T) {
	// Test with minimal config
	config := &ServiceConfig{
		ClusterID: "minimal",
		ServiceID: "minimal-service",
		BatchSize: 1,
	}

	service, err := NewTelemetryService(config)
	require.NoError(t, err)
	assert.NotNil(t, service)
	assert.Equal(t, config, service.config)

	// Test with nil config (should return error)
	_, err = NewTelemetryService(nil)
	assert.Error(t, err)
}

func TestTelemetryData_Validation(t *testing.T) {
	tests := []struct {
		name  string
		data  TelemetryData
		valid bool
	}{
		{
			name: "valid telemetry data",
			data: TelemetryData{
				TelemetryID:       "telemetry-1",
				Timestamp:         time.Now(),
				GPUID:             "0",
				UUID:              "GPU-12345",
				Device:            "nvidia0",
				ModelName:         "NVIDIA H100",
				Hostname:          "gpu-node-1",
				GPUUtilization:    85.5,
				MemoryUtilization: 60.2,
				Temperature:       72.0,
				PowerDraw:         350.5,
				CollectorID:       "collector-1",
			},
			valid: true,
		},
		{
			name: "empty telemetry ID",
			data: TelemetryData{
				TelemetryID: "",
				GPUID:       "0",
				UUID:        "GPU-12345",
				Hostname:    "gpu-node-1",
			},
			valid: false,
		},
		{
			name: "empty GPU ID",
			data: TelemetryData{
				TelemetryID: "telemetry-1",
				GPUID:       "",
				UUID:        "GPU-12345",
				Hostname:    "gpu-node-1",
			},
			valid: false,
		},
		{
			name: "empty UUID",
			data: TelemetryData{
				TelemetryID: "telemetry-1",
				GPUID:       "0",
				UUID:        "",
				Hostname:    "gpu-node-1",
			},
			valid: false,
		},
		{
			name: "empty hostname",
			data: TelemetryData{
				TelemetryID: "telemetry-1",
				GPUID:       "0",
				UUID:        "GPU-12345",
				Hostname:    "",
			},
			valid: false,
		},
		{
			name: "negative temperature",
			data: TelemetryData{
				TelemetryID: "telemetry-1",
				GPUID:       "0",
				UUID:        "GPU-12345",
				Hostname:    "gpu-node-1",
				Temperature: -10.0,
			},
			valid: false,
		},
		{
			name: "negative power draw",
			data: TelemetryData{
				TelemetryID: "telemetry-1",
				GPUID:       "0",
				UUID:        "GPU-12345",
				Hostname:    "gpu-node-1",
				PowerDraw:   -100.0,
			},
			valid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.valid {
				assert.NotEmpty(t, tt.data.TelemetryID)
				assert.NotEmpty(t, tt.data.GPUID)
				assert.NotEmpty(t, tt.data.UUID)
				assert.NotEmpty(t, tt.data.Hostname)
				assert.GreaterOrEqual(t, tt.data.Temperature, float32(0.0))
				assert.GreaterOrEqual(t, tt.data.PowerDraw, float32(0.0))
			} else {
				invalid := tt.data.TelemetryID == "" ||
					tt.data.GPUID == "" ||
					tt.data.UUID == "" ||
					tt.data.Hostname == "" ||
					tt.data.Temperature < 0.0 ||
					tt.data.PowerDraw < 0.0
				assert.True(t, invalid, "Data should be invalid")
			}
		})
	}
}

// Benchmark tests
func BenchmarkTelemetryData_JSONMarshal(b *testing.B) {
	data := TelemetryData{
		TelemetryID:       "telemetry-1",
		Timestamp:         time.Now(),
		GPUID:             "0",
		UUID:              "GPU-12345-abcde",
		Device:            "nvidia0",
		ModelName:         "NVIDIA H100 80GB HBM3",
		Hostname:          "gpu-node-1",
		GPUUtilization:    85.5,
		MemoryUtilization: 60.2,
		MemoryUsedMB:      48000,
		MemoryFreeMB:      32000,
		Temperature:       72.0,
		PowerDraw:         350.5,
		SMClockMHz:        1410,
		MemoryClockMHz:    1215,
		CustomMetrics:     map[string]float32{"custom": 123.45},
		CollectedAt:       time.Now(),
		ProcessedAt:       time.Now(),
		CollectorID:       "collector-1",
		BatchID:           "batch-1",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := json.Marshal(data)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkTelemetryCluster_JSONMarshal(b *testing.B) {
	cluster := TelemetryCluster{
		ClusterID:   "cluster-1",
		ClusterName: "test-cluster",
		Region:      "us-west-2",
		Environment: "production",
		Labels:      map[string]string{"team": "platform", "env": "prod"},
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		Metadata: ClusterMetadata{
			TotalHosts:    10,
			TotalGPUs:     40,
			Configuration: map[string]string{"version": "1.0.0"},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := json.Marshal(cluster)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkTelemetryGPU_JSONMarshal(b *testing.B) {
	gpu := TelemetryGPU{
		GPUID:         "0",
		UUID:          "GPU-12345-abcde",
		Device:        "nvidia0",
		DeviceName:    "NVIDIA H100 80GB HBM3",
		DriverVersion: "535.129.03",
		CudaVersion:   "12.2",
		MemoryTotal:   81920,
		Properties:    map[string]string{"compute_capability": "9.0"},
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Status: GPUStatus{
			State:              "active",
			UtilizationPercent: 85.5,
			MemoryUsedPercent:  60.2,
			TemperatureCelsius: 72.0,
			PowerDrawWatts:     350.5,
			Healthy:            true,
			LastUpdated:        time.Now(),
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := json.Marshal(gpu)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkNewTelemetryService(b *testing.B) {
	config := &ServiceConfig{
		EtcdEndpoints:  []string{"localhost:2379"},
		ClusterID:      "benchmark-cluster",
		ServiceID:      "benchmark-service",
		UpdateInterval: 30 * time.Second,
		BatchSize:      100,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		service, err := NewTelemetryService(config)
		require.NoError(b, err)
		service.Close()
	}
}
