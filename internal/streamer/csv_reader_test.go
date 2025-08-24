package streamer

import (
	"io"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createTestCSV creates a temporary CSV file for testing
func createTestCSV(t *testing.T, content string) string {
	tmpFile, err := os.CreateTemp("", "test_telemetry_*.csv")
	require.NoError(t, err)

	_, err = tmpFile.WriteString(content)
	require.NoError(t, err)

	err = tmpFile.Close()
	require.NoError(t, err)

	return tmpFile.Name()
}

func TestNewCSVReader(t *testing.T) {
	csvContent := `timestamp,metric_name,gpu_id,device,uuid,modelName,Hostname,container,pod,namespace,value,labels_raw
2025-07-18T20:42:34Z,DCGM_FI_DEV_GPU_UTIL,0,nvidia0,GPU-12345,NVIDIA H100,host1,,,,"85.5","key=value"
2025-07-18T20:42:35Z,DCGM_FI_DEV_GPU_TEMP,0,nvidia0,GPU-12345,NVIDIA H100,host1,,,,"72.0","key=value"`

	filename := createTestCSV(t, csvContent)
	defer os.Remove(filename)

	reader, err := NewCSVReader(filename, false)
	require.NoError(t, err)
	require.NotNil(t, reader)

	assert.Equal(t, filename, reader.GetFilename())
	assert.False(t, reader.IsLoopMode())
	assert.Equal(t, int64(0), reader.GetPosition())

	headers := reader.GetHeaders()
	expectedHeaders := []string{"timestamp", "metric_name", "gpu_id", "device", "uuid", "modelName", "Hostname", "container", "pod", "namespace", "value", "labels_raw"}
	assert.Equal(t, expectedHeaders, headers)

	err = reader.Close()
	assert.NoError(t, err)
}

func TestNewCSVReaderFileNotFound(t *testing.T) {
	_, err := NewCSVReader("non-existent-file.csv", false)
	assert.Error(t, err)
}

func TestCSVReaderReadBatch(t *testing.T) {
	csvContent := `timestamp,metric_name,gpu_id,device,uuid,modelName,Hostname,container,pod,namespace,value,labels_raw
2025-07-18T20:42:34Z,DCGM_FI_DEV_GPU_UTIL,0,nvidia0,GPU-12345,NVIDIA H100,host1,,,,"85.5","key=value"
2025-07-18T20:42:35Z,DCGM_FI_DEV_GPU_TEMP,0,nvidia0,GPU-12345,NVIDIA H100,host1,,,,"72.0","key=value"
2025-07-18T20:42:36Z,DCGM_FI_DEV_MEM_UTIL,1,nvidia1,GPU-67890,NVIDIA H100,host1,,,,"45.2","key=value"`

	filename := createTestCSV(t, csvContent)
	defer os.Remove(filename)

	reader, err := NewCSVReader(filename, false)
	require.NoError(t, err)
	defer reader.Close()

	// Read batch of 2
	batch, err := reader.ReadBatch(2)
	assert.NoError(t, err)
	assert.Len(t, batch, 2)

	// Check first record
	assert.Equal(t, "DCGM_FI_DEV_GPU_UTIL", batch[0].MetricName)
	assert.Equal(t, "0", batch[0].GPUID)
	assert.Equal(t, "nvidia0", batch[0].Device)
	assert.Equal(t, "GPU-12345", batch[0].UUID)
	assert.Equal(t, "NVIDIA H100", batch[0].ModelName)
	assert.Equal(t, "host1", batch[0].Hostname)
	assert.Equal(t, 85.5, batch[0].Value)

	// Check second record
	assert.Equal(t, "DCGM_FI_DEV_GPU_TEMP", batch[1].MetricName)
	assert.Equal(t, 72.0, batch[1].Value)

	// Read remaining batch
	batch, err = reader.ReadBatch(2)
	if err == io.EOF {
		// EOF is expected when we reach end of file
		assert.Len(t, batch, 1) // Should still get the remaining record
	} else {
		assert.NoError(t, err)
		assert.Len(t, batch, 1) // Only one record left
	}

	assert.Equal(t, "DCGM_FI_DEV_MEM_UTIL", batch[0].MetricName)
	assert.Equal(t, "1", batch[0].GPUID)
	assert.Equal(t, 45.2, batch[0].Value)

	// Try to read more - should get EOF
	_, err = reader.ReadBatch(1)
	assert.ErrorIs(t, err, io.EOF)
}

func TestCSVReaderReadAll(t *testing.T) {
	csvContent := `timestamp,metric_name,gpu_id,device,uuid,modelName,Hostname,container,pod,namespace,value,labels_raw
2025-07-18T20:42:34Z,DCGM_FI_DEV_GPU_UTIL,0,nvidia0,GPU-12345,NVIDIA H100,host1,,,,"85.5","key=value"
2025-07-18T20:42:35Z,DCGM_FI_DEV_GPU_TEMP,0,nvidia0,GPU-12345,NVIDIA H100,host1,,,,"72.0","key=value"`

	filename := createTestCSV(t, csvContent)
	defer os.Remove(filename)

	reader, err := NewCSVReader(filename, false)
	require.NoError(t, err)
	defer reader.Close()

	data, err := reader.ReadAll()
	assert.NoError(t, err)
	assert.Len(t, data, 2)

	assert.Equal(t, "DCGM_FI_DEV_GPU_UTIL", data[0].MetricName)
	assert.Equal(t, "DCGM_FI_DEV_GPU_TEMP", data[1].MetricName)
}

func TestCSVReaderLoopMode(t *testing.T) {
	csvContent := `timestamp,metric_name,gpu_id,device,uuid,modelName,Hostname,container,pod,namespace,value,labels_raw
2025-07-18T20:42:34Z,DCGM_FI_DEV_GPU_UTIL,0,nvidia0,GPU-12345,NVIDIA H100,host1,,,,"85.5","key=value"`

	filename := createTestCSV(t, csvContent)
	defer os.Remove(filename)

	reader, err := NewCSVReader(filename, true)
	require.NoError(t, err)
	defer reader.Close()

	assert.True(t, reader.IsLoopMode())

	// Read the single record
	batch1, err := reader.ReadBatch(1)
	assert.NoError(t, err)
	assert.Len(t, batch1, 1)
	assert.Equal(t, "DCGM_FI_DEV_GPU_UTIL", batch1[0].MetricName)

	// In loop mode, should reset and read again
	batch2, err := reader.ReadBatch(1)
	assert.NoError(t, err)
	assert.Len(t, batch2, 1)
	assert.Equal(t, "DCGM_FI_DEV_GPU_UTIL", batch2[0].MetricName)

	// Position should reset to 1 (after reading one record again)
	assert.Equal(t, int64(1), reader.GetPosition())
}

func TestCSVReaderParseRecord(t *testing.T) {
	csvContent := `timestamp,metric_name,gpu_id,device,uuid,modelName,Hostname,container,pod,namespace,value,labels_raw
"2025-07-18T20:42:34Z","DCGM_FI_DEV_GPU_UTIL","0","nvidia0","GPU-12345","NVIDIA H100","host1","","","","85.5","key=value"`

	filename := createTestCSV(t, csvContent)
	defer os.Remove(filename)

	reader, err := NewCSVReader(filename, false)
	require.NoError(t, err)
	defer reader.Close()

	batch, err := reader.ReadBatch(1)
	assert.NoError(t, err)
	assert.Len(t, batch, 1)

	data := batch[0]

	// Check timestamp parsing - just verify it's not zero
	assert.False(t, data.Timestamp.IsZero())

	// Check string fields (quotes should be removed)
	assert.Equal(t, "DCGM_FI_DEV_GPU_UTIL", data.MetricName)
	assert.Equal(t, "0", data.GPUID)
	assert.Equal(t, "nvidia0", data.Device)
	assert.Equal(t, "GPU-12345", data.UUID)
	assert.Equal(t, "NVIDIA H100", data.ModelName)
	assert.Equal(t, "host1", data.Hostname)

	// Check numeric value parsing
	assert.Equal(t, 85.5, data.Value)
}

func TestCSVReaderInvalidTimestamp(t *testing.T) {
	csvContent := `timestamp,metric_name,gpu_id,device,uuid,modelName,Hostname,container,pod,namespace,value,labels_raw
invalid-timestamp,DCGM_FI_DEV_GPU_UTIL,0,nvidia0,GPU-12345,NVIDIA H100,host1,,,,"85.5","key=value"`

	filename := createTestCSV(t, csvContent)
	defer os.Remove(filename)

	reader, err := NewCSVReader(filename, false)
	require.NoError(t, err)
	defer reader.Close()

	batch, err := reader.ReadBatch(1)
	assert.NoError(t, err)
	assert.Len(t, batch, 1)

	// Should use current time for invalid timestamp
	data := batch[0]
	assert.True(t, time.Since(data.Timestamp) < time.Minute) // Should be recent
}

func TestCSVReaderInvalidValue(t *testing.T) {
	csvContent := `timestamp,metric_name,gpu_id,device,uuid,modelName,Hostname,container,pod,namespace,value,labels_raw
2025-07-18T20:42:34Z,DCGM_FI_DEV_GPU_UTIL,0,nvidia0,GPU-12345,NVIDIA H100,host1,,,,"invalid","key=value"`

	filename := createTestCSV(t, csvContent)
	defer os.Remove(filename)

	reader, err := NewCSVReader(filename, false)
	require.NoError(t, err)
	defer reader.Close()

	batch, err := reader.ReadBatch(1)
	assert.NoError(t, err)
	assert.Len(t, batch, 1)

	// Should use 0.0 for invalid value
	data := batch[0]
	assert.Equal(t, 0.0, data.Value)
}

func TestCSVReaderMissingFields(t *testing.T) {
	csvContent := `timestamp,metric_name,gpu_id,device,uuid,modelName,Hostname,container,pod,namespace,value,labels_raw
2025-07-18T20:42:34Z,DCGM_FI_DEV_GPU_UTIL,0`

	filename := createTestCSV(t, csvContent)
	defer os.Remove(filename)

	reader, err := NewCSVReader(filename, false)
	require.NoError(t, err)
	defer reader.Close()

	batch, err := reader.ReadBatch(1)
	assert.NoError(t, err)
	assert.Len(t, batch, 1)

	// Should handle missing fields gracefully
	data := batch[0]
	assert.Equal(t, "DCGM_FI_DEV_GPU_UTIL", data.MetricName)
	assert.Equal(t, "0", data.GPUID)
	assert.Equal(t, "", data.Device) // Missing field should be empty
	assert.Equal(t, 0.0, data.Value) // Missing value should be 0.0
}

func TestCSVReaderPosition(t *testing.T) {
	csvContent := `timestamp,metric_name,gpu_id,device,uuid,modelName,Hostname,container,pod,namespace,value,labels_raw
2025-07-18T20:42:34Z,DCGM_FI_DEV_GPU_UTIL,0,nvidia0,GPU-12345,NVIDIA H100,host1,,,,"85.5","key=value"
2025-07-18T20:42:35Z,DCGM_FI_DEV_GPU_TEMP,0,nvidia0,GPU-12345,NVIDIA H100,host1,,,,"72.0","key=value"`

	filename := createTestCSV(t, csvContent)
	defer os.Remove(filename)

	reader, err := NewCSVReader(filename, false)
	require.NoError(t, err)
	defer reader.Close()

	assert.Equal(t, int64(0), reader.GetPosition())

	_, err = reader.ReadBatch(1)
	assert.NoError(t, err)
	assert.Equal(t, int64(1), reader.GetPosition())

	_, err = reader.ReadBatch(1)
	assert.NoError(t, err)
	assert.Equal(t, int64(2), reader.GetPosition())
}

func TestCSVReaderClose(t *testing.T) {
	csvContent := `timestamp,metric_name,gpu_id,device,uuid,modelName,Hostname,container,pod,namespace,value,labels_raw
2025-07-18T20:42:34Z,DCGM_FI_DEV_GPU_UTIL,0,nvidia0,GPU-12345,NVIDIA H100,host1,,,,"85.5","key=value"`

	filename := createTestCSV(t, csvContent)
	defer os.Remove(filename)

	reader, err := NewCSVReader(filename, false)
	require.NoError(t, err)

	err = reader.Close()
	assert.NoError(t, err)

	// Calling close again should not error
	err = reader.Close()
	assert.NoError(t, err)
}
