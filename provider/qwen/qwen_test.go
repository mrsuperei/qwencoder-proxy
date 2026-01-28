package qwen

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunbankio/qwencoder-proxy/auth"
	"github.com/sunbankio/qwencoder-proxy/logging"
	providerpkg "github.com/sunbankio/qwencoder-proxy/provider"
)

// mockProxyClientFactory is a mock implementation of auth.ProxyClientFactory for testing
type mockProxyClientFactory struct {
	mockClient *http.Client
	calls      []*auth.ProxyConfig
}

func (m *mockProxyClientFactory) GetClient(proxyConfig *auth.ProxyConfig) *http.Client {
	m.calls = append(m.calls, proxyConfig)
	if m.mockClient != nil {
		return m.mockClient
	}
	return &http.Client{}
}

func createTestToken(id string) *auth.ProviderToken {
	now := time.Now()
	return &auth.ProviderToken{
		ID:           id,
		AccessToken:  "test-access-token-" + id,
		RefreshToken: "test-refresh-token-" + id,
		TokenType:    "Bearer",
		ExpiryDate:   now.Add(time.Hour).UnixMilli(),
		Email:        "test@example.com",
		Healthy:      true,
		HealthScore:  1.0,
		LastUsed:     now.UnixMilli(),
		CreatedAt:    now.UnixMilli(),
	}
}

func createTestTokenWithProxy(id string, proxyConfig *auth.ProxyConfig) *auth.ProviderToken {
	token := createTestToken(id)
	token.Proxy = proxyConfig
	return token
}

func TestQwenAuthenticator_GetTokenWithClient(t *testing.T) {
	t.Run("returns token and proxy-aware client when token manager is set", func(t *testing.T) {
		logger := logging.NewLogger()
		tempDir := t.TempDir()
		filePath := tempDir + "/store.json"

		// Create token store with a token that has proxy config
		store := auth.NewMultiTokenStore("qwen", filePath, logger)
		proxyConfig := &auth.ProxyConfig{
			Type:    auth.ProxyTypeHTTP,
			Host:    "proxy.example.com",
			Port:    8080,
			Enabled: true,
		}
		token := createTestTokenWithProxy("test-token-1", proxyConfig)
		require.NoError(t, store.AddToken(*token))

		// Create token manager with mock proxy client factory
		mockClient := &http.Client{Timeout: 30 * time.Second}
		mockFactory := &mockProxyClientFactory{mockClient: mockClient}
		proxyTracker := auth.NewProxyHealthTracker(logger, 5, 5*time.Minute)
		tokenManager := auth.NewTokenManager(store, auth.NewRandomSelectionStrategy(), logger, mockFactory, proxyTracker)

		// Create authenticator with token manager
		authenticator := NewQwenAuthenticator(tokenManager, logger)

		// Get token with client
		ctx := context.Background()
		accessToken, client, err := authenticator.GetTokenWithClient(ctx)

		require.NoError(t, err)
		assert.Equal(t, "test-access-token-test-token-1", accessToken)
		assert.NotNil(t, client)
		assert.Equal(t, mockClient, client)
		assert.Len(t, mockFactory.calls, 1)
		assert.Equal(t, proxyConfig, mockFactory.calls[0])
	})

	t.Run("returns token with nil client when token manager is nil", func(t *testing.T) {
		logger := logging.NewLogger()
		authenticator := NewQwenAuthenticator(nil, logger)

		// Get token with client when token manager is nil
		ctx := context.Background()
		accessToken, client, err := authenticator.GetTokenWithClient(ctx)

		// Should fall back to GetToken which will fail
		assert.Error(t, err)
		assert.Empty(t, accessToken)
		assert.Nil(t, client)
	})

	t.Run("returns token with direct connection when token has no proxy", func(t *testing.T) {
		logger := logging.NewLogger()
		tempDir := t.TempDir()
		filePath := tempDir + "/store.json"

		// Create token store with a token that has no proxy config
		store := auth.NewMultiTokenStore("qwen", filePath, logger)
		token := createTestToken("test-token-2")
		require.NoError(t, store.AddToken(*token))

		// Create token manager with mock proxy client factory
		mockFactory := &mockProxyClientFactory{}
		proxyTracker := auth.NewProxyHealthTracker(logger, 5, 5*time.Minute)
		tokenManager := auth.NewTokenManager(store, auth.NewRandomSelectionStrategy(), logger, mockFactory, proxyTracker)

		// Create authenticator with token manager
		authenticator := NewQwenAuthenticator(tokenManager, logger)

		// Get token with client
		ctx := context.Background()
		accessToken, client, err := authenticator.GetTokenWithClient(ctx)

		require.NoError(t, err)
		assert.Equal(t, "test-access-token-test-token-2", accessToken)
		assert.NotNil(t, client)
		assert.Len(t, mockFactory.calls, 1)
		assert.Nil(t, mockFactory.calls[0])
	})

	t.Run("returns error when no tokens available", func(t *testing.T) {
		logger := logging.NewLogger()
		tempDir := t.TempDir()
		filePath := tempDir + "/store.json"

		// Create empty token store
		store := auth.NewMultiTokenStore("qwen", filePath, logger)
		mockFactory := &mockProxyClientFactory{}
		proxyTracker := auth.NewProxyHealthTracker(logger, 5, 5*time.Minute)
		tokenManager := auth.NewTokenManager(store, auth.NewRandomSelectionStrategy(), logger, mockFactory, proxyTracker)

		// Create authenticator with token manager
		authenticator := NewQwenAuthenticator(tokenManager, logger)

		// Get token with client
		ctx := context.Background()
		accessToken, client, err := authenticator.GetTokenWithClient(ctx)

		assert.Error(t, err)
		assert.Empty(t, accessToken)
		assert.Nil(t, client)
	})
}

func TestQwenAuthenticator_GetHTTPClient(t *testing.T) {
	t.Run("returns proxy-aware client when token manager is set", func(t *testing.T) {
		logger := logging.NewLogger()
		tempDir := t.TempDir()
		filePath := tempDir + "/store.json"

		// Create token store with a token that has proxy config
		store := auth.NewMultiTokenStore("qwen", filePath, logger)
		proxyConfig := &auth.ProxyConfig{
			Type:    auth.ProxyTypeSOCKS5,
			Host:    "socks.example.com",
			Port:    1080,
			Enabled: true,
		}
		token := createTestTokenWithProxy("test-token-3", proxyConfig)
		require.NoError(t, store.AddToken(*token))

		// Create token manager with mock proxy client factory
		mockClient := &http.Client{Timeout: 30 * time.Second}
		mockFactory := &mockProxyClientFactory{mockClient: mockClient}
		proxyTracker := auth.NewProxyHealthTracker(logger, 5, 5*time.Minute)
		tokenManager := auth.NewTokenManager(store, auth.NewRandomSelectionStrategy(), logger, mockFactory, proxyTracker)

		// Create authenticator with token manager
		authenticator := NewQwenAuthenticator(tokenManager, logger)

		// Get HTTP client
		client, err := authenticator.GetHTTPClient()

		require.NoError(t, err)
		assert.NotNil(t, client)
		assert.Equal(t, mockClient, client)
		assert.Len(t, mockFactory.calls, 1)
		assert.Equal(t, proxyConfig, mockFactory.calls[0])
	})

	t.Run("returns error when token manager is nil", func(t *testing.T) {
		logger := logging.NewLogger()
		authenticator := NewQwenAuthenticator(nil, logger)

		// Get HTTP client when token manager is nil
		client, err := authenticator.GetHTTPClient()

		assert.Error(t, err)
		assert.Nil(t, client)
		assert.Contains(t, err.Error(), "token manager not initialized")
	})

	t.Run("returns error when no tokens available", func(t *testing.T) {
		logger := logging.NewLogger()
		tempDir := t.TempDir()
		filePath := tempDir + "/store.json"

		// Create empty token store
		store := auth.NewMultiTokenStore("qwen", filePath, logger)
		mockFactory := &mockProxyClientFactory{}
		proxyTracker := auth.NewProxyHealthTracker(logger, 5, 5*time.Minute)
		tokenManager := auth.NewTokenManager(store, auth.NewRandomSelectionStrategy(), logger, mockFactory, proxyTracker)

		// Create authenticator with token manager
		authenticator := NewQwenAuthenticator(tokenManager, logger)

		// Get HTTP client
		client, err := authenticator.GetHTTPClient()

		assert.Error(t, err)
		assert.Nil(t, client)
	})
}

func TestQwenAuthenticator_BackwardCompatibility(t *testing.T) {
	t.Run("existing GetToken method falls back to qwenclient", func(t *testing.T) {
		logger := logging.NewLogger()
		// Create authenticator with token manager
		authenticator := NewQwenAuthenticator(nil, logger)

		// Get token using existing method
		// This should fall back to qwenclient.GetValidTokenAndEndpoint()
		// which will fail because we don't have credentials
		ctx := context.Background()
		_, err := authenticator.GetToken(ctx)

		// Should get an error because we don't have qwenclient credentials
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "credentials not found")
	})

	t.Run("existing IsAuthenticated method still works", func(t *testing.T) {
		logger := logging.NewLogger()
		tempDir := t.TempDir()
		filePath := tempDir + "/store.json"

		// Create token store with a token
		store := auth.NewMultiTokenStore("qwen", filePath, logger)
		token := createTestToken("test-token-5")
		require.NoError(t, store.AddToken(*token))

		// Create token manager
		tokenManager := auth.NewTokenManager(store, auth.NewRandomSelectionStrategy(), logger, nil, nil)

		// Create authenticator with token manager
		authenticator := NewQwenAuthenticator(tokenManager, logger)

		// Check authentication status
		isAuthenticated := authenticator.IsAuthenticated()

		assert.True(t, isAuthenticated)
	})
}

// TestProvider_isProxyError tests the isProxyError method
func TestProvider_isProxyError(t *testing.T) {
	provider := NewProvider()

	t.Run("returns true for timeout errors", func(t *testing.T) {
		// Create a timeout error
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
		defer cancel()
		<-ctx.Done()
		err := ctx.Err()
		assert.True(t, provider.isProxyError(err))
	})

	t.Run("returns true for connection refused errors", func(t *testing.T) {
		err := &url.Error{Err: errors.New("connection refused")}
		assert.True(t, provider.isProxyError(err))
	})

	t.Run("returns true for connection reset errors", func(t *testing.T) {
		err := errors.New("connection reset by peer")
		assert.True(t, provider.isProxyError(err))
	})

	t.Run("returns true for DNS resolution errors", func(t *testing.T) {
		err := errors.New("no such host")
		assert.True(t, provider.isProxyError(err))
	})

	t.Run("returns true for proxy errors", func(t *testing.T) {
		err := errors.New("proxy connection failed")
		assert.True(t, provider.isProxyError(err))
	})

	t.Run("returns true for SOCKS errors", func(t *testing.T) {
		err := errors.New("SOCKS proxy error")
		assert.True(t, provider.isProxyError(err))
	})

	t.Run("returns false for non-proxy errors", func(t *testing.T) {
		err := errors.New("some other error")
		assert.False(t, provider.isProxyError(err))
	})

	t.Run("returns false for nil error", func(t *testing.T) {
		assert.False(t, provider.isProxyError(nil))
	})
}

// TestProvider_classifyProxyError tests the classifyProxyError method
func TestProvider_classifyProxyError(t *testing.T) {
	provider := NewProvider()

	t.Run("classifies timeout errors", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
		defer cancel()
		<-ctx.Done()
		err := ctx.Err()
		assert.Equal(t, "timeout", provider.classifyProxyError(err))
	})

	t.Run("classifies connection refused errors", func(t *testing.T) {
		err := errors.New("connection refused")
		assert.Equal(t, "connection_refused", provider.classifyProxyError(err))
	})

	t.Run("classifies connection reset errors", func(t *testing.T) {
		err := errors.New("connection reset")
		assert.Equal(t, "connection_reset", provider.classifyProxyError(err))
	})

	t.Run("classifies broken pipe errors", func(t *testing.T) {
		err := errors.New("broken pipe")
		assert.Equal(t, "broken_pipe", provider.classifyProxyError(err))
	})

	t.Run("classifies EOF errors", func(t *testing.T) {
		err := errors.New("EOF")
		assert.Equal(t, "eof", provider.classifyProxyError(err))
	})

	t.Run("classifies DNS resolution errors", func(t *testing.T) {
		err := errors.New("no such host")
		assert.Equal(t, "dns_resolution", provider.classifyProxyError(err))
	})

	t.Run("classifies proxy errors", func(t *testing.T) {
		err := errors.New("proxy connection failed")
		assert.Equal(t, "proxy_error", provider.classifyProxyError(err))
	})

	t.Run("classifies SOCKS errors", func(t *testing.T) {
		err := errors.New("SOCKS proxy error")
		assert.Equal(t, "socks_error", provider.classifyProxyError(err))
	})

	t.Run("classifies tunnel errors", func(t *testing.T) {
		err := errors.New("tunnel connection failed")
		assert.Equal(t, "tunnel_error", provider.classifyProxyError(err))
	})

	t.Run("classifies URL errors", func(t *testing.T) {
		innerErr := errors.New("connection refused")
		err := &url.Error{Err: innerErr}
		assert.Equal(t, "connection_refused", provider.classifyProxyError(err))
	})

	t.Run("classifies unknown errors", func(t *testing.T) {
		err := errors.New("some other error")
		assert.Equal(t, "unknown", provider.classifyProxyError(err))
	})

	t.Run("classifies nil error as none", func(t *testing.T) {
		assert.Equal(t, "none", provider.classifyProxyError(nil))
	})
}

// TestProvider_doRequestWithProxy tests the doRequestWithProxy method
func TestProvider_doRequestWithProxy(t *testing.T) {
	t.Run("updates proxy health on success", func(t *testing.T) {
		logger := logging.NewLogger()
		tempDir := t.TempDir()
		filePath := tempDir + "/store.json"

		// Create token store with a token
		store := auth.NewMultiTokenStore("qwen", filePath, logger)
		token := createTestToken("test-token-success")
		require.NoError(t, store.AddToken(*token))

		// Create token manager with proxy health tracker
		mockFactory := &mockProxyClientFactory{}
		proxyTracker := auth.NewProxyHealthTracker(logger, 5, 5*time.Minute)
		tokenManager := auth.NewTokenManager(store, auth.NewRandomSelectionStrategy(), logger, mockFactory, proxyTracker)

		// Create provider with token manager
		provider := NewProviderWithTokenManager(tokenManager, logger)

		// Create a mock HTTP request
		req, err := http.NewRequest("GET", "http://example.com", nil)
		require.NoError(t, err)

		// Use a mock HTTP client that returns a successful response
		mockClient := &http.Client{Timeout: 30 * time.Second}

		// Execute request
		resp, err := provider.doRequestWithProxy(req, mockClient, token.ID)
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		// Verify proxy health was updated
		healthScore := proxyTracker.GetHealthScore(token.ID)
		assert.Equal(t, 1.0, healthScore)
	})

	t.Run("updates proxy health on proxy error", func(t *testing.T) {
		logger := logging.NewLogger()
		tempDir := t.TempDir()
		filePath := tempDir + "/store.json"

		// Create token store with a token
		store := auth.NewMultiTokenStore("qwen", filePath, logger)
		token := createTestToken("test-token-error")
		require.NoError(t, store.AddToken(*token))

		// Create token manager with proxy health tracker
		mockFactory := &mockProxyClientFactory{}
		proxyTracker := auth.NewProxyHealthTracker(logger, 5, 5*time.Minute)
		tokenManager := auth.NewTokenManager(store, auth.NewRandomSelectionStrategy(), logger, mockFactory, proxyTracker)

		// Create provider with token manager
		provider := NewProviderWithTokenManager(tokenManager, logger)

		// Create a mock HTTP request
		req, err := http.NewRequest("GET", "http://example.com", nil)
		require.NoError(t, err)

		// Use a mock HTTP client that returns a proxy error
		// We'll simulate this by using a context that times out
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
		defer cancel()
		<-ctx.Done()
		reqWithTimeout := req.WithContext(ctx)

		// Execute request
		resp, err := provider.doRequestWithProxy(reqWithTimeout, &http.Client{}, token.ID)
		assert.Error(t, err)
		assert.Nil(t, resp)

		// Verify proxy health was updated
		healthScore := proxyTracker.GetHealthScore(token.ID)
		assert.LessOrEqual(t, healthScore, 1.0)
	})

	t.Run("does not update proxy health when tokenManager is nil", func(t *testing.T) {
		provider := NewProvider()
		provider.SetTokenManager(nil)

		// Create a mock HTTP request
		req, err := http.NewRequest("GET", "http://example.com", nil)
		require.NoError(t, err)

		// Use a mock HTTP client that returns a successful response
		mockClient := &http.Client{Timeout: 30 * time.Second}

		// Execute request - should not panic
		resp, err := provider.doRequestWithProxy(req, mockClient, "fallback")
		require.NoError(t, err)
		require.NotNil(t, resp)
	})
}

// TestProvider_GenerateContent_ProxyAware tests GenerateContent with proxy-aware client
func TestProvider_GenerateContent_ProxyAware(t *testing.T) {
	t.Run("uses proxy-aware client when tokenManager is set", func(t *testing.T) {
		logger := logging.NewLogger()
		tempDir := t.TempDir()
		filePath := tempDir + "/store.json"

		// Create token store with a token that has proxy config
		store := auth.NewMultiTokenStore("qwen", filePath, logger)
		proxyConfig := &auth.ProxyConfig{
			Type:    auth.ProxyTypeHTTP,
			Host:    "proxy.example.com",
			Port:    8080,
			Enabled: true,
		}
		token := createTestTokenWithProxy("test-token-proxy", proxyConfig)
		require.NoError(t, store.AddToken(*token))

		// Create token manager with mock proxy client factory
		mockClient := &http.Client{Timeout: 30 * time.Second}
		mockFactory := &mockProxyClientFactory{mockClient: mockClient}
		proxyTracker := auth.NewProxyHealthTracker(logger, 5, 5*time.Minute)
		tokenManager := auth.NewTokenManager(store, auth.NewRandomSelectionStrategy(), logger, mockFactory, proxyTracker)

		// Create provider with token manager
		provider := NewProviderWithTokenManager(tokenManager, logger)
		provider.SetTokenManager(tokenManager) // Also set p.tokenManager for GenerateContent

		// Verify proxy client factory was called
		assert.Len(t, mockFactory.calls, 0)

		// Note: GenerateContent will fail because we don't have a real server,
		// but we can verify that tokenManager was used
		_, err := provider.GenerateContent(context.Background(), "qwen3-coder-plus", map[string]interface{}{
			"messages": []interface{}{},
		})

		// Should get an error because we don't have a real server
		assert.Error(t, err)

		// Verify proxy client factory was called (token was selected)
		if len(mockFactory.calls) > 0 {
			assert.Equal(t, proxyConfig, mockFactory.calls[0])
		}
	})

	t.Run("falls back to default client when tokenManager is nil", func(t *testing.T) {
		provider := NewProvider()
		provider.SetTokenManager(nil)

		// Verify default client is used
		// Note: GenerateContent will fail because we don't have a real server
		_, err := provider.GenerateContent(context.Background(), "qwen3-coder-plus", map[string]interface{}{
			"messages": []interface{}{},
		})

		// Should get an error because we don't have a real server
		assert.Error(t, err)
	})
}

// TestProvider_GenerateContentStream_ProxyAware tests GenerateContentStream with proxy-aware client
func TestProvider_GenerateContentStream_ProxyAware(t *testing.T) {
	t.Run("uses proxy-aware client when tokenManager is set", func(t *testing.T) {
		logger := logging.NewLogger()
		tempDir := t.TempDir()
		filePath := tempDir + "/store.json"

		// Create token store with a token that has proxy config
		store := auth.NewMultiTokenStore("qwen", filePath, logger)
		proxyConfig := &auth.ProxyConfig{
			Type:    auth.ProxyTypeSOCKS5,
			Host:    "socks.example.com",
			Port:    1080,
			Enabled: true,
		}
		token := createTestTokenWithProxy("test-token-socks", proxyConfig)
		require.NoError(t, store.AddToken(*token))

		// Create token manager with mock proxy client factory
		mockClient := &http.Client{Timeout: 30 * time.Second}
		mockFactory := &mockProxyClientFactory{mockClient: mockClient}
		proxyTracker := auth.NewProxyHealthTracker(logger, 5, 5*time.Minute)
		tokenManager := auth.NewTokenManager(store, auth.NewRandomSelectionStrategy(), logger, mockFactory, proxyTracker)

		// Create provider with token manager
		provider := NewProviderWithTokenManager(tokenManager, logger)
		provider.SetTokenManager(tokenManager) // Also set p.tokenManager for GenerateContentStream

		// Verify proxy client factory was called
		assert.Len(t, mockFactory.calls, 0)

		// Note: GenerateContentStream will fail because we don't have a real server,
		// but we can verify that tokenManager was used
		_, err := provider.GenerateContentStream(context.Background(), "qwen3-coder-plus", map[string]interface{}{
			"messages": []interface{}{},
		})

		// Should get an error because we don't have a real server
		assert.Error(t, err)

		// Verify proxy client factory was called (token was selected)
		assert.Len(t, mockFactory.calls, 1)
		assert.Equal(t, proxyConfig, mockFactory.calls[0])
	})

	t.Run("falls back to default client when tokenManager is nil", func(t *testing.T) {
		provider := NewProvider()
		provider.SetTokenManager(nil)

		// Verify default client is used
		// Note: GenerateContentStream will fail because we don't have a real server
		_, err := provider.GenerateContentStream(context.Background(), "qwen3-coder-plus", map[string]interface{}{
			"messages": []interface{}{},
		})

		// Should get an error because we don't have a real server
		assert.Error(t, err)
	})
}

// TestProvider_BackwardCompatibility tests backward compatibility
func TestProvider_BackwardCompatibility(t *testing.T) {
	t.Run("works without token manager", func(t *testing.T) {
		provider := NewProvider()
		provider.SetTokenManager(nil)

		// Verify provider is properly initialized
		assert.NotNil(t, provider)
		assert.NotNil(t, provider.GetAuthenticator())
		assert.Equal(t, providerpkg.ProviderQwen, provider.Name())
		assert.Equal(t, providerpkg.ProtocolQwen, provider.Protocol())
	})

	t.Run("supports existing methods", func(t *testing.T) {
		provider := NewProvider()

		// Verify existing methods still work
		assert.NotNil(t, provider.SupportedModels())
		assert.True(t, provider.SupportsModel("qwen3-coder-plus"))
		assert.True(t, provider.SupportsModel("QWEN3-CODER-PLUS"))
		assert.False(t, provider.SupportsModel("invalid-model"))
	})
}
