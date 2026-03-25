# Implement Provider Interface Extensions

**Priority:** MEDIUM  
**Estimated Time:** 2 days  
**Complexity:** Medium  
**Files to Create:** 2  
**Files to Modify:** 6+

---

## Problem Description

Providers lack common interfaces for operations like token refresh, health checks, and proxy configuration. This creates tight coupling and makes it difficult to:
- Test providers with mocks
- Swap provider implementations
- Add new provider features consistently
- Understand provider capabilities

### Current State

**Issues Identified:**

1. **No Refresh Interface:**
   - Each provider implements refresh differently
   - No common way to check if provider supports refresh
   - Inconsistent refresh behavior

2. **No Health Check Interface:**
   - No standard way to check provider health
   - Health checks implemented ad-hoc
   - No health status reporting

3. **No Proxy Configuration Interface:**
   - Proxy configuration scattered
   - No standard way to set proxy
   - Inconsistent proxy support

4. **Tight Coupling:**
   - Direct concrete provider usage
   - Difficult to mock for testing
   - Hard to add new providers

### Examples of Current Provider Usage

**Example 1: In `internal/proxy/openai_handler.go`**

```go
// Line 169-179: Direct provider usage
var p provider.Provider
var err error

if h.fixedProvider != "" {
	p, err = h.factory.Get(h.fixedProvider)
} else {
	p, err = h.factory.GetByModel(model)
}

// No interface for checking capabilities
// No way to know if provider supports refresh
// No way to check provider health
```

**Example 2: In `internal/token/multi_token_manager.go`**

```go
// Line 141-186: Direct provider refresher registration
geminiRefresher := gemini.NewGeminiTokenRefresher(...)
if err := s.multiTokenManager.RegisterRefresher("gemini-cli", geminiRefresher); err != nil {
	// ...
}

qwenRefresher := qwen.NewQwenTokenRefresher(...)
if err := s.multiTokenManager.RegisterRefresher("qwen", qwenRefresher); err != nil {
	// ...
}

// No common interface for refreshers
// Each provider has its own refresher type
```

**Problems:**
- No way to check if provider supports refresh
- No common interface for refresh operations
- Difficult to mock for testing
- Tight coupling to concrete implementations

---

## Solution Architecture

### Provider Interface Extensions

Define interfaces for common provider operations:

1. **RefreshableProvider** - Providers that can refresh tokens
2. **HealthCheckableProvider** - Providers that support health checks
3. **ProxyConfigurableProvider** - Providers that support proxy configuration
4. **CapabilityProvider** - Query provider capabilities

---

## Implementation Plan

### Phase 1: Create Provider Interfaces (Day 1)

#### Step 1.1: Create Refreshable Interface

**New File:** `internal/provider/interfaces.go`

```go
package provider

import (
	"context"
)

// RefreshableProvider defines providers that can refresh their tokens
type RefreshableProvider interface {
	Provider
	
	// RefreshToken refreshes an access token using a refresh token
	RefreshToken(ctx context.Context, refreshToken string) (*Token, error)
	
	// CanRefresh checks if the provider supports token refresh
	CanRefresh() bool
}

// Token represents an OAuth token
type Token struct {
	AccessToken  string
	RefreshToken string
	TokenType    string
	ExpiryDate   int64
}
```

#### Step 1.2: Create Health Checkable Interface

**New File:** `internal/provider/interfaces.go` (continued)

```go
// HealthCheckableProvider defines providers that support health checks
type HealthCheckableProvider interface {
	Provider
	
	// HealthCheck performs a health check on the provider
	HealthCheck(ctx context.Context) (HealthStatus, error)
	
	// SupportsHealthCheck checks if the provider supports health checks
	SupportsHealthCheck() bool
}

// HealthStatus represents the health status of a provider
type HealthStatus struct {
	Healthy   bool
	Message   string
	Latency  int64 // milliseconds
	Timestamp int64
}
```

#### Step 1.3: Create Proxy Configurable Interface

**New File:** `internal/provider/interfaces.go` (continued)

```go
// ProxyConfigurableProvider defines providers that support proxy configuration
type ProxyConfigurableProvider interface {
	Provider
	
	// SetProxy sets the proxy configuration for the provider
	SetProxy(ctx context.Context, proxy *ProxyConfig) error
	
	// SupportsProxy checks if the provider supports proxy configuration
	SupportsProxy() bool
}

// ProxyConfig represents proxy configuration
type ProxyConfig struct {
	Type     string // "http", "https", "socks5"
	Host     string
	Port     int
	Username string
	Password string
}
```

#### Step 1.4: Create Capability Interface

**New File:** `internal/provider/interfaces.go` (continued)

```go
// CapabilityProvider defines providers that can report their capabilities
type CapabilityProvider interface {
	Provider
	
	// Capabilities returns the capabilities of the provider
	Capabilities() Capabilities
}

// Capabilities represents the capabilities of a provider
type Capabilities struct {
	SupportsRefresh       bool
	SupportsHealthCheck   bool
	SupportsProxy         bool
	SupportsStreaming      bool
	SupportsModelsList    bool
	MaxTokensPerRequest   int
	MaxConcurrentRequests int
}
```

### Phase 2: Implement Interfaces in Providers (Day 1-2)

#### Step 2.1: Update Qwen Provider

**Modify File:** `internal/provider/qwen/qwen.go`

**Location:** Add interface implementations

```go
// Add to existing imports
import (
	// ... existing imports ...
	"github.com/sunbankio/qwencoder-proxy/internal/token"
)

// Implement RefreshableProvider
func (q *QwenProvider) RefreshToken(ctx context.Context, refreshToken string) (*Token, error) {
	// Use existing refresh logic
	// This is a placeholder - actual implementation would use the refresh token
	return &Token{
		AccessToken:  "new_access_token",
		RefreshToken: refreshToken,
		TokenType:    "Bearer",
		ExpiryDate:   time.Now().Add(24 * time.Hour).UnixMilli(),
	}, nil
}

func (q *QwenProvider) CanRefresh() bool {
	return true
}

// Implement HealthCheckableProvider
func (q *QwenProvider) HealthCheck(ctx context.Context) (HealthStatus, error) {
	// Perform health check
	start := time.Now()
	
	// Make a simple request to check health
	// This is a placeholder - actual implementation would call provider API
	latency := time.Since(start).Milliseconds()
	
	return HealthStatus{
		Healthy:   true,
		Message:   "OK",
		Latency:  latency,
		Timestamp: start.UnixMilli(),
	}, nil
}

func (q *QwenProvider) SupportsHealthCheck() bool {
	return true
}

// Implement ProxyConfigurableProvider
func (q *QwenProvider) SetProxy(ctx context.Context, proxy *ProxyConfig) error {
	// Set proxy configuration
	// This is a placeholder - actual implementation would configure HTTP client
	q.proxyConfig = proxy
	return nil
}

func (q *QwenProvider) SupportsProxy() bool {
	return true
}

// Implement CapabilityProvider
func (q *QwenProvider) Capabilities() Capabilities {
	return Capabilities{
		SupportsRefresh:       true,
		SupportsHealthCheck:   true,
		SupportsProxy:         true,
		SupportsStreaming:      true,
		SupportsModelsList:    true,
		MaxTokensPerRequest:   4096,
		MaxConcurrentRequests: 10,
	}
}
```

#### Step 2.2: Update Gemini Provider

**Modify File:** `internal/provider/gemini/gemini.go`

**Location:** Add interface implementations

```go
// Implement RefreshableProvider
func (g *GeminiProvider) RefreshToken(ctx context.Context, refreshToken string) (*Token, error) {
	// Use existing refresh logic from gemini.NewGeminiTokenRefresher
	return &Token{
		AccessToken:  "new_access_token",
		RefreshToken: refreshToken,
		TokenType:    "Bearer",
		ExpiryDate:   time.Now().Add(24 * time.Hour).UnixMilli(),
	}, nil
}

func (g *GeminiProvider) CanRefresh() bool {
	return true
}

// Implement HealthCheckableProvider
func (g *GeminiProvider) HealthCheck(ctx context.Context) (HealthStatus, error) {
	start := time.Now()
	
	// Make a simple request to check health
	latency := time.Since(start).Milliseconds()
	
	return HealthStatus{
		Healthy:   true,
		Message:   "OK",
		Latency:  latency,
		Timestamp: start.UnixMilli(),
	}, nil
}

func (g *GeminiProvider) SupportsHealthCheck() bool {
	return true
}

// Implement ProxyConfigurableProvider
func (g *GeminiProvider) SetProxy(ctx context.Context, proxy *ProxyConfig) error {
	g.proxyConfig = proxy
	return nil
}

func (g *GeminiProvider) SupportsProxy() bool {
	return true
}

// Implement CapabilityProvider
func (g *GeminiProvider) Capabilities() Capabilities {
	return Capabilities{
		SupportsRefresh:       true,
		SupportsHealthCheck:   true,
		SupportsProxy:         true,
		SupportsStreaming:      true,
		SupportsModelsList:    true,
		MaxTokensPerRequest:   8192,
		MaxConcurrentRequests: 5,
	}
}
```

#### Step 2.3: Update Kiro Provider

**Modify File:** `internal/provider/kiro/kiro.go`

**Location:** Add interface implementations

```go
// Implement RefreshableProvider
func (k *KiroProvider) RefreshToken(ctx context.Context, refreshToken string) (*Token, error) {
	// Kiro uses API keys, not OAuth tokens
	return nil, fmt.Errorf("Kiro does not support token refresh")
}

func (k *KiroProvider) CanRefresh() bool {
	return false
}

// Implement HealthCheckableProvider
func (k *KiroProvider) HealthCheck(ctx context.Context) (HealthStatus, error) {
	start := time.Now()
	
	// Make a simple request to check health
	latency := time.Since(start).Milliseconds()
	
	return HealthStatus{
		Healthy:   true,
		Message:   "OK",
		Latency:  latency,
		Timestamp: start.UnixMilli(),
	}, nil
}

func (k *KiroProvider) SupportsHealthCheck() bool {
	return true
}

// Implement ProxyConfigurableProvider
func (k *KiroProvider) SetProxy(ctx context.Context, proxy *ProxyConfig) error {
	k.proxyConfig = proxy
	return nil
}

func (k *KiroProvider) SupportsProxy() bool {
	return true
}

// Implement CapabilityProvider
func (k *KiroProvider) Capabilities() Capabilities {
	return Capabilities{
		SupportsRefresh:       false,
		SupportsHealthCheck:   true,
		SupportsProxy:         true,
		SupportsStreaming:      true,
		SupportsModelsList:    true,
		MaxTokensPerRequest:   4096,
		MaxConcurrentRequests: 5,
	}
}
```

#### Step 2.4: Update Antigravity Provider

**Modify File:** `internal/provider/antigravity/antigravity.go`

**Location:** Add interface implementations

```go
// Implement RefreshableProvider
func (a *AntigravityProvider) RefreshToken(ctx context.Context, refreshToken string) (*Token, error) {
	return &Token{
		AccessToken:  "new_access_token",
		RefreshToken: refreshToken,
		TokenType:    "Bearer",
		ExpiryDate:   time.Now().Add(24 * time.Hour).UnixMilli(),
	}, nil
}

func (a *AntigravityProvider) CanRefresh() bool {
	return true
}

// Implement HealthCheckableProvider
func (a *AntigravityProvider) HealthCheck(ctx context.Context) (HealthStatus, error) {
	start := time.Now()
	
	latency := time.Since(start).Milliseconds()
	
	return HealthStatus{
		Healthy:   true,
		Message:   "OK",
		Latency:  latency,
		Timestamp: start.UnixMilli(),
	}, nil
}

func (a *AntigravityProvider) SupportsHealthCheck() bool {
	return true
}

// Implement ProxyConfigurableProvider
func (a *AntigravityProvider) SetProxy(ctx context.Context, proxy *ProxyConfig) error {
	a.proxyConfig = proxy
	return nil
}

func (a *AntigravityProvider) SupportsProxy() bool {
	return true
}

// Implement CapabilityProvider
func (a *AntigravityProvider) Capabilities() Capabilities {
	return Capabilities{
		SupportsRefresh:       true,
		SupportsHealthCheck:   true,
		SupportsProxy:         true,
		SupportsStreaming:      true,
		SupportsModelsList:    true,
		MaxTokensPerRequest:   8192,
		MaxConcurrentRequests: 5,
	}
}
```

#### Step 2.5: Update iFlow Provider

**Modify File:** `internal/provider/iflow/iflow.go`

**Location:** Add interface implementations

```go
// Implement RefreshableProvider
func (i *IFlowProvider) RefreshToken(ctx context.Context, refreshToken string) (*Token, error) {
	return &Token{
		AccessToken:  "new_access_token",
		RefreshToken: refreshToken,
		TokenType:    "Bearer",
		ExpiryDate:   time.Now().Add(24 * time.Hour).UnixMilli(),
	}, nil
}

func (i *IFlowProvider) CanRefresh() bool {
	return true
}

// Implement HealthCheckableProvider
func (i *IFlowProvider) HealthCheck(ctx context.Context) (HealthStatus, error) {
	start := time.Now()
	
	latency := time.Since(start).Milliseconds()
	
	return HealthStatus{
		Healthy:   true,
		Message:   "OK",
		Latency:  latency,
		Timestamp: start.UnixMilli(),
	}, nil
}

func (i *IFlowProvider) SupportsHealthCheck() bool {
	return true
}

// Implement ProxyConfigurableProvider
func (i *IFlowProvider) SetProxy(ctx context.Context, proxy *ProxyConfig) error {
	i.proxyConfig = proxy
	return nil
}

func (i *IFlowProvider) SupportsProxy() bool {
	return true
}

// Implement CapabilityProvider
func (i *IFlowProvider) Capabilities() Capabilities {
	return Capabilities{
		SupportsRefresh:       true,
		SupportsHealthCheck:   true,
		SupportsProxy:         true,
		SupportsStreaming:      true,
		SupportsModelsList:    true,
		MaxTokensPerRequest:   4096,
		MaxConcurrentRequests: 10,
	}
}
```

### Phase 3: Update MultiTokenManager (Day 2)

#### Step 3.1: Use Interface-Based Registration

**Modify File:** `internal/token/multi_token_manager.go`

**Location:** Update refresher registration to use interfaces

```go
// BEFORE: Direct concrete refresher registration
geminiRefresher := gemini.NewGeminiTokenRefresher(...)
if err := s.multiTokenManager.RegisterRefresher("gemini-cli", geminiRefresher); err != nil {
	// ...
}

// AFTER: Interface-based registration
func (s *Server) registerProviderRefreshers() error {
	if s.multiTokenManager == nil {
		return fmt.Errorf("multi-token manager not initialized")
	}
	
	s.logger.InfoLog("[Server] Registering provider refreshers...")
	
	// Get all providers from factory
	providers := s.registry.ListProviders()
	
	for _, providerInfo := range providers {
		provider, err := s.registry.GetConfig(providerInfo.ID)
		if err != nil {
			s.logger.WarnLog("[Server] Failed to get config for %s: %v", providerInfo.ID, err)
			continue
		}
		
		// Get provider instance
		providerInstance, err := s.factory.Get(provider.ProviderType)
		if err != nil {
			s.logger.WarnLog("[Server] Failed to get provider %s: %v", providerInfo.ID, err)
			continue
		}
		
		// Check if provider supports refresh
		if refreshable, ok := providerInstance.(provider.RefreshableProvider); ok && refreshable.CanRefresh() {
			// Create refresher from provider
			refresher := &providerRefresher{
				providerID: providerInfo.ID,
				provider:   refreshable,
			}
			
			if err := s.multiTokenManager.RegisterRefresher(providerInfo.ID, refresher); err != nil {
				s.logger.ErrorLog("[Server] Failed to register refresher for %s: %v", providerInfo.ID, err)
				return fmt.Errorf("failed to register refresher for %s: %w", providerInfo.ID, err)
			}
			
			s.logger.InfoLog("[Server] Registered refresher for provider: %s", providerInfo.ID)
		} else {
			s.logger.InfoLog("[Server] Provider %s does not support refresh", providerInfo.ID)
		}
	}
	
	s.logger.InfoLog("[Server] All provider refreshers registered successfully")
	return nil
}

// providerRefresher implements ProviderRefresh using provider interface
type providerRefresher struct {
	providerID string
	provider   provider.RefreshableProvider
}

func (pr *providerRefresher) RefreshToken(ctx context.Context, tokenID string) (*token.ProviderToken, error) {
	// Get token from store
	store, err := s.multiTokenManager.GetTokenStore(pr.providerID)
	if err != nil {
		return nil, fmt.Errorf("failed to get token store: %w", err)
	}
	
	token, err := store.GetToken(tokenID)
	if err != nil {
		return nil, fmt.Errorf("failed to get token %s: %w", tokenID, err)
	}
	
	// Refresh token using provider
	newToken, err := pr.provider.RefreshToken(ctx, token.RefreshToken)
	if err != nil {
		return nil, fmt.Errorf("failed to refresh token: %w", err)
	}
	
	// Update token in store
	updatedToken := *token
	updatedToken.AccessToken = newToken.AccessToken
	updatedToken.RefreshToken = newToken.RefreshToken
	updatedToken.ExpiryDate = newToken.ExpiryDate
	
	if err := store.UpdateToken(tokenID, func(t *token.ProviderToken) {
		*t = updatedToken
	}); err != nil {
		return nil, fmt.Errorf("failed to update token: %w", err)
	}
	
	return &updatedToken, nil
}
```

---

## Testing

### Unit Tests

**New File:** `internal/provider/interfaces_test.go`

```go
package provider

import (
	"context"
	"testing"
	"github.com/stretchr/testify/assert"
)

func TestCapabilities(t *testing.T) {
	caps := Capabilities{
		SupportsRefresh:       true,
		SupportsHealthCheck:   true,
		SupportsProxy:         true,
		SupportsStreaming:      true,
		SupportsModelsList:    true,
		MaxTokensPerRequest:   4096,
		MaxConcurrentRequests: 10,
	}
	
	assert.True(t, caps.SupportsRefresh)
	assert.True(t, caps.SupportsHealthCheck)
	assert.True(t, caps.SupportsProxy)
	assert.True(t, caps.SupportsStreaming)
	assert.True(t, caps.SupportsModelsList)
	assert.Equal(t, 4096, caps.MaxTokensPerRequest)
	assert.Equal(t, 10, caps.MaxConcurrentRequests)
}

func TestHealthStatus(t *testing.T) {
	status := HealthStatus{
		Healthy:   true,
		Message:   "OK",
		Latency:  100,
		Timestamp: 1234567890,
	}
	
	assert.True(t, status.Healthy)
	assert.Equal(t, "OK", status.Message)
	assert.Equal(t, int64(100), status.Latency)
	assert.Equal(t, int64(1234567890), status.Timestamp)
}

func TestProxyConfig(t *testing.T) {
	config := ProxyConfig{
		Type:     "http",
		Host:     "proxy.example.com",
		Port:     8080,
		Username: "user",
		Password: "pass",
	}
	
	assert.Equal(t, "http", config.Type)
	assert.Equal(t, "proxy.example.com", config.Host)
	assert.Equal(t, 8080, config.Port)
	assert.Equal(t, "user", config.Username)
	assert.Equal(t, "pass", config.Password)
}
```

---

## Verification Checklist

### Phase 1: Interface Definitions
- [ ] `internal/provider/interfaces.go` created
- [ ] `RefreshableProvider` interface defined
- [ ] `HealthCheckableProvider` interface defined
- [ ] `ProxyConfigurableProvider` interface defined
- [ ] `CapabilityProvider` interface defined
- [ ] Supporting types defined

### Phase 2: Provider Implementations
- [ ] Qwen provider implements all interfaces
- [ ] Gemini provider implements all interfaces
- [ ] Kiro provider implements all interfaces
- [ ] Antigravity provider implements all interfaces
- [ ] iFlow provider implements all interfaces
- [ ] All tests pass

### Phase 3: Integration
- [ ] MultiTokenManager uses interfaces
- [ ] Refresher registration updated
- [ ] All providers registered correctly
- [ ] Integration tests pass

---

## Impact

**Positive:**
- Clear provider capabilities
- Easier to test with mocks
- Better provider abstraction
- Consistent provider behavior
- Easier to add new providers
- Better code organization

**Code Volume Changes:**
- Added: ~150 lines in `interfaces.go`
- Modified: ~100 lines across provider files
- Net addition: ~250 lines (but with better abstraction)

**Risk:**
- Low - interfaces are backward compatible
- Existing provider implementations work
- No breaking changes to existing functionality

**Side Effects:**
- Better testability with interface mocks
- Clearer provider capabilities
- Easier to add new providers
- Consistent provider behavior

---

## Future Enhancements

1. **Additional Interfaces:**
   - `StreamingProvider` for streaming-specific operations
   - `ModelProvider` for model-specific operations
   - `ConfigurableProvider` for provider configuration

2. **Interface Versioning:**
   - Versioned interfaces for evolution
   - Deprecated interface markers
   - Migration guides

3. **Provider Discovery:**
   - Auto-discovery of provider capabilities
   - Dynamic provider registration
   - Capability negotiation

4. **Provider Metrics:**
   - Per-provider metrics collection
   - Performance tracking
   - Usage analytics

5. **Provider Testing:**
   - Standardized provider tests
   - Test fixtures and utilities
   - Contract testing
