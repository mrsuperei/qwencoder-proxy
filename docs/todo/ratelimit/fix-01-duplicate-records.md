# Fix Plan: Problem 1 - Duplicate Records in request_history

**Priority:** HIGH  
**Status:** Ready for Implementation  
**Created:** 2026-03-18

---

## Problem Description

### Current Behavior
Each request to the LLM proxy creates **TWO records** in the `request_history` table:

1. **Record 1** (from `RecordUsage`):
   - Has `token_count` (total tokens)
   - Has `request_count`
   - **Missing:** `model`, `input_tokens`, `output_tokens`
   - Timestamp: T1

2. **Record 2** (from `RecordModelUsage`):
   - Has `model`, `input_tokens`, `output_tokens`
   - Has `request_count`
   - **Missing:** `token_count`
   - Timestamp: T2 (typically T1 + few milliseconds)

### Root Cause
The [`SequentialHandler.handleNonStreamingRequest()`](qwencoder-proxy/internal/proxy/sequential_handler.go:145) method calls **BOTH** recording methods:

1. Line 257: `quotaManager.RecordUsage(ctx, providerID, selectedToken.ID, inputTokens, outputTokens)`
2. Line 267: `quotaManager.RecordModelUsage(ctx, providerID, selectedToken.ID, model, inputTokens, outputTokens)`

Both methods insert records into `request_history`, causing duplication.

### Impact
- **Storage waste:** Doubles the storage requirements for request history
- **Query complexity:** Makes querying request history confusing and inefficient
- **Data inconsistency:** Different timestamps for the same logical request
- **Missing model info:** First record doesn't have model name
- **Inaccurate metrics:** Counting requests requires deduplication

---

## Solution Overview

**Approach:** Remove the `RecordUsage` call and rely solely on `RecordModelUsage` for all request history tracking.

**Rationale:**
- `RecordModelUsage` already captures all necessary data (model, input_tokens, output_tokens)
- `RecordUsage` is redundant and creates duplicate records
- `RecordModelUsage` also updates `model_usage` table, which is valuable for analytics
- This simplifies the code and eliminates duplication

**Changes Required:**
1. Remove `RecordUsage` call from SequentialHandler
2. Update `RecordModelUsage` to also update `provider_usage` and `token_usage` tables
3. Remove or deprecate `RecordUsage` method (or keep for backward compatibility but mark as deprecated)
4. Update async recorder to handle the change
5. Update tests to reflect new behavior

---

## Detailed Implementation Plan

### Step 1: Update RecordModelUsage to Update All Tables

**File:** [`qwencoder-proxy/internal/ratelimit/usage_tracker.go`](qwencoder-proxy/internal/ratelimit/usage_tracker.go)

**Current Behavior:** `RecordModelUsage` only updates `request_history` and `model_usage` tables.

**Required Changes:**
- Add logic to also update `provider_usage` and `token_usage` tables (similar to what `RecordUsage` does)

**Implementation:**

```go
// RecordModelUsage records usage for a specific model.
func (ut *usageTrackerImpl) RecordModelUsage(
    ctx context.Context,
    providerID string,
    tokenID string,
    model string,
    inputTokens int,
    outputTokens int,
) error {
    now := time.Now()
    nowMs := now.UnixMilli()
    totalTokens := inputTokens + outputTokens

    ut.logger.DebugLog("[UsageTracker] Recording model usage - Provider: %s, Token: %s, Model: %s, Input: %d, Output: %d",
        providerID, tokenID, model, inputTokens, outputTokens)

    // Use retry logic for the entire transaction
    return ut.executeWithRetry("RecordModelUsage", func() error {
        // Use transaction for atomicity.
        tx, err := ut.db.BeginTx(ctx, nil)
        if err != nil {
            ut.logger.ErrorLog("[UsageTracker] Failed to begin transaction: %v", err)
            return fmt.Errorf("failed to begin transaction: %w", err)
        }
        defer tx.Rollback()

        // Check if token exists before recording usage
        var tokenExists bool
        if tokenID != "" {
            var exists int
            err := tx.QueryRowContext(ctx, "SELECT 1 FROM tokens WHERE id = ?", tokenID).Scan(&exists)
            if err != nil {
                if errors.Is(err, sql.ErrNoRows) {
                    ut.logger.WarnLog("[UsageTracker] Token %s does not exist in tokens table, skipping model usage tracking", tokenID)
                    tokenExists = false
                } else {
                    ut.logger.ErrorLog("[UsageTracker] Failed to check token existence: %v", err)
                    return fmt.Errorf("failed to check token existence: %w", err)
                }
            } else {
                tokenExists = true
            }
        }

        // Record in request_history with model details (only if token exists).
        if tokenID != "" && tokenExists {
            historyID := uuid.New().String()
            historyQuery := `
                INSERT INTO request_history (id, token_id, model, request_count,
                                         input_tokens, output_tokens, timestamp)
                VALUES (?, ?, ?, 1, ?, ?, ?)
            `
            if _, err := tx.ExecContext(ctx, historyQuery,
                historyID, tokenID, model, inputTokens, outputTokens, nowMs); err != nil {
                ut.logger.ErrorLog("[UsageTracker] Failed to insert request history: %v", err)
                return fmt.Errorf("failed to insert request history: %w", err)
            }
            ut.logger.DebugLog("[UsageTracker] Inserted request_history record: %s", historyID)
        }

        // Update provider_usage with atomic increment (only if token exists).
        if tokenID != "" && tokenExists {
            providerQuery := `
                INSERT INTO provider_usage (id, provider_id, requests_today, requests_in_minute, tokens_in_minute, window_start, day_start)
                VALUES (?, ?, 0, 0, 0, ?, ?)
                ON CONFLICT(provider_id) DO UPDATE SET
                    requests_today = requests_today + ?,
                    requests_in_minute = requests_in_minute + ?,
                    tokens_in_minute = tokens_in_minute + ?,
                    updated_at = ?
                WHERE provider_id = ?
            `
            dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
            windowStart := now.Add(-time.Minute)

            if _, err := tx.ExecContext(ctx, providerQuery,
                uuid.New().String(), providerID, windowStart.UnixMilli(), dayStart.UnixMilli(),
                1, 1, totalTokens, nowMs, providerID); err != nil {
                ut.logger.ErrorLog("[UsageTracker] Failed to update provider usage: %v", err)
                return fmt.Errorf("failed to update provider usage: %w", err)
            }
            ut.logger.DebugLog("[UsageTracker] Updated provider_usage for: %s", providerID)
        }

        // Update token_usage with atomic increment (only if token exists).
        if tokenID != "" && tokenExists {
            tokenQuery := `
                INSERT INTO token_usage (id, token_id, requests_today, requests_in_minute, tokens_in_minute, window_start, day_start)
                VALUES (?, ?, 0, 0, 0, ?, ?)
                ON CONFLICT(token_id) DO UPDATE SET
                    requests_today = requests_today + ?,
                    requests_in_minute = requests_in_minute + ?,
                    tokens_in_minute = tokens_in_minute + ?,
                    updated_at = ?
                WHERE token_id = ?
            `
            dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
            windowStart := now.Add(-time.Minute)

            if _, err := tx.ExecContext(ctx, tokenQuery,
                uuid.New().String(), tokenID, windowStart.UnixMilli(), dayStart.UnixMilli(),
                1, 1, totalTokens, nowMs, tokenID); err != nil {
                ut.logger.ErrorLog("[UsageTracker] Failed to update token usage: %v", err)
                return fmt.Errorf("failed to update token usage: %w", err)
            }
            ut.logger.DebugLog("[UsageTracker] Updated token_usage for: %s", tokenID)
        }

        // Update model_usage with atomic increment (only if token exists).
        if tokenID != "" && tokenExists {
            dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
            windowStart := now.Add(-time.Minute)

            modelQuery := `
                INSERT INTO model_usage (id, token_id, provider_id, model,
                                       request_count, input_tokens, output_tokens, total_tokens,
                                       window_start, day_start)
                VALUES (?, ?, ?, ?, 0, 0, 0, 0, ?, ?)
                ON CONFLICT(token_id, model) DO UPDATE SET
                    request_count = request_count + 1,
                    input_tokens = input_tokens + ?,
                    output_tokens = output_tokens + ?,
                    total_tokens = total_tokens + ?,
                    updated_at = ?
                WHERE token_id = ? AND model = ?
            `
            if _, err := tx.ExecContext(ctx, modelQuery,
                uuid.New().String(), tokenID, providerID, model,
                windowStart.UnixMilli(), dayStart.UnixMilli(),
                inputTokens, outputTokens, totalTokens, nowMs,
                tokenID, model); err != nil {
                ut.logger.ErrorLog("[UsageTracker] Failed to update model usage: %v", err)
                return fmt.Errorf("failed to update model usage: %w", err)
            }
            ut.logger.DebugLog("[UsageTracker] Updated model_usage for: %s / %s", tokenID, model)
        }

        // Commit transaction.
        if err := tx.Commit(); err != nil {
            ut.logger.ErrorLog("[UsageTracker] Failed to commit transaction: %v", err)
            return fmt.Errorf("failed to commit transaction: %w", err)
        }

        ut.logger.InfoLog("[UsageTracker] Successfully recorded model usage - Provider: %s, Token: %s, Model: %s, Total: %d",
            providerID, tokenID, model, totalTokens)

        return nil
    })
}
```

**Key Changes:**
- Added `provider_usage` update section (lines after request_history insert)
- Added `token_usage` update section (after provider_usage)
- Kept existing `model_usage` update section
- All updates are in the same transaction for atomicity

---

### Step 2: Remove RecordUsage Call from SequentialHandler

**File:** [`qwencoder-proxy/internal/proxy/sequential_handler.go`](qwencoder-proxy/internal/proxy/sequential_handler.go)

**Current Code (lines 251-280):**
```go
// Step 5: Update database to increment usage counter
// responseWrapper.statusCode is now properly set by the wrapper
if responseWrapper.statusCode == http.StatusOK || responseWrapper.statusCode == 0 {
    inputTokens, outputTokens, _, err := h.quotaManager.ExtractTokensFromResponse(response.(map[string]interface{}))
    if err == nil {
        // Record general usage
        if err := h.quotaManager.RecordUsage(ctx, providerID, selectedToken.ID, inputTokens, outputTokens); err != nil {
            h.logger.ErrorLog("[SequentialHandler] Step 5: Failed to record usage: %v", err)
            h.logger.ErrorLog("[SequentialHandler] Step 5: Details - ProviderID: %s, TokenID: %s, Input: %d, Output: %d",
                providerID, selectedToken.ID, inputTokens, outputTokens)
        } else {
            h.logger.InfoLog("[SequentialHandler] Step 5: Successfully recorded usage: %d tokens (input: %d, output: %d)",
                inputTokens+outputTokens, inputTokens, outputTokens)
        }

        // Record model-specific usage
        if err := h.quotaManager.RecordModelUsage(ctx, providerID, selectedToken.ID, model, inputTokens, outputTokens); err != nil {
            h.logger.ErrorLog("[SequentialHandler] Step 5: Failed to record model usage: %v", err)
            h.logger.ErrorLog("[SequentialHandler] Step 5: Model Details - ProviderID: %s, TokenID: %s, Model: %s, Input: %d, Output: %d",
                providerID, selectedToken.ID, model, inputTokens, outputTokens)
        } else {
            h.logger.InfoLog("[SequentialHandler] Step 5: Successfully recorded model usage: %s - %d tokens (input: %d, output: %d)",
                model, inputTokens+outputTokens, inputTokens, outputTokens)
        }
    } else {
        h.logger.WarnLog("[SequentialHandler] Step 5: Failed to extract tokens from response: %v", err)
    }
} else {
    h.logger.WarnLog("[SequentialHandler] Step 5: Skipping usage recording - status code: %d", responseWrapper.statusCode)
}
```

**Required Changes:**
Remove the `RecordUsage` call and keep only `RecordModelUsage`:

```go
// Step 5: Update database to increment usage counter
// responseWrapper.statusCode is now properly set by the wrapper
if responseWrapper.statusCode == http.StatusOK || responseWrapper.statusCode == 0 {
    inputTokens, outputTokens, _, err := h.quotaManager.ExtractTokensFromResponse(response.(map[string]interface{}))
    if err == nil {
        // Record model-specific usage (this now also updates provider_usage and token_usage)
        if err := h.quotaManager.RecordModelUsage(ctx, providerID, selectedToken.ID, model, inputTokens, outputTokens); err != nil {
            h.logger.ErrorLog("[SequentialHandler] Step 5: Failed to record model usage: %v", err)
            h.logger.ErrorLog("[SequentialHandler] Step 5: Model Details - ProviderID: %s, TokenID: %s, Model: %s, Input: %d, Output: %d",
                providerID, selectedToken.ID, model, inputTokens, outputTokens)
        } else {
            h.logger.InfoLog("[SequentialHandler] Step 5: Successfully recorded model usage: %s - %d tokens (input: %d, output: %d)",
                model, inputTokens+outputTokens, inputTokens, outputTokens)
        }
    } else {
        h.logger.WarnLog("[SequentialHandler] Step 5: Failed to extract tokens from response: %v", err)
    }
} else {
    h.logger.WarnLog("[SequentialHandler] Step 5: Skipping usage recording - status code: %d", responseWrapper.statusCode)
}
```

**Key Changes:**
- Removed lines 256-264 (RecordUsage call)
- Kept lines 267-274 (RecordModelUsage call)
- Updated comment to clarify that RecordModelUsage now updates all tables

---

### Step 3: Update Streaming Usage Recorder

**File:** [`qwencoder-proxy/internal/proxy/sequential_handler.go`](qwencoder-proxy/internal/proxy/sequential_handler.go)

**Current Code (lines 386-394):**
```go
// Record usage atomically
if err := w.quotaManager.RecordUsage(w.ctx, w.providerID, w.tokenID, w.inputTokens, w.outputTokens); err != nil {
    w.logger.ErrorLog("[streamingUsageRecorder] Failed to record usage: %v", err)
}

// Record model usage atomically
if err := w.quotaManager.RecordModelUsage(w.ctx, w.providerID, w.tokenID, w.model, w.inputTokens, w.outputTokens); err != nil {
    w.logger.ErrorLog("[streamingUsageRecorder] Failed to record model usage: %v", err)
}
```

**Required Changes:**
Remove the `RecordUsage` call:

```go
// Record model usage atomically (this now also updates provider_usage and token_usage)
if err := w.quotaManager.RecordModelUsage(w.ctx, w.providerID, w.tokenID, w.model, w.inputTokens, w.outputTokens); err != nil {
    w.logger.ErrorLog("[streamingUsageRecorder] Failed to record model usage: %v", err)
}
```

**Key Changes:**
- Removed lines 387-389 (RecordUsage call)
- Kept lines 392-394 (RecordModelUsage call)
- Updated comment

---

### Step 4: Update QuotaManager.RecordUsage Method

**File:** [`qwencoder-proxy/internal/ratelimit/quota_manager.go`](qwencoder-proxy/internal/ratelimit/quota_manager.go)

**Option A: Deprecate RecordUsage**
Add deprecation notice but keep for backward compatibility:

```go
// RecordUsage records usage after a successful request.
// DEPRECATED: Use RecordModelUsage instead for complete tracking.
// This method is kept for backward compatibility but may be removed in future versions.
func (qm *QuotaManager) RecordUsage(ctx context.Context, providerID string, tokenID string, inputTokens int, outputTokens int) error {
    qm.logger.WarnLog("[QuotaManager] RecordUsage is deprecated, use RecordModelUsage instead")
    
    // Use async recorder if enabled
    if qm.asyncRecorder != nil && qm.asyncRecorder.IsEnabled() {
        if err := qm.asyncRecorder.RecordUsageAsync(providerID, tokenID, 1, inputTokens, outputTokens); err != nil {
            return fmt.Errorf("failed to queue async usage recording: %w", err)
        }
        qm.logger.DebugLog("[QuotaManager] Queued async usage recording for %s: %d tokens (input: %d, output: %d)",
            providerID, inputTokens+outputTokens, inputTokens, outputTokens)
        return nil
    }

    // Fallback to synchronous recording
    totalTokens := inputTokens + outputTokens
    if err := qm.usageTracker.RecordUsage(ctx, providerID, tokenID, 1, totalTokens); err != nil {
        return fmt.Errorf("failed to record usage: %w", err)
    }

    qm.logger.DebugLog("[QuotaManager] Recorded usage for %s: %d tokens (input: %d, output: %d)",
        providerID, totalTokens, inputTokens, outputTokens)

    return nil
}
```

**Option B: Remove RecordUsage**
If you're confident no other code uses it, you can remove it entirely.

**Recommendation:** Use Option A (deprecate) for safer migration.

---

### Step 5: Update Async Usage Recorder

**File:** [`qwencoder-proxy/internal/ratelimit/async_usage_recorder.go`](qwencoder-proxy/internal/ratelimit/async_usage_recorder.go)

**Current Code (lines 256-297):**
```go
// RecordUsageAsync records usage asynchronously.
// Returns immediately without blocking on database writes.
func (ar *AsyncUsageRecorder) RecordUsageAsync(
    providerID string,
    tokenID string,
    requestCount int,
    inputTokens int,
    outputTokens int,
) error {
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
        RequestCount:  requestCount,
        InputTokens:   inputTokens,
        OutputTokens:  outputTokens,
        Timestamp:     time.Now(),
        RetryCount:    0,
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
```

**Required Changes:**
Add deprecation notice:

```go
// RecordUsageAsync records usage asynchronously.
// DEPRECATED: Use RecordModelUsageAsync instead for complete tracking.
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
        RequestCount:  requestCount,
        InputTokens:   inputTokens,
        OutputTokens:  outputTokens,
        Timestamp:     time.Now(),
        RetryCount:    0,
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
```

---

### Step 6: Update Tests

**Files to Update:**
- [`qwencoder-proxy/internal/ratelimit/usage_tracker_test.go`](qwencoder-proxy/internal/ratelimit/usage_tracker_test.go)
- [`qwencoder-proxy/internal/ratelimit/integrated_system_test.go`](qwencoder-proxy/internal/ratelimit/integrated_system_test.go)
- [`qwencoder-proxy/internal/proxy/sequential_handler_test.go`](qwencoder-proxy/internal/proxy/sequential_handler_test.go)

**Required Changes:**
1. Update tests that expect 2 records in request_history to expect 1 record
2. Update tests that call RecordUsage to call RecordModelUsage instead
3. Add tests to verify that RecordModelUsage updates all 4 tables (request_history, provider_usage, token_usage, model_usage)
4. Update tests that verify token_count field to verify input_tokens and output_tokens instead

---

### Step 7: Database Migration (Optional)

**File:** [`qwencoder-proxy/internal/ratelimit/migration.go`](qwencoder-proxy/internal/ratelimit/migration.go)

**Optional:** Add a migration to clean up existing duplicate records:

```go
// CleanupDuplicateRequestHistory removes duplicate records from request_history
// This migration cleans up the duplicate records created by the old dual-recording system
func CleanupDuplicateRequestHistory(db *sql.DB) error {
    // This is a complex operation that requires careful consideration
    // For now, we recommend leaving old data as-is and only fixing new requests
    // Future requests will not create duplicates
    
    log.Println("[Migration] Skipping duplicate cleanup - old data preserved")
    return nil
}
```

**Recommendation:** Don't clean up old data to avoid data loss. Just fix new requests.

---

## Verification Steps

After implementing the changes, verify:

1. **Single Record Creation:**
   - Make a test request
   - Query `request_history` table
   - Verify only 1 record exists (not 2)

2. **Complete Data:**
   - Verify the single record has: `model`, `input_tokens`, `output_tokens`, `request_count`, `timestamp`

3. **All Tables Updated:**
   - Verify `provider_usage` is updated
   - Verify `token_usage` is updated
   - Verify `model_usage` is updated

4. **No Data Loss:**
   - Compare total request counts before and after
   - Verify no requests are lost

5. **Performance:**
   - Verify no performance degradation
   - Check that database operations are still efficient

---

## Rollback Plan

If issues arise after deployment:

1. **Revert code changes** to restore RecordUsage call
2. **No data migration needed** - old data structure is compatible
3. **Monitor** for any data inconsistencies

---

## Summary

**Files to Modify:**
1. [`qwencoder-proxy/internal/ratelimit/usage_tracker.go`](qwencoder-proxy/internal/ratelimit/usage_tracker.go) - Update RecordModelUsage
2. [`qwencoder-proxy/internal/proxy/sequential_handler.go`](qwencoder-proxy/internal/proxy/sequential_handler.go) - Remove RecordUsage calls (2 locations)
3. [`qwencoder-proxy/internal/ratelimit/quota_manager.go`](qwencoder-proxy/internal/ratelimit/quota_manager.go) - Deprecate RecordUsage
4. [`qwencoder-proxy/internal/ratelimit/async_usage_recorder.go`](qwencoder-proxy/internal/ratelimit/async_usage_recorder.go) - Deprecate RecordUsageAsync
5. Test files - Update tests

**Expected Outcome:**
- Each request creates exactly 1 record in `request_history`
- All 4 tables are updated (request_history, provider_usage, token_usage, model_usage)
- No duplicate records
- Complete data capture (model, input_tokens, output_tokens)
- Simplified codebase

**Estimated Effort:** 2-3 hours

---

## Notes for Code Agent

- **Focus on Step 1 and Step 2 first** - these are the critical changes
- **Test thoroughly** after each step
- **Consider backward compatibility** - deprecate rather than remove RecordUsage
- **Add logging** to help debug if issues arise
- **Document the changes** in code comments
