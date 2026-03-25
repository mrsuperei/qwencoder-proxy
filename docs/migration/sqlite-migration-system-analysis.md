# SQLite Migration System Analysis

## Executive Summary

This document analyzes the current SQLite migration system in `qwencoder-proxy`, identifies issues with the `type` column warning, and provides recommendations for improvement.

---

## Current Migration System

### Architecture Overview

The current migration system is **embedded in Go code** within [`internal/token/sqlite_store.go`](internal/token/sqlite_store.go:1):

```
sqlite_store.go (1719 lines)
├── Schema constants (lines 19-94)
│   ├── Table names
│   ├── SQL CREATE statements
│   └── Index creation statements
├── Migration system (lines 417-648)
│   ├── migrate() - Main migration runner
│   ├── migrateToV1() - Initial schema
│   ├── migrateToV2() - Add type column
│   └── migrateToV3() - Ensure type column exists
└── Runtime fallback queries (lines 1370-1514)
```

### Migration Flow

```
NewSQLiteStore()
    │
    ├── Open database connection
    ├── Apply pragmas (WAL, cache, etc.)
    ├── migrate()
    │   ├── Read current version from schema_migrations
    │   ├── Run pending migrations
    │   └── Record applied migrations
    └── Prepare statements
```

### Current Migrations

| Version | Description | Status |
|---------|-------------|--------|
| V1 | Initial schema with all tables and indexes | ✅ Implemented |
| V2 | Add type column to proxy_configs table | ✅ Implemented |
| V3 | Ensure type column exists in proxy_configs table | ✅ Implemented |

### Schema Definitions

All schema is defined as Go string constants:

```go
const (
    sqlCreateTokensTable = `CREATE TABLE IF NOT EXISTS tokens (...)`
    sqlCreateProviderSettingsTable = `CREATE TABLE IF NOT EXISTS provider_settings (...)`
    sqlCreateProxyConfigsTable = `CREATE TABLE IF NOT EXISTS proxy_configs (...)`
    sqlCreateSchemaMigrationsTable = `CREATE TABLE IF NOT EXISTS schema_migrations (...)`
)
```

---

## The Type Column Warning Issue

### Problem Description

```
2026/03/25 22:17:33 ⚠️  [WARN] [SQLiteStore] type column missing from proxy_configs table, using fallback query with default type 'http'.
```

### Root Cause Analysis

**The warning occurs because:**

1. **Historical Databases Exist**: There are databases created before the migration system was properly implemented that have a `proxy_configs` table WITHOUT the `type` column.

2. **Runtime Column Checking**: Both [`GetProxy()`](internal/token/sqlite_store.go:1370) and [`ListProxies()`](internal/token/sqlite_store.go:1437) check at runtime if the `type` column exists:

```go
// Check if the type column exists
var hasTypeColumn bool
err := s.db.QueryRow(`
    SELECT COUNT(*) FROM pragma_table_info('proxy_configs')
    WHERE name = 'type'
`).Scan(&hasTypeColumn)

if !hasTypeColumn {
    // Log warning and use fallback query
    query = `SELECT id, 'http' as type, host, port, ...`
}
```

3. **Migration V1 vs V2 Mismatch**: The V1 migration creates `proxy_configs` WITH the `type` column, but existing databases were created with a different schema.

### Why This Happens

```
Timeline:
┌─────────────────────────────────────────────────────────────┐
│ Early Development                                           │
│   proxy_configs table created WITHOUT type column           │
│   (manual creation or old code)                             │
├─────────────────────────────────────────────────────────────┤
│ Migration System Added                                      │
│   V1: Creates proxy_configs WITH type column                │
│   V2: Adds type column (ALTER TABLE)                        │
│   V3: Safety net to ensure type column exists               │
├─────────────────────────────────────────────────────────────┤
│ Current State                                               │
│   Existing databases: NO type column                        │
│   New databases: HAVE type column                           │
│   Runtime checks: Handle both cases                         │
└─────────────────────────────────────────────────────────────┘
```

### Why Migrations Aren't Running

The migrations V2 and V3 should add the `type` column, but they may not be running because:

1. **Migration Version Tracking Issue**: The `schema_migrations` table might not exist or have incorrect version data.

2. **Migration Logic**: The migrations check if the column exists and skip if it does, but for existing databases without the column, they should add it.

3. **Multiple SQLiteStore Instances**: Each provider creates its own `SQLiteStore` instance, and they might be checking migrations at different times.

---

## Problems with Current Approach

### 1. No Separate SQL Files

**Issue**: Schema is hardcoded in Go string constants.

```go
// sqlite_store.go - Lines 33-84
const (
    sqlCreateTokensTable = `
        CREATE TABLE IF NOT EXISTS tokens (
            id TEXT PRIMARY KEY,
            ...
        )
    `
)
```

**Problems**:
- Cannot use external SQL tools (DB Browser for SQLite, etc.)
- Difficult to review schema changes
- Cannot version control schema separately
- No single source of truth for schema

### 2. Migration Logic Mixed with Application Code

**Issue**: Migration functions are methods on `SQLiteStore`.

```go
func (s *SQLiteStore) migrate() error { ... }
func (s *SQLiteStore) migrateToV1() error { ... }
func (s *SQLiteStore) migrateToV2() error { ... }
```

**Problems**:
- Tight coupling between store and migrations
- Cannot test migrations independently
- Cannot run migrations without full store initialization
- Difficult to reuse migrations across different stores

### 3. Runtime Fallback Queries Instead of Proper Migration

**Issue**: Queries check for column existence at runtime.

```go
// Lines 1374-1403
var hasTypeColumn bool
err := s.db.QueryRow(`
    SELECT COUNT(*) FROM pragma_table_info('proxy_configs')
    WHERE name = 'type'
`).Scan(&hasTypeColumn)

if !hasTypeColumn {
    query = `SELECT id, 'http' as type, ...`  // Fallback
}
```

**Problems**:
- Performance overhead (column check on every query)
- Schema inconsistency across databases
- Hidden migration issues
- Difficult to debug schema problems

### 4. No Single Schema File for Fresh Installations

**Issue**: No `schema.sql` file for initial database creation.

**Problems**:
- Cannot create fresh database without running Go code
- Cannot use database initialization tools
- Difficult to set up development environments
- No clear baseline for schema versioning

### 5. Migration Version Tracking is Fragile

**Issue**: Version tracking relies on `schema_migrations` table and manual version reading.

```go
// Lines 426-442
var version int
err := s.db.QueryRow("SELECT MAX(version) FROM schema_migrations").Scan(&version)
if err != nil {
    // Multiple error handling branches
    if strings.Contains(err.Error(), "no such table") {
        version = 0
    } else if strings.Contains(err.Error(), "no such column") {
        version = 0
    } else {
        return fmt.Errorf("failed to get schema version: %w", err)
    }
}
```

**Problems**:
- Complex error handling for edge cases
- Fragile string matching for error detection
- Multiple code paths for same outcome
- Difficult to debug migration state

### 6. No Migration Rollback Support

**Issue**: Migrations only go forward, no rollback mechanism.

**Problems**:
- Cannot undo failed migrations
- Cannot test migration rollback scenarios
- Difficult to recover from migration errors

### 7. No Migration Testing

**Issue**: No dedicated migration tests (only integration tests).

**Problems**:
- Cannot test migrations in isolation
- Cannot verify migration correctness
- Cannot test migration rollback scenarios
- Difficult to catch migration bugs early

---

## Recommendations

### 1. Create Separate SQL Migration Files

**Structure**:
```
internal/token/migrations/
├── 0001_initial_schema.sql
├── 0002_add_proxy_type_column.sql
├── 0003_ensure_proxy_type_column.sql
└── schema.sql  (Combined schema for fresh installs)
```

**Example Migration File**:
```sql
-- 0001_initial_schema.sql
-- Initial schema with all tables and indexes

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
    project_id TEXT,
    healthy INTEGER NOT NULL DEFAULT 1,
    health_score REAL NOT NULL DEFAULT 1.0,
    last_used INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL DEFAULT (strftime('%s', 'subsec') * 1000),
    error_count INTEGER NOT NULL DEFAULT 0,
    last_error TEXT,
    proxy_id TEXT
);

CREATE TABLE IF NOT EXISTS provider_settings (
    provider_id TEXT PRIMARY KEY,
    selection_strategy TEXT NOT NULL DEFAULT 'random',
    refresh_buffer_sec INTEGER NOT NULL DEFAULT 1800,
    max_error_count INTEGER NOT NULL DEFAULT 3,
    updated_at INTEGER NOT NULL DEFAULT (strftime('%s', 'subsec') * 1000)
);

CREATE TABLE IF NOT EXISTS proxy_configs (
    id TEXT PRIMARY KEY,
    type TEXT NOT NULL DEFAULT 'http',
    host TEXT NOT NULL,
    port INTEGER NOT NULL,
    username TEXT,
    password TEXT,
    created_at INTEGER NOT NULL DEFAULT (strftime('%s', 'subsec') * 1000)
);

CREATE TABLE IF NOT EXISTS schema_migrations (
    version INTEGER PRIMARY KEY,
    applied_at INTEGER NOT NULL DEFAULT (strftime('%s', 'subsec') * 1000),
    description TEXT
);

-- Indexes
CREATE INDEX IF NOT EXISTS idx_tokens_provider_id ON tokens(provider_id);
CREATE INDEX IF NOT EXISTS idx_tokens_email ON tokens(email);
CREATE INDEX IF NOT EXISTS idx_tokens_expiry_date ON tokens(expiry_date);
CREATE INDEX IF NOT EXISTS idx_tokens_project_id ON tokens(project_id);
CREATE INDEX IF NOT EXISTS idx_tokens_healthy ON tokens(healthy);
CREATE INDEX IF NOT EXISTS idx_tokens_provider_healthy ON tokens(provider_id, healthy);
CREATE INDEX IF NOT EXISTS idx_tokens_provider_expiry ON tokens(provider_id, expiry_date);
```

### 2. Use a Dedicated Migration Package

**Create `internal/migrate/` package**:

```go
// internal/migrate/migrator.go
package migrate

import (
    "context"
    "database/sql"
    "fmt"
    "io/fs"
    "sort"
    "strings"
    "time"
)

type Migration struct {
    Version     int
    Name        string
    Up          string
    Down        string // Optional rollback
    AppliedAt   time.Time
}

type Migrator struct {
    db         *sql.DB
    migrations []Migration
}

func NewMigrator(db *sql.DB) *Migrator {
    return &Migrator{db: db}
}

// LoadMigrationsFromFS loads migrations from an embedded filesystem
func (m *Migrator) LoadMigrationsFromFS(fsys fs.FS) error {
    entries, err := fs.ReadDir(fsys, ".")
    if err != nil {
        return fmt.Errorf("failed to read migrations directory: %w", err)
    }

    for _, entry := range entries {
        if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
            continue
        }

        content, err := fs.ReadFile(fsys, entry.Name())
        if err != nil {
            return fmt.Errorf("failed to read migration %s: %w", entry.Name(), err)
        }

        // Parse version from filename: 0001_initial_schema.sql
        versionStr := strings.Split(entry.Name(), "_")[0]
        var version int
        _, err = fmt.Sscanf(versionStr, "%d", &version)
        if err != nil {
            return fmt.Errorf("invalid migration filename format: %s", entry.Name())
        }

        name := strings.TrimSuffix(entry.Name(), ".sql")
        name = strings.TrimPrefix(name, versionStr+"_")

        m.migrations = append(m.migrations, Migration{
            Version: version,
            Name:    name,
            Up:      string(content),
        })
    }

    // Sort by version
    sort.Slice(m.migrations, func(i, j int) bool {
        return m.migrations[i].Version < m.migrations[j].Version
    })

    return nil
}

func (m *Migrator) Up(ctx context.Context) error {
    // Create migrations table if not exists
    if _, err := m.db.Exec(`
        CREATE TABLE IF NOT EXISTS schema_migrations (
            version INTEGER PRIMARY KEY,
            name TEXT NOT NULL,
            applied_at INTEGER NOT NULL
        )
    `); err != nil {
        return fmt.Errorf("failed to create migrations table: %w", err)
    }

    // Get current version
    var currentVersion int
    err := m.db.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_migrations").Scan(&currentVersion)
    if err != nil {
        return fmt.Errorf("failed to get current migration version: %w", err)
    }

    // Run pending migrations
    for _, migration := range m.migrations {
        if migration.Version <= currentVersion {
            continue
        }

        if err := m.runMigration(ctx, migration); err != nil {
            return fmt.Errorf("migration %d failed: %w", migration.Version, err)
        }
    }

    return nil
}

func (m *Migrator) runMigration(ctx context.Context, migration Migration) error {
    tx, err := m.db.BeginTx(ctx, nil)
    if err != nil {
        return fmt.Errorf("failed to begin transaction: %w", err)
    }
    defer tx.Rollback()

    // Execute migration
    if _, err := tx.ExecContext(ctx, migration.Up); err != nil {
        return fmt.Errorf("failed to execute migration: %w", err)
    }

    // Record migration
    if _, err := tx.ExecContext(ctx, `
        INSERT INTO schema_migrations (version, name, applied_at)
        VALUES (?, ?, ?)
    `, migration.Version, migration.Name, time.Now().UnixMilli()); err != nil {
        return fmt.Errorf("failed to record migration: %w", err)
    }

    if err := tx.Commit(); err != nil {
        return fmt.Errorf("failed to commit migration: %w", err)
    }

    return nil
}
```

### 3. Embed Migration Files

**Use Go embed**:

```go
// internal/migrate/files.go
package migrate

import "embed"

//go:embed *.sql
var MigrationFS embed.FS
```

**Usage**:

```go
// internal/token/sqlite_store.go
import "qwencoder-proxy/internal/migrate"

func NewSQLiteStore(dbPath, providerID string, logger logging.Logger) (*SQLiteStore, error) {
    // ... open database ...

    migrator := migrate.NewMigrator(db)
    if err := migrator.LoadMigrationsFromFS(migrate.MigrationFS); err != nil {
        return nil, fmt.Errorf("failed to load migrations: %w", err)
    }

    if err := migrator.Up(context.Background()); err != nil {
        return nil, fmt.Errorf("failed to run migrations: %w", err)
    }

    // ... continue with store initialization ...
}
```

### 4. Create Single Schema File for Fresh Installs

**`internal/token/migrations/schema.sql`**:

```sql
-- schema.sql
-- Complete schema for fresh database installation
-- This file combines all migrations into a single schema definition
-- Use this for new database creation

-- Tokens table
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
    project_id TEXT,
    healthy INTEGER NOT NULL DEFAULT 1,
    health_score REAL NOT NULL DEFAULT 1.0,
    last_used INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL DEFAULT (strftime('%s', 'subsec') * 1000),
    error_count INTEGER NOT NULL DEFAULT 0,
    last_error TEXT,
    proxy_id TEXT
);

-- Provider settings table
CREATE TABLE IF NOT EXISTS provider_settings (
    provider_id TEXT PRIMARY KEY,
    selection_strategy TEXT NOT NULL DEFAULT 'random',
    refresh_buffer_sec INTEGER NOT NULL DEFAULT 1800,
    max_error_count INTEGER NOT NULL DEFAULT 3,
    updated_at INTEGER NOT NULL DEFAULT (strftime('%s', 'subsec') * 1000)
);

-- Proxy configs table
CREATE TABLE IF NOT EXISTS proxy_configs (
    id TEXT PRIMARY KEY,
    type TEXT NOT NULL DEFAULT 'http',
    host TEXT NOT NULL,
    port INTEGER NOT NULL,
    username TEXT,
    password TEXT,
    created_at INTEGER NOT NULL DEFAULT (strftime('%s', 'subsec') * 1000)
);

-- Schema migrations table
CREATE TABLE IF NOT EXISTS schema_migrations (
    version INTEGER PRIMARY KEY,
    name TEXT NOT NULL,
    applied_at INTEGER NOT NULL
);

-- Indexes for tokens table
CREATE INDEX IF NOT EXISTS idx_tokens_provider_id ON tokens(provider_id);
CREATE INDEX IF NOT EXISTS idx_tokens_email ON tokens(email);
CREATE INDEX IF NOT EXISTS idx_tokens_expiry_date ON tokens(expiry_date);
CREATE INDEX IF NOT EXISTS idx_tokens_project_id ON tokens(project_id);
CREATE INDEX IF NOT EXISTS idx_tokens_healthy ON tokens(healthy);
CREATE INDEX IF NOT EXISTS idx_tokens_provider_healthy ON tokens(provider_id, healthy);
CREATE INDEX IF NOT EXISTS idx_tokens_provider_expiry ON tokens(provider_id, expiry_date);

-- Mark schema as up-to-date
INSERT OR IGNORE INTO schema_migrations (version, name, applied_at)
VALUES (3, 'initial_schema', (strftime('%s', 'subsec') * 1000));
```

### 5. Remove Runtime Column Checks

**After proper migrations, remove fallback logic**:

```go
// Before (lines 1374-1403)
var hasTypeColumn bool
err := s.db.QueryRow(`
    SELECT COUNT(*) FROM pragma_table_info('proxy_configs')
    WHERE name = 'type'
`).Scan(&hasTypeColumn)

if !hasTypeColumn {
    query = `SELECT id, 'http' as type, ...`  // Fallback
}

// After (simplified)
query := `
    SELECT id, type, host, port, username, password, created_at
    FROM proxy_configs
    WHERE id = ?
`
```

### 6. Add Migration Testing

**`internal/migrate/migrator_test.go`**:

```go
package migrate

import (
    "context"
    "testing"
    "time"

    _ "modernc.org/sqlite"
)

func TestMigrator(t *testing.T) {
    db, err := sql.Open("sqlite", ":memory:")
    if err != nil {
        t.Fatalf("failed to open database: %v", err)
    }
    defer db.Close()

    migrator := NewMigrator(db)
    if err := migrator.LoadMigrationsFromFS(MigrationFS); err != nil {
        t.Fatalf("failed to load migrations: %v", err)
    }

    // Test initial migration
    if err := migrator.Up(context.Background()); err != nil {
        t.Fatalf("failed to run migrations: %v", err)
    }

    // Verify tables exist
    tables := []string{"tokens", "provider_settings", "proxy_configs", "schema_migrations"}
    for _, table := range tables {
        var count int
        err := db.QueryRow(`
            SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?
        `, table).Scan(&count)
        if err != nil {
            t.Errorf("failed to check table %s: %v", table, err)
        }
        if count != 1 {
            t.Errorf("table %s not found", table)
        }
    }

    // Verify migration version
    var version int
    err = db.QueryRow("SELECT MAX(version) FROM schema_migrations").Scan(&version)
    if err != nil {
        t.Fatalf("failed to get migration version: %v", err)
    }
    if version == 0 {
        t.Error("no migrations applied")
    }

    // Test idempotent migration (running again should not fail)
    if err := migrator.Up(context.Background()); err != nil {
        t.Errorf("second migration run failed: %v", err)
    }
}

func TestProxyTypeColumn(t *testing.T) {
    db, err := sql.Open("sqlite", ":memory:")
    if err != nil {
        t.Fatalf("failed to open database: %v", err)
    }
    defer db.Close()

    migrator := NewMigrator(db)
    if err := migrator.LoadMigrationsFromFS(MigrationFS); err != nil {
        t.Fatalf("failed to load migrations: %v", err)
    }

    if err := migrator.Up(context.Background()); err != nil {
        t.Fatalf("failed to run migrations: %v", err)
    }

    // Verify type column exists
    var hasColumn bool
    err = db.QueryRow(`
        SELECT COUNT(*) FROM pragma_table_info('proxy_configs')
        WHERE name = 'type'
    `).Scan(&hasColumn)
    if err != nil {
        t.Fatalf("failed to check type column: %v", err)
    }
    if !hasColumn {
        t.Error("type column not found in proxy_configs table")
    }
}
```

### 7. Add Migration CLI Command

**`cmd/migrate/main.go`**:

```go
package main

import (
    "context"
    "database/sql"
    "flag"
    "fmt"
    "log"

    "qwencoder-proxy/internal/migrate"

    _ "modernc.org/sqlite"
)

func main() {
    dbPath := flag.String("db", ".credentials/tokens.db", "Path to SQLite database")
    action := flag.String("action", "up", "Migration action: up, status")
    flag.Parse()

    db, err := sql.Open("sqlite", *dbPath)
    if err != nil {
        log.Fatalf("failed to open database: %v", err)
    }
    defer db.Close()

    migrator := migrate.NewMigrator(db)
    if err := migrator.LoadMigrationsFromFS(migrate.MigrationFS); err != nil {
        log.Fatalf("failed to load migrations: %v", err)
    }

    switch *action {
    case "up":
        if err := migrator.Up(context.Background()); err != nil {
            log.Fatalf("migration failed: %v", err)
        }
        fmt.Println("Migrations completed successfully")
    case "status":
        status, err := migrator.Status(context.Background())
        if err != nil {
            log.Fatalf("failed to get migration status: %v", err)
        }
        fmt.Printf("Current version: %d\n", status.CurrentVersion)
        fmt.Printf("Latest version: %d\n", status.LatestVersion)
        fmt.Printf("Pending migrations: %d\n", len(status.PendingMigrations))
    default:
        log.Fatalf("unknown action: %s", *action)
    }
}
```

---

## Implementation Plan

### Phase 1: Create Migration Infrastructure

1. Create `internal/migrate/` package
2. Implement `Migrator` type with `Up()` method
3. Add migration file loading from embed.FS
4. Create migration test files

### Phase 2: Extract SQL Migrations

1. Create `internal/token/migrations/` directory
2. Extract V1 migration to `0001_initial_schema.sql`
3. Extract V2 migration to `0002_add_proxy_type_column.sql`
4. Create `schema.sql` for fresh installs
5. Embed migration files

### Phase 3: Update SQLiteStore

1. Replace embedded migration functions with migrator
2. Remove runtime column checks from `GetProxy()` and `ListProxies()`
3. Simplify query logic
4. Update tests

### Phase 4: Fix Existing Databases

1. Create migration tool to fix databases without type column
2. Add diagnostic command to check database schema
3. Provide migration path for users with old databases

### Phase 5: Documentation

1. Update migration documentation
2. Add migration guide for developers
3. Document migration file naming convention
4. Add troubleshooting guide

---

## Summary

### Current State

| Aspect | Current Approach | Problems |
|--------|------------------|----------|
| Schema Definition | Embedded in Go constants | No external SQL tools, hard to review |
| Migration Logic | Methods on SQLiteStore | Tight coupling, hard to test |
| Runtime Checks | Column existence checks | Performance overhead, schema inconsistency |
| Fresh Installs | Run Go code | No schema.sql file |
| Version Tracking | schema_migrations table | Fragile error handling |
| Rollback | Not supported | Cannot undo migrations |
| Testing | Integration tests only | No isolated migration tests |

### Recommended State

| Aspect | Recommended Approach | Benefits |
|--------|---------------------|----------|
| Schema Definition | Separate SQL files | External tools, easy review, version control |
| Migration Logic | Dedicated migrate package | Decoupled, testable, reusable |
| Runtime Checks | None (proper migrations) | Better performance, consistent schema |
| Fresh Installs | schema.sql file | Easy setup, clear baseline |
| Version Tracking | Robust migrator | Simple error handling |
| Rollback | Supported | Recovery from errors |
| Testing | Dedicated migration tests | Early bug detection |

### Next Steps

1. **Immediate**: Fix the type column issue by ensuring migrations run properly
2. **Short-term**: Implement the new migration system
3. **Long-term**: Add migration rollback and advanced features
