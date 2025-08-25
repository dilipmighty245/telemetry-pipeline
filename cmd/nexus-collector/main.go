// Package main provides the entry point for the Nexus Collector service.
//
// The Nexus Collector is responsible for consuming telemetry messages from the etcd-based
// message queue and processing them for storage. It operates as a background service that
// continuously polls for new messages and processes them in batches for optimal performance.
//
// Key responsibilities:
//   - Message consumption from etcd message queue
//   - Telemetry data processing and validation
//   - GPU registration and metadata management
//   - Concurrent processing with configurable worker pools
//
// The collector supports graceful shutdown and can be scaled horizontally up to 10 instances
// for high-throughput scenarios.
package main

import (
	"context"
	"io"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/dilipmighty245/telemetry-pipeline/internal/collector"
	"github.com/dilipmighty245/telemetry-pipeline/pkg/logging"
)

// main is the entry point for the Nexus Collector service.
//
// It initializes the collector service and handles graceful shutdown on SIGINT/SIGTERM signals.
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
			logging.Fatalf("could not run Collector: %v", err)
		}
	}
}

// Run initializes and starts the Nexus Collector service with the provided arguments and output writer.
//
// This function is exported to enable integration testing by allowing tests to control
// the program arguments and capture output. It creates a collector service instance,
// sets up signal handling for graceful shutdown, and delegates execution to the service.
//
// Parameters:
//   - args: Command-line arguments passed to the program (typically os.Args)
//   - stdout: Writer for program output (typically os.Stdout, but can be redirected for testing)
//
// Returns:
//   - error: Any error that occurred during service execution, or nil on successful shutdown
//
// The function sets up a context that will be canceled when SIGINT or SIGTERM is received,
// enabling graceful shutdown of the collector service and all its background workers.
func Run(args []string, stdout io.Writer) error {
	// Create the collector service
	service := &collector.NexusCollectorService{}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	// Run the service
	return service.Run(ctx, args, stdout)
}
