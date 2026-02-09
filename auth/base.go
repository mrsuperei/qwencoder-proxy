package auth

import (
	"net/http"
	"sync"
	"time"

	"github.com/sunbankio/qwencoder-proxy/logging"
)

// BaseAuthenticator provides common behavior for authenticator implementations.
// Thread-safe using sync.RWMutex for concurrent access protection.
type BaseAuthenticator struct {
	tokenManager  *TokenManager
	multiTokenMgr *MultiTokenManager
	mu            sync.RWMutex
	logger        logging.Logger
	httpClient    *http.Client
	clientFactory HTTPClientFactory // Optional factory for proxy-aware HTTP clients
}

// NewBaseAuthenticator creates a new base authenticator with default HTTP client (30-second timeout).
// This constructor maintains backward compatibility.
func NewBaseAuthenticator(logger logging.Logger) *BaseAuthenticator {
	return &BaseAuthenticator{
		logger: logger,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// NewBaseAuthenticatorWithFactory creates a new base authenticator with HTTPClientFactory.
// This enables proxy-aware authenticators for the proxy-per-token feature.
//
// Parameters:
//   - logger: Logger for authenticator operations
//   - clientFactory: Factory for creating HTTP clients with proxy support
//
// Returns:
//   - A new BaseAuthenticator instance configured with the HTTP client factory
func NewBaseAuthenticatorWithFactory(logger logging.Logger, clientFactory HTTPClientFactory) *BaseAuthenticator {
	return &BaseAuthenticator{
		logger:        logger,
		clientFactory: clientFactory,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// SetTokenManager sets the token manager. Thread-safe.
func (b *BaseAuthenticator) SetTokenManager(tokenManager *TokenManager) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.tokenManager = tokenManager
}

// SetMultiTokenManager sets the multi-token manager. Thread-safe.
func (b *BaseAuthenticator) SetMultiTokenManager(mtm *MultiTokenManager) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.multiTokenMgr = mtm
}

// GetTokenManager returns the token manager. Thread-safe.
func (b *BaseAuthenticator) GetTokenManager() *TokenManager {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.tokenManager
}

// GetMultiTokenManager returns the multi-token manager. Thread-safe.
func (b *BaseAuthenticator) GetMultiTokenManager() *MultiTokenManager {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.multiTokenMgr
}

// GetLogger returns the logger. Immutable, no mutex needed.
func (b *BaseAuthenticator) GetLogger() logging.Logger {
	return b.logger
}

// GetHTTPClient returns the HTTP client. Immutable, no mutex needed.
func (b *BaseAuthenticator) GetHTTPClient() *http.Client {
	return b.httpClient
}

// GetClientWithProxy returns an HTTP client configured with proxy settings.
// This method supports the proxy-per-token feature by using the HTTPClientFactory
// to create clients with appropriate proxy configurations.
//
// Parameters:
//   - proxyConfig: Optional proxy configuration. If nil, returns the default client.
//
// Returns:
//   - An HTTP client configured with the specified proxy settings, or the default
//     client if no factory is available or proxyConfig is nil.
//
// Usage:
//   - When a clientFactory is set, this method returns a proxy-aware client
//   - When no clientFactory is set, falls back to the default httpClient
//   - This enables authenticators to use token-specific proxy configurations
func (b *BaseAuthenticator) GetClientWithProxy(proxyConfig *ProxyConfig) *http.Client {
	if b.clientFactory != nil {
		return b.clientFactory.GetClient(proxyConfig)
	}
	return b.httpClient
}
