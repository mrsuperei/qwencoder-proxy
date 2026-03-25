// Package iflow provides authentication for iFlow provider
package iflow

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/sunbankio/qwencoder-proxy/internal/logging"
	tokenpkg "github.com/sunbankio/qwencoder-proxy/internal/token"
	"golang.org/x/oauth2"
)

// OAuthConfig holds the OAuth configuration for iFlow
type OAuthConfig struct {
	ClientID     string
	ClientSecret string
	RedirectPort int
	CredsDir     string
	CredsFile    string
}

// DefaultOAuthConfig returns the default iFlow OAuth configuration
func DefaultOAuthConfig() *OAuthConfig {
	return &OAuthConfig{
		ClientID:     ClientID,
		ClientSecret: ClientSecret,
		RedirectPort: DefaultPort,
		CredsDir:     ".iflow",
		CredsFile:    "oauth_creds.json",
	}
}

// Credentials represents the stored OAuth credentials for iFlow
type Credentials struct {
	AuthType        string `json:"auth_type"` // "oauth" or "cookie"
	AccessToken     string `json:"access_token,omitempty"`
	RefreshToken    string `json:"refresh_token,omitempty"`
	Expire          string `json:"expire,omitempty"`
	ExpiresAt       string `json:"expires_at,omitempty"`
	ExpiryDate      int64  `json:"expiry_date,omitempty"` // Added to match user's file format
	Cookies         string `json:"cookies,omitempty"`
	CookieExpiresAt string `json:"cookie_expires_at,omitempty"`
	Email           string `json:"email,omitempty"`
	UserID          string `json:"user_id,omitempty"`
	LastRefresh     string `json:"last_refresh,omitempty"`
	APIKey          string `json:"apiKey,omitempty"` // Changed tag to "apiKey"
	TokenType       string `json:"token_type,omitempty"`
	Scope           string `json:"scope,omitempty"`
	Type            string `json:"type"` // "iflow"
}

// OAuthFileCredentials represents the exact file structure required by the user
type OAuthFileCredentials struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiryDate   int64  `json:"expiry_date"`
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope"`
	APIKey       string `json:"apiKey"`
}

// PKCECodes represents PKCE codes for OAuth2 authorization
type PKCECodes struct {
	CodeVerifier  string `json:"code_verifier"`
	CodeChallenge string `json:"code_challenge"`
}

// OAuthCallbackResult represents the result of OAuth callback
type OAuthCallbackResult struct {
	Code  string `json:"code"`
	State string `json:"state"`
	Error string `json:"error,omitempty"`
}

// IsExpired checks if the credentials are expired
func (c *Credentials) IsExpired() bool {
	if c.Expire != "" {
		if expire, err := time.Parse(time.RFC3339, c.Expire); err == nil {
			return expire.Before(time.Now().Add(5 * time.Minute))
		}
	}
	if c.ExpiresAt != "" {
		if expire, err := time.Parse(time.RFC3339, c.ExpiresAt); err == nil {
			return expire.Before(time.Now().Add(5 * time.Minute))
		}
	}
	return true
}

// IsValid checks if the credentials are valid
func (c *Credentials) IsValid() bool {
	if c.AuthType == "oauth" {
		return c.AccessToken != "" && !c.IsExpired()
	}
	if c.AuthType == "cookie" {
		return c.Cookies != "" || c.APIKey != ""
	}
	return false
}

// GetExpire returns the expire time string
func (c *Credentials) GetExpire() string {
	if c.Expire != "" {
		return c.Expire
	}
	return c.ExpiresAt
}

// Authenticator implements the authentication for iFlow
type Authenticator struct {
	tokenManager  *tokenpkg.TokenManager
	multiTokenMgr *tokenpkg.MultiTokenManager
	config        *OAuthConfig
	credentials   *Credentials
	mu            sync.RWMutex
	tokenSource   oauth2.TokenSource
	logger        logging.Logger
	httpClient    *http.Client
}

// NewAuthenticator creates a new iFlow authenticator
func NewAuthenticator(config *OAuthConfig) *Authenticator {
	if config == nil {
		config = DefaultOAuthConfig()
	}
	return &Authenticator{
		config:     config,
		logger:     logging.NewLogger(),
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// SetTokenManager sets the token manager
func (a *Authenticator) SetTokenManager(tokenManager *tokenpkg.TokenManager) {
	a.tokenManager = tokenManager
}

// SetMultiTokenManager sets the multi-token manager
func (a *Authenticator) SetMultiTokenManager(multiTokenMgr *tokenpkg.MultiTokenManager) {
	a.multiTokenMgr = multiTokenMgr
}

// GetTokenManager returns the token manager
func (a *Authenticator) GetTokenManager() *tokenpkg.TokenManager {
	return a.tokenManager
}

// GetMultiTokenManager returns the multi-token manager
func (a *Authenticator) GetMultiTokenManager() *tokenpkg.MultiTokenManager {
	return a.multiTokenMgr
}

// GetLogger returns the logger
func (a *Authenticator) GetLogger() logging.Logger {
	return a.logger
}

// GetHTTPClient returns an HTTP client configured with the token's proxy settings.
// This is an optional method - authenticators that do not support proxy-aware clients
// should return an error indicating the method is not implemented.
func (a *Authenticator) GetHTTPClient() (*http.Client, error) {
	return a.httpClient, nil
}

// Authenticate performs the OAuth authentication flow
func (a *Authenticator) Authenticate(ctx context.Context) error {
	// Generate PKCE codes
	pkceCodes, err := a.generatePKCECodes()
	if err != nil {
		return fmt.Errorf("failed to generate PKCE codes: %w", err)
	}

	// Start OAuth server to handle callback
	callbackResult, err := a.waitForCallback()
	if err != nil {
		return fmt.Errorf("OAuth callback failed: %w", err)
	}

	if callbackResult.Error != "" {
		return fmt.Errorf("OAuth error: %s", callbackResult.Error)
	}

	// Exchange authorization code for tokens
	if err := a.exchangeCodeForTokens(callbackResult.Code, pkceCodes); err != nil {
		return fmt.Errorf("failed to exchange code for tokens: %w", err)
	}

	// Fetch user info and API key
	if err := a.fetchUserInfo(); err != nil {
		a.GetLogger().DebugLog("[iFlow] Failed to fetch user info: %v", err)
	}

	return nil
}

// GetToken returns a valid API key for LLM calls, refreshing if necessary
func (a *Authenticator) GetToken(ctx context.Context) (string, error) {
	tokenManager := a.GetTokenManager()
	// If token manager is set, use it for token selection
	if tokenManager != nil {
		token, err := tokenManager.SelectToken()
		if err != nil {
			return "", fmt.Errorf("failed to select token from token manager: %w", err)
		}
		// Return API key if available, otherwise return access token
		if token.APIKey != "" {
			return token.APIKey, nil
		}
		return token.AccessToken, nil
	}

	// Fallback to legacy credential loading
	a.mu.Lock()
	defer a.mu.Unlock()

	// Load credentials if not loaded
	if a.credentials == nil {
		a.mu.Unlock()
		a.loadCredentials()
		a.mu.Lock()
	}

	// Check if we have credentials
	if a.credentials == nil || a.credentials.AccessToken == "" {
		return "", errors.New("no valid credentials available")
	}

	// Convert to oauth2.Token
	oauthToken := &oauth2.Token{
		AccessToken:  a.credentials.AccessToken,
		RefreshToken: a.credentials.RefreshToken,
		TokenType:    a.credentials.TokenType,
	}

	// Parse expiry if available
	if a.credentials.ExpiresAt != "" {
		if expiry, err := time.Parse(time.RFC3339, a.credentials.ExpiresAt); err == nil {
			oauthToken.Expiry = expiry
		}
	}

	// Setup OAuth2 config for iFlow
	conf := &oauth2.Config{
		ClientID:     a.config.ClientID,
		ClientSecret: a.config.ClientSecret,
		Endpoint: oauth2.Endpoint{
			AuthURL:  AuthURL,
			TokenURL: TokenURL,
		},
	}

	// Create a context with the custom HTTP client
	client, err := a.GetHTTPClient()
	if err != nil {
		return "", fmt.Errorf("failed to get HTTP client: %w", err)
	}
	oauth2Context := context.WithValue(ctx, oauth2.HTTPClient, client)

	// Create TokenSource with the current token
	ts := conf.TokenSource(oauth2Context, oauthToken)

	// Get token (this will refresh if needed)
	newToken, err := ts.Token()
	if err != nil {
		a.GetLogger().ErrorLog("[iFlow Auth] Token refresh failed: %v", err)
		return "", fmt.Errorf("failed to refresh token: %w", err)
	}

	// Update credentials if token changed
	if newToken.AccessToken != a.credentials.AccessToken || newToken.RefreshToken != a.credentials.RefreshToken {
		a.GetLogger().InfoLog("[iFlow Auth] Token refreshed successfully, saving credentials")

		a.credentials.AccessToken = newToken.AccessToken
		a.credentials.RefreshToken = newToken.RefreshToken
		a.credentials.TokenType = newToken.TokenType
		if !newToken.Expiry.IsZero() {
			a.credentials.ExpiresAt = newToken.Expiry.Format(time.RFC3339)
			a.credentials.ExpiryDate = newToken.Expiry.UnixMilli()
		}

		// Fetch user info and API key after refresh
		a.mu.Unlock()
		if err := a.fetchUserInfo(); err != nil {
			a.GetLogger().ErrorLog("[iFlow Auth] Failed to fetch user info after refresh: %v", err)
		} else {
			a.GetLogger().InfoLog("[iFlow Auth] User info and API key updated successfully")
		}

		if err := a.saveCredentials(); err != nil {
			a.GetLogger().ErrorLog("Failed to save refreshed credentials: %v", err)
		}
		a.mu.Lock()
	}

	// If we still don't have an API key, try to fetch it
	if a.credentials.APIKey == "" {
		a.mu.Unlock()
		if err := a.fetchUserInfo(); err != nil {
			a.GetLogger().ErrorLog("[iFlow Auth] Failed to fetch API key: %v", err)
		} else {
			a.saveCredentials()
		}
		a.mu.Lock()
	}

	// Return API key for LLM calls, as requested by the user
	if a.credentials.APIKey != "" {
		return a.credentials.APIKey, nil
	}

	// Fallback to AccessToken if APIKey is still not available (should not happen)
	return a.credentials.AccessToken, nil
}

// GetAPIKey returns the stored API key
func (a *Authenticator) GetAPIKey() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.credentials == nil {
		return ""
	}
	return a.credentials.APIKey
}

// GetTokenWithClient returns a valid access token and an HTTP client.
// The HTTP client is configured with the proxy settings from the selected token.
// If a token is already selected in the context (by rate limiting middleware),
// it will be used instead of selecting a new token.
func (a *Authenticator) GetTokenWithClient(ctx context.Context) (string, *http.Client, error) {
	// Check if token is already selected in context (by rate limiting middleware)
	if selectedToken, ok := ctx.Value("selected_token").(*tokenpkg.ProviderToken); ok {
		a.GetLogger().DebugLog("[IFlowAuth] Using pre-selected token from context: ID=%s", selectedToken.ID)
		// Use default HTTP client for now
		// Note: Proxy-aware client creation would require access to client factory
		// Return API key if available, otherwise return access token
		if selectedToken.APIKey != "" {
			return selectedToken.APIKey, &http.Client{Timeout: 30 * time.Second}, nil
		}
		return selectedToken.AccessToken, &http.Client{Timeout: 30 * time.Second}, nil
	}

	// Fallback to original selection logic
	tokenManager := a.GetTokenManager()
	if tokenManager == nil {
		token, err := a.GetToken(ctx)
		return token, nil, err
	}

	token, client, err := tokenManager.SelectTokenWithClient()
	if err != nil {
		return "", nil, fmt.Errorf("failed to select token with client: %w", err)
	}

	// Return API key if available, otherwise return access token
	if token.APIKey != "" {
		return token.APIKey, client, nil
	}
	return token.AccessToken, client, nil
}

// GetHTTPClientForToken returns an HTTP client configured with proxy settings.
// The client is configured with the proxy settings from the selected token.
func (a *Authenticator) GetHTTPClientForToken() (*http.Client, error) {
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

// IsAuthenticated checks if valid credentials exist
func (a *Authenticator) IsAuthenticated() bool {
	tokenManager := a.GetTokenManager()
	// If token manager is set, check if it has tokens
	if tokenManager != nil {
		return tokenManager.GetTokenCount() > 0
	}

	// Fallback to legacy credential check
	a.mu.RLock()
	if a.credentials == nil {
		a.mu.RUnlock()
		// Try to load from file
		a.loadCredentials()
		a.mu.RLock()
		if a.credentials == nil {
			a.mu.RUnlock()
			return false
		}
	}

	result := a.credentials.IsValid()
	a.mu.RUnlock()
	return result
}

// GetCredentialsPath returns the path to stored credentials
func (a *Authenticator) GetCredentialsPath() string {
	homeDir, _ := os.UserHomeDir()
	return filepath.Join(homeDir, a.config.CredsDir, a.config.CredsFile)
}

// ClearCredentials removes stored credentials
func (a *Authenticator) ClearCredentials() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Multi-token system handles token removal separately
	// Don't delete the credentials file as it may contain other tokens
	a.credentials = nil
	return nil
}

// ForceRefresh forces a token refresh regardless of expiry
func (a *Authenticator) ForceRefresh(ctx context.Context) error {
	// Multi-token system handles refresh via TokenManager
	// This is a no-op since TokenManager handles refresh internally
	return nil
}

// loadCredentials loads credentials from file
func (a *Authenticator) loadCredentials() {
	credsPath := a.GetCredentialsPath()

	data, err := os.ReadFile(credsPath)
	if err != nil {
		return
	}

	// Try to load as OAuthFileCredentials first (strict user format)
	var fileCreds OAuthFileCredentials
	if err := json.Unmarshal(data, &fileCreds); err == nil && fileCreds.AccessToken != "" {
		// Map to internal struct
		creds := &Credentials{
			AuthType:     "oauth",
			Type:         "iflow",
			AccessToken:  fileCreds.AccessToken,
			RefreshToken: fileCreds.RefreshToken,
			TokenType:    fileCreds.TokenType,
			Scope:        fileCreds.Scope,
			APIKey:       fileCreds.APIKey,
			ExpiryDate:   fileCreds.ExpiryDate,
		}

		// Convert expiry date (millis) to RFC3339 string for internal use
		if fileCreds.ExpiryDate > 0 {
			t := time.UnixMilli(fileCreds.ExpiryDate)
			creds.ExpiresAt = t.Format(time.RFC3339)
			creds.Expire = creds.ExpiresAt
		}

		a.credentials = creds

		// If we have access token but no API key, try to fetch user info
		if a.credentials.APIKey == "" && a.credentials.AccessToken != "" {
			if err := a.fetchUserInfo(); err != nil {
				a.GetLogger().DebugLog("[iFlow] Failed to fetch user info during load: %v", err)
			}
		}
		return
	}

	// Fallback to standard loading
	var creds Credentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return
	}

	a.credentials = &creds

	// Ensure auth_type is set if empty
	if a.credentials.AuthType == "" {
		a.credentials.AuthType = "oauth"
	}
	if a.credentials.Type == "" {
		a.credentials.Type = "iflow"
	}

	// If we have access token but no API key, try to fetch user info
	if a.credentials.APIKey == "" && a.credentials.AccessToken != "" {
		if err := a.fetchUserInfo(); err != nil {
			a.GetLogger().DebugLog("[iFlow] Failed to fetch user info during load: %v", err)
		}
	}
}

// saveCredentials saves credentials to file
func (a *Authenticator) saveCredentials() error {
	if a.credentials == nil {
		return fmt.Errorf("no credentials to save")
	}

	credsPath := a.GetCredentialsPath()

	// Create directory if it doesn't exist
	if err := os.MkdirAll(filepath.Dir(credsPath), 0700); err != nil {
		return fmt.Errorf("failed to create credentials directory: %w", err)
	}

	var data []byte
	var err error

	// Use strict format for OAuth
	if a.credentials.AuthType == "oauth" {
		fileCreds := OAuthFileCredentials{
			AccessToken:  a.credentials.AccessToken,
			RefreshToken: a.credentials.RefreshToken,
			TokenType:    a.credentials.TokenType,
			Scope:        a.credentials.Scope,
			APIKey:       a.credentials.APIKey,
		}

		// Handle ExpiryDate
		if a.credentials.ExpiryDate > 0 {
			fileCreds.ExpiryDate = a.credentials.ExpiryDate
		} else if a.credentials.ExpiresAt != "" {
			if t, err := time.Parse(time.RFC3339, a.credentials.ExpiresAt); err == nil {
				fileCreds.ExpiryDate = t.UnixMilli()
			}
		}

		data, err = json.MarshalIndent(fileCreds, "", "  ")
	} else {
		// Standard format for others
		data, err = json.MarshalIndent(a.credentials, "", "  ")
	}

	if err != nil {
		return fmt.Errorf("failed to marshal credentials: %w", err)
	}

	if err := os.WriteFile(credsPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write credentials file: %w", err)
	}

	return nil
}

// generatePKCECodes generates PKCE codes for OAuth2
func (a *Authenticator) generatePKCECodes() (*PKCECodes, error) {
	// Generate 96 random bytes for code verifier
	bytes := make([]byte, 96)
	if _, err := rand.Read(bytes); err != nil {
		return nil, fmt.Errorf("failed to generate random bytes: %w", err)
	}
	codeVerifier := base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString(bytes)

	// Generate code challenge using S256 method
	hasher := sha256.New()
	hasher.Write([]byte(codeVerifier))
	hash := hasher.Sum(nil)
	codeChallenge := base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString(hash)

	return &PKCECodes{
		CodeVerifier:  codeVerifier,
		CodeChallenge: codeChallenge,
	}, nil
}

// generateState generates a random state string for CSRF protection
func (a *Authenticator) generateState() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}
	return base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString(bytes), nil
}

// generateAuthURL generates the OAuth authorization URL
func (a *Authenticator) generateAuthURL(state string, pkceCodes *PKCECodes) (string, error) {
	params := url.Values{
		"client_id":             {a.config.ClientID},
		"response_type":         {"code"},
		"redirect_uri":          {a.getRedirectURI()},
		"scope":                 {"openid email profile offline_access"},
		"state":                 {state},
		"code_challenge":        {pkceCodes.CodeChallenge},
		"code_challenge_method": {"S256"},
	}

	return fmt.Sprintf("%s?%s", AuthURL, params.Encode()), nil
}

// getRedirectURI returns the redirect URI for OAuth callback
func (a *Authenticator) getRedirectURI() string {
	return fmt.Sprintf("http://localhost:%d/auth/callback", a.config.RedirectPort)
}

// waitForCallback waits for OAuth callback
func (a *Authenticator) waitForCallback() (*OAuthCallbackResult, error) {
	// For now, we'll implement a simple manual approach
	// In a full implementation, this would start an HTTP server
	// For the scope of this task, we'll instruct the user to manually provide the authorization code

	pkceCodes, err := a.generatePKCECodes()
	if err != nil {
		return nil, fmt.Errorf("failed to generate PKCE codes: %w", err)
	}

	state, err := a.generateState()
	if err != nil {
		return nil, fmt.Errorf("failed to generate state: %w", err)
	}

	authURL, err := a.generateAuthURL(state, pkceCodes)
	if err != nil {
		return nil, fmt.Errorf("failed to generate auth URL: %w", err)
	}

	a.logger.InfoLog("[iFlow] Please open the following URL in your browser:")
	a.GetLogger().InfoLog("[iFlow] %s", authURL)
	a.GetLogger().InfoLog("[iFlow] After authorization, you will be redirected to a page showing the authorization code.")
	a.GetLogger().InfoLog("[iFlow] Please copy the authorization code from the URL parameter 'code' and provide it to continue.")

	// For now, return an error indicating manual intervention is needed
	return nil, fmt.Errorf("manual OAuth flow requires user interaction - please implement full OAuth server for automated flow")
}

// exchangeCodeForTokens exchanges authorization code for tokens
func (a *Authenticator) exchangeCodeForTokens(code string, pkceCodes *PKCECodes) error {
	// Create token request with PKCE
	tokenURL := fmt.Sprintf("%s?grant_type=authorization_code&client_id=%s&client_secret=%s&code=%s&redirect_uri=%s&code_verifier=%s",
		TokenURL,
		url.QueryEscape(a.config.ClientID),
		url.QueryEscape(a.config.ClientSecret),
		url.QueryEscape(code),
		url.QueryEscape(a.getRedirectURI()),
		url.QueryEscape(pkceCodes.CodeVerifier),
	)

	req, err := http.NewRequest("POST", tokenURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create token request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	client, err := a.GetHTTPClient()
	if err != nil {
		return fmt.Errorf("failed to get HTTP client: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send token request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("token exchange failed (status %d): %s", resp.StatusCode, string(body))
	}

	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		TokenType    string `json:"token_type"`
		ExpiresIn    int64  `json:"expires_in"`
		Scope        string `json:"scope"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return fmt.Errorf("failed to decode token response: %w", err)
	}

	if tokenResp.AccessToken == "" {
		return fmt.Errorf("no access_token in response")
	}

	expiresAt := time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)

	// Check if multi-token manager is available
	multiTokenMgr := a.GetMultiTokenManager()
	if multiTokenMgr == nil {
		// Fallback to legacy saving
		a.mu.Lock()
		a.credentials = &Credentials{
			AuthType:     "oauth",
			AccessToken:  tokenResp.AccessToken,
			RefreshToken: tokenResp.RefreshToken,
			TokenType:    tokenResp.TokenType,
			Expire:       expiresAt.Format(time.RFC3339),
			ExpiresAt:    expiresAt.Format(time.RFC3339),
			ExpiryDate:   expiresAt.UnixMilli(),
			LastRefresh:  time.Now().Format(time.RFC3339),
			Type:         "iflow",
		}
		a.mu.Unlock()
		// Fetch user info and API key
		if err := a.fetchUserInfo(); err != nil {
			a.GetLogger().DebugLog("[iFlow] Failed to fetch user info: %v", err)
			// Don't fail exchange, just log error
		}
		return a.saveCredentials()
	}

	// Extract email from token response
	tokenResponse := make(map[string]interface{})
	tokenResponse["access_token"] = tokenResp.AccessToken
	tokenResponse["refresh_token"] = tokenResp.RefreshToken
	tokenResponse["token_type"] = tokenResp.TokenType
	tokenResponse["expires_in"] = tokenResp.ExpiresIn
	tokenResponse["scope"] = tokenResp.Scope

	email, err := multiTokenMgr.ExtractEmail(context.Background(), "iflow", tokenResponse, tokenResp.AccessToken)
	if err != nil {
		a.GetLogger().WarnLog("[iFlow Auth] Failed to extract email: %v", err)
		email = ""
	}

	// Fetch user info and API key
	if err := a.fetchUserInfo(); err != nil {
		a.GetLogger().DebugLog("[iFlow] Failed to fetch user info: %v", err)
		// Don't fail exchange, just log error
	}

	// Use email from credentials if available (fetchUserInfo may have set it)
	a.mu.RLock()
	credsAPIKey := ""
	if a.credentials != nil {
		if a.credentials.Email != "" {
			email = a.credentials.Email
		}
		credsAPIKey = a.credentials.APIKey
	}
	a.mu.RUnlock()

	// Create ProviderToken with email and API key
	now := time.Now()
	providerToken := tokenpkg.ProviderToken{
		ID:           uuid.New().String(),
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		TokenType:    tokenResp.TokenType,
		ExpiryDate:   expiresAt.UnixMilli(),
		Email:        email,
		Scope:        tokenResp.Scope,
		APIKey:       credsAPIKey,
		Healthy:      true,
		HealthScore:  1.0,
		LastUsed:     now.UnixMilli(),
		CreatedAt:    now.UnixMilli(),
	}

	// Save token to multi-token store
	if err := multiTokenMgr.SaveToken("iflow", providerToken); err != nil {
		return fmt.Errorf("failed to save token to multi-token store: %w", err)
	}

	a.GetLogger().DebugLog("[iFlow Auth] Authentication successful, credentials saved to multi-token store")
	return nil
}

// fetchUserInfo fetches user information and API key
func (a *Authenticator) fetchUserInfo() error {
	if a.credentials == nil || a.credentials.AccessToken == "" {
		return fmt.Errorf("no access token available")
	}

	userInfoURL := fmt.Sprintf("%s?accessToken=%s", UserInfoURL, url.QueryEscape(a.credentials.AccessToken))

	req, err := http.NewRequest("GET", userInfoURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create user info request: %w", err)
	}

	req.Header.Set("Accept", "application/json")

	client, err := a.GetHTTPClient()
	if err != nil {
		return fmt.Errorf("failed to get HTTP client: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send user info request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("user info request failed (status %d): %s", resp.StatusCode, string(body))
	}

	var userInfoResp map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&userInfoResp); err != nil {
		return fmt.Errorf("failed to decode user info response: %w", err)
	}

	if success, ok := userInfoResp["success"].(bool); !ok || !success {
		return fmt.Errorf("user info request unsuccessful")
	}

	data, ok := userInfoResp["data"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("no data in user info response")
	}

	if apiKey, ok := data["apiKey"].(string); ok {
		a.credentials.APIKey = apiKey
	}

	if email, ok := data["email"].(string); ok {
		a.credentials.Email = email
	} else if phone, ok := data["phone"].(string); ok {
		a.credentials.Email = phone
	}

	return a.saveCredentials()
}

// iflowTokenRefresher implements ProviderRefresh for iFlow
type iflowTokenRefresher struct {
	clientID     string
	clientSecret string
	tokenURL     string
	logger       logging.Logger
}

// NewIFlowTokenRefresher creates a new iFlow token refresher
func NewIFlowTokenRefresher(clientID, clientSecret, tokenURL string, logger logging.Logger) *iflowTokenRefresher {
	return &iflowTokenRefresher{
		clientID:     clientID,
		clientSecret: clientSecret,
		tokenURL:     tokenURL,
		logger:       logger,
	}
}

// ProviderID returns the provider identifier
func (i *iflowTokenRefresher) ProviderID() string {
	return "iflow"
}

// RefreshToken refreshes an iFlow OAuth token
func (i *iflowTokenRefresher) RefreshToken(ctx context.Context, token tokenpkg.ProviderToken) (tokenpkg.ProviderToken, error) {
	i.logger.InfoLog("[iFlowRefresh] Starting token refresh for token ID %s", token.ID)

	if token.RefreshToken == "" {
		i.logger.ErrorLog("[iFlowRefresh] No refresh token available for token ID %s", token.ID)
		return tokenpkg.ProviderToken{}, fmt.Errorf("no refresh token available")
	}

	// Prepare refresh request
	data := url.Values{}
	data.Set("client_id", i.clientID)
	data.Set("client_secret", i.clientSecret)
	data.Set("refresh_token", token.RefreshToken)
	data.Set("grant_type", "refresh_token")

	// Make HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", i.tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		i.logger.ErrorLog("[iFlowRefresh] Failed to create refresh request for token ID %s: %v", token.ID, err)
		return tokenpkg.ProviderToken{}, fmt.Errorf("failed to create refresh request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		i.logger.ErrorLog("[iFlowRefresh] Failed to send refresh request for token ID %s: %v", token.ID, err)
		return tokenpkg.ProviderToken{}, fmt.Errorf("failed to send refresh request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		i.logger.ErrorLog("[iFlowRefresh] Refresh failed with status %d for token ID %s: %s", resp.StatusCode, token.ID, string(body))
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
		i.logger.ErrorLog("[iFlowRefresh] Failed to decode refresh response for token ID %s: %v", token.ID, err)
		return tokenpkg.ProviderToken{}, fmt.Errorf("failed to decode refresh response: %w", err)
	}

	// Calculate expiry
	expiryDate := time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	i.logger.InfoLog("[iFlowRefresh] Token refresh successful for token ID %s, new expiry %s", token.ID, expiryDate.Format(time.RFC3339))

	// Return refreshed token
	return tokenpkg.ProviderToken{
		ID:           token.ID,
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		TokenType:    tokenResp.TokenType,
		ExpiryDate:   expiryDate.UnixMilli(),
		Email:        token.Email,
		APIKey:       token.APIKey,
		Healthy:      true,
		HealthScore:  1.0,
		LastUsed:     token.LastUsed,
		CreatedAt:    token.CreatedAt,
	}, nil
}
