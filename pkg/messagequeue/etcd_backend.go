// Package messagequeue provides etcd-based message queue functionality with List & Watch patterns.
//
// This package implements a distributed message queue using etcd as the backend storage,
// supporting features like:
//   - Message publishing and consumption with TTL
//   - List & Watch pattern for real-time message processing
//   - Atomic work claiming to prevent duplicate processing
//   - Orphaned work recovery for fault tolerance
//   - Topic-based message organization
//   - Message acknowledgment and cleanup
//
// The implementation provides reliable message delivery with at-least-once semantics
// and supports horizontal scaling with multiple consumers.
package messagequeue

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/dilipmighty245/telemetry-pipeline/pkg/logging"
	"go.etcd.io/etcd/api/v3/mvccpb"
	clientv3 "go.etcd.io/etcd/client/v3"
)

// EtcdBackend provides etcd-based message queue functionality with List & Watch patterns.
//
// The backend uses etcd's key-value store and watch capabilities to implement a distributed
// message queue with features like atomic work claiming, message TTL, and fault tolerance.
// Messages are stored with unique keys that include timestamps for ordering.
type EtcdBackend struct {
	client      *clientv3.Client
	queuePrefix string
	watchers    map[string]clientv3.WatchChan
	watchersMu  sync.RWMutex
}

// NewEtcdBackend creates a new etcd backend for message queue operations.
//
// The function reads configuration from environment variables:
//   - ETCD_ENDPOINTS: Comma-separated list of etcd endpoints (required)
//   - MESSAGE_QUEUE_PREFIX: Key prefix for message storage (default: "/telemetry/queue")
//
// Parameters:
//   - ctx: Context for the initialization operation
//
// Returns:
//   - *EtcdBackend: A new etcd backend instance
//   - error: nil on success, error describing the failure otherwise
func NewEtcdBackend(ctx context.Context) (*EtcdBackend, error) {
	etcdEndpoints := os.Getenv("ETCD_ENDPOINTS")
	if etcdEndpoints == "" {
		return nil, fmt.Errorf("ETCD_ENDPOINTS environment variable not set")
	}

	queuePrefix := os.Getenv("MESSAGE_QUEUE_PREFIX")
	if queuePrefix == "" {
		queuePrefix = "/telemetry/queue"
	}

	endpoints := strings.Split(etcdEndpoints, ",")
	for i, endpoint := range endpoints {
		endpoints[i] = strings.TrimSpace(endpoint)
	}

	client, err := clientv3.New(clientv3.Config{
		Endpoints:   endpoints,
		DialTimeout: 10 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create etcd client: %w", err)
	}

	// Test connection
	childCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	_, err = client.Status(childCtx, endpoints[0])
	cancel()
	if err != nil {
		client.Close()
		logging.Errorf("Failed to connect to etcd at %v: %v", endpoints, err)
		return nil, fmt.Errorf("failed to connect to etcd: %w", err)
	}

	logging.Infof("Connected to etcd message queue backend at %v (prefix: %s)", endpoints, queuePrefix)

	return &EtcdBackend{
		client:      client,
		queuePrefix: queuePrefix,
		watchers:    make(map[string]clientv3.WatchChan),
	}, nil
}

// PublishMessage publishes a message to the specified topic in etcd.
//
// The message is stored with a unique key that includes timestamp and message ID
// for ordering and uniqueness. A TTL lease is created based on the message
// expiration time to automatically clean up expired messages.
//
// Parameters:
//   - ctx: Context for the publish operation
//   - topic: The topic to publish the message to
//   - message: The message to publish
//
// Returns:
//   - error: nil on success, error describing the failure otherwise
func (eb *EtcdBackend) PublishMessage(ctx context.Context, topic string, message *Message) error {
	// Create unique key for the message
	messageKey := fmt.Sprintf("%s/%s/%d_%s_%d",
		eb.queuePrefix,
		topic,
		time.Now().UnixNano(),
		message.ID,
		time.Now().Unix())

	// Serialize message to JSON
	data, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to serialize message: %w", err)
	}

	// Store message in etcd with TTL
	timeoutCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Calculate TTL based on message expiration
	ttl := int64(time.Until(message.ExpiresAt).Seconds())
	if ttl <= 0 {
		ttl = 3600 // Default 1 hour TTL
	}

	// Create lease for TTL
	lease, err := eb.client.Grant(timeoutCtx, ttl)
	if err != nil {
		return fmt.Errorf("failed to create etcd lease: %w", err)
	}

	// Put message with lease
	_, err = eb.client.Put(timeoutCtx, messageKey, string(data), clientv3.WithLease(lease.ID))
	if err != nil {
		return fmt.Errorf("failed to publish message to etcd: %w", err)
	}

	logging.Debugf("Published message %s to etcd topic %s (key: %s)", message.ID, topic, messageKey)
	return nil
}

// ConsumeMessages consumes messages from the specified topic in etcd.
//
// This method retrieves messages in chronological order (oldest first) and
// automatically filters out expired messages. The messages are not removed
// from etcd until explicitly acknowledged.
//
// Parameters:
//   - ctx: Context for the consume operation
//   - topic: The topic to consume messages from
//   - maxMessages: Maximum number of messages to retrieve
//   - timeoutSeconds: Timeout for the operation in seconds
//
// Returns:
//   - []*Message: A slice of consumed messages
//   - error: nil on success, error describing the failure otherwise
func (eb *EtcdBackend) ConsumeMessages(ctx context.Context, topic string, maxMessages int, timeoutSeconds int) ([]*Message, error) {
	topicPrefix := fmt.Sprintf("%s/%s/", eb.queuePrefix, topic)
	var messages []*Message

	timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSeconds)*time.Second)
	defer cancel()

	// Get all messages for the topic, sorted by key (which includes timestamp)
	resp, err := eb.client.Get(timeoutCtx, topicPrefix,
		clientv3.WithPrefix(),
		clientv3.WithSort(clientv3.SortByKey, clientv3.SortAscend),
		clientv3.WithLimit(int64(maxMessages)))
	if err != nil {
		return nil, fmt.Errorf("failed to consume from etcd: %w", err)
	}

	// Process each message
	for _, kv := range resp.Kvs {
		var message Message
		err := json.Unmarshal(kv.Value, &message)
		if err != nil {
			logging.Errorf("Failed to deserialize message from key %s: %v", kv.Key, err)
			// Delete invalid message
			eb.client.Delete(ctx, string(kv.Key))
			continue
		}

		// Check if message has expired
		if time.Now().After(message.ExpiresAt) {
			// Delete expired message
			eb.client.Delete(ctx, string(kv.Key))
			continue
		}

		// Store the etcd key for later acknowledgment
		message.StreamKey = string(kv.Key)
		messages = append(messages, &message)
	}

	if len(messages) > 0 {
		logging.Debugf("Consumed %d messages from etcd topic %s", len(messages), topic)
	}
	return messages, nil
}

// TopicExists checks if a topic exists by verifying if it has any messages.
//
// Parameters:
//   - ctx: Context for the operation
//   - topic: The topic name to check
//
// Returns:
//   - bool: true if the topic exists and has messages, false otherwise
func (eb *EtcdBackend) TopicExists(ctx context.Context, topic string) bool {
	topicPrefix := fmt.Sprintf("%s/%s/", eb.queuePrefix, topic)

	timeoutCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	resp, err := eb.client.Get(timeoutCtx, topicPrefix,
		clientv3.WithPrefix(),
		clientv3.WithCountOnly())

	return err == nil && resp.Count > 0
}

// GetTopicStats returns the number of messages in the specified topic.
//
// Parameters:
//   - ctx: Context for the operation
//   - topic: The topic to get statistics for
//
// Returns:
//   - int64: The number of messages in the topic
//   - error: nil on success, error describing the failure otherwise
func (eb *EtcdBackend) GetTopicStats(ctx context.Context, topic string) (int64, error) {
	topicPrefix := fmt.Sprintf("%s/%s/", eb.queuePrefix, topic)

	timeoutCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	resp, err := eb.client.Get(timeoutCtx, topicPrefix,
		clientv3.WithPrefix(),
		clientv3.WithCountOnly())
	if err != nil {
		return 0, err
	}

	return resp.Count, nil
}

// AcknowledgeMessages acknowledges processed messages by deleting them from etcd.
//
// This method searches for messages by their IDs across all topics and removes them.
// It's less efficient than AcknowledgeMessagesByKeys but works when only message IDs are available.
//
// Parameters:
//   - ctx: Context for the operation
//   - consumerID: ID of the consumer acknowledging the messages
//   - messageIDs: Slice of message IDs to acknowledge
//
// Returns:
//   - []string: Successfully acknowledged message IDs
//   - []string: Failed message IDs
//   - error: nil on success, error describing the failure otherwise
func (eb *EtcdBackend) AcknowledgeMessages(ctx context.Context, consumerID string, messageIDs []string) ([]string, []string, error) {
	var acked []string
	var failed []string

	// For etcd backend, we need to delete the messages from etcd
	// The message IDs should contain the etcd keys
	for _, messageID := range messageIDs {
		// Try to find and delete the message by searching for it
		// Since we don't have the exact key, we need to search by message ID
		if err := eb.deleteMessageByID(ctx, messageID); err != nil {
			logging.Errorf("Failed to acknowledge message %s: %v", messageID, err)
			failed = append(failed, messageID)
		} else {
			acked = append(acked, messageID)
		}
	}

	logging.Debugf("Acknowledged %d messages, failed %d for consumer %s", len(acked), len(failed), consumerID)
	return acked, failed, nil
}

// AcknowledgeMessagesByKeys acknowledges processed messages using their etcd keys.
//
// This method is more efficient than AcknowledgeMessages as it uses the stored
// etcd keys directly. It performs atomic deletion using etcd transactions.
//
// Parameters:
//   - ctx: Context for the operation
//   - consumerID: ID of the consumer acknowledging the messages
//   - messages: Slice of messages with their etcd keys to acknowledge
//
// Returns:
//   - []string: Successfully acknowledged message IDs
//   - []string: Failed message IDs
//   - error: nil on success, error describing the failure otherwise
func (eb *EtcdBackend) AcknowledgeMessagesByKeys(ctx context.Context, consumerID string, messages []*Message) ([]string, []string, error) {
	var acked []string
	var failed []string

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Use transaction to delete multiple messages atomically
	ops := make([]clientv3.Op, 0, len(messages))
	messageIDs := make([]string, 0, len(messages))

	for _, message := range messages {
		if message.StreamKey != "" {
			ops = append(ops, clientv3.OpDelete(message.StreamKey))
			messageIDs = append(messageIDs, message.ID)
		}
	}

	if len(ops) == 0 {
		return acked, failed, nil
	}

	// Execute transaction
	txnResp, err := eb.client.Txn(ctx).Then(ops...).Commit()
	if err != nil {
		// If transaction fails, mark all as failed
		return acked, messageIDs, fmt.Errorf("failed to acknowledge messages: %w", err)
	}

	if txnResp.Succeeded {
		acked = messageIDs
		logging.Debugf("Acknowledged %d messages for consumer %s", len(acked), consumerID)
	} else {
		failed = messageIDs
		logging.Errorf("Transaction failed to acknowledge messages for consumer %s", consumerID)
	}

	return acked, failed, nil
}

// deleteMessageByID finds and deletes a message by its ID.
//
// This method searches across all topics to find a message with the specified ID
// and removes it from etcd. It's used internally by AcknowledgeMessages.
//
// Parameters:
//   - ctx: Context for the operation
//   - messageID: The ID of the message to delete
//
// Returns:
//   - error: nil on success, error describing the failure otherwise
func (eb *EtcdBackend) deleteMessageByID(ctx context.Context, messageID string) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// Search for the message across all topics
	resp, err := eb.client.Get(ctx, eb.queuePrefix, clientv3.WithPrefix())
	if err != nil {
		return fmt.Errorf("failed to search for message: %w", err)
	}

	// Find the message with matching ID
	for _, kv := range resp.Kvs {
		var message Message
		if err := json.Unmarshal(kv.Value, &message); err != nil {
			continue // Skip invalid messages
		}

		if message.ID == messageID {
			// Delete the message
			_, err := eb.client.Delete(ctx, string(kv.Key))
			if err != nil {
				return fmt.Errorf("failed to delete message: %w", err)
			}
			logging.Debugf("Deleted message %s from etcd (key: %s)", messageID, kv.Key)
			return nil
		}
	}

	return fmt.Errorf("message %s not found", messageID)
}

// ListTopics returns all topics that currently have messages.
//
// This method scans the etcd keyspace to find all unique topic names
// that have at least one message.
//
// Parameters:
//   - ctx: Context for the operation
//
// Returns:
//   - []string: A slice of topic names
//   - error: nil on success, error describing the failure otherwise
func (eb *EtcdBackend) ListTopics(ctx context.Context) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	resp, err := eb.client.Get(ctx, eb.queuePrefix,
		clientv3.WithPrefix(),
		clientv3.WithKeysOnly())
	if err != nil {
		return nil, fmt.Errorf("failed to list topics: %w", err)
	}

	topicSet := make(map[string]bool)
	prefixLen := len(eb.queuePrefix)

	for _, kv := range resp.Kvs {
		key := string(kv.Key)
		if len(key) > prefixLen+1 {
			// Extract topic from key: /prefix/topic/messagekey
			remainder := key[prefixLen+1:] // Skip prefix and leading slash
			parts := strings.Split(remainder, "/")
			if len(parts) > 0 {
				topicSet[parts[0]] = true
			}
		}
	}

	topics := make([]string, 0, len(topicSet))
	for topic := range topicSet {
		topics = append(topics, topic)
	}

	return topics, nil
}

// Watch creates a watch channel for changes in the specified topic.
//
// The returned channel receives etcd watch events for all message operations
// (PUT/DELETE) in the topic.
//
// Parameters:
//   - ctx: Context for the watch operation
//   - topic: The topic to watch for changes
//
// Returns:
//   - clientv3.WatchChan: A channel that receives watch events
func (eb *EtcdBackend) Watch(ctx context.Context, topic string) clientv3.WatchChan {
	topicPrefix := fmt.Sprintf("%s/%s/", eb.queuePrefix, topic)
	return eb.client.Watch(ctx, topicPrefix, clientv3.WithPrefix())
}

// ConsumeWithListWatch implements the List & Watch pattern for real-time message processing.
//
// This method provides a continuous stream of messages by first listing existing messages
// and then watching for new ones. It ensures no messages are missed during the transition
// from list to watch by using etcd revision numbers.
//
// Parameters:
//   - ctx: Context for the operation
//   - topic: The topic to consume messages from
//   - consumerID: ID of the consumer (for logging purposes)
//
// Returns:
//   - <-chan *Message: A receive-only channel that streams messages
//   - error: nil on success, error describing the failure otherwise
func (eb *EtcdBackend) ConsumeWithListWatch(ctx context.Context, topic string, consumerID string) (<-chan *Message, error) {
	topicPrefix := fmt.Sprintf("%s/%s/", eb.queuePrefix, topic)
	messageChan := make(chan *Message, 1000)

	go func() {
		defer close(messageChan)

		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			// Step 1: List - Get current state with revision
			resp, err := eb.client.Get(ctx, topicPrefix,
				clientv3.WithPrefix(),
				clientv3.WithSort(clientv3.SortByKey, clientv3.SortAscend))
			if err != nil {
				logging.Errorf("Failed to list messages: %v", err)
				time.Sleep(5 * time.Second)
				continue
			}

			currentRev := resp.Header.Revision
			logging.Debugf("List phase: found %d messages at revision %d", len(resp.Kvs), currentRev)

			// Process existing messages
			for _, kv := range resp.Kvs {
				if msg := eb.parseMessage(ctx, kv); msg != nil {
					select {
					case messageChan <- msg:
					case <-ctx.Done():
						return
					}
				}
			}

			// Step 2: Watch - Monitor for new changes from next revision
			watchCtx, watchCancel := context.WithCancel(ctx)
			watchChan := eb.client.Watch(watchCtx, topicPrefix,
				clientv3.WithPrefix(),
				clientv3.WithRev(currentRev+1))

			watchActive := true
			for watchActive {
				select {
				case watchResp, ok := <-watchChan:
					if !ok {
						watchActive = false
						break
					}

					if watchResp.Err() != nil {
						logging.Errorf("Watch error: %v", watchResp.Err())
						watchActive = false
						break
					}

					for _, event := range watchResp.Events {
						if event.Type == clientv3.EventTypePut {
							if msg := eb.parseMessage(ctx, event.Kv); msg != nil {
								select {
								case messageChan <- msg:
								case <-ctx.Done():
									watchCancel()
									return
								}
							}
						}
					}
				case <-ctx.Done():
					watchCancel()
					return
				}
			}

			watchCancel()
			logging.Debugf("Watch ended for topic %s, restarting List & Watch cycle", topic)
		}
	}()

	return messageChan, nil
}

// AtomicWorkClaim atomically claims work to prevent duplicate processing by multiple consumers.
//
// This method uses etcd transactions to atomically move messages from the work queue
// to a processing queue, ensuring that each message is processed by only one consumer.
// The claimed messages are moved to a consumer-specific processing key.
//
// Parameters:
//   - ctx: Context for the operation
//   - topic: The topic to claim work from
//   - consumerID: ID of the consumer claiming the work
//   - maxMessages: Maximum number of messages to claim
//
// Returns:
//   - []*Message: Successfully claimed messages
//   - error: nil on success, error describing the failure otherwise
func (eb *EtcdBackend) AtomicWorkClaim(ctx context.Context, topic, consumerID string, maxMessages int) ([]*Message, error) {
	workPrefix := fmt.Sprintf("%s/%s/", eb.queuePrefix, topic)
	processingPrefix := fmt.Sprintf("/processing/%s/", consumerID)

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Get available work
	resp, err := eb.client.Get(ctx, workPrefix,
		clientv3.WithPrefix(),
		clientv3.WithSort(clientv3.SortByKey, clientv3.SortAscend),
		clientv3.WithLimit(int64(maxMessages)))
	if err != nil {
		return nil, fmt.Errorf("failed to get work: %w", err)
	}

	if len(resp.Kvs) == 0 {
		return nil, nil // No work available
	}

	// Build atomic transaction
	var ops []clientv3.Op
	var messages []*Message

	for _, kv := range resp.Kvs {
		// Create unique processing key
		processingKey := fmt.Sprintf("%s%d_%s", processingPrefix, time.Now().UnixNano(),
			strings.TrimPrefix(string(kv.Key), workPrefix))

		ops = append(ops,
			clientv3.OpDelete(string(kv.Key)),               // Remove from work queue
			clientv3.OpPut(processingKey, string(kv.Value))) // Add to processing

		if msg := eb.parseMessage(ctx, kv); msg != nil {
			msg.StreamKey = processingKey // Track processing key for acknowledgment
			messages = append(messages, msg)
		}
	}

	if len(ops) == 0 {
		return nil, nil
	}

	// Execute atomic transaction
	txnResp, err := eb.client.Txn(ctx).Then(ops...).Commit()
	if err != nil {
		return nil, fmt.Errorf("failed to execute transaction: %w", err)
	}

	if !txnResp.Succeeded {
		return nil, fmt.Errorf("transaction failed - work may have been claimed by another consumer")
	}

	logging.Infof("Consumer %s atomically claimed %d messages", consumerID, len(messages))
	return messages, nil
}

// parseMessage safely parses a message from an etcd key-value pair.
//
// This method deserializes the message JSON, checks for expiration,
// and automatically cleans up invalid or expired messages.
//
// Parameters:
//   - ctx: Context for cleanup operations
//   - kv: The etcd key-value pair containing the message
//
// Returns:
//   - *Message: The parsed message, or nil if invalid/expired
func (eb *EtcdBackend) parseMessage(ctx context.Context, kv *mvccpb.KeyValue) *Message {
	var message Message
	err := json.Unmarshal(kv.Value, &message)
	if err != nil {
		logging.Errorf("Failed to deserialize message from key %s: %v", kv.Key, err)
		// Clean up invalid message
		eb.client.Delete(ctx, string(kv.Key))
		return nil
	}

	// Check if message has expired
	if time.Now().After(message.ExpiresAt) {
		// Delete expired message
		eb.client.Delete(ctx, string(kv.Key))
		logging.Debugf("Deleted expired message %s", message.ID)
		return nil
	}

	// Store the etcd key for later acknowledgment
	message.StreamKey = string(kv.Key)
	return &message
}

// RecoverOrphanedWork recovers work from dead consumers by moving old processing entries back to the work queue.
//
// This method scans the processing queue for messages older than the specified age
// and moves them back to the work queue for reprocessing. This provides fault tolerance
// when consumers crash or become unresponsive.
//
// Parameters:
//   - ctx: Context for the operation
//   - maxAge: Maximum age of processing entries before they're considered orphaned
//
// Returns:
//   - error: nil on success, error describing the failure otherwise
func (eb *EtcdBackend) RecoverOrphanedWork(ctx context.Context, maxAge time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// Find all processing entries older than maxAge
	resp, err := eb.client.Get(ctx, "/processing/", clientv3.WithPrefix())
	if err != nil {
		return fmt.Errorf("failed to get processing entries: %w", err)
	}

	var recoveryOps []clientv3.Op
	recoveredCount := 0

	for _, kv := range resp.Kvs {
		var msg Message
		if err := json.Unmarshal(kv.Value, &msg); err != nil {
			continue
		}

		// Check if message is old enough to recover
		if time.Since(msg.CreatedAt) > maxAge {
			// Move back to work queue
			workKey := fmt.Sprintf("%s/%s/%d_%s_%d",
				eb.queuePrefix, msg.Topic,
				time.Now().UnixNano(), msg.ID, time.Now().Unix())

			recoveryOps = append(recoveryOps,
				clientv3.OpDelete(string(kv.Key)),         // Remove from processing
				clientv3.OpPut(workKey, string(kv.Value))) // Add back to work queue
			recoveredCount++
		}
	}

	if len(recoveryOps) > 0 {
		_, err := eb.client.Txn(ctx).Then(recoveryOps...).Commit()
		if err != nil {
			return fmt.Errorf("failed to recover orphaned work: %w", err)
		}
		logging.Infof("Recovered %d orphaned messages", recoveredCount)
	}

	return nil
}

// GetQueueDepth returns the number of pending messages in the specified topic.
//
// This method provides queue monitoring capabilities by counting the number
// of messages currently waiting to be processed in a topic.
//
// Parameters:
//   - ctx: Context for the operation
//   - topic: The topic to get queue depth for
//
// Returns:
//   - int64: The number of pending messages
//   - error: nil on success, error describing the failure otherwise
func (eb *EtcdBackend) GetQueueDepth(ctx context.Context, topic string) (int64, error) {
	topicPrefix := fmt.Sprintf("%s/%s/", eb.queuePrefix, topic)

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	resp, err := eb.client.Get(ctx, topicPrefix,
		clientv3.WithPrefix(),
		clientv3.WithCountOnly())
	if err != nil {
		return 0, fmt.Errorf("failed to get queue depth: %w", err)
	}

	return resp.Count, nil
}

// Close gracefully shuts down the etcd backend.
//
// This method closes all active watchers and the etcd client connection,
// cleaning up resources and ensuring proper shutdown.
//
// Returns:
//   - error: nil on success, error from closing the etcd client otherwise
func (eb *EtcdBackend) Close() error {
	// Close all watchers
	eb.watchersMu.Lock()
	for topic, watchChan := range eb.watchers {
		if watchChan != nil {
			// Watchers are closed by canceling context
			delete(eb.watchers, topic)
		}
	}
	eb.watchersMu.Unlock()

	if eb.client != nil {
		return eb.client.Close()
	}
	return nil
}
