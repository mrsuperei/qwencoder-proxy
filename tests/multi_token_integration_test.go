package tests

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunbankio/qwencoder-proxy/auth"
	"github.com/sunbankio/qwencoder-proxy/logging"
)

// createTestToken creates a test token with given ID
func createTestToken(id string, expiryOffset time.Duration) auth.ProviderToken {
	now := time.Now()
	return auth.ProviderToken{
		ID:           id,
		AccessToken:  "access-" + id,
		RefreshToken: "refresh-" + id,
		TokenType:    "Bearer",
		ExpiryDate:   now.Add(expiryOffset).UnixMilli(),
		Email:        id + "@example.com",
		Healthy:      true,
		HealthScore:  1.0,
		LastUsed:     now.UnixMilli(),
		CreatedAt:    now.UnixMilli(),
		ErrorCount:   0,
	}
}

// setupTestEnvironment creates a temporary directory and initializes a multi-token manager
func setupTestEnvironment(t *testing.T) (*auth.MultiTokenManager, func()) {
	tempDir := t.TempDir()
	logger := logging.NewLogger()

	manager := auth.NewMultiTokenManager(logger)
	manager.SetCredentialsDir(tempDir)

	// Initialize manager
	err := manager.Initialize()
	require.NoError(t, err)

	cleanup := func() {
		manager.Stop()
	}

	return manager, cleanup
}

func TestMultiTokenFlow(t *testing.T) {
	t.Run("complete token lifecycle", func(t *testing.T) {
		manager, cleanup := setupTestEnvironment(t)
		defer cleanup()

		providerID := "test-provider"

		// Register provider
		config := auth.ProviderConfig{
			ID:           providerID,
			ClientID:     "test-client-id",
			ClientSecret: "test-client-secret",
		}
		err := manager.RegisterProvider(providerID, config)
		require.NoError(t, err)

		// 1. Add tokens
		tokens := []auth.ProviderToken{
			createTestToken("token-1", 2*time.Hour),
			createTestToken("token-2", 3*time.Hour),
			createTestToken("token-3", 4*time.Hour),
		}

		for _, token := range tokens {
			err := manager.SaveToken(providerID, token)
			require.NoError(t, err)
		}

		// Verify tokens were added
		count, err := manager.GetProviderTokenCount(providerID)
		require.NoError(t, err)
		assert.Equal(t, 3, count)

		// 2. Select tokens
		selected, err := manager.SelectToken(providerID)
		require.NoError(t, err)
		require.NotNil(t, selected)
		assert.NotEmpty(t, selected.ID)
		assert.True(t, selected.Healthy)

		// 3. Test health tracking
		err = manager.ReportTokenSuccess(providerID, selected.ID)
		require.NoError(t, err)

		// Verify health score is still good
		store, err := manager.GetTokenStore(providerID)
		require.NoError(t, err)
		token, err := store.GetToken(selected.ID)
		require.NoError(t, err)
		assert.True(t, token.Healthy)
		assert.Equal(t, 1.0, token.HealthScore)

		// Test failure reporting
		err = manager.ReportTokenFailure(providerID, selected.ID, assert.AnError)
		require.NoError(t, err)

		token, err = store.GetToken(selected.ID)
		require.NoError(t, err)
		assert.Equal(t, 1, token.ErrorCount)
		assert.Less(t, token.HealthScore, 1.0)

		// 4. Test token removal
		err = manager.RemoveToken(providerID, selected.ID)
		require.NoError(t, err)

		count, err = manager.GetProviderTokenCount(providerID)
		require.NoError(t, err)
		assert.Equal(t, 2, count)
	})
}

func TestOAuthCallbackIntegration(t *testing.T) {
	t.Run("save token with email extraction", func(t *testing.T) {
		manager, cleanup := setupTestEnvironment(t)
		defer cleanup()

		providerID := "test-provider"

		// Register provider
		config := auth.ProviderConfig{
			ID:           providerID,
			ClientID:     "test-client-id",
			ClientSecret: "test-client-secret",
		}
		err := manager.RegisterProvider(providerID, config)
		require.NoError(t, err)

		// Get email extraction manager
		emailManager := manager.GetEmailExtractionManager()
		require.NotNil(t, emailManager)

		// Register mock extractor
		mockExtractor := &MockEmailExtractor{
			providerID: providerID,
			email:      "oauth@example.com",
		}
		emailManager.RegisterExtractor(mockExtractor)

		// Simulate OAuth callback with token response
		tokenResponse := map[string]interface{}{
			"access_token":  "oauth-access-token",
			"refresh_token": "oauth-refresh-token",
			"token_type":    "Bearer",
			"expires_in":    3600,
		}

		// Extract email
		ctx := context.Background()
		email, err := manager.ExtractEmail(ctx, providerID, tokenResponse, "oauth-access-token")
		require.NoError(t, err)
		assert.Equal(t, "oauth@example.com", email)

		// Create and save token
		token := auth.ProviderToken{
			AccessToken:  tokenResponse["access_token"].(string),
			RefreshToken: tokenResponse["refresh_token"].(string),
			TokenType:    tokenResponse["token_type"].(string),
			ExpiryDate:   time.Now().Add(time.Hour).UnixMilli(),
			Email:        email,
		}

		err = manager.SaveToken(providerID, token)
		require.NoError(t, err)

		// Verify token was saved
		store, err := manager.GetTokenStore(providerID)
		require.NoError(t, err)
		tokens := store.ListTokens()
		assert.Len(t, tokens, 1)
		assert.Equal(t, "oauth@example.com", tokens[0].Email)
	})
}

func TestTokenSelectionStrategies(t *testing.T) {
	t.Run("random strategy distributes selections", func(t *testing.T) {
		manager, cleanup := setupTestEnvironment(t)
		defer cleanup()

		providerID := "test-provider"

		// Register provider
		config := auth.ProviderConfig{
			ID:           providerID,
			ClientID:     "test-client-id",
			ClientSecret: "test-client-secret",
		}
		err := manager.RegisterProvider(providerID, config)
		require.NoError(t, err)

		// Add tokens
		for i := 1; i <= 5; i++ {
			token := createTestToken(string(rune('0'+i)), time.Hour)
			err := manager.SaveToken(providerID, token)
			require.NoError(t, err)
		}

		// Select tokens multiple times and track distribution
		counts := make(map[string]int)
		for i := 0; i < 50; i++ {
			token, err := manager.SelectToken(providerID)
			require.NoError(t, err)
			counts[token.ID]++
		}

		// All tokens should be selected at least a few times
		for id := 1; id <= 5; id++ {
			assert.Greater(t, counts[string(rune('0'+id))], 0, "Token %s should be selected", string(rune('0'+id)))
		}
	})

	t.Run("round-robin strategy cycles through tokens", func(t *testing.T) {
		manager, cleanup := setupTestEnvironment(t)
		defer cleanup()

		providerID := "test-provider"

		// Register provider
		config := auth.ProviderConfig{
			ID:           providerID,
			ClientID:     "test-client-id",
			ClientSecret: "test-client-secret",
		}
		err := manager.RegisterProvider(providerID, config)
		require.NoError(t, err)

		// Add tokens
		tokens := []auth.ProviderToken{
			createTestToken("a", time.Hour),
			createTestToken("b", time.Hour),
			createTestToken("c", time.Hour),
		}

		for _, token := range tokens {
			err := manager.SaveToken(providerID, token)
			require.NoError(t, err)
		}

		// Get token manager and change strategy
		tokenManager, err := manager.GetTokenManager(providerID)
		require.NoError(t, err)

		tokenManager.SetStrategy(auth.NewRoundRobinSelectionStrategy())

		// Select tokens and verify order
		selections := make([]string, 0)
		for i := 0; i < 6; i++ {
			token, err := manager.SelectToken(providerID)
			require.NoError(t, err)
			selections = append(selections, token.ID)
		}

		// Should cycle through a, b, c
		assert.Equal(t, "a", selections[0])
		assert.Equal(t, "b", selections[1])
		assert.Equal(t, "c", selections[2])
		assert.Equal(t, "a", selections[3])
		assert.Equal(t, "b", selections[4])
		assert.Equal(t, "c", selections[5])
	})

	t.Run("least-used strategy selects least recently used", func(t *testing.T) {
		manager, cleanup := setupTestEnvironment(t)
		defer cleanup()

		providerID := "test-provider"

		// Register provider
		config := auth.ProviderConfig{
			ID:           providerID,
			ClientID:     "test-client-id",
			ClientSecret: "test-client-secret",
		}
		err := manager.RegisterProvider(providerID, config)
		require.NoError(t, err)

		// Add tokens with different LastUsed times
		now := time.Now()
		tokens := []auth.ProviderToken{
			{ID: "a", AccessToken: "a1", ExpiryDate: now.Add(time.Hour).UnixMilli(), Healthy: true, LastUsed: now.Add(-30 * time.Minute).UnixMilli()},
			{ID: "b", AccessToken: "a2", ExpiryDate: now.Add(time.Hour).UnixMilli(), Healthy: true, LastUsed: now.Add(-10 * time.Minute).UnixMilli()},
			{ID: "c", AccessToken: "a3", ExpiryDate: now.Add(time.Hour).UnixMilli(), Healthy: true, LastUsed: now.Add(-5 * time.Minute).UnixMilli()},
		}

		for _, token := range tokens {
			err := manager.SaveToken(providerID, token)
			require.NoError(t, err)
		}

		// Get token manager and change strategy
		tokenManager, err := manager.GetTokenManager(providerID)
		require.NoError(t, err)

		tokenManager.SetStrategy(auth.NewLeastUsedSelectionStrategy())

		// Select token - should be 'a' (least recently used)
		token, err := manager.SelectToken(providerID)
		require.NoError(t, err)
		assert.Equal(t, "a", token.ID)
	})
}

func TestRefreshCoordination(t *testing.T) {
	t.Run("schedule and process refresh", func(t *testing.T) {
		manager, cleanup := setupTestEnvironment(t)
		defer cleanup()

		providerID := "test-provider"

		// Register provider
		config := auth.ProviderConfig{
			ID:           providerID,
			ClientID:     "test-client-id",
			ClientSecret: "test-client-secret",
		}
		err := manager.RegisterProvider(providerID, config)
		require.NoError(t, err)

		// Register mock refresher
		mockRefresher := &MockRefresher{
			providerID: providerID,
		}
		err = manager.RegisterRefresher(providerID, mockRefresher)
		require.NoError(t, err)

		// Add tokens
		token := createTestToken("token-1", 2*time.Hour)
		err = manager.SaveToken(providerID, token)
		require.NoError(t, err)

		// Start refresh scheduler
		err = manager.Start()
		require.NoError(t, err)
		defer manager.Stop()

		// Get refresh coordinator and schedule refresh
		store, err := manager.GetTokenStore(providerID)
		require.NoError(t, err)

		coordinator := auth.NewRefreshCoordinator(store, 2, logging.NewLogger())
		coordinator.RegisterRefresher(mockRefresher)
		err = coordinator.Start()
		require.NoError(t, err)
		defer coordinator.Stop()

		// Schedule refresh
		err = coordinator.ScheduleRefresh("token-1", 10)
		require.NoError(t, err)

		// Wait for refresh to process
		time.Sleep(1 * time.Second)

		// Verify token was refreshed
		refreshedToken, err := store.GetToken("token-1")
		require.NoError(t, err)
		assert.Contains(t, refreshedToken.AccessToken, "refreshed-")
	})
}

func TestHealthTrackingIntegration(t *testing.T) {
	t.Run("track token health over usage", func(t *testing.T) {
		manager, cleanup := setupTestEnvironment(t)
		defer cleanup()

		providerID := "test-provider"

		// Register provider
		config := auth.ProviderConfig{
			ID:           providerID,
			ClientID:     "test-client-id",
			ClientSecret: "test-client-secret",
		}
		err := manager.RegisterProvider(providerID, config)
		require.NoError(t, err)

		// Add tokens
		tokens := []auth.ProviderToken{
			createTestToken("token-1", time.Hour),
			createTestToken("token-2", time.Hour),
		}

		for _, token := range tokens {
			err := manager.SaveToken(providerID, token)
			require.NoError(t, err)
		}

		// Report successes for token-1
		for i := 0; i < 5; i++ {
			err := manager.ReportTokenSuccess(providerID, "token-1")
			require.NoError(t, err)
		}

		// Report failures for token-2
		for i := 0; i < 3; i++ {
			err := manager.ReportTokenFailure(providerID, "token-2", assert.AnError)
			require.NoError(t, err)
		}

		// Verify health states
		store, err := manager.GetTokenStore(providerID)
		require.NoError(t, err)

		token1, err := store.GetToken("token-1")
		require.NoError(t, err)
		assert.True(t, token1.Healthy)
		assert.Equal(t, 1.0, token1.HealthScore)
		assert.Equal(t, 0, token1.ErrorCount)

		token2, err := store.GetToken("token-2")
		require.NoError(t, err)
		assert.False(t, token2.Healthy) // Should be unhealthy after 3 errors
		assert.Less(t, token2.HealthScore, 1.0)
		assert.Equal(t, 3, token2.ErrorCount)

		// Get healthiest token
		healthTracker, err := manager.GetHealthTracker(providerID)
		require.NoError(t, err)

		healthiest, err := healthTracker.GetHealthiestToken()
		require.NoError(t, err)
		assert.Equal(t, "token-1", healthiest.ID)

		// Get unhealthy tokens
		unhealthy := healthTracker.GetUnhealthyTokens()
		assert.Len(t, unhealthy, 1)
		assert.Equal(t, "token-2", unhealthy[0].ID)
	})
}

func TestMultiProviderSupport(t *testing.T) {
	t.Run("manage tokens for multiple providers", func(t *testing.T) {
		manager, cleanup := setupTestEnvironment(t)
		defer cleanup()

		providers := []string{"qwen", "gemini", "kiro"}

		// Register all providers
		for _, providerID := range providers {
			config := auth.ProviderConfig{
				ID:           providerID,
				ClientID:     providerID + "-client-id",
				ClientSecret: providerID + "-client-secret",
			}
			err := manager.RegisterProvider(providerID, config)
			require.NoError(t, err)

			// Add tokens for each provider
			for i := 1; i <= 3; i++ {
				token := createTestToken(providerID+"-"+string(rune('0'+i)), time.Hour)
				err := manager.SaveToken(providerID, token)
				require.NoError(t, err)
			}
		}

		// Verify all providers are registered
		providerList := manager.ListProviders()
		assert.Len(t, providerList, 3)
		assert.Contains(t, providerList, "qwen")
		assert.Contains(t, providerList, "gemini")
		assert.Contains(t, providerList, "kiro")

		// Verify token counts
		for _, providerID := range providers {
			count, err := manager.GetProviderTokenCount(providerID)
			require.NoError(t, err)
			assert.Equal(t, 3, count)
		}

		// Select tokens from each provider
		for _, providerID := range providers {
			token, err := manager.SelectToken(providerID)
			require.NoError(t, err)
			require.NotNil(t, token)
			assert.Contains(t, token.ID, providerID)
		}
	})
}

func TestTokenPersistence(t *testing.T) {
	t.Run("tokens persist across manager restarts", func(t *testing.T) {
		tempDir := t.TempDir()
		logger := logging.NewLogger()

		providerID := "test-provider"

		// Create first manager and add tokens
		manager1 := auth.NewMultiTokenManager(logger)
		manager1.SetCredentialsDir(tempDir)
		err := manager1.Initialize()
		require.NoError(t, err)

		config := auth.ProviderConfig{
			ID:           providerID,
			ClientID:     "test-client-id",
			ClientSecret: "test-client-secret",
		}
		err = manager1.RegisterProvider(providerID, config)
		require.NoError(t, err)

		tokens := []auth.ProviderToken{
			createTestToken("token-1", 2*time.Hour),
			createTestToken("token-2", 3*time.Hour),
		}

		for _, token := range tokens {
			err := manager1.SaveToken(providerID, token)
			require.NoError(t, err)
		}

		manager1.Stop()

		// Create second manager and verify tokens are loaded
		manager2 := auth.NewMultiTokenManager(logger)
		manager2.SetCredentialsDir(tempDir)
		err = manager2.Initialize()
		require.NoError(t, err)
		defer manager2.Stop()

		count, err := manager2.GetProviderTokenCount(providerID)
		require.NoError(t, err)
		assert.Equal(t, 2, count)

		// Verify token details
		store, err := manager2.GetTokenStore(providerID)
		require.NoError(t, err)

		loadedTokens := store.ListTokens()
		assert.Len(t, loadedTokens, 2)

		ids := make([]string, 0, len(loadedTokens))
		for _, t := range loadedTokens {
			ids = append(ids, t.ID)
		}
		assert.Contains(t, ids, "token-1")
		assert.Contains(t, ids, "token-2")
	})
}

func TestEdgeCases(t *testing.T) {
	t.Run("handle empty token store", func(t *testing.T) {
		manager, cleanup := setupTestEnvironment(t)
		defer cleanup()

		providerID := "test-provider"

		// Register provider but don't add tokens
		config := auth.ProviderConfig{
			ID:           providerID,
			ClientID:     "test-client-id",
			ClientSecret: "test-client-secret",
		}
		err := manager.RegisterProvider(providerID, config)
		require.NoError(t, err)

		// Try to select token - should fail
		_, err = manager.SelectToken(providerID)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no tokens available")
	})

	t.Run("handle all unhealthy tokens", func(t *testing.T) {
		manager, cleanup := setupTestEnvironment(t)
		defer cleanup()

		providerID := "test-provider"

		// Register provider
		config := auth.ProviderConfig{
			ID:           providerID,
			ClientID:     "test-client-id",
			ClientSecret: "test-client-secret",
		}
		err := manager.RegisterProvider(providerID, config)
		require.NoError(t, err)

		// Add unhealthy tokens
		tokens := []auth.ProviderToken{
			createTestToken("token-1", time.Hour),
			createTestToken("token-2", time.Hour),
		}

		for _, token := range tokens {
			token.Healthy = false
			err := manager.SaveToken(providerID, token)
			require.NoError(t, err)
		}

		// Try to select token - should fail
		_, err = manager.SelectToken(providerID)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no valid tokens")
	})

	t.Run("handle expired tokens", func(t *testing.T) {
		manager, cleanup := setupTestEnvironment(t)
		defer cleanup()

		providerID := "test-provider"

		// Register provider
		config := auth.ProviderConfig{
			ID:           providerID,
			ClientID:     "test-client-id",
			ClientSecret: "test-client-secret",
		}
		err := manager.RegisterProvider(providerID, config)
		require.NoError(t, err)

		// Add expired tokens
		tokens := []auth.ProviderToken{
			createTestToken("token-1", -time.Hour), // Expired
			createTestToken("token-2", time.Hour),  // Valid
		}

		for _, token := range tokens {
			err := manager.SaveToken(providerID, token)
			require.NoError(t, err)
		}

		// Select token - should only get valid one
		token, err := manager.SelectToken(providerID)
		require.NoError(t, err)
		assert.Equal(t, "token-2", token.ID)
	})
}

func TestClearProviderTokens(t *testing.T) {
	t.Run("clear all tokens for a provider", func(t *testing.T) {
		manager, cleanup := setupTestEnvironment(t)
		defer cleanup()

		providerID := "test-provider"

		// Register provider
		config := auth.ProviderConfig{
			ID:           providerID,
			ClientID:     "test-client-id",
			ClientSecret: "test-client-secret",
		}
		err := manager.RegisterProvider(providerID, config)
		require.NoError(t, err)

		// Add tokens
		for i := 1; i <= 5; i++ {
			token := createTestToken(string(rune('0'+i)), time.Hour)
			err := manager.SaveToken(providerID, token)
			require.NoError(t, err)
		}

		// Verify tokens exist
		count, err := manager.GetProviderTokenCount(providerID)
		require.NoError(t, err)
		assert.Equal(t, 5, count)

		// Clear all tokens
		err = manager.ClearProviderTokens(providerID)
		require.NoError(t, err)

		// Verify tokens are cleared
		count, err = manager.GetProviderTokenCount(providerID)
		require.NoError(t, err)
		assert.Equal(t, 0, count)
	})
}

// Mock implementations for testing

// MockEmailExtractor implements auth.EmailExtractor
type MockEmailExtractor struct {
	providerID string
	email      string
	err        error
}

func (m *MockEmailExtractor) ExtractEmail(ctx context.Context, tokenResponse map[string]interface{}, accessToken string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return m.email, nil
}

func (m *MockEmailExtractor) ProviderID() string {
	return m.providerID
}

func (m *MockEmailExtractor) UserInfoURL() string {
	return "https://mock.example.com/userinfo"
}

// MockRefresher implements auth.ProviderRefresh
type MockRefresher struct {
	providerID string
}

func (m *MockRefresher) RefreshToken(ctx context.Context, token auth.ProviderToken) (auth.ProviderToken, error) {
	// Return a refreshed token
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

// Test that the mock implementations satisfy the interfaces
var _ auth.EmailExtractor = (*MockEmailExtractor)(nil)
var _ auth.ProviderRefresh = (*MockRefresher)(nil)
