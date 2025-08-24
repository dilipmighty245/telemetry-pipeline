// Package config provides dynamic configuration management using etcd as the backend store.
// It supports real-time configuration updates, caching, and watching for configuration changes.
package config

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/dilipmighty245/telemetry-pipeline/pkg/logging"
	clientv3 "go.etcd.io/etcd/client/v3"
)

// ConfigUpdate represents a configuration change event from etcd.
// It contains information about what configuration key was changed,
// its new value, the type of change (PUT/DELETE), and when it occurred.
type ConfigUpdate struct {
	Key       string      `json:"key"`       // The configuration key that was changed
	Value     interface{} `json:"value"`     // The new value (nil for DELETE operations)
	Type      string      `json:"type"`      // Type of change: "PUT" or "DELETE"
	Timestamp time.Time   `json:"timestamp"` // When the change occurred
}

// ConfigManager manages dynamic configuration using etcd as the backend store.
// It provides caching, real-time updates, and thread-safe access to configuration values.
// The manager supports watching for configuration changes and maintains a local cache
// for fast access to frequently used configuration values.
type ConfigManager struct {
	client      *clientv3.Client             // etcd client for backend operations
	configCache map[string]interface{}       // Local cache of configuration values
	watchers    map[string]chan ConfigUpdate // Active watchers for configuration keys
	mu          sync.RWMutex                 // Mutex for thread-safe access
	ctx         context.Context              // Context for cancellation
	cancel      context.CancelFunc           // Cancel function for graceful shutdown
}

// NewConfigManager creates a new configuration manager with the provided etcd client.
// It initializes the internal cache, watchers map, and sets up a cancellable context
// for managing the lifecycle of background operations.
//
// Parameters:
//   - client: An initialized etcd client for backend operations
//
// Returns:
//   - *ConfigManager: A new configuration manager instance ready for use
func NewConfigManager(client *clientv3.Client) *ConfigManager {
	ctx, cancel := context.WithCancel(context.Background())

	return &ConfigManager{
		client:      client,
		configCache: make(map[string]interface{}),
		watchers:    make(map[string]chan ConfigUpdate),
		ctx:         ctx,
		cancel:      cancel,
	}
}

// Get retrieves a configuration value from the local cache.
// This method is thread-safe and provides fast access to cached configuration values.
//
// Parameters:
//   - key: The configuration key to retrieve
//
// Returns:
//   - interface{}: The configuration value if found, nil otherwise
//   - bool: true if the key exists, false otherwise
func (cm *ConfigManager) Get(key string) (interface{}, bool) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	value, exists := cm.configCache[key]
	return value, exists
}

// GetString retrieves a string configuration value with type safety.
// If the key doesn't exist or the value is not a string, returns the default value.
//
// Parameters:
//   - key: The configuration key to retrieve
//   - defaultValue: The value to return if key doesn't exist or is not a string
//
// Returns:
//   - string: The configuration value as a string, or defaultValue if not found/invalid
func (cm *ConfigManager) GetString(key string, defaultValue string) string {
	if value, exists := cm.Get(key); exists {
		if str, ok := value.(string); ok {
			return str
		}
	}
	return defaultValue
}

// GetInt retrieves an integer configuration value with type safety.
// Handles both int and float64 types (JSON unmarshaling converts numbers to float64).
// If the key doesn't exist or the value is not numeric, returns the default value.
//
// Parameters:
//   - key: The configuration key to retrieve
//   - defaultValue: The value to return if key doesn't exist or is not numeric
//
// Returns:
//   - int: The configuration value as an integer, or defaultValue if not found/invalid
func (cm *ConfigManager) GetInt(key string, defaultValue int) int {
	if value, exists := cm.Get(key); exists {
		if num, ok := value.(float64); ok {
			return int(num)
		}
		if num, ok := value.(int); ok {
			return num
		}
	}
	return defaultValue
}

// GetFloat retrieves a float64 configuration value with type safety.
// If the key doesn't exist or the value is not a float64, returns the default value.
//
// Parameters:
//   - key: The configuration key to retrieve
//   - defaultValue: The value to return if key doesn't exist or is not a float64
//
// Returns:
//   - float64: The configuration value as a float64, or defaultValue if not found/invalid
func (cm *ConfigManager) GetFloat(key string, defaultValue float64) float64 {
	if value, exists := cm.Get(key); exists {
		if num, ok := value.(float64); ok {
			return num
		}
	}
	return defaultValue
}

// GetBool retrieves a boolean configuration value with type safety.
// If the key doesn't exist or the value is not a boolean, returns the default value.
//
// Parameters:
//   - key: The configuration key to retrieve
//   - defaultValue: The value to return if key doesn't exist or is not a boolean
//
// Returns:
//   - bool: The configuration value as a boolean, or defaultValue if not found/invalid
func (cm *ConfigManager) GetBool(key string, defaultValue bool) bool {
	if value, exists := cm.Get(key); exists {
		if b, ok := value.(bool); ok {
			return b
		}
	}
	return defaultValue
}

// Set stores a configuration value in etcd and updates the local cache.
// The value is JSON-marshaled before storage and the operation includes a timeout.
// On successful storage, the local cache is immediately updated.
//
// Parameters:
//   - key: The configuration key to set
//   - value: The value to store (must be JSON-marshalable)
//
// Returns:
//   - error: nil on success, error describing the failure otherwise
func (cm *ConfigManager) Set(key string, value interface{}) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal config value: %w", err)
	}

	configKey := fmt.Sprintf("/config/%s", key)
	ctx, cancel := context.WithTimeout(cm.ctx, 10*time.Second)
	defer cancel()

	_, err = cm.client.Put(ctx, configKey, string(data))
	if err != nil {
		return fmt.Errorf("failed to store config: %w", err)
	}

	// Update local cache
	cm.mu.Lock()
	cm.configCache[key] = value
	cm.mu.Unlock()

	logging.Debugf("Config updated: %s = %v", key, value)
	return nil
}

// LoadInitialConfig loads all existing configuration from etcd into the local cache.
// This method should be called after creating a ConfigManager to populate the cache
// with existing configuration values. It uses a prefix scan to load all keys under "/config/".
//
// Returns:
//   - error: nil on success, error describing the failure otherwise
func (cm *ConfigManager) LoadInitialConfig() error {
	ctx, cancel := context.WithTimeout(cm.ctx, 30*time.Second)
	defer cancel()

	resp, err := cm.client.Get(ctx, "/config/", clientv3.WithPrefix())
	if err != nil {
		return fmt.Errorf("failed to load initial config: %w", err)
	}

	cm.mu.Lock()
	defer cm.mu.Unlock()

	for _, kv := range resp.Kvs {
		key := string(kv.Key)[8:] // Remove "/config/" prefix

		var value interface{}
		if err := json.Unmarshal(kv.Value, &value); err != nil {
			logging.Errorf("Failed to unmarshal config %s: %v", key, err)
			continue
		}

		cm.configCache[key] = value
		logging.Debugf("Loaded config: %s = %v", key, value)
	}

	logging.Infof("Loaded %d configuration values", len(cm.configCache))
	return nil
}

// WatchConfig watches for changes to a specific configuration key.
// Returns a channel that receives ConfigUpdate events when the key changes.
// The watcher automatically updates the local cache when changes occur.
// Only one watcher per key is allowed; subsequent calls return the existing channel.
//
// Parameters:
//   - key: The configuration key to watch
//
// Returns:
//   - <-chan ConfigUpdate: A receive-only channel for configuration updates
func (cm *ConfigManager) WatchConfig(key string) <-chan ConfigUpdate {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if ch, exists := cm.watchers[key]; exists {
		return ch
	}

	ch := make(chan ConfigUpdate, 10)
	cm.watchers[key] = ch

	go func() {
		defer func() {
			cm.mu.Lock()
			// Only close if the channel is still in the watchers map
			// This prevents double-close when Close() is called
			if _, exists := cm.watchers[key]; exists {
				delete(cm.watchers, key)
				close(ch)
			}
			cm.mu.Unlock()
		}()

		configKey := fmt.Sprintf("/config/%s", key)
		watchChan := cm.client.Watch(cm.ctx, configKey)

		for watchResp := range watchChan {
			if watchResp.Err() != nil {
				logging.Errorf("Config watch error for key %s: %v", key, watchResp.Err())
				return
			}

			for _, event := range watchResp.Events {
				var value interface{}
				if event.Type == clientv3.EventTypePut {
					if err := json.Unmarshal(event.Kv.Value, &value); err != nil {
						logging.Errorf("Failed to unmarshal config update for %s: %v", key, err)
						continue
					}

					// Update local cache
					cm.mu.Lock()
					cm.configCache[key] = value
					cm.mu.Unlock()
				} else if event.Type == clientv3.EventTypeDelete {
					// Remove from cache
					cm.mu.Lock()
					delete(cm.configCache, key)
					cm.mu.Unlock()
				}

				update := ConfigUpdate{
					Key:       key,
					Value:     value,
					Type:      event.Type.String(),
					Timestamp: time.Now(),
				}

				select {
				case ch <- update:
				case <-cm.ctx.Done():
					return
				}
			}
		}
	}()

	return ch
}

// WatchAllConfig watches for changes to all configuration keys under the "/config/" prefix.
// Returns a channel that receives ConfigUpdate events for any configuration change.
// The watcher automatically updates the local cache when changes occur.
//
// Returns:
//   - <-chan ConfigUpdate: A receive-only channel for all configuration updates
func (cm *ConfigManager) WatchAllConfig() <-chan ConfigUpdate {
	updateChan := make(chan ConfigUpdate, 50)

	go func() {
		defer close(updateChan)

		watchChan := cm.client.Watch(cm.ctx, "/config/", clientv3.WithPrefix())

		for watchResp := range watchChan {
			if watchResp.Err() != nil {
				logging.Errorf("Config watch error: %v", watchResp.Err())
				return
			}

			for _, event := range watchResp.Events {
				key := string(event.Kv.Key)[8:] // Remove "/config/" prefix

				var value interface{}
				if event.Type == clientv3.EventTypePut {
					if err := json.Unmarshal(event.Kv.Value, &value); err != nil {
						logging.Errorf("Failed to unmarshal config update for %s: %v", key, err)
						continue
					}

					// Update local cache
					cm.mu.Lock()
					cm.configCache[key] = value
					cm.mu.Unlock()
				} else if event.Type == clientv3.EventTypeDelete {
					// Remove from cache
					cm.mu.Lock()
					delete(cm.configCache, key)
					cm.mu.Unlock()
				}

				update := ConfigUpdate{
					Key:       key,
					Value:     value,
					Type:      event.Type.String(),
					Timestamp: time.Now(),
				}

				select {
				case updateChan <- update:
				case <-cm.ctx.Done():
					return
				}
			}
		}
	}()

	return updateChan
}

// SetDefaults sets default configuration values for keys that don't already exist.
// This method is useful for initializing configuration with sensible defaults
// without overwriting existing user-configured values.
//
// Parameters:
//   - defaults: A map of key-value pairs to set as defaults
//
// Returns:
//   - error: nil on success, error describing the failure otherwise
func (cm *ConfigManager) SetDefaults(defaults map[string]interface{}) error {
	for key, value := range defaults {
		if _, exists := cm.Get(key); !exists {
			if err := cm.Set(key, value); err != nil {
				return fmt.Errorf("failed to set default for %s: %w", key, err)
			}
		}
	}
	return nil
}

// GetAllConfig returns a copy of all configuration values from the local cache.
// This method is thread-safe and returns a new map to prevent external modifications
// to the internal cache.
//
// Returns:
//   - map[string]interface{}: A copy of all cached configuration values
func (cm *ConfigManager) GetAllConfig() map[string]interface{} {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	config := make(map[string]interface{})
	for k, v := range cm.configCache {
		config[k] = v
	}
	return config
}

// Close gracefully shuts down the configuration manager.
// It cancels the internal context, closes all active watchers, and cleans up resources.
// This method should be called when the configuration manager is no longer needed.
//
// Returns:
//   - error: Always returns nil (kept for interface compatibility)
func (cm *ConfigManager) Close() error {
	cm.cancel()

	// Close all watchers
	cm.mu.Lock()
	for key, ch := range cm.watchers {
		close(ch)
		delete(cm.watchers, key)
	}
	cm.mu.Unlock()

	return nil
}
