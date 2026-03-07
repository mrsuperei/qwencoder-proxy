package token

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/sunbankio/qwencoder-proxy/logging"
	_ "modernc.org/sqlite"
)

// Schema constants
const (
	// Table names
	tableTokens           = "tokens"
	tableProviderSettings = "provider_settings"
	tableProxyConfigs     = "proxy_configs"
	tableSchemaMigrations = "schema_migrations"

	// Query timeouts
	defaultQueryTimeout = 30 * time.Second
)

// SQL statements
const (
	sqlCreateTokensTable = `
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
			proxy_id TEXT
		)
	`

	sqlCreateProviderSettingsTable = `
		CREATE TABLE IF NOT EXISTS provider_settings (
			provider_id TEXT PRIMARY KEY,
			selection_strategy TEXT NOT NULL DEFAULT 'random',
			refresh_buffer_sec INTEGER NOT NULL DEFAULT 1800,
			max_error_count INTEGER NOT NULL DEFAULT 3,
			updated_at INTEGER NOT NULL DEFAULT (strftime('%s', 'subsec') * 1000)
		)
	`

	sqlCreateProxyConfigsTable = `
		CREATE TABLE IF NOT EXISTS proxy_configs (
			id TEXT PRIMARY KEY,
			host TEXT NOT NULL,
			port INTEGER NOT NULL,
			username TEXT,
			password TEXT,
			created_at INTEGER NOT NULL DEFAULT (strftime('%s', 'subsec') * 1000)
		)
	`

	sqlCreateSchemaMigrationsTable = `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			applied_at INTEGER NOT NULL DEFAULT (strftime('%s', 'subsec') * 1000),
			description TEXT
		)
	`

	// Index creation statements
	sqlCreateIndexTokensProviderID      = "CREATE INDEX IF NOT EXISTS idx_tokens_provider_id ON tokens(provider_id)"
	sqlCreateIndexTokensEmail           = "CREATE INDEX IF NOT EXISTS idx_tokens_email ON tokens(email)"
	sqlCreateIndexTokensExpiryDate      = "CREATE INDEX IF NOT EXISTS idx_tokens_expiry_date ON tokens(expiry_date)"
	sqlCreateIndexTokensProjectID       = "CREATE INDEX IF NOT EXISTS idx_tokens_project_id ON tokens(project_id)"
	sqlCreateIndexTokensHealthy         = "CREATE INDEX IF NOT EXISTS idx_tokens_healthy ON tokens(healthy)"
	sqlCreateIndexTokensProviderHealthy = "CREATE INDEX IF NOT EXISTS idx_tokens_provider_healthy ON tokens(provider_id, healthy)"
	sqlCreateIndexTokensProviderExpiry  = "CREATE INDEX IF NOT EXISTS idx_tokens_provider_expiry ON tokens(provider_id, expiry_date)"
)

// SQLiteStore implements token storage using SQLite database
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

	// Statement health tracking
	stmtHealth          *sql.Stmt
	stmtLastHealthCheck time.Time
}

// StatementHealth represents the health status of a prepared statement
type StatementHealth struct {
	Healthy    bool
	LastCheck  time.Time
	ErrorCount int
}

// applyPragmas applies SQLite pragmas for optimal configuration
func applyPragmas(db *sql.DB, logger logging.Logger) error {
	pragmas := []struct {
		name  string
		value string
	}{
		// Enable WAL (Write-Ahead Logging) mode for better concurrency
		{"journal_mode", "WAL"},
		// Set synchronous mode to NORMAL (balance between safety and performance)
		{"synchronous", "NORMAL"},
		// Increase cache size to 10MB (default is 2MB)
		{"cache_size", "-10240"},
		// Enable foreign key constraints
		{"foreign_keys", "ON"},
		// Set busy timeout to 5 seconds (database waits if locked)
		{"busy_timeout", "5000"},
		// Enable query planner improvements
		{"query_only", "0"},
		// Set temp_store to MEMORY (use memory for temporary tables)
		{"temp_store", "2"},
	}

	for _, pragma := range pragmas {
		_, err := db.Exec(fmt.Sprintf("PRAGMA %s = %s", pragma.name, pragma.value))
		if err != nil {
			return fmt.Errorf("failed to set pragma %s: %w", pragma.name, err)
		}

		// Verify pragma was set (for some pragmas)
		if pragma.name == "journal_mode" {
			var mode string
			if err := db.QueryRow(fmt.Sprintf("PRAGMA %s", pragma.name)).Scan(&mode); err != nil {
				logger.WarnLog("[SQLiteStore] Could not verify pragma %s: %v", pragma.name, err)
			} else {
				logger.DebugLog("[SQLiteStore] PRAGMA %s = %s", pragma.name, mode)
			}
		}
	}

	logger.InfoLog("[SQLiteStore] Applied %d SQLite pragmas", len(pragmas))
	return nil
}

// prepareStatements prepares all SQL statements for performance
func (s *SQLiteStore) prepareStatements() error {
	var err error

	// Prepare INSERT statement for tokens
	s.stmtInsertToken, err = s.db.Prepare(`
		INSERT INTO tokens (
			id, provider_id, access_token, refresh_token, token_type,
			expiry_date, email, resource_url, scope, api_key, project_id,
			healthy, health_score, last_used, created_at,
			error_count, last_error, proxy_id
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare insert token statement: %w", err)
	}

	// Prepare SELECT statement by token ID
	s.stmtSelectTokenByID, err = s.db.Prepare(`
		SELECT id, access_token, refresh_token, token_type, expiry_date,
		       email, resource_url, scope, api_key, project_id,
		       healthy, health_score, last_used, created_at, error_count,
		       last_error, proxy_id
		FROM tokens WHERE provider_id = ? AND id = ?
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare select token by ID statement: %w", err)
	}

	// Prepare SELECT statement by provider
	s.stmtSelectTokensByProvider, err = s.db.Prepare(`
		SELECT id, access_token, refresh_token, token_type, expiry_date,
		       email, resource_url, scope, api_key, project_id,
		       healthy, health_score, last_used, created_at, error_count,
		       last_error, proxy_id
		FROM tokens WHERE provider_id = ?
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare select tokens by provider statement: %w", err)
	}

	// Prepare SELECT statement for valid tokens (healthy and not expired)
	s.stmtSelectValidTokens, err = s.db.Prepare(`
		SELECT id, access_token, refresh_token, token_type, expiry_date,
		       email, resource_url, scope, api_key, project_id,
		       healthy, health_score, last_used, created_at, error_count,
		       last_error, proxy_id
		FROM tokens
		WHERE provider_id = ? AND healthy = 1 AND expiry_date > ?
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare select valid tokens statement: %w", err)
	}

	// Prepare UPDATE statement for tokens
	s.stmtUpdateToken, err = s.db.Prepare(`
		UPDATE tokens SET
			access_token = ?, refresh_token = ?, token_type = ?,
			expiry_date = ?, email = ?, resource_url = ?, scope = ?,
			api_key = ?, project_id = ?, healthy = ?, health_score = ?,
			last_used = ?, error_count = ?, last_error = ?
		WHERE provider_id = ? AND id = ?
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare update token statement: %w", err)
	}

	// Prepare DELETE statement for tokens
	s.stmtDeleteToken, err = s.db.Prepare(`
		DELETE FROM tokens WHERE provider_id = ? AND id = ?
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare delete token statement: %w", err)
	}

	// Prepare SELECT statement for provider settings
	s.stmtSelectSettings, err = s.db.Prepare(`
		SELECT selection_strategy, refresh_buffer_sec,
		       max_error_count, updated_at
		FROM provider_settings WHERE provider_id = ?
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare select settings statement: %w", err)
	}

	// Prepare UPSERT statement for provider settings
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

	s.logger.InfoLog("[SQLiteStore] Prepared %d SQL statements", 8)
	return nil
}

// NewSQLiteStore creates a new SQLite-backed token store
func NewSQLiteStore(dbPath, providerID string, logger logging.Logger) (*SQLiteStore, error) {
	// Open database connection
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Verify connection works
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	// Configure connection pool
	// SetMaxOpenConns: Maximum number of open connections to the database
	db.SetMaxOpenConns(25)
	// SetMaxIdleConns: Maximum number of idle connections in the pool
	db.SetMaxIdleConns(5)
	// SetConnMaxLifetime: Maximum amount of time a connection may be reused
	db.SetConnMaxLifetime(5 * time.Minute)
	// SetConnMaxIdleTime: Maximum amount of time a connection may be idle
	db.SetConnMaxIdleTime(1 * time.Minute)

	// Apply SQLite pragmas for optimal performance and safety
	if err := applyPragmas(db, logger); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to apply pragmas: %w", err)
	}

	store := &SQLiteStore{
		db:         db,
		providerID: providerID,
		logger:     logger,
	}

	// Run schema migrations
	if err := store.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	// Prepare statements for performance
	if err := store.prepareStatements(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to prepare statements: %w", err)
	}

	logger.InfoLog("[SQLiteStore] Initialized SQLite store for provider %s at %s", providerID, dbPath)
	return store, nil
}

// migration represents a single database schema migration
type migration struct {
	version     int
	description string
	fn          func() error
}

// migrate runs pending database migrations
func (s *SQLiteStore) migrate() error {
	// Get current schema version
	var version int
	err := s.db.QueryRow("SELECT MAX(version) FROM schema_migrations").Scan(&version)
	if err != nil {
		// Check if the error is because the table doesn't exist
		if strings.Contains(err.Error(), "no such table") {
			// Table doesn't exist, this is a fresh installation
			version = 0
			s.logger.InfoLog("[SQLiteStore] schema_migrations table not found, treating as fresh installation")
		} else {
			return fmt.Errorf("failed to get schema version: %w", err)
		}
	}

	// Run migrations in order
	migrations := []migration{
		{1, "Initial schema with all tables and indexes", func() error { return s.migrateToV1() }},
		{2, "Add project_id column for Gemini provider", func() error { return s.migrateToV2() }},
		// Future migrations here
	}

	for _, m := range migrations {
		if version >= m.version {
			continue // Already applied
		}

		s.logger.InfoLog("[SQLiteStore] Running migration to version %d: %s", m.version, m.description)

		if err := m.fn(); err != nil {
			return fmt.Errorf("migration to v%d failed: %w", m.version, err)
		}

		// Record migration
		if _, err := s.db.Exec(
			"INSERT INTO schema_migrations (version, description) VALUES (?, ?)",
			m.version, m.description,
		); err != nil {
			return fmt.Errorf("failed to record migration: %w", err)
		}

		s.logger.InfoLog("[SQLiteStore] Migration to version %d completed", m.version)
	}

	return nil
}

// migrateToV1 creates the initial database schema (version 1)
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
			proxy_id TEXT
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
			created_at INTEGER NOT NULL DEFAULT (strftime('%s', 'subsec') * 1000)
		)
	`); err != nil {
		return fmt.Errorf("failed to create proxy_configs table: %w", err)
	}

	// Create schema_migrations table
	if _, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			applied_at INTEGER NOT NULL DEFAULT (strftime('%s', 'subsec') * 1000),
			description TEXT
		)
	`); err != nil {
		return fmt.Errorf("failed to create schema_migrations table: %w", err)
	}

	// Create indexes for tokens table
	indexes := []string{
		"CREATE INDEX IF NOT EXISTS idx_tokens_provider_id ON tokens(provider_id)",
		"CREATE INDEX IF NOT EXISTS idx_tokens_email ON tokens(email)",
		"CREATE INDEX IF NOT EXISTS idx_tokens_expiry_date ON tokens(expiry_date)",
		"CREATE INDEX IF NOT EXISTS idx_tokens_project_id ON tokens(project_id)",
		"CREATE INDEX IF NOT EXISTS idx_tokens_healthy ON tokens(healthy)",
		"CREATE INDEX IF NOT EXISTS idx_tokens_provider_healthy ON tokens(provider_id, healthy)",
		"CREATE INDEX IF NOT EXISTS idx_tokens_provider_expiry ON tokens(provider_id, expiry_date)",
	}

	for _, idx := range indexes {
		if _, err := s.db.Exec(idx); err != nil {
			return fmt.Errorf("failed to create index: %w", err)
		}
	}

	return nil
}

// migrateToV2 adds project_id column for Gemini provider
func (s *SQLiteStore) migrateToV2() error {
	// Add project_id column to tokens table
	if _, err := s.db.Exec(`
		ALTER TABLE tokens ADD COLUMN project_id TEXT
	`); err != nil {
		return fmt.Errorf("failed to add project_id column: %w", err)
	}

	// Create index for project_id (optional, for queries filtering by project)
	if _, err := s.db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_tokens_project_id ON tokens(project_id)
	`); err != nil {
		return fmt.Errorf("failed to create project_id index: %w", err)
	}

	s.logger.InfoLog("[SQLiteStore] Migration to V2 completed: added project_id column")
	return nil
}

// boolToInt converts a boolean to integer for SQLite storage
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// intToBool converts an integer to boolean from SQLite storage
func intToBool(i int) bool {
	return i != 0
}

// scanToken scans a token from a database row
func (s *SQLiteStore) scanToken(scanner interface{ Scan(...interface{}) error }) (TokenMetadata, error) {
	var token TokenMetadata
	var proxyID sql.NullString
	var projectID sql.NullString
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
		&projectID,
		&healthy,
		&token.HealthScore,
		&token.LastUsed,
		&token.CreatedAt,
		&token.ErrorCount,
		&lastError,
		&proxyID,
	)

	if err != nil {
		return token, fmt.Errorf("failed to scan token: %w", err)
	}

	token.Healthy = intToBool(healthy)
	if lastError.Valid {
		token.LastError = lastError.String
	}
	if projectID.Valid {
		token.ProjectID = projectID.String
	}

	// Note: proxy_id is stored but ProxyConfig is loaded separately
	// This will be handled in a future phase or by the caller

	return token, nil
}

// bindToken binds token parameters to a prepared statement
func (s *SQLiteStore) bindToken(stmt *sql.Stmt, token TokenMetadata) error {
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
		sql.NullString{String: token.ProjectID, Valid: token.ProjectID != ""},
		boolToInt(token.Healthy),
		token.HealthScore,
		token.LastUsed,
		token.CreatedAt,
		token.ErrorCount,
		sql.NullString{String: token.LastError, Valid: token.LastError != ""},
		nil, // proxy_id - will be handled in future phase
	)
	return err
}

// Load loads all tokens for the provider from SQLite
func (s *SQLiteStore) Load() (map[string]TokenMetadata, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.stmtSelectTokensByProvider.Query(s.providerID)
	if err != nil {
		return nil, fmt.Errorf("failed to query tokens: %w", err)
	}
	defer rows.Close()

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

// Save persists all tokens to SQLite using a transaction
func (s *SQLiteStore) Save(tokens map[string]TokenMetadata) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Start transaction
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Delete existing tokens for this provider
	if _, err := tx.Exec("DELETE FROM tokens WHERE provider_id = ?", s.providerID); err != nil {
		return fmt.Errorf("failed to delete existing tokens: %w", err)
	}

	// Update or insert each token
	for _, token := range tokens {
		// Check if token exists
		var existingID string
		err := tx.QueryRow(
			"SELECT id FROM tokens WHERE provider_id = ? AND id = ?",
			s.providerID, token.ID,
		).Scan(&existingID)

		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			// Unexpected error
			return fmt.Errorf("failed to check for existing token: %w", err)
		}

		if existingID != "" {
			// Token exists, update it
			_, err = tx.Stmt(s.stmtUpdateToken).Exec(
				token.AccessToken,
				token.RefreshToken,
				token.TokenType,
				token.ExpiryDate,
				token.Email,
				token.ResourceURL,
				token.Scope,
				token.APIKey,
				sql.NullString{String: token.ProjectID, Valid: token.ProjectID != ""},
				boolToInt(token.Healthy),
				token.HealthScore,
				token.LastUsed,
				token.ErrorCount,
				sql.NullString{String: token.LastError, Valid: token.LastError != ""},
				s.providerID,
				token.ID,
			)
			if err != nil {
				return fmt.Errorf("failed to update token %s: %w", token.ID, err)
			}
		} else {
			// Token doesn't exist, insert it
			_, err = tx.Stmt(s.stmtInsertToken).Exec(
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
				sql.NullString{String: token.ProjectID, Valid: token.ProjectID != ""},
				boolToInt(token.Healthy),
				token.HealthScore,
				token.LastUsed,
				token.CreatedAt,
				token.ErrorCount,
				sql.NullString{String: token.LastError, Valid: token.LastError != ""},
				nil, // proxy_id
			)
			if err != nil {
				return fmt.Errorf("failed to insert token %s: %w", token.ID, err)
			}
		}
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	s.logger.DebugLog("[SQLiteStore] Saved %d tokens for provider %s", len(tokens), s.providerID)
	return nil
}

// AddToken adds a new token to the store
func (s *SQLiteStore) AddToken(token TokenMetadata) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check for duplicate by refresh_token
	var existingID string
	err := s.db.QueryRow(
		"SELECT id FROM tokens WHERE provider_id = ? AND refresh_token = ?",
		s.providerID, token.RefreshToken,
	).Scan(&existingID)

	if err == nil {
		// Duplicate found, update existing
		s.logger.DebugLog("[SQLiteStore] Duplicate refresh_token found, updating existing token %s with new access_token=%s", existingID, token.AccessToken)
		return s.updateTokenLocked(existingID, token)
	} else if !errors.Is(err, sql.ErrNoRows) {
		// Unexpected error
		return fmt.Errorf("failed to check for duplicate token: %w", err)
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
		sql.NullString{String: token.ProjectID, Valid: token.ProjectID != ""},
		boolToInt(token.Healthy),
		token.HealthScore,
		token.LastUsed,
		token.CreatedAt,
		token.ErrorCount,
		sql.NullString{String: token.LastError, Valid: token.LastError != ""},
		nil, // proxy_id
	)

	if err != nil {
		return fmt.Errorf("failed to insert token: %w", err)
	}

	s.logger.DebugLog("[SQLiteStore] Added token %s (email: %s)", token.ID, token.Email)
	return nil
}

// getTokenLocked retrieves a token by ID (caller must hold lock)
func (s *SQLiteStore) getTokenLocked(tokenID string) (*TokenMetadata, error) {
	row := s.stmtSelectTokenByID.QueryRow(s.providerID, tokenID)
	token, err := s.scanToken(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("token %s not found", tokenID)
		}
		return nil, err
	}

	return &token, nil
}

// GetToken retrieves a token by ID
func (s *SQLiteStore) GetToken(tokenID string) (*TokenMetadata, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.getTokenLocked(tokenID)
}

// UpdateToken updates a token with a function
func (s *SQLiteStore) UpdateToken(tokenID string, updateFunc func(*TokenMetadata)) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.updateTokenLocked(tokenID, updateFunc)
}

// updateTokenLocked updates a token (must hold lock)
func (s *SQLiteStore) updateTokenLocked(tokenID string, updateFunc interface{}) error {
	// Get current token (already holding lock)
	token, err := s.getTokenLocked(tokenID)
	if err != nil {
		return err
	}

	// Apply update
	if fn, ok := updateFunc.(func(*TokenMetadata)); ok {
		fn(token)
	} else if tokenData, ok := updateFunc.(TokenMetadata); ok {
		*token = tokenData
	} else {
		return fmt.Errorf("invalid update function type")
	}

	// Update in database
	s.logger.DebugLog("[SQLiteStore] Updating token %s with access_token=%s", tokenID, token.AccessToken)
	_, err = s.stmtUpdateToken.Exec(
		token.AccessToken,
		token.RefreshToken,
		token.TokenType,
		token.ExpiryDate,
		token.Email,
		token.ResourceURL,
		token.Scope,
		token.APIKey,
		sql.NullString{String: token.ProjectID, Valid: token.ProjectID != ""},
		boolToInt(token.Healthy),
		token.HealthScore,
		token.LastUsed,
		token.ErrorCount,
		sql.NullString{String: token.LastError, Valid: token.LastError != ""},
		s.providerID,
		tokenID,
	)

	if err != nil {
		return fmt.Errorf("failed to update token: %w", err)
	}

	s.logger.DebugLog("[SQLiteStore] Updated token %s", tokenID)
	return nil
}

// RemoveToken removes a token by ID
func (s *SQLiteStore) RemoveToken(tokenID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	result, err := s.stmtDeleteToken.Exec(s.providerID, tokenID)
	if err != nil {
		return fmt.Errorf("failed to delete token: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("token %s not found", tokenID)
	}

	s.logger.DebugLog("[SQLiteStore] Removed token %s", tokenID)
	return nil
}

// GetValidTokens returns all valid (healthy and not expired) tokens
func (s *SQLiteStore) GetValidTokens() []ProviderToken {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Get settings to calculate expiry threshold
	settings, err := s.GetSettings()
	if err != nil {
		s.logger.ErrorLog("[SQLiteStore] Failed to get settings: %v", err)
		return []ProviderToken{}
	}

	// Calculate expiry threshold - token is valid if it hasn't expired or is within refresh buffer
	// Tokens that expired less than 'buffer' time ago are still considered valid
	bufferMs := int64(settings.RefreshBufferSec * 1000)
	nowMs := time.Now().UnixMilli()
	// Use negative buffer to allow tokens that expired within buffer period
	expiryThreshold := nowMs - bufferMs

	// Query for tokens that are still valid (expiry > now - buffer)
	// This means tokens expiring within the buffer are still considered valid
	rows, err := s.stmtSelectValidTokens.Query(s.providerID, expiryThreshold)
	if err != nil {
		s.logger.ErrorLog("[SQLiteStore] Failed to query valid tokens: %v", err)
		return []ProviderToken{}
	}
	defer rows.Close()

	tokens := []ProviderToken{}
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
		s.logger.ErrorLog("[SQLiteStore] Error iterating valid tokens: %v", err)
		return tokens
	}

	s.logger.DebugLog("[SQLiteStore] Found %d valid tokens for provider %s", len(tokens), s.providerID)
	return tokens
}

// IsTokenValid checks if a token is valid (not expired and healthy).
func (s *SQLiteStore) IsTokenValid(token ProviderToken) bool {
	// Get settings to calculate expiry threshold
	settings, err := s.GetSettings()
	if err != nil {
		s.logger.ErrorLog("[SQLiteStore] Failed to get settings: %v", err)
		return false
	}

	// Calculate expiry threshold
	bufferMs := int64(settings.RefreshBufferSec * 1000)
	nowMs := time.Now().UnixMilli()
	expiryThreshold := nowMs + bufferMs

	// Check if token is healthy and not expired
	return token.Healthy && token.ExpiryDate > expiryThreshold
}

// MarkTokenHealthy marks a token as healthy.
func (s *SQLiteStore) MarkTokenHealthy(tokenID string) error {
	return s.UpdateToken(tokenID, func(token *TokenMetadata) {
		token.Healthy = true
		token.HealthScore = 1.0
		token.ErrorCount = 0
		token.LastError = ""
		token.LastUsed = time.Now().UnixMilli()
	})
}

// MarkTokenUnhealthy marks a token as unhealthy.
func (s *SQLiteStore) MarkTokenUnhealthy(tokenID string, err error) error {
	return s.UpdateToken(tokenID, func(token *TokenMetadata) {
		token.Healthy = false
		token.ErrorCount++
		token.HealthScore = max(0.0, token.HealthScore-0.2)
		if err != nil {
			token.LastError = err.Error()
		}
	})
}

// GetValidTokenCount returns the number of valid tokens.
func (s *SQLiteStore) GetValidTokenCount() int {
	return len(s.GetValidTokens())
}

// ListTokens returns all tokens.
func (s *SQLiteStore) ListTokens() []ProviderToken {
	s.mu.RLock()
	defer s.mu.RUnlock()

	tokens, err := s.Load()
	if err != nil {
		s.logger.ErrorLog("[SQLiteStore] Failed to load tokens: %v", err)
		return []ProviderToken{}
	}

	result := make([]ProviderToken, 0, len(tokens))
	for _, token := range tokens {
		result = append(result, token)
	}
	return result
}

// GetSettings implements TokenStore.GetSettings() interface method
// Retrieves provider settings from SQLite
func (s *SQLiteStore) GetSettings() (StoreSettings, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var settings StoreSettings
	var selectionStrategy sql.NullString

	err := s.stmtSelectSettings.QueryRow(s.providerID).Scan(
		&selectionStrategy,
		&settings.RefreshBufferSec,
		&settings.MaxErrorCount,
		&settings.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// Return default settings if not found
			return StoreSettings{
				SelectionStrategy: "random",
				RefreshBufferSec:  1800,
				MaxErrorCount:     3,
				UpdatedAt:         time.Now().UnixMilli(),
			}, nil
		}
		return StoreSettings{}, fmt.Errorf("failed to get settings: %w", err)
	}

	if selectionStrategy.Valid {
		settings.SelectionStrategy = selectionStrategy.String
	} else {
		settings.SelectionStrategy = "random"
	}

	return settings, nil
}

// SaveSettings implements TokenStore.SaveSettings() interface method
// Saves provider settings to SQLite
func (s *SQLiteStore) SaveSettings(settings StoreSettings) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Set default values if not provided
	if settings.SelectionStrategy == "" {
		settings.SelectionStrategy = "random"
	}
	if settings.RefreshBufferSec == 0 {
		settings.RefreshBufferSec = 1800
	}
	if settings.MaxErrorCount == 0 {
		settings.MaxErrorCount = 3
	}
	if settings.UpdatedAt == 0 {
		settings.UpdatedAt = time.Now().UnixMilli()
	}

	_, err := s.stmtUpsertSettings.Exec(
		s.providerID,
		settings.SelectionStrategy,
		settings.RefreshBufferSec,
		settings.MaxErrorCount,
		settings.UpdatedAt,
	)

	if err != nil {
		return fmt.Errorf("failed to save settings: %w", err)
	}

	s.logger.DebugLog("[SQLiteStore] Saved settings for provider %s", s.providerID)
	return nil
}

// Clear removes all tokens for the provider
func (s *SQLiteStore) Clear() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	result, err := s.db.Exec("DELETE FROM tokens WHERE provider_id = ?", s.providerID)
	if err != nil {
		return fmt.Errorf("failed to clear tokens: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	s.logger.InfoLog("[SQLiteStore] Cleared %d tokens for provider %s", rowsAffected, s.providerID)
	return nil
}

// Close closes the database connection and all prepared statements
func (s *SQLiteStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var errs []error

	// Close all prepared statements
	statements := []*sql.Stmt{
		s.stmtInsertToken,
		s.stmtUpdateToken,
		s.stmtSelectTokenByID,
		s.stmtSelectTokensByProvider,
		s.stmtSelectValidTokens,
		s.stmtDeleteToken,
		s.stmtSelectSettings,
		s.stmtUpsertSettings,
	}

	for i, stmt := range statements {
		if stmt != nil {
			if err := stmt.Close(); err != nil {
				errs = append(errs, fmt.Errorf("failed to close statement %d: %w", i, err))
			}
		}
	}

	// Close database connection
	if s.db != nil {
		if err := s.db.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close database: %w", err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("close errors: %v", errs)
	}

	s.logger.InfoLog("[SQLiteStore] Closed SQLite store for provider %s", s.providerID)
	return nil
}

// Ping verifies the database connection is still alive
func (s *SQLiteStore) Ping() error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := s.db.PingContext(ctx); err != nil {
		return fmt.Errorf("database ping failed: %w", err)
	}

	// Verify schema version
	var version int
	err := s.db.QueryRow("SELECT MAX(version) FROM schema_migrations").Scan(&version)
	if err != nil {
		return fmt.Errorf("failed to get schema version: %w", err)
	}

	s.logger.DebugLog("[SQLiteStore] Ping successful, schema version: %d", version)
	return nil
}

// GetCredentialsPath returns the path to the credentials file/directory
func (s *SQLiteStore) GetCredentialsPath() string {
	return ".credentials/tokens.db"
}

// GetTokenCount returns the total number of tokens in the store
func (s *SQLiteStore) GetTokenCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM tokens WHERE provider_id = ?", s.providerID).Scan(&count)
	if err != nil {
		s.logger.ErrorLog("[SQLiteStore] Failed to get token count: %v", err)
		return 0
	}
	return count
}

// StorageType returns the storage type
func (s *SQLiteStore) StorageType() string {
	return "sqlite"
}
