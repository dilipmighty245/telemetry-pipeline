package messagequeue

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/dilipmighty245/telemetry-pipeline/pkg/logging"
	"github.com/redis/go-redis/v9"
)

// RedisStreamsBackend provides Redis Streams-based message queue functionality
// This provides guaranteed message delivery with proper acknowledgment
type RedisStreamsBackend struct {
	client *redis.Client
	ctx    context.Context
	cancel context.CancelFunc
}

// StreamMessage represents a message in Redis Streams
type StreamMessage struct {
	ID      string            `json:"id"`
	Stream  string            `json:"stream"`
	Fields  map[string]string `json:"fields"`
	Message *Message          `json:"message"`
}

// NewRedisStreamsBackend creates a new Redis Streams backend
func NewRedisStreamsBackend() (*RedisStreamsBackend, error) {
	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		return nil, fmt.Errorf("REDIS_URL environment variable not set")
	}

	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Redis URL: %w", err)
	}

	client := redis.NewClient(opts)
	ctx, cancel := context.WithCancel(context.Background())

	// Test connection
	pong, err := client.Ping(ctx).Result()
	if err != nil {
		logging.Errorf("Failed to ping Redis at %s: %v", opts.Addr, err)
		cancel()
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	logging.Infof("Connected to Redis Streams message queue backend at %s (ping: %s)", opts.Addr, pong)

	return &RedisStreamsBackend{
		client: client,
		ctx:    ctx,
		cancel: cancel,
	}, nil
}

// PublishMessage publishes a message to Redis Streams
func (rsb *RedisStreamsBackend) PublishMessage(topic string, message *Message) error {
	streamKey := fmt.Sprintf("stream:%s", topic)

	// Serialize the message to JSON for the payload field
	messageData, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to serialize message: %w", err)
	}

	// Create stream fields
	fields := map[string]interface{}{
		"id":        message.ID,
		"payload":   string(messageData),
		"timestamp": message.Timestamp.Unix(),
		"headers":   message.HeadersJSON(),
	}

	// Add individual headers as fields for easier querying
	for k, v := range message.Headers {
		fields[fmt.Sprintf("header_%s", k)] = v
	}

	// Publish to Redis Stream with auto-generated ID
	result := rsb.client.XAdd(rsb.ctx, &redis.XAddArgs{
		Stream: streamKey,
		Values: fields,
	})

	if result.Err() != nil {
		return fmt.Errorf("failed to publish message to Redis Stream: %w", result.Err())
	}

	streamID := result.Val()
	logging.Debugf("Published message %s to Redis Stream %s with ID %s", message.ID, topic, streamID)
	return nil
}

// ConsumeMessages consumes messages from Redis Streams using consumer groups
func (rsb *RedisStreamsBackend) ConsumeMessages(topic, consumerGroup, consumerID string, maxMessages int, timeoutSeconds int) ([]*Message, error) {
	streamKey := fmt.Sprintf("stream:%s", topic)

	// Ensure consumer group exists
	err := rsb.ensureConsumerGroup(streamKey, consumerGroup)
	if err != nil {
		return nil, fmt.Errorf("failed to ensure consumer group: %w", err)
	}

	// First, try to read pending messages for this consumer
	pendingMessages, err := rsb.readPendingMessages(streamKey, consumerGroup, consumerID, maxMessages)
	if err != nil {
		logging.Warnf("Failed to read pending messages: %v", err)
	}

	remainingMessages := maxMessages - len(pendingMessages)
	var newMessages []*Message

	// If we need more messages, read new ones from the stream
	if remainingMessages > 0 {
		newMessages, err = rsb.readNewMessages(streamKey, consumerGroup, consumerID, remainingMessages, timeoutSeconds)
		if err != nil {
			return pendingMessages, fmt.Errorf("failed to read new messages: %w", err)
		}
	}

	// Combine pending and new messages
	allMessages := append(pendingMessages, newMessages...)

	if len(allMessages) > 0 {
		logging.Debugf("Consumer %s consumed %d messages (%d pending, %d new) from Redis Stream %s",
			consumerID, len(allMessages), len(pendingMessages), len(newMessages), topic)
	}

	return allMessages, nil
}

// readPendingMessages reads messages that were delivered but not acknowledged
func (rsb *RedisStreamsBackend) readPendingMessages(streamKey, consumerGroup, consumerID string, maxMessages int) ([]*Message, error) {
	// Get pending messages for this specific consumer
	pendingResult := rsb.client.XPendingExt(rsb.ctx, &redis.XPendingExtArgs{
		Stream:   streamKey,
		Group:    consumerGroup,
		Start:    "-",
		End:      "+",
		Count:    int64(maxMessages),
		Consumer: consumerID,
	})

	if pendingResult.Err() != nil {
		if strings.Contains(pendingResult.Err().Error(), "NOGROUP") {
			return nil, nil // No consumer group yet
		}
		return nil, pendingResult.Err()
	}

	if len(pendingResult.Val()) == 0 {
		return nil, nil
	}

	// Extract message IDs from pending messages
	var messageIDs []string
	for _, pending := range pendingResult.Val() {
		messageIDs = append(messageIDs, pending.ID)
	}

	// Claim these pending messages
	claimResult := rsb.client.XClaim(rsb.ctx, &redis.XClaimArgs{
		Stream:   streamKey,
		Group:    consumerGroup,
		Consumer: consumerID,
		MinIdle:  time.Minute, // Claim messages idle for more than 1 minute
		Messages: messageIDs,
	})

	if claimResult.Err() != nil {
		return nil, claimResult.Err()
	}

	return rsb.parseStreamMessages(claimResult.Val(), streamKey)
}

// readNewMessages reads new messages from the stream
func (rsb *RedisStreamsBackend) readNewMessages(streamKey, consumerGroup, consumerID string, maxMessages, timeoutSeconds int) ([]*Message, error) {
	// Read new messages from the stream
	result := rsb.client.XReadGroup(rsb.ctx, &redis.XReadGroupArgs{
		Group:    consumerGroup,
		Consumer: consumerID,
		Streams:  []string{streamKey, ">"},
		Count:    int64(maxMessages),
		Block:    time.Duration(timeoutSeconds) * time.Second,
	})

	if result.Err() != nil {
		if result.Err() == redis.Nil {
			// Timeout or no messages
			return nil, nil
		}
		return nil, result.Err()
	}

	if len(result.Val()) == 0 || len(result.Val()[0].Messages) == 0 {
		return nil, nil
	}

	return rsb.parseStreamMessages(result.Val()[0].Messages, streamKey)
}

// parseStreamMessages converts Redis stream messages to our Message format
func (rsb *RedisStreamsBackend) parseStreamMessages(streamMessages []redis.XMessage, streamKey string) ([]*Message, error) {
	var messages []*Message

	for _, streamMsg := range streamMessages {
		// Extract the serialized message from payload field
		payloadStr, ok := streamMsg.Values["payload"].(string)
		if !ok {
			logging.Warnf("Message %s missing payload field", streamMsg.ID)
			continue
		}

		var message Message
		err := json.Unmarshal([]byte(payloadStr), &message)
		if err != nil {
			logging.Errorf("Failed to deserialize message %s: %v", streamMsg.ID, err)
			continue
		}

		// Store the Redis Stream ID for acknowledgment
		message.StreamID = streamMsg.ID
		message.StreamKey = streamKey

		messages = append(messages, &message)
	}

	return messages, nil
}

// AcknowledgeMessages acknowledges processed messages in Redis Streams
func (rsb *RedisStreamsBackend) AcknowledgeMessages(consumerGroup string, messages []*Message) ([]string, []string, error) {
	var acked []string
	var failed []string

	// Group messages by stream for batch acknowledgment
	streamGroups := make(map[string][]string)
	for _, msg := range messages {
		if msg.StreamID == "" || msg.StreamKey == "" {
			failed = append(failed, msg.ID)
			logging.Warnf("Message %s missing stream information for acknowledgment", msg.ID)
			continue
		}
		streamGroups[msg.StreamKey] = append(streamGroups[msg.StreamKey], msg.StreamID)
	}

	// Acknowledge messages by stream
	for streamKey, streamIDs := range streamGroups {
		result := rsb.client.XAck(rsb.ctx, streamKey, consumerGroup, streamIDs...)
		if result.Err() != nil {
			logging.Errorf("Failed to acknowledge messages in stream %s: %v", streamKey, result.Err())
			// Add all these messages to failed list
			for _, msg := range messages {
				if msg.StreamKey == streamKey {
					failed = append(failed, msg.ID)
				}
			}
			continue
		}

		// Count successful acknowledgments
		ackedCount := result.Val()
		logging.Debugf("Successfully acknowledged %d/%d messages in stream %s", ackedCount, len(streamIDs), streamKey)

		// Add successfully acknowledged messages to acked list
		for i, msg := range messages {
			if msg.StreamKey == streamKey && i < int(ackedCount) {
				acked = append(acked, msg.ID)
			}
		}
	}

	if len(failed) > 0 {
		logging.Warnf("Failed to acknowledge %d messages", len(failed))
	}

	logging.Debugf("Acknowledged %d messages, %d failed", len(acked), len(failed))
	return acked, failed, nil
}

// ensureConsumerGroup creates a consumer group if it doesn't exist
func (rsb *RedisStreamsBackend) ensureConsumerGroup(streamKey, consumerGroup string) error {
	// Try to create the consumer group starting from the beginning of the stream
	result := rsb.client.XGroupCreateMkStream(rsb.ctx, streamKey, consumerGroup, "0")
	if result.Err() != nil {
		// If group already exists, that's fine
		if strings.Contains(result.Err().Error(), "BUSYGROUP") {
			return nil
		}
		return result.Err()
	}

	logging.Debugf("Created consumer group %s for stream %s", consumerGroup, streamKey)
	return nil
}

// TopicExists checks if a stream exists
func (rsb *RedisStreamsBackend) TopicExists(topic string) bool {
	streamKey := fmt.Sprintf("stream:%s", topic)
	result := rsb.client.XLen(rsb.ctx, streamKey)
	return result.Err() == nil
}

// GetTopicStats returns statistics for a stream
func (rsb *RedisStreamsBackend) GetTopicStats(topic string) (int64, error) {
	streamKey := fmt.Sprintf("stream:%s", topic)
	result := rsb.client.XLen(rsb.ctx, streamKey)
	if result.Err() != nil {
		return 0, result.Err()
	}
	return result.Val(), nil
}

// GetStreamInfo returns detailed information about a stream
func (rsb *RedisStreamsBackend) GetStreamInfo(topic string) (*StreamInfo, error) {
	streamKey := fmt.Sprintf("stream:%s", topic)

	// Get stream info
	infoResult := rsb.client.XInfoStream(rsb.ctx, streamKey)
	if infoResult.Err() != nil {
		return nil, infoResult.Err()
	}

	info := infoResult.Val()

	// Get consumer groups info
	groupsResult := rsb.client.XInfoGroups(rsb.ctx, streamKey)
	var groups []ConsumerGroupInfo
	if groupsResult.Err() == nil {
		for _, group := range groupsResult.Val() {
			groups = append(groups, ConsumerGroupInfo{
				Name:    group.Name,
				Pending: group.Pending,
				LastID:  group.LastDeliveredID,
			})
		}
	}

	return &StreamInfo{
		Name:         topic,
		Length:       info.Length,
		Groups:       groups,
		FirstEntryID: info.FirstEntry.ID,
		LastEntryID:  info.LastEntry.ID,
	}, nil
}

// StreamInfo contains information about a Redis Stream
type StreamInfo struct {
	Name         string              `json:"name"`
	Length       int64               `json:"length"`
	Groups       []ConsumerGroupInfo `json:"groups"`
	FirstEntryID string              `json:"first_entry_id"`
	LastEntryID  string              `json:"last_entry_id"`
}

// ConsumerGroupInfo contains information about a consumer group
type ConsumerGroupInfo struct {
	Name    string `json:"name"`
	Pending int64  `json:"pending"`
	LastID  string `json:"last_id"`
}

// Close closes the Redis connection
func (rsb *RedisStreamsBackend) Close() error {
	rsb.cancel()
	return rsb.client.Close()
}

// CleanupOldMessages removes messages older than the specified duration
func (rsb *RedisStreamsBackend) CleanupOldMessages(topic string, maxAge time.Duration) error {
	streamKey := fmt.Sprintf("stream:%s", topic)

	// Calculate the minimum timestamp to keep
	minTimestamp := time.Now().Add(-maxAge).UnixMilli()
	minID := fmt.Sprintf("%d-0", minTimestamp)

	// Trim the stream to remove old messages
	result := rsb.client.XTrimMinID(rsb.ctx, streamKey, minID)
	if result.Err() != nil {
		return fmt.Errorf("failed to cleanup old messages: %w", result.Err())
	}

	deleted := result.Val()
	if deleted > 0 {
		logging.Infof("Cleaned up %d old messages from stream %s", deleted, topic)
	}

	return nil
}
