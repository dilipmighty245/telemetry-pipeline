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

// ServiceInfo represents a service instance
type ServiceInfo struct {
	ID           string            `json:"id"`
	Type         string            `json:"type"`
	Address      string            `json:"address"`
	Port         int               `json:"port"`
	Metadata     map[string]string `json:"metadata"`
	RegisteredAt time.Time         `json:"registered_at"`
	Health       string            `json:"health"`
	Version      string            `json:"version"`
}

// ServiceRegistry manages service registration and discovery
type ServiceRegistry struct {
	client     *clientv3.Client
	leaseID    clientv3.LeaseID
	serviceKey string
	ttl        int64
	mu         sync.RWMutex
}

// NewServiceRegistry creates a new service registry
func NewServiceRegistry(client *clientv3.Client, ttl int64) *ServiceRegistry {
	return &ServiceRegistry{
		client: client,
		ttl:    ttl,
	}
}

// Register registers a service instance with heartbeat
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

// Discover finds all instances of a service type
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

// WatchServices watches for changes in service instances
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

// UpdateHealth updates the health status of the registered service
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

// Deregister removes the service from registry
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

// GetServiceByID finds a specific service instance by ID
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
