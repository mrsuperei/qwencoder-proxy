# Task 003: Create Proxy-Aware HTTP Client Factory

## Description
Create a factory that produces HTTP clients configured with proxy settings. This factory implements client caching to optimize resource usage and supports HTTP, HTTPS, and SOCKS5 proxy protocols.

## Technical Context
This task is central to the proxy-per-token architecture. The factory pattern enables:

- **Factory Pattern**: Centralized creation of proxy-configured HTTP clients
- **Caching Strategy**: Tokens with identical proxy configurations share HTTP clients
- **Connection Pooling**: Leverages Go's http.Transport for efficient connections

**Design from Proxy-Per-Token Architecture**:
- Factory maintains `proxyCache: map[ProxyConfigKey]*http.Client`
- LRU eviction when cache is full (default: 50 clients)
- Cache key excludes credentials (security consideration)
- Separate transports for different proxy configurations

**Proxy Protocol Support**:
- **HTTP/HTTPS**: Using `http.ProxyURL` from standard library
- **SOCKS5**: Using `golang.org/x/net/proxy` library
- **None/Direct**: Standard connection without proxy

**Related Components**:
- [`config/http_client.go`](config/http_client.go) - Uses HTTPClient interface
- [`auth/proxy_config.go`](auth/proxy_config.go) - Uses ProxyConfig and ProxyConfigKey
- [`config/config.go`](config/config.go) - Will integrate factory into Config

**Architectural Principles**:
- **Single Responsibility**: Factory only responsible for client creation and caching
- **Open/Closed**: Easy to add new proxy types without modifying factory
- **Dependency Inversion**: Returns HTTPClient interface, not concrete type

## Subtasks
1. Create `config/proxy_client_factory.go` file
2. Define ProxyAwareHTTPClientFactory struct with fields:
   - baseConfig: HTTPClientConfig
   - proxyCache: map[ProxyConfigKey]*http.Client
   - cacheLock: sync.RWMutex
   - logger: *logging.Logger
   - maxSize: int (for LRU eviction)
3. Implement GetClient(proxyConfig *ProxyConfig) *http.Client method:
   - Check cache for existing client
   - If cached, return existing client
   - If not cached, create new client via CreateClientWithProxy()
   - Add to cache
   - Apply LRU eviction if cache full
4. Implement CreateClientWithProxy(proxyConfig *ProxyConfig) *http.Client method:
   - Handle ProxyType "none": create standard client
   - Handle ProxyType "http"/"https": use http.ProxyURL()
   - Handle ProxyType "socks5": use golang.org/x/net/proxy
   - Configure http.Transport with proxy settings
   - Set timeout from baseConfig
   - Configure connection pooling (MaxIdleConns, IdleConnTimeout)
5. Implement ClearCache() method to empty client cache
6. Implement LRU eviction logic:
   - Track access order for each cached client
   - Remove least recently used when cache full
7. Add logging for:
   - Client creation events
   - Cache hits/misses
   - Cache eviction
8. Add unit tests for:
   - Client creation with each proxy type
   - Client caching behavior
   - Cache invalidation
   - LRU eviction
   - SOCKS5 proxy configuration
   - HTTP/HTTPS proxy configuration
   - No-proxy (direct connection) configuration

## Dependencies
- Task 001: Create Proxy Configuration Data Structures (uses ProxyConfig, ProxyConfigKey)
- Task 002: Create HTTP Client Interface (uses HTTPClientConfig)

## Files to Create
- `config/proxy_client_factory.go` (new file)
- `config/proxy_client_factory_test.go` (new test file)

## External Dependencies
- `golang.org/x/net/proxy` (for SOCKS5 support)

## Related Tasks
- Task 005: Extend Config with Proxy Client Factory (integrates this factory)
- Task 006: Add SelectTokenWithClient method to TokenManager (uses this factory)
