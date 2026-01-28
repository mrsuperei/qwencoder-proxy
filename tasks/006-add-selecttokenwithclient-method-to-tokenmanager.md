# Task 006: Add SelectTokenWithClient Method to TokenManager

## Description
Extend TokenManager to return both a selected token and its configured HTTP client. This enables providers to use token-specific proxy configurations for API requests.

## Technical Context
This task is critical for proxy-per-token functionality, enabling the integration between token selection and proxy-aware HTTP clients.

**Current TokenManager** ([`auth/token_selection.go:127`](auth/token_selection.go:127)):
```go
type TokenManager struct {
    store          *MultiTokenStore
    strategy       SelectionStrategy
    providerType   provider.ProviderType
    logger         *logging.Logger
    // ... other fields
}

func (tm *TokenManager) SelectToken() (*ProviderToken, error) {
    // Uses selection strategy to pick token
    // Returns token only
}
```

**Extended TokenManager** (after this task):
```go
type TokenManager struct {
    store              *MultiTokenStore
    strategy           SelectionStrategy
    providerType       provider.ProviderType
    logger             *logging.Logger
    clientFactory      *ProxyAwareHTTPClientFactory  // NEW
    proxyHealthTracker  *ProxyHealthTracker        // NEW
}

func (tm *TokenManager) SelectTokenWithClient() (*ProviderToken, *http.Client, error) {
    // Select token using existing strategy
    // Get proxy config from token
    // Get client from factory with proxy config
    // Return both token and client
}
```

**SelectTokenWithClient() Flow**:
1. Call existing SelectionStrategy.SelectToken() to get ProviderToken
2. Get ProviderToken from MultiTokenStore with full metadata
3. Extract ProxyConfig from token (may be nil)
4. Call ProxyAwareHTTPClientFactory.GetClient(proxyConfig)
5. Update proxy health metrics via ProxyHealthTracker
6. Return (token, client, nil)

**Related Components**:
- [`auth/token_selection.go`](auth/token_selection.go) - TokenManager implementation
- [`auth/proxy_client_factory.go`](config/proxy_client_factory.go) - Creates proxy-aware clients
- [`auth/proxy_health_tracker.go`](auth/proxy_health_tracker.go) - Tracks proxy health
- [`auth/multi_token_store.go`](auth/multi_token_store.go) - Provides token with proxy config

**Architectural Principles**:
- **Open/Closed**: Extends existing TokenManager without breaking changes
- **Dependency Inversion**: Depends on HTTPClient interface abstraction
- **Strategy Pattern**: Existing selection strategies remain unchanged

## Subtasks
1. Modify [`auth/token_selection.go`](auth/token_selection.go) TokenManager struct:
   - Add `clientFactory *ProxyAwareHTTPClientFactory` field
   - Add `proxyHealthTracker *ProxyHealthTracker` field
2. Implement SelectTokenWithClient() (*ProviderToken, *http.Client, error) method:
   - Call existing SelectToken() to get token ID
   - Get full ProviderToken from store
   - Extract ProxyConfig from token
   - Call clientFactory.GetClient(proxyConfig)
   - Log client creation/caching events
   - Return (token, client, nil)
3. Implement GetTokenClient(tokenID string) (*http.Client, error) method:
   - Get ProviderToken from store
   - Extract ProxyConfig
   - Return client from factory
4. Implement UpdateProxyHealth(tokenID string, healthy bool, err error) method:
   - Call proxyHealthTracker.UpdateHealth()
   - Update ProviderToken.ProxyHealthScore in store
   - Save updated token
5. Update NewTokenManager() constructor:
   - Accept clientFactory parameter
   - Accept proxyHealthTracker parameter
   - Store references in struct fields
6. Add error handling for:
   - Client factory errors
   - Proxy health tracker errors
   - Store errors
7. Add logging for:
   - Token selection with proxy
   - Client retrieval
   - Proxy health updates
8. Add unit tests for:
   - Token selection with proxy client
   - Token selection without proxy (direct connection)
   - Client caching integration
   - Proxy health updates
   - Error handling

## Dependencies
- Task 003: Create Proxy-Aware HTTP Client Factory (uses ProxyAwareHTTPClientFactory)
- Task 005: Add Proxy Health Tracking to Token Metadata (uses ProxyHealthTracker)
- Task 004: Extend Token Metadata with Proxy Configuration (uses ProviderToken with ProxyConfig)

## Files to Modify
- `auth/token_selection.go` (modify TokenManager struct and add methods)
- `auth/token_selection_test.go` (add tests for new methods)

## Related Tasks
- Task 009: Extend Provider interface for token manager injection (TokenManager will be injected)
- Task 015: Update token refresh implementations (uses SelectTokenWithClient)
