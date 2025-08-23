# Relative Timestamps Quick Reference

## 📋 Quick Reference Card

### Format: `now[+/-]<number><unit>`

| Expression | Description | Example Output |
|------------|-------------|----------------|
| `now` | Current time | `2025-08-23T07:37:57Z` |
| `now-5s` | 5 seconds ago | `2025-08-23T07:37:52Z` |
| `now-30s` | 30 seconds ago | `2025-08-23T07:37:27Z` |
| `now-1m` | 1 minute ago | `2025-08-23T07:36:57Z` |
| `now-5m` | 5 minutes ago | `2025-08-23T07:32:57Z` |
| `now-15m` | 15 minutes ago | `2025-08-23T07:22:57Z` |
| `now-30m` | 30 minutes ago | `2025-08-23T07:07:57Z` |
| `now-1h` | 1 hour ago | `2025-08-23T06:37:57Z` |
| `now-2h` | 2 hours ago | `2025-08-23T05:37:57Z` |
| `now-6h` | 6 hours ago | `2025-08-23T01:37:57Z` |
| `now-12h` | 12 hours ago | `2025-08-22T19:37:57Z` |
| `now-1d` | 1 day ago | `2025-08-22T07:37:57Z` |
| `now-2d` | 2 days ago | `2025-08-21T07:37:57Z` |
| `now-7d` | 1 week ago | `2025-08-16T07:37:57Z` |
| `now+5m` | 5 minutes from now | `2025-08-23T07:42:57Z` |
| `now+1h` | 1 hour from now | `2025-08-23T08:37:57Z` |

### Units
- `s` = seconds
- `m` = minutes  
- `h` = hours
- `d` = days

## 🚀 Usage Examples

### Command Line
```bash
# Show examples
./scripts/generate-demo-timestamps.sh examples

# Generate specific timestamps
./scripts/generate-demo-timestamps.sh generate now-5m now-1m now

# Generate CSV with timestamp range (last 2 hours, 100 records)
./scripts/generate-demo-timestamps.sh csv demo.csv now-2h now 100
```

### Go Code
```go
// Single timestamp
timestamp, err := utils.RelativeTimeString("now-5m")

// Multiple timestamps
expressions := []string{"now-1h", "now-30m", "now-15m", "now"}
timestamps, err := utils.GenerateRelativeTimestamps(expressions)

// Time range (50 points from 1 hour ago to now)
timestamps, err := utils.GenerateTimeRange("now-1h", "now", 50)

// Quick references
timestamp, err := utils.GetQuickTimestamp("5m_ago")
```

### Demo Program
```bash
# Show all examples
go run cmd/demo-timestamps/main.go examples

# Generate specific timestamps
go run cmd/demo-timestamps/main.go generate now-5m now-1m now

# Generate CSV file
go run cmd/demo-timestamps/main.go csv

# Show quick references
go run cmd/demo-timestamps/main.go quick
```

## 🎯 Common Demo Patterns

### Recent Activity (5-minute window)
```
now-5m, now-4m, now-3m, now-2m, now-1m, now
```

### Hourly Snapshots (6 hours)
```
now-6h, now-5h, now-4h, now-3h, now-2h, now-1h, now
```

### Daily Snapshots (1 week)
```
now-7d, now-6d, now-5d, now-4d, now-3d, now-2d, now-1d, now
```

### High-Frequency Monitoring (30 seconds)
```
now-5m, now-4m30s, now-4m, now-3m30s, now-3m, now-2m30s, now-2m, now-1m30s, now-1m, now-30s, now
```

## 🔧 Integration with Telemetry Pipeline

### 1. Processing Timestamps
When storing in etcd, the system uses `time.Now()` for processing timestamps:
```go
messageKey := fmt.Sprintf("%s/%s/%d_%s_%d",
    eb.queuePrefix, topic,
    time.Now().UnixNano(),  // Processing time (nanoseconds)
    message.ID,
    time.Now().Unix())      // Processing time (seconds)
```

### 2. Data Timestamps
Original timestamps from CSV are preserved, with fallback to `time.Now()`:
```go
if tr.Timestamp == "" {
    tr.Timestamp = time.Now().Format(time.RFC3339)
}
```

### 3. Demo Data Generation
Use relative timestamps for realistic demo data:
```go
// Generate demo CSV with timestamps from 1 hour ago to now
timestamps, _ := utils.GenerateTimeRange("now-1h", "now", 100)
```

## 📝 Tips for Demos

1. **Use realistic time ranges**: `now-1h` to `now` for recent activity
2. **Add jitter**: Real systems don't have perfect intervals
3. **Consider time zones**: All timestamps are in UTC
4. **Test edge cases**: Use future timestamps (`now+5m`) for testing
5. **Show progression**: Use sequential timestamps to show trends

## 🛠️ Files Created

- `pkg/utils/timestamp.go` - Core utilities
- `cmd/demo-timestamps/main.go` - Go demo program  
- `scripts/generate-demo-timestamps.sh` - Shell script
- `docs/TIMESTAMP_HANDLING_GUIDE.md` - Comprehensive guide
- `docs/RELATIVE_TIMESTAMPS_QUICK_REFERENCE.md` - This reference

## 🎉 Ready to Use!

Your telemetry pipeline now supports intuitive relative timestamp generation for demos and testing. No more manually writing timestamp formats!
