package provider

import (
	"net/http"
	"time"

	auth "github.com/sunbankio/qwencoder-proxy/internal/token"
	"github.com/sunbankio/qwencoder-proxy/logging"
)

// BaseProvider provides common behavior for provider implementations.
// Fields are immutable after construction, no mutex needed.
type BaseProvider struct {
	httpClient   *http.Client
	tokenManager *auth.TokenManager
	logger       logging.Logger
}

// NewBaseProvider creates a new base provider with configurable HTTP client timeout.
// Recommended default timeout: 5 minutes.
func NewBaseProvider(logger logging.Logger, timeout time.Duration) *BaseProvider {
	return &BaseProvider{
		logger: logger,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

// GetHTTPClient returns the HTTP client. Immutable, no mutex needed.
func (b *BaseProvider) GetHTTPClient() *http.Client {
	return b.httpClient
}

// GetLogger returns the logger. Immutable, no mutex needed.
func (b *BaseProvider) GetLogger() logging.Logger {
	return b.logger
}

// GetTokenManager returns the token manager. Set once during init, no mutex needed.
func (b *BaseProvider) GetTokenManager() *auth.TokenManager {
	return b.tokenManager
}

// SetTokenManager sets the token manager via dependency injection.
// Should be set once during initialization.
func (b *BaseProvider) SetTokenManager(tm *auth.TokenManager) {
	b.tokenManager = tm
}

// SetHTTPClient sets the HTTP client via dependency injection.
// Should be set once during initialization.
func (b *BaseProvider) SetHTTPClient(client *http.Client) {
	b.httpClient = client
}
