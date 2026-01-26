package auth

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunbankio/qwencoder-proxy/logging"
)

func createTestTokens(count int) []ProviderToken {
	tokens := make([]ProviderToken, count)
	now := time.Now()
	for i := 0; i < count; i++ {
		tokens[i] = ProviderToken{
			ID:          string(rune('a' + i)),
			AccessToken: "access-" + string(rune('0'+i)),
			ExpiryDate:  now.Add(time.Duration(i+1) * time.Hour).UnixMilli(),
			Healthy:     true,
			HealthScore: 1.0,
			LastUsed:    now.Add(-time.Duration(i) * time.Minute).UnixMilli(),
			CreatedAt:   now.UnixMilli(),
		}
	}
	return tokens
}

func createTestStore(tokens []ProviderToken) *MultiTokenStore {
	logger := logging.NewLogger()
	store := NewMultiTokenStore("test-provider", "", logger)
	store.Tokens = tokens
	return store
}

func TestRandomSelectionStrategy(t *testing.T) {
	strategy := NewRandomSelectionStrategy()

	t.Run("returns strategy name", func(t *testing.T) {
		assert.Equal(t, "random", strategy.Name())
	})

	t.Run("selects token from valid tokens", func(t *testing.T) {
		tokens := createTestTokens(3)
		token, err := strategy.SelectToken(tokens)
		require.NoError(t, err)
		require.NotNil(t, token)
		assert.Contains(t, []string{"a", "b", "c"}, token.ID)
	})

	t.Run("returns error when no valid tokens", func(t *testing.T) {
		tokens := []ProviderToken{}
		_, err := strategy.SelectToken(tokens)
		assert.Error(t, err)
		assert.Equal(t, ErrNoValidTokens, err)
	})

	t.Run("filters out unhealthy tokens", func(t *testing.T) {
		tokens := createTestTokens(3)
		tokens[1].Healthy = false

		token, err := strategy.SelectToken(tokens)
		require.NoError(t, err)
		assert.NotEqual(t, "b", token.ID, "Should not select unhealthy token")
	})

	t.Run("filters out expired tokens", func(t *testing.T) {
		tokens := createTestTokens(3)
		tokens[2].ExpiryDate = time.Now().Add(-time.Hour).UnixMilli()

		token, err := strategy.SelectToken(tokens)
		require.NoError(t, err)
		assert.NotEqual(t, "c", token.ID, "Should not select expired token")
	})

	t.Run("distributes selections randomly", func(t *testing.T) {
		tokens := createTestTokens(10)
		counts := make(map[string]int)

		for i := 0; i < 100; i++ {
			token, err := strategy.SelectToken(tokens)
			require.NoError(t, err)
			counts[token.ID]++
		}

		// All tokens should be selected at least a few times
		for id := 0; id < 10; id++ {
			assert.Greater(t, counts[string(rune('a'+id))], 0, "Token %s should be selected", string(rune('a'+id)))
		}
	})
}

func TestRoundRobinSelectionStrategy(t *testing.T) {
	strategy := NewRoundRobinSelectionStrategy()

	t.Run("returns strategy name", func(t *testing.T) {
		assert.Equal(t, "round_robin", strategy.Name())
	})

	t.Run("selects tokens in order", func(t *testing.T) {
		tokens := createTestTokens(3)

		token1, err := strategy.SelectToken(tokens)
		require.NoError(t, err)
		assert.Equal(t, "a", token1.ID)

		token2, err := strategy.SelectToken(tokens)
		require.NoError(t, err)
		assert.Equal(t, "b", token2.ID)

		token3, err := strategy.SelectToken(tokens)
		require.NoError(t, err)
		assert.Equal(t, "c", token3.ID)

		token4, err := strategy.SelectToken(tokens)
		require.NoError(t, err)
		assert.Equal(t, "a", token4.ID) // Wraps around
	})

	t.Run("returns error when no valid tokens", func(t *testing.T) {
		tokens := []ProviderToken{}
		_, err := strategy.SelectToken(tokens)
		assert.Error(t, err)
		assert.Equal(t, ErrNoValidTokens, err)
	})

	t.Run("filters out unhealthy tokens", func(t *testing.T) {
		tokens := createTestTokens(3)
		tokens[1].Healthy = false

		token1, err := strategy.SelectToken(tokens)
		require.NoError(t, err)
		assert.Equal(t, "a", token1.ID)

		token2, err := strategy.SelectToken(tokens)
		require.NoError(t, err)
		assert.Equal(t, "c", token2.ID) // Skips unhealthy 'b'

		token3, err := strategy.SelectToken(tokens)
		require.NoError(t, err)
		assert.Equal(t, "a", token3.ID) // Wraps around
	})

	t.Run("handles single token", func(t *testing.T) {
		tokens := createTestTokens(1)

		for i := 0; i < 5; i++ {
			token, err := strategy.SelectToken(tokens)
			require.NoError(t, err)
			assert.Equal(t, "a", token.ID)
		}
	})
}

func TestLeastUsedSelectionStrategy(t *testing.T) {
	strategy := NewLeastUsedSelectionStrategy()

	t.Run("returns strategy name", func(t *testing.T) {
		assert.Equal(t, "least_used", strategy.Name())
	})

	t.Run("selects least recently used token", func(t *testing.T) {
		now := time.Now()
		tokens := []ProviderToken{
			{ID: "a", AccessToken: "a1", ExpiryDate: now.Add(time.Hour).UnixMilli(), Healthy: true, LastUsed: now.Add(-30 * time.Minute).UnixMilli()},
			{ID: "b", AccessToken: "a2", ExpiryDate: now.Add(time.Hour).UnixMilli(), Healthy: true, LastUsed: now.Add(-10 * time.Minute).UnixMilli()},
			{ID: "c", AccessToken: "a3", ExpiryDate: now.Add(time.Hour).UnixMilli(), Healthy: true, LastUsed: now.Add(-5 * time.Minute).UnixMilli()},
		}

		token, err := strategy.SelectToken(tokens)
		require.NoError(t, err)
		assert.Equal(t, "a", token.ID, "Should select least recently used token")
	})

	t.Run("returns error when no valid tokens", func(t *testing.T) {
		tokens := []ProviderToken{}
		_, err := strategy.SelectToken(tokens)
		assert.Error(t, err)
		assert.Equal(t, ErrNoValidTokens, err)
	})

	t.Run("filters out unhealthy tokens", func(t *testing.T) {
		now := time.Now()
		tokens := []ProviderToken{
			{ID: "a", AccessToken: "a1", ExpiryDate: now.Add(time.Hour).UnixMilli(), Healthy: true, LastUsed: now.Add(-30 * time.Minute).UnixMilli()},
			{ID: "b", AccessToken: "a2", ExpiryDate: now.Add(time.Hour).UnixMilli(), Healthy: false, LastUsed: now.Add(-60 * time.Minute).UnixMilli()},
			{ID: "c", AccessToken: "a3", ExpiryDate: now.Add(time.Hour).UnixMilli(), Healthy: true, LastUsed: now.Add(-10 * time.Minute).UnixMilli()},
		}

		token, err := strategy.SelectToken(tokens)
		require.NoError(t, err)
		assert.Equal(t, "a", token.ID, "Should skip unhealthy token")
	})

	t.Run("handles single token", func(t *testing.T) {
		tokens := createTestTokens(1)

		for i := 0; i < 5; i++ {
			token, err := strategy.SelectToken(tokens)
			require.NoError(t, err)
			assert.Equal(t, "a", token.ID)
		}
	})
}

func TestTokenManager_SelectToken(t *testing.T) {
	logger := logging.NewLogger()
	tempDir := t.TempDir()
	filePath := tempDir + "/store.json"

	t.Run("selects token using configured strategy", func(t *testing.T) {
		store := NewMultiTokenStore("test-provider", filePath, logger)
		tokens := createTestTokens(3)
		for _, token := range tokens {
			require.NoError(t, store.AddToken(token))
		}

		manager := NewTokenManager(store, NewRandomSelectionStrategy(), logger)

		token, err := manager.SelectToken()
		require.NoError(t, err)
		require.NotNil(t, token)
		assert.Contains(t, []string{"a", "b", "c"}, token.ID)
	})

	t.Run("returns error when no tokens available", func(t *testing.T) {
		store := NewMultiTokenStore("test-provider", filePath, logger)
		manager := NewTokenManager(store, NewRandomSelectionStrategy(), logger)

		_, err := manager.SelectToken()
		assert.Error(t, err)
		assert.Equal(t, ErrNoTokensAvailable, err)
	})

	t.Run("returns error when no valid tokens", func(t *testing.T) {
		store := NewMultiTokenStore("test-provider", filePath, logger)
		tokens := createTestTokens(3)
		for i := range tokens {
			tokens[i].Healthy = false
		}
		for _, token := range tokens {
			require.NoError(t, store.AddToken(token))
		}

		manager := NewTokenManager(store, NewRandomSelectionStrategy(), logger)

		_, err := manager.SelectToken()
		assert.Error(t, err)
		assert.Equal(t, ErrNoValidTokens, err)
	})

	t.Run("updates LastUsed timestamp", func(t *testing.T) {
		store := NewMultiTokenStore("test-provider", filePath, logger)
		tokens := createTestTokens(1)
		for _, token := range tokens {
			require.NoError(t, store.AddToken(token))
		}

		manager := NewTokenManager(store, NewRandomSelectionStrategy(), logger)

		token, err := manager.SelectToken()
		require.NoError(t, err)

		retrieved, err := store.GetToken(token.ID)
		require.NoError(t, err)
		assert.Greater(t, retrieved.LastUsed, tokens[0].LastUsed)
	})
}

func TestTokenManager_SelectTokenByID(t *testing.T) {
	logger := logging.NewLogger()
	tempDir := t.TempDir()
	filePath := tempDir + "/store.json"

	t.Run("selects specific token by ID", func(t *testing.T) {
		store := NewMultiTokenStore("test-provider", filePath, logger)
		tokens := createTestTokens(3)
		for _, token := range tokens {
			require.NoError(t, store.AddToken(token))
		}

		manager := NewTokenManager(store, NewRandomSelectionStrategy(), logger)

		token, err := manager.SelectTokenByID("b")
		require.NoError(t, err)
		assert.Equal(t, "b", token.ID)
		assert.Equal(t, "access-1", token.AccessToken)
	})

	t.Run("returns error for non-existent token", func(t *testing.T) {
		store := NewMultiTokenStore("test-provider", filePath, logger)
		tokens := createTestTokens(3)
		for _, token := range tokens {
			require.NoError(t, store.AddToken(token))
		}

		manager := NewTokenManager(store, NewRandomSelectionStrategy(), logger)

		_, err := manager.SelectTokenByID("non-existent")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "token not found")
	})

	t.Run("returns error for invalid token", func(t *testing.T) {
		store := NewMultiTokenStore("test-provider", filePath, logger)
		tokens := createTestTokens(3)
		tokens[1].Healthy = false
		for _, token := range tokens {
			require.NoError(t, store.AddToken(token))
		}

		manager := NewTokenManager(store, NewRandomSelectionStrategy(), logger)

		_, err := manager.SelectTokenByID("b")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not valid")
	})

	t.Run("updates LastUsed timestamp", func(t *testing.T) {
		store := NewMultiTokenStore("test-provider", filePath, logger)
		tokens := createTestTokens(1)
		for _, token := range tokens {
			require.NoError(t, store.AddToken(token))
		}

		manager := NewTokenManager(store, NewRandomSelectionStrategy(), logger)

		token, err := manager.SelectTokenByID("a")
		require.NoError(t, err)

		retrieved, err := store.GetToken(token.ID)
		require.NoError(t, err)
		assert.Greater(t, retrieved.LastUsed, tokens[0].LastUsed)
	})
}

func TestTokenManager_SetStrategy(t *testing.T) {
	logger := logging.NewLogger()
	tempDir := t.TempDir()
	filePath := tempDir + "/store.json"

	t.Run("changes selection strategy", func(t *testing.T) {
		store := NewMultiTokenStore("test-provider", filePath, logger)
		tokens := createTestTokens(3)
		for _, token := range tokens {
			require.NoError(t, store.AddToken(token))
		}

		manager := NewTokenManager(store, NewRandomSelectionStrategy(), logger)

		assert.Equal(t, "random", manager.GetStrategy().Name())

		manager.SetStrategy(NewRoundRobinSelectionStrategy())
		assert.Equal(t, "round_robin", manager.GetStrategy().Name())

		manager.SetStrategy(NewLeastUsedSelectionStrategy())
		assert.Equal(t, "least_used", manager.GetStrategy().Name())
	})
}

func TestTokenManager_GetValidTokens(t *testing.T) {
	logger := logging.NewLogger()
	tempDir := t.TempDir()
	filePath := tempDir + "/store.json"

	t.Run("returns all valid tokens", func(t *testing.T) {
		store := NewMultiTokenStore("test-provider", filePath, logger)
		tokens := createTestTokens(5)
		tokens[1].Healthy = false
		tokens[3].ExpiryDate = time.Now().Add(-time.Hour).UnixMilli()
		for _, token := range tokens {
			require.NoError(t, store.AddToken(token))
		}

		manager := NewTokenManager(store, NewRandomSelectionStrategy(), logger)

		validTokens, err := manager.GetValidTokens()
		require.NoError(t, err)
		assert.Len(t, validTokens, 3)

		ids := make([]string, 0, len(validTokens))
		for _, t := range validTokens {
			ids = append(ids, t.ID)
		}
		assert.Contains(t, ids, "a")
		assert.Contains(t, ids, "c")
		assert.Contains(t, ids, "e")
	})
}

func TestTokenManager_GetTokenCount(t *testing.T) {
	logger := logging.NewLogger()
	tempDir := t.TempDir()
	filePath := tempDir + "/store.json"

	t.Run("returns total token count", func(t *testing.T) {
		store := NewMultiTokenStore("test-provider", filePath, logger)
		tokens := createTestTokens(5)
		for _, token := range tokens {
			require.NoError(t, store.AddToken(token))
		}

		manager := NewTokenManager(store, NewRandomSelectionStrategy(), logger)

		assert.Equal(t, 5, manager.GetTokenCount())
	})
}

func TestTokenManager_GetValidTokenCount(t *testing.T) {
	logger := logging.NewLogger()
	tempDir := t.TempDir()
	filePath := tempDir + "/store.json"

	t.Run("returns valid token count", func(t *testing.T) {
		store := NewMultiTokenStore("test-provider", filePath, logger)
		tokens := createTestTokens(5)
		tokens[1].Healthy = false
		tokens[3].ExpiryDate = time.Now().Add(-time.Hour).UnixMilli()
		for _, token := range tokens {
			require.NoError(t, store.AddToken(token))
		}

		manager := NewTokenManager(store, NewRandomSelectionStrategy(), logger)

		assert.Equal(t, 3, manager.GetValidTokenCount())
	})
}

func TestHealthTracker_ReportSuccess(t *testing.T) {
	logger := logging.NewLogger()
	tempDir := t.TempDir()
	filePath := tempDir + "/store.json"

	t.Run("reports success and updates token", func(t *testing.T) {
		store := NewMultiTokenStore("test-provider", filePath, logger)
		token := ProviderToken{
			ID:          "test-token",
			AccessToken: "access",
			ExpiryDate:  time.Now().Add(time.Hour).UnixMilli(),
			Healthy:     false,
			HealthScore: 0.5,
			ErrorCount:  3,
			LastError:   "some error",
		}
		require.NoError(t, store.AddToken(token))

		tracker := NewHealthTracker(store, logger)
		err := tracker.ReportSuccess("test-token")
		require.NoError(t, err)

		retrieved, err := store.GetToken("test-token")
		require.NoError(t, err)
		assert.True(t, retrieved.Healthy)
		assert.Equal(t, 0, retrieved.ErrorCount)
		assert.Empty(t, retrieved.LastError)
		assert.Greater(t, retrieved.LastUsed, token.LastUsed)
	})

	t.Run("increases health score gradually", func(t *testing.T) {
		store := NewMultiTokenStore("test-provider", filePath, logger)
		token := ProviderToken{
			ID:          "test-token",
			AccessToken: "access",
			ExpiryDate:  time.Now().Add(time.Hour).UnixMilli(),
			HealthScore: 0.5,
		}
		require.NoError(t, store.AddToken(token))

		tracker := NewHealthTracker(store, logger)

		err := tracker.ReportSuccess("test-token")
		require.NoError(t, err)

		retrieved, err := store.GetToken("test-token")
		require.NoError(t, err)
		assert.Equal(t, 0.6, retrieved.HealthScore)
	})

	t.Run("caps health score at 1.0", func(t *testing.T) {
		store := NewMultiTokenStore("test-provider", filePath, logger)
		token := ProviderToken{
			ID:          "test-token",
			AccessToken: "access",
			ExpiryDate:  time.Now().Add(time.Hour).UnixMilli(),
			HealthScore: 0.95,
		}
		require.NoError(t, store.AddToken(token))

		tracker := NewHealthTracker(store, logger)

		err := tracker.ReportSuccess("test-token")
		require.NoError(t, err)

		retrieved, err := store.GetToken("test-token")
		require.NoError(t, err)
		assert.Equal(t, 1.0, retrieved.HealthScore)
	})
}

func TestHealthTracker_ReportFailure(t *testing.T) {
	logger := logging.NewLogger()
	tempDir := t.TempDir()
	filePath := tempDir + "/store.json"

	t.Run("reports failure and updates token", func(t *testing.T) {
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

		tracker := NewHealthTracker(store, logger)
		testErr := assert.AnError
		err := tracker.ReportFailure("test-token", testErr)
		require.NoError(t, err)

		retrieved, err := store.GetToken("test-token")
		require.NoError(t, err)
		assert.Equal(t, 1, retrieved.ErrorCount)
		assert.Equal(t, 0.8, retrieved.HealthScore)
		assert.Contains(t, retrieved.LastError, testErr.Error())
	})

	t.Run("marks token unhealthy after max errors", func(t *testing.T) {
		store := NewMultiTokenStore("test-provider", filePath, logger)
		token := ProviderToken{
			ID:          "test-token",
			AccessToken: "access",
			ExpiryDate:  time.Now().Add(time.Hour).UnixMilli(),
			Healthy:     true,
			HealthScore: 1.0,
			ErrorCount:  2,
		}
		require.NoError(t, store.AddToken(token))

		tracker := NewHealthTracker(store, logger)
		err := tracker.ReportFailure("test-token", assert.AnError)
		require.NoError(t, err)

		retrieved, err := store.GetToken("test-token")
		require.NoError(t, err)
		assert.False(t, retrieved.Healthy)
		assert.Equal(t, 3, retrieved.ErrorCount)
	})

	t.Run("decreases health score gradually", func(t *testing.T) {
		store := NewMultiTokenStore("test-provider", filePath, logger)
		token := ProviderToken{
			ID:          "test-token",
			AccessToken: "access",
			ExpiryDate:  time.Now().Add(time.Hour).UnixMilli(),
			HealthScore: 1.0,
		}
		require.NoError(t, store.AddToken(token))

		tracker := NewHealthTracker(store, logger)

		for i := 0; i < 3; i++ {
			err := tracker.ReportFailure("test-token", assert.AnError)
			require.NoError(t, err)
		}

		retrieved, err := store.GetToken("test-token")
		require.NoError(t, err)
		assert.Equal(t, 0.4, retrieved.HealthScore)
	})
}

func TestHealthTracker_GetHealthiestToken(t *testing.T) {
	logger := logging.NewLogger()
	tempDir := t.TempDir()
	filePath := tempDir + "/store.json"

	t.Run("returns token with highest health score", func(t *testing.T) {
		store := NewMultiTokenStore("test-provider", filePath, logger)
		tokens := []ProviderToken{
			{ID: "a", AccessToken: "a1", ExpiryDate: time.Now().Add(time.Hour).UnixMilli(), Healthy: true, HealthScore: 0.7},
			{ID: "b", AccessToken: "a2", ExpiryDate: time.Now().Add(time.Hour).UnixMilli(), Healthy: true, HealthScore: 0.9},
			{ID: "c", AccessToken: "a3", ExpiryDate: time.Now().Add(time.Hour).UnixMilli(), Healthy: true, HealthScore: 0.5},
		}
		for _, token := range tokens {
			require.NoError(t, store.AddToken(token))
		}

		tracker := NewHealthTracker(store, logger)
		token, err := tracker.GetHealthiestToken()
		require.NoError(t, err)
		assert.Equal(t, "b", token.ID)
	})

	t.Run("returns error when no valid tokens", func(t *testing.T) {
		store := NewMultiTokenStore("test-provider", filePath, logger)
		tracker := NewHealthTracker(store, logger)

		_, err := tracker.GetHealthiestToken()
		assert.Error(t, err)
		assert.Equal(t, ErrNoValidTokens, err)
	})

	t.Run("filters out unhealthy tokens", func(t *testing.T) {
		store := NewMultiTokenStore("test-provider", filePath, logger)
		tokens := []ProviderToken{
			{ID: "a", AccessToken: "a1", ExpiryDate: time.Now().Add(time.Hour).UnixMilli(), Healthy: true, HealthScore: 0.7},
			{ID: "b", AccessToken: "a2", ExpiryDate: time.Now().Add(time.Hour).UnixMilli(), Healthy: false, HealthScore: 0.9},
			{ID: "c", AccessToken: "a3", ExpiryDate: time.Now().Add(time.Hour).UnixMilli(), Healthy: true, HealthScore: 0.5},
		}
		for _, token := range tokens {
			require.NoError(t, store.AddToken(token))
		}

		tracker := NewHealthTracker(store, logger)
		token, err := tracker.GetHealthiestToken()
		require.NoError(t, err)
		assert.Equal(t, "a", token.ID)
	})
}

func TestHealthTracker_GetUnhealthyTokens(t *testing.T) {
	logger := logging.NewLogger()
	tempDir := t.TempDir()
	filePath := tempDir + "/store.json"

	t.Run("returns unhealthy tokens", func(t *testing.T) {
		store := NewMultiTokenStore("test-provider", filePath, logger)
		tokens := []ProviderToken{
			{ID: "a", AccessToken: "a1", ExpiryDate: time.Now().Add(time.Hour).UnixMilli(), Healthy: true},
			{ID: "b", AccessToken: "a2", ExpiryDate: time.Now().Add(time.Hour).UnixMilli(), Healthy: false},
			{ID: "c", AccessToken: "a3", ExpiryDate: time.Now().Add(time.Hour).UnixMilli(), Healthy: true},
			{ID: "d", AccessToken: "a4", ExpiryDate: time.Now().Add(time.Hour).UnixMilli(), Healthy: false},
		}
		for _, token := range tokens {
			require.NoError(t, store.AddToken(token))
		}

		tracker := NewHealthTracker(store, logger)
		unhealthy := tracker.GetUnhealthyTokens()

		assert.Len(t, unhealthy, 2)
		ids := make([]string, 0, len(unhealthy))
		for _, t := range unhealthy {
			ids = append(ids, t.ID)
		}
		assert.Contains(t, ids, "b")
		assert.Contains(t, ids, "d")
	})

	t.Run("returns empty list when all tokens are healthy", func(t *testing.T) {
		store := NewMultiTokenStore("test-provider", filePath, logger)
		tokens := createTestTokens(3)
		for _, token := range tokens {
			require.NoError(t, store.AddToken(token))
		}

		tracker := NewHealthTracker(store, logger)
		unhealthy := tracker.GetUnhealthyTokens()

		assert.Empty(t, unhealthy)
	})
}

func TestStrategyFactory(t *testing.T) {
	factory := NewStrategyFactory()

	t.Run("creates random strategy", func(t *testing.T) {
		strategy, err := factory.CreateStrategy("random")
		require.NoError(t, err)
		assert.Equal(t, "random", strategy.Name())
	})

	t.Run("creates round_robin strategy", func(t *testing.T) {
		strategy, err := factory.CreateStrategy("round_robin")
		require.NoError(t, err)
		assert.Equal(t, "round_robin", strategy.Name())
	})

	t.Run("creates least_used strategy", func(t *testing.T) {
		strategy, err := factory.CreateStrategy("least_used")
		require.NoError(t, err)
		assert.Equal(t, "least_used", strategy.Name())
	})

	t.Run("returns error for unknown strategy", func(t *testing.T) {
		_, err := factory.CreateStrategy("unknown")
		assert.Error(t, err)
		assert.Equal(t, ErrUnknownStrategy, err)
	})

	t.Run("lists available strategies", func(t *testing.T) {
		strategies := factory.ListStrategies()
		assert.Len(t, strategies, 3)
		assert.Contains(t, strategies, "random")
		assert.Contains(t, strategies, "round_robin")
		assert.Contains(t, strategies, "least_used")
	})
}

func TestFilterValidTokens(t *testing.T) {
	t.Run("filters valid tokens", func(t *testing.T) {
		now := time.Now()
		tokens := []ProviderToken{
			{ID: "valid-1", AccessToken: "a1", ExpiryDate: now.Add(time.Hour).UnixMilli(), Healthy: true},
			{ID: "valid-2", AccessToken: "a2", ExpiryDate: now.Add(2 * time.Hour).UnixMilli(), Healthy: true},
			{ID: "unhealthy", AccessToken: "a3", ExpiryDate: now.Add(time.Hour).UnixMilli(), Healthy: false},
			{ID: "expired", AccessToken: "a4", ExpiryDate: now.Add(-time.Hour).UnixMilli(), Healthy: true},
			{ID: "no-expiry", AccessToken: "a5", ExpiryDate: 0, Healthy: true},
		}

		valid := filterValidTokens(tokens)
		assert.Len(t, valid, 2)

		ids := make([]string, 0, len(valid))
		for _, t := range valid {
			ids = append(ids, t.ID)
		}
		assert.Contains(t, ids, "valid-1")
		assert.Contains(t, ids, "valid-2")
	})

	t.Run("handles empty list", func(t *testing.T) {
		valid := filterValidTokens([]ProviderToken{})
		assert.Empty(t, valid)
	})
}
