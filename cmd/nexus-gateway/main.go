// Package main provides the Nexus API Gateway service
//
// @title Nexus Telemetry API Gateway
// @version 2.0
// @description API Gateway component for Nexus-enhanced Telemetry Pipeline providing REST API endpoints for querying telemetry data, with GraphQL and WebSocket support
// @termsOfService http://swagger.io/terms/
//
// @contact.name API Support
// @contact.url http://www.example.com/support
// @contact.email support@example.com
//
// @license.name MIT
// @license.url https://opensource.org/licenses/MIT
//
// @host localhost:8080
// @BasePath /
// @schemes http https
//
// @securityDefinitions.basic BasicAuth
//
// @externalDocs.description OpenAPI
// @externalDocs.url https://swagger.io/resources/open-api/
package main

import (
	"context"
	"io"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/dilipmighty245/telemetry-pipeline/internal/gateway"
	"github.com/dilipmighty245/telemetry-pipeline/pkg/logging"
)

// main is the entry point for the Nexus Gateway service.
//
// It initializes the gateway service and handles graceful shutdown on SIGINT/SIGTERM signals.
// The gateway provides REST, GraphQL, and WebSocket APIs for querying telemetry data.
// The function distinguishes between expected shutdown scenarios and actual errors.
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
		case http.ErrServerClosed:
			// not considered error
		default:
			logging.Fatalf("could not run Gateway Service: %v", err)
		}
	}
}

// Run initializes and starts the Nexus Gateway service with the provided arguments and output writer.
//
// This function is exported to enable integration testing by allowing tests to control
// the program arguments and capture output. It creates a gateway service instance,
// sets up signal handling for graceful shutdown, and delegates execution to the service.
//
// The gateway service provides:
//   - REST API endpoints (/api/v1/gpus, /api/v1/gpus/{id}/telemetry)
//   - GraphQL interface for flexible queries
//   - WebSocket support for real-time data streaming
//   - OpenAPI documentation at /swagger/
//   - Health check endpoint at /health
//
// Parameters:
//   - args: Command-line arguments passed to the program (typically os.Args)
//   - stdout: Writer for program output (typically os.Stdout, but can be redirected for testing)
//
// Returns:
//   - error: Any error that occurred during service execution, or nil on successful shutdown
//
// The function sets up a context that will be canceled when SIGINT or SIGTERM is received,
// enabling graceful shutdown of the HTTP server and all active connections.
func Run(args []string, stdout io.Writer) error {
	// Create the gateway service
	service := &gateway.NexusGatewayService{}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Run the service
	return service.Run(ctx, args, stdout)
}
