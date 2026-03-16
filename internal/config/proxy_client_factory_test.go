// Package config provides configuration management for the application.
package config

import (
	"net/http"
	"testing"
	"time"

	auth "github.com/sunbankio/qwencoder-proxy/internal/token"
	"github.com/sunbankio/qwencoder-proxy/logging"
)

// TestNewProxyAwareHTTPClientFactory verifies factory creation with default and custom values.
func TestNewProxyAwareHTTPClientFactory(t *testing.T) {
	logger := logging.NewLogger()
	baseConfig := DefaultConfig().HTTPClient

	// Test with default max size
	factory := NewProxyAwareHTTPClientFactory(baseConfig, logger, 0)
	if factory == nil {
		t.Fatal("Expected factory to not be nil")
	}
	if factory.maxSize != 50 {
		t.Errorf("Expected default maxSize to be 50, got %d", factory.maxSize)
	}

	// Test with custom max size
	factory = NewProxyAwareHTTPClientFactory(baseConfig, logger, 100)
	if factory.maxSize != 100 {
		t.Errorf("Expected maxSize to be 100, got %d", factory.maxSize)
	}
}

// TestGetClientNilConfig verifies that nil proxy config returns direct client.
func TestGetClientNilConfig(t *testing.T) {
	logger := logging.NewLogger()
	baseConfig := DefaultConfig().HTTPClient
	factory := NewProxyAwareHTTPClientFactory(baseConfig, logger, 50)

	client := factory.GetClient(nil)
	if client == nil {
		t.Fatal("Expected client to not be nil")
	}

	// Verify it's a direct connection (no proxy)
	if client.Transport == nil {
		t.Error("Expected transport to be set")
	}
}

// TestGetClientNoneType verifies that ProxyTypeNone returns direct client.
func TestGetClientNoneType(t *testing.T) {
	logger := logging.NewLogger()
	baseConfig := DefaultConfig().HTTPClient
	factory := NewProxyAwareHTTPClientFactory(baseConfig, logger, 50)

	proxyConfig := &auth.ProxyConfig{
		Type: auth.ProxyTypeNone,
	}

	client := factory.GetClient(proxyConfig)
	if client == nil {
		t.Fatal("Expected client to not be nil")
	}
}

// TestGetClientCaching verifies that identical proxy configs return cached clients.
func TestGetClientCaching(t *testing.T) {
	logger := logging.NewLogger()
	baseConfig := DefaultConfig().HTTPClient
	factory := NewProxyAwareHTTPClientFactory(baseConfig, logger, 50)

	proxyConfig1 := &auth.ProxyConfig{
		Type: auth.ProxyTypeHTTP,
		Host: "proxy.example.com",
		Port: 8080,
	}

	proxyConfig2 := &auth.ProxyConfig{
		Type: auth.ProxyTypeHTTP,
		Host: "proxy.example.com",
		Port: 8080,
	}

	client1 := factory.GetClient(proxyConfig1)
	client2 := factory.GetClient(proxyConfig2)

	// Should be the same client instance (cached)
	if client1 != client2 {
		t.Error("Expected cached client to be returned")
	}

	// Cache size should be 1
	if factory.GetCacheSize() != 1 {
		t.Errorf("Expected cache size to be 1, got %d", factory.GetCacheSize())
	}
}

// TestGetClientDifferentConfigs verifies that different proxy configs create different clients.
func TestGetClientDifferentConfigs(t *testing.T) {
	logger := logging.NewLogger()
	baseConfig := DefaultConfig().HTTPClient
	factory := NewProxyAwareHTTPClientFactory(baseConfig, logger, 50)

	proxyConfig1 := &auth.ProxyConfig{
		Type: auth.ProxyTypeHTTP,
		Host: "proxy1.example.com",
		Port: 8080,
	}

	proxyConfig2 := &auth.ProxyConfig{
		Type: auth.ProxyTypeHTTP,
		Host: "proxy2.example.com",
		Port: 8080,
	}

	client1 := factory.GetClient(proxyConfig1)
	client2 := factory.GetClient(proxyConfig2)

	// Should be different clients
	if client1 == client2 {
		t.Error("Expected different clients for different proxy configs")
	}

	// Cache size should be 2
	if factory.GetCacheSize() != 2 {
		t.Errorf("Expected cache size to be 2, got %d", factory.GetCacheSize())
	}
}

// TestGetClientCredentialsExcluded verifies that credentials are excluded from cache key.
func TestGetClientCredentialsExcluded(t *testing.T) {
	logger := logging.NewLogger()
	baseConfig := DefaultConfig().HTTPClient
	factory := NewProxyAwareHTTPClientFactory(baseConfig, logger, 50)

	proxyConfig1 := &auth.ProxyConfig{
		Type:     auth.ProxyTypeHTTP,
		Host:     "proxy.example.com",
		Port:     8080,
		Username: "user1",
		Password: "pass1",
	}

	proxyConfig2 := &auth.ProxyConfig{
		Type:     auth.ProxyTypeHTTP,
		Host:     "proxy.example.com",
		Port:     8080,
		Username: "user2",
		Password: "pass2",
	}

	client1 := factory.GetClient(proxyConfig1)
	client2 := factory.GetClient(proxyConfig2)

	// Should be the same client (credentials excluded from cache key)
	if client1 != client2 {
		t.Error("Expected cached client to be returned (credentials excluded from cache key)")
	}

	// Cache size should be 1
	if factory.GetCacheSize() != 1 {
		t.Errorf("Expected cache size to be 1, got %d", factory.GetCacheSize())
	}
}

// TestCreateClientWithProxy verifies client creation with different proxy types.
func TestCreateClientWithProxy(t *testing.T) {
	logger := logging.NewLogger()
	baseConfig := DefaultConfig().HTTPClient
	factory := NewProxyAwareHTTPClientFactory(baseConfig, logger, 50)

	tests := []struct {
		name    string
		config  *auth.ProxyConfig
		wantErr bool
	}{
		{
			name: "HTTP proxy",
			config: &auth.ProxyConfig{
				Type: auth.ProxyTypeHTTP,
				Host: "proxy.example.com",
				Port: 8080,
			},
			wantErr: false,
		},
		{
			name: "HTTPS proxy",
			config: &auth.ProxyConfig{
				Type: auth.ProxyTypeHTTPS,
				Host: "secure.proxy.com",
				Port: 443,
			},
			wantErr: false,
		},
		{
			name: "SOCKS5 proxy",
			config: &auth.ProxyConfig{
				Type: auth.ProxyTypeSOCKS5,
				Host: "socks.example.com",
				Port: 1080,
			},
			wantErr: false,
		},
		{
			name:    "Nil config",
			config:  nil,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := factory.CreateClientWithProxy(tt.config)
			if client == nil {
				t.Error("Expected client to not be nil")
			}

			// Verify timeout is set
			if client.Timeout == 0 {
				t.Error("Expected timeout to be set")
			}
		})
	}
}

// TestClearCache verifies that cache can be cleared.
func TestClearCache(t *testing.T) {
	logger := logging.NewLogger()
	baseConfig := DefaultConfig().HTTPClient
	factory := NewProxyAwareHTTPClientFactory(baseConfig, logger, 50)

	// Add some clients to cache
	proxyConfig := &auth.ProxyConfig{
		Type: auth.ProxyTypeHTTP,
		Host: "proxy.example.com",
		Port: 8080,
	}

	factory.GetClient(proxyConfig)
	if factory.GetCacheSize() != 1 {
		t.Errorf("Expected cache size to be 1, got %d", factory.GetCacheSize())
	}

	// Clear cache
	factory.ClearCache()
	if factory.GetCacheSize() != 0 {
		t.Errorf("Expected cache size to be 0 after clear, got %d", factory.GetCacheSize())
	}
}

// TestLRUEviction verifies that LRU eviction works correctly.
func TestLRUEviction(t *testing.T) {
	logger := logging.NewLogger()
	baseConfig := DefaultConfig().HTTPClient
	factory := NewProxyAwareHTTPClientFactory(baseConfig, logger, 3) // Small cache size

	// Fill cache to capacity
	for i := 0; i < 3; i++ {
		proxyConfig := &auth.ProxyConfig{
			Type: auth.ProxyTypeHTTP,
			Host: "proxy.example.com",
			Port: 8080 + i,
		}
		factory.GetClient(proxyConfig)
	}

	if factory.GetCacheSize() != 3 {
		t.Errorf("Expected cache size to be 3, got %d", factory.GetCacheSize())
	}

	// Access the first client to make it MRU
	proxyConfig1 := &auth.ProxyConfig{
		Type: auth.ProxyTypeHTTP,
		Host: "proxy.example.com",
		Port: 8080,
	}
	client1 := factory.GetClient(proxyConfig1)

	// Add a fourth client, should evict the LRU (port 8081)
	proxyConfig4 := &auth.ProxyConfig{
		Type: auth.ProxyTypeHTTP,
		Host: "proxy.example.com",
		Port: 8083,
	}
	factory.GetClient(proxyConfig4)

	// Cache size should still be 3
	if factory.GetCacheSize() != 3 {
		t.Errorf("Expected cache size to be 3, got %d", factory.GetCacheSize())
	}

	// First client should still be in cache (was accessed)
	proxyConfig1Check := &auth.ProxyConfig{
		Type: auth.ProxyTypeHTTP,
		Host: "proxy.example.com",
		Port: 8080,
	}
	client1Check := factory.GetClient(proxyConfig1Check)
	if client1 != client1Check {
		t.Error("Expected first client to still be in cache")
	}
}

// TestProxyConfigKeyString verifies string representation of cache keys.
func TestProxyConfigKeyString(t *testing.T) {
	tests := []struct {
		name string
		key  auth.ProxyConfigKey
		want string
	}{
		{
			name: "HTTP proxy key",
			key: auth.ProxyConfigKey{
				Type: auth.ProxyTypeHTTP,
				Host: "proxy.example.com",
				Port: 8080,
			},
			want: "http:proxy.example.com:8080",
		},
		{
			name: "SOCKS5 proxy key",
			key: auth.ProxyConfigKey{
				Type: auth.ProxyTypeSOCKS5,
				Host: "socks.example.com",
				Port: 1080,
			},
			want: "socks5:socks.example.com:1080",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.key.String(); got != tt.want {
				t.Errorf("ProxyConfigKey.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestCreateDirectClient verifies that direct client is created correctly.
func TestCreateDirectClient(t *testing.T) {
	logger := logging.NewLogger()
	baseConfig := DefaultConfig().HTTPClient
	factory := NewProxyAwareHTTPClientFactory(baseConfig, logger, 50)

	client := factory.GetClient(nil)
	if client == nil {
		t.Fatal("Expected client to not be nil")
	}

	// Verify timeout
	expectedTimeout := time.Duration(baseConfig.RequestTimeoutSeconds) * time.Second
	if client.Timeout != expectedTimeout {
		t.Errorf("Expected timeout to be %v, got %v", expectedTimeout, client.Timeout)
	}

	// Verify transport
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatal("Expected transport to be *http.Transport")
	}

	if transport.MaxIdleConns != baseConfig.MaxIdleConns {
		t.Errorf("Expected MaxIdleConns to be %d, got %d", baseConfig.MaxIdleConns, transport.MaxIdleConns)
	}
}

// TestAuthenticatedProxyClient verifies that authenticated proxy clients are created correctly.
func TestAuthenticatedProxyClient(t *testing.T) {
	logger := logging.NewLogger()
	baseConfig := DefaultConfig().HTTPClient
	factory := NewProxyAwareHTTPClientFactory(baseConfig, logger, 50)

	tests := []struct {
		name     string
		config   *auth.ProxyConfig
		wantAuth bool
	}{
		{
			name: "HTTP proxy with authentication",
			config: &auth.ProxyConfig{
				Type:     auth.ProxyTypeHTTP,
				Host:     "proxy.example.com",
				Port:     8080,
				Username: "testuser",
				Password: "testpass",
			},
			wantAuth: true,
		},
		{
			name: "HTTPS proxy with authentication",
			config: &auth.ProxyConfig{
				Type:     auth.ProxyTypeHTTPS,
				Host:     "secure.proxy.com",
				Port:     443,
				Username: "secureuser",
				Password: "securepass",
			},
			wantAuth: true,
		},
		{
			name: "SOCKS5 proxy with authentication",
			config: &auth.ProxyConfig{
				Type:     auth.ProxyTypeSOCKS5,
				Host:     "socks.example.com",
				Port:     1080,
				Username: "socksuser",
				Password: "sockspass",
			},
			wantAuth: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := factory.CreateClientWithProxy(tt.config)
			if client == nil {
				t.Fatal("Expected client to not be nil")
			}

			// Verify client has transport configured
			if client.Transport == nil {
				t.Error("Expected transport to be set")
			}

			transport, ok := client.Transport.(*http.Transport)
			if !ok {
				t.Fatal("Expected transport to be *http.Transport")
			}

			// Verify timeout is set
			if client.Timeout == 0 {
				t.Error("Expected timeout to be set")
			}

			// Verify connection pooling is configured
			if transport.MaxIdleConns != baseConfig.MaxIdleConns {
				t.Errorf("Expected MaxIdleConns to be %d, got %d", baseConfig.MaxIdleConns, transport.MaxIdleConns)
			}
		})
	}
}

// TestUnauthenticatedProxyClient verifies that unauthenticated proxy clients are created correctly.
func TestUnauthenticatedProxyClient(t *testing.T) {
	logger := logging.NewLogger()
	baseConfig := DefaultConfig().HTTPClient
	factory := NewProxyAwareHTTPClientFactory(baseConfig, logger, 50)

	tests := []struct {
		name   string
		config *auth.ProxyConfig
	}{
		{
			name: "HTTP proxy without authentication",
			config: &auth.ProxyConfig{
				Type: auth.ProxyTypeHTTP,
				Host: "proxy.example.com",
				Port: 8080,
			},
		},
		{
			name: "HTTPS proxy without authentication",
			config: &auth.ProxyConfig{
				Type: auth.ProxyTypeHTTPS,
				Host: "secure.proxy.com",
				Port: 443,
			},
		},
		{
			name: "SOCKS5 proxy without authentication",
			config: &auth.ProxyConfig{
				Type: auth.ProxyTypeSOCKS5,
				Host: "socks.example.com",
				Port: 1080,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := factory.CreateClientWithProxy(tt.config)
			if client == nil {
				t.Fatal("Expected client to not be nil")
			}

			// Verify client has transport configured
			if client.Transport == nil {
				t.Error("Expected transport to be set")
			}

			// Verify timeout is set
			if client.Timeout == 0 {
				t.Error("Expected timeout to be set")
			}

			// Verify transport type
			_, ok := client.Transport.(*http.Transport)
			if !ok {
				t.Fatal("Expected transport to be *http.Transport")
			}
		})
	}
}

// TestNewClientAfterClearing verifies that new clients can be created after cache is cleared.
func TestNewClientAfterClearing(t *testing.T) {
	logger := logging.NewLogger()
	baseConfig := DefaultConfig().HTTPClient
	factory := NewProxyAwareHTTPClientFactory(baseConfig, logger, 50)

	// Add a client to cache
	proxyConfig := &auth.ProxyConfig{
		Type: auth.ProxyTypeHTTP,
		Host: "proxy.example.com",
		Port: 8080,
	}

	client1 := factory.GetClient(proxyConfig)
	if factory.GetCacheSize() != 1 {
		t.Errorf("Expected cache size to be 1, got %d", factory.GetCacheSize())
	}

	// Clear cache
	factory.ClearCache()
	if factory.GetCacheSize() != 0 {
		t.Errorf("Expected cache size to be 0 after clear, got %d", factory.GetCacheSize())
	}

	// Create a new client after clearing
	client2 := factory.GetClient(proxyConfig)
	if client2 == nil {
		t.Fatal("Expected client to not be nil after clearing cache")
	}

	// Cache size should be 1 again
	if factory.GetCacheSize() != 1 {
		t.Errorf("Expected cache size to be 1 after new client, got %d", factory.GetCacheSize())
	}

	// The new client should be different from the old one
	if client1 == client2 {
		t.Error("Expected new client to be different from old client after cache clear")
	}
}

// TestLRUEvictionOrder verifies that the least recently used client is evicted first.
func TestLRUEvictionOrder(t *testing.T) {
	logger := logging.NewLogger()
	baseConfig := DefaultConfig().HTTPClient
	factory := NewProxyAwareHTTPClientFactory(baseConfig, logger, 3) // Small cache size

	// Create three clients in order
	config1 := &auth.ProxyConfig{
		Type: auth.ProxyTypeHTTP,
		Host: "proxy.example.com",
		Port: 8080,
	}
	config2 := &auth.ProxyConfig{
		Type: auth.ProxyTypeHTTP,
		Host: "proxy.example.com",
		Port: 8081,
	}
	config3 := &auth.ProxyConfig{
		Type: auth.ProxyTypeHTTP,
		Host: "proxy.example.com",
		Port: 8082,
	}

	client1 := factory.GetClient(config1)
	client2 := factory.GetClient(config2)
	client3 := factory.GetClient(config3)

	// Cache should be full
	if factory.GetCacheSize() != 3 {
		t.Errorf("Expected cache size to be 3, got %d", factory.GetCacheSize())
	}

	// Access config2 to make it MRU (config1 is now LRU)
	_ = factory.GetClient(config2)

	// Add a fourth config, should evict config1 (LRU)
	config4 := &auth.ProxyConfig{
		Type: auth.ProxyTypeHTTP,
		Host: "proxy.example.com",
		Port: 8083,
	}
	_ = factory.GetClient(config4)

	// Cache should still be size 3
	if factory.GetCacheSize() != 3 {
		t.Errorf("Expected cache size to be 3, got %d", factory.GetCacheSize())
	}

	// config1 should be evicted (new client should be different)
	client1Check := factory.GetClient(config1)
	if client1 == client1Check {
		t.Error("Expected config1 client to be evicted and recreated")
	}

	// config2 should still be cached (was accessed)
	client2Check := factory.GetClient(config2)
	if client2 != client2Check {
		t.Error("Expected config2 client to still be cached")
	}

	// config3 should have been evicted (was second LRU)
	client3Check := factory.GetClient(config3)
	if client3 == client3Check {
		t.Error("Expected config3 client to be evicted and recreated")
	}
}

// TestAllClientsEvictedWhenFull verifies that all clients are evicted when cache fills.
func TestAllClientsEvictedWhenFull(t *testing.T) {
	logger := logging.NewLogger()
	baseConfig := DefaultConfig().HTTPClient
	factory := NewProxyAwareHTTPClientFactory(baseConfig, logger, 2) // Very small cache

	// Create three clients with different configs
	var clients []*http.Client
	for i := 0; i < 5; i++ {
		config := &auth.ProxyConfig{
			Type: auth.ProxyTypeHTTP,
			Host: "proxy.example.com",
			Port: 8080 + i,
		}
		client := factory.GetClient(config)
		clients = append(clients, client)
	}

	// Cache should be at max size
	if factory.GetCacheSize() != 2 {
		t.Errorf("Expected cache size to be 2, got %d", factory.GetCacheSize())
	}

	// The first three clients should have been evicted
	config0 := &auth.ProxyConfig{
		Type: auth.ProxyTypeHTTP,
		Host: "proxy.example.com",
		Port: 8080,
	}
	config1 := &auth.ProxyConfig{
		Type: auth.ProxyTypeHTTP,
		Host: "proxy.example.com",
		Port: 8081,
	}
	config2 := &auth.ProxyConfig{
		Type: auth.ProxyTypeHTTP,
		Host: "proxy.example.com",
		Port: 8082,
	}

	client0Check := factory.GetClient(config0)
	client1Check := factory.GetClient(config1)
	client2Check := factory.GetClient(config2)

	// First three should be different from original (evicted)
	if clients[0] == client0Check {
		t.Error("Expected first client to be evicted")
	}
	if clients[1] == client1Check {
		t.Error("Expected second client to be evicted")
	}
	if clients[2] == client2Check {
		t.Error("Expected third client to be evicted")
	}
}

// TestNewClientAfterEviction verifies that new clients are added after eviction.
func TestNewClientAfterEviction(t *testing.T) {
	logger := logging.NewLogger()
	baseConfig := DefaultConfig().HTTPClient
	factory := NewProxyAwareHTTPClientFactory(baseConfig, logger, 2) // Very small cache

	// Fill cache
	config1 := &auth.ProxyConfig{
		Type: auth.ProxyTypeHTTP,
		Host: "proxy.example.com",
		Port: 8080,
	}
	config2 := &auth.ProxyConfig{
		Type: auth.ProxyTypeHTTP,
		Host: "proxy.example.com",
		Port: 8081,
	}

	client1 := factory.GetClient(config1)
	client2 := factory.GetClient(config2)

	if factory.GetCacheSize() != 2 {
		t.Errorf("Expected cache size to be 2, got %d", factory.GetCacheSize())
	}

	// Add a third config, should evict the first
	config3 := &auth.ProxyConfig{
		Type: auth.ProxyTypeHTTP,
		Host: "proxy.example.com",
		Port: 8082,
	}
	client3 := factory.GetClient(config3)

	// Cache should still be size 2
	if factory.GetCacheSize() != 2 {
		t.Errorf("Expected cache size to be 2, got %d", factory.GetCacheSize())
	}

	// New client should be added
	if client3 == nil {
		t.Error("Expected new client to be added after eviction")
	}

	// Verify client3 is different from client1 and client2
	if client3 == client1 || client3 == client2 {
		t.Error("Expected new client to be different from existing clients")
	}
}

// TestTimeoutConfigurationApplied verifies that timeout configuration is applied to clients.
func TestTimeoutConfigurationApplied(t *testing.T) {
	logger := logging.NewLogger()
	baseConfig := DefaultConfig().HTTPClient
	baseConfig.RequestTimeoutSeconds = 60
	factory := NewProxyAwareHTTPClientFactory(baseConfig, logger, 50)

	tests := []struct {
		name   string
		config *auth.ProxyConfig
	}{
		{
			name:   "Direct client timeout",
			config: nil,
		},
		{
			name: "HTTP proxy client timeout",
			config: &auth.ProxyConfig{
				Type: auth.ProxyTypeHTTP,
				Host: "proxy.example.com",
				Port: 8080,
			},
		},
		{
			name: "HTTPS proxy client timeout",
			config: &auth.ProxyConfig{
				Type: auth.ProxyTypeHTTPS,
				Host: "secure.proxy.com",
				Port: 443,
			},
		},
		{
			name: "SOCKS5 proxy client timeout",
			config: &auth.ProxyConfig{
				Type: auth.ProxyTypeSOCKS5,
				Host: "socks.example.com",
				Port: 1080,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := factory.CreateClientWithProxy(tt.config)
			if client == nil {
				t.Fatal("Expected client to not be nil")
			}

			expectedTimeout := time.Duration(baseConfig.RequestTimeoutSeconds) * time.Second
			if client.Timeout != expectedTimeout {
				t.Errorf("Expected timeout to be %v, got %v", expectedTimeout, client.Timeout)
			}
		})
	}
}

// TestConnectionPoolingConfigured verifies that connection pooling is configured correctly.
func TestConnectionPoolingConfigured(t *testing.T) {
	logger := logging.NewLogger()
	baseConfig := DefaultConfig().HTTPClient
	baseConfig.MaxIdleConns = 100
	baseConfig.MaxIdleConnsPerHost = 50
	factory := NewProxyAwareHTTPClientFactory(baseConfig, logger, 50)

	tests := []struct {
		name   string
		config *auth.ProxyConfig
	}{
		{
			name:   "Direct client connection pooling",
			config: nil,
		},
		{
			name: "HTTP proxy client connection pooling",
			config: &auth.ProxyConfig{
				Type: auth.ProxyTypeHTTP,
				Host: "proxy.example.com",
				Port: 8080,
			},
		},
		{
			name: "SOCKS5 proxy client connection pooling",
			config: &auth.ProxyConfig{
				Type: auth.ProxyTypeSOCKS5,
				Host: "socks.example.com",
				Port: 1080,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := factory.CreateClientWithProxy(tt.config)
			if client == nil {
				t.Fatal("Expected client to not be nil")
			}

			transport, ok := client.Transport.(*http.Transport)
			if !ok {
				t.Fatal("Expected transport to be *http.Transport")
			}

			if transport.MaxIdleConns != baseConfig.MaxIdleConns {
				t.Errorf("Expected MaxIdleConns to be %d, got %d", baseConfig.MaxIdleConns, transport.MaxIdleConns)
			}

			if transport.MaxIdleConnsPerHost != baseConfig.MaxIdleConnsPerHost {
				t.Errorf("Expected MaxIdleConnsPerHost to be %d, got %d", baseConfig.MaxIdleConnsPerHost, transport.MaxIdleConnsPerHost)
			}

			// MaxConnsPerHost should be 0 (no limit)
			if transport.MaxConnsPerHost != 0 {
				t.Errorf("Expected MaxConnsPerHost to be 0, got %d", transport.MaxConnsPerHost)
			}
		})
	}
}

// TestIdleConnectionTimeoutSet verifies that idle connection timeout is configured.
func TestIdleConnectionTimeoutSet(t *testing.T) {
	logger := logging.NewLogger()
	baseConfig := DefaultConfig().HTTPClient
	baseConfig.IdleConnTimeoutSeconds = 90
	factory := NewProxyAwareHTTPClientFactory(baseConfig, logger, 50)

	tests := []struct {
		name   string
		config *auth.ProxyConfig
	}{
		{
			name:   "Direct client idle timeout",
			config: nil,
		},
		{
			name: "HTTP proxy client idle timeout",
			config: &auth.ProxyConfig{
				Type: auth.ProxyTypeHTTP,
				Host: "proxy.example.com",
				Port: 8080,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := factory.CreateClientWithProxy(tt.config)
			if client == nil {
				t.Fatal("Expected client to not be nil")
			}

			transport, ok := client.Transport.(*http.Transport)
			if !ok {
				t.Fatal("Expected transport to be *http.Transport")
			}

			expectedTimeout := time.Duration(baseConfig.IdleConnTimeoutSeconds) * time.Second
			if transport.IdleConnTimeout != expectedTimeout {
				t.Errorf("Expected IdleConnTimeout to be %v, got %v", expectedTimeout, transport.IdleConnTimeout)
			}
		})
	}
}

// TestMaxIdleConnectionsSet verifies that max idle connections is configured.
func TestMaxIdleConnectionsSet(t *testing.T) {
	logger := logging.NewLogger()
	baseConfig := DefaultConfig().HTTPClient
	baseConfig.MaxIdleConns = 200
	factory := NewProxyAwareHTTPClientFactory(baseConfig, logger, 50)

	tests := []struct {
		name   string
		config *auth.ProxyConfig
	}{
		{
			name:   "Direct client max idle connections",
			config: nil,
		},
		{
			name: "HTTP proxy client max idle connections",
			config: &auth.ProxyConfig{
				Type: auth.ProxyTypeHTTP,
				Host: "proxy.example.com",
				Port: 8080,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := factory.CreateClientWithProxy(tt.config)
			if client == nil {
				t.Fatal("Expected client to not be nil")
			}

			transport, ok := client.Transport.(*http.Transport)
			if !ok {
				t.Fatal("Expected transport to be *http.Transport")
			}

			if transport.MaxIdleConns != baseConfig.MaxIdleConns {
				t.Errorf("Expected MaxIdleConns to be %d, got %d", baseConfig.MaxIdleConns, transport.MaxIdleConns)
			}
		})
	}
}

// TestInvalidProxyTypeError verifies that invalid proxy types are handled gracefully.
func TestInvalidProxyTypeError(t *testing.T) {
	logger := logging.NewLogger()
	baseConfig := DefaultConfig().HTTPClient
	factory := NewProxyAwareHTTPClientFactory(baseConfig, logger, 50)

	// Create a config with an invalid proxy type
	proxyConfig := &auth.ProxyConfig{
		Type: auth.ProxyType("invalid"),
		Host: "proxy.example.com",
		Port: 8080,
	}

	// Should return a direct client instead of failing
	client := factory.CreateClientWithProxy(proxyConfig)
	if client == nil {
		t.Fatal("Expected client to not be nil")
	}

	// Verify it's a direct connection (no proxy)
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatal("Expected transport to be *http.Transport")
	}

	// Direct connection should not have a proxy configured
	if transport.Proxy != nil {
		t.Error("Expected no proxy for invalid proxy type")
	}
}

// TestInvalidHostError verifies that invalid hosts are handled.
func TestInvalidHostError(t *testing.T) {
	logger := logging.NewLogger()
	baseConfig := DefaultConfig().HTTPClient
	factory := NewProxyAwareHTTPClientFactory(baseConfig, logger, 50)

	tests := []struct {
		name   string
		config *auth.ProxyConfig
	}{
		{
			name: "Empty host",
			config: &auth.ProxyConfig{
				Type: auth.ProxyTypeHTTP,
				Host: "",
				Port: 8080,
			},
		},
		{
			name: "Host with spaces",
			config: &auth.ProxyConfig{
				Type: auth.ProxyTypeHTTP,
				Host: "invalid host",
				Port: 8080,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// The factory should still create a client (it may fail at runtime)
			client := factory.CreateClientWithProxy(tt.config)
			if client == nil {
				t.Fatal("Expected client to not be nil")
			}
		})
	}
}

// TestInvalidPortError verifies that invalid ports are handled.
func TestInvalidPortError(t *testing.T) {
	logger := logging.NewLogger()
	baseConfig := DefaultConfig().HTTPClient
	factory := NewProxyAwareHTTPClientFactory(baseConfig, logger, 50)

	tests := []struct {
		name   string
		config *auth.ProxyConfig
	}{
		{
			name: "Port 0",
			config: &auth.ProxyConfig{
				Type: auth.ProxyTypeHTTP,
				Host: "proxy.example.com",
				Port: 0,
			},
		},
		{
			name: "Negative port",
			config: &auth.ProxyConfig{
				Type: auth.ProxyTypeHTTP,
				Host: "proxy.example.com",
				Port: -1,
			},
		},
		{
			name: "Port out of range",
			config: &auth.ProxyConfig{
				Type: auth.ProxyTypeHTTP,
				Host: "proxy.example.com",
				Port: 70000,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// The factory should still create a client (it may fail at runtime)
			client := factory.CreateClientWithProxy(tt.config)
			if client == nil {
				t.Fatal("Expected client to not be nil")
			}
		})
	}
}

// TestInvalidSOCKS5ConfigError verifies that invalid SOCKS5 configurations are handled.
func TestInvalidSOCKS5ConfigError(t *testing.T) {
	logger := logging.NewLogger()
	baseConfig := DefaultConfig().HTTPClient
	factory := NewProxyAwareHTTPClientFactory(baseConfig, logger, 50)

	tests := []struct {
		name   string
		config *auth.ProxyConfig
	}{
		{
			name: "SOCKS5 with empty host",
			config: &auth.ProxyConfig{
				Type: auth.ProxyTypeSOCKS5,
				Host: "",
				Port: 1080,
			},
		},
		{
			name: "SOCKS5 with invalid port",
			config: &auth.ProxyConfig{
				Type: auth.ProxyTypeSOCKS5,
				Host: "socks.example.com",
				Port: 0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// The factory should still create a client (it may fail at runtime)
			client := factory.CreateClientWithProxy(tt.config)
			if client == nil {
				t.Fatal("Expected client to not be nil")
			}
		})
	}
}

// TestPortBoundaryValues tests boundary values for port numbers.
func TestPortBoundaryValues(t *testing.T) {
	logger := logging.NewLogger()
	baseConfig := DefaultConfig().HTTPClient
	factory := NewProxyAwareHTTPClientFactory(baseConfig, logger, 50)

	tests := []struct {
		name  string
		port  int
		valid bool
	}{
		{"Port 1 (minimum valid)", 1, true},
		{"Port 80 (common HTTP)", 80, true},
		{"Port 443 (common HTTPS)", 443, true},
		{"Port 1080 (common SOCKS)", 1080, true},
		{"Port 3128 (common proxy)", 3128, true},
		{"Port 8080 (common proxy)", 8080, true},
		{"Port 65535 (maximum valid)", 65535, true},
		{"Port 0 (invalid)", 0, false},
		{"Port -1 (invalid)", -1, false},
		{"Port 65536 (invalid)", 65536, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &auth.ProxyConfig{
				Type: auth.ProxyTypeHTTP,
				Host: "proxy.example.com",
				Port: tt.port,
			}

			client := factory.CreateClientWithProxy(config)
			if client == nil {
				t.Fatal("Expected client to not be nil")
			}

			// Verify client was created
			if client.Transport == nil {
				t.Error("Expected transport to be set")
			}
		})
	}
}

// TestEmptyStringsForOptionalFields tests that empty strings for optional fields work correctly.
func TestEmptyStringsForOptionalFields(t *testing.T) {
	logger := logging.NewLogger()
	baseConfig := DefaultConfig().HTTPClient
	factory := NewProxyAwareHTTPClientFactory(baseConfig, logger, 50)

	tests := []struct {
		name     string
		config   *auth.ProxyConfig
		wantAuth bool
	}{
		{
			name: "Empty username and password",
			config: &auth.ProxyConfig{
				Type:     auth.ProxyTypeHTTP,
				Host:     "proxy.example.com",
				Port:     8080,
				Username: "",
				Password: "",
			},
			wantAuth: false,
		},
		{
			name: "Only username provided (invalid but should not crash)",
			config: &auth.ProxyConfig{
				Type:     auth.ProxyTypeHTTP,
				Host:     "proxy.example.com",
				Port:     8080,
				Username: "testuser",
				Password: "",
			},
			wantAuth: false,
		},
		{
			name: "Only password provided (invalid but should not crash)",
			config: &auth.ProxyConfig{
				Type:     auth.ProxyTypeHTTP,
				Host:     "proxy.example.com",
				Port:     8080,
				Username: "",
				Password: "testpass",
			},
			wantAuth: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := factory.CreateClientWithProxy(tt.config)
			if client == nil {
				t.Fatal("Expected client to not be nil")
			}
		})
	}
}

// TestSpecialCharactersInHost tests that special characters in host names are handled.
func TestSpecialCharactersInHost(t *testing.T) {
	logger := logging.NewLogger()
	baseConfig := DefaultConfig().HTTPClient
	factory := NewProxyAwareHTTPClientFactory(baseConfig, logger, 50)

	tests := []struct {
		name string
		host string
	}{
		{"Host with hyphen", "proxy-server.example.com"},
		{"Host with underscore", "proxy_server.example.com"},
		{"Host with numbers", "proxy123.example.com"},
		{"Subdomain", "proxy.sub.example.com"},
		{"IP address", "192.168.1.1"},
		{"IPv6 address", "[::1]"},
		{"Host with dots", "proxy.example.co.uk"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &auth.ProxyConfig{
				Type: auth.ProxyTypeHTTP,
				Host: tt.host,
				Port: 8080,
			}

			client := factory.CreateClientWithProxy(config)
			if client == nil {
				t.Fatal("Expected client to not be nil")
			}
		})
	}
}

// TestUnicodeCharactersInCredentials tests that Unicode characters in credentials are handled.
func TestUnicodeCharactersInCredentials(t *testing.T) {
	logger := logging.NewLogger()
	baseConfig := DefaultConfig().HTTPClient
	factory := NewProxyAwareHTTPClientFactory(baseConfig, logger, 50)

	tests := []struct {
		name     string
		username string
		password string
	}{
		{"Unicode username", "useré", "password"},
		{"Unicode password", "user", "pässwörd"},
		{"Emojis in password", "user", "pass🔑word"},
		{"Mixed unicode", "用户", "パスワード"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &auth.ProxyConfig{
				Type:     auth.ProxyTypeHTTP,
				Host:     "proxy.example.com",
				Port:     8080,
				Username: tt.username,
				Password: tt.password,
			}

			client := factory.CreateClientWithProxy(config)
			if client == nil {
				t.Fatal("Expected client to not be nil")
			}
		})
	}
}

// TestLargeCacheSizes tests that large cache sizes work correctly.
func TestLargeCacheSizes(t *testing.T) {
	logger := logging.NewLogger()
	baseConfig := DefaultConfig().HTTPClient
	factory := NewProxyAwareHTTPClientFactory(baseConfig, logger, 1000) // Large cache

	// Create many clients
	numClients := 500
	for i := 0; i < numClients; i++ {
		config := &auth.ProxyConfig{
			Type: auth.ProxyTypeHTTP,
			Host: "proxy.example.com",
			Port: 8080 + i,
		}
		factory.GetClient(config)
	}

	// Cache should contain all clients
	if factory.GetCacheSize() != numClients {
		t.Errorf("Expected cache size to be %d, got %d", numClients, factory.GetCacheSize())
	}

	// Clear cache and verify it's empty
	factory.ClearCache()
	if factory.GetCacheSize() != 0 {
		t.Errorf("Expected cache size to be 0 after clear, got %d", factory.GetCacheSize())
	}
}

// TestConcurrentAccessToCache tests that concurrent access to the cache is safe.
func TestConcurrentAccessToCache(t *testing.T) {
	logger := logging.NewLogger()
	baseConfig := DefaultConfig().HTTPClient
	factory := NewProxyAwareHTTPClientFactory(baseConfig, logger, 100)

	done := make(chan bool)
	numGoroutines := 50
	iterationsPerGoroutine := 100

	// Launch multiple goroutines to access the cache concurrently
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			for j := 0; j < iterationsPerGoroutine; j++ {
				config := &auth.ProxyConfig{
					Type: auth.ProxyTypeHTTP,
					Host: "proxy.example.com",
					Port: 8080 + (j % 10), // Use only 10 different configs
				}
				client := factory.GetClient(config)
				if client == nil {
					t.Errorf("Goroutine %d: Expected client to not be nil", id)
				}
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	// Cache should contain at most 10 clients (the number of unique configs)
	cacheSize := factory.GetCacheSize()
	if cacheSize > 10 {
		t.Errorf("Expected cache size to be at most 10, got %d", cacheSize)
	}
	if cacheSize == 0 {
		t.Error("Expected cache to contain some clients")
	}
}

// TestGetClientWithUnknownProxyType verifies that unknown proxy types return direct client.
func TestGetClientWithUnknownProxyType(t *testing.T) {
	logger := logging.NewLogger()
	baseConfig := DefaultConfig().HTTPClient
	factory := NewProxyAwareHTTPClientFactory(baseConfig, logger, 50)

	proxyConfig := &auth.ProxyConfig{
		Type: auth.ProxyType("unknown"),
		Host: "proxy.example.com",
		Port: 8080,
	}

	client := factory.GetClient(proxyConfig)
	if client == nil {
		t.Fatal("Expected client to not be nil")
	}

	// Verify it's a direct connection (no proxy)
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatal("Expected transport to be *http.Transport")
	}

	if transport.Proxy != nil {
		t.Error("Expected no proxy for unknown proxy type")
	}
}

// TestGetCacheSizeWithEmptyCache verifies cache size for empty cache.
func TestGetCacheSizeWithEmptyCache(t *testing.T) {
	logger := logging.NewLogger()
	baseConfig := DefaultConfig().HTTPClient
	factory := NewProxyAwareHTTPClientFactory(baseConfig, logger, 50)

	if factory.GetCacheSize() != 0 {
		t.Errorf("Expected cache size to be 0 for empty cache, got %d", factory.GetCacheSize())
	}
}

// TestClearCacheWithEmptyCache verifies that clearing an empty cache doesn't panic.
func TestClearCacheWithEmptyCache(t *testing.T) {
	logger := logging.NewLogger()
	baseConfig := DefaultConfig().HTTPClient
	factory := NewProxyAwareHTTPClientFactory(baseConfig, logger, 50)

	// Should not panic
	factory.ClearCache()

	if factory.GetCacheSize() != 0 {
		t.Errorf("Expected cache size to be 0, got %d", factory.GetCacheSize())
	}
}

// TestProxyConfigKeyEquality verifies that ProxyConfigKey equality works correctly.
func TestProxyConfigKeyEquality(t *testing.T) {
	tests := []struct {
		name string
		key1 auth.ProxyConfigKey
		key2 auth.ProxyConfigKey
		want bool
	}{
		{
			name: "Equal keys",
			key1: auth.ProxyConfigKey{
				Type: auth.ProxyTypeHTTP,
				Host: "proxy.example.com",
				Port: 8080,
			},
			key2: auth.ProxyConfigKey{
				Type: auth.ProxyTypeHTTP,
				Host: "proxy.example.com",
				Port: 8080,
			},
			want: true,
		},
		{
			name: "Different type",
			key1: auth.ProxyConfigKey{
				Type: auth.ProxyTypeHTTP,
				Host: "proxy.example.com",
				Port: 8080,
			},
			key2: auth.ProxyConfigKey{
				Type: auth.ProxyTypeHTTPS,
				Host: "proxy.example.com",
				Port: 8080,
			},
			want: false,
		},
		{
			name: "Different host",
			key1: auth.ProxyConfigKey{
				Type: auth.ProxyTypeHTTP,
				Host: "proxy1.example.com",
				Port: 8080,
			},
			key2: auth.ProxyConfigKey{
				Type: auth.ProxyTypeHTTP,
				Host: "proxy2.example.com",
				Port: 8080,
			},
			want: false,
		},
		{
			name: "Different port",
			key1: auth.ProxyConfigKey{
				Type: auth.ProxyTypeHTTP,
				Host: "proxy.example.com",
				Port: 8080,
			},
			key2: auth.ProxyConfigKey{
				Type: auth.ProxyTypeHTTP,
				Host: "proxy.example.com",
				Port: 8081,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.key1 == tt.key2
			if got != tt.want {
				t.Errorf("ProxyConfigKey equality = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestNewProxyConfigKey verifies NewProxyConfigKey function behavior.
func TestNewProxyConfigKey(t *testing.T) {
	tests := []struct {
		name    string
		config  *auth.ProxyConfig
		wantNil bool
	}{
		{
			name:    "Nil config",
			config:  nil,
			wantNil: true,
		},
		{
			name: "ProxyTypeNone",
			config: &auth.ProxyConfig{
				Type: auth.ProxyTypeNone,
				Host: "proxy.example.com",
				Port: 8080,
			},
			wantNil: true,
		},
		{
			name: "Valid HTTP config",
			config: &auth.ProxyConfig{
				Type: auth.ProxyTypeHTTP,
				Host: "proxy.example.com",
				Port: 8080,
			},
			wantNil: false,
		},
		{
			name: "Valid SOCKS5 config",
			config: &auth.ProxyConfig{
				Type: auth.ProxyTypeSOCKS5,
				Host: "socks.example.com",
				Port: 1080,
			},
			wantNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := auth.NewProxyConfigKey(tt.config)
			if (key == nil) != tt.wantNil {
				t.Errorf("NewProxyConfigKey() = %v, wantNil %v", key, tt.wantNil)
			}
		})
	}
}

// TestGetClientWithNilKey verifies that GetClient handles nil key gracefully.
func TestGetClientWithNilKey(t *testing.T) {
	logger := logging.NewLogger()
	baseConfig := DefaultConfig().HTTPClient
	factory := NewProxyAwareHTTPClientFactory(baseConfig, logger, 50)

	// Create a config that will result in nil key (ProxyTypeNone)
	proxyConfig := &auth.ProxyConfig{
		Type: auth.ProxyTypeNone,
		Host: "",
		Port: 0,
	}

	client := factory.GetClient(proxyConfig)
	if client == nil {
		t.Fatal("Expected client to not be nil")
	}

	// Should return a direct client
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatal("Expected transport to be *http.Transport")
	}

	if transport.Proxy != nil {
		t.Error("Expected no proxy for ProxyTypeNone")
	}
}
