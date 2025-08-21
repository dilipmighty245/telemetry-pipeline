package messagequeue

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMessageQueue_Enhanced(t *testing.T) {
	t.Run("NewMessageQueue_WithRedisStreams", func(t *testing.T) {
		// Test with Redis Streams if available
		if os.Getenv("REDIS_URL") != "" {
			mq := NewMessageQueue()
			assert.NotNil(t, mq)
			assert.NotNil(t, mq.redisStreamsBackend)
		}
	})

	t.Run("NewMessageQueue_WithoutRedis", func(t *testing.T) {
		// Temporarily unset REDIS_URL
		originalURL := os.Getenv("REDIS_URL")
		os.Unsetenv("REDIS_URL")
		defer func() {
			if originalURL != "" {
				os.Setenv("REDIS_URL", originalURL)
			}
		}()

		mq := NewMessageQueue()
		assert.NotNil(t, mq)
		assert.Nil(t, mq.redisStreamsBackend)
		assert.Nil(t, mq.redisBackend)
	})

	t.Run("Message_HeadersJSON", func(t *testing.T) {
		// Test empty headers
		msg := &Message{}
		json := msg.HeadersJSON()
		assert.Equal(t, "{}", json)

		// Test with headers
		msg.Headers = map[string]string{
			"key1": "value1",
			"key2": "value2",
		}
		json = msg.HeadersJSON()
		assert.Contains(t, json, `"key1":"value1"`)
		assert.Contains(t, json, `"key2":"value2"`)
		assert.True(t, json[0] == '{' && json[len(json)-1] == '}')
	})

	t.Run("CreateTopic_InMemory", func(t *testing.T) {
		mq := &MessageQueue{
			topics: make(map[string]*Topic),
			stats: &QueueStats{
				TopicStats: make(map[string]*TopicStats),
			},
		}

		config := map[string]string{
			"max_size": "100",
			"ttl":      "3600",
		}

		err := mq.CreateTopic("test-topic", config)
		assert.NoError(t, err)

		// Verify topic was created
		topics := mq.ListTopics()
		assert.Contains(t, topics, "test-topic")
		assert.Equal(t, config, topics["test-topic"].Config)
	})

	t.Run("Publish_InMemory", func(t *testing.T) {
		mq := &MessageQueue{
			topics: make(map[string]*Topic),
			stats: &QueueStats{
				TopicStats: make(map[string]*TopicStats),
			},
		}

		ctx := context.Background()

		// Create topic first
		err := mq.CreateTopic("test-topic", nil)
		require.NoError(t, err)

		payload := []byte("test payload")
		headers := map[string]string{"type": "test"}

		msg, err := mq.Publish(ctx, "test-topic", payload, headers, 3600)
		assert.NoError(t, err)
		assert.NotNil(t, msg)
		assert.Equal(t, "test-topic", msg.Topic)
		assert.Equal(t, payload, msg.Payload)
		assert.Equal(t, headers, msg.Headers)
	})

	t.Run("Consume_InMemory", func(t *testing.T) {
		mq := &MessageQueue{
			topics: make(map[string]*Topic),
			stats: &QueueStats{
				TopicStats: make(map[string]*TopicStats),
			},
		}

		ctx := context.Background()

		// Create topic first
		err := mq.CreateTopic("consume-topic", nil)
		require.NoError(t, err)

		// Publish test messages
		for i := 0; i < 3; i++ {
			payload := []byte(fmt.Sprintf("message-%d", i))
			_, err := mq.Publish(ctx, "consume-topic", payload, nil, 3600)
			require.NoError(t, err)
		}

		// Consume messages
		messages, err := mq.Consume(ctx, "consume-topic", "test-group", "consumer-1", 5, 1)
		assert.NoError(t, err)
		assert.Len(t, messages, 3)

		// Verify consumer was registered
		topics := mq.ListTopics()
		topic := topics["consume-topic"]
		assert.Contains(t, topic.Consumers, "consumer-1")
	})

	t.Run("Consume_NonExistentTopic", func(t *testing.T) {
		mq := &MessageQueue{
			topics: make(map[string]*Topic),
			stats: &QueueStats{
				TopicStats: make(map[string]*TopicStats),
			},
		}

		ctx := context.Background()
		messages, err := mq.Consume(ctx, "non-existent", "group", "consumer", 1, 1)
		assert.Error(t, err)
		assert.Equal(t, ErrTopicNotFound, err)
		assert.Nil(t, messages)
	})

	t.Run("Acknowledge_InMemory", func(t *testing.T) {
		mq := &MessageQueue{
			topics: make(map[string]*Topic),
			stats: &QueueStats{
				TopicStats: make(map[string]*TopicStats),
			},
		}

		ctx := context.Background()

		// Create and publish
		err := mq.CreateTopic("ack-topic", nil)
		require.NoError(t, err)

		msg, err := mq.Publish(ctx, "ack-topic", []byte("test"), nil, 3600)
		require.NoError(t, err)

		// Consume
		messages, err := mq.Consume(ctx, "ack-topic", "group", "consumer", 1, 1)
		require.NoError(t, err)
		require.Len(t, messages, 1)

		// Acknowledge
		acked, failed, err := mq.Acknowledge("group", messages)
		assert.NoError(t, err)
		assert.Len(t, acked, 1)
		assert.Len(t, failed, 0)
		assert.Equal(t, msg.ID, acked[0])

		// Verify message is marked as processed
		topics := mq.ListTopics()
		topic := topics["ack-topic"]
		assert.True(t, topic.Messages[0].Processed)
	})

	t.Run("Acknowledge_NonExistentMessage", func(t *testing.T) {
		mq := &MessageQueue{
			topics: make(map[string]*Topic),
			stats: &QueueStats{
				TopicStats: make(map[string]*TopicStats),
			},
		}

		fakeMessage := &Message{
			ID:    "non-existent",
			Topic: "fake-topic",
		}

		acked, failed, err := mq.Acknowledge("group", []*Message{fakeMessage})
		assert.NoError(t, err)
		assert.Len(t, acked, 0)
		assert.Len(t, failed, 1)
		assert.Equal(t, "non-existent", failed[0])
	})

	t.Run("GetStats", func(t *testing.T) {
		mq := &MessageQueue{
			topics: make(map[string]*Topic),
			stats: &QueueStats{
				TotalMessages:     10,
				ProcessedMessages: 5,
				FailedMessages:    1,
				TopicStats:        make(map[string]*TopicStats),
			},
		}

		stats := mq.GetStats()
		assert.NotNil(t, stats)
		assert.Equal(t, int64(10), stats.TotalMessages)
		assert.Equal(t, int64(5), stats.ProcessedMessages)
		assert.Equal(t, int64(1), stats.FailedMessages)
		assert.Equal(t, int64(4), stats.PendingMessages) // 10 - 5 - 1
	})

	t.Run("ListTopics", func(t *testing.T) {
		mq := &MessageQueue{
			topics: make(map[string]*Topic),
			stats: &QueueStats{
				TopicStats: make(map[string]*TopicStats),
			},
		}

		// Initially empty
		topics := mq.ListTopics()
		assert.Len(t, topics, 0)

		// Create topics
		err := mq.CreateTopic("topic1", nil)
		require.NoError(t, err)
		err = mq.CreateTopic("topic2", nil)
		require.NoError(t, err)

		topics = mq.ListTopics()
		assert.Len(t, topics, 2)
		assert.Contains(t, topics, "topic1")
		assert.Contains(t, topics, "topic2")
	})
}

func TestMessageQueue_ConcurrencySafety(t *testing.T) {
	mq := &MessageQueue{
		topics: make(map[string]*Topic),
		stats: &QueueStats{
			TopicStats: make(map[string]*TopicStats),
		},
	}

	ctx := context.Background()
	const numGoroutines = 10
	const operationsPerGoroutine = 20

	t.Run("ConcurrentPublish", func(t *testing.T) {
		err := mq.CreateTopic("concurrent-topic", nil)
		require.NoError(t, err)

		var wg sync.WaitGroup
		errChan := make(chan error, numGoroutines*operationsPerGoroutine)

		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				for j := 0; j < operationsPerGoroutine; j++ {
					payload := []byte(fmt.Sprintf("msg-%d-%d", id, j))
					_, err := mq.Publish(ctx, "concurrent-topic", payload, nil, 3600)
					errChan <- err
				}
			}(i)
		}

		wg.Wait()
		close(errChan)

		// Check all operations succeeded
		for err := range errChan {
			assert.NoError(t, err)
		}

		// Verify correct number of messages
		topics := mq.ListTopics()
		topic := topics["concurrent-topic"]
		assert.Len(t, topic.Messages, numGoroutines*operationsPerGoroutine)
	})

	t.Run("ConcurrentConsume", func(t *testing.T) {
		// Publish messages first
		err := mq.CreateTopic("consume-concurrent", nil)
		require.NoError(t, err)

		for i := 0; i < 50; i++ {
			payload := []byte(fmt.Sprintf("consume-msg-%d", i))
			_, err := mq.Publish(ctx, "consume-concurrent", payload, nil, 3600)
			require.NoError(t, err)
		}

		var wg sync.WaitGroup
		totalConsumed := make(chan int, numGoroutines)

		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				consumerID := fmt.Sprintf("consumer-%d", id)
				_, err := mq.Consume(ctx, "consume-concurrent", "group", consumerID, 10, 1)
				if err != nil {
					totalConsumed <- 0
				} else {
					totalConsumed <- 1 // Just count successful consumption
				}
			}(i)
		}

		wg.Wait()
		close(totalConsumed)

		total := 0
		for count := range totalConsumed {
			total += count
		}

		// Should have consumed some messages
		assert.Greater(t, total, 0)
	})
}

func TestMessageQueue_MessageExpiry(t *testing.T) {
	mq := &MessageQueue{
		topics: make(map[string]*Topic),
		stats: &QueueStats{
			TopicStats: make(map[string]*TopicStats),
		},
	}

	ctx := context.Background()
	err := mq.CreateTopic("expiry-topic", nil)
	require.NoError(t, err)

	t.Run("ExpiredMessage", func(t *testing.T) {
		// Publish message with very short TTL
		msg, err := mq.Publish(ctx, "expiry-topic", []byte("expiring"), nil, 1)
		require.NoError(t, err)

		// Wait for message to expire
		time.Sleep(2 * time.Second)

		// Try to consume - should not get expired message
		_, err = mq.Consume(ctx, "expiry-topic", "group", "consumer", 1, 1)
		assert.NoError(t, err)

		// Find our message and check if it's expired
		topics := mq.ListTopics()
		topic := topics["expiry-topic"]
		found := false
		for _, topicMsg := range topic.Messages {
			if topicMsg.ID == msg.ID {
				found = true
				assert.True(t, time.Now().After(topicMsg.ExpiresAt))
				break
			}
		}
		assert.True(t, found, "Message should exist in topic")
	})
}

func TestGenerateMessageID(t *testing.T) {
	id1 := generateMessageID()
	id2 := generateMessageID()

	assert.NotEmpty(t, id1)
	assert.NotEmpty(t, id2)
	assert.NotEqual(t, id1, id2) // Should generate unique IDs
	assert.Contains(t, id1, "-") // Should contain timestamp separator
}

func TestMessageQueue_HelperFunctions(t *testing.T) {
	mq := &MessageQueue{
		topics: make(map[string]*Topic),
		stats: &QueueStats{
			TopicStats: make(map[string]*TopicStats),
		},
	}

	t.Run("registerConsumer", func(t *testing.T) {
		topic := &Topic{
			Name:      "test-topic",
			Messages:  []*Message{},
			Consumers: make(map[string]*Consumer),
			Config:    make(map[string]string),
			CreatedAt: time.Now(),
		}

		// Register new consumer
		consumer := mq.registerConsumer(topic, "group1", "consumer1")
		assert.NotNil(t, consumer)
		assert.Equal(t, "consumer1", consumer.ID)
		assert.Equal(t, "group1", consumer.Group)
		assert.Contains(t, topic.Consumers, "consumer1")

		// Register existing consumer (should update LastSeen)
		oldLastSeen := consumer.LastSeen
		time.Sleep(10 * time.Millisecond)
		consumer2 := mq.registerConsumer(topic, "group1", "consumer1")
		assert.Equal(t, consumer, consumer2) // Should be same instance
		assert.True(t, consumer2.LastSeen.After(oldLastSeen))
	})

	t.Run("isMessageProcessedByConsumer", func(t *testing.T) {
		consumer := &Consumer{
			ID:           "test-consumer",
			Group:        "test-group",
			LastSeen:     time.Now(),
			ProcessedIDs: []string{"msg1", "msg2", "msg3"},
		}

		// Test processed message
		assert.True(t, mq.isMessageProcessedByConsumer(consumer, "msg2"))

		// Test unprocessed message
		assert.False(t, mq.isMessageProcessedByConsumer(consumer, "msg4"))
	})

	t.Run("updateStats", func(t *testing.T) {
		mq.stats.TopicStats["test-topic"] = &TopicStats{
			MessageCount:   0,
			ConsumerCount:  0,
			LastMessage:    time.Now(),
			ProcessedCount: 0,
			PendingCount:   0,
		}

		mq.updateStats("test-topic", 5, 2, 1, 0)

		stats := mq.stats.TopicStats["test-topic"]
		assert.Equal(t, int64(5), stats.MessageCount)
		assert.Equal(t, int64(2), stats.ProcessedCount)

		// Check global stats
		assert.Equal(t, int64(5), mq.stats.TotalMessages)
		assert.Equal(t, int64(2), mq.stats.ProcessedMessages)
		assert.Equal(t, int64(1), mq.stats.FailedMessages)
	})
}
