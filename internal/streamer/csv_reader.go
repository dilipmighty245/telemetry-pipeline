package streamer

import (
	"encoding/csv"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/dilipmighty245/telemetry-pipeline/pkg/logging"
	"github.com/dilipmighty245/telemetry-pipeline/pkg/models"
)

// CSVReader handles reading telemetry data from CSV files
type CSVReader struct {
	filename string
	file     *os.File
	reader   *csv.Reader
	headers  []string
	position int64
	loopMode bool
}

// NewCSVReader creates a new CSV reader
func NewCSVReader(filename string, loopMode bool) (*CSVReader, error) {
	file, err := os.Open(filename)
	if err != nil {
		logging.Errorf("Failed to open CSV file %s: %v", filename, err)
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

	csvReader := &CSVReader{
		filename: filename,
		file:     file,
		reader:   reader,
		headers:  headers,
		position: 0,
		loopMode: loopMode,
	}

	logging.Infof("Created CSV reader for file %s with %d headers", filename, len(headers))
	return csvReader, nil
}

// ReadBatch reads a batch of records from the CSV file
func (cr *CSVReader) ReadBatch(batchSize int) ([]*models.TelemetryData, error) {
	var telemetryData []*models.TelemetryData

	for i := 0; i < batchSize; i++ {
		record, err := cr.reader.Read()
		if err == io.EOF {
			if cr.loopMode {
				// Reset to beginning of file for continuous streaming
				err = cr.resetToBeginning()
				if err != nil {
					logging.Errorf("Failed to reset CSV file: %v", err)
					return telemetryData, err
				}
				// Try reading again after reset
				record, err = cr.reader.Read()
				if err != nil {
					logging.Errorf("Failed to read after reset: %v", err)
					return telemetryData, err
				}
			} else {
				// End of file reached, return what we have
				logging.Infof("Reached end of CSV file, returning %d records", len(telemetryData))
				return telemetryData, io.EOF
			}
		} else if err != nil {
			logging.Errorf("Failed to read CSV record: %v", err)
			return telemetryData, err
		}

		// Parse the record
		data, err := cr.parseRecord(record)
		if err != nil {
			logging.Warnf("Failed to parse CSV record at position %d: %v", cr.position, err)
			continue // Skip invalid records
		}

		telemetryData = append(telemetryData, data)
		cr.position++
	}

	logging.Debugf("Read batch of %d telemetry records", len(telemetryData))
	return telemetryData, nil
}

// ReadAll reads all remaining records from the CSV file
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
			logging.Warnf("Failed to parse CSV record at position %d: %v", cr.position, err)
			continue
		}

		telemetryData = append(telemetryData, data)
		cr.position++
	}

	logging.Infof("Read all %d telemetry records from CSV", len(telemetryData))
	return telemetryData, nil
}

// Close closes the CSV reader and file
func (cr *CSVReader) Close() error {
	if cr.file != nil {
		err := cr.file.Close()
		if err != nil {
			logging.Errorf("Failed to close CSV file: %v", err)
			return err
		}
		cr.file = nil
		logging.Infof("Closed CSV reader for file %s", cr.filename)
	}
	return nil
}

// GetPosition returns the current position in the CSV file
func (cr *CSVReader) GetPosition() int64 {
	return cr.position
}

// IsLoopMode returns whether the reader is in loop mode
func (cr *CSVReader) IsLoopMode() bool {
	return cr.loopMode
}

// parseRecord parses a CSV record into TelemetryData
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
	timestampStr := fieldMap["timestamp"]
	if timestampStr == "" {
		// Use current time if timestamp is missing
		timestampStr = time.Now().Format(time.RFC3339)
	}

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

// resetToBeginning resets the CSV reader to the beginning of the file
func (cr *CSVReader) resetToBeginning() error {
	// Close current file
	if cr.file != nil {
		cr.file.Close()
	}

	// Reopen file
	file, err := os.Open(cr.filename)
	if err != nil {
		return err
	}

	// Create new reader
	reader := csv.NewReader(file)
	reader.FieldsPerRecord = -1

	// Skip headers
	_, err = reader.Read()
	if err != nil {
		file.Close()
		return err
	}

	cr.file = file
	cr.reader = reader
	cr.position = 0

	logging.Debugf("Reset CSV reader to beginning of file %s", cr.filename)
	return nil
}

// GetHeaders returns the CSV headers
func (cr *CSVReader) GetHeaders() []string {
	return cr.headers
}

// GetFilename returns the CSV filename
func (cr *CSVReader) GetFilename() string {
	return cr.filename
}
