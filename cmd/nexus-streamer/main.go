package main

import (
	"context"
	"crypto/md5"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	configpkg "github.com/dilipmighty245/telemetry-pipeline/pkg/config"
	"github.com/dilipmighty245/telemetry-pipeline/pkg/discovery"
	"github.com/dilipmighty245/telemetry-pipeline/pkg/scaling"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	log "github.com/sirupsen/logrus"
	clientv3 "go.etcd.io/etcd/client/v3"
	"golang.org/x/sync/errgroup"
)

// StreamerConfig holds configuration for the Nexus streamer
type StreamerConfig struct {
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

	// Enhanced etcd features
	serviceRegistry *discovery.ServiceRegistry
	configManager   *configpkg.ConfigManager
	scalingCoord    *scaling.ScalingCoordinator

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

func main() {
	if err := run(os.Args, os.Stdout); err != nil {
		switch err {
		case context.Canceled:
			// not considered error
		default:
			log.Fatalf("could not run Nexus Streamer: %v", err)
		}
	}
}

// run accepts the program arguments and where to send output (default: stdout)
func run(args []string, _ io.Writer) error {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	cfg, err := parseConfig(args)
	if err != nil {
		return fmt.Errorf("failed to parse configuration: %w", err)
	}

	// Set log level
	level, err := log.ParseLevel(cfg.LogLevel)
	if err != nil {
		return fmt.Errorf("invalid log level: %w", err)
	}
	log.SetLevel(level)

	log.Infof("Starting Nexus telemetry streamer (API-only mode)")
	log.Infof("Cluster ID: %s, Streamer ID: %s", cfg.ClusterID, cfg.StreamerID)
	log.Infof("HTTP Port: %d, Batch Size: %d, Interval: %v", cfg.HTTPPort, cfg.BatchSize, cfg.StreamInterval)

	// Create and start the streamer
	streamer, err := NewNexusStreamer(ctx, cfg)
	if err != nil {
		return fmt.Errorf("failed to create streamer: %w", err)
	}
	defer streamer.Close()

	// Start streaming
	if err := streamer.Start(ctx); err != nil {
		return fmt.Errorf("failed to start streamer: %w", err)
	}

	log.Info("Nexus Streamer started successfully")
	<-ctx.Done()
	log.Info("Shutting down Nexus Streamer...")
	streamer.PrintStats()

	return nil
}

// parseConfig parses command line flags and environment variables
func parseConfig(args []string) (*StreamerConfig, error) {
	cfg := &StreamerConfig{}

	// etcd configuration
	flag.StringVar(&cfg.ClusterID, "cluster-id", getEnv("CLUSTER_ID", "default-cluster"), "Telemetry cluster ID")
	flag.StringVar(&cfg.StreamerID, "streamer-id", getEnv("STREAMER_ID", fmt.Sprintf("streamer-%d", time.Now().Unix())), "Unique streamer ID")

	etcdEndpointsStr := getEnv("ETCD_ENDPOINTS", "localhost:2379")
	cfg.EtcdEndpoints = strings.Split(etcdEndpointsStr, ",")

	// Message queue configuration
	flag.StringVar(&cfg.MessageQueuePrefix, "message-queue-prefix", getEnv("MESSAGE_QUEUE_PREFIX", "/telemetry/queue"), "etcd message queue prefix")

	// Processing configuration
	flag.IntVar(&cfg.BatchSize, "batch-size", getEnvInt("BATCH_SIZE", 100), "Batch size for streaming")
	flag.DurationVar(&cfg.StreamInterval, "stream-interval", getEnvDuration("STREAM_INTERVAL", 3*time.Second), "Interval between batches")

	// HTTP server configuration for CSV upload
	flag.IntVar(&cfg.HTTPPort, "http-port", getEnvInt("HTTP_PORT", 8081), "HTTP server port for CSV upload")
	cfg.EnableHTTP = true // Always enable HTTP server in API-only mode
	flag.StringVar(&cfg.UploadDir, "upload-dir", getEnv("UPLOAD_DIR", "/tmp/telemetry-uploads"), "Directory for uploaded CSV files")
	cfg.MaxUploadSize = int64(getEnvInt("MAX_UPLOAD_SIZE", 100*1024*1024)) // 100MB
	cfg.MaxMemory = int64(getEnvInt("MAX_MEMORY", 32*1024*1024))           // 32MB

	// Logging
	flag.StringVar(&cfg.LogLevel, "log-level", getEnv("LOG_LEVEL", "info"), "Log level (debug, info, warn, error)")

	// Parse flags after defining them
	if err := flag.CommandLine.Parse(args[1:]); err != nil {
		return nil, fmt.Errorf("failed to parse flags: %w", err)
	}

	return cfg, nil
}

// NewNexusStreamer creates a new etcd-based streamer
func NewNexusStreamer(ctx context.Context, config *StreamerConfig) (*NexusStreamer, error) {

	// Create etcd client
	etcdClient, err := clientv3.New(clientv3.Config{
		Endpoints:   config.EtcdEndpoints,
		DialTimeout: 10 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create etcd client: %w", err)
	}

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

	streamer := &NexusStreamer{
		config:          config,
		etcdClient:      etcdClient,
		serviceRegistry: serviceRegistry,
		configManager:   configManager,
		scalingCoord:    scalingCoord,
		echo:            echoServer,
		startTime:       time.Now(),
	}

	// Setup HTTP routes (always enabled in API-only mode)
	streamer.setupHTTPRoutes()

	return streamer, nil
}

// Start starts the streaming process
func (ns *NexusStreamer) Start(ctx context.Context) error {
	log.Info("Starting enhanced etcd-based telemetry streaming")

	g, gCtx := errgroup.WithContext(ctx)

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
			"mode":       "api-only",
			"version":    "2.0.0",
		},
		Health:  "healthy",
		Version: "2.0.0",
	}

	if err := ns.serviceRegistry.Register(gCtx, serviceInfo); err != nil {
		log.Errorf("Failed to register service: %v", err)
	} else {
		log.Infof("Service registered: %s", ns.config.StreamerID)
	}

	// Start scaling coordinator
	if err := ns.scalingCoord.Start(); err != nil {
		log.Errorf("Failed to start scaling coordinator: %v", err)
	}

	// Start configuration watcher
	g.Go(func() error {
		return ns.watchConfiguration(gCtx)
	})

	// Start HTTP server (always enabled in API-only mode)
	g.Go(func() error {
		return ns.startHTTPServer(gCtx)
	})

	// In API-only mode, we don't run the streaming loop
	// CSV processing happens only when files are uploaded via the API

	return g.Wait()
}

// Note: streamingLoop and streamCSVFile functions removed in API-only mode
// CSV processing now happens only through the upload API endpoint

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
func (ns *NexusStreamer) publishBatch(ctx context.Context, batch []*TelemetryRecord) error {
	if len(batch) == 0 {
		return nil
	}

	queueKey := ns.config.MessageQueuePrefix + "/telemetry"

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
	txnCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	txnResp, err := ns.etcdClient.Txn(txnCtx).Then(ops...).Commit()
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
func (ns *NexusStreamer) watchConfiguration(ctx context.Context) error {
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

		case <-ctx.Done():
			log.Info("Stopping configuration watcher")
			return ctx.Err()
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
		// Use background context for cleanup
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := ns.serviceRegistry.Deregister(cleanupCtx); err != nil {
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

// setupHTTPRoutes configures HTTP routes for CSV upload
func (ns *NexusStreamer) setupHTTPRoutes() {
	// Middleware
	ns.echo.Use(middleware.Logger())
	ns.echo.Use(middleware.Recover())
	ns.echo.Use(middleware.CORS())

	// Health check
	ns.echo.GET("/health", ns.healthHandler)

	// CSV upload endpoint
	ns.echo.POST("/api/v1/csv/upload", ns.uploadCSVHandler)

	// Status endpoint
	ns.echo.GET("/api/v1/status", ns.statusHandler)
}

// startHTTPServer starts the HTTP server for CSV upload
func (ns *NexusStreamer) startHTTPServer(ctx context.Context) error {
	addr := fmt.Sprintf(":%d", ns.config.HTTPPort)
	log.Infof("Starting HTTP server for CSV upload on %s", addr)

	server := &http.Server{
		Addr:         addr,
		Handler:      ns.echo,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	// Start server in goroutine
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Errorf("HTTP server error: %v", err)
		}
	}()

	// Wait for context cancellation
	<-ctx.Done()
	log.Info("Attempting graceful shutdown of HTTP server")

	// Shutdown server
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Errorf("HTTP server shutdown error: %v", err)
		return err
	}

	log.Info("HTTP server shutdown completed successfully")
	return nil
}

// healthHandler handles health check requests
func (ns *NexusStreamer) healthHandler(c echo.Context) error {
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
func (ns *NexusStreamer) statusHandler(c echo.Context) error {
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

// uploadCSVHandler handles CSV file upload and immediate processing
func (ns *NexusStreamer) uploadCSVHandler(c echo.Context) error {
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

	log.Infof("Starting CSV upload and processing: %s (Size: %d bytes)", fileHeader.Filename, fileHeader.Size)

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

	// Process CSV file immediately
	processedRecords, totalRecords, headers, err := ns.processUploadedCSV(c.Request().Context(), filePath)
	if err != nil {
		os.Remove(filePath) // Clean up on error
		log.Errorf("CSV processing failed: %v", err)
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"error":   fmt.Sprintf("CSV processing failed: %v", err),
		})
	}

	duration := time.Since(startTime)
	processedAt := time.Now()

	log.Infof("CSV processing completed: %s (Records: %d/%d processed, Duration: %v)",
		fileHeader.Filename, processedRecords, totalRecords, duration)

	// Clean up uploaded file after processing (for production)
	if err := os.Remove(filePath); err != nil {
		log.Warnf("Failed to clean up uploaded file: %v", err)
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
func (ns *NexusStreamer) processUploadedCSV(ctx context.Context, filePath string) (int, int, []string, error) {
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

	log.Infof("CSV validation successful: %d headers found", len(headers))

	// Process records in batches
	batch := make([]*TelemetryRecord, 0, ns.config.BatchSize)
	totalRecords := 0
	processedRecords := 0

	for {
		record, err := reader.Read()
		if err == io.EOF {
			// Process final batch
			if len(batch) > 0 {
				if err := ns.publishBatch(ctx, batch); err != nil {
					log.Warnf("Failed to process final batch: %v", err)
				} else {
					processedRecords += len(batch)
				}
			}
			break
		}
		if err != nil {
			log.Warnf("Failed to read CSV record at line %d: %v", totalRecords+2, err)
			totalRecords++
			continue
		}

		// Parse record
		telemetryRecord, err := ns.parseCSVRecord(record, headerMap)
		if err != nil {
			log.Warnf("Failed to parse CSV record: %v", err)
			totalRecords++
			continue
		}

		batch = append(batch, telemetryRecord)
		totalRecords++

		if len(batch) >= ns.config.BatchSize {
			if err := ns.publishBatch(ctx, batch); err != nil {
				log.Warnf("Failed to process batch: %v", err)
			} else {
				processedRecords += len(batch)
			}
			batch = batch[:0] // Reset batch
		}
	}

	return processedRecords, totalRecords, headers, nil
}
