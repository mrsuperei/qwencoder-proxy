# Phase 1: Core Interfaces and Base Structures

**Phase**: 1 of 7  
**Estimated Time**: 1-2 days  
**Risk Level**: Low  
**Dependencies**: None (Foundational phase)

---

## Phase Objectives

This phase establishes the foundational interfaces and base structures that will be used throughout the refactoring. By creating these abstractions first, we ensure all subsequent phases have a solid foundation to build upon.

**Primary Goals:**
1. Create core interfaces for Dependency Inversion Principle compliance
2. Create base structures to eliminate code duplication
3. Ensure backward compatibility with existing code
4. Establish patterns for thread-safe operations

---

## Target Files to Create

1. `logging/interface.go` - Logger interface
2. `auth/store.go` - TokenStore interface
3. `auth/base.go` - BaseAuthenticator struct
4. `provider/base.go` - BaseProvider struct
5. `proxy/base.go` - BaseHandler struct

---

## Step-by-Step Instructions

### Step 1: Create Logger Interface

**File to Create**: `logging/interface.go`

```go
// Package logging provides interface for logging operations
package logging

// Logger defines the interface for logging operations
// This interface follows the Dependency Inversion Principle
// All logging operations should use this interface instead of concrete *logging.Logger
type Logger interface {
	// InfoLog logs informational messages
	InfoLog(format string, args ...interface{})
	
	// DebugLog logs debug messages
	DebugLog(format string, args ...interface{})
	
	// ErrorLog logs error messages
	ErrorLog(format string, args ...interface{})
	
	// WarnLog logs warning messages
	WarnLog(format string, args ...interface{})
}
```

**Verification:**
- File created at `logging/interface.go`
- Interface defines all 4 logging methods
- No implementation details (interface only)

---

### Step 2: Create TokenStore Interface

**File to Create**: `auth/store.go`

```go
// Package auth provides interface for token storage operations
package auth

// TokenStore defines the interface for token storage operations
// This interface follows the Dependency Inversion Principle
// It allows swapping storage implementations (file, database, cloud, in-memory for tests)
type TokenStore interface {
	// Load loads all tokens from storage
	Load() (map[string]TokenMetadata, error)
	
	// Save saves all tokens to storage
	Save(tokens map[string]TokenMetadata) error
	
	// GetCredentialsPath returns the path to the credentials file
	GetCredentialsPath() string
	
	// Clear removes all tokens from storage
	Clear() error
}
```

**Verification:**
- File created at `auth/store.go`
- Interface defines all 4 storage operations
- No implementation details (interface only)

---

### Step 3: Create BaseAuthenticator

**File to Create**: `auth/base.go`

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
// This implements the Template Method pattern for common authentication behavior
// Thread-safe with mutex protection for all shared fields
type BaseAuthenticator struct {
	tokenManager  *TokenManager
	multiTokenMgr *MultiTokenManager
	mu            sync.RWMutex
	logger        logging.Logger
	httpClient    *http.Client
}

// NewBaseAuthenticator creates a new base authenticator with default HTTP client
// Returns a BaseAuthenticator initialized with thread-safe defaults
func NewBaseAuthenticator(logger logging.Logger) *BaseAuthenticator {
	return &BaseAuthenticator{
		logger:     logger,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// SetTokenManager sets the token manager for this authenticator
// Thread-safe with mutex protection to prevent race conditions
// This method enables dependency injection for proxy-aware token selection
func (ba *BaseAuthenticator) SetTokenManager(tokenManager *TokenManager) {
	ba.mu.Lock()
	defer ba.mu.Unlock()
	ba.tokenManager = tokenManager
}

// SetMultiTokenManager sets the multi-token manager for this authenticator
// Thread-safe with mutex protection to prevent race conditions
func (ba *BaseAuthenticator) SetMultiTokenManager(mtm *MultiTokenManager) {
	ba.mu.Lock()
	defer ba.mu.Unlock()
	ba.multiTokenMgr = mtm
}

// GetTokenManager returns the token manager
// Thread-safe with mutex protection to prevent race conditions
func (ba *BaseAuthenticator) GetTokenManager() *TokenManager {
	ba.mu.RLock()
	defer ba.mu.RUnlock()
	return ba.tokenManager
}

// GetMultiTokenManager returns the multi-token manager
// Thread-safe with mutex protection to prevent race conditions
func (ba *BaseAuthenticator) GetMultiTokenManager() *MultiTokenManager {
	ba.mu.RLock()
	defer ba.mu.RUnlock()
	return ba.multiTokenMgr
}

// GetLogger returns the logger
// No mutex needed - logger is immutable after construction
func (ba *BaseAuthenticator) GetLogger() logging.Logger {
	return ba.logger
}

// GetHTTPClient returns the HTTP client
// No mutex needed - httpClient is immutable after construction
func (ba *BaseAuthenticator) GetHTTPClient() *http.Client {
	return ba.httpClient
}
```

**Edge Cases:**
- **Nil Logger**: The constructor accepts a logger interface, which could be nil. The calling code is responsible for providing a valid logger.
- **Concurrent Access**: All getter/setter methods use RWMutex for thread-safe access to shared fields.
- **HTTP Client Immutability**: The HTTP client is created once and reused. For proxy-aware clients, this will be enhanced in Phase 2.

**Verification:**
- File created at `auth/base.go`
- BaseAuthenticator struct has all common fields
- All setter methods use mutex.Lock()
- All getter methods use mutex.RLock()
- Logger uses interface type (not concrete *logging.Logger)

---

### Step 4: Create BaseProvider

**File to Create**: `provider/base.go`

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
// This implements the Template Method pattern for common provider behavior
type BaseProvider struct {
	httpClient   *http.Client
	tokenManager *auth.TokenManager
	logger       logging.Logger
}

// NewBaseProvider creates a new base provider with default HTTP client
// Returns a BaseProvider initialized with standard defaults
func NewBaseProvider(logger logging.Logger, timeout time.Duration) *BaseProvider {
	return &BaseProvider{
		httpClient: &http.Client{Timeout: timeout},
		logger:     logger,
	}
}

// GetHTTPClient returns the HTTP client
// No mutex needed - httpClient is immutable after construction
func (bp *BaseProvider) GetHTTPClient() *http.Client {
	return bp.httpClient
}

// GetLogger returns the logger
// No mutex needed - logger is immutable after construction
func (bp *BaseProvider) GetLogger() logging.Logger {
	return bp.logger
}

// GetTokenManager returns the token manager
// No mutex needed - tokenManager is set once during initialization
func (bp *BaseProvider) GetTokenManager() *auth.TokenManager {
	return bp.tokenManager
}

// SetTokenManager sets the token manager for this provider
// This method enables dependency injection for proxy-aware token selection
// Thread-safe: tokenManager should only be set once during initialization
func (bp *BaseProvider) SetTokenManager(tm *auth.TokenManager) {
	bp.tokenManager = tm
}
```

**Edge Cases:**
- **Nil Logger**: The constructor accepts a logger interface, which could be nil. The calling code is responsible for providing a valid logger.
- **HTTP Client Timeout**: The timeout is configurable via constructor parameter. Default should be 5 minutes for providers.
- **Token Manager**: Should be set once during initialization via dependency injection.

**Verification:**
- File created at `provider/base.go`
- BaseProvider struct has all common fields
- Logger uses interface type (not concrete *logging.Logger)
- TokenManager uses concrete type (will be abstracted in later phases)

---

### Step 5: Create BaseHandler

**File to Create**: `proxy/base.go`

```go
// Package proxy provides base structures for HTTP handlers
package proxy

import (
	"net/http"

	"github.com/sunbankio/qwencoder-proxy/auth"
	"github.com/sunbankio/qwencoder-proxy/logging"
)

// BaseHandler provides common fields and methods for HTTP handlers
// This implements the Template Method pattern for common handler behavior
type BaseHandler struct {
	logger       logging.Logger
	tokenManager *auth.TokenManager
}

// NewBaseHandler creates a new base handler
// Returns a BaseHandler initialized with provided dependencies
func NewBaseHandler(logger logging.Logger, tokenManager *auth.TokenManager) *BaseHandler {
	return &BaseHandler{
		logger:       logger,
		tokenManager: tokenManager,
	}
}

// SetCORSHeaders sets CORS headers for the response
// Centralizes CORS configuration to ensure consistency across all handlers
func (bh *BaseHandler) SetCORSHeaders(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
}

// HandleOptions handles OPTIONS requests for CORS preflight
// Returns 200 OK for all OPTIONS requests
func (bh *BaseHandler) HandleOptions(w http.ResponseWriter) {
	w.WriteHeader(http.StatusOK)
}

// GetLogger returns the logger
// No mutex needed - logger is immutable after construction
func (bh *BaseHandler) GetLogger() logging.Logger {
	return bh.logger
}

// GetTokenManager returns the token manager
// No mutex needed - tokenManager is set once during initialization
func (bh *BaseHandler) GetTokenManager() *auth.TokenManager {
	return bh.tokenManager
}

// IsProxyError checks if an error is a proxy-related error
// Returns true for common proxy error patterns
func (bh *BaseHandler) IsProxyError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return contains(errStr, "proxy") || 
		contains(errStr, "connection refused") || 
		contains(errStr, "timeout") ||
		contains(errStr, "network")
}

// HandleProxyError handles proxy-related errors
// Logs the error and returns a 502 Bad Gateway response
func (bh *BaseHandler) HandleProxyError(w http.ResponseWriter, err error) {
	bh.GetLogger().ErrorLog("[BaseHandler] Proxy error: %v", err)
	http.Error(w, "Proxy connection failed", http.StatusBadGateway)
}

// contains is a helper function for string matching
// Returns true if substr is found in s
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || 
		len(s) > len(substr) && (s[:len(substr)] == substr || 
		s[len(s)-len(substr):] == substr || 
		indexOfSubstring(s, substr) >= 0))
}

// indexOfSubstring is a helper function for finding substring index
// Returns the index of substr in s, or -1 if not found
func indexOfSubstring(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
```

**Edge Cases:**
- **Nil Logger**: The constructor accepts a logger interface, which could be nil. The calling code is responsible for providing a valid logger.
- **Nil TokenManager**: The constructor accepts a tokenManager which could be nil. This is acceptable as some handlers don't require token management.
- **Nil Error**: IsProxyError safely handles nil errors by returning false.

**Verification:**
- File created at `proxy/base.go`
- BaseHandler struct has all common fields
- Logger uses interface type (not concrete *logging.Logger)
- CORS headers are centralized
- Error handling methods are provided

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
2. Verify interface method signatures match existing usage
3. Ensure all files are in the correct directories
4. Check for circular dependencies

---

## Test Verification

After successful build, run existing tests to ensure no regressions:

```bash
go test ./logging/... -v
go test ./auth/... -v
go test ./provider/... -v
go test ./proxy/... -v
```

**Expected Output**: All existing tests pass

**Note**: New files created in this phase do not yet have tests. Tests will be added in later phases.

---

## API Compatibility Verification

This phase creates new interfaces and base structures but does not modify existing code. Therefore, API compatibility is maintained.

**Verification Steps**:
1. No existing files were modified
2. All new files are additions only
3. No changes to public interfaces of existing code

---

## Phase Completion Criteria

Phase 1 is complete when:

- [x] `logging/interface.go` created with Logger interface
- [x] `auth/store.go` created with TokenStore interface
- [x] `auth/base.go` created with BaseAuthenticator struct
- [x] `provider/base.go` created with BaseProvider struct
- [x] `proxy/base.go` created with BaseHandler struct
- [x] Code builds successfully with `go build ./...`
- [x] All existing tests pass

---

## Next Phase

Proceed to **Phase 2: Factory Patterns Implementation** after Phase 1 is complete.

Phase 2 will build upon these interfaces to create factory patterns for consistent object creation.
