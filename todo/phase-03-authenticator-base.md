# Phase 3: Common Authenticator Base

## Problem

### Code Duplication: Authenticator Methods

The project contains **identical authenticator method implementations duplicated across 3 provider authenticators**. Each authenticator implements the same 6 methods with nearly identical code:

| Method | Lines per Provider | Total Lines |
|--------|-------------------|--------------|
| `SetTokenManager()` | ~5 × 3 | 15 |
| `SetMultiTokenManager()` | ~5 × 3 | 15 |
| `GetTokenManager()` | ~5 × 3 | 15 |
| `GetMultiTokenManager()` | ~5 × 3 | 15 |
| `GetLogger()` | ~5 × 3 | 15 |
| `GetHTTPClient()` | ~5 × 3 | 15 |

**Total duplicated code: ~90 lines**

### Affected Files

| File | Lines | Methods Duplicated |
|------|--------|-------------------|
| [`provider/gemini/auth.go`](qwencoder-proxy/provider/gemini/auth.go:75-105) | 75-105 | All 6 methods |
| [`provider/iflow/auth.go`](qwencoder-proxy/provider/iflow/auth.go:147-177) | 147-177 | All 6 methods |
| [`provider/qwen/qwen.go`](qwencoder-proxy/provider/qwen/qwen.go:67-154) | 67-154 | All 6 methods (in QwenAuthenticator) |

### Example of Identical Code

All 3 authenticators have **exactly the same** method implementations:

```go
// This pattern is repeated 3 times with only the receiver type changing
func (a *Authenticator) SetTokenManager(tokenManager *tokenpkg.TokenManager) {
    a.tokenManager = tokenManager
}

func (a *Authenticator) SetMultiTokenManager(multiTokenMgr *tokenpkg.MultiTokenManager) {
    a.multiTokenMgr = multiTokenMgr
}

func (a *Authenticator) GetTokenManager() *tokenpkg.TokenManager {
    return a.tokenManager
}

func (a *Authenticator) GetMultiTokenManager() *tokenpkg.MultiTokenManager {
    return a.multiTokenMgr
}

func (a *Authenticator) GetLogger() logging.Logger {
    return a.logger
}

func (a *Authenticator) GetHTTPClient() (*http.Client, error) {
    return a.httpClient, nil
}
```

### Issues with Current Implementation

1. **Maintenance burden**: Changes to authenticator structure require updates to 3 files
2. **Inconsistency risk**: One authenticator might have different behavior
3. **Code bloat**: 90 lines of duplicated code
4. **No shared interface**: No common base for authenticator implementations

---

## How to Fix

### Step 1: Create Base Authenticator

Create `provider/auth_base.go`:

```go
package provider

import (
    "net/http"
    "time"
    
    "github.com/sunbankio/qwencoder-proxy/internal/token"
    "github.com/sunbankio/qwencoder-proxy/logging"
)

// BaseAuthenticator provides common authenticator functionality shared across
// all provider implementations.
//
// This base struct encapsulates the common fields and methods that are
// duplicated across gemini, iflow, and qwen authenticators.
//
// Usage:
//
//   type MyAuthenticator struct {
//       *provider.BaseAuthenticator
//       // provider-specific fields
//   }
type BaseAuthenticator struct {
    tokenManager  *token.TokenManager
    multiTokenMgr *token.MultiTokenManager
    logger        logging.Logger
    httpClient    *http.Client
}

// NewBaseAuthenticator creates a new base authenticator with the provided
// logger and HTTP client.
//
// Parameters:
//   - logger: Logger instance for logging (defaults to NewLogger() if nil)
//   - httpClient: HTTP client for requests (defaults to 30s timeout client if nil)
//
// Returns:
//   - *BaseAuthenticator: Initialized base authenticator
func NewBaseAuthenticator(logger logging.Logger, httpClient *http.Client) *BaseAuthenticator {
    if logger == nil {
        logger = logging.NewLogger()
    }
    if httpClient == nil {
        httpClient = &http.Client{Timeout: 30 * time.Second}
    }
    return &BaseAuthenticator{
        logger:     logger,
        httpClient: httpClient,
    }
}

// SetTokenManager sets the token manager for this authenticator.
//
// The token manager is used to select and manage authentication tokens.
// This method is typically called during provider initialization.
func (a *BaseAuthenticator) SetTokenManager(tokenManager *token.TokenManager) {
    a.tokenManager = tokenManager
}

// SetMultiTokenManager sets the multi-token manager for this authenticator.
//
// The multi-token manager is used for managing multiple tokens across
// different providers. This method is typically called during provider
// initialization.
func (a *BaseAuthenticator) SetMultiTokenManager(multiTokenMgr *token.MultiTokenManager) {
    a.multiTokenMgr = multiTokenMgr
}

// GetTokenManager returns the token manager associated with this authenticator.
//
// Returns:
//   - *token.TokenManager: The token manager (may be nil)
func (a *BaseAuthenticator) GetTokenManager() *token.TokenManager {
    return a.tokenManager
}

// GetMultiTokenManager returns the multi-token manager associated with this authenticator.
//
// Returns:
//   - *token.MultiTokenManager: The multi-token manager (may be nil)
func (a *BaseAuthenticator) GetMultiTokenManager() *token.MultiTokenManager {
    return a.multiTokenMgr
}

// GetLogger returns the logger associated with this authenticator.
//
// Returns:
//   - logging.Logger: The logger instance
func (a *BaseAuthenticator) GetLogger() logging.Logger {
    return a.logger
}

// GetHTTPClient returns the HTTP client associated with this authenticator.
//
// This method returns the HTTP client that should be used for making
// authenticated requests. The client may be configured with proxy settings.
//
// Returns:
//   - *http.Client: The HTTP client
//   - error: Always nil (provided for interface compatibility)
func (a *BaseAuthenticator) GetHTTPClient() (*http.Client, error) {
    return a.httpClient, nil
}
```

### Step 2: Update Provider Authenticators

#### 2.1 Update `provider/gemini/auth.go`

```go
// Add import:
import (
    // ... existing imports ...
    "github.com/sunbankio/qwencoder-proxy/provider"
)

// Update Authenticator struct to embed BaseAuthenticator:
type Authenticator struct {
    *provider.BaseAuthenticator  // EMBED instead of duplicating fields
    config        *OAuthConfig
}

// Remove these fields (now in BaseAuthenticator):
// tokenManager  *tokenpkg.TokenManager
// multiTokenMgr *tokenpkg.MultiTokenManager
// logger        logging.Logger
// httpClient    *http.Client

// Update NewAuthenticator:
func NewAuthenticator(config *OAuthConfig) *Authenticator {
    if config == nil {
        config = DefaultOAuthConfig()
    }
    
    return &Authenticator{
        BaseAuthenticator: provider.NewBaseAuthenticator(nil, &http.Client{Timeout: 30 * time.Second}),
        config:          config,
    }
}

// REMOVE these methods (lines 75-105):
// func (a *Authenticator) SetTokenManager(tokenManager *tokenpkg.TokenManager) { ... }
// func (a *Authenticator) SetMultiTokenManager(multiTokenMgr *tokenpkg.MultiTokenManager) { ... }
// func (a *Authenticator) GetTokenManager() *tokenpkg.TokenManager { ... }
// func (a *Authenticator) GetMultiTokenManager() *tokenpkg.MultiTokenManager { ... }
// func (a *Authenticator) GetLogger() logging.Logger { ... }
// func (a *Authenticator) GetHTTPClient() (*http.Client, error) { ... }

// These methods are now inherited from BaseAuthenticator
```

#### 2.2 Update `provider/iflow/auth.go`

```go
// Add import:
import (
    // ... existing imports ...
    "github.com/sunbankio/qwencoder-proxy/provider"
)

// Update Authenticator struct to embed BaseAuthenticator:
type Authenticator struct {
    *provider.BaseAuthenticator  // EMBED instead of duplicating fields
    config        *OAuthConfig
    credentials   *Credentials
    mu            sync.RWMutex
    tokenSource   oauth2.TokenSource
}

// Remove these fields (now in BaseAuthenticator):
// tokenManager  *tokenpkg.TokenManager
// multiTokenMgr *tokenpkg.MultiTokenManager
// logger        logging.Logger
// httpClient    *http.Client

// Update NewAuthenticator:
func NewAuthenticator(config *OAuthConfig) *Authenticator {
    if config == nil {
        config = DefaultOAuthConfig()
    }
    
    return &Authenticator{
        BaseAuthenticator: provider.NewBaseAuthenticator(nil, &http.Client{Timeout: 30 * time.Second}),
        config:          config,
        credentials:     nil,
        mu:              sync.RWMutex{},
        tokenSource:     nil,
    }
}

// REMOVE these methods (lines 147-177):
// func (a *Authenticator) SetTokenManager(tokenManager *tokenpkg.TokenManager) { ... }
// func (a *Authenticator) SetMultiTokenManager(multiTokenMgr *tokenpkg.MultiTokenManager) { ... }
// func (a *Authenticator) GetTokenManager() *tokenpkg.TokenManager { ... }
// func (a *Authenticator) GetMultiTokenManager() *tokenpkg.MultiTokenManager { ... }
// func (a *Authenticator) GetLogger() logging.Logger { ... }
// func (a *Authenticator) GetHTTPClient() (*http.Client, error) { ... }

// These methods are now inherited from BaseAuthenticator
```

#### 2.3 Update `provider/qwen/qwen.go`

```go
// Add import:
import (
    // ... existing imports ...
    "github.com/sunbankio/qwencoder-proxy/provider"
)

// Update QwenAuthenticator struct to embed BaseAuthenticator:
type QwenAuthenticator struct {
    *provider.BaseAuthenticator  // EMBED instead of duplicating fields
}

// Remove these fields (now in BaseAuthenticator):
// tokenManager  *tokenpkg.TokenManager
// multiTokenMgr *tokenpkg.MultiTokenManager
// logger        logging.Logger
// httpClient    *http.Client

// Update NewQwenAuthenticator:
func NewQwenAuthenticator(tokenManager *tokenpkg.TokenManager, logger logging.Logger) *QwenAuthenticator {
    return &QwenAuthenticator{
        BaseAuthenticator: provider.NewBaseAuthenticator(logger, nil),
    }
}

// Update SetMultiTokenManager:
func (a *QwenAuthenticator) SetMultiTokenManager(multiTokenMgr *tokenpkg.MultiTokenManager) {
    a.BaseAuthenticator.SetMultiTokenManager(multiTokenMgr)
    // Also set tokenManager if available
    if multiTokenMgr != nil {
        if tm, err := multiTokenMgr.GetTokenManager("qwen"); err == nil {
            a.SetTokenManager(tm)
        }
    }
}

// REMOVE these methods (lines 67-98):
// func (a *QwenAuthenticator) SetTokenManager(tokenManager *tokenpkg.TokenManager) { ... }
// func (a *QwenAuthenticator) GetTokenManager() *tokenpkg.TokenManager { ... }
// func (a *QwenAuthenticator) GetMultiTokenManager() *tokenpkg.MultiTokenManager { ... }
// func (a *QwenAuthenticator) GetLogger() logging.Logger { ... }
// func (a *QwenAuthenticator) GetHTTPClient() (*http.Client, error) { ... }

// These methods are now inherited from BaseAuthenticator
```

### Step 3: Verify All Method Calls

After embedding BaseAuthenticator, ensure all method calls still work:

- `authenticator.SetTokenManager(tm)` → `authenticator.BaseAuthenticator.SetTokenManager(tm)`
- `authenticator.GetTokenManager()` → `authenticator.BaseAuthenticator.GetTokenManager()`
- etc.

Since Go allows promoted methods from embedded structs, the calls should work without changes. However, verify that:

1. All method signatures match the provider.Authenticator interface
2. No direct field access to the removed fields exists
3. All tests still pass

---

## Implementation Checklist

- [ ] Create `provider/auth_base.go`
- [ ] Update `provider/gemini/auth.go`:
  - [ ] Add provider import
  - [ ] Embed BaseAuthenticator
  - [ ] Remove duplicate fields
  - [ ] Update NewAuthenticator()
  - [ ] Remove 6 duplicate methods
- [ ] Update `provider/iflow/auth.go`:
  - [ ] Add provider import
  - [ ] Embed BaseAuthenticator
  - [ ] Remove duplicate fields
  - [ ] Update NewAuthenticator()
  - [ ] Remove 6 duplicate methods
- [ ] Update `provider/qwen/qwen.go`:
  - [ ] Add provider import
  - [ ] Embed BaseAuthenticator
  - [ ] Remove duplicate fields
  - [ ] Update NewQwenAuthenticator()
  - [ ] Update SetMultiTokenManager()
  - [ ] Remove 5 duplicate methods
- [ ] Run existing tests
- [ ] Write unit tests for auth_base.go

---

## Testing

### Unit Tests for `provider/auth_base.go`

Create `provider/auth_base_test.go`:

```go
package provider

import (
    "net/http"
    "testing"
    "time"
    
    "github.com/sunbankio/qwencoder-proxy/logging"
)

func TestNewBaseAuthenticator_Defaults(t *testing.T) {
    auth := NewBaseAuthenticator(nil, nil)
    
    if auth.GetLogger() == nil {
        t.Error("Expected default logger to be created")
    }
    
    if auth.GetHTTPClient() == nil {
        t.Error("Expected default HTTP client to be created")
    }
}

func TestNewBaseAuthenticator_Custom(t *testing.T) {
    logger := logging.NewLogger()
    client := &http.Client{Timeout: 60 * time.Second}
    
    auth := NewBaseAuthenticator(logger, client)
    
    if auth.GetLogger() != logger {
        t.Error("Expected provided logger")
    }
    
    if auth.GetHTTPClient() == nil {
        t.Error("Expected provided HTTP client")
    }
}

func TestBaseAuthenticator_SetTokenManager(t *testing.T) {
    auth := NewBaseAuthenticator(nil, nil)
    
    // Initially nil
    if auth.GetTokenManager() != nil {
        t.Error("Expected initial token manager to be nil")
    }
    
    // Set token manager
    tm := &token.TokenManager{}
    auth.SetTokenManager(tm)
    
    if auth.GetTokenManager() != tm {
        t.Error("Expected token manager to be set")
    }
}

func TestBaseAuthenticator_SetMultiTokenManager(t *testing.T) {
    auth := NewBaseAuthenticator(nil, nil)
    
    // Initially nil
    if auth.GetMultiTokenManager() != nil {
        t.Error("Expected initial multi-token manager to be nil")
    }
    
    // Set multi-token manager
    mtm := &token.MultiTokenManager{}
    auth.SetMultiTokenManager(mtm)
    
    if auth.GetMultiTokenManager() != mtm {
        t.Error("Expected multi-token manager to be set")
    }
}

func TestBaseAuthenticator_GetLogger(t *testing.T) {
    logger := logging.NewLogger()
    auth := NewBaseAuthenticator(logger, nil)
    
    if auth.GetLogger() != logger {
        t.Error("Expected provided logger")
    }
}

func TestBaseAuthenticator_GetHTTPClient(t *testing.T) {
    client := &http.Client{Timeout: 30 * time.Second}
    auth := NewBaseAuthenticator(nil, client)
    
    returnedClient, err := auth.GetHTTPClient()
    
    if err != nil {
        t.Errorf("Expected no error, got %v", err)
    }
    if returnedClient != client {
        t.Error("Expected provided HTTP client")
    }
}
```

### Integration Tests
- Test all providers with embedded BaseAuthenticator
- Verify all authenticator methods work correctly
- Test token manager integration

---

## Estimated Effort

| Task | Time |
|------|------|
| Create provider/auth_base.go | 30 minutes |
| Write unit tests for auth_base.go | 30 minutes |
| Update provider/gemini/auth.go | 30 minutes |
| Update provider/iflow/auth.go | 30 minutes |
| Update provider/qwen/qwen.go | 30 minutes |
| Run and fix tests | 30 minutes |
| **Total** | **3 hours** |

---

## Benefits

1. **Code reduction**: ~90 lines of duplicate code removed
2. **Single source of truth**: Authenticator methods defined once
3. **Consistency**: All authenticators have identical behavior
4. **Extensibility**: New authenticators can easily embed BaseAuthenticator
5. **Maintainability**: Changes to authenticator interface apply to all providers
