package api

import (
	"net/http"

	"github.com/gorilla/mux"
	httpSwagger "github.com/swaggo/http-swagger"
)

// SetupRouter sets up the HTTP router with all routes
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
