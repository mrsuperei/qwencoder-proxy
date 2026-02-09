// Package auth provides factory for creating authenticators
package auth

import (
	"context"
	"net/http"
	"time"

	"github.com/sunbankio/qwencoder-proxy/logging"
)

// Authenticator defines the interface for provider authentication
// This interface is defined in auth to avoid circular dependency with provider package
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

// HTTPClientFactory defines the interface for creating HTTP clients
// This interface is defined in auth to avoid circular dependency with config package
type HTTPClientFactory interface {
	// GetClient returns an HTTP client for the given proxy configuration
	// If proxyConfig is nil, returns a default client without proxy
	GetClient(proxyConfig *ProxyConfig) *http.Client

	// GetDefaultClient returns a default HTTP client (no proxy)
	GetDefaultClient() *http.Client

	// GetClientWithTimeout returns an HTTP client with specific timeout
	GetClientWithTimeout(timeout time.Duration) *http.Client
}

// AuthenticatorFactory creates authenticators for different providers
// This follows the Factory pattern and Abstract Factory pattern
// It enables dependency injection and consistent authenticator creation
type AuthenticatorFactory struct {
	logger        logging.Logger
	clientFactory HTTPClientFactory
}

// NewAuthenticatorFactory creates a new authenticator factory
// Returns a factory initialized with the provided dependencies
func NewAuthenticatorFactory(logger logging.Logger, clientFactory HTTPClientFactory) *AuthenticatorFactory {
	return &AuthenticatorFactory{
		logger:        logger,
		clientFactory: clientFactory,
	}
}

// CreateGeminiAuthenticator creates a new Gemini authenticator
// Uses the existing NewGeminiAuthenticator constructor
// Returns a GeminiAuthenticator with default config if config is nil
// Note: In Phase3, this will be refactored to embed BaseAuthenticator
func (f *AuthenticatorFactory) CreateGeminiAuthenticator(config *GeminiOAuthConfig) *GeminiAuthenticator {
	if config == nil {
		config = DefaultGeminiOAuthConfig()
	}
	return NewGeminiAuthenticator(config)
}

// CreateKiroAuthenticator creates a new Kiro authenticator
// Uses the existing NewKiroAuthenticator constructor
// Returns a KiroAuthenticator with default config if config is nil
// Note: In Phase3, this will be refactored to embed BaseAuthenticator
func (f *AuthenticatorFactory) CreateKiroAuthenticator(config *KiroOAuthConfig) *KiroAuthenticator {
	if config == nil {
		config = DefaultKiroOAuthConfig()
	}
	return NewKiroAuthenticator(config)
}

// CreateIFlowAuthenticator creates a new iFlow authenticator
// Uses the existing NewIFlowAuthenticator constructor
// Returns an IFlowAuthenticator with default config if config is nil
// Note: In Phase3, this will be refactored to embed BaseAuthenticator
func (f *AuthenticatorFactory) CreateIFlowAuthenticator(config *IFlowOAuthConfig) *IFlowAuthenticator {
	if config == nil {
		config = DefaultIFlowOAuthConfig()
	}
	return NewIFlowAuthenticator(config)
}

// ProviderType represents the type of provider
// This enum is used for factory method selection
type ProviderType string

const (
	ProviderGemini      ProviderType = "gemini"
	ProviderKiro        ProviderType = "kiro"
	ProviderIFlow       ProviderType = "iflow"
	ProviderQwen        ProviderType = "qwen"
	ProviderAntigravity ProviderType = "antigravity"
)

// CreateAuthenticator creates an authenticator for the specified provider type
// This follows the Abstract Factory pattern
// Returns an Authenticator interface or nil if provider type is unknown
func (f *AuthenticatorFactory) CreateAuthenticator(providerType ProviderType, config interface{}) Authenticator {
	switch providerType {
	case ProviderGemini:
		return f.CreateGeminiAuthenticator(config.(*GeminiOAuthConfig))
	case ProviderKiro:
		return f.CreateKiroAuthenticator(config.(*KiroOAuthConfig))
	case ProviderIFlow:
		return f.CreateIFlowAuthenticator(config.(*IFlowOAuthConfig))
	default:
		f.logger.ErrorLog("[AuthenticatorFactory] Unknown provider type: %s", providerType)
		return nil
	}
}
