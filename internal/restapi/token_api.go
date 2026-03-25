// Package restapi provides REST API endpoints for OAuth2 authentication flows
package restapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	tokpkg "github.com/sunbankio/qwencoder-proxy/internal/token"
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
	ProviderID string               `json:"provider_id"`
	Tokens     []ProviderTokenInfo  `json:"tokens"`
	Settings   tokpkg.StoreSettings `json:"settings"`
}

// TokenSelectionResponse represents the response when selecting a token
type TokenSelectionResponse struct {
	AccessToken string `json:"access_token"`
	Email       string `json:"email"`
	TokenID     string `json:"token_id"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int64  `json:"expires_in"`
}

// ProxyConfigResponse represents the response for proxy configuration
type ProxyConfigResponse struct {
	TokenID      string              `json:"token_id"`
	Proxy        *tokpkg.ProxyConfig `json:"proxy"`
	HealthStatus *tokpkg.ProxyHealth `json:"health_status,omitempty"`
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

// loadCredentials loads credentials for a provider
func (s *Server) loadCredentials(providerID string) (tokpkg.OAuthCreds, error) {
	credsPath, err := s.registry.GetCredentialsPath(providerID)
	if err != nil {
		return tokpkg.OAuthCreds{}, err
	}

	data, err := os.ReadFile(credsPath)
	if err != nil {
		return tokpkg.OAuthCreds{}, err
	}

	var creds tokpkg.OAuthCreds
	if err := json.Unmarshal(data, &creds); err != nil {
		return tokpkg.OAuthCreds{}, err
	}

	return creds, nil
}

// refreshToken refreshes an access token using the refresh token
func (s *Server) refreshToken(config *ProviderConfig, creds tokpkg.OAuthCreds) (tokpkg.OAuthCreds, error) {
	if creds.RefreshToken == "" {
		return tokpkg.OAuthCreds{}, fmt.Errorf("no refresh token available")
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
		return tokpkg.OAuthCreds{}, fmt.Errorf("failed to refresh token: %w", err)
	}

	updated := tokpkg.OAuthCreds{
		AccessToken:  newToken.AccessToken,
		TokenType:    newToken.TokenType,
		RefreshToken: newToken.RefreshToken,
		ExpiryDate:   newToken.Expiry.UnixMilli(),
	}

	if resourceURL, ok := newToken.Extra("resource_url").(string); ok {
		updated.ResourceURL = resourceURL
	}

	// Save updated credentials to file (for backward compatibility)
	credsPath, err := s.registry.GetCredentialsPath(config.ID)
	if err != nil {
		return tokpkg.OAuthCreds{}, fmt.Errorf("failed to get credentials path: %w", err)
	}

	data, err := json.Marshal(updated)
	if err != nil {
		return tokpkg.OAuthCreds{}, fmt.Errorf("failed to marshal credentials: %w", err)
	}

	if err := os.WriteFile(credsPath, data, 0644); err != nil {
		return tokpkg.OAuthCreds{}, fmt.Errorf("failed to save credentials: %w", err)
	}

	return updated, nil
}

// providerTokenToInfo converts ProviderToken to ProviderTokenInfo
func (s *Server) providerTokenToInfo(token tokpkg.ProviderToken) ProviderTokenInfo {
	s.logger.InfoLog("[providerTokenToInfo] Converting token with ID: %s", token.ID)
	expiresIn := (token.ExpiryDate - time.Now().UnixMilli()) / 1000
	if expiresIn < 0 {
		expiresIn = 0
	}

	info := ProviderTokenInfo{
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
	s.logger.InfoLog("[providerTokenToInfo] Conversion complete")
	return info
}

// validateProvider validates that provider ID is valid
func (s *Server) validateProvider(providerID string) error {
	// Get provider config to validate
	_, err := s.registry.GetConfig(providerID)
	return err
}

// refreshProviderToken refreshes a token using provider-specific logic
func (s *Server) refreshProviderToken(config *ProviderConfig, token *tokpkg.ProviderToken) (tokpkg.ProviderToken, error) {
	s.logger.InfoLog("[refreshProviderToken] Starting token refresh, tokenURL: %s", config.TokenURL)
	if token.RefreshToken == "" {
		s.logger.ErrorLog("[refreshProviderToken] No refresh token available")
		return tokpkg.ProviderToken{}, fmt.Errorf("no refresh token available")
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

	s.logger.InfoLog("[refreshProviderToken] Creating token source and calling Token()...")
	tokenSource := oauthConfig.TokenSource(ctx, oauthToken)
	newToken, err := tokenSource.Token()
	if err != nil {
		s.logger.ErrorLog("[refreshProviderToken] Token refresh failed: %v", err)
		return tokpkg.ProviderToken{}, fmt.Errorf("failed to refresh token: %w", err)
	}
	s.logger.InfoLog("[refreshProviderToken] Token refreshed successfully")

	refreshed := tokpkg.ProviderToken{
		AccessToken:  newToken.AccessToken,
		TokenType:    newToken.TokenType,
		RefreshToken: newToken.RefreshToken,
		ExpiryDate:   newToken.Expiry.UnixMilli(),
		ResourceURL:  token.ResourceURL,
	}

	return refreshed, nil
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

	// Update LastUsed timestamp using UpdateToken (more efficient than Save)
	store, err := s.multiTokenManager.GetTokenStore(providerID)
	if err != nil {
		s.logger.WarnLog("Failed to get token store: %v", err)
	} else {
		// Update just the selected token's LastUsed timestamp
		if updateErr := store.UpdateToken(token.ID, func(t *tokpkg.ProviderToken) {
			t.LastUsed = time.Now().UnixMilli()
		}); updateErr != nil {
			s.logger.WarnLog("Failed to update LastUsed timestamp: %v", updateErr)
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

		// Load tokens using interface method
		tokensMap, loadErr := store.Load()
		if loadErr != nil {
			continue // Skip providers that fail to load
		}

		// Convert map to slice
		tokens := make([]tokpkg.ProviderToken, 0, len(tokensMap))
		for _, token := range tokensMap {
			tokens = append(tokens, token)
		}

		// Get settings using interface method
		settings, settingsErr := store.GetSettings()
		if settingsErr != nil {
			continue // Skip providers that fail to get settings
		}

		// Calculate valid token count
		validTokenCount := 0
		now := time.Now().UnixMilli()
		for _, token := range tokens {
			if token.Healthy && (token.ExpiryDate == 0 || token.ExpiryDate > now) {
				validTokenCount++
			}
		}

		// Convert tokens to token infos
		tokenInfos := make([]ProviderTokenInfo, 0, len(tokens))
		for _, token := range tokens {
			tokenInfos = append(tokenInfos, s.providerTokenToInfo(token))
		}

		info := map[string]interface{}{
			"provider":     provider.ID,
			"total_tokens": len(tokens),
			"valid_tokens": validTokenCount,
			"settings":     settings,
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

// handleProviderCredentials handles provider-specific credential operations
func (s *Server) handleProviderCredentials(w http.ResponseWriter, r *http.Request) {
	s.logger.InfoLog("[handleProviderCredentials] Path: %s, Method: %s", r.URL.Path, r.Method)
	// Extract provider ID and action from path
	// Path format: /api/credentials/{provider} or /api/credentials/{provider}/{tokenID}/{action}
	// Normalize path by removing duplicate slashes
	normalizedPath := strings.Join(strings.Split(r.URL.Path, "/"), "/")
	parts := strings.Split(normalizedPath, "/")
	s.logger.InfoLog("[handleProviderCredentials] Parts: %v, Length: %d", parts, len(parts))
	if len(parts) < 4 {
		WriteError(w, http.StatusNotFound, "not_found", "Endpoint not found")
		return
	}

	providerID := parts[3]
	s.logger.InfoLog("[handleProviderCredentials] providerID: %s (from parts[3])", providerID)

	switch r.Method {
	case http.MethodGet:
		if len(parts) == 4 {
			// GET /api/credentials/{provider} - List all tokens for a provider
			s.handleGetProviderCredentials(w, r, providerID)
		} else if len(parts) == 6 && parts[5] == "proxy" {
			// GET /api/credentials/{provider}/{tokenID}/proxy - Get proxy config for a token
			tokenID := parts[4]
			s.getProxyConfigHandler(w, r, providerID, tokenID)
		} else {
			WriteError(w, http.StatusNotFound, "not_found", "Endpoint not found")
		}
	case http.MethodPost:
		if len(parts) == 4 {
			// POST /api/credentials/{provider} - Add a new token
			s.handleAddToken(w, r, providerID)
		} else if len(parts) == 6 && parts[5] == "refresh" {
			// POST /api/credentials/{provider}/{tokenID}/refresh - Refresh a specific token
			tokenID := parts[4]
			s.handleRefreshTokenByID(w, r, providerID, tokenID)
		} else {
			WriteError(w, http.StatusNotFound, "not_found", "Endpoint not found")
		}
	case http.MethodDelete:
		if len(parts) == 5 {
			// DELETE /api/credentials/{provider}/{tokenID} - Delete a specific token
			tokenID := parts[4]
			s.handleDeleteTokenByID(w, r, providerID, tokenID)
		} else if len(parts) == 6 && parts[5] == "proxy" {
			// DELETE /api/credentials/{provider}/{tokenID}/proxy - Remove proxy config for a token
			tokenID := parts[4]
			s.deleteProxyConfigHandler(w, r, providerID, tokenID)
		} else {
			WriteError(w, http.StatusNotFound, "not_found", "Endpoint not found")
		}
	case http.MethodPut:
		if len(parts) == 5 && parts[4] == "settings" {
			// PUT /api/credentials/{provider}/settings - Update provider settings
			s.handleUpdateProviderSettings(w, r, providerID)
		} else if len(parts) == 6 && parts[5] == "proxy" {
			// PUT /api/credentials/{provider}/{tokenID}/proxy - Update proxy config for a token
			tokenID := parts[4]
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

	// Load tokens using interface method
	var tokensMap map[string]tokpkg.TokenMetadata
	tokensMap, err = store.Load()
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "load_failed", err.Error())
		return
	}

	// Convert map to slice
	tokens := make([]tokpkg.ProviderToken, 0, len(tokensMap))
	for _, token := range tokensMap {
		tokens = append(tokens, token)
	}

	tokenInfos := make([]ProviderTokenInfo, 0, len(tokens))
	for _, token := range tokens {
		tokenInfos = append(tokenInfos, s.providerTokenToInfo(token))
	}

	// Get settings using interface method
	settings, err := store.GetSettings()
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "settings_failed", err.Error())
		return
	}

	WriteJSON(w, http.StatusOK, ProviderCredentialsInfo{
		ProviderID: providerID,
		Tokens:     tokenInfos,
		Settings:   settings,
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
		extractor := tokpkg.NewEmailExtractionManager(s.logger)
		emailExtractor, err := extractor.GetExtractor(providerID)
		if err != nil {
			s.logger.WarnLog("Failed to get email extractor for %s: %v", providerID, err)
			email = "unknown@example.com"
		} else {
			if extractedEmail, err := emailExtractor.ExtractEmail(context.Background(), nil, req.AccessToken); err == nil {
				email = extractedEmail
			} else {
				s.logger.WarnLog("Failed to extract email for %s: %v", providerID, err)
				email = "unknown@example.com"
			}
		}
	}

	// Calculate expiry date
	expiryDate := int64(0)
	if req.ExpiresIn > 0 {
		expiryDate = time.Now().UnixMilli() + (req.ExpiresIn * 1000)
	}

	token := tokpkg.ProviderToken{
		ID:           uuid.New().String(),
		AccessToken:  req.AccessToken,
		RefreshToken: req.RefreshToken,
		TokenType:    req.TokenType,
		ExpiryDate:   expiryDate,
		ResourceURL:  req.ResourceURL,
		Email:        email,
		Healthy:      true,
		HealthScore:  1.0,
		LastUsed:     time.Now().UnixMilli(),
		CreatedAt:    time.Now().UnixMilli(),
		ErrorCount:   0,
	}

	// Load existing tokens
	var tokensMap map[string]tokpkg.TokenMetadata
	tokensMap, err = store.Load()
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "load_failed", err.Error())
		return
	}

	// Add new token
	tokensMap[token.ID] = token

	// Save all tokens
	if err := store.Save(tokensMap); err != nil {
		WriteError(w, http.StatusInternalServerError, "save_failed", err.Error())
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

	// Use RemoveToken method directly
	if err := store.RemoveToken(tokenID); err != nil {
		if strings.Contains(err.Error(), "not found") {
			WriteError(w, http.StatusNotFound, "token_not_found", err.Error())
		} else {
			WriteError(w, http.StatusInternalServerError, "delete_failed", err.Error())
		}
		return
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "Token deleted successfully",
	})
}

// handleRefreshTokenByID refreshes a specific token
func (s *Server) handleRefreshTokenByID(w http.ResponseWriter, r *http.Request, providerID, tokenID string) {
	defer func() {
		if r := recover(); r != nil {
			s.logger.ErrorLog("[handleRefreshTokenByID] PANIC recovered: %v", r)
			WriteError(w, http.StatusInternalServerError, "internal_error", fmt.Sprintf("Internal error: %v", r))
		}
	}()
	s.logger.InfoLog("[handleRefreshTokenByID] Starting refresh for provider: %s, tokenID: %s", providerID, tokenID)
	store, err := s.multiTokenManager.GetTokenStore(providerID)
	if err != nil {
		s.logger.ErrorLog("[handleRefreshTokenByID] Failed to get token store: %v", err)
		WriteError(w, http.StatusNotFound, "not_found", err.Error())
		return
	}

	// Load just the token being refreshed (not all tokens)
	tokenMetadata, err := store.GetToken(tokenID)
	if err != nil {
		s.logger.ErrorLog("[handleRefreshTokenByID] Failed to get token: %v", err)
		WriteError(w, http.StatusNotFound, "token_not_found", fmt.Sprintf("Token not found: %s", tokenID))
		return
	}
	s.logger.InfoLog("[handleRefreshTokenByID] Loaded token %s, has refresh token: %v", tokenID, tokenMetadata.RefreshToken != "")

	// Get provider config for refresh
	config, err := s.registry.GetConfig(providerID)
	if err != nil {
		s.logger.ErrorLog("[handleRefreshTokenByID] Failed to get provider config: %v", err)
		WriteError(w, http.StatusBadRequest, "invalid_provider", err.Error())
		return
	}
	s.logger.InfoLog("[handleRefreshTokenByID] Got provider config, tokenURL: %s", config.TokenURL)

	// Refresh token using provider-specific logic
	s.logger.InfoLog("[handleRefreshTokenByID] Calling refreshProviderToken...")
	refreshed, err := s.refreshProviderToken(config, tokenMetadata)
	if err != nil {
		s.logger.ErrorLog("[handleRefreshTokenByID] Failed to refresh token: %v", err)
		WriteError(w, http.StatusInternalServerError, "refresh_failed", err.Error())
		return
	}
	s.logger.InfoLog("[handleRefreshTokenByID] Token refreshed successfully")

	// Preserve original metadata
	originalEmail := tokenMetadata.Email
	originalCreatedAt := tokenMetadata.CreatedAt

	// Update just this one token in the store using UpdateToken
	s.logger.InfoLog("[handleRefreshTokenByID] Updating token in store...")
	err = store.UpdateToken(tokenID, func(t *tokpkg.ProviderToken) {
		t.AccessToken = refreshed.AccessToken
		t.RefreshToken = refreshed.RefreshToken
		t.TokenType = refreshed.TokenType
		t.ExpiryDate = refreshed.ExpiryDate
		t.Email = originalEmail
		t.CreatedAt = originalCreatedAt
		t.LastUsed = time.Now().UnixMilli()
		t.Healthy = true
		t.HealthScore = 1.0
		t.ErrorCount = 0
		t.LastError = ""
	})
	if err != nil {
		s.logger.ErrorLog("[handleRefreshTokenByID] Failed to update token: %v", err)
		WriteError(w, http.StatusInternalServerError, "save_failed", err.Error())
		return
	}
	s.logger.InfoLog("[handleRefreshTokenByID] Token updated successfully")

	s.logger.InfoLog("[handleRefreshTokenByID] Writing JSON response...")
	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"token":   s.providerTokenToInfo(refreshed),
	})
	s.logger.InfoLog("[handleRefreshTokenByID] Response written successfully")
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
	settings, err := store.GetSettings()
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "get_settings_failed", err.Error())
		return
	}

	if req.SelectionStrategy != "" {
		// Validate strategy
		factory := tokpkg.NewStrategyFactory()
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

	// Update the timestamp
	settings.UpdatedAt = time.Now().UnixMilli()

	if err := store.SaveSettings(settings); err != nil {
		WriteError(w, http.StatusInternalServerError, "save_failed", err.Error())
		return
	}

	// Update token manager strategy if it exists
	if manager, ok := s.tokenManagers[providerID]; ok {
		factory := tokpkg.NewStrategyFactory()
		if strategy, err := factory.CreateStrategy(settings.SelectionStrategy); err == nil {
			manager.SetStrategy(strategy)
		}
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"success":  true,
		"settings": settings,
	})
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
	var tokensMap map[string]tokpkg.TokenMetadata
	tokensMap, err = store.Load()
	if err != nil {
		s.logger.ErrorLog("[getProxyConfig] Failed to load tokens - provider: %s: %v", providerID, err)
		WriteError(w, http.StatusInternalServerError, "load_failed", err.Error())
		return
	}

	token, exists := tokensMap[tokenID]
	if !exists {
		s.logger.ErrorLog("[getProxyConfig] Token not found - provider: %s, tokenID: %s", providerID, tokenID)
		WriteError(w, http.StatusNotFound, "not_found", "Token not found")
		return
	}

	// Get proxy health tracker
	proxyHealthTracker, err := s.multiTokenManager.GetProxyHealthTracker(providerID)
	if err != nil {
		s.logger.WarnLog("[getProxyConfig] Failed to get proxy health tracker for %s: %v", providerID, err)
	}

	// Get health status
	var healthStatus *tokpkg.ProxyHealth
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
	var proxyConfig tokpkg.ProxyConfig
	if err := ParseJSON(r, &proxyConfig); err != nil {
		s.logger.ErrorLog("[updateProxyConfig] Failed to parse request: %v", err)
		WriteError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	// Set default proxy type if not specified
	if proxyConfig.Type == "" {
		proxyConfig.Type = tokpkg.ProxyTypeNone
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

	// Log the change for audit trail
	s.logger.InfoLog("[updateProxyConfig] Updating proxy config for token %s - Type: %s, Host: %s, Port: %d",
		tokenID, proxyConfig.Type, proxyConfig.Host, proxyConfig.Port)

	// Update just this one token using UpdateToken (more efficient than Save)
	err = store.UpdateToken(tokenID, func(t *tokpkg.ProviderToken) {
		t.Proxy = &proxyConfig
		t.ProxyHealthScore = 1.0 // Reset proxy health score on config change
	})
	if err != nil {
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

	// Check if proxy was configured
	token, err := store.GetToken(tokenID)
	if err != nil {
		s.logger.ErrorLog("[deleteProxyConfig] Token not found - provider: %s, tokenID: %s", providerID, tokenID)
		WriteError(w, http.StatusNotFound, "not_found", "Token not found")
		return
	}

	hadProxy := token.Proxy != nil

	// Log the deletion for audit trail
	s.logger.InfoLog("[deleteProxyConfig] Removing proxy config for token %s (had proxy: %v)", tokenID, hadProxy)

	// Update just this one token using UpdateToken (more efficient than Save)
	err = store.UpdateToken(tokenID, func(t *tokpkg.ProviderToken) {
		t.Proxy = nil
		t.ProxyHealthScore = 1.0 // Reset proxy health score
	})
	if err != nil {
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

// handleProxyTest handles POST /api/proxy/test
// Tests a proxy connection without saving configuration
func (s *Server) handleProxyTest(w http.ResponseWriter, r *http.Request) {
	s.logger.InfoLog("[ProxyTest] POST request received")

	// Only allow POST method
	if r.Method != http.MethodPost {
		s.logger.WarnLog("[ProxyTest] Method not allowed: %s", r.Method)
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
	proxyType := tokpkg.ProxyType(req.Type)
	if err := proxyType.Validate(); err != nil {
		s.logger.ErrorLog("[ProxyTest] Invalid proxy type: %s", req.Type)
		WriteError(w, http.StatusBadRequest, "invalid_request", fmt.Sprintf("Invalid proxy type: %s", req.Type))
		return
	}

	// Create proxy configuration
	proxyConfig := &tokpkg.ProxyConfig{
		Type:     proxyType,
		Host:     req.Host,
		Port:     req.Port,
		Username: req.Username,
		Password: req.Password,
	}

	// Create proxy tester and test connection
	proxyTester := tokpkg.NewProxyTester(s.logger)
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
		s.logger.WarnLog("[ProxyTest] Test failed - Host: %s:%d, Error: %s", req.Host, req.Port, result.Error)
	}

	WriteJSON(w, http.StatusOK, response)
}
