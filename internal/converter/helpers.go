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

// GetString safely extracts a string value from a map
func GetString(m map[string]interface{}, key string) string {
	if val, ok := m[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}

// ConvertContent converts content interface to string
func ConvertContent(content interface{}) string {
	if contentStr, ok := content.(string); ok {
		return contentStr
	}

	// Handle content as array of content parts (for vision models)
	if contentArr, ok := content.([]interface{}); ok {
		var result string
		for _, part := range contentArr {
			if partMap, ok := part.(map[string]interface{}); ok {
				if text, exists := partMap["text"]; exists {
					if textStr, ok := text.(string); ok {
						result += textStr
					}
				}
			}
		}
		return result
	}

	return fmt.Sprintf("%v", content)
}
