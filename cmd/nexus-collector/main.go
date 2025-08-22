package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/dilipmighty245/telemetry-pipeline/internal/nexus"
	log "github.com/sirupsen/logrus"
	clientv3 "go.etcd.io/etcd/client/v3"
)

// CollectorConfig holds configuration for the Nexus-enhanced collector
type CollectorConfig struct {
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

	// Database configuration
	DBHost     string
	DBPort     int
	DBUser     string
	DBPassword string
	DBName     string
	DBSSLMode  string

	// Feature flags
	EnableNexus     bool
	EnableWatchAPI  bool
	EnableGraphQL   bool
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

// NexusCollector integrates the existing collector with Nexus-style patterns
type NexusCollector struct {
	config         *CollectorConfig
	nexusService   *nexus.TelemetryService
	etcdClient     *clientv3.Client
	ctx            context.Context
	cancel         context.CancelFunc
	hostRegistry   map[string]bool
	gpuRegistry    map[string]bool
	registryMutex  sync.RWMutex
	processingChan chan *TelemetryRecord
}

func main() {
	config := parseFlags()

	// Set log level
	level, err := log.ParseLevel(config.LogLevel)
	if err != nil {
		log.Fatalf("Invalid log level: %v", err)
	}
	log.SetLevel(level)

	log.Infof("Starting Nexus-enhanced telemetry collector")
	log.Infof("Cluster ID: %s, Collector ID: %s", config.ClusterID, config.CollectorID)

	// Create and start the collector
	collector, err := NewNexusCollector(config)
	if err != nil {
		log.Fatalf("Failed to create collector: %v", err)
	}
	defer collector.Close()

	// Start the collector
	if err := collector.Start(); err != nil {
		log.Fatalf("Failed to start collector: %v", err)
	}

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	log.Info("Collector started successfully. Press Ctrl+C to stop.")
	<-sigChan

	log.Info("Shutting down collector...")
}

// parseFlags parses command line flags and environment variables
func parseFlags() *CollectorConfig {
	config := &CollectorConfig{}

	// Nexus configuration
	flag.StringVar(&config.ClusterID, "cluster-id", getEnv("CLUSTER_ID", "default-cluster"), "Telemetry cluster ID")
	flag.StringVar(&config.CollectorID, "collector-id", getEnv("COLLECTOR_ID", fmt.Sprintf("collector-%d", time.Now().Unix())), "Unique collector ID")

	// etcd configuration
	etcdEndpointsStr := getEnv("ETCD_ENDPOINTS", "localhost:2379")
	config.EtcdEndpoints = strings.Split(etcdEndpointsStr, ",")

	// Message queue configuration (etcd-based)
	flag.StringVar(&config.MessageQueuePrefix, "message-queue-prefix", getEnv("MESSAGE_QUEUE_PREFIX", "/telemetry/queue"), "etcd message queue prefix")
	flag.DurationVar(&config.PollTimeout, "poll-timeout", getEnvDuration("POLL_TIMEOUT", 5*time.Second), "Message polling timeout")

	// Processing configuration
	flag.IntVar(&config.BatchSize, "batch-size", getEnvInt("BATCH_SIZE", 100), "Batch size for processing")
	flag.DurationVar(&config.PollInterval, "poll-interval", getEnvDuration("POLL_INTERVAL", 1*time.Second), "Polling interval")
	flag.IntVar(&config.BufferSize, "buffer-size", getEnvInt("BUFFER_SIZE", 10000), "Buffer size for channels")
	flag.IntVar(&config.Workers, "workers", getEnvInt("WORKERS", 8), "Number of worker goroutines")

	// Database configuration
	flag.StringVar(&config.DBHost, "db-host", getEnv("DB_HOST", "localhost"), "Database host")
	flag.IntVar(&config.DBPort, "db-port", getEnvInt("DB_PORT", 5433), "Database port")
	flag.StringVar(&config.DBUser, "db-user", getEnv("DB_USER", "postgres"), "Database user")
	flag.StringVar(&config.DBPassword, "db-password", getEnv("DB_PASSWORD", "postgres"), "Database password")
	flag.StringVar(&config.DBName, "db-name", getEnv("DB_NAME", "telemetry"), "Database name")
	flag.StringVar(&config.DBSSLMode, "db-ssl-mode", getEnv("DB_SSL_MODE", "disable"), "Database SSL mode")

	// Feature flags
	flag.BoolVar(&config.EnableNexus, "enable-nexus", getEnvBool("ENABLE_NEXUS", true), "Enable Nexus integration")
	flag.BoolVar(&config.EnableWatchAPI, "enable-watch-api", getEnvBool("ENABLE_WATCH_API", true), "Enable Watch API")
	flag.BoolVar(&config.EnableGraphQL, "enable-graphql", getEnvBool("ENABLE_GRAPHQL", true), "Enable GraphQL API")
	flag.BoolVar(&config.EnableStreaming, "enable-streaming", getEnvBool("ENABLE_STREAMING", true), "Enable streaming processing")

	// Logging
	flag.StringVar(&config.LogLevel, "log-level", getEnv("LOG_LEVEL", "info"), "Log level (debug, info, warn, error)")

	flag.Parse()
	return config
}

// NewNexusCollector creates a new Nexus-enhanced collector
func NewNexusCollector(config *CollectorConfig) (*NexusCollector, error) {
	ctx, cancel := context.WithCancel(context.Background())

	collector := &NexusCollector{
		config:         config,
		ctx:            ctx,
		cancel:         cancel,
		hostRegistry:   make(map[string]bool),
		gpuRegistry:    make(map[string]bool),
		processingChan: make(chan *TelemetryRecord, config.BufferSize),
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
			EnableGraphQL:  config.EnableGraphQL,
		}

		nexusService, err := nexus.NewTelemetryService(nexusConfig)
		if err != nil {
			cancel()
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
		collector.Close()
		return nil, fmt.Errorf("failed to create etcd client: %w", err)
	}
	collector.etcdClient = etcdClient

	// Test etcd connection
	testCtx, testCancel := context.WithTimeout(context.Background(), 5*time.Second)
	_, err = etcdClient.Status(testCtx, config.EtcdEndpoints[0])
	testCancel()
	if err != nil {
		collector.Close()
		return nil, fmt.Errorf("failed to connect to etcd: %w", err)
	}

	return collector, nil
}

// Start starts the collector processing
func (nc *NexusCollector) Start() error {
	log.Info("Starting Nexus collector processing")

	// Setup watch API if enabled
	if nc.config.EnableNexus && nc.config.EnableWatchAPI {
		if err := nc.setupWatchAPI(); err != nil {
			log.Errorf("Failed to setup watch API: %v", err)
		}
	}

	// Start worker goroutines for processing
	for i := 0; i < nc.config.Workers; i++ {
		go nc.processingWorker(i)
	}

	// Start etcd message consumer
	go nc.etcdMessageConsumer()

	log.Infof("Collector started with %d workers", nc.config.Workers)
	return nil
}

// setupWatchAPI sets up the Nexus Watch API for real-time notifications
func (nc *NexusCollector) setupWatchAPI() error {
	return nc.nexusService.WatchTelemetryChanges(func(eventType string, data []byte, key string) {
		log.Debugf("Received telemetry change event: %s for key %s", eventType, key)
		// Handle real-time telemetry changes
		// This could trigger immediate processing or notifications
	})
}

// etcdMessageConsumer consumes messages from etcd message queue
func (nc *NexusCollector) etcdMessageConsumer() {
	log.Info("Starting etcd message consumer")

	queueKey := nc.config.MessageQueuePrefix + "/" + nc.config.ClusterID

	for {
		select {
		case <-nc.ctx.Done():
			return
		default:
			// Watch for new messages in the queue
			nc.consumeFromQueue(queueKey)
		}
	}
}

// consumeFromQueue consumes messages from a specific etcd queue
func (nc *NexusCollector) consumeFromQueue(queueKey string) {
	// Get all pending messages
	resp, err := nc.etcdClient.Get(nc.ctx, queueKey+"/", clientv3.WithPrefix(), clientv3.WithSort(clientv3.SortByKey, clientv3.SortAscend))
	if err != nil {
		log.Errorf("Failed to get messages from queue: %v", err)
		time.Sleep(nc.config.PollInterval)
		return
	}

	// Process each message
	for _, kv := range resp.Kvs {
		// Parse the message
		var record TelemetryRecord
		if err := json.Unmarshal(kv.Value, &record); err != nil {
			log.Errorf("Failed to parse telemetry record from key %s: %v", kv.Key, err)
			// Delete invalid message
			nc.etcdClient.Delete(nc.ctx, string(kv.Key))
			continue
		}

		// Send to processing channel
		select {
		case nc.processingChan <- &record:
			// Successfully queued for processing, delete from etcd queue
			if _, err := nc.etcdClient.Delete(nc.ctx, string(kv.Key)); err != nil {
				log.Errorf("Failed to delete processed message: %v", err)
			} else {
				log.Debugf("Consumed message from queue: %s", kv.Key)
			}
		case <-nc.ctx.Done():
			return
		default:
			log.Warn("Processing channel full, will retry message later")
			// Don't delete the message, it will be retried
			return
		}
	}

	// If no messages were found, wait before checking again
	if len(resp.Kvs) == 0 {
		time.Sleep(nc.config.PollInterval)
	}
}

// processingWorker runs a worker goroutine for processing telemetry data
func (nc *NexusCollector) processingWorker(workerID int) {
	log.Infof("Starting processing worker %d", workerID)

	for {
		select {
		case record := <-nc.processingChan:
			if err := nc.processRecord(record); err != nil {
				log.Errorf("Worker %d failed to process record: %v", workerID, err)
			}
		case <-nc.ctx.Done():
			log.Infof("Stopping processing worker %d", workerID)
			return
		}
	}
}

// processRecord processes a single telemetry record
func (nc *NexusCollector) processRecord(record *TelemetryRecord) error {
	// Debug logging to check what collector receives
	log.Debugf("Collector received: GPUID=%s, UUID=%s, Device=%s, ModelName=%s, Hostname=%s",
		record.GPUID, record.UUID, record.Device, record.ModelName, record.Hostname)

	// Register host and GPU if not already registered
	if err := nc.ensureHostRegistered(record.Hostname); err != nil {
		log.Warnf("Failed to register host %s: %v", record.Hostname, err)
	}

	if err := nc.ensureGPURegistered(record.Hostname, record); err != nil {
		log.Warnf("Failed to register GPU %s: %v", record.GPUID, err)
	}

	// Store in Nexus if enabled
	if nc.config.EnableNexus {
		if err := nc.storeInNexus(record); err != nil {
			log.Errorf("Failed to store in Nexus: %v", err)
			// Continue with database storage as fallback
		}
	}

	// Store in PostgreSQL database (existing functionality)
	if err := nc.storeInDatabase(record); err != nil {
		return fmt.Errorf("failed to store in database: %w", err)
	}

	return nil
}

// ensureHostRegistered ensures a host is registered in Nexus
func (nc *NexusCollector) ensureHostRegistered(hostname string) error {
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
func (nc *NexusCollector) ensureGPURegistered(hostname string, record *TelemetryRecord) error {
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
func (nc *NexusCollector) storeInNexus(record *TelemetryRecord) error {
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

// storeInDatabase stores telemetry data in PostgreSQL (existing functionality)
func (nc *NexusCollector) storeInDatabase(record *TelemetryRecord) error {
	// This would integrate with your existing database storage logic
	// For now, we'll just log that we're storing it
	log.Debugf("Storing in database: GPU %s on host %s", record.GPUID, record.Hostname)
	return nil
}

// Close closes the collector and cleans up resources
func (nc *NexusCollector) Close() error {
	log.Info("Closing Nexus collector")

	nc.cancel()

	if nc.etcdClient != nil {
		nc.etcdClient.Close()
	}

	if nc.nexusService != nil {
		nc.nexusService.Close()
	}

	return nil
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
