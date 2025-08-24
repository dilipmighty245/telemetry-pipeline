package main

import (
	"context"
	"io"
	"net/http"
	"os"

	"github.com/dilipmighty245/telemetry-pipeline/internal/streamer"
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
			logging.Fatalf("could not run Streamer: %v", err)
		}
	}
}

// Run accepts the program arguments and where to send output (default: stdout)
// This is exported for use in integration tests
func Run(args []string, stdout io.Writer) error {
	// Create the streamer service
	service := &streamer.NexusStreamerService{}

	// Run the service
	return service.Run(args, stdout)
}
