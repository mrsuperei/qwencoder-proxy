# Task 017: Update GeminiHandler for Proxy-Aware Token Selection

## Description
Update GeminiHandler to handle proxy-related errors and ensure proper error messages are returned to clients when proxy connections fail.

## Technical Context
This task updates the Gemini proxy handler to integrate with proxy-per-token architecture.

**Current GeminiHandler** ([`proxy/gemini_handler.go`](proxy/gemini_handler.go)):
```go
type GeminiHandler struct {
    factory      *provider.Factory
    convFactory  *converter.Factory
    logger       *logging.Logger
    // ... other fields
}
```

**New GeminiHandler** (after this task):
```go
type GeminiHandler struct {
    factory      *provider.Factory
    convFactory  *converter.Factory
    logger       *logging.Logger
    tokenManager *TokenManager  // NEW - for proxy error handling
}
```

**Integration Points**:
- Similar to OpenAIHandler pattern
- Uses [`proxy/common.go`](proxy/common.go) functions
- Handles Gemini-specific request/response format
- Streaming uses [`StreamConverter`](proxy/stream_converter.go)

**Error Handling for Proxy**:
- Detect proxy connection errors from provider responses
- Distinguish proxy errors from API errors
- Return appropriate error messages to clients
- Update proxy health on failures

**Request Flow**:
```
1. Client sends request to Gemini endpoint
2. Handler parses Gemini request
3. Handler calls provider.GenerateContent() or GenerateContentStream()
4. Provider uses proxy-aware client (from Task 010)
5. If proxy error occurs, handler formats error response
6. Error response includes proxy details and suggested actions
```

**Architectural Principles**:
- **Separation of Concerns**: Handler handles HTTP concerns, proxy errors handled by providers
- **Error Classification**: Distinguish proxy errors from API errors
- **User Experience**: Clear error messages for proxy failures

## Subtasks
1. Modify [`proxy/gemini_handler.go`](proxy/gemini_handler.go) GeminiHandler struct:
   - Add `tokenManager *TokenManager` field (optional, for error handling)
   - Update NewGeminiHandler() to accept tokenManager parameter
2. Update ServeHTTP() method:
   - Add error handling for proxy connection failures
   - Check error type for proxy-related errors
   - Return proxy-specific error messages
   - Update proxy health on proxy failures
3. Update request parsing:
   - Ensure proxy errors are caught after provider calls
   - Handle both streaming and non-streaming paths
4. Add error classification helper:
   - Create isProxyError(err error) bool function
   - Create getProxyErrorDetails(err error) map function
   - Create formatProxyError(err error) []byte function
5. Update error logging:
   - Log proxy connection errors with details
   - Log proxy timeout errors
   - Log proxy DNS errors
   - Mask proxy credentials in logs
6. Add unit tests for:
   - Proxy error detection
   - Proxy error response formatting
   - Non-streaming with proxy error
   - Streaming with proxy error
   - Error classification
   - Gemini-specific error handling

## Dependencies
- Task 006: Add SelectTokenWithClient Method to TokenManager (for error handling)
- Task 010: Update GeminiProvider for proxy-aware clients (provider uses proxy)

## Files to Modify
- `proxy/gemini_handler.go` (update GeminiHandler struct and methods)
- `proxy/gemini_handler_test.go` (add tests)

## Related Tasks
- Task 016: Update OpenAIHandler for proxy-aware token selection (similar pattern)
- Task 018: Update AnthropicHandler for proxy-aware token selection (similar pattern)
