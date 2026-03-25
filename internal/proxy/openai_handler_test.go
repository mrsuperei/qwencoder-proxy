// Package proxy provides HTTP handlers for the proxy server
package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sunbankio/qwencoder-proxy/internal/converter"
	"github.com/sunbankio/qwencoder-proxy/internal/logging"
	"github.com/sunbankio/qwencoder-proxy/internal/provider"
	"github.com/sunbankio/qwencoder-proxy/internal/token"
)

// mockProvider is a mock implementation of provider.Provider for testing
type mockProvider struct {
	name     provider.ProviderType
	protocol provider.ProtocolType
	models   interface{}
	failGen  bool
	failErr  error
}

func (m *mockProvider) Name() provider.ProviderType {
	return m.name
}

func (m *mockProvider) Protocol() provider.ProtocolType {
	return m.protocol
}

func (m *mockProvider) SupportedModels() []string {
	return []string{"test-model"}
}

func (m *mockProvider) SupportsModel(model string) bool {
	return model == "test-model"
}

func (m *mockProvider) ListModels(ctx context.Context) (interface{}, error) {
	if m.failGen {
		return nil, m.failErr
	}
	return m.models, nil
}

func (m *mockProvider) GenerateContent(ctx context.Context, model string, req interface{}) (interface{}, error) {
	if m.failGen {
		return nil, m.failErr
	}
	return map[string]interface{}{
		"id":      "test-id",
		"object":  "chat.completion",
		"created": 1234567890,
		"model":   model,
		"choices": []interface{}{
			map[string]interface{}{
				"index": 0,
				"message": map[string]interface{}{
					"role":    "assistant",
					"content": "Test response",
				},
				"finish_reason": "stop",
			},
		},
	}, nil
}

func (m *mockProvider) GenerateContentStream(ctx context.Context, model string, req interface{}) (io.ReadCloser, error) {
	if m.failGen {
		return nil, m.failErr
	}
	return &mockReadCloser{}, nil
}

func (m *mockProvider) GetAuthenticator() provider.Authenticator {
	return nil
}

func (m *mockProvider) IsHealthy(ctx context.Context) bool {
	return !m.failGen
}

// mockReadCloser is a mock io.ReadCloser for testing
type mockReadCloser struct{}

func (m *mockReadCloser) Read(p []byte) (n int, err error) {
	return 0, io.EOF
}

func (m *mockReadCloser) Close() error {
	return nil
}

// mockAuthenticator is a mock implementation of provider.Authenticator for testing
type mockAuthenticator struct{}

func (m *mockAuthenticator) Authenticate(ctx context.Context) error {
	return nil
}

func (m *mockAuthenticator) GetToken(ctx context.Context) (string, error) {
	return "test-token", nil
}

func (m *mockAuthenticator) GetTokenWithClient(ctx context.Context) (string, *http.Client, error) {
	return "test-token", nil, nil
}

func (m *mockAuthenticator) IsAuthenticated() bool {
	return true
}

func (m *mockAuthenticator) GetCredentialsPath() string {
	return "/tmp/test-credentials"
}

func (m *mockAuthenticator) ClearCredentials() error {
	return nil
}

func (m *mockAuthenticator) GetHTTPClient() (*http.Client, error) {
	return nil, errors.New("not implemented")
}

// TestNewOpenAIHandler tests constructor
func TestNewOpenAIHandler(t *testing.T) {
	factory := provider.NewFactory(logging.NewLogger())
	convFactory := converter.NewFactory()

	handler := NewOpenAIHandler(factory, convFactory)

	if handler == nil {
		t.Fatal("Expected non-nil handler")
	}
	if handler.factory == nil {
		t.Error("Expected factory to be set")
	}
	if handler.convFactory == nil {
		t.Error("Expected convFactory to be set")
	}
	if handler.logger == nil {
		t.Error("Expected logger to be set")
	}
	if handler.tokenManager != nil {
		t.Error("Expected tokenManager to be nil for backward compatibility")
	}
}

// TestNewOpenAIHandlerWithTokenManager tests the constructor with token manager
func TestNewOpenAIHandlerWithTokenManager(t *testing.T) {
	factory := provider.NewFactory(logging.NewLogger())
	convFactory := converter.NewFactory()
	tokenManager := token.NewTokenManager(nil, token.NewRandomSelectionStrategy(), logging.NewLogger(), nil, nil)

	handler := NewOpenAIHandlerWithTokenManager(factory, convFactory, tokenManager)

	if handler == nil {
		t.Fatal("Expected non-nil handler")
	}
	if handler.tokenManager == nil {
		t.Error("Expected tokenManager to be set")
	}
}

// TestNewProviderSpecificHandler tests the provider-specific constructor
func TestNewProviderSpecificHandler(t *testing.T) {
	factory := provider.NewFactory(logging.NewLogger())
	convFactory := converter.NewFactory()

	handler := NewProviderSpecificHandler(factory, convFactory, provider.ProviderGeminiCLI)

	if handler == nil {
		t.Fatal("Expected non-nil handler")
	}
	if handler.fixedProvider != provider.ProviderGeminiCLI {
		t.Errorf("Expected fixedProvider to be %s, got %s", provider.ProviderGeminiCLI, handler.fixedProvider)
	}
}

// TestNewProviderSpecificHandlerWithTokenManager tests the provider-specific constructor with token manager
func TestNewProviderSpecificHandlerWithTokenManager(t *testing.T) {
	factory := provider.NewFactory(logging.NewLogger())
	convFactory := converter.NewFactory()
	tokenManager := token.NewTokenManager(nil, token.NewRandomSelectionStrategy(), logging.NewLogger(), nil, nil)

	handler := NewProviderSpecificHandlerWithTokenManager(factory, convFactory, provider.ProviderGeminiCLI, tokenManager)

	if handler == nil {
		t.Fatal("Expected non-nil handler")
	}
	if handler.fixedProvider != provider.ProviderGeminiCLI {
		t.Errorf("Expected fixedProvider to be %s, got %s", provider.ProviderGeminiCLI, handler.fixedProvider)
	}
	if handler.tokenManager == nil {
		t.Error("Expected tokenManager to be set")
	}
}

// TestIsProxyError tests proxy error detection
func TestIsProxyError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
		{
			name: "connection refused",
			err:  errors.New("connection refused"),
			want: true,
		},
		{
			name: "proxy authentication failed",
			err:  errors.New("proxy authentication failed"),
			want: true,
		},
		{
			name: "407 proxy auth",
			err:  errors.New("407 Proxy Authentication Required"),
			want: true,
		},
		{
			name: "DNS lookup failure",
			err:  errors.New("dial tcp: lookup proxy.example.com: no such host"),
			want: true,
		},
		{
			name: "SOCKS proxy error",
			err:  errors.New("socks: connection refused"),
			want: true,
		},
		{
			name: "timeout error",
			err:  errors.New("i/o timeout"),
			want: true,
		},
		{
			name: "network unreachable",
			err:  errors.New("network unreachable"),
			want: true,
		},
		{
			name: "generic API error",
			err:  errors.New("API rate limit exceeded"),
			want: false,
		},
		{
			name: "context deadline exceeded",
			err:  context.DeadlineExceeded,
			want: true,
		},
	}

	handler := NewOpenAIHandler(provider.NewFactory(logging.NewLogger()), converter.NewFactory())

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := handler.IsProxyError(tt.err)
			if got != tt.want {
				t.Errorf("IsProxyError(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

// TestGetProxyErrorDetails tests proxy error details extraction
func TestGetProxyErrorDetails(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantHost string
		wantType string
	}{
		{
			name:     "DNS lookup failure",
			err:      errors.New("dial tcp: lookup proxy.example.com:1080: no such host"),
			wantHost: "proxy.example.com",
			wantType: "unknown",
		},
		{
			name:     "SOCKS5 error",
			err:      errors.New("socks5: connection refused"),
			wantHost: "unknown",
			wantType: "socks5",
		},
		{
			name:     "HTTP proxy error",
			err:      errors.New("http: proxy connection refused"),
			wantHost: "unknown",
			wantType: "http",
		},
		{
			name:     "generic error",
			err:      errors.New("some error"),
			wantHost: "unknown",
			wantType: "unknown",
		},
	}

	handler := NewOpenAIHandler(provider.NewFactory(logging.NewLogger()), converter.NewFactory())

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			details := handler.GetProxyErrorDetails(tt.err)
			if details["proxy_host"] != tt.wantHost {
				t.Errorf("GetProxyErrorDetails(%v)[proxy_host] = %v, want %v", tt.err, details["proxy_host"], tt.wantHost)
			}
			if details["proxy_type"] != tt.wantType {
				t.Errorf("GetProxyErrorDetails(%v)[proxy_type] = %v, want %v", tt.err, details["proxy_type"], tt.wantType)
			}
			if details["original_error"] == nil {
				t.Error("GetProxyErrorDetails should include original_error")
			}
		})
	}
}

// TestFormatProxyError tests proxy error formatting
func TestFormatProxyError(t *testing.T) {
	handler := NewOpenAIHandler(provider.NewFactory(logging.NewLogger()), converter.NewFactory())
	err := errors.New("dial tcp: lookup proxy.example.com:1080: no such host")

	jsonBytes := handler.FormatProxyError(err)

	var result map[string]interface{}
	if err := json.Unmarshal(jsonBytes, &result); err != nil {
		t.Fatalf("Failed to unmarshal JSON: %v", err)
	}

	if result["error"] != "proxy_connection_failed" {
		t.Errorf("Expected error to be 'proxy_connection_failed', got %v", result["error"])
	}

	if result["message"] == nil {
		t.Error("Expected message to be set")
	}

	if result["details"] == nil {
		t.Error("Expected details to be set")
	}

	if result["suggested_action"] == nil {
		t.Error("Expected suggested_action to be set")
	}
}

// TestHandleProxyError tests the handleProxyError method
func TestHandleProxyError(t *testing.T) {
	handler := NewOpenAIHandler(provider.NewFactory(logging.NewLogger()), converter.NewFactory())
	err := errors.New("dial tcp: lookup proxy.example.com:1080: no such host")

	w := httptest.NewRecorder()
	handler.HandleProxyError(w, err)

	if w.Code != http.StatusBadGateway {
		t.Errorf("Expected status %d, got %d", http.StatusBadGateway, w.Code)
	}

	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Expected Content-Type application/json, got %s", contentType)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("Failed to unmarshal JSON: %v", err)
	}

	if result["error"] != "proxy_connection_failed" {
		t.Errorf("Expected error to be 'proxy_connection_failed', got %v", result["error"])
	}

	if result["message"] == nil {
		t.Error("Expected message to be set")
	}

	if result["details"] == nil {
		t.Error("Expected details to be set")
	}

	if result["suggested_action"] == nil {
		t.Error("Expected suggested_action to be set")
	}

	// Check details structure
	details, ok := result["details"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected details to be a map")
	}

	if details["proxy_host"] == nil {
		t.Error("Expected proxy_host in details")
	}

	if details["proxy_port"] == nil {
		t.Error("Expected proxy_port in details")
	}

	if details["original_error"] == nil {
		t.Error("Expected original_error in details")
	}
}

// TestHandleStreamingError tests streaming error handling
func TestHandleStreamingError(t *testing.T) {
	handler := NewOpenAIHandler(provider.NewFactory(logging.NewLogger()), converter.NewFactory())

	tests := []struct {
		name        string
		err         error
		expectProxy bool
	}{
		{
			name:        "proxy error",
			err:         errors.New("dial tcp: lookup proxy.example.com:1080: no such host"),
			expectProxy: true,
		},
		{
			name:        "non-proxy error",
			err:         errors.New("API rate limit exceeded"),
			expectProxy: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			handler.handleStreamingError(w, tt.err)

			body := w.Body.String()
			isProxyResponse := strings.Contains(body, "proxy_connection_failed")

			if tt.expectProxy && !isProxyResponse {
				t.Error("Expected proxy error response for proxy error")
			}
			if !tt.expectProxy && isProxyResponse {
				t.Error("Did not expect proxy error response for non-proxy error")
			}
		})
	}
}

// TestBackwardCompatibility tests backward compatibility with nil tokenManager
func TestBackwardCompatibility(t *testing.T) {
	handler := NewOpenAIHandler(provider.NewFactory(logging.NewLogger()), converter.NewFactory())

	// Create a proxy error
	err := errors.New("dial tcp: lookup proxy.example.com:1080: no such host")

	w := httptest.NewRecorder()
	handler.HandleProxyError(w, err)

	// Should still detect proxy error and return appropriate response
	body := w.Body.String()
	if !strings.Contains(body, "proxy_connection_failed") {
		t.Error("Expected proxy error response even with nil tokenManager")
	}
}

// TestLogProxyError tests the logProxyError method
func TestLogProxyError(t *testing.T) {
	handler := NewOpenAIHandler(provider.NewFactory(logging.NewLogger()), converter.NewFactory())
	err := errors.New("dial tcp: lookup proxy.example.com:1080: no such host")
	details := map[string]interface{}{
		"proxy_type":     "socks5",
		"proxy_host":     "proxy.example.com",
		"proxy_port":     1080,
		"original_error": err.Error(),
	}

	// This should not panic
	handler.LogProxyError(err, details)
}

// TestServeHTTPWithCORS tests CORS headers
func TestServeHTTPWithCORS(t *testing.T) {
	handler := NewOpenAIHandler(provider.NewFactory(logging.NewLogger()), converter.NewFactory())

	req := httptest.NewRequest("OPTIONS", "/v1/chat/completions", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	if w.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Error("Expected Access-Control-Allow-Origin to be *")
	}

	if w.Header().Get("Access-Control-Allow-Methods") != "GET, POST, OPTIONS" {
		t.Error("Expected Access-Control-Allow-Methods to be GET, POST, OPTIONS")
	}

	if w.Header().Get("Access-Control-Allow-Headers") != "Content-Type, Authorization" {
		t.Error("Expected Access-Control-Allow-Headers to be Content-Type, Authorization")
	}
}

// TestServeHTTPWithInvalidJSON tests ServeHTTP method with invalid JSON
func TestServeHTTPWithInvalidJSON(t *testing.T) {
	handler := NewOpenAIHandler(provider.NewFactory(logging.NewLogger()), converter.NewFactory())

	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`invalid json`))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

// TestServeHTTPWithMissingModel tests ServeHTTP method with missing model
func TestServeHTTPWithMissingModel(t *testing.T) {
	handler := NewOpenAIHandler(provider.NewFactory(logging.NewLogger()), converter.NewFactory())

	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{
		"messages": [{"role": "user", "content": "Hello"}]
	}`))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

// TestServeHTTPWithUnknownModel tests ServeHTTP method with unknown model
func TestServeHTTPWithUnknownModel(t *testing.T) {
	handler := NewOpenAIHandler(provider.NewFactory(logging.NewLogger()), converter.NewFactory())

	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{
		"model": "unknown-model",
		"messages": [{"role": "user", "content": "Hello"}]
	}`))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

// TestHandleListModels tests the handleListModels method
func TestHandleListModels(t *testing.T) {
	factory := provider.NewFactory(logging.NewLogger())
	convFactory := converter.NewFactory()

	// Create a provider with models
	mockProv := &mockProvider{
		name:     provider.ProviderGeminiCLI,
		protocol: provider.ProtocolGemini,
		models: map[string]interface{}{
			"models": []interface{}{
				map[string]interface{}{"id": "gemini-pro"},
				map[string]interface{}{"id": "gemini-1.5"},
			},
		},
	}
	factory.Register(mockProv)

	handler := NewOpenAIHandler(factory, convFactory)

	req := httptest.NewRequest("GET", "/v1/models", nil)
	w := httptest.NewRecorder()

	handler.handleListModels(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("Failed to unmarshal JSON: %v", err)
	}

	if result["object"] != "list" {
		t.Errorf("Expected object to be 'list', got %v", result["object"])
	}

	data, ok := result["data"].([]interface{})
	if !ok {
		t.Fatal("Expected data to be an array")
	}

	if len(data) != 2 {
		t.Errorf("Expected 2 models, got %d", len(data))
	}
}

// TestHandleNonStreamCompletionsWithProxyError tests non-streaming completions with proxy errors
func TestHandleNonStreamCompletionsWithProxyError(t *testing.T) {
	factory := provider.NewFactory(logging.NewLogger())
	convFactory := converter.NewFactory()

	// Create a provider that will fail with a proxy error
	mockProv := &mockProvider{
		name:     provider.ProviderGeminiCLI,
		protocol: provider.ProtocolGemini,
		failGen:  true,
		failErr:  errors.New("dial tcp: lookup proxy.example.com:1080: no such host"),
	}
	factory.Register(mockProv)

	handler := NewOpenAIHandler(factory, convFactory)

	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{
		"model": "test-model",
		"messages": [{"role": "user", "content": "Hello"}]
	}`))
	w := httptest.NewRecorder()

	conv, _ := convFactory.Get(provider.ProtocolGemini)
	nativeReq, _ := conv.FromOpenAIRequest(map[string]interface{}{
		"model": "test-model",
		"messages": []interface{}{
			map[string]interface{}{"role": "user", "content": "Hello"},
		},
	})

	handler.handleNonStreamCompletions(w, req, mockProv, conv, nativeReq, "test-model")

	// Proxy errors return 502 Bad Gateway
	if w.Code != http.StatusBadGateway {
		t.Errorf("Expected status %d, got %d", http.StatusBadGateway, w.Code)
	}

	// Check that proxy error was detected and handled
	body := w.Body.String()
	if strings.Contains(body, "proxy_connection_failed") {
		t.Log("Proxy error was properly formatted")
	}
}

// TestHandleNonStreamCompletionsWithNonProxyError tests non-streaming completions with non-proxy errors
func TestHandleNonStreamCompletionsWithNonProxyError(t *testing.T) {
	factory := provider.NewFactory(logging.NewLogger())
	convFactory := converter.NewFactory()

	// Create a provider that will fail with a non-proxy error
	mockProv := &mockProvider{
		name:     provider.ProviderGeminiCLI,
		protocol: provider.ProtocolGemini,
		failGen:  true,
		failErr:  errors.New("API rate limit exceeded"),
	}
	factory.Register(mockProv)

	handler := NewOpenAIHandler(factory, convFactory)

	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{
		"model": "test-model",
		"messages": [{"role": "user", "content": "Hello"}]
	}`))
	w := httptest.NewRecorder()

	conv, _ := convFactory.Get(provider.ProtocolGemini)
	nativeReq, _ := conv.FromOpenAIRequest(map[string]interface{}{
		"model": "test-model",
		"messages": []interface{}{
			map[string]interface{}{"role": "user", "content": "Hello"},
		},
	})

	handler.handleNonStreamCompletions(w, req, mockProv, conv, nativeReq, "test-model")

	if w.Code != http.StatusInternalServerError {
		t.Errorf("Expected status %d, got %d", http.StatusInternalServerError, w.Code)
	}

	// Non-proxy errors should not return proxy error format
	body := w.Body.String()
	if strings.Contains(body, "proxy_connection_failed") {
		t.Error("Non-proxy error should not return proxy error format")
	}
}

// TestHandleNonStreamCompletionsWithSuccess tests non-streaming completions with success
func TestHandleNonStreamCompletionsWithSuccess(t *testing.T) {
	factory := provider.NewFactory(logging.NewLogger())
	convFactory := converter.NewFactory()

	// Create a provider that succeeds
	mockProv := &mockProvider{
		name:     provider.ProviderGeminiCLI,
		protocol: provider.ProtocolGemini,
		failGen:  false,
	}
	factory.Register(mockProv)

	handler := NewOpenAIHandler(factory, convFactory)

	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{
		"model": "test-model",
		"messages": [{"role": "user", "content": "Hello"}]
	}`))
	w := httptest.NewRecorder()

	conv, _ := convFactory.Get(provider.ProtocolGemini)
	nativeReq, _ := conv.FromOpenAIRequest(map[string]interface{}{
		"model": "test-model",
		"messages": []interface{}{
			map[string]interface{}{"role": "user", "content": "Hello"},
		},
	})

	handler.handleNonStreamCompletions(w, req, mockProv, conv, nativeReq, "test-model")

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Expected Content-Type application/json, got %s", contentType)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("Failed to unmarshal JSON: %v", err)
	}

	if result["id"] == nil {
		t.Error("Expected id in response")
	}
}
