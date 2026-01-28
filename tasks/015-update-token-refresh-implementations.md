# Task 015: Update Token Refresh Implementations

## Description
Update all token refresh implementations to use proxy-aware HTTP clients. This ensures token refresh operations use the same proxy configuration as the token being refreshed.

## Technical Context
This task updates token refresh implementations to integrate with proxy-per-token architecture.

**Current ProviderRefresh Interface** ([`auth/token_refresh.go:35`](auth/token_refresh.go:35)):
```go
type ProviderRefresh interface {
    RefreshToken(ctx context.Context, token ProviderToken) (*ProviderToken, error)
    CanRefresh(token ProviderToken) bool
}
```

**Current ProviderRefresh Implementations**:
- All have `httpClient *http.Client` field
- Use static client for refresh requests
- Need to use proxy-aware client based on token's proxy config

**Refresh Flow with Proxy**:
```
1. RefreshCoordinator retrieves token with proxy config
2. Refresher gets HTTP client from factory with token's proxy config
3. Refresh request made through proxy
4. Proxy health updated based on refresh success/failure
5. Updated token saved with proxy health
```

**Affected Refresh Implementations**:
- [`auth/token_refresh.go:390`](auth/token_refresh.go:390) - QwenRefresher
- [`auth/token_refresh.go:448`](auth/token_refresh.go:448) - GeminiRefresher
- [`auth/token_refresh.go:528`](auth/token_refresh.go:528) - IFlowRefresher
- [`auth/token_refresh.go:628`](auth/token_refresh.go:628) - KiroRefresher

**Integration Points**:
- [`auth/token_refresh.go:43`](auth/token_refresh.go:43) - RefreshCoordinator
- [`config/proxy_client_factory.go`](config/proxy_client_factory.go) - Creates proxy-aware clients
- [`auth/proxy_health_tracker.go`](auth/proxy_health_tracker.go) - Tracks proxy health

**Architectural Principles**:
- **Dependency Injection**: Refreshers receive ProxyAwareHTTPClientFactory
- **Single Responsibility**: Refreshers handle token refresh, proxy handled by factory
- **Open/Closed**: Extends refreshers without breaking existing functionality

## Subtasks
1. Modify [`auth/token_refresh.go:35`](auth/token_refresh.go:35) ProviderRefresh interface:
   - Add `clientFactory *ProxyAwareHTTPClientFactory` field to implementations
   - Update RefreshToken() to use proxy-aware client
2. Update QwenRefresher ([`auth/token_refresh.go:390`](auth/token_refresh.go:390)):
   - Add `clientFactory *ProxyAwareHTTPClientFactory` field
   - Update RefreshToken() to get client from factory
   - Use client.GetClient(token.Proxy) for refresh request
   - Add error handling for proxy failures
   - Update proxy health on success/failure
3. Update GeminiRefresher ([`auth/token_refresh.go:448`](auth/token_refresh.go:448)):
   - Add `clientFactory *ProxyAwareHTTPClientFactory` field
   - Update RefreshToken() to get client from factory
   - Use client.GetClient(token.Proxy) for refresh request
   - Add error handling for proxy failures
   - Update proxy health on success/failure
4. Update IFlowRefresher ([`auth/token_refresh.go:528`](auth/token_refresh.go:528)):
   - Add `clientFactory *ProxyAwareHTTPClientFactory` field
   - Update RefreshToken() to get client from factory
   - Use client.GetClient(token.Proxy) for refresh request
   - Add error handling for proxy failures
   - Update proxy health on success/failure
5. Update KiroRefresher ([`auth/token_refresh.go:628`](auth/token_refresh.go:628)):
   - Add `clientFactory *ProxyAwareHTTPClientFactory` field
   - Update RefreshToken() to get client from factory
   - Use client.GetClient(token.Proxy) for refresh request
   - Add error handling for proxy failures
   - Update proxy health on success/failure
6. Update RefreshCoordinator ([`auth/token_refresh.go:43`](auth/token_refresh.go:43)):
   - Pass clientFactory to refreshers when creating them
   - Update refresher registration to include clientFactory
7. Add error classification:
   - Detect proxy connection errors
   - Detect proxy timeout errors
   - Detect proxy DNS errors
   - Update proxy health via TokenManager.UpdateProxyHealth()
8. Add logging for:
   - Token refresh with proxy
   - Proxy-aware client usage
   - Proxy error events
   - Proxy health updates
9. Add unit tests for:
   - Token refresh with proxy
   - Token refresh without proxy
   - Proxy error handling during refresh
   - Proxy health updates
   - All refresher implementations

## Dependencies
- Task 003: Create Proxy-Aware HTTP Client Factory (uses ProxyAwareHTTPClientFactory)
- Task 004: Extend Token Metadata with Proxy Configuration (uses ProviderToken.Proxy)

## Files to Modify
- `auth/token_refresh.go` (update all refresher implementations)
- `auth/token_refresh_test.go` (update tests)

## Related Tasks
- Task 008: Update all authenticator implementations (refreshers may use authenticators)
