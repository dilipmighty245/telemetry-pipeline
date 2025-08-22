package main

import (
	"flag"
	"os"

	"github.com/dilipmighty245/telemetry-pipeline/internal/collector"
	"github.com/dilipmighty245/telemetry-pipeline/pkg/config"
	"github.com/dilipmighty245/telemetry-pipeline/pkg/logging"
)

func main() {
	// Parse command line flags
	var configFile string
	flag.StringVar(&configFile, "config", "", "Path to configuration file")
	flag.Parse()

	// Load configuration
	cfg, err := config.LoadFromEnv()
	if err != nil {
		logging.Errorf("Failed to load configuration: %v", err)
		os.Exit(1)
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		logging.Errorf("Invalid configuration: %v", err)
		os.Exit(1)
	}

	// Log effective configuration
	cfg.LogEffectiveConfig()

	// Create enhanced collector
	enhancedCollector, err := collector.NewEnhancedCollector(cfg)
	if err != nil {
		logging.Errorf("Failed to create enhanced collector: %v", err)
		os.Exit(1)
	}

	// Start the collector
	if err := enhancedCollector.Start(); err != nil {
		logging.Errorf("Failed to start enhanced collector: %v", err)
		os.Exit(1)
	}

	logging.Infof("Enhanced collector started successfully")

	// Wait for shutdown
	enhancedCollector.WaitForShutdown()

	logging.Infof("Enhanced collector shutdown complete")
}
