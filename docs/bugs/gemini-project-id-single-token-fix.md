# Gemini Project ID Per-Token Fix - Implementation Plan

## Overview

This implementation plan fixes the issue where only one project_id is saved for multiple Gemini tokens. Each token should get its own project_id from the Gemini OAuth flow.

## Prerequisites

- Read the analysis document: [`bugs/gemini-project-id-single-token-issue-analysis.md`](bugs/gemini-project-id-single-token-issue-analysis.md)
- Understand the Go codebase structure
- Have access to modify files in the repository

## Implementation Steps

### Step 1: Fix Database Schema

**File**: `internal/token/sqlite_store.go`

**Action**: Update the `sqlCreateTokensTable` constant to include `project_id` column.

**Current Code** (lines 30-50):
```go
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
        healthy INTEGER NOT NULL DEFAULT 1,
        health_score REAL NOT NULL DEFAULT 1.0,
        last_used INTEGER NOT NULL DEFAULT 0,
        created_at INTEGER NOT NULL DEFAULT (strftime('%s', 'subsec') * 1000),
        error_count INTEGER NOT NULL DEFAULT 0,
        last_error TEXT,
        proxy_id TEXT
    )
`
```

**Fixed Code**:
```go
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
        project_id TEXT,
        healthy INTEGER NOT NULL DEFAULT 1,
        health_score REAL NOT NULL DEFAULT 1.0,
        last_used INTEGER NOT NULL DEFAULT 0,
        created_at INTEGER NOT NULL DEFAULT (strftime('%s', 'subsec') * 1000),
        error_count INTEGER NOT NULL DEFAULT 0,
        last_error TEXT,
        proxy_id TEXT
    )
`
```

**Change**: Add `project_id TEXT,` after `api_key TEXT,` (before line 42).

---

### Step 2: Remove Shared Project ID Field

**File**: `provider/gemini/gemini.go`

**Action**: Remove the shared `projectID` field from the `Provider` struct.

**Current Code** (lines 39-45):
```go
type Provider struct {
    *provider.BaseProvider // Embedded base provider provides common fields and methods
    baseURL                string
    authenticator          *Authenticator
    projectID              string
    projectInitError       error // Store any initialization error to prevent repeated attempts
}
```

**Fixed Code**:
```go
type Provider struct {
    *provider.BaseProvider // Embedded base provider provides common fields and methods
    baseURL                string
    authenticator          *Authenticator
    projectInitError       error // Store any initialization error to prevent repeated attempts
}
```

**Change**: Remove the `projectID string` field from the struct.

---

### Step 3: Remove initializeProject() Method

**File**: `provider/gemini/gemini.go`

**Action**: Remove or deprecate the `initializeProject()` method (lines 311-519) since it sets the shared `projectID` field.

**Action**: Remove any calls to `initializeProject()` from the codebase.

**Note**: The project ID is now discovered per-token during OAuth flow in `exchangeCodeForTokens()` and lazily during API calls in `discoverProjectIDForToken()`.

---

### Step 4: Verify OAuth Flow Correctly Saves Project ID

**File**: `provider/gemini/auth.go`

**Action**: Verify that the `exchangeCodeForTokens()` method correctly discovers and saves project_id per token.

**Current Code** (lines 382-409) - This is already correct:
```go
// Discover project ID for this account
projectID, err := a.discoverProjectID(ctx, tokenResp.AccessToken)
if err != nil {
    a.GetLogger().WarnLog("[Gemini Auth] Failed to discover project ID: %v", err)
    projectID = "" // Empty string means project ID not yet discovered
} else {
    a.GetLogger().InfoLog("[Gemini Auth] Discovered project ID: %s for email: %s", projectID, email)
}

providerToken := tokenpkg.ProviderToken{
    ID:           uuid.New().String(),
    AccessToken:  tokenResp.AccessToken,
    RefreshToken: tokenResp.RefreshToken,
    TokenType:    tokenResp.TokenType,
    ExpiryDate:   expiry.UnixMilli(),
    Email:        email,
    Scope:        tokenResp.Scope,
    ProjectID:    projectID, // Store discovered project ID
    Healthy:      true,
    HealthScore:  1.0,
    LastUsed:     now.UnixMilli(),
    CreatedAt:    now.UnixMilli(),
}

// Save token to multi-token store
if err := multiTokenMgr.SaveToken("gemini", providerToken); err != nil {
    return fmt.Errorf("failed to save token to multi-token store: %w", err)
}
```

**Action**: No changes needed - this code is already correct.

---

### Step 5: Verify Lazy Project ID Discovery

**File**: `provider/gemini/gemini.go`

**Action**: Verify that `discoverProjectIDForToken()` correctly updates the token's project_id in the store.

**Current Code** (lines 521-539) - This is already correct:
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

**Action**: No changes needed - this code is already correct.

---

### Step 6: Verify GenerateContent Uses Token-Specific Project ID

**File**: `provider/gemini/gemini.go`

**Action**: Verify that `GenerateContent()` uses `selectedToken.ProjectID` instead of `p.projectID`.

**Current Code** (lines 542-597) - This is already correct:
```go
func (p *Provider) GenerateContent(ctx context.Context, model string, request interface{}) (interface{}, error) {
    // Get token and proxy-aware client
    var client *http.Client
    var token string
    var tokenID string
    var projectID string // Use token-specific project ID

    // Try to use token manager for proxy-aware client selection
    if p.GetTokenManager() != nil {
        selectedToken, selectedClient, selectErr := p.GetTokenManager().SelectTokenWithClient()
        if selectErr != nil {
            p.GetLogger().ErrorLog("[Gemini] Token selection failed: %v", selectErr)
            return nil, fmt.Errorf("failed to select token: %w", selectErr)
        }
        if selectedClient == nil {
            p.GetLogger().DebugLog("[Gemini] Token manager returned nil client, using default HTTP client")
            client = p.GetHTTPClient()
        } else {
            client = selectedClient
        }
        token = selectedToken.AccessToken
        tokenID = selectedToken.ID
        projectID = selectedToken.ProjectID // Use token's project ID

        // DEBUG: Log token details
        p.GetLogger().DebugLog("[Gemini] Selected token: ID=%s, Email=%s, ProjectID=%s", tokenID, selectedToken.Email, projectID)

        // Log proxy usage
        if selectedToken.Proxy != nil && selectedToken.Proxy.Type != "none" {
            p.GetLogger().DebugLog("[Gemini] GenerateContent using token %s with proxy: %s:%d", tokenID, selectedToken.Proxy.Host, selectedToken.Proxy.Port)
        } else {
            p.GetLogger().DebugLog("[Gemini] GenerateContent using token %s with direct connection", tokenID)
        }

        // Lazy project ID discovery for tokens without project ID
        if projectID == "" {
            p.GetLogger().DebugLog("[Gemini] Token %s has no project ID, discovering...", tokenID)
            discoveredID, err := p.discoverProjectIDForToken(ctx, tokenID, token)
            if err != nil {
                return nil, fmt.Errorf("failed to discover project ID for token %s: %w", tokenID, err)
            }
            projectID = discoveredID
            p.GetLogger().DebugLog("[Gemini] Discovered project ID %s for token %s", projectID, tokenID)
        }
    } else {
        // No token manager, use authenticator and default client
        var authErr error
        token, authErr = p.authenticator.GetToken(ctx)
        if authErr != nil {
            p.GetLogger().ErrorLog("[Gemini] Token retrieval failed: %v", authErr)
            return nil, fmt.Errorf("failed to get token: %w", authErr)
        }
        client = p.GetHTTPClient()
        tokenID = "fallback"
        projectID = "" // Will trigger lazy initialization
    }

    // ... rest of the method uses projectID variable
}
```

**Action**: No changes needed - this code is already correct.

---

### Step 7: Verify GenerateContentStream Uses Token-Specific Project ID

**File**: `provider/gemini/gemini.go`

**Action**: Verify that `GenerateContentStream()` uses `selectedToken.ProjectID` instead of `p.projectID`.

**Current Code** (lines 743-796) - This is already correct:
```go
func (p *Provider) GenerateContentStream(ctx context.Context, model string, request interface{}) (io.ReadCloser, error) {
    // Get token and proxy-aware client
    var client *http.Client
    var token string
    var tokenID string
    var projectID string // Use token-specific project ID

    // Try to use token manager for proxy-aware client selection
    if p.GetTokenManager() != nil {
        selectedToken, selectedClient, selectErr := p.GetTokenManager().SelectTokenWithClient()
        if selectErr != nil {
            p.GetLogger().ErrorLog("[Gemini] Token selection failed: %v", selectErr)
            return nil, fmt.Errorf("failed to select token: %w", selectErr)
        }
        if selectedClient == nil {
            p.GetLogger().DebugLog("[Gemini] Token manager returned nil client, using default HTTP client")
            client = p.GetHTTPClient()
        } else {
            client = selectedClient
        }
        token = selectedToken.AccessToken
        tokenID = selectedToken.ID
        projectID = selectedToken.ProjectID // Use token's project ID

        // Log proxy usage
        if selectedToken.Proxy != nil && selectedToken.Proxy.Type != "none" {
            p.GetLogger().DebugLog("[Gemini] GenerateContentStream using token %s with proxy: %s:%d", tokenID, selectedToken.Proxy.Host, selectedToken.Proxy.Port)
        } else {
            p.GetLogger().DebugLog("[Gemini] GenerateContentStream using token %s with direct connection", tokenID)
        }

        // Lazy project ID discovery for tokens without project ID
        if projectID == "" {
            p.GetLogger().DebugLog("[Gemini] Token %s has no project ID, discovering...", tokenID)
            discoveredID, err := p.discoverProjectIDForToken(ctx, tokenID, token)
            if err != nil {
                return nil, fmt.Errorf("failed to discover project ID for token %s: %w", tokenID, err)
            }
            projectID = discoveredID
            p.GetLogger().DebugLog("[Gemini] Discovered project ID %s for token %s", projectID, tokenID)
        }
    } else {
        // No token manager, use authenticator and default client
        var authErr error
        token, authErr = p.authenticator.GetToken(ctx)
        if authErr != nil {
            p.GetLogger().ErrorLog("[Gemini] Token retrieval failed: %v", authErr)
            return nil, fmt.Errorf("failed to get token: %w", authErr)
        }
        client = p.GetHTTPClient()
        tokenID = "fallback"
        projectID = "" // Will trigger lazy initialization
    }

    // ... rest of the method uses projectID variable
}
```

**Action**: No changes needed - this code is already correct.

---

### Step 8: Search for References to p.projectID

**File**: `provider/gemini/gemini.go`

**Action**: Search for all references to `p.projectID` in the file and replace them with token-specific project ID.

**Search Pattern**: `p\.projectID`

**Expected Locations**:
- Line 429: `p.projectID = projectID` - Remove this line (inside `initializeProject()`)
- Line 504: `p.projectID = id` - Remove this line (inside `initializeProject()`)
- Line 513: `p.projectID = id` - Remove this line (inside `initializeProject()`)

**Action**: Since we're removing the `initializeProject()` method, these lines will be removed automatically.

---

### Step 9: Search for Calls to initializeProject()

**File**: All files in the project

**Action**: Search for all calls to `initializeProject()` and remove them.

**Search Pattern**: `initializeProject`

**Expected Locations**:
- `provider/gemini/gemini.go` - Any calls within the file
- Other files that might call this method

**Action**: Remove all calls to `initializeProject()`.

---

### Step 10: Add Database Migration for Existing Databases

**File**: `internal/token/sqlite_store.go`

**Action**: Ensure the migration to V2 correctly adds the `project_id` column to existing databases.

**Current Code** (lines 459-475) - This is already correct:
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

**Action**: No changes needed - this code is already correct.

---

### Step 11: Add Validation for project_id Column

**File**: `internal/token/sqlite_store.go`

**Action**: Add a check to verify that the `project_id` column exists before attempting to save tokens.

**Location**: In the `Save()` method, before the transaction starts.

**Add Code**:
```go
// Validate that project_id column exists
var columns []string
rows, err := s.db.Query("PRAGMA table_info(tokens)")
if err != nil {
    return fmt.Errorf("failed to query table info: %w", err)
}
defer rows.Close()

for rows.Next() {
    var cid int
    var name string
    var ctype string
    var notnull int
    var dfltValue interface{}
    var pk int
    if err := rows.Scan(&cid, &name, &ctype, &notnull, &dfltValue, &pk); err != nil {
        return fmt.Errorf("failed to scan column info: %w", err)
    }
    columns = append(columns, name)
}

hasProjectID := false
for _, col := range columns {
    if col == "project_id" {
        hasProjectID = true
        break
    }
}

if !hasProjectID {
    // Trigger migration to add project_id column
    s.logger.WarnLog("[SQLiteStore] project_id column missing, triggering migration")
    if err := s.migrateToV2(); err != nil {
        return fmt.Errorf("failed to migrate to V2: %w", err)
    }
}
```

---

### Step 12: Add Logging for Project ID Discovery

**File**: `provider/gemini/auth.go`

**Action**: Add detailed logging to track project ID discovery and usage.

**Location**: In the `exchangeCodeForTokens()` method.

**Add Logging**:
```go
// After discovering project ID
if projectID != "" {
    a.GetLogger().InfoLog("[Gemini Auth] Successfully discovered project ID '%s' for email '%s'", projectID, email)
} else {
    a.GetLogger().WarnLog("[Gemini Auth] Failed to discover project ID for email '%s'", email)
}

// After saving token
a.GetLogger().InfoLog("[Gemini Auth] Token saved with ID='%s', Email='%s', ProjectID='%s'", providerToken.ID, providerToken.Email, providerToken.ProjectID)
```

---

### Step 13: Run Tests

**Action**: Run all existing tests to ensure no regressions.

**Commands**:
```bash
# Run all tests
go test ./...

# Run tests with coverage
go test -cover ./...

# Run tests for specific packages
go test ./internal/token/...
go test ./provider/gemini/...
```

---

### Step 14: Manual Testing

**Action**: Manually test the fix with multiple Gemini tokens.

**Steps**:
1. Delete existing database or backup it
2. Start the application
3. Authenticate with first Google account via OAuth
4. Verify that token is saved with project_id in the database
5. Authenticate with second Google account via OAuth
6. Verify that second token is saved with different project_id in the database
7. Make API calls using both tokens
8. Verify that each token uses its own project_id

**Verification Queries**:
```sql
-- Check tokens and their project IDs
SELECT id, email, project_id FROM tokens WHERE provider_id = 'gemini-cli';

-- Expected output:
-- id | email | project_id
----|-------|------------
-- abc | user1@gmail.com | project-1
-- def | user2@gmail.com | project-2
```

---

### Step 15: Update Documentation

**File**: `docs/gemini-project-id-per-token-fix.md`

**Action**: Update the documentation to reflect the completed fix.

**Add**:
- Summary of changes made
- How to verify the fix
- Troubleshooting steps if issues persist

---

## Summary of Changes

| File | Change | Lines |
|-------|---------|--------|
| `internal/token/sqlite_store.go` | Add `project_id` to initial schema | ~42 |
| `internal/token/sqlite_store.go` | Add validation for `project_id` column | New code block |
| `provider/gemini/gemini.go` | Remove `projectID` field from Provider struct | ~44 |
| `provider/gemini/gemini.go` | Remove `initializeProject()` method | 311-519 |
| `provider/gemini/gemini.go` | Remove calls to `initializeProject()` | Multiple locations |
| `provider/gemini/auth.go` | Add enhanced logging for project ID discovery | ~400 |

## Testing Checklist

- [ ] Unit tests pass
- [ ] Integration tests pass
- [ ] Manual OAuth flow with first account succeeds
- [ ] Manual OAuth flow with second account succeeds
- [ ] Both tokens have different project_ids in database
- [ ] API calls with first token use correct project_id
- [ ] API calls with second token use correct project_id
- [ ] No errors in logs related to project_id
- [ ] Existing database migration works correctly

## Rollback Plan

If issues arise after implementing this fix:

1. **Revert Database Schema**: Remove the `project_id` column from the initial schema
2. **Restore Shared Field**: Add back the `projectID` field to the Provider struct
3. **Restore initializeProject()**: Add back the `initializeProject()` method
4. **Restore Calls**: Add back calls to `initializeProject()`

## Notes

- The OAuth flow in `exchangeCodeForTokens()` already correctly discovers and saves project_id per token
- The lazy discovery in `discoverProjectIDForToken()` already correctly updates the token's project_id
- The main issue was the shared `p.projectID` field and the missing `project_id` column in the initial schema
- After removing the shared field and fixing the schema, each token will use its own project_id
