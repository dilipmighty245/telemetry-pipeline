package utils

import (
	"fmt"
	"math/rand"
	"regexp"
	"strconv"
	"strings"
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

// RelativeTime generates a timestamp relative to now
// Supports formats like: "now-5m", "now-30s", "now-2h", "now-1d"
func RelativeTime(relative string) (time.Time, error) {
	// Clean the input
	relative = strings.TrimSpace(strings.ToLower(relative))

	// Handle "now" case
	if relative == "now" {
		return time.Now().UTC(), nil
	}

	// Parse relative time expressions
	re := regexp.MustCompile(`^now\s*([+-])\s*(\d+)\s*([smhd])$`)
	matches := re.FindStringSubmatch(relative)

	if len(matches) != 4 {
		return time.Time{}, fmt.Errorf("invalid relative time format: %s (expected format: now-5m, now+30s, etc.)", relative)
	}

	sign := matches[1]
	valueStr := matches[2]
	unit := matches[3]

	value, err := strconv.Atoi(valueStr)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid number in relative time: %s", valueStr)
	}

	var duration time.Duration
	switch unit {
	case "s":
		duration = time.Duration(value) * time.Second
	case "m":
		duration = time.Duration(value) * time.Minute
	case "h":
		duration = time.Duration(value) * time.Hour
	case "d":
		duration = time.Duration(value) * 24 * time.Hour
	default:
		return time.Time{}, fmt.Errorf("invalid time unit: %s (use s, m, h, or d)", unit)
	}

	now := time.Now().UTC()
	if sign == "-" {
		return now.Add(-duration), nil
	} else {
		return now.Add(duration), nil
	}
}

// RelativeTimeString generates a timestamp string relative to now
func RelativeTimeString(relative string) (string, error) {
	t, err := RelativeTime(relative)
	if err != nil {
		return "", err
	}
	return t.Format(time.RFC3339), nil
}

// GenerateRelativeTimestamps generates multiple timestamps with relative offsets
func GenerateRelativeTimestamps(relatives []string) ([]string, error) {
	timestamps := make([]string, len(relatives))

	for i, relative := range relatives {
		timestamp, err := RelativeTimeString(relative)
		if err != nil {
			return nil, fmt.Errorf("error generating timestamp for '%s': %w", relative, err)
		}
		timestamps[i] = timestamp
	}

	return timestamps, nil
}

// GenerateTimeRange generates timestamps between two relative times
func GenerateTimeRange(startRelative, endRelative string, count int) ([]string, error) {
	startTime, err := RelativeTime(startRelative)
	if err != nil {
		return nil, fmt.Errorf("invalid start time '%s': %w", startRelative, err)
	}

	endTime, err := RelativeTime(endRelative)
	if err != nil {
		return nil, fmt.Errorf("invalid end time '%s': %w", endRelative, err)
	}

	if startTime.After(endTime) {
		startTime, endTime = endTime, startTime // Swap if needed
	}

	duration := endTime.Sub(startTime)
	if count <= 1 {
		return []string{startTime.Format(time.RFC3339)}, nil
	}

	interval := duration / time.Duration(count-1)
	timestamps := make([]string, count)

	for i := 0; i < count; i++ {
		timestamp := startTime.Add(time.Duration(i) * interval)
		timestamps[i] = timestamp.Format(time.RFC3339)
	}

	return timestamps, nil
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

// DemoTimestampsRelative generates demo timestamps using relative time expressions
func DemoTimestampsRelative(relatives []string) []string {
	timestamps, err := GenerateRelativeTimestamps(relatives)
	if err != nil {
		// Fallback to current time if there's an error
		fallback := make([]string, len(relatives))
		now := time.Now().UTC().Format(time.RFC3339)
		for i := range fallback {
			fallback[i] = now
		}
		return fallback
	}
	return timestamps
}

// ParseDuration parses duration strings like "5m", "30s", "2h"
func ParseDuration(s string) (time.Duration, error) {
	// Handle common formats
	s = strings.TrimSpace(strings.ToLower(s))

	// Add 's' suffix if just a number
	if matched, _ := regexp.MatchString(`^\d+$`, s); matched {
		s += "s"
	}

	return time.ParseDuration(s)
}

// TimeAgo returns a human-readable "time ago" string
func TimeAgo(t time.Time) string {
	now := time.Now().UTC()
	if t.After(now) {
		t = now // Don't show future times as "ago"
	}

	duration := now.Sub(t)

	switch {
	case duration < time.Minute:
		return fmt.Sprintf("%d seconds ago", int(duration.Seconds()))
	case duration < time.Hour:
		return fmt.Sprintf("%d minutes ago", int(duration.Minutes()))
	case duration < 24*time.Hour:
		return fmt.Sprintf("%d hours ago", int(duration.Hours()))
	case duration < 30*24*time.Hour:
		return fmt.Sprintf("%d days ago", int(duration.Hours()/24))
	default:
		return fmt.Sprintf("%d months ago", int(duration.Hours()/(24*30)))
	}
}

// Examples and helper functions for common use cases

// QuickTimestamps provides quick access to common relative timestamps
var QuickTimestamps = map[string]string{
	"now":     "now",
	"5s_ago":  "now-5s",
	"30s_ago": "now-30s",
	"1m_ago":  "now-1m",
	"5m_ago":  "now-5m",
	"15m_ago": "now-15m",
	"30m_ago": "now-30m",
	"1h_ago":  "now-1h",
	"2h_ago":  "now-2h",
	"6h_ago":  "now-6h",
	"12h_ago": "now-12h",
	"1d_ago":  "now-1d",
	"2d_ago":  "now-2d",
	"1w_ago":  "now-7d",
}

// GetQuickTimestamp returns a timestamp for a quick reference
func GetQuickTimestamp(key string) (string, error) {
	if relative, exists := QuickTimestamps[key]; exists {
		return RelativeTimeString(relative)
	}
	return "", fmt.Errorf("unknown quick timestamp key: %s", key)
}

// GenerateRecentTimestamps generates timestamps for recent time periods
func GenerateRecentTimestamps(count int, maxAgeMinutes int) []string {
	timestamps := make([]string, count)
	now := time.Now().UTC()

	for i := 0; i < count; i++ {
		// Random time within the last maxAgeMinutes
		randomMinutes := rand.Intn(maxAgeMinutes)
		timestamp := now.Add(-time.Duration(randomMinutes) * time.Minute)
		timestamps[i] = timestamp.Format(time.RFC3339)
	}

	return timestamps
}
