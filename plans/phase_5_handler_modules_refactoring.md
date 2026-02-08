# Phase 5: Handler Modules Refactoring

**Phase**: 5 of 7  
**Estimated Time**: 1-2 days  
**Risk Level**: Low  
**Dependencies**: Phase 1 (Core interfaces and base structures)

---

## Phase Objectives

This phase refactors the handler implementations to use BaseHandler, eliminating code duplication and ensuring consistent behavior across all handlers.

**Primary Goals:**
1. Refactor GeminiHandler to embed BaseHandler
2. Refactor AnthropicHandler to embed BaseHandler
3. Refactor OpenAIHandler to embed BaseHandler
4. Remove duplicate fields and methods from each handler
5. Ensure backward compatibility with existing code
6. Maintain thread-safety for all operations

---

## Target Files to Modify

1. `proxy/gemini_handler.go` - Refactor to embed BaseHandler
2. `proxy/anthropic_handler.go` - Refactor to embed BaseHandler
3. `proxy/openai_handler.go` - Refactor to embed BaseHandler

---

## Step-by-Step Instructions

### Step 1: Refactor GeminiHandler

**File to Modify**: `proxy/gemini_handler.go`

**Current Structure** (lines 20-24):
```go
type GeminiHandler struct {
	provider     *gemini.Provider
	logger       *logging.Logger
	tokenManager *auth.TokenManager
}
```

**Refactored Structure**:
```go
type GeminiHandler struct {
	*BaseHandler  // Embedded base handler
	provider     *gemini.Provider
}
```

**Modification Steps**:

1. **Update struct definition** (around line 20):

```go
// GeminiHandler handles requests to /gemini/* routes
// Embeds BaseHandler for common functionality
type GeminiHandler struct {
	*BaseHandler  // Embedded base handler provides common fields and methods
	provider     *gemini.Provider
}
```

2. **Update NewGeminiHandler constructor** (around line 27):

```go
// NewGeminiHandler creates a new Gemini route handler
// Uses BaseHandler for common functionality
func NewGeminiHandler(p *gemini.Provider) *GeminiHandler {
	return &GeminiHandler{
		BaseHandler: NewBaseHandler(logging.NewLogger(), nil),
		provider:     p,
	}
}
```

3. **Update NewGeminiHandlerWithTokenManager constructor** (around line 36):

```go
// NewGeminiHandlerWithTokenManager creates a new Gemini route handler with token manager for proxy error handling
// Uses BaseHandler for common functionality
func NewGeminiHandlerWithTokenManager(p *gemini.Provider, tokenManager *auth.TokenManager) *GeminiHandler {
	return &GeminiHandler{
		BaseHandler: NewBaseHandler(logging.NewLogger(), tokenManager),
		provider:     p,
	}
}
```

4. **Remove duplicate fields**:

The following fields should be **removed** as they are now provided by BaseHandler:
- `logger` (line 22)
- `tokenManager` (line 23)

5. **Remove duplicate methods**:

The following methods should be **removed** as they are now provided by BaseHandler:
- `isProxyError()` (if present)
- `handleProxyError()` (if present)

6. **Update methods that access removed fields**:

Any method that accessed `h.logger` or `h.tokenManager` should now use the embedded BaseHandler methods:

```go
// Example: Update ServeHTTP method (around line 45)
// Before:
// h.logger.DebugLog("[Gemini Handler] Request: %s %s", r.Method, path)

// After:
// h.GetLogger().DebugLog("[Gemini Handler] Request: %s %s", r.Method, path)

// Example: Update handleListModels method (around line 75)
// Before:
// h.logger.ErrorLog("[Gemini Handler] Failed to list models: %v", err)
// if h.isProxyError(err) {
//     h.handleProxyError(w, err)
// }

// After:
// h.GetLogger().ErrorLog("[Gemini Handler] Failed to list models: %v", err)
// if h.IsProxyError(err) {
//     h.HandleProxyError(w, err)
// }
```

7. **Update ServeHTTP to use BaseHandler methods** (around line 45):

```go
// ServeHTTP handles HTTP requests for Gemini routes
func (h *GeminiHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Set CORS headers using BaseHandler method
	h.SetCORSHeaders(w)

	if r.Method == http.MethodOptions {
		h.HandleOptions(w)  // Use BaseHandler method
		return
	}

	// Parse the path: /gemini/models, /gemini/models/{model}:generateContent, etc.
	path := strings.TrimPrefix(r.URL.Path, "/gemini")
	path = strings.TrimPrefix(path, "/")

	h.GetLogger().DebugLog("[Gemini Handler] Request: %s %s", r.Method, path)

	switch {
	case path == "models" && r.Method == http.MethodGet:
		h.handleListModels(w, r)
	case strings.HasPrefix(path, "models/") && strings.Contains(path, ":generateContent"):
		h.handleGenerateContent(w, r, path)
	case strings.HasPrefix(path, "models/") && strings.Contains(path, ":streamGenerateContent"):
		h.handleStreamGenerateContent(w, r, path)
	default:
		http.Error(w, "Not found", http.StatusNotFound)
	}
}
```

8. **Keep handler-specific fields and methods**:

The following should be **kept** as they are specific to GeminiHandler:
- `provider` field (gemini.Provider)
- All handler-specific methods (handleListModels, handleGenerateContent, handleStreamGenerateContent, etc.)

**Edge Cases:**
- **Nil Provider**: NewGeminiHandler accepts a provider which could be nil. The calling code is responsible for providing a valid provider.
- **Nil TokenManager**: NewGeminiHandlerWithTokenManager accepts a tokenManager which could be nil. BaseHandler handles this gracefully.
- **CORS Headers**: SetCORSHeaders() centralizes CORS configuration to ensure consistency.

**Verification:**
- GeminiHandler embeds BaseHandler
- Duplicate fields (logger, tokenManager) are removed
- Duplicate methods (isProxyError, handleProxyError) are removed
- All remaining methods use BaseHandler methods for common operations
- Handler-specific fields and methods are preserved

---

### Step 2: Refactor AnthropicHandler

**File to Modify**: `proxy/anthropic_handler.go`

**Current Structure** (lines 20-24):
```go
type AnthropicHandler struct {
	provider     *kiro.Provider
	logger       *logging.Logger
	tokenManager *auth.TokenManager
}
```

**Refactored Structure**:
```go
type AnthropicHandler struct {
	*BaseHandler  // Embedded base handler
	provider     *kiro.Provider
}
```

**Modification Steps**:

1. **Update struct definition** (around line 20):

```go
// AnthropicHandler handles requests to /anthropic/* routes
// Embeds BaseHandler for common functionality
type AnthropicHandler struct {
	*BaseHandler  // Embedded base handler provides common fields and methods
	provider     *kiro.Provider
}
```

2. **Update NewAnthropicHandler constructor** (around line 27):

```go
// NewAnthropicHandler creates a new Anthropic route handler
// Uses BaseHandler for common functionality
func NewAnthropicHandler(p *kiro.Provider) *AnthropicHandler {
	return &AnthropicHandler{
		BaseHandler: NewBaseHandler(logging.NewLogger(), nil),
		provider:     p,
	}
}
```

3. **Update NewAnthropicHandlerWithTokenManager constructor** (around line 36):

```go
// NewAnthropicHandlerWithTokenManager creates a new Anthropic route handler with token manager for proxy error handling
// Uses BaseHandler for common functionality
func NewAnthropicHandlerWithTokenManager(p *kiro.Provider, tokenManager *auth.TokenManager) *AnthropicHandler {
	return &AnthropicHandler{
		BaseHandler: NewBaseHandler(logging.NewLogger(), tokenManager),
		provider:     p,
	}
}
```

4. **Remove duplicate fields**:

The following fields should be **removed** as they are now provided by BaseHandler:
- `logger` (line 22)
- `tokenManager` (line 23)

5. **Remove duplicate methods**:

The following methods should be **removed** as they are now provided by BaseHandler:
- `isProxyError()` (if present)
- `handleProxyError()` (if present)

6. **Update methods that access removed fields**:

Any method that accessed `h.logger` or `h.tokenManager` should now use the embedded BaseHandler methods:

```go
// Example: Update ServeHTTP method (around line 45)
// Before:
// h.logger.DebugLog("[Anthropic Handler] Request: %s %s", r.Method, path)

// After:
// h.GetLogger().DebugLog("[Anthropic Handler] Request: %s %s", r.Method, path)

// Example: Update handleListModels method (around line 73)
// Before:
// h.logger.ErrorLog("[Anthropic Handler] Failed to list models: %v", err)
// if h.isProxyError(err) {
//     h.handleProxyError(w, err)
// }

// After:
// h.GetLogger().ErrorLog("[Anthropic Handler] Failed to list models: %v", err)
// if h.IsProxyError(err) {
//     h.HandleProxyError(w, err)
// }
```

7. **Update ServeHTTP to use BaseHandler methods** (around line 45):

```go
// ServeHTTP handles HTTP requests for Anthropic routes
func (h *AnthropicHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Set CORS headers using BaseHandler method
	h.SetCORSHeaders(w)

	if r.Method == http.MethodOptions {
		h.HandleOptions(w)  // Use BaseHandler method
		return
	}

	// Parse the path: /anthropic/models, /anthropic/messages, etc.
	path := strings.TrimPrefix(r.URL.Path, "/anthropic")
	path = strings.TrimPrefix(path, "/")

	h.GetLogger().DebugLog("[Anthropic Handler] Request: %s %s", r.Method, path)

	switch {
	case path == "models" && r.Method == http.MethodGet:
		h.handleListModels(w, r)
	case path == "messages" && r.Method == http.MethodPost:
		h.handleMessages(w, r)
	default:
		http.Error(w, "Not found", http.StatusNotFound)
	}
}
```

8. **Keep handler-specific fields and methods**:

The following should be **kept** as they are specific to AnthropicHandler:
- `provider` field (kiro.Provider)
- All handler-specific methods (handleListModels, handleMessages, etc.)

**Edge Cases:**
- **Nil Provider**: NewAnthropicHandler accepts a provider which could be nil. The calling code is responsible for providing a valid provider.
- **Nil TokenManager**: NewAnthropicHandlerWithTokenManager accepts a tokenManager which could be nil. BaseHandler handles this gracefully.
- **CORS Headers**: SetCORSHeaders() centralizes CORS configuration to ensure consistency.

**Verification:**
- AnthropicHandler embeds BaseHandler
- Duplicate fields (logger, tokenManager) are removed
- Duplicate methods (isProxyError, handleProxyError) are removed
- All remaining methods use BaseHandler methods for common operations
- Handler-specific fields and methods are preserved

---

### Step 3: Refactor OpenAIHandler

**File to Modify**: `proxy/openai_handler.go`

**Current Structure** (lines 20-26):
```go
type OpenAIHandler struct {
	factory       *provider.Factory
	convFactory   *converter.Factory
	logger        *logging.Logger
	fixedProvider provider.ProviderType
	tokenManager  *auth.TokenManager
}
```

**Refactored Structure**:
```go
type OpenAIHandler struct {
	*BaseHandler  // Embedded base handler
	factory       *provider.Factory
	convFactory   *converter.Factory
	fixedProvider provider.ProviderType
}
```

**Modification Steps**:

1. **Update struct definition** (around line 20):

```go
// OpenAIHandler handles OpenAI-compatible requests and routes them to appropriate providers
// Embeds BaseHandler for common functionality
type OpenAIHandler struct {
	*BaseHandler  // Embedded base handler provides common fields and methods
	factory       *provider.Factory
	convFactory   *converter.Factory
	fixedProvider provider.ProviderType  // If set, always use this provider
}
```

2. **Update NewOpenAIHandler constructor** (around line 29):

```go
// NewOpenAIHandler creates a new OpenAI-compatible handler
// Uses BaseHandler for common functionality
func NewOpenAIHandler(factory *provider.Factory, convFactory *converter.Factory) *OpenAIHandler {
	return &OpenAIHandler{
		BaseHandler:  NewBaseHandler(logging.NewLogger(), nil),
		factory:      factory,
		convFactory:  convFactory,
		fixedProvider: "",
	}
}
```

3. **Update NewOpenAIHandlerWithTokenManager constructor** (around line 38):

```go
// NewOpenAIHandlerWithTokenManager creates a new OpenAI-compatible handler with token manager for proxy error handling
// Uses BaseHandler for common functionality
func NewOpenAIHandlerWithTokenManager(factory *provider.Factory, convFactory *converter.Factory, tokenManager *auth.TokenManager) *OpenAIHandler {
	return &OpenAIHandler{
		BaseHandler:  NewBaseHandler(logging.NewLogger(), tokenManager),
		factory:      factory,
		convFactory:  convFactory,
		fixedProvider: "",
	}
}
```

4. **Update NewProviderSpecificHandler constructor** (around line 48):

```go
// NewProviderSpecificHandler creates a new handler that forces requests to use a specific provider
// Uses BaseHandler for common functionality
func NewProviderSpecificHandler(factory *provider.Factory, convFactory *converter.Factory, providerType provider.ProviderType) *OpenAIHandler {
	return &OpenAIHandler{
		BaseHandler:  NewBaseHandler(logging.NewLogger(), nil),
		factory:       factory,
		convFactory:   convFactory,
		fixedProvider: providerType,
	}
}
```

5. **Update NewProviderSpecificHandlerWithTokenManager constructor** (around line 58):

```go
// NewProviderSpecificHandlerWithTokenManager creates a new handler that forces requests to use a specific provider with token manager
// Uses BaseHandler for common functionality
func NewProviderSpecificHandlerWithTokenManager(factory *provider.Factory, convFactory *converter.Factory, providerType provider.ProviderType, tokenManager *auth.TokenManager) *OpenAIHandler {
	return &OpenAIHandler{
		BaseHandler:  NewBaseHandler(logging.NewLogger(), tokenManager),
		factory:       factory,
		convFactory:   convFactory,
		fixedProvider: providerType,
	}
}
```

6. **Remove duplicate fields**:

The following fields should be **removed** as they are now provided by BaseHandler:
- `logger` (line 23)
- `tokenManager` (line 25)

7. **Remove duplicate methods**:

The following methods should be **removed** as they are now provided by BaseHandler:
- `isProxyError()` (if present)
- `handleProxyError()` (if present)

8. **Update methods that access removed fields**:

Any method that accessed `h.logger` or `h.tokenManager` should now use the embedded BaseHandler methods:

```go
// Example: Update ServeHTTP method (around line 69)
// Before:
// h.logger.DebugLog("[Handler] Original URL.Path: %s, Stripped path: %s, Method: %s (Fixed Provider: %s)", originalPath, path, r.Method, h.fixedProvider)

// After:
// h.GetLogger().DebugLog("[Handler] Original URL.Path: %s, Stripped path: %s, Method: %s (Fixed Provider: %s)", originalPath, path, r.Method, h.fixedProvider)

// Example: Update any method that needs tokenManager
// Before:
// token := h.tokenManager

// After:
// token := h.GetTokenManager()
```

9. **Update ServeHTTP to use BaseHandler methods** (around line 69):

```go
// ServeHTTP handles HTTP requests for OpenAI-compatible routes
func (h *OpenAIHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Set CORS headers using BaseHandler method
	h.SetCORSHeaders(w)

	if r.Method == http.MethodOptions {
		h.HandleOptions(w)  // Use BaseHandler method
		return
	}

	// Determine the path by stripping known prefixes
	originalPath := r.URL.Path
	path := originalPath
	prefixes := []string{"/v1", "/qwen/v1", "/gemini/v1", "/kiro/v1", "/antigravity/v1", "/iflow/v1"}
	for _, prefix := range prefixes {
		if strings.HasPrefix(path, prefix) {
			path = strings.TrimPrefix(path, prefix)
			break
		}
	}
	path = strings.TrimPrefix(path, "/")

	h.GetLogger().DebugLog("[Handler] Original URL.Path: %s, Stripped path: %s, Method: %s (Fixed Provider: %s)", originalPath, path, r.Method, h.fixedProvider)

	switch {
	case path == "models" && r.Method == http.MethodGet:
		h.handleListModels(w, r)
	case path == "chat/completions" && r.Method == http.MethodPost:
		h.handleChatCompletions(w, r)
	default:
		h.GetLogger().ErrorLog("[Handler] No match found - path: '%s', method: '%s', expecting 'models' (GET) or 'chat/completions' (POST)", path, r.Method)
		http.Error(w, "Not found", http.StatusNotFound)
	}
}
```

10. **Keep handler-specific fields and methods**:

The following should be **kept** as they are specific to OpenAIHandler:
- `factory` field (provider.Factory)
- `convFactory` field (converter.Factory)
- `fixedProvider` field (provider.ProviderType)
- All handler-specific methods (handleListModels, handleChatCompletions, etc.)

**Edge Cases:**
- **Nil Factory**: NewOpenAIHandler accepts a factory which could be nil. The calling code is responsible for providing a valid factory.
- **Nil TokenManager**: NewOpenAIHandlerWithTokenManager accepts a tokenManager which could be nil. BaseHandler handles this gracefully.
- **Fixed Provider**: The `fixedProvider` field is preserved as it's specific to OpenAIHandler's routing logic.
- **CORS Headers**: SetCORSHeaders() centralizes CORS configuration to ensure consistency.

**Verification:**
- OpenAIHandler embeds BaseHandler
- Duplicate fields (logger, tokenManager) are removed
- Duplicate methods (isProxyError, handleProxyError) are removed
- All remaining methods use BaseHandler methods for common operations
- Handler-specific fields and methods are preserved

---

## Build Verification

After completing all steps, verify the code builds successfully:

```bash
cd c:/Users/jappa/projecten/qwencoder-proxy/qwencoder-proxy
go build ./...
```

**Expected Output**: No build errors

**If Build Fails**:
1. Check for missing imports (removed sync package if no longer needed)
2. Verify all removed fields are no longer accessed
3. Ensure all method calls to embedded fields use correct syntax
4. Check for circular dependencies

---

## Test Verification

After successful build, run existing tests to ensure no regressions:

```bash
go test ./proxy/... -v
```

**Expected Output**: All existing tests pass

**Note**: The refactored handlers should behave identically to the original implementations. All tests should pass without modification.

---

## API Compatibility Verification

This phase modifies the internal structure of handlers but should maintain backward compatibility with existing code.

**Verification Steps**:
1. All public methods of each handler are preserved
2. Method signatures are unchanged
3. Handler-specific fields are preserved
4. No changes to the http.Handler interface

**Backward Compatibility Checklist**:
- [x] GeminiHandler still implements http.Handler interface
- [x] AnthropicHandler still implements http.Handler interface
- [x] OpenAIHandler still implements http.Handler interface
- [x] All public methods are preserved
- [x] Method signatures are unchanged

---

## Concurrency Considerations

### Thread Safety After Refactoring

1. **BaseHandler**: Provides thread-safe access to tokenManager via GetTokenManager(). This is preserved after refactoring.

2. **Handler-Specific Fields**: Fields like `factory`, `convFactory`, `fixedProvider`, and `provider` are typically read-only after construction. If they are modified, each handler should add its own mutex protection.

3. **Method Calls**: Methods that previously accessed `h.tokenManager` now use `h.GetTokenManager()` which returns the tokenManager. This maintains thread safety.

### Concurrency Edge Cases

1. **Concurrent Request Handling**: Multiple goroutines can call ServeHTTP() simultaneously. Each request is independent, so no shared state exists. Thread-safe by design.

2. **Concurrent Proxy Error Handling**: Multiple goroutines can call HandleProxyError() simultaneously. BaseHandler's HandleProxyError() uses GetLogger() which returns the immutable logger. Thread-safe by design.

3. **Concurrent Token Manager Access**: Multiple goroutines can access GetTokenManager() simultaneously. BaseHandler's GetTokenManager() returns the tokenManager which is set once during construction. Thread-safe by design.

---

## Phase Completion Criteria

Phase 5 is complete when:

- [x] `proxy/gemini_handler.go` refactored to embed BaseHandler
- [x] `proxy/anthropic_handler.go` refactored to embed BaseHandler
- [x] `proxy/openai_handler.go` refactored to embed BaseHandler
- [x] Duplicate fields removed from all handlers
- [x] Duplicate methods removed from all handlers
- [x] All remaining methods use BaseHandler methods for common operations
- [x] CORS headers are centralized in BaseHandler
- [x] Code builds successfully with `go build ./...`
- [x] All existing tests pass
- [x] API compatibility is maintained

---

## Next Phase

Proceed to **Phase 6: Converter Modules Refactoring** after Phase 5 is complete.

Phase 6 will refactor the converter implementations to use helper functions, eliminating code duplication and ensuring consistent behavior across all converters.
