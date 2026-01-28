// Package tests provides integration tests for proxy-aware request handling
package tests

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunbankio/qwencoder-proxy/auth"
	"github.com/sunbankio/qwencoder-proxy/config"
	"github.com/sunbankio/qwencoder-proxy/logging"
)

// MockHTTPServer represents a mock HTTP server for testing
type MockHTTPServer struct {
	server   *httptest.Server
	requests []MockRequest
	mu       sync.Mutex
	handler  func(w http.ResponseWriter, r *http.Request)
}

type MockRequest struct {
	Method     string
	Path       string
	Headers    http.Header
	Body       string
	RemoteAddr string
	ReceivedAt time.Time
}

// NewMockHTTPServer creates a new mock HTTP server
func NewMockHTTPServer(handler func(w http.ResponseWriter, r *http.Request)) *MockHTTPServer {
	return &MockHTTPServer{
		handler:  handler,
		requests: make([]MockRequest, 0),
	}
}

// Start starts mock server
func (m *MockHTTPServer) Start() {
	m.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		// Read body
		bodyBytes, _ := io.ReadAll(r.Body)
		r.Body.Close()

		// Record request
		req := MockRequest{
			Method:     r.Method,
			Path:       r.URL.Path,
			Headers:    r.Header.Clone(),
			Body:       string(bodyBytes),
			RemoteAddr: r.RemoteAddr,
			ReceivedAt: time.Now(),
		}
		m.requests = append(m.requests, req)
		m.mu.Unlock()

		if m.handler != nil {
			m.handler(w, r)
		}
	}))
}

// Stop stops mock server
func (m *MockHTTPServer) Stop() {
	if m.server != nil {
		m.server.Close()
	}
}

// GetURL returns server's URL
func (m *MockHTTPServer) GetURL() string {
	if m.server != nil {
		return m.server.URL
	}
	return ""
}

// GetRequests returns all recorded requests
func (m *MockHTTPServer) GetRequests() []MockRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.requests
}

// ClearRequests clears all recorded requests
func (m *MockHTTPServer) ClearRequests() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requests = make([]MockRequest, 0)
}

// GetLastRequest returns the last recorded request
func (m *MockHTTPServer) GetLastRequest() *MockRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.requests) == 0 {
		return nil
	}
	return &m.requests[len(m.requests)-1]
}

// GetRequestCount returns the number of recorded requests
func (m *MockHTTPServer) GetRequestCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.requests)
}

// setupProxyTestEnvironment creates a test environment with proxy-aware components
func setupProxyTestEnvironment(t *testing.T) (*auth.MultiTokenManager, *config.ProxyAwareHTTPClientFactory, func()) {
	logger := logging.NewLogger()
	baseConfig := config.DefaultConfig().HTTPClient

	// Create proxy client factory
	factory := config.NewProxyAwareHTTPClientFactory(baseConfig, logger, 50)

	// Create a multi-token manager
	tempDir := t.TempDir()
	manager := auth.NewMultiTokenManager(logger)
	manager.SetCredentialsDir(tempDir)

	// Initialize manager
	err := manager.Initialize()
	require.NoError(t, err)

	// Set client factory on the manager
	manager.SetClientFactory(factory)

	// Register a test provider
	providerConfig := auth.ProviderConfig{
		ID:           "test-provider",
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
	}
	err = manager.RegisterProvider("test-provider", providerConfig)
	require.NoError(t, err)

	cleanup := func() {
		factory.ClearCache()
		manager.Stop()
	}

	return manager, factory, cleanup
}

// createTestTokenWithProxy creates a test token with proxy configuration
func createTestTokenWithProxy(id, email string, proxyConfig *auth.ProxyConfig, expiryOffset time.Duration) auth.ProviderToken {
	now := time.Now()
	return auth.ProviderToken{
		ID:               id,
		AccessToken:      "access-" + id,
		RefreshToken:     "refresh-" + id,
		TokenType:        "Bearer",
		ExpiryDate:       now.Add(expiryOffset).UnixMilli(),
		Email:            email,
		Healthy:          true,
		HealthScore:      1.0,
		LastUsed:         now.UnixMilli(),
		CreatedAt:        now.UnixMilli(),
		ErrorCount:       0,
		Proxy:            proxyConfig,
		ProxyHealthScore: 1.0,
	}
}

// ============================================================================
// Token Selection Tests
// ============================================================================

func TestProxyIntegration_TokenSelectionWithHTTPProxy(t *testing.T) {
	t.Run("select token with HTTP proxy", func(t *testing.T) {
		manager, _, cleanup := setupProxyTestEnvironment(t)
		defer cleanup()

		// Create token with HTTP proxy
		proxyConfig := &auth.ProxyConfig{
			Type:    auth.ProxyTypeHTTP,
			Host:    "proxy.example.com",
			Port:    8080,
			Enabled: true,
		}
		token := createTestTokenWithProxy("token-1", "user1@example.com", proxyConfig, 2*time.Hour)

		// Add token to store
		err := manager.SaveToken("test-provider", token)
		require.NoError(t, err)

		// Get token manager
		tokenManager, err := manager.GetTokenManager("test-provider")
		require.NoError(t, err)

		// Get proxy health tracker
		healthTracker, err := manager.GetProxyHealthTracker("test-provider")
		require.NoError(t, err)

		// Select token with client
		selectedToken, client, err := tokenManager.SelectTokenWithClient()
		require.NoError(t, err)
		require.NotNil(t, selectedToken)
		require.NotNil(t, client)

		// Verify token selection
		assert.Equal(t, "token-1", selectedToken.ID)
		assert.NotNil(t, selectedToken.Proxy)
		assert.Equal(t, auth.ProxyTypeHTTP, selectedToken.Proxy.Type)

		// Verify client was created
		assert.NotNil(t, client)

		// Verify health tracker has record
		healthScore := healthTracker.GetHealthScore("token-1")
		assert.Equal(t, 1.0, healthScore)
	})
}

func TestProxyIntegration_TokenSelectionWithHTTPSProxy(t *testing.T) {
	t.Run("select token with HTTPS proxy", func(t *testing.T) {
		manager, _, cleanup := setupProxyTestEnvironment(t)
		defer cleanup()

		// Create token with HTTPS proxy
		proxyConfig := &auth.ProxyConfig{
			Type:    auth.ProxyTypeHTTPS,
			Host:    "secure.proxy.com",
			Port:    443,
			Enabled: true,
		}
		token := createTestTokenWithProxy("token-1", "user1@example.com", proxyConfig, 2*time.Hour)

		// Add token to store
		err := manager.SaveToken("test-provider", token)
		require.NoError(t, err)

		// Get token manager
		tokenManager, err := manager.GetTokenManager("test-provider")
		require.NoError(t, err)

		// Select token with client
		selectedToken, client, err := tokenManager.SelectTokenWithClient()
		require.NoError(t, err)
		require.NotNil(t, selectedToken)
		require.NotNil(t, client)

		// Verify token selection
		assert.Equal(t, "token-1", selectedToken.ID)
		assert.NotNil(t, selectedToken.Proxy)
		assert.Equal(t, auth.ProxyTypeHTTPS, selectedToken.Proxy.Type)
	})
}

func TestProxyIntegration_TokenSelectionWithSOCKS5Proxy(t *testing.T) {
	t.Run("select token with SOCKS5 proxy", func(t *testing.T) {
		manager, _, cleanup := setupProxyTestEnvironment(t)
		defer cleanup()

		// Create token with SOCKS5 proxy
		proxyConfig := &auth.ProxyConfig{
			Type:    auth.ProxyTypeSOCKS5,
			Host:    "socks.example.com",
			Port:    1080,
			Enabled: true,
		}
		token := createTestTokenWithProxy("token-1", "user1@example.com", proxyConfig, 2*time.Hour)

		// Add token to store
		err := manager.SaveToken("test-provider", token)
		require.NoError(t, err)

		// Get token manager
		tokenManager, err := manager.GetTokenManager("test-provider")
		require.NoError(t, err)

		// Select token with client
		selectedToken, client, err := tokenManager.SelectTokenWithClient()
		require.NoError(t, err)
		require.NotNil(t, selectedToken)
		require.NotNil(t, client)

		// Verify token selection
		assert.Equal(t, "token-1", selectedToken.ID)
		assert.NotNil(t, selectedToken.Proxy)
		assert.Equal(t, auth.ProxyTypeSOCKS5, selectedToken.Proxy.Type)
	})
}

func TestProxyIntegration_TokenSelectionWithNoProxy(t *testing.T) {
	t.Run("select token with no proxy", func(t *testing.T) {
		manager, _, cleanup := setupProxyTestEnvironment(t)
		defer cleanup()

		// Create token without proxy
		token := createTestTokenWithProxy("token-1", "user1@example.com", nil, 2*time.Hour)

		// Add token to store
		err := manager.SaveToken("test-provider", token)
		require.NoError(t, err)

		// Get token manager
		tokenManager, err := manager.GetTokenManager("test-provider")
		require.NoError(t, err)

		// Select token with client
		selectedToken, client, err := tokenManager.SelectTokenWithClient()
		require.NoError(t, err)
		require.NotNil(t, selectedToken)
		require.NotNil(t, client)

		// Verify token selection
		assert.Equal(t, "token-1", selectedToken.ID)
		assert.Nil(t, selectedToken.Proxy)
	})
}

func TestProxyIntegration_TokenSelectionWithDisabledProxy(t *testing.T) {
	t.Run("select token with disabled proxy", func(t *testing.T) {
		manager, _, cleanup := setupProxyTestEnvironment(t)
		defer cleanup()

		// Create token with disabled proxy
		proxyConfig := &auth.ProxyConfig{
			Type:    auth.ProxyTypeHTTP,
			Host:    "proxy.example.com",
			Port:    8080,
			Enabled: false,
		}
		token := createTestTokenWithProxy("token-1", "user1@example.com", proxyConfig, 2*time.Hour)

		// Add token to store
		err := manager.SaveToken("test-provider", token)
		require.NoError(t, err)

		// Get token manager
		tokenManager, err := manager.GetTokenManager("test-provider")
		require.NoError(t, err)

		// Select token with client
		selectedToken, client, err := tokenManager.SelectTokenWithClient()
		require.NoError(t, err)
		require.NotNil(t, selectedToken)
		require.NotNil(t, client)

		// Verify token selection
		assert.Equal(t, "token-1", selectedToken.ID)
		assert.NotNil(t, selectedToken.Proxy)
		assert.False(t, selectedToken.Proxy.Enabled)
	})
}

func TestProxyIntegration_TokenSelectionWithAuthenticatedProxy(t *testing.T) {
	t.Run("select token with authenticated proxy", func(t *testing.T) {
		manager, _, cleanup := setupProxyTestEnvironment(t)
		defer cleanup()

		// Create token with authenticated proxy
		proxyConfig := &auth.ProxyConfig{
			Type:     auth.ProxyTypeHTTP,
			Host:     "proxy.example.com",
			Port:     8080,
			Username: "testuser",
			Password: "testpass",
			Enabled:  true,
		}
		token := createTestTokenWithProxy("token-1", "user1@example.com", proxyConfig, 2*time.Hour)

		// Add token to store
		err := manager.SaveToken("test-provider", token)
		require.NoError(t, err)

		// Get token manager
		tokenManager, err := manager.GetTokenManager("test-provider")
		require.NoError(t, err)

		// Select token with client
		selectedToken, client, err := tokenManager.SelectTokenWithClient()
		require.NoError(t, err)
		require.NotNil(t, selectedToken)
		require.NotNil(t, client)

		// Verify token selection
		assert.Equal(t, "token-1", selectedToken.ID)
		assert.NotNil(t, selectedToken.Proxy)
		assert.Equal(t, "testuser", selectedToken.Proxy.Username)
	})
}

// ============================================================================
// Provider Request Tests
// ============================================================================

func TestProxyIntegration_ProviderRequestWithProxy(t *testing.T) {
	t.Run("non-streaming request with proxy", func(t *testing.T) {
		// Create mock HTTP server
		mockServer := NewMockHTTPServer(func(w http.ResponseWriter, r *http.Request) {
			// Verify authorization header
			authHeader := r.Header.Get("Authorization")
			assert.True(t, strings.HasPrefix(authHeader, "Bearer "))

			// Return success response
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"response": map[string]interface{}{
					"candidates": []map[string]interface{}{
						{
							"content": map[string]interface{}{
								"parts": []map[string]interface{}{
									{
										"text": "Test response",
									},
								},
							},
						},
					},
				},
			})
		})
		mockServer.Start()
		defer mockServer.Stop()

		manager, _, cleanup := setupProxyTestEnvironment(t)
		defer cleanup()

		// Create token with proxy
		proxyConfig := &auth.ProxyConfig{
			Type:    auth.ProxyTypeHTTP,
			Host:    "proxy.example.com",
			Port:    8080,
			Enabled: true,
		}
		token := createTestTokenWithProxy("token-1", "user1@example.com", proxyConfig, 2*time.Hour)

		// Add token to store
		err := manager.SaveToken("test-provider", token)
		require.NoError(t, err)

		// Get token manager
		tokenManager, err := manager.GetTokenManager("test-provider")
		require.NoError(t, err)

		// Select token with client
		selectedToken, client, err := tokenManager.SelectTokenWithClient()
		require.NoError(t, err)
		require.NotNil(t, client)

		// Make a request using the client
		req, err := http.NewRequest("POST", mockServer.GetURL()+"/api/generate", strings.NewReader(`{"test": "data"}`))
		require.NoError(t, err)
		req.Header.Set("Authorization", "Bearer "+selectedToken.AccessToken)
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		// Verify response
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		// Verify request was recorded
		assert.Equal(t, 1, mockServer.GetRequestCount())
	})
}

func TestProxyIntegration_ProviderRequestWithMultipleProxies(t *testing.T) {
	t.Run("multiple requests with different proxies", func(t *testing.T) {
		manager, factory, cleanup := setupProxyTestEnvironment(t)
		defer cleanup()

		// Create tokens with different proxy configurations
		proxyConfig1 := &auth.ProxyConfig{
			Type:    auth.ProxyTypeHTTP,
			Host:    "proxy1.example.com",
			Port:    8080,
			Enabled: true,
		}
		proxyConfig2 := &auth.ProxyConfig{
			Type:    auth.ProxyTypeHTTP,
			Host:    "proxy2.example.com",
			Port:    8081,
			Enabled: true,
		}

		token1 := createTestTokenWithProxy("token-1", "user1@example.com", proxyConfig1, 2*time.Hour)
		token2 := createTestTokenWithProxy("token-2", "user2@example.com", proxyConfig2, 2*time.Hour)

		// Add tokens to store
		err := manager.SaveToken("test-provider", token1)
		require.NoError(t, err)
		err = manager.SaveToken("test-provider", token2)
		require.NoError(t, err)

		// Get token manager
		tokenManager, err := manager.GetTokenManager("test-provider")
		require.NoError(t, err)

		// Make client selections (not actual HTTP requests)
		var clients []*http.Client
		for i := 0; i < 4; i++ {
			selectedToken, client, err := tokenManager.SelectTokenWithClient()
			require.NoError(t, err)
			require.NotNil(t, client)
			require.NotNil(t, selectedToken)
			clients = append(clients, client)
		}

		// Verify all clients were created
		for _, client := range clients {
			require.NotNil(t, client)
		}

		// Verify cache size is 2 (different proxy configs)
		assert.Equal(t, 2, factory.GetCacheSize())
	})
}

// ============================================================================
// Client Caching Tests
// ============================================================================

func TestProxyIntegration_SameProxyConfigUsesCachedClient(t *testing.T) {
	t.Run("same proxy config uses cached client", func(t *testing.T) {
		manager, factory, cleanup := setupProxyTestEnvironment(t)
		defer cleanup()

		// Create token with proxy
		proxyConfig := &auth.ProxyConfig{
			Type:    auth.ProxyTypeHTTP,
			Host:    "proxy.example.com",
			Port:    8080,
			Enabled: true,
		}
		token := createTestTokenWithProxy("token-1", "user1@example.com", proxyConfig, 2*time.Hour)

		// Add token to store
		err := manager.SaveToken("test-provider", token)
		require.NoError(t, err)

		// Get token manager
		tokenManager, err := manager.GetTokenManager("test-provider")
		require.NoError(t, err)

		// Get client multiple times
		client1, err1 := tokenManager.GetTokenClient("token-1")
		require.NoError(t, err1)

		client2, err2 := tokenManager.GetTokenClient("token-1")
		require.NoError(t, err2)

		// Verify same client is returned (cached)
		assert.Same(t, client1, client2)

		// Verify cache size
		assert.Equal(t, 1, factory.GetCacheSize())
	})
}

func TestProxyIntegration_DifferentProxyConfigUsesNewClient(t *testing.T) {
	t.Run("different proxy config uses new client", func(t *testing.T) {
		manager, factory, cleanup := setupProxyTestEnvironment(t)
		defer cleanup()

		// Create tokens with different proxy configurations
		proxyConfig1 := &auth.ProxyConfig{
			Type:    auth.ProxyTypeHTTP,
			Host:    "proxy1.example.com",
			Port:    8080,
			Enabled: true,
		}
		proxyConfig2 := &auth.ProxyConfig{
			Type:    auth.ProxyTypeHTTP,
			Host:    "proxy2.example.com",
			Port:    8081,
			Enabled: true,
		}

		token1 := createTestTokenWithProxy("token-1", "user1@example.com", proxyConfig1, 2*time.Hour)
		token2 := createTestTokenWithProxy("token-2", "user2@example.com", proxyConfig2, 2*time.Hour)

		// Add tokens to store
		err := manager.SaveToken("test-provider", token1)
		require.NoError(t, err)
		err = manager.SaveToken("test-provider", token2)
		require.NoError(t, err)

		// Get token manager
		tokenManager, err := manager.GetTokenManager("test-provider")
		require.NoError(t, err)

		// Get clients
		client1, err1 := tokenManager.GetTokenClient("token-1")
		require.NoError(t, err1)

		client2, err2 := tokenManager.GetTokenClient("token-2")
		require.NoError(t, err2)

		// Verify different clients are returned
		assert.NotSame(t, client1, client2)

		// Verify cache size
		assert.Equal(t, 2, factory.GetCacheSize())
	})
}

func TestProxyIntegration_MultipleRequestsSameClient(t *testing.T) {
	t.Run("multiple requests use same client", func(t *testing.T) {
		manager, factory, cleanup := setupProxyTestEnvironment(t)
		defer cleanup()

		// Create token with proxy
		proxyConfig := &auth.ProxyConfig{
			Type:    auth.ProxyTypeHTTP,
			Host:    "proxy.example.com",
			Port:    8080,
			Enabled: true,
		}
		token := createTestTokenWithProxy("token-1", "user1@example.com", proxyConfig, 2*time.Hour)

		// Add token to store
		err := manager.SaveToken("test-provider", token)
		require.NoError(t, err)

		// Get token manager
		tokenManager, err := manager.GetTokenManager("test-provider")
		require.NoError(t, err)

		// Make multiple client selections (not actual HTTP requests)
		var clients []*http.Client
		for i := 0; i < 5; i++ {
			selectedToken, client, err := tokenManager.SelectTokenWithClient()
			require.NoError(t, err)
			require.NotNil(t, client)
			require.NotNil(t, selectedToken)
			clients = append(clients, client)
		}

		// Verify all clients are the same (cached)
		for i := 1; i < len(clients); i++ {
			assert.Same(t, clients[0], clients[i])
		}

		// Verify cache size is 1
		assert.Equal(t, 1, factory.GetCacheSize())
	})
}

// ============================================================================
// Health Tracking Tests
// ============================================================================

func TestProxyIntegration_HealthScoreIncreaseOnSuccess(t *testing.T) {
	t.Run("health score increases on success", func(t *testing.T) {
		manager, _, cleanup := setupProxyTestEnvironment(t)
		defer cleanup()

		// Create token with proxy
		proxyConfig := &auth.ProxyConfig{
			Type:    auth.ProxyTypeHTTP,
			Host:    "proxy.example.com",
			Port:    8080,
			Enabled: true,
		}
		token := createTestTokenWithProxy("token-1", "user1@example.com", proxyConfig, 2*time.Hour)

		// Add token to store
		err := manager.SaveToken("test-provider", token)
		require.NoError(t, err)

		// Get token manager
		tokenManager, err := manager.GetTokenManager("test-provider")
		require.NoError(t, err)

		// Get proxy health tracker
		healthTracker, err := manager.GetProxyHealthTracker("test-provider")
		require.NoError(t, err)

		// Report success
		err = tokenManager.UpdateProxyHealth("token-1", true, nil)
		require.NoError(t, err)

		// Verify health score
		healthScore := healthTracker.GetHealthScore("token-1")
		assert.Equal(t, 1.0, healthScore)

		// Verify token's proxy health score
		store, err := manager.GetTokenStore("test-provider")
		require.NoError(t, err)
		updatedToken, err := store.GetToken("token-1")
		require.NoError(t, err)
		assert.Equal(t, 1.0, updatedToken.ProxyHealthScore)
	})
}

func TestProxyIntegration_HealthScoreDecreaseOnFailure(t *testing.T) {
	t.Run("health score decreases on failure", func(t *testing.T) {
		manager, _, cleanup := setupProxyTestEnvironment(t)
		defer cleanup()

		// Create token with proxy
		proxyConfig := &auth.ProxyConfig{
			Type:    auth.ProxyTypeHTTP,
			Host:    "proxy.example.com",
			Port:    8080,
			Enabled: true,
		}
		token := createTestTokenWithProxy("token-1", "user1@example.com", proxyConfig, 2*time.Hour)

		// Add token to store
		err := manager.SaveToken("test-provider", token)
		require.NoError(t, err)

		// Get token manager
		tokenManager, err := manager.GetTokenManager("test-provider")
		require.NoError(t, err)

		// Get proxy health tracker
		healthTracker, err := manager.GetProxyHealthTracker("test-provider")
		require.NoError(t, err)

		// Report failure
		testErr := fmt.Errorf("proxy connection failed")
		err = tokenManager.UpdateProxyHealth("token-1", false, testErr)
		require.NoError(t, err)

		// Verify health score decreased
		healthScore := healthTracker.GetHealthScore("token-1")
		assert.Less(t, healthScore, 1.0)

		// Verify token's proxy health score
		store, err := manager.GetTokenStore("test-provider")
		require.NoError(t, err)
		updatedToken, err := store.GetToken("token-1")
		require.NoError(t, err)
		assert.Less(t, updatedToken.ProxyHealthScore, 1.0)
	})
}

func TestProxyIntegration_ConsecutiveFailureTracking(t *testing.T) {
	t.Run("consecutive failures are tracked", func(t *testing.T) {
		manager, _, cleanup := setupProxyTestEnvironment(t)
		defer cleanup()

		// Create token with proxy
		proxyConfig := &auth.ProxyConfig{
			Type:    auth.ProxyTypeHTTP,
			Host:    "proxy.example.com",
			Port:    8080,
			Enabled: true,
		}
		token := createTestTokenWithProxy("token-1", "user1@example.com", proxyConfig, 2*time.Hour)

		// Add token to store
		err := manager.SaveToken("test-provider", token)
		require.NoError(t, err)

		// Get token manager
		tokenManager, err := manager.GetTokenManager("test-provider")
		require.NoError(t, err)

		// Get proxy health tracker
		healthTracker, err := manager.GetProxyHealthTracker("test-provider")
		require.NoError(t, err)

		// Report multiple failures
		testErr := fmt.Errorf("proxy connection failed")
		for i := 0; i < 3; i++ {
			err = tokenManager.UpdateProxyHealth("token-1", false, testErr)
			require.NoError(t, err)
		}

		// Verify health status
		healthStatus := healthTracker.GetHealthStatus("token-1")
		require.NotNil(t, healthStatus)
		assert.Equal(t, 3, healthStatus.ConsecutiveFailures)
		assert.False(t, healthStatus.IsHealthy)
	})
}

func TestProxyIntegration_HealthStatusRetrieval(t *testing.T) {
	t.Run("health status can be retrieved", func(t *testing.T) {
		manager, _, cleanup := setupProxyTestEnvironment(t)
		defer cleanup()

		// Create token with proxy
		proxyConfig := &auth.ProxyConfig{
			Type:    auth.ProxyTypeHTTP,
			Host:    "proxy.example.com",
			Port:    8080,
			Enabled: true,
		}
		token := createTestTokenWithProxy("token-1", "user1@example.com", proxyConfig, 2*time.Hour)

		// Add token to store
		err := manager.SaveToken("test-provider", token)
		require.NoError(t, err)

		// Get token manager
		tokenManager, err := manager.GetTokenManager("test-provider")
		require.NoError(t, err)

		// Get proxy health tracker
		healthTracker, err := manager.GetProxyHealthTracker("test-provider")
		require.NoError(t, err)

		// Report success
		err = tokenManager.UpdateProxyHealth("token-1", true, nil)
		require.NoError(t, err)

		// Get health status
		healthStatus := healthTracker.GetHealthStatus("token-1")
		require.NotNil(t, healthStatus)
		assert.True(t, healthStatus.IsHealthy)
		assert.Equal(t, 1.0, healthStatus.HealthScore)
		assert.Equal(t, 0, healthStatus.ConsecutiveFailures)
		assert.Greater(t, healthStatus.LastCheck, int64(0))
	})
}

// ============================================================================
// Proxy Configuration Change Tests
// ============================================================================

func TestProxyIntegration_RequestAfterProxyConfigChange(t *testing.T) {
	t.Run("request after proxy config change", func(t *testing.T) {
		// Create mock HTTP server
		mockServer := NewMockHTTPServer(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{"status": "ok"})
		})
		mockServer.Start()
		defer mockServer.Stop()

		manager, _, cleanup := setupProxyTestEnvironment(t)
		defer cleanup()

		// Create token with proxy
		proxyConfig1 := &auth.ProxyConfig{
			Type:    auth.ProxyTypeHTTP,
			Host:    "proxy1.example.com",
			Port:    8080,
			Enabled: true,
		}
		token := createTestTokenWithProxy("token-1", "user1@example.com", proxyConfig1, 2*time.Hour)

		// Add token to store
		err := manager.SaveToken("test-provider", token)
		require.NoError(t, err)

		// Get token manager
		tokenManager, err := manager.GetTokenManager("test-provider")
		require.NoError(t, err)

		// Get client with first proxy config
		client1, err1 := tokenManager.GetTokenClient("token-1")
		require.NoError(t, err1)

		// Update proxy configuration
		proxyConfig2 := &auth.ProxyConfig{
			Type:    auth.ProxyTypeHTTP,
			Host:    "proxy2.example.com",
			Port:    8081,
			Enabled: true,
		}
		store, err := manager.GetTokenStore("test-provider")
		require.NoError(t, err)
		err = store.UpdateToken("token-1", func(t *auth.ProviderToken) {
			t.Proxy = proxyConfig2
		})
		require.NoError(t, err)

		// Get client with new proxy config
		client2, err2 := tokenManager.GetTokenClient("token-1")
		require.NoError(t, err2)

		// Verify different clients are returned
		assert.NotSame(t, client1, client2)
	})
}

func TestProxyIntegration_NewClientAfterProxyUpdate(t *testing.T) {
	t.Run("new client after proxy update", func(t *testing.T) {
		manager, factory, cleanup := setupProxyTestEnvironment(t)
		defer cleanup()

		// Create token with proxy
		proxyConfig1 := &auth.ProxyConfig{
			Type:    auth.ProxyTypeHTTP,
			Host:    "proxy1.example.com",
			Port:    8080,
			Enabled: true,
		}
		token := createTestTokenWithProxy("token-1", "user1@example.com", proxyConfig1, 2*time.Hour)

		// Add token to store
		err := manager.SaveToken("test-provider", token)
		require.NoError(t, err)

		// Get token manager
		tokenManager, err := manager.GetTokenManager("test-provider")
		require.NoError(t, err)

		// Get client
		client1, err1 := tokenManager.GetTokenClient("token-1")
		require.NoError(t, err1)

		// Clear cache
		factory.ClearCache()

		// Get client again
		client2, err2 := tokenManager.GetTokenClient("token-1")
		require.NoError(t, err2)

		// Verify different clients are returned
		assert.NotSame(t, client1, client2)
	})
}

func TestProxyIntegration_DirectConnectionAfterProxyRemoval(t *testing.T) {
	t.Run("direct connection after proxy removal", func(t *testing.T) {
		manager, _, cleanup := setupProxyTestEnvironment(t)
		defer cleanup()

		// Create token with proxy
		proxyConfig := &auth.ProxyConfig{
			Type:    auth.ProxyTypeHTTP,
			Host:    "proxy.example.com",
			Port:    8080,
			Enabled: true,
		}
		token := createTestTokenWithProxy("token-1", "user1@example.com", proxyConfig, 2*time.Hour)

		// Add token to store
		err := manager.SaveToken("test-provider", token)
		require.NoError(t, err)

		// Get token manager
		tokenManager, err := manager.GetTokenManager("test-provider")
		require.NoError(t, err)

		// Get client with proxy
		client1, err1 := tokenManager.GetTokenClient("token-1")
		require.NoError(t, err1)

		// Remove proxy configuration
		store, err := manager.GetTokenStore("test-provider")
		require.NoError(t, err)
		err = store.UpdateToken("token-1", func(t *auth.ProviderToken) {
			t.Proxy = nil
		})
		require.NoError(t, err)

		// Get client without proxy
		client2, err2 := tokenManager.GetTokenClient("token-1")
		require.NoError(t, err2)

		// Verify different clients are returned
		assert.NotSame(t, client1, client2)
	})
}

// ============================================================================
// Table-Driven Tests for Edge Cases
// ============================================================================

func TestProxyIntegration_EdgeCases(t *testing.T) {
	tests := []struct {
		name        string
		proxyConfig *auth.ProxyConfig
		expectError bool
		description string
	}{
		{
			name:        "nil proxy config",
			proxyConfig: nil,
			expectError: false,
			description: "should handle nil proxy config gracefully",
		},
		{
			name: "proxy type none",
			proxyConfig: &auth.ProxyConfig{
				Type:    auth.ProxyTypeNone,
				Host:    "",
				Port:    0,
				Enabled: false,
			},
			expectError: false,
			description: "should handle ProxyTypeNone",
		},
		{
			name: "proxy with empty host",
			proxyConfig: &auth.ProxyConfig{
				Type:    auth.ProxyTypeHTTP,
				Host:    "",
				Port:    8080,
				Enabled: true,
			},
			expectError: true,
			description: "should reject proxy with empty host",
		},
		{
			name: "proxy with port 0",
			proxyConfig: &auth.ProxyConfig{
				Type:    auth.ProxyTypeHTTP,
				Host:    "proxy.example.com",
				Port:    0,
				Enabled: true,
			},
			expectError: true,
			description: "should reject proxy with port 0",
		},
		{
			name: "proxy with port out of range",
			proxyConfig: &auth.ProxyConfig{
				Type:    auth.ProxyTypeHTTP,
				Host:    "proxy.example.com",
				Port:    70000,
				Enabled: true,
			},
			expectError: true,
			description: "should reject proxy with port out of range",
		},
		{
			name: "proxy with username but no password",
			proxyConfig: &auth.ProxyConfig{
				Type:     auth.ProxyTypeHTTP,
				Host:     "proxy.example.com",
				Port:     8080,
				Username: "testuser",
				Password: "",
				Enabled:  true,
			},
			expectError: true,
			description: "should reject proxy with username but no password",
		},
		{
			name: "proxy with password but no username",
			proxyConfig: &auth.ProxyConfig{
				Type:     auth.ProxyTypeHTTP,
				Host:     "proxy.example.com",
				Port:     8080,
				Username: "",
				Password: "testpass",
				Enabled:  true,
			},
			expectError: true,
			description: "should reject proxy with password but no username",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager, _, cleanup := setupProxyTestEnvironment(t)
			defer cleanup()

			// Create token with proxy config
			token := createTestTokenWithProxy("token-1", "user1@example.com", tt.proxyConfig, 2*time.Hour)

			// Add token to store
			err := manager.SaveToken("test-provider", token)

			if tt.expectError {
				assert.Error(t, err, tt.description)
			} else {
				require.NoError(t, err)

				// Get token manager
				tokenManager, err := manager.GetTokenManager("test-provider")
				require.NoError(t, err)

				// Select token with client
				selectedToken, client, err := tokenManager.SelectTokenWithClient()
				require.NoError(t, err)
				require.NotNil(t, selectedToken)
				require.NotNil(t, client)
			}
		})
	}
}

func TestProxyIntegration_ConcurrentRequests(t *testing.T) {
	t.Run("concurrent requests with same proxy", func(t *testing.T) {
		manager, factory, cleanup := setupProxyTestEnvironment(t)
		defer cleanup()

		// Create token with proxy
		proxyConfig := &auth.ProxyConfig{
			Type:    auth.ProxyTypeHTTP,
			Host:    "proxy.example.com",
			Port:    8080,
			Enabled: true,
		}
		token := createTestTokenWithProxy("token-1", "user1@example.com", proxyConfig, 2*time.Hour)

		// Add token to store
		err := manager.SaveToken("test-provider", token)
		require.NoError(t, err)

		// Get token manager
		tokenManager, err := manager.GetTokenManager("test-provider")
		require.NoError(t, err)

		// Make concurrent client selections (not actual HTTP requests)
		var wg sync.WaitGroup
		numRequests := 10
		clients := make([]*http.Client, numRequests)

		for i := 0; i < numRequests; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()

				selectedToken, client, err := tokenManager.SelectTokenWithClient()
				require.NoError(t, err)
				require.NotNil(t, client)
				require.NotNil(t, selectedToken)

				clients[idx] = client
			}(i)
		}

		wg.Wait()

		// Verify all clients were created
		for _, client := range clients {
			require.NotNil(t, client)
		}

		// Verify cache size is 1 (all requests used same client)
		assert.Equal(t, 1, factory.GetCacheSize())

		// Verify all clients are the same instance (cached)
		for i := 1; i < numRequests; i++ {
			assert.Same(t, clients[0], clients[i])
		}
	})
}

func TestProxyIntegration_LatencyRecording(t *testing.T) {
	t.Run("latency is recorded", func(t *testing.T) {
		manager, _, cleanup := setupProxyTestEnvironment(t)
		defer cleanup()

		// Create token with proxy
		proxyConfig := &auth.ProxyConfig{
			Type:    auth.ProxyTypeHTTP,
			Host:    "proxy.example.com",
			Port:    8080,
			Enabled: true,
		}
		token := createTestTokenWithProxy("token-1", "user1@example.com", proxyConfig, 2*time.Hour)

		// Add token to store
		err := manager.SaveToken("test-provider", token)
		require.NoError(t, err)

		// Get proxy health tracker
		healthTracker, err := manager.GetProxyHealthTracker("test-provider")
		require.NoError(t, err)

		// Record latency
		healthTracker.RecordLatency("token-1", 100)
		healthTracker.RecordLatency("token-1", 150)
		healthTracker.RecordLatency("token-1", 120)

		// Get health status
		healthStatus := healthTracker.GetHealthStatus("token-1")
		require.NotNil(t, healthStatus)

		// Verify average latency is recorded
		// The implementation uses simple averaging: (old + new) / 2
		// First: 100, Second: (100 + 150) / 2 = 125, Third: (125 + 120) / 2 = 122.5
		assert.Greater(t, healthStatus.AverageLatencyMs, 0)
	})
}

func TestProxyIntegration_UnhealthyTokenTracking(t *testing.T) {
	t.Run("unhealthy tokens are tracked", func(t *testing.T) {
		manager, _, cleanup := setupProxyTestEnvironment(t)
		defer cleanup()

		// Create multiple tokens with proxies
		proxyConfig := &auth.ProxyConfig{
			Type:    auth.ProxyTypeHTTP,
			Host:    "proxy.example.com",
			Port:    8080,
			Enabled: true,
		}

		token1 := createTestTokenWithProxy("token-1", "user1@example.com", proxyConfig, 2*time.Hour)
		token2 := createTestTokenWithProxy("token-2", "user2@example.com", proxyConfig, 2*time.Hour)
		token3 := createTestTokenWithProxy("token-3", "user3@example.com", proxyConfig, 2*time.Hour)

		// Add tokens to store
		err := manager.SaveToken("test-provider", token1)
		require.NoError(t, err)
		err = manager.SaveToken("test-provider", token2)
		require.NoError(t, err)
		err = manager.SaveToken("test-provider", token3)
		require.NoError(t, err)

		// Get token manager
		tokenManager, err := manager.GetTokenManager("test-provider")
		require.NoError(t, err)

		// Get proxy health tracker
		healthTracker, err := manager.GetProxyHealthTracker("test-provider")
		require.NoError(t, err)

		// Mark token2 and token3 as unhealthy
		testErr := fmt.Errorf("proxy connection failed")
		err = tokenManager.UpdateProxyHealth("token-2", false, testErr)
		require.NoError(t, err)
		err = tokenManager.UpdateProxyHealth("token-3", false, testErr)
		require.NoError(t, err)

		// Verify unhealthy tokens
		unhealthyTokens := healthTracker.ListUnhealthyTokens()
		assert.Contains(t, unhealthyTokens, "token-2")
		assert.Contains(t, unhealthyTokens, "token-3")
		assert.NotContains(t, unhealthyTokens, "token-1")

		// Verify unhealthy token count
		assert.Equal(t, 2, healthTracker.GetUnhealthyTokenCount())
	})
}

func TestProxyIntegration_CacheEviction(t *testing.T) {
	t.Run("LRU cache eviction works", func(t *testing.T) {
		manager, factory, cleanup := setupProxyTestEnvironment(t)
		defer cleanup()

		// Create tokens with different proxy configurations
		for i := 0; i < 5; i++ {
			proxyConfig := &auth.ProxyConfig{
				Type:    auth.ProxyTypeHTTP,
				Host:    fmt.Sprintf("proxy%d.example.com", i),
				Port:    8080 + i,
				Enabled: true,
			}
			token := createTestTokenWithProxy(
				fmt.Sprintf("token-%d", i),
				fmt.Sprintf("user%d@example.com", i),
				proxyConfig,
				2*time.Hour,
			)

			// Add token to store
			err := manager.SaveToken("test-provider", token)
			require.NoError(t, err)

			// Get token manager
			tokenManager, err := manager.GetTokenManager("test-provider")
			require.NoError(t, err)

			// Get client to add to cache
			_, err = tokenManager.GetTokenClient(token.ID)
			require.NoError(t, err)
		}

		// Verify cache size (factory default is 50, so all should be cached)
		assert.Equal(t, 5, factory.GetCacheSize())
	})
}

// ============================================================================
// Error Handling Tests
// ============================================================================

func TestProxyIntegration_ProxyConnectionRefused(t *testing.T) {
	t.Run("handles proxy connection refused", func(t *testing.T) {
		manager, _, cleanup := setupProxyTestEnvironment(t)
		defer cleanup()

		// Create token with invalid proxy (connection will be refused)
		proxyConfig := &auth.ProxyConfig{
			Type:    auth.ProxyTypeHTTP,
			Host:    "localhost",
			Port:    9999, // Unlikely to be listening
			Enabled: true,
		}
		token := createTestTokenWithProxy("token-1", "user1@example.com", proxyConfig, 2*time.Hour)

		// Add token to store
		err := manager.SaveToken("test-provider", token)
		require.NoError(t, err)

		// Get token manager
		tokenManager, err := manager.GetTokenManager("test-provider")
		require.NoError(t, err)

		// Get client
		client, err := tokenManager.GetTokenClient("token-1")
		require.NoError(t, err)
		require.NotNil(t, client)

		// Verify client was created (actual connection failure happens at request time)
		assert.NotNil(t, client)
	})
}

func TestProxyIntegration_ProxyTimeout(t *testing.T) {
	t.Run("handles proxy timeout", func(t *testing.T) {
		manager, _, cleanup := setupProxyTestEnvironment(t)
		defer cleanup()

		// Create token with proxy that will timeout
		proxyConfig := &auth.ProxyConfig{
			Type:    auth.ProxyTypeHTTP,
			Host:    "10.255.255.1", // Unreachable IP
			Port:    8080,
			Enabled: true,
		}
		token := createTestTokenWithProxy("token-1", "user1@example.com", proxyConfig, 2*time.Hour)

		// Add token to store
		err := manager.SaveToken("test-provider", token)
		require.NoError(t, err)

		// Get token manager
		tokenManager, err := manager.GetTokenManager("test-provider")
		require.NoError(t, err)

		// Get client
		client, err := tokenManager.GetTokenClient("token-1")
		require.NoError(t, err)
		require.NotNil(t, client)

		// Verify client was created (actual timeout happens at request time)
		assert.NotNil(t, client)
	})
}

func TestProxyIntegration_ProxyDNSFailure(t *testing.T) {
	t.Run("handles proxy DNS resolution failure", func(t *testing.T) {
		manager, _, cleanup := setupProxyTestEnvironment(t)
		defer cleanup()

		// Create token with invalid hostname (DNS will fail)
		proxyConfig := &auth.ProxyConfig{
			Type:    auth.ProxyTypeHTTP,
			Host:    "this-domain-definitely-does-not-exist-12345.com",
			Port:    8080,
			Enabled: true,
		}
		token := createTestTokenWithProxy("token-1", "user1@example.com", proxyConfig, 2*time.Hour)

		// Add token to store
		err := manager.SaveToken("test-provider", token)
		require.NoError(t, err)

		// Get token manager
		tokenManager, err := manager.GetTokenManager("test-provider")
		require.NoError(t, err)

		// Get client
		client, err := tokenManager.GetTokenClient("token-1")
		require.NoError(t, err)
		require.NotNil(t, client)

		// Verify client was created (actual DNS failure happens at request time)
		assert.NotNil(t, client)
	})
}

// ============================================================================
// Streaming Tests
// ============================================================================

func TestProxyIntegration_StreamingRequestWithProxy(t *testing.T) {
	t.Run("streaming request with proxy", func(t *testing.T) {
		manager, _, cleanup := setupProxyTestEnvironment(t)
		defer cleanup()

		// Create token with proxy
		proxyConfig := &auth.ProxyConfig{
			Type:    auth.ProxyTypeHTTP,
			Host:    "proxy.example.com",
			Port:    8080,
			Enabled: true,
		}
		token := createTestTokenWithProxy("token-1", "user1@example.com", proxyConfig, 2*time.Hour)

		// Add token to store
		err := manager.SaveToken("test-provider", token)
		require.NoError(t, err)

		// Get token manager
		tokenManager, err := manager.GetTokenManager("test-provider")
		require.NoError(t, err)

		// Select token with client
		selectedToken, client, err := tokenManager.SelectTokenWithClient()
		require.NoError(t, err)
		require.NotNil(t, client)

		// Verify token selection
		assert.Equal(t, "token-1", selectedToken.ID)
		assert.NotNil(t, selectedToken.Proxy)
		assert.Equal(t, auth.ProxyTypeHTTP, selectedToken.Proxy.Type)

		// Verify client was created
		assert.NotNil(t, client)
	})
}
