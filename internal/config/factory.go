// Package config provides factory interfaces for HTTP client creation
package config

import (
	"net/http"
	"time"

	"github.com/sunbankio/qwencoder-proxy/internal/logging"
	auth "github.com/sunbankio/qwencoder-proxy/internal/token"
)

// HTTPClientFactory defines the interface for creating HTTP clients
// This interface follows the Dependency Inversion Principle
// It allows swapping implementations for testing and different proxy strategies
type HTTPClientFactory interface {
	// GetClient returns an HTTP client for the given proxy configuration
	// If proxyConfig is nil, returns a default client without proxy
	GetClient(proxyConfig *auth.ProxyConfig) *http.Client

	// GetDefaultClient returns a default HTTP client (no proxy)
	GetDefaultClient() *http.Client

	// GetClientWithTimeout returns an HTTP client with specific timeout
	GetClientWithTimeout(timeout time.Duration) *http.Client
}

// StandardHTTPClientFactory implements HTTPClientFactory
// This follows the Factory pattern for consistent HTTP client creation
// It wraps the existing ProxyAwareHTTPClientFactory
type StandardHTTPClientFactory struct {
	proxyFactory *ProxyAwareHTTPClientFactory
	logger       logging.Logger
	baseConfig   HTTPClientConfig
}

// NewStandardHTTPClientFactory creates a new standard HTTP client factory
// Returns a factory initialized with the provided configuration
func NewStandardHTTPClientFactory(baseConfig HTTPClientConfig, logger logging.Logger) *StandardHTTPClientFactory {
	return &StandardHTTPClientFactory{
		proxyFactory: NewProxyAwareHTTPClientFactory(baseConfig, logger, 50),
		logger:       logger,
		baseConfig:   baseConfig,
	}
}

// GetClient returns an HTTP client for the given proxy configuration
// Delegates to the underlying ProxyAwareHTTPClientFactory
func (f *StandardHTTPClientFactory) GetClient(proxyConfig *auth.ProxyConfig) *http.Client {
	return f.proxyFactory.GetClient(proxyConfig)
}

// GetDefaultClient returns a default HTTP client (no proxy)
// Delegates to the underlying ProxyAwareHTTPClientFactory with nil config
func (f *StandardHTTPClientFactory) GetDefaultClient() *http.Client {
	return f.GetClient(nil)
}

// GetClientWithTimeout returns an HTTP client with specific timeout
// Creates a new client with the specified timeout (not cached)
func (f *StandardHTTPClientFactory) GetClientWithTimeout(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout}
}

// DefaultTimeouts defines standard timeout values for different use cases
// This centralizes timeout configuration across the codebase
type DefaultTimeouts struct {
	ProviderTimeout      time.Duration // 5 minutes for provider requests
	AuthenticatorTimeout time.Duration // 30 seconds for auth requests
	StreamingTimeout     time.Duration // 15 minutes for streaming
	RequestTimeout       time.Duration // 45 seconds for standard requests
}

// GetDefaultTimeouts returns standard timeout values
func GetDefaultTimeouts() DefaultTimeouts {
	return DefaultTimeouts{
		ProviderTimeout:      5 * time.Minute,
		AuthenticatorTimeout: 30 * time.Second,
		StreamingTimeout:     15 * time.Minute,
		RequestTimeout:       45 * time.Second,
	}
}
