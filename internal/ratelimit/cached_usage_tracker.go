package ratelimit

import (
	"context"
	"database/sql"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sunbankio/qwencoder-proxy/internal/logging"
)

// CachedUsageMetrics wraps usage metrics with cache metadata.
type CachedUsageMetrics struct {
	Metrics   *UsageMetrics
	CachedAt  time.Time
	ExpiresAt time.Time
	HitCount  atomic.Int64
	MissCount atomic.Int64
}

// CacheConfig configures the in-memory cache.
type CacheConfig struct {
	// TTL settings
	ProviderMetricsTTL time.Duration // Default: 5 seconds
	TokenMetricsTTL    time.Duration // Default: 5 seconds

	// Cache size limits
	MaxProviderEntries int // Default: 100
	MaxTokenEntries    int // Default: 1000

	// Refresh strategy
	RefreshBeforeExpiry time.Duration // Default: 1 second before expiry
}

// DefaultCacheConfig returns default cache configuration.
func DefaultCacheConfig() *CacheConfig {
	return &CacheConfig{
		ProviderMetricsTTL:  5 * time.Second,
		TokenMetricsTTL:     5 * time.Second,
		MaxProviderEntries:  100,
		MaxTokenEntries:     1000,
		RefreshBeforeExpiry: 1 * time.Second,
	}
}

// CachedUsageTracker wraps UsageTracker with in-memory caching.
type CachedUsageTracker struct {
	// Underlying usage tracker (database)
	underlying UsageTracker

	// In-memory caches
	providerCache map[string]*CachedUsageMetrics
	tokenCache    map[string]*tokenCacheEntry

	// Cache configuration
	config CacheConfig

	// Synchronization
	mu sync.RWMutex

	// Metrics
	cacheHits   atomic.Int64
	cacheMisses atomic.Int64
	cacheErrors atomic.Int64

	// Background refresh
	ctx    context.Context
	cancel context.CancelFunc
	logger logging.Logger

	// Cache invalidation
	invalidator      *CacheInvalidator
	invalidationChan chan InvalidationEvent
	unsubscribeFunc  func()
}

type tokenCacheEntry struct {
	providerID string
	metrics    *CachedUsageMetrics
}

// NewCachedUsageTracker creates a new cached usage tracker.
func NewCachedUsageTracker(underlying UsageTracker, config *CacheConfig, logger logging.Logger, invalidator *CacheInvalidator) UsageTracker {
	if config == nil {
		config = DefaultCacheConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())

	tracker := &CachedUsageTracker{
		underlying:       underlying,
		providerCache:    make(map[string]*CachedUsageMetrics),
		tokenCache:       make(map[string]*tokenCacheEntry),
		config:           *config,
		ctx:              ctx,
		cancel:           cancel,
		logger:           logger,
		invalidator:      invalidator,
		invalidationChan: make(chan InvalidationEvent, 10),
	}

	// Start background refresh
	tracker.startBackgroundRefresh()

	// Start invalidation event handler if invalidator is provided
	if invalidator != nil {
		tracker.startInvalidationHandler()
	}

	logger.InfoLog("[CachedUsageTracker] Initialized with cache: providers=%d, tokens=%d, TTL=%v",
		config.MaxProviderEntries, config.MaxTokenEntries, config.ProviderMetricsTTL)

	return tracker
}

// getProviderMetricsFromCache retrieves provider metrics from cache.
func (c *CachedUsageTracker) getProviderMetricsFromCache(providerID string) *CachedUsageMetrics {
	c.mu.RLock()
	defer c.mu.RUnlock()

	cached, exists := c.providerCache[providerID]
	if !exists {
		return nil
	}

	// Check expiration
	if time.Now().After(cached.ExpiresAt) {
		return nil
	}

	// Update access time and hit count
	cached.CachedAt = time.Now()
	cached.HitCount.Add(1)

	return cached
}

// getTokenMetricsFromCache retrieves token metrics from cache.
func (c *CachedUsageTracker) getTokenMetricsFromCache(tokenID string) *CachedUsageMetrics {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, exists := c.tokenCache[tokenID]
	if !exists {
		return nil
	}

	// Check expiration
	if time.Now().After(entry.metrics.ExpiresAt) {
		return nil
	}

	// Update access time and hit count
	entry.metrics.CachedAt = time.Now()
	entry.metrics.HitCount.Add(1)

	return entry.metrics
}

// updateProviderCache updates provider cache with new metrics.
func (c *CachedUsageTracker) updateProviderCache(providerID string, metrics *UsageMetrics) {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	cached := &CachedUsageMetrics{
		Metrics:   metrics,
		CachedAt:  now,
		ExpiresAt: now.Add(c.config.ProviderMetricsTTL),
		HitCount:  atomic.Int64{},
		MissCount: atomic.Int64{},
	}

	c.providerCache[providerID] = cached
	c.evictIfNeeded()
}

// updateTokenCache updates token cache with new metrics.
func (c *CachedUsageTracker) updateTokenCache(tokenID string, providerID string, metrics *UsageMetrics) {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	cached := &CachedUsageMetrics{
		Metrics:   metrics,
		CachedAt:  now,
		ExpiresAt: now.Add(c.config.TokenMetricsTTL),
		HitCount:  atomic.Int64{},
		MissCount: atomic.Int64{},
	}

	c.tokenCache[tokenID] = &tokenCacheEntry{
		providerID: providerID,
		metrics:    cached,
	}

	c.evictIfNeeded()
}

// invalidateProviderCache removes provider metrics from cache.
func (c *CachedUsageTracker) invalidateProviderCache(providerID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.providerCache, providerID)
}

// invalidateTokenCache removes token metrics from cache.
func (c *CachedUsageTracker) invalidateTokenCache(tokenID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.tokenCache, tokenID)
}

// invalidateAll clears all cache entries.
func (c *CachedUsageTracker) invalidateAll() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.providerCache = make(map[string]*CachedUsageMetrics)
	c.tokenCache = make(map[string]*tokenCacheEntry)
}

// GetProviderUsage retrieves provider usage with caching.
func (c *CachedUsageTracker) GetProviderUsage(ctx context.Context, providerID string) (*UsageMetrics, error) {
	// Try cache first
	if cached := c.getProviderMetricsFromCache(providerID); cached != nil {
		c.cacheHits.Add(1)
		c.logger.DebugLog("[CachedUsageTracker] Cache hit for provider: %s", providerID)
		return cached.Metrics, nil
	}

	// Cache miss - query database
	c.cacheMisses.Add(1)
	c.logger.DebugLog("[CachedUsageTracker] Cache miss for provider: %s", providerID)

	metrics, err := c.underlying.GetProviderUsage(ctx, providerID)
	if err != nil {
		c.cacheErrors.Add(1)
		return nil, err
	}

	// Update cache
	c.updateProviderCache(providerID, metrics)

	return metrics, nil
}

// GetTokenUsage retrieves token usage with caching.
func (c *CachedUsageTracker) GetTokenUsage(ctx context.Context, providerID string, tokenID string) (*UsageMetrics, error) {
	// Try cache first
	if cached := c.getTokenMetricsFromCache(tokenID); cached != nil {
		c.cacheHits.Add(1)
		c.logger.DebugLog("[CachedUsageTracker] Cache hit for token: %s", tokenID)
		return cached.Metrics, nil
	}

	// Cache miss - query database
	c.cacheMisses.Add(1)
	c.logger.DebugLog("[CachedUsageTracker] Cache miss for token: %s", tokenID)

	metrics, err := c.underlying.GetTokenUsage(ctx, providerID, tokenID)
	if err != nil {
		c.cacheErrors.Add(1)
		return nil, err
	}

	// Update cache
	c.updateTokenCache(tokenID, providerID, metrics)

	return metrics, nil
}

// GetAllProviderUsage retrieves all provider usage with caching.
func (c *CachedUsageTracker) GetAllProviderUsage(ctx context.Context) (map[string]*UsageMetrics, error) {
	// For GetAll, we can't rely entirely on cache as it might not have all providers
	// Query database and update cache as we go

	allMetrics, err := c.underlying.GetAllProviderUsage(ctx)
	if err != nil {
		return nil, err
	}

	// Update cache with fresh data
	for providerID, metrics := range allMetrics {
		c.updateProviderCache(providerID, metrics)
	}

	return allMetrics, nil
}

// RecordUsage records usage with cache invalidation.
func (c *CachedUsageTracker) RecordUsage(ctx context.Context, providerID string, tokenID string, requestCount int, tokenCount int) error {
	// Record to underlying tracker (database)
	err := c.underlying.RecordUsage(ctx, providerID, tokenID, requestCount, tokenCount)
	if err != nil {
		return err
	}

	// Invalidate affected cache entries
	c.invalidateProviderCache(providerID)
	if tokenID != "" {
		c.invalidateTokenCache(tokenID)
	}

	// Publish invalidation events
	if c.invalidator != nil {
		c.invalidator.PublishInvalidation(InvalidationProvider, providerID, "write")
		if tokenID != "" {
			c.invalidator.PublishInvalidation(InvalidationToken, tokenID, "write")
		}
	}

	return nil
}

// RecordModelUsage records model usage with cache invalidation.
func (c *CachedUsageTracker) RecordModelUsage(ctx context.Context, providerID string, tokenID string, model string, inputTokens int, outputTokens int) error {
	// Record to underlying tracker (database)
	err := c.underlying.RecordModelUsage(ctx, providerID, tokenID, model, inputTokens, outputTokens)
	if err != nil {
		return err
	}

	// Invalidate affected cache entries
	c.invalidateProviderCache(providerID)
	if tokenID != "" {
		c.invalidateTokenCache(tokenID)
	}

	// Publish invalidation events
	if c.invalidator != nil {
		c.invalidator.PublishInvalidation(InvalidationProvider, providerID, "write")
		if tokenID != "" {
			c.invalidator.PublishInvalidation(InvalidationToken, tokenID, "write")
		}
	}

	return nil
}

// ResetUsage resets usage metrics with cache invalidation.
func (c *CachedUsageTracker) ResetUsage(ctx context.Context, providerID string, tokenID string) error {
	err := c.underlying.ResetUsage(ctx, providerID, tokenID)
	if err != nil {
		return err
	}

	// Invalidate affected cache entries
	c.invalidateProviderCache(providerID)
	if tokenID != "" {
		c.invalidateTokenCache(tokenID)
	}

	return nil
}

// CleanupOrphanedUsageRecords cleans up orphaned records.
func (c *CachedUsageTracker) CleanupOrphanedUsageRecords(ctx context.Context) error {
	return c.underlying.CleanupOrphanedUsageRecords(ctx)
}

// GetDB returns the underlying database connection.
func (c *CachedUsageTracker) GetDB() *sql.DB {
	return c.underlying.GetDB()
}

// Close closes the cached usage tracker.
func (c *CachedUsageTracker) Close() error {
	// Stop background refresh
	c.cancel()

	// Unsubscribe from invalidation events
	if c.unsubscribeFunc != nil {
		c.unsubscribeFunc()
	}

	// Close underlying tracker
	return c.underlying.Close()
}

// Model usage tracking methods

// GetModelUsage retrieves model usage (no caching for now).
func (c *CachedUsageTracker) GetModelUsage(ctx context.Context, providerID string, providerID2 string, model string) (*ModelUsageMetrics, error) {
	return c.underlying.GetModelUsage(ctx, providerID, providerID2, model)
}

// GetAllModelUsage retrieves all model usage (no caching for now).
func (c *CachedUsageTracker) GetAllModelUsage(ctx context.Context, providerID string) (map[string]*ModelUsageMetrics, error) {
	return c.underlying.GetAllModelUsage(ctx, providerID)
}

// GetTokenModelUsage retrieves token model usage (no caching for now).
func (c *CachedUsageTracker) GetTokenModelUsage(ctx context.Context, tokenID string) (map[string]*ModelUsageMetrics, error) {
	return c.underlying.GetTokenModelUsage(ctx, tokenID)
}

// GetProviderModelUsage retrieves provider model usage (no caching for now).
func (c *CachedUsageTracker) GetProviderModelUsage(ctx context.Context, providerID string) (map[string]*ModelUsageMetrics, error) {
	return c.underlying.GetProviderModelUsage(ctx, providerID)
}

// ResetModelUsage resets model usage with cache invalidation.
func (c *CachedUsageTracker) ResetModelUsage(ctx context.Context, providerID string, tokenID string, model string) error {
	err := c.underlying.ResetModelUsage(ctx, providerID, tokenID, model)
	if err != nil {
		return err
	}

	// Invalidate affected cache entries
	c.invalidateProviderCache(providerID)
	if tokenID != "" {
		c.invalidateTokenCache(tokenID)
	}

	return nil
}

// startBackgroundRefresh runs periodic cache refresh.
func (c *CachedUsageTracker) startBackgroundRefresh() {
	ticker := time.NewTicker(c.config.RefreshBeforeExpiry)
	defer ticker.Stop()

	go func() {
		for {
			select {
			case <-c.ctx.Done():
				c.logger.DebugLog("[CachedUsageTracker] Background refresh stopped")
				return
			case <-ticker.C:
				c.refreshExpiringEntries()
			}
		}
	}()
}

// startInvalidationHandler starts the goroutine that handles invalidation events.
func (c *CachedUsageTracker) startInvalidationHandler() {
	// Subscribe to invalidation events
	c.invalidationChan, c.unsubscribeFunc = c.invalidator.Subscribe()

	go c.handleInvalidationEvents()
}

// handleInvalidationEvents processes incoming invalidation events.
func (c *CachedUsageTracker) handleInvalidationEvents() {
	for {
		select {
		case <-c.ctx.Done():
			c.logger.DebugLog("[CachedUsageTracker] Invalidation event handler stopped")
			return
		case event, ok := <-c.invalidationChan:
			if !ok {
				c.logger.DebugLog("[CachedUsageTracker] Invalidation channel closed")
				return
			}
			c.processInvalidationEvent(event)
		}
	}
}

// processInvalidationEvent processes a single invalidation event.
func (c *CachedUsageTracker) processInvalidationEvent(event InvalidationEvent) {
	switch event.Type {
	case InvalidationProvider:
		c.invalidateProviderCache(event.Target)
		c.logger.DebugLog("[CachedUsageTracker] Invalidated provider cache: %s (source: %s)",
			event.Target, event.Source)
	case InvalidationToken:
		c.invalidateTokenCache(event.Target)
		c.logger.DebugLog("[CachedUsageTracker] Invalidated token cache: %s (source: %s)",
			event.Target, event.Source)
	case InvalidationAll:
		c.invalidateAll()
		c.logger.DebugLog("[CachedUsageTracker] Invalidated all cache (source: %s)", event.Source)
	case InvalidationConfig:
		// Config changes may affect all cached data
		c.invalidateAll()
		c.logger.DebugLog("[CachedUsageTracker] Invalidated all cache due to config change (source: %s)",
			event.Source)
	}
}

// refreshExpiringEntries refreshes cache entries that are about to expire.
func (c *CachedUsageTracker) refreshExpiringEntries() {
	c.mu.RLock()

	// Collect entries to refresh
	var providersToRefresh []string
	var tokensToRefresh []string

	now := time.Now()
	refreshThreshold := now.Add(c.config.RefreshBeforeExpiry)

	for providerID, cached := range c.providerCache {
		if cached.ExpiresAt.Before(refreshThreshold) && cached.HitCount.Load() > 10 {
			providersToRefresh = append(providersToRefresh, providerID)
		}
	}

	for tokenID, entry := range c.tokenCache {
		if entry.metrics.ExpiresAt.Before(refreshThreshold) && entry.metrics.HitCount.Load() > 10 {
			tokensToRefresh = append(tokensToRefresh, tokenID)
		}
	}

	c.mu.RUnlock()

	// Refresh in background
	ctx := context.Background()
	for _, providerID := range providersToRefresh {
		go c.refreshProviderCache(ctx, providerID)
	}
	for _, tokenID := range tokensToRefresh {
		go c.refreshTokenCache(ctx, tokenID)
	}
}

// refreshProviderCache refreshes provider metrics from database.
func (c *CachedUsageTracker) refreshProviderCache(ctx context.Context, providerID string) {
	metrics, err := c.underlying.GetProviderUsage(ctx, providerID)
	if err != nil {
		c.logger.WarnLog("[CachedUsageTracker] Failed to refresh provider cache %s: %v", providerID, err)
		return
	}

	c.updateProviderCache(providerID, metrics)
	c.logger.DebugLog("[CachedUsageTracker] Refreshed provider cache: %s", providerID)
}

// refreshTokenCache refreshes token metrics from database.
func (c *CachedUsageTracker) refreshTokenCache(ctx context.Context, tokenID string) {
	// Need provider ID for token lookup - get from cache entry
	c.mu.RLock()
	entry, exists := c.tokenCache[tokenID]
	c.mu.RUnlock()

	if !exists {
		return
	}

	providerID := entry.providerID
	metrics, err := c.underlying.GetTokenUsage(ctx, providerID, tokenID)
	if err != nil {
		c.logger.WarnLog("[CachedUsageTracker] Failed to refresh token cache %s: %v", tokenID, err)
		return
	}

	c.updateTokenCache(tokenID, providerID, metrics)
	c.logger.DebugLog("[CachedUsageTracker] Refreshed token cache: %s", tokenID)
}

// evictIfNeeded evicts cache entries if size limit is reached.
func (c *CachedUsageTracker) evictIfNeeded() {
	// Evict provider cache if needed
	if len(c.providerCache) > c.config.MaxProviderEntries {
		c.evictLRUProviderEntries()
	}

	// Evict token cache if needed
	if len(c.tokenCache) > c.config.MaxTokenEntries {
		c.evictLRUTokenEntries()
	}
}

// evictLRUProviderEntries removes least recently used provider entries.
func (c *CachedUsageTracker) evictLRUProviderEntries() {
	type entry struct {
		providerID string
		lastAccess time.Time
	}

	var entries []entry
	for providerID, cached := range c.providerCache {
		entries = append(entries, entry{
			providerID: providerID,
			lastAccess: cached.CachedAt,
		})
	}

	// Sort by last access time
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].lastAccess.Before(entries[j].lastAccess)
	})

	// Remove oldest entries (10% of cache)
	toRemove := len(entries) / 10
	for i := 0; i < toRemove; i++ {
		delete(c.providerCache, entries[i].providerID)
	}

	c.logger.DebugLog("[CachedUsageTracker] Evicted %d provider entries (LRU)", toRemove)
}

// evictLRUTokenEntries removes least recently used token entries.
func (c *CachedUsageTracker) evictLRUTokenEntries() {
	type entry struct {
		tokenID    string
		lastAccess time.Time
	}

	var entries []entry
	for tokenID, cachedEntry := range c.tokenCache {
		entries = append(entries, entry{
			tokenID:    tokenID,
			lastAccess: cachedEntry.metrics.CachedAt,
		})
	}

	// Sort by last access time
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].lastAccess.Before(entries[j].lastAccess)
	})

	// Remove oldest entries (10% of cache)
	toRemove := len(entries) / 10
	for i := 0; i < toRemove; i++ {
		delete(c.tokenCache, entries[i].tokenID)
	}

	c.logger.DebugLog("[CachedUsageTracker] Evicted %d token entries (LRU)", toRemove)
}

// CacheStats represents cache statistics.
type CacheStats struct {
	ProviderCacheSize int     `json:"provider_cache_size"`
	TokenCacheSize    int     `json:"token_cache_size"`
	CacheHits         int64   `json:"cache_hits"`
	CacheMisses       int64   `json:"cache_misses"`
	CacheHitRate      float64 `json:"cache_hit_rate"`
	CacheErrors       int64   `json:"cache_errors"`
}

// GetCacheStats returns cache statistics.
func (c *CachedUsageTracker) GetCacheStats() CacheStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	hits := c.cacheHits.Load()
	misses := c.cacheMisses.Load()
	total := hits + misses
	hitRate := float64(0)
	if total > 0 {
		hitRate = float64(hits) / float64(total) * 100
	}

	return CacheStats{
		ProviderCacheSize: len(c.providerCache),
		TokenCacheSize:    len(c.tokenCache),
		CacheHits:         hits,
		CacheMisses:       misses,
		CacheHitRate:      hitRate,
		CacheErrors:       c.cacheErrors.Load(),
	}
}

// GetRequestHistory retrieves request history with optional filters and pagination.
// Delegates to underlying tracker as request history is not cached.
func (c *CachedUsageTracker) GetRequestHistory(ctx context.Context, filter *RequestHistoryFilter, page int, pageSize int) (*RequestHistoryResponse, error) {
	return c.underlying.GetRequestHistory(ctx, filter, page, pageSize)
}

// GetRequestHistoryByToken retrieves request history for a specific token.
// Delegates to underlying tracker as request history is not cached.
func (c *CachedUsageTracker) GetRequestHistoryByToken(ctx context.Context, tokenID string, filter *RequestHistoryFilter, page int, pageSize int) (*RequestHistoryResponse, error) {
	return c.underlying.GetRequestHistoryByToken(ctx, tokenID, filter, page, pageSize)
}

// GetRequestHistoryByModel retrieves request history for a specific model.
// Delegates to underlying tracker as request history is not cached.
func (c *CachedUsageTracker) GetRequestHistoryByModel(ctx context.Context, model string, filter *RequestHistoryFilter, page int, pageSize int) (*RequestHistoryResponse, error) {
	return c.underlying.GetRequestHistoryByModel(ctx, model, filter, page, pageSize)
}

// GetRequestHistorySummary returns aggregated statistics for request history.
// Delegates to underlying tracker as request history is not cached.
func (c *CachedUsageTracker) GetRequestHistorySummary(ctx context.Context, tokenID string, model string, startTime int64, endTime int64) (*RequestHistorySummary, error) {
	return c.underlying.GetRequestHistorySummary(ctx, tokenID, model, startTime, endTime)
}

// DeleteRequestHistory deletes request history records matching the filter.
// Delegates to underlying tracker as request history is not cached.
func (c *CachedUsageTracker) DeleteRequestHistory(ctx context.Context, filter *RequestHistoryFilter) (int64, error) {
	return c.underlying.DeleteRequestHistory(ctx, filter)
}
