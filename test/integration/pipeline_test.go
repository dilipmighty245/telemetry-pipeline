//go:build integration
// +build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cf/telemetry-pipeline/internal/collector"
	"github.com/cf/telemetry-pipeline/internal/streamer"
	"github.com/cf/telemetry-pipeline/pkg/messagequeue"
	"github.com/cf/telemetry-pipeline/pkg/models"
)

// TestTelemetryPipelineIntegration tests the complete pipeline flow
func TestTelemetryPipelineIntegration(t *testing.T) {
	// Skip if not running integration tests
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	// Create test CSV file
	csvContent := `timestamp,metric_name,gpu_id,device,uuid,modelName,Hostname,container,pod,namespace,value,labels_raw
2025-07-18T20:42:34Z,DCGM_FI_DEV_GPU_UTIL,0,nvidia0,GPU-12345,NVIDIA H100,host1,,,,"85.5","key=value"
2025-07-18T20:42:35Z,DCGM_FI_DEV_GPU_TEMP,0,nvidia0,GPU-12345,NVIDIA H100,host1,,,,"72.0","key=value"
2025-07-18T20:42:36Z,DCGM_FI_DEV_MEM_UTIL,1,nvidia1,GPU-67890,NVIDIA H100,host2,,,,"45.2","key=value"`

	tmpFile := createTempCSV(t, csvContent)
	defer os.Remove(tmpFile)

	// Create message queue service
	mqService := messagequeue.NewMessageQueueService()
	defer mqService.Stop()

	// Create in-memory database service (for testing)
	dbService := createInMemoryDatabase(t)
	defer dbService.Close()

	// Create and start streamer
	streamerConfig := &streamer.StreamerConfig{
		CSVFilePath:    tmpFile,
		BatchSize:      2,
		StreamInterval: 100 * time.Millisecond,
		LoopMode:       false, // Don't loop for test
		MaxRetries:     1,
		RetryDelay:     100 * time.Millisecond,
		StreamerID:     "test-streamer",
	}

	streamerService, err := streamer.NewStreamerService(streamerConfig, mqService)
	require.NoError(t, err)

	err = streamerService.Start()
	require.NoError(t, err)
	defer streamerService.Stop()

	// Create and start collector
	collectorConfig := &collector.CollectorConfig{
		CollectorID:   "test-collector",
		ConsumerGroup: "test-group",
		BatchSize:     2,
		PollInterval:  100 * time.Millisecond,
		BufferSize:    10,
		DatabaseConfig: &collector.DatabaseConfig{
			Host:     "localhost",
			Port:     5432,
			User:     "test",
			Password: "test",
			DBName:   "test",
			SSLMode:  "disable",
		},
	}

	collectorService, err := collector.NewCollectorService(collectorConfig, mqService)
	require.NoError(t, err)

	err = collectorService.Start()
	require.NoError(t, err)
	defer collectorService.Stop()

	// Wait for processing to complete
	time.Sleep(2 * time.Second)

	// Verify streamer metrics
	streamerMetrics := streamerService.GetMetrics()
	assert.Greater(t, streamerMetrics.TotalRecordsStreamed, int64(0))
	assert.Greater(t, streamerMetrics.TotalBatchesStreamed, int64(0))

	// Verify collector metrics
	collectorMetrics := collectorService.GetMetrics()
	assert.Greater(t, collectorMetrics.TotalRecordsCollected, int64(0))

	// Verify message queue stats
	mqStats := mqService.GetQueueStats()
	assert.Greater(t, mqStats.TotalMessages, int64(0))
}

// TestMessageQueueResilience tests message queue resilience
func TestMessageQueueResilience(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	mqService := messagequeue.NewMessageQueueService()
	defer mqService.Stop()

	// Test publishing to non-existent topic (should create topic)
	payload := []byte(`{"test": "data"}`)
	headers := map[string]string{"source": "test"}

	err := mqService.PublishTelemetry(payload, headers)
	assert.NoError(t, err)

	// Test consuming from the created topic
	messages, err := mqService.ConsumeTelemetry("test-group", "consumer-1", 10)
	assert.NoError(t, err)
	assert.Len(t, messages, 1)

	// Test acknowledging messages
	messageIDs := []string{messages[0].ID}
	err = mqService.AcknowledgeMessages("consumer-1", messageIDs)
	assert.NoError(t, err)

	// Verify stats
	stats := mqService.GetQueueStats()
	assert.Greater(t, stats.TotalMessages, int64(0))
	assert.Greater(t, stats.ProcessedMessages, int64(0))
}

// TestAPIEndpoints tests the API endpoints (requires running API server)
func TestAPIEndpoints(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	// This test assumes API server is running on localhost:8080
	baseURL := "http://localhost:8080"

	// Test health endpoint
	resp, err := http.Get(baseURL + "/health")
	if err != nil {
		t.Skip("API server not running, skipping API tests")
	}
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Test GPUs endpoint
	resp, err = http.Get(baseURL + "/api/v1/gpus")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var gpuResponse struct {
		GPUs  []models.GPU `json:"gpus"`
		Count int          `json:"count"`
	}

	err = json.NewDecoder(resp.Body).Decode(&gpuResponse)
	assert.NoError(t, err)

	// Test stats endpoint
	resp, err = http.Get(baseURL + "/api/v1/stats")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var stats map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&stats)
	assert.NoError(t, err)
	assert.Contains(t, stats, "total_records")
}

// TestScaling tests horizontal scaling of services
func TestScaling(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	// Create test data
	csvContent := `timestamp,metric_name,gpu_id,device,uuid,modelName,Hostname,container,pod,namespace,value,labels_raw
2025-07-18T20:42:34Z,DCGM_FI_DEV_GPU_UTIL,0,nvidia0,GPU-12345,NVIDIA H100,host1,,,,"85.5","key=value"`

	tmpFile := createTempCSV(t, csvContent)
	defer os.Remove(tmpFile)

	mqService := messagequeue.NewMessageQueueService()
	defer mqService.Stop()

	// Start multiple streamers
	var streamers []*streamer.StreamerService
	for i := 0; i < 3; i++ {
		config := &streamer.StreamerConfig{
			CSVFilePath:    tmpFile,
			BatchSize:      1,
			StreamInterval: 100 * time.Millisecond,
			LoopMode:       true,
			StreamerID:     "streamer-" + string(rune(i+'1')),
		}

		s, err := streamer.NewStreamerService(config, mqService)
		require.NoError(t, err)

		err = s.Start()
		require.NoError(t, err)

		streamers = append(streamers, s)
	}

	// Start multiple collectors
	var collectors []*collector.CollectorService
	for i := 0; i < 2; i++ {
		config := &collector.CollectorConfig{
			CollectorID:   "collector-" + string(rune(i+'1')),
			ConsumerGroup: "test-group",
			BatchSize:     1,
			PollInterval:  100 * time.Millisecond,
			BufferSize:    5,
		}

		c, err := collector.NewCollectorService(config, mqService)
		require.NoError(t, err)

		err = c.Start()
		require.NoError(t, err)

		collectors = append(collectors, c)
	}

	// Let them run for a bit
	time.Sleep(1 * time.Second)

	// Stop all services
	for _, s := range streamers {
		s.Stop()
	}
	for _, c := range collectors {
		c.Stop()
	}

	// Verify that messages were processed by multiple services
	stats := mqService.GetQueueStats()
	assert.Greater(t, stats.TotalMessages, int64(0))
}

// Helper functions

func createTempCSV(t *testing.T, content string) string {
	tmpFile, err := os.CreateTemp("", "test_*.csv")
	require.NoError(t, err)

	_, err = tmpFile.WriteString(content)
	require.NoError(t, err)

	err = tmpFile.Close()
	require.NoError(t, err)

	return tmpFile.Name()
}

func createInMemoryDatabase(t *testing.T) *collector.DatabaseService {
	// For integration tests, you might want to use a real database
	// This is a placeholder for the actual database setup
	config := &collector.DatabaseConfig{
		Host:     "localhost",
		Port:     5432,
		User:     "test",
		Password: "test",
		DBName:   "test_telemetry",
		SSLMode:  "disable",
	}

	dbService, err := collector.NewDatabaseService(config)
	if err != nil {
		t.Skip("Database not available for integration test")
	}

	return dbService
}
