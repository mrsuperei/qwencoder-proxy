// Package auth provides authentication and token management functionality.
// This file contains the proxy health tracker component.
package token

import (
	"sync"
	"time"

	"github.com/sunbankio/qwencoder-proxy/internal/logging"
)

// ProxyHealthTracker monitors and tracks the health status of proxy connections.
// This enables proactive proxy management and fallback strategies.
//
// The tracker maintains separate health records for each token, allowing
// per-token proxy health tracking independent of token health.
//
// Health Score Algorithm:
// - Initial score: 1.0
// - On success: score = min(1.0, score + 0.1)
// - On failure: score = max(0.0, score - 0.2)
// - Consecutive failures > 5: mark proxy as unhealthy
type ProxyHealthTracker struct {
	healthStore         map[string]*ProxyHealth // key: tokenID
	storeLock           sync.RWMutex
	logger              logging.Logger
	failureThreshold    int           // Default: 5
	healthCheckInterval time.Duration // Default: 5 minutes
}

// NewProxyHealthTracker creates a new ProxyHealthTracker.
//
// Parameters:
//   - logger: Logger for tracker operations
//   - failureThreshold: Number of consecutive failures before marking unhealthy (default: 5)
//   - healthCheckInterval: Interval for periodic health checks (default: 5 minutes)
func NewProxyHealthTracker(logger logging.Logger, failureThreshold int, healthCheckInterval time.Duration) *ProxyHealthTracker {
	if failureThreshold <= 0 {
		failureThreshold = 5
	}
	if healthCheckInterval <= 0 {
		healthCheckInterval = 5 * time.Minute
	}

	return &ProxyHealthTracker{
		healthStore:         make(map[string]*ProxyHealth),
		logger:              logger,
		failureThreshold:    failureThreshold,
		healthCheckInterval: healthCheckInterval,
	}
}

// UpdateHealth updates the health status for a token's proxy connection.
//
// This method:
// - Gets or creates a ProxyHealth record for the tokenID
// - Updates the LastCheck timestamp
// - Updates IsHealthy status based on the healthy parameter
// - Updates LastError if an error is provided
// - Increments/decrements ConsecutiveFailures
// - Marks proxy as unhealthy if consecutive failures exceed threshold
// - Calculates and returns the new health score
//
// Health Score Calculation:
// - On success (healthy=true): score = min(1.0, score + 0.1)
// - On failure (healthy=false): score = max(0.0, score - 0.2)
//
// Returns the updated health score (0.0-1.0).
func (pht *ProxyHealthTracker) UpdateHealth(tokenID string, healthy bool, err error) float64 {
	pht.storeLock.Lock()
	defer pht.storeLock.Unlock()

	// Get or create health record
	health, exists := pht.healthStore[tokenID]
	if !exists {
		health = &ProxyHealth{
			LastCheck:           time.Now().UnixMilli(),
			IsHealthy:           true,
			ConsecutiveFailures: 0,
			AverageLatencyMs:    0,
			HealthScore:         1.0, // Default to healthy
		}
		pht.healthStore[tokenID] = health
	}

	// Update timestamp
	health.LastCheck = time.Now().UnixMilli()

	// Update health status
	if healthy {
		health.IsHealthy = true
		health.ConsecutiveFailures = 0
		health.LastError = ""
		// Reset health score on success (recovery)
		health.HealthScore = 1.0

		pht.logger.DebugLog("Proxy health updated for token %s: healthy, score: %.2f", tokenID, health.HealthScore)
		return health.HealthScore
	}

	// On failure
	health.IsHealthy = false
	health.ConsecutiveFailures++

	if err != nil {
		health.LastError = err.Error()
	}

	// Check if consecutive failures exceed threshold
	if health.ConsecutiveFailures > pht.failureThreshold {
		pht.logger.WarnLog("Proxy for token %s marked unhealthy after %d consecutive failures",
			tokenID, health.ConsecutiveFailures)
	}

	// Decrease health score on failure
	health.HealthScore = max(0.0, health.HealthScore-0.2)

	pht.logger.DebugLog("Proxy health updated for token %s: unhealthy, score: %.2f, consecutive failures: %d",
		tokenID, health.HealthScore, health.ConsecutiveFailures)

	return health.HealthScore
}

// GetHealthScore returns the current health score for a token's proxy.
// Returns 1.0 (healthy) if no health record exists.
func (pht *ProxyHealthTracker) GetHealthScore(tokenID string) float64 {
	pht.storeLock.RLock()
	defer pht.storeLock.RUnlock()

	health, exists := pht.healthStore[tokenID]
	if !exists {
		return 1.0 // Default to healthy
	}

	return health.HealthScore
}

// GetHealthStatus returns the full health status for a token's proxy.
// Returns nil if no health record exists.
func (pht *ProxyHealthTracker) GetHealthStatus(tokenID string) *ProxyHealth {
	pht.storeLock.RLock()
	defer pht.storeLock.RUnlock()

	health, exists := pht.healthStore[tokenID]
	if !exists {
		return nil
	}

	// Return a copy to prevent external modifications
	copy := &ProxyHealth{
		LastCheck:           health.LastCheck,
		IsHealthy:           health.IsHealthy,
		LastError:           health.LastError,
		ConsecutiveFailures: health.ConsecutiveFailures,
		AverageLatencyMs:    health.AverageLatencyMs,
		HealthScore:         health.HealthScore,
	}
	return copy
}

// RecordLatency records a latency measurement for a token's proxy.
// Updates the running average latency.
func (pht *ProxyHealthTracker) RecordLatency(tokenID string, latencyMs int) {
	pht.storeLock.Lock()
	defer pht.storeLock.Unlock()

	health, exists := pht.healthStore[tokenID]
	if !exists {
		health = &ProxyHealth{
			LastCheck:           time.Now().UnixMilli(),
			IsHealthy:           true,
			ConsecutiveFailures: 0,
			AverageLatencyMs:    latencyMs,
			HealthScore:         1.0, // Default to healthy
		}
		pht.healthStore[tokenID] = health
		pht.logger.DebugLog("Recorded initial latency for token %s: %dms", tokenID, latencyMs)
		return
	}

	// Update running average
	// New average = ((old average * (n-1)) + new value) / n
	// For simplicity, we'll use a simple weighted average
	health.AverageLatencyMs = (health.AverageLatencyMs + latencyMs) / 2
	health.LastCheck = time.Now().UnixMilli()

	pht.logger.DebugLog("Updated latency for token %s: %dms (average: %dms)",
		tokenID, latencyMs, health.AverageLatencyMs)
}

// IsHealthy returns whether the proxy for a token is currently healthy.
// Returns true if no health record exists (default to healthy).
func (pht *ProxyHealthTracker) IsHealthy(tokenID string) bool {
	pht.storeLock.RLock()
	defer pht.storeLock.RUnlock()

	health, exists := pht.healthStore[tokenID]
	if !exists {
		return true // Default to healthy
	}

	return health.IsHealthy
}

// ClearHealth removes the health record for a token.
// Useful when a token is removed or proxy configuration changes.
func (pht *ProxyHealthTracker) ClearHealth(tokenID string) {
	pht.storeLock.Lock()
	defer pht.storeLock.Unlock()

	delete(pht.healthStore, tokenID)
	pht.logger.DebugLog("Cleared health record for token %s", tokenID)
}

// ClearAllHealth removes all health records.
// Useful when resetting the tracker.
func (pht *ProxyHealthTracker) ClearAllHealth() {
	pht.storeLock.Lock()
	defer pht.storeLock.Unlock()

	pht.healthStore = make(map[string]*ProxyHealth)
	pht.logger.DebugLog("Cleared all health records")
}

// GetTrackedTokenCount returns the number of tokens being tracked.
func (pht *ProxyHealthTracker) GetTrackedTokenCount() int {
	pht.storeLock.RLock()
	defer pht.storeLock.RUnlock()
	return len(pht.healthStore)
}

// GetUnhealthyTokenCount returns the number of tokens with unhealthy proxies.
func (pht *ProxyHealthTracker) GetUnhealthyTokenCount() int {
	pht.storeLock.RLock()
	defer pht.storeLock.RUnlock()

	count := 0
	for _, health := range pht.healthStore {
		if !health.IsHealthy {
			count++
		}
	}
	return count
}

// ListUnhealthyTokens returns a list of token IDs with unhealthy proxies.
func (pht *ProxyHealthTracker) ListUnhealthyTokens() []string {
	pht.storeLock.RLock()
	defer pht.storeLock.RUnlock()

	unhealthy := make([]string, 0)
	for tokenID, health := range pht.healthStore {
		if !health.IsHealthy {
			unhealthy = append(unhealthy, tokenID)
		}
	}
	return unhealthy
}
