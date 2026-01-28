// Package proxy provides HTTP handlers for the proxy server
package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"

	"github.com/sunbankio/qwencoder-proxy/auth"
	"github.com/sunbankio/qwencoder-proxy/logging"
	"github.com/sunbankio/qwencoder-proxy/provider"
	"github.com/sunbankio/qwencoder-proxy/provider/gemini"
)

// GeminiHandler handles requests to /gemini/* routes
type GeminiHandler struct {
	provider     *gemini.Provider
	logger       *logging.Logger
	tokenManager *auth.TokenManager // Optional, for proxy error handling
}

// NewGeminiHandler creates a new Gemini route handler
func NewGeminiHandler(p *gemini.Provider) *GeminiHandler {
	return &GeminiHandler{
		provider:     p,
		logger:       logging.NewLogger(),
		tokenManager: nil,
	}
}

// NewGeminiHandlerWithTokenManager creates a new Gemini route handler with token manager for proxy error handling
func NewGeminiHandlerWithTokenManager(p *gemini.Provider, tokenManager *auth.TokenManager) *GeminiHandler {
	return &GeminiHandler{
		provider:     p,
		logger:       logging.NewLogger(),
		tokenManager: tokenManager,
	}
}

// ServeHTTP handles HTTP requests for Gemini routes
func (h *GeminiHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Set CORS headers
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Parse the path: /gemini/models, /gemini/models/{model}:generateContent, etc.
	path := strings.TrimPrefix(r.URL.Path, "/gemini")
	path = strings.TrimPrefix(path, "/")

	h.logger.DebugLog("[Gemini Handler] Request: %s %s", r.Method, path)

	switch {
	case path == "models" && r.Method == http.MethodGet:
		h.handleListModels(w, r)
	case strings.HasPrefix(path, "models/") && strings.Contains(path, ":generateContent"):
		h.handleGenerateContent(w, r, path)
	case strings.HasPrefix(path, "models/") && strings.Contains(path, ":streamGenerateContent"):
		h.handleStreamGenerateContent(w, r, path)
	default:
		http.Error(w, "Not found", http.StatusNotFound)
	}
}

// handleListModels handles GET /gemini/models
func (h *GeminiHandler) handleListModels(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	models, err := h.provider.ListModels(ctx)
	if err != nil {
		h.logger.ErrorLog("[Gemini Handler] Failed to list models: %v", err)

		// Check if this is a proxy error
		if h.isProxyError(err) {
			h.handleProxyError(w, err)
			return
		}

		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(models); err != nil {
		h.logger.ErrorLog("[Gemini Handler] Failed to encode response: %v", err)
	}
}

// handleGenerateContent handles POST /gemini/models/{model}:generateContent
func (h *GeminiHandler) handleGenerateContent(w http.ResponseWriter, r *http.Request, path string) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract model name from path: models/{model}:generateContent
	model := extractModelFromPath(path, ":generateContent")
	if model == "" {
		http.Error(w, "Invalid model path", http.StatusBadRequest)
		return
	}

	// Parse request body
	var request gemini.GeminiRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		h.logger.ErrorLog("[Gemini Handler] Failed to decode request: %v", err)
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	response, err := h.provider.GenerateContent(ctx, model, &request)
	if err != nil {
		h.logger.ErrorLog("[Gemini Handler] GenerateContent failed: %v", err)

		// Check if this is a proxy error
		if h.isProxyError(err) {
			h.handleProxyError(w, err)
			return
		}

		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		h.logger.ErrorLog("[Gemini Handler] Failed to encode response: %v", err)
	}
}

// handleStreamGenerateContent handles POST /gemini/models/{model}:streamGenerateContent
func (h *GeminiHandler) handleStreamGenerateContent(w http.ResponseWriter, r *http.Request, path string) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract model name from path: models/{model}:streamGenerateContent
	model := extractModelFromPath(path, ":streamGenerateContent")
	if model == "" {
		http.Error(w, "Invalid model path", http.StatusBadRequest)
		return
	}

	// Parse request body
	var request gemini.GeminiRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		h.logger.ErrorLog("[Gemini Handler] Failed to decode request: %v", err)
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Create a minimal factory for the converter (this is a temporary solution)
	// In a real implementation, the factory should be passed to the handler
	factory := &provider.Factory{}
	if err := ConvertedStreamResponse(w, r, factory, h.provider, &request, model, h.logger); err != nil {
		h.logger.ErrorLog("[Gemini Handler] Stream error: %v", err)

		// Check if this is a proxy error
		if h.isProxyError(err) {
			h.handleProxyError(w, err)
			return
		}

		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// extractModelFromPath extracts the model name from a path like "models/{model}:action"
func extractModelFromPath(path, action string) string {
	// Remove "models/" prefix
	path = strings.TrimPrefix(path, "models/")
	// Remove the action suffix
	idx := strings.Index(path, action)
	if idx == -1 {
		return ""
	}
	return path[:idx]
}

// handleProxyError handles proxy-related errors and returns structured error responses
func (h *GeminiHandler) handleProxyError(w http.ResponseWriter, err error) {
	// Get proxy details from error
	details := h.getProxyErrorDetails(err)

	// Log the proxy error with masked credentials
	h.logProxyError(err, details)

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

// isProxyError detects if an error is related to proxy connection issues
func (h *GeminiHandler) isProxyError(err error) bool {
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

// getProxyErrorDetails extracts proxy-related details from an error
func (h *GeminiHandler) getProxyErrorDetails(err error) map[string]interface{} {
	details := make(map[string]interface{})

	// Default values
	details["proxy_type"] = "unknown"
	details["proxy_host"] = "unknown"
	details["proxy_port"] = 0
	details["original_error"] = err.Error()

	// Try to extract proxy details from token manager if available
	if h.tokenManager != nil {
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

// logProxyError logs proxy errors with masked credentials
func (h *GeminiHandler) logProxyError(err error, details map[string]interface{}) {
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
	h.logger.ErrorLog("[Gemini Handler] Proxy connection failed - Type: %s, Host: %s, Port: %d, Error: %v",
		maskedDetails["proxy_type"],
		maskedDetails["proxy_host"],
		maskedDetails["proxy_port"],
		err)
}

// formatProxyError formats a proxy error as a JSON response
func (h *GeminiHandler) formatProxyError(err error) []byte {
	details := h.getProxyErrorDetails(err)
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

// RegisterGeminiRoutes registers Gemini routes with the given provider factory
func RegisterGeminiRoutes(mux *http.ServeMux, factory *provider.Factory) error {
	p, err := factory.Get(provider.ProviderGeminiCLI)
	if err != nil {
		return err
	}

	geminiProvider, ok := p.(*gemini.Provider)
	if !ok {
		return nil // Provider not available or wrong type
	}

	handler := NewGeminiHandler(geminiProvider)
	mux.Handle("/gemini/", handler)
	return nil
}

// RegisterGeminiRoutesWithTokenManager registers Gemini routes with token manager for proxy error handling
func RegisterGeminiRoutesWithTokenManager(mux *http.ServeMux, factory *provider.Factory, tokenManager *auth.TokenManager) error {
	p, err := factory.Get(provider.ProviderGeminiCLI)
	if err != nil {
		return err
	}

	geminiProvider, ok := p.(*gemini.Provider)
	if !ok {
		return nil // Provider not available or wrong type
	}

	handler := NewGeminiHandlerWithTokenManager(geminiProvider, tokenManager)
	mux.Handle("/gemini/", handler)
	return nil
}
