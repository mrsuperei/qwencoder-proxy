# Task 005: Add Proxy Health Tracking to Token Metadata

## Description
Create ProxyHealthTracker component to monitor proxy connection health separately from token health. This enables proactive proxy management and fallback strategies.

## Technical Context
This task implements proxy health tracking as defined in proxy-per-token architecture:

- **Separate Health Tracking**: Proxy health is independent of token health
- **Health Score Calculation**: Enables proxy selection based on reliability
- **Failure Tracking**: Identifies failing proxies for token rotation

**ProxyHealth Structure** (from Task 001):
```go
type ProxyHealth struct {
    LastCheck           int64   // Timestamp of last health check
    IsHealthy           bool     // Current health status
    LastError           string   // Optional error message
    ConsecutiveFailures  int      // Count of consecutive failures
    AverageLatencyMs    int      // Average latency in milliseconds
}
```

**Health Score Algorithm**:
- Initial score: 1.0
- On success: `score = min(1.0, score + 0.1)`
- On failure: `score = max(0.0, score - 0.2)`
- Consecutive failures > 5: mark proxy as unhealthy

**Health Check Triggers**:
- On token refresh
- On API request failure
- Periodic background check (configurable, default: 5 minutes)
- Manual health check via API

**Related Components**:
- [`auth/proxy_config.go`](auth/proxy_config.go) - Uses ProxyHealth structure
- [`auth/multi_token_store.go`](auth/multi_token_store.go) - Stores ProxyHealthScore in ProviderToken
- [`config/proxy_client_factory.go`](config/proxy_client_factory.go) - May use health for client selection

**Architectural Principles**:
- **Single Responsibility**: Tracker only handles proxy health monitoring
- **Observer Pattern**: Health updates can trigger callbacks
- **Strategy Pattern**: Different health calculation strategies

## Subtasks
1. Create `auth/proxy_health_tracker.go` file
2. Define ProxyHealthTracker struct with fields:
   - healthStore: map[string]*ProxyHealth (key: tokenID)
   - storeLock: sync.RWMutex
   - logger: *logging.Logger
   - failureThreshold: int (default: 5)
   - healthCheckInterval: time.Duration (default: 5 minutes)
3. Implement UpdateHealth(tokenID string, healthy bool, err error) method:
   - Get or create ProxyHealth for tokenID
   - Update LastCheck timestamp
   - Update IsHealthy status
   - Update LastError if provided
   - Increment/decrement ConsecutiveFailures
   - Mark unhealthy if consecutive failures > threshold
   - Calculate new health score
   - Return updated health score
4. Implement GetHealthScore(tokenID string) float64 method:
   - Return stored health score or default 1.0
5. Implement GetHealthStatus(tokenID string) *ProxyHealth method:
   - Return full health status for token
6. Implement RecordLatency(tokenID string, latencyMs int) method:
   - Update AverageLatencyMs using running average
7. Implement StartPeriodicHealthChecks() method (optional):
   - Schedule periodic health checks
   - Stop on context cancellation
8. Implement StopPeriodicHealthChecks() method (optional)
9. Add logging for health updates:
   - Log health score changes
   - Log consecutive failure threshold breaches
   - Log latency measurements
10. Add unit tests for:
    - Health score calculation
    - Consecutive failure tracking
    - Latency tracking
    - Health status retrieval
    - Default values

## Dependencies
- Task 001: Create Proxy Configuration Data Structures (uses ProxyHealth)

## Files to Create
- `auth/proxy_health_tracker.go` (new file)
- `auth/proxy_health_tracker_test.go` (new test file)

## Related Tasks
- Task 006: Add SelectTokenWithClient method to TokenManager (integrates tracker)
- Task 007: Add GetHTTPClient method to Authenticator interface (may use health info)
