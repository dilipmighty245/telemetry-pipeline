// Package discovery provides service registration and discovery functionality using etcd.
// It enables services to register themselves, discover other services, and maintain
// health status information in a distributed system.
package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/dilipmighty245/telemetry-pipeline/pkg/logging"
	clientv3 "go.etcd.io/etcd/client/v3"
)

// ServiceInfo represents a service instance in the distributed system.
// It contains all the necessary information for service discovery and health monitoring.
type ServiceInfo struct {
	ID           string            `json:"id"`            // Unique identifier for the service instance
	Type         string            `json:"type"`          // Type/category of the service (e.g., "collector", "streamer")
	Address      string            `json:"address"`       // Network address where the service is accessible
	Port         int               `json:"port"`          // Port number the service is listening on
	Metadata     map[string]string `json:"metadata"`      // Additional key-value metadata about the service
	RegisteredAt time.Time         `json:"registered_at"` // Timestamp when the service was registered
	Health       string            `json:"health"`        // Current health status ("healthy", "unhealthy", etc.)
	Version      string            `json:"version"`       // Version of the service
}

// ServiceRegistry manages service registration and discovery using etcd as the backend.
// It provides functionality for services to register themselves, discover other services,
// and maintain health status with automatic lease management and heartbeats.
type ServiceRegistry struct {
	client     *clientv3.Client // etcd client for backend operations
	leaseID    clientv3.LeaseID // Lease ID for the registered service
	serviceKey string           // etcd key where this service is registered
	ttl        int64            // Time-to-live for the service lease in seconds
	mu         sync.RWMutex     // Mutex for thread-safe access
}

// NewServiceRegistry creates a new service registry with the provided etcd client and TTL.
// The TTL determines how long a service registration remains valid without heartbeat updates.
//
// Parameters:
//   - client: An initialized etcd client for backend operations
//   - ttl: Time-to-live for service registrations in seconds
//
// Returns:
//   - *ServiceRegistry: A new service registry instance
func NewServiceRegistry(client *clientv3.Client, ttl int64) *ServiceRegistry {
	return &ServiceRegistry{
		client: client,
		ttl:    ttl,
	}
}

// Register registers a service instance with automatic heartbeat management.
// It creates a lease, stores the service information in etcd, and starts a background
// goroutine to maintain the lease through periodic heartbeats.
//
// Parameters:
//   - ctx: Context for the registration operation
//   - service: Service information to register
//
// Returns:
//   - error: nil on success, error describing the failure otherwise
func (sr *ServiceRegistry) Register(ctx context.Context, service ServiceInfo) error {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	// Create lease for heartbeat
	lease, err := sr.client.Grant(ctx, sr.ttl)
	if err != nil {
		return fmt.Errorf("failed to create lease: %w", err)
	}
	sr.leaseID = lease.ID

	// Set registration time
	service.RegisteredAt = time.Now()
	if service.Health == "" {
		service.Health = "healthy"
	}

	// Serialize service info
	data, err := json.Marshal(service)
	if err != nil {
		return fmt.Errorf("failed to serialize service info: %w", err)
	}

	// Register service
	sr.serviceKey = fmt.Sprintf("/services/%s/%s", service.Type, service.ID)
	_, err = sr.client.Put(ctx, sr.serviceKey, string(data), clientv3.WithLease(sr.leaseID))
	if err != nil {
		return fmt.Errorf("failed to register service: %w", err)
	}

	// Keep lease alive
	ch, err := sr.client.KeepAlive(ctx, sr.leaseID)
	if err != nil {
		return fmt.Errorf("failed to start keep alive: %w", err)
	}

	go func() {
		for ka := range ch {
			if ka == nil {
				logging.Warnf("Service %s lease expired", service.ID)
				return // Lease expired or context cancelled
			}
			logging.Debugf("Service %s heartbeat: TTL=%d", service.ID, ka.TTL)
		}
	}()

	logging.Infof("Service %s (%s) registered successfully", service.ID, service.Type)
	return nil
}

// Discover finds all registered instances of a specific service type.
// It queries etcd for all services under the specified service type prefix
// and returns their information.
//
// Parameters:
//   - ctx: Context for the discovery operation
//   - serviceType: The type of services to discover (e.g., "collector", "streamer")
//
// Returns:
//   - []ServiceInfo: A slice of discovered service instances
//   - error: nil on success, error describing the failure otherwise
func (sr *ServiceRegistry) Discover(ctx context.Context, serviceType string) ([]ServiceInfo, error) {
	resp, err := sr.client.Get(ctx, fmt.Sprintf("/services/%s/", serviceType),
		clientv3.WithPrefix())
	if err != nil {
		return nil, fmt.Errorf("failed to discover services: %w", err)
	}

	var services []ServiceInfo
	for _, kv := range resp.Kvs {
		var service ServiceInfo
		if err := json.Unmarshal(kv.Value, &service); err == nil {
			services = append(services, service)
		} else {
			logging.Errorf("Failed to unmarshal service from key %s: %v", kv.Key, err)
		}
	}

	logging.Debugf("Discovered %d instances of service type %s", len(services), serviceType)
	return services, nil
}

// WatchServices watches for changes in service instances of a specific type.
// It returns a channel that receives updated service lists whenever services
// are registered, deregistered, or their information changes.
//
// Parameters:
//   - ctx: Context for the watch operation
//   - serviceType: The type of services to watch
//
// Returns:
//   - <-chan []ServiceInfo: A receive-only channel for service updates
func (sr *ServiceRegistry) WatchServices(ctx context.Context, serviceType string) <-chan []ServiceInfo {
	servicesChan := make(chan []ServiceInfo, 10)

	go func() {
		defer close(servicesChan)

		// Initial list
		services, err := sr.Discover(ctx, serviceType)
		if err == nil {
			servicesChan <- services
		}

		// Watch for changes
		watchChan := sr.client.Watch(ctx, fmt.Sprintf("/services/%s/", serviceType),
			clientv3.WithPrefix())

		for watchResp := range watchChan {
			if watchResp.Err() != nil {
				logging.Errorf("Watch error for service type %s: %v", serviceType, watchResp.Err())
				continue
			}

			// Refresh service list on any change
			if services, err := sr.Discover(ctx, serviceType); err == nil {
				select {
				case servicesChan <- services:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return servicesChan
}

// UpdateHealth updates the health status of the currently registered service.
// The service must be registered before calling this method.
//
// Parameters:
//   - ctx: Context for the update operation
//   - health: New health status (e.g., "healthy", "unhealthy", "degraded")
//
// Returns:
//   - error: nil on success, error describing the failure otherwise
func (sr *ServiceRegistry) UpdateHealth(ctx context.Context, health string) error {
	sr.mu.RLock()
	serviceKey := sr.serviceKey
	sr.mu.RUnlock()

	if serviceKey == "" {
		return fmt.Errorf("service not registered")
	}

	// Get current service info
	resp, err := sr.client.Get(ctx, serviceKey)
	if err != nil {
		return fmt.Errorf("failed to get service info: %w", err)
	}

	if len(resp.Kvs) == 0 {
		return fmt.Errorf("service not found")
	}

	var service ServiceInfo
	if err := json.Unmarshal(resp.Kvs[0].Value, &service); err != nil {
		return fmt.Errorf("failed to unmarshal service info: %w", err)
	}

	// Update health
	service.Health = health

	// Serialize and update
	data, err := json.Marshal(service)
	if err != nil {
		return fmt.Errorf("failed to serialize service info: %w", err)
	}

	_, err = sr.client.Put(ctx, serviceKey, string(data), clientv3.WithLease(sr.leaseID))
	if err != nil {
		return fmt.Errorf("failed to update service health: %w", err)
	}

	logging.Debugf("Updated service health to %s", health)
	return nil
}

// Deregister removes the currently registered service from the registry.
// It revokes the lease and explicitly deletes the service key from etcd.
// This method is safe to call multiple times or on unregistered services.
//
// Parameters:
//   - ctx: Context for the deregistration operation
//
// Returns:
//   - error: Always returns nil (errors are logged but not returned)
func (sr *ServiceRegistry) Deregister(ctx context.Context) error {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	if sr.serviceKey == "" {
		return nil // Not registered
	}

	// Revoke lease (this will automatically remove the service)
	if sr.leaseID != 0 {
		_, err := sr.client.Revoke(ctx, sr.leaseID)
		if err != nil {
			logging.Errorf("Failed to revoke lease: %v", err)
		}
	}

	// Also explicitly delete the key
	_, err := sr.client.Delete(ctx, sr.serviceKey)
	if err != nil {
		logging.Errorf("Failed to delete service key: %v", err)
	}

	logging.Infof("Service deregistered: %s", sr.serviceKey)
	sr.serviceKey = ""
	sr.leaseID = 0

	return nil
}

// GetServiceByID finds a specific service instance by its ID and type.
// This method is useful when you need to get detailed information about
// a specific service instance.
//
// Parameters:
//   - ctx: Context for the operation
//   - serviceType: The type of the service to find
//   - serviceID: The unique ID of the service instance
//
// Returns:
//   - *ServiceInfo: The service information if found, nil otherwise
//   - error: nil on success, error describing the failure otherwise
func (sr *ServiceRegistry) GetServiceByID(ctx context.Context, serviceType, serviceID string) (*ServiceInfo, error) {
	serviceKey := fmt.Sprintf("/services/%s/%s", serviceType, serviceID)

	resp, err := sr.client.Get(ctx, serviceKey)
	if err != nil {
		return nil, fmt.Errorf("failed to get service: %w", err)
	}

	if len(resp.Kvs) == 0 {
		return nil, fmt.Errorf("service not found: %s/%s", serviceType, serviceID)
	}

	var service ServiceInfo
	if err := json.Unmarshal(resp.Kvs[0].Value, &service); err != nil {
		return nil, fmt.Errorf("failed to unmarshal service info: %w", err)
	}

	return &service, nil
}
