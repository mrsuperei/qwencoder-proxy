package token

import (
	"errors"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDefaultRetryConfig tests the default retry configuration.
func TestDefaultRetryConfig(t *testing.T) {
	config := DefaultRetryConfig()

	assert.Equal(t, 10, config.MaxRetries, "MaxRetries should be 10")
	assert.Equal(t, 50*time.Millisecond, config.BaseDelay, "BaseDelay should be 50ms")
	assert.Equal(t, 2*time.Second, config.MaxDelay, "MaxDelay should be 2 seconds")
}

// TestRetryMetrics_GetRetryMetrics tests the retry metrics retrieval.
func TestRetryMetrics_GetRetryMetrics(t *testing.T) {
	metrics := &RetryMetrics{}

	// Initially all metrics should be zero
	m := metrics.GetRetryMetrics()
	assert.Equal(t, int64(0), m["total_attempts"])
	assert.Equal(t, int64(0), m["successful_retries"])
	assert.Equal(t, int64(0), m["failed_after_retry"])
	assert.Equal(t, int64(0), m["busy_errors"])

	// Increment some metrics
	metrics.TotalAttempts.Add(5)
	metrics.SuccessfulRetry.Add(3)
	metrics.BusyErrors.Add(2)

	m = metrics.GetRetryMetrics()
	assert.Equal(t, int64(5), m["total_attempts"])
	assert.Equal(t, int64(3), m["successful_retries"])
	assert.Equal(t, int64(0), m["failed_after_retry"])
	assert.Equal(t, int64(2), m["busy_errors"])
}

// TestIsSQLiteBusy tests the SQLITE_BUSY error detection.
func TestIsSQLiteBusy(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
		{
			name: "database is locked",
			err:  errors.New("database is locked"),
			want: true,
		},
		{
			name: "SQLITE_BUSY",
			err:  errors.New("SQLITE_BUSY"),
			want: true,
		},
		{
			name: "error code 5",
			err:  errors.New("error (5)"),
			want: true,
		},
		{
			name: "other error",
			err:  errors.New("some other error"),
			want: false,
		},
		{
			name: "not found error",
			err:  errors.New("not found"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isSQLiteBusy(tt.err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestExecuteWithRetry_Success tests that successful operation returns immediately.
func TestExecuteWithRetry_Success(t *testing.T) {
	store := &SQLiteStore{
		retryMetrics: &RetryMetrics{},
		retryConfig:  DefaultRetryConfig(),
		logger:       &mockLogger{},
	}

	called := 0
	err := store.executeWithRetry("testOperation", func() error {
		called++
		return nil
	})

	require.NoError(t, err)
	assert.Equal(t, 1, called, "Function should be called exactly once for success")

	metrics := store.retryMetrics.GetRetryMetrics()
	assert.Equal(t, int64(1), metrics["total_attempts"])
	assert.Equal(t, int64(0), metrics["successful_retries"])
	assert.Equal(t, int64(0), metrics["failed_after_retry"])
	assert.Equal(t, int64(0), metrics["busy_errors"])
}

// TestExecuteWithRetry_SQLiteBusy tests that SQLITE_BUSY triggers retry.
func TestExecuteWithRetry_SQLiteBusy(t *testing.T) {
	config := &RetryConfig{
		MaxRetries: 5,
		BaseDelay:  10 * time.Millisecond,
		MaxDelay:   100 * time.Millisecond,
	}

	store := &SQLiteStore{
		retryMetrics: &RetryMetrics{},
		retryConfig:  config,
		logger:       &mockLogger{},
	}

	attempts := 0
	err := store.executeWithRetry("testOperation", func() error {
		attempts++
		if attempts < 3 {
			return errors.New("database is locked")
		}
		return nil
	})

	require.NoError(t, err)
	assert.Equal(t, 3, attempts, "Function should be called 3 times (2 failures + 1 success)")

	metrics := store.retryMetrics.GetRetryMetrics()
	assert.Equal(t, int64(3), metrics["total_attempts"])
	assert.Equal(t, int64(1), metrics["successful_retries"])
	assert.Equal(t, int64(0), metrics["failed_after_retry"])
	assert.Equal(t, int64(2), metrics["busy_errors"])
}

// TestExecuteWithRetry_NonRetryableError tests that non-retryable errors return immediately.
func TestExecuteWithRetry_NonRetryableError(t *testing.T) {
	store := &SQLiteStore{
		retryMetrics: &RetryMetrics{},
		retryConfig:  DefaultRetryConfig(),
		logger:       &mockLogger{},
	}

	called := 0
	expectedErr := errors.New("some other error")
	err := store.executeWithRetry("testOperation", func() error {
		called++
		return expectedErr
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, expectedErr)
	assert.Equal(t, 1, called, "Function should be called exactly once for non-retryable error")

	metrics := store.retryMetrics.GetRetryMetrics()
	assert.Equal(t, int64(1), metrics["total_attempts"])
	assert.Equal(t, int64(0), metrics["successful_retries"])
	assert.Equal(t, int64(1), metrics["failed_after_retry"])
	assert.Equal(t, int64(0), metrics["busy_errors"])
}

// TestExecuteWithRetry_MaxRetriesExceeded tests behavior when max retries is exceeded.
func TestExecuteWithRetry_MaxRetriesExceeded(t *testing.T) {
	config := &RetryConfig{
		MaxRetries: 3,
		BaseDelay:  10 * time.Millisecond,
		MaxDelay:   100 * time.Millisecond,
	}

	store := &SQLiteStore{
		retryMetrics: &RetryMetrics{},
		retryConfig:  config,
		logger:       &mockLogger{},
	}

	attempts := 0
	busyErr := errors.New("database is locked")
	err := store.executeWithRetry("testOperation", func() error {
		attempts++
		return busyErr
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, busyErr)
	assert.Equal(t, 3, attempts, "Function should be called maxRetries times")

	metrics := store.retryMetrics.GetRetryMetrics()
	assert.Equal(t, int64(3), metrics["total_attempts"])
	assert.Equal(t, int64(0), metrics["successful_retries"])
	assert.Equal(t, int64(1), metrics["failed_after_retry"])
	assert.Equal(t, int64(2), metrics["busy_errors"]) // First 2 are busy errors
}

// TestExecuteWithRetry_ExponentialBackoff tests that exponential backoff is applied.
func TestExecuteWithRetry_ExponentialBackoff(t *testing.T) {
	config := &RetryConfig{
		MaxRetries: 5,
		BaseDelay:  10 * time.Millisecond,
		MaxDelay:   100 * time.Millisecond,
	}

	store := &SQLiteStore{
		retryMetrics: &RetryMetrics{},
		retryConfig:  config,
		logger:       &mockLogger{},
	}

	var delays []time.Duration
	startTime := time.Now()

	attempts := 0
	err := store.executeWithRetry("testOperation", func() error {
		if attempts > 0 {
			delays = append(delays, time.Since(startTime))
			startTime = time.Now()
		}
		attempts++
		if attempts < 4 {
			return errors.New("database is locked")
		}
		return nil
	})

	require.NoError(t, err)
	assert.Equal(t, 4, attempts)
	assert.Len(t, delays, 3, "Should have 3 delays for 3 retries")

	// Check that delays are increasing (exponential backoff)
	// Expected: 10ms, 20ms, 40ms (with some tolerance for execution time)
	tolerance := 15 * time.Millisecond

	assert.InDelta(t, 10*time.Millisecond, delays[0], float64(tolerance),
		"First retry delay should be ~10ms")
	assert.InDelta(t, 20*time.Millisecond, delays[1], float64(tolerance),
		"Second retry delay should be ~20ms")
	assert.InDelta(t, 40*time.Millisecond, delays[2], float64(tolerance),
		"Third retry delay should be ~40ms")
}

// TestExecuteWithRetry_MaxDelay tests that max delay is respected.
func TestExecuteWithRetry_MaxDelay(t *testing.T) {
	config := &RetryConfig{
		MaxRetries: 10,
		BaseDelay:  50 * time.Millisecond,
		MaxDelay:   100 * time.Millisecond,
	}

	store := &SQLiteStore{
		retryMetrics: &RetryMetrics{},
		retryConfig:  config,
		logger:       &mockLogger{},
	}

	var delays []time.Duration
	startTime := time.Now()

	attempts := 0
	err := store.executeWithRetry("testOperation", func() error {
		if attempts > 0 {
			delays = append(delays, time.Since(startTime))
			startTime = time.Now()
		}
		attempts++
		if attempts < 6 {
			return errors.New("database is locked")
		}
		return nil
	})

	require.NoError(t, err)
	assert.Equal(t, 6, attempts)
	assert.Len(t, delays, 5)

	// Expected: 50ms, 100ms, 100ms, 100ms, 100ms (capped at MaxDelay)
	tolerance := 20 * time.Millisecond

	assert.InDelta(t, 50*time.Millisecond, delays[0], float64(tolerance),
		"First retry delay should be ~50ms")

	// All subsequent delays should be capped at MaxDelay
	for i := 1; i < len(delays); i++ {
		assert.LessOrEqual(t, delays[i], config.MaxDelay+tolerance,
			"Retry delay %d should be capped at MaxDelay", i+1)
	}
}

// TestDefaultRetryConfig_FromEnv tests loading configuration from environment variables.
func TestDefaultRetryConfig_FromEnv(t *testing.T) {
	// Save original env values
	origEnabled := os.Getenv("SQLITE_RETRY_ENABLED")
	origMaxRetries := os.Getenv("SQLITE_MAX_RETRIES")
	origBaseDelayMs := os.Getenv("SQLITE_BASE_DELAY_MS")
	origMaxDelayMs := os.Getenv("SQLITE_MAX_DELAY_MS")

	// Clean up after test
	defer func() {
		if origEnabled != "" {
			os.Setenv("SQLITE_RETRY_ENABLED", origEnabled)
		} else {
			os.Unsetenv("SQLITE_RETRY_ENABLED")
		}
		if origMaxRetries != "" {
			os.Setenv("SQLITE_MAX_RETRIES", origMaxRetries)
		} else {
			os.Unsetenv("SQLITE_MAX_RETRIES")
		}
		if origBaseDelayMs != "" {
			os.Setenv("SQLITE_BASE_DELAY_MS", origBaseDelayMs)
		} else {
			os.Unsetenv("SQLITE_BASE_DELAY_MS")
		}
		if origMaxDelayMs != "" {
			os.Setenv("SQLITE_MAX_DELAY_MS", origMaxDelayMs)
		} else {
			os.Unsetenv("SQLITE_MAX_DELAY_MS")
		}
	}()

	// Clear all env vars
	os.Unsetenv("SQLITE_RETRY_ENABLED")
	os.Unsetenv("SQLITE_MAX_RETRIES")
	os.Unsetenv("SQLITE_BASE_DELAY_MS")
	os.Unsetenv("SQLITE_MAX_DELAY_MS")

	tests := []struct {
		name           string
		enabled        string
		maxRetries     string
		baseDelayMs    string
		maxDelayMs     string
		wantMaxRetries int
		wantBaseDelay  time.Duration
		wantMaxDelay   time.Duration
	}{
		{
			name:           "default values",
			wantMaxRetries: 10,
			wantBaseDelay:  50 * time.Millisecond,
			wantMaxDelay:   2 * time.Second,
		},
		{
			name:           "custom max retries",
			maxRetries:     "20",
			wantMaxRetries: 20,
			wantBaseDelay:  50 * time.Millisecond,
			wantMaxDelay:   2 * time.Second,
		},
		{
			name:           "custom delays",
			baseDelayMs:    "100",
			maxDelayMs:     "5000",
			wantMaxRetries: 10,
			wantBaseDelay:  100 * time.Millisecond,
			wantMaxDelay:   5 * time.Second,
		},
		{
			name:           "all custom",
			maxRetries:     "5",
			baseDelayMs:    "25",
			maxDelayMs:     "1000",
			wantMaxRetries: 5,
			wantBaseDelay:  25 * time.Millisecond,
			wantMaxDelay:   1 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear all env vars
			os.Unsetenv("SQLITE_RETRY_ENABLED")
			os.Unsetenv("SQLITE_MAX_RETRIES")
			os.Unsetenv("SQLITE_BASE_DELAY_MS")
			os.Unsetenv("SQLITE_MAX_DELAY_MS")

			// Set test values
			if tt.enabled != "" {
				os.Setenv("SQLITE_RETRY_ENABLED", tt.enabled)
			}
			if tt.maxRetries != "" {
				os.Setenv("SQLITE_MAX_RETRIES", tt.maxRetries)
			}
			if tt.baseDelayMs != "" {
				os.Setenv("SQLITE_BASE_DELAY_MS", tt.baseDelayMs)
			}
			if tt.maxDelayMs != "" {
				os.Setenv("SQLITE_MAX_DELAY_MS", tt.maxDelayMs)
			}

			config := DefaultRetryConfig()

			assert.Equal(t, tt.wantMaxRetries, config.MaxRetries)
			assert.Equal(t, tt.wantBaseDelay, config.BaseDelay)
			assert.Equal(t, tt.wantMaxDelay, config.MaxDelay)
		})
	}
}

// TestDefaultRetryConfig_Disabled tests that SQLITE_RETRY_ENABLED=false disables retries.
func TestDefaultRetryConfig_Disabled(t *testing.T) {
	// Save original env value
	origEnabled := os.Getenv("SQLITE_RETRY_ENABLED")
	defer func() {
		if origEnabled != "" {
			os.Setenv("SQLITE_RETRY_ENABLED", origEnabled)
		} else {
			os.Unsetenv("SQLITE_RETRY_ENABLED")
		}
	}()

	// Clear other env vars
	os.Unsetenv("SQLITE_MAX_RETRIES")
	os.Unsetenv("SQLITE_BASE_DELAY_MS")
	os.Unsetenv("SQLITE_MAX_DELAY_MS")

	// Test with enabled=false
	os.Setenv("SQLITE_RETRY_ENABLED", "false")
	config := DefaultRetryConfig()

	assert.Equal(t, 0, config.MaxRetries, "MaxRetries should be 0 when disabled")

	// Test with enabled=true (should use defaults)
	os.Setenv("SQLITE_RETRY_ENABLED", "true")
	config = DefaultRetryConfig()

	assert.Equal(t, 10, config.MaxRetries, "MaxRetries should be default when enabled=true")
}

// TestDefaultRetryConfig_InvalidEnv tests that invalid environment values are ignored.
func TestDefaultRetryConfig_InvalidEnv(t *testing.T) {
	// Save original env values
	origMaxRetries := os.Getenv("SQLITE_MAX_RETRIES")
	origBaseDelayMs := os.Getenv("SQLITE_BASE_DELAY_MS")
	origMaxDelayMs := os.Getenv("SQLITE_MAX_DELAY_MS")

	defer func() {
		if origMaxRetries != "" {
			os.Setenv("SQLITE_MAX_RETRIES", origMaxRetries)
		} else {
			os.Unsetenv("SQLITE_MAX_RETRIES")
		}
		if origBaseDelayMs != "" {
			os.Setenv("SQLITE_BASE_DELAY_MS", origBaseDelayMs)
		} else {
			os.Unsetenv("SQLITE_BASE_DELAY_MS")
		}
		if origMaxDelayMs != "" {
			os.Setenv("SQLITE_MAX_DELAY_MS", origMaxDelayMs)
		} else {
			os.Unsetenv("SQLITE_MAX_DELAY_MS")
		}
	}()

	// Clear all env vars
	os.Unsetenv("SQLITE_RETRY_ENABLED")
	os.Unsetenv("SQLITE_MAX_RETRIES")
	os.Unsetenv("SQLITE_BASE_DELAY_MS")
	os.Unsetenv("SQLITE_MAX_DELAY_MS")

	tests := []struct {
		name           string
		maxRetries     string
		baseDelayMs    string
		maxDelayMs     string
		wantMaxRetries int
		wantBaseDelay  time.Duration
		wantMaxDelay   time.Duration
	}{
		{
			name:           "invalid max retries - non-numeric",
			maxRetries:     "abc",
			wantMaxRetries: 10,
			wantBaseDelay:  50 * time.Millisecond,
			wantMaxDelay:   2 * time.Second,
		},
		{
			name:           "invalid base delay - non-numeric",
			baseDelayMs:    "xyz",
			wantMaxRetries: 10,
			wantBaseDelay:  50 * time.Millisecond,
			wantMaxDelay:   2 * time.Second,
		},
		{
			name:           "invalid max delay - non-numeric",
			maxDelayMs:     "foo",
			wantMaxRetries: 10,
			wantBaseDelay:  50 * time.Millisecond,
			wantMaxDelay:   2 * time.Second,
		},
		{
			name:           "negative max retries - should be ignored",
			maxRetries:     "-5",
			wantMaxRetries: 10,
			wantBaseDelay:  50 * time.Millisecond,
			wantMaxDelay:   2 * time.Second,
		},
		{
			name:           "zero base delay - should be ignored",
			baseDelayMs:    "0",
			wantMaxRetries: 10,
			wantBaseDelay:  50 * time.Millisecond,
			wantMaxDelay:   2 * time.Second,
		},
		{
			name:           "zero max delay - should be ignored",
			maxDelayMs:     "0",
			wantMaxRetries: 10,
			wantBaseDelay:  50 * time.Millisecond,
			wantMaxDelay:   2 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear all env vars
			os.Unsetenv("SQLITE_RETRY_ENABLED")
			os.Unsetenv("SQLITE_MAX_RETRIES")
			os.Unsetenv("SQLITE_BASE_DELAY_MS")
			os.Unsetenv("SQLITE_MAX_DELAY_MS")

			// Set test values
			if tt.maxRetries != "" {
				os.Setenv("SQLITE_MAX_RETRIES", tt.maxRetries)
			}
			if tt.baseDelayMs != "" {
				os.Setenv("SQLITE_BASE_DELAY_MS", tt.baseDelayMs)
			}
			if tt.maxDelayMs != "" {
				os.Setenv("SQLITE_MAX_DELAY_MS", tt.maxDelayMs)
			}

			config := DefaultRetryConfig()

			assert.Equal(t, tt.wantMaxRetries, config.MaxRetries)
			assert.Equal(t, tt.wantBaseDelay, config.BaseDelay)
			assert.Equal(t, tt.wantMaxDelay, config.MaxDelay)
		})
	}
}

// mockLogger is a mock implementation of logging.Logger for testing.
type mockLogger struct{}

func (m *mockLogger) DebugLog(format string, args ...interface{}) {}
func (m *mockLogger) InfoLog(format string, args ...interface{})  {}
func (m *mockLogger) WarnLog(format string, args ...interface{})  {}
func (m *mockLogger) ErrorLog(format string, args ...interface{}) {}
