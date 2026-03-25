// Package proxy provides HTTP handlers for the proxy server
package proxy

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/sunbankio/qwencoder-proxy/internal/converter"
	"github.com/sunbankio/qwencoder-proxy/internal/logging"
	"github.com/sunbankio/qwencoder-proxy/internal/provider"
	auth "github.com/sunbankio/qwencoder-proxy/internal/token"
)

// OpenAIHandler handles OpenAI-compatible requests and routes them to appropriate providers
// Embeds BaseHandler for common functionality
type OpenAIHandler struct {
	*BaseHandler  // Embedded base handler provides common fields and methods
	factory       *provider.Factory
	convFactory   *converter.Factory
	fixedProvider provider.ProviderType // If set, always use this provider
}

// NewOpenAIHandler creates a new OpenAI-compatible handler
// Uses BaseHandler for common functionality
func NewOpenAIHandler(factory *provider.Factory, convFactory *converter.Factory) *OpenAIHandler {
	return &OpenAIHandler{
		BaseHandler:   NewBaseHandler(logging.NewLogger(), nil),
		factory:       factory,
		convFactory:   convFactory,
		fixedProvider: "",
	}
}

// NewOpenAIHandlerWithTokenManager creates a new OpenAI-compatible handler with token manager for proxy error handling
// Uses BaseHandler for common functionality
func NewOpenAIHandlerWithTokenManager(factory *provider.Factory, convFactory *converter.Factory, tokenManager *auth.TokenManager) *OpenAIHandler {
	return &OpenAIHandler{
		BaseHandler:   NewBaseHandler(logging.NewLogger(), tokenManager),
		factory:       factory,
		convFactory:   convFactory,
		fixedProvider: "",
	}
}

// NewProviderSpecificHandler creates a new handler that forces requests to use a specific provider
// Uses BaseHandler for common functionality
func NewProviderSpecificHandler(factory *provider.Factory, convFactory *converter.Factory, providerType provider.ProviderType) *OpenAIHandler {
	return &OpenAIHandler{
		BaseHandler:   NewBaseHandler(logging.NewLogger(), nil),
		factory:       factory,
		convFactory:   convFactory,
		fixedProvider: providerType,
	}
}

// NewProviderSpecificHandlerWithTokenManager creates a new handler that forces requests to use a specific provider with token manager
// Uses BaseHandler for common functionality
func NewProviderSpecificHandlerWithTokenManager(factory *provider.Factory, convFactory *converter.Factory, providerType provider.ProviderType, tokenManager *auth.TokenManager) *OpenAIHandler {
	return &OpenAIHandler{
		BaseHandler:   NewBaseHandler(logging.NewLogger(), tokenManager),
		factory:       factory,
		convFactory:   convFactory,
		fixedProvider: providerType,
	}
}

// ServeHTTP handles HTTP requests for OpenAI-compatible routes
func (h *OpenAIHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Set CORS headers using BaseHandler method
	h.SetCORSHeaders(w)

	if r.Method == http.MethodOptions {
		h.HandleOptions(w) // Use BaseHandler method
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

	h.GetLogger().DebugLog("[Handler] Original URL.Path: %s, Stripped path: %s, Method: %s (Fixed Provider: %s)", originalPath, path, r.Method, h.fixedProvider)

	switch {
	case path == "models" && r.Method == http.MethodGet:
		h.handleListModels(w, r)
	case path == "chat/completions" && r.Method == http.MethodPost:
		h.handleChatCompletions(w, r)
	default:
		h.GetLogger().ErrorLog("[Handler] No match found - path: '%s', method: '%s', expecting 'models' (GET) or 'chat/completions' (POST)", path, r.Method)
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
			h.GetLogger().ErrorLog("[Handler] Failed to list models for provider %s: %v", h.fixedProvider, err)
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
				h.GetLogger().ErrorLog("[Handler] Failed to list models for provider %s: %v", providerType, err)
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
		h.GetLogger().ErrorLog("[Handler] Failed to decode request: %v", err)
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
		h.GetLogger().ErrorLog("[Handler] Failed to get provider: %v", err)
		http.Error(w, fmt.Sprintf("Provider not found: %v", err), http.StatusBadRequest)
		return
	}

	h.GetLogger().DebugLog("[Handler] Using provider %s for model %s", p.Name(), model)

	conv, err := h.convFactory.Get(p.Protocol())
	if err != nil {
		h.GetLogger().ErrorLog("[Handler] No converter for protocol %s: %v", p.Protocol(), err)
		http.Error(w, "Protocol conversion not supported", http.StatusInternalServerError)
		return
	}

	nativeReq, err := conv.FromOpenAIRequest(openaiReq)
	if err != nil {
		h.GetLogger().ErrorLog("[Handler] Conversion failed: %v", err)
		http.Error(w, "Failed to convert request", http.StatusInternalServerError)
		return
	}

	isStreaming, _ := openaiReq["stream"].(bool)
	if isStreaming {
		// Use converted streaming for providers that need format conversion
		if needsStreamConversion(p.Protocol()) {
			if err := ConvertedStreamResponse(w, r, h.factory, p, nativeReq, model, h.GetLogger()); err != nil {
				h.handleStreamingError(w, err)
			}
		} else {
			// Use raw streaming for providers that already format correctly (like Kiro)
			if err := StreamResponse(w, r, h.factory, p, nativeReq, model, h.GetLogger()); err != nil {
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
		h.GetLogger().ErrorLog("[Handler] GenerateAndConvert failed with %s: %v", p.Name(), err)

		// Check if this is a proxy error
		if h.IsProxyError(err) {
			h.HandleProxyError(w, err)
			return
		}

		// Try alternative if not a fixed provider request
		if h.fixedProvider == "" {
			if altProvider, altErr := h.factory.GetAlternativeProvider(model, p.Name()); altErr == nil {
				h.GetLogger().DebugLog("[Handler] Retrying with alternative %s", altProvider.Name())
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
	if h.IsProxyError(err) {
		h.HandleProxyError(w, err)
		return
	}

	// For other errors, log and return generic error
	h.GetLogger().ErrorLog("[Handler] Streaming error: %v", err)
	http.Error(w, "Streaming failed", http.StatusInternalServerError)
}

// RegisterOpenAIRoutes registers all OpenAI-compatible routes
func RegisterOpenAIRoutes(mux *http.ServeMux, factory *provider.Factory, convFactory *converter.Factory) {
	// General route
	mux.Handle("/v1/", NewOpenAIHandler(factory, convFactory))
}

// RegisterOpenAIRoutesWithTokenManager registers all OpenAI-compatible routes with token manager support
// The general /v1/ route uses nil token manager as providers have their own
func RegisterOpenAIRoutesWithTokenManager(mux *http.ServeMux, factory *provider.Factory, convFactory *converter.Factory, tokenManager *auth.TokenManager) {
	// General route - use nil token manager since providers have their own
	mux.Handle("/v1/", NewOpenAIHandlerWithTokenManager(factory, convFactory, nil))
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

// RegisterProviderSpecificRoutesWithTokenManager registers provider-specific OpenAI-compatible routes with token manager support
// Each provider route uses its respective token manager for proxy-aware token selection
func RegisterProviderSpecificRoutesWithTokenManager(mux *http.ServeMux, factory *provider.Factory, convFactory *converter.Factory, tokenManagers map[provider.ProviderType]*auth.TokenManager) {
	// Provider-specific routes with token managers
	if tm, ok := tokenManagers[provider.ProviderQwen]; ok {
		mux.Handle("/qwen/v1/", NewProviderSpecificHandlerWithTokenManager(factory, convFactory, provider.ProviderQwen, tm))
	} else {
		mux.Handle("/qwen/v1/", NewProviderSpecificHandlerWithTokenManager(factory, convFactory, provider.ProviderQwen, nil))
	}

	if tm, ok := tokenManagers[provider.ProviderGeminiCLI]; ok {
		mux.Handle("/gemini/v1/", NewProviderSpecificHandlerWithTokenManager(factory, convFactory, provider.ProviderGeminiCLI, tm))
	} else {
		mux.Handle("/gemini/v1/", NewProviderSpecificHandlerWithTokenManager(factory, convFactory, provider.ProviderGeminiCLI, nil))
	}

	if tm, ok := tokenManagers[provider.ProviderKiro]; ok {
		mux.Handle("/kiro/v1/", NewProviderSpecificHandlerWithTokenManager(factory, convFactory, provider.ProviderKiro, tm))
	} else {
		mux.Handle("/kiro/v1/", NewProviderSpecificHandlerWithTokenManager(factory, convFactory, provider.ProviderKiro, nil))
	}

	if tm, ok := tokenManagers[provider.ProviderAntigravity]; ok {
		mux.Handle("/antigravity/v1/", NewProviderSpecificHandlerWithTokenManager(factory, convFactory, provider.ProviderAntigravity, tm))
	} else {
		mux.Handle("/antigravity/v1/", NewProviderSpecificHandlerWithTokenManager(factory, convFactory, provider.ProviderAntigravity, nil))
	}

	if tm, ok := tokenManagers[provider.ProviderIFlow]; ok {
		mux.Handle("/iflow/v1/", NewProviderSpecificHandlerWithTokenManager(factory, convFactory, provider.ProviderIFlow, tm))
	} else {
		mux.Handle("/iflow/v1/", NewProviderSpecificHandlerWithTokenManager(factory, convFactory, provider.ProviderIFlow, nil))
	}
}
