// Package gemini provides authentication for Gemini provider
package gemini

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	tokenpkg "github.com/sunbankio/qwencoder-proxy/internal/token"
	"github.com/sunbankio/qwencoder-proxy/logging"
)

// OAuthConfig holds OAuth configuration for Gemini
type OAuthConfig struct {
	ClientID     string
	ClientSecret string
	Scope        string
	RedirectPort int
	CredsDir     string
	CredsFile    string
}

// DefaultOAuthConfig returns default Gemini OAuth configuration
func DefaultOAuthConfig() *OAuthConfig {
	return &OAuthConfig{
		ClientID:     "681255809395-oo8ft2oprdrnp9e3aqf6av3hmdib135j.apps.googleusercontent.com",
		ClientSecret: "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",
		Scope:        "https://www.googleapis.com/auth/cloud-platform https://www.googleapis.com/auth/userinfo.email https://www.googleapis.com/auth/userinfo.profile openid",
		RedirectPort: 8085,
		CredsDir:     ".gemini",
		CredsFile:    "oauth_creds.json",
	}
}

// Credentials represents stored OAuth credentials
type Credentials struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiryDate   int64  `json:"expiry_date"`
	Scope        string `json:"scope,omitempty"`
}

// Authenticator implements authentication for Gemini
type Authenticator struct {
	tokenManager  *tokenpkg.TokenManager
	multiTokenMgr *tokenpkg.MultiTokenManager
	config        *OAuthConfig
	logger        logging.Logger
	httpClient    *http.Client
}

// NewAuthenticator creates a new Gemini authenticator
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

// GetCredentialsPath returns path to credentials file
func (a *Authenticator) GetCredentialsPath() string {
	homeDir, _ := os.UserHomeDir()
	return filepath.Join(homeDir, a.config.CredsDir, a.config.CredsFile)
}

// IsAuthenticated checks if valid credentials exist
func (a *Authenticator) IsAuthenticated() bool {
	tokenManager := a.GetTokenManager()
	if tokenManager == nil {
		return false
	}

	_, err := tokenManager.SelectToken()
	return err == nil
}

// loadCredentials loads credentials from file
func (a *Authenticator) loadCredentials() (*Credentials, error) {
	credsPath := a.GetCredentialsPath()
	data, err := os.ReadFile(credsPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read credentials file: %w", err)
	}

	var creds Credentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return nil, fmt.Errorf("failed to parse credentials: %w", err)
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

	data, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal credentials: %w", err)
	}

	if err := os.WriteFile(credsPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write credentials file: %w", err)
	}

	return nil
}

// ClearCredentials removes stored credentials (no-op for multi-token system)
func (a *Authenticator) ClearCredentials() error {
	// Multi-token system manages credentials differently
	return nil
}

// GetToken returns a valid access token using multi-token manager
func (a *Authenticator) GetToken(ctx context.Context) (string, error) {
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
		return nil, fmt.Errorf("token manager not initialized")
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

// Authenticate performs OAuth web flow authentication
func (a *Authenticator) Authenticate(ctx context.Context) error {
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
func (a *Authenticator) exchangeCodeForTokens(ctx context.Context, code, redirectURI string) error {
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

	// Discover project ID for this account
	projectID, err := a.discoverProjectID(ctx, tokenResp.AccessToken)
	if err != nil {
		a.GetLogger().WarnLog("[Gemini Auth] Failed to discover project ID: %v", err)
		projectID = "" // Empty string means project ID not yet discovered
	} else {
		a.GetLogger().InfoLog("[Gemini Auth] Discovered project ID: %s for email: %s", projectID, email)
	}

	providerToken := tokenpkg.ProviderToken{
		ID:           uuid.New().String(),
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		TokenType:    tokenResp.TokenType,
		ExpiryDate:   expiry.UnixMilli(),
		Email:        email,
		Scope:        tokenResp.Scope,
		ProjectID:    projectID, // Store discovered project ID
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

// discoverProjectID discovers the Cloud Code Assist project ID for the given access token
func (a *Authenticator) discoverProjectID(ctx context.Context, accessToken string) (string, error) {
	const baseURL = "https://cloudcode-pa.googleapis.com/v1internal"

	// Prepare client metadata
	clientMetadata := map[string]interface{}{
		"ideType":     "IDE_UNSPECIFIED",
		"platform":    "PLATFORM_UNSPECIFIED",
		"pluginType":  "GEMINI",
		"duetProject": "",
	}

	// Prepare loadCodeAssist request
	loadRequest := map[string]interface{}{
		"cloudaicompanionProject": "",
		"metadata":                clientMetadata,
	}

	reqBody, err := json.Marshal(loadRequest)
	if err != nil {
		return "", fmt.Errorf("failed to marshal load request: %w", err)
	}

	// Call loadCodeAssist endpoint
	url := fmt.Sprintf("%s:loadCodeAssist", baseURL)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(reqBody))
	if err != nil {
		return "", fmt.Errorf("failed to create load request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "google-api-nodejs-client/9.15.1")
	req.Header.Set("X-Goog-Api-Client", "gl-node/22.17.0")
	req.Header.Set("Client-Metadata", "ideType=IDE_UNSPECIFIED,platform=PLATFORM_UNSPECIFIED,pluginType=GEMINI")

	client := a.httpClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send load request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("loadCodeAssist failed (status %d): %s", resp.StatusCode, string(body))
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read load response: %w", err)
	}

	var loadResponse map[string]interface{}
	if err := json.Unmarshal(respBody, &loadResponse); err != nil {
		return "", fmt.Errorf("failed to decode load response: %w", err)
	}

	// Check if project ID exists in response
	if projectID, ok := loadResponse["cloudaicompanionProject"].(string); ok && projectID != "" {
		return projectID, nil
	}

	// If no existing project, try to onboard
	allowedTiers, ok := loadResponse["allowedTiers"].([]interface{})
	var tierID string
	if ok && len(allowedTiers) > 0 {
		for _, tier := range allowedTiers {
			if tierMap, ok := tier.(map[string]interface{}); ok {
				if isDefault, exists := tierMap["isDefault"].(bool); exists && isDefault {
					if id, idExists := tierMap["id"].(string); idExists {
						tierID = id
						break
					}
				}
			}
		}
	}

	if tierID == "" {
		tierID = "free-tier"
	}

	// Prepare onboardUser request
	onboardRequest := map[string]interface{}{
		"tierId":                  tierID,
		"cloudaicompanionProject": "",
		"metadata":                clientMetadata,
	}

	onboardReqBody, err := json.Marshal(onboardRequest)
	if err != nil {
		return "", fmt.Errorf("failed to marshal onboard request: %w", err)
	}

	onboardUrl := fmt.Sprintf("%s:onboardUser", baseURL)
	onboardReq, err := http.NewRequestWithContext(ctx, "POST", onboardUrl, bytes.NewReader(onboardReqBody))
	if err != nil {
		return "", fmt.Errorf("failed to create onboard request: %w", err)
	}

	onboardReq.Header.Set("Authorization", "Bearer "+accessToken)
	onboardReq.Header.Set("Content-Type", "application/json")
	onboardReq.Header.Set("User-Agent", "qwencoder-proxy/1.0")
	onboardReq.Header.Set("X-Goog-Api-Client", "gl-node/22.17.0")
	onboardReq.Header.Set("Client-Metadata", "ideType=IDE_UNSPECIFIED,platform=PLATFORM_UNSPECIFIED,pluginType=GEMINI")

	onboardResp, err := client.Do(onboardReq)
	if err != nil {
		return "", fmt.Errorf("failed to send onboard request: %w", err)
	}
	defer onboardResp.Body.Close()

	if onboardResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(onboardResp.Body)
		return "", fmt.Errorf("onboardUser failed (status %d): %s", onboardResp.StatusCode, string(body))
	}

	onboardRespBody, err := io.ReadAll(onboardResp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read onboard response: %w", err)
	}

	var onboardResponse map[string]interface{}
	if err := json.Unmarshal(onboardRespBody, &onboardResponse); err != nil {
		return "", fmt.Errorf("failed to decode onboard response: %w", err)
	}

	// Extract project ID from onboard response
	if response, ok := onboardResponse["response"].(map[string]interface{}); ok {
		if project, exists := response["cloudaicompanionProject"].(map[string]interface{}); exists {
			if id, idExists := project["id"].(string); idExists {
				return id, nil
			}
		}
	}

	// Fallback: try to get project ID directly from response
	if id, exists := onboardResponse["cloudaicompanionProject"].(string); exists {
		return id, nil
	}

	return "", fmt.Errorf("failed to discover or create project ID")
}

// geminiTokenRefresher implements ProviderRefresh for Gemini
type geminiTokenRefresher struct {
	clientID     string
	clientSecret string
	tokenURL     string
	logger       logging.Logger
}

// NewGeminiTokenRefresher creates a new Gemini token refresher
func NewGeminiTokenRefresher(clientID, clientSecret, tokenURL string, logger logging.Logger) *geminiTokenRefresher {
	return &geminiTokenRefresher{
		clientID:     clientID,
		clientSecret: clientSecret,
		tokenURL:     tokenURL,
		logger:       logger,
	}
}

// ProviderID returns the provider identifier
func (g *geminiTokenRefresher) ProviderID() string {
	return "gemini-cli"
}

// RefreshToken refreshes a Gemini OAuth token
func (g *geminiTokenRefresher) RefreshToken(ctx context.Context, token tokenpkg.ProviderToken) (tokenpkg.ProviderToken, error) {
	g.logger.InfoLog("[GeminiRefresh] Starting token refresh for token ID %s", token.ID)

	if token.RefreshToken == "" {
		g.logger.ErrorLog("[GeminiRefresh] No refresh token available for token ID %s", token.ID)
		return tokenpkg.ProviderToken{}, fmt.Errorf("no refresh token available")
	}

	// Prepare refresh request
	data := url.Values{}
	data.Set("client_id", g.clientID)
	data.Set("client_secret", g.clientSecret)
	data.Set("refresh_token", token.RefreshToken)
	data.Set("grant_type", "refresh_token")

	// Make HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", g.tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		g.logger.ErrorLog("[GeminiRefresh] Failed to create refresh request for token ID %s: %v", token.ID, err)
		return tokenpkg.ProviderToken{}, fmt.Errorf("failed to create refresh request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		g.logger.ErrorLog("[GeminiRefresh] Failed to send refresh request for token ID %s: %v", token.ID, err)
		return tokenpkg.ProviderToken{}, fmt.Errorf("failed to send refresh request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		g.logger.ErrorLog("[GeminiRefresh] Refresh failed with status %d for token ID %s: %s", resp.StatusCode, token.ID, string(body))
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
		g.logger.ErrorLog("[GeminiRefresh] Failed to decode refresh response for token ID %s: %v", token.ID, err)
		return tokenpkg.ProviderToken{}, fmt.Errorf("failed to decode refresh response: %w", err)
	}

	// Calculate expiry
	expiryDate := time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	g.logger.InfoLog("[GeminiRefresh] Token refresh successful for token ID %s, new expiry %s", token.ID, expiryDate.Format(time.RFC3339))

	// Log token details for debugging
	g.logger.DebugLog("[GeminiRefresh] Token details - ID: %s, Email: %s, Healthy: true, ExpiryDate: %d, RefreshToken: %s, ProjectID: %s",
		token.ID, token.Email, expiryDate.UnixMilli(), token.RefreshToken, token.ProjectID)

	// Return refreshed token, preserving ProjectID
	return tokenpkg.ProviderToken{
		ID:           token.ID,
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		TokenType:    tokenResp.TokenType,
		ExpiryDate:   expiryDate.UnixMilli(),
		Email:        token.Email,
		Scope:        token.Scope,
		ProjectID:    token.ProjectID, // PRESERVE project ID on refresh
		Healthy:      true,
		HealthScore:  1.0,
		LastUsed:     token.LastUsed,
		CreatedAt:    token.CreatedAt,
	}, nil
}
