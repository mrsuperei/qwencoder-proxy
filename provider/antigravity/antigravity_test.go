package antigravity

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

	tokpkg "github.com/sunbankio/qwencoder-proxy/internal/token"
)

// mockProxyClientFactory is a mock implementation of ProxyClientFactory
type mockProxyClientFactory struct {
	clients map[string]*http.Client
}

func newMockClientFactory() *mockProxyClientFactory {
	return &mockProxyClientFactory{
		clients: make(map[string]*http.Client),
	}
}

func (m *mockProxyClientFactory) GetClient(proxyConfig *tokpkg.ProxyConfig) *http.Client {
	if proxyConfig == nil {
		return http.DefaultClient
	}
	key := fmt.Sprintf("%s:%d", proxyConfig.Host, proxyConfig.Port)
	if client, ok := m.clients[key]; ok {
		return client
	}
	client := &http.Client{Timeout: 30 * time.Second}
	m.clients[key] = client
	return client
}

// TestIsProxyError tests the isProxyError helper method
func TestIsProxyError(t *testing.T) {
	provider := NewProvider(nil)

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
			name:     "DNS error",
			err:      errors.New("no such host"),
			expected: true,
		},
		{
			name:     "lookup error",
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
			err:      errors.New("SOCKS proxy error"),
			expected: true,
		},
		{
			name:     "tunnel error",
			err:      errors.New("tunnel connection failed"),
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

// timeoutError is a mock timeout error for testing
type timeoutError struct{}

func (e *timeoutError) Error() string   { return "timeout" }
func (e *timeoutError) Timeout() bool   { return true }
func (e *timeoutError) Temporary() bool { return true }

// TestClassifyProxyError tests the classifyProxyError helper method
func TestClassifyProxyError(t *testing.T) {
	provider := NewProvider(nil)

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
			err:      &url.Error{Err: &timeoutError{}},
			expected: "timeout",
		},
		{
			name:     "connection refused",
			err:      errors.New("connection refused"),
			expected: "connection_refused",
		},
		{
			name:     "connection reset",
			err:      errors.New("connection reset by peer"),
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
			name:     "DNS error",
			err:      errors.New("no such host"),
			expected: "dns_resolution",
		},
		{
			name:     "proxy error",
			err:      errors.New("proxy connection failed"),
			expected: "proxy_error",
		},
		{
			name:     "SOCKS error",
			err:      errors.New("SOCKS proxy error"),
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

// TestDoRequestWithProxy tests the doRequestWithProxy helper method
func TestDoRequestWithProxy(t *testing.T) {
	tests := []struct {
		name           string
		tokenManager   *tokpkg.TokenManager
		expectProxyErr bool
	}{
		{
			name:           "successful request",
			tokenManager:   nil,
			expectProxyErr: false,
		},
		{
			name:           "proxy error",
			tokenManager:   nil,
			expectProxyErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := NewProvider(nil)
			provider.SetTokenManager(tt.tokenManager)

			var server *httptest.Server
			if tt.expectProxyErr {
				// Create a listener that will cause connection refused error
				listener, err := net.Listen("tcp", "127.0.0.1:0")
				if err != nil {
					t.Fatalf("Failed to create listener: %v", err)
				}
				// Close immediately to simulate connection refused
				listener.Close()
				// Create a dummy server for URL construction
				server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
					w.Write([]byte(`{"response": {}}`))
				}))
				defer server.Close()
				// Use the listener's address instead
				req, err := http.NewRequest("GET", "http://"+listener.Addr().String(), nil)
				if err != nil {
					t.Fatalf("Failed to create request: %v", err)
				}
				client := http.DefaultClient
				resp, err := provider.doRequestWithProxy(req, client, "test-token")
				if err == nil {
					t.Error("Expected error but got none")
				}
				if resp != nil {
					resp.Body.Close()
				}
				return
			} else {
				server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
					w.Write([]byte(`{"response": {}}`))
				}))
			}
			defer server.Close()

			req, err := http.NewRequest("GET", server.URL, nil)
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}

			client := http.DefaultClient
			resp, err := provider.doRequestWithProxy(req, client, "test-token")

			if tt.expectProxyErr {
				if err == nil {
					t.Error("Expected error but got none")
				}
				if resp != nil {
					resp.Body.Close()
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if resp == nil {
					t.Error("Expected response but got nil")
				} else {
					resp.Body.Close()
				}
			}
		})
	}
}

// TestGenerateContentWithTokenManager tests GenerateContent with token manager
func TestGenerateContentWithTokenManager(t *testing.T) {
	// Create a mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check for authorization header
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"response": {
				"candidates": [{
					"content": {
						"parts": [{"text": "test response"}]
					}
				}]
			}
		}`))
	}))
	defer server.Close()

	// Create provider
	provider := NewProvider(nil)
	provider.dailyBaseURL = server.URL
	provider.isInitialized = true
	provider.projectID = "test-project"

	// Test without token manager (backward compatibility)
	t.Run("without token manager", func(t *testing.T) {
		provider.SetTokenManager(nil)

		request := map[string]interface{}{
			"contents": []map[string]interface{}{
				{
					"role": "user",
					"parts": []map[string]interface{}{
						{"text": "test"},
					},
				},
			},
		}

		_, err := provider.GenerateContent(context.Background(), "gemini-3-flash", request)
		if err != nil {
			t.Errorf("GenerateContent failed without token manager: %v", err)
		}
	})

	// Test with token manager
	t.Run("with token manager", func(t *testing.T) {
		// Create a mock token store
		store := tokpkg.NewMultiTokenStore("test", "", provider.GetLogger())

		// Add a test token
		token := tokpkg.ProviderToken{
			ID:           "test-token-1",
			AccessToken:  "test-access-token",
			RefreshToken: "test-refresh-token",
			ExpiryDate:   time.Now().Add(1 * time.Hour).UnixMilli(),
			Healthy:      true,
			HealthScore:  1.0,
			Proxy: &tokpkg.ProxyConfig{
				Type: tokpkg.ProxyTypeNone,
			},
		}
		if err := store.AddToken(token); err != nil {
			t.Fatalf("Failed to add token: %v", err)
		}

		// Create token manager
		factory := newMockClientFactory()
		proxyHealthTracker := tokpkg.NewProxyHealthTracker(provider.GetLogger(), 5, 5*time.Minute)
		tokenManager := tokpkg.NewTokenManager(store, tokpkg.NewRandomSelectionStrategy(), provider.GetLogger(), factory, proxyHealthTracker)

		provider.SetTokenManager(tokenManager)

		request := map[string]interface{}{
			"contents": []map[string]interface{}{
				{
					"role": "user",
					"parts": []map[string]interface{}{
						{"text": "test"},
					},
				},
			},
		}

		_, err := provider.GenerateContent(context.Background(), "gemini-3-flash", request)
		if err != nil {
			t.Errorf("GenerateContent failed with token manager: %v", err)
		}
	})
}

// TestGenerateContentStreamWithTokenManager tests GenerateContentStream with token manager
func TestGenerateContentStreamWithTokenManager(t *testing.T) {
	// Create a mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check for authorization header
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("data: {}\n\n"))
	}))
	defer server.Close()

	// Create provider
	provider := NewProvider(nil)
	provider.dailyBaseURL = server.URL
	provider.isInitialized = true
	provider.projectID = "test-project"

	// Test without token manager (backward compatibility)
	t.Run("without token manager", func(t *testing.T) {
		provider.SetTokenManager(nil)

		request := map[string]interface{}{
			"contents": []map[string]interface{}{
				{
					"role": "user",
					"parts": []map[string]interface{}{
						{"text": "test"},
					},
				},
			},
		}

		stream, err := provider.GenerateContentStream(context.Background(), "gemini-3-flash", request)
		if err != nil {
			t.Errorf("GenerateContentStream failed without token manager: %v", err)
		}
		if stream != nil {
			stream.Close()
		}
	})

	// Test with token manager
	t.Run("with token manager", func(t *testing.T) {
		// Create a mock token store
		store := tokpkg.NewMultiTokenStore("test", "", provider.GetLogger())

		// Add a test token
		token := tokpkg.ProviderToken{
			ID:           "test-token-1",
			AccessToken:  "test-access-token",
			RefreshToken: "test-refresh-token",
			ExpiryDate:   time.Now().Add(1 * time.Hour).UnixMilli(),
			Healthy:      true,
			HealthScore:  1.0,
			Proxy: &tokpkg.ProxyConfig{
				Type: tokpkg.ProxyTypeNone,
			},
		}
		if err := store.AddToken(token); err != nil {
			t.Fatalf("Failed to add token: %v", err)
		}

		// Create token manager
		factory := newMockClientFactory()
		proxyHealthTracker := tokpkg.NewProxyHealthTracker(provider.GetLogger(), 5, 5*time.Minute)
		tokenManager := tokpkg.NewTokenManager(store, tokpkg.NewRandomSelectionStrategy(), provider.GetLogger(), factory, proxyHealthTracker)

		provider.SetTokenManager(tokenManager)

		request := map[string]interface{}{
			"contents": []map[string]interface{}{
				{
					"role": "user",
					"parts": []map[string]interface{}{
						{"text": "test"},
					},
				},
			},
		}

		stream, err := provider.GenerateContentStream(context.Background(), "gemini-3-flash", request)
		if err != nil {
			t.Errorf("GenerateContentStream failed with token manager: %v", err)
		}
		if stream != nil {
			stream.Close()
		}
	})
}

// TestProxyHealthTracking tests proxy health tracking
func TestProxyHealthTracking(t *testing.T) {
	// Create a mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"response": {}}`))
	}))
	defer server.Close()

	// Create provider
	provider := NewProvider(nil)
	provider.dailyBaseURL = server.URL
	provider.isInitialized = true
	provider.projectID = "test-project"

	// Create a mock token store
	store := tokpkg.NewMultiTokenStore("test", "", provider.GetLogger())

	// Add a test token with proxy configuration
	token := tokpkg.ProviderToken{
		ID:           "test-token-1",
		AccessToken:  "test-access-token",
		RefreshToken: "test-refresh-token",
		ExpiryDate:   time.Now().Add(1 * time.Hour).UnixMilli(),
		Healthy:      true,
		HealthScore:  1.0,
		Proxy: &tokpkg.ProxyConfig{
			Type: tokpkg.ProxyTypeHTTP,
			Host: "proxy.example.com",
			Port: 8080,
		},
	}
	if err := store.AddToken(token); err != nil {
		t.Fatalf("Failed to add token: %v", err)
	}

	// Create token manager with proxy health tracker
	factory := newMockClientFactory()
	proxyHealthTracker := tokpkg.NewProxyHealthTracker(provider.GetLogger(), 5, 5*time.Minute)
	tokenManager := tokpkg.NewTokenManager(store, tokpkg.NewRandomSelectionStrategy(), provider.GetLogger(), factory, proxyHealthTracker)

	provider.SetTokenManager(tokenManager)

	// Make a request
	request := map[string]interface{}{
		"contents": []map[string]interface{}{
			{
				"role": "user",
				"parts": []map[string]interface{}{
					{"text": "test"},
				},
			},
		},
	}

	_, err := provider.GenerateContent(context.Background(), "gemini-3-flash", request)
	if err != nil {
		t.Errorf("GenerateContent failed: %v", err)
	}

	// Check that proxy health was updated
	healthScore := proxyHealthTracker.GetHealthScore("test-token-1")
	if healthScore != 1.0 {
		t.Errorf("Expected health score 1.0, got %.2f", healthScore)
	}
}

// TestProxyErrorClassification tests proxy error classification and health tracking
func TestProxyErrorClassification(t *testing.T) {
	// Create a mock server that returns errors
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return a 502 Bad Gateway error
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()

	// Create provider
	provider := NewProvider(nil)
	provider.dailyBaseURL = server.URL
	provider.isInitialized = true
	provider.projectID = "test-project"

	// Create a mock token store
	store := tokpkg.NewMultiTokenStore("test", "", provider.GetLogger())

	// Add a test token
	token := tokpkg.ProviderToken{
		ID:           "test-token-1",
		AccessToken:  "test-access-token",
		RefreshToken: "test-refresh-token",
		ExpiryDate:   time.Now().Add(1 * time.Hour).UnixMilli(),
		Healthy:      true,
		HealthScore:  1.0,
		Proxy: &tokpkg.ProxyConfig{
			Type: tokpkg.ProxyTypeNone,
		},
	}
	if err := store.AddToken(token); err != nil {
		t.Fatalf("Failed to add token: %v", err)
	}

	// Create token manager with proxy health tracker
	factory := newMockClientFactory()
	proxyHealthTracker := tokpkg.NewProxyHealthTracker(provider.GetLogger(), 5, 5*time.Minute)
	tokenManager := tokpkg.NewTokenManager(store, tokpkg.NewRandomSelectionStrategy(), provider.GetLogger(), factory, proxyHealthTracker)

	provider.SetTokenManager(tokenManager)

	// Make a request that will fail
	request := map[string]interface{}{
		"contents": []map[string]interface{}{
			{
				"role": "user",
				"parts": []map[string]interface{}{
					{"text": "test"},
				},
			},
		},
	}

	_, err := provider.GenerateContent(context.Background(), "gemini-3-flash", request)
	if err == nil {
		t.Error("Expected error but got none")
	}

	// Note: 502 Bad Gateway is not a proxy connection error, so health score should remain unchanged
	healthScore := proxyHealthTracker.GetHealthScore("test-token-1")
	if healthScore != 1.0 {
		t.Errorf("Expected health score to remain 1.0, got %.2f", healthScore)
	}
}

// TestSetTokenManager tests the SetTokenManager method
func TestSetTokenManager(t *testing.T) {
	provider := NewProvider(nil)

	// Initially tokenManager should be nil
	if provider.GetTokenManager() != nil {
		t.Error("Expected tokenManager to be nil initially")
	}

	// Create a token manager
	store := tokpkg.NewMultiTokenStore("test", "", provider.GetLogger())
	factory := newMockClientFactory()
	proxyHealthTracker := tokpkg.NewProxyHealthTracker(provider.GetLogger(), 5, 5*time.Minute)
	tokenManager := tokpkg.NewTokenManager(store, tokpkg.NewRandomSelectionStrategy(), provider.GetLogger(), factory, proxyHealthTracker)

	// Set the token manager
	provider.SetTokenManager(tokenManager)

	// Check that tokenManager is set
	if provider.GetTokenManager() != tokenManager {
		t.Error("tokenManager was not set correctly")
	}
}

// TestBackwardCompatibility tests backward compatibility when tokenManager is nil
func TestBackwardCompatibility(t *testing.T) {
	// Create a mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"response": {}}`))
	}))
	defer server.Close()

	// Create provider without token manager
	provider := NewProvider(nil)
	provider.dailyBaseURL = server.URL
	provider.isInitialized = true
	provider.projectID = "test-project"
	provider.SetTokenManager(nil)

	// Test ListModels
	_, err := provider.ListModels(context.Background())
	if err != nil {
		t.Errorf("ListModels failed: %v", err)
	}

	// Test GenerateContent
	request := map[string]interface{}{
		"contents": []map[string]interface{}{
			{
				"role": "user",
				"parts": []map[string]interface{}{
					{"text": "test"},
				},
			},
		},
	}
	_, err = provider.GenerateContent(context.Background(), "gemini-3-flash", request)
	if err != nil {
		t.Errorf("GenerateContent failed: %v", err)
	}

	// Test GenerateContentStream
	stream, err := provider.GenerateContentStream(context.Background(), "gemini-3-flash", request)
	if err != nil {
		t.Errorf("GenerateContentStream failed: %v", err)
	}
	if stream != nil {
		stream.Close()
	}
}
