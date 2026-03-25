# Implement Request Context Tracking

**Priority:** LOW  
**Estimated Time:** 1 day  
**Complexity:** Low  
**Files to Create:** 1  
**Files to Modify:** 5+

---

## Problem Description

Request context (token ID, provider ID, request ID) is not consistently tracked throughout the request lifecycle. This makes it difficult to:
- Trace requests through the system
- Debug issues with specific requests
- Implement rate limiting per token
- Log request metadata
- Correlate logs with requests

### Current State

**Issues Identified:**

1. **No Request ID:**
   - No unique identifier for each request
   - Difficult to correlate logs across components
   - Can't trace request flow through system

2. **Inconsistent Token ID Tracking:**
   - Token ID available in some places but not all
   - No standard way to pass token ID through request
   - Token ID lost in middleware layers

3. **No Provider ID Tracking:**
   - Provider ID not consistently tracked
   - Difficult to debug provider-specific issues
   - Can't correlate logs by provider

4. **Scattered Context Management:**
   - Context values set ad-hoc
   - No standard context keys
   - Inconsistent context usage

### Examples of Current State

**Example 1: In `internal/proxy/openai_handler.go`**

```go
// Line 196: Token selected but not added to context
token, err := h.tokenManager.SelectToken()
if err != nil {
	// ...
}

// Token ID available but not in request context
// Can't be used by middleware for rate limiting
// Can't be used for logging correlation
```

**Example 2: In `internal/restapi/middleware.go`**

```go
// Line 64-77: Logging middleware
func Logging(logger logging.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			
			// Wrap response writer to capture status code
			wrapped := &responseWriter{ResponseWriter: w}
			
			next.ServeHTTP(wrapped, r)
			
			duration := time.Since(start)
			logger.InfoLog("[%s] %s %s - Status: %d - Duration: %v",
				r.Method, r.URL.Path, r.RemoteAddr, wrapped.statusCode, duration)
		})
	}
}

// No request ID logged
// No correlation ID for request tracking
// Can't trace request through system
```

**Example 3: In `internal/restapi/rest_api.go`**

```go
// Line 789: Callback handler
func (s *Server) handleCallback(w http.ResponseWriter, r *http.Request) {
	s.logger.InfoLog("[handleCallback] Received callback request")
	
	// No request ID
	// No correlation with other logs
	// ...
}
```

**Problems:**
- No request ID for correlation
- No token ID in request context
- No provider ID in request context
- Difficult to trace request flow
- Poor debugging experience

---

## Solution Architecture

### Request Context Package

Create a dedicated package for request context management with:
- Standard context keys
- Request ID generation
- Token ID management
- Provider ID management
- Context propagation helpers

---

## Implementation Plan

### Phase 1: Create Context Package (Day 1)

#### Step 1.1: Create Context Keys

**New File:** `internal/context/keys.go`

```go
package context

import "context"

// ContextKey is the type for context keys
type ContextKey string

// Standard context keys
const (
	// Request identification
	RequestIDKey ContextKey = "request_id"
	
	// Token identification
	TokenIDKey ContextKey = "token_id"
	
	// Provider identification
	ProviderIDKey ContextKey = "provider_id"
	
	// Provider type
	ProviderTypeKey ContextKey = "provider_type"
	
	// Model identification
	ModelKey ContextKey = "model"
	
	// User identification
	UserIDKey ContextKey = "user_id"
	
	// Email identification
	EmailKey ContextKey = "email"
	
	// Request start time
	RequestStartTimeKey ContextKey = "request_start_time"
	
	// Request metadata
	RequestMetadataKey ContextKey = "request_metadata"
)
```

#### Step 1.2: Create Request Context

**New File:** `internal/context/request_context.go`

```go
package context

import (
	"context"
	"fmt"
	"time"
	"github.com/google/uuid"
)

// RequestMetadata holds metadata about a request
type RequestMetadata struct {
	RequestID    string
	StartTime    time.Time
	RemoteAddr   string
	UserAgent    string
	ContentType string
	ContentLength int64
}

// NewRequestContext creates a new request context with request ID
func NewRequestContext(r *http.Request) context.Context {
	// Generate unique request ID
	requestID := uuid.New().String()
	
	// Create request metadata
	metadata := RequestMetadata{
		RequestID:    requestID,
		StartTime:    time.Now(),
		RemoteAddr:   r.RemoteAddr,
		UserAgent:    r.UserAgent(),
		ContentType: r.Header.Get("Content-Type"),
		ContentLength: r.ContentLength,
	}
	
	// Create base context
	ctx := context.Background()
	
	// Add request ID
	ctx = context.WithValue(ctx, RequestIDKey, requestID)
	
	// Add request metadata
	ctx = context.WithValue(ctx, RequestMetadataKey, metadata)
	
	// Add request start time
	ctx = context.WithValue(ctx, RequestStartTimeKey, metadata.StartTime)
	
	return ctx
}

// GetRequestID retrieves the request ID from context
func GetRequestID(ctx context.Context) string {
	if requestID := ctx.Value(RequestIDKey); requestID != nil {
		if id, ok := requestID.(string); ok {
			return id
		}
	}
	return ""
}

// GetTokenID retrieves the token ID from context
func GetTokenID(ctx context.Context) string {
	if tokenID := ctx.Value(TokenIDKey); tokenID != nil {
		if id, ok := tokenID.(string); ok {
			return id
		}
	}
	return ""
}

// WithTokenID adds a token ID to the context
func WithTokenID(ctx context.Context, tokenID string) context.Context {
	return context.WithValue(ctx, TokenIDKey, tokenID)
}

// GetProviderID retrieves the provider ID from context
func GetProviderID(ctx context.Context) string {
	if providerID := ctx.Value(ProviderIDKey); providerID != nil {
		if id, ok := providerID.(string); ok {
			return id
		}
	}
	return ""
}

// WithProviderID adds a provider ID to the context
func WithProviderID(ctx context.Context, providerID string) context.Context {
	return context.WithValue(ctx, ProviderIDKey, providerID)
}

// GetProviderType retrieves the provider type from context
func GetProviderType(ctx context.Context) string {
	if providerType := ctx.Value(ProviderTypeKey); providerType != nil {
		if pt, ok := providerType.(string); ok {
			return pt
		}
	}
	return ""
}

// WithProviderType adds a provider type to the context
func WithProviderType(ctx context.Context, providerType string) context.Context {
	return context.WithValue(ctx, ProviderTypeKey, providerType)
}

// GetRequestMetadata retrieves request metadata from context
func GetRequestMetadata(ctx context.Context) RequestMetadata {
	if metadata := ctx.Value(RequestMetadataKey); metadata != nil {
		if m, ok := metadata.(RequestMetadata); ok {
			return m
		}
	}
	return RequestMetadata{}
}

// GetRequestStartTime retrieves the request start time from context
func GetRequestStartTime(ctx context.Context) time.Time {
	if startTime := ctx.Value(RequestStartTimeKey); startTime != nil {
		if t, ok := startTime.(time.Time); ok {
			return t
		}
	}
	return time.Time{}
}
```

### Phase 2: Update Proxy Handlers (Day 1)

#### Step 2.1: Add Request Context to OpenAI Handler

**Modify File:** `internal/proxy/openai_handler.go`

**Location:** Update `handleChatCompletions()` method (around line 152)

```go
// Add import
import (
	// ... existing imports ...
	"github.com/sunbankio/qwencoder-proxy/internal/context"
)

// BEFORE: No request context
func (h *OpenAIHandler) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	// ... existing code ...
}

// AFTER: Add request context
func (h *OpenAIHandler) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	// Create request context with request ID
	ctx := context.NewRequestContext(r)
	
	// Use context instead of r.Context() for all operations
	r = r.WithContext(ctx)
	
	// ... rest of the method ...
}
```

#### Step 2.2: Add Token ID to Context

**Modify File:** `internal/proxy/openai_handler.go`

**Location:** Update token selection in `handleChatCompletions()` (around line 196)

```go
// BEFORE: Token selected but not in context
token, err := h.tokenManager.SelectToken()
if err != nil {
	// ...
}

// AFTER: Add token ID to request context
token, err := h.tokenManager.SelectToken()
if err != nil {
	// ...
}

// Add token ID to request context
if token != nil {
	ctx := context.WithTokenID(r.Context(), token.ID)
	r = r.WithContext(ctx)
}
```

#### Step 2.3: Add Provider ID to Context

**Modify File:** `internal/proxy/openai_handler.go`

**Location:** Update provider usage in `handleChatCompletions()` (around line 170)

```go
// BEFORE: Provider used but not in context
if h.fixedProvider != "" {
	p, err = h.factory.Get(h.fixedProvider)
} else {
	p, err = h.factory.GetByModel(model)
}

// AFTER: Add provider ID to request context
if h.fixedProvider != "" {
	p, err = h.factory.Get(h.fixedProvider)
	if p != nil {
		ctx := context.WithProviderType(r.Context(), string(h.fixedProvider))
		r = r.WithContext(ctx)
	}
} else {
	p, err = h.factory.GetByModel(model)
	if p != nil {
		ctx := context.WithProviderType(r.Context(), string(p.ProviderType()))
		r = r.WithContext(ctx)
	}
}
```

### Phase 3: Update REST API (Day 1)

#### Step 3.1: Add Request Context to REST API Handlers

**Modify File:** `internal/restapi/rest_api.go`

**Location:** Update all handler methods to use request context

```go
// Add import
import (
	// ... existing imports ...
	"github.com/sunbankio/qwencoder-proxy/internal/context"
)

// Update handleCallback
func (s *Server) handleCallback(w http.ResponseWriter, r *http.Request) {
	// Create request context
	ctx := context.NewRequestContext(r)
	r = r.WithContext(ctx)
	
	s.logger.InfoLog("[handleCallback] Received callback request - RequestID: %s", 
		context.GetRequestID(ctx))
	
	// ... rest of the method ...
}
```

#### Step 3.2: Add Request Context to Token API

**Modify File:** `internal/restapi/token_api.go`

**Location:** Update all handler methods to use request context

```go
// Update handleGetToken
func (s *Server) handleGetToken(w http.ResponseWriter, r *http.Request, providerID string) {
	// Create request context
	ctx := context.NewRequestContext(r)
	r = r.WithContext(ctx)
	
	// Add provider ID to context
	ctx = context.WithProviderID(ctx, providerID)
	r = r.WithContext(ctx)
	
	// Select a token using the configured strategy
	token, err := manager.SelectToken()
	// ...
}
```

### Phase 4: Update Middleware (Day 1)

#### Step 4.1: Add Request Context to Logging Middleware

**Modify File:** `internal/restapi/middleware.go`

**Location:** Update `Logging()` middleware (around line 64)

```go
// BEFORE: No request ID in logs
func Logging(logger logging.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			
			wrapped := &responseWriter{ResponseWriter: w}
			
			next.ServeHTTP(wrapped, r)
			
			duration := time.Since(start)
			logger.InfoLog("[%s] %s %s - Status: %d - Duration: %v",
				r.Method, r.URL.Path, r.RemoteAddr, wrapped.statusCode, duration)
		})
	}
}

// AFTER: Include request ID in logs
func Logging(logger logging.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Get request ID from context
			requestID := context.GetRequestID(r.Context())
			
			start := time.Now()
			
			wrapped := &responseWriter{ResponseWriter: w}
			
			next.ServeHTTP(wrapped, r)
			
			duration := time.Since(start)
			
			// Log with request ID
			if requestID != "" {
				logger.InfoLog("[%s] %s %s - RequestID: %s - Status: %d - Duration: %v",
					r.Method, r.URL.Path, r.RemoteAddr, requestID, wrapped.statusCode, duration)
			} else {
				logger.InfoLog("[%s] %s %s - Status: %d - Duration: %v",
					r.Method, r.URL.Path, r.RemoteAddr, wrapped.statusCode, duration)
			}
		})
	}
}
```

#### Step 4.2: Add Request Context to Rate Limiting Middleware

**Modify File:** `internal/ratelimit/middleware.go` (if created)

**Location:** Update rate limiting middleware to use request context

```go
// BEFORE: No request context
func RateLimitMiddleware(limiter RateLimiter, logger logging.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Extract token ID from request context
			tokenID := r.Context().Value("token_id")
			// ...
		})
	}
}

// AFTER: Use context package
func RateLimitMiddleware(limiter RateLimiter, logger logging.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Extract token ID from request context using context package
			tokenID := context.GetTokenID(r.Context())
			
			// Get request ID for logging
			requestID := context.GetRequestID(r.Context())
			
			if tokenID == "" {
				next.ServeHTTP(w, r)
				return
			}
			
			// Check rate limit
			allowed, retryAfter, err := limiter.Allow(r.Context(), tokenID)
			// ...
		})
	}
}
```

---

## Testing

### Unit Tests

**New File:** `internal/context/request_context_test.go`

```go
package context

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"github.com/stretchr/testify/assert"
)

func TestNewRequestContext(t *testing.T) {
	req := httptest.NewRequest("GET", "/test", nil)
	
	ctx := NewRequestContext(req)
	
	requestID := GetRequestID(ctx)
	assert.NotEmpty(t, requestID)
	assert.Regexp(t, `^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$`, requestID)
	
	metadata := GetRequestMetadata(ctx)
	assert.NotZero(t, metadata.StartTime)
	assert.Equal(t, "/test", metadata.RequestID)
}

func TestWithTokenID(t *testing.T) {
	ctx := context.Background()
	
	ctx = WithTokenID(ctx, "test-token")
	
	tokenID := GetTokenID(ctx)
	assert.Equal(t, "test-token", tokenID)
}

func TestWithProviderID(t *testing.T) {
	ctx := context.Background()
	
	ctx = WithProviderID(ctx, "test-provider")
	
	providerID := GetProviderID(ctx)
	assert.Equal(t, "test-provider", providerID)
}

func TestGetRequestID(t *testing.T) {
	ctx := context.Background()
	ctx = context.WithValue(ctx, RequestIDKey, "test-request-id")
	
	requestID := GetRequestID(ctx)
	assert.Equal(t, "test-request-id", requestID)
}

func TestGetTokenID(t *testing.T) {
	ctx := context.Background()
	ctx = context.WithValue(ctx, TokenIDKey, "test-token")
	
	tokenID := GetTokenID(ctx)
	assert.Equal(t, "test-token", tokenID)
}

func TestGetProviderID(t *testing.T) {
	ctx := context.Background()
	ctx = context.WithValue(ctx, ProviderIDKey, "test-provider")
	
	providerID := GetProviderID(ctx)
	assert.Equal(t, "test-provider", providerID)
}
```

---

## Verification Checklist

### Phase 1: Context Package
- [ ] `internal/context/keys.go` created
- [ ] All context keys defined
- [ ] `internal/context/request_context.go` created
- [ ] All helper functions implemented
- [ ] Unit tests pass

### Phase 2: Proxy Handler Updates
- [ ] `openai_handler.go` imports context package
- [ ] Request context created in handlers
- [ ] Token ID added to context
- [ ] Provider ID added to context
- [ ] All tests pass

### Phase 3: REST API Updates
- [ ] `rest_api.go` imports context package
- [ ] Request context used in handlers
- [ ] Request ID logged in handlers
- [ ] All tests pass

### Phase 4: Middleware Updates
- [ ] `middleware.go` imports context package
- [ ] Logging middleware uses request ID
- [ ] Rate limiting middleware uses request ID
- [ ] All tests pass

---

## Impact

**Positive:**
- Request ID for correlation
- Better debugging experience
- Consistent context usage
- Better request tracing
- Improved logging
- Easier rate limiting implementation

**Code Volume Changes:**
- Added: ~150 lines in `keys.go`
- Added: ~200 lines in `request_context.go`
- Modified: ~50 lines across handlers
- Modified: ~30 lines in middleware
- Net addition: ~380 lines (but with better observability)

**Risk:**
- Low - context package is a well-established pattern
- Backward compatible (context values optional)
- No breaking changes to existing functionality

**Side Effects:**
- Request IDs in all logs
- Token IDs consistently tracked
- Provider IDs consistently tracked
- Better request correlation
- Improved debugging experience

---

## Future Enhancements

1. **Distributed Tracing:**
   - Integration with OpenTelemetry/Jaeger
   - Distributed request tracing
   - Span propagation
   - Trace context propagation

2. **Request Correlation:**
   - Cross-service request correlation
   - Parent/child request relationships
   - Correlation ID propagation

3. **Context Propagation:**
   - gRPC context propagation
   - GraphQL context propagation
   - WebSocket context propagation

4. **Request Metadata:**
   - Custom metadata support
   - User-defined context values
   - Metadata validation

5. **Context Visualization:**
   - Request flow visualization
   - Context tree visualization
   - Timeline views
