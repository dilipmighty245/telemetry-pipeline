package utils

import (
	"fmt"
	"net"
	"os"
	"strings"
)

// GetServiceAddress determines the appropriate network address for service registration
// in multi-cluster environments. It follows this priority order:
// 1. SERVICE_ADDRESS environment variable (for explicit override - critical for multi-cluster)
// 2. EXTERNAL_SERVICE_ADDRESS for cross-cluster communication (LoadBalancer/NodePort IPs)
// 3. Pod IP in Kubernetes (from multiple sources)
// 4. First non-loopback IPv4 address from network interfaces
// 5. Fallback to "localhost" (only for local development)
func GetServiceAddress() string {
	// 1. Check for explicit service address override (highest priority for multi-cluster)
	if addr := os.Getenv("SERVICE_ADDRESS"); addr != "" {
		return addr
	}

	// 2. Check for external service address (for cross-cluster communication)
	// This should be set to LoadBalancer IP, NodePort accessible IP, or Ingress IP
	if addr := os.Getenv("EXTERNAL_SERVICE_ADDRESS"); addr != "" {
		return addr
	}

	// 3. In Kubernetes, try multiple methods to get the correct IP
	if addr := getKubernetesServiceAddress(); addr != "" {
		return addr
	}

	// 4. Try to get the first non-loopback IPv4 address
	if addr := getFirstNonLoopbackIPv4(); addr != "" {
		return addr
	}

	// 5. Fallback to localhost (only for local development)
	return "localhost"
}

// getKubernetesServiceAddress attempts to determine the service address in Kubernetes
// using multiple methods suitable for multi-cluster environments
func getKubernetesServiceAddress() string {
	// Method 1: Check POD_IP environment variable (set by Kubernetes downward API)
	if podIP := os.Getenv("POD_IP"); podIP != "" {
		if ip := net.ParseIP(podIP); ip != nil && !ip.IsLoopback() {
			return podIP
		}
	}

	// Method 2: Check HOSTNAME and resolve it (works in most Kubernetes setups)
	if hostname := os.Getenv("HOSTNAME"); hostname != "" {
		if ips, err := net.LookupIP(hostname); err == nil {
			for _, ip := range ips {
				if ipv4 := ip.To4(); ipv4 != nil && !ipv4.IsLoopback() {
					return ipv4.String()
				}
			}
		}
	}

	// Method 3: Try to resolve the service's own FQDN if available
	if serviceName := os.Getenv("SERVICE_NAME"); serviceName != "" {
		if namespace := os.Getenv("NAMESPACE"); namespace != "" {
			// Try service FQDN: service-name.namespace.svc.cluster.local
			fqdn := fmt.Sprintf("%s.%s.svc.cluster.local", serviceName, namespace)
			if ips, err := net.LookupIP(fqdn); err == nil {
				for _, ip := range ips {
					if ipv4 := ip.To4(); ipv4 != nil && !ipv4.IsLoopback() {
						return ipv4.String()
					}
				}
			}
		}
	}

	return ""
}

// getFirstNonLoopbackIPv4 returns the first non-loopback IPv4 address found
// on the system's network interfaces.
func getFirstNonLoopbackIPv4() string {
	interfaces, err := net.Interfaces()
	if err != nil {
		return ""
	}

	for _, iface := range interfaces {
		// Skip down interfaces and loopback
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}

			// Check if it's a valid IPv4 address and not loopback
			if ip != nil {
				if ipv4 := ip.To4(); ipv4 != nil && !ipv4.IsLoopback() {
					// Prefer non-private addresses, but accept private ones too
					return ipv4.String()
				}
			}
		}
	}

	return ""
}

// GetServiceAddressWithFallback gets the service address with a custom fallback.
// This is useful when you want to specify a different fallback than "localhost".
func GetServiceAddressWithFallback(fallback string) string {
	if addr := GetServiceAddress(); addr != "localhost" {
		return addr
	}
	return fallback
}

// ValidateServiceAddress checks if the provided address is reachable.
// This can be used to verify that the determined address is actually accessible.
func ValidateServiceAddress(address string, port int) error {
	conn, err := net.Dial("tcp", net.JoinHostPort(address, fmt.Sprintf("%d", port)))
	if err != nil {
		return fmt.Errorf("cannot connect to %s:%d - %w", address, port, err)
	}
	conn.Close()
	return nil
}

// IsPrivateIP checks if an IP address is in a private network range
func IsPrivateIP(ip net.IP) bool {
	if ip == nil {
		return false
	}

	// IPv4 private ranges:
	// 10.0.0.0/8
	// 172.16.0.0/12
	// 192.168.0.0/16
	privateRanges := []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
	}

	for _, cidr := range privateRanges {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			continue
		}
		if network.Contains(ip) {
			return true
		}
	}

	return false
}

// GetPublicIP attempts to determine the public IP address by connecting to a remote service.
// This should be used sparingly and with caution in production environments.
func GetPublicIP() (string, error) {
	// Try to connect to a well-known service to determine our external IP
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return "", fmt.Errorf("failed to determine public IP: %w", err)
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP.String(), nil
}

// FormatServiceEndpoint formats a service address and port into a standard endpoint format
func FormatServiceEndpoint(address string, port int) string {
	// Handle IPv6 addresses
	if strings.Contains(address, ":") && !strings.HasPrefix(address, "[") {
		return fmt.Sprintf("[%s]:%d", address, port)
	}
	return fmt.Sprintf("%s:%d", address, port)
}
