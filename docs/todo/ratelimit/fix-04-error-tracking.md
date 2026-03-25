# Fix Plan: Problem 4 - Missing Error Tracking

**Priority:** MEDIUM  
**Status:** Ready for Implementation  
**Created:** 2026-03-18

---

## Problem Description

### Current Behavior
The system has no mechanism to track rate limit errors or server errors from LLM providers.

**Existing Infrastructure:**
- The `tokens` table has error tracking fields ([`sqlite_store.go:49-50`](qwencoder-proxy/internal/token/sqlite_store.go:49)):
  ```sql
  error_count INTEGER NOT NULL DEFAULT 0,
  last_error TEXT
  ```
- The `tokens` table has health tracking fields:
  ```sql
  healthy INTEGER NOT NULL DEFAULT 1,
  health_score REAL NOT NULL DEFAULT 1.0,
  ```

**The Problem:**
- These fields are **never updated** when providers return errors
- No visibility into which tokens are experiencing rate limits
- No tracking of server errors (HTTP 5xx)
- No automatic token health management based on errors
- Difficult to troubleshoot issues when requests fail

### Root Cause
The proxy handlers don't catch and record provider errors in the database. When a provider returns:
- **429 Rate Limit** - Token should be marked with error
- **500 Internal Server Error** - Token should be marked with error
- **503 Service Unavailable** - Token should be marked with error
- Other errors - Should be tracked for debugging

Currently, these errors are logged but not persisted to the database.

### Impact
- **No error visibility:** Can't see which tokens are failing
- **Poor load balancing:** Load balancer can't avoid failing tokens
- **Manual troubleshooting:** Must check logs to find issues
- **No automatic recovery:** Tokens stay unhealthy indefinitely
- **Inefficient resource usage:** Failing tokens continue to be selected

---

## Solution Overview

**Approach:** Implement comprehensive error tracking that:
1. Catches provider errors (rate limits, server errors, etc.)
2. Records errors in the `tokens` table
3. Updates token health based on error patterns
4. Uses error data in load balancing decisions
5. Provides API endpoints for error visibility

**Rationale:**
- Leverages existing database fields (`error_count`, `last_error`, `healthy`, `health_score`)
- Improves system reliability and observability
- Enables automatic token health management
- Supports better load balancing decisions
- Provides debugging and monitoring capabilities

**Changes Required:**
1. Add error recording method to QuotaManager
2. Update proxy handlers to catch and record errors
3. Implement token health management based on errors
4. Update load balancing to use error data
5. Add API endpoints for error visibility
6. Add tests

---

## Detailed Implementation Plan

### Step 1: Add Error Recording Method to QuotaManager

**File:** [`qwencoder-proxy/internal/ratelimit/quota_manager.go`](qwencoder-proxy/internal/ratelimit/quota_manager.go)

**Add New Method:**

```go
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
    if qm.cachedTracker != nil {
        if cached, ok := qm.cachedTracker.(*CachedUsageTracker); ok {
            cached.InvalidateToken(tokenID)
        }
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

    // Decay the penalty over time (simplified - could be more sophisticated)
    // For now, just apply the penalty
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

    // Mark as unhealthy for consecutive rate limits
    // (This would need additional state tracking)

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
    if qm.cachedTracker != nil {
        if cached, ok := qm.cachedTracker.(*CachedUsageTracker); ok {
            cached.InvalidateToken(tokenID)
        }
    }

    return nil
}
```

**Add Type Definitions:**

```go
// TokenErrorInfo represents error information for a token.
type TokenErrorInfo struct {
    TokenID     string  `json:"token_id"`
    ProviderID  string  `json:"provider_id"`
    ErrorCount  int     `json:"error_count"`
    LastError   string  `json:"last_error"`
    HealthScore float64 `json:"health_score"`
    Healthy     bool    `json:"healthy"`
}

// ErrorType represents the type of error that occurred.
type ErrorType string

const (
    ErrorTypeRateLimit      ErrorType = "rate_limit"
    ErrorTypeServerError    ErrorType = "server_error"
    ErrorTypeTimeout       ErrorType = "timeout"
    ErrorTypeAuth          ErrorType = "authentication"
    ErrorTypeNetwork       ErrorType = "network"
    ErrorTypeUnknown       ErrorType = "unknown"
)
```

---

### Step 2: Update SequentialHandler to Record Errors

**File:** [`qwencoder-proxy/internal/proxy/sequential_handler.go`](qwencoder-proxy/internal/proxy/sequential_handler.go)

**Add Error Handling Wrapper:**

Create a helper method to wrap request execution with error recording:

```go
// executeWithRetry executes a request with retry logic and error recording.
func (h *SequentialHandler) executeWithRetry(
    ctx context.Context,
    w http.ResponseWriter,
    r *http.Request,
    p provider.Provider,
    conv converter.Converter,
    nativeReq interface{},
    model string,
    providerID string,
    selectedToken *token.ProviderToken,
) (interface{}, int, error) {
    var lastError error
    var lastStatusCode int

    // Try the request
    response, statusCode, err := p.Execute(ctx, nativeReq)

    if err != nil {
        // Determine error type
        errorType := h.classifyError(err, statusCode)
        errorMessage := err.Error()

        // Record the error
        if recordErr := h.quotaManager.RecordError(ctx, providerID, selectedToken.ID, string(errorType), errorMessage, statusCode); recordErr != nil {
            h.logger.ErrorLog("[SequentialHandler] Failed to record error: %v", recordErr)
        }

        // Return error to client
        h.handleError(w, statusCode, err)
        return nil, statusCode, err
    }

    // Success - return response
    return response, statusCode, nil
}

// classifyError determines the type of error based on status code and error message.
func (h *SequentialHandler) classifyError(err error, statusCode int) ratelimit.ErrorType {
    // Check status code first
    if statusCode == http.StatusTooManyRequests {
        return ratelimit.ErrorTypeRateLimit
    }
    if statusCode >= 500 && statusCode < 600 {
        return ratelimit.ErrorTypeServerError
    }

    // Check error message for additional context
    errStr := err.Error()
    if strings.Contains(errStr, "timeout") || strings.Contains(errStr, "deadline exceeded") {
        return ratelimit.ErrorTypeTimeout
    }
    if strings.Contains(errStr, "authentication") || strings.Contains(errStr, "unauthorized") {
        return ratelimit.ErrorTypeAuth
    }
    if strings.Contains(errStr, "connection refused") || strings.Contains(errStr, "network") {
        return ratelimit.ErrorTypeNetwork
    }

    return ratelimit.ErrorTypeUnknown
}

// handleError handles errors and returns appropriate HTTP responses.
func (h *SequentialHandler) handleError(w http.ResponseWriter, statusCode int, err error) {
    switch statusCode {
    case http.StatusTooManyRequests:
        w.WriteHeader(http.StatusTooManyRequests)
        json.NewEncoder(w).Encode(map[string]interface{}{
            "error": "rate_limit_exceeded",
            "message": "Rate limit exceeded, please try again later",
        })
    case http.StatusInternalServerError:
        w.WriteHeader(http.StatusInternalServerError)
        json.NewEncoder(w).Encode(map[string]interface{}{
            "error": "internal_server_error",
            "message": "Internal server error occurred",
        })
    default:
        w.WriteHeader(statusCode)
        json.NewEncoder(w).Encode(map[string]interface{}{
            "error": "request_failed",
            "message": err.Error(),
        })
    }
}
```

**Update handleNonStreamingRequest:**

Modify the request execution section to use the new error handling:

```go
// Step 4: Execute request
h.logger.DebugLog("[SequentialHandler] Step 4: Executing request to provider")

response, statusCode, err := h.executeWithRetry(ctx, responseWrapper, r, p, nativeReq, model, providerID, selectedToken)
if err != nil {
    // Error already handled by executeWithRetry
    h.logger.ErrorLog("[SequentialHandler] Step 4: Request failed: %v", err)
    return
}

h.logger.DebugLog("[SequentialHandler] Step 4: Request executed successfully")
```

**Update handleStreamingRequest:**

Add error handling for streaming requests:

```go
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
```

---

### Step 3: Update Load Balancing to Use Error Data

**File:** [`qwencoder-proxy/internal/ratelimit/load_balancing.go`](qwencoder-proxy/internal/ratelimit/load_balancing.go)

**Update AdaptiveSelector:**

The adaptive selector already considers `error_rate` factor, but we should make it more effective:

```go
// Factor 4: Error rate (enhanced)
if t.ErrorCount > 0 {
    // Exponential decay based on error count
    // More errors = much lower score
    errorFactor := 1.0 / math.Pow(float64(t.ErrorCount+1), 1.5)
    factors["error_rate"] = errorFactor
} else {
    factors["error_rate"] = 1.0
}

// Additional factor: Healthy status
if t.Healthy == 0 {
    // Severely penalize unhealthy tokens
    factors["healthy_status"] = 0.1
} else {
    factors["healthy_status"] = 1.0
}

// Calculate weighted score (updated)
score := (factors["rate_limit"] * 0.4) +
    (factors["health"] * 0.15) +
    (factors["recency"] * 0.15) +
    (factors["error_rate"] * 0.25) +
    (factors["healthy_status"] * 0.05)
```

**Update WeightedRoundRobinSelector:**

Skip unhealthy tokens:

```go
func (s *WeightedRoundRobinSelector) getWeightedTokens(ctx context.Context, providerID string) ([]weightedToken, error) {
    // Get all valid tokens for the provider
    query := `
        SELECT id, access_token, refresh_token, token_type, expiry_date,
               email, resource_url, scope, api_key, project_id,
               healthy, health_score, last_used, created_at, error_count,
               last_error, proxy_id
        FROM tokens
        WHERE provider_id = ? AND healthy = 1 AND expiry_date > ?
    `

    // ... rest of the method ...
}
```

**Update LeastConnectionsSelector:**

Skip unhealthy tokens:

```go
func (s *LeastConnectionsSelector) SelectToken(ctx context.Context, providerID string) (*token.ProviderToken, error) {
    // Get tokens with their current connection counts
    query := `
        SELECT t.id, t.access_token, t.refresh_token, t.token_type, t.expiry_date,
               t.email, t.resource_url, t.scope, t.api_key, t.project_id,
               t.healthy, t.health_score, t.last_used, t.created_at, t.error_count,
               t.last_error, t.proxy_id,
               COALESCE(u.requests_in_minute, 0) as current_connections
        FROM tokens t
        LEFT JOIN token_usage u ON t.id = u.token_id
        WHERE t.provider_id = ? AND t.healthy = 1 AND t.expiry_date > ?
        ORDER BY current_connections ASC
        LIMIT 1
    `

    // ... rest of the method ...
}
```

---

### Step 4: Add API Endpoints for Error Visibility

**File:** [`qwencoder-proxy/internal/restapi/rate_limit_api.go`](qwencoder-proxy/internal/restapi/rate_limit_api.go) (or create new file)

**Add New Endpoints:**

```go
// GetTokenErrors returns error information for a specific token.
func (api *RateLimitAPI) GetTokenErrors(w http.ResponseWriter, r *http.Request) {
    // Extract parameters
    providerID := r.URL.Query().Get("provider_id")
    tokenID := r.URL.Query().Get("token_id")

    if providerID == "" || tokenID == "" {
        w.WriteHeader(http.StatusBadRequest)
        json.NewEncoder(w).Encode(map[string]interface{}{
            "error": "missing_parameters",
            "message": "provider_id and token_id are required",
        })
        return
    }

    // Get error information
    errors, err := api.quotaManager.GetTokenErrors(r.Context(), providerID, tokenID)
    if err != nil {
        w.WriteHeader(http.StatusInternalServerError)
        json.NewEncoder(w).Encode(map[string]interface{}{
            "error": "internal_error",
            "message": err.Error(),
        })
        return
    }

    // Return response
    json.NewEncoder(w).Encode(errors)
}

// GetAllTokenErrors returns error information for all tokens of a provider.
func (api *RateLimitAPI) GetAllTokenErrors(w http.ResponseWriter, r *http.Request) {
    // Extract parameters
    providerID := r.URL.Query().Get("provider_id")

    if providerID == "" {
        w.WriteHeader(http.StatusBadRequest)
        json.NewEncoder(w).Encode(map[string]interface{}{
            "error": "missing_parameter",
            "message": "provider_id is required",
        })
        return
    }

    // Get all error information
    errors, err := api.quotaManager.GetAllTokenErrors(r.Context(), providerID)
    if err != nil {
        w.WriteHeader(http.StatusInternalServerError)
        json.NewEncoder(w).Encode(map[string]interface{}{
            "error": "internal_error",
            "message": err.Error(),
        })
        return
    }

    // Return response
    json.NewEncoder(w).Encode(map[string]interface{}{
        "provider_id": providerID,
        "errors":      errors,
    })
}

// ResetTokenErrors resets error tracking for a token.
func (api *RateLimitAPI) ResetTokenErrors(w http.ResponseWriter, r *http.Request) {
    // Extract parameters
    providerID := r.URL.Query().Get("provider_id")
    tokenID := r.URL.Query().Get("token_id")

    if providerID == "" || tokenID == "" {
        w.WriteHeader(http.StatusBadRequest)
        json.NewEncoder(w).Encode(map[string]interface{}{
            "error": "missing_parameters",
            "message": "provider_id and token_id are required",
        })
        return
    }

    // Reset errors
    err := api.quotaManager.ResetTokenErrors(r.Context(), providerID, tokenID)
    if err != nil {
        w.WriteHeader(http.StatusInternalServerError)
        json.NewEncoder(w).Encode(map[string]interface{}{
            "error": "internal_error",
            "message": err.Error(),
        })
        return
    }

    // Return success
    json.NewEncoder(w).Encode(map[string]interface{}{
        "message": "errors_reset",
        "token_id": tokenID,
    })
}
```

**Register Routes:**

```go
// Register error tracking routes
api.router.HandleFunc("/api/ratelimit/errors", api.GetAllTokenErrors).Methods("GET")
api.router.HandleFunc("/api/ratelimit/errors/token", api.GetTokenErrors).Methods("GET")
api.router.HandleFunc("/api/ratelimit/errors/reset", api.ResetTokenErrors).Methods("POST")
```

---

### Step 5: Add Error Recovery Mechanism

**File:** [`qwencoder-proxy/internal/ratelimit/quota_manager.go`](qwencoder-proxy/internal/ratelimit/quota_manager.go)

**Add Token Health Recovery:**

```go
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
        if qm.cachedTracker != nil {
            if cached, ok := qm.cachedTracker.(*CachedUsageTracker); ok {
                cached.InvalidateToken(tokenID)
            }
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
```

---

### Step 6: Add Tests

**File:** [`qwencoder-proxy/internal/ratelimit/quota_manager_test.go`](qwencoder-proxy/internal/ratelimit/quota_manager_test.go) (or create new test file)

**Add Error Tracking Tests:**

```go
func TestRecordError(t *testing.T) {
    // Setup
    db := setupTestDB(t)
    quotaManager := NewQuotaManager(nil, logger, db, nil, nil, nil)
    ctx := context.Background()

    // Record an error
    err := quotaManager.RecordError(ctx, "provider1", "token1", "rate_limit", "Rate limit exceeded", 429)
    if err != nil {
        t.Fatalf("Failed to record error: %v", err)
    }

    // Verify error was recorded
    var errorCount int
    var lastError string
    var healthScore float64
    var healthy int

    err = db.QueryRow(`
        SELECT error_count, last_error, health_score, healthy
        FROM tokens
        WHERE id = ?
    `, "token1").Scan(&errorCount, &lastError, &healthScore, &healthy)

    if err != nil {
        t.Fatalf("Failed to query token: %v", err)
    }

    if errorCount != 1 {
        t.Errorf("Expected error_count to be 1, got: %d", errorCount)
    }
    if lastError != "Rate limit exceeded" {
        t.Errorf("Expected last_error to be 'Rate limit exceeded', got: %s", lastError)
    }
    if healthScore >= 1.0 {
        t.Errorf("Expected health_score to be less than 1.0, got: %.2f", healthScore)
    }
    if healthy != 1 {
        t.Errorf("Expected healthy to be 1, got: %d", healthy)
    }
}

func TestMultipleErrors(t *testing.T) {
    // Setup
    db := setupTestDB(t)
    quotaManager := NewQuotaManager(nil, logger, db, nil, nil, nil)
    ctx := context.Background()

    // Record multiple errors
    for i := 0; i < 5; i++ {
        err := quotaManager.RecordError(ctx, "provider1", "token1", "rate_limit", "Rate limit exceeded", 429)
        if err != nil {
            t.Fatalf("Failed to record error: %v", err)
        }
    }

    // Verify error count increased
    var errorCount int
    err := db.QueryRow(`
        SELECT error_count
        FROM tokens
        WHERE id = ?
    `, "token1").Scan(&errorCount)

    if err != nil {
        t.Fatalf("Failed to query token: %v", err)
    }

    if errorCount != 5 {
        t.Errorf("Expected error_count to be 5, got: %d", errorCount)
    }
}

func TestResetTokenErrors(t *testing.T) {
    // Setup
    db := setupTestDB(t)
    quotaManager := NewQuotaManager(nil, logger, db, nil, nil, nil)
    ctx := context.Background()

    // Record errors
    quotaManager.RecordError(ctx, "provider1", "token1", "rate_limit", "Error", 429)

    // Reset errors
    err := quotaManager.ResetTokenErrors(ctx, "provider1", "token1")
    if err != nil {
        t.Fatalf("Failed to reset errors: %v", err)
    }

    // Verify reset
    var errorCount int
    var healthScore float64
    var healthy int

    err = db.QueryRow(`
        SELECT error_count, health_score, healthy
        FROM tokens
        WHERE id = ?
    `, "token1").Scan(&errorCount, &healthScore, &healthy)

    if err != nil {
        t.Fatalf("Failed to query token: %v", err)
    }

    if errorCount != 0 {
        t.Errorf("Expected error_count to be 0, got: %d", errorCount)
    }
    if healthScore != 1.0 {
        t.Errorf("Expected health_score to be 1.0, got: %.2f", healthScore)
    }
    if healthy != 1 {
        t.Errorf("Expected healthy to be 1, got: %d", healthy)
    }
}

func TestHealthScoreCalculation(t *testing.T) {
    // Setup
    db := setupTestDB(t)
    quotaManager := NewQuotaManager(nil, logger, db, nil, nil, nil)
    ctx := context.Background()

    // Record rate limit error (should have higher penalty)
    err := quotaManager.RecordError(ctx, "provider1", "token1", "rate_limit", "Rate limit", 429)
    if err != nil {
        t.Fatalf("Failed to record error: %v", err)
    }

    // Get health score
    var healthScore float64
    err = db.QueryRow(`
        SELECT health_score
        FROM tokens
        WHERE id = ?
    `, "token1").Scan(&healthScore)

    if err != nil {
        t.Fatalf("Failed to query token: %v", err)
    }

    // Rate limit should reduce health score significantly
    if healthScore >= 0.8 {
        t.Errorf("Expected health_score to be less than 0.8 for rate limit error, got: %.2f", healthScore)
    }
}
```

---

### Step 7: Add Periodic Health Recovery (Optional)

**File:** [`qwencoder-proxy/internal/ratelimit/quota_manager.go`](qwencoder-proxy/internal/ratelimit/quota_manager.go)

**Add Background Recovery:**

```go
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
```

**Call in main:**

```go
// Start health recovery (runs every 5 minutes)
quotaManager.StartHealthRecovery(context.Background(), 5*time.Minute)
```

---

## Verification Steps

After implementing changes, verify:

1. **Error Recording:**
   - Trigger a rate limit error (429)
   - Query `tokens` table
   - Verify `error_count` incremented
   - Verify `last_error` set
   - Verify `health_score` decreased

2. **Health Status:**
   - Record multiple errors for a token
   - Verify token becomes unhealthy when health_score < 0.3
   - Verify unhealthy tokens are excluded from load balancing

3. **Load Balancing:**
   - Create multiple tokens
   - Make some tokens unhealthy
   - Verify load balancer avoids unhealthy tokens
   - Verify healthy tokens are selected

4. **API Endpoints:**
   - Call `/api/ratelimit/errors?provider_id=xxx`
   - Verify error information returned
   - Call `/api/ratelimit/errors/reset?provider_id=xxx&token_id=yyy`
   - Verify errors reset

5. **Recovery:**
   - Wait for health recovery interval
   - Verify unhealthy tokens with low error counts recover
   - Verify health_score increases

---

## Rollback Plan

If issues arise after deployment:

1. **Disable error recording** by adding feature flag
2. **Revert load balancing changes** to skip healthy check
3. **No data rollback needed** - error tracking is additive

---

## Summary

**Files to Modify:**
1. [`qwencoder-proxy/internal/ratelimit/quota_manager.go`](qwencoder-proxy/internal/ratelimit/quota_manager.go) - Add error recording methods
2. [`qwencoder-proxy/internal/proxy/sequential_handler.go`](qwencoder-proxy/internal/proxy/sequential_handler.go) - Add error handling
3. [`qwencoder-proxy/internal/ratelimit/load_balancing.go`](qwencoder-proxy/internal/ratelimit/load_balancing.go) - Use error data in selection
4. [`qwencoder-proxy/internal/restapi/rate_limit_api.go`](qwencoder-proxy/internal/restapi/rate_limit_api.go) - Add error endpoints (or create new file)
5. Test files - Add error tracking tests

**Expected Outcome:**
- All provider errors are tracked in database
- Token health automatically managed based on errors
- Load balancer avoids unhealthy tokens
- API endpoints provide error visibility
- Automatic recovery of unhealthy tokens
- Better system reliability and observability

**Estimated Effort:** 4-6 hours

---

## Notes for Code Agent

- **Test error classification** with various error types
- **Monitor health scores** to ensure they're reasonable
- **Adjust penalty values** based on production data
- **Add logging** for all error recording operations
- **Consider rate limiting** the error recording to avoid database overload
- **Document API endpoints** for team members
- **Add monitoring** for error rates and token health
- **Consider adding alerts** for high error rates
- **Test recovery mechanism** thoroughly
- **Consider adding exponential backoff** for tokens with many errors
