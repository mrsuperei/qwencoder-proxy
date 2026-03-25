package ratelimit

import "fmt"

// ErrorCode defines rate limit error codes (internal use only).
type ErrorCode string

const (
	ErrCodeRateLimitExceeded  ErrorCode = "rate_limit_exceeded"
	ErrCodeInvalidProvider    ErrorCode = "invalid_provider"
	ErrCodeConfigurationError ErrorCode = "configuration_error"
)

// ErrorType represents the type of error that occurred.
type ErrorType string

const (
	ErrorTypeRateLimit   ErrorType = "rate_limit"
	ErrorTypeServerError ErrorType = "server_error"
	ErrorTypeTimeout     ErrorType = "timeout"
	ErrorTypeAuth        ErrorType = "authentication"
	ErrorTypeNetwork     ErrorType = "network"
	ErrorTypeUnknown     ErrorType = "unknown"
)

// RateLimitError represents a rate limiting error (internal logging only).
type RateLimitError struct {
	Code       ErrorCode
	Message    string
	ProviderID string
	TokenID    string
	Reasons    []string
}

// Error implements the error interface.
func (e *RateLimitError) Error() string {
	return fmt.Sprintf("[%s] %s (provider: %s, token: %s, reasons: %v)",
		e.Code, e.Message, e.ProviderID, e.TokenID, e.Reasons)
}

// NewRateLimitExceededError creates a new rate limit exceeded error.
func NewRateLimitExceededError(providerID string, tokenID string, reasons []string) *RateLimitError {
	return &RateLimitError{
		Code:       ErrCodeRateLimitExceeded,
		Message:    "Rate limit exceeded",
		ProviderID: providerID,
		TokenID:    tokenID,
		Reasons:    reasons,
	}
}

// NewInvalidProviderError creates a new invalid provider error.
func NewInvalidProviderError(providerID string) *RateLimitError {
	return &RateLimitError{
		Code:       ErrCodeInvalidProvider,
		Message:    "Invalid provider ID",
		ProviderID: providerID,
		TokenID:    "",
		Reasons:    []string{},
	}
}

// NewConfigurationError creates a new configuration error.
func NewConfigurationError(message string) *RateLimitError {
	return &RateLimitError{
		Code:       ErrCodeConfigurationError,
		Message:    message,
		ProviderID: "",
		TokenID:    "",
		Reasons:    []string{},
	}
}

// TokenErrorInfo represents error information for a token.
type TokenErrorInfo struct {
	TokenID     string  `json:"token_id"`
	ProviderID  string  `json:"provider_id"`
	ErrorCount  int     `json:"error_count"`
	LastError   string  `json:"last_error"`
	HealthScore float64 `json:"health_score"`
	Healthy     bool    `json:"healthy"`
}
