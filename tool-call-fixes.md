# Tool Call Fixes for qwencoder-proxy

## Problem Summary

Two distinct issues were identified causing tool call failures:

1. **Gemini Model**: Tools are not being passed from OpenAI request to Gemini format
2. **Qwen Model**: `tool_calls` array is not being extracted from Qwen response to OpenAI format

---

## Fix 1: Update Gemini Types to Support Function Calls

**File**: `provider/gemini/types.go`

### Add FunctionCall struct after line 37 (after FileData struct):

```go
// FunctionCall represents a function call in a response part
type FunctionCall struct {
    Name string                 `json:"name"`
    Args map[string]interface{} `json:"args"`
}
```

### Update Part struct to include FunctionCall field (around line 21-25):

```go
// Part represents a part of content (text, image, etc.)
type Part struct {
    Text         string        `json:"text,omitempty"`
    InlineData   *InlineData  `json:"inlineData,omitempty"`
    FileData     *FileData    `json:"fileData,omitempty"`
    FunctionCall *FunctionCall `json:"functionCall,omitempty"` // NEW: Added for tool calls
}
```

---

## Fix 2: Add Helper Function to Gemini Converter

**File**: `converter/gemini.go`

### Add helper function at the end of the file (before convertFinishReason):

```go
// getString safely extracts a string value from a map
func getString(m map[string]interface{}, key string) string {
    if val, ok := m[key]; ok {
        if str, ok := val.(string); ok {
            return str
        }
    }
    return ""
}
```

---

## Fix 3: Convert Tools from OpenAI to Gemini Format

**File**: `converter/gemini.go`

### In `FromOpenAIRequest()` function, add this code after line 226 (after generation config conversion, before return):

```go
    // Convert tools from OpenAI format to Gemini format
    if tools, hasTools := openAIReq["tools"]; hasTools {
        if toolsArray, ok := tools.([]interface{}); ok {
            var geminiTools []gemini.Tool
            for _, tool := range toolsArray {
                if toolMap, ok := tool.(map[string]interface{}); ok {
                    if function, hasFunction := toolMap["function"]; hasFunction {
                        if funcMap, ok := function.(map[string]interface{}); ok {
                            functionDecl := gemini.FunctionDeclaration{
                                Name:        getString(funcMap, "name"),
                                Description: getString(funcMap, "description"),
                            }
                            if params, hasParams := funcMap["parameters"]; hasParams {
                                if paramsMap, ok := params.(map[string]interface{}); ok {
                                    functionDecl.Parameters = paramsMap
                                }
                            }
                            geminiTools = append(geminiTools, gemini.Tool{
                                FunctionDeclarations: []gemini.FunctionDeclaration{functionDecl},
                            })
                        }
                    }
                }
            }
            geminiReq.Tools = geminiTools
        }
    }

    // Convert tool_choice to ToolConfig
    if toolChoice, hasToolChoice := openAIReq["tool_choice"]; hasToolChoice {
        if choiceStr, ok := toolChoice.(string); ok && choiceStr != "none" {
            geminiReq.ToolConfig = &gemini.ToolConfig{
                FunctionCallingConfig: &gemini.FunctionCallingConfig{
                    Mode: "AUTO", // Could be "ANY" depending on tool_choice value
                },
            }
        }
    }
```

---

## Fix 4: Extract Function Calls from Gemini Response

**File**: `converter/gemini.go`

### In `ToOpenAIResponse()` function, replace the choice building logic (around lines 64-84) with:

```go
    choices := []interface{}{}
    for i, candidate := range geminiResp.Candidates {
        if candidate.Content != nil {
            var content string
            var toolCalls []interface{}
            
            for _, part := range candidate.Content.Parts {
                if part.Text != "" {
                    content += part.Text
                }
                // Check for function calls
                if part.FunctionCall != nil {
                    toolCall := map[string]interface{}{
                        "id":   fmt.Sprintf("call_%d", time.Now().UnixNano()),
                        "type": "function",
                        "function": map[string]interface{}{
                            "name":      part.FunctionCall.Name,
                            "arguments": part.FunctionCall.Args,
                        },
                    }
                    toolCalls = append(toolCalls, toolCall)
                }
            }

            choice := map[string]interface{}{
                "index": i,
                "message": map[string]interface{}{
                    "role":    "assistant",
                    "content": content,
                },
                "finish_reason": convertFinishReason(candidate.FinishReason),
            }

            // If we have tool calls, update the message
            if len(toolCalls) > 0 {
                choice["message"].(map[string]interface{})["content"] = nil
                choice["message"].(map[string]interface{})["tool_calls"] = toolCalls
                choice["finish_reason"] = "tool_calls"
            }

            choices = append(choices, choice)
        }
    }
```

---

## Fix 5: Extract tool_calls from Qwen Response

**File**: `converter/qwen.go`

### In `ToOpenAIResponse()` function, modify the choice building logic (around lines 74-103):

```go
    // DIAGNOSTIC: Log raw Qwen response structure
    fmt.Printf("[QwenConverter DIAGNOSTIC] Raw Qwen response: %+v\n", qwenResp)
    
    if choices, ok := extractChoices(qwenResp); ok {
        var openAIChoices []interface{}
        for i, choice := range choices {
            if choiceMap, ok := choice.(map[string]interface{}); ok {
                // DIAGNOSTIC: Log each choice
                fmt.Printf("[QwenConverter DIAGNOSTIC] Choice %d: %+v\n", i, choiceMap)
                
                content := ""
                if msg, exists := choiceMap["message"]; exists {
                    if msgMap, msgOk := msg.(map[string]interface{}); msgOk {
                        // DIAGNOSTIC: Log message structure
                        fmt.Printf("[QwenConverter DIAGNOSTIC] Choice %d message: %+v\n", i, msgMap)
                        if c, ok := msgMap["content"].(string); ok {
                            content = c
                        }
                        // DIAGNOSTIC: Check for tool_calls
                        if toolCalls, hasToolCalls := msgMap["tool_calls"]; hasToolCalls {
                            fmt.Printf("[QwenConverter DIAGNOSTIC] Tool calls found in choice %d: %+v\n", i, toolCalls)
                        } else {
                            fmt.Printf("[QwenConverter DIAGNOSTIC] No tool_calls found in choice %d message\n", i)
                        }
                    }
                }

                finishReason := "stop"
                if reason, ok := choiceMap["finish_reason"].(string); ok {
                    finishReason = reason
                }

                openAIChoice := map[string]interface{}{
                    "index": i,
                    "message": map[string]interface{}{
                        "role":    "assistant",
                        "content": content,
                    },
                    "finish_reason": finishReason,
                }

                // Extract tool_calls from Qwen response
                if msg, exists := choiceMap["message"]; exists {
                    if msgMap, msgOk := msg.(map[string]interface{}); msgOk {
                        if toolCalls, hasToolCalls := msgMap["tool_calls"]; hasToolCalls {
                            // Update the choice with tool_calls
                            openAIChoice["message"].(map[string]interface{})["tool_calls"] = toolCalls
                            // Clear content if tool_calls are present
                            openAIChoice["message"].(map[string]interface{})["content"] = nil
                        }
                    }
                }

                openAIChoices = append(openAIChoices, openAIChoice)
            }
        }
        openAIResp["choices"] = openAIChoices
    }
```

---

## Summary of Changes

| File | Change Type | Description |
|------|-------------|-------------|
| `provider/gemini/types.go` | Add struct | Add `FunctionCall` struct |
| `provider/gemini/types.go` | Modify struct | Add `FunctionCall` field to `Part` struct |
| `converter/gemini.go` | Add function | Add `getString()` helper function |
| `converter/gemini.go` | Add logic | Convert `tools` from OpenAI to Gemini format |
| `converter/gemini.go` | Add logic | Convert `tool_choice` from OpenAI to Gemini format |
| `converter/gemini.go` | Modify logic | Extract `functionCall` from Gemini response and convert to OpenAI `tool_calls` |
| `converter/qwen.go` | Modify logic | Extract `tool_calls` from Qwen response message |

---

## Testing After Fixes

After applying these fixes, test with the following requests:

### Test 1: Gemini with tools
```json
{
  "model": "gemini-2.5-flash",
  "messages": [
    {"role": "user", "content": "What is the weather in Amsterdam?"}
  ],
  "tools": [
    {
      "type": "function",
      "function": {
        "name": "get_weather",
        "description": "Get the current weather for a location",
        "parameters": {
          "type": "object",
          "properties": {
            "location": {
              "type": "string",
              "description": "The city name"
            },
            "unit": {
              "type": "string",
              "enum": ["celsius", "fahrenheit"],
              "description": "Temperature unit"
            }
          },
          "required": ["location"]
        }
      }
    }
  ],
  "tool_choice": "auto",
  "stream": false
}
```

**Expected Result**: Response should include `tool_calls` array with the function call details.

### Test 2: Qwen with tools
```json
{
  "model": "qwen3-coder-plus",
  "messages": [
    {"role": "user", "content": "What is the weather in Amsterdam?"}
  ],
  "tools": [
    {
      "type": "function",
      "function": {
        "name": "get_weather",
        "description": "Get the current weather for a location",
        "parameters": {
          "type": "object",
          "properties": {
            "location": {
              "type": "string",
              "description": "The city name"
            },
            "unit": {
              "type": "string",
              "enum": ["celsius", "fahrenheit"],
              "description": "Temperature unit"
            }
          },
          "required": ["location"]
        }
      }
    }
  ],
  "tool_choice": "auto",
  "stream": false
}
```

**Expected Result**: Response should include `tool_calls` array with the function call details.

---

## Notes

1. The diagnostic logging added to `converter/gemini.go` and `converter/qwen.go` can be removed after confirming the fixes work correctly.

2. The `tool_choice` conversion for Gemini uses "AUTO" mode. You may want to enhance this to handle other values like "required", "none", or specific function names.

3. The `tool_calls` ID generation uses a timestamp-based approach. You may want to use a more deterministic approach if needed.
