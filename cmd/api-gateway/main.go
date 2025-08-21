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
	"io"
	"net/http"
	_ "net/http/pprof" // register pprof handlers
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	_ "github.com/dilipmighty245/telemetry-pipeline/docs" // swagger docs
	"github.com/dilipmighty245/telemetry-pipeline/internal/api"
	"github.com/dilipmighty245/telemetry-pipeline/internal/collector"
	"github.com/dilipmighty245/telemetry-pipeline/pkg/logging"
	"golang.org/x/sync/errgroup"
)

const (
	httpDebugAddr = ":8082"         // pprof bind address
	httpTimeout   = 3 * time.Second // timeouts used to protect the server
)

var (
	port     = flag.Int("port", 8080, "HTTP server port")
	logLevel = flag.String("log-level", "info", "Log level (debug, info, warn, error, fatal)")

	// Database configuration
	dbHost     = flag.String("db-host", "localhost", "Database host")
	dbPort     = flag.Int("db-port", 5433, "Database port")
	dbUser     = flag.String("db-user", "postgres", "Database user")
	dbPassword = flag.String("db-password", "postgres", "Database password")
	dbName     = flag.String("db-name", "telemetry", "Database name")
	dbSSLMode  = flag.String("db-sslmode", "disable", "Database SSL mode")
)

func main() {
	if err := run(os.Args, os.Stdout); err != nil {
		switch err {
		case context.Canceled:
			// not considered error
		case http.ErrServerClosed:
			// not considered error
		default:
			logging.Fatalf("could not run API Gateway: %v", err)
		}
	}
}

// run accepts the program arguments and where to send output (default: stdout)
func run(args []string, _ io.Writer) error {
	flags := flag.NewFlagSet(args[0], flag.ExitOnError)
	port := flags.Int("port", 8080, "HTTP server port")
	logLevel := flags.String("log-level", "info", "Log level (debug, info, warn, error, fatal)")

	// Database configuration
	dbHost := flags.String("db-host", "localhost", "Database host")
	dbPort := flags.Int("db-port", 5433, "Database port")
	dbUser := flags.String("db-user", "postgres", "Database user")
	dbPassword := flags.String("db-password", "postgres", "Database password")
	dbName := flags.String("db-name", "telemetry", "Database name")
	dbSSLMode := flags.String("db-sslmode", "disable", "Database SSL mode")

	if err := flags.Parse(args[1:]); err != nil {
		return err
	}

	// Set log level
	logging.SetLogLevel(*logLevel, "")
	logging.Infof("set log level to %q", *logLevel)
	logging.Infof("starting Telemetry API Gateway")

	// Override with environment variables if available
	overrideWithEnvVars(port, dbHost, dbPort, dbUser, dbPassword, dbName, dbSSLMode)

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
		return err
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

	// main HTTP server
	errGrp.Go(func() error {
		logging.Infof("API Gateway listening on port %d", *port)
		logging.Infof("Swagger documentation available at http://localhost:%d/swagger/index.html", *port)
		return server.ListenAndServe()
	})

	// graceful shutdown handler
	errGrp.Go(func() error {
		<-doneCh
		logging.Infof("attempting graceful shutdown of API Gateway")
		server.SetKeepAlivesEnabled(false)
		closeCtx, closeFn := context.WithTimeout(context.Background(), 30*time.Second)
		defer closeFn()
		return server.Shutdown(closeCtx)
	})

	return errGrp.Wait()
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

// overrideWithEnvVars overrides flag values with environment variables if available
func overrideWithEnvVars(port *int, dbHost *string, dbPort *int, dbUser *string, dbPassword *string, dbName *string, dbSSLMode *string) {
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
