// Package qwen provides device flow authentication for Qwen provider
package qwen

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/google/uuid"
	tokenpkg "github.com/sunbankio/qwencoder-proxy/internal/token"
	"github.com/sunbankio/qwencoder-proxy/internal/logging"
	"golang.org/x/oauth2"
)

const (
	// OAuthTokenURL is the OAuth token URL
	OAuthTokenURL = "https://chat.qwen.ai/api/v1/oauth2/token"
	// OAuthClientID is the OAuth client ID
	OAuthClientID = "f0304373b74a44d2b584a3fb70ca9e56"
	// OAuthScope is the OAuth scope
	OAuthScope = "openid profile email model.completion"
	// OAuthDeviceAuthURL is the OAuth device authorization URL
	OAuthDeviceAuthURL = "https://chat.qwen.ai/api/v1/oauth2/device/code"
)

// AuthenticateWithDeviceFlow handles the OAuth 2.0 device authorization flow using the golang.org/x/oauth2 package.
// It requires a MultiTokenManager to save the token using the multi-token store.
func AuthenticateWithDeviceFlow(ctx context.Context, logger logging.Logger, multiTokenMgr *tokenpkg.MultiTokenManager) error {
	conf := &oauth2.Config{
		ClientID: OAuthClientID,
		Scopes:   []string{OAuthScope},
		Endpoint: oauth2.Endpoint{
			TokenURL:      OAuthTokenURL,
			DeviceAuthURL: OAuthDeviceAuthURL,
		},
	}

	if ctx == nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
	}

	if logger == nil {
		logger = logging.NewLogger()
	}

	codeVerifier, err := generateCodeVerifier()
	if err != nil {
		return fmt.Errorf("failed to generate code verifier: %w", err)
	}
	codeChallenge := generateCodeChallenge(codeVerifier)

	deviceAuthResponse, err := conf.DeviceAuth(ctx,
		oauth2.SetAuthURLParam("code_challenge", codeChallenge),
		oauth2.SetAuthURLParam("code_challenge_method", "S256"),
	)
	if err != nil {
		return fmt.Errorf("failed to start device auth flow: %w", err)
	}

	// Construct verification URL with user code and client parameter
	// Use "qwen-code" as the client parameter value
	var verificationURL string
	if deviceAuthResponse.VerificationURIComplete != "" {
		verificationURL = deviceAuthResponse.VerificationURIComplete
	} else {
		verificationURL = fmt.Sprintf("%s?user_code=%s&client=qwen-code", deviceAuthResponse.VerificationURI, deviceAuthResponse.UserCode)
	}

	// Try to open the verification URI in the browser
	if err := openBrowser(verificationURL); err != nil {
		logger.WarnLog("Failed to open browser automatically: %v. Please open the URL manually.", err)
	}

	fmt.Printf("\n=== Qwen OAuth Authentication ===\n")
	fmt.Printf("If your browser didn't open, please go to: %s\n", verificationURL)
	fmt.Printf("And enter this code: %s\n\n", deviceAuthResponse.UserCode)
	fmt.Println("Waiting for authorization...")

	oauth2Token, err := conf.DeviceAccessToken(ctx, deviceAuthResponse, oauth2.SetAuthURLParam("code_verifier", codeVerifier))
	if err != nil {
		return fmt.Errorf("failed to get token: %w", err)
	}

	// Ensure multi-token manager is initialized
	if multiTokenMgr == nil {
		return fmt.Errorf("multi-token manager is required but was nil")
	}

	if err := multiTokenMgr.Initialize(); err != nil {
		return fmt.Errorf("failed to initialize multi-token manager: %w", err)
	}

	// Extract email from token response
	tokenResponse := make(map[string]interface{})
	tokenResponse["access_token"] = oauth2Token.AccessToken
	tokenResponse["refresh_token"] = oauth2Token.RefreshToken
	tokenResponse["token_type"] = oauth2Token.TokenType
	tokenResponse["expires_in"] = int64(oauth2Token.Expiry.Sub(time.Now()).Seconds())

	email, err := multiTokenMgr.ExtractEmail(ctx, "qwen", tokenResponse, oauth2Token.AccessToken)
	if err != nil {
		logger.WarnLog("[Qwen OAuth] Failed to extract email: %v", err)
		email = ""
	}

	// Create ProviderToken with email
	now := time.Now()
	providerToken := tokenpkg.ProviderToken{
		ID:           uuid.New().String(),
		AccessToken:  oauth2Token.AccessToken,
		RefreshToken: oauth2Token.RefreshToken,
		TokenType:    oauth2Token.TokenType,
		ExpiryDate:   oauth2Token.Expiry.UnixMilli(),
		Email:        email,
		Healthy:      true,
		HealthScore:  1.0,
		LastUsed:     now.UnixMilli(),
		CreatedAt:    now.UnixMilli(),
	}

	// Extract resource URL if available
	if resourceURL, ok := oauth2Token.Extra("resource_url").(string); ok {
		providerToken.ResourceURL = resourceURL
	}

	// Save token to multi-token store
	if err := multiTokenMgr.SaveToken("qwen", providerToken); err != nil {
		return fmt.Errorf("failed to save token to multi-token store: %w", err)
	}

	fmt.Println("Authentication successful! Credentials saved.")
	return nil
}

// openBrowser opens the default browser with the given URL.
func openBrowser(url string) error {
	var err error
	switch runtime.GOOS {
	case "linux":
		err = exec.Command("xdg-open", url).Start()
	case "windows":
		err = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		err = exec.Command("open", url).Start()
	default:
		err = fmt.Errorf("unsupported platform")
	}
	return err
}

// generateCodeVerifier generates a random code verifier for PKCE.
func generateCodeVerifier() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// generateCodeChallenge generates a code challenge from a code verifier using SHA-256.
func generateCodeChallenge(codeVerifier string) string {
	h := sha256.New()
	h.Write([]byte(codeVerifier))
	return base64.RawURLEncoding.EncodeToString(h.Sum(nil))
}

// qwenTokenRefresher implements ProviderRefresh for Qwen
type qwenTokenRefresher struct {
	clientID string
	tokenURL string
	logger   logging.Logger
}

// NewQwenTokenRefresher creates a new Qwen token refresher
func NewQwenTokenRefresher(logger logging.Logger) *qwenTokenRefresher {
	return &qwenTokenRefresher{
		clientID: OAuthClientID,
		tokenURL: OAuthTokenURL,
		logger:   logger,
	}
}

// ProviderID returns the provider identifier
func (q *qwenTokenRefresher) ProviderID() string {
	return "qwen"
}

// RefreshToken refreshes a Qwen OAuth token
func (q *qwenTokenRefresher) RefreshToken(ctx context.Context, token tokenpkg.ProviderToken) (tokenpkg.ProviderToken, error) {
	q.logger.InfoLog("[QwenRefresh] Starting token refresh for token ID %s", token.ID)

	if token.RefreshToken == "" {
		q.logger.ErrorLog("[QwenRefresh] No refresh token available for token ID %s", token.ID)
		return tokenpkg.ProviderToken{}, fmt.Errorf("no refresh token available")
	}

	// Prepare refresh request
	data := url.Values{}
	data.Set("client_id", q.clientID)
	data.Set("refresh_token", token.RefreshToken)
	data.Set("grant_type", "refresh_token")

	// Make HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", q.tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		q.logger.ErrorLog("[QwenRefresh] Failed to create refresh request for token ID %s: %v", token.ID, err)
		return tokenpkg.ProviderToken{}, fmt.Errorf("failed to create refresh request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		q.logger.ErrorLog("[QwenRefresh] Failed to send refresh request for token ID %s: %v", token.ID, err)
		return tokenpkg.ProviderToken{}, fmt.Errorf("failed to send refresh request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		q.logger.ErrorLog("[QwenRefresh] Refresh failed with status %d for token ID %s: %s", resp.StatusCode, token.ID, string(body))
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
		q.logger.ErrorLog("[QwenRefresh] Failed to decode refresh response for token ID %s: %v", token.ID, err)
		return tokenpkg.ProviderToken{}, fmt.Errorf("failed to decode refresh response: %w", err)
	}

	// Calculate expiry
	expiryDate := time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	q.logger.InfoLog("[QwenRefresh] Token refresh successful for token ID %s, new expiry %s", token.ID, expiryDate.Format(time.RFC3339))

	// Return refreshed token
	return tokenpkg.ProviderToken{
		ID:           token.ID,
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		TokenType:    tokenResp.TokenType,
		ExpiryDate:   expiryDate.UnixMilli(),
		Email:        token.Email,
		ResourceURL:  token.ResourceURL,
		Healthy:      true,
		HealthScore:  1.0,
		LastUsed:     token.LastUsed,
		CreatedAt:    token.CreatedAt,
	}, nil
}
