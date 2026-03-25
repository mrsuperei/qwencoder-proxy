package ratelimit

import (
	"database/sql"
	"fmt"
	"sync"
	"time"

	"github.com/sunbankio/qwencoder-proxy/internal/logging"
)

// SystemConfig holds configuration for integrated rate limit system.
type SystemConfig struct {
	// AsyncRecording configures async usage recording
	AsyncRecording *AsyncUsageRecorderConfig

	// Caching configures in-memory caching
	Caching *CacheConfig

	// DBNotification configures database change notification
	DBNotification *DBNotificationConfig
}

// DBNotificationConfig configures database change notification.
type DBNotificationConfig struct {
	// Enabled enables database change monitoring
	Enabled bool

	// PollingInterval is the interval between database checks
	PollingInterval time.Duration
}

// DefaultDBNotificationConfig returns default database notification configuration.
func DefaultDBNotificationConfig() *DBNotificationConfig {
	return &DBNotificationConfig{
		Enabled:         false,
		PollingInterval: 5 * time.Second,
	}
}

// DefaultSystemConfig returns default system configuration.
func DefaultSystemConfig() *SystemConfig {
	return &SystemConfig{
		AsyncRecording: DefaultAsyncUsageRecorderConfig(),
		Caching:        DefaultCacheConfig(),
		DBNotification: DefaultDBNotificationConfig(),
	}
}

// SystemMetrics holds metrics for the integrated system.
type SystemMetrics struct {
	// StartTime is when the system was started
	StartTime time.Time

	// Uptime is how long the system has been running
	Uptime time.Duration

	// RequestsProcessed is the total number of requests processed
	RequestsProcessed int64

	// RequestsRejected is the total number of requests rejected
	RequestsRejected int64

	// AsyncRecording metrics
	AsyncRecording AsyncUsageRecorderMetrics

	// Cache metrics
	CacheHits   int64
	CacheMisses int64
	CacheErrors int64

	// Database notification metrics
	DBChecksPerformed int64
	DBChangesDetected int64
}

// IntegratedRateLimitSystem integrates all rate limiting components.
type IntegratedRateLimitSystem struct {
	// Core components
	quotaManager  *QuotaManager
	asyncRecorder *AsyncUsageRecorder
	cachedTracker UsageTracker
	invalidator   *CacheInvalidator
	dbNotifier    *DatabaseChangeNotifier

	// Configuration
	config *SystemConfig

	// Metrics
	metrics      SystemMetrics
	metricsMutex sync.RWMutex

	// Lifecycle
	started bool
	stopped bool
	mu      sync.RWMutex

	// Dependencies
	db     *sql.DB
	logger logging.Logger
}

// NewIntegratedRateLimitSystem creates a new integrated rate limit system.
func NewIntegratedRateLimitSystem(
	db *sql.DB,
	logger logging.Logger,
	config *SystemConfig,
) (*IntegratedRateLimitSystem, error) {
	if config == nil {
		config = DefaultSystemConfig()
	}

	// Step 1: Create base usage tracker (database) with shared connection
	baseTracker, err := NewUsageTracker(db, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create usage tracker: %w", err)
	}

	// Step 2: Create cache invalidator
	invalidator := NewCacheInvalidator(logger)

	// Step 3: Create cached usage tracker with invalidator
	cachedTracker := NewCachedUsageTracker(baseTracker, config.Caching, logger, invalidator)

	// Step 4: Create async usage recorder
	asyncRecorder := NewAsyncUsageRecorder(cachedTracker, logger, config.AsyncRecording)

	// Step 5: Create quota manager with cached tracker
	quotaManager := NewQuotaManager(cachedTracker, logger, db, nil, nil, invalidator)

	// Step 6: Create database change notifier (optional)
	var dbNotifier *DatabaseChangeNotifier
	if config.DBNotification.Enabled {
		dbNotifier = NewDatabaseChangeNotifier(db, logger, invalidator, config.DBNotification.PollingInterval)
	}

	system := &IntegratedRateLimitSystem{
		quotaManager:  quotaManager,
		asyncRecorder: asyncRecorder,
		cachedTracker: cachedTracker,
		invalidator:   invalidator,
		dbNotifier:    dbNotifier,
		config:        config,
		metrics: SystemMetrics{
			StartTime: time.Now(),
		},
		db:     db,
		logger: logger,
	}

	logger.InfoLog("[IntegratedRateLimitSystem] System created successfully")
	return system, nil
}

// Start starts all components of the integrated system.
func (s *IntegratedRateLimitSystem) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.started {
		return fmt.Errorf("system already started")
	}

	s.logger.InfoLog("[IntegratedRateLimitSystem] Starting integrated system...")

	// Start database change notifier if enabled
	if s.dbNotifier != nil {
		s.dbNotifier.Start()
		s.logger.InfoLog("[IntegratedRateLimitSystem] Database change notifier started")
	}

	s.started = true
	s.metrics.StartTime = time.Now()

	s.logger.InfoLog("[IntegratedRateLimitSystem] System started successfully")
	return nil
}

// Stop stops all components of the integrated system gracefully.
func (s *IntegratedRateLimitSystem) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.stopped {
		return fmt.Errorf("system already stopped")
	}

	s.logger.InfoLog("[IntegratedRateLimitSystem] Stopping integrated system...")

	// Stop database change notifier first
	if s.dbNotifier != nil {
		if err := s.dbNotifier.Close(); err != nil {
			s.logger.ErrorLog("[IntegratedRateLimitSystem] Failed to stop database notifier: %v", err)
		}
		s.logger.InfoLog("[IntegratedRateLimitSystem] Database change notifier stopped")
	}

	// Stop async recorder
	if err := s.asyncRecorder.Close(); err != nil {
		s.logger.ErrorLog("[IntegratedRateLimitSystem] Failed to stop async recorder: %v", err)
	}
	s.logger.InfoLog("[IntegratedRateLimitSystem] Async recorder stopped")

	// Close quota manager
	if err := s.quotaManager.Close(); err != nil {
		s.logger.ErrorLog("[IntegratedRateLimitSystem] Failed to close quota manager: %v", err)
	}
	s.logger.InfoLog("[IntegratedRateLimitSystem] Quota manager closed")

	// Close cached tracker
	if err := s.cachedTracker.Close(); err != nil {
		s.logger.ErrorLog("[IntegratedRateLimitSystem] Failed to close cached tracker: %v", err)
	}
	s.logger.InfoLog("[IntegratedRateLimitSystem] Cached tracker closed")

	// Stop cache invalidator
	s.invalidator.Close()
	s.logger.InfoLog("[IntegratedRateLimitSystem] Cache invalidator stopped")

	s.stopped = true
	s.started = false
	s.metrics.Uptime = time.Since(s.metrics.StartTime)

	s.logger.InfoLog("[IntegratedRateLimitSystem] System stopped successfully (uptime: %v)", s.metrics.Uptime)
	return nil
}

// GetQuotaManager returns the quota manager.
func (s *IntegratedRateLimitSystem) GetQuotaManager() *QuotaManager {
	return s.quotaManager
}

// GetCacheInvalidator returns the cache invalidator.
func (s *IntegratedRateLimitSystem) GetCacheInvalidator() *CacheInvalidator {
	return s.invalidator
}

// GetAsyncRecorder returns the async usage recorder.
func (s *IntegratedRateLimitSystem) GetAsyncRecorder() *AsyncUsageRecorder {
	return s.asyncRecorder
}

// GetCachedTracker returns the cached usage tracker.
func (s *IntegratedRateLimitSystem) GetCachedTracker() UsageTracker {
	return s.cachedTracker
}

// GetMetrics returns the current system metrics.
func (s *IntegratedRateLimitSystem) GetMetrics() SystemMetrics {
	s.metricsMutex.RLock()
	defer s.metricsMutex.RUnlock()

	// Update uptime
	metrics := s.metrics
	metrics.Uptime = time.Since(metrics.StartTime)

	// Collect component metrics
	if s.asyncRecorder != nil {
		metrics.AsyncRecording = s.asyncRecorder.GetMetrics()
	}

	// Access cache metrics through cached tracker
	if cached, ok := s.cachedTracker.(*CachedUsageTracker); ok {
		metrics.CacheHits = cached.cacheHits.Load()
		metrics.CacheMisses = cached.cacheMisses.Load()
		metrics.CacheErrors = cached.cacheErrors.Load()
	}

	if s.dbNotifier != nil {
		stats := s.dbNotifier.GetStats()
		metrics.DBChecksPerformed = stats.ChecksPerformed
		metrics.DBChangesDetected = stats.ChangesDetected
	}

	return metrics
}

// RecordRequestProcessed increments the requests processed counter.
func (s *IntegratedRateLimitSystem) RecordRequestProcessed() {
	s.metricsMutex.Lock()
	defer s.metricsMutex.Unlock()
	s.metrics.RequestsProcessed++
}

// RecordRequestRejected increments the requests rejected counter.
func (s *IntegratedRateLimitSystem) RecordRequestRejected() {
	s.metricsMutex.Lock()
	defer s.metricsMutex.Unlock()
	s.metrics.RequestsRejected++
}

// IsStarted returns whether the system has been started.
func (s *IntegratedRateLimitSystem) IsStarted() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.started
}

// IsStopped returns whether the system has been stopped.
func (s *IntegratedRateLimitSystem) IsStopped() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.stopped
}
