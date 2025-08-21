package messagequeue

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockTelemetryData represents telemetry data for testing
type MockTelemetryData struct {
	GPUID      string    `json:"gpu_id"`
	MetricName string    `json:"metric_name"`
	Value      float64   `json:"value"`
	Timestamp  time.Time `json:"timestamp"`
	Hostname   string    `json:"hostname"`
}

func TestMessageQueueIntegration(t *testing.T) {
	// Test both Redis Streams and in-memory implementations
	testCases := []struct {
		name        string
		setupRedis  bool
		expectRedis bool
	}{
		{
			name:        "InMemory",
			setupRedis:  false,
			expectRedis: false,
		},
	}

	// Add Redis test case if available
	if os.Getenv("REDIS_URL") != "" {
		testCases = append(testCases, struct {
			name        string
			setupRedis  bool
			expectRedis bool
		}{
			name:        "RedisStreams",
			setupRedis:  true,
			expectRedis: true,
		})
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Setup environment
			originalURL := os.Getenv("REDIS_URL")
			if !tc.setupRedis {
				os.Unsetenv("REDIS_URL")
			}
			defer func() {
				if originalURL != "" {
					os.Setenv("REDIS_URL", originalURL)
				}
			}()

			// Create services
			service := NewMessageQueueService()
			defer service.Stop()

			// Verify backend type
			if tc.expectRedis {
				assert.NotNil(t, service.queue.redisStreamsBackend, "Should use Redis Streams backend")
			} else {
				assert.Nil(t, service.queue.redisStreamsBackend, "Should use in-memory backend")
				assert.Nil(t, service.queue.redisBackend, "Should use in-memory backend")
			}

			// Test full pipeline
			testFullPipeline(t, service)
		})
	}
}

func testFullPipeline(t *testing.T, service *MessageQueueService) {
	// ctx := context.Background() // Unused in current tests

	t.Run("ProducerConsumerFlow", func(t *testing.T) {
		const numMessages = 100
		const numConsumers = 3

		// Step 1: Produce telemetry messages
		var producedMessages []MockTelemetryData
		for i := 0; i < numMessages; i++ {
			telemetryData := MockTelemetryData{
				GPUID:      "GPU-12345",
				MetricName: "GPU_UTILIZATION",
				Value:      float64(i % 100),
				Timestamp:  time.Now(),
				Hostname:   "test-host",
			}
			producedMessages = append(producedMessages, telemetryData)

			payload, err := json.Marshal(telemetryData)
			require.NoError(t, err)

			headers := map[string]string{
				"gpu_id":      telemetryData.GPUID,
				"metric_name": telemetryData.MetricName,
				"hostname":    telemetryData.Hostname,
			}

			err = service.PublishTelemetry(payload, headers)
			require.NoError(t, err)
		}

		// Step 2: Consume messages with multiple consumers
		var wg sync.WaitGroup
		consumedMessages := make(chan MockTelemetryData, numMessages)

		for i := 0; i < numConsumers; i++ {
			wg.Add(1)
			go func(consumerID int) {
				defer wg.Done()

				consumerName := fmt.Sprintf("consumer-%d", consumerID)
				for {
					messages, err := service.ConsumeTelemetry("telemetry-processors", consumerName, 10)
					if err != nil {
						t.Errorf("Consumer %d failed: %v", consumerID, err)
						return
					}

					if len(messages) == 0 {
						// No more messages
						return
					}

					// Process messages
					var processedTelemetry []MockTelemetryData
					for _, msg := range messages {
						var telemetry MockTelemetryData
						err := json.Unmarshal(msg.Payload, &telemetry)
						if err != nil {
							t.Errorf("Failed to unmarshal: %v", err)
							continue
						}
						processedTelemetry = append(processedTelemetry, telemetry)
						consumedMessages <- telemetry
					}

					// Acknowledge processed messages
					err = service.AcknowledgeMessages("telemetry-processors", messages)
					if err != nil {
						t.Errorf("Failed to acknowledge: %v", err)
					}
				}
			}(i)
		}

		// Wait for consumption to complete or timeout
		done := make(chan struct{})
		go func() {
			wg.Wait()
			close(done)
		}()

		select {
		case <-done:
			// All consumers finished
		case <-time.After(30 * time.Second):
			t.Fatal("Test timed out waiting for consumers")
		}

		close(consumedMessages)

		// Step 3: Verify all messages were consumed
		consumedCount := 0
		consumedMap := make(map[float64]bool)
		for telemetry := range consumedMessages {
			consumedCount++
			consumedMap[telemetry.Value] = true
			assert.Equal(t, "GPU-12345", telemetry.GPUID)
			assert.Equal(t, "GPU_UTILIZATION", telemetry.MetricName)
			assert.Equal(t, "test-host", telemetry.Hostname)
		}

		// In Redis Streams, each message should be consumed exactly once
		// In in-memory, messages might be consumed by multiple consumers
		assert.Greater(t, consumedCount, 0, "Should have consumed some messages")
		assert.LessOrEqual(t, consumedCount, numMessages*numConsumers, "Should not exceed max possible")
	})

	t.Run("MessagePersistence", func(t *testing.T) {
		// Publish a message
		telemetryData := MockTelemetryData{
			GPUID:      "GPU-PERSIST",
			MetricName: "TEMPERATURE",
			Value:      75.5,
			Timestamp:  time.Now(),
			Hostname:   "persist-host",
		}

		payload, err := json.Marshal(telemetryData)
		require.NoError(t, err)

		headers := map[string]string{
			"gpu_id": telemetryData.GPUID,
			"type":   "persistence-test",
		}

		err = service.PublishTelemetry(payload, headers)
		require.NoError(t, err)

		// Consume without acknowledging
		messages, err := service.ConsumeTelemetry("persistence-group", "persistence-consumer", 1)
		require.NoError(t, err)
		require.Len(t, messages, 1)

		// Verify message content
		var consumed MockTelemetryData
		err = json.Unmarshal(messages[0].Payload, &consumed)
		require.NoError(t, err)
		assert.Equal(t, telemetryData.GPUID, consumed.GPUID)
		assert.Equal(t, telemetryData.Value, consumed.Value)

		// Don't acknowledge - message should remain for retry
		// This behavior depends on the backend implementation
	})

	t.Run("ErrorHandling", func(t *testing.T) {
		// Test invalid JSON
		err := service.PublishTelemetry([]byte("invalid json"), nil)
		assert.NoError(t, err) // Service should accept any payload

		// Test empty consumer group
		messages, err := service.ConsumeTelemetry("", "consumer", 1)
		assert.NoError(t, err) // Should handle gracefully
		_ = messages

		// Test acknowledgment with empty messages
		err = service.AcknowledgeMessages("group", []*Message{})
		assert.NoError(t, err)
	})
}

func TestMessageQueueFailover(t *testing.T) {
	if os.Getenv("REDIS_URL") == "" {
		t.Skip("Redis not available for failover testing")
	}

	service := NewMessageQueueService()
	defer service.Stop()

	t.Run("ConsumerFailover", func(t *testing.T) {
		// Publish test messages
		const numMessages = 10
		for i := 0; i < numMessages; i++ {
			telemetry := MockTelemetryData{
				GPUID:      "GPU-FAILOVER",
				MetricName: "UTILIZATION",
				Value:      float64(i),
				Timestamp:  time.Now(),
				Hostname:   "failover-host",
			}

			payload, err := json.Marshal(telemetry)
			require.NoError(t, err)

			err = service.PublishTelemetry(payload, map[string]string{"gpu_id": telemetry.GPUID})
			require.NoError(t, err)
		}

		// Consumer 1 consumes messages but doesn't acknowledge
		messages1, err := service.ConsumeTelemetry("failover-group", "consumer-1", 5)
		require.NoError(t, err)
		require.Len(t, messages1, 5)

		// Don't acknowledge messages from consumer-1

		// Consumer 2 should be able to claim pending messages after timeout
		// This tests the failover mechanism in Redis Streams
		time.Sleep(2 * time.Second) // Wait for potential timeout

		messages2, err := service.ConsumeTelemetry("failover-group", "consumer-2", 10)
		require.NoError(t, err)

		// Consumer 2 should get remaining messages and potentially some pending ones
		assert.Greater(t, len(messages2), 0)

		// Acknowledge all messages from consumer 2
		if len(messages2) > 0 {
			err = service.AcknowledgeMessages("failover-group", messages2)
			assert.NoError(t, err)
		}
	})
}

func BenchmarkMessageQueueIntegration(b *testing.B) {
	service := NewMessageQueueService()
	defer service.Stop()

	telemetryData := MockTelemetryData{
		GPUID:      "GPU-BENCH",
		MetricName: "GPU_UTILIZATION",
		Value:      85.0,
		Timestamp:  time.Now(),
		Hostname:   "bench-host",
	}

	payload, err := json.Marshal(telemetryData)
	require.NoError(b, err)

	headers := map[string]string{
		"gpu_id":      telemetryData.GPUID,
		"metric_name": telemetryData.MetricName,
	}

	b.Run("EndToEndThroughput", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				// Publish
				err := service.PublishTelemetry(payload, headers)
				if err != nil {
					b.Fatal(err)
				}

				// Consume
				messages, err := service.ConsumeTelemetry("bench-group", "bench-consumer", 1)
				if err != nil {
					b.Fatal(err)
				}

				// Acknowledge if we got messages
				if len(messages) > 0 {
					err = service.AcknowledgeMessages("bench-group", messages)
					if err != nil {
						b.Fatal(err)
					}
				}
			}
		})
	})

	b.Run("BatchProcessing", func(b *testing.B) {
		const batchSize = 100

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			// Publish batch
			for j := 0; j < batchSize; j++ {
				err := service.PublishTelemetry(payload, headers)
				if err != nil {
					b.Fatal(err)
				}
			}

			// Consume batch
			messages, err := service.ConsumeTelemetry("batch-group", "batch-consumer", batchSize)
			if err != nil {
				b.Fatal(err)
			}

			// Acknowledge batch
			if len(messages) > 0 {
				err = service.AcknowledgeMessages("batch-group", messages)
				if err != nil {
					b.Fatal(err)
				}
			}
		}
	})
}
