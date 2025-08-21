package models

import (
	"time"

	"gorm.io/gorm"
)

// TelemetryData represents the telemetry data model for database storage
type TelemetryData struct {
	ID         uint      `gorm:"primarykey" json:"id"`
	Timestamp  time.Time `gorm:"index;not null" json:"timestamp"`
	MetricName string    `gorm:"index;size:255;not null" json:"metric_name"`
	GPUID      string    `gorm:"index;size:255;not null" json:"gpu_id"`
	Device     string    `gorm:"size:255" json:"device"`
	UUID       string    `gorm:"index;size:255;not null" json:"uuid"`
	ModelName  string    `gorm:"size:255" json:"model_name"`
	Hostname   string    `gorm:"index;size:255" json:"hostname"`
	Container  string    `gorm:"size:255" json:"container"`
	Pod        string    `gorm:"size:255" json:"pod"`
	Namespace  string    `gorm:"size:255" json:"namespace"`
	Value      float64   `gorm:"not null" json:"value"`
	LabelsRaw  string    `gorm:"type:text" json:"labels_raw"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// GPU represents GPU information for API responses
type GPU struct {
	GPUID     string `json:"gpu_id"`
	UUID      string `json:"uuid"`
	ModelName string `json:"model_name"`
	Hostname  string `json:"hostname"`
	Device    string `json:"device"`
}

// TelemetryQuery represents query parameters for telemetry data
type TelemetryQuery struct {
	GPUID     string    `json:"gpu_id" form:"gpu_id"`
	StartTime time.Time `json:"start_time" form:"start_time"`
	EndTime   time.Time `json:"end_time" form:"end_time"`
	Limit     int       `json:"limit" form:"limit"`
	Offset    int       `json:"offset" form:"offset"`
}

// TelemetryQueryResponse represents the response for telemetry queries
type TelemetryQueryResponse struct {
	DataPoints []TelemetryData `json:"data_points"`
	TotalCount int64           `json:"total_count"`
	HasMore    bool            `json:"has_more"`
}

// CSVRecord represents a single CSV record structure
type CSVRecord struct {
	Timestamp  string `csv:"timestamp"`
	MetricName string `csv:"metric_name"`
	GPUID      string `csv:"gpu_id"`
	Device     string `csv:"device"`
	UUID       string `csv:"uuid"`
	ModelName  string `csv:"modelName"`
	Hostname   string `csv:"Hostname"`
	Container  string `csv:"container"`
	Pod        string `csv:"pod"`
	Namespace  string `csv:"namespace"`
	Value      string `csv:"value"`
	LabelsRaw  string `csv:"labels_raw"`
}

// TableName returns the table name for TelemetryData
func (TelemetryData) TableName() string {
	return "telemetry_data"
}

// BeforeCreate will set a UUID rather than numeric ID.
func (t *TelemetryData) BeforeCreate(tx *gorm.DB) error {
	if t.CreatedAt.IsZero() {
		t.CreatedAt = time.Now()
	}
	if t.UpdatedAt.IsZero() {
		t.UpdatedAt = time.Now()
	}
	return nil
}

// BeforeUpdate will update the UpdatedAt timestamp
func (t *TelemetryData) BeforeUpdate(tx *gorm.DB) error {
	t.UpdatedAt = time.Now()
	return nil
}
