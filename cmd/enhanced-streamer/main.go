package main

import (
	"flag"
	"os"

	"github.com/dilipmighty245/telemetry-pipeline/internal/streamer"
	"github.com/dilipmighty245/telemetry-pipeline/pkg/config"
	"github.com/dilipmighty245/telemetry-pipeline/pkg/logging"
)

func main() {
	// Parse command line flags
	var configFile string
	var csvFile string
	flag.StringVar(&configFile, "config", "", "Path to configuration file")
	flag.StringVar(&csvFile, "csv", "", "Path to CSV file to stream")
	flag.Parse()

	// Load configuration
	cfg, err := config.LoadFromEnv()
	if err != nil {
		logging.Errorf("Failed to load configuration: %v", err)
		os.Exit(1)
	}

	// Override CSV file if provided via command line
	if csvFile != "" {
		cfg.Streamer.CSVFilePath = csvFile
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		logging.Errorf("Invalid configuration: %v", err)
		os.Exit(1)
	}

	// Check if CSV file is specified
	if cfg.Streamer.CSVFilePath == "" {
		logging.Errorf("CSV file path must be specified via --csv flag or STREAMER_CSV_FILE_PATH environment variable")
		os.Exit(1)
	}

	// Log effective configuration
	cfg.LogEffectiveConfig()

	// Create enhanced streamer
	enhancedStreamer, err := streamer.NewEnhancedStreamer(cfg)
	if err != nil {
		logging.Errorf("Failed to create enhanced streamer: %v", err)
		os.Exit(1)
	}

	// Start the streamer
	if err := enhancedStreamer.Start(); err != nil {
		logging.Errorf("Failed to start enhanced streamer: %v", err)
		os.Exit(1)
	}

	logging.Infof("Enhanced streamer started successfully")

	// Wait for shutdown
	enhancedStreamer.WaitForShutdown()

	logging.Infof("Enhanced streamer shutdown complete")
}
