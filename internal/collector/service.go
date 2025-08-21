package collector

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/dilipmighty245/telemetry-pipeline/pkg/logging"
	"github.com/dilipmighty245/telemetry-pipeline/pkg/messagequeue"
	"github.com/dilipmighty245/telemetry-pipeline/pkg/models"
)

// CollectorConfig represents configuration for the collector service
type CollectorConfig struct {
	CollectorID    string          `json:"collector_id"`
	ConsumerGroup  string          `json:"consumer_group"`
	BatchSize      int             `json:"batch_size"`
	PollInterval   time.Duration   `json:"poll_interval"`
	MaxRetries     int             `json:"max_retries"`
	RetryDelay     time.Duration   `json:"retry_delay"`
	BufferSize     int             `json:"buffer_size"`
	EnableMetrics  bool            `json:"enable_metrics"`
	DatabaseConfig *DatabaseConfig `json:"database_config"`
}

// CollectorService handles collecting telemetry data from message queue and persisting to database
type CollectorService struct {
	config         *CollectorConfig
	messageQueue   *messagequeue.MessageQueueService
	database       *DatabaseService
	ctx            context.Context
	cancel         context.CancelFunc
	wg             sync.WaitGroup
	isRunning      bool
	mu             sync.RWMutex
	metrics        *CollectorMetrics
	lastError      error
	totalCollected int64
	buffer         []*models.TelemetryData
	bufferMu       sync.Mutex
}

// CollectorMetrics tracks collection metrics
type CollectorMetrics struct {
	TotalRecordsCollected int64     `json:"total_records_collected"`
	TotalBatchesCollected int64     `json:"total_batches_collected"`
	TotalRecordsPersisted int64     `json:"total_records_persisted"`
	TotalErrors           int64     `json:"total_errors"`
	LastCollectionTime    time.Time `json:"last_collection_time"`
	LastPersistTime       time.Time `json:"last_persist_time"`
	CollectionRate        float64   `json:"collection_rate"` // records per second
	StartTime             time.Time `json:"start_time"`
	mu                    sync.RWMutex
}

// NewCollectorService creates a new collector service
func NewCollectorService(config *CollectorConfig, mqService *messagequeue.MessageQueueService) (*CollectorService, error) {
	// Set default values
	if config.BatchSize <= 0 {
		config.BatchSize = 100
	}
	if config.PollInterval <= 0 {
		config.PollInterval = 1 * time.Second
	}
	if config.MaxRetries <= 0 {
		config.MaxRetries = 3
	}
	if config.RetryDelay <= 0 {
		config.RetryDelay = 1 * time.Second
	}
	if config.CollectorID == "" {
		config.CollectorID = "collector-" + time.Now().Format("20060102150405")
	}
	if config.ConsumerGroup == "" {
		config.ConsumerGroup = "telemetry-collectors"
	}
	if config.BufferSize <= 0 {
		config.BufferSize = 1000
	}

	// Create database service
	var dbService *DatabaseService
	var err error
	if config.DatabaseConfig != nil {
		dbService, err = NewDatabaseService(config.DatabaseConfig)
		if err != nil {
			return nil, err
		}
	}

	ctx, cancel := context.WithCancel(context.Background())

	service := &CollectorService{
		config:       config,
		messageQueue: mqService,
		database:     dbService,
		ctx:          ctx,
		cancel:       cancel,
		metrics: &CollectorMetrics{
			StartTime: time.Now(),
		},
		buffer: make([]*models.TelemetryData, 0, config.BufferSize),
	}

	logging.Infof("Created collector service %s", config.CollectorID)
	return service, nil
}

// Start starts the collector service
func (cs *CollectorService) Start() error {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	if cs.isRunning {
		return nil
	}

	cs.isRunning = true
	cs.wg.Add(2) // Collection and persistence loops

	go cs.collectionLoop()
	go cs.persistenceLoop()

	logging.Infof("Started collector service %s", cs.config.CollectorID)
	return nil
}

// Stop stops the collector service
func (cs *CollectorService) Stop() error {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	if !cs.isRunning {
		return nil
	}

	cs.isRunning = false
	cs.cancel()

	// Wait for loops to finish
	cs.wg.Wait()

	// Flush remaining buffer
	cs.flushBuffer()

	// Close database connection
	if cs.database != nil {
		cs.database.Close()
	}

	logging.Infof("Stopped collector service %s", cs.config.CollectorID)
	return nil
}

// IsRunning returns whether the service is currently running
func (cs *CollectorService) IsRunning() bool {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return cs.isRunning
}

// GetMetrics returns current collection metrics
func (cs *CollectorService) GetMetrics() *CollectorMetrics {
	cs.metrics.mu.RLock()
	defer cs.metrics.mu.RUnlock()

	// Calculate collection rate
	elapsed := time.Since(cs.metrics.StartTime).Seconds()
	if elapsed > 0 {
		cs.metrics.CollectionRate = float64(cs.metrics.TotalRecordsCollected) / elapsed
	}

	// Create a copy of metrics
	return &CollectorMetrics{
		TotalRecordsCollected: cs.metrics.TotalRecordsCollected,
		TotalBatchesCollected: cs.metrics.TotalBatchesCollected,
		TotalRecordsPersisted: cs.metrics.TotalRecordsPersisted,
		TotalErrors:           cs.metrics.TotalErrors,
		LastCollectionTime:    cs.metrics.LastCollectionTime,
		LastPersistTime:       cs.metrics.LastPersistTime,
		CollectionRate:        cs.metrics.CollectionRate,
		StartTime:             cs.metrics.StartTime,
	}
}

// GetLastError returns the last error encountered
func (cs *CollectorService) GetLastError() error {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return cs.lastError
}

// GetTotalCollected returns total number of records collected
func (cs *CollectorService) GetTotalCollected() int64 {
	return cs.totalCollected
}

// GetDatabaseService returns the database service instance
func (cs *CollectorService) GetDatabaseService() *DatabaseService {
	return cs.database
}

// collectionLoop is the main collection loop
func (cs *CollectorService) collectionLoop() {
	defer cs.wg.Done()
	defer func() {
		if r := recover(); r != nil {
			logging.Errorf("Collector service %s collection loop panic: %v", cs.config.CollectorID, r)
		}
	}()

	ticker := time.NewTicker(cs.config.PollInterval)
	defer ticker.Stop()

	logging.Infof("Started collection loop for service %s", cs.config.CollectorID)

	for {
		select {
		case <-cs.ctx.Done():
			logging.Infof("Collection loop stopped for service %s", cs.config.CollectorID)
			return
		case <-ticker.C:
			// Check context again before processing to ensure quick shutdown
			if cs.ctx.Err() != nil {
				logging.Infof("Context cancelled, stopping collection loop for service %s", cs.config.CollectorID)
				return
			}

			err := cs.collectBatch()
			if err != nil {
				// Don't log context cancellation as an error
				if cs.ctx.Err() != nil {
					logging.Infof("Collection stopped due to context cancellation")
					return
				}

				cs.mu.Lock()
				cs.lastError = err
				cs.mu.Unlock()

				cs.updateMetrics(0, 0, 0, 1)
			}
		}
	}
}

// persistenceLoop handles persisting collected data to database
func (cs *CollectorService) persistenceLoop() {
	defer cs.wg.Done()
	defer func() {
		if r := recover(); r != nil {
			logging.Errorf("Collector service %s persistence loop panic: %v", cs.config.CollectorID, r)
		}
	}()

	ticker := time.NewTicker(5 * time.Second) // Persist every 5 seconds
	defer ticker.Stop()

	logging.Infof("Started persistence loop for service %s", cs.config.CollectorID)

	for {
		select {
		case <-cs.ctx.Done():
			logging.Infof("Persistence loop stopped for service %s", cs.config.CollectorID)
			// Flush any remaining data before stopping
			cs.flushBuffer()
			return
		case <-ticker.C:
			// Check context before flushing to ensure quick shutdown
			if cs.ctx.Err() != nil {
				logging.Infof("Context cancelled, stopping persistence loop for service %s", cs.config.CollectorID)
				// Flush any remaining data before stopping
				cs.flushBuffer()
				return
			}
			cs.flushBuffer()
		}
	}
}

// collectBatch collects a batch of telemetry data from the message queue
func (cs *CollectorService) collectBatch() error {
	// Consume messages from message queue
	messages, err := cs.messageQueue.ConsumeTelemetry(
		cs.config.ConsumerGroup,
		cs.config.CollectorID,
		cs.config.BatchSize,
	)
	if err != nil {
		logging.Errorf("Failed to consume telemetry messages: %v", err)
		return err
	}

	if len(messages) == 0 {
		return nil // No messages to process
	}

	var telemetryData []*models.TelemetryData
	var messageIDs []string

	// Parse messages
	for _, msg := range messages {
		// Check context before processing each message for quick shutdown
		if cs.ctx.Err() != nil {
			logging.Infof("Context cancelled during message processing, stopping")
			return cs.ctx.Err()
		}

		var data models.TelemetryData
		err := json.Unmarshal(msg.Payload, &data)
		if err != nil {
			logging.Warnf("Failed to unmarshal telemetry data: %v", err)
			continue
		}

		telemetryData = append(telemetryData, &data)
		messageIDs = append(messageIDs, msg.ID)
	}

	if len(telemetryData) == 0 {
		return nil
	}

	// Add to buffer
	cs.addToBuffer(telemetryData)

	// Acknowledge processed messages
	err = cs.messageQueue.AcknowledgeMessages(cs.config.ConsumerGroup, messages)
	if err != nil {
		logging.Errorf("Failed to acknowledge messages: %v", err)
		// Don't return error here as data is already collected
	}

	// Update metrics
	cs.updateMetrics(int64(len(telemetryData)), 1, 0, 0)
	cs.totalCollected += int64(len(telemetryData))

	logging.Debugf("Collected batch of %d records from service %s", len(telemetryData), cs.config.CollectorID)
	return nil
}

// addToBuffer adds telemetry data to the internal buffer
func (cs *CollectorService) addToBuffer(data []*models.TelemetryData) {
	cs.bufferMu.Lock()
	defer cs.bufferMu.Unlock()

	cs.buffer = append(cs.buffer, data...)

	// If buffer is full, trigger immediate flush
	if len(cs.buffer) >= cs.config.BufferSize {
		go cs.flushBuffer()
	}
}

// flushBuffer persists buffered data to the database
func (cs *CollectorService) flushBuffer() {
	cs.bufferMu.Lock()
	if len(cs.buffer) == 0 {
		cs.bufferMu.Unlock()
		return
	}

	// Take a copy of the buffer and clear it
	dataToFlush := make([]*models.TelemetryData, len(cs.buffer))
	copy(dataToFlush, cs.buffer)
	cs.buffer = cs.buffer[:0] // Clear buffer
	cs.bufferMu.Unlock()

	// Persist to database if database service is available
	if cs.database != nil {
		err := cs.database.SaveTelemetryDataBatch(dataToFlush)
		if err != nil {
			logging.Errorf("Failed to persist telemetry data batch: %v", err)
			cs.updateMetrics(0, 0, 0, 1)

			// Return data to buffer on failure
			cs.bufferMu.Lock()
			cs.buffer = append(dataToFlush, cs.buffer...)
			cs.bufferMu.Unlock()
			return
		}

		// Update metrics
		cs.updateMetrics(0, 0, int64(len(dataToFlush)), 0)
		logging.Debugf("Persisted %d telemetry records to database", len(dataToFlush))
	} else {
		logging.Warnf("No database service configured, dropping %d telemetry records", len(dataToFlush))
	}
}

// updateMetrics updates collection metrics
func (cs *CollectorService) updateMetrics(recordsCollected, batchesCollected, recordsPersisted, errors int64) {
	cs.metrics.mu.Lock()
	defer cs.metrics.mu.Unlock()

	cs.metrics.TotalRecordsCollected += recordsCollected
	cs.metrics.TotalBatchesCollected += batchesCollected
	cs.metrics.TotalRecordsPersisted += recordsPersisted
	cs.metrics.TotalErrors += errors

	if recordsCollected > 0 {
		cs.metrics.LastCollectionTime = time.Now()
	}
	if recordsPersisted > 0 {
		cs.metrics.LastPersistTime = time.Now()
	}
}

// Health returns the health status of the collector service
func (cs *CollectorService) Health() bool {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	// Service is healthy if it's running and database is healthy
	if !cs.isRunning {
		return false
	}

	if cs.database != nil && !cs.database.Health() {
		return false
	}

	// Check if we have recent activity (within last 5 minutes)
	cs.metrics.mu.RLock()
	lastActivity := cs.metrics.LastCollectionTime
	cs.metrics.mu.RUnlock()

	if !lastActivity.IsZero() && time.Since(lastActivity) > 5*time.Minute {
		return false
	}

	return true
}

// GetConfig returns the collector configuration
func (cs *CollectorService) GetConfig() *CollectorConfig {
	return cs.config
}

// UpdateConfig updates the collector configuration (some fields only)
func (cs *CollectorService) UpdateConfig(newConfig *CollectorConfig) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	// Only update safe fields while running
	if newConfig.PollInterval > 0 {
		cs.config.PollInterval = newConfig.PollInterval
	}
	if newConfig.BatchSize > 0 {
		cs.config.BatchSize = newConfig.BatchSize
	}
	if newConfig.MaxRetries > 0 {
		cs.config.MaxRetries = newConfig.MaxRetries
	}
	if newConfig.RetryDelay > 0 {
		cs.config.RetryDelay = newConfig.RetryDelay
	}

	logging.Infof("Updated configuration for collector service %s", cs.config.CollectorID)
	return nil
}
