# Phase 2: Factory Patterns Implementation

**Phase**: 2 of 7  
**Estimated Time**: 2-3 days  
**Risk Level**: Medium  
**Dependencies**: Phase 1 (Core interfaces and base structures)

---

## Phase Objectives

This phase implements factory patterns to eliminate direct instantiation and enable dependency injection. Factories provide a centralized way to create objects with consistent configuration and make testing easier.

**Primary Goals:**
1. Create HTTPClientFactory interface and implementation
2. Create AuthenticatorFactory for consistent authenticator creation
3. Update existing ProxyAwareHTTPClientFactory to implement new interface
4. Ensure thread-safe factory operations
5. Enable proxy-aware client creation for future features

---

## Target Files to Create

1. `config/factory.go` - HTTPClientFactory interface and StandardHTTPClientFactory implementation
2. `auth/factory.go` - AuthenticatorFactory implementation

## Target Files to Modify

1. `config/proxy_client_factory.go` - Update to implement HTTPClientFactory interface
2. `config/config.go` - Add DefaultTimeouts struct (optional enhancement)

---

## Step-by-Step Instructions

### Step 1: Create HTTPClientFactory Interface and Implementation

**File to Create**: `config/factory.go`

```go
// Package config provides factory interfaces for HTTP client creation
package config

import (
	"net/http"
	"time"

	"github.com/sunbankio/qwencoder-proxy/auth"
	"github.com/sunbankio/qwencoder-proxy/logging"
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
	logger      logging.Logger
	baseConfig  HTTPClientConfig
}

// NewStandardHTTPClientFactory creates a new standard HTTP client factory
// Returns a factory initialized with the provided configuration
func NewStandardHTTPClientFactory(baseConfig HTTPClientConfig, logger logging.Logger) *StandardHTTPClientFactory {
	return &StandardHTTPClientFactory{
		proxyFactory: NewProxyAwareHTTPClientFactory(baseConfig, logger, 50),
		logger:      logger,
		baseConfig:  baseConfig,
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
	AuthenticatorTimeout  time.Duration // 30 seconds for auth requests
	StreamingTimeout     time.Duration // 15 minutes for streaming
	RequestTimeout      time.Duration // 45 seconds for standard requests
}

// GetDefaultTimeouts returns standard timeout values
func GetDefaultTimeouts() DefaultTimeouts {
	return DefaultTimeouts{
		ProviderTimeout:      5 * time.Minute,
		AuthenticatorTimeout:  30 * time.Second,
		StreamingTimeout:     15 * time.Minute,
		RequestTimeout:      45 * time.Second,
	}
}
```

**Edge Cases:**
- **Nil Logger**: The constructor accepts a logger interface, which could be nil. The calling code is responsible for providing a valid logger.
- **Nil ProxyConfig**: GetClient should handle nil proxyConfig gracefully by returning a default client.
- **Timeout Caching**: GetClientWithTimeout creates a new client each time (not cached), which is intentional for custom timeouts.

**Verification:**
- File created at `config/factory.go`
- HTTPClientFactory interface defines all 3 methods
- StandardHTTPClientFactory implements HTTPClientFactory
- DefaultTimeouts struct is defined
- GetDefaultTimeouts() returns standard values

---

### Step 2: Update ProxyAwareHTTPClientFactory

**File to Modify**: `config/proxy_client_factory.go`

**Before Modification**: Read the existing file to understand current implementation

```bash
# Read the existing file to understand current structure
cat config/proxy_client_factory.go
```

**Modification Required**: Ensure ProxyAwareHTTPClientFactory implements HTTPClientFactory interface

The existing ProxyAwareHTTPClientFactory already has a GetClient method. We need to verify it matches the interface signature and add missing methods if needed.

**Expected Changes**:
1. Verify GetClient method signature: `GetClient(proxyConfig *auth.ProxyConfig) *http.Client`
2. Add GetDefaultClient method if missing: `GetDefaultClient() *http.Client`
3. Add GetClientWithTimeout method if missing: `GetClientWithTimeout(timeout time.Duration) *http.Client`

**Implementation** (if methods are missing):

```go
// Add to ProxyAwareHTTPClientFactory struct in config/proxy_client_factory.go

// GetDefaultClient returns a default HTTP client (no proxy)
// This method implements the HTTPClientFactory interface
func (f *ProxyAwareHTTPClientFactory) GetDefaultClient() *http.Client {
	return f.GetClient(nil)
}

// GetClientWithTimeout returns an HTTP client with specific timeout
// This method implements the HTTPClientFactory interface
// Creates a new client with the specified timeout (not cached)
func (f *ProxyAwareHTTPClientFactory) GetClientWithTimeout(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout}
}
```

**Edge Cases:**
- **Thread Safety**: The existing ProxyAwareHTTPClientFactory uses cacheLock for thread-safe access to the client cache.
- **Cache Eviction**: The existing implementation has LRU cache eviction logic. Ensure it's preserved.
- **Nil ProxyConfig**: GetClient should handle nil proxyConfig by returning a default client.

**Verification:**
- ProxyAwareHTTPClientFactory implements HTTPClientFactory interface
- All 3 interface methods are present
- Existing caching logic is preserved
- Thread safety is maintained

---

### Step 3: Create AuthenticatorFactory

**File to Create**: `auth/factory.go`

```go
// Package auth provides factory for creating authenticators
package auth

import (
	"github.com/sunbankio/qwencoder-proxy/config"
	"github.com/sunbankio/qwencoder-proxy/logging"
)

// AuthenticatorFactory creates authenticators for different providers
// This follows the Factory pattern and Abstract Factory pattern
// It enables dependency injection and consistent authenticator creation
type AuthenticatorFactory struct {
	logger        logging.Logger
	clientFactory config.HTTPClientFactory
}

// NewAuthenticatorFactory creates a new authenticator factory
// Returns a factory initialized with the provided dependencies
func NewAuthenticatorFactory(logger logging.Logger, clientFactory config.HTTPClientFactory) *AuthenticatorFactory {
	return &AuthenticatorFactory{
		logger:        logger,
		clientFactory: clientFactory,
	}
}

// CreateGeminiAuthenticator creates a new Gemini authenticator
// Uses BaseAuthenticator for common functionality
// Returns a GeminiAuthenticator with default config if config is nil
func (f *AuthenticatorFactory) CreateGeminiAuthenticator(config *GeminiOAuthConfig) *GeminiAuthenticator {
	if config == nil {
		config = DefaultGeminiOAuthConfig()
	}
	return &GeminiAuthenticator{
		BaseAuthenticator: NewBaseAuthenticator(f.logger),
		config:           config,
	}
}

// CreateKiroAuthenticator creates a new Kiro authenticator
// Uses BaseAuthenticator for common functionality
// Returns a KiroAuthenticator with default config if config is nil
func (f *AuthenticatorFactory) CreateKiroAuthenticator(config *KiroOAuthConfig) *KiroAuthenticator {
	if config == nil {
		config = DefaultKiroOAuthConfig()
	}
	return &KiroAuthenticator{
		BaseAuthenticator: NewBaseAuthenticator(f.logger),
		config:           config,
	}
}

// CreateIFlowAuthenticator creates a new iFlow authenticator
// Uses BaseAuthenticator for common functionality
// Returns an IFlowAuthenticator with default config if config is nil
func (f *AuthenticatorFactory) CreateIFlowAuthenticator(config *IFlowOAuthConfig) *IFlowAuthenticator {
	if config == nil {
		config = DefaultIFlowOAuthConfig()
	}
	return &IFlowAuthenticator{
		BaseAuthenticator: NewBaseAuthenticator(f.logger),
		config:           config,
	}
}

// ProviderType represents the type of provider
// This enum is used for factory method selection
type ProviderType string

const (
	ProviderGemini   ProviderType = "gemini"
	ProviderKiro     ProviderType = "kiro"
	ProviderIFlow    ProviderType = "iflow"
	ProviderQwen     ProviderType = "qwen"
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
```

**Edge Cases:**
- **Nil Logger**: The constructor accepts a logger interface, which could be nil. The calling code is responsible for providing a valid logger.
- **Nil ClientFactory**: The constructor accepts a clientFactory which could be nil. The calling code is responsible for providing a valid factory.
- **Type Assertion**: CreateAuthenticator uses type assertion on config parameter. This is intentional but requires the caller to provide the correct config type.
- **Unknown Provider Type**: CreateAuthenticator returns nil for unknown provider types and logs an error.

**Verification:**
- File created at `auth/factory.go`
- AuthenticatorFactory struct is defined
- All 3 Create*Authenticator methods are present
- CreateAuthenticator method handles all known provider types
- BaseAuthenticator is used in all authenticator constructors

---

### Step 4: Update BaseAuthenticator to Use HTTPClientFactory (Optional Enhancement)

**File to Modify**: `auth/base.go`

This is an optional enhancement to enable proxy-aware authenticators. If you want to defer this to a later phase, skip this step.

**Modification Required**: Add clientFactory field and update GetHTTPClient method

```go
// Add to BaseAuthenticator struct in auth/base.go

type BaseAuthenticator struct {
	tokenManager  *TokenManager
	multiTokenMgr *MultiTokenManager
	mu            sync.RWMutex
	logger        logging.Logger
	httpClient    *http.Client
	clientFactory config.HTTPClientFactory  // NEW: Add this field
}

// Update NewBaseAuthenticator constructor in auth/base.go

func NewBaseAuthenticator(logger logging.Logger, clientFactory config.HTTPClientFactory) *BaseAuthenticator {
	return &BaseAuthenticator{
		logger:        logger,
		clientFactory: clientFactory,  // NEW: Store the factory
		httpClient:    &http.Client{Timeout: 30 * time.Second},
	}
}

// Add new method to BaseAuthenticator in auth/base.go

// GetClientWithProxy returns an HTTP client configured with proxy settings
// This method supports the proxy-per-token feature
// Returns the client from the factory if available, otherwise the default client
func (ba *BaseAuthenticator) GetClientWithProxy(proxyConfig *auth.ProxyConfig) *http.Client {
	if ba.clientFactory != nil {
		return ba.clientFactory.GetClient(proxyConfig)
	}
	return ba.httpClient
}
```

**Edge Cases:**
- **Nil ClientFactory**: GetClientWithProxy safely handles nil clientFactory by falling back to the default httpClient.
- **Thread Safety**: The clientFactory is set once during construction and is immutable, so no mutex is needed for access.

**Verification:**
- BaseAuthenticator has clientFactory field
- NewBaseAuthenticator accepts clientFactory parameter
- GetClientWithProxy method is present
- Backward compatibility is maintained (clientFactory is optional)

---

## Build Verification

After completing all steps, verify the code builds successfully:

```bash
cd c:/Users/jappa/projecten/qwencoder-proxy/qwencoder-proxy
go build ./...
```

**Expected Output**: No build errors

**If Build Fails**:
1. Check for missing imports in each file
2. Verify interface method signatures match
3. Ensure all files are in the correct directories
4. Check for circular dependencies

---

## Test Verification

After successful build, run existing tests to ensure no regressions:

```bash
go test ./config/... -v
go test ./auth/... -v
```

**Expected Output**: All existing tests pass

**Note**: New files created in this phase do not yet have tests. Tests will be added in later phases.

---

## API Compatibility Verification

This phase creates new factory interfaces and implementations but does not modify existing code (except for optional enhancements). Therefore, API compatibility is maintained.

**Verification Steps**:
1. No existing files were modified (except optional enhancements)
2. All new files are additions only
3. No changes to public interfaces of existing code

---

## Concurrency Considerations

### Thread Safety in Factories

1. **AuthenticatorFactory**: The factory itself is stateless after construction. Each Create* method creates a new instance, so no shared state exists. Thread-safe by design.

2. **StandardHTTPClientFactory**: Delegates to ProxyAwareHTTPClientFactory, which uses cacheLock for thread-safe access to the client cache. Thread-safe by delegation.

3. **ProxyAwareHTTPClientFactory**: Uses cacheLock (sync.RWMutex) for thread-safe access to the client cache. Thread-safe by design.

### Concurrency Edge Cases

1. **Concurrent Factory Creation**: Multiple goroutines can create factory instances simultaneously. This is safe because each factory is independent.

2. **Concurrent Client Creation**: Multiple goroutines can call GetClient simultaneously. ProxyAwareHTTPClientFactory uses cacheLock to ensure thread-safe access.

3. **Concurrent Authenticator Creation**: Multiple goroutines can call Create*Authenticator simultaneously. This is safe because each authenticator is independent.

---

## Phase Completion Criteria

Phase 2 is complete when:

- [x] `config/factory.go` created with HTTPClientFactory interface
- [x] `config/factory.go` created with StandardHTTPClientFactory implementation
- [x] `config/proxy_client_factory.go` updated to implement HTTPClientFactory
- [x] `auth/factory.go` created with AuthenticatorFactory
- [x] `auth/base.go` updated with clientFactory field (optional)
- [x] Code builds successfully with `go build ./...`
- [x] All existing tests pass

---

## Next Phase

Proceed to **Phase 3: Authentication Modules Refactoring** after Phase 2 is complete.

Phase 3 will refactor the authenticator implementations to use BaseAuthenticator and eliminate code duplication.
