# Centralize Error Handling

**Priority:** MEDIUM  
**Estimated Time:** 2 days  
**Complexity:** Low  
**Files to Create:** 1  
**Files to Modify:** 8+

---

## Problem Description

Error handling is scattered throughout the codebase with repeated patterns and inconsistent error context. This makes debugging difficult and leads to poor error messages for users.

### Current State

**Issues Identified:**

1. **Repeated Error Patterns:**
   - Same error handling code in multiple locations
   - Inconsistent error message formatting
   - Missing context in errors

2. **Poor Error Context:**
   - Errors lack provider ID
   - Errors lack token ID
   - Errors lack request context
   - Errors lack operation details

3. **Inconsistent Error Types:**
   - Mix of string errors and custom error types
   - No standardized error codes
   - No error classification

### Examples of Current Error Handling

**Example 1: In `internal/restapi/rest_api.go`**

```go
// Line 1122: Generic error with minimal context
if err != nil {
	s.logger.WarnLog("Failed to extract email for %s: %v", providerID, err)
	email = "unknown@example.com"
}

// Line 1173: Error without context
return "", fmt.Errorf("failed to get email extractor for %s: %w", providerID, err)

// Line 1179: Error without context
return "", fmt.Errorf("failed to extract email: %w", err)
```

**Example 2: In `internal/proxy/openai_handler.go`**

```go
// Line 177: Generic error
http.Error(w, fmt.Sprintf("Provider not found: %v", err), http.StatusBadRequest)

// Line 186: Generic error
http.Error(w, "Protocol conversion not supported", http.StatusInternalServerError)

// Line 218: Generic error
h.GetLogger().ErrorLog("[Handler] GenerateAndConvert failed with %s: %v", p.Name(), err)
```

**Problems:**
- No error codes for programmatic handling
- No error context (provider, token, request)
- Inconsistent error message format
- Difficult to track error patterns

---

## Solution Architecture

### Centralized Error Package

Create a dedicated error package with:
- Standardized error types
- Error codes for programmatic handling
- Rich error context
- Error wrapping with context
- Error classification

---

## Implementation Plan

### Phase 1: Create Error Package (Day 1)

#### Step 1.1: Create Error Types

**New File:** `internal/errors/errors.go`

```go
package errors

import (
	"fmt"
	"net/http"
)

// ErrorCode defines standardized error codes
type ErrorCode string

const (
	// Authentication errors
	ErrCodeUnauthorized          ErrorCode = "UNAUTHORIZED"
	ErrCodeTokenExpired         ErrorCode = "TOKEN_EXPIRED"
	ErrCodeTokenInvalid         ErrorCode = "TOKEN_INVALID"
	ErrCodeEmailExtractionFailed ErrorCode = "EMAIL_EXTRACTION_FAILED"
	
	// Rate limiting errors
	ErrCodeRateLimitExceeded   ErrorCode = "RATE_LIMIT_EXCEEDED"
	ErrCodeQuotaExceeded       ErrorCode = "QUOTA_EXCEEDED"
	
	// Provider errors
	ErrCodeProviderUnavailable  ErrorCode = "PROVIDER_UNAVAILABLE"
	ErrCodeProviderTimeout     ErrorCode = "PROVIDER_TIMEOUT"
	ErrCodeProviderError       ErrorCode = "PROVIDER_ERROR"
	
	// Request errors
	ErrCodeInvalidRequest      ErrorCode = "INVALID_REQUEST"
	ErrCodeMissingParameter   ErrorCode = "MISSING_PARAMETER"
	ErrCodeInvalidModel       ErrorCode = "INVALID_MODEL"
	
	// Storage errors
	ErrCodeStorageError        ErrorCode = "STORAGE_ERROR"
	ErrCodeTokenNotFound       ErrorCode = "TOKEN_NOT_FOUND"
	ErrCodeDatabaseError      ErrorCode = "DATABASE_ERROR"
	
	// Conversion errors
	ErrCodeConversionError     ErrorCode = "CONVERSION_ERROR"
	ErrCodeFormatMismatch      ErrorCode = "FORMAT_MISMATCH"
)

// ErrorSeverity defines the severity of an error
type ErrorSeverity string

const (
	SeverityCritical ErrorSeverity = "critical"
	SeverityHigh     ErrorSeverity = "high"
	SeverityMedium   ErrorSeverity = "medium"
	SeverityLow      ErrorSeverity = "low"
	SeverityInfo     ErrorSeverity = "info"
)

// ErrorContext provides contextual information about an error
type ErrorContext struct {
	ProviderID string `json:"provider_id,omitempty"`
	TokenID    string `json:"token_id,omitempty"`
	RequestID  string `json:"request_id,omitempty"`
	Operation   string `json:"operation,omitempty"`
	Model       string `json:"model,omitempty"`
	Endpoint    string `json:"endpoint,omitempty"`
}

// AppError represents a standardized application error
type AppError struct {
	Code       ErrorCode      `json:"code"`
	Message    string         `json:"message"`
	Severity   ErrorSeverity  `json:"severity,omitempty"`
	Context    ErrorContext   `json:"context,omitempty"`
	HTTPStatus int            `json:"http_status,omitempty"`
	Cause      error          `json:"-"`
}

// Error implements the error interface
func (e *AppError) Error() string {
	return e.Message
}

// Unwrap returns the underlying error
func (e *AppError) Unwrap() error {
	return e.Cause
}

// Is checks if the error matches a specific error code
func (e *AppError) Is(code ErrorCode) bool {
	return e.Code == code
}

// New creates a new application error
func New(code ErrorCode, message string) *AppError {
	return &AppError{
		Code:       code,
		Message:    message,
		Severity:   SeverityMedium,
		HTTPStatus: http.StatusInternalServerError,
	}
}

// Newf creates a new application error with formatted message
func Newf(code ErrorCode, format string, args ...interface{}) *AppError {
	return &AppError{
		Code:       code,
		Message:    fmt.Sprintf(format, args...),
		Severity:   SeverityMedium,
		HTTPStatus: http.StatusInternalServerError,
	}
}

// Wrap wraps an existing error with additional context
func Wrap(err error, code ErrorCode, message string) *AppError {
	if err == nil {
		return New(code, message)
	}
	
	return &AppError{
		Code:       code,
		Message:    message,
		Severity:   SeverityMedium,
		HTTPStatus: http.StatusInternalServerError,
		Cause:      err,
	}
}

// Wrapf wraps an existing error with formatted message and context
func Wrapf(err error, code ErrorCode, format string, args ...interface{}) *AppError {
	if err == nil {
		return Newf(code, format, args...)
	}
	
	return &AppError{
		Code:       code,
		Message:    fmt.Sprintf(format, args...),
		Severity:   SeverityMedium,
		HTTPStatus: http.StatusInternalServerError,
		Cause:      err,
	}
}

// WithContext adds context to an error
func (e *AppError) WithContext(ctx ErrorContext) *AppError {
	e.Context = ctx
	return e
}

// WithSeverity sets the severity of an error
func (e *AppError) WithSeverity(severity ErrorSeverity) *AppError {
	e.Severity = severity
	return e
}

// WithHTTPStatus sets the HTTP status code for an error
func (e *AppError) WithHTTPStatus(status int) *AppError {
	e.HTTPStatus = status
	return e
}

// ToHTTPResponse converts an AppError to an HTTP response
func (e *AppError) ToHTTPResponse() map[string]interface{} {
	response := map[string]interface{}{
		"error": map[string]interface{}{
			"code":    string(e.Code),
			"message": e.Message,
		},
	}
	
	if e.Severity != "" {
		response["error"].(map[string]interface{})["severity"] = string(e.Severity)
	}
	
	if e.Context.ProviderID != "" {
		response["error"].(map[string]interface{})["provider_id"] = e.Context.ProviderID
	}
	
	if e.Context.TokenID != "" {
		response["error"].(map[string]interface{})["token_id"] = e.Context.TokenID
	}
	
	if e.Context.RequestID != "" {
		response["error"].(map[string]interface{})["request_id"] = e.Context.RequestID
	}
	
	return response
}

// Predefined errors for common scenarios
var (
	ErrTokenExpired = New(ErrCodeTokenExpired, "Token has expired and needs to be refreshed")
	
	ErrTokenNotFound = New(ErrCodeTokenNotFound, "Token not found")
	
	ErrInvalidModel = New(ErrCodeInvalidModel, "Invalid model specified")
	
	ErrProviderUnavailable = New(ErrCodeProviderUnavailable, "Provider is currently unavailable")
	
	ErrRateLimitExceeded = New(ErrCodeRateLimitExceeded, "Rate limit exceeded. Please retry later.")
	
	ErrEmailExtractionFailed = New(ErrCodeEmailExtractionFailed, "Failed to extract email from token")
	
	ErrUnauthorized = New(ErrCodeUnauthorized, "Unauthorized access")
)
```

#### Step 1.2: Create HTTP Error Handler

**New File:** `internal/errors/http_handler.go`

```go
package errors

import (
	"encoding/json"
	"net/http"
)

// WriteError writes an error response in JSON format
func WriteError(w http.ResponseWriter, err *AppError) {
	w.Header().Set("Content-Type", "application/json")
	
	status := err.HTTPStatus
	if status == 0 {
		status = http.StatusInternalServerError
	}
	
	w.WriteHeader(status)
	
	response := err.ToHTTPResponse()
	json.NewEncoder(w).Encode(response)
}

// WriteErrorWithDetails writes an error response with additional details
func WriteErrorWithDetails(w http.ResponseWriter, err *AppError, details map[string]interface{}) {
	w.Header().Set("Content-Type", "application/json")
	
	status := err.HTTPStatus
	if status == 0 {
		status = http.StatusInternalServerError
	}
	
	w.WriteHeader(status)
	
	response := err.ToHTTPResponse()
	if details != nil {
		if errorResp, ok := response["error"].(map[string]interface{}); ok {
			errorResp["details"] = details
		}
	}
	
	json.NewEncoder(w).Encode(response)
}

// WriteJSON writes a successful JSON response
func WriteJSON(w http.ResponseWriter, status int, data interface{}) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	return json.NewEncoder(w).Encode(data)
}
```

### Phase 2: Update REST API (Day 1)

#### Step 2.1: Replace Error Handling in rest_api.go

**Modify File:** `internal/restapi/rest_api.go`

**Location:** Replace error handling throughout the file

```go
// Add import
import (
	// ... existing imports ...
	"github.com/sunbankio/qwencoder-proxy/internal/errors"
)

// BEFORE: Generic error handling
if err != nil {
	s.logger.WarnLog("Failed to extract email for %s: %v", providerID, err)
	email = "unknown@example.com"
}

// AFTER: Centralized error handling
if err != nil {
	s.logger.WarnLog("Failed to extract email for %s: %v", providerID, err)
	email = "unknown@example.com"
	// Optionally wrap error for better context
	// err = errors.Wrapf(err, errors.ErrCodeEmailExtractionFailed, 
	//     "failed to extract email for provider %s", providerID)
}
```

**More examples:**

```go
// BEFORE: Generic error at line 1173
return "", fmt.Errorf("failed to get email extractor for %s: %w", providerID, err)

// AFTER: Centralized error with context
return "", errors.Wrapf(err, errors.ErrCodeProviderUnavailable, 
	"failed to get email extractor for provider %s", providerID).WithContext(errors.ErrorContext{
	ProviderID: providerID,
	Operation:  "get_email_extractor",
})
```

```go
// BEFORE: Generic error at line 872
s.writeCallbackHTML(w, false, "save_failed", err.Error())

// AFTER: Centralized error
appErr := errors.Wrapf(err, errors.ErrCodeStorageError, 
	"failed to save credentials for provider %s", providerID).WithContext(errors.ErrorContext{
	ProviderID: providerID,
	Operation:  "save_credentials",
})
s.writeCallbackHTMLWithError(w, appErr)
```

#### Step 2.2: Update writeCallbackHTML

**Modify File:** `internal/restapi/rest_api.go`

**Location:** Modify `writeCallbackHTML()` function (around line 880)

```go
// BEFORE: Separate success/error paths
func (s *Server) writeCallbackHTML(w http.ResponseWriter, success bool, errorCode, errorDesc string) {
	w.Header().Set("Content-Type", "text/html")
	
	if success {
		// ... success HTML ...
	} else {
		// ... error HTML with errorCode and errorDesc ...
	}
}

// AFTER: Use centralized error handling
func (s *Server) writeCallbackHTML(w http.ResponseWriter, success bool, errorCode, errorDesc string) {
	w.Header().Set("Content-Type", "text/html")
	
	if success {
		html := `<!DOCTYPE html>
<html>
<head><title>Authorization Successful</title></head>
<body>
   <h1>Authorization Successful!</h1>
   <p>You can close this window and return to your application.</p>
   <script>
     if (window.opener) {
       window.opener.postMessage({type: 'oauth_success'}, '*');
     }
     setTimeout(() => window.close(), 2000);
   </script>
</body>
</html>`
		w.Write([]byte(html))
	} else {
		// Create standardized error
		var appErr *errors.AppError
		if errorCode != "" {
			appErr = errors.Newf(errors.ErrorCode(errorCode), errorDesc)
		} else {
			appErr = errors.New(errors.ErrCodeProviderUnavailable, errorDesc)
		}
		
		html := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head><title>Authorization Failed</title></head>
<body>
   <h1>Authorization Failed</h1>
   <p>Error: %s</p>
   <p>%s</p>
   <p>Please try again.</p>
</body>
</html>`, appErr.Code, appErr.Message)
		w.Write([]byte(html))
	}
}

// Add helper method
func (s *Server) writeCallbackHTMLWithError(w http.ResponseWriter, err *errors.AppError) {
	s.writeCallbackHTML(w, false, string(err.Code), err.Message)
}
```

### Phase 3: Update Proxy Handlers (Day 2)

#### Step 3.1: Replace Error Handling in openai_handler.go

**Modify File:** `internal/proxy/openai_handler.go`

**Location:** Replace error handling throughout the file

```go
// Add import
import (
	// ... existing imports ...
	"github.com/sunbankio/qwencoder-proxy/internal/errors"
)

// BEFORE: Generic error at line 177
http.Error(w, fmt.Sprintf("Provider not found: %v", err), http.StatusBadRequest)

// AFTER: Centralized error with context
appErr := errors.Wrapf(err, errors.ErrCodeProviderUnavailable, 
	"provider not found: %v", err).WithContext(errors.ErrorContext{
	Operation: "get_provider",
}).WithHTTPStatus(http.StatusBadRequest)
errors.WriteError(w, appErr)
```

```go
// BEFORE: Generic error at line 186
http.Error(w, "Protocol conversion not supported", http.StatusInternalServerError)

// AFTER: Centralized error
appErr := errors.New(errors.ErrCodeConversionError, 
	"protocol conversion not supported").WithHTTPStatus(http.StatusInternalServerError)
errors.WriteError(w, appErr)
```

```go
// BEFORE: Generic error at line 218
h.GetLogger().ErrorLog("[Handler] GenerateAndConvert failed with %s: %v", p.Name(), err)

// AFTER: Centralized error
appErr := errors.Wrapf(err, errors.ErrCodeProviderError, 
	"GenerateAndConvert failed with provider %s", p.Name()).WithContext(errors.ErrorContext{
	ProviderID: string(p.ProviderType()),
	Operation:  "generate_and_convert",
})
h.GetLogger().ErrorLog("[Handler] %v", appErr)
```

#### Step 3.2: Replace Error Handling in gemini_handler.go

**Modify File:** `internal/proxy/gemini_handler.go`

**Location:** Replace error handling throughout the file

```go
// Add import
import (
	// ... existing imports ...
	"github.com/sunbankio/qwencoder-proxy/internal/errors"
)

// Replace all http.Error() calls with errors.WriteError()
// Replace all fmt.Errorf() calls with errors.New() or errors.Wrapf()
```

#### Step 3.3: Replace Error Handling in anthropic_handler.go

**Modify File:** `internal/proxy/anthropic_handler.go`

**Location:** Replace error handling throughout the file

```go
// Add import
import (
	// ... existing imports ...
	"github.com/sunbankio/qwencoder-proxy/internal/errors"
)

// Replace all error handling with centralized errors package
```

### Phase 4: Update Token Package (Day 2)

#### Step 4.1: Replace Error Handling in token files

**Modify Files:** 
- `internal/token/multi_token_manager.go`
- `internal/token/sqlite_store.go`
- `internal/token/token_selection.go`

```go
// Add import
import (
	// ... existing imports ...
	"github.com/sunbankio/qwencoder-proxy/internal/errors"
)

// Replace all fmt.Errorf() calls with errors.New() or errors.Wrapf()
// Add context to errors where appropriate
```

**Example in `multi_token_manager.go`:**

```go
// BEFORE: Generic error at line 129
return fmt.Errorf("multi-token manager not initialized")

// AFTER: Centralized error
return errors.New(errors.ErrCodeStorageError, 
	"multi-token manager not initialized").WithSeverity(errors.SeverityCritical)
```

**Example in `sqlite_store.go`:**

```go
// BEFORE: Generic error at line 500
return fmt.Errorf("failed to insert token: %w", err)

// AFTER: Centralized error with context
return errors.Wrapf(err, errors.ErrCodeDatabaseError, 
	"failed to insert token").WithContext(errors.ErrorContext{
	Operation: "insert_token",
})
```

### Phase 5: Update Middleware (Day 2)

#### Step 5.1: Replace Error Handling in middleware.go

**Modify File:** `internal/restapi/middleware.go`

**Location:** Replace error handling functions

```go
// Add import
import (
	// ... existing imports ...
	"github.com/sunbankio/qwencoder-proxy/internal/errors"
)

// BEFORE: Generic error at line 127
return fmt.Errorf("failed to parse JSON: %w", err)

// AFTER: Centralized error
return errors.Wrapf(err, errors.ErrCodeInvalidRequest, 
	"failed to parse JSON: %w", err)
```

```go
// BEFORE: Generic WriteError function at line 100
func WriteError(w http.ResponseWriter, status int, code, message string) error {
	errResp := ErrorResponse{
		Error: ErrorDetail{
			Code:    code,
			Message: message,
		},
	}
	return WriteJSON(w, status, errResp)
}

// AFTER: Use centralized error handling
func WriteError(w http.ResponseWriter, status int, code, message string) error {
	appErr := errors.Newf(errors.ErrorCode(code), message).WithHTTPStatus(status)
	return errors.WriteError(w, appErr)
}
```

---

## Testing

### Unit Tests

**New File:** `internal/errors/errors_test.go`

```go
package errors

import (
	"net/http"
	"testing"
	"github.com/stretchr/testify/assert"
)

func TestNewError(t *testing.T) {
	err := New(ErrCodeTokenExpired, "Token has expired")
	
	assert.Equal(t, ErrCodeTokenExpired, err.Code)
	assert.Equal(t, "Token has expired", err.Message)
	assert.Equal(t, SeverityMedium, err.Severity)
	assert.Equal(t, http.StatusInternalServerError, err.HTTPStatus)
}

func TestWrapError(t *testing.T) {
	original := New(ErrCodeTokenExpired, "Original error")
	wrapped := Wrap(original, ErrCodeProviderUnavailable, "Wrapper message")
	
	assert.Equal(t, ErrCodeProviderUnavailable, wrapped.Code)
	assert.Equal(t, "Wrapper message", wrapped.Message)
	assert.Equal(t, original, wrapped.Unwrap())
}

func TestWrapfError(t *testing.T) {
	original := New(ErrCodeTokenExpired, "Original error")
	wrapped := Wrapf(original, ErrCodeProviderUnavailable, 
		"Wrapper message for %s", "test")
	
	assert.Equal(t, ErrCodeProviderUnavailable, wrapped.Code)
	assert.Contains(t, wrapped.Message, "test")
	assert.Equal(t, original, wrapped.Unwrap())
}

func TestErrorWithContext(t *testing.T) {
	err := New(ErrCodeTokenExpired, "Test error")
	
	ctx := ErrorContext{
		ProviderID: "test-provider",
		TokenID:    "test-token",
		Operation:   "test-operation",
	}
	
	errWithContext := err.WithContext(ctx)
	
	assert.Equal(t, "test-provider", errWithContext.Context.ProviderID)
	assert.Equal(t, "test-token", errWithContext.Context.TokenID)
	assert.Equal(t, "test-operation", errWithContext.Context.Operation)
}

func TestErrorWithSeverity(t *testing.T) {
	err := New(ErrCodeTokenExpired, "Test error")
	
	errWithSeverity := err.WithSeverity(SeverityCritical)
	
	assert.Equal(t, SeverityCritical, errWithSeverity.Severity)
}

func TestErrorWithHTTPStatus(t *testing.T) {
	err := New(ErrCodeTokenExpired, "Test error")
	
	errWithStatus := err.WithHTTPStatus(http.StatusUnauthorized)
	
	assert.Equal(t, http.StatusUnauthorized, errWithStatus.HTTPStatus)
}

func TestErrorToHTTPResponse(t *testing.T) {
	err := New(ErrCodeTokenExpired, "Test error").
		WithContext(ErrorContext{
			ProviderID: "test-provider",
			TokenID:    "test-token",
		}).
		WithHTTPStatus(http.StatusUnauthorized)
	
	response := err.ToHTTPResponse()
	
	assert.NotNil(t, response)
	assert.NotNil(t, response["error"])
	
	errorResp := response["error"].(map[string]interface{})
	assert.Equal(t, string(ErrCodeTokenExpired), errorResp["code"])
	assert.Equal(t, "Test error", errorResp["message"])
	assert.Equal(t, string(SeverityMedium), errorResp["severity"])
	assert.Equal(t, "test-provider", errorResp["provider_id"])
	assert.Equal(t, "test-token", errorResp["token_id"])
}

func TestPredefinedErrors(t *testing.T) {
	assert.Equal(t, ErrCodeTokenExpired, ErrTokenExpired.Code)
	assert.Equal(t, "Token has expired", ErrTokenExpired.Message)
	
	assert.Equal(t, ErrCodeTokenNotFound, ErrTokenNotFound.Code)
	assert.Equal(t, "Token not found", ErrTokenNotFound.Message)
	
	assert.Equal(t, ErrCodeInvalidModel, ErrInvalidModel.Code)
	assert.Equal(t, "Invalid model specified", ErrInvalidModel.Message)
}
```

---

## Verification Checklist

### Phase 1: Error Package
- [ ] `internal/errors/errors.go` created
- [ ] `internal/errors/http_handler.go` created
- [ ] All error codes defined
- [ ] ErrorContext struct defined
- [ ] AppError struct defined
- [ ] All helper methods implemented
- [ ] Predefined errors created
- [ ] Unit tests pass

### Phase 2: REST API Updates
- [ ] `rest_api.go` imports errors package
- [ ] All error handling in `rest_api.go` updated
- [ ] `writeCallbackHTML()` uses centralized errors
- [ ] All tests pass

### Phase 3: Proxy Handler Updates
- [ ] `openai_handler.go` imports errors package
- [ ] All error handling in `openai_handler.go` updated
- [ ] `gemini_handler.go` imports errors package
- [ ] All error handling in `gemini_handler.go` updated
- [ ] `anthropic_handler.go` imports errors package
- [ ] All error handling in `anthropic_handler.go` updated
- [ ] All tests pass

### Phase 4: Token Package Updates
- [ ] `multi_token_manager.go` imports errors package
- [ ] All error handling in `multi_token_manager.go` updated
- [ ] `sqlite_store.go` imports errors package
- [ ] All error handling in `sqlite_store.go` updated
- [ ] `token_selection.go` imports errors package
- [ ] All error handling in `token_selection.go` updated
- [ ] All tests pass

### Phase 5: Middleware Updates
- [ ] `middleware.go` imports errors package
- [ ] All error handling in `middleware.go` updated
- [ ] `WriteError()` uses centralized errors
- [ ] All tests pass

---

## Impact

**Positive:**
- Consistent error handling across codebase
- Better error messages for users
- Easier debugging with error context
- Standardized error codes for API consumers
- Better error tracking and monitoring
- Reduced code duplication

**Code Volume Reduction:**
- Removed ~200 lines of duplicate error handling
- Added ~300 lines in centralized error package
- Net addition: ~100 lines (but with much better maintainability)

**Risk:**
- Low - centralized error handling is a well-established pattern
- Backward compatible (errors still implement error interface)
- No breaking changes to existing functionality

**Side Effects:**
- All error responses will have consistent format
- Error codes will be standardized
- Error context will be available in all errors
- Better logging with structured errors

---

## Future Enhancements

1. **Error Recovery:**
   - Panic recovery middleware
   - Graceful degradation on errors
   - Fallback mechanisms

2. **Error Aggregation:**
   - Collect errors across requests
   - Error pattern analysis
   - Error rate limiting

3. **Error Notifications:**
   - Email alerts for critical errors
   - Slack/webhook integration
   - Error dashboards

4. **Error Documentation:**
   - Auto-generate error documentation
   - Error code catalog
   - Troubleshooting guides

5. **Internationalization:**
   - Multi-language error messages
   - Locale-specific error formats
   - User-friendly error descriptions
