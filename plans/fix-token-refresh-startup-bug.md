# Fix Token Refresh Startup Bug

## Overview

This plan addresses the critical bug where token refresh schedulers are never started, preventing automatic token refresh from working. The issue is a timing problem in the initialization sequence of the REST API server.

## Problem Statement

### Current State

In [`restapi/rest_api.go:96-131`](../restapi/rest_api.go:96), the initialization sequence is:

1. Create MultiTokenManager
2. Initialize MultiTokenManager
3. **Start MultiTokenManager** ← Called too early!
4. Register Provider Refreshers ← Creates schedulers but they're never started

### Impact

- ❌ Auto-refresh does NOT run in the background
- ❌ Tokens are not refreshed automatically before expiry
- ❌ Users must manually refresh tokens via the UI
- ❌ Tokens expire unexpectedly

### Root Cause

The [`MultiTokenManager.Start()`](../internal/token/multi_token_manager.go:114) method iterates over `mtm.schedulers` to start them. However, schedulers are only added to this map during [`RegisterRefresher()`](../internal/token/multi_token_manager.go:327), which is called AFTER `Start()`.

## Solution

### Primary Fix: Reorder Initialization Sequence

**File:** [`restapi/rest_api.go`](../restapi/rest_api.go)

**Location:** Lines 96-131

**Change:** Swap the order of `multiTokenManager.Start()` and `server.registerProviderRefreshers()`

#### Before (Broken):

```go
// Create multi-token manager
multiTokenManager := tokpkg.NewMultiTokenManager(logger)
if err := multiTokenManager.Initialize(); err != nil {
    logger.ErrorLog("Failed to initialize multi-token manager: %v", err)
}
if err := multiTokenManager.Start(); err != nil {
    logger.ErrorLog("Failed to start multi-token manager: %v", err)
}

server := &Server{
    // ... other fields
    multiTokenManager: multiTokenManager,
}

// Register provider refreshers
if err := server.registerProviderRefreshers(); err != nil {
    logger.ErrorLog("Failed to register provider refreshers: %v", err)
}
```

#### After (Fixed):

```go
// Create multi-token manager
multiTokenManager := tokpkg.NewMultiTokenManager(logger)
if err := multiTokenManager.Initialize(); err != nil {
    logger.ErrorLog("Failed to initialize multi-token manager: %v", err)
}

server := &Server{
    // ... other fields
    multiTokenManager: multiTokenManager,
}

// Register provider refreshers FIRST (this creates schedulers)
if err := server.registerProviderRefreshers(); err != nil {
    logger.ErrorLog("Failed to register provider refreshers: %v", err)
}

// THEN start multi-token manager (now schedulers exist)
if err := multiTokenManager.Start(); err != nil {
    logger.ErrorLog("Failed to start multi-token manager: %v", err)
}
```

### Enhancement: Add Startup Token Refresh

**File:** [`internal/token/multi_token_manager.go`](../internal/token/multi_token_manager.go)

**Location:** After the `Start()` method

**Purpose:** Explicitly refresh all tokens on startup to ensure they're fresh

#### Implementation:

```go
// RefreshAllTokens checks all tokens and schedules refreshes for expiring ones
func (mtm *MultiTokenManager) RefreshAllTokens() error {
    mtm.mu.RLock()
    defer mtm.mu.RUnlock()

    mtm.logger.InfoLog("[MultiTokenManager] Checking all tokens for refresh...")

    // Check tokens for all registered providers
    for providerID, scheduler := range mtm.schedulers {
        mtm.logger.InfoLog("[MultiTokenManager] Checking tokens for provider: %s", providerID)
        scheduler.CheckAndSchedule()
    }

    mtm.logger.InfoLog("[MultiTokenManager] Token refresh check complete")
    return nil
}
```

#### Usage in Server:

**File:** [`restapi/rest_api.go`](../restapi/rest_api.go)

**Location:** After `multiTokenManager.Start()`

```go
// Start multi-token manager
if err := multiTokenManager.Start(); err != nil {
    logger.ErrorLog("Failed to start multi-token manager: %v", err)
}

// Refresh all tokens on startup
if err := multiTokenManager.RefreshAllTokens(); err != nil {
    logger.ErrorLog("Failed to refresh tokens on startup: %v", err)
}
```

### Enhancement: Add Token Health Check on Load

**File:** [`internal/token/multi_token_store.go`](../internal/token/multi_token_store.go)

**Location:** In the `Load()` method

**Purpose:** Mark expired tokens as unhealthy when loaded from disk

#### Implementation:

Add this helper function:

```go
// isTokenExpired checks if a token is expired
func isTokenExpired(token ProviderToken) bool {
    if token.ExpiryDate == 0 {
        return false
    }
    expiry := time.UnixMilli(token.ExpiryDate)
    return time.Until(expiry) <= 0
}
```

Update the `Load()` method to check token health:

```go
func (mts *MultiTokenStore) Load() error {
    mts.mu.Lock()
    defer mts.mu.Unlock()

    // ... existing load code ...

    // Check token health after loading
    expiredCount := 0
    for i := range mts.Tokens {
        token := &mts.Tokens[i]
        if isTokenExpired(*token) {
            token.Healthy = false
            token.LastError = "Token expired"
            expiredCount++
            mts.logger.WarnLog("[MultiTokenStore] Token %s is expired", token.ID)
        }
    }

    if expiredCount > 0 {
        mts.logger.WarnLog("[MultiTokenStore] Loaded %d expired tokens", expiredCount)
    }

    return nil
}
```

## Implementation Steps

### Step 1: Fix Initialization Order (Critical)

1. Open [`restapi/rest_api.go`](../restapi/rest_api.go)
2. Locate the `NewServer` function (around line 96)
3. Move `server.registerProviderRefreshers()` call BEFORE `multiTokenManager.Start()`
4. Ensure the server struct is created before calling `registerProviderRefreshers()`
5. Test that schedulers are started by checking logs for "Started scheduler for provider"

### Step 2: Add Startup Token Refresh (High Priority)

1. Open [`internal/token/multi_token_manager.go`](../internal/token/multi_token_manager.go)
2. Add the `RefreshAllTokens()` method after `Start()`
3. Open [`restapi/rest_api.go`](../restapi/rest_api.go)
4. Call `multiTokenManager.RefreshAllTokens()` after `multiTokenManager.Start()`
5. Test that tokens are refreshed on startup

### Step 3: Add Token Health Check (Medium Priority)

1. Open [`internal/token/multi_token_store.go`](../internal/token/multi_token_store.go)
2. Add the `isTokenExpired()` helper function
3. Update the `Load()` method to check token health
4. Test that expired tokens are marked as unhealthy

### Step 4: Verification and Testing

1. Start the server and check logs for:
   - "Registering refresher for provider: <provider>"
   - "Started scheduler for provider: <provider>"
   - "Checking all tokens for refresh..."
2. Create a test token that expires in 25 minutes
3. Wait 5 minutes and verify it was refreshed
4. Check the token store file to confirm the expiry date was updated
5. Test with multiple providers

## Testing Checklist

- [ ] Server starts without errors
- [ ] Logs show "Started scheduler for provider" for each provider
- [ ] Logs show "Checking all tokens for refresh..." on startup
- [ ] Tokens expiring within 30 minutes are refreshed automatically
- [ ] Token expiry dates are updated in the JSON files
- [ ] Manual refresh still works via the UI
- [ ] Expired tokens are marked as unhealthy on load
- [ ] No duplicate schedulers are created
- [ ] Scheduler continues running after startup
- [ ] Tokens are refreshed every 5 minutes as expected

## Rollback Plan

If issues arise after implementing the fix:

1. Revert the initialization order change in [`restapi/rest_api.go`](../restapi/rest_api.go)
2. Remove the `RefreshAllTokens()` call
3. Remove the token health check changes
4. Verify the server starts without the fix

## Expected Behavior After Fix

### On Server Startup

1. MultiTokenManager is initialized
2. Provider refreshers are registered (creates schedulers)
3. Schedulers are started (begin running in background)
4. All tokens are checked immediately
5. Tokens expiring within 30 minutes are refreshed

### During Runtime

1. Schedulers run continuously in background
2. Every 5 minutes, all tokens are checked
3. Tokens expiring within 30 minutes are refreshed
4. Refreshed tokens are saved to disk
5. Token health is tracked and updated

### User Experience

- Tokens refresh automatically before expiry
- No manual intervention required
- Dashboard shows current token status
- Expired tokens are clearly marked

## Related Files

- [`restapi/rest_api.go`](../restapi/rest_api.go) - Server initialization
- [`internal/token/multi_token_manager.go`](../internal/token/multi_token_manager.go) - Multi-token manager
- [`internal/token/multi_token_store.go`](../internal/token/multi_token_store.go) - Token storage
- [`internal/token/token_refresh.go`](../internal/token/token_refresh.go) - Refresh coordinator and scheduler
- [`provider/gemini/auth.go`](../provider/gemini/auth.go) - Gemini token refresher
- [`provider/qwen/auth_device.go`](../provider/qwen/auth_device.go) - Qwen token refresher
- [`provider/iflow/auth.go`](../provider/iflow/auth.go) - iFlow token refresher
- [`provider/antigravity/auth.go`](../provider/antigravity/auth.go) - Antigravity token refresher

## References

- Original analysis: [`token-refresh-implementation-analysis.md`](../token-refresh-implementation-analysis.md)
- Implementation plan: [`plans/token-refresh-implementation.md`](token-refresh-implementation.md)

---

**Plan Version:** 1.0  
**Created:** 2025-02-18  
**Priority:** Critical  
**Estimated Effort:** 1-2 hours
