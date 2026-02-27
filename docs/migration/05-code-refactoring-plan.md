# Code Refactoring Plan for SQLite Integration - qwencoder-proxy

## Table of Contents

1. [Overview](#overview)
2. [Refactoring Principles](#refactoring-principles)
3. [Configuration Refactoring](#configuration-refactoring)
4. [Store Factory Pattern](#store-factory-pattern)
5. [MultiTokenManager Refactoring](#multitokenmanager-refactoring)
6. [MultiTokenStore Refactoring](#multitokenstore-refactoring)
7. [Main Entry Point Refactoring](#main-entry-point-refactoring)
8. [REST API Refactoring](#rest-api-refactoring)
9. [Proxy Handler Refactoring](#proxy-handler-refactoring)
10. [Token Refresh Refactoring](#token-refresh-refactoring)
11. [File-to-Database Migration Integration](#file-to-database-migration-integration)
12. [Deprecation and Cleanup Plan](#deprecation-and-cleanup-plan)
13. [Implementation Phases](#implementation-phases)
14. [Testing Strategy](#testing-strategy)
15. [Rollback Plan](#rollback-plan)

---

## Overview

This document provides a detailed code refactoring plan for integrating SQLite storage into the qwencoder-proxy project. The refactoring is designed to be **non-breaking**, enabling gradual migration from file-based storage to SQLite while maintaining full backward compatibility.

### Related Documents

- [`01-current-architecture-analysis.md`](./01-current-architecture-analysis.md) - Current architecture analysis
- [`02-sqlite-database-schema-design.md`](./02-sqlite-database-schema-design.md) - SQLite database schema design
- [`03-database-initialization-strategy.md`](./03-database-initialization-strategy.md) - Database initialization strategy
- [`04-sqlite-storage-layer-design.md`](./04-sqlite-storage-layer-design.md) - SQLite storage layer design

### Refactoring Goals

| Goal | Description |
|-------|-------------|
| **Non-Breaking** | Existing file-based storage continues to work without changes |
| **Backward Compatible** | All existing APIs and behaviors are preserved |
| **Configurable** | Storage backend selection via configuration |
| **Graceful Migration** | Seamless migration path from file to SQLite |
| **Clean Architecture** | Follow Go best practices and existing code patterns |

---

## Refactoring Principles

### Go Best Practices

1. **Interface Segregation**: Small, focused interfaces for specific responsibilities
2. **Dependency Inversion**: Depend on abstractions, not concrete implementations
3. **Explicit Error Handling**: All errors are checked and properly wrapped with context
4. **Resource Management**: Proper cleanup of database connections and prepared statements
5. **Thread Safety**: All operations are safe for concurrent access

### Refactoring Approach

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                        Refactoring Strategy                                │
│                                                                         │
│  ┌───────────────────────────────────────────────────────────────────────────┐  │
│  │  Phase 1: Add SQLite Support (Non-Breaking)                │  │
│  │  - Add configuration for storage backend selection                    │  │
│  │  - Implement SQLiteStore with TokenStore interface                  │  │
│  │  - Create factory for store creation                                 │  │
│  │  - File-based storage remains default                                │  │
│  └───────────────────────────────────────────────────────────────────────────┘  │
│                                                                         │
│  ┌───────────────────────────────────────────────────────────────────────────┐  │
│  │  Phase 2: Testing & Validation                                  │  │
│  │  - Comprehensive testing of both backends                          │  │
│  │  - Performance benchmarking                                        │  │
│  │  - Data integrity validation                                        │  │
│  └───────────────────────────────────────────────────────────────────────────┘  │
│                                                                         │
│  ┌───────────────────────────────────────────────────────────────────────────┐  │
│  │  Phase 3: Default to SQLite                                      │  │
│  │  - Change default backend to "sqlite"                                │  │
│  │  - Add deprecation warnings for file-based storage                   │  │
│  │  - File-based storage remains available as fallback                     │  │
│  └───────────────────────────────────────────────────────────────────────────┘  │
│                                                                         │
│  ┌───────────────────────────────────────────────────────────────────────────┐  │
│  │  Phase 4: Cleanup (Future Release)                                │  │
│  │  - Remove file-based storage code                                     │  │
│  │  - Remove migration utilities                                         │  │
│  │  - Update documentation                                              │  │
│  └───────────────────────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────────────────────┘
```

---

## Configuration Refactoring

### Current Configuration Structure

The current [`Config`](../config/config.go) structure is:

```go
// config/config.go

type Config struct {
    Server      ServerConfig
    HTTPClient  HTTPClientConfig
    Logging     LoggingConfig
    OAuthServer OAuthServerConfig
}
```

### New Configuration Structure

Add `StorageConfig` to support storage backend selection:

```go
// config/config.go

// StorageConfig holds storage-related configuration
type StorageConfig struct {
    // Backend specifies the storage backend to use
    // Valid values: "file", "sqlite", "auto"
    // - "file": Use file-based JSON storage (legacy)
    // - "sqlite": Use SQLite database storage
    // - "auto": Auto-detect based on existing data (default)
    Backend string `json:"backend" env:"STORAGE_BACKEND"`

    // Path specifies the storage location
    // For file backend: Directory path (e.g., ".credentials")
    // For sqlite backend: Database file path (e.g., ".credentials/tokens.db")
    // For auto backend: Base path for both backends
    Path string `json:"path" env:"STORAGE_PATH"`

    // MigrateOnStartup enables automatic migration from file to SQLite
    // Only applies when backend is "auto" or "sqlite"
    // Default: true
    MigrateOnStartup bool `json:"migrate_on_startup" env:"MIGRATE_ON_STARTUP"`

    // BackupOnMigration creates a backup of file-based storage before migration
    // Default: true
    BackupOnMigration bool `json:"backup_on_migration" env:"BACKUP_ON_MIGRATION"`
}

type Config struct {
    Server      ServerConfig
    HTTPClient  HTTPClientConfig
    Logging     LoggingConfig
    OAuthServer OAuthServerConfig
    Storage     StorageConfig // NEW: Storage configuration
}
```

### Default Configuration Update

Update [`DefaultConfig()`](../config/config.go#L70) function:

```go
// config/config.go

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
            Backend:           "auto", // Auto-detect for seamless migration
            Path:              ".credentials/tokens.db", // SQLite path
            MigrateOnStartup:  true,
            BackupOnMigration: true,
        },
    }
}
```

### Configuration Validation

Add validation function:

```go
// config/config.go

// ValidateStorageConfig validates the storage configuration
func (c *Config) ValidateStorageConfig() error {
    switch c.Storage.Backend {
    case "file", "sqlite", "auto":
        // Valid backend
    case "":
        // Default to auto
        c.Storage.Backend = "auto"
    default:
        return fmt.Errorf("invalid storage backend: %s (must be 'file', 'sqlite', or 'auto')", c.Storage.Backend)
    }

    // Validate path
    if c.Storage.Path == "" {
        return fmt.Errorf("storage path cannot be empty")
    }

    return nil
}
```

### Environment Variable Support

The configuration already supports environment variables via struct tags. Ensure the following environment variables are documented:

| Environment Variable | Description | Default |
|---------------------|-------------|----------|
| `STORAGE_BACKEND` | Storage backend: "file", "sqlite", "auto" | "auto" |
| `STORAGE_PATH` | Storage path for file or database | ".credentials/tokens.db" |
| `MIGRATE_ON_STARTUP` | Enable automatic migration on startup | "true" |
| `BACKUP_ON_MIGRATION` | Create backup before migration | "true" |

### Refactoring Checklist

- [ ] Add `StorageConfig` struct to [`config/config.go`](../config/config.go)
- [ ] Update `Config` struct to include `Storage` field
- [ ] Update `DefaultConfig()` function with storage defaults
- [ ] Add `ValidateStorageConfig()` function
- [ ] Update environment variable documentation in README
- [ ] Add unit tests for storage configuration

---

## Store Factory Pattern

### Design Overview

The store factory pattern provides a centralized way to create storage backends based on configuration. This follows the **Factory Pattern** and **Dependency Inversion Principle**.

### Factory Interface

```go
// internal/token/store_factory.go

package token

import (
    "fmt"
    "os"
    "path/filepath"

    "github.com/sunbankio/qwencoder-proxy/config"
    "github.com/sunbankio/qwencoder-proxy/logging"
)

// StoreFactory creates token stores based on configuration
type StoreFactory struct {
    config     *config.StorageConfig
    logger     logging.Logger
    httpClient *http.Client
    clientFactory ProxyClientFactory
}

// NewStoreFactory creates a new store factory
func NewStoreFactory(
    storageConfig *config.StorageConfig,
    logger logging.Logger,
    httpClient *http.Client,
    clientFactory ProxyClientFactory,
) *StoreFactory {
    return &StoreFactory{
        config:     storageConfig,
        logger:     logger,
        httpClient: httpClient,
        clientFactory: clientFactory,
    }
}

// CreateStore creates a token store for a provider
// Returns TokenStore interface for abstraction
func (sf *StoreFactory) CreateStore(providerID string) (TokenStore, error) {
    switch sf.config.Backend {
    case "file":
        return sf.createFileStore(providerID)
    case "sqlite":
        return sf.createSQLiteStore(providerID)
    case "auto":
        return sf.autoDetectBackend(providerID)
    default:
        return nil, fmt.Errorf("unsupported storage backend: %s", sf.config.Backend)
    }
}

// createFileStore creates a file-based token store
func (sf *StoreFactory) createFileStore(providerID string) (TokenStore, error) {
    // Determine credentials directory
    credsDir := sf.config.Path
    if credsDir == "" {
        credsDir = ".credentials"
    }

    // Create provider-specific path
    providerPath := filepath.Join(credsDir, providerID)

    sf.logger.InfoLog("[StoreFactory] Creating file-based store for provider: %s at: %s", providerID, providerPath)

    return NewMultiTokenStore(providerID, providerPath, sf.logger), nil
}

// createSQLiteStore creates a SQLite-backed token store
func (sf *StoreFactory) createSQLiteStore(providerID string) (TokenStore, error) {
    // Determine database path
    dbPath := sf.config.Path
    if dbPath == "" {
        dbPath = ".credentials/tokens.db"
    }

    sf.logger.InfoLog("[StoreFactory] Creating SQLite store for provider: %s at: %s", providerID, dbPath)

    return NewSQLiteStore(dbPath, providerID, sf.logger)
}

// autoDetectBackend automatically detects the appropriate storage backend
func (sf *StoreFactory) autoDetectBackend(providerID string) (TokenStore, error) {
    dbPath := sf.config.Path
    if dbPath == "" {
        dbPath = ".credentials/tokens.db"
    }

    // Check if SQLite database exists
    if _, err := os.Stat(dbPath); err == nil {
        sf.logger.InfoLog("[StoreFactory] Auto-detected SQLite backend (database exists)")
        return sf.createSQLiteStore(providerID)
    }

    // Check if file-based storage exists
    credsDir := ".credentials"
    if sf.config.Path != "" && !filepath.Ext(sf.config.Path) == "" {
        credsDir = sf.config.Path
    }
    providerDir := filepath.Join(credsDir, providerID)

    if _, err := os.Stat(providerDir); err == nil {
        sf.logger.InfoLog("[StoreFactory] Auto-detected file-based backend (credentials exist)")
        sf.logger.WarnLog("[StoreFactory] Consider migrating to SQLite for better performance")
        return sf.createFileStore(providerID)
    }

    // No existing storage - use SQLite for new installations
    sf.logger.InfoLog("[StoreFactory] No existing storage found, using SQLite backend")
    return sf.createSQLiteStore(providerID)
}
```

### Storage Backend Detection Flow

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                    StoreFactory.CreateStore(providerID)                     │
└────────────────────────────────┬────────────────────────────────────────────────┘
                             │
                             ▼
                  ┌────────────────────────┐
                  │ Check Backend Config │
                  └────────┬───────────┘
                           │
          ┌──────────────┼──────────────┐
          │              │              │
          ▼              ▼              ▼
    ┌──────────┐   ┌──────────┐   ┌──────────┐
    │ "file"   │   │ "sqlite"  │   │ "auto"    │
    └─────┬────┘   └─────┬────┘   └─────┬────┘
          │                │                │
          ▼                ▼                ▼
    ┌──────────────┐  ┌──────────────┐  ┌──────────────────────────┐
    │ createFile  │  │ createSQLite│  │ autoDetectBackend()    │
    │ Store()     │  │ Store()     │  └──────────┬───────────┘
    └─────┬──────┘  └─────┬──────┘             │
          │                 │                      │
          ▼                 ▼                      ▼
    ┌──────────────────────────────────────────────────────┐
    │ Return TokenStore (MultiTokenStore or SQLiteStore) │
    └──────────────────────────────────────────────────┘
```

### Refactoring Checklist

- [ ] Create [`internal/token/store_factory.go`](../internal/token/store_factory.go)
- [ ] Implement `StoreFactory` struct
- [ ] Implement `CreateStore()` method
- [ ] Implement `createFileStore()` method
- [ ] Implement `createSQLiteStore()` method
- [ ] Implement `autoDetectBackend()` method
- [ ] Add unit tests for store factory
- [ ] Update [`MultiTokenManager`](../internal/token/multi_token_manager.go) to use factory

---

## MultiTokenManager Refactoring

### Current Structure

The current [`MultiTokenManager`](../internal/token/multi_token_manager.go) stores:

```go
type MultiTokenManager struct {
    stores              map[string]*MultiTokenStore  // Concrete type
    managers            map[string]*TokenManager
    refreshers          map[string]ProviderRefresh
    schedulers          map[string]*RefreshScheduler
    extractors          map[string]EmailExtractor
    emailManager        *EmailExtractionManager
    healthTrackers      map[string]*HealthTracker
    proxyHealthTrackers map[string]*ProxyHealthTracker
    strategyFactory     *StrategyFactory
    mu                  sync.RWMutex
    logger              logging.Logger
    httpClient          *http.Client
    clientFactory       ProxyClientFactory
    credentialsDir      string
    initialized         bool
}
```

### Refactored Structure

Change stores to use `TokenStore` interface:

```go
// internal/token/multi_token_manager.go

type MultiTokenManager struct {
    stores              map[string]TokenStore  // CHANGED: Use interface
    managers            map[string]*TokenManager
    refreshers          map[string]ProviderRefresh
    schedulers          map[string]*RefreshScheduler
    extractors          map[string]EmailExtractor
    emailManager        *EmailExtractionManager
    healthTrackers      map[string]*HealthTracker
    proxyHealthTrackers map[string]*ProxyHealthTracker
    strategyFactory     *StrategyFactory
    storeFactory        *StoreFactory  // NEW: Store factory
    mu                  sync.RWMutex
    logger              logging.Logger
    httpClient          *http.Client
    clientFactory       ProxyClientFactory
    storageConfig       *config.StorageConfig  // NEW: Storage config
    initialized         bool
}
```

### Updated Constructor

```go
// internal/token/multi_token_manager.go

func NewMultiTokenManager(logger logging.Logger) *MultiTokenManager {
    if logger == nil {
        logger = logging.NewLogger()
    }

    return &MultiTokenManager{
        stores:              make(map[string]TokenStore),  // Interface type
        managers:            make(map[string]*TokenManager),
        refreshers:          make(map[string]ProviderRefresh),
        schedulers:          make(map[string]*RefreshScheduler),
        extractors:          make(map[string]EmailExtractor),
        healthTrackers:      make(map[string]*HealthTracker),
        proxyHealthTrackers: make(map[string]*ProxyHealthTracker),
        strategyFactory:     NewStrategyFactory(),
        logger:              logger,
        httpClient:          &http.Client{Timeout: 30 * time.Second},
        credentialsDir:      ".credentials",
    }
}

// SetStorageConfig sets the storage configuration
func (mtm *MultiTokenManager) SetStorageConfig(config *config.StorageConfig) {
    mtm.mu.Lock()
    defer mtm.mu.Unlock()

    mtm.storageConfig = config
    mtm.storeFactory = NewStoreFactory(config, mtm.logger, mtm.httpClient, mtm.clientFactory)
}

// SetStoreFactory sets the store factory (for testing)
func (mtm *MultiTokenManager) SetStoreFactory(factory *StoreFactory) {
    mtm.mu.Lock()
    defer mtm.mu.Unlock()
    mtm.storeFactory = factory
}
```

### Updated GetTokenStore Method

```go
// internal/token/multi_token_manager.go

// GetTokenStore returns or creates a token store for a provider
// Now uses the store factory for backend selection
func (mtm *MultiTokenManager) GetTokenStore(providerID string) (TokenStore, error) {
    mtm.mu.RLock()
    store, exists := mtm.stores[providerID]
    mtm.mu.RUnlock()

    if exists {
        return store, nil
    }

    mtm.mu.Lock()
    defer mtm.mu.Unlock()

    // Double-check after acquiring write lock
    if store, exists := mtm.stores[providerID]; exists {
        return store, nil
    }

    // Create store using factory
    if mtm.storeFactory == nil {
        // Fallback to file-based storage for backward compatibility
        mtm.logger.WarnLog("[MultiTokenManager] No store factory set, using file-based storage")
        store = NewMultiTokenStore(providerID, filepath.Join(mtm.credentialsDir, providerID), mtm.logger)
    } else {
        var err error
        store, err = mtm.storeFactory.CreateStore(providerID)
        if err != nil {
            return nil, fmt.Errorf("failed to create store for provider %s: %w", providerID, err)
        }
    }

    // Initialize store
    if err := store.Load(); err != nil {
        mtm.logger.ErrorLog("[MultiTokenManager] Failed to load store for provider %s: %v", providerID, err)
        return nil, fmt.Errorf("failed to load store: %w", err)
    }

    mtm.stores[providerID] = store
    mtm.logger.InfoLog("[MultiTokenManager] Created store for provider: %s", providerID)

    return store, nil
}
```

### Updated Stop Method

```go
// internal/token/multi_token_manager.go

// Stop stops all components and closes stores
func (mtm *MultiTokenManager) Stop() {
    mtm.mu.Lock()
    defer mtm.mu.Unlock()

    mtm.logger.InfoLog("[MultiTokenManager] Stopping multi-token manager...")

    // Stop all schedulers
    for providerID, scheduler := range mtm.schedulers {
        scheduler.Stop()
        mtm.logger.InfoLog("[MultiTokenManager] Stopped scheduler for provider: %s", providerID)
    }

    // Close all stores (support SQLiteStore.Close())
    for providerID, store := range mtm.stores {
        if closer, ok := store.(interface{ Close() error }); ok {
            if err := closer.Close(); err != nil {
                mtm.logger.ErrorLog("[MultiTokenManager] Failed to close store for provider %s: %v", providerID, err)
            }
        }
    }

    mtm.logger.InfoLog("[MultiTokenManager] Stopped")
}
```

### Refactoring Checklist

- [ ] Update `stores` field type from `map[string]*MultiTokenStore` to `map[string]TokenStore`
- [ ] Add `storeFactory` field to `MultiTokenManager`
- [ ] Add `storageConfig` field to `MultiTokenManager`
- [ ] Add `SetStorageConfig()` method
- [ ] Add `SetStoreFactory()` method
- [ ] Update `GetTokenStore()` to use factory
- [ ] Update `Stop()` to close stores
- [ ] Update all methods that access stores to use interface
- [ ] Add unit tests for refactored methods

---

## MultiTokenStore Refactoring

### Current Implementation

The current [`MultiTokenStore`](../internal/token/multi_token_store.go) is file-based only:

```go
type MultiTokenStore struct {
    ProviderID  string
    Version     int
    Tokens      []ProviderToken
    Settings    StoreSettings
    mu          sync.RWMutex
    providerDir string
    logger      logging.Logger
}
```

### Refactoring Strategy

**No changes to `MultiTokenStore` are required.** The existing file-based implementation remains as-is for backward compatibility.

Instead, we will:

1. Keep `MultiTokenStore` as the file-based implementation
2. Create new `SQLiteStore` as a separate implementation
3. Both implement the `TokenStore` interface
4. Use the factory pattern to select the appropriate implementation

### Interface Compliance Verification

Ensure `MultiTokenStore` fully implements `TokenStore`:

```go
// internal/token/multi_token_store.go

// Verify MultiTokenStore implements TokenStore interface
var _ TokenStore = (*MultiTokenStore)(nil)
```

### Deprecation Notice

Add deprecation notice to `MultiTokenStore`:

```go
// internal/token/multi_token_store.go

// MultiTokenStore manages multiple tokens for a provider using file-based storage.
//
// Deprecated: File-based storage is deprecated. Use SQLite storage for new installations.
// This implementation is maintained for backward compatibility.
// See [SQLiteStore] for the recommended storage implementation.
//
// Migration: Use the Migrator utility to migrate to SQLite storage.
type MultiTokenStore struct {
    // ... existing fields ...
}
```

### Refactoring Checklist

- [ ] Add deprecation notice to `MultiTokenStore` documentation
- [ ] Verify `MultiTokenStore` implements `TokenStore` interface
- [ ] Add compile-time interface assertion
- [ ] No code changes required (backward compatible)

---

## Main Entry Point Refactoring

### Current Implementation

The current [`main()`](../cmd/qwencoder-proxy/main.go) function:

```go
func main() {
    // Parse command-line flags
    port := flag.String("port", "", "Server port (default: 8143)")
    debug := flag.Bool("debug", false, "Enable debug logging")
    flag.Parse()

    // Load configuration from environment
    cfg := config.DefaultConfig()

    // Override with command-line flags
    if *port != "" {
        cfg.Server.Port = *port
    }
    if *debug {
        cfg.Logging.IsDebugMode = true
        logging.IsDebugMode = true
    }

    // Initialize logger
    logger := logging.NewLogger()
    if cfg.Logging.IsDebugMode {
        logger.DebugLog("Debug mode enabled")
    }

    logger.InfoLog("Starting qwencoder-proxy server on port %s", cfg.Server.Port)

    // Create multi-token manager
    multiTokenMgr := token.NewMultiTokenManager(logger)
    if err := multiTokenMgr.Initialize(); err != nil {
        logger.ErrorLog("Failed to initialize multi-token manager: %v", err)
        os.Exit(1)
    }

    // Start refresh schedulers
    if err := multiTokenMgr.Start(); err != nil {
        logger.ErrorLog("Failed to start multi-token manager: %v", err)
        os.Exit(1)
    }

    // Set credentials directory (can be overridden by environment)
    if credsDir := os.Getenv("CREDENTIALS_DIR"); credsDir != "" {
        multiTokenMgr.SetCredentialsDir(credsDir)
        logger.InfoLog("Using credentials directory: %s", credsDir)
    }

    // Create proxy client factory for proxy-aware HTTP clients
    proxyClientFactory := config.NewProxyAwareHTTPClientFactory(
        cfg.HTTPClient,
        logger,
        50, // max cache size
    )

    // Set client factory on multi-token manager
    multiTokenMgr.SetClientFactory(proxyClientFactory)

    // Create provider factory
    providerFactory := provider.NewFactory()

    // Create converter factory
    converterFactory := converter.NewFactory()

    // Initialize providers with token manager injection
    if err := initializeProviders(providerFactory, multiTokenMgr, logger); err != nil {
        logger.ErrorLog("Failed to initialize providers: %v", err)
        os.Exit(1)
    }

    // Create HTTP mux and register routes
    mux := http.NewServeMux()

    // Create REST API server for dashboard and OAuth flows
    apiConfig := &restapi.Config{
        Port:            cfg.Server.Port,
        CallbackBaseURL: "http://localhost:" + cfg.Server.Port,
        StateTTL:        10 * time.Minute,
        DeviceCodeTTL:   15 * time.Minute,
        EnableCORS:      true,
        AllowedOrigins:  []string{"*"},
    }
    restAPIServer := restapi.NewServer(apiConfig, logger)

    // Inject main application's multi-token manager into REST API server
    restAPIServer.SetMultiTokenManager(multiTokenMgr)

    // Register REST API routes (dashboard, providers, credentials, etc.)
    restAPIServer.RegisterRoutes(mux)

    // Register OpenAI-compatible routes with token manager integration
    restAPIServer.RegisterProxyRoutes(mux, providerFactory, converterFactory)

    // Apply middleware (CORS, logging)
    handler := applyMiddleware(mux, logger, cfg)

    // Create server address
    addr := ":" + cfg.Server.Port

    // Setup graceful shutdown
    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

    // Start server in goroutine
    errChan := make(chan error, 1)
    go func() {
        defer func() {
            if r := recover(); r != nil {
                logger.ErrorLog("Panic recovered: %v", r)
            }
        }()
        logger.InfoLog("Server listening on %s", addr)
        errChan <- http.ListenAndServe(addr, handler)
    }()

    // Wait for shutdown or error
    select {
    case err := <-errChan:
        logger.ErrorLog("Server error: %v", err)
        multiTokenMgr.Stop()
        os.Exit(1)
    case sig := <-sigChan:
        logger.InfoLog("Received signal %v, shutting down...", sig)
        multiTokenMgr.Stop()
        // Stop REST API server
        restAPIServer.Stop()
    }
}
```

### Refactored Implementation

```go
// cmd/qwencoder-proxy/main.go

func main() {
    // Parse command-line flags
    port := flag.String("port", "", "Server port (default: 8143)")
    debug := flag.Bool("debug", false, "Enable debug logging")
    storageBackend := flag.String("storage", "", "Storage backend: file, sqlite, auto (default: auto)")
    storagePath := flag.String("storage-path", "", "Storage path for file or database")
    migrate := flag.Bool("migrate", false, "Force migration from file to SQLite")
    flag.Parse()

    // Load configuration from environment
    cfg := config.DefaultConfig()

    // Override with command-line flags
    if *port != "" {
        cfg.Server.Port = *port
    }
    if *debug {
        cfg.Logging.IsDebugMode = true
        logging.IsDebugMode = true
    }
    if *storageBackend != "" {
        cfg.Storage.Backend = *storageBackend
    }
    if *storagePath != "" {
        cfg.Storage.Path = *storagePath
    }

    // Validate configuration
    if err := cfg.ValidateStorageConfig(); err != nil {
        fmt.Fprintf(os.Stderr, "Invalid configuration: %v\n", err)
        os.Exit(1)
    }

    // Initialize logger
    logger := logging.NewLogger()
    if cfg.Logging.IsDebugMode {
        logger.DebugLog("Debug mode enabled")
    }

    logger.InfoLog("Starting qwencoder-proxy server on port %s", cfg.Server.Port)
    logger.InfoLog("Storage backend: %s", cfg.Storage.Backend)
    logger.InfoLog("Storage path: %s", cfg.Storage.Path)

    // Create multi-token manager
    multiTokenMgr := token.NewMultiTokenManager(logger)

    // Set storage configuration
    multiTokenMgr.SetStorageConfig(&cfg.Storage)

    // Initialize multi-token manager
    if err := multiTokenMgr.Initialize(); err != nil {
        logger.ErrorLog("Failed to initialize multi-token manager: %v", err)
        os.Exit(1)
    }

    // Handle forced migration
    if *migrate {
        logger.InfoLog("Forced migration requested")
        if err := handleMigration(multiTokenMgr, logger); err != nil {
            logger.ErrorLog("Migration failed: %v", err)
            os.Exit(1)
        }
    }

    // Start refresh schedulers
    if err := multiTokenMgr.Start(); err != nil {
        logger.ErrorLog("Failed to start multi-token manager: %v", err)
        os.Exit(1)
    }

    // Create proxy client factory for proxy-aware HTTP clients
    proxyClientFactory := config.NewProxyAwareHTTPClientFactory(
        cfg.HTTPClient,
        logger,
        50, // max cache size
    )

    // Set client factory on multi-token manager
    multiTokenMgr.SetClientFactory(proxyClientFactory)

    // Create provider factory
    providerFactory := provider.NewFactory()

    // Create converter factory
    converterFactory := converter.NewFactory()

    // Initialize providers with token manager injection
    if err := initializeProviders(providerFactory, multiTokenMgr, logger); err != nil {
        logger.ErrorLog("Failed to initialize providers: %v", err)
        os.Exit(1)
    }

    // Create HTTP mux and register routes
    mux := http.NewServeMux()

    // Create REST API server for dashboard and OAuth flows
    apiConfig := &restapi.Config{
        Port:            cfg.Server.Port,
        CallbackBaseURL: "http://localhost:" + cfg.Server.Port,
        StateTTL:        10 * time.Minute,
        DeviceCodeTTL:   15 * time.Minute,
        EnableCORS:      true,
        AllowedOrigins:  []string{"*"},
    }
    restAPIServer := restapi.NewServer(apiConfig, logger)

    // Inject main application's multi-token manager into REST API server
    restAPIServer.SetMultiTokenManager(multiTokenMgr)

    // Register REST API routes (dashboard, providers, credentials, etc.)
    restAPIServer.RegisterRoutes(mux)

    // Register OpenAI-compatible routes with token manager integration
    restAPIServer.RegisterProxyRoutes(mux, providerFactory, converterFactory)

    // Apply middleware (CORS, logging)
    handler := applyMiddleware(mux, logger, cfg)

    // Create server address
    addr := ":" + cfg.Server.Port

    // Setup graceful shutdown
    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

    // Start server in goroutine
    errChan := make(chan error, 1)
    go func() {
        defer func() {
            if r := recover(); r != nil {
                logger.ErrorLog("Panic recovered: %v", r)
            }
        }()
        logger.InfoLog("Server listening on %s", addr)
        errChan <- http.ListenAndServe(addr, handler)
    }()

    // Wait for shutdown or error
    select {
    case err := <-errChan:
        logger.ErrorLog("Server error: %v", err)
        multiTokenMgr.Stop()
        os.Exit(1)
    case sig := <-sigChan:
        logger.InfoLog("Received signal %v, shutting down...", sig)
        multiTokenMgr.Stop()
        // Stop REST API server
        restAPIServer.Stop()
    }
}

// handleMigration handles migration from file to SQLite storage
func handleMigration(mtm *token.MultiTokenManager, logger logging.Logger) error {
    logger.InfoLog("Starting migration from file-based to SQLite storage")

    // Get all providers with file-based stores
    // This would require adding a method to MultiTokenManager to list providers
    // For now, we'll just log a message
    logger.InfoLog("Migration feature will be implemented in the storage layer")

    return nil
}
```

### Refactoring Checklist

- [ ] Add `--storage` command-line flag
- [ ] Add `--storage-path` command-line flag
- [ ] Add `--migrate` command-line flag
- [ ] Call `cfg.ValidateStorageConfig()` after loading configuration
- [ ] Call `multiTokenMgr.SetStorageConfig()` before initialization
- [ ] Add `handleMigration()` function
- [ ] Update startup logging to show storage backend
- [ ] Update documentation for new flags

---

## REST API Refactoring

### Current Implementation

The current [`Server`](../restapi/rest_api.go) uses concrete types:

```go
type Server struct {
    config            *Config
    registry          *ProviderRegistry
    stateManager      *StateManager
    logger            logging.Logger
    httpClient        *http.Client
    tokenStores       map[string]*tokpkg.MultiTokenStore  // Concrete type
    tokenManagers     map[string]*tokpkg.TokenManager
    multiTokenManager *tokpkg.MultiTokenManager
}
```

### Refactored Implementation

Change `tokenStores` to use interface:

```go
// restapi/rest_api.go

type Server struct {
    config            *Config
    registry          *ProviderRegistry
    stateManager      *StateManager
    logger            logging.Logger
    httpClient        *http.Client
    tokenStores       map[string]tokpkg.TokenStore  // CHANGED: Use interface
    tokenManagers     map[string]*tokpkg.TokenManager
    multiTokenManager *tokpkg.MultiTokenManager
}
```

### Updated Constructor

```go
// restapi/rest_api.go

func NewServer(config *Config, logger logging.Logger) *Server {
    if config == nil {
        config = DefaultConfig()
    }
    if logger == nil {
        logger = logging.NewLogger()
    }

    server := &Server{
        config:            config,
        registry:          NewProviderRegistry(),
        stateManager:      NewStateManager(),
        logger:            logger,
        httpClient:        &http.Client{Timeout: 30 * time.Second},
        tokenStores:       make(map[string]tokpkg.TokenStore),  // Interface type
        tokenManagers:     make(map[string]*tokpkg.TokenManager),
        multiTokenManager: nil,
    }

    return server
}
```

### Updated GetTokenStore Method

```go
// restapi/rest_api.go

// GetTokenStore returns or creates a token store for a provider
func (s *Server) GetTokenStore(providerID string) (tokpkg.TokenStore, error) {
    // Check if store already exists
    if store, exists := s.tokenStores[providerID]; exists {
        return store, nil
    }

    // Get store from MultiTokenManager
    store, err := s.multiTokenManager.GetTokenStore(providerID)
    if err != nil {
        return nil, fmt.Errorf("failed to get token store: %w", err)
    }

    // Cache the store
    s.tokenStores[providerID] = store

    return store, nil
}
```

### Updated Handle Callback Method

The OAuth callback handler saves tokens. This should work with both storage backends without changes:

```go
// restapi/rest_api.go

// handleCallback handles OAuth2 callback
func (s *Server) handleCallback(w http.ResponseWriter, r *http.Request) {
    // ... existing code for state validation, code exchange, etc. ...

    // Get token store (works with both backends via interface)
    store, err := s.GetTokenStore(providerID)
    if err != nil {
        s.logger.ErrorLog("[Server] Failed to get token store: %v", err)
        http.Error(w, "Failed to get token store", http.StatusInternalServerError)
        return
    }

    // Create provider token
    token := tokpkg.ProviderToken{
        // ... populate token fields ...
    }

    // Add token to store (works with both backends via interface)
    tokens := store.Load()
    tokens[token.ID] = token
    if err := store.Save(tokens); err != nil {
        s.logger.ErrorLog("[Server] Failed to save token: %v", err)
        http.Error(w, "Failed to save token", http.StatusInternalServerError)
        return
    }

    // ... rest of callback handling ...
}
```

### Updated Handle Credentials Method

```go
// restapi/rest_api.go

// handleCredentials returns all stored credentials
func (s *Server) handleCredentials(w http.ResponseWriter, r *http.Request) {
    s.logger.DebugLog("[Server] handleCredentials called")

    // Get all providers
    providers := s.multiTokenManager.ListProviders()

    credentials := make([]ProviderCredentialsInfo, 0, len(providers))

    for _, providerID := range providers {
        // Get token store (works with both backends)
        store, err := s.GetTokenStore(providerID)
        if err != nil {
            s.logger.WarnLog("[Server] Failed to get store for provider %s: %v", providerID, err)
            continue
        }

        // Load tokens from store (works with both backends)
        tokensMap, err := store.Load()
        if err != nil {
            s.logger.WarnLog("[Server] Failed to load tokens for provider %s: %v", providerID, err)
            continue
        }

        // Convert to response format
        tokens := make([]ProviderTokenInfo, 0, len(tokensMap))
        for _, token := range tokensMap {
            tokens = append(tokens, ProviderTokenInfo{
                ID:          token.ID,
                Email:       token.Email,
                ExpiryDate:  token.ExpiryDate,
                ExpiresIn:   (token.ExpiryDate - time.Now().UnixMilli()) / 1000,
                TokenType:   token.TokenType,
                Healthy:     token.Healthy,
                HealthScore: token.HealthScore,
                LastUsed:    token.LastUsed,
                CreatedAt:   token.CreatedAt,
                ErrorCount:  token.ErrorCount,
            })
        }

        // Get settings (file-based only, SQLite has separate method)
        var settings tokpkg.StoreSettings
        if fileStore, ok := store.(*tokpkg.MultiTokenStore); ok {
            settings = fileStore.Settings
        } else if sqliteStore, ok := store.(interface{ LoadSettings() (tokpkg.StoreSettings, error) }); ok {
            if s, err := sqliteStore.LoadSettings(); err == nil {
                settings = s
            }
        }

        credentials = append(credentials, ProviderCredentialsInfo{
            ProviderID: providerID,
            Tokens:     tokens,
            Settings:   settings,
        })
    }

    // Return response
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(map[string]interface{}{
        "object": "list",
        "data":   credentials,
    })
}
```

### Refactoring Checklist

- [ ] Update `tokenStores` field type to `map[string]tokpkg.TokenStore`
- [ ] Update `NewServer()` constructor
- [ ] Update `GetTokenStore()` method
- [ ] Verify `handleCallback()` works with interface
- [ ] Update `handleCredentials()` to handle both backends
- [ ] Add type assertions for backend-specific operations
- [ ] Add unit tests for refactored methods

---

## Proxy Handler Refactoring

### Current Implementation

The current [`OpenAIHandler`](../proxy/openai_handler.go) gets tokens via [`BaseHandler`](../proxy/base.go):

```go
type BaseHandler struct {
    logger       logging.Logger
    tokenManager *auth.TokenManager  // Per-provider token manager
}
```

### Refactoring Strategy

**No changes required.** The proxy handler uses `TokenManager` which already uses the `TokenStore` interface indirectly through `MultiTokenManager`. The storage backend abstraction is already in place.

### Verification

Ensure the token retrieval flow works with both backends:

```
OpenAIHandler.handleChatCompletions()
    ↓
Provider.Authenticator.GetToken()
    ↓
TokenManager.SelectToken()
    ↓
MultiTokenManager.GetTokenStore(providerID)  // Returns TokenStore interface
    ↓
StoreFactory.CreateStore(providerID)  // Creates appropriate backend
    ↓
TokenStore.Load()  // Works with both backends
    ↓
Filter and select token
    ↓
TokenStore.Save()  // Works with both backends
```

### Refactoring Checklist

- [ ] Verify token retrieval flow works with both backends
- [ ] Add integration tests for both storage backends
- [ ] No code changes required (already abstracted)

---

## Token Refresh Refactoring

### Current Implementation

The current [`RefreshCoordinator`](../internal/token/token_refresh.go) uses concrete `MultiTokenStore`:

```go
type RefreshCoordinator struct {
    store        *MultiTokenStore  // Concrete type
    refreshers   map[string]ProviderRefresh
    requestQueue chan RefreshRequest
    resultQueue  chan RefreshResult
    workers      int
    ctx          context.Context
    cancel       context.CancelFunc
    wg           sync.WaitGroup
    mu           sync.RWMutex
    logger       logging.Logger
}
```

### Refactored Implementation

Change `store` to use `TokenStore` interface:

```go
// internal/token/token_refresh.go

type RefreshCoordinator struct {
    store        TokenStore  // CHANGED: Use interface
    refreshers   map[string]ProviderRefresh
    requestQueue chan RefreshRequest
    resultQueue  chan RefreshResult
    workers      int
    ctx          context.Context
    cancel       context.CancelFunc
    wg           sync.WaitGroup
    mu           sync.RWMutex
    logger       logging.Logger
}

func NewRefreshCoordinator(store TokenStore, workers int, logger logging.Logger, clientFactory ProxyClientFactory) *RefreshCoordinator {
    if workers <= 0 {
        workers = 3
    }
    if logger == nil {
        logger = logging.NewLogger()
    }

    ctx, cancel := context.WithCancel(context.Background())

    return &RefreshCoordinator{
        store:        store,  // Interface type
        refreshers:   make(map[string]ProviderRefresh),
        requestQueue: make(chan RefreshRequest, 100),
        resultQueue:  make(chan RefreshResult, 100),
        workers:      workers,
        ctx:          ctx,
        cancel:       cancel,
        logger:       logger,
    }
}
```

### Updated Process Request Method

```go
// internal/token/token_refresh.go

func (rc *RefreshCoordinator) processRequest(request RefreshRequest) {
    startTime := time.Now()
    result := RefreshResult{TokenID: request.TokenID}

    rc.mu.RLock()
    refresher, ok := rc.refreshers[request.ProviderID]
    rc.mu.RUnlock()

    if !ok {
        result.Success = false
        result.Error = fmt.Errorf("no refresher registered for provider: %s", request.ProviderID)
        result.Duration = time.Since(startTime)
        rc.sendResult(result)
        return
    }

    newToken, err := refresher.RefreshToken(rc.ctx, request.Token)
    result.Duration = time.Since(startTime)
    if err != nil {
        result.Success = false
        result.Error = err

        // Mark token as unhealthy (works with both backends via interface)
        tokens, loadErr := rc.store.Load()
        if loadErr == nil {
            if token, exists := tokens[request.TokenID]; exists {
                token.Healthy = false
                token.ErrorCount++
                token.LastError = err.Error()
                tokens[request.TokenID] = token
                _ = rc.store.Save(tokens)
            }
        }

        rc.sendResult(result)
        return
    }

    // Update token with new values (works with both backends via interface)
    tokens, loadErr := rc.store.Load()
    if loadErr != nil {
        result.Success = false
        result.Error = fmt.Errorf("failed to load tokens: %w", loadErr)
        result.Duration = time.Since(startTime)
        rc.sendResult(result)
        return
    }

    if token, exists := tokens[request.TokenID]; exists {
        token.AccessToken = newToken.AccessToken
        token.RefreshToken = newToken.RefreshToken
        token.ExpiryDate = newToken.ExpiryDate
        token.LastUsed = GetCurrentTimestamp()
        token.Healthy = true
        token.ErrorCount = 0
        token.LastError = ""
        tokens[request.TokenID] = token

        if saveErr := rc.store.Save(tokens); saveErr != nil {
            result.Success = false
            result.Error = fmt.Errorf("failed to save token: %w", saveErr)
        } else {
            result.Success = true
            result.NewToken = &newToken
        }
    }

    result.Duration = time.Since(startTime)
    rc.sendResult(result)
}
```

### Updated RefreshScheduler

The [`RefreshScheduler`](../internal/token/token_refresh.go) also needs to use the interface:

```go
// internal/token/token_refresh.go

type RefreshScheduler struct {
    store          TokenStore  // CHANGED: Use interface
    coordinator    *RefreshCoordinator
    checkInterval  time.Duration
    providerID     string
    stopChan       chan struct{}
    mu             sync.RWMutex
    logger         logging.Logger
}

func NewRefreshScheduler(store TokenStore, providerID string, checkInterval time.Duration, logger logging.Logger) *RefreshScheduler {
    return &RefreshScheduler{
        store:         store,  // Interface type
        coordinator:   nil,
        checkInterval: checkInterval,
        providerID:    providerID,
        stopChan:      make(chan struct{}),
        logger:        logger,
    }
}
```

### Refactoring Checklist

- [ ] Update `RefreshCoordinator.store` to `TokenStore` interface
- [ ] Update `NewRefreshCoordinator()` to accept `TokenStore`
- [ ] Update `RefreshScheduler.store` to `TokenStore` interface
- [ ] Update `NewRefreshScheduler()` to accept `TokenStore`
- [ ] Update `processRequest()` to work with interface
- [ ] Add unit tests for refactored methods

---

## File-to-Database Migration Integration

### Migration Utility

The migration utility is defined in [`internal/token/migrator.go`](../internal/token/migrator.go):

```go
// internal/token/migrator.go

package token

import (
    "fmt"
    "os"
    "path/filepath"
    "time"

    "github.com/sunbankio/qwencoder-proxy/logging"
)

// Migrator handles migration from file-based to SQLite storage
type Migrator struct {
    fileStore *MultiTokenStore
    dbStore   *SQLiteStore
    logger    logging.Logger
}

// NewMigrator creates a new migrator
func NewMigrator(fileStore *MultiTokenStore, dbStore *SQLiteStore, logger logging.Logger) *Migrator {
    return &Migrator{
        fileStore: fileStore,
        dbStore:   dbStore,
        logger:    logger,
    }
}

// Migrate performs the migration from file to SQLite
func (m *Migrator) Migrate() (*MigrationResult, error) {
    m.logger.InfoLog("[Migrator] Starting migration from file-based to SQLite storage")

    result := &MigrationResult{
        StartTime: time.Now(),
        TokensMigrated: 0,
        TokensFailed:    0,
        SettingsMigrated: false,
    }

    // Load all tokens from file store
    fileTokens, err := m.fileStore.Load()
    if err != nil {
        return nil, fmt.Errorf("failed to load tokens from file store: %w", err)
    }

    m.logger.InfoLog("[Migrator] Found %d tokens to migrate", len(fileTokens))

    // Migrate each token
    for tokenID, token := range fileTokens {
        if err := m.dbStore.AddToken(token); err != nil {
            m.logger.ErrorLog("[Migrator] Failed to migrate token %s: %v", tokenID, err)
            result.TokensFailed++
            result.Errors = append(result.Errors, fmt.Sprintf("Token %s: %v", tokenID, err))
            continue
        }
        m.logger.DebugLog("[Migrator] Migrated token %s (email: %s)", tokenID, token.Email)
        result.TokensMigrated++
    }

    // Migrate settings
    settings := m.fileStore.Settings
    if err := m.dbStore.SaveSettings(settings); err != nil {
        m.logger.ErrorLog("[Migrator] Failed to migrate settings: %v", err)
        result.Errors = append(result.Errors, fmt.Sprintf("Settings: %v", err))
    } else {
        result.SettingsMigrated = true
    }

    result.EndTime = time.Now()
    result.Duration = result.EndTime.Sub(result.StartTime)
    result.Success = result.TokensFailed == 0

    if result.Success {
        m.logger.InfoLog("[Migrator] Migration completed successfully in %v", result.Duration)
    } else {
        m.logger.ErrorLog("[Migrator] Migration completed with %d errors in %v", len(result.Errors), result.Duration)
    }

    return result, nil
}

// MigrationResult represents the result of a migration operation
type MigrationResult struct {
    StartTime         time.Time
    EndTime           time.Time
    Duration          time.Duration
    TokensMigrated   int
    TokensFailed      int
    SettingsMigrated bool
    Success           bool
    Errors            []string
}

// BackupFileStore creates a backup of the file-based storage
func (m *Migrator) BackupFileStore() (string, error) {
    backupDir := ".credentials.backup." + time.Now().Format("20060102-150405")

    m.logger.InfoLog("[Migrator] Creating backup at: %s", backupDir)

    if err := os.MkdirAll(backupDir, 0755); err != nil {
        return "", fmt.Errorf("failed to create backup directory: %w", err)
    }

    // Copy all provider directories
    providerDir := filepath.Dir(m.fileStore.providerDir)
    entries, err := os.ReadDir(providerDir)
    if err != nil {
        return "", fmt.Errorf("failed to read provider directory: %w", err)
    }

    for _, entry := range entries {
        if !entry.IsDir() {
            continue
        }

        src := filepath.Join(providerDir, entry.Name())
        dst := filepath.Join(backupDir, entry.Name())

        if err := copyDir(src, dst); err != nil {
            m.logger.ErrorLog("[Migrator] Failed to backup %s: %v", entry.Name(), err)
            continue
        }

        m.logger.DebugLog("[Migrator] Backed up: %s", entry.Name())
    }

    return backupDir, nil
}

// copyDir recursively copies a directory
func copyDir(src, dst string) error {
    return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
        if err != nil {
            return err
        }

        relPath, err := filepath.Rel(src, path)
        if err != nil {
            return err
        }

        dstPath := filepath.Join(dst, relPath)

        if info.IsDir() {
            return os.MkdirAll(dstPath, info.Mode())
        }

        return copyFile(path, dstPath)
    })
}

// copyFile copies a file
func copyFile(src, dst string) error {
    data, err := os.ReadFile(src)
    if err != nil {
        return err
    }
    return os.WriteFile(dst, data, 0644)
}
```

### Integration Points

The migration utility needs to be integrated at the following points:

#### 1. Main Entry Point Integration

```go
// cmd/qwencoder-proxy/main.go

func handleMigration(mtm *token.MultiTokenManager, logger logging.Logger) error {
    logger.InfoLog("Starting migration from file-based to SQLite storage")

    // Get all providers
    providers := mtm.ListProviders()

    totalMigrated := 0
    totalFailed := 0

    for _, providerID := range providers {
        // Get file-based store
        fileStore, err := mtm.GetFileStore(providerID)
        if err != nil {
            logger.WarnLog("No file-based store for provider %s, skipping", providerID)
            continue
        }

        // Get SQLite store
        dbStore, err := mtm.GetSQLiteStore(providerID)
        if err != nil {
            logger.WarnLog("No SQLite store for provider %s, skipping", providerID)
            continue
        }

        // Create migrator
        migrator := token.NewMigrator(fileStore, dbStore, logger)

        // Perform migration
        result, err := migrator.Migrate()
        if err != nil {
            logger.ErrorLog("Migration failed for provider %s: %v", providerID, err)
            totalFailed++
            continue
        }

        totalMigrated += result.TokensMigrated
        totalFailed += result.TokensFailed

        logger.InfoLog("Migration for provider %s: %d migrated, %d failed",
            providerID, result.TokensMigrated, result.TokensFailed)
    }

    logger.InfoLog("Migration complete: %d tokens migrated, %d failed", totalMigrated, totalFailed)

    if totalFailed > 0 {
        return fmt.Errorf("migration completed with %d errors", totalFailed)
    }

    return nil
}
```

#### 2. REST API Migration Endpoint

Add a migration endpoint to the REST API:

```go
// restapi/rest_api.go

// handleMigration handles migration from file to SQLite
func (s *Server) handleMigration(w http.ResponseWriter, r *http.Request) {
    s.logger.InfoLog("[Server] Migration requested via API")

    // Check if migration is already in progress
    if s.migrationInProgress {
        http.Error(w, "Migration already in progress", http.StatusConflict)
        return
    }

    s.migrationInProgress = true
    defer func() {
        s.migrationInProgress = false
    }()

    // Perform migration
    result, err := handleMigration(s.multiTokenManager, s.logger)
    if err != nil {
        s.logger.ErrorLog("[Server] Migration failed: %v", err)
        http.Error(w, fmt.Sprintf("Migration failed: %v", err), http.StatusInternalServerError)
        return
    }

    // Return success response
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(map[string]interface{}{
        "success": true,
        "message": "Migration completed successfully",
        "result":  result,
    })
}
```

### Migration Trigger Conditions

The migration can be triggered in the following ways:

| Trigger | Description |
|----------|-------------|
| **Startup Auto-Detect** | When `backend=auto` and file-based storage exists, prompt for migration |
| **Command-Line Flag** | `--migrate` flag forces migration |
| **REST API Endpoint** | `POST /api/migrate` triggers migration |
| **Configuration** | `MIGRATE_ON_STARTUP=true` enables automatic migration |

### Migration Progress Tracking

Track migration progress for user feedback:

```go
// internal/token/migrator.go

type MigrationProgress struct {
    ProviderID    string
    TotalTokens   int
    MigratedTokens int
    FailedTokens  int
    CurrentToken  string
    Status        string // "pending", "in_progress", "completed", "failed"
    Error         string
}

type MigrationProgressCallback func(progress MigrationProgress)

// MigrateWithProgress performs migration with progress callbacks
func (m *Migrator) MigrateWithProgress(callback MigrationProgressCallback) (*MigrationResult, error) {
    m.logger.InfoLog("[Migrator] Starting migration with progress tracking")

    result := &MigrationResult{
        StartTime: time.Now(),
        TokensMigrated: 0,
        TokensFailed:    0,
        SettingsMigrated: false,
    }

    // Load all tokens from file store
    fileTokens, err := m.fileStore.Load()
    if err != nil {
        return nil, fmt.Errorf("failed to load tokens from file store: %w", err)
    }

    totalTokens := len(fileTokens)
    callback(MigrationProgress{
        ProviderID:    m.dbStore.ProviderID,
        TotalTokens:   totalTokens,
        MigratedTokens: 0,
        FailedTokens:  0,
        Status:        "pending",
    })

    // Migrate each token
    for tokenID, token := range fileTokens {
        callback(MigrationProgress{
            ProviderID:    m.dbStore.ProviderID,
            TotalTokens:   totalTokens,
            MigratedTokens: result.TokensMigrated,
            FailedTokens:  result.TokensFailed,
            CurrentToken:  tokenID,
            Status:        "in_progress",
        })

        if err := m.dbStore.AddToken(token); err != nil {
            m.logger.ErrorLog("[Migrator] Failed to migrate token %s: %v", tokenID, err)
            result.TokensFailed++
            result.Errors = append(result.Errors, fmt.Sprintf("Token %s: %v", tokenID, err))

            callback(MigrationProgress{
                ProviderID:    m.dbStore.ProviderID,
                TotalTokens:   totalTokens,
                MigratedTokens: result.TokensMigrated,
                FailedTokens:  result.TokensFailed,
                CurrentToken:  tokenID,
                Status:        "failed",
                Error:         err.Error(),
            })
            continue
        }

        m.logger.DebugLog("[Migrator] Migrated token %s (email: %s)", tokenID, token.Email)
        result.TokensMigrated++
    }

    // Migrate settings
    settings := m.fileStore.Settings
    if err := m.dbStore.SaveSettings(settings); err != nil {
        m.logger.ErrorLog("[Migrator] Failed to migrate settings: %v", err)
        result.Errors = append(result.Errors, fmt.Sprintf("Settings: %v", err))
    } else {
        result.SettingsMigrated = true
    }

    result.EndTime = time.Now()
    result.Duration = result.EndTime.Sub(result.StartTime)
    result.Success = result.TokensFailed == 0

    callback(MigrationProgress{
        ProviderID:    m.dbStore.ProviderID,
        TotalTokens:   totalTokens,
        MigratedTokens: result.TokensMigrated,
        FailedTokens:  result.TokensFailed,
        Status:        result.Success ? "completed" : "failed",
    })

    if result.Success {
        m.logger.InfoLog("[Migrator] Migration completed successfully in %v", result.Duration)
    } else {
        m.logger.ErrorLog("[Migrator] Migration completed with %d errors in %v", len(result.Errors), result.Duration)
    }

    return result, nil
}
```

### Refactoring Checklist

- [ ] Create [`internal/token/migrator.go`](../internal/token/migrator.go)
- [ ] Implement `Migrator` struct
- [ ] Implement `Migrate()` method
- [ ] Implement `BackupFileStore()` method
- [ ] Implement `MigrateWithProgress()` method
- [ ] Add migration endpoint to REST API
- [ ] Add migration to main entry point
- [ ] Add unit tests for migration

---

## Deprecation and Cleanup Plan

### Deprecation Timeline

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                        Deprecation Timeline                                │
│                                                                         │
│  Phase 1: Current Release (Non-Breaking)                              │
│  ┌───────────────────────────────────────────────────────────────────────────┐  │
│  │  - Add SQLite support alongside file-based storage                   │  │
│  │  - File-based storage remains default                                  │  │
│  │  - No deprecation warnings                                          │  │
│  └───────────────────────────────────────────────────────────────────────────┘  │
│                                                                         │
│  Phase 2: Next Release (Default to SQLite)                               │
│  ┌───────────────────────────────────────────────────────────────────────────┐  │
│  │  - SQLite becomes default backend                                      │  │
│  │  - Add deprecation warning for file-based storage                    │  │
│  │  - File-based storage remains available                                 │  │
│  └───────────────────────────────────────────────────────────────────────────┘  │
│                                                                         │
│  Phase 3: Following Release (Warning)                                     │
│  ┌───────────────────────────────────────────────────────────────────────────┐  │
│  │  - Stronger deprecation warnings                                      │  │
│  │  - Documentation emphasizes migration                                  │  │
│  │  - File-based storage marked as "legacy"                              │  │
│  └───────────────────────────────────────────────────────────────────────────┘  │
│                                                                         │
│  Phase 4: Future Release (Removal)                                        │
│  ┌───────────────────────────────────────────────────────────────────────────┐  │
│  │  - File-based storage code removed                                    │  │
│  │  - Migration utilities removed                                         │  │
│  │  - Documentation updated                                             │  │
│  └───────────────────────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────────────────────┘
```

### Deprecation Warnings

Add deprecation warnings at appropriate points:

#### 1. Configuration Deprecation Warning

```go
// config/config.go

func (c *Config) ValidateStorageConfig() error {
    switch c.Storage.Backend {
    case "file":
        // Log deprecation warning
        log.Printf("[DEPRECATION] File-based storage is deprecated. " +
            "Please migrate to SQLite storage. " +
            "See documentation for migration instructions.")
    case "sqlite", "auto":
        // Valid backend
    case "":
        // Default to auto
        c.Storage.Backend = "auto"
    default:
        return fmt.Errorf("invalid storage backend: %s", c.Storage.Backend)
    }
    return nil
}
```

#### 2. Store Creation Deprecation Warning

```go
// internal/token/store_factory.go

func (sf *StoreFactory) createFileStore(providerID string) (TokenStore, error) {
    sf.logger.WarnLog("[DEPRECATION] File-based storage is deprecated. " +
        "Consider migrating to SQLite for better performance and reliability. " +
        "See documentation for migration instructions.")

    // ... rest of implementation ...
}
```

#### 3. Documentation Deprecation Notice

Add deprecation notice to README:

```markdown
## Storage Configuration

### Storage Backends

The qwencoder-proxy supports multiple storage backends for token persistence:

| Backend | Status | Description |
|----------|--------|-------------|
| `sqlite` | **Recommended** | SQLite database storage. Best performance, ACID transactions, concurrent access. |
| `file` | **Deprecated** | File-based JSON storage. Legacy implementation. Will be removed in future version. |
| `auto` | Default | Auto-detect storage backend based on existing data. |

### Migration from File-Based Storage

**File-based storage is deprecated and will be removed in a future release.**

To migrate from file-based to SQLite storage:

1. Set `STORAGE_BACKEND=sqlite` environment variable
2. Start the server - migration will be performed automatically
3. Verify migration was successful
4. Optionally, backup and remove `.credentials/` directory

See [Migration Guide](./docs/migration/migration-guide.md) for detailed instructions.
```

### Cleanup Plan

#### Phase 1: Preparation (Current Release)

- [ ] Add deprecation warnings to file-based storage
- [ ] Document migration path
- [ ] Add migration utilities
- [ ] Ensure backward compatibility

#### Phase 2: Default to SQLite (Next Release)

- [ ] Change default backend to "sqlite"
- [ ] Add stronger deprecation warnings
- [ ] Update all documentation
- [ ] Add migration guide

#### Phase 3: Warning Release (Following Release)

- [ ] Add prominent deprecation notices
- [ ] Log warnings on every startup with file backend
- [ ] Add removal timeline to documentation

#### Phase 4: Removal (Future Release)

- [ ] Remove `MultiTokenStore` implementation
- [ ] Remove file-based storage code
- [ ] Remove migration utilities
- [ ] Remove `file` backend option
- [ ] Update all documentation

### Refactoring Checklist

- [ ] Add deprecation warnings to configuration validation
- [ ] Add deprecation warnings to store factory
- [ ] Update README with deprecation notice
- [ ] Create migration guide documentation
- [ ] Add timeline for removal
- [ ] Document cleanup steps

---

## Implementation Phases

### Phase 1: Foundation (Week 1)

**Objective:** Add SQLite storage layer and configuration support

**Tasks:**
1. Add `modernc.org/sqlite` dependency to [`go.mod`](../go.mod)
2. Create [`internal/token/sqlite_store.go`](../internal/token/sqlite_store.go)
3. Implement database schema and migrations
4. Implement basic CRUD operations
5. Create [`internal/token/migrator.go`](../internal/token/migrator.go)
6. Add `StorageConfig` to [`config/config.go`](../config/config.go)
7. Update `DefaultConfig()` function
8. Add `ValidateStorageConfig()` function

**Deliverables:**
- SQLite storage layer with schema v1
- Migration utility
- Storage configuration support
- Unit tests for new components

**Acceptance Criteria:**
- All unit tests pass
- SQLite store implements `TokenStore` interface
- Configuration validation works
- Migration utility works

### Phase 2: Integration (Week 2)

**Objective:** Integrate SQLite storage into existing architecture

**Tasks:**
1. Create [`internal/token/store_factory.go`](../internal/token/store_factory.go)
2. Update [`MultiTokenManager`](../internal/token/multi_token_manager.go) to use factory
3. Update [`RefreshCoordinator`](../internal/token/token_refresh.go) to use interface
4. Update [`RefreshScheduler`](../internal/token/token_refresh.go) to use interface
5. Update [`Server`](../restapi/rest_api.go) to use interface
6. Update [`main()`](../cmd/qwencoder-proxy/main.go) to use storage config

**Deliverables:**
- Store factory implementation
- Refactored token manager
- Refactored refresh coordinator
- Refactored REST API server
- Updated main entry point

**Acceptance Criteria:**
- Server starts with file-based storage (existing behavior)
- Server starts with SQLite storage (new behavior)
- All existing tests pass
- No breaking changes

### Phase 3: Testing (Week 3)

**Objective:** Comprehensive testing of both storage backends

**Tasks:**
1. Create unit tests for SQLite store
2. Create unit tests for store factory
3. Create unit tests for migrator
4. Create integration tests for OAuth flow with SQLite
5. Create integration tests for token refresh with SQLite
6. Performance benchmarking (file vs SQLite)
7. Concurrent access testing
8. Data integrity validation

**Deliverables:**
- Comprehensive test suite
- Performance benchmarks
- Test coverage report

**Acceptance Criteria:**
- Test coverage > 80% for new code
- Performance benchmarks show SQLite is at least as fast as file-based
- No data loss or corruption in concurrent access tests

### Phase 4: Documentation (Week 4)

**Objective:** Update documentation for new features

**Tasks:**
1. Update README with storage configuration
2. Create migration guide for users
3. Update API documentation
4. Create troubleshooting guide
5. Add deprecation notices

**Deliverables:**
- Updated README
- Migration guide
- Troubleshooting guide
- API documentation updates

**Acceptance Criteria:**
- All documentation is clear and accurate
- Migration guide is tested and verified
- Deprecation notices are clear

### Phase 5: Default to SQLite (Week 5)

**Objective:** Change default storage backend to SQLite

**Tasks:**
1. Change default backend to "sqlite"
2. Add deprecation warnings for file-based storage
3. Update examples and tutorials
4. Write release notes
5. Update version number

**Deliverables:**
- Default configuration uses SQLite
- Deprecation warnings added
- Release notes
- Version bump

**Acceptance Criteria:**
- New installations use SQLite by default
- Existing installations can continue using file-based storage
- Clear migration path documented
- Deprecation warnings are visible

### Phase 6: Cleanup (Future Release)

**Objective:** Remove file-based storage code

**Tasks:**
1. Remove `MultiTokenStore` implementation
2. Remove file-based storage code
3. Remove migration utilities
4. Remove `file` backend option
5. Update all documentation
6. Final release

**Deliverables:**
- Codebase without file-based storage
- Updated documentation
- Final release notes

**Acceptance Criteria:**
- All file-based storage code removed
- Documentation reflects SQLite-only storage
- Release notes document changes

---

## Testing Strategy

### Unit Tests

**Test Files:**
- `internal/token/sqlite_store_test.go`
- `internal/token/store_factory_test.go`
- `internal/token/migrator_test.go`
- `config/config_test.go`

**Test Coverage:**
- CRUD operations (Create, Read, Update, Delete)
- Configuration validation
- Store factory creation
- Migration operations
- Error handling

### Integration Tests

**Test File:**
- `internal/token/integration_test.go`

**Test Scenarios:**
1. Full OAuth flow with SQLite storage
2. Token refresh with SQLite storage
3. Concurrent token access
4. Migration from file to SQLite
5. Backend switching

### Performance Benchmarks

**Test File:**
- `internal/token/benchmark_test.go`

**Benchmarks:**
- Load tokens (100, 1000, 10000)
- Save tokens (100, 1000, 10000)
- Get valid tokens
- Update token
- Select token by strategy

### Migration Tests

**Test Scenarios:**
1. Migrate empty file store
2. Migrate single token
3. Migrate multiple tokens
4. Migrate with settings
5. Migrate with proxy configs
6. Verify data integrity after migration

---

## Rollback Plan

### Rollback Triggers

1. **Critical Bug**: Data corruption or loss during migration
2. **Performance Degradation**: SQLite is significantly slower than file-based storage
3. **Compatibility Issues**: Problems with specific platforms or configurations

### Rollback Procedure

1. **Immediate Rollback** (if caught during Phase 1-2):
   - Revert code changes
   - Keep file-based storage as default
   - Document issues found

2. **Graceful Rollback** (if caught after default change):
   - Add configuration option to force file-based storage
   - Update documentation
   - Provide migration back to file-based storage

3. **Data Recovery**:
   - Keep backup of `.credentials/` directory
   - Provide utility to export from SQLite back to JSON files

### Rollback Verification

1. Start server with file-based storage
2. Verify all tokens are accessible
3. Test OAuth flow
4. Test token refresh
5. Test `/v1/chat/completions` endpoint

---

## Summary

This code refactoring plan provides a comprehensive strategy for integrating SQLite storage into the qwencoder-proxy project. The key principles are:

1. **Non-Breaking**: All changes maintain backward compatibility
2. **Interface-Based**: Storage abstraction enables easy backend switching
3. **Factory Pattern**: Centralized store creation with backend selection
4. **Gradual Migration**: Phased approach allows testing and validation
5. **Clear Deprecation Path**: File-based storage is deprecated with clear timeline

The refactoring ensures minimal disruption to existing users while providing a clear path forward for new installations.

### Related Documents

- [`01-current-architecture-analysis.md`](./01-current-architecture-analysis.md)
- [`02-sqlite-database-schema-design.md`](./02-sqlite-database-schema-design.md)
- [`03-database-initialization-strategy.md`](./03-database-initialization-strategy.md)
- [`04-sqlite-storage-layer-design.md`](./04-sqlite-storage-layer-design.md)
