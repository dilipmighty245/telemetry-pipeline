package graphql

import (
	"time"

	"github.com/dilipmighty245/telemetry-pipeline/internal/nexus"
	"github.com/graphql-go/graphql"
	log "github.com/sirupsen/logrus"
)

// GraphQLService provides GraphQL interface for telemetry data (compatible with existing gateway)
type GraphQLService struct {
	nexusService *nexus.TelemetryService
	schema       graphql.Schema
}

// NewGraphQLService creates a new GraphQL service for telemetry data
func NewGraphQLService(nexusService *nexus.TelemetryService) (*GraphQLService, error) {
	service := &GraphQLService{
		nexusService: nexusService,
	}

	schema, err := service.buildSchema()
	if err != nil {
		return nil, err
	}

	service.schema = schema
	return service, nil
}

// ExecuteQuery executes a GraphQL query and returns the result
func (s *GraphQLService) ExecuteQuery(query string, variables map[string]interface{}) *graphql.Result {
	return graphql.Do(graphql.Params{
		Schema:         s.schema,
		RequestString:  query,
		VariableValues: variables,
	})
}

// buildSchema creates the GraphQL schema for telemetry data
func (s *GraphQLService) buildSchema() (graphql.Schema, error) {
	// Define telemetry data type
	telemetryType := graphql.NewObject(graphql.ObjectConfig{
		Name: "TelemetryData",
		Fields: graphql.Fields{
			"timestamp": &graphql.Field{
				Type: graphql.String,
			},
			"hostname": &graphql.Field{
				Type: graphql.String,
			},
			"gpuId": &graphql.Field{
				Type: graphql.String,
			},
			"uuid": &graphql.Field{
				Type: graphql.String,
			},
			"device": &graphql.Field{
				Type: graphql.String,
			},
			"modelName": &graphql.Field{
				Type: graphql.String,
			},
			"gpuUtilization": &graphql.Field{
				Type: graphql.Float,
			},
			"memoryUtilization": &graphql.Field{
				Type: graphql.Float,
			},
			"memoryUsedMB": &graphql.Field{
				Type: graphql.Float,
			},
			"memoryFreeMB": &graphql.Field{
				Type: graphql.Float,
			},
			"temperature": &graphql.Field{
				Type: graphql.Float,
			},
			"powerDraw": &graphql.Field{
				Type: graphql.Float,
			},
			"smClockMHz": &graphql.Field{
				Type: graphql.Float,
			},
			"memoryClockMHz": &graphql.Field{
				Type: graphql.Float,
			},
		},
	})

	// Define GPU type
	gpuType := graphql.NewObject(graphql.ObjectConfig{
		Name: "GPU",
		Fields: graphql.Fields{
			"id": &graphql.Field{
				Type: graphql.String,
			},
			"uuid": &graphql.Field{
				Type: graphql.String,
			},
			"hostname": &graphql.Field{
				Type: graphql.String,
			},
			"modelName": &graphql.Field{
				Type: graphql.String,
			},
			"device": &graphql.Field{
				Type: graphql.String,
			},
			"status": &graphql.Field{
				Type: graphql.String,
			},
		},
	})

	// Define cluster type
	clusterType := graphql.NewObject(graphql.ObjectConfig{
		Name: "Cluster",
		Fields: graphql.Fields{
			"id": &graphql.Field{
				Type: graphql.String,
			},
			"name": &graphql.Field{
				Type: graphql.String,
			},
			"totalHosts": &graphql.Field{
				Type: graphql.Int,
			},
			"totalGPUs": &graphql.Field{
				Type: graphql.Int,
			},
			"activeHosts": &graphql.Field{
				Type: graphql.Int,
			},
			"activeGPUs": &graphql.Field{
				Type: graphql.Int,
			},
		},
	})

	// Define query root
	rootQuery := graphql.NewObject(graphql.ObjectConfig{
		Name: "Query",
		Fields: graphql.Fields{
			// Get telemetry data with filters
			"telemetry": &graphql.Field{
				Type: graphql.NewList(telemetryType),
				Args: graphql.FieldConfigArgument{
					"hostname": &graphql.ArgumentConfig{
						Type: graphql.String,
					},
					"gpuId": &graphql.ArgumentConfig{
						Type: graphql.String,
					},
					"limit": &graphql.ArgumentConfig{
						Type:         graphql.Int,
						DefaultValue: 100,
					},
					"startTime": &graphql.ArgumentConfig{
						Type: graphql.String,
					},
					"endTime": &graphql.ArgumentConfig{
						Type: graphql.String,
					},
				},
				Resolve: s.resolveTelemetry,
			},

			// Get all GPUs
			"gpus": &graphql.Field{
				Type: graphql.NewList(gpuType),
				Args: graphql.FieldConfigArgument{
					"hostname": &graphql.ArgumentConfig{
						Type: graphql.String,
					},
				},
				Resolve: s.resolveGPUs,
			},

			// Get clusters
			"clusters": &graphql.Field{
				Type:    graphql.NewList(clusterType),
				Resolve: s.resolveClusters,
			},

			// Get latest telemetry for dashboard
			"dashboard": &graphql.Field{
				Type:    graphql.NewList(telemetryType),
				Resolve: s.resolveDashboard,
			},
		},
	})

	// Create schema
	return graphql.NewSchema(graphql.SchemaConfig{
		Query: rootQuery,
	})
}

// Resolvers - these use your existing Nexus service

func (s *GraphQLService) resolveTelemetry(p graphql.ResolveParams) (interface{}, error) {
	hostname, _ := p.Args["hostname"].(string)
	gpuId, _ := p.Args["gpuId"].(string)
	limit, _ := p.Args["limit"].(int)
	startTimeStr, _ := p.Args["startTime"].(string)
	endTimeStr, _ := p.Args["endTime"].(string)

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

	// Use your existing Nexus service to get telemetry data
	if hostname != "" && gpuId != "" {
		telemetryData, err := s.nexusService.GetGPUTelemetryData(hostname, gpuId, startTime, endTime, limit)
		if err != nil {
			log.Errorf("Failed to get telemetry data: %v", err)
			return []interface{}{}, nil
		}
		return telemetryData, nil
	}

	// Return mock data for now if no specific filters
	return []map[string]interface{}{
		{
			"timestamp":         time.Now().Format(time.RFC3339),
			"hostname":          "example-host",
			"gpuId":             "0",
			"uuid":              "GPU-12345",
			"device":            "nvidia0",
			"modelName":         "NVIDIA H100",
			"gpuUtilization":    95.5,
			"memoryUtilization": 80.2,
			"memoryUsedMB":      64000.0,
			"memoryFreeMB":      16000.0,
			"temperature":       65.0,
			"powerDraw":         300.0,
			"smClockMHz":        1500.0,
			"memoryClockMHz":    1200.0,
		},
	}, nil
}

func (s *GraphQLService) resolveGPUs(p graphql.ResolveParams) (interface{}, error) {
	hostname, _ := p.Args["hostname"].(string)

	// Use your existing Nexus service to get GPU list
	if hostname != "" {
		// Get GPUs for specific host
		hostInfo, err := s.nexusService.GetHostInfo(hostname)
		if err != nil {
			log.Errorf("Failed to get host info: %v", err)
			return []interface{}{}, nil
		}

		var gpus []map[string]interface{}
		// Convert host GPU info to GraphQL format
		// This would be implemented based on your Nexus service structure
		_ = hostInfo // placeholder

		return gpus, nil
	}

	// Return mock data for now
	return []map[string]interface{}{
		{
			"id":        "0",
			"uuid":      "GPU-12345",
			"hostname":  "example-host",
			"modelName": "NVIDIA H100",
			"device":    "nvidia0",
			"status":    "active",
		},
	}, nil
}

func (s *GraphQLService) resolveClusters(p graphql.ResolveParams) (interface{}, error) {
	// Use your existing Nexus service to get cluster info
	cluster, err := s.nexusService.GetClusterInfo()
	if err != nil {
		log.Errorf("Failed to get cluster info: %v", err)
		return []interface{}{}, nil
	}

	return []map[string]interface{}{
		{
			"id":          cluster.ClusterID,
			"name":        cluster.ClusterName,
			"totalHosts":  cluster.Metadata.TotalHosts,
			"totalGPUs":   cluster.Metadata.TotalGPUs,
			"activeHosts": cluster.Metadata.ActiveHosts,
			"activeGPUs":  cluster.Metadata.ActiveGPUs,
		},
	}, nil
}

func (s *GraphQLService) resolveDashboard(p graphql.ResolveParams) (interface{}, error) {
	// Get latest telemetry data for dashboard view
	// This would use your Nexus service to get recent data from all GPUs
	return []map[string]interface{}{
		{
			"timestamp":         time.Now().Format(time.RFC3339),
			"hostname":          "host-1",
			"gpuId":             "0",
			"uuid":              "GPU-12345",
			"device":            "nvidia0",
			"modelName":         "NVIDIA H100",
			"gpuUtilization":    95.5,
			"memoryUtilization": 80.2,
			"temperature":       65.0,
			"powerDraw":         300.0,
		},
		{
			"timestamp":         time.Now().Format(time.RFC3339),
			"hostname":          "host-2",
			"gpuId":             "1",
			"uuid":              "GPU-67890",
			"device":            "nvidia1",
			"modelName":         "NVIDIA H100",
			"gpuUtilization":    88.3,
			"memoryUtilization": 75.1,
			"temperature":       62.0,
			"powerDraw":         285.0,
		},
	}, nil
}
