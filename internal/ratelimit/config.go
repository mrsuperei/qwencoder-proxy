package ratelimit

import (
	"time"
)

// ProviderRateLimitConfig defines rate limits for a specific provider.
type ProviderRateLimitConfig struct {
	ProviderID string `json:"provider_id"`

	// RequestsPerDay limits total requests per 24-hour rolling window.
	RequestsPerDay int `json:"requests_per_day"`

	// RequestsPerMinute limits requests per 60-second sliding window.
	RequestsPerMinute int `json:"requests_per_minute"`

	// TokensPerMinute limits tokens (input + output) per 60-second sliding window.
	TokensPerMinute int `json:"tokens_per_minute"`

	// Enabled indicates if rate limiting is active for this provider.
	Enabled bool `json:"enabled"`

	// UpdatedAt timestamp of last configuration update.
	UpdatedAt time.Time `json:"updated_at"`
}

// DefaultProviderConfigs returns default configurations for all providers.
func DefaultProviderConfigs() map[string]ProviderRateLimitConfig {
	return map[string]ProviderRateLimitConfig{
		"gemini-cli": {
			ProviderID:        "gemini-cli",
			RequestsPerDay:    15000, // Conservative default
			RequestsPerMinute: 60,    // Standard rate limit
			TokensPerMinute:   32000, // ~1M tokens/day
			Enabled:           true,
			UpdatedAt:         time.Now(),
		},
		"qwen": {
			ProviderID:        "qwen",
			RequestsPerDay:    10000,
			RequestsPerMinute: 50,
			TokensPerMinute:   30000,
			Enabled:           true,
			UpdatedAt:         time.Now(),
		},
		"kiro": {
			ProviderID:        "kiro",
			RequestsPerDay:    5000,
			RequestsPerMinute: 30,
			TokensPerMinute:   20000,
			Enabled:           true,
			UpdatedAt:         time.Now(),
		},
		"antigravity": {
			ProviderID:        "antigravity",
			RequestsPerDay:    10000,
			RequestsPerMinute: 50,
			TokensPerMinute:   30000,
			Enabled:           true,
			UpdatedAt:         time.Now(),
		},
		"iflow": {
			ProviderID:        "iflow",
			RequestsPerDay:    5000,
			RequestsPerMinute: 30,
			TokensPerMinute:   20000,
			Enabled:           true,
			UpdatedAt:         time.Now(),
		},
	}
}

// UsageMetrics represents current usage metrics for a provider or token.
type UsageMetrics struct {
	// RequestsToday is the count of requests in the current 24-hour window.
	RequestsToday int `json:"requests_today"`

	// RequestsInMinute is the count of requests in the current 60-second window.
	RequestsInMinute int `json:"requests_in_minute"`

	// TokensInMinute is the count of tokens in the current 60-second window.
	TokensInMinute int `json:"tokens_in_minute"`

	// WindowStart marks the start of the current sliding window.
	WindowStart time.Time `json:"window_start"`

	// DayStart marks the start of the current 24-hour period.
	DayStart time.Time `json:"day_start"`
}

// QuotaStatus represents the remaining quota for a provider or token.
type QuotaStatus struct {
	ProviderID string `json:"provider_id"`
	TokenID    string `json:"token_id,omitempty"`

	// Remaining quotas.
	RemainingRequestsPerDay    int `json:"remaining_requests_per_day"`
	RemainingRequestsPerMinute int `json:"remaining_requests_per_minute"`
	RemainingTokensPerMinute   int `json:"remaining_tokens_per_minute"`

	// Usage percentages (0-100).
	UsagePercentagePerDay         float64 `json:"usage_percentage_per_day"`
	UsagePercentagePerMinute      float64 `json:"usage_percentage_per_minute"`
	TokenUsagePercentagePerMinute float64 `json:"token_usage_percentage_per_minute"`

	// Time until quota reset.
	SecondsUntilDayReset    int64 `json:"seconds_until_day_reset"`
	SecondsUntilMinuteReset int64 `json:"seconds_until_minute_reset"`

	// IsLimited indicates if any quota is currently exceeded.
	IsLimited bool `json:"is_limited"`

	// LimitReasons lists which quotas are exceeded.
	LimitReasons []string `json:"limit_reasons"`
}

// RateLimitConfig is the main configuration for rate limiting.
type RateLimitConfig struct {
	// Async usage recording configuration
	AsyncUsageRecorder *AsyncUsageRecorderConfig `json:"async_usage_recorder"`

	// Cache configuration
	EnableCache           bool          `json:"enable_cache" default:"true"`
	CacheProviderTTL      time.Duration `json:"cache_provider_ttl" default:"5s"`
	CacheTokenTTL         time.Duration `json:"cache_token_ttl" default:"5s"`
	CacheMaxProviders     int           `json:"cache_max_providers" default:"100"`
	CacheMaxTokens        int           `json:"cache_max_tokens" default:"1000"`
	CacheRefreshBeforeTTL time.Duration `json:"cache_refresh_before_ttl" default:"1s"`
}
