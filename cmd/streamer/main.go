package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/cf/telemetry-pipeline/internal/streamer"
	"github.com/cf/telemetry-pipeline/pkg/logging"
	"github.com/cf/telemetry-pipeline/pkg/messagequeue"
)

var (
	csvFile        = flag.String("csv", "dcgm_metrics_20250718_134233.csv", "Path to CSV file containing telemetry data")
	batchSize      = flag.Int("batch-size", 100, "Number of records to process in each batch")
	streamInterval = flag.Duration("stream-interval", 1*time.Second, "Interval between streaming batches")
	loopMode       = flag.Bool("loop", true, "Enable loop mode to continuously stream data")
	streamerID     = flag.String("streamer-id", "", "Unique identifier for this streamer instance")
	maxRetries     = flag.Int("max-retries", 3, "Maximum number of retries for failed operations")
	retryDelay     = flag.Duration("retry-delay", 1*time.Second, "Delay between retries")
	logLevel       = flag.String("log-level", "info", "Log level (debug, info, warn, error, fatal)")
)

func main() {
	flag.Parse()

	// Set log level
	logging.SetLogLevel(*logLevel, "")

	logging.Infof("Starting Telemetry Streamer Service")
	logging.Infof("CSV File: %s", *csvFile)
	logging.Infof("Batch Size: %d", *batchSize)
	logging.Infof("Stream Interval: %v", *streamInterval)
	logging.Infof("Loop Mode: %v", *loopMode)

	// Validate CSV file exists
	if _, err := os.Stat(*csvFile); os.IsNotExist(err) {
		logging.Fatalf("CSV file does not exist: %s", *csvFile)
	}

	// Generate streamer ID if not provided
	if *streamerID == "" {
		hostname, _ := os.Hostname()
		*streamerID = hostname + "-streamer-" + time.Now().Format("20060102150405")
	}

	// Create message queue service
	mqService := messagequeue.NewMessageQueueService()
	defer mqService.Stop()

	// Create streamer configuration
	config := &streamer.StreamerConfig{
		CSVFilePath:    *csvFile,
		BatchSize:      *batchSize,
		StreamInterval: *streamInterval,
		LoopMode:       *loopMode,
		MaxRetries:     *maxRetries,
		RetryDelay:     *retryDelay,
		EnableMetrics:  true,
		StreamerID:     *streamerID,
		BufferSize:     1000,
	}

	// Create streamer service
	streamerService, err := streamer.NewStreamerService(config, mqService)
	if err != nil {
		logging.Fatalf("Failed to create streamer service: %v", err)
	}

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		logging.Infof("Received shutdown signal, stopping streamer service...")
		cancel()
	}()

	// Start streamer service
	err = streamerService.Start()
	if err != nil {
		logging.Fatalf("Failed to start streamer service: %v", err)
	}

	// Start metrics reporting goroutine
	go metricsReporter(ctx, streamerService)

	// Wait for shutdown signal
	<-ctx.Done()

	// Graceful shutdown
	logging.Infof("Shutting down streamer service...")
	err = streamerService.Stop()
	if err != nil {
		logging.Errorf("Error stopping streamer service: %v", err)
	}

	logging.Infof("Telemetry Streamer Service stopped")
}

// metricsReporter periodically reports metrics
func metricsReporter(ctx context.Context, service *streamer.StreamerService) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			metrics := service.GetMetrics()
			logging.Infof("Streamer Metrics - Records: %d, Batches: %d, Errors: %d, Rate: %.2f records/sec",
				metrics.TotalRecordsStreamed,
				metrics.TotalBatchesStreamed,
				metrics.TotalErrors,
				metrics.StreamingRate)

			if lastError := service.GetLastError(); lastError != nil {
				logging.Warnf("Last error: %v", lastError)
			}
		}
	}
}

func init() {
	// Ensure CSV file path is absolute if it's relative
	flag.Parse()
	if *csvFile != "" && !filepath.IsAbs(*csvFile) {
		wd, err := os.Getwd()
		if err != nil {
			logging.Errorf("Failed to get working directory: %v", err)
		} else {
			*csvFile = filepath.Join(wd, *csvFile)
		}
	}
}
