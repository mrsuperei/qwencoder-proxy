# Comprehensive SQLite Migration Plan
## File-Based to SQLite Token Storage Migration

**Document Version:** 1.0  
**Date:** 2026-02-26  
**Author:** Senior Software Architect  
**Project:** qwencoder-proxy

---

## Table of Contents

1. [Executive Summary](#executive-summary)
2. [Current Architecture Overview](#current-architecture-overview)
3. [Phase 1: Database Schema Design and Architecture](#phase-1-database-schema-design-and-architecture)
4. [Phase 2: Implementation of the SQLite Data Access Layer](#phase-2-implementation-of-the-sqlite-data-access-layer)
5. [Phase 3: Data Migration Strategy](#phase-3-data-migration-strategy)
6. [Phase 4: Refactoring and Integration](#phase-4-refactoring-and-integration)
7. [Phase 5: Comprehensive Testing](#phase-5-comprehensive-testing)
8. [Phase 6: Cleanup and Removal](#phase-6-cleanup-and-removal)
9. [Implementation Roadmap](#implementation-roadmap)
10. [Risk Assessment and Mitigation](#risk-assessment-and-mitigation)
11. [Rollback Strategy](#rollback-strategy)

---

## Executive Summary

This document provides a comprehensive, multi-phase migration plan to transition the qwencoder-proxy application's token storage system from a file-based approach to SQLite as the primary and default backend. The migration ensures:

- **Zero Data Loss**: All existing tokens are preserved during migration
- **Backward Compatibility**: Graceful fallback to file-based storage if needed
- **Performance Improvement**: SQLite provides better query performance and concurrent access
- **Data Integrity**: ACID transactions ensure consistent token state
- **Scalability**: Single database file for all providers with efficient indexing

### Key Benefits

| Aspect | File-Based | SQLite |
|--------|------------|--------|
| **Concurrent Access** | File locks (limited) | WAL mode (full concurrency) |
| **Query Performance** | Linear scan | Indexed queries |
| **Data Integrity** | No transactions | ACID compliant |
| **Scalability** | Many small files | Single database |
| **Backup** | Copy directory | Single file backup |
| **Migration** | Manual file operations | Built-in migration system |

---

## Current Architecture Overview

### File-Based Storage Architecture

The current implementation uses file-based JSON storage with the following structure:

```
.credentials/
├── gemini/
│   ├── {sanitized_email}.json    # Individual token files
│   ├── settings.json             # Provider settings
│   └── *.lock                    # File locks
├── qwen/
│   ├── {sanitized_email}.json
│   └── settings.json
└── ...
```

### Key Components

| Component | File | Description |
|-----------|------|-------------|
| [`TokenStore`](qwencoder-proxy/internal/token/store.go:8-22) | [`store.go`](qwencoder-proxy/internal/token/store.go) | Storage interface |
| [`MultiTokenStore`](qwencoder-proxy/internal/token/multi_token_store.go:45-54) | [`multi_token_store.go`](qwencoder-proxy/internal/token/multi_token_store.go) | File-based implementation |
| [`SQLiteStore`](qwencoder-proxy/internal/token/sqlite_store.go:85-101) | [`sqlite_store.go`](qwencoder-proxy/internal/token/sqlite_store.go) | SQLite implementation |
| [`Migrator`](qwencoder-proxy/internal/token/migrator.go:13-18) | [`migrator.go`](qwencoder-proxy/internal/token/migrator.go) | Migration utility |
| [`StoreFactory`](qwencoder-proxy/internal/token/store_factory.go:21-33) | [`store_factory.go`](qwencoder-proxy/internal/token/store_factory.go) | Factory pattern |

### Data Structures

```go
// ProviderToken represents a single stored token
type ProviderToken struct {
    ID               string       `json:"id"`
    AccessToken      string       `json:"access_token"`
    RefreshToken     string       `json:"refresh_token"`
    TokenType        string       `json:"token_type"`
    ExpiryDate       int64        `json:"expiry_date"`
    Email            string       `json:"email"`
    ResourceURL      string       `json:"resource_url,omitempty"`
    Scope            string       `json:"scope,omitempty"`
    APIKey           string       `json:"api_key,omitempty"`
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

---

## Phase 1: Database Schema Design and Architecture

### 1.1 Schema Overview

The SQLite schema is already designed and implemented in [`sqlite_store.go`](qwencoder-proxy/internal/token/sqlite_store.go). This phase documents and validates the schema design.

### 1.2 Table Definitions

#### 1.2.1 tokens Table

**Purpose:** Stores OAuth tokens with all metadata for each provider.

```sql
CREATE TABLE IF NOT EXISTS tokens (
    -- Primary Key
    id TEXT PRIMARY KEY,
    
    -- Provider Identification
    provider_id TEXT NOT NULL,
    
    -- OAuth Token Data
    access_token TEXT NOT NULL,
    refresh_token TEXT,
    token_type TEXT NOT NULL DEFAULT 'Bearer',
    expiry_date INTEGER NOT NULL,
    
    -- Provider-Specific Fields
    email TEXT,
    resource_url TEXT,
    scope TEXT,
    api_key TEXT,
    
    -- Health and Usage Tracking
    healthy INTEGER NOT NULL DEFAULT 1,
    health_score REAL NOT NULL DEFAULT 1.0,
    last_used INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL DEFAULT (strftime('%s', 'subsec') * 1000),
    error_count INTEGER NOT NULL DEFAULT 0,
    last_error TEXT,
    
    -- Proxy Configuration (Foreign Key)
    proxy_id TEXT,
    
    -- Timestamps
    updated_at INTEGER NOT NULL DEFAULT (strftime('%s', 'subsec') * 1000),
    
    -- Constraints
    FOREIGN KEY (proxy_id) REFERENCES proxy_configs(id) ON DELETE SET NULL
);
```

#### Field Specifications

| Field | Type | Constraints | Description |
|-------|------|-------------|-------------|
| `id` | TEXT | PRIMARY KEY | UUID v4, unique token identifier |
| `provider_id` | TEXT | NOT NULL | Provider identifier (e.g., "gemini-cli") |
| `access_token` | TEXT | NOT NULL | OAuth access token |
| `refresh_token` | TEXT | NULLABLE | OAuth refresh token |
| `token_type` | TEXT | NOT NULL, DEFAULT 'Bearer' | Token type |
| `expiry_date` | INTEGER | NOT NULL | Expiry timestamp (Unix milliseconds) |
| `email` | TEXT | NULLABLE | User email extracted from token |
| `resource_url` | TEXT | NULLABLE | Resource URL (Qwen-specific) |
| `scope` | TEXT | NULLABLE | OAuth scope (Gemini-specific) |
| `api_key` | TEXT | NULLABLE | API key (iFlow-specific) |
| `healthy` | INTEGER | NOT NULL, DEFAULT 1 | 0=false, 1=true |
| `health_score` | REAL | NOT NULL, DEFAULT 1.0 | Health score (0.0-1.0) |
| `last_used` | INTEGER | NOT NULL, DEFAULT 0 | Last usage timestamp |
| `created_at` | INTEGER | NOT NULL | Creation timestamp (auto) |
| `error_count` | INTEGER | NOT NULL, DEFAULT 0 | Consecutive error count |
| `last_error` | TEXT | NULLABLE | Last error message |
| `proxy_id` | TEXT | NULLABLE, FK | Foreign key to proxy_configs |
| `updated_at` | INTEGER | NOT NULL | Last update timestamp (auto) |

#### 1.2.2 provider_settings Table

**Purpose:** Stores provider-specific settings like token selection strategy and refresh parameters.

```sql
CREATE TABLE IF NOT EXISTS provider_settings (
    provider_id TEXT PRIMARY KEY,
    selection_strategy TEXT NOT NULL DEFAULT 'random',
    refresh_buffer_sec INTEGER NOT NULL DEFAULT 1800,
    max_error_count INTEGER NOT NULL DEFAULT 3,
    updated_at INTEGER NOT NULL DEFAULT (strftime('%s', 'subsec') * 1000)
);
```

#### Field Specifications

| Field | Type | Constraints | Description |
|-------|------|-------------|-------------|
| `provider_id` | TEXT | PRIMARY KEY | Provider identifier |
| `selection_strategy` | TEXT | NOT NULL, DEFAULT 'random' | "random", "round_robin", "least_used" |
| `refresh_buffer_sec` | INTEGER | NOT NULL, DEFAULT 1800 | Seconds before expiry to refresh |
| `max_error_count` | INTEGER | NOT NULL, DEFAULT 3 | Max errors before marking unhealthy |
| `updated_at` | INTEGER | NOT NULL | Last update timestamp |

#### 1.2.3 proxy_configs Table

**Purpose:** Normalized proxy configurations to avoid duplication and enable sharing.

```sql
CREATE TABLE IF NOT EXISTS proxy_configs (
    id TEXT PRIMARY KEY,
    host TEXT NOT NULL,
    port INTEGER NOT NULL,
    username TEXT,
    password TEXT,
    created_at INTEGER NOT NULL DEFAULT (strftime('%s', 'subsec') * 1000)
);
```

#### Field Specifications

| Field | Type | Constraints | Description |
|-------|------|-------------|-------------|
| `id` | TEXT | PRIMARY KEY | UUID v4 |
| `host` | TEXT | NOT NULL | Proxy host |
| `port` | INTEGER | NOT NULL | Proxy port |
| `username` | TEXT | NULLABLE | Proxy username |
| `password` | TEXT | NULLABLE | Proxy password |
| `created_at` | INTEGER | NOT NULL | Creation timestamp |

#### 1.2.4 schema_migrations Table

**Purpose:** Tracks database schema migrations.

```sql
CREATE TABLE IF NOT EXISTS schema_migrations (
    version INTEGER PRIMARY KEY,
    applied_at INTEGER NOT NULL DEFAULT (strftime('%s', 'subsec') * 1000),
    description TEXT
);
```

### 1.3 Index Design

#### Index Strategy

Indexes are created to optimize common query patterns:

| Index Name | Fields | Purpose | Query Pattern |
|------------|--------|---------|---------------|
| `idx_tokens_provider_id` | `provider_id` | Filter by provider | `WHERE provider_id = ?` |
| `idx_tokens_email` | `email` | Filter by email | `WHERE email = ?` |
| `idx_tokens_expiry_date` | `expiry_date` | Filter by expiry | `WHERE expiry_date > ?` |
| `idx_tokens_healthy` | `healthy` | Filter by health | `WHERE healthy = 1` |
| `idx_tokens_provider_healthy` | `provider_id, healthy` | Composite filter | `WHERE provider_id = ? AND healthy = ?` |
| `idx_tokens_provider_expiry` | `provider_id, expiry_date` | Composite filter | `WHERE provider_id = ? AND expiry_date > ?` |

#### Index Creation SQL

```sql
CREATE INDEX IF NOT EXISTS idx_tokens_provider_id ON tokens(provider_id);
CREATE INDEX IF NOT EXISTS idx_tokens_email ON tokens(email);
CREATE INDEX IF NOT EXISTS idx_tokens_expiry_date ON tokens(expiry_date);
CREATE INDEX IF NOT EXISTS idx_tokens_healthy ON tokens(healthy);
CREATE INDEX IF NOT EXISTS idx_tokens_provider_healthy ON tokens(provider_id, healthy);
CREATE INDEX IF NOT EXISTS idx_tokens_provider_expiry ON tokens(provider_id, expiry_date);
```

### 1.4 Database Pragmas

The following SQLite pragmas are applied for optimal performance and safety:

| Pragma | Value | Purpose |
|--------|-------|---------|
| `journal_mode` | WAL | Enable Write-Ahead Logging for concurrent reads/writes |
| `synchronous` | NORMAL | Balance between safety and performance |
| `cache_size` | -10240 | Increase cache to 10MB (default 2MB) |
| `foreign_keys` | ON | Enable foreign key constraints |
| `busy_timeout` | 5000 | Wait 5 seconds if database is locked |
| `temp_store` | 2 | Use memory for temporary tables |

### 1.5 Data Type Mapping

| Go Type | SQLite Type | Notes |
|---------|-------------|-------|
| `string` | TEXT | All string fields |
| `int` | INTEGER | All integer fields |
| `int64` | INTEGER | Timestamps (Unix milliseconds) |
| `float64` | REAL | Health scores |
| `bool` | INTEGER | 0=false, 1=true |
| `*ProxyConfig` | TEXT (FK) | Foreign key reference |

### 1.6 Schema Versioning

The schema uses a versioning system to support future migrations:

```go
type migration struct {
    version     int
    description string
    fn          func() error
}
```

Current version: **1**  
Migration tracking: `schema_migrations` table

---

## Phase 2: Implementation of the SQLite Data Access Layer

### 2.1 Architecture Overview

The SQLite Data Access Layer is already implemented in [`sqlite_store.go`](qwencoder-proxy/internal/token/sqlite_store.go). This phase documents the implementation and identifies any enhancements needed.

### 2.2 Core Components

#### 2.2.1 SQLiteStore Structure

```go
type SQLiteStore struct {
    db         *sql.DB
    providerID string
    logger     logging.Logger
    mu         sync.RWMutex

    // Prepared statements for performance
    stmtInsertToken            *sql.Stmt
    stmtUpdateToken            *sql.Stmt
    stmtSelectTokenByID        *sql.Stmt
    stmtSelectTokensByProvider *sql.Stmt
    stmtSelectValidTokens      *sql.Stmt
    stmtDeleteToken            *sql.Stmt
    stmtSelectSettings         *sql.Stmt
    stmtUpsertSettings         *sql.Stmt
}
```

#### 2.2.2 Connection Management

**Initialization Function:**

```go
func NewSQLiteStore(dbPath, providerID string, logger logging.Logger) (*SQLiteStore, error)
```

**Connection Pool Configuration:**

| Parameter | Value | Purpose |
|-----------|-------|---------|
| `SetMaxOpenConns` | 25 | Maximum open connections |
| `SetMaxIdleConns` | 5 | Maximum idle connections |
| `SetConnMaxLifetime` | 5 minutes | Maximum connection reuse time |
| `SetConnMaxIdleTime` | 1 minute | Maximum idle connection time |

#### 2.2.3 Prepared Statements

The following prepared statements are used for performance:

| Statement | Purpose |
|-----------|---------|
| `stmtInsertToken` | Insert new token |
| `stmtUpdateToken` | Update existing token |
| `stmtSelectTokenByID` | Select token by ID |
| `stmtSelectTokensByProvider` | Select all tokens for provider |
| `stmtSelectValidTokens` | Select healthy, non-expired tokens |
| `stmtDeleteToken` | Delete token by ID |
| `stmtSelectSettings` | Select provider settings |
| `stmtUpsertSettings` | Insert or update provider settings |

### 2.3 CRUD Operations

#### 2.3.1 Load Operation

```go
func (s *SQLiteStore) Load() (map[string]TokenMetadata, error)
```

**Purpose:** Load all tokens for a provider from SQLite.

**Implementation Details:**
- Uses prepared statement `stmtSelectTokensByProvider`
- Returns tokens as map keyed by token ID
- Handles scanning errors gracefully
- Logs debug information

#### 2.3.2 Save Operation

```go
func (s *SQLiteStore) Save(tokens map[string]TokenMetadata) error
```

**Purpose:** Persist all tokens to SQLite using a transaction.

**Implementation Details:**
- Uses transaction for atomicity
- Deletes existing tokens for provider first
- Inserts new tokens in batch
- Rolls back on error
- Commits only on success

#### 2.3.3 AddToken Operation

```go
func (s *SQLiteStore) AddToken(token TokenMetadata) error
```

**Purpose:** Add a new token to the store.

**Implementation Details:**
- Checks for duplicate by `refresh_token`
- Updates existing token if duplicate found
- Inserts new token otherwise

#### 2.3.4 UpdateToken Operation

```go
func (s *SQLiteStore) UpdateToken(tokenID string, updateFunc func(*TokenMetadata)) error
```

**Purpose:** Update an existing token using a callback function.

**Implementation Details:**
- Loads token by ID
- Applies update function
- Saves updated token
- Handles token not found

#### 2.3.5 DeleteToken Operation

```go
func (s *SQLiteStore) DeleteToken(tokenID string) error
```

**Purpose:** Remove a token from the store.

**Implementation Details:**
- Uses prepared statement `stmtDeleteToken`
- Filters by provider_id and token ID

### 2.4 Transaction Management

#### Transaction Pattern

```go
func (s *SQLiteStore) Save(tokens map[string]TokenMetadata) error {
    s.mu.Lock()
    defer s.mu.Unlock()

    // Start transaction
    tx, err := s.db.Begin()
    if err != nil {
        return fmt.Errorf("failed to begin transaction: %w", err)
    }
    defer tx.Rollback()

    // ... transaction operations ...

    // Commit transaction
    if err := tx.Commit(); err != nil {
        return fmt.Errorf("failed to commit transaction: %w", err)
    }

    return nil
}
```

#### Transaction Guarantees

- **Atomicity**: All operations succeed or none succeed
- **Consistency**: Database remains in valid state
- **Isolation**: Concurrent transactions don't interfere
- **Durability**: Committed changes persist

### 2.5 Error Handling Strategy

#### Error Types

| Error Type | Handling | Recovery |
|------------|----------|----------|
| `sql.ErrNoRows` | Expected for missing data | Return empty result |
| Connection errors | Log and retry | Reconnect |
| Constraint violations | Log and skip | Continue processing |
| Transaction errors | Rollback | Return error to caller |

#### Error Wrapping Pattern

```go
if err != nil {
    return fmt.Errorf("context: %w", err)
}
```

### 2.6 Helper Functions

#### Type Conversion Functions

```go
// boolToInt converts boolean to SQLite INTEGER
func boolToInt(b bool) int

// intToBool converts SQLite INTEGER to boolean
func intToBool(i int) bool

// sqlNullString converts string to sql.NullString
func sqlNullString(s string) sql.NullString

// isSQLNoRows checks if error is sql.ErrNoRows
func isSQLNoRows(err error) bool
```

#### Scanning Functions

```go
// scanToken scans a token from a database row
func (s *SQLiteStore) scanToken(scanner interface{ Scan(...interface{}) error }) (TokenMetadata, error)

// bindToken binds token parameters to a prepared statement
func (s *SQLiteStore) bindToken(stmt *sql.Stmt, token TokenMetadata) error
```

### 2.7 Enhancements Needed

The following enhancements should be considered:

1. **Connection Health Checks**: Periodic ping to verify connection
2. **Statement Revalidation**: Re-prepare statements if connection is lost
3. **Query Timeout**: Add context with timeout for long-running queries
4. **Metrics Collection**: Track query performance and connection pool stats
5. **Connection Pool Monitoring**: Log when connections are exhausted

---

## Phase 3: Data Migration Strategy

### 3.1 Migration Overview

The migration strategy transfers all existing tokens from file-based storage to SQLite without data loss. The [`Migrator`](qwencoder-proxy/internal/token/migrator.go:13-18) component in [`migrator.go`](qwencoder-proxy/internal/token/migrator.go) handles this process.

### 3.2 Migration Architecture

```
┌─────────────────┐
│  File Store     │
│  (Source)       │
└────────┬────────┘
         │
         │ 1. Load tokens
         │
         ▼
┌─────────────────┐
│   Migrator      │
│  - Validate     │
│  - Transform    │
│  - Deduplicate  │
└────────┬────────┘
         │
         │ 2. Insert tokens
         │
         ▼
┌─────────────────┐
│  SQLite Store   │
│  (Destination)  │
└─────────────────┘
```

### 3.3 Migration Process Flow

#### Step 1: Load Tokens from File Store

```go
func (m *Migrator) Migrate() (*MigrationStats, error) {
    // Step 1: Load tokens from file store
    tokens, err := m.fileStore.Load()
    if err != nil {
        return nil, fmt.Errorf("failed to load tokens from file store: %w", err)
    }
    stats.TotalTokens = len(tokens)
}
```

**Validation:**
- Verify file store directory exists
- Check for corrupted JSON files
- Log any files that cannot be parsed

#### Step 2: Load Settings from File Store

```go
// Type assertion to get MultiTokenStore for Settings field
multiStore, ok := m.fileStore.(*MultiTokenStore)
if !ok {
    return nil, fmt.Errorf("fileStore is not a MultiTokenStore, cannot access settings")
}
settings := multiStore.Settings
```

**Validation:**
- Verify settings file exists
- Validate settings structure
- Use defaults if settings file is missing

#### Step 3: Migrate Tokens to SQLite

```go
for id, token := range tokens {
    if err := m.migrateToken(token); err != nil {
        stats.FailedTokens++
        stats.Errors = append(stats.Errors, MigrationError{
            TokenID: id,
            Email:   token.Email,
            Error:   err.Error(),
        })
        m.logger.ErrorLog("[Migrator] Failed to migrate token %s (email: %s): %v", id, token.Email, err)
    } else {
        stats.MigratedTokens++
        m.logger.DebugLog("[Migrator] Migrated token %s (email: %s)", id, token.Email)
    }
}
```

**Deduplication Strategy:**
- Check for duplicate by `refresh_token`
- Update existing token if duplicate found
- Preserve original token ID on update

#### Step 4: Migrate Settings to SQLite

```go
providerSettings := StoreSettings{
    SelectionStrategy: settings.SelectionStrategy,
    RefreshBufferSec:  settings.RefreshBufferSec,
    MaxErrorCount:     settings.MaxErrorCount,
    UpdatedAt:         time.Now().UnixMilli(),
}
if err := m.dbStore.SaveSettings(providerSettings); err != nil {
    m.logger.ErrorLog("[Migrator] Failed to migrate settings: %v", err)
    stats.Errors = append(stats.Errors, MigrationError{
        TokenID: "settings",
        Email:   "",
        Error:   fmt.Sprintf("Failed to migrate settings: %v", err),
    })
} else {
    stats.SettingsMigrated = true
    m.logger.InfoLog("[Migrator] Successfully migrated settings")
}
```

#### Step 5: Verify Migration

```go
func (m *Migrator) verifyMigration(stats *MigrationStats) error {
    // Count tokens in SQLite store
    dbTokens, err := m.dbStore.Load()
    if err != nil {
        return fmt.Errorf("failed to load tokens from SQLite store: %w", err)
    }

    // Count tokens in file store
    fileTokens, err := m.fileStore.Load()
    if err != nil {
        return fmt.Errorf("failed to load tokens from file store: %w", err)
    }

    // Verify counts match (excluding failed tokens)
    expectedCount := stats.TotalTokens - stats.FailedTokens
    if len(dbTokens) != expectedCount {
        return fmt.Errorf("token count mismatch: expected %d, got %d", expectedCount, len(dbTokens))
    }

    // Verify each token was migrated correctly
    for id, fileToken := range fileTokens {
        dbToken, exists := dbTokens[id]
        if !exists {
            return fmt.Errorf("token %s not found in SQLite store", id)
        }

        // Verify key fields match
        if dbToken.Email != fileToken.Email {
            m.logger.WarnLog("[Migrator] Email mismatch for token %s: file=%s, db=%s", 
                id, fileToken.Email, dbToken.Email)
        }
    }

    return nil
}
```

#### Step 6: Create Backup of File-Based Storage

```go
func (m *Migrator) createBackup() error {
    // Create backup directory
    backupDir := ".credentials.backup"
    if err := os.MkdirAll(backupDir, 0700); err != nil {
        return fmt.Errorf("failed to create backup directory: %w", err)
    }

    // Copy provider directory to backup
    multiStore := m.fileStore.(*MultiTokenStore)
    sourceDir := multiStore.providerDir
    destDir := filepath.Join(backupDir, filepath.Base(sourceDir))

    return copyDir(sourceDir, destDir)
}
```

### 3.4 Error Handling

#### Corrupt Entry Handling

| Error Type | Detection | Handling |
|------------|-----------|----------|
| Invalid JSON | JSON unmarshal error | Log warning, skip file |
| Missing fields | Field validation | Use defaults, log warning |
| Invalid timestamp | Date validation | Use current time, log warning |
| Duplicate refresh token | Database query | Update existing token |

#### Error Recovery

```go
// Continue processing even if individual tokens fail
for id, token := range tokens {
    if err := m.migrateToken(token); err != nil {
        stats.FailedTokens++
        stats.Errors = append(stats.Errors, MigrationError{
            TokenID: id,
            Email:   token.Email,
            Error:   err.Error(),
        })
        // Continue with next token
        continue
    }
    stats.MigratedTokens++
}
```

### 3.5 Migration Statistics

```go
type MigrationStats struct {
    TotalTokens      int
    MigratedTokens   int
    FailedTokens     int
    SkippedTokens    int
    SettingsMigrated bool
    Errors           []MigrationError
}

type MigrationError struct {
    TokenID string
    Email   string
    Error   string
}
```

### 3.6 Rollback Strategy

If migration fails:

1. **Partial Migration**: SQLite store contains some tokens
2. **Rollback Action**: Delete all migrated tokens for the provider
3. **Fallback**: Continue using file-based storage
4. **Log Error**: Detailed error logging for troubleshooting

```go
func (m *Migrator) Rollback() error {
    // Delete all tokens for this provider from SQLite
    _, err := m.dbStore.db.Exec("DELETE FROM tokens WHERE provider_id = ?", m.dbStore.providerID)
    if err != nil {
        return fmt.Errorf("failed to rollback migration: %w", err)
    }
    m.logger.InfoLog("[Migrator] Rolled back migration for provider %s", m.dbStore.providerID)
    return nil
}
```

### 3.7 Dry Run Mode

A dry run mode should be added to validate migration without making changes:

```go
func (m *Migrator) DryRun() (*MigrationStats, error) {
    stats := &MigrationStats{Errors: make([]MigrationError, 0)}
    
    // Load tokens from file store
    tokens, err := m.fileStore.Load()
    if err != nil {
        return nil, fmt.Errorf("failed to load tokens: %w", err)
    }
    stats.TotalTokens = len(tokens)
    
    // Validate each token
    for id, token := range tokens {
        if err := m.validateToken(token); err != nil {
            stats.FailedTokens++
            stats.Errors = append(stats.Errors, MigrationError{
                TokenID: id,
                Email:   token.Email,
                Error:   err.Error(),
            })
        } else {
            stats.MigratedTokens++
        }
    }
    
    return stats, nil
}
```

---

## Phase 4: Refactoring and Integration

### 4.1 Refactoring Overview

This phase describes the steps to identify all instances of file storage usage and replace them with the new SQLite interface, ensuring backward compatibility.

### 4.2 Component Analysis

#### 4.2.1 Store Factory Pattern

The [`StoreFactory`](qwencoder-proxy/internal/token/store_factory.go:21-33) in [`store_factory.go`](qwencoder-proxy/internal/token/store_factory.go) already provides a factory pattern for switching between storage backends.

**Current Implementation:**

```go
func NewTokenStore(config StoreConfig) (TokenStore, error) {
    switch config.Backend {
    case "sqlite":
        return newSQLiteStore(config)
    case "file":
        return newFileStore(config)
    default:
        return nil, fmt.Errorf("unsupported storage backend: %s", config.Backend)
    }
}
```

**Enhancement Needed:**

Add "auto" mode to detect and migrate automatically:

```go
func NewTokenStore(config StoreConfig) (TokenStore, error) {
    switch config.Backend {
    case "sqlite":
        return newSQLiteStore(config)
    case "file":
        return newFileStore(config)
    case "auto":
        return newAutoStore(config)
    default:
        return nil, fmt.Errorf("unsupported storage backend: %s", config.Backend)
    }
}

func newAutoStore(config StoreConfig) (TokenStore, error) {
    // Check if SQLite database exists
    dbPath := config.DBPath
    if _, err := os.Stat(dbPath); err == nil {
        // SQLite database exists, use it
        return newSQLiteStore(config)
    }
    
    // Check if file-based storage exists
    fileStore := NewMultiTokenStore(config.ProviderID, config.FilePath, config.Logger)
    if err := fileStore.Load(); err == nil && len(fileStore.Tokens) > 0 {
        // File-based storage has tokens, migrate to SQLite
        return migrateAndCreateSQLiteStore(config, fileStore)
    }
    
    // No existing storage, create new SQLite store (default)
    return newSQLiteStore(config)
}

func migrateAndCreateSQLiteStore(config StoreConfig, fileStore *MultiTokenStore) (TokenStore, error) {
    // Create SQLite store
    dbStore, err := newSQLiteStore(config)
    if err != nil {
        return nil, fmt.Errorf("failed to create SQLite store: %w", err)
    }
    
    // Create migrator
    migrator := NewMigrator(fileStore, dbStore.(*SQLiteStore), config.Logger)
    
    // Perform migration
    stats, err := migrator.Migrate()
    if err != nil {
        config.Logger.ErrorLog("[AutoStore] Migration failed: %v", err)
        // Fall back to file store
        return fileStore, nil
    }
    
    config.Logger.InfoLog("[AutoStore] Migration completed: %d/%d tokens migrated", 
        stats.MigratedTokens, stats.TotalTokens)
    
    return dbStore, nil
}
```

#### 4.2.2 MultiTokenManager Integration

The [`MultiTokenManager`](qwencoder-proxy/internal/token/multi_token_manager.go:27-44) in [`multi_token_manager.go`](qwencoder-proxy/internal/token/multi_token_manager.go) manages token stores for all providers.

**Current Implementation:**

```go
func (mtm *MultiTokenManager) GetTokenStore(providerID string) (*MultiTokenStore, error) {
    mtm.mu.RLock()
    store, exists := mtm.stores[providerID]
    mtm.mu.RUnlock()
    
    if exists {
        return store, nil
    }
    
    mtm.mu.Lock()
    defer mtm.mu.Unlock()
    
    // Create new store
    store = NewMultiTokenStore(providerID, mtm.getFilePath(providerID), mtm.logger)
    if err := store.Load(); err != nil {
        return nil, fmt.Errorf("failed to load token store: %w", err)
    }
    
    mtm.stores[providerID] = store
    return store, nil
}
```

**Refactored Implementation:**

```go
func (mtm *MultiTokenManager) GetTokenStore(providerID string) (TokenStore, error) {
    mtm.mu.RLock()
    store, exists := mtm.stores[providerID]
    mtm.mu.RUnlock()
    
    if exists {
        return store, nil
    }
    
    mtm.mu.Lock()
    defer mtm.mu.Unlock()
    
    // Create store using factory
    config := StoreConfig{
        Backend:     mtm.storageBackend, // "auto", "sqlite", or "file"
        DBPath:      mtm.getDBPath(),
        ProviderID:  providerID,
        FilePath:    mtm.getFilePath(providerID),
        Credentials: mtm.credentialsDir,
        Logger:      mtm.logger,
    }
    
    store, err := NewTokenStore(config)
    if err != nil {
        return nil, fmt.Errorf("failed to create token store: %w", err)
    }
    
    // Load tokens
    if err := store.Load(); err != nil {
        return nil, fmt.Errorf("failed to load token store: %w", err)
    }
    
    mtm.stores[providerID] = store
    return store, nil
}
```

### 4.3 Configuration Changes

#### 4.3.1 Environment Variables

Add new environment variables:

| Variable | Description | Default |
|----------|-------------|---------|
| `STORAGE_BACKEND` | Storage backend type | `auto` |
| `STORAGE_DB_PATH` | SQLite database path | `.credentials/tokens.db` |
| `STORAGE_CREDENTIALS_DIR` | Credentials directory | `.credentials` |

#### 4.3.2 Configuration Structure

```go
type StorageConfig struct {
    Backend     string `json:"backend" env:"STORAGE_BACKEND"`
    DBPath      string `json:"db_path" env:"STORAGE_DB_PATH"`
    Credentials string `json:"credentials" env:"STORAGE_CREDENTIALS_DIR"`
}

func (c *StorageConfig) Load() error {
    // Load from environment variables
    if c.Backend == "" {
        c.Backend = "auto"
    }
    if c.DBPath == "" {
        c.DBPath = ".credentials/tokens.db"
    }
    if c.Credentials == "" {
        c.Credentials = ".credentials"
    }
    return nil
}
```

### 4.4 Backward Compatibility Strategy

#### 4.4.1 Gradual Migration

The migration should support gradual adoption:

1. **Phase 1**: SQLite as opt-in (backend="sqlite")
2. **Phase 2**: Auto-migration (backend="auto")
3. **Phase 3**: SQLite as default (backend="sqlite")
4. **Phase 4**: Remove file-based storage

#### 4.4.2 Fallback Mechanism

If SQLite operations fail, fall back to file storage:

```go
type FallbackStore struct {
    primary   TokenStore  // SQLite store
    secondary TokenStore  // File store (fallback)
    logger    logging.Logger
}

func (fs *FallbackStore) Load() (map[string]TokenMetadata, error) {
    tokens, err := fs.primary.Load()
    if err != nil {
        fs.logger.WarnLog("[FallbackStore] Primary store failed, using fallback: %v", err)
        return fs.secondary.Load()
    }
    return tokens, nil
}

func (fs *FallbackStore) Save(tokens map[string]TokenMetadata) error {
    err := fs.primary.Save(tokens)
    if err != nil {
        fs.logger.WarnLog("[FallbackStore] Primary save failed, using fallback: %v", err)
        return fs.secondary.Save(tokens)
    }
    return nil
}
```

### 4.5 Code Changes Required

#### 4.5.1 Files to Modify

| File | Changes |
|------|---------|
| [`internal/token/store_factory.go`](qwencoder-proxy/internal/token/store_factory.go) | Add "auto" backend mode |
| [`internal/token/multi_token_manager.go`](qwencoder-proxy/internal/token/multi_token_manager.go) | Use factory for store creation |
| [`config/config.go`](qwencoder-proxy/config/config.go) | Add storage configuration |
| [`cmd/qwencoder-proxy/main.go`](qwencoder-proxy/cmd/qwencoder-proxy/main.go) | Load storage configuration |
| [`internal/token/migrator.go`](qwencoder-proxy/internal/token/migrator.go) | Add dry run and rollback methods |

#### 4.5.2 New Files to Create

| File | Purpose |
|------|---------|
| `internal/token/fallback_store.go` | Fallback storage implementation |
| `internal/token/auto_store.go` | Auto-detection and migration store |

### 4.6 Testing Strategy During Refactoring

1. **Unit Tests**: Test each refactored component independently
2. **Integration Tests**: Test the full migration flow
3. **Regression Tests**: Ensure existing functionality still works
4. **Performance Tests**: Compare performance between backends

---

## Phase 5: Comprehensive Testing

### 5.1 Testing Overview

This phase outlines unit tests for the new database logic and integration tests to verify the migration process and application stability.

### 5.2 Unit Tests

#### 5.2.1 SQLite Store Tests

**Test File:** [`internal/token/sqlite_store_test.go`](qwencoder-proxy/internal/token/sqlite_store_test.go)

**Test Categories:**

| Test Category | Description |
|---------------|-------------|
| Initialization | Test store creation and connection |
| CRUD Operations | Test Create, Read, Update, Delete |
| Transaction Handling | Test transaction commit/rollback |
| Error Handling | Test error scenarios |
| Concurrency | Test concurrent access |
| Prepared Statements | Test statement preparation and execution |

**Example Test Cases:**

```go
func TestSQLiteStore_Load_Empty(t *testing.T) {
    store := setupTestStore(t)
    defer store.Close()
    
    tokens, err := store.Load()
    assert.NoError(t, err)
    assert.Empty(t, tokens)
}

func TestSQLiteStore_AddToken(t *testing.T) {
    store := setupTestStore(t)
    defer store.Close()
    
    token := createTestToken()
    err := store.AddToken(token)
    
    assert.NoError(t, err)
    
    // Verify token was added
    tokens, err := store.Load()
    assert.NoError(t, err)
    assert.Len(t, tokens, 1)
    assert.Equal(t, token.ID, tokens[token.ID].ID)
}

func TestSQLiteStore_AddToken_DuplicateRefreshToken(t *testing.T) {
    store := setupTestStore(t)
    defer store.Close()
    
    token1 := createTestToken()
    token2 := createTestToken()
    token2.ID = "different-id" // Same refresh token, different ID
    token2.RefreshToken = token1.RefreshToken
    
    err := store.AddToken(token1)
    assert.NoError(t, err)
    
    err = store.AddToken(token2)
    assert.NoError(t, err) // Should update existing token
    
    // Verify only one token exists
    tokens, err := store.Load()
    assert.NoError(t, err)
    assert.Len(t, tokens, 1)
}

func TestSQLiteStore_UpdateToken(t *testing.T) {
    store := setupTestStore(t)
    defer store.Close()
    
    token := createTestToken()
    err := store.AddToken(token)
    assert.NoError(t, err)
    
    // Update token
    err = store.UpdateToken(token.ID, func(t *TokenMetadata) {
        t.Healthy = false
        t.ErrorCount = 5
    })
    assert.NoError(t, err)
    
    // Verify update
    tokens, _ := store.Load()
    updated := tokens[token.ID]
    assert.False(t, updated.Healthy)
    assert.Equal(t, 5, updated.ErrorCount)
}

func TestSQLiteStore_DeleteToken(t *testing.T) {
    store := setupTestStore(t)
    defer store.Close()
    
    token := createTestToken()
    err := store.AddToken(token)
    assert.NoError(t, err)
    
    // Delete token
    err = store.DeleteToken(token.ID)
    assert.NoError(t, err)
    
    // Verify deletion
    tokens, _ := store.Load()
    assert.Empty(t, tokens)
}

func TestSQLiteStore_Transaction_Rollback(t *testing.T) {
    store := setupTestStore(t)
    defer store.Close()
    
    token := createTestToken()
    err := store.AddToken(token)
    assert.NoError(t, err)
    
    // Start transaction and fail
    tx, _ := store.db.Begin()
    tx.Exec("DELETE FROM tokens WHERE provider_id = ?", store.providerID)
    tx.Rollback()
    
    // Verify token still exists
    tokens, _ := store.Load()
    assert.Len(t, tokens, 1)
}
```

#### 5.2.2 Migrator Tests

**Test File:** [`internal/token/migrator_test.go`](qwencoder-proxy/internal/token/migrator_test.go)

**Test Categories:**

| Test Category | Description |
|---------------|-------------|
| Migration Success | Test successful migration |
| Migration Failure | Test error handling |
| Duplicate Handling | Test duplicate token handling |
| Settings Migration | Test settings migration |
| Verification | Test migration verification |
| Rollback | Test rollback functionality |

**Example Test Cases:**

```go
func TestMigrator_Migrate_Success(t *testing.T) {
    fileStore := setupFileStoreWithTokens(t, 3)
    dbStore := setupTestSQLiteStore(t)
    migrator := NewMigrator(fileStore, dbStore, testLogger)
    
    stats, err := migrator.Migrate()
    
    assert.NoError(t, err)
    assert.Equal(t, 3, stats.TotalTokens)
    assert.Equal(t, 3, stats.MigratedTokens)
    assert.Equal(t, 0, stats.FailedTokens)
    assert.True(t, stats.SettingsMigrated)
}

func TestMigrator_Migrate_DuplicateRefreshToken(t *testing.T) {
    fileStore := setupFileStoreWithTokens(t, 2)
    dbStore := setupTestSQLiteStore(t)
    
    // Add one token to DB with same refresh token as first file token
    existingToken := createTestToken()
    existingToken.ID = "existing-id"
    existingToken.RefreshToken = fileStore.Tokens[0].RefreshToken
    dbStore.AddToken(existingToken)
    
    migrator := NewMigrator(fileStore, dbStore, testLogger)
    stats, err := migrator.Migrate()
    
    assert.NoError(t, err)
    assert.Equal(t, 2, stats.MigratedTokens)
    
    // Verify duplicate was updated
    tokens, _ := dbStore.Load()
    assert.Contains(t, tokens, "existing-id")
}

func TestMigrator_Migrate_CorruptToken(t *testing.T) {
    fileStore := setupFileStoreWithTokens(t, 3)
    dbStore := setupTestSQLiteStore(t)
    
    // Corrupt one token
    fileStore.Tokens[1].AccessToken = "" // Invalid token
    
    migrator := NewMigrator(fileStore, dbStore, testLogger)
    stats, err := migrator.Migrate()
    
    assert.NoError(t, err) // Migration should succeed
    assert.Equal(t, 2, stats.MigratedTokens)
    assert.Equal(t, 1, stats.FailedTokens)
    assert.Len(t, stats.Errors, 1)
}

func TestMigrator_VerifyMigration_Success(t *testing.T) {
    fileStore := setupFileStoreWithTokens(t, 3)
    dbStore := setupTestSQLiteStore(t)
    migrator := NewMigrator(fileStore, dbStore, testLogger)
    
    stats, _ := migrator.Migrate()
    err := migrator.verifyMigration(stats)
    
    assert.NoError(t, err)
}

func TestMigrator_VerifyMigration_TokenMismatch(t *testing.T) {
    fileStore := setupFileStoreWithTokens(t, 3)
    dbStore := setupTestSQLiteStore(t)
    migrator := NewMigrator(fileStore, dbStore, testLogger)
    
    stats, _ := migrator.Migrate()
    
    // Modify a token in DB to cause mismatch
    tokens, _ := dbStore.Load()
    for id := range tokens {
        dbStore.UpdateToken(id, func(t *TokenMetadata) {
            t.Email = "modified@example.com"
        })
        break
    }
    
    err := migrator.verifyMigration(stats)
    assert.Error(t, err) // Should detect mismatch
}

func TestMigrator_Rollback(t *testing.T) {
    fileStore := setupFileStoreWithTokens(t, 3)
    dbStore := setupTestSQLiteStore(t)
    migrator := NewMigrator(fileStore, dbStore, testLogger)
    
    // Migrate
    stats, _ := migrator.Migrate()
    assert.Equal(t, 3, stats.MigratedTokens)
    
    // Rollback
    err := migrator.Rollback()
    assert.NoError(t, err)
    
    // Verify all tokens removed
    tokens, _ := dbStore.Load()
    assert.Empty(t, tokens)
}
```

#### 5.2.3 Store Factory Tests

**Test File:** [`internal/token/store_factory_test.go`](qwencoder-proxy/internal/token/store_factory_test.go)

**Test Categories:**

| Test Category | Description |
|---------------|-------------|
| SQLite Backend | Test SQLite store creation |
| File Backend | Test file store creation |
| Auto Backend | Test auto-detection |
| Invalid Backend | Test error handling |

**Example Test Cases:**

```go
func TestNewTokenStore_SQLite(t *testing.T) {
    config := StoreConfig{
        Backend:    "sqlite",
        DBPath:     ":memory:",
        ProviderID: "test-provider",
        Logger:     testLogger,
    }
    
    store, err := NewTokenStore(config)
    
    assert.NoError(t, err)
    assert.IsType(t, &SQLiteStore{}, store)
}

func TestNewTokenStore_File(t *testing.T) {
    config := StoreConfig{
        Backend:     "file",
        FilePath:    filepath.Join(t.TempDir(), "test.json"),
        ProviderID: "test-provider",
        Credentials: t.TempDir(),
        Logger:      testLogger,
    }
    
    store, err := NewTokenStore(config)
    
    assert.NoError(t, err)
    assert.IsType(t, &MultiTokenStore{}, store)
}

func TestNewTokenStore_Invalid(t *testing.T) {
    config := StoreConfig{
        Backend: "invalid",
        Logger:  testLogger,
    }
    
    store, err := NewTokenStore(config)
    
    assert.Error(t, err)
    assert.Nil(t, store)
}
```

### 5.3 Integration Tests

#### 5.3.1 End-to-End Migration Test

**Test File:** [`internal/token/migration_integration_test.go`](qwencoder-proxy/internal/token/migration_integration_test.go)

```go
func TestMigration_EndToEnd(t *testing.T) {
    // Setup: Create file store with multiple tokens
    tempDir := t.TempDir()
    fileStore := setupFileStoreWithTokens(t, 10)
    
    // Execute: Migrate to SQLite
    dbPath := filepath.Join(tempDir, "tokens.db")
    dbStore, err := NewSQLiteStore(dbPath, "test-provider", testLogger)
    assert.NoError(t, err)
    
    migrator := NewMigrator(fileStore, dbStore, testLogger)
    stats, err := migrator.Migrate()
    assert.NoError(t, err)
    
    // Verify: All tokens migrated
    assert.Equal(t, 10, stats.MigratedTokens)
    assert.Equal(t, 0, stats.FailedTokens)
    
    // Verify: Can use SQLite store
    tokens, err := dbStore.Load()
    assert.NoError(t, err)
    assert.Len(t, tokens, 10)
    
    // Verify: Can add new tokens to SQLite store
    newToken := createTestToken()
    err = dbStore.AddToken(newToken)
    assert.NoError(t, err)
    
    tokens, _ = dbStore.Load()
    assert.Len(t, tokens, 11)
}

func TestMigration_WithApplication(t *testing.T) {
    // This test simulates the actual application flow
    
    // 1. Create MultiTokenManager
    logger := logging.NewLogger()
    mtm := NewMultiTokenManager(logger)
    mtm.SetCredentialsDir(t.TempDir())
    mtm.Initialize()
    
    // 2. Register provider
    config := ProviderConfig{
        ID:           "test-provider",
        ClientID:     "test-client",
        ClientSecret: "test-secret",
        AuthURL:      "https://example.com/auth",
        TokenURL:     "https://example.com/token",
        Flow:         "device",
    }
    mtm.RegisterProviderConfig(config)
    
    // 3. Get token store (should use file-based initially)
    store, err := mtm.GetTokenStore("test-provider")
    assert.NoError(t, err)
    assert.IsType(t, &MultiTokenStore{}, store)
    
    // 4. Add some tokens
    for i := 0; i < 5; i++ {
        token := createTestToken()
        token.ID = fmt.Sprintf("token-%d", i)
        store.AddToken(token)
    }
    
    // 5. Now switch to SQLite and migrate
    mtm.storageBackend = "auto"
    mtm.dbPath = filepath.Join(t.TempDir(), "tokens.db")
    
    // 6. Get store again (should trigger migration)
    store2, err := mtm.GetTokenStore("test-provider")
    assert.NoError(t, err)
    assert.IsType(t, &SQLiteStore{}, store2)
    
    // 7. Verify all tokens are available
    tokens, err := store2.Load()
    assert.NoError(t, err)
    assert.Len(t, tokens, 5)
}
```

#### 5.3.2 Concurrent Access Test

```go
func TestSQLiteStore_ConcurrentAccess(t *testing.T) {
    store := setupTestStore(t)
    defer store.Close()
    
    // Add initial token
    token := createTestToken()
    store.AddToken(token)
    
    // Run concurrent operations
    var wg sync.WaitGroup
    errors := make(chan error, 100)
    
    for i := 0; i < 50; i++ {
        wg.Add(1)
        go func(iteration int) {
            defer wg.Done()
            
            // Randomly choose operation
            switch iteration % 4 {
            case 0:
                _, err := store.Load()
                errors <- err
            case 1:
                newToken := createTestToken()
                newToken.ID = fmt.Sprintf("token-%d", iteration)
                errors <- store.AddToken(newToken)
            case 2:
                errors <- store.UpdateToken(token.ID, func(t *TokenMetadata) {
                    t.LastUsed = time.Now().UnixMilli()
                })
            case 3:
                // This might fail if token doesn't exist
                store.DeleteToken(fmt.Sprintf("token-%d", iteration))
            }
        }(i)
    }
    
    wg.Wait()
    close(errors)
    
    // Check for errors
    for err := range errors {
        if err != nil && !errors.Is(err, sql.ErrNoRows) {
            t.Errorf("Concurrent operation failed: %v", err)
        }
    }
    
    // Verify final state
    tokens, err := store.Load()
    assert.NoError(t, err)
    assert.Greater(t, len(tokens), 0)
}
```

### 5.4 Performance Tests

#### 5.4.1 Load Performance

```go
func BenchmarkSQLiteStore_Load_100Tokens(b *testing.B) {
    store := setupBenchmarkStore(b, 100)
    defer store.Close()
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        store.Load()
    }
}

func BenchmarkSQLiteStore_Load_1000Tokens(b *testing.B) {
    store := setupBenchmarkStore(b, 1000)
    defer store.Close()
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        store.Load()
    }
}

func BenchmarkFileStore_Load_100Tokens(b *testing.B) {
    store := setupBenchmarkFileStore(b, 100)
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        store.Load()
    }
}
```

#### 5.4.2 Migration Performance

```go
func BenchmarkMigrator_Migrate_100Tokens(b *testing.B) {
    fileStore := setupBenchmarkFileStore(b, 100)
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        dbStore := setupBenchmarkSQLiteStore(b)
        migrator := NewMigrator(fileStore, dbStore, testLogger)
        migrator.Migrate()
        dbStore.Close()
    }
}
```

### 5.5 Test Coverage Goals

| Component | Target Coverage |
|-----------|----------------|
| SQLiteStore | 90%+ |
| Migrator | 95%+ |
| StoreFactory | 85%+ |
| MultiTokenManager | 80%+ |
| Overall | 85%+ |

### 5.6 Test Execution

```bash
# Run all tests
go test ./internal/token/... -v

# Run with coverage
go test ./internal/token/... -cover -coverprofile=coverage.out

# Run specific test
go test ./internal/token -run TestMigrator_Migrate_Success -v

# Run benchmarks
go test ./internal/token -bench=. -benchmem

# Run race detector
go test ./internal/token/... -race
```

---

## Phase 6: Cleanup and Removal

### 6.1 Cleanup Overview

This phase lists the specific files, functions, and dependencies related to the old file storage that must be deleted to prevent technical debt after successful migration.

### 6.2 Files to Remove

#### 6.2.1 Primary Files

| File | Reason for Removal |
|------|-------------------|
| `internal/token/multi_token_store.go` | Replaced by SQLiteStore |
| `internal/token/fallback_store.go` | No longer needed after migration |
| `internal/token/auto_store.go` | No longer needed after migration |

**Note:** These files should be removed **after** migration is complete and verified.

#### 6.2.2 Test Files

| File | Reason for Removal |
|------|-------------------|
| `internal/token/multi_token_store_test.go` | Tests for removed component |
| `internal/token/fallback_store_test.go` | Tests for removed component |

### 6.3 Functions to Remove

#### 6.3.1 From multi_token_store.go

| Function | Replacement |
|----------|-------------|
| `NewMultiTokenStore()` | `NewSQLiteStore()` |
| `(*MultiTokenStore).Load()` | `(*SQLiteStore).Load()` |
| `(*MultiTokenStore).Save()` | `(*SQLiteStore).Save()` |
| `(*MultiTokenStore).AddToken()` | `(*SQLiteStore).AddToken()` |
| `(*MultiTokenStore).UpdateToken()` | `(*SQLiteStore).UpdateToken()` |
| `(*MultiTokenStore).RemoveToken()` | `(*SQLiteStore).DeleteToken()` |
| `(*MultiTokenStore).ListTokens()` | `(*SQLiteStore).Load()` |
| `(*MultiTokenStore).GetSettings()` | `(*SQLiteStore).GetSettings()` |
| `(*MultiTokenStore).migrateFromLegacy()` | No longer needed |

#### 6.3.2 From store_factory.go

| Function | Reason |
|----------|--------|
| `newFileStore()` | File backend no longer supported |
| `newAutoStore()` | Auto mode no longer needed |

### 6.4 Dependencies to Remove

| Dependency | Reason |
|------------|--------|
| `github.com/gofrs/flock` | File locking no longer needed |

### 6.5 Configuration Changes

#### 6.5.1 Environment Variables to Deprecate

| Variable | Status |
|----------|--------|
| `STORAGE_BACKEND` | Remove "file" and "auto" options |
| `STORAGE_CREDENTIALS_DIR` | No longer needed for file storage |

#### 6.5.2 Configuration Structure Changes

```go
// Before
type StorageConfig struct {
    Backend     string // "file", "sqlite", "auto"
    DBPath      string
    FilePath    string
    Credentials string
}

// After
type StorageConfig struct {
    Backend string // Only "sqlite"
    DBPath  string
}
```

### 6.6 Directory Cleanup

#### 6.6.1 Directories to Remove

After successful migration and verification:

```
.credentials/
├── gemini/              # Remove after migration
├── qwen/               # Remove after migration
├── iflow/              # Remove after migration
├── kiro/               # Remove after migration
├── antigravity/        # Remove after migration
├── .credentials.backup/ # Remove after verification period
└── tokens.db           # Keep (new SQLite database)
```

#### 6.6.2 Cleanup Script

```bash
#!/bin/bash
# cleanup_file_storage.sh - Remove file-based storage after migration

CREDENTIALS_DIR=".credentials"
BACKUP_DIR=".credentials.backup"

echo "WARNING: This will remove all file-based token storage."
echo "Please ensure migration is complete and verified."
read -p "Continue? (yes/no): " confirm

if [ "$confirm" != "yes" ]; then
    echo "Aborted."
    exit 0
fi

# Create final backup
echo "Creating final backup..."
tar -czf "credentials-backup-$(date +%Y%m%d-%H%M%S).tar.gz" "$CREDENTIALS_DIR"

# Remove provider directories
for provider in gemini qwen iflow kiro antigravity; do
    if [ -d "$CREDENTIALS_DIR/$provider" ]; then
        echo "Removing $CREDENTIALS_DIR/$provider"
        rm -rf "$CREDENTIALS_DIR/$provider"
    fi
done

# Remove backup directory
if [ -d "$BACKUP_DIR" ]; then
    echo "Removing $BACKUP_DIR"
    rm -rf "$BACKUP_DIR"
fi

echo "Cleanup complete."
```

### 6.7 Code Refactoring

#### 6.7.1 Type Alias Removal

```go
// Remove this alias
type TokenMetadata = ProviderToken

// Use ProviderToken directly
```

#### 6.7.2 Interface Simplification

```go
// Before
type TokenStore interface {
    Load() (map[string]TokenMetadata, error)
    Save(tokens map[string]TokenMetadata) error
    GetCredentialsPath() string
    Clear() error
}

// After (simplified)
type TokenStore interface {
    Load() (map[string]ProviderToken, error)
    Save(tokens map[string]ProviderToken) error
    Clear() error
}
```

### 6.8 Documentation Updates

#### 6.8.1 Documentation to Update

| Document | Changes |
|----------|---------|
| README.md | Update storage documentation |
| docs/migration/* | Mark as historical |
| docs/api-reference.md | Update API examples |
| how_to_enable_sqlite_storage.md | Update to reflect default status |

#### 6.8.2 Deprecation Notices

Add deprecation notices to removed components:

```go
// Deprecated: Use SQLiteStore instead.
// This will be removed in version 2.0.
type MultiTokenStore struct {
    // ...
}
```

### 6.9 Migration Verification Checklist

Before cleanup, verify:

- [ ] All tokens successfully migrated
- [ ] Application runs with SQLite backend
- [ ] All tests pass
- [ ] Performance is acceptable
- [ ] No file storage references in code
- [ ] Backup created
- [ ] Documentation updated
- [ ] Team notified of changes

### 6.10 Rollback Plan

If issues arise after cleanup:

1. **Restore from Backup**: Extract backup tarball
2. **Revert Code**: Restore from git tag
3. **Switch Backend**: Set `STORAGE_BACKEND=file`
4. **Notify Users**: Inform of rollback

---

## Implementation Roadmap

### Timeline

| Phase | Duration | Dependencies |
|-------|----------|--------------|
| Phase 1: Schema Design | 1 week | None |
| Phase 2: Data Access Layer | 1 week | Phase 1 |
| Phase 3: Migration Strategy | 1 week | Phase 2 |
| Phase 4: Refactoring | 2 weeks | Phase 3 |
| Phase 5: Testing | 2 weeks | Phase 4 |
| Phase 6: Cleanup | 1 week | Phase 5 |

**Total Duration:** 8 weeks

### Milestones

| Milestone | Date | Deliverable |
|-----------|------|-------------|
| M1: Schema Complete | Week 1 | Schema documentation |
| M2: Data Access Layer | Week 2 | SQLiteStore implementation |
| M3: Migration Working | Week 3 | Migrator with tests |
| M4: Integration Complete | Week 5 | Application using SQLite |
| M5: Testing Complete | Week 7 | Test coverage 85%+ |
| M6: Production Ready | Week 8 | File storage removed |

### Resource Requirements

| Role | Allocation |
|------|------------|
| Senior Software Architect | 20% |
| Go Developer | 100% |
| QA Engineer | 50% |
| DevOps Engineer | 10% |

---

## Risk Assessment and Mitigation

### Risk Matrix

| Risk | Probability | Impact | Mitigation |
|------|-------------|--------|------------|
| Data loss during migration | Low | Critical | Backup before migration, verification after |
| Performance degradation | Medium | Medium | Performance testing, optimization |
| Application downtime | Low | High | Rolling migration, fallback mechanism |
| Concurrent access issues | Medium | Medium | WAL mode, proper locking |
| Schema incompatibility | Low | High | Schema versioning, migration tests |

### Mitigation Strategies

#### 1. Data Loss Prevention

- **Backup**: Create full backup before migration
- **Verification**: Verify all tokens after migration
- **Rollback**: Keep backup until verification complete
- **Dry Run**: Test migration with copy of data

#### 2. Performance Mitigation

- **Benchmarking**: Compare performance before/after
- **Indexing**: Optimize indexes based on query patterns
- **Connection Pooling**: Tune connection pool settings
- **Caching**: Implement in-memory cache for hot data

#### 3. Downtime Minimization

- **Rolling Migration**: Migrate provider by provider
- **Fallback**: Keep file storage as fallback
- **Zero Downtime**: Use auto-detection with fallback
- **Monitoring**: Monitor for issues during migration

#### 4. Concurrent Access

- **WAL Mode**: Enable Write-Ahead Logging
- **Connection Pool**: Use connection pooling
- **Prepared Statements**: Use prepared statements
- **Proper Locking**: Use sync.RWMutex correctly

#### 5. Schema Compatibility

- **Versioning**: Use schema versioning system
- **Migration Tests**: Test all migrations
- **Backward Compatibility**: Support old schema versions
- **Rollback**: Ability to rollback schema changes

---

## Rollback Strategy

### Rollback Triggers

Rollback should be triggered if:

1. **Data Loss**: Any tokens missing after migration
2. **Critical Errors**: Application cannot start
3. **Performance Issues**: Significant performance degradation
4. **Data Corruption**: Database corruption detected
5. **Security Issues**: Security vulnerabilities discovered

### Rollback Procedure

#### Step 1: Stop Application

```bash
# Stop the application
systemctl stop qwencoder-proxy
# or
pkill qwencoder-proxy
```

#### Step 2: Restore Code

```bash
# Restore from git tag
git checkout tags/pre-migration

# or restore specific files
git checkout HEAD~1 internal/token/multi_token_store.go
```

#### Step 3: Restore Data

```bash
# Extract backup
tar -xzf credentials-backup-YYYYMMDD-HHMMSS.tar.gz

# Restore .credentials directory
rm -rf .credentials
mv .credentials.backup .credentials
```

#### Step 4: Update Configuration

```bash
# Set backend to file
export STORAGE_BACKEND=file

# Or update config file
sed -i 's/backend: sqlite/backend: file/' config.yaml
```

#### Step 5: Start Application

```bash
# Start the application
systemctl start qwencoder-proxy

# Verify it's running
systemctl status qwencoder-proxy
```

#### Step 6: Verify

```bash
# Check logs for errors
journalctl -u qwencoder-proxy -f

# Verify tokens are accessible
curl http://localhost:8080/api/tokens
```

### Rollback Verification Checklist

- [ ] Application starts successfully
- [ ] All tokens are accessible
- [ ] No errors in logs
- [ ] API endpoints respond correctly
- [ ] Token refresh works
- [ ] Health checks pass

### Rollback Communication

Notify stakeholders of rollback:

1. **Development Team**: Immediate notification
2. **Operations Team**: Within 15 minutes
3. **Users**: Within 1 hour (if user-facing)
4. **Management**: Within 2 hours

---

## Conclusion

This comprehensive migration plan provides a structured approach to transitioning the qwencoder-proxy application from file-based token storage to SQLite. The plan ensures:

- **Zero Data Loss**: Through careful migration and verification
- **Minimal Downtime**: Through rolling migration and fallback
- **Performance Improvement**: Through optimized SQLite implementation
- **Future-Proof**: Through schema versioning and extensibility
- **Safe Rollback**: Through comprehensive backup and rollback procedures

Following this plan will result in a more robust, performant, and maintainable token storage system that can scale with the application's growth.

---

**Document Status:** Draft  
**Next Review Date:** 2026-03-01  
**Approval Required:** Yes
