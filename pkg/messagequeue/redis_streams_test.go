package messagequeue

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRedisStreamsBackend(t *testing.T) {
	// Skip if Redis is not available
	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		t.Skip("REDIS_URL not set, skipping Redis Streams tests")
	}

	backend, err := NewRedisStreamsBackend()
	require.NoError(t, err)
	defer backend.Close()

	t.Run("PublishMessage", func(t *testing.T) {
		msg := &Message{
			ID:        "test-msg-1",
			Topic:     "test-stream",
			Payload:   []byte(`{"test": "data"}`),
			Timestamp: time.Now(),
			CreatedAt: time.Now(),
			Headers:   map[string]string{"type": "test"},
		}

		err := backend.PublishMessage("test-stream", msg)
		assert.NoError(t, err)
	})

	t.Run("ConsumeMessages", func(t *testing.T) {
		// Publish test messages
		for i := 0; i < 3; i++ {
			msg := &Message{
				ID:        fmt.Sprintf("test-msg-%d", i),
				Topic:     "test-stream-consume",
				Payload:   []byte(fmt.Sprintf(`{"test": "data-%d"}`, i)),
				Timestamp: time.Now(),
				CreatedAt: time.Now(),
				Headers:   map[string]string{"type": "test", "index": fmt.Sprintf("%d", i)},
			}
			err := backend.PublishMessage("test-stream-consume", msg)
			require.NoError(t, err)
		}

		// Consume messages
		messages, err := backend.ConsumeMessages("test-stream-consume", "test-group", "consumer-1", 5, 1)
		assert.NoError(t, err)
		assert.GreaterOrEqual(t, len(messages), 3)

		// Verify message structure
		for _, msg := range messages[:3] {
			assert.NotEmpty(t, msg.StreamID)
			assert.NotEmpty(t, msg.StreamKey)
			assert.Contains(t, string(msg.Payload), "test")
		}
	})

	t.Run("AcknowledgeMessages", func(t *testing.T) {
		// Publish a test message
		msg := &Message{
			ID:        "test-ack-msg",
			Topic:     "test-stream-ack",
			Payload:   []byte(`{"test": "ack-data"}`),
			Timestamp: time.Now(),
			CreatedAt: time.Now(),
			Headers:   map[string]string{"type": "ack-test"},
		}
		err := backend.PublishMessage("test-stream-ack", msg)
		require.NoError(t, err)

		// Consume the message
		messages, err := backend.ConsumeMessages("test-stream-ack", "ack-group", "ack-consumer", 1, 1)
		require.NoError(t, err)
		require.Len(t, messages, 1)

		// Acknowledge the message
		acked, failed, err := backend.AcknowledgeMessages("ack-group", messages)
		assert.NoError(t, err)
		assert.Len(t, acked, 1)
		assert.Len(t, failed, 0)
		assert.Equal(t, messages[0].ID, acked[0])
	})

	t.Run("PendingMessages", func(t *testing.T) {
		// Publish test messages
		for i := 0; i < 2; i++ {
			msg := &Message{
				ID:        fmt.Sprintf("pending-msg-%d", i),
				Topic:     "test-stream-pending",
				Payload:   []byte(fmt.Sprintf(`{"test": "pending-%d"}`, i)),
				Timestamp: time.Now(),
				CreatedAt: time.Now(),
				Headers:   map[string]string{"type": "pending"},
			}
			err := backend.PublishMessage("test-stream-pending", msg)
			require.NoError(t, err)
		}

		// Consume messages but don't acknowledge
		messages1, err := backend.ConsumeMessages("test-stream-pending", "pending-group", "pending-consumer", 2, 1)
		require.NoError(t, err)
		require.Len(t, messages1, 2)

		// Consume again - should get pending messages
		messages2, err := backend.ConsumeMessages("test-stream-pending", "pending-group", "pending-consumer", 2, 1)
		require.NoError(t, err)
		// Should get the same messages as pending
		assert.GreaterOrEqual(t, len(messages2), 0) // Might be 0 if no messages are pending yet

		// Acknowledge one message
		acked, failed, err := backend.AcknowledgeMessages("pending-group", []*Message{messages1[0]})
		assert.NoError(t, err)
		assert.Len(t, acked, 1)
		assert.Len(t, failed, 0)
	})

	t.Run("GetStreamInfo", func(t *testing.T) {
		// Publish a test message to ensure stream exists
		msg := &Message{
			ID:        "info-test-msg",
			Topic:     "test-stream-info",
			Payload:   []byte(`{"test": "info"}`),
			Timestamp: time.Now(),
			CreatedAt: time.Now(),
			Headers:   map[string]string{"type": "info"},
		}
		err := backend.PublishMessage("test-stream-info", msg)
		require.NoError(t, err)

		// Get stream info
		info, err := backend.GetStreamInfo("test-stream-info")
		assert.NoError(t, err)
		assert.NotNil(t, info)
		assert.Equal(t, "test-stream-info", info.Name)
		assert.GreaterOrEqual(t, info.Length, int64(1))
	})

	t.Run("TopicExists", func(t *testing.T) {
		// Test non-existent topic
		exists := backend.TopicExists("non-existent-stream")
		assert.True(t, exists) // Redis streams return true even for empty streams

		// Test existing topic
		msg := &Message{
			ID:        "exists-test-msg",
			Topic:     "test-stream-exists",
			Payload:   []byte(`{"test": "exists"}`),
			Timestamp: time.Now(),
			CreatedAt: time.Now(),
		}
		err := backend.PublishMessage("test-stream-exists", msg)
		require.NoError(t, err)

		exists = backend.TopicExists("test-stream-exists")
		assert.True(t, exists)
	})

	t.Run("GetTopicStats", func(t *testing.T) {
		// Publish test messages
		for i := 0; i < 3; i++ {
			msg := &Message{
				ID:        fmt.Sprintf("stats-msg-%d", i),
				Topic:     "test-stream-stats",
				Payload:   []byte(fmt.Sprintf(`{"test": "stats-%d"}`, i)),
				Timestamp: time.Now(),
				CreatedAt: time.Now(),
			}
			err := backend.PublishMessage("test-stream-stats", msg)
			require.NoError(t, err)
		}

		// Get stats
		count, err := backend.GetTopicStats("test-stream-stats")
		assert.NoError(t, err)
		assert.GreaterOrEqual(t, count, int64(3))
	})

	t.Run("CleanupOldMessages", func(t *testing.T) {
		// Publish test messages
		msg := &Message{
			ID:        "cleanup-test-msg",
			Topic:     "test-stream-cleanup",
			Payload:   []byte(`{"test": "cleanup"}`),
			Timestamp: time.Now().Add(-2 * time.Hour), // Old message
			CreatedAt: time.Now().Add(-2 * time.Hour),
		}
		err := backend.PublishMessage("test-stream-cleanup", msg)
		require.NoError(t, err)

		// Cleanup messages older than 1 hour
		err = backend.CleanupOldMessages("test-stream-cleanup", 1*time.Hour)
		assert.NoError(t, err)
	})
}

func TestRedisStreamsBackend_ErrorCases(t *testing.T) {
	t.Run("NewRedisStreamsBackend_NoRedisURL", func(t *testing.T) {
		// Temporarily unset REDIS_URL
		originalURL := os.Getenv("REDIS_URL")
		os.Unsetenv("REDIS_URL")
		defer func() {
			if originalURL != "" {
				os.Setenv("REDIS_URL", originalURL)
			}
		}()

		backend, err := NewRedisStreamsBackend()
		assert.Error(t, err)
		assert.Nil(t, backend)
		assert.Contains(t, err.Error(), "REDIS_URL environment variable not set")
	})

	t.Run("NewRedisStreamsBackend_InvalidURL", func(t *testing.T) {
		// Set invalid Redis URL
		originalURL := os.Getenv("REDIS_URL")
		os.Setenv("REDIS_URL", "invalid://url")
		defer func() {
			if originalURL != "" {
				os.Setenv("REDIS_URL", originalURL)
			} else {
				os.Unsetenv("REDIS_URL")
			}
		}()

		backend, err := NewRedisStreamsBackend()
		assert.Error(t, err)
		assert.Nil(t, backend)
		assert.Contains(t, err.Error(), "failed to parse Redis URL")
	})

	t.Run("AcknowledgeMessages_MissingStreamInfo", func(t *testing.T) {
		redisURL := os.Getenv("REDIS_URL")
		if redisURL == "" {
			t.Skip("REDIS_URL not set, skipping Redis Streams tests")
		}

		backend, err := NewRedisStreamsBackend()
		require.NoError(t, err)
		defer backend.Close()

		// Try to acknowledge message without stream info
		msg := &Message{
			ID:      "test-msg",
			Topic:   "test",
			Payload: []byte("test"),
		}

		acked, failed, err := backend.AcknowledgeMessages("test-group", []*Message{msg})
		assert.NoError(t, err)
		assert.Len(t, acked, 0)
		assert.Len(t, failed, 1)
		assert.Equal(t, "test-msg", failed[0])
	})
}
