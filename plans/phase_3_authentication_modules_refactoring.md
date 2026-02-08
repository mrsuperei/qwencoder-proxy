# Phase 3: Authentication Modules Refactoring

**Phase**: 3 of 7  
**Estimated Time**: 1-2 days  
**Risk Level**: Low  
**Dependencies**: Phase 1 (Core interfaces and base structures), Phase 2 (Factory patterns)

---

## Phase Objectives

This phase refactors the authenticator implementations to use BaseAuthenticator, eliminating code duplication and ensuring consistent behavior across all authenticators.

**Primary Goals:**
1. Refactor GeminiAuthenticator to embed BaseAuthenticator
2. Refactor KiroAuthenticator to embed BaseAuthenticator
3. Refactor IFlowAuthenticator to embed BaseAuthenticator
4. Remove duplicate fields and methods from each authenticator
5. Ensure backward compatibility with existing code
6. Maintain thread-safety for all operations

---

## Target Files to Modify

1. `auth/gemini_auth.go` - Refactor to embed BaseAuthenticator
2. `auth/kiro_auth.go` - Refactor to embed BaseAuthenticator
3. `auth/iflow_auth.go` - Refactor to embed BaseAuthenticator

---

## Step-by-Step Instructions

### Step 1: Refactor GeminiAuthenticator

**File to Modify**: `auth/gemini_auth.go`

**Current Structure** (lines 55-62):
```go
type GeminiAuthenticator struct {
	config        *GeminiOAuthConfig
	tokenManager  *TokenManager
	multiTokenMgr *MultiTokenManager
	mu            sync.RWMutex
	logger        *logging.Logger
	httpClient    *http.Client
}
```

**Refactored Structure**:
```go
type GeminiAuthenticator struct {
	*BaseAuthenticator  // Embedded base authenticator
	config        *GeminiOAuthConfig
}
```

**Modification Steps**:

1. **Update struct definition** (around line 55):

```go
// GeminiAuthenticator implements Authenticator interface for Gemini
// Embeds BaseAuthenticator for common functionality
type GeminiAuthenticator struct {
	*BaseAuthenticator  // Embedded base authenticator provides common fields and methods
	config        *GeminiOAuthConfig
}
```

2. **Update NewGeminiAuthenticator constructor** (around line 65):

```go
// NewGeminiAuthenticator creates a new Gemini authenticator
// Uses BaseAuthenticator for common functionality
func NewGeminiAuthenticator(config *GeminiOAuthConfig) *GeminiAuthenticator {
	if config == nil {
		config = DefaultGeminiOAuthConfig()
	}
	return &GeminiAuthenticator{
		BaseAuthenticator: NewBaseAuthenticator(logging.NewLogger()),
		config:           config,
	}
}
```

3. **Remove duplicate methods**:

The following methods should be **removed** as they are now provided by BaseAuthenticator:
- `SetTokenManager()` (around lines 79-83)
- `SetMultiTokenManager()` (around lines 86-90)

**Note**: Do NOT remove `GetCredentialsPath()` as it's specific to GeminiAuthenticator.

4. **Update methods that access removed fields**:

Any method that accessed `a.tokenManager`, `a.multiTokenMgr`, `a.mu`, `a.logger`, or `a.httpClient` should now use the embedded BaseAuthenticator methods:

```go
// Example: Update Authenticate method (around line 105)
// Before:
// a.mu.Lock()
// defer a.mu.Unlock()
// token := a.tokenManager

// After:
// token := a.GetTokenManager()

// Example: Update GetToken method (around line 150)
// Before:
// a.mu.RLock()
// defer a.mu.RUnlock()
// client := a.httpClient

// After:
// client := a.GetHTTPClient()
```

5. **Keep provider-specific methods**:

The following methods should be **kept** as they are specific to GeminiAuthenticator:
- `GetCredentialsPath()` (around line 93)
- `IsAuthenticated()` (around line 98)
- `Authenticate()` (around line 105)
- `GetToken()` (around line 150)
- `ClearCredentials()` (around line 200)
- Any other Gemini-specific methods

**Edge Cases:**
- **Nil Config**: NewGeminiAuthenticator handles nil config by using DefaultGeminiOAuthConfig().
- **Thread Safety**: BaseAuthenticator provides thread-safe access to tokenManager and multiTokenMgr via mutex-protected getters/setters.
- **HTTP Client**: BaseAuthenticator provides a default HTTP client. For proxy-aware clients, use the factory from Phase 2.

**Verification:**
- GeminiAuthenticator embeds BaseAuthenticator
- Duplicate fields (tokenManager, multiTokenMgr, mu, logger, httpClient) are removed
- Duplicate methods (SetTokenManager, SetMultiTokenManager) are removed
- All remaining methods use BaseAuthenticator methods for common operations
- Provider-specific methods are preserved

---

### Step 2: Refactor KiroAuthenticator

**File to Modify**: `auth/kiro_auth.go`

**Current Structure** (lines 55-63):
```go
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

**Refactored Structure**:
```go
type KiroAuthenticator struct {
	*BaseAuthenticator  // Embedded base authenticator
	config        *KiroOAuthConfig
	credentials   *KiroCredentials
}
```

**Modification Steps**:

1. **Update struct definition** (around line 55):

```go
// KiroAuthenticator implements the Authenticator interface for Kiro
// Embeds BaseAuthenticator for common functionality
type KiroAuthenticator struct {
	*BaseAuthenticator  // Embedded base authenticator provides common fields and methods
	config        *KiroOAuthConfig
	credentials   *KiroCredentials  // Kiro-specific field
}
```

2. **Update NewKiroAuthenticator constructor** (around line 66):

```go
// NewKiroAuthenticator creates a new Kiro authenticator
// Uses BaseAuthenticator for common functionality
func NewKiroAuthenticator(config *KiroOAuthConfig) *KiroAuthenticator {
	if config == nil {
		config = DefaultKiroOAuthConfig()
	}
	return &KiroAuthenticator{
		BaseAuthenticator: NewBaseAuthenticator(logging.NewLogger()),
		config:           config,
	}
}
```

3. **Remove duplicate methods**:

The following methods should be **removed** as they are now provided by BaseAuthenticator:
- `SetTokenManager()` (around lines 80-84)
- `SetMultiTokenManager()` (around lines 87-91)

**Note**: Do NOT remove `GetCredentialsPath()` as it's specific to KiroAuthenticator.

4. **Update methods that access removed fields**:

Any method that accessed `a.tokenManager`, `a.multiTokenMgr`, `a.mu`, `a.logger`, or `a.httpClient` should now use the embedded BaseAuthenticator methods:

```go
// Example: Update Authenticate method (around line 100)
// Before:
// a.mu.Lock()
// defer a.mu.Unlock()
// token := a.tokenManager

// After:
// token := a.GetTokenManager()

// Example: Update GetToken method (around line 150)
// Before:
// a.mu.RLock()
// defer a.mu.RUnlock()
// client := a.httpClient

// After:
// client := a.GetHTTPClient()
```

5. **Keep provider-specific methods**:

The following methods should be **kept** as they are specific to KiroAuthenticator:
- `GetCredentialsPath()` (around line 94)
- `IsAuthenticated()` (around line 99)
- `Authenticate()` (around line 100)
- `GetToken()` (around line 150)
- `ClearCredentials()` (around line 200)
- Any other Kiro-specific methods

**Edge Cases:**
- **Nil Config**: NewKiroAuthenticator handles nil config by using DefaultKiroOAuthConfig().
- **Thread Safety**: BaseAuthenticator provides thread-safe access to tokenManager and multiTokenMgr via mutex-protected getters/setters.
- **HTTP Client**: BaseAuthenticator provides a default HTTP client. For proxy-aware clients, use the factory from Phase 2.

**Verification:**
- KiroAuthenticator embeds BaseAuthenticator
- Duplicate fields (tokenManager, multiTokenMgr, mu, logger, httpClient) are removed
- Duplicate methods (SetTokenManager, SetMultiTokenManager) are removed
- All remaining methods use BaseAuthenticator methods for common operations
- Provider-specific methods are preserved
- Kiro-specific `credentials` field is preserved

---

### Step 3: Refactor IFlowAuthenticator

**File to Modify**: `auth/iflow_auth.go`

**Current Structure** (lines 131-138):
```go
type IFlowAuthenticator struct {
	config        *IFlowOAuthConfig
	tokenManager  *TokenManager
	multiTokenMgr *MultiTokenManager
	mu            sync.RWMutex
	logger        *logging.Logger
	httpClient    *http.Client
}
```

**Refactored Structure**:
```go
type IFlowAuthenticator struct {
	*BaseAuthenticator  // Embedded base authenticator
	config        *IFlowOAuthConfig
}
```

**Modification Steps**:

1. **Update struct definition** (around line 131):

```go
// IFlowAuthenticator implements the auth.Authenticator interface for iFlow
// Embeds BaseAuthenticator for common functionality
type IFlowAuthenticator struct {
	*BaseAuthenticator  // Embedded base authenticator provides common fields and methods
	config        *IFlowOAuthConfig
}
```

2. **Update NewIFlowAuthenticator constructor** (around line 142):

```go
// NewIFlowAuthenticator creates a new iFlow authenticator
// Uses BaseAuthenticator for common functionality
func NewIFlowAuthenticator(config *IFlowOAuthConfig) *IFlowAuthenticator {
	if config == nil {
		config = DefaultIFlowOAuthConfig()
	}
	return &IFlowAuthenticator{
		BaseAuthenticator: NewBaseAuthenticator(logging.NewLogger()),
		config:           config,
	}
}
```

3. **Remove duplicate methods**:

The following methods should be **removed** as they are now provided by BaseAuthenticator:
- `SetTokenManager()` (around lines 150-154)
- `SetMultiTokenManager()` (around lines 157-161)

**Note**: Do NOT remove `GetCredentialsPath()` as it's specific to IFlowAuthenticator.

4. **Update methods that access removed fields**:

Any method that accessed `a.tokenManager`, `a.multiTokenMgr`, `a.mu`, `a.logger`, or `a.httpClient` should now use the embedded BaseAuthenticator methods:

```go
// Example: Update Authenticate method (around line 170)
// Before:
// a.mu.Lock()
// defer a.mu.Unlock()
// token := a.tokenManager

// After:
// token := a.GetTokenManager()

// Example: Update GetToken method (around line 220)
// Before:
// a.mu.RLock()
// defer a.mu.RUnlock()
// client := a.httpClient

// After:
// client := a.GetHTTPClient()
```

5. **Keep provider-specific methods**:

The following methods should be **kept** as they are specific to IFlowAuthenticator:
- `GetCredentialsPath()` (around line 165)
- `IsAuthenticated()` (around line 170)
- `Authenticate()` (around line 170)
- `GetToken()` (around line 220)
- `ClearCredentials()` (around line 270)
- Any other IFlow-specific methods

**Edge Cases:**
- **Nil Config**: NewIFlowAuthenticator handles nil config by using DefaultIFlowOAuthConfig().
- **Thread Safety**: BaseAuthenticator provides thread-safe access to tokenManager and multiTokenMgr via mutex-protected getters/setters.
- **HTTP Client**: BaseAuthenticator provides a default HTTP client. For proxy-aware clients, use the factory from Phase 2.

**Verification:**
- IFlowAuthenticator embeds BaseAuthenticator
- Duplicate fields (tokenManager, multiTokenMgr, mu, logger, httpClient) are removed
- Duplicate methods (SetTokenManager, SetMultiTokenManager) are removed
- All remaining methods use BaseAuthenticator methods for common operations
- Provider-specific methods are preserved

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
2. Verify all removed methods are no longer called
3. Ensure all method calls to embedded fields use correct syntax
4. Check for circular dependencies

---

## Test Verification

After successful build, run existing tests to ensure no regressions:

```bash
go test ./auth/... -v
```

**Expected Output**: All existing tests pass

**Note**: The refactored authenticators should behave identically to the original implementations. All tests should pass without modification.

---

## API Compatibility Verification

This phase modifies the internal structure of authenticators but should maintain backward compatibility with existing code.

**Verification Steps**:
1. All public methods of each authenticator are preserved
2. Method signatures are unchanged
3. Provider-specific fields (config, credentials) are preserved
4. No changes to the Authenticator interface

**Backward Compatibility Checklist**:
- [x] GeminiAuthenticator still implements Authenticator interface
- [x] KiroAuthenticator still implements Authenticator interface
- [x] IFlowAuthenticator still implements Authenticator interface
- [x] All public methods are preserved
- [x] Method signatures are unchanged

---

## Concurrency Considerations

### Thread Safety After Refactoring

1. **BaseAuthenticator**: Provides thread-safe access to tokenManager and multiTokenMgr via mutex-protected getters/setters. This is preserved after refactoring.

2. **Provider-Specific Fields**: Fields like `config` and `credentials` are typically read-only after construction. If they are modified, each authenticator should add its own mutex protection.

3. **Method Calls**: Methods that previously used `a.mu.Lock()` now use `a.GetTokenManager()` which internally uses `a.mu.RLock()`. This maintains thread safety.

### Concurrency Edge Cases

1. **Concurrent Authentication**: Multiple goroutines can call Authenticate() simultaneously. The authenticator should use BaseAuthenticator's mutex-protected getters to access tokenManager.

2. **Concurrent Token Access**: Multiple goroutines can call GetToken() simultaneously. The authenticator should use BaseAuthenticator's mutex-protected getters to access tokenManager.

3. **Concurrent Manager Setting**: Multiple goroutines can call SetTokenManager() simultaneously. BaseAuthenticator's SetTokenManager() uses mutex.Lock() to ensure thread-safe access.

---

## Phase Completion Criteria

Phase 3 is complete when:

- [x] `auth/gemini_auth.go` refactored to embed BaseAuthenticator
- [x] `auth/kiro_auth.go` refactored to embed BaseAuthenticator
- [x] `auth/iflow_auth.go` refactored to embed BaseAuthenticator
- [x] Duplicate fields removed from all authenticators
- [x] Duplicate methods removed from all authenticators
- [x] All remaining methods use BaseAuthenticator methods for common operations
- [x] Code builds successfully with `go build ./...`
- [x] All existing tests pass
- [x] API compatibility is maintained

---

## Next Phase

Proceed to **Phase 4: Provider Implementations Refactoring** after Phase 3 is complete.

Phase 4 will refactor the provider implementations to use BaseProvider and AuthenticatorFactory, eliminating code duplication and enabling dependency injection.
