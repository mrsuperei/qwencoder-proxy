# Token Refresh Implementation Plan

## Overview

This plan implements automatic token refresh functionality for all providers. The infrastructure already exists (RefreshCoordinator, RefreshScheduler, MultiTokenManager), but provider-specific refreshers are not registered, causing tokens to expire and require manual re-authentication.

## Problem Statement

**Current State**:
- Token refresh infrastructure exists in [`internal/token/token_refresh.go`](../internal/token/token_refresh.go)
- RefreshScheduler checks for expiring tokens every 5 minutes
- MultiTokenManager starts schedulers on server initialization
- **BUT**: Provider-specific refreshers are NOT registered
- Result: Tokens expire, refresh fails with "no refresher registered for provider", users must re-authenticate

## Solution Architecture

### ProviderRefresh Interface

Each provider must implement the `ProviderRefresh` interface:

```go
// From internal/token/token_refresh.go
type ProviderRefresh interface {
    // RefreshToken refreshes a token and returns the updated token.
    RefreshToken(ctx context.Context, token ProviderToken) (ProviderToken, error)
    // ProviderID returns the provider identifier.
    ProviderID() string
}
```

### Registration Flow

```
┌─────────────────────────────────────────────────────────────────┐
│  MultiTokenManager.Start()                         │
│         │                                          │
│         ▼                                          │
│  For each provider:                                 │
│         │                                          │
│         ▼                                          │
│  scheduler.Start() ──► RefreshScheduler.CheckAndSchedule() │
│         │                                          │
│         ▼                                          │
│  isTokenExpiringSoon()?                              │
│         │                                          │
│         ▼                                          │
│  coordinator.ScheduleRefresh() ──► RefreshCoordinator     │
│         │                                          │
│         ▼                                          │
│  processRequest() ──► refresher.RefreshToken()       │
│         │                                          │
│         ▼                                          │
│  store.UpdateToken() ──► Persist to disk           │
└─────────────────────────────────────────────────────────────────┘
```

## Implementation Steps

### Step 1: Create Provider Refresher Implementations

#### 1.1 Gemini Refresher

**File**: `provider/gemini/auth.go`

Add new struct and methods:

```go
// geminiTokenRefresher implements ProviderRefresh for Gemini
type geminiTokenRefresher struct {
    clientID     string
    clientSecret string
    tokenURL     string
    logger       logging.Logger
}

// NewGeminiTokenRefresher creates a new Gemini token refresher
func NewGeminiTokenRefresher(clientID, clientSecret, tokenURL string, logger logging.Logger) *geminiTokenRefresher {
    return &geminiTokenRefresher{
        clientID:     clientID,
        clientSecret: clientSecret,
        tokenURL:     tokenURL,
        logger:       logger,
    }
}

// ProviderID returns the provider identifier
func (g *geminiTokenRefresher) ProviderID() string {
    return "gemini-cli"
}

// RefreshToken refreshes a Gemini OAuth token
func (g *geminiTokenRefresher) RefreshToken(ctx context.Context, token token.ProviderToken) (token.ProviderToken, error) {
    if token.RefreshToken == "" {
        return token.ProviderToken{}, fmt.Errorf("no refresh token available")
    }

    // Prepare refresh request
    data := url.Values{}
    data.Set("client_id", g.clientID)
    data.Set("client_secret", g.clientSecret)
    data.Set("refresh_token", token.RefreshToken)
    data.Set("grant_type", "refresh_token")

    // Make HTTP request
    req, err := http.NewRequestWithContext(ctx, "POST", g.tokenURL, strings.NewReader(data.Encode()))
    if err != nil {
        return token.ProviderToken{}, fmt.Errorf("failed to create refresh request: %w", err)
    }
    req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

    client := &http.Client{Timeout: 30 * time.Second}
    resp, err := client.Do(req)
    if err != nil {
        return token.ProviderToken{}, fmt.Errorf("failed to send refresh request: %w", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        body, _ := io.ReadAll(resp.Body)
        return token.ProviderToken{}, fmt.Errorf("refresh failed with status %d: %s", resp.StatusCode, string(body))
    }

    // Parse response
    var tokenResp struct {
        AccessToken  string `json:"access_token"`
        RefreshToken string `json:"refresh_token"`
        TokenType    string `json:"token_type"`
        ExpiresIn    int64  `json:"expires_in"`
    }
    if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
        return token.ProviderToken{}, fmt.Errorf("failed to decode refresh response: %w", err)
    }

    // Calculate expiry
    expiryDate := time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)

    // Return refreshed token
    return token.ProviderToken{
        ID:           token.ID,
        AccessToken:  tokenResp.AccessToken,
        RefreshToken: tokenResp.RefreshToken,
        TokenType:    tokenResp.TokenType,
        ExpiryDate:   expiryDate.UnixMilli(),
        Email:        token.Email,
        Scope:        token.Scope,
        Healthy:       true,
        HealthScore:   1.0,
        LastUsed:      token.LastUsed,
        CreatedAt:     token.CreatedAt,
    }, nil
}
```

#### 1.2 Qwen Refresher

**File**: `provider/qwen/auth_device.go`

Add new struct and methods:

```go
// qwenTokenRefresher implements ProviderRefresh for Qwen
type qwenTokenRefresher struct {
    clientID string
    tokenURL string
    logger   logging.Logger
}

// NewQwenTokenRefresher creates a new Qwen token refresher
func NewQwenTokenRefresher(logger logging.Logger) *qwenTokenRefresher {
    return &qwenTokenRefresher{
        clientID: OAuthClientID, // From constants
        tokenURL: OAuthTokenURL,     // From constants
        logger:   logger,
    }
}

// ProviderID returns the provider identifier
func (q *qwenTokenRefresher) ProviderID() string {
    return "qwen"
}

// RefreshToken refreshes a Qwen OAuth token
func (q *qwenTokenRefresher) RefreshToken(ctx context.Context, token token.ProviderToken) (token.ProviderToken, error) {
    if token.RefreshToken == "" {
        return token.ProviderToken{}, fmt.Errorf("no refresh token available")
    }

    // Prepare refresh request
    data := url.Values{}
    data.Set("client_id", q.clientID)
    data.Set("refresh_token", token.RefreshToken)
    data.Set("grant_type", "refresh_token")

    // Make HTTP request
    req, err := http.NewRequestWithContext(ctx, "POST", q.tokenURL, strings.NewReader(data.Encode()))
    if err != nil {
        return token.ProviderToken{}, fmt.Errorf("failed to create refresh request: %w", err)
    }
    req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

    client := &http.Client{Timeout: 30 * time.Second}
    resp, err := client.Do(req)
    if err != nil {
        return token.ProviderToken{}, fmt.Errorf("failed to send refresh request: %w", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        body, _ := io.ReadAll(resp.Body)
        return token.ProviderToken{}, fmt.Errorf("refresh failed with status %d: %s", resp.StatusCode, string(body))
    }

    // Parse response
    var tokenResp struct {
        AccessToken  string `json:"access_token"`
        RefreshToken string `json:"refresh_token"`
        TokenType    string `json:"token_type"`
        ExpiresIn    int64  `json:"expires_in"`
    }
    if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
        return token.ProviderToken{}, fmt.Errorf("failed to decode refresh response: %w", err)
    }

    // Calculate expiry
    expiryDate := time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)

    // Return refreshed token
    return token.ProviderToken{
        ID:          token.ID,
        AccessToken:  tokenResp.AccessToken,
        RefreshToken: tokenResp.RefreshToken,
        TokenType:    tokenResp.TokenType,
        ExpiryDate:   expiryDate.UnixMilli(),
        Email:        token.Email,
        ResourceURL:   token.ResourceURL,
        Healthy:       true,
        HealthScore:   1.0,
        LastUsed:      token.LastUsed,
        CreatedAt:     token.CreatedAt,
    }, nil
}
```

#### 1.3 iFlow Refresher

**File**: `provider/iflow/auth.go`

Add new struct and methods:

```go
// iflowTokenRefresher implements ProviderRefresh for iFlow
type iflowTokenRefresher struct {
    clientID     string
    clientSecret string
    tokenURL     string
    logger       logging.Logger
}

// NewIFlowTokenRefresher creates a new iFlow token refresher
func NewIFlowTokenRefresher(clientID, clientSecret, tokenURL string, logger logging.Logger) *iflowTokenRefresher {
    return &iflowTokenRefresher{
        clientID:     clientID,
        clientSecret: clientSecret,
        tokenURL:     tokenURL,
        logger:       logger,
    }
}

// ProviderID returns the provider identifier
func (i *iflowTokenRefresher) ProviderID() string {
    return "iflow"
}

// RefreshToken refreshes an iFlow OAuth token
func (i *iflowTokenRefresher) RefreshToken(ctx context.Context, token token.ProviderToken) (token.ProviderToken, error) {
    if token.RefreshToken == "" {
        return token.ProviderToken{}, fmt.Errorf("no refresh token available")
    }

    // Prepare refresh request
    data := url.Values{}
    data.Set("client_id", i.clientID)
    data.Set("client_secret", i.clientSecret)
    data.Set("refresh_token", token.RefreshToken)
    data.Set("grant_type", "refresh_token")

    // Make HTTP request
    req, err := http.NewRequestWithContext(ctx, "POST", i.tokenURL, strings.NewReader(data.Encode()))
    if err != nil {
        return token.ProviderToken{}, fmt.Errorf("failed to create refresh request: %w", err)
    }
    req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

    client := &http.Client{Timeout: 30 * time.Second}
    resp, err := client.Do(req)
    if err != nil {
        return token.ProviderToken{}, fmt.Errorf("failed to send refresh request: %w", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        body, _ := io.ReadAll(resp.Body)
        return token.ProviderToken{}, fmt.Errorf("refresh failed with status %d: %s", resp.StatusCode, string(body))
    }

    // Parse response
    var tokenResp struct {
        AccessToken  string `json:"access_token"`
        RefreshToken string `json:"refresh_token"`
        TokenType    string `json:"token_type"`
        ExpiresIn    int64  `json:"expires_in"`
    }
    if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
        return token.ProviderToken{}, fmt.Errorf("failed to decode refresh response: %w", err)
    }

    // Calculate expiry
    expiryDate := time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)

    // Return refreshed token
    return token.ProviderToken{
        ID:          token.ID,
        AccessToken:  tokenResp.AccessToken,
        RefreshToken: tokenResp.RefreshToken,
        TokenType:    tokenResp.TokenType,
        ExpiryDate:   expiryDate.UnixMilli(),
        Email:        token.Email,
        APIKey:       token.APIKey,
        Healthy:       true,
        HealthScore:   1.0,
        LastUsed:      token.LastUsed,
        CreatedAt:     token.CreatedAt,
    }, nil
}
```

#### 1.4 Kiro Refresher

**File**: `provider/kiro/auth.go`

The Kiro provider already has a refresher implementation (`kiroTokenRefresher`), but it needs to be adapted to implement the `ProviderRefresh` interface:

```go
// kiroTokenRefresherAdapter adapts existing kiroTokenRefresher to ProviderRefresh interface
type kiroTokenRefresherAdapter struct {
    auth *Authenticator
}

// NewKiroTokenRefresherAdapter creates a new adapter
func NewKiroTokenRefresherAdapter(auth *Authenticator) *kiroTokenRefresherAdapter {
    return &kiroTokenRefresherAdapter{auth: auth}
}

// ProviderID returns the provider identifier
func (k *kiroTokenRefresherAdapter) ProviderID() string {
    return "kiro"
}

// RefreshToken refreshes a Kiro token using the existing authenticator
func (k *kiroTokenRefresherAdapter) RefreshToken(ctx context.Context, token token.ProviderToken) (token.ProviderToken, error) {
    if token.RefreshToken == "" {
        return token.ProviderToken{}, fmt.Errorf("no refresh token available")
    }

    // Use existing performRefresh method from Authenticator
    oauthToken, err := k.auth.performRefresh(ctx)
    if err != nil {
        return token.ProviderToken{}, fmt.Errorf("failed to refresh Kiro token: %w", err)
    }

    // Return refreshed token
    return token.ProviderToken{
        ID:          token.ID,
        AccessToken:  oauthToken.AccessToken,
        RefreshToken: oauthToken.RefreshToken,
        TokenType:    oauthToken.TokenType,
        ExpiryDate:   oauthToken.Expiry.UnixMilli(),
        Email:        token.Email,
        Healthy:       true,
        HealthScore:   1.0,
        LastUsed:      token.LastUsed,
        CreatedAt:     token.CreatedAt,
    }, nil
}
```

#### 1.5 Antigravity Refresher

**File**: `provider/antigravity/auth.go`

Antigravity uses the same OAuth flow as Gemini, so it can use a similar refresher:

```go
// antigravityTokenRefresher implements ProviderRefresh for Antigravity
type antigravityTokenRefresher struct {
    clientID     string
    clientSecret string
    tokenURL     string
    logger       logging.Logger
}

// NewAntigravityTokenRefresher creates a new Antigravity token refresher
func NewAntigravityTokenRefresher(clientID, clientSecret, tokenURL string, logger logging.Logger) *antigravityTokenRefresher {
    return &antigravityTokenRefresher{
        clientID:     clientID,
        clientSecret: clientSecret,
        tokenURL:     tokenURL,
        logger:       logger,
    }
}

// ProviderID returns the provider identifier
func (a *antigravityTokenRefresher) ProviderID() string {
    return "antigravity"
}

// RefreshToken refreshes an Antigravity OAuth token
func (a *antigravityTokenRefresher) RefreshToken(ctx context.Context, token token.ProviderToken) (token.ProviderToken, error) {
    // Same implementation as Gemini refresher
    // (Antigravity uses Google OAuth)
    if token.RefreshToken == "" {
        return token.ProviderToken{}, fmt.Errorf("no refresh token available")
    }

    // Prepare refresh request
    data := url.Values{}
    data.Set("client_id", a.clientID)
    data.Set("client_secret", a.clientSecret)
    data.Set("refresh_token", token.RefreshToken)
    data.Set("grant_type", "refresh_token")

    // Make HTTP request
    req, err := http.NewRequestWithContext(ctx, "POST", a.tokenURL, strings.NewReader(data.Encode()))
    if err != nil {
        return token.ProviderToken{}, fmt.Errorf("failed to create refresh request: %w", err)
    }
    req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

    client := &http.Client{Timeout: 30 * time.Second}
    resp, err := client.Do(req)
    if err != nil {
        return token.ProviderToken{}, fmt.Errorf("failed to send refresh request: %w", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        body, _ := io.ReadAll(resp.Body)
        return token.ProviderToken{}, fmt.Errorf("refresh failed with status %d: %s", resp.StatusCode, string(body))
    }

    // Parse response
    var tokenResp struct {
        AccessToken  string `json:"access_token"`
        RefreshToken string `json:"refresh_token"`
        TokenType    string `json:"token_type"`
        ExpiresIn    int64  `json:"expires_in"`
    }
    if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
        return token.ProviderToken{}, fmt.Errorf("failed to decode refresh response: %w", err)
    }

    // Calculate expiry
    expiryDate := time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)

    // Return refreshed token
    return token.ProviderToken{
        ID:          token.ID,
        AccessToken:  tokenResp.AccessToken,
        RefreshToken: tokenResp.RefreshToken,
        TokenType:    tokenResp.TokenType,
        ExpiryDate:   expiryDate.UnixMilli(),
        Email:        token.Email,
        Healthy:       true,
        HealthScore:   1.0,
        LastUsed:      token.LastUsed,
        CreatedAt:     token.CreatedAt,
    }, nil
}
```

### Step 2: Register Refreshers During Provider Initialization

#### 2.1 Update restapi/rest_api.go

Add a method to register all provider refreshers:

```go
// registerProviderRefreshers registers token refreshers for all providers
func (s *Server) registerProviderRefreshers() error {
    if s.multiTokenManager == nil {
        return fmt.Errorf("multi-token manager not initialized")
    }

    s.logger.InfoLog("[Server] Registering provider refreshers...")

    // Register Gemini refresher
    geminiRefresher := gemini.NewGeminiTokenRefresher(
        "681255809395-oo8ft2oprdrnp9e3aqf6av3hmdib135j.apps.googleusercontent.com",
        "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",
        "https://oauth2.googleapis.com/token",
        s.logger,
    )
    if err := s.multiTokenManager.RegisterRefresher("gemini-cli", geminiRefresher); err != nil {
        s.logger.ErrorLog("[Server] Failed to register Gemini refresher: %v", err)
        return fmt.Errorf("failed to register Gemini refresher: %w", err)
    }
    s.logger.InfoLog("[Server] Registered Gemini refresher")

    // Register Qwen refresher
    qwenRefresher := qwen.NewQwenTokenRefresher(s.logger)
    if err := s.multiTokenManager.RegisterRefresher("qwen", qwenRefresher); err != nil {
        s.logger.ErrorLog("[Server] Failed to register Qwen refresher: %v", err)
        return fmt.Errorf("failed to register Qwen refresher: %w", err)
    }
    s.logger.InfoLog("[Server] Registered Qwen refresher")

    // Register iFlow refresher
    iflowRefresher := iflow.NewIFlowTokenRefresher(
        "10009311001",
        "4Z3YjXycVsQvyGF1etiNlIBB4RsqSDtW",
        "https://iflow.cn/oauth/token",
        s.logger,
    ); iflowRefresher != nil {
        if err := s.multiTokenManager.RegisterRefresher("iflow", iflowRefresher); err != nil {
            s.logger.ErrorLog("[Server] Failed to register iFlow refresher: %v", err)
            return fmt.Errorf("failed to register iFlow refresher: %w", err)
        }
        s.logger.InfoLog("[Server] Registered iFlow refresher")
    }

    // Register Kiro refresher
    // Note: Kiro uses a different auth mechanism, need to get authenticator instance
    // This will be handled separately when providers are initialized

    // Register Antigravity refresher
    antigravityRefresher := antigravity.NewAntigravityTokenRefresher(
        "681255809395-oo8ft2oprdrnp9e3aqf6av3hmdib135j.apps.googleusercontent.com",
        "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",
        "https://oauth2.googleapis.com/token",
        s.logger,
    )
    if err := s.multiTokenManager.RegisterRefresher("antigravity", antigravityRefresher); err != nil {
        s.logger.ErrorLog("[Server] Failed to register Antigravity refresher: %v", err)
        return fmt.Errorf("failed to register Antigravity refresher: %w", err)
    }
    s.logger.InfoLog("[Server] Registered Antigravity refresher")

    s.logger.InfoLog("[Server] All provider refreshers registered successfully")
    return nil
}
```

#### 2.2 Call Registration During Server Initialization

Update the `NewServer` function:

```go
// NewServer creates a new OAuth REST API server
func NewServer(config *Config, logger logging.Logger) *Server {
    if config == nil {
        config = DefaultConfig()
    }
    if logger == nil {
        // Create a simple logger if none provided
        logger = logging.NewLogger()
    }

    // Create multi-token manager
    multiTokenManager := tokpkg.NewMultiTokenManager(logger)
    if err := multiTokenManager.Initialize(); err != nil {
        logger.ErrorLog("Failed to initialize multi-token manager: %v", err)
    }
    if err := multiTokenManager.Start(); err != nil {
        logger.ErrorLog("Failed to start multi-token manager: %v", err)
    }

    server := &Server{
        config:            config,
        registry:          NewProviderRegistry(),
        stateManager:      NewStateManager(),
        logger:            logger,
        httpClient:        &http.Client{Timeout: 30 * time.Second},
        tokenStores:       make(map[string]*tokpkg.MultiTokenStore),
        tokenManagers:     make(map[string]*tokpkg.TokenManager),
        multiTokenManager: multiTokenManager,
    }

    // Register provider refreshers
    if err := server.registerProviderRefreshers(); err != nil {
        logger.ErrorLog("Failed to register provider refreshers: %v", err)
    }

    return server
}
```

### Step 3: Ensure Refresh Tokens Are Saved

Update authentication flows to ensure refresh tokens are saved:

#### 3.1 Update provider/gemini/auth.go

```go
// In the exchange code flow, ensure refresh_token is saved
func (a *Authenticator) exchangeCodeForToken(ctx context.Context, code string) (*Credentials, error) {
    // ... existing code ...

    // Save credentials with refresh token
    if err := a.saveCredentials(creds); err != nil {
        return nil, fmt.Errorf("failed to save credentials: %w", err)
    }

    return creds, nil
}
```

#### 3.2 Update provider/qwen/auth_device.go

Already saves refresh token correctly (line 103):
```go
tokenResponse["refresh_token"] = oauth2Token.RefreshToken
```

#### 3.3 Update provider/iflow/auth.go

Ensure refresh token is saved in the token response handling.

### Step 4: Add Logging and Monitoring

Add comprehensive logging for token refresh operations:

```go
// In each refresher's RefreshToken method:
func (r *refresher) RefreshToken(ctx context.Context, token token.ProviderToken) (token.ProviderToken, error) {
    r.logger.InfoLog("[Refresh] Starting token refresh for provider %s, token ID %s", r.ProviderID(), token.ID)
    
    // ... refresh logic ...
    
    if err != nil {
        r.logger.ErrorLog("[Refresh] Token refresh failed for provider %s, token ID %s: %v", r.ProviderID(), token.ID, err)
        return token.ProviderToken{}, err
    }
    
    r.logger.InfoLog("[Refresh] Token refresh successful for provider %s, token ID %s, new expiry %s", 
        r.ProviderID(), token.ID, expiryDate.Format(time.RFC3339))
    
    return refreshedToken, nil
}
```

### Step 5: Testing

#### 5.1 Unit Tests

Create unit tests for each refresher:

```go
// provider/gemini/auth_test.go
func TestGeminiTokenRefresher(t *testing.T) {
    logger := logging.NewLogger()
    refresher := NewGeminiTokenRefresher("test-id", "test-secret", "https://test.com/token", logger)
    
    // Test with valid token
    token := token.ProviderToken{
        ID:           "test-id",
        RefreshToken: "valid-refresh-token",
        Email:        "test@example.com",
    }
    
    // Mock HTTP server for testing
    // ...
}
```

#### 5.2 Integration Tests

Create integration tests to verify refresh flow:

```go
// internal/token/token_refresh_integration_test.go
func TestTokenRefreshFlow(t *testing.T) {
    // Create multi-token manager
    mtm := NewMultiTokenManager(logging.NewLogger())
    mtm.Initialize()
    
    // Register test refresher
    testRefresher := &testTokenRefresher{}
    mtm.RegisterRefresher("test-provider", testRefresher)
    mtm.Start()
    
    // Add expiring token
    token := ProviderToken{
        ID:           "test-token",
        RefreshToken: "refresh-token",
        ExpiryDate:   time.Now().Add(10 * time.Minute).UnixMilli(),
        Healthy:       true,
    }
    mtm.SaveToken("test-provider", token)
    
    // Wait for refresh
    time.Sleep(10 * time.Second)
    
    // Verify token was refreshed
    refreshed, _ := mtm.GetTokenStore("test-provider").GetToken("test-token")
    assert.True(t, refreshed.ExpiryDate > token.ExpiryDate)
}
```

#### 5.3 Manual Testing

1. Start server with refreshers registered
2. Authenticate with a provider
3. Wait for token to approach expiry (or adjust refresh buffer)
4. Verify token is automatically refreshed
5. Check logs for refresh operations
6. Verify no re-authentication is required

## Definition of Done

- [ ] All providers have `ProviderRefresh` implementations
- [ ] Refreshers are registered during server initialization
- [ ] Refresh tokens are saved during authentication
- [ ] Unit tests exist for all refreshers
- [ ] Integration tests verify refresh flow
- [ ] Manual testing confirms automatic refresh works
- [ ] Logs show successful refresh operations
- [ ] No "no refresher registered" errors in logs
- [ ] Tokens persist across server restarts
- [ ] Users do not need to re-authenticate after expiry

## Rollback Plan

If issues arise:

1. Disable automatic refresh by commenting out `RegisterRefresher` calls
2. System falls back to manual re-authentication
3. Investigate logs for specific refresh failures
4. Fix individual refresher implementations
5. Re-enable refreshers one at a time

## References

- [`internal/token/token_refresh.go`](../internal/token/token_refresh.go) - RefreshCoordinator and RefreshScheduler
- [`internal/token/multi_token_manager.go`](../internal/token/multi_token_manager.go) - MultiTokenManager
- [`internal/token/multi_token_store.go`](../internal/token/multi_token_store.go) - Token persistence
- [`provider/kiro/auth.go`](../provider/kiro/auth.go) - Existing Kiro refresh implementation
- [`restapi/rest_api.go`](../restapi/rest_api.go) - Server initialization
