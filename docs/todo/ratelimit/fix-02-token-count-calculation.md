# Fix Plan: Problem 2 - Incorrect Token Count Calculation

**Priority:** HIGH  
**Status:** Ready for Implementation  
**Created:** 2026-03-18

---

## Problem Description

### Current Behavior
The `request_history` table has inconsistent token counting:

1. **Old Schema (from RecordUsage):**
   - Uses `token_count` field (total tokens)
   - No breakdown of input vs output
   - No model information

2. **New Schema (from RecordModelUsage):**
   - Uses `input_tokens` and `output_tokens` fields
   - Includes model information
   - More detailed and accurate

3. **The Problem:**
   - The table schema definition doesn't include `input_tokens`, `output_tokens`, or `model` columns
   - These columns are added dynamically by the INSERT statements
   - This creates schema inconsistency and potential issues
   - Some records have `token_count`, others have `input_tokens`/`output_tokens`

### Root Cause
The initial table schema at [`usage_tracker.go:149-158`](qwencoder-proxy/internal/ratelimit/usage_tracker.go:149) was defined as:

```sql
CREATE TABLE IF NOT EXISTS request_history (
    id TEXT PRIMARY KEY,
    token_id TEXT NOT NULL,
    request_count INTEGER NOT NULL DEFAULT 1,
    token_count INTEGER NOT NULL DEFAULT 0,
    timestamp INTEGER NOT NULL,
    created_at INTEGER NOT NULL DEFAULT (strftime('%s', 'subsec') * 1000),
    FOREIGN KEY (token_id) REFERENCES tokens(id) ON DELETE CASCADE
)
```

But the `RecordModelUsage` method at line 860 inserts:
```sql
INSERT INTO request_history (id, token_id, model, request_count,
                           input_tokens, output_tokens, timestamp)
VALUES (?, ?, ?, 1, ?, ?, ?)
```

This creates columns that don't exist in the schema definition.

### Impact
- **Schema inconsistency:** Table structure doesn't match actual usage
- **Query complexity:** Different records have different columns
- **Data integrity:** No enforced constraints on input/output tokens
- **Migration issues:** Hard to migrate or backup inconsistent schema
- **Analytics difficulty:** Hard to query token usage consistently

---

## Solution Overview

**Approach:** Update the `request_history` table schema to properly include `model`, `input_tokens`, and `output_tokens` columns, and ensure all records use this consistent structure.

**Rationale:**
- Provides complete token tracking (input + output breakdown)
- Enables better analytics and reporting
- Maintains data consistency
- Aligns with provider response format
- Supports per-model token usage tracking

**Changes Required:**
1. Update database schema to add missing columns
2. Create migration script for existing databases
3. Update RecordUsage to use the new schema (or deprecate it)
4. Ensure all inserts use the consistent schema
5. Update queries to use the new columns
6. Update tests

---

## Detailed Implementation Plan

### Step 1: Update Database Schema Definition

**File:** [`qwencoder-proxy/internal/ratelimit/usage_tracker.go`](qwencoder-proxy/internal/ratelimit/usage_tracker.go)

**Current Schema (lines 148-158):**
```go
// Create request_history table for proper sliding window tracking.
requestHistoryTable := `
    CREATE TABLE IF NOT EXISTS request_history (
        id TEXT PRIMARY KEY,
        token_id TEXT NOT NULL,
        request_count INTEGER NOT NULL DEFAULT 1,
        token_count INTEGER NOT NULL DEFAULT 0,
        timestamp INTEGER NOT NULL,
        created_at INTEGER NOT NULL DEFAULT (strftime('%s', 'subsec') * 1000),
        FOREIGN KEY (token_id) REFERENCES tokens(id) ON DELETE CASCADE
    )
`
```

**Updated Schema:**
```go
// Create request_history table for proper sliding window tracking.
requestHistoryTable := `
    CREATE TABLE IF NOT EXISTS request_history (
        id TEXT PRIMARY KEY,
        token_id TEXT NOT NULL,
        model TEXT,
        request_count INTEGER NOT NULL DEFAULT 1,
        input_tokens INTEGER NOT NULL DEFAULT 0,
        output_tokens INTEGER NOT NULL DEFAULT 0,
        timestamp INTEGER NOT NULL,
        created_at INTEGER NOT NULL DEFAULT (strftime('%s', 'subsec') * 1000),
        FOREIGN KEY (token_id) REFERENCES tokens(id) ON DELETE CASCADE
    )
`
```

**Key Changes:**
- Added `model TEXT` column (nullable, since some requests might not have model info)
- Changed `token_count` to `input_tokens` and `output_tokens`
- Made both `input_tokens` and `output_tokens` NOT NULL with DEFAULT 0
- Removed `token_count` column (or keep as deprecated, see Step 3)

---

### Step 2: Create Migration Script

**File:** [`qwencoder-proxy/internal/ratelimit/migration.go`](qwencoder-proxy/internal/ratelimit/migration.go)

**Add New Migration Function:**

```go
// MigrateRequestHistorySchema updates the request_history table schema
// to include model, input_tokens, and output_tokens columns
func MigrateRequestHistorySchema(db *sql.DB) error {
    log.Println("[Migration] Starting request_history schema migration...")

    // Step 1: Check if migration is needed
    var hasModelColumn int
    err := db.QueryRow(`
        SELECT COUNT(*)
        FROM pragma_table_info('request_history')
        WHERE name = 'model'
    `).Scan(&hasModelColumn)
    if err != nil {
        return fmt.Errorf("failed to check model column: %w", err)
    }

    if hasModelColumn > 0 {
        log.Println("[Migration] request_history schema already migrated")
        return nil
    }

    // Step 2: Begin transaction
    tx, err := db.Begin()
    if err != nil {
        return fmt.Errorf("failed to begin migration transaction: %w", err)
    }
    defer tx.Rollback()

    // Step 3: Add new columns
    columnsToAdd := []string{
        "model TEXT",
        "input_tokens INTEGER NOT NULL DEFAULT 0",
        "output_tokens INTEGER NOT NULL DEFAULT 0",
    }

    for _, col := range columnsToAdd {
        _, err := tx.Exec(fmt.Sprintf("ALTER TABLE request_history ADD COLUMN %s", col))
        if err != nil {
            return fmt.Errorf("failed to add column %s: %w", col, err)
        }
        log.Printf("[Migration] Added column: %s", col)
    }

    // Step 4: Migrate existing token_count data to input_tokens/output_tokens
    // Since we don't have the breakdown, we'll put everything in input_tokens
    // This is a best-effort migration
    _, err = tx.Exec(`
        UPDATE request_history
        SET input_tokens = token_count,
            output_tokens = 0
        WHERE token_count > 0 AND input_tokens = 0
    `)
    if err != nil {
        return fmt.Errorf("failed to migrate token_count data: %w", err)
    }
    log.Println("[Migration] Migrated existing token_count data to input_tokens")

    // Step 5: Update indexes
    // Add index on model for faster queries
    _, err = tx.Exec(`
        CREATE INDEX IF NOT EXISTS idx_request_history_model
        ON request_history(model)
    `)
    if err != nil {
        return fmt.Errorf("failed to create model index: %w", err)
    }
    log.Println("[Migration] Created index on model column")

    // Step 6: Commit transaction
    if err := tx.Commit(); err != nil {
        return fmt.Errorf("failed to commit migration: %w", err)
    }

    log.Println("[Migration] Successfully migrated request_history schema")
    return nil
}
```

**Call this migration in initializeDB:**

In [`usage_tracker.go`](qwencoder-proxy/internal/ratelimit/usage_tracker.go), after creating tables (around line 220), add:

```go
// Migrate request_history schema to add model, input_tokens, output_tokens
if err := ut.migrateRequestHistorySchema(); err != nil {
    ut.logger.WarnLog("[UsageTracker] Failed to migrate request_history schema: %v", err)
    // Don't fail initialization, just log the error
}
```

Add the migration method:

```go
// migrateRequestHistorySchema updates the request_history table schema
func (ut *usageTrackerImpl) migrateRequestHistorySchema() error {
    ut.logger.InfoLog("[UsageTracker] Starting request_history schema migration...")

    // Step 1: Check if migration is needed
    var hasModelColumn int
    err := ut.db.QueryRow(`
        SELECT COUNT(*)
        FROM pragma_table_info('request_history')
        WHERE name = 'model'
    `).Scan(&hasModelColumn)
    if err != nil {
        return fmt.Errorf("failed to check model column: %w", err)
    }

    if hasModelColumn > 0 {
        ut.logger.InfoLog("[UsageTracker] request_history schema already migrated")
        return nil
    }

    // Step 2: Begin transaction
    tx, err := ut.db.Begin()
    if err != nil {
        return fmt.Errorf("failed to begin migration transaction: %w", err)
    }
    defer tx.Rollback()

    // Step 3: Add new columns
    columnsToAdd := []string{
        "model TEXT",
        "input_tokens INTEGER NOT NULL DEFAULT 0",
        "output_tokens INTEGER NOT NULL DEFAULT 0",
    }

    for _, col := range columnsToAdd {
        _, err := tx.Exec(fmt.Sprintf("ALTER TABLE request_history ADD COLUMN %s", col))
        if err != nil {
            return fmt.Errorf("failed to add column %s: %w", col, err)
        }
        ut.logger.InfoLog("[UsageTracker] Added column: %s", col)
    }

    // Step 4: Migrate existing token_count data to input_tokens/output_tokens
    // Since we don't have the breakdown, we'll put everything in input_tokens
    // This is a best-effort migration
    _, err = tx.Exec(`
        UPDATE request_history
        SET input_tokens = token_count,
            output_tokens = 0
        WHERE token_count > 0 AND input_tokens = 0
    `)
    if err != nil {
        return fmt.Errorf("failed to migrate token_count data: %w", err)
    }
    ut.logger.InfoLog("[UsageTracker] Migrated existing token_count data to input_tokens")

    // Step 5: Update indexes
    // Add index on model for faster queries
    _, err = tx.Exec(`
        CREATE INDEX IF NOT EXISTS idx_request_history_model
        ON request_history(model)
    `)
    if err != nil {
        return fmt.Errorf("failed to create model index: %w", err)
    }
    ut.logger.InfoLog("[UsageTracker] Created index on model column")

    // Step 6: Commit transaction
    if err := tx.Commit(); err != nil {
        return fmt.Errorf("failed to commit migration: %w", err)
    }

    ut.logger.InfoLog("[UsageTracker] Successfully migrated request_history schema")
    return nil
}
```

---

### Step 3: Update RecordUsage (or Deprecate)

**Option A: Update RecordUsage to Use New Schema**

**File:** [`qwencoder-proxy/internal/ratelimit/usage_tracker.go`](qwencoder-proxy/internal/ratelimit/usage_tracker.go)

**Current Code (lines 363-373):**
```go
historyQuery := `
    INSERT INTO request_history (id, token_id, request_count, token_count, timestamp)
    VALUES (?, ?, ?, ?, ?)
`
if _, err := tx.ExecContext(ctx, historyQuery,
    historyID, tokenID, requestCount, tokenCount, nowMs); err != nil {
```

**Updated Code:**
```go
historyQuery := `
    INSERT INTO request_history (id, token_id, model, request_count, input_tokens, output_tokens, timestamp)
    VALUES (?, ?, ?, ?, ?, ?, ?)
`
if _, err := tx.ExecContext(ctx, historyQuery,
    historyID, tokenID, nil, requestCount, tokenCount, 0, nowMs); err != nil {
```

**Key Changes:**
- Added `model` parameter (nil for RecordUsage since it doesn't have model info)
- Changed `token_count` to `input_tokens` and `output_tokens`
- Put all tokens in `input_tokens` (since we don't have breakdown)
- Set `output_tokens` to 0

**Option B: Deprecate RecordUsage**

If you're implementing Fix 1 (Duplicate Records), you should deprecate RecordUsage entirely. See Fix 1 for details.

**Recommendation:** If implementing Fix 1, skip this step and deprecate RecordUsage instead.

---

### Step 4: Update RecordModelUsage

**File:** [`qwencoder-proxy/internal/ratelimit/usage_tracker.go`](qwencoder-proxy/internal/ratelimit/usage_tracker.go)

**Current Code (lines 859-869):**
```go
historyQuery := `
    INSERT INTO request_history (id, token_id, model, request_count,
                             input_tokens, output_tokens, timestamp)
    VALUES (?, ?, ?, 1, ?, ?, ?)
`
if _, err := tx.ExecContext(ctx, historyQuery,
    historyID, tokenID, model, inputTokens, outputTokens, nowMs); err != nil {
```

**Status:** This is already correct! No changes needed.

The RecordModelUsage method already uses the correct schema with `model`, `input_tokens`, and `output_tokens`.

---

### Step 5: Update Queries That Use request_history

**File:** [`qwencoder-proxy/internal/ratelimit/usage_tracker.go`](qwencoder-proxy/internal/ratelimit/usage_tracker.go)

**Update GetProviderUsage (lines 447-456):**
```go
// Count requests in sliding window (last 60 seconds).
minuteQuery := `
    SELECT COALESCE(SUM(request_count), 0), COALESCE(SUM(input_tokens + output_tokens), 0)
    FROM request_history
    WHERE timestamp >= ?
`
var requestsInMinute, tokensInMinute int
err := ut.db.QueryRowContext(ctx, minuteQuery, windowStartMs).Scan(&requestsInMinute, &tokensInMinute)
```

**Key Change:** Changed `token_count` to `input_tokens + output_tokens`

**Update GetTokenUsage (lines 489-498):**
```go
// Count requests in sliding window (last 60 seconds).
minuteQuery := `
    SELECT COALESCE(SUM(request_count), 0), COALESCE(SUM(input_tokens + output_tokens), 0)
    FROM request_history
    WHERE token_id = ? AND timestamp >= ?
`
var requestsInMinute, tokensInMinute int
err := ut.db.QueryRowContext(ctx, minuteQuery, tokenID, windowStartMs).Scan(&requestsInMinute, &tokensInMinute)
```

**Key Change:** Changed `token_count` to `input_tokens + output_tokens`

**Update GetAllProviderUsage (lines 524-528):**
```go
// Get all unique provider IDs from provider_usage table.
providersQuery := `
    SELECT DISTINCT provider_id FROM provider_usage
`
```

**Status:** No changes needed - this queries provider_usage, not request_history.

---

### Step 6: Update Indexes

**File:** [`qwencoder-proxy/internal/ratelimit/usage_tracker.go`](qwencoder-proxy/internal/ratelimit/usage_tracker.go)

**Current Indexes (lines 189-198):**
```go
indexes := []string{
    "CREATE INDEX IF NOT EXISTS idx_provider_usage_provider ON provider_usage(provider_id)",
    "CREATE INDEX IF NOT EXISTS idx_token_usage_token ON token_usage(token_id)",
    "CREATE INDEX IF NOT EXISTS idx_request_history_token ON request_history(token_id, timestamp)",
    "CREATE INDEX IF NOT EXISTS idx_request_history_timestamp ON request_history(timestamp)",
    "CREATE INDEX IF NOT EXISTS idx_model_usage_token ON model_usage(token_id)",
    "CREATE INDEX IF NOT EXISTS idx_model_usage_provider ON model_usage(provider_id)",
    "CREATE INDEX IF NOT EXISTS idx_model_usage_model ON model_usage(model)",
    "CREATE INDEX IF NOT EXISTS idx_model_usage_provider_model ON model_usage(provider_id, model)",
}
```

**Add New Index:**
```go
indexes := []string{
    "CREATE INDEX IF NOT EXISTS idx_provider_usage_provider ON provider_usage(provider_id)",
    "CREATE INDEX IF NOT EXISTS idx_token_usage_token ON token_usage(token_id)",
    "CREATE INDEX IF NOT EXISTS idx_request_history_token ON request_history(token_id, timestamp)",
    "CREATE INDEX IF NOT EXISTS idx_request_history_timestamp ON request_history(timestamp)",
    "CREATE INDEX IF NOT EXISTS idx_request_history_model ON request_history(model)",
    "CREATE INDEX IF NOT EXISTS idx_model_usage_token ON model_usage(token_id)",
    "CREATE INDEX IF NOT EXISTS idx_model_usage_provider ON model_usage(provider_id)",
    "CREATE INDEX IF NOT EXISTS idx_model_usage_model ON model_usage(model)",
    "CREATE INDEX IF NOT EXISTS idx_model_usage_provider_model ON model_usage(provider_id, model)",
}
```

**Key Change:** Added index on `model` column for faster model-based queries.

---

### Step 7: Update Tests

**Files to Update:**
- [`qwencoder-proxy/internal/ratelimit/usage_tracker_test.go`](qwencoder-proxy/internal/ratelimit/usage_tracker_test.go)
- [`qwencoder-proxy/internal/ratelimit/integrated_system_test.go`](qwencoder-proxy/internal/ratelimit/integrated_system_test.go)

**Required Changes:**

1. **Update test schema definitions** to include new columns:

```go
func setupTestDB(t *testing.T) *sql.DB {
    db, err := sql.Open("sqlite", ":memory:")
    if err != nil {
        t.Fatalf("Failed to open test database: %v", err)
    }

    // Create tables with new schema
    _, err = db.Exec(`
        CREATE TABLE IF NOT EXISTS request_history (
            id TEXT PRIMARY KEY,
            token_id TEXT NOT NULL,
            model TEXT,
            request_count INTEGER NOT NULL DEFAULT 1,
            input_tokens INTEGER NOT NULL DEFAULT 0,
            output_tokens INTEGER NOT NULL DEFAULT 0,
            timestamp INTEGER NOT NULL,
            created_at INTEGER NOT NULL DEFAULT (strftime('%s', 'subsec') * 1000),
            FOREIGN KEY (token_id) REFERENCES tokens(id) ON DELETE CASCADE
        )
    `)
    if err != nil {
        t.Fatalf("Failed to create request_history table: %v", err)
    }

    // ... other tables ...

    return db
}
```

2. **Update test assertions** to check for new columns:

```go
func TestRecordUsage(t *testing.T) {
    // ... setup ...

    err := tracker.RecordUsage(ctx, "provider1", "token1", 1, 100)
    if err != nil {
        t.Fatalf("Failed to record usage: %v", err)
    }

    // Verify record was created with new columns
    var model sql.NullString
    var inputTokens, outputTokens int
    err = db.QueryRow(`
        SELECT model, input_tokens, output_tokens
        FROM request_history
        WHERE token_id = ?
    `, "token1").Scan(&model, &inputTokens, &outputTokens)
    if err != nil {
        t.Fatalf("Failed to query request_history: %v", err)
    }

    if model.Valid {
        t.Errorf("Expected model to be NULL for RecordUsage, got: %s", model.String)
    }
    if inputTokens != 100 {
        t.Errorf("Expected input_tokens to be 100, got: %d", inputTokens)
    }
    if outputTokens != 0 {
        t.Errorf("Expected output_tokens to be 0, got: %d", outputTokens)
    }
}
```

3. **Add tests for token count calculations:**

```go
func TestTokenCountCalculation(t *testing.T) {
    // ... setup ...

    // Record usage with input and output tokens
    err := tracker.RecordModelUsage(ctx, "provider1", "token1", "gpt-4", 50, 30)
    if err != nil {
        t.Fatalf("Failed to record model usage: %v", err)
    }

    // Verify token counts
    var inputTokens, outputTokens int
    err = db.QueryRow(`
        SELECT input_tokens, output_tokens
        FROM request_history
        WHERE token_id = ? AND model = ?
    `, "token1", "gpt-4").Scan(&inputTokens, &outputTokens)
    if err != nil {
        t.Fatalf("Failed to query request_history: %v", err)
    }

    if inputTokens != 50 {
        t.Errorf("Expected input_tokens to be 50, got: %d", inputTokens)
    }
    if outputTokens != 30 {
        t.Errorf("Expected output_tokens to be 30, got: %d", outputTokens)
    }
}
```

---

### Step 8: Update API Responses (if applicable)

**File:** [`qwencoder-proxy/internal/restapi/rate_limit_api.go`](qwencoder-proxy/internal/restapi/rate_limit_api.go) (if exists)

**Check if any API endpoints return request_history data and update them to include the new columns.**

---

## Verification Steps

After implementing the changes, verify:

1. **Schema Migration:**
   - Run the migration on an existing database
   - Verify new columns are added: `model`, `input_tokens`, `output_tokens`
   - Verify old data is migrated (token_count → input_tokens)

2. **New Records:**
   - Make a test request
   - Query `request_history` table
   - Verify record has: `model`, `input_tokens`, `output_tokens`
   - Verify `input_tokens` and `output_tokens` are correct

3. **Old Records:**
   - Query old records (before migration)
   - Verify they have `input_tokens` populated from old `token_count`
   - Verify `output_tokens` is 0 for old records

4. **Queries Work:**
   - Test GetProviderUsage
   - Test GetTokenUsage
   - Verify they return correct token counts (input + output)

5. **Performance:**
   - Verify queries are still efficient with new indexes
   - Check that model-based queries are fast

---

## Rollback Plan

If issues arise after deployment:

1. **Revert schema changes** in code
2. **No data rollback needed** - old data structure is compatible
3. **Monitor** for any data inconsistencies

---

## Summary

**Files to Modify:**
1. [`qwencoder-proxy/internal/ratelimit/usage_tracker.go`](qwencoder-proxy/internal/ratelimit/usage_tracker.go) - Update schema, add migration, update queries
2. [`qwencoder-proxy/internal/ratelimit/migration.go`](qwencoder-proxy/internal/ratelimit/migration.go) - Add migration function
3. Test files - Update tests

**Expected Outcome:**
- Consistent schema across all records
- Complete token tracking (input + output breakdown)
- Model information captured for all requests
- Better analytics and reporting capabilities
- No data loss during migration

**Estimated Effort:** 2-3 hours

---

## Notes for Code Agent

- **Test migration on a copy of production database** before deploying
- **Backup database** before running migration
- **Monitor logs** during migration to catch any issues
- **Consider running migration during maintenance window** for large databases
- **Verify token counts** after migration to ensure accuracy
- **Update documentation** to reflect new schema
- **Communicate changes** to team members who query request_history

---

## Integration with Fix 1

If implementing both Fix 1 (Duplicate Records) and Fix 2 (Token Count Calculation):

1. **Implement Fix 1 first** - This removes the duplicate RecordUsage call
2. **Then implement Fix 2** - This updates the schema for the remaining RecordModelUsage calls
3. **Result:** Single, consistent record with complete token breakdown

This combination provides the best outcome:
- No duplicate records
- Consistent schema
- Complete token tracking
- Model information captured
