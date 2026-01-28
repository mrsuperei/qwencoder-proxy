# Per-OAuth/Device Token Proxy Support - Architectural Design

**Project**: qwencoder-proxy  
**Design Date**: 2026-01-26  
**Purpose**: Architectural design for per-token proxy configuration support

---

## Executive Summary

This document outlines the architectural design for enabling each OAuth/device token to use its own HTTPS or SOCKS5 proxy for connections to the provider. The design extends the existing multi-token storage system to support proxy configuration per token, introduces HTTP client abstractions for proxy-aware connections, and defines the integration points across the authentication, provider, and REST API layers.

The design builds upon the SOLID principles refactoring recommendations from the previous analysis, particularly the HTTP Client Interface (P1) and Token Store Interface (P1) recommendations.

---

## 1. Data Model Design

### 1.1 Proxy Configuration Structure

A new `ProxyConfig` structure will be introduced to represent proxy settings for a token:

```
ProxyConfig
├── Type: ProxyType (enum: none, http, https, socks5)
├── Host: string (optional)
├── Port: int (optional)
├── Username: string (optional, for authenticated proxies)
├── Password: string (optional, for authenticated proxies)
├── Enabled: bool (flag to enable/disable proxy without removing config)
└── HealthStatus: ProxyHealth (optional, for tracking proxy health)
```

**Rationale**:
- `Type` enum allows clear distinction between proxy protocols
- `Enabled` flag allows temporary disabling without losing configuration
- Optional fields support "no proxy" (direct connection) scenario
- `HealthStatus` enables proxy health tracking separate from token health

### 1.2 Extended ProviderToken Structure

The existing [`ProviderToken`](auth/multi_token_store.go:18) structure will be extended:

```
ProviderToken (Extended)
├── Existing fields...
├── Proxy: ProxyConfig (new field)
└── ProxyHealthScore: float64 (new field, 0.0-1.0)
```

**Backward Compatibility Strategy**:
- New fields are optional (omitempty in JSON tags)
- Existing tokens without proxy configuration default to `Type: none`, `Enabled: false`
- JSON unmarshaling handles missing proxy fields gracefully
- File-based storage format remains backward compatible

### 1.3 Proxy Health Tracking

A new `ProxyHealth` structure for tracking proxy connection health:

```
ProxyHealth
├── LastCheck: int64 (timestamp)
├── IsHealthy: bool
├── LastError: string (optional)
├── ConsecutiveFailures: int
└── AverageLatencyMs: int (optional)
```

**Rationale**:
- Enables separate health tracking for proxy vs token
- Supports proxy-specific fallback strategies
- Helps identify failing proxies for token rotation

### 1.4 Storage Schema

The JSON file storage format for each token will include the new proxy configuration:

```json
{
  "id": "uuid",
  "access_token": "...",
  "refresh_token": "...",
  "token_type": "Bearer",
  "expiry_date": 1234567890000,
  "email": "user@example.com",
  "healthy": true,
  "health_score": 1.0,
  "last_used": 1234567890000,
  "created_at": 1234567890000,
  "error_count": 0,
  "proxy": {
    "type": "socks5",
    "host": "proxy.example.com",
    "port": 1080,
    "username": "user",
    "password": "encrypted_password",
    "enabled": true
  },
  "proxy_health_score": 0.95
}
```

---

## 2. HTTP Client Abstraction Design

### 2.1 HTTP Client Interface

Following the SOLID principles analysis recommendation (P1), a `HTTPClient` interface will be introduced:

```
HTTPClient (Interface)
└── Do(req *http.Request) (*http.Response, error)
```

**Rationale**:
- Minimal interface satisfied by `*http.Client`
- Enables mocking for testing
- Supports wrapper implementations for proxy, retry, logging

### 2.2 Proxy-Aware HTTP Client Factory

A new `ProxyAwareHTTPClientFactory` will create HTTP clients with proxy configuration:

```
ProxyAwareHTTPClientFactory
├── baseConfig: HTTPClientConfig
├── proxyCache: map[ProxyConfigKey]*http.Client
├── cacheLock: sync.RWMutex
└── logger: *logging.Logger
```

**Key Methods**:
- `GetClient(proxyConfig *ProxyConfig) *http.Client` - Returns cached or creates new client
- `ClearCache()` - Clears client cache for configuration changes
- `CreateClientWithProxy(proxyConfig *ProxyConfig) *http.Client` - Creates new client with proxy

### 2.3 Client Pooling Strategy

To optimize resource usage, HTTP clients will be cached based on proxy configuration:

```
ProxyConfigKey
├── Type: ProxyType
├── Host: string
├── Port: int
└── HasAuth: bool
```

**Caching Strategy**:
- Tokens with identical proxy configurations share HTTP clients
- Cache key excludes credentials (security consideration)
- Maximum cache size limit with LRU eviction
- Connection pooling handled by Go's `http.Transport`

### 2.4 Integration with Existing Config

The existing [`config.Config`](config/config.go:39) will be extended:

```
Config (Extended)
├── Existing fields...
├── ProxyClientFactory: *ProxyAwareHTTPClientFactory (new)
└── ProxyCacheMaxSize: int (new, default: 50)
```

**Rationale**:
- Centralizes proxy-aware client creation
- Maintains single source of truth for client configuration
- Enables dependency injection throughout the application

### 2.5 Connection Reuse Considerations

- Go's `http.Transport` handles connection pooling internally
- Each proxy configuration gets its own `http.Transport` instance
- Idle connections respect `IdleConnTimeout` from existing config
- Proxy connections are not shared across different proxy configurations
- Streaming clients use separate transport to avoid timeout conflicts

---

## 3. Token Selection Integration

### 3.1 Extended TokenManager

The existing [`TokenManager`](auth/token_selection.go:127) will be modified to return configured HTTP clients:

```
TokenManager (Extended)
├── Existing fields...
├── clientFactory: *ProxyAwareHTTPClientFactory (new)
└── proxyHealthTracker: *ProxyHealthTracker (new)
```

**New Methods**:
- `SelectTokenWithClient() (*ProviderToken, *http.Client, error)` - Returns token with configured client
- `GetTokenClient(tokenID string) (*http.Client, error)` - Gets client for specific token
- `UpdateProxyHealth(tokenID string, healthy bool, err error)` - Updates proxy health

### 3.2 Token Selection Strategy Impact

Existing selection strategies ([`RandomSelectionStrategy`](auth/token_selection.go:31), [`RoundRobinSelectionStrategy`](auth/token_selection.go:61), [`LeastUsedSelectionStrategy`](auth/token_selection.go:96)) remain unchanged:

- Strategies select tokens based on token health, not proxy health
- Proxy health is tracked separately
- Token selection can optionally consider proxy health (future enhancement)

### 3.3 Provider Integration Point

Providers will receive token-specific HTTP clients through the authenticator:

```
Authenticator (Extended Interface)
├── Existing methods...
└── GetTokenWithClient(ctx context.Context) (string, *http.Client, error) (new)
```

**Rationale**:
- Maintains backward compatibility with existing `GetToken()` method
- Allows providers to receive configured HTTP clients
- Enables per-request proxy application

### 3.4 Token Selection Flow

The flow when selecting a token for a provider request:

```
1. TokenManager.SelectTokenWithClient() called
2. Selection strategy selects ProviderToken
3. TokenManager retrieves ProxyConfig from token
4. ProxyAwareHTTPClientFactory.GetClient() called with proxy config
5. Factory returns cached or creates new HTTP client with proxy
6. TokenManager returns (ProviderToken, *http.Client)
7. Provider uses returned HTTP client for API requests
```

---

## 4. Provider Integration Strategy

### 4.1 Provider Interface Changes

The existing [`Provider`](provider/provider.go:31) interface will be extended:

```
Provider (Extended Interface)
├── Existing methods...
└── SetHTTPClientFactory(factory ProxyAwareHTTPClientFactory) (new)
```

**Rationale**:
- Allows providers to use proxy-aware clients
- Maintains backward compatibility (optional method)
- Enables providers to create clients dynamically per request

### 4.2 Provider Implementation Changes

Each provider implementation ([`QwenProvider`](provider/qwen/qwen.go:34), [`GeminiProvider`](provider/gemini/gemini.go:36), etc.) will be modified:

**Current Pattern**:
```go
type Provider struct {
    httpClient *http.Client  // Single client for all requests
    ...
}
```

**New Pattern**:
```go
type Provider struct {
    clientFactory *ProxyAwareHTTPClientFactory  // Factory for creating clients
    tokenManager *auth.TokenManager  // For token selection
    ...
}

func (p *Provider) GenerateContent(ctx context.Context, model string, request interface{}) (interface{}, error) {
    // Select token with configured client
    token, client, err := p.tokenManager.SelectTokenWithClient()
    if err != nil {
        return nil, err
    }
    
    // Use token-specific client for request
    req, _ := http.NewRequestWithContext(ctx, "POST", url, body)
    resp, err := client.Do(req)
    ...
}
```

### 4.3 Token Refresh Integration

The existing [`ProviderRefresh`](auth/token_refresh.go:35) implementations will use proxy-aware clients:

```
ProviderRefresh (Extended)
├── Existing fields...
├── clientFactory: *ProxyAwareHTTPClientFactory (new)
└── token: ProviderToken (includes proxy config)
```

**Refresh Flow with Proxy**:
1. RefreshCoordinator retrieves token with proxy config
2. Refresher gets HTTP client from factory with token's proxy config
3. Refresh request made through proxy
4. Proxy health updated based on refresh success/failure

### 4.4 Streaming Considerations

For streaming requests ([`GenerateContentStream`](provider/provider.go:50)):

- Streaming clients use separate HTTP client instance
- Timeout configuration from existing `StreamingHTTPClient()` applied
- Proxy configuration applied to streaming client
- Connection pooling considerations: streaming connections not reused

---

## 5. Configuration Management API

### 5.1 REST API Endpoints

New endpoints for proxy configuration management:

```
GET    /api/credentials/{provider}/{tokenID}/proxy
       Get proxy configuration for a specific token

PUT    /api/credentials/{provider}/{tokenID}/proxy
       Update proxy configuration for a specific token

DELETE /api/credentials/{provider}/{tokenID}/proxy
       Remove proxy configuration from a token

GET    /api/credentials/{provider}/proxies
       List all proxy configurations for a provider

POST   /api/credentials/{provider}/proxy/test
       Test a proxy configuration
```

### 5.2 Request/Response Schemas

**Proxy Configuration Request**:
```json
{
  "type": "socks5",
  "host": "proxy.example.com",
  "port": 1080,
  "username": "user",
  "password": "pass",
  "enabled": true
}
```

**Proxy Configuration Response**:
```json
{
  "token_id": "uuid",
  "proxy": {
    "type": "socks5",
    "host": "proxy.example.com",
    "port": 1080,
    "username": "user",
    "password": "***",  // Masked in responses
    "enabled": true
  },
  "health_status": {
    "is_healthy": true,
    "last_check": 1234567890000,
    "average_latency_ms": 150
  }
}
```

**Proxy Test Request**:
```json
{
  "type": "socks5",
  "host": "proxy.example.com",
  "port": 1080,
  "username": "user",
  "password": "pass",
  "test_url": "https://api.provider.com/health"
}
```

**Proxy Test Response**:
```json
{
  "success": true,
  "latency_ms": 150,
  "error": ""
}
```

### 5.3 Validation Rules

**Proxy Configuration Validation**:
- `Type` must be one of: none, http, https, socks5
- `Host` required when `Type` is not none
- `Port` must be between 1 and 65535
- `Username` and `Password` must both be provided or both empty
- `Enabled` defaults to true if not specified

**Proxy Test Validation**:
- Test URL must be HTTPS (for security)
- Test timeout: 30 seconds maximum
- Only one concurrent test per proxy configuration

### 5.4 Dashboard Integration

The existing dashboard ([`web/dashboard/index.html`](web/dashboard/index.html)) will be extended:

**New UI Components**:
- Proxy configuration form per token
- Proxy health indicator
- Proxy test button
- Proxy configuration list view

**User Flow**:
1. User navigates to token details page
2. Clicks "Configure Proxy" button
3. Fills in proxy configuration form
4. Clicks "Test Proxy" to validate
5. Clicks "Save" to apply configuration
6. Dashboard shows proxy health indicator

---

## 6. Error Handling Strategy

### 6.1 Proxy Connection Failures

**Error Classification**:
- `ProxyConnectionError`: Cannot connect to proxy
- `ProxyAuthError`: Proxy authentication failed
- `ProxyTimeoutError`: Proxy request timed out
- `ProxyDNSError`: Proxy DNS resolution failed

**Handling Strategy**:
1. Log error with proxy details (excluding credentials)
2. Update proxy health status in token metadata
3. Increment consecutive failure counter
4. Mark token as unhealthy if failures exceed threshold
5. Return clear error message to user

### 6.2 Fallback Strategies

**Strategy Options**:

1. **No Fallback** (Default):
   - Request fails immediately on proxy error
   - User must manually reconfigure or disable proxy

2. **Direct Connection Fallback**:
   - On proxy failure, retry without proxy
   - Log fallback event
   - Update proxy health status
   - Configurable per token

3. **Alternative Proxy Fallback**:
   - Configure multiple proxies per token
   - Rotate to next proxy on failure
   - Future enhancement

**Configuration**:
```
ProxyConfig (Extended)
├── FallbackStrategy: FallbackType (enum: none, direct, alternate)
└── AlternateProxies: []ProxyConfig (future)
```

### 6.3 Health Tracking for Proxies

**Health Score Calculation**:
- Initial score: 1.0
- On success: `score = min(1.0, score + 0.1)`
- On failure: `score = max(0.0, score - 0.2)`
- Consecutive failures > 5: mark proxy as unhealthy

**Health Check Triggers**:
- On token refresh
- On API request failure
- Periodic background check (configurable, default: 5 minutes)
- Manual health check via API

### 6.4 Error Response Format

**Standardized Error Response**:
```json
{
  "error": "proxy_connection_failed",
  "message": "Failed to connect to proxy proxy.example.com:1080",
  "details": {
    "proxy_type": "socks5",
    "proxy_host": "proxy.example.com",
    "proxy_port": 1080,
    "original_error": "dial tcp: lookup proxy.example.com: no such host"
  },
  "suggested_action": "Check proxy configuration or disable proxy for this token"
}
```

---

## 7. Security Considerations

### 7.1 Secure Credential Storage

**Proxy Credential Storage**:
- Passwords encrypted at rest using AES-256-GCM
- Encryption key derived from system-specific secret
- Credentials never logged (masked in logs with `***`)
- Credentials excluded from API responses

**Encryption Implementation**:
```
ProxyCredentialManager
├── Encrypt(plaintext string) (string, error)
├── Decrypt(ciphertext string) (string, error)
└── keyProvider: EncryptionKeyProvider
```

### 7.2 SOCKS5 Authentication

**Authentication Methods**:
- Username/password (supported)
- No authentication (supported)
- GSSAPI/Kerberos (future enhancement)

**Security Notes**:
- SOCKS5 authentication credentials sent in clear over proxy connection
- Use HTTPS proxies when possible for credential protection
- Consider proxy trust model before configuring

### 7.3 TLS Considerations

**HTTPS Proxy Support**:
- TLS termination at proxy
- Proxy certificate validation configurable
- Support for proxy CA certificates

**Configuration**:
```
ProxyConfig (Extended)
├── InsecureSkipVerify: bool (default: false)
└── CustomCACertificates: []string (paths to CA certs)
```

### 7.4 Access Control

**API Access Control**:
- Proxy configuration endpoints require authentication (future enhancement)
- Admin-only operations: list all proxies, delete proxy
- Token owner operations: view/update own token proxy

**Audit Logging**:
- Log all proxy configuration changes
- Include: timestamp, user, token ID, action
- Store in separate audit log file

---

## 8. Testing Strategy

### 8.1 Unit Testing

**HTTP Client Factory Tests**:
- Test client creation with different proxy types
- Test client caching behavior
- Test cache invalidation
- Test proxy configuration parsing

**Proxy Configuration Tests**:
- Test proxy config validation
- Test JSON marshaling/unmarshaling
- Test backward compatibility (missing proxy fields)
- Test encryption/decryption of credentials

**Token Selection Tests**:
- Test token selection with proxy configuration
- Test client factory integration
- Test proxy health tracking

### 8.2 Integration Testing

**Proxy Server Mocking**:
- Use `golang.org/x/net/proxy` for SOCKS5 test server
- Use `httptest` for HTTP proxy test server
- Simulate proxy failures (timeout, auth error, connection refused)

**Test Scenarios**:
1. Token with no proxy (direct connection)
2. Token with HTTP proxy
3. Token with HTTPS proxy
4. Token with SOCKS5 proxy
5. Token with authenticated proxy
6. Proxy connection failure handling
7. Proxy health tracking
8. Token refresh through proxy
9. Streaming requests through proxy

### 8.3 End-to-End Testing

**Test Flow**:
1. Configure proxy for token via API
2. Test proxy configuration
3. Make provider request using token
4. Verify request goes through proxy
5. Verify proxy health updated
6. Disable proxy for token
7. Verify direct connection used
8. Re-enable proxy and verify cached client used

### 8.4 Performance Testing

**Metrics to Measure**:
- HTTP client creation time
- Proxy connection latency
- Request throughput with proxy vs without
- Memory usage with client caching
- Connection pool efficiency

---

## 9. Implementation Phases

### Phase 1: Foundation (High Priority)

**Objective**: Establish core abstractions and data model

**Tasks**:
1. Create `HTTPClient` interface
2. Create `ProxyConfig` structure
3. Extend `ProviderToken` with proxy fields
4. Create `ProxyAwareHTTPClientFactory`
5. Update JSON serialization for backward compatibility
6. Add proxy configuration validation

**Dependencies**: None
**Risk**: Low
**Estimated Complexity**: Medium

### Phase 2: Token Storage Integration (High Priority)

**Objective**: Integrate proxy configuration into token storage

**Tasks**:
1. Update `MultiTokenStore` to handle proxy fields
2. Add proxy CRUD operations to token store
3. Implement proxy credential encryption/decryption
4. Add proxy health tracking to token metadata
5. Update token file format (backward compatible)

**Dependencies**: Phase 1
**Risk**: Medium
**Estimated Complexity**: Medium

### Phase 3: Token Manager Integration (High Priority)

**Objective**: Enable token selection with proxy-aware clients

**Tasks**:
1. Extend `TokenManager` with client factory
2. Add `SelectTokenWithClient()` method
3. Add proxy health tracking methods
4. Update token selection flow
5. Integrate with existing selection strategies

**Dependencies**: Phase 1, Phase 2
**Risk**: Medium
**Estimated Complexity**: Medium

### Phase 4: Provider Integration (Medium Priority)

**Objective**: Update providers to use proxy-aware clients

**Tasks**:
1. Extend `Provider` interface (optional method)
2. Update `QwenProvider` implementation
3. Update `GeminiProvider` implementation
4. Update `KiroProvider` implementation
5. Update `IFlowProvider` implementation
6. Update `AntigravityProvider` implementation
7. Update token refresh implementations

**Dependencies**: Phase 1, Phase 3
**Risk**: Medium
**Estimated Complexity**: High

### Phase 5: REST API (Medium Priority)

**Objective**: Provide API endpoints for proxy configuration

**Tasks**:
1. Add proxy configuration endpoints
2. Add proxy test endpoint
3. Add proxy listing endpoint
4. Update token info responses with proxy data
5. Add request/response validation
6. Add error handling for proxy operations

**Dependencies**: Phase 2
**Risk**: Low
**Estimated Complexity**: Medium

### Phase 6: Dashboard Updates (Low Priority)

**Objective**: Update UI for proxy configuration

**Tasks**:
1. Add proxy configuration form
2. Add proxy health indicators
3. Add proxy test functionality
4. Update token details view
5. Add proxy list view

**Dependencies**: Phase 5
**Risk**: Low
**Estimated Complexity**: Low

### Phase 7: Advanced Features (Low Priority)

**Objective**: Implement advanced proxy features

**Tasks**:
1. Implement fallback strategies
2. Implement alternate proxy rotation
3. Add proxy health check scheduling
4. Add proxy performance metrics
5. Add audit logging

**Dependencies**: Phase 1-6
**Risk**: Low
**Estimated Complexity**: High

---

## 10. Component Interaction Diagrams

### 10.1 Token Selection with Proxy Flow

```
┌─────────────────┐
│   Client App   │
└───────┬────────┘
        │
        │ GET /api/token/{provider}
        ▼
┌─────────────────────────────────┐
│      REST API Server         │
│  ┌───────────────────────┐  │
│  │  TokenManager         │  │
│  │  - SelectTokenWithClient()│ │
│  └───────────┬───────────┘  │
└──────────────┼────────────────┘
               │
               │ Get token with proxy config
               ▼
┌─────────────────────────────────┐
│     MultiTokenStore         │
│  - GetToken()              │
│  - Returns ProviderToken     │
│    with ProxyConfig          │
└───────────┬─────────────────┘
            │
            │ Get client with proxy config
            ▼
┌─────────────────────────────────┐
│ ProxyAwareHTTPClientFactory  │
│  - GetClient(ProxyConfig)   │
│  - Returns *http.Client      │
└───────────┬─────────────────┘
            │
            │ Returns (Token, Client)
            ▼
┌─────────────────────────────────┐
│      REST API Server         │
│  - Returns token + client   │
└───────────┬─────────────────┘
            │
            │ Response with token info
            ▼
┌─────────────────┐
│   Client App   │
└─────────────────┘
```

### 10.2 Provider Request with Proxy Flow

```
┌─────────────────┐
│   Client App   │
└───────┬────────┘
        │
        │ Provider API request
        ▼
┌─────────────────────────────────┐
│      Provider (e.g., Qwen)  │
│  ┌───────────────────────┐  │
│  │  TokenManager         │  │
│  │  - SelectTokenWithClient()│ │
│  └───────────┬───────────┘  │
└──────────────┼────────────────┘
               │
               │ Get token + client
               ▼
┌─────────────────────────────────┐
│ ProxyAwareHTTPClientFactory  │
│  - GetClient(ProxyConfig)   │
│  - Returns *http.Client      │
│    configured with proxy     │
└───────────┬─────────────────┘
            │
            │ HTTP request
            ▼
┌─────────────────────────────────┐
│      Proxy Server           │
│  - SOCKS5 or HTTP/HTTPS   │
└───────────┬─────────────────┘
            │
            │ Forwarded request
            ▼
┌─────────────────────────────────┐
│   Provider API Server       │
└───────────┬─────────────────┘
            │
            │ Response
            ▼
┌─────────────────┐
│   Client App   │
└─────────────────┘
```

### 10.3 Token Refresh with Proxy Flow

```
┌─────────────────────────────────┐
│   RefreshScheduler           │
│  - Detects expiring token   │
└───────────┬─────────────────┘
            │
            │ Schedule refresh
            ▼
┌─────────────────────────────────┐
│   RefreshCoordinator        │
│  ┌───────────────────────┐  │
│  │  ProviderRefresher     │  │
│  │  - RefreshToken()      │  │
│  └───────────┬───────────┘  │
└──────────────┼────────────────┘
               │
               │ Get token with proxy config
               ▼
┌─────────────────────────────────┐
│     MultiTokenStore         │
│  - GetToken()              │
└───────────┬─────────────────┘
            │
            │ Get client with proxy config
            ▼
┌─────────────────────────────────┐
│ ProxyAwareHTTPClientFactory  │
│  - GetClient(ProxyConfig)   │
└───────────┬─────────────────┘
            │
            │ Refresh request
            ▼
┌─────────────────────────────────┐
│      Proxy Server           │
└───────────┬─────────────────┘
            │
            │ Forwarded request
            ▼
┌─────────────────────────────────┐
│   Provider OAuth Server      │
└───────────┬─────────────────┘
            │
            │ New tokens
            ▼
┌─────────────────────────────────┐
│     MultiTokenStore         │
│  - UpdateToken()            │
│  - Update proxy health      │
└─────────────────────────────┘
```

---

## 11. Migration Strategy for Existing Tokens

### 11.1 Backward Compatibility

**No Proxy Configuration**:
- Existing tokens load without proxy fields
- Default proxy config: `Type: none`, `Enabled: false`
- No changes required to existing token files

**JSON Unmarshaling**:
```go
type ProviderToken struct {
    // Existing fields...
    Proxy *ProxyConfig `json:"proxy,omitempty"`
    ProxyHealthScore float64 `json:"proxy_health_score,omitempty"`
}
```

### 11.2 Migration Path

**Optional Migration**:
- Provide migration tool to add proxy configuration to existing tokens
- Migration triggered via API endpoint or CLI command
- Migration is optional, not automatic

**Migration Endpoint**:
```
POST /api/migrate/proxy
{
  "provider": "qwen",
  "default_proxy": {
    "type": "socks5",
    "host": "proxy.example.com",
    "port": 1080
  }
}
```

### 11.3 Rollback Strategy

**Rollback Scenario**:
- If proxy configuration causes issues, user can disable proxy
- Disable proxy without removing configuration
- Token remains functional with direct connection

**Disable Proxy**:
```
PUT /api/credentials/{provider}/{tokenID}/proxy
{
  "enabled": false
}
```

---

## 12. Risk Assessment and Mitigation

### 12.1 Technical Risks

| Risk | Impact | Probability | Mitigation |
|-------|---------|--------------|-------------|
| Proxy connection failures break token usage | High | Medium | Implement fallback strategies, proxy health tracking |
| Client caching causes stale proxy configs | Medium | Low | Cache invalidation on proxy update, TTL-based cache |
| SOCKS5 implementation bugs | Medium | Low | Use proven library (golang.org/x/net/proxy), extensive testing |
| Performance degradation with proxy | Medium | Medium | Proxy health tracking, latency monitoring, connection pooling |
| Memory leaks from client caching | Medium | Low | LRU cache with size limit, periodic cleanup |

### 12.2 Security Risks

| Risk | Impact | Probability | Mitigation |
|-------|---------|--------------|-------------|
| Proxy credentials exposed in logs | High | Low | Credential masking, structured logging, audit log review |
| Proxy credentials stored in plain text | High | Low | Encryption at rest, key management |
| Man-in-the-middle via malicious proxy | High | Low | Proxy validation, HTTPS enforcement, certificate pinning (future) |
| Unauthorized proxy configuration changes | Medium | Medium | Access control, audit logging, authentication (future) |

### 12.3 Operational Risks

| Risk | Impact | Probability | Mitigation |
|-------|---------|--------------|-------------|
| Complex configuration increases user errors | Medium | High | Clear UI, validation, helpful error messages |
| Proxy health tracking overhead | Low | Medium | Efficient tracking, configurable check intervals |
| Increased code complexity | Medium | High | SOLID principles, clear separation of concerns, comprehensive tests |

---

## 13. Dependencies on SOLID Refactoring Recommendations

### 13.1 Required Dependencies

**HTTP Client Interface (P1)**:
- **Required**: Yes
- **Reason**: Proxy configuration requires HTTP client abstraction
- **Implementation Priority**: Phase 1

**Token Store Interface (P1)**:
- **Required**: Yes
- **Reason**: Proxy configuration stored with token metadata
- **Implementation Priority**: Phase 1

### 13.2 Optional Dependencies

**Authenticator Factory (P2)**:
- **Required**: No
- **Reason**: Can implement proxy support without factory
- **Benefit**: Cleaner separation of concerns

**MultiTokenManager Decomposition (P2)**:
- **Required**: No
- **Reason**: Can integrate with existing manager
- **Benefit**: Easier testing, better separation

### 13.3 Implementation Order

**Recommended Order**:
1. Implement HTTP Client Interface (SOLID P1)
2. Implement Token Store Interface (SOLID P1)
3. Implement Per-Token Proxy Support (this design)
4. Implement Authenticator Factory (SOLID P2)
5. Implement MultiTokenManager Decomposition (SOLID P2)

---

## 14. Performance Considerations

### 14.1 Client Caching

**Cache Size**:
- Default: 50 clients
- Configurable via `ProxyCacheMaxSize`
- LRU eviction when full

**Cache Key**:
- Based on proxy type, host, port, auth presence
- Excludes actual credentials (security)
- Enables sharing across tokens with same proxy

### 14.2 Connection Pooling

**Go's http.Transport**:
- Automatically manages connection pooling
- Each proxy configuration gets own transport
- Idle connections respect `IdleConnTimeout`

**Configuration**:
```go
transport := &http.Transport{
    Proxy: http.ProxyURL(proxyURL),
    MaxIdleConns: 50,
    MaxIdleConnsPerHost: 50,
    IdleConnTimeout: 180 * time.Second,
}
```

### 14.3 Latency Overhead

**Proxy Latency**:
- SOCKS5: ~5-10ms additional handshake
- HTTP/HTTPS: ~2-5ms additional hop
- Measured and tracked in proxy health

**Mitigation**:
- Proxy health tracking to identify slow proxies
- Latency monitoring
- Fallback to direct connection if proxy too slow

---

## 15. Monitoring and Observability

### 15.1 Metrics to Track

**Proxy Metrics**:
- Proxy connection success rate
- Proxy connection failure rate
- Average proxy latency
- Proxy health score distribution
- Active proxy configurations count

**Token Metrics**:
- Tokens with proxy vs without proxy
- Token success rate by proxy type
- Token refresh success rate by proxy

### 15.2 Logging Strategy

**Proxy Operation Logs**:
```
[Proxy] Created client for token {tokenID} with proxy {type}://{host}:{port}
[Proxy] Using cached client for proxy {type}://{host}:{port}
[Proxy] Proxy connection failed for token {tokenID}: {error}
[Proxy] Proxy health updated for token {tokenID}: healthy={bool}, score={score}
```

**Security Logs** (separate file):
```
[Audit] Proxy configured for token {tokenID} by user {userID} at {timestamp}
[Audit] Proxy removed from token {tokenID} by user {userID} at {timestamp}
[Audit] Proxy tested for token {tokenID} by user {userID} at {timestamp}
```

### 15.3 Health Check Endpoints

**New Health Endpoints**:
```
GET /health/proxies
       Returns overall proxy health status

GET /health/proxies/{tokenID}
       Returns proxy health for specific token

GET /health/proxies/stats
       Returns proxy statistics
```

---

## 16. Future Enhancements

### 16.1 Proxy Rotation

**Feature**: Automatic rotation through multiple proxies per token

**Configuration**:
```
ProviderToken (Future)
├── Proxies: []ProxyConfig
├── CurrentProxyIndex: int
└── RotationStrategy: RotationType (enum: round_robin, least_latency, health_based)
```

### 16.2 Geographic Proxy Selection

**Feature**: Select proxy based on target API region

**Configuration**:
```
ProxyConfig (Future)
├── Region: string (e.g., "us-east-1")
└── PreferredRegions: []string
```

### 16.3 Smart Proxy Selection

**Feature**: Automatically select best proxy based on health and latency

**Algorithm**:
- Consider proxy health score
- Consider proxy latency
- Consider recent success rate
- Weighted scoring to select optimal proxy

### 16.4 Proxy Authentication Methods

**Feature**: Support additional SOCKS5 authentication methods

**Methods**:
- Username/password (current)
- GSSAPI/Kerberos (future)
- Client certificate (future)

---

## 17. Conclusion

This architectural design provides a comprehensive approach to implementing per-OAuth/Device token proxy support in the qwencoder-proxy project. The design:

1. **Extends the existing data model** with proxy configuration while maintaining backward compatibility
2. **Introduces HTTP client abstractions** following SOLID principles recommendations
3. **Integrates with token selection** to provide proxy-aware clients to providers
4. **Defines clear API endpoints** for proxy configuration management
5. **Addresses security concerns** with credential encryption and access control
6. **Provides a testing strategy** for validating proxy functionality
7. **Outlines implementation phases** with clear priorities and dependencies

The design leverages the existing multi-token storage system, token selection strategies, and provider abstractions, minimizing disruption while adding significant new functionality.

**Key Benefits**:
- Each token can use a different proxy
- Proxy configuration stored alongside token metadata
- HTTP client caching optimizes resource usage
- Proxy health tracking enables proactive management
- Backward compatible with existing tokens

**Next Steps**:
1. Review and approve this architectural design
2. Begin Phase 1 implementation (Foundation)
3. Implement HTTP Client Interface (SOLID P1 prerequisite)
4. Implement Token Store Interface (SOLID P1 prerequisite)
5. Progress through remaining phases

---

**End of Architectural Design**
