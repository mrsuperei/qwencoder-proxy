// Package qwen provides the Qwen provider implementation
package qwen

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strings"
	"time"

	tokenpkg "github.com/sunbankio/qwencoder-proxy/internal/token"
	"github.com/sunbankio/qwencoder-proxy/logging"
	"github.com/sunbankio/qwencoder-proxy/provider"
)

const (
	// DefaultBaseURL is the default Qwen API base URL
	DefaultBaseURL = "https://portal.qwen.ai/v1"
)

// SupportedModels lists all supported Qwen models
var SupportedModels = []string{
	"qwen3-coder-plus",
	"qwen3-coder-flash",
}

// Provider implements the provider.Provider interface for Qwen
// Embeds BaseProvider for common functionality
type Provider struct {
	*provider.BaseProvider // Embedded base provider provides common fields and methods
	authenticator          *QwenAuthenticator
}

// QwenAuthenticator wraps the multi-token manager for token selection
type QwenAuthenticator struct {
	tokenManager  *tokenpkg.TokenManager
	multiTokenMgr *tokenpkg.MultiTokenManager
	logger        logging.Logger
}

// NewQwenAuthenticator creates a new Qwen authenticator with token manager
func NewQwenAuthenticator(tokenManager *tokenpkg.TokenManager, logger logging.Logger) *QwenAuthenticator {
	return &QwenAuthenticator{
		tokenManager:  tokenManager,
		multiTokenMgr: nil,
		logger:        logger,
	}
}

// NewQwenAuthenticatorWithMultiTokenManager creates a new Qwen authenticator with both token manager and multi-token manager
func NewQwenAuthenticatorWithMultiTokenManager(tokenManager *tokenpkg.TokenManager, multiTokenMgr *tokenpkg.MultiTokenManager, logger logging.Logger) *QwenAuthenticator {
	return &QwenAuthenticator{
		tokenManager:  tokenManager,
		multiTokenMgr: multiTokenMgr,
		logger:        logger,
	}
}

// SetMultiTokenManager sets the multi-token manager
func (a *QwenAuthenticator) SetMultiTokenManager(multiTokenMgr *tokenpkg.MultiTokenManager) {
	a.multiTokenMgr = multiTokenMgr
}

// Authenticate performs the authentication flow
func (a *QwenAuthenticator) Authenticate(ctx context.Context) error {
	return AuthenticateWithDeviceFlow(ctx, a.logger, a.multiTokenMgr)
}

// GetToken returns a valid access token
func (a *QwenAuthenticator) GetToken(ctx context.Context) (string, error) {
	if a.tokenManager == nil {
		return "", errors.New("token manager not initialized")
	}

	token, err := a.tokenManager.SelectToken()
	if err != nil {
		// If we get an auth error, try to trigger the authentication flow
		if strings.Contains(err.Error(), "no tokens available") {
			// Trigger authentication flow to get new credentials
			authErr := AuthenticateWithDeviceFlow(ctx, a.logger, a.multiTokenMgr)
			if authErr != nil {
				return "", fmt.Errorf("authentication required but failed: %v. Error getting token: %w", authErr, err)
			}
			// Try again after authentication
			token, err = a.tokenManager.SelectToken()
			if err != nil {
				return "", err
			}
			return token.AccessToken, nil
		}
		return "", err
	}
	return token.AccessToken, nil
}

// IsAuthenticated checks if valid credentials exist
func (a *QwenAuthenticator) IsAuthenticated() bool {
	if a.tokenManager == nil {
		return false
	}

	_, err := a.tokenManager.SelectToken()
	return err == nil
}

// GetCredentialsPath returns the path to stored credentials
func (a *QwenAuthenticator) GetCredentialsPath() string {
	// Multi-token system uses different storage path
	return ""
}

// ClearCredentials removes stored credentials
func (a *QwenAuthenticator) ClearCredentials() error {
	// Use the credentials path to remove the file
	credsPath := a.GetCredentialsPath()
	return os.Remove(credsPath)
}

// GetTokenWithClient returns a valid access token and an HTTP client.
// The HTTP client is configured with the proxy settings from the selected token.
func (a *QwenAuthenticator) GetTokenWithClient(ctx context.Context) (string, *http.Client, error) {
	if a.tokenManager == nil {
		return "", nil, errors.New("token manager not initialized")
	}

	token, client, err := a.tokenManager.SelectTokenWithClient()
	if err != nil {
		return "", nil, fmt.Errorf("failed to select token with client: %w", err)
	}
	return token.AccessToken, client, nil
}

// GetHTTPClient returns an HTTP client configured with proxy settings.
// The client is configured with the proxy settings from the selected token.
func (a *QwenAuthenticator) GetHTTPClient() (*http.Client, error) {
	if a.tokenManager == nil {
		return nil, errors.New("token manager not initialized")
	}

	token, client, err := a.tokenManager.SelectTokenWithClient()
	if err != nil {
		return nil, fmt.Errorf("failed to select token with client: %w", err)
	}
	_ = token // Token is not needed for GetHTTPClient
	return client, nil
}

// NewProvider creates a new Qwen provider
// Uses BaseProvider for common functionality
func NewProvider() *Provider {
	return &Provider{
		BaseProvider:  provider.NewBaseProvider(logging.NewLogger(), 5*time.Minute),
		authenticator: NewQwenAuthenticator(nil, logging.NewLogger()),
	}
}

// NewProviderWithTokenManager creates a new Qwen provider with token manager
// Uses BaseProvider for common functionality
func NewProviderWithTokenManager(tokenManager *tokenpkg.TokenManager, logger logging.Logger) *Provider {
	return &Provider{
		BaseProvider:  provider.NewBaseProvider(logger, 5*time.Minute),
		authenticator: NewQwenAuthenticator(tokenManager, logger),
	}
}

// Name returns the provider identifier
func (p *Provider) Name() provider.ProviderType {
	return provider.ProviderQwen
}

// Protocol returns the native protocol
func (p *Provider) Protocol() provider.ProtocolType {
	return provider.ProtocolQwen
}

// SupportedModels returns list of supported model IDs
func (p *Provider) SupportedModels() []string {
	return SupportedModels
}

// SupportsModel checks if the provider supports the given model
func (p *Provider) SupportsModel(model string) bool {
	modelLower := strings.ToLower(model)
	if strings.HasPrefix(modelLower, "qwen") {
		return true
	}
	for _, m := range SupportedModels {
		if strings.EqualFold(m, model) {
			return true
		}
	}
	return false
}

// GetAuthenticator returns the auth handler for this provider
func (p *Provider) GetAuthenticator() provider.Authenticator {
	return p.authenticator
}

// IsHealthy checks if the provider is available
func (p *Provider) IsHealthy(ctx context.Context) bool {
	// Try to list models as a health check
	_, err := p.ListModels(ctx)
	return err == nil
}

// ListModels returns available models in native format
func (p *Provider) ListModels(ctx context.Context) (interface{}, error) {
	// For now, return static list since Qwen doesn't have a public models endpoint
	models := make([]interface{}, len(SupportedModels))
	for i, model := range SupportedModels {
		models[i] = map[string]interface{}{
			"id":       model,
			"object":   "model",
			"created":  1677648736,
			"owned_by": "qwen",
		}
	}
	return map[string]interface{}{
		"object": "list",
		"data":   models,
	}, nil
}

// isProxyError checks if an error is a proxy-related error
// This includes connection errors, timeout errors, and DNS resolution errors
func (p *Provider) isProxyError(err error) bool {
	if err == nil {
		return false
	}

	// Check for timeout errors
	if netErr, ok := err.(net.Error); ok {
		if netErr.Timeout() {
			return true
		}
	}

	// Check for connection errors
	if strings.Contains(err.Error(), "connection refused") ||
		strings.Contains(err.Error(), "connection reset") ||
		strings.Contains(err.Error(), "broken pipe") ||
		strings.Contains(err.Error(), "EOF") {
		return true
	}

	// Check for DNS resolution errors
	if strings.Contains(err.Error(), "no such host") ||
		strings.Contains(err.Error(), "dns") ||
		strings.Contains(err.Error(), "lookup") {
		return true
	}

	// Check for proxy-specific errors
	if strings.Contains(err.Error(), "proxy") ||
		strings.Contains(err.Error(), "SOCKS") ||
		strings.Contains(err.Error(), "tunnel") {
		return true
	}

	// Check for URL errors (often related to proxy configuration)
	if urlErr, ok := err.(*url.Error); ok {
		return p.isProxyError(urlErr.Err)
	}

	return false
}

// classifyProxyError returns a string classification of the proxy error type
// This is used for health tracking and logging purposes
func (p *Provider) classifyProxyError(err error) string {
	if err == nil {
		return "none"
	}

	// Check for timeout errors
	if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
		return "timeout"
	}

	// Check for connection errors
	if strings.Contains(err.Error(), "connection refused") {
		return "connection_refused"
	}
	if strings.Contains(err.Error(), "connection reset") {
		return "connection_reset"
	}
	if strings.Contains(err.Error(), "broken pipe") {
		return "broken_pipe"
	}
	if strings.Contains(err.Error(), "EOF") {
		return "eof"
	}

	// Check for DNS resolution errors
	if strings.Contains(err.Error(), "no such host") ||
		strings.Contains(err.Error(), "lookup") {
		return "dns_resolution"
	}

	// Check for proxy-specific errors
	if strings.Contains(err.Error(), "SOCKS") {
		return "socks_error"
	}
	if strings.Contains(err.Error(), "proxy") {
		return "proxy_error"
	}
	if strings.Contains(err.Error(), "tunnel") {
		return "tunnel_error"
	}

	// Check for URL errors
	if urlErr, ok := err.(*url.Error); ok {
		return "url_error: " + p.classifyProxyError(urlErr.Err)
	}

	return "unknown"
}

// doRequestWithProxy executes an HTTP request using the provided client
// and handles proxy error classification and health tracking
func (p *Provider) doRequestWithProxy(req *http.Request, client *http.Client, tokenID string) (*http.Response, error) {
	// Log request headers before sending
	p.GetLogger().DebugLog("[Qwen] Request URL: %s", req.URL.String())
	p.GetLogger().DebugLog("[Qwen] Request Method: %s", req.Method)
	p.GetLogger().DebugLog("[Qwen] Request Headers: %v", req.Header)

	resp, err := client.Do(req)
	if err != nil {
		// Check if this is a proxy error
		if p.isProxyError(err) {
			errorType := p.classifyProxyError(err)
			p.GetLogger().ErrorLog("[Qwen] Proxy error for token %s: %s (%s)", tokenID, err.Error(), errorType)

			// Update proxy health if tokenManager is available
			if p.GetTokenManager() != nil {
				if updateErr := p.GetTokenManager().UpdateProxyHealth(tokenID, false, err); updateErr != nil {
					p.GetLogger().WarnLog("[Qwen] Failed to update proxy health for token %s: %v", tokenID, updateErr)
				}
			}
		} else {
			p.GetLogger().ErrorLog("[Qwen] Request error for token %s: %v", tokenID, err)
		}
		return nil, err
	}

	// Log response headers after receiving
	p.GetLogger().DebugLog("[Qwen] Response Status: %s", resp.Status)
	p.GetLogger().DebugLog("[Qwen] Response Headers: %v", resp.Header)

	// Update proxy health on success
	if p.GetTokenManager() != nil {
		if updateErr := p.GetTokenManager().UpdateProxyHealth(tokenID, true, nil); updateErr != nil {
			p.GetLogger().WarnLog("[Qwen] Failed to update proxy health for token %s: %v", tokenID, updateErr)
		}
	}

	return resp, nil
}

// GenerateContent handles non-streaming requests with native format
func (p *Provider) GenerateContent(ctx context.Context, model string, request interface{}) (interface{}, error) {
	// Determine which client and token to use
	var client *http.Client
	var token string
	var tokenID string

	// Try to use token manager for proxy-aware client selection
	if p.GetTokenManager() != nil {
		selectedToken, selectedClient, selectErr := p.GetTokenManager().SelectTokenWithClient()
		if selectErr != nil {
			p.GetLogger().ErrorLog("[Qwen] Token selection failed: %v", selectErr)
			return nil, fmt.Errorf("failed to select token: %w", selectErr)
		}
		if selectedClient == nil {
			p.GetLogger().DebugLog("[Qwen] Token manager returned nil client, using default HTTP client")
			client = p.GetHTTPClient()
		} else {
			client = selectedClient
		}
		token = selectedToken.AccessToken
		tokenID = selectedToken.ID

		// Log proxy usage
		if selectedToken.Proxy != nil {
			p.GetLogger().DebugLog("[Qwen] GenerateContent using token %s with proxy: %s:%d", tokenID, selectedToken.Proxy.Host, selectedToken.Proxy.Port)
		} else {
			p.GetLogger().DebugLog("[Qwen] GenerateContent using token %s with direct connection", tokenID)
		}
	} else {
		return nil, errors.New("token manager not initialized")
	}

	// Log the original request before conversion
	p.GetLogger().DebugLog("[Qwen] Original request before conversion: %+v", request)

	// Convert request to proper format for Qwen API
	reqBody, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	p.GetLogger().DebugLog("[Qwen] Request body being sent: %s", string(reqBody))

	// Use the default endpoint from qwen package
	endpoint := DefaultBaseURL

	// Since this is called from the OpenAI handler for /v1/chat/completions,
	// we construct the appropriate path
	url := fmt.Sprintf("%s/chat/completions", endpoint)
	p.GetLogger().DebugLog("[Qwen] Constructed target URL: %s", url)

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set the required headers for Qwen API (following reference implementation)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-DashScope-AuthType", "qwen-oauth")
	req.Header.Set("X-DashScope-UserAgent", fmt.Sprintf("QwenCode/0.0.10 (%s; %s)", runtime.GOOS, runtime.GOARCH))
	req.Header.Set("X-DashScope-CacheControl", "enable")

	p.GetLogger().DebugLog("[Qwen] Sending request to %s with headers: %v", url, req.Header)

	resp, err := p.doRequestWithProxy(req, client, tokenID)
	if err != nil {
		p.GetLogger().ErrorLog("[Qwen] Failed to send request: %v", err)
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	p.GetLogger().DebugLog("[Qwen] Response status: %d", resp.StatusCode)

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		p.GetLogger().ErrorLog("[Qwen] API error (status %d): %s", resp.StatusCode, string(body))
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}

	var qwenResp map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&qwenResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	p.GetLogger().DebugLog("[Qwen] Received response: %+v", qwenResp)
	return qwenResp, nil
}

// GenerateContentStream handles streaming requests with native format
func (p *Provider) GenerateContentStream(ctx context.Context, model string, request interface{}) (io.ReadCloser, error) {
	// Determine which client and token to use
	var client *http.Client
	var token string
	var tokenID string

	// Try to use token manager for proxy-aware client selection
	if p.GetTokenManager() != nil {
		selectedToken, selectedClient, selectErr := p.GetTokenManager().SelectTokenWithClient()
		if selectErr != nil {
			p.GetLogger().ErrorLog("[Qwen] Token selection failed: %v", selectErr)
			return nil, fmt.Errorf("failed to select token: %w", selectErr)
		}
		if selectedClient == nil {
			p.GetLogger().DebugLog("[Qwen] Token manager returned nil client, using default HTTP client")
			client = p.GetHTTPClient()
		} else {
			client = selectedClient
		}
		token = selectedToken.AccessToken
		tokenID = selectedToken.ID

		// Log proxy usage
		if selectedToken.Proxy != nil {
			p.GetLogger().DebugLog("[Qwen] GenerateContentStream using token %s with proxy: %s:%d", tokenID, selectedToken.Proxy.Host, selectedToken.Proxy.Port)
		} else {
			p.GetLogger().DebugLog("[Qwen] GenerateContentStream using token %s with direct connection", tokenID)
		}
	} else {
		return nil, errors.New("token manager not initialized")
	}

	// Convert request to proper format for Qwen API
	reqBody, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Use the default endpoint from qwen package
	endpoint := DefaultBaseURL
	url := fmt.Sprintf("%s/chat/completions", endpoint)
	p.GetLogger().DebugLog("[Qwen] Constructed streaming target URL: %s", url)

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set the required headers for Qwen API
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("X-DashScope-AuthType", "qwen-oauth")
	req.Header.Set("X-DashScope-UserAgent", fmt.Sprintf("QwenCode/0.0.10 (%s; %s)", runtime.GOOS, runtime.GOARCH))
	req.Header.Set("X-DashScope-CacheControl", "enable")

	p.GetLogger().DebugLog("[Qwen] Sending streaming request to %s", url)

	resp, err := p.doRequestWithProxy(req, client, tokenID)
	if err != nil {
		p.GetLogger().ErrorLog("[Qwen] Failed to send streaming request: %v", err)
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	p.GetLogger().DebugLog("[Qwen] Streaming response status: %d", resp.StatusCode)

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		p.GetLogger().ErrorLog("[Qwen] Streaming API error (status %d): %s", resp.StatusCode, string(body))
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}

	return resp.Body, nil
}
