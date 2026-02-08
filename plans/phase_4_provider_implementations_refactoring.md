# Phase 4: Provider Implementations Refactoring

**Phase**: 4 of 7  
**Estimated Time**: 2-3 days  
**Risk Level**: Medium  
**Dependencies**: Phase 1 (Core interfaces and base structures), Phase 2 (Factory patterns), Phase 3 (Authentication modules refactoring)

---

## Phase Objectives

This phase refactors the provider implementations to use BaseProvider and AuthenticatorFactory, eliminating code duplication and enabling dependency injection.

**Primary Goals:**
1. Refactor Gemini Provider to embed BaseProvider
2. Refactor Qwen Provider to embed BaseProvider
3. Refactor Kiro Provider to embed BaseProvider
4. Refactor iFlow Provider to embed BaseProvider
5. Refactor Antigravity Provider to embed BaseProvider
6. Replace direct authenticator instantiation with AuthenticatorFactory
7. Remove duplicate fields and methods from each provider
8. Ensure backward compatibility with existing code

---

## Target Files to Modify

1. `provider/gemini/gemini.go` - Refactor to embed BaseProvider
2. `provider/qwen/qwen.go` - Refactor to embed BaseProvider
3. `provider/kiro/kiro.go` - Refactor to embed BaseProvider
4. `provider/iflow/iflow.go` - Refactor to embed BaseProvider
5. `provider/antigravity/antigravity.go` - Refactor to embed BaseProvider

---

## Step-by-Step Instructions

### Step 1: Refactor Gemini Provider

**File to Modify**: `provider/gemini/gemini.go`

**Current Structure** (lines 38-46):
```go
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

**Refactored Structure**:
```go
type Provider struct {
	*provider.BaseProvider  // Embedded base provider
	baseURL          string
	authenticator    *auth.GeminiAuthenticator
	projectID        string
	projectInitError error
}
```

**Modification Steps**:

1. **Update struct definition** (around line 38):

```go
// Provider implements the provider.Provider interface for Gemini CLI
// Embeds BaseProvider for common functionality
type Provider struct {
	*provider.BaseProvider  // Embedded base provider provides common fields and methods
	baseURL          string
	authenticator    *auth.GeminiAuthenticator
	projectID        string
	projectInitError error  // Store any initialization error to prevent repeated attempts
}
```

2. **Update NewProvider constructor** (around line 49):

```go
// NewProvider creates a new Gemini provider
// Uses BaseProvider for common functionality
func NewProvider(authenticator *auth.GeminiAuthenticator) *Provider {
	if authenticator == nil {
		// Use direct instantiation for now - will be updated to use factory in later phase
		authenticator = auth.NewGeminiAuthenticator(nil)
	}
	return &Provider{
		BaseProvider: provider.NewBaseProvider(logging.NewLogger(), 5*time.Minute),
		baseURL:     DefaultBaseURL,
		authenticator: authenticator,
	}
}
```

3. **Remove duplicate fields**:

The following fields should be **removed** as they are now provided by BaseProvider:
- `httpClient` (line 41)
- `tokenManager` (line 42)
- `logger` (line 43)

4. **Remove duplicate methods**:

The following method should be **removed** as it is now provided by BaseProvider:
- `SetTokenManager()` (around lines 96-98)

5. **Update methods that access removed fields**:

Any method that accessed `p.httpClient`, `p.tokenManager`, or `p.logger` should now use the embedded BaseProvider methods:

```go
// Example: Update GenerateContent method (around line 150)
// Before:
// client := p.httpClient

// After:
// client := p.GetHTTPClient()

// Example: Update any method that needs tokenManager
// Before:
// token := p.tokenManager

// After:
// token := p.GetTokenManager()

// Example: Update any method that needs logger
// Before:
// p.logger.DebugLog(...)

// After:
// p.GetLogger().DebugLog(...)
```

6. **Keep provider-specific fields and methods**:

The following should be **kept** as they are specific to Gemini Provider:
- `baseURL` field
- `authenticator` field
- `projectID` field
- `projectInitError` field
- All provider-specific methods (GenerateContent, GenerateContentStream, ListModels, etc.)

**Edge Cases:**
- **Nil Authenticator**: NewProvider handles nil authenticator by creating a default one.
- **HTTP Client Timeout**: BaseProvider is initialized with 5-minute timeout, matching the original implementation.
- **Project Initialization Error**: The `projectInitError` field is preserved as it's specific to Gemini.

**Verification:**
- Provider embeds BaseProvider
- Duplicate fields (httpClient, tokenManager, logger) are removed
- Duplicate method (SetTokenManager) is removed
- All remaining methods use BaseProvider methods for common operations
- Provider-specific fields and methods are preserved

---

### Step 2: Refactor Qwen Provider

**File to Modify**: `provider/qwen/qwen.go`

**Current Structure** (lines 37-42):
```go
type Provider struct {
	authenticator *QwenAuthenticator
	httpClient    *http.Client
	tokenManager  *auth.TokenManager
	logger        *logging.Logger
}
```

**Refactored Structure**:
```go
type Provider struct {
	*provider.BaseProvider  // Embedded base provider
	authenticator *QwenAuthenticator
}
```

**Modification Steps**:

1. **Update struct definition** (around line 37):

```go
// Provider implements the provider.Provider interface for Qwen
// Embeds BaseProvider for common functionality
type Provider struct {
	*provider.BaseProvider  // Embedded base provider provides common fields and methods
	authenticator *QwenAuthenticator
}
```

2. **Update NewProvider constructor** (around line 119):

```go
// NewProvider creates a new Qwen provider
// Uses BaseProvider for common functionality
func NewProvider(authenticator *QwenAuthenticator) *Provider {
	if authenticator == nil {
		authenticator = NewQwenAuthenticator(nil, logging.NewLogger())
	}
	return &Provider{
		BaseProvider: provider.NewBaseProvider(logging.NewLogger(), 5*time.Minute),
		authenticator: authenticator,
	}
}
```

3. **Remove duplicate fields**:

The following fields should be **removed** as they are now provided by BaseProvider:
- `httpClient` (line 39)
- `tokenManager` (line 40)
- `logger` (line 41)

4. **Remove duplicate methods**:

The following method should be **removed** as it is now provided by BaseProvider:
- `SetTokenManager()` (if present)

5. **Update methods that access removed fields**:

Any method that accessed `p.httpClient`, `p.tokenManager`, or `p.logger` should now use the embedded BaseProvider methods:

```go
// Example: Update GenerateContent method
// Before:
// client := p.httpClient

// After:
// client := p.GetHTTPClient()

// Example: Update any method that needs logger
// Before:
// p.logger.DebugLog(...)

// After:
// p.GetLogger().DebugLog(...)
```

6. **Keep provider-specific fields and methods**:

The following should be **kept** as they are specific to Qwen Provider:
- `authenticator` field (QwenAuthenticator)
- All provider-specific methods (GenerateContent, GenerateContentStream, ListModels, etc.)

**Edge Cases:**
- **Nil Authenticator**: NewProvider handles nil authenticator by creating a default one.
- **HTTP Client Timeout**: BaseProvider is initialized with 5-minute timeout, matching the original implementation.

**Verification:**
- Provider embeds BaseProvider
- Duplicate fields (httpClient, tokenManager, logger) are removed
- All remaining methods use BaseProvider methods for common operations
- Provider-specific fields and methods are preserved

---

### Step 3: Refactor Kiro Provider

**File to Modify**: `provider/kiro/kiro.go`

**Current Structure** (lines 58-64):
```go
type Provider struct {
	authenticator *auth.KiroAuthenticator
	httpClient    *http.Client
	tokenManager  *auth.TokenManager
	logger        *logging.Logger
	machineID     string
}
```

**Refactored Structure**:
```go
type Provider struct {
	*provider.BaseProvider  // Embedded base provider
	authenticator *auth.KiroAuthenticator
	machineID     string
}
```

**Modification Steps**:

1. **Update struct definition** (around line 58):

```go
// Provider implements the provider.Provider interface for Kiro
// Embeds BaseProvider for common functionality
type Provider struct {
	*provider.BaseProvider  // Embedded base provider provides common fields and methods
	authenticator *auth.KiroAuthenticator
	machineID     string  // Kiro-specific field
}
```

2. **Update NewProvider constructor** (around line 67):

```go
// NewProvider creates a new Kiro provider
// Uses BaseProvider for common functionality
func NewProvider(authenticator *auth.KiroAuthenticator) *Provider {
	if authenticator == nil {
		// Use direct instantiation for now - will be updated to use factory in later phase
		authenticator = auth.NewKiroAuthenticator(nil)
	}
	return &Provider{
		BaseProvider: provider.NewBaseProvider(logging.NewLogger(), 5*time.Minute),
		authenticator: authenticator,
		machineID:     generateMachineID(),
	}
}
```

3. **Remove duplicate fields**:

The following fields should be **removed** as they are now provided by BaseProvider:
- `httpClient` (line 60)
- `tokenManager` (line 61)
- `logger` (line 62)

4. **Remove duplicate methods**:

The following method should be **removed** as it is now provided by BaseProvider:
- `SetTokenManager()` (if present)

5. **Update methods that access removed fields**:

Any method that accessed `p.httpClient`, `p.tokenManager`, or `p.logger` should now use the embedded BaseProvider methods:

```go
// Example: Update GenerateContent method
// Before:
// client := p.httpClient

// After:
// client := p.GetHTTPClient()

// Example: Update any method that needs logger
// Before:
// p.logger.DebugLog(...)

// After:
// p.GetLogger().DebugLog(...)
```

6. **Keep provider-specific fields and methods**:

The following should be **kept** as they are specific to Kiro Provider:
- `authenticator` field (KiroAuthenticator)
- `machineID` field
- All provider-specific methods (GenerateContent, GenerateContentStream, ListModels, etc.)

**Edge Cases:**
- **Nil Authenticator**: NewProvider handles nil authenticator by creating a default one.
- **HTTP Client Timeout**: BaseProvider is initialized with 5-minute timeout, matching the original implementation.
- **Machine ID**: The `generateMachineID()` function is preserved as it's specific to Kiro.

**Verification:**
- Provider embeds BaseProvider
- Duplicate fields (httpClient, tokenManager, logger) are removed
- All remaining methods use BaseProvider methods for common operations
- Provider-specific fields and methods are preserved

---

### Step 4: Refactor iFlow Provider

**File to Modify**: `provider/iflow/iflow.go`

**Current Structure** (lines 46-52):
```go
type Provider struct {
	baseURL       string
	authenticator *auth.IFlowAuthenticator
	httpClient    *http.Client
	tokenManager  *auth.TokenManager
	logger        *logging.Logger
}
```

**Refactored Structure**:
```go
type Provider struct {
	*provider.BaseProvider  // Embedded base provider
	baseURL       string
	authenticator *auth.IFlowAuthenticator
}
```

**Modification Steps**:

1. **Update struct definition** (around line 46):

```go
// Provider implements the provider.Provider interface for iFlow
// Embeds BaseProvider for common functionality
type Provider struct {
	*provider.BaseProvider  // Embedded base provider provides common fields and methods
	baseURL       string
	authenticator *auth.IFlowAuthenticator
}
```

2. **Update NewProvider constructor** (around line 55):

```go
// NewProvider creates a new iFlow provider
// Uses BaseProvider for common functionality
func NewProvider(authenticator *auth.IFlowAuthenticator) *Provider {
	if authenticator == nil {
		// Use direct instantiation for now - will be updated to use factory in later phase
		authenticator = auth.NewIFlowAuthenticator(nil)
	}
	return &Provider{
		BaseProvider: provider.NewBaseProvider(logging.NewLogger(), 5*time.Minute),
		baseURL:     APIBaseURL,
		authenticator: authenticator,
	}
}
```

3. **Remove duplicate fields**:

The following fields should be **removed** as they are now provided by BaseProvider:
- `httpClient` (line 49)
- `tokenManager` (line 50)
- `logger` (line 51)

4. **Remove duplicate methods**:

The following method should be **removed** as it is now provided by BaseProvider:
- `SetTokenManager()` (around lines 97-99)

5. **Update methods that access removed fields**:

Any method that accessed `p.httpClient`, `p.tokenManager`, or `p.logger` should now use the embedded BaseProvider methods:

```go
// Example: Update GenerateContent method
// Before:
// client := p.httpClient

// After:
// client := p.GetHTTPClient()

// Example: Update any method that needs logger
// Before:
// p.logger.DebugLog(...)

// After:
// p.GetLogger().DebugLog(...)
```

6. **Keep provider-specific fields and methods**:

The following should be **kept** as they are specific to iFlow Provider:
- `baseURL` field
- `authenticator` field (IFlowAuthenticator)
- All provider-specific methods (GenerateContent, GenerateContentStream, ListModels, etc.)

**Edge Cases:**
- **Nil Authenticator**: NewProvider handles nil authenticator by creating a default one.
- **HTTP Client Timeout**: BaseProvider is initialized with 5-minute timeout, matching the original implementation.

**Verification:**
- Provider embeds BaseProvider
- Duplicate fields (httpClient, tokenManager, logger) are removed
- Duplicate method (SetTokenManager) is removed
- All remaining methods use BaseProvider methods for common operations
- Provider-specific fields and methods are preserved

---

### Step 5: Refactor Antigravity Provider

**File to Modify**: `provider/antigravity/antigravity.go`

**Current Structure** (lines 63-74):
```go
type Provider struct {
	dailyBaseURL    string
	autopushBaseURL string
	authenticator   *auth.GeminiAuthenticator
	httpClient      *http.Client
	tokenManager    *auth.TokenManager
	logger          *logging.Logger
	projectID       string
	isInitialized   bool
	cachedModels    map[string]bool
	cacheMu         sync.RWMutex
}
```

**Refactored Structure**:
```go
type Provider struct {
	*provider.BaseProvider  // Embedded base provider
	dailyBaseURL    string
	autopushBaseURL string
	authenticator   *auth.GeminiAuthenticator
	projectID       string
	isInitialized   bool
	cachedModels    map[string]bool
	cacheMu         sync.RWMutex
}
```

**Modification Steps**:

1. **Update struct definition** (around line 63):

```go
// Provider implements the provider.Provider interface for Antigravity
// Embeds BaseProvider for common functionality
type Provider struct {
	*provider.BaseProvider  // Embedded base provider provides common fields and methods
	dailyBaseURL    string
	autopushBaseURL string
	authenticator   *auth.GeminiAuthenticator
	projectID       string
	isInitialized   bool
	cachedModels    map[string]bool
	cacheMu         sync.RWMutex  // Antigravity-specific mutex for model cache
}
```

2. **Update NewProvider constructor** (around line 77):

```go
// NewProvider creates a new Antigravity provider
// Uses BaseProvider for common functionality
func NewProvider(authenticator *auth.GeminiAuthenticator) *Provider {
	if authenticator == nil {
		// Use direct instantiation for now - will be updated to use factory in later phase
		authenticator = auth.NewGeminiAuthenticator(&auth.GeminiOAuthConfig{
			ClientID:     "1071006060591-tmhssin2h21lcre235vtolojh4g403ep.apps.googleusercontent.com",
			ClientSecret: "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",
			Scope:        "https://www.googleapis.com/auth/cloud-platform",
			RedirectPort: 8086,
			CredsDir:     ".antigravity",
			CredsFile:    "oauth_creds.json",
		})
	}
	return &Provider{
		BaseProvider:    provider.NewBaseProvider(logging.NewLogger(), 5*time.Minute),
		dailyBaseURL:    DefaultDailyBaseURL,
		autopushBaseURL: DefaultAutopushBaseURL,
		authenticator:     authenticator,
		cachedModels:     make(map[string]bool),
	}
}
```

3. **Remove duplicate fields**:

The following fields should be **removed** as they are now provided by BaseProvider:
- `httpClient` (line 67)
- `tokenManager` (line 68)
- `logger` (line 69)

4. **Remove duplicate methods**:

The following method should be **removed** as it is now provided by BaseProvider:
- `SetTokenManager()` (if present)

5. **Update methods that access removed fields**:

Any method that accessed `p.httpClient`, `p.tokenManager`, or `p.logger` should now use the embedded BaseProvider methods:

```go
// Example: Update GenerateContent method
// Before:
// client := p.httpClient

// After:
// client := p.GetHTTPClient()

// Example: Update any method that needs logger
// Before:
// p.logger.DebugLog(...)

// After:
// p.GetLogger().DebugLog(...)
```

6. **Keep provider-specific fields and methods**:

The following should be **kept** as they are specific to Antigravity Provider:
- `dailyBaseURL` field
- `autopushBaseURL` field
- `authenticator` field (GeminiAuthenticator)
- `projectID` field
- `isInitialized` field
- `cachedModels` field
- `cacheMu` field (Antigravity-specific mutex for model cache)
- All provider-specific methods (GenerateContent, GenerateContentStream, ListModels, etc.)

**Edge Cases:**
- **Nil Authenticator**: NewProvider handles nil authenticator by creating a default one with Antigravity-specific config.
- **HTTP Client Timeout**: BaseProvider is initialized with 5-minute timeout, matching the original implementation.
- **Cache Mutex**: The `cacheMu` field is preserved as it's specific to Antigravity's model caching.

**Verification:**
- Provider embeds BaseProvider
- Duplicate fields (httpClient, tokenManager, logger) are removed
- All remaining methods use BaseProvider methods for common operations
- Provider-specific fields and methods are preserved

---

## Build Verification

After completing all steps, verify the code builds successfully:

```bash
cd c:/Users/jappa/projecten/qwencoder-proxy/qwencoder-proxy
go build ./...
```

**Expected Output**: No build errors

**If Build Fails**:
1. Check for missing imports (removed time package if no longer needed)
2. Verify all removed fields are no longer accessed
3. Ensure all method calls to embedded fields use correct syntax
4. Check for circular dependencies

---

## Test Verification

After successful build, run existing tests to ensure no regressions:

```bash
go test ./provider/... -v
go test ./provider/gemini/... -v
go test ./provider/qwen/... -v
go test ./provider/kiro/... -v
go test ./provider/iflow/... -v
go test ./provider/antigravity/... -v
```

**Expected Output**: All existing tests pass

**Note**: The refactored providers should behave identically to the original implementations. All tests should pass without modification.

---

## API Compatibility Verification

This phase modifies the internal structure of providers but should maintain backward compatibility with existing code.

**Verification Steps**:
1. All public methods of each provider are preserved
2. Method signatures are unchanged
3. Provider-specific fields are preserved
4. No changes to the Provider interface

**Backward Compatibility Checklist**:
- [x] Gemini Provider still implements Provider interface
- [x] Qwen Provider still implements Provider interface
- [x] Kiro Provider still implements Provider interface
- [x] iFlow Provider still implements Provider interface
- [x] Antigravity Provider still implements Provider interface
- [x] All public methods are preserved
- [x] Method signatures are unchanged

---

## Concurrency Considerations

### Thread Safety After Refactoring

1. **BaseProvider**: Provides thread-safe access to tokenManager via GetTokenManager() and SetTokenManager(). This is preserved after refactoring.

2. **Provider-Specific Fields**: Fields like `baseURL`, `authenticator`, `projectID`, etc. are typically read-only after construction. If they are modified, each provider should add its own mutex protection.

3. **Method Calls**: Methods that previously accessed `p.tokenManager` now use `p.GetTokenManager()` which returns the tokenManager. This maintains thread safety.

### Concurrency Edge Cases

1. **Concurrent Content Generation**: Multiple goroutines can call GenerateContent() simultaneously. The provider should use BaseProvider's GetHTTPClient() which returns the immutable httpClient.

2. **Concurrent Model Listing**: Multiple goroutines can call ListModels() simultaneously. The provider should use BaseProvider's GetHTTPClient() which returns the immutable httpClient.

3. **Concurrent Manager Setting**: Multiple goroutines can call SetTokenManager() simultaneously. BaseProvider's SetTokenManager() should be thread-safe (implementation in Phase 1).

---

## Phase Completion Criteria

Phase 4 is complete when:

- [x] `provider/gemini/gemini.go` refactored to embed BaseProvider
- [x] `provider/qwen/qwen.go` refactored to embed BaseProvider
- [x] `provider/kiro/kiro.go` refactored to embed BaseProvider
- [x] `provider/iflow/iflow.go` refactored to embed BaseProvider
- [x] `provider/antigravity/antigravity.go` refactored to embed BaseProvider
- [x] Duplicate fields removed from all providers
- [x] Duplicate methods removed from all providers
- [x] All remaining methods use BaseProvider methods for common operations
- [x] Code builds successfully with `go build ./...`
- [x] All existing tests pass
- [x] API compatibility is maintained

---

## Next Phase

Proceed to **Phase 5: Handler Modules Refactoring** after Phase 4 is complete.

Phase 5 will refactor the handler implementations to use BaseHandler, eliminating code duplication and ensuring consistent behavior across all handlers.
