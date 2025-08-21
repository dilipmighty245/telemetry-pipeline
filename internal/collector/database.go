package collector

import (
	"strconv"
	"time"

	"github.com/cf/telemetry-pipeline/pkg/logging"
	"github.com/cf/telemetry-pipeline/pkg/models"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// DatabaseConfig represents database configuration
type DatabaseConfig struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	User     string `json:"user"`
	Password string `json:"password"`
	DBName   string `json:"dbname"`
	SSLMode  string `json:"sslmode"`
}

// DatabaseService handles database operations for telemetry data
type DatabaseService struct {
	db     *gorm.DB
	config *DatabaseConfig
}

// NewDatabaseService creates a new database service
func NewDatabaseService(config *DatabaseConfig) (*DatabaseService, error) {
	// Build connection string
	dsn := buildConnectionString(config)

	// Configure GORM logger
	gormLogger := logger.New(
		&gormLogWriter{},
		logger.Config{
			SlowThreshold:             time.Second,
			LogLevel:                  logger.Warn,
			IgnoreRecordNotFoundError: true,
			Colorful:                  false,
		},
	)

	// Connect to database
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: gormLogger,
	})
	if err != nil {
		logging.Errorf("Failed to connect to database: %v", err)
		return nil, err
	}

	// Configure connection pool
	sqlDB, err := db.DB()
	if err != nil {
		logging.Errorf("Failed to get database instance: %v", err)
		return nil, err
	}

	sqlDB.SetMaxOpenConns(25)
	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetConnMaxLifetime(5 * time.Minute)

	service := &DatabaseService{
		db:     db,
		config: config,
	}

	// Auto-migrate the schema
	err = service.migrate()
	if err != nil {
		logging.Errorf("Failed to migrate database schema: %v", err)
		return nil, err
	}

	logging.Infof("Connected to database %s@%s:%d/%s", config.User, config.Host, config.Port, config.DBName)
	return service, nil
}

// migrate performs database schema migration
func (ds *DatabaseService) migrate() error {
	err := ds.db.AutoMigrate(&models.TelemetryData{})
	if err != nil {
		return err
	}

	// Create indexes for better query performance
	err = ds.db.Exec("CREATE INDEX IF NOT EXISTS idx_telemetry_timestamp ON telemetry_data (timestamp)").Error
	if err != nil {
		logging.Warnf("Failed to create timestamp index: %v", err)
	}

	err = ds.db.Exec("CREATE INDEX IF NOT EXISTS idx_telemetry_gpu_timestamp ON telemetry_data (gpu_id, timestamp)").Error
	if err != nil {
		logging.Warnf("Failed to create gpu_id+timestamp index: %v", err)
	}

	err = ds.db.Exec("CREATE INDEX IF NOT EXISTS idx_telemetry_hostname ON telemetry_data (hostname)").Error
	if err != nil {
		logging.Warnf("Failed to create hostname index: %v", err)
	}

	logging.Infof("Database schema migration completed")
	return nil
}

// SaveTelemetryData saves a single telemetry data point
func (ds *DatabaseService) SaveTelemetryData(data *models.TelemetryData) error {
	result := ds.db.Create(data)
	if result.Error != nil {
		logging.Errorf("Failed to save telemetry data: %v", result.Error)
		return result.Error
	}

	logging.Debugf("Saved telemetry data: GPU %s, Metric %s, Value %f",
		data.GPUID, data.MetricName, data.Value)
	return nil
}

// SaveTelemetryDataBatch saves multiple telemetry data points in a batch
func (ds *DatabaseService) SaveTelemetryDataBatch(dataList []*models.TelemetryData) error {
	if len(dataList) == 0 {
		return nil
	}

	// Use batch insert for better performance
	batchSize := 100
	for i := 0; i < len(dataList); i += batchSize {
		end := i + batchSize
		if end > len(dataList) {
			end = len(dataList)
		}

		batch := dataList[i:end]
		result := ds.db.CreateInBatches(batch, batchSize)
		if result.Error != nil {
			logging.Errorf("Failed to save telemetry data batch: %v", result.Error)
			return result.Error
		}
	}

	logging.Debugf("Saved batch of %d telemetry data points", len(dataList))
	return nil
}

// GetGPUs returns all unique GPUs that have telemetry data
func (ds *DatabaseService) GetGPUs() ([]models.GPU, error) {
	var gpus []models.GPU

	result := ds.db.Model(&models.TelemetryData{}).
		Select("DISTINCT gpu_id, uuid, model_name, hostname, device").
		Where("gpu_id != '' AND uuid != ''").
		Scan(&gpus)

	if result.Error != nil {
		logging.Errorf("Failed to get GPUs: %v", result.Error)
		return nil, result.Error
	}

	logging.Debugf("Found %d unique GPUs", len(gpus))
	return gpus, nil
}

// QueryTelemetryData queries telemetry data based on the provided query parameters
func (ds *DatabaseService) QueryTelemetryData(query *models.TelemetryQuery) (*models.TelemetryQueryResponse, error) {
	var telemetryData []models.TelemetryData
	var totalCount int64

	// Build the base query
	baseQuery := ds.db.Model(&models.TelemetryData{})

	// Apply filters
	if query.GPUID != "" {
		baseQuery = baseQuery.Where("gpu_id = ?", query.GPUID)
	}

	if !query.StartTime.IsZero() {
		baseQuery = baseQuery.Where("timestamp >= ?", query.StartTime)
	}

	if !query.EndTime.IsZero() {
		baseQuery = baseQuery.Where("timestamp <= ?", query.EndTime)
	}

	// Get total count
	err := baseQuery.Count(&totalCount).Error
	if err != nil {
		logging.Errorf("Failed to count telemetry data: %v", err)
		return nil, err
	}

	// Apply pagination and ordering
	query.Limit = max(query.Limit, 1000) // Cap at 1000 records
	if query.Limit <= 0 {
		query.Limit = 100 // Default limit
	}

	dataQuery := baseQuery.
		Order("timestamp DESC").
		Limit(query.Limit).
		Offset(query.Offset)

	err = dataQuery.Find(&telemetryData).Error
	if err != nil {
		logging.Errorf("Failed to query telemetry data: %v", err)
		return nil, err
	}

	response := &models.TelemetryQueryResponse{
		DataPoints: telemetryData,
		TotalCount: totalCount,
		HasMore:    int64(query.Offset+len(telemetryData)) < totalCount,
	}

	logging.Debugf("Queried %d telemetry records (total: %d)", len(telemetryData), totalCount)
	return response, nil
}

// GetTelemetryStats returns statistics about stored telemetry data
func (ds *DatabaseService) GetTelemetryStats() (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	// Total records
	var totalRecords int64
	err := ds.db.Model(&models.TelemetryData{}).Count(&totalRecords).Error
	if err != nil {
		return nil, err
	}
	stats["total_records"] = totalRecords

	// Unique GPUs
	var uniqueGPUs int64
	err = ds.db.Model(&models.TelemetryData{}).
		Distinct("gpu_id").
		Count(&uniqueGPUs).Error
	if err != nil {
		return nil, err
	}
	stats["unique_gpus"] = uniqueGPUs

	// Unique hosts
	var uniqueHosts int64
	err = ds.db.Model(&models.TelemetryData{}).
		Distinct("hostname").
		Count(&uniqueHosts).Error
	if err != nil {
		return nil, err
	}
	stats["unique_hosts"] = uniqueHosts

	// Date range
	var minTime, maxTime time.Time
	err = ds.db.Model(&models.TelemetryData{}).
		Select("MIN(timestamp) as min_time, MAX(timestamp) as max_time").
		Scan(&struct {
			MinTime time.Time `gorm:"column:min_time"`
			MaxTime time.Time `gorm:"column:max_time"`
		}{MinTime: minTime, MaxTime: maxTime}).Error
	if err != nil {
		return nil, err
	}
	stats["earliest_timestamp"] = minTime
	stats["latest_timestamp"] = maxTime

	return stats, nil
}

// Health checks the health of the database connection
func (ds *DatabaseService) Health() bool {
	sqlDB, err := ds.db.DB()
	if err != nil {
		return false
	}

	err = sqlDB.Ping()
	return err == nil
}

// Close closes the database connection
func (ds *DatabaseService) Close() error {
	sqlDB, err := ds.db.DB()
	if err != nil {
		return err
	}

	err = sqlDB.Close()
	if err != nil {
		logging.Errorf("Failed to close database connection: %v", err)
		return err
	}

	logging.Infof("Database connection closed")
	return nil
}

// buildConnectionString builds PostgreSQL connection string from config
func buildConnectionString(config *DatabaseConfig) string {
	if config.SSLMode == "" {
		config.SSLMode = "disable"
	}

	return "host=" + config.Host +
		" port=" + strconv.Itoa(config.Port) +
		" user=" + config.User +
		" password=" + config.Password +
		" dbname=" + config.DBName +
		" sslmode=" + config.SSLMode
}

// gormLogWriter implements GORM logger interface using our logging package
type gormLogWriter struct{}

func (w *gormLogWriter) Printf(format string, args ...interface{}) {
	logging.Debugf(format, args...)
}

// Helper function to get max of two integers
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
