// Package proxy provides HTTP handlers for the proxy server
package proxy

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/sunbankio/qwencoder-proxy/internal/logging"
	"github.com/sunbankio/qwencoder-proxy/internal/provider"
	"github.com/sunbankio/qwencoder-proxy/internal/provider/kiro"
	auth "github.com/sunbankio/qwencoder-proxy/internal/token"
)

// AnthropicHandler handles requests to /anthropic/* routes
// Embeds BaseHandler for common functionality
type AnthropicHandler struct {
	*BaseHandler // Embedded base handler provides common fields and methods
	provider     *kiro.Provider
}

// NewAnthropicHandler creates a new Anthropic route handler
// Uses BaseHandler for common functionality
func NewAnthropicHandler(p *kiro.Provider) *AnthropicHandler {
	return &AnthropicHandler{
		BaseHandler: NewBaseHandler(logging.NewLogger(), nil),
		provider:    p,
	}
}

// NewAnthropicHandlerWithTokenManager creates a new Anthropic route handler with token manager for proxy error handling
// Uses BaseHandler for common functionality
func NewAnthropicHandlerWithTokenManager(p *kiro.Provider, tokenManager *auth.TokenManager) *AnthropicHandler {
	return &AnthropicHandler{
		BaseHandler: NewBaseHandler(logging.NewLogger(), tokenManager),
		provider:    p,
	}
}

// ServeHTTP handles HTTP requests for Anthropic routes
func (h *AnthropicHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Set CORS headers using BaseHandler method
	h.SetCORSHeaders(w)

	if r.Method == http.MethodOptions {
		h.HandleOptions(w) // Use BaseHandler method
		return
	}

	// Parse the path: /anthropic/models, /anthropic/messages, etc.
	path := strings.TrimPrefix(r.URL.Path, "/anthropic")
	path = strings.TrimPrefix(path, "/")

	h.GetLogger().DebugLog("[Anthropic Handler] Request: %s %s", r.Method, path)

	switch {
	case path == "models" && r.Method == http.MethodGet:
		h.handleListModels(w, r)
	case path == "messages" && r.Method == http.MethodPost:
		h.handleMessages(w, r)
	default:
		http.Error(w, "Not found", http.StatusNotFound)
	}
}

// handleListModels handles GET /anthropic/models
func (h *AnthropicHandler) handleListModels(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	models, err := h.provider.ListModels(ctx)
	if err != nil {
		h.GetLogger().ErrorLog("[Anthropic Handler] Failed to list models: %v", err)

		// Check if this is a proxy error
		if h.IsProxyError(err) {
			h.HandleProxyError(w, err)
			return
		}

		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(models); err != nil {
		h.GetLogger().ErrorLog("[Anthropic Handler] Failed to encode response: %v", err)
	}
}

// handleMessages handles POST /anthropic/messages
func (h *AnthropicHandler) handleMessages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse request body
	var request kiro.ClaudeRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		h.GetLogger().ErrorLog("[Anthropic Handler] Failed to decode request: %v", err)
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Check if streaming
	isStreaming := request.Stream

	if isStreaming {
		h.handleStreamMessages(w, r, &request)
	} else {
		h.handleNonStreamMessages(w, r, &request)
	}
}

// handleNonStreamMessages handles non-streaming messages
func (h *AnthropicHandler) handleNonStreamMessages(w http.ResponseWriter, r *http.Request, request *kiro.ClaudeRequest) {
	ctx := r.Context()
	response, err := h.provider.GenerateContent(ctx, request.Model, request)
	if err != nil {
		h.GetLogger().ErrorLog("[Anthropic Handler] GenerateContent failed: %v", err)

		// Check if this is a proxy error
		if h.IsProxyError(err) {
			h.HandleProxyError(w, err)
			return
		}

		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		h.GetLogger().ErrorLog("[Anthropic Handler] Failed to encode response: %v", err)
	}
}

// handleStreamMessages handles streaming messages
func (h *AnthropicHandler) handleStreamMessages(w http.ResponseWriter, r *http.Request, request *kiro.ClaudeRequest) {
	ctx := r.Context()
	stream, err := h.provider.GenerateContentStream(ctx, request.Model, request)
	if err != nil {
		h.GetLogger().ErrorLog("[Anthropic Handler] GenerateContentStream failed: %v", err)

		// Check if this is a proxy error
		if h.IsProxyError(err) {
			h.HandleProxyError(w, err)
			return
		}

		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer stream.Close()

	// Set streaming headers
	SetStreamingHeaders(w)

	if err := CopyStreamToResponse(w, stream, h.GetLogger()); err != nil {
		h.GetLogger().ErrorLog("[Anthropic Handler] Stream error: %v", err)

		// Check if this is a proxy error
		if h.IsProxyError(err) {
			// For streaming errors, we can't send a proper JSON response
			// since headers are already set. Log the error details.
			details := h.GetProxyErrorDetails(err)
			h.LogProxyError(err, details)
			return
		}
	}
}

// RegisterAnthropicRoutes registers Anthropic routes with the given provider factory
func RegisterAnthropicRoutes(mux *http.ServeMux, factory *provider.Factory) error {
	p, err := factory.Get(provider.ProviderKiro)
	if err != nil {
		return err
	}

	kiroProvider, ok := p.(*kiro.Provider)
	if !ok {
		return nil // Provider not available or wrong type
	}

	handler := NewAnthropicHandler(kiroProvider)
	mux.Handle("/anthropic/", handler)
	return nil
}

// RegisterAnthropicRoutesWithTokenManager registers Anthropic routes with token manager for proxy error handling
func RegisterAnthropicRoutesWithTokenManager(mux *http.ServeMux, factory *provider.Factory, tokenManager *auth.TokenManager) error {
	p, err := factory.Get(provider.ProviderKiro)
	if err != nil {
		return err
	}

	kiroProvider, ok := p.(*kiro.Provider)
	if !ok {
		return nil // Provider not available or wrong type
	}

	handler := NewAnthropicHandlerWithTokenManager(kiroProvider, tokenManager)
	mux.Handle("/anthropic/", handler)
	return nil
}
