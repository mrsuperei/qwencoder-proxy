// Package iflow provides the iFlow provider implementation
package iflow

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/sunbankio/qwencoder-proxy/auth"
	"github.com/sunbankio/qwencoder-proxy/logging"
	"github.com/sunbankio/qwencoder-proxy/provider"
)

const (
	// OAuth constants from reference/iflow.rs
	AuthURL      = "https://iflow.cn/oauth"
	TokenURL     = "https://iflow.cn/oauth/token"
	UserInfoURL  = "https://iflow.cn/api/oauth/getUserInfo"
	APIKeyURL    = "https://platform.iflow.cn/api/openapi/apikey"
	ClientID     = "10009311001"
	ClientSecret = "4Z3YjXycVsQvyGF1etiNlIBB4RsqSDtW"
	DefaultPort  = 11451
	APIBaseURL   = "https://apis.iflow.cn/v1"
)

// SupportedModels lists all supported iFlow models
var SupportedModels = []string{
	"glm-4.6",
	"qwen3-coder-plus",
	"qwen3-max",
	"deepseek-v3.2",
	"deepseek-r1",
	"qwen3-vl-plus",
	"kimi-k2",
	"kimi-k2-0905",
}

// Provider implements the provider.Provider interface for iFlow
// Embeds BaseProvider for common functionality
type Provider struct {
	*provider.BaseProvider // Embedded base provider provides common fields and methods
	baseURL                string
	authenticator          *auth.IFlowAuthenticator
}

// NewProvider creates a new iFlow provider
// Uses BaseProvider for common functionality
func NewProvider(authenticator *auth.IFlowAuthenticator) *Provider {
	if authenticator == nil {
		// Use direct instantiation for now - will be updated to use factory in later phase
		authenticator = auth.NewIFlowAuthenticator(nil)
	}
	return &Provider{
		BaseProvider:  provider.NewBaseProvider(logging.NewLogger(), 5*time.Minute),
		baseURL:       APIBaseURL,
		authenticator: authenticator,
	}
}

// Name returns the provider identifier
func (p *Provider) Name() provider.ProviderType {
	return "iflow"
}

// Protocol returns the native protocol
func (p *Provider) Protocol() provider.ProtocolType {
	return provider.ProtocolOpenAI
}

// SupportedModels returns list of supported model IDs
func (p *Provider) SupportedModels() []string {
	return SupportedModels
}

// SupportsModel checks if the provider supports the given model
func (p *Provider) SupportsModel(model string) bool {
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

// ListModels returns available models in OpenAI format
func (p *Provider) ListModels(ctx context.Context) (interface{}, error) {
	// iFlow doesn't support /models endpoint, return hardcoded models
	modelsResp := OpenAIModelsResponse{
		Object: "list",
		Data:   []OpenAIModel{},
	}

	for _, modelID := range SupportedModels {
		modelsResp.Data = append(modelsResp.Data, OpenAIModel{
			ID:      modelID,
			Object:  "model",
			Created: time.Now().Unix(),
			OwnedBy: "iflow",
		})
	}

	return &modelsResp, nil
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
	if strings.Contains(err.Error(), "proxy") {
		return "proxy_error"
	}
	if strings.Contains(err.Error(), "SOCKS") {
		return "socks_error"
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
	resp, err := client.Do(req)
	if err != nil {
		// Check if this is a proxy error
		if p.isProxyError(err) {
			errorType := p.classifyProxyError(err)
			p.GetLogger().ErrorLog("[iFlow] Proxy error for token %s: %s (%s)", tokenID, err.Error(), errorType)

			// Update proxy health if tokenManager is available
			if p.GetTokenManager() != nil {
				if updateErr := p.GetTokenManager().UpdateProxyHealth(tokenID, false, err); updateErr != nil {
					p.GetLogger().WarnLog("[iFlow] Failed to update proxy health for token %s: %v", tokenID, updateErr)
				}
			}
		} else {
			p.GetLogger().ErrorLog("[iFlow] Request error for token %s: %v", tokenID, err)
		}
		return nil, err
	}

	// Update proxy health on success
	if p.GetTokenManager() != nil {
		if updateErr := p.GetTokenManager().UpdateProxyHealth(tokenID, true, nil); updateErr != nil {
			p.GetLogger().WarnLog("[iFlow] Failed to update proxy health for token %s: %v", tokenID, updateErr)
		}
	}

	return resp, nil
}

// GenerateContent handles non-streaming requests with OpenAI format
func (p *Provider) GenerateContent(ctx context.Context, model string, request interface{}) (interface{}, error) {
	// Get token and proxy-aware client
	var client *http.Client
	var token string
	var tokenID string

	// Try to use token manager for proxy-aware client selection
	if p.GetTokenManager() != nil {
		selectedToken, selectedClient, selectErr := p.GetTokenManager().SelectTokenWithClient()
		if selectErr != nil {
			p.GetLogger().ErrorLog("[iFlow] Token selection failed: %v", selectErr)
			return nil, fmt.Errorf("failed to select token: %w", selectErr)
		}
		if selectedClient == nil {
			p.GetLogger().DebugLog("[iFlow] Token manager returned nil client, using default HTTP client")
			client = p.GetHTTPClient()
		} else {
			client = selectedClient
		}
		token = selectedToken.AccessToken
		tokenID = selectedToken.ID

		// Log proxy usage
		if selectedToken.Proxy != nil && selectedToken.Proxy.Enabled {
			p.GetLogger().DebugLog("[iFlow] GenerateContent using token %s with proxy: %s:%d", tokenID, selectedToken.Proxy.Host, selectedToken.Proxy.Port)
		} else {
			p.GetLogger().DebugLog("[iFlow] GenerateContent using token %s with direct connection", tokenID)
		}
	} else {
		// No token manager, use authenticator and default client (backward compatibility)
		var authErr error
		token, authErr = p.authenticator.GetToken(ctx)
		if authErr != nil {
			p.GetLogger().ErrorLog("[iFlow] Token retrieval failed: %v", authErr)
			return nil, fmt.Errorf("failed to get token: %w", authErr)
		}
		client = p.GetHTTPClient()
		tokenID = "fallback"
	}

	tokenPrefix := token
	if len(token) > 20 {
		tokenPrefix = token[:20]
	}
	p.GetLogger().DebugLog("[iFlow] Using access token (first 20 chars): %s", tokenPrefix)

	// Marshal the request
	reqBody, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/chat/completions", p.baseURL)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "iflow-cli/0.4.8")

	p.GetLogger().DebugLog("[iFlow] Sending chat completions request to %s", url)

	resp, err := p.doRequestWithProxy(req, client, tokenID)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	p.GetLogger().DebugLog("[iFlow] Response status: %d", resp.StatusCode)
	p.GetLogger().DebugLog("[iFlow] Response headers: %v", resp.Header)

	if resp.StatusCode == http.StatusUnauthorized {
		// Try to refresh token and retry
		p.GetLogger().DebugLog("[iFlow] Received %d, attempting to refresh token and retry", resp.StatusCode)

		// Read and close the original response body
		bodyBytes, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		_, refreshErr := p.authenticator.GetToken(ctx)
		if refreshErr != nil {
			p.GetLogger().ErrorLog("[iFlow] Token refresh failed: %v", refreshErr)
			return nil, fmt.Errorf("unauthorized and token refresh failed: %s", string(bodyBytes))
		}

		// Retry with refreshed token
		refreshedToken, tokenErr := p.authenticator.GetToken(ctx)
		if tokenErr != nil {
			return nil, fmt.Errorf("failed to get refreshed token: %w", tokenErr)
		}
		req.Header.Set("Authorization", "Bearer "+refreshedToken)

		resp, err = p.doRequestWithProxy(req, client, tokenID)
		if err != nil {
			return nil, fmt.Errorf("failed to send retry request: %w", err)
		}
		defer resp.Body.Close()
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		p.GetLogger().DebugLog("[iFlow] API error response body: %s", string(body))
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	p.GetLogger().DebugLog("[iFlow] Raw response body: %s", string(respBody))

	var chatResp OpenAIChatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &chatResp, nil
}

// GenerateContentStream handles streaming requests with OpenAI format
func (p *Provider) GenerateContentStream(ctx context.Context, model string, request interface{}) (io.ReadCloser, error) {
	// Get token and proxy-aware client
	var client *http.Client
	var token string
	var tokenID string

	// Try to use token manager for proxy-aware client selection
	if p.GetTokenManager() != nil {
		selectedToken, selectedClient, selectErr := p.GetTokenManager().SelectTokenWithClient()
		if selectErr != nil {
			p.GetLogger().ErrorLog("[iFlow] Token selection failed: %v", selectErr)
			return nil, fmt.Errorf("failed to select token: %w", selectErr)
		}
		if selectedClient == nil {
			p.GetLogger().DebugLog("[iFlow] Token manager returned nil client, using default HTTP client")
			client = p.GetHTTPClient()
		} else {
			client = selectedClient
		}
		token = selectedToken.AccessToken
		tokenID = selectedToken.ID

		// Log proxy usage
		if selectedToken.Proxy != nil && selectedToken.Proxy.Enabled {
			p.GetLogger().DebugLog("[iFlow] GenerateContentStream using token %s with proxy: %s:%d", tokenID, selectedToken.Proxy.Host, selectedToken.Proxy.Port)
		} else {
			p.GetLogger().DebugLog("[iFlow] GenerateContentStream using token %s with direct connection", tokenID)
		}
	} else {
		// No token manager, use authenticator and default client (backward compatibility)
		var authErr error
		token, authErr = p.authenticator.GetToken(ctx)
		if authErr != nil {
			p.GetLogger().ErrorLog("[iFlow] Token retrieval failed: %v", authErr)
			return nil, fmt.Errorf("failed to get token: %w", authErr)
		}
		client = p.GetHTTPClient()
		tokenID = "fallback"
	}

	// Marshal the request
	reqBody, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/chat/completions", p.baseURL)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream, application/json")
	req.Header.Set("User-Agent", "qwencoder-proxy/1.0")

	p.GetLogger().DebugLog("[iFlow] Sending streaming chat completions request to %s", url)

	resp, err := p.doRequestWithProxy(req, client, tokenID)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	p.GetLogger().DebugLog("[iFlow] Streaming response status: %d", resp.StatusCode)
	p.GetLogger().DebugLog("[iFlow] Streaming response headers: %v", resp.Header)

	if resp.StatusCode == http.StatusUnauthorized {
		// Try to refresh token and retry
		p.GetLogger().DebugLog("[iFlow] Received %d, attempting to refresh token and retry", resp.StatusCode)

		// Read and close the original response body
		bodyBytes, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		_, refreshErr := p.authenticator.GetToken(ctx)
		if refreshErr != nil {
			p.GetLogger().ErrorLog("[iFlow] Token refresh failed: %v", refreshErr)
			return nil, fmt.Errorf("unauthorized and token refresh failed: %s", string(bodyBytes))
		}

		// Retry with refreshed token
		refreshedToken, tokenErr := p.authenticator.GetToken(ctx)
		if tokenErr != nil {
			return nil, fmt.Errorf("failed to get refreshed token: %w", tokenErr)
		}
		req.Header.Set("Authorization", "Bearer "+refreshedToken)

		resp, err = p.doRequestWithProxy(req, client, tokenID)
		if err != nil {
			return nil, fmt.Errorf("failed to send retry request: %w", err)
		}
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		p.GetLogger().DebugLog("[iFlow] Streaming API error response body: %s", string(body))
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}

	return resp.Body, nil
}

// OpenAIModelsResponse represents OpenAI models API response
type OpenAIModelsResponse struct {
	Object string        `json:"object"`
	Data   []OpenAIModel `json:"data"`
}

// OpenAIModel represents a model in OpenAI format
type OpenAIModel struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

// OpenAIChatResponse represents OpenAI chat completion response
type OpenAIChatResponse struct {
	ID      string         `json:"id"`
	Object  string         `json:"object"`
	Created int64          `json:"created"`
	Model   string         `json:"model"`
	Choices []OpenAIChoice `json:"choices"`
	Usage   OpenAIUsage    `json:"usage"`
}

// OpenAIChoice represents a choice in OpenAI response
type OpenAIChoice struct {
	Index        int           `json:"index"`
	Message      OpenAIMessage `json:"message"`
	FinishReason string        `json:"finish_reason"`
}

// OpenAIMessage represents a message in OpenAI format
type OpenAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// OpenAIUsage represents token usage in OpenAI format
type OpenAIUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}
