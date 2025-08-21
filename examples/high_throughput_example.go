package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/cf/telemetry-pipeline/internal/collector"
	"github.com/cf/telemetry-pipeline/internal/streaming"
	"github.com/cf/telemetry-pipeline/internal/streamer"
	"github.com/cf/telemetry-pipeline/pkg/messagequeue"
)

// Example: High-Throughput Telemetry Pipeline using newStreamAdapter patterns
func main() {
	fmt.Println("🚀 Starting High-Throughput Telemetry Pipeline")
	fmt.Println("📊 Leveraging patterns from prtc's newStreamAdapter")

	// Create message queue service
	mqConfig := &messagequeue.Config{
		Type: "redis",
		Redis: &messagequeue.RedisConfig{
			Host:     "localhost",
			Port:     6379,
			PoolSize: 50,
		},
	}
	
	mqService, err := messagequeue.NewMessageQueueService(mqConfig)
	if err != nil {
		log.Fatalf("Failed to create message queue service: %v", err)
	}

	// Start enhanced services
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 1. Start Enhanced Streamer (High-throughput data ingestion)
	streamerService := startEnhancedStreamer(mqService)
	
	// 2. Start Enhanced Collector (High-throughput data collection and streaming)
	collectorService := startEnhancedCollector(mqService)

	// 3. Start metrics monitoring
	go monitorPerformance(streamerService, collectorService)

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	fmt.Println("✅ Pipeline started successfully!")
	fmt.Println("📈 Monitor metrics at http://localhost:8080/metrics")
	fmt.Println("🔍 Health check at http://localhost:8080/health")
	fmt.Println("Press Ctrl+C to stop...")

	<-sigChan
	fmt.Println("\n🛑 Shutting down pipeline...")

	// Graceful shutdown
	shutdownServices(streamerService, collectorService)
	fmt.Println("✅ Pipeline stopped successfully")
}

func startEnhancedStreamer(mqService *messagequeue.MessageQueueService) *streamer.EnhancedStreamerService {
	config := &streamer.EnhancedStreamerConfig{
		StreamerConfig: &streamer.StreamerConfig{
			CSVFilePath:    "/data/dcgm_metrics.csv",
			BatchSize:      1000,
			StreamInterval: 200 * time.Millisecond,
			LoopMode:       true,
			MaxRetries:     3,
			RetryDelay:     time.Second,
			EnableMetrics:  true,
			StreamerID:     "high-throughput-streamer",
			BufferSize:     2000,
		},
		
		// Enhanced features
		EnableStreaming:     true,
		StreamDestination:   "http://analytics-service/telemetry",
		
		EnableParallelStreaming: true,
		ParallelWorkers:        6,
		
		EnableRateLimit: true,
		RateLimit:      5000.0, // 5000 records/sec
		BurstSize:      500,
		
		EnableBackPressure:    true,
		BackPressureThreshold: 80.0,
		BackPressureDelay:     100 * time.Millisecond,
		
		StreamingConfig: &streaming.StreamAdapterConfig{
			ChannelSize:   3000,
			BatchSize:     1500,
			Workers:       12,
			MaxRetries:    3,
			RetryDelay:    time.Second,
			FlushInterval: 2 * time.Second,
			HTTPTimeout:   30 * time.Second,
			EnableMetrics: true,
			PartitionBy:   "hostname",
		},
	}

	service, err := streamer.NewEnhancedStreamerService(config, mqService)
	if err != nil {
		log.Fatalf("Failed to create enhanced streamer service: %v", err)
	}

	err = service.Start()
	if err != nil {
		log.Fatalf("Failed to start enhanced streamer service: %v", err)
	}

	fmt.Println("✅ Enhanced Streamer started with:")
	fmt.Printf("   📊 Batch size: %d\n", config.BatchSize)
	fmt.Printf("   🔄 Workers: %d\n", config.ParallelWorkers)
	fmt.Printf("   ⚡ Rate limit: %.0f records/sec\n", config.RateLimit)
	fmt.Printf("   🔧 Streaming workers: %d\n", config.StreamingConfig.Workers)

	return service
}

func startEnhancedCollector(mqService *messagequeue.MessageQueueService) *collector.EnhancedCollectorService {
	config := &collector.EnhancedCollectorConfig{
		CollectorConfig: &collector.CollectorConfig{
			CollectorID:   "high-throughput-collector",
			ConsumerGroup: "telemetry-collectors",
			BatchSize:     500,
			PollInterval:  500 * time.Millisecond,
			MaxRetries:    3,
			RetryDelay:    time.Second,
			BufferSize:    2000,
			EnableMetrics: true,
			DatabaseConfig: &collector.DatabaseConfig{
				Host:         "localhost",
				Port:         5432,
				Database:     "telemetry",
				Username:     "telemetry_user",
				Password:     "telemetry_pass",
				MaxConns:     50,
				MaxIdleConns: 10,
			},
		},
		
		// Enhanced features
		EnableStreaming:   true,
		StreamDestination: "http://analytics-service/telemetry",
		
		EnableCircuitBreaker: true,
		CircuitBreakerConfig: &collector.CircuitBreakerConfig{
			FailureThreshold: 5,
			RecoveryTimeout:  30 * time.Second,
			HalfOpenRequests: 3,
		},
		
		EnableAdaptiveBatching: true,
		MinBatchSize:          100,
		MaxBatchSize:          2000,
		
		EnableLoadBalancing: true,
		Workers:            8,
		
		StreamingConfig: &streaming.StreamAdapterConfig{
			ChannelSize:   2000,
			BatchSize:     1000,
			Workers:       10,
			MaxRetries:    3,
			RetryDelay:    time.Second,
			FlushInterval: 3 * time.Second,
			HTTPTimeout:   30 * time.Second,
			EnableMetrics: true,
			PartitionBy:   "hostname",
		},
	}

	service, err := collector.NewEnhancedCollectorService(config, mqService)
	if err != nil {
		log.Fatalf("Failed to create enhanced collector service: %v", err)
	}

	err = service.Start()
	if err != nil {
		log.Fatalf("Failed to start enhanced collector service: %v", err)
	}

	fmt.Println("✅ Enhanced Collector started with:")
	fmt.Printf("   📊 Base batch size: %d (adaptive: %d-%d)\n", 
		config.BatchSize, config.MinBatchSize, config.MaxBatchSize)
	fmt.Printf("   🔄 Workers: %d\n", config.Workers)
	fmt.Printf("   🛡️ Circuit breaker: enabled\n")
	fmt.Printf("   🔧 Streaming workers: %d\n", config.StreamingConfig.Workers)

	return service
}

func monitorPerformance(streamerService *streamer.EnhancedStreamerService, collectorService *collector.EnhancedCollectorService) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			fmt.Println("\n📊 === Performance Metrics ===")
			
			// Streamer metrics
			streamerMetrics := streamerService.GetEnhancedMetrics()
			if baseMetrics, ok := streamerMetrics["base_metrics"].(*streamer.StreamerMetrics); ok {
				fmt.Printf("📤 Streamer: %.0f records/sec, %d total streamed\n", 
					baseMetrics.StreamingRate, baseMetrics.TotalRecordsStreamed)
			}
			
			if streamingMetrics, ok := streamerMetrics["streaming_metrics"].(*streaming.StreamMetrics); ok {
				fmt.Printf("   🔄 Streaming: %.0f records/sec, %.1f%% channel util\n", 
					streamingMetrics.ProcessingRate, streamingMetrics.ChannelUtilization)
			}

			// Collector metrics
			collectorMetrics := collectorService.GetEnhancedMetrics()
			if baseMetrics, ok := collectorMetrics["base_metrics"].(*collector.CollectorMetrics); ok {
				fmt.Printf("📥 Collector: %.0f records/sec, %d total collected\n", 
					baseMetrics.CollectionRate, baseMetrics.TotalRecordsCollected)
			}
			
			if streamingMetrics, ok := collectorMetrics["streaming_metrics"].(*streaming.StreamMetrics); ok {
				fmt.Printf("   🔄 Streaming: %.0f records/sec, %.1f%% channel util\n", 
					streamingMetrics.ProcessingRate, streamingMetrics.ChannelUtilization)
			}

			// Circuit breaker status
			if cbMetrics, ok := collectorMetrics["circuit_breaker"].(map[string]interface{}); ok {
				fmt.Printf("   🛡️ Circuit breaker: state=%v, failures=%v\n", 
					cbMetrics["state"], cbMetrics["failures"])
			}

			// Adaptive batcher status
			if abMetrics, ok := collectorMetrics["adaptive_batcher"].(map[string]interface{}); ok {
				fmt.Printf("   🎯 Adaptive batch: size=%v, load_avg=%.2f\n", 
					abMetrics["current_batch_size"], abMetrics["load_average"])
			}

			fmt.Println("=============================")
		}
	}
}

func shutdownServices(streamerService *streamer.EnhancedStreamerService, collectorService *collector.EnhancedCollectorService) {
	// Stop services gracefully
	if err := streamerService.Stop(); err != nil {
		log.Printf("Error stopping streamer service: %v", err)
	}

	if err := collectorService.Stop(); err != nil {
		log.Printf("Error stopping collector service: %v", err)
	}

	// Give services time to finish gracefully
	time.Sleep(2 * time.Second)
}

// Example: Direct streaming without CSV
func exampleDirectStreaming(service *streamer.EnhancedStreamerService) {
	// Create sample telemetry data
	sampleData := []*models.TelemetryData{
		{
			Hostname:    "gpu-node-1",
			GPUID:       "GPU-001",
			MetricName:  "gpu_utilization",
			MetricValue: 85.5,
			Timestamp:   time.Now(),
		},
		{
			Hostname:    "gpu-node-1",
			GPUID:       "GPU-002",
			MetricName:  "memory_usage",
			MetricValue: 12.8,
			Timestamp:   time.Now(),
		},
	}

	// Stream directly (bypassing CSV)
	err := service.StreamDirectly(sampleData)
	if err != nil {
		log.Printf("Failed to stream data directly: %v", err)
	} else {
		fmt.Printf("✅ Streamed %d records directly\n", len(sampleData))
	}
}
