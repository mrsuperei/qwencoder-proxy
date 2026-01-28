// Package auth provides authentication and token management functionality.
package auth

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/sunbankio/qwencoder-proxy/logging"
)

// TestProviderTokenWithProxyConfig tests that ProviderToken can be marshaled/unmarshaled with proxy configuration.
func TestProviderTokenWithProxyConfig(t *testing.T) {
	proxyConfig := &ProxyConfig{
		Type:     ProxyTypeHTTP,
		Host:     "proxy.example.com",
		Port:     8080,
		Username: "user",
		Password: "pass",
		Enabled:  true,
	}

	token := ProviderToken{
		ID:               "test-token-id",
		AccessToken:      "test-access-token",
		RefreshToken:     "test-refresh-token",
		TokenType:        "Bearer",
		ExpiryDate:       1234567890,
		Email:            "test@example.com",
		Healthy:          true,
		HealthScore:      1.0,
		LastUsed:         1234567890,
		CreatedAt:        1234567890,
		ErrorCount:       0,
		Proxy:            proxyConfig,
		ProxyHealthScore: 0.95,
	}

	// Test JSON marshaling
	data, err := json.Marshal(token)
	if err != nil {
		t.Fatalf("Failed to marshal token: %v", err)
	}

	// Test JSON unmarshaling
	var unmarshaled ProviderToken
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal token: %v", err)
	}

	// Verify fields match
	if unmarshaled.ID != token.ID {
		t.Errorf("Expected ID %s, got %s", token.ID, unmarshaled.ID)
	}

	if unmarshaled.Proxy == nil {
		t.Error("Expected Proxy to not be nil")
	} else {
		if unmarshaled.Proxy.Type != token.Proxy.Type {
			t.Errorf("Expected Proxy.Type %s, got %s", token.Proxy.Type, unmarshaled.Proxy.Type)
		}
		if unmarshaled.Proxy.Host != token.Proxy.Host {
			t.Errorf("Expected Proxy.Host %s, got %s", token.Proxy.Host, unmarshaled.Proxy.Host)
		}
		if unmarshaled.Proxy.Port != token.Proxy.Port {
			t.Errorf("Expected Proxy.Port %d, got %d", token.Proxy.Port, unmarshaled.Proxy.Port)
		}
		if unmarshaled.Proxy.Username != token.Proxy.Username {
			t.Errorf("Expected Proxy.Username %s, got %s", token.Proxy.Username, unmarshaled.Proxy.Username)
		}
		if unmarshaled.Proxy.Password != token.Proxy.Password {
			t.Errorf("Expected Proxy.Password %s, got %s", token.Proxy.Password, unmarshaled.Proxy.Password)
		}
	}

	if unmarshaled.ProxyHealthScore != token.ProxyHealthScore {
		t.Errorf("Expected ProxyHealthScore %f, got %f", token.ProxyHealthScore, unmarshaled.ProxyHealthScore)
	}
}

// TestProviderTokenBackwardCompatibility tests that tokens without proxy fields can be loaded.
func TestProviderTokenBackwardCompatibility(t *testing.T) {
	// Old token JSON without proxy fields
	oldTokenJSON := `{
		"id": "test-token-id",
		"access_token": "test-access-token",
		"refresh_token": "test-refresh-token",
		"token_type": "Bearer",
		"expiry_date": 1234567890,
		"email": "test@example.com",
		"healthy": true,
		"health_score": 1.0,
		"last_used": 1234567890,
		"created_at": 1234567890,
		"error_count": 0
	}`

	var token ProviderToken
	if err := json.Unmarshal([]byte(oldTokenJSON), &token); err != nil {
		t.Fatalf("Failed to unmarshal old token: %v", err)
	}

	// Verify token loaded correctly
	if token.ID != "test-token-id" {
		t.Errorf("Expected ID test-token-id, got %s", token.ID)
	}

	// Proxy should be nil for backward compatibility
	if token.Proxy != nil {
		t.Error("Expected Proxy to be nil for old token format")
	}

	// ProxyHealthScore should default to 0 (omitempty means it won't be in JSON)
	if token.ProxyHealthScore != 0 {
		t.Errorf("Expected ProxyHealthScore to be 0 (not set), got %f", token.ProxyHealthScore)
	}
}

// TestAddTokenWithProxyConfig tests adding a token with proxy configuration.
func TestAddTokenWithProxyConfig(t *testing.T) {
	// Create temporary directory for testing
	tempDir := t.TempDir()
	providerDir := filepath.Join(tempDir, "test-provider")
	if err := os.MkdirAll(providerDir, 0755); err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}

	logger := logging.NewLogger()
	store := NewMultiTokenStore("test-provider", filepath.Join(tempDir, "test.json"), logger)
	store.providerDir = providerDir

	proxyConfig := &ProxyConfig{
		Type:    ProxyTypeHTTP,
		Host:    "proxy.example.com",
		Port:    8080,
		Enabled: true,
	}

	token := ProviderToken{
		ID:               "test-token-id",
		AccessToken:      "test-access-token",
		RefreshToken:     "test-refresh-token",
		TokenType:        "Bearer",
		ExpiryDate:       1234567890,
		Email:            "test@example.com",
		Healthy:          true,
		HealthScore:      1.0,
		LastUsed:         GetCurrentTimestamp(),
		CreatedAt:        GetCurrentTimestamp(),
		ErrorCount:       0,
		Proxy:            proxyConfig,
		ProxyHealthScore: 0.95,
	}

	// Add token
	if err := store.AddToken(token); err != nil {
		t.Fatalf("Failed to add token: %v", err)
	}

	// Verify token was added
	retrievedToken, err := store.GetToken("test-token-id")
	if err != nil {
		t.Fatalf("Failed to retrieve token: %v", err)
	}

	if retrievedToken.Proxy == nil {
		t.Error("Expected Proxy to not be nil")
	} else {
		if retrievedToken.Proxy.Type != proxyConfig.Type {
			t.Errorf("Expected Proxy.Type %s, got %s", proxyConfig.Type, retrievedToken.Proxy.Type)
		}
		if retrievedToken.Proxy.Host != proxyConfig.Host {
			t.Errorf("Expected Proxy.Host %s, got %s", proxyConfig.Host, retrievedToken.Proxy.Host)
		}
		if retrievedToken.Proxy.Port != proxyConfig.Port {
			t.Errorf("Expected Proxy.Port %d, got %d", proxyConfig.Port, retrievedToken.Proxy.Port)
		}
	}

	if retrievedToken.ProxyHealthScore != 0.95 {
		t.Errorf("Expected ProxyHealthScore 0.95, got %f", retrievedToken.ProxyHealthScore)
	}
}

// TestAddTokenWithInvalidProxyConfig tests that invalid proxy configuration is rejected.
func TestAddTokenWithInvalidProxyConfig(t *testing.T) {
	// Create temporary directory for testing
	tempDir := t.TempDir()
	providerDir := filepath.Join(tempDir, "test-provider")
	if err := os.MkdirAll(providerDir, 0755); err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}

	logger := logging.NewLogger()
	store := NewMultiTokenStore("test-provider", filepath.Join(tempDir, "test.json"), logger)
	store.providerDir = providerDir

	// Invalid proxy config (empty host)
	proxyConfig := &ProxyConfig{
		Type:    ProxyTypeHTTP,
		Host:    "", // Invalid: empty host
		Port:    8080,
		Enabled: true,
	}

	token := ProviderToken{
		ID:           "test-token-id",
		AccessToken:  "test-access-token",
		RefreshToken: "test-refresh-token",
		TokenType:    "Bearer",
		ExpiryDate:   1234567890,
		Email:        "test@example.com",
		Healthy:      true,
		HealthScore:  1.0,
		LastUsed:     GetCurrentTimestamp(),
		CreatedAt:    GetCurrentTimestamp(),
		ErrorCount:   0,
		Proxy:        proxyConfig,
	}

	// Add token should fail due to invalid proxy config
	err := store.AddToken(token)
	if err == nil {
		t.Error("Expected error when adding token with invalid proxy config")
	}

	// Verify error message contains "invalid proxy configuration"
	if err != nil && !containsString(err.Error(), "invalid proxy configuration") {
		t.Errorf("Expected error about invalid proxy configuration, got: %v", err)
	}
}

// TestAddTokenWithNilProxyConfig tests that nil proxy config is accepted.
func TestAddTokenWithNilProxyConfig(t *testing.T) {
	// Create temporary directory for testing
	tempDir := t.TempDir()
	providerDir := filepath.Join(tempDir, "test-provider")
	if err := os.MkdirAll(providerDir, 0755); err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}

	logger := logging.NewLogger()
	store := NewMultiTokenStore("test-provider", filepath.Join(tempDir, "test.json"), logger)
	store.providerDir = providerDir

	token := ProviderToken{
		ID:           "test-token-id",
		AccessToken:  "test-access-token",
		RefreshToken: "test-refresh-token",
		TokenType:    "Bearer",
		ExpiryDate:   1234567890,
		Email:        "test@example.com",
		Healthy:      true,
		HealthScore:  1.0,
		LastUsed:     GetCurrentTimestamp(),
		CreatedAt:    GetCurrentTimestamp(),
		ErrorCount:   0,
		Proxy:        nil, // No proxy
	}

	// Add token should succeed
	if err := store.AddToken(token); err != nil {
		t.Fatalf("Failed to add token with nil proxy config: %v", err)
	}

	// Verify token was added
	retrievedToken, err := store.GetToken("test-token-id")
	if err != nil {
		t.Fatalf("Failed to retrieve token: %v", err)
	}

	if retrievedToken.Proxy != nil {
		t.Error("Expected Proxy to be nil")
	}

	// ProxyHealthScore should default to 1.0
	if retrievedToken.ProxyHealthScore != 1.0 {
		t.Errorf("Expected ProxyHealthScore to default to 1.0, got %f", retrievedToken.ProxyHealthScore)
	}
}

// TestAddTokenWithDefaultProxyHealthScore tests that default proxy health score is set.
func TestAddTokenWithDefaultProxyHealthScore(t *testing.T) {
	// Create temporary directory for testing
	tempDir := t.TempDir()
	providerDir := filepath.Join(tempDir, "test-provider")
	if err := os.MkdirAll(providerDir, 0755); err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}

	logger := logging.NewLogger()
	store := NewMultiTokenStore("test-provider", filepath.Join(tempDir, "test.json"), logger)
	store.providerDir = providerDir

	proxyConfig := &ProxyConfig{
		Type:    ProxyTypeHTTP,
		Host:    "proxy.example.com",
		Port:    8080,
		Enabled: true,
	}

	token := ProviderToken{
		ID:           "test-token-id",
		AccessToken:  "test-access-token",
		RefreshToken: "test-refresh-token",
		TokenType:    "Bearer",
		ExpiryDate:   1234567890,
		Email:        "test@example.com",
		Healthy:      true,
		HealthScore:  1.0,
		LastUsed:     GetCurrentTimestamp(),
		CreatedAt:    GetCurrentTimestamp(),
		ErrorCount:   0,
		Proxy:        proxyConfig,
		// ProxyHealthScore not set (0)
	}

	// Add token
	if err := store.AddToken(token); err != nil {
		t.Fatalf("Failed to add token: %v", err)
	}

	// Verify default ProxyHealthScore was set
	retrievedToken, err := store.GetToken("test-token-id")
	if err != nil {
		t.Fatalf("Failed to retrieve token: %v", err)
	}

	if retrievedToken.ProxyHealthScore != 1.0 {
		t.Errorf("Expected ProxyHealthScore to default to 1.0, got %f", retrievedToken.ProxyHealthScore)
	}
}

// TestMarkTokenHealthyResetsProxyHealthScore tests that MarkTokenHealthy resets proxy health score.
func TestMarkTokenHealthyResetsProxyHealthScore(t *testing.T) {
	// Create temporary directory for testing
	tempDir := t.TempDir()
	providerDir := filepath.Join(tempDir, "test-provider")
	if err := os.MkdirAll(providerDir, 0755); err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}

	logger := logging.NewLogger()
	store := NewMultiTokenStore("test-provider", filepath.Join(tempDir, "test.json"), logger)
	store.providerDir = providerDir

	proxyConfig := &ProxyConfig{
		Type:    ProxyTypeHTTP,
		Host:    "proxy.example.com",
		Port:    8080,
		Enabled: true,
	}

	token := ProviderToken{
		ID:               "test-token-id",
		AccessToken:      "test-access-token",
		RefreshToken:     "test-refresh-token",
		TokenType:        "Bearer",
		ExpiryDate:       1234567890,
		Email:            "test@example.com",
		Healthy:          false,
		HealthScore:      0.5,
		LastUsed:         GetCurrentTimestamp(),
		CreatedAt:        GetCurrentTimestamp(),
		ErrorCount:       5,
		Proxy:            proxyConfig,
		ProxyHealthScore: 0.3, // Low proxy health score
	}

	// Add token
	if err := store.AddToken(token); err != nil {
		t.Fatalf("Failed to add token: %v", err)
	}

	// Mark token as healthy
	if err := store.MarkTokenHealthy("test-token-id"); err != nil {
		t.Fatalf("Failed to mark token as healthy: %v", err)
	}

	// Verify both health scores were reset
	retrievedToken, err := store.GetToken("test-token-id")
	if err != nil {
		t.Fatalf("Failed to retrieve token: %v", err)
	}

	if !retrievedToken.Healthy {
		t.Error("Expected Healthy to be true")
	}

	if retrievedToken.HealthScore != 1.0 {
		t.Errorf("Expected HealthScore to be 1.0, got %f", retrievedToken.HealthScore)
	}

	if retrievedToken.ProxyHealthScore != 1.0 {
		t.Errorf("Expected ProxyHealthScore to be 1.0, got %f", retrievedToken.ProxyHealthScore)
	}
}

// TestSaveAndLoadTokenWithProxyConfig tests saving and loading a token with proxy configuration.
func TestSaveAndLoadTokenWithProxyConfig(t *testing.T) {
	// Create temporary directory for testing
	tempDir := t.TempDir()
	providerDir := filepath.Join(tempDir, "test-provider")
	if err := os.MkdirAll(providerDir, 0755); err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}

	logger := logging.NewLogger()
	store := NewMultiTokenStore("test-provider", filepath.Join(tempDir, "test.json"), logger)
	store.providerDir = providerDir

	proxyConfig := &ProxyConfig{
		Type:     ProxyTypeSOCKS5,
		Host:     "socks.example.com",
		Port:     1080,
		Username: "socksuser",
		Password: "sockspass",
		Enabled:  true,
	}

	token := ProviderToken{
		ID:               "test-token-id",
		AccessToken:      "test-access-token",
		RefreshToken:     "test-refresh-token",
		TokenType:        "Bearer",
		ExpiryDate:       1234567890,
		Email:            "test@example.com",
		Healthy:          true,
		HealthScore:      1.0,
		LastUsed:         GetCurrentTimestamp(),
		CreatedAt:        GetCurrentTimestamp(),
		ErrorCount:       0,
		Proxy:            proxyConfig,
		ProxyHealthScore: 0.85,
	}

	// Add and save token
	if err := store.AddToken(token); err != nil {
		t.Fatalf("Failed to add token: %v", err)
	}

	// Create a new store and load tokens
	newStore := NewMultiTokenStore("test-provider", filepath.Join(tempDir, "test.json"), logger)
	newStore.providerDir = providerDir

	if err := newStore.Load(); err != nil {
		t.Fatalf("Failed to load store: %v", err)
	}

	// Verify token was loaded correctly
	retrievedToken, err := newStore.GetToken("test-token-id")
	if err != nil {
		t.Fatalf("Failed to retrieve token: %v", err)
	}

	if retrievedToken.Proxy == nil {
		t.Fatal("Expected Proxy to not be nil")
	}

	if retrievedToken.Proxy.Type != proxyConfig.Type {
		t.Errorf("Expected Proxy.Type %s, got %s", proxyConfig.Type, retrievedToken.Proxy.Type)
	}

	if retrievedToken.Proxy.Host != proxyConfig.Host {
		t.Errorf("Expected Proxy.Host %s, got %s", proxyConfig.Host, retrievedToken.Proxy.Host)
	}

	if retrievedToken.Proxy.Port != proxyConfig.Port {
		t.Errorf("Expected Proxy.Port %d, got %d", proxyConfig.Port, retrievedToken.Proxy.Port)
	}

	if retrievedToken.Proxy.Username != proxyConfig.Username {
		t.Errorf("Expected Proxy.Username %s, got %s", proxyConfig.Username, retrievedToken.Proxy.Username)
	}

	if retrievedToken.Proxy.Password != proxyConfig.Password {
		t.Errorf("Expected Proxy.Password %s, got %s", proxyConfig.Password, retrievedToken.Proxy.Password)
	}

	if retrievedToken.ProxyHealthScore != 0.85 {
		t.Errorf("Expected ProxyHealthScore 0.85, got %f", retrievedToken.ProxyHealthScore)
	}
}

// Helper function to check if a string contains a substring
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && containsStringHelper(s, substr))
}

func containsStringHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
