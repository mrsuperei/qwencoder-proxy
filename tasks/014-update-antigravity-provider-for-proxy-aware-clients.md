# Task 014: Update AntigravityProvider for Proxy-Aware Clients

## Description
Refactor AntigravityProvider to use proxy-aware HTTP clients from TokenManager instead of static httpClient field. This enables per-token proxy configuration for Antigravity API requests.

## Technical Context
This task updates AntigravityProvider to integrate with proxy-per-token architecture.

**Current AntigravityProvider** ([`provider/antigravity/antigravity.go:65`](provider/antigravity/antigravity.go:65)):
```go
type AntigravityProvider struct {
    httpClient    *http.Client  // Static client for all requests
    authenticator Authenticator
    logger        *logging.Logger
    // ... other fields
}
```

**New AntigravityProvider** (after this task):
```go
type AntigravityProvider struct {
    tokenManager  *TokenManager  // NEW - for proxy-aware client selection
    authenticator Authenticator
    logger        *logging.Logger
    // httpClient removed or marked as deprecated
}
```

**Integration Points**:
- Line 175: `p.authenticator.GetToken(ctx)` - remains unchanged
- Line 740: `p.httpClient.Do(req)` via `doRequestWithRetry()` - needs proxy-aware client
- Line 833: `p.httpClient.Do(req)` for streaming - needs proxy-aware client
- Line 173: `doRequestWithRetry()` - handles 401 retry
- Line 152-314: `initializeProject()` equivalent - `geminiToAntigravity()`

**Request Format**:
- Uses Antigravity-specific format with project/session IDs
- Headers: `User-Agent: antigravity/1.11.5`
- Model alias mapping for internal names
- Returns Gemini-compatible response (wrapped in `{"response": {...}}`)

**Request Flow with Proxy**:
```go
func (p *AntigravityProvider) GenerateContent(ctx context.Context, model string, request interface{}) (interface{}, error) {
    // Select token with proxy-aware client
    token, client, err := p.tokenManager.SelectTokenWithClient()
    if err != nil {
        return nil, err
    }

    // Transform request
    agReq := p.geminiToAntigravity(model, request)

    // Create request
    req, _ := http.NewRequestWithContext(ctx, "POST", url, body)
    req.Header.Set("Authorization", "Bearer " + token.AccessToken)
    req.Header.Set("User-Agent", "antigravity/1.11.5")

    // Use proxy-aware client
    resp, err := client.Do(req)
    // ... process response
}
```

**Error Handling for Proxy**:
- Detect proxy connection errors (connection refused, timeout, DNS failures)
- Update proxy health on failures via TokenManager
- Distinguish proxy errors from API errors
- Handle proxy errors during retry logic

**Architectural Principles**:
- **Dependency Injection**: TokenManager injected via SetTokenManager()
- **Single Responsibility**: Provider handles content generation, proxy handled by factory
- **Open/Closed**: Extends provider without breaking existing functionality

## Subtasks
1. Modify [`provider/antigravity/antigravity.go:65`](provider/antigravity/antigravity.go:65) AntigravityProvider struct:
   - Add `tokenManager *TokenManager` field
   - Remove or deprecate `httpClient *http.Client` field
   - Add SetTokenManager() method (from Task 009)
2. Update GenerateContent() method:
   - Call `p.tokenManager.SelectTokenWithClient()` to get token and client
   - Replace `p.httpClient.Do(req)` with `client.Do(req)` in doRequestWithRetry()
   - Add error handling for proxy failures
   - Update proxy health on success/failure
3. Update GenerateContentStream() method:
   - Call `p.tokenManager.SelectTokenWithClient()` to get token and client
   - Replace `p.httpClient.Do(req)` with `client.Do(req)`
   - Add error handling for proxy failures during stream
   - Update proxy health on success/failure
4. Update doRequestWithRetry() method (line 173):
   - Use proxy-aware client from parameter
   - Handle proxy errors during retry
   - Update proxy health on 401 retry failures
5. Update callAPI() method:
   - Use proxy-aware client
   - Handle proxy errors
6. Update all HTTP client usages:
   - Line 740: doRequestWithRetry() via GenerateContent()
   - Line 833: GenerateContentStream() request
7. Add error classification:
   - Detect proxy connection errors
   - Detect proxy timeout errors
   - Detect proxy DNS errors
   - Update proxy health via TokenManager.UpdateProxyHealth()
8. Add logging for:
   - Token selection with proxy
   - Proxy-aware client usage
   - Proxy error events
   - Proxy health updates
9. Add unit tests for:
   - GenerateContent() with proxy
   - GenerateContent() without proxy
   - GenerateContentStream() with proxy
   - doRequestWithRetry() with proxy
   - Proxy error handling
   - Proxy health updates
   - Backward compatibility

## Dependencies
- Task 006: Add SelectTokenWithClient Method to TokenManager (uses SelectTokenWithClient)
- Task 009: Extend Provider Interface for Token Manager Injection (implements SetTokenManager)

## Files to Modify
- `provider/antigravity/antigravity.go` (update AntigravityProvider struct and methods)
- `provider/antigravity/antigravity_test.go` (update tests)

## Related Tasks
- Task 016: Update OpenAIHandler for proxy-aware token selection (uses updated provider)
