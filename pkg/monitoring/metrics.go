// Package monitoring provides comprehensive monitoring and metrics collection for the telemetry pipeline.
//
// This package implements detailed metrics collection including:
//   - Performance metrics (throughput, latency, error rates)
//   - Resource utilization metrics (CPU, memory, disk, network)
//   - Business metrics (message counts, processing rates)
//   - Health indicators and alerting thresholds
//   - Custom metrics with labels and dimensions
package monitoring

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dilipmighty245/telemetry-pipeline/pkg/logging"
)

// MetricType represents the type of metric being collected
type MetricType string

const (
	MetricTypeCounter   MetricType = "counter"
	MetricTypeGauge     MetricType = "gauge"
	MetricTypeHistogram MetricType = "histogram"
	MetricTypeSummary   MetricType = "summary"
)

// Metric represents a single metric with its metadata
type Metric struct {
	Name        string                 `json:"name"`
	Type        MetricType             `json:"type"`
	Value       float64                `json:"value"`
	Labels      map[string]string      `json:"labels"`
	Description string                 `json:"description"`
	Unit        string                 `json:"unit"`
	Timestamp   time.Time              `json:"timestamp"`
	Metadata    map[string]interface{} `json:"metadata"`
}

// Counter represents a monotonically increasing counter metric
type Counter struct {
	name        string
	description string
	labels      map[string]string
	value       int64
	mu          sync.RWMutex
}

// NewCounter creates a new counter metric
func NewCounter(name, description string, labels map[string]string) *Counter {
	return &Counter{
		name:        name,
		description: description,
		labels:      copyLabels(labels),
	}
}

// Inc increments the counter by 1
func (c *Counter) Inc() {
	c.Add(1)
}

// Add adds the given value to the counter
func (c *Counter) Add(value float64) {
	if value < 0 {
		logging.Warnf("Counter %s: negative value %f ignored", c.name, value)
		return
	}
	atomic.AddInt64(&c.value, int64(value))
}

// Get returns the current counter value
func (c *Counter) Get() float64 {
	return float64(atomic.LoadInt64(&c.value))
}

// ToMetric converts the counter to a Metric struct
func (c *Counter) ToMetric() Metric {
	return Metric{
		Name:        c.name,
		Type:        MetricTypeCounter,
		Value:       c.Get(),
		Labels:      copyLabels(c.labels),
		Description: c.description,
		Timestamp:   time.Now(),
	}
}

// Gauge represents a gauge metric that can go up and down
type Gauge struct {
	name        string
	description string
	labels      map[string]string
	value       int64 // Using int64 for atomic operations, will convert to float64
	mu          sync.RWMutex
}

// NewGauge creates a new gauge metric
func NewGauge(name, description string, labels map[string]string) *Gauge {
	return &Gauge{
		name:        name,
		description: description,
		labels:      copyLabels(labels),
	}
}

// Set sets the gauge to the given value
func (g *Gauge) Set(value float64) {
	atomic.StoreInt64(&g.value, int64(value*1000)) // Store as int with 3 decimal precision
}

// Inc increments the gauge by 1
func (g *Gauge) Inc() {
	g.Add(1)
}

// Dec decrements the gauge by 1
func (g *Gauge) Dec() {
	g.Add(-1)
}

// Add adds the given value to the gauge
func (g *Gauge) Add(value float64) {
	atomic.AddInt64(&g.value, int64(value*1000))
}

// Get returns the current gauge value
func (g *Gauge) Get() float64 {
	return float64(atomic.LoadInt64(&g.value)) / 1000.0
}

// ToMetric converts the gauge to a Metric struct
func (g *Gauge) ToMetric() Metric {
	return Metric{
		Name:        g.name,
		Type:        MetricTypeGauge,
		Value:       g.Get(),
		Labels:      copyLabels(g.labels),
		Description: g.description,
		Timestamp:   time.Now(),
	}
}

// Histogram represents a histogram metric for measuring distributions
type Histogram struct {
	name        string
	description string
	labels      map[string]string
	buckets     []float64
	counts      []int64
	sum         int64
	count       int64
	mu          sync.RWMutex
}

// NewHistogram creates a new histogram metric with default buckets
func NewHistogram(name, description string, labels map[string]string) *Histogram {
	// Default buckets for latency measurements (in milliseconds)
	buckets := []float64{1, 5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000, 10000}
	return NewHistogramWithBuckets(name, description, labels, buckets)
}

// NewHistogramWithBuckets creates a new histogram metric with custom buckets
func NewHistogramWithBuckets(name, description string, labels map[string]string, buckets []float64) *Histogram {
	return &Histogram{
		name:        name,
		description: description,
		labels:      copyLabels(labels),
		buckets:     append([]float64{}, buckets...), // Copy buckets
		counts:      make([]int64, len(buckets)+1),   // +1 for +Inf bucket
	}
}

// Observe adds an observation to the histogram
func (h *Histogram) Observe(value float64) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Update sum and count
	atomic.AddInt64(&h.sum, int64(value*1000)) // Store with 3 decimal precision
	atomic.AddInt64(&h.count, 1)

	// Find the appropriate bucket
	bucketIndex := len(h.buckets) // Default to +Inf bucket
	for i, bucket := range h.buckets {
		if value <= bucket {
			bucketIndex = i
			break
		}
	}

	// Increment all buckets from the found index to the end
	for i := bucketIndex; i < len(h.counts); i++ {
		atomic.AddInt64(&h.counts[i], 1)
	}
}

// GetCount returns the total number of observations
func (h *Histogram) GetCount() int64 {
	return atomic.LoadInt64(&h.count)
}

// GetSum returns the sum of all observations
func (h *Histogram) GetSum() float64 {
	return float64(atomic.LoadInt64(&h.sum)) / 1000.0
}

// GetBuckets returns the histogram buckets and their counts
func (h *Histogram) GetBuckets() map[string]int64 {
	h.mu.RLock()
	defer h.mu.RUnlock()

	buckets := make(map[string]int64)
	for i, bucket := range h.buckets {
		buckets[fmt.Sprintf("%.3f", bucket)] = atomic.LoadInt64(&h.counts[i])
	}
	buckets["+Inf"] = atomic.LoadInt64(&h.counts[len(h.counts)-1])
	return buckets
}

// ToMetric converts the histogram to a Metric struct
func (h *Histogram) ToMetric() Metric {
	return Metric{
		Name:        h.name,
		Type:        MetricTypeHistogram,
		Value:       float64(h.GetCount()),
		Labels:      copyLabels(h.labels),
		Description: h.description,
		Timestamp:   time.Now(),
		Metadata: map[string]interface{}{
			"count":   h.GetCount(),
			"sum":     h.GetSum(),
			"buckets": h.GetBuckets(),
		},
	}
}

// MetricsRegistry manages all metrics for the application
type MetricsRegistry struct {
	counters   map[string]*Counter
	gauges     map[string]*Gauge
	histograms map[string]*Histogram
	mu         sync.RWMutex
	config     *MetricsConfig
}

// MetricsConfig holds configuration for metrics collection
type MetricsConfig struct {
	EnableSystemMetrics bool          `json:"enable_system_metrics"`
	CollectionInterval  time.Duration `json:"collection_interval"`
	RetentionPeriod     time.Duration `json:"retention_period"`
	ExportInterval      time.Duration `json:"export_interval"`
	Labels              map[string]string `json:"labels"`
}

// DefaultMetricsConfig returns a default metrics configuration
func DefaultMetricsConfig() *MetricsConfig {
	return &MetricsConfig{
		EnableSystemMetrics: true,
		CollectionInterval:  10 * time.Second,
		RetentionPeriod:     24 * time.Hour,
		ExportInterval:      60 * time.Second,
		Labels: map[string]string{
			"service": "telemetry-pipeline",
			"version": "v1.0.0",
		},
	}
}

// NewMetricsRegistry creates a new metrics registry
func NewMetricsRegistry(config *MetricsConfig) *MetricsRegistry {
	if config == nil {
		config = DefaultMetricsConfig()
	}

	registry := &MetricsRegistry{
		counters:   make(map[string]*Counter),
		gauges:     make(map[string]*Gauge),
		histograms: make(map[string]*Histogram),
		config:     config,
	}

	// Start system metrics collection if enabled
	if config.EnableSystemMetrics {
		go registry.collectSystemMetrics()
	}

	return registry
}

// GetOrCreateCounter gets an existing counter or creates a new one
func (mr *MetricsRegistry) GetOrCreateCounter(name, description string, labels map[string]string) *Counter {
	key := mr.buildKey(name, labels)
	
	mr.mu.RLock()
	if counter, exists := mr.counters[key]; exists {
		mr.mu.RUnlock()
		return counter
	}
	mr.mu.RUnlock()

	mr.mu.Lock()
	defer mr.mu.Unlock()
	
	// Double-check after acquiring write lock
	if counter, exists := mr.counters[key]; exists {
		return counter
	}

	// Merge global labels with metric labels
	mergedLabels := make(map[string]string)
	for k, v := range mr.config.Labels {
		mergedLabels[k] = v
	}
	for k, v := range labels {
		mergedLabels[k] = v
	}

	counter := NewCounter(name, description, mergedLabels)
	mr.counters[key] = counter
	return counter
}

// GetOrCreateGauge gets an existing gauge or creates a new one
func (mr *MetricsRegistry) GetOrCreateGauge(name, description string, labels map[string]string) *Gauge {
	key := mr.buildKey(name, labels)
	
	mr.mu.RLock()
	if gauge, exists := mr.gauges[key]; exists {
		mr.mu.RUnlock()
		return gauge
	}
	mr.mu.RUnlock()

	mr.mu.Lock()
	defer mr.mu.Unlock()
	
	// Double-check after acquiring write lock
	if gauge, exists := mr.gauges[key]; exists {
		return gauge
	}

	// Merge global labels with metric labels
	mergedLabels := make(map[string]string)
	for k, v := range mr.config.Labels {
		mergedLabels[k] = v
	}
	for k, v := range labels {
		mergedLabels[k] = v
	}

	gauge := NewGauge(name, description, mergedLabels)
	mr.gauges[key] = gauge
	return gauge
}

// GetOrCreateHistogram gets an existing histogram or creates a new one
func (mr *MetricsRegistry) GetOrCreateHistogram(name, description string, labels map[string]string) *Histogram {
	key := mr.buildKey(name, labels)
	
	mr.mu.RLock()
	if histogram, exists := mr.histograms[key]; exists {
		mr.mu.RUnlock()
		return histogram
	}
	mr.mu.RUnlock()

	mr.mu.Lock()
	defer mr.mu.Unlock()
	
	// Double-check after acquiring write lock
	if histogram, exists := mr.histograms[key]; exists {
		return histogram
	}

	// Merge global labels with metric labels
	mergedLabels := make(map[string]string)
	for k, v := range mr.config.Labels {
		mergedLabels[k] = v
	}
	for k, v := range labels {
		mergedLabels[k] = v
	}

	histogram := NewHistogram(name, description, mergedLabels)
	mr.histograms[key] = histogram
	return histogram
}

// GetAllMetrics returns all metrics in the registry
func (mr *MetricsRegistry) GetAllMetrics() []Metric {
	mr.mu.RLock()
	defer mr.mu.RUnlock()

	var metrics []Metric

	for _, counter := range mr.counters {
		metrics = append(metrics, counter.ToMetric())
	}

	for _, gauge := range mr.gauges {
		metrics = append(metrics, gauge.ToMetric())
	}

	for _, histogram := range mr.histograms {
		metrics = append(metrics, histogram.ToMetric())
	}

	return metrics
}

// GetMetricsByType returns all metrics of a specific type
func (mr *MetricsRegistry) GetMetricsByType(metricType MetricType) []Metric {
	mr.mu.RLock()
	defer mr.mu.RUnlock()

	var metrics []Metric

	switch metricType {
	case MetricTypeCounter:
		for _, counter := range mr.counters {
			metrics = append(metrics, counter.ToMetric())
		}
	case MetricTypeGauge:
		for _, gauge := range mr.gauges {
			metrics = append(metrics, gauge.ToMetric())
		}
	case MetricTypeHistogram:
		for _, histogram := range mr.histograms {
			metrics = append(metrics, histogram.ToMetric())
		}
	}

	return metrics
}

// buildKey creates a unique key for a metric based on name and labels
func (mr *MetricsRegistry) buildKey(name string, labels map[string]string) string {
	key := name
	if len(labels) > 0 {
		key += "{"
		first := true
		for k, v := range labels {
			if !first {
				key += ","
			}
			key += fmt.Sprintf("%s=%s", k, v)
			first = false
		}
		key += "}"
	}
	return key
}

// collectSystemMetrics collects system-level metrics periodically
func (mr *MetricsRegistry) collectSystemMetrics() {
	ticker := time.NewTicker(mr.config.CollectionInterval)
	defer ticker.Stop()

	for range ticker.C {
		mr.updateSystemMetrics()
	}
}

// updateSystemMetrics updates system-level metrics
func (mr *MetricsRegistry) updateSystemMetrics() {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	// Memory metrics
	mr.GetOrCreateGauge("system_memory_alloc_bytes", "Allocated memory in bytes", nil).Set(float64(memStats.Alloc))
	mr.GetOrCreateGauge("system_memory_sys_bytes", "System memory in bytes", nil).Set(float64(memStats.Sys))
	mr.GetOrCreateGauge("system_memory_heap_alloc_bytes", "Heap allocated memory in bytes", nil).Set(float64(memStats.HeapAlloc))
	mr.GetOrCreateGauge("system_memory_heap_sys_bytes", "Heap system memory in bytes", nil).Set(float64(memStats.HeapSys))
	mr.GetOrCreateGauge("system_memory_heap_idle_bytes", "Heap idle memory in bytes", nil).Set(float64(memStats.HeapIdle))
	mr.GetOrCreateGauge("system_memory_heap_inuse_bytes", "Heap in-use memory in bytes", nil).Set(float64(memStats.HeapInuse))

	// GC metrics
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	mr.GetOrCreateCounter("system_gc_total", "Total number of GC cycles", nil).Add(float64(stats.NumGC))
	mr.GetOrCreateGauge("system_gc_pause_ns", "GC pause time in nanoseconds", nil).Set(float64(memStats.PauseTotalNs))

	// Goroutine metrics
	mr.GetOrCreateGauge("system_goroutines_total", "Total number of goroutines", nil).Set(float64(runtime.NumGoroutine()))
	mr.GetOrCreateGauge("system_threads_total", "Total number of OS threads", nil).Set(float64(runtime.GOMAXPROCS(0)))
}

// TelemetryMetrics provides pre-defined metrics for the telemetry pipeline
type TelemetryMetrics struct {
	registry *MetricsRegistry

	// Message processing metrics
	MessagesReceived    *Counter
	MessagesProcessed   *Counter
	MessagesFailed      *Counter
	ProcessingLatency   *Histogram
	QueueDepth          *Gauge

	// CSV processing metrics
	CSVFilesProcessed   *Counter
	CSVRecordsProcessed *Counter
	CSVParsingErrors    *Counter
	CSVValidationErrors *Counter

	// etcd metrics
	EtcdOperations      *Counter
	EtcdLatency         *Histogram
	EtcdErrors          *Counter
	EtcdConnections     *Gauge

	// Component health metrics
	ComponentHealth     *Gauge
	ComponentUptime     *Counter
	ComponentErrors     *Counter

	// Resource utilization
	CPUUsage           *Gauge
	MemoryUsage        *Gauge
	DiskUsage          *Gauge
	NetworkBytesIn     *Counter
	NetworkBytesOut    *Counter
}

// NewTelemetryMetrics creates a new set of telemetry-specific metrics
func NewTelemetryMetrics(registry *MetricsRegistry, component string) *TelemetryMetrics {
	labels := map[string]string{"component": component}

	return &TelemetryMetrics{
		registry: registry,

		// Message processing metrics
		MessagesReceived:    registry.GetOrCreateCounter("messages_received_total", "Total messages received", labels),
		MessagesProcessed:   registry.GetOrCreateCounter("messages_processed_total", "Total messages processed successfully", labels),
		MessagesFailed:      registry.GetOrCreateCounter("messages_failed_total", "Total messages that failed processing", labels),
		ProcessingLatency:   registry.GetOrCreateHistogram("message_processing_duration_ms", "Message processing latency in milliseconds", labels),
		QueueDepth:          registry.GetOrCreateGauge("queue_depth", "Current queue depth", labels),

		// CSV processing metrics
		CSVFilesProcessed:   registry.GetOrCreateCounter("csv_files_processed_total", "Total CSV files processed", labels),
		CSVRecordsProcessed: registry.GetOrCreateCounter("csv_records_processed_total", "Total CSV records processed", labels),
		CSVParsingErrors:    registry.GetOrCreateCounter("csv_parsing_errors_total", "Total CSV parsing errors", labels),
		CSVValidationErrors: registry.GetOrCreateCounter("csv_validation_errors_total", "Total CSV validation errors", labels),

		// etcd metrics
		EtcdOperations:      registry.GetOrCreateCounter("etcd_operations_total", "Total etcd operations", labels),
		EtcdLatency:         registry.GetOrCreateHistogram("etcd_operation_duration_ms", "etcd operation latency in milliseconds", labels),
		EtcdErrors:          registry.GetOrCreateCounter("etcd_errors_total", "Total etcd errors", labels),
		EtcdConnections:     registry.GetOrCreateGauge("etcd_connections", "Number of active etcd connections", labels),

		// Component health metrics
		ComponentHealth:     registry.GetOrCreateGauge("component_health", "Component health status (1=healthy, 0=unhealthy)", labels),
		ComponentUptime:     registry.GetOrCreateCounter("component_uptime_seconds", "Component uptime in seconds", labels),
		ComponentErrors:     registry.GetOrCreateCounter("component_errors_total", "Total component errors", labels),

		// Resource utilization
		CPUUsage:           registry.GetOrCreateGauge("cpu_usage_percent", "CPU usage percentage", labels),
		MemoryUsage:        registry.GetOrCreateGauge("memory_usage_bytes", "Memory usage in bytes", labels),
		DiskUsage:          registry.GetOrCreateGauge("disk_usage_bytes", "Disk usage in bytes", labels),
		NetworkBytesIn:     registry.GetOrCreateCounter("network_bytes_received_total", "Total network bytes received", labels),
		NetworkBytesOut:    registry.GetOrCreateCounter("network_bytes_sent_total", "Total network bytes sent", labels),
	}
}

// RecordMessageProcessing records metrics for message processing
func (tm *TelemetryMetrics) RecordMessageProcessing(success bool, latencyMs float64) {
	tm.MessagesReceived.Inc()
	tm.ProcessingLatency.Observe(latencyMs)
	
	if success {
		tm.MessagesProcessed.Inc()
	} else {
		tm.MessagesFailed.Inc()
	}
}

// RecordCSVProcessing records metrics for CSV processing
func (tm *TelemetryMetrics) RecordCSVProcessing(records int, parseErrors int, validationErrors int) {
	tm.CSVFilesProcessed.Inc()
	tm.CSVRecordsProcessed.Add(float64(records))
	tm.CSVParsingErrors.Add(float64(parseErrors))
	tm.CSVValidationErrors.Add(float64(validationErrors))
}

// RecordEtcdOperation records metrics for etcd operations
func (tm *TelemetryMetrics) RecordEtcdOperation(operation string, success bool, latencyMs float64) {
	labels := map[string]string{"operation": operation}
	tm.registry.GetOrCreateCounter("etcd_operations_total", "Total etcd operations", labels).Inc()
	tm.registry.GetOrCreateHistogram("etcd_operation_duration_ms", "etcd operation latency", labels).Observe(latencyMs)
	
	if !success {
		tm.registry.GetOrCreateCounter("etcd_errors_total", "Total etcd errors", labels).Inc()
	}
}

// SetComponentHealth sets the component health status
func (tm *TelemetryMetrics) SetComponentHealth(healthy bool) {
	if healthy {
		tm.ComponentHealth.Set(1)
	} else {
		tm.ComponentHealth.Set(0)
	}
}

// copyLabels creates a copy of the labels map
func copyLabels(labels map[string]string) map[string]string {
	if labels == nil {
		return make(map[string]string)
	}
	
	copy := make(map[string]string, len(labels))
	for k, v := range labels {
		copy[k] = v
	}
	return copy
}

// MetricsExporter handles exporting metrics to external systems
type MetricsExporter struct {
	registry *MetricsRegistry
	config   *ExporterConfig
	ctx      context.Context
	cancel   context.CancelFunc
}

// ExporterConfig holds configuration for metrics export
type ExporterConfig struct {
	Enabled      bool          `json:"enabled"`
	Endpoint     string        `json:"endpoint"`
	Interval     time.Duration `json:"interval"`
	Format       string        `json:"format"` // "prometheus", "json", "influx"
	BatchSize    int           `json:"batch_size"`
	Timeout      time.Duration `json:"timeout"`
	Headers      map[string]string `json:"headers"`
}

// NewMetricsExporter creates a new metrics exporter
func NewMetricsExporter(registry *MetricsRegistry, config *ExporterConfig) *MetricsExporter {
	ctx, cancel := context.WithCancel(context.Background())
	
	exporter := &MetricsExporter{
		registry: registry,
		config:   config,
		ctx:      ctx,
		cancel:   cancel,
	}

	if config.Enabled {
		go exporter.exportLoop()
	}

	return exporter
}

// exportLoop runs the metrics export loop
func (me *MetricsExporter) exportLoop() {
	ticker := time.NewTicker(me.config.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-me.ctx.Done():
			return
		case <-ticker.C:
			if err := me.exportMetrics(); err != nil {
				logging.Errorf("Failed to export metrics: %v", err)
			}
		}
	}
}

// exportMetrics exports all metrics to the configured endpoint
func (me *MetricsExporter) exportMetrics() error {
	metrics := me.registry.GetAllMetrics()
	
	switch me.config.Format {
	case "json":
		return me.exportJSON(metrics)
	case "prometheus":
		return me.exportPrometheus(metrics)
	default:
		logging.Debugf("Exported %d metrics (format: %s)", len(metrics), me.config.Format)
		return nil
	}
}

// exportJSON exports metrics in JSON format
func (me *MetricsExporter) exportJSON(metrics []Metric) error {
	logging.Debugf("Exporting %d metrics in JSON format", len(metrics))
	// Implementation would send JSON to configured endpoint
	return nil
}

// exportPrometheus exports metrics in Prometheus format
func (me *MetricsExporter) exportPrometheus(metrics []Metric) error {
	logging.Debugf("Exporting %d metrics in Prometheus format", len(metrics))
	// Implementation would send Prometheus format to configured endpoint
	return nil
}

// Stop stops the metrics exporter
func (me *MetricsExporter) Stop() {
	me.cancel()
}
