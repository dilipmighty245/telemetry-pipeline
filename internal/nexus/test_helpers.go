package nexus

import (
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"

	"go.etcd.io/etcd/server/v3/embed"
)

// EmbeddedEtcdServer wraps an embedded etcd server for testing
type EmbeddedEtcdServer struct {
	server    *embed.Etcd
	clientURL string
	dataDir   string
}

// NewEmbeddedEtcdServer creates and starts an embedded etcd server for testing
func NewEmbeddedEtcdServer(t *testing.T) *EmbeddedEtcdServer {
	// Create temporary directory for etcd data
	dataDir, err := os.MkdirTemp("", "etcd-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir for etcd: %v", err)
	}

	// Configure embedded etcd
	cfg := embed.NewConfig()
	cfg.Name = "test-etcd"
	cfg.Dir = dataDir
	cfg.LogLevel = "error" // Reduce log noise in tests
	cfg.Logger = "zap"
	cfg.LogOutputs = []string{"/dev/null"} // Suppress logs during tests

	// Use available ports
	clientURL := "http://127.0.0.1:0" // Let etcd choose an available port
	peerURL := "http://127.0.0.1:0"

	clientURLParsed, err := url.Parse(clientURL)
	if err != nil {
		t.Fatalf("Failed to parse client URL: %v", err)
	}
	peerURLParsed, err := url.Parse(peerURL)
	if err != nil {
		t.Fatalf("Failed to parse peer URL: %v", err)
	}

	cfg.ListenClientUrls = []url.URL{*clientURLParsed}
	cfg.AdvertiseClientUrls = []url.URL{*clientURLParsed}
	cfg.ListenPeerUrls = []url.URL{*peerURLParsed}
	cfg.AdvertisePeerUrls = []url.URL{*peerURLParsed}
	cfg.InitialCluster = fmt.Sprintf("%s=%s", cfg.Name, peerURL)
	cfg.ClusterState = embed.ClusterStateFlagNew

	// Disable auto-compaction for tests
	cfg.AutoCompactionRetention = "0"

	// Create and start etcd server
	server, err := embed.StartEtcd(cfg)
	if err != nil {
		os.RemoveAll(dataDir)
		t.Fatalf("Failed to start embedded etcd: %v", err)
	}

	// Wait for etcd to be ready
	select {
	case <-server.Server.ReadyNotify():
		// Server is ready
	case <-time.After(10 * time.Second):
		server.Close()
		os.RemoveAll(dataDir)
		t.Fatalf("Embedded etcd failed to start within timeout")
	}

	// Get the actual client URL that was assigned
	actualClientURL := server.Clients[0].Addr().String()

	return &EmbeddedEtcdServer{
		server:    server,
		clientURL: fmt.Sprintf("http://%s", actualClientURL),
		dataDir:   dataDir,
	}
}

// GetClientURL returns the client URL for connecting to the embedded etcd server
func (e *EmbeddedEtcdServer) GetClientURL() string {
	return e.clientURL
}

// Close stops the embedded etcd server and cleans up resources
func (e *EmbeddedEtcdServer) Close() {
	if e.server != nil {
		e.server.Close()
	}
	if e.dataDir != "" {
		os.RemoveAll(e.dataDir)
	}
}

// SetupTestEtcd creates an embedded etcd server for testing and returns cleanup function
func SetupTestEtcd(t *testing.T) (clientURL string, cleanup func()) {
	server := NewEmbeddedEtcdServer(t)

	return server.GetClientURL(), func() {
		server.Close()
	}
}
