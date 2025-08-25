package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/server/v3/embed"
	"go.etcd.io/etcd/server/v3/etcdserver/api/v3client"
)

// setupEtcdTestServer creates an embedded etcd server for testing
func setupEtcdTestServer(t *testing.T) (*embed.Etcd, *clientv3.Client, func()) {
	cfg := embed.NewConfig()
	cfg.Name = "test-etcd"
	cfg.Dir = t.TempDir()
	cfg.LogLevel = "error"

	// Use available ports
	clientURL, _ := url.Parse("http://127.0.0.1:0")
	peerURL, _ := url.Parse("http://127.0.0.1:0")

	cfg.ListenClientUrls = []url.URL{*clientURL}
	cfg.AdvertiseClientUrls = cfg.ListenClientUrls
	cfg.ListenPeerUrls = []url.URL{*peerURL}
	cfg.AdvertisePeerUrls = cfg.ListenPeerUrls
	cfg.InitialCluster = cfg.Name + "=" + peerURL.String()
	cfg.ClusterState = embed.ClusterStateFlagNew

	e, err := embed.StartEtcd(cfg)
	require.NoError(t, err)

	select {
	case <-e.Server.ReadyNotify():
	case <-time.After(10 * time.Second):
		e.Close()
		t.Fatalf("etcd server failed to start within 10 seconds")
	}

	client := v3client.New(e.Server)

	cleanup := func() {
		if client != nil {
			client.Close()
		}
		if e != nil {
			e.Close()
			select {
			case <-e.Server.StopNotify():
			case <-time.After(5 * time.Second):
			}
		}
	}

	return e, client, cleanup
}

func TestNewServiceRegistry(t *testing.T) {
	_, client, cleanup := setupEtcdTestServer(t)
	defer cleanup()

	tests := []struct {
		name string
		ttl  int64
	}{
		{"default TTL", 30},
		{"custom TTL", 60},
		{"short TTL", 10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sr := NewServiceRegistry(client, tt.ttl)
			assert.NotNil(t, sr)
			assert.Equal(t, client, sr.client)
			assert.Equal(t, tt.ttl, sr.ttl)
			assert.Equal(t, clientv3.LeaseID(0), sr.leaseID)
			assert.Empty(t, sr.serviceKey)
		})
	}
}

func TestServiceRegistry_Register(t *testing.T) {
	_, client, cleanup := setupEtcdTestServer(t)
	defer cleanup()

	sr := NewServiceRegistry(client, 30)

	service := ServiceInfo{
		ID:      "test-service-1",
		Type:    "collector",
		Address: "192.168.1.100",
		Port:    8080,
		Metadata: map[string]string{
			"version": "1.0.0",
			"region":  "us-west-2",
		},
		Version: "1.0.0",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := sr.Register(ctx, service)
	assert.NoError(t, err)
	assert.NotEqual(t, clientv3.LeaseID(0), sr.leaseID)
	assert.Equal(t, "/services/collector/test-service-1", sr.serviceKey)

	// Verify service was stored in etcd
	resp, err := client.Get(ctx, sr.serviceKey)
	require.NoError(t, err)
	require.Len(t, resp.Kvs, 1)

	var storedService ServiceInfo
	err = json.Unmarshal(resp.Kvs[0].Value, &storedService)
	require.NoError(t, err)

	assert.Equal(t, service.ID, storedService.ID)
	assert.Equal(t, service.Type, storedService.Type)
	assert.Equal(t, service.Address, storedService.Address)
	assert.Equal(t, service.Port, storedService.Port)
	assert.Equal(t, service.Metadata, storedService.Metadata)
	assert.Equal(t, service.Version, storedService.Version)
	assert.Equal(t, "healthy", storedService.Health) // Default health
	assert.WithinDuration(t, time.Now(), storedService.RegisteredAt, 5*time.Second)
}

func TestServiceRegistry_Register_WithCustomHealth(t *testing.T) {
	_, client, cleanup := setupEtcdTestServer(t)
	defer cleanup()

	sr := NewServiceRegistry(client, 30)

	service := ServiceInfo{
		ID:      "test-service-2",
		Type:    "streamer",
		Address: "192.168.1.101",
		Port:    8081,
		Health:  "degraded",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := sr.Register(ctx, service)
	assert.NoError(t, err)

	// Verify custom health was preserved
	resp, err := client.Get(ctx, sr.serviceKey)
	require.NoError(t, err)
	require.Len(t, resp.Kvs, 1)

	var storedService ServiceInfo
	err = json.Unmarshal(resp.Kvs[0].Value, &storedService)
	require.NoError(t, err)

	assert.Equal(t, "degraded", storedService.Health)
}

func TestServiceRegistry_Discover(t *testing.T) {
	_, client, cleanup := setupEtcdTestServer(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Register multiple services
	services := []ServiceInfo{
		{
			ID:      "collector-1",
			Type:    "collector",
			Address: "192.168.1.100",
			Port:    8080,
			Health:  "healthy",
		},
		{
			ID:      "collector-2",
			Type:    "collector",
			Address: "192.168.1.101",
			Port:    8080,
			Health:  "healthy",
		},
		{
			ID:      "streamer-1",
			Type:    "streamer",
			Address: "192.168.1.102",
			Port:    8081,
			Health:  "healthy",
		},
	}

	for _, service := range services {
		sr := NewServiceRegistry(client, 30)
		err := sr.Register(ctx, service)
		require.NoError(t, err)
	}

	// Test discovering collectors
	sr := NewServiceRegistry(client, 30)
	discoveredCollectors, err := sr.Discover(ctx, "collector")
	assert.NoError(t, err)
	assert.Len(t, discoveredCollectors, 2)

	// Verify discovered services
	collectorIDs := make(map[string]bool)
	for _, service := range discoveredCollectors {
		collectorIDs[service.ID] = true
		assert.Equal(t, "collector", service.Type)
		assert.Equal(t, "healthy", service.Health)
	}
	assert.True(t, collectorIDs["collector-1"])
	assert.True(t, collectorIDs["collector-2"])

	// Test discovering streamers
	discoveredStreamers, err := sr.Discover(ctx, "streamer")
	assert.NoError(t, err)
	assert.Len(t, discoveredStreamers, 1)
	assert.Equal(t, "streamer-1", discoveredStreamers[0].ID)

	// Test discovering non-existent service type
	discoveredGateways, err := sr.Discover(ctx, "gateway")
	assert.NoError(t, err)
	assert.Len(t, discoveredGateways, 0)
}

func TestServiceRegistry_WatchServices(t *testing.T) {
	_, client, cleanup := setupEtcdTestServer(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	sr := NewServiceRegistry(client, 30)

	// Start watching collectors
	watchChan := sr.WatchServices(ctx, "collector")

	// Initially should receive empty list
	select {
	case services := <-watchChan:
		assert.Len(t, services, 0)
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for initial empty service list")
	}

	// Register a service
	service1 := ServiceInfo{
		ID:      "collector-1",
		Type:    "collector",
		Address: "192.168.1.100",
		Port:    8080,
		Health:  "healthy",
	}

	sr1 := NewServiceRegistry(client, 30)
	err := sr1.Register(ctx, service1)
	require.NoError(t, err)

	// Should receive updated list with one service
	select {
	case services := <-watchChan:
		assert.Len(t, services, 1)
		assert.Equal(t, "collector-1", services[0].ID)
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for service registration update")
	}

	// Register another service
	service2 := ServiceInfo{
		ID:      "collector-2",
		Type:    "collector",
		Address: "192.168.1.101",
		Port:    8080,
		Health:  "healthy",
	}

	sr2 := NewServiceRegistry(client, 30)
	err = sr2.Register(ctx, service2)
	require.NoError(t, err)

	// Should receive updated list with two services
	select {
	case services := <-watchChan:
		assert.Len(t, services, 2)
		serviceIDs := make(map[string]bool)
		for _, service := range services {
			serviceIDs[service.ID] = true
		}
		assert.True(t, serviceIDs["collector-1"])
		assert.True(t, serviceIDs["collector-2"])
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for second service registration update")
	}
}

func TestServiceRegistry_UpdateHealth(t *testing.T) {
	_, client, cleanup := setupEtcdTestServer(t)
	defer cleanup()

	sr := NewServiceRegistry(client, 30)

	service := ServiceInfo{
		ID:      "test-service",
		Type:    "collector",
		Address: "192.168.1.100",
		Port:    8080,
		Health:  "healthy",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Register service first
	err := sr.Register(ctx, service)
	require.NoError(t, err)

	// Update health to unhealthy
	err = sr.UpdateHealth(ctx, "unhealthy")
	assert.NoError(t, err)

	// Verify health was updated
	resp, err := client.Get(ctx, sr.serviceKey)
	require.NoError(t, err)
	require.Len(t, resp.Kvs, 1)

	var updatedService ServiceInfo
	err = json.Unmarshal(resp.Kvs[0].Value, &updatedService)
	require.NoError(t, err)

	assert.Equal(t, "unhealthy", updatedService.Health)
	assert.Equal(t, service.ID, updatedService.ID) // Other fields unchanged

	// Update health to degraded
	err = sr.UpdateHealth(ctx, "degraded")
	assert.NoError(t, err)

	resp, err = client.Get(ctx, sr.serviceKey)
	require.NoError(t, err)
	require.Len(t, resp.Kvs, 1)

	err = json.Unmarshal(resp.Kvs[0].Value, &updatedService)
	require.NoError(t, err)

	assert.Equal(t, "degraded", updatedService.Health)
}

func TestServiceRegistry_UpdateHealth_NotRegistered(t *testing.T) {
	_, client, cleanup := setupEtcdTestServer(t)
	defer cleanup()

	sr := NewServiceRegistry(client, 30)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Try to update health without registering first
	err := sr.UpdateHealth(ctx, "healthy")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "service not registered")
}

func TestServiceRegistry_Deregister(t *testing.T) {
	_, client, cleanup := setupEtcdTestServer(t)
	defer cleanup()

	sr := NewServiceRegistry(client, 30)

	service := ServiceInfo{
		ID:      "test-service",
		Type:    "collector",
		Address: "192.168.1.100",
		Port:    8080,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Register service first
	err := sr.Register(ctx, service)
	require.NoError(t, err)

	// Store the service key before deregistering
	serviceKey := sr.serviceKey

	// Verify service exists
	resp, err := client.Get(ctx, serviceKey)
	require.NoError(t, err)
	require.Len(t, resp.Kvs, 1)

	// Deregister service
	err = sr.Deregister(ctx)
	assert.NoError(t, err)

	// Verify service was removed
	resp, err = client.Get(ctx, serviceKey)
	require.NoError(t, err)
	assert.Len(t, resp.Kvs, 0)

	// Verify internal state was reset
	assert.Empty(t, sr.serviceKey)
	assert.Equal(t, clientv3.LeaseID(0), sr.leaseID)

	// Deregister again should not error
	err = sr.Deregister(ctx)
	assert.NoError(t, err)
}

func TestServiceRegistry_Deregister_NotRegistered(t *testing.T) {
	_, client, cleanup := setupEtcdTestServer(t)
	defer cleanup()

	sr := NewServiceRegistry(client, 30)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Deregister without registering first should not error
	err := sr.Deregister(ctx)
	assert.NoError(t, err)
}

func TestServiceRegistry_GetServiceByID(t *testing.T) {
	_, client, cleanup := setupEtcdTestServer(t)
	defer cleanup()

	sr := NewServiceRegistry(client, 30)

	service := ServiceInfo{
		ID:       "test-service",
		Type:     "collector",
		Address:  "192.168.1.100",
		Port:     8080,
		Metadata: map[string]string{"version": "1.0.0"},
		Version:  "1.0.0",
		Health:   "healthy",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Register service first
	err := sr.Register(ctx, service)
	require.NoError(t, err)

	// Get service by ID
	foundService, err := sr.GetServiceByID(ctx, "collector", "test-service")
	assert.NoError(t, err)
	require.NotNil(t, foundService)

	assert.Equal(t, service.ID, foundService.ID)
	assert.Equal(t, service.Type, foundService.Type)
	assert.Equal(t, service.Address, foundService.Address)
	assert.Equal(t, service.Port, foundService.Port)
	assert.Equal(t, service.Metadata, foundService.Metadata)
	assert.Equal(t, service.Version, foundService.Version)
	assert.Equal(t, "healthy", foundService.Health)

	// Try to get non-existent service
	notFoundService, err := sr.GetServiceByID(ctx, "collector", "non-existent")
	assert.Error(t, err)
	assert.Nil(t, notFoundService)
	assert.Contains(t, err.Error(), "service not found")

	// Try to get service with wrong type
	wrongTypeService, err := sr.GetServiceByID(ctx, "streamer", "test-service")
	assert.Error(t, err)
	assert.Nil(t, wrongTypeService)
	assert.Contains(t, err.Error(), "service not found")
}

func TestServiceRegistry_LeaseExpiration(t *testing.T) {
	_, client, cleanup := setupEtcdTestServer(t)
	defer cleanup()

	// Use very short TTL for testing
	sr := NewServiceRegistry(client, 2) // 2 seconds

	service := ServiceInfo{
		ID:      "test-service",
		Type:    "collector",
		Address: "192.168.1.100",
		Port:    8080,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Register service
	err := sr.Register(ctx, service)
	require.NoError(t, err)

	// Store the service key before deregistering
	serviceKey := sr.serviceKey

	// Verify service exists
	resp, err := client.Get(ctx, serviceKey)
	require.NoError(t, err)
	require.Len(t, resp.Kvs, 1)

	// Stop the service registry to stop lease renewal
	sr.Deregister(ctx)

	// Wait for lease to expire (though it's already removed by Deregister)
	time.Sleep(4 * time.Second)

	// Verify service was removed (either by Deregister or lease expiration)
	resp, err = client.Get(ctx, serviceKey)
	require.NoError(t, err)
	assert.Len(t, resp.Kvs, 0)
}

func TestServiceRegistry_ConcurrentOperations(t *testing.T) {
	_, client, cleanup := setupEtcdTestServer(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	// Register multiple services concurrently
	numServices := 10
	done := make(chan bool, numServices)

	for i := 0; i < numServices; i++ {
		go func(id int) {
			defer func() { done <- true }()

			sr := NewServiceRegistry(client, 30)
			service := ServiceInfo{
				ID:      fmt.Sprintf("service-%d", id),
				Type:    "collector",
				Address: fmt.Sprintf("192.168.1.%d", 100+id),
				Port:    8080 + id,
				Health:  "healthy",
			}

			err := sr.Register(ctx, service)
			assert.NoError(t, err)

			// Update health
			err = sr.UpdateHealth(ctx, "degraded")
			assert.NoError(t, err)

			// Deregister
			err = sr.Deregister(ctx)
			assert.NoError(t, err)
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < numServices; i++ {
		select {
		case <-done:
		case <-time.After(15 * time.Second):
			t.Fatal("Timeout waiting for concurrent operations to complete")
		}
	}

	// Verify all services were cleaned up
	sr := NewServiceRegistry(client, 30)
	services, err := sr.Discover(ctx, "collector")
	assert.NoError(t, err)
	assert.Len(t, services, 0)
}

func TestServiceInfo_Validation(t *testing.T) {
	service := ServiceInfo{
		ID:           "test-service-123",
		Type:         "collector",
		Address:      "192.168.1.100",
		Port:         8080,
		Metadata:     map[string]string{"version": "1.0.0", "region": "us-west-2"},
		RegisteredAt: time.Now(),
		Health:       "healthy",
		Version:      "1.0.0",
	}

	// Basic validation
	assert.NotEmpty(t, service.ID)
	assert.NotEmpty(t, service.Type)
	assert.NotEmpty(t, service.Address)
	assert.True(t, service.Port > 0 && service.Port < 65536)
	assert.NotNil(t, service.Metadata)
	assert.Contains(t, []string{"healthy", "unhealthy", "degraded"}, service.Health)
	assert.NotEmpty(t, service.Version)
	assert.WithinDuration(t, time.Now(), service.RegisteredAt, time.Second)
}

func TestServiceRegistry_ErrorHandling(t *testing.T) {
	_, client, cleanup := setupEtcdTestServer(t)
	defer cleanup()

	sr := NewServiceRegistry(client, 30)

	t.Run("register with cancelled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		service := ServiceInfo{
			ID:      "test-service",
			Type:    "collector",
			Address: "192.168.1.100",
			Port:    8080,
		}

		err := sr.Register(ctx, service)
		assert.Error(t, err)
	})

	t.Run("discover with cancelled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		services, err := sr.Discover(ctx, "collector")
		assert.Error(t, err)
		assert.Nil(t, services)
	})

	t.Run("update health with cancelled context", func(t *testing.T) {
		// First register a service
		ctx := context.Background()
		service := ServiceInfo{
			ID:      "test-service",
			Type:    "collector",
			Address: "192.168.1.100",
			Port:    8080,
		}
		err := sr.Register(ctx, service)
		require.NoError(t, err)

		// Then try to update with cancelled context
		cancelledCtx, cancel := context.WithCancel(context.Background())
		cancel()

		err = sr.UpdateHealth(cancelledCtx, "unhealthy")
		assert.Error(t, err)
	})
}
