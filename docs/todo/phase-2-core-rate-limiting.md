# Phase 2: Core Rate Limiting - In-Memory Checker with Caching

**Phase Goal:** Implement in-memory rate limiting with caching for high-performance limit checks.

**Duration:** Week 2-3  
**Status:** Ready to Implement  
**Dependencies:** Phase 1 (Foundation)

---

## Task Overview

This phase implements the core rate limiting logic with:

1. Implementing the `RateLimiter` interface
2. Creating in-memory `RateLimitCache` for O(1) lookups
3. Implementing `RateLimitChecker` for limit evaluation
4. Adding periodic cache refresh from database
5. Implementing provider schema interfaces
6. Implementing default schemas for Qwen and Gemini
7. Writing comprehensive unit and integration tests

---

## Task 2.1: Implement `RateLimiter` Interface

**File:** `qwencoder-proxy/ratelimit/limiter.go`

Create the core rate limiter interface and its implementation.

**Implementation Requirements:**

```go
package ratelimit

import (
    "context"
    "time"
    
    "github.com/sunbankio/qwencoder-proxy/ratelimit/store"
)

// RateLimiter defines the interface for rate limiting operations
type RateLimiter interface {
    // CheckLimit checks if a request is allowed
    CheckLimit(ctx context.Context, req *LimitCheckRequest) (*LimitCheckResult, error)
    
    // RecordUsage records usage metrics (non-blocking)
    RecordUsage(ctx context.Context, usage *UsageRecord) error
    
    // GetUsage retrieves current usage statistics
    GetUsage(ctx context.Context, query *store.UsageQuery) (*UsageStats, error)
    
    // GetUsageHistory retrieves historical usage data
    GetUsageHistory(ctx context.Context, query *store.HistoryQuery) ([]*UsageRecord, error)
    
    // GetTokenQuota retrieves quota information for a token
    GetTokenQuota(ctx context.Context, providerID, tokenID string) (*QuotaInfo, error)
    
    // GetApplicableLimits retrieves all applicable rate limits for a request
    GetApplicableLimits(ctx context.Context, providerID, tokenID, model string) ([]*RateLimitConfig, error)
    
    // Start starts the rate limiter (cache refresh, worker pool)
    Start() error
    
    // Stop stops the rate limiter gracefully
    Stop()
}

// rateLimiter implements the RateLimiter interface
type rateLimiter struct {
    store       store.Store
    cache       *RateLimitCache
    tracker     *UsageTracker
    config      *Config
    logger      Logger
    stopChan    chan struct{}
    started     bool
    mu          sync.RWMutex
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(
    store store.Store,
    schemaFactory ProviderSchemaFactory,
    config *Config,
    logger Logger,
) RateLimiter {
    return &rateLimiter{
        store:   store,
        config:  config,
        logger:  logger,
        stopChan: make(chan struct{}),
    }
}

// Start starts the rate limiter
func (r *rateLimiter) Start() error {
    r.mu.Lock()
    defer r.mu.Unlock()
    
    if r.started {
        return nil
    }
    
    // Initialize cache
    r.cache = NewRateLimitCache(r.store, r.config.CacheRefreshInterval, r.logger)
    if err := r.cache.Start(); err != nil {
        return fmt.Errorf("failed to start cache: %w", err)
    }
    
    // Initialize tracker
    r.tracker = NewUsageTracker(r.store, r.config.BufferConfig, r.logger)
    if err := r.tracker.Start(); err != nil {
        r.cache.Stop()
        return fmt.Errorf("failed to start tracker: %w", err)
    }
    
    r.started = true
    r.logger.InfoLog("[RateLimiter] Started successfully")
    return nil
}

// Stop stops the rate limiter gracefully
func (r *rateLimiter) Stop() {
    r.mu.Lock()
    defer r.mu.Unlock()
    
    if !r.started {
        return
    }
    
    close(r.stopChan)
    
    if r.cache != nil {
        r.cache.Stop()
    }
    
    if r.tracker != nil {
        r.tracker.Stop()
    }
    
    r.started = false
    r.logger.InfoLog("[RateLimiter] Stopped successfully")
}

// CheckLimit checks if a request is allowed
func (r *rateLimiter) CheckLimit(ctx context.Context, req *LimitCheckRequest) (*LimitCheckResult, error) {
    // Get applicable rate limits from cache
    limits, err := r.cache.GetApplicableLimits(req.ProviderID, req.TokenID, req.Model)
    if err != nil {
        r.logger.ErrorLog("[RateLimiter] Failed to get applicable limits: %v", err)
        // Fail open - allow request if we can't check limits
        return &LimitCheckResult{
            Allowed: true,
            Reason:  "error",
        }, nil
    }
    
    // Get current usage from cache
    usage, err := r.cache.GetCurrentUsage(req.ProviderID, req.TokenID, req.Model)
    if err != nil {
        r.logger.ErrorLog("[RateLimiter] Failed to get current usage: %v", err)
        // Fail open
        return &LimitCheckResult{
            Allowed: true,
            Reason:  "error",
        }, nil
    }
    
    // Check each limit type in priority order
    for _, limit := range limits {
        result := r.checkSingleLimit(limit, usage, req)
        if !result.Allowed {
            return result, nil
        }
    }
    
    // All limits passed
    return &LimitCheckResult{
        Allowed:       true,
        Reason:        "ok",
        CurrentUsage:  usage,
        AvailableQuota: r.calculateQuotaInfo(limits, usage),
    }, nil
}

// checkSingleLimit checks a single rate limit
func (r *rateLimiter) checkSingleLimit(limit *RateLimitConfig, usage map[TimeWindow]*UsageStats, req *LimitCheckRequest) *LimitCheckResult {
    var window TimeWindow
    var timeWindowDuration time.Duration
    
    switch limit.LimitType {
    case "daily":
        window = TimeWindowDay
        timeWindowDuration = 24 * time.Hour
    case "burst":
        window = TimeWindowMinute
        timeWindowDuration = 1 * time.Minute
    case "token":
        window = TimeWindowDay
        timeWindowDuration = 24 * time.Hour
    default:
        return &LimitCheckResult{
            Allowed: true,
            Reason:  "ok",
        }
    }
    
    windowUsage, exists := usage[window]
    if !exists {
        return &LimitCheckResult{
            Allowed: true,
            Reason:  "ok",
        }
    }
    
    var used int64
    if limit.LimitType == "token" {
        used = windowUsage.TokenCount
    } else {
        used = windowUsage.RequestCount
    }
    
    if used >= limit.LimitValue {
        // Calculate retry after based on window type
        retryAfter := r.calculateRetryAfter(timeWindowDuration, windowUsage.StartTime)
        
        return &LimitCheckResult{
            Allowed:    false,
            Reason:     limit.LimitType + "_limit",
            RetryAfter: retryAfter,
            CurrentUsage: windowUsage,
        }
    }
    
    return &LimitCheckResult{Allowed: true}
}

// calculateRetryAfter calculates the retry-after duration
func (r *rateLimiter) calculateRetryAfter(windowDuration time.Duration, windowStart time.Time) time.Duration {
    now := time.Now()
    windowEnd := windowStart.Add(windowDuration)
    
    if windowEnd.Before(now) {
        return 0
    }
    
    return windowEnd.Sub(now)
}

// calculateQuotaInfo calculates quota information from limits and usage
func (r *rateLimiter) calculateQuotaInfo(limits []*RateLimitConfig, usage map[TimeWindow]*UsageStats) *QuotaInfo {
    quota := &QuotaInfo{
        DailyLimit:     0,
        DailyUsed:      0,
        DailyRemaining:  0,
        BurstLimit:     0,
        BurstUsed:      0,
        BurstRemaining: 0,
        TokenLimit:     0,
        TokenUsed:      0,
        TokenRemaining: 0,
    }
    
    for _, limit := range limits {
        windowUsage, exists := usage[TimeWindowDay]
        if !exists {
            continue
        }
        
        switch limit.LimitType {
        case "daily":
            quota.DailyLimit = limit.LimitValue
            quota.DailyUsed = windowUsage.RequestCount
            quota.DailyRemaining = max(0, quota.DailyLimit-quota.DailyUsed)
        case "burst":
            minuteUsage, exists := usage[TimeWindowMinute]
            if exists {
                quota.BurstLimit = limit.LimitValue
                quota.BurstUsed = minuteUsage.RequestCount
                quota.BurstRemaining = max(0, quota.BurstLimit-quota.BurstUsed)
            }
        case "token":
            quota.TokenLimit = limit.LimitValue
            quota.TokenUsed = windowUsage.TokenCount
            quota.TokenRemaining = max(0, quota.TokenLimit-quota.TokenUsed)
        }
    }
    
    return quota
}

// RecordUsage records usage metrics (non-blocking)
func (r *rateLimiter) RecordUsage(ctx context.Context, usage *UsageRecord) error {
    return r.tracker.RecordUsage(ctx, usage)
}

// GetUsage retrieves current usage statistics
func (r *rateLimiter) GetUsage(ctx context.Context, query *store.UsageQuery) (*UsageStats, error) {
    return r.store.GetUsage(ctx, query)
}

// GetUsageHistory retrieves historical usage data
func (r *rateLimiter) GetUsageHistory(ctx context.Context, query *store.HistoryQuery) ([]*UsageRecord, error) {
    return r.store.GetUsageHistory(ctx, query)
}

// GetTokenQuota retrieves quota information for a token
func (r *rateLimiter) GetTokenQuota(ctx context.Context, providerID, tokenID string) (*QuotaInfo, error) {
    // Get all models used by this token
    // For simplicity, we'll get all limits for this token
    limits, err := r.store.GetRateLimits(ctx, &store.RateLimitFilter{
        ProviderID: providerID,
        TokenID:    tokenID,
        Enabled:    boolPtr(true),
    })
    if err != nil {
        return nil, err
    }
    
    // Get current usage for all models
    usage, err := r.cache.GetCurrentUsage(providerID, tokenID, "")
    if err != nil {
        return nil, err
    }
    
    // Aggregate usage across all models
    return r.calculateQuotaInfo(limits, usage), nil
}

// GetApplicableLimits retrieves all applicable rate limits for a request
func (r *rateLimiter) GetApplicableLimits(ctx context.Context, providerID, tokenID, model string) ([]*RateLimitConfig, error) {
    return r.cache.GetApplicableLimits(providerID, tokenID, model)
}

func boolPtr(b bool) *bool {
    return &b
}

func max(a, b int64) int64 {
    if a > b {
        return a
    }
    return b
}
```

---

## Task 2.2: Implement In-Memory `RateLimitCache`

**File:** `qwencoder-proxy/ratelimit/cache.go`

Create the in-memory cache for rate limits and usage.

**Implementation Requirements:**

```go
package ratelimit

import (
    "context"
    "sync"
    "time"
    
    "github.com/sunbankio/qwencoder-proxy/ratelimit/store"
)

// RateLimitCache holds in-memory rate limit and usage data
type RateLimitCache struct {
    mu       sync.RWMutex
    store    store.Store
    limits   map[string]*CachedLimit
    usage    map[string]*CachedUsage
    ttl      time.Duration
    lastSync time.Time
    logger   Logger
    stopChan chan struct{}
    ticker   *time.Ticker
}

// CachedLimit represents a cached rate limit
type CachedLimit struct {
    *RateLimitConfig
    ExpiresAt time.Time
}

// CachedUsage represents cached usage statistics
type CachedUsage struct {
    *UsageStats
    ExpiresAt time.Time
}

// NewRateLimitCache creates a new rate limit cache
func NewRateLimitCache(store store.Store, ttl time.Duration, logger Logger) *RateLimitCache {
    return &RateLimitCache{
        store:    store,
        limits:   make(map[string]*CachedLimit),
        usage:    make(map[string]*CachedUsage),
        ttl:      ttl,
        logger:   logger,
        stopChan: make(chan struct{}),
    }
}

// Start starts the cache refresh goroutine
func (c *RateLimitCache) Start() error {
    // Initial sync
    if err := c.sync(context.Background()); err != nil {
        return err
    }
    
    // Start periodic refresh
    c.ticker = time.NewTicker(c.ttl)
    go c.refreshLoop()
    
    return nil
}

// Stop stops the cache refresh
func (c *RateLimitCache) Stop() {
    if c.ticker != nil {
        c.ticker.Stop()
    }
    close(c.stopChan)
}

// refreshLoop periodically refreshes the cache
func (c *RateLimitCache) refreshLoop() {
    for {
        select {
        case <-c.stopChan:
            return
        case <-c.ticker.C:
            ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
            if err := c.sync(ctx); err != nil {
                c.logger.ErrorLog("[Cache] Failed to sync: %v", err)
            }
            cancel()
        }
    }
}

// sync synchronizes the cache with the database
func (c *RateLimitCache) sync(ctx context.Context) error {
    c.mu.Lock()
    defer c.mu.Unlock()
    
    // Sync rate limits
    limits, err := c.store.GetRateLimits(ctx, &store.RateLimitFilter{
        Enabled: boolPtr(true),
    })
    if err != nil {
        return err
    }
    
    // Update cache
    newLimits := make(map[string]*CachedLimit)
    for _, limit := range limits {
        newLimits[limit.ID] = &CachedLimit{
            RateLimitConfig: limit,
            ExpiresAt:      time.Now().Add(c.ttl),
        }
    }
    c.limits = newLimits
    
    // Sync usage (get current usage for all active tokens)
    // This is a simplified version - in production, you'd want to be more selective
    usage, err := c.store.GetCurrentUsage(ctx, "", "", "")
    if err != nil {
        c.logger.WarnLog("[Cache] Failed to sync usage: %v", err)
    } else {
        newUsage := make(map[string]*CachedUsage)
        for key, stats := range usage {
            newUsage[key] = &CachedUsage{
                UsageStats: stats,
                ExpiresAt:  time.Now().Add(c.ttl),
            }
        }
        c.usage = newUsage
    }
    
    c.lastSync = time.Now()
    c.logger.DebugLog("[Cache] Synced %d limits and %d usage entries", len(c.limits), len(c.usage))
    
    return nil
}

// GetApplicableLimits retrieves applicable rate limits for a request
func (c *RateLimitCache) GetApplicableLimits(providerID, tokenID, model string) ([]*RateLimitConfig, error) {
    c.mu.RLock()
    defer c.mu.RUnlock()
    
    var applicable []*RateLimitConfig
    
    // Collect all applicable limits
    for _, cached := range c.limits {
        limit := cached.RateLimitConfig
        
        // Check if limit has expired
        if time.Now().After(cached.ExpiresAt) {
            continue
        }
        
        // Check if limit applies
        if !c.limitApplies(limit, providerID, tokenID, model) {
            continue
        }
        
        applicable = append(applicable, limit)
    }
    
    // Sort by priority (higher priority first)
    sort.Slice(applicable, func(i, j int) bool {
        return applicable[i].Priority > applicable[j].Priority
    })
    
    return applicable, nil
}

// limitApplies checks if a rate limit applies to a request
func (c *RateLimitCache) limitApplies(limit *RateLimitConfig, providerID, tokenID, model string) bool {
    // Check provider
    if limit.ProviderID != providerID {
        return false
    }
    
    // Check token (nil = provider-wide)
    if limit.TokenID != "" && limit.TokenID != tokenID {
        return false
    }
    
    // Check model (nil = all models)
    if limit.Model != "" && limit.Model != model {
        return false
    }
    
    return true
}

// GetCurrentUsage retrieves current usage from cache
func (c *RateLimitCache) GetCurrentUsage(providerID, tokenID, model string) (map[TimeWindow]*UsageStats, error) {
    c.mu.RLock()
    defer c.mu.RUnlock()
    
    result := make(map[TimeWindow]*UsageStats)
    
    // Build cache keys
    keys := []string{
        c.buildUsageKey(providerID, tokenID, model, TimeWindowDay),
        c.buildUsageKey(providerID, tokenID, model, TimeWindowMinute),
        c.buildUsageKey(providerID, tokenID, model, TimeWindowSecond),
    }
    
    for _, key := range keys {
        cached, exists := c.usage[key]
        if exists && time.Now().Before(cached.ExpiresAt) {
            result[cached.Period] = cached.UsageStats
        }
    }
    
    return result, nil
}

// buildUsageKey builds a cache key for usage
func (c *RateLimitCache) buildUsageKey(providerID, tokenID, model string, window TimeWindow) string {
    return fmt.Sprintf("%s:%s:%s:%s", providerID, tokenID, model, window)
}

// UpdateUsage updates usage in the cache (write-through)
func (c *RateLimitCache) UpdateUsage(record *UsageRecord) {
    c.mu.Lock()
    defer c.mu.Unlock()
    
    // Update all time windows
    windows := []TimeWindow{TimeWindowDay, TimeWindowMinute, TimeWindowSecond}
    
    for _, window := range windows {
        key := c.buildUsageKey(record.ProviderID, record.TokenID, record.Model, window)
        
        cached, exists := c.usage[key]
        if !exists {
            cached = &CachedUsage{
                UsageStats: &UsageStats{
                    ProviderID: record.ProviderID,
                    TokenID:    record.TokenID,
                    Model:      record.Model,
                    Period:     window,
                },
                ExpiresAt: time.Now().Add(c.ttl),
            }
            c.usage[key] = cached
        }
        
        // Update counts
        cached.RequestCount += int64(record.RequestCount)
        cached.TokenCount += int64(record.TokenCount)
    }
}
```

---

## Task 2.3: Implement `RateLimitChecker`

**File:** `qwencoder-proxy/ratelimit/checker.go`

Create the rate limit checker with optimized limit evaluation.

**Implementation Requirements:**

```go
package ratelimit

import (
    "context"
    "time"
)

// RateLimitChecker provides optimized rate limit checking
type RateLimitChecker struct {
    cache  *RateLimitCache
    logger Logger
}

// NewRateLimitChecker creates a new rate limit checker
func NewRateLimitChecker(cache *RateLimitCache, logger Logger) *RateLimitChecker {
    return &RateLimitChecker{
        cache:  cache,
        logger: logger,
    }
}

// Check checks if a request is allowed
func (c *RateLimitChecker) Check(ctx context.Context, req *LimitCheckRequest) (*LimitCheckResult, error) {
    // Get applicable limits from cache (O(1) lookup)
    limits, err := c.cache.GetApplicableLimits(req.ProviderID, req.TokenID, req.Model)
    if err != nil {
        c.logger.ErrorLog("[Checker] Failed to get limits: %v", err)
        return &LimitCheckResult{Allowed: true, Reason: "error"}, nil
    }
    
    // Get current usage from cache (O(1) lookup)
    usage, err := c.cache.GetCurrentUsage(req.ProviderID, req.TokenID, req.Model)
    if err != nil {
        c.logger.ErrorLog("[Checker] Failed to get usage: %v", err)
        return &LimitCheckResult{Allowed: true, Reason: "error"}, nil
    }
    
    // Check each limit (typically 1-3 limits)
    for _, limit := range limits {
        result := c.checkLimit(limit, usage, req)
        if !result.Allowed {
            return result, nil
        }
    }
    
    return &LimitCheckResult{
        Allowed:       true,
        Reason:        "ok",
        CurrentUsage:  c.mergeUsage(usage),
        AvailableQuota: c.calculateQuota(limits, usage),
    }, nil
}

// checkLimit checks a single rate limit
func (c *RateLimitChecker) checkLimit(limit *RateLimitConfig, usage map[TimeWindow]*UsageStats, req *LimitCheckRequest) *LimitCheckResult {
    window, timeWindow := c.getWindowForLimitType(limit.LimitType)
    if window == "" {
        return &LimitCheckResult{Allowed: true, Reason: "ok"}
    }
    
    windowUsage, exists := usage[window]
    if !exists {
        return &LimitCheckResult{Allowed: true, Reason: "ok"}
    }
    
    var used int64
    if limit.LimitType == "token" {
        used = windowUsage.TokenCount
    } else {
        used = windowUsage.RequestCount
    }
    
    if used >= limit.LimitValue {
        retryAfter := c.calculateRetryAfter(timeWindow, windowUsage.StartTime)
        return &LimitCheckResult{
            Allowed:    false,
            Reason:     limit.LimitType + "_limit",
            RetryAfter: retryAfter,
            CurrentUsage: windowUsage,
        }
    }
    
    return &LimitCheckResult{Allowed: true}
}

// getWindowForLimitType returns the time window for a limit type
func (c *RateLimitChecker) getWindowForLimitType(limitType string) (TimeWindow, time.Duration) {
    switch limitType {
    case "daily":
        return TimeWindowDay, 24 * time.Hour
    case "burst":
        return TimeWindowMinute, 1 * time.Minute
    case "token":
        return TimeWindowDay, 24 * time.Hour
    default:
        return "", 0
    }
}

// calculateRetryAfter calculates retry-after duration
func (c *RateLimitChecker) calculateRetryAfter(windowDuration time.Duration, windowStart time.Time) time.Duration {
    now := time.Now()
    windowEnd := windowStart.Add(windowDuration)
    
    if windowEnd.Before(now) {
        return 0
    }
    
    return windowEnd.Sub(now)
}

// mergeUsage merges usage from multiple windows
func (c *RateLimitChecker) mergeUsage(usage map[TimeWindow]*UsageStats) *UsageStats {
    // Return the most detailed usage available
    if day, ok := usage[TimeWindowDay]; ok {
        return day
    }
    if minute, ok := usage[TimeWindowMinute]; ok {
        return minute
    }
    if second, ok := usage[TimeWindowSecond]; ok {
        return second
    }
    
    return &UsageStats{}
}

// calculateQuota calculates quota information
func (c *RateLimitChecker) calculateQuota(limits []*RateLimitConfig, usage map[TimeWindow]*UsageStats) *QuotaInfo {
    quota := &QuotaInfo{}
    
    for _, limit := range limits {
        switch limit.LimitType {
        case "daily":
            if day, ok := usage[TimeWindowDay]; ok {
                quota.DailyLimit = limit.LimitValue
                quota.DailyUsed = day.RequestCount
                quota.DailyRemaining = max(0, quota.DailyLimit-quota.DailyUsed)
            }
        case "burst":
            if minute, ok := usage[TimeWindowMinute]; ok {
                quota.BurstLimit = limit.LimitValue
                quota.BurstUsed = minute.RequestCount
                quota.BurstRemaining = max(0, quota.BurstLimit-quota.BurstUsed)
            }
        case "token":
            if day, ok := usage[TimeWindowDay]; ok {
                quota.TokenLimit = limit.LimitValue
                quota.TokenUsed = day.TokenCount
                quota.TokenRemaining = max(0, quota.TokenLimit-quota.TokenUsed)
            }
        }
    }
    
    return quota
}
```

---

## Task 2.4: Add Periodic Cache Refresh

The cache refresh is already implemented in `RateLimitCache` with the `refreshLoop` method. Ensure it's properly integrated.

**Additional Requirements:**

1. Add metrics for cache hit/miss rates
2. Add metrics for sync duration
3. Add logging for cache refresh events

---

## Task 2.5: Implement Provider Schema Interfaces

**File:** `qwencoder-proxy/ratelimit/provider/schema.go`

Create the provider schema interface for extensible rate limit schemas.

**Implementation Requirements:**

```go
package provider

import (
    "time"
    
    "github.com/sunbankio/qwencoder-proxy/ratelimit"
)

// ProviderRateLimitSchema defines the interface for provider-specific rate limit schemas
type ProviderRateLimitSchema interface {
    // GetProviderID returns the provider ID
    GetProviderID() string
    
    // GetDefaultLimits returns default rate limits for the provider
    GetDefaultLimits() []*ratelimit.RateLimitConfig
    
    // ParseLimitFromResponse parses rate limit info from provider response
    ParseLimitFromResponse(resp []byte) (*ratelimit.RateLimitConfig, error)
    
    // GetRetryAfter extracts retry-after duration from error response
    GetRetryAfter(err error) time.Duration
    
    // SupportsTokenLevelLimits returns true if provider supports per-token limits
    SupportsTokenLevelLimits() bool
    
    // GetLimitPriority returns the priority for this provider's limits
    GetLimitPriority() int
}

// ProviderSchemaFactory creates provider schemas
type ProviderSchemaFactory interface {
    // GetSchema returns the schema for a provider
    GetSchema(providerID string) (ProviderRateLimitSchema, error)
    
    // RegisterSchema registers a schema for a provider
    RegisterSchema(schema ProviderRateLimitSchema) error
    
    // GetAllSchemas returns all registered schemas
    GetAllSchemas() map[string]ProviderRateLimitSchema
}

// schemaFactory implements ProviderSchemaFactory
type schemaFactory struct {
    schemas map[string]ProviderRateLimitSchema
    mu      sync.RWMutex
}

// NewSchemaFactory creates a new provider schema factory
func NewSchemaFactory() ProviderSchemaFactory {
    return &schemaFactory{
        schemas: make(map[string]ProviderRateLimitSchema),
    }
}

// GetSchema returns the schema for a provider
func (f *schemaFactory) GetSchema(providerID string) (ProviderRateLimitSchema, error) {
    f.mu.RLock()
    defer f.mu.RUnlock()
    
    schema, exists := f.schemas[providerID]
    if !exists {
        return nil, fmt.Errorf("schema not found for provider: %s", providerID)
    }
    
    return schema, nil
}

// RegisterSchema registers a schema for a provider
func (f *schemaFactory) RegisterSchema(schema ProviderRateLimitSchema) error {
    f.mu.Lock()
    defer f.mu.Unlock()
    
    f.schemas[schema.GetProviderID()] = schema
    return nil
}

// GetAllSchemas returns all registered schemas
func (f *schemaFactory) GetAllSchemas() map[string]ProviderRateLimitSchema {
    f.mu.RLock()
    defer f.mu.RUnlock()
    
    result := make(map[string]ProviderRateLimitSchema)
    for k, v := range f.schemas {
        result[k] = v
    }
    
    return result
}
```

---

## Task 2.6: Implement Default Schemas (Qwen, Gemini)

**File:** `qwencoder-proxy/ratelimit/provider/qwen.go`

```go
package provider

import (
    "fmt"
    "time"
    
    "github.com/sunbankio/qwencoder-proxy/ratelimit"
)

// QwenSchema implements the rate limit schema for Qwen
type QwenSchema struct{}

// NewQwenSchema creates a new Qwen schema
func NewQwenSchema() *QwenSchema {
    return &QwenSchema{}
}

// GetProviderID returns the provider ID
func (s *QwenSchema) GetProviderID() string {
    return "qwen"
}

// GetDefaultLimits returns default rate limits for Qwen
func (s *QwenSchema) GetDefaultLimits() []*ratelimit.RateLimitConfig {
    now := time.Now()
    return []*ratelimit.RateLimitConfig{
        {
            ID:          "qwen-daily-default",
            ProviderID:  "qwen",
            LimitType:   "daily",
            LimitValue:  1000,
            TimeWindow:  24 * time.Hour,
            Enabled:     true,
            Priority:    10,
            CreatedAt:   now,
            UpdatedAt:   now,
        },
        {
            ID:          "qwen-burst-default",
            ProviderID:  "qwen",
            LimitType:   "burst",
            LimitValue:  60,
            TimeWindow:  1 * time.Minute,
            Enabled:     true,
            Priority:    20,
            CreatedAt:   now,
            UpdatedAt:   now,
        },
    }
}

// ParseLimitFromResponse parses rate limit info from Qwen response
func (s *QwenSchema) ParseLimitFromResponse(resp []byte) (*ratelimit.RateLimitConfig, error) {
    // Qwen doesn't typically return rate limit info in responses
    // Return nil to indicate no rate limit info available
    return nil, nil
}

// GetRetryAfter extracts retry-after duration from Qwen error response
func (s *QwenSchema) GetRetryAfter(err error) time.Duration {
    // Qwen may return rate limit errors with retry-after info
    // Parse from error message if available
    // Default to 1 minute if rate limited
    return 1 * time.Minute
}

// SupportsTokenLevelLimits returns true if provider supports per-token limits
func (s *QwenSchema) SupportsTokenLevelLimits() bool {
    return true
}

// GetLimitPriority returns the priority for Qwen's limits
func (s *QwenSchema) GetLimitPriority() int {
    return 10
}
```

**File:** `qwencoder-proxy/ratelimit/provider/gemini.go`

```go
package provider

import (
    "encoding/json"
    "fmt"
    "time"
    
    "github.com/sunbankio/qwencoder-proxy/ratelimit"
)

// GeminiSchema implements the rate limit schema for Gemini
type GeminiSchema struct{}

// NewGeminiSchema creates a new Gemini schema
func NewGeminiSchema() *GeminiSchema {
    return &GeminiSchema{}
}

// GetProviderID returns the provider ID
func (s *GeminiSchema) GetProviderID() string {
    return "gemini-cli"
}

// GetDefaultLimits returns default rate limits for Gemini
func (s *GeminiSchema) GetDefaultLimits() []*ratelimit.RateLimitConfig {
    now := time.Now()
    return []*ratelimit.RateLimitConfig{
        {
            ID:          "gemini-daily-default",
            ProviderID:  "gemini-cli",
            LimitType:   "daily",
            LimitValue:  1500,
            TimeWindow:  24 * time.Hour,
            Enabled:     true,
            Priority:    10,
            CreatedAt:   now,
            UpdatedAt:   now,
        },
        {
            ID:          "gemini-burst-default",
            ProviderID:  "gemini-cli",
            LimitType:   "burst",
            LimitValue:  60,
            TimeWindow:  1 * time.Minute,
            Enabled:     true,
            Priority:    20,
            CreatedAt:   now,
            UpdatedAt:   now,
        },
        {
            ID:          "gemini-token-default",
            ProviderID:  "gemini-cli",
            LimitType:   "token",
            LimitValue:  1000000,
            TimeWindow:  24 * time.Hour,
            Enabled:     true,
            Priority:    15,
            CreatedAt:   now,
            UpdatedAt:   now,
        },
    }
}

// ParseLimitFromResponse parses rate limit info from Gemini response
func (s *GeminiSchema) ParseLimitFromResponse(resp []byte) (*ratelimit.RateLimitConfig, error) {
    // Gemini may return rate limit info in response headers or body
    // Parse if available
    var result map[string]interface{}
    if err := json.Unmarshal(resp, &result); err != nil {
        return nil, err
    }
    
    // Check for rate limit info
    if _, ok := result["rateLimit"]; ok {
        // Parse rate limit info
        // This is a placeholder - actual implementation depends on Gemini's response format
        return nil, nil
    }
    
    return nil, nil
}

// GetRetryAfter extracts retry-after duration from Gemini error response
func (s *GeminiSchema) GetRetryAfter(err error) time.Duration {
    // Gemini returns 429 with retry-after header
    // Parse from error if available
    // Default to 1 minute
    return 1 * time.Minute
}

// SupportsTokenLevelLimits returns true if provider supports per-token limits
func (s *GeminiSchema) SupportsTokenLevelLimits() bool {
    return true
}

// GetLimitPriority returns the priority for Gemini's limits
func (s *GeminiSchema) GetLimitPriority() int {
    return 10
}
```

---

## Task 2.7: Write Unit Tests for Rate Limiting

**File:** `qwencoder-proxy/ratelimit/limiter_test.go`

Write comprehensive unit tests for the rate limiter.

**Test Cases:**

1. **Rate Limit Check Tests:**
   - Test request allowed when under limit
   - Test request denied when over daily limit
   - Test request denied when over burst limit
   - Test request denied when over token limit
   - Test multiple limits checked in priority order

2. **Cache Tests:**
   - Test cache initialization
   - Test cache refresh
   - Test cache expiration
   - Test cache hit/miss

3. **Provider Schema Tests:**
   - Test default limits for Qwen
   - Test default limits for Gemini
   - Test schema registration

**Test Template:**

```go
package ratelimit

import (
    "context"
    "testing"
    "time"
    
    "github.com/sunbankio/qwencoder-proxy/ratelimit/provider"
    "github.com/sunbankio/qwencoder-proxy/ratelimit/store"
)

func setupTestLimiter(t *testing.T) RateLimiter {
    store := setupTestStore(t)
    schemaFactory := provider.NewSchemaFactory()
    config := DefaultConfig()
    logger := newTestLogger()
    
    limiter := NewRateLimiter(store, schemaFactory, config, logger)
    if err := limiter.Start(); err != nil {
        t.Fatalf("Failed to start limiter: %v", err)
    }
    
    t.Cleanup(func() {
        limiter.Stop()
    })
    
    return limiter
}

func TestCheckLimit_UnderLimit(t *testing.T) {
    limiter := setupTestLimiter(t)
    
    req := &LimitCheckRequest{
        ProviderID: "qwen",
        TokenID:    "test-token",
        Model:      "qwen-turbo",
    }
    
    result, err := limiter.CheckLimit(context.Background(), req)
    if err != nil {
        t.Fatalf("CheckLimit failed: %v", err)
    }
    
    if !result.Allowed {
        t.Errorf("Expected request to be allowed, got: %s", result.Reason)
    }
}

// Implement more tests...
```

---

## Task 2.8: Write Integration Tests

**File:** `qwencoder-proxy/ratelimit/integration_test.go`

Write integration tests that test the entire rate limiting flow.

**Test Cases:**

1. **End-to-End Rate Limiting:**
   - Create rate limit
   - Check limit (allowed)
   - Record usage
   - Check limit again (denied if over limit)

2. **Cache Integration:**
   - Test cache refresh from database
   - Test cache updates on database changes

3. **Multiple Limits:**
   - Test multiple limits for same provider
   - Test limit priority ordering

---

## Deliverables

After completing this phase, you should have:

1. ✅ `RateLimiter` interface and implementation
2. ✅ `RateLimitCache` with in-memory caching
3. ✅ `RateLimitChecker` for optimized limit evaluation
4. ✅ Periodic cache refresh working
5. ✅ Provider schema interfaces
6. ✅ Default schemas for Qwen and Gemini
7. ✅ Unit tests with >90% coverage
8. ✅ Integration tests

---

## Success Criteria

- [ ] Rate limit checks complete in <1ms
- [ ] Cache hit rate >95%
- [ ] All unit tests pass
- [ ] All integration tests pass
- [ ] Code follows Go idioms
- [ ] Error handling is comprehensive
- [ ] Logging is appropriate

---

## Next Phase

After completing Phase 2, proceed to **Phase 3: Async Tracking** which implements non-blocking usage tracking with worker pools.

**File:** `../todo/phase-3-async-tracking.md`
