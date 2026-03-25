// Package proxy provides HTTP handlers for the proxy server
package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/sunbankio/qwencoder-proxy/internal/converter"
	"github.com/sunbankio/qwencoder-proxy/internal/logging"
	"github.com/sunbankio/qwencoder-proxy/internal/provider"
	"github.com/sunbankio/qwencoder-proxy/internal/ratelimit"
	"github.com/sunbankio/qwencoder-proxy/internal/token"
)

// SequentialHandler implements the sequential flow:
// Model → Provider → Token Selection → API Request → Usage Tracking
type SequentialHandler struct {
	factory       *provider.Factory
	convFactory   *converter.Factory
	tokenSelector *ratelimit.RateAwareTokenSelector
	quotaManager  *ratelimit.QuotaManager
	logger        logging.Logger
}

// NewSequentialHandler creates a new sequential handler
func NewSequentialHandler(factory *provider.Factory, convFactory *converter.Factory,
	tokenSelector *ratelimit.RateAwareTokenSelector,
	quotaManager *ratelimit.QuotaManager,
	logger logging.Logger) *SequentialHandler {
	return &SequentialHandler{
		factory:       factory,
		convFactory:   convFactory,
		tokenSelector: tokenSelector,
		quotaManager:  quotaManager,
		logger:        logger,
	}
}

// ServeHTTP handles the sequential request flow
func (h *SequentialHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Set CORS headers
	h.SetCORSHeaders(w)

	if r.Method == http.MethodOptions {
		h.HandleOptions(w)
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

	h.logger.DebugLog("[SequentialHandler] Original URL.Path: %s, Stripped path: %s, Method: %s",
		originalPath, path, r.Method)

	switch {
	case path == "models" && r.Method == http.MethodGet:
		h.handleListModels(w, r)
	case path == "chat/completions" && r.Method == http.MethodPost:
		h.handleChatCompletions(w, r)
	default:
		h.logger.ErrorLog("[SequentialHandler] No match found - path: '%s', method: '%s'", path, r.Method)
		http.Error(w, fmt.Sprintf("Unsupported endpoint: %s", path), http.StatusNotFound)
	}
}

// handleListModels handles GET /v1/models
func (h *SequentialHandler) handleListModels(w http.ResponseWriter, r *http.Request) {
	var allModels []provider.OpenAIModel

	// Show all models from all providers
	for _, providerType := range h.factory.ListTypes() {
		p, err := h.factory.Get(providerType)
		if err != nil {
			continue
		}

		modelsData, err := p.ListModels(r.Context())
		if err != nil {
			h.logger.ErrorLog("[SequentialHandler] Failed to list models for provider %s: %v", providerType, err)
			continue
		}

		providerModels := h.factory.FormatOpenAIModels(modelsData, providerType)
		allModels = append(allModels, providerModels...)
	}

	response := map[string]interface{}{
		"object": "list",
		"data":   allModels,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleChatCompletions handles POST /v1/chat/completions with sequential flow
func (h *SequentialHandler) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Step 1: Receive incoming request and map model to provider
	var openaiReq map[string]interface{}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		h.logger.ErrorLog("[SequentialHandler] Failed to read request body: %v", err)
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Restore body for potential retries
	r.Body = io.NopCloser(bytes.NewBuffer(body))

	if err := json.Unmarshal(body, &openaiReq); err != nil {
		h.logger.ErrorLog("[SequentialHandler] Failed to decode request: %v", err)
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	model, ok := openaiReq["model"].(string)
	if !ok || model == "" {
		h.logger.ErrorLog("[SequentialHandler] Model is required")
		http.Error(w, "Model is required", http.StatusBadRequest)
		return
	}

	// Map model to provider
	p, err := h.factory.GetByModel(model)
	if err != nil {
		h.logger.ErrorLog("[SequentialHandler] Failed to get provider for model %s: %v", model, err)
		http.Error(w, fmt.Sprintf("Provider not found for model: %s", model), http.StatusBadRequest)
		return
	}

	providerID := string(p.Name())
	h.logger.DebugLog("[SequentialHandler] Step 1: Mapped model %s to provider %s", model, providerID)

	// Step 2: Query tokens.db to retrieve all active tokens for the provider
	selectedToken, err := h.tokenSelector.SelectToken(ctx, providerID)
	if err != nil {
		h.logger.ErrorLog("[SequentialHandler] Step 2: Failed to select token for provider %s: %v", providerID, err)
		http.Error(w, "No available tokens", http.StatusServiceUnavailable)
		return
	}

	h.logger.DebugLog("[SequentialHandler] Step 2: Selected token %s for provider %s", selectedToken.ID, providerID)

	// Step 3: Load-balancing algorithm already applied in tokenSelector.SelectToken()
	// (round-robin or least-connections approach via score calculation)

	// Estimate input tokens for quota checking
	estimatedTokens, err := h.quotaManager.EstimateInputTokens(openaiReq)
	if err != nil {
		h.logger.WarnLog("[SequentialHandler] Failed to estimate tokens: %v", err)
		estimatedTokens = 0 // Conservative: assume zero tokens
	}

	// Check quota before proceeding
	allowed, status, err := h.quotaManager.CheckQuota(ctx, providerID, selectedToken.ID, estimatedTokens)
	if err != nil {
		h.logger.ErrorLog("[SequentialHandler] Failed to check quota: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if !allowed {
		h.logger.WarnLog("[SequentialHandler] Rate limit exceeded for provider %s: %v", providerID, status.LimitReasons)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": map[string]interface{}{
				"message": "The server is currently experiencing high load. Please try again later.",
				"type":    "server_error",
			},
		})
		return
	}

	// Step 4: Execute external API request using selected token
	conv, err := h.convFactory.Get(p.Protocol())
	if err != nil {
		h.logger.ErrorLog("[SequentialHandler] No converter for protocol %s: %v", p.Protocol(), err)
		http.Error(w, "Protocol conversion not supported", http.StatusInternalServerError)
		return
	}

	nativeReq, err := conv.FromOpenAIRequest(openaiReq)
	if err != nil {
		h.logger.ErrorLog("[SequentialHandler] Conversion failed: %v", err)
		http.Error(w, "Failed to convert request", http.StatusInternalServerError)
		return
	}

	isStreaming, _ := openaiReq["stream"].(bool)

	if isStreaming {
		// Handle streaming request
		h.handleStreamingRequest(w, r, ctx, p, conv, nativeReq, model, providerID, selectedToken)
	} else {
		// Handle non-streaming request
		h.handleNonStreamingRequest(w, r, ctx, p, conv, nativeReq, model, providerID, selectedToken)
	}
}

// handleNonStreamingRequest handles non-streaming requests
func (h *SequentialHandler) handleNonStreamingRequest(w http.ResponseWriter, r *http.Request,
	ctx context.Context, p provider.Provider, conv converter.Converter,
	nativeReq interface{}, model string, providerID string, selectedToken *token.ProviderToken) {

	// Create response wrapper for usage tracking
	responseWrapper := &usageTrackingResponseWriter{
		ResponseWriter: w,
		quotaManager:   h.quotaManager,
		tokenID:        selectedToken.ID,
		providerID:     providerID,
		logger:         h.logger,
		ctx:            ctx,
	}

	// Execute request
	response, err := GenerateAndConvert(ctx, p, conv, nativeReq, model)
	if err != nil {
		// Classify and record error
		errorType := h.classifyError(err, 0)
		if recordErr := h.quotaManager.RecordError(ctx, providerID, selectedToken.ID, string(errorType), err.Error(), 0); recordErr != nil {
			h.logger.ErrorLog("[SequentialHandler] Failed to record error: %v", recordErr)
		}
		h.handleError(w, 0, err)
		return
	}

	h.logger.DebugLog("[SequentialHandler] Step 4: Request executed successfully")

	// Write response using responseWrapper (FIXED)
	responseWrapper.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(responseWrapper).Encode(response); err != nil {
		h.logger.ErrorLog("[SequentialHandler] Failed to encode response: %v", err)
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}

	// Step 5: Update database to increment usage counter
	// responseWrapper.statusCode is now properly set by the wrapper
	if responseWrapper.statusCode == http.StatusOK || responseWrapper.statusCode == 0 {
		inputTokens, outputTokens, _, err := h.quotaManager.ExtractTokensFromResponse(response.(map[string]interface{}))
		if err == nil {
			// Record model-specific usage (this now also updates provider_usage and token_usage)
			if err := h.quotaManager.RecordModelUsage(ctx, providerID, selectedToken.ID, model, inputTokens, outputTokens); err != nil {
				h.logger.ErrorLog("[SequentialHandler] Step 5: Failed to record model usage: %v", err)
				h.logger.ErrorLog("[SequentialHandler] Step 5: Model Details - ProviderID: %s, TokenID: %s, Model: %s, Input: %d, Output: %d",
					providerID, selectedToken.ID, model, inputTokens, outputTokens)
			} else {
				h.logger.InfoLog("[SequentialHandler] Step 5: Successfully recorded model usage: %s - %d tokens (input: %d, output: %d)",
					model, inputTokens+outputTokens, inputTokens, outputTokens)
			}
		} else {
			h.logger.WarnLog("[SequentialHandler] Step 5: Failed to extract tokens from response: %v", err)
		}
	} else {
		h.logger.WarnLog("[SequentialHandler] Step 5: Skipping usage recording - status code: %d", responseWrapper.statusCode)
	}
}

// handleStreamingRequest handles streaming requests
func (h *SequentialHandler) handleStreamingRequest(w http.ResponseWriter, r *http.Request,
	ctx context.Context, p provider.Provider, conv converter.Converter,
	nativeReq interface{}, model string, providerID string, selectedToken *token.ProviderToken) {

	h.logger.DebugLog("[SequentialHandler] Step 4: Starting streaming request")

	// Create usage recording wrapper
	usageRecorder := &streamingUsageRecorder{
		ResponseWriter: w,
		quotaManager:   h.quotaManager,
		tokenID:        selectedToken.ID,
		providerID:     providerID,
		model:          model,
		logger:         h.logger,
		ctx:            ctx,
	}

	// Check if stream conversion is needed
	if needsStreamConversion(p.Protocol()) {
		if err := ConvertedStreamResponse(usageRecorder, r, h.factory, p, nativeReq, model, h.logger); err != nil {
			// Classify and record error
			errorType := h.classifyError(err, 0)
			if recordErr := h.quotaManager.RecordError(ctx, providerID, selectedToken.ID, string(errorType), err.Error(), 0); recordErr != nil {
				h.logger.ErrorLog("[SequentialHandler] Failed to record streaming error: %v", recordErr)
			}
			h.logger.ErrorLog("[SequentialHandler] Streaming request failed: %v", err)
			http.Error(w, "Failed to execute request", http.StatusInternalServerError)
			return
		}
	} else {
		if err := StreamResponse(usageRecorder, r, h.factory, p, nativeReq, model, h.logger); err != nil {
			// Classify and record error
			errorType := h.classifyError(err, 0)
			if recordErr := h.quotaManager.RecordError(ctx, providerID, selectedToken.ID, string(errorType), err.Error(), 0); recordErr != nil {
				h.logger.ErrorLog("[SequentialHandler] Failed to record streaming error: %v", recordErr)
			}
			h.logger.ErrorLog("[SequentialHandler] Streaming request failed: %v", err)
			http.Error(w, "Failed to execute request", http.StatusInternalServerError)
			return
		}
	}

	h.logger.DebugLog("[SequentialHandler] Step 4: Streaming request completed")
}

// usageTrackingResponseWriter wraps response writer for usage tracking
type usageTrackingResponseWriter struct {
	http.ResponseWriter
	quotaManager *ratelimit.QuotaManager
	tokenID      string
	providerID   string
	logger       logging.Logger
	ctx          context.Context
	statusCode   int
}

func (w *usageTrackingResponseWriter) WriteHeader(statusCode int) {
	w.statusCode = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}

// streamingUsageRecorder wraps response writer to record usage for streaming responses
type streamingUsageRecorder struct {
	http.ResponseWriter
	quotaManager *ratelimit.QuotaManager
	tokenID      string
	providerID   string
	model        string
	logger       logging.Logger
	ctx          context.Context
	tokenCounted bool
	inputTokens  int
	outputTokens int
}

// Write captures streaming chunks and counts tokens
func (w *streamingUsageRecorder) Write(b []byte) (int, error) {
	// Write to original response writer
	n, err := w.ResponseWriter.Write(b)
	if err != nil {
		return n, err
	}

	// Try to extract and count tokens from SSE chunk
	if !w.tokenCounted {
		// Look for "data:" prefix in SSE chunks
		if bytes.HasPrefix(b, []byte("data: ")) {
			// Extract JSON data after "data: "
			jsonData := bytes.TrimPrefix(b, []byte("data: "))
			jsonData = bytes.TrimSpace(jsonData)

			// Skip [DONE] message
			if bytes.Equal(jsonData, []byte("[DONE]")) {
				return n, nil
			}

			var chunk map[string]interface{}
			if err := json.Unmarshal(jsonData, &chunk); err == nil {
				if usage, ok := chunk["usage"].(map[string]interface{}); ok {
					if promptTokens, ok := usage["prompt_tokens"].(float64); ok {
						w.inputTokens = int(promptTokens)
					}
					if completionTokens, ok := usage["completion_tokens"].(float64); ok {
						w.outputTokens = int(completionTokens)
					}
					if _, ok := usage["total_tokens"].(float64); ok {
						// Mark as counted when we have total tokens
						w.tokenCounted = true
						totalUsage := w.inputTokens + w.outputTokens
						w.logger.DebugLog("[streamingUsageRecorder] Recorded streaming usage: %d tokens (input: %d, output: %d)",
							totalUsage, w.inputTokens, w.outputTokens)

						// Record model usage atomically (this now also updates provider_usage and token_usage)
						if err := w.quotaManager.RecordModelUsage(w.ctx, w.providerID, w.tokenID, w.model, w.inputTokens, w.outputTokens); err != nil {
							w.logger.ErrorLog("[streamingUsageRecorder] Failed to record model usage: %v", err)
						}
					}
				}
			}
		}
	}

	return n, err
}

// WriteHeader passes through to original response writer
func (w *streamingUsageRecorder) WriteHeader(statusCode int) {
	w.ResponseWriter.WriteHeader(statusCode)
}

// SetCORSHeaders sets CORS headers for the response
func (h *SequentialHandler) SetCORSHeaders(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
}

// HandleOptions handles OPTIONS requests for CORS preflight
func (h *SequentialHandler) HandleOptions(w http.ResponseWriter) {
	w.WriteHeader(http.StatusOK)
}

// classifyError determines the type of error based on error message.
func (h *SequentialHandler) classifyError(err error, statusCode int) ratelimit.ErrorType {
	errStr := err.Error()

	// Check for specific error patterns
	if strings.Contains(errStr, "timeout") || strings.Contains(errStr, "deadline exceeded") || strings.Contains(errStr, "context canceled") {
		return ratelimit.ErrorTypeTimeout
	}
	if strings.Contains(errStr, "authentication") || strings.Contains(errStr, "unauthorized") || strings.Contains(errStr, "invalid token") {
		return ratelimit.ErrorTypeAuth
	}
	if strings.Contains(errStr, "connection refused") || strings.Contains(errStr, "network") || strings.Contains(errStr, "no such host") {
		return ratelimit.ErrorTypeNetwork
	}
	if strings.Contains(errStr, "rate limit") || strings.Contains(errStr, "429") || strings.Contains(errStr, "too many requests") {
		return ratelimit.ErrorTypeRateLimit
	}
	if strings.Contains(errStr, "500") || strings.Contains(errStr, "502") || strings.Contains(errStr, "503") || strings.Contains(errStr, "internal server error") {
		return ratelimit.ErrorTypeServerError
	}

	return ratelimit.ErrorTypeUnknown
}

// handleError handles errors and returns appropriate HTTP responses.
func (h *SequentialHandler) handleError(w http.ResponseWriter, statusCode int, err error) {
	// Try to extract status code from error if not provided
	if statusCode == 0 {
		errStr := err.Error()
		if strings.Contains(errStr, "429") || strings.Contains(errStr, "rate limit") {
			statusCode = http.StatusTooManyRequests
		} else if strings.Contains(errStr, "401") || strings.Contains(errStr, "unauthorized") {
			statusCode = http.StatusUnauthorized
		} else if strings.Contains(errStr, "500") || strings.Contains(errStr, "internal server error") {
			statusCode = http.StatusInternalServerError
		} else {
			statusCode = http.StatusInternalServerError
		}
	}

	switch statusCode {
	case http.StatusTooManyRequests:
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": map[string]interface{}{
				"message": "Rate limit exceeded, please try again later",
				"type":    "rate_limit_exceeded",
			},
		})
	case http.StatusUnauthorized:
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": map[string]interface{}{
				"message": "Authentication failed",
				"type":    "authentication_error",
			},
		})
	case http.StatusInternalServerError:
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": map[string]interface{}{
				"message": "Internal server error occurred",
				"type":    "server_error",
			},
		})
	default:
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": map[string]interface{}{
				"message": err.Error(),
				"type":    "request_failed",
			},
		})
	}
}
