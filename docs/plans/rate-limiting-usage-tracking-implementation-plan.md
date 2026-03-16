# Rate Limiting and Usage Tracking System - Implementation Plan

**Document Version:** 1.0  
**Date:** 2026-03-09  
**Status:** Draft

---

## Table of Contents

1. [Executive Summary](#executive-summary)
2. [System Overview](#system-overview)
3. [Architecture Design](#architecture-design)
4. [Database Schema](#database-schema)
5. [Package Structure](#package-structure)
6. [Concurrent Processing Design](#concurrent-processing-design)
7. [Rate Limiting Logic](#rate-limiting-logic)
8. [Admin Dashboard Integration](#admin-dashboard-integration)
9. [Implementation Roadmap](#implementation-roadmap)
10. [Integration Guide](#integration-guide)
11. [Testing Strategy](#testing-strategy)
12. [Performance Considerations](#performance-considerations)

---

## Executive Summary

This document outlines a comprehensive implementation plan for a high-performance rate limiting and usage tracking system for the QWEncoder Proxy AI API gateway. The system provides:

- **Granular tracking** per provider and per API token
- **Multi-dimensional metrics**: requests/day, requests/minute, requests/second, tokens/day, tokens/minute
- **Non-blocking architecture** using Go channels and worker pools
- **Dynamic rate limits** configurable at runtime via admin dashboard
- **Automatic routing** to optimal tokens based on real-time quota availability
- **Extensible provider abstraction** for future provider integrations

### Key Requirements

| Requirement | Description |
|-------------|-------------|
| **Tracking Granularity** | Per provider and per API token |
| **Metrics** | Requests/day, requests/minute, requests/second, tokens/day, tokens/minute |
| **Rate Limits** | Daily caps, burst limits (e.g., Qwen 1000/day, Gemini 60/sec) |
| **Performance** | Zero impact on API response times (non-blocking) |
| **Configuration** | Runtime modification without code deployment |
| **Extensibility** | Support for new providers with unique rate limit schemas |

---

## System Overview

### Current Architecture Analysis

The existing codebase provides a solid foundation:

- **Provider abstraction**: [`provider.Provider`](qwencoder-proxy/provider/provider.go:34) interface
- **Token management**: [`token.TokenManager`](qwencoder-proxy/internal/token/token_selection.go:136) with selection strategies
- **SQLite storage**: [`token.SQLiteStore`](qwencoder-proxy/internal/token/sqlite_store.go:93) with WAL mode and prepared statements
- **REST API**: [`restapi.Server`](qwencoder-proxy/restapi/rest_api.go:86) for dashboard and OAuth flows
- **Web dashboard**: JavaScript-based admin interface

### Rate Limiting System Components

```
┌─────────────────────────────────────────────────────────────────┐
│                         API Gateway                              │
├─────────────────────────────────────────────────────────────────┤
│  ┌─────────────┐    ┌──────────────────────────────────────┐   │
│  │   Proxy     │───▶│     Rate Limiting Middleware          │   │
│  │  Handlers   │    │  (Check limits before proxying)       │   │
│  └─────────────┘    └──────────────────────────────────────┘   │
│                            │                                     │
│                            ▼                                     │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │              Usage Tracking System                       │   │
│  │  ┌─────────────┐    ┌────────────────────────────────┐   │   │
│  │  │   Metrics   │───▶│   Async Write Channel         │   │   │
│  │  │  Collector  │    │   (buffered, non-blocking)    │   │   │
│  │  └─────────────┘    └────────────────────────────────┘   │   │
│  │                            │                               │   │
│  │                            ▼                               │   │
│  │  ┌──────────────────────────────────────────────────┐   │   │
│  │  │         Worker Pool (N workers)                    │   │   │
│  │  │  ┌─────────┐  ┌─────────┐  ┌─────────┐          │   │   │
│  │  │  │ Worker 1│  │ Worker 2│  │ Worker N│  ...     │   │   │
│  │  │  └─────────┘  └─────────┘  └─────────┘          │   │   │
│  │  └──────────────────────────────────────────────────┘   │   │
│  │                            │                               │   │
│  │                            ▼                               │   │
│  │  ┌──────────────────────────────────────────────────┐   │   │
│  │  │         SQLite Database (Async Writes)            │   │   │
│  │  │  - rate_limits (configurable limits)             │   │   │
│  │  │  - usage_tracking (request/token counters)       │   │   │
│  │  │  - usage_history (historical data)               │   │   │
│  │  └──────────────────────────────────────────────────┘   │   │
│  └─────────────────────────────────────────────────────────┘   │
│                            │                                     │
│                            ▼                                     │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │              Smart Token Selector                         │   │
│  │  (Routes to token with best quota availability)          │   │
│  └─────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────────────┐
│                    Admin Dashboard                              │
│  - Real-time usage visualization                               │
│  - Rate limit configuration                                    │
│  - Token quota monitoring                                       │
└─────────────────────────────────────────────────────────────────┘
```

---

## Architecture Design

### Core Principles

1. **Non-blocking Operations**: All tracking writes are asynchronous
2. **Separation of Concerns**: Rate limiting, tracking, and storage are decoupled
3. **Interface-based Design**: Easy to mock and extend
4. **Go Idioms**: Channels for communication, worker pools for concurrency

### Key Interfaces

```go
// RateLimiter defines the interface for rate limiting
type RateLimiter interface {
    // CheckLimit checks if a request is allowed
    CheckLimit(ctx context.Context, req *LimitCheckRequest) (*LimitCheckResult, error)
    
    // RecordUsage records usage metrics (non-blocking)
    RecordUsage(ctx context.Context, usage *UsageRecord) error
    
    // GetUsage retrieves current usage statistics
    GetUsage(ctx context.Context, query *UsageQuery) (*UsageStats, error)
}

// LimitCheckRequest represents a rate limit check request
type LimitCheckRequest struct {
    ProviderID    string
    TokenID       string
    Model         string
    RequestType   string // "chat", "completion", etc.
    EstimatedTokens int // Estimated token count (optional)
}

// LimitCheckResult represents the result of a rate limit check
type LimitCheckResult struct {
    Allowed       bool
    Reason        string // "ok", "daily_limit", "burst_limit", "token_limit"
    RetryAfter    time.Duration
    CurrentUsage  *UsageStats
    AvailableQuota *QuotaInfo
}

// UsageRecord represents a usage metric to record
type UsageRecord struct {
    ProviderID      string
    TokenID         string
    Model           string
    RequestType     string
    RequestCount    int
    TokenCount      int
    Timestamp       time.Time
    ResponseTimeMs  int64
    Success         bool
    ErrorCode       string
}

// UsageStats represents usage statistics
type UsageStats struct {
    ProviderID      string
    TokenID         string
    Model           string
    Period          TimeWindow // "day", "minute", "second"
    RequestCount    int64
    TokenCount      int64
    StartTime       time.Time
    EndTime         time.Time
}

// QuotaInfo represents quota availability
type QuotaInfo struct {
    DailyLimit      int64
    DailyUsed       int64
    DailyRemaining  int64
    BurstLimit      int64
    BurstUsed       int64
    BurstRemaining  int64
    TokenLimit      int64
    TokenUsed       int64
    TokenRemaining  int64
}
```

### Component Hierarchy

```
ratelimit/
├── limiter.go          # Core RateLimiter interface and implementation
├── checker.go          # Rate limit checker (in-memory with periodic sync)
├── tracker.go          # Usage tracking with async writes
├── worker_pool.go      # Worker pool for database writes
├── selector.go         # Smart token selector based on quota
├── config.go           # Rate limit configuration management
├── metrics.go          # Metrics collection and aggregation
└── errors.go           # Custom error types

ratelimit/store/
├── store.go            # Storage interface
├── sqlite_store.go     # SQLite implementation
└── memory_store.go     # In-memory cache

ratelimit/provider/
├── provider.go         # Provider-specific rate limit schemas
├── qwen.go             # Qwen rate limit schema
├── gemini.go           # Gemini rate limit schema
├── antigravity.go      # Antigravity rate limit schema
└── factory.go          # Factory for creating provider schemas
```

---

## Database Schema

### New Tables

#### 1. `rate_limits` - Configurable rate limits

```sql
CREATE TABLE IF NOT EXISTS rate_limits (
    id TEXT PRIMARY KEY,
    provider_id TEXT NOT NULL,
    token_id TEXT,                    -- NULL = provider-wide limit
    model TEXT,                       -- NULL = all models
    limit_type TEXT NOT NULL,         -- "daily", "burst", "token"
    limit_value INTEGER NOT NULL,     -- The limit value
    time_window_seconds INTEGER,      -- For burst limits (e.g., 60 for per-minute)
    enabled INTEGER NOT NULL DEFAULT 1,
    priority INTEGER NOT NULL DEFAULT 0, -- Higher priority = checked first
    created_at INTEGER NOT NULL DEFAULT (strftime('%s', 'subsec') * 1000),
    updated_at INTEGER NOT NULL DEFAULT (strftime('%s', 'subsec') * 1000),
    
    FOREIGN KEY (provider_id) REFERENCES providers(id) ON DELETE CASCADE,
    FOREIGN KEY (token_id) REFERENCES tokens(id) ON DELETE CASCADE,
    UNIQUE(provider_id, token_id, model, limit_type)
);

CREATE INDEX IF NOT EXISTS idx_rate_limits_provider 
    ON rate_limits(provider_id);
CREATE INDEX IF NOT EXISTS idx_rate_limits_token 
    ON rate_limits(token_id);
CREATE INDEX IF NOT EXISTS idx_rate_limits_enabled 
    ON rate_limits(enabled) WHERE enabled = 1;
```

**Sample Data:**

```sql
-- Provider-wide daily limit for Qwen
INSERT INTO rate_limits (id, provider_id, token_id, model, limit_type, limit_value, time_window_seconds, priority)
VALUES ('qwen-daily', 'qwen', NULL, NULL, 'daily', 1000, 86400, 10);

-- Token-specific burst limit for Gemini
INSERT INTO rate_limits (id, provider_id, token_id, model, limit_type, limit_value, time_window_seconds, priority)
VALUES ('gemini-burst-token-123', 'gemini-cli', 'token-123', NULL, 'burst', 60, 60, 20);

-- Model-specific token limit for Antigravity
INSERT INTO rate_limits (id, provider_id, token_id, model, limit_type, limit_value, time_window_seconds, priority)
VALUES ('antigravity-token-gpt4', 'antigravity', NULL, 'gpt-4', 'token', 100000, 86400, 15);
```

#### 2. `usage_tracking` - Real-time usage counters

```sql
CREATE TABLE IF NOT EXISTS usage_tracking (
    id TEXT PRIMARY KEY,
    provider_id TEXT NOT NULL,
    token_id TEXT NOT NULL,
    model TEXT NOT NULL,
    window_type TEXT NOT NULL,        -- "day", "minute", "second"
    window_start INTEGER NOT NULL,     -- Start of the time window (ms timestamp)
    request_count INTEGER NOT NULL DEFAULT 0,
    token_count INTEGER NOT NULL DEFAULT 0,
    error_count INTEGER NOT NULL DEFAULT 0,
    last_updated INTEGER NOT NULL DEFAULT (strftime('%s', 'subsec') * 1000),
    
    FOREIGN KEY (provider_id) REFERENCES providers(id) ON DELETE CASCADE,
    FOREIGN KEY (token_id) REFERENCES tokens(id) ON DELETE CASCADE,
    UNIQUE(provider_id, token_id, model, window_type, window_start)
);

CREATE INDEX IF NOT EXISTS idx_usage_tracking_window 
    ON usage_tracking(provider_id, token_id, model, window_type, window_start);
CREATE INDEX IF NOT EXISTS idx_usage_tracking_updated 
    ON usage_tracking(last_updated);
```

#### 3. `usage_history` - Historical usage data (for analytics)

```sql
CREATE TABLE IF NOT EXISTS usage_history (
    id TEXT PRIMARY KEY,
    provider_id TEXT NOT NULL,
    token_id TEXT NOT NULL,
    model TEXT NOT NULL,
    request_type TEXT NOT NULL,
    request_count INTEGER NOT NULL,
    token_count INTEGER NOT NULL,
    response_time_ms INTEGER,
    success INTEGER NOT NULL,
    error_code TEXT,
    timestamp INTEGER NOT NULL,       -- Request timestamp (ms)
    
    FOREIGN KEY (provider_id) REFERENCES providers(id) ON DELETE CASCADE,
    FOREIGN KEY (token_id) REFERENCES tokens(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_usage_history_timestamp 
    ON usage_history(timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_usage_history_provider 
    ON usage_history(provider_id, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_usage_history_token 
    ON usage_history(token_id, timestamp DESC);
```

### Schema Migration Strategy

```sql
-- Migration version 001: Add rate limiting tables
INSERT INTO schema_migrations (version, applied_at, description)
VALUES (1, strftime('%s', 'subsec') * 1000, 'Add rate limiting and usage tracking tables');
```

### Data Retention Policy

| Table | Retention Period | Cleanup Strategy |
|-------|------------------|------------------|
| `usage_tracking` | 2 days | Automatic cleanup of old windows |
| `usage_history` | 30 days | Scheduled cleanup job |
| `rate_limits` | Permanent | No cleanup |

---

## Package Structure

### New Package: `ratelimit`

```
qwencoder-proxy/ratelimit/
├── limiter.go              # Core RateLimiter interface
├── checker.go              # In-memory rate limit checker
├── tracker.go              # Usage tracker with async writes
├── worker_pool.go          # Worker pool for DB writes
├── selector.go             # Smart token selector
├── config.go               # Configuration management
├── metrics.go              # Metrics aggregation
├── errors.go               # Custom errors
│
├── store/
│   ├── store.go            # Storage interface
│   ├── sqlite_store.go     # SQLite implementation
│   └── cache.go            # In-memory cache layer
│
├── provider/
│   ├── provider.go         # Provider schema interface
│   ├── qwen.go             # Qwen schema
│   ├── gemini.go           # Gemini schema
│   ├── antigravity.go      # Antigravity schema
│   ├── kiro.go             # Kiro schema
│   ├── iflow.go            # iFlow schema
│   └── factory.go          # Schema factory
│
├── middleware/
│   ├── middleware.go       # HTTP middleware for rate limiting
│   └── context.go          # Context key definitions
│
└── api/
    ├── handlers.go         # REST API handlers
    └── routes.go           # Route definitions
```

### Integration Points

| Existing Package | Integration Point | Description |
|------------------|------------------|-------------|
| `proxy` | Middleware | Rate limiting middleware before proxy handlers |
| `token` | Selector | Smart token selection based on quota |
| `restapi` | API endpoints | Admin API for rate limit configuration |
| `config` | Configuration | Rate limiter config in main config |

---

## Concurrent Processing Design

### Non-Blocking Architecture

The system uses a producer-consumer pattern with buffered channels to ensure API response times are not impacted by database writes.

#### Channel Design

```go
// Usage tracking channel (buffered, non-blocking)
type UsageTracker struct {
    usageChan    chan *UsageRecord
    workerPool   *WorkerPool
    bufferConfig BufferConfig
}

type BufferConfig struct {
    ChannelSize      int    // Channel buffer size (default: 1000)
    FlushInterval    time.Duration // Batch flush interval (default: 100ms)
    MaxBatchSize     int    // Max records per batch (default: 100)
    DropWhenFull     bool   // Drop records when channel is full (default: false)
}

// Worker pool configuration
type WorkerPoolConfig struct {
    NumWorkers       int           // Number of worker goroutines (default: 4)
    QueueSize        int           // Internal queue size (default: 500)
    MaxRetries       int           // Max retries for failed writes (default: 3)
    RetryDelay       time.Duration // Delay between retries (default: 100ms)
    ShutdownTimeout  time.Duration // Graceful shutdown timeout (default: 5s)
}
```

#### Worker Pool Implementation

```go
// WorkerPool manages a pool of worker goroutines for async writes
type WorkerPool struct {
    config      *WorkerPoolConfig
    jobChan     chan *WriteJob
    workers     []*worker
    stopChan    chan struct{}
    wg          sync.WaitGroup
    logger      logging.Logger
    metrics     *WorkerMetrics
}

type WriteJob struct {
    Record      *UsageRecord
    RetryCount  int
    Callback    func(error)
}

type worker struct {
    id          int
    jobChan     <-chan *WriteJob
    stopChan    <-chan struct{}
    store       Store
    logger      logging.Logger
    metrics     *WorkerMetrics
}

func (w *worker) start() {
    w.wg.Add(1)
    go func() {
        defer w.wg.Done()
        for {
            select {
            case <-w.stopChan:
                return
            case job := <-w.jobChan:
                w.processJob(job)
            }
        }
    }()
}

func (w *worker) processJob(job *WriteJob) {
    var err error
    for i := 0; i <= w.config.MaxRetries; i++ {
        err = w.store.WriteUsageRecord(job.Record)
        if err == nil {
            w.metrics.RecordSuccess()
            if job.Callback != nil {
                job.Callback(nil)
            }
            return
        }
        
        // Retry with backoff
        if i < w.config.MaxRetries {
            time.Sleep(w.config.RetryDelay * time.Duration(i+1))
        }
    }
    
    // All retries failed
    w.metrics.RecordFailure()
    w.logger.ErrorLog("[Worker %d] Failed to write usage record after %d retries: %v", 
        w.id, w.config.MaxRetries, err)
    if job.Callback != nil {
        job.Callback(err)
    }
}
```

#### Usage Tracking Flow

```
┌──────────────┐
│  API Request │
└──────┬───────┘
       │
       ▼
┌─────────────────────────┐
│  Rate Limit Check      │  (In-memory, O(1))
│  - Check cached limits │
│  - Return immediately  │
└──────┬──────────────────┘
       │
       ▼
┌─────────────────────────┐
│  Proxy Request          │
└──────┬──────────────────┘
       │
       ▼
┌─────────────────────────┐
│  Record Usage           │  (Non-blocking)
│  - Create UsageRecord   │
│  - Send to channel      │  (select with default)
└──────┬──────────────────┘
       │
       ▼
┌─────────────────────────┐
│  Buffered Channel       │  (size: 1000)
│  usageChan             │
└──────┬──────────────────┘
       │
       ▼
┌─────────────────────────────────┐
│  Worker Pool (4 workers)        │
│  ┌─────┐ ┌─────┐ ┌─────┐ ┌─────┐ │
│  │ W1  │ │ W2  │ │ W3  │ │ W4  │ │
│  └──┬──┘ └──┬──┘ └──┬──┘ └──┬──┘ │
└─────┼──────┼──────┼──────┼─────┘
      │      │      │      │
      └──────┴──────┴──────┘
                    │
                    ▼
         ┌──────────────────┐
         │  SQLite Database │
         │  (Async Writes)  │
         └──────────────────┘
```

### In-Memory Caching

To minimize database reads for rate limit checks, the system maintains an in-memory cache:

```go
// RateLimitCache holds in-memory rate limit data
type RateLimitCache struct {
    mu       sync.RWMutex
    limits   map[string]*CachedLimit
    usage    map[string]*CachedUsage
    ttl      time.Duration
    lastSync time.Time
}

type CachedLimit struct {
    ProviderID    string
    TokenID       string
    Model         string
    LimitType     string
    LimitValue    int64
    TimeWindow    time.Duration
    ExpiresAt     time.Time
}

type CachedUsage struct {
    ProviderID    string
    TokenID       string
    Model         string
    WindowType    string
    WindowStart   time.Time
    RequestCount  int64
    TokenCount    int64
    LastUpdated   time.Time
}
```

**Cache Refresh Strategy:**

1. **Periodic Sync**: Refresh every 30 seconds from database
2. **On-Demand Sync**: Refresh when a limit is not found in cache
3. **Write-Through**: Update cache on write operations

---

## Rate Limiting Logic

### Limit Check Algorithm

```go
// CheckLimit checks if a request is allowed
func (r *RateLimiter) CheckLimit(ctx context.Context, req *LimitCheckRequest) (*LimitCheckResult, error) {
    // 1. Get applicable rate limits (provider, token, model specific)
    limits := r.cache.GetApplicableLimits(req.ProviderID, req.TokenID, req.Model)
    
    // 2. Get current usage from cache
    usage := r.cache.GetCurrentUsage(req.ProviderID, req.TokenID, req.Model)
    
    // 3. Check each limit type in priority order
    for _, limit := range limits {
        result := r.checkSingleLimit(limit, usage, req)
        if !result.Allowed {
            return result, nil
        }
    }
    
    // 4. All limits passed
    return &LimitCheckResult{
        Allowed:      true,
        Reason:       "ok",
        CurrentUsage: usage,
        AvailableQuota: r.calculateQuotaInfo(limits, usage),
    }, nil
}

func (r *RateLimiter) checkSingleLimit(limit *CachedLimit, usage *CachedUsage, req *LimitCheckRequest) *LimitCheckResult {
    windowUsage := r.getWindowUsage(usage, limit.TimeWindow)
    
    if windowUsage >= limit.LimitValue {
        // Calculate retry after based on window type
        retryAfter := r.calculateRetryAfter(limit.TimeWindow, usage.WindowStart)
        
        return &LimitCheckResult{
            Allowed:    false,
            Reason:     limit.LimitType + "_limit",
            RetryAfter: retryAfter,
            CurrentUsage: usage,
        }
    }
    
    return &LimitCheckResult{Allowed: true}
}
```

### Smart Token Selection

The system extends the existing [`token.TokenManager`](qwencoder-proxy/internal/token/token_selection.go:136) to include quota-aware selection:

```go
// QuotaAwareSelectionStrategy selects tokens based on quota availability
type QuotaAwareSelectionStrategy struct {
    rateLimiter RateLimiter
    fallback    SelectionStrategy // Fallback strategy when quotas are equal
}

func (s *QuotaAwareSelectionStrategy) SelectToken(tokens []ProviderToken) (*ProviderToken, error) {
    if len(tokens) == 0 {
        return nil, ErrNoValidTokens
    }
    
    // 1. Filter valid tokens
    validTokens := filterValidTokens(tokens)
    if len(validTokens) == 0 {
        return nil, ErrNoValidTokens
    }
    
    // 2. Get quota info for each token
    tokenQuotas := make([]*TokenQuotaInfo, 0, len(validTokens))
    for _, token := range validTokens {
        quota, err := s.rateLimiter.GetTokenQuota(token.ProviderID, token.ID)
        if err != nil {
            s.logger.WarnLog("Failed to get quota for token %s: %v", token.ID, err)
            continue
        }
        tokenQuotas = append(tokenQuotas, &TokenQuotaInfo{
            Token: token,
            Quota: quota,
            Score: s.calculateQuotaScore(quota),
        })
    }
    
    // 3. Sort by quota score (highest first)
    sort.Slice(tokenQuotas, func(i, j int) bool {
        return tokenQuotas[i].Score > tokenQuotas[j].Score
    })
    
    // 4. Return token with highest quota
    if len(tokenQuotas) > 0 {
        return tokenQuotas[0].Token, nil
    }
    
    // 5. Fallback to original strategy if no quota info available
    return s.fallback.SelectToken(validTokens)
}

func (s *QuotaAwareSelectionStrategy) calculateQuotaScore(quota *QuotaInfo) float64 {
    // Score based on remaining quota across all dimensions
    dailyScore := float64(quota.DailyRemaining) / float64(quota.DailyLimit)
    burstScore := float64(quota.BurstRemaining) / float64(quota.BurstLimit)
    tokenScore := float64(quota.TokenRemaining) / float64(quota.TokenLimit)
    
    // Weighted average (daily is most important)
    return (dailyScore * 0.5) + (burstScore * 0.3) + (tokenScore * 0.2)
}
```

### Provider-Specific Rate Limit Schemas

Each provider can have unique rate limit characteristics:

```go
// ProviderRateLimitSchema defines the rate limit schema for a provider
type ProviderRateLimitSchema interface {
    // GetDefaultLimits returns default rate limits for the provider
    GetDefaultLimits() []*RateLimitConfig
    
    // ParseLimitFromResponse parses rate limit info from provider response
    ParseLimitFromResponse(resp []byte) (*RateLimitInfo, error)
    
    // GetRetryAfter extracts retry-after duration from error response
    GetRetryAfter(err error) time.Duration
    
    // SupportsTokenLevelLimits returns true if provider supports per-token limits
    SupportsTokenLevelLimits() bool
}

// Example: Qwen Schema
type QwenSchema struct{}

func (s *QwenSchema) GetDefaultLimits() []*RateLimitConfig {
    return []*RateLimitConfig{
        {
            LimitType:     "daily",
            LimitValue:    1000,
            TimeWindow:    24 * time.Hour,
            Priority:      10,
        },
        {
            LimitType:     "burst",
            LimitValue:    60,
            TimeWindow:    1 * time.Minute,
            Priority:      20,
        },
    }
}

// Example: Gemini Schema
type GeminiSchema struct{}

func (s *GeminiSchema) GetDefaultLimits() []*RateLimitConfig {
    return []*RateLimitConfig{
        {
            LimitType:     "daily",
            LimitValue:    1500,
            TimeWindow:    24 * time.Hour,
            Priority:      10,
        },
        {
            LimitType:     "burst",
            LimitValue:    60,
            TimeWindow:    1 * time.Minute,
            Priority:      20,
        },
        {
            LimitType:     "token",
            LimitValue:    1000000,
            TimeWindow:    24 * time.Hour,
            Priority:      15,
        },
    }
}
```

---

## Admin Dashboard Integration

### New REST API Endpoints

#### Rate Limit Management

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/rate-limits` | List all rate limits |
| GET | `/api/rate-limits/:id` | Get specific rate limit |
| POST | `/api/rate-limits` | Create new rate limit |
| PUT | `/api/rate-limits/:id` | Update rate limit |
| DELETE | `/api/rate-limits/:id` | Delete rate limit |
| POST | `/api/rate-limits/:id/enable` | Enable rate limit |
| POST | `/api/rate-limits/:id/disable` | Disable rate limit |

#### Usage Monitoring

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/usage` | Get usage statistics |
| GET | `/api/usage/provider/:providerId` | Get provider usage |
| GET | `/api/usage/token/:tokenId` | Get token usage |
| GET | `/api/usage/history` | Get historical usage data |
| GET | `/api/usage/export` | Export usage data (CSV/JSON) |

#### Quota Information

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/quota` | Get overall quota status |
| GET | `/api/quota/provider/:providerId` | Get provider quota |
| GET | `/api/quota/token/:tokenId` | Get token quota |
| GET | `/api/quota/summary` | Get quota summary |

### Dashboard UI Enhancements

#### New Tab: "Rate Limits"

```javascript
// Rate Limits Tab UI Structure
<div class="tab-content" id="rateLimitsTab">
    <!-- Summary Cards -->
    <div class="summary-stats">
        <div class="stat-card">
            <h3>Total Limits</h3>
            <span class="stat-value" id="totalLimits">0</span>
        </div>
        <div class="stat-card">
            <h3>Active Limits</h3>
            <span class="stat-value" id="activeLimits">0</span>
        </div>
        <div class="stat-card">
            <h3>Providers Configured</h3>
            <span class="stat-value" id="configuredProviders">0</span>
        </div>
    </div>
    
    <!-- Rate Limits Table -->
    <div class="table-container">
        <table id="rateLimitsTable">
            <thead>
                <tr>
                    <th>Provider</th>
                    <th>Token</th>
                    <th>Model</th>
                    <th>Limit Type</th>
                    <th>Limit Value</th>
                    <th>Time Window</th>
                    <th>Enabled</th>
                    <th>Actions</th>
                </tr>
            </thead>
            <tbody id="rateLimitsTableBody">
                <!-- Dynamic content -->
            </tbody>
        </table>
    </div>
    
    <!-- Add Rate Limit Button -->
    <button class="btn btn-primary" id="addRateLimitBtn">
        + Add Rate Limit
    </button>
</div>
```

#### New Tab: "Usage"

```javascript
// Usage Tab UI Structure
<div class="tab-content" id="usageTab">
    <!-- Time Range Selector -->
    <div class="filter-bar">
        <select id="timeRange">
            <option value="hour">Last Hour</option>
            <option value="day">Last 24 Hours</option>
            <option value="week">Last 7 Days</option>
            <option value="month">Last 30 Days</option>
        </select>
        <select id="usageProvider">
            <option value="all">All Providers</option>
        </select>
        <select id="usageToken">
            <option value="all">All Tokens</option>
        </select>
        <button class="btn btn-secondary" id="exportUsageBtn">
            📥 Export
        </button>
    </div>
    
    <!-- Usage Charts -->
    <div class="charts-container">
        <div class="chart-card">
            <h3>Requests Over Time</h3>
            <canvas id="requestsChart"></canvas>
        </div>
        <div class="chart-card">
            <h3>Tokens Over Time</h3>
            <canvas id="tokensChart"></canvas>
        </div>
    </div>
    
    <!-- Usage Table -->
    <div class="table-container">
        <table id="usageTable">
            <thead>
                <tr>
                    <th>Provider</th>
                    <th>Token</th>
                    <th>Model</th>
                    <th>Requests</th>
                    <th>Tokens</th>
                    <th>Success Rate</th>
                    <th>Avg Response Time</th>
                </tr>
            </thead>
            <tbody id="usageTableBody">
                <!-- Dynamic content -->
            </tbody>
        </table>
    </div>
</div>
```

#### Enhanced Token Tab with Quota Info

Add quota information to the existing tokens table:

```javascript
// Enhanced token row with quota info
<tr data-token-id="${token.id}">
    <td>${token.provider}</td>
    <td>${token.email}</td>
    <td>${token.healthy ? '✅' : '❌'}</td>
    <td>${token.health_score.toFixed(2)}</td>
    <td>${formatDate(token.expiry_date)}</td>
    <td>${formatDate(token.last_used)}</td>
    <td>${formatDate(token.created_at)}</td>
    <!-- New quota info column -->
    <td>
        <div class="quota-info">
            <div class="quota-bar">
                <div class="quota-fill" style="width: ${quota.dailyPercent}%"></div>
            </div>
            <span class="quota-text">${quota.dailyUsed}/${quota.dailyLimit} daily</span>
        </div>
    </td>
    <td>
        <!-- Actions -->
    </td>
</tr>
```

### API Client Extensions

```javascript
// Extend APIClient with rate limiting methods
class APIClient {
    // ... existing methods ...
    
    // Rate Limits
    async getRateLimits() {
        return this.get('/api/rate-limits');
    }
    
    async createRateLimit(config) {
        return this.post('/api/rate-limits', config);
    }
    
    async updateRateLimit(id, config) {
        return this.put(`/api/rate-limits/${id}`, config);
    }
    
    async deleteRateLimit(id) {
        return this.delete(`/api/rate-limits/${id}`);
    }
    
    // Usage
    async getUsage(params) {
        return this.get('/api/usage', params);
    }
    
    async getUsageHistory(params) {
        return this.get('/api/usage/history', params);
    }
    
    async exportUsage(format = 'csv') {
        return this.get(`/api/usage/export?format=${format}`);
    }
    
    // Quota
    async getQuotaSummary() {
        return this.get('/api/quota/summary');
    }
    
    async getTokenQuota(tokenId) {
        return this.get(`/api/quota/token/${tokenId}`);
    }
}
```

---

## Implementation Roadmap

### Phase 1: Foundation (Week 1-2)

**Goal**: Establish core infrastructure and database schema

| Task | Description | Priority | Dependencies |
|------|-------------|----------|--------------|
| 1.1 | Create `ratelimit` package structure | High | None |
| 1.2 | Design and implement database schema | High | None |
| 1.3 | Create migration scripts for new tables | High | 1.2 |
| 1.4 | Implement `store.Store` interface | High | 1.1 |
| 1.5 | Implement `store.SQLiteStore` | High | 1.3, 1.4 |
| 1.6 | Add rate limit configuration to main config | Medium | 1.1 |
| 1.7 | Write unit tests for storage layer | High | 1.5 |

**Deliverables**:
- Database schema with migrations
- Storage layer with SQLite implementation
- Unit tests for storage

### Phase 2: Core Rate Limiting (Week 2-3)

**Goal**: Implement in-memory rate limiting with caching

| Task | Description | Priority | Dependencies |
|------|-------------|----------|--------------|
| 2.1 | Implement `RateLimiter` interface | High | 1.4 |
| 2.2 | Implement in-memory `RateLimitCache` | High | 2.1 |
| 2.3 | Implement `RateLimitChecker` | High | 2.2 |
| 2.4 | Add periodic cache refresh | Medium | 2.3 |
| 2.5 | Implement provider schema interfaces | High | 2.1 |
| 2.6 | Implement default schemas (Qwen, Gemini) | Medium | 2.5 |
| 2.7 | Write unit tests for rate limiting | High | 2.3 |
| 2.8 | Write integration tests | High | 2.7 |

**Deliverables**:
- Rate limiter with in-memory checking
- Provider schemas for Qwen and Gemini
- Unit and integration tests

### Phase 3: Async Tracking (Week 3-4)

**Goal**: Implement non-blocking usage tracking

| Task | Description | Priority | Dependencies |
|------|-------------|----------|--------------|
| 3.1 | Implement `UsageTracker` with channels | High | 1.5 |
| 3.2 | Implement `WorkerPool` for async writes | High | 3.1 |
| 3.3 | Implement batch write optimization | Medium | 3.2 |
| 3.4 | Add error handling and retries | High | 3.2 |
| 3.5 | Implement graceful shutdown | Medium | 3.2 |
| 3.6 | Write unit tests for worker pool | High | 3.2 |
| 3.7 | Write load tests | Medium | 3.6 |

**Deliverables**:
- Non-blocking usage tracking
- Worker pool with retries
- Performance benchmarks

### Phase 4: Smart Token Selection (Week 4)

**Goal**: Integrate quota-aware token selection

| Task | Description | Priority | Dependencies |
|------|-------------|----------|--------------|
| 4.1 | Implement `QuotaAwareSelectionStrategy` | High | 2.3 |
| 4.2 | Integrate with existing `TokenManager` | High | 4.1 |
| 4.3 | Add quota score calculation | Medium | 4.1 |
| 4.4 | Write unit tests for selection strategy | High | 4.1 |
| 4.5 | Write integration tests with proxy | High | 4.2 |

**Deliverables**:
- Quota-aware token selection
- Integration with existing token manager
- Tests

### Phase 5: Middleware Integration (Week 4-5)

**Goal**: Integrate rate limiting into proxy handlers

| Task | Description | Priority | Dependencies |
|------|-------------|----------|--------------|
| 5.1 | Implement rate limiting middleware | High | 2.3, 3.1 |
| 5.2 | Add middleware to proxy handlers | High | 5.1 |
| 5.3 | Implement usage recording hooks | High | 5.2 |
| 5.4 | Add context propagation | Medium | 5.2 |
| 5.5 | Write integration tests | High | 5.2 |

**Deliverables**:
- Rate limiting middleware
- Integration with proxy handlers
- End-to-end tests

### Phase 6: Admin API (Week 5-6)

**Goal**: Implement REST API for rate limit management

| Task | Description | Priority | Dependencies |
|------|-------------|----------|--------------|
| 6.1 | Implement rate limit CRUD handlers | High | 1.5 |
| 6.2 | Implement usage query handlers | High | 1.5 |
| 6.3 | Implement quota info handlers | High | 2.3 |
| 6.4 | Add API routes to REST server | High | 6.1, 6.2, 6.3 |
| 6.5 | Add authentication/authorization | Medium | 6.4 |
| 6.6 | Write API tests | High | 6.4 |

**Deliverables**:
- REST API for rate limit management
- Usage monitoring endpoints
- API tests

### Phase 7: Dashboard UI (Week 6-7)

**Goal**: Implement admin dashboard UI

| Task | Description | Priority | Dependencies |
|------|-------------|----------|--------------|
| 7.1 | Create Rate Limits tab UI | High | 6.1 |
| 7.2 | Create Usage tab UI | High | 6.2 |
| 7.3 | Add quota info to Tokens tab | Medium | 6.3 |
| 7.4 | Implement charts (using Chart.js) | Medium | 7.2 |
| 7.5 | Add export functionality | Low | 7.2 |
| 7.6 | Write UI tests | Medium | 7.1 |

**Deliverables**:
- Rate Limits dashboard tab
- Usage monitoring dashboard tab
- Enhanced Tokens tab with quota info

### Phase 8: Testing & Optimization (Week 7-8)

**Goal**: Comprehensive testing and performance optimization

| Task | Description | Priority | Dependencies |
|------|-------------|----------|--------------|
| 8.1 | Write end-to-end tests | High | All phases |
| 8.2 | Performance benchmarking | High | All phases |
| 8.3 | Load testing with concurrent requests | High | 8.2 |
| 8.4 | Optimize database queries | Medium | 8.2 |
| 8.5 | Optimize cache refresh strategy | Medium | 8.2 |
| 8.6 | Add monitoring/metrics | Medium | 8.1 |

**Deliverables**:
- Comprehensive test suite
- Performance benchmarks
- Optimization report

### Phase 9: Documentation & Deployment (Week 8)

**Goal**: Complete documentation and prepare for deployment

| Task | Description | Priority | Dependencies |
|------|-------------|----------|--------------|
| 9.1 | Write API documentation | High | 6.4 |
| 9.2 | Write integration guide | High | All phases |
| 9.3 | Write deployment guide | Medium | 9.2 |
| 9.4 | Create migration scripts | High | 1.3 |
| 9.5 | Update main README | Medium | 9.1 |
| 9.6 | Prepare release notes | Low | 9.5 |

**Deliverables**:
- API documentation
- Integration guide
- Deployment guide
- Migration scripts

---

## Integration Guide

### Step 1: Database Migration

Run the migration to create new tables:

```bash
# Apply migration
go run cmd/migrate/main.go --db-path .credentials/tokens.db --version 1

# Or use the existing migration system
# The migration will be automatically applied on startup
```

### Step 2: Update Main Configuration

Add rate limiter configuration to [`config/config.go`](qwencoder-proxy/config/config.go:71):

```go
// RateLimiterConfig holds rate limiting configuration
type RateLimiterConfig struct {
    Enabled              bool          // Enable rate limiting (default: true)
    CacheRefreshInterval time.Duration // Cache refresh interval (default: 30s)
    WorkerPoolSize      int           // Number of worker goroutines (default: 4)
    ChannelBufferSize   int           // Usage channel buffer size (default: 1000)
    BatchFlushInterval  time.Duration // Batch flush interval (default: 100ms)
    MaxBatchSize        int           // Max records per batch (default: 100)
    DropWhenFull        bool          // Drop records when channel is full (default: false)
}

// Config holds all configuration for application
type Config struct {
    Server      ServerConfig
    HTTPClient  HTTPClientConfig
    Logging     LoggingConfig
    OAuthServer OAuthServerConfig
    Storage     StorageConfig
    RateLimiter RateLimiterConfig  // NEW
}
```

Update [`DefaultConfig()`](qwencoder-proxy/config/config.go:81):

```go
func DefaultConfig() *Config {
    return &Config{
        Server: ServerConfig{
            Port: "8143",
        },
        // ... existing config ...
        RateLimiter: RateLimiterConfig{
            Enabled:              true,
            CacheRefreshInterval: 30 * time.Second,
            WorkerPoolSize:      4,
            ChannelBufferSize:   1000,
            BatchFlushInterval:  100 * time.Millisecond,
            MaxBatchSize:        100,
            DropWhenFull:        false,
        },
    }
}
```

### Step 3: Initialize Rate Limiter in Main

Update [`cmd/main.go`](qwencoder-proxy/cmd/main.go:27):

```go
import (
    // ... existing imports ...
    "github.com/sunbankio/qwencoder-proxy/ratelimit"
    "github.com/sunbankio/qwencoder-proxy/ratelimit/provider"
)

func main() {
    // ... existing initialization ...
    
    // Initialize rate limiter
    rateLimiterStore, err := ratelimit.NewSQLiteStore(cfg.Storage.DBPath, logger)
    if err != nil {
        logger.ErrorLog("Failed to create rate limiter store: %v", err)
        os.Exit(1)
    }
    
    // Create provider schema factory
    schemaFactory := provider.NewSchemaFactory()
    
    // Create rate limiter
    rateLimiter := ratelimit.NewRateLimiter(
        rateLimiterStore,
        schemaFactory,
        &ratelimit.Config{
            CacheRefreshInterval: cfg.RateLimiter.CacheRefreshInterval,
            WorkerPoolConfig: &ratelimit.WorkerPoolConfig{
                NumWorkers:      cfg.RateLimiter.WorkerPoolSize,
                QueueSize:       cfg.RateLimiter.ChannelBufferSize,
                MaxRetries:      3,
                RetryDelay:      100 * time.Millisecond,
                ShutdownTimeout: 5 * time.Second,
            },
            BufferConfig: &ratelimit.BufferConfig{
                ChannelSize:    cfg.RateLimiter.ChannelBufferSize,
                FlushInterval:  cfg.RateLimiter.BatchFlushInterval,
                MaxBatchSize:   cfg.RateLimiter.MaxBatchSize,
                DropWhenFull:   cfg.RateLimiter.DropWhenFull,
            },
        },
        logger,
    )
    
    // Start rate limiter
    if err := rateLimiter.Start(); err != nil {
        logger.ErrorLog("Failed to start rate limiter: %v", err)
        os.Exit(1)
    }
    defer rateLimiter.Stop()
    
    // Initialize providers with rate limiter
    if err := initializeProvidersWithRateLimiter(
        providerFactory, 
        multiTokenMgr, 
        rateLimiter,
        logger,
    ); err != nil {
        logger.ErrorLog("Failed to initialize providers: %v", err)
        os.Exit(1)
    }
    
    // ... rest of main ...
}
```

### Step 4: Update Token Manager

Update [`internal/token/token_selection.go`](qwencoder-proxy/internal/token/token_selection.go:136) to support quota-aware selection:

```go
// Add new import
import (
    "github.com/sunbankio/qwencoder-proxy/ratelimit"
)

// Update TokenManager to optionally use rate limiter
type TokenManager struct {
    store              TokenRepository
    strategy           SelectionStrategy
    mu                 sync.RWMutex
    logger             logging.Logger
    clientFactory      ProxyClientFactory
    proxyHealthTracker *ProxyHealthTracker
    rateLimiter        ratelimit.RateLimiter  // NEW
}

// Update NewTokenManager
func NewTokenManager(
    store TokenRepository, 
    strategy SelectionStrategy, 
    logger logging.Logger,
    clientFactory ProxyClientFactory, 
    proxyHealthTracker *ProxyHealthTracker,
    rateLimiter ratelimit.RateLimiter,  // NEW
) *TokenManager {
    return &TokenManager{
        store:              store,
        strategy:           strategy,
        logger:             logger,
        clientFactory:      clientFactory,
        proxyHealthTracker: proxyHealthTracker,
        rateLimiter:        rateLimiter,
    }
}

// Add SetRateLimiter method
func (tm *TokenManager) SetRateLimiter(rl ratelimit.RateLimiter) {
    tm.mu.Lock()
    defer tm.mu.Unlock()
    tm.rateLimiter = rl
    
    // Wrap strategy with quota-aware selection if rate limiter is set
    if rl != nil {
        quotaStrategy := ratelimit.NewQuotaAwareSelectionStrategy(rl, tm.strategy)
        tm.strategy = quotaStrategy
    }
}
```

### Step 5: Add Rate Limiting Middleware

Create new file `proxy/rate_limit_middleware.go`:

```go
package proxy

import (
    "context"
    "net/http"
    "time"
    
    "github.com/sunbankio/qwencoder-proxy/ratelimit"
)

// RateLimitMiddleware creates middleware for rate limiting
func RateLimitMiddleware(rateLimiter ratelimit.RateLimiter) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            // Extract provider and token from context (set by handlers)
            providerID := r.Context().Value("provider_id").(string)
            tokenID := r.Context().Value("token_id").(string)
            model := r.Context().Value("model").(string)
            
            // Check rate limit
            checkReq := &ratelimit.LimitCheckRequest{
                ProviderID: providerID,
                TokenID:    tokenID,
                Model:      model,
            }
            
            result, err := rateLimiter.CheckLimit(r.Context(), checkReq)
            if err != nil {
                // Log error but allow request (fail open)
                next.ServeHTTP(w, r)
                return
            }
            
            if !result.Allowed {
                // Return 429 Too Many Requests
                w.Header().Set("Retry-After", result.RetryAfter.String())
                w.Header().Set("Content-Type", "application/json")
                w.WriteHeader(http.StatusTooManyRequests)
                w.Write([]byte(`{
                    "error": "rate_limit_exceeded",
                    "message": "Rate limit exceeded. Please retry later.",
                    "retry_after": "` + result.RetryAfter.String() + `"
                }`))
                return
            }
            
            // Add quota info to context
            ctx := context.WithValue(r.Context(), "quota_info", result.AvailableQuota)
            
            // Record usage start time
            startTime := time.Now()
            
            // Wrap response writer to capture response
            rw := &responseWriter{ResponseWriter: w}
            
            // Call next handler
            next.ServeHTTP(rw, r.WithContext(ctx))
            
            // Record usage (non-blocking)
            recordUsage(rateLimiter, providerID, tokenID, model, startTime, rw.statusCode)
        })
    }
}

type responseWriter struct {
    http.ResponseWriter
    statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
    rw.statusCode = code
    rw.ResponseWriter.WriteHeader(code)
}

func recordUsage(rateLimiter ratelimit.RateLimiter, providerID, tokenID, model string, startTime time.Time, statusCode int) {
    // Create usage record
    usage := &ratelimit.UsageRecord{
        ProviderID:     providerID,
        TokenID:        tokenID,
        Model:          model,
        RequestCount:   1,
        TokenCount:     0, // TODO: Extract from response if available
        Timestamp:      time.Now(),
        ResponseTimeMs: time.Since(startTime).Milliseconds(),
        Success:        statusCode < 400,
        ErrorCode:      "",
    }
    
    // Record usage (non-blocking)
    _ = rateLimiter.RecordUsage(context.Background(), usage)
}
```

### Step 6: Update REST API Server

Update [`restapi/rest_api.go`](qwencoder-proxy/restapi/rest_api.go:86) to add rate limiting endpoints:

```go
// Add import
import (
    "github.com/sunbankio/qwencoder-proxy/ratelimit"
    "github.com/sunbankio/qwencoder-proxy/ratelimit/api"
)

// Update Server struct
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

### Step 7: Update Provider Initialization

Update the provider initialization to pass rate limiter:

```go
func initializeProvidersWithRateLimiter(
    factory *provider.Factory,
    multiTokenMgr *tokpkg.MultiTokenManager,
    rateLimiter ratelimit.RateLimiter,
    logger logging.Logger,
) error {
    // Get all providers
    providers := factory.GetAllProviders()
    
    for _, p := range providers {
        // Set token manager
        if tmAware, ok := p.(provider.TokenManagerAware); ok {
            tmAware.SetTokenManager(multiTokenMgr.GetTokenManager(p.Name()))
        }
        
        // Set rate limiter (if provider supports it)
        if rlAware, ok := p.(provider.RateLimitAware); ok {
            rlAware.SetRateLimiter(rateLimiter)
        }
    }
    
    return nil
}
```

### Step 8: Build and Test

```bash
# Build the application
make build

# Run tests
make test

# Run with rate limiting enabled
./qwencoder-proxy --port 8143 --debug

# Verify rate limiting is working
curl -H "Content-Type: application/json" \
     -d '{"model":"gpt-4","messages":[{"role":"user","content":"test"}]}' \
     http://localhost:8143/v1/chat/completions
```

### Step 9: Configure Rate Limits via Dashboard

1. Open the dashboard: `http://localhost:8143/dashboard`
2. Navigate to the "Rate Limits" tab
3. Click "+ Add Rate Limit"
4. Configure the limit:
   - Provider: `qwen`
   - Token: (leave blank for provider-wide)
   - Model: (leave blank for all models)
   - Limit Type: `daily`
   - Limit Value: `1000`
   - Time Window: `86400` (seconds)
5. Click "Save"

### Step 10: Monitor Usage

1. Navigate to the "Usage" tab
2. Select time range and filters
3. View charts and tables
4. Export data if needed

---

## Testing Strategy

### Unit Tests

| Component | Test Coverage | Key Test Cases |
|-----------|--------------|----------------|
| Storage Layer | 90%+ | CRUD operations, migrations, error handling |
| Rate Limiter | 90%+ | Limit checks, cache operations, provider schemas |
| Worker Pool | 90%+ | Job processing, retries, shutdown |
| Token Selector | 85%+ | Quota scoring, fallback behavior |
| Middleware | 85%+ | Request blocking, usage recording |

### Integration Tests

| Scenario | Description |
|----------|-------------|
| E2E Rate Limiting | Full request flow with rate limiting |
| Concurrent Requests | Multiple simultaneous requests |
| Cache Refresh | Cache sync with database |
| Worker Pool | Async write processing |
| Dashboard API | CRUD operations via API |

### Performance Tests

| Test | Target | Metric |
|------|--------|--------|
| Rate Limit Check | < 1ms | Check latency |
| Usage Recording | < 100μs | Non-blocking send |
| Worker Throughput | > 10k req/s | Write throughput |
| Cache Refresh | < 100ms | Refresh duration |

### Load Testing

```bash
# Using vegeta for load testing
echo "GET http://localhost:8143/v1/models" | vegeta attack -rate=100 -duration=30s | vegeta report

# Test rate limiting with concurrent requests
vegeta attack -targets=targets.txt -rate=200 -duration=60s | vegeta report
```

---

## Performance Considerations

### Optimization Strategies

1. **In-Memory Caching**: All rate limit checks use in-memory cache
2. **Batch Writes**: Database writes are batched for efficiency
3. **Connection Pooling**: SQLite connection pooling with prepared statements
4. **Index Optimization**: Proper indexes on frequently queried columns
5. **Channel Buffering**: Buffered channels prevent blocking

### Memory Usage

| Component | Estimated Memory |
|-----------|------------------|
| Rate Limit Cache | ~10MB (1000 limits) |
| Usage Cache | ~50MB (10000 entries) |
| Worker Pool | ~5MB (4 workers) |
| Channel Buffer | ~10MB (1000 records) |
| **Total** | ~75MB |

### Database Performance

| Operation | Target Latency |
|-----------|----------------|
| Read limit | < 5ms |
| Write usage | < 10ms |
| Aggregate query | < 50ms |

### Monitoring Metrics

Key metrics to monitor:

- Rate limit check latency
- Usage recording backlog
- Worker pool queue depth
- Database query latency
- Cache hit rate
- Error rates

---

## Appendix

### A. Default Rate Limits

| Provider | Daily Limit | Burst Limit | Token Limit |
|----------|-------------|-------------|-------------|
| Qwen | 1000 | 60/min | N/A |
| Gemini | 1500 | 60/min | 1,000,000 |
| Antigravity | 500 | 30/min | 500,000 |
| Kiro | 2000 | 100/min | 2,000,000 |
| iFlow | 800 | 40/min | 800,000 |

### B. Error Codes

| Code | Description | HTTP Status |
|------|-------------|-------------|
| `rate_limit_exceeded` | Rate limit exceeded | 429 |
| `daily_limit_exceeded` | Daily limit exceeded | 429 |
| `burst_limit_exceeded` | Burst limit exceeded | 429 |
| `token_limit_exceeded` | Token limit exceeded | 429 |
| `invalid_rate_limit_config` | Invalid rate limit configuration | 400 |

### C. Configuration Examples

```yaml
# config.yaml
rate_limiter:
  enabled: true
  cache_refresh_interval: 30s
  worker_pool_size: 4
  channel_buffer_size: 1000
  batch_flush_interval: 100ms
  max_batch_size: 100
  drop_when_full: false
```

### D. API Response Examples

**Rate Limit Check Response:**
```json
{
  "allowed": true,
  "reason": "ok",
  "retry_after": "0s",
  "current_usage": {
    "provider_id": "qwen",
    "token_id": "token-123",
    "model": "qwen-turbo",
    "period": "day",
    "request_count": 450,
    "token_count": 125000,
    "start_time": "2026-03-09T00:00:00Z",
    "end_time": "2026-03-10T00:00:00Z"
  },
  "available_quota": {
    "daily_limit": 1000,
    "daily_used": 450,
    "daily_remaining": 550,
    "burst_limit": 60,
    "burst_used": 15,
    "burst_remaining": 45,
    "token_limit": 0,
    "token_used": 125000,
    "token_remaining": 0
  }
}
```

**Usage Statistics Response:**
```json
{
  "provider_id": "qwen",
  "token_id": "token-123",
  "model": "qwen-turbo",
  "daily_usage": {
    "request_count": 450,
    "token_count": 125000,
    "success_rate": 0.98,
    "avg_response_time_ms": 1250
  },
  "minute_usage": {
    "request_count": 15,
    "token_count": 4200,
    "success_rate": 1.0,
    "avg_response_time_ms": 1180
  }
}
```

---

**Document End**
