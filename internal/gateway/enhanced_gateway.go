package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/dilipmighty245/telemetry-pipeline/pkg/config"
	"github.com/dilipmighty245/telemetry-pipeline/pkg/logging"
	"github.com/dilipmighty245/telemetry-pipeline/pkg/models"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	clientv3 "go.etcd.io/etcd/client/v3"
)

// EnhancedGateway provides REST API and WebSocket endpoints for telemetry data
type EnhancedGateway struct {
	config      *config.CentralizedConfig
	etcdClient  *clientv3.Client
	server      *http.Server
	router      *mux.Router
	upgrader    websocket.Upgrader
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
	isRunning   bool
	mu          sync.RWMutex
	wsClients   map[*websocket.Conn]bool
	wsClientsMu sync.RWMutex
	shutdownCh  chan struct{}
}

// GPUInfo represents GPU information for API responses
type GPUInfo struct {
	GPUID    string `json:"gpu_id"`
	UUID     string `json:"uuid"`
	Hostname string `json:"hostname"`
	Device   string `json:"device"`
}

// TelemetryResponse represents API response for telemetry queries
type TelemetryResponse struct {
	DataPoints []models.TelemetryDataV1 `json:"data_points"`
	TotalCount int                      `json:"total_count"`
	HasMore    bool                     `json:"has_more"`
	NextCursor string                   `json:"next_cursor,omitempty"`
}

// ErrorResponse represents API error response
type ErrorResponse struct {
	Error   string `json:"error"`
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// NewEnhancedGateway creates a new enhanced API gateway
func NewEnhancedGateway(cfg *config.CentralizedConfig) (*EnhancedGateway, error) {
	// Create etcd client
	etcdClient, err := clientv3.New(clientv3.Config{
		Endpoints:   cfg.Etcd.Endpoints,
		DialTimeout: cfg.Etcd.DialTimeout,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to etcd: %w", err)
	}

	// Test etcd connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	_, err = etcdClient.Status(ctx, cfg.Etcd.Endpoints[0])
	cancel()
	if err != nil {
		etcdClient.Close()
		return nil, fmt.Errorf("failed to connect to etcd: %w", err)
	}

	ctx, cancel = context.WithCancel(context.Background())

	// Setup WebSocket upgrader
	upgrader := websocket.Upgrader{
		ReadBufferSize:  cfg.Gateway.WebSocket.ReadBufferSize,
		WriteBufferSize: cfg.Gateway.WebSocket.WriteBufferSize,
		CheckOrigin: func(r *http.Request) bool {
			// Allow connections from configured origins
			origin := r.Header.Get("Origin")
			if len(cfg.Gateway.CORSOrigins) == 0 || cfg.Gateway.CORSOrigins[0] == "*" {
				return true
			}
			for _, allowedOrigin := range cfg.Gateway.CORSOrigins {
				if origin == allowedOrigin {
					return true
				}
			}
			return false
		},
	}

	gateway := &EnhancedGateway{
		config:     cfg,
		etcdClient: etcdClient,
		upgrader:   upgrader,
		ctx:        ctx,
		cancel:     cancel,
		wsClients:  make(map[*websocket.Conn]bool),
		shutdownCh: make(chan struct{}),
	}

	// Setup router and routes
	gateway.setupRoutes()

	// Create HTTP server
	gateway.server = &http.Server{
		Addr:           fmt.Sprintf("%s:%d", cfg.Gateway.Host, cfg.Gateway.Port),
		Handler:        gateway.router,
		ReadTimeout:    cfg.Gateway.ReadTimeout,
		WriteTimeout:   cfg.Gateway.WriteTimeout,
		MaxHeaderBytes: cfg.Gateway.MaxHeaderSize,
	}

	logging.Infof("Enhanced gateway initialized on %s:%d", cfg.Gateway.Host, cfg.Gateway.Port)
	return gateway, nil
}

// setupRoutes sets up all API routes
func (eg *EnhancedGateway) setupRoutes() {
	eg.router = mux.NewRouter()

	// API v1 routes
	api := eg.router.PathPrefix("/api/v1").Subrouter()

	// CORS middleware
	if eg.config.Gateway.EnableCORS {
		eg.router.Use(eg.corsMiddleware)
	}

	// Rate limiting middleware
	if eg.config.Gateway.RateLimit.Enabled {
		eg.router.Use(eg.rateLimitMiddleware)
	}

	// Logging middleware
	eg.router.Use(eg.loggingMiddleware)

	// GPU endpoints
	api.HandleFunc("/gpus", eg.listGPUs).Methods("GET", "OPTIONS")
	api.HandleFunc("/gpus/{gpu_id}/telemetry", eg.getGPUTelemetry).Methods("GET", "OPTIONS")

	// WebSocket endpoint
	api.HandleFunc("/ws/telemetry", eg.handleWebSocket).Methods("GET")

	// Health check
	api.HandleFunc("/health", eg.healthCheck).Methods("GET")

	// Metrics endpoint
	if eg.config.Observability.Metrics.Enabled {
		eg.router.HandleFunc(eg.config.Observability.Metrics.Path, eg.metricsHandler).Methods("GET")
	}
}

// Start starts the enhanced gateway
func (eg *EnhancedGateway) Start() error {
	eg.mu.Lock()
	defer eg.mu.Unlock()

	if eg.isRunning {
		return fmt.Errorf("gateway is already running")
	}

	// Set up signal handling for graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)

	// Start WebSocket client manager
	if eg.config.Gateway.WebSocket.Enabled {
		eg.wg.Add(1)
		go eg.wsClientManager()
	}

	eg.isRunning = true

	// Handle graceful shutdown
	go func() {
		<-sigCh
		logging.Infof("Received shutdown signal, starting graceful shutdown...")
		eg.Stop()
	}()

	// Start HTTP server
	go func() {
		logging.Infof("Starting enhanced gateway server on %s", eg.server.Addr)
		if err := eg.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logging.Errorf("Gateway server error: %v", err)
		}
	}()

	logging.Infof("Enhanced gateway started successfully")
	return nil
}

// Stop stops the gateway gracefully
func (eg *EnhancedGateway) Stop() error {
	eg.mu.Lock()
	defer eg.mu.Unlock()

	if !eg.isRunning {
		return nil
	}

	logging.Infof("Stopping enhanced gateway...")

	// Cancel context
	eg.cancel()

	// Shutdown HTTP server
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := eg.server.Shutdown(ctx); err != nil {
		logging.Errorf("Failed to shutdown server gracefully: %v", err)
	}

	// Close all WebSocket connections
	eg.wsClientsMu.Lock()
	for conn := range eg.wsClients {
		conn.Close()
		delete(eg.wsClients, conn)
	}
	eg.wsClientsMu.Unlock()

	// Wait for all goroutines to finish
	done := make(chan struct{})
	go func() {
		eg.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		logging.Infof("All gateway workers stopped gracefully")
	case <-time.After(30 * time.Second):
		logging.Warnf("Timeout waiting for gateway workers to stop")
	}

	// Close etcd client
	if eg.etcdClient != nil {
		eg.etcdClient.Close()
	}

	eg.isRunning = false
	close(eg.shutdownCh)

	logging.Infof("Enhanced gateway stopped")
	return nil
}

// listGPUs handles GET /api/v1/gpus
func (eg *EnhancedGateway) listGPUs(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(eg.ctx, 30*time.Second)
	defer cancel()

	// Get all telemetry data keys to extract GPU information (Nexus format)
	resp, err := eg.etcdClient.Get(ctx, "/telemetry/clusters/", clientv3.WithPrefix(), clientv3.WithKeysOnly())
	if err != nil {
		eg.writeError(w, http.StatusInternalServerError, "Failed to retrieve GPU data", err)
		return
	}

	// Extract unique GPUs from keys
	gpuMap := make(map[string]*GPUInfo)
	for _, kv := range resp.Kvs {
		key := string(kv.Key)
		// Key format: /telemetry/clusters/{cluster_id}/hosts/{hostname}/gpus/{gpu_id}/data/{data_key}
		parts := strings.Split(key, "/")
		if len(parts) >= 7 && parts[5] == "gpus" && parts[7] == "data" {
			hostname := parts[4]
			gpuID := parts[6]

			if _, exists := gpuMap[hostname+":"+gpuID]; !exists {
				gpuMap[hostname+":"+gpuID] = &GPUInfo{
					GPUID:    gpuID,
					Hostname: hostname,
					Device:   fmt.Sprintf("gpu%s", gpuID),
				}
			}
		}
	}

	// Convert map to slice
	gpus := make([]*GPUInfo, 0, len(gpuMap))
	for _, gpu := range gpuMap {
		gpus = append(gpus, gpu)
	}

	eg.writeJSON(w, http.StatusOK, gpus)
}

// getGPUTelemetry handles GET /api/v1/gpus/{gpu_id}/telemetry
func (eg *EnhancedGateway) getGPUTelemetry(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	gpuID := vars["gpu_id"]

	ctx, cancel := context.WithTimeout(eg.ctx, 30*time.Second)
	defer cancel()

	// Parse query parameters
	query := r.URL.Query()

	// Pagination
	limit := eg.config.Gateway.Pagination.DefaultLimit
	if limitStr := query.Get("limit"); limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil {
			if parsedLimit > 0 && parsedLimit <= eg.config.Gateway.Pagination.MaxLimit {
				limit = parsedLimit
			}
		}
	}

	// Time filters
	var startTime, endTime *time.Time
	if startTimeStr := query.Get("start_time"); startTimeStr != "" {
		if t, err := time.Parse(time.RFC3339, startTimeStr); err == nil {
			startTime = &t
		}
	}
	if endTimeStr := query.Get("end_time"); endTimeStr != "" {
		if t, err := time.Parse(time.RFC3339, endTimeStr); err == nil {
			endTime = &t
		}
	}

	// Hostname filter (optional)
	hostname := query.Get("hostname")

	// Build search prefix (Nexus format)
	searchPrefix := "/telemetry/clusters/"
	if hostname != "" {
		// Search for specific host and GPU
		searchPrefix = fmt.Sprintf("/telemetry/clusters/local-cluster/hosts/%s/gpus/%s/data/", hostname, gpuID)
	}

	// Get telemetry data from etcd
	resp, err := eg.etcdClient.Get(ctx, searchPrefix,
		clientv3.WithPrefix(),
		clientv3.WithSort(clientv3.SortByKey, clientv3.SortDescend),
		clientv3.WithLimit(int64(limit*2))) // Get more to allow for filtering
	if err != nil {
		eg.writeError(w, http.StatusInternalServerError, "Failed to retrieve telemetry data", err)
		return
	}

	var telemetryData []models.TelemetryDataV1
	count := 0

	for _, kv := range resp.Kvs {
		if count >= limit {
			break
		}

		key := string(kv.Key)

		// Filter by GPU ID if hostname not specified
		if hostname == "" {
			parts := strings.Split(key, "/")
			// Key format: /telemetry/clusters/{cluster_id}/hosts/{hostname}/gpus/{gpu_id}/data/{data_key}
			if len(parts) < 7 || parts[6] != gpuID || parts[7] != "data" {
				continue
			}
		}

		var data models.TelemetryDataV1
		if err := json.Unmarshal(kv.Value, &data); err != nil {
			logging.Errorf("Failed to unmarshal telemetry data from key %s: %v", key, err)
			continue
		}

		// Apply time filters
		if startTime != nil && data.Timestamp.Before(*startTime) {
			continue
		}
		if endTime != nil && data.Timestamp.After(*endTime) {
			continue
		}

		telemetryData = append(telemetryData, data)
		count++
	}

	// Build response
	response := TelemetryResponse{
		DataPoints: telemetryData,
		TotalCount: len(telemetryData),
		HasMore:    len(resp.Kvs) > limit,
	}

	eg.writeJSON(w, http.StatusOK, response)
}

// handleWebSocket handles WebSocket connections for real-time telemetry
func (eg *EnhancedGateway) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	if !eg.config.Gateway.WebSocket.Enabled {
		eg.writeError(w, http.StatusServiceUnavailable, "WebSocket is disabled", nil)
		return
	}

	conn, err := eg.upgrader.Upgrade(w, r, nil)
	if err != nil {
		logging.Errorf("Failed to upgrade WebSocket connection: %v", err)
		return
	}

	// Check connection limit
	eg.wsClientsMu.Lock()
	if len(eg.wsClients) >= eg.config.Gateway.WebSocket.MaxConnections {
		eg.wsClientsMu.Unlock()
		conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseServiceRestart, "Connection limit reached"))
		conn.Close()
		return
	}

	eg.wsClients[conn] = true
	eg.wsClientsMu.Unlock()

	logging.Infof("New WebSocket connection established (total: %d)", len(eg.wsClients))

	// Handle connection in goroutine
	go eg.handleWSConnection(conn)
}

// handleWSConnection handles individual WebSocket connection
func (eg *EnhancedGateway) handleWSConnection(conn *websocket.Conn) {
	defer func() {
		eg.wsClientsMu.Lock()
		delete(eg.wsClients, conn)
		eg.wsClientsMu.Unlock()
		conn.Close()
		logging.Infof("WebSocket connection closed (remaining: %d)", len(eg.wsClients))
	}()

	// Set timeouts
	conn.SetReadDeadline(time.Now().Add(eg.config.Gateway.WebSocket.PongTimeout))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(eg.config.Gateway.WebSocket.PongTimeout))
		return nil
	})

	// Start ping ticker
	ticker := time.NewTicker(eg.config.Gateway.WebSocket.PingInterval)
	defer ticker.Stop()

	// Ping loop
	go func() {
		for {
			select {
			case <-ticker.C:
				conn.SetWriteDeadline(time.Now().Add(eg.config.Gateway.WebSocket.WriteTimeout))
				if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
					return
				}
			case <-eg.ctx.Done():
				return
			}
		}
	}()

	// Read messages (mainly for close detection)
	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				logging.Errorf("WebSocket error: %v", err)
			}
			break
		}
	}
}

// wsClientManager manages WebSocket clients and broadcasts updates
func (eg *EnhancedGateway) wsClientManager() {
	defer eg.wg.Done()

	// Watch for telemetry data changes
	watchChan := eg.etcdClient.Watch(eg.ctx, "/telemetry/data/", clientv3.WithPrefix())

	for watchResp := range watchChan {
		if watchResp.Err() != nil {
			logging.Errorf("WebSocket watch error: %v", watchResp.Err())
			continue
		}

		for _, event := range watchResp.Events {
			if event.Type == clientv3.EventTypePut {
				var data models.TelemetryDataV1
				if err := json.Unmarshal(event.Kv.Value, &data); err != nil {
					continue
				}

				// Broadcast to all WebSocket clients
				eg.broadcastToWebSockets(data)
			}
		}
	}
}

// broadcastToWebSockets broadcasts telemetry data to all connected WebSocket clients
func (eg *EnhancedGateway) broadcastToWebSockets(data models.TelemetryDataV1) {
	eg.wsClientsMu.RLock()
	clients := make([]*websocket.Conn, 0, len(eg.wsClients))
	for conn := range eg.wsClients {
		clients = append(clients, conn)
	}
	eg.wsClientsMu.RUnlock()

	message, err := json.Marshal(data)
	if err != nil {
		return
	}

	for _, conn := range clients {
		conn.SetWriteDeadline(time.Now().Add(eg.config.Gateway.WebSocket.WriteTimeout))
		if err := conn.WriteMessage(websocket.TextMessage, message); err != nil {
			// Remove failed connection
			eg.wsClientsMu.Lock()
			delete(eg.wsClients, conn)
			eg.wsClientsMu.Unlock()
			conn.Close()
		}
	}
}

// healthCheck handles GET /api/v1/health
func (eg *EnhancedGateway) healthCheck(w http.ResponseWriter, r *http.Request) {
	health := map[string]interface{}{
		"status":    "healthy",
		"timestamp": time.Now(),
		"version":   "1.0.0",
	}

	// Check etcd connectivity
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := eg.etcdClient.Status(ctx, eg.config.Etcd.Endpoints[0]); err != nil {
		health["status"] = "unhealthy"
		health["etcd_error"] = err.Error()
		eg.writeJSON(w, http.StatusServiceUnavailable, health)
		return
	}

	health["websocket_connections"] = len(eg.wsClients)
	eg.writeJSON(w, http.StatusOK, health)
}

// metricsHandler handles GET /metrics
func (eg *EnhancedGateway) metricsHandler(w http.ResponseWriter, r *http.Request) {
	metrics := map[string]interface{}{
		"websocket_connections": len(eg.wsClients),
		"uptime_seconds":        time.Since(time.Now()).Seconds(), // This would be tracked properly
	}

	eg.writeJSON(w, http.StatusOK, metrics)
}

// Middleware functions

func (eg *EnhancedGateway) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(eg.config.Gateway.CORSOrigins) > 0 && eg.config.Gateway.CORSOrigins[0] != "*" {
			origin := r.Header.Get("Origin")
			for _, allowedOrigin := range eg.config.Gateway.CORSOrigins {
				if origin == allowedOrigin {
					w.Header().Set("Access-Control-Allow-Origin", origin)
					break
				}
			}
		} else {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		}

		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (eg *EnhancedGateway) rateLimitMiddleware(next http.Handler) http.Handler {
	// Simple in-memory rate limiter (in production, use Redis or similar)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// TODO: Implement proper rate limiting
		next.ServeHTTP(w, r)
	})
}

func (eg *EnhancedGateway) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		logging.Infof("%s %s %v", r.Method, r.URL.Path, time.Since(start))
	})
}

// Utility functions

func (eg *EnhancedGateway) writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func (eg *EnhancedGateway) writeError(w http.ResponseWriter, status int, message string, err error) {
	errorResponse := ErrorResponse{
		Error:   message,
		Code:    status,
		Message: message,
	}

	if err != nil {
		logging.Errorf("%s: %v", message, err)
		errorResponse.Message = fmt.Sprintf("%s: %v", message, err)
	}

	eg.writeJSON(w, status, errorResponse)
}

// IsRunning returns whether the gateway is running
func (eg *EnhancedGateway) IsRunning() bool {
	eg.mu.RLock()
	defer eg.mu.RUnlock()
	return eg.isRunning
}

// WaitForShutdown waits for the gateway to shutdown
func (eg *EnhancedGateway) WaitForShutdown() {
	<-eg.shutdownCh
}
