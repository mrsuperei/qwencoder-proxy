# iFlow OAuth Authentication Issue Analysis

**Issue:** iFlow OAuth authentication fails after account selection - clicking "Authorize" redirects to iFlow login page, after login redirects to choose account, but selecting account results in no action.

**Date:** 2025-02-18  
**Status:** Analysis Complete

---

## Executive Summary

After thoroughly analyzing the current iFlow OAuth implementation and comparing it with a working reference implementation, **multiple critical issues** have been identified. The primary cause is that the current implementation uses **standard OAuth2 parameters** while iFlow requires **custom parameters** for their authorization flow.

---

## Critical Issues Found

### Issue #1: Wrong Authorization URL Parameters (PRIMARY ISSUE)

**Current Implementation** ([`restapi/rest_api.go:823-831`](../restapi/rest_api.go:823-831)):
```go
authURL := fmt.Sprintf(
    "%s?client_id=%s&redirect_uri=%s&response_type=code&scope=%s&access_type=offline&prompt=consent&state=%s&code_challenge=%s&code_challenge_method=S256",
    config.AuthURL,
    url.QueryEscape(config.ClientID),
    url.QueryEscape(redirectURI),
    url.QueryEscape(strings.Join(config.Scopes, " ")),
    url.QueryEscape(state),
    url.QueryEscape(codeChallenge),
)
```

**Working Implementation** (from reference code):
```go
values := url.Values{}
values.Set("loginMethod", "phone")
values.Set("type", "phone")
values.Set("redirect", redirectURI)
values.Set("state", state)
values.Set("client_id", iFlowOAuthClientID)
authURL = fmt.Sprintf("%s?%s", iFlowOAuthAuthorizeEndpoint, values.Encode())
```

**Parameter Comparison:**

| Parameter | Current | Working | Issue |
|-----------|---------|---------|-------|
| `loginMethod` | ❌ Missing | ✅ `phone` | **Required by iFlow** |
| `type` | ❌ Missing | ✅ `phone` | **Required by iFlow** |
| `redirect` | ❌ Uses `redirect_uri` | ✅ `redirect` | **Wrong parameter name** |
| `response_type` | ✅ `code` | ❌ Missing | iFlow doesn't expect this |
| `scope` | ✅ `openid email profile offline_access` | ❌ Missing | iFlow doesn't use scopes |
| `access_type` | ✅ `offline` | ❌ Missing | iFlow doesn't expect this |
| `prompt` | ✅ `consent` | ❌ Missing | iFlow doesn't expect this |
| `code_challenge` | ✅ PKCE | ❌ Missing | iFlow may not support PKCE |
| `code_challenge_method` | ✅ `S256` | ❌ Missing | iFlow may not support PKCE |

**Impact:** iFlow's authorization server doesn't recognize standard OAuth2 parameters and is missing required parameters, causing the flow to hang after account selection.

---

### Issue #2: Wrong Redirect URI Port and Path

**Current Implementation:**
- Redirect URI: `http://localhost:8080/api/callback` ([`restapi/rest_api.go:806`](../restapi/rest_api.go:806))
- Port: 8080 (dashboard server port)
- Path: `/api/callback`

**Working Implementation:**
- Redirect URI: `http://localhost:11451/oauth2callback`
- Port: 11451 (defined in [`provider/iflow/iflow.go:28`](../provider/iflow/iflow.go:28) as `DefaultPort`)
- Path: `/oauth2callback`

**Impact:** The redirect URI mismatch prevents iFlow from properly redirecting back to the application after authorization.

---

### Issue #3: Missing iFlow Success Redirect URL

**Current Implementation:**
- ❌ No success redirect URL defined

**Working Implementation:**
```go
const iFlowSuccessRedirectURL = "https://iflow.cn/oauth/success"
```

**Impact:** iFlow may require this URL to complete the authorization flow properly.

---

### Issue #4: PKCE Support Uncertainty

The current implementation uses PKCE (Proof Key for Code Exchange) with `code_challenge` and `code_verifier`. However, the working implementation doesn't use PKCE at all. This suggests iFlow may not support PKCE, and its inclusion could be causing issues.

**PKCE Code Generation** ([`restapi/rest_api.go:792-801`](../restapi/rest_api.go:792-801)):
```go
codeVerifier, err := generateCodeVerifier()
if err != nil {
    s.logger.ErrorLog("[handleAuthStart] Failed to generate code verifier: %v", err)
    WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to generate code verifier")
    return
}
codeChallenge := generateCodeChallenge(codeVerifier)
```

---

### Issue #5: Callback Handler Mismatch

The callback handler ([`restapi/rest_api.go:845-935`](../restapi/rest_api.go:845-935)) expects to receive callbacks at `/api/callback` on port 8080, but iFlow is configured to redirect to a different port (11451) and path (`/oauth2callback`).

**Current Callback Handler:**
```go
func (s *Server) handleCallback(w http.ResponseWriter, r *http.Request) {
    // Handles callbacks at /api/callback on port 8080
}
```

**Expected Callback Handler:**
- Should handle `/oauth2callback` on port 11451

---

## Root Cause Analysis

### Why "Nothing Happens" After Account Selection

1. **Wrong Parameters:** iFlow's authorization page receives parameters it doesn't understand (`response_type`, `scope`, `access_type`, `prompt`, `code_challenge`, `code_challenge_method`) and is missing required parameters (`loginMethod`, `type`).

2. **Wrong Redirect URI:** When you select an account, iFlow tries to redirect to `http://localhost:8080/api/callback` but expects `http://localhost:11451/oauth2callback`.

3. **No Callback Handler:** The current implementation doesn't have a handler for `/oauth2callback` on port 11451, so the redirect fails silently.

4. **PKCE Incompatibility:** iFlow may not support PKCE, causing the authorization flow to fail silently.

---

## Recommended Fixes

### Fix #1: Update Authorization URL Parameters for iFlow

**File:** [`restapi/rest_api.go:823-831`](../restapi/rest_api.go:823-831)

**Change:** Modify authorization URL generation to use iFlow-specific parameters:

```go
// Build authorization URL
var authURL string
if req.Provider == "iflow" {
    // iFlow uses custom parameters
    authURL = fmt.Sprintf(
        "%s?loginMethod=phone&type=phone&redirect=%s&state=%s&client_id=%s",
        config.AuthURL,
        url.QueryEscape(redirectURI),
        url.QueryEscape(state),
        url.QueryEscape(config.ClientID),
    )
} else {
    // Standard OAuth2 for other providers
    authURL = fmt.Sprintf(
        "%s?client_id=%s&redirect_uri=%s&response_type=code&scope=%s&access_type=offline&prompt=consent&state=%s&code_challenge=%s&code_challenge_method=S256",
        config.AuthURL,
        url.QueryEscape(config.ClientID),
        url.QueryEscape(redirectURI),
        url.QueryEscape(strings.Join(config.Scopes, " ")),
        url.QueryEscape(state),
        url.QueryEscape(codeChallenge),
    )
}
```

---

### Fix #2: Update Redirect URI for iFlow

**File:** [`restapi/rest_api.go:804-807`](../restapi/rest_api.go:804-807)

**Change:** Use the correct redirect URI for iFlow:

```go
// Determine redirect URI
redirectURI := req.RedirectURI
if redirectURI == "" {
    if req.Provider == "iflow" {
        // iFlow expects redirect to port 11451 with /oauth2callback path
        redirectURI = fmt.Sprintf("http://localhost:%d/oauth2callback", 11451)
    } else {
        redirectURI = fmt.Sprintf("%s/api/callback", s.config.CallbackBaseURL)
    }
}
```

---

### Fix #3: Add OAuth2 Callback Handler on Port 11451

**File:** Create new file or modify [`provider/iflow/auth.go`](../provider/iflow/auth.go)

**Change:** Add a callback server for iFlow on port 11451:

```go
// In provider/iflow/auth.go
import (
    "fmt"
    "net/http"
)

// startCallbackServer starts an HTTP server to handle iFlow OAuth callbacks
func (a *Authenticator) startCallbackServer() error {
    mux := http.NewServeMux()
    mux.HandleFunc("/oauth2callback", a.handleOAuth2Callback)
    
    a.GetLogger().InfoLog("[iFlow] Starting OAuth callback server on port %d", DefaultPort)
    return http.ListenAndServe(fmt.Sprintf(":%d", DefaultPort), mux)
}

// handleOAuth2Callback handles the OAuth2 callback from iFlow
func (a *Authenticator) handleOAuth2Callback(w http.ResponseWriter, r *http.Request) {
    a.GetLogger().InfoLog("[iFlow Callback] Received callback")
    
    // Extract query parameters
    code := r.URL.Query().Get("code")
    state := r.URL.Query().Get("state")
    errorCode := r.URL.Query().Get("error")
    errorDesc := r.URL.Query().Get("error_description")
    
    a.GetLogger().InfoLog("[iFlow Callback] Code: %s, State: %s, Error: %s", code, state, errorCode)
    
    // Handle similar to current /api/callback handler
    // but without PKCE since iFlow doesn't use it
    
    // Write success response
    w.Header().Set("Content-Type", "text/html")
    html := `<!DOCTYPE html>
<html>
<head><title>Authorization Successful</title></head>
<body>
  <h1>Authorization Successful!</h1>
  <p>You can close this window and return to your application.</p>
  <script>
    if (window.opener) {
      window.opener.postMessage({type: 'oauth_success'}, '*');
    }
    setTimeout(() => window.close(), 2000);
  </script>
</body>
</html>`
    w.Write([]byte(html))
}
```

**Update:** Modify the `Authenticate` method to start the callback server:

```go
// In provider/iflow/auth.go, Authenticate method
func (a *Authenticator) Authenticate(ctx context.Context) error {
    // Start callback server in background
    go func() {
        if err := a.startCallbackServer(); err != nil {
            a.GetLogger().ErrorLog("[iFlow] Failed to start callback server: %v", err)
        }
    }()
    
    // Rest of authentication flow...
}
```

---

### Fix #4: Remove PKCE for iFlow

**File:** [`restapi/rest_api.go:783-801`](../restapi/rest_api.go:783-801)

**Change:** Skip PKCE code generation for iFlow:

```go
// Generate state and PKCE codes
stateBytes := make([]byte, 16)
if _, err := rand.Read(stateBytes); err != nil {
    s.logger.ErrorLog("[handleAuthStart] Failed to generate state bytes: %v", err)
    WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to generate state")
    return
}
state := base64.URLEncoding.EncodeToString(stateBytes)

var codeVerifier, codeChallenge string
var err error

// Only generate PKCE codes for providers that support it
if req.Provider != "iflow" {
    codeVerifier, err = generateCodeVerifier()
    if err != nil {
        s.logger.ErrorLog("[handleAuthStart] Failed to generate code verifier: %v", err)
        WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to generate code verifier")
        return
    }
    codeChallenge = generateCodeChallenge(codeVerifier)
}

s.logger.InfoLog("[handleAuthStart] PKCE generated - verifier length: %d, challenge length: %d",
    len(codeVerifier), len(codeChallenge))
```

---

### Fix #5: Update Token Exchange for iFlow

**File:** [`restapi/rest_api.go:972-1015`](../restapi/rest_api.go:972-1015)

**Change:** Remove `code_verifier` parameter for iFlow:

```go
func (s *Server) exchangeCodeForTokensWithResponse(config *ProviderConfig, code, redirectURI, codeVerifier string) (tokpkg.OAuthCreds, map[string]interface{}, error) {
    s.logger.InfoLog("[exchangeCodeForTokensWithResponse] Exchanging code for tokens - TokenURL: %s", config.TokenURL)

    data := url.Values{}
    data.Set("client_id", config.ClientID)
    data.Set("client_secret", config.ClientSecret)
    data.Set("code", code)
    data.Set("grant_type", "authorization_code")
    
    // Use correct redirect parameter name for iFlow
    if config.ID == "iflow" {
        data.Set("redirect", redirectURI)
    } else {
        data.Set("redirect_uri", redirectURI)
        if codeVerifier != "" {
            data.Set("code_verifier", codeVerifier)
        }
    }

    s.logger.InfoLog("[exchangeCodeForTokensWithResponse] Request data prepared - client_id: %s, redirect_uri: %s, has_code_verifier: %v",
        config.ClientID, redirectURI, codeVerifier != "")
    
    // Rest of function...
}
```

---

## File Locations Reference

| Component | File | Line Numbers | Description |
|-----------|------|--------------|-------------|
| Authorization URL generation | [`restapi/rest_api.go`](../restapi/rest_api.go:823-831) | 823-831 | Builds OAuth authorization URL |
| Redirect URI determination | [`restapi/rest_api.go`](../restapi/rest_api.go:804-807) | 804-807 | Determines callback URL |
| PKCE code generation | [`restapi/rest_api.go`](../restapi/rest_api.go:792-801) | 792-801 | Generates PKCE codes |
| Callback handler | [`restapi/rest_api.go`](../restapi/rest_api.go:845-935) | 845-935 | Handles OAuth callbacks |
| Token exchange | [`restapi/rest_api.go`](../restapi/rest_api.go:972-1015) | 972-1015 | Exchanges code for tokens |
| Provider config | [`restapi/provider_registry.go`](../restapi/provider_registry.go:80-92) | 80-92 | iFlow provider configuration |
| iFlow constants | [`provider/iflow/iflow.go`](../provider/iflow/iflow.go:20-30) | 20-30 | iFlow endpoints and constants |
| State manager | [`restapi/state_manager.go`](../restapi/state_manager.go:1) | 1-280 | OAuth state management |
| Dashboard auth start | [`web/dashboard/js/dashboard.js`](../web/dashboard/js/dashboard.js:925-952) | 925-952 | Frontend auth initiation |
| API client | [`web/dashboard/js/api/client.js`](../web/dashboard/js/api/client.js:105-109) | 105-109 | API communication |

---

## Testing Recommendations

After implementing fixes:

1. **Test Authorization URL Generation:**
   - Verify that iFlow authorization URL contains `loginMethod=phone`, `type=phone`, and `redirect` parameters
   - Verify that redirect URI points to `http://localhost:11451/oauth2callback`

2. **Test Callback Handling:**
   - Ensure callback server is running on port 11451
   - Verify that `/oauth2callback` endpoint is accessible
   - Test that callback properly exchanges code for tokens

3. **Test Token Exchange:**
   - Verify token exchange uses correct parameter names (`redirect` instead of `redirect_uri`)
   - Ensure no `code_verifier` is sent for iFlow

4. **Test End-to-End Flow:**
   - Click "Authorize" button in dashboard
   - Verify redirect to iFlow login page
   - Login and select account
   - Verify redirect back to application
   - Verify token is saved successfully

---

## Conclusion

The iFlow OAuth flow is failing because the current implementation uses standard OAuth2 parameters while iFlow requires custom parameters (`loginMethod`, `type`, `redirect` instead of `redirect_uri`). Additionally, the redirect URI port and path are incorrect, and PKCE may not be supported by iFlow.

The fixes outlined above should resolve the issue by matching the working implementation's approach. The primary changes needed are:

1. Use iFlow-specific authorization URL parameters
2. Update redirect URI to use port 11451 and `/oauth2callback` path
3. Add a callback handler on port 11451
4. Remove PKCE for iFlow
5. Use correct parameter names in token exchange

---

## References

- **Working Implementation:** Reference code provided by user (CLIProxyAPI/v6)
- **Current Implementation:** qwencoder-proxy project
- **OAuth2 Standard:** RFC 6749
- **PKCE Standard:** RFC 7636

---

**Report Generated:** 2025-02-18  
**Analyzed By:** Project Research Mode  
**Priority:** HIGH - Critical authentication failure
