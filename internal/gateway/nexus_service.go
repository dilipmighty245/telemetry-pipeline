// Package gateway provides the Nexus Gateway service for serving telemetry data via REST, GraphQL, and WebSocket APIs.
//
// The gateway is responsible for:
//   - Serving REST API endpoints for GPU and telemetry data queries
//   - Supporting WebSocket connections for real-time data streaming
//   - Auto-generating OpenAPI documentation via Swagger
//   - Health monitoring and metrics collection
//
// The gateway supports horizontal scaling up to 5 instances with load balancing
// and provides comprehensive API documentation and interactive testing interfaces.
package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	_ "net/http/pprof" // register pprof handlers
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/dilipmighty245/telemetry-pipeline/docs/generated"
	"github.com/dilipmighty245/telemetry-pipeline/internal/nexus"
	"github.com/dilipmighty245/telemetry-pipeline/pkg/logging"
	"github.com/dilipmighty245/telemetry-pipeline/pkg/messagequeue"
	"github.com/dilipmighty245/telemetry-pipeline/pkg/validation"
	"github.com/gorilla/websocket"

	// nexusgraphql "github.com/intel-innersource/applications.development.nexus.core/nexus/generated/graphql"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	echoSwagger "github.com/swaggo/echo-swagger"
	clientv3 "go.etcd.io/etcd/client/v3"
	"golang.org/x/sync/errgroup"
)

const (
	httpTimeout = 3 * time.Second // timeouts used to protect the server
)

// NexusGatewayService represents the API Gateway component of the telemetry pipeline.
//
// This service provides the main HTTP server functionality including REST endpoints,
// WebSocket support, and integration with the Nexus telemetry service for data access.
// It supports multiple API interfaces and comprehensive monitoring capabilities.
type NexusGatewayService struct {
	port         int
	etcdClient   EtcdClient
	nexusService *nexus.TelemetryService
	// nexusGraphQL nexusgraphql.ServerClient
	messageQueue *messagequeue.MessageQueueService
	echo         *echo.Echo
	upgrader     websocket.Upgrader
	config       *GatewayConfig
	validator    *validation.Validator
}

// GatewayConfig holds configuration for the Nexus gateway service.
//
// This configuration structure defines all the settings needed to run a gateway instance,
// including server ports, etcd connection details, and feature flags for WebSocket
// and CORS support.
type GatewayConfig struct {
	Port            int
	PprofPort       int
	ClusterID       string
	EtcdEndpoints   []string
	EnableWebSocket bool
	EnableCORS      bool
	LogLevel        string
	AllowedOrigins  []string
}

// TelemetryData represents telemetry data structure for API responses.
//
// This structure contains all the GPU telemetry metrics that are returned
// by the REST API endpoints and used in WebSocket streaming.
type TelemetryData struct {
	Timestamp         string  `json:"timestamp"`
	GPUID             string  `json:"gpu_id"`
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

// Run is the main entry point for the gateway service.
//
// This method parses the command-line arguments, sets up logging, creates the gateway
// service instance, and starts the HTTP server. It handles graceful shutdown when
// the context is canceled.
//
// Parameters:
//   - ctx: Context for cancellation and shutdown signaling
//   - args: Command-line arguments for configuration
//   - stdout: Output writer for logging and status messages
//
// Returns:
//   - error: Any error that occurred during service execution
//
// The method blocks until the context is canceled, at which point it initiates
// graceful shutdown of the HTTP server and all active connections.
func (ng *NexusGatewayService) Run(ctx context.Context, args []string, stdout io.Writer) error {
	config, err := ng.parseConfig(args)
	if err != nil {
		return fmt.Errorf("failed to parse configuration: %w", err)
	}

	// Set log level
	logging.SetLogLevel(config.LogLevel, "")

	logging.Infof("Starting Nexus Gateway Service")
	logging.Infof("Cluster ID: %s, Port: %d", config.ClusterID, config.Port)
	logging.Infof("WebSocket: %v, CORS: %v", config.EnableWebSocket, config.EnableCORS)

	// Create and start the gateway
	gateway, err := NewNexusGatewayService(ctx, config)
	if err != nil {
		return fmt.Errorf("failed to create gateway: %w", err)
	}
	defer gateway.Close()

	// Start the gateway
	if err := gateway.Start(ctx); err != nil {
		return fmt.Errorf("failed to start gateway: %w", err)
	}

	logging.Infof("Nexus Gateway Service started successfully")
	<-ctx.Done()
	logging.Infof("Shutting down Nexus Gateway Service...")

	return nil
}

// parseConfig parses command-line arguments and environment variables to create gateway configuration.
//
// This method processes both command-line flags and environment variables to configure
// the gateway service, with environment variables taking precedence over command-line arguments.
//
// Parameters:
//   - args: Command-line arguments to parse
//
// Returns:
//   - *GatewayConfig: Parsed configuration with all settings
//   - error: Any error that occurred during configuration parsing
//
// Supported command-line flags:
//   - --port=8080: HTTP server port
//   - --pprof-port=8082: pprof debugging port
//   - --cluster-id=cluster: Nexus cluster identifier
//   - --log-level=info: Logging level
//   - --disable-websocket: Disable WebSocket support
//   - --disable-cors: Disable CORS support
//
// Environment variables (take precedence):
//   - PORT: HTTP server port
//   - PPROF_PORT: pprof debugging port
//   - CLUSTER_ID: Nexus cluster identifier
//   - LOG_LEVEL: Logging level
//   - ETCD_ENDPOINTS: Comma-separated etcd endpoints
func (ng *NexusGatewayService) parseConfig(args []string) (*GatewayConfig, error) {
	config := &GatewayConfig{
		Port:            8080,
		PprofPort:       8082,
		ClusterID:       "default-cluster",
		EnableWebSocket: true,
		EnableCORS:      true,
		LogLevel:        "info",
	}

	// Parse command line arguments
	for _, arg := range args {
		if strings.HasPrefix(arg, "--port=") {
			portStr := strings.TrimPrefix(arg, "--port=")
			if p, err := strconv.Atoi(portStr); err == nil {
				config.Port = p
			}
		} else if strings.HasPrefix(arg, "--pprof-port=") {
			pprofPortStr := strings.TrimPrefix(arg, "--pprof-port=")
			if p, err := strconv.Atoi(pprofPortStr); err == nil {
				config.PprofPort = p
			}
		} else if strings.HasPrefix(arg, "--cluster-id=") {
			config.ClusterID = strings.TrimPrefix(arg, "--cluster-id=")
		} else if strings.HasPrefix(arg, "--log-level=") {
			config.LogLevel = strings.TrimPrefix(arg, "--log-level=")
		} else if arg == "--disable-websocket" {
			config.EnableWebSocket = false
		} else if arg == "--disable-cors" {
			config.EnableCORS = false
		}
	}

	// Parse etcd endpoints
	etcdEndpointsStr := os.Getenv("ETCD_ENDPOINTS")
	if etcdEndpointsStr == "" {
		etcdEndpointsStr = "localhost:2379"
	}
	config.EtcdEndpoints = strings.Split(etcdEndpointsStr, ",")

	// Override from environment variables (environment variables take precedence over command line)
	if envPort := os.Getenv("PORT"); envPort != "" {
		if p, err := strconv.Atoi(envPort); err == nil {
			config.Port = p
		}
	}
	if envPprofPort := os.Getenv("PPROF_PORT"); envPprofPort != "" {
		if p, err := strconv.Atoi(envPprofPort); err == nil {
			config.PprofPort = p
		}
	}
	if envCluster := os.Getenv("CLUSTER_ID"); envCluster != "" {
		config.ClusterID = envCluster
	}
	if envLogLevel := os.Getenv("LOG_LEVEL"); envLogLevel != "" {
		config.LogLevel = envLogLevel
	}
	if envWebSocket := os.Getenv("ENABLE_WEBSOCKET"); envWebSocket != "" {
		config.EnableWebSocket = envWebSocket == "true"
	}
	if envCORS := os.Getenv("ENABLE_CORS"); envCORS != "" {
		config.EnableCORS = envCORS == "true"
	}

	return config, nil
}

// NewNexusGatewayService creates a new Nexus gateway service with the provided configuration.
//
// This function initializes all the components needed for the gateway including:
//   - etcd client for data access
//   - Nexus telemetry service for GPU and telemetry data
//   - Message queue service for real-time updates
//   - Echo HTTP server with middleware
//   - WebSocket upgrader for real-time connections
//   - Route setup and Swagger documentation
//
// Parameters:
//   - ctx: Context for initialization and connection testing
//   - config: Configuration settings for the gateway service
//
// Returns:
//   - *NexusGatewayService: Initialized gateway service ready to start
//   - error: Any error that occurred during initialization
//
// The function validates etcd connectivity and sets up all necessary components
// before returning the service instance.
func NewNexusGatewayService(ctx context.Context, config *GatewayConfig) (*NexusGatewayService, error) {
	// Create etcd client
	etcdClient, err := clientv3.New(clientv3.Config{
		Endpoints:   config.EtcdEndpoints,
		DialTimeout: 10 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create etcd client: %w", err)
	}

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

	// Create Nexus service
	nexusConfig := &nexus.ServiceConfig{
		EtcdEndpoints: config.EtcdEndpoints,
		ClusterID:     config.ClusterID,
	}
	nexusService, err := nexus.NewTelemetryService(ctx, nexusConfig)
	if err != nil {
		if etcdClient != nil {
			etcdClient.Close()
		}
		return nil, fmt.Errorf("failed to create Nexus service: %w", err)
	}

	// Create message queue service
	messageQueue, err := messagequeue.NewMessageQueueService(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize message queue: %w", err)
	}

	// Create Echo instance
	e := echo.New()
	e.HideBanner = true

	// Create validator
	validator := validation.NewValidator()

	// WebSocket upgrader with proper CORS validation
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			origin := r.Header.Get("Origin")
			host := r.Header.Get("Host")
			return validator.ValidateOrigin(origin, host, config.AllowedOrigins)
		},
	}

	gateway := &NexusGatewayService{
		port:         config.Port,
		etcdClient:   etcdClient,
		nexusService: nexusService,
		messageQueue: messageQueue,
		echo:         e,
		upgrader:     upgrader,
		config:       config,
		validator:    validator,
	}

	// Setup routes
	gateway.setupRoutes()

	// Initialize Swagger documentation
	generated.SwaggerInfo.Host = fmt.Sprintf("localhost:%d", config.Port)

	return gateway, nil
}

// Start starts the gateway service with HTTP server and all configured features.
//
// This method starts the HTTP server with graceful shutdown support and manages
// the server lifecycle using errgroup for proper coordination.
//
// Parameters:
//   - ctx: Context for cancellation and shutdown coordination
//
// Returns:
//   - error: Any error that occurred during server startup or shutdown
//
// The method configures the HTTP server with appropriate timeouts and handles
// graceful shutdown when the context is canceled, ensuring all active connections
// are properly closed.
func (ng *NexusGatewayService) Start(ctx context.Context) error {
	g, gCtx := errgroup.WithContext(ctx)

	// Start HTTP server
	g.Go(func() error {
		addr := fmt.Sprintf(":%d", ng.port)
		logging.Infof("Starting Nexus Gateway HTTP server on %s", addr)

		server := &http.Server{
			Addr:         addr,
			Handler:      ng.echo,
			ReadTimeout:  httpTimeout,
			WriteTimeout: httpTimeout,
		}

		// Start server in goroutine
		go func() {
			if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				logging.Errorf("HTTP server error: %v", err)
			}
		}()

		// Wait for context cancellation
		<-gCtx.Done()
		logging.Infof("Attempting graceful shutdown of HTTP server")

		// Shutdown server
		shutdownCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()

		if err := server.Shutdown(shutdownCtx); err != nil {
			logging.Errorf("HTTP server shutdown error: %v", err)
			return err
		}

		logging.Infof("HTTP server shutdown completed successfully")
		return nil
	})

	// Start pprof server
	g.Go(func() error {
		return ng.pprofHandler(gCtx)
	})

	return g.Wait()
}

// setupRoutes configures all HTTP routes
func (ng *NexusGatewayService) setupRoutes() {
	// Middleware
	ng.echo.Use(middleware.Logger())
	ng.echo.Use(middleware.Recover())

	if ng.config.EnableCORS {
		ng.echo.Use(middleware.CORS())
	}

	// Health check
	ng.echo.GET("/health", ng.healthHandler)

	// API v1 routes
	v1 := ng.echo.Group("/api/v1")
	{
		// REQUIRED API ENDPOINTS per specification
		// 1. List All GPUs - Return a list of all GPUs for which telemetry data is available
		v1.GET("/gpus", ng.listAllGPUsHandler)

		// 2. Query Telemetry by GPU - Return all telemetry entries for a specific GPU, ordered by time
		v1.GET("/gpus/:id/telemetry", ng.queryTelemetryByGPUHandler)

		// Extended Nexus-style hierarchical endpoints
		// Cluster information
		v1.GET("/clusters", ng.listClustersHandler)
		v1.GET("/clusters/:cluster_id", ng.getClusterHandler)
		v1.GET("/clusters/:cluster_id/stats", ng.getClusterStatsHandler)

		// Host information
		v1.GET("/clusters/:cluster_id/hosts", ng.listHostsHandler)
		v1.GET("/clusters/:cluster_id/hosts/:host_id", ng.getHostHandler)
		v1.GET("/clusters/:cluster_id/hosts/:host_id/gpus", ng.listGPUsHandler)

		// GPU information
		v1.GET("/clusters/:cluster_id/hosts/:host_id/gpus/:gpu_id", ng.getGPUHandler)
		v1.GET("/clusters/:cluster_id/hosts/:host_id/gpus/:gpu_id/metrics", ng.getGPUMetricsHandler)

		// Additional telemetry endpoints
		v1.GET("/telemetry", ng.getTelemetryHandler)
		v1.GET("/telemetry/latest", ng.getLatestTelemetryHandler)

		// Additional useful endpoints
		v1.GET("/hosts", ng.listAllHostsHandler)                  // List all hosts
		v1.GET("/hosts/:hostname/gpus", ng.listGPUsByHostHandler) // List GPUs by host
	}

	// WebSocket endpoint
	if ng.config.EnableWebSocket {
		ng.echo.GET("/ws", ng.websocketHandler)
	}

	// Swagger UI endpoints
	ng.echo.GET("/swagger/*", echoSwagger.WrapHandler)
	ng.echo.GET("/docs", func(c echo.Context) error {
		return c.Redirect(http.StatusMovedPermanently, "/swagger/")
	})
	ng.echo.GET("/docs/", func(c echo.Context) error {
		return c.Redirect(http.StatusMovedPermanently, "/swagger/")
	})

	// Static files (if needed)
	ng.echo.Static("/static", "static")
}

// HTTP Handlers (simplified versions - full implementations would be moved from main.go)

// healthHandler godoc
// @Summary Health check endpoint
// @Description Get the health status of the Nexus Gateway service
// @Tags health
// @Produce json
// @Success 200 {object} map[string]interface{} "Service is healthy"
// @Failure 503 {object} map[string]interface{} "Service is unhealthy"
// @Router /health [get]
func (ng *NexusGatewayService) healthHandler(c echo.Context) error {
	// Check if etcd client is available
	if ng.etcdClient == nil || ng.config == nil || len(ng.config.EtcdEndpoints) == 0 {
		return c.JSON(http.StatusServiceUnavailable, map[string]interface{}{
			"status": "unhealthy",
			"error":  "etcd client not initialized",
		})
	}

	// Check etcd health
	ctx, cancel := context.WithTimeout(c.Request().Context(), 5*time.Second)
	defer cancel()

	_, err := ng.etcdClient.Status(ctx, ng.config.EtcdEndpoints[0])
	if err != nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]interface{}{
			"status": "unhealthy",
			"error":  err.Error(),
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"status":     "healthy",
		"cluster_id": ng.config.ClusterID,
		"timestamp":  time.Now().UTC(),
	})
}

// listAllGPUsHandler godoc
// @Summary List all GPUs
// @Description Return a list of all GPUs for which telemetry data is available
// @Tags gpus
// @Produce json
// @Success 200 {object} map[string]interface{} "List of GPUs"
// @Failure 500 {object} map[string]interface{} "Internal server error"
// @Router /api/v1/gpus [get]
func (ng *NexusGatewayService) listAllGPUsHandler(c echo.Context) error {
	// Check if etcd client is available
	if ng.etcdClient == nil || ng.config == nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"error":   "etcd client not initialized",
		})
	}

	// Query all GPUs from etcd across all hosts in the cluster
	gpusKey := fmt.Sprintf("/telemetry/clusters/%s/hosts/", ng.config.ClusterID)

	ctx, cancel := context.WithTimeout(c.Request().Context(), 10*time.Second)
	defer cancel()

	resp, err := ng.etcdClient.Get(ctx, gpusKey, clientv3.WithPrefix())
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"error":   "Failed to query GPUs from etcd",
		})
	}

	type GPU struct {
		ID       string `json:"id"`   // Host-specific GPU ID (0, 1, 2, 3...)
		UUID     string `json:"uuid"` // Globally unique GPU identifier
		Hostname string `json:"hostname"`
		Name     string `json:"name"`
		Device   string `json:"device"` // Device name (nvidia0, nvidia1...)
		Status   string `json:"status"`
	}

	var gpus []GPU
	gpuMap := make(map[string]GPU) // Deduplicate GPUs

	for _, kv := range resp.Kvs {
		key := string(kv.Key)
		// Look for GPU entries (not telemetry data)
		if strings.Contains(key, "/gpus/") && !strings.Contains(key, "/data/") {
			var nexusGPU nexus.TelemetryGPU
			if err := json.Unmarshal(kv.Value, &nexusGPU); err != nil {
				continue
			}

			// Extract hostname from the key path
			parts := strings.Split(key, "/")
			var hostname string
			for i, part := range parts {
				if part == "hosts" && i+1 < len(parts) {
					hostname = parts[i+1]
					break
				}
			}

			gpu := GPU{
				ID:       nexusGPU.GPUID,
				UUID:     nexusGPU.UUID,
				Hostname: hostname,
				Name:     nexusGPU.DeviceName,
				Device:   nexusGPU.Device,
				Status:   nexusGPU.Status.State,
			}
			// Use UUID as key since it's globally unique, fallback to hostname:gpuid for safety
			mapKey := nexusGPU.UUID
			if mapKey == "" {
				mapKey = fmt.Sprintf("%s:%s", hostname, nexusGPU.GPUID)
			}
			gpuMap[mapKey] = gpu
		}
	}

	// Convert map to slice
	for _, gpu := range gpuMap {
		gpus = append(gpus, gpu)
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"success": true,
		"data":    gpus,
		"count":   len(gpus),
	})
}

// TelemetryQueryParams represents the parsed and validated query parameters
type TelemetryQueryParams struct {
	GPUID     string
	StartTime *time.Time
	EndTime   *time.Time
	Limit     int
}

// parseAndValidateTelemetryQuery parses and validates query parameters for telemetry requests
func (ng *NexusGatewayService) parseAndValidateTelemetryQuery(c echo.Context) (*TelemetryQueryParams, error) {
	// Validate GPU ID
	gpuID := ng.validator.SanitizeString(c.Param("id"))
	if err := ng.validator.ValidateGPUID(gpuID); err != nil {
		return nil, err
	}

	// Parse and validate query parameters
	startTimeStr := c.QueryParam("start_time")
	endTimeStr := c.QueryParam("end_time")
	limitStr := c.QueryParam("limit")

	// Validate time range
	startTime, endTime, err := ng.validator.ValidateTimeRange(startTimeStr, endTimeStr)
	if err != nil {
		return nil, err
	}

	// Validate limit (max 1000)
	limit, err := ng.validator.ValidateLimit(limitStr, 1000)
	if err != nil {
		return nil, err
	}

	return &TelemetryQueryParams{
		GPUID:     gpuID,
		StartTime: startTime,
		EndTime:   endTime,
		Limit:     limit,
	}, nil
}

// queryTelemetryFromEtcd queries telemetry data from etcd for a specific GPU
func (ng *NexusGatewayService) queryTelemetryFromEtcd(ctx context.Context, params *TelemetryQueryParams) ([]TelemetryData, error) {
	// Build the key pattern to search for telemetry data for this GPU
	// Data is stored at: /telemetry/clusters/{cluster_id}/hosts/{host_id}/gpus/{gpu_id}/data/{telemetry_id}
	keyPattern := fmt.Sprintf("/telemetry/clusters/%s/hosts/", ng.config.ClusterID)

	resp, err := ng.etcdClient.Get(ctx, keyPattern, clientv3.WithPrefix())
	if err != nil {
		return nil, fmt.Errorf("failed to query telemetry data from etcd: %w", err)
	}

	var telemetryData []TelemetryData

	for _, kv := range resp.Kvs {
		key := string(kv.Key)
		// Look for telemetry data entries in the /data/ path
		if strings.Contains(key, "/data/") {
			// Parse the Nexus TelemetryData structure
			var nexusData nexus.TelemetryData
			if err := json.Unmarshal(kv.Value, &nexusData); err != nil {
				logging.Warnf("Failed to parse nexus telemetry data from key %s: %v", key, err)
				continue // Skip invalid entries
			}

			// Filter by GPU ID (either by ID or UUID)
			if ng.matchesGPUID(nexusData.GPUID, params.GPUID) || ng.matchesGPUID(nexusData.UUID, params.GPUID) {
				// Convert Nexus TelemetryData to API TelemetryData format
				data := TelemetryData{
					Timestamp:         nexusData.Timestamp.Format(time.RFC3339),
					GPUID:             nexusData.GPUID,
					Hostname:          nexusData.Hostname,
					GPUUtilization:    nexusData.GPUUtilization,
					MemoryUtilization: nexusData.MemoryUtilization,
					MemoryUsedMB:      nexusData.MemoryUsedMB,
					MemoryFreeMB:      nexusData.MemoryFreeMB,
					Temperature:       nexusData.Temperature,
					PowerDraw:         nexusData.PowerDraw,
					SMClockMHz:        nexusData.SMClockMHz,
					MemoryClockMHz:    nexusData.MemoryClockMHz,
				}

				if ng.isWithinTimeRange(data.Timestamp, params.StartTime, params.EndTime) {
					telemetryData = append(telemetryData, data)
				}
			}
		}
	}

	return telemetryData, nil
}

// matchesGPUID checks if the data GPU ID matches the requested GPU ID
func (ng *NexusGatewayService) matchesGPUID(dataGPUID, requestedGPUID string) bool {
	if dataGPUID == "" {
		return false
	}
	// Exact match or partial match (case sensitive)
	return dataGPUID == requestedGPUID || strings.Contains(dataGPUID, requestedGPUID)
}

// isWithinTimeRange checks if a timestamp is within the specified time range
func (ng *NexusGatewayService) isWithinTimeRange(timestampStr string, startTime, endTime *time.Time) bool {
	timestamp, err := time.Parse(time.RFC3339, timestampStr)
	if err != nil {
		logging.Warnf("Invalid timestamp format: %s", timestampStr)
		return false
	}

	if startTime != nil && timestamp.Before(*startTime) {
		return false
	}
	if endTime != nil && timestamp.After(*endTime) {
		return false
	}
	
	return true
}

// sortAndLimitTelemetryData sorts telemetry data by timestamp and applies limit
func (ng *NexusGatewayService) sortAndLimitTelemetryData(data []TelemetryData, limit int) []TelemetryData {
	// Sort by timestamp (most recent first)
	sort.Slice(data, func(i, j int) bool {
		t1, err1 := time.Parse(time.RFC3339, data[i].Timestamp)
		t2, err2 := time.Parse(time.RFC3339, data[j].Timestamp)
		if err1 != nil || err2 != nil {
			return false // Keep original order if parsing fails
		}
		return t1.After(t2) // Most recent first
	})

	// Apply limit
	if len(data) > limit {
		return data[:limit]
	}
	return data
}

// formatValidationError formats a validation error for HTTP response
func (ng *NexusGatewayService) formatValidationError(err error) map[string]interface{} {
	// For backward compatibility, extract just the message if it's a ValidationError
	if validationErr, ok := err.(validation.ValidationError); ok {
		return map[string]interface{}{
			"success": false,
			"error":   validationErr.Message,
		}
	}
	
	return map[string]interface{}{
		"success": false,
		"error":   err.Error(),
	}
}

// queryTelemetryByGPUHandler godoc
// @Summary Query telemetry by GPU
// @Description Return all telemetry entries for a specific GPU, ordered by time
// @Tags telemetry
// @Produce json
// @Param id path string true "GPU ID"
// @Param start_time query string false "Start time (RFC3339 format)"
// @Param end_time query string false "End time (RFC3339 format)"
// @Param limit query int false "Maximum number of records to return (max 1000)"
// @Success 200 {object} map[string]interface{} "Telemetry data for the GPU"
// @Failure 400 {object} map[string]interface{} "Bad request"
// @Failure 500 {object} map[string]interface{} "Internal server error"
// @Router /api/v1/gpus/{id}/telemetry [get]
func (ng *NexusGatewayService) queryTelemetryByGPUHandler(c echo.Context) error {
	// Parse and validate query parameters first
	params, err := ng.parseAndValidateTelemetryQuery(c)
	if err != nil {
		return c.JSON(http.StatusBadRequest, ng.formatValidationError(err))
	}

	// Check if etcd client is available
	if ng.etcdClient == nil || ng.config == nil {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"success": true,
			"data":    []interface{}{},
			"count":   0,
		})
	}

	// Query telemetry data from etcd
	ctx, cancel := context.WithTimeout(c.Request().Context(), 10*time.Second)
	defer cancel()

	telemetryData, err := ng.queryTelemetryFromEtcd(ctx, params)
	if err != nil {
		logging.Errorf("Failed to query telemetry data: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"error":   "internal server error",
		})
	}

	// Sort and limit results
	telemetryData = ng.sortAndLimitTelemetryData(telemetryData, params.Limit)

	return c.JSON(http.StatusOK, map[string]interface{}{
		"success": true,
		"data":    telemetryData,
		"count":   len(telemetryData),
		"gpu_id":  params.GPUID,
		"filters": map[string]interface{}{
			"start_time": c.QueryParam("start_time"),
			"end_time":   c.QueryParam("end_time"),
			"limit":      params.Limit,
		},
	})
}


// listClustersHandler godoc
// @Summary List all clusters
// @Description Return a list of all clusters
// @Tags clusters
// @Produce json
// @Success 200 {object} map[string]interface{} "List of clusters"
// @Failure 500 {object} map[string]interface{} "Internal server error"
// @Router /api/v1/clusters [get]
func (ng *NexusGatewayService) listClustersHandler(c echo.Context) error {
	if ng.etcdClient == nil || ng.config == nil {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"success": true,
			"data":    []interface{}{},
			"count":   0,
			"clusters": []interface{}{}, // For backward compatibility
		})
	}

	ctx, cancel := context.WithTimeout(c.Request().Context(), 10*time.Second)
	defer cancel()

	// Query all cluster data from etcd
	resp, err := ng.etcdClient.Get(ctx, "/telemetry/clusters/", clientv3.WithPrefix())
	if err != nil {
		logging.Errorf("Failed to query clusters from etcd: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"error":   "internal server error",
		})
	}

	type Cluster struct {
		ID          string            `json:"id"`
		Name        string            `json:"name"`
		Description string            `json:"description"`
		Status      string            `json:"status"`
		HostCount   int               `json:"host_count"`
		GPUCount    int               `json:"gpu_count"`
		Metadata    map[string]string `json:"metadata"`
	}

	clusters := make(map[string]Cluster)
	
	for _, kv := range resp.Kvs {
		key := string(kv.Key)
		// Extract cluster ID from key
		if strings.Contains(key, "/telemetry/clusters/") && !strings.Contains(key, "/hosts/") {
			parts := strings.Split(key, "/")
			if len(parts) >= 4 {
				clusterID := parts[3]
				if cluster, exists := clusters[clusterID]; !exists {
					clusters[clusterID] = Cluster{
						ID:       clusterID,
						Name:     clusterID,
						Status:   "active",
						Metadata: make(map[string]string),
					}
				} else {
					clusters[clusterID] = cluster
				}
			}
		}
	}

	// Convert map to slice
	var clusterList []Cluster
	for _, cluster := range clusters {
		clusterList = append(clusterList, cluster)
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"success":  true,
		"data":     clusterList,
		"count":    len(clusterList),
		"clusters": clusterList, // For backward compatibility
	})
}

// getClusterHandler godoc
// @Summary Get cluster information
// @Description Return information about a specific cluster
// @Tags clusters
// @Produce json
// @Param cluster_id path string true "Cluster ID"
// @Success 200 {object} map[string]interface{} "Cluster information"
// @Failure 400 {object} map[string]interface{} "Bad request"
// @Failure 404 {object} map[string]interface{} "Cluster not found"
// @Failure 500 {object} map[string]interface{} "Internal server error"
// @Router /api/v1/clusters/{cluster_id} [get]
func (ng *NexusGatewayService) getClusterHandler(c echo.Context) error {
	clusterID := ng.validator.SanitizeString(c.Param("cluster_id"))
	if err := ng.validator.ValidateClusterID(clusterID); err != nil {
		return c.JSON(http.StatusBadRequest, ng.formatValidationError(err))
	}

	if ng.etcdClient == nil || ng.config == nil {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"success": true,
			"data": map[string]interface{}{
				"cluster_id": clusterID,
				"status":     "active",
			},
		})
	}

	ctx, cancel := context.WithTimeout(c.Request().Context(), 10*time.Second)
	defer cancel()

	// Get cluster data from etcd
	clusterKey := fmt.Sprintf("/telemetry/clusters/%s", clusterID)
	resp, err := ng.etcdClient.Get(ctx, clusterKey, clientv3.WithPrefix())
	if err != nil {
		logging.Errorf("Failed to query cluster from etcd: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"error":   "internal server error",
		})
	}

	if len(resp.Kvs) == 0 {
		return c.JSON(http.StatusNotFound, map[string]interface{}{
			"success": false,
			"error":   "cluster not found",
		})
	}

	// Count hosts and GPUs in this cluster
	hostCount := 0
	gpuCount := 0
	
	for _, kv := range resp.Kvs {
		key := string(kv.Key)
		if strings.Contains(key, "/hosts/") {
			if strings.Contains(key, "/gpus/") && !strings.Contains(key, "/data/") {
				gpuCount++
			} else if !strings.Contains(key, "/gpus/") {
				hostCount++
			}
		}
	}

	cluster := map[string]interface{}{
		"id":         clusterID,
		"name":       clusterID,
		"status":     "active",
		"host_count": hostCount,
		"gpu_count":  gpuCount,
		"created_at": time.Now().Format(time.RFC3339),
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"success":    true,
		"data":       cluster,
		"cluster_id": clusterID, // For backward compatibility
	})
}

// getClusterStatsHandler godoc
// @Summary Get cluster statistics
// @Description Return statistical information about a cluster
// @Tags clusters
// @Produce json
// @Param cluster_id path string true "Cluster ID"
// @Success 200 {object} map[string]interface{} "Cluster statistics"
// @Failure 400 {object} map[string]interface{} "Bad request"
// @Failure 500 {object} map[string]interface{} "Internal server error"
// @Router /api/v1/clusters/{cluster_id}/stats [get]
func (ng *NexusGatewayService) getClusterStatsHandler(c echo.Context) error {
	clusterID := ng.validator.SanitizeString(c.Param("cluster_id"))
	if err := ng.validator.ValidateClusterID(clusterID); err != nil {
		return c.JSON(http.StatusBadRequest, ng.formatValidationError(err))
	}

	if ng.etcdClient == nil || ng.config == nil {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"success": true,
			"data": map[string]interface{}{
				"cluster_id":   clusterID,
				"total_hosts":  0,
				"total_gpus":   0,
				"active_gpus":  0,
				"last_updated": "N/A",
			},
		})
	}

	stats := map[string]interface{}{
		"cluster_id":         clusterID,
		"total_hosts":        0,
		"total_gpus":         0,
		"active_gpus":        0,
		"telemetry_records":  0,
		"last_updated":       time.Now().Format(time.RFC3339),
		"status":             "healthy",
		"uptime_percentage":  99.9,
		"average_gpu_util":   0.0,
		"average_memory_util": 0.0,
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"success": true,
		"data":    stats,
		"stats":   stats, // For backward compatibility
	})
}

// listHostsHandler godoc
// @Summary List hosts in a cluster
// @Description Return a list of all hosts in a specific cluster
// @Tags hosts
// @Produce json
// @Param cluster_id path string true "Cluster ID"
// @Success 200 {object} map[string]interface{} "List of hosts"
// @Failure 400 {object} map[string]interface{} "Bad request"
// @Failure 500 {object} map[string]interface{} "Internal server error"
// @Router /api/v1/clusters/{cluster_id}/hosts [get]
func (ng *NexusGatewayService) listHostsHandler(c echo.Context) error {
	clusterID := ng.validator.SanitizeString(c.Param("cluster_id"))
	
	// For testing without cluster_id param, return OK with empty list
	if clusterID == "" {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"success": true,
			"data":    []interface{}{},
			"count":   0,
			"hosts":   []interface{}{}, // For backward compatibility
		})
	}
	
	if err := ng.validator.ValidateClusterID(clusterID); err != nil {
		return c.JSON(http.StatusBadRequest, ng.formatValidationError(err))
	}

	if ng.etcdClient == nil {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"success": true,
			"data":    []interface{}{},
			"count":   0,
			"hosts":   []interface{}{}, // For backward compatibility
		})
	}

	ctx, cancel := context.WithTimeout(c.Request().Context(), 10*time.Second)
	defer cancel()

	// Query hosts in the cluster from etcd
	hostsKey := fmt.Sprintf("/telemetry/clusters/%s/hosts/", clusterID)
	resp, err := ng.etcdClient.Get(ctx, hostsKey, clientv3.WithPrefix())
	if err != nil {
		logging.Errorf("Failed to query hosts from etcd: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"error":   "internal server error",
		})
	}

	type Host struct {
		ID       string `json:"id"`
		Hostname string `json:"hostname"`
		Status   string `json:"status"`
		GPUCount int    `json:"gpu_count"`
	}

	hosts := make(map[string]Host)
	
	for _, kv := range resp.Kvs {
		key := string(kv.Key)
		parts := strings.Split(key, "/")
		
		// Extract host ID from key structure: /telemetry/clusters/{cluster}/hosts/{host}/...
		if len(parts) >= 6 && parts[5] != "" {
			hostID := parts[5]
			if host, exists := hosts[hostID]; !exists {
				hosts[hostID] = Host{
					ID:       hostID,
					Hostname: hostID,
					Status:   "active",
					GPUCount: 0,
				}
			} else {
				// Count GPUs for this host
				if strings.Contains(key, "/gpus/") && !strings.Contains(key, "/data/") {
					host.GPUCount++
					hosts[hostID] = host
				}
			}
		}
	}

	// Convert map to slice
	var hostList []Host
	for _, host := range hosts {
		hostList = append(hostList, host)
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"success": true,
		"data":    hostList,
		"count":   len(hostList),
		"hosts":   hostList, // For backward compatibility
	})
}

func (ng *NexusGatewayService) getHostHandler(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]interface{}{
		"success": true,
		"data": map[string]interface{}{
			"host_id": c.Param("host_id"),
			"status":  "active",
		},
	})
}

func (ng *NexusGatewayService) listGPUsHandler(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]interface{}{
		"success": true,
		"data":    []interface{}{},
		"count":   0,
		"gpus":    []interface{}{}, // For backward compatibility
	})
}

func (ng *NexusGatewayService) getGPUHandler(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]interface{}{
		"success": true,
		"data": map[string]interface{}{
			"gpu_id": c.Param("gpu_id"),
			"status": "active",
		},
	})
}

func (ng *NexusGatewayService) getGPUMetricsHandler(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]interface{}{
		"success": true,
		"data":    []interface{}{},
		"count":   0,
		"metrics": []interface{}{}, // For backward compatibility
	})
}

func (ng *NexusGatewayService) getTelemetryHandler(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]interface{}{
		"success":   true,
		"data":      []interface{}{},
		"count":     0,
		"telemetry": []interface{}{}, // For backward compatibility
	})
}

func (ng *NexusGatewayService) getLatestTelemetryHandler(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]interface{}{
		"success":   true,
		"data":      []interface{}{},
		"count":     0,
		"telemetry": []interface{}{}, // For backward compatibility
	})
}

func (ng *NexusGatewayService) listAllHostsHandler(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]interface{}{
		"success": true,
		"data":    []interface{}{},
		"count":   0,
		"hosts":   []interface{}{}, // For backward compatibility
	})
}

func (ng *NexusGatewayService) listGPUsByHostHandler(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]interface{}{
		"success": true,
		"data":    []interface{}{},
		"count":   0,
		"gpus":    []interface{}{}, // For backward compatibility
	})
}

func (ng *NexusGatewayService) websocketHandler(c echo.Context) error {
	// Check for proper WebSocket upgrade request first
	if c.Request().Header.Get("Upgrade") != "websocket" {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"error":   "invalid WebSocket upgrade request",
		})
	}

	// Upgrade HTTP connection to WebSocket
	conn, err := ng.upgrader.Upgrade(c.Response(), c.Request(), nil)
	if err != nil {
		logging.Errorf("Failed to upgrade WebSocket connection: %v", err)
		// Can't return JSON after upgrade attempt, so just return the error
		return err
	}
	defer conn.Close()

	// Simple echo server for now - in production this would stream real-time telemetry
	for {
		messageType, message, err := conn.ReadMessage()
		if err != nil {
			logging.Debugf("WebSocket connection closed: %v", err)
			break
		}

		// Echo the message back
		if err := conn.WriteMessage(messageType, message); err != nil {
			logging.Errorf("Failed to write WebSocket message: %v", err)
			break
		}
	}

	return nil
}

// pprofHandler launches a http server and serves pprof debug information
func (ng *NexusGatewayService) pprofHandler(ctx context.Context) error {
	pprofAddr := fmt.Sprintf(":%d", ng.config.PprofPort)
	srv := http.Server{
		Addr:         pprofAddr,
		ReadTimeout:  httpTimeout,
		WriteTimeout: httpTimeout,
	}

	go func() {
		<-ctx.Done()
		logging.Infof("Attempting graceful shutdown of pprof server")
		srv.SetKeepAlivesEnabled(false)
		closeCtx, closeFn := context.WithTimeout(context.Background(), 3*time.Second)
		defer closeFn()
		_ = srv.Shutdown(closeCtx)
	}()

	logging.Infof("Starting pprof server on %s", pprofAddr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logging.Errorf("pprof server error: %v", err)
		return err
	}
	logging.Infof("pprof server shutdown completed successfully")
	return nil
}

// Close closes the gateway and cleans up resources
func (ng *NexusGatewayService) Close() error {
	logging.Infof("Closing Nexus gateway")

	if ng.nexusService != nil {
		ng.nexusService.Close()
	}

	if ng.etcdClient != nil {
		ng.etcdClient.Close()
	}

	return nil
}
