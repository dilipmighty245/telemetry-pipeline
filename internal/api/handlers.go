package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/dilipmighty245/telemetry-pipeline/internal/collector"
	"github.com/dilipmighty245/telemetry-pipeline/pkg/logging"
	"github.com/dilipmighty245/telemetry-pipeline/pkg/models"
	"github.com/gorilla/mux"
)

// APIHandler handles HTTP requests for the telemetry API
type APIHandler struct {
	database *collector.DatabaseService
}

// NewAPIHandler creates a new API handler
func NewAPIHandler(database *collector.DatabaseService) *APIHandler {
	return &APIHandler{
		database: database,
	}
}

// HealthResponse represents health check response
type HealthResponse struct {
	Status    string    `json:"status"`
	Timestamp time.Time `json:"timestamp"`
	Service   string    `json:"service"`
}

// ErrorResponse represents error response
type ErrorResponse struct {
	Error   string `json:"error"`
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// ListGPUsResponse represents the response for listing GPUs
type ListGPUsResponse struct {
	GPUs  []models.GPU `json:"gpus"`
	Count int          `json:"count"`
}

// Health godoc
// @Summary Health check
// @Description Returns the health status of the API service
// @Tags health
// @Produce json
// @Success 200 {object} HealthResponse
// @Router /health [get]
func (h *APIHandler) Health(w http.ResponseWriter, r *http.Request) {
	response := HealthResponse{
		Status:    "healthy",
		Timestamp: time.Now(),
		Service:   "telemetry-api",
	}

	// Check database health
	if h.database != nil && !h.database.Health() {
		response.Status = "unhealthy"
		w.WriteHeader(http.StatusServiceUnavailable)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// ListGPUs godoc
// @Summary List all GPUs
// @Description Returns a list of all GPUs for which telemetry data is available
// @Tags gpus
// @Produce json
// @Success 200 {object} ListGPUsResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/gpus [get]
func (h *APIHandler) ListGPUs(w http.ResponseWriter, r *http.Request) {
	if h.database == nil {
		h.writeError(w, "Database service not available", http.StatusServiceUnavailable)
		return
	}

	gpus, err := h.database.GetGPUs()
	if err != nil {
		logging.Errorf("Failed to get GPUs: %v", err)
		h.writeError(w, "Failed to retrieve GPUs", http.StatusInternalServerError)
		return
	}

	response := ListGPUsResponse{
		GPUs:  gpus,
		Count: len(gpus),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)

	logging.Debugf("Listed %d GPUs", len(gpus))
}

// GetGPUTelemetry godoc
// @Summary Get telemetry data for a specific GPU
// @Description Returns telemetry data for a specific GPU with optional time window filters
// @Tags gpus
// @Produce json
// @Param id path string true "GPU ID"
// @Param start_time query string false "Start time (RFC3339 format)"
// @Param end_time query string false "End time (RFC3339 format)"
// @Param limit query int false "Maximum number of records to return (default: 100, max: 1000)"
// @Param offset query int false "Number of records to skip (default: 0)"
// @Success 200 {object} models.TelemetryQueryResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/gpus/{id}/telemetry [get]
func (h *APIHandler) GetGPUTelemetry(w http.ResponseWriter, r *http.Request) {
	if h.database == nil {
		h.writeError(w, "Database service not available", http.StatusServiceUnavailable)
		return
	}

	// Extract GPU ID from path
	vars := mux.Vars(r)
	gpuID := vars["id"]
	if gpuID == "" {
		h.writeError(w, "GPU ID is required", http.StatusBadRequest)
		return
	}

	// Parse query parameters
	query := &models.TelemetryQuery{
		GPUID:  gpuID,
		Limit:  100, // Default limit
		Offset: 0,   // Default offset
	}

	// Parse start_time
	if startTimeStr := r.URL.Query().Get("start_time"); startTimeStr != "" {
		startTime, err := time.Parse(time.RFC3339, startTimeStr)
		if err != nil {
			h.writeError(w, "Invalid start_time format, use RFC3339", http.StatusBadRequest)
			return
		}
		query.StartTime = startTime
	}

	// Parse end_time
	if endTimeStr := r.URL.Query().Get("end_time"); endTimeStr != "" {
		endTime, err := time.Parse(time.RFC3339, endTimeStr)
		if err != nil {
			h.writeError(w, "Invalid end_time format, use RFC3339", http.StatusBadRequest)
			return
		}
		query.EndTime = endTime
	}

	// Parse limit
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		limit, err := strconv.Atoi(limitStr)
		if err != nil || limit <= 0 {
			h.writeError(w, "Invalid limit parameter", http.StatusBadRequest)
			return
		}
		if limit > 1000 {
			limit = 1000 // Cap at 1000
		}
		query.Limit = limit
	}

	// Parse offset
	if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
		offset, err := strconv.Atoi(offsetStr)
		if err != nil || offset < 0 {
			h.writeError(w, "Invalid offset parameter", http.StatusBadRequest)
			return
		}
		query.Offset = offset
	}

	// Validate time range
	if !query.StartTime.IsZero() && !query.EndTime.IsZero() && query.StartTime.After(query.EndTime) {
		h.writeError(w, "start_time cannot be after end_time", http.StatusBadRequest)
		return
	}

	// Query telemetry data
	response, err := h.database.QueryTelemetryData(query)
	if err != nil {
		logging.Errorf("Failed to query telemetry data for GPU %s: %v", gpuID, err)
		h.writeError(w, "Failed to retrieve telemetry data", http.StatusInternalServerError)
		return
	}

	// Check if GPU exists (if no data found)
	if len(response.DataPoints) == 0 && response.TotalCount == 0 {
		// Check if GPU exists at all
		gpus, err := h.database.GetGPUs()
		if err == nil {
			found := false
			for _, gpu := range gpus {
				if gpu.GPUID == gpuID {
					found = true
					break
				}
			}
			if !found {
				h.writeError(w, "GPU not found", http.StatusNotFound)
				return
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)

	logging.Debugf("Retrieved %d telemetry records for GPU %s", len(response.DataPoints), gpuID)
}

// GetStats godoc
// @Summary Get telemetry statistics
// @Description Returns overall statistics about the telemetry data
// @Tags stats
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/stats [get]
func (h *APIHandler) GetStats(w http.ResponseWriter, r *http.Request) {
	if h.database == nil {
		h.writeError(w, "Database service not available", http.StatusServiceUnavailable)
		return
	}

	stats, err := h.database.GetTelemetryStats()
	if err != nil {
		logging.Errorf("Failed to get telemetry stats: %v", err)
		h.writeError(w, "Failed to retrieve statistics", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)

	logging.Debugf("Retrieved telemetry statistics")
}

// writeError writes an error response
func (h *APIHandler) writeError(w http.ResponseWriter, message string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)

	errorResponse := ErrorResponse{
		Error:   http.StatusText(code),
		Code:    code,
		Message: message,
	}

	json.NewEncoder(w).Encode(errorResponse)
}

// CORS middleware
func (h *APIHandler) CORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// LoggingMiddleware logs HTTP requests
func (h *APIHandler) LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Create a response writer wrapper to capture status code
		wrapper := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(wrapper, r)

		duration := time.Since(start)
		logging.Infof("%s %s %d %v", r.Method, r.URL.Path, wrapper.statusCode, duration)
	})
}

// responseWriter wraps http.ResponseWriter to capture status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}
