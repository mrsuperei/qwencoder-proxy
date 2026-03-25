// Package proxy provides HTTP handlers for the proxy server
package proxy

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/sunbankio/qwencoder-proxy/internal/logging"
	"github.com/sunbankio/qwencoder-proxy/internal/provider"
	"github.com/sunbankio/qwencoder-proxy/internal/provider/gemini"
	auth "github.com/sunbankio/qwencoder-proxy/internal/token"
)

// GeminiHandler handles requests to /gemini/* routes
// Embeds BaseHandler for common functionality
type GeminiHandler struct {
	*BaseHandler // Embedded base handler provides common fields and methods
	provider     *gemini.Provider
}

// NewGeminiHandler creates a new Gemini route handler
// Uses BaseHandler for common functionality
func NewGeminiHandler(p *gemini.Provider) *GeminiHandler {
	return &GeminiHandler{
		BaseHandler: NewBaseHandler(logging.NewLogger(), nil),
		provider:    p,
	}
}

// NewGeminiHandlerWithTokenManager creates a new Gemini route handler with token manager for proxy error handling
// Uses BaseHandler for common functionality
func NewGeminiHandlerWithTokenManager(p *gemini.Provider, tokenManager *auth.TokenManager) *GeminiHandler {
	return &GeminiHandler{
		BaseHandler: NewBaseHandler(logging.NewLogger(), tokenManager),
		provider:    p,
	}
}

// ServeHTTP handles HTTP requests for Gemini routes
func (h *GeminiHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Set CORS headers using BaseHandler method
	h.SetCORSHeaders(w)

	if r.Method == http.MethodOptions {
		h.HandleOptions(w) // Use BaseHandler method
		return
	}

	// Parse the path: /gemini/models, /gemini/models/{model}:generateContent, etc.
	path := strings.TrimPrefix(r.URL.Path, "/gemini")
	path = strings.TrimPrefix(path, "/")

	h.GetLogger().DebugLog("[Gemini Handler] Request: %s %s", r.Method, path)

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
		h.GetLogger().ErrorLog("[Gemini Handler] Failed to list models: %v", err)

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
		h.GetLogger().ErrorLog("[Gemini Handler] Failed to encode response: %v", err)
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
		h.GetLogger().ErrorLog("[Gemini Handler] Failed to decode request: %v", err)
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	response, err := h.provider.GenerateContent(ctx, model, &request)
	if err != nil {
		h.GetLogger().ErrorLog("[Gemini Handler] GenerateContent failed: %v", err)

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
		h.GetLogger().ErrorLog("[Gemini Handler] Failed to encode response: %v", err)
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
		h.GetLogger().ErrorLog("[Gemini Handler] Failed to decode request: %v", err)
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Create a minimal factory for the converter (this is a temporary solution)
	// In a real implementation, the factory should be passed to the handler
	factory := &provider.Factory{}
	if err := ConvertedStreamResponse(w, r, factory, h.provider, &request, model, h.GetLogger()); err != nil {
		h.GetLogger().ErrorLog("[Gemini Handler] Stream error: %v", err)

		// Check if this is a proxy error
		if h.IsProxyError(err) {
			h.HandleProxyError(w, err)
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
