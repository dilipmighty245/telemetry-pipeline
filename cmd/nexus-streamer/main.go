// Package main provides the entry point for the Nexus Streamer service.
//
// The Nexus Streamer is responsible for ingesting CSV files containing GPU telemetry data
// via HTTP API and streaming the processed data to the etcd-based message queue. It operates
// as an HTTP server that accepts CSV file uploads and processes them in batches for optimal
// performance and throughput.
//
// Key responsibilities:
//   - HTTP API for CSV file upload (/api/v1/csv/upload)
//   - CSV parsing and validation
//   - Batch processing (configurable batch sizes: 100-1000 records)
//   - Message streaming to etcd queue
//   - Load balancing support for multiple instances
//
// The streamer supports high throughput (10,000+ records/second per instance) and can be
// scaled horizontally up to 10 instances for handling large-scale telemetry ingestion.
package main

import (
	"context"
	"io"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/dilipmighty245/telemetry-pipeline/internal/streamer"
	"github.com/dilipmighty245/telemetry-pipeline/pkg/logging"
)

// main is the entry point for the Nexus Streamer service.
//
// It initializes the streamer service and handles graceful shutdown on SIGINT/SIGTERM signals.
// The function distinguishes between expected shutdown scenarios (context cancellation,
// server closed) and actual errors that should be logged as fatal.
//
// Exit behavior:
//   - Clean shutdown on SIGINT/SIGTERM: exits with code 0
//   - Server closed normally: exits with code 0
//   - Other errors: logs fatal error and exits with non-zero code
func main() {
	if err := Run(os.Args, os.Stdout); err != nil {
		switch err {
		case context.Canceled:
			// not considered error
			return
		case http.ErrServerClosed:
			// not considered error
			return
		default:
			logging.Fatalf("could not run Streamer: %v", err)
		}
	}
}

// Run initializes and starts the Nexus Streamer service with the provided arguments and output writer.
//
// This function is exported to enable integration testing by allowing tests to control
// the program arguments and capture output. It creates a streamer service instance,
// sets up signal handling for graceful shutdown, and delegates execution to the service.
//
// The streamer service provides:
//   - HTTP server on configurable port (default: 8081)
//   - POST /api/v1/csv/upload endpoint for CSV file ingestion
//   - CSV parsing with validation and error handling
//   - Batch processing with configurable batch sizes
//   - Message streaming to etcd-based queue
//
// Parameters:
//   - args: Command-line arguments passed to the program (typically os.Args)
//   - stdout: Writer for program output (typically os.Stdout, but can be redirected for testing)
//
// Returns:
//   - error: Any error that occurred during service execution, or nil on successful shutdown
//
// The function sets up a context that will be canceled when SIGINT or SIGTERM is received,
// enabling graceful shutdown of the HTTP server and completion of in-flight requests.
func Run(args []string, stdout io.Writer) error {
	// Create the streamer service
	service := &streamer.NexusStreamerService{}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	// Run the service
	return service.Run(ctx, args, stdout)
}
