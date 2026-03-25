# Fix Qwen Device Flow Email Extraction

## Problem Description

When using the OAuth device flow for Qwen provider, the user email is not being extracted correctly. Instead, it defaults to `unknown@example.com` or an empty string.

## Root Cause Analysis

### Current Implementation

**File:** [`internal/provider/qwen/auth_device.go`](internal/provider/qwen/auth_device.go)

The device flow implementation creates a `tokenResponse` map with only a few manually specified fields:

```go
// Line 105-110
tokenResponse := make(map[string]interface{})
tokenResponse["access_token"] = oauth2Token.AccessToken
tokenResponse["refresh_token"] = oauth2Token.RefreshToken
tokenResponse["token_type"] = oauth2Token.TokenType
tokenResponse["expires_in"] = int64(oauth2Token.Expiry.Sub(time.Now()).Seconds())
```

### The Issue

1. **Missing Fields from OAuth Response**: The `oauth2Token` returned by `conf.DeviceAccessToken()` from `golang.org/x/oauth2` package contains an `Extra` map that holds additional fields from the OAuth token response. The current code only copies a few specific fields and ignores the rest.

2. **Email in Token Response**: OAuth 2.0 flows with `openid profile email` scopes typically return user information (including email) in the token response. This email field is likely present in `oauth2Token.Extra` but is not being copied to `tokenResponse`.

3. **Fallback Failure**: When email is not found in the manually constructed `tokenResponse`, the code falls back to calling the user info endpoint (`https://qwen-api.aliyuncs.com/v1/user/info`). This endpoint may:
   - Not be the correct endpoint for Qwen's OAuth device flow
   - Require different authentication
   - Return data in a different format

### Call Chain

```
AuthenticateWithDeviceFlow()
  └─> conf.DeviceAccessToken() returns oauth2.Token
       └─> tokenResponse map created with limited fields
            └─> multiTokenMgr.ExtractEmail(ctx, "qwen", tokenResponse, accessToken)
                 └─> QwenEmailExtractor.ExtractEmail()
                      ├─> extractEmailFromTokenResponse() → fails (no email in tokenResponse)
                      └─> fetchEmailFromUserInfo() → likely fails (wrong endpoint or auth)
                           └─> Returns error
                                └─> Email set to "" (line 115) or "unknown@example.com" in other flows
```

## Solution

### Approach 1: Copy All Extra Fields (Recommended)

Modify the device flow to copy all fields from `oauth2Token.Extra` to the `tokenResponse` map. This ensures that any fields returned by the OAuth token response (including email) are available for email extraction.

**File:** [`internal/provider/qwen/auth_device.go`](internal/provider/qwen/auth_device.go)

```go
// Line 105-110 - BEFORE
tokenResponse := make(map[string]interface{})
tokenResponse["access_token"] = oauth2Token.AccessToken
tokenResponse["refresh_token"] = oauth2Token.RefreshToken
tokenResponse["token_type"] = oauth2Token.TokenType
tokenResponse["expires_in"] = int64(oauth2Token.Expiry.Sub(time.Now()).Seconds())

// Line 105-110 - AFTER
tokenResponse := make(map[string]interface{})
tokenResponse["access_token"] = oauth2Token.AccessToken
tokenResponse["refresh_token"] = oauth2Token.RefreshToken
tokenResponse["token_type"] = oauth2Token.TokenType
tokenResponse["expires_in"] = int64(oauth2Token.Expiry.Sub(time.Now()).Seconds())

// Copy all extra fields from OAuth token response
for key, value := range oauth2Token.Extra {
    tokenResponse[key] = value
}
```

### Approach 2: Update User Info Endpoint (Fallback)

If the email is not in the token response, update the Qwen user info endpoint to use the correct endpoint for the device flow.

**File:** [`internal/token/constants.go`](internal/token/constants.go)

```go
// Line 22 - BEFORE
QwenUserInfoURL = "https://qwen-api.aliyuncs.com/v1/user/info"

// Line 22 - AFTER
QwenUserInfoURL = "https://chat.qwen.ai/api/v1/user/info"
```

**Note:** This approach requires verifying the correct user info endpoint for Qwen's OAuth device flow.

## Implementation Plan

### Step 1: Copy Extra Fields in Device Flow

**File:** [`internal/provider/qwen/auth_device.go`](internal/provider/qwen/auth_device.go)

Modify the `AuthenticateWithDeviceFlow` function to copy all extra fields from the OAuth token response.

### Step 2: Add Debug Logging

Add logging to see what fields are being received in the token response:

```go
// Log extra fields for debugging
logger.DebugLog("[Qwen OAuth] Extra fields in token response: %v", oauth2Token.Extra)
```

### Step 3: Update User Info Endpoint (if needed)

If email is still not found after Step 1, update the user info endpoint constant.

### Step 4: Test

Test the device flow to verify email extraction works correctly.

## Related Issues

This fix complements the fix described in [`docs/todo/01-fix-oauth-email-extraction.md`](docs/todo/01-fix-oauth-email-extraction.md), which addresses a similar issue in the REST API callback flow.

## Verification

After implementing the fix:

1. Run the device flow for Qwen
2. Check that the email is correctly extracted and saved
3. Verify in the database:
   ```bash
   sqlite3 .credentials/tokens.db "SELECT email FROM tokens WHERE provider_id='qwen' ORDER BY created_at DESC LIMIT 1"
   ```
4. Check logs for debug output showing extra fields

## Files to Modify

1. [`internal/provider/qwen/auth_device.go`](internal/provider/qwen/auth_device.go) - Copy extra fields from OAuth token response
2. [`internal/token/constants.go`](internal/token/constants.go) - Update user info endpoint (if needed)

## Risk Assessment

**Low Risk:**
- Only adds code to copy additional fields
- Does not remove or modify existing functionality
- Backward compatible

**Potential Issues:**
- If extra fields contain sensitive data, they will be logged (need to be careful with logging)
- If user info endpoint is still wrong, email extraction may still fail (but less likely with Approach 1)
