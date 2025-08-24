// Package models defines the data structures used throughout the telemetry pipeline.
// It includes models for telemetry data, API requests/responses, and CSV record structures.
// All models are designed to work with GORM for database operations and JSON for API serialization.
package models

import (
	"time"

	"gorm.io/gorm"
)

// TelemetryData represents the core telemetry data model used throughout the pipeline.
// It contains GPU metrics and metadata with support for GORM database operations
// and JSON serialization for API responses.
type TelemetryData struct {
	ID         uint      `json:"id"`          // Database primary key
	Timestamp  time.Time `json:"timestamp"`   // When the metric was collected
	MetricName string    `json:"metric_name"` // Name of the metric (e.g., "gpu_utilization")
	GPUID      string    `json:"gpu_id"`      // GPU identifier
	Device     string    `json:"device"`      // Device name or identifier
	UUID       string    `json:"uuid"`        // Unique identifier for the GPU
	ModelName  string    `json:"model_name"`  // GPU model name
	Hostname   string    `json:"hostname"`    // Host machine name
	Container  string    `json:"container"`   // Container name (if applicable)
	Pod        string    `json:"pod"`         // Kubernetes pod name (if applicable)
	Namespace  string    `json:"namespace"`   // Kubernetes namespace (if applicable)
	Value      float64   `json:"value"`       // The actual metric value
	LabelsRaw  string    `json:"labels_raw"`  // Raw labels string for additional metadata
	CreatedAt  time.Time `json:"created_at"`  // Database creation timestamp
	UpdatedAt  time.Time `json:"updated_at"`  // Database last update timestamp
}

// GPU represents GPU information for API responses.
// This is a simplified view of GPU data used in API endpoints
// that don't require the full telemetry data structure.
type GPU struct {
	GPUID     string `json:"gpu_id"`     // GPU identifier
	UUID      string `json:"uuid"`       // Unique identifier for the GPU
	ModelName string `json:"model_name"` // GPU model name
	Hostname  string `json:"hostname"`   // Host machine name
	Device    string `json:"device"`     // Device name or identifier
}

// TelemetryQuery represents query parameters for telemetry data API requests.
// It supports filtering by GPU ID, time range, and pagination.
type TelemetryQuery struct {
	GPUID     string    `json:"gpu_id" form:"gpu_id"`         // Filter by specific GPU ID
	StartTime time.Time `json:"start_time" form:"start_time"` // Start of time range filter
	EndTime   time.Time `json:"end_time" form:"end_time"`     // End of time range filter
	Limit     int       `json:"limit" form:"limit"`           // Maximum number of results to return
	Offset    int       `json:"offset" form:"offset"`         // Number of results to skip (for pagination)
}

// TelemetryQueryResponse represents the response structure for telemetry data queries.
// It includes the actual data points along with pagination metadata.
type TelemetryQueryResponse struct {
	DataPoints []TelemetryData `json:"data_points"` // The telemetry data points matching the query
	TotalCount int64           `json:"total_count"` // Total number of matching records (for pagination)
	HasMore    bool            `json:"has_more"`    // Whether there are more results available
}

// CSVRecord represents a single CSV record structure for importing telemetry data.
// All fields are strings to handle CSV parsing, and values are converted to appropriate
// types when creating TelemetryData instances.
type CSVRecord struct {
	Timestamp  string `csv:"timestamp"`   // Timestamp as string (will be parsed to time.Time)
	MetricName string `csv:"metric_name"` // Name of the metric
	GPUID      string `csv:"gpu_id"`      // GPU identifier
	Device     string `csv:"device"`      // Device name or identifier
	UUID       string `csv:"uuid"`        // Unique identifier for the GPU
	ModelName  string `csv:"modelName"`   // GPU model name (note: different case from JSON)
	Hostname   string `csv:"Hostname"`    // Host machine name (note: different case from JSON)
	Container  string `csv:"container"`   // Container name
	Pod        string `csv:"pod"`         // Kubernetes pod name
	Namespace  string `csv:"namespace"`   // Kubernetes namespace
	Value      string `csv:"value"`       // Metric value as string (will be parsed to float64)
	LabelsRaw  string `csv:"labels_raw"`  // Raw labels string
}

// TableName returns the database table name for TelemetryData.
// This method is used by GORM to determine the table name for database operations.
//
// Returns:
//   - string: The database table name ("telemetry_data")
func (TelemetryData) TableName() string {
	return "telemetry_data"
}

// BeforeCreate is a GORM hook that runs before creating a new record in the database.
// It automatically sets the CreatedAt and UpdatedAt timestamps if they are not already set.
//
// Parameters:
//   - tx: The GORM database transaction
//
// Returns:
//   - error: Always returns nil (no validation errors)
func (t *TelemetryData) BeforeCreate(tx *gorm.DB) error {
	if t.CreatedAt.IsZero() {
		t.CreatedAt = time.Now()
	}
	if t.UpdatedAt.IsZero() {
		t.UpdatedAt = time.Now()
	}
	return nil
}

// BeforeUpdate is a GORM hook that runs before updating an existing record in the database.
// It automatically updates the UpdatedAt timestamp to the current time.
//
// Parameters:
//   - tx: The GORM database transaction
//
// Returns:
//   - error: Always returns nil (no validation errors)
func (t *TelemetryData) BeforeUpdate(tx *gorm.DB) error {
	t.UpdatedAt = time.Now()
	return nil
}
