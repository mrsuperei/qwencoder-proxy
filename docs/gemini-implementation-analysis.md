# Gemini Implementation Analysis Report

## Executive Summary

This report analyzes the differences between the working Gemini CLI implementation (reference codebase) and the current qwencoder-proxy implementation to identify why some models are not working and why streaming is not functioning correctly.

## Key Findings

### 1. Missing Critical Headers

The current implementation is missing several important headers that the working implementation uses:

| Header | Working Implementation | Current Implementation | Impact |
|--------|----------------------|----------------------|--------|
| `User-Agent` | `"google-api-nodejs-client/9.15.1"` | Not set | High - API may reject requests without proper user agent |
| `X-Goog-Api-Client` | `"gl-node/22.17.0"` | Not set | High - Required for Google API compatibility |
| `Client-Metadata` | `"ideType=IDE_UNSPECIFIED,platform=PLATFORM_UNSPECIFIED,pluginType=GEMINI"` | Not set | Medium - May affect API behavior and model availability |

#### Working Implementation Header Function
```go
func applyGeminiCLIHeaders(r *http.Request) {
    misc.EnsureHeader(r.Header, ginHeaders, "User-Agent", "google-api-nodejs-client/9.15.1")
    misc.EnsureHeader(r.Header, ginHeaders, "X-Goog-Api-Client", "gl-node/22.17.0")
    misc.EnsureHeader(r.Header, ginHeaders, "Client-Metadata", geminiCLIClientMetadata())
}

func geminiCLIClientMetadata() string {
    return "ideType=IDE_UNSPECIFIED,platform=PLATFORM_UNSPECIFIED,pluginType=GEMINI"
}
```

### 2. Request Payload Structure Differences

#### Non-Streaming Requests

**Working Implementation:**
```go
// Uses wrapped structure with project and model at top level
{
    "project": "project-id",
    "model": "gemini-2.5-pro",
    "request": {
        // Actual Gemini request contents here
        "contents": [...],
        "generationConfig": {...}
    }
}
```

**Current Implementation:**
```go
// Uses wrapped structure (CORRECT for non-streaming)
{
    "model": "gemini-2.5-pro",
    "project": "project-id",
    "request": {
        // Actual Gemini request contents here
        "contents": [...],
        "generationConfig": {...}
    }
}
```

**Status:** ✅ Non-streaming payload structure is correct

#### Streaming Requests

**Working Implementation:**
```go
// Uses wrapped structure with project and model at top level
payload = setJSONField(payload, "project", projectID)
payload = setJSONField(payload, "model", attemptModel)
```

**Current Implementation:**
```go
// Sends request directly WITHOUT wrapper (INCORRECT)
finalRequest := requestMap  // Direct map, no wrapper
```

**Status:** ❌ Streaming payload structure is INCORRECT - missing `project` and `model` wrapper

### 3. URL Construction Differences

#### Non-Streaming URLs

**Working Implementation:**
```
https://cloudcode-pa.googleapis.com/v1internal:generateContent
```

**Current Implementation:**
```
https://cloudcode-pa.googleapis.com/v1internal:generateContent
```

**Status:** ✅ URLs are identical

#### Streaming URLs

**Working Implementation:**
```go
url := fmt.Sprintf("%s/%s:%s", codeAssistEndpoint, codeAssistVersion, "streamGenerateContent")
// Result: https://cloudcode-pa.googleapis.com/v1internal:streamGenerateContent?alt=sse
```

**Current Implementation:**
```go
url := fmt.Sprintf("%s:streamGenerateContent?alt=sse", p.baseURL)
// Result: https://cloudcode-pa.googleapis.com/v1internal:streamGenerateContent?alt=sse
```

**Status:** ✅ URLs are equivalent (both produce the same final URL)

### 4. Streaming Implementation Differences

#### Working Implementation Streaming Logic

```go
func (e *GeminiCLIExecutor) ExecuteStream(...) {
    // 1. Prepare payload WITH project and model wrapper
    payload := setJSONField(payload, "project", projectID)
    payload := setJSONField(payload, "model", attemptModel)

    // 2. Build URL with alt parameter
    url := fmt.Sprintf("%s/%s:%s", codeAssistEndpoint, codeAssistVersion, "streamGenerateContent")
    if opts.Alt == "" {
        url = url + "?alt=sse"
    } else {
        url = url + fmt.Sprintf("?$alt=%s", opts.Alt)
    }

    // 3. Set headers
    reqHTTP.Header.Set("Content-Type", "application/json")
    reqHTTP.Header.Set("Authorization", "Bearer "+tok.AccessToken)
    applyGeminiCLIHeaders(reqHTTP)  // ← CRITICAL: Sets User-Agent, X-Goog-Api-Client, Client-Metadata
    reqHTTP.Header.Set("Accept", "text/event-stream")

    // 4. Process SSE stream
    scanner := bufio.NewScanner(resp.Body)
    for scanner.Scan() {
        line := scanner.Bytes()
        if bytes.HasPrefix(line, dataTag) {
            // Process SSE data
        }
    }
}
```

#### Current Implementation Streaming Logic

```go
func (p *Provider) GenerateContentStream(...) {
    // 1. Prepare payload WITHOUT project and model wrapper (INCORRECT)
    finalRequest := requestMap  // Direct map, no wrapper

    // 2. Build URL
    url := fmt.Sprintf("%s:streamGenerateContent?alt=sse", p.baseURL)

    // 3. Set headers (MISSING critical headers)
    req.Header.Set("Authorization", "Bearer "+token)
    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("Accept", "text/event-stream")
    // Missing: User-Agent, X-Goog-Api-Client, Client-Metadata

    // 4. Return raw body
    return resp.Body, nil
}
```

**Critical Issues:**
1. ❌ Missing `project` and `model` in streaming payload
2. ❌ Missing `User-Agent` header
3. ❌ Missing `X-Goog-Api-Client` header
4. ❌ Missing `Client-Metadata` header

### 5. Model Fallback Mechanism

**Working Implementation:**
```go
models := cliPreviewFallbackOrder(baseModel)
for idx, attemptModel := range models {
    payload := setJSONField(payload, "model", attemptModel)
    // Try request
    if httpResp.StatusCode == 429 {
        if idx+1 < len(models) {
            log.Debugf("rate limited, retrying with next model: %s", models[idx+1])
            continue  // Try next model
        }
    }
}
```

**Current Implementation:**
```go
// No model fallback mechanism
// Only tries the requested model once
```

**Status:** ❌ No fallback mechanism - this explains why "some models are not working"

### 6. Error Handling

**Working Implementation:**
- Handles 429 (rate limit) by retrying with fallback models
- Parses `RetryInfo.retryDelay` from error responses
- Supports `ErrorInfo.metadata.quotaResetDelay` as fallback
- Parses quota reset time from error message

**Current Implementation:**
- Basic error handling
- Token refresh on 401
- No rate limit handling with fallback

## Root Cause Analysis

### Why Some Models Are Not Working

1. **Missing Headers:** The API may reject requests for certain models without proper `User-Agent`, `X-Goog-Api-Client`, and `Client-Metadata` headers.

2. **No Fallback Mechanism:** When a model returns 429 (rate limit) or other errors, the current implementation doesn't try alternative models.

3. **Streaming Payload Structure:** The streaming endpoint may require the same `project` and `model` wrapper structure as non-streaming requests.

### Why Streaming Is Not Working

1. **Incorrect Payload Structure:** Streaming requests are sent without the `project` and `model` wrapper, which the API expects.

2. **Missing Headers:** Critical headers required by the API are not being sent with streaming requests.

## Recommended Fixes

### Priority 1: Add Missing Headers

Create a helper function to apply the required headers:

```go
// applyGeminiHeaders sets required headers for Gemini API requests
func (p *Provider) applyGeminiHeaders(req *http.Request) {
    req.Header.Set("User-Agent", "google-api-nodejs-client/9.15.1")
    req.Header.Set("X-Goog-Api-Client", "gl-node/22.17.0")
    req.Header.Set("Client-Metadata", "ideType=IDE_UNSPECIFIED,platform=PLATFORM_UNSPECIFIED,pluginType=GEMINI")
}
```

Apply these headers in both [`GenerateContent()`](qwencoder-proxy/provider/gemini/gemini.go:509) and [`GenerateContentStream()`](qwencoder-proxy/provider/gemini/gemini.go:704).

### Priority 2: Fix Streaming Payload Structure

Update [`GenerateContentStream()`](qwencoder-proxy/provider/gemini/gemini.go:704) to wrap the request with `project` and `model`:

```go
// In GenerateContentStream(), replace:
// finalRequest := requestMap

// With:
finalRequest := map[string]interface{}{
    "model":   model,
    "project": p.projectID,
    "request": requestMap,
}
```

### Priority 3: Add Model Fallback Mechanism

Implement a fallback mechanism similar to the working implementation:

```go
// Define fallback models
func getGeminiFallbackModels(baseModel string) []string {
    switch baseModel {
    case "gemini-2.5-pro":
        return []string{"gemini-2.5-pro-preview-06-05"}
    case "gemini-2.5-flash":
        return []string{"gemini-2.5-flash-preview-09-2025"}
    default:
        return nil
    }
}

// In GenerateContent() and GenerateContentStream(), implement retry logic:
models := append([]string{model}, getGeminiFallbackModels(model)...)
for _, attemptModel := range models {
    finalRequest["model"] = attemptModel
    // Try request
    if resp.StatusCode == http.StatusTooManyRequests {
        continue  // Try next model
    }
    // Success or non-429 error - break
    break
}
```

### Priority 4: Improve Error Handling

Add rate limit delay parsing:

```go
func parseRetryDelay(errorBody []byte) (*time.Duration, error) {
    // Parse RetryInfo.retryDelay from error response
    // Parse ErrorInfo.metadata.quotaResetDelay as fallback
    // Parse from error message as last resort
}
```

## Implementation Roadmap

1. **Phase 1: Headers (Quick Win)**
   - Add `applyGeminiHeaders()` helper function
   - Apply headers to all requests
   - Test with non-working models

2. **Phase 2: Streaming Fix**
   - Update streaming payload structure
   - Apply headers to streaming requests
   - Test streaming functionality

3. **Phase 3: Fallback Mechanism**
   - Implement model fallback logic
   - Add retry loop for 429 errors
   - Test with rate-limited scenarios

4. **Phase 4: Enhanced Error Handling**
   - Add retry delay parsing
   - Implement exponential backoff
   - Improve error messages

## Testing Recommendations

1. Test each model individually with the new headers
2. Test streaming with the corrected payload structure
3. Test rate limit handling with fallback models
4. Compare responses between working and fixed implementations

## Conclusion

The primary issues are:
1. **Missing critical headers** (`User-Agent`, `X-Goog-Api-Client`, `Client-Metadata`)
2. **Incorrect streaming payload structure** (missing `project` and `model` wrapper)
3. **No model fallback mechanism** for rate-limited models

Implementing the recommended fixes, starting with the headers, should resolve most of the issues with non-working models and streaming functionality.
