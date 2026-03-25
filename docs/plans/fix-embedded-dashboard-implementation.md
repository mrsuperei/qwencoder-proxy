# Fix Embedded Dashboard Implementation

## Problem Summary

The embedded dashboard implementation is experiencing MIME type errors and 404 errors because the current implementation deviates from the specified plan in `web2-dashboard-embed-implementation.md`.

### Current Errors
- All CSS files return `text/plain` instead of `text/css`
- JavaScript files return 404 errors
- Static assets not loading correctly

### Root Causes
1. **Wrong file location**: `embedded_dashboard.go` is in project root instead of `internal/restapi/`
2. **Wrong package name**: Using `package embedded` instead of `package restapi`
3. **Incorrect embed directives**: Single directive instead of multiple specific ones
4. **Filesystem structure mismatch**: Handlers expect different paths than what's embedded

## Solution Overview

This fix will:
1. Move the embedded dashboard handler to the correct location
2. Fix the package declaration and embed directives
3. Update imports and references
4. Ensure proper MIME types for all static assets

## Step-by-Step Fix

### Step 1: Delete the Incorrect File

**Action**: Delete the file in the wrong location
```bash
rm qwencoder-proxy/embedded_dashboard.go
```

**Why**: This file is in the wrong directory and has the wrong package structure.

### Step 2: Create the Correct Embedded Dashboard Handler

**Action**: Create `qwencoder-proxy/internal/restapi/embedded_dashboard.go` with the following content:

```go
package restapi

import (
	"embed"
	"io/fs"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/sunbankio/qwencoder-proxy/internal/logging"
)

//go:embed ../../web2/*
var web2FS embed.FS

//go:embed ../../web2/css/*
var web2CSS embed.FS

//go:embed ../../web2/js/*
var web2JS embed.FS

//go:embed ../../web2/assets/*
var web2Assets embed.FS

// EmbeddedDashboardHandler serves the web2 dashboard from embedded files
type EmbeddedDashboardHandler struct {
	logger      logging.Logger
	web2FS      fs.FS
	web2CSS     fs.FS
	web2JS      fs.FS
	web2Assets  fs.FS
}

// NewEmbeddedDashboardHandler creates a new handler for embedded dashboard
func NewEmbeddedDashboardHandler(logger logging.Logger) *EmbeddedDashboardHandler {
	return &EmbeddedDashboardHandler{
		logger:     logger,
		web2FS:     web2FS,
		web2CSS:    web2CSS,
		web2JS:     web2JS,
		web2Assets: web2Assets,
	}
}

// ServeIndex serves the index.html file
func (h *EmbeddedDashboardHandler) ServeIndex(w http.ResponseWriter, r *http.Request) {
	h.logger.InfoLog("[EmbeddedDashboard] Serving index.html")

	// Read index.html from embedded filesystem
	content, err := fs.ReadFile(h.web2FS, "index.html")
	if err != nil {
		h.logger.ErrorLog("[EmbeddedDashboard] Failed to read index.html: %v", err)
		http.Error(w, "Dashboard not found", http.StatusNotFound)
		return
	}

	// Set proper Content-Type header
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")

	// Write content
	w.Write(content)
}

// ServeCSS serves CSS files with proper MIME type
func (h *EmbeddedDashboardHandler) ServeCSS(w http.ResponseWriter, r *http.Request) {
	// Get the CSS file path (strip prefix)
	filePath := strings.TrimPrefix(r.URL.Path, "/css/")
	h.logger.InfoLog("[EmbeddedDashboard] Serving CSS: %s", filePath)

	// Read CSS file from embedded filesystem
	content, err := fs.ReadFile(h.web2CSS, filePath)
	if err != nil {
		h.logger.ErrorLog("[EmbeddedDashboard] Failed to read CSS file %s: %v", filePath, err)
		http.NotFound(w, r)
		return
	}

	// Set proper Content-Type header
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")

	// Write content
	w.Write(content)
}

// ServeJS serves JavaScript files with proper MIME type
func (h *EmbeddedDashboardHandler) ServeJS(w http.ResponseWriter, r *http.Request) {
	// Get the JS file path (strip prefix)
	filePath := strings.TrimPrefix(r.URL.Path, "/js/")
	h.logger.InfoLog("[EmbeddedDashboard] Serving JS: %s", filePath)

	// Read JS file from embedded filesystem
	content, err := fs.ReadFile(h.web2JS, filePath)
	if err != nil {
		h.logger.ErrorLog("[EmbeddedDashboard] Failed to read JS file %s: %v", filePath, err)
		http.NotFound(w, r)
		return
	}

	// Set proper Content-Type header
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")

	// Write content
	w.Write(content)
}

// ServeAssets serves asset files (images, etc.) with proper MIME type
func (h *EmbeddedDashboardHandler) ServeAssets(w http.ResponseWriter, r *http.Request) {
	// Get the asset file path (strip prefix)
	filePath := strings.TrimPrefix(r.URL.Path, "/assets/")
	h.logger.InfoLog("[EmbeddedDashboard] Serving asset: %s", filePath)

	// Determine MIME type based on file extension
	ext := strings.ToLower(filepath.Ext(filePath))
	var contentType string
	switch ext {
	case ".png":
		contentType = "image/png"
	case ".jpg", ".jpeg":
		contentType = "image/jpeg"
	case ".gif":
		contentType = "image/gif"
	case ".svg":
		contentType = "image/svg+xml"
	case ".ico":
		contentType = "image/x-icon"
	case ".woff":
		contentType = "font/woff"
	case ".woff2":
		contentType = "font/woff2"
	case ".ttf":
		contentType = "font/ttf"
	default:
		contentType = "application/octet-stream"
	}

	// Read asset file from embedded filesystem
	content, err := fs.ReadFile(h.web2Assets, filePath)
	if err != nil {
		h.logger.ErrorLog("[EmbeddedDashboard] Failed to read asset %s: %v", filePath, err)
		http.NotFound(w, r)
		return
	}

	// Set proper Content-Type header
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "public, max-age=3600")

	// Write content
	w.Write(content)
}
```

**Key Changes from Previous Implementation**:
- Package changed from `embedded` to `restapi`
- Four separate embed directives instead of one
- Simplified handler methods using direct filesystem access
- No `fs.Sub()` calls - direct file reading from specific embedded filesystems

### Step 3: Update `internal/restapi/rest_api.go`

**File**: `qwencoder-proxy/internal/restapi/rest_api.go`

#### 3.1 Remove Incorrect Import

**Location**: Line 19

**Remove this line**:
```go
embedded "github.com/sunbankio/qwencoder-proxy"
```

#### 3.2 Update Server Struct

**Location**: Lines 57-71

**Change line 70 from**:
```go
embeddedDashboard *embedded.DashboardHandler
```

**To**:
```go
embeddedDashboard *EmbeddedDashboardHandler
```

#### 3.3 Update NewServer Function

**Location**: Lines 73-98

**Change line 93 from**:
```go
embeddedDashboard: embedded.NewDashboardHandler(logger),
```

**To**:
```go
embeddedDashboard: NewEmbeddedDashboardHandler(logger),
```

#### 3.4 Verify registerRoutes Function

**Location**: Lines 205-274

**Ensure these lines are present and correct**:
```go
// Serve CSS files from embedded filesystem
mux.HandleFunc(dashboardPrefix+"/css/", s.embeddedDashboard.ServeCSS)

// Serve JavaScript files from embedded filesystem
mux.HandleFunc(dashboardPrefix+"/js/", s.embeddedDashboard.ServeJS)

// Serve assets (images, fonts, etc.) from embedded filesystem
mux.HandleFunc(dashboardPrefix+"/assets/", s.embeddedDashboard.ServeAssets)

// Handle dashboard root - serve index.html
mux.HandleFunc(dashboardPrefix, func(w http.ResponseWriter, r *http.Request) {
	// Only serve index.html for exact root path
	if r.URL.Path == dashboardPrefix || r.URL.Path == dashboardPrefix+"/" {
		s.embeddedDashboard.ServeIndex(w, r)
		return
	}

	// For any other path, return 404
	http.NotFound(w, r)
})
```

### Step 4: Verify `cmd/main.go`

**File**: `qwencoder-proxy/cmd/main.go`

**No changes needed** - The API configuration is already correct. The dashboard configuration should look like this (around lines 206-224):

```go
// Create REST API server for dashboard and OAuth flows
apiConfig := &restapi.Config{
	Port:            cfg.Server.Port,
	CallbackBaseURL: "http://localhost:" + cfg.Server.Port,
	StateTTL:        10 * time.Minute,
	DeviceCodeTTL:   15 * time.Minute,
	EnableCORS:      true,
	AllowedOrigins:  []string{"*"},
	DashboardPath:   "/",
}
```

### Step 5: Rebuild the Project

**Action**: Build the project with the corrected implementation
```bash
cd qwencoder-proxy
go build -o qwencoder-proxy.exe ./cmd/main.go
```

**Expected Result**: Binary builds successfully without errors.

### Step 6: Test the Dashboard

**Action**: Start the server and test
```bash
./qwencoder-proxy.exe
```

**Expected Logs**:
```
[registerRoutes] Registering embedded web2 dashboard routes
[registerRoutes] Dashboard routes registered at path: /
Starting qwencoder-proxy server on port 8143
Server listening on :8143
```

**Testing Steps**:

1. Open browser to `http://localhost:8143`
2. Open Developer Tools (F12) and check Console tab
3. Verify no MIME type errors
4. Verify no 404 errors for CSS/JS files
5. Check that dashboard displays with proper styling

**Expected Results**:
- ✅ No MIME type errors in console
- ✅ All CSS files load with `Content-Type: text/css`
- ✅ All JS files load with `Content-Type: application/javascript`
- ✅ Dashboard displays correctly with styling
- ✅ All interactive features work

### Step 7: Verify Static File Serving

**Action**: Test individual files with curl

```bash
# Test CSS file
curl -I http://localhost:8143/css/reset.css
# Expected: Content-Type: text/css; charset=utf-8

# Test JS file
curl -I http://localhost:8143/js/main.js
# Expected: Content-Type: application/javascript; charset=utf-8

# Test index.html
curl -I http://localhost:8143/
# Expected: Content-Type: text/html; charset=utf-8
```

## Verification Checklist

After implementing the fix, verify:

- [ ] File `embedded_dashboard.go` deleted from project root
- [ ] File `embedded_dashboard.go` created in `internal/restapi/`
- [ ] Package declaration is `package restapi`
- [ ] Four separate embed directives present
- [ ] No external embedded package import in `rest_api.go`
- [ ] Server struct references `*EmbeddedDashboardHandler`
- [ ] NewServer creates handler with `NewEmbeddedDashboardHandler(logger)`
- [ ] Binary builds successfully
- [ ] Server starts without errors
- [ ] Dashboard loads at `http://localhost:8143`
- [ ] No MIME type errors in browser console
- [ ] All CSS files load with correct Content-Type
- [ ] All JS files load with correct Content-Type
- [ ] All assets load with correct Content-Type
- [ ] Dashboard displays correctly with styling
- [ ] All API endpoints work
- [ ] Dashboard functionality works (tabs, forms, etc.)

## Common Issues and Solutions

### Issue: Build fails with "cannot find package"
**Solution**: Ensure the new file is in `internal/restapi/` directory and has `package restapi` declaration.

### Issue: MIME type errors persist
**Solution**: Verify that the embed directives are correct and the web2 directory structure matches the paths in handlers.

### Issue: 404 errors for static files
**Solution**: Check that the file paths in the web2 directory match what the handlers expect. Ensure `main.js` exists at `web2/js/main.js`.

### Issue: Dashboard loads but no styling
**Solution**: Check browser console for specific CSS file errors. Verify that CSS files are being served with correct Content-Type.

## Technical Details

### Why This Fix Works

1. **Correct Package Structure**: By placing the handler in `internal/restapi/` with `package restapi`, we avoid import complexity and ensure the handler is part of the same package.

2. **Separate Embed Directives**: Using four separate embed directives (`web2FS`, `web2CSS`, `web2JS`, `web2Assets`) provides direct access to specific file types without complex path manipulation.

3. **Direct File Reading**: Using `fs.ReadFile()` directly on the specific embedded filesystems avoids the need for `fs.Sub()` calls, which were causing path mismatches.

4. **Proper MIME Types**: Each handler explicitly sets the correct Content-Type header before writing the response, ensuring browsers interpret files correctly.

### File Structure After Fix

```
qwencoder-proxy/
├── internal/
│   └── restapi/
│       ├── embedded_dashboard.go  ✅ Correct location
│       ├── rest_api.go
│       └── ... (other restapi files)
├── web2/
│   ├── index.html
│   ├── css/
│   │   ├── reset.css
│   │   ├── variables.css
│   │   └── ... (other CSS files)
│   ├── js/
│   │   ├── main.js
│   │   ├── app.js
│   │   └── ... (other JS files)
│   └── assets/
│       └── images/
└── cmd/
    └── main.go
```

## Rollback Plan

If issues arise after implementing this fix:

1. **Restore original files**:
   ```bash
   git checkout internal/restapi/rest_api.go
   ```

2. **Delete the new file**:
   ```bash
   rm internal/restapi/embedded_dashboard.go
   ```

3. **Restore the old file** (if backed up):
   ```bash
   # Restore embedded_dashboard.go from backup if available
   ```

4. **Rebuild**:
   ```bash
   go build -o qwencoder-proxy.exe ./cmd/main.go
   ```

## Summary

This fix addresses the root causes of the MIME type and 404 errors by:
- Correcting the file location and package structure
- Using proper embed directives for different file types
- Simplifying the handler implementation
- Ensuring proper MIME types are set for all static assets

After implementing these changes, the embedded dashboard should work correctly with proper MIME types and no 404 errors.
