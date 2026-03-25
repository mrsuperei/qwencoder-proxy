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
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/sunbankio/qwencoder-proxy/internal/converter"
	"github.com/sunbankio/qwencoder-proxy/internal/logging"
	"github.com/sunbankio/qwencoder-proxy/internal/provider"
	"github.com/sunbankio/qwencoder-proxy/internal/provider/antigravity"
	"github.com/sunbankio/qwencoder-proxy/internal/provider/gemini"
	"github.com/sunbankio/qwencoder-proxy/internal/provider/iflow"
	"github.com/sunbankio/qwencoder-proxy/internal/provider/qwen"
	"github.com/sunbankio/qwencoder-proxy/internal/proxy"
	"github.com/sunbankio/qwencoder-proxy/internal/ratelimit"
	tokpkg "github.com/sunbankio/qwencoder-proxy/internal/token"
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
	logger            logging.Logger
	httpClient        *http.Client
	tokenStores       map[string]tokpkg.TokenStore    // providerID -> TokenStore interface
	tokenManagers     map[string]*tokpkg.TokenManager // providerID -> TokenManager
	multiTokenManager *tokpkg.MultiTokenManager       // Multi-token manager for all providers
	rateLimitManager  *ratelimit.QuotaManager         // Rate limit manager for quota enforcement
	cacheInvalidator  *ratelimit.CacheInvalidator     // Cache invalidator for cache management
}

// NewServer creates a new OAuth REST API server
func NewServer(config *Config, logger logging.Logger) *Server {
	if config == nil {
		config = DefaultConfig()
	}
	if logger == nil {
		// Create a simple logger if none provided
		logger = logging.NewLogger()
	}

	server := &Server{
		config:            config,
		registry:          NewProviderRegistry(),
		stateManager:      NewStateManager(logger),
		logger:            logger,
		httpClient:        &http.Client{Timeout: 30 * time.Second},
		tokenStores:       make(map[string]tokpkg.TokenStore),
		tokenManagers:     make(map[string]*tokpkg.TokenManager),
		multiTokenManager: nil, // Will be set via SetMultiTokenManager() from main application
	}

	// Note: Provider refreshers will be registered when SetMultiTokenManager() is called
	return server
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

// registerProviderRefreshers registers token refreshers for all providers
func (s *Server) registerProviderRefreshers() error {
	if s.multiTokenManager == nil {
		return fmt.Errorf("multi-token manager not initialized")
	}

	s.logger.InfoLog("[Server] Registering provider refreshers...")

	// Register Gemini refresher
	geminiRefresher := gemini.NewGeminiTokenRefresher(
		"681255809395-oo8ft2oprdrnp9e3aqf6av3hmdib135j.apps.googleusercontent.com",
		"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",
		"https://oauth2.googleapis.com/token",
		s.logger,
	)
	if err := s.multiTokenManager.RegisterRefresher("gemini-cli", geminiRefresher); err != nil {
		s.logger.ErrorLog("[Server] Failed to register Gemini refresher: %v", err)
		return fmt.Errorf("failed to register Gemini refresher: %w", err)
	}
	s.logger.InfoLog("[Server] Registered Gemini refresher")

	// Register Qwen refresher
	qwenRefresher := qwen.NewQwenTokenRefresher(s.logger)
	if err := s.multiTokenManager.RegisterRefresher("qwen", qwenRefresher); err != nil {
		s.logger.ErrorLog("[Server] Failed to register Qwen refresher: %v", err)
		return fmt.Errorf("failed to register Qwen refresher: %w", err)
	}
	s.logger.InfoLog("[Server] Registered Qwen refresher")

	// Register iFlow refresher
	iflowRefresher := iflow.NewIFlowTokenRefresher(
		"10009311001",
		"4Z3YjXycVsQvyGF1etiNlIBB4RsqSDtW",
		"https://iflow.cn/oauth/token",
		s.logger,
	)
	if err := s.multiTokenManager.RegisterRefresher("iflow", iflowRefresher); err != nil {
		s.logger.ErrorLog("[Server] Failed to register iFlow refresher: %v", err)
		return fmt.Errorf("failed to register iFlow refresher: %w", err)
	}
	s.logger.InfoLog("[Server] Registered iFlow refresher")

	// Register Antigravity refresher
	antigravityRefresher := antigravity.NewAntigravityTokenRefresher(
		"1071006060591-tmhssin2h21lcre235vtolojh4g403ep.apps.googleusercontent.com",
		"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",
		"https://oauth2.googleapis.com/token",
		s.logger,
	)
	if err := s.multiTokenManager.RegisterRefresher("antigravity", antigravityRefresher); err != nil {
		s.logger.ErrorLog("[Server] Failed to register Antigravity refresher: %v", err)
		return fmt.Errorf("failed to register Antigravity refresher: %w", err)
	}
	s.logger.InfoLog("[Server] Registered Antigravity refresher")

	s.logger.InfoLog("[Server] All provider refreshers registered successfully")
	return nil
}

// RegisterRoutes registers OAuth API routes with an external http.ServeMux
// This allows to OAuth server to be integrated into a larger server
func (s *Server) RegisterRoutes(mux *http.ServeMux) {
	s.registerRoutes(mux)
}

// registerRoutes registers all API routes
func (s *Server) registerRoutes(mux *http.ServeMux) {
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

	// Proxy management
	mux.HandleFunc("/api/proxies", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			s.handleListProxies(w, r)
		case http.MethodPost:
			s.handleAddProxy(w, r)
		default:
			WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed")
		}
	})
	mux.HandleFunc("/api/proxies/", s.handleProxy)

	// Rate limit management
	if s.rateLimitManager != nil {
		rateLimitAPI := NewRateLimitAPI(s.rateLimitManager, s.logger, s.cacheInvalidator)
		rateLimitAPI.RegisterRoutes(mux)
		s.logger.InfoLog("[registerRoutes] Rate limit API routes registered")
	}

	// Cache management API
	if s.cacheInvalidator != nil {
		cacheAPI := NewCacheAPI(s.cacheInvalidator, s.logger)
		cacheAPI.RegisterRoutes(mux)
		s.logger.InfoLog("[registerRoutes] Cache management API routes registered")
	}

	// Proxy connection test
	mux.HandleFunc("/api/proxy/test", s.handleProxyTest)
}

// getTokenStore gets or creates a token store for a provider
func (s *Server) getTokenStore(providerID string) (tokpkg.TokenStore, error) {
	if store, ok := s.tokenStores[providerID]; ok {
		return store, nil
	}

	// Use default database path
	dbPath := ".credentials/tokens.db"

	// Create SQLite store
	store, err := tokpkg.NewSQLiteStore(dbPath, providerID, s.logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create SQLite store: %w", err)
	}

	s.tokenStores[providerID] = store
	return store, nil
}

// getTokenManager gets or creates a token manager for a provider
func (s *Server) getTokenManager(providerID string) (*tokpkg.TokenManager, error) {
	if manager, ok := s.tokenManagers[providerID]; ok {
		return manager, nil
	}

	store, err := s.getTokenStore(providerID)
	if err != nil {
		return nil, err
	}

	// Type assertion to get *SQLiteStore (TokenManager requires concrete type)
	sqliteStore, ok := store.(*tokpkg.SQLiteStore)
	if !ok {
		return nil, fmt.Errorf("token store is not a SQLiteStore")
	}

	// Create strategy factory and get default strategy
	factory := tokpkg.NewStrategyFactory()
	strategy, err := factory.CreateStrategy(tokpkg.DefaultSelectionStrategy)
	if err != nil {
		return nil, err
	}

	// Get proxy health tracker for this provider
	proxyHealthTracker, _ := s.multiTokenManager.GetProxyHealthTracker(providerID)

	manager := tokpkg.NewTokenManager(sqliteStore, strategy, s.logger, nil, proxyHealthTracker)
	s.tokenManagers[providerID] = manager
	return manager, nil
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
		Email    string `json:"email,omitempty"`
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

	// For Qwen, email is required
	if req.Provider == "qwen" && req.Email == "" {
		s.logger.ErrorLog("[handleDeviceStart] Email is required for Qwen provider")
		WriteError(w, http.StatusBadRequest, "email_required", "Email or alias is required for Qwen provider")
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
		req.Email,
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
		s.pollForToken(pollCtx, pollID, req.Provider, req.Email, config, codeVerifier, deviceAuthResp)
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
func (s *Server) pollForToken(ctx context.Context, pollID, providerID, email string, config *ProviderConfig, codeVerifier string, deviceAuthResp *oauth2.DeviceAuthResponse) {
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

			creds := tokpkg.OAuthCreds{
				AccessToken:  token.AccessToken,
				TokenType:    token.TokenType,
				RefreshToken: token.RefreshToken,
				ExpiryDate:   token.Expiry.UnixMilli(),
			}

			if err := s.saveCredentials(providerID, creds, tokenResponse, email); err != nil {
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
		Email       string `json:"email,omitempty"`
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

	// For Qwen, email is required
	if req.Provider == "qwen" && req.Email == "" {
		s.logger.ErrorLog("[handleAuthStart] Email is required for Qwen provider")
		WriteError(w, http.StatusBadRequest, "email_required", "Email or alias is required for Qwen provider")
		return
	}

	// Create OAuth state
	state, err = s.stateManager.CreateState(req.Provider, codeVerifier, redirectURI, req.Email, s.config.StateTTL)
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

// handleCallback handles OAuth callback
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
	email := r.URL.Query().Get("email")

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
		s.logger.WarnLog("[Callback] Duplicate callback detected - code already processed: %s", code)
		// Return success to avoid confusing user (the original processing was successful)
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

	// Use email from state (if available) or from query parameter
	emailToUse := oauthState.Email
	if emailToUse == "" && email != "" {
		emailToUse = email
	}

	// Save credentials with email
	if err := s.saveCredentials(oauthState.Provider, creds, tokenResponseMap, emailToUse); err != nil {
		s.logger.ErrorLog("[Callback] Failed to save credentials: %v", err)
		s.writeCallbackHTML(w, false, "save_failed", err.Error())
		return
	}

	s.logger.InfoLog("[Callback] Credentials saved successfully for provider: %s", oauthState.Provider)
	s.writeCallbackHTML(w, true, "", "")
}

// writeCallbackHTML writes an HTML response for OAuth callback
func (s *Server) writeCallbackHTML(w http.ResponseWriter, success bool, errorCode, errorDesc string) {
	w.Header().Set("Content-Type", "text/html")

	if success {
		html := `<!DOCTYPE html>
<html>
<head>
	   <title>Authorization Successful</title>
	   <style>
	       body {
	           font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif;
	           display: flex;
	           justify-content: center;
	           align-items: center;
	           min-height: 100vh;
	           margin: 0;
	           background: #f9fafb;
	       }
	       .container {
	           text-align: center;
	           padding: 2rem;
	           background: white;
	           border-radius: 8px;
	           box-shadow: 0 4px 6px -1px rgba(0, 0, 0, 0.1);
	           max-width: 400px;
	       }
	       h1 {
	           color: #10b981;
	           margin-bottom: 1rem;
	       }
	       p {
	           color: #6b7280;
	           margin-bottom: 1.5rem;
	       }
	       .spinner {
	           border: 4px solid #e5e7eb;
	           border-top-color: #10b981;
	           border-radius: 50%;
	           width: 40px;
	           height: 40px;
	           animation: spin 1s linear infinite;
	           margin: 0 auto 1rem;
	       }
	       @keyframes spin {
	           to { transform: rotate(360deg); }
	       }
	       .success {
	           display: none;
	       }
	   </style>
</head>
<body>
	  <div class="container">
	      <div id="loading">
	          <div class="spinner"></div>
	          <p>Completing authentication...</p>
	      </div>
	      <div id="success" class="success">
	          <h1>Authorization Successful!</h1>
	          <p>Redirecting to dashboard...</p>
	      </div>
	  </div>
	  <script>
	    // Notify the parent window that authentication was successful
	    if (window.opener) {
	      window.opener.postMessage({type: 'oauth_success'}, '*');
	    }
	    
	    // Wait a bit before showing success message and redirecting
	    setTimeout(() => {
	        document.getElementById('loading').style.display = 'none';
	        document.getElementById('success').style.display = 'block';
	        
	        // Redirect to dashboard after showing success message
	        setTimeout(() => {
	            window.location.href = '/';
	        }, 1500);
	    }, 500);
	  </script>
</body>
</html>`
		w.Write([]byte(html))
	} else {
		html := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
	   <title>Authorization Failed</title>
	   <style>
	       body {
	           font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif;
	           display: flex;
	           justify-content: center;
	           align-items: center;
	           min-height: 100vh;
	           margin: 0;
	           background: #f9fafb;
	       }
	       .container {
	           text-align: center;
	           padding: 2rem;
	           background: white;
	           border-radius: 8px;
	           box-shadow: 0 4px 6px -1px rgba(0, 0, 0, 0.1);
	           max-width: 400px;
	       }
	       h1 {
	           color: #ef4444;
	           margin-bottom: 1rem;
	       }
	       p {
	           color: #6b7280;
	           margin-bottom: 0.75rem;
	       }
	       .error-code {
	           color: #ef4444;
	           font-weight: 600;
	       }
	   </style>
</head>
<body>
	  <div class="container">
	      <h1>Authorization Failed</h1>
	      <p class="error-code">Error: %s</p>
	      <p>%s</p>
	      <p>Redirecting to dashboard...</p>
	  </div>
	  <script>
	    // Notify the parent window that authentication failed
	    if (window.opener) {
	      window.opener.postMessage({type: 'oauth_error', error: '%s'}, '*');
	    }
	    
	    // Redirect to dashboard after showing error message
	    setTimeout(() => {
	        window.location.href = '/';
	    }, 3000);
	  </script>
</body>
</html>`, errorCode, errorDesc, errorCode)
		w.Write([]byte(html))
	}
}

// exchangeCodeForTokensWithResponse exchanges an authorization code for tokens and returns both creds and response map
func (s *Server) exchangeCodeForTokensWithResponse(config *ProviderConfig, code, redirectURI, codeVerifier string) (tokpkg.OAuthCreds, map[string]interface{}, error) {
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
		return tokpkg.OAuthCreds{}, nil, fmt.Errorf("failed to create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	s.logger.InfoLog("[exchangeCodeForTokensWithResponse] Sending token request to %s", config.TokenURL)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		s.logger.ErrorLog("[exchangeCodeForTokensWithResponse] Failed to send token request: %v", err)
		return tokpkg.OAuthCreds{}, nil, fmt.Errorf("failed to send token request: %w", err)
	}
	defer resp.Body.Close()

	s.logger.InfoLog("[exchangeCodeForTokensWithResponse] Received response - Status: %d", resp.StatusCode)

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		s.logger.ErrorLog("[exchangeCodeForTokensWithResponse] Token exchange failed - Status: %d, Body: %s", resp.StatusCode, string(body))
		return tokpkg.OAuthCreds{}, nil, fmt.Errorf("token exchange failed (status %d): %s", resp.StatusCode, string(body))
	}

	// Decode full response into map to preserve all fields including email
	var tokenResponse map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResponse); err != nil {
		s.logger.ErrorLog("[exchangeCodeForTokensWithResponse] Failed to decode token response: %v", err)
		return tokpkg.OAuthCreds{}, nil, fmt.Errorf("failed to decode token response: %w", err)
	}

	// Extract standard OAuth fields from the map
	accessToken, _ := tokenResponse["access_token"].(string)
	refreshToken, _ := tokenResponse["refresh_token"].(string)
	tokenType, _ := tokenResponse["token_type"].(string)
	expiresIn, _ := tokenResponse["expires_in"].(float64)
	resourceURL, _ := tokenResponse["resource_url"].(string)

	s.logger.InfoLog("[exchangeCodeForTokensWithResponse] Token response decoded - has_access_token: %v, has_refresh_token: %v, expires_in: %d",
		accessToken != "", refreshToken != "", int64(expiresIn))

	creds := tokpkg.OAuthCreds{
		AccessToken:  accessToken,
		TokenType:    tokenType,
		RefreshToken: refreshToken,
		ExpiryDate:   time.Now().Add(time.Duration(int64(expiresIn)) * time.Second).UnixMilli(), // Calculate absolute expiry time
		ResourceURL:  resourceURL,
	}

	return creds, tokenResponse, nil
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

// GetProviderConfig returns configuration for a provider (for use by other packages)
func (s *Server) GetProviderConfig(providerID string) (*ProviderConfig, error) {
	return s.registry.GetConfig(providerID)
}

// GetRegistry returns provider registry (for use by other packages)
func (s *Server) GetRegistry() *ProviderRegistry {
	return s.registry
}

// GetStateManager returns state manager (for use by other packages)
func (s *Server) GetStateManager() *StateManager {
	return s.stateManager
}

// SetMultiTokenManager sets an external multi-token manager (for integration with main application)
// This ensures both REST API server and providers use the same token manager instance
func (s *Server) SetMultiTokenManager(multiTokenMgr *tokpkg.MultiTokenManager) {
	s.multiTokenManager = multiTokenMgr

	// Register provider refreshers with shared multi-token manager
	if err := s.registerProviderRefreshers(); err != nil {
		s.logger.ErrorLog("Failed to register provider refreshers: %v", err)
	}
}

// SetRateLimitManager sets the rate limit manager for rate limiting enforcement
func (s *Server) SetRateLimitManager(rateLimitMgr *ratelimit.QuotaManager) {
	s.rateLimitManager = rateLimitMgr
	s.logger.InfoLog("[Server] Rate limit manager set")
}

// SetCacheInvalidator sets the cache invalidator for cache management
func (s *Server) SetCacheInvalidator(invalidator *ratelimit.CacheInvalidator) {
	s.cacheInvalidator = invalidator
	s.logger.InfoLog("[Server] Cache invalidator set")
}

// GetMultiTokenManager returns multi-token manager (for use by other packages)
func (s *Server) GetMultiTokenManager() *tokpkg.MultiTokenManager {
	return s.multiTokenManager
}

// RegisterProxyRoutes registers OpenAI-compatible proxy routes with sequential handler
// This method creates a SequentialHandler that implements the sequential flow:
// Model → Provider → Token Selection → API Request → Usage Tracking
func (s *Server) RegisterProxyRoutes(mux *http.ServeMux, factory *provider.Factory, convFactory *converter.Factory) {
	// Create sequential handler if rate limit manager is available
	if s.rateLimitManager != nil {
		// Get token selector from quota manager
		tokenSelector := s.rateLimitManager.GetTokenSelector()

		// Create sequential handler
		sequentialHandler := proxy.NewSequentialHandler(
			factory,
			convFactory,
			tokenSelector,
			s.rateLimitManager,
			s.logger,
		)

		// Register general /v1/ route with sequential handler
		mux.Handle("/v1/", sequentialHandler)
		s.logger.InfoLog("[RegisterProxyRoutes] Sequential handler registered for /v1/ route")

		// Register provider-specific routes with sequential handler
		// These routes use the same sequential handler but with fixed provider path
		mux.Handle("/qwen/v1/", sequentialHandler)
		mux.Handle("/gemini/v1/", sequentialHandler)
		mux.Handle("/kiro/v1/", sequentialHandler)
		mux.Handle("/antigravity/v1/", sequentialHandler)
		mux.Handle("/iflow/v1/", sequentialHandler)

		s.logger.InfoLog("[RegisterProxyRoutes] Sequential handler registered for all provider routes")
	} else {
		// Fallback to original handlers if rate limit manager is not available
		s.logger.WarnLog("[RegisterProxyRoutes] Rate limit manager not available, using fallback handlers")
		proxy.RegisterOpenAIRoutesWithTokenManager(mux, factory, convFactory, nil)
		proxy.RegisterProviderSpecificRoutesWithTokenManager(mux, factory, convFactory, nil)
	}

	s.logger.InfoLog("[RegisterProxyRoutes] Proxy routes registered")
}

// saveCredentials saves credentials for a provider with email
func (s *Server) saveCredentials(providerID string, creds tokpkg.OAuthCreds, tokenResponse map[string]interface{}, providedEmail string) error {
	var email string

	// For Qwen, use provided email (required)
	if providerID == "qwen" {
		email = strings.TrimSpace(strings.ToLower(providedEmail))
		if email == "" {
			return fmt.Errorf("email is required for Qwen provider")
		}
		s.logger.InfoLog("[saveCredentials] Using provided email for Qwen: %s", email)
	} else {
		// For other providers, try to extract email from token
		extractedEmail, err := s.extractEmailFromToken(providerID, creds.AccessToken, tokenResponse)
		if err != nil {
			s.logger.WarnLog("Failed to extract email for %s: %v", providerID, err)
			email = "unknown@example.com"
		} else {
			email = extractedEmail
		}
	}

	// Save credentials to database
	store, err := s.getTokenStore(providerID)
	if err != nil {
		return fmt.Errorf("failed to get token store: %w", err)
	}

	// Create provider token
	token := tokpkg.ProviderToken{
		ID:           uuid.New().String(),
		AccessToken:  creds.AccessToken,
		RefreshToken: creds.RefreshToken,
		TokenType:    creds.TokenType,
		ExpiryDate:   creds.ExpiryDate,
		ResourceURL:  creds.ResourceURL,
		Email:        email,
		Healthy:      true,
		HealthScore:  1.0,
		LastUsed:     time.Now().UnixMilli(),
		CreatedAt:    time.Now().UnixMilli(),
		ErrorCount:   0,
	}

	// Load existing tokens
	tokensMap, loadErr := store.Load()
	if loadErr != nil {
		return fmt.Errorf("failed to load tokens: %w", loadErr)
	}

	// Add new token
	tokensMap[token.ID] = token

	// Save all tokens
	if err := store.Save(tokensMap); err != nil {
		return fmt.Errorf("failed to save tokens: %w", err)
	}

	s.logger.InfoLog("Credentials saved successfully for provider %s: %s", providerID, email)
	return nil
}

// extractEmailFromToken extracts email from an access token
func (s *Server) extractEmailFromToken(providerID, accessToken string, tokenResponse map[string]interface{}) (string, error) {
	// Use the multiTokenManager's email extraction manager which has all extractors registered
	email, err := s.multiTokenManager.ExtractEmail(context.Background(), providerID, tokenResponse, accessToken)
	if err != nil {
		return "", fmt.Errorf("failed to extract email: %w", err)
	}

	return email, nil
}
