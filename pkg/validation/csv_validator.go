// Package validation provides comprehensive CSV file validation for the telemetry pipeline.
package validation

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/dilipmighty245/telemetry-pipeline/pkg/errors"
	"github.com/dilipmighty245/telemetry-pipeline/pkg/logging"
)

// ValidationError represents a validation error with context
type ValidationError struct {
	Field    string `json:"field"`
	Value    string `json:"value"`
	Rule     string `json:"rule"`
	Message  string `json:"message"`
	Line     int    `json:"line"`
	Column   int    `json:"column"`
	Severity string `json:"severity"`
}

func (ve ValidationError) Error() string {
	return fmt.Sprintf("validation %s at line %d, column %d (%s): %s", 
		ve.Severity, ve.Line, ve.Column, ve.Field, ve.Message)
}

// ValidationResult contains the results of CSV validation
type ValidationResult struct {
	Valid              bool               `json:"valid"`
	TotalRows          int                `json:"total_rows"`
	ValidRows          int                `json:"valid_rows"`
	ErrorRows          int                `json:"error_rows"`
	WarningRows        int                `json:"warning_rows"`
	Errors             []ValidationError  `json:"errors"`
	Warnings           []ValidationError  `json:"warnings"`
	Summary            map[string]int     `json:"summary"`
	ProcessingTime     time.Duration      `json:"processing_time"`
	FileSize           int64              `json:"file_size"`
}

// FieldSchema defines the validation schema for a CSV field
type FieldSchema struct {
	Name         string   `json:"name"`
	Type         string   `json:"type"`
	Required     bool     `json:"required"`
	MinLength    int      `json:"min_length"`
	MaxLength    int      `json:"max_length"`
	MinValue     *float64 `json:"min_value"`
	MaxValue     *float64 `json:"max_value"`
	Pattern      string   `json:"pattern"`
	Format       string   `json:"format"`
	AllowedValues []string `json:"allowed_values"`
	Description  string   `json:"description"`
}

// CSVSchema defines the complete validation schema for a CSV file
type CSVSchema struct {
	Name            string        `json:"name"`
	Version         string        `json:"version"`
	Description     string        `json:"description"`
	Fields          []FieldSchema `json:"fields"`
	RequiredFields  []string      `json:"required_fields"`
	HeaderRequired  bool          `json:"header_required"`
	Delimiter       rune          `json:"delimiter"`
	Quote           rune          `json:"quote"`
	MaxRows         int           `json:"max_rows"`
	MinRows         int           `json:"min_rows"`
	AllowExtraFields bool         `json:"allow_extra_fields"`
}

// GetTelemetryCSVSchema returns the standard schema for telemetry CSV files
func GetTelemetryCSVSchema() *CSVSchema {
	return &CSVSchema{
		Name:        "telemetry-csv-schema",
		Version:     "1.0.0",
		Description: "Standard schema for GPU telemetry CSV files",
		Fields: []FieldSchema{
			{
				Name: "timestamp", Type: "timestamp", Required: true,
				Format: "2006-01-02T15:04:05Z", Description: "Timestamp of the telemetry reading",
			},
			{
				Name: "gpu_id", Type: "string", Required: true,
				Pattern: `^[0-9]+$`, MinLength: 1, MaxLength: 10,
				Description: "GPU ID within the host (0, 1, 2, etc.)",
			},
			{
				Name: "hostname", Type: "string", Required: true,
				Pattern: `^[a-zA-Z0-9\-\.]+$`, MinLength: 1, MaxLength: 255,
				Description: "Hostname of the machine containing the GPU",
			},
			{
				Name: "uuid", Type: "string", Required: false,
				Pattern: `^[a-fA-F0-9\-]+$`, MinLength: 8, MaxLength: 50,
				Description: "Globally unique GPU identifier",
			},
			{
				Name: "device", Type: "string", Required: false,
				Pattern: `^nvidia[0-9]+$`, Description: "Device name (nvidia0, nvidia1, etc.)",
			},
			{
				Name: "modelname", Type: "string", Required: false,
				MinLength: 1, MaxLength: 100, Description: "GPU model name",
			},
			{
				Name: "gpu_utilization", Type: "float", Required: true,
				MinValue: floatPtr(0.0), MaxValue: floatPtr(100.0),
				Description: "GPU utilization percentage (0-100)",
			},
			{
				Name: "memory_utilization", Type: "float", Required: true,
				MinValue: floatPtr(0.0), MaxValue: floatPtr(100.0),
				Description: "Memory utilization percentage (0-100)",
			},
			{
				Name: "memory_used_mb", Type: "float", Required: false,
				MinValue: floatPtr(0.0), MaxValue: floatPtr(1000000.0),
				Description: "Memory used in megabytes",
			},
			{
				Name: "memory_free_mb", Type: "float", Required: false,
				MinValue: floatPtr(0.0), MaxValue: floatPtr(1000000.0),
				Description: "Memory free in megabytes",
			},
			{
				Name: "temperature", Type: "float", Required: false,
				MinValue: floatPtr(-50.0), MaxValue: floatPtr(150.0),
				Description: "GPU temperature in Celsius",
			},
			{
				Name: "power_draw", Type: "float", Required: false,
				MinValue: floatPtr(0.0), MaxValue: floatPtr(1000.0),
				Description: "Power draw in watts",
			},
		},
		RequiredFields:   []string{"timestamp", "gpu_id", "hostname", "gpu_utilization", "memory_utilization"},
		HeaderRequired:   true,
		Delimiter:        ',',
		Quote:            '"',
		MaxRows:          1000000,
		MinRows:          1,
		AllowExtraFields: true,
	}
}

// ValidatorConfig holds configuration for CSV validation
type ValidatorConfig struct {
	MaxErrors          int  `json:"max_errors"`
	MaxWarnings        int  `json:"max_warnings"`
	StrictMode         bool `json:"strict_mode"`
	ValidateDataTypes  bool `json:"validate_data_types"`
	ValidateRanges     bool `json:"validate_ranges"`
	ValidatePatterns   bool `json:"validate_patterns"`
	ValidateBusinessRules bool `json:"validate_business_rules"`
	LogProgress        bool `json:"log_progress"`
	ProgressInterval   int  `json:"progress_interval"`
}

// DefaultValidatorConfig returns a default validator configuration
func DefaultValidatorConfig() *ValidatorConfig {
	return &ValidatorConfig{
		MaxErrors:             100,
		MaxWarnings:           1000,
		StrictMode:            false,
		ValidateDataTypes:     true,
		ValidateRanges:        true,
		ValidatePatterns:      true,
		ValidateBusinessRules: true,
		LogProgress:           true,
		ProgressInterval:      10000,
	}
}

// CSVValidator provides comprehensive CSV validation capabilities
type CSVValidator struct {
	schema           *CSVSchema
	config           *ValidatorConfig
	fieldMap         map[string]*FieldSchema
	compiledPatterns map[string]*regexp.Regexp
}

// NewCSVValidator creates a new CSV validator
func NewCSVValidator(schema *CSVSchema, config *ValidatorConfig) (*CSVValidator, error) {
	if schema == nil {
		return nil, errors.NewValidationError("schema cannot be nil", nil)
	}
	
	if config == nil {
		config = DefaultValidatorConfig()
	}

	validator := &CSVValidator{
		schema:           schema,
		config:           config,
		fieldMap:         make(map[string]*FieldSchema),
		compiledPatterns: make(map[string]*regexp.Regexp),
	}

	// Build field map and compile patterns
	for i := range schema.Fields {
		field := &schema.Fields[i]
		validator.fieldMap[field.Name] = field
		
		if field.Pattern != "" {
			pattern, err := regexp.Compile(field.Pattern)
			if err != nil {
				return nil, errors.NewValidationError(fmt.Sprintf("invalid regex pattern for field %s", field.Name), err)
			}
			validator.compiledPatterns[field.Name] = pattern
		}
	}

	return validator, nil
}

// ValidateFile validates an entire CSV file
func (cv *CSVValidator) ValidateFile(filename string) (*ValidationResult, error) {
	startTime := time.Now()
	
	fileInfo, err := os.Stat(filename)
	if err != nil {
		return nil, errors.NewValidationError(fmt.Sprintf("cannot access file %s", filename), err)
	}

	file, err := os.Open(filename)
	if err != nil {
		return nil, errors.NewValidationError(fmt.Sprintf("cannot open file %s", filename), err)
	}
	defer file.Close()

	result := &ValidationResult{
		Valid:            true,
		Errors:           make([]ValidationError, 0),
		Warnings:         make([]ValidationError, 0),
		Summary:          make(map[string]int),
		FileSize:         fileInfo.Size(),
	}

	reader := csv.NewReader(file)
	reader.Comma = cv.schema.Delimiter
	reader.FieldsPerRecord = -1

	var headers []string
	lineNumber := 0

	// Read and validate headers
	if cv.schema.HeaderRequired {
		headerRecord, err := reader.Read()
		if err != nil {
			if err == io.EOF {
				cv.addError(result, ValidationError{
					Field: "file", Rule: "header_required",
					Message: "file is empty but header is required",
					Line: 1, Severity: "error",
				})
				return result, nil
			}
			return nil, errors.NewValidationError("failed to read CSV headers", err)
		}
		
		lineNumber++
		headers = headerRecord
		cv.validateHeaders(headers, result)
	}

	// Validate data rows
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			cv.addError(result, ValidationError{
				Field: "file", Rule: "csv_format",
				Message: fmt.Sprintf("CSV parsing error: %v", err),
				Line: lineNumber + 1, Severity: "error",
			})
			result.ErrorRows++
			lineNumber++
			continue
		}

		lineNumber++
		result.TotalRows++

		if cv.config.LogProgress && result.TotalRows%cv.config.ProgressInterval == 0 {
			logging.Infof("Validated %d rows, %d errors, %d warnings", 
				result.TotalRows, len(result.Errors), len(result.Warnings))
		}

		rowValid := cv.validateRecord(record, headers, lineNumber, result)
		if rowValid {
			result.ValidRows++
		}

		if len(result.Errors) >= cv.config.MaxErrors {
			logging.Warnf("Stopping validation after %d errors", len(result.Errors))
			break
		}
	}

	cv.validateFileConstraints(result)
	result.ProcessingTime = time.Since(startTime)
	result.Valid = len(result.Errors) == 0 && (!cv.config.StrictMode || len(result.Warnings) == 0)
	
	result.Summary = map[string]int{
		"total_rows":     result.TotalRows,
		"valid_rows":     result.ValidRows,
		"error_rows":     result.ErrorRows,
		"warning_rows":   result.WarningRows,
		"total_errors":   len(result.Errors),
		"total_warnings": len(result.Warnings),
	}

	logging.Infof("CSV validation completed: %d rows, %d errors, %d warnings (%.2fs)", 
		result.TotalRows, len(result.Errors), len(result.Warnings), result.ProcessingTime.Seconds())

	return result, nil
}

// validateHeaders validates CSV headers
func (cv *CSVValidator) validateHeaders(headers []string, result *ValidationResult) {
	headerMap := make(map[string]bool)
	
	for i, header := range headers {
		header = strings.TrimSpace(header)
		if headerMap[header] {
			cv.addError(result, ValidationError{
				Field: "header", Value: header, Rule: "duplicate_header",
				Message: "duplicate header found", Line: 1, Column: i + 1, Severity: "error",
			})
		}
		headerMap[header] = true
	}

	for _, requiredField := range cv.schema.RequiredFields {
		found := false
		for _, header := range headers {
			if strings.TrimSpace(header) == requiredField {
				found = true
				break
			}
		}
		if !found {
			cv.addError(result, ValidationError{
				Field: "header", Value: requiredField, Rule: "required_field_missing",
				Message: fmt.Sprintf("required field '%s' not found", requiredField),
				Line: 1, Severity: "error",
			})
		}
	}
}

// validateRecord validates a single CSV record
func (cv *CSVValidator) validateRecord(record []string, headers []string, lineNumber int, result *ValidationResult) bool {
	rowValid := true
	
	if len(headers) > 0 && len(record) != len(headers) {
		cv.addWarning(result, ValidationError{
			Field: "record", Rule: "field_count_mismatch",
			Message: fmt.Sprintf("record has %d fields but header has %d", len(record), len(headers)),
			Line: lineNumber, Severity: "warning",
		})
	}

	for i, value := range record {
		var fieldName string
		if i < len(headers) {
			fieldName = strings.TrimSpace(headers[i])
		} else {
			fieldName = fmt.Sprintf("field_%d", i+1)
		}

		fieldSchema := cv.fieldMap[fieldName]
		if fieldSchema != nil {
			if !cv.validateField(value, fieldSchema, lineNumber, i+1, result) {
				rowValid = false
			}
		}
	}

	if !rowValid {
		result.ErrorRows++
	}

	return rowValid
}

// validateField validates a single field value
func (cv *CSVValidator) validateField(value string, schema *FieldSchema, lineNumber, columnNumber int, result *ValidationResult) bool {
	value = strings.TrimSpace(value)
	fieldValid := true

	if schema.Required && value == "" {
		cv.addError(result, ValidationError{
			Field: schema.Name, Value: value, Rule: "required",
			Message: "required field is empty", Line: lineNumber, Column: columnNumber, Severity: "error",
		})
		return false
	}

	if !schema.Required && value == "" {
		return true
	}

	if cv.config.ValidateDataTypes {
		if !cv.validateDataType(value, schema, lineNumber, columnNumber, result) {
			fieldValid = false
		}
	}

	if schema.MinLength > 0 && len(value) < schema.MinLength {
		cv.addError(result, ValidationError{
			Field: schema.Name, Value: value, Rule: "min_length",
			Message: fmt.Sprintf("length %d < minimum %d", len(value), schema.MinLength),
			Line: lineNumber, Column: columnNumber, Severity: "error",
		})
		fieldValid = false
	}

	if schema.MaxLength > 0 && len(value) > schema.MaxLength {
		cv.addError(result, ValidationError{
			Field: schema.Name, Value: value, Rule: "max_length",
			Message: fmt.Sprintf("length %d > maximum %d", len(value), schema.MaxLength),
			Line: lineNumber, Column: columnNumber, Severity: "error",
		})
		fieldValid = false
	}

	if cv.config.ValidatePatterns && schema.Pattern != "" {
		if pattern := cv.compiledPatterns[schema.Name]; pattern != nil {
			if !pattern.MatchString(value) {
				cv.addError(result, ValidationError{
					Field: schema.Name, Value: value, Rule: "pattern",
					Message: fmt.Sprintf("does not match pattern %s", schema.Pattern),
					Line: lineNumber, Column: columnNumber, Severity: "error",
				})
				fieldValid = false
			}
		}
	}

	return fieldValid
}

// validateDataType validates field data type
func (cv *CSVValidator) validateDataType(value string, schema *FieldSchema, lineNumber, columnNumber int, result *ValidationResult) bool {
	switch schema.Type {
	case "string":
		return true
		
	case "int":
		if _, err := strconv.Atoi(value); err != nil {
			cv.addError(result, ValidationError{
				Field: schema.Name, Value: value, Rule: "data_type",
				Message: "not a valid integer", Line: lineNumber, Column: columnNumber, Severity: "error",
			})
			return false
		}
		
	case "float":
		floatValue, err := strconv.ParseFloat(value, 64)
		if err != nil {
			cv.addError(result, ValidationError{
				Field: schema.Name, Value: value, Rule: "data_type",
				Message: "not a valid float", Line: lineNumber, Column: columnNumber, Severity: "error",
			})
			return false
		}
		
		if cv.config.ValidateRanges {
			if schema.MinValue != nil && floatValue < *schema.MinValue {
				cv.addError(result, ValidationError{
					Field: schema.Name, Value: value, Rule: "min_value",
					Message: fmt.Sprintf("%.2f < minimum %.2f", floatValue, *schema.MinValue),
					Line: lineNumber, Column: columnNumber, Severity: "error",
				})
				return false
			}
			
			if schema.MaxValue != nil && floatValue > *schema.MaxValue {
				cv.addError(result, ValidationError{
					Field: schema.Name, Value: value, Rule: "max_value",
					Message: fmt.Sprintf("%.2f > maximum %.2f", floatValue, *schema.MaxValue),
					Line: lineNumber, Column: columnNumber, Severity: "error",
				})
				return false
			}
		}
		
	case "timestamp":
		format := schema.Format
		if format == "" {
			format = time.RFC3339
		}
		
		if _, err := time.Parse(format, value); err != nil {
			alternativeFormats := []string{
				"2006-01-02T15:04:05Z",
				"2006-01-02 15:04:05",
				"2006-01-02T15:04:05.000Z",
			}
			
			parsed := false
			for _, altFormat := range alternativeFormats {
				if _, err := time.Parse(altFormat, value); err == nil {
					parsed = true
					break
				}
			}
			
			if !parsed {
				cv.addError(result, ValidationError{
					Field: schema.Name, Value: value, Rule: "data_type",
					Message: fmt.Sprintf("not a valid timestamp (expected: %s)", format),
					Line: lineNumber, Column: columnNumber, Severity: "error",
				})
				return false
			}
		}
	}
	
	return true
}

// validateFileConstraints validates file-level constraints
func (cv *CSVValidator) validateFileConstraints(result *ValidationResult) {
	if cv.schema.MinRows > 0 && result.TotalRows < cv.schema.MinRows {
		cv.addError(result, ValidationError{
			Field: "file", Rule: "min_rows",
			Message: fmt.Sprintf("file has %d rows but minimum is %d", result.TotalRows, cv.schema.MinRows),
			Severity: "error",
		})
	}
	
	if cv.schema.MaxRows > 0 && result.TotalRows > cv.schema.MaxRows {
		cv.addError(result, ValidationError{
			Field: "file", Rule: "max_rows",
			Message: fmt.Sprintf("file has %d rows but maximum is %d", result.TotalRows, cv.schema.MaxRows),
			Severity: "error",
		})
	}
}

// addError adds an error to the validation result
func (cv *CSVValidator) addError(result *ValidationResult, err ValidationError) {
	result.Errors = append(result.Errors, err)
}

// addWarning adds a warning to the validation result
func (cv *CSVValidator) addWarning(result *ValidationResult, warning ValidationError) {
	result.Warnings = append(result.Warnings, warning)
	result.WarningRows++
}

// floatPtr returns a pointer to a float64 value
func floatPtr(f float64) *float64 {
	return &f
}
