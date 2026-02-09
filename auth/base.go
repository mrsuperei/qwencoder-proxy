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
