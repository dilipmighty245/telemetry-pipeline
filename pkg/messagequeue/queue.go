package messagequeue

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/dilipmighty245/telemetry-pipeline/pkg/logging"
)

var (
	ErrTopicNotFound    = errors.New("topic not found")
	ErrQueueFull        = errors.New("queue is full")
	ErrConsumerNotFound = errors.New("consumer not found")
	ErrMessageNotFound  = errors.New("message not found")
)

// Message represents a message in the queue
type Message struct {
	ID        string            `json:"id"`
	Topic     string            `json:"topic"`
	Payload   []byte            `json:"payload"`
	Timestamp time.Time         `json:"timestamp"`
	CreatedAt time.Time         `json:"created_at"`
	ExpiresAt time.Time         `json:"expires_at"`
	Headers   map[string]string `json:"headers"`
	Processed bool              `json:"processed"`
	Retries   int               `json:"retries"`

	// Redis Streams specific fields
	StreamID  string `json:"stream_id,omitempty"`  // Redis Stream message ID
	StreamKey string `json:"stream_key,omitempty"` // Redis Stream key
}

// HeadersJSON returns headers as JSON string for Redis storage
func (m *Message) HeadersJSON() string {
	if len(m.Headers) == 0 {
		return "{}"
	}

	// Simple JSON serialization for headers
	result := "{"
	first := true
	for k, v := range m.Headers {
		if !first {
			result += ","
		}
		result += fmt.Sprintf(`"%s":"%s"`, k, v)
		first = false
	}
	result += "}"
	return result
}

// Topic represents a message topic/queue
type Topic struct {
	Name      string               `json:"name"`
	Messages  []*Message           `json:"messages"`
	Consumers map[string]*Consumer `json:"consumers"`
	Config    map[string]string    `json:"config"`
	CreatedAt time.Time            `json:"created_at"`
	mu        sync.RWMutex
	maxSize   int
}

// Consumer represents a message consumer
type Consumer struct {
	ID           string    `json:"id"`
	Group        string    `json:"group"`
	LastSeen     time.Time `json:"last_seen"`
	ProcessedIDs []string  `json:"processed_ids"`
	mu           sync.RWMutex
}

// MessageQueue represents the message queue (in-memory or Redis Streams-backed)
type MessageQueue struct {
	topics              map[string]*Topic
	mu                  sync.RWMutex
	stats               *QueueStats
	redisBackend        *RedisBackend        // Legacy Redis lists backend (deprecated)
	redisStreamsBackend *RedisStreamsBackend // New Redis Streams backend for production
}

// QueueStats represents queue statistics
type QueueStats struct {
	TotalMessages     int64                  `json:"total_messages"`
	PendingMessages   int64                  `json:"pending_messages"`
	ProcessedMessages int64                  `json:"processed_messages"`
	FailedMessages    int64                  `json:"failed_messages"`
	TopicStats        map[string]*TopicStats `json:"topic_stats"`
	LastUpdated       time.Time              `json:"last_updated"`
	mu                sync.RWMutex
}

// TopicStats represents per-topic statistics
type TopicStats struct {
	MessageCount   int64     `json:"message_count"`
	ConsumerCount  int       `json:"consumer_count"`
	LastMessage    time.Time `json:"last_message"`
	ProcessedCount int64     `json:"processed_count"`
	PendingCount   int64     `json:"pending_count"`
}

// NewMessageQueue creates a new message queue instance
func NewMessageQueue() *MessageQueue {
	mq := &MessageQueue{
		topics: make(map[string]*Topic),
		stats: &QueueStats{
			TopicStats: make(map[string]*TopicStats),
		},
	}

	// Try to initialize Redis Streams backend first (production-grade)
	logging.Infof("Attempting to initialize Redis Streams backend...")
	redisStreamsBackend, err := NewRedisStreamsBackend()
	if err != nil {
		logging.Infof("Redis Streams backend not available, trying legacy Redis backend: %v", err)

		// Fall back to legacy Redis backend
		redisBackend, err := NewRedisBackend()
		if err != nil {
			logging.Infof("Redis backend not available, using in-memory queue: %v", err)
		} else {
			mq.redisBackend = redisBackend
			logging.Infof("Successfully initialized legacy Redis-backed message queue")
		}
	} else {
		mq.redisStreamsBackend = redisStreamsBackend
		logging.Infof("Successfully initialized Redis Streams-backed message queue (production-grade)")
	}

	return mq
}

// CreateTopic creates a new topic
func (mq *MessageQueue) CreateTopic(name string, config map[string]string) error {
	// For Redis backend, topics are created implicitly when first message is published
	if mq.redisBackend != nil {
		logging.Debugf("Topic %s will be created implicitly in Redis on first publish", name)
		return nil
	}

	// In-memory implementation
	mq.mu.Lock()
	defer mq.mu.Unlock()

	if _, exists := mq.topics[name]; exists {
		return nil // Topic already exists
	}

	maxSize := 10000 // Default max size
	if sizeStr, ok := config["max_size"]; ok {
		// Parse maxSize from config if provided
		_ = sizeStr // For now, use default
	}

	topic := &Topic{
		Name:      name,
		Messages:  make([]*Message, 0),
		Consumers: make(map[string]*Consumer),
		Config:    config,
		CreatedAt: time.Now(),
		maxSize:   maxSize,
	}

	mq.topics[name] = topic
	mq.stats.TopicStats[name] = &TopicStats{
		LastMessage: time.Now(),
	}

	logging.Infof("Created topic: %s", name)
	return nil
}

// Publish publishes a message to a topic
func (mq *MessageQueue) Publish(ctx context.Context, topic string, payload []byte, headers map[string]string, ttlSeconds int) (*Message, error) {
	// Create message
	msg := &Message{
		ID:        generateMessageID(),
		Topic:     topic,
		Payload:   payload,
		Timestamp: time.Now(),
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(time.Duration(ttlSeconds) * time.Second),
		Headers:   headers,
		Processed: false,
		Retries:   0,
	}

	// Use Redis Streams backend if available (preferred for production)
	if mq.redisStreamsBackend != nil {
		err := mq.redisStreamsBackend.PublishMessage(topic, msg)
		if err != nil {
			logging.Errorf("Failed to publish to Redis Streams backend: %v", err)
			return nil, err
		}
		return msg, nil
	}

	// Fall back to legacy Redis backend
	if mq.redisBackend != nil {
		err := mq.redisBackend.PublishMessage(topic, msg)
		if err != nil {
			logging.Errorf("Failed to publish to Redis backend: %v", err)
			return nil, err
		}
		return msg, nil
	}

	// Fall back to in-memory implementation
	mq.mu.RLock()
	t, exists := mq.topics[topic]
	mq.mu.RUnlock()

	if !exists {
		return nil, ErrTopicNotFound
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	// Check if topic is full
	if len(t.Messages) >= t.maxSize {
		return nil, ErrQueueFull
	}

	// Add to topic
	t.Messages = append(t.Messages, msg)

	// Update stats
	mq.updateStats(topic, 1, 0, 0, 0)

	logging.Debugf("Published message %s to topic %s", msg.ID, topic)
	return msg, nil
}

// Consume consumes messages from a topic
func (mq *MessageQueue) Consume(ctx context.Context, topic, consumerGroup, consumerID string, maxMessages int, timeoutSeconds int) ([]*Message, error) {
	// Use Redis Streams backend if available (preferred for production)
	if mq.redisStreamsBackend != nil {
		messages, err := mq.redisStreamsBackend.ConsumeMessages(topic, consumerGroup, consumerID, maxMessages, timeoutSeconds)
		if err != nil {
			logging.Errorf("Failed to consume from Redis Streams backend: %v", err)
			return nil, err
		}
		return messages, nil
	}

	// Fall back to legacy Redis backend
	if mq.redisBackend != nil {
		// For Redis, we don't need to check if topic exists - it will return empty if no messages
		messages, err := mq.redisBackend.ConsumeMessages(topic, maxMessages, timeoutSeconds)
		if err != nil {
			logging.Errorf("Failed to consume from Redis backend: %v", err)
			return nil, err
		}
		return messages, nil
	}

	// Fall back to in-memory implementation
	mq.mu.RLock()
	t, exists := mq.topics[topic]
	mq.mu.RUnlock()

	if !exists {
		return nil, ErrTopicNotFound
	}

	// Register consumer
	consumer := mq.registerConsumer(t, consumerGroup, consumerID)

	t.mu.RLock()
	defer t.mu.RUnlock()

	var messages []*Message
	count := 0

	// Find unprocessed messages
	for _, msg := range t.Messages {
		if count >= maxMessages {
			break
		}

		// Skip expired messages
		if time.Now().After(msg.ExpiresAt) {
			continue
		}

		// Skip already processed messages by this consumer
		if mq.isMessageProcessedByConsumer(consumer, msg.ID) {
			continue
		}

		// Skip messages already being processed
		if msg.Processed {
			continue
		}

		messages = append(messages, msg)
		count++
	}

	logging.Debugf("Consumer %s consumed %d messages from topic %s", consumerID, len(messages), topic)
	return messages, nil
}

// Acknowledge acknowledges processed messages
func (mq *MessageQueue) Acknowledge(consumerGroup string, messages []*Message) ([]string, []string, error) {
	// Use Redis Streams backend if available (preferred for production)
	if mq.redisStreamsBackend != nil {
		return mq.redisStreamsBackend.AcknowledgeMessages(consumerGroup, messages)
	}

	// Fall back to legacy Redis backend (convert messages to IDs)
	if mq.redisBackend != nil {
		messageIDs := make([]string, len(messages))
		for i, msg := range messages {
			messageIDs[i] = msg.ID
		}
		return mq.redisBackend.AcknowledgeMessages("", messageIDs) // Legacy doesn't use consumer group
	}

	// Fall back to in-memory implementation
	var acked []string
	var failed []string

	mq.mu.RLock()
	defer mq.mu.RUnlock()

	for _, message := range messages {
		found := false
		for _, topic := range mq.topics {
			topic.mu.Lock()
			for _, msg := range topic.Messages {
				if msg.ID == message.ID {
					msg.Processed = true
					found = true

					acked = append(acked, message.ID)
					mq.updateStats(topic.Name, 0, 1, 0, 0)
					break
				}
			}
			topic.mu.Unlock()
			if found {
				break
			}
		}

		if !found {
			failed = append(failed, message.ID)
		}
	}

	logging.Debugf("Consumer group %s acknowledged %d messages, failed %d", consumerGroup, len(acked), len(failed))
	return acked, failed, nil
}

// ListTopics returns all topics
func (mq *MessageQueue) ListTopics() map[string]*Topic {
	mq.mu.RLock()
	defer mq.mu.RUnlock()

	topics := make(map[string]*Topic)
	for name, topic := range mq.topics {
		topics[name] = topic
	}
	return topics
}

// GetStats returns queue statistics
func (mq *MessageQueue) GetStats() *QueueStats {
	mq.stats.mu.RLock()
	defer mq.stats.mu.RUnlock()

	// Create a copy of stats
	stats := &QueueStats{
		TotalMessages:     mq.stats.TotalMessages,
		PendingMessages:   mq.stats.PendingMessages,
		ProcessedMessages: mq.stats.ProcessedMessages,
		FailedMessages:    mq.stats.FailedMessages,
		LastUpdated:       mq.stats.LastUpdated,
		TopicStats:        make(map[string]*TopicStats),
	}

	for name, topicStats := range mq.stats.TopicStats {
		stats.TopicStats[name] = &TopicStats{
			MessageCount:   topicStats.MessageCount,
			ConsumerCount:  topicStats.ConsumerCount,
			LastMessage:    topicStats.LastMessage,
			ProcessedCount: topicStats.ProcessedCount,
			PendingCount:   topicStats.PendingCount,
		}
	}

	return stats
}

// CleanupExpiredMessages removes expired messages from all topics
func (mq *MessageQueue) CleanupExpiredMessages() {
	mq.mu.RLock()
	topics := make([]*Topic, 0, len(mq.topics))
	for _, topic := range mq.topics {
		topics = append(topics, topic)
	}
	mq.mu.RUnlock()

	for _, topic := range topics {
		topic.mu.Lock()
		var validMessages []*Message
		expiredCount := 0

		for _, msg := range topic.Messages {
			if time.Now().Before(msg.ExpiresAt) {
				validMessages = append(validMessages, msg)
			} else {
				expiredCount++
			}
		}

		topic.Messages = validMessages
		topic.mu.Unlock()

		if expiredCount > 0 {
			logging.Infof("Cleaned up %d expired messages from topic %s", expiredCount, topic.Name)
			mq.updateStats(topic.Name, 0, 0, 0, int64(expiredCount))
		}
	}
}

// Helper functions

func (mq *MessageQueue) registerConsumer(topic *Topic, group, id string) *Consumer {
	topic.mu.Lock()
	defer topic.mu.Unlock()

	consumer, exists := topic.Consumers[id]
	if !exists {
		consumer = &Consumer{
			ID:           id,
			Group:        group,
			LastSeen:     time.Now(),
			ProcessedIDs: make([]string, 0),
		}
		topic.Consumers[id] = consumer

		// Update topic stats
		if stats, exists := mq.stats.TopicStats[topic.Name]; exists {
			stats.ConsumerCount = len(topic.Consumers)
		}
	} else {
		consumer.LastSeen = time.Now()
	}

	return consumer
}

func (mq *MessageQueue) isMessageProcessedByConsumer(consumer *Consumer, messageID string) bool {
	consumer.mu.RLock()
	defer consumer.mu.RUnlock()

	for _, id := range consumer.ProcessedIDs {
		if id == messageID {
			return true
		}
	}
	return false
}

func (mq *MessageQueue) updateStats(topic string, total, processed, failed, expired int64) {
	mq.stats.mu.Lock()
	defer mq.stats.mu.Unlock()

	mq.stats.TotalMessages += total
	mq.stats.ProcessedMessages += processed
	mq.stats.FailedMessages += failed
	mq.stats.PendingMessages = mq.stats.TotalMessages - mq.stats.ProcessedMessages - mq.stats.FailedMessages
	mq.stats.LastUpdated = time.Now()

	if topicStats, exists := mq.stats.TopicStats[topic]; exists {
		topicStats.MessageCount += total
		topicStats.ProcessedCount += processed
		topicStats.PendingCount = topicStats.MessageCount - topicStats.ProcessedCount
		if total > 0 {
			topicStats.LastMessage = time.Now()
		}
	}
}

func generateMessageID() string {
	// Simple message ID generation using timestamp and random component
	return time.Now().Format("20060102150405") + "-" + randomString(8)
}

// Close closes the in-memory message queue (no-op for in-memory implementation)
func (mq *MessageQueue) Close() error {
	// No resources to clean up for in-memory implementation
	return nil
}

func randomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJ0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[time.Now().UnixNano()%int64(len(charset))]
	}
	return string(b)
}
