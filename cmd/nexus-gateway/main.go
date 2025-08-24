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

	"github.com/dilipmighty245/telemetry-pipeline/internal/gateway"
	"github.com/dilipmighty245/telemetry-pipeline/pkg/logging"
)

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

// Run accepts the program arguments and where to send output (default: stdout)
// This is exported for use in integration tests
func Run(args []string, stdout io.Writer) error {
	// Create the gateway service
	service := &gateway.NexusGatewayService{}

	// Run the service
	return service.Run(args, stdout)
}
