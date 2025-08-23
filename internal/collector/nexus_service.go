package collector

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/dilipmighty245/telemetry-pipeline/internal/nexus"
	"github.com/dilipmighty245/telemetry-pipeline/pkg/config"
	"github.com/dilipmighty245/telemetry-pipeline/pkg/discovery"
	"github.com/dilipmighty245/telemetry-pipeline/pkg/messagequeue"
	"github.com/dilipmighty245/telemetry-pipeline/pkg/scaling"
	log "github.com/sirupsen/logrus"
	clientv3 "go.etcd.io/etcd/client/v3"
	"golang.org/x/sync/errgroup"
)

// NexusCollectorConfig holds configuration for the Nexus-enhanced collector
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

	// Feature flags
	EnableNexus     bool
	EnableWatchAPI  bool
	EnableStreaming bool

	// Logging
	LogLevel string
}

// TelemetryRecord represents a telemetry data record from message queue
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

// NexusCollectorService integrates the existing collector with enhanced etcd features
type NexusCollectorService struct {
	config       *NexusCollectorConfig
	nexusService *nexus.TelemetryService
	etcdClient   *clientv3.Client

	// Enhanced etcd features
	serviceRegistry *discovery.ServiceRegistry
	configManager   *config.ConfigManager
	scalingCoord    *scaling.ScalingCoordinator
	etcdBackend     *messagequeue.EtcdBackend

	hostRegistry   map[string]bool
	gpuRegistry    map[string]bool
	registryMutex  sync.RWMutex
	processingChan chan *TelemetryRecord
	messageCount   int64
	startTime      time.Time
}

// NewNexusCollectorService creates a new Nexus-enhanced collector service
func NewNexusCollectorService(ctx context.Context, config *NexusCollectorConfig) (*NexusCollectorService, error) {
	collector := &NexusCollectorService{
		config:         config,
		hostRegistry:   make(map[string]bool),
		gpuRegistry:    make(map[string]bool),
		processingChan: make(chan *TelemetryRecord, config.BufferSize),
		startTime:      time.Now(),
	}

	// Initialize Nexus service if enabled
	if config.EnableNexus {
		nexusConfig := &nexus.ServiceConfig{
			EtcdEndpoints:  config.EtcdEndpoints,
			ClusterID:      config.ClusterID,
			ServiceID:      config.CollectorID,
			UpdateInterval: config.PollInterval,
			BatchSize:      config.BatchSize,
			EnableWatchAPI: config.EnableWatchAPI,
		}

		nexusService, err := nexus.NewTelemetryService(nexusConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to create Nexus service: %w", err)
		}
		collector.nexusService = nexusService

		log.Info("Nexus integration enabled")
	}

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
	testCtx, testCancel := context.WithTimeout(context.Background(), 5*time.Second)
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

// Run is the main entry point for the collector service
func (nc *NexusCollectorService) Run(args []string, _ io.Writer) error {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	config, err := nc.parseConfig(args)
	if err != nil {
		return fmt.Errorf("failed to parse configuration: %w", err)
	}

	// Set log level
	level, err := log.ParseLevel(config.LogLevel)
	if err != nil {
		return fmt.Errorf("invalid log level: %w", err)
	}
	log.SetLevel(level)

	log.Infof("Starting Nexus-enhanced telemetry collector")
	log.Infof("Cluster ID: %s, Collector ID: %s", config.ClusterID, config.CollectorID)

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

	log.Info("Nexus Collector started successfully")
	<-ctx.Done()
	log.Info("Shutting down Nexus Collector...")

	return nil
}

// Start starts the collector processing
func (nc *NexusCollectorService) Start(ctx context.Context) error {
	log.Info("Starting Nexus collector processing")

	g, gCtx := errgroup.WithContext(ctx)

	// Setup watch API if enabled
	if nc.config.EnableNexus && nc.config.EnableWatchAPI {
		if err := nc.setupWatchAPI(gCtx); err != nil {
			log.Errorf("Failed to setup watch API: %v", err)
		}
	}

	// Start worker goroutines for processing
	for i := 0; i < nc.config.Workers; i++ {
		workerID := i
		g.Go(func() error {
			return nc.processingWorker(gCtx, workerID)
		})
	}

	// Start message queue consumer (only process records from streamer queue)
	g.Go(func() error {
		return nc.messageQueueConsumer(gCtx)
	})

	log.Infof("Collector started with %d workers", nc.config.Workers)
	return g.Wait()
}

// setupWatchAPI sets up the Nexus Watch API for real-time notifications
func (nc *NexusCollectorService) setupWatchAPI(ctx context.Context) error {
	return nc.nexusService.WatchTelemetryChanges(func(eventType string, data []byte, key string) {
		log.Debugf("Received telemetry change event: %s for key %s", eventType, key)
		// Handle real-time telemetry changes
		// This could trigger immediate processing or notifications
	})
}

// messageQueueConsumer consumes messages directly from etcd and feeds them to processing workers
func (nc *NexusCollectorService) messageQueueConsumer(ctx context.Context) error {
	log.Info("Starting message queue consumer (direct etcd consumption)")

	ticker := time.NewTicker(nc.config.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Info("Stopping message queue consumer")
			return ctx.Err()
		case <-ticker.C:
			if err := nc.consumeFromQueue(ctx); err != nil {
				log.Errorf("Error consuming from queue: %v", err)
			}
		}
	}
}

// consumeFromQueue consumes messages directly from etcd message queue
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

	log.Debugf("Found %d messages to process", len(resp.Kvs))

	// Process each message
	for _, kv := range resp.Kvs {
		var record TelemetryRecord
		if err := json.Unmarshal(kv.Value, &record); err != nil {
			log.Warnf("Failed to unmarshal telemetry record: %v", err)
			// Delete malformed message
			nc.etcdClient.Delete(ctx, string(kv.Key))
			continue
		}

		// Send to processing channel (non-blocking)
		select {
		case nc.processingChan <- &record:
			nc.messageCount++
		case <-ctx.Done():
			return ctx.Err()
		default:
			log.Warnf("Processing channel full, dropping message")
		}

		// Delete message after queuing for processing (acknowledgment)
		if _, err := nc.etcdClient.Delete(ctx, string(kv.Key)); err != nil {
			log.Warnf("Failed to delete processed message: %v", err)
		}
	}

	log.Debugf("Queued %d messages for processing", len(resp.Kvs))
	return nil
}

// processingWorker runs a worker goroutine for processing telemetry data
func (nc *NexusCollectorService) processingWorker(ctx context.Context, workerID int) error {
	log.Infof("Starting processing worker %d", workerID)

	for {
		select {
		case record := <-nc.processingChan:
			if err := nc.processRecord(ctx, record); err != nil {
				log.Errorf("Worker %d failed to process record: %v", workerID, err)
			}
		case <-ctx.Done():
			log.Infof("Stopping processing worker %d", workerID)
			return ctx.Err()
		}
	}
}

// processRecord processes a single telemetry record
func (nc *NexusCollectorService) processRecord(ctx context.Context, record *TelemetryRecord) error {
	// Debug logging to check what collector receives
	log.Debugf("Collector received: GPUID=%s, UUID=%s, Device=%s, ModelName=%s, Hostname=%s",
		record.GPUID, record.UUID, record.Device, record.ModelName, record.Hostname)

	// Register host and GPU if not already registered
	if err := nc.ensureHostRegistered(ctx, record.Hostname); err != nil {
		log.Warnf("Failed to register host %s: %v", record.Hostname, err)
	}

	if err := nc.ensureGPURegistered(ctx, record.Hostname, record); err != nil {
		log.Warnf("Failed to register GPU %s: %v", record.GPUID, err)
	}

	// Store in Nexus if enabled
	if nc.config.EnableNexus {
		if err := nc.storeInNexus(ctx, record); err != nil {
			log.Errorf("Failed to store in Nexus: %v", err)
			// Continue with database storage
		}
	}

	return nil
}

// ensureHostRegistered ensures a host is registered in Nexus
func (nc *NexusCollectorService) ensureHostRegistered(ctx context.Context, hostname string) error {
	if !nc.config.EnableNexus {
		return nil
	}

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
	log.Infof("Host registered in Nexus: %s", hostname)
	return nil
}

// ensureGPURegistered ensures a GPU is registered in Nexus
func (nc *NexusCollectorService) ensureGPURegistered(ctx context.Context, hostname string, record *TelemetryRecord) error {
	if !nc.config.EnableNexus {
		return nil
	}

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
	log.Infof("GPU registered in Nexus: %s (UUID: %s) on host %s", record.GPUID, record.UUID, hostname)
	return nil
}

// storeInNexus stores telemetry data in Nexus
func (nc *NexusCollectorService) storeInNexus(ctx context.Context, record *TelemetryRecord) error {
	timestamp, err := time.Parse(time.RFC3339, record.Timestamp)
	if err != nil {
		// Try alternative formats if RFC3339 fails
		if timestamp, err = time.Parse("2006-01-02 15:04:05", record.Timestamp); err != nil {
			log.Warnf("Failed to parse timestamp '%s', using current time: %v", record.Timestamp, err)
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

// Close closes the collector and cleans up resources
func (nc *NexusCollectorService) Close() error {
	log.Info("Closing Nexus collector")

	if nc.etcdClient != nil {
		nc.etcdClient.Close()
	}

	if nc.nexusService != nil {
		nc.nexusService.Close()
	}

	return nil
}

// parseConfig parses command line flags and environment variables
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

	// Feature flags
	config.EnableNexus = getEnvBool("ENABLE_NEXUS", true)
	config.EnableWatchAPI = getEnvBool("ENABLE_WATCH_API", true)
	config.EnableStreaming = getEnvBool("ENABLE_STREAMING", true)

	// Logging
	config.LogLevel = getEnv("LOG_LEVEL", "info")

	return config, nil
}

// Utility functions for environment variable parsing
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if boolValue, err := strconv.ParseBool(value); err == nil {
			return boolValue
		}
	}
	return defaultValue
}

func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return defaultValue
}
