package messagequeue

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/dilipmighty245/telemetry-pipeline/pkg/logging"
	clientv3 "go.etcd.io/etcd/client/v3"
)

// EtcdBackend provides etcd-based message queue functionality
type EtcdBackend struct {
	client     *clientv3.Client
	ctx        context.Context
	queuePrefix string
}

// NewEtcdBackend creates a new etcd backend if ETCD_ENDPOINTS is set
func NewEtcdBackend() (*EtcdBackend, error) {
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

	ctx := context.Background()

	// Test connection
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	_, err = client.Status(ctx, endpoints[0])
	cancel()
	if err != nil {
		client.Close()
		logging.Errorf("Failed to connect to etcd at %v: %v", endpoints, err)
		return nil, fmt.Errorf("failed to connect to etcd: %w", err)
	}

	logging.Infof("Connected to etcd message queue backend at %v (prefix: %s)", endpoints, queuePrefix)

	return &EtcdBackend{
		client:      client,
		ctx:         context.Background(),
		queuePrefix: queuePrefix,
	}, nil
}

// PublishMessage publishes a message to etcd
func (eb *EtcdBackend) PublishMessage(topic string, message *Message) error {
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
	ctx, cancel := context.WithTimeout(eb.ctx, 10*time.Second)
	defer cancel()

	// Calculate TTL based on message expiration
	ttl := int64(message.ExpiresAt.Sub(time.Now()).Seconds())
	if ttl <= 0 {
		ttl = 3600 // Default 1 hour TTL
	}

	// Create lease for TTL
	lease, err := eb.client.Grant(ctx, ttl)
	if err != nil {
		return fmt.Errorf("failed to create etcd lease: %w", err)
	}

	// Put message with lease
	_, err = eb.client.Put(ctx, messageKey, string(data), clientv3.WithLease(lease.ID))
	if err != nil {
		return fmt.Errorf("failed to publish message to etcd: %w", err)
	}

	logging.Debugf("Published message %s to etcd topic %s (key: %s)", message.ID, topic, messageKey)
	return nil
}

// ConsumeMessages consumes messages from etcd
func (eb *EtcdBackend) ConsumeMessages(topic string, maxMessages int, timeoutSeconds int) ([]*Message, error) {
	topicPrefix := fmt.Sprintf("%s/%s/", eb.queuePrefix, topic)
	var messages []*Message

	ctx, cancel := context.WithTimeout(eb.ctx, time.Duration(timeoutSeconds)*time.Second)
	defer cancel()

	// Get all messages for the topic, sorted by key (which includes timestamp)
	resp, err := eb.client.Get(ctx, topicPrefix, 
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
			eb.client.Delete(eb.ctx, string(kv.Key))
			continue
		}

		// Check if message has expired
		if time.Now().After(message.ExpiresAt) {
			// Delete expired message
			eb.client.Delete(eb.ctx, string(kv.Key))
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

// TopicExists checks if a topic exists (has messages)
func (eb *EtcdBackend) TopicExists(topic string) bool {
	topicPrefix := fmt.Sprintf("%s/%s/", eb.queuePrefix, topic)
	
	ctx, cancel := context.WithTimeout(eb.ctx, 5*time.Second)
	defer cancel()
	
	resp, err := eb.client.Get(ctx, topicPrefix, 
		clientv3.WithPrefix(), 
		clientv3.WithCountOnly())
	
	return err == nil && resp.Count > 0
}

// GetTopicStats returns statistics for a topic
func (eb *EtcdBackend) GetTopicStats(topic string) (int64, error) {
	topicPrefix := fmt.Sprintf("%s/%s/", eb.queuePrefix, topic)
	
	ctx, cancel := context.WithTimeout(eb.ctx, 5*time.Second)
	defer cancel()
	
	resp, err := eb.client.Get(ctx, topicPrefix, 
		clientv3.WithPrefix(), 
		clientv3.WithCountOnly())
	if err != nil {
		return 0, err
	}
	
	return resp.Count, nil
}

// AcknowledgeMessages acknowledges processed messages by deleting them from etcd
func (eb *EtcdBackend) AcknowledgeMessages(consumerID string, messageIDs []string) ([]string, []string, error) {
	var acked []string
	var failed []string

	// For etcd backend, we need to delete the messages from etcd
	// The message IDs should contain the etcd keys
	for _, messageID := range messageIDs {
		// Try to find and delete the message by searching for it
		// Since we don't have the exact key, we need to search by message ID
		if err := eb.deleteMessageByID(messageID); err != nil {
			logging.Errorf("Failed to acknowledge message %s: %v", messageID, err)
			failed = append(failed, messageID)
		} else {
			acked = append(acked, messageID)
		}
	}

	logging.Debugf("Acknowledged %d messages, failed %d for consumer %s", len(acked), len(failed), consumerID)
	return acked, failed, nil
}

// AcknowledgeMessagesByKeys acknowledges processed messages by their etcd keys
func (eb *EtcdBackend) AcknowledgeMessagesByKeys(consumerID string, messages []*Message) ([]string, []string, error) {
	var acked []string
	var failed []string

	ctx, cancel := context.WithTimeout(eb.ctx, 10*time.Second)
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

// deleteMessageByID finds and deletes a message by its ID
func (eb *EtcdBackend) deleteMessageByID(messageID string) error {
	ctx, cancel := context.WithTimeout(eb.ctx, 5*time.Second)
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

// ListTopics returns all topics that have messages
func (eb *EtcdBackend) ListTopics() ([]string, error) {
	ctx, cancel := context.WithTimeout(eb.ctx, 5*time.Second)
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

// Watch watches for changes in a topic
func (eb *EtcdBackend) Watch(topic string) clientv3.WatchChan {
	topicPrefix := fmt.Sprintf("%s/%s/", eb.queuePrefix, topic)
	return eb.client.Watch(eb.ctx, topicPrefix, clientv3.WithPrefix())
}

// Close closes the etcd connection
func (eb *EtcdBackend) Close() error {
	if eb.client != nil {
		return eb.client.Close()
	}
	return nil
}
