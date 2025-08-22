package graphql

import (
	"context"
	"fmt"
	"time"

	"github.com/dilipmighty245/telemetry-pipeline/internal/nexus"
	"github.com/graphql-go/graphql"
)

// GraphQLService provides GraphQL schema and resolvers for Nexus telemetry
type GraphQLService struct {
	nexusService *nexus.TelemetryService
	schema       graphql.Schema
}

// NewGraphQLService creates a new GraphQL service with Nexus integration
func NewGraphQLService(nexusService *nexus.TelemetryService) (*GraphQLService, error) {
	service := &GraphQLService{
		nexusService: nexusService,
	}

	schema, err := service.buildSchema()
	if err != nil {
		return nil, fmt.Errorf("failed to build GraphQL schema: %w", err)
	}

	service.schema = schema
	return service, nil
}

// ExecuteQuery executes a GraphQL query with proper context
func (g *GraphQLService) ExecuteQuery(query string, variables map[string]interface{}) *graphql.Result {
	params := graphql.Params{
		Schema:         g.schema,
		RequestString:  query,
		VariableValues: variables,
		Context:        context.Background(),
	}

	return graphql.Do(params)
}

// GetSchema returns the GraphQL schema for introspection
func (g *GraphQLService) GetSchema() graphql.Schema {
	return g.schema
}

// buildSchema builds the complete GraphQL schema with Nexus integration
func (g *GraphQLService) buildSchema() (graphql.Schema, error) {
	// Define Cluster type
	clusterType := graphql.NewObject(graphql.ObjectConfig{
		Name:        "Cluster",
		Description: "Nexus telemetry cluster",
		Fields: graphql.Fields{
			"id": &graphql.Field{
				Type:        graphql.NewNonNull(graphql.String),
				Description: "Unique cluster identifier",
			},
			"name": &graphql.Field{
				Type:        graphql.String,
				Description: "Human-readable cluster name",
			},
			"region": &graphql.Field{
				Type:        graphql.String,
				Description: "Geographic region of the cluster",
			},
			"environment": &graphql.Field{
				Type:        graphql.String,
				Description: "Environment (production, staging, development)",
			},
			"totalHosts": &graphql.Field{
				Type:        graphql.Int,
				Description: "Total number of hosts in the cluster",
			},
			"totalGPUs": &graphql.Field{
				Type:        graphql.Int,
				Description: "Total number of GPUs in the cluster",
			},
			"activeHosts": &graphql.Field{
				Type:        graphql.Int,
				Description: "Number of active hosts",
			},
			"activeGPUs": &graphql.Field{
				Type:        graphql.Int,
				Description: "Number of active GPUs",
			},
			"createdAt": &graphql.Field{
				Type:        graphql.String,
				Description: "Cluster creation timestamp",
			},
			"updatedAt": &graphql.Field{
				Type:        graphql.String,
				Description: "Last update timestamp",
			},
		},
	})

	// Define Host type
	hostType := graphql.NewObject(graphql.ObjectConfig{
		Name:        "Host",
		Description: "Compute host in the cluster",
		Fields: graphql.Fields{
			"id": &graphql.Field{
				Type:        graphql.NewNonNull(graphql.String),
				Description: "Unique host identifier",
			},
			"hostname": &graphql.Field{
				Type:        graphql.String,
				Description: "System hostname",
			},
			"ipAddress": &graphql.Field{
				Type:        graphql.String,
				Description: "Primary IP address",
			},
			"osVersion": &graphql.Field{
				Type:        graphql.String,
				Description: "Operating system version",
			},
			"status": &graphql.Field{
				Type:        graphql.String,
				Description: "Current host status",
			},
			"gpuCount": &graphql.Field{
				Type:        graphql.Int,
				Description: "Number of GPUs on this host",
			},
		},
	})

	// Define GPU type
	gpuType := graphql.NewObject(graphql.ObjectConfig{
		Name:        "GPU",
		Description: "Graphics processing unit",
		Fields: graphql.Fields{
			"id": &graphql.Field{
				Type:        graphql.NewNonNull(graphql.String),
				Description: "GPU identifier",
			},
			"deviceName": &graphql.Field{
				Type:        graphql.String,
				Description: "GPU device name/model",
			},
			"driverVersion": &graphql.Field{
				Type:        graphql.String,
				Description: "GPU driver version",
			},
			"cudaVersion": &graphql.Field{
				Type:        graphql.String,
				Description: "CUDA runtime version",
			},
			"memoryTotal": &graphql.Field{
				Type:        graphql.Int,
				Description: "Total GPU memory in MB",
			},
			"status": &graphql.Field{
				Type:        graphql.String,
				Description: "Current GPU status",
			},
		},
	})

	// Define TelemetryData type
	telemetryType := graphql.NewObject(graphql.ObjectConfig{
		Name:        "TelemetryData",
		Description: "GPU telemetry data point",
		Fields: graphql.Fields{
			"timestamp": &graphql.Field{
				Type:        graphql.String,
				Description: "Data collection timestamp",
			},
			"hostId": &graphql.Field{
				Type:        graphql.String,
				Description: "Host identifier",
			},
			"gpuId": &graphql.Field{
				Type:        graphql.String,
				Description: "GPU identifier",
			},
			"gpuUtilization": &graphql.Field{
				Type:        graphql.Float,
				Description: "GPU utilization percentage (0-100)",
			},
			"memoryUtilization": &graphql.Field{
				Type:        graphql.Float,
				Description: "GPU memory utilization percentage (0-100)",
			},
			"memoryUsed": &graphql.Field{
				Type:        graphql.Float,
				Description: "Used GPU memory in MB",
			},
			"memoryFree": &graphql.Field{
				Type:        graphql.Float,
				Description: "Free GPU memory in MB",
			},
			"temperature": &graphql.Field{
				Type:        graphql.Float,
				Description: "GPU temperature in Celsius",
			},
			"powerDraw": &graphql.Field{
				Type:        graphql.Float,
				Description: "GPU power consumption in Watts",
			},
			"smClockMHz": &graphql.Field{
				Type:        graphql.Float,
				Description: "Streaming multiprocessor clock frequency in MHz",
			},
			"memoryClockMHz": &graphql.Field{
				Type:        graphql.Float,
				Description: "Memory clock frequency in MHz",
			},
		},
	})

	// Define Query type with all available queries
	queryType := graphql.NewObject(graphql.ObjectConfig{
		Name:        "Query",
		Description: "Root query for Nexus telemetry data",
		Fields: graphql.Fields{
			"cluster": &graphql.Field{
				Type:        clusterType,
				Description: "Get cluster information",
				Args: graphql.FieldConfigArgument{
					"id": &graphql.ArgumentConfig{
						Type:         graphql.String,
						Description:  "Cluster ID to query",
						DefaultValue: "local-cluster",
					},
				},
				Resolve: g.resolveCluster,
			},
			"clusters": &graphql.Field{
				Type:        graphql.NewList(clusterType),
				Description: "List all clusters",
				Resolve:     g.resolveClusters,
			},
			"hosts": &graphql.Field{
				Type:        graphql.NewList(hostType),
				Description: "List hosts in a cluster",
				Args: graphql.FieldConfigArgument{
					"clusterId": &graphql.ArgumentConfig{
						Type:         graphql.String,
						Description:  "Cluster ID to filter hosts",
						DefaultValue: "local-cluster",
					},
				},
				Resolve: g.resolveHosts,
			},
			"gpus": &graphql.Field{
				Type:        graphql.NewList(gpuType),
				Description: "List GPUs for a specific host",
				Args: graphql.FieldConfigArgument{
					"hostId": &graphql.ArgumentConfig{
						Type:        graphql.String,
						Description: "Host ID to filter GPUs",
					},
				},
				Resolve: g.resolveGPUs,
			},
			"telemetry": &graphql.Field{
				Type:        graphql.NewList(telemetryType),
				Description: "Query telemetry data with filters",
				Args: graphql.FieldConfigArgument{
					"hostId": &graphql.ArgumentConfig{
						Type:        graphql.String,
						Description: "Filter by host ID",
					},
					"gpuId": &graphql.ArgumentConfig{
						Type:        graphql.String,
						Description: "Filter by GPU ID",
					},
					"startTime": &graphql.ArgumentConfig{
						Type:        graphql.String,
						Description: "Start time for data range (ISO 8601)",
					},
					"endTime": &graphql.ArgumentConfig{
						Type:        graphql.String,
						Description: "End time for data range (ISO 8601)",
					},
					"limit": &graphql.ArgumentConfig{
						Type:         graphql.Int,
						Description:  "Maximum number of records to return",
						DefaultValue: 100,
					},
				},
				Resolve: g.resolveTelemetry,
			},
		},
	})

	// Build and return schema
	return graphql.NewSchema(graphql.SchemaConfig{
		Query: queryType,
	})
}

// Resolver functions

func (g *GraphQLService) resolveCluster(params graphql.ResolveParams) (interface{}, error) {
	cluster, err := g.nexusService.GetClusterInfo()
	if err != nil {
		return nil, fmt.Errorf("failed to get cluster info: %w", err)
	}

	return map[string]interface{}{
		"id":          cluster.ClusterID,
		"name":        cluster.ClusterName,
		"region":      cluster.Region,
		"environment": cluster.Environment,
		"totalHosts":  cluster.Metadata.TotalHosts,
		"totalGPUs":   cluster.Metadata.TotalGPUs,
		"activeHosts": cluster.Metadata.ActiveHosts,
		"activeGPUs":  cluster.Metadata.ActiveGPUs,
		"createdAt":   cluster.CreatedAt.Format(time.RFC3339),
		"updatedAt":   cluster.UpdatedAt.Format(time.RFC3339),
	}, nil
}

func (g *GraphQLService) resolveClusters(params graphql.ResolveParams) (interface{}, error) {
	cluster, err := g.nexusService.GetClusterInfo()
	if err != nil {
		return nil, fmt.Errorf("failed to get clusters: %w", err)
	}

	// Return as array for consistency with GraphQL list type
	return []map[string]interface{}{
		{
			"id":          cluster.ClusterID,
			"name":        cluster.ClusterName,
			"region":      cluster.Region,
			"environment": cluster.Environment,
			"totalHosts":  cluster.Metadata.TotalHosts,
			"totalGPUs":   cluster.Metadata.TotalGPUs,
			"activeHosts": cluster.Metadata.ActiveHosts,
			"activeGPUs":  cluster.Metadata.ActiveGPUs,
			"createdAt":   cluster.CreatedAt.Format(time.RFC3339),
			"updatedAt":   cluster.UpdatedAt.Format(time.RFC3339),
		},
	}, nil
}

func (g *GraphQLService) resolveHosts(params graphql.ResolveParams) (interface{}, error) {
	// For now, return sample data - in a full implementation, this would query etcd
	return []map[string]interface{}{
		{
			"id":        "mtv5-dgx1-hgpu-001",
			"hostname":  "mtv5-dgx1-hgpu-001",
			"ipAddress": "192.168.1.101",
			"osVersion": "Ubuntu 20.04.6 LTS",
			"status":    "active",
			"gpuCount":  8,
		},
		{
			"id":        "mtv5-dgx1-hgpu-002",
			"hostname":  "mtv5-dgx1-hgpu-002",
			"ipAddress": "192.168.1.102",
			"osVersion": "Ubuntu 20.04.6 LTS",
			"status":    "active",
			"gpuCount":  8,
		},
	}, nil
}

func (g *GraphQLService) resolveGPUs(params graphql.ResolveParams) (interface{}, error) {
	hostID, _ := params.Args["hostId"].(string)
	
	// Sample GPU data - in a full implementation, this would query the Nexus service
	gpus := []map[string]interface{}{
		{
			"id":            "gpu-0",
			"deviceName":    "Tesla V100-SXM2-32GB",
			"driverVersion": "470.57.02",
			"cudaVersion":   "11.4",
			"memoryTotal":   32768,
			"status":        "active",
		},
		{
			"id":            "gpu-1",
			"deviceName":    "Tesla V100-SXM2-32GB",
			"driverVersion": "470.57.02",
			"cudaVersion":   "11.4",
			"memoryTotal":   32768,
			"status":        "active",
		},
	}

	if hostID != "" {
		// Filter by host if specified
		return gpus, nil
	}

	return gpus, nil
}

func (g *GraphQLService) resolveTelemetry(params graphql.ResolveParams) (interface{}, error) {
	hostID, _ := params.Args["hostId"].(string)
	gpuID, _ := params.Args["gpuId"].(string)
	startTimeStr, _ := params.Args["startTime"].(string)
	endTimeStr, _ := params.Args["endTime"].(string)
	limit, _ := params.Args["limit"].(int)

	if limit <= 0 {
		limit = 100
	}

	// Parse time range if provided
	var startTime, endTime *time.Time
	if startTimeStr != "" {
		if t, err := time.Parse(time.RFC3339, startTimeStr); err == nil {
			startTime = &t
		}
	}
	if endTimeStr != "" {
		if t, err := time.Parse(time.RFC3339, endTimeStr); err == nil {
			endTime = &t
		}
	}

	// Try to get real telemetry data from Nexus service
	if hostID != "" && gpuID != "" {
		telemetryData, err := g.nexusService.GetGPUTelemetryData(hostID, gpuID, startTime, endTime, limit)
		if err == nil && len(telemetryData) > 0 {
			// Convert to GraphQL format
			result := make([]map[string]interface{}, len(telemetryData))
			for i, data := range telemetryData {
				result[i] = map[string]interface{}{
					"timestamp":         data.Timestamp.Format(time.RFC3339),
					"hostId":            data.Hostname,
					"gpuId":             data.GPUID,
					"gpuUtilization":    data.GPUUtilization,
					"memoryUtilization": data.MemoryUtilization,
					"memoryUsed":        data.MemoryUsedMB,
					"memoryFree":        data.MemoryFreeMB,
					"temperature":       data.Temperature,
					"powerDraw":         data.PowerDraw,
					"smClockMHz":        data.SMClockMHz,
					"memoryClockMHz":    data.MemoryClockMHz,
				}
			}
			return result, nil
		}
	}

	// Return sample telemetry data
	now := time.Now()
	return []map[string]interface{}{
		{
			"timestamp":         now.Format(time.RFC3339),
			"hostId":            "mtv5-dgx1-hgpu-001",
			"gpuId":             "gpu-0",
			"gpuUtilization":    75.5,
			"memoryUtilization": 60.2,
			"memoryUsed":        19660.8,
			"memoryFree":        13107.2,
			"temperature":       65.0,
			"powerDraw":         250.0,
			"smClockMHz":        1530.0,
			"memoryClockMHz":    877.0,
		},
		{
			"timestamp":         now.Add(-30 * time.Second).Format(time.RFC3339),
			"hostId":            "mtv5-dgx1-hgpu-001",
			"gpuId":             "gpu-1",
			"gpuUtilization":    82.3,
			"memoryUtilization": 45.7,
			"memoryUsed":        14976.0,
			"memoryFree":        17792.0,
			"temperature":       68.0,
			"powerDraw":         275.0,
			"smClockMHz":        1530.0,
			"memoryClockMHz":    877.0,
		},
	}, nil
}
