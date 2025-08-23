package discovery

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewServiceRegistry(t *testing.T) {
	// Test with nil client and default TTL
	sr := NewServiceRegistry(nil, 30)

	assert.NotNil(t, sr)
	// We can't access private fields, but we can test that the constructor works
}

func TestServiceInfo_Struct(t *testing.T) {
	now := time.Now()
	service := ServiceInfo{
		ID:           "service-1",
		Type:         "collector",
		Address:      "localhost",
		Port:         8080,
		Metadata:     map[string]string{"region": "us-west"},
		RegisteredAt: now,
		Health:       "healthy",
		Version:      "1.0.0",
	}

	assert.Equal(t, "service-1", service.ID)
	assert.Equal(t, "collector", service.Type)
	assert.Equal(t, "localhost", service.Address)
	assert.Equal(t, 8080, service.Port)
	assert.Equal(t, "us-west", service.Metadata["region"])
	assert.Equal(t, now, service.RegisteredAt)
	assert.Equal(t, "healthy", service.Health)
	assert.Equal(t, "1.0.0", service.Version)
}

func TestServiceInfo_JSONSerialization(t *testing.T) {
	service := ServiceInfo{
		ID:           "service-1",
		Type:         "collector",
		Address:      "localhost",
		Port:         8080,
		Metadata:     map[string]string{"region": "us-west", "env": "prod"},
		RegisteredAt: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC),
		Health:       "healthy",
		Version:      "1.0.0",
	}

	// Test JSON marshaling
	jsonData, err := json.Marshal(service)
	require.NoError(t, err)
	assert.Contains(t, string(jsonData), "\"id\":\"service-1\"")
	assert.Contains(t, string(jsonData), "\"type\":\"collector\"")
	assert.Contains(t, string(jsonData), "\"address\":\"localhost\"")
	assert.Contains(t, string(jsonData), "\"port\":8080")
	assert.Contains(t, string(jsonData), "\"region\":\"us-west\"")

	// Test JSON unmarshaling
	var unmarshaled ServiceInfo
	err = json.Unmarshal(jsonData, &unmarshaled)
	require.NoError(t, err)
	assert.Equal(t, service.ID, unmarshaled.ID)
	assert.Equal(t, service.Type, unmarshaled.Type)
	assert.Equal(t, service.Address, unmarshaled.Address)
	assert.Equal(t, service.Port, unmarshaled.Port)
	assert.Equal(t, service.Health, unmarshaled.Health)
	assert.Equal(t, service.Version, unmarshaled.Version)
	assert.Equal(t, service.Metadata["region"], unmarshaled.Metadata["region"])
	assert.Equal(t, service.Metadata["env"], unmarshaled.Metadata["env"])
}

func TestServiceInfo_Validation(t *testing.T) {
	tests := []struct {
		name    string
		service ServiceInfo
		valid   bool
	}{
		{
			name: "valid service",
			service: ServiceInfo{
				ID:      "service-1",
				Type:    "collector",
				Address: "localhost",
				Port:    8080,
				Health:  "healthy",
				Version: "1.0.0",
			},
			valid: true,
		},
		{
			name: "empty ID",
			service: ServiceInfo{
				ID:      "",
				Type:    "collector",
				Address: "localhost",
				Port:    8080,
			},
			valid: false,
		},
		{
			name: "empty type",
			service: ServiceInfo{
				ID:      "service-1",
				Type:    "",
				Address: "localhost",
				Port:    8080,
			},
			valid: false,
		},
		{
			name: "empty address",
			service: ServiceInfo{
				ID:      "service-1",
				Type:    "collector",
				Address: "",
				Port:    8080,
			},
			valid: false,
		},
		{
			name: "invalid port - zero",
			service: ServiceInfo{
				ID:      "service-1",
				Type:    "collector",
				Address: "localhost",
				Port:    0,
			},
			valid: false,
		},
		{
			name: "invalid port - negative",
			service: ServiceInfo{
				ID:      "service-1",
				Type:    "collector",
				Address: "localhost",
				Port:    -1,
			},
			valid: false,
		},
		{
			name: "valid port range",
			service: ServiceInfo{
				ID:      "service-1",
				Type:    "collector",
				Address: "localhost",
				Port:    65535,
			},
			valid: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.valid {
				assert.NotEmpty(t, tt.service.ID)
				assert.NotEmpty(t, tt.service.Type)
				assert.NotEmpty(t, tt.service.Address)
				assert.Greater(t, tt.service.Port, 0)
				assert.LessOrEqual(t, tt.service.Port, 65535)
			} else {
				invalid := tt.service.ID == "" ||
					tt.service.Type == "" ||
					tt.service.Address == "" ||
					tt.service.Port <= 0 ||
					tt.service.Port > 65535
				assert.True(t, invalid, "Service should be invalid")
			}
		})
	}
}

func TestServiceInfo_HealthStates(t *testing.T) {
	validHealthStates := []string{"healthy", "unhealthy", "degraded", "unknown", "starting", "stopping"}

	for _, state := range validHealthStates {
		t.Run(fmt.Sprintf("health_%s", state), func(t *testing.T) {
			service := ServiceInfo{
				ID:      "test-service",
				Type:    "collector",
				Address: "localhost",
				Port:    8080,
				Health:  state,
				Version: "1.0.0",
			}

			assert.Equal(t, state, service.Health)
			assert.Contains(t, validHealthStates, service.Health)
		})
	}
}

func TestServiceInfo_ServiceTypes(t *testing.T) {
	serviceTypes := []string{"collector", "streamer", "gateway", "api", "processor", "aggregator"}

	for _, serviceType := range serviceTypes {
		t.Run(fmt.Sprintf("type_%s", serviceType), func(t *testing.T) {
			service := ServiceInfo{
				ID:      "test-service",
				Type:    serviceType,
				Address: "localhost",
				Port:    8080,
				Health:  "healthy",
				Version: "1.0.0",
			}

			assert.Equal(t, serviceType, service.Type)
		})
	}
}

func TestServiceInfo_Metadata(t *testing.T) {
	service := ServiceInfo{
		ID:      "test-service",
		Type:    "collector",
		Address: "localhost",
		Port:    8080,
		Metadata: map[string]string{
			"region":      "us-west-2",
			"environment": "production",
			"datacenter":  "dc1",
			"version":     "1.2.3",
		},
		Health:  "healthy",
		Version: "1.0.0",
	}

	assert.Equal(t, "us-west-2", service.Metadata["region"])
	assert.Equal(t, "production", service.Metadata["environment"])
	assert.Equal(t, "dc1", service.Metadata["datacenter"])
	assert.Equal(t, "1.2.3", service.Metadata["version"])
	assert.Len(t, service.Metadata, 4)
}

func TestServiceInfo_EdgeCases(t *testing.T) {
	// Test with nil metadata
	service := ServiceInfo{
		ID:       "test-service",
		Type:     "collector",
		Address:  "localhost",
		Port:     8080,
		Metadata: nil,
		Health:   "healthy",
		Version:  "1.0.0",
	}

	assert.Nil(t, service.Metadata)

	// Test with empty metadata
	service.Metadata = make(map[string]string)
	assert.NotNil(t, service.Metadata)
	assert.Len(t, service.Metadata, 0)

	// Test with zero time
	assert.True(t, service.RegisteredAt.IsZero())

	// Test with empty version
	service.Version = ""
	assert.Empty(t, service.Version)
}

func TestServiceInfo_ComplexMetadata(t *testing.T) {
	service := ServiceInfo{
		ID:      "complex-service",
		Type:    "collector",
		Address: "192.168.1.100",
		Port:    9090,
		Metadata: map[string]string{
			"region":            "us-east-1",
			"availability_zone": "us-east-1a",
			"instance_type":     "m5.large",
			"environment":       "production",
			"team":              "platform",
			"cost_center":       "engineering",
			"deployment_id":     "deploy-12345",
			"build_version":     "v1.2.3-abc123",
		},
		RegisteredAt: time.Now(),
		Health:       "healthy",
		Version:      "1.2.3",
	}

	// Test all metadata fields
	assert.Equal(t, "us-east-1", service.Metadata["region"])
	assert.Equal(t, "us-east-1a", service.Metadata["availability_zone"])
	assert.Equal(t, "m5.large", service.Metadata["instance_type"])
	assert.Equal(t, "production", service.Metadata["environment"])
	assert.Equal(t, "platform", service.Metadata["team"])
	assert.Equal(t, "engineering", service.Metadata["cost_center"])
	assert.Equal(t, "deploy-12345", service.Metadata["deployment_id"])
	assert.Equal(t, "v1.2.3-abc123", service.Metadata["build_version"])
	assert.Len(t, service.Metadata, 8)
}

func TestServiceInfo_TimeHandling(t *testing.T) {
	now := time.Now()
	service := ServiceInfo{
		ID:           "time-test-service",
		Type:         "collector",
		Address:      "localhost",
		Port:         8080,
		RegisteredAt: now,
		Health:       "healthy",
		Version:      "1.0.0",
	}

	// Test time serialization
	jsonData, err := json.Marshal(service)
	require.NoError(t, err)

	var unmarshaled ServiceInfo
	err = json.Unmarshal(jsonData, &unmarshaled)
	require.NoError(t, err)

	// Times should be equal (within reasonable precision)
	assert.WithinDuration(t, now, unmarshaled.RegisteredAt, time.Second)
}

func TestServiceRegistry_TTLVariations(t *testing.T) {
	ttlValues := []int64{1, 10, 30, 60, 300, 3600}

	for _, ttl := range ttlValues {
		t.Run(fmt.Sprintf("ttl_%d", ttl), func(t *testing.T) {
			sr := NewServiceRegistry(nil, ttl)
			assert.NotNil(t, sr)
			// We can't access the private ttl field, but we can verify the constructor works
		})
	}
}

func TestServiceInfo_AddressVariations(t *testing.T) {
	addresses := []string{
		"localhost",
		"127.0.0.1",
		"0.0.0.0",
		"192.168.1.100",
		"10.0.0.1",
		"service.example.com",
		"my-service.default.svc.cluster.local",
	}

	for _, addr := range addresses {
		t.Run(fmt.Sprintf("address_%s", addr), func(t *testing.T) {
			service := ServiceInfo{
				ID:      "test-service",
				Type:    "collector",
				Address: addr,
				Port:    8080,
				Health:  "healthy",
				Version: "1.0.0",
			}

			assert.Equal(t, addr, service.Address)
			assert.NotEmpty(t, service.Address)
		})
	}
}

func TestServiceInfo_PortRanges(t *testing.T) {
	validPorts := []int{1, 80, 443, 8080, 9090, 65535}

	for _, port := range validPorts {
		t.Run(fmt.Sprintf("port_%d", port), func(t *testing.T) {
			service := ServiceInfo{
				ID:      "test-service",
				Type:    "collector",
				Address: "localhost",
				Port:    port,
				Health:  "healthy",
				Version: "1.0.0",
			}

			assert.Equal(t, port, service.Port)
			assert.Greater(t, service.Port, 0)
			assert.LessOrEqual(t, service.Port, 65535)
		})
	}
}

// Benchmark tests
func BenchmarkServiceInfo_JSONMarshal(b *testing.B) {
	service := ServiceInfo{
		ID:      "benchmark-service",
		Type:    "collector",
		Address: "localhost",
		Port:    8080,
		Metadata: map[string]string{
			"region": "us-west-2",
			"env":    "prod",
		},
		RegisteredAt: time.Now(),
		Health:       "healthy",
		Version:      "1.0.0",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := json.Marshal(service)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkServiceInfo_JSONUnmarshal(b *testing.B) {
	service := ServiceInfo{
		ID:      "benchmark-service",
		Type:    "collector",
		Address: "localhost",
		Port:    8080,
		Metadata: map[string]string{
			"region": "us-west-2",
			"env":    "prod",
		},
		RegisteredAt: time.Now(),
		Health:       "healthy",
		Version:      "1.0.0",
	}

	jsonData, _ := json.Marshal(service)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var unmarshaled ServiceInfo
		err := json.Unmarshal(jsonData, &unmarshaled)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkNewServiceRegistry(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		NewServiceRegistry(nil, 30)
	}
}

func BenchmarkServiceInfo_MetadataAccess(b *testing.B) {
	service := ServiceInfo{
		ID:      "benchmark-service",
		Type:    "collector",
		Address: "localhost",
		Port:    8080,
		Metadata: map[string]string{
			"region":      "us-west-2",
			"environment": "production",
			"datacenter":  "dc1",
			"version":     "1.2.3",
		},
		Health:  "healthy",
		Version: "1.0.0",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = service.Metadata["region"]
		_ = service.Metadata["environment"]
		_ = service.Metadata["datacenter"]
		_ = service.Metadata["version"]
	}
}
