# Gemini 404 Error - URL Construction Bug Analysis

## Issue Summary

Requests to Gemini models via the `/v1/chat/completions` endpoint fail with HTTP 404 "Not Found" error. The error message shows:

```
API error (status 404): <!DOCTYPE html>
<html lang=en>
  <meta charset=utf-8>
  <meta name=viewport content="initial-scale=1, minimum-scale=1, width=device-width">
  <title>Error 404 (Not Found)!!1</title>
  ...
  <p><b>404.</b> <ins>That's an error.</ins>
  <p>The requested URL <code>/v1internal/models/gemini-2.5-pro:generateContent</code> was not found on this server.  <ins>That's all we know.</ins>
```

**Key Finding:** The error shows the request is being sent to `/v1internal/models/gemini-2.5-pro:generateContent` which is an incorrect URL format for the Cloud Code Assist API.

---

## Root Cause Analysis

### Location of the Bug

**File:** `provider/gemini/gemini.go`
**Method:** `GenerateContent` (lines 508-703)
**Specific Line:** Line 605

### The Problem

The `GenerateContent` method constructs an incorrect URL by including `/models/{model}` in the path, even though the model is already specified in the request body.

### Code Evidence

**Broken Code (Line 605 in provider/gemini/gemini.go):**

```go
url := fmt.Sprintf("%s/models/%s:generateContent", p.baseURL, model)
```

With `p.baseURL = "https://cloudcode-pa.googleapis.com/v1internal"` and `model = "gemini-2.5-pro"`, this produces:

```
https://cloudcode-pa.googleapis.com/v1internal/models/gemini-2.5-pro:generateContent
```

This is the **incorrect** URL that causes the 404 error.

### Request Body Structure

The request body being sent (lines 588-593) already includes the model:

```go
finalRequest = map[string]interface{}{
    "model":   model,
    "project": p.projectID,
    "request": requestMap,
}
```

Example request body:
```json
{
  "model": "gemini-2.5-pro",
  "project": "projects/abc123",
  "request": {
    "contents": [{"role": "user", "parts": [{"text": "Hello!"}]}]
  }
}
```

Since the model is in the request body, it should NOT be in the URL path.

---

## Comparison with Working Endpoints

### Working: loadCodeAssist Endpoint

**File:** `provider/gemini/gemini.go`
**Line:** 386

```go
url := fmt.Sprintf("%s:loadCodeAssist", p.baseURL)
```

Produces:
```
https://cloudcode-pa.googleapis.com/v1internal:loadCodeAssist
```

✅ **Works correctly** - No `/models/` in the URL

### Working: Non-Streaming Endpoint (Current Implementation)

**File:** `provider/gemini/gemini.go`
**Line:** 580

```go
url := fmt.Sprintf("%s:generateContent", p.baseURL)
```

Produces:
```
https://cloudcode-pa.googleapis.com/v1internal:generateContent
```

✅ **Works correctly** - No `/models/` in the URL

### Broken: generateContent with Model in URL

**File:** `provider/gemini/gemini.go`
**Line:** 605 (THE BUG)

```go
url := fmt.Sprintf("%s/models/%s:generateContent", p.baseURL, model)
```

Produces:
```
https://cloudcode-pa.googleapis.com/v1internal/models/gemini-2.5-pro:generateContent
```

❌ **404 Error** - Incorrect URL format with `/models/{model}`

---

## URL Format Comparison Table

| Endpoint | URL Format | Status |
|-----------|-------------|--------|
| `loadCodeAssist` | `https://cloudcode-pa.googleapis.com/v1internal:loadCodeAssist` | ✅ Working |
| `:generateContent` (line 580) | `https://cloudcode-pa.googleapis.com/v1internal:generateContent` | ✅ Working |
| `/models/{model}:generateContent` (line 605) | `https://cloudcode-pa.googleapis.com/v1internal/models/gemini-2.5-pro:generateContent` | ❌ 404 Error |

---

## Cloud Code Assist API URL Pattern

Based on the working endpoints, the Cloud Code Assist API follows this URL pattern:

```
https://cloudcode-pa.googleapis.com/v1internal:{method}
```

Where `{method}` is:
- `:loadCodeAssist` - For project initialization
- `:generateContent` - For non-streaming content generation
- `:streamGenerateContent?alt=sse` - For streaming content generation

**Important:** The model is specified in the request body, NOT in the URL path.

---

## Request Flow Analysis

### Non-Streaming Request Flow (Broken)

1. **Client Request:** OpenAI format request
   ```json
   {
     "model": "gemini-2.5-pro",
     "messages": [{"role": "user", "content": "Hello!"}]
   }
   ```

2. **Handler:** `handleChatCompletions` in `proxy/openai_handler.go` (line 152)

3. **Converter:** `FromOpenAIRequest` in `converter/gemini.go` (line 166) converts to Gemini format
   ```json
   {
     "contents": [{"role": "user", "parts": [{"text": "Hello!"}]}]
   }
   ```

4. **Provider:** `GenerateContent` in `provider/gemini/gemini.go` (line 508)

5. **Wrapper Applied:** Request wrapped in Cloud Code Assist API format (lines 588-593)
   ```json
   {
     "model": "gemini-2.5-pro",
     "project": "projects/abc123",
     "request": {
       "contents": [{"role": "user", "parts": [{"text": "Hello!"}]}]
     }
   }
   ```

6. **URL Constructed (BUG):** Line 605 constructs incorrect URL
   ```
   POST https://cloudcode-pa.googleapis.com/v1internal/models/gemini-2.5-pro:generateContent
   ```

7. **Error:** API returns 404 because URL path is incorrect

---

## The Fix

### Required Changes

**File:** `provider/gemini/gemini.go`
**Line:** 605

**Change from:**
```go
url := fmt.Sprintf("%s/models/%s:generateContent", p.baseURL, model)
```

**Change to:**
```go
url := fmt.Sprintf("%s:generateContent", p.baseURL)
```

### Why This Fix Works

This change:
1. Removes the incorrect `/models/{model}` path segment from the URL
2. Makes the URL consistent with the working `loadCodeAssist` endpoint pattern
3. Relies on the model being specified in the request body (which it already is)
4. Follows the Cloud Code Assist API URL pattern: `{baseURL}:{method}`

The corrected URL will be:
```
https://cloudcode-pa.googleapis.com/v1internal:generateContent
```

---

## Additional Considerations

### Check for Similar Issues

The same URL construction pattern should be checked in other methods:

1. **GenerateContentStream** (line 779) - Currently uses correct format:
   ```go
   url := fmt.Sprintf("%s:streamGenerateContent?alt=sse", p.baseURL)
   ```
   ✅ This is correct

2. **initializeProject** methods (lines 386, 456) - Currently use correct format:
   ```go
   url := fmt.Sprintf("%s:loadCodeAssist", p.baseURL)
   url := fmt.Sprintf("%s:onboardUser", p.baseURL)
   ```
   ✅ These are correct

### Why Line 580 Works But Line 605 Doesn't

Looking at the code, there are TWO places where URLs are constructed for `generateContent`:

1. **Line 580:** Used in `initializeProject` method for discovery/testing
   ```go
   url := fmt.Sprintf("%s:generateContent", p.baseURL)
   ```
   ✅ Correct format

2. **Line 605:** Used in `GenerateContent` method for actual content generation
   ```go
   url := fmt.Sprintf("%s/models/%s:generateContent", p.baseURL, model)
   ```
   ❌ Incorrect format - includes `/models/{model}`

This inconsistency suggests that line 605 was incorrectly modified, possibly during a refactor or when adding support for multiple models.

---

## Testing Recommendations

After applying the fix, test the following scenarios:

### 1. Basic Non-Streaming Test
```bash
curl -X POST http://localhost:8143/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gemini-2.5-pro",
    "messages": [{"role": "user", "content": "Hello!"}]
  }'
```

### 2. Multiple Models Test
Test all supported Gemini models:
- `gemini-2.5-pro`
- `gemini-2.5-flash`
- `gemini-2.5-flash-lite`
- `gemini-2.5-pro-preview-06-05`
- `gemini-2.5-flash-preview-09-2025`
- `gemini-3-pro-preview`
- `gemini-3-flash-preview`

### 3. System Message Test
```bash
curl -X POST http://localhost:8143/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gemini-2.5-pro",
    "messages": [
      {"role": "system", "content": "You are a helpful assistant"},
      {"role": "user", "content": "Hello!"}
    ]
  }'
```

### 4. Multi-turn Conversation Test
```bash
curl -X POST http://localhost:8143/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gemini-2.5-pro",
    "messages": [
      {"role": "user", "content": "What is 2+2?"},
      {"role": "assistant", "content": "4"},
      {"role": "user", "content": "What about 3+3?"}
    ]
  }'
```

### 5. Verify Streaming Still Works
```bash
curl -X POST http://localhost:8143/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gemini-2.5-pro",
    "messages": [{"role": "user", "content": "Hello!"}],
    "stream": true
  }'
```

---

## Related Files

### Core Implementation Files

1. **`provider/gemini/gemini.go`**
   - Line 23: `DefaultBaseURL` constant
   - Line 386: `loadCodeAssist` URL (working reference)
   - Line 456: `onboardUser` URL (working reference)
   - Line 580: `generateContent` URL in `initializeProject` (working reference)
   - Line 605: `generateContent` URL in `GenerateContent` (**THE BUG**)
   - Line 779: `streamGenerateContent` URL (correct)

2. **`proxy/openai_handler.go`**
   - Line 152: `handleChatCompletions` method
   - Line 216: `handleNonStreamCompletions` method

3. **`proxy/common.go`**
   - Line 15: `GenerateAndConvert` function

4. **`converter/gemini.go`**
   - Line 166: `FromOpenAIRequest` method

---

## Summary

The root cause of the Gemini 404 error is that the `GenerateContent` method in `provider/gemini/gemini.go` constructs an incorrect URL by including `/models/{model}` in the path (line 605).

The Cloud Code Assist API follows the URL pattern `{baseURL}:{method}` where the model is specified in the request body, not in the URL path.

The fix is simple: change line 605 from:
```go
url := fmt.Sprintf("%s/models/%s:generateContent", p.baseURL, model)
```

To:
```go
url := fmt.Sprintf("%s:generateContent", p.baseURL)
```

This makes the URL consistent with the working `loadCodeAssist` and `onboardUser` endpoints, and follows the correct Cloud Code Assist API URL pattern.

---

**Priority:** Critical - This is a blocking issue that prevents all non-streaming Gemini requests from working.

**Complexity:** Very Low - The fix requires changing a single line of code.

**Risk:** Very Low - The change aligns with the working URL pattern used by other endpoints in the same file.

---

## Additional Notes

- The error message from Google clearly shows the incorrect URL path: `/v1internal/models/gemini-2.5-pro:generateContent`
- The inconsistency between line 580 (correct) and line 605 (incorrect) suggests this was introduced during a refactor
- The model is already in the request body in the Cloud Code Assist API wrapper format, so it should not be in the URL
- This fix only affects non-streaming requests; streaming requests use the correct URL format at line 779
