# Token Refresh Implementation Analysis

## Executive Summary

This document provides a comprehensive analysis of the token refresh implementation in the qwencoder-proxy project. The analysis covers both the backend (Go) and frontend (JavaScript) implementations, including startup behavior and the cronjob mechanism for automatic token refresh.

## Table of Contents

1. [Overview](#overview)
2. [Backend Token Refresh Implementation](#backend-token-refresh-implementation)
3. [Startup Token Refresh Behavior](#startup-token-refresh-behavior)
4. [Cronjob for Auto-Refresh](#cronjob-for-auto-refresh)
5. [Frontend Token Refresh](#frontend-token-refresh)
6. [Key Findings and Issues](#key-findings-and-issues)
7. [Recommendations](#recommendations)

---

## Overview

The token refresh system in qwencoder-proxy is built around three main components:

1. **RefreshCoordinator** ([`internal/token/token_refresh.go:38-94`](internal/token/token_refresh.go:38)) - Manages token refresh operations with a worker pool
2. **RefreshScheduler** ([`internal/token/token_refresh.go:186-299`](internal/token/token_refresh.go:186)) - Periodically checks for expiring tokens and schedules refreshes
3. **MultiTokenManager** ([`internal/token/multi_token_manager.go:27-44`](internal/token/multi_token_manager.go:27)) - Orchestrates all token management components

---

## Backend Token Refresh Implementation

### Core Components

#### 1. ProviderRefresh Interface

**Location:** [`internal/token/token_refresh.go:30-36`](internal/token/token_refresh.go:30)

```go
type ProviderRefresh interface {
    // RefreshToken refreshes a token and returns the updated token.
    RefreshToken(ctx context.Context, token ProviderToken) (ProviderToken, error)
    // ProviderID returns the provider identifier.
    ProviderID() string
}
```

This interface must be implemented by each provider to support token refresh.

#### 2. RefreshCoordinator

**Location:** [`internal/token/token_refresh.go:38-154`](internal/token/token_refresh.go:38)

The `RefreshCoordinator` manages token refresh operations with:
- A worker pool (default: 3 workers)
- Request queue (capacity: 100)
- Result queue (capacity: 100)
- Registry of provider-specific refreshers

**Key Methods:**
- [`Start()`](internal/token/token_refresh.go:76) - Starts coordinator workers
- [`ScheduleRefresh()`](internal/token/token_refresh.go:157) - Schedules a token refresh
- [`RegisterRefresher()`](internal/token/token_refresh.go:180) - Registers a provider-specific refresher
- [`processRequest()`](internal/token/token_refresh.go:109) - Processes refresh requests using registered refreshers

#### 3. RefreshScheduler

**Location:** [`internal/token/token_refresh.go:186-299`](internal/token/token_refresh.go:186)

The `RefreshScheduler` periodically checks for expiring tokens:
- Default check interval: 5 minutes
- Checks all tokens in the store
- Schedules refreshes for tokens expiring within the buffer period

**Key Methods:**
- [`Start()`](internal/token/token_refresh.go:219) - Starts the scheduler
- [`CheckAndSchedule()`](internal/token/token_refresh.go:249) - Checks tokens and schedules refreshes
- [`run()`](internal/token/token_refresh.go:230) - Main scheduler loop

**Important:** The scheduler calls `CheckAndSchedule()` **immediately** upon starting, then every 5 minutes.

#### 4. MultiTokenManager

**Location:** [`internal/token/multi_token_manager.go:27-44`](internal/token/multi_token_manager.go:27)

The `MultiTokenManager` orchestrates all token management:
- Manages token stores per provider
- Creates and manages refresh schedulers
- Registers provider refreshers

**Key Methods:**
- [`Initialize()`](internal/token/multi_token_manager.go:89) - Initializes all components
- [`Start()`](internal/token/multi_token_manager.go:114) - Starts all refresh schedulers
- [`RegisterRefresher()`](internal/token/multi_token_manager.go:327) - Registers a provider refresher and creates scheduler

### Provider Refresher Implementations

All provider refreshers are **properly registered** in the REST API server:

**Location:** [`restapi/rest_api.go:173-230`](restapi/rest_api.go:173)

The following refreshers are registered:

1. **Gemini** ([`restapi/rest_api.go:182-192`](restapi/rest_api.go:182))
   - Implementation: [`provider/gemini/auth.go:404-495`](provider/gemini/auth.go:404)
   - Uses Google OAuth2 refresh token flow

2. **Qwen** ([`restapi/rest_api.go:195-200`](restapi/rest_api.go:195))
   - Implementation: [`provider/qwen/auth_device.go`](provider/qwen/auth_device.go)
   - Uses Qwen OAuth2 refresh token flow

3. **iFlow** ([`restapi/rest_api.go:203-213`](restapi/rest_api.go:203))
   - Implementation: [`provider/iflow/auth.go:839`](provider/iflow/auth.go:839)
   - Uses iFlow OAuth2 refresh token flow

4. **Antigravity** ([`restapi/rest_api.go:216-226`](restapi/rest_api.go:216))
   - Implementation: [`provider/antigravity/auth.go:94`](provider/antigravity/auth.go:94)
   - Uses Google OAuth2 refresh token flow (same as Gemini)

### Token Expiry Logic

**Location:** [`internal/token/token_refresh.go:280-299`](internal/token/token_refresh.go:280)

The system uses a **buffer period** to refresh tokens before they expire:

- **Default Refresh Buffer:** 1800 seconds (30 minutes) - [`internal/token/constants.go:15`](internal/token/constants.go:15)
- **Check Interval:** 5 minutes - [`internal/token/token_refresh.go:200`](internal/token/token_refresh.go:200)

A token is considered "expiring soon" if:
```go
remaining < bufferDuration
```

Where `remaining` is the time until token expiry and `bufferDuration` is 30 minutes.

### Token Storage

**Location:** [`internal/token/multi_token_store.go:17-54`](internal/token/multi_token_store.go:17)

Tokens are stored with the following structure:
```go
type ProviderToken struct {
    ID               string       `json:"id"`
    AccessToken      string       `json:"access_token"`
    RefreshToken     string       `json:"refresh_token"`
    TokenType        string       `json:"token_type"`
    ExpiryDate       int64        `json:"expiry_date"`
    Email            string       `json:"email"`
    Healthy          bool         `json:"healthy"`
    HealthScore      float64      `json:"health_score"`
    LastUsed         int64        `json:"last_used"`
    CreatedAt        int64        `json:"created_at"`
    ErrorCount       int          `json:"error_count"`
    LastError        string       `json:"last_error,omitempty"`
    // ... other fields
}
```

Tokens are stored in JSON files in the `.credentials/` directory.

---

## Startup Token Refresh Behavior

### Server Initialization

**Location:** [`restapi/rest_api.go:96-131`](restapi/rest_api.go:96)

When the server is created:

1. **MultiTokenManager is initialized** ([`restapi/rest_api.go:107-110`](restapi/rest_api.go:107))
   ```go
   multiTokenManager := tokpkg.NewMultiTokenManager(logger)
   if err := multiTokenManager.Initialize(); err != nil {
       logger.ErrorLog("Failed to initialize multi-token manager: %v", err)
   }
   ```

2. **MultiTokenManager is started** ([`restapi/rest_api.go:111-113`](restapi/rest_api.go:111))
   ```go
   if err := multiTokenManager.Start(); err != nil {
       logger.ErrorLog("Failed to start multi-token manager: %v", err)
   }
   ```

3. **Provider refreshers are registered** ([`restapi/rest_api.go:127-129`](restapi/rest_api.go:127))
   ```go
   if err := server.registerProviderRefreshers(); err != nil {
       logger.ErrorLog("Failed to register provider refreshers: %v", err)
   }
   ```

### MultiTokenManager.Start() Behavior

**Location:** [`internal/token/multi_token_manager.go:114-131`](internal/token/multi_token_manager.go:114)

When `MultiTokenManager.Start()` is called:

```go
func (mtm *MultiTokenManager) Start() error {
    mtm.mu.Lock()
    defer mtm.mu.Unlock()

    if !mtm.initialized {
        return fmt.Errorf("multi-token manager not initialized")
    }

    mtm.logger.InfoLog("[MultiTokenManager] Starting refresh schedulers...")

    // Start schedulers for all registered providers
    for providerID, scheduler := range mtm.schedulers {
        scheduler.Start()
        mtm.logger.InfoLog("[MultiTokenManager] Started scheduler for provider: %s", providerID)
    }

    return nil
}
```

**Critical Issue Identified:** The `Start()` method only starts schedulers that have already been registered. However, schedulers are only created when `RegisterRefresher()` is called.

### Registration Timing Issue

**Location:** [`restapi/rest_api.go:96-131`](restapi/rest_api.go:96)

The initialization sequence is:

1. Create MultiTokenManager
2. Initialize MultiTokenManager
3. **Start MultiTokenManager** ← Schedulers don't exist yet!
4. Register Provider Refreshers ← This creates schedulers

**Problem:** When `Start()` is called at step 3, no schedulers exist yet because they are only created during `RegisterRefresher()` at step 4.

### RefreshScheduler.Start() Behavior

**Location:** [`internal/token/token_refresh.go:218-246`](internal/token/token_refresh.go:218)

When a scheduler is started:

```go
func (rs *RefreshScheduler) Start() {
    rs.wg.Add(1)
    go rs.run()
}

func (rs *RefreshScheduler) run() {
    defer rs.wg.Done()

    ticker := time.NewTicker(rs.checkInterval)
    defer ticker.Stop()

    rs.CheckAndSchedule()  // ← Immediate check on start

    for {
        select {
        case <-rs.ctx.Done():
            return
        case <-ticker.C:
            rs.CheckAndSchedule()  // ← Then every 5 minutes
        }
    }
}
```

**Key Point:** The scheduler calls `CheckAndSchedule()` **immediately** upon starting, then every 5 minutes.

### CheckAndSchedule() Behavior

**Location:** [`internal/token/token_refresh.go:248-262`](internal/token/token_refresh.go:248)

```go
func (rs *RefreshScheduler) CheckAndSchedule() {
    tokens := rs.store.ListTokens()
    bufferSeconds := rs.store.Settings.RefreshBufferSec

    for _, token := range tokens {
        if !token.Healthy {
            continue
        }
        if isTokenExpiringSoon(token, bufferSeconds) {
            priority := calculateRefreshPriority(token, bufferSeconds)
            _ = rs.coordinator.ScheduleRefresh(token.ID, priority)
        }
    }
}
```

**Key Point:** This checks ALL tokens in the store and schedules refreshes for any token expiring within the buffer period (30 minutes).

---

## Cronjob for Auto-Refresh

### RefreshScheduler as Cronjob

The `RefreshScheduler` acts as a cronjob with the following characteristics:

**Location:** [`internal/token/token_refresh.go:186-299`](internal/token/token_refresh.go:186)

1. **Check Interval:** 5 minutes (configurable)
2. **Initial Check:** Immediate upon scheduler start
3. **Subsequent Checks:** Every 5 minutes thereafter

### Refresh Priority Calculation

**Location:** [`internal/token/token_refresh.go:264-278`](internal/token/token_refresh.go:264)

Tokens are prioritized based on how close they are to expiry:

```go
func calculateRefreshPriority(token ProviderToken, bufferSeconds int) int {
    remaining := getExpiryTimeRemaining(token)
    bufferDuration := time.Duration(bufferSeconds) * time.Second

    if remaining < bufferDuration-(5*time.Minute) {
        return 0  // Highest priority
    }
    if remaining < bufferDuration-(15*time.Minute) {
        return 10
    }
    if remaining < bufferDuration-(30*time.Minute) {
        return 20
    }
    return 30  // Lowest priority
}
```

### Refresh Coordinator Worker Pool

**Location:** [`internal/token/token_refresh.go:96-107`](internal/token/token_refresh.go:96)

The coordinator uses a worker pool to process refresh requests:

- **Default Workers:** 3
- **Request Queue Capacity:** 100
- **Result Queue Capacity:** 100

Workers continuously pull requests from the queue and process them using the appropriate provider refresher.

---

## Frontend Token Refresh

### Dashboard Initialization

**Location:** [`web/dashboard/js/dashboard.js:14-49`](web/dashboard/js/dashboard.js:14)

When the dashboard loads:

```javascript
async init() {
    // Initialize API client with saved base URL
    this.api.setBaseUrl(this.state.getApiBaseUrl());
    document.getElementById('apiBaseUrl').value = this.state.getApiBaseUrl();

    // Bind event listeners
    this.bindEvents();

    // Load initial data
    await this.loadData();

    // Start periodic refresh
    this.startPeriodicRefresh();

    // Render initial view
    this.render();
}
```

### Data Loading

**Location:** [`web/dashboard/js/dashboard.js:51-73`](web/dashboard/js/dashboard.js:51)

```javascript
async loadData() {
    try {
        this.loading.show();
        
        // Load providers
        const providersData = await this.api.getProviders();
        this.providers = providersData.providers || [];
        this.state.setProviders(this.providers);

        // Load credentials
        const credentialsData = await this.api.getCredentials();
        this.credentials = credentialsData.credentials || [];
        this.state.setCredentials(this.credentials);

        // Check server status
        await this.checkServerStatus();
    } catch (error) {
        this.log(`Failed to load data: ${error.message}`, 'error');
        this.toast.show('Failed to load data', 'error');
    } finally {
        this.loading.hide();
    }
}
```

**Important:** The frontend only loads token data, it does NOT trigger token refresh on startup.

### Periodic Refresh

**Location:** [`web/dashboard/js/dashboard.js:1644-1649`](web/dashboard/js/dashboard.js:1644)

```javascript
startPeriodicRefresh() {
    const interval = this.state.getAutoRefreshInterval() * 1000;
    this.refreshInterval = setInterval(() => {
        this.checkServerStatus();
    }, interval);
}
```

**Key Points:**
- Default interval: 30 seconds (configurable in settings)
- Only calls `checkServerStatus()`, NOT token refresh
- This is for UI data refresh, not token refresh

### Manual Token Refresh

**Location:** [`web/dashboard/js/dashboard.js:1049-1063`](web/dashboard/js/dashboard.js:1049)

```javascript
async refreshTokenById(providerId, tokenId) {
    try {
        this.log(`Refreshing token ${tokenId} for ${providerId}`, 'info');
        this.loading.show();
        await this.api.refreshTokenById(providerId, tokenId);
        this.log(`Token ${tokenId} refreshed successfully`, 'success');
        this.toast.show('Token refreshed successfully', 'success');
        await this.loadData();  // Reload data after refresh
    } catch (error) {
        this.log(`Failed to refresh token: ${error.message}`, 'error');
        this.toast.show('Failed to refresh token', 'error');
        throw error;
    } finally {
        this.loading.hide();
    }
}
```

**Key Points:**
- Manual refresh is triggered by the "Refresh" button in the UI
- Calls the API endpoint to refresh the token
- Reloads data after successful refresh

### Token Service

**Location:** [`web/dashboard/js/services/token-service.js:48-59`](web/dashboard/js/services/token-service.js:48)

```javascript
async refreshTokenById(providerId, tokenId) {
    try {
        this.log(`Refreshing token ${tokenId} for ${providerId}`, 'info');
        await this.api.refreshTokenById(providerId, tokenId);
        this.log(`Token ${tokenId} refreshed successfully`, 'success');
        this.toast.show('Token refreshed successfully', 'success');
    } catch (error) {
        this.log(`Failed to refresh token: ${error.message}`, 'error');
        this.toast.show('Failed to refresh token', 'error');
        throw error;
    }
}
```

---

## Key Findings and Issues

### Issue 1: Scheduler Registration Timing Problem

**Severity:** High

**Location:** [`restapi/rest_api.go:96-131`](restapi/rest_api.go:96)

**Problem:** The `MultiTokenManager.Start()` method is called BEFORE provider refreshers are registered. This means:

1. `Start()` is called at line 111
2. No schedulers exist yet (they're created during `RegisterRefresher()`)
3. `RegisterRefresher()` is called at line 127
4. Schedulers are created but never started

**Impact:** Tokens are NOT automatically refreshed on startup or periodically because the schedulers are never started.

**Current Behavior:**
- Schedulers are created during `RegisterRefresher()`
- But `Start()` was already called before schedulers existed
- Result: Schedulers exist but are not running

### Issue 2: No Startup Token Refresh

**Severity:** Medium

**Problem:** Even if schedulers were running correctly, there's no explicit startup token refresh. The scheduler only checks tokens that are expiring within the buffer period (30 minutes).

**Impact:** If tokens are already expired or will expire in more than 30 minutes, they won't be refreshed on startup.

**Current Behavior:**
- Scheduler checks tokens every 5 minutes
- Only refreshes tokens expiring within 30 minutes
- No immediate refresh of all tokens on startup

### Issue 3: Frontend Doesn't Trigger Backend Refresh

**Severity:** Low

**Problem:** The frontend dashboard only loads token data on startup. It does NOT trigger backend token refresh.

**Impact:** Users must manually click the "Refresh" button to refresh tokens.

**Current Behavior:**
- Frontend loads token data on page load
- Backend schedulers should handle automatic refresh (if working)
- Manual refresh is only via UI button

### Issue 4: No Explicit Refresh on Token Load

**Severity:** Low

**Problem:** When tokens are loaded from disk during startup, there's no check to refresh them if they're already expired or near expiry.

**Impact:** Tokens may be in an expired state when the application starts.

**Current Behavior:**
- Tokens are loaded from JSON files
- No immediate expiry check or refresh
- Relies on scheduler to catch expiring tokens

---

## Recommendations

### Recommendation 1: Fix Scheduler Registration Timing

**Priority:** Critical

**Solution:** Change the initialization order in [`restapi/rest_api.go:96-131`](restapi/rest_api.go:96):

```go
// Current order (BROKEN):
// 1. Create MultiTokenManager
// 2. Initialize MultiTokenManager
// 3. Start MultiTokenManager ← Too early!
// 4. Register Provider Refreshers

// Fixed order:
// 1. Create MultiTokenManager
// 2. Initialize MultiTokenManager
// 3. Register Provider Refreshers ← Creates schedulers
// 4. Start MultiTokenManager ← Now schedulers exist
```

**Implementation:**
```go
// Create multi-token manager
multiTokenManager := tokpkg.NewMultiTokenManager(logger)
if err := multiTokenManager.Initialize(); err != nil {
    logger.ErrorLog("Failed to initialize multi-token manager: %v", err)
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

### Recommendation 2: Add Startup Token Refresh

**Priority:** High

**Solution:** Add an explicit startup token refresh that checks all tokens and refreshes any that are expired or near expiry.

**Implementation:** Add a method to `MultiTokenManager`:

```go
// RefreshAllTokens refreshes all tokens that need refreshing
func (mtm *MultiTokenManager) RefreshAllTokens() error {
    mtm.mu.RLock()
    defer mtm.mu.RUnlock()

    for providerID, scheduler := range mtm.schedulers {
        mtm.logger.InfoLog("[MultiTokenManager] Checking tokens for provider: %s", providerID)
        scheduler.CheckAndSchedule()
    }

    return nil
}
```

Call this after starting the schedulers:

```go
// Start schedulers
if err := multiTokenManager.Start(); err != nil {
    logger.ErrorLog("Failed to start multi-token manager: %v", err)
}

// Refresh all tokens on startup
if err := multiTokenManager.RefreshAllTokens(); err != nil {
    logger.ErrorLog("Failed to refresh tokens on startup: %v", err)
}
```

### Recommendation 3: Reduce Refresh Buffer for Testing

**Priority:** Low

**Solution:** Make the refresh buffer configurable or reduce it for testing purposes.

**Current:** 30 minutes ([`internal/token/constants.go:15`](internal/token/constants.go:15))

**Suggested:** Make it configurable via environment variable or settings.

### Recommendation 4: Add Health Check on Token Load

**Priority:** Low

**Solution:** When tokens are loaded from disk, check if they're expired and mark them as unhealthy.

**Implementation:** In [`internal/token/multi_token_store.go:Load()`](internal/token/multi_token_store.go:82):

```go
func (mts *MultiTokenStore) Load() error {
    // ... existing code ...

    // Check token health after loading
    for i := range mts.Tokens {
        token := &mts.Tokens[i]
        if isTokenExpired(*token) {
            token.Healthy = false
            token.LastError = "Token expired"
            mts.logger.WarnLog("Token %s is expired", token.ID)
        }
    }

    return nil
}
```

---

## Summary

### Current State

1. ✅ Token refresh infrastructure exists and is well-designed
2. ✅ Provider refreshers are properly implemented
3. ✅ Provider refreshers are registered in the server
4. ❌ **Schedulers are created but never started** (critical bug)
5. ❌ No explicit startup token refresh
6. ❌ Frontend doesn't trigger backend refresh on load

### Root Cause

The primary issue is the **timing of scheduler registration and startup**:

1. `MultiTokenManager.Start()` is called before schedulers are created
2. Schedulers are only created during `RegisterRefresher()`
3. Result: Schedulers exist but are never started
4. Consequence: No automatic token refresh occurs

### Fix Required

Change the initialization order in [`restapi/rest_api.go:96-131`](restapi/rest_api.go:96):

1. Initialize MultiTokenManager
2. **Register Provider Refreshers** (creates schedulers)
3. **Start MultiTokenManager** (starts schedulers)
4. Refresh all tokens on startup (optional but recommended)

This will enable the cronjob functionality and ensure tokens are automatically refreshed before they expire.

---

## References

### Key Files

- [`internal/token/token_refresh.go`](internal/token/token_refresh.go) - Token refresh coordinator and scheduler
- [`internal/token/multi_token_manager.go`](internal/token/multi_token_manager.go) - Multi-token manager
- [`internal/token/multi_token_store.go`](internal/token/multi_token_store.go) - Token storage
- [`internal/token/constants.go`](internal/token/constants.go) - Constants and defaults
- [`restapi/rest_api.go`](restapi/rest_api.go) - REST API server
- [`provider/gemini/auth.go`](provider/gemini/auth.go) - Gemini token refresher
- [`provider/qwen/auth_device.go`](provider/qwen/auth_device.go) - Qwen token refresher
- [`provider/iflow/auth.go`](provider/iflow/auth.go) - iFlow token refresher
- [`provider/antigravity/auth.go`](provider/antigravity/auth.go) - Antigravity token refresher
- [`web/dashboard/js/dashboard.js`](web/dashboard/js/dashboard.js) - Frontend dashboard
- [`web/dashboard/js/services/token-service.js`](web/dashboard/js/services/token-service.js) - Token service
- [`plans/token-refresh-implementation.md`](plans/token-refresh-implementation.md) - Implementation plan

### Key Functions

- [`RefreshCoordinator.Start()`](internal/token/token_refresh.go:76) - Starts refresh workers
- [`RefreshScheduler.Start()`](internal/token/token_refresh.go:219) - Starts scheduler
- [`RefreshScheduler.CheckAndSchedule()`](internal/token/token_refresh.go:249) - Checks and schedules refreshes
- [`MultiTokenManager.Start()`](internal/token/multi_token_manager.go:114) - Starts all schedulers
- [`MultiTokenManager.RegisterRefresher()`](internal/token/multi_token_manager.go:327) - Registers refresher and creates scheduler
- [`Server.registerProviderRefreshers()`](restapi/rest_api.go:174) - Registers all provider refreshers

---

**Document Version:** 1.0  
**Date:** 2025-02-18  
**Analysis Mode:** Project Research
