package main

import (
	"context"
	"flag"
	"io"
	"net/http"
	_ "net/http/pprof" // register pprof handlers
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/dilipmighty245/telemetry-pipeline/internal/collector"
	"github.com/dilipmighty245/telemetry-pipeline/pkg/logging"
	"github.com/dilipmighty245/telemetry-pipeline/pkg/messagequeue"
	"golang.org/x/sync/errgroup"
)

const (
	httpDebugAddr = ":8083"         // pprof bind address
	httpTimeout   = 3 * time.Second // timeouts used to protect the server
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
	dbPort     = flag.Int("db-port", 5433, "Database port")
	dbUser     = flag.String("db-user", "postgres", "Database user")
	dbPassword = flag.String("db-password", "postgres", "Database password")
	dbName     = flag.String("db-name", "telemetry", "Database name")
	dbSSLMode  = flag.String("db-sslmode", "disable", "Database SSL mode")
)

func main() {
	if err := run(os.Args, os.Stdout); err != nil {
		switch err {
		case context.Canceled:
			// not considered error
		case http.ErrServerClosed:
			// not considered error
		default:
			logging.Fatalf("could not run Collector Service: %v", err)
		}
	}
}

// run accepts the program arguments and where to send output (default: stdout)
func run(args []string, _ io.Writer) error {
	flags := flag.NewFlagSet(args[0], flag.ExitOnError)
	collectorID := flags.String("collector-id", "", "Unique identifier for this collector instance")
	consumerGroup := flags.String("consumer-group", "telemetry-collectors", "Consumer group name")
	batchSize := flags.Int("batch-size", 100, "Number of messages to process in each batch")
	pollInterval := flags.Duration("poll-interval", 1*time.Second, "Interval between polling for messages")
	maxRetries := flags.Int("max-retries", 3, "Maximum number of retries for failed operations")
	retryDelay := flags.Duration("retry-delay", 1*time.Second, "Delay between retries")
	bufferSize := flags.Int("buffer-size", 1000, "Internal buffer size for batching")
	logLevel := flags.String("log-level", "info", "Log level (debug, info, warn, error, fatal)")

	// Database configuration
	dbHost := flags.String("db-host", "localhost", "Database host")
	dbPort := flags.Int("db-port", 5433, "Database port")
	dbUser := flags.String("db-user", "postgres", "Database user")
	dbPassword := flags.String("db-password", "postgres", "Database password")
	dbName := flags.String("db-name", "telemetry", "Database name")
	dbSSLMode := flags.String("db-sslmode", "disable", "Database SSL mode")

	if err := flags.Parse(args[1:]); err != nil {
		return err
	}

	// Set log level
	logging.SetLogLevel(*logLevel, "")
	logging.Infof("set log level to %q", *logLevel)
	logging.Infof("starting Telemetry Collector Service")

	// Override with environment variables if available
	overrideWithEnvVars(dbHost, dbPort, dbUser, dbPassword, dbName, dbSSLMode)

	logging.Infof("Collector ID: %s", getCollectorID(*collectorID))
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
		CollectorID:    getCollectorID(*collectorID),
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
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	doneCh := make(chan struct{})
	errGrp, egCtx := errgroup.WithContext(ctx)

	// signal handler
	errGrp.Go(func() error {
		err := handleSignals(egCtx, cancel)
		doneCh <- struct{}{}
		return err
	})

	// pprof handler
	errGrp.Go(func() error {
		logging.Infof("starting pprof server on %s", httpDebugAddr)
		return pprofHandler(egCtx)
	})

	// collector service
	errGrp.Go(func() error {
		err := collectorService.Start()
		if err != nil {
			return err
		}
		<-egCtx.Done()
		logging.Infof("shutting down collector service...")
		return collectorService.Stop()
	})

	// metrics reporter
	errGrp.Go(func() error {
		return metricsReporter(egCtx, collectorService)
	})

	// graceful shutdown handler
	errGrp.Go(func() error {
		<-doneCh
		logging.Infof("attempting graceful shutdown of Collector Service")
		return nil
	})

	return errGrp.Wait()
}

// metricsReporter periodically reports metrics
func metricsReporter(ctx context.Context, service *collector.CollectorService) error {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
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
func getCollectorID(collectorID string) string {
	if collectorID != "" {
		return collectorID
	}

	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}

	pid := os.Getpid()
	return hostname + "-collector-" + strconv.Itoa(pid) + "-" + time.Now().Format("20060102150405")
}

// handleSignals will handle Interrupts or termination signals
func handleSignals(ctx context.Context, cancel context.CancelFunc) error {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	select {
	case s := <-sigCh:
		logging.Infof("got signal %v, stopping", s)
		cancel()
		return nil
	case <-ctx.Done():
		logging.Infof("context is done")
		return ctx.Err()
	}
}

// pprofHandler launches a http server and serves pprof debug information under
// http://<address>/debug/pprof
func pprofHandler(ctx context.Context) error {
	srv := http.Server{
		Addr:         httpDebugAddr,
		ReadTimeout:  httpTimeout,
		WriteTimeout: httpTimeout,
	}

	go func() {
		<-ctx.Done()
		logging.Infof("attempting graceful shutdown of pprof server")
		srv.SetKeepAlivesEnabled(false)
		closeCtx, closeFn := context.WithTimeout(context.Background(), 3*time.Second)
		defer closeFn()
		_ = srv.Shutdown(closeCtx)
	}()

	return srv.ListenAndServe()
}

// overrideWithEnvVars overrides flag values with environment variables if available
func overrideWithEnvVars(dbHost *string, dbPort *int, dbUser *string, dbPassword *string, dbName *string, dbSSLMode *string) {
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
