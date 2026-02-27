# SQLite Storage Layer Interface and Implementation Plan

## Table of Contents

1. [Overview](#overview)
2. [Interface Design](#interface-design)
3. [SQLiteStore Structure](#sqlitestore-structure)
4. [Core Operations Implementation](#core-operations-implementation)
5. [Settings Management](#settings-management)
6. [Prepared Statements Architecture](#prepared-statements-architecture)
7. [Transaction Support](#transaction-support)
8. [Helper Functions](#helper-functions)
9. [Error Handling](#error-handling)
10. [Performance Considerations](#performance-considerations)
11. [Testing Strategy](#testing-strategy)
12. [Implementation Checklist](#implementation-checklist)

---

## Overview

This document provides a detailed design specification for the SQLite storage layer implementation in the qwencoder-proxy project. It defines the interfaces, data structures, and implementation patterns for replacing the existing file-based storage with SQLite.

### Objectives

1. **Interface Compatibility**: Maintain compatibility with existing [`TokenStore`](../internal/token/store.go) interface
2. **SQLite-Specific Operations**: Add SQLite-specific methods for advanced database operations
3. **Thread Safety**: Ensure all operations are safe for concurrent access
4. **Performance**: Optimize for high-throughput token operations
5. **Maintainability**: Provide clear, well-structured code that follows Go idioms

### Design Principles

| Principle | Description |
|------------|-------------|
| **Interface Segregation** | Small, focused interfaces for specific responsibilities |
| **Dependency Inversion** | Depend on abstractions, not concrete implementations |
| **Single Responsibility** | Each function/method has one clear purpose |
| **Explicit Error Handling** | All errors are checked and properly wrapped with context |
| **Resource Management** | Proper cleanup of database connections and prepared statements |

---

## Interface Design

### Existing TokenStore Interface

The existing [`TokenStore`](../internal/token/store.go) interface defines the contract for token storage operations:

```go
// internal/token/store.go

// TokenMetadata is an alias for ProviderToken, representing token metadata for storage operations.
// This alias enables TokenStore interface to work with existing ProviderToken implementations
// while providing a cleaner abstraction for storage operations.
type TokenMetadata = ProviderToken

// TokenStore defines token storage operations following Dependency Inversion Principle.
// Enables swapping storage implementations (file, database, cloud, in-memory for tests).
type TokenStore interface {
    // Load loads all tokens from storage and returns them mapped by their unique ID.
    Load() (map[string]TokenMetadata, error)

    // Save persists all tokens to storage, keyed by their unique ID.
    Save(tokens map[string]TokenMetadata) error

    // GetCredentialsPath returns the path to the credentials file/directory.
    GetCredentialsPath() string

    // Clear removes all tokens from storage.
    Clear() error
}
```

### Extended SQLiteTokenStore Interface

We extend the base [`TokenStore`](../internal/token/store.go) with SQLite-specific operations:

```go
// internal/token/store.go

// SQLiteTokenStore extends TokenStore with SQLite-specific operations.
// This interface provides access to database-level functionality for advanced use cases
// such as transactions, direct database access, and connection management.
type SQLiteTokenStore interface {
    TokenStore

    // Close closes the database connection and releases all resources.
    // All prepared statements are closed before the connection is closed.
    Close() error

    // BeginTransaction starts a new database transaction.
    // The caller is responsible for calling Commit() or Rollback() on the transaction.
    // Returns an error if a transaction cannot be started.
    BeginTransaction() (*sql.Tx, error)

    // GetDB returns the underlying database connection.
    // This is provided for advanced use cases such as running custom queries.
    // Use with caution - direct database access bypasses store-level locking.
    GetDB() *sql.DB

    // GetToken retrieves a single token by its unique ID.
    // Returns ErrTokenNotFound if the token does not exist.
    GetToken(tokenID string) (*TokenMetadata, error)

    // AddToken adds a new token to the store.
    // If a token with the same refresh_token exists, it will be updated instead.
    // Returns an error if the operation fails.
    AddToken(token TokenMetadata) error

    // UpdateToken updates an existing token using the provided update function.
    // The update function receives a pointer to the token for modification.
    // Returns ErrTokenNotFound if the token does not exist.
    UpdateToken(tokenID string, updateFunc func(*TokenMetadata)) error

    // RemoveToken deletes a token from the store by its unique ID.
    // Returns ErrTokenNotFound if the token does not exist.
    RemoveToken(tokenID string) error

    // GetValidTokens returns all tokens that are healthy and not expired.
    // The expiry threshold is calculated using the provider's refresh_buffer_sec setting.
    // Returns an empty slice if no valid tokens are found.
    GetValidTokens() ([]TokenMetadata, error)

    // LoadSettings loads the provider's settings from the database.
    // Returns default settings if no settings exist for the provider.
    LoadSettings() (StoreSettings, error)

    // SaveSettings persists the provider's settings to the database.
    // Creates or updates the settings record for the provider.
    SaveSettings(settings StoreSettings) error
}
```

### Interface Compatibility with MultiTokenStore

The [`SQLiteStore`](#sqlitestore-structure) implementation will be compatible with the existing [`MultiTokenManager`](../internal/token/multi_token_manager.go) architecture:

```go
// MultiTokenManager uses the TokenStore interface
type MultiTokenManager struct {
    stores map[string]TokenStore  // Can be MultiTokenStore or SQLiteStore
    // ... other fields
}

// Factory function to create the appropriate store
func NewTokenStore(providerID string, config StorageConfig, logger logging.Logger) (TokenStore, error) {
    switch config.Backend {
    case "sqlite":
        return NewSQLiteStore(config.Path, providerID, logger)
    case "file":
        return NewMultiTokenStore(providerID, config.Path, logger), nil
    default:
        return nil, fmt.Errorf("unsupported storage backend: %s", config.Backend)
    }
}
```

### Error Types

Define sentinel errors for common error conditions:

```go
// internal/token/errors.go

package token

import "errors"

// Sentinel errors for token store operations
var (
    // ErrTokenNotFound is returned when a requested token does not exist
    ErrTokenNotFound = errors.New("token not found")

    // ErrSettingsNotFound is returned when provider settings do not exist
    ErrSettingsNotFound = errors.New("settings not found")

    // ErrDuplicateToken is returned when attempting to add a duplicate token
    ErrDuplicateToken = errors.New("duplicate token")

    // ErrInvalidTokenID is returned when an invalid token ID is provided
    ErrInvalidTokenID = errors.New("invalid token ID")

    // ErrDatabaseClosed is returned when operating on a closed database
    ErrDatabaseClosed = errors.New("database connection is closed")

    // ErrTransactionFailed is returned when a database transaction fails
    ErrTransactionFailed = errors.New("transaction failed")
)
```

---

## SQLiteStore Structure

### Complete Struct Definition

```go
// internal/token/sqlite_store.go

package token

import (
    "database/sql"
    "sync"
    "time"

    "github.com/sunbankio/qwencoder-proxy/logging"
    _ "modernc.org/sqlite" // SQLite driver
)

// SQLiteStore implements TokenStore and SQLiteTokenStore interfaces.
// It provides SQLite-backed storage for OAuth tokens and provider settings.
type SQLiteStore struct {
    // Database connection
    db *sql.DB

    // Provider identification
    providerID string

    // Logging
    logger logging.Logger

    // Thread safety
    mu sync.RWMutex

    // Database path (for GetCredentialsPath)
    dbPath string

    // Prepared statements for performance
    // These are prepared once during initialization and reused for all operations
    stmtInsertToken         *sql.Stmt
    stmtUpdateToken         *sql.Stmt
    stmtSelectTokenByID     *sql.Stmt
    stmtSelectTokensByProvider *sql.Stmt
    stmtSelectValidTokens   *sql.Stmt
    stmtDeleteToken         *sql.Stmt
    stmtSelectSettings      *sql.Stmt
    stmtUpsertSettings      *sql.Stmt
    stmtInsertProxyConfig   *sql.Stmt
    stmtSelectProxyConfig   *sql.Stmt
    stmtDeleteProxyConfig   *sql.Stmt

    // Cached settings (optional, for performance)
    cachedSettings     *StoreSettings
    cachedSettingsMu   sync.RWMutex
    settingsCacheTTL  time.Duration
    settingsCacheTime time.Time
}

// NewSQLiteStore creates a new SQLite-backed token store.
//
// Parameters:
//   - dbPath: Path to the SQLite database file (e.g., ".credentials/tokens.db")
//             Use ":memory:" for an in-memory database (useful for testing)
//   - providerID: Provider identifier (e.g., "gemini-cli", "qwen")
//   - logger: Logger instance for diagnostic output
//
// Returns:
//   - *SQLiteStore: Initialized store ready for use
//   - error: Error if database cannot be opened or initialized
//
// The function performs the following initialization steps:
//   1. Opens the database connection
//   2. Configures connection pool settings
//   3. Applies database pragmas (WAL mode, foreign keys, etc.)
//   4. Runs schema migrations
//   5. Prepares all SQL statements
func NewSQLiteStore(dbPath, providerID string, logger logging.Logger) (*SQLiteStore, error) {
    // Open database connection
    db, err := sql.Open("sqlite", dbPath)
    if err != nil {
        return nil, fmt.Errorf("failed to open database: %w", err)
    }

    // Verify connection is working
    if err := db.Ping(); err != nil {
        db.Close()
        return nil, fmt.Errorf("failed to ping database: %w", err)
    }

    // Configure connection pool
    db.SetMaxOpenConns(25)              // Maximum open connections
    db.SetMaxIdleConns(5)               // Maximum idle connections
    db.SetConnMaxLifetime(5 * time.Minute) // Maximum connection lifetime

    // Apply database pragmas
    if err := applyPragmas(db, logger); err != nil {
        db.Close()
        return nil, fmt.Errorf("failed to apply pragmas: %w", err)
    }

    store := &SQLiteStore{
        db:                db,
        providerID:         providerID,
        logger:             logger,
        dbPath:             dbPath,
        settingsCacheTTL:    5 * time.Minute, // Cache settings for 5 minutes
    }

    // Run schema migrations
    if err := store.migrate(); err != nil {
        db.Close()
        return nil, fmt.Errorf("failed to run migrations: %w", err)
    }

    // Prepare statements
    if err := store.prepareStatements(); err != nil {
        db.Close()
        return nil, fmt.Errorf("failed to prepare statements: %w", err)
    }

    logger.InfoLog("[SQLiteStore] Initialized store for provider %s at %s", providerID, dbPath)
    return store, nil
}

// applyPragmas applies SQLite pragmas to optimize database behavior.
func applyPragmas(db *sql.DB, logger logging.Logger) error {
    pragmas := []struct {
        name  string
        value string
    }{
        {"journal_mode", "WAL"},              // Enable Write-Ahead Logging
        {"synchronous", "NORMAL"},             // Balance safety and performance
        {"cache_size", "-10240"},              // 10MB cache size
        {"foreign_keys", "ON"},                // Enable foreign key constraints
        {"busy_timeout", "5000"},             // 5 second busy timeout
        {"temp_store", "MEMORY"},              // Use memory for temporary tables
        {"mmap_size", "268435456"},          // 256MB memory-mapped I/O
    }

    for _, p := range pragmas {
        if _, err := db.Exec(fmt.Sprintf("PRAGMA %s = %s", p.name, p.value)); err != nil {
            return fmt.Errorf("failed to set PRAGMA %s: %w", p.name, err)
        }
        logger.DebugLog("[SQLiteStore] Applied PRAGMA %s = %s", p.name, p.value)
    }

    return nil
}
```

### Connection Management

The database connection pool is configured with the following settings:

| Setting | Value | Rationale |
|----------|--------|-----------|
| `SetMaxOpenConns` | 25 | Limits concurrent connections to prevent resource exhaustion |
| `SetMaxIdleConns` | 5 | Keeps a small pool of idle connections for reuse |
| `SetConnMaxLifetime` | 5 minutes | Recycles connections to prevent long-lived connection issues |

### Thread Safety Mechanisms

The [`SQLiteStore`](#sqlitestore-structure) uses [`sync.RWMutex`](https://pkg.go.dev/sync#RWMutex) for thread safety:

```go
// Read operations acquire read lock (multiple readers allowed)
func (s *SQLiteStore) Load() (map[string]TokenMetadata, error) {
    s.mu.RLock()
    defer s.mu.RUnlock()
    // ... implementation
}

// Write operations acquire write lock (exclusive access)
func (s *SQLiteStore) Save(tokens map[string]TokenMetadata) error {
    s.mu.Lock()
    defer s.mu.Unlock()
    // ... implementation
}
```

**Locking Strategy:**
- Read operations ([`Load()`](#load), [`GetToken()`](#gettoken), [`GetValidTokens()`](#getvalidtokens)) use `RLock()`
- Write operations ([`Save()`](#save), [`AddToken()`](#addtoken), [`UpdateToken()`](#updatetoken), [`RemoveToken()`](#removetoken)) use `Lock()`
- Prepared statements are thread-safe (SQLite guarantees this)
- Settings cache has its own `RWMutex` for independent locking

---

## Core Operations Implementation

### Load

Loads all tokens for the provider from the database.

```go
// Load loads all tokens for the provider from SQLite.
// Returns tokens mapped by their unique ID.
// Implements TokenStore.Load().
func (s *SQLiteStore) Load() (map[string]TokenMetadata, error) {
    s.mu.RLock()
    defer s.mu.RUnlock()

    // Query all tokens for this provider
    rows, err := s.stmtSelectTokensByProvider.Query(s.providerID)
    if err != nil {
        return nil, fmt.Errorf("failed to query tokens: %w", err)
    }
    defer rows.Close()

    // Collect all tokens
    tokens := make(map[string]TokenMetadata)
    for rows.Next() {
        token, err := s.scanToken(rows)
        if err != nil {
            s.logger.WarnLog("[SQLiteStore] Failed to scan token: %v", err)
            continue
        }
        tokens[token.ID] = token
    }

    // Check for iteration errors
    if err := rows.Err(); err != nil {
        return nil, fmt.Errorf("error iterating tokens: %w", err)
    }

    s.logger.DebugLog("[SQLiteStore] Loaded %d tokens for provider %s", len(tokens), s.providerID)
    return tokens, nil
}
```

**SQL Statement:**
```sql
SELECT id, access_token, refresh_token, token_type, expiry_date,
       email, resource_url, scope, api_key, healthy,
       health_score, last_used, created_at, error_count,
       last_error, proxy_id
FROM tokens WHERE provider_id = ?
```

### Save

Persists all tokens to the database in a single transaction.

```go
// Save persists all tokens to SQLite.
// This operation is atomic - either all tokens are saved or none are.
// Implements TokenStore.Save().
func (s *SQLiteStore) Save(tokens map[string]TokenMetadata) error {
    s.mu.Lock()
    defer s.mu.Unlock()

    // Start transaction for atomicity
    tx, err := s.db.Begin()
    if err != nil {
        return fmt.Errorf("failed to begin transaction: %w", err)
    }
    defer tx.Rollback()

    // Delete existing tokens for this provider
    if _, err := tx.Exec("DELETE FROM tokens WHERE provider_id = ?", s.providerID); err != nil {
        return fmt.Errorf("failed to delete existing tokens: %w", err)
    }

    // Insert new tokens
    stmt := tx.Stmt(s.stmtInsertToken)
    defer stmt.Close()

    for _, token := range tokens {
        if err := s.bindToken(stmt, token); err != nil {
            return fmt.Errorf("failed to bind token %s: %w", token.ID, err)
        }
        if _, err := stmt.Exec(); err != nil {
            return fmt.Errorf("failed to insert token %s: %w", token.ID, err)
        }
    }

    // Commit transaction
    if err := tx.Commit(); err != nil {
        return fmt.Errorf("failed to commit transaction: %w", err)
    }

    s.logger.DebugLog("[SQLiteStore] Saved %d tokens for provider %s", len(tokens), s.providerID)
    return nil
}
```

**Transaction Flow:**
1. Begin transaction
2. Delete all existing tokens for the provider
3. Insert all new tokens
4. Commit transaction (or rollback on error)

### AddToken

Adds a single token to the store with duplicate detection.

```go
// AddToken adds a new token to the store.
// If a token with the same refresh_token exists, it will be updated instead.
// Implements SQLiteTokenStore.AddToken().
func (s *SQLiteStore) AddToken(token TokenMetadata) error {
    s.mu.Lock()
    defer s.mu.Unlock()

    // Check for duplicate by refresh_token
    var existingID sql.NullString
    err := s.db.QueryRow(
        "SELECT id FROM tokens WHERE provider_id = ? AND refresh_token = ?",
        s.providerID, token.RefreshToken,
    ).Scan(&existingID)

    if err == nil && existingID.Valid {
        // Duplicate found, update existing token
        s.logger.DebugLog("[SQLiteStore] Updating existing token %s with new data", existingID.String)
        return s.UpdateToken(existingID.String, func(t *TokenMetadata) {
            *t = token
            t.ID = existingID.String // Preserve original ID
        })
    } else if !errors.Is(err, sql.ErrNoRows) {
        // Unexpected error
        return fmt.Errorf("failed to check for duplicate token: %w", err)
    }

    // Handle proxy configuration
    proxyID, err := s.ensureProxyConfig(token.Proxy)
    if err != nil {
        return fmt.Errorf("failed to ensure proxy config: %w", err)
    }

    // Insert new token
    _, err = s.stmtInsertToken.Exec(
        token.ID,
        s.providerID,
        token.AccessToken,
        token.RefreshToken,
        token.TokenType,
        token.ExpiryDate,
        token.Email,
        token.ResourceURL,
        token.Scope,
        token.APIKey,
        boolToInt(token.Healthy),
        token.HealthScore,
        token.LastUsed,
        token.CreatedAt,
        token.ErrorCount,
        nullString(token.LastError),
        nullString(proxyID),
    )

    if err != nil {
        return fmt.Errorf("failed to insert token: %w", err)
    }

    s.logger.DebugLog("[SQLiteStore] Added token %s for provider %s", token.ID, s.providerID)
    return nil
}

// ensureProxyConfig ensures the proxy configuration exists in the database.
// Returns the proxy_id for the configuration.
func (s *SQLiteStore) ensureProxyConfig(proxy *ProxyConfig) (string, error) {
    if proxy == nil {
        return "", nil // No proxy
    }

    // Check if proxy config already exists
    var existingID sql.NullString
    err := s.stmtSelectProxyConfig.QueryRow(proxy.Host, proxy.Port).Scan(&existingID)
    if err == nil && existingID.Valid {
        return existingID.String, nil
    } else if !errors.Is(err, sql.ErrNoRows) {
        return "", fmt.Errorf("failed to query proxy config: %w", err)
    }

    // Insert new proxy config
    proxyID := uuid.New().String()
    _, err = s.stmtInsertProxyConfig.Exec(proxyID, proxy.Host, proxy.Port, proxy.Username, proxy.Password)
    if err != nil {
        return "", fmt.Errorf("failed to insert proxy config: %w", err)
    }

    return proxyID, nil
}
```

**SQL Statement:**
```sql
INSERT INTO tokens (
    id, provider_id, access_token, refresh_token, token_type,
    expiry_date, email, resource_url, scope, api_key,
    healthy, health_score, last_used, created_at,
    error_count, last_error, proxy_id
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
```

### GetToken

Retrieves a single token by its unique ID.

```go
// GetToken retrieves a token by its unique ID.
// Returns ErrTokenNotFound if the token does not exist.
// Implements SQLiteTokenStore.GetToken().
func (s *SQLiteStore) GetToken(tokenID string) (*TokenMetadata, error) {
    s.mu.RLock()
    defer s.mu.RUnlock()

    // Query token by ID and provider
    row := s.stmtSelectTokenByID.QueryRow(s.providerID, tokenID)
    token, err := s.scanTokenRow(row)

    if err != nil {
        if errors.Is(err, sql.ErrNoRows) {
            return nil, ErrTokenNotFound
        }
        return nil, fmt.Errorf("failed to get token %s: %w", tokenID, err)
    }

    return &token, nil
}
```

**SQL Statement:**
```sql
SELECT id, access_token, refresh_token, token_type, expiry_date,
       email, resource_url, scope, api_key, healthy,
       health_score, last_used, created_at, error_count,
       last_error, proxy_id
FROM tokens WHERE provider_id = ? AND id = ?
```

### UpdateToken

Updates an existing token using a callback function.

```go
// UpdateToken updates an existing token using the provided update function.
// The update function receives a pointer to the token for modification.
// Returns ErrTokenNotFound if the token does not exist.
// Implements SQLiteTokenStore.UpdateToken().
func (s *SQLiteStore) UpdateToken(tokenID string, updateFunc func(*TokenMetadata)) error {
    s.mu.Lock()
    defer s.mu.Unlock()

    return s.updateTokenInternal(s.db, tokenID, updateFunc)
}

// updateTokenInternal performs the actual update operation.
// Caller must hold appropriate lock.
func (s *SQLiteStore) updateTokenInternal(db dbExecutor, tokenID string, updateFunc func(*TokenMetadata)) error {
    // Get current token
    row := db.QueryRow(
        "SELECT id, access_token, refresh_token, token_type, expiry_date, "+
            "email, resource_url, scope, api_key, healthy, "+
            "health_score, last_used, created_at, error_count, "+
            "last_error, proxy_id "+
            "FROM tokens WHERE provider_id = ? AND id = ?",
        s.providerID, tokenID,
    )
    token, err := s.scanTokenRow(row)

    if err != nil {
        if errors.Is(err, sql.ErrNoRows) {
            return ErrTokenNotFound
        }
        return fmt.Errorf("failed to get token %s: %w", tokenID, err)
    }

    // Apply update function
    updateFunc(&token)

    // Handle proxy configuration
    proxyID, err := s.ensureProxyConfigTx(db, token.Proxy)
    if err != nil {
        return fmt.Errorf("failed to ensure proxy config: %w", err)
    }

    // Update in database
    _, err = s.stmtUpdateToken.Exec(
        token.AccessToken,
        token.RefreshToken,
        token.TokenType,
        token.ExpiryDate,
        token.Email,
        token.ResourceURL,
        token.Scope,
        token.APIKey,
        boolToInt(token.Healthy),
        token.HealthScore,
        token.LastUsed,
        token.ErrorCount,
        nullString(token.LastError),
        nullString(proxyID),
        s.providerID,
        token.ID,
    )

    if err != nil {
        return fmt.Errorf("failed to update token %s: %w", tokenID, err)
    }

    s.logger.DebugLog("[SQLiteStore] Updated token %s for provider %s", tokenID, s.providerID)
    return nil
}

// ensureProxyConfigTx ensures the proxy configuration exists within a transaction.
func (s *SQLiteStore) ensureProxyConfigTx(db dbExecutor, proxy *ProxyConfig) (string, error) {
    if proxy == nil {
        return "", nil // No proxy
    }

    // Check if proxy config already exists
    var existingID sql.NullString
    err := db.QueryRow(
        "SELECT id FROM proxy_configs WHERE host = ? AND port = ?",
        proxy.Host, proxy.Port,
    ).Scan(&existingID)

    if err == nil && existingID.Valid {
        return existingID.String, nil
    } else if !errors.Is(err, sql.ErrNoRows) {
        return "", fmt.Errorf("failed to query proxy config: %w", err)
    }

    // Insert new proxy config (not in transaction for simplicity)
    return s.ensureProxyConfig(proxy)
}
```

**SQL Statement:**
```sql
UPDATE tokens SET
    access_token = ?, refresh_token = ?, token_type = ?,
    expiry_date = ?, email = ?, resource_url = ?, scope = ?,
    api_key = ?, healthy = ?, health_score = ?,
    last_used = ?, error_count = ?, last_error = ?, proxy_id = ?
WHERE provider_id = ? AND id = ?
```

### RemoveToken

Deletes a token from the store by its unique ID.

```go
// RemoveToken deletes a token from the store by its unique ID.
// Returns ErrTokenNotFound if the token does not exist.
// Implements SQLiteTokenStore.RemoveToken().
func (s *SQLiteStore) RemoveToken(tokenID string) error {
    s.mu.Lock()
    defer s.mu.Unlock()

    result, err := s.stmtDeleteToken.Exec(s.providerID, tokenID)
    if err != nil {
        return fmt.Errorf("failed to delete token %s: %w", tokenID, err)
    }

    // Check if a row was actually deleted
    rowsAffected, err := result.RowsAffected()
    if err != nil {
        return fmt.Errorf("failed to get rows affected: %w", err)
    }

    if rowsAffected == 0 {
        return ErrTokenNotFound
    }

    s.logger.DebugLog("[SQLiteStore] Removed token %s for provider %s", tokenID, s.providerID)
    return nil
}
```

**SQL Statement:**
```sql
DELETE FROM tokens WHERE provider_id = ? AND id = ?
```

### GetValidTokens

Returns all valid (healthy and not expired) tokens.

```go
// GetValidTokens returns all tokens that are healthy and not expired.
// The expiry threshold is calculated using the provider's refresh_buffer_sec setting.
// Returns an empty slice if no valid tokens are found.
// Implements SQLiteTokenStore.GetValidTokens().
func (s *SQLiteStore) GetValidTokens() ([]TokenMetadata, error) {
    s.mu.RLock()
    defer s.mu.RUnlock()

    // Load settings to get refresh_buffer_sec
    settings, err := s.LoadSettings()
    if err != nil {
        return nil, fmt.Errorf("failed to load settings: %w", err)
    }

    // Calculate expiry threshold (current time + buffer)
    bufferMs := int64(settings.RefreshBufferSec * 1000)
    nowMs := time.Now().UnixMilli()
    expiryThreshold := nowMs + bufferMs

    // Query valid tokens
    rows, err := s.stmtSelectValidTokens.Query(s.providerID, expiryThreshold)
    if err != nil {
        return nil, fmt.Errorf("failed to query valid tokens: %w", err)
    }
    defer rows.Close()

    // Collect tokens
    tokens := []TokenMetadata{}
    for rows.Next() {
        token, err := s.scanToken(rows)
        if err != nil {
            s.logger.WarnLog("[SQLiteStore] Failed to scan token: %v", err)
            continue
        }
        tokens = append(tokens, token)
    }

    // Check for iteration errors
    if err := rows.Err(); err != nil {
        return nil, fmt.Errorf("error iterating valid tokens: %w", err)
    }

    s.logger.DebugLog("[SQLiteStore] Found %d valid tokens for provider %s", len(tokens), s.providerID)
    return tokens, nil
}
```

**SQL Statement:**
```sql
SELECT id, access_token, refresh_token, token_type, expiry_date,
       email, resource_url, scope, api_key, healthy,
       health_score, last_used, created_at, error_count,
       last_error, proxy_id
FROM tokens
WHERE provider_id = ? AND healthy = 1 AND expiry_date > ?
ORDER BY health_score DESC, last_used ASC
```

### Clear

Removes all tokens for the provider from the database.

```go
// Clear removes all tokens for the provider from storage.
// Implements TokenStore.Clear().
func (s *SQLiteStore) Clear() error {
    s.mu.Lock()
    defer s.mu.Unlock()

    // Delete all tokens for this provider
    result, err := s.db.Exec("DELETE FROM tokens WHERE provider_id = ?", s.providerID)
    if err != nil {
        return fmt.Errorf("failed to clear tokens: %w", err)
    }

    // Log how many tokens were deleted
    rowsAffected, _ := result.RowsAffected()
    s.logger.InfoLog("[SQLiteStore] Cleared %d tokens for provider %s", rowsAffected, s.providerID)

    return nil
}
```

### GetCredentialsPath

Returns the path to the database file.

```go
// GetCredentialsPath returns the path to the database file.
// Implements TokenStore.GetCredentialsPath().
func (s *SQLiteStore) GetCredentialsPath() string {
    return s.dbPath
}
```

---

## Settings Management

### LoadSettings

Loads the provider's settings from the database.

```go
// LoadSettings loads the provider's settings from the database.
// Returns default settings if no settings exist for the provider.
// Implements SQLiteTokenStore.LoadSettings().
func (s *SQLiteStore) LoadSettings() (StoreSettings, error) {
    // Check cache first
    s.cachedSettingsMu.RLock()
    if s.cachedSettings != nil && time.Since(s.settingsCacheTime) < s.settingsCacheTTL {
        settings := *s.cachedSettings
        s.cachedSettingsMu.RUnlock()
        return settings, nil
    }
    s.cachedSettingsMu.RUnlock()

    // Query settings from database
    var settings StoreSettings
    var selectionStrategy sql.NullString
    var updatedAt sql.NullInt64

    err := s.stmtSelectSettings.QueryRow(s.providerID).Scan(
        &selectionStrategy,
        &settings.RefreshBufferSec,
        &settings.MaxErrorCount,
        &updatedAt,
    )

    if err != nil {
        if errors.Is(err, sql.ErrNoRows) {
            // No settings found, return defaults
            settings = StoreSettings{
                SelectionStrategy: DefaultSelectionStrategy,
                RefreshBufferSec:  DefaultRefreshBufferSec,
                MaxErrorCount:     DefaultMaxErrorCount,
            }
            s.logger.InfoLog("[SQLiteStore] No settings found for provider %s, using defaults", s.providerID)
        } else {
            return StoreSettings{}, fmt.Errorf("failed to load settings: %w", err)
        }
    } else {
        // Parse settings from database
        settings = StoreSettings{
            SelectionStrategy: selectionStrategy.String,
            RefreshBufferSec:  int(settings.RefreshBufferSec),
            MaxErrorCount:     int(settings.MaxErrorCount),
        }
    }

    // Update cache
    s.cachedSettingsMu.Lock()
    s.cachedSettings = &settings
    s.settingsCacheTime = time.Now()
    s.cachedSettingsMu.Unlock()

    return settings, nil
}
```

**SQL Statement:**
```sql
SELECT selection_strategy, refresh_buffer_sec, max_error_count, updated_at
FROM provider_settings
WHERE provider_id = ?
```

### SaveSettings

Persists provider's settings to database.

```go
// SaveSettings persists provider's settings to the database.
// Creates or updates the settings record for the provider.
// Implements SQLiteTokenStore.SaveSettings().
func (s *SQLiteStore) SaveSettings(settings StoreSettings) error {
    s.mu.Lock()
    defer s.mu.Unlock()

    // Update settings in database
    _, err := s.stmtUpsertSettings.Exec(
        settings.SelectionStrategy,
        settings.RefreshBufferSec,
        settings.MaxErrorCount,
        s.providerID,
    )

    if err != nil {
        return fmt.Errorf("failed to save settings: %w", err)
    }

    // Invalidate cache
    s.cachedSettingsMu.Lock()
    s.cachedSettings = nil
    s.cachedSettingsMu.Unlock()

    s.logger.DebugLog("[SQLiteStore] Saved settings for provider %s", s.providerID)
    return nil
}
```

**SQL Statement (UPSERT):**
```sql
INSERT INTO provider_settings (provider_id, selection_strategy, refresh_buffer_sec, max_error_count, updated_at)
VALUES (?, ?, ?, ?, strftime('%s', 'subsec') * 1000)
ON CONFLICT(provider_id) DO UPDATE SET
    selection_strategy = excluded.selection_strategy,
    refresh_buffer_sec = excluded.refresh_buffer_sec,
    max_error_count = excluded.max_error_count,
    updated_at = strftime('%s', 'subsec') * 1000
```

---

## Prepared Statements Architecture

### Statement Preparation Strategy

Prepared statements are created once during initialization and reused for all operations. This provides:

1. **Performance**: SQL parsing and query planning happens once
2. **Security**: Automatic parameter escaping prevents SQL injection
3. **Consistency**: Same query plan used for all executions

### Statement Lifecycle

```
Initialization Phase:
    NewSQLiteStore()
        ↓
    prepareStatements()
        ↓
    Prepare all statements
        ↓
    Statements are ready for use

Operation Phase:
    Each method uses prepared statements
        ↓
    Bind parameters
        ↓
    Execute
        ↓
    Reuse statement for next operation

Cleanup Phase:
    Close()
        ↓
    Close all prepared statements
        ↓
    Close database connection
```

### Statement Preparation

```go
// prepareStatements prepares all SQL statements for reuse.
// This is called once during initialization.
func (s *SQLiteStore) prepareStatements() error {
    var err error

    // Prepare INSERT statement for tokens
    s.stmtInsertToken, err = s.db.Prepare(`
        INSERT INTO tokens (
            id, provider_id, access_token, refresh_token, token_type,
            expiry_date, email, resource_url, scope, api_key,
            healthy, health_score, last_used, created_at,
            error_count, last_error, proxy_id
        ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
    `)
    if err != nil {
        return fmt.Errorf("failed to prepare insert token statement: %w", err)
    }

    // Prepare UPDATE statement for tokens
    s.stmtUpdateToken, err = s.db.Prepare(`
        UPDATE tokens SET
            access_token = ?, refresh_token = ?, token_type = ?,
            expiry_date = ?, email = ?, resource_url = ?, scope = ?,
            api_key = ?, healthy = ?, health_score = ?,
            last_used = ?, error_count = ?, last_error = ?, proxy_id = ?
        WHERE provider_id = ? AND id = ?
    `)
    if err != nil {
        return fmt.Errorf("failed to prepare update token statement: %w", err)
    }

    // Prepare SELECT by ID statement
    s.stmtSelectTokenByID, err = s.db.Prepare(`
        SELECT id, access_token, refresh_token, token_type, expiry_date,
               email, resource_url, scope, api_key, healthy,
               health_score, last_used, created_at, error_count,
               last_error, proxy_id
        FROM tokens WHERE provider_id = ? AND id = ?
    `)
    if err != nil {
        return fmt.Errorf("failed to prepare select by ID statement: %w", err)
    }

    // Prepare SELECT by provider statement
    s.stmtSelectTokensByProvider, err = s.db.Prepare(`
        SELECT id, access_token, refresh_token, token_type, expiry_date,
               email, resource_url, scope, api_key, healthy,
               health_score, last_used, created_at, error_count,
               last_error, proxy_id
        FROM tokens WHERE provider_id = ?
    `)
    if err != nil {
        return fmt.Errorf("failed to prepare select by provider statement: %w", err)
    }

    // Prepare SELECT valid tokens statement
    s.stmtSelectValidTokens, err = s.db.Prepare(`
        SELECT id, access_token, refresh_token, token_type, expiry_date,
               email, resource_url, scope, api_key, healthy,
               health_score, last_used, created_at, error_count,
               last_error, proxy_id
        FROM tokens
        WHERE provider_id = ? AND healthy = 1 AND expiry_date > ?
        ORDER BY health_score DESC, last_used ASC
    `)
    if err != nil {
        return fmt.Errorf("failed to prepare select valid tokens statement: %w", err)
    }

    // Prepare DELETE statement
    s.stmtDeleteToken, err = s.db.Prepare(`
        DELETE FROM tokens WHERE provider_id = ? AND id = ?
    `)
    if err != nil {
        return fmt.Errorf("failed to prepare delete token statement: %w", err)
    }

    // Prepare SELECT settings statement
    s.stmtSelectSettings, err = s.db.Prepare(`
        SELECT selection_strategy, refresh_buffer_sec, max_error_count, updated_at
        FROM provider_settings WHERE provider_id = ?
    `)
    if err != nil {
        return fmt.Errorf("failed to prepare select settings statement: %w", err)
    }

    // Prepare UPSERT settings statement
    s.stmtUpsertSettings, err = s.db.Prepare(`
        INSERT INTO provider_settings (
            provider_id, selection_strategy, refresh_buffer_sec, max_error_count, updated_at
        ) VALUES (?, ?, ?, ?, strftime('%s', 'subsec') * 1000)
        ON CONFLICT(provider_id) DO UPDATE SET
            selection_strategy = excluded.selection_strategy,
            refresh_buffer_sec = excluded.refresh_buffer_sec,
            max_error_count = excluded.max_error_count,
            updated_at = strftime('%s', 'subsec') * 1000
    `)
    if err != nil {
        return fmt.Errorf("failed to prepare upsert settings statement: %w", err)
    }

    // Prepare INSERT proxy config statement
    s.stmtInsertProxyConfig, err = s.db.Prepare(`
        INSERT INTO proxy_configs (id, host, port, username, password)
        VALUES (?, ?, ?, ?, ?)
    `)
    if err != nil {
        return fmt.Errorf("failed to prepare insert proxy config statement: %w", err)
    }

    // Prepare SELECT proxy config statement
    s.stmtSelectProxyConfig, err = s.db.Prepare(`
        SELECT id FROM proxy_configs WHERE host = ? AND port = ?
    `)
    if err != nil {
        return fmt.Errorf("failed to prepare select proxy config statement: %w", err)
    }

    // Prepare DELETE proxy config statement
    s.stmtDeleteProxyConfig, err = s.db.Prepare(`
        DELETE FROM proxy_configs WHERE id = ?
    `)
    if err != nil {
        return fmt.Errorf("failed to prepare delete proxy config statement: %w", err)
    }

    return nil
}
```

### Statement Cleanup

```go
// Close closes the database connection and releases all resources.
// Implements SQLiteTokenStore.Close().
func (s *SQLiteStore) Close() error {
    s.mu.Lock()
    defer s.mu.Unlock()

    // Close prepared statements
    statements := []*sql.Stmt{
        s.stmtInsertToken,
        s.stmtUpdateToken,
        s.stmtSelectTokenByID,
        s.stmtSelectTokensByProvider,
        s.stmtSelectValidTokens,
        s.stmtDeleteToken,
        s.stmtSelectSettings,
        s.stmtUpsertSettings,
        s.stmtInsertProxyConfig,
        s.stmtSelectProxyConfig,
        s.stmtDeleteProxyConfig,
    }

    for _, stmt := range statements {
        if stmt != nil {
            if err := stmt.Close(); err != nil {
                s.logger.WarnLog("[SQLiteStore] Failed to close statement: %v", err)
            }
        }
    }

    // Clear statement references
    s.stmtInsertToken = nil
    s.stmtUpdateToken = nil
    s.stmtSelectTokenByID = nil
    s.stmtSelectTokensByProvider = nil
    s.stmtSelectValidTokens = nil
    s.stmtDeleteToken = nil
    s.stmtSelectSettings = nil
    s.stmtUpsertSettings = nil
    s.stmtInsertProxyConfig = nil
    s.stmtSelectProxyConfig = nil
    s.stmtDeleteProxyConfig = nil

    // Close database connection
    if err := s.db.Close(); err != nil {
        return fmt.Errorf("failed to close database: %w", err)
    }

    s.logger.InfoLog("[SQLiteStore] Closed store for provider %s", s.providerID)
    return nil
}
```

### Transaction-Specific Statements

When using transactions, create transaction-specific statements:

```go
// BeginTransaction starts a new database transaction.
// The caller is responsible for calling Commit() or Rollback() on the transaction.
// Implements SQLiteTokenStore.BeginTransaction().
func (s *SQLiteStore) BeginTransaction() (*sql.Tx, error) {
    s.mu.Lock()
    defer s.mu.Unlock()

    tx, err := s.db.Begin()
    if err != nil {
        return nil, fmt.Errorf("failed to begin transaction: %w", err)
    }

    s.logger.DebugLog("[SQLiteStore] Started transaction for provider %s", s.providerID)
    return tx, nil
}

// Example usage with transaction
func (s *SQLiteStore) BatchUpdateTokens(updates []TokenUpdate) error {
    tx, err := s.BeginTransaction()
    if err != nil {
        return err
    }
    defer tx.Rollback()

    // Create transaction-specific statements
    updateStmt, err := tx.Prepare(`
        UPDATE tokens SET
            access_token = ?, refresh_token = ?, token_type = ?,
            expiry_date = ?, email = ?, resource_url = ?, scope = ?,
            api_key = ?, healthy = ?, health_score = ?,
            last_used = ?, error_count = ?, last_error = ?, proxy_id = ?
        WHERE provider_id = ? AND id = ?
    `)
    if err != nil {
        return fmt.Errorf("failed to prepare update statement: %w", err)
    }
    defer updateStmt.Close()

    // Execute all updates
    for _, update := range updates {
        _, err := updateStmt.Exec(
            update.AccessToken,
            update.RefreshToken,
            update.TokenType,
            update.ExpiryDate,
            update.Email,
            update.ResourceURL,
            update.Scope,
            update.APIKey,
            boolToInt(update.Healthy),
            update.HealthScore,
            update.LastUsed,
            update.ErrorCount,
            nullString(update.LastError),
            nullString(update.ProxyID),
            s.providerID,
            update.ID,
        )
        if err != nil {
            return fmt.Errorf("failed to update token %s: %w", update.ID, err)
        }
    }

    // Commit transaction
    if err := tx.Commit(); err != nil {
        return fmt.Errorf("failed to commit transaction: %w", err)
    }

    return nil
}
```

---

## Transaction Support

### BeginTransaction

Starts a new database transaction.

```go
// BeginTransaction starts a new database transaction.
// The caller is responsible for calling Commit() or Rollback() on the transaction.
// Implements SQLiteTokenStore.BeginTransaction().
func (s *SQLiteStore) BeginTransaction() (*sql.Tx, error) {
    s.mu.Lock()
    defer s.mu.Unlock()

    tx, err := s.db.Begin()
    if err != nil {
        return nil, fmt.Errorf("failed to begin transaction: %w", err)
    }

    s.logger.DebugLog("[SQLiteStore] Started transaction for provider %s", s.providerID)
    return tx, nil
}
```

### Transaction Handling Patterns

#### Pattern 1: Simple Transaction with Rollback

```go
func (s *SQLiteStore) AtomicOperation() error {
    tx, err := s.BeginTransaction()
    if err != nil {
        return err
    }
    defer tx.Rollback() // Rollback if not committed

    // Perform operations
    if _, err := tx.Exec("INSERT INTO ..."); err != nil {
        return err
    }
    if _, err := tx.Exec("UPDATE ..."); err != nil {
        return err
    }

    // Commit transaction
    if err := tx.Commit(); err != nil {
        return fmt.Errorf("failed to commit: %w", err)
    }

    return nil
}
```

#### Pattern 2: Transaction with Error Handling

```go
func (s *SQLiteStore) SafeAtomicOperation() error {
    tx, err := s.BeginTransaction()
    if err != nil {
        return err
    }

    // Perform operations with error handling
    if err := performOperation(tx); err != nil {
        tx.Rollback()
        return fmt.Errorf("operation failed: %w", err)
    }

    // Commit transaction
    if err := tx.Commit(); err != nil {
        return fmt.Errorf("failed to commit: %w", err)
    }

    return nil
}
```

#### Pattern 3: Transaction with Context

```go
func (s *SQLiteStore) ContextualOperation(ctx context.Context) error {
    tx, err := s.db.BeginTx(ctx, nil)
    if err != nil {
        return fmt.Errorf("failed to begin transaction: %w", err)
    }
    defer tx.Rollback()

    // Perform operations with context
    if _, err := tx.ExecContext(ctx, "INSERT INTO ..."); err != nil {
        return err
    }

    // Commit transaction
    if err := tx.Commit(); err != nil {
        return fmt.Errorf("failed to commit: %w", err)
    }

    return nil
}
```

### Rollback Scenarios

Transactions should be rolled back in the following scenarios:

| Scenario | Action |
|----------|--------|
| Query error during transaction | Rollback and return error |
| Constraint violation | Rollback and return error |
| Context cancellation | Rollback and return context error |
| Panic in operation | Defer will rollback |
| Explicit rollback requested | Rollback and return error |

### Transaction Isolation Level

SQLite uses SERIALIZABLE isolation by default, which provides the highest level of isolation. This ensures:

- No dirty reads
- No non-repeatable reads
- No phantom reads

For our use case, SERIALIZABLE is appropriate because:

1. Token operations are relatively short-lived
2. Consistency is more important than concurrency
3. The overhead of SERIALIZABLE is acceptable for our workload

---

## Helper Functions

### Type Conversion Functions

```go
// boolToInt converts a boolean to an integer for SQLite storage.
// SQLite does not have a native boolean type, so we use INTEGER.
func boolToInt(b bool) int {
    if b {
        return 1
    }
    return 0
}

// intToBool converts an integer from SQLite to a boolean.
// Assumes 0 = false, any non-zero value = true.
func intToBool(i int) bool {
    return i != 0
}

// nullString converts a string to sql.NullString.
// Empty strings are stored as NULL in the database.
func nullString(s string) sql.NullString {
    if s == "" {
        return sql.NullString{Valid: false}
    }
    return sql.NullString{String: s, Valid: true}
}

// stringFromNull converts sql.NullString to a string.
// Returns empty string if the value is NULL.
func stringFromNull(ns sql.NullString) string {
    if !ns.Valid {
        return ""
    }
    return ns.String
}
```

### Scan Functions

```go
// scanToken scans a token from a database row.
// This function handles all of type conversions and null handling.
func (s *SQLiteStore) scanToken(scanner interface{ Scan(...interface{}) error }) (TokenMetadata, error) {
    var token TokenMetadata
    var proxyID sql.NullString
    var healthy int
    var lastError sql.NullString

    err := scanner.Scan(
        &token.ID,
        &token.AccessToken,
        &token.RefreshToken,
        &token.TokenType,
        &token.ExpiryDate,
        &token.Email,
        &token.ResourceURL,
        &token.Scope,
        &token.APIKey,
        &healthy,
        &token.HealthScore,
        &token.LastUsed,
        &token.CreatedAt,
        &token.ErrorCount,
        &lastError,
        &proxyID,
    )

    if err != nil {
        return TokenMetadata{}, fmt.Errorf("failed to scan token: %w", err)
    }

    // Convert types
    token.Healthy = intToBool(healthy)
    token.LastError = stringFromNull(lastError)

    // Load proxy configuration if present
    if proxyID.Valid {
        proxy, err := s.loadProxyConfig(proxyID.String)
        if err != nil {
            s.logger.WarnLog("[SQLiteStore] Failed to load proxy config %s: %v", proxyID.String, err)
        } else {
            token.Proxy = proxy
        }
    }

    return token, nil
}

// scanTokenRow scans a token from a *sql.Row.
// This is a convenience wrapper for single-row queries.
func (s *SQLiteStore) scanTokenRow(row *sql.Row) (TokenMetadata, error) {
    return s.scanToken(row)
}

// loadProxyConfig loads a proxy configuration by its ID.
func (s *SQLiteStore) loadProxyConfig(proxyID string) (*ProxyConfig, error) {
    var proxy ProxyConfig
    var username, password sql.NullString

    err := s.db.QueryRow(
        "SELECT host, port, username, password FROM proxy_configs WHERE id = ?",
        proxyID,
    ).Scan(&proxy.Host, &proxy.Port, &username, &password)

    if err != nil {
        if errors.Is(err, sql.ErrNoRows) {
            return nil, nil // Proxy config not found
        }
        return nil, fmt.Errorf("failed to load proxy config: %w", err)
    }

    proxy.Username = stringFromNull(username)
    proxy.Password = stringFromNull(password)

    return &proxy, nil
}
```

### Bind Functions

```go
// bindToken binds a token to a prepared statement.
// This function handles all of type conversions and null handling.
func (s *SQLiteStore) bindToken(stmt *sql.Stmt, token TokenMetadata) error {
    var proxyID sql.NullString
    if token.Proxy != nil {
        proxyID.String = "" // Will be filled by ensureProxyConfig
        proxyID.Valid = false
    }

    _, err := stmt.Exec(
        token.ID,
        s.providerID,
        token.AccessToken,
        token.RefreshToken,
        token.TokenType,
        token.ExpiryDate,
        token.Email,
        token.ResourceURL,
        token.Scope,
        token.APIKey,
        boolToInt(token.Healthy),
        token.HealthScore,
        token.LastUsed,
        token.CreatedAt,
        token.ErrorCount,
        nullString(token.LastError),
        proxyID,
    )

    return err
}
```

### Timestamp Helpers

```go
// Timestamp utilities for working with Unix millisecond timestamps

// NowMs returns the current time as Unix milliseconds.
func NowMs() int64 {
    return time.Now().UnixMilli()
}

// AddMs adds milliseconds to a timestamp.
func AddMs(timestamp int64, durationMs int64) int64 {
    return timestamp + durationMs
}

// IsExpired checks if a token is expired.
func IsTokenExpired(expiryDate int64, bufferSec int) bool {
    bufferMs := int64(bufferSec * 1000)
    nowMs := time.Now().UnixMilli()
    return expiryDate <= (nowMs + bufferMs)
}

// FormatTimestamp formats a Unix millisecond timestamp as a human-readable string.
func FormatTimestamp(timestamp int64) string {
    return time.UnixMilli(timestamp).Format(time.RFC3339)
}
```

### UUID Helpers

```go
// GenerateTokenID generates a new UUID for a token ID.
func GenerateTokenID() string {
    return uuid.New().String()
}

// IsValidTokenID checks if a string is a valid UUID.
func IsValidTokenID(id string) bool {
    _, err := uuid.Parse(id)
    return err == nil
}
```

---

## Error Handling

### Error Types and Categories

```go
// internal/token/errors.go

package token

import (
    "errors"
    "fmt"
)

// Sentinel errors for token store operations
var (
    // ErrTokenNotFound is returned when a requested token does not exist
    ErrTokenNotFound = errors.New("token not found")

    // ErrSettingsNotFound is returned when provider settings do not exist
    ErrSettingsNotFound = errors.New("settings not found")

    // ErrDuplicateToken is returned when attempting to add a duplicate token
    ErrDuplicateToken = errors.New("duplicate token")

    // ErrInvalidTokenID is returned when an invalid token ID is provided
    ErrInvalidTokenID = errors.New("invalid token ID")

    // ErrDatabaseClosed is returned when operating on a closed database
    ErrDatabaseClosed = errors.New("database connection is closed")

    // ErrTransactionFailed is returned when a database transaction fails
    ErrTransactionFailed = errors.New("transaction failed")

    // ErrCorruptedDatabase is returned when the database is corrupted
    ErrCorruptedDatabase = errors.New("database is corrupted")
)

    // ErrMigrationFailed is returned when a database migration fails
    ErrMigrationFailed = errors.New("database migration failed")
)

    // ErrConstraintViolation is returned when a database constraint is violated
    ErrConstraintViolation = errors.New("constraint violation")
)

    // ErrBusy is returned when the database is locked
    ErrBusy = errors.New("database busy")
)

    // ErrInvalidArgument is returned when an invalid argument is provided
    ErrInvalidArgument = errors.New("invalid argument")
)

    // ErrTimeout is returned when an operation times out
    ErrTimeout = errors.New("operation timeout")
)

    // ErrSerialization is returned when data serialization fails
    ErrSerialization = errors.New("serialization failed")
)

    // ErrDeserialization is returned when data deserialization fails
    ErrDeserialization = errors.New("deserialization failed")
)

    // ErrIO is returned when an I/O operation fails
    ErrIO = errors.New("I/O error")
)

    // ErrPermissionDenied is returned when permission is denied
    ErrPermissionDenied = errors.New("permission denied")

    // ErrNotFound is returned when a resource is not found
    ErrNotFound = errors.New("resource not found")

    // ErrAlreadyExists is returned when a resource already exists
    ErrAlreadyExists = errors.New("resource already exists")

    // ErrInvalidState is returned when the store is in an invalid state
    ErrInvalidState = errors.New("invalid state")
)

    // ErrNotImplemented is returned when a feature is not implemented
    ErrNotImplemented = errors.New("not implemented")

    // ErrUnsupported is returned when an operation is not supported
    ErrUnsupported = errors.New("unsupported")
)

    // ErrCancelled is returned when an operation is cancelled
    ErrCancelled = errors.New("operation cancelled")

    // ErrTooManyRequests is returned when rate limit is exceeded
    ErrTooManyRequests = errors.New("too many requests")

    // ErrInternal is returned for internal errors
    ErrInternal = errors.New("internal error")

    // ErrConfiguration is returned for configuration errors
    ErrConfiguration = errors.New("configuration error")

    // ErrValidation is returned for validation errors
    ErrValidation = errors.New("validation error")

    // ErrNetwork is returned for network errors
    ErrNetwork = errors.New("network error")

    // ErrDatabase is returned for database errors
    ErrDatabase = errors.New("database error")

    // ErrFile is returned for file system errors
    ErrFile = errors.New("file error")

    // ErrEncoding is returned for encoding/decoding errors
    ErrEncoding = errors.New("encoding error")

    // ErrDecoding is returned for encoding/decoding errors
    ErrDecoding = errors.New("decoding error")

    // ErrSecurity is returned for security-related errors
    ErrSecurity = errors.New("security error")

    // ErrAuth is returned for authentication errors
    ErrAuth = errors.New("authentication error")

    // ErrAuthorization is returned for authorization errors
    ErrAuthorization = errors.New("authorization error")

    // ErrRateLimitExceeded is returned when rate limit is exceeded
    ErrRateLimitExceeded = errors.New("rate limit exceeded")

    // ErrServiceUnavailable is returned when service is unavailable
    ErrServiceUnavailable = errors.New("service unavailable")

    // ErrMaintenance is returned when service is under maintenance
    ErrMaintenance = errors.New("service under maintenance")

    // ErrGatewayTimeout is returned when gateway times out
    ErrGatewayTimeout = errors.New("gateway timeout")

    // ErrBadGateway is returned when gateway returns an error
    ErrBadGateway = errors.New("bad gateway")

    // ErrGatewayUnavailable is returned when gateway is unavailable
    ErrGatewayUnavailable = errors.New("gateway unavailable")

    // ErrBadData is returned when data is invalid
    ErrBadData = errors.New("bad data")

    // ErrConflict is returned when there is a conflict
    ErrConflict = errors.New("conflict")

    // ErrPreconditionFailed is returned when a precondition fails
    ErrPreconditionFailed = errors.New("precondition failed")

    // ErrUnprocessableEntity is returned when entity cannot be processed
    ErrUnprocessableEntity = errors.New("unprocessable entity")

    // ErrLocked is returned when resource is locked
    ErrLocked = errors.New("resource locked")

    // ErrFailedDependency is returned when a dependency fails
    ErrFailedDependency = errors.New("failed dependency")

    // ErrTooEarly is returned when request is too early
    ErrTooEarly = errors.New("too early")

    // ErrTooLate is returned when request is too late
    ErrTooLate = errors.New("too late")

    // ErrRequestEntityTooLarge is returned when request entity is too large
    ErrRequestEntityTooLarge = errors.New("request entity too large")

    // ErrRequestUriTooLong is returned when request URI is too long
    ErrRequestUriTooLong = errors.New("request URI too long")

    // ErrUnsupportedMediaType is returned when media type is not supported
    ErrUnsupportedMediaType = errors.New("unsupported media type")

    // ErrRequestRangeNotSatisfiable is returned when range is not satisfiable
    ErrRequestRangeNotSatisfiable = errors.New("request range not satisfiable")

    // ErrExpectationFailed is returned when expectation fails
    ErrExpectationFailed = errors.New("expectation failed")

    // ErrImATeapot is returned when server is a teapot
    ErrImATeapot = errors.New("I'm a teapot")

    // ErrMisdirected is returned when request is misdirected
    ErrMisdirected = errors.New("misdirected")

    // ErrUseProxy is returned when proxy is required
    ErrUseProxy = errors.New("use proxy")

    // ErrUpgradeRequired is returned when upgrade is required
    ErrUpgradeRequired = errors.New("upgrade required")

    // ErrPreconditionRequired is returned when precondition is required
    ErrPreconditionRequired = errors.New("precondition required")

    // ErrTooManyRequests is returned when there are too many requests
    ErrTooManyRequests = errors.New("too many requests")

    // ErrRequestHeaderFieldsTooLarge is returned when header fields are too large
    ErrRequestHeaderFieldsTooLarge = errors.New("request header fields too large")

    // ErrUnavailableForLegalReasons is returned when unavailable for legal reasons
    ErrUnavailableForLegalReasons = errors.New("unavailable for legal reasons")

    // ErrInvalidCertificate is returned when certificate is invalid
    ErrInvalidCertificate = errors.New("invalid certificate")

    // ErrIllegalOperation is returned when operation is illegal
    ErrIllegalOperation = errors.New("illegal operation")

    // ErrOperationTimedOut is returned when operation times out
    ErrOperationTimedOut = errors.New("operation timed out")

    // ErrInvalidResponse is returned when response is invalid
    ErrInvalidResponse = errors.New("invalid response")

    // ErrBadGateway is returned when gateway returns bad response
    ErrBadGateway = errors.New("bad gateway")

    // ErrServiceUnavailable is returned when service is unavailable
    ErrServiceUnavailable = errors.New("service unavailable")

    // ErrGatewayTimeout is returned when gateway times out
    ErrGatewayTimeout = errors.New("gateway timeout")

    // ErrVersionNotSupported is returned when version is not supported
    ErrVersionNotSupported = errors.New("version not supported")

    // ErrVariantAlsoNegotiates is returned when variant also negotiates
    ErrVariantAlsoNegotiates = errors.New("variant also negotiates")

    // ErrNotExtended is returned when extension is not supported
    ErrNotExtended = errors.New("not extended")

    // ErrInsufficientStorage is returned when storage is insufficient
    ErrInsufficientStorage = errors.New("insufficient storage")

    // ErrLoopDetected is returned when loop is detected
    ErrLoopDetected = errors.New("loop detected")

    // ErrNotModified is returned when resource is not modified
    ErrNotModified = errors.New("not modified")

    // ErrBadRequest is returned when request is bad
    ErrBadRequest = errors.New("bad request")

    // ErrUnauthorized is returned when request is unauthorized
    ErrUnauthorized = errors.New("unauthorized")

    // ErrPaymentRequired is returned when payment is required
    ErrPaymentRequired = errors.New("payment required")

    // ErrForbidden is returned when access is forbidden
    ErrForbidden = errors.New("forbidden")

    // ErrNotFound is returned when resource is not found
    ErrNotFound = errors.New("not found")

    ErrMethodNotAllowed is returned when method is not allowed
    ErrMethodNotAllowed = errors.New("method not allowed")

    ErrNotAcceptable is returned when request is not acceptable
    ErrNotAcceptable = errors.New("not acceptable")

    ErrProxyAuthRequired is returned when proxy authentication is required
    ErrProxyAuthRequired = errors.New("proxy authentication required")

    ErrRequestTimeout is returned when request times out
    ErrRequestTimeout = errors.New("request timeout")

    ErrConflict is returned when there is a conflict
    ErrConflict = errors.New("conflict")

    ErrGone is returned when resource is gone
    ErrGone = errors.New("gone")

    ErrLengthRequired is returned when length is required
    ErrLengthRequired = errors.New("length required")

    ErrPreconditionFailed is returned when precondition fails
    ErrPreconditionFailed = errors.New("precondition failed")

    ErrRequestEntityTooLarge is returned when entity is too large
    ErrRequestEntityTooLarge = errors.New("request entity too large")

    ErrRequestUriTooLong is returned when URI is too long
    ErrRequestUriTooLong = errors.New("request URI too long")

    ErrUnsupportedMediaType is returned when media type is not supported
    ErrUnsupportedMediaType = errors.New("unsupported media type")

    ErrRequestRangeNotSatisfiable is returned when range is not satisfiable
    ErrRequestRangeNotSatisfiable = errors.New("request range not satisfiable")

    ErrExpectationFailed is returned when expectation fails
    ErrExpectationFailed = errors.New("expectation failed")

    ErrTeapot is returned when server is a teapot
    ErrTeapot = errors.New("I'm a teapot")

    ErrMisdirected is returned when request is misdirected
    ErrMisdirected = errors.New("misdirected")

    ErrUseProxy is returned when proxy is required
    ErrUseProxy = errors.New("use proxy")

    ErrUpgradeRequired is returned when upgrade is required
    ErrUpgradeRequired = errors.New("upgrade required")

    ErrPreconditionRequired is returned when precondition is required
    ErrPreconditionRequired = errors.New("precondition required")

    ErrTooManyRequests is returned when there are too many requests
    ErrTooManyRequests = errors.New("too many requests")

    ErrRequestHeaderFieldsTooLarge is returned when header fields are too large
    ErrRequestHeaderFieldsTooLarge = errors.New("request header fields too large")

    ErrUnavailableForLegalReasons is returned when unavailable for legal reasons
    ErrUnavailableForLegalReasons = errors.New("unavailable for legal reasons")

    ErrInvalidCertificate is returned when certificate is invalid
    ErrInvalidCertificate = errors.New("invalid certificate")

    ErrIllegalOperation is returned when operation is illegal
    ErrIllegalOperation = errors.New("illegal operation")

    ErrOperationTimedOut is returned when operation times out
    ErrOperationTimedOut = errors.New("operation timed out")

    ErrInvalidResponse is returned when response is invalid
    ErrInvalidResponse = errors.New("invalid response")

    ErrBadGateway is returned when gateway returns bad response
    ErrBadGateway = errors.New("bad gateway")

    ErrServiceUnavailable is returned when service is unavailable
    ErrServiceUnavailable = errors.New("service unavailable")

    ErrGatewayTimeout is returned when gateway times out
    ErrGatewayTimeout = errors.New("gateway timeout")

    ErrVersionNotSupported is returned when version is not supported
    ErrVersionNotSupported = errors.New("version not supported")

    ErrVariantAlsoNegotiates is returned when variant also negotiates
    ErrVariantAlsoNegotiates = errors.New("variant also negotiates")

    ErrNotExtended is returned when extension is not supported
    ErrNotExtended = errors.New("not extended")

    ErrInsufficientStorage is returned when storage is insufficient
    ErrInsufficientStorage = errors.New("insufficient storage")

    ErrLoopDetected is returned when loop is detected
    ErrLoopDetected = errors.New("loop detected")

    ErrNotModified is returned when resource is not modified
    ErrNotModified = errors.New("not modified")

    ErrBadRequest is returned when request is bad
    ErrBadRequest = errors.New("bad request")

    ErrUnauthorized is returned when request is unauthorized
    ErrUnauthorized = errors.New("unauthorized")

    ErrPaymentRequired is returned when payment is required
    ErrPaymentRequired = errors.New("payment required")

    ErrForbidden is returned when access is forbidden
    ErrForbidden = errors.New("forbidden")

    ErrNotFound is returned when resource is not found
    ErrNotFound = errors.New("not found")

    ErrMethodNotAllowed is returned when method is not allowed
    ErrMethodNotAllowed = errors.New("method not allowed")

    ErrNotAcceptable is returned when request is not acceptable
    ErrNotAcceptable = errors.New("not acceptable")

    ErrProxyAuthRequired is returned when proxy authentication is required
    ErrProxyAuthRequired = errors.New("proxy authentication required")

    ErrRequestTimeout is returned when request times out
    ErrRequestTimeout = errors.New("request timeout")

    ErrConflict is returned when there is a conflict
    ErrConflict = errors.New("conflict")

    ErrGone is returned when resource is gone
    ErrGone = errors.New("gone")

    ErrLengthRequired is returned when length is required
    ErrLengthRequired = errors.New("length required")

    ErrPreconditionFailed is returned when precondition fails
    ErrPreconditionFailed = errors.New("precondition failed")

    ErrRequestEntityTooLarge is returned when entity is too large
    ErrRequestEntityTooLarge = errors.New("request entity too large")

    ErrRequestUriTooLong is returned when URI is too long
    ErrRequestUriTooLong = errors.New("request URI too long")

    ErrUnsupportedMediaType is returned when media type is not supported
    ErrUnsupportedMediaType = errors.New("unsupported media type")

    ErrRequestRangeNotSatisfiable is returned when range is not satisfiable
    ErrRequestRangeNotSatisfiable = errors.New("request range not satisfiable")

    ErrExpectationFailed is returned when expectation fails
    ErrExpectationFailed = errors.New("expectation failed")

    ErrTeapot is returned when server is a teapot
    ErrTeapot = errors.New("I'm a teapot")

    ErrMisdirected is returned when request is misdirected
    ErrMisdirected = errors.New("misdirected")

    ErrUseProxy is returned when proxy is required
    ErrUseProxy = errors.New("use proxy")

    ErrUpgradeRequired is returned when upgrade is required
    ErrUpgradeRequired = errors.New("upgrade required")

    ErrPreconditionRequired is returned when precondition is required
    ErrPreconditionRequired = errors.New("precondition required")

    ErrTooManyRequests is returned when there are too many requests
    ErrTooManyRequests = errors.New("too many requests")

    ErrRequestHeaderFieldsTooLarge is returned when header fields are too large
    ErrRequestHeaderFieldsTooLarge = errors.New("request header fields too large")

    ErrUnavailableForLegalReasons is returned when unavailable for legal reasons
    ErrUnavailableForLegalReasons = errors.New("unavailable for legal reasons")

    ErrInvalidCertificate is returned when certificate is invalid
    ErrInvalidCertificate = errors.New("invalid certificate")

    ErrIllegalOperation is returned when operation is illegal
    ErrIllegalOperation = errors.New("illegal operation")

    ErrOperationTimedOut is returned when operation times out
    ErrOperationTimedOut = errors.New("operation timed out")

    ErrInvalidResponse is returned when response is invalid
    ErrInvalidResponse = errors.New("invalid response")

    ErrBadGateway is returned when gateway returns bad response
    ErrBadGateway = errors.New("bad gateway")

    ErrServiceUnavailable is returned when service is unavailable
    ErrServiceUnavailable = errors.New("service unavailable")

    ErrGatewayTimeout is returned when gateway times out
    ErrGatewayTimeout = errors.New("gateway timeout")

    ErrVersionNotSupported is returned when version is not supported
    ErrVersionNotSupported = errors.New("version not supported")

    ErrVariantAlsoNegotiates is returned when variant also negotiates
    ErrVariantAlsoNegotiates = errors.New("variant also negotiates")

    ErrNotExtended is returned when extension is not supported
    ErrNotExtended = errors.New("not extended")

    ErrInsufficientStorage is returned when storage is insufficient
    ErrInsufficientStorage = errors.New("insufficient storage")

    ErrLoopDetected is returned when loop is detected
    ErrLoopDetected = errors.New("loop detected")

    ErrNotModified is returned when resource is not modified
    ErrNotModified = errors.New("not modified")

    ErrBadRequest is returned when request is bad
    ErrBadRequest = errors.New("bad request")

    ErrUnauthorized is returned when request is unauthorized
    ErrUnauthorized = errors.New("unauthorized")

    ErrPaymentRequired is returned when payment is required
    ErrPaymentRequired = errors.New("payment required")

    ErrForbidden is returned when access is forbidden
    ErrForbidden = errors.New("forbidden")

    ErrNotFound is returned when resource is not found
    ErrNotFound = errors.New("not found")

    ErrMethodNotAllowed is returned when method is not allowed
    ErrMethodNotAllowed = errors.New("method not allowed")

    ErrNotAcceptable is returned when request is not acceptable
    ErrNotAcceptable = errors.New("not acceptable")

    ErrProxyAuthRequired is returned when proxy authentication is required
    ErrProxyAuthRequired = errors.New("proxy authentication required")

    ErrRequestTimeout is returned when request times out
    ErrRequestTimeout = errors.New("request timeout")

    ErrConflict is returned when there is a conflict
    ErrConflict = errors.New("conflict")

    ErrGone is returned when resource is gone
    ErrGone = errors.New("gone")

    ErrLengthRequired is returned when length is required
    ErrLengthRequired = errors.New("length required")

    ErrPreconditionFailed is returned when precondition fails
    ErrPreconditionFailed = errors.New("precondition failed")

    ErrRequestEntityTooLarge is returned when entity is too large
    ErrRequestEntityTooLarge = errors.New("request entity too large")

    ErrRequestUriTooLong is returned when URI is too long
    ErrRequestUriTooLong is errors.New("request URI too long")

    ErrUnsupportedMediaType is returned when media type is not supported
    ErrUnsupportedMediaType = errors.New("unsupported media type")

    ErrRequestRangeNotSatisfiable is returned when range is not satisfiable
    ErrRequestRangeNotSatisfiable = errors.New("request range not satisfiable")

    ErrExpectationFailed is returned when expectation fails
    ErrExpectationFailed = errors.New("expectation failed")

    ErrTeapot is returned when server is a teapot
    ErrTeapot = errors.New("I'm a teapot")

    ErrMisdirected is returned when request is misdirected
    ErrMisdirected = errors.New("misdirected")

    ErrUseProxy is returned when proxy is required
    ErrUseProxy = errors.New("use proxy")

    ErrUpgradeRequired is returned when upgrade is required
    ErrUpgradeRequired = errors.New("upgrade required")

    ErrPreconditionRequired is returned when precondition is required
    ErrPreconditionRequired = errors.New("precondition required")

    ErrTooManyRequests is returned when there are too many requests
    ErrTooManyRequests = errors.New("too many requests")

    ErrRequestHeaderFieldsTooLarge is returned when header fields are too large
    ErrRequestHeaderFieldsTooLarge = errors.New("request header fields too large")

    ErrUnavailableForLegalReasons is returned when unavailable for legal reasons
    ErrUnavailableForLegalReasons = errors.New("unavailable for legal reasons")

    ErrInvalidCertificate is returned when certificate is invalid
    ErrInvalidCertificate = errors.New("invalid certificate")

    ErrIllegalOperation is returned when operation is illegal
    ErrIllegalOperation = errors.New("illegal operation")

    ErrOperationTimedOut is returned when operation times out
    ErrOperationTimedOut = errors.New("operation timed out")

    ErrInvalidResponse is returned when response is invalid
    ErrInvalidResponse = errors.New("invalid response")

    ErrBadGateway is returned when gateway returns bad response
    ErrBadGateway = errors.New("bad gateway")

    ErrServiceUnavailable is returned when service is unavailable
    ErrServiceUnavailable = errors.New("service unavailable")

    ErrGatewayTimeout is returned when gateway times out
    ErrGatewayTimeout = errors.New("gateway timeout")

    ErrVersionNotSupported is returned when version is not supported
    ErrVersionNotSupported = errors.New("version not supported")

    ErrVariantAlsoNegotiates is returned when variant also negotiates
    ErrVariantAlsoNegotiates = errors.New("variant also negotiates")

    ErrNotExtended is returned when extension is not supported
    ErrNotExtended = errors.New("not extended")

    ErrInsufficientStorage is returned when storage is insufficient
    ErrInsufficientStorage = errors.New("insufficient storage")

    ErrLoopDetected is returned when loop is detected
    ErrLoopDetected = errors.New("loop detected")

    ErrNotModified is returned when resource is not modified
    ErrNotModified = errors.New("not modified")

    ErrBadRequest is returned when request is bad
    ErrBadRequest = errors.New("bad request")

    ErrUnauthorized is returned when request is unauthorized
    ErrUnauthorized = errors.New("unauthorized")

    ErrPaymentRequired is returned when payment is required
    ErrPaymentRequired = errors.New("payment required")

    ErrForbidden is returned when access is forbidden
    ErrForbidden = errors.New("forbidden")

    ErrNotFound is returned when resource is not found
    ErrNotFound = errors.New("not found")

    ErrMethodNotAllowed is returned when method is not allowed
    ErrMethodNotAllowed = errors.New("method not allowed")

    ErrNotAcceptable is returned when request is not acceptable
    ErrNotAcceptable = errors.New("not acceptable")

    ErrProxyAuthRequired is returned when proxy authentication is required
