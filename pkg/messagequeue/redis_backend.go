package messagequeue

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/dilipmighty245/telemetry-pipeline/pkg/logging"
	"github.com/redis/go-redis/v9"
)

// RedisBackend provides Redis-based message queue functionality
type RedisBackend struct {
	client *redis.Client
	ctx    context.Context
}

// NewRedisBackend creates a new Redis backend if REDIS_URL is set
func NewRedisBackend() (*RedisBackend, error) {
	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		return nil, fmt.Errorf("REDIS_URL environment variable not set")
	}

	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Redis URL: %w", err)
	}

	client := redis.NewClient(opts)
	ctx := context.Background()

	// Test connection
	pong, err := client.Ping(ctx).Result()
	if err != nil {
		logging.Errorf("Failed to ping Redis at %s: %v", opts.Addr, err)
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	logging.Infof("Connected to Redis message queue backend at %s (ping: %s)", opts.Addr, pong)

	return &RedisBackend{
		client: client,
		ctx:    ctx,
	}, nil
}

// PublishMessage publishes a message to Redis using lists
func (rb *RedisBackend) PublishMessage(topic string, message *Message) error {
	// Serialize message to JSON
	data, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to serialize message: %w", err)
	}

	// Use Redis list as a queue (LPUSH for publishing, BRPOP for consuming)
	listKey := fmt.Sprintf("queue:%s", topic)
	err = rb.client.LPush(rb.ctx, listKey, string(data)).Err()
	if err != nil {
		return fmt.Errorf("failed to publish message to Redis: %w", err)
	}

	// Set expiration on the list to prevent it from growing indefinitely
	rb.client.Expire(rb.ctx, listKey, 24*time.Hour)

	logging.Debugf("Published message %s to Redis topic %s", message.ID, topic)
	return nil
}

// ConsumeMessages consumes messages from Redis using blocking pop
func (rb *RedisBackend) ConsumeMessages(topic string, maxMessages int, timeoutSeconds int) ([]*Message, error) {
	listKey := fmt.Sprintf("queue:%s", topic)
	var messages []*Message

	// Use BRPOP for blocking consumption with timeout
	for i := 0; i < maxMessages; i++ {
		result := rb.client.BRPop(rb.ctx, time.Duration(timeoutSeconds)*time.Second, listKey)
		if result.Err() != nil {
			if result.Err() == redis.Nil {
				// Timeout or no messages available
				break
			}
			return nil, fmt.Errorf("failed to consume from Redis: %w", result.Err())
		}

		// result.Val()[1] contains the message data (result.Val()[0] is the key)
		if len(result.Val()) < 2 {
			continue
		}

		messageData := result.Val()[1]
		var message Message
		err := json.Unmarshal([]byte(messageData), &message)
		if err != nil {
			logging.Errorf("Failed to deserialize message: %v", err)
			continue
		}

		messages = append(messages, &message)
	}

	if len(messages) > 0 {
		logging.Debugf("Consumed %d messages from Redis topic %s", len(messages), topic)
	}
	return messages, nil
}

// TopicExists checks if a topic exists (has messages)
func (rb *RedisBackend) TopicExists(topic string) bool {
	listKey := fmt.Sprintf("queue:%s", topic)
	length := rb.client.LLen(rb.ctx, listKey)
	return length.Err() == nil && length.Val() >= 0
}

// GetTopicStats returns statistics for a topic
func (rb *RedisBackend) GetTopicStats(topic string) (int64, error) {
	listKey := fmt.Sprintf("queue:%s", topic)
	length := rb.client.LLen(rb.ctx, listKey)
	if length.Err() != nil {
		return 0, length.Err()
	}
	return length.Val(), nil
}

// Close closes the Redis connection
func (rb *RedisBackend) Close() error {
	return rb.client.Close()
}
