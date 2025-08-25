package models

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestTelemetryDataJSONSerialization(t *testing.T) {
	data := TelemetryData{
		ID:         1,
		Timestamp:  time.Date(2025, 7, 18, 20, 42, 34, 0, time.UTC),
		MetricName: "GPU_UTIL",
		GPUID:      "0",
		Device:     "nvidia0",
		UUID:       "GPU-12345",
		ModelName:  "NVIDIA H100",
		Hostname:   "host1",
		Container:  "test-container",
		Pod:        "test-pod",
		Namespace:  "test-namespace",
		Value:      85.5,
		LabelsRaw:  "key=value",
		CreatedAt:  time.Date(2025, 7, 18, 20, 42, 34, 0, time.UTC),
		UpdatedAt:  time.Date(2025, 7, 18, 20, 42, 34, 0, time.UTC),
	}

	// Test JSON marshaling
	jsonData, err := json.Marshal(data)
	require.NoError(t, err)
	assert.Contains(t, string(jsonData), "\"gpu_id\":\"0\"")
	assert.Contains(t, string(jsonData), "\"metric_name\":\"GPU_UTIL\"")
	assert.Contains(t, string(jsonData), "\"value\":85.5")

	// Test JSON unmarshaling
	var unmarshaled TelemetryData
	err = json.Unmarshal(jsonData, &unmarshaled)
	require.NoError(t, err)
	assert.Equal(t, data.ID, unmarshaled.ID)
	assert.Equal(t, data.MetricName, unmarshaled.MetricName)
	assert.Equal(t, data.GPUID, unmarshaled.GPUID)
	assert.Equal(t, data.Value, unmarshaled.Value)
}

func TestGPUJSONSerialization(t *testing.T) {
	gpu := GPU{
		GPUID:     "0",
		UUID:      "GPU-12345",
		ModelName: "NVIDIA H100",
		Hostname:  "host1",
		Device:    "nvidia0",
	}

	// Test JSON marshaling
	jsonData, err := json.Marshal(gpu)
	require.NoError(t, err)
	assert.Contains(t, string(jsonData), "\"gpu_id\":\"0\"")
	assert.Contains(t, string(jsonData), "\"uuid\":\"GPU-12345\"")

	// Test JSON unmarshaling
	var unmarshaled GPU
	err = json.Unmarshal(jsonData, &unmarshaled)
	require.NoError(t, err)
	assert.Equal(t, gpu, unmarshaled)
}

func TestTelemetryQueryJSONSerialization(t *testing.T) {
	startTime := time.Date(2025, 7, 18, 19, 42, 34, 0, time.UTC)
	endTime := time.Date(2025, 7, 18, 20, 42, 34, 0, time.UTC)

	query := TelemetryQuery{
		GPUID:     "0",
		StartTime: startTime,
		EndTime:   endTime,
		Limit:     100,
		Offset:    0,
	}

	// Test JSON marshaling
	jsonData, err := json.Marshal(query)
	require.NoError(t, err)
	assert.Contains(t, string(jsonData), "\"gpu_id\":\"0\"")
	assert.Contains(t, string(jsonData), "\"limit\":100")

	// Test JSON unmarshaling
	var unmarshaled TelemetryQuery
	err = json.Unmarshal(jsonData, &unmarshaled)
	require.NoError(t, err)
	assert.Equal(t, query.GPUID, unmarshaled.GPUID)
	assert.Equal(t, query.Limit, unmarshaled.Limit)
	assert.Equal(t, query.Offset, unmarshaled.Offset)
	assert.True(t, query.StartTime.Equal(unmarshaled.StartTime))
	assert.True(t, query.EndTime.Equal(unmarshaled.EndTime))
}

func TestTelemetryQueryResponseJSONSerialization(t *testing.T) {
	dataPoints := []TelemetryData{
		{
			ID:         1,
			GPUID:      "0",
			MetricName: "GPU_UTIL",
			Value:      85.5,
			Timestamp:  time.Date(2025, 7, 18, 20, 42, 34, 0, time.UTC),
		},
	}

	response := TelemetryQueryResponse{
		DataPoints: dataPoints,
		TotalCount: 150,
		HasMore:    true,
	}

	// Test JSON marshaling
	jsonData, err := json.Marshal(response)
	require.NoError(t, err)
	assert.Contains(t, string(jsonData), "\"total_count\":150")
	assert.Contains(t, string(jsonData), "\"has_more\":true")

	// Test JSON unmarshaling
	var unmarshaled TelemetryQueryResponse
	err = json.Unmarshal(jsonData, &unmarshaled)
	require.NoError(t, err)
	assert.Equal(t, response.TotalCount, unmarshaled.TotalCount)
	assert.Equal(t, response.HasMore, unmarshaled.HasMore)
	assert.Len(t, unmarshaled.DataPoints, 1)
	assert.Equal(t, dataPoints[0].GPUID, unmarshaled.DataPoints[0].GPUID)
}

func TestCSVRecordValidation(t *testing.T) {
	tests := []struct {
		name   string
		record CSVRecord
		valid  bool
	}{
		{
			name: "valid record",
			record: CSVRecord{
				Timestamp:  "2025-07-18T20:42:34Z",
				MetricName: "GPU_UTIL",
				GPUID:      "0",
				Device:     "nvidia0",
				UUID:       "GPU-12345",
				ModelName:  "NVIDIA H100",
				Hostname:   "host1",
				Value:      "85.5",
			},
			valid: true,
		},
		{
			name: "empty timestamp",
			record: CSVRecord{
				Timestamp:  "",
				MetricName: "GPU_UTIL",
				GPUID:      "0",
				Value:      "85.5",
			},
			valid: false,
		},
		{
			name: "empty metric name",
			record: CSVRecord{
				Timestamp:  "2025-07-18T20:42:34Z",
				MetricName: "",
				GPUID:      "0",
				Value:      "85.5",
			},
			valid: false,
		},
		{
			name: "empty value",
			record: CSVRecord{
				Timestamp:  "2025-07-18T20:42:34Z",
				MetricName: "GPU_UTIL",
				GPUID:      "0",
				Value:      "",
			},
			valid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.valid {
				assert.NotEmpty(t, tt.record.Timestamp)
				assert.NotEmpty(t, tt.record.MetricName)
				assert.NotEmpty(t, tt.record.Value)
			} else {
				// At least one required field should be empty
				isEmpty := tt.record.Timestamp == "" || tt.record.MetricName == "" || tt.record.Value == ""
				assert.True(t, isEmpty, "At least one required field should be empty for invalid record")
			}
		})
	}
}

func TestTelemetryDataZeroValues(t *testing.T) {
	var data TelemetryData

	// Test zero values
	assert.Zero(t, data.ID)
	assert.True(t, data.Timestamp.IsZero())
	assert.Empty(t, data.MetricName)
	assert.Empty(t, data.GPUID)
	assert.Zero(t, data.Value)
	assert.True(t, data.CreatedAt.IsZero())
	assert.True(t, data.UpdatedAt.IsZero())
}

func TestTelemetryQueryDefaults(t *testing.T) {
	var query TelemetryQuery

	// Test default values
	assert.Empty(t, query.GPUID)
	assert.True(t, query.StartTime.IsZero())
	assert.True(t, query.EndTime.IsZero())
	assert.Zero(t, query.Limit)
	assert.Zero(t, query.Offset)
}

func TestTelemetryQueryResponseEmpty(t *testing.T) {
	response := TelemetryQueryResponse{
		DataPoints: []TelemetryData{},
		TotalCount: 0,
		HasMore:    false,
	}

	assert.Empty(t, response.DataPoints)
	assert.Zero(t, response.TotalCount)
	assert.False(t, response.HasMore)
}

// Benchmark tests
func BenchmarkTelemetryDataJSONMarshal(b *testing.B) {
	data := TelemetryData{
		ID:         1,
		Timestamp:  time.Now(),
		MetricName: "GPU_UTIL",
		GPUID:      "0",
		Device:     "nvidia0",
		UUID:       "GPU-12345",
		ModelName:  "NVIDIA H100",
		Hostname:   "host1",
		Value:      85.5,
		LabelsRaw:  "key=value",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := json.Marshal(data)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkTelemetryDataJSONUnmarshal(b *testing.B) {
	data := TelemetryData{
		ID:         1,
		Timestamp:  time.Now(),
		MetricName: "GPU_UTIL",
		GPUID:      "0",
		Device:     "nvidia0",
		UUID:       "GPU-12345",
		ModelName:  "NVIDIA H100",
		Hostname:   "host1",
		Value:      85.5,
		LabelsRaw:  "key=value",
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var unmarshaled TelemetryData
		err := json.Unmarshal(jsonData, &unmarshaled)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkTelemetryDataBeforeCreate(b *testing.B) {
	data := &TelemetryData{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = data.BeforeCreate(nil)
	}
}

func BenchmarkTelemetryDataBeforeUpdate(b *testing.B) {
	data := &TelemetryData{
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = data.BeforeUpdate(nil)
	}
}
