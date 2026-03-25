# Fix OAuth Email Extraction Bug

**Priority:** CRITICAL  
**Estimated Time:** 5 minutes  
**Complexity:** Low  
**Files to Modify:** 1

---

## Problem Description

During OAuth authentication flow, user emails are being saved as `unknown@example.com` instead of the actual user email address extracted from the OAuth token response.

### Root Cause

**File:** `internal/restapi/rest_api.go`  
**Function:** `extractEmailFromToken()` (lines 1166-1184)

The function receives an `accessToken` parameter but **does not receive** the `tokenResponseMap` that contains the OAuth token response with email information. Instead, it passes `nil` to the email extractor's `ExtractEmail()` method.

### Call Chain Analysis

1. **`handleCallback()`** (line 789) receives OAuth callback
2. **`exchangeCodeForTokensWithResponse()`** (line 916) exchanges code for tokens and returns:
   - `creds tokpkg.OAuthCreds` - The OAuth credentials
   - `tokenResponseMap map[string]interface{}` - The full token response with email
3. **`saveCredentials()`** (line 1117) is called with both `creds` and `tokenResponseMap`
4. **`saveCredentials()`** calls `extractEmailFromToken()` (line 1120) with only `providerID` and `creds.AccessToken`
5. **`extractEmailFromToken()`** (line 1166) creates an `EmailExtractionManager` and calls:
   ```go
   emailExtractor.ExtractEmail(context.Background(), nil, accessToken)
   ```
   **Problem:** Passing `nil` as the `tokenResponse` parameter
6. Email extractor's `ExtractEmail()` method first tries to extract email from `tokenResponse` (which is `nil`), so it fails
7. Falls back to calling user info endpoint, which may fail or not return email
8. If both fail, error is caught and email is set to `unknown@example.com`

### Current Code

**File:** `internal/restapi/rest_api.go`

```go
// Line 1117-1124: saveCredentials function
func (s *Server) saveCredentials(providerID string, creds tokpkg.OAuthCreds, tokenResponse map[string]interface{}) error {
	// Extract email from token
	email, err := s.extractEmailFromToken(providerID, creds.AccessToken)  // ❌ tokenResponse not passed
	if err != nil {
		s.logger.WarnLog("Failed to extract email for %s: %v", providerID, err)
		email = "unknown@example.com"
	}
	// ... rest of function
}

// Line 1166-1184: extractEmailFromToken function
func (s *Server) extractEmailFromToken(providerID, accessToken string) (string, error) {
	// Create email extraction manager
	extractor := tokpkg.NewEmailExtractionManager(s.logger)

	// Get email extractor for this provider
	emailExtractor, err := extractor.GetExtractor(providerID)
	if err != nil {
		return "", fmt.Errorf("failed to get email extractor for %s: %w", providerID, err)
	}

	// Extract email from token
	email, err := emailExtractor.ExtractEmail(context.Background(), nil, accessToken)  // ❌ nil passed here
	if err != nil {
		return "", fmt.Errorf("failed to extract email: %w", err)
	}

	return email, nil
}
```

---

## Solution

### Step 1: Modify `extractEmailFromToken()` signature

**File:** `internal/restapi/rest_api.go`  
**Function:** `extractEmailFromToken()` (line 1166)

**Change:** Add `tokenResponse map[string]interface{}` parameter

```go
// BEFORE:
func (s *Server) extractEmailFromToken(providerID, accessToken string) (string, error) {

// AFTER:
func (s *Server) extractEmailFromToken(providerID, accessToken string, tokenResponse map[string]interface{}) (string, error) {
```

### Step 2: Pass `tokenResponse` to email extractor

**File:** `internal/restapi/rest_api.go`  
**Function:** `extractEmailFromToken()` (line 1178)

**Change:** Pass `tokenResponse` instead of `nil`

```go
// BEFORE:
email, err := emailExtractor.ExtractEmail(context.Background(), nil, accessToken)

// AFTER:
email, err := emailExtractor.ExtractEmail(context.Background(), tokenResponse, accessToken)
```

### Step 3: Update `saveCredentials()` call

**File:** `internal/restapi/rest_api.go`  
**Function:** `saveCredentials()` (line 1120)

**Change:** Pass `tokenResponseMap` to `extractEmailFromToken()`

```go
// BEFORE:
email, err := s.extractEmailFromToken(providerID, creds.AccessToken)

// AFTER:
email, err := s.extractEmailFromToken(providerID, creds.AccessToken, tokenResponse)
```

---

## Complete Fixed Code

**File:** `internal/restapi/rest_api.go`

```go
// saveCredentials saves credentials for a provider with email extraction
func (s *Server) saveCredentials(providerID string, creds tokpkg.OAuthCreds, tokenResponse map[string]interface{}) error {
	// Extract email from token
	email, err := s.extractEmailFromToken(providerID, creds.AccessToken, tokenResponse)  // ✅ Pass tokenResponse
	if err != nil {
		s.logger.WarnLog("Failed to extract email for %s: %v", providerID, err)
		email = "unknown@example.com"
	}

	// Save credentials to database
	store, err := s.getTokenStore(providerID)
	if err != nil {
		return fmt.Errorf("failed to get token store: %w", err)
	}

	// Create provider token
	token := tokpkg.ProviderToken{
		ID:           uuid.New().String(),
		AccessToken:  creds.AccessToken,
		RefreshToken: creds.RefreshToken,
		TokenType:    creds.TokenType,
		ExpiryDate:   creds.ExpiryDate,
		ResourceURL:  creds.ResourceURL,
		Email:        email,
		Healthy:      true,
		HealthScore:  1.0,
		LastUsed:     time.Now().UnixMilli(),
		CreatedAt:    time.Now().UnixMilli(),
		ErrorCount:   0,
	}

	// Load existing tokens
	tokensMap, loadErr := store.Load()
	if loadErr != nil {
		return fmt.Errorf("failed to load tokens: %w", loadErr)
	}

	// Add new token
	tokensMap[token.ID] = token

	// Save all tokens
	if err := store.Save(tokensMap); err != nil {
		return fmt.Errorf("failed to save tokens: %w", err)
	}

	s.logger.InfoLog("Credentials saved successfully for provider %s: %s", providerID, email)
	return nil
}

// extractEmailFromToken extracts email from an access token
func (s *Server) extractEmailFromToken(providerID, accessToken string, tokenResponse map[string]interface{}) (string, error) {  // ✅ Add tokenResponse parameter
	// Create email extraction manager
	extractor := tokpkg.NewEmailExtractionManager(s.logger)

	// Get email extractor for this provider
	emailExtractor, err := extractor.GetExtractor(providerID)
	if err != nil {
		return "", fmt.Errorf("failed to get email extractor for %s: %w", providerID, err)
	}

	// Extract email from token
	email, err := emailExtractor.ExtractEmail(context.Background(), tokenResponse, accessToken)  // ✅ Pass tokenResponse instead of nil
	if err != nil {
		return "", fmt.Errorf("failed to extract email: %w", err)
	}

	return email, nil
}
```

---

## Testing

### Manual Test Steps

1. Start the proxy server
2. Navigate to web dashboard
3. Click "Authenticate" for any OAuth provider (Qwen, Gemini, iFlow)
4. Complete OAuth flow in browser
5. Check dashboard - email should display actual user email, not `unknown@example.com`
6. Check SQLite database:
   ```bash
   sqlite3 .credentials/tokens.db "SELECT email FROM tokens ORDER BY created_at DESC LIMIT 1"
   ```
   Should return actual user email

### Automated Test

**File:** `internal/restapi/rest_api_test.go` (create if not exists)

```go
func TestExtractEmailFromToken(t *testing.T) {
	server := NewServer(DefaultConfig(), logging.NewLogger())
	
	// Test with token response containing email
	tokenResponse := map[string]interface{}{
		"email": "test@example.com",
		"access_token": "test_token",
	}
	
	email, err := server.extractEmailFromToken("gemini-cli", "test_token", tokenResponse)
	
	assert.NoError(t, err)
	assert.Equal(t, "test@example.com", email)
}

func TestExtractEmailFromTokenWithoutEmail(t *testing.T) {
	server := NewServer(DefaultConfig(), logging.NewLogger())
	
	// Test with token response without email
	tokenResponse := map[string]interface{}{
		"access_token": "test_token",
	}
	
	email, err := server.extractEmailFromToken("gemini-cli", "test_token", tokenResponse)
	
	// Should return error since email not in response
	assert.Error(t, err)
}
```

---

## Verification Checklist

- [ ] `extractEmailFromToken()` signature updated to include `tokenResponse` parameter
- [ ] `extractEmailFromToken()` passes `tokenResponse` to `emailExtractor.ExtractEmail()`
- [ ] `saveCredentials()` passes `tokenResponseMap` to `extractEmailFromToken()`
- [ ] Code compiles without errors
- [ ] Manual test: OAuth flow saves actual email
- [ ] Database verification: Email field contains user email
- [ ] All providers tested (Qwen, Gemini, iFlow, Kiro, Antigravity)

---

## Impact

**Positive:**
- Emails will be correctly extracted and saved during OAuth flow
- User identification improved in dashboard
- Better token management with accurate email metadata

**Risk:**
- None - this is a simple parameter addition
- No breaking changes to existing functionality
- Backward compatible (new parameter has default value in Go)

**Side Effects:**
- None - only affects email extraction logic
