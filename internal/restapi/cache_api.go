package restapi

import (
	"net/http"
	"strings"

	"github.com/sunbankio/qwencoder-proxy/internal/logging"
	"github.com/sunbankio/qwencoder-proxy/internal/ratelimit"
)

// CacheAPI handles cache management endpoints.
type CacheAPI struct {
	invalidator *ratelimit.CacheInvalidator
	logger      logging.Logger
}

// NewCacheAPI creates a new cache API handler.
func NewCacheAPI(invalidator *ratelimit.CacheInvalidator, logger logging.Logger) *CacheAPI {
	return &CacheAPI{
		invalidator: invalidator,
		logger:      logger,
	}
}

// RegisterRoutes registers cache management routes.
func (api *CacheAPI) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/cache/invalidate", api.handleInvalidateAll)
	mux.HandleFunc("/api/cache/invalidate/provider/", api.handleInvalidateProvider)
	mux.HandleFunc("/api/cache/invalidate/token/", api.handleInvalidateToken)
	mux.HandleFunc("/api/cache/stats", api.handleCacheStats)
}

// handleInvalidateAll handles POST /api/cache/invalidate
func (api *CacheAPI) handleInvalidateAll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed")
		return
	}

	// Publish invalidation event for all cache
	api.invalidator.PublishInvalidation(ratelimit.InvalidationAll, "", "api")

	api.logger.InfoLog("[CacheAPI] Cache invalidation requested via API")

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"message": "Cache invalidated successfully",
	})
}

// handleInvalidateProvider handles POST /api/cache/invalidate/provider/:provider
func (api *CacheAPI) handleInvalidateProvider(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed")
		return
	}

	// Extract provider ID from path
	path := strings.TrimPrefix(r.URL.Path, "/api/cache/invalidate/provider/")
	if path == "" {
		WriteError(w, http.StatusBadRequest, "invalid_request", "Provider ID required")
		return
	}

	// Handle trailing slash
	path = strings.TrimSuffix(path, "/")

	// Publish invalidation event for provider
	api.invalidator.PublishInvalidation(ratelimit.InvalidationProvider, path, "api")

	api.logger.InfoLog("[CacheAPI] Provider cache invalidation requested via API: %s", path)

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"message":  "Provider cache invalidated successfully",
		"provider": path,
	})
}

// handleInvalidateToken handles POST /api/cache/invalidate/token/:token
func (api *CacheAPI) handleInvalidateToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed")
		return
	}

	// Extract token ID from path
	path := strings.TrimPrefix(r.URL.Path, "/api/cache/invalidate/token/")
	if path == "" {
		WriteError(w, http.StatusBadRequest, "invalid_request", "Token ID required")
		return
	}

	// Handle trailing slash
	path = strings.TrimSuffix(path, "/")

	// Publish invalidation event for token
	api.invalidator.PublishInvalidation(ratelimit.InvalidationToken, path, "api")

	api.logger.InfoLog("[CacheAPI] Token cache invalidation requested via API: %s", path)

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"message": "Token cache invalidated successfully",
		"token":   path,
	})
}

// handleCacheStats handles GET /api/cache/stats
func (api *CacheAPI) handleCacheStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed")
		return
	}

	// Get cache invalidator stats
	stats := api.invalidator.GetStats()

	WriteJSON(w, http.StatusOK, stats)
}
