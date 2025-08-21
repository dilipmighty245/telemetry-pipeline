package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/cf/telemetry-pipeline/internal/api"
	"github.com/cf/telemetry-pipeline/pkg/logging"
)

var (
	port     = flag.String("port", "8080", "Port to run the API server on")
	logLevel = flag.String("log-level", "info", "Log level (debug, info, warn, error)")
)

func main() {
	flag.Parse()

	// Set up logging
	logging.SetLogLevel(*logLevel, "")

	log.Printf("Starting Telemetry Pipeline API Demo Server")
	log.Printf("Port: %s", *port)
	log.Printf("Log Level: %s", *logLevel)

	// Create a dummy API handler (in real implementation, this would connect to database)
	handler := &api.APIHandler{}

	// Set up the enhanced Gin router with Swagger UI
	router := api.SetupGinRouter(handler)

	// Print available endpoints
	printEndpoints(*port)

	// Start the server
	addr := fmt.Sprintf(":%s", *port)
	log.Printf("🚀 Server starting on http://localhost%s", addr)
	log.Printf("📚 Swagger UI available at: http://localhost%s/docs", addr)
	log.Printf("📋 API spec available at: http://localhost%s/swagger.json", addr)

	if err := http.ListenAndServe(addr, router); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

func printEndpoints(port string) {
	fmt.Println("\n" + "="*80)
	fmt.Println("🚀 TELEMETRY PIPELINE API - ENHANCED SWAGGER DEMO")
	fmt.Println("="*80)
	fmt.Printf("Server will start on: http://localhost:%s\n", port)
	fmt.Println("")
	fmt.Println("📚 DOCUMENTATION ENDPOINTS:")
	fmt.Printf("   • Swagger UI:     http://localhost:%s/docs\n", port)
	fmt.Printf("   • OpenAPI Spec:   http://localhost:%s/swagger.json\n", port)
	fmt.Printf("   • Legacy Swagger: http://localhost:%s/swagger/index.html\n", port)
	fmt.Println("")
	fmt.Println("🔍 API ENDPOINTS:")
	fmt.Printf("   • Health Check:   GET  http://localhost:%s/health\n", port)
	fmt.Printf("   • List GPUs:      GET  http://localhost:%s/api/v1/gpus\n", port)
	fmt.Printf("   • GPU Telemetry:  GET  http://localhost:%s/api/v1/gpus/{gpu_id}/telemetry\n", port)
	fmt.Printf("   • System Stats:   GET  http://localhost:%s/api/v1/stats\n", port)
	fmt.Printf("   • Metrics:        GET  http://localhost:%s/metrics\n", port)
	fmt.Println("")
	fmt.Println("🎯 EXAMPLE REQUESTS:")
	fmt.Printf("   curl http://localhost:%s/health\n", port)
	fmt.Printf("   curl http://localhost:%s/api/v1/gpus\n", port)
	fmt.Printf("   curl \"http://localhost:%s/api/v1/gpus/GPU-5fd4f087-86f3-7a43-b711-4771313afc50/telemetry?start_time=2025-07-18T20:00:00Z\"\n", port)
	fmt.Println("")
	fmt.Println("✨ FEATURES DEMONSTRATED:")
	fmt.Println("   • Auto-generated OpenAPI 3.0 specification")
	fmt.Println("   • Interactive Swagger UI with try-it-out functionality")
	fmt.Println("   • Comprehensive API documentation")
	fmt.Println("   • Cross-cluster deployment information")
	fmt.Println("   • Example responses for all endpoints")
	fmt.Println("   • Prometheus metrics endpoint")
	fmt.Println("")
	fmt.Println("🌐 DEPLOYMENT COMPATIBILITY:")
	fmt.Println("   • Same Cluster: All components in one Kubernetes cluster")
	fmt.Println("   • Cross-Cluster: Distributed across multiple clusters")
	fmt.Println("   • Edge Computing: Streamers at edge, collectors centralized")
	fmt.Println("   • Hybrid: Mixed deployment patterns")
	fmt.Println("")
	fmt.Println("Press Ctrl+C to stop the server")
	fmt.Println("="*80)
}

func init() {
	// Set environment variables for demo
	os.Setenv("GIN_MODE", "release")
}
