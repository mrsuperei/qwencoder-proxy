// Package kiro provides authentication for Kiro provider
package kiro

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/sunbankio/qwencoder-proxy/internal/token"
	"github.com/sunbankio/qwencoder-proxy/logging"
	"golang.org/x/oauth2"
)

const (
	// TokenRefreshBufferMs is the buffer time before token refresh (30 minutes)
	TokenRefreshBufferMs = 1800 * 1000
)

// OAuthConfig holds the OAuth configuration for Kiro
type OAuthConfig struct {
	Region        string
	RefreshURL    string
	RefreshIDCURL string
	BaseURL       string
	CredsPath     string
}

// DefaultOAuthConfig returns the default Kiro OAuth configuration
func DefaultOAuthConfig() *OAuthConfig {
	homeDir, _ := os.UserHomeDir()
	return &OAuthConfig{
		Region:        "us-east-1",
		RefreshURL:    "https://prod.{{region}}.auth.desktop.kiro.dev/refreshToken",
		RefreshIDCURL: "https://oidc.{{region}}.amazonaws.com/token",
		BaseURL:       "https://codewhisperer.{{region}}.amazonaws.com",
		CredsPath:     filepath.Join(homeDir, ".aws", "sso", "cache", "kiro-auth-token.json"),
	}
}

// Credentials represents the stored Kiro credentials
type Credentials struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken,omitempty"`
	ExpiresAt    string `json:"expiresAt,omitempty"`
	ClientID     string `json:"clientId,omitempty"`
	ClientSecret string `json:"clientSecret,omitempty"`
	AuthMethod   string `json:"authMethod,omitempty"`
	Region       string `json:"region,omitempty"`
	ProfileArn   string `json:"profileArn,omitempty"`
}

// Authenticator implements the authentication for Kiro
type Authenticator struct {
	tokenManager  *token.TokenManager
	multiTokenMgr *token.MultiTokenManager
	config        *OAuthConfig
	credentials   *Credentials
	mu            sync.RWMutex
	logger        logging.Logger
	httpClient    *http.Client
}

// NewAuthenticator creates a new Kiro authenticator
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
func (a *Authenticator) SetTokenManager(tokenManager *token.TokenManager) {
	a.tokenManager = tokenManager
}

// SetMultiTokenManager sets the multi-token manager
func (a *Authenticator) SetMultiTokenManager(multiTokenMgr *token.MultiTokenManager) {
	a.multiTokenMgr = multiTokenMgr
}

// GetTokenManager returns the token manager
func (a *Authenticator) GetTokenManager() *token.TokenManager {
	return a.tokenManager
}

// GetMultiTokenManager returns the multi-token manager
func (a *Authenticator) GetMultiTokenManager() *token.MultiTokenManager {
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

// GetCredentialsPath returns the path to the credentials file
func (a *Authenticator) GetCredentialsPath() string {
	return a.config.CredsPath
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
		creds, err := a.loadCredentials()
		if err != nil {
			return false
		}
		a.mu.Lock()
		a.credentials = creds
		a.mu.Unlock()
		a.mu.RLock()
	}

	// Check if token is still valid
	if a.credentials.ExpiresAt != "" {
		expiresAt, err := time.Parse(time.RFC3339, a.credentials.ExpiresAt)
		// Check against 30 minute buffer
		buffer := time.Duration(TokenRefreshBufferMs) * time.Millisecond
		if err == nil && expiresAt.Before(time.Now().Add(buffer)) {
			// Considered "not authenticated" (needs refresh) if we strictly check validity here.
			// But IsAuthenticated usually just checks if we have *some* credentials.
			// Let's stick to simple existence + expiry check.
			a.mu.RUnlock()
			return false
		}
	}

	result := a.credentials != nil && a.credentials.AccessToken != ""
	a.mu.RUnlock()
	return result
}

// loadCredentials loads credentials from file
func (a *Authenticator) loadCredentials() (*Credentials, error) {
	credsPath := a.GetCredentialsPath()

	// First try to load the main credentials file
	data, err := os.ReadFile(credsPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read credentials file: %w", err)
	}

	var creds Credentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return nil, fmt.Errorf("failed to parse credentials: %w", err)
	}

	// Also try to load additional credentials from the same directory
	dir := filepath.Dir(credsPath)
	files, err := os.ReadDir(dir)
	if err == nil {
		for _, file := range files {
			if file.IsDir() || !strings.HasSuffix(file.Name(), ".json") {
				continue
			}
			if file.Name() == filepath.Base(credsPath) {
				continue
			}

			filePath := filepath.Join(dir, file.Name())
			fileData, err := os.ReadFile(filePath)
			if err != nil {
				continue
			}

			var additionalCreds Credentials
			if err := json.Unmarshal(fileData, &additionalCreds); err != nil {
				continue
			}

			// Merge additional credentials (client info)
			if additionalCreds.ClientID != "" && creds.ClientID == "" {
				creds.ClientID = additionalCreds.ClientID
			}
			if additionalCreds.ClientSecret != "" && creds.ClientSecret == "" {
				creds.ClientSecret = additionalCreds.ClientSecret
			}
		}
	}

	// Set region from credentials or use default
	if creds.Region == "" {
		creds.Region = a.config.Region
	}

	// Extract email if multi-token manager is available
	multiTokenMgr := a.GetMultiTokenManager()
	if multiTokenMgr != nil && creds.AccessToken != "" {
		tokenResponse := make(map[string]interface{})
		tokenResponse["access_token"] = creds.AccessToken
		tokenResponse["refresh_token"] = creds.RefreshToken
		tokenResponse["token_type"] = "Bearer"

		email, err := multiTokenMgr.ExtractEmail(context.Background(), "kiro", tokenResponse, creds.AccessToken)
		if err != nil {
			a.GetLogger().DebugLog("[Kiro Auth] Failed to extract email: %v", err)
		} else {
			if email != "" {
				a.GetLogger().DebugLog("[Kiro Auth] Extracted email: %s", email)
			}
		}
	}

	return &creds, nil
}

// saveCredentials saves credentials to file
func (a *Authenticator) saveCredentials(creds *Credentials) error {
	credsPath := a.GetCredentialsPath()

	// Create directory if it doesn't exist
	dir := filepath.Dir(credsPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create credentials directory: %w", err)
	}

	// Load existing credentials to merge
	existingData, _ := os.ReadFile(credsPath)
	var existing Credentials
	if len(existingData) > 0 {
		json.Unmarshal(existingData, &existing)
	}

	// Merge new credentials with existing
	if creds.AccessToken != "" {
		existing.AccessToken = creds.AccessToken
	}
	if creds.RefreshToken != "" {
		existing.RefreshToken = creds.RefreshToken
	}
	if creds.ExpiresAt != "" {
		existing.ExpiresAt = creds.ExpiresAt
	}

	data, err := json.MarshalIndent(existing, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal credentials: %w", err)
	}

	// Write file (protected by caller lock usually)
	if err := os.WriteFile(credsPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write credentials file: %w", err)
	}

	return nil
}

// ClearCredentials removes stored credentials
func (a *Authenticator) ClearCredentials() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.credentials = nil
	// Don't delete the file as it may contain other AWS SSO credentials
	// Multi-token system handles token removal separately
	return nil
}

// kiroTokenRefresher implements oauth2.TokenSource for Kiro's custom protocol
type kiroTokenRefresher struct {
	auth *Authenticator
	ctx  context.Context
}

func (k *kiroTokenRefresher) Token() (*oauth2.Token, error) {
	// Call internal refresh logic
	return k.auth.performRefresh(k.ctx)
}

// GetToken returns a valid access token, refreshing if necessary
func (a *Authenticator) GetToken(ctx context.Context) (string, error) {
	tokenManager := a.GetTokenManager()
	// If token manager is set, use it for token selection
	if tokenManager != nil {
		token, err := tokenManager.SelectToken()
		if err != nil {
			return "", fmt.Errorf("failed to select token from token manager: %w", err)
		}
		return token.AccessToken, nil
	}

	// Fallback to legacy credential loading
	a.mu.Lock()
	defer a.mu.Unlock()

	// Load credentials if not in memory
	if a.credentials == nil {
		creds, err := a.loadCredentials()
		if err != nil {
			return "", fmt.Errorf("credentials not found: %w", err)
		}
		a.credentials = creds
	}

	if a.credentials.AccessToken == "" {
		return "", errors.New("no access token available")
	}

	// Construct oauth2.Token
	var expiry time.Time
	if a.credentials.ExpiresAt != "" {
		if t, err := time.Parse(time.RFC3339, a.credentials.ExpiresAt); err == nil {
			expiry = t
		}
	}

	token := &oauth2.Token{
		AccessToken:  a.credentials.AccessToken,
		RefreshToken: a.credentials.RefreshToken,
		Expiry:       expiry,
		TokenType:    "Bearer",
	}

	// Check buffer (30 mins)
	buffer := time.Duration(TokenRefreshBufferMs) * time.Millisecond
	if time.Until(token.Expiry) < buffer {
		a.GetLogger().InfoLog("[Kiro Auth] Token expiring in less than 30m or expired, forcing refresh")
		token.Expiry = time.Now().Add(-1 * time.Second)
	}

	// Create TokenSource
	ts := oauth2.ReuseTokenSource(token, &kiroTokenRefresher{auth: a, ctx: ctx})

	// Get token (this triggers refresh if expired)
	newToken, err := ts.Token()
	if err != nil {
		a.GetLogger().ErrorLog("[Kiro Auth] Failed to refresh token: %v", err)
		// Continue with existing token if refresh fails??
		// Original code: "Continue with existing token if refresh fails"
		// But if it's expired, we probably shouldn't.
		// Let's return error if we really needed it.
		// But existing logic was lenient.
		// However, oauth2.ReuseTokenSource returns error if refresh fails.
		// We'll return the error.
		return "", err
	}

	// Update credentials if changed
	if newToken.AccessToken != a.credentials.AccessToken || newToken.RefreshToken != a.credentials.RefreshToken {
		a.GetLogger().InfoLog("[Kiro Auth] Token refreshed successfully, saving credentials")
		a.credentials.AccessToken = newToken.AccessToken
		// ReuseTokenSource preserves refresh token if not returned, so it should be safe.
		// But kiroTokenRefresher logic below ensures it's set in the returned token.
		a.credentials.RefreshToken = newToken.RefreshToken
		a.credentials.ExpiresAt = newToken.Expiry.Format(time.RFC3339)

		if err := a.saveCredentials(a.credentials); err != nil {
			a.GetLogger().ErrorLog("Failed to save refreshed credentials: %v", err)
		}
	}

	return a.credentials.AccessToken, nil
}

// GetTokenWithClient returns a valid access token and an HTTP client.
// The HTTP client is configured with the proxy settings from the selected token.
func (a *Authenticator) GetTokenWithClient(ctx context.Context) (string, *http.Client, error) {
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

// ForceRefresh forces a token refresh regardless of expiry
func (a *Authenticator) ForceRefresh(ctx context.Context) error {
	// Multi-token system handles refresh via TokenManager
	// This is a no-op since TokenManager handles refresh internally
	return nil
}

// performRefresh executes the custom Kiro refresh logic and returns an oauth2.Token
func (a *Authenticator) performRefresh(ctx context.Context) (*oauth2.Token, error) {
	if a.credentials == nil || a.credentials.RefreshToken == "" {
		return nil, fmt.Errorf("no refresh token available")
	}

	region := a.credentials.Region
	if region == "" {
		region = a.config.Region
	}

	// Determine refresh URL based on auth method
	var refreshURL string
	if a.credentials.AuthMethod == "social" {
		refreshURL = strings.ReplaceAll(a.config.RefreshURL, "{{region}}", region)
	} else {
		refreshURL = strings.ReplaceAll(a.config.RefreshIDCURL, "{{region}}", region)
	}

	// Build request body
	requestBody := map[string]string{
		"refreshToken": a.credentials.RefreshToken,
	}
	if a.credentials.AuthMethod != "social" {
		requestBody["clientId"] = a.credentials.ClientID
		requestBody["clientSecret"] = a.credentials.ClientSecret
		requestBody["grantType"] = "refresh_token"
	}

	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal refresh request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", refreshURL, strings.NewReader(string(bodyBytes)))
	if err != nil {
		return nil, fmt.Errorf("failed to create refresh request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client, err := a.GetHTTPClient()
	if err != nil {
		return nil, fmt.Errorf("failed to get HTTP client: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send refresh request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token refresh failed with status: %d", resp.StatusCode)
	}

	var tokenResp struct {
		AccessToken  string `json:"accessToken"`
		ExpiresIn    int64  `json:"expiresIn"`
		RefreshToken string `json:"refreshToken,omitempty"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("failed to decode refresh response: %w", err)
	}

	// Return as oauth2.Token
	// Ensure we preserve the old refresh token if new one is empty
	refreshToken := tokenResp.RefreshToken
	if refreshToken == "" {
		refreshToken = a.credentials.RefreshToken
	}

	return &oauth2.Token{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: refreshToken,
		Expiry:       time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second),
		TokenType:    "Bearer",
	}, nil
}

// Authenticate performs authentication flow
// For Kiro, we expect pre-existing credentials in the AWS SSO cache
func (a *Authenticator) Authenticate(ctx context.Context) error {
	// Try to load existing credentials
	creds, err := a.loadCredentials()
	if err != nil {
		return fmt.Errorf("Kiro authentication requires pre-existing credentials in %s. Please authenticate with Kiro IDE first", a.GetCredentialsPath())
	}

	a.mu.Lock()
	a.credentials = creds
	a.mu.Unlock()

	// Try to refresh if needed (using new GetToken logic essentially, or just GetToken)
	_, err = a.GetToken(ctx)
	if err != nil {
		a.GetLogger().ErrorLog("[Kiro Auth] Initial token check/refresh failed: %v", err)
	}

	a.mu.RLock()
	accessToken := a.credentials.AccessToken
	a.mu.RUnlock()
	if accessToken == "" {
		return fmt.Errorf("no valid access token found in credentials")
	}

	// Save to multi-token store if available
	multiTokenMgr := a.GetMultiTokenManager()
	if multiTokenMgr != nil {
		a.mu.RLock()
		// Extract email from token response
		tokenResponse := make(map[string]interface{})
		tokenResponse["access_token"] = a.credentials.AccessToken
		tokenResponse["refresh_token"] = a.credentials.RefreshToken
		tokenResponse["token_type"] = "Bearer"

		email, err := multiTokenMgr.ExtractEmail(ctx, "kiro", tokenResponse, a.credentials.AccessToken)
		if err != nil {
			a.GetLogger().WarnLog("[Kiro Auth] Failed to extract email: %v", err)
			email = ""
		}

		// Create ProviderToken with email
		now := time.Now()
		var expiry time.Time
		if a.credentials.ExpiresAt != "" {
			expiry, _ = time.Parse(time.RFC3339, a.credentials.ExpiresAt)
		}
		if expiry.IsZero() {
			expiry = now.Add(1 * time.Hour) // Default to 1 hour if no expiry
		}

		providerToken := token.ProviderToken{
			ID:           uuid.New().String(),
			AccessToken:  a.credentials.AccessToken,
			RefreshToken: a.credentials.RefreshToken,
			TokenType:    "Bearer",
			ExpiryDate:   expiry.UnixMilli(),
			Email:        email,
			Healthy:      true,
			HealthScore:  1.0,
			LastUsed:     now.UnixMilli(),
			CreatedAt:    now.UnixMilli(),
		}
		a.mu.RUnlock()

		// Save token to multi-token store
		if err := multiTokenMgr.SaveToken("kiro", providerToken); err != nil {
			a.GetLogger().WarnLog("[Kiro Auth] Failed to save token to multi-token store: %v", err)
		} else {
			a.GetLogger().InfoLog("[Kiro Auth] Credentials saved to multi-token store")
		}
	}

	a.GetLogger().DebugLog("[Kiro Auth] Authentication successful using existing credentials")
	return nil
}

// GetRegion returns the configured region
func (a *Authenticator) GetRegion() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.credentials != nil && a.credentials.Region != "" {
		return a.credentials.Region
	}
	return a.config.Region
}

// GetAuthMethod returns the authentication method used
func (a *Authenticator) GetAuthMethod() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.credentials != nil {
		return a.credentials.AuthMethod
	}
	return ""
}

// GetProfileArn returns the profile ARN if available
func (a *Authenticator) GetProfileArn() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.credentials != nil {
		return a.credentials.ProfileArn
	}
	return ""
}
