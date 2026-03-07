# Gemini Project ID Single Token Issue Analysis

## Problem Statement

The user reports that only one project_id is being saved in the database for Gemini tokens, even though there are two tokens. When the second token is used, it attempts to use the project_id from the first token instead of its own project_id. Each token should get its own project_id from the Gemini OAuth flow.

## Root Cause Analysis

### Issue 1: Database Schema Mismatch

The initial table schema in [`internal/token/sqlite_store.go:30-50`](../internal/token/sqlite_store.go:30-50) does NOT include the `project_id` column:

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

However, the prepared statements (INSERT, UPDATE, SELECT) all reference `project_id`:

- INSERT statement at line168-175 includes `project_id`
- UPDATE statement at line218-224 includes `project_id`
- SELECT statements at lines181-198 and205-211 include `project_id`

This mismatch means:
1. If the table was created before migration V2, it won't have the `project_id` column
2. The INSERT/UPDATE operations would fail or silently ignore the `project_id` value
3. The migration `migrateToV2()` at line459-475 adds the column, but existing tokens won't have project_ids

### Issue 2: Shared Cached Project ID

The [`Provider`](../provider/gemini/gemini.go:39) struct has a cached `projectID` field:

```go
type Provider struct {
    *provider.BaseProvider
    baseURL                string
    authenticator          *Authenticator
    projectID              string  // <-- SHARED ACROSS ALL TOKENS
    projectInitError       error
}
```

The [`initializeProject()`](../provider/gemini/gemini.go:312) method sets this shared field:

```go
// Line 428-430
if projectID, ok := loadResponse["cloudaicompanionProject"].(string); ok && projectID != "" {
    p.projectID = projectID  // <-- Sets shared field
    p.GetLogger().DebugLog("[Gemini] Using existing project ID: %s", p.projectID)
    return nil
}

// Line 503-505
if id, idExists := project["id"].(string); idExists {
    p.projectID = id  // <-- Sets shared field
    p.GetLogger().DebugLog("[Gemini] Created new project ID: %s", p.projectID)
    return nil
}

// Line 512-514
if id, exists := onboardResponse["cloudaicompanionProject"].(string); exists {
    p.projectID = id  // <-- Sets shared field
    p.GetLogger().DebugLog("[Gemini] Discovered project ID: %s", p.projectID)
    return nil
}
```

This means:
1. When `initializeProject()` is called, it discovers a project_id for ONE token
2. This project_id is cached in `p.projectID` (shared field)
3. All subsequent tokens use this same cached project_id

### Issue 3: OAuth Flow Does Save Project ID Per Token

The OAuth flow in [`exchangeCodeForTokens()`](../provider/gemini/auth.go:315) DOES correctly discover and save project_id per token:

```go
// Line 382-389
// Discover project ID for this account
projectID, err := a.discoverProjectID(ctx, tokenResp.AccessToken)
if err != nil {
    a.GetLogger().WarnLog("[Gemini Auth] Failed to discover project ID: %v", err)
    projectID = "" // Empty string means project ID not yet discovered
} else {
    a.GetLogger().InfoLog("[Gemini Auth] Discovered project ID: %s for email: %s", projectID, email)
}

// Line 391-404
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

// Line 406-409
// Save token to multi-token store
if err := multiTokenMgr.SaveToken("gemini", providerToken); err != nil {
    return fmt.Errorf("failed to save token to multi-token store: %w", err)
}
```

This part is CORRECT - each token gets its own project_id during OAuth.

### Issue 4: Database Save May Fail Silently

Looking at the [`Save()`](../internal/token/sqlite_store.go:593) method, there's a potential issue:

```go
// Line 624-645
if existingID != "" {
    // Token exists, update it
    _, err = tx.Stmt(s.stmtUpdateToken).Exec(
        token.AccessToken,
        token.RefreshToken,
        token.TokenType,
        token.ExpiryDate,
        token.Email,
        token.ResourceURL,
        token.Scope,
        token.APIKey,
        sql.NullString{String: token.ProjectID, Valid: token.ProjectID != ""},
        // ...
    )
```

If the `project_id` column doesn't exist in the database (before migration), the UPDATE statement would fail. However, the error might be caught and the transaction rolled back, but the user wouldn't see a clear error message about the missing column.

## Summary of Issues

1. **Database Schema Mismatch**: The initial schema doesn't include `project_id`, but the code tries to use it.

2. **Shared Cached Project ID**: The `Provider.projectID` field is shared across all tokens. When `initializeProject()` is called, it sets this field to the project_id of the first token, and all subsequent tokens use this same value.

3. **Migration Timing**: If the database was created before migration V2, existing tokens won't have the `project_id` column. The migration adds the column, but existing tokens need to be updated.

4. **Potential Silent Failures**: The INSERT/UPDATE operations might fail silently if the `project_id` column doesn't exist.

## Solution

### Phase 1: Fix Database Schema

1. **Update Initial Schema**: Add `project_id` column to the initial table schema in [`sqlCreateTokensTable`](../internal/token/sqlite_store.go:30-50).

2. **Ensure Migration Runs**: Verify that the migration to V2 runs correctly and adds the column to existing databases.

3. **Add Validation**: Add a check to verify the `project_id` column exists before attempting to save tokens.

### Phase 2: Fix Shared Cached Project ID

1. **Remove Shared Field**: Remove or deprecate the `p.projectID` field from the [`Provider`](../provider/gemini/gemini.go:39) struct.

2. **Use Token-Specific Project ID**: Ensure all code paths use `selectedToken.ProjectID` instead of `p.projectID`.

3. **Remove initializeProject()**: The `initializeProject()` method sets the shared `p.projectID` field. This method should be removed or refactored to not use the shared field.

### Phase 3: Ensure Per-Token Project ID Discovery

1. **Verify OAuth Flow**: Ensure the OAuth flow in [`exchangeCodeForTokens()`](../provider/gemini/auth.go:315) correctly discovers and saves project_id per token.

2. **Lazy Discovery**: Ensure the lazy project ID discovery in [`GenerateContent()`](../provider/gemini/gemini.go:542) and [`GenerateContentStream()`](../provider/gemini/gemini.go:743) works correctly.

3. **Update Token on Discovery**: When project ID is discovered lazily, ensure it's saved to the database using `UpdateToken()`.

## Recommended Implementation Order

1. **Fix Database Schema First**: Update the initial schema to include `project_id` column.

2. **Remove Shared Field**: Remove the `p.projectID` field from the Provider struct.

3. **Update initializeProject()**: Refactor or remove this method to not use the shared field.

4. **Verify All Code Paths**: Ensure all code paths use `selectedToken.ProjectID` instead of `p.projectID`.

5. **Add Logging**: Add detailed logging to track project_id discovery and usage.

6. **Add Tests**: Add tests to verify that each token gets its own project_id.

## Files to Modify

1. [`internal/token/sqlite_store.go`](../internal/token/sqlite_store.go) - Update initial schema to include `project_id` column
2. [`provider/gemini/gemini.go`](../provider/gemini/gemini.go) - Remove shared `projectID` field, update `initializeProject()` method
3. [`provider/gemini/auth.go`](../provider/gemini/auth.go) - Verify OAuth flow correctly saves project_id per token

## Testing Strategy

1. **Unit Tests**: Test that each token gets its own project_id during OAuth.
2. **Integration Tests**: Test that tokens with different project_ids work correctly.
3. **Database Migration Tests**: Test that migration V2 correctly adds the `project_id` column.
4. **End-to-End Tests**: Test the complete flow from OAuth to API call with multiple tokens.
