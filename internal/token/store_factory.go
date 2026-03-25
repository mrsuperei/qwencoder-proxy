package token

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"github.com/sunbankio/qwencoder-proxy/internal/logging"
)

// StoreConfig holds configuration for creating a token store
type StoreConfig struct {
	DBPath     string // Path to SQLite database
	ProviderID string // Provider identifier
	Logger     logging.Logger
	DB         *sql.DB // Optional shared database connection (for sharing with rate limiting)
}

// NewTokenStore creates a SQLite-backed token store
func NewTokenStore(config StoreConfig) (TokenStore, error) {
	return newSQLiteStore(config)
}

// newSQLiteStore creates a new SQLite-backed token store
func newSQLiteStore(config StoreConfig) (TokenStore, error) {
	// Ensure database directory exists
	dbDir := filepath.Dir(config.DBPath)
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory %s: %w", dbDir, err)
	}

	// Create SQLite store with shared database connection if provided
	var store TokenStore
	var err error
	if config.DB != nil {
		// Use shared database connection (for rate limiting)
		store, err = NewSQLiteStoreWithDB(config.DBPath, config.ProviderID, config.Logger, config.DB)
	} else {
		// Create new database connection (backward compatibility)
		store, err = NewSQLiteStore(config.DBPath, config.ProviderID, config.Logger)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to create SQLite store: %w", err)
	}

	config.Logger.InfoLog("[StoreFactory] Created SQLite store for provider %s at %s", config.ProviderID, config.DBPath)
	return store, nil
}

// DefaultStoreConfig returns a default store configuration
func DefaultStoreConfig(providerID string, logger logging.Logger) StoreConfig {
	return StoreConfig{
		DBPath:     filepath.Join(".credentials", "tokens.db"),
		ProviderID: providerID,
		Logger:     logger,
	}
}
