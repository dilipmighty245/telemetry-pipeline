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

	messageCount int64
	startTime    time.Time
}

func main() {
	config := parseFlags()

	// Set log level
	level, err := log.ParseLevel(config.LogLevel)
	if err != nil {
		log.Fatalf("Invalid log level: %v", err)
	}
	log.SetLevel(level)

	log.Infof("Starting Nexus telemetry streamer")
	log.Infof("Cluster ID: %s, Streamer ID: %s", config.ClusterID, config.StreamerID)
	log.Infof("CSV File: %s, Batch Size: %d, Interval: %v", config.CSVFile, config.BatchSize, config.StreamInterval)

	// Create and start the streamer
	streamer, err := NewNexusStreamer(config)
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
	config := &StreamerConfig{}

	// etcd configuration
	flag.StringVar(&config.ClusterID, "cluster-id", getEnv("CLUSTER_ID", "default-cluster"), "Telemetry cluster ID")
	flag.StringVar(&config.StreamerID, "streamer-id", getEnv("STREAMER_ID", fmt.Sprintf("streamer-%d", time.Now().Unix())), "Unique streamer ID")

	etcdEndpointsStr := getEnv("ETCD_ENDPOINTS", "localhost:2379")
	config.EtcdEndpoints = strings.Split(etcdEndpointsStr, ",")

	// Message queue configuration
	flag.StringVar(&config.MessageQueuePrefix, "message-queue-prefix", getEnv("MESSAGE_QUEUE_PREFIX", "/telemetry/queue"), "etcd message queue prefix")

	// CSV configuration
	flag.StringVar(&config.CSVFile, "csv", getEnv("CSV_FILE", "dcgm_metrics_20250718_134233.csv"), "CSV file to stream")
	flag.IntVar(&config.BatchSize, "batch-size", getEnvInt("BATCH_SIZE", 100), "Batch size for streaming")
	flag.DurationVar(&config.StreamInterval, "stream-interval", getEnvDuration("STREAM_INTERVAL", 3*time.Second), "Interval between batches")
	flag.BoolVar(&config.LoopMode, "loop", getEnvBool("LOOP_MODE", false), "Loop through CSV file continuously")

	// Logging
	flag.StringVar(&config.LogLevel, "log-level", getEnv("LOG_LEVEL", "info"), "Log level (debug, info, warn, error)")

	flag.Parse()
	return config
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

	streamer := &NexusStreamer{
		config:     config,
		etcdClient: etcdClient,
		ctx:        ctx,
		cancel:     cancel,
		startTime:  time.Now(),
	}

	return streamer, nil
}

// Start starts the streaming process
func (ns *NexusStreamer) Start() error {
	log.Info("Starting etcd-based telemetry streaming")

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

// Close closes the streamer and cleans up resources
func (ns *NexusStreamer) Close() error {
	log.Info("Closing Nexus streamer")

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
