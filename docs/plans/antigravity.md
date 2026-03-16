Plan: Enable Full OAuth Support for Antigravity Provider
Overview
The Antigravity provider has OAuth authentication implemented but is not registered in the OAuth provider registry, which prevents it from appearing as an OAuth option in the dashboard. This plan outlines the steps to make Antigravity fully functional with OAuth support.

Current State Analysis
What's Already Implemented
✅ OAuth authentication implementation in provider/antigravity/auth.go
✅ Provider implementation in provider/antigravity/antigravity.go
✅ Route registration at /antigravity/v1/ in proxy/openai_handler.go:295
✅ Default OAuth configuration with Google credentials
What's Missing
❌ Antigravity not registered in OAuth provider registry
❌ Documentation incorrectly states "No OAuth2 Support"
❌ No OAuth flow exposed through REST API endpoints
❌ No tests for Antigravity OAuth integration
Phase 1: Register Antigravity in OAuth Provider Registry
Step 1.1: Add Antigravity to Provider Registry
File: restapi/provider_registry.go

Action: Add the following provider registration after line 102 (after Kiro registration):

// Antigravity - Authorization Code Flow
pr.RegisterProvider(&ProviderConfig{
    ID:           "antigravity",
    Name:         "Antigravity (Google)",
    Flow:         "authorization_code",
    ClientID:     "1071006060591-tmhssin2h21lcre235vtolojh4g403ep.apps.googleusercontent.com",
    ClientSecret: "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",
    AuthURL:      "https://accounts.google.com/o/oauth2/v2/auth",
    TokenURL:     "https://oauth2.googleapis.com/token",
    Scopes:       []string{"https://www.googleapis.com/auth/cloud-platform"},
    CredsDir:     ".antigravity",
    CredsFile:    "oauth_creds.json",
})

Verification: After this change, Antigravity should appear in the list returned by /api/providers endpoint.

Step 1.2: Test Provider Registration
Test Command:

curl http://localhost:8080/api/providers

Expected Result: Should include Antigravity in the providers array:

{
  "providers": [
    {
      "id": "qwen",
      "name": "Qwen",
      "flow": "device_code",
      ...
    },
    {
      "id": "antigravity",
      "name": "Antigravity (Google)",
      "flow": "authorization_code",
      "auth_url": "https://accounts.google.com/o/oauth2/v2/auth",
      "token_url": "https://oauth2.googleapis.com/token",
      "scopes": ["https://www.googleapis.com/auth/cloud-platform"]
    },
    ...
  ]
}

Phase 2: Update Documentation
Step 2.1: Correct Authentication Table
File: docs/authentication.md

Current (Incorrect):

| Provider | Authentication Method | OAuth2 Support |
|----------|----------------------|----------------|
| Antigravity | API Key | No |

Change To:

| Provider | Authentication Method | OAuth2 Support |
|----------|----------------------|----------------|
| Antigravity | OAuth2 / API Key | Yes |

Step 2.2: Add Antigravity OAuth Authentication Section
File: docs/authentication.md

Add after iFlow Authentication section:

### Antigravity Authentication

Antigravity uses OAuth2 Authorization Code Flow for authentication (similar to Gemini CLI).

#### Method 1: Web Dashboard (Recommended)

1. Open the web dashboard at `http://localhost:8080`
2. Navigate to the Antigravity provider section
3. Click "Authenticate" to start the OAuth2 flow
4. Follow the Google OAuth2 authorization prompts
5. Complete authentication and return to dashboard

#### Method 2: Manual OAuth2 Flow

```bash
# Start the authorization flow
curl -X POST http://localhost:8080/api/auth/start \
  -H "Content-Type: application/json" \
  -d '{
    "provider": "antigravity"
  }'

# Response:
{
  "auth_url": "https://accounts.google.com/o/oauth2/v2/auth?...",
  "state": "random_state_value"
}

# Visit the auth_url in your browser and authorize
# After authorization, the callback will handle token exchange automatically

Method 3: Direct API Key (Alternative)
If you have a Google Cloud access token, you can set it directly:

curl -X POST http://localhost:8080/api/antigravity/credentials \
  -H "Content-Type: application/json" \
  -d '{
    "access_token": "your-google-cloud-access-token"
  }'


### Step 2.3: Update API Reference

**File**: [`docs/api-reference.md`](docs/api-reference.md)

**Add Antigravity to OAuth providers section** if it exists.

---

## Phase 3: Verify Integration

### Step 3.1: Test OAuth Flow Through Dashboard

**Prerequisites**:
- Server running on port 8080
- Dashboard accessible at `http://localhost:8080`

**Steps**:
1. Open dashboard in browser
2. Navigate to Antigravity provider tab
3. Click "Authenticate" button
4. Verify Google OAuth2 authorization page opens
5. Complete authorization
6. Verify redirect back to dashboard
7. Check that token is saved and displayed

**Expected Result**: 
- OAuth flow completes successfully
- Token is stored in `~/.antigravity/oauth_creds.json`
- Dashboard shows authenticated status with email

### Step 3.2: Test Token Storage and Refresh

**Test Token Storage**:
```bash
# Check credentials file exists
cat ~/.antigravity/oauth_creds.json


Expected Content:

{
  "access_token": "ya29.a0AfH6SMB...",
  "refresh_token": "1//0g...",
  "token_type": "Bearer",
  "expiry_date": 1234567890,
  "scope": "https://www.googleapis.com/auth/cloud-platform"
}

Test Token Refresh:

curl -X POST http://localhost:8080/api/token/antigravity/refresh

Expected Result: New access token returned

Step 3.3: Test API Endpoints with OAuth Token
Test Model Listing:

curl http://localhost:8143/antigravity/v1/models \
  -H "Authorization: Bearer YOUR_ACCESS_TOKEN"

Expected Result: List of Antigravity models returned

Test Chat Completion:

curl -X POST http://localhost:8143/antigravity/v1/chat/completions \
  -H "Authorization: Bearer YOUR_ACCESS_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gemini-3-flash",
    "messages": [{"role": "user", "content": "Hello!"}]
  }'

Expected Result: Successful chat completion response

Step 3.4: Test Multi-Token Support
Steps:

Authenticate with multiple Google accounts
Verify both tokens appear in dashboard
Test switching between tokens
Verify token selection works for API requests
Expected Result: Multiple tokens managed correctly

Phase 4: Additional Enhancements
Step 4.1: Add Comprehensive Tests
File: Create restapi/antigravity_oauth_test.go

Test Cases to Implement:

Test Antigravity provider registration
Test OAuth flow initiation
Test token storage
Test token refresh
Test token selection
Test credential retrieval
Test OAuth callback handling
Example Test Structure:

func TestAntigravityOAuthFlow(t *testing.T) {
    // Test that Antigravity is registered
    registry := NewProviderRegistry()
    config, err := registry.GetConfig("antigravity")
    assert.NoError(t, err)
    assert.Equal(t, "antigravity", config.ID)
    assert.Equal(t, "authorization_code", config.Flow)
    
    // Test OAuth flow
    // Test token storage
    // Test token refresh
}

Step 4.2: Add Curl Examples
File: docs/curl-examples.md

Add Antigravity OAuth Examples:

## Antigravity OAuth Examples

### Start OAuth Flow
```bash
curl -X POST http://localhost:8080/api/auth/start \
  -H "Content-Type: application/json" \
  -d '{"provider": "antigravity"}'

Get Token
curl http://localhost:8080/api/token/antigravity

Refresh Token
curl -X POST http://localhost:8080/api/token/antigravity/refresh

List Credentials
curl http://localhost:8080/api/credentials/antigravity


### Step 4.3: Update README

**File**: [`README.md`](README.md)

**Add Antigravity OAuth to authentication section** if it exists.

---

## Phase 5: Final Verification

### Step 5.1: End-to-End Testing

**Complete Workflow**:
1. Start server: `./qwencoder-proxy`
2. Open dashboard: `http://localhost:8080`
3. Authenticate with Antigravity
4. Test model listing: `curl http://localhost:8143/antigravity/v1/models`
5. Test chat completion with authenticated token
6. Verify token refresh works
7. Test multiple accounts
8. Verify all documentation is accurate

### Step 5.2: Regression Testing

**Test Other Providers**:
- Verify Qwen OAuth still works
- Verify Gemini OAuth still works
- Verify iFlow OAuth still works
- Verify Kiro external auth still works

**Expected Result**: No regressions in existing provider OAuth flows