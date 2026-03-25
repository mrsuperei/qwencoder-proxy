// Package ratelimit provides provider-aware rate limiting for the qwencoder-proxy.
// It enforces internal quotas without exposing rate limit information to clients.
package ratelimit

import (
	"context"
	"database/sql"
	"time"
)

// RateLimiter defines the interface for rate limiting operations.
type RateLimiter interface {
	// CheckQuota checks if a request is allowed based on current usage.
	// Returns (allowed, quotaStatus, error).
	CheckQuota(ctx context.Context, providerID string, tokenID string, estimatedInputTokens int) (bool, *QuotaStatus, error)

	// RecordUsage records usage after a successful request.
	RecordUsage(ctx context.Context, providerID string, tokenID string, inputTokens int, outputTokens int) error

	// GetQuotaStatus retrieves current quota status.
	GetQuotaStatus(ctx context.Context, providerID string, tokenID string) (*QuotaStatus, error)

	// ResetUsage resets usage metrics.
	ResetUsage(ctx context.Context, providerID string, tokenID string) error

	// Close releases resources.
	Close() error
}

// ModelUsageMetrics represents usage metrics for a specific model.
type ModelUsageMetrics struct {
	ProviderID string `json:"provider_id"`
	TokenID    string `json:"token_id,omitempty"`
	Model      string `json:"model"`

	// Request counts
	RequestCount int `json:"request_count"`

	// Token counts (detailed)
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`

	// Time windows
	WindowStart time.Time `json:"window_start"`
	DayStart    time.Time `json:"day_start"`
}

// RequestHistoryRecord represents a single request history record.
type RequestHistoryRecord struct {
	ID           string `json:"id"`
	TokenID      string `json:"token_id"`
	Model        string `json:"model"`
	RequestCount int    `json:"request_count"`
	InputTokens  int    `json:"input_tokens"`
	OutputTokens int    `json:"output_tokens"`
	TotalTokens  int    `json:"total_tokens"`
	Timestamp    int64  `json:"timestamp"`
	CreatedAt    int64  `json:"created_at"`
}

// RequestHistoryFilter represents filters for request history queries.
type RequestHistoryFilter struct {
	TokenID   string `json:"token_id,omitempty"`
	Model     string `json:"model,omitempty"`
	StartTime int64  `json:"start_time,omitempty"`
	EndTime   int64  `json:"end_time,omitempty"`
	MinTokens int    `json:"min_tokens,omitempty"`
	MaxTokens int    `json:"max_tokens,omitempty"`
}

// RequestHistoryResponse represents the paginated response for request history.
type RequestHistoryResponse struct {
	Records     []RequestHistoryRecord `json:"records"`
	TotalCount  int                    `json:"total_count"`
	Page        int                    `json:"page"`
	PageSize    int                    `json:"page_size"`
	TotalPages  int                    `json:"total_pages"`
	HasNext     bool                   `json:"has_next"`
	HasPrevious bool                   `json:"has_previous"`
}

// RequestHistorySummary represents aggregated statistics for request history.
type RequestHistorySummary struct {
	TokenID       string  `json:"token_id,omitempty"`
	Model         string  `json:"model,omitempty"`
	TotalRequests int     `json:"total_requests"`
	TotalInput    int     `json:"total_input_tokens"`
	TotalOutput   int     `json:"total_output_tokens"`
	TotalTokens   int     `json:"total_tokens"`
	AverageTokens float64 `json:"average_tokens"`
	FirstRequest  int64   `json:"first_request"`
	LastRequest   int64   `json:"last_request"`
}

// UsageTracker defines the interface for usage tracking operations.
type UsageTracker interface {
	// RecordUsage records usage for a provider and optionally a token.
	RecordUsage(ctx context.Context, providerID string, tokenID string, requestCount int, tokenCount int) error

	// GetProviderUsage retrieves current usage metrics for a provider.
	GetProviderUsage(ctx context.Context, providerID string) (*UsageMetrics, error)

	// GetTokenUsage retrieves current usage metrics for a token.
	GetTokenUsage(ctx context.Context, providerID string, tokenID string) (*UsageMetrics, error)

	// GetAllProviderUsage retrieves usage metrics for all providers.
	GetAllProviderUsage(ctx context.Context) (map[string]*UsageMetrics, error)

	// ResetUsage resets usage metrics for a provider or token.
	ResetUsage(ctx context.Context, providerID string, tokenID string) error

	// CleanupOrphanedUsageRecords removes usage records for tokens that no longer exist.
	CleanupOrphanedUsageRecords(ctx context.Context) error

	// GetDB returns the underlying database connection.
	GetDB() *sql.DB

	// Close releases resources.
	Close() error

	// Model usage tracking methods

	// RecordModelUsage records usage for a specific model.
	RecordModelUsage(ctx context.Context, providerID string, tokenID string, model string,
		inputTokens int, outputTokens int) error

	// GetModelUsage retrieves usage metrics for a specific model.
	GetModelUsage(ctx context.Context, providerID string, tokenID string, model string) (*ModelUsageMetrics, error)

	// GetAllModelUsage retrieves all model usage for a provider.
	GetAllModelUsage(ctx context.Context, providerID string) (map[string]*ModelUsageMetrics, error)

	// GetTokenModelUsage retrieves all model usage for a token.
	GetTokenModelUsage(ctx context.Context, tokenID string) (map[string]*ModelUsageMetrics, error)

	// GetProviderModelUsage retrieves all model usage for a provider.
	GetProviderModelUsage(ctx context.Context, providerID string) (map[string]*ModelUsageMetrics, error)

	// ResetModelUsage resets usage metrics for a specific model.
	ResetModelUsage(ctx context.Context, providerID string, tokenID string, model string) error

	// Request history tracking methods

	// GetRequestHistory retrieves request history with optional filters and pagination.
	// Returns paginated results with total count.
	GetRequestHistory(ctx context.Context, filter *RequestHistoryFilter, page int, pageSize int) (*RequestHistoryResponse, error)

	// GetRequestHistoryByToken retrieves request history for a specific token.
	GetRequestHistoryByToken(ctx context.Context, tokenID string, filter *RequestHistoryFilter, page int, pageSize int) (*RequestHistoryResponse, error)

	// GetRequestHistoryByModel retrieves request history for a specific model.
	GetRequestHistoryByModel(ctx context.Context, model string, filter *RequestHistoryFilter, page int, pageSize int) (*RequestHistoryResponse, error)

	// GetRequestHistorySummary returns aggregated statistics for request history.
	GetRequestHistorySummary(ctx context.Context, tokenID string, model string, startTime int64, endTime int64) (*RequestHistorySummary, error)

	// DeleteRequestHistory deletes request history records matching the filter.
	DeleteRequestHistory(ctx context.Context, filter *RequestHistoryFilter) (int64, error)
}

// TokenCounter defines the interface for token counting operations.
type TokenCounter interface {
	// ExtractTokensFromResponse extracts token counts from provider response.
	// This is the primary method - uses provider-reported data for accuracy.
	ExtractTokensFromResponse(response map[string]interface{}) (inputTokens int, outputTokens int, totalTokens int, err error)

	// EstimateInputTokens estimates input tokens from request (fallback only).
	// Used for pre-request quota checking (conservative estimate).
	EstimateInputTokens(openaiReq map[string]interface{}) (int, error)
}
