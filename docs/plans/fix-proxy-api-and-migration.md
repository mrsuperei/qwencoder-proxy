# Fix Proxy API 405 Error and Database Migration Issues

## Summary

This plan addresses two related issues:
1. **405 Method Not Allowed** when trying to save proxies from the dashboard
2. **Warning about missing `type` column** in the `proxy_configs` table

## Issue Analysis

### Issue 1: 405 Method Not Allowed

**Root Cause:**
The `handleAddProxy` function exists in [`proxy_api.go`](../internal/restapi/proxy_api.go:101-167) but is NOT registered as a route. The route registration in [`rest_api.go`](../internal/restapi/rest_api.go:220-221) is:

```go
mux.HandleFunc("/api/proxies", s.handleListProxies)  // Only handles GET
mux.HandleFunc("/api/proxies/", s.handleProxy)     // Handles GET, PUT, DELETE
```

When the frontend sends POST to `/api/proxies` (without trailing slash), it goes to `handleListProxies` which only accepts GET requests, causing the 405 error.

The `handleProxy` function handles `/api/proxies/` (with trailing slash) but only supports GET, PUT, DELETE for specific proxy IDs - not POST for creating new proxies.

**Current Route Flow:**
```
POST /api/proxies → handleListProxies → 405 Method Not Allowed (only GET allowed)
```

### Issue 2: Missing type column warning

**Root Cause:**
The `proxy_configs` table schema includes the `type` column (defined in [`sqlite_store.go`](../internal/token/sqlite_store.go:66-76)), and migrations V2 and V3 should add it if missing. However:

1. The database may have been created before the `type` column was added to the schema
2. Multiple `SQLiteStore` instances are created (one per provider), each with its own `typeColumnWarningLogged` flag
3. Each instance logs the warning once when it first tries to query the `proxy_configs` table

The warning appears because the column check in `GetProxy` and `ListProxies` fails, and the fallback query is used.

## Proposed Solutions

### Solution 1: Fix Proxy API Routing

**Option A: Update handleProxy to handle POST for new proxies**
- Modify [`handleProxy`](../internal/restapi/proxy_api.go:377-405) to also handle POST requests
- When POST is received to `/api/proxies/` without an ID, call `handleAddProxy`

**Option B: Register handleAddProxy as a separate route**
- Update route registration to use a single handler that routes based on method
- Or register `handleAddProxy` for POST on `/api/proxies`

**Recommended Approach: Option A**
- Update `handleProxy` to handle POST requests for creating new proxies
- Check if the path is `/api/proxies/` (no ID) and method is POST, then call `handleAddProxy`
- This keeps the routing logic centralized

### Solution 2: Ensure Database Migrations Run Properly

The migration system exists but may not be running properly. The fix should:

1. **Verify migration execution**: Ensure migrations V2 and V3 run correctly
2. **Improve error handling**: Add better logging to understand why migrations might fail
3. **Remove fallback queries**: Once migrations are confirmed to work, remove the fallback query logic that logs the warning

**Additional Consideration:**
The `typeColumnWarningLogged` flag is per-instance. If multiple `SQLiteStore` instances are created, each will log the warning once. This is by design to avoid spamming the logs, but it can be confusing.

## Implementation Plan

### Step 1: Fix Proxy API Routing

**File:** [`internal/restapi/proxy_api.go`](../internal/restapi/proxy_api.go)

1. Update the `handleProxy` function to handle POST requests:

```go
// handleProxy handles all /api/proxies/ routes
func (s *Server) handleProxy(w http.ResponseWriter, r *http.Request) {
    // Check if this is a tokens request
    if strings.HasSuffix(r.URL.Path, "/tokens") {
        s.handleGetProxyTokens(w, r)
        return
    }

    // Extract proxy ID from path
    proxyID := extractProxyID(r.URL.Path)

    // If no proxy ID and POST method, handle as add proxy
    if proxyID == "" && r.Method == http.MethodPost {
        s.handleAddProxy(w, r)
        return
    }

    // If no proxy ID, return 404
    if proxyID == "" {
        WriteError(w, http.StatusNotFound, "not_found", "Proxy ID is required")
        return
    }

    // Route based on method
    switch r.Method {
    case http.MethodGet:
        s.handleGetProxy(w, r)
    case http.MethodPut:
        s.handleUpdateProxy(w, r)
    case http.MethodDelete:
        s.handleDeleteProxy(w, r)
    default:
        WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed")
    }
}
```

2. Alternatively, update route registration in [`internal/restapi/rest_api.go`](../internal/restapi/rest_api.go):

```go
// Register routes
mux.HandleFunc("/api/proxies", func(w http.ResponseWriter, r *http.Request) {
    switch r.Method {
    case http.MethodGet:
        s.handleListProxies(w, r)
    case http.MethodPost:
        s.handleAddProxy(w, r)
    default:
        WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed")
    }
})
mux.HandleFunc("/api/proxies/", s.handleProxy)
```

**Recommended:** Use the route registration approach (Option B) as it's cleaner and more explicit.

### Step 2: Verify and Improve Database Migrations

**File:** [`internal/token/sqlite_store.go`](../internal/token/sqlite_store.go)

1. Add better logging to understand migration failures:

```go
// migrateToV2 adds the type column to proxy_configs table
func (s *SQLiteStore) migrateToV2() error {
    s.logger.InfoLog("[SQLiteStore] Running V2 migration: Add type column to proxy_configs table")

    // Check if the type column already exists
    var columnName string
    err := s.db.QueryRow(`
        SELECT name FROM pragma_table_info('proxy_configs')
        WHERE name = 'type'
    `).Scan(&columnName)

    if err == nil {
        // Column already exists, skip migration
        s.logger.InfoLog("[SQLiteStore] V2: type column already exists in proxy_configs table, skipping migration")
        return nil
    }

    // Check if the error is because the table doesn't exist
    if strings.Contains(err.Error(), "no such table") {
        s.logger.InfoLog("[SQLiteStore] V2: proxy_configs table doesn't exist yet, will be created in V1")
        return nil
    }

    // Log the error for debugging
    s.logger.WarnLog("[SQLiteStore] V2: type column check failed: %v", err)

    // Try to add the type column
    if _, err := s.db.Exec(`
        ALTER TABLE proxy_configs ADD COLUMN type TEXT NOT NULL DEFAULT 'http'
    `); err != nil {
        // Check if it's a duplicate column error (column was added by another process)
        if strings.Contains(err.Error(), "duplicate column") {
            s.logger.InfoLog("[SQLiteStore] V2: type column already exists (duplicate column error), skipping migration")
            return nil
        }
        return fmt.Errorf("V2: failed to add type column to proxy_configs table: %w", err)
    }

    s.logger.InfoLog("[SQLiteStore] V2: Successfully added type column to proxy_configs table")
    return nil
}
```

2. Similarly improve logging in `migrateToV3`.

3. After confirming migrations work properly, consider removing the fallback query logic in `GetProxy` and `ListProxies`, or at least change the warning to an error since it indicates a migration failure.

### Step 3: Test the Fixes

1. Start the application and verify:
   - No warning about missing `type` column appears
   - POST to `/api/proxies` works correctly
   - Proxy can be saved from the dashboard

2. Test with an existing database (without the `type` column):
   - Verify migrations run and add the column
   - Verify no warnings appear after migration

## Files to Modify

1. [`internal/restapi/rest_api.go`](../internal/restapi/rest_api.go) - Update route registration
2. [`internal/restapi/proxy_api.go`](../internal/restapi/proxy_api.go) - Optionally update handleProxy
3. [`internal/token/sqlite_store.go`](../internal/token/sqlite_store.go) - Improve migration logging

## Testing Checklist

- [ ] Start application with fresh database - no warnings
- [ ] Start application with old database - migrations run, no warnings
- [ ] POST to `/api/proxies` works (405 error fixed)
- [ ] Proxy can be saved from dashboard
- [ ] Proxy can be updated (PUT)
- [ ] Proxy can be deleted (DELETE)
- [ ] Proxy list can be retrieved (GET)
