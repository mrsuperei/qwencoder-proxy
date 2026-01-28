# Task 025: Add End-to-End Tests for Proxy Configuration Flow

## Description
Create end-to-end tests that verify the complete proxy configuration flow from REST API through to provider requests. These tests ensure the entire proxy-per-token feature works correctly.

## Technical Context
This task creates end-to-end tests for the complete proxy configuration workflow.

**Test Coverage Requirements**:
- Configure proxy for token via REST API
- Test proxy configuration via REST API
- Delete proxy configuration via REST API
- Make provider request using configured proxy
- Verify proxy health tracking
- Verify client caching behavior
- Verify error handling for proxy failures
- All provider types tested

**End-to-End Test Flow**:
```
1. Start REST API server with all components
2. Create token with no proxy configuration
3. Configure proxy for token via PUT endpoint
4. Verify proxy configuration via GET endpoint
5. Make provider request (should use proxy)
6. Verify proxy health updated
7. Update proxy configuration
8. Verify new configuration used
9. Delete proxy configuration
10. Verify direct connection used
```

**Test Scenarios**:
1. **Configuration Flow Tests**:
   - Configure HTTP proxy for token
   - Configure HTTPS proxy for token
   - Configure SOCKS5 proxy for token
   - Configure authenticated proxy for token
   - Configure and disable proxy
   - Configure, delete, and reconfigure proxy
2. **Request Flow Tests**:
   - Request with configured proxy succeeds
   - Request with no proxy succeeds
   - Request after proxy change uses new proxy
   - Request after proxy delete uses direct connection
3. **Error Flow Tests**:
   - Invalid proxy configuration rejected
   - Proxy connection failure handled
   - Proxy timeout handled
   - Proxy health updated on failure
4. **Health Tracking Tests**:
   - Health score increases on success
   - Health score decreases on failure
   - Health status reflects proxy state
5. **Caching Tests**:
   - Same proxy config shares client
   - Different proxy config creates new client
   - Cache cleared on proxy change

**Related Components**:
- [`restapi/rest_api.go`](restapi/rest_api.go) - REST API endpoints
- [`auth/token_selection.go`](auth/token_selection.go) - TokenManager
- [`provider/gemini/gemini.go`](provider/gemini/gemini.go) - All providers
- [`config/proxy_client_factory.go`](config/proxy_client_factory.go) - Client factory
- [`auth/proxy_health_tracker.go`](auth/proxy_health_tracker.go) - Health tracker

**Test Infrastructure**:
- Real HTTP server for REST API
- Mock provider API endpoints
- Real proxy servers (or high-quality mocks)
- Test token storage (in-memory)
- Test logger for verification

**Architectural Principles**:
- **End-to-End Testing**: Tests verify complete user workflows
- **User Journey**: Tests follow actual user paths
- **System Integration**: All components tested together

## Subtasks
1. Create `tests/proxy_e2e_test.go` file
2. Set up test infrastructure:
   - Start REST API server
   - Create in-memory token store
   - Create proxy client factory
   - Create proxy health tracker
   - Create mock provider API servers
   - Create test proxy servers
3. Implement configuration flow tests:
   - TestConfigureHTTPProxy()
   - TestConfigureHTTPSProxy()
   - TestConfigureSOCKS5Proxy()
   - TestConfigureAuthenticatedProxy()
   - TestConfigureAndDisableProxy()
   - TestConfigureDeleteReconfigure()
4. Implement request flow tests:
   - TestRequestWithConfiguredProxy()
   - TestRequestWithNoProxy()
   - TestRequestAfterProxyChange()
   - TestRequestAfterProxyDelete()
5. Implement error flow tests:
   - TestInvalidProxyConfigurationRejected()
   - TestProxyConnectionFailureHandled()
   - TestProxyTimeoutHandled()
   - TestProxyHealthUpdatedOnFailure()
6. Implement health tracking tests:
   - TestHealthScoreIncreasesOnSuccess()
   - TestHealthScoreDecreasesOnFailure()
   - TestHealthStatusReflectsProxyState()
7. Implement caching tests:
   - TestSameProxyConfigSharesClient()
   - TestDifferentProxyConfigCreatesNewClient()
   - TestCacheClearedOnProxyChange()
8. Implement provider-specific tests:
   - TestGeminiProxyFlow()
   - TestIFlowProxyFlow()
   - TestKiroProxyFlow()
   - TestQwenProxyFlow()
   - TestAntigravityProxyFlow()
9. Add comprehensive test scenarios:
   - Multiple tokens with different proxies
   - Concurrent proxy configuration
   - Proxy configuration during active request
   - Streaming requests with proxy
   - Token refresh with proxy
10. Add test cleanup and reporting:
    - Cleanup test resources
    - Generate test coverage report
    - Log test results

## Dependencies
- Task 019: Add Proxy Configuration GET Endpoint (GET endpoint)
- Task 020: Add Proxy Configuration PUT Endpoint (PUT endpoint)
- Task 021: Add Proxy Configuration DELETE Endpoint (DELETE endpoint)
- Task 024: Add Integration Tests for Proxy-Aware Requests (integration testing)

## Files to Create
- `tests/proxy_e2e_test.go` (new test file)

## Related Tasks
- Task 022: Add Unit Tests for Proxy Configuration (unit testing)
- Task 023: Add Unit Tests for HTTP Client Factory (unit testing)
- Task 024: Add Integration Tests for Proxy-Aware Requests (integration testing)
