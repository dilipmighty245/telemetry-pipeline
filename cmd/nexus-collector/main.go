package main

import (
	"context"
	"io"
	"os"

	"github.com/dilipmighty245/telemetry-pipeline/internal/collector"
	log "github.com/sirupsen/logrus"
)

func main() {
	if err := Run(os.Args, os.Stdout); err != nil {
		switch err {
		case context.Canceled:
			// not considered error
		default:
			log.Fatalf("could not run Nexus Collector: %v", err)
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
