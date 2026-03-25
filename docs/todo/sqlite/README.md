# SQLite Database Lock Fix - Implementation Phases

## Overview

This directory contains individual phase files for fixing the SQLite `SQLITE_BUSY` database lock error that occurs when refreshing tokens from dashboard.

**Error Message**:
```
[ERROR] [handleRefreshTokenByID] Failed to update token: failed to update token: database is locked (5) (SQLITE_BUSY)
```

## Root Cause

The [`updateTokenLocked()`](../../internal/token/sqlite_store.go:841) method lacks retry logic for `SQLITE_BUSY` errors. Additionally, 5 async usage recorder workers are writing to the same SQLite database, causing lock contention.

## Implementation Phases

### Phase 0: Reduce Async Worker Count (PRIMARY FIX)

**File**: [`phase-00-reduce-worker-count.md`](./phase-00-reduce-worker-count.md)

**Priority**: CRITICAL
**Estimated Time**: 1 day

**Description**: Reduce async worker count from 5 to 1 to eliminate concurrent write contention.

**Why This Works**:
- SQLite allows multiple readers OR one writer at a time
- 5 workers = concurrent writes = lock contention = `SQLITE_BUSY` errors
- 1 worker = single writer = no contention = reliable operations
- Still provides async processing (offloads writes from request path)

**Expected Result**: `SQLITE_BUSY` errors should be completely eliminated.

---

### Phase 1: Add Retry Logic to updateTokenLocked (SECONDARY FIX)

**File**: [`phase-01-retry-logic.md`](./phase-01-retry-logic.md)

**Priority**: HIGH
**Estimated Time**: 2 days

**Description**: Add retry logic with exponential backoff to [`updateTokenLocked()`](../../internal/token/sqlite_store.go:841).

**What It Does**:
- Retries on transient `SQLITE_BUSY` errors
- Exponential backoff reduces contention with each retry
- Provides resilience for edge cases

**Expected Result**: Retry logic handles any remaining `SQLITE_BUSY` errors.

---

### Phase 2: Add Retry Logic to All Write Operations

**File**: [`phase-02-all-write-operations.md`](./phase-02-all-write-operations.md)

**Priority**: HIGH
**Estimated Time**: 2 days

**Description**: Apply the same retry logic to all write operations in [`SQLiteStore`](../../internal/token/sqlite_store.go:95).

**Methods Updated**:
- [`AddToken()`](../../internal/token/sqlite_store.go:760) - Insert new tokens
- [`RemoveToken()`](../../internal/token/sqlite_store.go:887) - Delete tokens
- [`UpdateSettings()`](../../internal/token/sqlite_store.go) - Update provider settings

**Expected Result**: Consistent retry behavior across all write operations.

---

### Phase 3: Increase Busy Timeout

**File**: [`phase-03-increase-busy-timeout.md`](./phase-03-increase-busy-timeout.md)

**Priority**: MEDIUM
**Estimated Time**: 1 day

**Description**: Increase SQLite busy timeout from 30 seconds to 60 seconds.

**Why This Helps**:
- Operations have more time to complete under load
- Better resilience to temporary load spikes
- No performance impact (only affects blocked operations)

**Expected Result**: More time for operations to complete before timeout.

---

### Phase 4: Add Configuration Support (OPTIONAL)

**File**: [`phase-04-configuration-support.md`](./phase-04-configuration-support.md)

**Priority**: LOW
**Estimated Time**: 2 days

**Description**: Add configuration support for retry parameters via environment variables.

**Environment Variables**:
- `SQLITE_RETRY_ENABLED` - Enable/disable retry logic
- `SQLITE_MAX_RETRIES` - Maximum retry attempts
- `SQLITE_BASE_DELAY_MS` - Base delay in milliseconds
- `SQLITE_MAX_DELAY_MS` - Maximum delay in milliseconds

**Expected Result**: Retry behavior configurable for different environments.

---

## Implementation Order

### Recommended Sequence

1. **Phase 0** - Reduce worker count (PRIMARY FIX - eliminates root cause)
2. **Phase 1** - Add retry logic to [`updateTokenLocked()`](../../internal/token/sqlite_store.go:841) (SECONDARY FIX - provides resilience)
3. **Phase 2** - Add retry logic to all write operations (consistency)
4. **Phase 3** - Increase busy timeout (additional resilience)
5. **Phase 4** - Add configuration support (optional enhancement)

### Why This Order?

- **Phase 0 first** - Eliminates the root cause (concurrent writes)
- **Phase 1-2 next** - Provides resilience for edge cases
- **Phase 3-4 last** - Optional enhancements

---

## Testing Strategy

### Phase 0 Testing

1. Update [`DefaultAsyncUsageRecorderConfig()`](../../internal/ratelimit/async_usage_recorder.go:37) to use `WorkerCount: 1`
2. Restart server
3. Test token refresh from dashboard
4. Verify no `SQLITE_BUSY` errors

### Phase 1-2 Testing

1. Implement retry logic
2. Write unit tests for retry behavior
3. Test with simulated `SQLITE_BUSY` errors
4. Manual testing with dashboard

### Phase 3-4 Testing

1. Implement configuration changes
2. Test with different environment variables
3. Verify configuration loading

---

## Rollback Plan

If issues occur after any phase:

1. Revert changes to original implementation
2. Restart server
3. Monitor for errors

### Phase-Specific Rollback

- **Phase 0**: Revert `WorkerCount` to 5
- **Phase 1-2**: Revert methods to original implementations
- **Phase 3**: Revert `busy_timeout` to 30000
- **Phase 4**: Revert to hardcoded values

---

## Monitoring

### Metrics to Track

After implementing retry logic, track:

| Metric | Description | Alert Threshold |
|--------|-------------|-----------------|
| `total_attempts` | Total database operations | - |
| `successful_retries` | Retries that succeeded | > 10% of total |
| `failed_after_retry` | Operations that failed after max retries | > 1% of total |
| `busy_errors` | `SQLITE_BUSY` errors encountered | > 5% of total |

### Log Messages

Look for retry messages in logs:

```
[SQLiteStore] updateToken: retry attempt 2/10 after 100ms
[SQLiteStore] Updated token 74b6874e-60d0-409c-b501-aa44975f81b4
```

---

## Related Documentation

- **[Analysis Document](../bugs/sqlite-database-lock-analysis.md)** - Detailed error analysis and flow diagrams
- **[Technical Specification](../bugs/sqlite-database-lock-fix-spec.md)** - Complete technical specification
- **[Executive Summary](../bugs/sqlite-database-lock-summary.md)** - High-level summary and roadmap

---

## Quick Reference

### Files to Modify

| Phase | File | Change |
|--------|------|---------|
| 0 | `internal/ratelimit/async_usage_recorder.go` | Reduce `WorkerCount` from 5 to 1 |
| 1 | `internal/token/sqlite_store_retry.go` | NEW: Retry helper functions |
| 1 | `internal/token/sqlite_store.go` | Update [`updateTokenLocked()`](../../internal/token/sqlite_store.go:841) |
| 2 | `internal/token/sqlite_store.go` | Update [`AddToken()`](../../internal/token/sqlite_store.go:760) |
| 2 | `internal/token/sqlite_store.go` | Update [`RemoveToken()`](../../internal/token/sqlite_store.go:887) |
| 2 | `internal/token/sqlite_store.go` | Update [`UpdateSettings()`](../../internal/token/sqlite_store.go) |
| 3 | `internal/token/sqlite_store.go` | Update [`applyPragmas()`](../../internal/token/sqlite_store.go:124) |
| 4 | `internal/config/config.go` | Add retry config fields |

### Go Patterns Used

- **Higher-order functions** - Accept functions as parameters
- **Atomic operations** - Lock-free metrics updates
- **Exponential backoff** - Reduce contention with each retry
- **Configuration loading** - Load from environment with defaults

---

## Next Steps

1. Review each phase file for detailed implementation instructions
2. Switch to **go-code mode** to begin implementation
3. Follow the recommended implementation order (Phase 0 → 4)
4. Test each phase before proceeding to the next
5. Monitor metrics after deployment
