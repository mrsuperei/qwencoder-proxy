# Task 007: Add GetHTTPClient Method to Authenticator Interface

## Description
Extend the Authenticator interface to optionally return an HTTP client configured with the token's proxy settings. This enables authenticators to provide proxy-aware clients for authentication operations.

## Technical Context
This task extends the existing Authenticator interface to support proxy-aware authentication operations.

**Current Authenticator Interface** ([`provider/provider.go:63`](provider/provider.go:63)):
```go
type Authenticator interface {
    Authenticate(ctx context.Context) error
    GetToken(ctx context.Context) (string, error)
    IsAuthenticated() bool
    GetCredentialsPath() string
    ClearCredentials() error
}
```

**Extended Authenticator Interface** (after this task):
```go
type Authenticator interface {
    Authenticate(ctx context.Context) error
    GetToken(ctx context.Context) (string, error)
    GetTokenWithClient(ctx context.Context) (string, *http.Client, error)  // NEW
    IsAuthenticated() bool
    GetCredentialsPath() string
    ClearCredentials() error
    GetHTTPClient() (*http.Client, error)  // NEW - optional
}
```

**Rationale for New Methods**:

1. **GetTokenWithClient()**: Returns both token and configured HTTP client
   - Used by providers for API requests
   - Returns client with proxy configuration from token
   - Maintains backward compatibility with GetToken()

2. **GetHTTPClient()**: Optional method to get proxy-aware client
   - Used for authentication operations (OAuth flows, token refresh)
   - Returns client configured with proxy settings
   - Optional method (not all authenticators need to implement)

**Affected Authenticator Implementations**:
- [`provider/qwen/qwen.go:41`](provider/qwen/qwen.go:41) - QwenAuthenticator
- [`auth/gemini_auth.go`](auth/gemini_auth.go) - GeminiAuthenticator
- [`auth/iflow_auth.go`](auth/iflow_auth.go) - IFlowAuthenticator
- [`auth/kiro_auth.go`](auth/kiro_auth.go) - KiroAuthenticator
- [`auth/oauth.go`](auth/oauth.go) - OAuthAuthenticator
- [`auth/oauth2_device.go`](auth/oauth2_device.go) - OAuth2DeviceAuthenticator

**Architectural Principles**:
- **Interface Segregation**: New methods are optional
- **Open/Closed**: Extends interface without breaking existing implementations
- **Dependency Inversion**: Authenticators depend on HTTPClient abstraction

## Subtasks
1. Modify [`provider/provider.go`](provider/provider.go) Authenticator interface:
   - Add GetTokenWithClient(ctx context.Context) (string, *http.Client, error) method
   - Add GetHTTPClient() (*http.Client, error) method
   - Add documentation explaining optional nature of GetHTTPClient()
2. Update all authenticator implementations to support new methods:
   - QwenAuthenticator (in [`provider/qwen/qwen.go`](provider/qwen/qwen.go:41))
   - GeminiAuthenticator (in [`auth/gemini_auth.go`](auth/gemini_auth.go))
   - IFlowAuthenticator (in [`auth/iflow_auth.go`](auth/iflow_auth.go))
   - KiroAuthenticator (in [`auth/kiro_auth.go`](auth/kiro_auth.go))
   - OAuthAuthenticator (in [`auth/oauth.go`](auth/oauth.go))
   - OAuth2DeviceAuthenticator (in [`auth/oauth2_device.go`](auth/oauth2_device.go))
3. For each authenticator:
   - Implement GetTokenWithClient() to call GetToken() and return nil for client (default)
   - Implement GetHTTPClient() to return error (not implemented by default)
   - Add TODO comments for future proxy-aware implementations
4. Add unit tests for interface changes:
   - Test backward compatibility (existing methods still work)
   - Test default implementations of new methods
5. Update documentation for Authenticator interface

## Dependencies
- Task 002: Create HTTP Client Interface (uses HTTPClient abstraction)

## Files to Modify
- `provider/provider.go` (extend Authenticator interface)
- `provider/qwen/qwen.go` (update QwenAuthenticator)
- `auth/gemini_auth.go` (update GeminiAuthenticator)
- `auth/iflow_auth.go` (update IFlowAuthenticator)
- `auth/kiro_auth.go` (update KiroAuthenticator)
- `auth/oauth.go` (update OAuthAuthenticator)
- `auth/oauth2_device.go` (update OAuth2DeviceAuthenticator)

## Related Tasks
- Task 008: Update all authenticator implementations (full proxy-aware implementation)
- Task 015: Update token refresh implementations (uses GetTokenWithClient)
