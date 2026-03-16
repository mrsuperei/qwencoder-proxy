# Phase 3: Async Tracking - Non-Blocking Usage Tracking

**Phase Goal:** Implement non-blocking usage tracking with worker pools for database writes.

**Duration:** Week 3-4  
**Status:** Ready to Implement  
**Dependencies:** Phase 1 (Foundation), Phase 2 (Core Rate Limiting)

---

## Task Overview

This phase implements the non-blocking usage tracking system with:

1. Implementing `UsageTracker` with buffered channels
2. Implementing `WorkerPool` for async writes
3. Implementing batch write optimization
4. Adding error handling and retries
5. Implementing graceful shutdown
6. Writing unit tests for worker pool
7. Writing load tests

---

## Task 3.1: Implement `UsageTracker` with Channels

**File:** `qwencoder-proxy/ratelimit/tracker.go`

Create the usage tracker with non-blocking channel-based design.

**Implementation Requirements:**

```go
package ratelimit

import (
    "context"
    "fmt"
    "sync"
    "time"
    
    "github.com/sunbankio/qwencoder-proxy/ratelimit/store"
)

// UsageTracker handles non-blocking usage tracking
type UsageTracker struct {
    usageChan   chan *UsageRecord
    workerPool  *WorkerPool
    bufferConfig *BufferConfig
    cache       *RateLimitCache
    logger      Logger
    stopChan    chan struct{}
    started     bool
    mu          sync.Mutex
}

// NewUsageTracker creates a new usage tracker
func NewUsageTracker(
    store store.Store,
    bufferConfig *BufferConfig,
    cache *RateLimitCache,
    logger Logger,
) *UsageTracker {
    return &UsageTracker{
        usageChan:    make(chan *UsageRecord, bufferConfig.ChannelSize),
        bufferConfig: bufferConfig,
        cache:        cache,
        logger:       logger,
        stopChan:     make(chan struct{}),
    }
}

// Start starts the usage tracker
func (t *UsageTracker) Start() error {
    t.mu.Lock()
    defer t.mu.Unlock()
    
    if t.started {
        return nil
    }
    
    // Create worker pool
    t.workerPool = NewWorkerPool(
        t.store,
        &WorkerPoolConfig{
            NumWorkers:      4,
            QueueSize:       500,
            MaxRetries:      3,
            RetryDelay:      100 * time.Millisecond,
            ShutdownTimeout: 5 * time.Second,
        },
        t.logger,
    )
    
    if err := t.workerPool.Start(); err != nil {
        return fmt.Errorf("failed to start worker pool: %w", err)
    }
    
    // Start batch processor
    go t.batchProcessor()
    
    t.started = true
    t.logger.InfoLog("[UsageTracker] Started successfully")
    return nil
}

// Stop stops the usage tracker gracefully
func (t *UsageTracker) Stop() {
    t.mu.Lock()
    defer t.mu.Unlock()
    
    if !t.started {
        return
    }
    
    close(t.stopChan)
    
    // Drain remaining records
    t.drainChannel()
    
    // Stop worker pool
    if t.workerPool != nil {
        t.workerPool.Stop()
    }
    
    t.started = false
    t.logger.InfoLog("[UsageTracker] Stopped successfully")
}

// RecordUsage records a usage metric (non-blocking)
func (t *UsageTracker) RecordUsage(ctx context.Context, record *UsageRecord) error {
    select {
    case t.usageChan <- record:
        // Update cache immediately (write-through)
        if t.cache != nil {
            t.cache.UpdateUsage(record)
        }
        return nil
    default:
        // Channel is full
        if t.bufferConfig.DropWhenFull {
            t.logger.WarnLog("[UsageTracker] Channel full, dropping usage record")
            return fmt.Errorf("usage tracking channel full, record dropped")
        }
        
        // Block until there's space
        select {
        case t.usageChan <- record:
            if t.cache != nil {
                t.cache.UpdateUsage(record)
            }
            return nil
        case <-ctx.Done():
            return ctx.Err()
        }
    }
}

// batchProcessor batches usage records and sends to worker pool
func (t *UsageTracker) batchProcessor() {
    ticker := time.NewTicker(t.bufferConfig.FlushInterval)
    defer ticker.Stop()
    
    batch := make([]*UsageRecord, 0, t.bufferConfig.MaxBatchSize)
    
    for {
        select {
        case <-t.stopChan:
            // Flush remaining batch
            if len(batch) > 0 {
                t.flushBatch(batch)
            }
            return
            
        case record := <-t.usageChan:
            batch = append(batch, record)
            
            // Flush if batch is full
            if len(batch) >= t.bufferConfig.MaxBatchSize {
                t.flushBatch(batch)
                batch = batch[:0]
            }
            
        case <-ticker.C:
            // Flush on timer
            if len(batch) > 0 {
                t.flushBatch(batch)
                batch = batch[:0]
            }
        }
    }
}

// flushBatch flushes a batch of usage records
func (t *UsageTracker) flushBatch(batch []*UsageRecord) {
    t.logger.DebugLog("[UsageTracker] Flushing batch of %d records", len(batch))
    
    for _, record := range batch {
        job := &WriteJob{
            Record: record,
            Callback: func(err error) {
                if err != nil {
                    t.logger.ErrorLog("[UsageTracker] Failed to write usage record: %v", err)
                }
            },
        }
        
        if err := t.workerPool.Submit(job); err != nil {
            t.logger.ErrorLog("[UsageTracker] Failed to submit job: %v", err)
        }
    }
}

// drainChannel drains remaining records from the channel
func (t *UsageTracker) drainChannel() {
    drained := 0
    for {
        select {
        case record := <-t.usageChan:
            drained++
            // Write directly to store
            if err := t.workerPool.Submit(&WriteJob{Record: record}); err != nil {
                t.logger.ErrorLog("[UsageTracker] Failed to write drained record: %v", err)
            }
        default:
            t.logger.InfoLog("[UsageTracker] Drained %d records", drained)
            return
        }
    }
}
```

---

## Task 3.2: Implement `WorkerPool` for Async Writes

**File:** `qwencoder-proxy/ratelimit/worker_pool.go`

Create the worker pool for async database writes.

**Implementation Requirements:**

```go
package ratelimit

import (
    "context"
    "fmt"
    "sync"
    "time"
    
    "github.com/sunbankio/qwencoder-proxy/ratelimit/store"
)

// WriteJob represents a write job for the worker pool
type WriteJob struct {
    Record    *UsageRecord
    RetryCount int
    Callback  func(error)
}

// WorkerPool manages a pool of worker goroutines for async writes
type WorkerPool struct {
    config   *WorkerPoolConfig
    store    store.Store
    jobChan  chan *WriteJob
    workers  []*worker
    stopChan chan struct{}
    wg       sync.WaitGroup
    metrics  *WorkerMetrics
    logger   Logger
    started  bool
    mu       sync.Mutex
}

// WorkerMetrics tracks worker pool metrics
type WorkerMetrics struct {
    JobsSubmitted int64
    JobsCompleted int64
    JobsFailed    int64
    mu           sync.RWMutex
}

// NewWorkerPool creates a new worker pool
func NewWorkerPool(
    store store.Store,
    config *WorkerPoolConfig,
    logger Logger,
) *WorkerPool {
    return &WorkerPool{
        config:   config,
        store:    store,
        jobChan:  make(chan *WriteJob, config.QueueSize),
        stopChan: make(chan struct{}),
        metrics:  &WorkerMetrics{},
        logger:   logger,
    }
}

// Start starts the worker pool
func (p *WorkerPool) Start() error {
    p.mu.Lock()
    defer p.mu.Unlock()
    
    if p.started {
        return nil
    }
    
    // Create workers
    p.workers = make([]*worker, p.config.NumWorkers)
    for i := 0; i < p.config.NumWorkers; i++ {
        p.workers[i] = newWorker(
            i,
            p.jobChan,
            p.stopChan,
            p.store,
            p.config,
            p.metrics,
            p.logger,
        )
        p.workers[i].start()
    }
    
    p.started = true
    p.logger.InfoLog("[WorkerPool] Started with %d workers", p.config.NumWorkers)
    return nil
}

// Stop stops the worker pool gracefully
func (p *WorkerPool) Stop() {
    p.mu.Lock()
    defer p.mu.Unlock()
    
    if !p.started {
        return
    }
    
    // Close job channel
    close(p.jobChan)
    
    // Wait for workers to finish with timeout
    done := make(chan struct{})
    go func() {
        p.wg.Wait()
        close(done)
    }()
    
    select {
    case <-done:
        p.logger.InfoLog("[WorkerPool] Stopped gracefully")
    case <-time.After(p.config.ShutdownTimeout):
        p.logger.WarnLog("[WorkerPool] Shutdown timeout, forcing stop")
    }
    
    p.started = false
}

// Submit submits a job to the worker pool
func (p *WorkerPool) Submit(job *WriteJob) error {
    p.metrics.mu.Lock()
    p.metrics.JobsSubmitted++
    p.metrics.mu.Unlock()
    
    select {
    case p.jobChan <- job:
        return nil
    default:
        return fmt.Errorf("worker pool queue full")
    }
}

// GetMetrics returns the worker pool metrics
func (p *WorkerPool) GetMetrics() WorkerMetrics {
    p.metrics.mu.RLock()
    defer p.metrics.mu.RUnlock()
    
    return WorkerMetrics{
        JobsSubmitted: p.metrics.JobsSubmitted,
        JobsCompleted: p.metrics.JobsCompleted,
        JobsFailed:    p.metrics.JobsFailed,
    }
}

// worker represents a single worker goroutine
type worker struct {
    id       int
    jobChan  <-chan *WriteJob
    stopChan <-chan struct{}
    store    store.Store
    config   *WorkerPoolConfig
    metrics  *WorkerMetrics
    logger   Logger
}

// newWorker creates a new worker
func newWorker(
    id int,
    jobChan <-chan *WriteJob,
    stopChan <-chan struct{},
    store store.Store,
    config *WorkerPoolConfig,
    metrics *WorkerMetrics,
    logger Logger,
) *worker {
    return &worker{
        id:       id,
        jobChan:  jobChan,
        stopChan: stopChan,
        store:    store,
        config:   config,
        metrics:  metrics,
        logger:   logger,
    }
}

// start starts the worker
func (w *worker) start() {
    w.metrics.wg.Add(1)
    go w.run()
}

// run is the main worker loop
func (w *worker) run() {
    defer w.metrics.wg.Done()
    
    for {
        select {
        case <-w.stopChan:
            w.logger.DebugLog("[Worker %d] Stopping", w.id)
            return
            
        case job, ok := <-w.jobChan:
            if !ok {
                w.logger.DebugLog("[Worker %d] Job channel closed", w.id)
                return
            }
            w.processJob(job)
        }
    }
}

// processJob processes a single job
func (w *worker) processJob(job *WriteJob) {
    var err error
    
    // Retry logic
    for attempt := 0; attempt <= w.config.MaxRetries; attempt++ {
        err = w.store.WriteUsageRecord(context.Background(), job.Record)
        
        if err == nil {
            // Success
            w.metrics.mu.Lock()
            w.metrics.JobsCompleted++
            w.metrics.mu.Unlock()
            
            if job.Callback != nil {
                job.Callback(nil)
            }
            return
        }
        
        // Retry with exponential backoff
        if attempt < w.config.MaxRetries {
            backoff := w.config.RetryDelay * time.Duration(1<<uint(attempt))
            w.logger.DebugLog("[Worker %d] Retry %d after %v", w.id, attempt+1, backoff)
            time.Sleep(backoff)
        }
    }
    
    // All retries failed
    w.metrics.mu.Lock()
    w.metrics.JobsFailed++
    w.metrics.mu.Unlock()
    
    w.logger.ErrorLog("[Worker %d] Failed to write usage record after %d retries: %v",
        w.id, w.config.MaxRetries, err)
    
    if job.Callback != nil {
        job.Callback(err)
    }
}
```

---

## Task 3.3: Implement Batch Write Optimization

The batch write optimization is already implemented in `UsageTracker.batchProcessor()`. Ensure:

1. Batch size is configurable
2. Flush interval is configurable
3. Batch is flushed when full or on timer

**Additional Requirements:**

1. Add metrics for batch size distribution
2. Add metrics for flush frequency

---

## Task 3.4: Add Error Handling and Retries

Error handling and retries are already implemented in `worker.processJob()`. Ensure:

1. Exponential backoff is used
2. Max retries is configurable
3. Errors are logged appropriately
4. Callbacks are called with error result

---

## Task 3.5: Implement Graceful Shutdown

Graceful shutdown is already implemented in `UsageTracker.Stop()` and `WorkerPool.Stop()`. Ensure:

1. Remaining records are flushed
2. Workers are given time to finish
3. Timeout is enforced
4. Resources are cleaned up

---

## Task 3.6: Write Unit Tests for Worker Pool

**File:** `qwencoder-proxy/ratelimit/worker_pool_test.go`

Write comprehensive unit tests for the worker pool.

**Test Cases:**

1. **Worker Pool Lifecycle:**
   - Test starting the worker pool
   - Test stopping the worker pool
   - Test graceful shutdown

2. **Job Processing:**
   - Test single job processing
   - Test multiple job processing
   - Test concurrent job submission

3. **Error Handling:**
   - Test retry logic
   - Test exponential backoff
   - Test callback invocation

4. **Metrics:**
   - Test job submission metrics
   - Test job completion metrics
   - Test job failure metrics

**Test Template:**

```go
package ratelimit

import (
    "context"
    "testing"
    "time"
    
    "github.com/sunbankio/qwencoder-proxy/ratelimit/store"
    "github.com/sunbankio/qwencoder-proxy/logging"
)

func setupTestWorkerPool(t *testing.T) *WorkerPool {
    store := setupTestStore(t)
    config := &WorkerPoolConfig{
        NumWorkers:      2,
        QueueSize:       10,
        MaxRetries:      3,
        RetryDelay:      10 * time.Millisecond,
        ShutdownTimeout: 1 * time.Second,
    }
    logger := logging.NewLogger()
    
    pool := NewWorkerPool(store, config, logger)
    if err := pool.Start(); err != nil {
        t.Fatalf("Failed to start worker pool: %v", err)
    }
    
    t.Cleanup(func() {
        pool.Stop()
    })
    
    return pool
}

func TestWorkerPool_StartStop(t *testing.T) {
    pool := setupTestWorkerPool(t)
    
    // Pool should be started
    if !pool.started {
        t.Error("Worker pool should be started")
    }
    
    // Stop should work
    pool.Stop()
    
    // Pool should be stopped
    if pool.started {
        t.Error("Worker pool should be stopped")
    }
}

func TestWorkerPool_ProcessJob(t *testing.T) {
    pool := setupTestWorkerPool(t)
    
    record := &UsageRecord{
        ProviderID:   "test",
        TokenID:      "token-1",
        Model:        "test-model",
        RequestCount:  1,
        TokenCount:    100,
        Timestamp:     time.Now(),
        Success:      true,
    }
    
    callbackCalled := false
    callbackError := error(nil)
    
    job := &WriteJob{
        Record: record,
        Callback: func(err error) {
            callbackCalled = true
            callbackError = err
        },
    }
    
    err := pool.Submit(job)
    if err != nil {
        t.Fatalf("Failed to submit job: %v", err)
    }
    
    // Wait for job to complete
    time.Sleep(100 * time.Millisecond)
    
    if !callbackCalled {
        t.Error("Callback was not called")
    }
    
    if callbackError != nil {
        t.Errorf("Callback error: %v", callbackError)
    }
}

// Implement more tests...
```

---

## Task 3.7: Write Load Tests

**File:** `qwencoder-proxy/ratelimit/worker_pool_bench_test.go`

Write load tests for the worker pool.

**Test Cases:**

1. **Throughput Test:**
   - Test processing 10,000 jobs
   - Measure throughput (jobs/second)

2. **Concurrency Test:**
   - Test concurrent job submission
   - Test with multiple goroutines

3. **Memory Test:**
   - Test memory usage under load
   - Test for memory leaks

**Benchmark Template:**

```go
package ratelimit

import (
    "testing"
    "time"
)

func BenchmarkWorkerPool_Throughput(b *testing.B) {
    pool := setupBenchmarkWorkerPool(b)
    defer pool.Stop()
    
    record := &UsageRecord{
        ProviderID:   "test",
        TokenID:      "token-1",
        Model:        "test-model",
        RequestCount:  1,
        TokenCount:    100,
        Timestamp:     time.Now(),
        Success:      true,
    }
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        job := &WriteJob{Record: record}
        pool.Submit(job)
    }
    
    // Wait for all jobs to complete
    time.Sleep(1 * time.Second)
}

func BenchmarkWorkerPool_Concurrent(b *testing.B) {
    pool := setupBenchmarkWorkerPool(b)
    defer pool.Stop()
    
    record := &UsageRecord{
        ProviderID:   "test",
        TokenID:      "token-1",
        Model:        "test-model",
        RequestCount:  1,
        TokenCount:    100,
        Timestamp:     time.Now(),
        Success:      true,
    }
    
    b.RunParallel(func(pb *testing.PB) {
        for pb.Next() {
            job := &WriteJob{Record: record}
            pool.Submit(job)
        }
    })
    
    // Wait for all jobs to complete
    time.Sleep(1 * time.Second)
}
```

---

## Deliverables

After completing this phase, you should have:

1. ✅ `UsageTracker` with non-blocking channel-based design
2. ✅ `WorkerPool` with configurable workers
3. ✅ Batch write optimization
4. ✅ Error handling with exponential backoff
5. ✅ Graceful shutdown implementation
6. ✅ Unit tests for worker pool
7. ✅ Load tests and benchmarks

---

## Success Criteria

- [ ] Usage recording completes in <100μs (non-blocking)
- [ ] Worker pool throughput >10,000 jobs/second
- [ ] All unit tests pass
- [ ] All load tests pass
- [ ] No memory leaks under load
- [ ] Graceful shutdown completes within timeout
- [ ] Error handling is comprehensive

---

## Performance Targets

| Metric | Target | Notes |
|--------|--------|-------|
| Record Usage (non-blocking) | <100μs | Channel send time |
| Worker Throughput | >10k req/s | Jobs processed per second |
| Batch Flush | <10ms | Batch write duration |
| Shutdown Timeout | <5s | Graceful shutdown |
| Memory Usage | <20MB | Worker pool + buffers |

---

## Next Phase

After completing Phase 3, proceed to **Phase 4: Smart Token Selection** which implements quota-aware token selection.

**File:** `../todo/phase-4-smart-token-selection.md`
