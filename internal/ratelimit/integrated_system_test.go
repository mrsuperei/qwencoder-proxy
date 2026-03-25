package ratelimit

import (
	"context"
	"database/sql"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

// testLogger is a mock logger for testing.
type testLogger struct{}

func (m *testLogger) InfoLog(msg string, args ...interface{})  {}
func (m *testLogger) DebugLog(msg string, args ...interface{}) {}
func (m *testLogger) WarnLog(msg string, args ...interface{})  {}
func (m *testLogger) ErrorLog(msg string, args ...interface{}) {}

// createTestDB creates a temporary database for testing.
func createTestDB(t *testing.T) *sql.DB {
	file, err := os.CreateTemp("", "testdb-*.db")
	require.NoError(t, err)
	file.Close()

	db, err := sql.Open("sqlite", file.Name())
	require.NoError(t, err)

	// Create tables
	_, err = db.Exec(`
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
	`)
	require.NoError(t, err)

	_, err = db.Exec(`
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
	`)
	require.NoError(t, err)

	_, err = db.Exec(`
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
	`)
	require.NoError(t, err)

	_, err = db.Exec(`
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
	`)
	require.NoError(t, err)

	// Create indexes for efficient queries
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
		_, err = db.Exec(idx)
		require.NoError(t, err)
	}

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS tokens (
			id TEXT PRIMARY KEY,
			provider_id TEXT NOT NULL,
			access_token TEXT,
			refresh_token TEXT,
			token_type TEXT,
			expires_at INTEGER,
			created_at INTEGER DEFAULT (strftime('%s', 'now')),
			updated_at INTEGER DEFAULT (strftime('%s', 'now')),
			email TEXT,
			resource_url TEXT,
			scope TEXT,
			api_key TEXT,
			project_id TEXT,
			healthy INTEGER NOT NULL DEFAULT 1,
			health_score REAL NOT NULL DEFAULT 1.0,
			last_used INTEGER NOT NULL DEFAULT 0,
			error_count INTEGER NOT NULL DEFAULT 0,
			last_error TEXT,
			proxy_id TEXT
		)
	`)
	require.NoError(t, err)

	t.Cleanup(func() {
		db.Close()
		os.Remove(file.Name())
	})

	// Insert a test token for the test provider
	_, err = db.Exec(`
		INSERT OR IGNORE INTO tokens (id, provider_id, token_type, created_at, updated_at, healthy, health_score, error_count)
		VALUES (?, ?, ?, strftime('%s', 'now'), strftime('%s', 'now'), 1, 1.0, 0)
	`, "test-token", "gemini-cli", "oauth")
	require.NoError(t, err)

	return db
}

// TestNewIntegratedRateLimitSystem tests system creation.
func TestNewIntegratedRateLimitSystem(t *testing.T) {
	tests := []struct {
		name    string
		config  *SystemConfig
		wantErr bool
	}{
		{
			name:    "default config",
			config:  DefaultSystemConfig(),
			wantErr: false,
		},
		{
			name:    "nil config",
			config:  nil,
			wantErr: false,
		},
		{
			name: "custom config with async enabled",
			config: &SystemConfig{
				AsyncRecording: &AsyncUsageRecorderConfig{
					Enabled:     true,
					WorkerCount: 3,
					QueueSize:   100,
				},
				Caching: DefaultCacheConfig(),
				DBNotification: &DBNotificationConfig{
					Enabled:         false,
					PollingInterval: 5 * time.Second,
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := createTestDB(t)
			logger := &testLogger{}

			system, err := NewIntegratedRateLimitSystem(db, logger, tt.config)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, system)
			} else {
				require.NoError(t, err)
				require.NotNil(t, system)

				assert.NotNil(t, system.GetQuotaManager())
				assert.NotNil(t, system.GetCacheInvalidator())
				assert.NotNil(t, system.GetAsyncRecorder())
				assert.NotNil(t, system.GetCachedTracker())
				assert.False(t, system.IsStarted())
				assert.False(t, system.IsStopped())
			}
		})
	}
}

// TestIntegratedRateLimitSystem_StartStop tests system lifecycle.
func TestIntegratedRateLimitSystem_StartStop(t *testing.T) {
	db := createTestDB(t)
	logger := &testLogger{}

	system, err := NewIntegratedRateLimitSystem(db, logger, DefaultSystemConfig())
	require.NoError(t, err)

	// Test starting
	err = system.Start()
	require.NoError(t, err)
	assert.True(t, system.IsStarted())
	assert.False(t, system.IsStopped())

	// Test double start
	err = system.Start()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already started")

	// Test stopping
	err = system.Stop()
	require.NoError(t, err)
	assert.True(t, system.IsStopped())
	assert.False(t, system.IsStarted())

	// Test double stop
	err = system.Stop()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already stopped")
}

// TestIntegratedRateLimitSystem_GetMetrics tests metrics collection.
func TestIntegratedRateLimitSystem_GetMetrics(t *testing.T) {
	db := createTestDB(t)
	logger := &testLogger{}

	system, err := NewIntegratedRateLimitSystem(db, logger, DefaultSystemConfig())
	require.NoError(t, err)

	// Get initial metrics
	metrics := system.GetMetrics()
	assert.NotNil(t, metrics)
	assert.False(t, metrics.StartTime.IsZero())
	assert.Equal(t, int64(0), metrics.RequestsProcessed)
	assert.Equal(t, int64(0), metrics.RequestsRejected)

	// Record some activity
	system.RecordRequestProcessed()
	system.RecordRequestProcessed()
	system.RecordRequestRejected()

	// Get updated metrics
	metrics = system.GetMetrics()
	assert.Equal(t, int64(2), metrics.RequestsProcessed)
	assert.Equal(t, int64(1), metrics.RequestsRejected)
}

// TestIntegratedRateLimitSystem_ConcurrentAccess tests concurrent access to system.
func TestIntegratedRateLimitSystem_ConcurrentAccess(t *testing.T) {
	db := createTestDB(t)
	logger := &testLogger{}

	system, err := NewIntegratedRateLimitSystem(db, logger, DefaultSystemConfig())
	require.NoError(t, err)

	err = system.Start()
	require.NoError(t, err)
	defer system.Stop()

	// Concurrent access test
	var wg sync.WaitGroup
	numGoroutines := 100
	numOperations := 1000

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < numOperations/numGoroutines; j++ {
				system.RecordRequestProcessed()
				_ = system.GetMetrics()
			}
		}()
	}

	wg.Wait()

	// Verify metrics
	metrics := system.GetMetrics()
	assert.Equal(t, int64(numOperations), metrics.RequestsProcessed)
}

// TestIntegratedRateLimitSystem_FullRequestFlow tests full request flow.
func TestIntegratedRateLimitSystem_FullRequestFlow(t *testing.T) {
	db := createTestDB(t)
	logger := &testLogger{}

	system, err := NewIntegratedRateLimitSystem(db, logger, DefaultSystemConfig())
	require.NoError(t, err)

	err = system.Start()
	require.NoError(t, err)
	defer system.Stop()

	quotaManager := system.GetQuotaManager()
	ctx := context.Background()

	// Test 1: Check quota for new provider
	providerID := "gemini-cli"
	tokenID := "test-token"

	allowed, status, err := quotaManager.CheckQuota(ctx, providerID, tokenID, 100)
	require.NoError(t, err)
	assert.True(t, allowed)
	assert.NotNil(t, status)

	// Test 2: Record usage (using RecordModelUsage for complete tracking)
	err = quotaManager.RecordModelUsage(ctx, providerID, tokenID, "test-model", 100, 50)
	require.NoError(t, err)

	// Record that a request was processed for metrics
	system.RecordRequestProcessed()

	// Test 3: Check quota again
	allowed, status, err = quotaManager.CheckQuota(ctx, providerID, tokenID, 100)
	require.NoError(t, err)
	assert.True(t, allowed)

	// Test 4: Get quota status
	status, err = quotaManager.GetQuotaStatus(ctx, providerID, tokenID)
	require.NoError(t, err)
	assert.NotNil(t, status)
	assert.Equal(t, providerID, status.ProviderID)
	assert.Equal(t, tokenID, status.TokenID)

	// Verify metrics
	metrics := system.GetMetrics()
	assert.Greater(t, metrics.RequestsProcessed, int64(0))
}

// TestDefaultSystemConfig tests default system configuration.
func TestDefaultSystemConfig(t *testing.T) {
	config := DefaultSystemConfig()

	assert.NotNil(t, config.AsyncRecording)
	assert.NotNil(t, config.Caching)
	assert.NotNil(t, config.DBNotification)

	// Check async defaults
	assert.False(t, config.AsyncRecording.Enabled)
	assert.Equal(t, 1, config.AsyncRecording.WorkerCount)
	assert.Equal(t, 1000, config.AsyncRecording.QueueSize)

	// Check cache defaults
	assert.Equal(t, 5*time.Second, config.Caching.ProviderMetricsTTL)
	assert.Equal(t, 5*time.Second, config.Caching.TokenMetricsTTL)
	assert.Equal(t, 100, config.Caching.MaxProviderEntries)
	assert.Equal(t, 1000, config.Caching.MaxTokenEntries)

	// Check DB notification defaults
	assert.False(t, config.DBNotification.Enabled)
	assert.Equal(t, 5*time.Second, config.DBNotification.PollingInterval)
}

// TestDefaultDBNotificationConfig tests default DB notification configuration.
func TestDefaultDBNotificationConfig(t *testing.T) {
	config := DefaultDBNotificationConfig()

	assert.False(t, config.Enabled)
	assert.Equal(t, 5*time.Second, config.PollingInterval)
}

// TestIntegratedRateLimitSystem_GracefulShutdown tests graceful shutdown.
func TestIntegratedRateLimitSystem_GracefulShutdown(t *testing.T) {
	db := createTestDB(t)
	logger := &testLogger{}

	config := &SystemConfig{
		AsyncRecording: &AsyncUsageRecorderConfig{
			Enabled:     true,
			WorkerCount: 2,
			QueueSize:   10,
		},
		Caching: DefaultCacheConfig(),
		DBNotification: &DBNotificationConfig{
			Enabled:         true,
			PollingInterval: 1 * time.Second,
		},
	}

	system, err := NewIntegratedRateLimitSystem(db, logger, config)
	require.NoError(t, err)

	err = system.Start()
	require.NoError(t, err)

	// Record some usage (using RecordModelUsage for complete tracking)
	quotaManager := system.GetQuotaManager()
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		_ = quotaManager.RecordModelUsage(ctx, "provider", "token", "test-model", 100, 50)
		system.RecordRequestProcessed()
	}

	// Give async recorder time to process
	time.Sleep(100 * time.Millisecond)

	// Stop system
	startTime := time.Now()
	err = system.Stop()
	duration := time.Since(startTime)

	require.NoError(t, err)
	assert.Less(t, duration, 5*time.Second, "Shutdown should complete quickly")

	// Verify uptime is recorded
	metrics := system.GetMetrics()
	assert.Greater(t, metrics.Uptime, time.Duration(0))
}

// TestIntegratedRateLimitSystem_ComponentAccess tests component access methods.
func TestIntegratedRateLimitSystem_ComponentAccess(t *testing.T) {
	db := createTestDB(t)
	logger := &testLogger{}

	system, err := NewIntegratedRateLimitSystem(db, logger, DefaultSystemConfig())
	require.NoError(t, err)

	// Test component access
	quotaManager := system.GetQuotaManager()
	assert.NotNil(t, quotaManager)

	cacheInvalidator := system.GetCacheInvalidator()
	assert.NotNil(t, cacheInvalidator)

	asyncRecorder := system.GetAsyncRecorder()
	assert.NotNil(t, asyncRecorder)

	cachedTracker := system.GetCachedTracker()
	assert.NotNil(t, cachedTracker)
}

// TestRecordError tests error recording functionality.
func TestRecordError(t *testing.T) {
	db := createTestDB(t)
	logger := &testLogger{}
	quotaManager := NewQuotaManager(nil, logger, db, nil, nil, nil)
	ctx := context.Background()

	// Insert an additional test token
	_, err := db.Exec(`
		INSERT OR IGNORE INTO tokens (id, provider_id, token_type, created_at, updated_at, healthy, health_score, error_count)
		VALUES (?, ?, ?, strftime('%s', 'now'), strftime('%s', 'now'), 1, 1.0, 0)
	`, "test-token-2", "provider1", "oauth")
	require.NoError(t, err)

	// Record a rate limit error
	err = quotaManager.RecordError(ctx, "provider1", "test-token-2", "rate_limit", "Rate limit exceeded", 429)
	require.NoError(t, err)

	// Verify error was recorded
	var errorCount int
	var lastError string
	var healthScore float64
	var healthy int

	err = db.QueryRow(`
		SELECT error_count, last_error, health_score, healthy
		FROM tokens
		WHERE id = ?
	`, "test-token-2").Scan(&errorCount, &lastError, &healthScore, &healthy)

	require.NoError(t, err)
	assert.Equal(t, 1, errorCount)
	assert.Equal(t, "Rate limit exceeded", lastError)
	assert.Less(t, healthScore, 1.0, "Health score should be reduced after error")
	assert.Equal(t, 1, healthy, "Token should still be healthy with one error")
}

// TestMultipleErrors tests recording multiple errors.
func TestMultipleErrors(t *testing.T) {
	db := createTestDB(t)
	logger := &testLogger{}
	quotaManager := NewQuotaManager(nil, logger, db, nil, nil, nil)
	ctx := context.Background()

	// Insert an additional test token
	_, err := db.Exec(`
		INSERT OR IGNORE INTO tokens (id, provider_id, token_type, created_at, updated_at, healthy, health_score, error_count)
		VALUES (?, ?, ?, strftime('%s', 'now'), strftime('%s', 'now'), 1, 1.0, 0)
	`, "test-token-3", "provider1", "oauth")
	require.NoError(t, err)

	// Record multiple errors
	for i := 0; i < 5; i++ {
		err = quotaManager.RecordError(ctx, "provider1", "test-token-3", "rate_limit", "Rate limit exceeded", 429)
		require.NoError(t, err)
	}

	// Verify error count increased
	var errorCount int
	var healthScore float64
	var healthy int

	err = db.QueryRow(`
		SELECT error_count, health_score, healthy
		FROM tokens
		WHERE id = ?
	`, "test-token-3").Scan(&errorCount, &healthScore, &healthy)

	require.NoError(t, err)
	assert.Equal(t, 5, errorCount)
	assert.Less(t, healthScore, 0.5, "Health score should be significantly reduced after multiple errors")
}

// TestResetTokenErrors tests error reset functionality.
func TestResetTokenErrors(t *testing.T) {
	db := createTestDB(t)
	logger := &testLogger{}
	quotaManager := NewQuotaManager(nil, logger, db, nil, nil, nil)
	ctx := context.Background()

	// Insert an additional test token
	_, err := db.Exec(`
		INSERT OR IGNORE INTO tokens (id, provider_id, token_type, created_at, updated_at, healthy, health_score, error_count, last_error)
		VALUES (?, ?, ?, strftime('%s', 'now'), strftime('%s', 'now'), 0, 0.5, 3, 'Some error')
	`, "test-token-4", "provider1", "oauth")
	require.NoError(t, err)

	// Reset errors
	err = quotaManager.ResetTokenErrors(ctx, "provider1", "test-token-4")
	require.NoError(t, err)

	// Verify reset
	var errorCount int
	var healthScore float64
	var healthy int
	var lastError sql.NullString

	err = db.QueryRow(`
		SELECT error_count, health_score, healthy, last_error
		FROM tokens
		WHERE id = ?
	`, "test-token-4").Scan(&errorCount, &healthScore, &healthy, &lastError)

	require.NoError(t, err)
	assert.Equal(t, 0, errorCount)
	assert.Equal(t, 1.0, healthScore)
	assert.Equal(t, 1, healthy)
	assert.False(t, lastError.Valid, "Last error should be NULL after reset")
}

// TestGetTokenErrors tests retrieving token error information.
func TestGetTokenErrors(t *testing.T) {
	db := createTestDB(t)
	logger := &testLogger{}
	quotaManager := NewQuotaManager(nil, logger, db, nil, nil, nil)
	ctx := context.Background()

	// Insert an additional test token with errors
	_, err := db.Exec(`
		INSERT OR IGNORE INTO tokens (id, provider_id, token_type, created_at, updated_at, healthy, health_score, error_count, last_error)
		VALUES (?, ?, ?, strftime('%s', 'now'), strftime('%s', 'now'), 1, 0.8, 2, 'Test error message')
	`, "test-token-5", "provider1", "oauth")
	require.NoError(t, err)

	// Get error information
	errorInfo, err := quotaManager.GetTokenErrors(ctx, "provider1", "test-token-5")
	require.NoError(t, err)
	assert.Equal(t, "test-token-5", errorInfo.TokenID)
	assert.Equal(t, "provider1", errorInfo.ProviderID)
	assert.Equal(t, 2, errorInfo.ErrorCount)
	assert.Equal(t, "Test error message", errorInfo.LastError)
	assert.Equal(t, 0.8, errorInfo.HealthScore)
	assert.True(t, errorInfo.Healthy)
}

// TestGetAllTokenErrors tests retrieving all token errors for a provider.
func TestGetAllTokenErrors(t *testing.T) {
	db := createTestDB(t)
	logger := &testLogger{}
	quotaManager := NewQuotaManager(nil, logger, db, nil, nil, nil)
	ctx := context.Background()

	// Insert multiple test tokens
	tokens := []struct {
		id         string
		providerID string
		errorCount int
	}{
		{"token-a", "provider1", 1},
		{"token-b", "provider1", 3},
		{"token-c", "provider1", 0},
	}

	for _, tok := range tokens {
		_, err := db.Exec(`
			INSERT OR IGNORE INTO tokens (id, provider_id, token_type, created_at, updated_at, healthy, health_score, error_count)
			VALUES (?, ?, ?, strftime('%s', 'now'), strftime('%s', 'now'), 1, 1.0, ?)
		`, tok.id, tok.providerID, "oauth", tok.errorCount)
		require.NoError(t, err)
	}

	// Get all errors
	errors, err := quotaManager.GetAllTokenErrors(ctx, "provider1")
	require.NoError(t, err)
	assert.Len(t, errors, 3, "Should return all tokens for provider")

	// Verify ordering (should be ordered by error_count DESC)
	assert.Equal(t, "token-b", errors[0].TokenID)
	assert.Equal(t, 3, errors[0].ErrorCount)
	assert.Equal(t, "token-a", errors[1].TokenID)
	assert.Equal(t, 1, errors[1].ErrorCount)
	assert.Equal(t, "token-c", errors[2].TokenID)
	assert.Equal(t, 0, errors[2].ErrorCount)
}

// TestRecoverTokenHealth tests token health recovery.
func TestRecoverTokenHealth(t *testing.T) {
	db := createTestDB(t)
	logger := &testLogger{}
	quotaManager := NewQuotaManager(nil, logger, db, nil, nil, nil)
	ctx := context.Background()

	// Insert a test token with low health score but low error count
	_, err := db.Exec(`
		INSERT OR IGNORE INTO tokens (id, provider_id, token_type, created_at, updated_at, healthy, health_score, error_count)
		VALUES (?, ?, ?, strftime('%s', 'now'), strftime('%s', 'now'), 1, 0.4, 2)
	`, "test-token-6", "provider1", "oauth")
	require.NoError(t, err)

	// Recover token health
	err = quotaManager.RecoverTokenHealth(ctx, "provider1", "test-token-6")
	require.NoError(t, err)

	// Verify recovery
	var healthScore float64
	var healthy int

	err = db.QueryRow(`
		SELECT health_score, healthy
		FROM tokens
		WHERE id = ?
	`, "test-token-6").Scan(&healthScore, &healthy)

	require.NoError(t, err)
	assert.Greater(t, healthScore, 0.4, "Health score should be increased after recovery")
	assert.Equal(t, 1, healthy, "Token should be marked healthy after recovery")
}

// TestRecoverTokenHealth_TooManyErrors tests that tokens with too many errors cannot be recovered.
func TestRecoverTokenHealth_TooManyErrors(t *testing.T) {
	db := createTestDB(t)
	logger := &testLogger{}
	quotaManager := NewQuotaManager(nil, logger, db, nil, nil, nil)
	ctx := context.Background()

	// Insert a test token with low health score and high error count
	_, err := db.Exec(`
		INSERT OR IGNORE INTO tokens (id, provider_id, token_type, created_at, updated_at, healthy, health_score, error_count)
		VALUES (?, ?, ?, strftime('%s', 'now'), strftime('%s', 'now'), 0, 0.2, 15)
	`, "test-token-7", "provider1", "oauth")
	require.NoError(t, err)

	// Try to recover token health (should fail)
	err = quotaManager.RecoverTokenHealth(ctx, "provider1", "test-token-7")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "too many errors")
}
