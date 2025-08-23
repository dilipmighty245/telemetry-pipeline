package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	_ "net/http/pprof" // register pprof handlers
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/dilipmighty245/telemetry-pipeline/internal/nexus"
	"github.com/dilipmighty245/telemetry-pipeline/pkg/messagequeue"
	"github.com/gorilla/websocket"
	nexusgraphql "github.com/intel-innersource/applications.development.nexus.core/nexus/generated/graphql"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	log "github.com/sirupsen/logrus"
	clientv3 "go.etcd.io/etcd/client/v3"
	"golang.org/x/sync/errgroup"
)

const (
	httpDebugAddr = ":8082"         // pprof bind address
	httpTimeout   = 3 * time.Second // timeouts used to protect the server
)

// NexusGatewayService represents the API Gateway component of the telemetry pipeline
type NexusGatewayService struct {
	port         int
	etcdClient   *clientv3.Client
	nexusService *nexus.TelemetryService
	nexusGraphQL nexusgraphql.ServerClient
	messageQueue *messagequeue.MessageQueueService
	echo         *echo.Echo
	upgrader     websocket.Upgrader
	config       *GatewayConfig
}

// GatewayConfig holds configuration for the Nexus gateway
type GatewayConfig struct {
	Port            int
	ClusterID       string
	EtcdEndpoints   []string
	EnableWebSocket bool
	EnableCORS      bool
	LogLevel        string
}

// TelemetryData represents telemetry data structure
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

// Run is the main entry point for the gateway service
func (ng *NexusGatewayService) Run(args []string, stdout io.Writer) error {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	config, err := ng.parseConfig(args)
	if err != nil {
		return fmt.Errorf("failed to parse configuration: %w", err)
	}

	// Set log level
	level, err := log.ParseLevel(config.LogLevel)
	if err != nil {
		return fmt.Errorf("invalid log level: %w", err)
	}
	log.SetLevel(level)

	log.Infof("Starting Nexus Gateway Service")
	log.Infof("Cluster ID: %s, Port: %d", config.ClusterID, config.Port)
	log.Infof("WebSocket: %v, CORS: %v", config.EnableWebSocket, config.EnableCORS)

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

	log.Info("Nexus Gateway Service started successfully")
	<-ctx.Done()
	log.Info("Shutting down Nexus Gateway Service...")

	return nil
}

func (ng *NexusGatewayService) parseConfig(args []string) (*GatewayConfig, error) {
	config := &GatewayConfig{
		Port:            8080,
		ClusterID:       "default-cluster",
		EnableWebSocket: true,
		EnableCORS:      true,
		LogLevel:        "info",
	}

	// Parse etcd endpoints
	etcdEndpointsStr := os.Getenv("ETCD_ENDPOINTS")
	if etcdEndpointsStr == "" {
		etcdEndpointsStr = "localhost:2379"
	}
	config.EtcdEndpoints = strings.Split(etcdEndpointsStr, ",")

	// Override from environment variables
	if envPort := os.Getenv("PORT"); envPort != "" {
		if p, err := strconv.Atoi(envPort); err == nil {
			config.Port = p
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

// NewNexusGatewayService creates a new Nexus gateway service
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

	// Create Nexus GraphQL client (would connect to Nexus GraphQL server)
	// For now, we'll use a mock/placeholder as we're integrating with existing Nexus
	var nexusGraphQLClient nexusgraphql.ServerClient

	// Create message queue service
	messageQueue, err := messagequeue.NewMessageQueueService()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize message queue: %w", err)
	}

	// Create Echo instance
	e := echo.New()
	e.HideBanner = true

	// WebSocket upgrader
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true // Allow connections from any origin
		},
	}

	gateway := &NexusGatewayService{
		port:         config.Port,
		etcdClient:   etcdClient,
		nexusService: nexusService,
		nexusGraphQL: nexusGraphQLClient,
		messageQueue: messageQueue,
		echo:         e,
		upgrader:     upgrader,
		config:       config,
	}

	// Setup routes
	gateway.setupRoutes()

	return gateway, nil
}

// Start starts the gateway service
func (ng *NexusGatewayService) Start(ctx context.Context) error {
	g, gCtx := errgroup.WithContext(ctx)

	// Start HTTP server
	g.Go(func() error {
		addr := fmt.Sprintf(":%d", ng.port)
		log.Infof("Starting Nexus Gateway HTTP server on %s", addr)

		server := &http.Server{
			Addr:         addr,
			Handler:      ng.echo,
			ReadTimeout:  httpTimeout,
			WriteTimeout: httpTimeout,
		}

		// Start server in goroutine
		go func() {
			if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Errorf("HTTP server error: %v", err)
			}
		}()

		// Wait for context cancellation
		<-gCtx.Done()
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

	// Swagger UI endpoint
	ng.echo.GET("/swagger/*", ng.swaggerHandler)
	ng.echo.GET("/docs", func(c echo.Context) error {
		return c.Redirect(http.StatusMovedPermanently, "/swagger/")
	})

	// Static files (if needed)
	ng.echo.Static("/static", "static")
}

// HTTP Handlers (simplified versions - full implementations would be moved from main.go)

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

func (ng *NexusGatewayService) queryTelemetryByGPUHandler(c echo.Context) error {
	gpuID := c.Param("id")
	if gpuID == "" {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"error":   "GPU ID is required",
		})
	}

	// Check if etcd client is available
	if ng.etcdClient == nil || ng.config == nil {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"success": true,
			"data":    []TelemetryData{},
			"count":   0,
			"message": "etcd client not initialized - returning empty result",
		})
	}

	// Parse query parameters
	startTimeStr := c.QueryParam("start_time")
	endTimeStr := c.QueryParam("end_time")
	limitStr := c.QueryParam("limit")

	var startTime, endTime *time.Time
	limit := 100

	if startTimeStr != "" {
		if t, err := time.Parse(time.RFC3339, startTimeStr); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]interface{}{
				"success": false,
				"error":   "Invalid start_time format. Use RFC3339 format (e.g., 2024-01-01T00:00:00Z)",
			})
		} else {
			startTime = &t
		}
	}
	if endTimeStr != "" {
		if t, err := time.Parse(time.RFC3339, endTimeStr); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]interface{}{
				"success": false,
				"error":   "Invalid end_time format. Use RFC3339 format (e.g., 2024-01-01T00:00:00Z)",
			})
		} else {
			endTime = &t
		}
	}
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err != nil || l <= 0 {
			return c.JSON(http.StatusBadRequest, map[string]interface{}{
				"success": false,
				"error":   "Invalid limit parameter. Must be a positive integer",
			})
		} else {
			limit = l
		}
	}

	// Validate time range
	if startTime != nil && endTime != nil && startTime.After(*endTime) {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"error":   "start_time must be before end_time",
		})
	}

	// Query telemetry data from etcd
	ctx, cancel := context.WithTimeout(c.Request().Context(), 10*time.Second)
	defer cancel()

	// Build the key pattern to search for telemetry data for this GPU
	// We search across all hosts and clusters for this GPU UUID
	keyPattern := fmt.Sprintf("/telemetry/clusters/%s/hosts/", ng.config.ClusterID)

	resp, err := ng.etcdClient.Get(ctx, keyPattern, clientv3.WithPrefix())
	if err != nil {
		log.Errorf("Failed to query telemetry data from etcd: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"error":   "Failed to query telemetry data",
		})
	}

	var telemetryData []TelemetryData

	for _, kv := range resp.Kvs {
		key := string(kv.Key)
		// Look for telemetry data entries that contain our GPU ID
		if strings.Contains(key, "/data/") && (strings.Contains(key, gpuID) || strings.Contains(key, fmt.Sprintf("gpu_%s", gpuID))) {
			var data TelemetryData
			if err := json.Unmarshal(kv.Value, &data); err != nil {
				continue // Skip invalid entries
			}

			// Filter by GPU ID (either by ID or UUID)
			if data.GPUID == gpuID || strings.Contains(data.GPUID, gpuID) {
				// Parse timestamp for filtering
				if timestamp, err := time.Parse(time.RFC3339, data.Timestamp); err == nil {
					// Apply time filters
					if startTime != nil && timestamp.Before(*startTime) {
						continue
					}
					if endTime != nil && timestamp.After(*endTime) {
						continue
					}
					telemetryData = append(telemetryData, data)
				}
			}
		}
	}

	// Sort by timestamp (most recent first)
	for i := 0; i < len(telemetryData)-1; i++ {
		for j := i + 1; j < len(telemetryData); j++ {
			t1, _ := time.Parse(time.RFC3339, telemetryData[i].Timestamp)
			t2, _ := time.Parse(time.RFC3339, telemetryData[j].Timestamp)
			if t1.Before(t2) {
				telemetryData[i], telemetryData[j] = telemetryData[j], telemetryData[i]
			}
		}
	}

	// Apply limit
	if len(telemetryData) > limit {
		telemetryData = telemetryData[:limit]
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"success": true,
		"data":    telemetryData,
		"count":   len(telemetryData),
		"gpu_id":  gpuID,
		"filters": map[string]interface{}{
			"start_time": startTimeStr,
			"end_time":   endTimeStr,
			"limit":      limit,
		},
	})
}

// Placeholder implementations for other handlers
func (ng *NexusGatewayService) listClustersHandler(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]interface{}{"clusters": []interface{}{}, "count": 0})
}

func (ng *NexusGatewayService) getClusterHandler(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]interface{}{"cluster_id": c.Param("cluster_id")})
}

func (ng *NexusGatewayService) getClusterStatsHandler(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]interface{}{"stats": "placeholder"})
}

func (ng *NexusGatewayService) listHostsHandler(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]interface{}{"hosts": []interface{}{}, "count": 0})
}

func (ng *NexusGatewayService) getHostHandler(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]interface{}{"host_id": c.Param("host_id")})
}

func (ng *NexusGatewayService) listGPUsHandler(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]interface{}{"gpus": []interface{}{}, "count": 0})
}

func (ng *NexusGatewayService) getGPUHandler(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]interface{}{"gpu_id": c.Param("gpu_id")})
}

func (ng *NexusGatewayService) getGPUMetricsHandler(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]interface{}{"metrics": []interface{}{}, "count": 0})
}

func (ng *NexusGatewayService) getTelemetryHandler(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]interface{}{"telemetry": []interface{}{}, "count": 0})
}

func (ng *NexusGatewayService) getLatestTelemetryHandler(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]interface{}{"telemetry": []interface{}{}, "count": 0})
}

func (ng *NexusGatewayService) listAllHostsHandler(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]interface{}{"hosts": []interface{}{}, "count": 0})
}

func (ng *NexusGatewayService) listGPUsByHostHandler(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]interface{}{"gpus": []interface{}{}, "count": 0})
}

func (ng *NexusGatewayService) websocketHandler(c echo.Context) error {
	return c.String(http.StatusOK, "WebSocket endpoint")
}

func (ng *NexusGatewayService) swaggerHandler(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]interface{}{"swagger": "placeholder"})
}

// pprofHandler launches a http server and serves pprof debug information
func (ng *NexusGatewayService) pprofHandler(ctx context.Context) error {
	srv := http.Server{
		Addr:         httpDebugAddr,
		ReadTimeout:  httpTimeout,
		WriteTimeout: httpTimeout,
	}

	go func() {
		<-ctx.Done()
		log.Info("Attempting graceful shutdown of pprof server")
		srv.SetKeepAlivesEnabled(false)
		closeCtx, closeFn := context.WithTimeout(context.Background(), 3*time.Second)
		defer closeFn()
		_ = srv.Shutdown(closeCtx)
	}()

	log.Infof("Starting pprof server on %s", httpDebugAddr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Errorf("pprof server error: %v", err)
		return err
	}
	log.Info("pprof server shutdown completed successfully")
	return nil
}

// Close closes the gateway and cleans up resources
func (ng *NexusGatewayService) Close() error {
	log.Info("Closing Nexus gateway")

	if ng.nexusService != nil {
		ng.nexusService.Close()
	}

	if ng.etcdClient != nil {
		ng.etcdClient.Close()
	}

	return nil
}
