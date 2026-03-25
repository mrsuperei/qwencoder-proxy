package token

import (
	"os"
	"strconv"
	"sync/atomic"
	"time"
)

// RetryConfig holds retry configuration for database operations.
type RetryConfig struct {
	MaxRetries int           // Maximum number of retry attempts
	BaseDelay  time.Duration // Base delay for exponential backoff
	MaxDelay   time.Duration // Maximum delay between retries
}

// DefaultRetryConfig returns default retry configuration.
// Configuration can be overridden via environment variables:
//   - SQLITE_RETRY_ENABLED: Set to "false" to disable retries (default: enabled)
//   - SQLITE_MAX_RETRIES: Maximum number of retry attempts (default: 10)
//   - SQLITE_BASE_DELAY_MS: Base delay in milliseconds (default: 50)
//   - SQLITE_MAX_DELAY_MS: Maximum delay in milliseconds (default: 2000)
func DefaultRetryConfig() *RetryConfig {
	config := &RetryConfig{
		MaxRetries: 10,
		BaseDelay:  50 * time.Millisecond,
		MaxDelay:   2 * time.Second,
	}

	// Check if retries are disabled
	if enabled := os.Getenv("SQLITE_RETRY_ENABLED"); enabled == "false" {
		config.MaxRetries = 0
		return config
	}

	// Load MaxRetries from environment
	if maxRetries := os.Getenv("SQLITE_MAX_RETRIES"); maxRetries != "" {
		if n, err := strconv.Atoi(maxRetries); err == nil && n >= 0 {
			config.MaxRetries = n
		}
	}

	// Load BaseDelay from environment
	if baseDelayMs := os.Getenv("SQLITE_BASE_DELAY_MS"); baseDelayMs != "" {
		if n, err := strconv.Atoi(baseDelayMs); err == nil && n > 0 {
			config.BaseDelay = time.Duration(n) * time.Millisecond
		}
	}

	// Load MaxDelay from environment
	if maxDelayMs := os.Getenv("SQLITE_MAX_DELAY_MS"); maxDelayMs != "" {
		if n, err := strconv.Atoi(maxDelayMs); err == nil && n > 0 {
			config.MaxDelay = time.Duration(n) * time.Millisecond
		}
	}

	return config
}

// RetryMetrics tracks retry statistics.
type RetryMetrics struct {
	TotalAttempts    atomic.Int64
	SuccessfulRetry  atomic.Int64
	FailedAfterRetry atomic.Int64
	BusyErrors       atomic.Int64
}

// GetRetryMetrics returns current retry metrics.
func (m *RetryMetrics) GetRetryMetrics() map[string]int64 {
	return map[string]int64{
		"total_attempts":     m.TotalAttempts.Load(),
		"successful_retries": m.SuccessfulRetry.Load(),
		"failed_after_retry": m.FailedAfterRetry.Load(),
		"busy_errors":        m.BusyErrors.Load(),
	}
}
