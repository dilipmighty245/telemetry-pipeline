package models

import (
	"encoding/json"
	"fmt"
	"time"
)

// SchemaVersion represents the telemetry data schema version
type SchemaVersion string

const (
	SchemaV1      SchemaVersion = "v1"
	SchemaV1Beta1 SchemaVersion = "v1beta1"
	SchemaV1Beta2 SchemaVersion = "v1beta2"

	// Current schema version
	CurrentSchemaVersion = SchemaV1
)

// VersionedTelemetryData represents telemetry data with schema versioning
type VersionedTelemetryData struct {
	// Metadata
	SchemaVersion SchemaVersion `json:"schema_version"`
	MessageID     string        `json:"message_id"` // ULID
	TraceID       string        `json:"trace_id"`   // OpenTelemetry trace ID

	// Core data (versioned)
	Data interface{} `json:"data"`

	// Processing metadata
	ReceivedAt  time.Time `json:"received_at"`
	ProcessedAt time.Time `json:"processed_at"`
	ValidatedAt time.Time `json:"validated_at"`
}

// TelemetryDataV1 represents the current telemetry data schema
type TelemetryDataV1 struct {
	// Identity
	ID         uint      `json:"id" gorm:"primaryKey"`
	Timestamp  time.Time `json:"timestamp" gorm:"index"`
	MetricName string    `json:"metric_name" gorm:"index"`

	// Device information
	GPUID     string `json:"gpu_id" gorm:"index"`
	Device    string `json:"device"`
	UUID      string `json:"uuid" gorm:"index"` // GPU UUID
	ModelName string `json:"model_name"`
	Hostname  string `json:"hostname" gorm:"index"`

	// Kubernetes context
	Container string `json:"container"`
	Pod       string `json:"pod" gorm:"index"`
	Namespace string `json:"namespace" gorm:"index"`

	// Metric value and metadata
	Value     float64 `json:"value"`
	LabelsRaw string  `json:"labels_raw"`

	// Schema and processing metadata
	SchemaVersion string    `json:"schema_version"`
	MessageID     string    `json:"message_id" gorm:"index"`
	TraceID       string    `json:"trace_id"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`

	// Validation metadata
	IsValid       bool   `json:"is_valid"`
	ValidationMsg string `json:"validation_msg,omitempty"`
}

// TelemetryDataV1Beta2 represents an extended schema with additional metrics
type TelemetryDataV1Beta2 struct {
	TelemetryDataV1

	// Extended metrics
	GPUUtilization    *float64 `json:"gpu_utilization,omitempty"`
	MemoryUtilization *float64 `json:"memory_utilization,omitempty"`
	MemoryUsedMB      *float64 `json:"memory_used_mb,omitempty"`
	MemoryFreeMB      *float64 `json:"memory_free_mb,omitempty"`
	Temperature       *float64 `json:"temperature,omitempty"`
	PowerDraw         *float64 `json:"power_draw,omitempty"`
	SMClockMHz        *float64 `json:"sm_clock_mhz,omitempty"`
	MemoryClockMHz    *float64 `json:"memory_clock_mhz,omitempty"`

	// Custom metrics as JSON
	CustomMetrics json.RawMessage `json:"custom_metrics,omitempty"`
}

// ValidationError represents a schema validation error
type ValidationError struct {
	Field   string      `json:"field"`
	Message string      `json:"message"`
	Value   interface{} `json:"value"`
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("validation error for field '%s': %s (value: %v)", e.Field, e.Message, e.Value)
}

// SchemaValidationResult represents the result of schema validation
type SchemaValidationResult struct {
	Valid  bool              `json:"valid"`
	Errors []ValidationError `json:"errors,omitempty"`
}

// NewVersionedTelemetryData creates a new versioned telemetry data instance
func NewVersionedTelemetryData(messageID, traceID string, data interface{}) *VersionedTelemetryData {
	return &VersionedTelemetryData{
		SchemaVersion: CurrentSchemaVersion,
		MessageID:     messageID,
		TraceID:       traceID,
		Data:          data,
		ReceivedAt:    time.Now(),
	}
}

// Validate validates the telemetry data against its schema version
func (vtd *VersionedTelemetryData) Validate() *SchemaValidationResult {
	vtd.ValidatedAt = time.Now()

	switch vtd.SchemaVersion {
	case SchemaV1:
		return vtd.validateV1()
	case SchemaV1Beta2:
		return vtd.validateV1Beta2()
	default:
		return &SchemaValidationResult{
			Valid: false,
			Errors: []ValidationError{
				{
					Field:   "schema_version",
					Message: "unsupported schema version",
					Value:   vtd.SchemaVersion,
				},
			},
		}
	}
}

// validateV1 validates against V1 schema
func (vtd *VersionedTelemetryData) validateV1() *SchemaValidationResult {
	result := &SchemaValidationResult{Valid: true}

	// Convert data to V1 struct
	dataBytes, err := json.Marshal(vtd.Data)
	if err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:   "data",
			Message: "failed to serialize data",
			Value:   err.Error(),
		})
		return result
	}

	var v1Data TelemetryDataV1
	if err := json.Unmarshal(dataBytes, &v1Data); err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:   "data",
			Message: "failed to parse V1 data",
			Value:   err.Error(),
		})
		return result
	}

	// Validate required fields
	if v1Data.Timestamp.IsZero() {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:   "timestamp",
			Message: "timestamp is required",
			Value:   v1Data.Timestamp,
		})
	}

	if v1Data.MetricName == "" {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:   "metric_name",
			Message: "metric_name is required",
			Value:   v1Data.MetricName,
		})
	}

	if v1Data.GPUID == "" {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:   "gpu_id",
			Message: "gpu_id is required",
			Value:   v1Data.GPUID,
		})
	}

	if v1Data.UUID == "" {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:   "uuid",
			Message: "uuid is required",
			Value:   v1Data.UUID,
		})
	}

	if v1Data.Hostname == "" {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:   "hostname",
			Message: "hostname is required",
			Value:   v1Data.Hostname,
		})
	}

	// Validate timestamp is not too far in the future
	if v1Data.Timestamp.After(time.Now().Add(5 * time.Minute)) {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:   "timestamp",
			Message: "timestamp cannot be more than 5 minutes in the future",
			Value:   v1Data.Timestamp,
		})
	}

	// Validate timestamp is not too old (more than 30 days for demo purposes)
	if v1Data.Timestamp.Before(time.Now().Add(-30 * 24 * time.Hour)) {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:   "timestamp",
			Message: "timestamp cannot be more than 30 days old",
			Value:   v1Data.Timestamp,
		})
	}

	return result
}

// validateV1Beta2 validates against V1Beta2 schema (includes V1 validation plus extended fields)
func (vtd *VersionedTelemetryData) validateV1Beta2() *SchemaValidationResult {
	// First validate base V1 schema
	result := vtd.validateV1()
	if !result.Valid {
		return result
	}

	// Convert data to V1Beta2 struct
	dataBytes, err := json.Marshal(vtd.Data)
	if err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:   "data",
			Message: "failed to serialize data for V1Beta2",
			Value:   err.Error(),
		})
		return result
	}

	var v1Beta2Data TelemetryDataV1Beta2
	if err := json.Unmarshal(dataBytes, &v1Beta2Data); err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:   "data",
			Message: "failed to parse V1Beta2 data",
			Value:   err.Error(),
		})
		return result
	}

	// Validate extended metrics ranges
	if v1Beta2Data.GPUUtilization != nil && (*v1Beta2Data.GPUUtilization < 0 || *v1Beta2Data.GPUUtilization > 100) {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:   "gpu_utilization",
			Message: "gpu_utilization must be between 0 and 100",
			Value:   *v1Beta2Data.GPUUtilization,
		})
	}

	if v1Beta2Data.MemoryUtilization != nil && (*v1Beta2Data.MemoryUtilization < 0 || *v1Beta2Data.MemoryUtilization > 100) {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:   "memory_utilization",
			Message: "memory_utilization must be between 0 and 100",
			Value:   *v1Beta2Data.MemoryUtilization,
		})
	}

	if v1Beta2Data.Temperature != nil && (*v1Beta2Data.Temperature < -50 || *v1Beta2Data.Temperature > 120) {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:   "temperature",
			Message: "temperature must be between -50 and 120 degrees Celsius",
			Value:   *v1Beta2Data.Temperature,
		})
	}

	if v1Beta2Data.PowerDraw != nil && *v1Beta2Data.PowerDraw < 0 {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:   "power_draw",
			Message: "power_draw cannot be negative",
			Value:   *v1Beta2Data.PowerDraw,
		})
	}

	// Validate custom metrics JSON if present
	if len(v1Beta2Data.CustomMetrics) > 0 {
		var customMetrics map[string]interface{}
		if err := json.Unmarshal(v1Beta2Data.CustomMetrics, &customMetrics); err != nil {
			result.Valid = false
			result.Errors = append(result.Errors, ValidationError{
				Field:   "custom_metrics",
				Message: "custom_metrics must be valid JSON",
				Value:   string(v1Beta2Data.CustomMetrics),
			})
		}
	}

	return result
}

// ConvertToV1 converts the versioned data to V1 format
func (vtd *VersionedTelemetryData) ConvertToV1() (*TelemetryDataV1, error) {
	vtd.ProcessedAt = time.Now()

	switch vtd.SchemaVersion {
	case SchemaV1:
		dataBytes, err := json.Marshal(vtd.Data)
		if err != nil {
			return nil, fmt.Errorf("failed to serialize V1 data: %w", err)
		}

		var v1Data TelemetryDataV1
		if err := json.Unmarshal(dataBytes, &v1Data); err != nil {
			return nil, fmt.Errorf("failed to parse V1 data: %w", err)
		}

		// Set metadata
		v1Data.SchemaVersion = string(vtd.SchemaVersion)
		v1Data.MessageID = vtd.MessageID
		v1Data.TraceID = vtd.TraceID

		return &v1Data, nil

	case SchemaV1Beta2:
		dataBytes, err := json.Marshal(vtd.Data)
		if err != nil {
			return nil, fmt.Errorf("failed to serialize V1Beta2 data: %w", err)
		}

		var v1Beta2Data TelemetryDataV1Beta2
		if err := json.Unmarshal(dataBytes, &v1Beta2Data); err != nil {
			return nil, fmt.Errorf("failed to parse V1Beta2 data: %w", err)
		}

		// Convert V1Beta2 to V1 by taking the base fields
		v1Data := v1Beta2Data.TelemetryDataV1

		// If extended metrics are present, store them in LabelsRaw as JSON
		if v1Beta2Data.GPUUtilization != nil || v1Beta2Data.MemoryUtilization != nil ||
			v1Beta2Data.Temperature != nil || v1Beta2Data.PowerDraw != nil {
			extendedMetrics := map[string]interface{}{}

			if v1Beta2Data.GPUUtilization != nil {
				extendedMetrics["gpu_utilization"] = *v1Beta2Data.GPUUtilization
			}
			if v1Beta2Data.MemoryUtilization != nil {
				extendedMetrics["memory_utilization"] = *v1Beta2Data.MemoryUtilization
			}
			if v1Beta2Data.Temperature != nil {
				extendedMetrics["temperature"] = *v1Beta2Data.Temperature
			}
			if v1Beta2Data.PowerDraw != nil {
				extendedMetrics["power_draw"] = *v1Beta2Data.PowerDraw
			}

			extendedBytes, _ := json.Marshal(extendedMetrics)
			v1Data.LabelsRaw = string(extendedBytes)
		}

		// Set metadata
		v1Data.SchemaVersion = string(vtd.SchemaVersion)
		v1Data.MessageID = vtd.MessageID
		v1Data.TraceID = vtd.TraceID

		return &v1Data, nil

	default:
		return nil, fmt.Errorf("unsupported schema version: %s", vtd.SchemaVersion)
	}
}

// TableName returns the table name for V1 telemetry data
func (TelemetryDataV1) TableName() string {
	return "telemetry_data"
}

// IsValidSchemaVersion checks if a schema version is supported
func IsValidSchemaVersion(version string) bool {
	switch SchemaVersion(version) {
	case SchemaV1, SchemaV1Beta1, SchemaV1Beta2:
		return true
	default:
		return false
	}
}

// GetSupportedSchemaVersions returns all supported schema versions
func GetSupportedSchemaVersions() []SchemaVersion {
	return []SchemaVersion{SchemaV1, SchemaV1Beta1, SchemaV1Beta2}
}

// MigrateSchemaVersion attempts to migrate data from one schema version to another
func MigrateSchemaVersion(vtd *VersionedTelemetryData, targetVersion SchemaVersion) error {
	if vtd.SchemaVersion == targetVersion {
		return nil // No migration needed
	}

	// Currently, we only support forward migration to V1 for storage
	if targetVersion != SchemaV1 {
		return fmt.Errorf("migration to %s is not supported", targetVersion)
	}

	// Convert to V1 and update the versioned data
	v1Data, err := vtd.ConvertToV1()
	if err != nil {
		return fmt.Errorf("failed to migrate to V1: %w", err)
	}

	vtd.SchemaVersion = SchemaV1
	vtd.Data = v1Data
	vtd.ProcessedAt = time.Now()

	return nil
}
