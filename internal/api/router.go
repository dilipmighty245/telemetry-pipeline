package api

import (
	"net/http"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/mux"
	httpSwagger "github.com/swaggo/http-swagger"
)

// SetupRouter sets up the HTTP router with all routes (Gorilla Mux version)
func SetupRouter(handler *APIHandler) *mux.Router {
	router := mux.NewRouter()

	// Apply middleware
	router.Use(handler.CORS)
	router.Use(handler.LoggingMiddleware)

	// Health check endpoint
	router.HandleFunc("/health", handler.Health).Methods("GET")

	// API v1 routes
	v1 := router.PathPrefix("/api/v1").Subrouter()

	// GPU endpoints
	v1.HandleFunc("/gpus", handler.ListGPUs).Methods("GET")
	v1.HandleFunc("/gpus/{id}/telemetry", handler.GetGPUTelemetry).Methods("GET")

	// Stats endpoint
	v1.HandleFunc("/stats", handler.GetStats).Methods("GET")

	// Swagger documentation
	router.PathPrefix("/swagger/").Handler(httpSwagger.WrapHandler)

	// Serve static files (if any)
	router.PathPrefix("/").Handler(http.FileServer(http.Dir("./static/")))

	return router
}

// SetupGinRouter sets up the Gin router with enhanced Swagger UI
func SetupGinRouter(handler *APIHandler) *gin.Engine {
	// Set Gin mode based on environment
	gin.SetMode(gin.ReleaseMode)
	
	router := gin.New()

	// Add middleware
	router.Use(gin.Logger())
	router.Use(gin.Recovery())
	router.Use(cors.Default())

	// Enhanced Swagger endpoints
	router.GET("/swagger.json", SwaggerHandler())
	router.GET("/docs", SwaggerUIHandler())
	router.GET("/swagger/*any", gin.WrapH(httpSwagger.WrapHandler))

	// Health check endpoint
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":    "ok",
			"timestamp": "2025-01-01T00:00:00Z", // This would be dynamic in real implementation
			"version":   "1.0.0",
		})
	})

	// API v1 group
	v1 := router.Group("/api/v1")
	{
		// GPU endpoints
		v1.GET("/gpus", func(c *gin.Context) {
			// This would call the actual handler in real implementation
			c.JSON(http.StatusOK, gin.H{
				"gpus": []gin.H{
					{
						"id":         "GPU-5fd4f087-86f3-7a43-b711-4771313afc50",
						"device":     "nvidia0",
						"model_name": "NVIDIA H100 80GB HBM3",
						"hostname":   "mtv5-dgx1-hgpu-031",
						"last_seen":  "2025-07-18T20:42:34Z",
					},
				},
				"total":     1,
				"page":      1,
				"page_size": 50,
			})
		})

		v1.GET("/gpus/:gpu_id/telemetry", func(c *gin.Context) {
			gpuID := c.Param("gpu_id")
			// This would call the actual handler in real implementation
			c.JSON(http.StatusOK, gin.H{
				"gpu_id": gpuID,
				"telemetry": []gin.H{
					{
						"timestamp":   "2025-07-18T20:42:34Z",
						"metric_name": "DCGM_FI_DEV_GPU_UTIL",
						"value":       "85",
						"device":      "nvidia0",
						"hostname":    "mtv5-dgx1-hgpu-031",
					},
				},
				"total":     1,
				"page":      1,
				"page_size": 100,
			})
		})

		// Stats endpoint
		v1.GET("/stats", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{
				"total_gpus":     64,
				"total_records":  150000,
				"unique_hosts":   8,
				"last_updated":   "2025-07-18T20:42:34Z",
			})
		})
	}

	// Metrics endpoint (Prometheus format)
	router.GET("/metrics", func(c *gin.Context) {
		c.String(http.StatusOK, `# HELP telemetry_records_total Total number of telemetry records
# TYPE telemetry_records_total counter
telemetry_records_total 150000

# HELP telemetry_gpus_total Total number of GPUs
# TYPE telemetry_gpus_total gauge
telemetry_gpus_total 64

# HELP telemetry_api_requests_total Total number of API requests
# TYPE telemetry_api_requests_total counter
telemetry_api_requests_total 1234
`)
	})

	return router
}
