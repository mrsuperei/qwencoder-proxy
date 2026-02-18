# Gemini Streaming 400 Error Analysis

## Issue Summary

Streaming requests to Gemini models fail with HTTP 400 "INVALID_ARGUMENT" error, while non-streaming requests work correctly. Qwen models continue to work fine with streaming enabled.

**Error Message:**
```
2026/02/18 14:19:24 ⚠️  [ERROR] [Handler] Streaming error: API error (status 400): {
  "error": {
    "code": 400,
    "message": "Invalid JSON payload received. Unknown name \"contents\": Cannot find field.\nInvalid JSON payload received. Unknown name \"systemInstruction\": Cannot find field.",
    "status": "INVALID_ARGUMENT",
    "details": [
      {
        "@type": "type.googleapis.com/google.rpc.BadRequest",
        "fieldViolations": [
          {
            "description": "Invalid JSON payload received. Unknown name \"contents\": Cannot find field."
          },
          {
            "description": "Invalid JSON payload received. Unknown name \"systemInstruction\": Cannot find field."
          }
        ]
      }
    ]
  }
}
```

**Symptoms:**
- ✅ Non-streaming Gemini requests work fine
- ❌ Streaming Gemini requests fail with 400 error
- ✅ Qwen streaming works correctly
- ⚠️ Issue is in the request formatting for streaming

---

## Root Cause Analysis

### Location of the Bug

**File:** `provider/gemini/gemini.go`
**Method:** `GenerateContentStream` (lines 679-824)
**Specific Line:** Line 742

### The Problem

The `GenerateContentStream` method incorrectly sends the Gemini request directly without wrapping it in the Cloud Code Assist API format. The streaming endpoint `:streamGenerateContent?alt=sse` actually **REQUIRES** the same wrapper format as the non-streaming endpoint, contrary to what the comment in the code suggests.

### Code Evidence

**Broken Code (Lines 739-742 in provider/gemini/gemini.go):**

```go
// FOR STREAMING: Send request directly without Cloud Code Assist wrapper
// The :streamGenerateContent?alt=sse endpoint does not support the wrapper format
// that is used for non-streaming requests with :generateContent endpoint
finalRequest := requestMap
```

**The Issue:** The `finalRequest` is set to `requestMap` (the raw Gemini request), which contains fields like `contents` and `systemInstruction`. However, the API expects these to be wrapped in a Cloud Code Assist API format with `model`, `project`, and `request` fields.

### Why Non-Streaming Works

**Working Code (Lines 567-572 in provider/gemini/gemini.go):**

```go
// This is a standard Gemini API request, format it for Cloud Code Assist API
finalRequest = map[string]interface{}{
    "model":   model,
    "project": p.projectID,
    "request": requestMap,
}
```

The non-streaming `GenerateContent` method correctly wraps the request in the Cloud Code Assist API format, which is why it works.

### Error Message Explanation

The error message:
```
"Invalid JSON payload received. Unknown name \"contents\": Cannot find field.\nInvalid JSON payload received. Unknown name \"systemInstruction\": Cannot find field."
```

This confirms that the API is receiving the raw Gemini request format (with `contents` and `systemInstruction` fields at the top level) instead of the wrapped format it expects.

---

## Request Flow Analysis

### Non-Streaming Request Flow (Working)

1. **Handler** ([`proxy/gemini_handler.go:93-133`](proxy/gemini_handler.go:93-133)):
   - Parses request as `gemini.GeminiRequest`
   - Calls `provider.GenerateContent(ctx, model, &request)`

2. **Provider** ([`provider/gemini/gemini.go:515-676`](provider/gemini/gemini.go:515-676)):
   - Wraps request in Cloud Code Assist format:
     ```json
     {
       "model": "gemini-2.5-pro",
       "project": "project-id",
       "request": {
         "contents": [...],
         "systemInstruction": {...}
       }
     }
     ```
   - Sends to `:generateContent` endpoint
   - ✅ **Works correctly**

### Streaming Request Flow (Broken)

1. **Handler** ([`proxy/gemini_handler.go:136-171`](proxy/gemini_handler.go:136-171)):
   - Parses request as `gemini.GeminiRequest`
   - Calls `ConvertedStreamResponse(w, r, factory, h.provider, &request, model, h.GetLogger())`

2. **Stream Converter** ([`proxy/stream_converter.go:149-176`](proxy/stream_converter.go:149-176)):
   - Calls `provider.GenerateContentStream(ctx, model, nativeReq)`

3. **Provider** ([`provider/gemini/gemini.go:679-824`](provider/gemini/gemini.go:679-824)):
   - **BUG:** Sends raw request without wrapping:
     ```json
     {
       "contents": [...],
       "systemInstruction": {...}
     }
     ```
   - Sends to `:streamGenerateContent?alt=sse` endpoint
   - ❌ **Fails with 400 error**

---

## Comparison with Qwen Implementation

The Qwen provider ([`provider/qwen/qwen.go:451-526`](provider/qwen/qwen.go:451-526)) handles streaming correctly because:

1. Qwen API uses OpenAI-compatible format, so no wrapping is needed
2. The request is marshaled directly:
   ```go
   reqBody, err := json.Marshal(request)
   ```
3. No special Cloud Code Assist API wrapper is required

This is why Qwen streaming works while Gemini streaming fails.

---

## Type Definitions

### GeminiRequest Structure

**File:** [`provider/gemini/types.go:5-12`](provider/gemini/types.go:5-12)

```go
type GeminiRequest struct {
    Contents          []Content         `json:"contents"`
    SystemInstruction *Content          `json:"systemInstruction,omitempty"`
    GenerationConfig  *GenerationConfig `json:"generationConfig,omitempty"`
    SafetySettings    []SafetySetting   `json:"safetySettings,omitempty"`
    Tools             []Tool            `json:"tools,omitempty"`
    ToolConfig        *ToolConfig       `json:"toolConfig,omitempty"`
}
```

This is the raw Gemini API format that needs to be wrapped.

---

## The Fix

### Required Changes

**File:** `provider/gemini/gemini.go`
**Lines:** 739-742

**Change from:**
```go
// FOR STREAMING: Send request directly without Cloud Code Assist wrapper
// The :streamGenerateContent?alt=sse endpoint does not support the wrapper format
// that is used for non-streaming requests with :generateContent endpoint
finalRequest := requestMap
```

**Change to:**
```go
// FOR STREAMING: Wrap request in Cloud Code Assist API format
// The :streamGenerateContent?alt=sse endpoint requires the same wrapper format
// as the non-streaming :generateContent endpoint
finalRequest = map[string]interface{}{
    "model":   model,
    "project": p.projectID,
    "request": requestMap,
}
```

### Why This Fix Works

This change makes the streaming implementation consistent with the non-streaming implementation by:

1. Wrapping the raw Gemini request in the Cloud Code Assist API format
2. Including the required `model` and `project` fields
3. Placing the actual request content in the `request` field

The API will then receive the request in the correct format:
```json
{
  "model": "gemini-2.5-pro",
  "project": "project-id",
  "request": {
    "contents": [...],
    "systemInstruction": {...}
  }
}
```

---

## Related Files

### Core Implementation Files

1. **[`provider/gemini/gemini.go`](provider/gemini/gemini.go)** - Gemini provider implementation
   - `GenerateContent` (lines 515-676) - Non-streaming (working)
   - `GenerateContentStream` (lines 679-824) - Streaming (broken)

2. **[`proxy/gemini_handler.go`](proxy/gemini_handler.go)** - Gemini HTTP handler
   - `handleGenerateContent` (lines 93-133) - Non-streaming handler
   - `handleStreamGenerateContent` (lines 136-171) - Streaming handler

3. **[`proxy/stream_converter.go`](proxy/stream_converter.go)** - Stream conversion logic
   - `ConvertedStreamResponse` (lines 149-176) - Common streaming handler

4. **[`provider/gemini/types.go`](provider/gemini/types.go)** - Type definitions
   - `GeminiRequest` (lines 5-12) - Request structure

### Comparison Files

5. **[`provider/qwen/qwen.go`](provider/qwen/qwen.go)** - Qwen provider (working reference)
   - `GenerateContentStream` (lines 451-526) - Working streaming implementation

### Documentation Files

6. **[`docs/api-reference.md`](docs/api-reference.md)** - API documentation
7. **[`docs/request-response-formats.md`](docs/request-response-formats.md)** - Request/response format documentation

---

## Testing Recommendations

After applying the fix, test the following scenarios:

1. **Basic streaming request:**
   ```bash
   curl -X POST http://localhost:8143/gemini/models/gemini-2.5-pro:streamGenerateContent \
     -H "Content-Type: application/json" \
     -d '{
       "contents": [{"role": "user", "parts": [{"text": "Hello"}]}],
       "systemInstruction": {"role": "user", "parts": [{"text": "You are a helpful assistant"}]}
     }'
   ```

2. **Streaming with OpenAI-compatible endpoint:**
   ```bash
   curl -X POST http://localhost:8143/v1/chat/completions \
     -H "Content-Type: application/json" \
     -d '{
       "model": "gemini-2.5-pro",
       "messages": [{"role": "user", "content": "Hello"}],
       "stream": true
     }'
   ```

3. **Verify non-streaming still works:**
   ```bash
   curl -X POST http://localhost:8143/gemini/models/gemini-2.5-pro:generateContent \
     -H "Content-Type: application/json" \
     -d '{
       "contents": [{"role": "user", "parts": [{"text": "Hello"}]}]
     }'
   ```

---

## Summary

The root cause of the Gemini streaming 400 error is that the `GenerateContentStream` method sends the raw Gemini request format directly to the API, instead of wrapping it in the Cloud Code Assist API format like the non-streaming `GenerateContent` method does.

The fix is simple: wrap the request in the same Cloud Code Assist API format for streaming as is done for non-streaming requests. This involves changing one line of code in [`provider/gemini/gemini.go:742`](provider/gemini/gemini.go:742).

The incorrect comment in the code suggests that the streaming endpoint doesn't support the wrapper format, but the error message clearly indicates that the API expects the wrapper format, not the raw request format.
