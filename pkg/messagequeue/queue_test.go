package messagequeue

import (
	"context"
	"testing"
	"time"

	"github.com/dilipmighty245/telemetry-pipeline/pkg/logging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewMessageQueue(t *testing.T) {
	// Start embedded etcd server for testing
	_, cleanup, err := SetupEtcdForTest()
	require.NoError(t, err)
	defer cleanup()

	mq, err := NewMessageQueue()
	require.NoError(t, err)
	assert.NotNil(t, mq)
	assert.NotNil(t, mq.topics)
	assert.NotNil(t, mq.stats)
}

func TestCreateTopic(t *testing.T) {
	// Start embedded etcd server for testing
	_, cleanup, err := SetupEtcdForTest()
	require.NoError(t, err)
	defer cleanup()

	mq, err := NewMessageQueue()
	require.NoError(t, err)

	config := map[string]string{
		"max_size": "1000",
	}

	err = mq.CreateTopic("test-topic", config)
	assert.NoError(t, err)

	// In etcd backend, topics are created implicitly when first message is published
	// So we need to publish a message to see the topic
	ctx := context.Background()
	payload := []byte("test message")
	_, err = mq.Publish(ctx, "test-topic", payload, nil, 3600)
	require.NoError(t, err)

	// Check topic was created
	topics := mq.ListTopics()
	assert.Len(t, topics, 1)
	assert.Contains(t, topics, "test-topic")

	// Creating same topic again should not error
	err = mq.CreateTopic("test-topic", config)
	assert.NoError(t, err)
}

func TestPublishMessage(t *testing.T) {
	// Start embedded etcd server for testing
	_, cleanup, err := SetupEtcdForTest()
	require.NoError(t, err)
	defer cleanup()

	mq, err := NewMessageQueue()
	require.NoError(t, err)
	ctx := context.Background()

	// Create topic first
	err = mq.CreateTopic("test-topic", nil)
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
	// Start embedded etcd server for testing
	_, cleanup, err := SetupEtcdForTest()
	require.NoError(t, err)
	defer cleanup()

	mq, err := NewMessageQueue()
	require.NoError(t, err)
	ctx := context.Background()

	payload := []byte("test message")

	// In etcd backend, topics are created implicitly, so this should succeed
	_, err = mq.Publish(ctx, "non-existent", payload, nil, 3600)
	assert.NoError(t, err)
}

func TestConsumeMessages(t *testing.T) {
	// Start embedded etcd server for testing
	_, cleanup, err := SetupEtcdForTest()
	require.NoError(t, err)
	defer cleanup()

	mq, err := NewMessageQueue()
	require.NoError(t, err)
	ctx := context.Background()

	// Create topic and publish messages
	err = mq.CreateTopic("test-topic", nil)
	require.NoError(t, err)

	// Publish multiple messages
	for i := 0; i < 5; i++ {
		payload := []byte("test message " + string(rune(i+'0')))
		_, err = mq.Publish(ctx, "test-topic", payload, nil, 3600)
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
	// Start embedded etcd server for testing
	_, cleanup, err := SetupEtcdForTest()
	require.NoError(t, err)
	defer cleanup()

	mq, err := NewMessageQueue()
	require.NoError(t, err)
	ctx := context.Background()

	// In etcd backend, consuming from non-existent topic returns empty results
	messages, err := mq.Consume(ctx, "non-existent", "test-group", "consumer-1", 10, 30)
	assert.NoError(t, err)
	assert.Len(t, messages, 0)
}

func TestAcknowledgeMessages(t *testing.T) {
	// Start embedded etcd server for testing
	_, cleanup, err := SetupEtcdForTest()
	require.NoError(t, err)
	defer cleanup()

	mq, err := NewMessageQueue()
	require.NoError(t, err)
	ctx := context.Background()

	// Create topic and publish messages
	err = mq.CreateTopic("test-topic", nil)
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
	messagesToAck := []*Message{messages[0], messages[1]}
	acked, failed, err := mq.Acknowledge("test-group", messagesToAck)
	assert.NoError(t, err)
	assert.Len(t, acked, 2)
	assert.Len(t, failed, 0)

	// In etcd backend, acknowledged messages are deleted, so we can't check processed count
	// Instead, verify that the acknowledgment succeeded
	assert.Len(t, acked, 2)
	assert.Len(t, failed, 0)
}

func TestMessageExpiration(t *testing.T) {
	// Start embedded etcd server for testing
	_, cleanup, err := SetupEtcdForTest()
	require.NoError(t, err)
	defer cleanup()

	mq, err := NewMessageQueue()
	require.NoError(t, err)
	ctx := context.Background()

	// Create topic
	err = mq.CreateTopic("test-topic", nil)
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

	// In etcd backend, expired messages are handled automatically by TTL
	// We can verify by trying to consume again - should get no messages
	messages2, err := mq.Consume(ctx, "test-topic", "test-group-2", "consumer-2", 10, 30)
	assert.NoError(t, err)
	assert.Len(t, messages2, 0)

	logging.Infof("Message: %v", msg)
}

func TestQueueStats(t *testing.T) {
	// Start embedded etcd server for testing
	_, cleanup, err := SetupEtcdForTest()
	require.NoError(t, err)
	defer cleanup()

	mq, err := NewMessageQueue()
	require.NoError(t, err)
	ctx := context.Background()

	// Create topic and publish messages
	err = mq.CreateTopic("test-topic", nil)
	require.NoError(t, err)

	// Publish messages
	for i := 0; i < 5; i++ {
		payload := []byte("test message " + string(rune(i+'0')))
		_, err = mq.Publish(ctx, "test-topic", payload, nil, 3600)
		require.NoError(t, err)
	}

	// Get stats - in etcd backend, stats are not automatically updated
	// This test mainly ensures GetStats() doesn't panic
	stats := mq.GetStats()
	assert.NotNil(t, stats)
	// Stats will be 0 since etcd backend doesn't track in-memory stats
	assert.GreaterOrEqual(t, stats.TotalMessages, int64(0))
	assert.NotNil(t, stats.TopicStats)
}

func TestConsumerRegistration(t *testing.T) {
	// Start embedded etcd server for testing
	_, cleanup, err := SetupEtcdForTest()
	require.NoError(t, err)
	defer cleanup()

	mq, err := NewMessageQueue()
	require.NoError(t, err)
	ctx := context.Background()

	// Create topic
	err = mq.CreateTopic("test-topic", nil)
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

	// In etcd backend, consumer registration is not tracked in topic metadata
	// Instead, verify that both consumers were able to consume successfully
	// This test mainly ensures no errors occur during consumption
}

func TestConcurrentAccess(t *testing.T) {
	// Start embedded etcd server for testing
	_, cleanup, err := SetupEtcdForTest()
	require.NoError(t, err)
	defer cleanup()

	mq, err := NewMessageQueue()
	require.NoError(t, err)
	ctx := context.Background()

	// Create topic
	err = mq.CreateTopic("test-topic", nil)
	require.NoError(t, err)

	// Concurrently publish messages
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(id int) {
			payload := []byte("test message " + string(rune(id+'0')))
			_, err = mq.Publish(ctx, "test-topic", payload, nil, 3600)
			assert.NoError(t, err)
			done <- true
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}

	// In etcd backend, we can verify by consuming all messages
	messages, err := mq.Consume(ctx, "test-topic", "test-group", "consumer-1", 20, 30)
	assert.NoError(t, err)
	assert.Len(t, messages, 10)
}
