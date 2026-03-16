# Phase 1: Foundation - Database Schema and Storage Layer

**Phase Goal:** Establish core infrastructure and database schema for rate limiting and usage tracking.

**Duration:** Week 1-2  
**Status:** Ready to Implement

---

## Task Overview

This phase creates the foundation for the entire rate limiting system by:

1. Creating the new `ratelimit` package structure
2. Designing and implementing the database schema
3. Creating migration scripts for new tables
4. Implementing the storage interface and SQLite implementation
5. Adding rate limiter configuration to main config

---

## Task 1.1: Create `ratelimit` Package Structure

**File:** `qwencoder-proxy/ratelimit/package.go`

Create the package structure:

```
qwencoder-proxy/ratelimit/
├── package.go              # Package documentation
├── errors.go               # Custom error types
├── config.go               # Configuration types
├── types.go                # Core type definitions
└── store/
    ├── store.go            # Storage interface
    └── sqlite_store.go     # SQLite implementation
```

**Implementation Requirements:**

1. Create `package.go` with package documentation
2. Create `errors.go` with custom error types:
   - `ErrRateLimitExceeded`
   - `ErrInvalidRateLimitConfig`
   - `ErrStorageError`
   - `ErrNotFound`
3. Create `config.go` with configuration types:
   - `Config` struct for rate limiter configuration
   - `WorkerPoolConfig` struct
   - `BufferConfig` struct
4. Create `types.go` with core type definitions:
   - `LimitCheckRequest`
   - `LimitCheckResult`
   - `UsageRecord`
   - `UsageStats`
   - `QuotaInfo`
   - `RateLimitConfig`

**Code Template for `errors.go`:**

```go
package ratelimit

import "errors"

var (
    // ErrRateLimitExceeded is returned when a rate limit is exceeded
    ErrRateLimitExceeded = errors.New("rate limit exceeded")

    // ErrInvalidRateLimitConfig is returned when rate limit configuration is invalid
    ErrInvalidRateLimitConfig = errors.New("invalid rate limit configuration")

    // ErrStorageError is returned when a storage operation fails
    ErrStorageError = errors.New("storage error")

    // ErrNotFound is returned when a requested resource is not found
    ErrNotFound = errors.New("not found")

    // ErrInvalidRequest is returned when a request is invalid
    ErrInvalidRequest = errors.New("invalid request")
)
```

**Code Template for `config.go`:**

```go
package ratelimit

import "time"

// Config holds the configuration for the rate limiter
type Config struct {
    // CacheRefreshInterval is the interval at which the in-memory cache is refreshed
    CacheRefreshInterval time.Duration

    // WorkerPoolConfig holds the worker pool configuration
    WorkerPoolConfig *WorkerPoolConfig

    // BufferConfig holds the buffer configuration for usage tracking
    BufferConfig *BufferConfig
}

// WorkerPoolConfig holds the configuration for the worker pool
type WorkerPoolConfig struct {
    // NumWorkers is the number of worker goroutines
    NumWorkers int

    // QueueSize is the internal queue size for each worker
    QueueSize int

    // MaxRetries is the maximum number of retries for failed writes
    MaxRetries int

    // RetryDelay is the delay between retries
    RetryDelay time.Duration

    // ShutdownTimeout is the graceful shutdown timeout
    ShutdownTimeout time.Duration
}

// BufferConfig holds the configuration for the usage tracking buffer
type BufferConfig struct {
    // ChannelSize is the size of the buffered channel for usage records
    ChannelSize int

    // FlushInterval is the interval at which the buffer is flushed
    FlushInterval time.Duration

    // MaxBatchSize is the maximum number of records per batch
    MaxBatchSize int

    // DropWhenFull determines whether to drop records when the channel is full
    DropWhenFull bool
}

// DefaultConfig returns the default configuration
func DefaultConfig() *Config {
    return &Config{
        CacheRefreshInterval: 30 * time.Second,
        WorkerPoolConfig: &WorkerPoolConfig{
            NumWorkers:      4,
            QueueSize:       500,
            MaxRetries:      3,
            RetryDelay:      100 * time.Millisecond,
            ShutdownTimeout: 5 * time.Second,
        },
        BufferConfig: &BufferConfig{
            ChannelSize:   1000,
            FlushInterval: 100 * time.Millisecond,
            MaxBatchSize:   100,
            DropWhenFull:   false,
        },
    }
}
```

**Code Template for `types.go`:**

```go
package ratelimit

import "time"

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

// TimeWindow represents a time window type
type TimeWindow string

const (
    TimeWindowDay     TimeWindow = "day"
    TimeWindowMinute  TimeWindow = "minute"
    TimeWindowSecond  TimeWindow = "second"
)

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

// RateLimitConfig represents a rate limit configuration
type RateLimitConfig struct {
    ID              string
    ProviderID      string
    TokenID         string // nil = provider-wide limit
    Model           string // nil = all models
    LimitType       string // "daily", "burst", "token"
    LimitValue      int64
    TimeWindow      time.Duration
    Enabled         bool
    Priority        int
    CreatedAt       time.Time
    UpdatedAt       time.Time
}
```

---

## Task 1.2: Design and Implement Database Schema

**File:** `qwencoder-proxy/ratelimit/store/schema.go`

Create the database schema for rate limiting and usage tracking.

### Table 1: `rate_limits`

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

### Table 2: `usage_tracking`

```sql
CREATE TABLE IF NOT EXISTS usage_tracking (
    id TEXT PRIMARY KEY,
    provider_id TEXT NOT NULL,
    token_id TEXT NOT NULL,
    model TEXT NOT NULL,
    window_type TEXT NOT NULL,        -- "day", "minute", "second"
    window_start INTEGER NOT NULL,     -- Start of time window (ms timestamp)
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

### Table 3: `usage_history`

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

**Implementation Requirements:**

1. Create `schema.go` with all table creation statements as constants
2. Include all indexes for optimal query performance
3. Add foreign key constraints for data integrity
4. Include proper default values and constraints

---

## Task 1.3: Create Migration Scripts

**File:** `qwencoder-proxy/ratelimit/store/migrations.go`

Create migration scripts for the new tables.

**Implementation Requirements:**

```go
package store

const (
    // Migration 001: Add rate limiting and usage tracking tables
    Migration001 = `
-- Create rate_limits table
CREATE TABLE IF NOT EXISTS rate_limits (
    id TEXT PRIMARY KEY,
    provider_id TEXT NOT NULL,
    token_id TEXT,
    model TEXT,
    limit_type TEXT NOT NULL,
    limit_value INTEGER NOT NULL,
    time_window_seconds INTEGER,
    enabled INTEGER NOT NULL DEFAULT 1,
    priority INTEGER NOT NULL DEFAULT 0,
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

-- Create usage_tracking table
CREATE TABLE IF NOT EXISTS usage_tracking (
    id TEXT PRIMARY KEY,
    provider_id TEXT NOT NULL,
    token_id TEXT NOT NULL,
    model TEXT NOT NULL,
    window_type TEXT NOT NULL,
    window_start INTEGER NOT NULL,
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

-- Create usage_history table
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
    timestamp INTEGER NOT NULL,
    FOREIGN KEY (provider_id) REFERENCES providers(id) ON DELETE CASCADE,
    FOREIGN KEY (token_id) REFERENCES tokens(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_usage_history_timestamp 
    ON usage_history(timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_usage_history_provider 
    ON usage_history(provider_id, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_usage_history_token 
    ON usage_history(token_id, timestamp DESC);

-- Insert migration record
INSERT INTO schema_migrations (version, applied_at, description)
VALUES (1, strftime('%s', 'subsec') * 1000, 'Add rate limiting and usage tracking tables');
`
)

// GetMigrations returns all migrations
func GetMigrations() map[int]string {
    return map[int]string{
        1: Migration001,
    }
}
```

---

## Task 1.4: Implement `store.Store` Interface

**File:** `qwencoder-proxy/ratelimit/store/store.go`

Create the storage interface for rate limiting operations.

**Implementation Requirements:**

```go
package store

import (
    "context"
    "time"
    
    "github.com/sunbankio/qwencoder-proxy/ratelimit"
)

// Store defines the interface for rate limiting storage operations
type Store interface {
    // Rate limit operations
    CreateRateLimit(ctx context.Context, config *ratelimit.RateLimitConfig) error
    GetRateLimit(ctx context.Context, id string) (*ratelimit.RateLimitConfig, error)
    GetRateLimits(ctx context.Context, filter *RateLimitFilter) ([]*ratelimit.RateLimitConfig, error)
    UpdateRateLimit(ctx context.Context, id string, config *ratelimit.RateLimitConfig) error
    DeleteRateLimit(ctx context.Context, id string) error
    EnableRateLimit(ctx context.Context, id string) error
    DisableRateLimit(ctx context.Context, id string) error
    
    // Usage tracking operations
    WriteUsageRecord(ctx context.Context, record *ratelimit.UsageRecord) error
    WriteUsageRecords(ctx context.Context, records []*ratelimit.UsageRecord) error
    GetUsage(ctx context.Context, query *UsageQuery) (*ratelimit.UsageStats, error)
    GetUsageHistory(ctx context.Context, query *HistoryQuery) ([]*ratelimit.UsageRecord, error)
    GetCurrentUsage(ctx context.Context, providerID, tokenID, model string) (map[timeWindow]*ratelimit.UsageStats, error)
    
    // Cleanup operations
    CleanupOldUsage(ctx context.Context, olderThan time.Time) (int64, error)
    CleanupOldHistory(ctx context.Context, olderThan time.Time) (int64, error)
    
    // Migration operations
    RunMigrations(ctx context.Context) error
    Close() error
}

// RateLimitFilter defines filters for querying rate limits
type RateLimitFilter struct {
    ProviderID string
    TokenID    string
    Model      string
    LimitType  string
    Enabled    *bool
}

// UsageQuery defines a query for usage statistics
type UsageQuery struct {
    ProviderID string
    TokenID    string
    Model      string
    Window     timeWindow
    StartTime  time.Time
    EndTime    time.Time
}

// HistoryQuery defines a query for usage history
type HistoryQuery struct {
    ProviderID string
    TokenID    string
    Model      string
    StartTime  time.Time
    EndTime    time.Time
    Limit      int
    Offset     int
}

type timeWindow string

const (
    timeWindowDay     timeWindow = "day"
    timeWindowMinute  timeWindow = "minute"
    timeWindowSecond  timeWindow = "second"
)
```

---

## Task 1.5: Implement `store.SQLiteStore`

**File:** `qwencoder-proxy/ratelimit/store/sqlite_store.go`

Implement the SQLite storage for rate limiting operations.

**Implementation Requirements:**

1. Create `SQLiteStore` struct with database connection
2. Implement all methods from `Store` interface
3. Use prepared statements for performance
4. Apply SQLite pragmas for optimal configuration
5. Handle errors properly with context

**Code Template:**

```go
package store

import (
    "context"
    "database/sql"
    "fmt"
    "sync"
    "time"
    
    "github.com/sunbankio/qwencoder-proxy/logging"
    "github.com/sunbankio/qwencoder-proxy/ratelimit"
    _ "modernc.org/sqlite"
)

// SQLiteStore implements the Store interface using SQLite
type SQLiteStore struct {
    db     *sql.DB
    logger logging.Logger
    mu     sync.RWMutex
    
    // Prepared statements
    stmtInsertRateLimit *sql.Stmt
    stmtGetRateLimit    *sql.Stmt
    stmtGetRateLimits   *sql.Stmt
    stmtUpdateRateLimit *sql.Stmt
    stmtDeleteRateLimit *sql.Stmt
    stmtWriteUsage      *sql.Stmt
    stmtGetUsage        *sql.Stmt
    stmtGetCurrentUsage  *sql.Stmt
}

// NewSQLiteStore creates a new SQLite store
func NewSQLiteStore(dbPath string, logger logging.Logger) (*SQLiteStore, error) {
    db, err := sql.Open("sqlite", dbPath)
    if err != nil {
        return nil, fmt.Errorf("failed to open database: %w", err)
    }
    
    // Apply pragmas
    if err := applyPragmas(db, logger); err != nil {
        db.Close()
        return nil, fmt.Errorf("failed to apply pragmas: %w", err)
    }
    
    store := &SQLiteStore{
        db:     db,
        logger: logger,
    }
    
    // Run migrations
    if err := store.RunMigrations(context.Background()); err != nil {
        db.Close()
        return nil, fmt.Errorf("failed to run migrations: %w", err)
    }
    
    // Prepare statements
    if err := store.prepareStatements(); err != nil {
        db.Close()
        return nil, fmt.Errorf("failed to prepare statements: %w", err)
    }
    
    return store, nil
}

func applyPragmas(db *sql.DB, logger logging.Logger) error {
    pragmas := []struct {
        name  string
        value string
    }{
        {"journal_mode", "WAL"},
        {"synchronous", "NORMAL"},
        {"cache_size", "-10240"},
        {"foreign_keys", "ON"},
        {"busy_timeout", "5000"},
        {"temp_store", "2"},
    }
    
    for _, pragma := range pragmas {
        _, err := db.Exec(fmt.Sprintf("PRAGMA %s = %s", pragma.name, pragma.value))
        if err != nil {
            return fmt.Errorf("failed to set pragma %s: %w", pragma.name, err)
        }
    }
    
    return nil
}

func (s *SQLiteStore) prepareStatements() error {
    // Prepare statements for rate limits
    var err error
    s.stmtInsertRateLimit, err = s.db.Prepare(`
        INSERT INTO rate_limits (id, provider_id, token_id, model, limit_type, 
            limit_value, time_window_seconds, enabled, priority, created_at, updated_at)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
    `)
    if err != nil {
        return fmt.Errorf("failed to prepare insert rate limit statement: %w", err)
    }
    
    // Prepare other statements...
    
    return nil
}

// Implement all Store interface methods...
// CreateRateLimit, GetRateLimit, GetRateLimits, UpdateRateLimit, DeleteRateLimit, etc.
```

---

## Task 1.6: Add Rate Limiter Configuration to Main Config

**File:** `qwencoder-proxy/config/config.go`

Add rate limiter configuration to the main configuration.

**Implementation Requirements:**

1. Add `RateLimiterConfig` struct
2. Add `RateLimiterConfig` field to `Config` struct
3. Update `DefaultConfig()` to include default rate limiter configuration

**Code to Add:**

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

// Update DefaultConfig()
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
        Storage: StorageConfig{
            DBPath: DefaultStoragePath,
        },
        RateLimiter: RateLimiterConfig{  // NEW
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

---

## Task 1.7: Write Unit Tests for Storage Layer

**File:** `qwencoder-proxy/ratelimit/store/sqlite_store_test.go`

Write comprehensive unit tests for the storage layer.

**Test Cases to Implement:**

1. **Rate Limit CRUD Operations:**
   - Test creating a rate limit
   - Test getting a rate limit by ID
   - Test getting rate limits with filters
   - Test updating a rate limit
   - Test deleting a rate limit
   - Test enabling/disabling rate limits

2. **Usage Tracking Operations:**
   - Test writing a usage record
   - Test writing multiple usage records in batch
   - Test getting usage statistics
   - Test getting current usage for multiple windows
   - Test getting usage history

3. **Cleanup Operations:**
   - Test cleaning up old usage records
   - Test cleaning up old history records

4. **Migration Operations:**
   - Test running migrations
   - Test idempotent migrations

**Test Template:**

```go
package store

import (
    "context"
    "testing"
    "time"
    
    "github.com/sunbankio/qwencoder-proxy/logging"
    "github.com/sunbankio/qwencoder-proxy/ratelimit"
)

func setupTestDB(t *testing.T) *SQLiteStore {
    db, err := NewSQLiteStore(":memory:", logging.NewLogger())
    if err != nil {
        t.Fatalf("Failed to create test database: %v", err)
    }
    return db
}

func TestCreateRateLimit(t *testing.T) {
    store := setupTestDB(t)
    defer store.Close()
    
    config := &ratelimit.RateLimitConfig{
        ID:         "test-limit-1",
        ProviderID: "qwen",
        TokenID:    "token-123",
        Model:      "qwen-turbo",
        LimitType:  "daily",
        LimitValue: 1000,
        TimeWindow: 24 * time.Hour,
        Enabled:    true,
        Priority:   10,
        CreatedAt:  time.Now(),
        UpdatedAt:  time.Now(),
    }
    
    err := store.CreateRateLimit(context.Background(), config)
    if err != nil {
        t.Fatalf("Failed to create rate limit: %v", err)
    }
    
    // Verify the rate limit was created
    retrieved, err := store.GetRateLimit(context.Background(), config.ID)
    if err != nil {
        t.Fatalf("Failed to get rate limit: %v", err)
    }
    
    if retrieved.ProviderID != config.ProviderID {
        t.Errorf("Expected provider ID %s, got %s", config.ProviderID, retrieved.ProviderID)
    }
    
    // Add more assertions...
}

// Implement more tests...
```

---

## Deliverables

After completing this phase, you should have:

1. ✅ Complete `ratelimit` package structure with:
   - `package.go`
   - `errors.go`
   - `config.go`
   - `types.go`

2. ✅ Database schema with:
   - `rate_limits` table
   - `usage_tracking` table
   - `usage_history` table
   - All required indexes

3. ✅ Migration scripts for new tables

4. ✅ `Store` interface defined

5. ✅ `SQLiteStore` implementation with all methods

6. ✅ Rate limiter configuration added to main config

7. ✅ Unit tests for storage layer with >90% coverage

---

## Success Criteria

- [ ] All package files created with proper Go documentation
- [ ] Database schema is valid and can be created
- [ ] Migrations run successfully
- [ ] All Store interface methods are implemented
- [ ] SQLiteStore passes all unit tests
- [ ] Configuration can be loaded and used
- [ ] Code follows Go idioms and best practices
- [ ] All error cases are properly handled

---

## Next Phase

After completing Phase 1, proceed to **Phase 2: Core Rate Limiting** which implements the in-memory rate limiting with caching.

**File:** `../todo/phase-2-core-rate-limiting.md`
