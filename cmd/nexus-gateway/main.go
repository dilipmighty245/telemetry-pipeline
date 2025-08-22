// Package main provides the Nexus API Gateway service
//
// @title Nexus Telemetry API Gateway
// @version 2.0
// @description API Gateway component for Nexus-enhanced Telemetry Pipeline with GraphQL, REST, and WebSocket support
// @termsOfService http://swagger.io/terms/
//
// @contact.name API Support
// @contact.url http://www.example.com/support
// @contact.email support@example.com
//
// @license.name MIT
// @license.url https://opensource.org/licenses/MIT
//
// @host localhost:8080
// @BasePath /
// @schemes http https
//
// @securityDefinitions.basic BasicAuth
//
// @externalDocs.description OpenAPI
// @externalDocs.url https://swagger.io/resources/open-api/
package main

import (
	"context"
	"encoding/json"
	"flag"
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

	"github.com/dilipmighty245/telemetry-pipeline/internal/graphql"
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

var (
	port            = flag.Int("port", 8080, "HTTP server port")
	logLevel        = flag.String("log-level", "info", "Log level (debug, info, warn, error, fatal)")
	enableGraphQL   = flag.Bool("enable-graphql", true, "Enable GraphQL endpoint")
	enableWebSocket = flag.Bool("enable-websocket", true, "Enable WebSocket endpoint")
	enableCORS      = flag.Bool("enable-cors", true, "Enable CORS middleware")
	clusterID       = flag.String("cluster-id", "default-cluster", "Telemetry cluster ID")
)

// NexusGateway represents the API Gateway component of the telemetry pipeline
type NexusGateway struct {
	port           int
	etcdClient     *clientv3.Client
	nexusService   *nexus.TelemetryService
	graphqlService *graphql.GraphQLService
	nexusGraphQL   nexusgraphql.ServerClient
	messageQueue   *messagequeue.MessageQueueService
	echo           *echo.Echo
	upgrader       websocket.Upgrader
	ctx            context.Context
	cancel         context.CancelFunc
	config         *GatewayConfig
}

// GatewayConfig holds configuration for the Nexus gateway
type GatewayConfig struct {
	Port            int
	ClusterID       string
	EtcdEndpoints   []string
	EnableGraphQL   bool
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

// GraphQLQuery represents a GraphQL query
type GraphQLQuery struct {
	Query     string                 `json:"query"`
	Variables map[string]interface{} `json:"variables,omitempty"`
}

// GraphQLResponse represents a GraphQL response
type GraphQLResponse struct {
	Data   interface{} `json:"data,omitempty"`
	Errors []string    `json:"errors,omitempty"`
}

func main() {
	if err := run(os.Args, os.Stdout); err != nil {
		switch err {
		case context.Canceled:
			// not considered error
		case http.ErrServerClosed:
			// not considered error
		default:
			log.Fatalf("could not run Nexus Gateway Service: %v", err)
		}
	}
}

func run(args []string, stdout io.Writer) error {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	config, err := parseConfig(args)
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
	log.Infof("GraphQL: %v, WebSocket: %v, CORS: %v", config.EnableGraphQL, config.EnableWebSocket, config.EnableCORS)

	// Create and start the gateway
	gateway, err := NewNexusGateway(config)
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

func parseConfig(args []string) (*GatewayConfig, error) {
	if err := flag.CommandLine.Parse(args[1:]); err != nil {
		return nil, fmt.Errorf("failed to parse flags: %w", err)
	}

	config := &GatewayConfig{
		Port:            *port,
		ClusterID:       *clusterID,
		EnableGraphQL:   *enableGraphQL,
		EnableWebSocket: *enableWebSocket,
		EnableCORS:      *enableCORS,
		LogLevel:        *logLevel,
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
	if envGraphQL := os.Getenv("ENABLE_GRAPHQL"); envGraphQL != "" {
		config.EnableGraphQL = envGraphQL == "true"
	}
	if envWebSocket := os.Getenv("ENABLE_WEBSOCKET"); envWebSocket != "" {
		config.EnableWebSocket = envWebSocket == "true"
	}
	if envCORS := os.Getenv("ENABLE_CORS"); envCORS != "" {
		config.EnableCORS = envCORS == "true"
	}

	return config, nil
}

// NewNexusGateway creates a new Nexus gateway
func NewNexusGateway(config *GatewayConfig) (*NexusGateway, error) {
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

	// Create Nexus service
	nexusConfig := &nexus.ServiceConfig{
		EtcdEndpoints: config.EtcdEndpoints,
		ClusterID:     config.ClusterID,
	}
	nexusService, err := nexus.NewTelemetryService(nexusConfig)
	if err != nil {
		etcdClient.Close()
		cancel()
		return nil, fmt.Errorf("failed to create Nexus service: %w", err)
	}

	// Create GraphQL service with Nexus integration
	graphqlService, err := graphql.NewGraphQLService(nexusService)
	if err != nil {
		etcdClient.Close()
		cancel()
		return nil, fmt.Errorf("failed to create GraphQL service: %w", err)
	}

	// Create Nexus GraphQL client (would connect to Nexus GraphQL server)
	// For now, we'll use a mock/placeholder as we're integrating with existing Nexus
	var nexusGraphQLClient nexusgraphql.ServerClient

	// Create message queue service
	messageQueue := messagequeue.NewMessageQueueService()

	// Create Echo instance
	e := echo.New()
	e.HideBanner = true

	// WebSocket upgrader
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true // Allow connections from any origin
		},
	}

	gateway := &NexusGateway{
		port:           config.Port,
		etcdClient:     etcdClient,
		nexusService:   nexusService,
		graphqlService: graphqlService,
		nexusGraphQL:   nexusGraphQLClient,
		messageQueue:   messageQueue,
		echo:           e,
		upgrader:       upgrader,
		ctx:            ctx,
		cancel:         cancel,
		config:         config,
	}

	// Setup routes
	gateway.setupRoutes()

	return gateway, nil
}

// Start starts the gateway service
func (ng *NexusGateway) Start(ctx context.Context) error {
	g, gCtx := errgroup.WithContext(ctx)

	// Start HTTP server
	g.Go(func() error {
		addr := fmt.Sprintf(":%d", ng.port)
		log.Infof("Starting Nexus Gateway HTTP server on %s", addr)

		server := &http.Server{
			Addr:    addr,
			Handler: ng.echo,
		}

		// Start server in goroutine
		go func() {
			if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Errorf("HTTP server error: %v", err)
			}
		}()

		// Wait for context cancellation
		<-gCtx.Done()

		// Shutdown server
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		return server.Shutdown(shutdownCtx)
	})

	return g.Wait()
}

// setupRoutes configures all HTTP routes
func (ng *NexusGateway) setupRoutes() {
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

	// GraphQL endpoint
	if ng.config.EnableGraphQL {
		ng.echo.POST("/graphql", ng.graphqlHandler)
		ng.echo.GET("/graphql", ng.graphqlPlaygroundHandler)
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

// HTTP Handlers

// REQUIRED API ENDPOINTS - implementing exact specification requirements

// listAllGPUsHandler godoc
// @Summary List all GPUs
// @Description Return a list of all GPUs for which telemetry data is available
// @Tags gpus
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/gpus [get]
func (ng *NexusGateway) listAllGPUsHandler(c echo.Context) error {
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

// queryTelemetryByGPUHandler godoc
// @Summary Query telemetry by GPU
// @Description Return all telemetry entries for a specific GPU, ordered by time
// @Tags telemetry
// @Produce json
// @Param id path string true "GPU ID"
// @Param start_time query string false "Start time (RFC3339 format)"
// @Param end_time query string false "End time (RFC3339 format)"
// @Param limit query int false "Limit number of results (default 100)"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/gpus/{id}/telemetry [get]
func (ng *NexusGateway) queryTelemetryByGPUHandler(c echo.Context) error {
	gpuID := c.Param("id")
	if gpuID == "" {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"error":   "GPU ID is required",
		})
	}

	// Parse query parameters
	startTimeStr := c.QueryParam("start_time")
	endTimeStr := c.QueryParam("end_time")
	limitStr := c.QueryParam("limit")

	var startTime, endTime *time.Time
	limit := 100

	if startTimeStr != "" {
		if t, err := time.Parse(time.RFC3339, startTimeStr); err == nil {
			startTime = &t
		}
	}
	if endTimeStr != "" {
		if t, err := time.Parse(time.RFC3339, endTimeStr); err == nil {
			endTime = &t
		}
	}
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	// First, find which host and numeric GPU ID corresponds to this UUID
	hostsKey := fmt.Sprintf("/telemetry/clusters/%s/hosts/", ng.config.ClusterID)

	ctx, cancel := context.WithTimeout(c.Request().Context(), 10*time.Second)
	defer cancel()

	resp, err := ng.etcdClient.Get(ctx, hostsKey, clientv3.WithPrefix())
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"error":   "Failed to query telemetry data",
		})
	}

	var targetHostname, targetGPUID string
	var telemetryData []*nexus.TelemetryData

	// Step 1: Find the hostname and numeric GPU ID for this UUID
	for _, kv := range resp.Kvs {
		key := string(kv.Key)
		// Look for GPU registration entries (not data entries)
		if strings.Contains(key, "/gpus/") && !strings.Contains(key, "/data/") {
			var gpu nexus.TelemetryGPU
			if err := json.Unmarshal(kv.Value, &gpu); err != nil {
				continue
			}

			// Check if this GPU has the UUID we're looking for
			if gpu.UUID == gpuID {
				// Extract hostname and GPU ID from the key
				// Key format: /telemetry/clusters/local-cluster/hosts/{hostname}/gpus/{gpu_id}
				parts := strings.Split(key, "/")
				if len(parts) >= 6 {
					targetHostname = parts[5] // hostname
					targetGPUID = parts[7]    // numeric gpu_id
					break
				}
			}
		}
	}

	// If we couldn't find the GPU with this UUID, return empty result
	if targetHostname == "" || targetGPUID == "" {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"success": true,
			"data":    []interface{}{},
			"count":   0,
		})
	}

	// Step 2: Now find telemetry data for this specific host and GPU ID
	for _, kv := range resp.Kvs {
		key := string(kv.Key)
		// Look for telemetry data entries for this specific hostname and GPU ID
		if strings.Contains(key, fmt.Sprintf("/hosts/%s/gpus/%s/data/", targetHostname, targetGPUID)) {
			var data nexus.TelemetryData
			if err := json.Unmarshal(kv.Value, &data); err != nil {
				continue
			}

			// Apply time filtering
			if startTime != nil && data.Timestamp.Before(*startTime) {
				continue
			}
			if endTime != nil && data.Timestamp.After(*endTime) {
				continue
			}

			telemetryData = append(telemetryData, &data)
		}
	}

	// Sort by timestamp (ascending)
	for i := 0; i < len(telemetryData)-1; i++ {
		for j := i + 1; j < len(telemetryData); j++ {
			if telemetryData[i].Timestamp.After(telemetryData[j].Timestamp) {
				telemetryData[i], telemetryData[j] = telemetryData[j], telemetryData[i]
			}
		}
	}

	// Apply limit
	if limit > 0 && len(telemetryData) > limit {
		telemetryData = telemetryData[:limit]
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"success": true,
		"data":    telemetryData,
		"count":   len(telemetryData),
	})
}

// listAllHostsHandler - Additional useful endpoint
func (ng *NexusGateway) listAllHostsHandler(c echo.Context) error {
	hostsKey := fmt.Sprintf("/telemetry/clusters/%s/hosts/", ng.config.ClusterID)

	ctx, cancel := context.WithTimeout(c.Request().Context(), 5*time.Second)
	defer cancel()

	resp, err := ng.etcdClient.Get(ctx, hostsKey, clientv3.WithPrefix())
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"error":   "Failed to query hosts",
		})
	}

	var hosts []string
	hostMap := make(map[string]bool)

	for _, kv := range resp.Kvs {
		key := string(kv.Key)
		// Extract hostname from path
		if !strings.Contains(key, "/gpus/") && !strings.Contains(key, "/data/") {
			parts := strings.Split(key, "/")
			for i, part := range parts {
				if part == "hosts" && i+1 < len(parts) {
					hostname := parts[i+1]
					if !hostMap[hostname] {
						hosts = append(hosts, hostname)
						hostMap[hostname] = true
					}
					break
				}
			}
		}
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"success": true,
		"data":    hosts,
		"count":   len(hosts),
	})
}

// listGPUsByHostHandler - Additional useful endpoint
func (ng *NexusGateway) listGPUsByHostHandler(c echo.Context) error {
	hostname := c.Param("hostname")

	gpusKey := fmt.Sprintf("/telemetry/clusters/%s/hosts/%s/gpus/", ng.config.ClusterID, hostname)

	ctx, cancel := context.WithTimeout(c.Request().Context(), 5*time.Second)
	defer cancel()

	resp, err := ng.etcdClient.Get(ctx, gpusKey, clientv3.WithPrefix())
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"error":   "Failed to query GPUs for host",
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

	for _, kv := range resp.Kvs {
		key := string(kv.Key)
		// Look for GPU entries (not telemetry data)
		if !strings.Contains(key, "/data/") {
			var nexusGPU nexus.TelemetryGPU
			if err := json.Unmarshal(kv.Value, &nexusGPU); err != nil {
				continue
			}

			gpu := GPU{
				ID:       nexusGPU.GPUID,
				UUID:     nexusGPU.UUID,
				Hostname: hostname,
				Name:     nexusGPU.DeviceName,
				Device:   nexusGPU.Device,
				Status:   nexusGPU.Status.State,
			}
			gpus = append(gpus, gpu)
		}
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"success": true,
		"data":    gpus,
		"count":   len(gpus),
	})
}

func (ng *NexusGateway) healthHandler(c echo.Context) error {
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

func (ng *NexusGateway) listClustersHandler(c echo.Context) error {
	cluster, err := ng.nexusService.GetClusterInfo()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": err.Error(),
		})
	}

	// Return array with single cluster for compatibility
	clusters := []*nexus.TelemetryCluster{cluster}
	return c.JSON(http.StatusOK, map[string]interface{}{
		"clusters": clusters,
		"count":    len(clusters),
	})
}

func (ng *NexusGateway) getClusterHandler(c echo.Context) error {
	clusterID := c.Param("cluster_id")

	// Validate cluster ID matches our service
	if clusterID != ng.config.ClusterID {
		return c.JSON(http.StatusNotFound, map[string]string{
			"error": "cluster not found",
		})
	}

	cluster, err := ng.nexusService.GetClusterInfo()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": err.Error(),
		})
	}

	return c.JSON(http.StatusOK, cluster)
}

func (ng *NexusGateway) getClusterStatsHandler(c echo.Context) error {
	clusterID := c.Param("cluster_id")

	// Validate cluster ID matches our service
	if clusterID != ng.config.ClusterID {
		return c.JSON(http.StatusNotFound, map[string]string{
			"error": "cluster not found",
		})
	}

	// Return basic stats (this would be enhanced with real metrics)
	stats := map[string]interface{}{
		"cluster_id": clusterID,
		"status":     "active",
		"timestamp":  time.Now().UTC(),
		"message":    "Basic stats - would be enhanced with real telemetry metrics",
	}

	return c.JSON(http.StatusOK, stats)
}

func (ng *NexusGateway) listHostsHandler(c echo.Context) error {
	clusterID := c.Param("cluster_id")

	// Validate cluster ID matches our service
	if clusterID != ng.config.ClusterID {
		return c.JSON(http.StatusNotFound, map[string]string{
			"error": "cluster not found",
		})
	}

	// This would query etcd for registered hosts
	// For now, return placeholder
	hosts := []map[string]interface{}{
		{
			"host_id":  "placeholder-host",
			"hostname": "example-host",
			"status":   "active",
		},
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"hosts": hosts,
		"count": len(hosts),
	})
}

func (ng *NexusGateway) getHostHandler(c echo.Context) error {
	clusterID := c.Param("cluster_id")
	hostID := c.Param("host_id")

	// Validate cluster ID
	if clusterID != ng.config.ClusterID {
		return c.JSON(http.StatusNotFound, map[string]string{
			"error": "cluster not found",
		})
	}

	host, err := ng.nexusService.GetHostInfo(hostID)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{
			"error": err.Error(),
		})
	}

	return c.JSON(http.StatusOK, host)
}

func (ng *NexusGateway) listGPUsHandler(c echo.Context) error {
	clusterID := c.Param("cluster_id")
	hostID := c.Param("host_id")

	// Validate cluster ID
	if clusterID != ng.config.ClusterID {
		return c.JSON(http.StatusNotFound, map[string]string{
			"error": "cluster not found",
		})
	}

	// This would query etcd for GPUs on the host
	// For now, return placeholder
	gpus := []map[string]interface{}{
		{
			"gpu_id":  "gpu-0",
			"host_id": hostID,
			"model":   "Tesla V100",
			"status":  "active",
		},
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"gpus":  gpus,
		"count": len(gpus),
	})
}

func (ng *NexusGateway) getGPUHandler(c echo.Context) error {
	clusterID := c.Param("cluster_id")
	hostID := c.Param("host_id")
	gpuID := c.Param("gpu_id")

	// Validate cluster ID
	if clusterID != ng.config.ClusterID {
		return c.JSON(http.StatusNotFound, map[string]string{
			"error": "cluster not found",
		})
	}

	// This would query etcd for specific GPU info
	gpu := map[string]interface{}{
		"gpu_id":  gpuID,
		"host_id": hostID,
		"model":   "Tesla V100",
		"status":  "active",
		"message": "GPU details would be queried from etcd",
	}

	return c.JSON(http.StatusOK, gpu)
}

func (ng *NexusGateway) getGPUMetricsHandler(c echo.Context) error {
	clusterID := c.Param("cluster_id")
	hostID := c.Param("host_id")
	gpuID := c.Param("gpu_id")

	// Validate cluster ID
	if clusterID != ng.config.ClusterID {
		return c.JSON(http.StatusNotFound, map[string]string{
			"error": "cluster not found",
		})
	}

	// Query parameters for time range
	startTimeStr := c.QueryParam("start_time")
	endTimeStr := c.QueryParam("end_time")
	limitStr := c.QueryParam("limit")

	// Parse time range
	var startTime, endTime *time.Time
	limit := 100

	if startTimeStr != "" {
		if t, err := time.Parse(time.RFC3339, startTimeStr); err == nil {
			startTime = &t
		}
	}
	if endTimeStr != "" {
		if t, err := time.Parse(time.RFC3339, endTimeStr); err == nil {
			endTime = &t
		}
	}
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil {
			limit = l
		}
	}

	metrics, err := ng.nexusService.GetGPUTelemetryData(hostID, gpuID, startTime, endTime, limit)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": err.Error(),
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"metrics": metrics,
		"count":   len(metrics),
	})
}

func (ng *NexusGateway) getTelemetryHandler(c echo.Context) error {
	// Query parameters
	clusterID := c.QueryParam("cluster_id")
	if clusterID == "" {
		clusterID = ng.config.ClusterID
	}

	// Validate cluster ID
	if clusterID != ng.config.ClusterID {
		return c.JSON(http.StatusNotFound, map[string]string{
			"error": "cluster not found",
		})
	}

	hostID := c.QueryParam("host_id")
	gpuID := c.QueryParam("gpu_id")
	startTimeStr := c.QueryParam("start_time")
	endTimeStr := c.QueryParam("end_time")
	limitStr := c.QueryParam("limit")

	// Parse parameters
	var startTime, endTime *time.Time
	limit := 100

	if startTimeStr != "" {
		if t, err := time.Parse(time.RFC3339, startTimeStr); err == nil {
			startTime = &t
		}
	}
	if endTimeStr != "" {
		if t, err := time.Parse(time.RFC3339, endTimeStr); err == nil {
			endTime = &t
		}
	}
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil {
			limit = l
		}
	}

	if hostID == "" || gpuID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "host_id and gpu_id are required",
		})
	}

	telemetry, err := ng.nexusService.GetGPUTelemetryData(hostID, gpuID, startTime, endTime, limit)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": err.Error(),
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"telemetry": telemetry,
		"count":     len(telemetry),
	})
}

func (ng *NexusGateway) getLatestTelemetryHandler(c echo.Context) error {
	clusterID := c.QueryParam("cluster_id")
	if clusterID == "" {
		clusterID = ng.config.ClusterID
	}

	// Validate cluster ID
	if clusterID != ng.config.ClusterID {
		return c.JSON(http.StatusNotFound, map[string]string{
			"error": "cluster not found",
		})
	}

	// This would query the latest telemetry from etcd
	// For now, return placeholder
	telemetry := map[string]interface{}{
		"cluster_id": clusterID,
		"timestamp":  time.Now().UTC(),
		"message":    "Latest telemetry would be queried from etcd",
		"data":       []interface{}{},
	}

	return c.JSON(http.StatusOK, telemetry)
}

// GraphQL handler - using proper GraphQL schema with Nexus integration
func (ng *NexusGateway) graphqlHandler(c echo.Context) error {
	var requestBody map[string]interface{}
	if err := c.Bind(&requestBody); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"errors": []map[string]interface{}{
				{"message": "Invalid request body"},
			},
		})
	}

	// Extract query and variables
	query, _ := requestBody["query"].(string)
	variables, _ := requestBody["variables"].(map[string]interface{})

	if query == "" {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"errors": []map[string]interface{}{
				{"message": "Missing query"},
			},
		})
	}

	// Execute GraphQL query using our schema
	result := ng.graphqlService.ExecuteQuery(query, variables)

	response := make(map[string]interface{})
	if result.Data != nil {
		response["data"] = result.Data
	}
	if len(result.Errors) > 0 {
		errors := make([]map[string]interface{}, len(result.Errors))
		for i, err := range result.Errors {
			errors[i] = map[string]interface{}{
				"message": err.Message,
			}
		}
		response["errors"] = errors
	}

	// Add Nexus extension info
	response["extensions"] = map[string]interface{}{
		"nexus":   true,
		"service": "telemetry-pipeline",
	}

	return c.JSON(http.StatusOK, response)
}

func (ng *NexusGateway) graphqlPlaygroundHandler(c echo.Context) error {
	playground := `
<!DOCTYPE html>
<html>
<head>
  <title>Telemetry Pipeline GraphQL</title>
  <link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/graphql-playground-react/build/static/css/index.css" />
</head>
<body>
  <div id="root">
    <style>
      body { margin: 0; font-family: Open Sans, sans-serif; overflow: hidden; }
      #root { height: 100vh; }
    </style>
  </div>
  <script src="https://cdn.jsdelivr.net/npm/graphql-playground-react/build/static/js/middleware.js"></script>
  <script>
    window.addEventListener('load', function (event) {
      GraphQLPlayground.init(document.getElementById('root'), {
        endpoint: '/graphql',
        settings: {
          'editor.theme': 'dark',
          'editor.fontSize': 14,
          'editor.fontFamily': 'Source Code Pro',
          'general.betaUpdates': false,
          'request.credentials': 'same-origin',
          'tracing.hideTracingResponse': false,
          'queryPlan.hideQueryPlanResponse': false,
          'editor.cursorShape': 'line',
          'editor.reuseHeaders': true,
          'schema.polling.enable': true,
          'schema.polling.endpointFilter': '*localhost*',
          'schema.polling.interval': 2000
        },
        tabs: [
          {
            endpoint: '/graphql',
            query: '# Welcome to GraphQL Playground\\n# Type queries in this side of the screen, and you will see intelligent typeaheads\\n# aware of the current GraphQL type schema and live syntax and validation errors highlighted within the text.\\n\\n# Try querying for cluster information:\\nquery GetClusters {\\n  clusters {\\n    id\\n    name\\n    totalHosts\\n    totalGPUs\\n  }\\n}\\n\\n# Or get telemetry data:\\nquery GetTelemetry {\\n  telemetry {\\n    timestamp\\n    hostId\\n    gpuId\\n    gpuUtilization\\n    memoryUtilization\\n    temperature\\n  }\\n}'
          }
        ]
      })
    })
  </script>
</body>
</html>`
	return c.HTML(http.StatusOK, playground)
}

// WebSocket handler
func (ng *NexusGateway) websocketHandler(c echo.Context) error {
	conn, err := ng.upgrader.Upgrade(c.Response(), c.Request(), nil)
	if err != nil {
		log.Errorf("WebSocket upgrade failed: %v", err)
		return err
	}
	defer conn.Close()

	log.Info("WebSocket connection established")

	// Handle WebSocket messages
	for {
		var msg map[string]interface{}
		if err := conn.ReadJSON(&msg); err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Errorf("WebSocket read error: %v", err)
			}
			break
		}

		// Echo the message back (in a real implementation, this would handle subscriptions)
		if err := conn.WriteJSON(map[string]interface{}{
			"type":      "response",
			"data":      msg,
			"timestamp": time.Now().UTC(),
		}); err != nil {
			log.Errorf("WebSocket write error: %v", err)
			break
		}
	}

	log.Info("WebSocket connection closed")
	return nil
}

// swaggerHandler serves Swagger UI
func (ng *NexusGateway) swaggerHandler(c echo.Context) error {
	path := c.Param("*")

	if path == "" || path == "/" {
		// Serve Swagger UI HTML
		swaggerHTML := `<!DOCTYPE html>
<html>
<head>
    <title>Nexus Telemetry API Documentation</title>
    <link rel="stylesheet" type="text/css" href="https://unpkg.com/swagger-ui-dist@3.25.0/swagger-ui.css" />
    <style>
        html { box-sizing: border-box; overflow: -moz-scrollbars-vertical; overflow-y: scroll; }
        *, *:before, *:after { box-sizing: inherit; }
        body { margin:0; background: #fafafa; }
    </style>
</head>
<body>
    <div id="swagger-ui"></div>
    <script src="https://unpkg.com/swagger-ui-dist@3.25.0/swagger-ui-bundle.js"></script>
    <script src="https://unpkg.com/swagger-ui-dist@3.25.0/swagger-ui-standalone-preset.js"></script>
    <script>
        window.onload = function() {
            const ui = SwaggerUIBundle({
                url: '/swagger/spec.json',
                dom_id: '#swagger-ui',
                deepLinking: true,
                presets: [
                    SwaggerUIBundle.presets.apis,
                    SwaggerUIStandalonePreset
                ],
                plugins: [
                    SwaggerUIBundle.plugins.DownloadUrl
                ],
                layout: "StandaloneLayout"
            });
        };
    </script>
</body>
</html>`
		return c.HTML(http.StatusOK, swaggerHTML)
	}

	if path == "spec.json" {
		// Serve OpenAPI spec
		spec := ng.generateOpenAPISpec()
		return c.JSON(http.StatusOK, spec)
	}

	return c.String(http.StatusNotFound, "Not found")
}

// generateOpenAPISpec generates OpenAPI 3.0 specification
func (ng *NexusGateway) generateOpenAPISpec() map[string]interface{} {
	return map[string]interface{}{
		"openapi": "3.0.0",
		"info": map[string]interface{}{
			"title":       "Nexus Telemetry Pipeline API",
			"description": "REST API for GPU telemetry pipeline integrated with Nexus Graph Framework and etcd backend",
			"version":     "2.0.0",
			"contact": map[string]interface{}{
				"name": "Telemetry Pipeline Team",
			},
			"license": map[string]interface{}{
				"name": "MIT",
			},
		},
		"servers": []map[string]interface{}{
			{
				"url":         "http://localhost:8080",
				"description": "Development server",
			},
		},
		"paths": map[string]interface{}{
			"/health": map[string]interface{}{
				"get": map[string]interface{}{
					"summary":     "Health check",
					"description": "Check API health and etcd connectivity",
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Service is healthy",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type": "object",
										"properties": map[string]interface{}{
											"status":     map[string]interface{}{"type": "string"},
											"cluster_id": map[string]interface{}{"type": "string"},
											"timestamp":  map[string]interface{}{"type": "string"},
										},
									},
								},
							},
						},
					},
				},
			},
			"/api/v1/gpus": map[string]interface{}{
				"get": map[string]interface{}{
					"summary":     "List all GPUs",
					"description": "Return a list of all GPUs for which telemetry data is available",
					"tags":        []string{"gpus"},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "List of GPUs",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type": "object",
										"properties": map[string]interface{}{
											"success": map[string]interface{}{"type": "boolean"},
											"data": map[string]interface{}{
												"type": "array",
												"items": map[string]interface{}{
													"$ref": "#/components/schemas/GPU",
												},
											},
											"count": map[string]interface{}{"type": "integer"},
										},
									},
								},
							},
						},
					},
				},
			},
			"/api/v1/gpus/{id}/telemetry": map[string]interface{}{
				"get": map[string]interface{}{
					"summary":     "Query telemetry by GPU",
					"description": "Return all telemetry entries for a specific GPU, ordered by time",
					"tags":        []string{"telemetry"},
					"parameters": []map[string]interface{}{
						{
							"name":        "id",
							"in":          "path",
							"required":    true,
							"schema":      map[string]interface{}{"type": "string"},
							"description": "GPU ID",
						},
						{
							"name":        "start_time",
							"in":          "query",
							"schema":      map[string]interface{}{"type": "string", "format": "date-time"},
							"description": "Start time (RFC3339 format)",
						},
						{
							"name":        "end_time",
							"in":          "query",
							"schema":      map[string]interface{}{"type": "string", "format": "date-time"},
							"description": "End time (RFC3339 format)",
						},
						{
							"name":        "limit",
							"in":          "query",
							"schema":      map[string]interface{}{"type": "integer"},
							"description": "Limit number of results (default 100)",
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Telemetry data",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type": "object",
										"properties": map[string]interface{}{
											"success": map[string]interface{}{"type": "boolean"},
											"data": map[string]interface{}{
												"type": "array",
												"items": map[string]interface{}{
													"$ref": "#/components/schemas/TelemetryData",
												},
											},
											"count": map[string]interface{}{"type": "integer"},
										},
									},
								},
							},
						},
					},
				},
			},
			"/api/v1/clusters": map[string]interface{}{
				"get": map[string]interface{}{
					"summary":     "List clusters",
					"description": "Get all telemetry clusters",
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "List of clusters",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type": "object",
										"properties": map[string]interface{}{
											"clusters": map[string]interface{}{
												"type": "array",
												"items": map[string]interface{}{
													"$ref": "#/components/schemas/Cluster",
												},
											},
											"count": map[string]interface{}{"type": "integer"},
										},
									},
								},
							},
						},
					},
				},
			},
			"/api/v1/clusters/{cluster_id}": map[string]interface{}{
				"get": map[string]interface{}{
					"summary":     "Get cluster",
					"description": "Get specific cluster information",
					"parameters": []map[string]interface{}{
						{
							"name":     "cluster_id",
							"in":       "path",
							"required": true,
							"schema":   map[string]interface{}{"type": "string"},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Cluster information",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"$ref": "#/components/schemas/Cluster",
									},
								},
							},
						},
					},
				},
			},
			"/api/v1/telemetry": map[string]interface{}{
				"get": map[string]interface{}{
					"summary":     "Get telemetry data",
					"description": "Query telemetry data with filters",
					"parameters": []map[string]interface{}{
						{
							"name":   "host_id",
							"in":     "query",
							"schema": map[string]interface{}{"type": "string"},
						},
						{
							"name":   "gpu_id",
							"in":     "query",
							"schema": map[string]interface{}{"type": "string"},
						},
						{
							"name":   "start_time",
							"in":     "query",
							"schema": map[string]interface{}{"type": "string", "format": "date-time"},
						},
						{
							"name":   "end_time",
							"in":     "query",
							"schema": map[string]interface{}{"type": "string", "format": "date-time"},
						},
						{
							"name":   "limit",
							"in":     "query",
							"schema": map[string]interface{}{"type": "integer"},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Telemetry data",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type": "object",
										"properties": map[string]interface{}{
											"telemetry": map[string]interface{}{
												"type": "array",
												"items": map[string]interface{}{
													"$ref": "#/components/schemas/TelemetryData",
												},
											},
											"count": map[string]interface{}{"type": "integer"},
										},
									},
								},
							},
						},
					},
				},
			},
			"/graphql": map[string]interface{}{
				"post": map[string]interface{}{
					"summary":     "GraphQL endpoint",
					"description": "Execute GraphQL queries and mutations",
					"requestBody": map[string]interface{}{
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"type": "object",
									"properties": map[string]interface{}{
										"query":     map[string]interface{}{"type": "string"},
										"variables": map[string]interface{}{"type": "object"},
									},
									"required": []string{"query"},
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "GraphQL response",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type": "object",
										"properties": map[string]interface{}{
											"data":   map[string]interface{}{"type": "object"},
											"errors": map[string]interface{}{"type": "array"},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		"components": map[string]interface{}{
			"schemas": map[string]interface{}{
				"GPU": map[string]interface{}{
					"type":        "object",
					"description": "GPU device information",
					"properties": map[string]interface{}{
						"id":       map[string]interface{}{"type": "string", "example": "0", "description": "Host-specific GPU ID"},
						"uuid":     map[string]interface{}{"type": "string", "example": "GPU-5fd4f087-86f3-7a43-b711-4771313afc50", "description": "Globally unique GPU identifier"},
						"hostname": map[string]interface{}{"type": "string", "example": "mtv5-dgx1-hgpu-031"},
						"name":     map[string]interface{}{"type": "string", "example": "NVIDIA H100 80GB HBM3"},
						"device":   map[string]interface{}{"type": "string", "example": "nvidia0", "description": "Device name"},
						"status":   map[string]interface{}{"type": "string", "example": "active"},
					},
				},
				"Cluster": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"cluster_id":   map[string]interface{}{"type": "string"},
						"cluster_name": map[string]interface{}{"type": "string"},
						"region":       map[string]interface{}{"type": "string"},
						"environment":  map[string]interface{}{"type": "string"},
						"created_at":   map[string]interface{}{"type": "string", "format": "date-time"},
						"updated_at":   map[string]interface{}{"type": "string", "format": "date-time"},
						"metadata": map[string]interface{}{
							"type": "object",
							"properties": map[string]interface{}{
								"total_hosts":  map[string]interface{}{"type": "integer"},
								"total_gpus":   map[string]interface{}{"type": "integer"},
								"active_hosts": map[string]interface{}{"type": "integer"},
								"active_gpus":  map[string]interface{}{"type": "integer"},
							},
						},
					},
				},
				"TelemetryData": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"timestamp":          map[string]interface{}{"type": "string", "format": "date-time"},
						"gpu_id":             map[string]interface{}{"type": "string"},
						"hostname":           map[string]interface{}{"type": "string"},
						"gpu_utilization":    map[string]interface{}{"type": "number"},
						"memory_utilization": map[string]interface{}{"type": "number"},
						"memory_used_mb":     map[string]interface{}{"type": "number"},
						"memory_free_mb":     map[string]interface{}{"type": "number"},
						"temperature":        map[string]interface{}{"type": "number"},
						"power_draw":         map[string]interface{}{"type": "number"},
						"sm_clock_mhz":       map[string]interface{}{"type": "number"},
						"memory_clock_mhz":   map[string]interface{}{"type": "number"},
					},
				},
			},
		},
	}
}

// Close closes the gateway and cleans up resources
func (ng *NexusGateway) Close() error {
	log.Info("Closing Nexus gateway")

	ng.cancel()

	// MessageQueueService cleanup is handled by context cancellation

	if ng.nexusService != nil {
		ng.nexusService.Close()
	}

	if ng.etcdClient != nil {
		ng.etcdClient.Close()
	}

	return nil
}
