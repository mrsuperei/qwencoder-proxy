# Task 002: Create HTTP Client Interface

## Description
Create an HTTPClient interface abstraction that enables dependency inversion and testability. This interface will be satisfied by the standard library's `*http.Client` and allow for proxy-aware wrapper implementations.

## Technical Context
This task implements a key recommendation from the SOLID Principles Architectural Analysis (Priority 1). The HTTP Client Interface is identified as:

- **High Impact on Testability**: Enables mocking HTTP behavior in tests
- **Foundation for Proxy-Per-Token**: Each token can use different HTTP client configuration
- **Dependency Inversion Principle**: High-level modules depend on abstraction, not concrete `*http.Client`

**Current Issues Addressed**:
- Scattered timeout configurations across providers (30s, 5min variations)
- No ability to intercept/mock HTTP calls
- Direct dependencies on `*http.Client` concrete type throughout codebase

**Affected Components** (will use this interface):
- [`provider/qwen/qwen.go:36`](provider/qwen/qwen.go:36) - `httpClient *http.Client`
- [`provider/gemini/gemini.go:39`](provider/gemini/gemini.go:39) - `httpClient *http.Client`
- [`restapi/rest_api.go:83`](restapi/rest_api.go:83) - `httpClient *http.Client`
- [`auth/multi_token_manager.go:39`](auth/multi_token_manager.go:39) - `httpClient *http.Client`
- [`auth/email_extraction.go:31`](auth/email_extraction.go:31) - All email extractors have `*http.Client`
- [`auth/token_refresh.go:390`](auth/token_refresh.go:390) - All refreshers have `*http.Client`

**Design Pattern**: Adapter pattern - existing `*http.Client` already satisfies the interface

## Subtasks
1. Create `config/http_client.go` file
2. Define HTTPClient interface with single method: `Do(req *http.Request) (*http.Response, error)`
3. Define HTTPClientConfig struct with timeout settings (Timeout, StreamingTimeout, IdleConnTimeout)
4. Add documentation explaining the interface purpose and usage
5. Add comment noting that `*http.Client` satisfies this interface
6. Create wrapper types documentation for future implementations:
   - StandardHTTPClient (wraps *http.Client)
   - RetryHTTPClient (with retry logic)
   - LoggingHTTPClient (with request/response logging)
7. Add unit tests demonstrating interface satisfaction by *http.Client
8. Add example usage in code comments

## Dependencies
- None (foundational task)

## Files to Create
- `config/http_client.go` (new file)
- `config/http_client_test.go` (new test file)

## Related Tasks
- Task 004: Create Proxy-Aware HTTP Client Factory (will use this interface)
- Task 003: Extend token metadata with proxy configuration (uses HTTP client concept)
