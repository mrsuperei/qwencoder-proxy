// Package converter provides format conversion between different LLM API formats
package converter

import (
	"github.com/sunbankio/qwencoder-proxy/provider"
)

// QwenConverter handles Qwen to/from OpenAI format conversions
type QwenConverter struct{}

// NewQwenConverter creates a new Qwen converter
func NewQwenConverter() *QwenConverter {
	return &QwenConverter{}
}

// ToOpenAIRequest converts Qwen format to OpenAI format
// ToOpenAIRequest is a placeholder implementation for future extensibility.
// Currently returns the request unchanged as no conversion is needed.
func (c *QwenConverter) ToOpenAIRequest(native interface{}) (interface{}, error) {
	return native, nil
}

// ToOpenAIResponse converts Qwen format to OpenAI format
func (c *QwenConverter) ToOpenAIResponse(native interface{}, model string) (interface{}, error) {
	qwenResp, ok := native.(map[string]interface{})
	if !ok {
		return native, nil
	}

	if qwenResp["error"] != nil {
		return native, nil
	}

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
						if c, ok := msgMap["content"].(string); ok {
							content = c
						}
					}
				}

				finishReason := "stop"
				if reason, ok := choiceMap["finish_reason"].(string); ok {
					finishReason = reason
				}

				openAIChoice := CreateOpenAIChoice(i, content, finishReason)

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

	if usage, ok := extractUsage(qwenResp); ok {
		targetUsage := openAIResp["usage"].(map[string]interface{})
		keys := []string{"prompt_tokens", "completion_tokens", "total_tokens"}
		for _, key := range keys {
			if val, ok := usage[key]; ok {
				if fval, ok := val.(float64); ok {
					targetUsage[key] = int(fval)
				} else if ival, ok := val.(int); ok {
					targetUsage[key] = ival
				}
			}
		}
	}

	return openAIResp, nil
}

// ToOpenAIStreamChunk converts Qwen format to OpenAI format
func (c *QwenConverter) ToOpenAIStreamChunk(native interface{}, model string) (interface{}, error) {
	return native, nil
}

// FromOpenAIRequest converts OpenAI format to Qwen format
// FromOpenAIRequest is a placeholder implementation for future extensibility.
// Currently returns the request unchanged as no conversion is needed.
func (c *QwenConverter) FromOpenAIRequest(req interface{}) (interface{}, error) {
	return req, nil
}

// FromOpenAIResponse converts OpenAI format to Qwen format
// FromOpenAIResponse is a placeholder implementation for future extensibility.
// Currently returns the response unchanged as no conversion is needed.
func (c *QwenConverter) FromOpenAIResponse(resp interface{}) (interface{}, error) {
	return resp, nil
}

// Protocol returns the native protocol
func (c *QwenConverter) Protocol() provider.ProtocolType {
	return provider.ProtocolQwen
}
