package messagequeue

import (
	"context"
	"testing"
	"time"

	"github.com/dilipmighty245/telemetry-pipeline/test/testhelper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMessageStruct(t *testing.T) {
	msg := &Message{
		ID:        "test-id",
		Topic:     "test-topic",
		Payload:   []byte("test payload"),
		Timestamp: time.Now(),
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(time.Hour),
		Headers:   map[string]string{"key": "value"},
		Processed: false,
		Retries:   0,
	}

	assert.Equal(t, "test-id", msg.ID)
	assert.Equal(t, "test-topic", msg.Topic)
	assert.Equal(t, []byte("test payload"), msg.Payload)
	assert.Equal(t, map[string]string{"key": "value"}, msg.Headers)
	assert.False(t, msg.Processed)
	assert.Equal(t, 0, msg.Retries)
}

func TestMessageHeadersJSON(t *testing.T) {
	msg := &Message{
		Headers: map[string]string{
			"key1": "value1",
			"key2": "value2",
		},
	}

	json := msg.HeadersJSON()
	assert.Contains(t, json, "key1")
	assert.Contains(t, json, "value1")
	assert.Contains(t, json, "key2")
	assert.Contains(t, json, "value2")

	// Test empty headers
	emptyMsg := &Message{}
	assert.Equal(t, "{}", emptyMsg.HeadersJSON())
}

func TestTopicStruct(t *testing.T) {
	topic := &Topic{
		Name:      "test-topic",
		Messages:  make([]*Message, 0),
		Consumers: make(map[string]*Consumer),
		Config:    map[string]string{"max_size": "1000"},
		CreatedAt: time.Now(),
		maxSize:   1000,
	}

	assert.Equal(t, "test-topic", topic.Name)
	assert.NotNil(t, topic.Messages)
	assert.NotNil(t, topic.Consumers)
	assert.Equal(t, "1000", topic.Config["max_size"])
	assert.Equal(t, 1000, topic.maxSize)
}

func TestConsumerStruct(t *testing.T) {
	consumer := &Consumer{
		ID:           "consumer-1",
		Group:        "group-1",
		LastSeen:     time.Now(),
		ProcessedIDs: make([]string, 0),
	}

	assert.Equal(t, "consumer-1", consumer.ID)
	assert.Equal(t, "group-1", consumer.Group)
	assert.NotNil(t, consumer.ProcessedIDs)
}

func TestQueueStatsStruct(t *testing.T) {
	stats := &QueueStats{
		TotalMessages:     100,
		PendingMessages:   50,
		ProcessedMessages: 45,
		FailedMessages:    5,
		TopicStats:        make(map[string]*TopicStats),
		LastUpdated:       time.Now(),
	}

	assert.Equal(t, int64(100), stats.TotalMessages)
	assert.Equal(t, int64(50), stats.PendingMessages)
	assert.Equal(t, int64(45), stats.ProcessedMessages)
	assert.Equal(t, int64(5), stats.FailedMessages)
	assert.NotNil(t, stats.TopicStats)
}

func TestTopicStatsStruct(t *testing.T) {
	topicStats := &TopicStats{
		MessageCount:   10,
		ConsumerCount:  2,
		LastMessage:    time.Now(),
		ProcessedCount: 8,
		PendingCount:   2,
	}

	assert.Equal(t, int64(10), topicStats.MessageCount)
	assert.Equal(t, 2, topicStats.ConsumerCount)
	assert.Equal(t, int64(8), topicStats.ProcessedCount)
	assert.Equal(t, int64(2), topicStats.PendingCount)
}

func TestNewMessageQueueBasic(t *testing.T) {
	// Start embedded etcd server for testing
	_, cleanup, err := testhelper.SetupEtcdForTest()
	require.NoError(t, err)
	defer cleanup()

	mq, err := NewMessageQueue(context.Background())
	require.NoError(t, err)
	assert.NotNil(t, mq)
	assert.NotNil(t, mq.topics)
	assert.NotNil(t, mq.stats)
	assert.NotNil(t, mq.stats.TopicStats)
}

func TestCreateTopicBasic(t *testing.T) {
	// Start embedded etcd server for testing
	_, cleanup, err := testhelper.SetupEtcdForTest()
	require.NoError(t, err)
	defer cleanup()

	mq, err := NewMessageQueue(context.Background())
	require.NoError(t, err)

	config := map[string]string{
		"max_size": "500",
	}

	err = mq.CreateTopic("test-topic", config)
	assert.NoError(t, err)

	// For etcd backend, topics are created implicitly when first message is published
	// This test mainly ensures no errors occur during topic creation.
}

func TestListTopicsBasic(t *testing.T) {
	// Start embedded etcd server for testing
	_, cleanup, err := testhelper.SetupEtcdForTest()
	require.NoError(t, err)
	defer cleanup()

	mq, err := NewMessageQueue(context.Background())
	require.NoError(t, err)
	ctx := context.Background()

	topics := mq.ListTopics(ctx)
	assert.NotNil(t, topics)
	assert.Equal(t, 0, len(topics))
}

func TestGetStatsBasic(t *testing.T) {
	// Start embedded etcd server for testing
	_, cleanup, err := testhelper.SetupEtcdForTest()
	require.NoError(t, err)
	defer cleanup()

	mq, err := NewMessageQueue(context.Background())
	require.NoError(t, err)

	stats := mq.GetStats()
	assert.NotNil(t, stats)
	assert.Equal(t, int64(0), stats.TotalMessages)
	assert.Equal(t, int64(0), stats.PendingMessages)
	assert.Equal(t, int64(0), stats.ProcessedMessages)
	assert.Equal(t, int64(0), stats.FailedMessages)
	assert.NotNil(t, stats.TopicStats)
}

func TestCleanupExpiredMessagesBasic(t *testing.T) {
	// Start embedded etcd server for testing
	_, cleanup, err := testhelper.SetupEtcdForTest()
	require.NoError(t, err)
	defer cleanup()

	mq, err := NewMessageQueue(context.Background())
	require.NoError(t, err)

	// Should not panic even with no topics
	assert.NotPanics(t, func() {
		mq.CleanupExpiredMessages()
	})
}

func TestCloseBasic(t *testing.T) {
	// Start embedded etcd server for testing
	_, cleanup, err := testhelper.SetupEtcdForTest()
	require.NoError(t, err)
	defer cleanup()

	mq, err := NewMessageQueue(context.Background())
	require.NoError(t, err)

	err = mq.Close()
	assert.NoError(t, err)
}

func TestErrorConstants(t *testing.T) {
	assert.NotNil(t, ErrTopicNotFound)
	assert.NotNil(t, ErrQueueFull)
	assert.NotNil(t, ErrConsumerNotFound)
	assert.NotNil(t, ErrMessageNotFound)

	assert.Equal(t, "topic not found", ErrTopicNotFound.Error())
	assert.Equal(t, "queue is full", ErrQueueFull.Error())
	assert.Equal(t, "consumer not found", ErrConsumerNotFound.Error())
	assert.Equal(t, "message not found", ErrMessageNotFound.Error())
}

func TestPublishBasic(t *testing.T) {
	// Start embedded etcd server for testing
	_, cleanup, err := testhelper.SetupEtcdForTest()
	require.NoError(t, err)
	defer cleanup()

	mq, err := NewMessageQueue(context.Background())
	require.NoError(t, err)
	ctx := context.Background()

	// With etcd backend, publishing to non-existent topics works (they're created implicitly)
	payload := []byte("test message")
	headers := map[string]string{"key": "value"}

	msg, err := mq.Publish(ctx, "test-topic", payload, headers, 3600)
	assert.NoError(t, err)
	assert.NotNil(t, msg)
	assert.Equal(t, "test-topic", msg.Topic)
	assert.Equal(t, payload, msg.Payload)
	assert.Equal(t, headers, msg.Headers)
}

func TestConsumeBasic(t *testing.T) {
	// Start embedded etcd server for testing
	_, cleanup, err := testhelper.SetupEtcdForTest()
	require.NoError(t, err)
	defer cleanup()

	mq, err := NewMessageQueue(context.Background())
	require.NoError(t, err)
	ctx := context.Background()

	// With etcd backend, consuming from non-existent topics returns empty results
	messages, err := mq.Consume(ctx, "test-topic", "test-group", "consumer-1", 10, 30)
	assert.NoError(t, err)
	assert.Len(t, messages, 0)
}

func TestAcknowledgeBasic(t *testing.T) {
	// Start embedded etcd server for testing
	_, cleanup, err := testhelper.SetupEtcdForTest()
	require.NoError(t, err)
	defer cleanup()

	mq, err := NewMessageQueue(context.Background())
	require.NoError(t, err)

	// Test with empty message list
	messages := []*Message{}
	acked, failed, err := mq.Acknowledge(context.Background(), "test-group", messages)
	assert.NoError(t, err)
	assert.Equal(t, 0, len(acked))
	assert.Equal(t, 0, len(failed))
}
