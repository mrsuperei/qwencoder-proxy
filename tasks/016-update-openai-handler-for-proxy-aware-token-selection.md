# Task 016: Update OpenAIHandler for Proxy-Aware Token Selection

## Description
Update OpenAIHandler to handle proxy-related errors and ensure proper error messages are returned to clients when proxy connections fail.

## Technical Context
This task updates the proxy layer handlers to integrate with proxy-per-token architecture.

**Current OpenAIHandler** ([`proxy/openai_handler.go:43`](proxy/openai_handler.go:43)):
```go
type OpenAIHandler struct {
    factory      *provider.Factory
    convFactory  *converter.Factory
    logger       *logging.Logger
    // ... other fields
}
```

**New OpenAIHandler** (after this task):
```go
type OpenAIHandler struct {
    factory      *provider.Factory
    convFactory  *converter.Factory
    logger       *logging.Logger
    tokenManager *TokenManager  // NEW - for proxy error handling
}
```

**Integration Points**:
- Line 165: `handleChatCompletions()` - main entry point
- Line 191: `handleNonStreamCompletions()` - non-streaming path
- Line 218: `needsStreamConversion()` - checks if stream conversion needed
- Line 149: `ConvertedStreamResponse()` - streaming with conversion
- Line 60: `StreamResponse()` - raw streaming

**Error Handling for Proxy**:
- Detect proxy connection errors from provider responses
- Distinguish proxy errors from API errors
- Return appropriate error messages to clients
- Update proxy health on failures

**Error Response Format**:
```json
{
  "error": "proxy_connection_failed",
  "message": "Failed to connect to proxy proxy.example.com:1080",
  "details": {
    "proxy_type": "socks5",
    "proxy_host": "proxy.example.com",
    "proxy_port": 1080,
    "original_error": "dial tcp: lookup proxy.example.com: no such host"
  },
  "suggested_action": "Check proxy configuration or disable proxy for this token"
}
```

**Architectural Principles**:
- **Separation of Concerns**: Handler handles HTTP concerns, proxy errors handled by providers
- **Error Classification**: Distinguish proxy errors from API errors
- **User Experience**: Clear error messages for proxy failures

## Subtasks
1. Modify [`proxy/openai_handler.go:43`](proxy/openai_handler.go:43) OpenAIHandler struct:
   - Add `tokenManager *TokenManager` field (optional, for error handling)
   - Update NewOpenAIHandler() to accept tokenManager parameter
2. Update handleChatCompletions() method:
   - Add error handling for proxy connection failures
   - Check error type for proxy-related errors
   - Return proxy-specific error messages
   - Update proxy health on proxy failures
3. Update handleNonStreamCompletions() method:
   - Add error handling for proxy failures
   - Distinguish proxy errors from API errors
   - Return appropriate error responses
4. Update streaming error handling:
   - Add proxy error detection in stream conversion
   - Handle proxy errors during streaming
   - Close stream on proxy failure
   - Update proxy health
5. Add error classification helper:
   - Create isProxyError(err error) bool function
   - Create getProxyErrorDetails(err error) map function
   - Create formatProxyError(err error) []byte function
6. Update error logging:
   - Log proxy connection errors with details
   - Log proxy timeout errors
   - Log proxy DNS errors
   - Mask proxy credentials in logs
7. Add unit tests for:
   - Proxy error detection
   - Proxy error response formatting
   - Non-streaming with proxy error
   - Streaming with proxy error
   - Error classification

## Dependencies
- Task 006: Add SelectTokenWithClient Method to TokenManager (for error handling)
- Task 009: Extend Provider Interface for Token Manager Injection (providers use tokenManager)

## Files to Modify
- `proxy/openai_handler.go` (update OpenAIHandler struct and methods)
- `proxy/openai_handler_test.go` (add tests)

## Related Tasks
- Task 017: Update GeminiHandler for proxy-aware token selection (similar pattern)
- Task 018: Update AnthropicHandler for proxy-aware token selection (similar pattern)
