package ratelimit

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"github.com/sunbankio/qwencoder-proxy/internal/logging"
)

// tokenCounterImpl extracts and counts tokens from provider responses.
type tokenCounterImpl struct {
	logger logging.Logger
}

// NewTokenCounter creates a new token counter.
func NewTokenCounter(logger logging.Logger) TokenCounter {
	return &tokenCounterImpl{
		logger: logger,
	}
}

// ExtractTokensFromResponse extracts token counts from provider response.
// This is the primary method - uses provider-reported data for accuracy.
func (tc *tokenCounterImpl) ExtractTokensFromResponse(response map[string]interface{}) (inputTokens int, outputTokens int, totalTokens int, err error) {
	// Check for usage information in response (standard OpenAI format).
	if usage, ok := response["usage"].(map[string]interface{}); ok {
		if promptTokens, ok := usage["prompt_tokens"].(float64); ok {
			inputTokens = int(promptTokens)
		}
		if completionTokens, ok := usage["completion_tokens"].(float64); ok {
			outputTokens = int(completionTokens)
		}
		if total, ok := usage["total_tokens"].(float64); ok {
			totalTokens = int(total)
		}

		// Validate we have all three values.
		if inputTokens > 0 && outputTokens > 0 && totalTokens > 0 {
			tc.logger.DebugLog("[TokenCounter] Extracted tokens from provider: input=%d, output=%d, total=%d",
				inputTokens, outputTokens, totalTokens)
			return inputTokens, outputTokens, totalTokens, nil
		}
	}

	// Fallback: estimate from response content if no usage info.
	tc.logger.WarnLog("[TokenCounter] No usage info in provider response, falling back to estimation")

	// Try to extract from choices content.
	choices, ok := response["choices"].([]interface{})
	if !ok {
		return 0, 0, 0, fmt.Errorf("no choices in response and no usage information")
	}

	outputTokens = 0
	for _, choice := range choices {
		choiceMap, ok := choice.(map[string]interface{})
		if !ok {
			continue
		}

		message, ok := choiceMap["message"].(map[string]interface{})
		if !ok {
			continue
		}

		content, ok := message["content"].(string)
		if !ok {
			continue
		}

		outputTokens += tc.estimateTokens(content)
	}

	// Input tokens would need to be estimated from original request.
	// This is a limitation when provider doesn't return usage info.
	totalTokens = outputTokens

	tc.logger.DebugLog("[TokenCounter] Estimated tokens from response: output=%d", outputTokens)
	return 0, outputTokens, totalTokens, nil
}

// EstimateInputTokens estimates the number of tokens in the input request.
// This is used ONLY as a fallback when provider doesn't return usage info
// or for pre-request quota checking (conservative estimate).
func (tc *tokenCounterImpl) EstimateInputTokens(openaiReq map[string]interface{}) (int, error) {
	messages, ok := openaiReq["messages"].([]interface{})
	if !ok {
		return 0, fmt.Errorf("invalid messages format")
	}

	totalTokens := 0

	for _, msg := range messages {
		msgMap, ok := msg.(map[string]interface{})
		if !ok {
			continue
		}

		content, ok := msgMap["content"]
		if !ok {
			continue
		}

		// Handle different content formats.
		switch v := content.(type) {
		case string:
			totalTokens += tc.estimateTokens(v)
		case []interface{}:
			// Multi-modal content (text + images).
			for _, item := range v {
				itemMap, ok := item.(map[string]interface{})
				if !ok {
					continue
				}

				if itemType, ok := itemMap["type"].(string); ok && itemType == "text" {
					if text, ok := itemMap["text"].(string); ok {
						totalTokens += tc.estimateTokens(text)
					}
				} else if itemType == "image_url" {
					// Images typically cost ~85-565 tokens depending on resolution.
					// Use conservative estimate.
					totalTokens += 85
				}
			}
		}
	}

	// Add tokens for system message overhead, formatting, etc.
	// Conservative estimate: ~10 tokens per message for formatting.
	totalTokens += len(messages) * 10

	tc.logger.DebugLog("[TokenCounter] Estimated %d input tokens for request (fallback)", totalTokens)
	return totalTokens, nil
}

// estimateTokens provides a rough estimate of token count for text.
// Uses character-based approximation: ~4 characters per token (English).
// NOTE: This is a fallback - prefer provider-reported token counts.
func (tc *tokenCounterImpl) estimateTokens(text string) int {
	if text == "" {
		return 0
	}

	// Remove whitespace for more accurate count.
	cleanText := strings.Join(strings.Fields(text), "")

	// Estimate: ~4 characters per token for English text.
	// This is a conservative estimate; actual tokenization varies by model.
	estimatedTokens := int(math.Ceil(float64(len(cleanText)) / 4.0))

	return estimatedTokens
}

// CountStreamingOutputTokens counts tokens from streaming response chunks.
// This is called during streaming to track real-time token usage.
// Note: For streaming, we still rely on final usage counts from provider.
func (tc *tokenCounterImpl) CountStreamingOutputTokens(chunk []byte) (int, error) {
	// Parse SSE chunk.
	chunkStr := string(chunk)
	if !strings.HasPrefix(chunkStr, "data: ") {
		return 0, nil
	}

	dataStr := strings.TrimPrefix(chunkStr, "data: ")
	if dataStr == "[DONE]" {
		return 0, nil
	}

	var chunkData map[string]interface{}
	if err := json.Unmarshal([]byte(dataStr), &chunkData); err != nil {
		return 0, fmt.Errorf("failed to parse chunk: %w", err)
	}

	// Check for usage information in streaming chunk.
	if usage, ok := chunkData["usage"].(map[string]interface{}); ok {
		if completionTokens, ok := usage["completion_tokens"].(float64); ok {
			return int(completionTokens), nil
		}
	}

	// Fallback: estimate from delta content.
	choices, ok := chunkData["choices"].([]interface{})
	if !ok || len(choices) == 0 {
		return 0, nil
	}

	choice, ok := choices[0].(map[string]interface{})
	if !ok {
		return 0, nil
	}

	delta, ok := choice["delta"].(map[string]interface{})
	if !ok {
		return 0, nil
	}

	content, ok := delta["content"].(string)
	if !ok {
		return 0, nil
	}

	// Estimate tokens in this chunk (fallback only).
	tokens := tc.estimateTokens(content)

	return tokens, nil
}
