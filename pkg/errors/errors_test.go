package errors

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTelemetryError_Error(t *testing.T) {
	// Test error without cause
	err := &TelemetryError{
		Type:    ValidationError,
		Message: "invalid input",
	}
	expected := "VALIDATION_ERROR: invalid input"
	assert.Equal(t, expected, err.Error())

	// Test error with cause
	cause := errors.New("underlying error")
	err = &TelemetryError{
		Type:    ProcessingError,
		Message: "failed to process",
		Cause:   cause,
	}
	expected = "PROCESSING_ERROR: failed to process (caused by: underlying error)"
	assert.Equal(t, expected, err.Error())
}

func TestTelemetryError_Unwrap(t *testing.T) {
	cause := errors.New("underlying error")
	err := &TelemetryError{
		Type:    InternalError,
		Message: "internal failure",
		Cause:   cause,
	}
	
	assert.Equal(t, cause, err.Unwrap())
	
	// Test without cause
	err = &TelemetryError{
		Type:    ValidationError,
		Message: "validation failed",
	}
	assert.Nil(t, err.Unwrap())
}

func TestNewValidationError(t *testing.T) {
	cause := errors.New("invalid format")
	err := NewValidationError("field validation failed", cause)
	
	require.NotNil(t, err)
	assert.Equal(t, ValidationError, err.Type)
	assert.Equal(t, "field validation failed", err.Message)
	assert.Equal(t, cause, err.Cause)
	assert.NotNil(t, err.Context)
	assert.Len(t, err.Context, 0)
}

func TestNewConnectionError(t *testing.T) {
	cause := errors.New("connection refused")
	err := NewConnectionError("failed to connect to database", cause)
	
	require.NotNil(t, err)
	assert.Equal(t, ConnectionError, err.Type)
	assert.Equal(t, "failed to connect to database", err.Message)
	assert.Equal(t, cause, err.Cause)
	assert.NotNil(t, err.Context)
}

func TestNewProcessingError(t *testing.T) {
	cause := errors.New("parse error")
	err := NewProcessingError("failed to parse CSV", cause)
	
	require.NotNil(t, err)
	assert.Equal(t, ProcessingError, err.Type)
	assert.Equal(t, "failed to parse CSV", err.Message)
	assert.Equal(t, cause, err.Cause)
	assert.NotNil(t, err.Context)
}

func TestNewConfigurationError(t *testing.T) {
	cause := errors.New("missing config file")
	err := NewConfigurationError("invalid configuration", cause)
	
	require.NotNil(t, err)
	assert.Equal(t, ConfigurationError, err.Type)
	assert.Equal(t, "invalid configuration", err.Message)
	assert.Equal(t, cause, err.Cause)
	assert.NotNil(t, err.Context)
}

func TestNewInternalError(t *testing.T) {
	cause := errors.New("unexpected panic")
	err := NewInternalError("internal system failure", cause)
	
	require.NotNil(t, err)
	assert.Equal(t, InternalError, err.Type)
	assert.Equal(t, "internal system failure", err.Message)
	assert.Equal(t, cause, err.Cause)
	assert.NotNil(t, err.Context)
}

func TestTelemetryError_WithContext(t *testing.T) {
	err := NewValidationError("validation failed", nil)
	
	// Add context
	result := err.WithContext("field", "username")
	assert.Same(t, err, result) // Should return same instance
	assert.Equal(t, "username", err.Context["field"])
	
	// Add more context
	err.WithContext("value", "invalid@email").WithContext("rule", "email_format")
	assert.Equal(t, "invalid@email", err.Context["value"])
	assert.Equal(t, "email_format", err.Context["rule"])
	assert.Len(t, err.Context, 3)
}

func TestErrorTypes(t *testing.T) {
	// Test all error type constants
	assert.Equal(t, ErrorType("VALIDATION_ERROR"), ValidationError)
	assert.Equal(t, ErrorType("CONNECTION_ERROR"), ConnectionError)
	assert.Equal(t, ErrorType("PROCESSING_ERROR"), ProcessingError)
	assert.Equal(t, ErrorType("CONFIGURATION_ERROR"), ConfigurationError)
	assert.Equal(t, ErrorType("INTERNAL_ERROR"), InternalError)
}

func TestErrorCreationWithoutCause(t *testing.T) {
	// Test creating errors without cause
	validationErr := NewValidationError("validation failed", nil)
	assert.Nil(t, validationErr.Cause)
	assert.Equal(t, "VALIDATION_ERROR: validation failed", validationErr.Error())
	
	connectionErr := NewConnectionError("connection failed", nil)
	assert.Nil(t, connectionErr.Cause)
	assert.Equal(t, "CONNECTION_ERROR: connection failed", connectionErr.Error())
	
	processingErr := NewProcessingError("processing failed", nil)
	assert.Nil(t, processingErr.Cause)
	assert.Equal(t, "PROCESSING_ERROR: processing failed", processingErr.Error())
	
	configErr := NewConfigurationError("config failed", nil)
	assert.Nil(t, configErr.Cause)
	assert.Equal(t, "CONFIGURATION_ERROR: config failed", configErr.Error())
	
	internalErr := NewInternalError("internal failed", nil)
	assert.Nil(t, internalErr.Cause)
	assert.Equal(t, "INTERNAL_ERROR: internal failed", internalErr.Error())
}
