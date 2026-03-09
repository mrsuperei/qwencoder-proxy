# Phase 6: Admin API - REST API for Rate Limit Management

**Phase Goal:** Implement REST API endpoints for rate limit management and usage monitoring.

**Duration:** Week 5-6  
**Status:** Ready to Implement  
**Dependencies:** Phase 1 (Foundation), Phase 2 (Core Rate Limiting), Phase 3 (Async Tracking)

---

## Task Overview

This phase implements the admin REST API for rate limiting:

1. Implementing rate limit CRUD handlers
2. Implementing usage query handlers
3. Implementing quota info handlers
4. Adding API routes to REST server
5. Adding authentication/authorization
6. Writing API tests

---

## Task 6.1: Implement Rate Limit CRUD Handlers

**File:** `qwencoder-proxy/ratelimit/api/handlers.go`

Create REST API handlers for rate limit management.

**Implementation Requirements:**

```go
package api

import (
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "time"
    
    "github.com/google/uuid"
    "github.com/sunbankio/qwencoder-proxy/ratelimit"
    "github.com/sunbankio/qwencoder-proxy/ratelimit/store"
)

// RateLimitAPI handles rate limit API endpoints
type RateLimitAPI struct {
    rateLimiter ratelimit.RateLimiter
    logger      Logger
}

// NewRateLimitAPI creates a new rate limit API
func NewRateLimitAPI(rateLimiter ratelimit.RateLimiter, logger Logger) *RateLimitAPI {
    return &RateLimitAPI{
        rateLimiter: rateLimiter,
        logger:      logger,
    }
}

// RegisterRoutes registers all rate limit API routes
func (api *RateLimitAPI) RegisterRoutes(mux *http.ServeMux) {
    // Rate limit endpoints
    mux.HandleFunc("/api/rate-limits", api.handleRateLimits)
    mux.HandleFunc("/api/rate-limits/", api.handleRateLimitByID)
    
    // Usage endpoints
    mux.HandleFunc("/api/usage", api.handleUsage)
    mux.HandleFunc("/api/usage/provider/", api.handleUsageByProvider)
    mux.HandleFunc("/api/usage/token/", api.handleUsageByToken)
    mux.HandleFunc("/api/usage/history", api.handleUsageHistory)
    mux.HandleFunc("/api/usage/export", api.handleUsageExport)
    
    // Quota endpoints
    mux.HandleFunc("/api/quota", api.handleQuota)
    mux.HandleFunc("/api/quota/provider/", api.handleQuotaByProvider)
    mux.HandleFunc("/api/quota/token/", api.handleQuotaByToken)
    mux.HandleFunc("/api/quota/summary", api.handleQuotaSummary)
}

// handleRateLimits handles GET/POST for rate limits
func (api *RateLimitAPI) handleRateLimits(w http.ResponseWriter, r *http.Request) {
    switch r.Method {
    case http.MethodGet:
        api.listRateLimits(w, r)
    case http.MethodPost:
        api.createRateLimit(w, r)
    default:
        writeMethodNotAllowed(w)
    }
}

// handleRateLimitByID handles GET/PUT/DELETE for a specific rate limit
func (api *RateLimitAPI) handleRateLimitByID(w http.ResponseWriter, r *http.Request) {
    // Extract ID from path
    id := extractIDFromPath(r.URL.Path)
    if id == "" {
        writeBadRequest(w, "Missing rate limit ID")
        return
    }
    
    switch r.Method {
    case http.MethodGet:
        api.getRateLimit(w, r, id)
    case http.MethodPut:
        api.updateRateLimit(w, r, id)
    case http.MethodDelete:
        api.deleteRateLimit(w, r, id)
    case http.MethodPost:
        // Handle enable/disable
        if r.URL.Path[len(id):] == "/enable" {
            api.enableRateLimit(w, r, id)
        } else if r.URL.Path[len(id):] == "/disable" {
            api.disableRateLimit(w, r, id)
        } else {
            writeMethodNotAllowed(w)
        }
    default:
        writeMethodNotAllowed(w)
    }
}

// listRateLimits lists all rate limits
func (api *RateLimitAPI) listRateLimits(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    
    // Parse filters from query params
    filter := &store.RateLimitFilter{}
    if providerID := r.URL.Query().Get("provider_id"); providerID != "" {
        filter.ProviderID = providerID
    }
    if tokenID := r.URL.Query().Get("token_id"); tokenID != "" {
        filter.TokenID = tokenID
    }
    if model := r.URL.Query().Get("model"); model != "" {
        filter.Model = model
    }
    if limitType := r.URL.Query().Get("limit_type"); limitType != "" {
        filter.LimitType = limitType
    }
    if enabledStr := r.URL.Query().Get("enabled"); enabledStr != "" {
        enabled := enabledStr == "true"
        filter.Enabled = &enabled
    }
    
    // Get rate limits
    limits, err := api.rateLimiter.GetApplicableLimits(ctx, 
        filter.ProviderID, filter.TokenID, filter.Model)
    if err != nil {
        writeError(w, http.StatusInternalServerError, 
            fmt.Sprintf("Failed to get rate limits: %v", err))
        return
    }
    
    writeJSON(w, http.StatusOK, map[string]interface{}{
        "rate_limits": limits,
        "count":       len(limits),
    })
}

// createRateLimit creates a new rate limit
func (api *RateLimitAPI) createRateLimit(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    
    // Parse request body
    var req CreateRateLimitRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        writeError(w, http.StatusBadRequest, 
            fmt.Sprintf("Invalid request body: %v", err))
        return
    }
    
    // Validate request
    if err := req.Validate(); err != nil {
        writeError(w, http.StatusBadRequest, err.Error())
        return
    }
    
    // Create rate limit config
    now := time.Now()
    config := &ratelimit.RateLimitConfig{
        ID:          uuid.New().String(),
        ProviderID:  req.ProviderID,
        TokenID:     req.TokenID,
        Model:       req.Model,
        LimitType:   req.LimitType,
        LimitValue:  req.LimitValue,
        TimeWindow:  req.TimeWindow,
        Enabled:     true,
        Priority:    req.Priority,
        CreatedAt:   now,
        UpdatedAt:   now,
    }
    
    // Store rate limit
    if err := api.rateLimiter.CreateRateLimit(ctx, config); err != nil {
        writeError(w, http.StatusInternalServerError, 
            fmt.Sprintf("Failed to create rate limit: %v", err))
        return
    }
    
    writeJSON(w, http.StatusCreated, config)
}

// getRateLimit gets a specific rate limit
func (api *RateLimitAPI) getRateLimit(w http.ResponseWriter, r *http.Request, id string) {
    ctx := r.Context()
    
    // Get rate limit
    limit, err := api.rateLimiter.GetRateLimit(ctx, id)
    if err != nil {
        if err == ratelimit.ErrNotFound {
            writeError(w, http.StatusNotFound, "Rate limit not found")
            return
        }
        writeError(w, http.StatusInternalServerError, 
            fmt.Sprintf("Failed to get rate limit: %v", err))
        return
    }
    
    writeJSON(w, http.StatusOK, limit)
}

// updateRateLimit updates a rate limit
func (api *RateLimitAPI) updateRateLimit(w http.ResponseWriter, r *http.Request, id string) {
    ctx := r.Context()
    
    // Parse request body
    var req UpdateRateLimitRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        writeError(w, http.StatusBadRequest, 
            fmt.Sprintf("Invalid request body: %v", err))
        return
    }
    
    // Get existing rate limit
    limit, err := api.rateLimiter.GetRateLimit(ctx, id)
    if err != nil {
        if err == ratelimit.ErrNotFound {
            writeError(w, http.StatusNotFound, "Rate limit not found")
            return
        }
        writeError(w, http.StatusInternalServerError, 
            fmt.Sprintf("Failed to get rate limit: %v", err))
        return
    }
    
    // Update fields
    if req.LimitValue != nil {
        limit.LimitValue = *req.LimitValue
    }
    if req.TimeWindow != nil {
        limit.TimeWindow = *req.TimeWindow
    }
    if req.Enabled != nil {
        limit.Enabled = *req.Enabled
    }
    if req.Priority != nil {
        limit.Priority = *req.Priority
    }
    limit.UpdatedAt = time.Now()
    
    // Update rate limit
    if err := api.rateLimiter.UpdateRateLimit(ctx, id, limit); err != nil {
        writeError(w, http.StatusInternalServerError, 
            fmt.Sprintf("Failed to update rate limit: %v", err))
        return
    }
    
    writeJSON(w, http.StatusOK, limit)
}

// deleteRateLimit deletes a rate limit
func (api *RateLimitAPI) deleteRateLimit(w http.ResponseWriter, r *http.Request, id string) {
    ctx := r.Context()
    
    // Delete rate limit
    if err := api.rateLimiter.DeleteRateLimit(ctx, id); err != nil {
        if err == ratelimit.ErrNotFound {
            writeError(w, http.StatusNotFound, "Rate limit not found")
            return
        }
        writeError(w, http.StatusInternalServerError, 
            fmt.Sprintf("Failed to delete rate limit: %v", err))
        return
    }
    
    writeJSON(w, http.StatusNoContent, nil)
}

// enableRateLimit enables a rate limit
func (api *RateLimitAPI) enableRateLimit(w http.ResponseWriter, r *http.Request, id string) {
    ctx := r.Context()
    
    // Enable rate limit
    if err := api.rateLimiter.EnableRateLimit(ctx, id); err != nil {
        if err == ratelimit.ErrNotFound {
            writeError(w, http.StatusNotFound, "Rate limit not found")
            return
        }
        writeError(w, http.StatusInternalServerError, 
            fmt.Sprintf("Failed to enable rate limit: %v", err))
        return
    }
    
    writeJSON(w, http.StatusOK, map[string]string{
        "status": "enabled",
        "id":     id,
    })
}

// disableRateLimit disables a rate limit
func (api *RateLimitAPI) disableRateLimit(w http.ResponseWriter, r *http.Request, id string) {
    ctx := r.Context()
    
    // Disable rate limit
    if err := api.rateLimiter.DisableRateLimit(ctx, id); err != nil {
        if err == ratelimit.ErrNotFound {
            writeError(w, http.StatusNotFound, "Rate limit not found")
            return
        }
        writeError(w, http.StatusInternalServerError, 
            fmt.Sprintf("Failed to disable rate limit: %v", err))
        return
    }
    
    writeJSON(w, http.StatusOK, map[string]string{
        "status": "disabled",
        "id":     id,
    })
}

// Request/Response types
type CreateRateLimitRequest struct {
    ProviderID string        `json:"provider_id" binding:"required"`
    TokenID    string        `json:"token_id"`
    Model      string        `json:"model"`
    LimitType  string        `json:"limit_type" binding:"required"`
    LimitValue int64         `json:"limit_value" binding:"required"`
    TimeWindow time.Duration `json:"time_window"`
    Priority   int           `json:"priority"`
}

func (r *CreateRateLimitRequest) Validate() error {
    if r.ProviderID == "" {
        return fmt.Errorf("provider_id is required")
    }
    if r.LimitType == "" {
        return fmt.Errorf("limit_type is required")
    }
    if r.LimitValue <= 0 {
        return fmt.Errorf("limit_value must be positive")
    }
    return nil
}

type UpdateRateLimitRequest struct {
    LimitValue *int64         `json:"limit_value"`
    TimeWindow *time.Duration `json:"time_window"`
    Enabled    *bool          `json:"enabled"`
    Priority   *int           `json:"priority"`
}
```

---

## Task 6.2: Implement Usage Query Handlers

**File:** `qwencoder-proxy/ratelimit/api/usage_handlers.go`

Create REST API handlers for usage monitoring.

**Implementation Requirements:**

```go
package api

// handleUsage handles GET for usage statistics
func (api *RateLimitAPI) handleUsage(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodGet {
        writeMethodNotAllowed(w)
        return
    }
    
    ctx := r.Context()
    
    // Parse query parameters
    query := &store.UsageQuery{
        ProviderID: r.URL.Query().Get("provider_id"),
        TokenID:    r.URL.Query().Get("token_id"),
        Model:      r.URL.Query().Get("model"),
    }
    
    // Parse time window
    if window := r.URL.Query().Get("window"); window != "" {
        query.Window = store.TimeWindow(window)
    }
    
    // Parse time range
    if startTime := r.URL.Query().Get("start_time"); startTime != "" {
        if t, err := time.Parse(time.RFC3339, startTime); err == nil {
            query.StartTime = t
        }
    }
    if endTime := r.URL.Query().Get("end_time"); endTime != "" {
        if t, err := time.Parse(time.RFC3339, endTime); err == nil {
            query.EndTime = t
        }
    }
    
    // Get usage statistics
    usage, err := api.rateLimiter.GetUsage(ctx, query)
    if err != nil {
        writeError(w, http.StatusInternalServerError, 
            fmt.Sprintf("Failed to get usage: %v", err))
        return
    }
    
    writeJSON(w, http.StatusOK, usage)
}

// handleUsageByProvider handles GET for usage by provider
func (api *RateLimitAPI) handleUsageByProvider(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodGet {
        writeMethodNotAllowed(w)
        return
    }
    
    // Extract provider ID from path
    providerID := extractIDFromPath(r.URL.Path[len("/api/usage/provider/"):])
    if providerID == "" {
        writeBadRequest(w, "Missing provider ID")
        return
    }
    
    ctx := r.Context()
    query := &store.UsageQuery{
        ProviderID: providerID,
    }
    
    // Get usage
    usage, err := api.rateLimiter.GetUsage(ctx, query)
    if err != nil {
        writeError(w, http.StatusInternalServerError, 
            fmt.Sprintf("Failed to get usage: %v", err))
        return
    }
    
    writeJSON(w, http.StatusOK, usage)
}

// handleUsageByToken handles GET for usage by token
func (api *RateLimitAPI) handleUsageByToken(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodGet {
        writeMethodNotAllowed(w)
        return
    }
    
    // Extract token ID from path
    tokenID := extractIDFromPath(r.URL.Path[len("/api/usage/token/"):])
    if tokenID == "" {
        writeBadRequest(w, "Missing token ID")
        return
    }
    
    ctx := r.Context()
    query := &store.UsageQuery{
        TokenID: tokenID,
    }
    
    // Get usage
    usage, err := api.rateLimiter.GetUsage(ctx, query)
    if err != nil {
        writeError(w, http.StatusInternalServerError, 
            fmt.Sprintf("Failed to get usage: %v", err))
        return
    }
    
    writeJSON(w, http.StatusOK, usage)
}

// handleUsageHistory handles GET for usage history
func (api *RateLimitAPI) handleUsageHistory(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodGet {
        writeMethodNotAllowed(w)
        return
    }
    
    ctx := r.Context()
    
    // Parse query parameters
    query := &store.HistoryQuery{
        ProviderID: r.URL.Query().Get("provider_id"),
        TokenID:    r.URL.Query().Get("token_id"),
        Model:      r.URL.Query().Get("model"),
    }
    
    // Parse time range
    if startTime := r.URL.Query().Get("start_time"); startTime != "" {
        if t, err := time.Parse(time.RFC3339, startTime); err == nil {
            query.StartTime = t
        }
    }
    if endTime := r.URL.Query().Get("end_time"); endTime != "" {
        if t, err := time.Parse(time.RFC3339, endTime); err == nil {
            query.EndTime = t
        }
    }
    
    // Parse limit and offset
    if limit := r.URL.Query().Get("limit"); limit != "" {
        if l, err := strconv.Atoi(limit); err == nil {
            query.Limit = l
        }
    }
    if offset := r.URL.Query().Get("offset"); offset != "" {
        if o, err := strconv.Atoi(offset); err == nil {
            query.Offset = o
        }
    }
    
    // Get usage history
    history, err := api.rateLimiter.GetUsageHistory(ctx, query)
    if err != nil {
        writeError(w, http.StatusInternalServerError, 
            fmt.Sprintf("Failed to get usage history: %v", err))
        return
    }
    
    writeJSON(w, http.StatusOK, map[string]interface{}{
        "history": history,
        "count":   len(history),
    })
}

// handleUsageExport handles GET for usage export
func (api *RateLimitAPI) handleUsageExport(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodGet {
        writeMethodNotAllowed(w)
        return
    }
    
    ctx := r.Context()
    
    // Parse format
    format := r.URL.Query().Get("format")
    if format == "" {
        format = "json"
    }
    
    // Get usage history
    query := &store.HistoryQuery{
        StartTime: time.Now().Add(-24 * time.Hour),
        EndTime:   time.Now(),
    }
    
    history, err := api.rateLimiter.GetUsageHistory(ctx, query)
    if err != nil {
        writeError(w, http.StatusInternalServerError, 
            fmt.Sprintf("Failed to get usage history: %v", err))
        return
    }
    
    // Export based on format
    switch format {
    case "csv":
        w.Header().Set("Content-Type", "text/csv")
        w.Header().Set("Content-Disposition", "attachment; filename=usage.csv")
        api.exportCSV(w, history)
    default:
        w.Header().Set("Content-Type", "application/json")
        w.Header().Set("Content-Disposition", "attachment; filename=usage.json")
        writeJSON(w, http.StatusOK, history)
    }
}

// exportCSV exports usage history as CSV
func (api *RateLimitAPI) exportCSV(w http.ResponseWriter, history []*ratelimit.UsageRecord) {
    // Write CSV header
    fmt.Fprintf(w, "Timestamp,ProviderID,TokenID,Model,RequestType,RequestCount,TokenCount,ResponseTimeMs,Success,ErrorCode\n")
    
    // Write data rows
    for _, record := range history {
        fmt.Fprintf(w, "%s,%s,%s,%s,%s,%d,%d,%d,%t,%s\n",
            record.Timestamp.Format(time.RFC3339),
            record.ProviderID,
            record.TokenID,
            record.Model,
            record.RequestType,
            record.RequestCount,
            record.TokenCount,
            record.ResponseTimeMs,
            record.Success,
            record.ErrorCode,
        )
    }
}
```

---

## Task 6.3: Implement Quota Info Handlers

**File:** `qwencoder-proxy/ratelimit/api/quota_handlers.go`

Create REST API handlers for quota information.

**Implementation Requirements:**

```go
package api

// handleQuota handles GET for overall quota status
func (api *RateLimitAPI) handleQuota(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodGet {
        writeMethodNotAllowed(w)
        return
    }
    
    ctx := r.Context()
    
    // Get quota summary
    // This would aggregate quota across all providers and tokens
    summary := map[string]interface{}{
        "providers": api.getProviderQuotaSummary(ctx),
        "tokens":    api.getTokenQuotaSummary(ctx),
    }
    
    writeJSON(w, http.StatusOK, summary)
}

// handleQuotaByProvider handles GET for provider quota
func (api *RateLimitAPI) handleQuotaByProvider(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodGet {
        writeMethodNotAllowed(w)
        return
    }
    
    // Extract provider ID from path
    providerID := extractIDFromPath(r.URL.Path[len("/api/quota/provider/"):])
    if providerID == "" {
        writeBadRequest(w, "Missing provider ID")
        return
    }
    
    ctx := r.Context()
    
    // Get provider quota
    quota, err := api.rateLimiter.GetProviderQuota(ctx, providerID)
    if err != nil {
        writeError(w, http.StatusInternalServerError, 
            fmt.Sprintf("Failed to get quota: %v", err))
        return
    }
    
    writeJSON(w, http.StatusOK, quota)
}

// handleQuotaByToken handles GET for token quota
func (api *RateLimitAPI) handleQuotaByToken(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodGet {
        writeMethodNotAllowed(w)
        return
    }
    
    // Extract token ID from path
    tokenID := extractIDFromPath(r.URL.Path[len("/api/quota/token/"):])
    if tokenID == "" {
        writeBadRequest(w, "Missing token ID")
        return
    }
    
    ctx := r.Context()
    
    // Get token quota
    quota, err := api.rateLimiter.GetTokenQuota(ctx, "", tokenID)
    if err != nil {
        writeError(w, http.StatusInternalServerError, 
            fmt.Sprintf("Failed to get quota: %v", err))
        return
    }
    
    writeJSON(w, http.StatusOK, quota)
}

// handleQuotaSummary handles GET for quota summary
func (api *RateLimitAPI) handleQuotaSummary(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodGet {
        writeMethodNotAllowed(w)
        return
    }
    
    ctx := r.Context()
    
    // Get quota summary across all providers
    summary := api.getQuotaSummary(ctx)
    
    writeJSON(w, http.StatusOK, summary)
}

// Helper functions
func (api *RateLimitAPI) getProviderQuotaSummary(ctx context.Context) map[string]interface{} {
    providers := []string{"qwen", "gemini-cli", "antigravity", "kiro", "iflow"}
    summary := make(map[string]interface{})
    
    for _, providerID := range providers {
        quota, err := api.rateLimiter.GetProviderQuota(ctx, providerID)
        if err != nil {
            api.logger.WarnLog("[API] Failed to get quota for provider %s: %v", providerID, err)
            continue
        }
        summary[providerID] = quota
    }
    
    return summary
}

func (api *RateLimitAPI) getTokenQuotaSummary(ctx context.Context) map[string]interface{} {
    // Get all tokens from store
    // This would require access to the token store
    // For now, return empty map
    return make(map[string]interface{})
}

func (api *RateLimitAPI) getQuotaSummary(ctx context.Context) map[string]interface{} {
    return map[string]interface{}{
        "total_providers": 5,
        "total_tokens":    0, // Would be calculated from token store
        "providers":       api.getProviderQuotaSummary(ctx),
    }
}
```

---

## Task 6.4: Add API Routes to REST Server

**File:** `qwencoder-proxy/restapi/rest_api.go`

Update the REST API server to register rate limiting endpoints.

**Implementation Requirements:**

Add to `Server` struct:

```go
type Server struct {
    config            *Config
    registry          *ProviderRegistry
    stateManager      *StateManager
    logger            logging.Logger
    httpClient        *http.Client
    tokenStores       map[string]tokpkg.TokenStore
    tokenManagers     map[string]*tokpkg.TokenManager
    multiTokenManager *tokpkg.MultiTokenManager
    rateLimiter       ratelimit.RateLimiter  // NEW
    rateLimitAPI      *api.RateLimitAPI      // NEW
}

// Add SetRateLimiter method
func (s *Server) SetRateLimiter(rl ratelimit.RateLimiter) {
    s.rateLimiter = rl
    s.rateLimitAPI = api.NewRateLimitAPI(rl, s.logger)
}

// Update registerRoutes to include rate limiting endpoints
func (s *Server) registerRoutes(mux *http.ServeMux) {
    // ... existing routes ...
    
    // Rate limiting endpoints
    if s.rateLimitAPI != nil {
        s.rateLimitAPI.RegisterRoutes(mux)
    }
}
```

---

## Task 6.5: Add Authentication/Authorization

For now, use basic authentication. In production, you'd want to use a more robust authentication mechanism.

**Implementation Requirements:**

```go
// Add authentication middleware
func (api *RateLimitAPI) authMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Check for API key in header
        apiKey := r.Header.Get("X-API-Key")
        if apiKey == "" {
            writeError(w, http.StatusUnauthorized, "API key required")
            return
        }
        
        // Validate API key (for now, just check it's not empty)
        // In production, you'd validate against a database or config
        if apiKey == "" {
            writeError(w, http.StatusUnauthorized, "Invalid API key")
            return
        }
        
        // Add user info to context
        ctx := context.WithValue(r.Context(), "api_key", apiKey)
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}

// Update RegisterRoutes to use auth middleware
func (api *RateLimitAPI) RegisterRoutes(mux *http.ServeMux) {
    // Apply auth middleware to all routes
    auth := api.authMiddleware
    
    // Rate limit endpoints
    mux.Handle("/api/rate-limits", auth(http.HandlerFunc(api.handleRateLimits)))
    mux.Handle("/api/rate-limits/", auth(http.HandlerFunc(api.handleRateLimitByID)))
    
    // Usage endpoints
    mux.Handle("/api/usage", auth(http.HandlerFunc(api.handleUsage)))
    // ... other routes ...
}
```

---

## Task 6.6: Write API Tests

**File:** `qwencoder-proxy/ratelimit/api/handlers_test.go`

Write comprehensive API tests.

**Test Cases:**

1. **Rate Limit CRUD Tests:**
   - Test creating a rate limit
   - Test getting a rate limit
   - Test listing rate limits
   - Test updating a rate limit
   - Test deleting a rate limit
   - Test enabling/disabling rate limits

2. **Usage Query Tests:**
   - Test getting usage statistics
   - Test getting usage by provider
   - Test getting usage by token
   - Test getting usage history
   - Test exporting usage

3. **Quota Info Tests:**
   - Test getting overall quota
   - Test getting provider quota
   - Test getting token quota
   - Test getting quota summary

**Test Template:**

```go
package api

import (
    "bytes"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"
    "time"
    
    "github.com/sunbankio/qwencoder-proxy/ratelimit"
)

func setupTestAPI(t *testing.T) *RateLimitAPI {
    limiter := setupMockRateLimiter(t)
    logger := newTestLogger()
    return NewRateLimitAPI(limiter, logger)
}

func TestCreateRateLimit(t *testing.T) {
    api := setupTestAPI(t)
    
    req := CreateRateLimitRequest{
        ProviderID: "qwen",
        LimitType:  "daily",
        LimitValue: 1000,
        TimeWindow: 24 * time.Hour,
        Priority:   10,
    }
    
    body, _ := json.Marshal(req)
    r := httptest.NewRequest("POST", "/api/rate-limits", bytes.NewReader(body))
    w := httptest.NewRecorder()
    
    api.createRateLimit(w, r)
    
    if w.Code != http.StatusCreated {
        t.Errorf("Expected status 201, got %d", w.Code)
    }
    
    var result ratelimit.RateLimitConfig
    if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
        t.Fatalf("Failed to parse response: %v", err)
    }
    
    if result.ProviderID != req.ProviderID {
        t.Errorf("Expected provider ID %s, got %s", req.ProviderID, result.ProviderID)
    }
}

// Implement more tests...
```

---

## Deliverables

After completing this phase, you should have:

1. ✅ Rate limit CRUD handlers
2. ✅ Usage query handlers
3. ✅ Quota info handlers
4. ✅ API routes registered in REST server
5. ✅ Basic authentication middleware
6. ✅ API tests

---

## Success Criteria

- [ ] All CRUD operations work for rate limits
- [ ] Usage can be queried and exported
- [ ] Quota information is accessible
- [ ] API is properly authenticated
- [ ] All API tests pass
- [ ] API documentation is complete
- [ ] Error responses are consistent

---

## Next Phase

After completing Phase 6, proceed to **Phase 7: Dashboard UI** which implements the admin dashboard interface.

**File:** `../todo/phase-7-dashboard-ui.md`
