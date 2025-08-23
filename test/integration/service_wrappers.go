//go:build integration
// +build integration

package integration

import (
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	// Import the service packages directly
	"github.com/dilipmighty245/telemetry-pipeline/internal/nexus"
	configpkg "github.com/dilipmighty245/telemetry-pipeline/pkg/config"
	"github.com/dilipmighty245/telemetry-pipeline/pkg/discovery"
	"github.com/dilipmighty245/telemetry-pipeline/pkg/scaling"

	"github.com/labstack/echo/v4"
	log "github.com/sirupsen/logrus"
	clientv3 "go.etcd.io/etcd/client/v3"
)

// ServiceWrapper provides utilities for running services in integration tests
type ServiceWrapper struct {
	stdout io.Writer
	stderr io.Writer
}

// TelemetryRecord represents a telemetry data record
type TelemetryRecord struct {
	Timestamp         string  `json:"timestamp"`
	GPUID             string  `json:"gpu_id"`
	UUID              string  `json:"uuid"`
	Device            string  `json:"device"`
	ModelName         string  `json:"model_name"`
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

// TestStreamer is a test version of the streamer service
type TestStreamer struct {
	config          *StreamerConfig
	etcdClient      *clientv3.Client
	serviceRegistry *discovery.ServiceRegistry
	configManager   *configpkg.ConfigManager
	scalingCoord    *scaling.ScalingCoordinator
	echo            *echo.Echo
	messageCount    int64
	startTime       time.Time
}

// Start starts the test streamer
func (ts *TestStreamer) Start(ctx context.Context) error {
	// Simplified start logic for testing
	return nil
}

// Close closes the test streamer
func (ts *TestStreamer) Close() error {
	if ts.etcdClient != nil {
		ts.etcdClient.Close()
	}
	return nil
}

// setupHTTPRoutes sets up HTTP routes for the test streamer
func (ts *TestStreamer) setupHTTPRoutes() {
	// Simplified setup for testing
}

// TestCollector is a test version of the collector service
type TestCollector struct {
	config         *CollectorConfig
	nexusService   *nexus.TelemetryService
	etcdClient     *clientv3.Client
	hostRegistry   map[string]bool
	gpuRegistry    map[string]bool
	registryMutex  sync.RWMutex
	processingChan chan *TelemetryRecord
	messageCount   int64
	startTime      time.Time
}

// Start starts the test collector
func (tc *TestCollector) Start(ctx context.Context) error {
	// Simplified start logic for testing
	return nil
}

// Close closes the test collector
func (tc *TestCollector) Close() error {
	if tc.etcdClient != nil {
		tc.etcdClient.Close()
	}
	if tc.nexusService != nil {
		tc.nexusService.Close()
	}
	return nil
}

// TestGateway is a test version of the gateway service
type TestGateway struct {
	port         int
	etcdClient   *clientv3.Client
	nexusService *nexus.TelemetryService
	echo         *echo.Echo
	config       *GatewayConfig
}

// Start starts the test gateway
func (tg *TestGateway) Start(ctx context.Context) error {
	// Simplified start logic for testing
	return nil
}

// Close closes the test gateway
func (tg *TestGateway) Close() error {
	if tg.etcdClient != nil {
		tg.etcdClient.Close()
	}
	if tg.nexusService != nil {
		tg.nexusService.Close()
	}
	return nil
}

// setupRoutes sets up routes for the test gateway
func (tg *TestGateway) setupRoutes() {
	// Simplified setup for testing
}

// NewServiceWrapper creates a new service wrapper
func NewServiceWrapper() *ServiceWrapper {
	return &ServiceWrapper{
		stdout: os.Stdout,
		stderr: os.Stderr,
	}
}

// StreamerConfig holds configuration for the test streamer
type StreamerConfig struct {
	EtcdEndpoints      []string
	ClusterID          string
	StreamerID         string
	MessageQueuePrefix string
	BatchSize          int
	StreamInterval     time.Duration
	HTTPPort           int
	EnableHTTP         bool
	UploadDir          string
	MaxUploadSize      int64
	MaxMemory          int64
	LogLevel           string
}

// CollectorConfig holds configuration for the test collector
type CollectorConfig struct {
	EtcdEndpoints      []string
	ClusterID          string
	CollectorID        string
	MessageQueuePrefix string
	PollTimeout        time.Duration
	BatchSize          int
	PollInterval       time.Duration
	BufferSize         int
	Workers            int
	EnableNexus        bool
	EnableWatchAPI     bool
	EnableGraphQL      bool
	EnableStreaming    bool
	LogLevel           string
}

// GatewayConfig holds configuration for the test gateway
type GatewayConfig struct {
	Port            int
	ClusterID       string
	EtcdEndpoints   []string
	EnableGraphQL   bool
	EnableWebSocket bool
	EnableCORS      bool
	LogLevel        string
}

// RunStreamer runs the streamer service with the given configuration
func (sw *ServiceWrapper) RunStreamer(ctx context.Context, args []string) error {
	config, err := sw.parseStreamerConfig(args)
	if err != nil {
		return fmt.Errorf("failed to parse streamer configuration: %w", err)
	}

	// Set log level
	level, err := log.ParseLevel(config.LogLevel)
	if err != nil {
		return fmt.Errorf("invalid log level: %w", err)
	}
	log.SetLevel(level)

	log.Infof("Starting test Nexus streamer")
	log.Infof("Cluster ID: %s, Streamer ID: %s", config.ClusterID, config.StreamerID)

	// Create streamer service
	streamer, err := sw.createStreamer(ctx, config)
	if err != nil {
		return fmt.Errorf("failed to create streamer: %w", err)
	}
	defer streamer.Close()

	// Start streaming
	if err := streamer.Start(ctx); err != nil {
		return fmt.Errorf("failed to start streamer: %w", err)
	}

	log.Info("Test Nexus Streamer started successfully")
	<-ctx.Done()
	log.Info("Shutting down test Nexus Streamer...")

	return nil
}

// RunCollector runs the collector service with the given configuration
func (sw *ServiceWrapper) RunCollector(ctx context.Context, args []string) error {
	config, err := sw.parseCollectorConfig(args)
	if err != nil {
		return fmt.Errorf("failed to parse collector configuration: %w", err)
	}

	// Set log level
	level, err := log.ParseLevel(config.LogLevel)
	if err != nil {
		return fmt.Errorf("invalid log level: %w", err)
	}
	log.SetLevel(level)

	log.Infof("Starting test Nexus collector")
	log.Infof("Cluster ID: %s, Collector ID: %s", config.ClusterID, config.CollectorID)

	// Create collector service
	collector, err := sw.createCollector(ctx, config)
	if err != nil {
		return fmt.Errorf("failed to create collector: %w", err)
	}
	defer collector.Close()

	// Start collector
	if err := collector.Start(ctx); err != nil {
		return fmt.Errorf("failed to start collector: %w", err)
	}

	log.Info("Test Nexus Collector started successfully")
	<-ctx.Done()
	log.Info("Shutting down test Nexus Collector...")

	return nil
}

// RunGateway runs the gateway service with the given configuration
func (sw *ServiceWrapper) RunGateway(ctx context.Context, args []string) error {
	config, err := sw.parseGatewayConfig(args)
	if err != nil {
		return fmt.Errorf("failed to parse gateway configuration: %w", err)
	}

	// Set log level
	level, err := log.ParseLevel(config.LogLevel)
	if err != nil {
		return fmt.Errorf("invalid log level: %w", err)
	}
	log.SetLevel(level)

	log.Infof("Starting test Nexus Gateway")
	log.Infof("Cluster ID: %s, Port: %d", config.ClusterID, config.Port)

	// Create gateway service
	gateway, err := sw.createGateway(ctx, config)
	if err != nil {
		return fmt.Errorf("failed to create gateway: %w", err)
	}
	defer gateway.Close()

	// Start gateway
	if err := gateway.Start(ctx); err != nil {
		return fmt.Errorf("failed to start gateway: %w", err)
	}

	log.Info("Test Nexus Gateway started successfully")
	<-ctx.Done()
	log.Info("Shutting down test Nexus Gateway...")

	return nil
}

// Configuration parsers

func (sw *ServiceWrapper) parseStreamerConfig(args []string) (*StreamerConfig, error) {
	config := &StreamerConfig{
		EtcdEndpoints:      []string{"localhost:2379"},
		ClusterID:          "default-cluster",
		StreamerID:         fmt.Sprintf("streamer-%d", time.Now().Unix()),
		MessageQueuePrefix: "/telemetry/queue",
		BatchSize:          100,
		StreamInterval:     3 * time.Second,
		HTTPPort:           8081,
		EnableHTTP:         true,
		UploadDir:          "/tmp/telemetry-uploads",
		MaxUploadSize:      100 * 1024 * 1024, // 100MB
		MaxMemory:          32 * 1024 * 1024,  // 32MB
		LogLevel:           "info",
	}

	// Parse command line arguments
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-cluster-id":
			if i+1 < len(args) {
				config.ClusterID = args[i+1]
				i++
			}
		case "-streamer-id":
			if i+1 < len(args) {
				config.StreamerID = args[i+1]
				i++
			}
		case "-http-port":
			if i+1 < len(args) {
				if port, err := strconv.Atoi(args[i+1]); err == nil {
					config.HTTPPort = port
				}
				i++
			}
		case "-batch-size":
			if i+1 < len(args) {
				if size, err := strconv.Atoi(args[i+1]); err == nil {
					config.BatchSize = size
				}
				i++
			}
		case "-stream-interval":
			if i+1 < len(args) {
				if interval, err := time.ParseDuration(args[i+1]); err == nil {
					config.StreamInterval = interval
				}
				i++
			}
		case "-log-level":
			if i+1 < len(args) {
				config.LogLevel = args[i+1]
				i++
			}
		}
	}

	// Override from environment variables
	if etcdEndpoints := os.Getenv("ETCD_ENDPOINTS"); etcdEndpoints != "" {
		config.EtcdEndpoints = strings.Split(etcdEndpoints, ",")
	}
	if uploadDir := os.Getenv("UPLOAD_DIR"); uploadDir != "" {
		config.UploadDir = uploadDir
	}

	return config, nil
}

func (sw *ServiceWrapper) parseCollectorConfig(args []string) (*CollectorConfig, error) {
	config := &CollectorConfig{
		EtcdEndpoints:      []string{"localhost:2379"},
		ClusterID:          "default-cluster",
		CollectorID:        fmt.Sprintf("collector-%d", time.Now().Unix()),
		MessageQueuePrefix: "/telemetry/queue",
		PollTimeout:        5 * time.Second,
		BatchSize:          100,
		PollInterval:       1 * time.Second,
		BufferSize:         10000,
		Workers:            8,
		EnableNexus:        true,
		EnableWatchAPI:     true,
		EnableGraphQL:      true,
		EnableStreaming:    true,
		LogLevel:           "info",
	}

	// Parse command line arguments
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-cluster-id":
			if i+1 < len(args) {
				config.ClusterID = args[i+1]
				i++
			}
		case "-collector-id":
			if i+1 < len(args) {
				config.CollectorID = args[i+1]
				i++
			}
		case "-batch-size":
			if i+1 < len(args) {
				if size, err := strconv.Atoi(args[i+1]); err == nil {
					config.BatchSize = size
				}
				i++
			}
		case "-poll-interval":
			if i+1 < len(args) {
				if interval, err := time.ParseDuration(args[i+1]); err == nil {
					config.PollInterval = interval
				}
				i++
			}
		case "-workers":
			if i+1 < len(args) {
				if workers, err := strconv.Atoi(args[i+1]); err == nil {
					config.Workers = workers
				}
				i++
			}
		case "-log-level":
			if i+1 < len(args) {
				config.LogLevel = args[i+1]
				i++
			}
		}
	}

	// Override from environment variables
	if etcdEndpoints := os.Getenv("ETCD_ENDPOINTS"); etcdEndpoints != "" {
		config.EtcdEndpoints = strings.Split(etcdEndpoints, ",")
	}

	return config, nil
}

func (sw *ServiceWrapper) parseGatewayConfig(args []string) (*GatewayConfig, error) {
	config := &GatewayConfig{
		Port:            8080,
		ClusterID:       "default-cluster",
		EtcdEndpoints:   []string{"localhost:2379"},
		EnableGraphQL:   true,
		EnableWebSocket: true,
		EnableCORS:      true,
		LogLevel:        "info",
	}

	// Parse command line arguments
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-port":
			if i+1 < len(args) {
				if port, err := strconv.Atoi(args[i+1]); err == nil {
					config.Port = port
				}
				i++
			}
		case "-cluster-id":
			if i+1 < len(args) {
				config.ClusterID = args[i+1]
				i++
			}
		case "-log-level":
			if i+1 < len(args) {
				config.LogLevel = args[i+1]
				i++
			}
		}
	}

	// Override from environment variables
	if etcdEndpoints := os.Getenv("ETCD_ENDPOINTS"); etcdEndpoints != "" {
		config.EtcdEndpoints = strings.Split(etcdEndpoints, ",")
	}

	return config, nil
}

// Service creators - simplified versions of the main package logic

func (sw *ServiceWrapper) createStreamer(ctx context.Context, config *StreamerConfig) (*TestStreamer, error) {
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

	// Create HTTP server
	echoServer := echo.New()
	echoServer.HideBanner = true

	streamer := &TestStreamer{
		config:          config,
		etcdClient:      etcdClient,
		serviceRegistry: serviceRegistry,
		configManager:   configManager,
		scalingCoord:    scalingCoord,
		echo:            echoServer,
		startTime:       time.Now(),
	}

	// Setup HTTP routes
	streamer.setupHTTPRoutes()

	return streamer, nil
}

func (sw *ServiceWrapper) createCollector(ctx context.Context, config *CollectorConfig) (*TestCollector, error) {
	collector := &TestCollector{
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
			EnableGraphQL:  config.EnableGraphQL,
		}

		nexusService, err := nexus.NewTelemetryService(nexusConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to create Nexus service: %w", err)
		}
		collector.nexusService = nexusService
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

	return collector, nil
}

func (sw *ServiceWrapper) createGateway(ctx context.Context, config *GatewayConfig) (*TestGateway, error) {
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

	// Create Nexus service
	nexusConfig := &nexus.ServiceConfig{
		EtcdEndpoints: config.EtcdEndpoints,
		ClusterID:     config.ClusterID,
	}
	nexusService, err := nexus.NewTelemetryService(nexusConfig)
	if err != nil {
		if etcdClient != nil {
			etcdClient.Close()
		}
		return nil, fmt.Errorf("failed to create Nexus service: %w", err)
	}

	// Create Echo instance
	e := echo.New()
	e.HideBanner = true

	gateway := &TestGateway{
		port:         config.Port,
		etcdClient:   etcdClient,
		nexusService: nexusService,
		echo:         e,
		config:       config,
	}

	// Setup routes
	gateway.setupRoutes()

	return gateway, nil
}
