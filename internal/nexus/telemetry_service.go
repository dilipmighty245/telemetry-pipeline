package nexus

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	log "github.com/sirupsen/logrus"
	clientv3 "go.etcd.io/etcd/client/v3"
)

// TelemetryService integrates telemetry pipeline with Nexus-like patterns
// This uses etcd directly instead of the full Nexus framework for simplicity
type TelemetryService struct {
	etcdClient *clientv3.Client
	config     *ServiceConfig
	ctx        context.Context
	cancel     context.CancelFunc
}

// ServiceConfig holds configuration for the telemetry service
type ServiceConfig struct {
	EtcdEndpoints  []string
	ClusterID      string
	ServiceID      string
	UpdateInterval time.Duration
	BatchSize      int
	EnableWatchAPI bool
}

// TelemetryCluster represents the root telemetry cluster
type TelemetryCluster struct {
	ClusterID   string            `json:"cluster_id"`
	ClusterName string            `json:"cluster_name"`
	Region      string            `json:"region"`
	Environment string            `json:"environment"`
	Labels      map[string]string `json:"labels"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
	Metadata    ClusterMetadata   `json:"metadata"`
}

// TelemetryHost represents a host in the cluster
type TelemetryHost struct {
	HostID    string            `json:"host_id"`
	Hostname  string            `json:"hostname"`
	IPAddress string            `json:"ip_address"`
	OSVersion string            `json:"os_version"`
	Labels    map[string]string `json:"labels"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
	Status    HostStatus        `json:"status"`
}

// TelemetryGPU represents a GPU device
type TelemetryGPU struct {
	GPUID         string            `json:"gpu_id"`      // Host-specific GPU ID (0, 1, 2, 3...)
	UUID          string            `json:"uuid"`        // Globally unique GPU identifier
	Device        string            `json:"device"`      // Device name (nvidia0, nvidia1...)
	DeviceName    string            `json:"device_name"` // GPU model name
	DriverVersion string            `json:"driver_version"`
	CudaVersion   string            `json:"cuda_version"`
	MemoryTotal   int32             `json:"memory_total_mb"`
	Properties    map[string]string `json:"properties"`
	CreatedAt     time.Time         `json:"created_at"`
	UpdatedAt     time.Time         `json:"updated_at"`
	Status        GPUStatus         `json:"status"`
}

// TelemetryData represents individual GPU telemetry data points
type TelemetryData struct {
	TelemetryID string    `json:"telemetry_id"`
	Timestamp   time.Time `json:"timestamp"`
	GPUID       string    `json:"gpu_id"`     // Host-specific GPU ID (0, 1, 2, 3...)
	UUID        string    `json:"uuid"`       // Globally unique GPU identifier
	Device      string    `json:"device"`     // Device name (nvidia0, nvidia1...)
	ModelName   string    `json:"model_name"` // GPU model name
	Hostname    string    `json:"hostname"`

	// Performance metrics
	GPUUtilization    float32 `json:"gpu_utilization"`
	MemoryUtilization float32 `json:"memory_utilization"`
	MemoryUsedMB      float32 `json:"memory_used_mb"`
	MemoryFreeMB      float32 `json:"memory_free_mb"`
	Temperature       float32 `json:"temperature"`
	PowerDraw         float32 `json:"power_draw"`
	SMClockMHz        float32 `json:"sm_clock_mhz"`
	MemoryClockMHz    float32 `json:"memory_clock_mhz"`

	// Additional metrics
	CustomMetrics map[string]float32 `json:"custom_metrics"`

	// Processing metadata
	CollectedAt time.Time `json:"collected_at"`
	ProcessedAt time.Time `json:"processed_at"`
	CollectorID string    `json:"collector_id"`
	BatchID     string    `json:"batch_id"`
}

// GPUStatus represents current GPU status
type GPUStatus struct {
	State              string    `json:"state"` // active, idle, error, maintenance
	UtilizationPercent float32   `json:"utilization_percent"`
	MemoryUsedPercent  float32   `json:"memory_used_percent"`
	TemperatureCelsius float32   `json:"temperature_celsius"`
	PowerDrawWatts     float32   `json:"power_draw_watts"`
	Healthy            bool      `json:"healthy"`
	LastUpdated        time.Time `json:"last_updated"`
}

// HostStatus represents current host status
type HostStatus struct {
	State        string            `json:"state"` // active, inactive, maintenance
	Healthy      bool              `json:"healthy"`
	LastSeen     time.Time         `json:"last_seen"`
	HealthChecks map[string]string `json:"health_checks"`
}

// ClusterMetadata represents cluster metadata
type ClusterMetadata struct {
	TotalHosts    int32             `json:"total_hosts"`
	TotalGPUs     int32             `json:"total_gpus"`
	ActiveHosts   int32             `json:"active_hosts"`
	ActiveGPUs    int32             `json:"active_gpus"`
	LastUpdated   time.Time         `json:"last_updated"`
	Configuration map[string]string `json:"configuration"`
}

// NewTelemetryService creates a new Nexus-style telemetry service
func NewTelemetryService(config *ServiceConfig) (*TelemetryService, error) {
	if config == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}

	// Create etcd client
	etcdClient, err := clientv3.New(clientv3.Config{
		Endpoints:   config.EtcdEndpoints,
		DialTimeout: 10 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create etcd client: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	service := &TelemetryService{
		etcdClient: etcdClient,
		config:     config,
		ctx:        ctx,
		cancel:     cancel,
	}

	// Initialize cluster node in etcd
	if err := service.initializeCluster(); err != nil {
		service.Close()
		return nil, fmt.Errorf("failed to initialize cluster: %w", err)
	}

	log.Infof("Nexus-style telemetry service initialized for cluster: %s", config.ClusterID)
	return service, nil
}

// initializeCluster creates or updates the cluster node in etcd
func (ts *TelemetryService) initializeCluster() error {
	cluster := &TelemetryCluster{
		ClusterID:   ts.config.ClusterID,
		ClusterName: fmt.Sprintf("Telemetry Cluster %s", ts.config.ClusterID),
		Region:      "default",
		Environment: "production",
		Labels: map[string]string{
			"service": "telemetry-pipeline",
			"version": "v1.0.0",
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Metadata: ClusterMetadata{
			TotalHosts:  0,
			TotalGPUs:   0,
			ActiveHosts: 0,
			ActiveGPUs:  0,
			LastUpdated: time.Now(),
			Configuration: map[string]string{
				"batch_size":      fmt.Sprintf("%d", ts.config.BatchSize),
				"update_interval": ts.config.UpdateInterval.String(),
				"watch_api":       fmt.Sprintf("%t", ts.config.EnableWatchAPI),
			},
		},
	}

	// Store cluster data in etcd
	key := fmt.Sprintf("/telemetry/clusters/%s", ts.config.ClusterID)
	data, err := json.Marshal(cluster)
	if err != nil {
		return fmt.Errorf("failed to marshal cluster data: %w", err)
	}

	_, err = ts.etcdClient.Put(ts.ctx, key, string(data))
	if err != nil {
		return fmt.Errorf("failed to store cluster data: %w", err)
	}

	log.Infof("Cluster initialized in etcd: %s", ts.config.ClusterID)
	return nil
}

// RegisterHost registers a new host in the telemetry cluster
func (ts *TelemetryService) RegisterHost(hostInfo *TelemetryHost) error {
	hostInfo.CreatedAt = time.Now()
	hostInfo.UpdatedAt = time.Now()

	// Set initial status
	hostInfo.Status = HostStatus{
		State:    "active",
		Healthy:  true,
		LastSeen: time.Now(),
		HealthChecks: map[string]string{
			"ping": "ok",
		},
	}

	// Store host data in etcd
	key := fmt.Sprintf("/telemetry/clusters/%s/hosts/%s", ts.config.ClusterID, hostInfo.HostID)
	data, err := json.Marshal(hostInfo)
	if err != nil {
		return fmt.Errorf("failed to marshal host data: %w", err)
	}

	_, err = ts.etcdClient.Put(ts.ctx, key, string(data))
	if err != nil {
		return fmt.Errorf("failed to register host: %w", err)
	}

	// Update cluster metadata
	if err := ts.updateClusterMetadata(); err != nil {
		log.Warnf("Failed to update cluster metadata: %v", err)
	}

	log.Infof("Host registered: %s (%s)", hostInfo.HostID, hostInfo.Hostname)
	return nil
}

// RegisterGPU registers a new GPU device for a host
func (ts *TelemetryService) RegisterGPU(hostID string, gpuInfo *TelemetryGPU) error {
	gpuInfo.CreatedAt = time.Now()
	gpuInfo.UpdatedAt = time.Now()

	// Set initial status
	gpuInfo.Status = GPUStatus{
		State:              "active",
		UtilizationPercent: 0.0,
		MemoryUsedPercent:  0.0,
		TemperatureCelsius: 0.0,
		PowerDrawWatts:     0.0,
		Healthy:            true,
		LastUpdated:        time.Now(),
	}

	// Store GPU data in etcd
	key := fmt.Sprintf("/telemetry/clusters/%s/hosts/%s/gpus/%s", ts.config.ClusterID, hostID, gpuInfo.GPUID)
	data, err := json.Marshal(gpuInfo)
	if err != nil {
		return fmt.Errorf("failed to marshal GPU data: %w", err)
	}

	_, err = ts.etcdClient.Put(ts.ctx, key, string(data))
	if err != nil {
		return fmt.Errorf("failed to register GPU: %w", err)
	}

	// Update cluster metadata
	if err := ts.updateClusterMetadata(); err != nil {
		log.Warnf("Failed to update cluster metadata: %v", err)
	}

	log.Infof("GPU registered: %s (%s) for host %s", gpuInfo.GPUID, gpuInfo.DeviceName, hostID)
	return nil
}

// StoreTelemetryData stores GPU telemetry data in etcd
func (ts *TelemetryService) StoreTelemetryData(hostID, gpuID string, telemetryData *TelemetryData) error {
	telemetryData.CollectedAt = time.Now()
	telemetryData.ProcessedAt = time.Now()

	// Store the telemetry data in etcd
	key := fmt.Sprintf("/telemetry/clusters/%s/hosts/%s/gpus/%s/data/%s",
		ts.config.ClusterID, hostID, gpuID, telemetryData.TelemetryID)

	data, err := json.Marshal(telemetryData)
	if err != nil {
		return fmt.Errorf("failed to marshal telemetry data: %w", err)
	}

	_, err = ts.etcdClient.Put(ts.ctx, key, string(data))
	if err != nil {
		return fmt.Errorf("failed to store telemetry data: %w", err)
	}

	// Update GPU status
	if err := ts.updateGPUStatus(hostID, gpuID, telemetryData); err != nil {
		log.Warnf("Failed to update GPU status: %v", err)
	}

	return nil
}

// updateGPUStatus updates the current status of a GPU based on latest telemetry
func (ts *TelemetryService) updateGPUStatus(hostID, gpuID string, data *TelemetryData) error {
	status := &GPUStatus{
		State:              "active",
		UtilizationPercent: data.GPUUtilization,
		MemoryUsedPercent:  data.MemoryUtilization,
		TemperatureCelsius: data.Temperature,
		PowerDrawWatts:     data.PowerDraw,
		Healthy:            data.Temperature < 85.0 && data.GPUUtilization >= 0, // Simple health check
		LastUpdated:        time.Now(),
	}

	// Update the GPU record with new status
	gpuKey := fmt.Sprintf("/telemetry/clusters/%s/hosts/%s/gpus/%s", ts.config.ClusterID, hostID, gpuID)

	// Get existing GPU data
	resp, err := ts.etcdClient.Get(ts.ctx, gpuKey)
	if err != nil {
		return fmt.Errorf("failed to get GPU data: %w", err)
	}

	if len(resp.Kvs) == 0 {
		return fmt.Errorf("GPU not found: %s", gpuID)
	}

	var gpu TelemetryGPU
	if err := json.Unmarshal(resp.Kvs[0].Value, &gpu); err != nil {
		return fmt.Errorf("failed to unmarshal GPU data: %w", err)
	}

	// Update status and timestamp
	gpu.Status = *status
	gpu.UpdatedAt = time.Now()

	// Store updated GPU data
	updatedData, err := json.Marshal(gpu)
	if err != nil {
		return fmt.Errorf("failed to marshal updated GPU data: %w", err)
	}

	_, err = ts.etcdClient.Put(ts.ctx, gpuKey, string(updatedData))
	if err != nil {
		return fmt.Errorf("failed to update GPU status: %w", err)
	}

	return nil
}

// updateClusterMetadata updates cluster-level statistics
func (ts *TelemetryService) updateClusterMetadata() error {
	// Query current hosts and GPUs count from etcd
	hostsKey := fmt.Sprintf("/telemetry/clusters/%s/hosts/", ts.config.ClusterID)

	hostsResp, err := ts.etcdClient.Get(ts.ctx, hostsKey, clientv3.WithPrefix())
	if err != nil {
		return fmt.Errorf("failed to query hosts: %w", err)
	}

	totalHosts := 0
	totalGPUs := 0
	activeHosts := 0
	activeGPUs := 0

	// Count hosts and their status
	hostMap := make(map[string]bool)
	for _, kv := range hostsResp.Kvs {
		key := string(kv.Key)
		if !isGPUKey(key) && !isTelemetryDataKey(key) {
			var host TelemetryHost
			if err := json.Unmarshal(kv.Value, &host); err != nil {
				continue
			}

			hostMap[host.HostID] = true
			if host.Status.Healthy {
				activeHosts++
			}
		}
	}
	totalHosts = len(hostMap)

	// Count GPUs
	gpusKey := fmt.Sprintf("/telemetry/clusters/%s/hosts/", ts.config.ClusterID)
	gpusResp, err := ts.etcdClient.Get(ts.ctx, gpusKey, clientv3.WithPrefix())
	if err == nil {
		for _, kv := range gpusResp.Kvs {
			key := string(kv.Key)
			if isGPUKey(key) && !isTelemetryDataKey(key) {
				var gpu TelemetryGPU
				if err := json.Unmarshal(kv.Value, &gpu); err != nil {
					continue
				}

				totalGPUs++
				if gpu.Status.Healthy {
					activeGPUs++
				}
			}
		}
	}

	// Update cluster metadata
	clusterKey := fmt.Sprintf("/telemetry/clusters/%s", ts.config.ClusterID)

	// Get existing cluster data
	clusterResp, err := ts.etcdClient.Get(ts.ctx, clusterKey)
	if err != nil {
		return fmt.Errorf("failed to get cluster data: %w", err)
	}

	if len(clusterResp.Kvs) == 0 {
		return fmt.Errorf("cluster not found: %s", ts.config.ClusterID)
	}

	var cluster TelemetryCluster
	if err := json.Unmarshal(clusterResp.Kvs[0].Value, &cluster); err != nil {
		return fmt.Errorf("failed to unmarshal cluster data: %w", err)
	}

	// Update metadata
	cluster.Metadata = ClusterMetadata{
		TotalHosts:  int32(totalHosts),
		TotalGPUs:   int32(totalGPUs),
		ActiveHosts: int32(activeHosts),
		ActiveGPUs:  int32(activeGPUs),
		LastUpdated: time.Now(),
		Configuration: map[string]string{
			"batch_size":      fmt.Sprintf("%d", ts.config.BatchSize),
			"update_interval": ts.config.UpdateInterval.String(),
			"watch_api":       fmt.Sprintf("%t", ts.config.EnableWatchAPI),
		},
	}
	cluster.UpdatedAt = time.Now()

	// Store updated cluster data
	updatedData, err := json.Marshal(cluster)
	if err != nil {
		return fmt.Errorf("failed to marshal updated cluster data: %w", err)
	}

	_, err = ts.etcdClient.Put(ts.ctx, clusterKey, string(updatedData))
	if err != nil {
		return fmt.Errorf("failed to update cluster metadata: %w", err)
	}

	return nil
}

// GetClusterInfo retrieves cluster information from etcd
func (ts *TelemetryService) GetClusterInfo() (*TelemetryCluster, error) {
	key := fmt.Sprintf("/telemetry/clusters/%s", ts.config.ClusterID)

	resp, err := ts.etcdClient.Get(ts.ctx, key)
	if err != nil {
		return nil, fmt.Errorf("failed to get cluster info: %w", err)
	}

	if len(resp.Kvs) == 0 {
		return nil, fmt.Errorf("cluster not found: %s", ts.config.ClusterID)
	}

	var cluster TelemetryCluster
	if err := json.Unmarshal(resp.Kvs[0].Value, &cluster); err != nil {
		return nil, fmt.Errorf("failed to unmarshal cluster data: %w", err)
	}

	return &cluster, nil
}

// GetHostInfo retrieves host information from etcd
func (ts *TelemetryService) GetHostInfo(hostID string) (*TelemetryHost, error) {
	key := fmt.Sprintf("/telemetry/clusters/%s/hosts/%s", ts.config.ClusterID, hostID)

	resp, err := ts.etcdClient.Get(ts.ctx, key)
	if err != nil {
		return nil, fmt.Errorf("failed to get host info: %w", err)
	}

	if len(resp.Kvs) == 0 {
		return nil, fmt.Errorf("host not found: %s", hostID)
	}

	var host TelemetryHost
	if err := json.Unmarshal(resp.Kvs[0].Value, &host); err != nil {
		return nil, fmt.Errorf("failed to unmarshal host data: %w", err)
	}

	return &host, nil
}

// GetGPUTelemetryData retrieves GPU telemetry data from etcd with filtering
func (ts *TelemetryService) GetGPUTelemetryData(hostID, gpuID string, startTime, endTime *time.Time, limit int) ([]*TelemetryData, error) {
	key := fmt.Sprintf("/telemetry/clusters/%s/hosts/%s/gpus/%s/data/", ts.config.ClusterID, hostID, gpuID)

	resp, err := ts.etcdClient.Get(ts.ctx, key, clientv3.WithPrefix())
	if err != nil {
		return nil, fmt.Errorf("failed to get GPU telemetry data: %w", err)
	}

	var telemetryData []*TelemetryData
	count := 0

	for _, kv := range resp.Kvs {
		if limit > 0 && count >= limit {
			break
		}

		var data TelemetryData
		if err := json.Unmarshal(kv.Value, &data); err != nil {
			log.Warnf("Failed to unmarshal telemetry data: %v", err)
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
		count++
	}

	return telemetryData, nil
}

// WatchTelemetryChanges sets up a watch for telemetry data changes using etcd watch
func (ts *TelemetryService) WatchTelemetryChanges(callback func(string, []byte, string)) error {
	if !ts.config.EnableWatchAPI {
		return fmt.Errorf("watch API is disabled")
	}

	watchKey := fmt.Sprintf("/telemetry/clusters/%s/", ts.config.ClusterID)

	go func() {
		rch := ts.etcdClient.Watch(ts.ctx, watchKey, clientv3.WithPrefix())

		for wresp := range rch {
			for _, ev := range wresp.Events {
				eventType := "unknown"
				switch ev.Type {
				case clientv3.EventTypePut:
					if ev.IsCreate() {
						eventType = "create"
					} else {
						eventType = "update"
					}
				case clientv3.EventTypeDelete:
					eventType = "delete"
				}

				if callback != nil {
					callback(eventType, ev.Kv.Value, string(ev.Kv.Key))
				}
			}
		}
	}()

	log.Infof("Watch API enabled for cluster: %s", ts.config.ClusterID)
	return nil
}

// Start starts the telemetry service
func (ts *TelemetryService) Start() error {
	log.Info("Nexus-style telemetry service started")
	return nil
}

// Stop stops the telemetry service
func (ts *TelemetryService) Stop() error {
	return ts.Close()
}

// Close closes the telemetry service and cleans up resources
func (ts *TelemetryService) Close() error {
	ts.cancel()

	if ts.etcdClient != nil {
		ts.etcdClient.Close()
	}

	log.Info("Nexus-style telemetry service closed")
	return nil
}

// Helper functions

// isGPUKey checks if an etcd key represents a GPU entry
func isGPUKey(key string) bool {
	if len(key) == 0 {
		return false
	}

	// Check if key ends with '/'
	if key[len(key)-1] == '/' {
		return false
	}

	// Check if key contains "/gpus/"
	if !contains(key, "/gpus/") {
		return false
	}

	// Check if key doesn't end with "/data" (original logic was: key[len(key)-36:] != "/data" || len(key) < 36)
	// This means: if key is less than 36 chars OR doesn't end with "/data", then continue
	// Inversely: if key is >= 36 chars AND ends with "/data", then return false
	if len(key) >= 5 && key[len(key)-5:] == "/data" {
		return false
	}

	return true
}

// isTelemetryDataKey checks if an etcd key represents telemetry data
func isTelemetryDataKey(key string) bool {
	return contains(key, "/data/")
}

// contains checks if a string contains a substring
func contains(s, substr string) bool {
	// Empty substring should return false based on test expectations
	if len(substr) == 0 {
		return false
	}

	// Empty string cannot contain non-empty substring
	if len(s) == 0 {
		return false
	}

	return indexOfSubstring(s, substr) >= 0
}

// indexOfSubstring finds the index of a substring in a string
func indexOfSubstring(s, substr string) int {
	// Empty substring should return 0 (found at beginning) based on test expectations
	if len(substr) == 0 {
		return 0
	}

	// Empty string cannot contain non-empty substring
	if len(s) == 0 {
		return -1
	}

	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
