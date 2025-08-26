// Package models defines data structures for the telemetry pipeline.
package models

import (
	"time"
)

// TelemetryData represents a single telemetry data point from GPU monitoring.
//
// This structure contains all the essential information collected from GPU
// telemetry systems including performance metrics, identification data,
// and timing information.
type TelemetryData struct {
	// Timing information
	Timestamp time.Time `json:"timestamp" db:"timestamp"`
	
	// Identification fields
	GPUID      string `json:"gpu_id" db:"gpu_id"`
	UUID       string `json:"uuid" db:"uuid"`
	Device     string `json:"device" db:"device"`
	ModelName  string `json:"model_name" db:"model_name"`
	Hostname   string `json:"hostname" db:"hostname"`
	ClusterID  string `json:"cluster_id" db:"cluster_id"`
	
	// Metric information
	MetricName string  `json:"metric_name" db:"metric_name"`
	Value      float64 `json:"value" db:"value"`
	Unit       string  `json:"unit" db:"unit"`
	
	// Kubernetes/Container metadata
	Container  string `json:"container,omitempty" db:"container"`
	Pod        string `json:"pod,omitempty" db:"pod"`
	Namespace  string `json:"namespace,omitempty" db:"namespace"`
	
	// Additional metadata
	Tags       map[string]string `json:"tags,omitempty" db:"tags"`
	Labels     map[string]string `json:"labels,omitempty" db:"labels"`
	LabelsRaw  string            `json:"labels_raw,omitempty" db:"labels_raw"`
	
	// Processing metadata
	ProcessedAt time.Time `json:"processed_at,omitempty" db:"processed_at"`
	Source      string    `json:"source,omitempty" db:"source"`
	CreatedAt   time.Time `json:"created_at,omitempty" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at,omitempty" db:"updated_at"` 
}

// TelemetryBatch represents a collection of telemetry data points.
//
// This structure is used for batch processing operations and includes
// metadata about the batch itself.
type TelemetryBatch struct {
	Data      []*TelemetryData `json:"data"`
	BatchID   string           `json:"batch_id"`
	Source    string           `json:"source"`
	Timestamp time.Time        `json:"timestamp"`
	Count     int              `json:"count"`
}

// HostInfo represents information about a host system.
type HostInfo struct {
	Hostname    string            `json:"hostname" db:"hostname"`
	ClusterID   string            `json:"cluster_id" db:"cluster_id"`
	IPAddress   string            `json:"ip_address" db:"ip_address"`
	OS          string            `json:"os" db:"os"`
	Architecture string           `json:"architecture" db:"architecture"`
	Metadata    map[string]string `json:"metadata,omitempty" db:"metadata"`
	RegisteredAt time.Time        `json:"registered_at" db:"registered_at"`
	LastSeen    time.Time         `json:"last_seen" db:"last_seen"`
}

// GPUInfo represents information about a GPU device.
type GPUInfo struct {
	GPUID       string            `json:"gpu_id" db:"gpu_id"`
	UUID        string            `json:"uuid" db:"uuid"`
	Device      string            `json:"device" db:"device"`
	ModelName   string            `json:"model_name" db:"model_name"`
	Hostname    string            `json:"hostname" db:"hostname"`
	ClusterID   string            `json:"cluster_id" db:"cluster_id"`
	Memory      int64             `json:"memory" db:"memory"`
	Cores       int               `json:"cores" db:"cores"`
	Metadata    map[string]string `json:"metadata,omitempty" db:"metadata"`
	RegisteredAt time.Time        `json:"registered_at" db:"registered_at"`
	LastSeen    time.Time         `json:"last_seen" db:"last_seen"`
}

// MetricDefinition represents the definition of a telemetry metric.
type MetricDefinition struct {
	Name        string `json:"name" db:"name"`
	Description string `json:"description" db:"description"`
	Unit        string `json:"unit" db:"unit"`
	Type        string `json:"type" db:"type"` // gauge, counter, histogram
	Tags        []string `json:"tags,omitempty" db:"tags"`
}
