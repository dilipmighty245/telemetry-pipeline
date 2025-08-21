package main

import (
	"context"
	"flag"
	"io"
	"net/http"
	_ "net/http/pprof" // register pprof handlers
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/dilipmighty245/telemetry-pipeline/internal/streamer"
	"github.com/dilipmighty245/telemetry-pipeline/pkg/logging"
	"github.com/dilipmighty245/telemetry-pipeline/pkg/messagequeue"
	"golang.org/x/sync/errgroup"
)

const (
	httpDebugAddr = ":8084"         // pprof bind address
	httpTimeout   = 3 * time.Second // timeouts used to protect the server
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
	if err := run(os.Args, os.Stdout); err != nil {
		switch err {
		case context.Canceled:
			// not considered error
		case http.ErrServerClosed:
			// not considered error
		default:
			logging.Fatalf("could not run Streamer Service: %v", err)
		}
	}
}

// run accepts the program arguments and where to send output (default: stdout)
func run(args []string, _ io.Writer) error {
	flags := flag.NewFlagSet(args[0], flag.ExitOnError)
	csvFile := flags.String("csv", "dcgm_metrics_20250718_134233.csv", "Path to CSV file containing telemetry data")
	batchSize := flags.Int("batch-size", 100, "Number of records to process in each batch")
	streamInterval := flags.Duration("stream-interval", 1*time.Second, "Interval between streaming batches")
	loopMode := flags.Bool("loop", true, "Enable loop mode to continuously stream data")
	streamerID := flags.String("streamer-id", "", "Unique identifier for this streamer instance")
	maxRetries := flags.Int("max-retries", 3, "Maximum number of retries for failed operations")
	retryDelay := flags.Duration("retry-delay", 1*time.Second, "Delay between retries")
	logLevel := flags.String("log-level", "info", "Log level (debug, info, warn, error, fatal)")

	if err := flags.Parse(args[1:]); err != nil {
		return err
	}

	// Set log level
	logging.SetLogLevel(*logLevel, "")
	logging.Infof("set log level to %q", *logLevel)
	logging.Infof("starting Telemetry Streamer Service")

	// Ensure CSV file path is absolute if it's relative
	ensureAbsolutePath(csvFile)

	logging.Infof("CSV File: %s", *csvFile)
	logging.Infof("Batch Size: %d", *batchSize)
	logging.Infof("Stream Interval: %v", *streamInterval)
	logging.Infof("Loop Mode: %v", *loopMode)

	// Validate CSV file exists
	if _, err := os.Stat(*csvFile); os.IsNotExist(err) {
		return err
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

	// streamer service
	errGrp.Go(func() error {
		err := streamerService.Start()
		if err != nil {
			return err
		}
		<-egCtx.Done()
		logging.Infof("shutting down streamer service...")
		return streamerService.Stop()
	})

	// metrics reporter
	errGrp.Go(func() error {
		return metricsReporter(egCtx, streamerService)
	})

	// graceful shutdown handler
	errGrp.Go(func() error {
		<-doneCh
		logging.Infof("attempting graceful shutdown of Streamer Service")
		return nil
	})

	return errGrp.Wait()
}

// metricsReporter periodically reports metrics
func metricsReporter(ctx context.Context, service *streamer.StreamerService) error {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
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

// ensureAbsolutePath ensures CSV file path is absolute if it's relative
func ensureAbsolutePath(csvFile *string) {
	if *csvFile != "" && !filepath.IsAbs(*csvFile) {
		wd, err := os.Getwd()
		if err != nil {
			logging.Errorf("Failed to get working directory: %v", err)
		} else {
			*csvFile = filepath.Join(wd, *csvFile)
		}
	}
}
