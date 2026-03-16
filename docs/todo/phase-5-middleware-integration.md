# Phase 5: Middleware Integration - Rate Limiting in Proxy Handlers

**Phase Goal:** Integrate rate limiting middleware into existing proxy handlers.

**Duration:** Week 4-5  
**Status:** Ready to Implement  
**Dependencies:** Phase 1 (Foundation), Phase 2 (Core Rate Limiting), Phase 3 (Async Tracking), Phase 4 (Smart Token Selection)

---

## Task Overview

This phase integrates rate limiting into the proxy request flow:

1. Implementing rate limiting middleware
2. Adding middleware to proxy handlers
3. Implementing usage recording hooks
4. Adding context propagation
5. Writing integration tests

---

## Task 5.1: Implement Rate Limiting Middleware

**File:** `qwencoder-proxy/proxy/rate_limit_middleware.go`

Create the rate limiting middleware for HTTP handlers.

**Implementation Requirements:**

```go
package proxy

import (
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "time"
    
    "github.com/sunbankio/qwencoder-proxy/ratelimit"
)

// RateLimitMiddleware creates middleware for rate limiting
func RateLimitMiddleware(rateLimiter ratelimit.RateLimiter) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            // Extract provider, token, model from context
            // These should be set by the proxy handlers
            providerID, _ := r.Context().Value("provider_id").(string)
            tokenID, _ := r.Context().Value("token_id").(string)
            model, _ := r.Context().Value("model").(string)
            
            // If no context values, skip rate limiting
            if providerID == "" || tokenID == "" {
                next.ServeHTTP(w, r)
                return
            }
            
            // Check rate limit
            checkReq := &ratelimit.LimitCheckRequest{
                ProviderID: providerID,
                TokenID:    tokenID,
                Model:      model,
            }
            
            result, err := rateLimiter.CheckLimit(r.Context(), checkReq)
            if err != nil {
                // Log error but allow request (fail open)
                // Add error to context for potential logging
                ctx := context.WithValue(r.Context(), "rate_limit_error", err)
                next.ServeHTTP(w, r.WithContext(ctx))
                return
            }
            
            if !result.Allowed {
                // Return 429 Too Many Requests
                writeRateLimitResponse(w, result)
                return
            }
            
            // Add quota info to context
            ctx := context.WithValue(r.Context(), "quota_info", result.AvailableQuota)
            
            // Record usage start time
            startTime := time.Now()
            ctx = context.WithValue(ctx, "request_start_time", startTime)
            
            // Wrap response writer to capture response
            rw := &responseWriter{ResponseWriter: w}
            
            // Call next handler
            next.ServeHTTP(rw, r.WithContext(ctx))
            
            // Record usage (non-blocking)
            recordUsage(rateLimiter, providerID, tokenID, model, startTime, rw.statusCode)
        })
    }
}

// writeRateLimitResponse writes a rate limit exceeded response
func writeRateLimitResponse(w http.ResponseWriter, result *ratelimit.LimitCheckResult) {
    w.Header().Set("Content-Type", "application/json")
    w.Header().Set("Retry-After", result.RetryAfter.String())
    w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", result.AvailableQuota.DailyLimit))
    w.Header().Set("X-RateLimit-Remaining", fmt.Sprintf("%d", result.AvailableQuota.DailyRemaining))
    w.Header().Set("X-RateLimit-Reset", fmt.Sprintf("%d", time.Now().Add(result.RetryAfter).Unix()))
    
    w.WriteHeader(http.StatusTooManyRequests)
    
    response := map[string]interface{}{
        "error":        "rate_limit_exceeded",
        "message":      "Rate limit exceeded. Please retry later.",
        "retry_after":  result.RetryAfter.String(),
        "quota_info":   result.AvailableQuota,
    }
    
    json.NewEncoder(w).Encode(response)
}

// responseWriter wraps http.ResponseWriter to capture status code
type responseWriter struct {
    http.ResponseWriter
    statusCode int
    written    bool
}

// WriteHeader captures the status code
func (rw *responseWriter) WriteHeader(code int) {
    if !rw.written {
        rw.statusCode = code
        rw.written = true
        rw.ResponseWriter.WriteHeader(code)
    }
}

// Write captures writes
func (rw *responseWriter) Write(b []byte) (int, error) {
    if !rw.written {
        rw.WriteHeader(http.StatusOK)
    }
    return rw.ResponseWriter.Write(b)
}

// recordUsage records usage metrics (non-blocking)
func recordUsage(
    rateLimiter ratelimit.RateLimiter,
    providerID, tokenID, model string,
    startTime time.Time,
    statusCode int,
) {
    // Create usage record
    usage := &ratelimit.UsageRecord{
        ProviderID:     providerID,
        TokenID:        tokenID,
        Model:          model,
        RequestType:    "chat", // TODO: Extract from request
        RequestCount:   1,
        TokenCount:     0, // TODO: Extract from response if available
        Timestamp:      time.Now(),
        ResponseTimeMs: time.Since(startTime).Milliseconds(),
        Success:        statusCode < 400,
        ErrorCode:      "",
    }
    
    // Record usage (non-blocking)
    ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
    defer cancel()
    
    if err := rateLimiter.RecordUsage(ctx, usage); err != nil {
        // Log error but don't block
        // In production, you might want to use a separate logger
        fmt.Printf("[RateLimitMiddleware] Failed to record usage: %v\n", err)
    }
}
```

---

## Task 5.2: Add Middleware to Proxy Handlers

**File:** `qwencoder-proxy/proxy/openai_handler.go` (and other handler files)

Update existing proxy handlers to set context values for rate limiting.

**Implementation Requirements:**

Add context values before proxying the request:

```go
// In OpenAI handler (and similar handlers)
func (h *OpenAIHandler) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
    // Select token
    token, err := h.tokenManager.SelectToken()
    if err != nil {
        writeError(w, err)
        return
    }
    
    // Extract model from request
    var req OpenAIChatRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        writeError(w, err)
        return
    }
    
    // Set context values for rate limiting
    ctx := context.WithValue(r.Context(), "provider_id", string(provider.ProviderQwen))
    ctx = context.WithValue(ctx, "token_id", token.ID)
    ctx = context.WithValue(ctx, "model", req.Model)
    
    // Proxy the request with updated context
    r = r.WithContext(ctx)
    
    // Continue with existing proxy logic...
}
```

---

## Task 5.3: Implement Usage Recording Hooks

The usage recording is already implemented in `recordUsage()`. Enhance it to:

1. Extract token count from response if available
2. Extract request type from request
3. Add more detailed error codes

**Enhanced Implementation:**

```go
// recordUsage records usage metrics (non-blocking)
func recordUsage(
    rateLimiter ratelimit.RateLimiter,
    providerID, tokenID, model string,
    startTime time.Time,
    statusCode int,
    response []byte, // NEW: pass response for token extraction
) {
    // Extract token count from response if available
    tokenCount := extractTokenCount(response)
    
    // Extract request type from path
    requestType := extractRequestType(startTime)
    
    // Determine error code
    errorCode := ""
    if statusCode >= 400 {
        errorCode = extractErrorCode(response)
    }
    
    // Create usage record
    usage := &ratelimit.UsageRecord{
        ProviderID:     providerID,
        TokenID:        tokenID,
        Model:          model,
        RequestType:    requestType,
        RequestCount:   1,
        TokenCount:     tokenCount,
        Timestamp:      time.Now(),
        ResponseTimeMs: time.Since(startTime).Milliseconds(),
        Success:        statusCode < 400,
        ErrorCode:      errorCode,
    }
    
    // Record usage (non-blocking)
    ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
    defer cancel()
    
    if err := rateLimiter.RecordUsage(ctx, usage); err != nil {
        fmt.Printf("[RateLimitMiddleware] Failed to record usage: %v\n", err)
    }
}

// extractTokenCount extracts token count from response
func extractTokenCount(response []byte) int {
    // Parse response to extract token count
    // This is provider-specific
    // For OpenAI format:
    var result map[string]interface{}
    if err := json.Unmarshal(response, &result); err != nil {
        return 0
    }
    
    if usage, ok := result["usage"].(map[string]interface{}); ok {
        if totalTokens, ok := usage["total_tokens"].(float64); ok {
            return int(totalTokens)
        }
    }
    
    return 0
}

// extractRequestType extracts request type
func extractRequestType(startTime time.Time) string {
    // This would be extracted from the request path/method
    // For now, default to "chat"
    return "chat"
}

// extractErrorCode extracts error code from response
func extractErrorCode(response []byte) string {
    // Parse response to extract error code
    var result map[string]interface{}
    if err := json.Unmarshal(response, &result); err != nil {
        return ""
    }
    
    if err, ok := result["error"].(map[string]interface{}); ok {
        if code, ok := err["code"].(string); ok {
            return code
        }
    }
    
    return ""
}
```

---

## Task 5.4: Add Context Propagation

Context propagation is already implemented in the middleware. Ensure:

1. All relevant context values are set
2. Context values are properly typed
3. Context is passed through the request chain

**Context Keys:**

```go
// Define context keys in a separate file for reusability
// File: qwencoder-proxy/proxy/context.go

package proxy

import "context"

type contextKey string

const (
    ContextKeyProviderID     contextKey = "provider_id"
    ContextKeyTokenID       contextKey = "token_id"
    ContextKeyModel         contextKey = "model"
    ContextKeyQuotaInfo    contextKey = "quota_info"
    ContextKeyRequestStart contextKey = "request_start_time"
    ContextKeyRateLimitError contextKey = "rate_limit_error"
)

// Helper functions for context values
func GetProviderID(ctx context.Context) string {
    if val, ok := ctx.Value(ContextKeyProviderID).(string); ok {
        return val
    }
    return ""
}

func GetTokenID(ctx context.Context) string {
    if val, ok := ctx.Value(ContextKeyTokenID).(string); ok {
        return val
    }
    return ""
}

func GetModel(ctx context.Context) string {
    if val, ok := ctx.Value(ContextKeyModel).(string); ok {
        return val
    }
    return ""
}

func GetQuotaInfo(ctx context.Context) *ratelimit.QuotaInfo {
    if val, ok := ctx.Value(ContextKeyQuotaInfo).(*ratelimit.QuotaInfo); ok {
        return val
    }
    return nil
}

// Update middleware to use these keys
// Instead of:
// ctx := context.WithValue(r.Context(), "provider_id", providerID)
// Use:
// ctx := context.WithValue(r.Context(), ContextKeyProviderID, providerID)
```

---

## Task 5.5: Write Integration Tests

**File:** `qwencoder-proxy/proxy/rate_limit_middleware_test.go`

Write integration tests for the rate limiting middleware.

**Test Cases:**

1. **Rate Limit Check:**
   - Test request allowed when under limit
   - Test request denied when over limit
   - Test proper error response

2. **Usage Recording:**
   - Test usage is recorded after request
   - Test usage recording doesn't block

3. **Context Propagation:**
   - Test context values are set correctly
   - Test context values are accessible

**Test Template:**

```go
package proxy

import (
    "context"
    "net/http"
    "net/http/httptest"
    "testing"
    "time"
    
    "github.com/sunbankio/qwencoder-proxy/ratelimit"
)

func setupTestMiddleware(t *testing.T) (http.Handler, ratelimit.RateLimiter) {
    limiter := setupMockRateLimiter(t)
    
    // Create test handler
    handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
        w.Write([]byte(`{"usage": {"total_tokens": 100}}`))
    })
    
    // Apply middleware
    middleware := RateLimitMiddleware(limiter)
    return middleware(handler), limiter
}

func TestRateLimitMiddleware_Allowed(t *testing.T) {
    handler, limiter := setupTestMiddleware(t)
    
    // Set up mock to allow request
    limiter.SetMockCheckResult(&ratelimit.LimitCheckResult{
        Allowed: true,
        Reason:  "ok",
    })
    
    // Create request with context
    ctx := context.Background()
    ctx = context.WithValue(ctx, ContextKeyProviderID, "qwen")
    ctx = context.WithValue(ctx, ContextKeyTokenID, "token-1")
    ctx = context.WithValue(ctx, ContextKeyModel, "qwen-turbo")
    
    req := httptest.NewRequest("GET", "/test", nil).WithContext(ctx)
    w := httptest.NewRecorder()
    
    handler.ServeHTTP(w, req)
    
    if w.Code != http.StatusOK {
        t.Errorf("Expected status 200, got %d", w.Code)
    }
}

func TestRateLimitMiddleware_Denied(t *testing.T) {
    handler, limiter := setupTestMiddleware(t)
    
    // Set up mock to deny request
    limiter.SetMockCheckResult(&ratelimit.LimitCheckResult{
        Allowed:    false,
        Reason:     "daily_limit",
        RetryAfter: 60 * time.Second,
        AvailableQuota: &ratelimit.QuotaInfo{
            DailyLimit:     1000,
            DailyUsed:      1000,
            DailyRemaining: 0,
        },
    })
    
    // Create request with context
    ctx := context.Background()
    ctx = context.WithValue(ctx, ContextKeyProviderID, "qwen")
    ctx = context.WithValue(ctx, ContextKeyTokenID, "token-1")
    ctx = context.WithValue(ctx, ContextKeyModel, "qwen-turbo")
    
    req := httptest.NewRequest("GET", "/test", nil).WithContext(ctx)
    w := httptest.NewRecorder()
    
    handler.ServeHTTP(w, req)
    
    if w.Code != http.StatusTooManyRequests {
        t.Errorf("Expected status 429, got %d", w.Code)
    }
    
    // Check response headers
    retryAfter := w.Header().Get("Retry-After")
    if retryAfter == "" {
        t.Error("Retry-After header not set")
    }
}

// Implement more tests...
```

---

## Deliverables

After completing this phase, you should have:

1. ✅ Rate limiting middleware implementation
2. ✅ Middleware integrated into proxy handlers
3. ✅ Usage recording hooks
4. ✅ Context propagation
5. ✅ Integration tests

---

## Success Criteria

- [ ] Rate limiting is applied to all proxy requests
- [ ] Requests over limit are properly rejected with 429
- [ ] Usage is recorded for all requests
- [ ] Context values are properly propagated
- [ ] All integration tests pass
- [ ] Middleware doesn't add significant latency (<1ms)
- [ ] Error handling is comprehensive

---

## Next Phase

After completing Phase 5, proceed to **Phase 6: Admin API** which implements REST API for rate limit management.

**File:** `../todo/phase-6-admin-api.md`
