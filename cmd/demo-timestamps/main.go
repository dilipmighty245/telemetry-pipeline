package main

import (
	"encoding/csv"
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"time"

	"github.com/dilipmighty245/telemetry-pipeline/pkg/utils"
)

func main() {
	if len(os.Args) < 2 {
		showUsage()
		return
	}

	command := os.Args[1]

	switch command {
	case "examples":
		showExamples()
	case "generate":
		if len(os.Args) < 3 {
			fmt.Println("Usage: demo-timestamps generate <relative-time-expressions...>")
			return
		}
		generateTimestamps(os.Args[2:])
	case "csv":
		generateCSV()
	case "range":
		generateRange()
	case "quick":
		showQuickTimestamps()
	default:
		fmt.Printf("Unknown command: %s\n", command)
		showUsage()
	}
}

func showUsage() {
	fmt.Println("Demo Timestamp Generator")
	fmt.Println("=======================")
	fmt.Println()
	fmt.Println("Usage: go run cmd/demo-timestamps/main.go <command> [options]")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  examples  - Show relative timestamp examples")
	fmt.Println("  generate  - Generate specific timestamps")
	fmt.Println("  csv       - Generate CSV file with timestamps")
	fmt.Println("  range     - Generate timestamp range")
	fmt.Println("  quick     - Show quick timestamp references")
	fmt.Println()
	fmt.Println("Relative Time Format:")
	fmt.Println("  now       - Current time")
	fmt.Println("  now-5s    - 5 seconds ago")
	fmt.Println("  now-5m    - 5 minutes ago")
	fmt.Println("  now-5h    - 5 hours ago")
	fmt.Println("  now-5d    - 5 days ago")
	fmt.Println("  now+5m    - 5 minutes from now")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  go run cmd/demo-timestamps/main.go examples")
	fmt.Println("  go run cmd/demo-timestamps/main.go generate now-5m now-1m now")
	fmt.Println("  go run cmd/demo-timestamps/main.go csv")
}

func showExamples() {
	fmt.Println("🕐 Relative Timestamp Examples")
	fmt.Println("==============================")
	fmt.Println()

	examples := []string{
		"now",
		"now-5s",
		"now-30s",
		"now-1m",
		"now-5m",
		"now-15m",
		"now-30m",
		"now-1h",
		"now-2h",
		"now-6h",
		"now-12h",
		"now-1d",
		"now-2d",
		"now-7d",
	}

	fmt.Println("Basic relative time expressions:")
	for _, expr := range examples {
		timestamp, err := utils.RelativeTimeString(expr)
		if err != nil {
			fmt.Printf("  %-12s -> ERROR: %v\n", expr, err)
		} else {
			fmt.Printf("  %-12s -> %s\n", expr, timestamp)
		}
	}

	fmt.Println()
	fmt.Println("Future timestamps (useful for testing):")
	futureExamples := []string{"now+5s", "now+1m", "now+1h", "now+1d"}
	for _, expr := range futureExamples {
		timestamp, err := utils.RelativeTimeString(expr)
		if err != nil {
			fmt.Printf("  %-12s -> ERROR: %v\n", expr, err)
		} else {
			fmt.Printf("  %-12s -> %s\n", expr, timestamp)
		}
	}

	fmt.Println()
	showCommonPatterns()
}

func showCommonPatterns() {
	fmt.Println("📊 Common Demo Patterns")
	fmt.Println("======================")
	fmt.Println()

	fmt.Println("Recent activity (last 5 minutes, 30-second intervals):")
	recentActivity := []string{"now-5m", "now-4m30s", "now-4m", "now-3m30s", "now-3m", "now-2m30s", "now-2m", "now-1m30s", "now-1m", "now-30s", "now"}
	for _, expr := range recentActivity {
		if timestamp, err := utils.RelativeTimeString(expr); err == nil {
			fmt.Printf("  %s\n", timestamp)
		}
	}

	fmt.Println()
	fmt.Println("Hourly snapshots (last 6 hours):")
	hourlySnapshots := []string{"now-6h", "now-5h", "now-4h", "now-3h", "now-2h", "now-1h", "now"}
	for _, expr := range hourlySnapshots {
		if timestamp, err := utils.RelativeTimeString(expr); err == nil {
			fmt.Printf("  %s\n", timestamp)
		}
	}

	fmt.Println()
	fmt.Println("Daily snapshots (last week):")
	dailySnapshots := []string{"now-7d", "now-6d", "now-5d", "now-4d", "now-3d", "now-2d", "now-1d", "now"}
	for _, expr := range dailySnapshots {
		if timestamp, err := utils.RelativeTimeString(expr); err == nil {
			fmt.Printf("  %s\n", timestamp)
		}
	}
}

func generateTimestamps(expressions []string) {
	fmt.Println("Generating timestamps for expressions:")
	fmt.Println()

	for _, expr := range expressions {
		timestamp, err := utils.RelativeTimeString(expr)
		if err != nil {
			fmt.Printf("  %-15s -> ERROR: %v\n", expr, err)
		} else {
			// Also show "time ago" format
			if t, parseErr := time.Parse(time.RFC3339, timestamp); parseErr == nil {
				timeAgo := utils.TimeAgo(t)
				fmt.Printf("  %-15s -> %s (%s)\n", expr, timestamp, timeAgo)
			} else {
				fmt.Printf("  %-15s -> %s\n", expr, timestamp)
			}
		}
	}
}

func generateCSV() {
	filename := "demo_relative_timestamps.csv"
	fmt.Printf("Generating CSV file: %s\n", filename)

	// Generate timestamps from 1 hour ago to now, with 100 records
	timestamps, err := utils.GenerateTimeRange("now-1h", "now", 100)
	if err != nil {
		fmt.Printf("Error generating timestamp range: %v\n", err)
		return
	}

	file, err := os.Create(filename)
	if err != nil {
		fmt.Printf("Error creating file: %v\n", err)
		return
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

	// Generate records
	for i, timestamp := range timestamps {
		record := []string{
			timestamp,
			strconv.Itoa(i % 4),                    // gpu_id (0-3)
			fmt.Sprintf("demo-host-%03d", (i/4)+1), // hostname
			fmt.Sprintf("GPU-%08d-1234-5678-9abc-%012d", i, i), // uuid
			fmt.Sprintf("nvidia%d", i%4),                       // device
			"NVIDIA H100 80GB HBM3",                            // modelName
			fmt.Sprintf("%.1f", 70.0+rand.Float64()*30),        // gpu_utilization
			fmt.Sprintf("%.1f", 60.0+rand.Float64()*30),        // memory_utilization
			strconv.Itoa(50000 + rand.Intn(30000)),             // memory_used_mb
			strconv.Itoa(10000 + rand.Intn(20000)),             // memory_free_mb
			fmt.Sprintf("%.1f", 55.0+rand.Float64()*20),        // temperature
			fmt.Sprintf("%.1f", 300.0+rand.Float64()*150),      // power_draw
			strconv.Itoa(1400 + rand.Intn(200)),                // sm_clock_mhz
			strconv.Itoa(1200 + rand.Intn(100)),                // memory_clock_mhz
		}
		writer.Write(record)
	}

	fmt.Printf("✅ Generated %s with %d records\n", filename, len(timestamps))
	fmt.Printf("   Time range: %s to %s\n", timestamps[0], timestamps[len(timestamps)-1])
}

func generateRange() {
	fmt.Println("Generating timestamp ranges:")
	fmt.Println()

	ranges := []struct {
		start, end string
		count      int
		desc       string
	}{
		{"now-1h", "now", 10, "Last hour (10 points)"},
		{"now-6h", "now", 24, "Last 6 hours (15-minute intervals)"},
		{"now-1d", "now", 48, "Last day (30-minute intervals)"},
		{"now-7d", "now", 168, "Last week (hourly)"},
	}

	for _, r := range ranges {
		fmt.Printf("%s:\n", r.desc)
		timestamps, err := utils.GenerateTimeRange(r.start, r.end, r.count)
		if err != nil {
			fmt.Printf("  Error: %v\n", err)
			continue
		}

		// Show first 3 and last 3 timestamps
		if len(timestamps) > 6 {
			for i := 0; i < 3; i++ {
				fmt.Printf("  %s\n", timestamps[i])
			}
			fmt.Printf("  ... (%d more) ...\n", len(timestamps)-6)
			for i := len(timestamps) - 3; i < len(timestamps); i++ {
				fmt.Printf("  %s\n", timestamps[i])
			}
		} else {
			for _, ts := range timestamps {
				fmt.Printf("  %s\n", ts)
			}
		}
		fmt.Println()
	}
}

func showQuickTimestamps() {
	fmt.Println("⚡ Quick Timestamp References")
	fmt.Println("============================")
	fmt.Println()

	quickKeys := []string{
		"now", "5s_ago", "30s_ago", "1m_ago", "5m_ago", "15m_ago", "30m_ago",
		"1h_ago", "2h_ago", "6h_ago", "12h_ago", "1d_ago", "2d_ago", "1w_ago",
	}

	for _, key := range quickKeys {
		timestamp, err := utils.GetQuickTimestamp(key)
		if err != nil {
			fmt.Printf("  %-10s -> ERROR: %v\n", key, err)
		} else {
			fmt.Printf("  %-10s -> %s\n", key, timestamp)
		}
	}

	fmt.Println()
	fmt.Println("Usage in Go code:")
	fmt.Println(`  timestamp, err := utils.GetQuickTimestamp("5m_ago")`)
	fmt.Println(`  timestamps := utils.DemoTimestampsRelative([]string{"now-1h", "now-30m", "now"})`)
}
