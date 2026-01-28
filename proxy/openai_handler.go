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
	"github.com/sunbankio/qwencoder-proxy/converter"
	"github.com/sunbankio/qwencoder-proxy/logging"
	"github.com/sunbankio/qwencoder-proxy/provider"
)

// OpenAIHandler handles OpenAI-compatible requests and routes them to appropriate providers
type OpenAIHandler struct {
	factory       *provider.Factory
	convFactory   *converter.Factory
	logger        *logging.Logger
	fixedProvider provider.ProviderType // If set, always use this provider
	tokenManager  *auth.TokenManager    // Optional, for proxy error handling
}

// NewOpenAIHandler creates a new OpenAI-compatible handler
func NewOpenAIHandler(factory *provider.Factory, convFactory *converter.Factory) *OpenAIHandler {
	return &OpenAIHandler{
		factory:     factory,
		convFactory: convFactory,
		logger:      logging.NewLogger(),
	}
}

// NewOpenAIHandlerWithTokenManager creates a new OpenAI-compatible handler with token manager for proxy error handling
func NewOpenAIHandlerWithTokenManager(factory *provider.Factory, convFactory *converter.Factory, tokenManager *auth.TokenManager) *OpenAIHandler {
	return &OpenAIHandler{
		factory:      factory,
		convFactory:  convFactory,
		logger:       logging.NewLogger(),
		tokenManager: tokenManager,
	}
}

// NewProviderSpecificHandler creates a new handler that forces requests to use a specific provider
func NewProviderSpecificHandler(factory *provider.Factory, convFactory *converter.Factory, providerType provider.ProviderType) *OpenAIHandler {
	return &OpenAIHandler{
		factory:       factory,
		convFactory:   convFactory,
		fixedProvider: providerType,
		logger:        logging.NewLogger(),
	}
}

// NewProviderSpecificHandlerWithTokenManager creates a new handler that forces requests to use a specific provider with token manager
func NewProviderSpecificHandlerWithTokenManager(factory *provider.Factory, convFactory *converter.Factory, providerType provider.ProviderType, tokenManager *auth.TokenManager) *OpenAIHandler {
	return &OpenAIHandler{
		factory:       factory,
		convFactory:   convFactory,
		fixedProvider: providerType,
		logger:        logging.NewLogger(),
		tokenManager:  tokenManager,
	}
}

// ServeHTTP handles HTTP requests for OpenAI-compatible routes
func (h *OpenAIHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Set CORS headers
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Determine the path by stripping known prefixes
	originalPath := r.URL.Path
	path := originalPath
	prefixes := []string{"/v1", "/qwen/v1", "/gemini/v1", "/kiro/v1", "/antigravity/v1", "/iflow/v1"}
	for _, prefix := range prefixes {
		if strings.HasPrefix(path, prefix) {
			path = strings.TrimPrefix(path, prefix)
			break
		}
	}
	path = strings.TrimPrefix(path, "/")

	h.logger.DebugLog("[Handler] Original URL.Path: %s, Stripped path: %s, Method: %s (Fixed Provider: %s)", originalPath, path, r.Method, h.fixedProvider)

	switch {
	case path == "models" && r.Method == http.MethodGet:
		h.handleListModels(w, r)
	case path == "chat/completions" && r.Method == http.MethodPost:
		h.handleChatCompletions(w, r)
	default:
		h.logger.ErrorLog("[Handler] No match found - path: '%s', method: '%s', expecting 'models' (GET) or 'chat/completions' (POST)", path, r.Method)
		http.Error(w, fmt.Sprintf("Unsupported OpenAI-compatible endpoint: %s", path), http.StatusNotFound)
	}
}

// handleListModels handles GET /v1/models
func (h *OpenAIHandler) handleListModels(w http.ResponseWriter, r *http.Request) {
	var allModels []provider.OpenAIModel

	if h.fixedProvider != "" {
		// Specific provider request
		p, err := h.factory.Get(h.fixedProvider)
		if err != nil {
			http.Error(w, fmt.Sprintf("Provider not available: %s", h.fixedProvider), http.StatusInternalServerError)
			return
		}
		modelsData, err := p.ListModels(r.Context())
		if err != nil {
			h.logger.ErrorLog("[Handler] Failed to list models for provider %s: %v", h.fixedProvider, err)
			http.Error(w, "Failed to list models", http.StatusInternalServerError)
			return
		}
		allModels = h.factory.FormatOpenAIModels(modelsData, h.fixedProvider)
	} else {
		// General /v1/models request - show all from all providers
		for _, providerType := range h.factory.ListTypes() {
			p, err := h.factory.Get(providerType)
			if err != nil {
				continue
			}

			modelsData, err := p.ListModels(r.Context())
			if err != nil {
				h.logger.ErrorLog("[Handler] Failed to list models for provider %s: %v", providerType, err)
				continue
			}

			// Use shared formatting logic from factory
			providerModels := h.factory.FormatOpenAIModels(modelsData, providerType)
			allModels = append(allModels, providerModels...)
		}
	}

	response := map[string]interface{}{
		"object": "list",
		"data":   allModels,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleChatCompletions handles POST /v1/chat/completions
func (h *OpenAIHandler) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	var openaiReq map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&openaiReq); err != nil {
		h.logger.ErrorLog("[Handler] Failed to decode request: %v", err)
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	model, _ := openaiReq["model"].(string)
	if model == "" {
		http.Error(w, "Model is required", http.StatusBadRequest)
		return
	}

	var p provider.Provider
	var err error

	if h.fixedProvider != "" {
		p, err = h.factory.Get(h.fixedProvider)
	} else {
		p, err = h.factory.GetByModel(model)
	}

	if err != nil {
		h.logger.ErrorLog("[Handler] Failed to get provider: %v", err)
		http.Error(w, fmt.Sprintf("Provider not found: %v", err), http.StatusBadRequest)
		return
	}

	h.logger.DebugLog("[Handler] Using provider %s for model %s", p.Name(), model)

	conv, err := h.convFactory.Get(p.Protocol())
	if err != nil {
		h.logger.ErrorLog("[Handler] No converter for protocol %s: %v", p.Protocol(), err)
		http.Error(w, "Protocol conversion not supported", http.StatusInternalServerError)
		return
	}

	nativeReq, err := conv.FromOpenAIRequest(openaiReq)
	if err != nil {
		h.logger.ErrorLog("[Handler] Conversion failed: %v", err)
		http.Error(w, "Failed to convert request", http.StatusInternalServerError)
		return
	}

	isStreaming, _ := openaiReq["stream"].(bool)
	if isStreaming {
		// Use converted streaming for providers that need format conversion
		if needsStreamConversion(p.Protocol()) {
			if err := ConvertedStreamResponse(w, r, h.factory, p, nativeReq, model, h.logger); err != nil {
				h.handleStreamingError(w, err)
			}
		} else {
			// Use raw streaming for providers that already format correctly (like Kiro)
			if err := StreamResponse(w, r, h.factory, p, nativeReq, model, h.logger); err != nil {
				h.handleStreamingError(w, err)
			}
		}
	} else {
		h.handleNonStreamCompletions(w, r, p, conv, nativeReq, model)
	}
}

func (h *OpenAIHandler) handleNonStreamCompletions(w http.ResponseWriter, r *http.Request, p provider.Provider, conv converter.Converter, nativeReq interface{}, model string) {
	resp, err := GenerateAndConvert(r.Context(), p, conv, nativeReq, model)
	if err != nil {
		h.logger.ErrorLog("[Handler] GenerateAndConvert failed with %s: %v", p.Name(), err)

		// Check if this is a proxy error
		if h.isProxyError(err) {
			h.handleProxyError(w, err)
			return
		}

		// Try alternative if not a fixed provider request
		if h.fixedProvider == "" {
			if altProvider, altErr := h.factory.GetAlternativeProvider(model, p.Name()); altErr == nil {
				h.logger.DebugLog("[Handler] Retrying with alternative %s", altProvider.Name())
				altConv, _ := h.convFactory.Get(altProvider.Protocol())
				if altResp, altErr := GenerateAndConvert(r.Context(), altProvider, altConv, nativeReq, model); altErr == nil {
					h.factory.RecordSuccess(model, altProvider.Name())
					w.Header().Set("Content-Type", "application/json")
					json.NewEncoder(w).Encode(altResp)
					return
				}
			}
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	h.factory.RecordSuccess(model, p.Name())
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// needsStreamConversion determines if a provider protocol needs stream format conversion
func needsStreamConversion(protocol provider.ProtocolType) bool {
	switch protocol {
	case provider.ProtocolGemini, provider.ProtocolQwen:
		// These providers return raw API streams that need conversion to OpenAI SSE format
		return true
	case provider.ProtocolClaude, provider.ProtocolOpenAI:
		// These providers already format their streams correctly
		return false
	default:
		// Default to conversion for unknown protocols
		return true
	}
}

// handleStreamingError handles errors during streaming responses
func (h *OpenAIHandler) handleStreamingError(w http.ResponseWriter, err error) {
	// Check if this is a proxy error
	if h.isProxyError(err) {
		h.handleProxyError(w, err)
		return
	}

	// For other errors, log and return generic error
	h.logger.ErrorLog("[Handler] Streaming error: %v", err)
	http.Error(w, "Streaming failed", http.StatusInternalServerError)
}

// handleProxyError handles proxy-related errors and returns structured error responses
func (h *OpenAIHandler) handleProxyError(w http.ResponseWriter, err error) {
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
func (h *OpenAIHandler) isProxyError(err error) bool {
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
func (h *OpenAIHandler) getProxyErrorDetails(err error) map[string]interface{} {
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
func (h *OpenAIHandler) logProxyError(err error, details map[string]interface{}) {
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
	h.logger.ErrorLog("[Handler] Proxy connection failed - Type: %s, Host: %s, Port: %d, Error: %v",
		maskedDetails["proxy_type"],
		maskedDetails["proxy_host"],
		maskedDetails["proxy_port"],
		err)
}

// formatProxyError formats a proxy error as a JSON response
func (h *OpenAIHandler) formatProxyError(err error) []byte {
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

// RegisterOpenAIRoutes registers all OpenAI-compatible routes
func RegisterOpenAIRoutes(mux *http.ServeMux, factory *provider.Factory, convFactory *converter.Factory) {
	// General route
	mux.Handle("/v1/", NewOpenAIHandler(factory, convFactory))
}

// RegisterProviderSpecificRoutes registers provider-specific OpenAI-compatible routes
func RegisterProviderSpecificRoutes(mux *http.ServeMux, factory *provider.Factory, convFactory *converter.Factory) {
	// Provider-specific routes
	mux.Handle("/qwen/v1/", NewProviderSpecificHandler(factory, convFactory, provider.ProviderQwen))
	mux.Handle("/gemini/v1/", NewProviderSpecificHandler(factory, convFactory, provider.ProviderGeminiCLI))
	mux.Handle("/kiro/v1/", NewProviderSpecificHandler(factory, convFactory, provider.ProviderKiro))
	mux.Handle("/antigravity/v1/", NewProviderSpecificHandler(factory, convFactory, provider.ProviderAntigravity))
	mux.Handle("/iflow/v1/", NewProviderSpecificHandler(factory, convFactory, provider.ProviderIFlow))
}
