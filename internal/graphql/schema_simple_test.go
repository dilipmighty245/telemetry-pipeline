package graphql

import (
	"testing"

	"github.com/dilipmighty245/telemetry-pipeline/internal/nexus"
	"github.com/graphql-go/graphql"
	"github.com/stretchr/testify/assert"
)

func TestNewGraphQLServiceSimple(t *testing.T) {
	service, err := NewGraphQLService(&nexus.TelemetryService{})

	assert.NoError(t, err)
	assert.NotNil(t, service)
	assert.NotNil(t, service.schema)
}

func TestGraphQLService_ExecuteQuery(t *testing.T) {
	service, err := NewGraphQLService(&nexus.TelemetryService{})
	assert.NoError(t, err)

	tests := []struct {
		name      string
		query     string
		variables map[string]interface{}
		wantErr   bool
	}{
		{
			name:      "simple telemetry query",
			query:     `{ telemetry { timestamp hostname gpuId } }`,
			variables: nil,
			wantErr:   false,
		},
		{
			name:      "gpus query",
			query:     `{ gpus { id uuid hostname modelName } }`,
			variables: nil,
			wantErr:   false,
		},
		{
			name:      "clusters query",
			query:     `{ clusters { id name totalHosts totalGPUs } }`,
			variables: nil,
			wantErr:   false,
		},
		{
			name:      "dashboard query",
			query:     `{ dashboard { timestamp hostname gpuId gpuUtilization temperature } }`,
			variables: nil,
			wantErr:   false,
		},
		{
			name:      "invalid query",
			query:     `{ invalidField }`,
			variables: nil,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := service.ExecuteQuery(tt.query, tt.variables)

			if tt.wantErr {
				assert.True(t, len(result.Errors) > 0)
			} else {
				assert.Empty(t, result.Errors)
				assert.NotNil(t, result.Data)
			}
		})
	}
}

func TestGraphQLService_BuildSchema(t *testing.T) {
	service := &GraphQLService{
		nexusService: &nexus.TelemetryService{},
	}

	schema, err := service.buildSchema()

	assert.NoError(t, err)
	assert.NotNil(t, schema)

	// Verify schema has the expected query fields
	queryType := schema.QueryType()
	assert.NotNil(t, queryType)

	fields := queryType.Fields()
	assert.Contains(t, fields, "telemetry")
	assert.Contains(t, fields, "gpus")
	assert.Contains(t, fields, "clusters")
	assert.Contains(t, fields, "dashboard")
}

func TestGraphQLService_SchemaTypes(t *testing.T) {
	service, err := NewGraphQLService(&nexus.TelemetryService{})
	assert.NoError(t, err)

	// Test that schema contains expected types
	schema := service.schema
	typeMap := schema.TypeMap()

	// Check for custom types
	assert.Contains(t, typeMap, "TelemetryData")
	assert.Contains(t, typeMap, "GPU")
	assert.Contains(t, typeMap, "Cluster")

	// Verify TelemetryData type fields
	telemetryType := typeMap["TelemetryData"]
	if objectType, ok := telemetryType.(*graphql.Object); ok {
		fields := objectType.Fields()
		assert.Contains(t, fields, "timestamp")
		assert.Contains(t, fields, "hostname")
		assert.Contains(t, fields, "gpuId")
		assert.Contains(t, fields, "gpuUtilization")
		assert.Contains(t, fields, "memoryUtilization")
		assert.Contains(t, fields, "temperature")
		assert.Contains(t, fields, "powerDraw")
	}

	// Verify GPU type fields
	gpuType := typeMap["GPU"]
	if objectType, ok := gpuType.(*graphql.Object); ok {
		fields := objectType.Fields()
		assert.Contains(t, fields, "id")
		assert.Contains(t, fields, "uuid")
		assert.Contains(t, fields, "hostname")
		assert.Contains(t, fields, "modelName")
		assert.Contains(t, fields, "status")
	}

	// Verify Cluster type fields
	clusterType := typeMap["Cluster"]
	if objectType, ok := clusterType.(*graphql.Object); ok {
		fields := objectType.Fields()
		assert.Contains(t, fields, "id")
		assert.Contains(t, fields, "name")
		assert.Contains(t, fields, "totalHosts")
		assert.Contains(t, fields, "totalGPUs")
		assert.Contains(t, fields, "activeHosts")
		assert.Contains(t, fields, "activeGPUs")
	}
}

func TestGraphQLService_ResolveDashboard(t *testing.T) {
	service, err := NewGraphQLService(&nexus.TelemetryService{})
	assert.NoError(t, err)

	params := graphql.ResolveParams{}

	result, err := service.resolveDashboard(params)

	assert.NoError(t, err)
	assert.NotNil(t, result)

	// Verify mock data structure
	dashboardData := result.([]map[string]interface{})
	assert.Len(t, dashboardData, 2) // Should return 2 mock entries

	// Verify first entry
	firstEntry := dashboardData[0]
	assert.Equal(t, "host-1", firstEntry["hostname"])
	assert.Equal(t, "0", firstEntry["gpuId"])
	assert.Equal(t, "GPU-12345", firstEntry["uuid"])
	assert.Equal(t, 95.5, firstEntry["gpuUtilization"])

	// Verify second entry
	secondEntry := dashboardData[1]
	assert.Equal(t, "host-2", secondEntry["hostname"])
	assert.Equal(t, "1", secondEntry["gpuId"])
	assert.Equal(t, "GPU-67890", secondEntry["uuid"])
	assert.Equal(t, 88.3, secondEntry["gpuUtilization"])
}
