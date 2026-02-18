# Gemini Streaming 429 Error Analysis

## Issue Summary

Streaming requests to Gemini models fail with HTTP 429 "RESOURCE_EXHAUSTED" error, while non-streaming requests work correctly. Qwen models continue to work fine with streaming enabled.

**Error Message:**
```
[ERROR] [Handler] Streaming error: API error (status 429): {
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

**Symptoms:**
- ✅ Non-streaming Gemini requests work fine
- ❌ Streaming Gemini requests fail with 429 error
- ✅ Qwen streaming works correctly
- ⚠️ Issue appeared after a recent refactor

---

## Root Cause Analysis

### Location of the Bug

**File:** `provider/gemini/gemini.go`
**Method:** `GenerateContentStream` (lines 679-841)

### The Problem

The `GenerateContentStream` method incorrectly wraps the Gemini request in a Cloud Code Assist API format before sending it to the streaming endpoint. The streaming endpoint `:streamGenerateContent?alt=sse` does not support this wrapper format, causing the API to reject the request with a 429 error.

### Code Evidence

**Broken Code (Lines 726-758 in provider/gemini/gemini.go):**

```go
// Prepare the request body with model information for Cloud Code Assist API
requestMap, ok := request.(map[string]interface{})
if !ok {
    // If it's not a map, try to convert it from a GeminiRequest struct
    jsonBytes, marshalErr := json.Marshal(request)
    if marshalErr != nil {
        return nil, fmt.Errorf("failed to marshal request: %w", marshalErr)
    }
    if err := json.Unmarshal(jsonBytes, &requestMap); err != nil {
        return nil, fmt.Errorf("failed to unmarshal request: %w", err)
    }
}

// Check if this is already a Cloud Code Assist API formatted request
_, hasModel := requestMap["model"]
_, hasProject := requestMap["project"]
_, hasRequest := requestMap["request"]

// Prepare the final request structure based on the format
var finalRequest map[string]interface{}
if hasModel && hasProject && hasRequest {
    // This is already a Cloud Code Assist API formatted request
    requestMap["project"] = p.projectID
    finalRequest = requestMap
} else {
    // This is a standard Gemini API request, format it for Cloud Code Assist API
    finalRequest = map[string]interface{}{
        "model":   model,
        "project": p.projectID,
        "request": requestMap,
    }
}

// Marshal the request
reqBody, marshalErr := json.Marshal(finalRequest)
if marshalErr != nil {
    return nil, fmt.Errorf("failed to marshal request: %w", marshalErr)
}

// Add the crucial alt=sse query parameter for streaming
url := fmt.Sprintf("%s:streamGenerateContent?alt=sse", p.baseURL)
req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(reqBody))
```

**The Issue:** The `finalRequest` is wrapped with `{"model": ..., "project": ..., "request": ...}` structure, which is NOT compatible with the `:streamGenerateContent?alt=sse` endpoint.

---

## Why Non-Streaming Works

The non-streaming method `GenerateContent` (lines 488-676) uses the **same wrapper format** but sends it to the `:generateContent` endpoint, which **does support** the Cloud Code Assist API wrapper format.

**Working Code (Lines 580-587 in provider/gemini/gemini.go):**

```go
url := fmt.Sprintf("%s:generateContent", p.baseURL)
req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(reqBody))
if err != nil {
    return nil, fmt.Errorf("failed to create request: %w", err)
}

req.Header.Set("Authorization", "Bearer "+token)
req.Header.Set("Content-Type", "application/json")
```

The `:generateContent` endpoint accepts the Cloud Code Assist API wrapper format, while `:streamGenerateContent?alt=sse` does not.

---

## Why Qwen Streaming Works

The Qwen provider in `provider/qwen/qwen.go` (lines 451-525) does **not** use any wrapper format. It sends the request directly to the `/chat/completions` endpoint.

**Working Code (Lines 484-506 in provider/qwen/qwen.go):**

```go
// Convert request to proper format for Qwen API
reqBody, err := json.Marshal(request)
if err != nil {
    return nil, fmt.Errorf("failed to marshal request: %w", err)
}

// Use the default endpoint from qwen package
endpoint := DefaultBaseURL
url := fmt.Sprintf("%s/chat/completions", endpoint)
p.GetLogger().DebugLog("[Qwen] Constructed streaming target URL: %s", url)

req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(reqBody))
if err != nil {
    return nil, fmt.Errorf("failed to create request: %w", err)
}

// Set the required headers for Qwen API
req.Header.Set("Authorization", "Bearer "+token)
req.Header.Set("Content-Type", "application/json")
req.Header.Set("Accept", "text/event-stream")
req.Header.Set("X-DashScope-AuthType", "qwen-oauth")
req.Header.Set("X-DashScope-UserAgent", fmt.Sprintf("QwenCode/0.0.10 (%s; %s)", runtime.GOOS, runtime.GOARCH))
req.Header.Set("X-DashScope-CacheControl", "enable")
```

Qwen sends the request directly without any wrapper, which is why streaming works correctly.

---

## Detailed Flow Analysis

### Streaming Request Flow (Broken)

1. **Client Request:** OpenAI format request with `"stream": true`
   ```json
   {
     "model": "gemini-2.5-pro",
     "messages": [{"role": "user", "content": "Hello!"}],
     "stream": true
   }
   ```

2. **Handler:** `handleChatCompletions` in `proxy/openai_handler.go` (line 152)

3. **Converter:** `FromOpenAIRequest` in `converter/gemini.go` (line 166) converts to Gemini format
   ```json
   {
     "contents": [{"role": "user", "parts": [{"text": "Hello!"}]}]
   }
   ```

4. **Provider:** `GenerateContentStream` in `provider/gemini/gemini.go` (line 679)

5. **Wrapper Applied:** Request wrapped in Cloud Code Assist API format (lines 746-758)
   ```json
   {
     "model": "gemini-2.5-pro",
     "project": "projects/abc123",
     "request": {
       "contents": [{"role": "user", "parts": [{"text": "Hello!"}]}]
     }
   }
   ```

6. **API Call:** Sent to `:streamGenerateContent?alt=sse` endpoint (line 768)
   ```
   POST https://cloudcode-pa.googleapis.com/v1internal:streamGenerateContent?alt=sse
   ```

7. **Error:** API returns 429 "RESOURCE_EXHAUSTED" because it doesn't recognize the wrapped format

### Non-Streaming Request Flow (Working)

1. **Client Request:** OpenAI format request without `"stream"` parameter
   ```json
   {
     "model": "gemini-2.5-pro",
     "messages": [{"role": "user", "content": "Hello!"}]
   }
   ```

2. **Handler:** `handleChatCompletions` in `proxy/openai_handler.go` (line 152)

3. **Converter:** `FromOpenAIRequest` in `converter/gemini.go` (line 166) converts to Gemini format

4. **Provider:** `GenerateContent` in `provider/gemini/gemini.go` (line 488)

5. **Wrapper Applied:** Request wrapped in Cloud Code Assist API format (lines 567-571)
   ```json
   {
     "model": "gemini-2.5-pro",
     "project": "projects/abc123",
     "request": {
       "contents": [{"role": "user", "parts": [{"text": "Hello!"}]}]
     }
   }
   ```

6. **API Call:** Sent to `:generateContent` endpoint (line 580)
   ```
   POST https://cloudcode-pa.googleapis.com/v1internal:generateContent
   ```

7. **Success:** API processes request correctly and returns response

---

## Key Differences Summary

| Aspect | Non-Streaming (Working) | Streaming (Broken) | Qwen Streaming (Working) |
|---------|------------------------|-------------------|---------------------------|
| **Endpoint** | `:generateContent` | `:streamGenerateContent?alt=sse` | `/chat/completions` |
| **Request Format** | Cloud Code Assist wrapper | Cloud Code Assist wrapper ❌ | Direct (no wrapper) ✅ |
| **Accept Header** | `application/json` | `text/event-stream` | `text/event-stream` |
| **Result** | Success ✅ | 429 Error ❌ | Success ✅ |

---

## Recommended Fix

### Primary Solution: Remove Cloud Code Assist Wrapper for Streaming

Modify the `GenerateContentStream` method in `provider/gemini/gemini.go` to send the request directly without the Cloud Code Assist API wrapper.

**Location:** `provider/gemini/gemini.go`, lines 726-758

**Current Code (Broken):**
```go
// Lines 746-758
finalRequest = map[string]interface{}{
    "model":   model,
    "project": p.projectID,
    "request": requestMap,
}
```

**Proposed Fix:**
```go
// Send request directly without wrapper for streaming
finalRequest = requestMap
```

### Implementation Details

The fix should be applied in the `GenerateContentStream` method around lines 746-758. Instead of wrapping the request in the Cloud Code Assist API format, the method should send the request directly to the streaming endpoint.

**Modified Code Section:**
```go
// Lines 726-758 in provider/gemini/gemini.go
// Prepare the request body with model information for Cloud Code Assist API
requestMap, ok := request.(map[string]interface{})
if !ok {
    // If it's not a map, try to convert it from a GeminiRequest struct
    jsonBytes, marshalErr := json.Marshal(request)
    if marshalErr != nil {
        return nil, fmt.Errorf("failed to marshal request: %w", marshalErr)
    }
    if err := json.Unmarshal(jsonBytes, &requestMap); err != nil {
        return nil, fmt.Errorf("failed to unmarshal request: %w", err)
    }
}

// FOR STREAMING: Send request directly without Cloud Code Assist wrapper
// The :streamGenerateContent?alt=sse endpoint does not support the wrapper format
finalRequest := requestMap

// Marshal the request
reqBody, marshalErr := json.Marshal(finalRequest)
if marshalErr != nil {
    return nil, fmt.Errorf("failed to marshal request: %w", marshalErr)
}

// Add the crucial alt=sse query parameter for streaming
url := fmt.Sprintf("%s:streamGenerateContent?alt=sse", p.baseURL)
```

### Alternative Solutions

#### Option 1: Use Native Gemini Streaming Endpoint

Research if there's a different streaming endpoint that supports the Cloud Code Assist API wrapper format. The current `:streamGenerateContent?alt=sse` endpoint may not be the correct one for wrapped requests.

#### Option 2: Conditional Wrapper Application

Apply the Cloud Code Assist wrapper only for non-streaming requests and send streaming requests directly:

```go
// In GenerateContentStream (lines 726-758)
// Always send streaming requests directly without wrapper
finalRequest := requestMap

// In GenerateContent (lines 567-571)
// Keep the wrapper for non-streaming requests
finalRequest = map[string]interface{}{
    "model":   model,
    "project": p.projectID,
    "request": requestMap,
}
```

#### Option 3: Different Streaming Format

Investigate if the Cloud Code Assist API has a specific streaming format or endpoint that should be used instead of `:streamGenerateContent?alt=sse`.

---

## Testing Strategy

### Test Cases

1. **Basic Streaming Test**
   - Model: `gemini-2.5-pro`
   - Request: Simple one-turn conversation
   - Expected: Successful streaming response

2. **Multi-turn Streaming Test**
   - Model: `gemini-2.5-flash`
   - Request: Multi-turn conversation with history
   - Expected: Successful streaming response with context awareness

3. **System Message Test**
   - Model: `gemini-2.5-pro`
   - Request: Conversation with system message
   - Expected: Successful streaming response respecting system instructions

4. **Multiple Models Test**
   - Test all supported Gemini models:
     - `gemini-2.5-pro`
     - `gemini-2.5-flash`
     - `gemini-2.5-flash-lite`
     - `gemini-2.5-pro-preview-06-05`
     - `gemini-2.5-flash-preview-09-2025`
     - `gemini-3-pro-preview`
     - `gemini-3-flash-preview`

5. **Stream Conversion Test**
   - Verify that `proxy/stream_converter.go` properly handles the response format
   - Ensure SSE chunks are correctly converted to OpenAI format
   - Test that `[DONE]` message is sent at the end

6. **Error Handling Test**
   - Test with invalid requests to ensure proper error handling
   - Verify that proxy errors are correctly identified and handled
   - Test token refresh on 401 errors

### Validation Steps

1. **Functional Validation**
   - Streaming responses are received in real-time
   - Content is correctly assembled from chunks
   - No data loss or corruption in streaming

2. **Format Validation**
   - SSE format is correct (`data: {...}\n\n`)
   - OpenAI format is properly applied to chunks
   - Final `[DONE]` message is sent

3. **Performance Validation**
   - Streaming latency is acceptable
   - No memory leaks or resource exhaustion
   - Multiple concurrent streaming requests work correctly

---

## Related Files

### Core Implementation Files

1. **`provider/gemini/gemini.go`**
   - Line 679-841: `GenerateContentStream` method (contains the bug)
   - Line 488-676: `GenerateContent` method (working reference)
   - Line 280-485: `initializeProject` method (project initialization)

2. **`provider/gemini/types.go`**
   - Line 5-12: `GeminiRequest` type definition
   - Line 86-91: `GeminiResponse` type definition
   - Line 140-144: `StreamChunk` type definition

3. **`provider/qwen/qwen.go`**
   - Line 451-525: `GenerateContentStream` method (working reference)
   - Line 361-448: `GenerateContent` method (working reference)

4. **`converter/gemini.go`**
   - Line 166-271: `FromOpenAIRequest` method (request conversion)
   - Line 94-163: `ToOpenAIStreamChunk` method (stream conversion)

5. **`proxy/stream_converter.go`**
   - Line 1-176: Stream conversion logic
   - Line 148-176: `ConvertedStreamResponse` function

6. **`proxy/openai_handler.go`**
   - Line 152-213: `handleChatCompletions` method (request routing)
   - Line 197-209: Streaming logic
   - Line 248-261: `needsStreamConversion` function

7. **`proxy/gemini_handler.go`**
   - Line 136-171: `handleStreamGenerateContent` method (native endpoint handler)

### Documentation Files

1. **`docs/api-reference.md`**
   - Line 104-117: Streaming parameter documentation
   - Line 214-286: Streaming response format

2. **`docs/request-response-formats.md`**
   - Line 99-117: Stream parameter specification
   - Line 372-417: Streaming response format

3. **`docs/curl-examples.md`**
   - Line 73-101: Streaming examples
   - Line 87-101: Gemini streaming examples

4. **`docs/troubleshooting.md`**
   - Line 417-424: Streaming troubleshooting

---

## Conclusion

The 429 error when streaming Gemini models is caused by an incorrect request format in the `GenerateContentStream` method. The streaming endpoint `:streamGenerateContent?alt=sse` does not support the Cloud Code Assist API wrapper format that is correctly used for non-streaming requests.

The fix is straightforward: remove the Cloud Code Assist API wrapper for streaming requests and send the request directly to the streaming endpoint, similar to how Qwen handles streaming. This aligns with the pattern that non-streaming uses the wrapper while streaming should send the request directly to the native endpoint.

**Priority:** High - This is a critical functionality issue that prevents users from using streaming with Gemini models.

**Complexity:** Low - The fix requires minimal code changes (removing the wrapper logic for streaming).

**Risk:** Low - The change only affects streaming requests, which are currently broken, so there's no regression risk for working functionality.

---

## Additional Notes

- The issue appeared after a refactor, suggesting that the streaming endpoint behavior may have changed or that the wrapper was incorrectly applied during the refactor.
- The error message "No capacity available" is misleading; the real issue is an unsupported request format.
- Testing should include both the OpenAI-compatible endpoint (`/v1/chat/completions`) and the native Gemini endpoint (`/gemini/models/{model}:streamGenerateContent`) to ensure both work correctly after the fix.
- Consider adding logging to show the actual request being sent to help debug similar issues in the future.
