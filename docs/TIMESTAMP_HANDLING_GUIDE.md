# Timestamp Handling Guide

## Overview

This guide covers timestamp handling in the telemetry pipeline, including storage in etcd, processing timestamps, and demo data generation.

## Current Timestamp Usage

### 1. Processing Timestamps with `time.Now()`

When storing telemetry data in etcd, the system uses `time.Now()` to capture the processing timestamp:

```go
// In etcd_backend.go - Message key generation
messageKey := fmt.Sprintf("%s/%s/%d_%s_%d",
    eb.queuePrefix,
    topic,
    time.Now().UnixNano(),  // Nanosecond precision for uniqueness
    message.ID,
    time.Now().Unix())      // Unix timestamp for readability
```

### 2. Telemetry Data Timestamps

The system handles two types of timestamps:

#### Original Timestamp (from CSV)
- Parsed from CSV `timestamp` column
- Format: RFC3339 (`2025-01-18T10:00:00Z`)
- Falls back to `time.Now()` if invalid or missing

#### Processing Timestamp
- When data is processed: `time.Now()`
- Used for etcd keys, message ordering, and system tracking

```go
// In csv_reader.go
tr.Timestamp = getColumn("timestamp")
if tr.Timestamp == "" {
    tr.Timestamp = time.Now().Format(time.RFC3339)
}

// Validate and normalize
if parsedTime, err := time.Parse(time.RFC3339, tr.Timestamp); err == nil {
    tr.Timestamp = parsedTime.Format(time.RFC3339)
} else {
    log.Warnf("Invalid timestamp format '%s', using current time", tr.Timestamp)
    tr.Timestamp = time.Now().Format(time.RFC3339)
}
```

## Timestamp Formats Used

### 1. RFC3339 (Primary Format)
```go
timestamp := time.Now().Format(time.RFC3339)
// Output: "2025-01-18T10:30:45Z"
```

### 2. Unix Timestamps
```go
unixTime := time.Now().Unix()        // Seconds since epoch
nanoTime := time.Now().UnixNano()    // Nanoseconds since epoch
```

### 3. Custom Formats
```go
// For IDs and file names
timeID := time.Now().Format("20060102150405")  // "20250118103045"
```

## Demo Timestamp Generation

### 1. Sequential Timestamps for Demo Data

```go
// Generate sequential timestamps for realistic demo data
func GenerateDemoTimestamps(startTime time.Time, count int, intervalSeconds int) []string {
    timestamps := make([]string, count)
    current := startTime
    
    for i := 0; i < count; i++ {
        timestamps[i] = current.Format(time.RFC3339)
        current = current.Add(time.Duration(intervalSeconds) * time.Second)
    }
    
    return timestamps
}

// Example usage:
startTime := time.Now().Add(-1 * time.Hour) // Start 1 hour ago
timestamps := GenerateDemoTimestamps(startTime, 100, 30) // 100 records, 30 sec intervals
```

### 2. Realistic Time Ranges

```go
// Generate timestamps for the last N hours with random intervals
func GenerateRealisticTimestamps(hours int, recordCount int) []string {
    endTime := time.Now()
    startTime := endTime.Add(-time.Duration(hours) * time.Hour)
    duration := endTime.Sub(startTime)
    
    timestamps := make([]string, recordCount)
    for i := 0; i < recordCount; i++ {
        // Random time within the range
        randomOffset := time.Duration(rand.Int63n(int64(duration)))
        timestamp := startTime.Add(randomOffset)
        timestamps[i] = timestamp.Format(time.RFC3339)
    }
    
    // Sort timestamps chronologically
    sort.Strings(timestamps)
    return timestamps
}
```

### 3. Demo CSV Generation with Timestamps

```go
// Generate demo CSV with proper timestamps
func GenerateDemoCSV(filename string, recordCount int) error {
    file, err := os.Create(filename)
    if err != nil {
        return err
    }
    defer file.Close()
    
    writer := csv.NewWriter(file)
    defer writer.Flush()
    
    // Write header
    header := []string{
        "timestamp", "gpu_id", "hostname", "uuid", "device", 
        "modelName", "gpu_utilization", "memory_utilization", 
        "memory_used_mb", "memory_free_mb", "temperature", "power_draw",
        "sm_clock_mhz", "memory_clock_mhz",
    }
    writer.Write(header)
    
    // Generate timestamps starting 1 hour ago, 30-second intervals
    startTime := time.Now().Add(-1 * time.Hour)
    
    for i := 0; i < recordCount; i++ {
        timestamp := startTime.Add(time.Duration(i*30) * time.Second)
        
        record := []string{
            timestamp.Format(time.RFC3339),
            fmt.Sprintf("%d", i%4),                    // gpu_id (0-3)
            fmt.Sprintf("demo-host-%03d", (i/4)+1),    // hostname
            fmt.Sprintf("GPU-%d", i),                   // uuid
            fmt.Sprintf("nvidia%d", i%4),              // device
            "NVIDIA H100 80GB HBM3",                   // modelName
            fmt.Sprintf("%.1f", 70.0+rand.Float64()*30), // gpu_utilization
            fmt.Sprintf("%.1f", 60.0+rand.Float64()*30), // memory_utilization
            fmt.Sprintf("%d", 50000+rand.Intn(30000)),    // memory_used_mb
            fmt.Sprintf("%d", 10000+rand.Intn(20000)),    // memory_free_mb
            fmt.Sprintf("%.1f", 55.0+rand.Float64()*20),  // temperature
            fmt.Sprintf("%.1f", 300.0+rand.Float64()*150), // power_draw
            fmt.Sprintf("%d", 1400+rand.Intn(200)),       // sm_clock_mhz
            fmt.Sprintf("%d", 1200+rand.Intn(100)),       // memory_clock_mhz
        }
        writer.Write(record)
    }
    
    return nil
}
```

## Best Practices

### 1. Always Use UTC
```go
// Good: Always use UTC for consistency
timestamp := time.Now().UTC().Format(time.RFC3339)

// Avoid: Local time zones can cause confusion
timestamp := time.Now().Format(time.RFC3339)
```

### 2. Consistent Format Validation
```go
func ValidateAndNormalizeTimestamp(timestampStr string) (time.Time, error) {
    // Try RFC3339 first (preferred format)
    if t, err := time.Parse(time.RFC3339, timestampStr); err == nil {
        return t.UTC(), nil
    }
    
    // Try alternative format
    if t, err := time.Parse("2006-01-02 15:04:05", timestampStr); err == nil {
        return t.UTC(), nil
    }
    
    return time.Time{}, fmt.Errorf("invalid timestamp format: %s", timestampStr)
}
```

### 3. Etcd Key Ordering
```go
// Use nanosecond precision for unique, ordered keys
func GenerateOrderedKey(topic, messageID string) string {
    now := time.Now()
    return fmt.Sprintf("/telemetry/queue/%s/%d_%s_%d",
        topic,
        now.UnixNano(),  // Ensures chronological ordering
        messageID,
        now.Unix())      // Human-readable component
}
```

## Relative Timestamp Generation

### 1. Relative Time Expressions

The system supports intuitive relative time expressions for demo data generation:

```go
// Examples of relative time expressions
expressions := []string{
    "now",        // Current time
    "now-5s",     // 5 seconds ago
    "now-30s",    // 30 seconds ago
    "now-5m",     // 5 minutes ago
    "now-1h",     // 1 hour ago
    "now-2h",     // 2 hours ago
    "now-1d",     // 1 day ago
    "now+5m",     // 5 minutes from now (future)
}

// Generate timestamps
timestamps, err := utils.GenerateRelativeTimestamps(expressions)
```

### 2. Usage Examples

#### Command Line (Shell Script)
```bash
# Generate specific timestamps
./scripts/generate-demo-timestamps.sh generate now-5m now-1m now

# Generate CSV with timestamp range
./scripts/generate-demo-timestamps.sh csv demo.csv now-2h now 100

# Show examples
./scripts/generate-demo-timestamps.sh examples
```

#### Go Program
```bash
# Show examples
go run cmd/demo-timestamps/main.go examples

# Generate specific timestamps
go run cmd/demo-timestamps/main.go generate now-5m now-1m now

# Generate CSV file
go run cmd/demo-timestamps/main.go csv
```

### 3. Common Demo Patterns

#### Recent Activity (Last 5 Minutes)
```go
recentTimestamps := []string{
    "now-5m", "now-4m", "now-3m", "now-2m", "now-1m", "now",
}
timestamps := utils.DemoTimestampsRelative(recentTimestamps)
```

#### Hourly Snapshots (Last 6 Hours)
```go
hourlyTimestamps := []string{
    "now-6h", "now-5h", "now-4h", "now-3h", "now-2h", "now-1h", "now",
}
timestamps := utils.DemoTimestampsRelative(hourlyTimestamps)
```

#### Daily Snapshots (Last Week)
```go
dailyTimestamps := []string{
    "now-7d", "now-6d", "now-5d", "now-4d", "now-3d", "now-2d", "now-1d", "now",
}
timestamps := utils.DemoTimestampsRelative(dailyTimestamps)
```

## Demo Utilities

### 2. Go Utility Functions

```go
// pkg/utils/timestamp.go
package utils

import (
    "fmt"
    "math/rand"
    "time"
)

// TimestampGenerator provides utilities for generating demo timestamps
type TimestampGenerator struct {
    baseTime time.Time
    interval time.Duration
    jitter   time.Duration
}

// NewTimestampGenerator creates a new timestamp generator
func NewTimestampGenerator(baseTime time.Time, interval, jitter time.Duration) *TimestampGenerator {
    return &TimestampGenerator{
        baseTime: baseTime,
        interval: interval,
        jitter:   jitter,
    }
}

// GenerateSequential generates sequential timestamps
func (tg *TimestampGenerator) GenerateSequential(count int) []string {
    timestamps := make([]string, count)
    current := tg.baseTime
    
    for i := 0; i < count; i++ {
        timestamps[i] = current.Format(time.RFC3339)
        current = current.Add(tg.interval)
    }
    
    return timestamps
}

// GenerateWithJitter generates timestamps with random jitter
func (tg *TimestampGenerator) GenerateWithJitter(count int) []string {
    timestamps := make([]string, count)
    current := tg.baseTime
    
    for i := 0; i < count; i++ {
        // Add random jitter
        jitterOffset := time.Duration(rand.Int63n(int64(tg.jitter*2))) - tg.jitter
        timestamp := current.Add(jitterOffset)
        timestamps[i] = timestamp.Format(time.RFC3339)
        current = current.Add(tg.interval)
    }
    
    return timestamps
}

// Common timestamp generators for demos
func DemoTimestampsLastHour(count int) []string {
    baseTime := time.Now().UTC().Add(-1 * time.Hour)
    generator := NewTimestampGenerator(baseTime, 30*time.Second, 5*time.Second)
    return generator.GenerateWithJitter(count)
}

func DemoTimestampsLastDay(count int) []string {
    baseTime := time.Now().UTC().Add(-24 * time.Hour)
    interval := 24 * time.Hour / time.Duration(count)
    generator := NewTimestampGenerator(baseTime, interval, interval/10)
    return generator.GenerateWithJitter(count)
}
```

## Integration Examples

### 1. Enhanced Demo Script

```bash
#!/bin/bash
# Enhanced demo script with dynamic timestamps

generate_demo_csv_with_timestamps() {
    local filename="$1"
    local record_count="${2:-100}"
    
    # Generate header
    echo "timestamp,gpu_id,hostname,uuid,device,modelName,gpu_utilization,memory_utilization,memory_used_mb,memory_free_mb,temperature,power_draw,sm_clock_mhz,memory_clock_mhz" > "$filename"
    
    # Generate records with realistic timestamps
    python3 -c "
import datetime
import random
import sys

record_count = int(sys.argv[1])
start_time = datetime.datetime.now(datetime.timezone.utc) - datetime.timedelta(hours=1)

for i in range(record_count):
    timestamp = start_time + datetime.timedelta(seconds=i * 30)
    gpu_id = i % 4
    hostname = f'demo-host-{(i//4)+1:03d}'
    uuid = f'GPU-{i:08d}-1234-5678-9abc-{i:012d}'
    device = f'nvidia{gpu_id}'
    model = 'NVIDIA H100 80GB HBM3'
    gpu_util = round(70 + random.random() * 30, 1)
    mem_util = round(60 + random.random() * 30, 1)
    mem_used = 50000 + random.randint(0, 30000)
    mem_free = 10000 + random.randint(0, 20000)
    temp = round(55 + random.random() * 20, 1)
    power = round(300 + random.random() * 150, 1)
    sm_clock = 1400 + random.randint(0, 200)
    mem_clock = 1200 + random.randint(0, 100)
    
    print(f'{timestamp.strftime(\"%Y-%m-%dT%H:%M:%SZ\")},{gpu_id},{hostname},{uuid},{device},{model},{gpu_util},{mem_util},{mem_used},{mem_free},{temp},{power},{sm_clock},{mem_clock}')
" $record_count >> "$filename"
    
    echo "Generated $filename with $record_count records"
}

# Usage
generate_demo_csv_with_timestamps "demo_data.csv" 500
```

## Summary

1. **Processing Timestamps**: Use `time.Now()` when storing in etcd for processing time tracking
2. **Data Timestamps**: Preserve original timestamps from CSV, fall back to `time.Now()` if invalid
3. **Format Consistency**: Always use RFC3339 format for interchange
4. **Demo Generation**: Use sequential timestamps with realistic intervals for demonstrations
5. **Etcd Keys**: Include nanosecond timestamps for uniqueness and ordering
6. **UTC Only**: Always use UTC to avoid timezone confusion

This approach ensures consistent timestamp handling across the telemetry pipeline while providing flexible tools for demo data generation.
