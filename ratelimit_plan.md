# Token-Based Rate Limiting Implementation Plan

## Executive Summary

This document provides a comprehensive implementation plan for adding token-based rate limiting to the qwencoder-proxy. The goal is to track requests per token per day and implement rate limits based on official provider limits (e.g., Gemini CLI: 1000 requests/day, 60 requests/minute; Qwen: 2000 requests/day).

**Recommended Approach:** In-memory token usage tracker with periodic persistence for minimal overhead.

---

## Table of Contents

1. [Current Architecture Overview](#current-architecture-overview)
2. [Implementation Strategy](#implementation-strategy)
3. [Phase 1: Core Rate Limiting](#phase-1-core-rate-limiting)
4. [Phase 2: API Integration](#phase-2-api-integration)
5. [Phase 3: Advanced Features](#phase-3-advanced-features)
6. [Testing Strategy](#testing-strategy)
7. [Configuration](#configuration)
8. [Future Enhancements](#future-enhancements)

---

## Current Architecture Overview

### Token Management System

The project uses a sophisticated multi-token management system located in [`internal/token/`](internal/token/):

#### Key Components:

1. **[`ProviderToken`](internal/token/multi_token_store.go:18-36)** - Stores token metadata:
   ```go
   type ProviderToken struct {
       ID               string       `json:"id"` // Unique token ID (UUID)
       AccessToken      string       `json:"access_token"`
       RefreshToken     string       `json:"refresh_token"`
       TokenType        string       `json:"token_type"`
       ExpiryDate       int64        `json:"expiry_date"`
       Email            string       `json:"email"`
       Healthy          bool         `json:"healthy"`
       HealthScore      float64      `json:"health_score"`
       LastUsed         int64        `json:"last_used"`
       CreatedAt        int64        `json:"created_at"`
       ErrorCount       int          `json:"error_count"`
       LastError        string       `json:"last_error,omitempty"`
       Proxy            *ProxyConfig `json:"proxy,omitempty"`
       ProxyHealthScore float64      `json:"proxy_health_score,omitempty"`
   }
   ```

2. **[`MultiTokenStore`](internal/token/multi_token_store.go:45-54)** - Manages multiple tokens per provider
3. **[`TokenManager`](internal/token/token_selection.go)** - Handles token selection with strategies:
   - `RandomSelectionStrategy` ([`internal/token/token_selection.go:40-68`](internal/token/token_selection.go:40-68))
   - `RoundRobinSelectionStrategy` ([`internal/token/token_selection.go:70-103`](internal/token/token_selection.go:70-103))
   - `LeastUsedSelectionStrategy` (referenced in [`StoreSettings`](internal/token/multi_token_store.go:38-43))

4. **[`StoreSettings`](internal/token/multi_token_store.go:38-43)** - Provider-specific settings:
   ```go
   type StoreSettings struct {
       SelectionStrategy string `json:"selection_strategy"` // "random", "round_robin", "least_used"
       RefreshBufferSec  int    `json:"refresh_buffer_sec"`
       MaxErrorCount     int    `json:"max_error_count"`
   }
   ```

### Request Flow

Requests flow through the following path:

```
User Request
    ↓
[OpenAIHandler.ServeHTTP()] (proxy/openai_handler.go:70-102)
    ↓
[handleChatCompletions()] (proxy/openai_handler.go:152-213)
    ↓
Provider Selection (factory.Get() or factory.GetByModel())
    ↓
Provider.GenerateContent() or GenerateContentStream()
    ↓
TokenManager.SelectTokenWithClient() ← RATE LIMIT CHECK HERE
    ↓
HTTP Request to Provider API
    ↓
Response
```

### Provider Implementations

Key provider implementations that use token selection:

1. **Gemini** ([`provider/gemini/gemini.go`](provider/gemini/gemini.go:487-590))
   - Uses `SelectTokenWithClient()` at line 508
   - Logs token usage with DebugLog

2. **Qwen** ([`provider/qwen/qwen.go`](provider/qwen/qwen.go:361-448))
   - Uses `SelectTokenWithClient()` at line 369
   - Logs token usage with DebugLog

3. **Kiro** ([`provider/kiro/kiro.go`](provider/kiro/kiro.go)) - Similar pattern
4. **Antigravity** ([`provider/antigravity/antigravity.go`](provider/antigravity/antigravity.go)) - Similar pattern
5. **IFlow** ([`provider/iflow/iflow.go`](provider/iflow/iflow.go)) - Similar pattern

### Logging Infrastructure

Existing logging system in [`logging/logger.go`](logging/logger.go:1-100):

- `StreamLog` - Streaming requests (blue)
- `NonStreamLog` - Non-streaming requests (cyan)
- `DoneLog` - Streaming completions (green)
- `DoneNonStreamLog` - Non-streaming completions (green)
- `ErrorLog` - Errors (red)
- `WarningLog` - Warnings (yellow)
- `DebugLog` - Debug messages (magenta, only when debug mode enabled)

---

## Implementation Strategy

### Architecture Design

```
┌─────────────────────────────────────────────────────────────┐
│                    Rate Limit Manager                         │
│  ┌─────────────────────────────────────────────────────┐   │
│  │  TokenUsageTracker (per-token usage counters)       │   │
│  │  - DailyRequestCount: map[tokenID]int               │   │
│  │  - MinuteRequestCount: map[tokenID]int             │   │
│  │  - LastResetTime: time.Time                        │   │
│  │  - mu: sync.RWMutex                                │   │
│  └─────────────────────────────────────────────────────┘   │
│  ┌─────────────────────────────────────────────────────┐   │
│  │  ProviderLimits (provider-specific limits)          │   │
│  │  - gemini-cli: 1000/day, 60/min                    │   │
│  │  - qwen: 2000/day, 0/min (no minute limit)         │   │
│  │  - kiro: [to be defined]                           │   │
│  │  - antigravity: [to be defined]                    │   │
│  │  - iflow: [to be defined]                          │   │
│  └─────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
                            │
                            │ Check limits before request
                            ▼
┌─────────────────────────────────────────────────────────────┐
│              TokenManager.SelectTokenWithClient()           │
│  └─ Modified to check rate limits before returning token  │
└─────────────────────────────────────────────────────────────┘
```

### Why This Approach?

| Aspect | In-Memory Tracker | Database | Redis |
|--------|------------------|----------|-------|
| **Overhead** | Minimal (O(1) counter) | High (disk I/O) | Medium (network) |
| **Latency** | < 1ms | 10-50ms | 5-20ms |
| **Complexity** | Low | Medium | Medium |
| **Persistence** | Optional (add later) | Built-in | Built-in |
| **Scalability** | Single instance | Multi-instance | Multi-instance |
| **Dependencies** | None | DB driver | Redis server |

**Winner:** In-Memory Tracker - Best balance of performance and simplicity for single-instance deployments.

---

## Phase 1: Core Rate Limiting

### Step 1.1: Create Rate Limit Manager

**File:** `internal/token/rate_limit_manager.go` (NEW FILE)

```go
package token

import (
	"sync"
	"time"

	"github.com/sunbankio/qwencoder-proxy/logging"
)

// ProviderLimits defines rate limits for each provider
type ProviderLimits struct {
	MaxRequestsPerDay   int
	MaxRequestsPerMinute int
}

// TokenUsage tracks usage for a single token
type TokenUsage struct {
	DailyCount  int
	MinuteCount int
	LastUsedDay int // Unix day (time.Now().Unix() / 86400)
	LastUsedMin int // Unix minute (time.Now().Unix() / 60)
}

// RateLimitManager manages rate limiting per token
type RateLimitManager struct {
	limits     map[ProviderType]ProviderLimits
	tokenUsage map[string]*TokenUsage // tokenID -> TokenUsage
	mu         sync.RWMutex
	logger     logging.Logger
}

// NewRateLimitManager creates a new rate limit manager
func NewRateLimitManager(logger logging.Logger) *RateLimitManager {
	if logger == nil {
		logger = logging.NewLogger()
	}

	return &RateLimitManager{
		limits: map[ProviderType]ProviderLimits{
			ProviderGeminiCLI:   {MaxRequestsPerDay: 1000, MaxRequestsPerMinute: 60},
			ProviderQwen:        {MaxRequestsPerDay: 2000, MaxRequestsPerMinute: 0},
			ProviderKiro:        {MaxRequestsPerDay: 0, MaxRequestsPerMinute: 0}, // No limits yet
			ProviderAntigravity: {MaxRequestsPerDay: 0, MaxRequestsPerMinute: 0},
			ProviderIFlow:       {MaxRequestsPerDay: 0, MaxRequestsPerMinute: 0},
		},
		tokenUsage: make(map[string]*TokenUsage),
		logger:     logger,
	}
}

// CheckRateLimit checks if a token can make a request
// Returns true if allowed, false if rate limited
func (rm *RateLimitManager) CheckRateLimit(providerType ProviderType, tokenID string) (bool, error) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	limits, ok := rm.limits[providerType]
	if !ok || (limits.MaxRequestsPerDay == 0 && limits.MaxRequestsPerMinute == 0) {
		// No limits defined for this provider
		return true, nil
	}

	now := time.Now()
	currentDay := int(now.Unix() / 86400)
	currentMin := int(now.Unix() / 60)

	usage, exists := rm.tokenUsage[tokenID]
	if !exists {
		usage = &TokenUsage{}
		rm.tokenUsage[tokenID] = usage
	}

	// Reset daily counter if day changed
	if usage.LastUsedDay != currentDay {
		usage.DailyCount = 0
		usage.LastUsedDay = currentDay
	}

	// Reset minute counter if minute changed
	if usage.LastUsedMin != currentMin {
		usage.MinuteCount = 0
		usage.LastUsedMin = currentMin
	}

	// Check daily limit
	if limits.MaxRequestsPerDay > 0 && usage.DailyCount >= limits.MaxRequestsPerDay {
		rm.logger.WarningLog("[RateLimit] Token %s exceeded daily limit (%d/%d)",
			tokenID, usage.DailyCount, limits.MaxRequestsPerDay)
		return false, nil
	}

	// Check minute limit
	if limits.MaxRequestsPerMinute > 0 && usage.MinuteCount >= limits.MaxRequestsPerMinute {
		rm.logger.WarningLog("[RateLimit] Token %s exceeded minute limit (%d/%d)",
			tokenID, usage.MinuteCount, limits.MaxRequestsPerMinute)
		return false, nil
	}

	// Increment counters
	usage.DailyCount++
	usage.MinuteCount++

	return true, nil
}

// GetTokenUsage returns current usage for a token
func (rm *RateLimitManager) GetTokenUsage(tokenID string) (daily, minute int, ok bool) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	usage, exists := rm.tokenUsage[tokenID]
	if !exists {
		return 0, 0, false
	}
	return usage.DailyCount, usage.MinuteCount, true
}

// GetProviderLimits returns the limits for a provider
func (rm *RateLimitManager) GetProviderLimits(providerType ProviderType) (daily, minute int, ok bool) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	limits, exists := rm.limits[providerType]
	if !exists {
		return 0, 0, false
	}
	return limits.MaxRequestsPerDay, limits.MaxRequestsPerMinute, true
}

// SetProviderLimits sets custom limits for a provider
func (rm *RateLimitManager) SetProviderLimits(providerType ProviderType, daily, minute int) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if rm.limits == nil {
		rm.limits = make(map[ProviderType]ProviderLimits)
	}
	rm.limits[providerType] = ProviderLimits{
		MaxRequestsPerDay:   daily,
		MaxRequestsPerMinute: minute,
	}
	rm.logger.InfoLog("[RateLimit] Updated limits for %s: %d/day, %d/min", providerType, daily, minute)
}

// ResetTokenUsage resets usage for a specific token
func (rm *RateLimitManager) ResetTokenUsage(tokenID string) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	delete(rm.tokenUsage, tokenID)
	rm.logger.InfoLog("[RateLimit] Reset usage for token %s", tokenID)
}

// ResetAllUsage resets all usage counters
func (rm *RateLimitManager) ResetAllUsage() {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	rm.tokenUsage = make(map[string]*TokenUsage)
	rm.logger.InfoLog("[RateLimit] Reset all usage counters")
}

// GetStats returns statistics about rate limiting
func (rm *RateLimitManager) GetStats() map[string]interface{} {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	return map[string]interface{}{
		"total_tokens_tracked": len(rm.tokenUsage),
		"providers_with_limits": len(rm.limits),
	}
}
```

### Step 1.2: Integrate Rate Limit Manager into TokenManager

**File:** `internal/token/token_selection.go` (MODIFY)

#### 1.2.1: Add RateLimitManager field to TokenManager

Find the `TokenManager` struct definition and add the rate limit manager:

```go
// TokenManager manages tokens for a single provider
type TokenManager struct {
	store              *MultiTokenStore
	selectionStrategy  SelectionStrategy
	logger             logging.Logger
	rateLimitManager   *RateLimitManager  // ADD THIS LINE
	mu                 sync.RWMutex
	// ... existing fields ...
}
```

#### 1.2.2: Initialize RateLimitManager in NewTokenManager

Find the `NewTokenManager` function and initialize the rate limit manager:

```go
// NewTokenManager creates a new token manager
func NewTokenManager(store *MultiTokenStore, logger logging.Logger) *TokenManager {
	if logger == nil {
		logger = logging.NewLogger()
	}

	tm := &TokenManager{
		store:             store,
		logger:            logger,
		rateLimitManager:  NewRateLimitManager(logger), // ADD THIS LINE
		mu:                sync.RWMutex{},
	}

	// Set default selection strategy
	strategy := store.Settings.SelectionStrategy
	if strategy == "" {
		strategy = DefaultSelectionStrategy
	}
	tm.selectionStrategy = NewStrategyFactory().CreateStrategy(strategy)

	return tm
}
```

#### 1.2.3: Modify SelectTokenWithClient to Check Rate Limits

Find the `SelectTokenWithClient` method and add rate limit checking:

```go
// SelectTokenWithClient selects a token and returns both the token and an HTTP client
// configured with the token's proxy settings. This is the preferred method for
// providers that need proxy-aware token selection.
func (tm *TokenManager) SelectTokenWithClient() (*ProviderToken, *http.Client, error) {
	tm.mu.RLock()
	tokens := tm.store.Tokens
	tm.mu.RUnlock()

	if len(tokens) == 0 {
		return nil, nil, ErrNoTokensAvailable
	}

	// Try to select a token that is not rate limited
	maxAttempts := len(tokens)
	var lastErr error

	for attempt := 0; attempt < maxAttempts; attempt++ {
		// Select a token using the configured strategy
		selectedToken, err := tm.selectionStrategy.SelectToken(tokens)
		if err != nil {
			lastErr = err
			continue
		}

		// NEW: Check rate limits before returning the token
		allowed, rateLimitErr := tm.rateLimitManager.CheckRateLimit(tm.store.ProviderID, selectedToken.ID)
		if rateLimitErr != nil {
			tm.logger.ErrorLog("[TokenManager] Rate limit check failed: %v", rateLimitErr)
			return nil, nil, rateLimitErr
		}

		if !allowed {
			tm.logger.DebugLog("[TokenManager] Token %s is rate limited, trying alternative", selectedToken.ID)
			// Remove this token from consideration for this round
			tokens = removeToken(tokens, selectedToken.ID)
			continue
		}

		// Token is available and not rate limited
		tm.logger.DebugLog("[TokenManager] Selected token %s (attempt %d/%d)", selectedToken.ID, attempt+1, maxAttempts)

		// Update last used timestamp
		tm.store.UpdateLastUsed(selectedToken.ID)

		// Get HTTP client with proxy configuration
		client := tm.getHTTPClient(selectedToken)

		return selectedToken, client, nil
	}

	// All tokens are rate limited or unavailable
	return nil, nil, fmt.Errorf("all tokens are rate limited or unavailable: %w", lastErr)
}

// Helper function to remove a token from a slice
func removeToken(tokens []ProviderToken, tokenID string) []ProviderToken {
	for i, t := range tokens {
		if t.ID == tokenID {
			return append(tokens[:i], tokens[i+1:]...)
		}
	}
	return tokens
}
```

#### 1.2.4: Add Getter for RateLimitManager

Add a method to expose the rate limit manager for API access:

```go
// GetRateLimitManager returns the rate limit manager
func (tm *TokenManager) GetRateLimitManager() *RateLimitManager {
	return tm.rateLimitManager
}
```

### Step 1.3: Add ProviderID to TokenManager

The `TokenManager` needs to know its provider type for rate limit checking. Ensure the `TokenManager` has access to the provider ID.

**File:** `internal/token/token_selection.go` (MODIFY)

```go
// TokenManager manages tokens for a single provider
type TokenManager struct {
	store              *MultiTokenStore
	selectionStrategy  SelectionStrategy
	logger             logging.Logger
	rateLimitManager   *RateLimitManager
	providerID         ProviderType  // ADD THIS LINE
	mu                 sync.RWMutex
	// ... existing fields ...
}

// NewTokenManager creates a new token manager
func NewTokenManager(store *MultiTokenStore, logger logging.Logger) *TokenManager {
	if logger == nil {
		logger = logger = logging.NewLogger()
	}

	tm := &TokenManager{
		store:             store,
		logger:            logger,
		rateLimitManager:  NewRateLimitManager(logger),
		providerID:        ProviderType(store.ProviderID),  // ADD THIS LINE
		mu:                sync.RWMutex{},
	}

	// Set default selection strategy
	strategy := store.Settings.SelectionStrategy
	if strategy == "" {
		strategy = DefaultSelectionStrategy
	}
	tm.selectionStrategy = NewStrategyFactory().CreateStrategy(strategy)

	return tm
}
```

### Step 1.4: Add Rate Limit Logging to Providers

**File:** `provider/gemini/gemini.go` (MODIFY)

Add rate limit logging after successful requests:

```go
// In GenerateContent method, after successful response:

// Log rate limit status
dailyUsed, dailyLimit, _ := p.GetTokenManager().GetRateLimitManager().GetTokenUsage(tokenID)
minuteUsed, minuteLimit, _ := p.GetTokenManager().GetRateLimitManager().GetTokenUsage(tokenID)
p.GetLogger().DebugLog("[Gemini] Rate limit status for token %s: %d/%d daily, %d/%d minute",
	tokenID, dailyUsed, dailyLimit, minuteUsed, minuteLimit)
```

**File:** `provider/qwen/qwen.go` (MODIFY)

Add similar rate limit logging:

```go
// In GenerateContent method, after successful response:

// Log rate limit status
dailyUsed, dailyLimit, _ := p.GetTokenManager().GetRateLimitManager().GetTokenUsage(tokenID)
minuteUsed, minuteLimit, _ := p.GetTokenManager().GetRateLimitManager().GetTokenUsage(tokenID)
p.GetLogger().DebugLog("[Qwen] Rate limit status for token %s: %d/%d daily, %d/%d minute",
	tokenID, dailyUsed, dailyLimit, minuteUsed, minuteLimit)
```

### Step 1.5: Add Configuration Support

**File:** `config/config.go` (MODIFY)

Add rate limit configuration:

```go
// RateLimitConfig holds rate limiting configuration
type RateLimitConfig struct {
	Enabled bool `json:"enabled"` // Enable/disable rate limiting

	// Provider-specific overrides (optional)
	GeminiCLIDailyLimit   int `json:"gemini_cli_daily_limit"`
	GeminiCLIMinuteLimit  int `json:"gemini_cli_minute_limit"`
	QwenDailyLimit        int `json:"qwen_daily_limit"`
	QwenMinuteLimit       int `json:"qwen_minute_limit"`
	KiroDailyLimit        int `json:"kiro_daily_limit"`
	KiroMinuteLimit       int `json:"kiro_minute_limit"`
	AntigravityDailyLimit int `json:"antigravity_daily_limit"`
	AntigravityMinuteLimit int `json:"antigravity_minute_limit"`
	IFlowDailyLimit       int `json:"iflow_daily_limit"`
	IFlowMinuteLimit      int `json:"iflow_minute_limit"`
}

// Config holds all configuration for application
type Config struct {
	Server      ServerConfig
	HTTPClient  HTTPClientConfig
	Logging     LoggingConfig
	OAuthServer OAuthServerConfig
	RateLimit   RateLimitConfig  // ADD THIS LINE
}

// DefaultConfig returns the default configuration
func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Port: "8143",
		},
		HTTPClient: HTTPClientConfig{
			MaxIdleConns:            50,
			MaxIdleConnsPerHost:     50,
			IdleConnTimeoutSeconds:  180,
			RequestTimeoutSeconds:   300,
			StreamingTimeoutSeconds: 900,
			ReadTimeoutSeconds:      45,
		},
		Logging: LoggingConfig{
			IsDebugMode: false,
		},
		OAuthServer: OAuthServerConfig{
			Port:            "8143",
			CallbackBaseURL: "http://localhost:8143",
			StateTTL:        10 * time.Minute,
			DeviceCodeTTL:   15 * time.Minute,
			EnableCORS:      false,
			AllowedOrigins:  []string{"*"},
		},
		RateLimit: RateLimitConfig{
			Enabled: true,
			// Use defaults (0 means use hardcoded limits)
			GeminiCLIDailyLimit:   0,
			GeminiCLIMinuteLimit:  0,
			QwenDailyLimit:        0,
			QwenMinuteLimit:       0,
			KiroDailyLimit:        0,
			KiroMinuteLimit:       0,
			AntigravityDailyLimit: 0,
			AntigravityMinuteLimit: 0,
			IFlowDailyLimit:       0,
			IFlowMinuteLimit:      0,
		},
	}
}
```

---

## Phase 2: API Integration

### Step 2.1: Add Rate Limit Info to API Responses

**File:** `restapi/rest_api.go` (MODIFY)

Update the `ProviderTokenInfo` struct to include rate limit information:

```go
// ProviderTokenInfo represents token information for API responses
type ProviderTokenInfo struct {
	ID          string  `json:"id"`
	Email       string  `json:"email"`
	ExpiryDate  int64   `json:"expiry_date"`
	ExpiresIn   int64   `json:"expires_in"`
	TokenType   string  `json:"token_type"`
	Healthy     bool    `json:"healthy"`
	HealthScore float64 `json:"health_score"`
	LastUsed    int64   `json:"last_used"`
	CreatedAt   int64   `json:"created_at"`
	ErrorCount  int     `json:"error_count"`

	// Rate limit information (NEW)
	DailyRequestsUsed   int `json:"daily_requests_used"`
	DailyRequestsLimit  int `json:"daily_requests_limit"`
	MinuteRequestsUsed  int `json:"minute_requests_used"`
	MinuteRequestsLimit int `json:"minute_requests_limit"`
}
```

### Step 2.2: Update Token Info Endpoint

**File:** `restapi/rest_api.go` (MODIFY)

Find the endpoint that returns token information and update it to include rate limit data:

```go
// In the function that returns ProviderTokenInfo, add rate limit data:

// Get rate limit information
rateLimitMgr := tokenManager.GetRateLimitManager()
dailyUsed, _ := rateLimitMgr.GetTokenUsage(token.ID)
minuteUsed, _ := rateLimitMgr.GetTokenUsage(token.ID)
dailyLimit, _, _ := rateLimitMgr.GetProviderLimits(ProviderType(providerID))
minuteLimit, _, _ := rateLimitMgr.GetProviderLimits(ProviderType(providerID))

tokenInfo := ProviderTokenInfo{
	ID:          token.ID,
	Email:       token.Email,
	ExpiryDate:  token.ExpiryDate,
	ExpiresIn:   token.ExpiryDate - time.Now().Unix(),
	TokenType:   token.TokenType,
	Healthy:     token.Healthy,
	HealthScore: token.HealthScore,
	LastUsed:    token.LastUsed,
	CreatedAt:   token.CreatedAt,
	ErrorCount:  token.ErrorCount,

	// Rate limit information
	DailyRequestsUsed:   dailyUsed,
	DailyRequestsLimit:  dailyLimit,
	MinuteRequestsUsed:  minuteUsed,
	MinuteRequestsLimit: minuteLimit,
}
```

### Step 2.3: Add Rate Limit Status Endpoint

**File:** `restapi/rest_api.go` (MODIFY)

Add a new endpoint to get overall rate limit statistics:

```go
// handleRateLimitStats returns rate limit statistics for all providers
func (s *Server) handleRateLimitStats(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	stats := make(map[string]interface{})

	for providerID := range s.tokenManagers {
		tm := s.tokenManagers[providerID]
		rlm := tm.GetRateLimitManager()

		dailyLimit, minuteLimit, _ := rlm.GetProviderLimits(ProviderType(providerID))
		rlmStats := rlm.GetStats()

		stats[providerID] = map[string]interface{}{
			"daily_limit":  dailyLimit,
			"minute_limit": minuteLimit,
			"stats":        rlmStats,
		}
	}

	json.NewEncoder(w).Encode(stats)
}

// Add this to the router setup:
// r.HandleFunc("/api/ratelimit/stats", s.handleRateLimitStats).Methods("GET")
```

### Step 2.4: Add Rate Limit Reset Endpoint

**File:** `restapi/rest_api.go` (MODIFY)

Add an endpoint to reset rate limit counters (admin only):

```go
// handleResetRateLimit resets rate limit counters for a specific token or all tokens
func (s *Server) handleResetRateLimit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		TokenID string `json:"token_id"` // If empty, reset all tokens
		All     bool   `json:"all"`     // If true, reset all tokens
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	providerID := r.URL.Query().Get("provider")
	if providerID == "" {
		http.Error(w, "Provider ID required", http.StatusBadRequest)
		return
	}

	tm, ok := s.tokenManagers[providerID]
	if !ok {
		http.Error(w, "Provider not found", http.StatusNotFound)
		return
	}

	rlm := tm.GetRateLimitManager()

	if req.All || req.TokenID == "" {
		rlm.ResetAllUsage()
		s.logger.InfoLog("[API] Reset all rate limit counters for provider %s", providerID)
	} else {
		rlm.ResetTokenUsage(req.TokenID)
		s.logger.InfoLog("[API] Reset rate limit counter for token %s in provider %s", req.TokenID, providerID)
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// Add this to the router setup:
// r.HandleFunc("/api/ratelimit/reset", s.handleResetRateLimit).Methods("POST")
```

---

## Phase 3: Advanced Features (Optional)

### Step 3.1: Add Persistence to Disk

**File:** `internal/token/rate_limit_manager.go` (MODIFY)

Add methods to save and load rate limit data:

```go
// SaveToFile saves rate limit data to a JSON file
func (rm *RateLimitManager) SaveToFile(filePath string) error {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	data := map[string]interface{}{
		"limits":     rm.limits,
		"tokenUsage": rm.tokenUsage,
		"savedAt":    time.Now().Unix(),
	}

	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal rate limit data: %w", err)
	}

	if err := os.WriteFile(filePath, jsonData, 0644); err != nil {
		return fmt.Errorf("failed to write rate limit data: %w", err)
	}

	rm.logger.InfoLog("[RateLimit] Saved rate limit data to %s", filePath)
	return nil
}

// LoadFromFile loads rate limit data from a JSON file
func (rm *RateLimitManager) LoadFromFile(filePath string) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	jsonData, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			rm.logger.DebugLog("[RateLimit] No existing rate limit data file at %s", filePath)
			return nil
		}
		return fmt.Errorf("failed to read rate limit data: %w", err)
	}

	var data struct {
		Limits     map[ProviderType]ProviderLimits `json:"limits"`
		TokenUsage map[string]*TokenUsage          `json:"tokenUsage"`
		SavedAt    int64                          `json:"savedAt"`
	}

	if err := json.Unmarshal(jsonData, &data); err != nil {
		return fmt.Errorf("failed to unmarshal rate limit data: %w", err)
	}

	// Check if data is too old (more than 1 day)
	age := time.Now().Unix() - data.SavedAt
	if age > 86400 {
		rm.logger.WarningLog("[RateLimit] Rate limit data is %d seconds old, ignoring", age)
		return nil
	}

	// Merge limits
	for provider, limits := range data.Limits {
		rm.limits[provider] = limits
	}

	// Merge token usage
	for tokenID, usage := range data.TokenUsage {
		rm.tokenUsage[tokenID] = usage
	}

	rm.logger.InfoLog("[RateLimit] Loaded rate limit data from %s", filePath)
	return nil
}
```

### Step 3.2: Add Periodic Persistence

**File:** `internal/token/rate_limit_manager.go` (MODIFY)

Add automatic periodic saving:

```go
// StartPeriodicSave starts a background goroutine to periodically save rate limit data
func (rm *RateLimitManager) StartPeriodicSave(filePath string, interval time.Duration) {
	ticker := time.NewTicker(interval)
	go func() {
		for range ticker.C {
			if err := rm.SaveToFile(filePath); err != nil {
				rm.logger.ErrorLog("[RateLimit] Failed to save rate limit data: %v", err)
			}
		}
	}()
	rm.logger.InfoLog("[RateLimit] Started periodic save every %v to %s", interval, filePath)
}

// StopPeriodicSave stops the periodic save (not implemented in this simple version)
// In production, you'd use context.Context for graceful shutdown
```

### Step 3.3: Add Rate Limit Warnings

**File:** `internal/token/rate_limit_manager.go` (MODIFY)

Add warning thresholds:

```go
// CheckRateLimitWithWarning checks if a token can make a request and logs warnings
// when approaching limits
func (rm *RateLimitManager) CheckRateLimitWithWarning(providerType ProviderType, tokenID string, warningThreshold float64) (bool, error) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	limits, ok := rm.limits[providerType]
	if !ok || (limits.MaxRequestsPerDay == 0 && limits.MaxRequestsPerMinute == 0) {
		return true, nil
	}

	now := time.Now()
	currentDay := int(now.Unix() / 86400)
	currentMin := int(now.Unix() / 60)

	usage, exists := rm.tokenUsage[tokenID]
	if !exists {
		usage = &TokenUsage{}
		rm.tokenUsage[tokenID] = usage
	}

	// Reset counters if time window changed
	if usage.LastUsedDay != currentDay {
		usage.DailyCount = 0
		usage.LastUsedDay = currentDay
	}
	if usage.LastUsedMin != currentMin {
		usage.MinuteCount = 0
		usage.LastUsedMin = currentMin
	}

	// Check and warn for daily limit
	if limits.MaxRequestsPerDay > 0 {
		dailyRatio := float64(usage.DailyCount) / float64(limits.MaxRequestsPerDay)
		if dailyRatio >= 1.0 {
			rm.logger.WarningLog("[RateLimit] Token %s exceeded daily limit (%d/%d)",
				tokenID, usage.DailyCount, limits.MaxRequestsPerDay)
			return false, nil
		} else if dailyRatio >= warningThreshold {
			rm.logger.WarningLog("[RateLimit] Token %s approaching daily limit (%.1f%% used, %d/%d)",
				tokenID, dailyRatio*100, usage.DailyCount, limits.MaxRequestsPerDay)
		}
	}

	// Check and warn for minute limit
	if limits.MaxRequestsPerMinute > 0 {
		minuteRatio := float64(usage.MinuteCount) / float64(limits.MaxRequestsPerMinute)
		if minuteRatio >= 1.0 {
			rm.logger.WarningLog("[RateLimit] Token %s exceeded minute limit (%d/%d)",
				tokenID, usage.MinuteCount, limits.MaxRequestsPerMinute)
			return false, nil
		} else if minuteRatio >= warningThreshold {
			rm.logger.WarningLog("[RateLimit] Token %s approaching minute limit (%.1f%% used, %d/%d)",
				tokenID, minuteRatio*100, usage.MinuteCount, limits.MaxRequestsPerMinute)
		}
	}

	// Increment counters
	usage.DailyCount++
	usage.MinuteCount++

	return true, nil
}
```

### Step 3.4: Add Sliding Window Rate Limiting (Advanced)

For more precise rate limiting, implement a sliding window algorithm:

```go
// SlidingWindowRateLimitManager implements sliding window rate limiting
type SlidingWindowRateLimitManager struct {
	limits     map[ProviderType]ProviderLimits
	requests   map[string][]int64 // tokenID -> list of request timestamps
	mu         sync.RWMutex
	logger     logging.Logger
}

// CheckRateLimitSlidingWindow checks rate limit using sliding window algorithm
func (rm *SlidingWindowRateLimitManager) CheckRateLimitSlidingWindow(providerType ProviderType, tokenID string) (bool, error) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	limits, ok := rm.limits[providerType]
	if !ok || (limits.MaxRequestsPerDay == 0 && limits.MaxRequestsPerMinute == 0) {
		return true, nil
	}

	now := time.Now().Unix()

	// Initialize request list if needed
	if _, exists := rm.requests[tokenID]; !exists {
		rm.requests[tokenID] = []int64{}
	}

	requests := rm.requests[tokenID]

	// Remove requests outside the time windows
	dayAgo := now - 86400
	minuteAgo := now - 60

	var validRequests []int64
	for _, ts := range requests {
		if ts > dayAgo {
			validRequests = append(validRequests, ts)
		}
	}
	requests = validRequests

	// Count requests in each window
	var dailyCount, minuteCount int
	for _, ts := range requests {
		if ts > minuteAgo {
			minuteCount++
		}
		dailyCount++
	}

	// Check limits
	if limits.MaxRequestsPerDay > 0 && dailyCount >= limits.MaxRequestsPerDay {
		return false, nil
	}
	if limits.MaxRequestsPerMinute > 0 && minuteCount >= limits.MaxRequestsPerMinute {
		return false, nil
	}

	// Add current request
	requests = append(requests, now)
	rm.requests[tokenID] = requests

	return true, nil
}
```

---

## Testing Strategy

### Unit Tests

**File:** `internal/token/rate_limit_manager_test.go` (NEW FILE)

```go
package token

import (
	"testing"
	"time"

	"github.com/sunbankio/qwencoder-proxy/logging"
)

func TestRateLimitManager_CheckRateLimit(t *testing.T) {
	logger := logging.NewLogger()
	rlm := NewRateLimitManager(logger)

	tests := []struct {
		name         string
		providerType ProviderType
		tokenID      string
		requests     int
		wantAllowed  bool
	}{
		{
			name:         "Gemini CLI within daily limit",
			providerType: ProviderGeminiCLI,
			tokenID:      "test-token-1",
			requests:     500,
			wantAllowed:  true,
		},
		{
			name:         "Gemini CLI exceeds daily limit",
			providerType: ProviderGeminiCLI,
			tokenID:      "test-token-2",
			requests:     1001,
			wantAllowed:  false,
		},
		{
			name:         "Gemini CLI within minute limit",
			providerType: ProviderGeminiCLI,
			tokenID:      "test-token-3",
			requests:     30,
			wantAllowed:  true,
		},
		{
			name:         "Gemini CLI exceeds minute limit",
			providerType: ProviderGeminiCLI,
			tokenID:      "test-token-4",
			requests:     61,
			wantAllowed:  false,
		},
		{
			name:         "Qwen within daily limit",
			providerType: ProviderQwen,
			tokenID:      "test-token-5",
			requests:     1000,
			wantAllowed:  true,
		},
		{
			name:         "Qwen exceeds daily limit",
			providerType: ProviderQwen,
			tokenID:      "test-token-6",
			requests:     2001,
			wantAllowed:  false,
		},
		{
			name:         "Provider with no limits",
			providerType: ProviderKiro,
			tokenID:      "test-token-7",
			requests:     10000,
			wantAllowed:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset manager for each test
			rlm = NewRateLimitManager(logger)

			// Make requests
			for i := 0; i < tt.requests; i++ {
				allowed, err := rlm.CheckRateLimit(tt.providerType, tt.tokenID)
				if err != nil {
					t.Errorf("CheckRateLimit() error = %v", err)
					return
				}
				if i == tt.requests-1 && allowed != tt.wantAllowed {
					t.Errorf("CheckRateLimit() last request allowed = %v, want %v", allowed, tt.wantAllowed)
				}
			}
		})
	}
}

func TestRateLimitManager_AutoReset(t *testing.T) {
	logger := logging.NewLogger()
	rlm := NewRateLimitManager(logger)

	tokenID := "test-token-auto-reset"

	// Make 1000 requests (Gemini daily limit)
	for i := 0; i < 1000; i++ {
		rlm.CheckRateLimit(ProviderGeminiCLI, tokenID)
	}

	// Next request should be rate limited
	allowed, _ := rlm.CheckRateLimit(ProviderGeminiCLI, tokenID)
	if allowed {
		t.Error("Expected rate limit to be enforced")
	}

	// Simulate day change by manipulating internal state
	// In production, this happens automatically via time.Now()
	rlm.mu.Lock()
	if usage, exists := rlm.tokenUsage[tokenID]; exists {
		usage.LastUsedDay = int(time.Now().Unix()/86400) - 1
	}
	rlm.mu.Unlock()

	// Next request should be allowed
	allowed, _ = rlm.CheckRateLimit(ProviderGeminiCLI, tokenID)
	if !allowed {
		t.Error("Expected rate limit to reset after day change")
	}
}

func TestRateLimitManager_GetTokenUsage(t *testing.T) {
	logger := logging.NewLogger()
	rlm := NewRateLimitManager(logger)

	tokenID := "test-token-usage"

	// Make some requests
	for i := 0; i < 10; i++ {
		rlm.CheckRateLimit(ProviderGeminiCLI, tokenID)
	}

	daily, minute, ok := rlm.GetTokenUsage(tokenID)
	if !ok {
		t.Error("GetTokenUsage() returned ok=false")
	}
	if daily != 10 {
		t.Errorf("GetTokenUsage() daily = %v, want 10", daily)
	}
	if minute != 10 {
		t.Errorf("GetTokenUsage() minute = %v, want 10", minute)
	}
}

func TestRateLimitManager_SetProviderLimits(t *testing.T) {
	logger := logging.NewLogger()
	rlm := NewRateLimitManager(logger)

	// Set custom limits
	rlm.SetProviderLimits(ProviderGeminiCLI, 500, 30)

	daily, minute, ok := rlm.GetProviderLimits(ProviderGeminiCLI)
	if !ok {
		t.Error("GetProviderLimits() returned ok=false")
	}
	if daily != 500 {
		t.Errorf("GetProviderLimits() daily = %v, want 500", daily)
	}
	if minute != 30 {
		t.Errorf("GetProviderLimits() minute = %v, want 30", minute)
	}

	// Test that new limits are enforced
	tokenID := "test-token-custom-limits"
	for i := 0; i < 500; i++ {
		allowed, _ := rlm.CheckRateLimit(ProviderGeminiCLI, tokenID)
		if i == 499 && !allowed {
			t.Error("Expected 500th request to be allowed")
		}
	}

	allowed, _ := rlm.CheckRateLimit(ProviderGeminiCLI, tokenID)
	if allowed {
		t.Error("Expected 501st request to be rate limited")
	}
}

func TestRateLimitManager_ResetTokenUsage(t *testing.T) {
	logger := logging.NewLogger()
	rlm := NewRateLimitManager(logger)

	tokenID := "test-token-reset"

	// Make some requests
	for i := 0; i < 100; i++ {
		rlm.CheckRateLimit(ProviderGeminiCLI, tokenID)
	}

	// Reset usage
	rlm.ResetTokenUsage(tokenID)

	// Usage should be 0
	daily, minute, ok := rlm.GetTokenUsage(tokenID)
	if !ok {
		t.Error("GetTokenUsage() returned ok=false after reset")
	}
	if daily != 0 {
		t.Errorf("GetTokenUsage() daily after reset = %v, want 0", daily)
	}
	if minute != 0 {
		t.Errorf("GetTokenUsage() minute after reset = %v, want 0", minute)
	}
}
```

### Integration Tests

**File:** `internal/token/token_manager_ratelimit_test.go` (NEW FILE)

```go
package token

import (
	"testing"
	"time"

	"github.com/sunbankio/qwencoder-proxy/logging"
)

func TestTokenManager_SelectTokenWithClient_RateLimit(t *testing.T) {
	logger := logging.NewLogger()
	store := NewMultiTokenStore("test-provider", ".credentials/test.json", logger)

	// Add multiple tokens
	tokens := []ProviderToken{
		{ID: "token-1", AccessToken: "token1", Email: "test1@example.com", Healthy: true},
		{ID: "token-2", AccessToken: "token2", Email: "test2@example.com", Healthy: true},
		{ID: "token-3", AccessToken: "token3", Email: "test3@example.com", Healthy: true},
	}

	for _, token := range tokens {
		store.AddToken(token)
	}

	tm := NewTokenManager(store, logger)

	// Set low limits for testing
	tm.GetRateLimitManager().SetProviderLimits("test-provider", 5, 2)

	// Make requests and verify rate limiting
	requestCount := 0
	for i := 0; i < 20; i++ {
		token, client, err := tm.SelectTokenWithClient()
		if err != nil {
			// Expected after rate limit is hit
			if requestCount < 5 {
				t.Errorf("Unexpected error before rate limit: %v", err)
			}
			break
		}

		if token == nil {
			t.Error("Expected non-nil token")
			break
		}

		if client == nil {
			t.Error("Expected non-nil client")
		}

		requestCount++
	}

	// Should have made exactly 5 requests (daily limit)
	if requestCount != 5 {
		t.Errorf("Expected 5 requests before rate limit, got %d", requestCount)
	}
}

func TestTokenManager_RoundRobinWithRateLimit(t *testing.T) {
	logger := logging.NewLogger()
	store := NewMultiTokenStore("test-provider", ".credentials/test-rr.json", logger)

	// Add multiple tokens
	tokens := []ProviderToken{
		{ID: "token-1", AccessToken: "token1", Email: "test1@example.com", Healthy: true},
		{ID: "token-2", AccessToken: "token2", Email: "test2@example.com", Healthy: true},
		{ID: "token-3", AccessToken: "token3", Email: "test3@example.com", Healthy: true},
	}

	for _, token := range tokens {
		store.AddToken(token)
	}

	// Set round-robin strategy
	store.Settings.SelectionStrategy = "round_robin"

	tm := NewTokenManager(store, logger)

	// Set low limits per token
	tm.GetRateLimitManager().SetProviderLimits("test-provider", 2, 1)

	// Make requests and verify round-robin with rate limiting
	tokenCounts := make(map[string]int)
	for i := 0; i < 10; i++ {
		token, _, err := tm.SelectTokenWithClient()
		if err != nil {
			// Expected after all tokens are rate limited
			break
		}

		if token != nil {
			tokenCounts[token.ID]++
		}
	}

	// Each token should have been used exactly 2 times
	for _, expectedToken := range tokens {
		if count := tokenCounts[expectedToken.ID]; count != 2 {
			t.Errorf("Token %s used %d times, expected 2", expectedToken.ID, count)
		}
	}
}
```

### Load Testing

Use a load testing tool like `hey` or `vegeta` to test rate limiting under load:

```bash
# Install hey
go install github.com/rakyll/hey@latest

# Test rate limiting with 100 concurrent requests
hey -n 1000 -c 100 -H "Content-Type: application/json" \
  -d '{"model":"gemini-1.5-pro","messages":[{"role":"user","content":"test"}]}' \
  http://localhost:8143/v1/chat/completions
```

---

## Configuration

### Environment Variables

Add to your `.env` file or `example.env`:

```env
# Rate Limiting Configuration
RATE_LIMIT_ENABLED=true

# Provider-specific limits (optional, 0 = use defaults)
GEMINI_CLI_DAILY_LIMIT=1000
GEMINI_CLI_MINUTE_LIMIT=60
QWEN_DAILY_LIMIT=2000
QWEN_MINUTE_LIMIT=0
KIRO_DAILY_LIMIT=0
KIRO_MINUTE_LIMIT=0
ANTIGRAVITY_DAILY_LIMIT=0
ANTIGRAVITY_MINUTE_LIMIT=0
IFLOW_DAILY_LIMIT=0
IFLOW_MINUTE_LIMIT=0

# Rate limit warning threshold (0.0-1.0, 0.8 = warn at 80%)
RATE_LIMIT_WARNING_THRESHOLD=0.8

# Persistence settings
RATE_LIMIT_PERSIST_ENABLED=false
RATE_LIMIT_PERSIST_INTERVAL=5m
RATE_LIMIT_PERSIST_PATH=.rate_limit_data.json
```

### Config File

Add to your config file (e.g., `config.yaml`):

```yaml
rate_limit:
  enabled: true
  warning_threshold: 0.8

  providers:
    gemini-cli:
      daily_limit: 1000
      minute_limit: 60
    qwen:
      daily_limit: 2000
      minute_limit: 0
    kiro:
      daily_limit: 0
      minute_limit: 0
    antigravity:
      daily_limit: 0
      minute_limit: 0
    iflow:
      daily_limit: 0
      minute_limit: 0

  persistence:
    enabled: false
    interval: 5m
    path: ".rate_limit_data.json"
```

---

## Future Enhancements

### 1. Distributed Rate Limiting

For multi-instance deployments, implement distributed rate limiting using Redis:

```go
// RedisRateLimitManager uses Redis for distributed rate limiting
type RedisRateLimitManager struct {
	client *redis.Client
	limits map[ProviderType]ProviderLimits
	logger logging.Logger
}

func (rm *RedisRateLimitManager) CheckRateLimit(providerType ProviderType, tokenID string) (bool, error) {
	key := fmt.Sprintf("ratelimit:%s:%s", providerType, tokenID)

	// Use Redis INCR with EXPIRE for atomic counter operations
	pipe := rm.client.Pipeline()
	dailyCmd := pipe.Incr(fmt.Sprintf("%s:daily", key))
	dailyExpire := pipe.Expire(fmt.Sprintf("%s:daily", key), 24*time.Hour)
	minuteCmd := pipe.Incr(fmt.Sprintf("%s:minute", key))
	minuteExpire := pipe.Expire(fmt.Sprintf("%s:minute", key), time.Minute)

	_, err := pipe.Exec()
	if err != nil {
		return false, err
	}

	// Check limits
	limits := rm.limits[providerType]
	if limits.MaxRequestsPerDay > 0 && dailyCmd.Val() > int64(limits.MaxRequestsPerDay) {
		return false, nil
	}
	if limits.MaxRequestsPerMinute > 0 && minuteCmd.Val() > int64(limits.MaxRequestsPerMinute) {
		return false, nil
	}

	return true, nil
}
```

### 2. Adaptive Rate Limiting

Adjust rate limits based on provider health and response times:

```go
// AdaptiveRateLimitManager adjusts limits based on provider health
type AdaptiveRateLimitManager struct {
	baseLimits      map[ProviderType]ProviderLimits
	currentLimits   map[ProviderType]ProviderLimits
	healthScores    map[string]float64 // tokenID -> health score
	responseTimes   map[string]time.Duration
	mu              sync.RWMutex
	logger          logging.Logger
}

func (rm *AdaptiveRateLimitManager) UpdateHealth(tokenID string, healthScore float64, responseTime time.Duration) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	rm.healthScores[tokenID] = healthScore
	rm.responseTimes[tokenID] = responseTime

	// Adjust limits based on health
	if healthScore < 0.5 {
		// Reduce limits for unhealthy tokens
		rm.adjustLimits(tokenID, 0.5)
	} else if healthScore > 0.9 && responseTime < 2*time.Second {
		// Increase limits for healthy, fast tokens
		rm.adjustLimits(tokenID, 1.2)
	}
}

func (rm *AdaptiveRateLimitManager) adjustLimits(tokenID string, factor float64) {
	// Implementation to adjust limits based on factor
}
```

### 3. Rate Limit Analytics

Add analytics endpoint for rate limit usage over time:

```go
// RateLimitAnalytics tracks rate limit usage over time
type RateLimitAnalytics struct {
	hourlyUsage map[string]map[int]int // tokenID -> hour -> count
	dailyUsage  map[string]map[int]int // tokenID -> day -> count
	mu          sync.RWMutex
}

func (ra *RateLimitAnalytics) RecordRequest(tokenID string) {
	ra.mu.Lock()
	defer ra.mu.Unlock()

	now := time.Now()
	hour := now.Hour()
	day := now.Day()

	if ra.hourlyUsage[tokenID] == nil {
		ra.hourlyUsage[tokenID] = make(map[int]int)
	}
	if ra.dailyUsage[tokenID] == nil {
		ra.dailyUsage[tokenID] = make(map[int]int)
	}

	ra.hourlyUsage[tokenID][hour]++
	ra.dailyUsage[tokenID][day]++
}

func (ra *RateLimitAnalytics) GetHourlyUsage(tokenID string, hours int) map[int]int {
	ra.mu.RLock()
	defer ra.mu.RUnlock()

	result := make(map[int]int)
	now := time.Now()

	for h := 0; h < hours; h++ {
		hour := (now.Hour() - h + 24) % 24
		if usage, ok := ra.hourlyUsage[tokenID]; ok {
			result[hour] = usage[hour]
		}
	}

	return result
}
```

### 4. Rate Limit Dashboard Integration

Add rate limit visualization to the web dashboard:

**File:** `web/dashboard/js/api/endpoints.js` (MODIFY)

```javascript
// Add rate limit endpoints
const rateLimitEndpoints = {
  stats: () => client.get('/api/ratelimit/stats'),
  reset: (providerId, tokenId, all) => client.post('/api/ratelimit/reset', { token_id: tokenId, all }, { params: { provider: providerId } }),
};

export { rateLimitEndpoints };
```

**File:** `web/dashboard/js/components/rate-limit-chart.js` (NEW FILE)

```javascript
export class RateLimitChart {
  constructor(container) {
    this.container = container;
    this.chart = null;
  }

  async render() {
    const stats = await rateLimitEndpoints.stats();
    // Render chart using Chart.js or similar library
    this.createChart(stats);
  }

  createChart(stats) {
    // Implementation to create rate limit visualization
  }
}
```

---

## Implementation Checklist

### Phase 1: Core Rate Limiting

- [ ] Create `internal/token/rate_limit_manager.go`
  - [ ] Implement `ProviderLimits` struct
  - [ ] Implement `TokenUsage` struct
  - [ ] Implement `RateLimitManager` struct
  - [ ] Implement `NewRateLimitManager()` function
  - [ ] Implement `CheckRateLimit()` method
  - [ ] Implement `GetTokenUsage()` method
  - [ ] Implement `GetProviderLimits()` method
  - [ ] Implement `SetProviderLimits()` method
  - [ ] Implement `ResetTokenUsage()` method
  - [ ] Implement `ResetAllUsage()` method
  - [ ] Implement `GetStats()` method

- [ ] Modify `internal/token/token_selection.go`
  - [ ] Add `rateLimitManager` field to `TokenManager`
  - [ ] Add `providerID` field to `TokenManager`
  - [ ] Initialize `RateLimitManager` in `NewTokenManager()`
  - [ ] Modify `SelectTokenWithClient()` to check rate limits
  - [ ] Add `removeToken()` helper function
  - [ ] Add `GetRateLimitManager()` getter method

- [ ] Modify `provider/gemini/gemini.go`
  - [ ] Add rate limit logging after successful requests

- [ ] Modify `provider/qwen/qwen.go`
  - [ ] Add rate limit logging after successful requests

- [ ] Modify `config/config.go`
  - [ ] Add `RateLimitConfig` struct
  - [ ] Add `RateLimit` field to `Config`
  - [ ] Update `DefaultConfig()` with rate limit defaults

- [ ] Write unit tests for `RateLimitManager`
  - [ ] Test basic rate limiting
  - [ ] Test auto-reset functionality
  - [ ] Test multiple providers
  - [ ] Test token usage tracking
  - [ ] Test custom limits
  - [ ] Test reset functionality

### Phase 2: API Integration

- [ ] Modify `restapi/rest_api.go`
  - [ ] Add rate limit fields to `ProviderTokenInfo`
  - [ ] Update token info endpoint to include rate limit data
  - [ ] Implement `handleRateLimitStats()` endpoint
  - [ ] Implement `handleResetRateLimit()` endpoint
  - [ ] Add routes to router

- [ ] Write integration tests
  - [ ] Test token selection with rate limiting
  - [ ] Test round-robin with rate limiting
  - [ ] Test API endpoints

### Phase 3: Advanced Features (Optional)

- [ ] Add persistence to disk
  - [ ] Implement `SaveToFile()` method
  - [ ] Implement `LoadFromFile()` method
  - [ ] Implement `StartPeriodicSave()` method

- [ ] Add rate limit warnings
  - [ ] Implement `CheckRateLimitWithWarning()` method
  - [ ] Configure warning threshold

- [ ] Add sliding window rate limiting (optional)
  - [ ] Implement `SlidingWindowRateLimitManager`
  - [ ] Implement `CheckRateLimitSlidingWindow()` method

- [ ] Add dashboard integration
  - [ ] Add rate limit API endpoints
  - [ ] Create rate limit chart component
  - [ ] Update dashboard UI

---

## Summary

This implementation plan provides a comprehensive approach to adding token-based rate limiting to the qwencoder-proxy with minimal overhead. The key advantages are:

1. **Minimal Overhead**: In-memory storage with O(1) counter increments
2. **Automatic Reset**: Daily and minute counters reset automatically
3. **Provider-Specific**: Easy to configure different limits per provider
4. **Extensible**: Can add persistence, sliding windows, or distributed rate limiting
5. **Thread-Safe**: Uses read-write locks for concurrent access

The implementation is divided into three phases:

- **Phase 1**: Core rate limiting functionality
- **Phase 2**: API integration for monitoring and management
- **Phase 3**: Advanced features (optional)

Start with Phase 1 to get basic rate limiting working, then proceed to Phase 2 for monitoring, and consider Phase 3 features based on your specific needs.

---

## References

- **Token Management**: [`internal/token/`](internal/token/)
- **Provider Implementations**: [`provider/`](provider/)
- **Request Flow**: [`proxy/openai_handler.go`](proxy/openai_handler.go)
- **Logging**: [`logging/logger.go`](logging/logger.go)
- **Configuration**: [`config/config.go`](config/config.go)
- **API Endpoints**: [`restapi/rest_api.go`](restapi/rest_api.go)
