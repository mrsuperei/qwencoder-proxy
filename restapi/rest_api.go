// Package restapi provides REST API endpoints for OAuth2 authentication flows
package restapi

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
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sunbankio/qwencoder-proxy/auth"
	"github.com/sunbankio/qwencoder-proxy/logging"
	"golang.org/x/oauth2"
)

// Config holds the configuration for the REST API server
type Config struct {
	Port            string        // Server port
	CallbackBaseURL string        // Base URL for callbacks (e.g., "http://localhost:8080")
	StateTTL        time.Duration // OAuth state TTL
	DeviceCodeTTL   time.Duration // Device code TTL
	EnableCORS      bool          // Enable CORS
	AllowedOrigins  []string      // CORS allowed origins
	DashboardDir    string        // Dashboard directory path (empty = auto-detect)
}

// DefaultConfig returns the default configuration
func DefaultConfig() *Config {
	return &Config{
		Port:            "8080",
		CallbackBaseURL: "http://localhost:8080",
		StateTTL:        10 * time.Minute,
		DeviceCodeTTL:   15 * time.Minute,
		EnableCORS:      false,
		AllowedOrigins:  []string{"*"},
	}
}

// Server represents the OAuth REST API server
type Server struct {
	config       *Config
	registry     *ProviderRegistry
	stateManager *StateManager
	logger       *logging.Logger
	httpClient   *http.Client
}

// NewServer creates a new OAuth REST API server
func NewServer(config *Config, logger *logging.Logger) *Server {
	if config == nil {
		config = DefaultConfig()
	}
	if logger == nil {
		// Create a simple logger if none provided
		logger = &logging.Logger{}
	}

	return &Server{
		config:       config,
		registry:     NewProviderRegistry(),
		stateManager: NewStateManager(),
		logger:       logger,
		httpClient:   &http.Client{Timeout: 30 * time.Second},
	}
}

// Start starts the HTTP server
func (s *Server) Start() error {
	// Log working directory for debugging
	wd, err := os.Getwd()
	if err != nil {
		s.logger.ErrorLog("Failed to get working directory: %v", err)
	} else {
		s.logger.InfoLog("Working directory: %s", wd)
	}

	mux := http.NewServeMux()

	// Register routes
	s.registerRoutes(mux)

	// Apply middleware
	var handler http.Handler = mux
	if s.config.EnableCORS {
		handler = CORS(s.config.AllowedOrigins)(handler)
	}
	handler = Logging(s.logger)(handler)

	addr := ":" + s.config.Port
	s.logger.InfoLog("Starting OAuth REST API server on %s", addr)
	s.logger.InfoLog("Callback URL: %s/api/callback", s.config.CallbackBaseURL)

	return http.ListenAndServe(addr, handler)
}

// RegisterRoutes registers OAuth API routes with an external http.ServeMux
// This allows the OAuth server to be integrated into a larger server
func (s *Server) RegisterRoutes(mux *http.ServeMux) {
	s.registerRoutes(mux)
}

// registerRoutes registers all API routes
func (s *Server) registerRoutes(mux *http.ServeMux) {
	// Resolve dashboard directory relative to executable location
	// This ensures the dashboard can be served regardless of working directory
	dashboardDir, err := s.resolveDashboardDir()
	if err != nil {
		s.logger.ErrorLog("Failed to resolve dashboard directory: %v", err)
		dashboardDir = "web/dashboard" // Fallback to relative path
	} else {
		s.logger.InfoLog("Dashboard directory: %s", dashboardDir)
	}

	// Handle root path - serve index.html
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// If the path is exactly "/", serve index.html
		if r.URL.Path == "/" {
			filePath := filepath.Join(dashboardDir, "index.html")
			absPath, err := filepath.Abs(filePath)
			if err != nil {
				s.logger.ErrorLog("Failed to get absolute path for %s: %v", filePath, err)
			} else {
				s.logger.InfoLog("Attempting to serve file: %s (absolute: %s)", filePath, absPath)
			}

			// Check if file exists
			if _, err := os.Stat(filePath); os.IsNotExist(err) {
				s.logger.ErrorLog("File does not exist: %s", filePath)
				http.Error(w, "File not found", http.StatusNotFound)
				return
			}

			http.ServeFile(w, r, filePath)
			return
		}
		// For other paths, serve from the dashboard directory
		filePath := filepath.Join(dashboardDir, r.URL.Path)
		s.logger.InfoLog("Attempting to serve file: %s", filePath)
		http.ServeFile(w, r, filePath)
	})

	// Provider discovery
	mux.HandleFunc("/api/providers", s.handleProviders)
	mux.HandleFunc("/api/providers/", s.handleProviderConfig)

	// Device code flow
	mux.HandleFunc("/api/device/start", s.handleDeviceStart)
	mux.HandleFunc("/api/device/status/", s.handleDeviceStatus)

	// Authorization code flow
	mux.HandleFunc("/api/auth/start", s.handleAuthStart)
	mux.HandleFunc("/api/callback", s.handleCallback)

	// Token management
	mux.HandleFunc("/api/token/", s.handleToken)

	// Credentials management
	mux.HandleFunc("/api/credentials", s.handleCredentials)
}

// handleProviders returns a list of all available providers
func (s *Server) handleProviders(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Only GET method is allowed")
		return
	}

	providers := s.registry.ListProviders()
	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"providers": providers,
	})
}

// handleProviderConfig returns configuration for a specific provider
func (s *Server) handleProviderConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Only GET method is allowed")
		return
	}

	// Extract provider ID from path
	// Path format: /api/providers/{provider}/config
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 5 || parts[4] != "config" {
		WriteError(w, http.StatusNotFound, "not_found", "Endpoint not found")
		return
	}

	providerID := parts[3]
	config, err := s.registry.GetConfig(providerID)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_provider", err.Error())
		return
	}

	info := ProviderInfo{
		ID:     config.ID,
		Name:   config.Name,
		Flow:   config.Flow,
		Scopes: config.Scopes,
	}

	if config.AuthURL != "" {
		info.AuthURL = config.AuthURL
	}
	if config.TokenURL != "" {
		info.TokenURL = config.TokenURL
	}
	if config.Description != "" {
		info.Description = config.Description
	}

	WriteJSON(w, http.StatusOK, info)
}

// handleDeviceStart initiates a device code flow
func (s *Server) handleDeviceStart(w http.ResponseWriter, r *http.Request) {
	s.logger.InfoLog("[handleDeviceStart] Received device start request")

	if r.Method != http.MethodPost {
		WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Only POST method is allowed")
		return
	}

	var req struct {
		Provider string `json:"provider"`
	}
	if err := ParseJSON(r, &req); err != nil {
		s.logger.ErrorLog("[handleDeviceStart] Failed to parse request: %v", err)
		WriteError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	s.logger.InfoLog("[handleDeviceStart] Starting device flow for provider: %s", req.Provider)

	if req.Provider == "" {
		s.logger.ErrorLog("[handleDeviceStart] Provider is empty")
		WriteError(w, http.StatusBadRequest, "invalid_request", "Provider is required")
		return
	}

	config, err := s.registry.GetConfig(req.Provider)
	if err != nil {
		s.logger.ErrorLog("[handleDeviceStart] Failed to get config for provider %s: %v", req.Provider, err)
		WriteError(w, http.StatusBadRequest, "invalid_provider", err.Error())
		return
	}

	s.logger.InfoLog("[handleDeviceStart] Provider config: ID=%s, Flow=%s, TokenURL=%s, DeviceAuthURL=%s",
		config.ID, config.Flow, config.TokenURL, config.DeviceAuthURL)

	if config.Flow != "device_code" {
		s.logger.ErrorLog("[handleDeviceStart] Provider %s has flow %s, not device_code", req.Provider, config.Flow)
		WriteError(w, http.StatusBadRequest, "invalid_flow", fmt.Sprintf("Provider %s does not support device code flow", req.Provider))
		return
	}

	// Generate PKCE codes
	codeVerifier, err := generateCodeVerifier()
	if err != nil {
		s.logger.ErrorLog("[handleDeviceStart] Failed to generate code verifier: %v", err)
		WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to generate code verifier")
		return
	}
	codeChallenge := generateCodeChallenge(codeVerifier)

	s.logger.InfoLog("[handleDeviceStart] PKCE generated - verifier length: %d, challenge length: %d",
		len(codeVerifier), len(codeChallenge))

	// Request device code
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	oauthConfig := &oauth2.Config{
		ClientID: config.ClientID,
		Scopes:   config.Scopes,
		Endpoint: oauth2.Endpoint{
			TokenURL:      config.TokenURL,
			DeviceAuthURL: config.DeviceAuthURL,
		},
	}

	s.logger.InfoLog("[handleDeviceStart] Requesting device code from provider %s", req.Provider)

	deviceAuthResp, err := oauthConfig.DeviceAuth(ctx,
		oauth2.SetAuthURLParam("code_challenge", codeChallenge),
		oauth2.SetAuthURLParam("code_challenge_method", "S256"),
	)
	if err != nil {
		s.logger.ErrorLog("[handleDeviceStart] Failed to get device auth response from %s: %v", req.Provider, err)
		WriteError(w, http.StatusInternalServerError, "device_auth_failed", err.Error())
		return
	}

	// Add nil check for deviceAuthResp
	if deviceAuthResp == nil {
		s.logger.ErrorLog("[handleDeviceStart] CRITICAL: Device auth response is nil!")
		WriteError(w, http.StatusInternalServerError, "internal_error", "Device auth response is nil")
		return
	}

	s.logger.InfoLog("[handleDeviceStart] Device auth response received - DeviceCode: %s, UserCode: %s, Interval: %d, Expiry: %v",
		deviceAuthResp.DeviceCode, deviceAuthResp.UserCode, deviceAuthResp.Interval, deviceAuthResp.Expiry)

	// Create poll state
	pollID, err := s.stateManager.CreatePoll(
		req.Provider,
		deviceAuthResp.DeviceCode,
		time.Duration(deviceAuthResp.Interval)*time.Second,
		s.config.DeviceCodeTTL,
	)
	if err != nil {
		s.logger.ErrorLog("[handleDeviceStart] Failed to create poll state: %v", err)
		WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to create poll state")
		return
	}

	s.logger.InfoLog("[handleDeviceStart] Poll state created - pollID: %s, DeviceCode: %s, Interval: %d seconds, TTL: %v",
		pollID, deviceAuthResp.DeviceCode, deviceAuthResp.Interval, s.config.DeviceCodeTTL)

	// Start background polling for token with panic recovery
	s.logger.InfoLog("[handleDeviceStart] Starting background goroutine for polling...")
	go func() {
		defer func() {
			if r := recover(); r != nil {
				s.logger.ErrorLog("[pollForToken] Panic recovered: %v", r)
				s.stateManager.UpdatePollError(pollID, "panic", fmt.Sprintf("%v", r))
			}
		}()
		// Create a new context for polling with the device code TTL
		pollCtx, pollCancel := context.WithTimeout(context.Background(), s.config.DeviceCodeTTL)
		defer pollCancel()
		s.pollForToken(pollCtx, pollID, req.Provider, config, codeVerifier, deviceAuthResp)
	}()

	response := map[string]interface{}{
		"device_code":               deviceAuthResp.DeviceCode,
		"user_code":                 deviceAuthResp.UserCode,
		"verification_uri":          deviceAuthResp.VerificationURI,
		"verification_uri_complete": deviceAuthResp.VerificationURIComplete,
		"expires_in":                int(deviceAuthResp.Expiry.Sub(time.Now()).Seconds()),
		"interval":                  deviceAuthResp.Interval,
		"poll_id":                   pollID,
	}

	s.logger.InfoLog("[handleDeviceStart] Sending response to client - pollID: %s, user_code: %s", pollID, deviceAuthResp.UserCode)
	WriteJSON(w, http.StatusOK, response)
}

// pollForToken polls for token completion in the background
func (s *Server) pollForToken(ctx context.Context, pollID, providerID string, config *ProviderConfig, codeVerifier string, deviceAuthResp *oauth2.DeviceAuthResponse) {
	s.logger.InfoLog("[pollForToken] Starting poll for token - pollID: %s, provider: %s", pollID, providerID)

	// Check if deviceAuthResp is nil
	if deviceAuthResp == nil {
		s.logger.ErrorLog("[pollForToken] CRITICAL: deviceAuthResp is nil!")
		s.stateManager.UpdatePollError(pollID, "internal_error", "deviceAuthResp is nil")
		return
	}

	// Log device auth response details for debugging (after nil check)
	s.logger.InfoLog("[pollForToken] DeviceCode: %s, UserCode: %s, Interval: %d, Expiry: %v",
		deviceAuthResp.DeviceCode, deviceAuthResp.UserCode, deviceAuthResp.Interval, deviceAuthResp.Expiry)

	// Add interval validation
	interval := deviceAuthResp.Interval
	if interval <= 0 {
		interval = 5 // Default to 5 seconds
		s.logger.InfoLog("[pollForToken] Using default interval of %d seconds", interval)
	}

	oauthConfig := &oauth2.Config{
		ClientID: config.ClientID,
		Scopes:   config.Scopes,
		Endpoint: oauth2.Endpoint{
			TokenURL:      config.TokenURL,
			DeviceAuthURL: config.DeviceAuthURL,
		},
	}

	ticker := time.NewTicker(time.Duration(interval) * time.Second)
	defer ticker.Stop()

	s.logger.InfoLog("[pollForToken] Ticker created with interval: %d seconds", interval)

	for {
		select {
		case <-ctx.Done():
			s.stateManager.UpdatePollError(pollID, "timeout", "Authorization timed out")
			return
		case <-ticker.C:
			token, err := oauthConfig.DeviceAccessToken(ctx, deviceAuthResp,
				oauth2.SetAuthURLParam("code_verifier", codeVerifier),
			)

			if err != nil {
				if strings.Contains(err.Error(), "authorization_pending") {
					// Continue polling - user hasn't authorized yet
					continue
				}
				if strings.Contains(err.Error(), "authorization_declined") {
					s.stateManager.UpdatePollError(pollID, "declined", "Authorization declined by user")
					return
				}
				if strings.Contains(err.Error(), "expired_token") {
					s.stateManager.UpdatePollError(pollID, "expired", "Device code expired")
					return
				}
				// Other errors
				s.stateManager.UpdatePollError(pollID, "auth_failed", err.Error())
				return
			}

			// Success - save credentials
			creds := auth.OAuthCreds{
				AccessToken:  token.AccessToken,
				TokenType:    token.TokenType,
				RefreshToken: token.RefreshToken,
				ExpiryDate:   token.Expiry.UnixMilli(),
			}

			if resourceURL, ok := token.Extra("resource_url").(string); ok {
				creds.ResourceURL = resourceURL
			}

			if err := s.saveCredentials(providerID, creds); err != nil {
				s.logger.ErrorLog("Failed to save credentials for %s: %v", providerID, err)
				s.stateManager.UpdatePollError(pollID, "save_failed", err.Error())
				return
			}

			s.stateManager.UpdatePoll(pollID, "authorized", &creds)
			return
		}
	}
}

// handleDeviceStatus polls for device code status
func (s *Server) handleDeviceStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Only GET method is allowed")
		return
	}

	// Extract poll ID from path
	// Path format: /api/device/status/{poll_id}
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 5 {
		WriteError(w, http.StatusNotFound, "not_found", "Poll ID not found")
		return
	}

	pollID := parts[4]

	poll, err := s.stateManager.GetPoll(pollID)
	if err != nil {
		WriteError(w, http.StatusNotFound, "poll_not_found", err.Error())
		return
	}

	if poll.Status == "authorized" && poll.Tokens != nil {
		response := map[string]interface{}{
			"status":        "authorized",
			"access_token":  poll.Tokens.AccessToken,
			"refresh_token": poll.Tokens.RefreshToken,
			"token_type":    poll.Tokens.TokenType,
			"expires_in":    (poll.Tokens.ExpiryDate - time.Now().UnixMilli()) / 1000,
			"expiry_date":   poll.Tokens.ExpiryDate,
		}
		if poll.Tokens.ResourceURL != "" {
			response["resource_url"] = poll.Tokens.ResourceURL
		}
		WriteJSON(w, http.StatusOK, response)
	} else if poll.Status == "error" {
		WriteJSON(w, http.StatusOK, map[string]interface{}{
			"status":            "error",
			"error":             poll.ErrorCode,
			"error_description": poll.ErrorMessage,
		})
	} else {
		WriteJSON(w, http.StatusOK, map[string]interface{}{
			"status":     "pending",
			"expires_in": int(poll.ExpiresAt.Sub(time.Now()).Seconds()),
		})
	}
}

// handleAuthStart initiates an authorization code flow
func (s *Server) handleAuthStart(w http.ResponseWriter, r *http.Request) {
	s.logger.InfoLog("[handleAuthStart] Received auth start request")

	if r.Method != http.MethodPost {
		WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Only POST method is allowed")
		return
	}

	var req struct {
		Provider    string `json:"provider"`
		RedirectURI string `json:"redirect_uri,omitempty"`
	}
	if err := ParseJSON(r, &req); err != nil {
		s.logger.ErrorLog("[handleAuthStart] Failed to parse request: %v", err)
		WriteError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	s.logger.InfoLog("[handleAuthStart] Starting authorization code flow for provider: %s", req.Provider)

	if req.Provider == "" {
		s.logger.ErrorLog("[handleAuthStart] Provider is empty")
		WriteError(w, http.StatusBadRequest, "invalid_request", "Provider is required")
		return
	}

	config, err := s.registry.GetConfig(req.Provider)
	if err != nil {
		s.logger.ErrorLog("[handleAuthStart] Failed to get config for provider %s: %v", req.Provider, err)
		WriteError(w, http.StatusBadRequest, "invalid_provider", err.Error())
		return
	}

	s.logger.InfoLog("[handleAuthStart] Provider config: ID=%s, Flow=%s, AuthURL=%s, TokenURL=%s",
		config.ID, config.Flow, config.AuthURL, config.TokenURL)

	if config.Flow != "authorization_code" {
		s.logger.ErrorLog("[handleAuthStart] Provider %s has flow %s, not authorization_code", req.Provider, config.Flow)
		WriteError(w, http.StatusBadRequest, "invalid_flow", fmt.Sprintf("Provider %s does not support authorization code flow", req.Provider))
		return
	}

	// Generate state and PKCE codes
	stateBytes := make([]byte, 16)
	if _, err := rand.Read(stateBytes); err != nil {
		s.logger.ErrorLog("[handleAuthStart] Failed to generate state bytes: %v", err)
		WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to generate state")
		return
	}
	state := base64.URLEncoding.EncodeToString(stateBytes)

	codeVerifier, err := generateCodeVerifier()
	if err != nil {
		s.logger.ErrorLog("[handleAuthStart] Failed to generate code verifier: %v", err)
		WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to generate code verifier")
		return
	}
	codeChallenge := generateCodeChallenge(codeVerifier)

	s.logger.InfoLog("[handleAuthStart] PKCE generated - verifier length: %d, challenge length: %d",
		len(codeVerifier), len(codeChallenge))

	// Determine redirect URI
	redirectURI := req.RedirectURI
	if redirectURI == "" {
		redirectURI = fmt.Sprintf("%s/api/callback", s.config.CallbackBaseURL)
	}

	s.logger.InfoLog("[handleAuthStart] Redirect URI: %s", redirectURI)

	// Create OAuth state
	state, err = s.stateManager.CreateState(req.Provider, codeVerifier, redirectURI, s.config.StateTTL)
	if err != nil {
		s.logger.ErrorLog("[handleAuthStart] Failed to create state: %v", err)
		WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to create state")
		return
	}

	s.logger.InfoLog("[AuthStart] Created OAuth state: %s for provider: %s with redirect URI: %s",
		state, req.Provider, redirectURI)

	// Build authorization URL
	authURL := fmt.Sprintf(
		"%s?client_id=%s&redirect_uri=%s&response_type=code&scope=%s&access_type=offline&prompt=consent&state=%s&code_challenge=%s&code_challenge_method=S256",
		config.AuthURL,
		url.QueryEscape(config.ClientID),
		url.QueryEscape(redirectURI),
		url.QueryEscape(strings.Join(config.Scopes, " ")),
		url.QueryEscape(state),
		url.QueryEscape(codeChallenge),
	)

	s.logger.InfoLog("[handleAuthStart] Authorization URL generated (length: %d)", len(authURL))

	response := map[string]interface{}{
		"auth_url":   authURL,
		"state":      state,
		"expires_at": time.Now().Add(s.config.StateTTL).UnixMilli(),
	}

	s.logger.InfoLog("[handleAuthStart] Sending response to client - state: %s", state)
	WriteJSON(w, http.StatusOK, response)
}

// handleCallback handles the OAuth callback
func (s *Server) handleCallback(w http.ResponseWriter, r *http.Request) {
	s.logger.InfoLog("[handleCallback] Received callback request")

	if r.Method != http.MethodGet {
		WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Only GET method is allowed")
		return
	}

	// Extract query parameters
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	errorCode := r.URL.Query().Get("error")
	errorDesc := r.URL.Query().Get("error_description")

	s.logger.InfoLog("[Callback] Received callback with state: %s, code: %s, error: %s",
		state, code, errorCode)

	// Handle error from provider
	if errorCode != "" {
		s.logger.ErrorLog("[Callback] Provider returned error: %s - %s", errorCode, errorDesc)
		s.writeCallbackHTML(w, false, errorCode, errorDesc)
		return
	}

	if code == "" {
		s.logger.ErrorLog("[Callback] No authorization code received")
		s.writeCallbackHTML(w, false, "no_code", "No authorization code received")
		return
	}

	if state == "" {
		s.logger.ErrorLog("[Callback] No state parameter received")
		s.writeCallbackHTML(w, false, "no_state", "No state parameter received")
		return
	}

	s.logger.InfoLog("[Callback] Validating state: %s", state)

	// Validate state
	oauthState, err := s.stateManager.ValidateAndConsume(state)
	if err != nil {
		s.logger.ErrorLog("[Callback] State validation failed: %v", err)
		s.writeCallbackHTML(w, false, "invalid_state", err.Error())
		return
	}

	s.logger.InfoLog("[Callback] State validated for provider: %s", oauthState.Provider)

	// Get provider config
	config, err := s.registry.GetConfig(oauthState.Provider)
	if err != nil {
		s.logger.ErrorLog("[Callback] Failed to get provider config: %v", err)
		s.writeCallbackHTML(w, false, "invalid_provider", err.Error())
		return
	}

	s.logger.InfoLog("[Callback] Exchanging code for tokens for provider: %s", oauthState.Provider)

	// Exchange code for tokens
	creds, err := s.exchangeCodeForTokens(config, code, oauthState.RedirectURI, oauthState.CodeVerifier)
	if err != nil {
		s.logger.ErrorLog("[Callback] Token exchange failed: %v", err)
		s.writeCallbackHTML(w, false, "token_exchange_failed", err.Error())
		return
	}

	s.logger.InfoLog("[Callback] Token exchange successful for provider: %s", oauthState.Provider)

	// Save credentials
	if err := s.saveCredentials(oauthState.Provider, creds); err != nil {
		s.logger.ErrorLog("[Callback] Failed to save credentials: %v", err)
		s.writeCallbackHTML(w, false, "save_failed", err.Error())
		return
	}

	s.logger.InfoLog("[Callback] Credentials saved successfully for provider: %s", oauthState.Provider)
	s.writeCallbackHTML(w, true, "", "")
}

// writeCallbackHTML writes an HTML response for the OAuth callback
func (s *Server) writeCallbackHTML(w http.ResponseWriter, success bool, errorCode, errorDesc string) {
	w.Header().Set("Content-Type", "text/html")

	if success {
		html := `<!DOCTYPE html>
<html>
<head><title>Authorization Successful</title></head>
<body>
  <h1>Authorization Successful!</h1>
  <p>You can close this window and return to your application.</p>
  <script>
    if (window.opener) {
      window.opener.postMessage({type: 'oauth_success'}, '*');
    }
    setTimeout(() => window.close(), 2000);
  </script>
</body>
</html>`
		w.Write([]byte(html))
	} else {
		html := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head><title>Authorization Failed</title></head>
<body>
  <h1>Authorization Failed</h1>
  <p>Error: %s</p>
  <p>%s</p>
  <p>Please try again.</p>
</body>
</html>`, errorCode, errorDesc)
		w.Write([]byte(html))
	}
}

// exchangeCodeForTokens exchanges an authorization code for tokens
func (s *Server) exchangeCodeForTokens(config *ProviderConfig, code, redirectURI, codeVerifier string) (auth.OAuthCreds, error) {
	s.logger.InfoLog("[exchangeCodeForTokens] Exchanging code for tokens - TokenURL: %s", config.TokenURL)

	data := url.Values{}
	data.Set("client_id", config.ClientID)
	data.Set("client_secret", config.ClientSecret)
	data.Set("code", code)
	data.Set("grant_type", "authorization_code")
	data.Set("redirect_uri", redirectURI)
	if codeVerifier != "" {
		data.Set("code_verifier", codeVerifier)
	}

	s.logger.InfoLog("[exchangeCodeForTokens] Request data prepared - client_id: %s, redirect_uri: %s, has_code_verifier: %v",
		config.ClientID, redirectURI, codeVerifier != "")

	req, err := http.NewRequest("POST", config.TokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		s.logger.ErrorLog("[exchangeCodeForTokens] Failed to create token request: %v", err)
		return auth.OAuthCreds{}, fmt.Errorf("failed to create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	s.logger.InfoLog("[exchangeCodeForTokens] Sending token request to %s", config.TokenURL)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		s.logger.ErrorLog("[exchangeCodeForTokens] Failed to send token request: %v", err)
		return auth.OAuthCreds{}, fmt.Errorf("failed to send token request: %w", err)
	}
	defer resp.Body.Close()

	s.logger.InfoLog("[exchangeCodeForTokens] Received response - Status: %d", resp.StatusCode)

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		s.logger.ErrorLog("[exchangeCodeForTokens] Token exchange failed - Status: %d, Body: %s", resp.StatusCode, string(body))
		return auth.OAuthCreds{}, fmt.Errorf("token exchange failed (status %d): %s", resp.StatusCode, string(body))
	}

	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
		TokenType    string `json:"token_type"`
		Scope        string `json:"scope"`
		ResourceURL  string `json:"resource_url,omitempty"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		s.logger.ErrorLog("[exchangeCodeForTokens] Failed to decode token response: %v", err)
		return auth.OAuthCreds{}, fmt.Errorf("failed to decode token response: %w", err)
	}

	s.logger.InfoLog("[exchangeCodeForTokens] Token response decoded - has_access_token: %v, has_refresh_token: %v, expires_in: %d",
		tokenResp.AccessToken != "", tokenResp.RefreshToken != "", tokenResp.ExpiresIn)

	creds := auth.OAuthCreds{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		TokenType:    tokenResp.TokenType,
		ExpiryDate:   time.Now().UnixMilli() + (tokenResp.ExpiresIn * 1000),
		ResourceURL:  tokenResp.ResourceURL,
	}

	return creds, nil
}

// handleToken handles token operations (GET /api/token/{provider}, DELETE /api/token/{provider})
func (s *Server) handleToken(w http.ResponseWriter, r *http.Request) {
	// Extract provider ID from path
	// Path format: /api/token/{provider} or /api/token/{provider}/refresh
	parts := strings.Split(strings.TrimSuffix(r.URL.Path, "/"), "/")
	if len(parts) < 4 {
		WriteError(w, http.StatusNotFound, "not_found", "Endpoint not found")
		return
	}

	providerID := parts[3]

	// Check if this is a refresh request
	if len(parts) >= 5 && parts[4] == "refresh" {
		if r.Method != http.MethodPost {
			WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Only POST method is allowed for refresh")
			return
		}
		s.handleTokenRefresh(w, r, providerID)
		return
	}

	// Otherwise, it's a get or delete token request
	if r.Method == http.MethodGet {
		s.handleGetToken(w, r, providerID)
	} else if r.Method == http.MethodDelete {
		s.handleDeleteToken(w, r, providerID)
	} else {
		WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Only GET and DELETE methods are allowed")
	}
}

// handleGetToken returns the current access token for a provider
func (s *Server) handleGetToken(w http.ResponseWriter, r *http.Request, providerID string) {
	config, err := s.registry.GetConfig(providerID)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_provider", err.Error())
		return
	}

	creds, err := s.loadCredentials(providerID)
	if err != nil {
		WriteError(w, http.StatusNotFound, "credentials_not_found", "No credentials found for provider")
		return
	}

	// Check if token is valid
	if !auth.IsTokenValid(creds) {
		// Try to refresh
		creds, err = s.refreshToken(config, creds)
		if err != nil {
			WriteError(w, http.StatusUnauthorized, "token_expired", "Token is expired and refresh failed")
			return
		}
	}

	response := map[string]interface{}{
		"access_token": creds.AccessToken,
		"token_type":   creds.TokenType,
		"expires_in":   (creds.ExpiryDate - time.Now().UnixMilli()) / 1000,
	}

	WriteJSON(w, http.StatusOK, response)
}

// handleDeleteToken deletes the stored token for a provider
func (s *Server) handleDeleteToken(w http.ResponseWriter, r *http.Request, providerID string) {
	credsPath, err := s.registry.GetCredentialsPath(providerID)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_provider", err.Error())
		return
	}

	if err := os.Remove(credsPath); err != nil && !os.IsNotExist(err) {
		WriteError(w, http.StatusInternalServerError, "delete_failed", err.Error())
		return
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "Token revoked successfully",
	})
}

// handleTokenRefresh refreshes an access token
func (s *Server) handleTokenRefresh(w http.ResponseWriter, r *http.Request, providerID string) {
	config, err := s.registry.GetConfig(providerID)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_provider", err.Error())
		return
	}

	creds, err := s.loadCredentials(providerID)
	if err != nil {
		WriteError(w, http.StatusNotFound, "credentials_not_found", "No credentials found for provider")
		return
	}

	refreshed, err := s.refreshToken(config, creds)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "refresh_failed", err.Error())
		return
	}

	response := map[string]interface{}{
		"access_token":  refreshed.AccessToken,
		"refresh_token": refreshed.RefreshToken,
		"token_type":    refreshed.TokenType,
		"expires_in":    (refreshed.ExpiryDate - time.Now().UnixMilli()) / 1000,
	}

	WriteJSON(w, http.StatusOK, response)
}

// handleCredentials handles credentials listing and clearing
func (s *Server) handleCredentials(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		s.handleListCredentials(w, r)
	} else if r.Method == http.MethodDelete {
		s.handleClearCredentials(w, r)
	} else {
		WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Only GET and DELETE methods are allowed")
	}
}

// handleListCredentials lists all stored credentials
func (s *Server) handleListCredentials(w http.ResponseWriter, r *http.Request) {
	providers := s.registry.ListProviders()
	credentials := make([]map[string]interface{}, 0)

	for _, provider := range providers {
		creds, err := s.loadCredentials(provider.ID)
		if err != nil {
			continue // Skip providers without credentials
		}

		info := map[string]interface{}{
			"provider":   provider.ID,
			"expires_at": creds.ExpiryDate,
			"token_type": creds.TokenType,
		}

		if auth.IsTokenValid(creds) {
			info["valid"] = true
			info["expires_in"] = (creds.ExpiryDate - time.Now().UnixMilli()) / 1000
		} else {
			info["valid"] = false
		}

		credentials = append(credentials, info)
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"credentials": credentials,
	})
}

// handleClearCredentials clears all stored credentials
func (s *Server) handleClearCredentials(w http.ResponseWriter, r *http.Request) {
	providers := s.registry.ListProviders()
	cleared := 0

	for _, provider := range providers {
		credsPath, err := s.registry.GetCredentialsPath(provider.ID)
		if err != nil {
			continue
		}

		if err := os.Remove(credsPath); err == nil {
			cleared++
		}
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Cleared %d credential files", cleared),
		"cleared": cleared,
	})
}

// loadCredentials loads credentials for a provider
func (s *Server) loadCredentials(providerID string) (auth.OAuthCreds, error) {
	credsPath, err := s.registry.GetCredentialsPath(providerID)
	if err != nil {
		return auth.OAuthCreds{}, err
	}

	data, err := os.ReadFile(credsPath)
	if err != nil {
		return auth.OAuthCreds{}, err
	}

	var creds auth.OAuthCreds
	if err := json.Unmarshal(data, &creds); err != nil {
		return auth.OAuthCreds{}, err
	}

	return creds, nil
}

// saveCredentials saves credentials for a provider
func (s *Server) saveCredentials(providerID string, creds auth.OAuthCreds) error {
	credsPath, err := s.registry.GetCredentialsPath(providerID)
	if err != nil {
		return err
	}

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

// refreshToken refreshes an access token using the refresh token
func (s *Server) refreshToken(config *ProviderConfig, creds auth.OAuthCreds) (auth.OAuthCreds, error) {
	if creds.RefreshToken == "" {
		return auth.OAuthCreds{}, fmt.Errorf("no refresh token available")
	}

	oauthConfig := &oauth2.Config{
		ClientID:     config.ClientID,
		ClientSecret: config.ClientSecret,
		Endpoint: oauth2.Endpoint{
			TokenURL: config.TokenURL,
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	token := &oauth2.Token{
		RefreshToken: creds.RefreshToken,
	}

	tokenSource := oauthConfig.TokenSource(ctx, token)
	newToken, err := tokenSource.Token()
	if err != nil {
		return auth.OAuthCreds{}, fmt.Errorf("failed to refresh token: %w", err)
	}

	updated := auth.OAuthCreds{
		AccessToken:  newToken.AccessToken,
		TokenType:    newToken.TokenType,
		RefreshToken: newToken.RefreshToken,
		ExpiryDate:   newToken.Expiry.UnixMilli(),
	}

	if resourceURL, ok := newToken.Extra("resource_url").(string); ok {
		updated.ResourceURL = resourceURL
	}

	// Save updated credentials
	if err := s.saveCredentials(config.ID, updated); err != nil {
		return auth.OAuthCreds{}, fmt.Errorf("failed to save refreshed credentials: %w", err)
	}

	return updated, nil
}

// generateCodeVerifier generates a random code verifier for PKCE
func generateCodeVerifier() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// generateCodeChallenge generates a code challenge from a code verifier using SHA-256
func generateCodeChallenge(codeVerifier string) string {
	h := sha256.New()
	h.Write([]byte(codeVerifier))
	return base64.RawURLEncoding.EncodeToString(h.Sum(nil))
}

// GetProviderConfig returns the configuration for a provider (for use by other packages)
func (s *Server) GetProviderConfig(providerID string) (*ProviderConfig, error) {
	return s.registry.GetConfig(providerID)
}

// GetRegistry returns the provider registry (for use by other packages)
func (s *Server) GetRegistry() *ProviderRegistry {
	return s.registry
}

// GetStateManager returns the state manager (for use by other packages)
func (s *Server) GetStateManager() *StateManager {
	return s.stateManager
}

// resolveDashboardDir resolves the dashboard directory path relative to the executable location
// This ensures the dashboard can be served regardless of the current working directory
func (s *Server) resolveDashboardDir() (string, error) {
	// Get the executable path
	execPath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("failed to get executable path: %w", err)
	}

	// Get the directory containing the executable
	execDir := filepath.Dir(execPath)

	// Resolve the dashboard directory relative to the executable
	dashboardDir := filepath.Join(execDir, "web", "dashboard")

	// Check if the dashboard directory exists
	if _, err := os.Stat(dashboardDir); os.IsNotExist(err) {
		// If not found relative to executable, try relative to current working directory
		// This handles development scenarios where the binary is run from the project root
		wd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("failed to get working directory: %w", err)
		}
		dashboardDir = filepath.Join(wd, "web", "dashboard")

		// Check again
		if _, err := os.Stat(dashboardDir); os.IsNotExist(err) {
			return "", fmt.Errorf("dashboard directory not found (tried: %s and %s)",
				filepath.Join(execDir, "web", "dashboard"),
				filepath.Join(wd, "web", "dashboard"))
		}
	}

	return dashboardDir, nil
}
