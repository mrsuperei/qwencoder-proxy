// Package restapi provides REST API endpoints for proxy management
package restapi

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"
	tokpkg "github.com/sunbankio/qwencoder-proxy/internal/token"
)

// ProxyInfo represents proxy information for API responses
type ProxyInfo struct {
	ID         string `json:"id"`
	Type       string `json:"type"`
	Host       string `json:"host"`
	Port       int    `json:"port"`
	Username   string `json:"username,omitempty"`
	CreatedAt  int64  `json:"created_at"`
	TokenCount int    `json:"token_count"`
}

// ProxyRequest represents a request to create or update a proxy
type ProxyRequest struct {
	Type     string `json:"type"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
}

// ProxyTokensResponse represents the response for tokens linked to a proxy
type ProxyTokensResponse struct {
	ProxyID string              `json:"proxy_id"`
	Tokens  []ProviderTokenInfo `json:"tokens"`
}

// getSQLiteStore gets a SQLiteStore for a provider
func (s *Server) getSQLiteStore(providerID string) (*tokpkg.SQLiteStore, error) {
	store, err := s.getTokenStore(providerID)
	if err != nil {
		return nil, err
	}

	// Type assertion to get *SQLiteStore
	sqliteStore, ok := store.(*tokpkg.SQLiteStore)
	if !ok {
		return nil, fmt.Errorf("token store is not a SQLiteStore")
	}

	return sqliteStore, nil
}

// handleListProxies handles GET /api/proxies
func (s *Server) handleListProxies(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Only GET method is allowed")
		return
	}

	// Get SQLiteStore (proxy_configs is shared across all providers)
	store, err := s.getSQLiteStore("qwen") // Use qwen as default to get a store
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "store_error", err.Error())
		return
	}

	// List all proxies
	proxies, err := store.ListProxies()
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "list_failed", err.Error())
		return
	}

	// Build response with token counts
	proxyInfos := make([]ProxyInfo, 0, len(proxies))
	for _, proxy := range proxies {
		tokenCount, err := store.GetProxyTokenCount(proxy.ID)
		if err != nil {
			s.logger.WarnLog("[Proxy] Failed to get token count for proxy %s: %v", proxy.ID, err)
			tokenCount = 0
		}

		proxyInfos = append(proxyInfos, ProxyInfo{
			ID:         proxy.ID,
			Type:       string(proxy.Type),
			Host:       proxy.Host,
			Port:       proxy.Port,
			Username:   proxy.Username,
			CreatedAt:  0, // We don't have created_at in ProxyConfig
			TokenCount: tokenCount,
		})
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"proxies": proxyInfos,
	})
}

// handleAddProxy handles POST /api/proxies
func (s *Server) handleAddProxy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Only POST method is allowed")
		return
	}

	// Parse request body
	var req ProxyRequest
	if err := ParseJSON(r, &req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	// Validate required fields
	if req.Host == "" {
		WriteError(w, http.StatusBadRequest, "invalid_request", "Host is required")
		return
	}

	if req.Port <= 0 || req.Port > 65535 {
		WriteError(w, http.StatusBadRequest, "invalid_request", "Port must be between 1 and 65535")
		return
	}

	// Set default proxy type
	if req.Type == "" {
		req.Type = "http"
	}

	// Validate proxy type
	proxyType := tokpkg.ProxyType(req.Type)
	if err := proxyType.Validate(); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_request", fmt.Sprintf("Invalid proxy type: %s", req.Type))
		return
	}

	// Create proxy configuration
	proxy := tokpkg.ProxyConfig{
		ID:       uuid.New().String(),
		Type:     proxyType,
		Host:     req.Host,
		Port:     req.Port,
		Username: req.Username,
		Password: req.Password,
	}

	// Get a SQLiteStore to add the proxy
	store, err := s.getSQLiteStore("qwen")
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "store_error", err.Error())
		return
	}

	// Add proxy
	if err := store.AddProxy(proxy); err != nil {
		WriteError(w, http.StatusInternalServerError, "add_failed", err.Error())
		return
	}

	s.logger.InfoLog("[Proxy] Added proxy: %s (%s://%s:%d)", proxy.ID, proxy.Type, proxy.Host, proxy.Port)

	WriteJSON(w, http.StatusCreated, map[string]interface{}{
		"id":      proxy.ID,
		"message": "Proxy added successfully",
	})
}

// handleGetProxy handles GET /api/proxies/{id}
func (s *Server) handleGetProxy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Only GET method is allowed")
		return
	}

	// Extract proxy ID from path
	proxyID := extractProxyID(r.URL.Path)
	if proxyID == "" {
		WriteError(w, http.StatusBadRequest, "invalid_request", "Proxy ID is required")
		return
	}

	// Get a SQLiteStore
	store, err := s.getSQLiteStore("qwen")
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "store_error", err.Error())
		return
	}

	// Get proxy
	proxy, err := store.GetProxy(proxyID)
	if err != nil {
		WriteError(w, http.StatusNotFound, "not_found", err.Error())
		return
	}

	// Get token count
	tokenCount, err := store.GetProxyTokenCount(proxyID)
	if err != nil {
		s.logger.WarnLog("[Proxy] Failed to get token count for proxy %s: %v", proxyID, err)
		tokenCount = 0
	}

	WriteJSON(w, http.StatusOK, ProxyInfo{
		ID:         proxy.ID,
		Type:       string(proxy.Type),
		Host:       proxy.Host,
		Port:       proxy.Port,
		Username:   proxy.Username,
		TokenCount: tokenCount,
	})
}

// handleUpdateProxy handles PUT /api/proxies/{id}
func (s *Server) handleUpdateProxy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Only PUT method is allowed")
		return
	}

	// Extract proxy ID from path
	proxyID := extractProxyID(r.URL.Path)
	if proxyID == "" {
		WriteError(w, http.StatusBadRequest, "invalid_request", "Proxy ID is required")
		return
	}

	// Parse request body
	var req ProxyRequest
	if err := ParseJSON(r, &req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	// Validate required fields
	if req.Host == "" {
		WriteError(w, http.StatusBadRequest, "invalid_request", "Host is required")
		return
	}

	if req.Port <= 0 || req.Port > 65535 {
		WriteError(w, http.StatusBadRequest, "invalid_request", "Port must be between 1 and 65535")
		return
	}

	// Validate proxy type
	proxyType := tokpkg.ProxyType(req.Type)
	if err := proxyType.Validate(); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_request", fmt.Sprintf("Invalid proxy type: %s", req.Type))
		return
	}

	// Create proxy configuration
	proxy := tokpkg.ProxyConfig{
		Type:     proxyType,
		Host:     req.Host,
		Port:     req.Port,
		Username: req.Username,
		Password: req.Password,
	}

	// Get a SQLiteStore
	store, err := s.getSQLiteStore("qwen")
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "store_error", err.Error())
		return
	}

	// Update proxy
	if err := store.UpdateProxy(proxyID, proxy); err != nil {
		WriteError(w, http.StatusInternalServerError, "update_failed", err.Error())
		return
	}

	s.logger.InfoLog("[Proxy] Updated proxy: %s (%s://%s:%d)", proxyID, proxy.Type, proxy.Host, proxy.Port)

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"message": "Proxy updated successfully",
	})
}

// handleDeleteProxy handles DELETE /api/proxies/{id}
func (s *Server) handleDeleteProxy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Only DELETE method is allowed")
		return
	}

	// Extract proxy ID from path
	proxyID := extractProxyID(r.URL.Path)
	if proxyID == "" {
		WriteError(w, http.StatusBadRequest, "invalid_request", "Proxy ID is required")
		return
	}

	// Get a SQLiteStore
	store, err := s.getSQLiteStore("qwen")
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "store_error", err.Error())
		return
	}

	// Check if proxy exists
	proxy, err := store.GetProxy(proxyID)
	if err != nil {
		WriteError(w, http.StatusNotFound, "not_found", err.Error())
		return
	}

	// Get token count before deletion
	tokenCount, err := store.GetProxyTokenCount(proxyID)
	if err != nil {
		s.logger.WarnLog("[Proxy] Failed to get token count for proxy %s: %v", proxyID, err)
	}

	// Delete proxy (will cascade by setting proxy_id to NULL for linked tokens)
	if err := store.DeleteProxy(proxyID); err != nil {
		WriteError(w, http.StatusInternalServerError, "delete_failed", err.Error())
		return
	}

	s.logger.InfoLog("[Proxy] Deleted proxy: %s (%s://%s:%d) - unlinked %d tokens",
		proxyID, proxy.Type, proxy.Host, proxy.Port, tokenCount)

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"message": fmt.Sprintf("Proxy deleted successfully. %d tokens unlinked.", tokenCount),
	})
}

// handleGetProxyTokens handles GET /api/proxies/{id}/tokens
func (s *Server) handleGetProxyTokens(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Only GET method is allowed")
		return
	}

	// Extract proxy ID from path
	proxyID := extractProxyID(r.URL.Path)
	if proxyID == "" {
		WriteError(w, http.StatusBadRequest, "invalid_request", "Proxy ID is required")
		return
	}

	// Get a SQLiteStore
	store, err := s.getSQLiteStore("qwen")
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "store_error", err.Error())
		return
	}

	// Check if proxy exists
	_, err = store.GetProxy(proxyID)
	if err != nil {
		WriteError(w, http.StatusNotFound, "not_found", err.Error())
		return
	}

	// Get linked tokens
	tokens, err := store.GetTokensByProxy(proxyID)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "get_tokens_failed", err.Error())
		return
	}

	// Convert to token info
	tokenInfos := make([]ProviderTokenInfo, 0, len(tokens))
	for _, token := range tokens {
		tokenInfos = append(tokenInfos, s.providerTokenToInfo(token))
	}

	WriteJSON(w, http.StatusOK, ProxyTokensResponse{
		ProxyID: proxyID,
		Tokens:  tokenInfos,
	})
}

// handleProxy handles all /api/proxies/ routes
func (s *Server) handleProxy(w http.ResponseWriter, r *http.Request) {
	// Check if this is a tokens request
	if strings.HasSuffix(r.URL.Path, "/tokens") {
		s.handleGetProxyTokens(w, r)
		return
	}

	// Extract proxy ID from path
	proxyID := extractProxyID(r.URL.Path)

	// If no proxy ID, return 404
	if proxyID == "" {
		WriteError(w, http.StatusNotFound, "not_found", "Proxy ID is required")
		return
	}

	// Route based on method
	switch r.Method {
	case http.MethodGet:
		s.handleGetProxy(w, r)
	case http.MethodPut:
		s.handleUpdateProxy(w, r)
	case http.MethodDelete:
		s.handleDeleteProxy(w, r)
	default:
		WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed")
	}
}

// extractProxyID extracts the proxy ID from the URL path
func extractProxyID(path string) string {
	// Path format: /api/proxies/{id} or /api/proxies/{id}/tokens
	parts := splitPath(path)
	if len(parts) >= 4 && parts[2] == "proxies" {
		return parts[3]
	}
	return ""
}

// splitPath splits a URL path into components
func splitPath(path string) []string {
	parts := []string{}
	current := ""
	for _, ch := range path {
		if ch == '/' {
			if current != "" {
				parts = append(parts, current)
				current = ""
			}
		} else {
			current += string(ch)
		}
	}
	if current != "" {
		parts = append(parts, current)
	}
	return parts
}
