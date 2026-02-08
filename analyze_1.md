# Code Duplication Analysis & Refactoring Report

**Project**: qwencoder-proxy  
**Analysis Date**: 2026-02-08  
**Purpose**: Identify code duplication, merging opportunities, and areas for cleaner code using factories and interfaces

---

## Executive Summary

This analysis identifies significant code duplication across providers, authenticators, converters, and handlers. The [`SOLID principles analysis`](plans/solid-principles-architectural-analysis.md:1) previously identified Dependency Inversion Principle violations, and this analysis confirms extensive duplication that can be addressed through base structs, factories, and interfaces.

**Key Findings:**
- **~650 lines** of duplicate code (~15% of analyzed codebase)
- All 5 providers share nearly identical struct fields and initialization patterns
- All authenticators have duplicate field declarations and setter methods
- Proxy handlers share identical CORS, constructor, and error handling patterns
- HTTP client creation is scattered with inconsistent timeouts
- Missing factory patterns for authenticators and consistent HTTP client management

---

## 1. Provider Implementations - High Duplication

**Location**: [`provider/gemini/gemini.go`](provider/gemini/gemini.go:38), [`provider/qwen/qwen.go`](provider/qwen/qwen.go:37), [`provider/kiro/kiro.go`](provider/kiro/kiro.go:58), [`provider/iflow/iflow.go`](provider/iflow/iflow.go:46), [`provider/antigravity/antigravity.go`](provider/antigravity/antigravity.go:63)

### Duplicate Patterns Identified

| Pattern | Duplicated In | Line References |
|---------|---------------|----------------|
| HTTP Client field | All providers | gemini:41, qwen:39, kiro:60, iflow:49, antigravity:67 |
| Token Manager field | All providers | gemini:42, qwen:40, kiro:61, iflow:50, antigravity:68 |
| Logger field | All providers | gemini:43, qwen:41, kiro:62, iflow:51, antigravity:69 |
| HTTP Client timeout (5min) | gemini, kiro, iflow | gemini:56, kiro:73, iflow:62 |
| Constructor pattern | All providers | gemini:48-59, kiro:66-77, iflow:54-65 |

### Example: Gemini Provider Structure
```go
// provider/gemini/gemini.go:38-46
type Provider struct {
    baseURL          string
    authenticator    *auth.GeminiAuthenticator
    httpClient       *http.Client
    tokenManager     *auth.TokenManager
    logger           *logging.Logger
    projectID        string
    projectInitError error
}
```

### Example: Qwen Provider Structure
```go
// provider/qwen/qwen.go:37-42
type Provider struct {
    authenticator *QwenAuthenticator
    httpClient    *http.Client
    tokenManager  *auth.TokenManager
    logger        *logging.Logger
}
```

### Recommendation: Create BaseProvider

Create `provider/base.go`:

```go
// Package provider provides base structures for provider implementations
package provider

import (
    "net/http"
    "time"

    "github.com/sunbankio/qwencoder-proxy/auth"
    "github.com/sunbankio/qwencoder-proxy/logging"
)

// BaseProvider provides common fields and methods for all providers
type BaseProvider struct {
    httpClient    *http.Client
    tokenManager  *auth.TokenManager
    logger        *logging.Logger
}

// NewBaseProvider creates a new base provider with default HTTP client
func NewBaseProvider(timeout time.Duration) *BaseProvider {
    return &BaseProvider{
        httpClient: &http.Client{Timeout: timeout},
        logger:    logging.NewLogger(),
    }
}

// GetHTTPClient returns the HTTP client
func (bp *BaseProvider) GetHTTPClient() *http.Client {
    return bp.httpClient
}

// GetLogger returns the logger
func (bp *BaseProvider) GetLogger() *logging.Logger {
    return bp.logger
}

// GetTokenManager returns the token manager
func (bp *BaseProvider) GetTokenManager() *auth.TokenManager {
    return bp.tokenManager
}

// SetTokenManager sets the token manager
func (bp *BaseProvider) SetTokenManager(tm *auth.TokenManager) {
    bp.tokenManager = tm
}
```

Then each provider can embed BaseProvider:

```go
// provider/gemini/gemini.go
type Provider struct {
    *provider.BaseProvider  // Embedded base provider
    baseURL          string
    authenticator    *auth.GeminiAuthenticator
    projectID        string
    projectInitError error
}

func NewProvider(authenticator *auth.GeminiAuthenticator) *Provider {
    if authenticator == nil {
        authenticator = auth.NewGeminiAuthenticator(nil)
    }
    return &Provider{
        BaseProvider: provider.NewBaseProvider(5 * time.Minute),
        baseURL:     DefaultBaseURL,
        authenticator: authenticator,
    }
}
```

### Estimated Code Reduction
- **~40 lines** eliminated by creating BaseProvider
- **~160 lines** eliminated by refactoring all 5 providers to use it

---

## 2. Authenticator Implementations - Very High Duplication

**Location**: [`auth/gemini_auth.go`](auth/gemini_auth.go:55), [`auth/kiro_auth.go`](auth/kiro_auth.go:55), [`auth/iflow_auth.go`](auth/iflow_auth.go:~120)

### Duplicate Patterns Identified

| Pattern | Duplicated In | Line References |
|---------|---------------|----------------|
| HTTP Client field | All authenticators | gemini:61, kiro:62, iflow:~120 |
| Token Manager field | All authenticators | gemini:57, kiro:58, iflow:~120 |
| MultiToken Manager field | All authenticators | gemini:58, kiro:59, iflow:~120 |
| Logger field | All authenticators | gemini:60, kiro:61, iflow:~120 |
| Mutex field | All authenticators | gemini:59, kiro:60, iflow:~120 |
| HTTP Client timeout (30s) | gemini, kiro | gemini:74, kiro:75 |
| SetTokenManager() method | All authenticators | gemini:79-83, kiro:80-84 |
| SetMultiTokenManager() method | All authenticators | gemini:86-90, kiro:87-91 |

### Example: Gemini Authenticator Structure
```go
// auth/gemini_auth.go:55-62
type GeminiAuthenticator struct {
    config        *GeminiOAuthConfig
    tokenManager  *TokenManager
    multiTokenMgr *MultiTokenManager
    mu            sync.RWMutex
    logger        *logging.Logger
    httpClient    *http.Client
}
```

### Example: Kiro Authenticator Structure
```go
// auth/kiro_auth.go:55-63
type KiroAuthenticator struct {
    config        *KiroOAuthConfig
    credentials   *KiroCredentials
    tokenManager  *TokenManager
    multiTokenMgr *MultiTokenManager
    mu            sync.RWMutex
    logger        *logging.Logger
    httpClient    *http.Client
}
```

### Recommendation: Create BaseAuthenticator

Create `auth/base.go`:

```go
// Package auth provides base structures for authenticator implementations
package auth

import (
    "net/http"
    "sync"
    "time"

    "github.com/sunbankio/qwencoder-proxy/logging"
)

// BaseAuthenticator provides common fields and methods for all authenticators
type BaseAuthenticator struct {
    tokenManager  *TokenManager
    multiTokenMgr *MultiTokenManager
    mu            sync.RWMutex
    logger        *logging.Logger
    httpClient    *http.Client
}

// NewBaseAuthenticator creates a new base authenticator with default HTTP client
func NewBaseAuthenticator() *BaseAuthenticator {
    return &BaseAuthenticator{
        logger:     logging.NewLogger(),
        httpClient: &http.Client{Timeout: 30 * time.Second},
    }
}

// SetTokenManager sets the token manager for this authenticator
func (ba *BaseAuthenticator) SetTokenManager(tokenManager *TokenManager) {
    ba.mu.Lock()
    defer ba.mu.Unlock()
    ba.tokenManager = tokenManager
}

// SetMultiTokenManager sets the multi-token manager for this authenticator
func (ba *BaseAuthenticator) SetMultiTokenManager(mtm *MultiTokenManager) {
    ba.mu.Lock()
    defer ba.mu.Unlock()
    ba.multiTokenMgr = mtm
}

// GetTokenManager returns the token manager
func (ba *BaseAuthenticator) GetTokenManager() *TokenManager {
    ba.mu.RLock()
    defer ba.mu.RUnlock()
    return ba.tokenManager
}

// GetMultiTokenManager returns the multi-token manager
func (ba *BaseAuthenticator) GetMultiTokenManager() *MultiTokenManager {
    ba.mu.RLock()
    defer ba.mu.RUnlock()
    return ba.multiTokenMgr
}

// GetLogger returns the logger
func (ba *BaseAuthenticator) GetLogger() *logging.Logger {
    return ba.logger
}

// GetHTTPClient returns the HTTP client
func (ba *BaseAuthenticator) GetHTTPClient() *http.Client {
    return ba.httpClient
}
```

Then each authenticator can embed BaseAuthenticator:

```go
// auth/gemini_auth.go
type GeminiAuthenticator struct {
    *BaseAuthenticator  // Embedded base authenticator
    config        *GeminiOAuthConfig
}

func NewGeminiAuthenticator(config *GeminiOAuthConfig) *GeminiAuthenticator {
    if config == nil {
        config = DefaultGeminiOAuthConfig()
    }
    return &GeminiAuthenticator{
        BaseAuthenticator: NewBaseAuthenticator(),
        config:           config,
    }
}
```

### Estimated Code Reduction
- **~50 lines** eliminated by creating BaseAuthenticator
- **~200 lines** eliminated by refactoring all authenticators to use it

---

## 3. Converter Implementations - Medium Duplication

**Location**: [`converter/gemini.go`](converter/gemini.go:14), [`converter/qwen.go`](converter/qwen.go:12), [`converter/claude.go`](converter/claude.go:13)

### Duplicate Patterns Identified

| Pattern | Duplicated In | Line References |
|---------|---------------|----------------|
| OpenAI response structure creation | gemini, qwen, claude | gemini:43-54, qwen:35-46, claude:~120-130 |
| ID generation | gemini, qwen | gemini:44, qwen:36 |
| Timestamp generation | gemini, qwen | gemini:46, qwen:38 |
| Empty usage initialization | gemini, qwen | gemini:49-53, qwen:41-45 |

### Example: Gemini Converter Response Creation
```go
// converter/gemini.go:43-54
openAIResp := map[string]interface{}{
    "id":      "chatcmpl-" + generateID(),
    "object":  "chat.completion",
    "created": getCurrentTimestamp(),
    "model":   model,
    "choices": []interface{}{},
    "usage": map[string]interface{}{
        "prompt_tokens":     0,
        "completion_tokens": 0,
        "total_tokens":      0,
    },
}
```

### Example: Qwen Converter Response Creation
```go
// converter/qwen.go:35-46
openAIResp := map[string]interface{}{
    "id":      fmt.Sprintf("chatcmpl-%s", model),
    "object":  "chat.completion",
    "created": time.Now().Unix(),
    "model":   model,
    "choices": []interface{}{},
    "usage": map[string]interface{}{
        "prompt_tokens":     0,
        "completion_tokens": 0,
        "total_tokens":      0,
    },
}
```

### Recommendation: Create Converter Helper Functions

Create `converter/helpers.go`:

```go
// Package converter provides helper functions for format conversion
package converter

import (
    "fmt"
    "time"
)

// CreateOpenAIResponseBase creates the base structure for an OpenAI response
func CreateOpenAIResponseBase(model string) map[string]interface{} {
    return map[string]interface{}{
        "id":      "chatcmpl-" + generateID(),
        "object":  "chat.completion",
        "created": getCurrentTimestamp(),
        "model":   model,
        "choices": []interface{}{},
        "usage": map[string]interface{}{
            "prompt_tokens":     0,
            "completion_tokens": 0,
            "total_tokens":      0,
        },
    }
}

// CreateOpenAIChoice creates a choice structure for OpenAI response
func CreateOpenAIChoice(index int, content string, finishReason string) map[string]interface{} {
    return map[string]interface{}{
        "index":         index,
        "message":       map[string]interface{}{"role": "assistant", "content": content},
        "finish_reason": finishReason,
    }
}

// CreateOpenAIChoiceWithToolCalls creates a choice with tool calls for OpenAI response
func CreateOpenAIChoiceWithToolCalls(index int, toolCalls []interface{}, finishReason string) map[string]interface{} {
    return map[string]interface{}{
        "index":         index,
        "message":       map[string]interface{}{"role": "assistant", "content": nil, "tool_calls": toolCalls},
        "finish_reason": finishReason,
    }
}

// UpdateUsage updates the usage field in OpenAI response
func UpdateUsage(resp map[string]interface{}, promptTokens, completionTokens, totalTokens int) {
    if resp["usage"] == nil {
        resp["usage"] = map[string]interface{}{}
    }
    usage := resp["usage"].(map[string]interface{})
    if promptTokens >= 0 {
        usage["prompt_tokens"] = promptTokens
    }
    if completionTokens >= 0 {
        usage["completion_tokens"] = completionTokens
    }
    if totalTokens >= 0 {
        usage["total_tokens"] = totalTokens
    }
}
```

### Code Cleanup Needed

Remove diagnostic `fmt.Printf` statements from [`converter/qwen.go:74-97`](converter/qwen.go:74-97):

```go
// REMOVE THESE DIAGNOSTIC STATEMENTS:
// converter/qwen.go:74-97
fmt.Printf("[QwenConverter DIAGNOSTIC] Raw Qwen response: %+v\n", qwenResp)
fmt.Printf("[QwenConverter DIAGNOSTIC] Choice %d: %+v\n", i, choiceMap)
fmt.Printf("[QwenConverter DIAGNOSTIC] Choice %d message: %+v\n", i, msgMap)
fmt.Printf("[QwenConverter DIAGNOSTIC] Tool calls found in choice %d: %+v\n", i, toolCalls)
fmt.Printf("[QwenConverter DIAGNOSTIC] No tool_calls found in choice %d message\n", i)
```

### Estimated Code Reduction
- **~30 lines** eliminated by creating helper functions
- **~25 lines** eliminated by removing diagnostic code
- **~45 lines** eliminated by refactoring converters to use helpers

---

## 4. Proxy Handlers - High Duplication

**Location**: [`proxy/gemini_handler.go`](proxy/gemini_handler.go:20), [`proxy/anthropic_handler.go`](proxy/anthropic_handler.go:20)

### Duplicate Patterns Identified

| Pattern | Duplicated In | Line References |
|---------|---------------|----------------|
| CORS headers | Both handlers | gemini:47-49, anthropic:47-49 |
| Token Manager field | Both handlers | gemini:23, anthropic:23 |
| Logger field | Both handlers | gemini:22, anthropic:22 |
| Constructor pattern (2 variants) | Both handlers | gemini:27-42, anthropic:27-42 |
| OPTIONS handling | Both handlers | gemini:51-54, anthropic:51-54 |
| handleListModels() | Both handlers | gemini:74-96, anthropic:73-94 |
| isProxyError() | Both handlers | Similar patterns |
| handleProxyError() | Both handlers | Similar patterns |

### Example: Gemini Handler Structure
```go
// proxy/gemini_handler.go:20-24
type GeminiHandler struct {
    provider     *gemini.Provider
    logger       *logging.Logger
    tokenManager *auth.TokenManager
}
```

### Example: Anthropic Handler Structure
```go
// proxy/anthropic_handler.go:20-24
type AnthropicHandler struct {
    provider     *kiro.Provider
    logger       *logging.Logger
    tokenManager *auth.TokenManager
}
```

### Example: CORS Headers (Identical in Both)
```go
// proxy/gemini_handler.go:47-49
w.Header().Set("Access-Control-Allow-Origin", "*")
w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

// proxy/anthropic_handler.go:47-49 (IDENTICAL)
w.Header().Set("Access-Control-Allow-Origin", "*")
w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
```

### Recommendation: Create BaseHandler

Create `proxy/base.go`:

```go
// Package proxy provides base structures for HTTP handlers
package proxy

import (
    "net/http"

    "github.com/sunbankio/qwencoder-proxy/auth"
    "github.com/sunbankio/qwencoder-proxy/logging"
)

// BaseHandler provides common fields and methods for HTTP handlers
type BaseHandler struct {
    logger       *logging.Logger
    tokenManager *auth.TokenManager
}

// NewBaseHandler creates a new base handler
func NewBaseHandler(tokenManager *auth.TokenManager) *BaseHandler {
    return &BaseHandler{
        logger:       logging.NewLogger(),
        tokenManager: tokenManager,
    }
}

// SetCORSHeaders sets CORS headers for the response
func (bh *BaseHandler) SetCORSHeaders(w http.ResponseWriter) {
    w.Header().Set("Access-Control-Allow-Origin", "*")
    w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
    w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
}

// HandleOptions handles OPTIONS requests for CORS preflight
func (bh *BaseHandler) HandleOptions(w http.ResponseWriter) {
    w.WriteHeader(http.StatusOK)
}

// GetLogger returns the logger
func (bh *BaseHandler) GetLogger() *logging.Logger {
    return bh.logger
}

// GetTokenManager returns the token manager
func (bh *BaseHandler) GetTokenManager() *auth.TokenManager {
    return bh.tokenManager
}

// IsProxyError checks if an error is a proxy-related error
func (bh *BaseHandler) IsProxyError(err error) bool {
    if err == nil {
        return false
    }
    // Check for common proxy error patterns
    errStr := err.Error()
    return contains(errStr, "proxy") || 
           contains(errStr, "connection refused") || 
           contains(errStr, "timeout") ||
           contains(errStr, "network")
}

// HandleProxyError handles proxy-related errors
func (bh *BaseHandler) HandleProxyError(w http.ResponseWriter, err error) {
    bh.logger.ErrorLog("[BaseHandler] Proxy error: %v", err)
    http.Error(w, "Proxy connection failed", http.StatusBadGateway)
}

func contains(s, substr string) bool {
    return len(s) >= len(substr) && (s == substr || 
           len(s) > len(substr) && (s[:len(substr)] == substr || 
           s[len(s)-len(substr):] == substr || 
           indexOfSubstring(s, substr) >= 0))
}

func indexOfSubstring(s, substr string) int {
    for i := 0; i <= len(s)-len(substr); i++ {
        if s[i:i+len(substr)] == substr {
            return i
        }
    }
    return -1
}
```

Then each handler can embed BaseHandler:

```go
// proxy/gemini_handler.go
type GeminiHandler struct {
    *BaseHandler  // Embedded base handler
    provider     *gemini.Provider
}

func NewGeminiHandler(p *gemini.Provider) *GeminiHandler {
    return &GeminiHandler{
        BaseHandler: NewBaseHandler(nil),
        provider:     p,
    }
}

func NewGeminiHandlerWithTokenManager(p *gemini.Provider, tokenManager *auth.TokenManager) *GeminiHandler {
    return &GeminiHandler{
        BaseHandler: NewBaseHandler(tokenManager),
        provider:     p,
    }
}
```

### Estimated Code Reduction
- **~30 lines** eliminated by creating BaseHandler
- **~70 lines** eliminated by refactoring handlers to use it

---

## 5. HTTP Client Management - Scattered

**Location**: Throughout codebase

### Current State

HTTP clients are created independently in multiple places with inconsistent timeouts:

| Location | Timeout | Line Reference |
|-----------|----------|----------------|
| Providers (gemini, kiro, iflow) | 5 minutes | gemini:56, kiro:73, iflow:62 |
| Authenticators (gemini, kiro) | 30 seconds | gemini:74, kiro:75 |
| REST API | Custom | restapi/rest_api.go |
| Proxy client factory | Centralized | config/proxy_client_factory.go:26 |

### Existing Good Pattern

[`config.ProxyAwareHTTPClientFactory`](config/proxy_client_factory.go:26) already implements a factory pattern with caching:

```go
// config/proxy_client_factory.go:26-33
type ProxyAwareHTTPClientFactory struct {
    baseConfig  HTTPClientConfig
    proxyCache  map[auth.ProxyConfigKey]*http.Client
    cacheLock   sync.RWMutex
    logger      *logging.Logger
    maxSize     int
    accessOrder []auth.ProxyConfigKey
}
```

### Recommendation: Standardize HTTP Client Usage

1. **Use ProxyAwareHTTPClientFactory consistently** across all providers and authenticators

2. **Create HTTPClientFactory interface** in `config/config.go`:

```go
// config/factory.go
package config

import (
    "net/http"

    "github.com/sunbankio/qwencoder-proxy/auth"
)

// HTTPClientFactory defines the interface for creating HTTP clients
type HTTPClientFactory interface {
    // GetClient returns an HTTP client for the given proxy configuration
    GetClient(proxyConfig *auth.ProxyConfig) *http.Client
    
    // GetDefaultClient returns a default HTTP client (no proxy)
    GetDefaultClient() *http.Client
    
    // GetClientWithTimeout returns an HTTP client with specific timeout
    GetClientWithTimeout(timeout time.Duration) *http.Client
}

// StandardHTTPClientFactory implements HTTPClientFactory
type StandardHTTPClientFactory struct {
    proxyFactory *ProxyAwareHTTPClientFactory
}

func NewStandardHTTPClientFactory(baseConfig HTTPClientConfig, logger *logging.Logger) *StandardHTTPClientFactory {
    return &StandardHTTPClientFactory{
        proxyFactory: NewProxyAwareHTTPClientFactory(baseConfig, logger, 50),
    }
}

func (f *StandardHTTPClientFactory) GetClient(proxyConfig *auth.ProxyConfig) *http.Client {
    return f.proxyFactory.GetClient(proxyConfig)
}

func (f *StandardHTTPClientFactory) GetDefaultClient() *http.Client {
    return f.GetClient(nil)
}

func (f *StandardHTTPClientFactory) GetClientWithTimeout(timeout time.Duration) *http.Client {
    return &http.Client{Timeout: timeout}
}
```

3. **Update BaseProvider and BaseAuthenticator** to use HTTPClientFactory:

```go
// provider/base.go
type BaseProvider struct {
    httpClient    config.HTTPClient  // Use interface instead of concrete type
    clientFactory config.HTTPClientFactory
    tokenManager  *auth.TokenManager
    logger        *logging.Logger
}

func NewBaseProvider(factory config.HTTPClientFactory) *BaseProvider {
    return &BaseProvider{
        clientFactory: factory,
        httpClient:    factory.GetDefaultClient(),
        logger:        logging.NewLogger(),
    }
}
```

### Estimated Code Reduction
- **~50 lines** eliminated by standardizing HTTP client creation
- Improved consistency and maintainability

---

## 6. Missing Factories

### Authenticator Factory Missing

**Current State**: No factory pattern for creating authenticators. Each provider directly instantiates its authenticator:

- [`provider/gemini/gemini.go:51`](provider/gemini/gemini.go:51): `auth.NewGeminiAuthenticator(nil)`
- [`provider/kiro/kiro.go:69`](provider/kiro/kiro.go:69): `auth.NewKiroAuthenticator(nil)`
- [`provider/iflow/iflow.go:57`](provider/iflow/iflow.go:57): `auth.NewIFlowAuthenticator(nil)`

### Recommendation: Create AuthenticatorFactory

Create `auth/factory.go`:

```go
// Package auth provides factory for creating authenticators
package auth

import (
    "github.com/sunbankio/qwencoder-proxy/config"
    "github.com/sunbankio/qwencoder-proxy/logging"
)

// AuthenticatorFactory creates authenticators for different providers
type AuthenticatorFactory struct {
    logger        *logging.Logger
    clientFactory config.HTTPClientFactory
}

// NewAuthenticatorFactory creates a new authenticator factory
func NewAuthenticatorFactory(logger *logging.Logger, clientFactory config.HTTPClientFactory) *AuthenticatorFactory {
    return &AuthenticatorFactory{
        logger:        logger,
        clientFactory: clientFactory,
    }
}

// CreateGeminiAuthenticator creates a new Gemini authenticator
func (f *AuthenticatorFactory) CreateGeminiAuthenticator(config *GeminiOAuthConfig) *GeminiAuthenticator {
    if config == nil {
        config = DefaultGeminiOAuthConfig()
    }
    return &GeminiAuthenticator{
        BaseAuthenticator: NewBaseAuthenticator(),
        config:           config,
    }
}

// CreateKiroAuthenticator creates a new Kiro authenticator
func (f *AuthenticatorFactory) CreateKiroAuthenticator(config *KiroOAuthConfig) *KiroAuthenticator {
    if config == nil {
        config = DefaultKiroOAuthConfig()
    }
    return &KiroAuthenticator{
        BaseAuthenticator: NewBaseAuthenticator(),
        config:           config,
    }
}

// CreateIFlowAuthenticator creates a new iFlow authenticator
func (f *AuthenticatorFactory) CreateIFlowAuthenticator(config *IFlowOAuthConfig) *IFlowAuthenticator {
    if config == nil {
        config = DefaultIFlowOAuthConfig()
    }
    return &IFlowAuthenticator{
        BaseAuthenticator: NewBaseAuthenticator(),
        config:           config,
    }
}
```

### Estimated Code Reduction
- **~30 lines** eliminated by centralizing authenticator creation
- Improved testability through dependency injection

---

## Prioritized Refactoring Recommendations

### Priority 1: High Impact, Low Risk

1. **Create BaseProvider struct** in `provider/base.go`
   - Eliminates ~40 lines of duplicate code across 5 providers
   - Simple embedding pattern, low risk
   - Estimated effort: 2-3 hours

2. **Create BaseAuthenticator struct** in `auth/base.go`
   - Eliminates ~50 lines of duplicate code across 3+ authenticators
   - Simple embedding pattern, low risk
   - Estimated effort: 2-3 hours

3. **Remove diagnostic code** from [`converter/qwen.go:74-97`](converter/qwen.go:74-97)
   - Eliminates ~25 lines of unnecessary code
   - Zero risk - just removal
   - Estimated effort: 5 minutes

4. **Create converter helper functions** in `converter/helpers.go`
   - Reduces duplication in OpenAI response creation
   - Simple function extraction, low risk
   - Estimated effort: 1-2 hours

### Priority 2: Medium Impact, Medium Risk

5. **Create BaseHandler struct** in `proxy/base.go`
   - Eliminates ~30 lines of duplicate code across 2+ handlers
   - Simple embedding pattern, medium risk
   - Estimated effort: 1-2 hours

6. **Create AuthenticatorFactory** in `auth/factory.go`
   - Improves testability and dependency injection
   - Medium risk - requires updating all providers
   - Estimated effort: 3-4 hours

7. **Standardize HTTP client usage** using [`config.ProxyAwareHTTPClientFactory`](config/proxy_client_factory.go:26)
   - Eliminates ~50 lines of scattered HTTP client creation
   - Medium risk - requires careful testing of timeout behavior
   - Estimated effort: 4-5 hours

### Priority 3: Lower Priority, Long-term Improvements

8. **Extract common provider initialization logic** into factory methods
   - Reduces duplication in provider constructors
   - Estimated effort: 2-3 hours

9. **Create shared error handling utilities** for proxy errors
   - Consolidates error handling across handlers
   - Estimated effort: 1-2 hours

10. **Consolidate credentials path logic** across authenticators
    - Reduces duplication in path generation
    - Estimated effort: 1 hour

---

## Code Reduction Estimates

| Component | Current Lines | After Refactoring | Reduction | % Reduction |
|-----------|---------------|-------------------|------------|--------------|
| Providers | ~1,500 | ~1,300 | ~200 lines | 13% |
| Authenticators | ~1,200 | ~950 | ~250 lines | 21% |
| Converters | ~900 | ~800 | ~100 lines | 11% |
| Handlers | ~800 | ~700 | ~100 lines | 12% |
| **Total** | **~4,400** | **~3,750** | **~650 lines** | **15%** |

---

## Architectural Improvements

### Interface Segregation

The [`Provider`](provider/provider.go:34) interface has 8 methods, which was noted in [`SOLID analysis`](plans/solid-principles-architectural-analysis.md:150). Consider splitting:

```go
// provider/provider.go

// Provider defines the core provider interface
type Provider interface {
    Name() ProviderType
    Protocol() ProtocolType
    SupportedModels() []string
    SupportsModel(model string) bool
    GetAuthenticator() Authenticator
    IsHealthy(ctx context.Context) bool
}

// ContentGenerator handles content generation operations
type ContentGenerator interface {
    GenerateContent(ctx context.Context, model string, request interface{}) (interface{}, error)
    GenerateContentStream(ctx context.Context, model string, request interface{}) (io.ReadCloser, error)
}

// ModelLister handles model listing operations
type ModelLister interface {
    ListModels(ctx context.Context) (interface{}, error)
}

// FullProvider combines all provider interfaces
type FullProvider interface {
    Provider
    ContentGenerator
    ModelLister
}
```

### Dependency Inversion

Create interfaces for:

1. **Logger Interface** (currently concrete type):

```go
// logging/logger.go
type Logger interface {
    InfoLog(format string, args ...interface{})
    DebugLog(format string, args ...interface{})
    ErrorLog(format string, args ...interface{})
    WarnLog(format string, args ...interface{})
}
```

2. **TokenStore Interface** (currently concrete [`MultiTokenStore`](auth/multi_token_store.go:44)):

```go
// auth/store.go
type TokenStore interface {
    Load() (map[string]TokenMetadata, error)
    Save(tokens map[string]TokenMetadata) error
    GetCredentialsPath() string
}
```

3. **Use existing HTTPClient interface** from [`config/config.go:25`](config/config.go:25):

The interface already exists but is not used consistently throughout the codebase.

---

## Specific File Locations for Refactoring

| Refactoring | New File | Affected Files |
|-------------|-----------|----------------|
| BaseProvider | `provider/base.go` | gemini/gemini.go, qwen/qwen.go, kiro/kiro.go, iflow/iflow.go, antigravity/antigravity.go |
| BaseAuthenticator | `auth/base.go` | gemini_auth.go, kiro_auth.go, iflow_auth.go |
| BaseHandler | `proxy/base.go` | gemini_handler.go, anthropic_handler.go |
| Converter Helpers | `converter/helpers.go` | gemini.go, qwen.go, claude.go |
| AuthenticatorFactory | `auth/factory.go` | All provider files |
| HTTPClientFactory Interface | `config/factory.go` | All files creating http.Client |
| Logger Interface | `logging/interface.go` | All files using logger |
| TokenStore Interface | `auth/interface.go` | multi_token_store.go, multi_token_manager.go |

---

## Implementation Plan

### Phase 1: Base Structures (Priority 1)
1. Create `provider/base.go` with BaseProvider
2. Create `auth/base.go` with BaseAuthenticator
3. Create `proxy/base.go` with BaseHandler
4. Update all providers to embed BaseProvider
5. Update all authenticators to embed BaseAuthenticator
6. Update all handlers to embed BaseHandler

### Phase 2: Helper Functions (Priority 1)
1. Create `converter/helpers.go` with helper functions
2. Remove diagnostic code from `converter/qwen.go`
3. Update all converters to use helper functions

### Phase 3: Factory Patterns (Priority 2)
1. Create `auth/factory.go` with AuthenticatorFactory
2. Create `config/factory.go` with HTTPClientFactory interface
3. Update all providers to use AuthenticatorFactory
4. Update BaseProvider and BaseAuthenticator to use HTTPClientFactory

### Phase 4: Interface Improvements (Priority 3)
1. Split Provider interface into smaller interfaces
2. Create Logger interface
3. Create TokenStore interface
4. Update code to use interfaces instead of concrete types

---

## Testing Strategy

### Unit Tests
- Create tests for BaseProvider, BaseAuthenticator, BaseHandler
- Create tests for converter helper functions
- Create tests for factory methods

### Integration Tests
- Test that providers still work correctly after refactoring
- Test that authenticators still work correctly after refactoring
- Test that handlers still work correctly after refactoring

### Regression Tests
- Ensure all existing tests still pass
- Ensure API compatibility is maintained
- Ensure no breaking changes to external interfaces

---

## Conclusion

The qwencoder-proxy project has significant code duplication that can be reduced by approximately **15% (650 lines)** through the creation of base structs, helper functions, and factory patterns.

The existing [`ProxyAwareHTTPClientFactory`](config/proxy_client_factory.go:26) demonstrates good factory pattern usage that should be extended to other components.

The refactoring recommendations align with findings in [`SOLID principles analysis`](plans/solid-principles-architectural-analysis.md:1) and would significantly improve:
- **Maintainability**: Less code to maintain, single source of truth
- **Testability**: Base structures are easier to test, dependency injection
- **Consistency**: Uniform patterns across components
- **SOLID Principles**: Better adherence to Single Responsibility, DRY, and Dependency Inversion

---

**Analysis completed**: 2026-02-08  
**Total lines analyzed**: ~4,400  
**Estimated reduction**: ~650 lines (15%)
