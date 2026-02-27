# Testing Strategy for SQLite Migration - qwencoder-proxy

## Table of Contents

1. [Overview](#overview)
2. [Testing Philosophy](#testing-philosophy)
3. [Unit Testing Strategy](#unit-testing-strategy)
4. [Integration Testing Strategy](#integration-testing-strategy)
5. [Performance Testing Strategy](#performance-testing-strategy)
6. [Migration Testing Strategy](#migration-testing-strategy)
7. [Edge Case Testing](#edge-case-testing)
8. [Test Environment Setup](#test-environment-setup)
9. [Test Coverage Goals](#test-coverage-goals)
10. [Test Automation](#test-automation)
11. [Test Documentation](#test-documentation)
12. [Implementation Roadmap](#implementation-roadmap)

---

## Overview

This document defines the comprehensive testing strategy for SQLite migration in the qwencoder-proxy project. The strategy ensures data integrity, reliability, and performance throughout the migration from file-based to SQLite-based token storage.

### Related Documents

- [`01-current-architecture-analysis.md`](./01-current-architecture-analysis.md) - Current architecture analysis
- [`02-sqlite-database-schema-design.md`](./02-sqlite-database-schema-design.md) - SQLite database schema design
- [`03-database-initialization-strategy.md`](./03-database-initialization-strategy.md) - Database initialization strategy
- [`04-sqlite-storage-layer-design.md`](./04-sqlite-storage-layer-design.md) - SQLite storage layer design
- [`05-code-refactoring-plan.md`](./05-code-refactoring-plan.md) - Code refactoring plan
- [`06-file-to-sqlite-migration-strategy.md`](./06-file-to-sqlite-migration-strategy.md) - File-to-SQLite migration strategy

### Testing Objectives

| Objective | Description | Success Criteria |
|-----------|-------------|------------------|
| **Data Integrity** | Ensure no data loss or corruption during migration | 100% data integrity verified across all test scenarios |
| **Functional Equivalence** | SQLite storage provides identical functionality to file-based storage | All existing tests pass with SQLite backend |
| **Performance Parity** | SQLite performs at least as well as file-based storage | Performance benchmarks meet or exceed targets |
| **Concurrency Safety** | SQLite handles concurrent access without race conditions | No race conditions detected in stress tests |
| **Error Resilience** | Graceful handling of errors and edge cases | All error paths tested and validated |
| **Migration Reliability** | Migration process is safe, reversible, and idempotent | Migration succeeds in all scenarios with rollback capability |

### Testing Scope

The testing strategy covers:

1. **Unit Tests**: Isolated testing of individual components
2. **Integration Tests**: Testing of component interactions and workflows
3. **Performance Tests**: Benchmarking and load testing
4. **Migration Tests**: Validation of the migration process
5. **Edge Case Tests**: Testing of unusual and error conditions
6. **End-to-End Tests**: Full workflow validation

---

## Testing Philosophy

### Testing Pyramid

We follow the testing pyramid principle, with the majority of tests at the unit level:

```
                    ┌─────────────┐
                    │   E2E Tests  │  (5%)
                    │   (Manual)   │
                    └──────┬──────┘
                           │
                    ┌──────▼──────┐
                    │ Integration  │  (25%)
                    │    Tests     │
                    └──────┬──────┘
                           │
          ┌────────────▼────────────┐
          │     Unit Tests          │  (70%)
          │  (Automated, Fast)     │
          └─────────────────────────┘
```

**Rationale:**
- **Unit Tests (70%)**: Fast, isolated, provide quick feedback
- **Integration Tests (25%)**: Validate component interactions
- **E2E Tests (5%)**: Validate complete workflows, often manual or semi-automated

### Test-Driven Development Approach

We adopt a TDD approach for new SQLite components:

1. **Red**: Write a failing test for new functionality
2. **Green**: Write minimal code to make the test pass
3. **Refactor**: Improve code while keeping tests green

**Benefits:**
- Ensures test coverage from the start
- Drives better API design
- Provides living documentation
- Catches regressions early

### Quality Criteria and Standards

| Criteria | Standard | Measurement |
|----------|-----------|-------------|
| **Test Coverage** | ≥80% for new code, ≥70% for modified code | `go test -cover` |
| **Test Reliability** | No flaky tests | Consistent CI results |
| **Test Speed** | Unit tests <1s each, integration tests <10s | `go test -bench` |
| **Code Quality** | No linting errors | `golangci-lint` |
| **Documentation** | All public functions documented | `godoc` |

---

## Unit Testing Strategy

### Unit Test Files

| File | Purpose |
|------|---------|
| `internal/token/sqlite_store_test.go` | SQLite storage layer tests |
| `internal/token/migrator_test.go` | Migration utility tests |
| `internal/token/migration_test.go` | Migration process tests |
| `config/storage_config_test.go` | Storage configuration tests |

### Test Categories

#### 1. CRUD Operations Tests

```go
// internal/token/sqlite_store_test.go

package token

import (
    "testing"
    "time"
    
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestSQLiteStore_AddToken(t *testing.T) {
    // Setup
    store := setupTestStore(t, "test-provider")
    defer store.Close()
    
    // Test
    token := createTestToken(t)
    token.ID = "test-id-1"
    token.Email = "test@example.com"
    
    err := store.AddToken(token)
    require.NoError(t, err)
    
    // Verify
    retrieved, err := store.GetToken("test-id-1")
    require.NoError(t, err)
    assert.Equal(t, token.ID, retrieved.ID)
    assert.Equal(t, token.Email, retrieved.Email)
    assert.Equal(t, token.AccessToken, retrieved.AccessToken)
}

func TestSQLiteStore_AddToken_Duplicate(t *testing.T) {
    store := setupTestStore(t, "test-provider")
    defer store.Close()
    
    token := createTestToken(t)
    token.ID = "test-id-1"
    token.Email = "test@example.com"
    
    // Add first token
    err := store.AddToken(token)
    require.NoError(t, err)
    
    // Try to add duplicate token (same refresh_token)
    token2 := createTestToken(t)
    token2.ID = "test-id-2"
    token2.RefreshToken = token.RefreshToken // Same refresh token
    token2.Email = "test2@example.com"
    
    err = store.AddToken(token2)
    require.NoError(t, err) // Should update existing, not fail
    
    // Verify only one token exists with that refresh token
    tokens, err := store.Load()
    require.NoError(t, err)
    assert.Len(t, tokens, 1)
}

func TestSQLiteStore_GetToken_NotFound(t *testing.T) {
    store := setupTestStore(t, "test-provider")
    defer store.Close()
    
    _, err := store.GetToken("non-existent-id")
    assert.Error(t, err)
}

func TestSQLiteStore_UpdateToken(t *testing.T) {
    store := setupTestStore(t, "test-provider")
    defer store.Close()
    
    token := createTestToken(t)
    token.ID = "test-id-1"
    token.Healthy = true
    token.HealthScore = 1.0
    
    err := store.AddToken(token)
    require.NoError(t, err)
    
    // Update token
    err = store.UpdateToken("test-id-1", func(t *TokenMetadata) {
        t.Healthy = false
        t.HealthScore = 0.5
        t.ErrorCount = 1
    })
    require.NoError(t, err)
    
    // Verify update
    retrieved, err := store.GetToken("test-id-1")
    require.NoError(t, err)
    assert.False(t, retrieved.Healthy)
    assert.Equal(t, 0.5, retrieved.HealthScore)
    assert.Equal(t, 1, retrieved.ErrorCount)
}

func TestSQLiteStore_RemoveToken(t *testing.T) {
    store := setupTestStore(t, "test-provider")
    defer store.Close()
    
    token := createTestToken(t)
    token.ID = "test-id-1"
    
    err := store.AddToken(token)
    require.NoError(t, err)
    
    // Remove token
    err = store.RemoveToken("test-id-1")
    require.NoError(t, err)
    
    // Verify removal
    _, err = store.GetToken("test-id-1")
    assert.Error(t, err)
}

func TestSQLiteStore_Load_Empty(t *testing.T) {
    store := setupTestStore(t, "test-provider")
    defer store.Close()
    
    tokens, err := store.Load()
    require.NoError(t, err)
    assert.Empty(t, tokens)
}

func TestSQLiteStore_Load_MultipleTokens(t *testing.T) {
    store := setupTestStore(t, "test-provider")
    defer store.Close()
    
    // Add multiple tokens
    for i := 0; i < 10; i++ {
        token := createTestToken(t)
        token.ID = fmt.Sprintf("test-id-%d", i)
        token.Email = fmt.Sprintf("user%d@example.com", i)
        err := store.AddToken(token)
        require.NoError(t, err)
    }
    
    // Load and verify
    tokens, err := store.Load()
    require.NoError(t, err)
    assert.Len(t, tokens, 10)
}
```

#### 2. Transaction Tests

```go
func TestSQLiteStore_Transaction_Commit(t *testing.T) {
    store := setupTestStore(t, "test-provider")
    defer store.Close()
    
    tx, err := store.BeginTransaction()
    require.NoError(t, err)
    
    // Insert tokens within transaction
    for i := 0; i < 5; i++ {
        token := createTestToken(t)
        token.ID = fmt.Sprintf("tx-id-%d", i)
        _, err := tx.Exec(
            "INSERT INTO tokens (id, provider_id, access_token, refresh_token, token_type, expiry_date, email, healthy, health_score, last_used, created_at, error_count) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
            token.ID, "test-provider", token.AccessToken, token.RefreshToken, token.TokenType,
            token.ExpiryDate, token.Email, 1, token.HealthScore, token.LastUsed,
            token.CreatedAt, token.ErrorCount,
        )
        require.NoError(t, err)
    }
    
    // Commit transaction
    err = tx.Commit()
    require.NoError(t, err)
    
    // Verify all tokens were inserted
    tokens, err := store.Load()
    require.NoError(t, err)
    assert.Len(t, tokens, 5)
}

func TestSQLiteStore_Transaction_Rollback(t *testing.T) {
    store := setupTestStore(t, "test-provider")
    defer store.Close()
    
    // Add initial token
    token := createTestToken(t)
    token.ID = "initial-id"
    err := store.AddToken(token)
    require.NoError(t, err)
    
    // Start transaction and add more tokens
    tx, err := store.BeginTransaction()
    require.NoError(t, err)
    
    for i := 0; i < 5; i++ {
        token := createTestToken(t)
        token.ID = fmt.Sprintf("tx-id-%d", i)
        _, err := tx.Exec(
            "INSERT INTO tokens (id, provider_id, access_token, refresh_token, token_type, expiry_date, email, healthy, health_score, last_used, created_at, error_count) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
            token.ID, "test-provider", token.AccessToken, token.RefreshToken, token.TokenType,
            token.ExpiryDate, token.Email, 1, token.HealthScore, token.LastUsed,
            token.CreatedAt, token.ErrorCount,
        )
        require.NoError(t, err)
    }
    
    // Rollback transaction
    err = tx.Rollback()
    require.NoError(t, err)
    
    // Verify only initial token exists
    tokens, err := store.Load()
    require.NoError(t, err)
    assert.Len(t, tokens, 1)
    assert.Equal(t, "initial-id", tokens["initial-id"].ID)
}
```

#### 3. Error Handling Tests

```go
func TestSQLiteStore_Error_DatabaseClosed(t *testing.T) {
    store := setupTestStore(t, "test-provider")
    
    // Close the store
    err := store.Close()
    require.NoError(t, err)
    
    // Try to add token after close
    token := createTestToken(t)
    err = store.AddToken(token)
    assert.Error(t, err)
}

func TestSQLiteStore_Error_InvalidData(t *testing.T) {
    store := setupTestStore(t, "test-provider")
    defer store.Close()
    
    // Try to add token with missing required fields
    token := TokenMetadata{
        ID: "test-id",
        // Missing AccessToken
    }
    
    err := store.AddToken(token)
    assert.Error(t, err)
}

func TestSQLiteStore_Error_ConstraintViolation(t *testing.T) {
    store := setupTestStore(t, "test-provider")
    defer store.Close()
    
    token := createTestToken(t)
    token.ID = "test-id-1"
    
    // Add token
    err := store.AddToken(token)
    require.NoError(t, err)
    
    // Try to add another token with same ID (violates PRIMARY KEY)
    token2 := createTestToken(t)
    token2.ID = "test-id-1" // Same ID
    token2.Email = "different@example.com"
    
    err = store.AddToken(token2)
    assert.Error(t, err)
}
```

#### 4. Query Tests

```go
func TestSQLiteStore_GetValidTokens(t *testing.T) {
    store := setupTestStore(t, "test-provider")
    defer store.Close()
    
    now := time.Now()
    
    // Add valid tokens (healthy and not expired)
    validToken := createTestToken(t)
    validToken.ID = "valid-id"
    validToken.Healthy = true
    validToken.ExpiryDate = now.Add(time.Hour).UnixMilli()
    err := store.AddToken(validToken)
    require.NoError(t, err)
    
    // Add expired token
    expiredToken := createTestToken(t)
    expiredToken.ID = "expired-id"
    expiredToken.Healthy = true
    expiredToken.ExpiryDate = now.Add(-time.Hour).UnixMilli()
    err = store.AddToken(expiredToken)
    require.NoError(t, err)
    
    // Add unhealthy token
    unhealthyToken := createTestToken(t)
    unhealthyToken.ID = "unhealthy-id"
    unhealthyToken.Healthy = false
    unhealthyToken.ExpiryDate = now.Add(time.Hour).UnixMilli()
    err = store.AddToken(unhealthyToken)
    require.NoError(t, err)
    
    // Get valid tokens
    validTokens, err := store.GetValidTokens()
    require.NoError(t, err)
    assert.Len(t, validTokens, 1)
    assert.Equal(t, "valid-id", validTokens[0].ID)
}

func TestSQLiteStore_QueryByProvider(t *testing.T) {
    store := setupTestStore(t, "test-provider")
    defer store.Close()
    
    // Add tokens for different providers
    for _, providerID := range []string{"gemini", "qwen", "iflow"} {
        token := createTestToken(t)
        token.ID = fmt.Sprintf("%s-id", providerID)
        token.Email = fmt.Sprintf("%s@example.com", providerID)
        
        store.SetProviderID(providerID)
        err := store.AddToken(token)
        require.NoError(t, err)
    }
    
    // Query tokens for specific provider
    store.SetProviderID("gemini")
    geminiTokens, err := store.Load()
    require.NoError(t, err)
    assert.Len(t, geminiTokens, 1)
    assert.Equal(t, "gemini-id", geminiTokens["gemini-id"].ID)
}
```

### Test Helper Functions

```go
// internal/token/sqlite_store_test.go

func setupTestStore(t *testing.T, providerID string) *SQLiteStore {
    store, err := NewSQLiteStore(":memory:", providerID, &testLogger{t: t})
    require.NoError(t, err)
    return store
}

func createTestToken(t *testing.T) TokenMetadata {
    now := time.Now()
    return TokenMetadata{
        ID:          uuid.New().String(),
        AccessToken:   "test-access-token-" + uuid.New().String(),
        RefreshToken:  "test-refresh-token-" + uuid.New().String(),
        TokenType:     "Bearer",
        ExpiryDate:   now.Add(time.Hour).UnixMilli(),
        Email:        "test@example.com",
        Healthy:      true,
        HealthScore:  1.0,
        LastUsed:     now.UnixMilli(),
        CreatedAt:    now.UnixMilli(),
        ErrorCount:   0,
    }
}

type testLogger struct {
    t *testing.T
}

func (l *testLogger) InfoLog(format string, args ...interface{}) {
    l.t.Logf("[INFO] "+format, args...)
}

func (l *testLogger) DebugLog(format string, args ...interface{}) {
    l.t.Logf("[DEBUG] "+format, args...)
}

func (l *testLogger) WarnLog(format string, args ...interface{}) {
    l.t.Logf("[WARN] "+format, args...)
}

func (l *testLogger) ErrorLog(format string, args ...interface{}) {
    l.t.Logf("[ERROR] "+format, args...)
}
```

---

## Integration Testing Strategy

### Integration Test Files

| File | Purpose |
|------|---------|
| `internal/token/integration_test.go` | Integration tests for token storage |
| `restapi/integration_test.go` | API integration tests with SQLite |
| `proxy/integration_test.go` | Proxy handler integration tests |

### Test Scenarios

#### 1. Full OAuth Flow Integration Test

```go
// internal/token/integration_test.go

func TestIntegration_OAuthFlowWithSQLite(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping integration test in short mode")
    }
    
    // Setup test server with SQLite storage
    config := createTestConfig(t)
    config.Storage.Backend = "sqlite"
    config.Storage.Path = ":memory:"
    
    server := setupTestServer(t, config)
    defer server.Close()
    
    // Simulate OAuth flow
    // 1. Start OAuth
    authURL, state := startOAuth(t, server, "gemini")
    require.NotEmpty(t, authURL)
    require.NotEmpty(t, state)
    
    // 2. Simulate callback
    tokenResponse := simulateCallback(t, server, "gemini", state, "test-auth-code")
    require.NotEmpty(t, tokenResponse.AccessToken)
    
    // 3. Verify token was saved to SQLite
    store := server.GetTokenStore("gemini")
    tokens, err := store.Load()
    require.NoError(t, err)
    assert.Len(t, tokens, 1)
    
    // 4. Verify token can be retrieved via API
    retrievedToken := getTokenViaAPI(t, server, "gemini")
    assert.Equal(t, tokenResponse.AccessToken, retrievedToken.AccessToken)
}
```

#### 2. Token Refresh Integration Test

```go
func TestIntegration_TokenRefreshWithSQLite(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping integration test in short mode")
    }
    
    config := createTestConfig(t)
    config.Storage.Backend = "sqlite"
    config.Storage.Path = ":memory:"
    
    server := setupTestServer(t, config)
    defer server.Close()
    
    // Add expiring token
    store := server.GetTokenStore("gemini")
    token := createTestToken(t)
    token.ExpiryDate = time.Now().Add(-10 * time.Minute).UnixMilli()
    err := store.AddToken(token)
    require.NoError(t, err)
    
    // Trigger token refresh
    refreshedToken, err := server.RefreshToken("gemini", token.ID)
    require.NoError(t, err)
    assert.NotEmpty(t, refreshedToken.AccessToken)
    assert.NotEqual(t, token.AccessToken, refreshedToken.AccessToken)
    
    // Verify token was updated in SQLite
    retrieved, err := store.GetToken(token.ID)
    require.NoError(t, err)
    assert.Equal(t, refreshedToken.AccessToken, retrieved.AccessToken)
    assert.Greater(t, retrieved.ExpiryDate, token.ExpiryDate)
}
```

#### 3. Concurrent Access Integration Test

```go
func TestIntegration_ConcurrentTokenAccess(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping integration test in short mode")
    }
    
    config := createTestConfig(t)
    config.Storage.Backend = "sqlite"
    config.Storage.Path = ":memory:"
    
    server := setupTestServer(t, config)
    defer server.Close()
    
    const numGoroutines = 50
    const numOperations = 10
    
    var wg sync.WaitGroup
    errors := make(chan error, numGoroutines*numOperations)
    
    // Start multiple goroutines accessing tokens
    for i := 0; i < numGoroutines; i++ {
        wg.Add(1)
        go func(goroutineID int) {
            defer wg.Done()
            
            for j := 0; j < numOperations; j++ {
                token := createTestToken(t)
                token.ID = fmt.Sprintf("token-%d-%d", goroutineID, j)
                token.Email = fmt.Sprintf("user%d@example.com", goroutineID)
                
                store := server.GetTokenStore("gemini")
                err := store.AddToken(token)
                if err != nil {
                    errors <- err
                    return
                }
                
                // Try to read token
                _, err = store.GetToken(token.ID)
                if err != nil {
                    errors <- err
                    return
                }
            }
        }(i)
    }
    
    wg.Wait()
    close(errors)
    
    // Check for errors
    for err := range errors {
        t.Errorf("Concurrent access error: %v", err)
    }
    
    // Verify all tokens were added
    store := server.GetTokenStore("gemini")
    tokens, err := store.Load()
    require.NoError(t, err)
    assert.Len(t, tokens, numGoroutines*numOperations)
}
```

#### 4. Migration Integration Test

```go
func TestIntegration_FileToSQLiteMigration(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping integration test in short mode")
    }
    
    // Setup file store with test data
    fileStore := setupFileStore(t, "test-provider")
    
    const numTokens = 100
    var expectedTokens []TokenMetadata
    for i := 0; i < numTokens; i++ {
        token := createTestToken(t)
        token.ID = fmt.Sprintf("file-token-%d", i)
        token.Email = fmt.Sprintf("user%d@example.com", i)
        
        err := fileStore.AddToken(token)
        require.NoError(t, err)
        expectedTokens = append(expectedTokens, token)
    }
    
    // Create SQLite store
    dbStore := setupTestStore(t, "test-provider")
    
    // Perform migration
    migrator := NewMigrator(fileStore, dbStore, &testLogger{t: t})
    err := migrator.Migrate()
    require.NoError(t, err)
    
    // Verify all tokens were migrated
    dbTokens, err := dbStore.Load()
    require.NoError(t, err)
    assert.Len(t, dbTokens, numTokens)
    
    // Verify data integrity
    for _, expected := range expectedTokens {
        actual, exists := dbTokens[expected.ID]
        assert.True(t, exists, "Token %s not found in SQLite", expected.ID)
        if exists {
            assert.Equal(t, expected.Email, actual.Email)
            assert.Equal(t, expected.AccessToken, actual.AccessToken)
            assert.Equal(t, expected.RefreshToken, actual.RefreshToken)
        }
    }
}
```

---

## Performance Testing Strategy

### Benchmark Files

| File | Purpose |
|------|---------|
| `internal/token/benchmark_test.go` | Storage layer benchmarks |
| `internal/token/migration_benchmark_test.go` | Migration benchmarks |

### Benchmark Scenarios

#### 1. Storage Operation Benchmarks

```go
// internal/token/benchmark_test.go

func BenchmarkSQLiteStore_Load_100(b *testing.B) {
    store := setupBenchmarkStore(b, 100)
    defer store.Close()
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        _, err := store.Load()
        if err != nil {
            b.Fatal(err)
        }
    }
}

func BenchmarkSQLiteStore_Load_1000(b *testing.B) {
    store := setupBenchmarkStore(b, 1000)
    defer store.Close()
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        _, err := store.Load()
        if err != nil {
            b.Fatal(err)
        }
    }
}

func BenchmarkSQLiteStore_Load_10000(b *testing.B) {
    store := setupBenchmarkStore(b, 10000)
    defer store.Close()
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        _, err := store.Load()
        if err != nil {
            b.Fatal(err)
        }
    }
}

func BenchmarkSQLiteStore_AddToken(b *testing.B) {
    store := setupBenchmarkStore(b, 1000)
    defer store.Close()
    
    token := createTestToken(b)
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        token.ID = uuid.New().String()
        err := store.AddToken(token)
        if err != nil {
            b.Fatal(err)
        }
    }
}

func BenchmarkSQLiteStore_GetToken(b *testing.B) {
    store := setupBenchmarkStore(b, 1000)
    defer store.Close()
    
    token := createTestToken(b)
    store.AddToken(token)
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        _, err := store.GetToken(token.ID)
        if err != nil {
            b.Fatal(err)
        }
    }
}

func BenchmarkSQLiteStore_UpdateToken(b *testing.B) {
    store := setupBenchmarkStore(b, 1000)
    defer store.Close()
    
    token := createTestToken(b)
    store.AddToken(token)
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        err := store.UpdateToken(token.ID, func(t *TokenMetadata) {
            t.LastUsed = time.Now().UnixMilli()
        })
        if err != nil {
            b.Fatal(err)
        }
    }
}

func BenchmarkSQLiteStore_GetValidTokens(b *testing.B) {
    store := setupBenchmarkStore(b, 1000)
    defer store.Close()
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        _, err := store.GetValidTokens()
        if err != nil {
            b.Fatal(err)
        }
    }
}
```

#### 2. Concurrency Benchmarks

```go
func BenchmarkSQLiteStore_ConcurrentReads_50(b *testing.B) {
    store := setupBenchmarkStore(b, 1000)
    defer store.Close()
    
    b.ResetTimer()
    b.RunParallel(func(pb *testing.PB) {
        for pb.Next() {
            _, err := store.Load()
            if err != nil {
                b.Fatal(err)
            }
        }
    })
}

func BenchmarkSQLiteStore_ConcurrentWrites_50(b *testing.B) {
    store := setupBenchmarkStore(b, 1000)
    defer store.Close()
    
    b.ResetTimer()
    b.RunParallel(func(pb *testing.PB) {
        for pb.Next() {
            token := createTestToken(b)
            token.ID = uuid.New().String()
            err := store.AddToken(token)
            if err != nil {
                b.Fatal(err)
            }
        }
    })
}

func BenchmarkSQLiteStore_ConcurrentReadWrite(b *testing.B) {
    store := setupBenchmarkStore(b, 1000)
    defer store.Close()
    
    token := createTestToken(b)
    store.AddToken(token)
    
    b.ResetTimer()
    b.RunParallel(func(pb *testing.PB) {
        for pb.Next() {
            // Mix of reads and writes
            if pb.Next() % 2 == 0 {
                _, err := store.Load()
                if err != nil {
                    b.Fatal(err)
                }
            } else {
                err := store.UpdateToken(token.ID, func(t *TokenMetadata) {
                    t.LastUsed = time.Now().UnixMilli()
                })
                if err != nil {
                    b.Fatal(err)
                }
            }
        }
    })
}
```

#### 3. Migration Benchmarks

```go
// internal/token/migration_benchmark_test.go

func BenchmarkMigration_FileToSQLite_100(b *testing.B) {
    if testing.Short() {
        b.Skip("Skipping benchmark in short mode")
    }
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        fileStore := setupFileStore(b, "test-provider")
        dbStore := setupTestStore(b, "test-provider")
        
        // Add 100 tokens to file store
        for j := 0; j < 100; j++ {
            token := createTestToken(b)
            token.ID = fmt.Sprintf("token-%d-%d", i, j)
            fileStore.AddToken(token)
        }
        
        // Migrate
        migrator := NewMigrator(fileStore, dbStore, &testLogger{b: b})
        err := migrator.Migrate()
        if err != nil {
            b.Fatal(err)
        }
        
        // Cleanup
        fileStore.Close()
        dbStore.Close()
    }
}

func BenchmarkMigration_FileToSQLite_1000(b *testing.B) {
    if testing.Short() {
        b.Skip("Skipping benchmark in short mode")
    }
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        fileStore := setupFileStore(b, "test-provider")
        dbStore := setupTestStore(b, "test-provider")
        
        // Add 1000 tokens to file store
        for j := 0; j < 1000; j++ {
            token := createTestToken(b)
            token.ID = fmt.Sprintf("token-%d-%d", i, j)
            fileStore.AddToken(token)
        }
        
        // Migrate
        migrator := NewMigrator(fileStore, dbStore, &testLogger{b: b})
        err := migrator.Migrate()
        if err != nil {
            b.Fatal(err)
        }
        
        // Cleanup
        fileStore.Close()
        dbStore.Close()
    }
}
```

### Performance Targets

| Operation | Target | Measurement |
|-----------|---------|-------------|
| Load 100 tokens | <10ms | `BenchmarkSQLiteStore_Load_100` |
| Load 1000 tokens | <50ms | `BenchmarkSQLiteStore_Load_1000` |
| Load 10000 tokens | <500ms | `BenchmarkSQLiteStore_Load_10000` |
| Add token | <1ms | `BenchmarkSQLiteStore_AddToken` |
| Get token | <0.5ms | `BenchmarkSQLiteStore_GetToken` |
| Update token | <1ms | `BenchmarkSQLiteStore_UpdateToken` |
| Get valid tokens | <5ms | `BenchmarkSQLiteStore_GetValidTokens` |
| Migrate 100 tokens | <100ms | `BenchmarkMigration_FileToSQLite_100` |
| Migrate 1000 tokens | <1s | `BenchmarkMigration_FileToSQLite_1000` |

### Benchmark Helper Functions

```go
// internal/token/benchmark_test.go

func setupBenchmarkStore(b *testing.B, numTokens int) *SQLiteStore {
    store, err := NewSQLiteStore(":memory:", "test-provider", &testLogger{b: b})
    if err != nil {
        b.Fatal(err)
    }
    
    // Pre-populate store
    for i := 0; i < numTokens; i++ {
        token := createTestToken(b)
        token.ID = fmt.Sprintf("bench-token-%d", i)
        token.Email = fmt.Sprintf("user%d@example.com", i)
        err := store.AddToken(token)
        if err != nil {
            b.Fatal(err)
        }
    }
    
    return store
}

func createTestToken(b testing.TB) TokenMetadata {
    now := time.Now()
    return TokenMetadata{
        ID:          uuid.New().String(),
        AccessToken:   "test-access-token-" + uuid.New().String(),
        RefreshToken:  "test-refresh-token-" + uuid.New().String(),
        TokenType:     "Bearer",
        ExpiryDate:   now.Add(time.Hour).UnixMilli(),
        Email:        "test@example.com",
        Healthy:      true,
        HealthScore:  1.0,
        LastUsed:     now.UnixMilli(),
        CreatedAt:    now.UnixMilli(),
        ErrorCount:   0,
    }
}
```

---

## Migration Testing Strategy

### Migration Test Categories

1. **Basic Migration Tests**: Simple migration scenarios
2. **Data Integrity Tests**: Verify data is preserved correctly
3. **Rollback Tests**: Verify rollback functionality
4. **Large Dataset Tests**: Test with large token counts
5. **Concurrent Migration Tests**: Test migration under concurrent access

### Basic Migration Tests

```go
// internal/token/migration_test.go

func TestMigration_EmptyFileStore(t *testing.T) {
    fileStore := setupFileStore(t, "test-provider")
    dbStore := setupTestStore(t, "test-provider")
    
    migrator := NewMigrator(fileStore, dbStore, &testLogger{t: t})
    err := migrator.Migrate()
    require.NoError(t, err)
    
    // Verify SQLite store is empty
    tokens, err := dbStore.Load()
    require.NoError(t, err)
    assert.Empty(t, tokens)
}

func TestMigration_SingleToken(t *testing.T) {
    fileStore := setupFileStore(t, "test-provider")
    dbStore := setupTestStore(t, "test-provider")
    
    // Add single token to file store
    token := createTestToken(t)
    token.ID = "test-id-1"
    token.Email = "test@example.com"
    err := fileStore.AddToken(token)
    require.NoError(t, err)
    
    // Migrate
    migrator := NewMigrator(fileStore, dbStore, &testLogger{t: t})
    err = migrator.Migrate()
    require.NoError(t, err)
    
    // Verify token was migrated
    tokens, err := dbStore.Load()
    require.NoError(t, err)
    assert.Len(t, tokens, 1)
    
    migratedToken := tokens["test-id-1"]
    assert.Equal(t, token.ID, migratedToken.ID)
    assert.Equal(t, token.Email, migratedToken.Email)
    assert.Equal(t, token.AccessToken, migratedToken.AccessToken)
    assert.Equal(t, token.RefreshToken, migratedToken.RefreshToken)
}

func TestMigration_MultipleTokens(t *testing.T) {
    fileStore := setupFileStore(t, "test-provider")
    dbStore := setupTestStore(t, "test-provider")
    
    const numTokens = 50
    var expectedTokens []TokenMetadata
    for i := 0; i < numTokens; i++ {
        token := createTestToken(t)
        token.ID = fmt.Sprintf("test-id-%d", i)
        token.Email = fmt.Sprintf("user%d@example.com", i)
        
        err := fileStore.AddToken(token)
        require.NoError(t, err)
        expectedTokens = append(expectedTokens, token)
    }
    
    // Migrate
    migrator := NewMigrator(fileStore, dbStore, &testLogger{t: t})
    err := migrator.Migrate()
    require.NoError(t, err)
    
    // Verify all tokens were migrated
    tokens, err := dbStore.Load()
    require.NoError(t, err)
    assert.Len(t, tokens, numTokens)
    
    // Verify data integrity
    for _, expected := range expectedTokens {
        actual, exists := tokens[expected.ID]
        assert.True(t, exists, "Token %s not found", expected.ID)
        if exists {
            assert.Equal(t, expected.Email, actual.Email)
            assert.Equal(t, expected.AccessToken, actual.AccessToken)
            assert.Equal(t, expected.RefreshToken, actual.RefreshToken)
        }
    }
}
```

### Data Integrity Tests

```go
func TestMigration_DataIntegrity_HashComparison(t *testing.T) {
    fileStore := setupFileStore(t, "test-provider")
    dbStore := setupTestStore(t, "test-provider")
    
    // Add tokens to file store
    const numTokens = 100
    for i := 0; i < numTokens; i++ {
        token := createTestToken(t)
        token.ID = fmt.Sprintf("test-id-%d", i)
        token.Email = fmt.Sprintf("user%d@example.com", i)
        err := fileStore.AddToken(token)
        require.NoError(t, err)
    }
    
    // Calculate hash of file store data
    fileTokens, err := fileStore.Load()
    require.NoError(t, err)
    fileHash := calculateTokenHash(fileTokens)
    
    // Migrate
    migrator := NewMigrator(fileStore, dbStore, &testLogger{t: t})
    err = migrator.Migrate()
    require.NoError(t, err)
    
    // Calculate hash of migrated data
    dbTokens, err := dbStore.Load()
    require.NoError(t, err)
    dbHash := calculateTokenHash(dbTokens)
    
    // Verify hashes match
    assert.Equal(t, fileHash, dbHash, "Data integrity check failed")
}

func TestMigration_DataIntegrity_Timestamps(t *testing.T) {
    fileStore := setupFileStore(t, "test-provider")
    dbStore := setupTestStore(t, "test-provider")
    
    // Add token with specific timestamps
    token := createTestToken(t)
    token.ID = "test-id-1"
    token.CreatedAt = 1700000000000 // Fixed timestamp
    token.LastUsed = 1700000001000 // Fixed timestamp
    token.ExpiryDate = 1700000002000 // Fixed timestamp
    
    err := fileStore.AddToken(token)
    require.NoError(t, err)
    
    // Migrate
    migrator := NewMigrator(fileStore, dbStore, &testLogger{t: t})
    err = migrator.Migrate()
    require.NoError(t, err)
    
    // Verify timestamps are preserved
    dbToken, err := dbStore.GetToken("test-id-1")
    require.NoError(t, err)
    assert.Equal(t, token.CreatedAt, dbToken.CreatedAt)
    assert.Equal(t, token.LastUsed, dbToken.LastUsed)
    assert.Equal(t, token.ExpiryDate, dbToken.ExpiryDate)
}

func TestMigration_DataIntegrity_HealthScores(t *testing.T) {
    fileStore := setupFileStore(t, "test-provider")
    dbStore := setupTestStore(t, "test-provider")
    
    // Add tokens with various health scores
    healthScores := []float64{0.0, 0.25, 0.5, 0.75, 1.0}
    for i, score := range healthScores {
        token := createTestToken(t)
        token.ID = fmt.Sprintf("test-id-%d", i)
        token.Healthy = score > 0.5
        token.HealthScore = score
        
        err := fileStore.AddToken(token)
        require.NoError(t, err)
    }
    
    // Migrate
    migrator := NewMigrator(fileStore, dbStore, &testLogger{t: t})
    err := migrator.Migrate()
    require.NoError(t, err)
    
    // Verify health scores are preserved
    dbTokens, err := dbStore.Load()
    require.NoError(t, err)
    assert.Len(t, dbTokens, len(healthScores))
    
    for i, score := range healthScores {
        tokenID := fmt.Sprintf("test-id-%d", i)
        dbToken := dbTokens[tokenID]
        assert.Equal(t, score > 0.5, dbToken.Healthy)
        assert.Equal(t, score, dbToken.HealthScore)
    }
}
```

### Rollback Tests

```go
func TestMigration_Rollback_BackupCreation(t *testing.T) {
    fileStore := setupFileStore(t, "test-provider")
    dbStore := setupTestStore(t, "test-provider")
    
    // Add tokens to file store
    const numTokens = 10
    for i := 0; i < numTokens; i++ {
        token := createTestToken(t)
        token.ID = fmt.Sprintf("test-id-%d", i)
        token.Email = fmt.Sprintf("user%d@example.com", i)
        err := fileStore.AddToken(token)
        require.NoError(t, err)
    }
    
    // Create backup
    migrator := NewMigrator(fileStore, dbStore, &testLogger{t: t})
    backupPath, err := migrator.CreateBackup()
    require.NoError(t, err)
    require.NotEmpty(t, backupPath)
    
    // Verify backup file exists
    _, err = os.Stat(backupPath)
    require.NoError(t, err)
    
    // Verify backup contains original data
    backupStore, err := NewFileStore(backupPath, "test-provider", &testLogger{t: t})
    require.NoError(t, err)
    backupTokens, err := backupStore.Load()
    require.NoError(t, err)
    assert.Len(t, backupTokens, numTokens)
}

func TestMigration_Rollback_RestoreFromBackup(t *testing.T) {
    fileStore := setupFileStore(t, "test-provider")
    dbStore := setupTestStore(t, "test-provider")
    
    // Add tokens to file store
    const numTokens = 10
    var expectedTokens []TokenMetadata
    for i := 0; i < numTokens; i++ {
        token := createTestToken(t)
        token.ID = fmt.Sprintf("test-id-%d", i)
        token.Email = fmt.Sprintf("user%d@example.com", i)
        
        err := fileStore.AddToken(token)
        require.NoError(t, err)
        expectedTokens = append(expectedTokens, token)
    }
    
    // Calculate original hash
    originalTokens, err := fileStore.Load()
    require.NoError(t, err)
    originalHash := calculateTokenHash(originalTokens)
    
    // Migrate
    migrator := NewMigrator(fileStore, dbStore, &testLogger{t: t})
    backupPath, err := migrator.CreateBackup()
    require.NoError(t, err)
    
    err = migrator.Migrate()
    require.NoError(t, err)
    
    // Restore from backup
    err = migrator.RestoreFromBackup(backupPath)
    require.NoError(t, err)
    
    // Verify restored data matches original
    restoredTokens, err := fileStore.Load()
    require.NoError(t, err)
    restoredHash := calculateTokenHash(restoredTokens)
    
    assert.Equal(t, originalHash, restoredHash, "File store should be restored from backup")
}
```

### Large Dataset Migration Testing

```go
// internal/token/migration_large_test.go

package token

import (
    "testing"
    "time"
    
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestMigrationLarge_10000Tokens(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping large dataset test in short mode")
    }
    
    // Setup file store with 10,000 tokens
    fileStore, _ := setupFileStore(t, "test-provider")
    
    const numTokens = 10000
    t.Logf("Creating %d test tokens...", numTokens)
    
    start := time.Now()
    var expectedTokens []TokenMetadata
    for i := 0; i < numTokens; i++ {
        token := createTestToken(t)
        token.ID = fmt.Sprintf("large-test-id-%d", i)
        token.Email = fmt.Sprintf("user%d@example.com", i)
        
        err := fileStore.AddToken(token)
        require.NoError(t, err)
        expectedTokens = append(expectedTokens, token)
        
        if (i+1)%1000 == 0 {
            t.Logf("Created %d/%d tokens", i+1, numTokens)
        }
    }
    createDuration := time.Since(start)
    t.Logf("Created %d tokens in %v (%.2f tokens/sec)", numTokens, createDuration, float64(numTokens)/createDuration.Seconds())
    
    // Create SQLite store
    dbStore := setupTestStore(t, "test-provider")
    
    // Migrate
    t.Logf("Starting migration of %d tokens...", numTokens)
    start = time.Now()
    
    migrator := NewMigrator(fileStore, dbStore, &testLogger{t: t})
    err := migrator.Migrate()
    require.NoError(t, err)
    
    migrationDuration := time.Since(start)
    t.Logf("Migrated %d tokens in %v (%.2f tokens/sec)", numTokens, migrationDuration, float64(numTokens)/migrationDuration.Seconds())
    
    // Verify all tokens were migrated
    dbTokens, err := dbStore.Load()
    require.NoError(t, err)
    assert.Len(t, dbTokens, numTokens)
    
    // Verify data integrity for sample of tokens
    sampleSize := 100
    for i := 0; i < sampleSize; i++ {
        expected := expectedTokens[i]
        actual, exists := dbTokens[expected.ID]
        assert.True(t, exists, "Token %s not found in SQLite", expected.ID)
        if exists {
            assert.Equal(t, expected.Email, actual.Email)
            assert.Equal(t, expected.AccessToken, actual.AccessToken)
            assert.Equal(t, expected.RefreshToken, actual.RefreshToken)
        }
    }
}

func TestMigrationLarge_MemoryUsage(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping memory usage test in short mode")
    }
    
    var m runtime.MemStats
    
    // Get baseline memory
    runtime.ReadMemStats(&m)
    baselineAlloc := m.Alloc
    
    // Setup file store with 10,000 tokens
    fileStore, _ := setupFileStore(t, "test-provider")
    
    const numTokens = 10000
    for i := 0; i < numTokens; i++ {
        token := createTestToken(t)
        token.ID = fmt.Sprintf("mem-test-id-%d", i)
        token.Email = fmt.Sprintf("user%d@example.com", i)
        err := fileStore.AddToken(token)
        require.NoError(t, err)
    }
    
    // Get memory after file store creation
    runtime.ReadMemStats(&m)
    fileStoreAlloc := m.Alloc
    
    // Create SQLite store and migrate
    dbStore := setupTestStore(t, "test-provider")
    
    migrator := NewMigrator(fileStore, dbStore, &testLogger{t: t})
    err := migrator.Migrate()
    require.NoError(t, err)
    
    // Get memory after migration
    runtime.ReadMemStats(&m)
    migrationAlloc := m.Alloc
    
    // Log memory usage
    t.Logf("Baseline memory: %d bytes", baselineAlloc)
    t.Logf("After file store: %d bytes (%.2f MB)", fileStoreAlloc, float64(fileStoreAlloc)/1024/1024)
    t.Logf("After migration: %d bytes (%.2f MB)", migrationAlloc, float64(migrationAlloc-baselineAlloc)/1024/1024)
    t.Logf("Migration memory overhead: %d bytes (%.2f MB)", migrationAlloc-baselineAlloc, float64(migrationAlloc-baselineAlloc)/1024/1024)
}
```

### Helper Functions

```go
// internal/token/migration_test.go

func setupFileStore(t *testing.T, providerID string) *FileStore {
    tmpDir := t.TempDir()
    store, err := NewFileStore(tmpDir, providerID, &testLogger{t: t})
    require.NoError(t, err)
    return store
}

func calculateTokenHash(tokens map[string]TokenMetadata) string {
    // Create a deterministic hash of token data
    var tokenIDs []string
    for id := range tokens {
        tokenIDs = append(tokenIDs, id)
    }
    sort.Strings(tokenIDs)
    
    h := sha256.New()
    for _, id := range tokenIDs {
        token := tokens[id]
        data := fmt.Sprintf("%s|%s|%s|%s|%d|%d|%d|%d|%d",
            token.ID, token.Email, token.AccessToken, token.RefreshToken,
            token.ExpiryDate, token.CreatedAt, token.LastUsed,
            token.ErrorCount, int(token.Healthy),
        )
        h.Write([]byte(data))
    }
    return hex.EncodeToString(h.Sum(nil))
}
```

---

## Edge Case Testing

### Edge Case Categories

1. **Empty State Tests**: Testing with empty storage
2. **Boundary Value Tests**: Testing with boundary values
3. **Special Character Tests**: Testing with special characters in data
4. **Concurrent Edge Cases**: Testing concurrent operations edge cases
5. **Error Path Tests**: Testing error handling paths

### Empty State Tests

```go
func TestEdgeCase_EmptyStore_Load(t *testing.T) {
    store := setupTestStore(t, "test-provider")
    defer store.Close()
    
    tokens, err := store.Load()
    require.NoError(t, err)
    assert.Empty(t, tokens)
}

func TestEdgeCase_EmptyStore_GetValidTokens(t *testing.T) {
    store := setupTestStore(t, "test-provider")
    defer store.Close()
    
    tokens, err := store.GetValidTokens()
    require.NoError(t, err)
    assert.Empty(t, tokens)
}

func TestEdgeCase_EmptyStore_RemoveToken(t *testing.T) {
    store := setupTestStore(t, "test-provider")
    defer store.Close()
    
    err := store.RemoveToken("non-existent-id")
    assert.Error(t, err)
}
```

### Boundary Value Tests

```go
func TestEdgeCase_Boundary_ExpiryDate_Zero(t *testing.T) {
    store := setupTestStore(t, "test-provider")
    defer store.Close()
    
    token := createTestToken(t)
    token.ExpiryDate = 0 // Unix epoch
    
    err := store.AddToken(token)
    require.NoError(t, err)
    
    // Verify token is retrieved
    retrieved, err := store.GetToken(token.ID)
    require.NoError(t, err)
    assert.Equal(t, int64(0), retrieved.ExpiryDate)
}

func TestEdgeCase_Boundary_ExpiryDate_Max(t *testing.T) {
    store := setupTestStore(t, "test-provider")
    defer store.Close()
    
    token := createTestToken(t)
    token.ExpiryDate = 9223372036854775807 // Max int64
    
    err := store.AddToken(token)
    require.NoError(t, err)
    
    retrieved, err := store.GetToken(token.ID)
    require.NoError(t, err)
    assert.Equal(t, int64(9223372036854775807), retrieved.ExpiryDate)
}

func TestEdgeCase_Boundary_HealthScore_Min(t *testing.T) {
    store := setupTestStore(t, "test-provider")
    defer store.Close()
    
    token := createTestToken(t)
    token.HealthScore = 0.0
    
    err := store.AddToken(token)
    require.NoError(t, err)
    
    retrieved, err := store.GetToken(token.ID)
    require.NoError(t, err)
    assert.Equal(t, 0.0, retrieved.HealthScore)
}

func TestEdgeCase_Boundary_HealthScore_Max(t *testing.T) {
    store := setupTestStore(t, "test-provider")
    defer store.Close()
    
    token := createTestToken(t)
    token.HealthScore = 1.0
    
    err := store.AddToken(token)
    require.NoError(t, err)
    
    retrieved, err := store.GetToken(token.ID)
    require.NoError(t, err)
    assert.Equal(t, 1.0, retrieved.HealthScore)
}

func TestEdgeCase_Boundary_ErrorCount_Max(t *testing.T) {
    store := setupTestStore(t, "test-provider")
    defer store.Close()
    
    token := createTestToken(t)
    token.ErrorCount = 2147483647 // Max int32
    
    err := store.AddToken(token)
    require.NoError(t, err)
    
    retrieved, err := store.GetToken(token.ID)
    require.NoError(t, err)
    assert.Equal(t, 2147483647, retrieved.ErrorCount)
}
```

### Special Character Tests

```go
func TestEdgeCase_SpecialCharacters_Email(t *testing.T) {
    store := setupTestStore(t, "test-provider")
    defer store.Close()
    
    specialEmails := []string{
        "user+tag@example.com",
        "user.name@example.com",
        "user_name@example.com",
        "user-name@example.com",
        "user@example.co.uk",
        "user@sub.domain.example.com",
    }
    
    for i, email := range specialEmails {
        token := createTestToken(t)
        token.ID = fmt.Sprintf("special-email-%d", i)
        token.Email = email
        
        err := store.AddToken(token)
        require.NoError(t, err)
        
        retrieved, err := store.GetToken(token.ID)
        require.NoError(t, err)
        assert.Equal(t, email, retrieved.Email)
    }
}

func TestEdgeCase_SpecialCharacters_AccessToken(t *testing.T) {
    store := setupTestStore(t, "test-provider")
    defer store.Close()
    
    specialTokens := []string{
        "ya29.a0AfH6SMB...very_long_token_with_many_characters",
        "token-with-dashes_and_underscores",
        "token.with.dots",
        "token/with/slashes",
        "token\\with\\backslashes",
        "token\"with\"quotes",
        "token'with'apostrophes",
    }
    
    for i, accessToken := range specialTokens {
        token := createTestToken(t)
        token.ID = fmt.Sprintf("special-token-%d", i)
        token.AccessToken = accessToken
        
        err := store.AddToken(token)
        require.NoError(t, err)
        
        retrieved, err := store.GetToken(token.ID)
        require.NoError(t, err)
        assert.Equal(t, accessToken, retrieved.AccessToken)
    }
}

func TestEdgeCase_UnicodeCharacters(t *testing.T) {
    store := setupTestStore(t, "test-provider")
    defer store.Close()
    
    unicodeEmails := []string{
        "用户@example.com",
        "пользователь@example.com",
        "benutzer@example.com",
        "utilisateur@example.com",
    }
    
    for i, email := range unicodeEmails {
        token := createTestToken(t)
        token.ID = fmt.Sprintf("unicode-%d", i)
        token.Email = email
        
        err := store.AddToken(token)
        require.NoError(t, err)
        
        retrieved, err := store.GetToken(token.ID)
        require.NoError(t, err)
        assert.Equal(t, email, retrieved.Email)
    }
}
```

### Concurrent Edge Cases

```go
func TestEdgeCase_Concurrent_SameTokenID(t *testing.T) {
    store := setupTestStore(t, "test-provider")
    defer store.Close()
    
    const numGoroutines = 10
    tokenID := "duplicate-id"
    
    var wg sync.WaitGroup
    errors := make(chan error, numGoroutines)
    
    // Try to add same token ID from multiple goroutines
    for i := 0; i < numGoroutines; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            
            token := createTestToken(t)
            token.ID = tokenID
            token.Email = fmt.Sprintf("user%d@example.com", i)
            
            err := store.AddToken(token)
            errors <- err
        }()
    }
    
    wg.Wait()
    close(errors)
    
    // At least one should succeed, others may fail or update
    successCount := 0
    for err := range errors {
        if err == nil {
            successCount++
        }
    }
    
    assert.Greater(t, successCount, 0, "At least one add should succeed")
}

func TestEdgeCase_Concurrent_ReadWhileWrite(t *testing.T) {
    store := setupTestStore(t, "test-provider")
    defer store.Close()
    
    // Add initial token
    token := createTestToken(t)
    token.ID = "read-write-test-id"
    err := store.AddToken(token)
    require.NoError(t, err)
    
    const numGoroutines = 50
    var wg sync.WaitGroup
    errors := make(chan error, numGoroutines*2)
    
    // Start concurrent reads and writes
    for i := 0; i < numGoroutines; i++ {
        // Read goroutine
        wg.Add(1)
        go func() {
            defer wg.Done()
            _, err := store.GetToken("read-write-test-id")
            errors <- err
        }()
        
        // Write goroutine
        wg.Add(1)
        go func() {
            defer wg.Done()
            err := store.UpdateToken("read-write-test-id", func(t *TokenMetadata) {
                t.LastUsed = time.Now().UnixMilli()
            })
            errors <- err
        }()
    }
    
    wg.Wait()
    close(errors)
    
    // Check for critical errors
    for err := range errors {
        if err != nil && !errors.Is(err, sql.ErrTxDone) {
            t.Errorf("Concurrent read/write error: %v", err)
        }
    }
}
```

### Error Path Tests

```go
func TestEdgeCase_Error_DatabaseLocked(t *testing.T) {
    // This test simulates database lock conditions
    store := setupTestStore(t, "test-provider")
    
    // Simulate lock by starting a transaction
    tx, err := store.BeginTransaction()
    require.NoError(t, err)
    defer tx.Rollback()
    
    // Try to add token while transaction is active
    token := createTestToken(t)
    token.ID = "lock-test-id"
    err = store.AddToken(token)
    
    // Should either succeed or return appropriate error
    // (depending on SQLite's locking behavior)
    if err != nil {
        t.Logf("Expected lock error: %v", err)
    }
}

func TestEdgeCase_Error_CorruptData(t *testing.T) {
    store := setupTestStore(t, "test-provider")
    defer store.Close()
    
    // Try to add token with nil values
    token := TokenMetadata{
        ID:          "corrupt-id",
        AccessToken: "", // Empty access token
        RefreshToken: "", // Empty refresh token
    }
    
    err := store.AddToken(token)
    assert.Error(t, err, "Should reject token with empty access token")
}

func TestEdgeCase_Error_DatabaseFull(t *testing.T) {
    // This test simulates disk full conditions
    // In practice, this requires mocking the underlying database
    
    store := setupTestStore(t, "test-provider")
    defer store.Close()
    
    // Try to add extremely large token
    token := createTestToken(t)
    token.ID = "large-token-id"
    token.AccessToken = strings.Repeat("a", 1000000) // 1MB string
    
    err := store.AddToken(token)
    // Should either succeed or return appropriate error
    if err != nil {
        t.Logf("Expected disk full error: %v", err)
    }
}
```

---

## Test Environment Setup

### Local Development Environment

#### Prerequisites

- Go 1.21 or later
- SQLite 3.35 or later
- Test dependencies: `github.com/stretchr/testify`

#### Setup Script

```bash
#!/bin/bash
# scripts/setup-test-env.sh

set -e

echo "Setting up test environment..."

# Install test dependencies
go get github.com/stretchr/testify/assert
go get github.com/stretchr/testify/require
go get github.com/stretchr/testify/mock

# Create test directories
mkdir -p .test-data
mkdir -p .test-backups

# Set test environment variables
export TEST_DB_PATH=".test-data/test.db"
export TEST_FILE_PATH=".test-data/credentials"
export TEST_LOG_LEVEL="debug"

echo "Test environment setup complete!"
echo "TEST_DB_PATH=$TEST_DB_PATH"
echo "TEST_FILE_PATH=$TEST_FILE_PATH"
```

### CI/CD Environment

#### GitHub Actions Configuration

```yaml
# .github/workflows/test.yml

name: Test

on:
  push:
    branches: [ main, develop ]
  pull_request:
    branches: [ main, develop ]

jobs:
  test:
    runs-on: ${{ matrix.os }}
    strategy:
      matrix:
        os: [ubuntu-latest, windows-latest, macos-latest]
        go-version: ['1.21', '1.22']
    
    steps:
    - name: Set up Go
      uses: actions/setup-go@v4
      with:
        go-version: ${{ matrix.go-version }}
    
    - name: Checkout code
      uses: actions/checkout@v3
    
    - name: Install dependencies
      run: go mod download
    
    - name: Run unit tests
      run: go test -v -race -coverprofile=coverage.out ./internal/token/...
    
    - name: Run integration tests
      run: go test -v -race ./internal/token/... -tags=integration
    
    - name: Run benchmarks
      run: go test -bench=. -benchmem ./internal/token/...
    
    - name: Upload coverage
      uses: codecov/codecov-action@v3
      with:
        files: ./coverage.out
        flags: unittests
    
    - name: Build
      run: go build -v ./cmd/qwencoder-proxy/
```

#### Docker Test Environment

```dockerfile
# Dockerfile.test

FROM golang:1.22-alpine

# Install test dependencies
RUN apk add --no-cache sqlite

# Set working directory
WORKDIR /app

# Copy source code
COPY . .

# Run tests
CMD ["go", "test", "-v", "-race", "-coverprofile=/coverage/coverage.out", "./..."]
```

```yaml
# docker-compose.test.yml

version: '3.8'

services:
  test:
    build:
      context: .
      dockerfile: Dockerfile.test
    volumes:
      - ./test-results:/coverage
    environment:
      - TEST_DB_PATH=/tmp/test.db
      - TEST_LOG_LEVEL=debug
```

### Test Data Management

#### Test Data Directory Structure

```
.test-data/
├── tokens.db          # SQLite test database
├── credentials/        # File-based test storage
│   ├── gemini/
│   ├── qwen/
│   └── iflow/
└── backups/           # Test backups
```

#### Test Data Cleanup

```go
// internal/token/test_cleanup.go

package token

import (
    "os"
    "path/filepath"
    "testing"
)

func CleanupTestEnvironment(t *testing.T) {
    testDir := ".test-data"
    
    // Remove all files in test directory
    err := filepath.Walk(testDir, func(path string, info os.FileInfo, err error) error {
        if err != nil {
            return err
        }
        
        // Skip directory itself
        if path == testDir {
            return nil
        }
        
        return os.RemoveAll(path)
    })
    
    if err != nil && !os.IsNotExist(err) {
        t.Logf("Warning: Failed to cleanup test directory: %v", err)
    }
}

func SetupTestEnvironment(t *testing.T) {
    // Create test directory
    err := os.MkdirAll(".test-data", 0755)
    if err != nil && !os.IsExist(err) {
        t.Fatalf("Failed to create test directory: %v", err)
    }
    
    // Create subdirectories
    os.MkdirAll(".test-data/credentials/gemini", 0755)
    os.MkdirAll(".test-data/credentials/qwen", 0755)
    os.MkdirAll(".test-data/credentials/iflow", 0755)
    os.MkdirAll(".test-data/backups", 0755)
    
    // Cleanup on test completion
    t.Cleanup(CleanupTestEnvironment)
}
```

---

## Test Coverage Goals

### Coverage Targets

| Component | Target | Current | Status |
|-----------|---------|---------|--------|
| SQLite Storage Layer | ≥85% | - | Pending |
| Migration Utility | ≥90% | - | Pending |
| Configuration | ≥80% | - | Pending |
| Token Manager | ≥75% | - | Pending |
| Overall Project | ≥80% | - | Pending |

### Coverage Measurement

```bash
# Run tests with coverage
go test -v -race -coverprofile=coverage.out ./...

# Generate coverage report
go tool cover -html=coverage.out -o coverage.html

# View coverage in terminal
go tool cover -func=coverage.out

# Generate coverage percentage
go tool cover -func=coverage.out | grep total
```

### Coverage Report Format

```
coverage: 85.3% of statements

internal/token/sqlite_store.go:    92.1%
internal/token/migrator.go:         88.5%
internal/token/store.go:            70.2%
config/config.go:                    82.3%
internal/token/multi_token_manager.go: 75.8%
```

### Coverage Exclusions

```go
// internal/token/sqlite_store.go

//go:build !coverage

func (s *SQLiteStore) internalMethod() {
    // Code excluded from coverage
}

//go:build !coverage

func init() {
    // Initialization code excluded from coverage
}
```

### Coverage Enforcement

```go
// internal/token/sqlite_store_coverage_test.go

//go:build coverage

package token

import (
    "testing"
    
    "github.com/stretchr/testify/require"
)

func TestCoverage_EnforceMinimum(t *testing.T) {
    // This test enforces minimum coverage
    // It should be run in CI with coverage flag
    
    coverage := getCoverage(t)
    require.GreaterOrEqual(t, coverage, 0.85, "Coverage must be at least 85%")
}

func getCoverage(t *testing.T) float64 {
    // Parse coverage from coverage.out file
    // Implementation depends on go tool cover output
    return 0.0 // Placeholder
}
```

---

## Test Automation

### Pre-commit Hooks

```bash
#!/bin/bash
# .git/hooks/pre-commit

set -e

echo "Running pre-commit checks..."

# Run unit tests
echo "Running unit tests..."
go test -short ./internal/token/...

# Run linter
echo "Running linter..."
golangci-lint run ./...

# Check for TODO comments
echo "Checking for TODO comments..."
if grep -r "TODO" --include="*.go" internal/; then
    echo "Warning: TODO comments found in code"
fi

echo "Pre-commit checks passed!"
```

### Automated Test Runner

```go
// cmd/test-runner/main.go

package main

import (
    "flag"
    "fmt"
    "os"
    "os/exec"
    "strings"
)

func main() {
    var (
        runUnit       = flag.Bool("unit", false, "Run unit tests")
        runIntegration = flag.Bool("integration", false, "Run integration tests")
        runBench      = flag.Bool("bench", false, "Run benchmarks")
        runCoverage   = flag.Bool("cover", false, "Generate coverage report")
        raceDetector  = flag.Bool("race", false, "Enable race detector")
    )
    
    flag.Parse()
    
    var commands []string
    
    if *runUnit {
        commands = append(commands, "go test -v ./internal/token/...")
    }
    
    if *runIntegration {
        commands = append(commands, "go test -v -tags=integration ./internal/token/...")
    }
    
    if *runBench {
        commands = append(commands, "go test -bench=. -benchmem ./internal/token/...")
    }
    
    if *runCoverage {
        commands = append(commands, "go test -coverprofile=coverage.out ./...")
        commands = append(commands, "go tool cover -html=coverage.out -o coverage.html")
    }
    
    if *raceDetector {
        for i := range commands {
            commands[i] += " -race"
        }
    }
    
    // Execute commands
    for _, cmd := range commands {
        parts := strings.Fields(cmd)
        execCmd := exec.Command(parts[0], parts[1:]...)
        execCmd.Stdout = os.Stdout
        execCmd.Stderr = os.Stderr
        
        fmt.Printf("Running: %s\n", cmd)
        err := execCmd.Run()
        if err != nil {
            fmt.Fprintf(os.Stderr, "Error: %v\n", err)
            os.Exit(1)
        }
    }
}
```

### Continuous Integration

```yaml
# .github/workflows/automated-tests.yml

name: Automated Tests

on:
  schedule:
    # Run tests daily at 2 AM UTC
    - cron: '0 2 * * * *'
  workflow_dispatch:

jobs:
  test:
    runs-on: ubuntu-latest
    
    steps:
    - name: Checkout code
      uses: actions/checkout@v3
    
    - name: Set up Go
      uses: actions/setup-go@v4
      with:
        go-version: '1.22'
    
    - name: Run all tests
      run: |
        go test -v -race -coverprofile=coverage.out ./...
        go tool cover -func=coverage.out | grep total
    
    - name: Run benchmarks
      run: go test -bench=. -benchmem ./...
    
    - name: Check coverage threshold
      run: |
        COVERAGE=$(go tool cover -func=coverage.out | grep total | awk '{print $3}' | sed 's/%//')
        echo "Total coverage: $COVERAGE%"
        
        if (( $(echo "$COVERAGE < 80" | bc -l) )); then
          echo "Coverage below 80% threshold"
          exit 1
        fi
    
    - name: Upload artifacts
      uses: actions/upload-artifact@v3
      with:
        name: test-results
        path: |
          coverage.out
          coverage.html
          benchmark.txt
```

---

## Test Documentation

### Test Documentation Files

| File | Purpose |
|------|---------|
| `TESTING.md` | Overall testing guide |
| `internal/token/sqlite_store_test.go` | Test documentation in code |
| `test-plans/` | Detailed test plans |

### Test Plan Template

```markdown
# Test Plan: [Feature Name]

## Overview
Brief description of what is being tested.

## Test Scope
List of components and features being tested.

## Test Cases

| ID | Description | Steps | Expected Result | Status |
|----|-------------|--------|----------------|--------|
| TC001 | [Test case description] | [Test steps] | [Expected result] | [Pass/Fail] |

## Test Data
Description of test data used.

## Dependencies
List of dependencies and their versions.

## Success Criteria
Definition of what constitutes a successful test run.
```

### Code Documentation

```go
// internal/token/sqlite_store_test.go

// TestSQLiteStore_AddToken verifies that a token can be added to the store
// and retrieved correctly.
//
// This test ensures:
// - Tokens are persisted correctly
// - All fields are preserved
// - Duplicate tokens are handled appropriately
func TestSQLiteStore_AddToken(t *testing.T) {
    // Test implementation...
}
```

### Test Result Reporting

```go
// internal/token/test_reporter.go

package token

import (
    "encoding/json"
    "os"
    "testing"
    "time"
)

type TestResult struct {
    Name      string    `json:"name"`
    Status    string    `json:"status"`
    Duration  string    `json:"duration"`
    Error     string    `json:"error,omitempty"`
    Coverage  float64   `json:"coverage,omitempty"`
}

type TestReport struct {
    Timestamp   time.Time    `json:"timestamp"`
    GoVersion   string      `json:"go_version"`
    OS          string      `json:"os"`
    Results     []TestResult `json:"results"`
    Summary     Summary     `json:"summary"`
}

type Summary struct {
    Total    int     `json:"total"`
    Passed   int     `json:"passed"`
    Failed   int     `json:"failed"`
    Skipped  int     `json:"skipped"`
    Duration string  `json:"duration"`
}

func GenerateTestReport(t *testing.T, results []TestResult) {
    report := TestReport{
        Timestamp: time.Now(),
        GoVersion: runtime.Version(),
        OS:        runtime.GOOS,
        Results:    results,
    }
    
    // Calculate summary
    for _, result := range results {
        report.Summary.Total++
        switch result.Status {
        case "PASS":
            report.Summary.Passed++
        case "FAIL":
            report.Summary.Failed++
        case "SKIP":
            report.Summary.Skipped++
        }
    }
    
    // Write report to file
    data, err := json.MarshalIndent(report, "", "  ")
    if err != nil {
        t.Fatalf("Failed to marshal test report: %v", err)
    }
    
    err = os.WriteFile("test-report.json", data, 0644)
    if err != nil {
        t.Fatalf("Failed to write test report: %v", err)
    }
}
```

---

## Implementation Roadmap

### Phase 1: Test Foundation (Week 1)

**Tasks:**
1. Set up test environment and CI/CD pipeline
2. Create test helper functions and fixtures
3. Implement unit test framework
4. Set up coverage reporting

**Deliverables:**
- Test environment setup scripts
- CI/CD pipeline configuration
- Test helper functions
- Coverage reporting

**Acceptance Criteria:**
- Test environment can be set up with single command
- CI/CD pipeline runs tests successfully
- Coverage reports are generated

### Phase 2: Unit Tests (Week 2)

**Tasks:**
1. Write unit tests for SQLite storage layer
2. Write unit tests for migration utility
3. Write unit tests for configuration
4. Write unit tests for token manager integration

**Deliverables:**
- Complete unit test suite for SQLite storage
- Complete unit test suite for migration
- Unit tests for configuration changes
- Integration tests for token manager

**Acceptance Criteria:**
- All unit tests pass
- Test coverage ≥ 85% for SQLite storage
- Test coverage ≥ 90% for migration utility

### Phase 3: Integration Tests (Week 3)

**Tasks:**
1. Write integration tests for OAuth flow
2. Write integration tests for token refresh
3. Write integration tests for concurrent access
4. Write integration tests for migration

**Deliverables:**
- OAuth flow integration tests
- Token refresh integration tests
- Concurrent access integration tests
- Migration integration tests

**Acceptance Criteria:**
- All integration tests pass
- OAuth flow works correctly with SQLite storage
- Token refresh works correctly with SQLite storage
- No race conditions detected in concurrent access tests
- Migration from file to SQLite completes successfully
- Data integrity verified after migration

### Phase 4: Performance Tests (Week 4)

**Tasks:**
1. Write performance benchmarks for SQLite storage
2. Write performance benchmarks for file-based storage
3. Compare performance metrics
4. Identify performance bottlenecks
5. Optimize based on benchmark results

**Deliverables:**
- Complete benchmark suite for both storage backends
- Performance comparison report
- Optimization recommendations

**Acceptance Criteria:**
- All benchmarks run successfully
- SQLite performance meets or exceeds file-based storage performance
- Performance bottlenecks identified and documented
- At least one optimization implemented based on benchmark results

### Phase 5: Migration Tests (Week 5)

**Tasks:**
1. Write migration tests for all scenarios
2. Test migration rollback functionality
3. Test migration idempotency
4. Test migration with corrupted data
5. Test migration with large datasets

**Deliverables:**
- Complete migration test suite
- Migration test results
- Rollback procedure documentation

**Acceptance Criteria:**
- All migration tests pass
- Migration rollback works correctly
- Migration is idempotent (can be run multiple times)
- Migration handles corrupted data gracefully
- Migration completes successfully with large datasets (10,000+ tokens)

### Phase 6: Documentation (Week 6)

**Tasks:**
1. Write comprehensive test documentation
2. Create test execution guide
3. Document test data setup
4. Create troubleshooting guide for tests
5. Document coverage reporting

**Deliverables:**
- Complete test documentation
- Test execution guide
- Test data setup guide
- Test troubleshooting guide

**Acceptance Criteria:**
- All test scenarios documented
- Test execution guide is clear and accurate
- Test data setup is documented
- Troubleshooting guide covers common issues

### Phase 7: CI/CD Integration (Week 7)

**Tasks:**
1. Configure GitHub Actions for automated testing
2. Set up coverage reporting
3. Configure benchmark reporting
4. Set up test result notifications
5. Configure test artifact storage

**Deliverables:**
- Complete CI/CD pipeline configuration
- Coverage reports generated automatically
- Benchmark reports generated automatically
- Test result notifications configured

**Acceptance Criteria:**
- CI/CD pipeline runs all tests automatically
- Coverage reports are generated and published
- Benchmark results are tracked over time
- Test failures trigger notifications

### Phase 8: Final Validation (Week 8)

**Tasks:**
1. Run full test suite on production-like environment
2. Validate all test objectives
3. Document any remaining issues
4. Create final test report
5. Obtain sign-off from stakeholders

**Deliverables:**
- Final test report
- Validation results
- Issue documentation (if any)
- Stakeholder sign-off

**Acceptance Criteria:**
- All test objectives validated
- Test coverage meets or exceeds targets
- Performance benchmarks meet requirements
- Migration process validated
- Stakeholder sign-off obtained

---

## Summary

This testing strategy provides a comprehensive approach to ensuring the SQLite migration is successful, reliable, and maintains data integrity. The strategy covers:

1. **Testing Philosophy**: Following testing pyramid principles with TDD approach
2. **Unit Testing**: Comprehensive unit tests for all components
3. **Integration Testing**: Validating component interactions and workflows
4. **Performance Testing**: Benchmarking and load testing to ensure performance parity
5. **Migration Testing**: Validating the migration process with rollback capability
6. **Edge Case Testing**: Testing unusual and error conditions
7. **Test Environment**: Isolated and reproducible test environments
8. **Coverage Goals**: Clear targets for test coverage (≥80% for new code)
9. **Test Automation**: CI/CD integration for automated testing
10. **Test Documentation**: Comprehensive documentation for all test procedures
11. **Implementation Roadmap**: 8-week phased approach to implementing the test strategy

### Key Success Metrics

| Metric | Target | Current Status |
|---------|---------|----------------|
| Unit Test Coverage | ≥85% | TBD |
| Integration Test Coverage | ≥75% | TBD |
| Migration Test Success Rate | 100% | TBD |
| Performance Parity | SQLite ≥ File | TBD |
| Data Integrity | 100% | TBD |
| CI/CD Pass Rate | ≥95% | TBD |

### Next Steps

1. Review and approve this testing strategy
2. Set up test environment
3. Begin Phase 1: Test Foundation
4. Track progress against implementation roadmap
5. Adjust strategy as needed based on findings

---

**Document Version**: 1.0
**Last Updated**: 2024-02-24
**Author**: Go Architect Team
**Status**: Draft - Pending Review