package validation

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewValidator(t *testing.T) {
	v := NewValidator()
	assert.NotNil(t, v)
}

func TestValidator_ValidateGPUID(t *testing.T) {
	v := NewValidator()

	tests := []struct {
		name    string
		gpuID   string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid GPU ID",
			gpuID:   "gpu-0",
			wantErr: false,
		},
		{
			name:    "valid GPU ID with underscore",
			gpuID:   "gpu_0",
			wantErr: false,
		},
		{
			name:    "valid UUID-like GPU ID",
			gpuID:   "GPU-12345-abcde-67890",
			wantErr: false,
		},
		{
			name:    "empty GPU ID",
			gpuID:   "",
			wantErr: true,
			errMsg:  "GPU ID is required",
		},
		{
			name:    "GPU ID too long",
			gpuID:   string(make([]byte, 101)), // 101 characters
			wantErr: true,
			errMsg:  "GPU ID must be less than 100 characters",
		},
		{
			name:    "GPU ID with invalid characters",
			gpuID:   "gpu@0",
			wantErr: true,
			errMsg:  "GPU ID contains invalid characters",
		},
		{
			name:    "GPU ID with spaces",
			gpuID:   "gpu 0",
			wantErr: true,
			errMsg:  "GPU ID contains invalid characters",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.ValidateGPUID(tt.gpuID)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidator_ValidateHostname(t *testing.T) {
	v := NewValidator()

	tests := []struct {
		name     string
		hostname string
		wantErr  bool
		errMsg   string
	}{
		{
			name:     "valid hostname",
			hostname: "gpu-node-01",
			wantErr:  false,
		},
		{
			name:     "valid FQDN",
			hostname: "gpu-node-01.example.com",
			wantErr:  false,
		},
		{
			name:     "empty hostname",
			hostname: "",
			wantErr:  true,
			errMsg:   "hostname is required",
		},
		{
			name:     "hostname too long",
			hostname: string(make([]byte, 254)), // 254 characters
			wantErr:  true,
			errMsg:   "hostname must be less than 253 characters",
		},
		{
			name:     "hostname with invalid start",
			hostname: "-invalid",
			wantErr:  true,
			errMsg:   "invalid hostname format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.ValidateHostname(tt.hostname)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidator_ValidateClusterID(t *testing.T) {
	v := NewValidator()

	tests := []struct {
		name      string
		clusterID string
		wantErr   bool
		errMsg    string
	}{
		{
			name:      "valid cluster ID",
			clusterID: "production-cluster",
			wantErr:   false,
		},
		{
			name:      "valid cluster ID with underscore",
			clusterID: "test_cluster_1",
			wantErr:   false,
		},
		{
			name:      "empty cluster ID",
			clusterID: "",
			wantErr:   true,
			errMsg:    "cluster ID is required",
		},
		{
			name:      "cluster ID too long",
			clusterID: string(make([]byte, 51)), // 51 characters
			wantErr:   true,
			errMsg:    "cluster ID must be less than 50 characters",
		},
		{
			name:      "cluster ID with invalid characters",
			clusterID: "cluster@1",
			wantErr:   true,
			errMsg:    "cluster ID contains invalid characters",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.ValidateClusterID(tt.clusterID)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidator_ValidateTimeRange(t *testing.T) {
	v := NewValidator()

	now := time.Now()
	validStart := now.Add(-time.Hour).Format(time.RFC3339)
	validEnd := now.Format(time.RFC3339)
	futureTime := now.Add(2 * time.Hour).Format(time.RFC3339)
	pastTime := now.AddDate(-2, 0, 0).Format(time.RFC3339)

	tests := []struct {
		name         string
		startTimeStr string
		endTimeStr   string
		wantErr      bool
		errMsg       string
	}{
		{
			name:         "valid time range",
			startTimeStr: validStart,
			endTimeStr:   validEnd,
			wantErr:      false,
		},
		{
			name:         "empty time range",
			startTimeStr: "",
			endTimeStr:   "",
			wantErr:      false,
		},
		{
			name:         "only start time",
			startTimeStr: validStart,
			endTimeStr:   "",
			wantErr:      false,
		},
		{
			name:         "invalid start time format",
			startTimeStr: "invalid-time",
			endTimeStr:   "",
			wantErr:      true,
			errMsg:       "invalid start_time format",
		},
		{
			name:         "invalid end time format",
			startTimeStr: "",
			endTimeStr:   "invalid-time",
			wantErr:      true,
			errMsg:       "invalid end_time format",
		},
		{
			name:         "start time after end time",
			startTimeStr: validEnd,
			endTimeStr:   validStart,
			wantErr:      true,
			errMsg:       "start_time must be before end_time",
		},
		{
			name:         "start time too far in past",
			startTimeStr: pastTime,
			endTimeStr:   "",
			wantErr:      true,
			errMsg:       "start_time cannot be more than 2 years in the past",
		},
		{
			name:         "end time too far in future",
			startTimeStr: "",
			endTimeStr:   futureTime,
			wantErr:      true,
			errMsg:       "end_time cannot be more than 1 hour in the future",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			startTime, endTime, err := v.ValidateTimeRange(tt.startTimeStr, tt.endTimeStr)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
				if tt.startTimeStr != "" {
					assert.NotNil(t, startTime)
				}
				if tt.endTimeStr != "" {
					assert.NotNil(t, endTime)
				}
			}
		})
	}
}

func TestValidator_ValidateLimit(t *testing.T) {
	v := NewValidator()

	tests := []struct {
		name     string
		limitStr string
		maxLimit int
		want     int
		wantErr  bool
		errMsg   string
	}{
		{
			name:     "valid limit",
			limitStr: "50",
			maxLimit: 100,
			want:     50,
			wantErr:  false,
		},
		{
			name:     "empty limit (default)",
			limitStr: "",
			maxLimit: 100,
			want:     100,
			wantErr:  false,
		},
		{
			name:     "invalid limit format",
			limitStr: "abc",
			maxLimit: 100,
			want:     0,
			wantErr:  true,
			errMsg:   "limit must be a valid integer",
		},
		{
			name:     "zero limit",
			limitStr: "0",
			maxLimit: 100,
			want:     0,
			wantErr:  true,
			errMsg:   "limit must be greater than 0",
		},
		{
			name:     "negative limit",
			limitStr: "-10",
			maxLimit: 100,
			want:     0,
			wantErr:  true,
			errMsg:   "limit must be greater than 0",
		},
		{
			name:     "limit exceeds max",
			limitStr: "200",
			maxLimit: 100,
			want:     0,
			wantErr:  true,
			errMsg:   "limit cannot exceed 100",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			limit, err := v.ValidateLimit(tt.limitStr, tt.maxLimit)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				assert.Equal(t, tt.want, limit)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, limit)
			}
		})
	}
}

func TestValidator_ValidateOrigin(t *testing.T) {
	v := NewValidator()
	allowedOrigins := []string{"example.com", "api.example.com"}

	tests := []struct {
		name   string
		origin string
		host   string
		want   bool
	}{
		{
			name:   "empty origin",
			origin: "",
			host:   "example.com",
			want:   false,
		},
		{
			name:   "localhost origin",
			origin: "http://localhost:3000",
			host:   "api.example.com",
			want:   true,
		},
		{
			name:   "127.0.0.1 origin",
			origin: "https://127.0.0.1:8080",
			host:   "api.example.com",
			want:   true,
		},
		{
			name:   "allowed origin",
			origin: "https://example.com",
			host:   "api.example.com",
			want:   true,
		},
		{
			name:   "same origin",
			origin: "https://api.example.com",
			host:   "api.example.com",
			want:   true,
		},
		{
			name:   "disallowed origin",
			origin: "https://malicious.com",
			host:   "api.example.com",
			want:   false,
		},
		{
			name:   "invalid origin URL",
			origin: "not-a-valid-url",
			host:   "api.example.com",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := v.ValidateOrigin(tt.origin, tt.host, allowedOrigins)
			assert.Equal(t, tt.want, result)
		})
	}
}

func TestValidator_SanitizeString(t *testing.T) {
	v := NewValidator()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "clean string",
			input: "hello world",
			want:  "hello world",
		},
		{
			name:  "string with leading/trailing whitespace",
			input: "  hello world  ",
			want:  "hello world",
		},
		{
			name:  "string with null byte",
			input: "hello\x00world",
			want:  "helloworld",
		},
		{
			name:  "string with control characters",
			input: "hello\x01\x02world",
			want:  "helloworld",
		},
		{
			name:  "string with valid whitespace",
			input: "hello\tworld\ntest",
			want:  "hello\tworld\ntest",
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := v.SanitizeString(tt.input)
			assert.Equal(t, tt.want, result)
		})
	}
}

func TestValidationError(t *testing.T) {
	err := ValidationError{
		Field:   "test_field",
		Value:   "test_value",
		Message: "test message",
	}

	assert.Contains(t, err.Error(), "test_field")
	assert.Contains(t, err.Error(), "test message")
}

// Benchmark tests
func BenchmarkValidator_ValidateGPUID(b *testing.B) {
	v := NewValidator()
	for i := 0; i < b.N; i++ {
		_ = v.ValidateGPUID("gpu-0")
	}
}

func BenchmarkValidator_ValidateTimeRange(b *testing.B) {
	v := NewValidator()
	now := time.Now()
	start := now.Add(-time.Hour).Format(time.RFC3339)
	end := now.Format(time.RFC3339)

	for i := 0; i < b.N; i++ {
		_, _, _ = v.ValidateTimeRange(start, end)
	}
}

func BenchmarkValidator_SanitizeString(b *testing.B) {
	v := NewValidator()
	input := "hello\x00world\x01test"

	for i := 0; i < b.N; i++ {
		_ = v.SanitizeString(input)
	}
}