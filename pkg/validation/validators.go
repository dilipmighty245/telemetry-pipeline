// Package validation provides input validation utilities for the telemetry pipeline.
package validation

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// Common validation patterns
var (
	// GPUIDPattern matches valid GPU IDs (alphanumeric, hyphens, underscores)
	GPUIDPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
	
	// HostnamePattern matches valid hostnames
	HostnamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9.-]*[a-zA-Z0-9]$`)
	
	// ClusterIDPattern matches valid cluster IDs
	ClusterIDPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
)

// ValidationError represents a validation error
type ValidationError struct {
	Field   string `json:"field"`
	Value   string `json:"value"`
	Message string `json:"message"`
}

// Error implements the error interface
func (e ValidationError) Error() string {
	return fmt.Sprintf("validation error on field '%s': %s", e.Field, e.Message)
}

// Validator provides validation methods
type Validator struct{}

// NewValidator creates a new validator instance
func NewValidator() *Validator {
	return &Validator{}
}

// ValidateGPUID validates a GPU ID
func (v *Validator) ValidateGPUID(gpuID string) error {
	if gpuID == "" {
		return ValidationError{
			Field:   "gpu_id",
			Value:   gpuID,
			Message: "GPU ID is required",
		}
	}
	
	if len(gpuID) > 100 {
		return ValidationError{
			Field:   "gpu_id",
			Value:   gpuID,
			Message: "GPU ID must be less than 100 characters",
		}
	}
	
	if !GPUIDPattern.MatchString(gpuID) {
		return ValidationError{
			Field:   "gpu_id",
			Value:   gpuID,
			Message: "GPU ID contains invalid characters (only alphanumeric, hyphens, and underscores allowed)",
		}
	}
	
	return nil
}

// ValidateHostname validates a hostname
func (v *Validator) ValidateHostname(hostname string) error {
	if hostname == "" {
		return ValidationError{
			Field:   "hostname",
			Value:   hostname,
			Message: "hostname is required",
		}
	}
	
	if len(hostname) > 253 {
		return ValidationError{
			Field:   "hostname",
			Value:   hostname,
			Message: "hostname must be less than 253 characters",
		}
	}
	
	if !HostnamePattern.MatchString(hostname) {
		return ValidationError{
			Field:   "hostname",
			Value:   hostname,
			Message: "invalid hostname format",
		}
	}
	
	return nil
}

// ValidateClusterID validates a cluster ID
func (v *Validator) ValidateClusterID(clusterID string) error {
	if clusterID == "" {
		return ValidationError{
			Field:   "cluster_id",
			Value:   clusterID,
			Message: "cluster ID is required",
		}
	}
	
	if len(clusterID) > 50 {
		return ValidationError{
			Field:   "cluster_id",
			Value:   clusterID,
			Message: "cluster ID must be less than 50 characters",
		}
	}
	
	if !ClusterIDPattern.MatchString(clusterID) {
		return ValidationError{
			Field:   "cluster_id",
			Value:   clusterID,
			Message: "cluster ID contains invalid characters",
		}
	}
	
	return nil
}

// ValidateTimeRange validates start and end time parameters
func (v *Validator) ValidateTimeRange(startTimeStr, endTimeStr string) (*time.Time, *time.Time, error) {
	var startTime, endTime *time.Time
	
	if startTimeStr != "" {
		t, err := time.Parse(time.RFC3339, startTimeStr)
		if err != nil {
			return nil, nil, ValidationError{
				Field:   "start_time",
				Value:   startTimeStr,
				Message: "invalid start_time format (must be RFC3339, e.g., 2024-01-01T00:00:00Z)",
			}
		}
		
		// Check if start time is too far in the past (more than 2 years for testing compatibility)
		if t.Before(time.Now().AddDate(-2, 0, 0)) {
			return nil, nil, ValidationError{
				Field:   "start_time",
				Value:   startTimeStr,
				Message: "start_time cannot be more than 2 years in the past",
			}
		}
		
		startTime = &t
	}
	
	if endTimeStr != "" {
		t, err := time.Parse(time.RFC3339, endTimeStr)
		if err != nil {
			return nil, nil, ValidationError{
				Field:   "end_time",
				Value:   endTimeStr,
				Message: "invalid end_time format (must be RFC3339, e.g., 2024-01-01T00:00:00Z)",
			}
		}
		
		// Check if end time is in the future (with 1 hour buffer)
		if t.After(time.Now().Add(time.Hour)) {
			return nil, nil, ValidationError{
				Field:   "end_time",
				Value:   endTimeStr,
				Message: "end_time cannot be more than 1 hour in the future",
			}
		}
		
		endTime = &t
	}
	
	// Validate time range
	if startTime != nil && endTime != nil {
		if startTime.After(*endTime) {
			return nil, nil, ValidationError{
				Field:   "time_range",
				Value:   fmt.Sprintf("%s to %s", startTimeStr, endTimeStr),
				Message: "start_time must be before end_time",
			}
		}
		
		// Check if time range is too large (more than 30 days)
		if endTime.Sub(*startTime) > 30*24*time.Hour {
			return nil, nil, ValidationError{
				Field:   "time_range",
				Value:   fmt.Sprintf("%s to %s", startTimeStr, endTimeStr),
				Message: "time range cannot exceed 30 days",
			}
		}
	}
	
	return startTime, endTime, nil
}

// ValidateLimit validates the limit parameter
func (v *Validator) ValidateLimit(limitStr string, maxLimit int) (int, error) {
	if limitStr == "" {
		return maxLimit, nil // Default limit
	}
	
	// Parse limit
	var limit int
	if _, err := fmt.Sscanf(limitStr, "%d", &limit); err != nil {
		return 0, ValidationError{
			Field:   "limit",
			Value:   limitStr,
			Message: "limit must be a valid integer",
		}
	}
	
	if limit <= 0 {
		return 0, ValidationError{
			Field:   "limit",
			Value:   limitStr,
			Message: "limit must be greater than 0",
		}
	}
	
	if limit > maxLimit {
		return 0, ValidationError{
			Field:   "limit",
			Value:   limitStr,
			Message: fmt.Sprintf("limit cannot exceed %d", maxLimit),
		}
	}
	
	return limit, nil
}

// ValidateOrigin validates WebSocket origin for CORS
func (v *Validator) ValidateOrigin(origin, host string, allowedOrigins []string) bool {
	if origin == "" {
		return false
	}
	
	// Parse origin URL
	originURL, err := url.Parse(origin)
	if err != nil {
		return false
	}
	
	// Allow localhost and 127.0.0.1 for development
	if strings.Contains(originURL.Host, "localhost") || strings.Contains(originURL.Host, "127.0.0.1") {
		return true
	}
	
	// Check against allowed origins
	for _, allowed := range allowedOrigins {
		if originURL.Host == allowed {
			return true
		}
	}
	
	// Allow same-origin requests
	if host != "" && originURL.Host == host {
		return true
	}
	
	return false
}

// SanitizeString removes potentially harmful characters from strings
func (v *Validator) SanitizeString(input string) string {
	// Remove null bytes and control characters
	cleaned := strings.Map(func(r rune) rune {
		if r == 0 || (r < 32 && r != '\t' && r != '\n' && r != '\r') {
			return -1
		}
		return r
	}, input)
	
	// Trim whitespace
	return strings.TrimSpace(cleaned)
}