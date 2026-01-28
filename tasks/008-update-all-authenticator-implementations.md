# Task 008: Update All Authenticator Implementations

## Description
Implement full proxy-aware functionality in all authenticator implementations. Each authenticator will use proxy-aware HTTP clients for authentication operations.

## Technical Context
This task implements the proxy-aware methods added to the Authenticator interface in Task 007. Each authenticator needs to integrate with the proxy client factory.

**Authenticator Implementations to Update**:

1. **QwenAuthenticator** ([`provider/qwen/qwen.go:41`](provider/qwen/qwen.go:41))
   - Uses `qwenclient.GetValidTokenAndEndpoint()` for token management
   - Needs proxy-aware HTTP client for API calls

2. **GeminiAuthenticator** ([`auth/gemini_auth.go`](auth/gemini_auth.go))
   - Uses OAuth device code flow
   - Makes HTTP calls for token exchange

3. **IFlowAuthenticator** ([`auth/iflow_auth.go`](auth/iflow_auth.go))
   - Uses OAuth authorization code flow
   - Makes HTTP calls for token exchange

4. **KiroAuthenticator** ([`auth/kiro_auth.go`](auth/kiro_auth.go))
   - Uses social authentication
   - Makes HTTP calls for auth endpoints

5. **OAuthAuthenticator** ([`auth/oauth.go`](auth/oauth.go))
   - Generic OAuth implementation
   - Makes HTTP calls for token operations

6. **OAuth2DeviceAuthenticator** ([`auth/oauth2_device.go`](auth/oauth2_device.go))
   - OAuth 2.0 device code flow
   - Makes HTTP calls for device code and token exchange

**Implementation Pattern**:
```go
type Authenticator struct {
    // existing fields...
    tokenManager *TokenManager  // NEW
    httpClient   *http.Client   // Keep for backward compatibility
}

func (a *Authenticator) GetTokenWithClient(ctx context.Context) (string, *http.Client, error) {
    token, client, err := a.tokenManager.SelectTokenWithClient()
    if err != nil {
        return "", nil, err
    }
    return token.AccessToken, client, nil
}

func (a *Authenticator) GetHTTPClient() (*http.Client, error) {
    token, client, err := a.tokenManager.SelectTokenWithClient()
    if err != nil {
        return nil, err
    }
    return client, nil
}
```

**Integration Points**:
- [`auth/token_selection.go`](auth/token_selection.go) - TokenManager provides SelectTokenWithClient()
- [`config/proxy_client_factory.go`](config/proxy_client_factory.go) - Creates proxy-aware clients
- [`auth/multi_token_store.go`](auth/multi_token_store.go) - Provides tokens with proxy config

**Architectural Principles**:
- **Dependency Injection**: Authenticators receive TokenManager
- **Open/Closed**: Extends existing authenticators without breaking changes
- **Single Responsibility**: Authenticators handle auth, proxy handled by factory

## Subtasks
1. Update QwenAuthenticator ([`provider/qwen/qwen.go:41`](provider/qwen/qwen.go:41)):
   - Add `tokenManager *TokenManager` field
   - Implement GetTokenWithClient() using tokenManager.SelectTokenWithClient()
   - Implement GetHTTPClient() using tokenManager.SelectTokenWithClient()
   - Update constructor to accept tokenManager parameter
   - Update qwenclient to use proxy-aware client (if needed)
2. Update GeminiAuthenticator ([`auth/gemini_auth.go`](auth/gemini_auth.go)):
   - Add `tokenManager *TokenManager` field
   - Implement GetTokenWithClient() using tokenManager.SelectTokenWithClient()
   - Implement GetHTTPClient() using tokenManager.SelectTokenWithClient()
   - Update constructor to accept tokenManager parameter
   - Update OAuth device code flow to use proxy-aware client
3. Update IFlowAuthenticator ([`auth/iflow_auth.go`](auth/iflow_auth.go)):
   - Add `tokenManager *TokenManager` field
   - Implement GetTokenWithClient() using tokenManager.SelectTokenWithClient()
   - Implement GetHTTPClient() using tokenManager.SelectTokenWithClient()
   - Update constructor to accept tokenManager parameter
   - Update OAuth authorization code flow to use proxy-aware client
4. Update KiroAuthenticator ([`auth/kiro_auth.go`](auth/kiro_auth.go)):
   - Add `tokenManager *TokenManager` field
   - Implement GetTokenWithClient() using tokenManager.SelectTokenWithClient()
   - Implement GetHTTPClient() using tokenManager.SelectTokenWithClient()
   - Update constructor to accept tokenManager parameter
   - Update social auth flow to use proxy-aware client
5. Update OAuthAuthenticator ([`auth/oauth.go`](auth/oauth.go)):
   - Add `tokenManager *TokenManager` field
   - Implement GetTokenWithClient() using tokenManager.SelectTokenWithClient()
   - Implement GetHTTPClient() using tokenManager.SelectTokenWithClient()
   - Update constructor to accept tokenManager parameter
   - Update token exchange to use proxy-aware client
6. Update OAuth2DeviceAuthenticator ([`auth/oauth2_device.go`](auth/oauth2_device.go)):
   - Add `tokenManager *TokenManager` field
   - Implement GetTokenWithClient() using tokenManager.SelectTokenWithClient()
   - Implement GetHTTPClient() using tokenManager.SelectTokenWithClient()
   - Update constructor to accept tokenManager parameter
   - Update device code flow to use proxy-aware client
7. Add error handling for proxy failures:
   - Update proxy health on auth failures
   - Distinguish proxy errors from auth errors
   - Log proxy-related errors
8. Add unit tests for each authenticator:
   - Test GetTokenWithClient() returns proxy-aware client
   - Test GetHTTPClient() returns proxy-aware client
   - Test backward compatibility with existing methods
   - Test error handling for proxy failures

## Dependencies
- Task 006: Add SelectTokenWithClient Method to TokenManager (uses SelectTokenWithClient)
- Task 007: Add GetHTTPClient Method to Authenticator Interface (implements interface methods)

## Files to Modify
- `provider/qwen/qwen.go` (update QwenAuthenticator)
- `auth/gemini_auth.go` (update GeminiAuthenticator)
- `auth/iflow_auth.go` (update IFlowAuthenticator)
- `auth/kiro_auth.go` (update KiroAuthenticator)
- `auth/oauth.go` (update OAuthAuthenticator)
- `auth/oauth2_device.go` (update OAuth2DeviceAuthenticator)
- Update respective test files

## Related Tasks
- Task 015: Update token refresh implementations (uses proxy-aware authenticators)
- Task 009: Extend Provider interface for token manager injection (TokenManager passed to providers)
