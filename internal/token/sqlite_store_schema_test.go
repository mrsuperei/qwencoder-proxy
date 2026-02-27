package token

import (
	"testing"

	"github.com/sunbankio/qwencoder-proxy/logging"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSQLiteStore_MigrateToV1(t *testing.T) {
	// Setup
	logger := logging.NewLogger()
	store, err := NewSQLiteStore(":memory:", "test-provider", logger)
	require.NoError(t, err)
	defer store.Close()

	// Verify tables exist
	var tableExists int
	err = store.db.QueryRow(`
		SELECT COUNT(*) FROM sqlite_master
		WHERE type='table' AND name IN ('tokens', 'provider_settings', 'proxy_configs', 'schema_migrations')
	`).Scan(&tableExists)
	require.NoError(t, err)
	assert.Equal(t, 4, tableExists)

	// Verify indexes exist
	var indexCount int
	err = store.db.QueryRow(`
		SELECT COUNT(*) FROM sqlite_master
		WHERE type='index' AND tbl_name='tokens'
	`).Scan(&indexCount)
	require.NoError(t, err)
	// 6 explicit indexes + 1 PRIMARY KEY index = 7 total
	assert.GreaterOrEqual(t, 7, indexCount)

	// Verify schema_migrations table has version 1
	var version int
	err = store.db.QueryRow("SELECT version FROM schema_migrations WHERE version = 1").Scan(&version)
	require.NoError(t, err)
	assert.Equal(t, 1, version)
}

func TestSQLiteStore_BoolToInt(t *testing.T) {
	tests := []struct {
		name     string
		input    bool
		expected int
	}{
		{"true converts to 1", true, 1},
		{"false converts to 0", false, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := boolToInt(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSQLiteStore_IntToBool(t *testing.T) {
	tests := []struct {
		name     string
		input    int
		expected bool
	}{
		{"1 converts to true", 1, true},
		{"0 converts to false", 0, false},
		{"2 converts to true", 2, true},
		{"-1 converts to true", -1, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := intToBool(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
