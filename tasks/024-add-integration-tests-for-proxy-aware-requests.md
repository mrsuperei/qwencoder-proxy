# Task 024: Add Integration Tests for Proxy-Aware Requests

## Description
Create integration tests that verify end-to-end proxy-aware request flow through the provider layer. These tests ensure that proxy configuration is correctly applied to API requests.

## Technical Context
This task creates integration tests for the complete proxy-aware request flow from token selection through provider API calls.

**Test Coverage Requirements**:
- Token selection with proxy-aware client
- Provider requests using proxy-aware clients
- Proxy configuration applied to HTTP requests
- Proxy health tracking on success/failure
- Error handling for proxy failures
- All provider types (Qwen, Gemini, IFlow, Kiro, Antigravity)
- Both streaming and non-streaming requests

**Test Scenarios**:
1. **Proxy Configuration Scenarios**:
   - Token with HTTP proxy
   - Token with HTTPS proxy
   - Token with SOCKS5 proxy
   - Token with no proxy (direct connection)
   - Token with disabled proxy (enabled=false)
   - Token with authenticated proxy
2. **Provider Request Scenarios**:
   - Non-streaming request with proxy
   - Streaming request with proxy
   - Token refresh with proxy
   - Multiple requests with same proxy (client caching)
   - Request after proxy configuration change
3. **Error Scenarios**:
   - Proxy connection refused
   - Proxy timeout
   - Proxy DNS resolution failure
   - Proxy authentication failure
   - Proxy during stream
4. **Health Tracking Scenarios**:
   - Health score increase on success
   - Health score decrease on failure
   - Consecutive failure tracking
   - Health status retrieval

**Related Components**:
- [`auth/token_selection.go`](auth/token_selection.go) - TokenManager to test
- [`provider/gemini/gemini.go`](provider/gemini/gemini.go) - GeminiProvider to test
- [`provider/iflow/iflow.go`](provider/iflow/iflow.go) - IFlowProvider to test
- [`provider/kiro/kiro.go`](provider/kiro/kiro.go) - KiroProvider to test
- [`provider/qwen/qwen.go`](provider/qwen/qwen.go) - QwenProvider to test
- [`provider/antigravity/antigravity.go`](provider/antigravity/antigravity.go) - AntigravityProvider to test
- [`config/proxy_client_factory.go`](config/proxy_client_factory.go) - ProxyAwareHTTPClientFactory to test

**Mock Requirements**:
- Mock HTTP servers for provider API endpoints
- Mock proxy servers (HTTP, HTTPS, SOCKS5)
- Mock token store with proxy configurations
- Mock logger for log verification
- Mock proxy health tracker

**Architectural Principles**:
- **Integration Testing**: Tests verify component integration
- **Real-World Scenarios**: Tests mirror actual usage patterns
- **Provider Coverage**: All provider types tested

## Subtasks
1. Create `tests/proxy_integration_test.go` file
2. Set up test fixtures:
   - Mock HTTP servers for each provider
   - Mock proxy servers (HTTP, HTTPS, SOCKS5)
   - Mock token store with test tokens
   - Mock logger
   - Mock proxy health tracker
3. Implement token selection tests:
   - TestSelectTokenWithHTTPProxy()
   - TestSelectTokenWithHTTPSProxy()
   - TestSelectTokenWithSOCKS5Proxy()
   - TestSelectTokenWithNoProxy()
   - TestSelectTokenWithDisabledProxy()
   - TestSelectTokenWithAuthenticatedProxy()
4. Implement provider request tests:
   - TestGeminiNonStreamingWithProxy()
   - TestGeminiStreamingWithProxy()
   - TestIFlowNonStreamingWithProxy()
   - TestIFlowStreamingWithProxy()
   - TestKiroNonStreamingWithProxy()
   - TestKiroStreamingWithProxy()
   - TestQwenNonStreamingWithProxy()
   - TestQwenStreamingWithProxy()
   - TestAntigravityNonStreamingWithProxy()
   - TestAntigravityStreamingWithProxy()
5. Implement client caching tests:
   - TestSameProxyConfigUsesCachedClient()
   - TestDifferentProxyConfigUsesNewClient()
   - TestMultipleRequestsSameClient()
6. Implement error handling tests:
   - TestProxyConnectionRefused()
   - TestProxyTimeout()
   - TestProxyDNSFailure()
   - TestProxyAuthenticationFailure()
   - TestProxyErrorDuringStream()
7. Implement health tracking tests:
   - TestHealthScoreIncreaseOnSuccess()
   - TestHealthScoreDecreaseOnFailure()
   - TestConsecutiveFailureTracking()
   - TestHealthStatusRetrieval()
8. Implement proxy configuration change tests:
   - TestRequestAfterProxyConfigChange()
   - TestNewClientAfterProxyUpdate()
   - TestDirectConnectionAfterProxyRemoval()
9. Add table-driven tests for edge cases:
   - Invalid proxy configurations
   - Concurrent requests with same proxy
   - Large response handling with proxy
   - Timeout handling with proxy
   - Retry behavior with proxy failures

## Dependencies
- Task 006: Add SelectTokenWithClient Method to TokenManager (uses SelectTokenWithClient)
- Task 010: Update GeminiProvider for proxy-aware clients (Gemini uses proxy)
- Task 011: Update IFlowProvider for proxy-aware clients (IFlow uses proxy)
- Task 012: Update KiroProvider for proxy-aware clients (Kiro uses proxy)
- Task 013: Update QwenProvider for proxy-aware clients (Qwen uses proxy)
- Task 014: Update AntigravityProvider for proxy-aware clients (Antigravity uses proxy)

## Files to Create
- `tests/proxy_integration_test.go` (new test file)

## Related Tasks
- Task 022: Add Unit Tests for Proxy Configuration (unit testing)
- Task 023: Add Unit Tests for HTTP Client Factory (unit testing)
- Task 025: Add End-to-End Tests for Proxy Configuration Flow (e2e testing)
