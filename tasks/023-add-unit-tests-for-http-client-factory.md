# Task 023: Add Unit Tests for HTTP Client Factory

## Description
Create comprehensive unit tests for ProxyAwareHTTPClientFactory. These tests ensure that HTTP clients are correctly created and cached for different proxy configurations.

## Technical Context
This task creates unit tests for the proxy-aware HTTP client factory created in Task 003.

**Test Coverage Requirements**:
- Client creation for each proxy type (HTTP, HTTPS, SOCKS5, none)
- Client caching behavior (cache hits, cache misses)
- Cache invalidation
- LRU eviction when cache is full
- Proxy configuration parsing
- Connection pooling configuration
- Error handling for invalid proxy configurations

**Related Components**:
- [`config/proxy_client_factory.go`](config/proxy_client_factory.go) - Factory to test
- [`auth/proxy_config.go`](auth/proxy_config.go) - ProxyConfig structures used

**Test Categories**:
1. **Client Creation Tests**:
   - Create client with HTTP proxy
   - Create client with HTTPS proxy
   - Create client with SOCKS5 proxy
   - Create client with no proxy (direct connection)
   - Handle authenticated proxies
   - Handle unauthenticated proxies
2. **Client Caching Tests**:
   - Cache miss creates new client
   - Cache hit returns existing client
   - Same proxy config returns same client
   - Different proxy config returns different client
   - Credentials excluded from cache key
3. **Cache Invalidation Tests**:
   - ClearCache() removes all clients
   - Cache empty after clearing
   - New clients created after clearing
4. **LRU Eviction Tests**:
   - Least recently used client evicted first
   - Cache size limit respected
   - All clients evicted when cache full
   - New client added after eviction
5. **Configuration Tests**:
   - Timeout configuration applied
   - Connection pooling configured
   - Idle connection timeout set
   - Max idle connections set
6. **Error Handling Tests**:
   - Invalid proxy type returns error
   - Invalid host returns error
   - Invalid port returns error
   - Invalid SOCKS5 configuration returns error
7. **Logging Tests**:
   - Client creation logged
   - Cache hit logged
   - Cache miss logged
   - Cache eviction logged

**Mock Requirements**:
- Mock HTTP servers for testing client connections
- Mock proxy servers for SOCKS5 testing
- Mock logger for log verification
- Mock time for timeout testing

**Architectural Principles**:
- **Test-Driven Development**: Tests validate implementation correctness
- **Edge Case Coverage**: Tests cover boundary conditions and error cases
- **Isolation**: Tests don't require real proxy servers

## Subtasks
1. Create `config/proxy_client_factory_test.go` file
2. Set up test fixtures:
   - Mock logger
   - Test proxy configurations
   - Test HTTP client configurations
3. Implement client creation tests:
   - TestCreateHTTPProxyClient()
   - TestCreateHTTPSProxyClient()
   - TestCreateSOCKS5ProxyClient()
   - TestCreateNoProxyClient()
   - TestAuthenticatedProxyClient()
   - TestUnauthenticatedProxyClient()
4. Implement caching tests:
   - TestCacheMissCreatesNewClient()
   - TestCacheHitReturnsExistingClient()
   - TestSameProxyConfigSameClient()
   - TestDifferentProxyConfigDifferentClient()
   - TestCredentialsExcludedFromKey()
5. Implement cache invalidation tests:
   - TestClearCacheRemovesAllClients()
   - TestCacheEmptyAfterClearing()
   - TestNewClientAfterClearing()
6. Implement LRU eviction tests:
   - TestLRUEvictionOrder()
   - TestCacheSizeLimitRespected()
   - TestAllClientsEvictedWhenFull()
   - TestNewClientAfterEviction()
7. Implement configuration tests:
   - TestTimeoutConfigurationApplied()
   - TestConnectionPoolingConfigured()
   - TestIdleConnectionTimeoutSet()
   - TestMaxIdleConnectionsSet()
8. Implement error handling tests:
   - TestInvalidProxyTypeError()
   - TestInvalidHostError()
   - TestInvalidPortError()
   - TestInvalidSOCKS5ConfigError()
9. Implement logging tests:
   - TestClientCreationLogged()
   - TestCacheHitLogged()
   - TestCacheMissLogged()
   - TestCacheEvictionLogged()
10. Add table-driven tests for edge cases:
    - Boundary values for ports
    - Empty strings for optional fields
    - Special characters in host
    - Unicode characters in credentials
    - Large cache sizes
    - Concurrent access to cache

## Dependencies
- Task 003: Create Proxy-Aware HTTP Client Factory (factory to test)

## Files to Create
- `config/proxy_client_factory_test.go` (new test file)

## Related Tasks
- Task 022: Add Unit Tests for Proxy Configuration (related testing task)
- Task 024: Add Integration Tests for Proxy-Aware Requests (integration testing)
