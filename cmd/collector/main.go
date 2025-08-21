package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/cf/telemetry-pipeline/internal/collector"
	"github.com/cf/telemetry-pipeline/pkg/logging"
	"github.com/cf/telemetry-pipeline/pkg/messagequeue"
)

var (
	collectorID   = flag.String("collector-id", "", "Unique identifier for this collector instance")
	consumerGroup = flag.String("consumer-group", "telemetry-collectors", "Consumer group name")
	batchSize     = flag.Int("batch-size", 100, "Number of messages to process in each batch")
	pollInterval  = flag.Duration("poll-interval", 1*time.Second, "Interval between polling for messages")
	maxRetries    = flag.Int("max-retries", 3, "Maximum number of retries for failed operations")
	retryDelay    = flag.Duration("retry-delay", 1*time.Second, "Delay between retries")
	bufferSize    = flag.Int("buffer-size", 1000, "Internal buffer size for batching")
	logLevel      = flag.String("log-level", "info", "Log level (debug, info, warn, error, fatal)")

	// Database configuration
	dbHost     = flag.String("db-host", "localhost", "Database host")
	dbPort     = flag.Int("db-port", 5432, "Database port")
	dbUser     = flag.String("db-user", "postgres", "Database user")
	dbPassword = flag.String("db-password", "postgres", "Database password")
	dbName     = flag.String("db-name", "telemetry", "Database name")
	dbSSLMode  = flag.String("db-sslmode", "disable", "Database SSL mode")
)

func main() {
	flag.Parse()

	// Set log level
	logging.SetLogLevel(*logLevel, "")

	logging.Infof("Starting Telemetry Collector Service")
	logging.Infof("Collector ID: %s", getCollectorID())
	logging.Infof("Consumer Group: %s", *consumerGroup)
	logging.Infof("Batch Size: %d", *batchSize)
	logging.Infof("Poll Interval: %v", *pollInterval)
	logging.Infof("Database: %s@%s:%d/%s", *dbUser, *dbHost, *dbPort, *dbName)

	// Create message queue service
	mqService := messagequeue.NewMessageQueueService()
	defer mqService.Stop()

	// Create database configuration
	dbConfig := &collector.DatabaseConfig{
		Host:     *dbHost,
		Port:     *dbPort,
		User:     *dbUser,
		Password: *dbPassword,
		DBName:   *dbName,
		SSLMode:  *dbSSLMode,
	}

	// Create collector configuration
	config := &collector.CollectorConfig{
		CollectorID:    getCollectorID(),
		ConsumerGroup:  *consumerGroup,
		BatchSize:      *batchSize,
		PollInterval:   *pollInterval,
		MaxRetries:     *maxRetries,
		RetryDelay:     *retryDelay,
		BufferSize:     *bufferSize,
		EnableMetrics:  true,
		DatabaseConfig: dbConfig,
	}

	// Create collector service
	collectorService, err := collector.NewCollectorService(config, mqService)
	if err != nil {
		logging.Fatalf("Failed to create collector service: %v", err)
	}

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		logging.Infof("Received shutdown signal, stopping collector service...")
		cancel()
	}()

	// Start collector service
	err = collectorService.Start()
	if err != nil {
		logging.Fatalf("Failed to start collector service: %v", err)
	}

	// Start metrics reporting goroutine
	go metricsReporter(ctx, collectorService)

	// Wait for shutdown signal
	<-ctx.Done()

	// Graceful shutdown
	logging.Infof("Shutting down collector service...")
	err = collectorService.Stop()
	if err != nil {
		logging.Errorf("Error stopping collector service: %v", err)
	}

	logging.Infof("Telemetry Collector Service stopped")
}

// metricsReporter periodically reports metrics
func metricsReporter(ctx context.Context, service *collector.CollectorService) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			metrics := service.GetMetrics()
			logging.Infof("Collector Metrics - Collected: %d, Persisted: %d, Batches: %d, Errors: %d, Rate: %.2f records/sec",
				metrics.TotalRecordsCollected,
				metrics.TotalRecordsPersisted,
				metrics.TotalBatchesCollected,
				metrics.TotalErrors,
				metrics.CollectionRate)

			if lastError := service.GetLastError(); lastError != nil {
				logging.Warnf("Last error: %v", lastError)
			}

			// Report database stats if available
			if dbService := service.GetDatabaseService(); dbService != nil {
				stats, err := dbService.GetTelemetryStats()
				if err == nil {
					logging.Infof("Database Stats - Total Records: %v, Unique GPUs: %v, Unique Hosts: %v",
						stats["total_records"], stats["unique_gpus"], stats["unique_hosts"])
				}
			}
		}
	}
}

// getCollectorID generates a collector ID if not provided
func getCollectorID() string {
	if *collectorID != "" {
		return *collectorID
	}

	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}

	pid := os.Getpid()
	return hostname + "-collector-" + strconv.Itoa(pid) + "-" + time.Now().Format("20060102150405")
}

// getEnvOrDefault returns environment variable value or default
func getEnvOrDefault(envVar, defaultValue string) string {
	if value := os.Getenv(envVar); value != "" {
		return value
	}
	return defaultValue
}

func init() {
	// Override with environment variables if available
	if envHost := os.Getenv("DB_HOST"); envHost != "" {
		*dbHost = envHost
	}
	if envPort := os.Getenv("DB_PORT"); envPort != "" {
		if port, err := strconv.Atoi(envPort); err == nil {
			*dbPort = port
		}
	}
	if envUser := os.Getenv("DB_USER"); envUser != "" {
		*dbUser = envUser
	}
	if envPassword := os.Getenv("DB_PASSWORD"); envPassword != "" {
		*dbPassword = envPassword
	}
	if envName := os.Getenv("DB_NAME"); envName != "" {
		*dbName = envName
	}
	if envSSLMode := os.Getenv("DB_SSLMODE"); envSSLMode != "" {
		*dbSSLMode = envSSLMode
	}
}
