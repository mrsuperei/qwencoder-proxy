# Phase 2: Common Proxy Error Handling

## Problem

### Code Duplication: Proxy Error Handling Functions

The project contains **identical proxy error handling code duplicated across 5 provider implementations**. Each provider has its own copy of:

1. **`isProxyError()`** - ~40 lines × 5 = **200 lines**
2. **`classifyProxyError()`** - ~50 lines × 5 = **250 lines**
3. **`doRequestWithProxy()`** - ~30 lines × 5 = **150 lines**

**Total duplicated code: ~600 lines**

### Affected Files

| File | Lines | Functions Duplicated |
|------|--------|-------------------|
| [`provider/gemini/gemini.go`](qwencoder-proxy/provider/gemini/gemini.go:175-298) | 175-298 | `isProxyError()`, `classifyProxyError()`, `doRequestWithProxy()` |
| [`provider/iflow/iflow.go`](qwencoder-proxy/provider/iflow/iflow.go:125-248) | 125-248 | `isProxyError()`, `classifyProxyError()`, `doRequestWithProxy()` |
| [`provider/qwen/qwen.go`](qwencoder-proxy/provider/qwen/qwen.go:235-358) | 235-358 | `isProxyError()`, `classifyProxyError()`, `doRequestWithProxy()` |
| [`provider/kiro/kiro.go`](qwencoder-proxy/provider/kiro/kiro.go:132-255) | 132-255 | `isProxyError()`, `classifyProxyError()`, `doRequestWithProxy()` |
| [`provider/antigravity/antigravity.go`](qwencoder-proxy/provider/antigravity/antigravity.go:171-294) | 171-294 | `isProxyError()`, `classifyProxyError()`, `doRequestWithProxy()` |

### Example of Identical Code

All 5 providers have **exactly the same** `isProxyError()` implementation:

```go
// This pattern is repeated 5 times with only the provider name changing in comments
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
```

### Issues with Current Implementation

1. **Maintenance burden**: Bug fixes must be applied to 5 files
2. **Inconsistency risk**: One provider might have different logic
3. **Code bloat**: 600 lines of duplicated code
4. **Testing difficulty**: Need to test the same logic 5 times

---

## How to Fix

### Step 1: Create Shared Error Handling Package

Create `internal/proxy/errors.go`:

```go
package proxy

import (
    "net"
    "net/url"
    "strings"
)

// IsProxyError checks if an error is a proxy-related error.
// This includes connection errors, timeout errors, and DNS resolution errors.
//
// This function is shared across all providers to ensure consistent
// error handling and reduce code duplication.
func IsProxyError(err error) bool {
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
        return IsProxyError(urlErr.Err)
    }

    return false
}

// ClassifyProxyError returns a string classification of the proxy error type.
// This is used for health tracking and logging purposes.
//
// Returns one of: "none", "timeout", "connection_refused", "connection_reset",
// "broken_pipe", "eof", "dns_resolution", "proxy_error", "socks_error",
// "tunnel_error", "url_error: <subtype>", "unknown"
func ClassifyProxyError(err error) string {
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
        return "url_error: " + ClassifyProxyError(urlErr.Err)
    }

    return "unknown"
}
```

### Step 2: Create Shared Request Handler

Create `internal/proxy/request.go`:

```go
package proxy

import (
    "fmt"
    "net/http"
    
    "github.com/sunbankio/qwencoder-proxy/logging"
    "github.com/sunbankio/qwencoder-proxy/internal/token"
)

// RequestHandler provides common request handling with proxy error classification.
// It encapsulates the logic for executing HTTP requests with proxy-aware
// error handling and health tracking.
type RequestHandler struct {
    providerName string
    logger      logging.Logger
    tokenMgr    *token.TokenManager
}

// NewRequestHandler creates a new request handler for a specific provider.
//
// Parameters:
//   - providerName: Name of the provider (e.g., "Gemini", "iFlow")
//   - logger: Logger instance for logging
//   - tokenMgr: Token manager for health tracking (can be nil)
func NewRequestHandler(providerName string, logger logging.Logger, tokenMgr *token.TokenManager) *RequestHandler {
    return &RequestHandler{
        providerName: providerName,
        logger:      logger,
        tokenMgr:    tokenMgr,
    }
}

// DoRequestWithProxy executes an HTTP request using the provided client
// and handles proxy error classification and health tracking.
//
// This method:
// 1. Executes the HTTP request
// 2. Detects if the error is proxy-related
// 3. Logs appropriate error messages
// 4. Updates proxy health tracking if token manager is available
//
// Parameters:
//   - req: The HTTP request to execute
//   - client: The HTTP client to use (may be proxy-aware)
//   - tokenID: Token identifier for health tracking
//
// Returns:
//   - *http.Response: The HTTP response (nil on error)
//   - error: Any error that occurred
func (h *RequestHandler) DoRequestWithProxy(req *http.Request, client *http.Client, tokenID string) (*http.Response, error) {
    resp, err := client.Do(req)
    if err != nil {
        // Check if this is a proxy error
        if IsProxyError(err) {
            errorType := ClassifyProxyError(err)
            h.logger.ErrorLog("[%s] Proxy error for token %s: %s (%s)", h.providerName, tokenID, err.Error(), errorType)

            // Update proxy health if tokenManager is available
            if h.tokenMgr != nil {
                if updateErr := h.tokenMgr.UpdateProxyHealth(tokenID, false, err); updateErr != nil {
                    h.logger.WarnLog("[%s] Failed to update proxy health for token %s: %v", h.providerName, tokenID, updateErr)
                }
            }
        } else {
            h.logger.ErrorLog("[%s] Request error for token %s: %v", h.providerName, tokenID, err)
        }
        return nil, err
    }

    // Update proxy health on success
    if h.tokenMgr != nil {
        if updateErr := h.tokenMgr.UpdateProxyHealth(tokenID, true, nil); updateErr != nil {
            h.logger.WarnLog("[%s] Failed to update proxy health for token %s: %v", h.providerName, tokenID, updateErr)
        }
    }

    return resp, nil
}
```

### Step 3: Update Provider Files

#### 3.1 Update `provider/gemini/gemini.go`

```go
// Add import:
import (
    // ... existing imports ...
    "github.com/sunbankio/qwencoder-proxy/internal/proxy"
)

// Add field to Provider struct:
type Provider struct {
    *provider.BaseProvider
    baseURL                string
    authenticator          *Authenticator
    projectID              string
    projectInitError       error
    requestHandler         *proxy.RequestHandler  // NEW
}

// Update NewProviderWithTokenManager:
func NewProviderWithTokenManager(authenticator *Authenticator, tokenManager *tokenpkg.TokenManager, logger logging.Logger) *Provider {
    if authenticator == nil {
        authenticator = NewAuthenticator(nil)
    }
    if tokenManager != nil {
        authenticator.SetTokenManager(tokenManager)
    }
    if logger == nil {
        logger = logging.NewLogger()
    }
    
    return &Provider{
        BaseProvider:  provider.NewBaseProvider(logger, 5*time.Minute),
        baseURL:       DefaultBaseURL,
        authenticator: authenticator,
        requestHandler: proxy.NewRequestHandler("Gemini", logger, tokenManager),  // NEW
    }
}

// REMOVE these functions (lines 174-298):
// func (p *Provider) isProxyError(err error) bool { ... }
// func (p *Provider) classifyProxyError(err error) string { ... }
// func (p *Provider) doRequestWithProxy(req *http.Request, client *http.Client, tokenID string) (*http.Response, error) { ... }

// Update all calls to p.doRequestWithProxy() to use p.requestHandler.DoRequestWithProxy()
```

#### 3.2 Update `provider/iflow/iflow.go`

```go
// Add import:
import (
    // ... existing imports ...
    "github.com/sunbankio/qwencoder-proxy/internal/proxy"
)

// Add field to Provider struct:
type Provider struct {
    *provider.BaseProvider
    baseURL                string
    authenticator          *Authenticator
    requestHandler         *proxy.RequestHandler  // NEW
}

// Update NewProvider:
func NewProvider(authenticator *Authenticator) *Provider {
    if authenticator == nil {
        authenticator = NewAuthenticator(nil)
    }
    
    logger := logging.NewLogger()
    return &Provider{
        BaseProvider:    provider.NewBaseProvider(logger, 5*time.Minute),
        baseURL:         APIBaseURL,
        authenticator:    authenticator,
        requestHandler:  proxy.NewRequestHandler("iFlow", logger, nil),  // NEW
    }
}

// REMOVE these functions (lines 124-248):
// func (p *Provider) isProxyError(err error) bool { ... }
// func (p *Provider) classifyProxyError(err error) string { ... }
// func (p *Provider) doRequestWithProxy(req *http.Request, client *http.Client, tokenID string) (*http.Response, error) { ... }

// Update all calls to p.doRequestWithProxy() to use p.requestHandler.DoRequestWithProxy()
```

#### 3.3 Update `provider/qwen/qwen.go`

```go
// Add import:
import (
    // ... existing imports ...
    "github.com/sunbankio/qwencoder-proxy/internal/proxy"
)

// Add field to Provider struct:
type Provider struct {
    *provider.BaseProvider
    authenticator          *QwenAuthenticator
    requestHandler         *proxy.RequestHandler  // NEW
}

// Update NewProviderWithTokenManager:
func NewProviderWithTokenManager(tokenManager *tokenpkg.TokenManager, logger logging.Logger) *Provider {
    return &Provider{
        BaseProvider:    provider.NewBaseProvider(logger, 5*time.Minute),
        authenticator:    NewQwenAuthenticator(tokenManager, logger),
        requestHandler:  proxy.NewRequestHandler("Qwen", logger, tokenManager),  // NEW
    }
}

// REMOVE these functions (lines 234-358):
// func (p *Provider) isProxyError(err error) bool { ... }
// func (p *Provider) classifyProxyError(err error) string { ... }
// func (p *Provider) doRequestWithProxy(req *http.Request, client *http.Client, tokenID string) (*http.Response, error) { ... }

// Update all calls to p.doRequestWithProxy() to use p.requestHandler.DoRequestWithProxy()
```

#### 3.4 Update `provider/kiro/kiro.go`

```go
// Add import:
import (
    // ... existing imports ...
    "github.com/sunbankio/qwencoder-proxy/internal/proxy"
)

// Add field to Provider struct:
type Provider struct {
    *provider.BaseProvider
    authenticator          *Authenticator
    machineID              string
    requestHandler         *proxy.RequestHandler  // NEW
}

// Update NewProvider:
func NewProvider(authenticator *Authenticator) *Provider {
    if authenticator == nil {
        authenticator = NewAuthenticator(nil)
    }
    
    logger := logging.NewLogger()
    return &Provider{
        BaseProvider:    provider.NewBaseProvider(logger, 5*time.Minute),
        authenticator:    authenticator,
        machineID:       generateMachineID(),
        requestHandler:  proxy.NewRequestHandler("Kiro", logger, nil),  // NEW
    }
}

// REMOVE these functions (lines 131-255):
// func (p *Provider) isProxyError(err error) bool { ... }
// func (p *Provider) classifyProxyError(err error) string { ... }
// func (p *Provider) doRequestWithProxy(req *http.Request, client *http.Client, tokenID string) (*http.Response, error) { ... }

// Update all calls to p.doRequestWithProxy() to use p.requestHandler.DoRequestWithProxy()
```

#### 3.5 Update `provider/antigravity/antigravity.go`

```go
// Add import:
import (
    // ... existing imports ...
    "github.com/sunbankio/qwencoder-proxy/internal/proxy"
)

// Add field to Provider struct:
type Provider struct {
    *provider.BaseProvider
    dailyBaseURL           string
    autopushBaseURL        string
    authenticator          *Authenticator
    projectID              string
    isInitialized          bool
    cachedModels           map[string]bool
    cacheMu                sync.RWMutex
    requestHandler         *proxy.RequestHandler  // NEW
}

// Update NewProvider:
func NewProvider(authenticator *Authenticator) *Provider {
    if authenticator == nil {
        authenticator = NewAuthenticator(nil)
    }
    
    logger := logging.NewLogger()
    return &Provider{
        BaseProvider:    provider.NewBaseProvider(logger, 5*time.Minute),
        dailyBaseURL:    DefaultDailyBaseURL,
        autopushBaseURL: DefaultAutopushBaseURL,
        authenticator:   authenticator,
        projectID:       "",
        isInitialized:   false,
        cachedModels:    make(map[string]bool),
        requestHandler:  proxy.NewRequestHandler("Antigravity", logger, nil),  // NEW
    }
}

// REMOVE these functions (lines 169-294):
// func (p *Provider) isProxyError(err error) bool { ... }
// func (p *Provider) classifyProxyError(err error) string { ... }
// func (p *Provider) doRequestWithProxy(req *http.Request, client *http.Client, tokenID string) (*http.Response, error) { ... }

// Update all calls to p.doRequestWithProxy() to use p.requestHandler.DoRequestWithProxy()
```

### Step 4: Update Function Calls

Search and replace all occurrences of `p.doRequestWithProxy(` with `p.requestHandler.DoRequestWithProxy(` in all provider files.

---

## Implementation Checklist

- [ ] Create `internal/proxy/errors.go`
- [ ] Create `internal/proxy/request.go`
- [ ] Update `provider/gemini/gemini.go`:
  - [ ] Add proxy import
  - [ ] Add requestHandler field
  - [ ] Update constructors
  - [ ] Remove isProxyError()
  - [ ] Remove classifyProxyError()
  - [ ] Remove doRequestWithProxy()
  - [ ] Update function calls
- [ ] Update `provider/iflow/iflow.go`:
  - [ ] Add proxy import
  - [ ] Add requestHandler field
  - [ ] Update constructors
  - [ ] Remove duplicate functions
  - [ ] Update function calls
- [ ] Update `provider/qwen/qwen.go`:
  - [ ] Add proxy import
  - [ ] Add requestHandler field
  - [ ] Update constructors
  - [ ] Remove duplicate functions
  - [ ] Update function calls
- [ ] Update `provider/kiro/kiro.go`:
  - [ ] Add proxy import
  - [ ] Add requestHandler field
  - [ ] Update constructors
  - [ ] Remove duplicate functions
  - [ ] Update function calls
- [ ] Update `provider/antigravity/antigravity.go`:
  - [ ] Add proxy import
  - [ ] Add requestHandler field
  - [ ] Update constructors
  - [ ] Remove duplicate functions
  - [ ] Update function calls
- [ ] Run existing tests
- [ ] Write new unit tests for proxy package

---

## Testing

### Unit Tests for `internal/proxy/errors.go`

Create `internal/proxy/errors_test.go`:

```go
package proxy

import (
    "errors"
    "net"
    "net/url"
    "strings"
    "testing"
)

func TestIsProxyError_Timeout(t *testing.T) {
    err := &net.OpError{Timeout: true}
    if !IsProxyError(err) {
        t.Error("Expected timeout error to be classified as proxy error")
    }
}

func TestIsProxyError_ConnectionRefused(t *testing.T) {
    err := errors.New("connection refused")
    if !IsProxyError(err) {
        t.Error("Expected connection refused to be classified as proxy error")
    }
}

func TestIsProxyError_DNS(t *testing.T) {
    err := errors.New("no such host")
    if !IsProxyError(err) {
        t.Error("Expected DNS error to be classified as proxy error")
    }
}

func TestIsProxyError_Proxy(t *testing.T) {
    err := errors.New("proxy connection failed")
    if !IsProxyError(err) {
        t.Error("Expected proxy error to be classified as proxy error")
    }
}

func TestIsProxyError_Nil(t *testing.T) {
    if IsProxyError(nil) {
        t.Error("Expected nil error to return false")
    }
}

func TestClassifyProxyError_Timeout(t *testing.T) {
    err := &net.OpError{Timeout: true}
    result := ClassifyProxyError(err)
    if result != "timeout" {
        t.Errorf("Expected 'timeout', got '%s'", result)
    }
}

func TestClassifyProxyError_ConnectionRefused(t *testing.T) {
    err := errors.New("connection refused")
    result := ClassifyProxyError(err)
    if result != "connection_refused" {
        t.Errorf("Expected 'connection_refused', got '%s'", result)
    }
}

func TestClassifyProxyError_URL(t *testing.T) {
    innerErr := errors.New("connection refused")
    err := &url.Error{Err: innerErr}
    result := ClassifyProxyError(err)
    if !strings.HasPrefix(result, "url_error:") {
        t.Errorf("Expected URL error prefix, got '%s'", result)
    }
}
```

### Unit Tests for `internal/proxy/request.go`

Create `internal/proxy/request_test.go`:

```go
package proxy

import (
    "net/http"
    "testing"
    
    "github.com/sunbankio/qwencoder-proxy/logging"
)

func TestNewRequestHandler(t *testing.T) {
    logger := logging.NewLogger()
    handler := NewRequestHandler("TestProvider", logger, nil)
    
    if handler.providerName != "TestProvider" {
        t.Errorf("Expected provider name 'TestProvider', got '%s'", handler.providerName)
    }
}

func TestDoRequestWithProxy_Success(t *testing.T) {
    // Create a mock server
    server := createMockServer(t, http.StatusOK, `{"status":"ok"}`)
    defer server.Close()
    
    logger := logging.NewLogger()
    handler := NewRequestHandler("Test", logger, nil)
    
    req, _ := http.NewRequest("GET", server.URL, nil)
    resp, err := handler.DoRequestWithProxy(req, http.DefaultClient, "test-token")
    
    if err != nil {
        t.Fatalf("Unexpected error: %v", err)
    }
    if resp.StatusCode != http.StatusOK {
        t.Errorf("Expected status 200, got %d", resp.StatusCode)
    }
}
```

### Integration Tests
- Test all providers with the new shared error handling
- Verify proxy health tracking still works
- Test error classification for various error types

---

## Estimated Effort

| Task | Time |
|------|------|
| Create internal/proxy/errors.go | 30 minutes |
| Create internal/proxy/request.go | 30 minutes |
| Write unit tests for errors.go | 30 minutes |
| Write unit tests for request.go | 30 minutes |
| Update 5 provider files | 2 hours |
| Run and fix tests | 30 minutes |
| **Total** | **4.5 hours** |

---

## Benefits

1. **Code reduction**: ~600 lines of duplicate code removed
2. **Single source of truth**: Bug fixes apply once
3. **Consistency**: All providers use identical error handling
4. **Testability**: Shared logic only needs to be tested once
5. **Maintainability**: Future providers can reuse this code
