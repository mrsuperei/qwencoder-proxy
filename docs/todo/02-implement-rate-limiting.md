# Implement Per-Token Rate Limiting

**Priority:** CRITICAL  
**Estimated Time:** 2 weeks  
**Complexity:** High  
**Files to Create:** 3  
**Files to Modify:** 5

---

## Problem Description

The qwencoder-proxy currently lacks any rate limiting mechanism, which is critical for:
- Managing API quotas per token
- Preventing abuse of individual tokens
- Implementing fair usage across multiple tokens
- Enforcing provider-specific rate limits
- Providing rate limit feedback to clients

### Current State

**Missing Components:**
- No rate limiting middleware
- No token usage tracking
- No quota management
- No rate limit error responses
- No rate limit headers in responses

**Existing Infrastructure (Ready for Rate Limiting):**
- Token selection mechanism ([`internal/token/token_selection.go`](qwencoder-proxy/internal/token/token_selection.go))
- Token health tracking ([`internal/token/proxy_health_tracker.go`](qwencoder-proxy/internal/token/proxy_health_tracker.go))
- SQLite storage with efficient querying ([`internal/token/sqlite_store.go`](qwencoder-proxy/internal/token/sqlite_store.go))
- Middleware layer ([`internal/restapi/middleware.go`](qwencoder-proxy/internal/restapi/middleware.go))

---

## Solution Architecture

### Component Overview

```
internal/ratelimit/
├── rate_limiter.go       # RateLimiter interface and implementations
├── middleware.go          # HTTP middleware for rate limiting
├── strategies.go          # Rate limiting strategies (token-based, request-based, hybrid)
├── tracker.go             # Token usage tracking
└── errors.go             # Rate limit error types
```

### Integration Points

1. **Middleware Layer:** Add rate limiting check before provider requests
2. **Token Selection:** Filter rate-limited tokens during selection
3. **Database:** Track usage per token
4. **Response Headers:** Add rate limit information to responses

---

## Implementation Plan

### Phase 1: Create Rate Limiting Package (Days 1-3)

#### Step 1.1: Create Rate Limiter Interface

**New File:** `internal/ratelimit/rate_limiter.go`

```go
package ratelimit

import (
	"context"
	"time"
)

// RateLimiter defines the interface for rate limiting operations
type RateLimiter interface {
	// Allow checks if a request is allowed for the given token
	Allow(ctx context.Context, tokenID string) (bool, time.Duration, error)
	
	// RecordUsage records token usage after a successful request
	RecordUsage(ctx context.Context, tokenID string, tokensUsed int) error
	
	// GetRemaining returns the remaining quota for a token
	GetRemaining(ctx context.Context, tokenID string) (int, error)
	
	// Reset clears rate limit state for a token
	Reset(ctx context.Context, tokenID string) error
}

// RateLimitConfig holds configuration for rate limiting
type RateLimitConfig struct {
	// RequestsPerMinute limits the number of requests per minute
	RequestsPerMinute int
	
	// TokensPerMinute limits the number of tokens processed per minute
	TokensPerMinute int
	
	// BurstSize allows temporary bursts above the rate limit
	BurstSize int
	
	// WindowSize defines the sliding window size in minutes
	WindowSize int
}

// DefaultRateLimitConfig returns sensible defaults
func DefaultRateLimitConfig() RateLimitConfig {
	return RateLimitConfig{
		RequestsPerMinute: 60,
		TokensPerMinute:  90000,
		BurstSize:        10,
		WindowSize:        1,
	}
}
```

#### Step 1.2: Create Token-Based Rate Limiter

**New File:** `internal/ratelimit/rate_limiter.go` (continued)

```go
package ratelimit

import (
	"context"
	"sync"
	"time"
	"github.com/sunbankio/qwencoder-proxy/internal/token"
)

// TokenRateLimiter implements per-token rate limiting using sliding window
type TokenRateLimiter struct {
	config     RateLimitConfig
	store      token.TokenStore
	logger     logging.Logger
	mu         sync.RWMutex
	tokenStates map[string]*TokenState
}

// TokenState tracks rate limit state for a single token
type TokenState struct {
	requests  []time.Time  // Sliding window of request timestamps
	tokensUsed int          // Tokens used in current window
	lastReset  time.Time     // Last time window was reset
}

// NewTokenRateLimiter creates a new token-based rate limiter
func NewTokenRateLimiter(store token.TokenStore, logger logging.Logger, config RateLimitConfig) *TokenRateLimiter {
	return &TokenRateLimiter{
		config:     config,
		store:      store,
		logger:     logger,
		tokenStates: make(map[string]*TokenState),
	}
}

// Allow checks if a request is allowed for the given token
func (trl *TokenRateLimiter) Allow(ctx context.Context, tokenID string) (bool, time.Duration, error) {
	trl.mu.Lock()
	defer trl.mu.Unlock()
	
	// Get or create token state
	state, exists := trl.tokenStates[tokenID]
	if !exists {
		state = &TokenState{
			requests:  make([]time.Time, 0, trl.config.BurstSize),
			lastReset: time.Now(),
		}
		trl.tokenStates[tokenID] = state
	}
	
	now := time.Now()
	
	// Check if window needs reset
	if now.Sub(state.lastReset) >= time.Duration(trl.config.WindowSize)*time.Minute {
		state.requests = state.requests[:0]
		state.tokensUsed = 0
		state.lastReset = now
	}
	
	// Check request rate limit
	if len(state.requests) >= trl.config.RequestsPerMinute {
		trl.logger.DebugLog("[RateLimit] Token %s exceeded request rate limit", tokenID)
		return false, time.Minute, nil
	}
	
	// Check token rate limit
	if state.tokensUsed >= trl.config.TokensPerMinute {
		trl.logger.DebugLog("[RateLimit] Token %s exceeded token rate limit", tokenID)
		return false, time.Minute, nil
	}
	
	// Add current request to window
	state.requests = append(state.requests, now)
	
	return true, 0, nil
}

// RecordUsage records token usage after a successful request
func (trl *TokenRateLimiter) RecordUsage(ctx context.Context, tokenID string, tokensUsed int) error {
	trl.mu.Lock()
	defer trl.mu.Unlock()
	
	state, exists := trl.tokenStates[tokenID]
	if !exists {
		return nil // State will be created on next Allow() call
	}
	
	state.tokensUsed += tokensUsed
	trl.logger.DebugLog("[RateLimit] Recorded %d tokens for token %s (total: %d)", 
		tokensUsed, tokenID, state.tokensUsed)
	
	return nil
}

// GetRemaining returns the remaining quota for a token
func (trl *TokenRateLimiter) GetRemaining(ctx context.Context, tokenID string) (int, error) {
	trl.mu.RLock()
	defer trl.mu.RUnlock()
	
	state, exists := trl.tokenStates[tokenID]
	if !exists {
		return trl.config.TokensPerMinute, nil
	}
	
	remaining := trl.config.TokensPerMinute - state.tokensUsed
	if remaining < 0 {
		remaining = 0
	}
	
	return remaining, nil
}

// Reset clears rate limit state for a token
func (trl *TokenRateLimiter) Reset(ctx context.Context, tokenID string) error {
	trl.mu.Lock()
	defer trl.mu.Unlock()
	
	delete(trl.tokenStates, tokenID)
	trl.logger.InfoLog("[RateLimit] Reset rate limit state for token %s", tokenID)
	
	return nil
}
```

#### Step 1.3: Create Rate Limiting Middleware

**New File:** `internal/ratelimit/middleware.go`

```go
package ratelimit

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"
	"github.com/sunbankio/qwencoder-proxy/internal/logging"
)

// RateLimitMiddleware creates HTTP middleware for rate limiting
func RateLimitMiddleware(limiter RateLimiter, logger logging.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Extract token ID from request context
			tokenID := r.Context().Value("token_id")
			if tokenID == nil || tokenID == "" {
				// No token ID in context, skip rate limiting
				next.ServeHTTP(w, r)
				return
			}
			
			tokenIDStr := tokenID.(string)
			
			// Check rate limit
			allowed, retryAfter, err := limiter.Allow(r.Context(), tokenIDStr)
			if err != nil {
				logger.ErrorLog("[RateLimitMiddleware] Error checking rate limit: %v", err)
				// On error, allow request but log
				next.ServeHTTP(w, r)
				return
			}
			
			// Set rate limit headers
			remaining, _ := limiter.GetRemaining(r.Context(), tokenIDStr)
			w.Header().Set("X-RateLimit-Limit", strconv.Itoa(60)) // Requests per minute
			w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))
			w.Header().Set("X-RateLimit-Reset", time.Now().Add(time.Minute).Format(time.RFC3339))
			
			if !allowed {
				// Rate limit exceeded
				logger.WarnLog("[RateLimitMiddleware] Rate limit exceeded for token %s", tokenIDStr)
				
				w.Header().Set("Retry-After", retryAfter.String())
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				
				errorResponse := map[string]interface{}{
					"error": map[string]interface{}{
						"code":    "rate_limit_exceeded",
						"message": "Rate limit exceeded. Please retry later.",
						"retry_after": retryAfter.String(),
					},
				}
				
				w.Write([]byte(fmt.Sprintf(`{"error":{"code":"rate_limit_exceeded","message":"Rate limit exceeded","retry_after":"%s"}}`, 
					retryAfter.String()))
				return
			}
			
			// Rate limit not exceeded, proceed with request
			next.ServeHTTP(w, r)
		})
	}
}
```

#### Step 1.4: Create Rate Limiting Strategies

**New File:** `internal/ratelimit/strategies.go`

```go
package ratelimit

import (
	"context"
	"time"
)

// StrategyType defines the type of rate limiting strategy
type StrategyType string

const (
	StrategyTokenBased   StrategyType = "token_based"
	StrategyRequestBased StrategyType = "request_based"
	StrategyHybrid      StrategyType = "hybrid"
)

// RateLimitStrategy defines the strategy for rate limiting
type RateLimitStrategy interface {
	// Type returns the strategy type
	Type() StrategyType
	
	// Check checks if usage is within limits
	Check(ctx context.Context, usage *UsageMetrics) bool
	
	// GetLimit returns the configured limit
	GetLimit() RateLimitConfig
}

// UsageMetrics tracks usage metrics for rate limiting
type UsageMetrics struct {
	RequestCount int
	TokensUsed   int
	WindowStart  time.Time
	WindowEnd    time.Time
}

// TokenBasedStrategy implements token-based rate limiting
type TokenBasedStrategy struct {
	config RateLimitConfig
}

func NewTokenBasedStrategy(config RateLimitConfig) *TokenBasedStrategy {
	return &TokenBasedStrategy{config: config}
}

func (t *TokenBasedStrategy) Type() StrategyType {
	return StrategyTokenBased
}

func (t *TokenBasedStrategy) Check(ctx context.Context, usage *UsageMetrics) bool {
	return usage.TokensUsed < t.config.TokensPerMinute
}

func (t *TokenBasedStrategy) GetLimit() RateLimitConfig {
	return t.config
}

// RequestBasedStrategy implements request-based rate limiting
type RequestBasedStrategy struct {
	config RateLimitConfig
}

func NewRequestBasedStrategy(config RateLimitConfig) *RequestBasedStrategy {
	return &RequestBasedStrategy{config: config}
}

func (r *RequestBasedStrategy) Type() StrategyType {
	return StrategyRequestBased
}

func (r *RequestBasedStrategy) Check(ctx context.Context, usage *UsageMetrics) bool {
	return usage.RequestCount < r.config.RequestsPerMinute
}

func (r *RequestBasedStrategy) GetLimit() RateLimitConfig {
	return r.config
}

// HybridStrategy implements hybrid rate limiting (both token and request)
type HybridStrategy struct {
	config RateLimitConfig
}

func NewHybridStrategy(config RateLimitConfig) *HybridStrategy {
	return &HybridStrategy{config: config}
}

func (h *HybridStrategy) Type() StrategyType {
	return StrategyHybrid
}

func (h *HybridStrategy) Check(ctx context.Context, usage *UsageMetrics) bool {
	return usage.TokensUsed < h.config.TokensPerMinute && 
	       usage.RequestCount < h.config.RequestsPerMinute
}

func (h *HybridStrategy) GetLimit() RateLimitConfig {
	return h.config
}
```

#### Step 1.5: Create Rate Limit Errors

**New File:** `internal/ratelimit/errors.go`

```go
package ratelimit

import "fmt"

// ErrorCode defines rate limit error codes
type ErrorCode string

const (
	ErrCodeRateLimitExceeded ErrorCode = "rate_limit_exceeded"
	ErrCodeInvalidTokenID    ErrorCode = "invalid_token_id"
	ErrCodeLimiterError      ErrorCode = "limiter_error"
)

// RateLimitError represents a rate limiting error
type RateLimitError struct {
	Code       ErrorCode
	Message    string
	TokenID    string
	RetryAfter time.Duration
}

func (e *RateLimitError) Error() string {
	return fmt.Sprintf("[%s] %s (token: %s, retry_after: %s)", 
		e.Code, e.Message, e.TokenID, e.RetryAfter)
}

func NewRateLimitExceededError(tokenID string, retryAfter time.Duration) *RateLimitError {
	return &RateLimitError{
		Code:       ErrCodeRateLimitExceeded,
		Message:    "Rate limit exceeded for this token",
		TokenID:    tokenID,
		RetryAfter: retryAfter,
	}
}

func NewInvalidTokenIDError(tokenID string) *RateLimitError {
	return &RateLimitError{
		Code:       ErrCodeInvalidTokenID,
		Message:    "Invalid token ID provided",
		TokenID:    tokenID,
		RetryAfter: 0,
	}
}
```

### Phase 2: Database Schema Updates (Days 4-5)

#### Step 2.1: Add Token Usage Table

**Modify File:** `internal/token/sqlite_store.go`

**Location:** After existing table creation constants (around line 90)

```go
// Add to SQL statements section
const (
	// ... existing constants ...
	
	sqlCreateTokenUsageTable = `
		CREATE TABLE IF NOT EXISTS token_usage (
			id TEXT PRIMARY KEY,
			token_id TEXT NOT NULL,
			request_count INTEGER NOT NULL DEFAULT 0,
			tokens_used INTEGER NOT NULL DEFAULT 0,
			window_start INTEGER NOT NULL,
			window_end INTEGER NOT NULL,
			created_at INTEGER NOT NULL DEFAULT (strftime('%s', 'subsec') * 1000),
			updated_at INTEGER NOT NULL DEFAULT (strftime('%s', 'subsec') * 1000),
			FOREIGN KEY (token_id) REFERENCES tokens(id) ON DELETE CASCADE
		)
	`
	
	sqlCreateIndexTokenUsageTokenID = "CREATE INDEX IF NOT EXISTS idx_token_usage_token_id ON token_usage(token_id)"
	sqlCreateIndexTokenUsageWindow = "CREATE INDEX IF NOT EXISTS idx_token_usage_window ON token_usage(window_start, window_end)"
)
)
```

#### Step 2.2: Add Token Usage Methods to SQLiteStore

**Modify File:** `internal/token/sqlite_store.go`

**Location:** After existing CRUD methods (around line 500)

```go
// Add to SQLiteStore struct
type SQLiteStore struct {
	db         *sql.DB
	providerID string
	logger     logging.Logger
	mu         sync.RWMutex
	
	// ... existing prepared statements ...
	stmtInsertTokenUsage   *sql.Stmt
	stmtGetTokenUsage     *sql.Stmt
	stmtUpdateTokenUsage *sql.Stmt
	stmtDeleteTokenUsage *sql.Stmt
}

// Add to initializeDB() method
func (s *SQLiteStore) initializeDB() error {
	// ... existing table creation ...
	
	// Create token usage table
	if _, err := s.db.Exec(sqlCreateTokenUsageTable); err != nil {
		return fmt.Errorf("failed to create token_usage table: %w", err)
	}
	
	// Create indexes
	if _, err := s.db.Exec(sqlCreateIndexTokenUsageTokenID); err != nil {
		return fmt.Errorf("failed to create token_usage token_id index: %w", err)
	}
	if _, err := s.db.Exec(sqlCreateIndexTokenUsageWindow); err != nil {
		return fmt.Errorf("failed to create token_usage window index: %w", err)
	}
	
	// ... existing code ...
}

// Add to prepareStatements() method
func (s *SQLiteStore) prepareStatements() error {
	// ... existing statement preparation ...
	
	// Prepare token usage statements
	stmtInsertTokenUsage, err := s.db.Prepare(sqlInsertTokenUsage)
	if err != nil {
		return fmt.Errorf("failed to prepare insert token usage statement: %w", err)
	}
	s.stmtInsertTokenUsage = stmtInsertTokenUsage
	
	stmtGetTokenUsage, err := s.db.Prepare(sqlGetTokenUsage)
	if err != nil {
		return fmt.Errorf("failed to prepare get token usage statement: %w", err)
	}
	s.stmtGetTokenUsage = stmtGetTokenUsage
	
	stmtUpdateTokenUsage, err := s.db.Prepare(sqlUpdateTokenUsage)
	if err != nil {
		return fmt.Errorf("failed to prepare update token usage statement: %w", err)
	}
	s.stmtUpdateTokenUsage = stmtUpdateTokenUsage
	
	stmtDeleteTokenUsage, err := s.db.Prepare(sqlDeleteTokenUsage)
	if err != nil {
		return fmt.Errorf("failed to prepare delete token usage statement: %w", err)
	}
	s.stmtDeleteTokenUsage = stmtDeleteTokenUsage
	
	// ... existing code ...
}

// Add new methods for token usage tracking
func (s *SQLiteStore) RecordTokenUsage(tokenID string, tokensUsed int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	now := time.Now().UnixMilli()
	windowStart := now - 60000 // 1 minute ago
	windowEnd := now
	
	// Check if usage record exists for this token in current window
	var existingID string
	err := s.stmtGetTokenUsage.QueryRow(tokenID, windowStart, windowEnd).Scan(&existingID)
	if err == sql.ErrNoRows {
		// Insert new usage record
		_, err = s.stmtInsertTokenUsage.Exec(
			uuid.New().String(),
			tokenID,
			1, // request_count
			tokensUsed,
			windowStart,
			windowEnd,
		)
	} else if err == nil {
		// Update existing usage record
		_, err = s.stmtUpdateTokenUsage.Exec(
			tokensUsed,
			now,
			existingID,
		)
	}
	
	return err
}

func (s *SQLiteStore) GetTokenUsage(tokenID string) (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	var tokensUsed int
	err := s.stmtGetTokenUsage.QueryRow(tokenID, 
		time.Now().UnixMilli()-60000, // window_start
		time.Now().UnixMilli(),     // window_end
	).Scan(&tokensUsed)
	
	return tokensUsed, err
}

func (s *SQLiteStore) ResetTokenUsage(tokenID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	_, err := s.stmtDeleteTokenUsage.Exec(tokenID)
	return err
}
```

### Phase 3: Middleware Integration (Days 6-7)

#### Step 3.1: Add Token ID to Request Context

**Modify File:** `internal/proxy/openai_handler.go`

**Location:** In `handleChatCompletions()` method, after token selection (around line 196)

```go
// BEFORE:
nativeReq, err := conv.FromOpenAIRequest(openaiReq)

// AFTER:
nativeReq, err := conv.FromOpenAIRequest(openaiReq)

// Add token ID to request context for rate limiting
if token != nil {
	ctx := context.WithValue(r.Context(), "token_id", token.ID)
	r = r.WithContext(ctx)
}
```

#### Step 3.2: Apply Rate Limiting Middleware

**Modify File:** `cmd/main.go`

**Location:** In `applyMiddleware()` function or main function (around line 123)

```go
// Add import
import (
	// ... existing imports ...
	"github.com/sunbankio/qwencoder-proxy/internal/ratelimit"
)

// Create rate limiter
rateLimiter := ratelimit.NewTokenRateLimiter(
	multiTokenMgr,
	logger,
	ratelimit.DefaultRateLimitConfig(),
)

// Apply rate limiting middleware
handler = ratelimit.RateLimitMiddleware(rateLimiter, logger)(handler)
```

#### Step 3.3: Record Usage After Requests

**Modify File:** `internal/proxy/openai_handler.go`

**Location:** In `handleNonStreamCompletions()` method, after successful response (around line 243)

```go
// BEFORE:
h.factory.RecordSuccess(model, p.Name())
w.Header().Set("Content-Type", "application/json")
json.NewEncoder(w).Encode(resp)

// AFTER:
h.factory.RecordSuccess(model, p.Name())

// Record token usage for rate limiting
if token != nil {
	tokensUsed := extractTokenUsage(resp)
	if tokensUsed > 0 {
		if err := rateLimiter.RecordUsage(r.Context(), token.ID, tokensUsed); err != nil {
			h.GetLogger().WarnLog("Failed to record token usage: %v", err)
		}
	}
}

w.Header().Set("Content-Type", "application/json")
json.NewEncoder(w).Encode(resp)
```

### Phase 4: Token Selection Integration (Days 8-9)

#### Step 4.1: Filter Rate-Limited Tokens in Selection

**Modify File:** `internal/token/token_selection.go`

**Location:** In selection strategies, add rate limit filtering

```go
// Add import
import (
	// ... existing imports ...
	"github.com/sunbankio/qwencoder-proxy/internal/ratelimit"
)

// Modify RandomStrategy
func (rs *RandomStrategy) Select(tokens []*ProviderToken) (*ProviderToken, error) {
	if len(tokens) == 0 {
		return nil, ErrNoTokensAvailable
	}
	
	// Filter out rate-limited tokens
	availableTokens := make([]*ProviderToken, 0)
	for _, token := range tokens {
		// Check if token is rate-limited
		if rs.rateLimiter != nil {
			allowed, _, err := rs.rateLimiter.Allow(context.Background(), token.ID)
			if err != nil || !allowed {
				continue // Skip rate-limited token
			}
		}
		availableTokens = append(availableTokens, token)
	}
	
	if len(availableTokens) == 0 {
		return nil, ErrAllTokensRateLimited
	}
	
	// Select from available tokens
	selected := availableTokens[rs.rand.Intn(len(availableTokens))]
	return selected, nil
}

// Add error
var ErrAllTokensRateLimited = errors.New("all tokens are currently rate-limited")
```

#### Step 4.2: Add Rate Limiter to Strategy Factory

**Modify File:** `internal/token/token_selection.go`

**Location:** In StrategyFactory struct and constructor

```go
// Add to StrategyFactory struct
type StrategyFactory struct {
	rand       *rand.Rand
	rateLimiter ratelimit.RateLimiter  // Add rate limiter
}

// Update NewStrategyFactory
func NewStrategyFactory() *StrategyFactory {
	return &StrategyFactory{
		rand: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// Add method to set rate limiter
func (sf *StrategyFactory) SetRateLimiter(limiter ratelimit.RateLimiter) {
	sf.rateLimiter = limiter
}
```

---

## Testing

### Unit Tests

**New File:** `internal/ratelimit/rate_limiter_test.go`

```go
package ratelimit

import (
	"context"
	"testing"
	"time"
	"github.com/stretchr/testify/assert"
)

func TestTokenRateLimiter_Allow(t *testing.T) {
	limiter := NewTokenRateLimiter(nil, nil, DefaultRateLimitConfig())
	
	// First request should be allowed
	allowed, retryAfter, err := limiter.Allow(context.Background(), "token1")
	assert.NoError(t, err)
	assert.True(t, allowed)
	assert.Equal(t, time.Duration(0), retryAfter)
	
	// Second request should be allowed
	allowed, retryAfter, err = limiter.Allow(context.Background(), "token1")
	assert.NoError(t, err)
	assert.True(t, allowed)
}

func TestTokenRateLimiter_ExceedsLimit(t *testing.T) {
	limiter := NewTokenRateLimiter(nil, nil, RateLimitConfig{
		RequestsPerMinute: 2,
		TokensPerMinute:  100,
		BurstSize:        10,
		WindowSize:        1,
	})
	
	// Allow 2 requests (limit is 2)
	limiter.Allow(context.Background(), "token1")
	limiter.Allow(context.Background(), "token1")
	
	// Third request should be denied
	allowed, retryAfter, err := limiter.Allow(context.Background(), "token1")
	assert.NoError(t, err)
	assert.False(t, allowed)
	assert.Greater(t, retryAfter, time.Duration(0))
}

func TestTokenRateLimiter_RecordUsage(t *testing.T) {
	limiter := NewTokenRateLimiter(nil, nil, DefaultRateLimitConfig())
	
	// Record usage
	err := limiter.RecordUsage(context.Background(), "token1", 100)
	assert.NoError(t, err)
	
	// Check remaining
	remaining, err := limiter.GetRemaining(context.Background(), "token1")
	assert.NoError(t, err)
	assert.Equal(t, 90000-100, remaining) // Default is 90000
}
```

### Integration Tests

**New File:** `internal/ratelimit/middleware_test.go`

```go
package ratelimit

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"github.com/stretchr/testify/assert"
)

func TestRateLimitMiddleware_AllowsRequests(t *testing.T) {
	limiter := NewTokenRateLimiter(nil, nil, DefaultRateLimitConfig())
	middleware := RateLimitMiddleware(limiter, nil)
	
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))
	
	req := httptest.NewRequest("GET", "/test", nil)
	req = req.WithContext(context.WithValue(req.Context(), "token_id", "token1"))
	
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	
	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "OK", rr.Body.String())
}

func TestRateLimitMiddleware_BlocksRateLimited(t *testing.T) {
	limiter := NewTokenRateLimiter(nil, nil, RateLimitConfig{
		RequestsPerMinute: 1,
		TokensPerMinute:  100,
		BurstSize:        10,
		WindowSize:        1,
	})
	middleware := RateLimitMiddleware(limiter, nil)
	
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	
	// First request should succeed
	req1 := httptest.NewRequest("GET", "/test", nil)
	req1 = req1.WithContext(context.WithValue(req1.Context(), "token_id", "token1"))
	rr1 := httptest.NewRecorder()
	handler.ServeHTTP(rr1, req1)
	assert.Equal(t, http.StatusOK, rr1.Code)
	
	// Second request should be rate limited
	req2 := httptest.NewRequest("GET", "/test", nil)
	req2 = req2.WithContext(context.WithValue(req2.Context(), "token_id", "token1"))
	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req2)
	assert.Equal(t, http.StatusTooManyRequests, rr2.Code)
	assert.Contains(t, rr2.Body.String(), "rate_limit_exceeded")
}
```

---

## Configuration

### Environment Variables

Add to `.env` or environment:

```bash
# Rate limiting configuration
RATE_LIMIT_REQUESTS_PER_MINUTE=60
RATE_LIMIT_TOKENS_PER_MINUTE=90000
RATE_LIMIT_BURST_SIZE=10
RATE_LIMIT_WINDOW_SIZE=1
RATE_LIMIT_STRATEGY=token_based  # token_based, request_based, hybrid
```

### Configuration File

**Modify File:** `internal/config/config.go`

```go
// Add to Config struct
type Config struct {
	// ... existing fields ...
	
	// Rate limiting configuration
	RateLimit RateLimitConfig `mapstructure:"rate_limit"`
}

// Add to RateLimitConfig struct
type RateLimitConfig struct {
	RequestsPerMinute int    `mapstructure:"requests_per_minute"`
	TokensPerMinute  int    `mapstructure:"tokens_per_minute"`
	BurstSize        int    `mapstructure:"burst_size"`
	WindowSize        int    `mapstructure:"window_size"`
	Strategy         string `mapstructure:"strategy"`
}

// Add default values to DefaultConfig()
func DefaultConfig() *Config {
	return &Config{
		// ... existing defaults ...
		RateLimit: RateLimitConfig{
			RequestsPerMinute: 60,
			TokensPerMinute:  90000,
			BurstSize:        10,
			WindowSize:        1,
			Strategy:         "token_based",
		},
	}
}
```

---

## Verification Checklist

### Phase 1: Package Creation
- [ ] `internal/ratelimit/rate_limiter.go` created with RateLimiter interface
- [ ] `internal/ratelimit/middleware.go` created with HTTP middleware
- [ ] `internal/ratelimit/strategies.go` created with rate limit strategies
- [ ] `internal/ratelimit/errors.go` created with error types
- [ ] Unit tests for rate limiter pass
- [ ] Unit tests for middleware pass

### Phase 2: Database Updates
- [ ] Token usage table added to SQLite schema
- [ ] Token usage indexes created
- [ ] `RecordTokenUsage()` method implemented
- [ ] `GetTokenUsage()` method implemented
- [ ] `ResetTokenUsage()` method implemented
- [ ] Database migration tests pass

### Phase 3: Middleware Integration
- [ ] Token ID added to request context
- [ ] Rate limiting middleware applied in main.go
- [ ] Usage recording added after successful requests
- [ ] Rate limit headers added to responses
- [ ] Integration tests pass

### Phase 4: Token Selection Integration
- [ ] Rate limit filtering added to token selection
- [ ] Rate limiter added to strategy factory
- [ ] `ErrAllTokensRateLimited` error defined
- [ ] Token selection tests with rate limiting pass

### Phase 5: Configuration
- [ ] Environment variables documented
- [ ] Configuration struct updated
- [ ] Default values set
- [ ] Configuration loading tested

---

## Impact

**Positive:**
- Prevents token abuse and quota exhaustion
- Provides fair usage across multiple tokens
- Enforces provider-specific rate limits
- Improves system stability and predictability
- Better user experience with clear rate limit feedback

**Risk:**
- Medium complexity - requires careful testing
- Performance impact from rate limit checks (minimal)
- Database schema changes require migration

**Side Effects:**
- All requests will be rate-limited by default
- Rate limit headers will be added to all responses
- Token selection will prefer non-rate-limited tokens
- New database table for usage tracking

**Performance Considerations:**
- Rate limit check: O(1) - constant time
- Token state management: O(n) where n is number of active tokens
- Database queries: Indexed for efficient lookups
- Memory: ~100 bytes per token state

---

## Rollback Plan

If issues arise after deployment:

1. **Disable Rate Limiting:**
   - Set `RATE_LIMIT_STRATEGY=disabled` in environment
   - Or remove middleware from `cmd/main.go`

2. **Revert Database:**
   - Drop `token_usage` table: `DROP TABLE IF EXISTS token_usage;`
   - Remove indexes: `DROP INDEX IF EXISTS idx_token_usage_token_id;`

3. **Restore Previous Code:**
   - Revert `internal/proxy/openai_handler.go` changes
   - Revert `internal/token/token_selection.go` changes
   - Remove `internal/ratelimit` package

4. **Monitor:**
   - Check logs for rate limit errors
   - Monitor token usage patterns
   - Verify provider API responses

---

## Future Enhancements

1. **Per-Provider Rate Limits:**
   - Different limits for Qwen vs Gemini vs iFlow
   - Provider-specific configuration

2. **Dynamic Rate Limits:**
   - Adjust limits based on provider feedback
   - Learn optimal limits from usage patterns

3. **Quota Management:**
   - Daily/monthly quotas per token
   - Quota reset scheduling
   - Quota exceeded notifications

4. **Rate Limit Analytics:**
   - Track rate limit violations
   - Identify abuse patterns
   - Optimize limit values

5. **Graceful Degradation:**
   - Queue requests when rate limited
   - Prioritize important requests
   - Provide estimated wait times
