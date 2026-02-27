# File-to-SQLite Migration Strategy - qwencoder-proxy

## Table of Contents

1. [Migration Overview](#migration-overview)
2. [Pre-Migration Preparation](#pre-migration-preparation)
3. [Migration Execution](#migration-execution)
4. [Post-Migration Validation](#post-migration-validation)
5. [Migration Utility Design](#migration-utility-design)
6. [Rollback Strategy](#rollback-strategy)
7. [User Communication](#user-communication)
8. [Edge Cases Handling](#edge-cases-handling)
9. [Migration Timeline](#migration-timeline)
10. [Testing Migration Process](#testing-migration-process)

---

## Migration Overview

### Goals and Objectives

The migration from file-based storage to SQLite storage aims to achieve the following primary objectives:

| Objective | Description | Success Criteria |
|-----------|-------------|------------------|
| **Data Integrity** | Preserve all existing tokens and settings without data loss | 100% data integrity verified |
| **Zero Downtime** | No service interruption during migration | Server remains operational |
| **Backward Compatibility** | Support existing file-based storage during transition | Both backends functional |
| **Seamless Transition** | Transparent migration for end users | No user action required |
| **Performance Improvement** | Achieve equal or better performance than file-based storage | Benchmark targets met |
| **Rollback Capability** | Ability to revert to file-based storage if needed | Rollback procedure tested |
| **Minimal Complexity** | Simple, predictable migration process | Clear steps and documentation |

### Migration Approach

We will use a **phased, dual-mode migration strategy** that allows gradual transition from file-based to SQLite storage:

#### Strategy Overview

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                        Migration Strategy Overview                              │
└─────────────────────────────────────────────────────────────────────────────────┘

Phase 1: Preparation (Pre-Migration)
├─→ Backup existing file-based storage
├─→ Validate data integrity
├─→ Prepare migration environment
└─→ Verify prerequisites

Phase 2: Implementation (Add SQLite Support)
├─→ Implement SQLite storage layer
├─→ Create migration utility
├─→ Add storage backend configuration
└─→ File-based storage remains default

Phase 3: Testing & Validation
├─→ Test migration utility
├─→ Validate data integrity
├─→ Performance benchmarking
└─→ Concurrent access testing

Phase 4: Migration (Execute Migration)
├─→ Run migration utility
├─→ Verify migrated data
├─→ Switch to SQLite backend
└─→ Monitor for issues

Phase 5: Post-Migration
├─→ Validate functionality
├─→ Performance monitoring
├─→ Cleanup legacy files
└─→ Update documentation
```

#### Migration Modes

| Mode | Description | When to Use |
|------|-------------|-------------|
| **One-Time Migration** | Complete migration in single operation | Small to medium datasets (< 1000 tokens) |
| **Incremental Migration** | Migrate in batches over time | Large datasets (> 1000 tokens) or high availability requirements |
| **Parallel Operation** | Run both backends simultaneously | Validation and testing phase |

### Migration Triggers and Conditions

#### Automatic Migration Triggers

| Trigger | Condition | Action |
|---------|-----------|--------|
| **New Installation** | No existing `.credentials/` directory | Initialize SQLite directly |
| **Database Exists** | `tokens.db` file found | Use SQLite backend |
| **File Storage Detected** | `.credentials/` with token files found | Log migration prompt, use file backend |

#### Manual Migration Triggers

| Trigger | Condition | Action |
|---------|-----------|--------|
| **User Initiated** | Explicit migration command or configuration | Run migration utility |
| **Configuration Change** | `STORAGE_BACKEND=sqlite` set | Migrate if file data exists |
| **Admin Request** | Administrative migration request | Run migration with validation |

### Migration Scope and Boundaries

#### In Scope

| Component | Description |
|-----------|-------------|
| **OAuth Tokens** | All provider tokens (Gemini, Qwen, iFlow, Kiro, Antigravity) |
| **Token Metadata** | Health scores, error counts, last used timestamps |
| **Provider Settings** | Selection strategies, refresh buffers, error thresholds |
| **Proxy Configurations** | Proxy settings associated with tokens |
| **Token History** | Creation timestamps, error history |

#### Out of Scope

| Component | Reason |
|-----------|---------|
| **OAuth State** | Temporary state during OAuth flow, not persisted |
| **Application Logs** | Separate logging system |
| **Dashboard State** | Frontend state, not persisted in token store |
| **Runtime Metrics** | In-memory metrics, not persisted |

---

## Pre-Migration Preparation

### Backup Strategy for Existing File-Based Storage

#### Backup Requirements

| Requirement | Description |
|-------------|-------------|
| **Complete Backup** | All `.credentials/` directories and files |
| **Timestamped** | Include migration timestamp in backup name |
| **Verified** | Validate backup integrity before proceeding |
| **Accessible** | Quick access for rollback if needed |
| **Secure** | Preserve file permissions and ownership |

#### Backup Procedure

```
1. Stop the qwencoder-proxy server
   └─→ Ensures no active writes during backup

2. Create timestamped backup
   └─→ .credentials/.backup/<timestamp>/credentials/

3. Verify backup integrity
   ├─→ Check file count matches original
   ├─→ Validate JSON structure of sample files
   └─→ Verify settings files

4. Create backup manifest
   ├─→ List all files with checksums
   ├─→ Record backup timestamp
   └─→ Document migration context

5. Store backup securely
   └─→ Retain for at least 30 days post-migration
```

#### Backup Verification Checklist

| Check | Description | Pass Criteria |
|-------|-------------|---------------|
| **File Count** | Verify all files backed up | Count matches original |
| **Checksums** | Validate file integrity | All checksums match |
| **JSON Validity** | Verify JSON structure | All files parse correctly |
| **Settings Integrity** | Verify settings files | All settings present |
| **Permissions** | Verify file permissions | Original permissions preserved |

### Validation of File-Based Data Integrity

#### Data Validation Checks

| Validation Type | Description | Method |
|----------------|-------------|---------|
| **File Existence** | Verify all token files exist | Directory listing |
| **JSON Structure** | Validate JSON format | JSON parsing |
| **Required Fields** | Check for required fields | Schema validation |
| **Data Types** | Verify field data types | Type checking |
| **Reference Integrity** | Verify proxy references | Cross-reference check |
| **Timestamp Validity** | Check timestamp ranges | Range validation |

#### Pre-Migration Validation Process

```go
// Conceptual validation flow (design only)

type ValidationResult struct {
    Valid      bool
    Errors     []ValidationError
    Warnings   []ValidationWarning
    Stats      ValidationStats
}

type ValidationError struct {
    File       string
    Field      string
    Message    string
    Severity   ErrorSeverity
}

type ValidationStats struct {
    TotalFiles       int
    ValidFiles       int
    InvalidFiles     int
    TotalTokens      int
    ValidTokens      int
    InvalidTokens    int
}

// Validation steps:
// 1. Scan .credentials/ directory structure
// 2. Validate each token file
// 3. Validate settings files
// 4. Check for duplicate tokens
// 5. Verify proxy configurations
// 6. Generate validation report
```

#### Common Validation Issues

| Issue | Severity | Resolution |
|-------|----------|------------|
| **Missing Required Field** | Critical | Skip file, log error |
| **Invalid JSON** | Critical | Skip file, log error |
| **Duplicate Token ID** | Warning | Use first occurrence, log warning |
| **Invalid Timestamp** | Warning | Use current timestamp, log warning |
| **Missing Email** | Warning | Generate filename-based email, log warning |
| **Corrupted Proxy Config** | Warning | Remove proxy reference, log warning |

### Migration Prerequisites Checklist

#### System Requirements

| Requirement | Minimum | Recommended |
|-------------|---------|-------------|
| **Disk Space** | 2x current storage | 3x current storage |
| **Available Memory** | 512 MB | 1 GB |
| **Database Driver** | modernc.org/sqlite installed | Latest version |
| **Go Version** | 1.21+ | Latest stable |
| **File System** | Supports atomic rename | Any POSIX/NTFS |

#### Configuration Prerequisites

| Item | Description | Default Value |
|------|-------------|---------------|
| `STORAGE_BACKEND` | Target storage backend | `sqlite` |
| `STORAGE_PATH` | Database file path | `.credentials/tokens.db` |
| `MIGRATION_DRY_RUN` | Test migration without changes | `false` |
| `MIGRATION_BACKUP` | Create backup before migration | `true` |
| `MIGRATION_VERBOSE` | Detailed logging | `false` |

#### Operational Prerequisites

| Prerequisite | Description | Verification |
|--------------|-------------|---------------|
| **Server Stopped** | No active token operations | Process check |
| **Write Access** | Permissions to write database | File permission check |
| **Network Access** | Access to OAuth providers (for validation) | Connectivity test |
| **Backup Location** | Sufficient space for backup | Disk space check |

### Rollback Preparation

#### Rollback Readiness

| Component | Preparation Step |
|-----------|------------------|
| **Backup** | Complete backup verified and accessible |
| **Procedure** | Rollback steps documented and tested |
| **Configuration** | File-based backend configuration ready |
| **Validation** | Rollback verification checklist prepared |
| **Communication** | Rollback notification plan prepared |

#### Rollback Triggers

| Trigger | Condition | Action |
|---------|-----------|--------|
| **Data Corruption** | Validation fails after migration | Immediate rollback |
| **Performance Degradation** | > 50% performance drop | Evaluate, rollback if needed |
| **Critical Errors** | Server crashes or errors | Immediate rollback |
| **User Reported Issues** | Significant user complaints | Evaluate, rollback if needed |

---

## Migration Execution

### Step-by-Step Migration Process

#### Migration Flow Diagram

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                        Migration Execution Flow                                │
└─────────────────────────────────────────────────────────────────────────────────┘

1. Pre-Migration Checks
   ├─→ Verify prerequisites
   ├─→ Create backup
   ├─→ Validate source data
   └─→ Check available resources

2. Initialize Migration
   ├─→ Load migration configuration
   ├─→ Initialize target database
   ├─→ Apply database schema
   └─→ Prepare migration context

3. Migrate Provider Settings
   ├─→ For each provider:
   │   ├─→ Load settings.json
   │   ├─→ Validate settings
   │   ├─→ Insert into provider_settings table
   │   └─→ Log migration status

4. Migrate Proxy Configurations
   ├─→ Scan all tokens for proxy configs
   ├─→ Deduplicate proxy configurations
   ├─→ Insert into proxy_configs table
   └─→ Generate proxy_id references

5. Migrate Tokens (Batch Processing)
   ├─→ For each provider:
   │   ├─→ Load token files
   │   ├─→ For each batch:
   │   │   ├─→ Validate token data
   │   │   ├─→ Transform data format
   │   │   ├─→ Insert into tokens table
   │   │   └─→ Update progress
   │   └─→ Log batch completion

6. Post-Migration Verification
   ├─→ Count migrated tokens
   ├─→ Validate data integrity
   ├─→ Verify foreign key constraints
   └─→ Generate migration report

7. Cleanup
   ├─→ Archive source files (optional)
   ├─→ Update configuration
   └─→ Complete migration
```

#### Detailed Migration Steps

##### Step 1: Pre-Migration Checks

| Action | Description | Validation |
|--------|-------------|------------|
| **Verify Prerequisites** | Check all system and configuration requirements | Checklist complete |
| **Create Backup** | Create timestamped backup of `.credentials/` | Backup verified |
| **Validate Source Data** | Run data integrity validation | No critical errors |
| **Check Resources** | Verify disk space and memory | Sufficient resources available |
| **Lock Source** | Ensure no writes during migration | Server stopped or locked |

##### Step 2: Initialize Migration

| Action | Description | Validation |
|--------|-------------|------------|
| **Load Configuration** | Read migration configuration | Configuration valid |
| **Initialize Database** | Create/open SQLite database | Database accessible |
| **Apply Schema** | Run database migrations | Schema version 1 applied |
| **Prepare Context** | Initialize migration state and counters | Context ready |

##### Step 3: Migrate Provider Settings

| Action | Description | Validation |
|--------|-------------|------------|
| **Load Settings** | Read `settings.json` for each provider | Settings loaded |
| **Validate Settings** | Check required fields and values | Settings valid |
| **Insert Records** | Insert into `provider_settings` table | Records inserted |
| **Log Status** | Record migration status for provider | Status logged |

##### Step 4: Migrate Proxy Configurations

| Action | Description | Validation |
|--------|-------------|------------|
| **Scan Proxies** | Extract proxy configs from all tokens | Proxies identified |
| **Deduplicate** | Remove duplicate proxy configurations | Unique proxies |
| **Insert Records** | Insert into `proxy_configs` table | Records inserted |
| **Map IDs** | Generate proxy_id for token references | ID mapping created |

##### Step 5: Migrate Tokens

| Action | Description | Validation |
|--------|-------------|------------|
| **Load Tokens** | Read token files for provider | Tokens loaded |
| **Validate Data** | Check required fields and data types | Tokens valid |
| **Transform Data** | Convert to database format | Data transformed |
| **Insert Batch** | Insert batch into `tokens` table | Batch inserted |
| **Update Progress** | Update migration counters | Progress updated |
| **Handle Errors** | Log and continue on non-critical errors | Errors handled |

##### Step 6: Post-Migration Verification

| Action | Description | Validation |
|--------|-------------|------------|
| **Count Records** | Verify record counts match source | Counts match |
| **Validate Integrity** | Check data integrity constraints | Integrity valid |
| **Verify Foreign Keys** | Check foreign key relationships | Keys valid |
| **Generate Report** | Create migration summary report | Report generated |

##### Step 7: Cleanup

| Action | Description | Validation |
|--------|-------------|------------|
| **Archive Source** | Move source files to archive (optional) | Archive created |
| **Update Config** | Set storage backend to SQLite | Config updated |
| **Complete Migration** | Mark migration as complete | Status set |

### Data Transformation Rules

#### Field Mapping

| Source Field | Target Field | Transformation | Notes |
|--------------|--------------|----------------|--------|
| `id` | `id` | Direct copy | UUID string |
| `access_token` | `access_token` | Direct copy | OAuth access token |
| `refresh_token` | `refresh_token` | Direct copy | OAuth refresh token |
| `token_type` | `token_type` | Direct copy | Usually "Bearer" |
| `expiry_date` | `expiry_date` | Direct copy | Unix milliseconds |
| `email` | `email` | Direct copy | User email |
| `resource_url` | `resource_url` | Direct copy | Qwen-specific |
| `scope` | `scope` | Direct copy | Gemini-specific |
| `api_key` | `api_key` | Direct copy | iFlow-specific |
| `healthy` | `healthy` | bool → int | 1 = true, 0 = false |
| `health_score` | `health_score` | Direct copy | Float 0.0-1.0 |
| `last_used` | `last_used` | Direct copy | Unix milliseconds |
| `created_at` | `created_at` | Direct copy | Unix milliseconds |
| `error_count` | `error_count` | Direct copy | Integer |
| `last_error` | `last_error` | Direct copy | String |
| `proxy` | `proxy_id` | Proxy → ID | Foreign key reference |

#### Settings Field Mapping

| Source Field | Target Field | Transformation | Notes |
|--------------|--------------|----------------|--------|
| `selection_strategy` | `selection_strategy` | Direct copy | "random", "round_robin", "least_used" |
| `refresh_buffer_sec` | `refresh_buffer_sec` | Direct copy | Seconds |
| `max_error_count` | `max_error_count` | Direct copy | Integer |

#### Proxy Configuration Transformation

| Source Field | Target Field | Transformation | Notes |
|--------------|--------------|----------------|--------|
| `proxy.host` | `host` | Direct copy | Proxy hostname |
| `proxy.port` | `port` | Direct copy | Proxy port |
| `proxy.username` | `username` | Direct copy | Optional |
| `proxy.password` | `password` | Direct copy | Optional |

### Error Handling During Migration

#### Error Classification

| Error Type | Severity | Action | Recovery |
|------------|----------|--------|----------|
| **File Not Found** | Warning | Log warning, skip file | None needed |
| **Invalid JSON** | Critical | Log error, skip file | Manual review |
| **Missing Required Field** | Critical | Log error, skip file | Manual review |
| **Database Write Error** | Critical | Log error, retry 3x | Rollback if failed |
| **Constraint Violation** | Warning | Log warning, skip record | None needed |
| **Disk Space Error** | Critical | Stop migration | Free space, retry |

#### Error Handling Strategy

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                        Error Handling Strategy                                │
└─────────────────────────────────────────────────────────────────────────────────┘

1. Pre-Migration Errors
   ├─→ Critical: Abort migration
   ├─→ Warning: Log and continue
   └─→ Info: Log for reference

2. Migration Errors
   ├─→ Critical: Stop migration, initiate rollback
   ├─→ Recoverable: Retry up to 3 times
   ├─→ Non-Critical: Log and continue
   └─→ Warnings: Log and continue

3. Post-Migration Errors
   ├─→ Critical: Rollback to file-based storage
   ├─→ Validation Errors: Review and fix manually
   └─→ Warnings: Log for monitoring
```

#### Error Logging

| Log Level | Use Case | Example |
|-----------|----------|---------|
| **ERROR** | Critical failures | "Failed to migrate token {id}: database write error" |
| **WARN** | Non-critical issues | "Skipping token file {file}: invalid JSON" |
| **INFO** | Progress updates | "Migrated 100/500 tokens (20%)" |
| **DEBUG** | Detailed information | "Inserting token {id} with fields: {fields}" |

### Progress Tracking and Reporting

#### Progress Metrics

| Metric | Description | Update Frequency |
|---------|-------------|------------------|
| **Tokens Processed** | Total tokens migrated | Per batch |
| **Tokens Remaining** | Tokens yet to migrate | Per batch |
| **Progress Percentage** | Completion percentage | Per batch |
| **Elapsed Time** | Time since migration start | Per batch |
| **Estimated Time Remaining** | Projected completion time | Per batch |
| **Errors Encountered** | Total error count | Per error |
| **Warnings Encountered** | Total warning count | Per warning |

#### Migration Report

| Section | Description |
|---------|-------------|
| **Summary** | High-level migration overview |
| **Statistics** | Token counts, provider breakdown |
| **Errors** | List of errors encountered |
| **Warnings** | List of warnings encountered |
| **Performance** | Timing and throughput metrics |
| **Verification** | Post-migration validation results |

#### Progress Output Format

```
Migration Progress: [████████████████████░░░░] 75%
Tokens Migrated: 375/500
Providers: gemini (150), qwen (125), iflow (100)
Elapsed: 00:02:30
Estimated Remaining: 00:00:50
Errors: 0
Warnings: 2
```

---

## Post-Migration Validation

### Data Integrity Verification

#### Verification Checks

| Check Type | Description | Method |
|------------|-------------|---------|
| **Record Count** | Verify token counts match | Count comparison |
| **Field Comparison** | Verify field values match | Sample comparison |
| **Constraint Validation** | Check database constraints | Constraint queries |
| **Foreign Key Integrity** | Verify foreign key relationships | Referential integrity check |
| **Data Type Validation** | Verify data type correctness | Type checking |
| **Checksum Verification** | Verify data checksums | Hash comparison |

#### Verification Process

```
1. Record Count Verification
   ├─→ Count tokens in source files
   ├─→ Count tokens in database
   └─→ Compare counts

2. Sample Field Verification
   ├─→ Select random sample of tokens
   ├─→ Compare field values
   └─→ Report discrepancies

3. Constraint Validation
   ├─→ Check NOT NULL constraints
   ├─→ Check UNIQUE constraints
   ├─→ Check FOREIGN KEY constraints
   └─→ Report violations

4. Functional Verification
   ├─→ Load tokens from database
   ├─→ Verify token selection works
   ├─→ Verify token updates work
   └─→ Verify token deletion works
```

#### Verification Results

| Result | Description | Action |
|--------|-------------|--------|
| **PASSED** | All verifications successful | Proceed with migration |
| **PASSED_WITH_WARNINGS** | Minor discrepancies found | Review warnings, proceed |
| **FAILED** | Critical verification failures | Rollback migration |

### Functional Testing

#### Test Scenarios

| Scenario | Description | Expected Result |
|----------|-------------|-----------------|
| **Server Startup** | Start server with SQLite backend | Server starts successfully |
| **Token Load** | Load tokens from database | All tokens loaded |
| **Token Selection** | Select token for API request | Valid token selected |
| **OAuth Flow** | Complete OAuth flow | Token saved to database |
| **Token Refresh** | Refresh expired token | Token updated in database |
| **Token Deletion** | Delete token from database | Token removed |
| **Settings Update** | Update provider settings | Settings saved to database |
| **Concurrent Access** | Multiple simultaneous token operations | No data corruption |

#### Test Execution

```
1. Start Server
   └─→ Verify server starts with SQLite backend

2. Load Tokens
   └─→ Verify all tokens loaded correctly

3. API Request
   └─→ Verify token selection and API call

4. OAuth Flow
   └─→ Verify new token creation

5. Token Refresh
   └─→ Verify token refresh mechanism

6. Settings Update
   └─→ Verify settings persistence

7. Concurrent Operations
   └─→ Verify concurrent access safety
```

### Performance Comparison

#### Performance Metrics

| Metric | File-Based | SQLite | Target |
|--------|------------|--------|--------|
| **Startup Time** | Baseline | ≤ Baseline | No regression |
| **Token Load** | Baseline | ≤ Baseline | No regression |
| **Token Save** | Baseline | ≤ Baseline | No regression |
| **Token Query** | Baseline | ≤ Baseline | No regression |
| **Concurrent Operations** | Baseline | ≥ Baseline | Improvement |

#### Benchmark Methodology

```
1. Baseline Measurement
   └─→ Measure file-based performance

2. SQLite Measurement
   └─→ Measure SQLite performance

3. Comparison
   └─→ Compare metrics

4. Analysis
   └─→ Identify performance differences
```

#### Performance Acceptance Criteria

| Criterion | Requirement |
|-----------|-------------|
| **Startup Time** | ≤ 110% of file-based |
| **Token Load** | ≤ 110% of file-based |
| **Token Save** | ≤ 110% of file-based |
| **Token Query** | ≤ 90% of file-based (expected improvement) |
| **Concurrent Operations** | ≥ 120% of file-based (expected improvement) |

### Cleanup Procedures

#### Post-Migration Cleanup

| Action | Description | Timing |
|--------|-------------|---------|
| **Archive Source Files** | Move `.credentials/` to archive | After validation |
| **Update Documentation** | Update storage documentation | After validation |
| **Update Configuration** | Set default backend to SQLite | After validation |
| **Remove Legacy Code** | Remove file-based code (future) | After deprecation period |
| **Notify Users** | Inform users of migration | After validation |

#### Cleanup Options

| Option | Description | When to Use |
|--------|-------------|-------------|
| **Keep Archive** | Retain source files in archive | Recommended for 30 days |
| **Delete Source** | Remove source files immediately | After validation and backup confirmed |
| **Migrate Archive** | Move archive to long-term storage | For long-term retention |

#### Cleanup Verification

| Check | Description | Pass Criteria |
|-------|-------------|---------------|
| **Archive Created** | Source files archived | Archive exists and is complete |
| **Database Functional** | SQLite database operational | All operations work |
| **Configuration Updated** | Backend set to SQLite | Config reflects SQLite |
| **Documentation Updated** | Documentation reflects SQLite | Docs are accurate |

---

## Migration Utility Design

### Migrator Utility Architecture

#### Component Architecture

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                        Migrator Architecture                                  │
└─────────────────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────────────────┐
│                           Migrator (Main)                                  │
│  - Orchestrates migration process                                          │
│  - Manages migration state                                                  │
│  - Handles errors and recovery                                              │
└─────────────────────────────────────────────────────────────────────────────────┘
         │
         ├─────────────────────────────────────────────────────────────────────┐
         │                                                                     │
         ▼                                                                     ▼
┌──────────────────────────────┐      ┌──────────────────────────────────────┐
│  SourceReader              │      │  TargetWriter                      │
│  - Reads file-based store   │      │  - Writes to SQLite database       │
│  - Validates data          │      │  - Handles batch inserts          │
│  - Provides token stream    │      │  - Manages transactions          │
└──────────────────────────────┘      └──────────────────────────────────────┘
         │                                     │
         │                                     │
         ▼                                     ▼
┌──────────────────────────────┐      ┌──────────────────────────────────────┐
│  DataTransformer          │      │  ProgressTracker                  │
│  - Transforms data format  │      │  - Tracks migration progress     │
│  - Maps fields            │      │  - Calculates metrics            │
│  - Handles type conversion │      │  - Generates reports             │
└──────────────────────────────┘      └──────────────────────────────────────┘
         │
         ▼
┌─────────────────────────────────────────────────────────────────────────────────┐
│                           Validator                                        │
│  - Validates data integrity                                                │
│  - Checks constraints                                                      │
│  - Reports issues                                                         │
└─────────────────────────────────────────────────────────────────────────────────┘
```

#### Migrator Interface

```go
// Conceptual interface design (design only)

type Migrator interface {
    // Prepare prepares the migration environment
    Prepare() error
    
    // Validate validates source data before migration
    Validate() (*ValidationResult, error)
    
    // Migrate executes the migration
    Migrate(ctx context.Context) (*MigrationResult, error)
    
    // Verify verifies the migrated data
    Verify() (*VerificationResult, error)
    
    // Rollback rolls back the migration
    Rollback() error
    
    // GetProgress returns current migration progress
    GetProgress() *MigrationProgress
}

type MigrationConfig struct {
    SourcePath      string
    TargetPath      string
    BatchSize       int
    DryRun          bool
    CreateBackup    bool
    Verbose        bool
    OnError        ErrorAction
}

type MigrationResult struct {
    TokensMigrated   int
    ProvidersMigrated int
    SettingsMigrated int
    Errors          []MigrationError
    Warnings        []MigrationWarning
    Duration        time.Duration
}

type MigrationProgress struct {
    TotalTokens     int
    ProcessedTokens int
    Percentage      float64
    ElapsedTime    time.Duration
    EstimatedRemaining time.Duration
    CurrentProvider string
}
```

### Batch Processing Strategy

#### Batch Size Considerations

| Factor | Small Batch (10-50) | Medium Batch (50-200) | Large Batch (200-500) |
|--------|---------------------|----------------------|----------------------|
| **Memory Usage** | Low | Medium | High |
| **Transaction Time** | Short | Medium | Long |
| **Error Recovery** | Easy | Moderate | Difficult |
| **Progress Granularity** | Fine | Medium | Coarse |
| **Recommended For** | Testing | Production | Very large datasets |

#### Batch Processing Flow

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                        Batch Processing Flow                                  │
└─────────────────────────────────────────────────────────────────────────────────┘

1. Initialize Batch
   ├─→ Start database transaction
   ├─→ Clear batch state
   └─→ Set batch size

2. Load Batch
   ├─→ Read next N tokens from source
   ├─→ Validate token data
   └─→ Transform to target format

3. Process Batch
   ├─→ For each token in batch:
   │   ├─→ Validate data
   │   ├─→ Transform fields
   │   ├─→ Insert into database
   │   └─→ Update counters
   └─→ Handle errors

4. Commit Batch
   ├─→ Commit transaction
   ├─→ Update progress
   └─→ Log batch completion

5. Repeat
   └─→ Continue until all tokens migrated
```

#### Batch Error Handling

| Error Type | Batch Action | Recovery |
|------------|--------------|----------|
| **Single Token Error** | Skip token, continue batch | Log error, continue |
| **Multiple Token Errors** | Skip affected tokens | Log errors, continue |
| **Batch Transaction Error** | Rollback batch, retry | Retry up to 3 times |
| **Critical Error** | Stop migration | Rollback entire migration |

### Retry Logic for Failed Migrations

#### Retry Strategy

| Error Type | Retry Count | Retry Delay | Action on Failure |
|------------|-------------|--------------|------------------|
| **Transient Database Error** | 3 | 1s, 2s, 4s | Abort if failed |
| **Network Timeout** | 3 | 1s, 2s, 4s | Abort if failed |
| **Lock Contention** | 5 | 100ms, 200ms, 400ms, 800ms, 1600ms | Abort if failed |
| **Disk Space Error** | 0 | N/A | Abort immediately |
| **Corruption Error** | 0 | N/A | Abort immediately |

#### Retry Implementation

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                        Retry Logic Flow                                     │
└─────────────────────────────────────────────────────────────────────────────────┘

1. Attempt Operation
   └─→ Try to execute operation

2. Check Result
   ├─→ Success: Return result
   └─→ Failure: Check error type

3. Determine Retryability
   ├─→ Retryable: Proceed to retry
   └─→ Non-retryable: Abort

4. Check Retry Count
   ├─→ Below limit: Proceed to retry
   └─→ At limit: Abort with error

5. Calculate Delay
   └─→ Apply exponential backoff

6. Wait
   └─→ Sleep for delay duration

7. Retry
   └─→ Go to step 1
```

### Logging and Monitoring

#### Log Levels

| Level | Use Case | Output |
|-------|----------|--------|
| **ERROR** | Critical failures | Stderr, log file |
| **WARN** | Non-critical issues | Stdout, log file |
| **INFO** | Progress updates | Stdout, log file |
| **DEBUG** | Detailed information | Log file only |

#### Log Format

```
[timestamp] [level] [component] message
  context: {key: value}
  error: {error details}
```

#### Monitoring Metrics

| Metric | Description | Collection |
|--------|-------------|------------|
| **Migration Progress** | Percentage complete | Per batch |
| **Tokens Per Second** | Throughput metric | Per batch |
| **Error Rate** | Errors per 1000 tokens | Per batch |
| **Memory Usage** | Current memory consumption | Per batch |
| **Disk Usage** | Current disk usage | Per batch |

#### Monitoring Output

```
Migration Status: IN_PROGRESS
Progress: 75% (375/500 tokens)
Throughput: 125 tokens/second
Error Rate: 0.00%
Memory: 45 MB
Disk: 120 MB
Elapsed: 00:03:00
Estimated: 00:01:00
```

---

## Rollback Strategy

### Rollback Triggers

#### Automatic Rollback Triggers

| Trigger | Condition | Action |
|---------|-----------|--------|
| **Validation Failure** | Post-migration validation fails | Automatic rollback |
| **Critical Error** | Unrecoverable error during migration | Automatic rollback |
| **Data Corruption** | Database corruption detected | Automatic rollback |
| **Performance Degradation** | > 50% performance drop | Evaluate, rollback if needed |

#### Manual Rollback Triggers

| Trigger | Condition | Action |
|---------|-----------|--------|
| **User Request** | User initiates rollback | Manual rollback |
| **Admin Decision** | Administrative decision to rollback | Manual rollback |
| **Feature Incompatibility** | Critical feature not working | Manual rollback |

### Rollback Procedures

#### Immediate Rollback Procedure

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                        Immediate Rollback Procedure                            │
└─────────────────────────────────────────────────────────────────────────────────┘

1. Stop Migration
   └─→ Halt migration process immediately

2. Assess State
   ├─→ Determine migration progress
   ├─→ Identify affected data
   └─→ Assess rollback requirements

3. Restore Backup
   ├─→ Restore from backup
   ├─→ Verify backup integrity
   └─→ Confirm restoration

4. Update Configuration
   ├─→ Set storage backend to file
   ├─→ Remove SQLite database (optional)
   └─→ Restart server

5. Verify Operation
   ├─→ Start server
   ├─→ Verify token access
   └─→ Confirm functionality

6. Document Rollback
   ├─→ Record rollback reason
   ├─→ Document issues encountered
   └─→ Update migration status
```

#### Graceful Rollback Procedure

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                        Graceful Rollback Procedure                             │
└─────────────────────────────────────────────────────────────────────────────────┘

1. Notify Users
   ├─→ Announce planned rollback
   ├─→ Provide timeline
   └─→ Set expectations

2. Prepare for Rollback
   ├─→ Verify backup availability
   ├─→ Prepare rollback configuration
   └─→ Schedule maintenance window

3. Execute Rollback
   ├─→ Stop server gracefully
   ├─→ Restore from backup
   ├─→ Update configuration
   └─→ Restart server

4. Verify Operation
   ├─→ Verify all functionality
   ├─→ Run validation tests
   └─→ Confirm service restoration

5. Communicate Completion
   ├─→ Notify users of completion
   ├─→ Provide status update
   └─→ Document rollback
```

### Data Recovery Steps

#### Recovery Scenarios

| Scenario | Recovery Action | Success Criteria |
|----------|-----------------|------------------|
| **Partial Migration** | Restore from backup | All data restored |
| **Corrupted Database** | Restore from backup | Database functional |
| **Lost Backup** | Reconstruct from source files | Data reconstructed |
| **Missing Files** | Use archived files | Files recovered |

#### Recovery Process

```
1. Assess Damage
   ├─→ Identify affected components
   ├─→ Determine data loss
   └─→ Assess recovery options

2. Select Recovery Method
   ├─→ Full backup restore
   ├─→ Partial restore
   └─→ Manual reconstruction

3. Execute Recovery
   ├─→ Restore from backup
   ├─→ Verify restored data
   └─→ Validate functionality

4. Post-Recovery
   ├─→ Run validation tests
   ├─→ Monitor for issues
   └─→ Document recovery
```

### Rollback Verification

#### Verification Checklist

| Check | Description | Pass Criteria |
|-------|-------------|---------------|
| **Server Starts** | Server starts with file backend | Server running |
| **Tokens Accessible** | All tokens can be loaded | All tokens present |
| **OAuth Flow Works** | OAuth flow completes successfully | Flow functional |
| **Token Selection Works** | Token selection operates correctly | Selection works |
| **API Requests Work** | API requests complete successfully | Requests succeed |
| **Settings Persisted** | Settings are correctly loaded | Settings correct |

#### Verification Steps

```
1. Start Server
   └─→ Verify server starts with file backend

2. Load Tokens
   └─→ Verify all tokens load correctly

3. Test OAuth Flow
   └─→ Verify OAuth flow works

4. Test API Requests
   └─→ Verify API requests succeed

5. Verify Settings
   └─→ Verify settings are correct

6. Document Results
   └─→ Record verification results
```

---

## User Communication

### Pre-Migration Notifications

#### Notification Channels

| Channel | Use Case | Timing |
|---------|----------|--------|
| **Release Notes** | Document migration in release notes | Before release |
| **Documentation** | Update user documentation | Before release |
| **Email/Discord** | Direct communication to users | Before release |
| **In-App Notice** | Display notice in dashboard | On migration |
| **Log Messages** | Informative log messages | During migration |

#### Notification Content

| Element | Description |
|---------|-------------|
| **Migration Purpose** | Why migration is happening |
| **Migration Benefits** | What users will gain |
| **Migration Timeline** | When migration will occur |
| **User Action Required** | Any actions users need to take |
| **Downtime Information** | Expected downtime (if any) |
| **Support Contact** | Where to get help |

#### Pre-Migration Notice Template

```
Subject: Upcoming Storage Migration for qwencoder-proxy

Dear qwencoder-proxy User,

We will be migrating the token storage system from file-based to SQLite
to improve performance and reliability.

Migration Details:
- Date: [Date]
- Expected Downtime: Minimal (less than 1 minute)
- User Action Required: None (automatic migration)

Benefits:
- Improved performance
- Better data integrity
- Enhanced reliability

If you have any questions or concerns, please contact support.

Thank you for your patience during this upgrade.
```

### Migration Progress Reporting

#### Progress Reporting Methods

| Method | Description | Update Frequency |
|---------|-------------|------------------|
| **Console Output** | Real-time progress in terminal | Per batch |
| **Log File** | Detailed progress log | Per batch |
| **Progress File** | JSON progress file | Per batch |
| **API Endpoint** | Progress via API (optional) | Per batch |

#### Progress Report Format

```json
{
  "status": "in_progress",
  "progress": {
    "total_tokens": 500,
    "processed_tokens": 375,
    "percentage": 75.0,
    "current_provider": "qwen"
  },
  "timing": {
    "started_at": "2024-01-15T10:00:00Z",
    "elapsed_seconds": 150,
    "estimated_remaining_seconds": 50
  },
  "errors": [],
  "warnings": [
    {
      "message": "Skipping token with invalid email",
      "token_id": "abc123"
    }
  ]
}
```

### Post-Migration Confirmation

#### Confirmation Methods

| Method | Description |
|---------|-------------|
| **Console Message** | Success message in terminal |
| **Log Entry** | Completion log entry |
| **Health Check** | Verify migration status |
| **Dashboard Notice** | Display in dashboard |

#### Confirmation Content

| Element | Description |
|---------|-------------|
| **Migration Status** | Completed/Failed |
| **Tokens Migrated** | Number of tokens migrated |
| **Providers Migrated** | Number of providers migrated |
| **Errors Encountered** | Number of errors (if any) |
| **Warnings Encountered** | Number of warnings (if any) |
| **Next Steps** | What to do next |

#### Post-Migration Notice Template

```
Migration Complete

Storage migration completed successfully.

Summary:
- Tokens migrated: 500
- Providers migrated: 3 (gemini, qwen, iflow)
- Errors: 0
- Warnings: 2

Your qwencoder-proxy is now using SQLite storage.

Next Steps:
- No action required
- Your tokens are available immediately

If you experience any issues, please contact support.
```

### Troubleshooting Guidance

#### Common Issues

| Issue | Symptoms | Resolution |
|-------|----------|------------|
| **Migration Fails** | Error message during migration | Check logs, verify prerequisites |
| **Tokens Missing** | Some tokens not available after migration | Check warnings, restore from backup |
| **Server Won't Start** | Server fails to start after migration | Check configuration, rollback if needed |
| **Performance Issues** | Slower performance after migration | Check database pragmas, verify indexes |
| **OAuth Flow Fails** | OAuth flow not working after migration | Verify database connectivity, check logs |

#### Troubleshooting Steps

```
1. Identify Issue
   └─→ Review symptoms and error messages

2. Check Logs
   ├─→ Review migration logs
   ├─→ Check server logs
   └─→ Look for error patterns

3. Verify Configuration
   ├─→ Check storage backend setting
   ├─→ Verify database path
   └─→ Confirm file permissions

4. Test Basic Operations
   ├─→ Test server startup
   ├─→ Test token loading
   └─→ Test basic API requests

5. Seek Help
   ├─→ Review documentation
   ├─→ Check known issues
   └─→ Contact support if needed
```

#### Support Resources

| Resource | Description |
|----------|-------------|
| **Documentation** | Migration guide and troubleshooting |
| **GitHub Issues** | Known issues and solutions |
| **Community** | Discord/Slack for community support |
| **Email Support** | Direct support contact |

---

## Edge Cases Handling

### Corrupted Files

#### Detection

| Detection Method | Description |
|-----------------|-------------|
| **JSON Parse Error** | File cannot be parsed as JSON |
| **Missing Required Field** | Required field is absent |
| **Invalid Data Type** | Field value has wrong type |
| **Checksum Mismatch** | File checksum doesn't match |

#### Handling Strategy

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                        Corrupted File Handling                               │
└─────────────────────────────────────────────────────────────────────────────────┘

1. Detect Corruption
   └─→ Identify corrupted file

2. Log Error
   ├─→ Log file path
   ├─→ Log error details
   └─→ Log severity

3. Determine Action
   ├─→ Critical field missing: Skip file
   ├─→ Optional field missing: Use default
   ├─→ Invalid data type: Skip field
   └─→ Recoverable data: Extract what's possible

4. Record Decision
   ├─→ Log action taken
   ├─→ Record in migration report
   └─→ Flag for manual review

5. Continue Migration
   └─→ Proceed with remaining files
```

#### Recovery Options

| Option | Description | When to Use |
|--------|-------------|-------------|
| **Skip File** | Skip corrupted file entirely | Critical corruption |
| **Partial Migration** | Migrate available data | Partial corruption |
| **Manual Repair** | Manually fix file | Recoverable corruption |
| **Restore from Backup** | Use backup version | Backup available |

### Duplicate Tokens

#### Detection

| Detection Method | Description |
|-----------------|-------------|
| **Duplicate ID** | Same token ID in multiple files |
| **Duplicate Refresh Token** | Same refresh token for multiple IDs |
| **Duplicate Email** | Same email for multiple tokens |

#### Handling Strategy

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                        Duplicate Token Handling                              │
└─────────────────────────────────────────────────────────────────────────────────┘

1. Detect Duplicate
   └─→ Identify duplicate tokens

2. Compare Tokens
   ├─→ Compare all fields
   ├─→ Identify differences
   └─→ Determine most recent

3. Resolve Duplicate
   ├─→ Identical tokens: Keep one, skip others
   ├─→ Different tokens: Keep most recent
   └─→ Conflicting data: Log warning, keep first

4. Record Resolution
   ├─→ Log duplicate found
   ├─→ Log resolution action
   └─→ Record in migration report

5. Continue Migration
   └─→ Proceed with remaining tokens
```

#### Resolution Rules

| Scenario | Resolution |
|----------|------------|
| **Identical Tokens** | Keep first occurrence, skip others |
| **Different Expiry** | Keep token with later expiry |
| **Different Health Score** | Keep token with higher health score |
| **Different Last Used** | Keep token with more recent last used |
| **Conflicting Data** | Keep first occurrence, log warning |

### Missing Settings

#### Detection

| Detection Method | Description |
|-----------------|-------------|
| **File Not Found** | Settings file does not exist |
| **Invalid JSON** | Settings file cannot be parsed |
| **Missing Field** | Required setting field is absent |

#### Handling Strategy

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                        Missing Settings Handling                            │
└─────────────────────────────────────────────────────────────────────────────────┘

1. Detect Missing Settings
   └─→ Identify missing settings file or fields

2. Apply Defaults
   ├─→ Use default selection_strategy: "random"
   ├─→ Use default refresh_buffer_sec: 1800
   ├─→ Use default max_error_count: 3
   └─→ Use default updated_at: current timestamp

3. Log Warning
   ├─→ Log missing settings
   ├─→ Log defaults applied
   └─→ Record in migration report

4. Continue Migration
   └─→ Proceed with default settings
```

#### Default Values

| Setting | Default Value | Description |
|----------|---------------|-------------|
| `selection_strategy` | "random" | Token selection strategy |
| `refresh_buffer_sec` | 1800 | Refresh buffer in seconds |
| `max_error_count` | 3 | Maximum error count before marking unhealthy |
| `updated_at` | current timestamp | Last update timestamp |

### Partial Migration Recovery

#### Detection

| Detection Method | Description |
|-----------------|-------------|
| **Migration Interrupted** | Migration process stopped |
| **Incomplete Batch** | Batch not fully committed |
| **Progress File** | Progress file shows incomplete state |

#### Recovery Strategy

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                        Partial Migration Recovery                            │
└─────────────────────────────────────────────────────────────────────────────────┘

1. Detect Partial Migration
   └─→ Identify incomplete migration

2. Assess State
   ├─→ Check database state
   ├─→ Check source files
   └─→ Determine what was migrated

3. Choose Recovery Option
   ├─→ Resume migration: Continue from last checkpoint
   ├─→ Rollback: Restore from backup
   └─→ Restart: Delete database and start over

4. Execute Recovery
   ├─→ Resume: Continue migration
   ├─→ Rollback: Restore backup
   └─→ Restart: Delete and retry

5. Verify Recovery
   ├─→ Verify database state
   ├─→ Verify data integrity
   └─→ Confirm migration complete
```

#### Recovery Options

| Option | Description | When to Use |
|--------|-------------|-------------|
| **Resume Migration** | Continue from checkpoint | Migration checkpointed |
| **Rollback** | Restore from backup | Backup available |
| **Restart** | Delete database and retry | No checkpoint, no backup |
| **Manual Recovery** | Manually fix and continue | Special cases |

---

## Migration Timeline

### Estimated Time for Different Data Sizes

| Data Size | Estimated Time | Breakdown |
|-----------|----------------|-----------|
| **Small** (< 50 tokens) | < 1 minute | Preparation: 10s, Migration: 30s, Validation: 20s |
| **Medium** (50-200 tokens) | 1-3 minutes | Preparation: 30s, Migration: 60s, Validation: 90s |
| **Large** (200-1000 tokens) | 3-10 minutes | Preparation: 60s, Migration: 300s, Validation: 240s |
| **Very Large** (> 1000 tokens) | 10-30 minutes | Preparation: 120s, Migration: 1200s, Validation: 480s |

### Phased Migration Approach

#### Phase 1: Preparation (1-2 hours)

| Task | Duration | Description |
|------|-----------|-------------|
| **Backup Creation** | 5-30 minutes | Create and verify backup |
| **Data Validation** | 10-30 minutes | Validate source data |
| **Environment Setup** | 5-10 minutes | Prepare migration environment |
| **Configuration** | 5-10 minutes | Configure migration parameters |

#### Phase 2: Testing (1-2 hours)

| Task | Duration | Description |
|------|-----------|-------------|
| **Dry Run** | 10-30 minutes | Test migration without changes |
| **Validation Testing** | 20-30 minutes | Test validation procedures |
| **Rollback Testing** | 10-20 minutes | Test rollback procedures |
| **Documentation Review** | 20-40 minutes | Review migration documentation |

#### Phase 3: Execution (Variable)

| Task | Duration | Description |
|------|-----------|-------------|
| **Migration** | 1-30 minutes | Execute migration |
| **Verification** | 5-15 minutes | Verify migrated data |
| **Functional Testing** | 10-30 minutes | Test functionality |

#### Phase 4: Post-Migration (30-60 minutes)

| Task | Duration | Description |
|------|-----------|-------------|
| **Performance Monitoring** | 15-30 minutes | Monitor performance |
| **Issue Resolution** | Variable | Address any issues |
| **Cleanup** | 10-20 minutes | Cleanup source files |
| **Documentation** | 5-10 minutes | Update documentation |

### Concurrent Migration Options

#### Parallel Provider Migration

| Approach | Description | Benefits |
|----------|-------------|-----------|
| **Sequential** | Migrate providers one at a time | Simple, predictable |
| **Parallel** | Migrate providers concurrently | Faster, more complex |

#### Parallel Migration Strategy

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                        Parallel Migration Strategy                             │
└─────────────────────────────────────────────────────────────────────────────────┘

1. Initialize Migration
   └─→ Prepare migration context

2. Start Provider Migrations
   ├─→ Start gemini migration (goroutine 1)
   ├─→ Start qwen migration (goroutine 2)
   ├─→ Start iflow migration (goroutine 3)
   └─→ Wait for all to complete

3. Aggregate Results
   ├─→ Collect results from all providers
   ├─→ Check for errors
   └─→ Generate combined report

4. Complete Migration
   └─→ Finalize migration
```

#### Concurrent Access Considerations

| Consideration | Description | Mitigation |
|---------------|-------------|------------|
| **Database Locks** | Concurrent writes may conflict | Use WAL mode |
| **Memory Usage** | Concurrent migrations use more memory | Limit parallelism |
| **Error Handling** | Errors in one migration affect others | Isolate migrations |
| **Progress Tracking** | Track progress for each provider | Per-provider progress |

### Migration Scheduling

#### Scheduling Options

| Option | Description | When to Use |
|--------|-------------|-------------|
| **Immediate** | Migrate immediately | Small datasets, low usage |
| **Scheduled** | Schedule for specific time | Larger datasets, planned maintenance |
| **On-Demand** | User-initiated migration | User control required |

#### Recommended Schedule

| Scenario | Recommended Schedule |
|----------|---------------------|
| **Production - Small** | During low usage period |
| **Production - Large** | During planned maintenance window |
| **Development** | Anytime, immediate |
| **Staging** | Before production migration |

#### Scheduling Checklist

| Check | Description |
|-------|-------------|
| **Low Usage Period** | Schedule during low traffic |
| **Maintenance Window** | Announce maintenance window |
| **Backup Available** | Verify backup is available |
| **Rollback Plan Ready** | Confirm rollback plan is prepared |
| **Support Available** | Ensure support is available |

---

## Testing Migration Process

### Test Environment Setup

#### Test Environment Requirements

| Requirement | Description | Minimum |
|-------------|-------------|----------|
| **Isolated Environment** | Separate from production | Required |
| **Sample Data** | Representative dataset | Required |
| **Test Configuration** | Production-like configuration | Required |
| **Monitoring** | Logging and monitoring | Required |
| **Rollback Capability** | Ability to rollback | Required |

#### Test Data Preparation

| Data Type | Description | Source |
|-----------|-------------|--------|
| **Sample Tokens** | Representative token data | Production export or synthetic |
| **Edge Cases** | Tokens with edge cases | Synthetic |
| **Corrupted Data** | Files with various corruptions | Synthetic |
| **Large Dataset** | Large number of tokens | Synthetic or production export |

#### Test Environment Checklist

| Check | Description | Pass Criteria |
|-------|-------------|---------------|
| **Environment Isolated** | No impact on production | Environment separate |
| **Data Prepared** | Test data available | Data loaded |
| **Configuration Set** | Test configuration applied | Config correct |
| **Monitoring Enabled** | Logging and monitoring active | Monitoring working |
| **Rollback Ready** | Rollback procedure tested | Rollback tested |

### Dry-Run Migration

#### Dry-Run Purpose

| Purpose | Description |
|---------|-------------|
| **Validate Process** | Verify migration process works |
| **Estimate Time** | Estimate migration duration |
| **Identify Issues** | Find potential issues |
| **Test Rollback** | Test rollback procedures |

#### Dry-Run Process

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                        Dry-Run Migration Process                             │
└─────────────────────────────────────────────────────────────────────────────────┘

1. Initialize Dry-Run
   ├─→ Set dry-run flag
   ├─→ Enable verbose logging
   └─→ Disable actual writes

2. Execute Migration
   ├─→ Read source data
   ├─→ Validate data
   ├─→ Transform data
   └─→ Log what would be written

3. Generate Report
   ├─→ Report tokens that would be migrated
   ├─→ Report errors that would occur
   ├─→ Report warnings
   └─→ Estimate time

4. Review Results
   ├─→ Review migration report
   ├─→ Address any issues
   └─→ Approve for actual migration
```

#### Dry-Run Validation

| Validation | Description |
|-------------|-------------|
| **Process Completes** | Dry-run completes without errors |
| **All Data Validated** | All source data validated |
| **No Critical Errors** | No critical errors encountered |
| **Time Estimate** | Reasonable time estimate generated |

### Test Rollback Procedures

#### Rollback Test Scenarios

| Scenario | Description |
|----------|-------------|
| **Immediate Rollback** | Rollback immediately after migration |
| **Delayed Rollback** | Rollback after some usage |
| **Partial Rollback** | Rollback with partial migration |
| **Failed Rollback** | Test rollback failure handling |

#### Rollback Test Process

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                        Rollback Test Process                                 │
└─────────────────────────────────────────────────────────────────────────────────┘

1. Execute Migration
   └─→ Complete migration successfully

2. Verify Migration
   ├─→ Verify data migrated
   ├─→ Verify functionality
   └─→ Record state

3. Execute Rollback
   ├─→ Initiate rollback
   ├─→ Restore from backup
   └─→ Update configuration

4. Verify Rollback
   ├─→ Verify server starts
   ├─→ Verify tokens accessible
   └─→ Verify functionality

5. Document Results
   ├─→ Record rollback test results
   ├─→ Document any issues
   └─→ Update rollback procedures
```

#### Rollback Test Checklist

| Check | Description | Pass Criteria |
|-------|-------------|---------------|
| **Rollback Completes** | Rollback completes without errors | Rollback successful |
| **Data Restored** | All data restored from backup | Data complete |
| **Server Starts** | Server starts after rollback | Server running |
| **Functionality Works** | All functionality works | Operations successful |
| **No Data Loss** | No data lost during rollback | Data intact |

### Migration Validation Tests

#### Validation Test Scenarios

| Scenario | Description |
|----------|-------------|
| **Data Integrity** | Verify data integrity after migration |
| **Functionality** | Verify all functionality works |
| **Performance** | Verify performance meets targets |
| **Concurrency** | Verify concurrent access works |
| **Edge Cases** | Verify edge cases handled correctly |

#### Validation Test Process

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                        Validation Test Process                                │
└─────────────────────────────────────────────────────────────────────────────────┘

1. Data Integrity Tests
   ├─→ Compare record counts
   ├─→ Compare field values
   ├─→ Verify constraints
   └─→ Check foreign keys

2. Functionality Tests
   ├─→ Test server startup
   ├─→ Test token loading
   ├─→ Test token selection
   ├─→ Test OAuth flow
   ├─→ Test token refresh
   └─→ Test token deletion

3. Performance Tests
   ├─→ Measure startup time
   ├─→ Measure load time
   ├─→ Measure save time
   └─→ Compare to baseline

4. Concurrency Tests
   ├─→ Test concurrent reads
   ├─→ Test concurrent writes
   └─→ Test mixed operations

5. Edge Case Tests
   ├─→ Test with corrupted data
   ├─→ Test with duplicates
   ├─→ Test with missing settings
   └─→ Test with large datasets
```

#### Validation Test Results

| Test | Result | Notes |
|------|--------|-------|
| **Data Integrity** | PASSED | All data verified |
| **Functionality** | PASSED | All functions work |
| **Performance** | PASSED | Meets targets |
| **Concurrency** | PASSED | No issues found |
| **Edge Cases** | PASSED | All cases handled |

---

## Related Documents

This migration strategy document is part of a comprehensive SQLite migration plan for qwencoder-proxy. For complete context, refer to the following documents:

| Document | Description |
|----------|-------------|
| [`01-current-architecture-analysis.md`](./01-current-architecture-analysis.md) | Analysis of current file-based storage architecture |
| [`02-sqlite-database-schema-design.md`](./02-sqlite-database-schema-design.md) | SQLite database schema design and structure |
| [`03-database-initialization-strategy.md`](./03-database-initialization-strategy.md) | Database initialization and migration system |
| [`04-sqlite-storage-layer-design.md`](./04-sqlite-storage-layer-design.md) | SQLite storage layer interface and implementation |
| [`05-code-refactoring-plan.md`](./05-code-refactoring-plan.md) | Code refactoring plan for SQLite integration |

---

## Summary

This migration strategy provides a comprehensive, step-by-step approach for transitioning the qwencoder-proxy from file-based storage to SQLite storage. The strategy emphasizes:

1. **Data Integrity**: Comprehensive validation and verification ensure no data loss
2. **Zero Downtime**: Phased approach minimizes service disruption
3. **Rollback Capability**: Tested rollback procedures enable quick recovery
4. **User Safety**: Clear communication and troubleshooting guidance
5. **Edge Case Handling**: Robust handling of corrupted files, duplicates, and missing data

The migration is designed to be transparent to end users while providing administrators with the tools and procedures needed to execute a successful migration.

---

## Appendix

### A. Migration Command Reference

| Command | Description |
|----------|-------------|
| `migrate --dry-run` | Test migration without making changes |
| `migrate --backup` | Create backup before migration |
| `migrate --verbose` | Enable detailed logging |
| `migrate --batch-size=N` | Set batch size for migration |
| `migrate --rollback` | Rollback to file-based storage |
| `migrate --verify` | Verify migration integrity |

### B. Configuration Reference

| Configuration | Description | Default |
|---------------|-------------|---------|
| `STORAGE_BACKEND` | Storage backend to use | `auto` |
| `STORAGE_PATH` | Path to storage file/directory | `.credentials/tokens.db` |
| `MIGRATION_BATCH_SIZE` | Number of tokens per batch | `100` |
| `MIGRATION_DRY_RUN` | Test migration without changes | `false` |
| `MIGRATION_BACKUP` | Create backup before migration | `true` |
| `MIGRATION_VERBOSE` | Enable detailed logging | `false` |

### C. Exit Codes

| Code | Description |
|------|-------------|
| `0` | Success |
| `1` | General error |
| `2` | Validation error |
| `3` | Migration failed |
| `4` | Rollback failed |
| `5` | Verification failed |

### D. Glossary

| Term | Definition |
|-------|------------|
| **Batch** | Group of tokens processed together in a transaction |
| **Checkpoint** | Point in migration from which migration can be resumed |
| **Dry Run** | Test migration without making actual changes |
| **Migration** | Process of moving data from file-based to SQLite storage |
| **Rollback** | Process of reverting to file-based storage |
| **Validation** | Process of verifying data integrity and correctness |
