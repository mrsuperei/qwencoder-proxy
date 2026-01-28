# Task 010: Update GeminiProvider for Proxy-Aware Clients

## Description
Refactor GeminiProvider to use proxy-aware HTTP clients from TokenManager instead of static httpClient field. This enables per-token proxy configuration for Gemini API requests.

## Technical Context
This task updates GeminiProvider to integrate with proxy-per-token architecture.

**Current GeminiProvider** ([`provider/gemini/gemini.go:36`](provider/gemini/gemini.go:36)):
```go
type GeminiProvider struct {
    httpClient    *http.Client  // Static client for all requests
    authenticator Authenticator
    logger        *logging.Logger
    projectID     string
    // ... other fields
}
```

**New GeminiProvider** (after this task):
```go
type GeminiProvider struct {
    tokenManager  *TokenManager  // NEW - for proxy-aware client selection
    authenticator Authenticator
    logger        *logging.Logger
    projectID     string
    // httpClient removed or marked as deprecated
}
```

**Integration Points**:
- Line 162: `p.authenticator.GetToken(ctx)` - remains unchanged
- Line 204, 390: `p.httpClient.Do(req)` - needs proxy-aware client
- Line 545: `p.httpClient.Do(req)` for streaming - needs proxy-aware client
- Line 152-314: `initializeProject()` - may need proxy-aware client

**Request Flow with Proxy**:
```go
func (p *GeminiProvider) GenerateContent(ctx context.Context, model string, request interface{}) (interface{}, error) {
    // Select token with proxy-aware client
    token, client, err := p.tokenManager.SelectTokenWithClient()
    if err != nil {
        return nil, err
    }

    // Create request
    req, _ := http.NewRequestWithContext(ctx, "POST", url, body)
    req.Header.Set("Authorization", "Bearer " + token.AccessToken)

    // Use proxy-aware client
    resp, err := client.Do(req)
    // ... process response
}
```

**Error Handling for Proxy**:
- Detect proxy connection errors (connection refused, timeout, DNS failures)
- Update proxy health on failures via TokenManager
- Distinguish proxy errors from API errors
- Return appropriate error messages

**Architectural Principles**:
- **Dependency Injection**: TokenManager injected via SetTokenManager()
- **Single Responsibility**: Provider handles content generation, proxy handled by factory
- **Open/Closed**: Extends provider without breaking existing functionality

## Subtasks
1. Modify [`provider/gemini/gemini.go:36`](provider/gemini/gemini.go:36) GeminiProvider struct:
   - Add `tokenManager *TokenManager` field
   - Remove or deprecate `httpClient *http.Client` field
   - Add SetTokenManager() method (from Task 009)
2. Update GenerateContent() method:
   - Call `p.tokenManager.SelectTokenWithClient()` to get token and client
   - Replace `p.httpClient.Do(req)` with `client.Do(req)`
   - Add error handling for proxy failures
   - Update proxy health on success/failure
3. Update GenerateContentStream() method:
   - Call `p.tokenManager.SelectTokenWithClient()` to get token and client
   - Replace `p.httpClient.Do(req)` with `client.Do(req)`
   - Add error handling for proxy failures during stream
   - Update proxy health on success/failure
4. Update initializeProject() method (lines 152-314):
   - Use proxy-aware client if needed
   - Handle proxy errors during project initialization
5. Update all other HTTP client usages:
   - Line 204: GenerateContent() request
   - Line 390: GenerateContent() request (retry path)
   - Line 545: GenerateContentStream() request
6. Add error classification:
   - Detect proxy connection errors
   - Detect proxy timeout errors
   - Detect proxy DNS errors
   - Update proxy health via TokenManager.UpdateProxyHealth()
7. Add logging for:
   - Token selection with proxy
   - Proxy-aware client usage
   - Proxy error events
   - Proxy health updates
8. Add unit tests for:
   - GenerateContent() with proxy
   - GenerateContent() without proxy
   - GenerateContentStream() with proxy
   - Proxy error handling
   - Proxy health updates
   - Backward compatibility

## Dependencies
- Task 006: Add SelectTokenWithClient Method to TokenManager (uses SelectTokenWithClient)
- Task 009: Extend Provider Interface for Token Manager Injection (implements SetTokenManager)

## Files to Modify
- `provider/gemini/gemini.go` (update GeminiProvider struct and methods)
- `provider/gemini/gemini_test.go` (update tests)

## Related Tasks
- Task 016: Update OpenAIHandler for proxy-aware token selection (uses updated provider)
- Task 017: Update GeminiHandler for proxy-aware token selection (uses updated provider)
