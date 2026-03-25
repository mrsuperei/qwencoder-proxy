// Package antigravity provides authentication for Antigravity provider
package antigravity

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/sunbankio/qwencoder-proxy/internal/logging"
	"github.com/sunbankio/qwencoder-proxy/internal/provider/gemini"
	tokenpkg "github.com/sunbankio/qwencoder-proxy/internal/token"
)

// OAuthConfig holds OAuth configuration for Antigravity
// Antigravity uses Google OAuth with different client credentials
type OAuthConfig struct {
	ClientID     string
	ClientSecret string
	Scope        string
	RedirectPort int
	CredsDir     string
	CredsFile    string
}

// DefaultOAuthConfig returns default Antigravity OAuth configuration
func DefaultOAuthConfig() *OAuthConfig {
	return &OAuthConfig{
		ClientID:     "1071006060591-tmhssin2h21lcre235vtolojh4g403ep.apps.googleusercontent.com",
		ClientSecret: "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",
		Scope:        "https://www.googleapis.com/auth/cloud-platform",
		RedirectPort: 8086,
		CredsDir:     ".antigravity",
		CredsFile:    "oauth_creds.json",
	}
}

// Authenticator wraps Gemini authenticator for Antigravity
// Antigravity uses the same Google OAuth flow as Gemini CLI
type Authenticator struct {
	*gemini.Authenticator
}

// NewAuthenticator creates a new Antigravity authenticator
func NewAuthenticator(config *OAuthConfig) *Authenticator {
	if config == nil {
		config = DefaultOAuthConfig()
	}

	// Convert Antigravity config to Gemini config
	geminiConfig := &gemini.OAuthConfig{
		ClientID:     config.ClientID,
		ClientSecret: config.ClientSecret,
		Scope:        config.Scope,
		RedirectPort: config.RedirectPort,
		CredsDir:     config.CredsDir,
		CredsFile:    config.CredsFile,
	}

	return &Authenticator{
		Authenticator: gemini.NewAuthenticator(geminiConfig),
	}
}

// antigravityTokenRefresher implements ProviderRefresh for Antigravity
// Antigravity uses the same Google OAuth flow as Gemini
type antigravityTokenRefresher struct {
	clientID     string
	clientSecret string
	tokenURL     string
	logger       logging.Logger
}

// NewAntigravityTokenRefresher creates a new Antigravity token refresher
func NewAntigravityTokenRefresher(clientID, clientSecret, tokenURL string, logger logging.Logger) *antigravityTokenRefresher {
	return &antigravityTokenRefresher{
		clientID:     clientID,
		clientSecret: clientSecret,
		tokenURL:     tokenURL,
		logger:       logger,
	}
}

// ProviderID returns the provider identifier
func (a *antigravityTokenRefresher) ProviderID() string {
	return "antigravity"
}

// RefreshToken refreshes an Antigravity OAuth token
// Antigravity uses the same Google OAuth flow as Gemini
func (a *antigravityTokenRefresher) RefreshToken(ctx context.Context, token tokenpkg.ProviderToken) (tokenpkg.ProviderToken, error) {
	a.logger.InfoLog("[AntigravityRefresh] Starting token refresh for token ID %s", token.ID)

	if token.RefreshToken == "" {
		a.logger.ErrorLog("[AntigravityRefresh] No refresh token available for token ID %s", token.ID)
		return tokenpkg.ProviderToken{}, fmt.Errorf("no refresh token available")
	}

	// Prepare refresh request
	data := url.Values{}
	data.Set("client_id", a.clientID)
	data.Set("client_secret", a.clientSecret)
	data.Set("refresh_token", token.RefreshToken)
	data.Set("grant_type", "refresh_token")

	// Make HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", a.tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		a.logger.ErrorLog("[AntigravityRefresh] Failed to create refresh request for token ID %s: %v", token.ID, err)
		return tokenpkg.ProviderToken{}, fmt.Errorf("failed to create refresh request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		a.logger.ErrorLog("[AntigravityRefresh] Failed to send refresh request for token ID %s: %v", token.ID, err)
		return tokenpkg.ProviderToken{}, fmt.Errorf("failed to send refresh request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		a.logger.ErrorLog("[AntigravityRefresh] Refresh failed with status %d for token ID %s: %s", resp.StatusCode, token.ID, string(body))
		return tokenpkg.ProviderToken{}, fmt.Errorf("refresh failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		TokenType    string `json:"token_type"`
		ExpiresIn    int64  `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		a.logger.ErrorLog("[AntigravityRefresh] Failed to decode refresh response for token ID %s: %v", token.ID, err)
		return tokenpkg.ProviderToken{}, fmt.Errorf("failed to decode refresh response: %w", err)
	}

	// Calculate expiry
	expiryDate := time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	a.logger.InfoLog("[AntigravityRefresh] Token refresh successful for token ID %s, new expiry %s", token.ID, expiryDate.Format(time.RFC3339))

	// Return refreshed token
	return tokenpkg.ProviderToken{
		ID:           token.ID,
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		TokenType:    tokenResp.TokenType,
		ExpiryDate:   expiryDate.UnixMilli(),
		Email:        token.Email,
		Healthy:      true,
		HealthScore:  1.0,
		LastUsed:     token.LastUsed,
		CreatedAt:    token.CreatedAt,
	}, nil
}
