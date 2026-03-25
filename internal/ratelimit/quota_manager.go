package ratelimit

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/sunbankio/qwencoder-proxy/internal/logging"
)

// QuotaManager enforces rate limits based on configuration and usage.
type QuotaManager struct {
	configs       map[string]ProviderRateLimitConfig
	usageTracker  UsageTracker
	logger        logging.Logger
	mu            sync.RWMutex
	tokenCounter  TokenCounter
	tokenSelector *RateAwareTokenSelector
	db            *sql.DB
	asyncRecorder *AsyncUsageRecorder
	cachedTracker UsageTracker // Cached usage tracker (optional)
	invalidator   *CacheInvalidator
}

// NewQuotaManager creates a new quota manager with the default adaptive strategy.
func NewQuotaManager(usageTracker UsageTracker, logger logging.Logger, db *sql.DB, asyncConfig *AsyncUsageRecorderConfig, rateLimitConfig *RateLimitConfig, invalidator *CacheInvalidator) *QuotaManager {
	qm := &QuotaManager{
		configs:       DefaultProviderConfigs(),
		usageTracker:  usageTracker,
		logger:        logger,
		tokenCounter:  NewTokenCounter(logger),
		tokenSelector: NewRateAwareTokenSelector(db, nil, logger, nil),
		db:            db,
		invalidator:   invalidator,
	}

	// Initialize async recorder if config provided
	if asyncConfig != nil {
		qm.asyncRecorder = NewAsyncUsageRecorder(usageTracker, logger, asyncConfig)
	}

	// Initialize cached tracker if config provided and enabled
	if rateLimitConfig != nil && rateLimitConfig.EnableCache {
		cacheConfig := &CacheConfig{
			ProviderMetricsTTL:  rateLimitConfig.CacheProviderTTL,
			TokenMetricsTTL:     rateLimitConfig.CacheTokenTTL,
			MaxProviderEntries:  rateLimitConfig.CacheMaxProviders,
			MaxTokenEntries:     rateLimitConfig.CacheMaxTokens,
			RefreshBeforeExpiry: rateLimitConfig.CacheRefreshBeforeTTL,
		}
		qm.cachedTracker = NewCachedUsageTracker(usageTracker, cacheConfig, logger, invalidator)
		logger.InfoLog("[QuotaManager] Cached usage tracker enabled with TTL=%v", cacheConfig.ProviderMetricsTTL)
	} else {
		qm.cachedTracker = usageTracker
		logger.InfoLog("[QuotaManager] Cached usage tracker disabled, using direct database access")
	}

	// Set default adaptive strategy with proper quotaManager reference
	_ = qm.SetLoadBalancingStrategy("adaptive")

	return qm
}

// SetLoadBalancingStrategy changes the load balancing strategy.
func (qm *QuotaManager) SetLoadBalancingStrategy(strategyName string) error {
	var strategy LoadBalancingStrategy

	switch strategyName {
	case "weighted_round_robin":
		strategy = NewWeightedRoundRobinSelector(qm.db, qm, qm.logger)
	case "least_connections":
		strategy = NewLeastConnectionsSelector(qm.db, qm, qm.logger)
	case "adaptive":
		strategy = NewAdaptiveSelector(qm.db, qm, qm.logger)
	default:
		return fmt.Errorf("unknown load balancing strategy: %s", strategyName)
	}

	qm.tokenSelector.SetStrategy(strategy)
	qm.logger.InfoLog("[QuotaManager] Load balancing strategy changed to: %s", strategyName)

	return nil
}

// GetLoadBalancingStrategy returns the current load balancing strategy name.
func (qm *QuotaManager) GetLoadBalancingStrategy() string {
	return qm.tokenSelector.GetStrategy()
}

// CheckQuota checks if a request is allowed based on current usage.
// Returns (allowed, quotaStatus, error).
func (qm *QuotaManager) CheckQuota(ctx context.Context, providerID string, tokenID string, estimatedInputTokens int) (bool, *QuotaStatus, error) {
	qm.mu.RLock()
	config, exists := qm.configs[providerID]
	qm.mu.RUnlock()

	if !exists {
		return false, nil, fmt.Errorf("no rate limit configuration for provider: %s", providerID)
	}

	// If rate limiting is disabled, allow all requests.
	if !config.Enabled {
		return true, &QuotaStatus{IsLimited: false}, nil
	}

	// Get current usage metrics.
	var providerMetrics *UsageMetrics
	var tokenMetrics *UsageMetrics
	var err error

	providerMetrics, err = qm.usageTracker.GetProviderUsage(ctx, providerID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, nil, fmt.Errorf("failed to get provider usage: %w", err)
	}

	// If no usage record exists, assume zero usage.
	if providerMetrics == nil {
		now := time.Now()
		dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		windowStart := now.Add(-time.Minute)
		providerMetrics = &UsageMetrics{
			RequestsToday:    0,
			RequestsInMinute: 0,
			TokensInMinute:   0,
			WindowStart:      windowStart,
			DayStart:         dayStart,
		}
	}

	if tokenID != "" {
		tokenMetrics, err = qm.usageTracker.GetTokenUsage(ctx, providerID, tokenID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return false, nil, fmt.Errorf("failed to get token usage: %w", err)
		}

		// If no usage record exists, assume zero usage.
		if tokenMetrics == nil {
			now := time.Now()
			dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
			windowStart := now.Add(-time.Minute)
			tokenMetrics = &UsageMetrics{
				RequestsToday:    0,
				RequestsInMinute: 0,
				TokensInMinute:   0,
				WindowStart:      windowStart,
				DayStart:         dayStart,
			}
		}
	}

	// Build quota status.
	status := qm.buildQuotaStatus(providerID, tokenID, config, providerMetrics, tokenMetrics, estimatedInputTokens)

	// Check if any quota is exceeded.
	if status.IsLimited {
		qm.logger.WarnLog("[QuotaManager] Rate limit exceeded for provider %s: %v", providerID, status.LimitReasons)
		return false, status, nil
	}

	return true, status, nil
}

// buildQuotaStatus constructs quota status from metrics and configuration.
func (qm *QuotaManager) buildQuotaStatus(providerID string, tokenID string, config ProviderRateLimitConfig, providerMetrics *UsageMetrics, tokenMetrics *UsageMetrics, estimatedInputTokens int) *QuotaStatus {
	now := time.Now()

	// Use token metrics if available, otherwise use provider metrics.
	metrics := providerMetrics
	if tokenMetrics != nil {
		metrics = tokenMetrics
	}

	status := &QuotaStatus{
		ProviderID:   providerID,
		TokenID:      tokenID,
		IsLimited:    false,
		LimitReasons: []string{},
	}

	// Calculate remaining quotas.
	status.RemainingRequestsPerDay = config.RequestsPerDay - metrics.RequestsToday
	status.RemainingRequestsPerMinute = config.RequestsPerMinute - metrics.RequestsInMinute

	// For tokens, include estimated input tokens.
	status.RemainingTokensPerMinute = config.TokensPerMinute - metrics.TokensInMinute - estimatedInputTokens

	// Calculate usage percentages.
	if config.RequestsPerDay > 0 {
		status.UsagePercentagePerDay = float64(metrics.RequestsToday) / float64(config.RequestsPerDay) * 100
	}
	if config.RequestsPerMinute > 0 {
		status.UsagePercentagePerMinute = float64(metrics.RequestsInMinute) / float64(config.RequestsPerMinute) * 100
	}
	if config.TokensPerMinute > 0 {
		status.TokenUsagePercentagePerMinute = float64(metrics.TokensInMinute+estimatedInputTokens) / float64(config.TokensPerMinute) * 100
	}

	// Calculate time until reset.
	nextDayStart := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, now.Location())
	status.SecondsUntilDayReset = int64(nextDayStart.Sub(now).Seconds())

	nextMinuteStart := metrics.WindowStart.Add(time.Minute)
	status.SecondsUntilMinuteReset = int64(nextMinuteStart.Sub(now).Seconds())

	// Check if quotas are exceeded.
	if status.RemainingRequestsPerDay <= 0 {
		status.IsLimited = true
		status.LimitReasons = append(status.LimitReasons, "requests_per_day")
	}
	if status.RemainingRequestsPerMinute <= 0 {
		status.IsLimited = true
		status.LimitReasons = append(status.LimitReasons, "requests_per_minute")
	}
	if status.RemainingTokensPerMinute <= 0 {
		status.IsLimited = true
		status.LimitReasons = append(status.LimitReasons, "tokens_per_minute")
	}

	return status
}

// RecordUsage records usage after a successful request.
// DEPRECATED: Use RecordModelUsage instead for complete tracking.
// This method is kept for backward compatibility but may be removed in future versions.
func (qm *QuotaManager) RecordUsage(ctx context.Context, providerID string, tokenID string, inputTokens int, outputTokens int) error {
	qm.logger.WarnLog("[QuotaManager] RecordUsage is deprecated, use RecordModelUsage instead")

	// Use async recorder if enabled
	if qm.asyncRecorder != nil && qm.asyncRecorder.IsEnabled() {
		if err := qm.asyncRecorder.RecordUsageAsync(providerID, tokenID, 1, inputTokens, outputTokens); err != nil {
			return fmt.Errorf("failed to queue async usage recording: %w", err)
		}
		qm.logger.DebugLog("[QuotaManager] Queued async usage recording for %s: %d tokens (input: %d, output: %d)",
			providerID, inputTokens+outputTokens, inputTokens, outputTokens)
		return nil
	}

	// Fallback to synchronous recording
	totalTokens := inputTokens + outputTokens
	if err := qm.usageTracker.RecordUsage(ctx, providerID, tokenID, 1, totalTokens); err != nil {
		return fmt.Errorf("failed to record usage: %w", err)
	}

	qm.logger.DebugLog("[QuotaManager] Recorded usage for %s: %d tokens (input: %d, output: %d)",
		providerID, totalTokens, inputTokens, outputTokens)

	return nil
}

// RecordModelUsage records usage for a specific model after a successful request.
func (qm *QuotaManager) RecordModelUsage(ctx context.Context, providerID string, tokenID string, model string, inputTokens int, outputTokens int) error {
	// Use async recorder if enabled
	if qm.asyncRecorder != nil && qm.asyncRecorder.IsEnabled() {
		if err := qm.asyncRecorder.RecordModelUsageAsync(providerID, tokenID, model, inputTokens, outputTokens); err != nil {
			return fmt.Errorf("failed to queue async model usage recording: %w", err)
		}
		totalTokens := inputTokens + outputTokens
		qm.logger.DebugLog("[QuotaManager] Queued async model usage recording for %s/%s: %d tokens (input: %d, output: %d)",
			providerID, model, totalTokens, inputTokens, outputTokens)
		return nil
	}

	// Fallback to synchronous recording
	if err := qm.usageTracker.RecordModelUsage(ctx, providerID, tokenID, model, inputTokens, outputTokens); err != nil {
		return fmt.Errorf("failed to record model usage: %w", err)
	}

	totalTokens := inputTokens + outputTokens
	qm.logger.DebugLog("[QuotaManager] Recorded model usage for %s/%s: %d tokens (input: %d, output: %d)",
		providerID, model, totalTokens, inputTokens, outputTokens)

	return nil
}

// UpdateConfig updates rate limit configuration for a provider.
func (qm *QuotaManager) UpdateConfig(providerID string, config ProviderRateLimitConfig) error {
	qm.mu.Lock()
	defer qm.mu.Unlock()

	config.ProviderID = providerID
	config.UpdatedAt = time.Now()

	qm.configs[providerID] = config

	qm.logger.InfoLog("[QuotaManager] Updated rate limit config for %s: RPD=%d, RPM=%d, TPM=%d, Enabled=%v",
		providerID, config.RequestsPerDay, config.RequestsPerMinute, config.TokensPerMinute, config.Enabled)

	return nil
}

// GetConfig retrieves rate limit configuration for a provider.
func (qm *QuotaManager) GetConfig(providerID string) (ProviderRateLimitConfig, error) {
	qm.mu.RLock()
	defer qm.mu.RUnlock()

	config, exists := qm.configs[providerID]
	if !exists {
		return ProviderRateLimitConfig{}, fmt.Errorf("no configuration for provider: %s", providerID)
	}

	return config, nil
}

// GetAllConfigs retrieves all rate limit configurations.
func (qm *QuotaManager) GetAllConfigs() map[string]ProviderRateLimitConfig {
	qm.mu.RLock()
	defer qm.mu.RUnlock()

	// Return a copy to prevent external modifications.
	result := make(map[string]ProviderRateLimitConfig)
	for k, v := range qm.configs {
		result[k] = v
	}

	return result
}

// GetAllProviderUsage retrieves usage metrics for all providers.
func (qm *QuotaManager) GetAllProviderUsage(ctx context.Context) (map[string]*UsageMetrics, error) {
	return qm.usageTracker.GetAllProviderUsage(ctx)
}

// GetQuotaStatus retrieves current quota status for a provider.
func (qm *QuotaManager) GetQuotaStatus(ctx context.Context, providerID string, tokenID string) (*QuotaStatus, error) {
	qm.mu.RLock()
	config, exists := qm.configs[providerID]
	qm.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("no rate limit configuration for provider: %s", providerID)
	}

	providerMetrics, err := qm.usageTracker.GetProviderUsage(ctx, providerID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("failed to get provider usage: %w", err)
	}

	if providerMetrics == nil {
		now := time.Now()
		dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		windowStart := now.Add(-time.Minute)
		providerMetrics = &UsageMetrics{
			RequestsToday:    0,
			RequestsInMinute: 0,
			TokensInMinute:   0,
			WindowStart:      windowStart,
			DayStart:         dayStart,
		}
	}

	var tokenMetrics *UsageMetrics
	if tokenID != "" {
		tokenMetrics, err = qm.usageTracker.GetTokenUsage(ctx, providerID, tokenID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("failed to get token usage: %w", err)
		}
	}

	status := qm.buildQuotaStatus(providerID, tokenID, config, providerMetrics, tokenMetrics, 0)

	return status, nil
}

// ResetUsage resets usage metrics for a provider or token.
func (qm *QuotaManager) ResetUsage(ctx context.Context, providerID string, tokenID string) error {
	if err := qm.usageTracker.ResetUsage(ctx, providerID, tokenID); err != nil {
		return fmt.Errorf("failed to reset usage: %w", err)
	}

	qm.logger.InfoLog("[QuotaManager] Reset usage for %s (token: %s)", providerID, tokenID)
	return nil
}

// EstimateInputTokens estimates input tokens from a request.
// Used for pre-request quota checking (conservative estimate).
func (qm *QuotaManager) EstimateInputTokens(openaiReq map[string]interface{}) (int, error) {
	return qm.tokenCounter.EstimateInputTokens(openaiReq)
}

// ExtractTokensFromResponse extracts token counts from provider response.
// This is the primary method for accurate token counting.
func (qm *QuotaManager) ExtractTokensFromResponse(response map[string]interface{}) (inputTokens int, outputTokens int, totalTokens int, err error) {
	return qm.tokenCounter.ExtractTokensFromResponse(response)
}

// GetModelUsage retrieves usage metrics for a specific model.
func (qm *QuotaManager) GetModelUsage(ctx context.Context, providerID string, tokenID string, model string) (*ModelUsageMetrics, error) {
	return qm.usageTracker.GetModelUsage(ctx, providerID, tokenID, model)
}

// GetAllModelUsage retrieves all model usage for a provider.
func (qm *QuotaManager) GetAllModelUsage(ctx context.Context, providerID string) (map[string]*ModelUsageMetrics, error) {
	return qm.usageTracker.GetAllModelUsage(ctx, providerID)
}

// GetTokenModelUsage retrieves all model usage for a token.
func (qm *QuotaManager) GetTokenModelUsage(ctx context.Context, tokenID string) (map[string]*ModelUsageMetrics, error) {
	return qm.usageTracker.GetTokenModelUsage(ctx, tokenID)
}

// GetProviderModelUsage retrieves all model usage for a provider.
func (qm *QuotaManager) GetProviderModelUsage(ctx context.Context, providerID string) (map[string]*ModelUsageMetrics, error) {
	return qm.usageTracker.GetProviderModelUsage(ctx, providerID)
}

// ResetModelUsage resets usage metrics for a specific model.
func (qm *QuotaManager) ResetModelUsage(ctx context.Context, providerID string, tokenID string, model string) error {
	if err := qm.usageTracker.ResetModelUsage(ctx, providerID, tokenID, model); err != nil {
		return fmt.Errorf("failed to reset model usage: %w", err)
	}

	qm.logger.InfoLog("[QuotaManager] Reset model usage for %s/%s/%s", providerID, tokenID, model)
	return nil
}

// GetRequestHistory retrieves request history with optional filters and pagination.
func (qm *QuotaManager) GetRequestHistory(ctx context.Context, filter *RequestHistoryFilter, page int, pageSize int) (*RequestHistoryResponse, error) {
	return qm.usageTracker.GetRequestHistory(ctx, filter, page, pageSize)
}

// GetRequestHistoryByToken retrieves request history for a specific token.
func (qm *QuotaManager) GetRequestHistoryByToken(ctx context.Context, tokenID string, filter *RequestHistoryFilter, page int, pageSize int) (*RequestHistoryResponse, error) {
	return qm.usageTracker.GetRequestHistoryByToken(ctx, tokenID, filter, page, pageSize)
}

// GetRequestHistoryByModel retrieves request history for a specific model.
func (qm *QuotaManager) GetRequestHistoryByModel(ctx context.Context, model string, filter *RequestHistoryFilter, page int, pageSize int) (*RequestHistoryResponse, error) {
	return qm.usageTracker.GetRequestHistoryByModel(ctx, model, filter, page, pageSize)
}

// GetRequestHistorySummary returns aggregated statistics for request history.
func (qm *QuotaManager) GetRequestHistorySummary(ctx context.Context, tokenID string, model string, startTime int64, endTime int64) (*RequestHistorySummary, error) {
	return qm.usageTracker.GetRequestHistorySummary(ctx, tokenID, model, startTime, endTime)
}

// DeleteRequestHistory deletes request history records matching the filter.
func (qm *QuotaManager) DeleteRequestHistory(ctx context.Context, filter *RequestHistoryFilter) (int64, error) {
	return qm.usageTracker.DeleteRequestHistory(ctx, filter)
}

// GetDB returns the underlying database connection from the usage tracker.
func (qm *QuotaManager) GetDB() *sql.DB {
	return qm.usageTracker.GetDB()
}

// GetTokenSelector returns the token selector
func (qm *QuotaManager) GetTokenSelector() *RateAwareTokenSelector {
	return qm.tokenSelector
}

// Close releases resources.
func (qm *QuotaManager) Close() error {
	// Close async recorder first to drain queue
	if qm.asyncRecorder != nil {
		if err := qm.asyncRecorder.Close(); err != nil {
			qm.logger.ErrorLog("[QuotaManager] Failed to close async recorder: %v", err)
		}
	}

	return qm.usageTracker.Close()
}

// GetAsyncRecorderMetrics returns metrics from the async recorder.
func (qm *QuotaManager) GetAsyncRecorderMetrics() *AsyncUsageRecorderMetrics {
	if qm.asyncRecorder == nil {
		return &AsyncUsageRecorderMetrics{
			Enabled: false,
		}
	}
	metrics := qm.asyncRecorder.GetMetrics()
	return &metrics
}

// RecordError records an error for a specific token.
// This updates the token's error tracking and health status.
func (qm *QuotaManager) RecordError(ctx context.Context, providerID string, tokenID string, errorType string, errorMessage string, statusCode int) error {
	qm.logger.WarnLog("[QuotaManager] Recording error - Provider: %s, Token: %s, Type: %s, Status: %d, Message: %s",
		providerID, tokenID, errorType, statusCode, errorMessage)

	// Get current token state
	var errorCount int
	var healthScore float64
	var healthy int

	err := qm.db.QueryRowContext(ctx, `
		SELECT error_count, health_score, healthy
		FROM tokens
		WHERE id = ? AND provider_id = ?
	`, tokenID, providerID).Scan(&errorCount, &healthScore, &healthy)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			qm.logger.ErrorLog("[QuotaManager] Token %s not found, cannot record error", tokenID)
			return fmt.Errorf("token not found: %s", tokenID)
		}
		return fmt.Errorf("failed to query token: %w", err)
	}

	// Calculate new error count and health score
	newErrorCount := errorCount + 1
	newHealthScore := qm.calculateHealthScore(newErrorCount, healthScore, errorType, statusCode)
	newHealthy := qm.determineHealthyStatus(newHealthScore, errorType, statusCode)

	// Update token in database
	now := time.Now().UnixMilli()
	_, err = qm.db.ExecContext(ctx, `
		UPDATE tokens
		SET error_count = ?,
		    last_error = ?,
		    health_score = ?,
		    healthy = ?,
		    last_used = ?
		WHERE id = ? AND provider_id = ?
	`, newErrorCount, errorMessage, newHealthScore, newHealthy, now, tokenID, providerID)

	if err != nil {
		return fmt.Errorf("failed to update token: %w", err)
	}

	qm.logger.InfoLog("[QuotaManager] Recorded error for token %s - Count: %d, Health: %.2f, Healthy: %d",
		tokenID, newErrorCount, newHealthScore, newHealthy)

	// Invalidate cache for this token
	if qm.invalidator != nil {
		qm.invalidator.PublishInvalidation(InvalidationToken, tokenID, "error_recording")
	}

	return nil
}

// calculateHealthScore calculates a new health score based on error history.
// Returns a score between 0.0 and 1.0, where 1.0 is healthy.
func (qm *QuotaManager) calculateHealthScore(errorCount int, currentHealthScore float64, errorType string, statusCode int) float64 {
	// Base penalty for any error
	penalty := 0.1

	// Additional penalty based on error type
	switch errorType {
	case "rate_limit":
		penalty = 0.3 // Rate limits are more severe
	case "server_error":
		penalty = 0.2 // Server errors are moderately severe
	case "timeout":
		penalty = 0.25 // Timeouts are concerning
	case "authentication":
		penalty = 0.5 // Auth errors are very severe
	}

	// Additional penalty based on status code
	if statusCode >= 500 && statusCode < 600 {
		penalty += 0.1 // 5xx errors are worse
	}

	// Apply the penalty
	newScore := currentHealthScore - penalty

	// Ensure score stays within bounds
	if newScore < 0.0 {
		newScore = 0.0
	}
	if newScore > 1.0 {
		newScore = 1.0
	}

	return newScore
}

// determineHealthyStatus determines if a token should be marked as unhealthy.
func (qm *QuotaManager) determineHealthyStatus(healthScore float64, errorType string, statusCode int) int {
	// Mark as unhealthy if health score is too low
	if healthScore < 0.3 {
		return 0
	}

	// Mark as unhealthy for certain error types
	if errorType == "authentication" {
		return 0
	}

	return 1
}

// GetTokenErrors retrieves error information for a token.
func (qm *QuotaManager) GetTokenErrors(ctx context.Context, providerID string, tokenID string) (*TokenErrorInfo, error) {
	var errorCount int
	var lastError sql.NullString
	var healthScore float64
	var healthy int

	err := qm.db.QueryRowContext(ctx, `
		SELECT error_count, last_error, health_score, healthy
		FROM tokens
		WHERE id = ? AND provider_id = ?
	`, tokenID, providerID).Scan(&errorCount, &lastError, &healthScore, &healthy)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("token not found: %s", tokenID)
		}
		return nil, fmt.Errorf("failed to query token: %w", err)
	}

	var lastErrorStr string
	if lastError.Valid {
		lastErrorStr = lastError.String
	}

	return &TokenErrorInfo{
		TokenID:     tokenID,
		ProviderID:  providerID,
		ErrorCount:  errorCount,
		LastError:   lastErrorStr,
		HealthScore: healthScore,
		Healthy:     healthy == 1,
	}, nil
}

// GetAllTokenErrors retrieves error information for all tokens.
func (qm *QuotaManager) GetAllTokenErrors(ctx context.Context, providerID string) ([]*TokenErrorInfo, error) {
	query := `
		SELECT id, provider_id, error_count, last_error, health_score, healthy
		FROM tokens
		WHERE provider_id = ?
		ORDER BY error_count DESC
	`

	rows, err := qm.db.QueryContext(ctx, query, providerID)
	if err != nil {
		return nil, fmt.Errorf("failed to query tokens: %w", err)
	}
	defer rows.Close()

	var errors []*TokenErrorInfo
	for rows.Next() {
		var info TokenErrorInfo
		var lastError sql.NullString

		err := rows.Scan(
			&info.TokenID,
			&info.ProviderID,
			&info.ErrorCount,
			&lastError,
			&info.HealthScore,
			&info.Healthy,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan token: %w", err)
		}

		if lastError.Valid {
			info.LastError = lastError.String
		}

		errors = append(errors, &info)
	}

	return errors, nil
}

// ResetTokenErrors resets error tracking for a token.
func (qm *QuotaManager) ResetTokenErrors(ctx context.Context, providerID string, tokenID string) error {
	_, err := qm.db.ExecContext(ctx, `
		UPDATE tokens
		SET error_count = 0,
		    last_error = NULL,
		    health_score = 1.0,
		    healthy = 1
		WHERE id = ? AND provider_id = ?
	`, tokenID, providerID)

	if err != nil {
		return fmt.Errorf("failed to reset token errors: %w", err)
	}

	qm.logger.InfoLog("[QuotaManager] Reset errors for token %s", tokenID)

	// Invalidate cache
	if qm.invalidator != nil {
		qm.invalidator.PublishInvalidation(InvalidationToken, tokenID, "error_reset")
	}

	return nil
}

// RecoverTokenHealth attempts to recover a token's health status.
// This can be called periodically or manually to recover tokens that were marked unhealthy.
func (qm *QuotaManager) RecoverTokenHealth(ctx context.Context, providerID string, tokenID string) error {
	// Get current token state
	var errorCount int
	var healthScore float64

	err := qm.db.QueryRowContext(ctx, `
		SELECT error_count, health_score
		FROM tokens
		WHERE id = ? AND provider_id = ?
	`, tokenID, providerID).Scan(&errorCount, &healthScore)

	if err != nil {
		return fmt.Errorf("failed to query token: %w", err)
	}

	// Only recover if health score is low but not too many errors
	if healthScore < 0.5 && errorCount < 10 {
		// Boost health score
		newHealthScore := healthScore + 0.3
		if newHealthScore > 1.0 {
			newHealthScore = 1.0
		}

		// Mark as healthy
		_, err = qm.db.ExecContext(ctx, `
			UPDATE tokens
			SET health_score = ?,
			    healthy = 1
			WHERE id = ? AND provider_id = ?
		`, newHealthScore, tokenID, providerID)

		if err != nil {
			return fmt.Errorf("failed to recover token: %w", err)
		}

		qm.logger.InfoLog("[QuotaManager] Recovered token %s - New health score: %.2f", tokenID, newHealthScore)

		// Invalidate cache
		if qm.invalidator != nil {
			qm.invalidator.PublishInvalidation(InvalidationToken, tokenID, "health_recovery")
		}

		return nil
	}

	return fmt.Errorf("token cannot be recovered - too many errors or already healthy")
}

// RecoverAllUnhealthyTokens attempts to recover all unhealthy tokens.
func (qm *QuotaManager) RecoverAllUnhealthyTokens(ctx context.Context, providerID string) (int, error) {
	// Find all unhealthy tokens with low error counts
	query := `
		SELECT id, provider_id
		FROM tokens
		WHERE provider_id = ? AND healthy = 0 AND error_count < 10
	`

	rows, err := qm.db.QueryContext(ctx, query, providerID)
	if err != nil {
		return 0, fmt.Errorf("failed to query unhealthy tokens: %w", err)
	}
	defer rows.Close()

	recovered := 0
	for rows.Next() {
		var tokenID, pID string
		if err := rows.Scan(&tokenID, &pID); err != nil {
			qm.logger.WarnLog("[QuotaManager] Failed to scan token: %v", err)
			continue
		}

		if err := qm.RecoverTokenHealth(ctx, pID, tokenID); err == nil {
			recovered++
		}
	}

	qm.logger.InfoLog("[QuotaManager] Recovered %d unhealthy tokens for provider %s", recovered, providerID)
	return recovered, nil
}

// GetAllProviders returns all provider IDs.
func (qm *QuotaManager) GetAllProviders(ctx context.Context) ([]string, error) {
	query := `SELECT DISTINCT provider_id FROM tokens`

	rows, err := qm.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query providers: %w", err)
	}
	defer rows.Close()

	var providers []string
	for rows.Next() {
		var providerID string
		if err := rows.Scan(&providerID); err != nil {
			return nil, fmt.Errorf("failed to scan provider: %w", err)
		}
		providers = append(providers, providerID)
	}

	return providers, nil
}

// StartHealthRecovery starts a background goroutine that periodically recovers unhealthy tokens.
func (qm *QuotaManager) StartHealthRecovery(ctx context.Context, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				qm.logger.InfoLog("[QuotaManager] Health recovery stopped")
				return
			case <-ticker.C:
				// Get all providers
				providers, err := qm.GetAllProviders(ctx)
				if err != nil {
					qm.logger.ErrorLog("[QuotaManager] Failed to get providers: %v", err)
					continue
				}

				// Recover unhealthy tokens for each provider
				for _, providerID := range providers {
					recovered, err := qm.RecoverAllUnhealthyTokens(ctx, providerID)
					if err != nil {
						qm.logger.ErrorLog("[QuotaManager] Failed to recover tokens for %s: %v", providerID, err)
					} else if recovered > 0 {
						qm.logger.InfoLog("[QuotaManager] Recovered %d tokens for %s", recovered, providerID)
					}
				}
			}
		}
	}()
}
