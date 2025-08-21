package api

import (
	"bytes"
	//"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/swaggo/swag"
)

// SwaggerInfo holds exported Swagger Info so clients can modify it
var SwaggerInfo = &swag.Spec{
	Version:          "1.0.0",
	Host:             "localhost:8080",
	BasePath:         "/api/v1",
	Schemes:          []string{"http", "https"},
	Title:            "Telemetry Pipeline API",
	Description:      "GPU Telemetry Pipeline REST API for querying telemetry data across distributed clusters",
	InfoInstanceName: "swagger",
	SwaggerTemplate:  docTemplate,
	LeftDelim:        "{{",
	RightDelim:       "}}",
}

// SwaggerUIOpts configures the Swagger UI middleware
type SwaggerUIOpts struct {
	// BasePath for the UI path, defaults to: /
	BasePath string
	// Path combines with BasePath for the full UI path, defaults to: docs
	Path string
	// SpecURL the url to find the spec for
	SpecURL string
	// Title for the documentation site
	Title string
	// SwaggerJSON the swagger JSON spec
	SwaggerJSON string

	// CDN URLs for Swagger UI assets
	SwaggerURL       string
	SwaggerPresetURL string
	SwaggerStylesURL string
	Favicon32        string
	Favicon16        string
}

// EnsureDefaults sets default values for missing options
func (opts *SwaggerUIOpts) EnsureDefaults() {
	if opts.BasePath == "" {
		opts.BasePath = "/"
	}
	if opts.Path == "" {
		opts.Path = "docs"
	}
	if opts.SpecURL == "" {
		opts.SpecURL = "/swagger.json"
	}
	if opts.Title == "" {
		opts.Title = "Telemetry Pipeline API Documentation"
	}
	if opts.SwaggerURL == "" {
		opts.SwaggerURL = "https://unpkg.com/swagger-ui-dist/swagger-ui-bundle.js"
	}
	if opts.SwaggerPresetURL == "" {
		opts.SwaggerPresetURL = "https://unpkg.com/swagger-ui-dist/swagger-ui-standalone-preset.js"
	}
	if opts.SwaggerStylesURL == "" {
		opts.SwaggerStylesURL = "https://unpkg.com/swagger-ui-dist/swagger-ui.css"
	}
	if opts.Favicon32 == "" {
		opts.Favicon32 = "https://unpkg.com/swagger-ui-dist/favicon-32x32.png"
	}
	if opts.Favicon16 == "" {
		opts.Favicon16 = "https://unpkg.com/swagger-ui-dist/favicon-16x16.png"
	}
}

// SwaggerHandler returns the Swagger specification as JSON
func SwaggerHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Generate dynamic swagger spec based on current server info
		spec := generateSwaggerSpec(c.Request)
		c.Header("Content-Type", "application/json")
		c.JSON(http.StatusOK, spec)
	}
}

// SwaggerUIHandler serves the Swagger UI
func SwaggerUIHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		opts := SwaggerUIOpts{
			SpecURL: "/swagger.json",
			Title:   "Telemetry Pipeline API Documentation",
		}
		opts.EnsureDefaults()

		tmpl := template.Must(template.New("swaggerui").Parse(swaggerUITemplate))
		buf := bytes.NewBuffer(nil)
		if err := tmpl.Execute(buf, &opts); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to render Swagger UI"})
			return
		}

		c.Header("Content-Type", "text/html")
		c.Data(http.StatusOK, "text/html", buf.Bytes())
	}
}

// generateSwaggerSpec creates the OpenAPI 3.0 specification dynamically
func generateSwaggerSpec(req *http.Request) map[string]interface{} {
	host := req.Host
	if host == "" {
		host = "localhost:8080"
	}

	scheme := "http"
	if req.TLS != nil {
		scheme = "https"
	}

	return map[string]interface{}{
		"openapi": "3.0.0",
		"info": map[string]interface{}{
			"title":       "Telemetry Pipeline API",
			"description": "GPU Telemetry Pipeline REST API for querying telemetry data across distributed clusters.\n\nThis API provides access to GPU telemetry data collected from multiple sources and processed through a distributed pipeline. The system supports both same-cluster and cross-cluster deployments.",
			"version":     "1.0.0",
			"contact": map[string]interface{}{
				"name":  "Telemetry Pipeline Team",
				"email": "telemetry@company.com",
			},
			"license": map[string]interface{}{
				"name": "Apache 2.0",
				"url":  "http://www.apache.org/licenses/LICENSE-2.0.html",
			},
		},
		"servers": []map[string]interface{}{
			{
				"url":         fmt.Sprintf("%s://%s", scheme, host),
				"description": "Current server",
			},
			{
				"url":         "http://localhost:8080",
				"description": "Local development server",
			},
		},
		"paths": generatePaths(),
		"components": map[string]interface{}{
			"schemas":    generateSchemas(),
			"responses":  generateResponses(),
			"parameters": generateParameters(),
		},
		"tags": []map[string]interface{}{
			{
				"name":        "health",
				"description": "Health check endpoints",
			},
			{
				"name":        "gpus",
				"description": "GPU management and listing",
			},
			{
				"name":        "telemetry",
				"description": "Telemetry data queries",
			},
			{
				"name":        "metrics",
				"description": "System metrics and monitoring",
			},
		},
	}
}

// generatePaths creates the OpenAPI paths specification
func generatePaths() map[string]interface{} {
	return map[string]interface{}{
		"/health": map[string]interface{}{
			"get": map[string]interface{}{
				"tags":        []string{"health"},
				"summary":     "Health check",
				"description": "Check the health status of the API service",
				"operationId": "getHealth",
				"responses": map[string]interface{}{
					"200": map[string]interface{}{
						"description": "Service is healthy",
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"$ref": "#/components/schemas/HealthResponse",
								},
							},
						},
					},
				},
			},
		},
		"/api/v1/gpus": map[string]interface{}{
			"get": map[string]interface{}{
				"tags":        []string{"gpus"},
				"summary":     "List all GPUs",
				"description": "Retrieve a list of all GPUs in the system with their metadata",
				"operationId": "listGPUs",
				"parameters": []map[string]interface{}{
					{
						"$ref": "#/components/parameters/PageParam",
					},
					{
						"$ref": "#/components/parameters/PageSizeParam",
					},
				},
				"responses": map[string]interface{}{
					"200": map[string]interface{}{
						"description": "List of GPUs",
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"$ref": "#/components/schemas/GPUListResponse",
								},
							},
						},
					},
					"500": map[string]interface{}{
						"$ref": "#/components/responses/InternalServerError",
					},
				},
			},
		},
		"/api/v1/gpus/{gpu_id}/telemetry": map[string]interface{}{
			"get": map[string]interface{}{
				"tags":        []string{"telemetry"},
				"summary":     "Get telemetry data for a specific GPU",
				"description": "Retrieve telemetry data for a specific GPU with optional time range filtering",
				"operationId": "getGPUTelemetry",
				"parameters": []map[string]interface{}{
					{
						"name":        "gpu_id",
						"in":          "path",
						"description": "Unique identifier of the GPU",
						"required":    true,
						"schema": map[string]interface{}{
							"type":    "string",
							"example": "GPU-5fd4f087-86f3-7a43-b711-4771313afc50",
						},
					},
					{
						"name":        "start_time",
						"in":          "query",
						"description": "Start time for telemetry data (ISO 8601 format)",
						"required":    false,
						"schema": map[string]interface{}{
							"type":    "string",
							"format":  "date-time",
							"example": "2025-07-18T20:00:00Z",
						},
					},
					{
						"name":        "end_time",
						"in":          "query",
						"description": "End time for telemetry data (ISO 8601 format)",
						"required":    false,
						"schema": map[string]interface{}{
							"type":    "string",
							"format":  "date-time",
							"example": "2025-07-18T21:00:00Z",
						},
					},
					{
						"name":        "metric_name",
						"in":          "query",
						"description": "Filter by specific metric name",
						"required":    false,
						"schema": map[string]interface{}{
							"type": "string",
							"enum": []string{
								"DCGM_FI_DEV_GPU_UTIL",
								"DCGM_FI_DEV_MEM_COPY_UTIL",
								"DCGM_FI_DEV_GPU_TEMP",
								"DCGM_FI_DEV_POWER_USAGE",
								"DCGM_FI_DEV_PCIE_REPLAY_COUNTER",
							},
							"example": "DCGM_FI_DEV_GPU_UTIL",
						},
					},
					{
						"$ref": "#/components/parameters/PageParam",
					},
					{
						"$ref": "#/components/parameters/PageSizeParam",
					},
				},
				"responses": map[string]interface{}{
					"200": map[string]interface{}{
						"description": "Telemetry data for the specified GPU",
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"$ref": "#/components/schemas/TelemetryResponse",
								},
							},
						},
					},
					"400": map[string]interface{}{
						"$ref": "#/components/responses/BadRequest",
					},
					"404": map[string]interface{}{
						"$ref": "#/components/responses/NotFound",
					},
					"500": map[string]interface{}{
						"$ref": "#/components/responses/InternalServerError",
					},
				},
			},
		},
		"/metrics": map[string]interface{}{
			"get": map[string]interface{}{
				"tags":        []string{"metrics"},
				"summary":     "Prometheus metrics",
				"description": "Get Prometheus-format metrics for monitoring",
				"operationId": "getMetrics",
				"responses": map[string]interface{}{
					"200": map[string]interface{}{
						"description": "Prometheus metrics",
						"content": map[string]interface{}{
							"text/plain": map[string]interface{}{
								"schema": map[string]interface{}{
									"type": "string",
								},
							},
						},
					},
				},
			},
		},
	}
}

// generateSchemas creates the OpenAPI schema definitions
func generateSchemas() map[string]interface{} {
	return map[string]interface{}{
		"HealthResponse": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"status": map[string]interface{}{
					"type":        "string",
					"description": "Health status",
					"example":     "ok",
				},
				"timestamp": map[string]interface{}{
					"type":        "string",
					"format":      "date-time",
					"description": "Timestamp of health check",
					"example":     time.Now().Format(time.RFC3339),
				},
				"version": map[string]interface{}{
					"type":        "string",
					"description": "API version",
					"example":     "1.0.0",
				},
			},
			"required": []string{"status", "timestamp"},
		},
		"GPU": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"id": map[string]interface{}{
					"type":        "string",
					"description": "Unique GPU identifier",
					"example":     "GPU-5fd4f087-86f3-7a43-b711-4771313afc50",
				},
				"device": map[string]interface{}{
					"type":        "string",
					"description": "Device name",
					"example":     "nvidia0",
				},
				"model_name": map[string]interface{}{
					"type":        "string",
					"description": "GPU model name",
					"example":     "NVIDIA H100 80GB HBM3",
				},
				"hostname": map[string]interface{}{
					"type":        "string",
					"description": "Host machine name",
					"example":     "mtv5-dgx1-hgpu-031",
				},
				"last_seen": map[string]interface{}{
					"type":        "string",
					"format":      "date-time",
					"description": "Last time telemetry was received",
					"example":     "2025-07-18T20:42:34Z",
				},
			},
			"required": []string{"id", "device", "hostname"},
		},
		"GPUListResponse": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"gpus": map[string]interface{}{
					"type": "array",
					"items": map[string]interface{}{
						"$ref": "#/components/schemas/GPU",
					},
				},
				"total": map[string]interface{}{
					"type":        "integer",
					"description": "Total number of GPUs",
					"example":     64,
				},
				"page": map[string]interface{}{
					"type":        "integer",
					"description": "Current page number",
					"example":     1,
				},
				"page_size": map[string]interface{}{
					"type":        "integer",
					"description": "Number of items per page",
					"example":     50,
				},
			},
			"required": []string{"gpus", "total", "page", "page_size"},
		},
		"TelemetryData": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"timestamp": map[string]interface{}{
					"type":        "string",
					"format":      "date-time",
					"description": "Timestamp of the telemetry reading",
					"example":     "2025-07-18T20:42:34Z",
				},
				"metric_name": map[string]interface{}{
					"type":        "string",
					"description": "Name of the telemetry metric",
					"example":     "DCGM_FI_DEV_GPU_UTIL",
				},
				"value": map[string]interface{}{
					"type":        "string",
					"description": "Metric value",
					"example":     "85",
				},
				"device": map[string]interface{}{
					"type":        "string",
					"description": "Device identifier",
					"example":     "nvidia0",
				},
				"hostname": map[string]interface{}{
					"type":        "string",
					"description": "Host machine name",
					"example":     "mtv5-dgx1-hgpu-031",
				},
			},
			"required": []string{"timestamp", "metric_name", "value", "device", "hostname"},
		},
		"TelemetryResponse": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"gpu_id": map[string]interface{}{
					"type":        "string",
					"description": "GPU identifier",
					"example":     "GPU-5fd4f087-86f3-7a43-b711-4771313afc50",
				},
				"telemetry": map[string]interface{}{
					"type": "array",
					"items": map[string]interface{}{
						"$ref": "#/components/schemas/TelemetryData",
					},
				},
				"total": map[string]interface{}{
					"type":        "integer",
					"description": "Total number of telemetry records",
					"example":     1500,
				},
				"page": map[string]interface{}{
					"type":        "integer",
					"description": "Current page number",
					"example":     1,
				},
				"page_size": map[string]interface{}{
					"type":        "integer",
					"description": "Number of items per page",
					"example":     100,
				},
			},
			"required": []string{"gpu_id", "telemetry", "total", "page", "page_size"},
		},
		"ErrorResponse": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"error": map[string]interface{}{
					"type":        "string",
					"description": "Error message",
					"example":     "Invalid request parameters",
				},
				"code": map[string]interface{}{
					"type":        "integer",
					"description": "Error code",
					"example":     400,
				},
				"timestamp": map[string]interface{}{
					"type":        "string",
					"format":      "date-time",
					"description": "Error timestamp",
					"example":     time.Now().Format(time.RFC3339),
				},
			},
			"required": []string{"error", "code", "timestamp"},
		},
	}
}

// generateResponses creates common response definitions
func generateResponses() map[string]interface{} {
	return map[string]interface{}{
		"BadRequest": map[string]interface{}{
			"description": "Bad Request",
			"content": map[string]interface{}{
				"application/json": map[string]interface{}{
					"schema": map[string]interface{}{
						"$ref": "#/components/schemas/ErrorResponse",
					},
				},
			},
		},
		"NotFound": map[string]interface{}{
			"description": "Resource not found",
			"content": map[string]interface{}{
				"application/json": map[string]interface{}{
					"schema": map[string]interface{}{
						"$ref": "#/components/schemas/ErrorResponse",
					},
				},
			},
		},
		"InternalServerError": map[string]interface{}{
			"description": "Internal Server Error",
			"content": map[string]interface{}{
				"application/json": map[string]interface{}{
					"schema": map[string]interface{}{
						"$ref": "#/components/schemas/ErrorResponse",
					},
				},
			},
		},
	}
}

// generateParameters creates common parameter definitions
func generateParameters() map[string]interface{} {
	return map[string]interface{}{
		"PageParam": map[string]interface{}{
			"name":        "page",
			"in":          "query",
			"description": "Page number for pagination (1-based)",
			"required":    false,
			"schema": map[string]interface{}{
				"type":    "integer",
				"minimum": 1,
				"default": 1,
				"example": 1,
			},
		},
		"PageSizeParam": map[string]interface{}{
			"name":        "page_size",
			"in":          "query",
			"description": "Number of items per page (max 1000)",
			"required":    false,
			"schema": map[string]interface{}{
				"type":    "integer",
				"minimum": 1,
				"maximum": 1000,
				"default": 50,
				"example": 50,
			},
		},
	}
}

// Swagger UI HTML template
const swaggerUITemplate = `
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <title>{{ .Title }}</title>
    <link rel="stylesheet" type="text/css" href="{{ .SwaggerStylesURL }}" >
    <link rel="icon" type="image/png" href="{{ .Favicon32 }}" sizes="32x32" />
    <link rel="icon" type="image/png" href="{{ .Favicon16 }}" sizes="16x16" />
    <style>
        html {
            box-sizing: border-box;
            overflow: -moz-scrollbars-vertical;
            overflow-y: scroll;
        }
        *, *:before, *:after {
            box-sizing: inherit;
        }
        body {
            margin:0;
            background: #fafafa;
        }
        .swagger-ui .topbar {
            background-color: #1f2937;
        }
        .swagger-ui .topbar .download-url-wrapper .download-url-button {
            background-color: #3b82f6;
            border-color: #3b82f6;
        }
        .swagger-ui .info .title {
            color: #1f2937;
        }
        .custom-header {
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            color: white;
            padding: 20px;
            margin-bottom: 20px;
            border-radius: 8px;
            box-shadow: 0 4px 6px rgba(0, 0, 0, 0.1);
        }
        .custom-header h1 {
            margin: 0;
            font-size: 24px;
            font-weight: 600;
        }
        .custom-header p {
            margin: 8px 0 0 0;
            opacity: 0.9;
            font-size: 14px;
        }
        .deployment-info {
            background: #f8fafc;
            border: 1px solid #e2e8f0;
            border-radius: 6px;
            padding: 16px;
            margin: 16px 0;
        }
        .deployment-info h3 {
            margin: 0 0 8px 0;
            color: #374151;
            font-size: 16px;
        }
        .deployment-info ul {
            margin: 8px 0 0 0;
            padding-left: 20px;
        }
        .deployment-info li {
            color: #6b7280;
            font-size: 14px;
            margin: 4px 0;
        }
    </style>
</head>
<body>
    <div style="max-width: 1200px; margin: 0 auto; padding: 20px;">
        <div class="custom-header">
            <h1>🚀 Telemetry Pipeline API</h1>
            <p>GPU Telemetry Data API - Supports both same-cluster and cross-cluster deployments</p>
        </div>
        
        <div class="deployment-info">
            <h3>📋 Deployment Flexibility</h3>
            <p>This API works identically across different deployment patterns:</p>
            <ul>
                <li><strong>Same Cluster:</strong> All components (streamers, collectors, API) in one Kubernetes cluster</li>
                <li><strong>Cross-Cluster:</strong> Components distributed across multiple clusters with shared Redis</li>
                <li><strong>Edge Computing:</strong> Streamers at edge locations, collectors in central data centers</li>
                <li><strong>Hybrid:</strong> Mixed deployment patterns based on specific requirements</li>
            </ul>
        </div>
        
        <div class="deployment-info">
            <h3>🔧 Try the API</h3>
            <p>Use the interactive documentation below to explore and test the API endpoints:</p>
            <ul>
                <li><strong>/health</strong> - Check API service health</li>
                <li><strong>/api/v1/gpus</strong> - List all GPUs in the system</li>
                <li><strong>/api/v1/gpus/{gpu_id}/telemetry</strong> - Get telemetry data for specific GPU</li>
                <li><strong>/metrics</strong> - Prometheus metrics for monitoring</li>
            </ul>
        </div>
    </div>
    
    <div id="swagger-ui"></div>

    <script src="{{ .SwaggerURL }}"></script>
    <script src="{{ .SwaggerPresetURL }}"></script>
    <script>
    window.onload = function() {
        const ui = SwaggerUIBundle({
            url: '{{ .SpecURL }}',
            dom_id: '#swagger-ui',
            deepLinking: true,
            presets: [
                SwaggerUIBundle.presets.apis,
                SwaggerUIStandalonePreset
            ],
            plugins: [
                SwaggerUIBundle.plugins.DownloadUrl
            ],
            layout: "StandaloneLayout",
            tryItOutEnabled: true,
            supportedSubmitMethods: ['get', 'post', 'put', 'delete', 'patch'],
            onComplete: function() {
                console.log('Swagger UI loaded successfully');
            },
            onFailure: function(data) {
                console.error('Failed to load Swagger UI:', data);
            }
        });
        window.ui = ui;
    }
    </script>
</body>
</html>
`

const docTemplate = `{
    "schemes": {{ marshal .Schemes }},
    "swagger": "2.0",
    "info": {
        "description": "{{escape .Description}}",
        "title": "{{.Title}}",
        "contact": {},
        "version": "{{.Version}}"
    },
    "host": "{{.Host}}",
    "basePath": "{{.BasePath}}",
    "paths": {}
}`
