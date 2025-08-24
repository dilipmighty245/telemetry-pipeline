package main

import (
	"context"
	"io"
	"net/http"
	"os"

	"github.com/dilipmighty245/telemetry-pipeline/internal/collector"
	"github.com/dilipmighty245/telemetry-pipeline/pkg/logging"
)

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

// Run accepts the program arguments and where to send output (default: stdout)
// This is exported for use in integration tests
func Run(args []string, stdout io.Writer) error {
	// Create the collector service
	service := &collector.NexusCollectorService{}

	// Run the service
	return service.Run(args, stdout)
}
