# Database Initialization Strategy - qwencoder-proxy

## Table of Contents

1. [Overview](#overview)
2. [Initialization Flow Diagram](#initialization-flow-diagram)
3. [Storage Backend Detection Logic](#storage-backend-detection-logic)
4. [Database Connection Management](#database-connection-management)
5. [Schema Migration System](#schema-migration-system)
6. [Error Handling and Recovery](#error-handling-and-recovery)
7. [Prepared Statements Architecture](#prepared-statements-architecture)
8. [WAL Mode Management](#wal-mode-management)
9. [Configuration Integration](#configuration-integration)
10. [Startup Performance Optimization](#startup-performance-optimization)
11. [File-to-Database Migration Integration](#file-to-database-migration-integration)
12. [Implementation Checklist](#implementation-checklist)

---

## Overview

This document defines the comprehensive database initialization strategy for the qwencoder-proxy SQLite migration. The strategy covers the complete server startup process with database initialization, including storage backend detection, connection management, schema migrations, error handling, and performance optimization.

### Objectives

1. **Seamless Backend Selection**: Automatically detect and select the appropriate storage backend
2. **Robust Initialization**: Handle all edge cases during database startup
3. **Graceful Degradation**: Fallback to file-based storage on SQLite errors
4. **Performance Optimization**: Minimize startup time through lazy loading and caching
5. **Data Integrity**: Ensure database consistency through proper migration handling

### Key Design Principles

| Principle | Description |
|------------|-------------|
| **Fail-Safe** | Always provide a working storage backend |
| **Zero-Downtime** | Server remains operational during initialization |
| **Backward Compatible** | Existing file-based storage continues to work |
| **Auto-Discovery** | Intelligent detection of existing storage |
| **Configurable** | All behaviors can be overridden via configuration |

---

## Initialization Flow Diagram

### Complete Server Startup Flow

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                           Server Startup                                  │
└────────────────────────────────┬────────────────────────────────────────────────┘
                             │
                             ▼
┌─────────────────────────────────────────────────────────────────────────────────┐
│                      Load Configuration                                   │
│  - Read config file (if exists)                                          │
│  - Load environment variables                                               │
│  - Apply defaults                                                          │
│  - Validate configuration                                                   │
└────────────────────────────────┬────────────────────────────────────────────────┘
                             │
                             ▼
┌─────────────────────────────────────────────────────────────────────────────────┐
│                  Determine Storage Backend                                  │
│                                                                         │
│  ┌──────────────────────────────────────────────────────────────────────────┐  │
│  │  Check config.Storage.Backend                                        │  │
│  └────────────────────────────┬─────────────────────────────────────────┘  │
│                             │                                          │
│         ┌───────────────────┼───────────────────┐                    │
│         │                   │                   │                    │
│         ▼                   ▼                   ▼                    │
│  ┌───────────┐      ┌───────────┐      ┌───────────┐         │
│  │ "sqlite"   │      │  "file"    │      │  "auto"    │         │
│  └─────┬─────┘      └─────┬─────┘      └─────┬─────┘         │
│        │                    │                   │                    │
│        ▼                    ▼                   ▼                    │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │  Initialize SQLiteStore                                      │   │
│  │  - Open database connection                                   │   │
│  │  - Apply pragmas (WAL, synchronous, etc.)                │   │
│  │  - Run schema migrations                                    │   │
│  │  - Validate schema version                                  │   │
│  │  - Prepare statements                                       │   │
│  │  - Start WAL checkpoint goroutine                           │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                         │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │  Initialize FileStore (existing)                             │   │
│  │  - Create .credentials directory                               │   │
│  │  - Load existing token files                                 │   │
│  │  - Initialize file locks                                     │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                         │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │  Auto-Detection Logic                                        │   │
│  │  - Check if .credentials/tokens.db exists?                    │   │
│  │    ├─ Yes → Initialize SQLiteStore                            │   │
│  │    └─ No  → Check if .credentials/ has token files?         │   │
│  │                ├─ Yes → Initialize FileStore + log migration prompt│   │
│  │                └─ No  → Initialize SQLiteStore (new install)  │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
└────────────────────────────────┬────────────────────────────────────────────────┘
                             │
                             ▼
┌─────────────────────────────────────────────────────────────────────────────────┐
│                  Initialize MultiTokenManager                              │
│  - Create TokenManager for each provider                                 │
│  - Initialize RefreshSchedulers                                            │
│  - Start HealthTrackers                                                   │
└────────────────────────────────┬────────────────────────────────────────────────┘
                             │
                             ▼
┌─────────────────────────────────────────────────────────────────────────────────┐
│                  Register Providers                                       │
│  - Create provider instances with token manager injection                     │
│  - Register with HTTP router                                            │
└────────────────────────────────┬────────────────────────────────────────────────┘
                             │
                             ▼
┌─────────────────────────────────────────────────────────────────────────────────┐
│                  Start HTTP Server                                       │
│  - Listen on configured port                                              │
│  - Accept incoming requests                                                │
└─────────────────────────────────────────────────────────────────────────────────┘
```

### Detailed SQLite Initialization Flow

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                  SQLiteStore Initialization                                │
└────────────────────────────────┬────────────────────────────────────────────────┘
                             │
                             ▼
┌─────────────────────────────────────────────────────────────────────────────────┐
│  1. Open Database Connection                                          │
│  - Call sql.Open("sqlite", dbPath)                                    │
│  - Handle special paths (:memory:, :filecache:, etc.)                      │
│  - Configure connection pool settings                                      │
└────────────────────────────────┬────────────────────────────────────────────────┘
                             │
                             ▼
┌─────────────────────────────────────────────────────────────────────────────────┐
│  2. Verify Database Integrity                                         │
│  - Run PRAGMA integrity_check                                            │
│  - Handle corruption if detected                                         │
│  - Attempt recovery if possible                                          │
└────────────────────────────────┬────────────────────────────────────────────────┘
                             │
                             ▼
┌─────────────────────────────────────────────────────────────────────────────────┐
│  3. Apply Database Pragmas                                         │
│  - PRAGMA journal_mode = WAL                                          │
│  - PRAGMA synchronous = NORMAL                                           │
│  - PRAGMA cache_size = -10240                                           │
│  - PRAGMA foreign_keys = ON                                             │
│  - PRAGMA busy_timeout = 5000                                           │
└────────────────────────────────┬────────────────────────────────────────────────┘
                             │
                             ▼
┌─────────────────────────────────────────────────────────────────────────────────┐
│  4. Run Schema Migrations                                           │
│  - Check current schema version                                           │
│  - Apply pending migrations in order                                       │
│  - Record migration completion                                            │
└────────────────────────────────┬────────────────────────────────────────────────┘
                             │
                             ▼
┌─────────────────────────────────────────────────────────────────────────────────┐
│  5. Validate Schema Version                                          │
│  - Ensure expected version is present                                     │
│  - Log warning if version is ahead of application                         │
└────────────────────────────────┬────────────────────────────────────────────────┘
                             │
                             ▼
┌─────────────────────────────────────────────────────────────────────────────────┐
│  6. Prepare Statements (for performance)                              │
│  - Prepare INSERT/UPDATE/SELECT/DELETE statements                        │
│  - Store prepared statements in struct fields                             │
│  - Validate statement preparation                                         │
└────────────────────────────────┬────────────────────────────────────────────────┘
                             │
                             ▼
┌─────────────────────────────────────────────────────────────────────────────────┐
│  7. Start WAL Checkpoint Goroutine (optional)                           │
│  - Launch background goroutine for periodic checkpoints                    │
│  - Configure checkpoint interval                                          │
└────────────────────────────────┬────────────────────────────────────────────────┘
                             │
                             ▼
┌─────────────────────────────────────────────────────────────────────────────────┐
│  8. Return Initialized Store                                          │
└─────────────────────────────────────────────────────────────────────────────────┘
```

---

## Storage Backend Detection Logic

### Backend Types

| Backend | Description | Use Case |
|---------|-------------|-----------|
| `sqlite` | Force SQLite database storage | Explicit configuration |
| `file` | Force file-based storage | Explicit configuration |
| `auto` | Auto-detect appropriate backend | Default behavior |

### Auto-Detection Algorithm

```go
// internal/token/storage_factory.go

type StorageBackend string

const (
    StorageBackendSQLite StorageBackend = "sqlite"
    StorageBackendFile   StorageBackend = "file"
    StorageBackendAuto   StorageBackend = "auto"
)

func DetectStorageBackend(config *config.StorageConfig) StorageBackend {
    // If explicitly configured, use it
    if config.Backend != "" && config.Backend != string(StorageBackendAuto) {
        return StorageBackend(config.Backend)
    }
    
    // Auto-detection logic
    dbPath := getDatabasePath(config.Path)
    credentialsDir := getCredentialsPath(config.Path)
    
    // Check if SQLite database exists
    if fileExists(dbPath) {
        // Verify it's a valid SQLite database
        if isValidSQLiteDatabase(dbPath) {
            return StorageBackendSQLite
        }
        // Invalid database file, warn and fall through
        log.Warn("Invalid SQLite database at %s, falling back to file storage", dbPath)
    }
    
    // Check if file-based storage exists
    if hasTokenFiles(credentialsDir) {
        // Existing file-based installation
        log.Info("Detected existing file-based storage at %s", credentialsDir)
        log.Info("Consider migrating to SQLite for better performance")
        return StorageBackendFile
    }
    
    // New installation - default to SQLite
    log.Info("New installation detected, using SQLite storage")
    return StorageBackendSQLite
}

func fileExists(path string) bool {
    _, err := os.Stat(path)
    return !os.IsNotExist(err)
}

func isValidSQLiteDatabase(path string) bool {
    db, err := sql.Open("sqlite", path)
    if err != nil {
        return false
    }
    defer db.Close()
    
    // Try to query a known table
    var result string
    err = db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' LIMIT 1").Scan(&result)
    return err == nil
}

func hasTokenFiles(dir string) bool {
    entries, err := os.ReadDir(dir)
    if err != nil {
        return false
    }
    
    for _, entry := range entries {
        if entry.IsDir() {
            // Check for token files in provider directory
            providerPath := filepath.Join(dir, entry.Name())
            providerEntries, _ := os.ReadDir(providerPath)
            for _, pe := range providerEntries {
                if !pe.IsDir() && strings.HasSuffix(pe.Name(), ".json") {
                    return true
                }
            }
        }
    }
    return false
}

func getDatabasePath(configPath string) string {
    if configPath != "" {
        return configPath
    }
    return ".credentials/tokens.db"
}

func getCredentialsPath(configPath string) string {
    if configPath != "" {
        return configPath
    }
    return ".credentials"
}
```

### Configuration Precedence

```
1. Environment Variable (STORAGE_BACKEND)
   ↓
2. Configuration File (config.json)
   ↓
3. Default Value ("auto")
```

### Storage Factory Pattern

```go
// internal/token/storage_factory.go

type StorageFactory struct {
    config  *config.Config
    logger  logging.Logger
}

func NewStorageFactory(cfg *config.Config, logger logging.Logger) *StorageFactory {
    return &StorageFactory{
        config: cfg,
        logger:  logger,
    }
}

func (f *StorageFactory) CreateStore(providerID string) (TokenStore, error) {
    backend := DetectStorageBackend(&f.config.Storage)
    
    f.logger.InfoLog("[StorageFactory] Creating store for provider %s using backend: %s", 
        providerID, backend)
    
    switch backend {
    case StorageBackendSQLite:
        return f.createSQLiteStore(providerID)
    case StorageBackendFile:
        return f.createFileStore(providerID)
    default:
        return nil, fmt.Errorf("unknown storage backend: %s", backend)
    }
}

func (f *StorageFactory) createSQLiteStore(providerID string) (*SQLiteStore, error) {
    dbPath := getDatabasePath(f.config.Storage.Path)
    return NewSQLiteStore(dbPath, providerID, f.logger)
}

func (f *StorageFactory) createFileStore(providerID string) (*MultiTokenStore, error) {
    credentialsPath := getCredentialsPath(f.config.Storage.Path)
    return NewMultiTokenStore(credentialsPath, providerID, f.logger)
}
```

---

## Database Connection Management

### Connection Pool Configuration

```go
// internal/token/sqlite_store.go

type ConnectionPoolConfig struct {
    MaxOpenConns        int           // Maximum open connections to database
    MaxIdleConns        int           // Maximum idle connections in pool
    ConnMaxLifetime     time.Duration // Maximum time a connection can be reused
    ConnMaxIdleTime    time.Duration // Maximum time a connection can be idle
}

func DefaultConnectionPoolConfig() ConnectionPoolConfig {
    return ConnectionPoolConfig{
        MaxOpenConns:     25,                 // 25 concurrent connections
        MaxIdleConns:     5,                  // 5 idle connections
        ConnMaxLifetime:  5 * time.Minute,    // Reuse connections for 5 minutes
        ConnMaxIdleTime:  1 * time.Minute,    // Close idle after 1 minute
    }
}

func (s *SQLiteStore) configureConnectionPool(db *sql.DB, cfg ConnectionPoolConfig) {
    db.SetMaxOpenConns(cfg.MaxOpenConns)
    db.SetMaxIdleConns(cfg.MaxIdleConns)
    db.SetConnMaxLifetime(cfg.ConnMaxLifetime)
    db.SetConnMaxIdleTime(cfg.ConnMaxIdleTime)
    
    s.logger.DebugLog("[SQLiteStore] Connection pool configured: MaxOpen=%d, MaxIdle=%d, Lifetime=%v",
        cfg.MaxOpenConns, cfg.MaxIdleConns, cfg.ConnMaxLifetime)
}
```

### Database File Location Handling

| Scenario | Path Resolution | Notes |
|----------|-----------------|-------|
| **Default** | `.credentials/tokens.db` | Relative to working directory |
| **Env Var Set** | `$STORAGE_PATH` | Full or relative path |
| **In-Memory** | `:memory:` | For testing only |
| **File Cache** | `:filecache:?mode=memory` | Temporary in-memory cache |
| **Read-Only** | `file:/path?mode=ro` | For backup/inspection |

```go
func resolveDatabasePath(configPath string) (string, error) {
    if configPath == "" {
        // Default path
        return ".credentials/tokens.db", nil
    }
    
    // Check for special SQLite URIs
    if strings.HasPrefix(configPath, ":") {
        // In-memory or special mode
        return configPath, nil
    }
    
    // Resolve to absolute path
    absPath, err := filepath.Abs(configPath)
    if err != nil {
        return "", fmt.Errorf("failed to resolve database path: %w", err)
    }
    
    // Ensure parent directory exists
    parentDir := filepath.Dir(absPath)
    if err := os.MkdirAll(parentDir, 0755); err != nil {
        return "", fmt.Errorf("failed to create database directory: %w", err)
    }
    
    return absPath, nil
}
```

### In-Memory Database Support

For testing and development:

```go
func NewInMemorySQLiteStore(providerID string, logger logging.Logger) (*SQLiteStore, error) {
    return NewSQLiteStore(":memory:", providerID, logger)
}

// Usage in tests
func TestSQLiteStore(t *testing.T) {
    store, err := NewInMemorySQLiteStore("test-provider", logging.NewLogger())
    require.NoError(t, err)
    defer store.Close()
    
    // Test operations...
}
```

### Connection Health Checks

```go
// internal/token/sqlite_store.go

func (s *SQLiteStore) HealthCheck() error {
    s.mu.RLock()
    defer s.mu.RUnlock()
    
    // Ping the database
    ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
    defer cancel()
    
    err := s.db.PingContext(ctx)
    if err != nil {
        return fmt.Errorf("database ping failed: %w", err)
    }
    
    // Verify database is not locked
    var journalMode string
    err = s.db.QueryRow("PRAGMA journal_mode").Scan(&journalMode)
    if err != nil {
        return fmt.Errorf("failed to check journal mode: %w", err)
    }
    
    // Check integrity (lightweight)
    var integrity string
    err = s.db.QueryRow("PRAGMA quick_check").Scan(&integrity)
    if err != nil {
        return fmt.Errorf("integrity check failed: %w", err)
    }
    
    if integrity != "ok" {
        return fmt.Errorf("database integrity check failed: %s", integrity)
    }
    
    return nil
}

// Periodic health check (optional)
func (s *SQLiteStore) StartHealthCheck(interval time.Duration) {
    ticker := time.NewTicker(interval)
    go func() {
        defer ticker.Stop()
        for range ticker.C {
            if err := s.HealthCheck(); err != nil {
                s.logger.ErrorLog("[SQLiteStore] Health check failed: %v", err)
                // Could trigger recovery or alert
            }
        }
    }()
}
```

---

## Schema Migration System

### Migration Version Tracking

```go
// internal/token/migrations.go

type Migration struct {
    Version     int
    Description string
    Up          func(*SQLiteStore) error
    Down        func(*SQLiteStore) error // Optional rollback
}

type MigrationRegistry struct {
    migrations []Migration
}

func NewMigrationRegistry() *MigrationRegistry {
    return &MigrationRegistry{
        migrations: []Migration{
            {
                Version:     1,
                Description: "Initial schema with tokens, provider_settings, proxy_configs, schema_migrations tables",
                Up:          migrateToV1,
                Down:        rollbackV1,
            },
            // Future migrations...
        },
    }
}

func (r *MigrationRegistry) GetPending(currentVersion int) []Migration {
    var pending []Migration
    for _, m := range r.migrations {
        if m.Version > currentVersion {
            pending = append(pending, m)
        }
    }
    return pending
}
```

### Migration Execution Order

```
1. Get Current Version
   ↓
2. Get Pending Migrations
   ↓
3. For Each Pending Migration:
   ↓
   - Log migration start
   ↓
   - Execute Up migration
   ↓
   - Validate migration success
   ↓
   - Record migration in schema_migrations table
   ↓
   - Log migration complete
   ↓
4. Return success or first error
```

### Migration Execution Implementation

```go
// internal/token/sqlite_store.go

func (s *SQLiteStore) migrate() error {
    // Get current schema version
    currentVersion, err := s.getCurrentSchemaVersion()
    if err != nil {
        return fmt.Errorf("failed to get current schema version: %w", err)
    }
    
    s.logger.InfoLog("[SQLiteStore] Current schema version: %d", currentVersion)
    
    // Get pending migrations
    registry := NewMigrationRegistry()
    pending := registry.GetPending(currentVersion)
    
    if len(pending) == 0 {
        s.logger.InfoLog("[SQLiteStore] Database is up to date")
        return nil
    }
    
    // Execute pending migrations
    for _, m := range pending {
        s.logger.InfoLog("[SQLiteStore] Running migration to version %d: %s", 
            m.Version, m.Description)
        
        // Start transaction for migration
        tx, err := s.db.Begin()
        if err != nil {
            return fmt.Errorf("failed to begin migration transaction: %w", err)
        }
        defer tx.Rollback()
        
        // Execute migration
        if err := m.Up(s); err != nil {
            return fmt.Errorf("migration to v%d failed: %w", m.Version, err)
        }
        
        // Record migration
        if err := s.recordMigration(tx, m); err != nil {
            return fmt.Errorf("failed to record migration: %w", err)
        }
        
        // Commit transaction
        if err := tx.Commit(); err != nil {
            return fmt.Errorf("failed to commit migration: %w", err)
        }
        
        s.logger.InfoLog("[SQLiteStore] Migration to version %d completed", m.Version)
    }
    
    return nil
}

func (s *SQLiteStore) getCurrentSchemaVersion() (int, error) {
    var version int
    err := s.db.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_migrations").Scan(&version)
    if err != nil {
        return 0, fmt.Errorf("failed to query schema version: %w", err)
    }
    return version, nil
}

func (s *SQLiteStore) recordMigration(tx *sql.Tx, m Migration) error {
    _, err := tx.Exec(
        "INSERT INTO schema_migrations (version, description, applied_at) VALUES (?, ?, ?)",
        m.Version, m.Description, time.Now().UnixMilli(),
    )
    return err
}
```

### Rollback Capabilities

```go
// internal/token/sqlite_store.go

func (s *SQLiteStore) RollbackToVersion(targetVersion int) error {
    currentVersion, err := s.getCurrentSchemaVersion()
    if err != nil {
        return err
    }
    
    if currentVersion <= targetVersion {
        return fmt.Errorf("current version %d is already at or below target %d", 
            currentVersion, targetVersion)
    }
    
    registry := NewMigrationRegistry()
    
    // Rollback in reverse order
    for i := len(registry.migrations) - 1; i >= 0; i-- {
        m := registry.migrations[i]
        if m.Version <= targetVersion || m.Version > currentVersion {
            continue
        }
        
        if m.Down == nil {
            return fmt.Errorf("migration v%d does not support rollback", m.Version)
        }
        
        s.logger.WarnLog("[SQLiteStore] Rolling back migration v%d: %s", 
            m.Version, m.Description)
        
        if err := m.Down(s); err != nil {
            return fmt.Errorf("rollback of v%d failed: %w", m.Version, err)
        }
        
        // Remove migration record
        if _, err := s.db.Exec("DELETE FROM schema_migrations WHERE version = ?", m.Version); err != nil {
            return fmt.Errorf("failed to remove migration record: %w", err)
        }
        
        s.logger.InfoLog("[SQLiteStore] Rollback of v%d completed", m.Version)
    }
    
    return nil
}
```

### Migration Logging

```go
// internal/token/migration_logger.go

type MigrationLogger struct {
    logger logging.Logger
}

func NewMigrationLogger(logger logging.Logger) *MigrationLogger {
    return &MigrationLogger{logger: logger}
}

func (l *MigrationLogger) LogMigrationStart(version int, description string) {
    l.logger.InfoLog("[Migration] Starting migration v%d: %s", version, description)
}

func (l *MigrationLogger) LogMigrationComplete(version int, duration time.Duration) {
    l.logger.InfoLog("[Migration] Completed migration v%d in %v", version, duration)
}

func (l *MigrationLogger) LogMigrationError(version int, err error) {
    l.logger.ErrorLog("[Migration] Migration v%d failed: %v", version, err)
}

func (l *MigrationLogger) LogMigrationRollback(version int) {
    l.logger.WarnLog("[Migration] Rolling back migration v%d", version)
}
```

---

## Error Handling and Recovery

### Error Categories

| Category | Description | Recovery Strategy |
|-----------|-------------|-------------------|
| **Database Not Found** | Database file doesn't exist | Create new database |
| **Corruption** | Database file is corrupted | Attempt recovery, fallback to file |
| **Permission Denied** | No write access to database | Log error, exit with clear message |
| **Lock Timeout** | Database is locked | Retry with backoff, increase timeout |
| **Schema Mismatch** | Unexpected schema version | Log warning, attempt migration |
| **Migration Failure** | Migration execution failed | Rollback, fallback to file storage |

### Database Corruption Handling

```go
// internal/token/sqlite_store.go

func (s *SQLiteStore) handleCorruption(dbPath string) error {
    s.logger.ErrorLog("[SQLiteStore] Database corruption detected at %s", dbPath)
    
    // Attempt integrity check with detailed output
    var integrityResult string
    err := s.db.QueryRow("PRAGMA integrity_check").Scan(&integrityResult)
    if err != nil {
        s.logger.ErrorLog("[SQLiteStore] Integrity check failed: %v", err)
    } else {
        s.logger.ErrorLog("[SQLiteStore] Integrity check result: %s", integrityResult)
    }
    
    // Create backup before attempting recovery
    backupPath := dbPath + ".corrupted." + time.Now().Format("20060102-150405")
    if err := copyFile(dbPath, backupPath); err != nil {
        s.logger.ErrorLog("[SQLiteStore] Failed to create backup: %v", err)
    } else {
        s.logger.InfoLog("[SQLiteStore] Created backup at %s", backupPath)
    }
    
    // Attempt to dump and recover
    dumpPath := dbPath + ".dump.sql"
    if err := s.dumpDatabase(dumpPath); err != nil {
        s.logger.ErrorLog("[SQLiteStore] Database dump failed: %v", err)
        return fmt.Errorf("database corrupted and recovery failed: %w", err)
    }
    
    // Recreate database from dump
    if err := os.Remove(dbPath); err != nil {
        return fmt.Errorf("failed to remove corrupted database: %w", err)
    }
    
    if err := s.importFromDump(dumpPath); err != nil {
        return fmt.Errorf("failed to import from dump: %w", err)
    }
    
    s.logger.InfoLog("[SQLiteStore] Database recovered successfully")
    return nil
}

func (s *SQLiteStore) dumpDatabase(dumpPath string) error {
    dump, err := os.Create(dumpPath)
    if err != nil {
        return err
    }
    defer dump.Close()
    
    // Use SQLite's .dump command equivalent
    rows, err := s.db.Query("SELECT sql FROM sqlite_master WHERE sql NOT NULL")
    if err != nil {
        return err
    }
    defer rows.Close()
    
    for rows.Next() {
        var sql string
        if err := rows.Scan(&sql); err != nil {
            return err
        }
        if _, err := dump.WriteString(sql + ";\n"); err != nil {
            return err
        }
    }
    
    return nil
}
```

### Schema Version Mismatch Handling

```go
// internal/token/sqlite_store.go

func (s *SQLiteStore) handleVersionMismatch(currentVersion, expectedVersion int) error {
    s.logger.WarnLog("[SQLiteStore] Schema version mismatch: current=%d, expected=%d",
        currentVersion, expectedVersion)
    
    // If current is ahead, log warning but continue
    if currentVersion > expectedVersion {
        s.logger.WarnLog("[SQLiteStore] Database schema is newer than application expects")
        s.logger.WarnLog("[SQLiteStore] This may indicate a downgrade scenario")
        s.logger.WarnLog("[SQLiteStore] Proceeding with caution")
        return nil
    }
    
    // If current is behind, attempt migration
    s.logger.InfoLog("[SQLiteStore] Attempting to migrate database...")
    if err := s.migrate(); err != nil {
        return fmt.Errorf("migration failed: %w", err)
    }
    
    return nil
}
```

### Fallback to File-Based Storage

```go
// internal/token/storage_factory.go

func (f *StorageFactory) CreateStoreWithFallback(providerID string) (TokenStore, error) {
    // Try SQLite first
    backend := DetectStorageBackend(&f.config.Storage)
    
    if backend == StorageBackendSQLite {
        store, err := f.createSQLiteStore(providerID)
        if err == nil {
            return store, nil
        }
        
        // SQLite initialization failed, log and fallback
        f.logger.ErrorLog("[StorageFactory] SQLite initialization failed: %v", err)
        f.logger.WarnLog("[StorageFactory] Falling back to file-based storage")
        
        // Update configuration to use file storage
        f.config.Storage.Backend = string(StorageBackendFile)
    }
    
    // Create file store
    return f.createFileStore(providerID)
}
```

### Recovery Procedures

#### Procedure 1: Database Lock Timeout

```
1. Detect lock timeout error
   ↓
2. Wait with exponential backoff (100ms, 200ms, 400ms, 800ms)
   ↓
3. Retry operation up to 3 times
   ↓
4. If still failing, increase busy_timeout pragma
   ↓
5. Retry once more
   ↓
6. If still failing, return error
```

#### Procedure 2: Database Corruption Recovery

```
1. Detect corruption (integrity_check fails)
   ↓
2. Create backup of corrupted database
   ↓
3. Attempt SQLite's recovery mode
   ↓
4. If recovery fails:
   ↓
   - Log detailed error
   ↓
   - Fallback to file-based storage
   ↓
   - Notify user of data loss risk
```

#### Procedure 3: Migration Failure Recovery

```
1. Detect migration failure
   ↓
2. Rollback transaction
   ↓
3. Verify database is in consistent state
   ↓
4. Log migration details for debugging
   ↓
5. Fallback to file-based storage
   ↓
6. Notify user to report issue
```

---

## Prepared Statements Architecture

### Statement Preparation Strategy

Prepared statements are prepared once during initialization and reused for all subsequent operations. This provides:

1. **Performance**: SQL parsing and query planning done once
2. **Security**: Automatic SQL injection protection
3. **Consistency**: Same statement used across all operations

### Statement Lifecycle

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                  Statement Lifecycle                                      │
└────────────────────────────────┬────────────────────────────────────────────────┘
                             │
                             ▼
┌─────────────────────────────────────────────────────────────────────────────────┐
│  1. Preparation (during store initialization)                           │
│  - db.Prepare("INSERT INTO tokens (...) VALUES (?, ...)")                │
│  - Validate statement syntax                                             │
│  - Store in struct field                                                 │
└────────────────────────────────┬────────────────────────────────────────────────┘
                             │
                             ▼
┌─────────────────────────────────────────────────────────────────────────────────┐
│  2. Usage (during normal operations)                                   │
│  - stmt.Exec(args...) for INSERT/UPDATE/DELETE                           │
│  - stmt.Query(args...) for SELECT                                        │
│  - stmt.QueryRow(args...) for single row SELECT                             │
└────────────────────────────────┬────────────────────────────────────────────────┘
                             │
                             ▼
┌─────────────────────────────────────────────────────────────────────────────────┐
│  3. Cleanup (during store close)                                      │
│  - stmt.Close()                                                         │
│  - Release resources                                                     │
└─────────────────────────────────────────────────────────────────────────────────┘
```

### Statement Definitions

```go
// internal/token/sqlite_store.go

type SQLiteStore struct {
    db        *sql.DB
    providerID string
    logger    logging.Logger
    mu         sync.RWMutex
    
    // Prepared statements
    stmtInsertToken           *sql.Stmt
    stmtUpdateToken           *sql.Stmt
    stmtSelectTokenByID       *sql.Stmt
    stmtSelectTokensByProvider *sql.Stmt
    stmtSelectValidTokens     *sql.Stmt
    stmtSelectExpiringTokens  *sql.Stmt
    stmtDeleteToken           *sql.Stmt
    stmtUpsertSettings       *sql.Stmt
    stmtSelectSettings       *sql.Stmt
    stmtInsertProxyConfig     *sql.Stmt
    stmtSelectProxyConfig     *sql.Stmt
}

func (s *SQLiteStore) prepareStatements() error {
    var err error
    
    // INSERT token
    s.stmtInsertToken, err = s.db.Prepare(`
        INSERT INTO tokens (
            id, provider_id, access_token, refresh_token, token_type,
            expiry_date, email, resource_url, scope, api_key,
            healthy, health_score, last_used, created_at,
            error_count, last_error, proxy_id
        ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
    `)
    if err != nil {
        return fmt.Errorf("failed to prepare insert token statement: %w", err)
    }
    
    // UPDATE token
    s.stmtUpdateToken, err = s.db.Prepare(`
        UPDATE tokens SET
            access_token = ?, refresh_token = ?, token_type = ?,
            expiry_date = ?, email = ?, resource_url = ?, scope = ?,
            api_key = ?, healthy = ?, health_score = ?,
            last_used = ?, error_count = ?, last_error = ?,
            updated_at = ?
        WHERE provider_id = ? AND id = ?
    `)
    if err != nil {
        return fmt.Errorf("failed to prepare update token statement: %w", err)
    }
    
    // SELECT token by ID
    s.stmtSelectTokenByID, err = s.db.Prepare(`
        SELECT id, access_token, refresh_token, token_type, expiry_date,
               email, resource_url, scope, api_key, healthy,
               health_score, last_used, created_at, error_count,
               last_error, proxy_id, updated_at
        FROM tokens WHERE provider_id = ? AND id = ?
    `)
    if err != nil {
        return fmt.Errorf("failed to prepare select token by ID statement: %w", err)
    }
    
    // SELECT tokens by provider
    s.stmtSelectTokensByProvider, err = s.db.Prepare(`
        SELECT id, access_token, refresh_token, token_type, expiry_date,
               email, resource_url, scope, api_key, healthy,
               health_score, last_used, created_at, error_count,
               last_error, proxy_id, updated_at
        FROM tokens WHERE provider_id = ?
    `)
    if err != nil {
        return fmt.Errorf("failed to prepare select tokens by provider statement: %w", err)
    }
    
    // SELECT valid tokens (healthy and not expired)
    s.stmtSelectValidTokens, err = s.db.Prepare(`
        SELECT id, access_token, refresh_token, token_type, expiry_date,
               email, resource_url, scope, api_key, healthy,
               health_score, last_used, created_at, error_count,
               last_error, proxy_id, updated_at
        FROM tokens 
        WHERE provider_id = ? AND healthy = 1 AND expiry_date > ?
    `)
    if err != nil {
        return fmt.Errorf("failed to prepare select valid tokens statement: %w", err)
    }
    
    // SELECT expiring tokens
    s.stmtSelectExpiringTokens, err = s.db.Prepare(`
        SELECT id, access_token, refresh_token, token_type, expiry_date,
               email, resource_url, scope, api_key, healthy,
               health_score, last_used, created_at, error_count,
               last_error, proxy_id, updated_at
        FROM tokens 
        WHERE provider_id = ? AND expiry_date > ? AND expiry_date < ?
        ORDER BY expiry_date ASC
    `)
    if err != nil {
        return fmt.Errorf("failed to prepare select expiring tokens statement: %w", err)
    }
    
    // DELETE token
    s.stmtDeleteToken, err = s.db.Prepare(`
        DELETE FROM tokens WHERE provider_id = ? AND id = ?
    `)
    if err != nil {
        return fmt.Errorf("failed to prepare delete token statement: %w", err)
    }
    
    // UPSERT settings
    s.stmtUpsertSettings, err = s.db.Prepare(`
        INSERT INTO provider_settings (provider_id, selection_strategy, refresh_buffer_sec, max_error_count, updated_at)
        VALUES (?, ?, ?, ?, ?)
        ON CONFLICT(provider_id) DO UPDATE SET
            selection_strategy = excluded.selection_strategy,
            refresh_buffer_sec = excluded.refresh_buffer_sec,
            max_error_count = excluded.max_error_count,
            updated_at = excluded.updated_at
    `)
    if err != nil {
        return fmt.Errorf("failed to prepare upsert settings statement: %w", err)
    }
    
    // SELECT settings
    s.stmtSelectSettings, err = s.db.Prepare(`
        SELECT provider_id, selection_strategy, refresh_buffer_sec, max_error_count, updated_at
        FROM provider_settings WHERE provider_id = ?
    `)
    if err != nil {
        return fmt.Errorf("failed to prepare select settings statement: %w", err)
    }
    
    // INSERT proxy config
    s.stmtInsertProxyConfig, err = s.db.Prepare(`
        INSERT INTO proxy_configs (id, host, port, username, password, proxy_type, created_at)
        VALUES (?, ?, ?, ?, ?, ?, ?)
        ON CONFLICT(host, port) DO UPDATE SET
            username = excluded.username,
            password = excluded.password,
            proxy_type = excluded.proxy_type
    `)
    if err != nil {
        return fmt.Errorf("failed to prepare insert proxy config statement: %w", err)
    }
    
    // SELECT proxy config
    s.stmtSelectProxyConfig, err = s.db.Prepare(`
        SELECT id, host, port, username, password, proxy_type, created_at
        FROM proxy_configs WHERE host = ? AND port = ?
    `)
    if err != nil {
        return fmt.Errorf("failed to prepare select proxy config statement: %w", err)
    }
    
    return nil
}
```

### Performance Considerations

| Consideration | Recommendation |
|---------------|----------------|
| **Statement Reuse** | Prepare once, use many times |
| **Transaction Bound Statements** | Use `tx.Stmt()` for transaction-specific statements |
| **Parameter Binding** | Use positional parameters (?) for best performance |
| **Statement Caching** | Keep statements in struct fields for lifetime of store |
| **Cleanup** | Always close statements when store is closed |

### Transaction-Specific Statements

```go
// For operations within a transaction, create transaction-specific statements

func (s *SQLiteStore) UpdateMultipleTokens(updates []TokenUpdate) error {
    s.mu.Lock()
    defer s.mu.Unlock()
    
    tx, err := s.db.Begin()
    if err != nil {
        return fmt.Errorf("failed to begin transaction: %w", err)
    }
    defer tx.Rollback()
    
    // Create transaction-specific statement
    stmt, err := tx.Prepare(`
        UPDATE tokens SET
            access_token = ?, refresh_token = ?, expiry_date = ?,
            updated_at = ?
        WHERE provider_id = ? AND id = ?
    `)
    if err != nil {
        return fmt.Errorf("failed to prepare update statement: %w", err)
    }
    defer stmt.Close()
    
    // Execute updates
    for _, u := range updates {
        if _, err := stmt.Exec(
            u.AccessToken, u.RefreshToken, u.ExpiryDate,
            time.Now().UnixMilli(), s.providerID, u.ID,
        ); err != nil {
            return fmt.Errorf("failed to update token %s: %w", u.ID, err)
        }
    }
    
    return tx.Commit()
}
```

---

## WAL Mode Management

### WAL Mode Overview

Write-Ahead Logging (WAL) provides better concurrency by allowing readers and writers to operate simultaneously.

**Benefits:**
- Concurrent reads and writes
- Better performance under load
- Faster disk I/O in most cases

**Trade-offs:**
- Additional WAL file created
- Requires checkpointing to reclaim space
- Slightly more complex recovery

### WAL File Structure

```
.credentials/
├── tokens.db           # Main database file
├── tokens.db-wal      # Write-Ahead Log
└── tokens.db-shm      # Shared memory file
```

### WAL Checkpoint Goroutine

```go
// internal/token/sqlite_store.go

type WALCheckpointConfig struct {
    Interval       time.Duration // Checkpoint interval
    Pages         int           // Pages to checkpoint (0 = automatic)
    Truncate      bool          // Truncate WAL after checkpoint
}

func DefaultWALCheckpointConfig() WALCheckpointConfig {
    return WALCheckpointConfig{
        Interval:  5 * time.Minute,  // Checkpoint every 5 minutes
        Pages:     0,                 // Let SQLite decide
        Truncate:  true,              // Truncate WAL file
    }
}

func (s *SQLiteStore) startWALCheckpoint(cfg WALCheckpointConfig) {
    s.logger.InfoLog("[SQLiteStore] Starting WAL checkpoint goroutine (interval: %v)", cfg.Interval)
    
    ticker := time.NewTicker(cfg.Interval)
    go func() {
        defer ticker.Stop()
        
        for range ticker.C {
            if err := s.runCheckpoint(cfg); err != nil {
                s.logger.ErrorLog("[SQLiteStore] WAL checkpoint failed: %v", err)
            }
        }
    }()
}

func (s *SQLiteStore) runCheckpoint(cfg WALCheckpointConfig) error {
    s.mu.Lock()
    defer s.mu.Unlock()
    
    // Execute checkpoint
    result, err := s.db.QueryRow(
        "PRAGMA wal_checkpoint(TRUNCATE)",
    ).Scan()
    
    if err != nil {
        return fmt.Errorf("checkpoint failed: %w", err)
    }
    
    s.logger.DebugLog("[SQLiteStore] WAL checkpoint completed: %v", result)
    return nil
}

func (s *SQLiteStore) stopWALCheckpoint() {
    // Final checkpoint before closing
    if err := s.runCheckpoint(DefaultWALCheckpointConfig()); err != nil {
        s.logger.WarnLog("[SQLiteStore] Final WAL checkpoint failed: %v", err)
    }
}
```

### WAL File Cleanup

```go
// internal/token/sqlite_store.go

func (s *SQLiteStore) CleanupWALFiles() error {
    dbPath := s.getDatabasePath()
    dir := filepath.Dir(dbPath)
    base := filepath.Base(dbPath)
    
    // Find WAL and SHM files
    walPath := filepath.Join(dir, base+"-wal")
    shmPath := filepath.Join(dir, base+"-shm")
    
    // Checkpoint first to ensure WAL is flushed
    if _, err := s.db.Exec("PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
        return fmt.Errorf("checkpoint before cleanup failed: %w", err)
    }
    
    // Remove WAL file if it exists and is empty/small
    if info, err := os.Stat(walPath); err == nil {
        if info.Size() < 1024 { // Less than 1KB
            if err := os.Remove(walPath); err != nil {
                s.logger.WarnLog("[SQLiteStore] Failed to remove WAL file: %v", err)
            } else {
                s.logger.DebugLog("[SQLiteStore] Removed WAL file: %s", walPath)
            }
        }
    }
    
    // Remove SHM file if it exists
    if err := os.Remove(shmPath); err != nil && !os.IsNotExist(err) {
        s.logger.WarnLog("[SQLiteStore] Failed to remove SHM file: %v", err)
    }
    
    return nil
}
```

### Concurrency Considerations

| Scenario | WAL Behavior | Notes |
|----------|---------------|-------|
| **Single Reader** | Reads from main DB | No WAL needed |
| **Single Writer** | Writes to WAL | Checkpoint merges to main DB |
| **Multiple Readers** | All read from main DB | Concurrent reads allowed |
| **Reader + Writer** | Reader from main, Writer to WAL | No blocking |
| **Multiple Writers** | Serialized via lock | Only one writer at a time |

### WAL Mode Configuration

```go
// internal/token/sqlite_store.go

func (s *SQLiteStore) configureWALMode() error {
    // Enable WAL mode
    var journalMode string
    err := s.db.QueryRow("PRAGMA journal_mode=WAL").Scan(&journalMode)
    if err != nil {
        return fmt.Errorf("failed to set WAL mode: %w", err)
    }
    
    if journalMode != "wal" {
        return fmt.Errorf("WAL mode not enabled, got: %s", journalMode)
    }
    
    s.logger.InfoLog("[SQLiteStore] WAL mode enabled")
    
    // Configure checkpoint behavior
    if _, err := s.db.Exec("PRAGMA wal_autocheckpoint=1000"); err != nil {
        s.logger.WarnLog("[SQLiteStore] Failed to set autocheckpoint: %v", err)
    }
    
    return nil
}
```

---

## Configuration Integration

### Storage Configuration Structure

```go
// config/config.go

type StorageConfig struct {
    Backend string `json:"backend" env:"STORAGE_BACKEND"` // "file", "sqlite", "auto"
    Path    string `json:"path" env:"STORAGE_PATH"`      // Database file path or credentials directory
}

type SQLiteConfig struct {
    ConnectionPool ConnectionPoolConfig `json:"connection_pool"`
    WALCheckpoint  WALCheckpointConfig  `json:"wal_checkpoint"`
    EnableWAL      bool                `json:"enable_wal"`
    CacheSize      int                 `json:"cache_size"` // In pages
    BusyTimeout    int                 `json:"busy_timeout"` // In milliseconds
}

// Extend existing Config struct
type Config struct {
    Server      ServerConfig
    HTTPClient  HTTPClientConfig
    Logging     LoggingConfig
    OAuthServer OAuthServerConfig
    Storage     StorageConfig `json:"storage"`
    SQLite      SQLiteConfig  `json:"sqlite"` // Optional SQLite-specific config
}
```

### Configuration Loading

```go
// config/loader.go

func LoadConfig() *Config {
    config := DefaultConfig()
    
    // Load storage configuration
    if backend := os.Getenv("STORAGE_BACKEND"); backend != "" {
        config.Storage.Backend = backend
    }
    
    if path := os.Getenv("STORAGE_PATH"); path != "" {
        config.Storage.Path = path
    }
    
    // Load SQLite-specific configuration
    if cacheSize := os.Getenv("SQLITE_CACHE_SIZE"); cacheSize != "" {
        if val, err := strconv.Atoi(cacheSize); err == nil {
            config.SQLite.CacheSize = val
        }
    }
    
    if busyTimeout := os.Getenv("SQLITE_BUSY_TIMEOUT"); busyTimeout != "" {
        if val, err := strconv.Atoi(busyTimeout); err == nil {
            config.SQLite.BusyTimeout = val
        }
    }
    
    // ... load other configuration ...
    
    return config
}
```

### Configuration Validation

```go
// config/validator.go

func ValidateStorageConfig(cfg *StorageConfig) error {
    // Validate backend
    validBackends := map[string]bool{
        "file":   true,
        "sqlite": true,
        "auto":   true,
    }
    
    if cfg.Backend != "" && !validBackends[cfg.Backend] {
        return fmt.Errorf("invalid storage backend: %s (must be 'file', 'sqlite', or 'auto')", cfg.Backend)
    }
    
    // Validate path
    if cfg.Path != "" {
        // Check for invalid characters
        if strings.ContainsAny(cfg.Path, []string{"\x00", "\n", "\r"}) {
            return fmt.Errorf("invalid storage path: contains null bytes or newlines")
        }
        
        // Check if path is too long (platform-specific limits)
        if len(cfg.Path) > 4096 {
            return fmt.Errorf("storage path too long: %d characters", len(cfg.Path))
        }
    }
    
    return nil
}
```

### Environment Variable Support

| Variable | Description | Default |
|----------|-------------|----------|
| `STORAGE_BACKEND` | Storage backend type | `auto` |
| `STORAGE_PATH` | Database file or credentials directory | `.credentials/tokens.db` (SQLite) or `.credentials` (file) |
| `SQLITE_CACHE_SIZE` | SQLite cache size in pages | `-10240` (10MB) |
| `SQLITE_BUSY_TIMEOUT` | Lock wait timeout in milliseconds | `5000` (5 seconds) |
| `SQLITE_ENABLE_WAL` | Enable WAL mode | `true` |

### Configuration Precedence

```
Priority 1: Environment Variables
   ↓
Priority 2: Configuration File (config.json)
   ↓
Priority 3: Default Values
```

---

## Startup Performance Optimization

### Lazy Loading Strategies

```go
// internal/token/sqlite_store.go

type SQLiteStore struct {
    db        *sql.DB
    providerID string
    logger    logging.Logger
    mu         sync.RWMutex
    
    // Prepared statements (prepared on first use)
    stmtInsertToken           *sql.Stmt
    stmtUpdateToken           *sql.Stmt
    stmtSelectTokenByID       *sql.Stmt
    // ... other statements
    
    // Lazy initialization flags
    statementsPrepared bool
    walCheckpointStarted bool
}

func (s *SQLiteStore) ensureStatementsPrepared() error {
    s.mu.Lock()
    defer s.mu.Unlock()
    
    if s.statementsPrepared {
        return nil
    }
    
    if err := s.prepareStatements(); err != nil {
        return err
    }
    
    s.statementsPrepared = true
    return nil
}

// Lazy statement preparation in methods
func (s *SQLiteStore) GetToken(tokenID string) (*TokenMetadata, error) {
    if err := s.ensureStatementsPrepared(); err != nil {
        return nil, err
    }
    
    // Use prepared statement...
}
```

### Async Initialization Options

```go
// internal/token/async_initializer.go

type AsyncInitializer struct {
    store      *SQLiteStore
    readyChan  chan struct{}
    errChan    chan error
    logger     logging.Logger
}

func NewAsyncInitializer(store *SQLiteStore, logger logging.Logger) *AsyncInitializer {
    return &AsyncInitializer{
        store:     store,
        readyChan:  make(chan struct{}),
        errChan:    make(chan error, 1),
        logger:     logger,
    }
}

func (a *AsyncInitializer) Start() {
    go func() {
        defer close(a.readyChan)
        
        // Initialize in background
        if err := a.store.migrate(); err != nil {
            a.errChan <- err
            return
        }
        
        if err := a.store.prepareStatements(); err != nil {
            a.errChan <- err
            return
        }
        
        a.logger.InfoLog("[AsyncInitializer] Store initialization completed")
    }()
}

func (a *AsyncInitializer) Wait() error {
    select {
    case <-a.readyChan:
        return <-a.errChan
    case <-time.After(10 * time.Second):
        return fmt.Errorf("initialization timeout")
    }
}

// Usage in main.go
func main() {
    store, _ := NewSQLiteStore(...)
    initializer := NewAsyncInitializer(store, logger)
    initializer.Start()
    
    // Continue with other initialization...
    
    // Wait for store to be ready before serving requests
    if err := initializer.Wait(); err != nil {
        log.Fatal("Store initialization failed: %v", err)
    }
}
```

### Caching Considerations

```go
// internal/token/cache.go

type TokenCache struct {
    tokens    map[string]*TokenMetadata
    expiry    time.Time
    mu        sync.RWMutex
    ttl       time.Duration
}

func NewTokenCache(ttl time.Duration) *TokenCache {
    return &TokenCache{
        tokens: make(map[string]*TokenMetadata),
        ttl:    ttl,
    }
}

func (c *TokenCache) Get(tokenID string) (*TokenMetadata, bool) {
    c.mu.RLock()
    defer c.mu.RUnlock()
    
    // Check if cache is expired
    if time.Now().After(c.expiry) {
        return nil, false
    }
    
    token, ok := c.tokens[tokenID]
    return token, ok
}

func (c *TokenCache) Set(tokenID string, token *TokenMetadata) {
    c.mu.Lock()
    defer c.mu.Unlock()
    
    c.tokens[tokenID] = token
    c.expiry = time.Now().Add(c.ttl)
}

func (c *TokenCache) Invalidate() {
    c.mu.Lock()
    defer c.mu.Unlock()
    
    c.tokens = make(map[string]*TokenMetadata)
}

// Integrate cache with SQLiteStore
type CachedSQLiteStore struct {
    *SQLiteStore
    cache *TokenCache
}

func (s *CachedSQLiteStore) GetToken(tokenID string) (*TokenMetadata, error) {
    // Try cache first
    if token, ok := s.cache.Get(tokenID); ok {
        s.logger.DebugLog("[CachedSQLiteStore] Cache hit for token %s", tokenID)
        return token, nil
    }
    
    // Fall back to database
    token, err := s.SQLiteStore.GetToken(tokenID)
    if err != nil {
        return nil, err
    }
    
    // Update cache
    s.cache.Set(tokenID, token)
    return token, nil
}
```

### Startup Time Optimization

| Optimization | Expected Benefit | Implementation Complexity |
|--------------|-------------------|-------------------------|
| **Lazy Statement Preparation** | 10-20ms faster startup | Low |
| **Async Initialization** | 50-100ms faster perceived startup | Medium |
| **Connection Pool Reuse** | 5-10ms per operation | Low |
| **Schema Version Cache** | 5-10ms per startup | Low |
| **Prepared Statement Caching** | 1-5ms per query | Low |

---

## File-to-Database Migration Integration

### Migration Trigger Conditions

| Condition | Action | User Notification |
|-----------|---------|-------------------|
| **Auto-detection finds file storage** | Log migration prompt | Console message |
| **Explicit migration command** | Start migration immediately | Progress bar |
| **Server started with file storage** | Background migration suggestion | Dashboard notification |

### Migration Progress Tracking

```go
// internal/token/migrator.go

type MigrationProgress struct {
    TotalTokens    int
    MigratedTokens int
    FailedTokens   int
    CurrentToken   string
    StartTime      time.Time
    LastUpdate     time.Time
}

type Migrator struct {
    fileStore  *MultiTokenStore
    dbStore    *SQLiteStore
    logger     logging.Logger
    progress   MigrationProgress
    progressMu sync.RWMutex
    doneChan   chan struct{}
}

func (m *Migrator) Migrate() error {
    m.progress.StartTime = time.Now()
    m.progress.LastUpdate = time.Now()
    
    m.logger.InfoLog("[Migrator] Starting migration from file-based to SQLite storage")
    
    // Load all tokens from file store
    tokens := m.fileStore.ListTokens()
    m.progress.TotalTokens = len(tokens)
    m.updateProgress()
    
    // Migrate each token
    for _, token := range tokens {
        m.progress.CurrentToken = token.ID
        
        if err := m.dbStore.AddToken(token); err != nil {
            m.logger.ErrorLog("[Migrator] Failed to migrate token %s: %v", token.ID, err)
            m.progress.FailedTokens++
        } else {
            m.progress.MigratedTokens++
            m.logger.DebugLog("[Migrator] Migrated token %s (email: %s)", token.ID, token.Email)
        }
        
        m.updateProgress()
    }
    
    // Migrate settings
    settings := m.fileStore.Settings
    if err := m.dbStore.SaveSettings(settings); err != nil {
        return fmt.Errorf("failed to migrate settings: %w", err)
    }
    
    m.logger.InfoLog("[Migrator] Migration completed: %d/%d tokens migrated", 
        m.progress.MigratedTokens, m.progress.TotalTokens)
    
    close(m.doneChan)
    return nil
}

func (m *Migrator) updateProgress() {
    m.progressMu.Lock()
    defer m.progressMu.Unlock()
    
    m.progress.LastUpdate = time.Now()
    
    // Log progress every 10% or every 10 tokens
    if m.progress.MigratedTokens%10 == 0 || 
       m.progress.MigratedTokens*10/m.progress.TotalTokens > m.progress.MigratedTokens*10/m.progress.TotalTokens-1 {
        percent := float64(m.progress.MigratedTokens) / float64(m.progress.TotalTokens) * 100
        m.logger.InfoLog("[Migrator] Progress: %.1f%% (%d/%d tokens)", 
            percent, m.progress.MigratedTokens, m.progress.TotalTokens)
    }
}

func (m *Migrator) GetProgress() MigrationProgress {
    m.progressMu.RLock()
    defer m.progressMu.RUnlock()
    
    return m.progress
}
```

### User Notification

```go
// internal/token/migrator.go

func (m *Migrator) PromptUser() {
    progress := m.GetProgress()
    
    fmt.Println()
    fmt.Println("═════════════════════════════════════════════════════════════")
    fmt.Println("                    STORAGE MIGRATION AVAILABLE")
    fmt.Println("═════════════════════════════════════════════════════════════")
    fmt.Println()
    fmt.Printf("Found %d tokens in file-based storage\n", progress.TotalTokens)
    fmt.Println()
    fmt.Println("Migrating to SQLite will provide:")
    fmt.Println("  • Better performance")
    fmt.Println("  • Improved concurrency")
    fmt.Println("  • Easier backups")
    fmt.Println("  • ACID transactions")
    fmt.Println()
    fmt.Println("To migrate, set STORAGE_BACKEND=sqlite and restart the server")
    fmt.Println("Or run: qwencoder-proxy --migrate")
    fmt.Println()
    fmt.Println("═════════════════════════════════════════════════════════════")
    fmt.Println()
}

// Dashboard notification via REST API
func (m *Migrator) NotifyDashboard() {
    // This would be called from the REST API layer
    progress := m.GetProgress()
    
    notification := map[string]interface{}{
        "type": "migration_available",
        "data": map[string]interface{}{
            "total_tokens":   progress.TotalTokens,
            "can_migrate":   true,
            "current_backend": "file",
            "recommended_backend": "sqlite",
        },
    }
    
    // Send to connected dashboard clients via WebSocket
    // dashboard.BroadcastNotification(notification)
}
```

### Migration Timing

| Scenario | When to Trigger |
|----------|-----------------|
| **Server Startup** | If auto-detection finds file storage |
| **Explicit Command** | When user runs `--migrate` flag |
| **API Request** | When POST to `/api/storage/migrate` |
| **Scheduled** | During low-traffic hours (configurable) |

### Migration Safety

```go
// internal/token/migrator.go

func (m *Migrator) MigrateSafely() error {
    // Create backup before migration
    backupPath := m.createBackup()
    defer m.cleanupBackup(backupPath)
    
    // Attempt migration
    if err := m.Migrate(); err != nil {
        m.logger.ErrorLog("[Migrator] Migration failed: %v", err)
        
        // Restore from backup
        if restoreErr := m.restoreFromBackup(backupPath); restoreErr != nil {
            m.logger.ErrorLog("[Migrator] Backup restore failed: %v", restoreErr)
        }
        
        return fmt.Errorf("migration failed and restored from backup: %w", err)
    }
    
    // Verify migration
    if err := m.verifyMigration(); err != nil {
        m.logger.ErrorLog("[Migrator] Migration verification failed: %v", err)
        return fmt.Errorf("migration verification failed: %w", err)
    }
    
    // Migration successful
    m.logger.InfoLog("[Migrator] Migration completed and verified successfully")
    return nil
}

func (m *Migrator) createBackup() string {
    timestamp := time.Now().Format("20060102-150405")
    backupPath := fmt.Sprintf(".credentials/backup-%s", timestamp)
    
    // Copy entire .credentials directory to backup
    // ...
    
    return backupPath
}

func (m *Migrator) verifyMigration() error {
    // Count tokens in file store
    fileTokens := m.fileStore.ListTokens()
    
    // Count tokens in database
    dbTokens, err := m.dbStore.GetAllTokens()
    if err != nil {
        return err
    }
    
    // Verify counts match
    if len(fileTokens) != len(dbTokens) {
        return fmt.Errorf("token count mismatch: file=%d, db=%d", 
            len(fileTokens), len(dbTokens))
    }
    
    // Verify each token exists
    for _, fileToken := range fileTokens {
        dbToken, err := m.dbStore.GetToken(fileToken.ID)
        if err != nil {
            return fmt.Errorf("token %s not found in database", fileToken.ID)
        }
        
        // Verify key fields match
        if fileToken.Email != dbToken.Email {
            return fmt.Errorf("token %s email mismatch: %s != %s", 
                fileToken.ID, fileToken.Email, dbToken.Email)
        }
    }
    
    return nil
}
```

---

## Implementation Checklist

### Phase 1: Foundation

- [ ] Add `modernc.org/sqlite` dependency to [`go.mod`](go.mod)
- [ ] Create [`internal/token/sqlite_store.go`](internal/token/sqlite_store.go)
- [ ] Implement database connection management
- [ ] Implement schema migration system
- [ ] Implement prepared statements architecture
- [ ] Implement WAL mode management
- [ ] Add unit tests for initialization

### Phase 2: Configuration

- [ ] Update [`config/config.go`](config/config.go) with StorageConfig
- [ ] Add SQLiteConfig for SQLite-specific settings
- [ ] Update [`config/loader.go`](config/loader.go) to load storage config
- [ ] Add configuration validation
- [ ] Add environment variable support
- [ ] Add configuration tests

### Phase 3: Storage Factory

- [ ] Create [`internal/token/storage_factory.go`](internal/token/storage_factory.go)
- [ ] Implement storage backend detection logic
- [ ] Implement auto-detection algorithm
- [ ] Implement fallback to file storage
- [ ] Add factory tests

### Phase 4: Error Handling

- [ ] Implement database corruption handling
- [ ] Implement schema version mismatch handling
- [ ] Implement lock timeout handling
- [ ] Implement recovery procedures
- [ ] Add error handling tests

### Phase 5: Performance

- [ ] Implement lazy statement preparation
- [ ] Implement async initialization option
- [ ] Implement token caching
- [ ] Add performance benchmarks
- [ ] Profile and optimize hot paths

### Phase 6: Migration

- [ ] Create [`internal/token/migrator.go`](internal/token/migrator.go)
- [ ] Implement file-to-database migration
- [ ] Implement progress tracking
- [ ] Implement user notification
- [ ] Implement migration verification
- [ ] Add migration tests

### Phase 7: Integration

- [ ] Update [`cmd/qwencoder-proxy/main.go`](cmd/qwencoder-proxy/main.go) to use storage factory
- [ ] Update [`internal/token/multi_token_manager.go`](internal/token/multi_token_manager.go) for SQLite support
- [ ] Update REST API for migration endpoint
- [ ] Add integration tests
- [ ] Update documentation

### Phase 8: Documentation

- [ ] Update README with storage configuration
- [ ] Create migration guide for users
- [ ] Update API documentation
- [ ] Create troubleshooting guide
- [ ] Add examples

---

## References

### Related Documents

- [`01-current-architecture-analysis.md`](01-current-architecture-analysis.md) - Current architecture overview
- [`02-sqlite-database-schema-design.md`](02-sqlite-database-schema-design.md) - Database schema design
- [`sqlite-migration-plan.md`](../sqlite-migration-plan.md) - Overall migration plan

### External References

- [SQLite Documentation](https://www.sqlite.org/docs.html)
- [Go database/sql Package](https://pkg.go.dev/database/sql)
- [modernc.org/sqlite](https://gitlab.com/cznic/sqlite)
- [SQLite WAL Mode](https://www.sqlite.org/wal.html)

---

## Summary

This database initialization strategy provides a comprehensive approach to initializing the SQLite database for the qwencoder-proxy project. The strategy covers:

1. **Complete Initialization Flow**: From configuration loading to store creation
2. **Storage Backend Detection**: Auto-detection with explicit override support
3. **Connection Management**: Pool configuration, health checks, and special paths
4. **Schema Migration**: Versioned migrations with rollback support
5. **Error Handling**: Comprehensive error recovery and fallback strategies
6. **Prepared Statements**: Performance optimization through statement reuse
7. **WAL Management**: Checkpoint goroutine and file cleanup
8. **Configuration Integration**: Environment variables and validation
9. **Performance Optimization**: Lazy loading, async initialization, caching
10. **Migration Integration**: File-to-database migration with progress tracking

The strategy ensures robust, performant, and user-friendly database initialization that maintains backward compatibility while providing a clear migration path to SQLite.
