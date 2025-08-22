package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	configpkg "github.com/dilipmighty245/telemetry-pipeline/pkg/config"
	"github.com/dilipmighty245/telemetry-pipeline/pkg/discovery"
	"github.com/dilipmighty245/telemetry-pipeline/pkg/scaling"
	log "github.com/sirupsen/logrus"
	clientv3 "go.etcd.io/etcd/client/v3"
)

// StreamerConfig holds configuration for the Nexus streamer
type StreamerConfig struct {
	// etcd configuration
	EtcdEndpoints []string
	ClusterID     string
	StreamerID    string

	// Message queue configuration
	MessageQueuePrefix string

	// CSV configuration
	CSVFile        string
	BatchSize      int
	StreamInterval time.Duration
	LoopMode       bool

	// Logging
	LogLevel string
}

// TelemetryRecord represents a telemetry data record from CSV
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

// NexusStreamer streams telemetry data to etcd message queue
type NexusStreamer struct {
	config     *StreamerConfig
	etcdClient *clientv3.Client
	ctx        context.Context
	cancel     context.CancelFunc

	// Enhanced etcd features
	serviceRegistry *discovery.ServiceRegistry
	configManager   *configpkg.ConfigManager
	scalingCoord    *scaling.ScalingCoordinator

	messageCount int64
	startTime    time.Time
}

func main() {
	cfg := parseFlags()

	// Set log level
	level, err := log.ParseLevel(cfg.LogLevel)
	if err != nil {
		log.Fatalf("Invalid log level: %v", err)
	}
	log.SetLevel(level)

	log.Infof("Starting Nexus telemetry streamer")
	log.Infof("Cluster ID: %s, Streamer ID: %s", cfg.ClusterID, cfg.StreamerID)
	log.Infof("CSV File: %s, Batch Size: %d, Interval: %v", cfg.CSVFile, cfg.BatchSize, cfg.StreamInterval)

	// Create and start the streamer
	streamer, err := NewNexusStreamer(cfg)
	if err != nil {
		log.Fatalf("Failed to create streamer: %v", err)
	}
	defer streamer.Close()

	// Start streaming
	if err := streamer.Start(); err != nil {
		log.Fatalf("Failed to start streamer: %v", err)
	}

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	log.Info("Streamer started successfully. Press Ctrl+C to stop.")
	<-sigChan

	log.Info("Shutting down streamer...")
	streamer.PrintStats()
}

// parseFlags parses command line flags and environment variables
func parseFlags() *StreamerConfig {
	cfg := &StreamerConfig{}

	// etcd configuration
	flag.StringVar(&cfg.ClusterID, "cluster-id", getEnv("CLUSTER_ID", "default-cluster"), "Telemetry cluster ID")
	flag.StringVar(&cfg.StreamerID, "streamer-id", getEnv("STREAMER_ID", fmt.Sprintf("streamer-%d", time.Now().Unix())), "Unique streamer ID")

	etcdEndpointsStr := getEnv("ETCD_ENDPOINTS", "localhost:2379")
	cfg.EtcdEndpoints = strings.Split(etcdEndpointsStr, ",")

	// Message queue configuration
	flag.StringVar(&cfg.MessageQueuePrefix, "message-queue-prefix", getEnv("MESSAGE_QUEUE_PREFIX", "/telemetry/queue"), "etcd message queue prefix")

	// CSV configuration
	flag.StringVar(&cfg.CSVFile, "csv", getEnv("CSV_FILE", "dcgm_metrics_20250718_134233.csv"), "CSV file to stream")
	flag.IntVar(&cfg.BatchSize, "batch-size", getEnvInt("BATCH_SIZE", 100), "Batch size for streaming")
	flag.DurationVar(&cfg.StreamInterval, "stream-interval", getEnvDuration("STREAM_INTERVAL", 3*time.Second), "Interval between batches")
	flag.BoolVar(&cfg.LoopMode, "loop", getEnvBool("LOOP_MODE", false), "Loop through CSV file continuously")

	// Logging
	flag.StringVar(&cfg.LogLevel, "log-level", getEnv("LOG_LEVEL", "info"), "Log level (debug, info, warn, error)")

	flag.Parse()
	return cfg
}

// NewNexusStreamer creates a new etcd-based streamer
func NewNexusStreamer(config *StreamerConfig) (*NexusStreamer, error) {
	ctx, cancel := context.WithCancel(context.Background())

	// Create etcd client
	etcdClient, err := clientv3.New(clientv3.Config{
		Endpoints:   config.EtcdEndpoints,
		DialTimeout: 10 * time.Second,
	})
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create etcd client: %w", err)
	}

	// Test etcd connection
	testCtx, testCancel := context.WithTimeout(context.Background(), 5*time.Second)
	_, err = etcdClient.Status(testCtx, config.EtcdEndpoints[0])
	testCancel()
	if err != nil {
		etcdClient.Close()
		cancel()
		return nil, fmt.Errorf("failed to connect to etcd: %w", err)
	}

	// Initialize enhanced etcd features
	serviceRegistry := discovery.NewServiceRegistry(etcdClient, 30)
	configManager := configpkg.NewConfigManager(etcdClient)
	scalingRules := &scaling.ScalingRules{
		MinInstances:       1,
		MaxInstances:       10,
		ScaleUpThreshold:   0.8,
		ScaleDownThreshold: 0.3,
		CooldownPeriod:     5 * time.Minute,
		MetricWeights: map[string]float64{
			"cpu":    0.3,
			"memory": 0.3,
			"queue":  0.4,
		},
	}
	scalingCoord := scaling.NewScalingCoordinator(etcdClient, "nexus-streamer", config.StreamerID, scalingRules)

	streamer := &NexusStreamer{
		config:          config,
		etcdClient:      etcdClient,
		ctx:             ctx,
		cancel:          cancel,
		serviceRegistry: serviceRegistry,
		configManager:   configManager,
		scalingCoord:    scalingCoord,
		startTime:       time.Now(),
	}

	return streamer, nil
}

// Start starts the streaming process
func (ns *NexusStreamer) Start() error {
	log.Info("Starting enhanced etcd-based telemetry streaming")

	// Load initial configuration
	if err := ns.configManager.LoadInitialConfig(); err != nil {
		log.Warnf("Failed to load initial config: %v", err)
	}

	// Set default configuration values
	defaults := map[string]interface{}{
		"streamer/batch-size":      ns.config.BatchSize,
		"streamer/stream-interval": ns.config.StreamInterval.String(),
		"streamer/rate-limit":      1000.0,
		"streamer/enabled":         true,
	}
	if err := ns.configManager.SetDefaults(defaults); err != nil {
		log.Warnf("Failed to set default config: %v", err)
	}

	// Register service
	serviceInfo := discovery.ServiceInfo{
		ID:      ns.config.StreamerID,
		Type:    "nexus-streamer",
		Address: "localhost", // This could be determined dynamically
		Port:    8080,
		Metadata: map[string]string{
			"cluster_id": ns.config.ClusterID,
			"csv_file":   ns.config.CSVFile,
			"version":    "2.0.0",
		},
		Health:  "healthy",
		Version: "2.0.0",
	}

	if err := ns.serviceRegistry.Register(ns.ctx, serviceInfo); err != nil {
		log.Errorf("Failed to register service: %v", err)
	} else {
		log.Infof("Service registered: %s", ns.config.StreamerID)
	}

	// Start scaling coordinator
	if err := ns.scalingCoord.Start(); err != nil {
		log.Errorf("Failed to start scaling coordinator: %v", err)
	}

	// Start configuration watcher
	go ns.watchConfiguration()

	// Start streaming in a goroutine
	go ns.streamingLoop()

	return nil
}

// streamingLoop runs the main streaming loop
func (ns *NexusStreamer) streamingLoop() {
	for {
		select {
		case <-ns.ctx.Done():
			return
		default:
			if err := ns.streamCSVFile(); err != nil {
				log.Errorf("Error streaming CSV file: %v", err)
				time.Sleep(ns.config.StreamInterval)
				continue
			}

			if !ns.config.LoopMode {
				log.Info("Single-pass mode completed, stopping streamer")
				ns.cancel()
				return
			}

			log.Infof("Completed CSV file streaming, waiting %v before next iteration", ns.config.StreamInterval)
			time.Sleep(ns.config.StreamInterval)
		}
	}
}

// streamCSVFile reads and streams a CSV file to etcd message queue
func (ns *NexusStreamer) streamCSVFile() error {
	file, err := os.Open(ns.config.CSVFile)
	if err != nil {
		return fmt.Errorf("failed to open CSV file: %w", err)
	}
	defer file.Close()

	reader := csv.NewReader(file)

	// Read header
	header, err := reader.Read()
	if err != nil {
		return fmt.Errorf("failed to read CSV header: %w", err)
	}

	log.Debugf("CSV header: %v", header)

	// Create column mapping
	columnMap := make(map[string]int)
	for i, col := range header {
		columnMap[strings.ToLower(strings.TrimSpace(col))] = i
	}

	batch := make([]*TelemetryRecord, 0, ns.config.BatchSize)
	recordCount := 0

	for {
		select {
		case <-ns.ctx.Done():
			return nil
		default:
			record, err := reader.Read()
			if err == io.EOF {
				// Process remaining batch
				if len(batch) > 0 {
					if err := ns.publishBatch(batch); err != nil {
						log.Errorf("Failed to publish final batch: %v", err)
					}
				}
				log.Infof("✅ Finished streaming CSV file, processed %d records total", recordCount)
				log.Infof("📊 Processing Summary:")
				log.Infof("   - Total records processed: %d", recordCount)
				log.Infof("   - Batch size: %d", ns.config.BatchSize)
				log.Infof("   - Stream interval: %v", ns.config.StreamInterval)
				return nil
			}
			if err != nil {
				log.Errorf("Error reading CSV record: %v", err)
				continue
			}

			// Parse record
			telemetryRecord, err := ns.parseCSVRecord(record, columnMap)
			if err != nil {
				log.Errorf("Failed to parse CSV record: %v", err)
				continue
			}

			batch = append(batch, telemetryRecord)
			recordCount++

			// Publish batch when it's full
			if len(batch) >= ns.config.BatchSize {
				if err := ns.publishBatch(batch); err != nil {
					log.Errorf("Failed to publish batch: %v", err)
				} else {
					log.Infof("📤 Published batch of %d records (total processed: %d)", len(batch), recordCount)
				}
				batch = batch[:0] // Reset batch

				// Wait between batches
				time.Sleep(ns.config.StreamInterval)
			}
		}
	}
}

// parseCSVRecord parses a CSV record into a TelemetryRecord
func (ns *NexusStreamer) parseCSVRecord(record []string, columnMap map[string]int) (*TelemetryRecord, error) {
	tr := &TelemetryRecord{}

	// Helper function to get column value
	getColumn := func(name string) string {
		if idx, ok := columnMap[name]; ok && idx < len(record) {
			return strings.TrimSpace(record[idx])
		}
		return ""
	}

	// Helper function to parse float
	parseFloat := func(value string) float32 {
		if value == "" {
			return 0
		}
		if f, err := strconv.ParseFloat(value, 32); err == nil {
			return float32(f)
		}
		return 0
	}

	// Map CSV columns to struct fields
	tr.Timestamp = getColumn("timestamp")
	if tr.Timestamp == "" {
		tr.Timestamp = time.Now().Format(time.RFC3339)
	} else {
		// Remove quotes if present and ensure RFC3339 format
		tr.Timestamp = strings.Trim(tr.Timestamp, "\"")
		// Validate and normalize timestamp format
		if parsedTime, err := time.Parse(time.RFC3339, tr.Timestamp); err == nil {
			tr.Timestamp = parsedTime.Format(time.RFC3339)
		} else {
			log.Warnf("Invalid timestamp format '%s', using current time", tr.Timestamp)
			tr.Timestamp = time.Now().Format(time.RFC3339)
		}
	}

	tr.GPUID = getColumn("gpu_id")
	if tr.GPUID == "" {
		tr.GPUID = getColumn("gpu")
	}

	tr.UUID = getColumn("uuid")
	tr.Device = getColumn("device")
	tr.ModelName = getColumn("modelname") // CSV header "modelName" becomes "modelname" after ToLower()
	if tr.ModelName == "" {
		tr.ModelName = getColumn("model_name")
	}

	// Debug logging to check UUID extraction
	log.Debugf("Parsed record: GPUID=%s, UUID=%s, Device=%s, ModelName=%s, Hostname=%s",
		tr.GPUID, tr.UUID, tr.Device, tr.ModelName, tr.Hostname)

	tr.Hostname = getColumn("hostname") // CSV header "Hostname" becomes "hostname" after ToLower()
	if tr.Hostname == "" {
		tr.Hostname = getColumn("host")
	}

	tr.GPUUtilization = parseFloat(getColumn("gpu_utilization"))
	tr.MemoryUtilization = parseFloat(getColumn("memory_utilization"))
	tr.MemoryUsedMB = parseFloat(getColumn("memory_used_mb"))
	tr.MemoryFreeMB = parseFloat(getColumn("memory_free_mb"))
	tr.Temperature = parseFloat(getColumn("temperature"))
	tr.PowerDraw = parseFloat(getColumn("power_draw"))
	tr.SMClockMHz = parseFloat(getColumn("sm_clock_mhz"))
	tr.MemoryClockMHz = parseFloat(getColumn("memory_clock_mhz"))

	// Validate required fields
	if tr.GPUID == "" || tr.Hostname == "" {
		return nil, fmt.Errorf("missing required fields: gpu_id=%s, hostname=%s", tr.GPUID, tr.Hostname)
	}

	return tr, nil
}

// publishBatch publishes a batch of telemetry records to etcd message queue
func (ns *NexusStreamer) publishBatch(batch []*TelemetryRecord) error {
	if len(batch) == 0 {
		return nil
	}

	queueKey := ns.config.MessageQueuePrefix + "/" + ns.config.ClusterID

	// Use etcd transaction for atomic batch publishing
	ops := make([]clientv3.Op, 0, len(batch))

	for _, record := range batch {
		// Create unique key for each message
		messageKey := fmt.Sprintf("%s/%d_%s_%s_%d",
			queueKey,
			time.Now().UnixNano(),
			record.Hostname,
			record.GPUID,
			ns.messageCount)

		// Serialize record
		data, err := json.Marshal(record)
		if err != nil {
			log.Errorf("Failed to marshal telemetry record: %v", err)
			continue
		}

		ops = append(ops, clientv3.OpPut(messageKey, string(data)))
		ns.messageCount++
	}

	if len(ops) == 0 {
		return fmt.Errorf("no valid records in batch")
	}

	// Execute transaction
	ctx, cancel := context.WithTimeout(ns.ctx, 10*time.Second)
	defer cancel()

	txnResp, err := ns.etcdClient.Txn(ctx).Then(ops...).Commit()
	if err != nil {
		return fmt.Errorf("failed to publish batch to etcd: %w", err)
	}

	if !txnResp.Succeeded {
		return fmt.Errorf("etcd transaction failed")
	}

	log.Infof("Published batch of %d telemetry records to etcd queue", len(batch))
	return nil
}

// PrintStats prints streaming statistics
func (ns *NexusStreamer) PrintStats() {
	duration := time.Since(ns.startTime)
	rate := float64(ns.messageCount) / duration.Seconds()

	log.Infof("Streaming Statistics:")
	log.Infof("  Total Messages: %d", ns.messageCount)
	log.Infof("  Duration: %v", duration)
	log.Infof("  Rate: %.2f messages/second", rate)
}

// watchConfiguration watches for configuration changes
func (ns *NexusStreamer) watchConfiguration() {
	// Watch for batch size changes
	batchSizeChan := ns.configManager.WatchConfig("streamer/batch-size")

	// Watch for stream interval changes
	intervalChan := ns.configManager.WatchConfig("streamer/stream-interval")

	// Watch for rate limit changes
	rateLimitChan := ns.configManager.WatchConfig("streamer/rate-limit")

	for {
		select {
		case <-batchSizeChan:
			if newBatchSize := ns.configManager.GetInt("streamer/batch-size", ns.config.BatchSize); newBatchSize != ns.config.BatchSize {
				log.Infof("Updating batch size from %d to %d", ns.config.BatchSize, newBatchSize)
				ns.config.BatchSize = newBatchSize
			}

		case <-intervalChan:
			if intervalStr := ns.configManager.GetString("streamer/stream-interval", ns.config.StreamInterval.String()); intervalStr != ns.config.StreamInterval.String() {
				if newInterval, err := time.ParseDuration(intervalStr); err == nil {
					log.Infof("Updating stream interval from %v to %v", ns.config.StreamInterval, newInterval)
					ns.config.StreamInterval = newInterval
				}
			}

		case <-rateLimitChan:
			rateLimit := ns.configManager.GetFloat("streamer/rate-limit", 1000.0)
			log.Infof("Rate limit updated to %.2f messages/sec", rateLimit)

		case <-ns.ctx.Done():
			return
		}
	}
}

// reportMetrics reports instance metrics for scaling decisions
func (ns *NexusStreamer) reportMetrics() {
	metrics := scaling.InstanceMetrics{
		CPUUsage:       0.5,                            // This would be real CPU usage
		MemoryUsage:    0.4,                            // This would be real memory usage
		QueueDepth:     float64(ns.messageCount % 100), // Simulated queue depth
		ProcessingRate: float64(ns.messageCount) / time.Since(ns.startTime).Seconds(),
		ErrorRate:      0.01, // 1% error rate
		Health:         "healthy",
	}

	if err := ns.scalingCoord.ReportMetrics(metrics); err != nil {
		log.Debugf("Failed to report metrics: %v", err)
	}
}

// Close closes the streamer and cleans up resources
func (ns *NexusStreamer) Close() error {
	log.Info("Closing enhanced Nexus streamer")

	// Deregister service
	if ns.serviceRegistry != nil {
		if err := ns.serviceRegistry.Deregister(ns.ctx); err != nil {
			log.Errorf("Failed to deregister service: %v", err)
		}
	}

	// Stop scaling coordinator
	if ns.scalingCoord != nil {
		if err := ns.scalingCoord.Stop(); err != nil {
			log.Errorf("Failed to stop scaling coordinator: %v", err)
		}
	}

	// Close configuration manager
	if ns.configManager != nil {
		if err := ns.configManager.Close(); err != nil {
			log.Errorf("Failed to close config manager: %v", err)
		}
	}

	ns.cancel()

	if ns.etcdClient != nil {
		ns.etcdClient.Close()
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
