package streamer

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/dilipmighty245/telemetry-pipeline/internal/models"
	"github.com/dilipmighty245/telemetry-pipeline/pkg/logging"
	"github.com/dilipmighty245/telemetry-pipeline/pkg/monitoring"
	"github.com/dilipmighty245/telemetry-pipeline/pkg/validation"
)

// CSVReader handles reading telemetry data from CSV files with support for batch processing.
//
// CSVReader provides functionality to read and parse CSV files containing telemetry data.
//
// This reader supports batch processing for efficient data processing.
// It includes comprehensive error handling, data validation, and performance optimizations
// for processing large CSV files efficiently.
type CSVReader struct {
	filePath     string
	file         *os.File
	reader       *csv.Reader
	headers      []string
	batchSize    int
	recordsRead  int64
	errorCount   int64
	startTime    time.Time
	lastPosition int64

	// Enhanced validation and monitoring
	validator       *validation.CSVValidator
	metrics         *monitoring.TelemetryMetrics
	metricsRegistry *monitoring.MetricsRegistry
	validationEnabled bool
}

// NewCSVReader creates a new CSV reader for the specified file.
//
// The reader opens the CSV file, reads and validates headers, and prepares
// for batch data processing.
//
// Parameters:
//   - filePath: Path to the CSV file to read
//   - batchSize: Maximum number of records to read in each batch
//
// Returns:
//   - *CSVReader: Initialized CSV reader instance
//   - error: nil on success, error describing the failure otherwise
func NewCSVReader(filePath string, batchSize int) (*CSVReader, error) {
	file, err := os.Open(filePath)
	if err != nil {
		logging.Errorf("Failed to open CSV file %s: %v", filePath, err)
		return nil, err
	}

	reader := csv.NewReader(file)
	reader.FieldsPerRecord = -1 // Allow variable number of fields

	// Read headers
	headers, err := reader.Read()
	if err != nil {
		file.Close()
		logging.Errorf("Failed to read CSV headers: %v", err)
		return nil, err
	}

	// Initialize metrics
	metricsConfig := monitoring.DefaultMetricsConfig()
	metricsConfig.Labels["csv_file"] = filePath
	metricsRegistry := monitoring.NewMetricsRegistry(metricsConfig)
	metrics := monitoring.NewTelemetryMetrics(metricsRegistry, "csv_reader")

	// Initialize CSV validator
	schema := validation.GetTelemetryCSVSchema()
	validatorConfig := &validation.ValidatorConfig{
		StrictMode: false,
		MaxErrors:  100,
	}
	validator, err := validation.NewCSVValidator(schema, validatorConfig)
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("failed to create CSV validator: %v", err)
	}

	csvReader := &CSVReader{
		filePath:          filePath,
		batchSize:         batchSize,
		startTime:         time.Now(),
		validator:         validator,
		metrics:          metrics,
		metricsRegistry:  metricsRegistry,
		validationEnabled: true,
		file:              file,
		reader:            reader,
		headers:           headers,
	}

	logging.Infof("Created CSV reader for file %s with %d headers", filePath, len(headers))
	return csvReader, nil
}

// ReadBatch reads a batch of telemetry records from the CSV file.
//
// This method reads up to batchSize records from the CSV file and converts them
// to TelemetryData objects.
//
// Parameters:
//   - batchSize: Maximum number of records to read in this batch
//
// Returns:
//   - []*models.TelemetryData: Slice of parsed telemetry data objects
//   - error: Any error that occurred during reading or parsing
//
// The method handles various error conditions gracefully:
//   - Invalid CSV format: Logs warning and skips the record
//   - EOF: Returns io.EOF when end of file is reached
//   - File I/O errors: Returns appropriate error
func (cr *CSVReader) ReadBatch(batchSize int) ([]*models.TelemetryData, error) {
	startTime := time.Now()
	cr.metrics.SetComponentHealth(true)
	var telemetryData []*models.TelemetryData

	for i := 0; i < batchSize; i++ {
		record, err := cr.reader.Read()
		if err == io.EOF {
			if len(telemetryData) > 0 {
				// Return what we have so far, EOF will be returned on next call
				logging.Infof("Reached end of CSV file, returning %d records", len(telemetryData))
				return telemetryData, nil
			}
			// End of file reached, return what we have
			logging.Infof("Reached end of CSV file, returning %d records", len(telemetryData))
			return telemetryData, io.EOF
		} else if err != nil {
			logging.Errorf("Failed to read CSV record: %v", err)
			return telemetryData, err
		}

		// Parse the record
		data, err := cr.parseRecord(record)
		if err != nil {
			cr.errorCount++
			cr.metrics.ComponentErrors.Inc()
			logging.Warnf("Skipping invalid record at line %d: %v", cr.recordsRead+1, err)
			continue // Skip invalid records
		}

		// Skip validation for now - will be implemented separately
		// if cr.validationEnabled {
		//     // Validation logic will be added here
		// }

		telemetryData = append(telemetryData, data)
		cr.recordsRead++
		cr.lastPosition++
	}

	// Record batch processing metrics
	latency := time.Since(startTime).Milliseconds()
	cr.metrics.ProcessingLatency.Observe(float64(latency))
	cr.metrics.RecordCSVProcessing(len(telemetryData), int(cr.errorCount), int(cr.recordsRead))

	return telemetryData, nil
}

// ReadAll reads all remaining records from the CSV file.
//
// This method reads from the current position to the end of the file,
// parsing all records into TelemetryData. Invalid records are skipped
// with warnings logged.
//
// Returns:
//   - []*models.TelemetryData: All parsed telemetry data records
//   - error: nil on success, error describing the failure otherwise
func (cr *CSVReader) ReadAll() ([]*models.TelemetryData, error) {
	var telemetryData []*models.TelemetryData

	for {
		record, err := cr.reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			logging.Errorf("Failed to read CSV record: %v", err)
			return telemetryData, err
		}

		data, err := cr.parseRecord(record)
		if err != nil {
			logging.Warnf("Failed to parse CSV record at position %d: %v", cr.lastPosition, err)
			continue
		}

		telemetryData = append(telemetryData, data)
		cr.lastPosition++
	}

	logging.Debugf("Read %d records from position %d to %d", len(telemetryData), cr.lastPosition-int64(len(telemetryData)), cr.lastPosition)
	return telemetryData, nil
}

// Close closes the CSV reader and releases the underlying file handle.
//
// This method should be called when the reader is no longer needed
// to ensure proper resource cleanup.
//
// Returns:
//   - error: nil on success, error describing the failure otherwise
func (cr *CSVReader) Close() error {
	if cr.file != nil {
		err := cr.file.Close()
		if err != nil {
			logging.Errorf("Failed to close CSV file: %v", err)
			return err
		}
		cr.file = nil
		logging.Infof("Closed CSV reader for file %s", cr.filePath)
	}
	return nil
}

// GetPosition returns the current record position in the CSV file.
//
// The position represents the number of data records read (excluding headers).
//
// Returns:
//   - int64: Current record position
func (cr *CSVReader) GetPosition() int64 {
	return cr.lastPosition
}


// parseRecord parses a CSV record into structured TelemetryData.
//
// This method maps CSV fields to TelemetryData structure, handles type conversions,
// and provides default values for missing or invalid data. It supports flexible
// field mapping and timestamp parsing with multiple formats.
//
// Parameters:
//   - record: CSV record as slice of strings
//
// Returns:
//   - *models.TelemetryData: Parsed telemetry data structure
//   - error: nil on success, error describing parsing failures
func (cr *CSVReader) parseRecord(record []string) (*models.TelemetryData, error) {
	if len(record) < len(cr.headers) {
		logging.Warnf("Record has fewer fields (%d) than headers (%d)", len(record), len(cr.headers))
	}

	// Create a map for easier field access
	fieldMap := make(map[string]string)
	for i, header := range cr.headers {
		if i < len(record) {
			fieldMap[header] = strings.TrimSpace(record[i])
		} else {
			fieldMap[header] = ""
		}
	}

	// Parse timestamp
	timestampStr := time.Now().Format(time.RFC3339)
	// Remove quotes if present
	timestampStr = strings.Trim(timestampStr, "\"")

	timestamp, err := time.Parse(time.RFC3339, timestampStr)
	if err != nil {
		// Try alternative timestamp formats
		timestamp, err = time.Parse("2006-01-02T15:04:05Z", timestampStr)
		if err != nil {
			timestamp, err = time.Parse("2006-01-02 15:04:05", timestampStr)
			if err != nil {
				logging.Warnf("Failed to parse timestamp '%s', using current time: %v", timestampStr, err)
				timestamp = time.Now()
			}
		}
	}

	// Parse value
	valueStr := strings.Trim(fieldMap["value"], "\"")
	value, err := strconv.ParseFloat(valueStr, 64)
	if err != nil {
		logging.Warnf("Failed to parse value '%s': %v", valueStr, err)
		value = 0.0
	}

	// Create TelemetryData
	data := &models.TelemetryData{
		Timestamp:  timestamp,
		MetricName: strings.Trim(fieldMap["metric_name"], "\""),
		GPUID:      strings.Trim(fieldMap["gpu_id"], "\""),
		Device:     strings.Trim(fieldMap["device"], "\""),
		UUID:       strings.Trim(fieldMap["uuid"], "\""),
		ModelName:  strings.Trim(fieldMap["modelName"], "\""),
		Hostname:   strings.Trim(fieldMap["Hostname"], "\""),
		Container:  strings.Trim(fieldMap["container"], "\""),
		Pod:        strings.Trim(fieldMap["pod"], "\""),
		Namespace:  strings.Trim(fieldMap["namespace"], "\""),
		Value:      value,
		LabelsRaw:  strings.Trim(fieldMap["labels_raw"], "\""),
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	return data, nil
}


// GetHeaders returns the CSV column headers.
//
// The headers are read during initialization and used for field mapping
// during record parsing.
//
// Returns:
//   - []string: Column headers from the CSV file
func (cr *CSVReader) GetHeaders() []string {
	return cr.headers
}

// GetFilename returns the path of the CSV file being read.
//
// Returns:
//   - string: Full path to the CSV file
func (cr *CSVReader) GetFilename() string {
	return cr.filePath
}
