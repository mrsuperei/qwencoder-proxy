package ratelimit

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/sunbankio/qwencoder-proxy/internal/token"
	_ "modernc.org/sqlite"
)

// mockLogger is a simple mock logger for testing
type mockLogger struct{}

func (m *mockLogger) DebugLog(msg string, args ...interface{}) {}
func (m *mockLogger) InfoLog(msg string, args ...interface{})  {}
func (m *mockLogger) WarnLog(msg string, args ...interface{})  {}
func (m *mockLogger) ErrorLog(msg string, args ...interface{}) {}

// setupTestDB creates an in-memory SQLite database for testing
func setupTestDB(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}

	// Create tokens table
	_, err = db.Exec(`
		CREATE TABLE tokens (
			id TEXT PRIMARY KEY,
			provider_id TEXT NOT NULL,
			access_token TEXT NOT NULL,
			refresh_token TEXT,
			token_type TEXT NOT NULL,
			expiry_date INTEGER NOT NULL,
			email TEXT,
			resource_url TEXT,
			scope TEXT,
			api_key TEXT,
			project_id TEXT,
			healthy INTEGER DEFAULT 1,
			health_score REAL DEFAULT 1.0,
			last_used INTEGER NOT NULL,
			created_at INTEGER NOT NULL,
			error_count INTEGER DEFAULT 0,
			last_error TEXT,
			proxy_id TEXT
		)
	`)
	if err != nil {
		t.Fatalf("Failed to create tokens table: %v", err)
	}

	// Create token_usage table
	_, err = db.Exec(`
		CREATE TABLE token_usage (
			provider_id TEXT NOT NULL,
			token_id TEXT NOT NULL,
			requests_today INTEGER DEFAULT 0,
			requests_in_minute INTEGER DEFAULT 0,
			tokens_in_minute INTEGER DEFAULT 0,
			window_start INTEGER NOT NULL,
			day_start INTEGER NOT NULL,
			PRIMARY KEY (provider_id, token_id)
		)
	`)
	if err != nil {
		t.Fatalf("Failed to create token_usage table: %v", err)
	}

	return db
}

// insertTestToken inserts a test token into the database
func insertTestToken(t *testing.T, db *sql.DB, providerID string, tok *token.ProviderToken) {
	now := time.Now()
	expiry := now.Add(24 * time.Hour)

	_, err := db.Exec(`
		INSERT INTO tokens (
			id, provider_id, access_token, refresh_token, token_type, expiry_date,
			email, resource_url, scope, api_key, project_id,
			healthy, health_score, last_used, created_at, error_count, last_error, proxy_id
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		tok.ID, providerID, tok.AccessToken, tok.RefreshToken, tok.TokenType, expiry.UnixMilli(),
		tok.Email, tok.ResourceURL, tok.Scope, tok.APIKey, tok.ProjectID,
		1, 1.0, now.UnixMilli(), now.UnixMilli(), 0, "", nil,
	)
	if err != nil {
		t.Fatalf("Failed to insert test token: %v", err)
	}
}

// setupTestQuotaManager creates a test quota manager
func setupTestQuotaManager(t *testing.T, db *sql.DB) *QuotaManager {
	logger := &mockLogger{}
	// Use the shared database connection for the usage tracker
	usageTracker, err := NewUsageTracker(db, logger)
	if err != nil {
		t.Fatalf("Failed to create usage tracker: %v", err)
	}
	return NewQuotaManager(usageTracker, logger, db, nil, nil, nil)
}

// TestLoadBalancingStrategyInterface tests that all strategies implement the interface
func TestLoadBalancingStrategyInterface(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	logger := &mockLogger{}
	qm := setupTestQuotaManager(t, db)

	strategies := []LoadBalancingStrategy{
		NewWeightedRoundRobinSelector(db, qm, logger),
		NewLeastConnectionsSelector(db, qm, logger),
		NewAdaptiveSelector(db, qm, logger),
	}

	for _, strategy := range strategies {
		t.Run(strategy.Name(), func(t *testing.T) {
			// Verify Name() returns a non-empty string
			if strategy.Name() == "" {
				t.Error("Strategy name should not be empty")
			}

			// Verify SelectToken exists and returns appropriate error for no tokens
			ctx := context.Background()
			_, err := strategy.SelectToken(ctx, "nonexistent_provider")
			if err == nil {
				t.Error("Expected error when no tokens available")
			}
			if err != token.ErrNoValidTokens {
				t.Errorf("Expected ErrNoValidTokens, got %v", err)
			}
		})
	}
}

// TestWeightedRoundRobinStrategy tests the weighted round-robin strategy
func TestWeightedRoundRobinStrategy(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	logger := &mockLogger{}
	qm := setupTestQuotaManager(t, db)
	selector := NewWeightedRoundRobinSelector(db, qm, logger)

	providerID := "gemini-cli"

	// Insert test tokens with different quota statuses
	tokens := []*token.ProviderToken{
		{ID: "token1", AccessToken: "access1", TokenType: "Bearer"},
		{ID: "token2", AccessToken: "access2", TokenType: "Bearer"},
		{ID: "token3", AccessToken: "access3", TokenType: "Bearer"},
	}

	for _, tok := range tokens {
		insertTestToken(t, db, providerID, tok)
	}

	// Set up quota status for the tokens in the usage tracker
	// This is needed because the usage tracker uses a separate database
	now := time.Now()
	for _, tok := range tokens {
		_, err := db.Exec(`
			INSERT INTO token_usage (provider_id, token_id, requests_today, requests_in_minute, tokens_in_minute, window_start, day_start)
			VALUES (?, ?, 100, 50, 5000, ?, ?)
		`, providerID, tok.ID, now.UnixMilli(), now.UnixMilli())
		if err != nil {
			t.Fatalf("Failed to set usage for %s: %v", tok.ID, err)
		}
	}

	ctx := context.Background()

	// Select multiple tokens and verify round-robin behavior
	selectedTokens := make(map[string]int)
	for i := 0; i < 10; i++ {
		selected, err := selector.SelectToken(ctx, providerID)
		if err != nil {
			t.Fatalf("Failed to select token: %v", err)
		}
		selectedTokens[selected.ID]++
	}

	// All tokens should have been selected at least once
	for _, tok := range tokens {
		if selectedTokens[tok.ID] == 0 {
			t.Errorf("Token %s was never selected", tok.ID)
		}
	}

	t.Logf("Token selection distribution: %v", selectedTokens)
}

// TestLeastConnectionsStrategy tests the least connections strategy
func TestLeastConnectionsStrategy(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	logger := &mockLogger{}
	qm := setupTestQuotaManager(t, db)
	selector := NewLeastConnectionsSelector(db, qm, logger)

	providerID := "test_provider"

	// Insert test tokens
	tokens := []*token.ProviderToken{
		{ID: "token1", AccessToken: "access1", TokenType: "Bearer"},
		{ID: "token2", AccessToken: "access2", TokenType: "Bearer"},
		{ID: "token3", AccessToken: "access3", TokenType: "Bearer"},
	}

	for _, tok := range tokens {
		insertTestToken(t, db, providerID, tok)
	}

	// Set different connection counts for tokens
	now := time.Now()

	// Token 1: 5 connections
	_, err := db.Exec(`
		INSERT INTO token_usage (provider_id, token_id, requests_today, requests_in_minute, tokens_in_minute, window_start, day_start)
		VALUES (?, ?, 0, 5, 0, ?, ?)
	`, providerID, "token1", now.UnixMilli(), now.UnixMilli())
	if err != nil {
		t.Fatalf("Failed to set usage for token1: %v", err)
	}

	// Token 2: 10 connections
	_, err = db.Exec(`
		INSERT INTO token_usage (provider_id, token_id, requests_today, requests_in_minute, tokens_in_minute, window_start, day_start)
		VALUES (?, ?, 0, 10, 0, ?, ?)
	`, providerID, "token2", now.UnixMilli(), now.UnixMilli())
	if err != nil {
		t.Fatalf("Failed to set usage for token2: %v", err)
	}

	// Token 3: 0 connections (default)
	// No entry means 0 connections

	ctx := context.Background()

	// Select token - should select token3 (0 connections)
	selected, err := selector.SelectToken(ctx, providerID)
	if err != nil {
		t.Fatalf("Failed to select token: %v", err)
	}

	if selected.ID != "token3" {
		t.Errorf("Expected token3 to be selected (0 connections), got %s", selected.ID)
	}
}

// TestAdaptiveStrategy tests the adaptive strategy
func TestAdaptiveStrategy(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	logger := &mockLogger{}
	qm := setupTestQuotaManager(t, db)
	selector := NewAdaptiveSelector(db, qm, logger)

	providerID := "test_provider"

	// Insert test tokens with different health scores and error counts
	tokens := []*token.ProviderToken{
		{ID: "token1", AccessToken: "access1", TokenType: "Bearer"},
		{ID: "token2", AccessToken: "access2", TokenType: "Bearer"},
		{ID: "token3", AccessToken: "access3", TokenType: "Bearer"},
	}

	for _, tok := range tokens {
		insertTestToken(t, db, providerID, tok)
	}

	// Set different health scores and error counts
	_, err := db.Exec(`UPDATE tokens SET health_score = 0.9, error_count = 0 WHERE id = 'token1'`)
	if err != nil {
		t.Fatalf("Failed to update token1: %v", err)
	}

	_, err = db.Exec(`UPDATE tokens SET health_score = 0.5, error_count = 5 WHERE id = 'token2'`)
	if err != nil {
		t.Fatalf("Failed to update token2: %v", err)
	}

	_, err = db.Exec(`UPDATE tokens SET health_score = 1.0, error_count = 0 WHERE id = 'token3'`)
	if err != nil {
		t.Fatalf("Failed to update token3: %v", err)
	}

	ctx := context.Background()

	// Select token - should prefer token3 (highest health, no errors)
	selected, err := selector.SelectToken(ctx, providerID)
	if err != nil {
		t.Fatalf("Failed to select token: %v", err)
	}

	// Token3 should be selected due to best health and no errors
	if selected.ID != "token3" {
		t.Errorf("Expected token3 to be selected (best health, no errors), got %s", selected.ID)
	}
}

// TestSetLoadBalancingStrategy tests strategy switching
func TestSetLoadBalancingStrategy(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	qm := setupTestQuotaManager(t, db)

	// Test switching to each strategy
	strategies := []string{"weighted_round_robin", "least_connections", "adaptive"}

	for _, strategyName := range strategies {
		err := qm.SetLoadBalancingStrategy(strategyName)
		if err != nil {
			t.Errorf("Failed to set strategy %s: %v", strategyName, err)
		}

		// Verify the strategy was set
		current := qm.GetLoadBalancingStrategy()
		if current != strategyName {
			t.Errorf("Expected strategy %s, got %s", strategyName, current)
		}
	}

	// Test invalid strategy
	err := qm.SetLoadBalancingStrategy("invalid_strategy")
	if err == nil {
		t.Error("Expected error for invalid strategy")
	}
}

// TestRateAwareTokenSelectorDelegation tests that RateAwareTokenSelector delegates to strategy
func TestRateAwareTokenSelectorDelegation(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	logger := &mockLogger{}
	qm := setupTestQuotaManager(t, db)
	selector := NewRateAwareTokenSelector(db, qm, logger, NewAdaptiveSelector(db, qm, logger))

	providerID := "test_provider"

	// Insert test token
	tok := &token.ProviderToken{
		ID:          "token1",
		AccessToken: "access1",
		TokenType:   "Bearer",
	}
	insertTestToken(t, db, providerID, tok)

	ctx := context.Background()

	// Select token
	selected, err := selector.SelectToken(ctx, providerID)
	if err != nil {
		t.Fatalf("Failed to select token: %v", err)
	}

	if selected.ID != "token1" {
		t.Errorf("Expected token1, got %s", selected.ID)
	}

	// Verify GetStrategy returns correct name
	if selector.GetStrategy() != "adaptive" {
		t.Errorf("Expected strategy name 'adaptive', got %s", selector.GetStrategy())
	}
}

// TestRateAwareTokenSelectorSetStrategy tests changing strategies
func TestRateAwareTokenSelectorSetStrategy(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	logger := &mockLogger{}
	qm := setupTestQuotaManager(t, db)

	// Create selector with initial strategy
	selector := NewRateAwareTokenSelector(db, qm, logger, NewAdaptiveSelector(db, qm, logger))

	// Test changing to different strategies
	newStrategies := []LoadBalancingStrategy{
		NewWeightedRoundRobinSelector(db, qm, logger),
		NewLeastConnectionsSelector(db, qm, logger),
		NewAdaptiveSelector(db, qm, logger),
	}

	for _, newStrategy := range newStrategies {
		selector.SetStrategy(newStrategy)
		if selector.GetStrategy() != newStrategy.Name() {
			t.Errorf("Expected strategy %s, got %s", newStrategy.Name(), selector.GetStrategy())
		}
	}
}
