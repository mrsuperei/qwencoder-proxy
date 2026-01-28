package auth

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/sunbankio/qwencoder-proxy/logging"
)

// geminiMockProxyClientFactory is a mock implementation of ProxyClientFactory for testing
type geminiMockProxyClientFactory struct {
	mockClient *http.Client
	calls      []*ProxyConfig
}

func (m *geminiMockProxyClientFactory) GetClient(proxyConfig *ProxyConfig) *http.Client {
	m.calls = append(m.calls, proxyConfig)
	if m.mockClient != nil {
		return m.mockClient
	}
	return &http.Client{}
}

func createGeminiTestToken(id string) *ProviderToken {
	now := time.Now()
	return &ProviderToken{
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

func createGeminiTestTokenWithProxy(id string, proxyConfig *ProxyConfig) *ProviderToken {
	token := createGeminiTestToken(id)
	token.Proxy = proxyConfig
	return token
}

func TestGeminiAuthenticator_GetTokenWithClient(t *testing.T) {
	t.Run("returns token and proxy-aware client when token manager is set", func(t *testing.T) {
		tempDir := t.TempDir()
		filePath := tempDir + "/store.json"

		// Create token store with a token that has proxy config
		store := NewMultiTokenStore("gemini", filePath, logging.NewLogger())
		proxyConfig := &ProxyConfig{
			Type:    ProxyTypeHTTP,
			Host:    "proxy.example.com",
			Port:    8080,
			Enabled: true,
		}
		token := createGeminiTestTokenWithProxy("test-token-1", proxyConfig)
		require.NoError(t, store.AddToken(*token))

		// Create token manager with mock proxy client factory
		mockClient := &http.Client{Timeout: 30 * time.Second}
		mockFactory := &geminiMockProxyClientFactory{mockClient: mockClient}
		proxyTracker := NewProxyHealthTracker(logging.NewLogger(), 5, 5*time.Minute)
		tokenManager := NewTokenManager(store, NewRandomSelectionStrategy(), logging.NewLogger(), mockFactory, proxyTracker)

		// Create authenticator with token manager
		authenticator := NewGeminiAuthenticator(nil)
		authenticator.SetTokenManager(tokenManager)

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
		authenticator := NewGeminiAuthenticator(nil)

		// Get token with client when token manager is nil
		ctx := context.Background()
		accessToken, client, err := authenticator.GetTokenWithClient(ctx)

		// Should fall back to GetToken which will fail
		assert.Error(t, err)
		assert.Empty(t, accessToken)
		assert.Nil(t, client)
	})

	t.Run("returns token with direct connection when token has no proxy", func(t *testing.T) {
		tempDir := t.TempDir()
		filePath := tempDir + "/store.json"

		// Create token store with a token that has no proxy config
		store := NewMultiTokenStore("gemini", filePath, logging.NewLogger())
		token := createGeminiTestToken("test-token-2")
		require.NoError(t, store.AddToken(*token))

		// Create token manager with mock proxy client factory
		mockFactory := &geminiMockProxyClientFactory{}
		proxyTracker := NewProxyHealthTracker(logging.NewLogger(), 5, 5*time.Minute)
		tokenManager := NewTokenManager(store, NewRandomSelectionStrategy(), logging.NewLogger(), mockFactory, proxyTracker)

		// Create authenticator with token manager
		authenticator := NewGeminiAuthenticator(nil)
		authenticator.SetTokenManager(tokenManager)

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
		tempDir := t.TempDir()
		filePath := tempDir + "/store.json"

		// Create empty token store
		store := NewMultiTokenStore("gemini", filePath, logging.NewLogger())
		mockFactory := &geminiMockProxyClientFactory{}
		proxyTracker := NewProxyHealthTracker(logging.NewLogger(), 5, 5*time.Minute)
		tokenManager := NewTokenManager(store, NewRandomSelectionStrategy(), logging.NewLogger(), mockFactory, proxyTracker)

		// Create authenticator with token manager
		authenticator := NewGeminiAuthenticator(nil)
		authenticator.SetTokenManager(tokenManager)

		// Get token with client
		ctx := context.Background()
		accessToken, client, err := authenticator.GetTokenWithClient(ctx)

		assert.Error(t, err)
		assert.Empty(t, accessToken)
		assert.Nil(t, client)
	})
}

func TestGeminiAuthenticator_GetHTTPClient(t *testing.T) {
	t.Run("returns proxy-aware client when token manager is set", func(t *testing.T) {
		tempDir := t.TempDir()
		filePath := tempDir + "/store.json"

		// Create token store with a token that has proxy config
		store := NewMultiTokenStore("gemini", filePath, logging.NewLogger())
		proxyConfig := &ProxyConfig{
			Type:    ProxyTypeSOCKS5,
			Host:    "socks.example.com",
			Port:    1080,
			Enabled: true,
		}
		token := createGeminiTestTokenWithProxy("test-token-3", proxyConfig)
		require.NoError(t, store.AddToken(*token))

		// Create token manager with mock proxy client factory
		mockClient := &http.Client{Timeout: 30 * time.Second}
		mockFactory := &geminiMockProxyClientFactory{mockClient: mockClient}
		proxyTracker := NewProxyHealthTracker(logging.NewLogger(), 5, 5*time.Minute)
		tokenManager := NewTokenManager(store, NewRandomSelectionStrategy(), logging.NewLogger(), mockFactory, proxyTracker)

		// Create authenticator with token manager
		authenticator := NewGeminiAuthenticator(nil)
		authenticator.SetTokenManager(tokenManager)

		// Get HTTP client
		client, err := authenticator.GetHTTPClient()

		require.NoError(t, err)
		assert.NotNil(t, client)
		assert.Equal(t, mockClient, client)
		assert.Len(t, mockFactory.calls, 1)
		assert.Equal(t, proxyConfig, mockFactory.calls[0])
	})

	t.Run("returns error when token manager is nil", func(t *testing.T) {
		authenticator := NewGeminiAuthenticator(nil)

		// Get HTTP client when token manager is nil
		client, err := authenticator.GetHTTPClient()

		assert.Error(t, err)
		assert.Nil(t, client)
		assert.Contains(t, err.Error(), "token manager not initialized")
	})

	t.Run("returns error when no tokens available", func(t *testing.T) {
		tempDir := t.TempDir()
		filePath := tempDir + "/store.json"

		// Create empty token store
		store := NewMultiTokenStore("gemini", filePath, logging.NewLogger())
		mockFactory := &geminiMockProxyClientFactory{}
		proxyTracker := NewProxyHealthTracker(logging.NewLogger(), 5, 5*time.Minute)
		tokenManager := NewTokenManager(store, NewRandomSelectionStrategy(), logging.NewLogger(), mockFactory, proxyTracker)

		// Create authenticator with token manager
		authenticator := NewGeminiAuthenticator(nil)
		authenticator.SetTokenManager(tokenManager)

		// Get HTTP client
		client, err := authenticator.GetHTTPClient()

		assert.Error(t, err)
		assert.Nil(t, client)
	})
}

func TestGeminiAuthenticator_BackwardCompatibility(t *testing.T) {
	t.Run("existing GetToken method still works", func(t *testing.T) {
		tempDir := t.TempDir()
		filePath := tempDir + "/store.json"

		// Create token store with a token
		store := NewMultiTokenStore("gemini", filePath, logging.NewLogger())
		token := createGeminiTestToken("test-token-4")
		require.NoError(t, store.AddToken(*token))

		// Create token manager with mock proxy client factory
		mockFactory := &geminiMockProxyClientFactory{}
		proxyTracker := NewProxyHealthTracker(logging.NewLogger(), 5, 5*time.Minute)
		tokenManager := NewTokenManager(store, NewRandomSelectionStrategy(), logging.NewLogger(), mockFactory, proxyTracker)

		// Create authenticator with token manager
		authenticator := NewGeminiAuthenticator(nil)
		authenticator.SetTokenManager(tokenManager)

		// Get token using existing method
		ctx := context.Background()
		accessToken, err := authenticator.GetToken(ctx)

		require.NoError(t, err)
		assert.Equal(t, "test-access-token-test-token-4", accessToken)
	})

	t.Run("existing IsAuthenticated method still works", func(t *testing.T) {
		tempDir := t.TempDir()
		filePath := tempDir + "/store.json"

		// Create token store with a token
		store := NewMultiTokenStore("gemini", filePath, logging.NewLogger())
		token := createGeminiTestToken("test-token-5")
		require.NoError(t, store.AddToken(*token))

		// Create token manager
		tokenManager := NewTokenManager(store, NewRandomSelectionStrategy(), logging.NewLogger(), nil, nil)

		// Create authenticator with token manager
		authenticator := NewGeminiAuthenticator(nil)
		authenticator.SetTokenManager(tokenManager)

		// Check authentication status
		isAuthenticated := authenticator.IsAuthenticated()

		assert.True(t, isAuthenticated)
	})
}
