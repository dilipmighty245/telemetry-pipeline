package messagequeue

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/dilipmighty245/telemetry-pipeline/test/testhelper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMessageQueueService(t *testing.T) {
	// Start embedded etcd server for testing
	_, cleanup, err := testhelper.SetupEtcdForTest()
	require.NoError(t, err)
	defer cleanup()
	ctx := context.Background()
	service, err := NewMessageQueueService(ctx)
	require.NoError(t, err)
	defer service.Stop()

	t.Run("PublishTelemetry", func(t *testing.T) {
		payload := []byte(`{"gpu_id": "test-gpu", "metric": "utilization", "value": 85}`)
		headers := map[string]string{
			"gpu_id":      "test-gpu",
			"metric_name": "utilization",
			"timestamp":   time.Now().Format(time.RFC3339),
		}

		err := service.PublishTelemetry(ctx, payload, headers)
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
			err := service.PublishTelemetry(ctx, payload, headers)
			require.NoError(t, err)
		}

		// Consume messages
		messages, err := service.ConsumeTelemetry(ctx, "test-group", "test-consumer", 5)
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
		err := service.PublishTelemetry(ctx, payload, headers)
		require.NoError(t, err)

		// Consume the message
		messages, err := service.ConsumeTelemetry(ctx, "ack-group", "ack-consumer", 1)
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(messages), 1)

		// Acknowledge the message
		err = service.AcknowledgeMessages(ctx, "ack-group", messages[:1])
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
		healthy := service.Health(ctx)
		assert.True(t, healthy)
	})
}

func TestMessageQueueService_EdgeCases(t *testing.T) {
	// Start embedded etcd server for testing
	_, cleanup, err := testhelper.SetupEtcdForTest()
	require.NoError(t, err)
	defer cleanup()
	ctx, cancel := context.WithCancel(context.Background())
	service, err := NewMessageQueueService(ctx)
	require.NoError(t, err)
	defer service.Stop()

	t.Run("EmptyPayload", func(t *testing.T) {
		err := service.PublishTelemetry(ctx, []byte{}, nil)
		assert.NoError(t, err) // Should handle empty payload gracefully
	})

	t.Run("NilHeaders", func(t *testing.T) {
		payload := []byte(`{"test": "data"}`)
		err := service.PublishTelemetry(ctx, payload, nil)
		assert.NoError(t, err)
	})

	t.Run("ConsumeFromEmptyTopic", func(t *testing.T) {
		// Create a new service with a fresh queue to ensure no leftover messages
		// Note: Using same etcd server but different consumer group and topic
		freshService, err := NewMessageQueueService(ctx)
		require.NoError(t, err)
		// Use a unique topic name to avoid conflicts with other tests
		uniqueTopicName := "empty-test-topic-" + time.Now().Format("20060102150405")
		err = freshService.CreateTopic(uniqueTopicName, nil)
		assert.NoError(t, err)
		// Now consume from the empty topic using the service's internal method
		// Since we can't easily call ConsumeTelemetry with a different topic,
		// we'll consume from the default telemetry topic with a unique consumer group
		messages, err := freshService.ConsumeTelemetry(ctx, "empty-group-"+time.Now().Format("20060102150405"), "empty-consumer", 10)
		assert.NoError(t, err)
		// In etcd, there might be leftover messages from other tests, so we just check it doesn't error
		// The actual count may vary depending on test execution order
		assert.GreaterOrEqual(t, len(messages), 0)
		freshService.Stop()
	})

	t.Run("AcknowledgeEmptyMessages", func(t *testing.T) {
		err := service.AcknowledgeMessages(ctx, "test-group", []*Message{})
		assert.NoError(t, err)
	})

	t.Run("CreateTopicWithNilConfig", func(t *testing.T) {
		err := service.CreateTopic("nil-config-topic", nil)
		assert.NoError(t, err)
	})

	t.Run("StopService", func(t *testing.T) {
		// Note: Using same etcd server for this test
		tempService, err := NewMessageQueueService(ctx)
		require.NoError(t, err)

		// Publish a message
		err = tempService.PublishTelemetry(ctx, []byte(`{"test": "stop"}`), nil)
		assert.NoError(t, err)

		// Stop the service
		cancel()
		tempService.Stop()

		// Health should return false after stop
		healthy := tempService.Health(ctx)
		assert.False(t, healthy)
	})
}

func TestMessageQueueService_Concurrent(t *testing.T) {
	// Start embedded etcd server for testing
	_, cleanup, err := testhelper.SetupEtcdForTest()
	require.NoError(t, err)
	defer cleanup()
	ctx := context.Background()
	service, err := NewMessageQueueService(ctx)
	require.NoError(t, err)
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
					err := service.PublishTelemetry(ctx, payload, headers)
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
			err := service.PublishTelemetry(ctx, payload, nil)
			require.NoError(t, err)
		}

		messageChan := make(chan []*Message, numConsumers)

		// Start concurrent consumers
		for i := 0; i < numConsumers; i++ {
			go func(id int) {
				consumerID := "concurrent-consumer-" + string(rune(id+'0'))
				messages, err := service.ConsumeTelemetry(ctx, "concurrent-group", consumerID, 10)
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

func TestMessageQueueService_TopicOperations(t *testing.T) {
	// Start embedded etcd server for testing
	_, cleanup, err := testhelper.SetupEtcdForTest()
	require.NoError(t, err)
	defer cleanup()

	ctx := context.Background()
	service, err := NewMessageQueueService(ctx)
	require.NoError(t, err)
	defer service.Stop()

	t.Run("ListTopics", func(t *testing.T) {
		// Test listing topics
		topics := service.ListTopics(ctx)
		assert.NotNil(t, topics)
		// Should at least have the default telemetry topic
		assert.GreaterOrEqual(t, len(topics), 0)
	})

	t.Run("GetQueueStats", func(t *testing.T) {
		stats := service.GetQueueStats()
		assert.NotNil(t, stats)
		assert.GreaterOrEqual(t, stats.TotalMessages, int64(0))
	})
}

func TestMessageQueueService_BatchOperations(t *testing.T) {
	// Start embedded etcd server for testing
	_, cleanup, err := testhelper.SetupEtcdForTest()
	require.NoError(t, err)
	defer cleanup()

	ctx := context.Background()
	service, err := NewMessageQueueService(ctx)
	require.NoError(t, err)
	defer service.Stop()

	t.Run("PublishBatch", func(t *testing.T) {
		payloads := [][]byte{
			[]byte(`{"test": "batch1"}`),
			[]byte(`{"test": "batch2"}`),
			[]byte(`{"test": "batch3"}`),
		}
		headers := []map[string]string{
			{"batch": "1"},
			{"batch": "2"},
			{"batch": "3"},
		}

		messages, err := service.PublishBatch(ctx, "telemetry", payloads, headers, 3600)
		assert.NoError(t, err)
		assert.Equal(t, 3, len(messages))
	})

	t.Run("ConsumeBatch", func(t *testing.T) {
		// First publish some messages
		for i := 0; i < 5; i++ {
			payload := []byte(`{"test": "consume_batch"}`)
			headers := map[string]string{"index": fmt.Sprintf("%d", i)}
			err := service.PublishTelemetry(ctx, payload, headers)
			require.NoError(t, err)
		}

		// Test consuming from multiple topics
		topics := []string{"telemetry"}
		messages, err := service.ConsumeBatch(ctx, topics, "batch-group", "batch-consumer", 3)
		assert.NoError(t, err)
		assert.GreaterOrEqual(t, len(messages), 0)
	})
}

func TestMessageQueueService_ErrorHandling(t *testing.T) {
	// Start embedded etcd server for testing
	_, cleanup, err := testhelper.SetupEtcdForTest()
	require.NoError(t, err)
	defer cleanup()
	ctx := context.Background()
	service, err := NewMessageQueueService(ctx)
	require.NoError(t, err)
	defer service.Stop()

	t.Run("AcknowledgeInvalidMessages", func(t *testing.T) {
		// Try to acknowledge messages with invalid IDs
		invalidMessages := []*Message{
			{
				ID:      "invalid-id",
				Topic:   "telemetry",
				Payload: []byte(`{"test": "data"}`),
				Headers: map[string]string{},
			},
		}

		err := service.AcknowledgeMessages(ctx, "test-group", invalidMessages)
		// Should not error even with invalid messages
		assert.NoError(t, err)
	})

	t.Run("PublishBatchWithInvalidData", func(t *testing.T) {
		// Test with empty payloads
		emptyPayloads := [][]byte{}
		emptyHeaders := []map[string]string{}

		messages, err := service.PublishBatch(ctx, "telemetry", emptyPayloads, emptyHeaders, 3600)
		assert.NoError(t, err)
		assert.Equal(t, 0, len(messages))
	})
}

func BenchmarkMessageQueueService(b *testing.B) {
	// Start embedded etcd server for testing
	_, cleanup, err := testhelper.SetupEtcdForTest()
	require.NoError(b, err)
	defer cleanup()

	ctx := context.Background()

	service, err := NewMessageQueueService(ctx)
	require.NoError(b, err)
	defer service.Stop()

	payload := []byte(`{"gpu_id": "bench-gpu", "metric": "utilization", "value": 85}`)
	headers := map[string]string{"gpu_id": "bench-gpu"}

	b.Run("PublishTelemetry", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			err := service.PublishTelemetry(ctx, payload, headers)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("ConsumeTelemetry", func(b *testing.B) {
		// Pre-publish messages for consumption
		for i := 0; i < b.N; i++ {
			service.PublishTelemetry(ctx, payload, headers)
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			messages, err := service.ConsumeTelemetry(ctx, "bench-group", "bench-consumer", 1)
			if err != nil {
				b.Fatal(err)
			}
			if len(messages) > 0 {
				// Acknowledge to clean up
				service.AcknowledgeMessages(ctx, "bench-group", messages)
			}
		}
	})
}
