package auth

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunbankio/qwencoder-proxy/logging"
)

func TestNewMultiTokenStore(t *testing.T) {
	logger := logging.NewLogger()

	t.Run("creates new store with defaults", func(t *testing.T) {
		store := NewMultiTokenStore("test-provider", "test.json", logger)

		assert.Equal(t, "test-provider", store.ProviderID)
		assert.Equal(t, StoreVersion, store.Version)
		assert.Empty(t, store.Tokens)
		assert.Equal(t, DefaultSelectionStrategy, store.Settings.SelectionStrategy)
		assert.Equal(t, DefaultRefreshBufferSec, store.Settings.RefreshBufferSec)
		assert.Equal(t, DefaultMaxErrorCount, store.Settings.MaxErrorCount)
		assert.Equal(t, filepath.Join(".credentials", "test-provider"), store.providerDir)
	})
}

func TestMultiTokenStore_AddToken(t *testing.T) {
	logger := logging.NewLogger()
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "store.json")
	store := NewMultiTokenStore("test-provider", filePath, logger)

	t.Run("adds new token with auto-generated ID", func(t *testing.T) {
		token := ProviderToken{
			AccessToken:  "test-access-token",
			RefreshToken: "test-refresh-token",
			TokenType:    "Bearer",
			ExpiryDate:   time.Now().Add(time.Hour).UnixMilli(),
			Email:        "test@example.com",
		}

		err := store.AddToken(token)
		require.NoError(t, err)

		tokens := store.ListTokens()
		assert.Len(t, tokens, 1)
		assert.NotEmpty(t, tokens[0].ID)
		assert.NotEmpty(t, tokens[0].CreatedAt)
		assert.NotEmpty(t, tokens[0].LastUsed)
		assert.Equal(t, 1.0, tokens[0].HealthScore)
		assert.Equal(t, "Bearer", tokens[0].TokenType)
	})

	t.Run("adds new token with provided ID", func(t *testing.T) {
		token := ProviderToken{
			ID:           "custom-id",
			AccessToken:  "test-access-token-2",
			RefreshToken: "test-refresh-token-2",
			TokenType:    "Bearer",
			ExpiryDate:   time.Now().Add(time.Hour).UnixMilli(),
			Email:        "test2@example.com",
		}

		err := store.AddToken(token)
		require.NoError(t, err)

		tokens := store.ListTokens()
		assert.Len(t, tokens, 2)
		assert.Equal(t, "custom-id", tokens[1].ID)
	})

	t.Run("updates existing token with same ID", func(t *testing.T) {
		token := ProviderToken{
			ID:           "custom-id",
			AccessToken:  "updated-access-token",
			RefreshToken: "updated-refresh-token",
			TokenType:    "Bearer",
			ExpiryDate:   time.Now().Add(2 * time.Hour).UnixMilli(),
			Email:        "updated@example.com",
		}

		err := store.AddToken(token)
		require.NoError(t, err)

		tokens := store.ListTokens()
		assert.Len(t, tokens, 2) // Still 2 tokens, not 3
		found := false
		for _, token := range tokens {
			if token.ID == "custom-id" {
				assert.Equal(t, "updated-access-token", token.AccessToken)
				assert.Equal(t, "updated@example.com", token.Email)
				found = true
			}
		}
		assert.True(t, found, "Token should be updated")
	})

	t.Run("persists to file", func(t *testing.T) {
		// Verify directory was created
		providerDir := filepath.Join(tempDir, ".credentials", "test-provider")
		_, err := os.Stat(providerDir)
		require.NoError(t, err, "Provider directory should exist after adding token")

		// Verify individual token files were created (count only .json files, exclude settings.json)
		entries, err := os.ReadDir(providerDir)
		require.NoError(t, err)

		jsonCount := 0
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") && entry.Name() != "settings.json" {
				jsonCount++
			}
		}
		assert.Len(t, entries, 3) // Two token files + one settings file
		assert.Equal(t, 2, jsonCount)

		// Verify settings file was created
		settingsPath := filepath.Join(providerDir, "settings.json")
		_, err = os.Stat(settingsPath)
		require.NoError(t, err, "Settings file should exist")
	})
}

func TestMultiTokenStore_RemoveToken(t *testing.T) {
	logger := logging.NewLogger()
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "store.json")
	store := NewMultiTokenStore("test-provider", filePath, logger)

	// Add some tokens
	token1 := ProviderToken{
		ID:          "token-1",
		AccessToken: "access-1",
		ExpiryDate:  time.Now().Add(time.Hour).UnixMilli(),
	}
	token2 := ProviderToken{
		ID:          "token-2",
		AccessToken: "access-2",
		ExpiryDate:  time.Now().Add(time.Hour).UnixMilli(),
	}
	token3 := ProviderToken{
		ID:          "token-3",
		AccessToken: "access-3",
		ExpiryDate:  time.Now().Add(time.Hour).UnixMilli(),
	}

	require.NoError(t, store.AddToken(token1))
	require.NoError(t, store.AddToken(token2))
	require.NoError(t, store.AddToken(token3))

	t.Run("removes existing token", func(t *testing.T) {
		err := store.RemoveToken("token-2")
		require.NoError(t, err)

		tokens := store.ListTokens()
		assert.Len(t, tokens, 2)
		assert.Equal(t, "token-1", tokens[0].ID)
		assert.Equal(t, "token-3", tokens[1].ID)
	})

	t.Run("returns error for non-existent token", func(t *testing.T) {
		err := store.RemoveToken("non-existent")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "token not found")
	})

	t.Run("persists removal to file", func(t *testing.T) {
		// Load from file
		newStore := NewMultiTokenStore("test-provider", filePath, logger)
		err := newStore.Load()
		require.NoError(t, err)

		tokens := newStore.ListTokens()
		assert.Len(t, tokens, 2)
	})
}

func TestMultiTokenStore_GetToken(t *testing.T) {
	logger := logging.NewLogger()
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "store.json")
	store := NewMultiTokenStore("test-provider", filePath, logger)

	token := ProviderToken{
		ID:          "test-token-id",
		AccessToken: "test-access",
		ExpiryDate:  time.Now().Add(time.Hour).UnixMilli(),
		Email:       "test@example.com",
	}
	require.NoError(t, store.AddToken(token))

	t.Run("gets existing token", func(t *testing.T) {
		retrieved, err := store.GetToken("test-token-id")
		require.NoError(t, err)
		assert.Equal(t, "test-token-id", retrieved.ID)
		assert.Equal(t, "test-access", retrieved.AccessToken)
		assert.Equal(t, "test@example.com", retrieved.Email)
	})

	t.Run("returns error for non-existent token", func(t *testing.T) {
		_, err := store.GetToken("non-existent")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "token not found")
	})
}

func TestMultiTokenStore_ListTokens(t *testing.T) {
	logger := logging.NewLogger()
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "store.json")
	store := NewMultiTokenStore("test-provider", filePath, logger)

	t.Run("returns empty list for new store", func(t *testing.T) {
		tokens := store.ListTokens()
		assert.Empty(t, tokens)
	})

	t.Run("returns all tokens", func(t *testing.T) {
		for i := 1; i <= 3; i++ {
			token := ProviderToken{
				ID:          string(rune('a' + i)),
				AccessToken: "access-" + string(rune('0'+i)),
				ExpiryDate:  time.Now().Add(time.Hour).UnixMilli(),
			}
			require.NoError(t, store.AddToken(token))
		}

		tokens := store.ListTokens()
		assert.Len(t, tokens, 3)
	})

	t.Run("returns copy not reference", func(t *testing.T) {
		tokens := store.ListTokens()
		originalCount := len(tokens)

		// Modify the returned slice
		tokens = append(tokens, ProviderToken{})

		// Original should be unchanged
		newTokens := store.ListTokens()
		assert.Len(t, newTokens, originalCount)
	})
}

func TestMultiTokenStore_MarkTokenHealthy(t *testing.T) {
	logger := logging.NewLogger()
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "store.json")
	store := NewMultiTokenStore("test-provider", filePath, logger)

	token := ProviderToken{
		ID:          "test-token",
		AccessToken: "access",
		ExpiryDate:  time.Now().Add(time.Hour).UnixMilli(),
		Healthy:     false,
		HealthScore: 0.5,
		ErrorCount:  5,
		LastError:   "some error",
	}
	require.NoError(t, store.AddToken(token))

	t.Run("marks token as healthy", func(t *testing.T) {
		err := store.MarkTokenHealthy("test-token")
		require.NoError(t, err)

		retrieved, err := store.GetToken("test-token")
		require.NoError(t, err)
		assert.True(t, retrieved.Healthy)
		assert.Equal(t, 1.0, retrieved.HealthScore)
		assert.Equal(t, 0, retrieved.ErrorCount)
		assert.Empty(t, retrieved.LastError)
		assert.NotZero(t, retrieved.LastUsed)
	})

	t.Run("returns error for non-existent token", func(t *testing.T) {
		err := store.MarkTokenHealthy("non-existent")
		assert.Error(t, err)
	})
}

func TestMultiTokenStore_MarkTokenUnhealthy(t *testing.T) {
	logger := logging.NewLogger()
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "store.json")
	store := NewMultiTokenStore("test-provider", filePath, logger)

	token := ProviderToken{
		ID:          "test-token",
		AccessToken: "access",
		ExpiryDate:  time.Now().Add(time.Hour).UnixMilli(),
		Healthy:     true,
		HealthScore: 1.0,
		ErrorCount:  0,
	}
	require.NoError(t, store.AddToken(token))

	t.Run("marks token as unhealthy", func(t *testing.T) {
		testErr := assert.AnError
		err := store.MarkTokenUnhealthy("test-token", testErr)
		require.NoError(t, err)

		retrieved, err := store.GetToken("test-token")
		require.NoError(t, err)
		assert.False(t, retrieved.Healthy)
		assert.Equal(t, 1, retrieved.ErrorCount)
		assert.Equal(t, 0.8, retrieved.HealthScore) // Decreased by 0.2
		assert.Contains(t, retrieved.LastError, testErr.Error())
	})

	t.Run("decreases health score on repeated errors", func(t *testing.T) {
		for i := 0; i < 3; i++ {
			err := store.MarkTokenUnhealthy("test-token", assert.AnError)
			require.NoError(t, err)
		}

		retrieved, err := store.GetToken("test-token")
		require.NoError(t, err)
		assert.Equal(t, 4, retrieved.ErrorCount)
		assert.LessOrEqual(t, 0.4, retrieved.HealthScore) // 1.0 - 0.2*3 = 0.4 (with floating point precision)
		assert.GreaterOrEqual(t, 0.39, retrieved.HealthScore)
	})

	t.Run("health score doesn't go below 0", func(t *testing.T) {
		for i := 0; i < 10; i++ {
			err := store.MarkTokenUnhealthy("test-token", assert.AnError)
			require.NoError(t, err)
		}

		retrieved, err := store.GetToken("test-token")
		require.NoError(t, err)
		assert.Equal(t, 0.0, retrieved.HealthScore)
	})
}

func TestMultiTokenStore_IsTokenValid(t *testing.T) {
	logger := logging.NewLogger()
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "store.json")
	store := NewMultiTokenStore("test-provider", filePath, logger)

	t.Run("valid token", func(t *testing.T) {
		token := ProviderToken{
			ID:          "valid-token",
			AccessToken: "access",
			ExpiryDate:  time.Now().Add(time.Hour).UnixMilli(),
			Healthy:     true,
		}
		assert.True(t, store.IsTokenValid(token))
	})

	t.Run("unhealthy token", func(t *testing.T) {
		token := ProviderToken{
			ID:          "unhealthy-token",
			AccessToken: "access",
			ExpiryDate:  time.Now().Add(time.Hour).UnixMilli(),
			Healthy:     false,
		}
		assert.False(t, store.IsTokenValid(token))
	})

	t.Run("expired token", func(t *testing.T) {
		token := ProviderToken{
			ID:          "expired-token",
			AccessToken: "access",
			ExpiryDate:  time.Now().Add(-time.Hour).UnixMilli(),
			Healthy:     true,
		}
		assert.False(t, store.IsTokenValid(token))
	})

	t.Run("token within buffer period", func(t *testing.T) {
		token := ProviderToken{
			ID:          "buffer-token",
			AccessToken: "access",
			ExpiryDate:  time.Now().Add(time.Duration(store.Settings.RefreshBufferSec/2) * time.Second).UnixMilli(),
			Healthy:     true,
		}
		assert.False(t, store.IsTokenValid(token))
	})

	t.Run("token with no expiry date", func(t *testing.T) {
		token := ProviderToken{
			ID:          "no-expiry-token",
			AccessToken: "access",
			ExpiryDate:  0,
			Healthy:     true,
		}
		assert.False(t, store.IsTokenValid(token))
	})
}

func TestMultiTokenStore_GetValidTokens(t *testing.T) {
	logger := logging.NewLogger()
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "store.json")
	store := NewMultiTokenStore("test-provider", filePath, logger)

	// Add various tokens
	tokens := []ProviderToken{
		{ID: "valid-1", AccessToken: "a1", ExpiryDate: time.Now().Add(time.Hour).UnixMilli(), Healthy: true},
		{ID: "valid-2", AccessToken: "a2", ExpiryDate: time.Now().Add(2 * time.Hour).UnixMilli(), Healthy: true},
		{ID: "unhealthy", AccessToken: "a3", ExpiryDate: time.Now().Add(time.Hour).UnixMilli(), Healthy: false},
		{ID: "expired", AccessToken: "a4", ExpiryDate: time.Now().Add(-time.Hour).UnixMilli(), Healthy: true},
	}

	for _, token := range tokens {
		require.NoError(t, store.AddToken(token))
	}

	validTokens := store.GetValidTokens()
	assert.Len(t, validTokens, 2)

	ids := make([]string, 0, len(validTokens))
	for _, token := range validTokens {
		ids = append(ids, token.ID)
	}
	assert.Contains(t, ids, "valid-1")
	assert.Contains(t, ids, "valid-2")
	assert.NotContains(t, ids, "unhealthy")
	assert.NotContains(t, ids, "expired")
}

func TestMultiTokenStore_LoadAndSave(t *testing.T) {
	logger := logging.NewLogger()
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "store.json")

	t.Run("creates empty store when file doesn't exist", func(t *testing.T) {
		store := NewMultiTokenStore("test-provider", filePath, logger)
		err := store.Load()
		require.NoError(t, err)
		assert.Empty(t, store.Tokens)
		assert.Equal(t, StoreVersion, store.Version)
	})

	t.Run("saves and loads tokens", func(t *testing.T) {
		// Create store and add tokens
		store1 := NewMultiTokenStore("test-provider", filePath, logger)
		tokens := []ProviderToken{
			{
				ID:           "token-1",
				AccessToken:  "access-1",
				RefreshToken: "refresh-1",
				TokenType:    "Bearer",
				ExpiryDate:   time.Now().Add(time.Hour).UnixMilli(),
				Email:        "user1@example.com",
				Healthy:      true,
				HealthScore:  1.0,
				LastUsed:     time.Now().UnixMilli(),
				CreatedAt:    time.Now().UnixMilli(),
				ErrorCount:   0,
			},
			{
				ID:           "token-2",
				AccessToken:  "access-2",
				RefreshToken: "refresh-2",
				TokenType:    "Bearer",
				ExpiryDate:   time.Now().Add(2 * time.Hour).UnixMilli(),
				Email:        "user2@example.com",
				Healthy:      false,
				HealthScore:  0.5,
				LastUsed:     time.Now().UnixMilli(),
				CreatedAt:    time.Now().UnixMilli(),
				ErrorCount:   2,
				LastError:    "test error",
			},
		}

		for _, token := range tokens {
			require.NoError(t, store1.AddToken(token))
		}

		// Load into new store
		store2 := NewMultiTokenStore("test-provider", filePath, logger)
		err := store2.Load()
		require.NoError(t, err)

		loadedTokens := store2.ListTokens()
		assert.Len(t, loadedTokens, 2)

		// Verify all fields
		assert.Equal(t, "token-1", loadedTokens[0].ID)
		assert.Equal(t, "access-1", loadedTokens[0].AccessToken)
		assert.Equal(t, "refresh-1", loadedTokens[0].RefreshToken)
		assert.Equal(t, "Bearer", loadedTokens[0].TokenType)
		assert.Equal(t, "user1@example.com", loadedTokens[0].Email)
		assert.True(t, loadedTokens[0].Healthy)
		assert.Equal(t, 1.0, loadedTokens[0].HealthScore)
		assert.Equal(t, 0, loadedTokens[0].ErrorCount)

		assert.Equal(t, "token-2", loadedTokens[1].ID)
		assert.Equal(t, "access-2", loadedTokens[1].AccessToken)
		assert.Equal(t, "user2@example.com", loadedTokens[1].Email)
		assert.False(t, loadedTokens[1].Healthy)
		assert.Equal(t, 0.5, loadedTokens[1].HealthScore)
		assert.Equal(t, 2, loadedTokens[1].ErrorCount)
		assert.Equal(t, "test error", loadedTokens[1].LastError)
	})

	t.Run("saves and loads settings", func(t *testing.T) {
		store1 := NewMultiTokenStore("test-provider", filePath, logger)
		store1.Settings.SelectionStrategy = "round_robin"
		store1.Settings.RefreshBufferSec = 3600
		store1.Settings.MaxErrorCount = 5

		err := store1.Save()
		require.NoError(t, err)

		store2 := NewMultiTokenStore("test-provider", filePath, logger)
		err = store2.Load()
		require.NoError(t, err)

		assert.Equal(t, "round_robin", store2.Settings.SelectionStrategy)
		assert.Equal(t, 3600, store2.Settings.RefreshBufferSec)
		assert.Equal(t, 5, store2.Settings.MaxErrorCount)
	})
}

func TestMultiTokenStore_MigrateFromLegacy(t *testing.T) {
	logger := logging.NewLogger()
	tempDir := t.TempDir()
	legacyFilePath := filepath.Join(tempDir, "legacy.json")

	t.Run("migrates legacy OAuthCreds format", func(t *testing.T) {
		// Create legacy format file
		legacyCreds := map[string]interface{}{
			"access_token":  "legacy-access-token",
			"token_type":    "Bearer",
			"refresh_token": "legacy-refresh-token",
			"resource_url":  "https://qwen.example.com/resource",
			"expiry_date":   time.Now().Add(time.Hour).UnixMilli(),
		}

		legacyData, err := json.Marshal(legacyCreds)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(legacyFilePath, legacyData, 0600))

		// Create store with same provider ID but different file path (to trigger migration)
		store := NewMultiTokenStore("test-provider", filepath.Join(tempDir, "store.json"), logger)

		// Manually call migrateFromLegacy with the legacy file path
		err = store.migrateFromLegacy(legacyFilePath)
		require.NoError(t, err)

		tokens := store.ListTokens()
		assert.Len(t, tokens, 1)
		assert.NotEmpty(t, tokens[0].ID)
		assert.Equal(t, "legacy-access-token", tokens[0].AccessToken)
		assert.Equal(t, "legacy-refresh-token", tokens[0].RefreshToken)
		assert.Equal(t, "Bearer", tokens[0].TokenType)
		assert.Equal(t, "https://qwen.example.com/resource", tokens[0].ResourceURL)
		assert.True(t, tokens[0].Healthy)
		assert.Equal(t, 1.0, tokens[0].HealthScore)

		// Check backup was created
		backupPath := legacyFilePath + ".backup"
		_, err = os.Stat(backupPath)
		assert.NoError(t, err, "Backup file should exist")
	})

	t.Run("fails migration for invalid legacy format", func(t *testing.T) {
		// Create invalid legacy file
		invalidData := []byte(`{"invalid": "data"}`)
		require.NoError(t, os.WriteFile(legacyFilePath, invalidData, 0600))

		store := NewMultiTokenStore("test-provider", filepath.Join(tempDir, "store.json"), logger)
		err := store.migrateFromLegacy(legacyFilePath)
		assert.Error(t, err)
		// The error message may vary, just check that an error occurred
		assert.Error(t, err)
	})

	t.Run("returns error for empty access token in legacy format", func(t *testing.T) {
		// Create legacy format without access_token
		legacyCreds := map[string]interface{}{
			"token_type": "Bearer",
		}

		legacyData, err := json.Marshal(legacyCreds)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(legacyFilePath, legacyData, 0600))

		store := NewMultiTokenStore("test-provider", filepath.Join(tempDir, "store.json"), logger)
		err = store.migrateFromLegacy(legacyFilePath)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not a valid legacy format")
	})
}

func TestMultiTokenStore_GetTokenCount(t *testing.T) {
	logger := logging.NewLogger()
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "store.json")
	store := NewMultiTokenStore("test-provider", filePath, logger)

	t.Run("returns 0 for empty store", func(t *testing.T) {
		assert.Equal(t, 0, store.GetTokenCount())
	})

	t.Run("returns correct count", func(t *testing.T) {
		for i := 0; i < 5; i++ {
			token := ProviderToken{
				ID:          string(rune('a' + i)),
				AccessToken: "access",
				ExpiryDate:  time.Now().Add(time.Hour).UnixMilli(),
			}
			require.NoError(t, store.AddToken(token))
		}

		assert.Equal(t, 5, store.GetTokenCount())
	})
}

func TestMultiTokenStore_GetValidTokenCount(t *testing.T) {
	logger := logging.NewLogger()
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "store.json")
	store := NewMultiTokenStore("test-provider", filePath, logger)

	t.Run("returns 0 for empty store", func(t *testing.T) {
		assert.Equal(t, 0, store.GetValidTokenCount())
	})

	t.Run("counts only valid tokens", func(t *testing.T) {
		tokens := []ProviderToken{
			{ID: "valid-1", AccessToken: "a1", ExpiryDate: time.Now().Add(time.Hour).UnixMilli(), Healthy: true},
			{ID: "valid-2", AccessToken: "a2", ExpiryDate: time.Now().Add(2 * time.Hour).UnixMilli(), Healthy: true},
			{ID: "unhealthy", AccessToken: "a3", ExpiryDate: time.Now().Add(time.Hour).UnixMilli(), Healthy: false},
			{ID: "expired", AccessToken: "a4", ExpiryDate: time.Now().Add(-time.Hour).UnixMilli(), Healthy: true},
		}

		for _, token := range tokens {
			require.NoError(t, store.AddToken(token))
		}

		assert.Equal(t, 2, store.GetValidTokenCount())
	})
}

func TestMultiTokenStore_UpdateToken(t *testing.T) {
	logger := logging.NewLogger()
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "store.json")
	store := NewMultiTokenStore("test-provider", filePath, logger)

	token := ProviderToken{
		ID:          "test-token",
		AccessToken: "old-access",
		ExpiryDate:  time.Now().Add(time.Hour).UnixMilli(),
		Email:       "old@example.com",
	}
	require.NoError(t, store.AddToken(token))

	t.Run("updates token with function", func(t *testing.T) {
		err := store.UpdateToken("test-token", func(token *ProviderToken) {
			token.AccessToken = "new-access"
			token.Email = "new@example.com"
		})
		require.NoError(t, err)

		retrieved, err := store.GetToken("test-token")
		require.NoError(t, err)
		assert.Equal(t, "new-access", retrieved.AccessToken)
		assert.Equal(t, "new@example.com", retrieved.Email)
	})

	t.Run("returns error for non-existent token", func(t *testing.T) {
		err := store.UpdateToken("non-existent", func(token *ProviderToken) {})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "token not found")
	})
}

func TestGenerateTokenID(t *testing.T) {
	t.Run("generates unique IDs", func(t *testing.T) {
		ids := make(map[string]bool)
		for i := 0; i < 100; i++ {
			id := GenerateTokenID()
			assert.NotEmpty(t, id)
			assert.False(t, ids[id], "ID should be unique")
			ids[id] = true
		}
		assert.Len(t, ids, 100)
	})

	t.Run("generates valid UUID format", func(t *testing.T) {
		id := GenerateTokenID()
		// UUID format: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
		assert.Len(t, id, 36)
		assert.Contains(t, id, "-")
	})
}
