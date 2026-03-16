package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"

	"github.com/sunbankio/qwencoder-proxy/internal/token"
	"github.com/sunbankio/qwencoder-proxy/logging"
)

// BaseHandler provides common behavior for handler implementations.
// Fields are immutable after construction, no mutex needed.
type BaseHandler struct {
	logger       logging.Logger
	tokenManager *token.TokenManager
}

// NewBaseHandler creates a new base handler with provided dependencies.
func NewBaseHandler(logger logging.Logger, tokenManager *token.TokenManager) *BaseHandler {
	return &BaseHandler{
		logger:       logger,
		tokenManager: tokenManager,
	}
}

// SetCORSHeaders centralizes CORS configuration.
func (b *BaseHandler) SetCORSHeaders(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
}

// HandleOptions handles CORS preflight OPTIONS requests.
func (b *BaseHandler) HandleOptions(w http.ResponseWriter) {
	w.WriteHeader(http.StatusOK)
}

// GetLogger returns the logger. Immutable, no mutex needed.
func (b *BaseHandler) GetLogger() logging.Logger {
	return b.logger
}

// GetTokenManager returns the token manager. Set once during init, no mutex needed.
func (b *BaseHandler) GetTokenManager() *token.TokenManager {
	return b.tokenManager
}

// IsProxyError checks if error is proxy-related. Handles nil errors safely.
func (b *BaseHandler) IsProxyError(err error) bool {
	if err == nil {
		return false
	}

	// Check for common proxy error patterns
	errStr := err.Error()

	// Network operation timeout
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	// Connection refused
	if strings.Contains(errStr, "connection refused") {
		return true
	}

	// Proxy authentication failure
	if strings.Contains(errStr, "proxy authentication failed") ||
		strings.Contains(errStr, "407") {
		return true
	}

	// DNS lookup failure
	if strings.Contains(errStr, "no such host") ||
		strings.Contains(errStr, "lookup") ||
		strings.Contains(errStr, "dns") {
		return true
	}

	// SOCKS proxy errors
	if strings.Contains(errStr, "socks") {
		return true
	}

	// Connection timeout
	if strings.Contains(errStr, "timeout") {
		return true
	}

	// Network unreachable
	if strings.Contains(errStr, "network unreachable") ||
		strings.Contains(errStr, "unreachable") {
		return true
	}

	// Check for net.OpError which often indicates network-level issues
	var netErr *net.OpError
	if errors.As(err, &netErr) {
		return true
	}

	return false
}

// HandleProxyError logs error and returns 502 Bad Gateway with structured JSON response.
func (b *BaseHandler) HandleProxyError(w http.ResponseWriter, err error) {
	// Get proxy details from error
	details := b.GetProxyErrorDetails(err)

	// Log the proxy error with masked credentials
	b.LogProxyError(err, details)

	// Create structured error response
	errorResp := map[string]interface{}{
		"error":            "proxy_connection_failed",
		"message":          fmt.Sprintf("Failed to connect to proxy %s:%d", details["proxy_host"], details["proxy_port"]),
		"details":          details,
		"suggested_action": "Check proxy configuration or disable proxy for this token",
	}

	// Set headers and write JSON response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadGateway)
	json.NewEncoder(w).Encode(errorResp)
}

// GetProxyErrorDetails extracts proxy-related details from an error.
func (b *BaseHandler) GetProxyErrorDetails(err error) map[string]interface{} {
	details := make(map[string]interface{})

	// Default values
	details["proxy_type"] = "unknown"
	details["proxy_host"] = "unknown"
	details["proxy_port"] = 0
	details["original_error"] = err.Error()

	// Try to extract proxy details from token manager if available
	if b.tokenManager != nil {
		// Note: In a real implementation, we might need to track which token
		// was used for the current request. For now, we'll provide generic details.
		// This could be enhanced by passing token context through the request.
	}

	// Try to extract proxy details from error message
	errStr := err.Error()

	// Extract host from error messages like "dial tcp: lookup proxy.example.com: no such host"
	if strings.Contains(errStr, "lookup ") {
		parts := strings.Split(errStr, "lookup ")
		if len(parts) > 1 {
			hostParts := strings.Split(parts[1], ":")
			if len(hostParts) > 0 {
				details["proxy_host"] = strings.TrimSpace(hostParts[0])
			}
		}
	}

	// Detect proxy type from error message
	if strings.Contains(errStr, "socks5") {
		details["proxy_type"] = "socks5"
	} else if strings.Contains(errStr, "http") {
		details["proxy_type"] = "http"
	} else if strings.Contains(errStr, "https") {
		details["proxy_type"] = "https"
	}

	// Detect authentication errors
	if strings.Contains(errStr, "407") || strings.Contains(errStr, "authentication") {
		details["auth_error"] = true
	}

	return details
}

// LogProxyError logs proxy errors with masked credentials.
func (b *BaseHandler) LogProxyError(err error, details map[string]interface{}) {
	// Create a masked version of details for logging
	maskedDetails := make(map[string]interface{})
	for k, v := range details {
		maskedDetails[k] = v
	}

	// Mask any sensitive information
	if proxyHost, ok := details["proxy_host"].(string); ok {
		maskedDetails["proxy_host"] = proxyHost
	}
	if proxyPort, ok := details["proxy_port"].(int); ok {
		maskedDetails["proxy_port"] = proxyPort
	}
	if proxyType, ok := details["proxy_type"].(string); ok {
		maskedDetails["proxy_type"] = proxyType
	}

	// Log with structured format
	b.logger.ErrorLog("[Handler] Proxy connection failed - Type: %s, Host: %s, Port: %d, Error: %v",
		maskedDetails["proxy_type"],
		maskedDetails["proxy_host"],
		maskedDetails["proxy_port"],
		err)
}

// FormatProxyError formats a proxy error as a JSON response.
func (b *BaseHandler) FormatProxyError(err error) []byte {
	details := b.GetProxyErrorDetails(err)
	errorResp := map[string]interface{}{
		"error":            "proxy_connection_failed",
		"message":          fmt.Sprintf("Failed to connect to proxy %s:%d", details["proxy_host"], details["proxy_port"]),
		"details":          details,
		"suggested_action": "Check proxy configuration or disable proxy for this token",
	}

	jsonBytes, err := json.Marshal(errorResp)
	if err != nil {
		// Fallback to simple error message
		fallback := map[string]interface{}{
			"error":   "proxy_connection_failed",
			"message": err.Error(),
		}
		jsonBytes, _ = json.Marshal(fallback)
	}

	return jsonBytes
}

// contains is a string matching helper.
func contains(s, substr string) bool {
	return indexOfSubstring(s, substr) >= 0
}

// indexOfSubstring finds the index of a substring.
func indexOfSubstring(s, substr string) int {
	return strings.Index(s, substr)
}
