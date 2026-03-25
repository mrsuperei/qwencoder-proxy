# Model Usage Tracking Design

## Overview

This document analyzes the current database schema for rate limiting and request tracking, and provides recommendations for adding model-level usage tracking to the qwencoder-proxy.

## Current State Analysis

### Current Tables

#### 1. `token_usage` Table
**Purpose:** Tracks aggregated usage metrics per token for rate limiting.

```sql
CREATE TABLE IF NOT EXISTS token_usage (
    id TEXT PRIMARY KEY,
    token_id TEXT NOT NULL,
    requests_today INTEGER NOT NULL DEFAULT 0,
    requests_in_minute INTEGER NOT NULL DEFAULT 0,
    tokens_in_minute INTEGER NOT NULL DEFAULT 0,
    window_start INTEGER NOT NULL,
    day_start INTEGER NOT NULL,
    created_at INTEGER NOT NULL DEFAULT (strftime('%s', 'subsec') * 1000),
    updated_at INTEGER NOT NULL DEFAULT (strftime('%s', 'subsec') * 1000),
    UNIQUE(token_id),
    FOREIGN KEY (token_id) REFERENCES tokens(id) ON DELETE CASCADE
)
```

**Characteristics:**
- One row per token (UNIQUE constraint on `token_id`)
- Stores aggregated counts (today, in-minute)
- Uses sliding window approach
- Updated via UPSERT (INSERT ... ON CONFLICT)

#### 2. `provider_usage` Table
**Purpose:** Tracks aggregated usage metrics per provider for rate limiting.

```sql
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
```

**Characteristics:**
- One row per provider (UNIQUE constraint on `provider_id`)
- Stores aggregated counts (today, in-minute)
- Uses sliding window approach
- Updated via UPSERT (INSERT ... ON CONFLICT)

#### 3. `request_history` Table
**Purpose:** Stores individual request records for accurate sliding window calculations.

```sql
CREATE TABLE IF NOT EXISTS request_history (
    id TEXT PRIMARY KEY,
    token_id TEXT NOT NULL,
    request_count INTEGER NOT NULL DEFAULT 1,
    token_count INTEGER NOT NULL DEFAULT 0,
    timestamp INTEGER NOT NULL,
    created_at INTEGER NOT NULL DEFAULT (strftime('%s', 'subsec') * 1000),
    FOREIGN KEY (token_id) REFERENCES tokens(id) ON DELETE CASCADE
)
```

**Characteristics:**
- One row per request
- Stores individual request details
- Used for accurate sliding window calculations
- No model field (missing for model-level tracking)

### Current Usage Tracking Implementation

**Interface:** [`UsageTracker`](../internal/ratelimit/interfaces.go:29-54)

```go
type UsageTracker interface {
    RecordUsage(ctx context.Context, providerID string, tokenID string, requestCount int, tokenCount int) error
    GetProviderUsage(ctx context.Context, providerID string) (*UsageMetrics, error)
    GetTokenUsage(ctx context.Context, providerID string, tokenID string) (*UsageMetrics, error)
    GetAllProviderUsage(ctx context.Context) (map[string]*UsageMetrics, error)
    ResetUsage(ctx context.Context, providerID string, tokenID string) error
    CleanupOrphanedUsageRecords(ctx context.Context) error
    GetDB() *sql.DB
    Close() error
}
```

**Key Observations:**
1. No model parameter in `RecordUsage()` - cannot track model usage
2. No model-specific queries - cannot get usage by model
3. Aggregated metrics only - no per-model breakdown
4. Sliding window implemented via `request_history` table queries

### Current Rate Limiting

**Configuration:** [`ProviderRateLimitConfig`](../internal/ratelimit/config.go:5-23)

```go
type ProviderRateLimitConfig struct {
    ProviderID        string
    RequestsPerDay    int
    RequestsPerMinute int
    TokensPerMinute   int
    Enabled           bool
    UpdatedAt         time.Time
}
```

**Limitations:**
- Rate limits are per-provider only
- No per-model rate limiting capability
- Cannot differentiate between expensive and cheap models

## Problem Statement

### Missing Capabilities

1. **No Model-Level Tracking**
   - Cannot determine which models are being used most
   - Cannot track usage per model (e.g., gemini-2.5-flash vs gemini-2.5-pro)
   - No visibility into model distribution across tokens

2. **No Model-Level Rate Limiting**
   - Cannot apply different rate limits for different models
   - Cannot protect against expensive model overuse
   - Cannot prioritize cheaper models over expensive ones

3. **No Usage Analytics**
   - Cannot generate model usage reports
   - Cannot track costs per model
   - Cannot optimize token selection based on model usage patterns

### Use Cases for Model Usage Tracking

1. **Cost Management**
   - Track spending per model
   - Set different quotas for expensive vs cheap models
   - Alert on high-cost model usage

2. **Token Selection Optimization**
   - Prefer tokens with available quota for specific models
   - Balance load across models
   - Avoid rate limits on popular models

3. **Usage Analytics**
   - Dashboard showing model usage distribution
   - Trends over time
   - Per-token model usage breakdown

4. **Model-Specific Rate Limiting**
   - Different limits for different models
   - Protect against API provider model-specific limits
   - Implement fair usage policies

## Design Recommendations

### Option 1: Minimal Change - Add Model to Existing Tables

**Approach:** Add `model` field to `request_history` and `token_usage` tables.

#### Schema Changes

**Modified `request_history`:**
```sql
ALTER TABLE request_history ADD COLUMN model TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_request_history_model ON request_history(token_id, model, timestamp);
```

**Modified `token_usage`:**
```sql
-- Option 1a: Single model per token (last used model)
ALTER TABLE token_usage ADD COLUMN model TEXT;

-- Option 1b: Multiple models per token (JSON array)
ALTER TABLE token_usage ADD COLUMN models TEXT; -- JSON array of model names
```

**Pros:**
- Minimal schema changes
- Backward compatible (default values)
- Easy to implement

**Cons:**
- Option 1a: Only tracks last used model per token
- Option 1b: JSON array is not queryable in SQLite
- No model-specific aggregates
- No model-level rate limiting

**Verdict:** ❌ Not recommended - insufficient for model-level tracking needs

---

### Option 2: Separate Model Usage Table (Recommended)

**Approach:** Create a new `model_usage` table for per-model tracking while keeping existing tables for rate limiting.

#### Schema Design

**New `model_usage` table:**
```sql
CREATE TABLE IF NOT EXISTS model_usage (
    id TEXT PRIMARY KEY,
    token_id TEXT NOT NULL,
    provider_id TEXT NOT NULL,
    model TEXT NOT NULL,
    request_count INTEGER NOT NULL DEFAULT 0,
    input_tokens INTEGER NOT NULL DEFAULT 0,
    output_tokens INTEGER NOT NULL DEFAULT 0,
    total_tokens INTEGER NOT NULL DEFAULT 0,
    window_start INTEGER NOT NULL,
    day_start INTEGER NOT NULL,
    created_at INTEGER NOT NULL DEFAULT (strftime('%s', 'subsec') * 1000),
    updated_at INTEGER NOT NULL DEFAULT (strftime('%s', 'subsec') * 1000),
    UNIQUE(token_id, model),
    FOREIGN KEY (token_id) REFERENCES tokens(id) ON DELETE CASCADE
);

-- Indexes for efficient queries
CREATE INDEX IF NOT EXISTS idx_model_usage_token ON model_usage(token_id);
CREATE INDEX IF NOT EXISTS idx_model_usage_provider ON model_usage(provider_id);
CREATE INDEX IF NOT EXISTS idx_model_usage_model ON model_usage(model);
CREATE INDEX IF NOT EXISTS idx_model_usage_provider_model ON model_usage(provider_id, model);
```

**Modified `request_history` table:**
```sql
ALTER TABLE request_history ADD COLUMN model TEXT NOT NULL DEFAULT '';
ALTER TABLE request_history ADD COLUMN input_tokens INTEGER NOT NULL DEFAULT 0;
ALTER TABLE request_history ADD COLUMN output_tokens INTEGER NOT NULL DEFAULT 0;

CREATE INDEX IF NOT EXISTS idx_request_history_model ON request_history(token_id, model, timestamp);
```

#### Interface Changes

**Updated `UsageTracker` interface:**
```go
type UsageTracker interface {
    // Existing methods
    RecordUsage(ctx context.Context, providerID string, tokenID string, requestCount int, tokenCount int) error
    GetProviderUsage(ctx context.Context, providerID string) (*UsageMetrics, error)
    GetTokenUsage(ctx context.Context, providerID string, tokenID string) (*UsageMetrics, error)
    GetAllProviderUsage(ctx context.Context) (map[string]*UsageMetrics, error)
    ResetUsage(ctx context.Context, providerID string, tokenID string) error
    CleanupOrphanedUsageRecords(ctx context.Context) error
    GetDB() *sql.DB
    Close() error

    // New methods for model usage tracking
    RecordModelUsage(ctx context.Context, providerID string, tokenID string, model string, 
                     inputTokens int, outputTokens int) error
    GetModelUsage(ctx context.Context, providerID string, tokenID string, model string) (*ModelUsageMetrics, error)
    GetAllModelUsage(ctx context.Context, providerID string) (map[string]*ModelUsageMetrics, error)
    GetTokenModelUsage(ctx context.Context, tokenID string) (map[string]*ModelUsageMetrics, error)
    GetProviderModelUsage(ctx context.Context, providerID string) (map[string]*ModelUsageMetrics, error)
    ResetModelUsage(ctx context.Context, providerID string, tokenID string, model string) error
}
```

**New `ModelUsageMetrics` type:**
```go
type ModelUsageMetrics struct {
    ProviderID    string    `json:"provider_id"`
    TokenID       string    `json:"token_id,omitempty"`
    Model         string     `json:"model"`
    
    // Request counts
    RequestCount  int        `json:"request_count"`
    
    // Token counts (detailed)
    InputTokens   int        `json:"input_tokens"`
    OutputTokens  int        `json:"output_tokens"`
    TotalTokens   int        `json:"total_tokens"`
    
    // Time windows
    WindowStart   time.Time  `json:"window_start"`
    DayStart      time.Time  `json:"day_start"`
}
```

#### Implementation Strategy

**Phase 1: Database Migration**
```go
// Migration to add model tracking
func migrateModelTracking(tx *sql.Tx) error {
    // Add model column to request_history
    if _, err := tx.Exec(`
        ALTER TABLE request_history 
        ADD COLUMN model TEXT NOT NULL DEFAULT ''
    `); err != nil {
        return fmt.Errorf("failed to add model column: %w", err)
    }
    
    // Add input/output token columns to request_history
    if _, err := tx.Exec(`
        ALTER TABLE request_history 
        ADD COLUMN input_tokens INTEGER NOT NULL DEFAULT 0
    `); err != nil {
        return fmt.Errorf("failed to add input_tokens column: %w", err)
    }
    
    if _, err := tx.Exec(`
        ALTER TABLE request_history 
        ADD COLUMN output_tokens INTEGER NOT NULL DEFAULT 0
    `); err != nil {
        return fmt.Errorf("failed to add output_tokens column: %w", err)
    }
    
    // Create model_usage table
    modelUsageTable := `
        CREATE TABLE IF NOT EXISTS model_usage (
            id TEXT PRIMARY KEY,
            token_id TEXT NOT NULL,
            provider_id TEXT NOT NULL,
            model TEXT NOT NULL,
            request_count INTEGER NOT NULL DEFAULT 0,
            input_tokens INTEGER NOT NULL DEFAULT 0,
            output_tokens INTEGER NOT NULL DEFAULT 0,
            total_tokens INTEGER NOT NULL DEFAULT 0,
            window_start INTEGER NOT NULL,
            day_start INTEGER NOT NULL,
            created_at INTEGER NOT NULL DEFAULT (strftime('%s', 'subsec') * 1000),
            updated_at INTEGER NOT NULL DEFAULT (strftime('%s', 'subsec') * 1000),
            UNIQUE(token_id, model),
            FOREIGN KEY (token_id) REFERENCES tokens(id) ON DELETE CASCADE
        )
    `
    if _, err := tx.Exec(modelUsageTable); err != nil {
        return fmt.Errorf("failed to create model_usage table: %w", err)
    }
    
    // Create indexes
    indexes := []string{
        "CREATE INDEX IF NOT EXISTS idx_model_usage_token ON model_usage(token_id)",
        "CREATE INDEX IF NOT EXISTS idx_model_usage_provider ON model_usage(provider_id)",
        "CREATE INDEX IF NOT EXISTS idx_model_usage_model ON model_usage(model)",
        "CREATE INDEX IF NOT EXISTS idx_model_usage_provider_model ON model_usage(provider_id, model)",
        "CREATE INDEX IF NOT EXISTS idx_request_history_model ON request_history(token_id, model, timestamp)",
    }
    
    for _, idx := range indexes {
        if _, err := tx.Exec(idx); err != nil {
            return fmt.Errorf("failed to create index: %w", err)
        }
    }
    
    return nil
}
```

**Phase 2: Update UsageTracker Implementation**

```go
// RecordModelUsage records usage for a specific model
func (ut *usageTrackerImpl) RecordModelUsage(
    ctx context.Context, 
    providerID string, 
    tokenID string, 
    model string,
    inputTokens int, 
    outputTokens int,
) error {
    now := time.Now()
    nowMs := now.UnixMilli()
    totalTokens := inputTokens + outputTokens
    
    tx, err := ut.db.BeginTx(ctx, nil)
    if err != nil {
        return fmt.Errorf("failed to begin transaction: %w", err)
    }
    defer tx.Rollback()
    
    // Record in request_history with model details
    historyID := uuid.New().String()
    historyQuery := `
        INSERT INTO request_history (id, token_id, model, request_count, 
                                     input_tokens, output_tokens, timestamp)
        VALUES (?, ?, ?, 1, ?, ?, ?)
    `
    if _, err := tx.ExecContext(ctx, historyQuery, 
        historyID, tokenID, model, inputTokens, outputTokens, nowMs); err != nil {
        return fmt.Errorf("failed to insert request history: %w", err)
    }
    
    // Update model_usage with atomic increment
    dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
    windowStart := now.Add(-time.Minute)
    
    modelQuery := `
        INSERT INTO model_usage (id, token_id, provider_id, model, 
                               request_count, input_tokens, output_tokens, total_tokens,
                               window_start, day_start)
        VALUES (?, ?, ?, ?, 0, 0, 0, 0, ?, ?)
        ON CONFLICT(token_id, model) DO UPDATE SET
            request_count = request_count + 1,
            input_tokens = input_tokens + ?,
            output_tokens = output_tokens + ?,
            total_tokens = total_tokens + ?,
            updated_at = ?
        WHERE token_id = ? AND model = ?
    `
    if _, err := tx.ExecContext(ctx, modelQuery,
        uuid.New().String(), tokenID, providerID, model,
        windowStart.UnixMilli(), dayStart.UnixMilli(),
        inputTokens, outputTokens, totalTokens, nowMs,
        tokenID, model); err != nil {
        return fmt.Errorf("failed to update model usage: %w", err)
    }
    
    // Also update existing token_usage and provider_usage for backward compatibility
    // (existing RecordUsage logic)
    
    if err := tx.Commit(); err != nil {
        return fmt.Errorf("failed to commit transaction: %w", err)
    }
    
    return nil
}

// GetModelUsage retrieves usage metrics for a specific model
func (ut *usageTrackerImpl) GetModelUsage(
    ctx context.Context, 
    providerID string, 
    tokenID string, 
    model string,
) (*ModelUsageMetrics, error) {
    now := time.Now()
    windowStart := now.Add(-time.Minute)
    dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
    windowStartMs := windowStart.UnixMilli()
    dayStartMs := dayStart.UnixMilli()
    
    // Query from model_usage table
    query := `
        SELECT provider_id, token_id, model, request_count,
               input_tokens, output_tokens, total_tokens,
               window_start, day_start
        FROM model_usage
        WHERE token_id = ? AND model = ?
    `
    
    var metrics ModelUsageMetrics
    err := ut.db.QueryRowContext(ctx, query, tokenID, model).Scan(
        &metrics.ProviderID, &metrics.TokenID, &metrics.Model,
        &metrics.RequestCount, &metrics.InputTokens, 
        &metrics.OutputTokens, &metrics.TotalTokens,
        &metrics.WindowStart, &metrics.DayStart,
    )
    
    if err != nil {
        if errors.Is(err, sql.ErrNoRows) {
            // No usage yet, return zero metrics
            return &ModelUsageMetrics{
                ProviderID:  providerID,
                TokenID:     tokenID,
                Model:        model,
                RequestCount: 0,
                InputTokens:  0,
                OutputTokens: 0,
                TotalTokens:   0,
                WindowStart:  windowStart,
                DayStart:     dayStart,
            }, nil
        }
        return nil, fmt.Errorf("failed to query model usage: %w", err)
    }
    
    return &metrics, nil
}

// GetTokenModelUsage retrieves all model usage for a token
func (ut *usageTrackerImpl) GetTokenModelUsage(
    ctx context.Context, 
    tokenID string,
) (map[string]*ModelUsageMetrics, error) {
    query := `
        SELECT provider_id, token_id, model, request_count,
               input_tokens, output_tokens, total_tokens,
               window_start, day_start
        FROM model_usage
        WHERE token_id = ?
    `
    
    rows, err := ut.db.QueryContext(ctx, query, tokenID)
    if err != nil {
        return nil, fmt.Errorf("failed to query token model usage: %w", err)
    }
    defer rows.Close()
    
    result := make(map[string]*ModelUsageMetrics)
    for rows.Next() {
        var metrics ModelUsageMetrics
        if err := rows.Scan(
            &metrics.ProviderID, &metrics.TokenID, &metrics.Model,
            &metrics.RequestCount, &metrics.InputTokens, 
            &metrics.OutputTokens, &metrics.TotalTokens,
            &metrics.WindowStart, &metrics.DayStart,
        ); err != nil {
            return nil, fmt.Errorf("failed to scan model usage row: %w", err)
        }
        result[metrics.Model] = &metrics
    }
    
    return result, nil
}
```

**Phase 3: Update Recording Points**

```go
// In sequential_handler.go or wherever usage is recorded
func (h *SequentialHandler) handleNonStreamingRequest(...) {
    // ... existing code ...
    
    // Extract model from request
    model := openaiReq["model"].(string)
    
    // Extract token counts from response
    inputTokens, outputTokens, totalTokens, err := h.tokenCounter.ExtractTokensFromResponse(response)
    if err != nil {
        h.logger.WarnLog("Failed to extract tokens: %v", err)
        // Fallback to existing behavior
        inputTokens, outputTokens, totalTokens = 0, 0, 0
    }
    
    // Record model usage
    if err := h.usageTracker.RecordModelUsage(
        ctx, providerID, selectedToken.ID, model,
        inputTokens, outputTokens,
    ); err != nil {
        h.logger.ErrorLog("Failed to record model usage: %v", err)
    }
    
    // Also record for backward compatibility
    if err := h.usageTracker.RecordUsage(
        ctx, providerID, selectedToken.ID, 1, totalTokens,
    ); err != nil {
        h.logger.ErrorLog("Failed to record usage: %v", err)
    }
    
    // ... rest of code ...
}
```

**Pros:**
- ✅ Full model-level tracking with detailed metrics
- ✅ Separates concerns (rate limiting vs analytics)
- ✅ Backward compatible (existing tables unchanged)
- ✅ Supports model-specific rate limiting in future
- ✅ Queryable model usage data
- ✅ Can track input/output tokens separately

**Cons:**
- More complex schema
- Requires migration
- More storage (additional table)

**Verdict:** ✅ **Recommended** - Best balance of features and complexity

---

### Option 3: Fully Normalized Schema

**Approach:** Create separate tables for entities and relationships.

#### Schema Design

```sql
-- Requests table (individual requests)
CREATE TABLE IF NOT EXISTS requests (
    id TEXT PRIMARY KEY,
    token_id TEXT NOT NULL,
    provider_id TEXT NOT NULL,
    model TEXT NOT NULL,
    timestamp INTEGER NOT NULL,
    status_code INTEGER NOT NULL,
    input_tokens INTEGER NOT NULL DEFAULT 0,
    output_tokens INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL DEFAULT (strftime('%s', 'subsec') * 1000),
    FOREIGN KEY (token_id) REFERENCES tokens(id) ON DELETE CASCADE
);

-- Aggregated token usage (for rate limiting)
CREATE TABLE IF NOT EXISTS token_usage (
    id TEXT PRIMARY KEY,
    token_id TEXT NOT NULL,
    requests_today INTEGER NOT NULL DEFAULT 0,
    requests_in_minute INTEGER NOT NULL DEFAULT 0,
    tokens_in_minute INTEGER NOT NULL DEFAULT 0,
    window_start INTEGER NOT NULL,
    day_start INTEGER NOT NULL,
    created_at INTEGER NOT NULL DEFAULT (strftime('%s', 'subsec') * 1000),
    updated_at INTEGER NOT NULL DEFAULT (strftime('%s', 'subsec') * 1000),
    UNIQUE(token_id),
    FOREIGN KEY (token_id) REFERENCES tokens(id) ON DELETE CASCADE
);

-- Aggregated model usage (for analytics)
CREATE TABLE IF NOT EXISTS model_usage (
    id TEXT PRIMARY KEY,
    token_id TEXT NOT NULL,
    provider_id TEXT NOT NULL,
    model TEXT NOT NULL,
    request_count INTEGER NOT NULL DEFAULT 0,
    input_tokens INTEGER NOT NULL DEFAULT 0,
    output_tokens INTEGER NOT NULL DEFAULT 0,
    total_tokens INTEGER NOT NULL DEFAULT 0,
    window_start INTEGER NOT NULL,
    day_start INTEGER NOT NULL,
    created_at INTEGER NOT NULL DEFAULT (strftime('%s', 'subsec') * 1000),
    updated_at INTEGER NOT NULL DEFAULT (strftime('%s', 'subsec') * 1000),
    UNIQUE(token_id, model),
    FOREIGN KEY (token_id) REFERENCES tokens(id) ON DELETE CASCADE
);
```

**Pros:**
- Fully normalized
- Clear separation of concerns
- Flexible for future enhancements

**Cons:**
- Most complex
- Requires significant refactoring
- More storage (renamed request_history to requests)

**Verdict:** ⚠️ Consider for future - good design but may be overkill now

---

## Recommended Implementation Plan

### Phase 1: Database Schema Migration (Schema v2)

**File:** [`internal/ratelimit/migration.go`](../internal/ratelimit/migration.go)

1. Add migration to schema version 2
2. Add `model`, `input_tokens`, `output_tokens` columns to `request_history`
3. Create `model_usage` table
4. Create indexes for efficient queries

### Phase 2: Update UsageTracker Interface and Implementation

**Files:**
- [`internal/ratelimit/interfaces.go`](../internal/ratelimit/interfaces.go) - Add new interface methods
- [`internal/ratelimit/usage_tracker.go`](../internal/ratelimit/usage_tracker.go) - Implement new methods

1. Add `ModelUsageMetrics` type
2. Add model tracking methods to `UsageTracker` interface
3. Implement `RecordModelUsage()`
4. Implement `GetModelUsage()`, `GetTokenModelUsage()`, etc.
5. Update `RecordUsage()` to also call model tracking

### Phase 3: Update Usage Recording Points

**Files:**
- [`internal/proxy/sequential_handler.go`](../internal/proxy/sequential_handler.go)
- [`internal/ratelimit/middleware.go`](../internal/ratelimit/middleware.go)

1. Extract model from request (already done)
2. Extract input/output tokens from response (already done via `TokenCounter`)
3. Call `RecordModelUsage()` with model and token details
4. Ensure backward compatibility with existing `RecordUsage()`

### Phase 4: Update REST API

**File:** [`internal/restapi/rate_limit_api.go`](../internal/restapi/rate_limit_api.go)

Add new endpoints:
- `GET /api/ratelimit/model-usage` - All model usage
- `GET /api/ratelimit/model-usage/:provider` - Provider model usage
- `GET /api/ratelimit/model-usage/:provider/:token` - Token model usage
- `GET /api/ratelimit/model-usage/:provider/:token/:model` - Specific model usage

### Phase 5: Update Dashboard

**Files:**
- `web/dashboard/js/models/token.js`
- `web/dashboard/js/services/token-service.js`

Add model usage visualization:
- Per-token model usage breakdown
- Model usage distribution charts
- Input vs output token comparison

## Future Enhancements

### Model-Specific Rate Limiting

```go
// Extend ProviderRateLimitConfig
type ProviderRateLimitConfig struct {
    // Existing fields...
    ProviderID        string
    RequestsPerDay    int
    RequestsPerMinute int
    TokensPerMinute   int
    Enabled           bool
    UpdatedAt         time.Time
    
    // New: Model-specific limits
    ModelLimits map[string]ModelRateLimitConfig `json:"model_limits,omitempty"`
}

type ModelRateLimitConfig struct {
    Model             string  `json:"model"`
    RequestsPerDay    int     `json:"requests_per_day"`
    RequestsPerMinute int     `json:"requests_per_minute"`
    TokensPerMinute   int     `json:"tokens_per_minute"`
    CostPer1KTokens   float64 `json:"cost_per_1k_tokens,omitempty"`
}
```

### Usage Analytics

```go
// New analytics interface
type UsageAnalytics interface {
    GetModelUsageTrends(ctx context.Context, providerID string, days int) ([]ModelUsageTrend, error)
    GetTopModels(ctx context.Context, providerID string, limit int) ([]ModelUsageSummary, error)
    GetTokenModelDistribution(ctx context.Context, tokenID string) (map[string]float64, error)
}

type ModelUsageTrend struct {
    Model      string
    Date       time.Time
    Requests   int
    Tokens      int
    Cost       float64
}

type ModelUsageSummary struct {
    Model         string
    TotalRequests int
    TotalTokens   int
    TotalCost     float64
    AvgTokensPerRequest float64
}
```

### Cost Tracking

```go
// Add cost tracking to model_usage
type ModelUsageMetrics struct {
    // Existing fields...
    ProviderID    string
    TokenID       string
    Model         string
    RequestCount  int
    InputTokens   int
    OutputTokens  int
    TotalTokens   int
    WindowStart   time.Time
    DayStart      time.Time
    
    // New: Cost tracking
    EstimatedCost float64 `json:"estimated_cost"`
}
```

## Conclusion

### Recommendation Summary

**Use Option 2: Separate Model Usage Table**

This approach provides:
1. ✅ Full model-level tracking with detailed metrics
2. ✅ Separation of concerns (rate limiting vs analytics)
3. ✅ Backward compatibility with existing rate limiting
4. ✅ Foundation for model-specific rate limiting
5. ✅ Queryable model usage data for analytics
6. ✅ Reasonable complexity for implementation

### Key Benefits

1. **Analytics & Insights**
   - Track which models are most used
   - Understand usage patterns per token
   - Make data-driven decisions about token allocation

2. **Cost Management**
   - Track spending per model
   - Set different quotas for expensive models
   - Optimize for cost efficiency

3. **Future-Proof**
   - Foundation for model-specific rate limiting
   - Extensible for cost tracking
   - Supports advanced analytics

### Implementation Priority

1. **High Priority:** Database migration and basic model tracking
2. **Medium Priority:** REST API endpoints for model usage
3. **Low Priority:** Dashboard visualization and analytics

### Migration Path

1. Deploy schema v2 migration (non-breaking)
2. Update usage recording to track models
3. Add new API endpoints
4. Update dashboard with model usage views
5. (Optional) Implement model-specific rate limiting
