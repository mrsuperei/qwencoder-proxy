package ratelimit

import (
	"context"
	"database/sql"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sunbankio/qwencoder-proxy/internal/logging"
)

// DatabaseChangeNotifier monitors the database for changes and triggers cache invalidation.
type DatabaseChangeNotifier struct {
	db              *sql.DB
	logger          logging.Logger
	pollingInterval time.Duration
	invalidator     *CacheInvalidator
	lastCheckTime   time.Time
	lastTokenCheck  time.Time
	lastConfigCheck time.Time

	// Context for shutdown
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// Metrics
	checksPerformed atomic.Int64
	changesDetected atomic.Int64
}

// NewDatabaseChangeNotifier creates a new database change notifier.
func NewDatabaseChangeNotifier(db *sql.DB, logger logging.Logger, invalidator *CacheInvalidator, pollingInterval time.Duration) *DatabaseChangeNotifier {
	if pollingInterval == 0 {
		pollingInterval = 5 * time.Second
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &DatabaseChangeNotifier{
		db:              db,
		logger:          logger,
		pollingInterval: pollingInterval,
		invalidator:     invalidator,
		lastCheckTime:   time.Now(),
		lastTokenCheck:  time.Now(),
		lastConfigCheck: time.Now(),
		ctx:             ctx,
		cancel:          cancel,
	}
}

// Start begins monitoring the database for changes.
func (dcn *DatabaseChangeNotifier) Start() {
	dcn.logger.InfoLog("[DatabaseChangeNotifier] Starting database change monitoring with interval: %v", dcn.pollingInterval)

	dcn.wg.Add(1)
	go dcn.monitorDatabase()
}

// monitorDatabase runs the database monitoring loop.
func (dcn *DatabaseChangeNotifier) monitorDatabase() {
	defer dcn.wg.Done()

	ticker := time.NewTicker(dcn.pollingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-dcn.ctx.Done():
			dcn.logger.DebugLog("[DatabaseChangeNotifier] Database change monitoring stopped")
			return
		case <-ticker.C:
			dcn.checkForChanges()
		}
	}
}

// checkForChanges checks for database changes and triggers invalidation.
func (dcn *DatabaseChangeNotifier) checkForChanges() {
	dcn.checksPerformed.Add(1)

	// Check for token changes
	dcn.checkTokenChanges()

	// Check for configuration changes
	dcn.checkConfigChanges()
}

// checkTokenChanges checks for changes in token-related tables.
func (dcn *DatabaseChangeNotifier) checkTokenChanges() {
	// Check for new or updated tokens in provider_usage table
	query := `
		SELECT provider_id, MAX(updated_at) as updated_at
		FROM provider_usage
		WHERE updated_at > ?
		GROUP BY provider_id
	`

	now := time.Now()
	rows, err := dcn.db.Query(query, dcn.lastTokenCheck.UnixMilli())
	if err != nil {
		dcn.logger.WarnLog("[DatabaseChangeNotifier] Failed to check token changes: %v", err)
		return
	}
	defer rows.Close()

	var providersChanged []string
	for rows.Next() {
		var providerID string
		var updatedAt int64
		if err := rows.Scan(&providerID, &updatedAt); err != nil {
			dcn.logger.WarnLog("[DatabaseChangeNotifier] Failed to scan token change: %v", err)
			continue
		}
		providersChanged = append(providersChanged, providerID)
	}

	// Invalidate cache for changed providers
	for _, providerID := range providersChanged {
		dcn.invalidator.PublishInvalidation(InvalidationProvider, providerID, "database")
		dcn.changesDetected.Add(1)
		dcn.logger.DebugLog("[DatabaseChangeNotifier] Detected token change for provider: %s", providerID)
	}

	dcn.lastTokenCheck = now
}

// checkConfigChanges checks for changes in configuration.
func (dcn *DatabaseChangeNotifier) checkConfigChanges() {
	// For now, we'll trigger config invalidation periodically
	// In a production system, this would check a config table for changes
	now := time.Now()

	// Check if it's time to invalidate config cache (every 30 seconds)
	if now.Sub(dcn.lastConfigCheck) > 30*time.Second {
		dcn.invalidator.PublishInvalidation(InvalidationConfig, "", "database")
		dcn.changesDetected.Add(1)
		dcn.lastConfigCheck = now
		dcn.logger.DebugLog("[DatabaseChangeNotifier] Triggered config cache invalidation")
	}
}

// GetStats returns statistics about the database change notifier.
func (dcn *DatabaseChangeNotifier) GetStats() DatabaseChangeNotifierStats {
	return DatabaseChangeNotifierStats{
		ChecksPerformed: dcn.checksPerformed.Load(),
		ChangesDetected: dcn.changesDetected.Load(),
		LastCheckTime:   dcn.lastCheckTime,
		PollingInterval: dcn.pollingInterval,
	}
}

// Close stops the database change notifier.
func (dcn *DatabaseChangeNotifier) Close() error {
	dcn.logger.InfoLog("[DatabaseChangeNotifier] Stopping database change monitoring")
	dcn.cancel()
	dcn.wg.Wait()
	dcn.logger.InfoLog("[DatabaseChangeNotifier] Database change monitoring stopped")
	return nil
}

// DatabaseChangeNotifierStats represents statistics for the database change notifier.
type DatabaseChangeNotifierStats struct {
	ChecksPerformed int64         `json:"checks_performed"`
	ChangesDetected int64         `json:"changes_detected"`
	LastCheckTime   time.Time     `json:"last_check_time"`
	PollingInterval time.Duration `json:"polling_interval"`
}
