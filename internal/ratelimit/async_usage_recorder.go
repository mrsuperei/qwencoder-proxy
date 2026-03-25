package ratelimit

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/sunbankio/qwencoder-proxy/internal/logging"
)

// UsageRecordingJob represents a job for async usage recording.
type UsageRecordingJob struct {
	JobID        string
	ProviderID   string
	TokenID      string
	Model        string // Optional, for model-specific tracking
	RequestCount int
	InputTokens  int
	OutputTokens int
	Timestamp    time.Time
	RetryCount   int
}

// AsyncUsageRecorderConfig configures the async usage recorder.
type AsyncUsageRecorderConfig struct {
	Enabled     bool          // Enable async recording (default: false)
	WorkerCount int           // Number of worker goroutines (default: 1)
	QueueSize   int           // Channel buffer size (default: 1000)
	RetryLimit  int           // Max retries per job (default: 3)
	RetryDelay  time.Duration // Delay between retries (default: 100ms)
}

// DefaultAsyncUsageRecorderConfig returns default configuration.
func DefaultAsyncUsageRecorderConfig() *AsyncUsageRecorderConfig {
	return &AsyncUsageRecorderConfig{
		Enabled:     false,
		WorkerCount: 1,
		QueueSize:   1000,
		RetryLimit:  3,
		RetryDelay:  100 * time.Millisecond,
	}
}

// AsyncUsageRecorder handles async usage recording with a worker pool.
type AsyncUsageRecorder struct {
	usageTracker UsageTracker
	logger       logging.Logger
	config       *AsyncUsageRecorderConfig

	// Worker pool
	jobQueue    chan UsageRecordingJob
	workerCount int
	wg          sync.WaitGroup

	// Context for graceful shutdown
	ctx    context.Context
	cancel context.CancelFunc

	// Metrics
	jobsProcessed atomic.Int64
	jobsFailed    atomic.Int64
	queueSize     atomic.Int64
}

// NewAsyncUsageRecorder creates a new async usage recorder.
func NewAsyncUsageRecorder(
	usageTracker UsageTracker,
	logger logging.Logger,
	config *AsyncUsageRecorderConfig,
) *AsyncUsageRecorder {
	if config == nil {
		config = DefaultAsyncUsageRecorderConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())

	recorder := &AsyncUsageRecorder{
		usageTracker: usageTracker,
		logger:       logger,
		config:       config,
		jobQueue:     make(chan UsageRecordingJob, config.QueueSize),
		workerCount:  config.WorkerCount,
		ctx:          ctx,
		cancel:       cancel,
	}

	// Start worker pool if enabled
	if config.Enabled {
		recorder.startWorkers()
		logger.InfoLog("[AsyncUsageRecorder] Started with %d workers, queue size: %d",
			config.WorkerCount, config.QueueSize)
	} else {
		logger.InfoLog("[AsyncUsageRecorder] Disabled, using synchronous recording")
	}

	return recorder
}

// startWorkers starts the worker pool goroutines.
func (ar *AsyncUsageRecorder) startWorkers() {
	for i := 0; i < ar.workerCount; i++ {
		ar.wg.Add(1)
		go ar.worker(i)
	}
}

// worker processes jobs from the job queue.
func (ar *AsyncUsageRecorder) worker(id int) {
	defer ar.wg.Done()

	ar.logger.DebugLog("[AsyncUsageRecorder] Worker %d started", id)

	for {
		select {
		case <-ar.ctx.Done():
			ar.logger.DebugLog("[AsyncUsageRecorder] Worker %d shutting down", id)
			return
		case job, ok := <-ar.jobQueue:
			if !ok {
				ar.logger.DebugLog("[AsyncUsageRecorder] Worker %d: job queue closed", id)
				return
			}
			ar.processJob(job, id)
		}
	}
}

// processJob processes a single usage recording job.
func (ar *AsyncUsageRecorder) processJob(job UsageRecordingJob, workerID int) {
	ar.logger.DebugLog("[AsyncUsageRecorder] Worker %d processing job %s", workerID, job.JobID)

	err := ar.executeJob(job)
	if err != nil {
		ar.handleJobError(job, workerID, err)
		return
	}

	ar.jobsProcessed.Add(1)
	ar.queueSize.Add(-1)
	ar.logger.DebugLog("[AsyncUsageRecorder] Worker %d completed job %s", workerID, job.JobID)
}

// executeJob executes the usage recording operation.
func (ar *AsyncUsageRecorder) executeJob(job UsageRecordingJob) error {
	ctx := context.Background()

	if job.Model != "" {
		// Model-specific usage recording
		return ar.usageTracker.RecordModelUsage(
			ctx,
			job.ProviderID,
			job.TokenID,
			job.Model,
			job.InputTokens,
			job.OutputTokens,
		)
	}

	// Regular usage recording
	totalTokens := job.InputTokens + job.OutputTokens
	return ar.usageTracker.RecordUsage(
		ctx,
		job.ProviderID,
		job.TokenID,
		job.RequestCount,
		totalTokens,
	)
}

// handleJobError handles job processing errors with retry logic.
func (ar *AsyncUsageRecorder) handleJobError(job UsageRecordingJob, workerID int, err error) {
	// Check if this is a permanent error
	if ar.isPermanentError(err) {
		ar.logger.ErrorLog("[AsyncUsageRecorder] Worker %d: Permanent error for job %s: %v",
			workerID, job.JobID, err)
		ar.jobsFailed.Add(1)
		ar.queueSize.Add(-1)
		return
	}

	// Check retry limit
	if job.RetryCount >= ar.config.RetryLimit {
		ar.logger.ErrorLog("[AsyncUsageRecorder] Worker %d: Max retries exceeded for job %s: %v",
			workerID, job.JobID, err)
		ar.jobsFailed.Add(1)
		ar.queueSize.Add(-1)
		return
	}

	// Retry with exponential backoff
	job.RetryCount++
	retryDelay := ar.config.RetryDelay * time.Duration(1<<(job.RetryCount-1))

	ar.logger.WarnLog("[AsyncUsageRecorder] Worker %d: Retrying job %s (attempt %d/%d) after %v: %v",
		workerID, job.JobID, job.RetryCount, ar.config.RetryLimit, retryDelay, err)

	// Schedule retry
	go func() {
		select {
		case <-ar.ctx.Done():
			return
		case <-time.After(retryDelay):
			select {
			case ar.jobQueue <- job:
				ar.logger.DebugLog("[AsyncUsageRecorder] Job %s requeued for retry", job.JobID)
			case <-ar.ctx.Done():
				ar.logger.WarnLog("[AsyncUsageRecorder] Job %s retry cancelled due to shutdown", job.JobID)
			}
		}
	}()
}

// isPermanentError checks if an error is permanent (non-retryable).
func (ar *AsyncUsageRecorder) isPermanentError(err error) bool {
	if err == nil {
		return false
	}

	// Check for specific error types that indicate permanent failures
	errStr := err.Error()

	// Foreign key violations (token deleted)
	if contains(errStr, "FOREIGN KEY") || contains(errStr, "foreign key") {
		return true
	}

	// Schema errors
	if contains(errStr, "no such table") || contains(errStr, "column") {
		return true
	}

	return false
}

// contains checks if a string contains a substring.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr ||
		len(s) > len(substr) && (s[:len(substr)] == substr ||
			s[len(s)-len(substr):] == substr ||
			containsMiddle(s, substr)))
}

// containsMiddle checks if substring is in the middle of string.
func containsMiddle(s, substr string) bool {
	for i := 1; i < len(s)-len(substr)+1; i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// RecordUsageAsync records usage asynchronously.
// DEPRECATED: Use RecordModelUsageAsync instead for complete tracking.
// This method is kept for backward compatibility but may be removed in future versions.
// Returns immediately without blocking on database writes.
func (ar *AsyncUsageRecorder) RecordUsageAsync(
	providerID string,
	tokenID string,
	requestCount int,
	inputTokens int,
	outputTokens int,
) error {
	ar.logger.WarnLog("[AsyncUsageRecorder] RecordUsageAsync is deprecated, use RecordModelUsageAsync instead")

	if !ar.config.Enabled {
		// Fallback to synchronous recording
		ctx := context.Background()
		totalTokens := inputTokens + outputTokens
		return ar.usageTracker.RecordUsage(ctx, providerID, tokenID, requestCount, totalTokens)
	}

	job := UsageRecordingJob{
		JobID:        uuid.New().String(),
		ProviderID:   providerID,
		TokenID:      tokenID,
		RequestCount: requestCount,
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
		Timestamp:    time.Now(),
		RetryCount:   0,
	}

	// Try to send to queue (non-blocking)
	select {
	case ar.jobQueue <- job:
		ar.queueSize.Add(1)
		ar.logger.DebugLog("[AsyncUsageRecorder] Queued usage job %s (queue size: %d)",
			job.JobID, ar.queueSize.Load())
		return nil
	default:
		// Queue is full, handle overflow
		ar.logger.WarnLog("[AsyncUsageRecorder] Queue full, dropping job %s for provider %s",
			job.JobID, providerID)
		ar.jobsFailed.Add(1)
		return fmt.Errorf("async queue full, usage not recorded")
	}
}

// RecordModelUsageAsync records model usage asynchronously.
// Returns immediately without blocking on database writes.
func (ar *AsyncUsageRecorder) RecordModelUsageAsync(
	providerID string,
	tokenID string,
	model string,
	inputTokens int,
	outputTokens int,
) error {
	if !ar.config.Enabled {
		// Fallback to synchronous recording
		ctx := context.Background()
		return ar.usageTracker.RecordModelUsage(ctx, providerID, tokenID, model, inputTokens, outputTokens)
	}

	job := UsageRecordingJob{
		JobID:        uuid.New().String(),
		ProviderID:   providerID,
		TokenID:      tokenID,
		Model:        model,
		RequestCount: 1,
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
		Timestamp:    time.Now(),
		RetryCount:   0,
	}

	// Try to send to queue (non-blocking)
	select {
	case ar.jobQueue <- job:
		ar.queueSize.Add(1)
		ar.logger.DebugLog("[AsyncUsageRecorder] Queued model usage job %s (queue size: %d)",
			job.JobID, ar.queueSize.Load())
		return nil
	default:
		// Queue is full, handle overflow
		ar.logger.WarnLog("[AsyncUsageRecorder] Queue full, dropping model usage job %s for %s/%s",
			job.JobID, providerID, model)
		ar.jobsFailed.Add(1)
		return fmt.Errorf("async queue full, model usage not recorded")
	}
}

// GetMetrics returns current metrics for the async recorder.
func (ar *AsyncUsageRecorder) GetMetrics() AsyncUsageRecorderMetrics {
	return AsyncUsageRecorderMetrics{
		JobsProcessed: ar.jobsProcessed.Load(),
		JobsFailed:    ar.jobsFailed.Load(),
		QueueSize:     ar.queueSize.Load(),
		QueueCapacity: int64(ar.config.QueueSize),
		Enabled:       ar.config.Enabled,
		WorkerCount:   int64(ar.config.WorkerCount),
	}
}

// AsyncUsageRecorderMetrics represents metrics for the async recorder.
type AsyncUsageRecorderMetrics struct {
	JobsProcessed int64 `json:"jobs_processed"`
	JobsFailed    int64 `json:"jobs_failed"`
	QueueSize     int64 `json:"queue_size"`
	QueueCapacity int64 `json:"queue_capacity"`
	Enabled       bool  `json:"enabled"`
	WorkerCount   int64 `json:"worker_count"`
}

// Close gracefully shuts down the async recorder.
// Waits for all queued jobs to complete.
func (ar *AsyncUsageRecorder) Close() error {
	if !ar.config.Enabled {
		return nil
	}

	ar.logger.InfoLog("[AsyncUsageRecorder] Shutting down, waiting for %d jobs to complete...",
		ar.queueSize.Load())

	// Cancel context to stop workers
	ar.cancel()

	// Close job queue
	close(ar.jobQueue)

	// Wait for all workers to finish with timeout
	done := make(chan struct{})
	go func() {
		ar.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		ar.logger.InfoLog("[AsyncUsageRecorder] All workers stopped gracefully")
	case <-time.After(5 * time.Second):
		ar.logger.WarnLog("[AsyncUsageRecorder] Shutdown timeout, some jobs may not have completed")
	}

	metrics := ar.GetMetrics()
	ar.logger.InfoLog("[AsyncUsageRecorder] Final metrics: processed=%d, failed=%d",
		metrics.JobsProcessed, metrics.JobsFailed)

	return nil
}

// IsEnabled returns whether async recording is enabled.
func (ar *AsyncUsageRecorder) IsEnabled() bool {
	return ar.config.Enabled
}
