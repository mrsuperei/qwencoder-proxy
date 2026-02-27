# Gemini Per-Token Project ID Storage - Technical Specification

## Overview

This document describes the implementation of per-token project ID storage for the Gemini provider to fix the 403 PERMISSION_DENIED error that occurs when different Google accounts attempt to access a shared project ID.

## Problem Statement

### Current Issue
The [`Provider`](../provider/gemini/gemini.go:39) struct caches a single `projectID` field that is shared across all tokens. When the token manager rotates through different Google accounts/tokens, they all attempt to access the same project ID, but not all accounts have IAM permission to access each other's projects.

### Error Message
```
API error (status 403): {
  "error": {
    "code": 403,
    "message": "The caller does not have permission",
    "status": "PERMISSION_DENIED",
    "details": [
      {
        "@type": "type.googleapis.com/google.rpc.ErrorInfo",
        "reason": "IAM_PERMISSION_DENIED",
        "domain": "iam.googleapis.com",
        "metadata": {
          "permission": "cloudaicompanion.companions.generateChat",
          "resource": "projects/careful-braid-gsf6z"
        }
      }
    ]
  }
}
```

## Solution Design

### Architecture Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                     OAuth Flow                                │
│  1. User authenticates via OAuth                            │
│  2. Access token obtained                                   │
│  3. Email extracted from userinfo endpoint                   │
│  4. Project ID discovered via loadCodeAssist API            │
│  5. Token saved with project_id in database                 │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│              SQLite Database (tokens table)                    │
│  ┌─────────────────────────────────────────────────────────┐  │
│  │ id | email | access_token | project_id | ...          │  │
│  ├─────────────────────────────────────────────────────────┤  │
│  │ abc | user1@gmail.com | xxx... | project-1 | ...    │  │
│  │ def | user2@gmail.com | yyy... | project-2 | ...    │  │
│  └─────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│              Token Selection & API Calls                      │
│  1. TokenManager selects token based on strategy            │
│  2. Provider reads token's project_id from metadata        │
│  3. API call uses token-specific project_id                 │
│  4. No cross-account project access issues                 │
└─────────────────────────────────────────────────────────────────┘
```

## Implementation Plan

### Phase 1: Database Schema Migration

#### 1.1 Add `project_id` Column to Tokens Table

**File:** [`internal/token/sqlite_store.go`](../internal/token/sqlite_store.go)

**Changes:**

1. Update the `tokens` table schema constant to include `project_id`:

```go
const (
    sqlCreateTokensTable = `
        CREATE TABLE IF NOT EXISTS tokens (
            id TEXT PRIMARY KEY,
            provider_id TEXT NOT NULL,
            access_token TEXT NOT NULL,
            refresh_token TEXT,
            token_type TEXT NOT NULL DEFAULT 'Bearer',
            expiry_date INTEGER NOT NULL,
            email TEXT,
            resource_url TEXT,
            scope TEXT,
            api_key TEXT,
            project_id TEXT,                    // NEW COLUMN
            healthy INTEGER NOT NULL DEFAULT 1,
            health_score REAL NOT NULL DEFAULT 1.0,
            last_used INTEGER NOT NULL DEFAULT 0,
            created_at INTEGER NOT NULL DEFAULT (strftime('%s', 'subsec') * 1000),
            error_count INTEGER NOT NULL DEFAULT 0,
            last_error TEXT,
            proxy_id TEXT
        )
    `
)
```

2. Create migration V2 to add `project_id` column to existing databases:

```go
// In migrate() function, add migration V2:
migrations := []migration{
    {1, "Initial schema with all tables and indexes", func() error { return s.migrateToV1() }},
    {2, "Add project_id column for Gemini provider", func() error { return s.migrateToV2() }},
}
```

3. Implement `migrateToV2()` function:

```go
// migrateToV2 adds project_id column for Gemini provider
func (s *SQLiteStore) migrateToV2() error {
    // Add project_id column to tokens table
    if _, err := s.db.Exec(`
        ALTER TABLE tokens ADD COLUMN project_id TEXT
    `); err != nil {
        return fmt.Errorf("failed to add project_id column: %w", err)
    }

    // Create index for project_id (optional, for queries filtering by project)
    if _, err := s.db.Exec(`
        CREATE INDEX IF NOT EXISTS idx_tokens_project_id ON tokens(project_id)
    `); err != nil {
        return fmt.Errorf("failed to create project_id index: %w", err)
    }

    s.logger.InfoLog("[SQLiteStore] Migration to V2 completed: added project_id column")
    return nil
}
```

#### 1.2 Update Prepared Statements

**File:** [`internal/token/sqlite_store.go`](../internal/token/sqlite_store.go)

**Changes:**

Update all SQL statements that interact with the tokens table to include `project_id`:

```go
// Insert statement
const sqlInsertToken = `
    INSERT INTO tokens (
        id, provider_id, access_token, refresh_token, token_type,
        expiry_date, email, resource_url, scope, api_key, project_id,
        healthy, health_score, last_used, created_at, error_count, last_error, proxy_id
    ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`

// Update statement
const sqlUpdateToken = `
    UPDATE tokens SET
        access_token = ?, refresh_token = ?, token_type = ?,
        expiry_date = ?, email = ?, resource_url = ?, scope = ?,
        api_key = ?, project_id = ?, healthy = ?, health_score = ?,
        last_used = ?, error_count = ?, last_error = ?, proxy_id = ?
    WHERE id = ?
`

// Select by ID statement
const sqlSelectTokenByID = `
    SELECT id, provider_id, access_token, refresh_token, token_type,
           expiry_date, email, resource_url, scope, api_key, project_id,
           healthy, health_score, last_used, created_at, error_count, last_error, proxy_id
    FROM tokens WHERE id = ?
`
```

### Phase 2: Update Data Structures

#### 2.1 Update ProviderToken Struct

**File:** [`internal/token/store.go`](../internal/token/store.go)

**Changes:**

```go
// ProviderToken represents a single stored token with metadata
type ProviderToken struct {
    ID               string       `json:"id"`
    AccessToken      string       `json:"access_token"`
    RefreshToken     string       `json:"refresh_token"`
    TokenType        string       `json:"token_type"`
    ExpiryDate       int64        `json:"expiry_date"`
    Email            string       `json:"email"`
    ResourceURL      string       `json:"resource_url,omitempty"`
    Scope            string       `json:"scope,omitempty"`
    APIKey           string       `json:"api_key,omitempty"`
    ProjectID        string       `json:"project_id,omitempty"`  // NEW FIELD
    Healthy          bool         `json:"healthy"`
    HealthScore      float64      `json:"health_score"`
    LastUsed         int64        `json:"last_used"`
    CreatedAt        int64        `json:"created_at"`
    ErrorCount       int          `json:"error_count"`
    LastError        string       `json:"last_error,omitempty"`
    Proxy            *ProxyConfig `json:"proxy,omitempty"`
    ProxyHealthScore float64      `json:"proxy_health_score,omitempty"`
}
```

#### 2.2 Update SQLite Store Methods

**File:** [`internal/token/sqlite_store.go`](../internal/token/sqlite_store.go)

**Changes:**

Update the following methods to handle `project_id`:

1. `AddToken()` - Include `project_id` in INSERT statement
2. `UpdateToken()` - Include `project_id` in UPDATE statement
3. `GetToken()` - Include `project_id` in SELECT and scan
4. `ListTokens()` - Include `project_id` in SELECT and scan
5. `tokenFromRow()` - Update to scan `project_id` field

```go
// Example: tokenFromRow helper
func tokenFromRow(row *sql.Row) (*ProviderToken, error) {
    var token ProviderToken
    var proxyID sql.NullString
    var projectID sql.NullString

    err := row.Scan(
        &token.ID, &token.ProviderID, &token.AccessToken,
        &token.RefreshToken, &token.TokenType, &token.ExpiryDate,
        &token.Email, &token.ResourceURL, &token.Scope, &token.APIKey,
        &projectID,  // NEW
        &token.Healthy, &token.HealthScore, &token.LastUsed,
        &token.CreatedAt, &token.ErrorCount, &token.LastError,
        &proxyID,
    )

    if projectID.Valid {
        token.ProjectID = projectID.String
    }

    // ... rest of the function
}
```

### Phase 3: OAuth Flow Integration

#### 3.1 Add Project ID Discovery to OAuth Flow

**File:** [`provider/gemini/auth.go`](../provider/gemini/auth.go)

**Changes:**

Modify `exchangeCodeForTokens()` to discover and store the project ID:

```go
// exchangeCodeForTokens exchanges authorization code for tokens
func (a *Authenticator) exchangeCodeForTokens(ctx context.Context, code, redirectURI string) error {
    // ... existing token exchange code ...

    // Check if multi-token manager is available
    multiTokenMgr := a.GetMultiTokenManager()
    if multiTokenMgr == nil {
        // Fallback to legacy saving
        a.GetLogger().DebugLog("[Gemini Auth] Multi-token manager not available, using legacy save")
        return nil
    }

    // Extract email from token response
    email, err := multiTokenMgr.ExtractEmail(ctx, "gemini", tokenResponse, tokenResp.AccessToken)
    if err != nil {
        a.GetLogger().WarnLog("[Gemini Auth] Failed to extract email: %v", err)
        email = ""
    }

    // NEW: Discover project ID for this account
    projectID, err := a.discoverProjectID(ctx, tokenResp.AccessToken)
    if err != nil {
        a.GetLogger().WarnLog("[Gemini Auth] Failed to discover project ID: %v", err)
        projectID = ""  // Empty string means project ID not yet discovered
    } else {
        a.GetLogger().InfoLog("[Gemini Auth] Discovered project ID: %s for email: %s", projectID, email)
    }

    // Create ProviderToken with email and project ID
    now := time.Now()
    expiry := now.Add(time.Duration(tokenResp.ExpiresIn) * time.Second)

    providerToken := tokenpkg.ProviderToken{
        ID:           uuid.New().String(),
        AccessToken:  tokenResp.AccessToken,
        RefreshToken: tokenResp.RefreshToken,
        TokenType:    tokenResp.TokenType,
        ExpiryDate:   expiry.UnixMilli(),
        Email:        email,
        Scope:        tokenResp.Scope,
        ProjectID:    projectID,  // NEW: Store project ID
        Healthy:      true,
        HealthScore:  1.0,
        LastUsed:     now.UnixMilli(),
        CreatedAt:    now.UnixMilli(),
    }

    // Save token to multi-token store
    if err := multiTokenMgr.SaveToken("gemini", providerToken); err != nil {
        return fmt.Errorf("failed to save token to multi-token store: %w", err)
    }

    a.GetLogger().DebugLog("[Gemini Auth] Authentication successful, credentials saved to multi-token store")
    return nil
}
```

#### 3.2 Add discoverProjectID Method

**File:** [`provider/gemini/auth.go`](../provider/gemini/auth.go)

**New Method:**

```go
// discoverProjectID discovers the Cloud Code Assist project ID for the given access token
func (a *Authenticator) discoverProjectID(ctx context.Context, accessToken string) (string, error) {
    const baseURL = "https://cloudcode-pa.googleapis.com/v1internal"

    // Prepare client metadata
    clientMetadata := map[string]interface{}{
        "ideType":     "IDE_UNSPECIFIED",
        "platform":    "PLATFORM_UNSPECIFIED",
        "pluginType":  "GEMINI",
        "duetProject": "",
    }

    // Prepare loadCodeAssist request
    loadRequest := map[string]interface{}{
        "cloudaicompanionProject": "",
        "metadata":                clientMetadata,
    }

    reqBody, err := json.Marshal(loadRequest)
    if err != nil {
        return "", fmt.Errorf("failed to marshal load request: %w", err)
    }

    // Call loadCodeAssist endpoint
    url := fmt.Sprintf("%s:loadCodeAssist", baseURL)
    req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(reqBody))
    if err != nil {
        return "", fmt.Errorf("failed to create load request: %w", err)
    }

    req.Header.Set("Authorization", "Bearer "+accessToken)
    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("User-Agent", "google-api-nodejs-client/9.15.1")
    req.Header.Set("X-Goog-Api-Client", "gl-node/22.17.0")
    req.Header.Set("Client-Metadata", "ideType=IDE_UNSPECIFIED,platform=PLATFORM_UNSPECIFIED,pluginType=GEMINI")

    client := a.httpClient
    if client == nil {
        client = &http.Client{Timeout: 30 * time.Second}
    }

    resp, err := client.Do(req)
    if err != nil {
        return "", fmt.Errorf("failed to send load request: %w", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        body, _ := io.ReadAll(resp.Body)
        return "", fmt.Errorf("loadCodeAssist failed (status %d): %s", resp.StatusCode, string(body))
    }

    respBody, err := io.ReadAll(resp.Body)
    if err != nil {
        return "", fmt.Errorf("failed to read load response: %w", err)
    }

    var loadResponse map[string]interface{}
    if err := json.Unmarshal(respBody, &loadResponse); err != nil {
        return "", fmt.Errorf("failed to decode load response: %w", err)
    }

    // Check if project ID exists in response
    if projectID, ok := loadResponse["cloudaicompanionProject"].(string); ok && projectID != "" {
        return projectID, nil
    }

    // If no existing project, try to onboard
    allowedTiers, ok := loadResponse["allowedTiers"].([]interface{})
    var tierID string
    if ok && len(allowedTiers) > 0 {
        for _, tier := range allowedTiers {
            if tierMap, ok := tier.(map[string]interface{}); ok {
                if isDefault, exists := tierMap["isDefault"].(bool); exists && isDefault {
                    if id, idExists := tierMap["id"].(string); idExists {
                        tierID = id
                        break
                    }
                }
            }
        }
    }

    if tierID == "" {
        tierID = "free-tier"
    }

    // Prepare onboardUser request
    onboardRequest := map[string]interface{}{
        "tierId":                  tierID,
        "cloudaicompanionProject": "",
        "metadata":                clientMetadata,
    }

    onboardReqBody, err := json.Marshal(onboardRequest)
    if err != nil {
        return "", fmt.Errorf("failed to marshal onboard request: %w", err)
    }

    onboardUrl := fmt.Sprintf("%s:onboardUser", baseURL)
    onboardReq, err := http.NewRequestWithContext(ctx, "POST", onboardUrl, bytes.NewReader(onboardReqBody))
    if err != nil {
        return "", fmt.Errorf("failed to create onboard request: %w", err)
    }

    onboardReq.Header.Set("Authorization", "Bearer "+accessToken)
    onboardReq.Header.Set("Content-Type", "application/json")
    onboardReq.Header.Set("User-Agent", "qwencoder-proxy/1.0")
    onboardReq.Header.Set("X-Goog-Api-Client", "gl-node/22.17.0")
    onboardReq.Header.Set("Client-Metadata", "ideType=IDE_UNSPECIFIED,platform=PLATFORM_UNSPECIFIED,pluginType=GEMINI")

    onboardResp, err := client.Do(onboardReq)
    if err != nil {
        return "", fmt.Errorf("failed to send onboard request: %w", err)
    }
    defer onboardResp.Body.Close()

    if onboardResp.StatusCode != http.StatusOK {
        body, _ := io.ReadAll(onboardResp.Body)
        return "", fmt.Errorf("onboardUser failed (status %d): %s", onboardResp.StatusCode, string(body))
    }

    onboardRespBody, err := io.ReadAll(onboardResp.Body)
    if err != nil {
        return "", fmt.Errorf("failed to read onboard response: %w", err)
    }

    var onboardResponse map[string]interface{}
    if err := json.Unmarshal(onboardRespBody, &onboardResponse); err != nil {
        return "", fmt.Errorf("failed to decode onboard response: %w", err)
    }

    // Extract project ID from onboard response
    if response, ok := onboardResponse["response"].(map[string]interface{}); ok {
        if project, exists := response["cloudaicompanionProject"].(map[string]interface{}); exists {
            if id, idExists := project["id"].(string); idExists {
                return id, nil
            }
        }
    }

    // Fallback: try to get project ID directly from response
    if id, exists := onboardResponse["cloudaicompanionProject"].(string); exists {
        return id, nil
    }

    return "", fmt.Errorf("failed to discover or create project ID")
}
```

### Phase 4: Provider Implementation Changes

#### 4.1 Modify Provider to Use Token's Project ID

**File:** [`provider/gemini/gemini.go`](../provider/gemini/gemini.go)

**Changes:**

1. Remove the shared `projectID` field from Provider struct (or keep for backward compatibility):

```go
type Provider struct {
    *provider.BaseProvider
    baseURL                string
    authenticator          *Authenticator
    // projectID              string  // DEPRECATED: Use token.ProjectID instead
    projectInitError       error
}
```

2. Modify `GenerateContent()` to use token's project ID:

```go
func (p *Provider) GenerateContent(ctx context.Context, model string, request interface{}) (interface{}, error) {
    // Get token and proxy-aware client
    var client *http.Client
    var token string
    var tokenID string
    var projectID string  // NEW: Get project ID from token

    // Try to use token manager for proxy-aware client selection
    if p.GetTokenManager() != nil {
        selectedToken, selectedClient, selectErr := p.GetTokenManager().SelectTokenWithClient()
        if selectErr != nil {
            p.GetLogger().ErrorLog("[Gemini] Token selection failed: %v", selectErr)
            return nil, fmt.Errorf("failed to select token: %w", selectErr)
        }
        if selectedClient == nil {
            client = p.GetHTTPClient()
        } else {
            client = selectedClient
        }
        token = selectedToken.AccessToken
        tokenID = selectedToken.ID
        projectID = selectedToken.ProjectID  // NEW: Use token's project ID

        p.GetLogger().DebugLog("[Gemini] Selected token: ID=%s, Email=%s, ProjectID=%s", tokenID, selectedToken.Email, projectID)
    } else {
        // Fallback for legacy code path
        var authErr error
        token, authErr = p.authenticator.GetToken(ctx)
        if authErr != nil {
            return nil, fmt.Errorf("failed to get token: %w", authErr)
        }
        client = p.GetHTTPClient()
        tokenID = "fallback"
        projectID = ""  // Will trigger lazy initialization
    }

    // NEW: Lazy project ID discovery for tokens without project ID
    if projectID == "" {
        p.GetLogger().DebugLog("[Gemini] Token %s has no project ID, discovering...", tokenID)
        discoveredID, err := p.discoverProjectIDForToken(ctx, tokenID, token)
        if err != nil {
            return nil, fmt.Errorf("failed to discover project ID for token %s: %w", tokenID, err)
        }
        projectID = discoveredID
        p.GetLogger().DebugLog("[Gemini] Discovered project ID %s for token %s", projectID, tokenID)
    }

    // ... rest of the function, using projectID instead of p.projectID ...
    finalRequest := map[string]interface{}{
        "model":   model,
        "project": projectID,  // Use token-specific project ID
        "request": requestMap,
    }
    // ...
}
```

3. Add helper method to discover and update project ID for a token:

```go
// discoverProjectIDForToken discovers the project ID for a token and updates it in the store
func (p *Provider) discoverProjectIDForToken(ctx context.Context, tokenID, accessToken string) (string, error) {
    // Reuse the discoverProjectID method from authenticator
    projectID, err := p.authenticator.discoverProjectID(ctx, accessToken)
    if err != nil {
        return "", err
    }

    // Update the token's project ID in the store
    if p.GetTokenManager() != nil {
        if updateErr := p.GetTokenManager().UpdateToken(tokenID, func(t *tokenpkg.TokenMetadata) {
            t.ProjectID = projectID
        }); updateErr != nil {
            p.GetLogger().WarnLog("[Gemini] Failed to update project ID for token %s: %v", tokenID, updateErr)
        }
    }

    return projectID, nil
}
```

4. Similarly update `GenerateContentStream()` to use token's project ID

5. Remove or deprecate `initializeProject()` method as it's no longer needed

### Phase 5: Token Refresh Handling

#### 5.1 Preserve Project ID on Token Refresh

**File:** [`provider/gemini/auth.go`](../provider/gemini/auth.go)

**Changes:**

Update `geminiTokenRefresher.RefreshToken()` to preserve the project ID:

```go
func (g *geminiTokenRefresher) RefreshToken(ctx context.Context, token tokenpkg.ProviderToken) (tokenpkg.ProviderToken, error) {
    // ... existing refresh logic ...

    // Return refreshed token, preserving all metadata including ProjectID
    return tokenpkg.ProviderToken{
        ID:           token.ID,
        AccessToken:  tokenResp.AccessToken,
        RefreshToken: tokenResp.RefreshToken,
        TokenType:    tokenResp.TokenType,
        ExpiryDate:   expiryDate.UnixMilli(),
        Email:        token.Email,
        Scope:        token.Scope,
        ProjectID:    token.ProjectID,  // PRESERVE project ID on refresh
        Healthy:      true,
        HealthScore:  1.0,
        LastUsed:     token.LastUsed,
        CreatedAt:    token.CreatedAt,
    }, nil
}
```

### Phase 6: Testing Strategy

#### 6.1 Unit Tests

**Test Files:**
- [`internal/token/sqlite_store_crud_test.go`](../internal/token/sqlite_store_crud_test.go) - Update to test `project_id` field
- [`provider/gemini/auth_test.go`](../provider/gemini/auth_test.go) - Test project ID discovery
- [`provider/gemini/gemini_test.go`](../provider/gemini/gemini_test.go) - Test per-token project ID usage

**Test Cases:**
1. Verify `project_id` is correctly stored and retrieved
2. Verify project ID is discovered during OAuth flow
3. Verify project ID is preserved during token refresh
4. Verify tokens with different project IDs don't interfere with each other
5. Verify migration V2 correctly adds `project_id` column

#### 6.2 Integration Tests

**Test Scenarios:**
1. OAuth flow with multiple accounts - verify each gets unique project ID
2. API calls with token rotation - verify correct project ID is used for each token
3. Existing tokens without project ID - verify lazy discovery works
4. Token refresh - verify project ID is preserved

### Phase 7: Backward Compatibility

#### 7.1 Handle Existing Tokens

For existing tokens that don't have a `project_id` value:

1. **Lazy Discovery**: When a token without `project_id` is selected, discover it on-demand and update the token metadata
2. **Migration Script**: Optionally provide a script to discover project IDs for all existing tokens

#### 7.2 Deprecation Path

1. Keep `p.projectID` field for now but mark as deprecated
2. Add logging when using deprecated field path
3. Remove in future version after all tokens have project IDs

## Implementation Order

1. **Phase 1**: Database schema migration (V2)
2. **Phase 2**: Update data structures (`ProviderToken`, SQLite store)
3. **Phase 3**: OAuth flow integration (project ID discovery)
4. **Phase 4**: Provider implementation changes
5. **Phase 5**: Token refresh handling
6. **Phase 6**: Testing
7. **Phase 7**: Backward compatibility handling

## Files to Modify

1. [`internal/token/store.go`](../internal/token/store.go) - Add `ProjectID` to `ProviderToken`
2. [`internal/token/sqlite_store.go`](../internal/token/sqlite_store.go) - Schema migration V2, update CRUD operations
3. [`provider/gemini/auth.go`](../provider/gemini/auth.go) - Add `discoverProjectID()`, update OAuth flow
4. [`provider/gemini/gemini.go`](../provider/gemini/gemini.go) - Use token's project ID, add lazy discovery

## New Files to Create

1. [`provider/gemini/auth_test.go`](../provider/gemini/auth_test.go) - Tests for project ID discovery
2. [`internal/token/migration_v2_test.go`](../internal/token/migration_v2_test.go) - Tests for migration V2

## Success Criteria

1. ✅ Each Gemini token has its own `project_id` stored in the database
2. ✅ Project ID is discovered during OAuth flow
3. ✅ API calls use the token's specific `project_id`
4. ✅ 403 PERMISSION_DENIED errors are eliminated
5. ✅ Existing tokens without `project_id` are handled gracefully
6. ✅ Token refresh preserves `project_id`
7. ✅ All tests pass

## Rollback Plan

If issues arise:

1. Revert to using `p.projectID` with warning logs
2. Disable lazy project ID discovery
3. Provide option to manually configure project ID per token via dashboard

## Future Enhancements

1. **Project ID Validation**: Add validation to ensure project ID is accessible
2. **Project ID Rotation**: Support for changing project IDs per token
3. **Dashboard UI**: Display project ID for each token in the dashboard
4. **Health Checks**: Verify project ID accessibility during health checks
