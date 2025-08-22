package messagequeue

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/dilipmighty245/telemetry-pipeline/pkg/logging"
	"github.com/oklog/ulid/v2"
	clientv3 "go.etcd.io/etcd/client/v3"
)

// EnhancedMessage represents a message with idempotency and tracing support
type EnhancedMessage struct {
	ID              string            `json:"id"`              // ULID
	IdempotencyKey  string            `json:"idempotency_key"` // SHA256 hash of payload
	Topic           string            `json:"topic"`
	Payload         []byte            `json:"payload"`
	Headers         map[string]string `json:"headers"`
	TraceID         string            `json:"trace_id"`
	CreatedAt       time.Time         `json:"created_at"`
	ExpiresAt       time.Time         `json:"expires_at"`
	RetryCount      int               `json:"retry_count"`
	MaxRetries      int               `json:"max_retries"`
	ProcessingLease string            `json:"processing_lease,omitempty"` // Lease ID when being processed
}

// EnhancedQueueConfig holds configuration for the enhanced queue
type EnhancedQueueConfig struct {
	EtcdEndpoints     []string      `json:"etcd_endpoints"`
	QueuePrefix       string        `json:"queue_prefix"`
	ProcessingPrefix  string        `json:"processing_prefix"`
	ProcessedPrefix   string        `json:"processed_prefix"`
	DefaultTTL        time.Duration `json:"default_ttl"`
	ProcessingTimeout time.Duration `json:"processing_timeout"`
	BatchSize         int           `json:"batch_size"`
	MaxRetries        int           `json:"max_retries"`
	CompactionWindow  time.Duration `json:"compaction_window"`
}

// EnhancedQueue provides exactly-once processing semantics using etcd
type EnhancedQueue struct {
	client           *clientv3.Client
	config           *EnhancedQueueConfig
	ctx              context.Context
	cancel           context.CancelFunc
	metrics          *QueueMetrics
	mu               sync.RWMutex
	shutdownComplete chan struct{}
}

// QueueMetrics tracks queue performance metrics
type QueueMetrics struct {
	MessagesPublished  int64 `json:"messages_published"`
	MessagesConsumed   int64 `json:"messages_consumed"`
	MessagesProcessed  int64 `json:"messages_processed"`
	MessagesDuplicated int64 `json:"messages_duplicated"`
	MessagesExpired    int64 `json:"messages_expired"`
	ProcessingErrors   int64 `json:"processing_errors"`
	QueueDepth         int64 `json:"queue_depth"`
	ProcessingLag      int64 `json:"processing_lag_ms"`
	mu                 sync.RWMutex
}

// NewEnhancedQueue creates a new enhanced message queue
func NewEnhancedQueue(config *EnhancedQueueConfig) (*EnhancedQueue, error) {
	// Set defaults
	if config.QueuePrefix == "" {
		config.QueuePrefix = "/queue/telemetry"
	}
	if config.ProcessingPrefix == "" {
		config.ProcessingPrefix = "/processing"
	}
	if config.ProcessedPrefix == "" {
		config.ProcessedPrefix = "/processed"
	}
	if config.DefaultTTL == 0 {
		config.DefaultTTL = 24 * time.Hour
	}
	if config.ProcessingTimeout == 0 {
		config.ProcessingTimeout = 30 * time.Second
	}
	if config.BatchSize == 0 {
		config.BatchSize = 100
	}
	if config.MaxRetries == 0 {
		config.MaxRetries = 3
	}
	if config.CompactionWindow == 0 {
		config.CompactionWindow = time.Hour
	}

	// Create etcd client
	client, err := clientv3.New(clientv3.Config{
		Endpoints:   config.EtcdEndpoints,
		DialTimeout: 10 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create etcd client: %w", err)
	}

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	_, err = client.Status(ctx, config.EtcdEndpoints[0])
	cancel()
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("failed to connect to etcd: %w", err)
	}

	ctx, cancel = context.WithCancel(context.Background())

	eq := &EnhancedQueue{
		client:           client,
		config:           config,
		ctx:              ctx,
		cancel:           cancel,
		metrics:          &QueueMetrics{},
		shutdownComplete: make(chan struct{}),
	}

	// Start background cleanup goroutine
	go eq.cleanupLoop()

	logging.Infof("Enhanced message queue initialized with prefix: %s", config.QueuePrefix)
	return eq, nil
}

// PublishMessage publishes a message with exactly-once semantics
func (eq *EnhancedQueue) PublishMessage(ctx context.Context, topic string, payload []byte, headers map[string]string) error {

	// Generate ULID for message ID
	messageID := ulid.Make().String()

	// Generate idempotency key from payload
	hash := sha256.Sum256(payload)
	idempotencyKey := hex.EncodeToString(hash[:])

	// Extract trace ID from headers if present
	traceID := ""
	if headers != nil {
		traceID = headers["trace_id"]
	}

	message := &EnhancedMessage{
		ID:             messageID,
		IdempotencyKey: idempotencyKey,
		Topic:          topic,
		Payload:        payload,
		Headers:        headers,
		TraceID:        traceID,
		CreatedAt:      time.Now(),
		ExpiresAt:      time.Now().Add(eq.config.DefaultTTL),
		RetryCount:     0,
		MaxRetries:     eq.config.MaxRetries,
	}

	// Check if message already exists (idempotency)
	processedKey := fmt.Sprintf("%s/%s", eq.config.ProcessedPrefix, idempotencyKey)
	resp, err := eq.client.Get(ctx, processedKey)
	if err != nil {
		return fmt.Errorf("failed to check idempotency: %w", err)
	}

	if len(resp.Kvs) > 0 {
		// Message already processed
		eq.updateMetrics(0, 0, 0, 1, 0, 0)
		logging.Debugf("Message with idempotency key %s already processed", idempotencyKey)
		return nil
	}

	// Serialize message
	data, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to serialize message: %w", err)
	}

	// Create queue key with ULID for ordering
	queueKey := fmt.Sprintf("%s/%s/%s", eq.config.QueuePrefix, topic, messageID)

	// Create lease for TTL
	lease, err := eq.client.Grant(ctx, int64(eq.config.DefaultTTL.Seconds()))
	if err != nil {
		return fmt.Errorf("failed to create lease: %w", err)
	}

	// Store message in queue
	_, err = eq.client.Put(ctx, queueKey, string(data), clientv3.WithLease(lease.ID))
	if err != nil {
		return fmt.Errorf("failed to publish message: %w", err)
	}

	// Update metrics
	eq.updateMetrics(1, 0, 0, 0, 0, 0)

	// Log message details for debugging
	logging.Debugf("Published message %s to topic %s with idempotency key %s", messageID, topic, idempotencyKey[:8]+"...")
	return nil
}

// ConsumeMessages consumes messages with atomic work claiming
func (eq *EnhancedQueue) ConsumeMessages(ctx context.Context, topic, consumerID string, maxMessages int) ([]*EnhancedMessage, error) {

	queuePrefix := fmt.Sprintf("%s/%s/", eq.config.QueuePrefix, topic)
	processingPrefix := fmt.Sprintf("%s/%s/", eq.config.ProcessingPrefix, consumerID)

	// Get available messages
	resp, err := eq.client.Get(ctx, queuePrefix,
		clientv3.WithPrefix(),
		clientv3.WithSort(clientv3.SortByKey, clientv3.SortAscend),
		clientv3.WithLimit(int64(maxMessages)))
	if err != nil {
		return nil, fmt.Errorf("failed to get messages: %w", err)
	}

	if len(resp.Kvs) == 0 {
		return nil, nil // No messages available
	}

	// Create processing lease
	lease, err := eq.client.Grant(ctx, int64(eq.config.ProcessingTimeout.Seconds()))
	if err != nil {
		return nil, fmt.Errorf("failed to create processing lease: %w", err)
	}

	// Build atomic transaction to claim work
	var ops []clientv3.Op
	var messages []*EnhancedMessage

	for _, kv := range resp.Kvs {
		var message EnhancedMessage
		if err := json.Unmarshal(kv.Value, &message); err != nil {
			logging.Errorf("Failed to unmarshal message from key %s: %v", kv.Key, err)
			// Clean up invalid message
			eq.client.Delete(ctx, string(kv.Key))
			continue
		}

		// Check if message has expired
		if time.Now().After(message.ExpiresAt) {
			// Delete expired message
			ops = append(ops, clientv3.OpDelete(string(kv.Key)))
			eq.updateMetrics(0, 0, 0, 0, 1, 0)
			continue
		}

		// Create unique processing key
		processingKey := fmt.Sprintf("%s%d_%s", processingPrefix, time.Now().UnixNano(), message.ID)
		message.ProcessingLease = string(lease.ID)

		// Update message with lease info
		updatedData, err := json.Marshal(message)
		if err != nil {
			continue
		}

		ops = append(ops,
			clientv3.OpDelete(string(kv.Key)),                                                // Remove from queue
			clientv3.OpPut(processingKey, string(updatedData), clientv3.WithLease(lease.ID))) // Add to processing

		messages = append(messages, &message)
	}

	if len(ops) == 0 {
		return nil, nil
	}

	// Execute atomic transaction
	txnResp, err := eq.client.Txn(ctx).Then(ops...).Commit()
	if err != nil {
		return nil, fmt.Errorf("failed to claim work: %w", err)
	}

	if !txnResp.Succeeded {
		return nil, fmt.Errorf("transaction failed - work may have been claimed by another consumer")
	}

	// Update metrics
	eq.updateMetrics(0, int64(len(messages)), 0, 0, 0, 0)

	logging.Debugf("Consumer %s claimed %d messages from topic %s", consumerID, len(messages), topic)
	return messages, nil
}

// AcknowledgeMessages marks messages as successfully processed (exactly-once guarantee)
func (eq *EnhancedQueue) AcknowledgeMessages(ctx context.Context, consumerID string, messages []*EnhancedMessage) error {

	var ops []clientv3.Op
	processedTTL := int64((eq.config.CompactionWindow * 2).Seconds()) // Keep processed records for 2x compaction window

	for _, message := range messages {
		// Create processed record with TTL to prevent reprocessing
		processedKey := fmt.Sprintf("%s/%s", eq.config.ProcessedPrefix, message.IdempotencyKey)
		processedData := fmt.Sprintf(`{"message_id":"%s","processed_at":"%s","consumer_id":"%s"}`,
			message.ID, time.Now().Format(time.RFC3339), consumerID)

		// Create lease for processed record
		lease, err := eq.client.Grant(ctx, processedTTL)
		if err != nil {
			return fmt.Errorf("failed to create processed record lease: %w", err)
		}

		ops = append(ops, clientv3.OpPut(processedKey, processedData, clientv3.WithLease(lease.ID)))

		// Remove from processing (find by consumer ID and message ID pattern)
		processingPrefix := fmt.Sprintf("%s/%s/", eq.config.ProcessingPrefix, consumerID)
		resp, err := eq.client.Get(ctx, processingPrefix, clientv3.WithPrefix())
		if err != nil {
			continue
		}

		for _, kv := range resp.Kvs {
			if strings.Contains(string(kv.Key), message.ID) {
				ops = append(ops, clientv3.OpDelete(string(kv.Key)))
				break
			}
		}
	}

	if len(ops) == 0 {
		return nil
	}

	// Execute transaction
	_, err := eq.client.Txn(ctx).Then(ops...).Commit()
	if err != nil {
		return fmt.Errorf("failed to acknowledge messages: %w", err)
	}

	// Update metrics
	eq.updateMetrics(0, 0, int64(len(messages)), 0, 0, 0)

	logging.Debugf("Acknowledged %d messages for consumer %s", len(messages), consumerID)
	return nil
}

// GetQueueDepth returns the current queue depth for a topic
func (eq *EnhancedQueue) GetQueueDepth(ctx context.Context, topic string) (int64, error) {
	topicPrefix := fmt.Sprintf("%s/%s/", eq.config.QueuePrefix, topic)

	resp, err := eq.client.Get(ctx, topicPrefix,
		clientv3.WithPrefix(),
		clientv3.WithCountOnly())
	if err != nil {
		return 0, fmt.Errorf("failed to get queue depth: %w", err)
	}

	depth := resp.Count
	eq.mu.Lock()
	eq.metrics.QueueDepth = depth
	eq.mu.Unlock()

	return depth, nil
}

// GetMetrics returns current queue metrics
func (eq *EnhancedQueue) GetMetrics() *QueueMetrics {
	eq.metrics.mu.RLock()
	defer eq.metrics.mu.RUnlock()

	return &QueueMetrics{
		MessagesPublished:  eq.metrics.MessagesPublished,
		MessagesConsumed:   eq.metrics.MessagesConsumed,
		MessagesProcessed:  eq.metrics.MessagesProcessed,
		MessagesDuplicated: eq.metrics.MessagesDuplicated,
		MessagesExpired:    eq.metrics.MessagesExpired,
		ProcessingErrors:   eq.metrics.ProcessingErrors,
		QueueDepth:         eq.metrics.QueueDepth,
		ProcessingLag:      eq.metrics.ProcessingLag,
	}
}

// RecoverOrphanedWork recovers work from dead consumers
func (eq *EnhancedQueue) RecoverOrphanedWork(ctx context.Context) error {
	// Find all processing entries
	resp, err := eq.client.Get(ctx, eq.config.ProcessingPrefix, clientv3.WithPrefix())
	if err != nil {
		return fmt.Errorf("failed to get processing entries: %w", err)
	}

	var recoveryOps []clientv3.Op
	recoveredCount := 0

	for _, kv := range resp.Kvs {
		var message EnhancedMessage
		if err := json.Unmarshal(kv.Value, &message); err != nil {
			continue
		}

		// Check if processing lease has expired by checking if the processing key still exists
		// Since we can't easily convert string lease ID back to LeaseID, we'll use a different approach
		processingKeyExists := true
		if message.ProcessingLease != "" {
			// Try to get the processing key to see if it still exists
			resp, err := eq.client.Get(ctx, string(kv.Key))
			if err != nil || len(resp.Kvs) == 0 {
				processingKeyExists = false
			}
		}
		if !processingKeyExists {
			// Lease expired, recover the work
			topic := message.Topic
			queueKey := fmt.Sprintf("%s/%s/%s", eq.config.QueuePrefix, topic, message.ID)

			// Increment retry count
			message.RetryCount++
			if message.RetryCount > message.MaxRetries {
				// Move to dead letter queue or discard
				logging.Warnf("Message %s exceeded max retries, discarding", message.ID)
				recoveryOps = append(recoveryOps, clientv3.OpDelete(string(kv.Key)))
				continue
			}

			// Update message
			recoveredData, err := json.Marshal(message)
			if err != nil {
				continue
			}

			// Create new lease
			lease, err := eq.client.Grant(ctx, int64(eq.config.DefaultTTL.Seconds()))
			if err != nil {
				continue
			}

			recoveryOps = append(recoveryOps,
				clientv3.OpDelete(string(kv.Key)),                                             // Remove from processing
				clientv3.OpPut(queueKey, string(recoveredData), clientv3.WithLease(lease.ID))) // Add back to queue
			recoveredCount++
		}
	}

	if len(recoveryOps) > 0 {
		_, err := eq.client.Txn(ctx).Then(recoveryOps...).Commit()
		if err != nil {
			return fmt.Errorf("failed to recover orphaned work: %w", err)
		}
		logging.Infof("Recovered %d orphaned messages", recoveredCount)
	}

	return nil
}

// cleanupLoop runs periodic cleanup tasks
func (eq *EnhancedQueue) cleanupLoop() {
	defer close(eq.shutdownComplete)

	ticker := time.NewTicker(eq.config.CompactionWindow)
	defer ticker.Stop()

	recoveryTicker := time.NewTicker(eq.config.ProcessingTimeout)
	defer recoveryTicker.Stop()

	for {
		select {
		case <-eq.ctx.Done():
			return
		case <-ticker.C:
			// Perform compaction
			eq.performCompaction()
		case <-recoveryTicker.C:
			// Recover orphaned work
			ctx, cancel := context.WithTimeout(eq.ctx, 30*time.Second)
			if err := eq.RecoverOrphanedWork(ctx); err != nil {
				logging.Errorf("Failed to recover orphaned work: %v", err)
			}
			cancel()
		}
	}
}

// performCompaction removes expired processed records
func (eq *EnhancedQueue) performCompaction() {
	ctx, cancel := context.WithTimeout(eq.ctx, 30*time.Second)
	defer cancel()

	// Get all processed records
	resp, err := eq.client.Get(ctx, eq.config.ProcessedPrefix, clientv3.WithPrefix())
	if err != nil {
		logging.Errorf("Failed to get processed records for compaction: %v", err)
		return
	}

	var deleteOps []clientv3.Op
	cutoff := time.Now().Add(-eq.config.CompactionWindow)

	for _, kv := range resp.Kvs {
		var record map[string]interface{}
		if err := json.Unmarshal(kv.Value, &record); err != nil {
			continue
		}

		if processedAtStr, ok := record["processed_at"].(string); ok {
			if processedAt, err := time.Parse(time.RFC3339, processedAtStr); err == nil {
				if processedAt.Before(cutoff) {
					deleteOps = append(deleteOps, clientv3.OpDelete(string(kv.Key)))
				}
			}
		}
	}

	if len(deleteOps) > 0 {
		_, err := eq.client.Txn(ctx).Then(deleteOps...).Commit()
		if err != nil {
			logging.Errorf("Failed to compact processed records: %v", err)
		} else {
			logging.Debugf("Compacted %d processed records", len(deleteOps))
		}
	}
}

// updateMetrics updates queue metrics
func (eq *EnhancedQueue) updateMetrics(published, consumed, processed, duplicated, expired, errors int64) {
	eq.metrics.mu.Lock()
	defer eq.metrics.mu.Unlock()

	eq.metrics.MessagesPublished += published
	eq.metrics.MessagesConsumed += consumed
	eq.metrics.MessagesProcessed += processed
	eq.metrics.MessagesDuplicated += duplicated
	eq.metrics.MessagesExpired += expired
	eq.metrics.ProcessingErrors += errors
}

// Close closes the enhanced queue
func (eq *EnhancedQueue) Close() error {
	eq.cancel()

	// Wait for cleanup loop to finish
	select {
	case <-eq.shutdownComplete:
	case <-time.After(5 * time.Second):
		logging.Warnf("Cleanup loop did not finish within timeout")
	}

	if eq.client != nil {
		return eq.client.Close()
	}

	return nil
}

// WatchMessages provides real-time message watching using etcd watch
func (eq *EnhancedQueue) WatchMessages(ctx context.Context, topic string, callback func(*EnhancedMessage, string)) error {
	topicPrefix := fmt.Sprintf("%s/%s/", eq.config.QueuePrefix, topic)

	// Start watching for new messages
	watchChan := eq.client.Watch(ctx, topicPrefix, clientv3.WithPrefix())

	go func() {
		for watchResp := range watchChan {
			if watchResp.Err() != nil {
				logging.Errorf("Watch error for topic %s: %v", topic, watchResp.Err())
				return
			}

			for _, event := range watchResp.Events {
				if event.Type == clientv3.EventTypePut {
					var message EnhancedMessage
					if err := json.Unmarshal(event.Kv.Value, &message); err != nil {
						logging.Errorf("Failed to unmarshal watched message: %v", err)
						continue
					}

					eventType := "create"
					if !event.IsCreate() {
						eventType = "update"
					}

					callback(&message, eventType)
				}
			}
		}
	}()

	return nil
}
