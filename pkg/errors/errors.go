// Package errors provides standardized error handling patterns for the telemetry pipeline.
package errors

import (
	"fmt"
)

// ErrorType represents different categories of errors in the system.
type ErrorType string

const (
	// ValidationError represents input validation failures
	ValidationError ErrorType = "VALIDATION_ERROR"
	// ConnectionError represents network/connection failures
	ConnectionError ErrorType = "CONNECTION_ERROR"
	// ProcessingError represents data processing failures
	ProcessingError ErrorType = "PROCESSING_ERROR"
	// ConfigurationError represents configuration-related failures
	ConfigurationError ErrorType = "CONFIGURATION_ERROR"
	// InternalError represents unexpected internal failures
	InternalError ErrorType = "INTERNAL_ERROR"
)

// TelemetryError represents a standardized error with type and context.
type TelemetryError struct {
	Type    ErrorType
	Message string
	Cause   error
	Context map[string]interface{}
}

// Error implements the error interface.
func (e *TelemetryError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s (caused by: %v)", e.Type, e.Message, e.Cause)
	}
	return fmt.Sprintf("%s: %s", e.Type, e.Message)
}

// Unwrap returns the underlying cause error.
func (e *TelemetryError) Unwrap() error {
	return e.Cause
}

// NewValidationError creates a new validation error.
func NewValidationError(message string, cause error) *TelemetryError {
	return &TelemetryError{
		Type:    ValidationError,
		Message: message,
		Cause:   cause,
		Context: make(map[string]interface{}),
	}
}

// NewConnectionError creates a new connection error.
func NewConnectionError(message string, cause error) *TelemetryError {
	return &TelemetryError{
		Type:    ConnectionError,
		Message: message,
		Cause:   cause,
		Context: make(map[string]interface{}),
	}
}

// NewProcessingError creates a new processing error.
func NewProcessingError(message string, cause error) *TelemetryError {
	return &TelemetryError{
		Type:    ProcessingError,
		Message: message,
		Cause:   cause,
		Context: make(map[string]interface{}),
	}
}

// NewConfigurationError creates a new configuration error.
func NewConfigurationError(message string, cause error) *TelemetryError {
	return &TelemetryError{
		Type:    ConfigurationError,
		Message: message,
		Cause:   cause,
		Context: make(map[string]interface{}),
	}
}

// NewInternalError creates a new internal error.
func NewInternalError(message string, cause error) *TelemetryError {
	return &TelemetryError{
		Type:    InternalError,
		Message: message,
		Cause:   cause,
		Context: make(map[string]interface{}),
	}
}

// WithContext adds context information to the error.
func (e *TelemetryError) WithContext(key string, value interface{}) *TelemetryError {
	e.Context[key] = value
	return e
}
