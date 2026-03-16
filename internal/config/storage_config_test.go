package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStorageConfig_Defaults(t *testing.T) {
	cfg := DefaultConfig()

	assert.Equal(t, DefaultStoragePath, cfg.Storage.DBPath)
}

func TestStorageConfig_EnvVars(t *testing.T) {
	// Set environment variable
	customPath := "/custom/path/tokens.db"
	os.Setenv("STORAGE_DB_PATH", customPath)
	defer os.Unsetenv("STORAGE_DB_PATH")

	cfg := LoadConfig()
	assert.Equal(t, customPath, cfg.Storage.DBPath)
}

func TestStorageConfig_DefaultPath(t *testing.T) {
	cfg := DefaultConfig()
	require.NotNil(t, cfg.Storage)
	assert.Equal(t, ".credentials/tokens.db", cfg.Storage.DBPath)
}
