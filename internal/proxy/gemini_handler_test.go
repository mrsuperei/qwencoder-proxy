// Package proxy provides HTTP handlers for the proxy server
package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sunbankio/qwencoder-proxy/internal/token"
	"github.com/sunbankio/qwencoder-proxy/logging"
	"github.com/sunbankio/qwencoder-proxy/provider/gemini"
)

// TestNewGeminiHandler tests constructor
func TestNewGeminiHandler(t *testing.T) {
	provider := gemini.NewProvider(nil)
	handler := NewGeminiHandler(provider)

	if handler == nil {
		t.Fatal("Expected non-nil handler")
	}
	if handler.provider == nil {
		t.Error("Expected provider to be set")
	}
	if handler.GetLogger() == nil {
		t.Error("Expected logger to be set")
	}
	if handler.GetTokenManager() != nil {
		t.Error("Expected tokenManager to be nil for backward compatibility")
	}
}

// TestNewGeminiHandlerWithTokenManager tests the constructor with token manager
func TestNewGeminiHandlerWithTokenManager(t *testing.T) {
	provider := gemini.NewProvider(nil)
	tokenManager := token.NewTokenManager(nil, token.NewRandomSelectionStrategy(), logging.NewLogger(), nil, nil)

	handler := NewGeminiHandlerWithTokenManager(provider, tokenManager)

	if handler == nil {
		t.Fatal("Expected non-nil handler")
	}
	if handler.GetTokenManager() == nil {
		t.Error("Expected tokenManager to be set")
	}
}

// TestGeminiIsProxyError tests proxy error detection
func TestGeminiIsProxyError(t *testing.T) {
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

	handler := NewGeminiHandler(gemini.NewProvider(nil))

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := handler.IsProxyError(tt.err)
			if got != tt.want {
				t.Errorf("IsProxyError(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

// TestGeminiGetProxyErrorDetails tests proxy error details extraction
func TestGeminiGetProxyErrorDetails(t *testing.T) {
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

	handler := NewGeminiHandler(gemini.NewProvider(nil))

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

// TestGeminiFormatProxyError tests proxy error formatting
func TestGeminiFormatProxyError(t *testing.T) {
	handler := NewGeminiHandler(gemini.NewProvider(nil))
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

// TestGeminiHandleProxyError tests the handleProxyError method
func TestGeminiHandleProxyError(t *testing.T) {
	handler := NewGeminiHandler(gemini.NewProvider(nil))
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

// TestGeminiBackwardCompatibility tests backward compatibility with nil tokenManager
func TestGeminiBackwardCompatibility(t *testing.T) {
	handler := NewGeminiHandler(gemini.NewProvider(nil))

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

// TestGeminiLogProxyError tests the logProxyError method
func TestGeminiLogProxyError(t *testing.T) {
	handler := NewGeminiHandler(gemini.NewProvider(nil))
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

// TestGeminiServeHTTPWithCORS tests CORS headers
func TestGeminiServeHTTPWithCORS(t *testing.T) {
	handler := NewGeminiHandler(gemini.NewProvider(nil))

	req := httptest.NewRequest("OPTIONS", "/gemini/models", nil)
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

// TestGeminiHandleGenerateContentWithInvalidJSON tests handleGenerateContent with invalid JSON
func TestGeminiHandleGenerateContentWithInvalidJSON(t *testing.T) {
	provider := gemini.NewProvider(nil)
	handler := NewGeminiHandler(provider)

	req := httptest.NewRequest("POST", "/gemini/models/gemini-pro:generateContent", strings.NewReader(`invalid json`))
	w := httptest.NewRecorder()

	handler.handleGenerateContent(w, req, "models/gemini-pro:generateContent")

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

// TestGeminiHandleGenerateContentWithInvalidPath tests handleGenerateContent with invalid path
func TestGeminiHandleGenerateContentWithInvalidPath(t *testing.T) {
	provider := gemini.NewProvider(nil)
	handler := NewGeminiHandler(provider)

	requestBody := `{
		"contents": [{"parts": [{"text": "Hello"}]}]
	}`
	req := httptest.NewRequest("POST", "/gemini/models/gemini-pro:generateContent", strings.NewReader(requestBody))
	w := httptest.NewRecorder()

	handler.handleGenerateContent(w, req, "invalid-path")

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

// TestGeminiHandleStreamGenerateContentWithInvalidJSON tests handleStreamGenerateContent with invalid JSON
func TestGeminiHandleStreamGenerateContentWithInvalidJSON(t *testing.T) {
	provider := gemini.NewProvider(nil)
	handler := NewGeminiHandler(provider)

	req := httptest.NewRequest("POST", "/gemini/models/gemini-pro:streamGenerateContent", strings.NewReader(`invalid json`))
	w := httptest.NewRecorder()

	handler.handleStreamGenerateContent(w, req, "models/gemini-pro:streamGenerateContent")

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

// TestGeminiExtractModelFromPath tests extractModelFromPath function
func TestGeminiExtractModelFromPath(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		action   string
		expected string
	}{
		{
			name:     "valid generateContent path",
			path:     "models/gemini-pro:generateContent",
			action:   ":generateContent",
			expected: "gemini-pro",
		},
		{
			name:     "valid streamGenerateContent path",
			path:     "models/gemini-1.5:streamGenerateContent",
			action:   ":streamGenerateContent",
			expected: "gemini-1.5",
		},
		{
			name:     "invalid path - no action",
			path:     "models/gemini-pro",
			action:   ":generateContent",
			expected: "",
		},
		{
			name:     "invalid path - no models prefix",
			path:     "gemini-pro:generateContent",
			action:   ":generateContent",
			expected: "gemini-pro",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractModelFromPath(tt.path, tt.action)
			if result != tt.expected {
				t.Errorf("extractModelFromPath(%s, %s) = %s, want %s", tt.path, tt.action, result, tt.expected)
			}
		})
	}
}

// TestGeminiServeHTTPWithNotFound tests ServeHTTP with 404
func TestGeminiServeHTTPWithNotFound(t *testing.T) {
	handler := NewGeminiHandler(gemini.NewProvider(nil))

	req := httptest.NewRequest("GET", "/gemini/unknown", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}

// TestGeminiServeHTTPWithMethodNotAllowed tests ServeHTTP with method not allowed
func TestGeminiServeHTTPWithMethodNotAllowed(t *testing.T) {
	handler := NewGeminiHandler(gemini.NewProvider(nil))

	req := httptest.NewRequest("DELETE", "/gemini/models", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}

// TestGeminiHandlerWithTokenManagerIntegration tests handler with token manager integration
func TestGeminiHandlerWithTokenManagerIntegration(t *testing.T) {
	// Create a mock token store with a token
	store := token.NewMockStore("test", logging.NewLogger())
	providerToken := token.ProviderToken{
		ID:          "test-token",
		AccessToken: "test-access-token",
		Email:       "test@example.com",
		ExpiryDate:  time.Now().Add(time.Hour).UnixMilli(),
		Healthy:     true,
		HealthScore: 1.0,
	}
	if err := store.AddToken(providerToken); err != nil {
		t.Fatal(err)
	}

	// Create a mock token manager
	strategy := token.NewRandomSelectionStrategy()
	tokenManager := token.NewTokenManager(store, strategy, logging.NewLogger(), nil, nil)

	// Create handler with token manager
	provider := gemini.NewProvider(nil)
	handler := NewGeminiHandlerWithTokenManager(provider, tokenManager)

	if handler.tokenManager == nil {
		t.Error("Expected tokenManager to be set")
	}

	if handler.tokenManager != tokenManager {
		t.Error("tokenManager not set correctly")
	}
}
