package ratelimit

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/sunbankio/qwencoder-proxy/internal/logging"
	"github.com/sunbankio/qwencoder-proxy/internal/provider"
	"github.com/sunbankio/qwencoder-proxy/internal/token"
)

// RateLimitMiddleware creates HTTP middleware for rate limiting.
type RateLimitMiddleware struct {
	quotaManager    *QuotaManager
	logger          logging.Logger
	providerFactory *provider.Factory
}

// NewRateLimitMiddleware creates a new rate limiting middleware.
func NewRateLimitMiddleware(quotaManager *QuotaManager, logger logging.Logger,
	providerFactory *provider.Factory, db *sql.DB) *RateLimitMiddleware {
	return &RateLimitMiddleware{
		quotaManager:    quotaManager,
		logger:          logger,
		providerFactory: providerFactory,
	}
}

// Wrap wraps an http.Handler with rate limiting.
func (m *RateLimitMiddleware) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Only rate limit POST requests to /v1/chat/completions.
		if r.Method != http.MethodPost || r.URL.Path != "/v1/chat/completions" {
			next.ServeHTTP(w, r)
			return
		}

		// Parse request body to extract model/provider info.
		var openaiReq map[string]interface{}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			m.logger.ErrorLog("[RateLimitMiddleware] Failed to read request body: %v", err)
			http.Error(w, "Failed to read request body", http.StatusBadRequest)
			return
		}
		defer r.Body.Close()

		// Restore body for downstream handlers.
		r.Body = io.NopCloser(bytes.NewBuffer(body))

		if err := json.Unmarshal(body, &openaiReq); err != nil {
			m.logger.ErrorLog("[RateLimitMiddleware] Failed to parse request body: %v", err)
			http.Error(w, "Failed to parse request body", http.StatusBadRequest)
			return
		}

		// Extract model from request
		model, ok := openaiReq["model"].(string)
		if !ok || model == "" {
			m.logger.ErrorLog("[RateLimitMiddleware] Model is required")
			http.Error(w, "Model is required", http.StatusBadRequest)
			return
		}

		// Use Provider Factory to resolve provider
		provider, err := m.providerFactory.GetByModel(model)
		if err != nil {
			m.logger.ErrorLog("[RateLimitMiddleware] Failed to get provider for model %s: %v", model, err)
			http.Error(w, fmt.Sprintf("Provider not found for model: %s", model), http.StatusBadRequest)
			return
		}

		providerID := string(provider.Name())
		m.logger.DebugLog("[RateLimitMiddleware] Resolved model %s to provider %s", model, providerID)

		// Step 2: Select token using rate-aware selector
		selectedToken, err := m.quotaManager.tokenSelector.SelectToken(r.Context(), providerID)
		if err != nil {
			m.logger.ErrorLog("[RateLimitMiddleware] Failed to select token: %v", err)
			http.Error(w, "No available tokens", http.StatusServiceUnavailable)
			return
		}

		m.logger.DebugLog("[RateLimitMiddleware] Selected token %s for provider %s", selectedToken.ID, providerID)

		// Store selected token in context for downstream handlers
		ctx := context.WithValue(r.Context(), "selected_token", selectedToken)

		// Check if streaming request.
		isStreaming, _ := openaiReq["stream"].(bool)

		// Estimate input tokens for quota checking.
		estimatedTokens, err := m.quotaManager.EstimateInputTokens(openaiReq)
		if err != nil {
			m.logger.WarnLog("[RateLimitMiddleware] Failed to estimate tokens: %v", err)
			estimatedTokens = 0 // Conservative: assume zero tokens.
		}

		// Check quota before proceeding.
		allowed, status, err := m.quotaManager.CheckQuota(ctx, providerID, selectedToken.ID, estimatedTokens)
		if err != nil {
			m.logger.ErrorLog("[RateLimitMiddleware] Failed to check quota: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		if !allowed {
			// Rate limit exceeded - return 429 without exposing rate limit info.
			m.logger.WarnLog("[RateLimitMiddleware] Rate limit exceeded for provider %s: %v", providerID, status.LimitReasons)

			// Return generic error message.
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

		// For non-streaming requests, wrap response writer to capture response for token counting.
		if !isStreaming {
			wrappedWriter := &responseCaptureWriter{
				ResponseWriter: w,
				body:           bytes.NewBuffer(nil),
				statusCode:     http.StatusOK,
			}

			// Call next handler.
			next.ServeHTTP(wrappedWriter, r.WithContext(ctx))

			// If request was successful, extract token counts from response.
			if wrappedWriter.statusCode == http.StatusOK {
				var response map[string]interface{}
				if err := json.Unmarshal(wrappedWriter.body.Bytes(), &response); err == nil {
					inputTokens, outputTokens, totalTokens, err := m.quotaManager.ExtractTokensFromResponse(response)
					if err == nil {
						// Get selected token from context
						selectedToken := ctx.Value("selected_token").(*token.ProviderToken)

						// Record actual token usage from provider response.
						if err := m.quotaManager.RecordUsage(ctx, providerID, selectedToken.ID, inputTokens, outputTokens); err != nil {
							m.logger.ErrorLog("[RateLimitMiddleware] Failed to record usage: %v", err)
						} else {
							m.logger.DebugLog("[RateLimitMiddleware] Recorded usage: %d tokens (input: %d, output: %d)",
								totalTokens, inputTokens, outputTokens)
						}
					}
				}
			}
		} else {
			// For streaming requests, use a usage recording wrapper.
			selectedToken := ctx.Value("selected_token").(*token.ProviderToken)
			usageRecorder := &usageRecordingWriter{
				ResponseWriter: w,
				quotaManager:   m.quotaManager,
				logger:         m.logger,
				providerID:     providerID,
				ctx:            ctx,
				requestBody:    body,
				selectedToken:  selectedToken,
			}

			// Call next handler with usage recording wrapper.
			next.ServeHTTP(usageRecorder, r.WithContext(ctx))
		}
	})
}

// responseCaptureWriter captures response body for token counting.
type responseCaptureWriter struct {
	http.ResponseWriter
	body       *bytes.Buffer
	statusCode int
}

func (w *responseCaptureWriter) Write(b []byte) (int, error) {
	w.body.Write(b)
	return w.ResponseWriter.Write(b)
}

func (w *responseCaptureWriter) WriteHeader(statusCode int) {
	w.statusCode = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}

// usageRecordingWriter wraps response writer to record usage for streaming responses.
// It captures token counts from streaming SSE chunks.
type usageRecordingWriter struct {
	http.ResponseWriter
	quotaManager  *QuotaManager
	logger        logging.Logger
	providerID    string
	ctx           context.Context
	requestBody   []byte
	tokenCounted  bool
	inputTokens   int
	outputTokens  int
	selectedToken *token.ProviderToken
}

// Write captures streaming chunks and counts tokens.
func (w *usageRecordingWriter) Write(b []byte) (int, error) {
	// Write to original response writer.
	n, err := w.ResponseWriter.Write(b)
	if err != nil {
		return n, err
	}

	// Try to extract and count tokens from SSE chunk.
	if !w.tokenCounted {
		var chunk map[string]interface{}
		if err := json.Unmarshal(b, &chunk); err == nil {
			if usage, ok := chunk["usage"].(map[string]interface{}); ok {
				if promptTokens, ok := usage["prompt_tokens"].(float64); ok {
					w.inputTokens = int(promptTokens)
				}
				if completionTokens, ok := usage["completion_tokens"].(float64); ok {
					w.outputTokens = int(completionTokens)
				}
				if _, ok := usage["total_tokens"].(float64); ok {
					// Mark as counted when we have total tokens.
					w.tokenCounted = true
					totalUsage := w.inputTokens + w.outputTokens
					w.logger.DebugLog("[usageRecordingWriter] Recorded streaming usage: %d tokens (input: %d, output: %d)",
						totalUsage, w.inputTokens, w.outputTokens)

					// Record usage atomically.
					if err := w.quotaManager.RecordUsage(w.ctx, w.providerID, w.selectedToken.ID, w.inputTokens, w.outputTokens); err != nil {
						w.logger.ErrorLog("[usageRecordingWriter] Failed to record usage: %v", err)
					}
				}
			}
		}
	}

	return n, err
}

// WriteHeader passes through to original response writer.
func (w *usageRecordingWriter) WriteHeader(statusCode int) {
	w.ResponseWriter.WriteHeader(statusCode)
}
