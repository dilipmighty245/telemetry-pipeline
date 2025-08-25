package messagequeue

import (
	"context"
	"fmt"
	"time"

	"github.com/dilipmighty245/telemetry-pipeline/pkg/logging"
)

// MessageQueueService provides a high-level interface for the message queue
type MessageQueueService struct {
	queue         *MessageQueue
	cleanupTicker *time.Ticker
	ctx           context.Context
	cancel        context.CancelFunc
}

// NewMessageQueueService creates a new message queue service
func NewMessageQueueService() (*MessageQueueService, error) {
	ctx, cancel := context.WithCancel(context.Background())

	queue, err := NewMessageQueue()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create message queue: %w", err)
	}

	service := &MessageQueueService{
		queue:  queue,
		ctx:    ctx,
		cancel: cancel,
	}

	// Start cleanup routine
	service.startCleanupRoutine()

	return service, nil
}

// PublishTelemetry publishes telemetry data to the queue
func (mqs *MessageQueueService) PublishTelemetry(payload []byte, headers map[string]string) error {
	// Ensure telemetry topic exists
	err := mqs.queue.CreateTopic("telemetry", map[string]string{
		"max_size": "50000",
		"ttl":      "3600", // 1 hour
	})
	if err != nil {
		logging.Errorf("Failed to create telemetry topic: %v", err)
		return err
	}

	_, err = mqs.queue.Publish(mqs.ctx, "telemetry", payload, headers, 3600)
	if err != nil {
		logging.Errorf("Failed to publish telemetry message: %v", err)
		return err
	}

	return nil
}

// ConsumeTelemetry consumes telemetry data from the queue
func (mqs *MessageQueueService) ConsumeTelemetry(consumerGroup, consumerID string, maxMessages int) ([]*Message, error) {
	messages, err := mqs.queue.Consume(mqs.ctx, "telemetry", consumerGroup, consumerID, maxMessages, 30)
	if err != nil {
		logging.Errorf("Failed to consume telemetry messages: %v", err)
		return nil, err
	}

	return messages, nil
}

// AcknowledgeMessages acknowledges processed messages
func (mqs *MessageQueueService) AcknowledgeMessages(ctx context.Context, consumerGroup string, messages []*Message) error {
	acked, failed, err := mqs.queue.Acknowledge(ctx, consumerGroup, messages)
	if err != nil {
		logging.Errorf("Failed to acknowledge messages: %v", err)
		return err
	}

	if len(failed) > 0 {
		logging.Warnf("Failed to acknowledge %d messages: %v", len(failed), failed)
	}

	logging.Debugf("Successfully acknowledged %d messages", len(acked))
	return nil
}

// GetQueueStats returns queue statistics
func (mqs *MessageQueueService) GetQueueStats() *QueueStats {
	return mqs.queue.GetStats()
}

// CreateTopic creates a new topic with the given configuration
func (mqs *MessageQueueService) CreateTopic(name string, config map[string]string) error {
	return mqs.queue.CreateTopic(name, config)
}

// ListTopics returns all available topics
func (mqs *MessageQueueService) ListTopics(ctx context.Context) map[string]*Topic {
	return mqs.queue.ListTopics(ctx)
}

// Health checks the health of the message queue service
func (mqs *MessageQueueService) Health() bool {
	// Simple health check - service is healthy if context is not cancelled
	select {
	case <-mqs.ctx.Done():
		return false
	default:
		return true
	}
}

// Stop gracefully stops the message queue service
func (mqs *MessageQueueService) Stop() {
	logging.Infof("Stopping message queue service")

	if mqs.cleanupTicker != nil {
		mqs.cleanupTicker.Stop()
	}

	mqs.cancel()
	logging.Infof("Message queue service stopped")
}

// startCleanupRoutine starts the background cleanup routine for expired messages
func (mqs *MessageQueueService) startCleanupRoutine() {
	mqs.cleanupTicker = time.NewTicker(5 * time.Minute) // Cleanup every 5 minutes

	go func() {
		defer func() {
			if r := recover(); r != nil {
				logging.Errorf("Cleanup routine panic: %v", r)
			}
		}()

		for {
			select {
			case <-mqs.ctx.Done():
				logging.Infof("Stopping cleanup routine")
				return
			case <-mqs.cleanupTicker.C:
				logging.Debugf("Running message cleanup")
				mqs.queue.CleanupExpiredMessages()
			}
		}
	}()

	logging.Infof("Started message queue cleanup routine")
}

// GetMessageQueueInstance returns the underlying message queue instance
func (mqs *MessageQueueService) GetMessageQueueInstance() *MessageQueue {
	return mqs.queue
}

// PublishBatch publishes a batch of messages to the queue
func (mqs *MessageQueueService) PublishBatch(topic string, payloads [][]byte, headers []map[string]string, ttlSeconds int) ([]*Message, error) {
	var messages []*Message
	var errors []error

	for i, payload := range payloads {
		var msgHeaders map[string]string
		if i < len(headers) {
			msgHeaders = headers[i]
		}

		msg, err := mqs.queue.Publish(mqs.ctx, topic, payload, msgHeaders, ttlSeconds)
		if err != nil {
			errors = append(errors, err)
			logging.Errorf("Failed to publish message %d in batch: %v", i, err)
		} else {
			messages = append(messages, msg)
		}
	}

	if len(errors) > 0 {
		logging.Warnf("Published %d/%d messages successfully in batch", len(messages), len(payloads))
	} else {
		logging.Debugf("Successfully published batch of %d messages to topic %s", len(messages), topic)
	}

	return messages, nil
}

// ConsumeBatch consumes multiple batches of messages
func (mqs *MessageQueueService) ConsumeBatch(topics []string, consumerGroup, consumerID string, maxMessagesPerTopic int) (map[string][]*Message, error) {
	result := make(map[string][]*Message)

	for _, topic := range topics {
		messages, err := mqs.queue.Consume(mqs.ctx, topic, consumerGroup, consumerID, maxMessagesPerTopic, 30)
		if err != nil {
			logging.Errorf("Failed to consume from topic %s: %v", topic, err)
			continue
		}
		result[topic] = messages
	}

	return result, nil
}
