package ratelimit

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/sunbankio/qwencoder-proxy/internal/logging"
)

// HealthStatus represents the overall health status.
type HealthStatus string

const (
	// HealthStatusHealthy indicates the system is healthy
	HealthStatusHealthy HealthStatus = "healthy"
	// HealthStatusDegraded indicates the system is degraded but functional
	HealthStatusDegraded HealthStatus = "degraded"
	// HealthStatusUnhealthy indicates the system is unhealthy
	HealthStatusUnhealthy HealthStatus = "unhealthy"
)

// ComponentHealth represents the health status of a component.
type ComponentHealth struct {
	Name      string        `json:"name"`
	Status    HealthStatus  `json:"status"`
	Message   string        `json:"message,omitempty"`
	Timestamp time.Time     `json:"timestamp"`
	Duration  time.Duration `json:"duration,omitempty"`
}

// HealthCheckResponse represents the overall health check response.
type HealthCheckResponse struct {
	Status     HealthStatus      `json:"status"`
	Timestamp  time.Time         `json:"timestamp"`
	Components []ComponentHealth `json:"components"`
	Version    string            `json:"version,omitempty"`
}

// HealthChecker performs health checks on rate limit system components.
type HealthChecker struct {
	// System components
	system           *IntegratedRateLimitSystem
	usageTracker     UsageTracker
	asyncRecorder    *AsyncUsageRecorder
	cacheInvalidator *CacheInvalidator
	dbNotifier       *DatabaseChangeNotifier

	// Health check configuration
	checkInterval time.Duration
	timeout       time.Duration

	// Last health check results
	lastResults  map[string]ComponentHealth
	resultsMutex sync.RWMutex

	// Dependencies
	logger logging.Logger
}

// NewHealthChecker creates a new health checker.
func NewHealthChecker(
	system *IntegratedRateLimitSystem,
	usageTracker UsageTracker,
	asyncRecorder *AsyncUsageRecorder,
	cacheInvalidator *CacheInvalidator,
	dbNotifier *DatabaseChangeNotifier,
	logger logging.Logger,
) *HealthChecker {
	return &HealthChecker{
		system:           system,
		usageTracker:     usageTracker,
		asyncRecorder:    asyncRecorder,
		cacheInvalidator: cacheInvalidator,
		dbNotifier:       dbNotifier,
		checkInterval:    30 * time.Second,
		timeout:          5 * time.Second,
		lastResults:      make(map[string]ComponentHealth),
		logger:           logger,
	}
}

// Check performs a health check on all components.
func (hc *HealthChecker) Check() HealthCheckResponse {
	startTime := time.Now()

	response := HealthCheckResponse{
		Status:     HealthStatusHealthy,
		Timestamp:  startTime,
		Components: []ComponentHealth{},
	}

	// Check system health
	if hc.system != nil {
		component := hc.checkSystemHealth()
		response.Components = append(response.Components, component)
		hc.updateOverallStatus(&response.Status, component.Status)
	}

	// Check usage tracker health
	if hc.usageTracker != nil {
		component := hc.checkUsageTrackerHealth()
		response.Components = append(response.Components, component)
		hc.updateOverallStatus(&response.Status, component.Status)
	}

	// Check async recorder health
	if hc.asyncRecorder != nil {
		component := hc.checkAsyncRecorderHealth()
		response.Components = append(response.Components, component)
		hc.updateOverallStatus(&response.Status, component.Status)
	}

	// Check cache invalidator health
	if hc.cacheInvalidator != nil {
		component := hc.checkCacheInvalidatorHealth()
		response.Components = append(response.Components, component)
		hc.updateOverallStatus(&response.Status, component.Status)
	}

	// Check database notifier health
	if hc.dbNotifier != nil {
		component := hc.checkDBNotifierHealth()
		response.Components = append(response.Components, component)
		hc.updateOverallStatus(&response.Status, component.Status)
	}

	// Store results
	hc.resultsMutex.Lock()
	for _, comp := range response.Components {
		hc.lastResults[comp.Name] = comp
	}
	hc.resultsMutex.Unlock()

	response.Timestamp = time.Now()
	return response
}

// checkSystemHealth checks the integrated system health.
func (hc *HealthChecker) checkSystemHealth() ComponentHealth {
	startTime := time.Now()

	if hc.system == nil {
		return ComponentHealth{
			Name:      "system",
			Status:    HealthStatusUnhealthy,
			Message:   "system is nil",
			Timestamp: time.Now(),
		}
	}

	status := HealthStatusHealthy
	message := "system operational"

	if !hc.system.IsStarted() {
		status = HealthStatusDegraded
		message = "system not started"
	} else if hc.system.IsStopped() {
		status = HealthStatusUnhealthy
		message = "system stopped"
	}

	return ComponentHealth{
		Name:      "system",
		Status:    status,
		Message:   message,
		Timestamp: time.Now(),
		Duration:  time.Since(startTime),
	}
}

// checkUsageTrackerHealth checks the usage tracker health.
func (hc *HealthChecker) checkUsageTrackerHealth() ComponentHealth {
	startTime := time.Now()

	status := HealthStatusHealthy
	message := "usage tracker operational"

	// Check if we can get database connection
	db := hc.usageTracker.GetDB()
	if db == nil {
		status = HealthStatusUnhealthy
		message = "database connection is nil"
	} else {
		// Try to ping the database
		ctx, cancel := contextWithTimeout(5 * time.Second)
		defer cancel()
		err := db.PingContext(ctx)
		if err != nil {
			status = HealthStatusUnhealthy
			message = fmt.Sprintf("database ping failed: %v", err)
		}
	}

	return ComponentHealth{
		Name:      "usage_tracker",
		Status:    status,
		Message:   message,
		Timestamp: time.Now(),
		Duration:  time.Since(startTime),
	}
}

// checkAsyncRecorderHealth checks the async recorder health.
func (hc *HealthChecker) checkAsyncRecorderHealth() ComponentHealth {
	startTime := time.Now()

	status := HealthStatusHealthy
	message := "async recorder operational"

	if !hc.asyncRecorder.IsEnabled() {
		status = HealthStatusDegraded
		message = "async recorder disabled"
	} else {
		// Check queue size
		metrics := hc.asyncRecorder.GetMetrics()
		if metrics.QueueSize >= int64(metrics.QueueCapacity) {
			status = HealthStatusDegraded
			message = fmt.Sprintf("async queue full: %d/%d", metrics.QueueSize, metrics.QueueCapacity)
		}
	}

	return ComponentHealth{
		Name:      "async_recorder",
		Status:    status,
		Message:   message,
		Timestamp: time.Now(),
		Duration:  time.Since(startTime),
	}
}

// checkCacheInvalidatorHealth checks the cache invalidator health.
func (hc *HealthChecker) checkCacheInvalidatorHealth() ComponentHealth {
	startTime := time.Now()

	status := HealthStatusHealthy
	message := "cache invalidator operational"

	// Get stats to check if invalidator is responsive
	stats := hc.cacheInvalidator.GetStats()
	if stats.ActiveSubscribers == 0 {
		status = HealthStatusDegraded
		message = "no active subscribers"
	}

	return ComponentHealth{
		Name:      "cache_invalidator",
		Status:    status,
		Message:   message,
		Timestamp: time.Now(),
		Duration:  time.Since(startTime),
	}
}

// checkDBNotifierHealth checks the database notifier health.
func (hc *HealthChecker) checkDBNotifierHealth() ComponentHealth {
	startTime := time.Now()

	status := HealthStatusHealthy
	message := "database notifier operational"

	if hc.dbNotifier == nil {
		status = HealthStatusDegraded
		message = "database notifier not configured"
	}

	return ComponentHealth{
		Name:      "database_notifier",
		Status:    status,
		Message:   message,
		Timestamp: time.Now(),
		Duration:  time.Since(startTime),
	}
}

// updateOverallStatus updates the overall status based on component status.
func (hc *HealthChecker) updateOverallStatus(overall *HealthStatus, component HealthStatus) {
	// Unhealthy takes precedence
	if component == HealthStatusUnhealthy {
		*overall = HealthStatusUnhealthy
		return
	}

	// Degraded only if overall is still healthy
	if component == HealthStatusDegraded && *overall == HealthStatusHealthy {
		*overall = HealthStatusDegraded
	}
}

// GetLastResults returns the last health check results.
func (hc *HealthChecker) GetLastResults() map[string]ComponentHealth {
	hc.resultsMutex.RLock()
	defer hc.resultsMutex.RUnlock()

	// Return a copy
	results := make(map[string]ComponentHealth, len(hc.lastResults))
	for k, v := range hc.lastResults {
		results[k] = v
	}
	return results
}

// HTTPHandler returns an HTTP handler for health checks.
func (hc *HealthChecker) HTTPHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		response := hc.Check()

		w.Header().Set("Content-Type", "application/json")

		// Set appropriate status code based on health
		switch response.Status {
		case HealthStatusHealthy:
			w.WriteHeader(http.StatusOK)
		case HealthStatusDegraded:
			w.WriteHeader(http.StatusOK) // 200 but with degraded status
		case HealthStatusUnhealthy:
			w.WriteHeader(http.StatusServiceUnavailable)
		}

		json.NewEncoder(w).Encode(response)
	}
}

// ReadinessHandler returns an HTTP handler for readiness checks.
func (hc *HealthChecker) ReadinessHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Readiness is simpler - just check if system is started
		if hc.system == nil || !hc.system.IsStarted() {
			w.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(w).Encode(map[string]string{
				"status":  "not_ready",
				"message": "system not started",
			})
			return
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{
			"status":  "ready",
			"message": "system is ready",
		})
	}
}

// LivenessHandler returns an HTTP handler for liveness checks.
func (hc *HealthChecker) LivenessHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Liveness is simplest - just check if process is running
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{
			"status":  "alive",
			"message": "process is running",
		})
	}
}

// RegisterRoutes registers health check endpoints.
func (hc *HealthChecker) RegisterRoutes(mux *http.ServeMux, basePath string) {
	// Health endpoint
	mux.HandleFunc(basePath, hc.HTTPHandler())
	hc.logger.InfoLog("[HealthChecker] Health endpoint registered at %s", basePath)

	// Readiness endpoint
	mux.HandleFunc(basePath+"/ready", hc.ReadinessHandler())
	hc.logger.InfoLog("[HealthChecker] Readiness endpoint registered at %s/ready", basePath)

	// Liveness endpoint
	mux.HandleFunc(basePath+"/live", hc.LivenessHandler())
	hc.logger.InfoLog("[HealthChecker] Liveness endpoint registered at %s/live", basePath)
}

// contextWithTimeout creates a context with timeout.
func contextWithTimeout(timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), timeout)
}
