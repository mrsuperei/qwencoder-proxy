package ratelimit

import (
	"encoding/json"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sunbankio/qwencoder-proxy/internal/logging"
)

// Metrics holds all metrics for the rate limit system.
type Metrics struct {
	// Async recording metrics
	asyncJobsProcessed atomic.Int64
	asyncJobsFailed    atomic.Int64
	asyncQueueSize     atomic.Int64

	// Cache metrics
	cacheHits          atomic.Int64
	cacheMisses        atomic.Int64
	cacheErrors        atomic.Int64
	cacheInvalidations atomic.Int64

	// System metrics
	requestsProcessed atomic.Int64
	requestsRejected  atomic.Int64
	requestsAllowed   atomic.Int64

	// Database notification metrics
	dbChecksPerformed atomic.Int64
	dbChangesDetected atomic.Int64

	// Component-specific metrics
	componentMetrics map[string]interface{}
	metricsMutex     sync.RWMutex

	logger logging.Logger
}

// NewMetrics creates a new metrics collector.
func NewMetrics(logger logging.Logger) *Metrics {
	m := &Metrics{
		componentMetrics: make(map[string]interface{}),
		logger:           logger,
	}

	logger.InfoLog("[Metrics] Metrics collector initialized")
	return m
}

// RecordAsyncJobProcessed records a job being processed.
func (m *Metrics) RecordAsyncJobProcessed() {
	m.asyncJobsProcessed.Add(1)
}

// RecordAsyncJobFailed records a job failure.
func (m *Metrics) RecordAsyncJobFailed() {
	m.asyncJobsFailed.Add(1)
}

// SetAsyncQueueSize sets the current queue size.
func (m *Metrics) SetAsyncQueueSize(size int64) {
	m.asyncQueueSize.Store(size)
}

// RecordCacheHit records a cache hit.
func (m *Metrics) RecordCacheHit() {
	m.cacheHits.Add(1)
}

// RecordCacheMiss records a cache miss.
func (m *Metrics) RecordCacheMiss() {
	m.cacheMisses.Add(1)
}

// RecordCacheError records a cache error.
func (m *Metrics) RecordCacheError() {
	m.cacheErrors.Add(1)
}

// RecordCacheInvalidation records a cache invalidation.
func (m *Metrics) RecordCacheInvalidation() {
	m.cacheInvalidations.Add(1)
}

// RecordRequestProcessed records a request being processed.
func (m *Metrics) RecordRequestProcessed() {
	m.requestsProcessed.Add(1)
}

// RecordRequestRejected records a request being rejected.
func (m *Metrics) RecordRequestRejected() {
	m.requestsRejected.Add(1)
}

// RecordRequestAllowed records a request being allowed.
func (m *Metrics) RecordRequestAllowed() {
	m.requestsAllowed.Add(1)
}

// RecordDBCheck records a database check.
func (m *Metrics) RecordDBCheck() {
	m.dbChecksPerformed.Add(1)
}

// RecordDBChange records a database change.
func (m *Metrics) RecordDBChange() {
	m.dbChangesDetected.Add(1)
}

// SetComponentMetric sets a component-specific metric.
func (m *Metrics) SetComponentMetric(name string, value interface{}) {
	m.metricsMutex.Lock()
	defer m.metricsMutex.Unlock()
	m.componentMetrics[name] = value
}

// GetMetricsJSON returns all metrics as JSON.
func (m *Metrics) GetMetricsJSON() ([]byte, error) {
	m.metricsMutex.RLock()
	defer m.metricsMutex.RUnlock()

	metricsData := map[string]interface{}{
		"timestamp": time.Now().Unix(),
		"async_recording": map[string]int64{
			"jobs_processed": m.asyncJobsProcessed.Load(),
			"jobs_failed":    m.asyncJobsFailed.Load(),
			"queue_size":     m.asyncQueueSize.Load(),
		},
		"cache": map[string]int64{
			"hits":          m.cacheHits.Load(),
			"misses":        m.cacheMisses.Load(),
			"errors":        m.cacheErrors.Load(),
			"invalidations": m.cacheInvalidations.Load(),
		},
		"requests": map[string]int64{
			"processed": m.requestsProcessed.Load(),
			"rejected":  m.requestsRejected.Load(),
			"allowed":   m.requestsAllowed.Load(),
		},
		"database": map[string]int64{
			"checks_performed": m.dbChecksPerformed.Load(),
			"changes_detected": m.dbChangesDetected.Load(),
		},
		"components": m.componentMetrics,
	}

	return json.Marshal(metricsData)
}

// MetricsHTTPHandler returns an HTTP handler that serves metrics as JSON.
func (m *Metrics) MetricsHTTPHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		data, err := m.GetMetricsJSON()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
		w.Write(data)
	}
}

// RegisterRoutes registers the metrics endpoint.
func (m *Metrics) RegisterRoutes(mux *http.ServeMux, path string) {
	mux.HandleFunc(path, m.MetricsHTTPHandler())
	m.logger.InfoLog("[Metrics] Metrics endpoint registered at %s", path)
}

// UpdateFromSystemMetrics updates metrics from system metrics.
func (m *Metrics) UpdateFromSystemMetrics(sysMetrics SystemMetrics) {
	// Update async metrics
	m.asyncJobsProcessed.Add(sysMetrics.AsyncRecording.JobsProcessed)
	m.asyncJobsFailed.Add(sysMetrics.AsyncRecording.JobsFailed)
	m.asyncQueueSize.Store(sysMetrics.AsyncRecording.QueueSize)

	// Update cache metrics
	m.cacheHits.Add(sysMetrics.CacheHits)
	m.cacheMisses.Add(sysMetrics.CacheMisses)
	m.cacheErrors.Add(sysMetrics.CacheErrors)

	// Update system metrics
	m.requestsProcessed.Add(sysMetrics.RequestsProcessed)
	m.requestsRejected.Add(sysMetrics.RequestsRejected)

	// Update DB notification metrics
	m.dbChecksPerformed.Add(sysMetrics.DBChecksPerformed)
	m.dbChangesDetected.Add(sysMetrics.DBChangesDetected)
}

// GetAsyncJobsProcessed returns the number of async jobs processed.
func (m *Metrics) GetAsyncJobsProcessed() int64 {
	return m.asyncJobsProcessed.Load()
}

// GetAsyncJobsFailed returns the number of async jobs failed.
func (m *Metrics) GetAsyncJobsFailed() int64 {
	return m.asyncJobsFailed.Load()
}

// GetCacheHits returns the number of cache hits.
func (m *Metrics) GetCacheHits() int64 {
	return m.cacheHits.Load()
}

// GetCacheMisses returns the number of cache misses.
func (m *Metrics) GetCacheMisses() int64 {
	return m.cacheMisses.Load()
}

// GetRequestsProcessed returns the number of requests processed.
func (m *Metrics) GetRequestsProcessed() int64 {
	return m.requestsProcessed.Load()
}

// GetRequestsRejected returns the number of requests rejected.
func (m *Metrics) GetRequestsRejected() int64 {
	return m.requestsRejected.Load()
}

// Reset resets all metrics to zero.
func (m *Metrics) Reset() {
	m.asyncJobsProcessed.Store(0)
	m.asyncJobsFailed.Store(0)
	m.asyncQueueSize.Store(0)
	m.cacheHits.Store(0)
	m.cacheMisses.Store(0)
	m.cacheErrors.Store(0)
	m.cacheInvalidations.Store(0)
	m.requestsProcessed.Store(0)
	m.requestsRejected.Store(0)
	m.requestsAllowed.Store(0)
	m.dbChecksPerformed.Store(0)
	m.dbChangesDetected.Store(0)

	m.metricsMutex.Lock()
	m.componentMetrics = make(map[string]interface{})
	m.metricsMutex.Unlock()

	m.logger.InfoLog("[Metrics] All metrics reset to zero")
}

// Close cleans up resources.
func (m *Metrics) Close() error {
	m.logger.InfoLog("[Metrics] Metrics collector closed")
	return nil
}
