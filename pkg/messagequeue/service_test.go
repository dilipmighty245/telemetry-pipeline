package messagequeue

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMessageQueueService(t *testing.T) {
	service := NewMessageQueueService()
	defer service.Stop()

	// ctx := context.Background() // Unused in current tests

	t.Run("PublishTelemetry", func(t *testing.T) {
		payload := []byte(`{"gpu_id": "test-gpu", "metric": "utilization", "value": 85}`)
		headers := map[string]string{
			"gpu_id":      "test-gpu",
			"metric_name": "utilization",
			"timestamp":   time.Now().Format(time.RFC3339),
		}

		err := service.PublishTelemetry(payload, headers)
		assert.NoError(t, err)
	})

	t.Run("ConsumeTelemetry", func(t *testing.T) {
		// Publish test messages first
		for i := 0; i < 3; i++ {
			payload := []byte(`{"gpu_id": "test-gpu", "metric": "test", "value": ` + string(rune(i+'0')) + `}`)
			headers := map[string]string{
				"gpu_id": "test-gpu",
				"index":  string(rune(i + '0')),
			}
			err := service.PublishTelemetry(payload, headers)
			require.NoError(t, err)
		}

		// Consume messages
		messages, err := service.ConsumeTelemetry("test-group", "test-consumer", 5)
		assert.NoError(t, err)
		assert.GreaterOrEqual(t, len(messages), 3)

		// Verify message structure
		for _, msg := range messages[:3] {
			assert.NotEmpty(t, msg.ID)
			assert.Equal(t, "telemetry", msg.Topic)
			assert.NotEmpty(t, msg.Payload)
			assert.NotEmpty(t, msg.Headers)
		}
	})

	t.Run("AcknowledgeMessages", func(t *testing.T) {
		// Publish a test message
		payload := []byte(`{"gpu_id": "ack-gpu", "metric": "test", "value": 42}`)
		headers := map[string]string{"gpu_id": "ack-gpu"}
		err := service.PublishTelemetry(payload, headers)
		require.NoError(t, err)

		// Consume the message
		messages, err := service.ConsumeTelemetry("ack-group", "ack-consumer", 1)
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(messages), 1)

		// Acknowledge the message
		err = service.AcknowledgeMessages("ack-group", messages[:1])
		assert.NoError(t, err)
	})

	t.Run("GetQueueStats", func(t *testing.T) {
		stats := service.GetQueueStats()
		assert.NotNil(t, stats)
		assert.GreaterOrEqual(t, stats.TotalMessages, int64(0))
	})

	t.Run("CreateTopic", func(t *testing.T) {
		config := map[string]string{
			"max_size": "1000",
			"ttl":      "3600",
		}
		err := service.CreateTopic("custom-topic", config)
		assert.NoError(t, err)
	})

	t.Run("HealthCheck", func(t *testing.T) {
		healthy := service.Health()
		assert.True(t, healthy)
	})
}

func TestMessageQueueService_EdgeCases(t *testing.T) {
	service := NewMessageQueueService()
	defer service.Stop()

	t.Run("EmptyPayload", func(t *testing.T) {
		err := service.PublishTelemetry([]byte{}, nil)
		assert.NoError(t, err) // Should handle empty payload gracefully
	})

	t.Run("NilHeaders", func(t *testing.T) {
		payload := []byte(`{"test": "data"}`)
		err := service.PublishTelemetry(payload, nil)
		assert.NoError(t, err)
	})

	t.Run("ConsumeFromEmptyTopic", func(t *testing.T) {
		messages, err := service.ConsumeTelemetry("empty-group", "empty-consumer", 10)
		assert.NoError(t, err)
		assert.Len(t, messages, 0)
	})

	t.Run("AcknowledgeEmptyMessages", func(t *testing.T) {
		err := service.AcknowledgeMessages("test-group", []*Message{})
		assert.NoError(t, err)
	})

	t.Run("CreateTopicWithNilConfig", func(t *testing.T) {
		err := service.CreateTopic("nil-config-topic", nil)
		assert.NoError(t, err)
	})

	t.Run("StopService", func(t *testing.T) {
		tempService := NewMessageQueueService()

		// Publish a message
		err := tempService.PublishTelemetry([]byte(`{"test": "stop"}`), nil)
		assert.NoError(t, err)

		// Stop the service
		tempService.Stop()

		// Health should return false after stop
		healthy := tempService.Health()
		assert.False(t, healthy)
	})
}

func TestMessageQueueService_Concurrent(t *testing.T) {
	service := NewMessageQueueService()
	defer service.Stop()

	t.Run("ConcurrentPublish", func(t *testing.T) {
		const numGoroutines = 10
		const messagesPerGoroutine = 5

		errChan := make(chan error, numGoroutines*messagesPerGoroutine)

		// Start concurrent publishers
		for i := 0; i < numGoroutines; i++ {
			go func(id int) {
				for j := 0; j < messagesPerGoroutine; j++ {
					payload := []byte(`{"publisher": ` + string(rune(id+'0')) + `, "message": ` + string(rune(j+'0')) + `}`)
					headers := map[string]string{
						"publisher": string(rune(id + '0')),
						"message":   string(rune(j + '0')),
					}
					err := service.PublishTelemetry(payload, headers)
					errChan <- err
				}
			}(i)
		}

		// Check all publishes succeeded
		for i := 0; i < numGoroutines*messagesPerGoroutine; i++ {
			err := <-errChan
			assert.NoError(t, err)
		}
	})

	t.Run("ConcurrentConsume", func(t *testing.T) {
		const numConsumers = 5

		// Publish test messages first
		for i := 0; i < 20; i++ {
			payload := []byte(`{"concurrent": "test", "id": ` + string(rune(i+'0')) + `}`)
			err := service.PublishTelemetry(payload, nil)
			require.NoError(t, err)
		}

		messageChan := make(chan []*Message, numConsumers)

		// Start concurrent consumers
		for i := 0; i < numConsumers; i++ {
			go func(id int) {
				consumerID := "concurrent-consumer-" + string(rune(id+'0'))
				messages, err := service.ConsumeTelemetry("concurrent-group", consumerID, 10)
				if err == nil {
					messageChan <- messages
				} else {
					messageChan <- nil
				}
			}(i)
		}

		// Collect results
		totalMessages := 0
		for i := 0; i < numConsumers; i++ {
			messages := <-messageChan
			if messages != nil {
				totalMessages += len(messages)
			}
		}

		// Should have consumed some messages
		assert.Greater(t, totalMessages, 0)
	})
}

func BenchmarkMessageQueueService(b *testing.B) {
	service := NewMessageQueueService()
	defer service.Stop()

	payload := []byte(`{"gpu_id": "bench-gpu", "metric": "utilization", "value": 85}`)
	headers := map[string]string{"gpu_id": "bench-gpu"}

	b.Run("PublishTelemetry", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			err := service.PublishTelemetry(payload, headers)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("ConsumeTelemetry", func(b *testing.B) {
		// Pre-publish messages for consumption
		for i := 0; i < b.N; i++ {
			service.PublishTelemetry(payload, headers)
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			messages, err := service.ConsumeTelemetry("bench-group", "bench-consumer", 1)
			if err != nil {
				b.Fatal(err)
			}
			if len(messages) > 0 {
				// Acknowledge to clean up
				service.AcknowledgeMessages("bench-group", messages)
			}
		}
	})
}
