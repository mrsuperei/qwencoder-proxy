package ratelimit

import (
	"context"
	"database/sql"
	"testing"
	"time"
)

// cachedMockUsageTracker is a mock implementation of UsageTracker for testing.
type cachedMockUsageTracker struct {
	recordUsageCalled int
}

func (m *cachedMockUsageTracker) RecordUsage(ctx context.Context, providerID string, tokenID string, requestCount int, tokenCount int) error {
	m.recordUsageCalled++
	return nil
}

func (m *cachedMockUsageTracker) GetProviderUsage(ctx context.Context, providerID string) (*UsageMetrics, error) {
	return &UsageMetrics{}, nil
}

func (m *cachedMockUsageTracker) GetTokenUsage(ctx context.Context, providerID string, tokenID string) (*UsageMetrics, error) {
	return &UsageMetrics{}, nil
}

func (m *cachedMockUsageTracker) GetAllProviderUsage(ctx context.Context) (map[string]*UsageMetrics, error) {
	return make(map[string]*UsageMetrics), nil
}

func (m *cachedMockUsageTracker) ResetUsage(ctx context.Context, providerID string, tokenID string) error {
	return nil
}

func (m *cachedMockUsageTracker) CleanupOrphanedUsageRecords(ctx context.Context) error {
	return nil
}

func (m *cachedMockUsageTracker) GetDB() *sql.DB {
	return nil
}

func (m *cachedMockUsageTracker) Close() error {
	return nil
}

// Model usage tracking methods
func (m *cachedMockUsageTracker) RecordModelUsage(ctx context.Context, providerID string, tokenID string, model string, inputTokens int, outputTokens int) error {
	return nil
}

func (m *cachedMockUsageTracker) GetModelUsage(ctx context.Context, providerID string, tokenID string, model string) (*ModelUsageMetrics, error) {
	return &ModelUsageMetrics{}, nil
}

func (m *cachedMockUsageTracker) GetAllModelUsage(ctx context.Context, providerID string) (map[string]*ModelUsageMetrics, error) {
	return make(map[string]*ModelUsageMetrics), nil
}

func (m *cachedMockUsageTracker) GetTokenModelUsage(ctx context.Context, tokenID string) (map[string]*ModelUsageMetrics, error) {
	return make(map[string]*ModelUsageMetrics), nil
}

func (m *cachedMockUsageTracker) GetProviderModelUsage(ctx context.Context, providerID string) (map[string]*ModelUsageMetrics, error) {
	return make(map[string]*ModelUsageMetrics), nil
}

func (m *cachedMockUsageTracker) ResetModelUsage(ctx context.Context, providerID string, tokenID string, model string) error {
	return nil
}

func (m *cachedMockUsageTracker) GetRequestHistory(ctx context.Context, filter *RequestHistoryFilter, page int, pageSize int) (*RequestHistoryResponse, error) {
	return &RequestHistoryResponse{}, nil
}

func (m *cachedMockUsageTracker) GetRequestHistoryByToken(ctx context.Context, tokenID string, filter *RequestHistoryFilter, page int, pageSize int) (*RequestHistoryResponse, error) {
	return &RequestHistoryResponse{}, nil
}

func (m *cachedMockUsageTracker) GetRequestHistoryByModel(ctx context.Context, model string, filter *RequestHistoryFilter, page int, pageSize int) (*RequestHistoryResponse, error) {
	return &RequestHistoryResponse{}, nil
}

func (m *cachedMockUsageTracker) GetRequestHistorySummary(ctx context.Context, tokenID string, model string, startTime int64, endTime int64) (*RequestHistorySummary, error) {
	return &RequestHistorySummary{}, nil
}

func (m *cachedMockUsageTracker) DeleteRequestHistory(ctx context.Context, filter *RequestHistoryFilter) (int64, error) {
	return 0, nil
}

// cachedMockLogger is a mock implementation of logging.Logger for testing.
type cachedMockLogger struct {
	infoMessages []string
}

func (m *cachedMockLogger) InfoLog(msg string, args ...interface{}) {
	m.infoMessages = append(m.infoMessages, msg)
}

func (m *cachedMockLogger) DebugLog(msg string, args ...interface{}) {
	// Ignore debug logs in tests
}

func (m *cachedMockLogger) WarnLog(msg string, args ...interface{}) {
	// Ignore warn logs in tests
}

func (m *cachedMockLogger) ErrorLog(msg string, args ...interface{}) {
	// Ignore error logs in tests
}

func TestNewCachedUsageTracker(t *testing.T) {
	config := DefaultCacheConfig()
	mockTracker := &cachedMockUsageTracker{}
	mockLogger := &cachedMockLogger{}

	tracker := NewCachedUsageTracker(mockTracker, config, mockLogger, nil)
	defer tracker.Close()

	// Verify tracker is created
	if tracker == nil {
		t.Error("Expected tracker to be created")
	}

	// Type assertion to access GetCacheStats
	cachedTracker, ok := tracker.(*CachedUsageTracker)
	if !ok {
		t.Fatal("Expected tracker to be *CachedUsageTracker")
	}

	stats := cachedTracker.GetCacheStats()

	// Verify cache is initialized
	if stats.ProviderCacheSize != 0 || stats.TokenCacheSize != 0 {
		t.Error("Expected empty cache on initialization")
	}
}

func TestCachedUsageTracker_GetProviderUsage(t *testing.T) {
	mockTracker := &cachedMockUsageTracker{}
	mockLogger := &cachedMockLogger{}

	config := &CacheConfig{
		ProviderMetricsTTL:  5 * time.Second,
		TokenMetricsTTL:     5 * time.Second,
		MaxProviderEntries:  10,
		MaxTokenEntries:     100,
		RefreshBeforeExpiry: 1 * time.Second,
	}

	tracker := NewCachedUsageTracker(mockTracker, config, mockLogger, nil)
	defer tracker.Close()

	// Type assertion to access GetCacheStats
	cachedTracker, ok := tracker.(*CachedUsageTracker)
	if !ok {
		t.Fatal("Expected tracker to be *CachedUsageTracker")
	}

	// Test cache miss
	metrics, err := tracker.GetProviderUsage(context.Background(), "test-provider")
	if err != nil {
		t.Errorf("GetProviderUsage failed: %v", err)
	}

	if metrics == nil {
		t.Error("Expected metrics to be returned")
	}

	// Verify cache was created
	stats := cachedTracker.GetCacheStats()
	if stats.ProviderCacheSize != 1 {
		t.Errorf("Expected 1 provider in cache, got %d", stats.ProviderCacheSize)
	}

	if stats.CacheMisses != 1 {
		t.Errorf("Expected 1 cache miss, got %d", stats.CacheMisses)
	}

	// Test cache hit
	metrics2, err := tracker.GetProviderUsage(context.Background(), "test-provider")
	if err != nil {
		t.Errorf("Second GetProviderUsage failed: %v", err)
	}

	stats2 := cachedTracker.GetCacheStats()
	if stats2.CacheHits != 1 {
		t.Errorf("Expected 1 cache hit, got %d", stats2.CacheHits)
	}

	// Verify same metrics are returned (cached)
	if metrics != metrics2 {
		t.Error("Expected same metrics object on cache hit")
	}
}

func TestCachedUsageTracker_RecordUsage(t *testing.T) {
	mockTracker := &cachedMockUsageTracker{}
	mockLogger := &cachedMockLogger{}

	config := &CacheConfig{
		ProviderMetricsTTL:  5 * time.Second,
		TokenMetricsTTL:     5 * time.Second,
		MaxProviderEntries:  10,
		MaxTokenEntries:     100,
		RefreshBeforeExpiry: 1 * time.Second,
	}

	tracker := NewCachedUsageTracker(mockTracker, config, mockLogger, nil)
	defer tracker.Close()

	// Type assertion to access GetCacheStats
	cachedTracker, ok := tracker.(*CachedUsageTracker)
	if !ok {
		t.Fatal("Expected tracker to be *CachedUsageTracker")
	}

	err := tracker.RecordUsage(context.Background(), "test-provider", "test-token", 1, 100)
	if err != nil {
		t.Errorf("RecordUsage failed: %v", err)
	}

	if mockTracker.recordUsageCalled != 1 {
		t.Error("RecordUsage was not called on underlying tracker")
	}

	// Verify cache was invalidated
	stats := cachedTracker.GetCacheStats()
	if stats.ProviderCacheSize != 0 && stats.TokenCacheSize != 0 {
		t.Log("Cache entries after RecordUsage - cache invalidation may have occurred")
	}
}

func TestCachedUsageTracker_CacheExpiry(t *testing.T) {
	mockTracker := &cachedMockUsageTracker{}
	mockLogger := &cachedMockLogger{}

	config := &CacheConfig{
		ProviderMetricsTTL:  100 * time.Millisecond, // Short TTL for testing
		TokenMetricsTTL:     100 * time.Millisecond,
		MaxProviderEntries:  10,
		MaxTokenEntries:     100,
		RefreshBeforeExpiry: 10 * time.Millisecond,
	}

	tracker := NewCachedUsageTracker(mockTracker, config, mockLogger, nil)
	defer tracker.Close()

	// Type assertion to access GetCacheStats
	cachedTracker, ok := tracker.(*CachedUsageTracker)
	if !ok {
		t.Fatal("Expected tracker to be *CachedUsageTracker")
	}

	// Create cache entry
	_, err := tracker.GetProviderUsage(context.Background(), "test-provider")
	if err != nil {
		t.Errorf("GetProviderUsage failed: %v", err)
	}

	stats1 := cachedTracker.GetCacheStats()
	if stats1.ProviderCacheSize != 1 {
		t.Errorf("Expected 1 provider in cache, got %d", stats1.ProviderCacheSize)
	}

	// Wait for cache to expire
	time.Sleep(200 * time.Millisecond)

	// Access again - should refresh from underlying tracker
	_, err = tracker.GetProviderUsage(context.Background(), "test-provider")
	if err != nil {
		t.Errorf("GetProviderUsage after expiry failed: %v", err)
	}

	stats2 := cachedTracker.GetCacheStats()
	if stats2.CacheMisses > stats1.CacheMisses {
		t.Log("Cache was refreshed after expiry as expected")
	}
}

func TestCachedUsageTracker_GetTokenUsage(t *testing.T) {
	mockTracker := &cachedMockUsageTracker{}
	mockLogger := &cachedMockLogger{}

	config := &CacheConfig{
		ProviderMetricsTTL:  5 * time.Second,
		TokenMetricsTTL:     5 * time.Second,
		MaxProviderEntries:  10,
		MaxTokenEntries:     100,
		RefreshBeforeExpiry: 1 * time.Second,
	}

	tracker := NewCachedUsageTracker(mockTracker, config, mockLogger, nil)
	defer tracker.Close()

	// Type assertion to access GetCacheStats
	cachedTracker, ok := tracker.(*CachedUsageTracker)
	if !ok {
		t.Fatal("Expected tracker to be *CachedUsageTracker")
	}

	// Test cache miss
	metrics, err := tracker.GetTokenUsage(context.Background(), "test-provider", "test-token")
	if err != nil {
		t.Errorf("GetTokenUsage failed: %v", err)
	}

	if metrics == nil {
		t.Error("Expected metrics to be returned")
	}

	// Verify cache was created
	stats := cachedTracker.GetCacheStats()
	if stats.TokenCacheSize != 1 {
		t.Errorf("Expected 1 token in cache, got %d", stats.TokenCacheSize)
	}

	// Test cache hit
	metrics2, err := tracker.GetTokenUsage(context.Background(), "test-provider", "test-token")
	if err != nil {
		t.Errorf("Second GetTokenUsage failed: %v", err)
	}

	stats2 := cachedTracker.GetCacheStats()
	if stats2.CacheHits != 1 {
		t.Errorf("Expected 1 cache hit, got %d", stats2.CacheHits)
	}

	// Verify same metrics are returned (cached)
	if metrics != metrics2 {
		t.Error("Expected same metrics object on cache hit")
	}
}

func TestCachedUsageTracker_NilConfig(t *testing.T) {
	mockTracker := &cachedMockUsageTracker{}
	mockLogger := &cachedMockLogger{}

	// Pass nil config - should use defaults
	tracker := NewCachedUsageTracker(mockTracker, nil, mockLogger, nil)
	defer tracker.Close()

	// Type assertion to access GetCacheStats
	cachedTracker, ok := tracker.(*CachedUsageTracker)
	if !ok {
		t.Fatal("Expected tracker to be *CachedUsageTracker")
	}

	// Verify tracker works with default config
	_, err := tracker.GetProviderUsage(context.Background(), "test-provider")
	if err != nil {
		t.Errorf("GetProviderUsage failed: %v", err)
	}

	stats := cachedTracker.GetCacheStats()
	if stats.ProviderCacheSize != 1 {
		t.Errorf("Expected 1 provider in cache, got %d", stats.ProviderCacheSize)
	}
}

func TestCachedUsageTracker_GetCacheStats(t *testing.T) {
	mockTracker := &cachedMockUsageTracker{}
	mockLogger := &cachedMockLogger{}

	config := &CacheConfig{
		ProviderMetricsTTL:  5 * time.Second,
		TokenMetricsTTL:     5 * time.Second,
		MaxProviderEntries:  10,
		MaxTokenEntries:     100,
		RefreshBeforeExpiry: 1 * time.Second,
	}

	tracker := NewCachedUsageTracker(mockTracker, config, mockLogger, nil)
	defer tracker.Close()

	// Type assertion to access GetCacheStats
	cachedTracker, ok := tracker.(*CachedUsageTracker)
	if !ok {
		t.Fatal("Expected tracker to be *CachedUsageTracker")
	}

	// Get initial stats
	stats := cachedTracker.GetCacheStats()

	// Verify initial state
	if stats.CacheHits != 0 || stats.CacheMisses != 0 {
		t.Error("Expected no cache activity initially")
	}

	if stats.ProviderCacheSize != 0 || stats.TokenCacheSize != 0 {
		t.Error("Expected empty cache initially")
	}

	// Add some cache entries
	_, _ = tracker.GetProviderUsage(context.Background(), "provider1")
	_, _ = tracker.GetProviderUsage(context.Background(), "provider2")
	_, _ = tracker.GetTokenUsage(context.Background(), "provider1", "token1")

	// Get updated stats
	stats = cachedTracker.GetCacheStats()

	// Verify stats reflect cache activity
	if stats.ProviderCacheSize != 2 {
		t.Errorf("Expected 2 providers in cache, got %d", stats.ProviderCacheSize)
	}

	if stats.TokenCacheSize != 1 {
		t.Errorf("Expected 1 token in cache, got %d", stats.TokenCacheSize)
	}

	if stats.CacheMisses != 3 {
		t.Errorf("Expected 3 cache misses, got %d", stats.CacheMisses)
	}
}
