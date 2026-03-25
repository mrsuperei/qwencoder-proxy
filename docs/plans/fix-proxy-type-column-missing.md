# Fix Plan: Missing `type` Column in `proxy_configs` Table

## Problem Summary

When visiting the proxy page on the dashboard, users encounter an HTTP 500 error:
```
Error: failed to list proxies: SQL logic error: no such column: type (1)
```

## Root Cause Analysis

### Historical Context

The database schema has evolved over time:

1. **Original Schema (Pre-migration)**: The `proxy_configs` table was created with only these columns:
   - `id, host, port, username, password, created_at`
   - No `type` column existed

2. **Current Schema**: The code now expects a `type` column:
   - The [`ListProxies()`](../internal/token/sqlite_store.go:1349) method queries: `SELECT id, type, host, port, username, password, created_at`
   - The [`ProxyConfig`](../internal/token/proxy_config.go:42) struct has a `Type` field

3. **Migration System**: A V2 migration ([`migrateToV2()`](../internal/token/sqlite_store.go:556)) exists to add the `type` column to existing databases.

### Why the Migration Didn't Run

The most likely scenario is that the database was created with the original schema (before the migration system was fully implemented), and the V2 migration has not been applied.

Possible reasons:
- The database was created before the migration system was added
- The migration failed silently or was interrupted
- The `schema_migrations` table wasn't properly initialized

## Fix Strategy

### Approach: Multi-layered Solution

We'll implement a three-layered approach to ensure the issue is resolved:

1. **Immediate Fix**: Manual SQL command to add the missing column
2. **Code Improvement**: Enhanced migration system with better error handling
3. **Defensive Coding**: Graceful handling of missing columns

### Fix Options

#### Option 1: Manual SQL Fix (Immediate)

Run this SQL command directly on the database:

```sql
ALTER TABLE proxy_configs ADD COLUMN type TEXT NOT NULL DEFAULT 'http';
```

**Pros:**
- Immediate fix
- No code changes required
- Quick resolution

**Cons:**
- Doesn't fix the root cause
- Needs to be repeated for each database instance
- Not scalable for production deployments

#### Option 2: Enhanced Migration System (Recommended)

Improve the migration system to handle edge cases better:

1. **Fix the migration logic** to handle databases created before the migration system
2. **Add better logging** to diagnose migration issues
3. **Add a V3 migration** that ensures the `type` column exists

**Pros:**
- Fixes the root cause
- Prevents future occurrences
- Scalable solution

**Cons:**
- Requires code changes
- Needs testing
- Slightly more complex

#### Option 3: Defensive Coding (Supplemental)

Add defensive code to handle the missing column gracefully:

1. **Check column existence** before querying
2. **Provide fallback behavior** when column is missing
3. **Log warnings** for missing columns

**Pros:**
- Makes the application more robust
- Provides better error messages
- Prevents crashes

**Cons:**
- Doesn't fix the underlying schema issue
- Adds complexity
- May mask real problems

## Recommended Implementation Plan

### Phase 1: Immediate Relief (Manual Fix)

1. **Backup the database** (important safety step)
2. **Run the SQL command** to add the missing column:
   ```sql
   ALTER TABLE proxy_configs ADD COLUMN type TEXT NOT NULL DEFAULT 'http';
   ```
3. **Verify the fix** by checking the proxy page

### Phase 2: Root Cause Fix (Code Changes)

#### 2.1 Improve Migration System

Modify [`sqlite_store.go`](../internal/token/sqlite_store.go) to:

1. **Handle NULL scan results** in the migration version check
2. **Add better error logging** for migration failures
3. **Add a V3 migration** that ensures the `type` column exists

#### 2.2 Add Defensive Coding

Modify the [`ListProxies()`](../internal/token/sqlite_store.go:1344) method to:

1. **Check if the `type` column exists** before querying
2. **Use a fallback query** if the column is missing
3. **Log a warning** if using fallback behavior

### Phase 3: Testing and Validation

1. **Test with old databases** (without `type` column)
2. **Test with new databases** (with `type` column)
3. **Verify migration runs correctly**
4. **Test the proxy page** functionality

## Detailed Code Changes

### Change 1: Improve Migration Version Check

File: [`internal/token/sqlite_store.go`](../internal/token/sqlite_store.go)

```go
// migrate runs pending database migrations
func (s *SQLiteStore) migrate() error {
	// Get current schema version
	var version int
	err := s.db.QueryRow("SELECT MAX(version) FROM schema_migrations").Scan(&version)
	if err != nil {
		// Check if the error is because the table doesn't exist
		if strings.Contains(err.Error(), "no such table") {
			// Table doesn't exist, this is a fresh installation
			version = 0
			s.logger.InfoLog("[SQLiteStore] schema_migrations table not found, treating as fresh installation")
		} else if strings.Contains(err.Error(), "no such column") || strings.Contains(err.Error(), "database is locked") {
			// Handle edge cases: table exists but is empty or has no version column
			s.logger.WarnLog("[SQLiteStore] Unable to read schema version, treating as version 0: %v", err)
			version = 0
		} else {
			return fmt.Errorf("failed to get schema version: %w", err)
		}
	}

	// ... rest of migration logic
}
```

### Change 2: Add V3 Migration

File: [`internal/token/sqlite_store.go`](../internal/token/sqlite_store.go)

```go
// migrate runs pending database migrations
func (s *SQLiteStore) migrate() error {
	// ... version check code ...

	// Run migrations in order
	migrations := []migration{
		{1, "Initial schema with all tables and indexes including project_id", func() error { return s.migrateToV1() }},
		{2, "Add type column to proxy_configs table", func() error { return s.migrateToV2() }},
		{3, "Ensure type column exists in proxy_configs table", func() error { return s.migrateToV3() }},
		// Future migrations here
	}

	// ... rest of migration logic
}

// migrateToV3 ensures the type column exists in proxy_configs table
func (s *SQLiteStore) migrateToV3() error {
	// Check if the type column already exists
	var columnName string
	err := s.db.QueryRow(`
		SELECT name FROM pragma_table_info('proxy_configs')
		WHERE name = 'type'
	`).Scan(&columnName)

	if err == nil {
		// Column already exists, skip migration
		s.logger.InfoLog("[SQLiteStore] type column already exists in proxy_configs table, skipping migration")
		return nil
	}

	// Add the type column
	if _, err := s.db.Exec(`
		ALTER TABLE proxy_configs ADD COLUMN type TEXT NOT NULL DEFAULT 'http'
	`); err != nil {
		return fmt.Errorf("failed to add type column to proxy_configs table: %w", err)
	}

	s.logger.InfoLog("[SQLiteStore] Added type column to proxy_configs table")
	return nil
}
```

### Change 3: Add Defensive Coding to ListProxies

File: [`internal/token/sqlite_store.go`](../internal/token/sqlite_store.go)

```go
// ListProxies retrieves all proxy configurations
func (s *SQLiteStore) ListProxies() ([]ProxyConfig, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Check if the type column exists
	var hasTypeColumn bool
	err := s.db.QueryRow(`
		SELECT COUNT(*) FROM pragma_table_info('proxy_configs')
		WHERE name = 'type'
	`).Scan(&hasTypeColumn)
	if err != nil {
		s.logger.WarnLog("[SQLiteStore] Failed to check for type column: %v", err)
		hasTypeColumn = false
	}

	var query string
	if hasTypeColumn {
		query = `
			SELECT id, type, host, port, username, password, created_at
			FROM proxy_configs
			ORDER BY created_at DESC
		`
	} else {
		s.logger.WarnLog("[SQLiteStore] type column missing from proxy_configs table, using fallback query")
		query = `
			SELECT id, 'http' as type, host, port, username, password, created_at
			FROM proxy_configs
			ORDER BY created_at DESC
		`
	}

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to list proxies: %w", err)
	}
	defer rows.Close()

	var proxies []ProxyConfig

	for rows.Next() {
		var proxy ProxyConfig
		var createdAt int64
		var username, password sql.NullString

		err := rows.Scan(
			&proxy.ID,
			&proxy.Type,
			&proxy.Host,
			&proxy.Port,
			&username,
			&password,
			&createdAt,
		)

		if err != nil {
			return nil, fmt.Errorf("failed to scan proxy: %w", err)
		}

		if username.Valid {
			proxy.Username = username.String
		}
		if password.Valid {
			proxy.Password = password.String
		}

		proxies = append(proxies, proxy)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating proxy rows: %w", err)
	}

	return proxies, nil
}
```

## Implementation Steps

1. **Backup the database** (`.credentials/tokens.db`)
2. **Apply the manual SQL fix** for immediate relief
3. **Implement the code changes** in the following order:
   - Improve migration version check
   - Add V3 migration
   - Add defensive coding to ListProxies
4. **Test the changes**:
   - Start the application
   - Check the proxy page
   - Verify the migration ran successfully
5. **Verify the fix** by checking the database schema

## Testing Checklist

- [ ] Backup created successfully
- [ ] Manual SQL fix applied successfully
- [ ] Proxy page loads without errors
- [ ] Proxies are listed correctly
- [ ] Migration V3 runs successfully
- [ ] Defensive coding works with old databases
- [ ] Defensive coding works with new databases
- [ ] No regression in other features

## Rollback Plan

If issues occur after implementing the code changes:

1. **Revert the code changes** using git
2. **Restore the database from backup**
3. **Apply only the manual SQL fix** as a temporary solution

## Related Files

- [`internal/token/sqlite_store.go`](../internal/token/sqlite_store.go) - Main database store implementation
- [`internal/token/proxy_config.go`](../internal/token/proxy_config.go) - Proxy configuration types
- [`internal/restapi/proxy_api.go`](../internal/restapi/proxy_api.go) - Proxy API endpoints

## Timeline

- **Phase 1 (Immediate)**: 5 minutes
- **Phase 2 (Code Changes)**: 30-60 minutes
- **Phase 3 (Testing)**: 15-30 minutes

## Success Criteria

- Proxy page loads without errors
- Proxies are listed correctly
- Migration system works reliably
- Application is more robust against schema mismatches
