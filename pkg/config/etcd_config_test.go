package config

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewConfigManager(t *testing.T) {
	// Test with nil client (for testing core logic)
	cm := NewConfigManager(nil)

	assert.NotNil(t, cm)
	assert.NotNil(t, cm.configCache)
	assert.NotNil(t, cm.watchers)
	assert.NotNil(t, cm.ctx)
	assert.NotNil(t, cm.cancel)
}

func TestConfigManager_Get(t *testing.T) {
	cm := NewConfigManager(nil)

	// Test getting non-existent key
	value, exists := cm.Get("non-existent")
	assert.False(t, exists)
	assert.Nil(t, value)

	// Test getting existing key by directly setting cache
	cm.mu.Lock()
	cm.configCache["test-key"] = "test-value"
	cm.mu.Unlock()

	value, exists = cm.Get("test-key")
	assert.True(t, exists)
	assert.Equal(t, "test-value", value)
}

func TestConfigManager_GetString(t *testing.T) {
	cm := NewConfigManager(nil)

	// Test with non-existent key
	result := cm.GetString("non-existent", "default")
	assert.Equal(t, "default", result)

	// Test with existing string value
	cm.mu.Lock()
	cm.configCache["string-key"] = "test-string"
	cm.mu.Unlock()

	result = cm.GetString("string-key", "default")
	assert.Equal(t, "test-string", result)

	// Test with non-string value
	cm.mu.Lock()
	cm.configCache["non-string-key"] = 123
	cm.mu.Unlock()

	result = cm.GetString("non-string-key", "default")
	assert.Equal(t, "default", result)
}

func TestConfigManager_GetInt(t *testing.T) {
	cm := NewConfigManager(nil)

	// Test with non-existent key
	result := cm.GetInt("non-existent", 42)
	assert.Equal(t, 42, result)

	// Test with existing int value
	cm.mu.Lock()
	cm.configCache["int-key"] = 123
	cm.mu.Unlock()

	result = cm.GetInt("int-key", 42)
	assert.Equal(t, 123, result)

	// Test with existing float64 value (JSON unmarshaling converts numbers to float64)
	cm.mu.Lock()
	cm.configCache["float-key"] = 456.0
	cm.mu.Unlock()

	result = cm.GetInt("float-key", 42)
	assert.Equal(t, 456, result)

	// Test with non-numeric value
	cm.mu.Lock()
	cm.configCache["non-numeric-key"] = "not-a-number"
	cm.mu.Unlock()

	result = cm.GetInt("non-numeric-key", 42)
	assert.Equal(t, 42, result)
}

func TestConfigManager_GetFloat(t *testing.T) {
	cm := NewConfigManager(nil)

	// Test with non-existent key
	result := cm.GetFloat("non-existent", 3.14)
	assert.Equal(t, 3.14, result)

	// Test with existing float value
	cm.mu.Lock()
	cm.configCache["float-key"] = 2.71
	cm.mu.Unlock()

	result = cm.GetFloat("float-key", 3.14)
	assert.Equal(t, 2.71, result)

	// Test with existing int value - GetFloat only handles float64, so should return default
	cm.mu.Lock()
	cm.configCache["int-key"] = 42
	cm.mu.Unlock()

	result = cm.GetFloat("int-key", 3.14)
	assert.Equal(t, 3.14, result)

	// Test with non-float value
	cm.mu.Lock()
	cm.configCache["non-float-key"] = "not-a-float"
	cm.mu.Unlock()

	result = cm.GetFloat("non-float-key", 3.14)
	assert.Equal(t, 3.14, result)
}

func TestConfigManager_GetBool(t *testing.T) {
	cm := NewConfigManager(nil)

	// Test with non-existent key
	result := cm.GetBool("non-existent", true)
	assert.True(t, result)

	// Test with existing bool value
	cm.mu.Lock()
	cm.configCache["bool-key"] = false
	cm.mu.Unlock()

	result = cm.GetBool("bool-key", true)
	assert.False(t, result)

	// Test with non-bool value
	cm.mu.Lock()
	cm.configCache["non-bool-key"] = "not-a-bool"
	cm.mu.Unlock()

	result = cm.GetBool("non-bool-key", true)
	assert.True(t, result)
}

func TestConfigManager_GetAllConfig(t *testing.T) {
	cm := NewConfigManager(nil)

	// Add some test data directly to cache
	cm.mu.Lock()
	cm.configCache["key1"] = "value1"
	cm.configCache["key2"] = 123
	cm.configCache["key3"] = true
	cm.mu.Unlock()

	allConfig := cm.GetAllConfig()

	assert.Len(t, allConfig, 3)
	assert.Equal(t, "value1", allConfig["key1"])
	assert.Equal(t, 123, allConfig["key2"])
	assert.Equal(t, true, allConfig["key3"])
}

func TestConfigManager_Close(t *testing.T) {
	cm := NewConfigManager(nil)

	// Add a mock watcher
	cm.mu.Lock()
	watchChan := make(chan ConfigUpdate, 1)
	cm.watchers["test-key"] = watchChan
	cm.mu.Unlock()

	err := cm.Close()
	assert.NoError(t, err)

	// Verify context was cancelled
	select {
	case <-cm.ctx.Done():
		// Expected
	default:
		t.Error("Context was not cancelled")
	}

	// Verify watchers were cleaned up
	cm.mu.RLock()
	watcherCount := len(cm.watchers)
	cm.mu.RUnlock()
	assert.Equal(t, 0, watcherCount)
}

func TestConfigUpdate_Struct(t *testing.T) {
	update := ConfigUpdate{
		Key:       "test-key",
		Value:     "test-value",
		Timestamp: time.Now(),
	}

	assert.Equal(t, "test-key", update.Key)
	assert.Equal(t, "test-value", update.Value)
	assert.False(t, update.Timestamp.IsZero())
}

func TestConfigManager_ConcurrentAccess(t *testing.T) {
	cm := NewConfigManager(nil)

	// Test concurrent reads and writes
	done := make(chan bool, 2)

	// Writer goroutine
	go func() {
		for i := 0; i < 100; i++ {
			cm.mu.Lock()
			cm.configCache[fmt.Sprintf("key%d", i)] = fmt.Sprintf("value%d", i)
			cm.mu.Unlock()
		}
		done <- true
	}()

	// Reader goroutine
	go func() {
		for i := 0; i < 100; i++ {
			cm.Get(fmt.Sprintf("key%d", i))
		}
		done <- true
	}()

	// Wait for both goroutines
	<-done
	<-done

	// Verify some data was written
	value, exists := cm.Get("key50")
	if exists {
		assert.Equal(t, "value50", value)
	}
}

func TestConfigManager_TypeConversions(t *testing.T) {
	cm := NewConfigManager(nil)

	tests := []struct {
		name      string
		value     interface{}
		getString string
		getInt    int
		getFloat  float64
		getBool   bool
	}{
		{
			name:      "string value",
			value:     "hello",
			getString: "hello",
			getInt:    0,     // default when conversion fails
			getFloat:  0.0,   // default when conversion fails
			getBool:   false, // default when conversion fails
		},
		{
			name:      "int value",
			value:     42,
			getString: "", // default when conversion fails
			getInt:    42,
			getFloat:  0.0,   // default when conversion fails (GetFloat only handles float64)
			getBool:   false, // default when conversion fails
		},
		{
			name:      "float value",
			value:     3.14,
			getString: "", // default when conversion fails
			getInt:    3,  // truncated
			getFloat:  3.14,
			getBool:   false, // default when conversion fails
		},
		{
			name:      "bool value",
			value:     true,
			getString: "",  // default when conversion fails
			getInt:    0,   // default when conversion fails
			getFloat:  0.0, // default when conversion fails
			getBool:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := "test-" + tt.name
			cm.mu.Lock()
			cm.configCache[key] = tt.value
			cm.mu.Unlock()

			assert.Equal(t, tt.getString, cm.GetString(key, ""))
			assert.Equal(t, tt.getInt, cm.GetInt(key, 0))
			assert.Equal(t, tt.getFloat, cm.GetFloat(key, 0.0))
			assert.Equal(t, tt.getBool, cm.GetBool(key, false))
		})
	}
}

func TestConfigManager_EdgeCases(t *testing.T) {
	cm := NewConfigManager(nil)

	// Test with nil values
	cm.mu.Lock()
	cm.configCache["nil-key"] = nil
	cm.mu.Unlock()

	value, exists := cm.Get("nil-key")
	assert.True(t, exists)
	assert.Nil(t, value)

	// Test with empty string
	cm.mu.Lock()
	cm.configCache["empty-key"] = ""
	cm.mu.Unlock()

	stringVal := cm.GetString("empty-key", "default")
	assert.Equal(t, "", stringVal)

	// Test with zero values
	cm.mu.Lock()
	cm.configCache["zero-int"] = 0
	cm.mu.Unlock()

	intVal := cm.GetInt("zero-int", 42)
	assert.Equal(t, 0, intVal)

	cm.mu.Lock()
	cm.configCache["zero-float"] = 0.0
	cm.mu.Unlock()

	floatVal := cm.GetFloat("zero-float", 3.14)
	assert.Equal(t, 0.0, floatVal)

	cm.mu.Lock()
	cm.configCache["false-bool"] = false
	cm.mu.Unlock()

	boolVal := cm.GetBool("false-bool", true)
	assert.False(t, boolVal)
}

func TestConfigManager_DefaultValues(t *testing.T) {
	cm := NewConfigManager(nil)

	// Test all get methods with non-existent keys return defaults
	assert.Equal(t, "default-string", cm.GetString("missing", "default-string"))
	assert.Equal(t, 999, cm.GetInt("missing", 999))
	assert.Equal(t, 99.9, cm.GetFloat("missing", 99.9))
	assert.Equal(t, true, cm.GetBool("missing", true))
	assert.Equal(t, false, cm.GetBool("missing", false))
}

// Benchmark tests
func BenchmarkConfigManager_Get(b *testing.B) {
	cm := NewConfigManager(nil)
	cm.mu.Lock()
	cm.configCache["benchmark-key"] = "benchmark-value"
	cm.mu.Unlock()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cm.Get("benchmark-key")
	}
}

func BenchmarkConfigManager_GetString(b *testing.B) {
	cm := NewConfigManager(nil)
	cm.mu.Lock()
	cm.configCache["benchmark-key"] = "benchmark-value"
	cm.mu.Unlock()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cm.GetString("benchmark-key", "default")
	}
}

func BenchmarkConfigManager_GetInt(b *testing.B) {
	cm := NewConfigManager(nil)
	cm.mu.Lock()
	cm.configCache["benchmark-key"] = 42
	cm.mu.Unlock()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cm.GetInt("benchmark-key", 0)
	}
}

func BenchmarkConfigManager_ConcurrentRead(b *testing.B) {
	cm := NewConfigManager(nil)
	cm.mu.Lock()
	for i := 0; i < 100; i++ {
		cm.configCache[fmt.Sprintf("key%d", i)] = fmt.Sprintf("value%d", i)
	}
	cm.mu.Unlock()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			cm.Get("key50")
		}
	})
}
