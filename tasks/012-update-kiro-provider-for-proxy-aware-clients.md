# Task 012: Update KiroProvider for Proxy-Aware Clients

## Description
Refactor KiroProvider to use proxy-aware HTTP clients from TokenManager instead of static httpClient field. This enables per-token proxy configuration for Kiro API requests.

## Technical Context
This task updates KiroProvider to integrate with proxy-per-token architecture.

**Current KiroProvider** ([`provider/kiro/kiro.go:58`](provider/kiro/kiro.go:58)):
```go
type KiroProvider struct {
    httpClient    *http.Client  // Static client for all requests
    authenticator Authenticator
    logger        *logging.Logger
    // ... other fields
}
```

**New KiroProvider** (after this task):
```go
type KiroProvider struct {
    tokenManager  *TokenManager  // NEW - for proxy-aware client selection
    authenticator Authenticator
    logger        *logging.Logger
    // httpClient removed or marked as deprecated
}
```

**Integration Points**:
- Line 164: `p.authenticator.GetToken(ctx)` - remains unchanged
- Line 206: `p.httpClient.Do(req)` for non-streaming - needs proxy-aware client
- Line 274: `p.httpClient.Do(req)` for streaming - needs proxy-aware client
- Line 289: `convertBedrockStreamToOpenAI()` - stream conversion

**Request Format**:
- Uses Kiro-specific format with conversation state
- Headers include AWS-specific: `x-amzn-kiro-agent-mode`, `x-amz-user-agent`
- Non-streaming: AWS Event Stream binary format parsed to Claude response
- Streaming: `convertBedrockStreamToOpenAI()` converts binary to OpenAI SSE

**Request Flow with Proxy**:
```go
func (p *KiroProvider) GenerateContent(ctx context.Context, model string, request interface{}) (interface{}, error) {
    // Select token with proxy-aware client
    token, client, err := p.tokenManager.SelectTokenWithClient()
    if err != nil {
        return nil, err
    }

    // Build Kiro request
    kiroReq := p.buildKiroRequest(model, request)

    // Create request
    req, _ := http.NewRequestWithContext(ctx, "POST", url, body)
    req.Header.Set("Authorization", "Bearer " + token.AccessToken)
    req.Header.Set("x-amzn-kiro-agent-mode", "enabled")

    // Use proxy-aware client
    resp, err := client.Do(req)
    // ... process response
}
```

**Error Handling for Proxy**:
- Detect proxy connection errors (connection refused, timeout, DNS failures)
- Update proxy health on failures via TokenManager
- Distinguish proxy errors from API errors
- Handle proxy errors during stream conversion

**Architectural Principles**:
- **Dependency Injection**: TokenManager injected via SetTokenManager()
- **Single Responsibility**: Provider handles content generation, proxy handled by factory
- **Open/Closed**: Extends provider without breaking existing functionality

## Subtasks
1. Modify [`provider/kiro/kiro.go:58`](provider/kiro/kiro.go:58) KiroProvider struct:
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
4. Update convertBedrockStreamToOpenAI() method (line 289):
   - Add error handling for proxy connection failures during stream
   - Ensure proxy errors are propagated correctly
5. Update all HTTP client usages:
   - Line 206: GenerateContent() request
   - Line 274: GenerateContentStream() request
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
   - Bedrock stream conversion with proxy
   - Proxy error handling
   - Proxy health updates
   - Backward compatibility

## Dependencies
- Task 006: Add SelectTokenWithClient Method to TokenManager (uses SelectTokenWithClient)
- Task 009: Extend Provider Interface for Token Manager Injection (implements SetTokenManager)

## Files to Modify
- `provider/kiro/kiro.go` (update KiroProvider struct and methods)
- `provider/kiro/kiro_test.go` (update tests)

## Related Tasks
- Task 016: Update OpenAIHandler for proxy-aware token selection (uses updated provider)
