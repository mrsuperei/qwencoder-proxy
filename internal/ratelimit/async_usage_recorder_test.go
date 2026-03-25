package ratelimit

import (
	"context"
	"database/sql"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

// asyncMockUsageTracker is a mock implementation of UsageTracker for testing.
type asyncMockUsageTracker struct {
	recordUsageCalled      atomic.Int64
	recordModelUsageCalled atomic.Int64
	recordUsageError       error
	recordModelUsageError  error
}

func (m *asyncMockUsageTracker) RecordUsage(ctx context.Context, providerID string, tokenID string, requestCount int, tokenCount int) error {
	m.recordUsageCalled.Add(1)
	return m.recordUsageError
}

func (m *asyncMockUsageTracker) GetProviderUsage(ctx context.Context, providerID string) (*UsageMetrics, error) {
	return &UsageMetrics{}, nil
}

func (m *asyncMockUsageTracker) GetTokenUsage(ctx context.Context, providerID string, tokenID string) (*UsageMetrics, error) {
	return &UsageMetrics{}, nil
}

func (m *asyncMockUsageTracker) GetAllProviderUsage(ctx context.Context) (map[string]*UsageMetrics, error) {
	return make(map[string]*UsageMetrics), nil
}

func (m *asyncMockUsageTracker) ResetUsage(ctx context.Context, providerID string, tokenID string) error {
	return nil
}

func (m *asyncMockUsageTracker) CleanupOrphanedUsageRecords(ctx context.Context) error {
	return nil
}

func (m *asyncMockUsageTracker) GetDB() *sql.DB {
	return nil
}

func (m *asyncMockUsageTracker) Close() error {
	return nil
}

func (m *asyncMockUsageTracker) RecordModelUsage(ctx context.Context, providerID string, tokenID string, model string, inputTokens int, outputTokens int) error {
	m.recordModelUsageCalled.Add(1)
	return m.recordModelUsageError
}

func (m *asyncMockUsageTracker) GetModelUsage(ctx context.Context, providerID string, providerID2 string, model string) (*ModelUsageMetrics, error) {
	return &ModelUsageMetrics{}, nil
}

func (m *asyncMockUsageTracker) GetAllModelUsage(ctx context.Context, providerID string) (map[string]*ModelUsageMetrics, error) {
	return make(map[string]*ModelUsageMetrics), nil
}

func (m *asyncMockUsageTracker) GetTokenModelUsage(ctx context.Context, tokenID string) (map[string]*ModelUsageMetrics, error) {
	return make(map[string]*ModelUsageMetrics), nil
}

func (m *asyncMockUsageTracker) GetProviderModelUsage(ctx context.Context, providerID string) (map[string]*ModelUsageMetrics, error) {
	return make(map[string]*ModelUsageMetrics), nil
}

func (m *asyncMockUsageTracker) ResetModelUsage(ctx context.Context, providerID string, tokenID string, model string) error {
	return nil
}

func (m *asyncMockUsageTracker) GetRequestHistory(ctx context.Context, filter *RequestHistoryFilter, page int, pageSize int) (*RequestHistoryResponse, error) {
	return &RequestHistoryResponse{}, nil
}

func (m *asyncMockUsageTracker) GetRequestHistoryByToken(ctx context.Context, tokenID string, filter *RequestHistoryFilter, page int, pageSize int) (*RequestHistoryResponse, error) {
	return &RequestHistoryResponse{}, nil
}

func (m *asyncMockUsageTracker) GetRequestHistoryByModel(ctx context.Context, model string, filter *RequestHistoryFilter, page int, pageSize int) (*RequestHistoryResponse, error) {
	return &RequestHistoryResponse{}, nil
}

func (m *asyncMockUsageTracker) GetRequestHistorySummary(ctx context.Context, tokenID string, model string, startTime int64, endTime int64) (*RequestHistorySummary, error) {
	return &RequestHistorySummary{}, nil
}

func (m *asyncMockUsageTracker) DeleteRequestHistory(ctx context.Context, filter *RequestHistoryFilter) (int64, error) {
	return 0, nil
}

// asyncMockLogger is a mock implementation of logging.Logger for testing.
type asyncMockLogger struct {
	infoMessages  []string
	warnMessages  []string
	errorMessages []string
}

func (m *asyncMockLogger) InfoLog(msg string, args ...interface{}) {
	m.infoMessages = append(m.infoMessages, msg)
}

func (m *asyncMockLogger) DebugLog(msg string, args ...interface{}) {
	// Ignore debug logs in tests
}

func (m *asyncMockLogger) WarnLog(msg string, args ...interface{}) {
	m.warnMessages = append(m.warnMessages, msg)
}

func (m *asyncMockLogger) ErrorLog(msg string, args ...interface{}) {
	m.errorMessages = append(m.errorMessages, msg)
}

func TestNewAsyncUsageRecorder(t *testing.T) {
	tests := []struct {
		name    string
		config  *AsyncUsageRecorderConfig
		enabled bool
	}{
		{
			name:    "disabled by default",
			config:  DefaultAsyncUsageRecorderConfig(),
			enabled: false,
		},
		{
			name: "enabled with custom config",
			config: &AsyncUsageRecorderConfig{
				Enabled:     true,
				WorkerCount: 3,
				QueueSize:   500,
				RetryLimit:  5,
				RetryDelay:  200 * time.Millisecond,
			},
			enabled: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockTracker := &asyncMockUsageTracker{}
			mockLogger := &asyncMockLogger{}

			recorder := NewAsyncUsageRecorder(mockTracker, mockLogger, tt.config)

			if recorder.IsEnabled() != tt.enabled {
				t.Errorf("Expected enabled=%v, got %v", tt.enabled, recorder.IsEnabled())
			}

			if recorder.config != tt.config {
				t.Errorf("Config not set correctly")
			}

			recorder.Close()
		})
	}
}

func TestRecordUsageAsync(t *testing.T) {
	tests := []struct {
		name        string
		enabled     bool
		expectAsync bool
	}{
		{
			name:        "async enabled",
			enabled:     true,
			expectAsync: true,
		},
		{
			name:        "async disabled - synchronous fallback",
			enabled:     false,
			expectAsync: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockTracker := &asyncMockUsageTracker{}
			mockLogger := &asyncMockLogger{}

			config := &AsyncUsageRecorderConfig{
				Enabled:     tt.enabled,
				WorkerCount: 2,
				QueueSize:   10,
				RetryLimit:  2,
				RetryDelay:  50 * time.Millisecond,
			}

			recorder := NewAsyncUsageRecorder(mockTracker, mockLogger, config)
			defer recorder.Close()

			// Give workers time to start
			time.Sleep(10 * time.Millisecond)

			err := recorder.RecordUsageAsync("test-provider", "test-token", 1, 100, 50)

			if tt.expectAsync {
				// Async mode - should return immediately
				if err != nil {
					t.Errorf("Expected no error, got %v", err)
				}

				// Wait for job to be processed
				time.Sleep(100 * time.Millisecond)

				if mockTracker.recordUsageCalled.Load() == 0 {
					t.Error("RecordUsage was not called")
				}
			} else {
				// Synchronous mode
				if err != nil {
					t.Errorf("Expected no error, got %v", err)
				}

				if mockTracker.recordUsageCalled.Load() == 0 {
					t.Error("RecordUsage was not called")
				}
			}
		})
	}
}

func TestRecordModelUsageAsync(t *testing.T) {
	tests := []struct {
		name        string
		enabled     bool
		expectAsync bool
	}{
		{
			name:        "async enabled",
			enabled:     true,
			expectAsync: true,
		},
		{
			name:        "async disabled - synchronous fallback",
			enabled:     false,
			expectAsync: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockTracker := &asyncMockUsageTracker{}
			mockLogger := &asyncMockLogger{}

			config := &AsyncUsageRecorderConfig{
				Enabled:     tt.enabled,
				WorkerCount: 2,
				QueueSize:   10,
				RetryLimit:  2,
				RetryDelay:  50 * time.Millisecond,
			}

			recorder := NewAsyncUsageRecorder(mockTracker, mockLogger, config)
			defer recorder.Close()

			// Give workers time to start
			time.Sleep(10 * time.Millisecond)

			err := recorder.RecordModelUsageAsync("test-provider", "test-token", "test-model", 100, 50)

			if tt.expectAsync {
				// Async mode - should return immediately
				if err != nil {
					t.Errorf("Expected no error, got %v", err)
				}

				// Wait for job to be processed
				time.Sleep(100 * time.Millisecond)

				if mockTracker.recordModelUsageCalled.Load() == 0 {
					t.Error("RecordModelUsage was not called")
				}
			} else {
				// Synchronous mode
				if err != nil {
					t.Errorf("Expected no error, got %v", err)
				}

				if mockTracker.recordModelUsageCalled.Load() == 0 {
					t.Error("RecordModelUsage was not called")
				}
			}
		})
	}
}

func TestRetryLogic(t *testing.T) {
	mockTracker := &asyncMockUsageTracker{}
	mockLogger := &asyncMockLogger{}

	// Set error for first few calls, then succeed
	mockTracker.recordUsageError = errors.New("transient error")

	config := &AsyncUsageRecorderConfig{
		Enabled:     true,
		WorkerCount: 1,
		QueueSize:   10,
		RetryLimit:  3,
		RetryDelay:  20 * time.Millisecond,
	}

	recorder := NewAsyncUsageRecorder(mockTracker, mockLogger, config)
	defer recorder.Close()

	// Give worker time to start
	time.Sleep(10 * time.Millisecond)

	// Simulate transient error that resolves after retries
	go func() {
		time.Sleep(50 * time.Millisecond)
		mockTracker.recordUsageError = nil
	}()

	err := recorder.RecordUsageAsync("test-provider", "test-token", 1, 100, 50)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Wait for retries to complete
	time.Sleep(200 * time.Millisecond)

	metrics := recorder.GetMetrics()
	if metrics.JobsProcessed == 0 {
		t.Error("Expected jobs to be processed after retries")
	}
}

func TestPermanentError(t *testing.T) {
	mockTracker := &asyncMockUsageTracker{}
	mockLogger := &asyncMockLogger{}

	// Set permanent error (foreign key violation)
	mockTracker.recordUsageError = errors.New("FOREIGN KEY constraint failed")

	config := &AsyncUsageRecorderConfig{
		Enabled:     true,
		WorkerCount: 1,
		QueueSize:   10,
		RetryLimit:  3,
		RetryDelay:  20 * time.Millisecond,
	}

	recorder := NewAsyncUsageRecorder(mockTracker, mockLogger, config)
	defer recorder.Close()

	// Give worker time to start
	time.Sleep(10 * time.Millisecond)

	err := recorder.RecordUsageAsync("test-provider", "test-token", 1, 100, 50)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Wait for job to fail
	time.Sleep(100 * time.Millisecond)

	metrics := recorder.GetMetrics()
	if metrics.JobsFailed == 0 {
		t.Error("Expected job to fail with permanent error")
	}
}

func TestQueueOverflow(t *testing.T) {
	mockTracker := &asyncMockUsageTracker{}
	mockLogger := &asyncMockLogger{}

	config := &AsyncUsageRecorderConfig{
		Enabled:     true,
		WorkerCount: 1,
		QueueSize:   2,
		RetryLimit:  1,
		RetryDelay:  50 * time.Millisecond,
	}

	recorder := NewAsyncUsageRecorder(mockTracker, mockLogger, config)
	defer recorder.Close()

	// Give worker time to start
	time.Sleep(10 * time.Millisecond)

	// Fill queue
	for i := 0; i < 10; i++ {
		recorder.RecordUsageAsync("test-provider", "test-token", 1, 100, 50)
	}

	// Wait for some jobs to be processed
	time.Sleep(100 * time.Millisecond)

	metrics := recorder.GetMetrics()
	if metrics.JobsProcessed == 0 {
		t.Error("Expected some jobs to be processed")
	}

	if metrics.JobsFailed == 0 {
		t.Error("Expected some jobs to fail due to queue overflow")
	}
}

func TestGetMetrics(t *testing.T) {
	mockTracker := &asyncMockUsageTracker{}
	mockLogger := &asyncMockLogger{}

	config := &AsyncUsageRecorderConfig{
		Enabled:     true,
		WorkerCount: 3,
		QueueSize:   100,
		RetryLimit:  2,
		RetryDelay:  100 * time.Millisecond,
	}

	recorder := NewAsyncUsageRecorder(mockTracker, mockLogger, config)
	defer recorder.Close()

	// Give workers time to start
	time.Sleep(10 * time.Millisecond)

	metrics := recorder.GetMetrics()

	if metrics.Enabled != true {
		t.Error("Expected enabled=true")
	}
	if metrics.WorkerCount != 3 {
		t.Errorf("Expected WorkerCount=3, got %d", metrics.WorkerCount)
	}
	if metrics.QueueCapacity != 100 {
		t.Errorf("Expected QueueCapacity=100, got %d", metrics.QueueCapacity)
	}
}

func TestGracefulShutdown(t *testing.T) {
	mockTracker := &asyncMockUsageTracker{}
	mockLogger := &asyncMockLogger{}

	config := &AsyncUsageRecorderConfig{
		Enabled:     true,
		WorkerCount: 2,
		QueueSize:   10,
		RetryLimit:  2,
		RetryDelay:  20 * time.Millisecond,
	}

	recorder := NewAsyncUsageRecorder(mockTracker, mockLogger, config)

	// Give workers time to start
	time.Sleep(10 * time.Millisecond)

	// Submit some jobs
	for i := 0; i < 5; i++ {
		recorder.RecordUsageAsync("test-provider", "test-token", 1, 100, 50)
	}

	// Wait for some jobs to be queued
	time.Sleep(10 * time.Millisecond)

	// Close should wait for all jobs to complete
	start := time.Now()
	err := recorder.Close()
	duration := time.Since(start)

	if err != nil {
		t.Errorf("Close returned error: %v", err)
	}

	// Should complete within reasonable time (< 1 second)
	if duration > time.Second {
		t.Errorf("Close took too long: %v", duration)
	}

	metrics := recorder.GetMetrics()
	if metrics.JobsProcessed == 0 {
		t.Error("Expected jobs to be processed before shutdown")
	}
}

func TestConcurrentJobs(t *testing.T) {
	mockTracker := &asyncMockUsageTracker{}
	mockLogger := &asyncMockLogger{}

	config := &AsyncUsageRecorderConfig{
		Enabled:     true,
		WorkerCount: 5,
		QueueSize:   100,
		RetryLimit:  2,
		RetryDelay:  20 * time.Millisecond,
	}

	recorder := NewAsyncUsageRecorder(mockTracker, mockLogger, config)
	defer recorder.Close()

	// Give workers time to start
	time.Sleep(10 * time.Millisecond)

	// Submit many concurrent jobs
	numJobs := 50
	for i := 0; i < numJobs; i++ {
		recorder.RecordUsageAsync("test-provider", "test-token", 1, 100, 50)
	}

	// Wait for all jobs to be processed
	time.Sleep(500 * time.Millisecond)

	metrics := recorder.GetMetrics()

	if metrics.JobsProcessed < int64(numJobs) {
		t.Errorf("Expected %d jobs processed, got %d", numJobs, metrics.JobsProcessed)
	}

	if metrics.JobsFailed > 0 {
		t.Errorf("Expected no failures, got %d", metrics.JobsFailed)
	}
}
