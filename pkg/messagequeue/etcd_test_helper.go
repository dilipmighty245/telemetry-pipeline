package messagequeue

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/server/v3/embed"
	"go.etcd.io/etcd/server/v3/etcdserver/api/v3client"
)

// EtcdTestServer wraps an embedded etcd server for testing
type EtcdTestServer struct {
	Server    *embed.Etcd
	Client    *clientv3.Client
	Endpoints []string
	DataDir   string
}

// StartEtcdTestServer starts an embedded etcd server for testing
func StartEtcdTestServer() (*EtcdTestServer, error) {
	// Create temporary directory for etcd data
	dataDir, err := os.MkdirTemp("", "etcd-test-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}

	// Find available ports
	clientPort, err := findAvailablePort()
	if err != nil {
		os.RemoveAll(dataDir)
		return nil, fmt.Errorf("failed to find available client port: %w", err)
	}

	peerPort, err := findAvailablePort()
	if err != nil {
		os.RemoveAll(dataDir)
		return nil, fmt.Errorf("failed to find available peer port: %w", err)
	}

	// Configure embedded etcd
	cfg := embed.NewConfig()
	cfg.Name = "test-etcd"
	cfg.Dir = dataDir
	cfg.LogLevel = "error" // Reduce log noise in tests

	// Set client URLs
	clientURL := "http://127.0.0.1:" + strconv.Itoa(clientPort)
	cfg.ListenClientUrls, err = parseURLs([]string{clientURL})
	if err != nil {
		os.RemoveAll(dataDir)
		return nil, fmt.Errorf("failed to parse client URLs: %w", err)
	}
	cfg.AdvertiseClientUrls = cfg.ListenClientUrls

	// Set peer URLs
	peerURL := "http://127.0.0.1:" + strconv.Itoa(peerPort)
	cfg.ListenPeerUrls, err = parseURLs([]string{peerURL})
	if err != nil {
		os.RemoveAll(dataDir)
		return nil, fmt.Errorf("failed to parse peer URLs: %w", err)
	}
	cfg.AdvertisePeerUrls = cfg.ListenPeerUrls

	// Set initial cluster
	cfg.InitialCluster = cfg.Name + "=" + peerURL
	cfg.ClusterState = embed.ClusterStateFlagNew

	// Start etcd server
	e, err := embed.StartEtcd(cfg)
	if err != nil {
		os.RemoveAll(dataDir)
		return nil, fmt.Errorf("failed to start etcd: %w", err)
	}

	// Wait for etcd to be ready
	select {
	case <-e.Server.ReadyNotify():
		// Server is ready
	case <-time.After(10 * time.Second):
		e.Close()
		os.RemoveAll(dataDir)
		return nil, fmt.Errorf("etcd server failed to start within 10 seconds")
	}

	// Create client
	client := v3client.New(e.Server)

	endpoints := []string{clientURL}

	return &EtcdTestServer{
		Server:    e,
		Client:    client,
		Endpoints: endpoints,
		DataDir:   dataDir,
	}, nil
}

// Stop stops the etcd test server and cleans up
func (ets *EtcdTestServer) Stop() error {
	if ets.Client != nil {
		ets.Client.Close()
	}

	if ets.Server != nil {
		ets.Server.Close()
		select {
		case <-ets.Server.Server.StopNotify():
			// Server stopped
		case <-time.After(5 * time.Second):
			// Force stop if it takes too long
		}
	}

	// Clean up data directory
	if ets.DataDir != "" {
		return os.RemoveAll(ets.DataDir)
	}

	return nil
}

// SetupEtcdForTest sets up etcd environment for tests
func SetupEtcdForTest() (*EtcdTestServer, func(), error) {
	server, err := StartEtcdTestServer()
	if err != nil {
		return nil, nil, err
	}

	// Set environment variable for the message queue to use
	originalEndpoints := os.Getenv("ETCD_ENDPOINTS")
	os.Setenv("ETCD_ENDPOINTS", server.Endpoints[0])

	cleanup := func() {
		// Restore original environment
		if originalEndpoints != "" {
			os.Setenv("ETCD_ENDPOINTS", originalEndpoints)
		} else {
			os.Unsetenv("ETCD_ENDPOINTS")
		}

		// Stop server
		if stopErr := server.Stop(); stopErr != nil {
			fmt.Printf("Warning: failed to stop etcd test server: %v\n", stopErr)
		}
	}

	return server, cleanup, nil
}

// Helper functions

func parseURLs(urls []string) ([]url.URL, error) {
	var result []url.URL
	for _, u := range urls {
		parsed, err := url.Parse(u)
		if err != nil {
			return nil, err
		}
		result = append(result, *parsed)
	}
	return result, nil
}

func findAvailablePort() (int, error) {
	// Use net.Listen to find an actually available port
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()

	addr := listener.Addr().(*net.TCPAddr)
	return addr.Port, nil
}

// WaitForEtcdReady waits for etcd to be ready for connections
func WaitForEtcdReady(endpoints []string, timeout time.Duration) error {
	client, err := clientv3.New(clientv3.Config{
		Endpoints:   endpoints,
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		return fmt.Errorf("failed to create etcd client: %w", err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Try to get cluster status
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for etcd to be ready")
		default:
			_, err := client.Status(ctx, endpoints[0])
			if err == nil {
				return nil
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
}
