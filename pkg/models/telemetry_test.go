package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestTelemetryDataTableName(t *testing.T) {
	data := TelemetryData{}
	assert.Equal(t, "telemetry_data", data.TableName())
}

func TestTelemetryDataBeforeCreate(t *testing.T) {
	data := &TelemetryData{}

	// Test with zero time
	err := data.BeforeCreate(nil)
	assert.NoError(t, err)
	assert.False(t, data.CreatedAt.IsZero())
	assert.False(t, data.UpdatedAt.IsZero())

	// Test with existing time
	existingTime := time.Now().Add(-1 * time.Hour)
	data.CreatedAt = existingTime
	data.UpdatedAt = existingTime

	err = data.BeforeCreate(nil)
	assert.NoError(t, err)
	assert.Equal(t, existingTime, data.CreatedAt) // Should not change
	assert.Equal(t, existingTime, data.UpdatedAt) // Should not change
}

func TestTelemetryDataBeforeUpdate(t *testing.T) {
	data := &TelemetryData{
		CreatedAt: time.Now().Add(-1 * time.Hour),
		UpdatedAt: time.Now().Add(-30 * time.Minute),
	}

	originalCreatedAt := data.CreatedAt
	originalUpdatedAt := data.UpdatedAt

	err := data.BeforeUpdate(nil)
	assert.NoError(t, err)

	// CreatedAt should not change
	assert.Equal(t, originalCreatedAt, data.CreatedAt)

	// UpdatedAt should be updated
	assert.True(t, data.UpdatedAt.After(originalUpdatedAt))
}

func TestCSVRecord(t *testing.T) {
	record := CSVRecord{
		Timestamp:  "2025-07-18T20:42:34Z",
		MetricName: "DCGM_FI_DEV_GPU_UTIL",
		GPUID:      "0",
		Device:     "nvidia0",
		UUID:       "GPU-5fd4f087-86f3-7a43-b711-4771313afc50",
		ModelName:  "NVIDIA H100 80GB HBM3",
		Hostname:   "mtv5-dgx1-hgpu-031",
		Container:  "",
		Pod:        "",
		Namespace:  "",
		Value:      "0",
		LabelsRaw:  "DCGM_FI_DRIVER_VERSION=\"535.129.03\"",
	}

	assert.Equal(t, "2025-07-18T20:42:34Z", record.Timestamp)
	assert.Equal(t, "DCGM_FI_DEV_GPU_UTIL", record.MetricName)
	assert.Equal(t, "0", record.GPUID)
	assert.Equal(t, "nvidia0", record.Device)
	assert.Equal(t, "GPU-5fd4f087-86f3-7a43-b711-4771313afc50", record.UUID)
	assert.Equal(t, "NVIDIA H100 80GB HBM3", record.ModelName)
	assert.Equal(t, "mtv5-dgx1-hgpu-031", record.Hostname)
	assert.Equal(t, "0", record.Value)
}

func TestGPU(t *testing.T) {
	gpu := GPU{
		GPUID:     "0",
		UUID:      "GPU-5fd4f087-86f3-7a43-b711-4771313afc50",
		ModelName: "NVIDIA H100 80GB HBM3",
		Hostname:  "mtv5-dgx1-hgpu-031",
		Device:    "nvidia0",
	}

	assert.Equal(t, "0", gpu.GPUID)
	assert.Equal(t, "GPU-5fd4f087-86f3-7a43-b711-4771313afc50", gpu.UUID)
	assert.Equal(t, "NVIDIA H100 80GB HBM3", gpu.ModelName)
	assert.Equal(t, "mtv5-dgx1-hgpu-031", gpu.Hostname)
	assert.Equal(t, "nvidia0", gpu.Device)
}

func TestTelemetryQuery(t *testing.T) {
	startTime := time.Now().Add(-1 * time.Hour)
	endTime := time.Now()

	query := TelemetryQuery{
		GPUID:     "0",
		StartTime: startTime,
		EndTime:   endTime,
		Limit:     100,
		Offset:    0,
	}

	assert.Equal(t, "0", query.GPUID)
	assert.Equal(t, startTime, query.StartTime)
	assert.Equal(t, endTime, query.EndTime)
	assert.Equal(t, 100, query.Limit)
	assert.Equal(t, 0, query.Offset)
}

func TestTelemetryQueryResponse(t *testing.T) {
	dataPoints := []TelemetryData{
		{
			GPUID:      "0",
			MetricName: "GPU_UTIL",
			Value:      85.5,
			Timestamp:  time.Now(),
		},
		{
			GPUID:      "0",
			MetricName: "GPU_TEMP",
			Value:      72.0,
			Timestamp:  time.Now(),
		},
	}

	response := TelemetryQueryResponse{
		DataPoints: dataPoints,
		TotalCount: 150,
		HasMore:    true,
	}

	assert.Len(t, response.DataPoints, 2)
	assert.Equal(t, int64(150), response.TotalCount)
	assert.True(t, response.HasMore)
}

func TestTelemetryDataValidation(t *testing.T) {
	// Test valid telemetry data
	data := TelemetryData{
		Timestamp:  time.Now(),
		MetricName: "GPU_UTIL",
		GPUID:      "0",
		Device:     "nvidia0",
		UUID:       "GPU-12345",
		ModelName:  "NVIDIA H100",
		Hostname:   "host1",
		Value:      85.5,
	}

	assert.False(t, data.Timestamp.IsZero())
	assert.NotEmpty(t, data.MetricName)
	assert.NotEmpty(t, data.GPUID)
	assert.NotEmpty(t, data.UUID)
	assert.Greater(t, data.Value, 0.0)
}

func TestTelemetryDataJSON(t *testing.T) {
	data := TelemetryData{
		ID:         1,
		Timestamp:  time.Date(2025, 7, 18, 20, 42, 34, 0, time.UTC),
		MetricName: "GPU_UTIL",
		GPUID:      "0",
		Device:     "nvidia0",
		UUID:       "GPU-12345",
		ModelName:  "NVIDIA H100",
		Hostname:   "host1",
		Value:      85.5,
		LabelsRaw:  "key=value",
	}

	// Test that all fields are accessible
	assert.Equal(t, uint(1), data.ID)
	assert.Equal(t, "GPU_UTIL", data.MetricName)
	assert.Equal(t, "0", data.GPUID)
	assert.Equal(t, "nvidia0", data.Device)
	assert.Equal(t, "GPU-12345", data.UUID)
	assert.Equal(t, "NVIDIA H100", data.ModelName)
	assert.Equal(t, "host1", data.Hostname)
	assert.Equal(t, 85.5, data.Value)
	assert.Equal(t, "key=value", data.LabelsRaw)
}
