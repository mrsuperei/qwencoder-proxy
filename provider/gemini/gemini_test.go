package gemini

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/sunbankio/qwencoder-proxy/auth"
	"github.com/sunbankio/qwencoder-proxy/logging"
	providerpkg "github.com/sunbankio/qwencoder-proxy/provider"
)

// TestIsProxyError tests the isProxyError method
func TestIsProxyError(t *testing.T) {
	provider := &Provider{
		BaseProvider: providerpkg.NewBaseProvider(logging.NewLogger(), 5*time.Minute),
	}

	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "timeout error",
			err:      &timeoutError{},
			expected: true,
		},
		{
			name:     "connection refused",
			err:      errors.New("connection refused"),
			expected: true,
		},
		{
			name:     "connection reset",
			err:      errors.New("connection reset by peer"),
			expected: true,
		},
		{
			name:     "broken pipe",
			err:      errors.New("broken pipe"),
			expected: true,
		},
		{
			name:     "EOF",
			err:      errors.New("EOF"),
			expected: true,
		},
		{
			name:     "DNS error - no such host",
			err:      errors.New("no such host"),
			expected: true,
		},
		{
			name:     "DNS error - lookup",
			err:      errors.New("lookup failed"),
			expected: true,
		},
		{
			name:     "proxy error",
			err:      errors.New("proxy connection failed"),
			expected: true,
		},
		{
			name:     "SOCKS error",
			err:      errors.New("SOCKS connection failed"),
			expected: true,
		},
		{
			name:     "tunnel error",
			err:      errors.New("tunnel connection failed"),
			expected: true,
		},
		{
			name:     "URL error with proxy error",
			err:      &url.Error{Err: errors.New("connection refused")},
			expected: true,
		},
		{
			name:     "non-proxy error",
			err:      errors.New("some other error"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := provider.isProxyError(tt.err)
			if result != tt.expected {
				t.Errorf("isProxyError() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

// timeoutError implements net.Error for testing timeout errors
type timeoutError struct{}

func (e *timeoutError) Error() string   { return "timeout" }
func (e *timeoutError) Timeout() bool   { return true }
func (e *timeoutError) Temporary() bool { return false }

// TestClassifyProxyError tests the classifyProxyError method
func TestClassifyProxyError(t *testing.T) {
	provider := &Provider{
		BaseProvider: providerpkg.NewBaseProvider(logging.NewLogger(), 5*time.Minute),
	}

	tests := []struct {
		name     string
		err      error
		expected string
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: "none",
		},
		{
			name:     "timeout error",
			err:      &timeoutError{},
			expected: "timeout",
		},
		{
			name:     "connection refused",
			err:      errors.New("connection refused"),
			expected: "connection_refused",
		},
		{
			name:     "connection reset",
			err:      errors.New("connection reset"),
			expected: "connection_reset",
		},
		{
			name:     "broken pipe",
			err:      errors.New("broken pipe"),
			expected: "broken_pipe",
		},
		{
			name:     "EOF",
			err:      errors.New("EOF"),
			expected: "eof",
		},
		{
			name:     "DNS error - no such host",
			err:      errors.New("no such host"),
			expected: "dns_resolution",
		},
		{
			name:     "DNS error - lookup",
			err:      errors.New("lookup failed"),
			expected: "dns_resolution",
		},
		{
			name:     "proxy error",
			err:      errors.New("proxy connection failed"),
			expected: "proxy_error",
		},
		{
			name:     "SOCKS error",
			err:      errors.New("SOCKS connection failed"),
			expected: "socks_error",
		},
		{
			name:     "tunnel error",
			err:      errors.New("tunnel connection failed"),
			expected: "tunnel_error",
		},
		{
			name:     "unknown error",
			err:      errors.New("some other error"),
			expected: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := provider.classifyProxyError(tt.err)
			if result != tt.expected {
				t.Errorf("classifyProxyError() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

// TestDoRequestWithProxy_Success tests successful request with proxy
func TestDoRequestWithProxy_Success(t *testing.T) {
	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"success": true}`))
	}))
	defer server.Close()

	// Create a mock token manager with proxy health tracker
	mockTracker := auth.NewProxyHealthTracker(logging.NewLogger(), 5, 5*time.Minute)
	mockTracker.UpdateHealth("test-token", true, nil)

	// Create a mock token store
	store := auth.NewMultiTokenStore("test", ".test-tokens-do-request.json", logging.NewLogger())
	token := auth.ProviderToken{
		ID:          "test-token",
		AccessToken: "test-access-token",
		Email:       "test@example.com",
		ExpiryDate:  time.Now().Add(time.Hour).UnixMilli(),
		Healthy:     true,
		HealthScore: 1.0,
	}
	if err := store.AddToken(token); err != nil {
		t.Fatal(err)
	}

	// Create a mock token manager with proxy health tracker
	strategy := auth.NewRandomSelectionStrategy()
	tokenManager := auth.NewTokenManager(store, strategy, logging.NewLogger(), nil, mockTracker)

	base := providerpkg.NewBaseProvider(logging.NewLogger(), 5*time.Minute)
	base.SetTokenManager(tokenManager)
	provider := &Provider{
		BaseProvider: base,
	}

	req, _ := http.NewRequest("GET", server.URL, nil)
	resp, err := provider.doRequestWithProxy(req, server.Client(), "test-token")

	if err != nil {
		t.Fatalf("doRequestWithProxy() error = %v", err)
	}

	if resp == nil {
		t.Fatal("doRequestWithProxy() returned nil response")
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("doRequestWithProxy() status = %v, expected %v", resp.StatusCode, http.StatusOK)
	}

	resp.Body.Close()
}

// TestDoRequestWithProxy_ProxyError tests proxy error handling
func TestDoRequestWithProxy_ProxyError(t *testing.T) {
	// Create a test server that will reject connections
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	// Close immediately to simulate connection refused
	listener.Close()

	// Create a mock token manager with proxy health tracker
	mockTracker := auth.NewProxyHealthTracker(logging.NewLogger(), 5, 5*time.Minute)
	mockTracker.UpdateHealth("test-token", true, nil)

	// Create a mock token store
	store := auth.NewMultiTokenStore("test", ".test-tokens-do-request-error.json", logging.NewLogger())
	token := auth.ProviderToken{
		ID:          "test-token",
		AccessToken: "test-access-token",
		Email:       "test@example.com",
		ExpiryDate:  time.Now().Add(time.Hour).UnixMilli(),
		Healthy:     true,
		HealthScore: 1.0,
	}
	if err := store.AddToken(token); err != nil {
		t.Fatal(err)
	}

	// Create a mock token manager with proxy health tracker
	strategy := auth.NewRandomSelectionStrategy()
	tokenManager := auth.NewTokenManager(store, strategy, logging.NewLogger(), nil, mockTracker)

	base := providerpkg.NewBaseProvider(logging.NewLogger(), 5*time.Minute)
	base.SetTokenManager(tokenManager)
	provider := &Provider{
		BaseProvider: base,
	}

	req, _ := http.NewRequest("GET", "http://"+listener.Addr().String(), nil)
	resp, err := provider.doRequestWithProxy(req, http.DefaultClient, "test-token")

	if err == nil {
		t.Fatal("doRequestWithProxy() expected error, got nil")
	}

	if resp != nil {
		t.Error("doRequestWithProxy() returned non-nil response on error")
	}
}

// TestGenerateContent_WithTokenManager tests GenerateContent with token manager
func TestGenerateContent_WithTokenManager(t *testing.T) {
	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"response": {
				"candidates": [{
					"content": {
						"parts": [{"text": "Hello"}]
					}
				}]
			}
		}`))
	}))
	defer server.Close()

	// Create a mock token store with a token
	store := auth.NewMultiTokenStore("test", ".test-tokens.json", logging.NewLogger())
	token := auth.ProviderToken{
		ID:          "test-token",
		AccessToken: "test-access-token",
		Email:       "test@example.com",
		ExpiryDate:  time.Now().Add(time.Hour).UnixMilli(),
		Healthy:     true,
		HealthScore: 1.0,
		Proxy:       nil, // Direct connection
	}
	if err := store.AddToken(token); err != nil {
		t.Fatal(err)
	}

	// Create a mock token manager
	strategy := auth.NewRandomSelectionStrategy()
	tokenManager := auth.NewTokenManager(store, strategy, logging.NewLogger(), nil, nil)

	// Create provider with token manager
	provider := &Provider{
		baseURL:       strings.TrimSuffix(server.URL, "/") + "/v1internal",
		authenticator: nil,
		BaseProvider: func() *providerpkg.BaseProvider {
			base := providerpkg.NewBaseProvider(logging.NewLogger(), 5*time.Minute)
			base.SetHTTPClient(server.Client())
			base.SetTokenManager(tokenManager)
			return base
		}(),
		projectID: "test-project",
	}

	// Test request
	request := map[string]interface{}{
		"contents": []map[string]interface{}{
			{
				"role": "user",
				"parts": []map[string]interface{}{
					{"text": "Hello"},
				},
			},
		},
	}

	ctx := context.Background()
	resp, err := provider.GenerateContent(ctx, "gemini-2.5-flash", request)

	if err != nil {
		t.Fatalf("GenerateContent() error = %v", err)
	}

	if resp == nil {
		t.Fatal("GenerateContent() returned nil response")
	}
}

// TestGenerateContent_WithProxy tests GenerateContent with proxy configuration
func TestGenerateContent_WithProxy(t *testing.T) {
	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"response": {
				"candidates": [{
					"content": {
						"parts": [{"text": "Hello"}]
					}
				}]
			}
		}`))
	}))
	defer server.Close()

	// Create a mock token store with a token and proxy config
	store := auth.NewMultiTokenStore("test", ".test-tokens-proxy.json", logging.NewLogger())
	proxyConfig := &auth.ProxyConfig{
		Type:    auth.ProxyTypeHTTP,
		Host:    "127.0.0.1",
		Port:    8080,
		Enabled: true,
	}
	token := auth.ProviderToken{
		ID:          "test-token-proxy",
		AccessToken: "test-access-token",
		Email:       "test@example.com",
		ExpiryDate:  time.Now().Add(time.Hour).UnixMilli(),
		Healthy:     true,
		HealthScore: 1.0,
		Proxy:       proxyConfig,
	}
	if err := store.AddToken(token); err != nil {
		t.Fatal(err)
	}

	// Create a mock client factory
	clientFactory := &mockClientFactory{
		clients: make(map[string]*http.Client),
	}

	// Create a mock token manager
	strategy := auth.NewRandomSelectionStrategy()
	tokenManager := auth.NewTokenManager(store, strategy, logging.NewLogger(), clientFactory, nil)

	// Create provider with token manager
	provider := &Provider{
		baseURL:       strings.TrimSuffix(server.URL, "/") + "/v1internal",
		authenticator: nil,
		BaseProvider: func() *providerpkg.BaseProvider {
			base := providerpkg.NewBaseProvider(logging.NewLogger(), 5*time.Minute)
			base.SetHTTPClient(server.Client())
			base.SetTokenManager(tokenManager)
			return base
		}(),
		projectID: "test-project",
	}

	// Test request
	request := map[string]interface{}{
		"contents": []map[string]interface{}{
			{
				"role": "user",
				"parts": []map[string]interface{}{
					{"text": "Hello"},
				},
			},
		},
	}

	ctx := context.Background()
	resp, err := provider.GenerateContent(ctx, "gemini-2.5-flash", request)

	if err != nil {
		t.Fatalf("GenerateContent() error = %v", err)
	}

	if resp == nil {
		t.Fatal("GenerateContent() returned nil response")
	}
}

// TestGenerateContent_WithoutTokenManager tests backward compatibility
func TestGenerateContent_WithoutTokenManager(t *testing.T) {
	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"response": {
				"candidates": [{
					"content": {
						"parts": [{"text": "Hello"}]
					}
				}]
			}
		}`))
	}))
	defer server.Close()

	// Create a mock authenticator
	authenticator := auth.NewGeminiAuthenticator(nil)

	// Create provider without token manager
	provider := &Provider{
		baseURL:       strings.TrimSuffix(server.URL, "/") + "/v1internal",
		authenticator: authenticator,
		BaseProvider: func() *providerpkg.BaseProvider {
			base := providerpkg.NewBaseProvider(logging.NewLogger(), 5*time.Minute)
			base.SetHTTPClient(server.Client())
			return base
		}(),
		projectID: "test-project",
	}

	// Test request
	request := map[string]interface{}{
		"contents": []map[string]interface{}{
			{
				"role": "user",
				"parts": []map[string]interface{}{
					{"text": "Hello"},
				},
			},
		},
	}

	ctx := context.Background()
	resp, err := provider.GenerateContent(ctx, "gemini-2.5-flash", request)

	// This should fail because we don't have a valid token
	// but it should not panic
	if err == nil {
		t.Error("GenerateContent() expected error without valid token, got nil")
	}

	if resp != nil {
		t.Error("GenerateContent() returned non-nil response without valid token")
	}
}

// TestGenerateContentStream_WithTokenManager tests streaming with token manager
func TestGenerateContentStream_WithTokenManager(t *testing.T) {
	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"Hello\"}]}}}\n\n"))
	}))
	defer server.Close()

	// Create a mock token store with a token
	store := auth.NewMultiTokenStore("test", ".test-tokens-stream.json", logging.NewLogger())
	token := auth.ProviderToken{
		ID:          "test-token-stream",
		AccessToken: "test-access-token",
		Email:       "test@example.com",
		ExpiryDate:  time.Now().Add(time.Hour).UnixMilli(),
		Healthy:     true,
		HealthScore: 1.0,
		Proxy:       nil,
	}
	if err := store.AddToken(token); err != nil {
		t.Fatal(err)
	}

	// Create a mock token manager
	strategy := auth.NewRandomSelectionStrategy()
	tokenManager := auth.NewTokenManager(store, strategy, logging.NewLogger(), nil, nil)

	// Create provider with token manager
	provider := &Provider{
		baseURL:       strings.TrimSuffix(server.URL, "/") + "/v1internal",
		authenticator: nil,
		BaseProvider: func() *providerpkg.BaseProvider {
			base := providerpkg.NewBaseProvider(logging.NewLogger(), 5*time.Minute)
			base.SetHTTPClient(server.Client())
			base.SetTokenManager(tokenManager)
			return base
		}(),
		projectID: "test-project",
	}

	// Test request
	request := map[string]interface{}{
		"contents": []map[string]interface{}{
			{
				"role": "user",
				"parts": []map[string]interface{}{
					{"text": "Hello"},
				},
			},
		},
	}

	ctx := context.Background()
	stream, err := provider.GenerateContentStream(ctx, "gemini-2.5-flash", request)

	if err != nil {
		t.Fatalf("GenerateContentStream() error = %v", err)
	}

	if stream == nil {
		t.Fatal("GenerateContentStream() returned nil stream")
	}

	defer stream.Close()
}

// mockClientFactory is a mock implementation of ProxyClientFactory
type mockClientFactory struct {
	clients map[string]*http.Client
}

func (m *mockClientFactory) GetClient(proxyConfig *auth.ProxyConfig) *http.Client {
	key := "direct"
	if proxyConfig != nil {
		key = fmt.Sprintf("%s:%s:%d", proxyConfig.Type, proxyConfig.Host, proxyConfig.Port)
	}

	if client, ok := m.clients[key]; ok {
		return client
	}

	// Create a new client
	client := &http.Client{Timeout: 30 * time.Second}
	m.clients[key] = client
	return client
}

// TestProxyHealthTracking tests proxy health tracking on errors
func TestProxyHealthTracking(t *testing.T) {
	// Create a mock proxy health tracker
	tracker := auth.NewProxyHealthTracker(logging.NewLogger(), 5, 5*time.Minute)

	// Create a mock token manager with tracker
	store := auth.NewMultiTokenStore("test", ".test-tokens-health.json", logging.NewLogger())
	token := auth.ProviderToken{
		ID:          "test-token-health",
		AccessToken: "test-access-token",
		Email:       "test@example.com",
		ExpiryDate:  time.Now().Add(time.Hour).UnixMilli(),
		Healthy:     true,
		HealthScore: 1.0,
	}
	if err := store.AddToken(token); err != nil {
		t.Fatal(err)
	}

	strategy := auth.NewRandomSelectionStrategy()
	tokenManager := auth.NewTokenManager(store, strategy, logging.NewLogger(), nil, tracker)

	base := providerpkg.NewBaseProvider(logging.NewLogger(), 5*time.Minute)
	base.SetTokenManager(tokenManager)
	provider := &Provider{
		BaseProvider: base,
	}

	// Test successful health update
	err := provider.GetTokenManager().UpdateProxyHealth("test-token-health", true, nil)
	if err != nil {
		t.Errorf("UpdateProxyHealth(success) error = %v", err)
	}

	healthScore := tracker.GetHealthScore("test-token-health")
	if healthScore != 1.0 {
		t.Errorf("Expected health score 1.0 after success, got %f", healthScore)
	}

	// Test failure health update
	proxyErr := errors.New("connection refused")
	err = provider.GetTokenManager().UpdateProxyHealth("test-token-health", false, proxyErr)
	if err != nil {
		t.Errorf("UpdateProxyHealth(failure) error = %v", err)
	}

	healthScore = tracker.GetHealthScore("test-token-health")
	if healthScore >= 1.0 {
		t.Errorf("Expected health score < 1.0 after failure, got %f", healthScore)
	}

	healthStatus := tracker.GetHealthStatus("test-token-health")
	if healthStatus == nil {
		t.Fatal("GetHealthStatus() returned nil")
	}

	if healthStatus.IsHealthy {
		t.Error("Expected IsHealthy to be false after failure")
	}
}

// TestSetTokenManager tests setting the token manager
func TestSetTokenManager(t *testing.T) {
	provider := NewProvider(nil)

	if provider.GetTokenManager() != nil {
		t.Error("Expected tokenManager to be nil initially")
	}

	// Create a mock token manager
	store := auth.NewMultiTokenStore("test", ".test-tokens-set.json", logging.NewLogger())
	strategy := auth.NewRandomSelectionStrategy()
	tokenManager := auth.NewTokenManager(store, strategy, logging.NewLogger(), nil, nil)

	provider.SetTokenManager(tokenManager)

	if provider.GetTokenManager() == nil {
		t.Error("Expected tokenManager to be set")
	}

	if provider.GetTokenManager() != tokenManager {
		t.Error("tokenManager not set correctly")
	}
}
