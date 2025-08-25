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

	// Backend specific fields
	StreamKey string `json:"stream_key,omitempty"` // etcd key
}

// HeadersJSON returns headers as JSON string for storage
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

// MessageQueue represents the message queue (in-memory or etcd-backed)
type MessageQueue struct {
	topics      map[string]*Topic
	mu          sync.RWMutex
	stats       *QueueStats
	etcdBackend *EtcdBackend // Primary etcd backend for distributed messaging
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
func NewMessageQueue() (*MessageQueue, error) {
	mq := &MessageQueue{
		topics: make(map[string]*Topic),
		stats: &QueueStats{
			TopicStats: make(map[string]*TopicStats),
		},
	}

	// Initialize etcd backend - required for distributed systems
	logging.Infof("Initializing etcd backend...")
	etcdBackend, err := NewEtcdBackend()
	if err != nil {
		logging.Errorf("Failed to initialize etcd backend: %v", err)
		logging.Errorf("etcd backend is required - application cannot start without it")
		return nil, fmt.Errorf("etcd backend initialization failed: %w", err)
	}

	mq.etcdBackend = etcdBackend
	logging.Infof("Successfully initialized etcd-backed message queue")
	return mq, nil
}

// CreateTopic creates a new topic
func (mq *MessageQueue) CreateTopic(name string, config map[string]string) error {
	// Topics are created implicitly in etcd when first message is published
	logging.Debugf("Topic %s will be created implicitly in etcd on first publish", name)
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

	// Publish to etcd backend
	err := mq.etcdBackend.PublishMessage(ctx, topic, msg)
	if err != nil {
		logging.Errorf("Failed to publish to etcd backend: %v", err)
		return nil, err
	}
	return msg, nil
}

// Consume consumes messages from a topic
func (mq *MessageQueue) Consume(ctx context.Context, topic, consumerGroup, consumerID string, maxMessages int, timeoutSeconds int) ([]*Message, error) {
	// Consume from etcd backend
	messages, err := mq.etcdBackend.ConsumeMessages(ctx, topic, maxMessages, timeoutSeconds)
	if err != nil {
		logging.Errorf("Failed to consume from etcd backend: %v", err)
		return nil, err
	}
	return messages, nil
}

// Acknowledge acknowledges processed messages
func (mq *MessageQueue) Acknowledge(ctx context.Context, consumerGroup string, messages []*Message) ([]string, []string, error) {
	// Acknowledge messages in etcd backend
	return mq.etcdBackend.AcknowledgeMessagesByKeys(ctx, consumerGroup, messages)
}

// ListTopics returns all topics from etcd
func (mq *MessageQueue) ListTopics(ctx context.Context) map[string]*Topic {
	topicNames, err := mq.etcdBackend.ListTopics(ctx)
	if err != nil {
		logging.Errorf("Failed to list topics from etcd: %v", err)
		return make(map[string]*Topic)
	}

	topics := make(map[string]*Topic)
	for _, name := range topicNames {
		// Create minimal topic info since etcd doesn't store topic metadata
		topics[name] = &Topic{
			Name:      name,
			CreatedAt: time.Now(), // We don't have actual creation time from etcd
		}
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

// CleanupExpiredMessages is handled automatically by etcd TTL
func (mq *MessageQueue) CleanupExpiredMessages() {
	// No-op: etcd handles message expiration automatically via TTL
	logging.Debugf("Message cleanup handled automatically by etcd TTL")
}

// Helper functions

func generateMessageID() string {
	// Simple message ID generation using timestamp and random component
	return time.Now().Format("20060102150405") + "-" + randomString(8)
}

// Close closes the message queue and cleans up resources
func (mq *MessageQueue) Close() error {
	if closeErr := mq.etcdBackend.Close(); closeErr != nil {
		logging.Errorf("Failed to close etcd backend: %v", closeErr)
		return closeErr
	}
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
