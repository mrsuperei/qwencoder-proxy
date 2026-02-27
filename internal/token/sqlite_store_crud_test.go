package token

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/sunbankio/qwencoder-proxy/logging"
)

func TestSQLiteStore_Load_Empty(t *testing.T) {
	logger := logging.NewLogger()
	store, err := NewSQLiteStore(":memory:", "test-provider", logger)
	require.NoError(t, err)
	defer store.Close()

	tokens, err := store.Load()
	require.NoError(t, err)
	assert.Empty(t, tokens)
}

func TestSQLiteStore_AddToken(t *testing.T) {
	logger := logging.NewLogger()
	store, err := NewSQLiteStore(":memory:", "test-provider", logger)
	require.NoError(t, err)
	defer store.Close()

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

	err = store.AddToken(token)
	require.NoError(t, err)

	// Verify token was added
	retrieved, err := store.GetToken("test-id-1")
	require.NoError(t, err)
	assert.Equal(t, token.ID, retrieved.ID)
	assert.Equal(t, token.Email, retrieved.Email)
	assert.Equal(t, token.AccessToken, retrieved.AccessToken)
	assert.Equal(t, token.Healthy, retrieved.Healthy)
}

func TestSQLiteStore_AddToken_DuplicateRefreshToken(t *testing.T) {
	logger := logging.NewLogger()
	store, err := NewSQLiteStore(":memory:", "test-provider", logger)
	require.NoError(t, err)
	defer store.Close()

	// Add first token
	token1 := TokenMetadata{
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

	err = store.AddToken(token1)
	require.NoError(t, err)

	// Add another token with the same refresh_token (simulating token refresh)
	token2 := TokenMetadata{
		ID:           "test-id-2",
		AccessToken:  "test-access-token-2",
		RefreshToken: "test-refresh-token", // Same refresh token
		TokenType:    "Bearer",
		ExpiryDate:   time.Now().Add(2 * time.Hour).UnixMilli(),
		Email:        "test@example.com",
		Healthy:      true,
		HealthScore:  1.0,
		LastUsed:     time.Now().UnixMilli(),
		CreatedAt:    time.Now().UnixMilli(),
		ErrorCount:   0,
	}

	err = store.AddToken(token2)
	require.NoError(t, err)

	// Verify the first token was updated with the new access token from token2
	retrieved, err := store.GetToken("test-id-1")
	require.NoError(t, err)
	assert.Equal(t, token2.AccessToken, retrieved.AccessToken, "existing token should be updated with new access token")
	assert.Equal(t, token2.ExpiryDate, retrieved.ExpiryDate, "existing token should be updated with new expiry date")

	// Verify test-id-2 was NOT created (duplicate refresh_token updates existing)
	_, err = store.GetToken("test-id-2")
	assert.Error(t, err, "test-id-2 should not exist as it was a duplicate refresh_token")
}

func TestSQLiteStore_GetToken_NotFound(t *testing.T) {
	logger := logging.NewLogger()
	store, err := NewSQLiteStore(":memory:", "test-provider", logger)
	require.NoError(t, err)
	defer store.Close()

	_, err = store.GetToken("non-existent-id")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestSQLiteStore_UpdateToken(t *testing.T) {
	logger := logging.NewLogger()
	store, err := NewSQLiteStore(":memory:", "test-provider", logger)
	require.NoError(t, err)
	defer store.Close()

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

	err = store.AddToken(token)
	require.NoError(t, err)

	// Update token
	err = store.UpdateToken("test-id-1", func(t *TokenMetadata) {
		t.Healthy = false
		t.ErrorCount = 1
		t.LastError = "test error"
	})
	require.NoError(t, err)

	// Verify update
	retrieved, err := store.GetToken("test-id-1")
	require.NoError(t, err)
	assert.False(t, retrieved.Healthy)
	assert.Equal(t, 1, retrieved.ErrorCount)
	assert.Equal(t, "test error", retrieved.LastError)
}

func TestSQLiteStore_RemoveToken(t *testing.T) {
	logger := logging.NewLogger()
	store, err := NewSQLiteStore(":memory:", "test-provider", logger)
	require.NoError(t, err)
	defer store.Close()

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

	err = store.AddToken(token)
	require.NoError(t, err)

	// Remove token
	err = store.RemoveToken("test-id-1")
	require.NoError(t, err)

	// Verify token was removed
	_, err = store.GetToken("test-id-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestSQLiteStore_RemoveToken_NotFound(t *testing.T) {
	logger := logging.NewLogger()
	store, err := NewSQLiteStore(":memory:", "test-provider", logger)
	require.NoError(t, err)
	defer store.Close()

	err = store.RemoveToken("non-existent-id")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestSQLiteStore_Save(t *testing.T) {
	logger := logging.NewLogger()
	store, err := NewSQLiteStore(":memory:", "test-provider", logger)
	require.NoError(t, err)
	defer store.Close()

	tokens := map[string]TokenMetadata{
		"test-id-1": {
			ID:           "test-id-1",
			AccessToken:  "test-access-token-1",
			RefreshToken: "test-refresh-token-1",
			TokenType:    "Bearer",
			ExpiryDate:   time.Now().Add(time.Hour).UnixMilli(),
			Email:        "test1@example.com",
			Healthy:      true,
			HealthScore:  1.0,
			LastUsed:     time.Now().UnixMilli(),
			CreatedAt:    time.Now().UnixMilli(),
			ErrorCount:   0,
		},
		"test-id-2": {
			ID:           "test-id-2",
			AccessToken:  "test-access-token-2",
			RefreshToken: "test-refresh-token-2",
			TokenType:    "Bearer",
			ExpiryDate:   time.Now().Add(2 * time.Hour).UnixMilli(),
			Email:        "test2@example.com",
			Healthy:      true,
			HealthScore:  1.0,
			LastUsed:     time.Now().UnixMilli(),
			CreatedAt:    time.Now().UnixMilli(),
			ErrorCount:   0,
		},
	}

	err = store.Save(tokens)
	require.NoError(t, err)

	// Verify tokens were saved
	loaded, err := store.Load()
	require.NoError(t, err)
	assert.Len(t, loaded, 2)
	assert.Contains(t, loaded, "test-id-1")
	assert.Contains(t, loaded, "test-id-2")
}

func TestSQLiteStore_GetValidTokens(t *testing.T) {
	logger := logging.NewLogger()
	store, err := NewSQLiteStore(":memory:", "test-provider", logger)
	require.NoError(t, err)
	defer store.Close()

	now := time.Now().UnixMilli()

	// Add healthy token
	healthyToken := TokenMetadata{
		ID:           "test-id-1",
		AccessToken:  "test-access-token",
		RefreshToken: "test-refresh-token-1",
		TokenType:    "Bearer",
		ExpiryDate:   now + 3600000, // 1 hour from now
		Email:        "test@example.com",
		Healthy:      true,
		HealthScore:  1.0,
		LastUsed:     now,
		CreatedAt:    now,
		ErrorCount:   0,
	}
	err = store.AddToken(healthyToken)
	require.NoError(t, err)

	// Add unhealthy token
	unhealthyToken := TokenMetadata{
		ID:           "test-id-2",
		AccessToken:  "test-access-token-2",
		RefreshToken: "test-refresh-token-2",
		TokenType:    "Bearer",
		ExpiryDate:   now + 3600000,
		Email:        "test2@example.com",
		Healthy:      false,
		HealthScore:  0.0,
		LastUsed:     now,
		CreatedAt:    now,
		ErrorCount:   0,
	}
	err = store.AddToken(unhealthyToken)
	require.NoError(t, err)

	// Add expired token
	expiredToken := TokenMetadata{
		ID:           "test-id-3",
		AccessToken:  "test-access-token-3",
		RefreshToken: "test-refresh-token-3",
		TokenType:    "Bearer",
		ExpiryDate:   now - 3600000, // 1 hour ago
		Email:        "test3@example.com",
		Healthy:      true,
		HealthScore:  1.0,
		LastUsed:     now,
		CreatedAt:    now,
		ErrorCount:   0,
	}
	err = store.AddToken(expiredToken)
	require.NoError(t, err)

	// Get valid tokens (should only return healthy token)
	validTokens := store.GetValidTokens()
	assert.Len(t, validTokens, 1)
	assert.Equal(t, "test-id-1", validTokens[0].ID)
}

func TestSQLiteStore_GetSettings(t *testing.T) {
	logger := logging.NewLogger()
	store, err := NewSQLiteStore(":memory:", "test-provider", logger)
	require.NoError(t, err)
	defer store.Close()

	// Get default settings
	settings, err := store.GetSettings()
	require.NoError(t, err)
	assert.Equal(t, "random", settings.SelectionStrategy)
	assert.Equal(t, 1800, settings.RefreshBufferSec)
	assert.Equal(t, 3, settings.MaxErrorCount)

	// Save custom settings
	customSettings := StoreSettings{
		SelectionStrategy: "round_robin",
		RefreshBufferSec:  3600,
		MaxErrorCount:     5,
		UpdatedAt:         time.Now().UnixMilli(),
	}
	err = store.SaveSettings(customSettings)
	require.NoError(t, err)

	// Get custom settings
	settings, err = store.GetSettings()
	require.NoError(t, err)
	assert.Equal(t, "round_robin", settings.SelectionStrategy)
	assert.Equal(t, 3600, settings.RefreshBufferSec)
	assert.Equal(t, 5, settings.MaxErrorCount)
}

func TestSQLiteStore_Clear(t *testing.T) {
	logger := logging.NewLogger()
	store, err := NewSQLiteStore(":memory:", "test-provider", logger)
	require.NoError(t, err)
	defer store.Close()

	// Add tokens
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
	err = store.AddToken(token)
	require.NoError(t, err)

	// Clear tokens
	err = store.Clear()
	require.NoError(t, err)

	// Verify tokens were cleared
	tokens, err := store.Load()
	require.NoError(t, err)
	assert.Empty(t, tokens)
}

func TestSQLiteStore_ConcurrentAccess(t *testing.T) {
	logger := logging.NewLogger()
	store, err := NewSQLiteStore(":memory:", "test-provider", logger)
	require.NoError(t, err)
	defer store.Close()

	const numGoroutines = 10
	const numTokensPerGoroutine = 10

	done := make(chan error, numGoroutines)

	// Concurrently add tokens
	for i := 0; i < numGoroutines; i++ {
		go func(goroutineID int) {
			for j := 0; j < numTokensPerGoroutine; j++ {
				token := TokenMetadata{
					ID:           fmt.Sprintf("token-%d-%d", goroutineID, j),
					AccessToken:  fmt.Sprintf("access-%d-%d", goroutineID, j),
					RefreshToken: fmt.Sprintf("refresh-%d-%d", goroutineID, j),
					TokenType:    "Bearer",
					ExpiryDate:   time.Now().Add(time.Hour).UnixMilli(),
					Email:        fmt.Sprintf("user%d@example.com", goroutineID),
					Healthy:      true,
					HealthScore:  1.0,
					LastUsed:     time.Now().UnixMilli(),
					CreatedAt:    time.Now().UnixMilli(),
					ErrorCount:   0,
				}
				if err := store.AddToken(token); err != nil {
					done <- err
					return
				}
			}
			done <- nil
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < numGoroutines; i++ {
		err := <-done
		require.NoError(t, err)
	}

	// Verify all tokens were added
	tokens, err := store.Load()
	require.NoError(t, err)
	assert.Equal(t, numGoroutines*numTokensPerGoroutine, len(tokens))
}

func TestSQLiteStore_UpdateToken_WithFunc(t *testing.T) {
	logger := logging.NewLogger()
	store, err := NewSQLiteStore(":memory:", "test-provider", logger)
	require.NoError(t, err)
	defer store.Close()

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

	err = store.AddToken(token)
	require.NoError(t, err)

	// Update token with function
	err = store.UpdateToken("test-id-1", func(t *TokenMetadata) {
		t.AccessToken = "new-access-token"
		t.ExpiryDate = time.Now().Add(2 * time.Hour).UnixMilli()
		t.HealthScore = 0.5
	})
	require.NoError(t, err)

	// Verify update
	retrieved, err := store.GetToken("test-id-1")
	require.NoError(t, err)
	assert.Equal(t, "new-access-token", retrieved.AccessToken)
	assert.Equal(t, 0.5, retrieved.HealthScore)
}

func TestSQLiteStore_GetToken_WithLastError(t *testing.T) {
	logger := logging.NewLogger()
	store, err := NewSQLiteStore(":memory:", "test-provider", logger)
	require.NoError(t, err)
	defer store.Close()

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
		LastError:    "test error",
	}

	err = store.AddToken(token)
	require.NoError(t, err)

	// Retrieve token
	retrieved, err := store.GetToken("test-id-1")
	require.NoError(t, err)
	assert.Equal(t, "test error", retrieved.LastError)
}

func TestSQLiteStore_SaveSettings_Defaults(t *testing.T) {
	logger := logging.NewLogger()
	store, err := NewSQLiteStore(":memory:", "test-provider", logger)
	require.NoError(t, err)
	defer store.Close()

	// Save settings with empty values (should use defaults)
	emptySettings := StoreSettings{}
	err = store.SaveSettings(emptySettings)
	require.NoError(t, err)

	// Verify defaults were applied
	settings, err := store.GetSettings()
	require.NoError(t, err)
	assert.Equal(t, "random", settings.SelectionStrategy)
	assert.Equal(t, 1800, settings.RefreshBufferSec)
	assert.Equal(t, 3, settings.MaxErrorCount)
}

func TestSQLiteStore_GetValidTokens_WithRefreshBuffer(t *testing.T) {
	logger := logging.NewLogger()
	store, err := NewSQLiteStore(":memory:", "test-provider", logger)
	require.NoError(t, err)
	defer store.Close()

	now := time.Now().UnixMilli()

	// Set custom refresh buffer
	settings := StoreSettings{
		SelectionStrategy: "random",
		RefreshBufferSec:  600, // 10 minutes buffer
		MaxErrorCount:     3,
		UpdatedAt:         time.Now().UnixMilli(),
	}
	err = store.SaveSettings(settings)
	require.NoError(t, err)

	// Add token expiring in 5 minutes
	token := TokenMetadata{
		ID:           "test-id-1",
		AccessToken:  "test-access-token",
		RefreshToken: "test-refresh-token",
		TokenType:    "Bearer",
		ExpiryDate:   now + 300000, // 5 minutes from now
		Email:        "test@example.com",
		Healthy:      true,
		HealthScore:  1.0,
		LastUsed:     now,
		CreatedAt:    now,
		ErrorCount:   0,
	}
	err = store.AddToken(token)
	require.NoError(t, err)

	// Get valid tokens (should return token since it's within buffer)
	validTokens := store.GetValidTokens()
	assert.Len(t, validTokens, 1)
	assert.Equal(t, "test-id-1", validTokens[0].ID)
}

func TestSQLiteStore_ScanToken_WithNullValues(t *testing.T) {
	logger := logging.NewLogger()
	store, err := NewSQLiteStore(":memory:", "test-provider", logger)
	require.NoError(t, err)
	defer store.Close()

	// Insert token with null values
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
		// LastError is empty
	}

	err = store.AddToken(token)
	require.NoError(t, err)

	// Retrieve and verify
	retrieved, err := store.GetToken("test-id-1")
	require.NoError(t, err)
	assert.Equal(t, "", retrieved.LastError)
}

func TestSQLiteStore_BindToken_WithNullValues(t *testing.T) {
	logger := logging.NewLogger()
	store, err := NewSQLiteStore(":memory:", "test-provider", logger)
	require.NoError(t, err)
	defer store.Close()

	// Insert token with null values
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
		// LastError is empty
	}

	err = store.AddToken(token)
	require.NoError(t, err)

	// Verify by loading all tokens
	tokens, err := store.Load()
	require.NoError(t, err)
	assert.Len(t, tokens, 1)
	assert.Equal(t, "", tokens["test-id-1"].LastError)
}
