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

	"github.com/sunbankio/qwencoder-proxy/auth"
	"github.com/sunbankio/qwencoder-proxy/logging"
	"github.com/sunbankio/qwencoder-proxy/provider/kiro"
)

// TestNewAnthropicHandler tests constructor
func TestNewAnthropicHandler(t *testing.T) {
	provider := kiro.NewProvider(nil)
	handler := NewAnthropicHandler(provider)

	if handler == nil {
		t.Fatal("Expected non-nil handler")
	}
	if handler.provider == nil {
		t.Error("Expected provider to be set")
	}
	if handler.logger == nil {
		t.Error("Expected logger to be set")
	}
	if handler.tokenManager != nil {
		t.Error("Expected tokenManager to be nil for backward compatibility")
	}
}

// TestNewAnthropicHandlerWithTokenManager tests the constructor with token manager
func TestNewAnthropicHandlerWithTokenManager(t *testing.T) {
	provider := kiro.NewProvider(nil)
	tokenManager := auth.NewTokenManager(nil, auth.NewRandomSelectionStrategy(), logging.NewLogger(), nil, nil)

	handler := NewAnthropicHandlerWithTokenManager(provider, tokenManager)

	if handler == nil {
		t.Fatal("Expected non-nil handler")
	}
	if handler.tokenManager == nil {
		t.Error("Expected tokenManager to be set")
	}
}

// TestAnthropicIsProxyError tests proxy error detection
func TestAnthropicIsProxyError(t *testing.T) {
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

	handler := NewAnthropicHandler(kiro.NewProvider(nil))

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := handler.isProxyError(tt.err)
			if got != tt.want {
				t.Errorf("isProxyError(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

// TestAnthropicGetProxyErrorDetails tests proxy error details extraction
func TestAnthropicGetProxyErrorDetails(t *testing.T) {
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

	handler := NewAnthropicHandler(kiro.NewProvider(nil))

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			details := handler.getProxyErrorDetails(tt.err)
			if details["proxy_host"] != tt.wantHost {
				t.Errorf("getProxyErrorDetails(%v)[proxy_host] = %v, want %v", tt.err, details["proxy_host"], tt.wantHost)
			}
			if details["proxy_type"] != tt.wantType {
				t.Errorf("getProxyErrorDetails(%v)[proxy_type] = %v, want %v", tt.err, details["proxy_type"], tt.wantType)
			}
			if details["original_error"] == nil {
				t.Error("getProxyErrorDetails should include original_error")
			}
		})
	}
}

// TestAnthropicFormatProxyError tests proxy error formatting
func TestAnthropicFormatProxyError(t *testing.T) {
	handler := NewAnthropicHandler(kiro.NewProvider(nil))
	err := errors.New("dial tcp: lookup proxy.example.com:1080: no such host")

	jsonBytes := handler.formatProxyError(err)

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

// TestAnthropicHandleProxyError tests the handleProxyError method
func TestAnthropicHandleProxyError(t *testing.T) {
	handler := NewAnthropicHandler(kiro.NewProvider(nil))
	err := errors.New("dial tcp: lookup proxy.example.com:1080: no such host")

	w := httptest.NewRecorder()
	handler.handleProxyError(w, err)

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

// TestAnthropicBackwardCompatibility tests backward compatibility with nil tokenManager
func TestAnthropicBackwardCompatibility(t *testing.T) {
	handler := NewAnthropicHandler(kiro.NewProvider(nil))

	// Create a proxy error
	err := errors.New("dial tcp: lookup proxy.example.com:1080: no such host")

	w := httptest.NewRecorder()
	handler.handleProxyError(w, err)

	// Should still detect proxy error and return appropriate response
	body := w.Body.String()
	if !strings.Contains(body, "proxy_connection_failed") {
		t.Error("Expected proxy error response even with nil tokenManager")
	}
}

// TestAnthropicLogProxyError tests the logProxyError method
func TestAnthropicLogProxyError(t *testing.T) {
	handler := NewAnthropicHandler(kiro.NewProvider(nil))
	err := errors.New("dial tcp: lookup proxy.example.com:1080: no such host")
	details := map[string]interface{}{
		"proxy_type":     "socks5",
		"proxy_host":     "proxy.example.com",
		"proxy_port":     1080,
		"original_error": err.Error(),
	}

	// This should not panic
	handler.logProxyError(err, details)
}

// TestAnthropicServeHTTPWithCORS tests CORS headers
func TestAnthropicServeHTTPWithCORS(t *testing.T) {
	handler := NewAnthropicHandler(kiro.NewProvider(nil))

	req := httptest.NewRequest("OPTIONS", "/anthropic/models", nil)
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

// TestAnthropicHandleMessagesWithInvalidJSON tests handleMessages with invalid JSON
func TestAnthropicHandleMessagesWithInvalidJSON(t *testing.T) {
	provider := kiro.NewProvider(nil)
	handler := NewAnthropicHandler(provider)

	req := httptest.NewRequest("POST", "/anthropic/messages", strings.NewReader(`invalid json`))
	w := httptest.NewRecorder()

	handler.handleMessages(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

// TestAnthropicHandleMessagesWithWrongMethod tests handleMessages with wrong method
func TestAnthropicHandleMessagesWithWrongMethod(t *testing.T) {
	provider := kiro.NewProvider(nil)
	handler := NewAnthropicHandler(provider)

	req := httptest.NewRequest("GET", "/anthropic/messages", strings.NewReader(`{}`))
	w := httptest.NewRecorder()

	handler.handleMessages(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status %d, got %d", http.StatusMethodNotAllowed, w.Code)
	}
}

// TestAnthropicServeHTTPWithNotFound tests ServeHTTP with 404
func TestAnthropicServeHTTPWithNotFound(t *testing.T) {
	handler := NewAnthropicHandler(kiro.NewProvider(nil))

	req := httptest.NewRequest("GET", "/anthropic/unknown", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}

// TestAnthropicServeHTTPWithMethodNotAllowed tests ServeHTTP with method not allowed
func TestAnthropicServeHTTPWithMethodNotAllowed(t *testing.T) {
	handler := NewAnthropicHandler(kiro.NewProvider(nil))

	req := httptest.NewRequest("DELETE", "/anthropic/models", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}

// TestAnthropicHandlerWithTokenManagerIntegration tests handler with token manager integration
func TestAnthropicHandlerWithTokenManagerIntegration(t *testing.T) {
	// Create a mock token store with a token
	store := auth.NewMultiTokenStore("test", ".test-anthropic-handler.json", logging.NewLogger())
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

	// Create a mock token manager
	strategy := auth.NewRandomSelectionStrategy()
	tokenManager := auth.NewTokenManager(store, strategy, logging.NewLogger(), nil, nil)

	// Create handler with token manager
	provider := kiro.NewProvider(nil)
	handler := NewAnthropicHandlerWithTokenManager(provider, tokenManager)

	if handler.tokenManager == nil {
		t.Error("Expected tokenManager to be set")
	}

	if handler.tokenManager != tokenManager {
		t.Error("tokenManager not set correctly")
	}
}

// TestAnthropicProxyErrorDetailsExtraction tests various proxy error details extraction scenarios
func TestAnthropicProxyErrorDetailsExtraction(t *testing.T) {
	handler := NewAnthropicHandler(kiro.NewProvider(nil))

	tests := []struct {
		name              string
		err               error
		expectedHost      string
		expectedType      string
		expectedAuthError bool
	}{
		{
			name:              "DNS lookup with port",
			err:               errors.New("dial tcp: lookup proxy.example.com:1080: no such host"),
			expectedHost:      "proxy.example.com",
			expectedType:      "unknown",
			expectedAuthError: false,
		},
		{
			name:              "SOCKS5 connection refused",
			err:               errors.New("socks5: connection refused to 192.168.1.1:1080"),
			expectedHost:      "unknown",
			expectedType:      "socks5",
			expectedAuthError: false,
		},
		{
			name:              "HTTP proxy with auth error",
			err:               errors.New("http: 407 Proxy Authentication Required"),
			expectedHost:      "unknown",
			expectedType:      "http",
			expectedAuthError: true,
		},
		{
			name:              "HTTPS proxy",
			err:               errors.New("https: proxy connection timeout"),
			expectedHost:      "unknown",
			expectedType:      "http", // Matches "http" before "https" in the error message
			expectedAuthError: false,
		},
		{
			name:              "Generic network error",
			err:               errors.New("network unreachable"),
			expectedHost:      "unknown",
			expectedType:      "unknown",
			expectedAuthError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			details := handler.getProxyErrorDetails(tt.err)

			if details["proxy_host"] != tt.expectedHost {
				t.Errorf("Expected proxy_host %v, got %v", tt.expectedHost, details["proxy_host"])
			}

			if details["proxy_type"] != tt.expectedType {
				t.Errorf("Expected proxy_type %v, got %v", tt.expectedType, details["proxy_type"])
			}

			authError, ok := details["auth_error"].(bool)
			if ok && authError != tt.expectedAuthError {
				t.Errorf("Expected auth_error %v, got %v", tt.expectedAuthError, authError)
			}

			if details["original_error"] == nil {
				t.Error("Expected original_error to be set")
			}
		})
	}
}

// TestAnthropicProxyErrorDetectionComprehensive tests comprehensive proxy error detection
func TestAnthropicProxyErrorDetectionComprehensive(t *testing.T) {
	handler := NewAnthropicHandler(kiro.NewProvider(nil))

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
			name:     "connection refused",
			err:      errors.New("connection refused"),
			expected: true,
		},
		{
			name:     "proxy authentication failed",
			err:      errors.New("proxy authentication failed"),
			expected: true,
		},
		{
			name:     "407 Proxy Authentication Required",
			err:      errors.New("407 Proxy Authentication Required"),
			expected: true,
		},
		{
			name:     "DNS lookup failure",
			err:      errors.New("dial tcp: lookup proxy.example.com: no such host"),
			expected: true,
		},
		{
			name:     "SOCKS connection refused",
			err:      errors.New("socks: connection refused"),
			expected: true,
		},
		{
			name:     "SOCKS5 connection refused",
			err:      errors.New("socks5: connection refused"),
			expected: true,
		},
		{
			name:     "i/o timeout",
			err:      errors.New("i/o timeout"),
			expected: true,
		},
		{
			name:     "timeout",
			err:      errors.New("timeout"),
			expected: true,
		},
		{
			name:     "network unreachable",
			err:      errors.New("network unreachable"),
			expected: true,
		},
		{
			name:     "unreachable",
			err:      errors.New("unreachable"),
			expected: true,
		},
		{
			name:     "context deadline exceeded",
			err:      context.DeadlineExceeded,
			expected: true,
		},
		{
			name:     "API rate limit exceeded",
			err:      errors.New("API rate limit exceeded"),
			expected: false,
		},
		{
			name:     "invalid request",
			err:      errors.New("invalid request"),
			expected: false,
		},
		{
			name:     "unauthorized",
			err:      errors.New("unauthorized"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := handler.isProxyError(tt.err)
			if got != tt.expected {
				t.Errorf("isProxyError(%q) = %v, want %v", tt.err, got, tt.expected)
			}
		})
	}
}

// TestAnthropicFormatProxyErrorWithFallback tests formatProxyError with fallback
func TestAnthropicFormatProxyErrorWithFallback(t *testing.T) {
	handler := NewAnthropicHandler(kiro.NewProvider(nil))

	// Test with a normal error
	err := errors.New("dial tcp: lookup proxy.example.com:1080: no such host")
	jsonBytes := handler.formatProxyError(err)

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
}

// TestAnthropicHandleProxyErrorResponseStructure tests the structure of proxy error response
func TestAnthropicHandleProxyErrorResponseStructure(t *testing.T) {
	handler := NewAnthropicHandler(kiro.NewProvider(nil))
	err := errors.New("dial tcp: lookup proxy.example.com:1080: no such host")

	w := httptest.NewRecorder()
	handler.handleProxyError(w, err)

	var result map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("Failed to unmarshal JSON: %v", err)
	}

	// Check top-level fields
	requiredFields := []string{"error", "message", "details", "suggested_action"}
	for _, field := range requiredFields {
		if result[field] == nil {
			t.Errorf("Expected %s to be set in response", field)
		}
	}

	// Check details structure
	details, ok := result["details"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected details to be a map")
	}

	requiredDetailFields := []string{"proxy_type", "proxy_host", "proxy_port", "original_error"}
	for _, field := range requiredDetailFields {
		if details[field] == nil {
			t.Errorf("Expected %s to be set in details", field)
		}
	}
}
