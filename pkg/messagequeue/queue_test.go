package messagequeue

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewMessageQueue(t *testing.T) {
	mq := NewMessageQueue()
	assert.NotNil(t, mq)
	assert.NotNil(t, mq.topics)
	assert.NotNil(t, mq.stats)
}

func TestCreateTopic(t *testing.T) {
	mq := NewMessageQueue()

	config := map[string]string{
		"max_size": "1000",
	}

	err := mq.CreateTopic("test-topic", config)
	assert.NoError(t, err)

	// Check topic was created
	topics := mq.ListTopics()
	assert.Len(t, topics, 1)
	assert.Contains(t, topics, "test-topic")

	// Creating same topic again should not error
	err = mq.CreateTopic("test-topic", config)
	assert.NoError(t, err)
}

func TestPublishMessage(t *testing.T) {
	mq := NewMessageQueue()
	ctx := context.Background()

	// Create topic first
	err := mq.CreateTopic("test-topic", nil)
	require.NoError(t, err)

	// Publish message
	payload := []byte("test message")
	headers := map[string]string{"key": "value"}

	msg, err := mq.Publish(ctx, "test-topic", payload, headers, 3600)
	assert.NoError(t, err)
	assert.NotNil(t, msg)
	assert.Equal(t, "test-topic", msg.Topic)
	assert.Equal(t, payload, msg.Payload)
	assert.Equal(t, headers, msg.Headers)
	assert.False(t, msg.Processed)
}

func TestPublishToNonExistentTopic(t *testing.T) {
	mq := NewMessageQueue()
	ctx := context.Background()

	payload := []byte("test message")

	_, err := mq.Publish(ctx, "non-existent", payload, nil, 3600)
	assert.Error(t, err)
	assert.Equal(t, ErrTopicNotFound, err)
}

func TestConsumeMessages(t *testing.T) {
	mq := NewMessageQueue()
	ctx := context.Background()

	// Create topic and publish messages
	err := mq.CreateTopic("test-topic", nil)
	require.NoError(t, err)

	// Publish multiple messages
	for i := 0; i < 5; i++ {
		payload := []byte("test message " + string(rune(i+'0')))
		_, err := mq.Publish(ctx, "test-topic", payload, nil, 3600)
		require.NoError(t, err)
	}

	// Consume messages
	messages, err := mq.Consume(ctx, "test-topic", "test-group", "consumer-1", 3, 30)
	assert.NoError(t, err)
	assert.Len(t, messages, 3) // Should get max 3 messages

	// All messages should be unprocessed
	for _, msg := range messages {
		assert.False(t, msg.Processed)
		assert.Equal(t, "test-topic", msg.Topic)
	}
}

func TestConsumeFromNonExistentTopic(t *testing.T) {
	mq := NewMessageQueue()
	ctx := context.Background()

	_, err := mq.Consume(ctx, "non-existent", "test-group", "consumer-1", 10, 30)
	assert.Error(t, err)
	assert.Equal(t, ErrTopicNotFound, err)
}

func TestAcknowledgeMessages(t *testing.T) {
	mq := NewMessageQueue()
	ctx := context.Background()

	// Create topic and publish messages
	err := mq.CreateTopic("test-topic", nil)
	require.NoError(t, err)

	// Publish messages
	var publishedIDs []string
	for i := 0; i < 3; i++ {
		payload := []byte("test message " + string(rune(i+'0')))
		msg, err := mq.Publish(ctx, "test-topic", payload, nil, 3600)
		require.NoError(t, err)
		publishedIDs = append(publishedIDs, msg.ID)
	}

	// Consume messages
	messages, err := mq.Consume(ctx, "test-topic", "test-group", "consumer-1", 10, 30)
	require.NoError(t, err)
	require.Len(t, messages, 3)

	// Acknowledge first two messages
	messageIDs := []string{messages[0].ID, messages[1].ID}
	acked, failed, err := mq.Acknowledge("consumer-1", messageIDs)
	assert.NoError(t, err)
	assert.Len(t, acked, 2)
	assert.Len(t, failed, 0)

	// Check messages are marked as processed
	topics := mq.ListTopics()
	topic := topics["test-topic"]
	processedCount := 0
	for _, msg := range topic.Messages {
		if msg.Processed {
			processedCount++
		}
	}
	assert.Equal(t, 2, processedCount)
}

func TestMessageExpiration(t *testing.T) {
	mq := NewMessageQueue()
	ctx := context.Background()

	// Create topic
	err := mq.CreateTopic("test-topic", nil)
	require.NoError(t, err)

	// Publish message with very short TTL
	payload := []byte("test message")
	msg, err := mq.Publish(ctx, "test-topic", payload, nil, 1) // 1 second TTL
	require.NoError(t, err)

	// Wait for message to expire
	time.Sleep(2 * time.Second)

	// Try to consume - should get no messages
	messages, err := mq.Consume(ctx, "test-topic", "test-group", "consumer-1", 10, 30)
	assert.NoError(t, err)
	assert.Len(t, messages, 0)

	// Cleanup expired messages
	mq.CleanupExpiredMessages()

	// Check message was removed
	topics := mq.ListTopics()
	topic := topics["test-topic"]
	assert.Len(t, topic.Messages, 0)

	_ = msg // Use msg to avoid unused variable error
}

func TestQueueStats(t *testing.T) {
	mq := NewMessageQueue()
	ctx := context.Background()

	// Create topic and publish messages
	err := mq.CreateTopic("test-topic", nil)
	require.NoError(t, err)

	// Publish messages
	for i := 0; i < 5; i++ {
		payload := []byte("test message " + string(rune(i+'0')))
		_, err := mq.Publish(ctx, "test-topic", payload, nil, 3600)
		require.NoError(t, err)
	}

	// Get stats
	stats := mq.GetStats()
	assert.NotNil(t, stats)
	assert.Equal(t, int64(5), stats.TotalMessages)
	assert.Contains(t, stats.TopicStats, "test-topic")

	topicStats := stats.TopicStats["test-topic"]
	assert.Equal(t, int64(5), topicStats.MessageCount)
}

func TestConsumerRegistration(t *testing.T) {
	mq := NewMessageQueue()
	ctx := context.Background()

	// Create topic
	err := mq.CreateTopic("test-topic", nil)
	require.NoError(t, err)

	// Publish a message
	payload := []byte("test message")
	_, err = mq.Publish(ctx, "test-topic", payload, nil, 3600)
	require.NoError(t, err)

	// Consume with different consumers
	_, err = mq.Consume(ctx, "test-topic", "group-1", "consumer-1", 10, 30)
	assert.NoError(t, err)

	_, err = mq.Consume(ctx, "test-topic", "group-1", "consumer-2", 10, 30)
	assert.NoError(t, err)

	// Check consumers were registered
	topics := mq.ListTopics()
	topic := topics["test-topic"]
	assert.Len(t, topic.Consumers, 2)
	assert.Contains(t, topic.Consumers, "consumer-1")
	assert.Contains(t, topic.Consumers, "consumer-2")
}

func TestConcurrentAccess(t *testing.T) {
	mq := NewMessageQueue()
	ctx := context.Background()

	// Create topic
	err := mq.CreateTopic("test-topic", nil)
	require.NoError(t, err)

	// Concurrently publish messages
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(id int) {
			payload := []byte("test message " + string(rune(id+'0')))
			_, err := mq.Publish(ctx, "test-topic", payload, nil, 3600)
			assert.NoError(t, err)
			done <- true
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}

	// Check all messages were published
	topics := mq.ListTopics()
	topic := topics["test-topic"]
	assert.Len(t, topic.Messages, 10)
}
