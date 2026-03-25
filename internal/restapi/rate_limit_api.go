package restapi

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/sunbankio/qwencoder-proxy/internal/logging"
	"github.com/sunbankio/qwencoder-proxy/internal/ratelimit"
)

// RateLimitAPI handles rate limit management endpoints.
type RateLimitAPI struct {
	quotaManager     *ratelimit.QuotaManager
	logger           logging.Logger
	cacheInvalidator *ratelimit.CacheInvalidator
}

// NewRateLimitAPI creates a new rate limit API handler.
func NewRateLimitAPI(quotaManager *ratelimit.QuotaManager, logger logging.Logger, cacheInvalidator *ratelimit.CacheInvalidator) *RateLimitAPI {
	return &RateLimitAPI{
		quotaManager:     quotaManager,
		logger:           logger,
		cacheInvalidator: cacheInvalidator,
	}
}

// RegisterRoutes registers rate limit API routes.
func (api *RateLimitAPI) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/ratelimit/config", api.handleConfigs)
	mux.HandleFunc("/api/ratelimit/config/", api.handleProviderConfig)
	mux.HandleFunc("/api/ratelimit/usage", api.handleUsage)
	mux.HandleFunc("/api/ratelimit/usage/", api.handleProviderUsage)
	mux.HandleFunc("/api/ratelimit/reset/", api.handleReset)
	// Model usage endpoints
	mux.HandleFunc("/api/ratelimit/model-usage", api.handleModelUsage)
	mux.HandleFunc("/api/ratelimit/model-usage/", api.handleModelUsagePath)
	// Error tracking endpoints
	mux.HandleFunc("/api/ratelimit/errors", api.handleErrors)
	mux.HandleFunc("/api/ratelimit/errors/", api.handleTokenErrors)
	mux.HandleFunc("/api/ratelimit/errors/reset/", api.handleResetErrors)
	// Request history endpoints
	mux.HandleFunc("/api/ratelimit/request-history", api.handleRequestHistory)
	mux.HandleFunc("/api/ratelimit/request-history/", api.handleRequestHistoryPath)
	mux.HandleFunc("/api/ratelimit/request-history/summary", api.handleRequestHistorySummary)
	mux.HandleFunc("/api/ratelimit/request-history/delete", api.handleDeleteRequestHistory)
}

// handleConfigs handles GET /api/ratelimit/config.
func (api *RateLimitAPI) handleConfigs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed")
		return
	}

	configs := api.quotaManager.GetAllConfigs()
	WriteJSON(w, http.StatusOK, configs)
}

// handleProviderConfig handles GET/PUT /api/ratelimit/config/:provider.
func (api *RateLimitAPI) handleProviderConfig(w http.ResponseWriter, r *http.Request) {
	// Extract provider ID from path.
	path := strings.TrimPrefix(r.URL.Path, "/api/ratelimit/config/")
	if path == "" {
		WriteError(w, http.StatusBadRequest, "invalid_request", "Provider ID required")
		return
	}

	// Handle trailing slash.
	path = strings.TrimSuffix(path, "/")

	switch r.Method {
	case http.MethodGet:
		config, err := api.quotaManager.GetConfig(path)
		if err != nil {
			WriteError(w, http.StatusNotFound, "not_found", err.Error())
			return
		}
		WriteJSON(w, http.StatusOK, config)

	case http.MethodPut:
		var config ratelimit.ProviderRateLimitConfig
		if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
			WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON")
			return
		}

		if err := api.quotaManager.UpdateConfig(path, config); err != nil {
			WriteError(w, http.StatusInternalServerError, "update_failed", err.Error())
			return
		}

		// Publish cache invalidation event for config changes
		if api.cacheInvalidator != nil {
			api.cacheInvalidator.PublishInvalidation(ratelimit.InvalidationConfig, "", "api")
		}

		WriteJSON(w, http.StatusOK, map[string]interface{}{"message": "Configuration updated"})

	default:
		WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed")
	}
}

// handleUsage handles GET /api/ratelimit/usage.
func (api *RateLimitAPI) handleUsage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed")
		return
	}

	// Return all provider usage.
	usage, err := api.quotaManager.GetAllProviderUsage(r.Context())
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "query_failed", err.Error())
		return
	}

	WriteJSON(w, http.StatusOK, usage)
}

// handleProviderUsage handles GET /api/ratelimit/usage/:provider or /api/ratelimit/usage/:provider/:token.
func (api *RateLimitAPI) handleProviderUsage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed")
		return
	}

	// Extract provider ID and optional token ID from path.
	path := strings.TrimPrefix(r.URL.Path, "/api/ratelimit/usage/")
	if path == "" {
		WriteError(w, http.StatusBadRequest, "invalid_request", "Provider ID required")
		return
	}

	// Handle trailing slash.
	path = strings.TrimSuffix(path, "/")

	// Parse provider ID and optional token ID.
	parts := strings.Split(path, "/")
	providerID := parts[0]
	tokenID := ""
	if len(parts) > 1 {
		tokenID = parts[1]
	}

	// Get quota status.
	status, err := api.quotaManager.GetQuotaStatus(r.Context(), providerID, tokenID)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "query_failed", err.Error())
		return
	}

	WriteJSON(w, http.StatusOK, status)
}

// handleReset handles POST /api/ratelimit/reset/:provider.
func (api *RateLimitAPI) handleReset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed")
		return
	}

	// Extract provider ID from path.
	path := strings.TrimPrefix(r.URL.Path, "/api/ratelimit/reset/")
	if path == "" {
		WriteError(w, http.StatusBadRequest, "invalid_request", "Provider ID required")
		return
	}

	// Handle trailing slash.
	path = strings.TrimSuffix(path, "/")

	// Parse provider ID and optional token ID.
	parts := strings.Split(path, "/")
	providerID := parts[0]
	tokenID := ""
	if len(parts) > 1 {
		tokenID = parts[1]
	}

	if err := api.quotaManager.ResetUsage(r.Context(), providerID, tokenID); err != nil {
		WriteError(w, http.StatusInternalServerError, "reset_failed", err.Error())
		return
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{"message": "Usage reset"})
}

// handleModelUsage handles GET /api/ratelimit/model-usage.
// Returns all model usage across all providers.
func (api *RateLimitAPI) handleModelUsage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed")
		return
	}

	// Get all provider usage
	allProviderUsage, err := api.quotaManager.GetAllProviderUsage(r.Context())
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "query_failed", err.Error())
		return
	}

	// Collect all model usage
	allModelUsage := make(map[string]map[string]*ratelimit.ModelUsageMetrics)
	for providerID := range allProviderUsage {
		modelUsage, err := api.quotaManager.GetAllModelUsage(r.Context(), providerID)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "query_failed", err.Error())
			return
		}
		allModelUsage[providerID] = modelUsage
	}

	WriteJSON(w, http.StatusOK, allModelUsage)
}

// handleModelUsagePath handles GET /api/ratelimit/model-usage/:provider or /api/ratelimit/model-usage/:provider/:token or /api/ratelimit/model-usage/:provider/:token/:model.
func (api *RateLimitAPI) handleModelUsagePath(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed")
		return
	}

	// Extract provider ID and optional token ID and model from path.
	path := strings.TrimPrefix(r.URL.Path, "/api/ratelimit/model-usage/")
	if path == "" {
		WriteError(w, http.StatusBadRequest, "invalid_request", "Provider ID required")
		return
	}

	// Handle trailing slash.
	path = strings.TrimSuffix(path, "/")

	// Parse provider ID, optional token ID, and optional model.
	parts := strings.Split(path, "/")
	providerID := parts[0]
	tokenID := ""
	model := ""

	if len(parts) > 1 {
		tokenID = parts[1]
	}
	if len(parts) > 2 {
		model = parts[2]
	}

	// Return appropriate response based on path depth.
	if tokenID == "" {
		// GET /api/ratelimit/model-usage/:provider - all model usage for provider
		modelUsage, err := api.quotaManager.GetAllModelUsage(r.Context(), providerID)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "query_failed", err.Error())
			return
		}
		WriteJSON(w, http.StatusOK, modelUsage)
	} else if model == "" {
		// GET /api/ratelimit/model-usage/:provider/:token - all model usage for token
		modelUsage, err := api.quotaManager.GetTokenModelUsage(r.Context(), tokenID)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "query_failed", err.Error())
			return
		}
		WriteJSON(w, http.StatusOK, modelUsage)
	} else {
		// GET /api/ratelimit/model-usage/:provider/:token/:model - specific model usage
		modelUsage, err := api.quotaManager.GetModelUsage(r.Context(), providerID, tokenID, model)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "query_failed", err.Error())
			return
		}
		WriteJSON(w, http.StatusOK, modelUsage)
	}
}

// handleErrors handles GET /api/ratelimit/errors?provider_id=:provider.
// Returns error information for all tokens of a provider.
func (api *RateLimitAPI) handleErrors(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed")
		return
	}

	// Extract provider_id from query parameters
	providerID := r.URL.Query().Get("provider_id")
	if providerID == "" {
		WriteError(w, http.StatusBadRequest, "invalid_request", "provider_id query parameter is required")
		return
	}

	// Get all error information for the provider
	errors, err := api.quotaManager.GetAllTokenErrors(r.Context(), providerID)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "query_failed", err.Error())
		return
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"provider_id": providerID,
		"errors":      errors,
	})
}

// handleTokenErrors handles GET /api/ratelimit/errors/:provider/:token.
// Returns error information for a specific token.
func (api *RateLimitAPI) handleTokenErrors(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed")
		return
	}

	// Extract provider ID and token ID from path
	path := strings.TrimPrefix(r.URL.Path, "/api/ratelimit/errors/")
	if path == "" {
		WriteError(w, http.StatusBadRequest, "invalid_request", "Provider ID and token ID required")
		return
	}

	// Handle trailing slash
	path = strings.TrimSuffix(path, "/")

	// Parse provider ID and token ID
	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		WriteError(w, http.StatusBadRequest, "invalid_request", "Provider ID and token ID required")
		return
	}

	providerID := parts[0]
	tokenID := parts[1]

	// Get error information for the token
	errorInfo, err := api.quotaManager.GetTokenErrors(r.Context(), providerID, tokenID)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "query_failed", err.Error())
		return
	}

	WriteJSON(w, http.StatusOK, errorInfo)
}

// handleResetErrors handles POST /api/ratelimit/errors/reset/:provider/:token.
// Resets error tracking for a specific token.
func (api *RateLimitAPI) handleResetErrors(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed")
		return
	}

	// Extract provider ID and token ID from path
	path := strings.TrimPrefix(r.URL.Path, "/api/ratelimit/errors/reset/")
	if path == "" {
		WriteError(w, http.StatusBadRequest, "invalid_request", "Provider ID and token ID required")
		return
	}

	// Handle trailing slash
	path = strings.TrimSuffix(path, "/")

	// Parse provider ID and token ID
	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		WriteError(w, http.StatusBadRequest, "invalid_request", "Provider ID and token ID required")
		return
	}

	providerID := parts[0]
	tokenID := parts[1]

	// Reset errors for the token
	if err := api.quotaManager.ResetTokenErrors(r.Context(), providerID, tokenID); err != nil {
		WriteError(w, http.StatusInternalServerError, "reset_failed", err.Error())
		return
	}

	// Publish cache invalidation event
	if api.cacheInvalidator != nil {
		api.cacheInvalidator.PublishInvalidation(ratelimit.InvalidationToken, tokenID, "api")
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"message":     "errors_reset",
		"token_id":    tokenID,
		"provider_id": providerID,
	})
}

// handleRequestHistory handles GET /api/ratelimit/request-history with query parameters.
// Query parameters:
//   - token_id: Filter by token ID
//   - model: Filter by model
//   - start_time: Start timestamp (milliseconds)
//   - end_time: End timestamp (milliseconds)
//   - min_tokens: Minimum total tokens
//   - max_tokens: Maximum total tokens
//   - page: Page number (default: 1)
//   - page_size: Page size (default: 100, max: 1000)
func (api *RateLimitAPI) handleRequestHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed")
		return
	}

	// Parse query parameters
	filter := &ratelimit.RequestHistoryFilter{
		TokenID:   r.URL.Query().Get("token_id"),
		Model:     r.URL.Query().Get("model"),
		StartTime: parseTimestamp(r.URL.Query().Get("start_time")),
		EndTime:   parseTimestamp(r.URL.Query().Get("end_time")),
	}

	if minTokens := r.URL.Query().Get("min_tokens"); minTokens != "" {
		if val, err := strconv.Atoi(minTokens); err == nil {
			filter.MinTokens = val
		}
	}
	if maxTokens := r.URL.Query().Get("max_tokens"); maxTokens != "" {
		if val, err := strconv.Atoi(maxTokens); err == nil {
			filter.MaxTokens = val
		}
	}

	// Parse pagination
	page := parsePage(r.URL.Query().Get("page"))
	pageSize := parsePageSize(r.URL.Query().Get("page_size"))

	// Get request history
	response, err := api.quotaManager.GetRequestHistory(r.Context(), filter, page, pageSize)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "query_failed", err.Error())
		return
	}

	WriteJSON(w, http.StatusOK, response)
}

// handleRequestHistoryPath handles GET /api/ratelimit/request-history/:token or /api/ratelimit/request-history/:token/:model.
func (api *RateLimitAPI) handleRequestHistoryPath(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed")
		return
	}

	// Extract path components
	path := strings.TrimPrefix(r.URL.Path, "/api/ratelimit/request-history/")
	if path == "" {
		WriteError(w, http.StatusBadRequest, "invalid_request", "Token ID required")
		return
	}
	path = strings.TrimSuffix(path, "/")

	parts := strings.Split(path, "/")
	if len(parts) == 0 {
		WriteError(w, http.StatusBadRequest, "invalid_request", "Token ID required")
		return
	}

	tokenID := parts[0]
	model := ""
	if len(parts) > 1 && parts[1] != "summary" && parts[1] != "delete" {
		model = parts[1]
	}

	// Parse query parameters for additional filtering
	filter := &ratelimit.RequestHistoryFilter{
		TokenID:   tokenID,
		Model:     model,
		StartTime: parseTimestamp(r.URL.Query().Get("start_time")),
		EndTime:   parseTimestamp(r.URL.Query().Get("end_time")),
	}

	if minTokens := r.URL.Query().Get("min_tokens"); minTokens != "" {
		if val, err := strconv.Atoi(minTokens); err == nil {
			filter.MinTokens = val
		}
	}
	if maxTokens := r.URL.Query().Get("max_tokens"); maxTokens != "" {
		if val, err := strconv.Atoi(maxTokens); err == nil {
			filter.MaxTokens = val
		}
	}

	// Parse pagination
	page := parsePage(r.URL.Query().Get("page"))
	pageSize := parsePageSize(r.URL.Query().Get("page_size"))

	// Get request history
	var response *ratelimit.RequestHistoryResponse
	var err error

	if model == "" {
		response, err = api.quotaManager.GetRequestHistoryByToken(r.Context(), tokenID, filter, page, pageSize)
	} else {
		response, err = api.quotaManager.GetRequestHistoryByModel(r.Context(), model, filter, page, pageSize)
	}

	if err != nil {
		WriteError(w, http.StatusInternalServerError, "query_failed", err.Error())
		return
	}

	WriteJSON(w, http.StatusOK, response)
}

// handleRequestHistorySummary handles GET /api/ratelimit/request-history/summary.
// Query parameters:
//   - token_id: Filter by token ID (optional)
//   - model: Filter by model (optional)
//   - start_time: Start timestamp (milliseconds, optional)
//   - end_time: End timestamp (milliseconds, optional)
func (api *RateLimitAPI) handleRequestHistorySummary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed")
		return
	}

	// Parse query parameters
	tokenID := r.URL.Query().Get("token_id")
	model := r.URL.Query().Get("model")
	startTime := parseTimestamp(r.URL.Query().Get("start_time"))
	endTime := parseTimestamp(r.URL.Query().Get("end_time"))

	// Get summary
	summary, err := api.quotaManager.GetRequestHistorySummary(r.Context(), tokenID, model, startTime, endTime)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "query_failed", err.Error())
		return
	}

	WriteJSON(w, http.StatusOK, summary)
}

// handleDeleteRequestHistory handles DELETE /api/ratelimit/request-history/delete.
// Query parameters (at least one required):
//   - token_id: Filter by token ID
//   - model: Filter by model
//   - start_time: Start timestamp (milliseconds)
//   - end_time: End timestamp (milliseconds)
func (api *RateLimitAPI) handleDeleteRequestHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodDelete {
		WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed")
		return
	}

	// Parse query parameters
	filter := &ratelimit.RequestHistoryFilter{
		TokenID:   r.URL.Query().Get("token_id"),
		Model:     r.URL.Query().Get("model"),
		StartTime: parseTimestamp(r.URL.Query().Get("start_time")),
		EndTime:   parseTimestamp(r.URL.Query().Get("end_time")),
	}

	// Validate that at least one filter is provided
	if filter.TokenID == "" && filter.Model == "" && filter.StartTime == 0 && filter.EndTime == 0 {
		WriteError(w, http.StatusBadRequest, "invalid_request", "At least one filter parameter is required")
		return
	}

	// Delete request history
	rowsAffected, err := api.quotaManager.DeleteRequestHistory(r.Context(), filter)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "delete_failed", err.Error())
		return
	}

	// Publish cache invalidation event
	if api.cacheInvalidator != nil && filter.TokenID != "" {
		api.cacheInvalidator.PublishInvalidation(ratelimit.InvalidationToken, filter.TokenID, "api")
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"message":       "request_history_deleted",
		"rows_affected": rowsAffected,
	})
}

// Helper functions for parsing query parameters

func parseTimestamp(value string) int64 {
	if value == "" {
		return 0
	}
	if val, err := strconv.ParseInt(value, 10, 64); err == nil {
		return val
	}
	return 0
}

func parsePage(value string) int {
	if value == "" {
		return 1
	}
	if val, err := strconv.Atoi(value); err == nil && val > 0 {
		return val
	}
	return 1
}

func parsePageSize(value string) int {
	if value == "" {
		return 100
	}
	if val, err := strconv.Atoi(value); err == nil && val > 0 && val <= 1000 {
		return val
	}
	return 100
}
