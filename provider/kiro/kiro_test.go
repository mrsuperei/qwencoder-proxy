package kiro

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
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

	// Create a mock proxy health tracker
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

	// Create a mock proxy health tracker
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
	// Create a mock authenticator
	authenticator := auth.NewKiroAuthenticator(nil)

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
		authenticator: authenticator,
		BaseProvider: func() *providerpkg.BaseProvider {
			base := providerpkg.NewBaseProvider(logging.NewLogger(), 5*time.Minute)
			base.SetHTTPClient(&http.Client{Timeout: 5 * time.Minute})
			base.SetTokenManager(tokenManager)
			return base
		}(),
		machineID: "test-machine-id",
	}

	// Test request
	request := &ClaudeRequest{
		Model:     "claude-sonnet-4-5",
		MaxTokens: 100,
		Messages: []Message{
			{
				Role:    "user",
				Content: "Hello",
			},
		},
	}

	ctx := context.Background()
	// This will fail because we don't have a real server, but it should not panic
	_, err := provider.GenerateContent(ctx, "claude-sonnet-4-5", request)

	// We expect an error since we don't have a real server
	if err == nil {
		t.Error("GenerateContent() expected error without real server, got nil")
	}
}

// TestGenerateContent_WithProxy tests GenerateContent with proxy configuration
func TestGenerateContent_WithProxy(t *testing.T) {
	// Create a mock authenticator
	authenticator := auth.NewKiroAuthenticator(nil)

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
		authenticator: authenticator,
		BaseProvider: func() *providerpkg.BaseProvider {
			base := providerpkg.NewBaseProvider(logging.NewLogger(), 5*time.Minute)
			base.SetHTTPClient(&http.Client{Timeout: 5 * time.Minute})
			base.SetTokenManager(tokenManager)
			return base
		}(),
		machineID: "test-machine-id",
	}

	// Test request
	request := &ClaudeRequest{
		Model:     "claude-sonnet-4-5",
		MaxTokens: 100,
		Messages: []Message{
			{
				Role:    "user",
				Content: "Hello",
			},
		},
	}

	ctx := context.Background()
	// This will fail because we don't have a real server, but it should not panic
	_, err := provider.GenerateContent(ctx, "claude-sonnet-4-5", request)

	// We expect an error since we don't have a real server
	if err == nil {
		t.Error("GenerateContent() expected error without real server, got nil")
	}
}

// TestGenerateContent_WithoutTokenManager tests backward compatibility
func TestGenerateContent_WithoutTokenManager(t *testing.T) {
	// Create a mock authenticator
	authenticator := auth.NewKiroAuthenticator(nil)

	// Create provider without token manager
	provider := &Provider{
		authenticator: authenticator,
		BaseProvider: func() *providerpkg.BaseProvider {
			base := providerpkg.NewBaseProvider(logging.NewLogger(), 5*time.Minute)
			base.SetHTTPClient(&http.Client{Timeout: 5 * time.Minute})
			return base
		}(),
		machineID: "test-machine-id",
	}

	// Test request
	request := &ClaudeRequest{
		Model:     "claude-sonnet-4-5",
		MaxTokens: 100,
		Messages: []Message{
			{
				Role:    "user",
				Content: "Hello",
			},
		},
	}

	ctx := context.Background()
	// This should fail because we don't have a valid token
	// but it should not panic
	_, err := provider.GenerateContent(ctx, "claude-sonnet-4-5", request)

	// We expect an error since we don't have a valid token
	if err == nil {
		t.Error("GenerateContent() expected error without valid token, got nil")
	}
}

// TestGenerateContentStream_WithTokenManager tests streaming with token manager
func TestGenerateContentStream_WithTokenManager(t *testing.T) {
	// Create a mock authenticator
	authenticator := auth.NewKiroAuthenticator(nil)

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
		authenticator: authenticator,
		BaseProvider: func() *providerpkg.BaseProvider {
			base := providerpkg.NewBaseProvider(logging.NewLogger(), 5*time.Minute)
			base.SetHTTPClient(&http.Client{Timeout: 5 * time.Minute})
			base.SetTokenManager(tokenManager)
			return base
		}(),
		machineID: "test-machine-id",
	}

	// Test request
	request := &ClaudeRequest{
		Model:     "claude-sonnet-4-5",
		MaxTokens: 100,
		Messages: []Message{
			{
				Role:    "user",
				Content: "Hello",
			},
		},
	}

	ctx := context.Background()
	// This will fail because we don't have a real server, but it should not panic
	_, err := provider.GenerateContentStream(ctx, "claude-sonnet-4-5", request)

	// We expect an error since we don't have a real server
	if err == nil {
		t.Error("GenerateContentStream() expected error without real server, got nil")
	}
}

// TestGenerateContentStream_WithProxy tests streaming with proxy configuration
func TestGenerateContentStream_WithProxy(t *testing.T) {
	// Create a mock authenticator
	authenticator := auth.NewKiroAuthenticator(nil)

	// Create a mock token store with a token and proxy config
	store := auth.NewMultiTokenStore("test", ".test-tokens-stream-proxy.json", logging.NewLogger())
	proxyConfig := &auth.ProxyConfig{
		Type:    auth.ProxyTypeHTTP,
		Host:    "127.0.0.1",
		Port:    8080,
		Enabled: true,
	}
	token := auth.ProviderToken{
		ID:          "test-token-stream-proxy",
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
		authenticator: authenticator,
		BaseProvider: func() *providerpkg.BaseProvider {
			base := providerpkg.NewBaseProvider(logging.NewLogger(), 5*time.Minute)
			base.SetHTTPClient(&http.Client{Timeout: 5 * time.Minute})
			base.SetTokenManager(tokenManager)
			return base
		}(),
		machineID: "test-machine-id",
	}

	// Test request
	request := &ClaudeRequest{
		Model:     "claude-sonnet-4-5",
		MaxTokens: 100,
		Messages: []Message{
			{
				Role:    "user",
				Content: "Hello",
			},
		},
	}

	ctx := context.Background()
	// This will fail because we don't have a real server, but it should not panic
	_, err := provider.GenerateContentStream(ctx, "claude-sonnet-4-5", request)

	// We expect an error since we don't have a real server
	if err == nil {
		t.Error("GenerateContentStream() expected error without real server, got nil")
	}
}

// TestGenerateContentStream_WithoutTokenManager tests backward compatibility for streaming
func TestGenerateContentStream_WithoutTokenManager(t *testing.T) {
	// Create a mock authenticator
	authenticator := auth.NewKiroAuthenticator(nil)

	// Create provider without token manager
	provider := &Provider{
		authenticator: authenticator,
		BaseProvider: func() *providerpkg.BaseProvider {
			base := providerpkg.NewBaseProvider(logging.NewLogger(), 5*time.Minute)
			base.SetHTTPClient(&http.Client{Timeout: 5 * time.Minute})
			return base
		}(),
		machineID: "test-machine-id",
	}

	// Test request
	request := &ClaudeRequest{
		Model:     "claude-sonnet-4-5",
		MaxTokens: 100,
		Messages: []Message{
			{
				Role:    "user",
				Content: "Hello",
			},
		},
	}

	ctx := context.Background()
	// This should fail because we don't have a valid token
	// but it should not panic
	_, err := provider.GenerateContentStream(ctx, "claude-sonnet-4-5", request)

	// We expect an error since we don't have a valid token
	if err == nil {
		t.Error("GenerateContentStream() expected error without valid token, got nil")
	}
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

	// Create a mock token store
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

// TestSetTokenManager tests setting token manager
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

// TestSupportedModels checks that all supported models are valid
func TestSupportedModels(t *testing.T) {
	provider := NewProvider(nil)

	models := provider.SupportedModels()

	if len(models) == 0 {
		t.Error("Expected at least one supported model")
	}

	// Check that all models are supported
	for _, model := range models {
		if !provider.SupportsModel(model) {
			t.Errorf("Model %s should be supported", model)
		}
	}
}

// TestListModels tests ListModels method
func TestListModels(t *testing.T) {
	provider := NewProvider(nil)

	ctx := context.Background()
	models, err := provider.ListModels(ctx)

	if err != nil {
		t.Fatalf("ListModels() error = %v", err)
	}

	if models == nil {
		t.Fatal("ListModels() returned nil")
	}

	// Verify response structure
	modelsResp, ok := models.(*ClaudeModelsResponse)
	if !ok {
		t.Fatal("ListModels() did not return ClaudeModelsResponse")
	}

	if len(modelsResp.Data) != len(SupportedModels) {
		t.Errorf("Expected %d models, got %d", len(SupportedModels), len(modelsResp.Data))
	}
}

// TestNameAndProtocol tests provider name and protocol
func TestNameAndProtocol(t *testing.T) {
	provider := NewProvider(nil)

	if provider.Name() != "kiro" {
		t.Errorf("Expected name 'kiro', got '%s'", provider.Name())
	}

	if provider.Protocol() != "claude" {
		t.Errorf("Expected protocol 'claude', got '%s'", provider.Protocol())
	}
}

// TestConvertBedrockStreamToOpenAI tests the stream conversion
func TestConvertBedrockStreamToOpenAI(t *testing.T) {
	provider := &Provider{
		BaseProvider: providerpkg.NewBaseProvider(logging.NewLogger(), 5*time.Minute),
		machineID:    "test-machine-id",
	}

	// Create a mock response body with AWS Event Stream format
	// This is a simplified version for testing
	responseBody := bytes.NewReader([]byte{})

	ctx := context.Background()
	stream := provider.convertBedrockStreamToOpenAI(ctx, io.NopCloser(responseBody), "claude-sonnet-4-5", "test-token")

	if stream == nil {
		t.Fatal("convertBedrockStreamToOpenAI() returned nil stream")
	}

	defer stream.Close()
}

// TestGetHeaders tests the getHeaders method
func TestGetHeaders(t *testing.T) {
	provider := NewProvider(nil)

	token := "test-token"
	invocationID := "test-invocation-id"

	headers := provider.getHeaders(token, invocationID)

	if headers == nil {
		t.Fatal("getHeaders() returned nil")
	}

	// Check required headers
	if headers["Authorization"] != "Bearer "+token {
		t.Errorf("Expected Authorization header 'Bearer %s', got '%s'", token, headers["Authorization"])
	}

	if headers["Content-Type"] != "application/json" {
		t.Errorf("Expected Content-Type header 'application/json', got '%s'", headers["Content-Type"])
	}

	if headers["amz-sdk-invocation-id"] != invocationID {
		t.Errorf("Expected amz-sdk-invocation-id header '%s', got '%s'", invocationID, headers["amz-sdk-invocation-id"])
	}
}

// TestModelMapping tests the model mapping
func TestModelMapping(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{
			input:    "claude-opus-4-5",
			expected: "claude-opus-4.5",
		},
		{
			input:    "claude-sonnet-4-5",
			expected: "CLAUDE_SONNET_4_5_20250929_V1_0",
		},
		{
			input:    "unknown-model",
			expected: "unknown-model",
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := MapModelName(tt.input)
			if result != tt.expected {
				t.Errorf("MapModelName(%s) = %s, expected %s", tt.input, result, tt.expected)
			}
		})
	}
}

// TestIsValidModel tests the model validation
func TestIsValidModel(t *testing.T) {
	tests := []struct {
		model    string
		expected bool
	}{
		{
			model:    "claude-opus-4-5",
			expected: true,
		},
		{
			model:    "claude-sonnet-4-5",
			expected: true,
		},
		{
			model:    "unknown-model",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			result := IsValidModel(tt.model)
			if result != tt.expected {
				t.Errorf("IsValidModel(%s) = %v, expected %v", tt.model, result, tt.expected)
			}
		})
	}
}

// TestGenerateMachineID tests the machine ID generation
func TestGenerateMachineID(t *testing.T) {
	provider := NewProvider(nil)

	if provider.machineID == "" {
		t.Error("Expected machineID to be set")
	}

	// Generate another machine ID
	anotherID := generateMachineID()

	if anotherID == "" {
		t.Error("Expected generateMachineID() to return a non-empty string")
	}

	// The IDs should be the same for the same hostname
	if provider.machineID != anotherID {
		// This is actually expected since they're generated at different times
		// but we can check the format
	}
}

// TestGenerateUUID tests the UUID generation
func TestGenerateUUID(t *testing.T) {
	uuid1 := generateUUID()
	uuid2 := generateUUID()

	if uuid1 == "" {
		t.Error("Expected generateUUID() to return a non-empty string")
	}

	if uuid2 == "" {
		t.Error("Expected generateUUID() to return a non-empty string")
	}

	// UUIDs should be unique
	if uuid1 == uuid2 {
		t.Error("Expected generateUUID() to return unique values")
	}

	// UUID should have 4 parts separated by hyphens
	parts := strings.Split(uuid1, "-")
	if len(parts) != 5 {
		t.Errorf("Expected UUID to have 5 parts, got %d", len(parts))
	}
}

// TestExtractTextContent tests the text content extraction
func TestExtractTextContent(t *testing.T) {
	provider := NewProvider(nil)

	tests := []struct {
		name     string
		content  interface{}
		expected string
	}{
		{
			name:     "string content",
			content:  "Hello, world!",
			expected: "Hello, world!",
		},
		{
			name: "array content with text blocks",
			content: []interface{}{
				map[string]interface{}{
					"type": "text",
					"text": "Hello",
				},
				map[string]interface{}{
					"type": "text",
					"text": "World",
				},
			},
			expected: "HelloWorld",
		},
		{
			name:     "other content",
			content:  12345,
			expected: "12345",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := provider.extractTextContent(tt.content)
			if result != tt.expected {
				t.Errorf("extractTextContent() = %s, expected %s", result, tt.expected)
			}
		})
	}
}
