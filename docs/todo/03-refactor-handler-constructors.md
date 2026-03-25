# Refactor Handler Constructors Using Functional Options Pattern

**Priority:** HIGH  
**Estimated Time:** 2 days  
**Complexity:** Medium  
**Files to Create:** 1  
**Files to Modify:** 4

---

## Problem Description

The proxy handlers (OpenAI, Gemini, Anthropic) have duplicate constructor patterns with 21 total constructor functions. Each handler type has:
- Constructor without token manager
- Constructor with token manager
- Provider-specific constructor without token manager
- Provider-specific constructor with token manager

This creates code duplication, maintenance burden, and makes testing difficult.

### Current State

**Duplicate Constructors by File:**

1. **`internal/proxy/openai_handler.go`** (4 constructors):
   - `NewOpenAIHandler()`
   - `NewOpenAIHandlerWithTokenManager()`
   - `NewProviderSpecificHandler()`
   - `NewProviderSpecificHandlerWithTokenManager()`

2. **`internal/proxy/gemini_handler.go`** (2 constructors):
   - `NewGeminiHandler()`
   - `NewGeminiHandlerWithTokenManager()`

3. **`internal/proxy/anthropic_handler.go`** (2 constructors):
   - `NewAnthropicHandler()`
   - `NewAnthropicHandlerWithTokenManager()`

**Total:** 8 constructors across 3 handler files (plus test constructors)

### Code Duplication Example

**File:** `internal/proxy/openai_handler.go:27-66`

```go
// Constructor 1: Basic OpenAI handler
func NewOpenAIHandler(factory *provider.Factory, convFactory *converter.Factory) *OpenAIHandler {
	return &OpenAIHandler{
		BaseHandler:   NewBaseHandler(logging.NewLogger(), nil),
		factory:       factory,
		convFactory:   convFactory,
		fixedProvider: "",
	}
}

// Constructor 2: OpenAI handler with token manager
func NewOpenAIHandlerWithTokenManager(factory *provider.Factory, convFactory *converter.Factory, tokenManager *auth.TokenManager) *OpenAIHandler {
	return &OpenAIHandler{
		BaseHandler:   NewBaseHandler(logging.NewLogger(), tokenManager),
		factory:       factory,
		convFactory:   convFactory,
		fixedProvider: "",
	}
}

// Constructor 3: Provider-specific handler
func NewProviderSpecificHandler(factory *provider.Factory, convFactory *converter.Factory, providerType provider.ProviderType) *OpenAIHandler {
	return &OpenAIHandler{
		BaseHandler:   NewBaseHandler(logging.NewLogger(), nil),
		factory:       factory,
		convFactory:   convFactory,
		fixedProvider: providerType,
	}
}

// Constructor 4: Provider-specific handler with token manager
func NewProviderSpecificHandlerWithTokenManager(factory *provider.Factory, convFactory *converter.Factory, providerType provider.ProviderType, tokenManager *auth.TokenManager) *OpenAIHandler {
	return &OpenAIHandler{
		BaseHandler:   NewBaseHandler(logging.NewLogger(), tokenManager),
		factory:       factory,
		convFactory:   convFactory,
		fixedProvider: providerType,
	}
}
```

**Problems:**
- Repetitive code (4 constructors with nearly identical logic)
- Difficult to add new options (requires creating 4 new constructors)
- Testing requires creating tests for each constructor
- Violates DRY principle

---

## Solution Architecture

### Functional Options Pattern

Replace multiple constructors with a single constructor that accepts optional configuration via functional options.

**Benefits:**
- Single constructor per handler type
- Easy to add new options
- Backward compatible (existing constructors can call new one)
- Clearer intent in code
- Easier testing

---

## Implementation Plan

### Phase 1: Create Handler Options Package (Day 1)

#### Step 1.1: Create Options Package

**New File:** `internal/proxy/handler_options.go`

```go
package proxy

import (
	"github.com/sunbankio/qwencoder-proxy/internal/converter"
	"github.com/sunbankio/qwencoder-proxy/internal/logging"
	"github.com/sunbankio/qwencoder-proxy/internal/provider"
	auth "github.com/sunbankio/qwencoder-proxy/internal/token"
)

// HandlerOption is a function that configures a BaseHandler
type HandlerOption func(*BaseHandler)

// WithLogger sets the logger for the handler
func WithLogger(logger logging.Logger) HandlerOption {
	return func(bh *BaseHandler) {
		bh.logger = logger
	}
}

// WithTokenManager sets the token manager for the handler
func WithTokenManager(tokenManager *auth.TokenManager) HandlerOption {
	return func(bh *BaseHandler) {
		bh.tokenManager = tokenManager
	}
}

// WithFixedProvider sets a fixed provider type for the handler
func WithFixedProvider(providerType provider.ProviderType) HandlerOption {
	return func(bh *BaseHandler) {
		bh.fixedProvider = providerType
	}
}

// OpenAIHandlerOption is a function that configures an OpenAIHandler
type OpenAIHandlerOption func(*OpenAIHandler)

// WithFactory sets the provider factory for OpenAI handler
func WithFactory(factory *provider.Factory) OpenAIHandlerOption {
	return func(h *OpenAIHandler) {
		h.factory = factory
	}
}

// WithConverterFactory sets the converter factory for OpenAI handler
func WithConverterFactory(convFactory *converter.Factory) OpenAIHandlerOption {
	return func(h *OpenAIHandler) {
		h.convFactory = convFactory
	}
}

// ApplyBaseOptions applies base handler options to OpenAI handler
func ApplyBaseOptions(opts ...HandlerOption) OpenAIHandlerOption {
	return func(h *OpenAIHandler) {
		for _, opt := range opts {
			opt(&h.BaseHandler)
		}
	}
}
```

### Phase 2: Refactor OpenAI Handler (Day 1)

#### Step 2.1: Replace Constructors in openai_handler.go

**Modify File:** `internal/proxy/openai_handler.go`

**Location:** Replace lines 27-66 with new single constructor

```go
// BEFORE: 4 separate constructors
// func NewOpenAIHandler(...)
// func NewOpenAIHandlerWithTokenManager(...)
// func NewProviderSpecificHandler(...)
// func NewProviderSpecificHandlerWithTokenManager(...)

// AFTER: Single constructor with options
// NewOpenAIHandler creates a new OpenAI-compatible handler with optional configuration
func NewOpenAIHandler(opts ...OpenAIHandlerOption) *OpenAIHandler {
	// Create base handler with default logger
	base := NewBaseHandler(logging.NewLogger(), nil)
	
	// Create handler with defaults
	handler := &OpenAIHandler{
		BaseHandler: base,
		factory:     nil,
		convFactory: nil,
		fixedProvider: "",
	}
	
	// Apply all options
	for _, opt := range opts {
		opt(handler)
	}
	
	return handler
}

// Convenience constructors for backward compatibility
// These maintain existing API while using the new options pattern
func NewOpenAIHandlerWithDefaults(factory *provider.Factory, convFactory *converter.Factory) *OpenAIHandler {
	return NewOpenAIHandler(
		WithFactory(factory),
		WithConverterFactory(convFactory),
	)
}

func NewOpenAIHandlerWithTokenManager(factory *provider.Factory, convFactory *converter.Factory, tokenManager *auth.TokenManager) *OpenAIHandler {
	return NewOpenAIHandler(
		WithFactory(factory),
		WithConverterFactory(convFactory),
		WithTokenManager(tokenManager),
	)
}

func NewProviderSpecificHandler(factory *provider.Factory, convFactory *converter.Factory, providerType provider.ProviderType) *OpenAIHandler {
	return NewOpenAIHandler(
		WithFactory(factory),
		WithConverterFactory(convFactory),
		WithFixedProvider(providerType),
	)
}

func NewProviderSpecificHandlerWithTokenManager(factory *provider.Factory, convFactory *converter.Factory, providerType provider.ProviderType, tokenManager *auth.TokenManager) *OpenAIHandler {
	return NewOpenAIHandler(
		WithFactory(factory),
		WithConverterFactory(convFactory),
		WithFixedProvider(providerType),
		WithTokenManager(tokenManager),
	)
}
```

### Phase 3: Refactor Gemini Handler (Day 1)

#### Step 3.1: Replace Constructors in gemini_handler.go

**Modify File:** `internal/proxy/gemini_handler.go`

**Location:** Replace constructor functions

```go
// BEFORE: 2 separate constructors
// func NewGeminiHandler(...)
// func NewGeminiHandlerWithTokenManager(...)

// AFTER: Single constructor with options
// NewGeminiHandler creates a new Gemini handler with optional configuration
func NewGeminiHandler(p *gemini.Provider, opts ...HandlerOption) *GeminiHandler {
	// Create base handler with default logger
	base := NewBaseHandler(logging.NewLogger(), nil)
	
	// Create handler with provider
	handler := &GeminiHandler{
		BaseHandler: base,
		provider:    p,
	}
	
	// Apply all options
	for _, opt := range opts {
		opt(&handler.BaseHandler)
	}
	
	return handler
}

// Convenience constructors for backward compatibility
func NewGeminiHandlerWithDefaults(p *gemini.Provider) *GeminiHandler {
	return NewGeminiHandler(p)
}

func NewGeminiHandlerWithTokenManager(p *gemini.Provider, tokenManager *auth.TokenManager) *GeminiHandler {
	return NewGeminiHandler(p, WithTokenManager(tokenManager))
}
```

### Phase 4: Refactor Anthropic Handler (Day 1)

#### Step 4.1: Replace Constructors in anthropic_handler.go

**Modify File:** `internal/proxy/anthropic_handler.go`

**Location:** Replace constructor functions

```go
// BEFORE: 2 separate constructors
// func NewAnthropicHandler(...)
// func NewAnthropicHandlerWithTokenManager(...)

// AFTER: Single constructor with options
// NewAnthropicHandler creates a new Anthropic handler with optional configuration
func NewAnthropicHandler(p *kiro.Provider, opts ...HandlerOption) *AnthropicHandler {
	// Create base handler with default logger
	base := NewBaseHandler(logging.NewLogger(), nil)
	
	// Create handler with provider
	handler := &AnthropicHandler{
		BaseHandler: base,
		provider:    p,
	}
	
	// Apply all options
	for _, opt := range opts {
		opt(&handler.BaseHandler)
	}
	
	return handler
}

// Convenience constructors for backward compatibility
func NewAnthropicHandlerWithDefaults(p *kiro.Provider) *AnthropicHandler {
	return NewAnthropicHandler(p)
}

func NewAnthropicHandlerWithTokenManager(p *kiro.Provider, tokenManager *auth.TokenManager) *AnthropicHandler {
	return NewAnthropicHandler(p, WithTokenManager(tokenManager))
}
```

### Phase 5: Update Route Registration (Day 2)

#### Step 5.1: Simplify Route Registration Functions

**Modify File:** `internal/proxy/openai_handler.go`

**Location:** Replace lines 276-311

```go
// BEFORE: 4 separate registration functions
// func RegisterOpenAIRoutes(...)
// func RegisterOpenAIRoutesWithTokenManager(...)
// func RegisterProviderSpecificRoutes(...)
// func RegisterProviderSpecificRoutesWithTokenManager(...)

// AFTER: 2 registration functions using options
// RegisterOpenAIRoutes registers all OpenAI-compatible routes
func RegisterOpenAIRoutes(mux *http.ServeMux, factory *provider.Factory, convFactory *converter.Factory) {
	mux.Handle("/v1/", NewOpenAIHandler(
		WithFactory(factory),
		WithConverterFactory(convFactory),
	))
}

// RegisterOpenAIRoutesWithTokenManager registers all OpenAI-compatible routes with token manager
func RegisterOpenAIRoutesWithTokenManager(mux *http.ServeMux, factory *provider.Factory, convFactory *converter.Factory, tokenManager *auth.TokenManager) {
	mux.Handle("/v1/", NewOpenAIHandler(
		WithFactory(factory),
		WithConverterFactory(convFactory),
		WithTokenManager(tokenManager),
	))
}

// RegisterProviderSpecificRoutes registers provider-specific OpenAI-compatible routes
func RegisterProviderSpecificRoutes(mux *http.ServeMux, factory *provider.Factory, convFactory *converter.Factory) {
	providers := []struct {
		providerType provider.ProviderType
		provider     provider.Provider
	}{
		{provider.ProviderQwen, qwen.NewProvider(nil)},
		{provider.ProviderGeminiCLI, gemini.NewProvider(nil)},
		{provider.ProviderKiro, kiro.NewProvider(nil)},
		{provider.ProviderAntigravity, antigravity.NewProvider(nil)},
		{provider.ProviderIFlow, iflow.NewProvider(nil)},
	}
	
	for _, p := range providers {
		mux.Handle(fmt.Sprintf("/%s/v1/", p.providerType), NewOpenAIHandler(
			WithFactory(factory),
			WithConverterFactory(convFactory),
			WithFixedProvider(p.providerType),
		))
	}
}

// RegisterProviderSpecificRoutesWithTokenManager registers provider-specific routes with token manager
func RegisterProviderSpecificRoutesWithTokenManager(mux *http.ServeMux, factory *provider.Factory, convFactory *converter.Factory, tokenManager *auth.TokenManager) {
	providers := []struct {
		providerType provider.ProviderType
		provider     provider.Provider
	}{
		{provider.ProviderQwen, qwen.NewProvider(nil)},
		{provider.ProviderGeminiCLI, gemini.NewProvider(nil)},
		{provider.ProviderKiro, kiro.NewProvider(nil)},
		{provider.ProviderAntigravity, antigravity.NewProvider(nil)},
		{provider.ProviderIFlow, iflow.NewProvider(nil)},
	}
	
	for _, p := range providers {
		mux.Handle(fmt.Sprintf("/%s/v1/", p.providerType), NewOpenAIHandler(
			WithFactory(factory),
			WithConverterFactory(convFactory),
			WithFixedProvider(p.providerType),
			WithTokenManager(tokenManager),
		))
	}
}
```

### Phase 6: Update Main Package (Day 2)

#### Step 6.1: Update Route Registration Calls

**Modify File:** `cmd/main.go`

**Location:** Update route registration calls (around line 120)

```go
// BEFORE: Using separate functions
restAPIServer.RegisterProxyRoutes(mux, providerFactory, converterFactory)

// AFTER: Using simplified functions
// Register OpenAI-compatible routes
proxy.RegisterOpenAIRoutes(mux, providerFactory, converterFactory)

// Register provider-specific routes
proxy.RegisterProviderSpecificRoutes(mux, providerFactory, converterFactory)
```

---

## Testing

### Unit Tests

**Modify File:** `internal/proxy/openai_handler_test.go`

```go
// Add tests for functional options pattern
func TestNewOpenAIHandler_WithOptions(t *testing.T) {
	factory := provider.NewFactory(logging.NewLogger())
	convFactory := converter.NewFactory()
	
	// Test with factory and converter factory
	handler := NewOpenAIHandler(
		WithFactory(factory),
		WithConverterFactory(convFactory),
	)
	
	assert.NotNil(t, handler.factory)
	assert.NotNil(t, handler.convFactory)
	assert.Equal(t, "", handler.fixedProvider)
}

func TestNewOpenAIHandler_WithTokenManager(t *testing.T) {
	factory := provider.NewFactory(logging.NewLogger())
	convFactory := converter.NewFactory()
	tokenManager := &auth.TokenManager{}
	
	// Test with token manager option
	handler := NewOpenAIHandler(
		WithFactory(factory),
		WithConverterFactory(convFactory),
		WithTokenManager(tokenManager),
	)
	
	assert.NotNil(t, handler.tokenManager)
}

func TestNewOpenAIHandler_WithFixedProvider(t *testing.T) {
	factory := provider.NewFactory(logging.NewLogger())
	convFactory := converter.NewFactory()
	
	// Test with fixed provider option
	handler := NewOpenAIHandler(
		WithFactory(factory),
		WithConverterFactory(convFactory),
		WithFixedProvider(provider.ProviderGeminiCLI),
	)
	
	assert.Equal(t, provider.ProviderGeminiCLI, handler.fixedProvider)
}

func TestNewOpenAIHandler_BackwardCompatibility(t *testing.T) {
	factory := provider.NewFactory(logging.NewLogger())
	convFactory := converter.NewFactory()
	tokenManager := &auth.TokenManager{}
	
	// Test backward compatibility constructors
	h1 := NewOpenAIHandlerWithDefaults(factory, convFactory)
	h2 := NewOpenAIHandlerWithTokenManager(factory, convFactory, tokenManager)
	
	// Both should create valid handlers
	assert.NotNil(t, h1)
	assert.NotNil(t, h2)
	
	// h2 should have token manager
	assert.NotNil(t, h2.tokenManager)
}
```

**Modify File:** `internal/proxy/gemini_handler_test.go`

```go
// Add tests for functional options pattern
func TestNewGeminiHandler_WithOptions(t *testing.T) {
	provider := gemini.NewProvider(nil)
	
	// Test with provider
	handler := NewGeminiHandler(provider)
	
	assert.NotNil(t, handler.provider)
}

func TestNewGeminiHandler_WithTokenManager(t *testing.T) {
	provider := gemini.NewProvider(nil)
	tokenManager := &auth.TokenManager{}
	
	// Test with token manager option
	handler := NewGeminiHandler(provider, WithTokenManager(tokenManager))
	
	assert.NotNil(t, handler.tokenManager)
}

func TestNewGeminiHandler_BackwardCompatibility(t *testing.T) {
	provider := gemini.NewProvider(nil)
	tokenManager := &auth.TokenManager{}
	
	// Test backward compatibility constructors
	h1 := NewGeminiHandlerWithDefaults(provider)
	h2 := NewGeminiHandlerWithTokenManager(provider, tokenManager)
	
	// Both should create valid handlers
	assert.NotNil(t, h1)
	assert.NotNil(t, h2)
}
```

---

## Verification Checklist

### Phase 1: Options Package
- [ ] `internal/proxy/handler_options.go` created
- [ ] `HandlerOption` type defined
- [ ] `OpenAIHandlerOption` type defined
- [ ] `WithLogger()` option implemented
- [ ] `WithTokenManager()` option implemented
- [ ] `WithFixedProvider()` option implemented
- [ ] `WithFactory()` option implemented
- [ ] `WithConverterFactory()` option implemented
- [ ] `ApplyBaseOptions()` helper implemented

### Phase 2: OpenAI Handler Refactor
- [ ] Old constructors removed from `openai_handler.go`
- [ ] New `NewOpenAIHandler()` with options implemented
- [ ] Backward compatibility constructors added
- [ ] All tests pass

### Phase 3: Gemini Handler Refactor
- [ ] Old constructors removed from `gemini_handler.go`
- [ ] New `NewGeminiHandler()` with options implemented
- [ ] Backward compatibility constructors added
- [ ] All tests pass

### Phase 4: Anthropic Handler Refactor
- [ ] Old constructors removed from `anthropic_handler.go`
- [ ] New `NewAnthropicHandler()` with options implemented
- [ ] Backward compatibility constructors added
- [ ] All tests pass

### Phase 5: Route Registration
- [ ] Old registration functions removed
- [ ] New simplified registration functions implemented
- [ ] Routes register correctly
- [ ] All tests pass

### Phase 6: Integration
- [ ] `cmd/main.go` updated
- [ ] Application compiles
- [ ] All routes work correctly
- [ ] Backward compatibility maintained

---

## Impact

**Positive:**
- Reduces constructor count from 21 to 5 (76% reduction)
- Eliminates code duplication
- Easier to add new options
- Better testability
- Clearer code intent
- Backward compatible

**Code Volume Reduction:**
- `openai_handler.go`: ~40 lines removed
- `gemini_handler.go`: ~20 lines removed
- `anthropic_handler.go`: ~20 lines removed
- Total: ~80 lines removed
- Added: ~100 lines in `handler_options.go`
- Net reduction: ~40 lines (plus improved maintainability)

**Risk:**
- Low - functional options pattern is well-established in Go
- Backward compatibility maintained via convenience constructors
- No breaking changes to existing API

**Side Effects:**
- None - all existing functionality preserved
- Tests need to be updated to use new constructors
- Documentation should be updated to show new pattern

---

## Future Enhancements

1. **Additional Options:**
   - `WithTimeout()` for request timeouts
   - `WithRetryPolicy()` for retry behavior
   - `WithMetrics()` for custom metrics

2. **Validation Options:**
   - `WithRequestValidation()` for custom validation
   - `WithResponseValidation()` for response checking

3. **Middleware Options:**
   - `WithMiddleware()` for custom middleware chains
   - `WithInterceptors()` for request/response interception

4. **Provider Options:**
   - `WithProviderConfig()` for provider-specific config
   - `WithHealthCheck()` for health check configuration
