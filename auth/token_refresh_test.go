package auth

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunbankio/qwencoder-proxy/logging"
)

// MockRefresher is a mock implementation of ProviderRefresh for testing
type MockRefresher struct {
	providerID  string
	shouldFail  bool
	failCount   int
	refreshFunc func(ctx context.Context, token ProviderToken) (ProviderToken, error)
}

func NewMockRefresher(providerID string) *MockRefresher {
	return &MockRefresher{
		providerID: providerID,
	}
}

func (m *MockRefresher) RefreshToken(ctx context.Context, token ProviderToken) (ProviderToken, error) {
	if m.refreshFunc != nil {
		return m.refreshFunc(ctx, token)
	}

	if m.shouldFail {
		m.failCount++
		return ProviderToken{}, fmt.Errorf("mock refresh failed")
	}

	// Return a refreshed token with updated expiry
	newToken := token
	newToken.AccessToken = "refreshed-" + token.AccessToken
	newToken.ExpiryDate = time.Now().Add(time.Hour).UnixMilli()
	newToken.HealthScore = 1.0
	newToken.ErrorCount = 0
	newToken.LastError = ""

	return newToken, nil
}

func (m *MockRefresher) ProviderID() string {
	return m.providerID
}

func (m *MockRefresher) SetShouldFail(shouldFail bool) {
	m.shouldFail = shouldFail
}

func (m *MockRefresher) SetRefreshFunc(fn func(ctx context.Context, token ProviderToken) (ProviderToken, error)) {
	m.refreshFunc = fn
}

func createTestToken(id string) ProviderToken {
	return ProviderToken{
		ID:           id,
		AccessToken:  "access-" + id,
		RefreshToken: "refresh-" + id,
		TokenType:    "Bearer",
		ExpiryDate:   time.Now().Add(time.Hour).UnixMilli(),
		Email:        "test@example.com",
		Healthy:      true,
		HealthScore:  1.0,
		LastUsed:     time.Now().UnixMilli(),
		CreatedAt:    time.Now().UnixMilli(),
		ErrorCount:   0,
	}
}

func TestCalculateRefreshPriority(t *testing.T) {
	now := time.Now()

	t.Run("critical priority - already expired", func(t *testing.T) {
		token := ProviderToken{
			ExpiryDate: now.Add(-time.Minute).UnixMilli(),
		}
		priority := calculateRefreshPriority(token, 1800)
		assert.Equal(t, 0, priority)
	})

	t.Run("critical priority - within 5 minutes of buffer", func(t *testing.T) {
		token := ProviderToken{
			ExpiryDate: now.Add(25 * time.Minute).UnixMilli(), // 30 min buffer - 5 min
		}
		priority := calculateRefreshPriority(token, 1800)
		assert.Equal(t, 0, priority)
	})

	t.Run("high priority - within 15 minutes of buffer", func(t *testing.T) {
		token := ProviderToken{
			ExpiryDate: now.Add(20 * time.Minute).UnixMilli(), // 30 min buffer - 10 min
		}
		priority := calculateRefreshPriority(token, 1800)
		assert.Equal(t, 10, priority)
	})

	t.Run("medium priority - within 30 minutes of buffer", func(t *testing.T) {
		token := ProviderToken{
			ExpiryDate: now.Add(5 * time.Minute).UnixMilli(), // 30 min buffer - 25 min
		}
		priority := calculateRefreshPriority(token, 1800)
		assert.Equal(t, 20, priority)
	})

	t.Run("low priority - more than 30 minutes before buffer", func(t *testing.T) {
		token := ProviderToken{
			ExpiryDate: now.Add(2 * time.Hour).UnixMilli(),
		}
		priority := calculateRefreshPriority(token, 1800)
		assert.Equal(t, 30, priority)
	})
}

func TestIsTokenExpiringSoon(t *testing.T) {
	now := time.Now()

	t.Run("token is expiring soon", func(t *testing.T) {
		token := ProviderToken{
			ExpiryDate: now.Add(20 * time.Minute).UnixMilli(),
		}
		assert.True(t, isTokenExpiringSoon(token, 1800))
	})

	t.Run("token is not expiring soon", func(t *testing.T) {
		token := ProviderToken{
			ExpiryDate: now.Add(2 * time.Hour).UnixMilli(),
		}
		assert.False(t, isTokenExpiringSoon(token, 1800))
	})

	t.Run("token with no expiry date", func(t *testing.T) {
		token := ProviderToken{
			ExpiryDate: 0,
		}
		assert.False(t, isTokenExpiringSoon(token, 1800))
	})

	t.Run("expired token", func(t *testing.T) {
		token := ProviderToken{
			ExpiryDate: now.Add(-time.Hour).UnixMilli(),
		}
		assert.True(t, isTokenExpiringSoon(token, 1800))
	})
}

func TestGetExpiryTimeRemaining(t *testing.T) {
	now := time.Now()

	t.Run("returns remaining time", func(t *testing.T) {
		token := ProviderToken{
			ExpiryDate: now.Add(time.Hour).UnixMilli(),
		}
		remaining := getExpiryTimeRemaining(token)
		assert.Greater(t, remaining, 50*time.Minute)
		assert.Less(t, remaining, 70*time.Minute)
	})

	t.Run("returns 0 for expired token", func(t *testing.T) {
		token := ProviderToken{
			ExpiryDate: now.Add(-time.Hour).UnixMilli(),
		}
		remaining := getExpiryTimeRemaining(token)
		assert.Equal(t, time.Duration(0), remaining)
	})

	t.Run("returns 0 for token with no expiry", func(t *testing.T) {
		token := ProviderToken{
			ExpiryDate: 0,
		}
		remaining := getExpiryTimeRemaining(token)
		assert.Equal(t, time.Duration(0), remaining)
	})
}

func TestRefreshCoordinator_New(t *testing.T) {
	logger := logging.NewLogger()
	tempDir := t.TempDir()
	filePath := tempDir + "/store.json"

	t.Run("creates coordinator with defaults", func(t *testing.T) {
		store := NewMultiTokenStore("test-provider", filePath, logger)
		coordinator := NewRefreshCoordinator(store, 0, nil, nil)

		assert.Equal(t, 3, coordinator.workers) // Default worker count
		assert.NotNil(t, coordinator.logger)
		assert.NotNil(t, coordinator.ctx)
		assert.NotNil(t, coordinator.cancel)
		assert.NotNil(t, coordinator.refreshers)
	})

	t.Run("creates coordinator with custom workers", func(t *testing.T) {
		store := NewMultiTokenStore("test-provider", filePath, logger)
		coordinator := NewRefreshCoordinator(store, 5, logger, nil)

		assert.Equal(t, 5, coordinator.workers)
	})
}

func TestRefreshCoordinator_RegisterRefresher(t *testing.T) {
	logger := logging.NewLogger()
	tempDir := t.TempDir()
	filePath := tempDir + "/store.json"
	store := NewMultiTokenStore("test-provider", filePath, logger)
	coordinator := NewRefreshCoordinator(store, 3, logger, nil)

	t.Run("registers refresher", func(t *testing.T) {
		refresher := NewMockRefresher("test-provider")
		coordinator.RegisterRefresher(refresher)

		// Verify it was registered (we can't directly access the map, but we can test via RefreshToken)
	})
}

func TestRefreshCoordinator_StartStop(t *testing.T) {
	logger := logging.NewLogger()
	tempDir := t.TempDir()
	filePath := tempDir + "/store.json"
	store := NewMultiTokenStore("test-provider", filePath, logger)
	coordinator := NewRefreshCoordinator(store, 2, logger, nil)

	t.Run("starts and stops workers", func(t *testing.T) {
		err := coordinator.Start()
		require.NoError(t, err)

		// Give workers time to start
		time.Sleep(100 * time.Millisecond)

		status := coordinator.GetWorkerStatus()
		assert.Len(t, status, 2)
		assert.True(t, status[0])
		assert.True(t, status[1])

		// Stop coordinator
		coordinator.Stop()

		// Give workers time to stop
		time.Sleep(200 * time.Millisecond)

		// Workers should be stopped
		status = coordinator.GetWorkerStatus()
		assert.Len(t, status, 2)
		assert.False(t, status[0])
		assert.False(t, status[1])
	})
}

func TestRefreshCoordinator_ScheduleRefresh(t *testing.T) {
	logger := logging.NewLogger()
	tempDir := t.TempDir()
	filePath := tempDir + "/store.json"
	store := NewMultiTokenStore("test-provider", filePath, logger)
	coordinator := NewRefreshCoordinator(store, 2, logger, nil)
	refresher := NewMockRefresher("test-provider")
	coordinator.RegisterRefresher(refresher)

	t.Run("schedules refresh successfully", func(t *testing.T) {
		token := createTestToken("test-token")
		require.NoError(t, store.AddToken(token))

		err := coordinator.ScheduleRefresh("test-token", 10)
		require.NoError(t, err)
		assert.Equal(t, 1, coordinator.GetQueueSize())
	})

	t.Run("returns error for non-existent token", func(t *testing.T) {
		err := coordinator.ScheduleRefresh("non-existent", 10)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get token")
	})
}

func TestRefreshCoordinator_RefreshToken(t *testing.T) {
	logger := logging.NewLogger()
	tempDir := t.TempDir()
	filePath := tempDir + "/store.json"
	store := NewMultiTokenStore("test-provider", filePath, logger)
	coordinator := NewRefreshCoordinator(store, 2, logger, nil)
	refresher := NewMockRefresher("test-provider")
	coordinator.RegisterRefresher(refresher)

	t.Run("refreshes token successfully", func(t *testing.T) {
		token := createTestToken("test-token")
		require.NoError(t, store.AddToken(token))

		ctx := context.Background()
		newToken, err := coordinator.RefreshToken(ctx, "test-token")
		require.NoError(t, err)
		require.NotNil(t, newToken)

		assert.Equal(t, "test-token", newToken.ID)
		assert.Equal(t, "refreshed-access-test-token", newToken.AccessToken)
		assert.Greater(t, newToken.ExpiryDate, token.ExpiryDate)
		assert.True(t, newToken.Healthy)
		assert.Equal(t, 0, newToken.ErrorCount)

		// Verify store was updated
		retrieved, err := store.GetToken("test-token")
		require.NoError(t, err)
		assert.Equal(t, "refreshed-access-test-token", retrieved.AccessToken)
	})

	t.Run("returns error for non-existent token", func(t *testing.T) {
		ctx := context.Background()
		_, err := coordinator.RefreshToken(ctx, "non-existent")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get token")
	})

	t.Run("returns error when no refresher registered", func(t *testing.T) {
		store2 := NewMultiTokenStore("test-provider-2", tempDir+"/store2.json", logger)
		coordinator2 := NewRefreshCoordinator(store2, 2, logger, nil)
		// Don't register a refresher

		token := createTestToken("test-token")
		require.NoError(t, store2.AddToken(token))

		ctx := context.Background()
		_, err := coordinator2.RefreshToken(ctx, "test-token")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no refresher registered")
	})

	t.Run("handles refresh failure", func(t *testing.T) {
		refresher.SetShouldFail(true)

		token := createTestToken("test-token-2")
		require.NoError(t, store.AddToken(token))

		ctx := context.Background()
		_, err := coordinator.RefreshToken(ctx, "test-token-2")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to refresh token")

		// Verify token was marked unhealthy
		retrieved, err := store.GetToken("test-token-2")
		require.NoError(t, err)
		assert.False(t, retrieved.Healthy)
	})
}

func TestRefreshCoordinator_ProcessRequest(t *testing.T) {
	logger := logging.NewLogger()
	tempDir := t.TempDir()
	filePath := tempDir + "/store.json"
	store := NewMultiTokenStore("test-provider", filePath, logger)
	coordinator := NewRefreshCoordinator(store, 2, logger, nil)
	refresher := NewMockRefresher("test-provider")
	coordinator.RegisterRefresher(refresher)

	t.Run("processes successful refresh request", func(t *testing.T) {
		token := createTestToken("test-token")
		require.NoError(t, store.AddToken(token))

		// Start coordinator to process requests
		err := coordinator.Start()
		require.NoError(t, err)
		defer coordinator.Stop()

		// Schedule the request
		err = coordinator.ScheduleRefresh("test-token", 10)
		require.NoError(t, err)

		// Wait for processing
		time.Sleep(200 * time.Millisecond)

		// Verify token was refreshed
		retrieved, err := store.GetToken("test-token")
		require.NoError(t, err)
		assert.Equal(t, "refreshed-access-test-token", retrieved.AccessToken)
	})
}

func TestRefreshScheduler_New(t *testing.T) {
	logger := logging.NewLogger()
	tempDir := t.TempDir()
	filePath := tempDir + "/store.json"
	store := NewMultiTokenStore("test-provider", filePath, logger)
	coordinator := NewRefreshCoordinator(store, 2, logger, nil)

	t.Run("creates scheduler with defaults", func(t *testing.T) {
		scheduler := NewRefreshScheduler(coordinator, store, 0, nil)

		assert.Equal(t, 5*time.Minute, scheduler.checkInterval) // Default
		assert.NotNil(t, scheduler.logger)
	})

	t.Run("creates scheduler with custom interval", func(t *testing.T) {
		scheduler := NewRefreshScheduler(coordinator, store, 2*time.Minute, logger)

		assert.Equal(t, 2*time.Minute, scheduler.checkInterval)
	})
}

func TestRefreshScheduler_StartStop(t *testing.T) {
	logger := logging.NewLogger()
	tempDir := t.TempDir()
	filePath := tempDir + "/store.json"
	store := NewMultiTokenStore("test-provider", filePath, logger)
	coordinator := NewRefreshCoordinator(store, 2, logger, nil)
	scheduler := NewRefreshScheduler(coordinator, store, 100*time.Millisecond, logger)

	t.Run("starts and stops scheduler", func(t *testing.T) {
		scheduler.Start()
		time.Sleep(50 * time.Millisecond) // Let it run a bit
		scheduler.Stop()
	})
}

func TestRefreshScheduler_CheckAndSchedule(t *testing.T) {
	logger := logging.NewLogger()
	tempDir := t.TempDir()
	filePath := tempDir + "/store.json"
	store := NewMultiTokenStore("test-provider", filePath, logger)
	coordinator := NewRefreshCoordinator(store, 2, logger, nil)
	refresher := NewMockRefresher("test-provider")
	coordinator.RegisterRefresher(refresher)

	scheduler := NewRefreshScheduler(coordinator, store, 1*time.Minute, logger)

	t.Run("schedules refresh for expiring tokens", func(t *testing.T) {
		// Add tokens with various expiry times
		now := time.Now()
		tokens := []ProviderToken{
			{ID: "expiring-soon", AccessToken: "a1", ExpiryDate: now.Add(20 * time.Minute).UnixMilli(), Healthy: true},
			{ID: "not-expiring", AccessToken: "a2", ExpiryDate: now.Add(2 * time.Hour).UnixMilli(), Healthy: true},
			{ID: "unhealthy", AccessToken: "a3", ExpiryDate: now.Add(20 * time.Minute).UnixMilli(), Healthy: false},
		}

		for _, token := range tokens {
			require.NoError(t, store.AddToken(token))
		}

		scheduler.CheckAndSchedule()

		// Should have scheduled refresh for expiring-soon only
		assert.Equal(t, 1, coordinator.GetQueueSize())
	})

	t.Run("skips unhealthy tokens", func(t *testing.T) {
		store2 := NewMultiTokenStore("test-provider-2", tempDir+"/store2.json", logger)
		coordinator2 := NewRefreshCoordinator(store2, 2, logger, nil)
		scheduler2 := NewRefreshScheduler(coordinator2, store2, 1*time.Minute, logger)

		now := time.Now()
		token := ProviderToken{
			ID:          "unhealthy-token",
			AccessToken: "a1",
			ExpiryDate:  now.Add(20 * time.Minute).UnixMilli(),
			Healthy:     false,
		}
		require.NoError(t, store2.AddToken(token))

		scheduler2.CheckAndSchedule()

		// Should not schedule for unhealthy token
		assert.Equal(t, 0, coordinator2.GetQueueSize())
	})
}

func TestQwenRefresher(t *testing.T) {
	logger := logging.NewLogger()
	refresher := NewQwenRefresher(nil, logger, nil, nil)

	t.Run("returns provider ID", func(t *testing.T) {
		assert.Equal(t, "qwen", refresher.ProviderID())
	})

	t.Run("returns error for token without refresh token", func(t *testing.T) {
		token := ProviderToken{
			ID:          "test-token",
			AccessToken: "access",
			ExpiryDate:  time.Now().Add(time.Hour).UnixMilli(),
		}

		ctx := context.Background()
		_, err := refresher.RefreshToken(ctx, token)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no refresh token available")
	})
}

func TestGeminiRefresher(t *testing.T) {
	logger := logging.NewLogger()
	config := DefaultGeminiOAuthConfig()
	refresher := NewGeminiRefresher(config, nil, logger, nil, nil)

	t.Run("returns provider ID", func(t *testing.T) {
		assert.Equal(t, "gemini", refresher.ProviderID())
	})

	t.Run("returns error for token without refresh token", func(t *testing.T) {
		token := ProviderToken{
			ID:          "test-token",
			AccessToken: "access",
			ExpiryDate:  time.Now().Add(time.Hour).UnixMilli(),
		}

		ctx := context.Background()
		_, err := refresher.RefreshToken(ctx, token)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no refresh token available")
	})
}

func TestKiroRefresher(t *testing.T) {
	logger := logging.NewLogger()
	config := DefaultKiroOAuthConfig()
	refresher := NewKiroRefresher(config, nil, logger, nil, nil)

	t.Run("returns provider ID", func(t *testing.T) {
		assert.Equal(t, "kiro", refresher.ProviderID())
	})

	t.Run("returns error for token without refresh token", func(t *testing.T) {
		token := ProviderToken{
			ID:          "test-token",
			AccessToken: "access",
			ExpiryDate:  time.Now().Add(time.Hour).UnixMilli(),
		}

		ctx := context.Background()
		_, err := refresher.RefreshToken(ctx, token)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no refresh token available")
	})
}

func TestIFlowRefresher(t *testing.T) {
	logger := logging.NewLogger()
	config := DefaultIFlowOAuthConfig()
	refresher := NewIFlowRefresher(config, nil, logger, nil, nil)

	t.Run("returns provider ID", func(t *testing.T) {
		assert.Equal(t, "iflow", refresher.ProviderID())
	})

	t.Run("returns error for token without refresh token", func(t *testing.T) {
		token := ProviderToken{
			ID:          "test-token",
			AccessToken: "access",
			ExpiryDate:  time.Now().Add(time.Hour).UnixMilli(),
		}

		ctx := context.Background()
		_, err := refresher.RefreshToken(ctx, token)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no refresh token available")
	})
}

func TestRefreshCoordinator_GetQueueSize(t *testing.T) {
	logger := logging.NewLogger()
	tempDir := t.TempDir()
	filePath := tempDir + "/store.json"
	store := NewMultiTokenStore("test-provider", filePath, logger)
	coordinator := NewRefreshCoordinator(store, 2, logger, nil)
	refresher := NewMockRefresher("test-provider")
	coordinator.RegisterRefresher(refresher)

	t.Run("returns queue size", func(t *testing.T) {
		assert.Equal(t, 0, coordinator.GetQueueSize())

		token := createTestToken("test-token")
		require.NoError(t, store.AddToken(token))

		require.NoError(t, coordinator.ScheduleRefresh("test-token", 10))
		assert.Equal(t, 1, coordinator.GetQueueSize())

		token2 := createTestToken("test-token-2")
		require.NoError(t, store.AddToken(token2))
		require.NoError(t, coordinator.ScheduleRefresh("test-token-2", 10))
		assert.Equal(t, 2, coordinator.GetQueueSize())
	})
}

func TestRefreshCoordinator_GetWorkerStatus(t *testing.T) {
	logger := logging.NewLogger()
	tempDir := t.TempDir()
	filePath := tempDir + "/store.json"
	store := NewMultiTokenStore("test-provider", filePath, logger)
	coordinator := NewRefreshCoordinator(store, 3, logger, nil)

	t.Run("returns worker status", func(t *testing.T) {
		// Before start
		status := coordinator.GetWorkerStatus()
		assert.Len(t, status, 3)
		assert.False(t, status[0])
		assert.False(t, status[1])
		assert.False(t, status[2])

		// After start
		err := coordinator.Start()
		require.NoError(t, err)

		time.Sleep(50 * time.Millisecond)
		status = coordinator.GetWorkerStatus()
		assert.True(t, status[0])
		assert.True(t, status[1])
		assert.True(t, status[2])

		coordinator.Stop()
	})
}

func TestRefreshResult(t *testing.T) {
	t.Run("creates result with success", func(t *testing.T) {
		token := createTestToken("test-token")
		result := RefreshResult{
			TokenID:  "test-token",
			Success:  true,
			NewToken: &token,
			Error:    nil,
			Duration: 100 * time.Millisecond,
		}

		assert.Equal(t, "test-token", result.TokenID)
		assert.True(t, result.Success)
		assert.NotNil(t, result.NewToken)
		assert.NoError(t, result.Error)
		assert.Equal(t, 100*time.Millisecond, result.Duration)
	})

	t.Run("creates result with failure", func(t *testing.T) {
		testErr := errors.New("test error")
		result := RefreshResult{
			TokenID:  "test-token",
			Success:  false,
			NewToken: nil,
			Error:    testErr,
			Duration: 50 * time.Millisecond,
		}

		assert.Equal(t, "test-token", result.TokenID)
		assert.False(t, result.Success)
		assert.Nil(t, result.NewToken)
		assert.Error(t, result.Error)
		assert.Equal(t, testErr, result.Error)
		assert.Equal(t, 50*time.Millisecond, result.Duration)
	})
}

func TestRefreshRequest(t *testing.T) {
	t.Run("creates refresh request", func(t *testing.T) {
		token := createTestToken("test-token")
		request := RefreshRequest{
			TokenID:     "test-token",
			ProviderID:  "test-provider",
			Priority:    10,
			ScheduledAt: time.Now(),
			Token:       token,
		}

		assert.Equal(t, "test-token", request.TokenID)
		assert.Equal(t, "test-provider", request.ProviderID)
		assert.Equal(t, 10, request.Priority)
		assert.NotZero(t, request.ScheduledAt)
		assert.Equal(t, "test-token", request.Token.ID)
	})
}
