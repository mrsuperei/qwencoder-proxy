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

// ProviderTokenInfo represents token information for API responses
type ProviderTokenInfo struct {
	ID          string  `json:"id"`
	Email       string  `json:"email"`
	ExpiryDate  int64   `json:"expiry_date"`
	ExpiresIn   int64   `json:"expires_in"`
	TokenType   string  `json:"token_type"`
	Healthy     bool    `json:"healthy"`
	HealthScore float64 `json:"health_score"`
	LastUsed    int64   `json:"last_used"`
	CreatedAt   int64   `json:"created_at"`
	ErrorCount  int     `json:"error_count"`
}

// ProviderCredentialsInfo represents credentials info for a provider
type ProviderCredentialsInfo struct {
	ProviderID string              `json:"provider_id"`
	Tokens     []ProviderTokenInfo `json:"tokens"`
	Settings   auth.StoreSettings  `json:"settings"`
}

// TokenSelectionResponse represents the response when selecting a token
type TokenSelectionResponse struct {
	AccessToken string `json:"access_token"`
	Email       string `json:"email"`
	TokenID     string `json:"token_id"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int64  `json:"expires_in"`
}

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
	config            *Config
	registry          *ProviderRegistry
	stateManager      *StateManager
	logger            *logging.Logger
	httpClient        *http.Client
	tokenStores       map[string]*auth.MultiTokenStore // providerID -> MultiTokenStore
	tokenManagers     map[string]*auth.TokenManager    // providerID -> TokenManager
	multiTokenManager *auth.MultiTokenManager          // Multi-token manager for all providers
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

	// Create multi-token manager
	multiTokenManager := auth.NewMultiTokenManager(logger)
	if err := multiTokenManager.Initialize(); err != nil {
		logger.ErrorLog("Failed to initialize multi-token manager: %v", err)
	}
	if err := multiTokenManager.Start(); err != nil {
		logger.ErrorLog("Failed to start multi-token manager: %v", err)
	}

	return &Server{
		config:            config,
		registry:          NewProviderRegistry(),
		stateManager:      NewStateManager(),
		logger:            logger,
		httpClient:        &http.Client{Timeout: 30 * time.Second},
		tokenStores:       make(map[string]*auth.MultiTokenStore),
		tokenManagers:     make(map[string]*auth.TokenManager),
		multiTokenManager: multiTokenManager,
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

// Stop stops the server and cleans up resources
func (s *Server) Stop() {
	s.logger.InfoLog("Stopping OAuth REST API server...")

	// Stop multi-token manager
	if s.multiTokenManager != nil {
		s.multiTokenManager.Stop()
		s.logger.InfoLog("Multi-token manager stopped")
	}
}

// RegisterRoutes registers OAuth API routes with an external http.ServeMux
// This allows to OAuth server to be integrated into a larger server
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

			// Set proper Content-Type header for HTML
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			http.ServeFile(w, r, filePath)
			return
		}
		// For other paths, serve from the dashboard directory
		filePath := filepath.Join(dashboardDir, r.URL.Path)
		s.logger.InfoLog("Attempting to serve file: %s", filePath)
		http.ServeFile(w, r, filePath)
	})

	// Serve CSS files from /css/ path with proper MIME type and cache headers
	// Uses http.FileServer with http.StripPrefix for efficient static file serving
	cssDir := filepath.Join(dashboardDir, "css")
	mux.Handle("/css/", http.StripPrefix("/css/", s.createStaticFileHandler(cssDir, "text/css; charset=utf-8", 3600)))

	// Serve JavaScript files from /js/ path with proper MIME type and cache headers
	// Uses http.FileServer with http.StripPrefix for efficient static file serving
	jsDir := filepath.Join(dashboardDir, "js")
	mux.Handle("/js/", http.StripPrefix("/js/", s.createStaticFileHandler(jsDir, "application/javascript; charset=utf-8", 3600)))

	// Serve template files from /templates/ path with proper MIME type and cache headers
	// Uses http.FileServer with http.StripPrefix for efficient static file serving
	templatesDir := filepath.Join(dashboardDir, "templates")
	mux.Handle("/templates/", http.StripPrefix("/templates/", s.createStaticFileHandler(templatesDir, "text/html; charset=utf-8", 1800)))

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
	mux.HandleFunc("/api/credentials/", s.handleProviderCredentials)

	// Proxy connection test
	mux.HandleFunc("/api/proxy/test", s.handleProxyTest)
}

// createStaticFileHandler creates a handler for serving static files with proper MIME types and cache headers
// This wrapper ensures proper Content-Type headers and cache-control for static assets
func (s *Server) createStaticFileHandler(dir, contentType string, maxAge int) http.Handler {
	// Create a file server for the directory
	fileServer := http.FileServer(http.Dir(dir))

	// Wrap the file server to add custom headers
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Set the Content-Type header
		w.Header().Set("Content-Type", contentType)

		// Set cache-control headers for static assets
		w.Header().Set("Cache-Control", fmt.Sprintf("public, max-age=%d", maxAge))

		// Log the request for debugging
		s.logger.InfoLog("Serving static asset: %s (type: %s)", r.URL.Path, contentType)

		// Serve the file
		fileServer.ServeHTTP(w, r)
	})
}

// getTokenStore gets or creates a token store for a provider
func (s *Server) getTokenStore(providerID string) (*auth.MultiTokenStore, error) {
	if store, ok := s.tokenStores[providerID]; ok {
		return store, nil
	}

	credsPath, err := s.registry.GetCredentialsPath(providerID)
	if err != nil {
		return nil, err
	}

	store := auth.NewMultiTokenStore(providerID, credsPath, s.logger)
	if err := store.Load(); err != nil {
		s.logger.WarningLog("Failed to load token store for %s: %v", providerID, err)
	}

	s.tokenStores[providerID] = store
	return store, nil
}

// getTokenManager gets or creates a token manager for a provider
func (s *Server) getTokenManager(providerID string) (*auth.TokenManager, error) {
	if manager, ok := s.tokenManagers[providerID]; ok {
		return manager, nil
	}

	store, err := s.getTokenStore(providerID)
	if err != nil {
		return nil, err
	}

	// Create strategy factory and get default strategy
	factory := auth.NewStrategyFactory()
	strategy, err := factory.CreateStrategy(auth.DefaultSelectionStrategy)
	if err != nil {
		return nil, err
	}

	// Get proxy health tracker for this provider
	proxyHealthTracker, _ := s.multiTokenManager.GetProxyHealthTracker(providerID)

	manager := auth.NewTokenManager(store, strategy, s.logger, nil, proxyHealthTracker)
	s.tokenManagers[providerID] = manager
	return manager, nil
}

// providerTokenToInfo converts ProviderToken to ProviderTokenInfo
func (s *Server) providerTokenToInfo(token auth.ProviderToken) ProviderTokenInfo {
	expiresIn := (token.ExpiryDate - time.Now().UnixMilli()) / 1000
	if expiresIn < 0 {
		expiresIn = 0
	}

	return ProviderTokenInfo{
		ID:          token.ID,
		Email:       token.Email,
		ExpiryDate:  token.ExpiryDate,
		ExpiresIn:   expiresIn,
		TokenType:   token.TokenType,
		Healthy:     token.Healthy,
		HealthScore: token.HealthScore,
		LastUsed:    token.LastUsed,
		CreatedAt:   token.CreatedAt,
		ErrorCount:  token.ErrorCount,
	}
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

			// Success - save credentials with email extraction
			// Parse token response for email extraction
			tokenResponse := make(map[string]interface{})
			tokenResponse["access_token"] = token.AccessToken
			tokenResponse["token_type"] = token.TokenType
			tokenResponse["refresh_token"] = token.RefreshToken

			if resourceURL, ok := token.Extra("resource_url").(string); ok {
				tokenResponse["resource_url"] = resourceURL
			}

			creds := auth.OAuthCreds{
				AccessToken:  token.AccessToken,
				TokenType:    token.TokenType,
				RefreshToken: token.RefreshToken,
				ExpiryDate:   token.Expiry.UnixMilli(),
			}

			if err := s.saveCredentials(providerID, creds, tokenResponse); err != nil {
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

	// Check if this code has already been processed (idempotency)
	if s.stateManager.IsCodeProcessed(code) {
		s.logger.WarningLog("[Callback] Duplicate callback detected - code already processed: %s", code)
		// Return success to avoid confusing the user (the original processing was successful)
		s.writeCallbackHTML(w, true, "", "")
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

	// Mark this code as processed BEFORE exchanging tokens to prevent duplicate processing
	s.stateManager.MarkCodeProcessed(code)
	s.logger.InfoLog("[Callback] Marked code as processed: %s", code)

	// Get provider config
	config, err := s.registry.GetConfig(oauthState.Provider)
	if err != nil {
		s.logger.ErrorLog("[Callback] Failed to get provider config: %v", err)
		s.writeCallbackHTML(w, false, "invalid_provider", err.Error())
		return
	}

	s.logger.InfoLog("[Callback] Exchanging code for tokens for provider: %s", oauthState.Provider)

	// Exchange code for tokens with email extraction
	creds, tokenResponseMap, err := s.exchangeCodeForTokensWithResponse(config, code, oauthState.RedirectURI, oauthState.CodeVerifier)
	if err != nil {
		s.logger.ErrorLog("[Callback] Token exchange failed: %v", err)
		s.writeCallbackHTML(w, false, "token_exchange_failed", err.Error())
		return
	}

	s.logger.InfoLog("[Callback] Token exchange successful for provider: %s", oauthState.Provider)

	// Save credentials with email extraction
	if err := s.saveCredentials(oauthState.Provider, creds, tokenResponseMap); err != nil {
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

// exchangeCodeForTokensWithResponse exchanges an authorization code for tokens and returns both creds and response map
func (s *Server) exchangeCodeForTokensWithResponse(config *ProviderConfig, code, redirectURI, codeVerifier string) (auth.OAuthCreds, map[string]interface{}, error) {
	s.logger.InfoLog("[exchangeCodeForTokensWithResponse] Exchanging code for tokens - TokenURL: %s", config.TokenURL)

	data := url.Values{}
	data.Set("client_id", config.ClientID)
	data.Set("client_secret", config.ClientSecret)
	data.Set("code", code)
	data.Set("grant_type", "authorization_code")
	data.Set("redirect_uri", redirectURI)
	if codeVerifier != "" {
		data.Set("code_verifier", codeVerifier)
	}

	s.logger.InfoLog("[exchangeCodeForTokensWithResponse] Request data prepared - client_id: %s, redirect_uri: %s, has_code_verifier: %v",
		config.ClientID, redirectURI, codeVerifier != "")

	req, err := http.NewRequest("POST", config.TokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		s.logger.ErrorLog("[exchangeCodeForTokensWithResponse] Failed to create token request: %v", err)
		return auth.OAuthCreds{}, nil, fmt.Errorf("failed to create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	s.logger.InfoLog("[exchangeCodeForTokensWithResponse] Sending token request to %s", config.TokenURL)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		s.logger.ErrorLog("[exchangeCodeForTokensWithResponse] Failed to send token request: %v", err)
		return auth.OAuthCreds{}, nil, fmt.Errorf("failed to send token request: %w", err)
	}
	defer resp.Body.Close()

	s.logger.InfoLog("[exchangeCodeForTokensWithResponse] Received response - Status: %d", resp.StatusCode)

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		s.logger.ErrorLog("[exchangeCodeForTokensWithResponse] Token exchange failed - Status: %d, Body: %s", resp.StatusCode, string(body))
		return auth.OAuthCreds{}, nil, fmt.Errorf("token exchange failed (status %d): %s", resp.StatusCode, string(body))
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
		s.logger.ErrorLog("[exchangeCodeForTokensWithResponse] Failed to decode token response: %v", err)
		return auth.OAuthCreds{}, nil, fmt.Errorf("failed to decode token response: %w", err)
	}

	s.logger.InfoLog("[exchangeCodeForTokensWithResponse] Token response decoded - has_access_token: %v, has_refresh_token: %v, expires_in: %d",
		tokenResp.AccessToken != "", tokenResp.RefreshToken != "", tokenResp.ExpiresIn)

	// Build token response map for email extraction
	tokenResponse := make(map[string]interface{})
	tokenResponse["access_token"] = tokenResp.AccessToken
	tokenResponse["token_type"] = tokenResp.TokenType
	tokenResponse["refresh_token"] = tokenResp.RefreshToken

	if tokenResp.Scope != "" {
		tokenResponse["scope"] = tokenResp.Scope
	}

	creds := auth.OAuthCreds{
		AccessToken:  tokenResp.AccessToken,
		TokenType:    tokenResp.TokenType,
		RefreshToken: tokenResp.RefreshToken,
		ExpiryDate:   time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second).UnixMilli(), // Calculate absolute expiry time
		ResourceURL:  tokenResp.ResourceURL,
	}

	return creds, tokenResponse, nil
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
	manager, err := s.multiTokenManager.GetTokenManager(providerID)
	if err != nil {
		WriteError(w, http.StatusNotFound, "not_found", err.Error())
		return
	}

	// Select a token using the configured strategy
	token, err := manager.SelectToken()
	if err != nil {
		WriteError(w, http.StatusServiceUnavailable, "no_token", err.Error())
		return
	}

	// Update LastUsed timestamp
	store, err := s.multiTokenManager.GetTokenStore(providerID)
	if err != nil {
		s.logger.WarningLog("Failed to get token store: %v", err)
	} else {
		if err := store.UpdateToken(token.ID, func(t *auth.ProviderToken) {
			t.LastUsed = auth.GetCurrentTimestamp()
		}); err != nil {
			s.logger.WarningLog("Failed to update LastUsed timestamp: %v", err)
		}
	}

	response := TokenSelectionResponse{
		AccessToken: token.AccessToken,
		Email:       token.Email,
		TokenID:     token.ID,
		TokenType:   token.TokenType,
		ExpiresIn:   (token.ExpiryDate - time.Now().UnixMilli()) / 1000,
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
		store, err := s.multiTokenManager.GetTokenStore(provider.ID)
		if err != nil {
			continue // Skip providers without token store
		}

		tokens := store.ListTokens()
		tokenInfos := make([]ProviderTokenInfo, 0, len(tokens))
		for _, token := range tokens {
			tokenInfos = append(tokenInfos, s.providerTokenToInfo(token))
		}

		info := map[string]interface{}{
			"provider":     provider.ID,
			"total_tokens": len(tokens),
			"valid_tokens": store.GetValidTokenCount(),
			"settings":     store.Settings,
			"tokens":       tokenInfos,
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

// saveCredentials saves credentials for a provider using multi-token store
func (s *Server) saveCredentials(providerID string, creds auth.OAuthCreds, tokenResponse map[string]interface{}) error {
	// Extract email from token response
	email := ""
	if tokenResponse != nil {
		var err error
		email, err = s.multiTokenManager.ExtractEmail(context.Background(), providerID, tokenResponse, creds.AccessToken)
		if err != nil {
			s.logger.WarningLog("Failed to extract email for %s: %v", providerID, err)
			email = ""
		}
	}

	// Create provider token with email
	providerToken := auth.ProviderToken{
		ID:           auth.GenerateTokenID(),
		AccessToken:  creds.AccessToken,
		RefreshToken: creds.RefreshToken,
		TokenType:    creds.TokenType,
		ExpiryDate:   creds.ExpiryDate,
		Email:        email,
		ResourceURL:  creds.ResourceURL,
		Scope:        "", // Will be populated from token response if available
		Healthy:      true,
		HealthScore:  1.0,
		LastUsed:     auth.GetCurrentTimestamp(),
		CreatedAt:    auth.GetCurrentTimestamp(),
		ErrorCount:   0,
	}

	// Save token to multi-token store
	if err := s.multiTokenManager.SaveToken(providerID, providerToken); err != nil {
		return fmt.Errorf("failed to save token to multi-token store: %w", err)
	}

	s.logger.InfoLog("Saved token for provider %s with email %s", providerID, email)
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

	// Save updated credentials with empty token response map
	tokenResponse := make(map[string]interface{})
	if err := s.saveCredentials(config.ID, updated, tokenResponse); err != nil {
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

// handleProviderCredentials handles provider-specific credential operations
func (s *Server) handleProviderCredentials(w http.ResponseWriter, r *http.Request) {
	s.logger.InfoLog("[handleProviderCredentials] Path: %s, Method: %s", r.URL.Path, r.Method)
	// Extract provider ID and action from path
	// Path format: /api/credentials/{provider} or /api/credentials/{provider}/{action}
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	s.logger.InfoLog("[handleProviderCredentials] Parts: %v, Length: %d", parts, len(parts))
	if len(parts) < 4 {
		WriteError(w, http.StatusNotFound, "not_found", "Endpoint not found")
		return
	}

	providerID := parts[2]
	s.logger.InfoLog("[handleProviderCredentials] providerID: %s (from parts[2])", providerID)

	switch r.Method {
	case http.MethodGet:
		if len(parts) == 4 {
			// GET /api/credentials/{provider} - List all tokens for a provider
			s.handleGetProviderCredentials(w, r, providerID)
		} else if len(parts) == 5 && parts[4] == "proxy" {
			// GET /api/credentials/{provider}/{tokenID}/proxy - Get proxy config for a token
			tokenID := parts[3]
			s.getProxyConfigHandler(w, r, providerID, tokenID)
		} else {
			WriteError(w, http.StatusNotFound, "not_found", "Endpoint not found")
		}
	case http.MethodPost:
		if len(parts) == 4 {
			// POST /api/credentials/{provider} - Add a new token
			s.handleAddToken(w, r, providerID)
		} else if len(parts) == 5 && parts[4] == "refresh" {
			// POST /api/credentials/{provider}/{tokenID}/refresh - Refresh a specific token
			tokenID := parts[3]
			s.handleRefreshTokenByID(w, r, providerID, tokenID)
		} else {
			WriteError(w, http.StatusNotFound, "not_found", "Endpoint not found")
		}
	case http.MethodDelete:
		if len(parts) == 5 {
			// DELETE /api/credentials/{provider}/{tokenID} - Delete a specific token
			tokenID := parts[3]
			s.handleDeleteTokenByID(w, r, providerID, tokenID)
		} else if len(parts) == 5 && parts[4] == "proxy" {
			// DELETE /api/credentials/{provider}/{tokenID}/proxy - Remove proxy config for a token
			tokenID := parts[3]
			s.deleteProxyConfigHandler(w, r, providerID, tokenID)
		} else {
			WriteError(w, http.StatusNotFound, "not_found", "Endpoint not found")
		}
	case http.MethodPut:
		if len(parts) == 5 && parts[4] == "settings" {
			// PUT /api/credentials/{provider}/settings - Update provider settings
			s.handleUpdateProviderSettings(w, r, providerID)
		} else if len(parts) == 5 && parts[4] == "proxy" {
			// PUT /api/credentials/{provider}/{tokenID}/proxy - Update proxy config for a token
			tokenID := parts[3]
			s.updateProxyConfigHandler(w, r, providerID, tokenID)
		} else {
			WriteError(w, http.StatusNotFound, "not_found", "Endpoint not found")
		}
	default:
		WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed")
	}
}

// handleGetProviderCredentials returns all tokens for a provider
func (s *Server) handleGetProviderCredentials(w http.ResponseWriter, r *http.Request, providerID string) {
	store, err := s.multiTokenManager.GetTokenStore(providerID)
	if err != nil {
		WriteError(w, http.StatusNotFound, "not_found", err.Error())
		return
	}

	tokens := store.ListTokens()
	tokenInfos := make([]ProviderTokenInfo, 0, len(tokens))
	for _, token := range tokens {
		tokenInfos = append(tokenInfos, s.providerTokenToInfo(token))
	}

	WriteJSON(w, http.StatusOK, ProviderCredentialsInfo{
		ProviderID: providerID,
		Tokens:     tokenInfos,
		Settings:   store.Settings,
	})
}

// handleAddToken adds a new token to a provider
func (s *Server) handleAddToken(w http.ResponseWriter, r *http.Request, providerID string) {
	var req struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token,omitempty"`
		TokenType    string `json:"token_type,omitempty"`
		ExpiresIn    int64  `json:"expires_in,omitempty"`
		ResourceURL  string `json:"resource_url,omitempty"`
		Email        string `json:"email,omitempty"`
	}

	if err := ParseJSON(r, &req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	if req.AccessToken == "" {
		WriteError(w, http.StatusBadRequest, "invalid_request", "access_token is required")
		return
	}

	store, err := s.multiTokenManager.GetTokenStore(providerID)
	if err != nil {
		WriteError(w, http.StatusNotFound, "not_found", err.Error())
		return
	}

	// Extract email if not provided
	email := req.Email
	if email == "" {
		// Try to extract email from token
		extractor := auth.NewEmailExtractionManager(s.logger)
		emailExtractor, err := extractor.GetExtractor(providerID)
		if err != nil {
			s.logger.WarningLog("Failed to get email extractor for %s: %v", providerID, err)
			email = "unknown@example.com"
		} else {
			if extractedEmail, err := emailExtractor.ExtractEmail(context.Background(), nil, req.AccessToken); err == nil {
				email = extractedEmail
			} else {
				s.logger.WarningLog("Failed to extract email for %s: %v", providerID, err)
				email = "unknown@example.com"
			}
		}
	}

	// Calculate expiry date
	expiryDate := int64(0)
	if req.ExpiresIn > 0 {
		expiryDate = time.Now().UnixMilli() + (req.ExpiresIn * 1000)
	}

	token := auth.ProviderToken{
		ID:           auth.GenerateTokenID(),
		AccessToken:  req.AccessToken,
		RefreshToken: req.RefreshToken,
		TokenType:    req.TokenType,
		ExpiryDate:   expiryDate,
		ResourceURL:  req.ResourceURL,
		Email:        email,
		Healthy:      true,
		HealthScore:  1.0,
		LastUsed:     auth.GetCurrentTimestamp(),
		CreatedAt:    auth.GetCurrentTimestamp(),
		ErrorCount:   0,
	}

	if err := store.AddToken(token); err != nil {
		WriteError(w, http.StatusInternalServerError, "add_failed", err.Error())
		return
	}

	WriteJSON(w, http.StatusCreated, map[string]interface{}{
		"token_id": token.ID,
		"email":    token.Email,
		"success":  true,
	})
}

// handleDeleteTokenByID deletes a specific token
func (s *Server) handleDeleteTokenByID(w http.ResponseWriter, r *http.Request, providerID, tokenID string) {
	store, err := s.multiTokenManager.GetTokenStore(providerID)
	if err != nil {
		WriteError(w, http.StatusNotFound, "not_found", err.Error())
		return
	}

	if err := store.RemoveToken(tokenID); err != nil {
		WriteError(w, http.StatusNotFound, "token_not_found", err.Error())
		return
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "Token deleted successfully",
	})
}

// handleRefreshTokenByID refreshes a specific token
func (s *Server) handleRefreshTokenByID(w http.ResponseWriter, r *http.Request, providerID, tokenID string) {
	store, err := s.multiTokenManager.GetTokenStore(providerID)
	if err != nil {
		WriteError(w, http.StatusNotFound, "not_found", err.Error())
		return
	}

	token, err := store.GetToken(tokenID)
	if err != nil {
		WriteError(w, http.StatusNotFound, "token_not_found", err.Error())
		return
	}

	// Get provider config for refresh
	config, err := s.registry.GetConfig(providerID)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_provider", err.Error())
		return
	}

	// Refresh token using provider-specific logic
	refreshed, err := s.refreshProviderToken(config, token)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "refresh_failed", err.Error())
		return
	}

	// Update token in store
	refreshed.ID = tokenID
	refreshed.Email = token.Email
	refreshed.CreatedAt = token.CreatedAt
	refreshed.LastUsed = auth.GetCurrentTimestamp()
	refreshed.Healthy = true
	refreshed.HealthScore = 1.0
	refreshed.ErrorCount = 0

	if err := store.AddToken(refreshed); err != nil {
		WriteError(w, http.StatusInternalServerError, "save_failed", err.Error())
		return
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"token":   s.providerTokenToInfo(refreshed),
	})
}

// handleUpdateProviderSettings updates provider settings
func (s *Server) handleUpdateProviderSettings(w http.ResponseWriter, r *http.Request, providerID string) {
	var req struct {
		SelectionStrategy string `json:"selection_strategy"`
		RefreshBufferSec  int    `json:"refresh_buffer_sec"`
		MaxErrorCount     int    `json:"max_error_count"`
	}

	if err := ParseJSON(r, &req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	store, err := s.multiTokenManager.GetTokenStore(providerID)
	if err != nil {
		WriteError(w, http.StatusNotFound, "not_found", err.Error())
		return
	}

	// Update settings with validation
	settings := store.Settings
	if req.SelectionStrategy != "" {
		// Validate strategy
		factory := auth.NewStrategyFactory()
		if _, err := factory.CreateStrategy(req.SelectionStrategy); err != nil {
			WriteError(w, http.StatusBadRequest, "invalid_strategy", err.Error())
			return
		}
		settings.SelectionStrategy = req.SelectionStrategy
	}
	if req.RefreshBufferSec > 0 {
		settings.RefreshBufferSec = req.RefreshBufferSec
	}
	if req.MaxErrorCount > 0 {
		settings.MaxErrorCount = req.MaxErrorCount
	}

	store.Settings = settings
	if err := store.Save(); err != nil {
		WriteError(w, http.StatusInternalServerError, "save_failed", err.Error())
		return
	}

	// Update token manager strategy if it exists
	if manager, ok := s.tokenManagers[providerID]; ok {
		factory := auth.NewStrategyFactory()
		if strategy, err := factory.CreateStrategy(settings.SelectionStrategy); err == nil {
			manager.SetStrategy(strategy)
		}
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"success":  true,
		"settings": settings,
	})
}

// ProxyConfigResponse represents the response for proxy configuration
type ProxyConfigResponse struct {
	TokenID      string            `json:"token_id"`
	Proxy        *auth.ProxyConfig `json:"proxy"`
	HealthStatus *auth.ProxyHealth `json:"health_status,omitempty"`
}

// TestProxyRequest represents a request to test a proxy connection
type TestProxyRequest struct {
	Type     string `json:"type"`               // Type of proxy (none, http, https, socks5)
	Host     string `json:"host"`               // Proxy server hostname or IP address
	Port     int    `json:"port"`               // Proxy server port number
	Username string `json:"username,omitempty"` // Optional username for authentication
	Password string `json:"password,omitempty"` // Optional password for authentication
}

// TestProxyResponse represents the response from a proxy test
type TestProxyResponse struct {
	Success   bool   `json:"success"`           // Whether the connection test succeeded
	LatencyMs int    `json:"latency_ms"`        // Connection latency in milliseconds
	Error     string `json:"error,omitempty"`   // Error message if test failed
	Message   string `json:"message,omitempty"` // Human-readable message
}

// getProxyConfigHandler handles GET /api/credentials/{provider}/{tokenID}/proxy
// Returns proxy configuration with masked password and health status
func (s *Server) getProxyConfigHandler(w http.ResponseWriter, r *http.Request, providerID, tokenID string) {
	s.logger.InfoLog("[getProxyConfig] GET request - provider: %s, tokenID: %s", providerID, tokenID)

	// Validate provider type
	if err := s.validateProvider(providerID); err != nil {
		s.logger.ErrorLog("[getProxyConfig] Invalid provider: %s - %v", providerID, err)
		WriteError(w, http.StatusBadRequest, "invalid_provider", err.Error())
		return
	}

	// Get token store
	store, err := s.multiTokenManager.GetTokenStore(providerID)
	if err != nil {
		s.logger.ErrorLog("[getProxyConfig] Failed to get token store for %s: %v", providerID, err)
		WriteError(w, http.StatusBadRequest, "invalid_provider", err.Error())
		return
	}

	// Get token
	token, err := store.GetToken(tokenID)
	if err != nil {
		s.logger.ErrorLog("[getProxyConfig] Token not found - provider: %s, tokenID: %s", providerID, tokenID)
		WriteError(w, http.StatusNotFound, "not_found", "Token not found")
		return
	}

	// Get proxy health tracker
	proxyHealthTracker, err := s.multiTokenManager.GetProxyHealthTracker(providerID)
	if err != nil {
		s.logger.WarningLog("[getProxyConfig] Failed to get proxy health tracker for %s: %v", providerID, err)
	}

	// Get health status
	var healthStatus *auth.ProxyHealth
	if proxyHealthTracker != nil {
		healthStatus = proxyHealthTracker.GetHealthStatus(tokenID)
	}

	// Create response with masked password
	response := ProxyConfigResponse{
		TokenID:      tokenID,
		Proxy:        token.Proxy,
		HealthStatus: healthStatus,
	}

	// Mask password in response
	if response.Proxy != nil && response.Proxy.Password != "" {
		response.Proxy.Password = "***"
	}

	s.logger.InfoLog("[getProxyConfig] Successfully retrieved proxy config for token %s", tokenID)
	WriteJSON(w, http.StatusOK, response)
}

// updateProxyConfigHandler handles PUT /api/credentials/{provider}/{tokenID}/proxy
// Updates proxy configuration for a token
func (s *Server) updateProxyConfigHandler(w http.ResponseWriter, r *http.Request, providerID, tokenID string) {
	s.logger.InfoLog("[updateProxyConfig] PUT request - provider: %s, tokenID: %s", providerID, tokenID)
	s.logger.InfoLog("[updateProxyConfig] DEBUG - providerID length: %d, tokenID length: %d", len(providerID), len(tokenID))

	// Validate provider type
	if err := s.validateProvider(providerID); err != nil {
		s.logger.ErrorLog("[updateProxyConfig] Invalid provider: %s - %v", providerID, err)
		WriteError(w, http.StatusBadRequest, "invalid_provider", err.Error())
		return
	}

	// Parse request body
	var proxyConfig auth.ProxyConfig
	if err := ParseJSON(r, &proxyConfig); err != nil {
		s.logger.ErrorLog("[updateProxyConfig] Failed to parse request: %v", err)
		WriteError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	// Set default enabled value if not specified
	if !proxyConfig.Enabled && proxyConfig.Type == "" {
		proxyConfig.Enabled = true
	}

	// Validate proxy configuration
	if err := proxyConfig.Validate(); err != nil {
		s.logger.ErrorLog("[updateProxyConfig] Invalid proxy config: %v", err)
		WriteErrorWithDetails(w, http.StatusBadRequest, "validation_error", "Invalid proxy configuration", map[string]interface{}{
			"details": err.Error(),
		})
		return
	}

	// Get token store
	store, err := s.multiTokenManager.GetTokenStore(providerID)
	if err != nil {
		s.logger.ErrorLog("[updateProxyConfig] Failed to get token store for %s: %v", providerID, err)
		WriteError(w, http.StatusBadRequest, "invalid_provider", err.Error())
		return
	}

	// Check if token exists
	_, err = store.GetToken(tokenID)
	if err != nil {
		s.logger.ErrorLog("[updateProxyConfig] Token not found - provider: %s, tokenID: %s", providerID, tokenID)
		WriteError(w, http.StatusNotFound, "not_found", "Token not found")
		return
	}

	// Log the change for audit trail
	s.logger.InfoLog("[updateProxyConfig] Updating proxy config for token %s - Type: %s, Host: %s, Port: %d, Enabled: %t",
		tokenID, proxyConfig.Type, proxyConfig.Host, proxyConfig.Port, proxyConfig.Enabled)

	// Update token with new proxy config
	if err := store.UpdateToken(tokenID, func(t *auth.ProviderToken) {
		t.Proxy = &proxyConfig
		t.ProxyHealthScore = 1.0 // Reset proxy health score on config change
	}); err != nil {
		s.logger.ErrorLog("[updateProxyConfig] Failed to update token: %v", err)
		WriteError(w, http.StatusInternalServerError, "update_failed", err.Error())
		return
	}

	// Log successful update
	s.logger.InfoLog("[updateProxyConfig] Successfully updated proxy config for token %s", tokenID)

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "Proxy configuration updated",
	})
}

// deleteProxyConfigHandler handles DELETE /api/credentials/{provider}/{tokenID}/proxy
// Removes proxy configuration from a token (sets to nil)
func (s *Server) deleteProxyConfigHandler(w http.ResponseWriter, r *http.Request, providerID, tokenID string) {
	s.logger.InfoLog("[deleteProxyConfig] DELETE request - provider: %s, tokenID: %s", providerID, tokenID)

	// Validate provider type
	if err := s.validateProvider(providerID); err != nil {
		s.logger.ErrorLog("[deleteProxyConfig] Invalid provider: %s - %v", providerID, err)
		WriteError(w, http.StatusBadRequest, "invalid_provider", err.Error())
		return
	}

	// Get token store
	store, err := s.multiTokenManager.GetTokenStore(providerID)
	if err != nil {
		s.logger.ErrorLog("[deleteProxyConfig] Failed to get token store for %s: %v", providerID, err)
		WriteError(w, http.StatusBadRequest, "invalid_provider", err.Error())
		return
	}

	// Check if token exists
	token, err := store.GetToken(tokenID)
	if err != nil {
		s.logger.ErrorLog("[deleteProxyConfig] Token not found - provider: %s, tokenID: %s", providerID, tokenID)
		WriteError(w, http.StatusNotFound, "not_found", "Token not found")
		return
	}

	// Check if proxy was configured
	hadProxy := token.Proxy != nil

	// Log the deletion for audit trail
	s.logger.InfoLog("[deleteProxyConfig] Removing proxy config for token %s (had proxy: %v)", tokenID, hadProxy)

	// Update token to remove proxy config
	if err := store.UpdateToken(tokenID, func(t *auth.ProviderToken) {
		t.Proxy = nil
		t.ProxyHealthScore = 1.0 // Reset proxy health score
	}); err != nil {
		s.logger.ErrorLog("[deleteProxyConfig] Failed to update token: %v", err)
		WriteError(w, http.StatusInternalServerError, "update_failed", err.Error())
		return
	}

	// Log successful deletion
	s.logger.InfoLog("[deleteProxyConfig] Successfully removed proxy config for token %s", tokenID)

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "Proxy configuration removed",
	})
}

// validateProvider validates that the provider ID is valid
func (s *Server) validateProvider(providerID string) error {
	// Get provider config to validate
	_, err := s.registry.GetConfig(providerID)
	return err
}

// refreshProviderToken refreshes a token using provider-specific logic
func (s *Server) refreshProviderToken(config *ProviderConfig, token *auth.ProviderToken) (auth.ProviderToken, error) {
	if token.RefreshToken == "" {
		return auth.ProviderToken{}, fmt.Errorf("no refresh token available")
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

	oauthToken := &oauth2.Token{
		RefreshToken: token.RefreshToken,
	}

	tokenSource := oauthConfig.TokenSource(ctx, oauthToken)
	newToken, err := tokenSource.Token()
	if err != nil {
		return auth.ProviderToken{}, fmt.Errorf("failed to refresh token: %w", err)
	}

	refreshed := auth.ProviderToken{
		AccessToken:  newToken.AccessToken,
		TokenType:    newToken.TokenType,
		RefreshToken: newToken.RefreshToken,
		ExpiryDate:   newToken.Expiry.UnixMilli(),
		ResourceURL:  token.ResourceURL,
	}

	return refreshed, nil
}

// handleProxyTest handles POST /api/proxy/test
// Tests a proxy connection without saving the configuration
func (s *Server) handleProxyTest(w http.ResponseWriter, r *http.Request) {
	s.logger.InfoLog("[ProxyTest] POST request received")

	// Only allow POST method
	if r.Method != http.MethodPost {
		s.logger.WarningLog("[ProxyTest] Method not allowed: %s", r.Method)
		WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Only POST method is allowed")
		return
	}

	// Parse request body
	var req TestProxyRequest
	if err := ParseJSON(r, &req); err != nil {
		s.logger.ErrorLog("[ProxyTest] Failed to parse request: %v", err)
		WriteErrorWithDetails(w, http.StatusBadRequest, "invalid_request", "Failed to parse request body", map[string]interface{}{
			"details": err.Error(),
		})
		return
	}

	// Validate required fields
	if req.Host == "" {
		s.logger.ErrorLog("[ProxyTest] Missing required field: host")
		WriteError(w, http.StatusBadRequest, "invalid_request", "Host is required")
		return
	}

	if req.Port <= 0 || req.Port > 65535 {
		s.logger.ErrorLog("[ProxyTest] Invalid port: %d", req.Port)
		WriteError(w, http.StatusBadRequest, "invalid_request", "Port must be between 1 and 65535")
		return
	}

	if req.Type == "" {
		req.Type = "http" // Default to HTTP proxy
	}

	// Validate proxy type
	proxyType := auth.ProxyType(req.Type)
	if err := proxyType.Validate(); err != nil {
		s.logger.ErrorLog("[ProxyTest] Invalid proxy type: %s", req.Type)
		WriteError(w, http.StatusBadRequest, "invalid_request", fmt.Sprintf("Invalid proxy type: %s", req.Type))
		return
	}

	// Create proxy configuration
	proxyConfig := &auth.ProxyConfig{
		Type:     proxyType,
		Host:     req.Host,
		Port:     req.Port,
		Username: req.Username,
		Password: req.Password,
		Enabled:  true,
	}

	// Create proxy tester and test connection
	proxyTester := auth.NewProxyTester(s.logger)
	result, err := proxyTester.TestConnection(r.Context(), proxyConfig)
	if err != nil {
		s.logger.ErrorLog("[ProxyTest] Test failed: %v", err)
		WriteError(w, http.StatusInternalServerError, "test_failed", err.Error())
		return
	}

	// Build response
	response := TestProxyResponse{
		Success:   result.Success,
		LatencyMs: result.LatencyMs,
		Error:     result.Error,
	}

	if result.Success {
		response.Message = fmt.Sprintf("Proxy connection successful (%dms)", result.LatencyMs)
		s.logger.InfoLog("[ProxyTest] Test successful - Host: %s:%d, Latency: %dms", req.Host, req.Port, result.LatencyMs)
	} else {
		response.Message = "Proxy connection failed"
		s.logger.WarningLog("[ProxyTest] Test failed - Host: %s:%d, Error: %s", req.Host, req.Port, result.Error)
	}

	WriteJSON(w, http.StatusOK, response)
}

// GetStateManager returns the state manager (for use by other packages)
func (s *Server) GetStateManager() *StateManager {
	return s.stateManager
}

// SetMultiTokenManager sets an external multi-token manager (for integration with main application)
func (s *Server) SetMultiTokenManager(multiTokenMgr *auth.MultiTokenManager) {
	s.multiTokenManager = multiTokenMgr
}

// GetMultiTokenManager returns the multi-token manager (for use by other packages)
func (s *Server) GetMultiTokenManager() *auth.MultiTokenManager {
	return s.multiTokenManager
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
