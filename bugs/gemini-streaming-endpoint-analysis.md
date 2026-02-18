# Gemini Streaming Endpoint Analysis

## Current Streaming Endpoint

The streaming endpoint being used is:

```
https://cloudcode-pa.googleapis.com/v1internal:streamGenerateContent?alt=sse
```

**Location:** [`provider/gemini/gemini.go:758`](provider/gemini/gemini.go:758)

```go
// Add the crucial alt=sse query parameter for streaming
url := fmt.Sprintf("%s:streamGenerateContent?alt=sse", p.baseURL)
```

## Non-Streaming Endpoint

The non-streaming endpoint is:

```
https://cloudcode-pa.googleapis.com/v1internal:generateContent
```

**Location:** [`provider/gemini/gemini.go:580`](provider/gemini/gemini.go:580)

```go
url := fmt.Sprintf("%s:generateContent", p.baseURL)
```

## Base URL Configuration

The base URL is defined as:

```go
const (
    // DefaultBaseURL is the default Gemini API base URL
    DefaultBaseURL = "https://cloudcode-pa.googleapis.com/v1internal"
)
```

**Location:** [`provider/gemini/gemini.go:20-22`](provider/gemini/gemini.go:20-22)

## Current Error Analysis

### Error Message

```
2026/02/18 15:19:45 ⚠️  [ERROR] [Handler] Streaming error: API error (status 429): {
  "error": {
    "code": 429,
    "message": "No capacity available for model gemini-2.5-pro on the server",
    "status": "RESOURCE_EXHAUSTED",
    "details": [
      {
        "@type": "type.googleapis.com/google.rpc.ErrorInfo",
        "reason": "MODEL_CAPACITY_EXHAUSTED",
        "domain": "cloudcode-pa.googleapis.com",
        "metadata": {
          "model": "gemini-2.5-pro"
        }
      }
    ]
  }
}
```

### Analysis

The error has changed from the original **HTTP 400 "INVALID_ARGUMENT"** to **HTTP 429 "RESOURCE_EXHAUSTED"**. This indicates that:

1. **The fix is working:** The request format is now correct (no more "Unknown name" errors about `contents` and `systemInstruction` fields)
2. **New issue:** The server is rejecting the request due to capacity constraints

### Error Types Comparison

| Error | Status | Meaning | Status |
|-------|--------|---------|--------|
| Original | 400 INVALID_ARGUMENT | Invalid JSON payload - request format was wrong | ❌ Fixed |
| Current | 429 RESOURCE_EXHAUSTED | No capacity available for model | ⚠️ Server-side issue |

## Endpoint Summary

| Type | Endpoint | Query Params | Status |
|------|----------|--------------|--------|
| Non-Streaming | `https://cloudcode-pa.googleapis.com/v1internal:generateContent` | None | ✅ Working |
| Streaming | `https://cloudcode-pa.googleapis.com/v1internal:streamGenerateContent?alt=sse` | `alt=sse` | ⚠️ Capacity issue |

## Next Steps

The 429 error is a server-side capacity issue and not a code issue. Possible solutions:

1. **Wait and retry:** The capacity issue may be temporary
2. **Try a different model:** Use `gemini-2.0-flash` or another model with available capacity
3. **Check quota:** Verify if there are rate limits or quota constraints
4. **Contact support:** If this is a persistent issue, it may require investigation by the Cloud Code Assist team

## Related Files

- [`provider/gemini/gemini.go`](provider/gemini/gemini.go) - Gemini provider implementation
- [`provider/gemini/gemini.go:20-22`](provider/gemini/gemini.go:20-22) - Base URL constant
- [`provider/gemini/gemini.go:580`](provider/gemini/gemini.go:580) - Non-streaming endpoint
- [`provider/gemini/gemini.go:758`](provider/gemini/gemini.go:758) - Streaming endpoint
