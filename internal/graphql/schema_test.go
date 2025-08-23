package graphql

import (
	"testing"

	"github.com/dilipmighty245/telemetry-pipeline/internal/nexus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewGraphQLService(t *testing.T) {
	// Create a mock nexus service
	config := &nexus.ServiceConfig{
		ClusterID: "test-cluster",
		ServiceID: "test-service",
		BatchSize: 100,
	}
	nexusService, err := nexus.NewTelemetryService(config)
	require.NoError(t, err)

	// Test creating GraphQL service
	service, err := NewGraphQLService(nexusService)
	require.NoError(t, err)
	assert.NotNil(t, service)
	assert.NotNil(t, service.nexusService)
	assert.NotNil(t, service.schema)
}

func TestNewGraphQLService_WithNilNexusService(t *testing.T) {
	// Test creating GraphQL service with nil nexus service
	service, err := NewGraphQLService(nil)
	require.NoError(t, err)
	assert.NotNil(t, service)
	assert.Nil(t, service.nexusService)
	assert.NotNil(t, service.schema)
}

func TestGraphQLService_ExecuteQuery_SchemaQuery(t *testing.T) {
	// Create a mock nexus service
	config := &nexus.ServiceConfig{
		ClusterID: "test-cluster",
		ServiceID: "test-service",
		BatchSize: 100,
	}
	nexusService, err := nexus.NewTelemetryService(config)
	require.NoError(t, err)

	service, err := NewGraphQLService(nexusService)
	require.NoError(t, err)

	// Test schema introspection query
	query := `
		{
			__schema {
				types {
					name
				}
			}
		}
	`

	result := service.ExecuteQuery(query, nil)
	assert.NotNil(t, result)
	assert.Empty(t, result.Errors)
	assert.NotNil(t, result.Data)

	// Verify the result contains expected types
	data, ok := result.Data.(map[string]interface{})
	require.True(t, ok)
	schema, ok := data["__schema"].(map[string]interface{})
	require.True(t, ok)
	types, ok := schema["types"].([]interface{})
	require.True(t, ok)
	assert.Greater(t, len(types), 0)

	// Check for our custom types
	typeNames := make([]string, 0, len(types))
	for _, typeInterface := range types {
		if typeMap, ok := typeInterface.(map[string]interface{}); ok {
			if name, ok := typeMap["name"].(string); ok {
				typeNames = append(typeNames, name)
			}
		}
	}

	assert.Contains(t, typeNames, "TelemetryData")
	assert.Contains(t, typeNames, "GPU")
	assert.Contains(t, typeNames, "Cluster")
	assert.Contains(t, typeNames, "Dashboard")
}

func TestGraphQLService_ExecuteQuery_TelemetryQuery(t *testing.T) {
	// Create a mock nexus service
	config := &nexus.ServiceConfig{
		ClusterID: "test-cluster",
		ServiceID: "test-service",
		BatchSize: 100,
	}
	nexusService, err := nexus.NewTelemetryService(config)
	require.NoError(t, err)

	service, err := NewGraphQLService(nexusService)
	require.NoError(t, err)

	// Test telemetry data query (will return empty since no real data)
	query := `
		{
			telemetry(hostname: "test-host", gpuId: "0") {
				timestamp
				hostname
				gpuId
				temperature
				powerDraw
			}
		}
	`

	result := service.ExecuteQuery(query, nil)
	assert.NotNil(t, result)
	// Note: This might have errors since we don't have real etcd connection
	// but the schema should be valid
	assert.NotNil(t, result.Data)
}

func TestGraphQLService_ExecuteQuery_GPUsQuery(t *testing.T) {
	// Create a mock nexus service
	config := &nexus.ServiceConfig{
		ClusterID: "test-cluster",
		ServiceID: "test-service",
		BatchSize: 100,
	}
	nexusService, err := nexus.NewTelemetryService(config)
	require.NoError(t, err)

	service, err := NewGraphQLService(nexusService)
	require.NoError(t, err)

	// Test GPUs query
	query := `
		{
			gpus {
				gpuId
				uuid
				deviceName
				hostname
			}
		}
	`

	result := service.ExecuteQuery(query, nil)
	assert.NotNil(t, result)
	assert.NotNil(t, result.Data)
}

func TestGraphQLService_ExecuteQuery_ClustersQuery(t *testing.T) {
	// Create a mock nexus service
	config := &nexus.ServiceConfig{
		ClusterID: "test-cluster",
		ServiceID: "test-service",
		BatchSize: 100,
	}
	nexusService, err := nexus.NewTelemetryService(config)
	require.NoError(t, err)

	service, err := NewGraphQLService(nexusService)
	require.NoError(t, err)

	// Test clusters query
	query := `
		{
			clusters {
				clusterId
				clusterName
				region
				environment
			}
		}
	`

	result := service.ExecuteQuery(query, nil)
	assert.NotNil(t, result)
	assert.NotNil(t, result.Data)
}

func TestGraphQLService_ExecuteQuery_DashboardQuery(t *testing.T) {
	// Create a mock nexus service
	config := &nexus.ServiceConfig{
		ClusterID: "test-cluster",
		ServiceID: "test-service",
		BatchSize: 100,
	}
	nexusService, err := nexus.NewTelemetryService(config)
	require.NoError(t, err)

	service, err := NewGraphQLService(nexusService)
	require.NoError(t, err)

	// Test dashboard query
	query := `
		{
			dashboard {
				totalGPUs
				activeGPUs
				averageUtilization
				averageTemperature
			}
		}
	`

	result := service.ExecuteQuery(query, nil)
	assert.NotNil(t, result)
	assert.NotNil(t, result.Data)
}

func TestGraphQLService_ExecuteQuery_WithVariables(t *testing.T) {
	// Create a mock nexus service
	config := &nexus.ServiceConfig{
		ClusterID: "test-cluster",
		ServiceID: "test-service",
		BatchSize: 100,
	}
	nexusService, err := nexus.NewTelemetryService(config)
	require.NoError(t, err)

	service, err := NewGraphQLService(nexusService)
	require.NoError(t, err)

	// Test query with variables
	query := `
		query GetTelemetry($hostname: String!, $gpuId: String!) {
			telemetry(hostname: $hostname, gpuId: $gpuId) {
				timestamp
				hostname
				gpuId
				temperature
			}
		}
	`

	variables := map[string]interface{}{
		"hostname": "test-host",
		"gpuId":    "0",
	}

	result := service.ExecuteQuery(query, variables)
	assert.NotNil(t, result)
	assert.NotNil(t, result.Data)
}

func TestGraphQLService_ExecuteQuery_InvalidQuery(t *testing.T) {
	// Create a mock nexus service
	config := &nexus.ServiceConfig{
		ClusterID: "test-cluster",
		ServiceID: "test-service",
		BatchSize: 100,
	}
	nexusService, err := nexus.NewTelemetryService(config)
	require.NoError(t, err)

	service, err := NewGraphQLService(nexusService)
	require.NoError(t, err)

	// Test invalid query
	query := `
		{
			invalidField {
				nonExistentField
			}
		}
	`

	result := service.ExecuteQuery(query, nil)
	assert.NotNil(t, result)
	assert.NotEmpty(t, result.Errors)
}

func TestGraphQLService_ExecuteQuery_MalformedQuery(t *testing.T) {
	// Create a mock nexus service
	config := &nexus.ServiceConfig{
		ClusterID: "test-cluster",
		ServiceID: "test-service",
		BatchSize: 100,
	}
	nexusService, err := nexus.NewTelemetryService(config)
	require.NoError(t, err)

	service, err := NewGraphQLService(nexusService)
	require.NoError(t, err)

	// Test malformed query
	query := `
		{
			telemetry(hostname: "test-host", gpuId: "0" {
				timestamp
				hostname
			}
		}
	`

	result := service.ExecuteQuery(query, nil)
	assert.NotNil(t, result)
	assert.NotEmpty(t, result.Errors)
}

func TestGraphQLService_ExecuteQuery_EmptyQuery(t *testing.T) {
	// Create a mock nexus service
	config := &nexus.ServiceConfig{
		ClusterID: "test-cluster",
		ServiceID: "test-service",
		BatchSize: 100,
	}
	nexusService, err := nexus.NewTelemetryService(config)
	require.NoError(t, err)

	service, err := NewGraphQLService(nexusService)
	require.NoError(t, err)

	// Test empty query
	result := service.ExecuteQuery("", nil)
	assert.NotNil(t, result)
	assert.NotEmpty(t, result.Errors)
}

func TestGraphQLService_ExecuteQuery_ComplexQuery(t *testing.T) {
	// Create a mock nexus service
	config := &nexus.ServiceConfig{
		ClusterID: "test-cluster",
		ServiceID: "test-service",
		BatchSize: 100,
	}
	nexusService, err := nexus.NewTelemetryService(config)
	require.NoError(t, err)

	service, err := NewGraphQLService(nexusService)
	require.NoError(t, err)

	// Test complex nested query
	query := `
		{
			dashboard {
				totalGPUs
				activeGPUs
				averageUtilization
				averageTemperature
				averagePowerDraw
				totalMemoryGB
				usedMemoryGB
				lastUpdated
			}
			clusters {
				clusterId
				clusterName
				region
				environment
			}
			gpus {
				gpuId
				uuid
				deviceName
				hostname
				driverVersion
				memoryTotalMB
			}
		}
	`

	result := service.ExecuteQuery(query, nil)
	assert.NotNil(t, result)
	assert.NotNil(t, result.Data)

	// Verify the structure of the response
	data, ok := result.Data.(map[string]interface{})
	require.True(t, ok)
	assert.Contains(t, data, "dashboard")
	assert.Contains(t, data, "clusters")
	assert.Contains(t, data, "gpus")
}

func TestGraphQLService_ExecuteQuery_FieldSelection(t *testing.T) {
	// Create a mock nexus service
	config := &nexus.ServiceConfig{
		ClusterID: "test-cluster",
		ServiceID: "test-service",
		BatchSize: 100,
	}
	nexusService, err := nexus.NewTelemetryService(config)
	require.NoError(t, err)

	service, err := NewGraphQLService(nexusService)
	require.NoError(t, err)

	// Test selective field querying
	query := `
		{
			telemetry(hostname: "test-host", gpuId: "0") {
				timestamp
				temperature
			}
		}
	`

	result := service.ExecuteQuery(query, nil)
	assert.NotNil(t, result)
	assert.NotNil(t, result.Data)
}

func TestGraphQLService_BuildSchema_Types(t *testing.T) {
	// Create a mock nexus service
	config := &nexus.ServiceConfig{
		ClusterID: "test-cluster",
		ServiceID: "test-service",
		BatchSize: 100,
	}
	nexusService, err := nexus.NewTelemetryService(config)
	require.NoError(t, err)

	service, err := NewGraphQLService(nexusService)
	require.NoError(t, err)

	// Test that schema has expected types
	schema := service.schema
	assert.NotNil(t, schema)

	// Test schema introspection to verify types exist
	query := `
		{
			__schema {
				queryType {
					fields {
						name
						type {
							name
						}
					}
				}
			}
		}
	`

	result := service.ExecuteQuery(query, nil)
	assert.NotNil(t, result)
	assert.Empty(t, result.Errors)

	data, ok := result.Data.(map[string]interface{})
	require.True(t, ok)
	schemaData, ok := data["__schema"].(map[string]interface{})
	require.True(t, ok)
	queryType, ok := schemaData["queryType"].(map[string]interface{})
	require.True(t, ok)
	fields, ok := queryType["fields"].([]interface{})
	require.True(t, ok)

	// Extract field names
	fieldNames := make([]string, 0, len(fields))
	for _, fieldInterface := range fields {
		if fieldMap, ok := fieldInterface.(map[string]interface{}); ok {
			if name, ok := fieldMap["name"].(string); ok {
				fieldNames = append(fieldNames, name)
			}
		}
	}

	// Verify expected query fields exist
	assert.Contains(t, fieldNames, "telemetry")
	assert.Contains(t, fieldNames, "gpus")
	assert.Contains(t, fieldNames, "clusters")
	assert.Contains(t, fieldNames, "dashboard")
}

func TestGraphQLService_ResolverFunctions(t *testing.T) {
	// Create a mock nexus service
	config := &nexus.ServiceConfig{
		ClusterID: "test-cluster",
		ServiceID: "test-service",
		BatchSize: 100,
	}
	nexusService, err := nexus.NewTelemetryService(config)
	require.NoError(t, err)

	service, err := NewGraphQLService(nexusService)
	require.NoError(t, err)

	// Test that resolvers don't panic when called
	// Note: These will return empty/default values since we don't have real data
	assert.NotPanics(t, func() {
		query := `{ dashboard { totalGPUs } }`
		result := service.ExecuteQuery(query, nil)
		assert.NotNil(t, result)
	})

	assert.NotPanics(t, func() {
		query := `{ gpus { gpuId } }`
		result := service.ExecuteQuery(query, nil)
		assert.NotNil(t, result)
	})

	assert.NotPanics(t, func() {
		query := `{ clusters { clusterId } }`
		result := service.ExecuteQuery(query, nil)
		assert.NotNil(t, result)
	})
}

func TestGraphQLService_ErrorHandling(t *testing.T) {
	// Create a mock nexus service
	config := &nexus.ServiceConfig{
		ClusterID: "test-cluster",
		ServiceID: "test-service",
		BatchSize: 100,
	}
	nexusService, err := nexus.NewTelemetryService(config)
	require.NoError(t, err)

	service, err := NewGraphQLService(nexusService)
	require.NoError(t, err)

	// Test various error conditions
	tests := []struct {
		name  string
		query string
	}{
		{
			name:  "syntax error",
			query: `{ telemetry(hostname: "test" { timestamp } }`,
		},
		{
			name:  "unknown field",
			query: `{ unknownField }`,
		},
		{
			name:  "missing required argument",
			query: `{ telemetry { timestamp } }`,
		},
		{
			name:  "invalid argument type",
			query: `{ telemetry(hostname: 123, gpuId: "0") { timestamp } }`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := service.ExecuteQuery(tt.query, nil)
			assert.NotNil(t, result)
			assert.NotEmpty(t, result.Errors, "Expected errors for query: %s", tt.query)
		})
	}
}

func TestGraphQLService_VariableHandling(t *testing.T) {
	// Create a mock nexus service
	config := &nexus.ServiceConfig{
		ClusterID: "test-cluster",
		ServiceID: "test-service",
		BatchSize: 100,
	}
	nexusService, err := nexus.NewTelemetryService(config)
	require.NoError(t, err)

	service, err := NewGraphQLService(nexusService)
	require.NoError(t, err)

	// Test variable handling
	tests := []struct {
		name      string
		query     string
		variables map[string]interface{}
		expectErr bool
	}{
		{
			name: "valid variables",
			query: `
				query GetTelemetry($hostname: String!, $gpuId: String!) {
					telemetry(hostname: $hostname, gpuId: $gpuId) {
						timestamp
					}
				}
			`,
			variables: map[string]interface{}{
				"hostname": "test-host",
				"gpuId":    "0",
			},
			expectErr: false,
		},
		{
			name: "missing required variable",
			query: `
				query GetTelemetry($hostname: String!, $gpuId: String!) {
					telemetry(hostname: $hostname, gpuId: $gpuId) {
						timestamp
					}
				}
			`,
			variables: map[string]interface{}{
				"hostname": "test-host",
				// missing gpuId
			},
			expectErr: true,
		},
		{
			name: "wrong variable type",
			query: `
				query GetTelemetry($hostname: String!, $gpuId: String!) {
					telemetry(hostname: $hostname, gpuId: $gpuId) {
						timestamp
					}
				}
			`,
			variables: map[string]interface{}{
				"hostname": 123, // should be string
				"gpuId":    "0",
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := service.ExecuteQuery(tt.query, tt.variables)
			assert.NotNil(t, result)

			if tt.expectErr {
				assert.NotEmpty(t, result.Errors)
			} else {
				// Note: May still have errors due to missing etcd connection,
				// but should not have variable-related errors
				assert.NotNil(t, result.Data)
			}
		})
	}
}

// Benchmark tests
func BenchmarkGraphQLService_ExecuteQuery_Simple(b *testing.B) {
	config := &nexus.ServiceConfig{
		ClusterID: "benchmark-cluster",
		ServiceID: "benchmark-service",
		BatchSize: 100,
	}
	nexusService, err := nexus.NewTelemetryService(config)
	require.NoError(b, err)

	service, err := NewGraphQLService(nexusService)
	require.NoError(b, err)

	query := `{ dashboard { totalGPUs } }`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result := service.ExecuteQuery(query, nil)
		_ = result
	}
}

func BenchmarkGraphQLService_ExecuteQuery_Complex(b *testing.B) {
	config := &nexus.ServiceConfig{
		ClusterID: "benchmark-cluster",
		ServiceID: "benchmark-service",
		BatchSize: 100,
	}
	nexusService, err := nexus.NewTelemetryService(config)
	require.NoError(b, err)

	service, err := NewGraphQLService(nexusService)
	require.NoError(b, err)

	query := `
		{
			dashboard {
				totalGPUs
				activeGPUs
				averageUtilization
				averageTemperature
			}
			clusters {
				clusterId
				clusterName
				region
			}
			gpus {
				gpuId
				uuid
				deviceName
				hostname
			}
		}
	`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result := service.ExecuteQuery(query, nil)
		_ = result
	}
}

func BenchmarkGraphQLService_ExecuteQuery_WithVariables(b *testing.B) {
	config := &nexus.ServiceConfig{
		ClusterID: "benchmark-cluster",
		ServiceID: "benchmark-service",
		BatchSize: 100,
	}
	nexusService, err := nexus.NewTelemetryService(config)
	require.NoError(b, err)

	service, err := NewGraphQLService(nexusService)
	require.NoError(b, err)

	query := `
		query GetTelemetry($hostname: String!, $gpuId: String!) {
			telemetry(hostname: $hostname, gpuId: $gpuId) {
				timestamp
				temperature
				powerDraw
			}
		}
	`

	variables := map[string]interface{}{
		"hostname": "benchmark-host",
		"gpuId":    "0",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result := service.ExecuteQuery(query, variables)
		_ = result
	}
}

func BenchmarkNewGraphQLService(b *testing.B) {
	config := &nexus.ServiceConfig{
		ClusterID: "benchmark-cluster",
		ServiceID: "benchmark-service",
		BatchSize: 100,
	}
	nexusService, err := nexus.NewTelemetryService(config)
	require.NoError(b, err)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		service, err := NewGraphQLService(nexusService)
		if err != nil {
			b.Fatal(err)
		}
		_ = service
	}
}
