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

// ConfigUpdate represents a configuration change
type ConfigUpdate struct {
	Key       string      `json:"key"`
	Value     interface{} `json:"value"`
	Type      string      `json:"type"`
	Timestamp time.Time   `json:"timestamp"`
}

// ConfigManager manages dynamic configuration using etcd
type ConfigManager struct {
	client      *clientv3.Client
	configCache map[string]interface{}
	watchers    map[string]chan ConfigUpdate
	mu          sync.RWMutex
	ctx         context.Context
	cancel      context.CancelFunc
}

// NewConfigManager creates a new configuration manager
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

// Get retrieves a configuration value
func (cm *ConfigManager) Get(key string) (interface{}, bool) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	value, exists := cm.configCache[key]
	return value, exists
}

// GetString retrieves a string configuration value
func (cm *ConfigManager) GetString(key string, defaultValue string) string {
	if value, exists := cm.Get(key); exists {
		if str, ok := value.(string); ok {
			return str
		}
	}
	return defaultValue
}

// GetInt retrieves an integer configuration value
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

// GetFloat retrieves a float configuration value
func (cm *ConfigManager) GetFloat(key string, defaultValue float64) float64 {
	if value, exists := cm.Get(key); exists {
		if num, ok := value.(float64); ok {
			return num
		}
	}
	return defaultValue
}

// GetBool retrieves a boolean configuration value
func (cm *ConfigManager) GetBool(key string, defaultValue bool) bool {
	if value, exists := cm.Get(key); exists {
		if b, ok := value.(bool); ok {
			return b
		}
	}
	return defaultValue
}

// Set stores a configuration value in etcd
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

// LoadInitialConfig loads all existing configuration from etcd
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

// WatchConfig watches for changes to a specific configuration key
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

// WatchAllConfig watches for changes to all configuration keys
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

// SetDefaults sets default configuration values if they don't exist
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

// GetAllConfig returns all configuration values
func (cm *ConfigManager) GetAllConfig() map[string]interface{} {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	config := make(map[string]interface{})
	for k, v := range cm.configCache {
		config[k] = v
	}
	return config
}

// Close closes the configuration manager
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
