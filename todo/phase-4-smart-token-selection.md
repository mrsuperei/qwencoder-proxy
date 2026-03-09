# Phase 4: Smart Token Selection - Quota-Aware Routing

**Phase Goal:** Integrate quota-aware token selection into the existing token manager.

**Duration:** Week 4  
**Status:** Ready to Implement  
**Dependencies:** Phase 1 (Foundation), Phase 2 (Core Rate Limiting), Phase 3 (Async Tracking)

---

## Task Overview

This phase implements smart token selection based on quota availability:

1. Implementing `QuotaAwareSelectionStrategy`
2. Integrating with existing `TokenManager`
3. Adding quota score calculation
4. Writing unit tests for selection strategy
5. Writing integration tests with proxy

---

## Task 4.1: Implement `QuotaAwareSelectionStrategy`

**File:** `qwencoder-proxy/ratelimit/selector.go`

Create the quota-aware token selection strategy.

**Implementation Requirements:**

```go
package ratelimit

import (
    "context"
    "sort"
    "sync"
    
    "github.com/sunbankio/qwencoder-proxy/internal/token"
)

// QuotaAwareSelectionStrategy selects tokens based on quota availability
type QuotaAwareSelectionStrategy struct {
    rateLimiter RateLimiter
    fallback    token.SelectionStrategy
    logger      Logger
    mu          sync.RWMutex
}

// TokenQuotaInfo represents quota information for a token
type TokenQuotaInfo struct {
    Token *token.ProviderToken
    Quota *QuotaInfo
    Score float64
}

// NewQuotaAwareSelectionStrategy creates a new quota-aware selection strategy
func NewQuotaAwareSelectionStrategy(
    rateLimiter RateLimiter,
    fallback token.SelectionStrategy,
    logger Logger,
) *QuotaAwareSelectionStrategy {
    return &QuotaAwareSelectionStrategy{
        rateLimiter: rateLimiter,
        fallback:    fallback,
        logger:      logger,
    }
}

// SelectToken selects a token based on quota availability
func (s *QuotaAwareSelectionStrategy) SelectToken(tokens []token.ProviderToken) (*token.ProviderToken, error) {
    if len(tokens) == 0 {
        return nil, token.ErrNoTokensAvailable
    }
    
    // Filter valid tokens
    validTokens := s.filterValidTokens(tokens)
    if len(validTokens) == 0 {
        return nil, token.ErrNoValidTokens
    }
    
    // If rate limiter is not available, use fallback
    if s.rateLimiter == nil {
        s.logger.DebugLog("[QuotaAware] Rate limiter not available, using fallback strategy")
        return s.fallback.SelectToken(validTokens)
    }
    
    // Get quota info for each token
    tokenQuotas := s.getTokenQuotas(validTokens)
    if len(tokenQuotas) == 0 {
        s.logger.DebugLog("[QuotaAware] No quota info available, using fallback strategy")
        return s.fallback.SelectToken(validTokens)
    }
    
    // Sort by quota score (highest first)
    sort.Slice(tokenQuotas, func(i, j int) bool {
        // Higher score first
        if tokenQuotas[i].Score != tokenQuotas[j].Score {
            return tokenQuotas[i].Score > tokenQuotas[j].Score
        }
        
        // If scores are equal, prefer healthier token
        return tokenQuotas[i].Token.Healthy && !tokenQuotas[j].Token.Healthy
    })
    
    selected := tokenQuotas[0].Token
    s.logger.DebugLog("[QuotaAware] Selected token %s with score %.2f", 
        selected.ID, tokenQuotas[0].Score)
    
    return selected, nil
}

// Name returns the strategy name
func (s *QuotaAwareSelectionStrategy) Name() string {
    return "quota_aware"
}

// filterValidTokens filters valid tokens
func (s *QuotaAwareSelectionStrategy) filterValidTokens(tokens []token.ProviderToken) []token.ProviderToken {
    var valid []token.ProviderToken
    now := time.Now().UnixMilli()
    
    for _, token := range tokens {
        // Check if token is healthy
        if !token.Healthy {
            continue
        }
        
        // Check if token is expired
        if token.ExpiryDate > 0 && token.ExpiryDate < now {
            continue
        }
        
        valid = append(valid, token)
    }
    
    return valid
}

// getTokenQuotas gets quota information for all tokens
func (s *QuotaAwareSelectionStrategy) getTokenQuotas(tokens []token.ProviderToken) []*TokenQuotaInfo {
    var tokenQuotas []*TokenQuotaInfo
    var mu sync.Mutex
    
    // Get quotas concurrently
    var wg sync.WaitGroup
    results := make(chan *TokenQuotaInfo, len(tokens))
    
    for _, t := range tokens {
        wg.Add(1)
        go func(token token.ProviderToken) {
            defer wg.Done()
            
            quota, err := s.rateLimiter.GetTokenQuota(context.Background(), 
                token.ProviderID, token.ID)
            if err != nil {
                s.logger.WarnLog("[QuotaAware] Failed to get quota for token %s: %v", 
                    token.ID, err)
                return
            }
            
            score := s.calculateQuotaScore(quota)
            
            mu.Lock()
            results <- &TokenQuotaInfo{
                Token: &token,
                Quota: quota,
                Score: score,
            }
            mu.Unlock()
        }(t)
    }
    
    wg.Wait()
    close(results)
    
    // Collect results
    for result := range results {
        tokenQuotas = append(tokenQuotas, result)
    }
    
    return tokenQuotas
}

// calculateQuotaScore calculates a score for quota availability
func (s *QuotaAwareSelectionStrategy) calculateQuotaScore(quota *QuotaInfo) float64 {
    var dailyScore, burstScore, tokenScore float64
    
    // Calculate daily score (50% weight)
    if quota.DailyLimit > 0 {
        dailyScore = float64(quota.DailyRemaining) / float64(quota.DailyLimit)
    } else {
        dailyScore = 1.0 // No limit means full score
    }
    
    // Calculate burst score (30% weight)
    if quota.BurstLimit > 0 {
        burstScore = float64(quota.BurstRemaining) / float64(quota.BurstLimit)
    } else {
        burstScore = 1.0 // No limit means full score
    }
    
    // Calculate token score (20% weight)
    if quota.TokenLimit > 0 {
        tokenScore = float64(quota.TokenRemaining) / float64(quota.TokenLimit)
    } else {
        tokenScore = 1.0 // No limit means full score
    }
    
    // Weighted average
    score := (dailyScore * 0.5) + (burstScore * 0.3) + (tokenScore * 0.2)
    
    return score
}

// SetFallback sets the fallback strategy
func (s *QuotaAwareSelectionStrategy) SetFallback(strategy token.SelectionStrategy) {
    s.mu.Lock()
    defer s.mu.Unlock()
    s.fallback = strategy
}

// GetFallback returns the fallback strategy
func (s *QuotaAwareSelectionStrategy) GetFallback() token.SelectionStrategy {
    s.mu.RLock()
    defer s.mu.RUnlock()
    return s.fallback
}
```

---

## Task 4.2: Integrate with Existing `TokenManager`

**File:** `qwencoder-proxy/internal/token/token_selection.go`

Update the existing token manager to support quota-aware selection.

**Implementation Requirements:**

Add the following to the existing `TokenManager` struct:

```go
// Add field to TokenManager struct
type TokenManager struct {
    store              TokenRepository
    strategy           SelectionStrategy
    mu                 sync.RWMutex
    logger             logging.Logger
    clientFactory      ProxyClientFactory
    proxyHealthTracker *ProxyHealthTracker
    rateLimiter        ratelimit.RateLimiter  // NEW
    originalStrategy    SelectionStrategy      // NEW - store original strategy
}

// Add SetRateLimiter method
func (tm *TokenManager) SetRateLimiter(rl ratelimit.RateLimiter) {
    tm.mu.Lock()
    defer tm.mu.Unlock()
    
    tm.rateLimiter = rl
    
    // Wrap strategy with quota-aware selection if rate limiter is set
    if rl != nil {
        // Store original strategy
        if tm.originalStrategy == nil {
            tm.originalStrategy = tm.strategy
        }
        
        // Create quota-aware strategy
        quotaStrategy := ratelimit.NewQuotaAwareSelectionStrategy(
            rl,
            tm.originalStrategy,
            tm.logger,
        )
        
        tm.strategy = quotaStrategy
        tm.logger.InfoLog("[TokenManager] Enabled quota-aware token selection")
    } else {
        // Restore original strategy
        if tm.originalStrategy != nil {
            tm.strategy = tm.originalStrategy
            tm.logger.InfoLog("[TokenManager] Disabled quota-aware token selection")
        }
    }
}

// Add GetRateLimiter method
func (tm *TokenManager) GetRateLimiter() ratelimit.RateLimiter {
    tm.mu.RLock()
    defer tm.mu.RUnlock()
    return tm.rateLimiter
}
```

---

## Task 4.3: Add Quota Score Calculation

The quota score calculation is already implemented in `QuotaAwareSelectionStrategy.calculateQuotaScore()`. 

**Additional Requirements:**

1. Make score weights configurable
2. Add logging for score calculation
3. Add metrics for score distribution

**Enhanced Implementation:**

```go
// QuotaScoreWeights defines weights for quota score calculation
type QuotaScoreWeights struct {
    Daily float64
    Burst float64
    Token float64
}

// DefaultQuotaScoreWeights returns default weights
func DefaultQuotaScoreWeights() *QuotaScoreWeights {
    return &QuotaScoreWeights{
        Daily: 0.5,
        Burst: 0.3,
        Token: 0.2,
    }
}

// Update QuotaAwareSelectionStrategy to use configurable weights
type QuotaAwareSelectionStrategy struct {
    rateLimiter RateLimiter
    fallback    token.SelectionStrategy
    logger      Logger
    weights     *QuotaScoreWeights
    mu          sync.RWMutex
}

// calculateQuotaScore calculates a score for quota availability
func (s *QuotaAwareSelectionStrategy) calculateQuotaScore(quota *QuotaInfo) float64 {
    var dailyScore, burstScore, tokenScore float64
    
    // Calculate daily score
    if quota.DailyLimit > 0 {
        dailyScore = float64(quota.DailyRemaining) / float64(quota.DailyLimit)
    } else {
        dailyScore = 1.0
    }
    
    // Calculate burst score
    if quota.BurstLimit > 0 {
        burstScore = float64(quota.BurstRemaining) / float64(quota.BurstLimit)
    } else {
        burstScore = 1.0
    }
    
    // Calculate token score
    if quota.TokenLimit > 0 {
        tokenScore = float64(quota.TokenRemaining) / float64(quota.TokenLimit)
    } else {
        tokenScore = 1.0
    }
    
    // Weighted average
    score := (dailyScore * s.weights.Daily) + 
              (burstScore * s.weights.Burst) + 
              (tokenScore * s.weights.Token)
    
    s.logger.DebugLog("[QuotaAware] Score: daily=%.2f, burst=%.2f, token=%.2f, total=%.2f",
        dailyScore, burstScore, tokenScore, score)
    
    return score
}

// SetScoreWeights sets the score weights
func (s *QuotaAwareSelectionStrategy) SetScoreWeights(weights *QuotaScoreWeights) {
    s.mu.Lock()
    defer s.mu.Unlock()
    s.weights = weights
}
```

---

## Task 4.4: Write Unit Tests for Selection Strategy

**File:** `qwencoder-proxy/ratelimit/selector_test.go`

Write comprehensive unit tests for the quota-aware selection strategy.

**Test Cases:**

1. **Token Selection Tests:**
   - Test selecting token with highest quota
   - Test selecting token when all have equal quota
   - Test fallback when rate limiter is nil
   - Test filtering invalid tokens

2. **Score Calculation Tests:**
   - Test score calculation with daily limit
   - Test score calculation with burst limit
   - Test score calculation with token limit
   - Test score calculation with no limits

3. **Concurrency Tests:**
   - Test concurrent token selection
   - Test concurrent quota fetching

**Test Template:**

```go
package ratelimit

import (
    "testing"
    "time"
    
    "github.com/sunbankio/qwencoder-proxy/internal/token"
)

func setupTestSelector(t *testing.T) (*QuotaAwareSelectionStrategy, RateLimiter) {
    limiter := setupMockRateLimiter(t)
    fallback := token.NewRandomSelectionStrategy()
    logger := newTestLogger()
    
    selector := NewQuotaAwareSelectionStrategy(limiter, fallback, logger)
    return selector, limiter
}

func TestQuotaAwareSelection_SelectToken(t *testing.T) {
    selector, limiter := setupTestSelector(t)
    
    // Create test tokens
    tokens := []token.ProviderToken{
        {
            ID:         "token-1",
            ProviderID: "qwen",
            Healthy:    true,
            ExpiryDate: time.Now().Add(24 * time.Hour).UnixMilli(),
        },
        {
            ID:         "token-2",
            ProviderID: "qwen",
            Healthy:    true,
            ExpiryDate: time.Now().Add(24 * time.Hour).UnixMilli(),
        },
    }
    
    // Set up quota mock
    limiter.SetMockQuota("token-1", &QuotaInfo{
        DailyLimit:     1000,
        DailyUsed:      500,
        DailyRemaining:  500,
        BurstLimit:     60,
        BurstUsed:      20,
        BurstRemaining:  40,
        TokenLimit:     0,
        TokenUsed:      0,
        TokenRemaining: 0,
    })
    
    limiter.SetMockQuota("token-2", &QuotaInfo{
        DailyLimit:     1000,
        DailyUsed:      200,
        DailyRemaining:  800,
        BurstLimit:     60,
        BurstUsed:      10,
        BurstRemaining:  50,
        TokenLimit:     0,
        TokenUsed:      0,
        TokenRemaining: 0,
    })
    
    // Select token
    selected, err := selector.SelectToken(tokens)
    if err != nil {
        t.Fatalf("SelectToken failed: %v", err)
    }
    
    // Should select token-2 (higher quota)
    if selected.ID != "token-2" {
        t.Errorf("Expected token-2, got %s", selected.ID)
    }
}

func TestQuotaAwareSelection_CalculateScore(t *testing.T) {
    selector, _ := setupTestSelector(t)
    
    quota := &QuotaInfo{
        DailyLimit:     1000,
        DailyUsed:      500,
        DailyRemaining:  500,
        BurstLimit:     60,
        BurstUsed:      20,
        BurstRemaining:  40,
        TokenLimit:     100000,
        TokenUsed:      50000,
        TokenRemaining:  50000,
    }
    
    score := selector.calculateQuotaScore(quota)
    
    // Expected: (0.5 * 0.5) + (0.667 * 0.3) + (0.5 * 0.2) = 0.55
    expected := (0.5 * 0.5) + (0.667 * 0.3) + (0.5 * 0.2)
    
    if score < expected-0.01 || score > expected+0.01 {
        t.Errorf("Expected score %.2f, got %.2f", expected, score)
    }
}

// Implement more tests...
```

---

## Task 4.5: Write Integration Tests with Proxy

**File:** `qwencoder-proxy/ratelimit/selector_integration_test.go`

Write integration tests that test the quota-aware selection with the proxy.

**Test Cases:**

1. **End-to-End Selection:**
   - Test token selection through proxy
   - Verify quota-aware routing works

2. **Rate Limit Interaction:**
   - Test that rate limits affect token selection
   - Test that exhausted tokens are not selected

3. **Fallback Behavior:**
   - Test fallback when quota is unavailable
   - Test fallback when rate limiter is disabled

---

## Deliverables

After completing this phase, you should have:

1. ✅ `QuotaAwareSelectionStrategy` implementation
2. ✅ Integration with existing `TokenManager`
3. ✅ Configurable quota score calculation
4. ✅ Unit tests for selection strategy
5. ✅ Integration tests with proxy

---

## Success Criteria

- [ ] Token selection considers quota availability
- [ ] Tokens with higher quota are preferred
- [ ] Fallback strategy works when quota is unavailable
- [ ] All unit tests pass
- [ ] All integration tests pass
- [ ] No race conditions
- [ ] Logging is appropriate

---

## Next Phase

After completing Phase 4, proceed to **Phase 5: Middleware Integration** which integrates rate limiting into proxy handlers.

**File:** `../todo/phase-5-middleware-integration.md`
