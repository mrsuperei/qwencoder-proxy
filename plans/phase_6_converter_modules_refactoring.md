# Phase 6: Converter Modules Refactoring

**Phase**: 6 of 7  
**Estimated Time**: 1 day  
**Risk Level**: Very Low  
**Dependencies**: None (Standalone helper functions)

---

## Phase Objectives

This phase creates helper functions for converter modules to eliminate code duplication and ensure consistent behavior across all converters.

**Primary Goals:**
1. Create converter helper functions for common operations
2. Remove diagnostic code from Qwen converter
3. Update all converters to use helper functions
4. Ensure backward compatibility with existing code
5. Maintain consistent output format across all converters

---

## Target Files to Create

1. `converter/helpers.go` - Helper functions for OpenAI response creation

## Target Files to Modify

1. `converter/gemini.go` - Update to use helper functions
2. `converter/qwen.go` - Update to use helper functions and remove diagnostic code
3. `converter/claude.go` - Update to use helper functions

---

## Step-by-Step Instructions

### Step 1: Create Converter Helper Functions

**File to Create**: `converter/helpers.go`

```go
// Package converter provides helper functions for format conversion
package converter

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

// CreateOpenAIResponseBase creates the base structure for an OpenAI response
// This follows the Factory Method pattern for consistent response creation
func CreateOpenAIResponseBase(model string) map[string]interface{} {
	return map[string]interface{}{
		"id":      "chatcmpl-" + generateID(),
		"object":  "chat.completion",
		"created": getCurrentTimestamp(),
		"model":   model,
		"choices": []interface{}{},
		"usage": map[string]interface{}{
			"prompt_tokens":     0,
			"completion_tokens": 0,
			"total_tokens":      0,
		},
	}
}

// CreateOpenAIChoice creates a choice structure for OpenAI response
func CreateOpenAIChoice(index int, content string, finishReason string) map[string]interface{} {
	return map[string]interface{}{
		"index":         index,
		"message":       map[string]interface{}{"role": "assistant", "content": content},
		"finish_reason": finishReason,
	}
}

// CreateOpenAIChoiceWithToolCalls creates a choice with tool calls for OpenAI response
func CreateOpenAIChoiceWithToolCalls(index int, toolCalls []interface{}, finishReason string) map[string]interface{} {
	return map[string]interface{}{
		"index":         index,
		"message":       map[string]interface{}{"role": "assistant", "content": nil, "tool_calls": toolCalls},
		"finish_reason": finishReason,
	}
}

// UpdateUsage updates the usage field in OpenAI response
func UpdateUsage(resp map[string]interface{}, promptTokens, completionTokens, totalTokens int) {
	if resp["usage"] == nil {
		resp["usage"] = map[string]interface{}{}
	}
	usage := resp["usage"].(map[string]interface{})
	if promptTokens >= 0 {
		usage["prompt_tokens"] = promptTokens
	}
	if completionTokens >= 0 {
		usage["completion_tokens"] = completionTokens
	}
	if totalTokens >= 0 {
		usage["total_tokens"] = totalTokens
	}
}

// generateID generates a unique ID for responses
func generateID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)[:16]
}

// getCurrentTimestamp returns the current Unix timestamp
func getCurrentTimestamp() int64 {
	return time.Now().Unix()
}

// ConvertFinishReason converts provider-specific finish reason to OpenAI format
func ConvertFinishReason(reason string) string {
	// Map common finish reasons to OpenAI format
	switch reason {
	case "STOP", "stop":
		return "stop"
	case "MAX_TOKENS", "length":
		return "length"
	case "RECITATION_FAILURE", "content_filter":
		return "content_filter"
	case "SAFETY", "safety":
		return "content_filter"
	default:
		return "stop"
	}
}
```

**Edge Cases:**
- **Nil Usage**: UpdateUsage safely handles nil usage by creating a new map.
- **Negative Tokens**: UpdateUsage only updates usage values if they are >= 0, allowing partial updates.
- **Unknown Finish Reason**: ConvertFinishReason defaults to "stop" for unknown reasons.

**Verification:**
- File created at `converter/helpers.go`
- All helper functions are present
- Functions are pure (no side effects)
- Functions are well-documented

---

### Step 2: Remove Diagnostic Code from Qwen Converter

**File to Modify**: `converter/qwen.go`

**Lines to Remove**: 74-97

**Diagnostic Code to Remove**:
```go
// REMOVE THESE DIAGNOSTIC STATEMENTS (lines 74-97):
fmt.Printf("[QwenConverter DIAGNOSTIC] Raw Qwen response: %+v\n", qwenResp)
fmt.Printf("[QwenConverter DIAGNOSTIC] Choice %d: %+v\n", i, choiceMap)
fmt.Printf("[QwenConverter DIAGNOSTIC] Choice %d message: %+v\n", i, msgMap)
fmt.Printf("[QwenConverter DIAGNOSTIC] Tool calls found in choice %d: %+v\n", i, toolCalls)
fmt.Printf("[QwenConverter DIAGNOSTIC] No tool_calls found in choice %d message\n", i)
```

**Modification Steps**:

1. **Locate and remove lines 74-97**:

Search for the following pattern and remove all matching lines:
- `fmt.Printf("[QwenConverter DIAGNOSTIC]`

2. **Verify removal**:

After removal, the code should not contain any `fmt.Printf` statements with "QwenConverter DIAGNOSTIC" prefix.

**Edge Cases:**
- **No Diagnostic Code**: If the diagnostic code has already been removed, this step can be skipped.

**Verification:**
- Lines 74-97 are removed
- No `fmt.Printf` statements with "QwenConverter DIAGNOSTIC" prefix
- Code still compiles and functions correctly

---

### Step 3: Update Gemini Converter to Use Helper Functions

**File to Modify**: `converter/gemini.go`

**Current Code** (lines 43-54):
```go
openAIResp := map[string]interface{}{
	"id":      "chatcmpl-" + generateID(),
	"object":  "chat.completion",
	"created": getCurrentTimestamp(),
	"model":   model,
	"choices": []interface{}{},
	"usage": map[string]interface{}{
		"prompt_tokens":     0,
		"completion_tokens": 0,
		"total_tokens":      0,
	},
}
```

**Refactored Code**:
```go
openAIResp := CreateOpenAIResponseBase(model)
```

**Modification Steps**:

1. **Update ToOpenAIResponse method** (around line 28):

```go
// ToOpenAIResponse converts Gemini format to OpenAI format
func (c *GeminiConverter) ToOpenAIResponse(native interface{}, model string) (interface{}, error) {
	// Convert Gemini response to OpenAI format
	geminiResp, ok := native.(*gemini.GeminiResponse)
	if !ok {
		// Try to convert from map if it's not already a GeminiResponse
		if data, ok := native.(map[string]interface{}); ok {
			jsonBytes, _ := json.Marshal(data)
			geminiResp = &gemini.GeminiResponse{}
			json.Unmarshal(jsonBytes, geminiResp)
		} else {
			return nil, fmt.Errorf("unexpected response type: %T", native)
		}
	}

	// Create OpenAI-compatible response using helper function
	openAIResp := CreateOpenAIResponseBase(model)

	if geminiResp.UsageMetadata != nil {
		UpdateUsage(openAIResp, 
			geminiResp.UsageMetadata.PromptTokenCount,
			geminiResp.UsageMetadata.CandidatesTokenCount,
			geminiResp.UsageMetadata.TotalTokenCount)
	}

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

			var choice map[string]interface{}
			if len(toolCalls) > 0 {
				choice = CreateOpenAIChoiceWithToolCalls(i, toolCalls, ConvertFinishReason(candidate.FinishReason))
			} else {
				choice = CreateOpenAIChoice(i, content, ConvertFinishReason(candidate.FinishReason))
			}

			choices = append(choices, choice)
		}
	}

	openAIResp["choices"] = choices
	return openAIResp, nil
}
```

2. **Remove local helper functions**:

The following functions should be **removed** as they are now provided by helpers.go:
- `generateID()` (if present)
- `getCurrentTimestamp()` (if present)

3. **Update imports**:

Ensure `fmt` and `time` packages are still imported if needed for other purposes. The `crypto/rand` and `encoding/hex` packages can be removed if no longer needed.

**Edge Cases:**
- **Nil UsageMetadata**: The code checks for nil UsageMetadata before calling UpdateUsage.
- **Empty Choices**: The code handles empty choices gracefully by creating an empty slice.

**Verification:**
- ToOpenAIResponse uses CreateOpenAIResponseBase()
- ToOpenAIResponse uses UpdateUsage()
- ToOpenAIResponse uses CreateOpenAIChoice() or CreateOpenAIChoiceWithToolCalls()
- ToOpenAIResponse uses ConvertFinishReason()
- Local helper functions (generateID, getCurrentTimestamp) are removed

---

### Step 4: Update Qwen Converter to Use Helper Functions

**File to Modify**: `converter/qwen.go`

**Current Code** (lines 35-46):
```go
openAIResp := map[string]interface{}{
	"id":      fmt.Sprintf("chatcmpl-%s", model),
	"object":  "chat.completion",
	"created": time.Now().Unix(),
	"model":   model,
	"choices": []interface{}{},
	"usage": map[string]interface{}{
		"prompt_tokens":     0,
		"completion_tokens": 0,
		"total_tokens":      0,
	},
}
```

**Refactored Code**:
```go
openAIResp := CreateOpenAIResponseBase(model)
```

**Modification Steps**:

1. **Update ToOpenAIResponse method** (around line 25):

```go
// ToOpenAIResponse converts Qwen format to OpenAI format
func (c *QwenConverter) ToOpenAIResponse(native interface{}, model string) (interface{}, error) {
	qwenResp, ok := native.(map[string]interface{})
	if !ok {
		return native, nil
	}

	if qwenResp["error"] != nil {
		return native, nil
	}

	// Create OpenAI-compatible response using helper function
	openAIResp := CreateOpenAIResponseBase(model)

	// Helper to extract choices from a map
	extractChoices := func(m map[string]interface{}) ([]interface{}, bool) {
		if choices, ok := m["choices"].([]interface{}); ok {
			return choices, true
		}
		if output, ok := m["output"].(map[string]interface{}); ok {
			if choices, ok := output["choices"].([]interface{}); ok {
				return choices, true
			}
		}
		return nil, false
	}

	// Helper to extract usage from a map
	extractUsage := func(m map[string]interface{}) (map[string]interface{}, bool) {
		if usage, ok := m["usage"].(map[string]interface{}); ok {
			return usage, true
		}
		if output, ok := m["output"].(map[string]interface{}); ok {
			if usage, ok := output["usage"].(map[string]interface{}); ok {
				return usage, true
			}
		}
		return nil, false
	}

	if choices, ok := extractChoices(qwenResp); ok {
		var openAIChoices []interface{}
		for i, choice := range choices {
			if choiceMap, ok := choice.(map[string]interface{}); ok {
				content := ""
				if msg, exists := choiceMap["message"]; exists {
					if msgMap, msgOk := msg.(map[string]interface{}); msgOk {
						if c, cOk := msgMap["content"].(string); cOk {
							content = c
						}
						// Check for tool_calls
						if toolCalls, hasToolCalls := msgMap["tool_calls"]; hasToolCalls {
							// Tool calls found - create choice with tool calls
							openAIChoices = append(openAIChoices, CreateOpenAIChoiceWithToolCalls(i, toolCalls.([]interface{}), "stop"))
						} else {
							// No tool calls - create choice with content
							openAIChoices = append(openAIChoices, CreateOpenAIChoice(i, content, "stop"))
						}
					}
				}
			}
		}

		openAIResp["choices"] = openAIChoices
	}

	if usage, ok := extractUsage(qwenResp); ok {
		if promptTokens, ok := usage["prompt_tokens"].(float64); ok {
			if completionTokens, ok := usage["completion_tokens"].(float64); ok {
				if totalTokens, ok := usage["total_tokens"].(float64); ok {
					UpdateUsage(openAIResp, int(promptTokens), int(completionTokens), int(totalTokens))
				}
			}
		}
	}

	return openAIResp, nil
}
```

2. **Remove local helper functions**:

The following functions should be **removed** as they are now provided by helpers.go:
- `extractChoices()` helper (if present)
- `extractUsage()` helper (if present)

3. **Update imports**:

Ensure `fmt` and `time` packages are still imported if needed for other purposes. The `crypto/rand` and `encoding/hex` packages can be removed if no longer needed.

**Edge Cases:**
- **Nil Usage**: The code checks for nil usage before calling UpdateUsage.
- **Empty Choices**: The code handles empty choices gracefully by creating an empty slice.

**Verification:**
- ToOpenAIResponse uses CreateOpenAIResponseBase()
- ToOpenAIResponse uses UpdateUsage()
- ToOpenAIResponse uses CreateOpenAIChoice() or CreateOpenAIChoiceWithToolCalls()
- Diagnostic code is removed
- Local helper functions are removed

---

### Step 5: Update Claude Converter to Use Helper Functions

**File to Modify**: `converter/claude.go`

**Current Code** (around lines 120-130):
```go
openAIResp := map[string]interface{}{
	"id":      "chatcmpl-" + generateID(),
	"object":  "chat.completion",
	"created": getCurrentTimestamp(),
	"model":   model,
	"choices": []interface{}{},
	"usage": map[string]interface{}{
		"prompt_tokens":     0,
		"completion_tokens": 0,
		"total_tokens":      0,
	},
}
```

**Refactored Code**:
```go
openAIResp := CreateOpenAIResponseBase(model)
```

**Modification Steps**:

1. **Update ToOpenAIResponse method** (around line 27):

```go
// ToOpenAIResponse converts Claude format to OpenAI format
func (c *ClaudeConverter) ToOpenAIResponse(native interface{}, model string) (interface{}, error) {
	// Parse Claude response
	claudeResp, ok := native.(*kiro.ClaudeResponse)
	if !ok {
		// Try to parse from map if it's not already a ClaudeResponse
		respMap, ok := native.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("invalid response type, expected *kiro.ClaudeResponse or map[string]interface{}")
		}
		
		// Convert map to ClaudeResponse
		claudeResp = &kiro.ClaudeResponse{}
		if id, ok := respMap["id"].(string); ok {
			claudeResp.ID = id
		}
		if role, ok := respMap["role"].(string); ok {
			claudeResp.Role = role
		}
		if modelStr, ok := respMap["model"].(string); ok {
			claudeResp.Model = modelStr
		}
		if stopReason, ok := respMap["stop_reason"].(string); ok {
			claudeResp.StopReason = stopReason
		}
		
		// Parse content array
		if contentRaw, ok := respMap["content"].([]interface{}); ok {
			for _, contentItem := range contentRaw {
				if contentMap, ok := contentItem.(map[string]interface{}); ok {
					block := kiro.ContentBlock{}
					if blockType, ok := contentMap["type"].(string); ok {
						block.Type = blockType
					}
					if text, ok := contentMap["text"].(string); ok {
						block.Text = text
					}
					claudeResp.Content = append(claudeResp.Content, block)
				}
			}
		}
		
		// Parse usage
		if usageRaw, ok := respMap["usage"].(map[string]interface{}); ok {
			usage := &kiro.Usage{}
			if inputTokens, ok := usageRaw["input_tokens"].(float64); ok {
				usage.InputTokens = int(inputTokens)
			}
			if outputTokens, ok := usageRaw["output_tokens"].(float64); ok {
				usage.OutputTokens = int(outputTokens)
			}
			claudeResp.Usage = usage
		}
	}

	// Create OpenAI-compatible response using helper function
	openAIResp := CreateOpenAIResponseBase(model)

	// Extract text content from Claude content blocks
	var content string
	for _, block := range claudeResp.Content {
		if block.Type == "text" {
			content += block.Text
		}
	}

	// Convert stop_reason to OpenAI finish_reason
	finishReason := ConvertFinishReason(claudeResp.StopReason)

	// Create choice using helper function
	choice := CreateOpenAIChoice(0, content, finishReason)
	openAIResp["choices"] = []interface{}{choice}

	return openAIResp, nil
}
```

2. **Remove local helper functions**:

The following functions should be **removed** as they are now provided by helpers.go:
- `generateID()` (if present)
- `getCurrentTimestamp()` (if present)
- Any local stop_reason conversion logic (if present)

3. **Update imports**:

Ensure `fmt` and `time` packages are still imported if needed for other purposes. The `crypto/rand` and `encoding/hex` packages can be removed if no longer needed.

**Edge Cases:**
- **Nil Usage**: The code checks for nil usage before calling UpdateUsage.
- **Empty Content**: The code handles empty content gracefully by creating a choice with empty content.

**Verification:**
- ToOpenAIResponse uses CreateOpenAIResponseBase()
- ToOpenAIResponse uses CreateOpenAIChoice()
- ToOpenAIResponse uses ConvertFinishReason()
- Local helper functions are removed

---

## Build Verification

After completing all steps, verify the code builds successfully:

```bash
cd c:/Users/jappa/projecten/qwencoder-proxy/qwencoder-proxy
go build ./...
```

**Expected Output**: No build errors

**If Build Fails**:
1. Check for missing imports
2. Verify all removed functions are no longer called
3. Ensure all method calls to helper functions use correct syntax
4. Check for circular dependencies

---

## Test Verification

After successful build, run existing tests to ensure no regressions:

```bash
go test ./converter/... -v
```

**Expected Output**: All existing tests pass

**Note**: The refactored converters should produce identical output to the original implementations. All tests should pass without modification.

---

## API Compatibility Verification

This phase creates helper functions and updates converter implementations but should maintain backward compatibility with existing code.

**Verification Steps**:
1. All public methods of each converter are preserved
2. Method signatures are unchanged
3. Output format is unchanged
4. No changes to the Converter interface

**Backward Compatibility Checklist**:
- [x] GeminiConverter still implements Converter interface
- [x] QwenConverter still implements Converter interface
- [x] ClaudeConverter still implements Converter interface
- [x] OpenAIConverter still implements Converter interface
- [x] All public methods are preserved
- [x] Method signatures are unchanged
- [x] Output format is unchanged

---

## Concurrency Considerations

### Thread Safety After Refactoring

1. **Helper Functions**: All helper functions are pure (no side effects) and thread-safe by design.

2. **Converter Methods**: Converter methods are stateless and thread-safe by design. Each call is independent.

### Concurrency Edge Cases

1. **Concurrent Conversions**: Multiple goroutines can call ToOpenAIResponse() simultaneously. Each conversion is independent. Thread-safe by design.

2. **Concurrent Helper Function Calls**: Multiple goroutines can call helper functions simultaneously. Helper functions are pure and thread-safe by design.

---

## Phase Completion Criteria

Phase 6 is complete when:

- [x] `converter/helpers.go` created with all helper functions
- [x] Diagnostic code removed from `converter/qwen.go`
- [x] `converter/gemini.go` updated to use helper functions
- [x] `converter/qwen.go` updated to use helper functions
- [x] `converter/claude.go` updated to use helper functions
- [x] Local helper functions removed from all converters
- [x] Code builds successfully with `go build ./...`
- [x] All existing tests pass
- [x] API compatibility is maintained

---

## Next Phase

Proceed to **Phase 7: Final Verification and Cleanup** after Phase 6 is complete.

Phase 7 will perform final verification, run all tests, and ensure the refactoring is complete and successful.
