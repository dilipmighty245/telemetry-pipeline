package monitoring

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSimpleMetricsRegistry(t *testing.T) {
	registry := NewMetricsRegistry(nil)
	require.NotNil(t, registry)
}

func TestSimpleCounter(t *testing.T) {
	counter := NewCounter("test_counter", "Test counter", nil)
	require.NotNil(t, counter)
	
	// Initial value should be 0
	assert.Equal(t, float64(0), counter.Get())
	
	// Test increment
	counter.Inc()
	assert.Equal(t, float64(1), counter.Get())
	
	// Test add
	counter.Add(5)
	assert.Equal(t, float64(6), counter.Get())
}

func TestSimpleGauge(t *testing.T) {
	gauge := NewGauge("test_gauge", "Test gauge", nil)
	require.NotNil(t, gauge)
	
	// Initial value should be 0
	assert.Equal(t, float64(0), gauge.Get())
	
	// Test set
	gauge.Set(42.5)
	assert.Equal(t, 42.5, gauge.Get())
	
	// Test inc
	gauge.Inc()
	assert.Equal(t, 43.5, gauge.Get())
	
	// Test dec
	gauge.Dec()
	assert.Equal(t, 42.5, gauge.Get())
	
	// Test add
	gauge.Add(7.5)
	assert.Equal(t, 50.0, gauge.Get())
}

func TestMetricToMetric(t *testing.T) {
	counter := NewCounter("test_counter", "Test counter", map[string]string{"env": "test"})
	counter.Add(5)
	
	metric := counter.ToMetric()
	assert.Equal(t, "test_counter", metric.Name)
	assert.Equal(t, MetricTypeCounter, metric.Type)
	assert.Equal(t, float64(5), metric.Value)
	assert.Equal(t, "test", metric.Labels["env"])
}

func TestCounterNegativeValue(t *testing.T) {
	counter := NewCounter("test_counter", "Test counter", nil)
	
	// Adding negative value should be ignored
	counter.Add(-5)
	assert.Equal(t, float64(0), counter.Get())
	
	// Add positive value should work
	counter.Add(10)
	assert.Equal(t, float64(10), counter.Get())
}

func TestHistogram(t *testing.T) {
	hist := NewHistogram("test_histogram", "Test histogram", map[string]string{"type": "latency"})
	require.NotNil(t, hist)
	
	// Test observations
	hist.Observe(5.0)
	hist.Observe(15.0)
	hist.Observe(150.0)
	
	assert.Equal(t, int64(3), hist.GetCount())
	assert.Equal(t, 170.0, hist.GetSum())
	
	// Test buckets
	buckets := hist.GetBuckets()
	assert.Contains(t, buckets, "10.000")
	assert.Contains(t, buckets, "+Inf")
	
	// Test ToMetric
	metric := hist.ToMetric()
	assert.Equal(t, "test_histogram", metric.Name)
	assert.Equal(t, MetricTypeHistogram, metric.Type)
	assert.Equal(t, float64(3), metric.Value)
	assert.Equal(t, "latency", metric.Labels["type"])
	assert.Contains(t, metric.Metadata, "count")
	assert.Contains(t, metric.Metadata, "sum")
	assert.Contains(t, metric.Metadata, "buckets")
}

func TestHistogramWithCustomBuckets(t *testing.T) {
	buckets := []float64{1, 5, 10}
	hist := NewHistogramWithBuckets("custom_hist", "Custom histogram", nil, buckets)
	require.NotNil(t, hist)
	
	hist.Observe(3.0)
	hist.Observe(7.0)
	
	assert.Equal(t, int64(2), hist.GetCount())
	assert.Equal(t, 10.0, hist.GetSum())
}

func TestMetricsRegistry(t *testing.T) {
	config := &MetricsConfig{
		EnableSystemMetrics: false,
		Labels: map[string]string{"service": "test"},
	}
	registry := NewMetricsRegistry(config)
	require.NotNil(t, registry)
	
	// Test counter creation and retrieval
	counter1 := registry.GetOrCreateCounter("test_counter", "Test counter", map[string]string{"env": "prod"})
	counter2 := registry.GetOrCreateCounter("test_counter", "Test counter", map[string]string{"env": "prod"})
	assert.Same(t, counter1, counter2) // Should return same instance
	
	// Test gauge creation
	gauge := registry.GetOrCreateGauge("test_gauge", "Test gauge", map[string]string{"type": "memory"})
	require.NotNil(t, gauge)
	
	// Test histogram creation
	hist := registry.GetOrCreateHistogram("test_histogram", "Test histogram", nil)
	require.NotNil(t, hist)
	
	// Test getting all metrics
	counter1.Inc()
	gauge.Set(42.0)
	hist.Observe(100.0)
	
	metrics := registry.GetAllMetrics()
	assert.Len(t, metrics, 3)
	
	// Test getting metrics by type
	counterMetrics := registry.GetMetricsByType(MetricTypeCounter)
	assert.Len(t, counterMetrics, 1)
	
	gaugeMetrics := registry.GetMetricsByType(MetricTypeGauge)
	assert.Len(t, gaugeMetrics, 1)
	
	histogramMetrics := registry.GetMetricsByType(MetricTypeHistogram)
	assert.Len(t, histogramMetrics, 1)
}

func TestDefaultMetricsConfig(t *testing.T) {
	config := DefaultMetricsConfig()
	require.NotNil(t, config)
	
	assert.True(t, config.EnableSystemMetrics)
	assert.Equal(t, 10*time.Second, config.CollectionInterval)
	assert.Equal(t, 24*time.Hour, config.RetentionPeriod)
	assert.Equal(t, 60*time.Second, config.ExportInterval)
	assert.Equal(t, "telemetry-pipeline", config.Labels["service"])
}

func TestTelemetryMetrics(t *testing.T) {
	registry := NewMetricsRegistry(nil)
	tm := NewTelemetryMetrics(registry, "test-component")
	require.NotNil(t, tm)
	
	// Test message processing recording
	tm.RecordMessageProcessing(true, 50.0)
	assert.Equal(t, float64(1), tm.MessagesReceived.Get())
	assert.Equal(t, float64(1), tm.MessagesProcessed.Get())
	assert.Equal(t, float64(0), tm.MessagesFailed.Get())
	
	tm.RecordMessageProcessing(false, 75.0)
	assert.Equal(t, float64(2), tm.MessagesReceived.Get())
	assert.Equal(t, float64(1), tm.MessagesProcessed.Get())
	assert.Equal(t, float64(1), tm.MessagesFailed.Get())
	
	// Test CSV processing recording
	tm.RecordCSVProcessing(100, 2, 1)
	assert.Equal(t, float64(1), tm.CSVFilesProcessed.Get())
	assert.Equal(t, float64(100), tm.CSVRecordsProcessed.Get())
	assert.Equal(t, float64(2), tm.CSVParsingErrors.Get())
	assert.Equal(t, float64(1), tm.CSVValidationErrors.Get())
	
	// Test etcd operation recording
	tm.RecordEtcdOperation("get", true, 25.0)
	tm.RecordEtcdOperation("put", false, 50.0)
	
	// Test component health
	tm.SetComponentHealth(true)
	assert.Equal(t, float64(1), tm.ComponentHealth.Get())
	
	tm.SetComponentHealth(false)
	assert.Equal(t, float64(0), tm.ComponentHealth.Get())
}

func TestMetricsExporter(t *testing.T) {
	registry := NewMetricsRegistry(nil)
	config := &ExporterConfig{
		Enabled:   false, // Disabled for test
		Endpoint:  "http://localhost:9090",
		Interval:  time.Second,
		Format:    "json",
		BatchSize: 100,
		Timeout:   5 * time.Second,
	}
	
	exporter := NewMetricsExporter(registry, config)
	require.NotNil(t, exporter)
	
	// Test export methods
	metrics := []Metric{
		{Name: "test_metric", Type: MetricTypeCounter, Value: 1.0},
	}
	
	err := exporter.exportJSON(metrics)
	assert.NoError(t, err)
	
	err = exporter.exportPrometheus(metrics)
	assert.NoError(t, err)
	
	err = exporter.exportMetrics()
	assert.NoError(t, err)
	
	exporter.Stop()
}

func TestCopyLabels(t *testing.T) {
	// Test nil labels
	result := copyLabels(nil)
	assert.NotNil(t, result)
	assert.Len(t, result, 0)
	
	// Test copying labels
	original := map[string]string{"key1": "value1", "key2": "value2"}
	copied := copyLabels(original)
	assert.Equal(t, original, copied)
	
	// Ensure it's a copy, not the same reference
	copied["key3"] = "value3"
	assert.NotContains(t, original, "key3")
}
