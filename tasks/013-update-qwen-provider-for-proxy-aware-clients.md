# Task 013: Update QwenProvider for Proxy-Aware Clients

## Description
Refactor QwenProvider to use proxy-aware HTTP clients from TokenManager instead of static httpClient field. This enables per-token proxy configuration for Qwen API requests.

## Technical Context
This task updates QwenProvider to integrate with proxy-per-token architecture.

**Current QwenProvider** ([`provider/qwen/qwen.go:34`](provider/qwen/qwen.go:34)):
```go
type QwenProvider struct {
    httpClient    *http.Client  // Static client for all requests
    authenticator Authenticator
    logger        *logging.Logger
    // ... other fields
}
```

**New QwenProvider** (after this task):
```go
type QwenProvider struct {
    tokenManager  *TokenManager  // NEW - for proxy-aware client selection
    authenticator Authenticator
    logger        *logging.Logger
    // httpClient removed or marked as deprecated
}
```

**Integration Points**:
- Line 200: `qwenclient.GetValidTokenAndEndpoint()` - token retrieval
- Line 246: `p.httpClient.Do(req)` for non-streaming - needs proxy-aware client
- Line 312: `p.httpClient.Do(req)` for streaming - needs proxy-aware client

**Request Format**:
- Uses OpenAI-compatible format with custom headers
- Headers: `X-DashScope-AuthType: qwen-oauth`, `X-DashScope-UserAgent`
- Endpoint from credentials includes `/v1` path
- Returns OpenAI-compatible response directly

**Request Flow with Proxy**:
```go
func (p *QwenProvider) GenerateContent(ctx context.Context, model string, request interface{}) (interface{}, error) {
    // Select token with proxy-aware client
    token, client, err := p.tokenManager.SelectTokenWithClient()
    if err != nil {
        return nil, err
    }

    // Get endpoint from token
    endpoint, _ := qwenclient.GetValidTokenAndEndpoint(token.AccessToken)

    // Create request
    req, _ := http.NewRequestWithContext(ctx, "POST", endpoint, body)
    req.Header.Set("Authorization", "Bearer " + token.AccessToken)
    req.Header.Set("X-DashScope-AuthType", "qwen-oauth")

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
1. Modify [`provider/qwen/qwen.go:34`](provider/qwen/qwen.go:34) QwenProvider struct:
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
4. Update all HTTP client usages:
   - Line 246: GenerateContent() request
   - Line 312: GenerateContentStream() request
5. Add error classification:
   - Detect proxy connection errors
   - Detect proxy timeout errors
   - Detect proxy DNS errors
   - Update proxy health via TokenManager.UpdateProxyHealth()
6. Add logging for:
   - Token selection with proxy
   - Proxy-aware client usage
   - Proxy error events
   - Proxy health updates
7. Add unit tests for:
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
- `provider/qwen/qwen.go` (update QwenProvider struct and methods)
- `provider/qwen/qwen_test.go` (update tests)

## Related Tasks
- Task 016: Update OpenAIHandler for proxy-aware token selection (uses updated provider)
