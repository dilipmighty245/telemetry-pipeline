package config

import (
	"context"
	"net/url"
	"testing"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/server/v3/embed"
	"go.etcd.io/etcd/server/v3/etcdserver/api/v3client"
)

func TestConfigUpdate(t *testing.T) {
	update := ConfigUpdate{
		Key:       "test-key",
		Value:     "test-value",
		Type:      "PUT",
		Timestamp: time.Now(),
	}

	if update.Key != "test-key" {
		t.Errorf("Expected key 'test-key', got %s", update.Key)
	}
	if update.Value != "test-value" {
		t.Errorf("Expected value 'test-value', got %v", update.Value)
	}
	if update.Type != "PUT" {
		t.Errorf("Expected type 'PUT', got %s", update.Type)
	}
}

// Basic unit tests for ConfigManager methods (without etcd)

// setupEtcdTestServer creates an embedded etcd server for testing
func setupEtcdTestServer(t *testing.T) (*embed.Etcd, *clientv3.Client, func()) {
	// Create embedded etcd configuration
	cfg := embed.NewConfig()
	cfg.Name = "test-etcd"
	cfg.Dir = t.TempDir()
	cfg.LogLevel = "error" // Reduce log noise in tests

	// Use available ports - let etcd choose available ports
	clientURL, _ := url.Parse("http://127.0.0.1:0")
	peerURL, _ := url.Parse("http://127.0.0.1:0")

	cfg.ListenClientUrls = []url.URL{*clientURL}
	cfg.AdvertiseClientUrls = cfg.ListenClientUrls
	cfg.ListenPeerUrls = []url.URL{*peerURL}
	cfg.AdvertisePeerUrls = cfg.ListenPeerUrls
	cfg.InitialCluster = cfg.Name + "=" + peerURL.String()
	cfg.ClusterState = embed.ClusterStateFlagNew

	// Start etcd server
	e, err := embed.StartEtcd(cfg)
	if err != nil {
		t.Fatalf("Failed to start etcd: %v", err)
	}

	// Wait for etcd to be ready
	select {
	case <-e.Server.ReadyNotify():
		// Server is ready
	case <-time.After(10 * time.Second):
		e.Close()
		t.Fatalf("etcd server failed to start within 10 seconds")
	}

	// Create client
	client := v3client.New(e.Server)

	cleanup := func() {
		if client != nil {
			client.Close()
		}
		if e != nil {
			e.Close()
			select {
			case <-e.Server.StopNotify():
				// Server stopped
			case <-time.After(5 * time.Second):
				// Force stop if it takes too long
			}
		}
	}

	return e, client, cleanup
}

func TestConfigManager_WithEtcd_SetAndGet(t *testing.T) {
	_, client, cleanup := setupEtcdTestServer(t)
	defer cleanup()

	cm := NewConfigManager(client)
	defer cm.Close()

	// Test setting and getting a string value
	err := cm.Set("test-string", "hello-world")
	if err != nil {
		t.Fatalf("Failed to set config: %v", err)
	}

	// Wait a bit for the value to be stored
	time.Sleep(100 * time.Millisecond)

	value := cm.GetString("test-string", "default")
	if value != "hello-world" {
		t.Errorf("Expected 'hello-world', got %s", value)
	}

	// Test setting and getting an int value
	err = cm.Set("test-int", 42)
	if err != nil {
		t.Fatalf("Failed to set config: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	intValue := cm.GetInt("test-int", 0)
	if intValue != 42 {
		t.Errorf("Expected 42, got %d", intValue)
	}

	// Test setting and getting a bool value
	err = cm.Set("test-bool", true)
	if err != nil {
		t.Fatalf("Failed to set config: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	boolValue := cm.GetBool("test-bool", false)
	if !boolValue {
		t.Errorf("Expected true, got %t", boolValue)
	}
}

func TestConfigManager_WithEtcd_LoadInitialConfig(t *testing.T) {
	_, client, cleanup := setupEtcdTestServer(t)
	defer cleanup()

	// Pre-populate etcd with some config values
	ctx := context.Background()
	_, err := client.Put(ctx, "/config/preloaded-string", `"test-value"`)
	if err != nil {
		t.Fatalf("Failed to pre-populate etcd: %v", err)
	}

	_, err = client.Put(ctx, "/config/preloaded-int", "123")
	if err != nil {
		t.Fatalf("Failed to pre-populate etcd: %v", err)
	}

	// Create config manager and load initial config
	cm := NewConfigManager(client)
	defer cm.Close()

	err = cm.LoadInitialConfig()
	if err != nil {
		t.Fatalf("Failed to load initial config: %v", err)
	}

	// Verify the preloaded values are available
	stringValue := cm.GetString("preloaded-string", "default")
	if stringValue != "test-value" {
		t.Errorf("Expected 'test-value', got %s", stringValue)
	}

	intValue := cm.GetInt("preloaded-int", 0)
	if intValue != 123 {
		t.Errorf("Expected 123, got %d", intValue)
	}
}

func TestConfigManager_WithEtcd_SetDefaults(t *testing.T) {
	_, client, cleanup := setupEtcdTestServer(t)
	defer cleanup()

	cm := NewConfigManager(client)
	defer cm.Close()

	// Set some defaults
	defaults := map[string]interface{}{
		"default-string": "default-value",
		"default-int":    100,
		"default-bool":   true,
	}

	err := cm.SetDefaults(defaults)
	if err != nil {
		t.Fatalf("Failed to set defaults: %v", err)
	}

	// Wait for values to be stored
	time.Sleep(200 * time.Millisecond)

	// Verify defaults were set
	stringValue := cm.GetString("default-string", "fallback")
	if stringValue != "default-value" {
		t.Errorf("Expected 'default-value', got %s", stringValue)
	}

	intValue := cm.GetInt("default-int", 0)
	if intValue != 100 {
		t.Errorf("Expected 100, got %d", intValue)
	}

	boolValue := cm.GetBool("default-bool", false)
	if !boolValue {
		t.Errorf("Expected true, got %t", boolValue)
	}

	// Now set a value that already exists - it should not be overwritten
	err = cm.Set("default-string", "new-value")
	if err != nil {
		t.Fatalf("Failed to set config: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	// Try to set defaults again - existing values should not be overwritten
	err = cm.SetDefaults(map[string]interface{}{
		"default-string": "should-not-overwrite",
	})
	if err != nil {
		t.Fatalf("Failed to set defaults: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	// Verify the value was not overwritten
	stringValue = cm.GetString("default-string", "fallback")
	if stringValue != "new-value" {
		t.Errorf("Expected 'new-value' (should not be overwritten), got %s", stringValue)
	}
}

func TestConfigManager_WithEtcd_WatchConfig(t *testing.T) {
	_, client, cleanup := setupEtcdTestServer(t)
	defer cleanup()

	cm := NewConfigManager(client)
	defer cm.Close()

	// Start watching a key
	watchChan := cm.WatchConfig("watch-test")

	// Set a value for the watched key
	err := cm.Set("watch-test", "initial-value")
	if err != nil {
		t.Fatalf("Failed to set config: %v", err)
	}

	// Wait for the watch event
	select {
	case update := <-watchChan:
		if update.Key != "watch-test" {
			t.Errorf("Expected key 'watch-test', got %s", update.Key)
		}
		if update.Value != "initial-value" {
			t.Errorf("Expected value 'initial-value', got %v", update.Value)
		}
		if update.Type != "PUT" {
			t.Errorf("Expected type 'PUT', got %s", update.Type)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for watch event")
	}

	// Update the value
	err = cm.Set("watch-test", "updated-value")
	if err != nil {
		t.Fatalf("Failed to update config: %v", err)
	}

	// Wait for the second watch event
	select {
	case update := <-watchChan:
		if update.Key != "watch-test" {
			t.Errorf("Expected key 'watch-test', got %s", update.Key)
		}
		if update.Value != "updated-value" {
			t.Errorf("Expected value 'updated-value', got %v", update.Value)
		}
		if update.Type != "PUT" {
			t.Errorf("Expected type 'PUT', got %s", update.Type)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for second watch event")
	}
}

func TestConfigManager_WithEtcd_WatchAllConfig(t *testing.T) {
	_, client, cleanup := setupEtcdTestServer(t)
	defer cleanup()

	cm := NewConfigManager(client)
	defer cm.Close()

	// Start watching all config changes
	watchChan := cm.WatchAllConfig()

	// Set multiple values
	err := cm.Set("watch-all-1", "value1")
	if err != nil {
		t.Fatalf("Failed to set config: %v", err)
	}

	err = cm.Set("watch-all-2", "value2")
	if err != nil {
		t.Fatalf("Failed to set config: %v", err)
	}

	// Collect watch events
	receivedUpdates := make(map[string]string)
	timeout := time.After(5 * time.Second)

	for len(receivedUpdates) < 2 {
		select {
		case update := <-watchChan:
			if update.Type == "PUT" {
				receivedUpdates[update.Key] = update.Value.(string)
			}
		case <-timeout:
			t.Fatal("Timeout waiting for watch events")
		}
	}

	// Verify we received both updates
	if receivedUpdates["watch-all-1"] != "value1" {
		t.Errorf("Expected 'value1' for watch-all-1, got %s", receivedUpdates["watch-all-1"])
	}
	if receivedUpdates["watch-all-2"] != "value2" {
		t.Errorf("Expected 'value2' for watch-all-2, got %s", receivedUpdates["watch-all-2"])
	}
}

func TestConfigManager_WithEtcd_ErrorHandling(t *testing.T) {
	_, client, cleanup := setupEtcdTestServer(t)
	defer cleanup()

	cm := NewConfigManager(client)
	defer cm.Close()

	// Test setting an invalid value that can't be JSON marshaled
	err := cm.Set("invalid-key", make(chan int))
	if err == nil {
		t.Error("Expected error when setting invalid value, got nil")
	}

	// Test that the cache wasn't updated with invalid value
	value, exists := cm.Get("invalid-key")
	if exists {
		t.Errorf("Expected key not to exist after failed set, but got value: %v", value)
	}
}
