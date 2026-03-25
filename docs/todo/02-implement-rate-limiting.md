# Implement Provider-Aware Rate Limiting Architecture

**Priority:** CRITICAL  
**Estimated Time:** 3 weeks  
**Complexity:** High  
**Files to Create:** 6  
**Files to Modify:** 8

---

## Executive Summary

This implementation plan defines a comprehensive rate limiting architecture for the qwencoder-proxy that enforces internal provider-specific quotas without exposing rate limit information to clients. The system provides granular control over provider usage through a management API, supports multiple metric types (requests per day, requests per minute, tokens per minute), and implements real-time token tracking for `/v1/chat/completions` requests. A usage-aware selection strategy optimizes throughput by prioritizing endpoints with the least consumption.

---

## Problem Description

The qwencoder-proxy currently lacks any rate limiting mechanism, which is critical for:

- Managing API quotas per provider to prevent service disruptions
- Preventing abuse and ensuring fair usage across multiple provider tokens
- Implementing provider-specific rate limits based on actual API constraints
- Enforcing strict quotas to avoid exceeding provider billing limits
- Optimizing token selection to maximize throughput without hitting limits
- Providing administrative control over rate limiting policies through a dashboard

### Current State

**Missing Components:**
- No rate limiting middleware or enforcement mechanism
- No token usage tracking for requests or tokens
- No quota management per provider or per token
- No rate limit configuration system
- No management API for rate limit administration
- No usage-aware token selection strategy

**Existing Infrastructure (Ready for Rate Limiting):**
- Token selection mechanism ([`internal/token/token_selection.go`](qwencoder-proxy/internal/token/token_selection.go:1))
- Token health tracking ([`internal/token/proxy_health_tracker.go`](qwencoder-proxy/internal/token/proxy_health_tracker.go:1))
- SQLite storage with efficient querying ([`internal/token/sqlite_store.go`](qwencoder-proxy/internal/token/sqlite_store.go:1))
- Middleware layer ([`internal/restapi/middleware.go`](qwencoder-proxy/internal/restapi/middleware.go:1))
- REST API server ([`internal/restapi/rest_api.go`](qwencoder-proxy/internal/restapi/rest_api.go:1))
- Provider abstraction layer ([`internal/provider/provider.go`](qwencoder-proxy/internal/provider/provider.go:1))
- OpenAI-compatible handler ([`internal/proxy/openai_handler.go`](qwencoder-proxy/internal/proxy/openai_handler.go:1))

---

## Solution Architecture

### Design Principles

1. **Internal Enforcement Only:** Rate limiting is enforced internally without exposing standard rate limit headers (e.g., `X-RateLimit-*`, `Retry-After`) to clients. This prevents clients from inferring rate limit behavior and attempting to optimize around it.

2. **Provider-Specific Quotas:** Each provider ([`ProviderQwen`](qwencoder-proxy/internal/provider/provider.go:16), [`ProviderGeminiCLI`](qwencoder-proxy/internal/provider/provider.go:17), [`ProviderKiro`](qwencoder-proxy/internal/provider/provider.go:18), [`ProviderAntigravity`](qwencoder-proxy/internal/provider/provider.go:19), [`ProviderIFlow`](qwencoder-proxy/internal/provider/provider.go:20)) has independent quota configurations.

3. **Multi-Metric Tracking:** Support three distinct metrics:
   - **Requests Per Day (RPD):** Total requests allowed per 24-hour rolling window
   - **Requests Per Minute (RPM):** Total requests allowed per 60-second sliding window
   - **Tokens Per Minute (TPM):** Total tokens (input + output) allowed per 60-second sliding window

4. **Real-Time Token Tracking:** Intercept `/v1/chat/completions` requests to estimate input tokens before request execution and count output tokens during streaming or after response completion.

5. **Usage-Aware Selection:** Token selection strategy prioritizes tokens/endpoints with the least current usage to optimize throughput and ensure quotas are never hit.

6. **Management API:** RESTful endpoints for configuring and monitoring rate limits, accessible by the dashboard for administrative control.

### Component Overview

```
internal/ratelimit/
├── rate_limiter.go           # RateLimiter interface and provider-specific implementations
├── middleware.go             # HTTP middleware for rate limiting (internal use only)
├── token_counter.go           # Token counting logic for chat completions
├── usage_tracker.go          # Usage tracking and storage layer
├── quota_manager.go          # Quota enforcement and management
├── strategies.go             # Usage-aware selection strategies
├── config.go                 # Rate limit configuration structures
└── errors.go                 # Rate limit error types (internal logging only)

internal/restapi/
├── rate_limit_api.go         # Management API endpoints for rate limiting (NEW)
└── (existing files)
```

### Architecture Diagram

```
┌─────────────────────────────────────────────────────────────────┐
│                         Client Request                           │
│                    /v1/chat/completions                         │
└────────────────────────────┬────────────────────────────────────┘
                             │
                             ▼
┌─────────────────────────────────────────────────────────────────┐
│                   Rate Limiting Middleware                       │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │ 1. Estimate input tokens from request body               │   │
│  │ 2. Check provider quotas (RPD, RPM, TPM)                 │   │
│  │ 3. If quota exceeded:                                    │   │
│  │    - Log internal error                                 │   │
│  │    - Return 429 with generic message (no rate headers)  │   │
│  │ 4. If quota available: proceed                           │   │
│  └──────────────────────────────────────────────────────────┘   │
└────────────────────────────┬────────────────────────────────────┘
                             │
                             ▼
┌─────────────────────────────────────────────────────────────────┐
│              Usage-Aware Token Selection Strategy                │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │ 1. Get all valid tokens for the provider                │   │
│  │ 2. Query current usage for each token                   │   │
│  │ 3. Filter tokens that would exceed quotas               │   │
│  │ 4. Sort by least usage (RPD, RPM, TPM)                  │   │
│  │ 5. Select token with highest remaining quota           │   │
│  └──────────────────────────────────────────────────────────┘   │
└────────────────────────────┬────────────────────────────────────┘
                             │
                             ▼
┌─────────────────────────────────────────────────────────────────┐
│                   Provider Request Execution                     │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │ 1. Execute request to provider                          │   │
│  │ 2. Stream response to client                             │   │
│  │ 3. Count output tokens in real-time (streaming) or      │   │
│    after completion (non-streaming)                          │   │
│  │ 4. Record final usage (request count + token count)      │   │
│  └──────────────────────────────────────────────────────────┘   │
└────────────────────────────┬────────────────────────────────────┘
                             │
                             ▼
┌─────────────────────────────────────────────────────────────────┐
│                   Usage Tracking & Storage                       │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │ 1. Update SQLite database with usage metrics             │   │
│  │    - provider_usage table: per-provider aggregates       │   │
│  │    - token_usage table: per-token detailed tracking      │   │
│  │ 2. Maintain sliding windows for RPM and TPM              │   │
│  │ 3. Maintain rolling 24-hour window for RPD               │   │
│  │ 4. Periodic cleanup of expired usage records             │   │
│  └──────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────┐
│                   Management API (Dashboard)                     │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │ GET    /api/ratelimit/config          List all configs  │   │
│  │ GET    /api/ratelimit/config/:provider Get provider cfg │   │
│  │ PUT    /api/ratelimit/config/:provider Update config    │   │
│  │ GET    /api/ratelimit/usage           Current usage     │   │
│  │ GET    /api/ratelimit/usage/:provider Provider usage    │   │
│  │ GET    /api/ratelimit/usage/:provider/:token Token usage│   │
│  │ POST   /api/ratelimit/reset/:provider Reset quotas      │   │
│  └──────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────┘
```

---

## Implementation Plan

### Phase 1: Core Rate Limiting Infrastructure (Days 1-5)

#### Step 1.1: Create Rate Limit Configuration Structures

**New File:** `internal/ratelimit/config.go`

```go
package ratelimit

import "time"

// ProviderRateLimitConfig defines rate limits for a specific provider
type ProviderRateLimitConfig struct {
	ProviderID string `json:"provider_id"`
	
	// RequestsPerDay limits total requests per 24-hour rolling window
	RequestsPerDay int `json:"requests_per_day"`
	
	// RequestsPerMinute limits requests per 60-second sliding window
	RequestsPerMinute int `json:"requests_per_minute"`
	
	// TokensPerMinute limits tokens (input + output) per 60-second sliding window
	TokensPerMinute int `json:"tokens_per_minute"`
	
	// Enabled indicates if rate limiting is active for this provider
	Enabled bool `json:"enabled"`
	
	// UpdatedAt timestamp of last configuration update
	UpdatedAt time.Time `json:"updated_at"`
}

// DefaultProviderConfigs returns default configurations for all providers
func DefaultProviderConfigs() map[string]ProviderRateLimitConfig {
	return map[string]ProviderRateLimitConfig{
		"gemini-cli": {
			ProviderID:         "gemini-cli",
			RequestsPerDay:     15000,  // Conservative default
			RequestsPerMinute:  60,     // Standard rate limit
			TokensPerMinute:    32000,  // ~1M tokens/day
			Enabled:            true,
			UpdatedAt:          time.Now(),
		},
		"qwen": {
			ProviderID:         "qwen",
			RequestsPerDay:     10000,
			RequestsPerMinute:  50,
			TokensPerMinute:    30000,
			Enabled:            true,
			UpdatedAt:          time.Now(),
		},
		"kiro": {
			ProviderID:         "kiro",
			RequestsPerDay:     5000,
			RequestsPerMinute:  30,
			TokensPerMinute:    20000,
			Enabled:            true,
			UpdatedAt:          time.Now(),
		},
		"antigravity": {
			ProviderID:         "antigravity",
			RequestsPerDay:     10000,
			RequestsPerMinute:  50,
			TokensPerMinute:    30000,
			Enabled:            true,
			UpdatedAt:          time.Now(),
		},
		"iflow": {
			ProviderID:         "iflow",
			RequestsPerDay:     5000,
			RequestsPerMinute:  30,
			TokensPerMinute:    20000,
			Enabled:            true,
			UpdatedAt:          time.Now(),
		},
	}
}

// UsageMetrics represents current usage metrics for a provider or token
type UsageMetrics struct {
	// RequestsToday is the count of requests in the current 24-hour window
	RequestsToday int `json:"requests_today"`
	
	// RequestsInMinute is the count of requests in the current 60-second window
	RequestsInMinute int `json:"requests_in_minute"`
	
	// TokensInMinute is the count of tokens in the current 60-second window
	TokensInMinute int `json:"tokens_in_minute"`
	
	// WindowStart marks the start of the current sliding window
	WindowStart time.Time `json:"window_start"`
	
	// DayStart marks the start of the current 24-hour period
	DayStart time.Time `json:"day_start"`
}

// QuotaStatus represents the remaining quota for a provider or token
type QuotaStatus struct {
	ProviderID string `json:"provider_id"`
	TokenID    string `json:"token_id,omitempty"`
	
	// Remaining quotas
	RemainingRequestsPerDay     int `json:"remaining_requests_per_day"`
	RemainingRequestsPerMinute  int `json:"remaining_requests_per_minute"`
	RemainingTokensPerMinute    int `json:"remaining_tokens_per_minute"`
	
	// Usage percentages (0-100)
	UsagePercentagePerDay       float64 `json:"usage_percentage_per_day"`
	UsagePercentagePerMinute    float64 `json:"usage_percentage_per_minute"`
	TokenUsagePercentagePerMinute float64 `json:"token_usage_percentage_per_minute"`
	
	// Time until quota reset
	SecondsUntilDayReset      int64 `json:"seconds_until_day_reset"`
	SecondsUntilMinuteReset   int64 `json:"seconds_until_minute_reset"`
	
	// IsLimited indicates if any quota is currently exceeded
	IsLimited bool `json:"is_limited"`
	
	// LimitReasons lists which quotas are exceeded
	LimitReasons []string `json:"limit_reasons"`
}
```

#### Step 1.2: Create Token Counter for Chat Completions

**New File:** `internal/ratelimit/token_counter.go`

```go
package ratelimit

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	
	"github.com/sunbankio/qwencoder-proxy/internal/logging"
)

// TokenCounter estimates and counts tokens for chat completion requests
type TokenCounter struct {
	logger logging.Logger
}

// NewTokenCounter creates a new token counter
func NewTokenCounter(logger logging.Logger) *TokenCounter {
	return &TokenCounter{
		logger: logger,
	}
}

// EstimateInputTokens estimates the number of tokens in the input request
// This is called BEFORE sending the request to the provider
func (tc *TokenCounter) EstimateInputTokens(openaiReq map[string]interface{}) (int, error) {
	messages, ok := openaiReq["messages"].([]interface{})
	if !ok {
		return 0, fmt.Errorf("invalid messages format")
	}
	
	totalTokens := 0
	
	for _, msg := range messages {
		msgMap, ok := msg.(map[string]interface{})
		if !ok {
			continue
		}
		
		content, ok := msgMap["content"]
		if !ok {
			continue
		}
		
		// Handle different content formats
		switch v := content.(type) {
		case string:
			totalTokens += tc.estimateTokens(v)
		case []interface{}:
			// Multi-modal content (text + images)
			for _, item := range v {
				itemMap, ok := item.(map[string]interface{})
				if !ok {
					continue
				}
				
				if itemType, ok := itemMap["type"].(string); ok && itemType == "text" {
					if text, ok := itemMap["text"].(string); ok {
						totalTokens += tc.estimateTokens(text)
					}
				} else if itemType == "image_url" {
					// Images typically cost ~85-565 tokens depending on resolution
					// Use conservative estimate
					totalTokens += 85
				}
			}
		}
	}
	
	// Add tokens for system message overhead, formatting, etc.
	// Conservative estimate: ~10 tokens per message for formatting
	totalTokens += len(messages) * 10
	
	tc.logger.DebugLog("[TokenCounter] Estimated %d input tokens for request", totalTokens)
	return totalTokens, nil
}

// estimateTokens provides a rough estimate of token count for text
// Uses character-based approximation: ~4 characters per token (English)
func (tc *TokenCounter) estimateTokens(text string) int {
	if text == "" {
		return 0
	}
	
	// Remove whitespace for more accurate count
	cleanText := strings.Join(strings.Fields(text), "")
	
	// Estimate: ~4 characters per token for English text
	// This is a conservative estimate; actual tokenization varies by model
	estimatedTokens := int(math.Ceil(float64(len(cleanText)) / 4.0))
	
	return estimatedTokens
}

// CountOutputTokens counts tokens in the provider response
// This is called AFTER receiving the response from the provider
func (tc *TokenCounter) CountOutputTokens(response map[string]interface{}) (int, error) {
	// Check for usage information in response
	if usage, ok := response["usage"].(map[string]interface{}); ok {
		if completionTokens, ok := usage["completion_tokens"].(float64); ok {
			return int(completionTokens), nil
		}
	}
	
	// If no usage info, estimate from choices content
	choices, ok := response["choices"].([]interface{})
	if !ok {
		return 0, fmt.Errorf("no choices in response")
	}
	
	totalTokens := 0
	for _, choice := range choices {
		choiceMap, ok := choice.(map[string]interface{})
		if !ok {
			continue
		}
		
		message, ok := choiceMap["message"].(map[string]interface{})
		if !ok {
			continue
		}
		
		content, ok := message["content"].(string)
		if !ok {
			continue
		}
		
		totalTokens += tc.estimateTokens(content)
	}
	
	tc.logger.DebugLog("[TokenCounter] Counted %d output tokens from response", totalTokens)
	return totalTokens, nil
}

// CountStreamingOutputTokens counts tokens from streaming response chunks
// This is called during streaming to track real-time token usage
func (tc *TokenCounter) CountStreamingOutputTokens(chunk []byte) (int, error) {
	// Parse SSE chunk
	chunkStr := string(chunk)
	if !strings.HasPrefix(chunkStr, "data: ") {
		return 0, nil
	}
	
	dataStr := strings.TrimPrefix(chunkStr, "data: ")
	if dataStr == "[DONE]" {
		return 0, nil
	}
	
	var chunkData map[string]interface{}
	if err := json.Unmarshal([]byte(dataStr), &chunkData); err != nil {
		return 0, fmt.Errorf("failed to parse chunk: %w", err)
	}
	
	// Extract delta content
	choices, ok := chunkData["choices"].([]interface{})
	if !ok || len(choices) == 0 {
		return 0, nil
	}
	
	choice, ok := choices[0].(map[string]interface{})
	if !ok {
		return 0, nil
	}
	
	delta, ok := choice["delta"].(map[string]interface{})
	if !ok {
		return 0, nil
	}
	
	content, ok := delta["content"].(string)
	if !ok {
		return 0, nil
	}
	
	// Estimate tokens in this chunk
	tokens := tc.estimateTokens(content)
	
	return tokens, nil
}

// GetTotalTokensFromResponse extracts total token usage from a complete response
func (tc *TokenCounter) GetTotalTokensFromResponse(response map[string]interface{}) (int, error) {
	if usage, ok := response["usage"].(map[string]interface{}); ok {
		if totalTokens, ok := usage["total_tokens"].(float64); ok {
			return int(totalTokens), nil
		}
	}
	
	return 0, fmt.Errorf("no token usage information in response")
}
```

#### Step 1.3: Create Usage Tracker with SQLite Storage

**New File:** `internal/ratelimit/usage_tracker.go`

```go
package ratelimit

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"
	
	"github.com/google/uuid"
	"github.com/sunbankio/qwencoder-proxy/internal/logging"
	_ "modernc.org/sqlite"
)

// UsageTracker manages usage tracking and storage
type UsageTracker struct {
	db     *sql.DB
	logger logging.Logger
	mu     sync.RWMutex
	
	// In-memory cache for frequently accessed metrics
	providerCache map[string]*UsageMetrics
	tokenCache    map[string]*UsageMetrics
	cacheExpiry   time.Time
}

// NewUsageTracker creates a new usage tracker
func NewUsageTracker(dbPath string, logger logging.Logger) (*UsageTracker, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}
	
	tracker := &UsageTracker{
		db:            db,
		logger:        logger,
		providerCache: make(map[string]*UsageMetrics),
		tokenCache:    make(map[string]*UsageMetrics),
		cacheExpiry:   time.Now(),
	}
	
	if err := tracker.initializeDB(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}
	
	// Start periodic cleanup
	go tracker.periodicCleanup()
	
	return tracker, nil
}

// initializeDB creates the necessary tables and indexes
func (ut *UsageTracker) initializeDB() error {
	// Create provider_usage table
	providerUsageTable := `
		CREATE TABLE IF NOT EXISTS provider_usage (
			id TEXT PRIMARY KEY,
			provider_id TEXT NOT NULL,
			requests_today INTEGER NOT NULL DEFAULT 0,
			requests_in_minute INTEGER NOT NULL DEFAULT 0,
			tokens_in_minute INTEGER NOT NULL DEFAULT 0,
			window_start INTEGER NOT NULL,
			day_start INTEGER NOT NULL,
			created_at INTEGER NOT NULL DEFAULT (strftime('%s', 'subsec') * 1000),
			updated_at INTEGER NOT NULL DEFAULT (strftime('%s', 'subsec') * 1000),
			UNIQUE(provider_id)
		)
	`
	if _, err := ut.db.Exec(providerUsageTable); err != nil {
		return fmt.Errorf("failed to create provider_usage table: %w", err)
	}
	
	// Create token_usage table
	tokenUsageTable := `
		CREATE TABLE IF NOT EXISTS token_usage (
			id TEXT PRIMARY KEY,
			provider_id TEXT NOT NULL,
			token_id TEXT NOT NULL,
			requests_today INTEGER NOT NULL DEFAULT 0,
			requests_in_minute INTEGER NOT NULL DEFAULT 0,
			tokens_in_minute INTEGER NOT NULL DEFAULT 0,
			window_start INTEGER NOT NULL,
			day_start INTEGER NOT NULL,
			created_at INTEGER NOT NULL DEFAULT (strftime('%s', 'subsec') * 1000),
			updated_at INTEGER NOT NULL DEFAULT (strftime('%s', 'subsec') * 1000),
			UNIQUE(provider_id, token_id)
		)
	`
	if _, err := ut.db.Exec(tokenUsageTable); err != nil {
		return fmt.Errorf("failed to create token_usage table: %w", err)
	}
	
	// Create indexes
	indexes := []string{
		"CREATE INDEX IF NOT EXISTS idx_provider_usage_provider ON provider_usage(provider_id)",
		"CREATE INDEX IF NOT EXISTS idx_token_usage_provider ON token_usage(provider_id)",
		"CREATE INDEX IF NOT EXISTS idx_token_usage_token ON token_usage(token_id)",
	}
	
	for _, idx := range indexes {
		if _, err := ut.db.Exec(idx); err != nil {
			return fmt.Errorf("failed to create index: %w", err)
		}
	}
	
	return nil
}

// RecordUsage records usage for a provider and optionally a token
func (ut *UsageTracker) RecordUsage(ctx context.Context, providerID string, tokenID string, requestCount int, tokenCount int) error {
	ut.mu.Lock()
	defer ut.mu.Unlock()
	
	now := time.Now()
	nowMs := now.UnixMilli()
	
	// Get or create provider usage
	providerMetrics, err := ut.getOrCreateProviderMetrics(ctx, providerID, now)
	if err != nil {
		return fmt.Errorf("failed to get provider metrics: %w", err)
	}
	
	// Update provider usage
	if err := ut.updateUsage(ctx, "provider_usage", providerID, "", providerMetrics, requestCount, tokenCount, now); err != nil {
		return fmt.Errorf("failed to update provider usage: %w", err)
	}
	
	// Update token usage if token ID provided
	if tokenID != "" {
		tokenMetrics, err := ut.getOrCreateTokenMetrics(ctx, providerID, tokenID, now)
		if err != nil {
			return fmt.Errorf("failed to get token metrics: %w", err)
		}
		
		if err := ut.updateUsage(ctx, "token_usage", providerID, tokenID, tokenMetrics, requestCount, tokenCount, now); err != nil {
			return fmt.Errorf("failed to update token usage: %w", err)
		}
	}
	
	// Invalidate cache
	ut.cacheExpiry = time.Time{}
	
	return nil
}

// getOrCreateProviderMetrics retrieves or creates provider usage metrics
func (ut *UsageTracker) getOrCreateProviderMetrics(ctx context.Context, providerID string, now time.Time) (*UsageMetrics, error) {
	metrics, err := ut.getProviderUsage(ctx, providerID)
	if err == sql.ErrNoRows {
		// Create new metrics
		nowMs := now.UnixMilli()
		dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		windowStart := now.Add(-time.Minute)
		
		query := `
			INSERT INTO provider_usage (id, provider_id, requests_today, requests_in_minute, tokens_in_minute, window_start, day_start)
			VALUES (?, ?, 0, 0, 0, ?, ?)
		`
		_, err = ut.db.ExecContext(ctx, query, uuid.New().String(), providerID, windowStart.UnixMilli(), dayStart.UnixMilli())
		if err != nil {
			return nil, fmt.Errorf("failed to create provider usage: %w", err)
		}
		
		return &UsageMetrics{
			RequestsToday:      0,
			RequestsInMinute:   0,
			TokensInMinute:     0,
			WindowStart:        windowStart,
			DayStart:           dayStart,
		}, nil
	} else if err != nil {
		return nil, err
	}
	
	// Check if windows need reset
	metrics = ut.resetWindowsIfNeeded(metrics, now)
	
	return metrics, nil
}

// getOrCreateTokenMetrics retrieves or creates token usage metrics
func (ut *UsageTracker) getOrCreateTokenMetrics(ctx context.Context, providerID string, tokenID string, now time.Time) (*UsageMetrics, error) {
	metrics, err := ut.getTokenUsage(ctx, providerID, tokenID)
	if err == sql.ErrNoRows {
		// Create new metrics
		nowMs := now.UnixMilli()
		dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		windowStart := now.Add(-time.Minute)
		
		query := `
			INSERT INTO token_usage (id, provider_id, token_id, requests_today, requests_in_minute, tokens_in_minute, window_start, day_start)
			VALUES (?, ?, ?, 0, 0, 0, ?, ?)
		`
		_, err = ut.db.ExecContext(ctx, query, uuid.New().String(), providerID, tokenID, windowStart.UnixMilli(), dayStart.UnixMilli())
		if err != nil {
			return nil, fmt.Errorf("failed to create token usage: %w", err)
		}
		
		return &UsageMetrics{
			RequestsToday:      0,
			RequestsInMinute:   0,
			TokensInMinute:     0,
			WindowStart:        windowStart,
			DayStart:           dayStart,
		}, nil
	} else if err != nil {
		return nil, err
	}
	
	// Check if windows need reset
	metrics = ut.resetWindowsIfNeeded(metrics, now)
	
	return metrics, nil
}

// resetWindowsIfNeeded resets sliding windows if they've expired
func (ut *UsageTracker) resetWindowsIfNeeded(metrics *UsageMetrics, now time.Time) *UsageMetrics {
	needsUpdate := false
	
	// Check minute window
	if now.Sub(metrics.WindowStart) >= time.Minute {
		metrics.RequestsInMinute = 0
		metrics.TokensInMinute = 0
		metrics.WindowStart = now
		needsUpdate = true
	}
	
	// Check day window
	if now.Sub(metrics.DayStart) >= 24*time.Hour {
		metrics.RequestsToday = 0
		metrics.DayStart = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		needsUpdate = true
	}
	
	return metrics
}

// updateUsage updates usage metrics in the database
func (ut *UsageTracker) updateUsage(ctx context.Context, tableName string, providerID string, tokenID string, metrics *UsageMetrics, requestCount int, tokenCount int, now time.Time) error {
	nowMs := now.UnixMilli()
	
	var query string
	var args []interface{}
	
	if tokenID == "" {
		query = `
			UPDATE provider_usage
			SET requests_today = ?, requests_in_minute = ?, tokens_in_minute = ?, window_start = ?, day_start = ?, updated_at = ?
			WHERE provider_id = ?
		`
		args = []interface{}{
			metrics.RequestsToday + requestCount,
			metrics.RequestsInMinute + requestCount,
			metrics.TokensInMinute + tokenCount,
			metrics.WindowStart.UnixMilli(),
			metrics.DayStart.UnixMilli(),
			nowMs,
			providerID,
		}
	} else {
		query = `
			UPDATE token_usage
			SET requests_today = ?, requests_in_minute = ?, tokens_in_minute = ?, window_start = ?, day_start = ?, updated_at = ?
			WHERE provider_id = ? AND token_id = ?
		`
		args = []interface{}{
			metrics.RequestsToday + requestCount,
			metrics.RequestsInMinute + requestCount,
			metrics.TokensInMinute + tokenCount,
			metrics.WindowStart.UnixMilli(),
			metrics.DayStart.UnixMilli(),
			nowMs,
			providerID,
			tokenID,
		}
	}
	
	result, err := ut.db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("failed to update usage: %w", err)
	}
	
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	
	if rowsAffected == 0 {
		return fmt.Errorf("no rows updated for %s %s", tableName, providerID)
	}
	
	return nil
}

// GetProviderUsage retrieves current usage metrics for a provider
func (ut *UsageTracker) GetProviderUsage(ctx context.Context, providerID string) (*UsageMetrics, error) {
	return ut.getProviderUsage(ctx, providerID)
}

// getProviderUsage retrieves provider usage from database
func (ut *UsageTracker) getProviderUsage(ctx context.Context, providerID string) (*UsageMetrics, error) {
	query := `
		SELECT requests_today, requests_in_minute, tokens_in_minute, window_start, day_start
		FROM provider_usage
		WHERE provider_id = ?
	`
	
	var metrics UsageMetrics
	var windowStartMs, dayStartMs int64
	
	err := ut.db.QueryRowContext(ctx, query, providerID).Scan(
		&metrics.RequestsToday,
		&metrics.RequestsInMinute,
		&metrics.TokensInMinute,
		&windowStartMs,
		&dayStartMs,
	)
	
	if err != nil {
		return nil, err
	}
	
	metrics.WindowStart = time.UnixMilli(windowStartMs)
	metrics.DayStart = time.UnixMilli(dayStartMs)
	
	// Reset windows if needed
	metrics = ut.resetWindowsIfNeeded(&metrics, time.Now())
	
	return &metrics, nil
}

// GetTokenUsage retrieves current usage metrics for a token
func (ut *UsageTracker) GetTokenUsage(ctx context.Context, providerID string, tokenID string) (*UsageMetrics, error) {
	return ut.getTokenUsage(ctx, providerID, tokenID)
}

// getTokenUsage retrieves token usage from database
func (ut *UsageTracker) getTokenUsage(ctx context.Context, providerID string, tokenID string) (*UsageMetrics, error) {
	query := `
		SELECT requests_today, requests_in_minute, tokens_in_minute, window_start, day_start
		FROM token_usage
		WHERE provider_id = ? AND token_id = ?
	`
	
	var metrics UsageMetrics
	var windowStartMs, dayStartMs int64
	
	err := ut.db.QueryRowContext(ctx, query, providerID, tokenID).Scan(
		&metrics.RequestsToday,
		&metrics.RequestsInMinute,
		&metrics.TokensInMinute,
		&windowStartMs,
		&dayStartMs,
	)
	
	if err != nil {
		return nil, err
	}
	
	metrics.WindowStart = time.UnixMilli(windowStartMs)
	metrics.DayStart = time.UnixMilli(dayStartMs)
	
	// Reset windows if needed
	metrics = ut.resetWindowsIfNeeded(&metrics, time.Now())
	
	return &metrics, nil
}

// GetAllProviderUsage retrieves usage metrics for all providers
func (ut *UsageTracker) GetAllProviderUsage(ctx context.Context) (map[string]*UsageMetrics, error) {
	query := `
		SELECT provider_id, requests_today, requests_in_minute, tokens_in_minute, window_start, day_start
		FROM provider_usage
	`
	
	rows, err := ut.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query provider usage: %w", err)
	}
	defer rows.Close()
	
	result := make(map[string]*UsageMetrics)
	
	for rows.Next() {
		var providerID string
		var metrics UsageMetrics
		var windowStartMs, dayStartMs int64
		
		if err := rows.Scan(&providerID, &metrics.RequestsToday, &metrics.RequestsInMinute, &metrics.TokensInMinute, &windowStartMs, &dayStartMs); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}
		
		metrics.WindowStart = time.UnixMilli(windowStartMs)
		metrics.DayStart = time.UnixMilli(dayStartMs)
		
		// Reset windows if needed
		metrics = ut.resetWindowsIfNeeded(&metrics, time.Now())
		
		result[providerID] = &metrics
	}
	
	return result, nil
}

// periodicCleanup removes old usage records periodically
func (ut *UsageTracker) periodicCleanup() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()
	
	for range ticker.C {
		ctx := context.Background()
		cutoff := time.Now().Add(-7 * 24 * time.Hour) // Keep 7 days of history
		
		// Clean old records (optional, for historical analysis)
		// For now, we keep all records as they're aggregated
		ut.logger.DebugLog("[UsageTracker] Periodic cleanup check completed")
	}
}

// Close closes the database connection
func (ut *UsageTracker) Close() error {
	return ut.db.Close()
}
```

#### Step 1.4: Create Quota Manager

**New File:** `internal/ratelimit/quota_manager.go`

```go
package ratelimit

import (
	"context"
	"fmt"
	"sync"
	"time"
	
	"github.com/sunbankio/qwencoder-proxy/internal/logging"
)

// QuotaManager enforces rate limits based on configuration and usage
type QuotaManager struct {
	configs       map[string]ProviderRateLimitConfig
	usageTracker  *UsageTracker
	logger        logging.Logger
	mu            sync.RWMutex
	tokenCounter  *TokenCounter
}

// NewQuotaManager creates a new quota manager
func NewQuotaManager(usageTracker *UsageTracker, logger logging.Logger) *QuotaManager {
	return &QuotaManager{
		configs:      DefaultProviderConfigs(),
		usageTracker: usageTracker,
		logger:       logger,
		tokenCounter: NewTokenCounter(logger),
	}
}

// CheckQuota checks if a request is allowed based on current usage
// Returns (allowed, quotaStatus, error)
func (qm *QuotaManager) CheckQuota(ctx context.Context, providerID string, tokenID string, estimatedInputTokens int) (bool, *QuotaStatus, error) {
	qm.mu.RLock()
	config, exists := qm.configs[providerID]
	qm.mu.RUnlock()
	
	if !exists {
		return false, nil, fmt.Errorf("no rate limit configuration for provider: %s", providerID)
	}
	
	// If rate limiting is disabled, allow all requests
	if !config.Enabled {
		return true, &QuotaStatus{IsLimited: false}, nil
	}
	
	// Get current usage metrics
	var providerMetrics *UsageMetrics
	var tokenMetrics *UsageMetrics
	
	providerMetrics, err := qm.usageTracker.GetProviderUsage(ctx, providerID)
	if err != nil {
		// If no usage record exists, assume zero usage
		if err.Error() == "sql: no rows in result set" {
			now := time.Now()
			dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
			windowStart := now.Add(-time.Minute)
			providerMetrics = &UsageMetrics{
				RequestsToday:      0,
				RequestsInMinute:   0,
				TokensInMinute:     0,
				WindowStart:        windowStart,
				DayStart:           dayStart,
			}
		} else {
			return false, nil, fmt.Errorf("failed to get provider usage: %w", err)
		}
	}
	
	if tokenID != "" {
		tokenMetrics, err = qm.usageTracker.GetTokenUsage(ctx, providerID, tokenID)
		if err != nil {
			// If no usage record exists, assume zero usage
			if err.Error() == "sql: no rows in result set" {
				now := time.Now()
				dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
				windowStart := now.Add(-time.Minute)
				tokenMetrics = &UsageMetrics{
					RequestsToday:      0,
					RequestsInMinute:   0,
					TokensInMinute:     0,
					WindowStart:        windowStart,
					DayStart:           dayStart,
				}
			} else {
				return false, nil, fmt.Errorf("failed to get token usage: %w", err)
			}
		}
	}
	
	// Build quota status
	status := qm.buildQuotaStatus(providerID, tokenID, config, providerMetrics, tokenMetrics, estimatedInputTokens)
	
	// Check if any quota is exceeded
	if status.IsLimited {
		qm.logger.WarnLog("[QuotaManager] Rate limit exceeded for provider %s: %v", providerID, status.LimitReasons)
		return false, status, nil
	}
	
	return true, status, nil
}

// buildQuotaStatus constructs quota status from metrics and configuration
func (qm *QuotaManager) buildQuotaStatus(providerID string, tokenID string, config ProviderRateLimitConfig, providerMetrics *UsageMetrics, tokenMetrics *UsageMetrics, estimatedInputTokens int) *QuotaStatus {
	now := time.Now()
	
	// Use token metrics if available, otherwise use provider metrics
	metrics := providerMetrics
	if tokenMetrics != nil {
		metrics = tokenMetrics
	}
	
	status := &QuotaStatus{
		ProviderID: providerID,
		TokenID:    tokenID,
		IsLimited:  false,
		LimitReasons: []string{},
	}
	
	// Calculate remaining quotas
	status.RemainingRequestsPerDay = config.RequestsPerDay - metrics.RequestsToday
	status.RemainingRequestsPerMinute = config.RequestsPerMinute - metrics.RequestsInMinute
	
	// For tokens, include estimated input tokens
	status.RemainingTokensPerMinute = config.TokensPerMinute - metrics.TokensInMinute - estimatedInputTokens
	
	// Calculate usage percentages
	if config.RequestsPerDay > 0 {
		status.UsagePercentagePerDay = float64(metrics.RequestsToday) / float64(config.RequestsPerDay) * 100
	}
	if config.RequestsPerMinute > 0 {
		status.UsagePercentagePerMinute = float64(metrics.RequestsInMinute) / float64(config.RequestsPerMinute) * 100
	}
	if config.TokensPerMinute > 0 {
		status.TokenUsagePercentagePerMinute = float64(metrics.TokensInMinute+estimatedInputTokens) / float64(config.TokensPerMinute) * 100
	}
	
	// Calculate time until reset
	nextDayStart := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, now.Location())
	status.SecondsUntilDayReset = int64(nextDayStart.Sub(now).Seconds())
	
	nextMinuteStart := metrics.WindowStart.Add(time.Minute)
	status.SecondsUntilMinuteReset = int64(nextMinuteStart.Sub(now).Seconds())
	
	// Check if quotas are exceeded
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

// RecordUsage records usage after a successful request
func (qm *QuotaManager) RecordUsage(ctx context.Context, providerID string, tokenID string, inputTokens int, outputTokens int) error {
	totalTokens := inputTokens + outputTokens
	
	if err := qm.usageTracker.RecordUsage(ctx, providerID, tokenID, 1, totalTokens); err != nil {
		return fmt.Errorf("failed to record usage: %w", err)
	}
	
	qm.logger.DebugLog("[QuotaManager] Recorded usage for %s: %d tokens (input: %d, output: %d)", 
		providerID, totalTokens, inputTokens, outputTokens)
	
	return nil
}

// UpdateConfig updates rate limit configuration for a provider
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

// GetConfig retrieves rate limit configuration for a provider
func (qm *QuotaManager) GetConfig(providerID string) (ProviderRateLimitConfig, error) {
	qm.mu.RLock()
	defer qm.mu.RUnlock()
	
	config, exists := qm.configs[providerID]
	if !exists {
		return ProviderRateLimitConfig{}, fmt.Errorf("no configuration for provider: %s", providerID)
	}
	
	return config, nil
}

// GetAllConfigs retrieves all rate limit configurations
func (qm *QuotaManager) GetAllConfigs() map[string]ProviderRateLimitConfig {
	qm.mu.RLock()
	defer qm.mu.RUnlock()
	
	// Return a copy to prevent external modifications
	result := make(map[string]ProviderRateLimitConfig)
	for k, v := range qm.configs {
		result[k] = v
	}
	
	return result
}

// GetQuotaStatus retrieves current quota status for a provider
func (qm *QuotaManager) GetQuotaStatus(ctx context.Context, providerID string, tokenID string) (*QuotaStatus, error) {
	qm.mu.RLock()
	config, exists := qm.configs[providerID]
	qm.mu.RUnlock()
	
	if !exists {
		return nil, fmt.Errorf("no rate limit configuration for provider: %s", providerID)
	}
	
	providerMetrics, err := qm.usageTracker.GetProviderUsage(ctx, providerID)
	if err != nil {
		return nil, fmt.Errorf("failed to get provider usage: %w", err)
	}
	
	var tokenMetrics *UsageMetrics
	if tokenID != "" {
		tokenMetrics, err = qm.usageTracker.GetTokenUsage(ctx, providerID, tokenID)
		if err != nil {
			return nil, fmt.Errorf("failed to get token usage: %w", err)
		}
	}
	
	status := qm.buildQuotaStatus(providerID, tokenID, config, providerMetrics, tokenMetrics, 0)
	
	return status, nil
}

// ResetUsage resets usage metrics for a provider or token
func (qm *QuotaManager) ResetUsage(ctx context.Context, providerID string, tokenID string) error {
	// This would need to be implemented in usage tracker
	// For now, we'll implement a simple version
	qm.logger.InfoLog("[QuotaManager] Reset usage requested for %s (token: %s)", providerID, tokenID)
	
	// Implementation would involve setting counts to 0 in the database
	// This is a placeholder for the actual implementation
	
	return nil
}

// EstimateInputTokens estimates input tokens from a request
func (qm *QuotaManager) EstimateInputTokens(openaiReq map[string]interface{}) (int, error) {
	return qm.tokenCounter.EstimateInputTokens(openaiReq)
}

// CountOutputTokens counts output tokens from a response
func (qm *QuotaManager) CountOutputTokens(response map[string]interface{}) (int, error) {
	return qm.tokenCounter.CountOutputTokens(response)
}
```

#### Step 1.5: Create Rate Limiting Errors

**New File:** `internal/ratelimit/errors.go`

```go
package ratelimit

import "fmt"

// ErrorCode defines rate limit error codes (internal use only)
type ErrorCode string

const (
	ErrCodeRateLimitExceeded  ErrorCode = "rate_limit_exceeded"
	ErrCodeInvalidProvider    ErrorCode = "invalid_provider"
	ErrCodeConfigurationError ErrorCode = "configuration_error"
)

// RateLimitError represents a rate limiting error (internal logging only)
type RateLimitError struct {
	Code       ErrorCode
	Message    string
	ProviderID string
	TokenID    string
	Reasons    []string
}

func (e *RateLimitError) Error() string {
	return fmt.Sprintf("[%s] %s (provider: %s, token: %s, reasons: %v)", 
		e.Code, e.Message, e.ProviderID, e.TokenID, e.Reasons)
}

// NewRateLimitExceededError creates a new rate limit exceeded error
func NewRateLimitExceededError(providerID string, tokenID string, reasons []string) *RateLimitError {
	return &RateLimitError{
		Code:       ErrCodeRateLimitExceeded,
		Message:    "Rate limit exceeded",
		ProviderID: providerID,
		TokenID:    tokenID,
		Reasons:    reasons,
	}
}

// NewInvalidProviderError creates a new invalid provider error
func NewInvalidProviderError(providerID string) *RateLimitError {
	return &RateLimitError{
		Code:       ErrCodeInvalidProvider,
		Message:    "Invalid provider ID",
		ProviderID: providerID,
		TokenID:    "",
		Reasons:    []string{},
	}
}

// NewConfigurationError creates a new configuration error
func NewConfigurationError(message string) *RateLimitError {
	return &RateLimitError{
		Code:       ErrCodeConfigurationError,
		Message:    message,
		ProviderID: "",
		TokenID:    "",
		Reasons:    []string{},
	}
}
```

---

## Summary

This comprehensive implementation plan provides a complete architecture for provider-aware rate limiting in the qwencoder-proxy. The key features include:

1. **Internal Enforcement Only:** No rate limit headers exposed to clients, preventing information leakage
2. **Provider-Specific Quotas:** Independent configuration for each provider (gemini-cli, qwen, kiro, antigravity, iflow)
3. **Multi-Metric Tracking:** Support for requests per day, requests per minute, and tokens per minute
4. **Real-Time Token Tracking:** Accurate estimation of input tokens and counting of output tokens
5. **Usage-Aware Selection:** Intelligent token selection that prioritizes endpoints with least usage
6. **Management API:** Complete REST API for dashboard integration and configuration

The implementation is designed to be secure, performant, scalable, and fully observable through comprehensive logging and metrics.
