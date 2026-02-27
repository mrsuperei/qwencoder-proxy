# Phase 4: Common Token Selection Pattern

## Problem

### Code Duplication: Token Selection Logic

The project contains **nearly identical token selection logic duplicated across 4 provider implementations**. Each provider has its own copy of the token selection pattern in both `GenerateContent()` and `GenerateContentStream()` methods.

### Affected Files

| File | Method | Lines | Pattern |
|------|----------|--------|----------|
| [`provider/gemini/gemini.go`](qwencoder-proxy/provider/gemini/gemini.go:317-347) | `initializeProject()` | 317-347 |
| [`provider/iflow/iflow.go`](qwencoder-proxy/provider/iflow/iflow.go:258-289) | `GenerateContent()` | 258-289 |
| [`provider/iflow/iflow.go`](qwencoder-proxy/provider/iflow/iflow.go:382-413) | `GenerateContentStream()` | 382-413 |
| [`provider/qwen/qwen.go`](qwencoder-proxy/provider/qwen/qwen.go:368-391) | `GenerateContent()` | 368-391 |
| [`provider/qwen/qwen.go`](qwencoder-proxy/provider/qwen/qwen.go:458-481) | `GenerateContentStream()` | 458-481 |

**Total duplicated code: ~120 lines**

### Example of Identical Code Pattern

All 4 providers have **nearly identical** token selection logic:

```go
// This pattern is repeated 4 times with only the provider name changing
// Try to use token manager for proxy-aware client selection
if p.GetTokenManager() != nil {
    selectedToken, selectedClient, selectErr := p.GetTokenManager().SelectTokenWithClient()
    if selectErr != nil {
        p.GetLogger().ErrorLog("[ProviderName] Token selection failed: %v", selectErr)
        return nil, fmt.Errorf("failed to select token: %w", selectErr)
    }
    if selectedClient == nil {
        p.GetLogger().DebugLog("[ProviderName] Token manager returned nil client, using default HTTP client")
        client = p.GetHTTPClient()
    } else {
        client = selectedClient
    }
    token = selectedToken.AccessToken
    tokenID = selectedToken.ID

    // Log proxy usage
    if selectedToken.Proxy != nil && selectedToken.Proxy.Type != "none" {
        p.GetLogger().DebugLog("[ProviderName] GenerateContent using token %s with proxy: %s:%d", tokenID, selectedToken.Proxy.Host, selectedToken.Proxy.Port)
    } else {
        p.GetLogger().DebugLog("[ProviderName] GenerateContent using token %s with direct connection", tokenID)
    }
} else {
    // No token manager, use authenticator and default client (backward compatibility)
    var authErr error
    token, authErr = p.authenticator.GetToken(ctx)
    if authErr != nil {
        p.GetLogger().ErrorLog("[ProviderName] Token retrieval failed: %v", authErr)
        return nil, fmt.Errorf("failed to get token: %w", authErr)
    }
    client = p.GetHTTPClient()
    tokenID = "fallback"
}
```

### Issues with Current Implementation

1. **Maintenance burden**: Changes to token selection require updates to 4 files
2. **Inconsistency risk**: One provider might have different logic
3. **Code bloat**: ~120 lines of duplicated code
4. **Error handling variance**: Different providers might handle errors differently
5. **Logging inconsistency**: Provider names are hardcoded in log messages

---

## How to Fix

### Step 1: Create Shared Token Selection

Create `provider/token_selection.go`:

```go
package provider

import (
    "context"
    "fmt"
    "net/http"
    
    "github.com/sunbankio/qwencoder-proxy/internal/token"
    "github.com/sunbankio/qwencoder-proxy/logging"
)

// TokenSelectionResult holds the result of a token selection operation.
//
// This struct encapsulates all the information needed to make an
// authenticated request with the selected token and its associated
// HTTP client (which may be configured with proxy settings).
type TokenSelectionResult struct {
    Client    *http.Client
    Token     string
    TokenID   string
    TokenInfo *token.ProviderToken
}

// TokenSelector defines the interface for selecting tokens.
//
// This is implemented by provider authenticators that need to
// select tokens for making authenticated requests.
type TokenSelector interface {
    GetTokenManager() *token.TokenManager
    GetLogger() logging.Logger
    GetHTTPClient() *http.Client
    GetToken(ctx context.Context) (string, error)
}

// SelectTokenWithClient selects a token and returns client, token, and token ID.
//
// This is a common pattern used across all providers. It handles:
// 1. Selecting a token from the token manager (if available)
// 2. Getting the associated HTTP client (which may be proxy-aware)
// 3. Falling back to authenticator if no token manager is available
// 4. Logging proxy usage for debugging
//
// Parameters:
//   - ctx: Context for the operation (used for authenticator.GetToken())
//   - providerName: Name of the provider (for logging)
//   - selector: TokenSelector interface (typically the provider's authenticator)
//   - defaultClient: Default HTTP client to use if token manager returns nil
//
// Returns:
//   - *TokenSelectionResult: Contains client, token, token ID, and token info
//   - error: Any error that occurred during token selection
func SelectTokenWithClient(
    ctx context.Context,
    providerName string,
    selector TokenSelector,
    defaultClient *http.Client,
) (*TokenSelectionResult, error) {
    logger := selector.GetLogger()
    tokenMgr := selector.GetTokenManager()
    
    if tokenMgr == nil {
        // No token manager, use authenticator and default client (backward compatibility)
        token, authErr := selector.GetToken(ctx)
        if authErr != nil {
            logger.ErrorLog("[%s] Token retrieval failed: %v", providerName, authErr)
            return nil, fmt.Errorf("failed to get token: %w", authErr)
        }
        
        return &TokenSelectionResult{
            Client:    defaultClient,
            Token:     token,
            TokenID:   "fallback",
            TokenInfo: nil,
        }, nil
    }

    // Use token manager for proxy-aware client selection
    selectedToken, selectedClient, selectErr := tokenMgr.SelectTokenWithClient()
    if selectErr != nil {
        logger.ErrorLog("[%s] Token selection failed: %v", providerName, selectErr)
        return nil, fmt.Errorf("failed to select token: %w", selectErr)
    }

    // Use provided default client if token manager returns nil
    if selectedClient == nil {
        logger.DebugLog("[%s] Token manager returned nil client, using default HTTP client", providerName)
        selectedClient = defaultClient
    }

    // Log proxy usage
    if selectedToken.Proxy != nil && selectedToken.Proxy.Type != "none" {
        logger.DebugLog("[%s] Using token %s with proxy: %s:%d", 
            providerName, selectedToken.ID, selectedToken.Proxy.Host, selectedToken.Proxy.Port)
    } else {
        logger.DebugLog("[%s] Using token %s with direct connection", providerName, selectedToken.ID)
    }

    return &TokenSelectionResult{
        Client:    selectedClient,
        Token:     selectedToken.AccessToken,
        TokenID:   selectedToken.ID,
        TokenInfo: selectedToken,
    }, nil
}
```

### Step 2: Update Provider Files

#### 2.1 Update `provider/gemini/gemini.go`

```go
// Add import:
import (
    // ... existing imports ...
    "github.com/sunbankio/qwencoder-proxy/provider"
)

// Update initializeProject method (lines 300-359):
func (p *Provider) initializeProject(ctx context.Context) error {
    p.GetLogger().DebugLog("[Gemini] Starting project initialization...")

    // Check if we have cached credentials
    if p.authenticator.IsAuthenticated() {
        p.GetLogger().DebugLog("[Gemini] Found valid cached credentials")
    } else {
        p.GetLogger().DebugLog("[Gemini] No valid cached credentials found")
    }

    // Use common token selection
    selection, err := provider.SelectTokenWithClient(
        ctx,
        "Gemini",
        p.authenticator,
        p.GetHTTPClient(),
    )
    if err != nil {
        p.projectInitError = err
        p.GetLogger().ErrorLog("[Gemini] Failed to get token for project initialization: %v", err)
        return fmt.Errorf("failed to get token for project initialization: %w", err)
    }

    client := selection.Client
    token := selection.Token
    tokenID := selection.TokenID

    p.GetLogger().DebugLog("[Gemini] Successfully obtained access token (length: %d)", len(token))

    // Clear any previous initialization errors since we got the token successfully
    p.projectInitError = nil

    // ... rest of function continues with client, token, tokenID ...
}
```

#### 2.2 Update `provider/iflow/iflow.go`

```go
// Add import:
import (
    // ... existing imports ...
    "github.com/sunbankio/qwencoder-proxy/provider"
)

// Update GenerateContent method (lines 251-290):
func (p *Provider) GenerateContent(ctx context.Context, model string, request interface{}) (interface{}, error) {
    // Use common token selection
    selection, err := provider.SelectTokenWithClient(
        ctx,
        "iFlow",
        p.authenticator,
        p.GetHTTPClient(),
    )
    if err != nil {
        return nil, err
    }

    client := selection.Client
    token := selection.Token
    tokenID := selection.TokenID

    tokenPrefix := token
    if len(token) > 20 {
        tokenPrefix = token[:20]
    }
    p.GetLogger().DebugLog("[iFlow] Using access token (first 20 chars): %s", tokenPrefix)

    // ... rest of function continues with client, token, tokenID ...
}

// Update GenerateContentStream method (lines 374-413):
func (p *Provider) GenerateContentStream(ctx context.Context, model string, request interface{}) (io.ReadCloser, error) {
    // Use common token selection
    selection, err := provider.SelectTokenWithClient(
        ctx,
        "iFlow",
        p.authenticator,
        p.GetHTTPClient(),
    )
    if err != nil {
        return nil, err
    }

    client := selection.Client
    token := selection.Token
    tokenID := selection.TokenID

    // ... rest of function continues with client, token, tokenID ...
}
```

#### 2.3 Update `provider/qwen/qwen.go`

```go
// Add import:
import (
    // ... existing imports ...
    "github.com/sunbankio/qwencoder-proxy/provider"
)

// Update GenerateContent method (lines 361-391):
func (p *Provider) GenerateContent(ctx context.Context, model string, request interface{}) (interface{}, error) {
    // Use common token selection
    selection, err := provider.SelectTokenWithClient(
        ctx,
        "Qwen",
        p.authenticator,
        p.GetHTTPClient(),
    )
    if err != nil {
        return nil, err
    }

    client := selection.Client
    token := selection.Token
    tokenID := selection.TokenID

    // Log the original request before conversion
    p.GetLogger().DebugLog("[Qwen] Original request before conversion: %+v", request)

    // ... rest of function continues with client, token, tokenID ...
}

// Update GenerateContentStream method (lines 451-481):
func (p *Provider) GenerateContentStream(ctx context.Context, model string, request interface{}) (io.ReadCloser, error) {
    // Use common token selection
    selection, err := provider.SelectTokenWithClient(
        ctx,
        "Qwen",
        p.authenticator,
        p.GetHTTPClient(),
    )
    if err != nil {
        return nil, err
    }

    client := selection.Client
    token := selection.Token
    tokenID := selection.TokenID

    // ... rest of function continues with client, token, tokenID ...
}
```

---

## Implementation Checklist

- [ ] Create `provider/token_selection.go`
- [ ] Update `provider/gemini/gemini.go`:
  - [ ] Add provider import
  - [ ] Update initializeProject() to use SelectTokenWithClient()
- [ ] Update `provider/iflow/iflow.go`:
  - [ ] Add provider import
  - [ ] Update GenerateContent() to use SelectTokenWithClient()
  - [ ] Update GenerateContentStream() to use SelectTokenWithClient()
- [ ] Update `provider/qwen/qwen.go`:
  - [ ] Add provider import
  - [ ] Update GenerateContent() to use SelectTokenWithClient()
  - [ ] Update GenerateContentStream() to use SelectTokenWithClient()
- [ ] Run existing tests
- [ ] Write unit tests for token_selection.go

---

## Testing

### Unit Tests for `provider/token_selection.go`

Create `provider/token_selection_test.go`:

```go
package provider

import (
    "context"
    "net/http"
    "testing"
    
    "github.com/sunbankio/qwencoder-proxy/internal/token"
    "github.com/sunbankio/qwencoder-proxy/logging"
)

// MockTokenSelector implements TokenSelector for testing
type MockTokenSelector struct {
    tokenManager *token.TokenManager
    logger       logging.Logger
    httpClient   *http.Client
    token        string
    tokenErr     error
}

func (m *MockTokenSelector) GetTokenManager() *token.TokenManager {
    return m.tokenManager
}

func (m *MockTokenSelector) GetLogger() logging.Logger {
    return m.logger
}

func (m *MockTokenSelector) GetHTTPClient() *http.Client {
    return m.httpClient
}

func (m *MockTokenSelector) GetToken(ctx context.Context) (string, error) {
    return m.token, m.tokenErr
}

func TestSelectTokenWithClient_WithTokenManager(t *testing.T) {
    ctx := context.Background()
    logger := logging.NewLogger()
    defaultClient := &http.Client{}
    
    // Create mock token manager
    mockTokenMgr := &MockTokenManager{
        token: &token.ProviderToken{
            ID:          "test-token-id",
            AccessToken:  "test-access-token",
            Proxy:        &token.ProxyConfig{Type: "none"},
        },
        client: &http.Client{},
    }
    
    selector := &MockTokenSelector{
        tokenManager: mockTokenMgr,
        logger:       logger,
        httpClient:   defaultClient,
    }
    
    result, err := SelectTokenWithClient(ctx, "TestProvider", selector, defaultClient)
    
    if err != nil {
        t.Fatalf("Unexpected error: %v", err)
    }
    
    if result.Token != "test-access-token" {
        t.Errorf("Expected token 'test-access-token', got '%s'", result.Token)
    }
    
    if result.TokenID != "test-token-id" {
        t.Errorf("Expected token ID 'test-token-id', got '%s'", result.TokenID)
    }
}

func TestSelectTokenWithClient_NoTokenManager(t *testing.T) {
    ctx := context.Background()
    logger := logging.NewLogger()
    defaultClient := &http.Client{}
    
    selector := &MockTokenSelector{
        tokenManager: nil,
        logger:       logger,
        httpClient:   defaultClient,
        token:        "fallback-token",
    }
    
    result, err := SelectTokenWithClient(ctx, "TestProvider", selector, defaultClient)
    
    if err != nil {
        t.Fatalf("Unexpected error: %v", err)
    }
    
    if result.Token != "fallback-token" {
        t.Errorf("Expected fallback token, got '%s'", result.Token)
    }
    
    if result.TokenID != "fallback" {
        t.Errorf("Expected fallback token ID, got '%s'", result.TokenID)
    }
    
    if result.Client != defaultClient {
        t.Error("Expected default client")
    }
}

func TestSelectTokenWithClient_TokenManagerError(t *testing.T) {
    ctx := context.Background()
    logger := logging.NewLogger()
    defaultClient := &http.Client{}
    
    // Create mock token manager that returns error
    mockTokenMgr := &MockTokenManager{
        selectError: fmt.Errorf("no tokens available"),
    }
    
    selector := &MockTokenSelector{
        tokenManager: mockTokenMgr,
        logger:       logger,
        httpClient:   defaultClient,
    }
    
    _, err := SelectTokenWithClient(ctx, "TestProvider", selector, defaultClient)
    
    if err == nil {
        t.Error("Expected error from token manager")
    }
    
    if !strings.Contains(err.Error(), "failed to select token") {
        t.Errorf("Expected token selection error, got: %v", err)
    }
}

func TestSelectTokenWithClient_AuthenticatorError(t *testing.T) {
    ctx := context.Background()
    logger := logging.NewLogger()
    defaultClient := &http.Client{}
    
    selector := &MockTokenSelector{
        tokenManager: nil,
        logger:       logger,
        httpClient:   defaultClient,
        tokenErr:     fmt.Errorf("authentication failed"),
    }
    
    _, err := SelectTokenWithClient(ctx, "TestProvider", selector, defaultClient)
    
    if err == nil {
        t.Error("Expected error from authenticator")
    }
    
    if !strings.Contains(err.Error(), "failed to get token") {
        t.Errorf("Expected get token error, got: %v", err)
    }
}
```

### Integration Tests
- Test all providers with shared token selection
- Verify proxy logging works correctly
- Test fallback behavior when token manager is nil

---

## Estimated Effort

| Task | Time |
|------|------|
| Create provider/token_selection.go | 30 minutes |
| Write unit tests for token_selection.go | 30 minutes |
| Update provider/gemini/gemini.go | 20 minutes |
| Update provider/iflow/iflow.go | 30 minutes |
| Update provider/qwen/qwen.go | 30 minutes |
| Run and fix tests | 30 minutes |
| **Total** | **3 hours** |

---

## Benefits

1. **Code reduction**: ~120 lines of duplicate code removed
2. **Single source of truth**: Token selection logic defined once
3. **Consistency**: All providers use identical token selection
4. **Testability**: Shared logic only needs to be tested once
5. **Maintainability**: Changes to token selection apply to all providers
6. **Logging consistency**: Provider names passed as parameters
