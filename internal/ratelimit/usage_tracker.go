package ratelimit

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/sunbankio/qwencoder-proxy/internal/logging"
	_ "modernc.org/sqlite"
)

// usageTrackerImpl manages usage tracking and storage with proper sliding window.
type usageTrackerImpl struct {
	db     *sql.DB
	logger logging.Logger
	mu     sync.RWMutex

	// Context for cleanup goroutine.
	ctx    context.Context
	cancel context.CancelFunc

	// In-memory cache for frequently accessed metrics.
	providerCache map[string]*UsageMetrics
	tokenCache    map[string]*UsageMetrics
	cacheExpiry   time.Time

	// ownsDB indicates if this tracker owns the database connection
	// and should close it on shutdown. If false, the connection is
	// shared and managed externally.
	ownsDB bool

	// Retry configuration for database operations
	retryConfig *retryConfig
}

// retryConfig holds retry configuration for database operations.
type retryConfig struct {
	maxRetries int
	baseDelay  time.Duration
	maxDelay   time.Duration
}

// defaultRetryConfig returns default retry configuration.
func defaultRetryConfig() *retryConfig {
	return &retryConfig{
		maxRetries: 10,
		baseDelay:  50 * time.Millisecond,
		maxDelay:   2 * time.Second,
	}
}

// NewUsageTracker creates a new usage tracker with a shared database connection.
func NewUsageTracker(db *sql.DB, logger logging.Logger) (UsageTracker, error) {
	logger.InfoLog("[UsageTracker] Initializing usage tracker with shared database connection")

	// Validate the shared database connection
	if db == nil {
		return nil, fmt.Errorf("database connection is nil")
	}

	// Test database connection
	if err := db.Ping(); err != nil {
		logger.ErrorLog("[UsageTracker] Failed to ping database: %v", err)
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}
	logger.InfoLog("[UsageTracker] Database connection established successfully")

	// Create context for cleanup goroutine.
	ctx, cancel := context.WithCancel(context.Background())

	tracker := &usageTrackerImpl{
		db:            db,
		logger:        logger,
		ctx:           ctx,
		cancel:        cancel,
		providerCache: make(map[string]*UsageMetrics),
		tokenCache:    make(map[string]*UsageMetrics),
		cacheExpiry:   time.Now(),
		ownsDB:        false, // Using shared connection, don't own it
		retryConfig:   defaultRetryConfig(),
	}

	if err := tracker.initializeDB(); err != nil {
		logger.ErrorLog("[UsageTracker] Failed to initialize database schema: %v", err)
		db.Close()
		cancel()
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}
	logger.InfoLog("[UsageTracker] Database schema initialized successfully")

	// Start periodic cleanup.
	go tracker.periodicCleanup()

	return tracker, nil
}

// initializeDB creates the necessary tables and indexes.
func (ut *usageTrackerImpl) initializeDB() error {
	ut.logger.DebugLog("[UsageTracker] Initializing database schema...")

	// Create provider_usage table.
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
	if _, err := ut.db.Exec(providerUsageTable); err != nil {
		return fmt.Errorf("failed to create provider_usage table: %w", err)
	}
	ut.logger.DebugLog("[UsageTracker] Created provider_usage table")

	// Create token_usage table.
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
	if _, err := ut.db.Exec(tokenUsageTable); err != nil {
		return fmt.Errorf("failed to create token_usage table: %w", err)
	}
	ut.logger.DebugLog("[UsageTracker] Created token_usage table")

	// Create request_history table for proper sliding window tracking.
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
	if _, err := ut.db.Exec(requestHistoryTable); err != nil {
		return fmt.Errorf("failed to create request_history table: %w", err)
	}
	ut.logger.DebugLog("[UsageTracker] Created request_history table")

	// Create model_usage table for model-level tracking.
	modelUsageTable := `
		CREATE TABLE IF NOT EXISTS model_usage (
			id TEXT PRIMARY KEY,
			token_id TEXT NOT NULL,
			provider_id TEXT NOT NULL,
			model TEXT NOT NULL,
			request_count INTEGER NOT NULL DEFAULT 0,
			input_tokens INTEGER NOT NULL DEFAULT 0,
			output_tokens INTEGER NOT NULL DEFAULT 0,
			total_tokens INTEGER NOT NULL DEFAULT 0,
			window_start INTEGER NOT NULL,
			day_start INTEGER NOT NULL,
			created_at INTEGER NOT NULL DEFAULT (strftime('%s', 'subsec') * 1000),
			updated_at INTEGER NOT NULL DEFAULT (strftime('%s', 'subsec') * 1000),
			UNIQUE(token_id, model),
			FOREIGN KEY (token_id) REFERENCES tokens(id) ON DELETE CASCADE
		)
	`
	if _, err := ut.db.Exec(modelUsageTable); err != nil {
		return fmt.Errorf("failed to create model_usage table: %w", err)
	}
	ut.logger.DebugLog("[UsageTracker] Created model_usage table")

	// Create indexes for efficient queries.
	indexes := []string{
		"CREATE INDEX IF NOT EXISTS idx_provider_usage_provider ON provider_usage(provider_id)",
		"CREATE INDEX IF NOT EXISTS idx_token_usage_token ON token_usage(token_id)",
		"CREATE INDEX IF NOT EXISTS idx_request_history_token ON request_history(token_id, timestamp)",
		"CREATE INDEX IF NOT EXISTS idx_request_history_timestamp ON request_history(timestamp)",
		"CREATE INDEX IF NOT EXISTS idx_request_history_model ON request_history(model)",
		"CREATE INDEX IF NOT EXISTS idx_model_usage_token ON model_usage(token_id)",
		"CREATE INDEX IF NOT EXISTS idx_model_usage_provider ON model_usage(provider_id)",
		"CREATE INDEX IF NOT EXISTS idx_model_usage_model ON model_usage(model)",
		"CREATE INDEX IF NOT EXISTS idx_model_usage_provider_model ON model_usage(provider_id, model)",
	}

	for _, idx := range indexes {
		if _, err := ut.db.Exec(idx); err != nil {
			return fmt.Errorf("failed to create index: %w", err)
		}
	}
	ut.logger.DebugLog("[UsageTracker] Created all indexes")

	// Migrate existing databases to add UNIQUE constraint if missing
	if err := ut.migrateTokenUsageUniqueConstraint(); err != nil {
		ut.logger.WarnLog("[UsageTracker] Failed to migrate token_usage UNIQUE constraint: %v", err)
		// Don't fail initialization, just log the error
	}

	// Migrate to schema v2 for model tracking
	if err := ut.migrateToSchemaV2(); err != nil {
		ut.logger.WarnLog("[UsageTracker] Failed to migrate to schema v2: %v", err)
		// Don't fail initialization, just log the error
	}

	// Migrate request_history schema to add model, input_tokens, output_tokens
	if err := ut.migrateRequestHistorySchema(); err != nil {
		ut.logger.WarnLog("[UsageTracker] Failed to migrate request_history schema: %v", err)
		// Don't fail initialization, just log the error
	}

	return nil
}

// migrateTokenUsageUniqueConstraint adds UNIQUE constraint to token_usage table if it's missing
// This is needed for databases created before the UNIQUE constraint was added to the schema
func (ut *usageTrackerImpl) migrateTokenUsageUniqueConstraint() error {
	// Check if the UNIQUE constraint exists by trying to query sqlite_master
	var constraintExists int
	err := ut.db.QueryRow(`
		SELECT COUNT(*)
		FROM sqlite_master
		WHERE type='table' AND name='token_usage' AND sql LIKE '%UNIQUE(token_id)%'
	`).Scan(&constraintExists)
	if err != nil {
		return fmt.Errorf("failed to check UNIQUE constraint: %w", err)
	}

	if constraintExists > 0 {
		ut.logger.DebugLog("[UsageTracker] UNIQUE constraint already exists on token_usage.token_id")
		return nil
	}

	ut.logger.InfoLog("[UsageTracker] Migrating token_usage table to add UNIQUE constraint...")

	// Start transaction for migration
	tx, err := ut.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin migration transaction: %w", err)
	}
	defer tx.Rollback()

	// Create new table with UNIQUE constraint
	createTableSQL := `
		CREATE TABLE token_usage_new (
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
	if _, err := tx.Exec(createTableSQL); err != nil {
		return fmt.Errorf("failed to create new token_usage table: %w", err)
	}

	// Copy data from old table to new table (handle duplicates by keeping the most recent)
	copyDataSQL := `
		INSERT INTO token_usage_new (id, token_id, requests_today, requests_in_minute, tokens_in_minute, window_start, day_start, created_at, updated_at)
		SELECT
			MIN(id) as id,
			token_id,
			MAX(requests_today) as requests_today,
			MAX(requests_in_minute) as requests_in_minute,
			MAX(tokens_in_minute) as tokens_in_minute,
			MAX(window_start) as window_start,
			MAX(day_start) as day_start,
			MAX(created_at) as created_at,
			MAX(updated_at) as updated_at
		FROM token_usage
		GROUP BY token_id
	`
	result, err := tx.Exec(copyDataSQL)
	if err != nil {
		return fmt.Errorf("failed to copy data to new table: %w", err)
	}
	rowsCopied, _ := result.RowsAffected()
	ut.logger.InfoLog("[UsageTracker] Copied %d rows to new token_usage table", rowsCopied)

	// Drop old table
	if _, err := tx.Exec("DROP TABLE token_usage"); err != nil {
		return fmt.Errorf("failed to drop old token_usage table: %w", err)
	}

	// Rename new table to original name
	if _, err := tx.Exec("ALTER TABLE token_usage_new RENAME TO token_usage"); err != nil {
		return fmt.Errorf("failed to rename new token_usage table: %w", err)
	}

	// Recreate indexes
	indexes := []string{
		"CREATE INDEX IF NOT EXISTS idx_token_usage_token ON token_usage(token_id)",
	}
	for _, idx := range indexes {
		if _, err := tx.Exec(idx); err != nil {
			return fmt.Errorf("failed to recreate index: %w", err)
		}
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit migration: %w", err)
	}

	ut.logger.InfoLog("[UsageTracker] Successfully migrated token_usage table with UNIQUE constraint")
	return nil
}

// RecordUsage records usage for a provider and optionally a token.
// Uses atomic SQL operations to prevent race conditions.
func (ut *usageTrackerImpl) RecordUsage(ctx context.Context, providerID string, tokenID string, requestCount int, tokenCount int) error {
	now := time.Now()
	nowMs := now.UnixMilli()

	ut.logger.DebugLog("[UsageTracker] Recording usage - Provider: %s, Token: %s, Requests: %d, Tokens: %d",
		providerID, tokenID, requestCount, tokenCount)

	// Use retry logic for the entire transaction
	return ut.executeWithRetry("RecordUsage", func() error {
		// Use transaction for atomicity.
		tx, err := ut.db.BeginTx(ctx, nil)
		if err != nil {
			ut.logger.ErrorLog("[UsageTracker] Failed to begin transaction: %v", err)
			return fmt.Errorf("failed to begin transaction: %w", err)
		}
		defer tx.Rollback()

		// Check if token exists before recording usage (both request_history and token_usage have FK constraints)
		var tokenExists bool
		if tokenID != "" {
			var exists int
			err := tx.QueryRowContext(ctx, "SELECT 1 FROM tokens WHERE id = ?", tokenID).Scan(&exists)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					ut.logger.WarnLog("[UsageTracker] Token %s does not exist in tokens table, skipping usage tracking", tokenID)
					// Token doesn't exist, skip usage tracking but continue with provider usage
					tokenExists = false
				} else {
					ut.logger.ErrorLog("[UsageTracker] Failed to check token existence: %v", err)
					return fmt.Errorf("failed to check token existence: %w", err)
				}
			} else {
				tokenExists = true
			}
		}

		// Record in request_history for sliding window tracking (only if token exists).
		if tokenID != "" && tokenExists {
			historyID := uuid.New().String()
			historyQuery := `
				INSERT INTO request_history (id, token_id, model, request_count, input_tokens, output_tokens, timestamp)
				VALUES (?, ?, ?, ?, ?, ?, ?)
			`
			// For RecordUsage, we don't have model info, so use NULL
			// Put all tokens in input_tokens since we don't have the breakdown
			if _, err := tx.ExecContext(ctx, historyQuery, historyID, tokenID, nil, requestCount, tokenCount, 0, nowMs); err != nil {
				ut.logger.ErrorLog("[UsageTracker] Failed to insert request history: %v", err)
				ut.logger.ErrorLog("[UsageTracker] Query: %s, Params: %s, %s, %v, %d, %d, %d, %d",
					historyQuery, historyID, tokenID, nil, requestCount, tokenCount, 0, nowMs)
				return fmt.Errorf("failed to insert request history: %w", err)
			}
			ut.logger.DebugLog("[UsageTracker] Inserted request_history record: %s", historyID)
		}

		// Update provider usage with atomic increment.
		providerQuery := `
			INSERT INTO provider_usage (id, provider_id, requests_today, requests_in_minute, tokens_in_minute, window_start, day_start)
			VALUES (?, ?, 0, 0, 0, ?, ?)
			ON CONFLICT(provider_id) DO UPDATE SET
				requests_today = requests_today + ?,
				requests_in_minute = requests_in_minute + ?,
				tokens_in_minute = tokens_in_minute + ?,
				updated_at = ?
			WHERE provider_id = ?
		`
		dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		windowStart := now.Add(-time.Minute)

		if _, err := tx.ExecContext(ctx, providerQuery,
			uuid.New().String(), providerID, windowStart.UnixMilli(), dayStart.UnixMilli(),
			requestCount, requestCount, tokenCount, nowMs, providerID); err != nil {
			ut.logger.ErrorLog("[UsageTracker] Failed to update provider usage: %v", err)
			return fmt.Errorf("failed to update provider usage: %w", err)
		}
		ut.logger.DebugLog("[UsageTracker] Updated provider_usage for: %s", providerID)

		// Update token usage if token ID provided and token exists.
		if tokenID != "" && tokenExists {
			tokenQuery := `
				INSERT INTO token_usage (id, token_id, requests_today, requests_in_minute, tokens_in_minute, window_start, day_start)
				VALUES (?, ?, 0, 0, 0, ?, ?)
				ON CONFLICT(token_id) DO UPDATE SET
					requests_today = requests_today + ?,
					requests_in_minute = requests_in_minute + ?,
					tokens_in_minute = tokens_in_minute + ?,
					updated_at = ?
				WHERE token_id = ?
			`
			if _, err := tx.ExecContext(ctx, tokenQuery,
				uuid.New().String(), tokenID, windowStart.UnixMilli(), dayStart.UnixMilli(),
				requestCount, requestCount, tokenCount, nowMs, tokenID); err != nil {
				ut.logger.ErrorLog("[UsageTracker] Failed to update token usage: %v", err)
				return fmt.Errorf("failed to update token usage: %w", err)
			}
			ut.logger.DebugLog("[UsageTracker] Updated token_usage for: %s", tokenID)
		}

		// Commit transaction.
		if err := tx.Commit(); err != nil {
			ut.logger.ErrorLog("[UsageTracker] Failed to commit transaction: %v", err)
			return fmt.Errorf("failed to commit transaction: %w", err)
		}

		ut.logger.InfoLog("[UsageTracker] Successfully recorded usage - Provider: %s, Token: %s, Requests: %d, Tokens: %d",
			providerID, tokenID, requestCount, tokenCount)

		// Invalidate cache.
		ut.mu.Lock()
		ut.cacheExpiry = time.Time{}
		ut.mu.Unlock()

		return nil
	})
}

// GetProviderUsage retrieves current usage metrics for a provider.
// Uses proper sliding window calculation.
func (ut *usageTrackerImpl) GetProviderUsage(ctx context.Context, providerID string) (*UsageMetrics, error) {
	now := time.Now()
	windowStart := now.Add(-time.Minute)
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	windowStartMs := windowStart.UnixMilli()
	dayStartMs := dayStart.UnixMilli()

	// Count requests in sliding window (last 60 seconds).
	minuteQuery := `
		SELECT COALESCE(SUM(request_count), 0), COALESCE(SUM(input_tokens + output_tokens), 0)
		FROM request_history
		WHERE timestamp >= ?
	`
	var requestsInMinute, tokensInMinute int
	err := ut.db.QueryRowContext(ctx, minuteQuery, windowStartMs).Scan(&requestsInMinute, &tokensInMinute)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("failed to query minute usage: %w", err)
	}

	// Count requests in rolling 24-hour window.
	dayQuery := `
		SELECT COALESCE(SUM(request_count), 0)
		FROM request_history
		WHERE timestamp >= ?
	`
	var requestsToday int
	err = ut.db.QueryRowContext(ctx, dayQuery, dayStartMs).Scan(&requestsToday)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("failed to query day usage: %w", err)
	}

	return &UsageMetrics{
		RequestsToday:    requestsToday,
		RequestsInMinute: requestsInMinute,
		TokensInMinute:   tokensInMinute,
		WindowStart:      windowStart,
		DayStart:         dayStart,
	}, nil
}

// GetTokenUsage retrieves current usage metrics for a token.
// Uses proper sliding window calculation.
func (ut *usageTrackerImpl) GetTokenUsage(ctx context.Context, providerID string, tokenID string) (*UsageMetrics, error) {
	now := time.Now()
	windowStart := now.Add(-time.Minute)
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	windowStartMs := windowStart.UnixMilli()
	dayStartMs := dayStart.UnixMilli()

	// Count requests in sliding window (last 60 seconds).
	minuteQuery := `
		SELECT COALESCE(SUM(request_count), 0), COALESCE(SUM(input_tokens + output_tokens), 0)
		FROM request_history
		WHERE token_id = ? AND timestamp >= ?
	`
	var requestsInMinute, tokensInMinute int
	err := ut.db.QueryRowContext(ctx, minuteQuery, tokenID, windowStartMs).Scan(&requestsInMinute, &tokensInMinute)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("failed to query minute usage: %w", err)
	}

	// Count requests in rolling 24-hour window.
	dayQuery := `
		SELECT COALESCE(SUM(request_count), 0)
		FROM request_history
		WHERE token_id = ? AND timestamp >= ?
	`
	var requestsToday int
	err = ut.db.QueryRowContext(ctx, dayQuery, tokenID, dayStartMs).Scan(&requestsToday)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("failed to query day usage: %w", err)
	}

	return &UsageMetrics{
		RequestsToday:    requestsToday,
		RequestsInMinute: requestsInMinute,
		TokensInMinute:   tokensInMinute,
		WindowStart:      windowStart,
		DayStart:         dayStart,
	}, nil
}

// GetAllProviderUsage retrieves usage metrics for all providers.
func (ut *usageTrackerImpl) GetAllProviderUsage(ctx context.Context) (map[string]*UsageMetrics, error) {
	// Get all unique provider IDs from provider_usage table.
	providersQuery := `
		SELECT DISTINCT provider_id FROM provider_usage
	`
	rows, err := ut.db.QueryContext(ctx, providersQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to query providers: %w", err)
	}
	defer rows.Close()

	result := make(map[string]*UsageMetrics)

	for rows.Next() {
		var providerID string
		if err := rows.Scan(&providerID); err != nil {
			return nil, fmt.Errorf("failed to scan provider: %w", err)
		}

		metrics, err := ut.GetProviderUsage(ctx, providerID)
		if err != nil {
			ut.logger.WarnLog("[UsageTracker] Failed to get usage for %s: %v", providerID, err)
			continue
		}

		result[providerID] = metrics
	}

	return result, nil
}

// ResetUsage resets usage metrics for a provider or token.
func (ut *usageTrackerImpl) ResetUsage(ctx context.Context, providerID string, tokenID string) error {
	// Use retry logic for the entire transaction
	return ut.executeWithRetry("ResetUsage", func() error {
		tx, err := ut.db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("failed to begin transaction: %w", err)
		}
		defer tx.Rollback()

		// Delete request history.
		if tokenID != "" {
			_, err = tx.ExecContext(ctx, "DELETE FROM request_history WHERE token_id = ?", tokenID)
		} else {
			// For provider-level reset, we need to delete all request history
			// Since we removed provider_id from request_history, we need to join with tokens table
			// For now, we'll just reset provider_usage table
		}

		if err != nil {
			return fmt.Errorf("failed to delete request history: %w", err)
		}

		// Reset usage table.
		if tokenID != "" {
			_, err = tx.ExecContext(ctx, `
				UPDATE token_usage
				SET requests_today = 0, requests_in_minute = 0, tokens_in_minute = 0, updated_at = ?
				WHERE token_id = ?
			`, time.Now().UnixMilli(), tokenID)
		} else {
			_, err = tx.ExecContext(ctx, `
				UPDATE provider_usage
				SET requests_today = 0, requests_in_minute = 0, tokens_in_minute = 0, updated_at = ?
				WHERE provider_id = ?
			`, time.Now().UnixMilli(), providerID)
		}
		if err != nil {
			return fmt.Errorf("failed to reset usage: %w", err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("failed to commit transaction: %w", err)
		}

		// Invalidate cache.
		ut.mu.Lock()
		ut.cacheExpiry = time.Time{}
		ut.mu.Unlock()

		return nil
	})
}

// periodicCleanup removes old usage records periodically.
// Uses context for proper cancellation.
func (ut *usageTrackerImpl) periodicCleanup() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ut.ctx.Done():
			ut.logger.InfoLog("[UsageTracker] Cleanup goroutine stopped")
			return
		case <-ticker.C:
			ctx := context.Background()
			cutoff := time.Now().Add(-7 * 24 * time.Hour) // Keep 7 days of history.
			cutoffMs := cutoff.UnixMilli()

			// Delete old request history records.
			result, err := ut.db.ExecContext(ctx, "DELETE FROM request_history WHERE timestamp < ?", cutoffMs)
			if err != nil {
				ut.logger.ErrorLog("[UsageTracker] Failed to cleanup old records: %v", err)
			} else if rowsAffected, _ := result.RowsAffected(); rowsAffected > 0 {
				ut.logger.InfoLog("[UsageTracker] Cleaned up %d old request history records", rowsAffected)
			}
		}
	}
}

// CleanupOrphanedUsageRecords removes usage records for tokens that no longer exist.
// This should be called periodically or on startup to clean up orphaned records.
func (ut *usageTrackerImpl) CleanupOrphanedUsageRecords(ctx context.Context) error {
	ut.logger.InfoLog("[UsageTracker] Cleaning up orphaned usage records...")

	// Delete token_usage records where the token doesn't exist in tokens table
	result, err := ut.db.ExecContext(ctx, `
		DELETE FROM token_usage
		WHERE token_id NOT IN (SELECT id FROM tokens)
	`)
	if err != nil {
		ut.logger.ErrorLog("[UsageTracker] Failed to cleanup orphaned usage records: %v", err)
		return fmt.Errorf("failed to cleanup orphaned usage records: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected > 0 {
		ut.logger.InfoLog("[UsageTracker] Cleaned up %d orphaned usage records", rowsAffected)
	} else {
		ut.logger.DebugLog("[UsageTracker] No orphaned usage records found")
	}

	// Also cleanup request_history
	result, err = ut.db.ExecContext(ctx, `
		DELETE FROM request_history
		WHERE token_id NOT IN (SELECT id FROM tokens)
	`)
	if err != nil {
		ut.logger.ErrorLog("[UsageTracker] Failed to cleanup orphaned request history: %v", err)
		return fmt.Errorf("failed to cleanup orphaned request history: %w", err)
	}

	rowsAffected, _ = result.RowsAffected()
	if rowsAffected > 0 {
		ut.logger.InfoLog("[UsageTracker] Cleaned up %d orphaned request history records", rowsAffected)
	}

	return nil
}

// GetDB returns the underlying database connection.
func (ut *usageTrackerImpl) GetDB() *sql.DB {
	return ut.db
}

// Close stops the cleanup goroutine and closes the database connection if owned.
func (ut *usageTrackerImpl) Close() error {
	// Stop the cleanup goroutine.
	ut.cancel()

	// Close the database connection only if we own it.
	if ut.ownsDB {
		return ut.db.Close()
	}
	return nil
}

// isSQLiteBusy checks if an error is a SQLITE_BUSY error.
func isSQLiteBusy(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "database is locked") ||
		strings.Contains(errStr, "SQLITE_BUSY") ||
		strings.Contains(errStr, "(5)")
}

// executeWithRetry executes a function with exponential backoff retry.
func (ut *usageTrackerImpl) executeWithRetry(
	operation string,
	fn func() error,
) error {
	var lastErr error

	for attempt := 0; attempt < ut.retryConfig.maxRetries; attempt++ {
		if attempt > 0 {
			// Calculate exponential backoff delay
			delay := ut.retryConfig.baseDelay * time.Duration(1<<uint(attempt-1))
			if delay > ut.retryConfig.maxDelay {
				delay = ut.retryConfig.maxDelay
			}

			ut.logger.DebugLog("[UsageTracker] %s: retry attempt %d/%d after %v",
				operation, attempt+1, ut.retryConfig.maxRetries, delay)
			time.Sleep(delay)
		}

		err := fn()
		if err == nil {
			if attempt > 0 {
				ut.logger.DebugLog("[UsageTracker] %s: succeeded on attempt %d", operation, attempt+1)
			}
			return nil
		}

		lastErr = err

		// Check if this is a retryable error
		if isSQLiteBusy(err) && attempt < ut.retryConfig.maxRetries-1 {
			ut.logger.DebugLog("[UsageTracker] %s: database busy, will retry", operation)
			continue
		}

		// Non-retryable error or max retries reached
		break
	}

	return fmt.Errorf("%s failed after %d attempts: %w", operation, ut.retryConfig.maxRetries, lastErr)
}

// migrateToSchemaV2 migrates the database to schema v2 for model tracking.
// This adds model, input_tokens, and output_tokens columns to request_history.
func (ut *usageTrackerImpl) migrateToSchemaV2() error {
	// Check if model column already exists in request_history
	var modelColumnExists int
	err := ut.db.QueryRow(`
		SELECT COUNT(*)
		FROM pragma_table_info('request_history')
		WHERE name = 'model'
	`).Scan(&modelColumnExists)
	if err != nil {
		return fmt.Errorf("failed to check model column: %w", err)
	}

	if modelColumnExists > 0 {
		ut.logger.DebugLog("[UsageTracker] Schema v2 migration already applied")
		return nil
	}

	ut.logger.InfoLog("[UsageTracker] Migrating to schema v2 for model tracking...")

	tx, err := ut.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin migration transaction: %w", err)
	}
	defer tx.Rollback()

	// Add model column to request_history
	if _, err := tx.Exec(`
		ALTER TABLE request_history
		ADD COLUMN model TEXT NOT NULL DEFAULT ''
	`); err != nil {
		return fmt.Errorf("failed to add model column: %w", err)
	}

	// Add input_tokens column to request_history
	if _, err := tx.Exec(`
		ALTER TABLE request_history
		ADD COLUMN input_tokens INTEGER NOT NULL DEFAULT 0
	`); err != nil {
		return fmt.Errorf("failed to add input_tokens column: %w", err)
	}

	// Add output_tokens column to request_history
	if _, err := tx.Exec(`
		ALTER TABLE request_history
		ADD COLUMN output_tokens INTEGER NOT NULL DEFAULT 0
	`); err != nil {
		return fmt.Errorf("failed to add output_tokens column: %w", err)
	}

	// Create index for model queries
	if _, err := tx.Exec(`
		CREATE INDEX IF NOT EXISTS idx_request_history_model
		ON request_history(token_id, model, timestamp)
	`); err != nil {
		return fmt.Errorf("failed to create model index: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit migration: %w", err)
	}

	ut.logger.InfoLog("[UsageTracker] Successfully migrated to schema v2")
	return nil
}

// migrateRequestHistorySchema updates the request_history table schema
// to include model, input_tokens, and output_tokens columns
func (ut *usageTrackerImpl) migrateRequestHistorySchema() error {
	ut.logger.InfoLog("[UsageTracker] Starting request_history schema migration...")

	// Step 1: Check if migration is needed
	var hasModelColumn int
	err := ut.db.QueryRow(`
		SELECT COUNT(*)
		FROM pragma_table_info('request_history')
		WHERE name = 'model'
	`).Scan(&hasModelColumn)
	if err != nil {
		return fmt.Errorf("failed to check model column: %w", err)
	}

	if hasModelColumn > 0 {
		ut.logger.InfoLog("[UsageTracker] request_history schema already migrated")
		return nil
	}

	// Step 2: Begin transaction
	tx, err := ut.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin migration transaction: %w", err)
	}
	defer tx.Rollback()

	// Step 3: Add new columns
	columnsToAdd := []string{
		"model TEXT",
		"input_tokens INTEGER NOT NULL DEFAULT 0",
		"output_tokens INTEGER NOT NULL DEFAULT 0",
	}

	for _, col := range columnsToAdd {
		_, err := tx.Exec(fmt.Sprintf("ALTER TABLE request_history ADD COLUMN %s", col))
		if err != nil {
			return fmt.Errorf("failed to add column %s: %w", col, err)
		}
		ut.logger.InfoLog("[UsageTracker] Added column: %s", col)
	}

	// Step 4: Migrate existing token_count data to input_tokens/output_tokens
	// Since we don't have the breakdown, we'll put everything in input_tokens
	// This is a best-effort migration
	_, err = tx.Exec(`
		UPDATE request_history
		SET input_tokens = COALESCE(token_count, 0),
		    output_tokens = 0
		WHERE input_tokens = 0
	`)
	if err != nil {
		return fmt.Errorf("failed to migrate token_count data: %w", err)
	}
	ut.logger.InfoLog("[UsageTracker] Migrated existing token_count data to input_tokens")

	// Step 5: Update indexes
	// Add index on model for faster queries
	_, err = tx.Exec(`
		CREATE INDEX IF NOT EXISTS idx_request_history_model
		ON request_history(model)
	`)
	if err != nil {
		return fmt.Errorf("failed to create model index: %w", err)
	}
	ut.logger.InfoLog("[UsageTracker] Created index on model column")

	// Step 6: Commit transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit migration: %w", err)
	}

	ut.logger.InfoLog("[UsageTracker] Successfully migrated request_history schema")
	return nil
}

// RecordModelUsage records usage for a specific model.
func (ut *usageTrackerImpl) RecordModelUsage(
	ctx context.Context,
	providerID string,
	tokenID string,
	model string,
	inputTokens int,
	outputTokens int,
) error {
	now := time.Now()
	nowMs := now.UnixMilli()
	totalTokens := inputTokens + outputTokens

	ut.logger.DebugLog("[UsageTracker] Recording model usage - Provider: %s, Token: %s, Model: %s, Input: %d, Output: %d",
		providerID, tokenID, model, inputTokens, outputTokens)

	// Use retry logic for the entire transaction
	return ut.executeWithRetry("RecordModelUsage", func() error {
		// Use transaction for atomicity.
		tx, err := ut.db.BeginTx(ctx, nil)
		if err != nil {
			ut.logger.ErrorLog("[UsageTracker] Failed to begin transaction: %v", err)
			return fmt.Errorf("failed to begin transaction: %w", err)
		}
		defer tx.Rollback()

		// Check if token exists before recording usage
		var tokenExists bool
		if tokenID != "" {
			var exists int
			err := tx.QueryRowContext(ctx, "SELECT 1 FROM tokens WHERE id = ?", tokenID).Scan(&exists)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					ut.logger.WarnLog("[UsageTracker] Token %s does not exist in tokens table, skipping model usage tracking", tokenID)
					tokenExists = false
				} else {
					ut.logger.ErrorLog("[UsageTracker] Failed to check token existence: %v", err)
					return fmt.Errorf("failed to check token existence: %w", err)
				}
			} else {
				tokenExists = true
			}
		}

		// Record in request_history with model details (only if token exists).
		if tokenID != "" && tokenExists {
			historyID := uuid.New().String()
			historyQuery := `
				INSERT INTO request_history (id, token_id, model, request_count,
				                         input_tokens, output_tokens, timestamp)
				VALUES (?, ?, ?, 1, ?, ?, ?)
			`
			if _, err := tx.ExecContext(ctx, historyQuery,
				historyID, tokenID, model, inputTokens, outputTokens, nowMs); err != nil {
				ut.logger.ErrorLog("[UsageTracker] Failed to insert request history: %v", err)
				return fmt.Errorf("failed to insert request history: %w", err)
			}
			ut.logger.DebugLog("[UsageTracker] Inserted request_history record: %s", historyID)
		}

		// Update provider_usage with atomic increment (only if token exists).
		if tokenID != "" && tokenExists {
			providerQuery := `
				INSERT INTO provider_usage (id, provider_id, requests_today, requests_in_minute, tokens_in_minute, window_start, day_start)
				VALUES (?, ?, 0, 0, 0, ?, ?)
				ON CONFLICT(provider_id) DO UPDATE SET
					requests_today = requests_today + ?,
					requests_in_minute = requests_in_minute + ?,
					tokens_in_minute = tokens_in_minute + ?,
					updated_at = ?
				WHERE provider_id = ?
			`
			dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
			windowStart := now.Add(-time.Minute)

			if _, err := tx.ExecContext(ctx, providerQuery,
				uuid.New().String(), providerID, windowStart.UnixMilli(), dayStart.UnixMilli(),
				1, 1, totalTokens, nowMs, providerID); err != nil {
				ut.logger.ErrorLog("[UsageTracker] Failed to update provider usage: %v", err)
				return fmt.Errorf("failed to update provider usage: %w", err)
			}
			ut.logger.DebugLog("[UsageTracker] Updated provider_usage for: %s", providerID)
		}

		// Update token_usage with atomic increment (only if token exists).
		if tokenID != "" && tokenExists {
			tokenQuery := `
				INSERT INTO token_usage (id, token_id, requests_today, requests_in_minute, tokens_in_minute, window_start, day_start)
				VALUES (?, ?, 0, 0, 0, ?, ?)
				ON CONFLICT(token_id) DO UPDATE SET
					requests_today = requests_today + ?,
					requests_in_minute = requests_in_minute + ?,
					tokens_in_minute = tokens_in_minute + ?,
					updated_at = ?
				WHERE token_id = ?
			`
			dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
			windowStart := now.Add(-time.Minute)

			if _, err := tx.ExecContext(ctx, tokenQuery,
				uuid.New().String(), tokenID, windowStart.UnixMilli(), dayStart.UnixMilli(),
				1, 1, totalTokens, nowMs, tokenID); err != nil {
				ut.logger.ErrorLog("[UsageTracker] Failed to update token usage: %v", err)
				return fmt.Errorf("failed to update token usage: %w", err)
			}
			ut.logger.DebugLog("[UsageTracker] Updated token_usage for: %s", tokenID)
		}

		// Update model_usage with atomic increment (only if token exists).
		if tokenID != "" && tokenExists {
			dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
			windowStart := now.Add(-time.Minute)

			modelQuery := `
				INSERT INTO model_usage (id, token_id, provider_id, model,
				                       request_count, input_tokens, output_tokens, total_tokens,
				                       window_start, day_start)
				VALUES (?, ?, ?, ?, 0, 0, 0, 0, ?, ?)
				ON CONFLICT(token_id, model) DO UPDATE SET
					request_count = request_count + 1,
					input_tokens = input_tokens + ?,
					output_tokens = output_tokens + ?,
					total_tokens = total_tokens + ?,
					updated_at = ?
				WHERE token_id = ? AND model = ?
			`
			if _, err := tx.ExecContext(ctx, modelQuery,
				uuid.New().String(), tokenID, providerID, model,
				windowStart.UnixMilli(), dayStart.UnixMilli(),
				inputTokens, outputTokens, totalTokens, nowMs,
				tokenID, model); err != nil {
				ut.logger.ErrorLog("[UsageTracker] Failed to update model usage: %v", err)
				return fmt.Errorf("failed to update model usage: %w", err)
			}
			ut.logger.DebugLog("[UsageTracker] Updated model_usage for: %s / %s", tokenID, model)
		}

		// Commit transaction.
		if err := tx.Commit(); err != nil {
			ut.logger.ErrorLog("[UsageTracker] Failed to commit transaction: %v", err)
			return fmt.Errorf("failed to commit transaction: %w", err)
		}

		ut.logger.InfoLog("[UsageTracker] Successfully recorded model usage - Provider: %s, Token: %s, Model: %s, Total: %d",
			providerID, tokenID, model, totalTokens)

		return nil
	})
}

// GetModelUsage retrieves usage metrics for a specific model.
func (ut *usageTrackerImpl) GetModelUsage(
	ctx context.Context,
	providerID string,
	tokenID string,
	model string,
) (*ModelUsageMetrics, error) {
	now := time.Now()
	windowStart := now.Add(-time.Minute)
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	// Query from model_usage table
	query := `
		SELECT provider_id, token_id, model, request_count,
		       input_tokens, output_tokens, total_tokens,
		       window_start, day_start
		FROM model_usage
		WHERE token_id = ? AND model = ?
	`

	var metrics ModelUsageMetrics
	var windowStartMs, dayStartMs int64
	err := ut.db.QueryRowContext(ctx, query, tokenID, model).Scan(
		&metrics.ProviderID, &metrics.TokenID, &metrics.Model,
		&metrics.RequestCount, &metrics.InputTokens,
		&metrics.OutputTokens, &metrics.TotalTokens,
		&windowStartMs, &dayStartMs,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// No usage yet, return zero metrics
			return &ModelUsageMetrics{
				ProviderID:   providerID,
				TokenID:      tokenID,
				Model:        model,
				RequestCount: 0,
				InputTokens:  0,
				OutputTokens: 0,
				TotalTokens:  0,
				WindowStart:  windowStart,
				DayStart:     dayStart,
			}, nil
		}
		return nil, fmt.Errorf("failed to query model usage: %w", err)
	}

	metrics.WindowStart = time.UnixMilli(windowStartMs)
	metrics.DayStart = time.UnixMilli(dayStartMs)

	return &metrics, nil
}

// GetAllModelUsage retrieves all model usage for a provider.
func (ut *usageTrackerImpl) GetAllModelUsage(
	ctx context.Context,
	providerID string,
) (map[string]*ModelUsageMetrics, error) {
	query := `
		SELECT provider_id, token_id, model, request_count,
		       input_tokens, output_tokens, total_tokens,
		       window_start, day_start
		FROM model_usage
		WHERE provider_id = ?
	`

	rows, err := ut.db.QueryContext(ctx, query, providerID)
	if err != nil {
		return nil, fmt.Errorf("failed to query all model usage: %w", err)
	}
	defer rows.Close()

	result := make(map[string]*ModelUsageMetrics)
	for rows.Next() {
		var metrics ModelUsageMetrics
		var windowStartMs, dayStartMs int64
		if err := rows.Scan(
			&metrics.ProviderID, &metrics.TokenID, &metrics.Model,
			&metrics.RequestCount, &metrics.InputTokens,
			&metrics.OutputTokens, &metrics.TotalTokens,
			&windowStartMs, &dayStartMs,
		); err != nil {
			return nil, fmt.Errorf("failed to scan model usage row: %w", err)
		}
		metrics.WindowStart = time.UnixMilli(windowStartMs)
		metrics.DayStart = time.UnixMilli(dayStartMs)
		// Use composite key for uniqueness
		key := metrics.TokenID + ":" + metrics.Model
		result[key] = &metrics
	}

	return result, nil
}

// GetTokenModelUsage retrieves all model usage for a token.
func (ut *usageTrackerImpl) GetTokenModelUsage(
	ctx context.Context,
	tokenID string,
) (map[string]*ModelUsageMetrics, error) {
	query := `
		SELECT provider_id, token_id, model, request_count,
		       input_tokens, output_tokens, total_tokens,
		       window_start, day_start
		FROM model_usage
		WHERE token_id = ?
	`

	rows, err := ut.db.QueryContext(ctx, query, tokenID)
	if err != nil {
		return nil, fmt.Errorf("failed to query token model usage: %w", err)
	}
	defer rows.Close()

	result := make(map[string]*ModelUsageMetrics)
	for rows.Next() {
		var metrics ModelUsageMetrics
		var windowStartMs, dayStartMs int64
		if err := rows.Scan(
			&metrics.ProviderID, &metrics.TokenID, &metrics.Model,
			&metrics.RequestCount, &metrics.InputTokens,
			&metrics.OutputTokens, &metrics.TotalTokens,
			&windowStartMs, &dayStartMs,
		); err != nil {
			return nil, fmt.Errorf("failed to scan model usage row: %w", err)
		}
		metrics.WindowStart = time.UnixMilli(windowStartMs)
		metrics.DayStart = time.UnixMilli(dayStartMs)
		result[metrics.Model] = &metrics
	}

	return result, nil
}

// GetProviderModelUsage retrieves all model usage for a provider.
func (ut *usageTrackerImpl) GetProviderModelUsage(
	ctx context.Context,
	providerID string,
) (map[string]*ModelUsageMetrics, error) {
	query := `
		SELECT provider_id, token_id, model, request_count,
		       input_tokens, output_tokens, total_tokens,
		       window_start, day_start
		FROM model_usage
		WHERE provider_id = ?
	`

	rows, err := ut.db.QueryContext(ctx, query, providerID)
	if err != nil {
		return nil, fmt.Errorf("failed to query provider model usage: %w", err)
	}
	defer rows.Close()

	result := make(map[string]*ModelUsageMetrics)
	for rows.Next() {
		var metrics ModelUsageMetrics
		var windowStartMs, dayStartMs int64
		if err := rows.Scan(
			&metrics.ProviderID, &metrics.TokenID, &metrics.Model,
			&metrics.RequestCount, &metrics.InputTokens,
			&metrics.OutputTokens, &metrics.TotalTokens,
			&windowStartMs, &dayStartMs,
		); err != nil {
			return nil, fmt.Errorf("failed to scan model usage row: %w", err)
		}
		metrics.WindowStart = time.UnixMilli(windowStartMs)
		metrics.DayStart = time.UnixMilli(dayStartMs)
		// Use model as key for provider-level aggregation
		result[metrics.Model] = &metrics
	}

	return result, nil
}

// ResetModelUsage resets usage metrics for a specific model.
func (ut *usageTrackerImpl) ResetModelUsage(
	ctx context.Context,
	providerID string,
	tokenID string,
	model string,
) error {
	tx, err := ut.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Delete request history for this model
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM request_history
		WHERE token_id = ? AND model = ?
	`, tokenID, model); err != nil {
		return fmt.Errorf("failed to delete request history: %w", err)
	}

	// Reset model_usage table
	if _, err := tx.ExecContext(ctx, `
		UPDATE model_usage
		SET request_count = 0, input_tokens = 0, output_tokens = 0,
		    total_tokens = 0, updated_at = ?
		WHERE token_id = ? AND model = ?
	`, time.Now().UnixMilli(), tokenID, model); err != nil {
		return fmt.Errorf("failed to reset model usage: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// GetRequestHistory retrieves request history with optional filters and pagination.
func (ut *usageTrackerImpl) GetRequestHistory(
	ctx context.Context,
	filter *RequestHistoryFilter,
	page int,
	pageSize int,
) (*RequestHistoryResponse, error) {
	// Set default pagination values
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 1000 {
		pageSize = 100
	}
	offset := (page - 1) * pageSize

	// Build query with filters
	query := `
		SELECT id, token_id, model, request_count, input_tokens, output_tokens, timestamp, created_at
		FROM request_history
		WHERE 1=1
	`
	args := []interface{}{}
	argCount := 0

	if filter != nil {
		if filter.TokenID != "" {
			argCount++
			query += fmt.Sprintf(" AND token_id = $%d", argCount)
			args = append(args, filter.TokenID)
		}
		if filter.Model != "" {
			argCount++
			query += fmt.Sprintf(" AND model = $%d", argCount)
			args = append(args, filter.Model)
		}
		if filter.StartTime > 0 {
			argCount++
			query += fmt.Sprintf(" AND timestamp >= $%d", argCount)
			args = append(args, filter.StartTime)
		}
		if filter.EndTime > 0 {
			argCount++
			query += fmt.Sprintf(" AND timestamp <= $%d", argCount)
			args = append(args, filter.EndTime)
		}
		if filter.MinTokens > 0 {
			argCount++
			query += fmt.Sprintf(" AND (input_tokens + output_tokens) >= $%d", argCount)
			args = append(args, filter.MinTokens)
		}
		if filter.MaxTokens > 0 {
			argCount++
			query += fmt.Sprintf(" AND (input_tokens + output_tokens) <= $%d", argCount)
			args = append(args, filter.MaxTokens)
		}
	}

	// Get total count
	countQuery := "SELECT COUNT(*) FROM request_history" + query[31:] // Skip "SELECT id, token_id..."
	var totalCount int
	if err := ut.db.QueryRowContext(ctx, countQuery, args...).Scan(&totalCount); err != nil {
		return nil, fmt.Errorf("failed to count request history: %w", err)
	}

	// Add ordering and pagination
	query += " ORDER BY timestamp DESC"
	argCount++
	query += fmt.Sprintf(" LIMIT $%d", argCount)
	args = append(args, pageSize)
	argCount++
	query += fmt.Sprintf(" OFFSET $%d", argCount)
	args = append(args, offset)

	// Execute query
	rows, err := ut.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query request history: %w", err)
	}
	defer rows.Close()

	var records []RequestHistoryRecord
	for rows.Next() {
		var record RequestHistoryRecord
		if err := rows.Scan(
			&record.ID,
			&record.TokenID,
			&record.Model,
			&record.RequestCount,
			&record.InputTokens,
			&record.OutputTokens,
			&record.Timestamp,
			&record.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan request history record: %w", err)
		}
		record.TotalTokens = record.InputTokens + record.OutputTokens
		records = append(records, record)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating request history: %w", err)
	}

	totalPages := (totalCount + pageSize - 1) / pageSize
	if totalPages == 0 {
		totalPages = 1
	}

	return &RequestHistoryResponse{
		Records:     records,
		TotalCount:  totalCount,
		Page:        page,
		PageSize:    pageSize,
		TotalPages:  totalPages,
		HasNext:     page < totalPages,
		HasPrevious: page > 1,
	}, nil
}

// GetRequestHistoryByToken retrieves request history for a specific token.
func (ut *usageTrackerImpl) GetRequestHistoryByToken(
	ctx context.Context,
	tokenID string,
	filter *RequestHistoryFilter,
	page int,
	pageSize int,
) (*RequestHistoryResponse, error) {
	if filter == nil {
		filter = &RequestHistoryFilter{}
	}
	filter.TokenID = tokenID
	return ut.GetRequestHistory(ctx, filter, page, pageSize)
}

// GetRequestHistoryByModel retrieves request history for a specific model.
func (ut *usageTrackerImpl) GetRequestHistoryByModel(
	ctx context.Context,
	model string,
	filter *RequestHistoryFilter,
	page int,
	pageSize int,
) (*RequestHistoryResponse, error) {
	if filter == nil {
		filter = &RequestHistoryFilter{}
	}
	filter.Model = model
	return ut.GetRequestHistory(ctx, filter, page, pageSize)
}

// GetRequestHistorySummary returns aggregated statistics for request history.
func (ut *usageTrackerImpl) GetRequestHistorySummary(
	ctx context.Context,
	tokenID string,
	model string,
	startTime int64,
	endTime int64,
) (*RequestHistorySummary, error) {
	query := `
		SELECT
			COALESCE(SUM(request_count), 0) as total_requests,
			COALESCE(SUM(input_tokens), 0) as total_input,
			COALESCE(SUM(output_tokens), 0) as total_output,
			COALESCE(MIN(timestamp), 0) as first_request,
			COALESCE(MAX(timestamp), 0) as last_request
		FROM request_history
		WHERE 1=1
	`
	args := []interface{}{}
	argCount := 0

	if tokenID != "" {
		argCount++
		query += fmt.Sprintf(" AND token_id = $%d", argCount)
		args = append(args, tokenID)
	}
	if model != "" {
		argCount++
		query += fmt.Sprintf(" AND model = $%d", argCount)
		args = append(args, model)
	}
	if startTime > 0 {
		argCount++
		query += fmt.Sprintf(" AND timestamp >= $%d", argCount)
		args = append(args, startTime)
	}
	if endTime > 0 {
		argCount++
		query += fmt.Sprintf(" AND timestamp <= $%d", argCount)
		args = append(args, endTime)
	}

	var summary RequestHistorySummary
	summary.TokenID = tokenID
	summary.Model = model

	if err := ut.db.QueryRowContext(ctx, query, args...).Scan(
		&summary.TotalRequests,
		&summary.TotalInput,
		&summary.TotalOutput,
		&summary.FirstRequest,
		&summary.LastRequest,
	); err != nil {
		return nil, fmt.Errorf("failed to query request history summary: %w", err)
	}

	summary.TotalTokens = summary.TotalInput + summary.TotalOutput
	if summary.TotalRequests > 0 {
		summary.AverageTokens = float64(summary.TotalTokens) / float64(summary.TotalRequests)
	}

	return &summary, nil
}

// DeleteRequestHistory deletes request history records matching the filter.
func (ut *usageTrackerImpl) DeleteRequestHistory(
	ctx context.Context,
	filter *RequestHistoryFilter,
) (int64, error) {
	query := "DELETE FROM request_history WHERE 1=1"
	args := []interface{}{}
	argCount := 0

	if filter != nil {
		if filter.TokenID != "" {
			argCount++
			query += fmt.Sprintf(" AND token_id = $%d", argCount)
			args = append(args, filter.TokenID)
		}
		if filter.Model != "" {
			argCount++
			query += fmt.Sprintf(" AND model = $%d", argCount)
			args = append(args, filter.Model)
		}
		if filter.StartTime > 0 {
			argCount++
			query += fmt.Sprintf(" AND timestamp >= $%d", argCount)
			args = append(args, filter.StartTime)
		}
		if filter.EndTime > 0 {
			argCount++
			query += fmt.Sprintf(" AND timestamp <= $%d", argCount)
			args = append(args, filter.EndTime)
		}
	} else {
		// Safety: don't allow deleting all records without a filter
		return 0, fmt.Errorf("filter is required for delete operation")
	}

	result, err := ut.db.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("failed to delete request history: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	return rowsAffected, nil
}
