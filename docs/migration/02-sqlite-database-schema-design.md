# SQLite Database Schema Design - qwencoder-proxy

## Table of Contents

1. [Overview](#overview)
2. [Database File Location](#database-file-location)
3. [Schema Versioning Strategy](#schema-versioning-strategy)
4. [Table Definitions](#table-definitions)
5. [Index Design](#index-design)
6. [Database Pragmas](#database-pragmas)
7. [Data Type Mapping](#data-type-mapping)
8. [Migration Path](#migration-path)
9. [Performance Considerations](#performance-considerations)
10. [Backup and Recovery Strategy](#backup-and-recovery-strategy)

---

## Overview

This document defines the SQLite database schema for storing OAuth tokens and metadata in the qwencoder-proxy application. The schema is designed to:

1. **Replace file-based storage** with a single SQLite database
2. **Support all providers** (Gemini, Qwen, iFlow, Kiro, Antigravity)
3. **Maintain data integrity** with proper constraints and transactions
4. **Enable efficient querying** through strategic indexing
5. **Support schema migrations** for future enhancements
6. **Follow SQLite best practices** for performance and reliability

### Design Principles

| Principle | Description |
|-----------|-------------|
| **Single Database** | All providers and tokens stored in one database file |
| **Provider Isolation** | Each provider's tokens are isolated by `provider_id` |
| **Data Integrity** | Foreign key constraints and transactions ensure consistency |
| **Query Performance** | Strategic indexes on common query patterns |
| **Concurrent Access** | WAL mode enables concurrent reads and writes |
| **Extensibility** | Schema versioning allows future enhancements |

---

## Database File Location

### Default Location

```
.credentials/tokens.db
```

### Configuration

The database path is configurable via:
- **Environment Variable**: `STORAGE_PATH`
- **Configuration File**: `config/config.go` - `StorageConfig.Path`

### File Organization

```
.credentials/
├── tokens.db          # SQLite database (all providers)
└── [legacy files]      # Old JSON files (during migration)
    ├── gemini/
    ├── qwen/
    ├── iflow/
    └── ...
```

---

## Schema Versioning Strategy

### Version Table

```sql
CREATE TABLE schema_migrations (
    version INTEGER PRIMARY KEY,
    applied_at INTEGER NOT NULL DEFAULT (strftime('%s', 'subsec') * 1000),
    description TEXT NOT NULL
);
```

### Version History

| Version | Description | Migration Date |
|---------|-------------|----------------|
| 1 | Initial schema with tokens, provider_settings, proxy_configs, schema_migrations tables | Initial Release |
| 2 | Reserved for future use | - |

### Migration Function Signature

```go
type Migration struct {
    version     int
    description string
    fn          func(*SQLiteStore) error
}
```

---

## Table Definitions

### 1. tokens Table

**Purpose:** Stores OAuth tokens with all metadata for each provider.

```sql
CREATE TABLE tokens (
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

#### Field Descriptions

| Field | Type | Description | Notes |
|-------|------|-------------|-------|
| `id` | TEXT | Unique token identifier (UUID) | Primary key |
| `provider_id` | TEXT | Provider identifier (e.g., "gemini-cli", "qwen") | Indexed |
| `access_token` | TEXT | OAuth access token | Encrypted at rest |
| `refresh_token` | TEXT | OAuth refresh token | Nullable |
| `token_type` | TEXT | Token type (usually "Bearer") | Default: 'Bearer' |
| `expiry_date` | INTEGER | Token expiry timestamp (Unix milliseconds) | Indexed |
| `email` | TEXT | User email extracted from token | Nullable, Indexed |
| `resource_url` | TEXT | Resource URL (Qwen-specific) | Nullable |
| `scope` | TEXT | OAuth scope (Gemini-specific) | Nullable |
| `api_key` | TEXT | API key (iFlow-specific) | Nullable |
| `healthy` | INTEGER | Token health status | 0=false, 1=true, Indexed |
| `health_score` | REAL | Token health score (0.0-1.0) | Default: 1.0 |
| `last_used` | INTEGER | Last usage timestamp (Unix milliseconds) | Indexed |
| `created_at` | INTEGER | Token creation timestamp (Unix milliseconds) | Auto-generated |
| `error_count` | INTEGER | Consecutive error count | Default: 0 |
| `last_error` | TEXT | Last error message | Nullable |
| `proxy_id` | TEXT | Foreign key to proxy_configs | Nullable |
| `updated_at` | INTEGER | Last update timestamp | Auto-generated |

### 2. provider_settings Table

**Purpose:** Stores provider-specific settings like token selection strategy and refresh parameters.

```sql
CREATE TABLE provider_settings (
    -- Primary Key
    provider_id TEXT PRIMARY KEY,
    
    -- Selection Settings
    selection_strategy TEXT NOT NULL DEFAULT 'random',
    refresh_buffer_sec INTEGER NOT NULL DEFAULT 1800,
    max_error_count INTEGER NOT NULL DEFAULT 3,
    
    -- Timestamps
    updated_at INTEGER NOT NULL DEFAULT (strftime('%s', 'subsec') * 1000)
);
```

#### Field Descriptions

| Field | Type | Description | Allowed Values |
|-------|------|-------------|----------------|
| `provider_id` | TEXT | Provider identifier (primary key) | "gemini-cli", "qwen", "iflow", "kiro", "antigravity" |
| `selection_strategy` | TEXT | Token selection strategy | "random", "round_robin", "least_used" |
| `refresh_buffer_sec` | INTEGER | Seconds before expiry to refresh | Default: 1800 (30 minutes) |
| `max_error_count` | INTEGER | Max errors before marking unhealthy | Default: 3 |
| `updated_at` | INTEGER | Last update timestamp | Auto-generated |

### 3. proxy_configs Table

**Purpose:** Normalized proxy configurations to avoid duplication and enable sharing.

```sql
CREATE TABLE proxy_configs (
    -- Primary Key
    id TEXT PRIMARY KEY,
    
    -- Proxy Configuration
    host TEXT NOT NULL,
    port INTEGER NOT NULL,
    username TEXT,
    password TEXT,
    proxy_type TEXT NOT NULL DEFAULT 'http',
    
    -- Timestamps
    created_at INTEGER NOT NULL DEFAULT (strftime('%s', 'subsec') * 1000),
    
    -- Unique Constraint
    UNIQUE(host, port)
);
```

#### Field Descriptions

| Field | Type | Description | Notes |
|-------|------|-------------|-------|
| `id` | TEXT | Unique proxy configuration identifier (UUID) | Primary key |
| `host` | TEXT | Proxy hostname | Required |
| `port` | INTEGER | Proxy port number | Required |
| `username` | TEXT | Proxy username | Nullable |
| `password` | TEXT | Proxy password | Nullable |
| `proxy_type` | TEXT | Proxy type | "http", "https", "socks5" |
| `created_at` | INTEGER | Creation timestamp | Auto-generated |

#### Proxy Type Enum

| Type | Description |
|------|-------------|
| `http` | HTTP proxy |
| `https` | HTTPS proxy |
| `socks5` | SOCKS5 proxy |

### 4. schema_migrations Table

**Purpose:** Tracks applied schema migrations for version control.

```sql
CREATE TABLE schema_migrations (
    version INTEGER PRIMARY KEY,
    applied_at INTEGER NOT NULL DEFAULT (strftime('%s', 'subsec') * 1000),
    description TEXT NOT NULL
);
```

#### Field Descriptions

| Field | Type | Description |
|-------|------|-------------|
| `version` | INTEGER | Schema version number | Primary key |
| `applied_at` | INTEGER | Migration application timestamp | Auto-generated |
| `description` | TEXT | Human-readable migration description | Required |

---

## Index Design

### Indexes for tokens Table

| Index Name | Definition | Purpose | Query Pattern |
|-----------|-----------|---------|--------------|
| `idx_tokens_provider_id` | `CREATE INDEX idx_tokens_provider_id ON tokens(provider_id)` | Filter tokens by provider | `WHERE provider_id = ?` |
| `idx_tokens_email` | `CREATE INDEX idx_tokens_email ON tokens(email)` | Find token by email | `WHERE email = ?` |
| `idx_tokens_expiry_date` | `CREATE INDEX idx_tokens_expiry_date ON tokens(expiry_date)` | Find expiring tokens | `WHERE expiry_date < ?` |
| `idx_tokens_healthy` | `CREATE INDEX idx_tokens_healthy ON tokens(healthy)` | Filter healthy tokens | `WHERE healthy = 1` |
| `idx_tokens_provider_healthy` | `CREATE INDEX idx_tokens_provider_healthy ON tokens(provider_id, healthy)` | Filter healthy tokens by provider | `WHERE provider_id = ? AND healthy = 1` |
| `idx_tokens_provider_expiry` | `CREATE INDEX idx_tokens_provider_expiry ON tokens(provider_id, expiry_date)` | Find expiring tokens by provider | `WHERE provider_id = ? AND expiry_date < ?` |

### Indexes for proxy_configs Table

| Index Name | Definition | Purpose | Query Pattern |
|-----------|-----------|---------|--------------|
| `idx_proxy_configs_host_port` | `CREATE INDEX idx_proxy_configs_host_port ON proxy_configs(host, port)` | Find proxy by host:port | `WHERE host = ? AND port = ?` |

---

## Database Pragmas

### Pragma Configuration

```sql
-- Enable WAL mode for better concurrency
PRAGMA journal_mode = WAL;

-- Set synchronous mode to NORMAL (balance between safety and performance)
PRAGMA synchronous = NORMAL;

-- Increase cache size (default is 2MB, use 10MB)
PRAGMA cache_size = -10240;

-- Enable foreign key constraints
PRAGMA foreign_keys = ON;

-- Set busy timeout to 5 seconds
PRAGMA busy_timeout = 5000;

-- Optimize for queries
PRAGMA optimize;
```

### Pragma Descriptions

| Pragma | Value | Purpose |
|--------|-------|---------|
| `journal_mode = WAL` | Write-Ahead Logging | Enables concurrent reads and writes |
| `synchronous = NORMAL` | Sync mode | Balance between safety and performance |
| `cache_size = -10240` | Cache size | 10MB cache for better performance |
| `foreign_keys = ON` | Foreign keys | Enable referential integrity |
| `busy_timeout = 5000` | Timeout | 5 second wait for lock acquisition |
| `optimize` | - | Query optimization | Run ANALYZE periodically |

---

## Data Type Mapping

### Go to SQLite Type Mapping

| Go Type | SQLite Type | Notes |
|----------|-------------|-------|
| `string` | TEXT | All string fields |
| `int` | INTEGER | Counters, ports |
| `int64` | INTEGER | Timestamps stored as milliseconds |
| `bool` | INTEGER | 0 = false, 1 = true |
| `float64` | REAL | Health scores |
| `time.Time` | INTEGER | Stored as Unix milliseconds |
| `uuid.UUID` | TEXT | Stored as string |
| `*ProxyConfig` | TEXT | Serialized as JSON in proxy_config table |

### Boolean Handling

```go
// Helper functions for boolean conversion
func boolToInt(b bool) int {
    if b {
        return 1
    }
    return 0
}

func intToBool(i int) bool {
    return i != 0
}
```

### Timestamp Handling

```go
// Convert time.Time to Unix milliseconds for storage
func timeToMs(t time.Time) int64 {
    return t.UnixMilli()
}

// Convert Unix milliseconds back to time.Time
func msToTime(ms int64) time.Time {
    return time.Unix(0, ms/1000)
}
```

---

## Migration Path

### Migration V1: Initial Schema

```go
func (s *SQLiteStore) migrateToV1() error {
    // Create tokens table
    if _, err := s.db.Exec(`
        CREATE TABLE IF NOT EXISTS tokens (
            id TEXT PRIMARY KEY,
            provider_id TEXT NOT NULL,
            access_token TEXT NOT NULL,
            refresh_token TEXT,
            token_type TEXT NOT NULL DEFAULT 'Bearer',
            expiry_date INTEGER NOT NULL,
            email TEXT,
            resource_url TEXT,
            scope TEXT,
            api_key TEXT,
            healthy INTEGER NOT NULL DEFAULT 1,
            health_score REAL NOT NULL DEFAULT 1.0,
            last_used INTEGER NOT NULL DEFAULT 0,
            created_at INTEGER NOT NULL DEFAULT (strftime('%s', 'subsec') * 1000),
            error_count INTEGER NOT NULL DEFAULT 0,
            last_error TEXT,
            proxy_id TEXT,
            updated_at INTEGER NOT NULL DEFAULT (strftime('%s', 'subsec') * 1000),
            FOREIGN KEY (proxy_id) REFERENCES proxy_configs(id) ON DELETE SET NULL
        )
    `); err != nil {
        return fmt.Errorf("failed to create tokens table: %w", err)
    }
    
    // Create provider_settings table
    if _, err := s.db.Exec(`
        CREATE TABLE IF NOT EXISTS provider_settings (
            provider_id TEXT PRIMARY KEY,
            selection_strategy TEXT NOT NULL DEFAULT 'random',
            refresh_buffer_sec INTEGER NOT NULL DEFAULT 1800,
            max_error_count INTEGER NOT NULL DEFAULT 3,
            updated_at INTEGER NOT NULL DEFAULT (strftime('%s', 'subsec') * 1000)
        )
    `); err != nil {
        return fmt.Errorf("failed to create provider_settings table: %w", err)
    }
    
    // Create proxy_configs table
    if _, err := s.db.Exec(`
        CREATE TABLE IF NOT EXISTS proxy_configs (
            id TEXT PRIMARY KEY,
            host TEXT NOT NULL,
            port INTEGER NOT NULL,
            username TEXT,
            password TEXT,
            proxy_type TEXT NOT NULL DEFAULT 'http',
            created_at INTEGER NOT NULL DEFAULT (strftime('%s', 'subsec') * 1000),
            UNIQUE(host, port)
        )
    `); err != nil {
        return fmt.Errorf("failed to create proxy_configs table: %w", err)
    }
    
    // Create schema_migrations table
    if _, err := s.db.Exec(`
        CREATE TABLE IF NOT EXISTS schema_migrations (
            version INTEGER PRIMARY KEY,
            applied_at INTEGER NOT NULL DEFAULT (strftime('%s', 'subsec') * 1000),
            description TEXT NOT NULL
        )
    `); err != nil {
        return fmt.Errorf("failed to create schema_migrations table: %w", err)
    }
    
    // Create indexes for tokens table
    indexes := []string{
        "CREATE INDEX IF NOT EXISTS idx_tokens_provider_id ON tokens(provider_id)",
        "CREATE INDEX IF NOT EXISTS idx_tokens_email ON tokens(email)",
        "CREATE INDEX IF NOT EXISTS idx_tokens_expiry_date ON tokens(expiry_date)",
        "CREATE INDEX IF NOT EXISTS idx_tokens_healthy ON tokens(healthy)",
        "CREATE INDEX IF NOT EXISTS idx_tokens_provider_healthy ON tokens(provider_id, healthy)",
        "CREATE INDEX IF NOT EXISTS idx_tokens_provider_expiry ON tokens(provider_id, expiry_date)",
    }
    
    for _, idx := range indexes {
        if _, err := s.db.Exec(idx); err != nil {
            return fmt.Errorf("failed to create index: %w", err)
        }
    }
    
    // Create index for proxy_configs table
    if _, err := s.db.Exec(
        "CREATE INDEX IF NOT EXISTS idx_proxy_configs_host_port ON proxy_configs(host, port)",
    ); err != nil {
        return fmt.Errorf("failed to create proxy index: %w", err)
    }
    
    // Record migration
    if _, err := s.db.Exec(
        "INSERT INTO schema_migrations (version, description, applied_at) VALUES (1, 'Initial schema', strftime('%s', 'subsec') * 1000))",
    ); err != nil {
        return fmt.Errorf("failed to record migration: %w", err)
    }
    
    return nil
}
```

---

## Performance Considerations

### Query Patterns

| Query Pattern | Frequency | Optimization |
|--------------|----------|-------------|
| **Get all tokens for provider** | High | `idx_tokens_provider_id` index |
| **Get valid tokens for provider** | High | `idx_tokens_provider_healthy` + `idx_tokens_provider_expiry` |
| **Find expiring tokens** | Periodic | `idx_tokens_provider_expiry` index |
| **Find token by email** | Medium | `idx_tokens_email` index |
| **Update token metadata** | High | Primary key lookup |
| **Get provider settings** | High | Primary key lookup |

### Prepared Statements

For optimal performance, prepare commonly used statements:

```go
// Prepared statements for SQLiteStore
type PreparedStatements struct {
    insertToken        *sql.Stmt
    updateToken        *sql.Stmt
    selectTokenByID    *sql.Stmt
    selectTokensByProvider *sql.Stmt
    selectValidTokens  *sql.Stmt
    deleteToken        *sql.Stmt
    selectSettings     *sql.Stmt
    upsertSettings     *sql.Stmt
}
```

### Connection Pooling

```go
// Configure connection pool for optimal performance
db.SetMaxOpenConns(25)    // Maximum open connections
db.SetMaxIdleConns(5)     // Maximum idle connections
db.SetConnMaxLifetime(5 * time.Minute)  // Connection lifetime
```

---

## Backup and Recovery Strategy

### Backup Methods

1. **File Backup**: Copy `tokens.db` file to backup location
2. **SQL Dump**: Use `.dump` command to export SQL
3. **Application Export**: Implement `/api/export` endpoint to export tokens as JSON

### Recovery Procedures

1. **Corruption Detection**: Check database integrity on startup
2. **Automatic Backup**: Create backup before major operations
3. **Rollback Support**: Keep previous version of database before migrations

### Integrity Check

```go
func (s *SQLiteStore) verifyIntegrity() error {
    // Check database file integrity
    var result string
    err := s.db.QueryRow("PRAGMA integrity_check").Scan(&result)
    if err != nil {
        return err
    }
    
    if result != "ok" {
        return fmt.Errorf("database integrity check failed: %s", result)
    }
    
    return nil
}
```

---

## Summary

### Schema Benefits

| Benefit | Description |
|---------|-------------|
| **Single Database** | All tokens in one file, easier backup |
| **Normalized Data** | Proxy configurations deduplicated |
| **Query Performance** | Strategic indexes for fast lookups |
| **Concurrent Access** | WAL mode enables parallel operations |
| **Data Integrity** | Foreign keys and transactions |
| **Extensibility** | Schema versioning for future enhancements |
| **Portability** | Single SQLite file, cross-platform compatible |

### Migration Path

1. **Phase 1**: Add SQLite support alongside file-based storage
2. **Phase 2**: Testing and validation
3. **Phase 3**: Default to SQLite
4. **Phase 4**: Deprecate file-based storage

### Next Steps

1. Implement [`SQLiteStore`](internal/token/sqlite_store.go) with schema v1
2. Implement [`Migrator`](internal/token/migrator.go) for file-to-database migration
3. Update [`config/config.go`](config/config.go) with storage configuration
4. Update [`MultiTokenManager`](internal/token/multi_token_manager.go) for storage backend selection
5. Create comprehensive test suite
6. Document migration process for users

---

## Appendix A: Complete Schema DDL

```sql
-- ============================================================================
-- SQLite Database Schema for qwencoder-proxy
-- Version: 1
-- Description: Initial schema for token storage migration
-- ============================================================================

-- Enable pragmas for performance
PRAGMA journal_mode = WAL;
PRAGMA synchronous = NORMAL;
PRAGMA cache_size = -10240;
PRAGMA foreign_keys = ON;
PRAGMA busy_timeout = 5000;

-- ============================================================================
-- Table: tokens
-- Stores OAuth tokens with metadata for all providers
-- ============================================================================
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

-- Indexes for tokens
CREATE INDEX IF NOT EXISTS idx_tokens_provider_id ON tokens(provider_id);
CREATE INDEX IF NOT EXISTS idx_tokens_email ON tokens(email);
CREATE INDEX IF NOT EXISTS idx_tokens_expiry_date ON tokens(expiry_date);
CREATE INDEX IF NOT EXISTS idx_tokens_healthy ON tokens(healthy);
CREATE INDEX IF NOT EXISTS idx_tokens_provider_healthy ON tokens(provider_id, healthy);
CREATE INDEX IF NOT EXISTS idx_tokens_provider_expiry ON tokens(provider_id, expiry_date);

-- ============================================================================
-- Table: provider_settings
-- Stores provider-specific settings
-- ============================================================================
CREATE TABLE IF NOT EXISTS provider_settings (
    provider_id TEXT PRIMARY KEY,
    selection_strategy TEXT NOT NULL DEFAULT 'random',
    refresh_buffer_sec INTEGER NOT NULL DEFAULT 1800,
    max_error_count INTEGER NOT NULL DEFAULT 3,
    updated_at INTEGER NOT NULL DEFAULT (strftime('%s', 'subsec') * 1000)
);

-- ============================================================================
-- Table: proxy_configs
-- Normalized proxy configurations
-- ============================================================================
CREATE TABLE IF NOT EXISTS proxy_configs (
    id TEXT PRIMARY KEY,
    host TEXT NOT NULL,
    port INTEGER NOT NULL,
    username TEXT,
    password TEXT,
    proxy_type TEXT NOT NULL DEFAULT 'http',
    created_at INTEGER NOT NULL DEFAULT (strftime('%s', 'subsec') * 1000),
    UNIQUE(host, port)
);

-- Index for proxy_configs
CREATE INDEX IF NOT EXISTS idx_proxy_configs_host_port ON proxy_configs(host, port);

-- ============================================================================
-- Table: schema_migrations
-- Tracks applied schema migrations
-- ============================================================================
CREATE TABLE IF NOT EXISTS schema_migrations (
    version INTEGER PRIMARY KEY,
    applied_at INTEGER NOT NULL DEFAULT (strftime('%s', 'subsec') * 1000),
    description TEXT NOT NULL
);

-- ============================================================================
-- Initial Data (if needed)
-- ============================================================================

-- Default provider settings for each provider
INSERT OR IGNORE INTO provider_settings (provider_id, selection_strategy, refresh_buffer_sec, max_error_count, updated_at) VALUES
    ('gemini-cli', 'random', 1800, 3, strftime('%s', 'subsec') * 1000)),
    ('qwen', 'random', 1800, 3, strftime('%s', 'subsec') * 1000)),
    ('iflow', 'random', 1800, 3, strftime('%s', 'subsec') * 1000)),
    ('kiro', 'random', 1800, 3, strftime('%s', 'subsec') * 1000)),
    ('antigravity', 'random', 1800, 3, strftime('%s', 'subsec') * 1000));

-- Record migration
INSERT INTO schema_migrations (version, description, applied_at) VALUES (1, 'Initial schema', strftime('%s', 'subsec') * 1000));
```

This schema provides a solid foundation for migrating from file-based to SQLite storage with support for all current functionality and future enhancements.
