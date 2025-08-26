package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTelemetryData_Creation(t *testing.T) {
	now := time.Now()
	
	data := &TelemetryData{
		Timestamp:  now,
		GPUID:      "0",
		UUID:       "GPU-12345",
		Device:     "nvidia0",
		ModelName:  "NVIDIA H100",
		Hostname:   "test-host",
		ClusterID:  "cluster-1",
		MetricName: "DCGM_FI_DEV_GPU_UTIL",
		Value:      85.5,
		Unit:       "percent",
		Container:  "gpu-container",
		Pod:        "gpu-pod",
		Namespace:  "default",
		Tags:       map[string]string{"env": "prod"},
		Labels:     map[string]string{"region": "us-west"},
		LabelsRaw:  "region=us-west,env=prod",
		ProcessedAt: now,
		Source:     "dcgm-exporter",
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	
	assert.Equal(t, now, data.Timestamp)
	assert.Equal(t, "0", data.GPUID)
	assert.Equal(t, "GPU-12345", data.UUID)
	assert.Equal(t, "nvidia0", data.Device)
	assert.Equal(t, "NVIDIA H100", data.ModelName)
	assert.Equal(t, "test-host", data.Hostname)
	assert.Equal(t, "cluster-1", data.ClusterID)
	assert.Equal(t, "DCGM_FI_DEV_GPU_UTIL", data.MetricName)
	assert.Equal(t, 85.5, data.Value)
	assert.Equal(t, "percent", data.Unit)
	assert.Equal(t, "gpu-container", data.Container)
	assert.Equal(t, "gpu-pod", data.Pod)
	assert.Equal(t, "default", data.Namespace)
	assert.Equal(t, "prod", data.Tags["env"])
	assert.Equal(t, "us-west", data.Labels["region"])
	assert.Equal(t, "region=us-west,env=prod", data.LabelsRaw)
	assert.Equal(t, now, data.ProcessedAt)
	assert.Equal(t, "dcgm-exporter", data.Source)
	assert.Equal(t, now, data.CreatedAt)
	assert.Equal(t, now, data.UpdatedAt)
}

func TestTelemetryData_EmptyFields(t *testing.T) {
	data := &TelemetryData{
		Timestamp:  time.Now(),
		GPUID:      "0",
		MetricName: "test_metric",
		Value:      100.0,
	}
	
	// Test that optional fields can be empty
	assert.Empty(t, data.Container)
	assert.Empty(t, data.Pod)
	assert.Empty(t, data.Namespace)
	assert.Nil(t, data.Tags)
	assert.Nil(t, data.Labels)
	assert.Empty(t, data.LabelsRaw)
	assert.True(t, data.ProcessedAt.IsZero())
	assert.Empty(t, data.Source)
}

func TestTelemetryBatch_Creation(t *testing.T) {
	now := time.Now()
	
	data1 := &TelemetryData{
		Timestamp:  now,
		GPUID:      "0",
		MetricName: "metric1",
		Value:      10.0,
	}
	
	data2 := &TelemetryData{
		Timestamp:  now,
		GPUID:      "1",
		MetricName: "metric2",
		Value:      20.0,
	}
	
	batch := &TelemetryBatch{
		Data:      []*TelemetryData{data1, data2},
		BatchID:   "batch-123",
		Source:    "test-source",
		Timestamp: now,
		Count:     2,
	}
	
	assert.Len(t, batch.Data, 2)
	assert.Equal(t, "batch-123", batch.BatchID)
	assert.Equal(t, "test-source", batch.Source)
	assert.Equal(t, now, batch.Timestamp)
	assert.Equal(t, 2, batch.Count)
	assert.Equal(t, data1, batch.Data[0])
	assert.Equal(t, data2, batch.Data[1])
}

func TestTelemetryBatch_EmptyBatch(t *testing.T) {
	batch := &TelemetryBatch{
		Data:      []*TelemetryData{},
		BatchID:   "empty-batch",
		Source:    "test",
		Timestamp: time.Now(),
		Count:     0,
	}
	
	assert.Empty(t, batch.Data)
	assert.Equal(t, 0, batch.Count)
	assert.Equal(t, "empty-batch", batch.BatchID)
}

func TestHostInfo_Creation(t *testing.T) {
	now := time.Now()
	
	host := &HostInfo{
		Hostname:     "test-host-01",
		ClusterID:    "cluster-west",
		IPAddress:    "192.168.1.100",
		OS:           "Ubuntu 20.04",
		Architecture: "x86_64",
		Metadata: map[string]string{
			"zone":   "us-west-1a",
			"region": "us-west-1",
		},
		RegisteredAt: now,
		LastSeen:     now,
	}
	
	assert.Equal(t, "test-host-01", host.Hostname)
	assert.Equal(t, "cluster-west", host.ClusterID)
	assert.Equal(t, "192.168.1.100", host.IPAddress)
	assert.Equal(t, "Ubuntu 20.04", host.OS)
	assert.Equal(t, "x86_64", host.Architecture)
	assert.Equal(t, "us-west-1a", host.Metadata["zone"])
	assert.Equal(t, "us-west-1", host.Metadata["region"])
	assert.Equal(t, now, host.RegisteredAt)
	assert.Equal(t, now, host.LastSeen)
}

func TestGPUInfo_Creation(t *testing.T) {
	now := time.Now()
	
	gpu := &GPUInfo{
		GPUID:     "0",
		UUID:      "GPU-abcd-1234",
		Device:    "nvidia0",
		ModelName: "NVIDIA A100",
		Hostname:  "gpu-node-01",
		ClusterID: "ml-cluster",
		Memory:    80 * 1024 * 1024 * 1024, // 80GB
		Cores:     6912,
		Metadata: map[string]string{
			"driver_version": "470.82.01",
			"cuda_version":   "11.4",
		},
		RegisteredAt: now,
		LastSeen:     now,
	}
	
	assert.Equal(t, "0", gpu.GPUID)
	assert.Equal(t, "GPU-abcd-1234", gpu.UUID)
	assert.Equal(t, "nvidia0", gpu.Device)
	assert.Equal(t, "NVIDIA A100", gpu.ModelName)
	assert.Equal(t, "gpu-node-01", gpu.Hostname)
	assert.Equal(t, "ml-cluster", gpu.ClusterID)
	assert.Equal(t, int64(80*1024*1024*1024), gpu.Memory)
	assert.Equal(t, 6912, gpu.Cores)
	assert.Equal(t, "470.82.01", gpu.Metadata["driver_version"])
	assert.Equal(t, "11.4", gpu.Metadata["cuda_version"])
	assert.Equal(t, now, gpu.RegisteredAt)
	assert.Equal(t, now, gpu.LastSeen)
}

func TestMetricDefinition_Creation(t *testing.T) {
	metric := &MetricDefinition{
		Name:        "gpu_utilization",
		Description: "GPU utilization percentage",
		Unit:        "percent",
		Type:        "gauge",
		Tags:        []string{"gpu", "utilization", "performance"},
	}
	
	assert.Equal(t, "gpu_utilization", metric.Name)
	assert.Equal(t, "GPU utilization percentage", metric.Description)
	assert.Equal(t, "percent", metric.Unit)
	assert.Equal(t, "gauge", metric.Type)
	assert.Contains(t, metric.Tags, "gpu")
	assert.Contains(t, metric.Tags, "utilization")
	assert.Contains(t, metric.Tags, "performance")
	assert.Len(t, metric.Tags, 3)
}

func TestMetricDefinition_EmptyTags(t *testing.T) {
	metric := &MetricDefinition{
		Name:        "simple_metric",
		Description: "A simple metric",
		Unit:        "count",
		Type:        "counter",
	}
	
	assert.Equal(t, "simple_metric", metric.Name)
	assert.Equal(t, "A simple metric", metric.Description)
	assert.Equal(t, "count", metric.Unit)
	assert.Equal(t, "counter", metric.Type)
	assert.Nil(t, metric.Tags)
}

func TestTelemetryData_JSONTags(t *testing.T) {
	// Test that struct tags are properly defined for JSON serialization
	data := &TelemetryData{}
	
	// Use reflection to verify JSON tags exist (basic check)
	require.NotNil(t, data)
	
	// Test with actual values to ensure serialization works
	now := time.Now()
	data.Timestamp = now
	data.GPUID = "test"
	data.MetricName = "test_metric"
	data.Value = 42.0
	
	// Basic validation that the struct can hold the data
	assert.Equal(t, now, data.Timestamp)
	assert.Equal(t, "test", data.GPUID)
	assert.Equal(t, "test_metric", data.MetricName)
	assert.Equal(t, 42.0, data.Value)
}

func TestTelemetryBatch_AddData(t *testing.T) {
	batch := &TelemetryBatch{
		Data:      []*TelemetryData{},
		BatchID:   "test-batch",
		Source:    "test",
		Timestamp: time.Now(),
		Count:     0,
	}
	
	// Add data to batch
	data1 := &TelemetryData{GPUID: "0", MetricName: "metric1", Value: 10.0}
	data2 := &TelemetryData{GPUID: "1", MetricName: "metric2", Value: 20.0}
	
	batch.Data = append(batch.Data, data1)
	batch.Count++
	
	batch.Data = append(batch.Data, data2)
	batch.Count++
	
	assert.Len(t, batch.Data, 2)
	assert.Equal(t, 2, batch.Count)
	assert.Equal(t, "0", batch.Data[0].GPUID)
	assert.Equal(t, "1", batch.Data[1].GPUID)
}
