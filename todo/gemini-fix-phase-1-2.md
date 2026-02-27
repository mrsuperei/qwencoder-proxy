# Gemini Provider Fix Plan: Phase 1 & Phase 2

## Overview

This plan details the implementation of fixes for the Gemini provider to resolve:
1. Missing critical headers causing model availability issues
2. Streaming functionality not working due to incorrect payload structure

## Phase 1: Add Missing Headers

### Objective
Add the three critical headers (`User-Agent`, `X-Goog-Api-Client`, `Client-Metadata`) to all Gemini API requests to match the working implementation.

### Files to Modify
- [`qwencoder-proxy/provider/gemini/gemini.go`](qwencoder-proxy/provider/gemini/gemini.go:1)

### Implementation Steps

#### Step 1.1: Create Header Helper Function

**Location:** Add new function after line 266 (after `doRequestWithProxy` function)

**Code to Add:**
```go
// applyGeminiHeaders sets required headers for Gemini API requests
// These headers are required by the Cloud Code Assist API for proper request handling
func (p *Provider) applyGeminiHeaders(req *http.Request) {
    if req == nil {
        return
    }
    // User-Agent identifies the client to the API
    req.Header.Set("User-Agent", "google-api-nodejs-client/9.15.1")
    
    // X-Goog-Api-Client provides API client information
    req.Header.Set("X-Goog-Api-Client", "gl-node/22.17.0")
    
    // Client-Metadata provides IDE and platform context
    req.Header.Set("Client-Metadata", "ideType=IDE_UNSPECIFIED,platform=PLATFORM_UNSPECIFIED,pluginType=GEMINI")
}
```

**Rationale:**
- Centralizes header logic for consistency
- Makes it easy to update headers in one place
- Matches the working implementation's pattern

#### Step 1.2: Apply Headers in `initializeProject`

**Location:** [`qwencoder-proxy/provider/gemini/gemini.go:387-393`](qwencoder-proxy/provider/gemini/gemini.go:387)

**Current Code:**
```go
req.Header.Set("Authorization", "Bearer "+token)
req.Header.Set("Content-Type", "application/json")
```

**Modified Code:**
```go
req.Header.Set("Authorization", "Bearer "+token)
req.Header.Set("Content-Type", "application/json")
p.applyGeminiHeaders(req)  // Add this line
```

#### Step 1.3: Apply Headers in `initializeProject` (onboardUser request)

**Location:** [`qwencoder-proxy/provider/gemini/gemini.go:462-464`](qwencoder-proxy/provider/gemini/gemini.go:462)

**Current Code:**
```go
onboardReq.Header.Set("Authorization", "Bearer "+token)
onboardReq.Header.Set("Content-Type", "application/json")
onboardReq.Header.Set("User-Agent", "qwencoder-proxy/1.0")
```

**Modified Code:**
```go
onboardReq.Header.Set("Authorization", "Bearer "+token)
onboardReq.Header.Set("Content-Type", "application/json")
onboardReq.Header.Set("User-Agent", "qwencoder-proxy/1.0")
p.applyGeminiHeaders(onboardReq)  // Add this line
```

#### Step 1.4: Apply Headers in `GenerateContent` (initial request)

**Location:** [`qwencoder-proxy/provider/gemini/gemini.go:611-613`](qwencoder-proxy/provider/gemini/gemini.go:611)

**Current Code:**
```go
req.Header.Set("Authorization", "Bearer "+token)
req.Header.Set("Content-Type", "application/json")
```

**Modified Code:**
```go
req.Header.Set("Authorization", "Bearer "+token)
req.Header.Set("Content-Type", "application/json")
p.applyGeminiHeaders(req)  // Add this line
```

#### Step 1.5: Apply Headers in `GenerateContent` (retry request)

**Location:** [`qwencoder-proxy/provider/gemini/gemini.go:656-658`](qwencoder-proxy/provider/gemini/gemini.go:656)

**Current Code:**
```go
retryReq.Header.Set("Authorization", "Bearer "+refreshedToken)
retryReq.Header.Set("Content-Type", "application/json")
```

**Modified Code:**
```go
retryReq.Header.Set("Authorization", "Bearer "+refreshedToken)
retryReq.Header.Set("Content-Type", "application/json")
p.applyGeminiHeaders(retryReq)  // Add this line
```

#### Step 1.6: Apply Headers in `GenerateContentStream` (initial request)

**Location:** [`qwencoder-proxy/provider/gemini/gemini.go:783-785`](qwencoder-proxy/provider/gemini/gemini.go:783)

**Current Code:**
```go
req.Header.Set("Authorization", "Bearer "+token)
req.Header.Set("Content-Type", "application/json")
req.Header.Set("Accept", "text/event-stream")
```

**Modified Code:**
```go
req.Header.Set("Authorization", "Bearer "+token)
req.Header.Set("Content-Type", "application/json")
req.Header.Set("Accept", "text/event-stream")
p.applyGeminiHeaders(req)  // Add this line
```

#### Step 1.7: Apply Headers in `GenerateContentStream` (retry request)

**Location:** [`qwencoder-proxy/provider/gemini/gemini.go:827-829`](qwencoder-proxy/provider/gemini/gemini.go:827)

**Current Code:**
```go
retryReq.Header.Set("Authorization", "Bearer "+refreshedToken)
retryReq.Header.Set("Content-Type", "application/json")
retryReq.Header.Set("Accept", "text/event-stream")
```

**Modified Code:**
```go
retryReq.Header.Set("Authorization", "Bearer "+refreshedToken)
retryReq.Header.Set("Content-Type", "application/json")
retryReq.Header.Set("Accept", "text/event-stream")
p.applyGeminiHeaders(retryReq)  // Add this line
```

### Testing for Phase 1

#### Test 1.1: Non-streaming Request with Headers
```bash
# Test with a model that was previously failing
curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gemini-2.5-pro",
    "messages": [{"role": "user", "content": "Hello"}],
    "stream": false
  }'
```

**Expected Result:** Request succeeds with proper response

#### Test 1.2: Streaming Request with Headers
```bash
# Test streaming with headers
curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gemini-2.5-flash",
    "messages": [{"role": "user", "content": "Hello"}],
    "stream": true
  }'
```

**Expected Result:** Request may still fail (due to Phase 2 bug), but headers should be sent correctly

#### Test 1.3: Verify Headers in Logs
Check logs to confirm headers are being sent:
```
[Gemini] Sending generateContent request to https://cloudcode-pa.googleapis.com/v1internal:generateContent
```

### Success Criteria for Phase 1
- [ ] `applyGeminiHeaders()` function implemented
- [ ] Headers applied to all 7 request locations
- [ ] Non-streaming requests work with previously failing models
- [ ] Logs show requests are being sent with proper headers

---

## Phase 2: Fix Streaming Payload Structure

### Objective
Fix the streaming payload structure to include the `project` and `model` wrapper, matching the non-streaming request format.

### Root Cause
In [`GenerateContentStream()`](qwencoder-proxy/provider/gemini/gemini.go:767), the request is sent directly without the wrapper structure that the API expects:

```go
// CURRENT (INCORRECT):
finalRequest := requestMap  // Direct map, no wrapper

// EXPECTED (CORRECT):
finalRequest := map[string]interface{}{
    "model":   model,
    "project": p.projectID,
    "request": requestMap,
}
```

### Files to Modify
- [`qwencoder-proxy/provider/gemini/gemini.go`](qwencoder-proxy/provider/gemini/gemini.go:1)

### Implementation Steps

#### Step 2.1: Fix Streaming Payload Structure

**Location:** [`qwencoder-proxy/provider/gemini/gemini.go:763-777`](qwencoder-proxy/provider/gemini/gemini.go:763)

**Current Code:**
```go
// FOR STREAMING: Send request directly without Cloud Code Assist wrapper
// The :streamGenerateContent?alt=sse endpoint does not support the wrapper format
// that is used for non-streaming requests with :generateContent endpoint
finalRequest := requestMap

// Marshal the request
reqBody, marshalErr := json.Marshal(finalRequest)
if marshalErr != nil {
    return nil, fmt.Errorf("failed to marshal request: %w", marshalErr)
}

// Add the crucial alt=sse query parameter for streaming
// Include the model name in the URL path
url := fmt.Sprintf("%s:streamGenerateContent?alt=sse", p.baseURL)
```

**Modified Code:**
```go
// FOR STREAMING: Use the same wrapper format as non-streaming requests
// The :streamGenerateContent?alt=sse endpoint requires the same structure
// with project and model at the top level, and the actual request in a "request" field
finalRequest := map[string]interface{}{
    "model":   model,
    "project": p.projectID,
    "request": requestMap,
}

// Marshal the request
reqBody, marshalErr := json.Marshal(finalRequest)
if marshalErr != nil {
    return nil, fmt.Errorf("failed to marshal request: %w", marshalErr)
}

// DEBUG: Log the actual streaming request being sent
p.GetLogger().DebugLog("[Gemini] Streaming request payload: %s", string(reqBody))

// Add the crucial alt=sse query parameter for streaming
url := fmt.Sprintf("%s:streamGenerateContent?alt=sse", p.baseURL)
```

**Rationale:**
- The working implementation shows that streaming requests need the same wrapper structure
- The comment about "does not support the wrapper format" was incorrect
- Adding debug logging will help verify the fix

#### Step 2.2: Update Retry Request Payload

**Location:** [`qwencoder-proxy/provider/gemini/gemini.go:809-818`](qwencoder-proxy/provider/gemini/gemini.go:809)

**Current Code:**
```go
// Retry the request with the refreshed token
// Recreate the request body with the same finalRequest
retryReqBody, marshalErr := json.Marshal(finalRequest)
if marshalErr != nil {
    return nil, fmt.Errorf("failed to marshal request for retry: %w", marshalErr)
}
```

**Modified Code:**
```go
// Retry the request with the refreshed token
// Recreate the request body with the same finalRequest
retryReqBody, marshalErr := json.Marshal(finalRequest)
if marshalErr != nil {
    return nil, fmt.Errorf("failed to marshal request for retry: %w", marshalErr)
}

// DEBUG: Log the retry streaming request
p.GetLogger().DebugLog("[Gemini] Retrying streaming request with refreshed token")
```

### Testing for Phase 2

#### Test 2.1: Basic Streaming Request
```bash
# Test basic streaming
curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gemini-2.5-flash",
    "messages": [{"role": "user", "content": "Say hello"}],
    "stream": true
  }'
```

**Expected Result:** Server-Sent Events (SSE) stream returned with content chunks

#### Test 2.2: Streaming with Different Models
```bash
# Test with different models
for model in "gemini-2.5-pro" "gemini-2.5-flash-lite" "gemini-3-flash-preview"; do
  echo "Testing model: $model"
  curl -X POST http://localhost:8080/v1/chat/completions \
    -H "Content-Type: application/json" \
    -d "{
      \"model\": \"$model\",
      \"messages\": [{\"role\": \"user\", \"content\": \"Hello\"}],
      \"stream\": true
    }"
  echo ""
done
```

**Expected Result:** All models return streaming responses

#### Test 2.3: Verify Payload Structure in Logs
Check logs to confirm the payload structure:
```
[Gemini] Streaming request payload: {"model":"gemini-2.5-flash","project":"project-id","request":{"contents":[...]}}
```

**Expected Result:** Payload includes `model`, `project`, and `request` fields

#### Test 2.4: Compare Non-Streaming vs Streaming
```bash
# Test non-streaming (should work)
curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gemini-2.5-flash",
    "messages": [{"role": "user", "content": "Test"}],
    "stream": false
  }' > non_stream.json

# Test streaming (should now also work)
curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gemini-2.5-flash",
    "messages": [{"role": "user", "content": "Test"}],
    "stream": true
  }' > stream.json
```

**Expected Result:** Both requests succeed with similar content

### Success Criteria for Phase 2
- [ ] Streaming payload structure corrected with `model`, `project`, and `request` wrapper
- [ ] Basic streaming requests return SSE responses
- [ ] Multiple models work with streaming
- [ ] Logs show correct payload structure
- [ ] Non-streaming and streaming produce consistent results

---

## Implementation Checklist

### Phase 1: Headers
- [ ] Create `applyGeminiHeaders()` helper function
- [ ] Apply headers to `initializeProject` loadCodeAssist request
- [ ] Apply headers to `initializeProject` onboardUser request
- [ ] Apply headers to `GenerateContent` initial request
- [ ] Apply headers to `GenerateContent` retry request
- [ ] Apply headers to `GenerateContentStream` initial request
- [ ] Apply headers to `GenerateContentStream` retry request
- [ ] Test non-streaming with previously failing models
- [ ] Verify headers in logs

### Phase 2: Streaming
- [ ] Fix streaming payload structure in `GenerateContentStream`
- [ ] Add debug logging for streaming payload
- [ ] Test basic streaming request
- [ ] Test streaming with multiple models
- [ ] Verify payload structure in logs
- [ ] Compare non-streaming vs streaming results

---

## Rollback Plan

If issues arise after implementation:

### Phase 1 Rollback
1. Remove `applyGeminiHeaders()` function
2. Remove all `p.applyGeminiHeaders(req)` calls
3. Restore original header code

### Phase 2 Rollback
1. Revert streaming payload to `finalRequest := requestMap`
2. Remove debug logging
3. Restore original comment about wrapper format

---

## Notes

1. **Header Values:** The header values (`User-Agent`, `X-Goog-Api-Client`) are taken directly from the working implementation. These may need to be updated if the API changes.

2. **Streaming Payload:** The comment about streaming not supporting the wrapper format was incorrect. The working implementation clearly shows that streaming requests use the same wrapper structure.

3. **Debug Logging:** Added debug logging for streaming requests to help verify the fix. This can be removed after testing.

4. **Future Enhancements:** After Phase 1 and Phase 2 are complete, consider implementing Phase 3 (model fallback mechanism) and Phase 4 (enhanced error handling) from the full analysis report.

---

## References

- Full Analysis Report: [`qwencoder-proxy/docs/gemini-implementation-analysis.md`](qwencoder-proxy/docs/gemini-implementation-analysis.md:1)
- Working Implementation: Reference codebase provided by user
- Current Implementation: [`qwencoder-proxy/provider/gemini/gemini.go`](qwencoder-proxy/provider/gemini/gemini.go:1)
