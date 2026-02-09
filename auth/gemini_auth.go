// Package auth provides authentication implementations for various providers
package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/sunbankio/qwencoder-proxy/logging"
)

// GeminiOAuthConfig holds OAuth configuration for Gemini
type GeminiOAuthConfig struct {
	ClientID     string
	ClientSecret string
	Scope        string
	RedirectPort int
	CredsDir     string
	CredsFile    string
}

// DefaultGeminiOAuthConfig returns default Gemini OAuth configuration
func DefaultGeminiOAuthConfig() *GeminiOAuthConfig {
	return &GeminiOAuthConfig{
		ClientID:     "681255809395-oo8ft2oprdrnp9e3aqf6av3hmdib135j.apps.googleusercontent.com",
		ClientSecret: "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",
		Scope:        "https://www.googleapis.com/auth/cloud-platform https://www.googleapis.com/auth/userinfo.email https://www.googleapis.com/auth/userinfo.profile openid",
		RedirectPort: 8085,
		CredsDir:     ".gemini",
		CredsFile:    "oauth_creds.json",
	}
}

// GeminiCredentials represents stored OAuth credentials
type GeminiCredentials struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiryDate   int64  `json:"expiry_date"`
	Scope        string `json:"scope,omitempty"`
}

// GeminiAuthenticator implements Authenticator interface for Gemini
// Embeds BaseAuthenticator for common functionality
type GeminiAuthenticator struct {
	*BaseAuthenticator // Embedded base authenticator provides common fields and methods
	config             *GeminiOAuthConfig
}

// NewGeminiAuthenticator creates a new Gemini authenticator
// Uses BaseAuthenticator for common functionality
func NewGeminiAuthenticator(config *GeminiOAuthConfig) *GeminiAuthenticator {
	if config == nil {
		config = DefaultGeminiOAuthConfig()
	}
	return &GeminiAuthenticator{
		BaseAuthenticator: NewBaseAuthenticator(logging.NewLogger()),
		config:            config,
	}
}

// GetCredentialsPath returns path to credentials file
func (a *GeminiAuthenticator) GetCredentialsPath() string {
	homeDir, _ := os.UserHomeDir()
	return filepath.Join(homeDir, a.config.CredsDir, a.config.CredsFile)
}

// IsAuthenticated checks if valid credentials exist
func (a *GeminiAuthenticator) IsAuthenticated() bool {
	tokenManager := a.GetTokenManager()
	if tokenManager == nil {
		return false
	}

	_, err := tokenManager.SelectToken()
	return err == nil
}

// loadCredentials loads credentials from file
func (a *GeminiAuthenticator) loadCredentials() (*GeminiCredentials, error) {
	credsPath := a.GetCredentialsPath()
	data, err := os.ReadFile(credsPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read credentials file: %w", err)
	}

	var creds GeminiCredentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return nil, fmt.Errorf("failed to parse credentials: %w", err)
	}

	return &creds, nil
}

// saveCredentials saves credentials to file
func (a *GeminiAuthenticator) saveCredentials(creds *GeminiCredentials) error {
	credsPath := a.GetCredentialsPath()

	// Create directory if it doesn't exist
	dir := filepath.Dir(credsPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create credentials directory: %w", err)
	}

	data, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal credentials: %w", err)
	}

	// Lock file write operation is handled by OS filesystem locking usually,
	// but here we ensure process-level safety via to caller holding a.mu.
	// For added safety we write to temp file and rename.
	// But sticking to os.WriteFile with 0600 is standard.
	if err := os.WriteFile(credsPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write credentials file: %w", err)
	}

	return nil
}

// ClearCredentials removes stored credentials (no-op for multi-token system)
func (a *GeminiAuthenticator) ClearCredentials() error {
	// Multi-token system manages credentials differently
	return nil
}

// GetToken returns a valid access token using multi-token manager
func (a *GeminiAuthenticator) GetToken(ctx context.Context) (string, error) {
	tokenManager := a.GetTokenManager()
	if tokenManager == nil {
		a.GetLogger().ErrorLog("[GeminiAuth] GetToken: token manager is nil")
		return "", fmt.Errorf("token manager not initialized")
	}

	a.GetLogger().DebugLog("[GeminiAuth] GetToken: calling tokenManager.SelectToken()")
	token, err := tokenManager.SelectToken()
	if err != nil {
		a.GetLogger().ErrorLog("[GeminiAuth] GetToken: tokenManager.SelectToken() failed: %v", err)
		return "", fmt.Errorf("failed to select token: %w", err)
	}

	a.GetLogger().DebugLog("[GeminiAuth] GetToken: selected token ID=%s, Email=%s", token.ID, token.Email)
	return token.AccessToken, nil
}

// GetTokenWithClient returns a valid access token and an HTTP client.
// The HTTP client is configured with the proxy settings from the selected token.
func (a *GeminiAuthenticator) GetTokenWithClient(ctx context.Context) (string, *http.Client, error) {
	tokenManager := a.GetTokenManager()
	if tokenManager == nil {
		token, err := a.GetToken(ctx)
		return token, nil, err
	}

	token, client, err := tokenManager.SelectTokenWithClient()
	if err != nil {
		return "", nil, fmt.Errorf("failed to select token with client: %w", err)
	}
	return token.AccessToken, client, nil
}

// GetHTTPClient returns an HTTP client configured with proxy settings.
// The client is configured with the proxy settings from the selected token.
func (a *GeminiAuthenticator) GetHTTPClient() (*http.Client, error) {
	tokenManager := a.GetTokenManager()
	if tokenManager == nil {
		return nil, errors.New("token manager not initialized")
	}

	token, client, err := tokenManager.SelectTokenWithClient()
	if err != nil {
		return nil, fmt.Errorf("failed to select token with client: %w", err)
	}
	_ = token // Token is not needed for GetHTTPClient
	return client, nil
}

// ForceRefresh forces a token refresh regardless of expiry
func (a *GeminiAuthenticator) ForceRefresh(ctx context.Context) error {
	// Multi-token system handles refresh via TokenManager
	// This is a no-op since TokenManager handles refresh internally
	return nil
}

// Authenticate performs OAuth web flow authentication
func (a *GeminiAuthenticator) Authenticate(ctx context.Context) error {
	// Generate state for CSRF protection
	stateBytes := make([]byte, 16)
	if _, err := rand.Read(stateBytes); err != nil {
		return fmt.Errorf("failed to generate state: %w", err)
	}
	state := base64.URLEncoding.EncodeToString(stateBytes)

	redirectURI := fmt.Sprintf("http://localhost:%d", a.config.RedirectPort)

	// Build authorization URL
	authURL := fmt.Sprintf(
		"https://accounts.google.com/o/oauth2/v2/auth?client_id=%s&redirect_uri=%s&response_type=code&scope=%s&access_type=offline&prompt=consent&state=%s",
		url.QueryEscape(a.config.ClientID),
		url.QueryEscape(redirectURI),
		url.QueryEscape(a.config.Scope),
		url.QueryEscape(state),
	)

	// Channel to receive authorization code
	codeChan := make(chan string, 1)
	errChan := make(chan error, 1)

	// Start local server to receive callback
	server := &http.Server{Addr: fmt.Sprintf(":%d", a.config.RedirectPort)}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Verify state
		if r.URL.Query().Get("state") != state {
			errChan <- fmt.Errorf("state mismatch")
			http.Error(w, "State mismatch", http.StatusBadRequest)
			return
		}

		code := r.URL.Query().Get("code")
		if code == "" {
			errChan <- fmt.Errorf("no code in callback")
			http.Error(w, "No code received", http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("<html><body><h1>Authorization successful!</h1><p>You can close this window.</p></body></html>"))
		codeChan <- code
	})

	// Start server in goroutine with panic recovery
	go func() {
		defer func() {
			if r := recover(); r != nil {
				a.GetLogger().ErrorLog("[Gemini Auth] Panic recovered in server goroutine: %v", r)
				errChan <- fmt.Errorf("panic: %v", r)
			}
		}()
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			errChan <- err
		}
	}()

	// Print authorization URL for user
	fmt.Printf("\n[Gemini Auth] Please visit the following URL to authorize:\n\n%s\n\n", authURL)
	fmt.Println("[Gemini Auth] Waiting for authorization...")

	// Wait for code or error
	var code string
	select {
	case code = <-codeChan:
		// Got the code
	case err := <-errChan:
		server.Shutdown(ctx)
		return fmt.Errorf("authorization failed: %w", err)
	case <-ctx.Done():
		server.Shutdown(ctx)
		return ctx.Err()
	case <-time.After(5 * time.Minute):
		server.Shutdown(ctx)
		return fmt.Errorf("authorization timeout")
	}

	// Shutdown server
	server.Shutdown(ctx)

	// Exchange code for tokens
	return a.exchangeCodeForTokens(ctx, code, redirectURI)
}

// exchangeCodeForTokens exchanges authorization code for tokens
func (a *GeminiAuthenticator) exchangeCodeForTokens(ctx context.Context, code, redirectURI string) error {
	data := url.Values{}
	data.Set("client_id", a.config.ClientID)
	data.Set("client_secret", a.config.ClientSecret)
	data.Set("code", code)
	data.Set("grant_type", "authorization_code")
	data.Set("redirect_uri", redirectURI)

	req, err := http.NewRequestWithContext(ctx, "POST", "https://oauth2.googleapis.com/token", strings.NewReader(data.Encode()))
	if err != nil {
		return fmt.Errorf("failed to create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := a.BaseAuthenticator.GetHTTPClient().Do(req)
	if err != nil {
		return fmt.Errorf("failed to send token request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("token exchange failed with status: %d", resp.StatusCode)
	}

	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
		TokenType    string `json:"token_type"`
		Scope        string `json:"scope"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return fmt.Errorf("failed to decode token response: %w", err)
	}

	// Check if multi-token manager is available
	multiTokenMgr := a.GetMultiTokenManager()
	if multiTokenMgr == nil {
		// Fallback to legacy saving
		a.GetLogger().DebugLog("[Gemini Auth] Multi-token manager not available, using legacy save")
		a.GetLogger().DebugLog("[Gemini Auth] Authentication successful, credentials saved")
		return nil
	}

	// Extract email from token response
	tokenResponse := make(map[string]interface{})
	tokenResponse["access_token"] = tokenResp.AccessToken
	tokenResponse["refresh_token"] = tokenResp.RefreshToken
	tokenResponse["token_type"] = tokenResp.TokenType
	tokenResponse["expires_in"] = tokenResp.ExpiresIn
	tokenResponse["scope"] = tokenResp.Scope

	email, err := multiTokenMgr.ExtractEmail(ctx, "gemini", tokenResponse, tokenResp.AccessToken)
	if err != nil {
		a.GetLogger().WarnLog("[Gemini Auth] Failed to extract email: %v", err)
		email = ""
	}

	// Create ProviderToken with email
	now := time.Now()
	expiry := now.Add(time.Duration(tokenResp.ExpiresIn) * time.Second)

	providerToken := ProviderToken{
		ID:           uuid.New().String(),
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		TokenType:    tokenResp.TokenType,
		ExpiryDate:   expiry.UnixMilli(),
		Email:        email,
		Scope:        tokenResp.Scope,
		Healthy:      true,
		HealthScore:  1.0,
		LastUsed:     now.UnixMilli(),
		CreatedAt:    now.UnixMilli(),
	}

	// Save token to multi-token store
	if err := multiTokenMgr.SaveToken("gemini", providerToken); err != nil {
		return fmt.Errorf("failed to save token to multi-token store: %w", err)
	}

	a.GetLogger().DebugLog("[Gemini Auth] Authentication successful, credentials saved to multi-token store")
	return nil
}
