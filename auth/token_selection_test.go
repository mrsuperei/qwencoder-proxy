package auth

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunbankio/qwencoder-proxy/logging"
)

// mockProxyClientFactory is a mock implementation of ProxyClientFactory for testing
type mockProxyClientFactory struct {
	mockClient *http.Client
	calls      []*ProxyConfig
}

func (m *mockProxyClientFactory) GetClient(proxyConfig *ProxyConfig) *http.Client {
	m.calls = append(m.calls, proxyConfig)
	if m.mockClient != nil {
		return m.mockClient
	}
	return &http.Client{}
}

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

		manager := NewTokenManager(store, NewRandomSelectionStrategy(), logger, nil, nil)

		token, err := manager.SelectToken()
		require.NoError(t, err)
		require.NotNil(t, token)
		assert.Contains(t, []string{"a", "b", "c"}, token.ID)
	})

	t.Run("returns error when no tokens available", func(t *testing.T) {
		store := NewMultiTokenStore("test-provider", filePath, logger)
		manager := NewTokenManager(store, NewRandomSelectionStrategy(), logger, nil, nil)

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

		manager := NewTokenManager(store, NewRandomSelectionStrategy(), logger, nil, nil)

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

		manager := NewTokenManager(store, NewRandomSelectionStrategy(), logger, nil, nil)

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

		manager := NewTokenManager(store, NewRandomSelectionStrategy(), logger, nil, nil)

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

		manager := NewTokenManager(store, NewRandomSelectionStrategy(), logger, nil, nil)

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

		manager := NewTokenManager(store, NewRandomSelectionStrategy(), logger, nil, nil)

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

		manager := NewTokenManager(store, NewRandomSelectionStrategy(), logger, nil, nil)

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

		manager := NewTokenManager(store, NewRandomSelectionStrategy(), logger, nil, nil)

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

		manager := NewTokenManager(store, NewRandomSelectionStrategy(), logger, nil, nil)

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

		manager := NewTokenManager(store, NewRandomSelectionStrategy(), logger, nil, nil)

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

		manager := NewTokenManager(store, NewRandomSelectionStrategy(), logger, nil, nil)

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

func TestTokenManager_SelectTokenWithClient(t *testing.T) {
	logger := logging.NewLogger()
	tempDir := t.TempDir()
	filePath := tempDir + "/store.json"

	t.Run("selects token with direct connection (no proxy)", func(t *testing.T) {
		store := NewMultiTokenStore("test-provider", filePath, logger)
		tokens := createTestTokens(1)
		for _, token := range tokens {
			require.NoError(t, store.AddToken(token))
		}

		mockFactory := &mockProxyClientFactory{}
		proxyTracker := NewProxyHealthTracker(logger, 5, 5*time.Minute)
		manager := NewTokenManager(store, NewRandomSelectionStrategy(), logger, mockFactory, proxyTracker)

		token, client, err := manager.SelectTokenWithClient()
		require.NoError(t, err)
		require.NotNil(t, token)
		require.NotNil(t, client)
		assert.Equal(t, "a", token.ID)
		assert.Nil(t, token.Proxy)
		assert.Len(t, mockFactory.calls, 1)
		assert.Nil(t, mockFactory.calls[0])
	})

	t.Run("selects token with HTTP proxy", func(t *testing.T) {
		store := NewMultiTokenStore("test-provider", filePath, logger)
		tokens := createTestTokens(1)
		tokens[0].Proxy = &ProxyConfig{
			Type:    ProxyTypeHTTP,
			Host:    "proxy.example.com",
			Port:    8080,
			Enabled: true,
		}
		for _, token := range tokens {
			require.NoError(t, store.AddToken(token))
		}

		mockFactory := &mockProxyClientFactory{}
		proxyTracker := NewProxyHealthTracker(logger, 5, 5*time.Minute)
		manager := NewTokenManager(store, NewRandomSelectionStrategy(), logger, mockFactory, proxyTracker)

		token, client, err := manager.SelectTokenWithClient()
		require.NoError(t, err)
		require.NotNil(t, token)
		require.NotNil(t, client)
		assert.Equal(t, "a", token.ID)
		assert.NotNil(t, token.Proxy)
		assert.Equal(t, ProxyTypeHTTP, token.Proxy.Type)
		assert.Len(t, mockFactory.calls, 1)
		assert.Equal(t, token.Proxy, mockFactory.calls[0])
	})

	t.Run("returns error when no tokens available", func(t *testing.T) {
		store := NewMultiTokenStore("test-provider", filePath, logger)
		mockFactory := &mockProxyClientFactory{}
		proxyTracker := NewProxyHealthTracker(logger, 5, 5*time.Minute)
		manager := NewTokenManager(store, NewRandomSelectionStrategy(), logger, mockFactory, proxyTracker)

		_, _, err := manager.SelectTokenWithClient()
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

		mockFactory := &mockProxyClientFactory{}
		proxyTracker := NewProxyHealthTracker(logger, 5, 5*time.Minute)
		manager := NewTokenManager(store, NewRandomSelectionStrategy(), logger, mockFactory, proxyTracker)

		_, _, err := manager.SelectTokenWithClient()
		assert.Error(t, err)
		assert.Equal(t, ErrNoValidTokens, err)
	})

	t.Run("handles nil client factory gracefully", func(t *testing.T) {
		store := NewMultiTokenStore("test-provider", filePath, logger)
		tokens := createTestTokens(1)
		for _, token := range tokens {
			require.NoError(t, store.AddToken(token))
		}

		proxyTracker := NewProxyHealthTracker(logger, 5, 5*time.Minute)
		manager := NewTokenManager(store, NewRandomSelectionStrategy(), logger, nil, proxyTracker)

		token, client, err := manager.SelectTokenWithClient()
		require.NoError(t, err)
		require.NotNil(t, token)
		assert.Nil(t, client)
	})

	t.Run("handles nil proxy health tracker gracefully", func(t *testing.T) {
		store := NewMultiTokenStore("test-provider", filePath, logger)
		tokens := createTestTokens(1)
		for _, token := range tokens {
			require.NoError(t, store.AddToken(token))
		}

		mockFactory := &mockProxyClientFactory{}
		manager := NewTokenManager(store, NewRandomSelectionStrategy(), logger, mockFactory, nil)

		token, client, err := manager.SelectTokenWithClient()
		require.NoError(t, err)
		require.NotNil(t, token)
		require.NotNil(t, client)
	})

	t.Run("updates LastUsed timestamp", func(t *testing.T) {
		store := NewMultiTokenStore("test-provider", filePath, logger)
		tokens := createTestTokens(1)
		for _, token := range tokens {
			require.NoError(t, store.AddToken(token))
		}

		mockFactory := &mockProxyClientFactory{}
		proxyTracker := NewProxyHealthTracker(logger, 5, 5*time.Minute)
		manager := NewTokenManager(store, NewRandomSelectionStrategy(), logger, mockFactory, proxyTracker)

		token, _, err := manager.SelectTokenWithClient()
		require.NoError(t, err)

		retrieved, err := store.GetToken(token.ID)
		require.NoError(t, err)
		assert.Greater(t, retrieved.LastUsed, tokens[0].LastUsed)
	})
}

func TestTokenManager_GetTokenClient(t *testing.T) {
	logger := logging.NewLogger()
	tempDir := t.TempDir()
	filePath := tempDir + "/store.json"

	t.Run("returns client for token without proxy", func(t *testing.T) {
		store := NewMultiTokenStore("test-provider", filePath, logger)
		tokens := createTestTokens(1)
		for _, token := range tokens {
			require.NoError(t, store.AddToken(token))
		}

		mockFactory := &mockProxyClientFactory{}
		proxyTracker := NewProxyHealthTracker(logger, 5, 5*time.Minute)
		manager := NewTokenManager(store, NewRandomSelectionStrategy(), logger, mockFactory, proxyTracker)

		client, err := manager.GetTokenClient("a")
		require.NoError(t, err)
		require.NotNil(t, client)
		assert.Len(t, mockFactory.calls, 1)
		assert.Nil(t, mockFactory.calls[0])
	})

	t.Run("returns client for token with proxy", func(t *testing.T) {
		store := NewMultiTokenStore("test-provider", filePath, logger)
		tokens := createTestTokens(1)
		tokens[0].Proxy = &ProxyConfig{
			Type:    ProxyTypeSOCKS5,
			Host:    "socks.example.com",
			Port:    1080,
			Enabled: true,
		}
		for _, token := range tokens {
			require.NoError(t, store.AddToken(token))
		}

		mockFactory := &mockProxyClientFactory{}
		proxyTracker := NewProxyHealthTracker(logger, 5, 5*time.Minute)
		manager := NewTokenManager(store, NewRandomSelectionStrategy(), logger, mockFactory, proxyTracker)

		client, err := manager.GetTokenClient("a")
		require.NoError(t, err)
		require.NotNil(t, client)
		assert.Len(t, mockFactory.calls, 1)
		assert.Equal(t, tokens[0].Proxy, mockFactory.calls[0])
	})

	t.Run("returns error for non-existent token", func(t *testing.T) {
		store := NewMultiTokenStore("test-provider", filePath, logger)
		mockFactory := &mockProxyClientFactory{}
		proxyTracker := NewProxyHealthTracker(logger, 5, 5*time.Minute)
		manager := NewTokenManager(store, NewRandomSelectionStrategy(), logger, mockFactory, proxyTracker)

		_, err := manager.GetTokenClient("non-existent")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("returns error when client factory is nil", func(t *testing.T) {
		store := NewMultiTokenStore("test-provider", filePath, logger)
		tokens := createTestTokens(1)
		for _, token := range tokens {
			require.NoError(t, store.AddToken(token))
		}

		proxyTracker := NewProxyHealthTracker(logger, 5, 5*time.Minute)
		manager := NewTokenManager(store, NewRandomSelectionStrategy(), logger, nil, proxyTracker)

		_, err := manager.GetTokenClient("a")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "client factory not set")
	})
}

func TestTokenManager_UpdateProxyHealth(t *testing.T) {
	logger := logging.NewLogger()
	tempDir := t.TempDir()
	filePath := tempDir + "/store.json"

	t.Run("updates proxy health to healthy", func(t *testing.T) {
		store := NewMultiTokenStore("test-provider", filePath, logger)
		tokens := createTestTokens(1)
		for _, token := range tokens {
			require.NoError(t, store.AddToken(token))
		}

		proxyTracker := NewProxyHealthTracker(logger, 5, 5*time.Minute)
		manager := NewTokenManager(store, NewRandomSelectionStrategy(), logger, nil, proxyTracker)

		err := manager.UpdateProxyHealth("a", true, nil)
		require.NoError(t, err)

		healthScore := proxyTracker.GetHealthScore("a")
		assert.Equal(t, 1.0, healthScore)
	})

	t.Run("updates proxy health to unhealthy", func(t *testing.T) {
		store := NewMultiTokenStore("test-provider", filePath, logger)
		tokens := createTestTokens(1)
		for _, token := range tokens {
			require.NoError(t, store.AddToken(token))
		}

		proxyTracker := NewProxyHealthTracker(logger, 5, 5*time.Minute)
		manager := NewTokenManager(store, NewRandomSelectionStrategy(), logger, nil, proxyTracker)

		testErr := errors.New("proxy connection failed")
		err := manager.UpdateProxyHealth("a", false, testErr)
		require.NoError(t, err)

		healthScore := proxyTracker.GetHealthScore("a")
		assert.Less(t, healthScore, 1.0)
	})

	t.Run("updates token ProxyHealthScore in store", func(t *testing.T) {
		store := NewMultiTokenStore("test-provider", filePath, logger)
		tokens := createTestTokens(1)
		for _, token := range tokens {
			require.NoError(t, store.AddToken(token))
		}

		proxyTracker := NewProxyHealthTracker(logger, 5, 5*time.Minute)
		manager := NewTokenManager(store, NewRandomSelectionStrategy(), logger, nil, proxyTracker)

		err := manager.UpdateProxyHealth("a", false, errors.New("test error"))
		require.NoError(t, err)

		retrieved, err := store.GetToken("a")
		require.NoError(t, err)
		assert.Less(t, retrieved.ProxyHealthScore, 1.0)
	})

	t.Run("returns error for non-existent token", func(t *testing.T) {
		store := NewMultiTokenStore("test-provider", filePath, logger)
		proxyTracker := NewProxyHealthTracker(logger, 5, 5*time.Minute)
		manager := NewTokenManager(store, NewRandomSelectionStrategy(), logger, nil, proxyTracker)

		err := manager.UpdateProxyHealth("non-existent", true, nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("returns error when proxy health tracker is nil", func(t *testing.T) {
		store := NewMultiTokenStore("test-provider", filePath, logger)
		tokens := createTestTokens(1)
		for _, token := range tokens {
			require.NoError(t, store.AddToken(token))
		}

		manager := NewTokenManager(store, NewRandomSelectionStrategy(), logger, nil, nil)

		err := manager.UpdateProxyHealth("a", true, nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "proxy health tracker not set")
	})
}
