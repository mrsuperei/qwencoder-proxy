package ratelimit

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// MigrateRateLimitingToTokensDB migrates rate limiting data from separate database to main tokens.db
func MigrateRateLimitingToTokensDB(sourceDBPath, targetDBPath string) error {
	// Open source database (rate limiting DB)
	sourceDB, err := sql.Open("sqlite", sourceDBPath)
	if err != nil {
		return fmt.Errorf("failed to open source database: %w", err)
	}
	defer sourceDB.Close()

	// Open target database (tokens.db)
	targetDB, err := sql.Open("sqlite", targetDBPath)
	if err != nil {
		return fmt.Errorf("failed to open target database: %w", err)
	}
	defer targetDB.Close()

	// Start transaction
	tx, err := targetDB.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Step 1: Create new tables in target database
	if err := createRateLimitingTables(tx); err != nil {
		return fmt.Errorf("failed to create rate limiting tables: %w", err)
	}

	// Step 2: Attach source database for cross-database queries
	attachQuery := fmt.Sprintf("ATTACH DATABASE '%s' AS source_db", sourceDBPath)
	if _, err := tx.Exec(attachQuery); err != nil {
		return fmt.Errorf("failed to attach source database: %w", err)
	}

	// Step 3: Migrate token_usage data
	if err := migrateTokenUsage(tx); err != nil {
		return fmt.Errorf("failed to migrate token_usage: %w", err)
	}

	// Step 4: Migrate request_history data
	if err := migrateRequestHistory(tx); err != nil {
		return fmt.Errorf("failed to migrate request_history: %w", err)
	}

	// Step 5: Migrate provider_usage data
	if err := migrateProviderUsage(tx); err != nil {
		return fmt.Errorf("failed to migrate provider_usage: %w", err)
	}

	// Detach source database
	if _, err := tx.Exec("DETACH DATABASE source_db"); err != nil {
		return fmt.Errorf("failed to detach source database: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// createRateLimitingTables creates the new rate limiting tables in the target database
func createRateLimitingTables(tx *sql.Tx) error {
	// Create token_usage table
	tokenUsageTable := `
		CREATE TABLE IF NOT EXISTS token_usage (
			id TEXT PRIMARY KEY,
			token_id TEXT NOT NULL,
			requests_today INTEGER NOT NULL DEFAULT 0,
			requests_in_minute INTEGER NOT NULL DEFAULT 0,
			tokens_in_minute INTEGER NOT NULL DEFAULT 0,
			window_start INTEGER NOT NULL,
			day_start INTEGER NOT NULL,
			created_at INTEGER NOT NULL DEFAULT (strftime('%s', 'subsec') * 1000),
			updated_at INTEGER NOT NULL DEFAULT (strftime('%s', 'subsec') * 1000),
			UNIQUE(token_id),
			FOREIGN KEY (token_id) REFERENCES tokens(id) ON DELETE CASCADE
		)
	`
	if _, err := tx.Exec(tokenUsageTable); err != nil {
		return fmt.Errorf("failed to create token_usage table: %w", err)
	}

	// Create request_history table
	requestHistoryTable := `
		CREATE TABLE IF NOT EXISTS request_history (
			id TEXT PRIMARY KEY,
			token_id TEXT NOT NULL,
			model TEXT,
			request_count INTEGER NOT NULL DEFAULT 1,
			input_tokens INTEGER NOT NULL DEFAULT 0,
			output_tokens INTEGER NOT NULL DEFAULT 0,
			timestamp INTEGER NOT NULL,
			created_at INTEGER NOT NULL DEFAULT (strftime('%s', 'subsec') * 1000),
			FOREIGN KEY (token_id) REFERENCES tokens(id) ON DELETE CASCADE
		)
	`
	if _, err := tx.Exec(requestHistoryTable); err != nil {
		return fmt.Errorf("failed to create request_history table: %w", err)
	}

	// Create provider_usage table (optional, for aggregates)
	providerUsageTable := `
		CREATE TABLE IF NOT EXISTS provider_usage (
			id TEXT PRIMARY KEY,
			provider_id TEXT NOT NULL,
			requests_today INTEGER NOT NULL DEFAULT 0,
			requests_in_minute INTEGER NOT NULL DEFAULT 0,
			tokens_in_minute INTEGER NOT NULL DEFAULT 0,
			window_start INTEGER NOT NULL,
			day_start INTEGER NOT NULL,
			created_at INTEGER NOT NULL DEFAULT (strftime('%s', 'subsec') * 1000),
			updated_at INTEGER NOT NULL DEFAULT (strftime('%s', 'subsec') * 1000),
			UNIQUE(provider_id)
		)
	`
	if _, err := tx.Exec(providerUsageTable); err != nil {
		return fmt.Errorf("failed to create provider_usage table: %w", err)
	}

	// Create indexes
	indexes := []string{
		"CREATE INDEX IF NOT EXISTS idx_token_usage_token ON token_usage(token_id)",
		"CREATE INDEX IF NOT EXISTS idx_request_history_token ON request_history(token_id, timestamp)",
		"CREATE INDEX IF NOT EXISTS idx_request_history_timestamp ON request_history(timestamp)",
		"CREATE INDEX IF NOT EXISTS idx_request_history_model ON request_history(model)",
		"CREATE INDEX IF NOT EXISTS idx_provider_usage_provider ON provider_usage(provider_id)",
	}

	for _, idx := range indexes {
		if _, err := tx.Exec(idx); err != nil {
			return fmt.Errorf("failed to create index: %w", err)
		}
	}

	return nil
}

// migrateTokenUsage migrates token_usage data from source to target database
func migrateTokenUsage(tx *sql.Tx) error {
	// Check if source table exists
	var tableExists int
	err := tx.QueryRow(`
		SELECT COUNT(*) FROM source_db.sqlite_master 
		WHERE type='table' AND name='token_usage'
	`).Scan(&tableExists)
	if err != nil {
		return fmt.Errorf("failed to check if token_usage table exists: %w", err)
	}

	if tableExists == 0 {
		// Source table doesn't exist, skip migration
		return nil
	}

	// Migrate data - note: we need to derive provider_id from tokens table or handle it differently
	// For now, we'll migrate what we can and handle provider_id separately
	query := `
		INSERT INTO token_usage (id, token_id, requests_today, requests_in_minute, 
								tokens_in_minute, window_start, day_start, 
								created_at, updated_at)
		SELECT id, token_id, requests_today, requests_in_minute,
			   tokens_in_minute, window_start, day_start,
			   created_at, updated_at
		FROM source_db.token_usage
	`
	if _, err := tx.Exec(query); err != nil {
		return fmt.Errorf("failed to migrate token_usage data: %w", err)
	}

	return nil
}

// migrateRequestHistory migrates request_history data from source to target database
func migrateRequestHistory(tx *sql.Tx) error {
	// Check if source table exists
	var tableExists int
	err := tx.QueryRow(`
		SELECT COUNT(*) FROM source_db.sqlite_master 
		WHERE type='table' AND name='request_history'
	`).Scan(&tableExists)
	if err != nil {
		return fmt.Errorf("failed to check if request_history table exists: %w", err)
	}

	if tableExists == 0 {
		// Source table doesn't exist, skip migration
		return nil
	}

	// Migrate data - filter out records without token_id since they won't have foreign key
	// For old schema with token_count, migrate to input_tokens (best effort)
	query := `
		INSERT INTO request_history (id, token_id, model, request_count,
								  input_tokens, output_tokens, timestamp, created_at)
		SELECT id, token_id, NULL, request_count,
			   COALESCE(token_count, 0), 0,
			   timestamp, created_at
		FROM source_db.request_history
		WHERE token_id IS NOT NULL AND token_id != ''
	`
	if _, err := tx.Exec(query); err != nil {
		return fmt.Errorf("failed to migrate request_history data: %w", err)
	}

	return nil
}

// migrateProviderUsage migrates provider_usage data from source to target database
func migrateProviderUsage(tx *sql.Tx) error {
	// Check if source table exists
	var tableExists int
	err := tx.QueryRow(`
		SELECT COUNT(*) FROM source_db.sqlite_master 
		WHERE type='table' AND name='provider_usage'
	`).Scan(&tableExists)
	if err != nil {
		return fmt.Errorf("failed to check if provider_usage table exists: %w", err)
	}

	if tableExists == 0 {
		// Source table doesn't exist, skip migration
		return nil
	}

	// Migrate data
	query := `
		INSERT INTO provider_usage (id, provider_id, requests_today, requests_in_minute, 
								tokens_in_minute, window_start, day_start, 
								created_at, updated_at)
		SELECT id, provider_id, requests_today, requests_in_minute,
			   tokens_in_minute, window_start, day_start,
			   created_at, updated_at
		FROM source_db.provider_usage
	`
	if _, err := tx.Exec(query); err != nil {
		return fmt.Errorf("failed to migrate provider_usage data: %w", err)
	}

	return nil
}

// BackupDatabase creates a backup of the specified database
func BackupDatabase(dbPath string) (string, error) {
	backupPath := dbPath + ".backup." + time.Now().Format("20060102-150405")

	sourceDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return "", fmt.Errorf("failed to open source database: %w", err)
	}
	defer sourceDB.Close()

	backupDB, err := sql.Open("sqlite", backupPath)
	if err != nil {
		return "", fmt.Errorf("failed to open backup database: %w", err)
	}
	defer backupDB.Close()

	// Use SQLite's backup API through VACUUM INTO
	_, err = backupDB.Exec(fmt.Sprintf("VACUUM INTO '%s'", dbPath))
	if err != nil {
		return "", fmt.Errorf("failed to create backup: %w", err)
	}

	return backupPath, nil
}
