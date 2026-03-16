package token

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/sunbankio/qwencoder-proxy/internal/logging"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewSQLiteStore(t *testing.T) {
	tests := []struct {
		name        string
		dbPath      string
		providerID  string
		expectError bool
	}{
		{
			name:        "successful initialization with in-memory database",
			dbPath:      ":memory:",
			providerID:  "test-provider",
			expectError: false,
		},
		{
			name:        "successful initialization with file database",
			dbPath:      filepath.Join(t.TempDir(), "test.db"),
			providerID:  "test-provider",
			expectError: false,
		},
		{
			name:        "initialization with invalid path",
			dbPath:      "/invalid/path/that/does/not/exist/test.db",
			providerID:  "test-provider",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := logging.NewLogger()
			store, err := NewSQLiteStore(tt.dbPath, tt.providerID, logger)

			if tt.expectError {
				require.Error(t, err)
				assert.Nil(t, store)
			} else {
				require.NoError(t, err)
				require.NotNil(t, store)
				assert.NotNil(t, store.db)
				assert.Equal(t, tt.providerID, store.providerID)
				assert.NotNil(t, store.stmtInsertToken)
				assert.NotNil(t, store.stmtUpdateToken)
				assert.NotNil(t, store.stmtSelectTokenByID)
				assert.NotNil(t, store.stmtSelectTokensByProvider)
				assert.NotNil(t, store.stmtSelectValidTokens)
				assert.NotNil(t, store.stmtDeleteToken)
				assert.NotNil(t, store.stmtSelectSettings)
				assert.NotNil(t, store.stmtUpsertSettings)

				// Cleanup
				if tt.dbPath != ":memory:" {
					require.NoError(t, store.Close())
				}
			}
		})
	}
}

func TestSQLiteStore_Ping(t *testing.T) {
	logger := logging.NewLogger()
	store, err := NewSQLiteStore(":memory:", "test-provider", logger)
	require.NoError(t, err)
	defer store.Close()

	// Test successful ping
	err = store.Ping()
	assert.NoError(t, err)
}

func TestSQLiteStore_Close(t *testing.T) {
	logger := logging.NewLogger()
	store, err := NewSQLiteStore(":memory:", "test-provider", logger)
	require.NoError(t, err)

	// Test successful close
	err = store.Close()
	assert.NoError(t, err)

	// Verify statements are closed
	assert.Nil(t, store.stmtInsertToken)
	assert.Nil(t, store.stmtUpdateToken)
	assert.Nil(t, store.stmtSelectTokenByID)
	assert.Nil(t, store.stmtSelectTokensByProvider)
	assert.Nil(t, store.stmtSelectValidTokens)
	assert.Nil(t, store.stmtDeleteToken)
	assert.Nil(t, store.stmtSelectSettings)
	assert.Nil(t, store.stmtUpsertSettings)
	assert.Nil(t, store.db)

	// Double close should not panic
	err = store.Close()
	assert.NoError(t, err)
}

func TestSQLiteStore_Pragmas(t *testing.T) {
	logger := logging.NewLogger()
	store, err := NewSQLiteStore(":memory:", "test-provider", logger)
	require.NoError(t, err)
	defer store.Close()

	// Verify WAL mode is enabled
	var journalMode string
	err = store.db.QueryRow("PRAGMA journal_mode").Scan(&journalMode)
	require.NoError(t, err)
	assert.Equal(t, "wal", journalMode)

	// Verify foreign keys are enabled
	var foreignKeys int
	err = store.db.QueryRow("PRAGMA foreign_keys").Scan(&foreignKeys)
	require.NoError(t, err)
	assert.Equal(t, 1, foreignKeys)

	// Verify synchronous mode
	var synchronous string
	err = store.db.QueryRow("PRAGMA synchronous").Scan(&synchronous)
	require.NoError(t, err)
	assert.Equal(t, "2", synchronous) // NORMAL = 2
}

func TestSQLiteStore_ConnectionPool(t *testing.T) {
	logger := logging.NewLogger()
	store, err := NewSQLiteStore(":memory:", "test-provider", logger)
	require.NoError(t, err)
	defer store.Close()

	// Verify connection pool is configured
	assert.Equal(t, 25, store.db.Stats().MaxOpenConnections)
}

func TestSQLiteStore_ConcurrentInitialization(t *testing.T) {
	logger := logging.NewLogger()
	dbPath := filepath.Join(t.TempDir(), "concurrent.db")

	// Initialize multiple stores concurrently
	const numStores = 10
	stores := make([]*SQLiteStore, numStores)
	errors := make(chan error, numStores)

	for i := 0; i < numStores; i++ {
		go func(idx int) {
			store, err := NewSQLiteStore(dbPath, "test-provider", logger)
			if err != nil {
				errors <- err
				return
			}
			stores[idx] = store
			errors <- nil
		}(i)
	}

	// Wait for all initializations
	for i := 0; i < numStores; i++ {
		err := <-errors
		require.NoError(t, err)
	}

	// Verify all stores are initialized
	for _, store := range stores {
		require.NotNil(t, store)
		require.NoError(t, store.Close())
	}
}

func TestSQLiteStore_ReopenExistingDatabase(t *testing.T) {
	logger := logging.NewLogger()
	dbPath := filepath.Join(t.TempDir(), "reopen.db")

	// Create and close a store
	store1, err := NewSQLiteStore(dbPath, "test-provider", logger)
	require.NoError(t, err)
	require.NoError(t, store1.Close())

	// Reopen the same database
	store2, err := NewSQLiteStore(dbPath, "test-provider", logger)
	require.NoError(t, err)
	defer store2.Close()

	// Verify schema version is preserved
	var version int
	err = store2.db.QueryRow("SELECT MAX(version) FROM schema_migrations").Scan(&version)
	require.NoError(t, err)
	assert.Equal(t, 1, version)
}

func TestSQLiteStore_Persistence(t *testing.T) {
	logger := logging.NewLogger()
	dbPath := filepath.Join(t.TempDir(), "persist.db")

	// Create store and add a token
	store1, err := NewSQLiteStore(dbPath, "test-provider", logger)
	require.NoError(t, err)

	token := TokenMetadata{
		ID:           "test-id-1",
		AccessToken:  "test-access-token",
		RefreshToken: "test-refresh-token",
		TokenType:    "Bearer",
		ExpiryDate:   time.Now().Add(time.Hour).UnixMilli(),
		Email:        "test@example.com",
		Healthy:      true,
		HealthScore:  1.0,
		LastUsed:     time.Now().UnixMilli(),
		CreatedAt:    time.Now().UnixMilli(),
		ErrorCount:   0,
	}

	err = store1.AddToken(token)
	require.NoError(t, err)
	require.NoError(t, store1.Close())

	// Reopen and verify token persists
	store2, err := NewSQLiteStore(dbPath, "test-provider", logger)
	require.NoError(t, err)
	defer store2.Close()

	retrieved, err := store2.GetToken("test-id-1")
	require.NoError(t, err)
	assert.Equal(t, token.ID, retrieved.ID)
	assert.Equal(t, token.Email, retrieved.Email)
	assert.Equal(t, token.AccessToken, retrieved.AccessToken)
}
