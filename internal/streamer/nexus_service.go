package streamer

import (
	"context"
	"crypto/md5"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dilipmighty245/telemetry-pipeline/internal/streaming"
	configpkg "github.com/dilipmighty245/telemetry-pipeline/pkg/config"
	"github.com/dilipmighty245/telemetry-pipeline/pkg/discovery"
	"github.com/dilipmighty245/telemetry-pipeline/pkg/logging"
	"github.com/dilipmighty245/telemetry-pipeline/pkg/scaling"
	"github.com/dilipmighty245/telemetry-pipeline/pkg/utils"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	clientv3 "go.etcd.io/etcd/client/v3"
	"golang.org/x/sync/errgroup"
)

// NexusStreamerConfig holds configuration for the Nexus streamer
type NexusStreamerConfig struct {
	// etcd configuration
	EtcdEndpoints []string
	ClusterID     string
	StreamerID    string

	// Message queue configuration
	MessageQueuePrefix string

	// Processing configuration
	BatchSize      int
	StreamInterval time.Duration

	// HTTP server configuration for CSV upload
	HTTPPort      int
	EnableHTTP    bool
	UploadDir     string
	MaxUploadSize int64
	MaxMemory     int64

	// Enhanced streaming features (always enabled for performance)
	StreamingConfig       *streaming.StreamAdapterConfig `json:"streaming_config"`
	StreamDestination     string                         `json:"stream_destination"`
	ParallelWorkers       int                            `json:"parallel_workers"`
	RateLimit             float64                        `json:"rate_limit"`
	BurstSize             int                            `json:"burst_size"`
	BackPressureThreshold float64                        `json:"back_pressure_threshold"`
	BackPressureDelay     time.Duration                  `json:"back_pressure_delay"`

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

// StreamerWorker handles parallel streaming
type StreamerWorker struct {
	id       int
	service  *NexusStreamerService
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	isActive bool
	mu       sync.RWMutex
}

// RateLimiter implements token bucket rate limiting
type RateLimiter struct {
	rate       float64
	burstSize  int
	tokens     float64
	lastUpdate time.Time
	mu         sync.Mutex
}

// NexusStreamerService streams telemetry data to etcd message queue
type NexusStreamerService struct {
	config     *NexusStreamerConfig
	etcdClient *clientv3.Client

	// Enhanced etcd features
	serviceRegistry *discovery.ServiceRegistry
	configManager   *configpkg.ConfigManager
	scalingCoord    *scaling.ScalingCoordinator

	// Enhanced streaming features
	streamAdapter *streaming.StreamAdapter
	rateLimiter   *RateLimiter
	workers       []*StreamerWorker
	workerPool    chan *StreamerWorker
	isEnhanced    bool

	// HTTP server for CSV upload
	echo *echo.Echo

	messageCount int64
	startTime    time.Time
}

// CSVUploadRequest represents a CSV file upload request
type CSVUploadRequest struct {
	File        multipart.File        `json:"-"`
	FileHeader  *multipart.FileHeader `json:"-"`
	Description string                `json:"description,omitempty"`
	Tags        []string              `json:"tags,omitempty"`
	Metadata    map[string]string     `json:"metadata,omitempty"`
}

// CSVUploadResponse represents the response after CSV upload
type CSVUploadResponse struct {
	Success     bool              `json:"success"`
	FileID      string            `json:"file_id"`
	Filename    string            `json:"filename"`
	Size        int64             `json:"size"`
	MD5Hash     string            `json:"md5_hash"`
	RecordCount int               `json:"record_count"`
	Headers     []string          `json:"headers"`
	UploadedAt  time.Time         `json:"uploaded_at"`
	ProcessedAt *time.Time        `json:"processed_at,omitempty"`
	Status      string            `json:"status"` // uploaded, processing, processed, error
	Error       string            `json:"error,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// Run is the main entry point for the streamer service
func (ns *NexusStreamerService) Run(ctx context.Context, args []string, _ io.Writer) error {
	cfg, err := ns.parseConfig(args)
	if err != nil {
		return fmt.Errorf("failed to parse configuration: %w", err)
	}

	// Set log level
	logging.SetLogLevel(cfg.LogLevel, "")

	logging.Infof("Starting Nexus telemetry streamer")
	logging.Infof("Cluster ID: %s, Streamer ID: %s", cfg.ClusterID, cfg.StreamerID)
	logging.Infof("HTTP Port: %d, Batch Size: %d, Interval: %v", cfg.HTTPPort, cfg.BatchSize, cfg.StreamInterval)

	// Update service config
	ns.config = cfg

	// Create and start the streamer
	streamer, err := NewNexusStreamerService(ctx, cfg)
	if err != nil {
		return fmt.Errorf("failed to create streamer: %w", err)
	}
	defer streamer.Close(ctx)

	// Start streaming
	if err := streamer.Start(ctx); err != nil {
		return fmt.Errorf("failed to start streamer: %w", err)
	}

	logging.Infof("Nexus Streamer started successfully")
	<-ctx.Done()
	logging.Infof("Shutting down Nexus Streamer...")
	streamer.PrintStats()

	return nil
}

// parseConfig parses command line flags and environment variables
func (ns *NexusStreamerService) parseConfig(args []string) (*NexusStreamerConfig, error) {
	cfg := &NexusStreamerConfig{}

	// Set defaults
	cfg.ClusterID = "default-cluster"
	cfg.StreamerID = fmt.Sprintf("streamer-%d", time.Now().Unix())
	cfg.HTTPPort = 8081
	cfg.BatchSize = 100
	cfg.StreamInterval = 3 * time.Second
	cfg.LogLevel = "info"

	// Parse command line arguments
	for _, arg := range args {
		if strings.HasPrefix(arg, "--port=") {
			portStr := strings.TrimPrefix(arg, "--port=")
			if p, err := strconv.Atoi(portStr); err == nil {
				cfg.HTTPPort = p
			}
		} else if strings.HasPrefix(arg, "--cluster-id=") {
			cfg.ClusterID = strings.TrimPrefix(arg, "--cluster-id=")
		} else if strings.HasPrefix(arg, "--streamer-id=") {
			cfg.StreamerID = strings.TrimPrefix(arg, "--streamer-id=")
		} else if strings.HasPrefix(arg, "--log-level=") {
			cfg.LogLevel = strings.TrimPrefix(arg, "--log-level=")
		} else if strings.HasPrefix(arg, "--batch-size=") {
			batchStr := strings.TrimPrefix(arg, "--batch-size=")
			if b, err := strconv.Atoi(batchStr); err == nil {
				cfg.BatchSize = b
			}
		}
	}

	// etcd configuration
	cfg.ClusterID = getEnv("CLUSTER_ID", cfg.ClusterID)
	cfg.StreamerID = getEnv("STREAMER_ID", cfg.StreamerID)

	etcdEndpointsStr := getEnv("ETCD_ENDPOINTS", "localhost:2379")
	cfg.EtcdEndpoints = strings.Split(etcdEndpointsStr, ",")

	// Message queue configuration
	cfg.MessageQueuePrefix = getEnv("MESSAGE_QUEUE_PREFIX", "/telemetry/queue")

	// Processing configuration (environment variables override command line)
	cfg.BatchSize = getEnvInt("BATCH_SIZE", cfg.BatchSize)
	cfg.StreamInterval = getEnvDuration("STREAM_INTERVAL", cfg.StreamInterval)

	// HTTP server configuration for CSV upload (environment variables override command line)
	cfg.HTTPPort = getEnvInt("HTTP_PORT", cfg.HTTPPort)
	cfg.EnableHTTP = true // Always enable HTTP server in API-only mode
	cfg.UploadDir = getEnv("UPLOAD_DIR", "/tmp/telemetry-uploads")
	cfg.MaxUploadSize = int64(getEnvInt("MAX_UPLOAD_SIZE", 100*1024*1024)) // 100MB
	cfg.MaxMemory = int64(getEnvInt("MAX_MEMORY", 32*1024*1024))           // 32MB

	// Logging (environment variables override command line)
	cfg.LogLevel = getEnv("LOG_LEVEL", cfg.LogLevel)

	return cfg, nil
}

// NewNexusStreamerService creates a new etcd-based streamer service
func NewNexusStreamerService(ctx context.Context, config *NexusStreamerConfig) (*NexusStreamerService, error) {
	// Set defaults for enhanced features
	if config.StreamingConfig == nil {
		config.StreamingConfig = &streaming.StreamAdapterConfig{
			ChannelSize:   2000, // Higher for streaming
			BatchSize:     1000, // Larger batches for streaming
			Workers:       8,    // More workers for streaming
			MaxRetries:    3,
			RetryDelay:    time.Second,
			FlushInterval: 2 * time.Second, // Faster flush for streaming
			HTTPTimeout:   30 * time.Second,
			EnableMetrics: true,
			PartitionBy:   "hostname",
		}
	}

	if config.ParallelWorkers <= 0 {
		config.ParallelWorkers = 5
	}

	if config.RateLimit <= 0 {
		config.RateLimit = 1000 // 1000 records per second default
	}

	if config.BurstSize <= 0 {
		config.BurstSize = 100
	}

	if config.BackPressureThreshold <= 0 {
		config.BackPressureThreshold = 80.0 // 80% channel utilization
	}

	if config.BackPressureDelay <= 0 {
		config.BackPressureDelay = 100 * time.Millisecond
	}

	// Create etcd client
	client, err := clientv3.New(clientv3.Config{
		Endpoints:   config.EtcdEndpoints,
		DialTimeout: 10 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create etcd client: %w", err)
	}
	etcdClient := client

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

	// Create HTTP server (always enabled in API-only mode)
	echoServer := echo.New()
	echoServer.HideBanner = true

	streamer := &NexusStreamerService{
		config:          config,
		etcdClient:      etcdClient,
		serviceRegistry: serviceRegistry,
		configManager:   configManager,
		scalingCoord:    scalingCoord,
		echo:            echoServer,
		startTime:       time.Now(),
		isEnhanced:      true,
	}

	// Initialize streaming adapter (always enabled for performance)
	streamer.streamAdapter = streaming.NewStreamAdapter(ctx, config.StreamingConfig, config.StreamDestination)

	// Initialize rate limiter (always enabled for performance)
	streamer.rateLimiter = &RateLimiter{
		rate:       config.RateLimit,
		burstSize:  config.BurstSize,
		tokens:     float64(config.BurstSize),
		lastUpdate: time.Now(),
	}

	// Initialize worker pool (always enabled for parallel streaming performance)
	streamer.workerPool = make(chan *StreamerWorker, config.ParallelWorkers)
	streamer.workers = make([]*StreamerWorker, config.ParallelWorkers)

	for i := 0; i < config.ParallelWorkers; i++ {
		childCtx, cancel := context.WithCancel(ctx)
		worker := &StreamerWorker{
			id:      i,
			service: streamer,
			ctx:     childCtx,
			cancel:  cancel,
		}
		streamer.workers[i] = worker
		streamer.workerPool <- worker
	}

	logging.Infof("Created enhanced streamer service with parallel=true, rate_limit=true, back_pressure=true")

	// Setup HTTP routes
	streamer.setupHTTPRoutes(ctx)

	return streamer, nil
}

// Start starts the streaming process
func (ns *NexusStreamerService) Start(ctx context.Context) error {
	logging.Infof("Starting enhanced etcd-based telemetry streaming")

	// Start streaming adapter
	err := ns.streamAdapter.Start(ctx)
	if err != nil {
		logging.Errorf("Failed to start stream adapter: %v", err)
		return err
	}

	// Start worker pool (parallel streaming always enabled)
	for _, worker := range ns.workers {
		go worker.start()
	}

	g, gCtx := errgroup.WithContext(ctx)

	// Load initial configuration
	if err := ns.configManager.LoadInitialConfig(); err != nil {
		logging.Warnf("Failed to load initial config: %v", err)
	}

	// Set default configuration values
	defaults := map[string]interface{}{
		"streamer/batch-size":      ns.config.BatchSize,
		"streamer/stream-interval": ns.config.StreamInterval.String(),
		"streamer/rate-limit":      1000.0,
		"streamer/enabled":         true,
	}
	if err := ns.configManager.SetDefaults(defaults); err != nil {
		logging.Warnf("Failed to set default config: %v", err)
	}

	// Register service with dynamically determined address
	serviceAddress := utils.GetServiceAddress()
	logging.Infof("Service will register with address: %s:%d", serviceAddress, ns.config.HTTPPort)
	serviceInfo := discovery.ServiceInfo{
		ID:      ns.config.StreamerID,
		Type:    "nexus-streamer",
		Address: serviceAddress,
		Port:    ns.config.HTTPPort, // Use the actual configured HTTP port
		Metadata: map[string]string{
			"cluster_id": ns.config.ClusterID,
			"mode":       "api-only",
			"version":    "2.0.0",
			"endpoint":   utils.FormatServiceEndpoint(serviceAddress, ns.config.HTTPPort),
		},
		Health:  "healthy",
		Version: "2.0.0",
	}

	if err := ns.serviceRegistry.Register(gCtx, serviceInfo); err != nil {
		return err
	}

	logging.Infof("Service registered: %s", ns.config.StreamerID)

	// Start scaling coordinator
	if err := ns.scalingCoord.Start(ctx); err != nil {
		logging.Errorf("Failed to start scaling coordinator: %v", err)
	}

	// Start configuration watcher
	g.Go(func() error {
		return ns.watchConfiguration(gCtx)
	})

	// Start HTTP server (always enabled in API-only mode)
	g.Go(func() error {
		return ns.startHTTPServer(gCtx)
	})

	// Wait for all goroutines to complete
	err = g.Wait()
	if err != nil && err != context.Canceled {
		return err
	}

	// Context cancellation is expected during graceful shutdown
	return nil
}

// parseCSVRecord parses a CSV record into a TelemetryRecord
func (ns *NexusStreamerService) parseCSVRecord(record []string, columnMap map[string]int) (*TelemetryRecord, error) {
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

	// add timestamp at the time of processing
	tr.Timestamp = time.Now().Format(time.RFC3339)

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
	logging.Debugf("Parsed record: GPUID=%s, UUID=%s, Device=%s, ModelName=%s, Hostname=%s",
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

// publishBatch publishes a batch of telemetry records with enhanced streaming capabilities
func (ns *NexusStreamerService) publishBatch(ctx context.Context, batch []*TelemetryRecord) error {
	if len(batch) == 0 {
		return nil
	}

	// Check back pressure (always enabled for performance)
	metrics := ns.streamAdapter.GetMetrics()
	if metrics.ChannelUtilization > ns.config.BackPressureThreshold {
		logging.Warnf("Back pressure detected (%.1f%% utilization), applying delay", metrics.ChannelUtilization)
		time.Sleep(ns.config.BackPressureDelay)
	}

	// Use worker pool (parallel streaming always enabled)
	return ns.publishBatchWithWorkers(ctx, batch)
}

// publishBatchWithWorkers distributes work across worker pool
func (ns *NexusStreamerService) publishBatchWithWorkers(ctx context.Context, batch []*TelemetryRecord) error {
	select {
	case worker := <-ns.workerPool:
		go func() {
			defer func() {
				ns.workerPool <- worker // Return worker to pool
			}()

			// Use background context for worker operations to avoid cancellation
			// when the original request context is canceled

			err := worker.publishBatch(ctx, batch)
			if err != nil {
				logging.Errorf("Worker %d publishing failed: %v", worker.id, err)
			}
		}()
		return nil
	default:
		// No workers available, fallback to direct processing
		// Use background context to avoid cancellation issues
		return ns.publishBatchEnhanced(ctx, batch)
	}
}

// publishBatchEnhanced performs enhanced publishing with streaming and rate limiting
func (ns *NexusStreamerService) publishBatchEnhanced(ctx context.Context, batch []*TelemetryRecord) error {
	// Apply rate limiting
	if ns.rateLimiter != nil {
		for range batch {
			if !ns.rateLimiter.allow() {
				// Rate limit exceeded, wait
				time.Sleep(10 * time.Millisecond)
			}
		}
	}

	// Stream data using streaming adapter
	for _, record := range batch {
		headers := map[string]string{
			"streamer_id": ns.config.StreamerID,
			"gpu_id":      record.GPUID,
			"hostname":    record.Hostname,
			"timestamp":   record.Timestamp,
			"metric_name": "telemetry",
		}

		// Convert to interface{} for streaming
		telemetryData := map[string]interface{}{
			"timestamp":          record.Timestamp,
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

		// For now, simulate streaming until proper integration
		logging.Debugf("Would stream telemetry data: %+v with headers: %+v", telemetryData, headers)
		// err := ns.streamAdapter.WriteTelemetry(telemetryData, headers)
		// Handle streaming error if needed
	}

	// Fallback to etcd message queue publishing
	queueKey := ns.config.MessageQueuePrefix + "/telemetry"
	logging.Debugf("Publishing batch of %d records to etcd queue: %s", len(batch), queueKey)

	// Use etcd transaction for atomic batch publishing
	ops := make([]clientv3.Op, 0, len(batch))

	for i, record := range batch {
		// Create unique key for each message
		messageKey := fmt.Sprintf("%s/%d_%s_%s_%d",
			queueKey,
			time.Now().UnixNano(),
			record.Hostname,
			record.GPUID,
			ns.messageCount)

		// Log key generation for debugging (first and last record only to avoid spam)
		if i == 0 || i == len(batch)-1 {
			logging.Debugf("Generated etcd key for record %d/%d: %s", i+1, len(batch), messageKey)
		}

		// Serialize record
		data, err := json.Marshal(record)
		if err != nil {
			logging.Errorf("Failed to marshal telemetry record: %v", err)
			continue
		}

		ops = append(ops, clientv3.OpPut(messageKey, string(data)))
		ns.messageCount++
	}

	if len(ops) == 0 {
		return fmt.Errorf("no valid records in batch")
	}

	// Execute transaction
	txnCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Check if context is already canceled before attempting transaction
	if ctx.Err() != nil {
		return fmt.Errorf("context canceled before etcd transaction: %w", ctx.Err())
	}

	logging.Debugf("Executing etcd transaction with %d operations", len(ops))
	txnResp, err := ns.etcdClient.Txn(txnCtx).Then(ops...).Commit()
	if err != nil {
		return fmt.Errorf("failed to publish batch to etcd: %w", err)
	}

	if !txnResp.Succeeded {
		return fmt.Errorf("etcd transaction failed")
	}

	logging.Infof("Published batch of %d telemetry records to etcd queue (revision: %d)", len(batch), txnResp.Header.Revision)
	return nil
}

// PrintStats prints streaming statistics
func (ns *NexusStreamerService) PrintStats() {
	duration := time.Since(ns.startTime)
	rate := float64(ns.messageCount) / duration.Seconds()

	logging.Infof("Streaming Statistics:")
	logging.Infof("  Total Messages: %d", ns.messageCount)
	logging.Infof("  Duration: %v", duration)
	logging.Infof("  Rate: %.2f messages/second", rate)
}

// watchConfiguration watches for configuration changes
func (ns *NexusStreamerService) watchConfiguration(ctx context.Context) error {
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
				logging.Infof("Updating batch size from %d to %d", ns.config.BatchSize, newBatchSize)
				ns.config.BatchSize = newBatchSize
			}

		case <-intervalChan:
			if intervalStr := ns.configManager.GetString("streamer/stream-interval", ns.config.StreamInterval.String()); intervalStr != ns.config.StreamInterval.String() {
				if newInterval, err := time.ParseDuration(intervalStr); err == nil {
					logging.Infof("Updating stream interval from %v to %v", ns.config.StreamInterval, newInterval)
					ns.config.StreamInterval = newInterval
				}
			}

		case <-rateLimitChan:
			rateLimit := ns.configManager.GetFloat("streamer/rate-limit", 1000.0)
			logging.Infof("Rate limit updated to %.2f messages/sec", rateLimit)

		case <-ctx.Done():
			logging.Infof("Stopping configuration watcher")
			return ctx.Err()
		}
	}
}

// Close closes the streamer and cleans up resources
func (ns *NexusStreamerService) Close(ctx context.Context) error {
	logging.Infof("Closing enhanced Nexus streamer")

	// Stop enhanced features
	if err := ns.Stop(); err != nil {
		logging.Warnf("Error stopping enhanced features during close: %v", err)
	}

	// Deregister service
	if ns.serviceRegistry != nil {
		// Use background context for cleanup with shorter timeout to avoid hanging
		cleanupCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		if err := ns.serviceRegistry.Deregister(cleanupCtx); err != nil {
			// Log as warning instead of error since service shutdown should not fail due to deregistration issues
			logging.Warnf("Failed to deregister service (this is expected if etcd is unavailable): %v", err)
		} else {
			logging.Infof("Service deregistered successfully")
		}
	}

	// Stop scaling coordinator
	if ns.scalingCoord != nil {
		if err := ns.scalingCoord.Stop(ctx); err != nil {
			logging.Warnf("Failed to stop scaling coordinator: %v", err)
		} else {
			logging.Infof("Scaling coordinator stopped successfully")
		}
	}

	// Close configuration manager
	if ns.configManager != nil {
		if err := ns.configManager.Close(); err != nil {
			logging.Warnf("Failed to close config manager: %v", err)
		} else {
			logging.Infof("Configuration manager closed successfully")
		}
	}

	if ns.etcdClient != nil {
		ns.etcdClient.Close()
		logging.Infof("etcd client closed")
	}

	logging.Infof("Nexus streamer closed successfully")
	return nil
}

// Stop stops enhanced streaming features
func (ns *NexusStreamerService) Stop() error {
	// Stop streaming adapter
	err := ns.streamAdapter.Stop()
	if err != nil {
		logging.Errorf("Failed to stop stream adapter: %v", err)
	}

	// Stop workers (parallel streaming always enabled)
	for _, worker := range ns.workers {
		worker.stop()
	}

	return nil
}

// StreamerWorker methods

func (sw *StreamerWorker) start() {
	sw.mu.Lock()
	sw.isActive = true
	sw.mu.Unlock()

	logging.Infof("Started streamer worker %d", sw.id)
}

func (sw *StreamerWorker) stop() {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	if !sw.isActive {
		return
	}

	sw.isActive = false
	sw.cancel()
	sw.wg.Wait()

	logging.Infof("Stopped streamer worker %d", sw.id)
}

func (sw *StreamerWorker) publishBatch(ctx context.Context, batch []*TelemetryRecord) error {
	// Delegate to enhanced service's publishing logic
	return sw.service.publishBatchEnhanced(ctx, batch)
}

// RateLimiter methods (Token bucket algorithm)

func (rl *RateLimiter) allow() bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(rl.lastUpdate).Seconds()
	rl.lastUpdate = now

	// Add tokens based on elapsed time
	rl.tokens += elapsed * rl.rate
	if rl.tokens > float64(rl.burstSize) {
		rl.tokens = float64(rl.burstSize)
	}

	// Check if we have tokens available
	if rl.tokens >= 1.0 {
		rl.tokens -= 1.0
		return true
	}

	return false
}

// GetEnhancedMetrics returns comprehensive metrics including streaming
func (ns *NexusStreamerService) GetEnhancedMetrics() map[string]interface{} {
	enhanced := map[string]interface{}{
		"is_enhanced":   ns.isEnhanced,
		"message_count": ns.messageCount,
		"uptime":        time.Since(ns.startTime).String(),
		"streamer_id":   ns.config.StreamerID,
		"cluster_id":    ns.config.ClusterID,
	}

	enhanced["streaming_metrics"] = ns.streamAdapter.GetMetrics()

	if ns.rateLimiter != nil {
		ns.rateLimiter.mu.Lock()
		enhanced["rate_limiter"] = map[string]interface{}{
			"rate":       ns.rateLimiter.rate,
			"burst_size": ns.rateLimiter.burstSize,
			"tokens":     ns.rateLimiter.tokens,
		}
		ns.rateLimiter.mu.Unlock()
	}

	// Parallel streaming is always enabled
	activeWorkers := 0
	for _, worker := range ns.workers {
		worker.mu.RLock()
		if worker.isActive {
			activeWorkers++
		}
		worker.mu.RUnlock()
	}
	enhanced["worker_pool"] = map[string]interface{}{
		"total_workers":     len(ns.workers),
		"active_workers":    activeWorkers,
		"available_workers": len(ns.workerPool),
	}

	return enhanced
}

// StreamDirectly streams data directly without going through CSV
func (ns *NexusStreamerService) StreamDirectly(data []*TelemetryRecord) error {
	// Rate limiting is always enabled
	for range data {
		if !ns.rateLimiter.allow() {
			time.Sleep(10 * time.Millisecond)
		}
	}

	// Stream data using streaming adapter
	for _, record := range data {
		headers := map[string]string{
			"streamer_id":   ns.config.StreamerID,
			"gpu_id":        record.GPUID,
			"hostname":      record.Hostname,
			"timestamp":     record.Timestamp,
			"direct_stream": "true",
		}

		// Convert to interface{} for streaming
		telemetryData := map[string]interface{}{
			"timestamp":          record.Timestamp,
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

		// For now, simulate streaming until proper integration
		logging.Debugf("Would directly stream telemetry data: %+v with headers: %+v", telemetryData, headers)
		// err := ns.streamAdapter.WriteTelemetry(telemetryData, headers)
		// Handle streaming error if needed
	}

	logging.Debugf("Direct streamed %d records", len(data))
	return nil
}

// setupHTTPRoutes configures HTTP routes for CSV upload
func (ns *NexusStreamerService) setupHTTPRoutes(ctx context.Context) {
	// Middleware
	ns.echo.Use(middleware.Logger())
	ns.echo.Use(middleware.Recover())
	ns.echo.Use(middleware.CORS())

	// Health check
	ns.echo.GET("/health", ns.healthHandler)

	// CSV upload endpoint
	ns.echo.POST("/api/v1/csv/upload", ns.uploadCSVHandlerWithContext(ctx))

	// Status endpoint
	ns.echo.GET("/api/v1/status", ns.statusHandler)
}

// startHTTPServer starts the HTTP server for CSV upload
func (ns *NexusStreamerService) startHTTPServer(ctx context.Context) error {
	addr := fmt.Sprintf(":%d", ns.config.HTTPPort)
	logging.Infof("Starting HTTP server for CSV upload on %s", addr)

	server := &http.Server{
		Addr:         addr,
		Handler:      ns.echo,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	// Start server in goroutine
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logging.Errorf("HTTP server error: %v", err)
		}
	}()

	// Wait for context cancellation
	<-ctx.Done()
	logging.Infof("Attempting graceful shutdown of HTTP server")

	// Shutdown server with fresh context since ctx is already canceled
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		logging.Errorf("HTTP server shutdown error: %v", err)
		return err
	}

	logging.Infof("HTTP server shutdown completed successfully")
	return nil
}

// healthHandler handles health check requests
func (ns *NexusStreamerService) healthHandler(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]interface{}{
		"status":      "healthy",
		"service":     "nexus-streamer",
		"streamer_id": ns.config.StreamerID,
		"cluster_id":  ns.config.ClusterID,
		"timestamp":   time.Now().UTC(),
		"uptime":      time.Since(ns.startTime).String(),
	})
}

// statusHandler handles status requests
func (ns *NexusStreamerService) statusHandler(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]interface{}{
		"service":       "nexus-streamer",
		"streamer_id":   ns.config.StreamerID,
		"cluster_id":    ns.config.ClusterID,
		"message_count": ns.messageCount,
		"uptime":        time.Since(ns.startTime).String(),
		"config": map[string]interface{}{
			"batch_size":      ns.config.BatchSize,
			"stream_interval": ns.config.StreamInterval.String(),
			"mode":            "api-only",
			"http_port":       ns.config.HTTPPort,
		},
	})
}

// uploadCSVHandlerWithContext returns a handler function with the provided context
func (ns *NexusStreamerService) uploadCSVHandlerWithContext(ctx context.Context) echo.HandlerFunc {
	return func(c echo.Context) error {
		return ns.uploadCSVHandler(ctx, c)
	}
}

// uploadCSVHandler handles CSV file upload and immediate processing
func (ns *NexusStreamerService) uploadCSVHandler(ctx context.Context, c echo.Context) error {
	startTime := time.Now()

	// Parse multipart form with size limit
	if err := c.Request().ParseMultipartForm(ns.config.MaxMemory); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"error":   "Failed to parse multipart form",
		})
	}

	// Get file from form
	file, fileHeader, err := c.Request().FormFile("file")
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"error":   "No file provided or invalid file",
		})
	}
	defer file.Close()

	// Validate file size
	if fileHeader.Size > ns.config.MaxUploadSize {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"error":   fmt.Sprintf("File size exceeds maximum allowed size of %d bytes", ns.config.MaxUploadSize),
		})
	}

	// Validate file extension
	if !strings.HasSuffix(strings.ToLower(fileHeader.Filename), ".csv") {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"error":   "Only .csv files are allowed",
		})
	}

	logging.Infof("Starting CSV upload and processing: %s (Size: %d bytes, MD5 will be calculated)", fileHeader.Filename, fileHeader.Size)

	// Create upload directory if it doesn't exist
	if err := os.MkdirAll(ns.config.UploadDir, 0755); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"error":   "Failed to create upload directory",
		})
	}

	// Generate unique file ID and path
	fileID := fmt.Sprintf("%d_%s", time.Now().UnixNano(), strings.ReplaceAll(fileHeader.Filename, " ", "_"))
	filePath := filepath.Join(ns.config.UploadDir, fileID)

	// Save file to disk
	dst, err := os.Create(filePath)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"error":   "Failed to create destination file",
		})
	}
	defer dst.Close()

	// Calculate MD5 hash while copying
	hasher := md5.New()
	if _, err := io.Copy(dst, io.TeeReader(file, hasher)); err != nil {
		os.Remove(filePath) // Clean up on error
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"error":   "Failed to save file",
		})
	}

	md5Hash := fmt.Sprintf("%x", hasher.Sum(nil))
	logging.Infof("File saved successfully with MD5: %s, starting CSV processing", md5Hash)

	// Process CSV file immediately
	// Use the provided context for CSV processing
	processedRecords, totalRecords, headers, err := ns.processUploadedCSV(ctx, filePath)
	if err != nil {
		os.Remove(filePath) // Clean up on error
		logging.Errorf("CSV processing failed: %v", err)
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"error":   fmt.Sprintf("CSV processing failed: %v", err),
		})
	}

	duration := time.Since(startTime)
	processedAt := time.Now()

	logging.Infof("CSV processing completed: %s (Records: %d/%d processed, Duration: %v)",
		fileHeader.Filename, processedRecords, totalRecords, duration)

	// Clean up uploaded file after processing (for production)
	if err := os.Remove(filePath); err != nil {
		logging.Warnf("Failed to clean up uploaded file: %v", err)
	}

	// Return success response
	response := &CSVUploadResponse{
		Success:     true,
		FileID:      fileID,
		Filename:    fileHeader.Filename,
		Size:        fileHeader.Size,
		MD5Hash:     md5Hash,
		RecordCount: processedRecords,
		Headers:     headers,
		UploadedAt:  startTime,
		ProcessedAt: &processedAt,
		Status:      "processed",
		Metadata: map[string]string{
			"processing_time":    duration.String(),
			"records_per_second": fmt.Sprintf("%.2f", float64(processedRecords)/duration.Seconds()),
			"total_records":      fmt.Sprintf("%d", totalRecords),
			"skipped_records":    fmt.Sprintf("%d", totalRecords-processedRecords),
		},
	}

	return c.JSON(http.StatusOK, response)
}

// processUploadedCSV processes an uploaded CSV file and streams it to the message queue
func (ns *NexusStreamerService) processUploadedCSV(ctx context.Context, filePath string) (int, int, []string, error) {
	logging.Infof("Starting to process CSV file: %s", filePath)

	file, err := os.Open(filePath)
	if err != nil {
		return 0, 0, nil, fmt.Errorf("failed to open CSV file: %w", err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.FieldsPerRecord = -1 // Allow variable number of fields

	// Read header
	headers, err := reader.Read()
	if err != nil {
		return 0, 0, nil, fmt.Errorf("failed to read CSV header: %w", err)
	}

	if len(headers) == 0 {
		return 0, 0, nil, fmt.Errorf("CSV file has no headers")
	}

	// Validate required headers for telemetry data
	requiredHeaders := []string{"timestamp", "gpu_id", "hostname"}
	headerMap := make(map[string]int)
	for i, header := range headers {
		headerMap[strings.ToLower(strings.TrimSpace(header))] = i
	}

	var missingHeaders []string
	for _, required := range requiredHeaders {
		if _, exists := headerMap[required]; !exists {
			missingHeaders = append(missingHeaders, required)
		}
	}

	if len(missingHeaders) > 0 {
		return 0, 0, headers, fmt.Errorf("missing required headers: %v", missingHeaders)
	}

	logging.Infof("CSV validation successful: %d headers found, starting record processing", len(headers))

	// Process records in batches
	batch := make([]*TelemetryRecord, 0, ns.config.BatchSize)
	totalRecords := 0
	processedRecords := 0
	batchCount := 0

	for {
		record, err := reader.Read()
		if err == io.EOF {
			// Process final batch
			if len(batch) > 0 {
				batchCount++
				logging.Infof("Processing final batch %d with %d records", batchCount, len(batch))
				if err := ns.publishBatch(ctx, batch); err != nil {
					logging.Warnf("Failed to process final batch %d: %v", batchCount, err)
				} else {
					processedRecords += len(batch)
					logging.Infof("Successfully processed final batch %d (%d records)", batchCount, len(batch))
				}
			}
			break
		}
		if err != nil {
			logging.Warnf("Failed to read CSV record at line %d: %v", totalRecords+2, err)
			totalRecords++
			continue
		}

		// Parse record
		telemetryRecord, err := ns.parseCSVRecord(record, headerMap)
		if err != nil {
			logging.Warnf("Failed to parse CSV record: %v", err)
			totalRecords++
			continue
		}

		batch = append(batch, telemetryRecord)
		totalRecords++

		if len(batch) >= ns.config.BatchSize {
			batchCount++
			logging.Infof("Processing batch %d with %d records", batchCount, len(batch))
			if err := ns.publishBatch(ctx, batch); err != nil {
				logging.Warnf("Failed to process batch %d: %v", batchCount, err)
			} else {
				processedRecords += len(batch)
				logging.Infof("Successfully processed batch %d (%d records)", batchCount, len(batch))
			}
			batch = batch[:0] // Reset batch
		}
	}

	logging.Infof("CSV processing completed: %d/%d records processed successfully in %d batches",
		processedRecords, totalRecords, batchCount)
	return processedRecords, totalRecords, headers, nil
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
