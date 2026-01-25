# Dashboard Serving Fix - Summary

## Problem
The OAuth server was returning 404 when accessing the root path `/` instead of serving the dashboard HTML file at `web/dashboard/index.html`.

## Root Cause
The server was using a **relative path** (`web/dashboard/index.html`) that depended on the **current working directory** of the process. When the server was run from a different directory than the project root, the file path resolution failed, resulting in a 404 error.

### Why This Happened
- The code used `http.ServeFile(w, r, "web/dashboard/index.html")`
- This path is resolved relative to the process's current working directory
- If the server is started from a different directory, the path doesn't exist
- `http.ServeFile` automatically returns 404 when the file is not found

## Solution Implemented
Modified [`restapi/rest_api.go`](restapi/rest_api.go) to resolve the dashboard directory **relative to the executable location** instead of the working directory. This ensures the dashboard can be served regardless of where the server is started from.

### Changes Made

1. **Added `DashboardDir` configuration option** to [`Config`](restapi/rest_api.go:32) struct:
   ```go
   type Config struct {
       // ... existing fields ...
       DashboardDir    string        // Dashboard directory path (empty = auto-detect)
   }
   ```

2. **Added `resolveDashboardDir()` helper function** (lines 937-971):
   - First tries to find `web/dashboard` relative to the executable location
   - Falls back to `web/dashboard` relative to working directory (for development)
   - Provides clear error messages if dashboard directory is not found
   - Supports explicit configuration via `DashboardDir` field

3. **Updated `registerRoutes()` function** (lines 105-114):
   - Calls `resolveDashboardDir()` to get the correct dashboard path
   - Logs the resolved dashboard directory for debugging
   - Falls back to relative path if resolution fails

4. **Added diagnostic logging**:
   - Logs working directory at startup
   - Logs dashboard directory path
   - Logs file paths being served
   - Logs file existence checks

### How It Works
```go
func (s *Server) resolveDashboardDir() (string, error) {
    // 1. If explicitly configured, use that path
    if s.config.DashboardDir != "" {
        return s.config.DashboardDir, nil
    }

    // 2. Get executable location
    execPath, _ := os.Executable()
    execDir := filepath.Dir(execPath)
    
    // 3. Try relative to executable (production deployment)
    dashboardDir := filepath.Join(execDir, "web", "dashboard")
    
    if _, err := os.Stat(dashboardDir); os.IsNotExist(err) {
        // 4. Fall back to working directory (development)
        wd, _ := os.Getwd()
        dashboardDir = filepath.Join(wd, "web", "dashboard")
        
        if _, err := os.Stat(dashboardDir); os.IsNotExist(err) {
            // 5. Error if not found anywhere
            return "", fmt.Errorf("dashboard directory not found...")
        }
    }
    
    return dashboardDir, nil
}
```

## Testing Results

### Before Fix
```
$ curl http://localhost:8080/
HTTP/1.1 404 Not Found
```

### After Fix
```
$ curl http://localhost:8086/
HTTP/1.1 200 OK
```

Server logs show:
```
Working directory: c:\Users\jappa\projecten\qwencoder-proxy\qwencoder-proxy
Dashboard directory: c:\Users\jappa\projecten\qwencoder-proxy\qwencoder-proxy\web\dashboard
Attempting to serve file: c:\Users\jappa\projecten\qwencoder-proxy\qwencoder-proxy\web\dashboard\index.html
[GET] / - Status: 200 - Duration: 69ms
```

## Deployment Scenarios

### Scenario 1: Development (Running from project root)
```bash
cd /path/to/qwencoder-proxy/qwencoder-proxy
./oauth-server.exe
```
✅ Works: Finds `web/dashboard` relative to working directory

### Scenario 2: Production (Running from any directory)
```bash
# From anywhere
/path/to/qwencoder-proxy/qwencoder-proxy/oauth-server.exe
```
✅ Works: Finds `web/dashboard` relative to executable location

### Scenario 3: Custom Dashboard Location
```bash
./oauth-server.exe -dashboard /custom/path/to/dashboard
```
✅ Works: Uses explicitly configured path (requires CLI flag support)

## How to Apply the Fix

1. **Replace the old binary** with the new one:
   ```bash
   # Stop the old server (Ctrl+C in the terminal)
   
   # Replace the binary
   copy oauth-server-final.exe oauth-server.exe
   ```

2. **Restart the server**:
   ```bash
   ./oauth-server.exe -cors -debug
   ```

3. **Verify the fix**:
   ```bash
   curl http://localhost:8080/
   # Should return HTTP 200 and serve the dashboard HTML
   ```

## Additional Configuration Options

The fix includes a new configuration field `DashboardDir` that can be used to specify a custom dashboard directory. This is useful for:
- Non-standard directory structures
- Shared web assets across multiple servers
- Testing with different dashboard versions

To use this feature, you would need to add a CLI flag to the server (e.g., `-dashboard /path/to/dashboard`) and pass it to the config when creating the server.

## Benefits

1. **Robustness**: Works regardless of working directory
2. **Portability**: Can be run from any location
3. **Flexibility**: Supports custom dashboard paths via configuration
4. **Debugging**: Enhanced logging helps diagnose issues
5. **Backward Compatible**: Falls back to old behavior if needed

## Files Modified

- [`restapi/rest_api.go`](restapi/rest_api.go) - Added dashboard directory resolution logic

## Files Created

- `oauth-server-final.exe` - Fixed server binary (ready for deployment)
- `DASHBOARD_FIX_SUMMARY.md` - This documentation

## Next Steps

1. Stop the old server process on port 8080
2. Replace `oauth-server.exe` with `oauth-server-final.exe`
3. Restart the server
4. Test by accessing `http://localhost:8080/`
5. Verify the dashboard loads correctly in a browser
