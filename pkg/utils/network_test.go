package utils

import (
	"context"
	"net"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetServiceAddress(t *testing.T) {
	// Save original environment
	originalEnv := map[string]string{
		"SERVICE_ADDRESS":          os.Getenv("SERVICE_ADDRESS"),
		"EXTERNAL_SERVICE_ADDRESS": os.Getenv("EXTERNAL_SERVICE_ADDRESS"),
		"POD_IP":                   os.Getenv("POD_IP"),
		"HOSTNAME":                 os.Getenv("HOSTNAME"),
		"SERVICE_NAME":             os.Getenv("SERVICE_NAME"),
		"NAMESPACE":                os.Getenv("NAMESPACE"),
	}

	// Clean up after test
	defer func() {
		for key, value := range originalEnv {
			if value == "" {
				os.Unsetenv(key)
			} else {
				os.Setenv(key, value)
			}
		}
	}()

	tests := []struct {
		name        string
		setupEnv    func()
		expected    string
		description string
	}{
		{
			name: "SERVICE_ADDRESS override",
			setupEnv: func() {
				os.Setenv("SERVICE_ADDRESS", "192.168.1.100")
				os.Unsetenv("EXTERNAL_SERVICE_ADDRESS")
				os.Unsetenv("POD_IP")
				os.Unsetenv("HOSTNAME")
			},
			expected:    "192.168.1.100",
			description: "Should use SERVICE_ADDRESS when explicitly set (highest priority)",
		},
		{
			name: "EXTERNAL_SERVICE_ADDRESS for multi-cluster",
			setupEnv: func() {
				os.Unsetenv("SERVICE_ADDRESS")
				os.Setenv("EXTERNAL_SERVICE_ADDRESS", "203.0.113.50")
				os.Setenv("POD_IP", "10.244.1.5")
			},
			expected:    "203.0.113.50",
			description: "Should use EXTERNAL_SERVICE_ADDRESS for cross-cluster communication",
		},
		{
			name: "POD_IP from Kubernetes downward API",
			setupEnv: func() {
				os.Unsetenv("SERVICE_ADDRESS")
				os.Unsetenv("EXTERNAL_SERVICE_ADDRESS")
				os.Setenv("POD_IP", "10.244.1.5")
				os.Unsetenv("HOSTNAME")
			},
			expected:    "10.244.1.5",
			description: "Should use POD_IP when available",
		},
		{
			name: "Invalid POD_IP should be ignored",
			setupEnv: func() {
				os.Unsetenv("SERVICE_ADDRESS")
				os.Unsetenv("EXTERNAL_SERVICE_ADDRESS")
				os.Setenv("POD_IP", "invalid-ip")
				os.Unsetenv("HOSTNAME")
			},
			expected:    "", // Will fall back to network interface detection
			description: "Should ignore invalid POD_IP and fall back to network interfaces",
		},
		{
			name: "Loopback POD_IP should be ignored",
			setupEnv: func() {
				os.Unsetenv("SERVICE_ADDRESS")
				os.Unsetenv("EXTERNAL_SERVICE_ADDRESS")
				os.Setenv("POD_IP", "127.0.0.1")
				os.Unsetenv("HOSTNAME")
			},
			expected:    "", // Will fall back to network interface detection
			description: "Should ignore loopback POD_IP",
		},
		{
			name: "HOSTNAME resolution",
			setupEnv: func() {
				os.Unsetenv("SERVICE_ADDRESS")
				os.Unsetenv("EXTERNAL_SERVICE_ADDRESS")
				os.Unsetenv("POD_IP")
				os.Setenv("HOSTNAME", "non-existent-hostname-12345") // Use a hostname that won't resolve
			},
			expected:    "", // Will fall back to network interface detection
			description: "Should handle hostname resolution failure gracefully",
		},
		{
			name: "Clean environment - network interface fallback",
			setupEnv: func() {
				os.Unsetenv("SERVICE_ADDRESS")
				os.Unsetenv("EXTERNAL_SERVICE_ADDRESS")
				os.Unsetenv("POD_IP")
				os.Unsetenv("HOSTNAME")
				os.Unsetenv("SERVICE_NAME")
				os.Unsetenv("NAMESPACE")
			},
			expected:    "", // Will depend on actual network interfaces
			description: "Should fall back to network interface detection when no env vars set",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Add timeout to prevent tests from hanging
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			done := make(chan struct{})
			var result string

			go func() {
				defer close(done)
				tt.setupEnv()
				result = GetServiceAddress()
			}()

			select {
			case <-done:
				if tt.expected == "" {
					// For cases where we expect fallback behavior, we should get either
					// a valid IP address or "localhost"
					assert.True(t, result != "", "Should return some address, not empty string")
					if result != "localhost" {
						// If not localhost, should be a valid IP
						ip := net.ParseIP(result)
						assert.NotNil(t, ip, "Should return valid IP address or localhost")
					}
				} else {
					assert.Equal(t, tt.expected, result, tt.description)
				}
			case <-ctx.Done():
				t.Fatalf("Test timed out after 5 seconds - likely stuck on network resolution")
			}
		})
	}
}

func TestGetKubernetesServiceAddress(t *testing.T) {
	// Save original environment
	originalEnv := map[string]string{
		"POD_IP":       os.Getenv("POD_IP"),
		"HOSTNAME":     os.Getenv("HOSTNAME"),
		"SERVICE_NAME": os.Getenv("SERVICE_NAME"),
		"NAMESPACE":    os.Getenv("NAMESPACE"),
	}

	// Clean up after test
	defer func() {
		for key, value := range originalEnv {
			if value == "" {
				os.Unsetenv(key)
			} else {
				os.Setenv(key, value)
			}
		}
	}()

	tests := []struct {
		name     string
		setupEnv func()
		expected string
	}{
		{
			name: "Valid POD_IP",
			setupEnv: func() {
				os.Setenv("POD_IP", "10.244.1.5")
				os.Unsetenv("HOSTNAME")
				os.Unsetenv("SERVICE_NAME")
				os.Unsetenv("NAMESPACE")
			},
			expected: "10.244.1.5",
		},
		{
			name: "Invalid POD_IP ignored",
			setupEnv: func() {
				os.Setenv("POD_IP", "not-an-ip")
				os.Unsetenv("HOSTNAME")
				os.Unsetenv("SERVICE_NAME")
				os.Unsetenv("NAMESPACE")
			},
			expected: "",
		},
		{
			name: "Loopback POD_IP ignored",
			setupEnv: func() {
				os.Setenv("POD_IP", "127.0.0.1")
				os.Unsetenv("HOSTNAME")
				os.Unsetenv("SERVICE_NAME")
				os.Unsetenv("NAMESPACE")
			},
			expected: "",
		},
		{
			name: "No environment variables",
			setupEnv: func() {
				os.Unsetenv("POD_IP")
				os.Unsetenv("HOSTNAME")
				os.Unsetenv("SERVICE_NAME")
				os.Unsetenv("NAMESPACE")
			},
			expected: "",
		},
		{
			name: "SERVICE_NAME without NAMESPACE",
			setupEnv: func() {
				os.Unsetenv("POD_IP")
				os.Unsetenv("HOSTNAME")
				os.Setenv("SERVICE_NAME", "my-service")
				os.Unsetenv("NAMESPACE")
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupEnv()
			result := getKubernetesServiceAddress()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetServiceAddressWithFallback(t *testing.T) {
	// Save original environment
	originalEnv := map[string]string{
		"SERVICE_ADDRESS":          os.Getenv("SERVICE_ADDRESS"),
		"EXTERNAL_SERVICE_ADDRESS": os.Getenv("EXTERNAL_SERVICE_ADDRESS"),
		"POD_IP":                   os.Getenv("POD_IP"),
		"HOSTNAME":                 os.Getenv("HOSTNAME"),
	}

	// Clean up after test
	defer func() {
		for key, value := range originalEnv {
			if value == "" {
				os.Unsetenv(key)
			} else {
				os.Setenv(key, value)
			}
		}
	}()

	tests := []struct {
		name     string
		setupEnv func()
		fallback string
		expected string
	}{
		{
			name: "Uses SERVICE_ADDRESS when available",
			setupEnv: func() {
				os.Setenv("SERVICE_ADDRESS", "192.168.1.100")
			},
			fallback: "custom-fallback",
			expected: "192.168.1.100",
		},
		{
			name: "Uses custom fallback when GetServiceAddress returns localhost",
			setupEnv: func() {
				// Clear all env vars to force localhost fallback
				os.Unsetenv("SERVICE_ADDRESS")
				os.Unsetenv("EXTERNAL_SERVICE_ADDRESS")
				os.Unsetenv("POD_IP")
				os.Unsetenv("HOSTNAME")
			},
			fallback: "custom-fallback",
			expected: "custom-fallback", // Only if GetServiceAddress actually returns localhost
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupEnv()
			result := GetServiceAddressWithFallback(tt.fallback)

			if tt.name == "Uses custom fallback when GetServiceAddress returns localhost" {
				// This test is tricky because GetServiceAddress might not return localhost
				// if there are actual network interfaces available
				serviceAddr := GetServiceAddress()
				if serviceAddr == "localhost" {
					assert.Equal(t, tt.fallback, result)
				} else {
					assert.Equal(t, serviceAddr, result)
				}
			} else {
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestValidateServiceAddress(t *testing.T) {
	tests := []struct {
		name      string
		address   string
		port      int
		expectErr bool
	}{
		{
			name:      "Invalid address",
			address:   "999.999.999.999",
			port:      8080,
			expectErr: true,
		},
		{
			name:      "Invalid port",
			address:   "127.0.0.1",
			port:      99999,
			expectErr: true,
		},
		{
			name:      "Unreachable address",
			address:   "192.0.2.1", // TEST-NET-1 (RFC 5737) - should be unreachable
			port:      8080,
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Add timeout to prevent hanging on network calls
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			done := make(chan error)
			go func() {
				done <- ValidateServiceAddress(tt.address, tt.port)
			}()

			select {
			case err := <-done:
				if tt.expectErr {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
				}
			case <-ctx.Done():
				if tt.expectErr {
					// Timeout is also a valid error for these test cases
					t.Logf("Test timed out (expected for unreachable addresses)")
				} else {
					t.Fatalf("Test timed out unexpectedly")
				}
			}
		})
	}
}

func TestIsPrivateIP(t *testing.T) {
	tests := []struct {
		name     string
		ip       string
		expected bool
	}{
		{
			name:     "10.0.0.0/8 range",
			ip:       "10.0.0.1",
			expected: true,
		},
		{
			name:     "172.16.0.0/12 range",
			ip:       "172.16.0.1",
			expected: true,
		},
		{
			name:     "172.31.255.255 (end of 172.16.0.0/12)",
			ip:       "172.31.255.255",
			expected: true,
		},
		{
			name:     "172.32.0.1 (outside 172.16.0.0/12)",
			ip:       "172.32.0.1",
			expected: false,
		},
		{
			name:     "192.168.0.0/16 range",
			ip:       "192.168.1.1",
			expected: true,
		},
		{
			name:     "Public IP",
			ip:       "8.8.8.8",
			expected: false,
		},
		{
			name:     "Localhost",
			ip:       "127.0.0.1",
			expected: false,
		},
		{
			name:     "Invalid IP",
			ip:       "invalid",
			expected: false,
		},
		{
			name:     "Nil IP",
			ip:       "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var ip net.IP
			if tt.ip != "" {
				ip = net.ParseIP(tt.ip)
			}
			result := IsPrivateIP(ip)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetPublicIP(t *testing.T) {
	// This test is environment-dependent and might fail in some network configurations
	t.Run("GetPublicIP returns valid IP", func(t *testing.T) {
		// Add timeout to prevent hanging
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		done := make(chan struct{})
		var ip string
		var err error

		go func() {
			defer close(done)
			ip, err = GetPublicIP()
		}()

		select {
		case <-done:
			if err != nil {
				// This might fail in restricted network environments, so we'll skip
				t.Skipf("GetPublicIP failed (expected in some environments): %v", err)
				return
			}

			// If it succeeds, the result should be a valid IP
			parsedIP := net.ParseIP(ip)
			assert.NotNil(t, parsedIP, "Should return valid IP address")
			assert.NotEqual(t, "127.0.0.1", ip, "Should not return localhost")
		case <-ctx.Done():
			t.Skipf("GetPublicIP timed out - likely network connectivity issue")
		}
	})
}

func TestFormatServiceEndpoint(t *testing.T) {
	tests := []struct {
		name     string
		address  string
		port     int
		expected string
	}{
		{
			name:     "IPv4 address",
			address:  "192.168.1.1",
			port:     8080,
			expected: "192.168.1.1:8080",
		},
		{
			name:     "IPv6 address",
			address:  "2001:db8::1",
			port:     8080,
			expected: "[2001:db8::1]:8080",
		},
		{
			name:     "IPv6 address already bracketed",
			address:  "[2001:db8::1]",
			port:     8080,
			expected: "[2001:db8::1]:8080",
		},
		{
			name:     "Hostname",
			address:  "example.com",
			port:     443,
			expected: "example.com:443",
		},
		{
			name:     "Localhost",
			address:  "localhost",
			port:     3000,
			expected: "localhost:3000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatServiceEndpoint(tt.address, tt.port)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetFirstNonLoopbackIPv4(t *testing.T) {
	// This test is system-dependent, so we'll just verify it returns a valid result
	t.Run("Returns valid IP or empty string", func(t *testing.T) {
		result := getFirstNonLoopbackIPv4()

		if result != "" {
			// If it returns something, it should be a valid IPv4 address
			ip := net.ParseIP(result)
			require.NotNil(t, ip, "Should return valid IP address if not empty")

			ipv4 := ip.To4()
			require.NotNil(t, ipv4, "Should return IPv4 address")

			assert.False(t, ip.IsLoopback(), "Should not return loopback address")
		}
		// If empty, that's also valid (no non-loopback interfaces found)
	})
}

// Benchmark tests
func BenchmarkGetServiceAddress(b *testing.B) {
	// Set up a typical Kubernetes environment
	os.Setenv("POD_IP", "10.244.1.5")
	defer os.Unsetenv("POD_IP")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		GetServiceAddress()
	}
}

func BenchmarkFormatServiceEndpoint(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		FormatServiceEndpoint("192.168.1.1", 8080)
	}
}

// Integration test for multi-cluster scenario
func TestMultiClusterScenario(t *testing.T) {
	// Save original environment
	originalEnv := map[string]string{
		"SERVICE_ADDRESS":          os.Getenv("SERVICE_ADDRESS"),
		"EXTERNAL_SERVICE_ADDRESS": os.Getenv("EXTERNAL_SERVICE_ADDRESS"),
		"POD_IP":                   os.Getenv("POD_IP"),
		"HOSTNAME":                 os.Getenv("HOSTNAME"),
		"SERVICE_NAME":             os.Getenv("SERVICE_NAME"),
		"NAMESPACE":                os.Getenv("NAMESPACE"),
	}

	// Clean up after test
	defer func() {
		for key, value := range originalEnv {
			if value == "" {
				os.Unsetenv(key)
			} else {
				os.Setenv(key, value)
			}
		}
	}()

	t.Run("Multi-cluster streamer registration", func(t *testing.T) {
		// Simulate a streamer running in cluster A that needs to be accessible from cluster B
		os.Setenv("EXTERNAL_SERVICE_ADDRESS", "203.0.113.50") // LoadBalancer IP
		os.Setenv("POD_IP", "10.244.1.5")                     // Internal pod IP
		os.Setenv("SERVICE_NAME", "nexus-streamer")
		os.Setenv("NAMESPACE", "telemetry")

		address := GetServiceAddress()
		assert.Equal(t, "203.0.113.50", address, "Should use external address for multi-cluster access")

		endpoint := FormatServiceEndpoint(address, 8081)
		assert.Equal(t, "203.0.113.50:8081", endpoint)
	})

	t.Run("Single cluster with pod IP", func(t *testing.T) {
		os.Unsetenv("EXTERNAL_SERVICE_ADDRESS")
		os.Setenv("POD_IP", "10.244.1.5")
		os.Setenv("SERVICE_NAME", "nexus-streamer")
		os.Setenv("NAMESPACE", "telemetry")

		address := GetServiceAddress()
		assert.Equal(t, "10.244.1.5", address, "Should use pod IP for single-cluster access")
	})
}
