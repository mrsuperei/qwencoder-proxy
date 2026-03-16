// Package provider defines the interface and types for LLM providers
package provider

import (
	"context"
	"io"
	"net/http"

	auth "github.com/sunbankio/qwencoder-proxy/internal/token"
)

// ProviderType identifies the provider
type ProviderType string

const (
	ProviderQwen        ProviderType = "qwen"
	ProviderGeminiCLI   ProviderType = "gemini-cli"
	ProviderKiro        ProviderType = "kiro"
	ProviderAntigravity ProviderType = "antigravity"
	ProviderIFlow       ProviderType = "iflow"
)

// ProtocolType identifies the native API protocol
type ProtocolType string

const (
	ProtocolOpenAI ProtocolType = "openai"
	ProtocolGemini ProtocolType = "gemini"
	ProtocolClaude ProtocolType = "claude"
	ProtocolQwen   ProtocolType = "qwen"
)

// Provider defines the interface for LLM providers
type Provider interface {
	// Name returns the provider identifier
	Name() ProviderType

	// Protocol returns the native protocol (openai, gemini, claude)
	Protocol() ProtocolType

	// SupportedModels returns list of supported model IDs
	SupportedModels() []string

	// SupportsModel checks if the provider supports the given model
	SupportsModel(model string) bool

	// GenerateContent handles non-streaming requests with native format
	// The request and response are in the provider's native format
	GenerateContent(ctx context.Context, model string, request interface{}) (interface{}, error)

	// GenerateContentStream handles streaming requests with native format
	// Returns a reader for SSE stream data
	GenerateContentStream(ctx context.Context, model string, request interface{}) (io.ReadCloser, error)

	// ListModels returns available models in native format
	ListModels(ctx context.Context) (interface{}, error)

	// GetAuthenticator returns the auth handler for this provider
	GetAuthenticator() Authenticator

	// IsHealthy checks if the provider is available
	IsHealthy(ctx context.Context) bool
}

// TokenManagerAware is an optional interface that providers can implement
// to receive a TokenManager for proxy-aware token selection.
// This interface is separate from Provider to maintain backward compatibility.
type TokenManagerAware interface {
	// SetTokenManager sets the TokenManager for this provider.
	// This enables proxy-aware token selection when making API requests.
	SetTokenManager(manager *auth.TokenManager)
}

// Authenticator defines the interface for provider authentication
type Authenticator interface {
	// Authenticate performs the authentication flow
	Authenticate(ctx context.Context) error

	// GetToken returns a valid access token, refreshing if necessary
	GetToken(ctx context.Context) (string, error)

	// GetTokenWithClient returns a valid access token and an HTTP client configured
	// with the token's proxy settings. The HTTP client may be nil if the authenticator
	// does not support proxy-aware clients. This method maintains backward compatibility
	// with GetToken() while enabling proxy-aware authentication operations.
	GetTokenWithClient(ctx context.Context) (string, *http.Client, error)

	// IsAuthenticated checks if valid credentials exist
	IsAuthenticated() bool

	// GetCredentialsPath returns the path to stored credentials
	GetCredentialsPath() string

	// ClearCredentials removes stored credentials
	ClearCredentials() error

	// GetHTTPClient returns an HTTP client configured with the token's proxy settings.
	// This is an optional method - authenticators that do not support proxy-aware clients
	// should return an error indicating the method is not implemented.
	GetHTTPClient() (*http.Client, error)
}

// Model represents a model in the provider's catalog
type Model struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Provider    string `json:"provider,omitempty"`
}

// ChatRequest represents a chat completion request
type ChatRequest struct {
	Prompt      string  `json:"prompt"`
	System      string  `json:"system,omitempty"`
	Temperature float64 `json:"temperature,omitempty"`
}

// ChatResponse represents a chat completion response
type ChatResponse struct {
	Content string `json:"content"`
	Done    bool   `json:"done"`
}

// ChatChunk represents a streaming chat chunk
type ChatChunk struct {
	Content string `json:"content"`
	Done    bool   `json:"done"`
}
