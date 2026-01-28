# Task 009: Extend Provider Interface for Token Manager Injection

## Description
Extend the Provider interface with an optional method for injecting TokenManager. This enables providers to use proxy-aware token selection for API requests.

## Technical Context
This task extends the Provider interface to support dependency injection of TokenManager.

**Current Provider Interface** ([`provider/provider.go:31`](provider/provider.go:31)):
```go
type Provider interface {
    Name() ProviderType
    Protocol() ProtocolType
    SupportedModels() []string
    SupportsModel(model string) bool
    GenerateContent(ctx context.Context, model string, request interface{}) (interface{}, error)
    GenerateContentStream(ctx context.Context, model string, request interface{}) (io.ReadCloser, error)
    ListModels(ctx context.Context) (interface{}, error)
    GetAuthenticator() Authenticator
    IsHealthy(ctx context.Context) bool
}
```

**Extended Provider Interface** (after this task):
```go
type Provider interface {
    Name() ProviderType
    Protocol() ProtocolType
    SupportedModels() []string
    SupportsModel(model string) bool
    GenerateContent(ctx context.Context, model string, request interface{}) (interface{}, error)
    GenerateContentStream(ctx context.Context, model string, request interface{}) (io.ReadCloser, error)
    ListModels(ctx context.Context) (interface{}, error)
    GetAuthenticator() Authenticator
    IsHealthy(ctx context.Context) bool
    SetTokenManager(manager *TokenManager)  // NEW - optional method
}
```

**Rationale for SetTokenManager()**:
- **Optional Method**: Not all providers need to implement immediately
- **Backward Compatible**: Existing providers work without this method
- **Dependency Injection**: Enables providers to use proxy-aware token selection
- **Type Assertion**: Factory can check if provider implements method before calling

**Affected Provider Implementations**:
- [`provider/gemini/gemini.go:36`](provider/gemini/gemini.go:36) - GeminiProvider
- [`provider/iflow/iflow.go`](provider/iflow/iflow.go) - IFlowProvider
- [`provider/kiro/kiro.go`](provider/kiro/kiro.go) - KiroProvider
- [`provider/qwen/qwen.go:34`](provider/qwen/qwen.go:34) - QwenProvider
- [`provider/antigravity/antigravity.go`](provider/antigravity/antigravity.go) - AntigravityProvider

**Architectural Principles**:
- **Interface Segregation**: New method is optional
- **Dependency Inversion**: Providers depend on TokenManager abstraction
- **Open/Closed**: Extends interface without breaking existing implementations

## Subtasks
1. Modify [`provider/provider.go`](provider/provider.go) Provider interface:
   - Add SetTokenManager(manager *TokenManager) method
   - Add documentation explaining optional nature
   - Add example usage in comments
2. Update all provider implementations to add SetTokenManager() method:
   - GeminiProvider ([`provider/gemini/gemini.go:36`](provider/gemini/gemini.go:36))
   - IFlowProvider ([`provider/iflow/iflow.go`](provider/iflow/iflow.go))
   - KiroProvider ([`provider/kiro/kiro.go`](provider/kiro/kiro.go))
   - QwenProvider ([`provider/qwen/qwen.go:34`](provider/qwen/qwen.go:34))
   - AntigravityProvider ([`provider/antigravity/antigravity.go`](provider/antigravity/antigravity.go))
3. For each provider:
   - Add `tokenManager *TokenManager` field
   - Implement SetTokenManager() to store reference
   - Add TODO comment for using tokenManager in subsequent tasks
4. Update [`provider/factory.go`](provider/factory.go) to inject TokenManager:
   - Add TokenManager field to Factory struct
   - Update NewFactory() to accept TokenManager parameter
   - After creating each provider, check if SetTokenManager exists
   - Call SetTokenManager() if method exists (type assertion)
5. Add unit tests for:
   - Interface extension (backward compatibility)
   - SetTokenManager() method implementation
   - Factory injection logic
   - Type assertion for optional method

## Dependencies
- Task 006: Add SelectTokenWithClient Method to TokenManager (uses TokenManager)

## Files to Modify
- `provider/provider.go` (extend Provider interface)
- `provider/gemini/gemini.go` (update GeminiProvider)
- `provider/iflow/iflow.go` (update IFlowProvider)
- `provider/kiro/kiro.go` (update KiroProvider)
- `provider/qwen/qwen.go` (update QwenProvider)
- `provider/antigravity/antigravity.go` (update AntigravityProvider)
- `provider/factory.go` (update Factory to inject TokenManager)

## Related Tasks
- Task 010: Update GeminiProvider for proxy-aware clients (uses tokenManager)
- Task 011: Update IFlowProvider for proxy-aware clients (uses tokenManager)
- Task 012: Update KiroProvider for proxy-aware clients (uses tokenManager)
- Task 013: Update QwenProvider for proxy-aware clients (uses tokenManager)
- Task 014: Update AntigravityProvider for proxy-aware clients (uses tokenManager)
