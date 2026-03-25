# Remove File-Based Storage

**Priority:** LOW  
**Estimated Time:** 1 day  
**Complexity:** Low  
**Files to Delete:** 5  
**Files to Modify:** 3

---

## Problem Description

The codebase maintains dual storage backends - file-based (legacy) and SQLite (new). This creates:
- Increased complexity with dual storage paths
- Migration logic required
- Potential data inconsistency
- Code duplication for storage operations
- Maintenance burden

### Current State

**File-Based Storage (Legacy):**
- Location: `.credentials/{provider}/*.json`
- Used by: Old token management code
- Purpose: Store tokens as individual JSON files

**SQLite Storage (New):**
- Location: `.credentials/tokens.db`
- Used by: New token management code
- Purpose: Store all tokens in single database

**Migration Complexity:**
- Migration plan: [`docs/migration/comprehensive-sqlite-migration-plan.md`](qwencoder-proxy/docs/migration/comprehensive-sqlite-migration-plan.md)
- Migration code in `internal/token/migrator.go`
- Dual storage support in code
- Complexity in handling both backends

### Examples of Dual Storage

**Example 1: In `internal/token/store_factory.go`**

```go
// Returns either file-based or SQLite store
func (sf *StoreFactory) GetStore(providerID string) (TokenStore, error) {
	// Check if SQLite is enabled
	if sf.useSQLite {
		return sf.getSQLiteStore(providerID)
	}
	
	// Fall back to file-based store
	return sf.getFileStore(providerID)
}
```

**Example 2: In `internal/token/multi_token_manager.go`**

```go
// Manages both SQLite and file-based stores
type MultiTokenManager struct {
	stores              map[string]*SQLiteStore
	fileStores          map[string]*FileStore  // Legacy
	// ...
}
```

**Problems:**
- Code must handle both storage types
- Testing requires mocking both types
- Difficult to ensure consistency
- Migration logic scattered throughout code

---

## Solution Architecture

### Remove File-Based Storage Completely

After SQLite migration is complete and verified:
1. Remove all file-based storage code
2. Remove migration logic
3. Simplify storage factory
4. Update all references to use SQLite only

---

## Implementation Plan

### Phase 1: Remove File-Based Store Implementation (Day 1)

#### Step 1.1: Delete FileStore

**Delete File:** `internal/token/file_store.go`

**Action:** Completely remove this file

**Reason:** File-based storage is legacy and no longer needed after SQLite migration.

#### Step 1.2: Delete Migrator

**Delete File:** `internal/token/migrator.go`

**Action:** Completely remove this file

**Reason:** Migration logic no longer needed after full SQLite adoption.

#### Step 1.3: Update StoreFactory

**Modify File:** `internal/token/store_factory.go`

**Location:** Simplify to only use SQLite

```go
// BEFORE: Dual storage support
type StoreFactory struct {
	useSQLite   bool
	credentialsDir string
	logger      logging.Logger
}

func (sf *StoreFactory) GetStore(providerID string) (TokenStore, error) {
	if sf.useSQLite {
		return sf.getSQLiteStore(providerID)
	}
	return sf.getFileStore(providerID)
}

// AFTER: SQLite-only storage
type StoreFactory struct {
	dbPath  string
	logger  logging.Logger
}

func NewStoreFactory(dbPath string, logger logging.Logger) *StoreFactory {
	return &StoreFactory{
		dbPath: dbPath,
		logger: logger,
	}
}

func (sf *StoreFactory) GetStore(providerID string) (TokenStore, error) {
	return sf.getSQLiteStore(providerID)
}

func (sf *StoreFactory) getSQLiteStore(providerID string) (TokenStore, error) {
	store, err := NewSQLiteStore(providerID, sf.dbPath, sf.logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create SQLite store for %s: %w", providerID, err)
	}
	return store, nil
}
```

### Phase 2: Update MultiTokenManager (Day 1)

#### Step 2.1: Remove File Store References

**Modify File:** `internal/token/multi_token_manager.go`

**Location:** Remove file store fields and methods

```go
// BEFORE: Dual storage support
type MultiTokenManager struct {
	stores              map[string]*SQLiteStore
	fileStores          map[string]*FileStore  // Remove this
	// ...
}

// AFTER: SQLite-only storage
type MultiTokenManager struct {
	stores map[string]*SQLiteStore
	// ...
}
```

#### Step 2.2: Remove Migration Methods

**Modify File:** `internal/token/multi_token_manager.go`

**Location:** Remove migration-related methods

```go
// Remove these methods if they exist:
// - MigrateFromFile()
// - MigrateToFile()
// - GetMigrationStatus()
```

### Phase 3: Update REST API (Day 1)

#### Step 3.1: Remove File-Based Storage References

**Modify File:** `internal/restapi/rest_api.go`

**Location:** Update to use only SQLite storage

```go
// BEFORE: Dual storage support
func (s *Server) getTokenStore(providerID string) (TokenStore, error) {
	// May return file-based or SQLite store
	// ...
}

// AFTER: SQLite-only storage
func (s *Server) getTokenStore(providerID string) (TokenStore, error) {
	if s.multiTokenManager == nil {
		return nil, fmt.Errorf("multi-token manager not initialized")
	}
	
	return s.multiTokenManager.GetTokenStore(providerID)
}
```

### Phase 4: Update Main Package (Day 1)

#### Step 4.1: Remove Migration Flags

**Modify File:** `cmd/main.go`

**Location:** Remove migration-related configuration

```go
// BEFORE: Migration configuration
type Config struct {
	// ...
	Migration MigrationConfig `mapstructure:"migration"`
}

// AFTER: No migration configuration
type Config struct {
	// ... (remove Migration field)
}

// Remove migration-related code
// - Remove migration initialization
// - Remove migration status checks
```

### Phase 5: Update Configuration (Day 1)

#### Step 5.1: Remove File Storage Config

**Modify File:** `internal/config/config.go`

**Location:** Remove file storage configuration

```go
// BEFORE: Dual storage configuration
type Config struct {
	Storage StorageConfig `mapstructure:"storage"`
}

type StorageConfig struct {
	UseSQLite    bool   `mapstructure:"use_sqlite"`
	DBPath       string `mapstructure:"db_path"`
	CredentialsDir string `mapstructure:"credentials_dir"`
}

// AFTER: SQLite-only configuration
type Config struct {
	Storage StorageConfig `mapstructure:"storage"`
}

type StorageConfig struct {
	DBPath       string `mapstructure:"db_path"`  // Remove UseSQLite and CredentialsDir
}
```

### Phase 6: Update Documentation (Day 1)

#### Step 6.1: Update Migration Documentation

**Modify File:** `docs/migration/comprehensive-sqlite-migration-plan.md`

**Location:** Mark as complete and add cleanup instructions

```markdown
## Migration Complete

The migration from file-based to SQLite storage is complete.

### Cleanup Instructions

1. **Remove File-Based Storage Code:**
   - Delete `internal/token/file_store.go`
   - Delete `internal/token/migrator.go`
   - Remove all file store references

2. **Update Configuration:**
   - Remove `USE_SQLITE` environment variable
   - Remove migration configuration options

3. **Verify SQLite Storage:**
   - Ensure all tokens are in SQLite database
   - Verify token access works correctly
   - Check that no file-based storage is used

4. **Clean Up Files:**
   - Remove `.credentials/{provider}/*.json` files after verification
   - Remove `.credentials/{provider}/` directories if empty
   - Keep only `.credentials/tokens.db`

### Rollback Plan

If issues arise, restore file-based storage from git:
```bash
git checkout HEAD~1 -- internal/token/file_store.go
git checkout HEAD~1 -- internal/token/migrator.go
```
```

---

## Testing

### Integration Tests

**New File:** `internal/token/storage_test.go`

```go
package token

import (
	"testing"
	"github.com/stretchr/testify/assert"
)

func TestSQLiteOnlyStorage(t *testing.T) {
	// Test that only SQLite storage is used
	factory := NewStoreFactory(":memory:", logging.NewLogger())
	
	store, err := factory.GetStore("test-provider")
	assert.NoError(t, err)
	assert.IsType(t, &SQLiteStore{}, store)
}

func TestNoFileStore(t *testing.T) {
	// Test that file store is not available
	factory := NewStoreFactory(":memory:", logging.NewLogger())
	
	// Should only return SQLite store
	store, err := factory.GetStore("test-provider")
	assert.NoError(t, err)
	assert.IsType(t, &SQLiteStore{}, store)
	
	// FileStore type should not exist or be inaccessible
	// This test verifies file-based storage is removed
}

func TestStorageConsistency(t *testing.T) {
	// Test that storage is consistent (only SQLite)
	factory := NewStoreFactory(":memory:", logging.NewLogger())
	
	store1, _ := factory.GetStore("provider1")
	store2, _ := factory.GetStore("provider2")
	
	// Both should be SQLite stores
	assert.IsType(t, &SQLiteStore{}, store1)
	assert.IsType(t, &SQLiteStore{}, store2)
}
```

---

## Verification Checklist

### Phase 1: Remove File-Based Code
- [ ] `internal/token/file_store.go` deleted
- [ ] `internal/token/migrator.go` deleted
- [ ] No references to file store remain

### Phase 2: Update Store Factory
- [ ] `store_factory.go` simplified to SQLite-only
- [ ] `useSQLite` field removed
- [ ] `getFileStore()` method removed
- [ ] All tests pass

### Phase 3: Update MultiTokenManager
- [ ] `fileStores` field removed
- [ ] Migration methods removed
- [ ] All tests pass

### Phase 4: Update REST API
- [ ] All storage references use SQLite
- [ ] No file-based storage logic remains
- [ ] All tests pass

### Phase 5: Update Main Package
- [ ] Migration configuration removed
- [ ] Migration flags removed
- [ ] All tests pass

### Phase 6: Update Configuration
- [ ] File storage config removed
- [ ] `UseSQLite` config removed
- [ ] All tests pass

### Phase 7: Documentation
- [ ] Migration docs marked complete
- [ ] Cleanup instructions added
- [ ] Rollback plan documented

---

## Impact

**Positive:**
- Simplified codebase (single storage backend)
- Reduced complexity and maintenance burden
- Eliminated migration logic
- Better data consistency
- Easier testing (single storage type)
- Reduced code volume

**Code Volume Reduction:**
- Deleted: ~500 lines (file_store.go)
- Deleted: ~300 lines (migrator.go)
- Removed: ~200 lines (dual storage support)
- Total removed: ~1000 lines
- Net reduction: ~1000 lines

**Risk:**
- Low - SQLite migration should be complete and verified
- Rollback available via git
- No breaking changes to existing functionality

**Side Effects:**
- File-based storage no longer available
- All storage operations use SQLite
- Migration code removed
- Simpler configuration

---

## Prerequisites

### Before Removal

1. **Complete SQLite Migration:**
   - All tokens migrated to SQLite
   - SQLite storage verified working
   - No data loss during migration

2. **Backup File-Based Storage:**
   - Backup `.credentials/` directory
   - Verify backup integrity
   - Keep backup for rollback period

3. **Testing:**
   - All tests pass with SQLite-only storage
   - Integration tests pass
   - Manual verification complete

4. **Documentation:**
   - Migration plan complete
   - Rollback procedure documented
   - Cleanup instructions clear

---

## Cleanup Commands

### Remove File-Based Storage Files

```bash
# Remove file-based storage implementation
rm internal/token/file_store.go
rm internal/token/migrator.go

# Remove file-based storage directories (after verification)
# Only run after confirming SQLite migration is complete
rm -rf .credentials/qwen
rm -rf .credentials/gemini-cli
rm -rf .credentials/kiro
rm -rf .credentials/antigravity
rm -rf .credentials/iflow

# Keep only SQLite database
# .credentials/tokens.db remains
```

### Remove Migration Configuration

```bash
# Remove migration environment variables
# From .env or system environment
unset USE_SQLITE
unset MIGRATION_STATUS
unset MIGRATION_SOURCE
unset MIGRATION_TARGET
```

---

## Future Enhancements

1. **Storage Abstractions:**
   - Support for additional storage backends
   - Pluggable storage architecture
   - Storage interface for different databases

2. **Data Migration Tools:**
   - Automated migration between storage types
   - Data validation during migration
   - Rollback capabilities

3. **Storage Monitoring:**
   - Storage performance metrics
   - Storage health checks
   - Capacity planning

4. **Backup and Recovery:**
   - Automated backup schedules
   - Point-in-time recovery
   - Data integrity verification

5. **Storage Optimization:**
   - Query optimization
   - Index management
   - Connection pooling
