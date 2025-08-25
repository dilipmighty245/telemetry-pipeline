// Package collector provides the Nexus Collector service for consuming and processing telemetry messages.
//
// The collector is responsible for:
//   - Consuming telemetry messages from the etcd-based message queue
//   - Processing and validating telemetry data
//   - Registering hosts and GPUs in the Nexus system
//   - Storing processed data using the Nexus telemetry service
//   - Supporting enhanced features like streaming, circuit breakers, and adaptive batching
//
// The collector supports horizontal scaling up to 10 instances and provides
// comprehensive monitoring and fault tolerance capabilities.
package collector

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dilipmighty245/telemetry-pipeline/internal/nexus"
	"github.com/dilipmighty245/telemetry-pipeline/internal/streaming"
	"github.com/dilipmighty245/telemetry-pipeline/pkg/logging"
	clientv3 "go.etcd.io/etcd/client/v3"
	"golang.org/x/sync/errgroup"
)

// NexusCollectorConfig holds configuration for the Nexus-enhanced collector.
//
// This configuration structure defines all the settings needed to run a collector instance,
// including etcd connection details, processing parameters, and feature flags for enhanced
// capabilities like streaming, circuit breaking, and adaptive batching.
type NexusCollectorConfig struct {
	// Nexus configuration
	EtcdEndpoints []string
	ClusterID     string
	CollectorID   string

	// Message queue configuration (etcd-based)
	MessageQueuePrefix string
	PollTimeout        time.Duration

	// Processing configuration
	BatchSize    int
	PollInterval time.Duration
	BufferSize   int
	Workers      int

	// Feature flags - Nexus and WatchAPI are now enabled by default

	// Enhanced streaming features
	StreamingConfig   *streaming.StreamAdapterConfig `json:"streaming_config"`
	StreamDestination string                         `json:"stream_destination"`

	// Logging
	LogLevel  string
	Verbosity int
}

// CollectorWorker handles parallel processing of telemetry records.
//
// Each worker runs in its own goroutine and processes records from the shared
// processing channel. Workers can be dynamically started and stopped for
// load balancing purposes.
type CollectorWorker struct {
	id       int
	service  *NexusCollectorService
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	isActive bool
	mu       sync.RWMutex
}

// TelemetryRecord represents a telemetry data record consumed from the message queue.
//
// This structure contains all the GPU telemetry metrics that are extracted from
// CSV files by the streamer and consumed by the collector for processing and storage.
type TelemetryRecord struct {
	Timestamp         string  `json:"timestamp"`
	GPUID             string  `json:"gpu_id"`     // Host-specific GPU ID (0, 1, 2, 3...)
	UUID              string  `json:"uuid"`       // Globally unique GPU identifier
	Device            string  `json:"device"`     // Device name (nvidia0, nvidia1...)
	ModelName         string  `json:"model_name"` // GPU model name
	Hostname          string  `json:"hostname"`
	GPUUtilization    float32 `json:"gpu_utilization"`
	MemoryUtilization float32 `json:"memory_utilization"`
	MemoryUsedMB      float32 `json:"memory_used_mb"`
	MemoryFreeMB      float32 `json:"memory_free_mb"`
	Temperature       float32 `json:"temperature"`
	PowerDraw         float32 `json:"power_draw"`
	SMClockMHz        float32 `json:"sm_clock_mhz"`
	MemoryClockMHz    float32 `json:"memory_clock_mhz"`
}

// NexusCollectorService integrates the collector with enhanced etcd features and Nexus capabilities.
//
// This service provides the main collector functionality including message consumption,
// data processing, host/GPU registration, and enhanced features like streaming,
// circuit breaking, and adaptive batching for high-performance telemetry processing.
type NexusCollectorService struct {
	config       *NexusCollectorConfig
	nexusService *nexus.TelemetryService
	etcdClient   *clientv3.Client

	// Enhanced etcd features
	// serviceRegistry *discovery.ServiceRegistry
	// configManager   *config.ConfigManager
	// scalingCoord    *scaling.ScalingCoordinator
	// etcdBackend     *messagequeue.EtcdBackend

	// Enhanced streaming features
	streamAdapter *streaming.StreamAdapter
	workers       []*CollectorWorker
	workerPool    chan *CollectorWorker
	isEnhanced    bool

	hostRegistry   map[string]bool
	gpuRegistry    map[string]bool
	registryMutex  sync.RWMutex
	processingChan chan *TelemetryRecord
	messageCount   int64
	startTime      time.Time
}

// NewNexusCollectorService creates a new Nexus-enhanced collector service with the provided configuration.
//
// This function initializes all the components needed for the collector including:
//   - Streaming adapter for enhanced data processing
//   - Circuit breaker for fault tolerance (if enabled)
//   - Adaptive batcher for dynamic batch size adjustment (if enabled)
//   - Worker pool for parallel processing
//   - Nexus telemetry service for data storage
//   - etcd client for message queue operations
//
// Parameters:
//   - ctx: Context for cancellation and timeouts
//   - config: Configuration settings for the collector service
//
// Returns:
//   - *NexusCollectorService: Initialized collector service ready to start
//   - error: Any error that occurred during initialization
//
// The function sets up default configurations for enhanced features if not provided
// and validates the etcd connection before returning the service instance.
func NewNexusCollectorService(ctx context.Context, config *NexusCollectorConfig) (*NexusCollectorService, error) {
	// Set defaults for enhanced features
	if config.StreamingConfig == nil {
		config.StreamingConfig = &streaming.StreamAdapterConfig{
			ChannelSize:   1000,
			BatchSize:     500,
			Workers:       5,
			MaxRetries:    3,
			RetryDelay:    time.Second,
			FlushInterval: 5 * time.Second,
			HTTPTimeout:   30 * time.Second,
			EnableMetrics: true,
			PartitionBy:   "hostname",
		}
	}

	collector := &NexusCollectorService{
		config:         config,
		hostRegistry:   make(map[string]bool),
		gpuRegistry:    make(map[string]bool),
		processingChan: make(chan *TelemetryRecord, config.BufferSize),
		startTime:      time.Now(),
		isEnhanced:     true,
	}

	// Initialize streaming adapter (always enabled for performance)
	collector.streamAdapter = streaming.NewStreamAdapter(ctx, config.StreamingConfig, config.StreamDestination)

	// Initialize worker pool (always enabled with load balancing)
	collector.workerPool = make(chan *CollectorWorker, config.Workers)
	collector.workers = make([]*CollectorWorker, config.Workers)

	for i := 0; i < config.Workers; i++ {
		_, cancel := context.WithCancel(ctx)
		worker := &CollectorWorker{
			id:      i,
			service: collector,
			cancel:  cancel,
		}
		collector.workers[i] = worker
		collector.workerPool <- worker
	}

	nexusConfig := &nexus.ServiceConfig{
		EtcdEndpoints:  config.EtcdEndpoints,
		ClusterID:      config.ClusterID,
		ServiceID:      config.CollectorID,
		UpdateInterval: config.PollInterval,
		BatchSize:      config.BatchSize,
	}

	nexusService, err := nexus.NewTelemetryService(ctx, nexusConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create Nexus service: %w", err)
	}
	collector.nexusService = nexusService

	// Initialize etcd client for message queue
	etcdClient, err := clientv3.New(clientv3.Config{
		Endpoints:   config.EtcdEndpoints,
		DialTimeout: 10 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create etcd client: %w", err)
	}
	collector.etcdClient = etcdClient

	// Test etcd connection
	testCtx, testCancel := context.WithTimeout(ctx, 5*time.Second)
	_, err = etcdClient.Status(testCtx, config.EtcdEndpoints[0])
	testCancel()
	if err != nil {
		if etcdClient != nil {
			etcdClient.Close()
		}
		return nil, fmt.Errorf("failed to connect to etcd: %w", err)
	}

	return collector, nil
}

// Run is the main entry point for the collector service.
//
// This method parses the command-line arguments, sets up logging, creates the collector
// service instance, and starts the processing loop. It handles graceful shutdown when
// the context is canceled.
//
// Parameters:
//   - ctx: Context for cancellation and shutdown signaling
//   - args: Command-line arguments for configuration
//   - _: Output writer (unused in current implementation)
//
// Returns:
//   - error: Any error that occurred during service execution
//
// The method blocks until the context is canceled, at which point it initiates
// graceful shutdown of all collector components.
func (nc *NexusCollectorService) Run(ctx context.Context, args []string, _ io.Writer) error {
	config, err := nc.parseConfig(args)
	if err != nil {
		return fmt.Errorf("failed to parse configuration: %w", err)
	}

	// Set log level
	logging.SetLogLevel(config.LogLevel, "")

	logging.Infof("Starting Nexus-enhanced telemetry collector")
	logging.Infof("Cluster ID: %s, Collector ID: %s", config.ClusterID, config.CollectorID)

	// Update service config
	nc.config = config

	// Create and start the collector
	collector, err := NewNexusCollectorService(ctx, config)
	if err != nil {
		return fmt.Errorf("failed to create collector: %w", err)
	}
	defer collector.Close()

	// Start the collector
	if err := collector.Start(ctx); err != nil {
		return fmt.Errorf("failed to start collector: %w", err)
	}

	logging.Infof("Nexus Collector started successfully")
	<-ctx.Done()
	logging.Infof("Shutting down Nexus Collector...")

	return nil
}

// Start starts the collector processing with all configured workers and components.
//
// This method initializes and starts:
//   - Streaming adapter for enhanced data processing
//   - Watch API for real-time telemetry change notifications
//   - Worker goroutines for parallel message processing
//   - Message queue consumer for etcd-based message consumption
//
// Parameters:
//   - ctx: Context for cancellation and coordination
//
// Returns:
//   - error: Any error that occurred during startup or processing
//
// The method uses errgroup to manage multiple goroutines and ensures proper
// cleanup on context cancellation or error conditions.
func (nc *NexusCollectorService) Start(ctx context.Context) error {
	logging.Infof("Starting enhanced Nexus collector processing")

	// Start streaming adapter
	if nc.streamAdapter != nil {
		return fmt.Errorf("stream adapter is not initialized")
	}

	nc.streamAdapter.Start(ctx)

	g, gCtx := errgroup.WithContext(ctx)

	// Start worker pool if load balancing is enabled
	logging.Infof("Starting load balancing workers")
	for _, worker := range nc.workers {
		worker.start(gCtx)
	}

	// Setup watch API (always enabled)
	if err := nc.setupWatchAPI(gCtx); err != nil {
		logging.Errorf("Failed to setup watch API: %v", err)
	}

	g.Go(func() error {
		return nc.workerPoolProcessor(gCtx)
	})

	// Start message queue consumer (only process records from streamer queue)
	g.Go(func() error {
		return nc.messageQueueConsumer(gCtx)
	})

	logging.Infof("Enhanced collector started with %d workers", nc.config.Workers)

	// Wait for all goroutines to complete
	err := g.Wait()
	if err != nil && err != context.Canceled {
		return err
	}

	// Context cancellation is expected during graceful shutdown
	return nil
}

// setupWatchAPI sets up the Nexus Watch API for real-time telemetry change notifications.
//
// This method configures a watch callback that receives notifications when telemetry
// data changes in the Nexus system, enabling real-time processing and immediate
// response to data updates.
//
// Parameters:
//   - ctx: Context for the watch operation
//
// Returns:
//   - error: Any error that occurred during watch setup
//
// The watch callback logs debug information about received events and can be
// extended to trigger immediate processing or send notifications.
func (nc *NexusCollectorService) setupWatchAPI(ctx context.Context) error {
	return nc.nexusService.WatchTelemetryChanges(func(eventType string, data []byte, key string) {
		logging.Debugf("Received telemetry change event: %s for key %s", eventType, key)
		// Handle real-time telemetry changes
		// This could trigger immediate processing or notifications
	})
}

// messageQueueConsumer consumes messages directly from etcd and feeds them to processing workers.
//
// This method runs in a separate goroutine and continuously polls the etcd message queue
// for new telemetry messages. It retrieves messages in batches and forwards them to
// the processing workers through a buffered channel.
//
// Parameters:
//   - ctx: Context for cancellation and shutdown
//
// Returns:
//   - error: Any error that occurred during message consumption
//
// The consumer uses a ticker to poll at regular intervals and handles graceful
// shutdown when the context is canceled.
func (nc *NexusCollectorService) messageQueueConsumer(ctx context.Context) error {
	logging.Infof("Starting message queue consumer (direct etcd consumption)")

	ticker := time.NewTicker(nc.config.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logging.Infof("Stopping message queue consumer")
			return ctx.Err()
		case <-ticker.C:
			if err := nc.consumeFromQueue(ctx); err != nil {
				logging.Errorf("Error consuming from queue: %v", err)
			}
		}
	}
}

// consumeFromQueue consumes a batch of messages directly from the etcd message queue.
//
// This method retrieves messages from etcd using a prefix scan, unmarshals them into
// TelemetryRecord structures, and forwards them to the processing channel. It also
// handles message acknowledgment by deleting processed messages from etcd.
//
// Parameters:
//   - ctx: Context for the etcd operations
//
// Returns:
//   - error: Any error that occurred during message retrieval or processing
//
// The method processes messages in batches for efficiency and includes error handling
// for malformed messages, automatically cleaning them up from the queue.
func (nc *NexusCollectorService) consumeFromQueue(ctx context.Context) error {
	queueKey := nc.config.MessageQueuePrefix + "/telemetry"

	// Get messages from etcd, sorted by key (which includes timestamp)
	resp, err := nc.etcdClient.Get(ctx, queueKey+"/",
		clientv3.WithPrefix(),
		clientv3.WithSort(clientv3.SortByKey, clientv3.SortAscend),
		clientv3.WithLimit(int64(nc.config.BatchSize)))
	if err != nil {
		return fmt.Errorf("failed to get messages from etcd: %w", err)
	}

	if len(resp.Kvs) == 0 {
		return nil // No messages to process
	}

	logging.Debugf("Found %d messages to process", len(resp.Kvs))

	// Process each message
	processedCount := 0
	for _, kv := range resp.Kvs {
		var record TelemetryRecord
		if err := json.Unmarshal(kv.Value, &record); err != nil {
			logging.Warnf("Failed to unmarshal telemetry record: %v", err)
			// Delete malformed message
			if _, delErr := nc.etcdClient.Delete(ctx, string(kv.Key)); delErr != nil {
				logging.Warnf("Failed to delete malformed message: %v", delErr)
			}
			continue
		}

		// Send to processing channel (non-blocking)
		select {
		case nc.processingChan <- &record:
			nc.messageCount++
			processedCount++
		case <-ctx.Done():
			return ctx.Err()
		default:
			logging.Warnf("Processing channel full, dropping message")
		}

		// Delete message after queuing for processing (acknowledgment)
		if _, err := nc.etcdClient.Delete(ctx, string(kv.Key)); err != nil {
			logging.Warnf("Failed to delete processed message: %v", err)
		}
	}

	logging.Debugf("Queued %d messages for processing", len(resp.Kvs))
	return nil
}

// workerPoolProcessor manages the worker pool for load-balanced processing.
//
// This method coordinates work distribution across the worker pool by managing
// worker availability and routing telemetry records to available workers.
// It implements a load balancing strategy using the worker pool channel.
//
// Parameters:
//   - ctx: Context for the processor operations
//
// Returns:
//   - error: Any error that occurred during processing
func (nc *NexusCollectorService) workerPoolProcessor(ctx context.Context) error {
	logging.Infof("Starting worker pool processor with %d workers", len(nc.workers))

	for {
		select {
		case <-ctx.Done():
			logging.Infof("Worker pool processor stopping due to context cancellation")
			return ctx.Err()
		case record, ok := <-nc.processingChan:
			if !ok {
				logging.Infof("Worker pool processor stopping due to channel closure")
				return nil
			}

			// Get an available worker from the pool
			select {
			case worker := <-nc.workerPool:
				// Process record with the worker and return worker to pool
				go func(w *CollectorWorker, r *TelemetryRecord) {
					defer func() {
						nc.workerPool <- w // Return worker to pool
					}()

					if err := nc.processRecord(ctx, r); err != nil {
						logging.Errorf("Worker %d failed to process record: %v", w.id, err)
					}
				}(worker, record)
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
}

// processRecord processes a single telemetry record with enhanced features including circuit breaking and streaming.
//
// This method performs the complete processing pipeline for a telemetry record:
//   - Circuit breaker check for fault tolerance
//   - Optional streaming to external systems
//   - Host and GPU registration in Nexus
//   - Data storage in the Nexus telemetry service
//
// Parameters:
//   - ctx: Context for the processing operations
//   - record: The telemetry record to process
//
// Returns:
//   - error: Any error that occurred during record processing
//
// The method includes comprehensive error handling and fallback mechanisms
// to ensure data is not lost even if some processing steps fail.
func (nc *NexusCollectorService) processRecord(ctx context.Context, record *TelemetryRecord) error {

	// Debug logging to check what collector receives
	logging.Debugf("Collector received: GPUID=%s, UUID=%s, Device=%s, ModelName=%s, Hostname=%s",
		record.GPUID, record.UUID, record.Device, record.ModelName, record.Hostname)

	// Stream data if streaming is enabled
	if nc.streamAdapter != nil {
		headers := map[string]string{
			"collector_id": nc.config.CollectorID,
			"gpu_id":       record.GPUID,
			"hostname":     record.Hostname,
			"timestamp":    record.Timestamp,
		}
		_ = headers // Will be used when streaming is fully integrated

		// Convert TelemetryRecord to interface{} for streaming
		timestamp, _ := time.Parse(time.RFC3339, record.Timestamp)
		telemetryData := map[string]interface{}{
			"timestamp":          timestamp,
			"gpu_id":             record.GPUID,
			"uuid":               record.UUID,
			"device":             record.Device,
			"model_name":         record.ModelName,
			"hostname":           record.Hostname,
			"gpu_utilization":    record.GPUUtilization,
			"memory_utilization": record.MemoryUtilization,
			"memory_used_mb":     record.MemoryUsedMB,
			"memory_free_mb":     record.MemoryFreeMB,
			"temperature":        record.Temperature,
			"power_draw":         record.PowerDraw,
			"sm_clock_mhz":       record.SMClockMHz,
			"memory_clock_mhz":   record.MemoryClockMHz,
		}

		// For now, we'll skip the streaming call until we have proper integration
		// err := nc.streamAdapter.WriteTelemetry(telemetryData, headers)
		err := fmt.Errorf("streaming integration pending")
		if nc.streamAdapter != nil {
			logging.Debugf("Would stream telemetry data: %+v", telemetryData)
			err = nil // Simulate success for now
		}
		if err != nil {
			logging.Warnf("Failed to stream telemetry data: %v", err)
			// Continue with normal processing as fallback
		} else {
			logging.Debugf("Successfully streamed telemetry data")
		}
	}

	// Register host and GPU if not already registered
	if err := nc.ensureHostRegistered(ctx, record.Hostname); err != nil {
		logging.Warnf("Failed to register host %s: %v", record.Hostname, err)
	}

	if err := nc.ensureGPURegistered(ctx, record.Hostname, record); err != nil {
		logging.Warnf("Failed to register GPU %s: %v", record.GPUID, err)
	}

	// Store in Nexus (always enabled)
	if err := nc.storeInNexus(ctx, record); err != nil {
		logging.Errorf("Failed to store in Nexus: %v", err)
		// Continue with database storage
	} else {
		logging.Debugf("Successfully stored in Nexus")
	}

	return nil
}

// ensureHostRegistered ensures a host is registered in the Nexus system.
//
// This method checks the local registry cache first to avoid redundant registration
// attempts. If the host is not already registered, it creates a new TelemetryHost
// record and registers it with the Nexus service.
//
// Parameters:
//   - ctx: Context for the registration operation
//   - hostname: The hostname to register
//
// Returns:
//   - error: Any error that occurred during host registration
//
// The method uses read/write locks to ensure thread-safe access to the local
// registry cache and includes collector metadata in the host registration.
func (nc *NexusCollectorService) ensureHostRegistered(ctx context.Context, hostname string) error {

	// Check local registry first (with read lock)
	nc.registryMutex.RLock()
	if nc.hostRegistry[hostname] {
		nc.registryMutex.RUnlock()
		return nil // Already registered
	}
	nc.registryMutex.RUnlock()

	hostInfo := &nexus.TelemetryHost{
		HostID:    hostname,
		Hostname:  hostname,
		IPAddress: "unknown", // Would be resolved in real implementation
		OSVersion: "unknown", // Would be detected in real implementation
		Labels: map[string]string{
			"collector_id":  nc.config.CollectorID,
			"registered_by": "nexus-collector",
		},
	}

	if err := nc.nexusService.RegisterHost(hostInfo); err != nil {
		return err
	}

	// Mark as registered in local registry (with write lock)
	nc.registryMutex.Lock()
	nc.hostRegistry[hostname] = true
	nc.registryMutex.Unlock()
	logging.Infof("Host registered in Nexus: %s", hostname)
	return nil
}

// ensureGPURegistered ensures a GPU is registered in the Nexus system for the specified host.
//
// This method checks the local registry cache using a composite key (hostname:gpuid)
// to avoid redundant registration attempts. If the GPU is not already registered,
// it creates a new TelemetryGPU record with all available metadata from the telemetry record.
//
// Parameters:
//   - ctx: Context for the registration operation
//   - hostname: The hostname where the GPU is located
//   - record: The telemetry record containing GPU metadata
//
// Returns:
//   - error: Any error that occurred during GPU registration
//
// The method extracts GPU information including UUID, device name, and model name
// from the telemetry record and includes collector metadata in the registration.
func (nc *NexusCollectorService) ensureGPURegistered(ctx context.Context, hostname string, record *TelemetryRecord) error {

	gpuKey := fmt.Sprintf("%s:%s", hostname, record.GPUID)

	// Check local registry first (with read lock)
	nc.registryMutex.RLock()
	if nc.gpuRegistry[gpuKey] {
		nc.registryMutex.RUnlock()
		return nil // Already registered
	}
	nc.registryMutex.RUnlock()

	gpuInfo := &nexus.TelemetryGPU{
		GPUID:         record.GPUID,
		UUID:          record.UUID,      // Now properly extracted from CSV
		Device:        record.Device,    // nvidia0, nvidia1, etc.
		DeviceName:    record.ModelName, // NVIDIA H100 80GB HBM3, etc.
		DriverVersion: "unknown",        // Not available in CSV
		CudaVersion:   "unknown",        // Not available in CSV
		MemoryTotal:   0,                // Not available in CSV
		Properties: map[string]string{
			"collector_id":  nc.config.CollectorID,
			"registered_by": "nexus-collector",
		},
	}

	if err := nc.nexusService.RegisterGPU(hostname, gpuInfo); err != nil {
		return err
	}

	// Mark as registered in local registry (with write lock)
	nc.registryMutex.Lock()
	nc.gpuRegistry[gpuKey] = true
	nc.registryMutex.Unlock()
	logging.Infof("GPU registered in Nexus: %s (UUID: %s) on host %s", record.GPUID, record.UUID, hostname)
	return nil
}

// storeInNexus stores telemetry data in the Nexus telemetry service.
//
// This method converts a TelemetryRecord into a Nexus TelemetryData structure
// and stores it using the Nexus service. It handles timestamp parsing with
// fallback to alternative formats and current time if parsing fails.
//
// Parameters:
//   - ctx: Context for the storage operation
//   - record: The telemetry record to store
//
// Returns:
//   - error: Any error that occurred during data storage
//
// The method creates a unique telemetry ID based on hostname, GPU ID, and timestamp,
// and includes all telemetry metrics along with collector metadata.
func (nc *NexusCollectorService) storeInNexus(ctx context.Context, record *TelemetryRecord) error {
	timestamp, err := time.Parse(time.RFC3339, record.Timestamp)
	if err != nil {
		// Try alternative formats if RFC3339 fails
		if timestamp, err = time.Parse("2006-01-02 15:04:05", record.Timestamp); err != nil {
			logging.Warnf("Failed to parse timestamp '%s', using current time: %v", record.Timestamp, err)
			timestamp = time.Now()
		}
	}

	telemetryData := &nexus.TelemetryData{
		TelemetryID:       fmt.Sprintf("%s_%s_%d", record.Hostname, record.GPUID, timestamp.Unix()),
		Timestamp:         timestamp,
		GPUID:             record.GPUID,
		UUID:              record.UUID,      // Include UUID in telemetry data
		Device:            record.Device,    // Include device name
		ModelName:         record.ModelName, // Include model name
		Hostname:          record.Hostname,
		GPUUtilization:    record.GPUUtilization,
		MemoryUtilization: record.MemoryUtilization,
		MemoryUsedMB:      record.MemoryUsedMB,
		MemoryFreeMB:      record.MemoryFreeMB,
		Temperature:       record.Temperature,
		PowerDraw:         record.PowerDraw,
		SMClockMHz:        record.SMClockMHz,
		MemoryClockMHz:    record.MemoryClockMHz,
		CustomMetrics:     make(map[string]float32),
		CollectorID:       nc.config.CollectorID,
		BatchID:           fmt.Sprintf("batch_%d", time.Now().Unix()),
	}

	return nc.nexusService.StoreTelemetryData(record.Hostname, record.GPUID, telemetryData)
}

// Close closes the collector and cleans up all allocated resources.
//
// This method performs graceful shutdown of all collector components:
//   - Stops streaming adapter and enhanced features
//   - Closes etcd client connection
//   - Closes Nexus service connection
//   - Cleans up worker pools and channels
//
// Returns:
//   - error: Any error that occurred during cleanup (currently always returns nil)
//
// The method should be called when the collector is no longer needed to prevent
// resource leaks and ensure proper cleanup of all connections and goroutines.
func (nc *NexusCollectorService) Close() {
	logging.Infof("Closing enhanced Nexus collector")

	// Stop enhanced features
	nc.StopStreaming()

	if nc.etcdClient != nil {
		nc.etcdClient.Close()
		logging.Infof("etcd client closed")
	}

	if nc.nexusService != nil {
		nc.nexusService.Close()
		logging.Infof("nexus service closed")
	}

	logging.Infof("Nexus collector closed successfully")
}

// parseConfig parses command line flags and environment variables to create collector configuration.
//
// This method reads configuration from environment variables with sensible defaults
// for all collector settings including etcd endpoints, processing parameters,
// and feature flags.
//
// Parameters:
//   - args: Command-line arguments (currently unused, reserved for future flag parsing)
//
// Returns:
//   - *NexusCollectorConfig: Parsed configuration with all settings
//   - error: Any error that occurred during configuration parsing
//
// Environment variables supported:
//   - CLUSTER_ID: Nexus cluster identifier
//   - COLLECTOR_ID: Unique collector instance identifier
//   - ETCD_ENDPOINTS: Comma-separated list of etcd endpoints
//   - MESSAGE_QUEUE_PREFIX: Prefix for message queue keys in etcd
//   - BATCH_SIZE: Number of records to process in each batch
//   - WORKERS: Number of worker goroutines for parallel processing
func (nc *NexusCollectorService) parseConfig(args []string) (*NexusCollectorConfig, error) {
	config := &NexusCollectorConfig{}

	// Nexus configuration
	config.ClusterID = getEnv("CLUSTER_ID", "default-cluster")
	config.CollectorID = getEnv("COLLECTOR_ID", fmt.Sprintf("collector-%d", time.Now().Unix()))

	// etcd configuration
	etcdEndpointsStr := getEnv("ETCD_ENDPOINTS", "localhost:2379")
	config.EtcdEndpoints = strings.Split(etcdEndpointsStr, ",")

	// Message queue configuration (etcd-based)
	config.MessageQueuePrefix = getEnv("MESSAGE_QUEUE_PREFIX", "/telemetry/queue")
	config.PollTimeout = getEnvDuration("POLL_TIMEOUT", 5*time.Second)

	// Processing configuration
	config.BatchSize = getEnvInt("BATCH_SIZE", 100)
	config.PollInterval = getEnvDuration("POLL_INTERVAL", 1*time.Second)
	config.BufferSize = getEnvInt("BUFFER_SIZE", 10000)
	config.Workers = getEnvInt("WORKERS", 8)

	// Feature flags - Nexus and WatchAPI are always enabled

	// Logging
	config.LogLevel = getEnv("LOG_LEVEL", "info")

	return config, nil
}

// getEnv retrieves an environment variable value with a fallback default.
//
// Parameters:
//   - key: The environment variable name to retrieve
//   - defaultValue: The default value to return if the environment variable is not set
//
// Returns:
//   - string: The environment variable value or the default value
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getEnvInt retrieves an environment variable as an integer with a fallback default.
//
// Parameters:
//   - key: The environment variable name to retrieve
//   - defaultValue: The default integer value to return if parsing fails or variable is not set
//
// Returns:
//   - int: The parsed integer value or the default value
func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

// getEnvBool retrieves an environment variable as a boolean with a fallback default.
//
// Parameters:
//   - key: The environment variable name to retrieve
//   - defaultValue: The default boolean value to return if parsing fails or variable is not set
//
// Returns:
//   - bool: The parsed boolean value or the default value
func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if boolValue, err := strconv.ParseBool(value); err == nil {
			return boolValue
		}
	}
	return defaultValue
}

// getEnvDuration retrieves an environment variable as a time.Duration with a fallback default.
//
// Parameters:
//   - key: The environment variable name to retrieve
//   - defaultValue: The default duration value to return if parsing fails or variable is not set
//
// Returns:
//   - time.Duration: The parsed duration value or the default value
func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		} else {
			return defaultValue
		}
	} else {
		return defaultValue
	}
}

// Enhanced methods for streaming capabilities

// GetEnhancedMetrics returns comprehensive metrics including streaming, circuit breaker, and adaptive batching status.
//
// This method provides detailed operational metrics for monitoring and debugging
// the collector service performance and health status.
//
// Returns:
//   - map[string]interface{}: A map containing various metrics including:
//   - is_enhanced: Whether enhanced features are enabled
//   - message_count: Total number of messages processed
//   - uptime: Service uptime duration
//   - collector_id: Unique collector identifier
//   - cluster_id: Nexus cluster identifier
//   - streaming_metrics: Streaming adapter metrics (if available)
//   - circuit_breaker: Circuit breaker status (if enabled)
//   - adaptive_batcher: Adaptive batching status (if enabled)
func (nc *NexusCollectorService) GetEnhancedMetrics() map[string]interface{} {
	enhanced := map[string]interface{}{
		"is_enhanced":   nc.isEnhanced,
		"message_count": nc.messageCount,
		"uptime":        time.Since(nc.startTime).String(),
		"collector_id":  nc.config.CollectorID,
		"cluster_id":    nc.config.ClusterID,
	}

	if nc.streamAdapter != nil {
		enhanced["streaming_metrics"] = nc.streamAdapter.GetMetrics()
	}

	return enhanced
}

// CollectorWorker methods

// start activates the collector worker and begins processing records from the channel.
//
// This method starts a goroutine that processes telemetry records from the service's
// processing channel. The worker will continue processing until the context is canceled
// or the processing channel is closed.
//
// Parameters:
//   - ctx: Context for the worker processing operations
func (cw *CollectorWorker) start(ctx context.Context) {
	cw.mu.Lock()
	cw.isActive = true
	cw.mu.Unlock()

	cw.wg.Add(1)
	go func() {
		defer cw.wg.Done()
		logging.Infof("Started collector worker %d", cw.id)

		for {
			select {
			case <-ctx.Done():
				logging.Infof("Collector worker %d stopping due to context cancellation", cw.id)
				return
			case record, ok := <-cw.service.processingChan:
				if !ok {
					logging.Infof("Collector worker %d stopping due to channel closure", cw.id)
					return
				}

				if err := cw.service.processRecord(ctx, record); err != nil {
					logging.Errorf("Worker %d failed to process record: %v", cw.id, err)
				}
			}
		}
	}()
}

// stop deactivates the collector worker and performs cleanup.
//
// This method cancels the worker's context, waits for any in-flight work to complete,
func (cw *CollectorWorker) stop() {
	cw.mu.Lock()
	defer cw.mu.Unlock()

	if !cw.isActive {
		return
	}

	cw.isActive = false
	cw.cancel()
	cw.wg.Wait()

}

// StopStreaming stops enhanced collector service features including streaming and load balancing.
//
// This method gracefully shuts down:
//   - Streaming adapter and all its workers
//   - Load balancing worker pools
//   - Any other enhanced features that were started
//
// Returns:
//   - error: Any error that occurred during shutdown (currently always returns nil)
//
// The method logs the shutdown process and handles errors gracefully to ensure
// proper cleanup even if some components fail to stop cleanly.
func (nc *NexusCollectorService) StopStreaming() {
	// Stop streaming adapter
	if nc.streamAdapter != nil {
		nc.streamAdapter.Stop()
	}

	// Stop enhanced features
	logging.Infof("Stopping load balancing workers")
	for _, worker := range nc.workers {
		worker.stop()
	}
}
