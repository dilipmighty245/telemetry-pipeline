// Package main provides the API Gateway service
//
// @title Telemetry Pipeline API
// @version 1.0
// @description Elastic GPU Telemetry Pipeline REST API
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
	"flag"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/cf/telemetry-pipeline/internal/api"
	"github.com/cf/telemetry-pipeline/internal/collector"
	"github.com/cf/telemetry-pipeline/pkg/logging"
)

var (
	port     = flag.Int("port", 8080, "HTTP server port")
	logLevel = flag.String("log-level", "info", "Log level (debug, info, warn, error, fatal)")

	// Database configuration
	dbHost     = flag.String("db-host", "localhost", "Database host")
	dbPort     = flag.Int("db-port", 5432, "Database port")
	dbUser     = flag.String("db-user", "postgres", "Database user")
	dbPassword = flag.String("db-password", "postgres", "Database password")
	dbName     = flag.String("db-name", "telemetry", "Database name")
	dbSSLMode  = flag.String("db-sslmode", "disable", "Database SSL mode")
)

func main() {
	flag.Parse()

	// Set log level
	logging.SetLogLevel(*logLevel, "")

	logging.Infof("Starting Telemetry API Gateway")
	logging.Infof("Port: %d", *port)
	logging.Infof("Database: %s@%s:%d/%s", *dbUser, *dbHost, *dbPort, *dbName)

	// Create database configuration
	dbConfig := &collector.DatabaseConfig{
		Host:     *dbHost,
		Port:     *dbPort,
		User:     *dbUser,
		Password: *dbPassword,
		DBName:   *dbName,
		SSLMode:  *dbSSLMode,
	}

	// Create database service
	dbService, err := collector.NewDatabaseService(dbConfig)
	if err != nil {
		logging.Fatalf("Failed to create database service: %v", err)
	}
	defer dbService.Close()

	// Create API handler
	apiHandler := api.NewAPIHandler(dbService)

	// Setup router
	router := api.SetupRouter(apiHandler)

	// Create HTTP server
	server := &http.Server{
		Addr:         ":" + strconv.Itoa(*port),
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		logging.Infof("Received shutdown signal, stopping API gateway...")
		cancel()

		// Graceful shutdown with timeout
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer shutdownCancel()

		if err := server.Shutdown(shutdownCtx); err != nil {
			logging.Errorf("Error during server shutdown: %v", err)
		}
	}()

	// Start HTTP server
	logging.Infof("API Gateway listening on port %d", *port)
	logging.Infof("Swagger documentation available at http://localhost:%d/swagger/index.html", *port)

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logging.Fatalf("Failed to start HTTP server: %v", err)
	}

	// Wait for shutdown to complete
	<-ctx.Done()
	logging.Infof("Telemetry API Gateway stopped")
}

func init() {
	// Override with environment variables if available
	if envPort := os.Getenv("PORT"); envPort != "" {
		if p, err := strconv.Atoi(envPort); err == nil {
			*port = p
		}
	}
	if envHost := os.Getenv("DB_HOST"); envHost != "" {
		*dbHost = envHost
	}
	if envPort := os.Getenv("DB_PORT"); envPort != "" {
		if p, err := strconv.Atoi(envPort); err == nil {
			*dbPort = p
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
