# Add Request History API Endpoints

## Overview

This task adds REST API endpoints to retrieve request history data from the `request_history` table. The `request_history` table already exists and is populated, but there are no API endpoints to query this data.

## Current State

- `request_history` table exists with columns: `id`, `token_id`, `model`, `request_count`, `input_tokens`, `output_tokens`, `timestamp`, `created_at`
- Data is being recorded in the table
- No REST API endpoints exist to retrieve this data
- Dashboard cannot display request history details

## Implementation Plan

### 1. Add Data Structures

**File: `internal/ratelimit/interfaces.go`**

Add the following data structures after the existing `ModelUsageMetrics` definition:

```go
// RequestHistoryRecord represents a single request history record.
type RequestHistoryRecord struct {
	ID           string `json:"id"`
	TokenID      string `json:"token_id"`
	Model        string `json:"model"`
	RequestCount int    `json:"request_count"`
	InputTokens  int    `json:"input_tokens"`
	OutputTokens int    `json:"output_tokens"`
	TotalTokens  int    `json:"total_tokens"`
	Timestamp    int64  `json:"timestamp"`
	CreatedAt    int64  `json:"created_at"`
}

// RequestHistoryFilter represents filters for request history queries.
type RequestHistoryFilter struct {
	TokenID     string `json:"token_id,omitempty"`
	Model       string `json:"model,omitempty"`
	StartTime   int64  `json:"start_time,omitempty"`
	EndTime     int64  `json:"end_time,omitempty"`
	MinTokens   int    `json:"min_tokens,omitempty"`
	MaxTokens   int    `json:"max_tokens,omitempty"`
}

// RequestHistoryResponse represents the paginated response for request history.
type RequestHistoryResponse struct {
	Records      []RequestHistoryRecord `json:"records"`
	TotalCount   int                   `json:"total_count"`
	Page         int                   `json:"page"`
	PageSize     int                   `json:"page_size"`
	TotalPages   int                   `json:"total_pages"`
	HasNext      bool                  `json:"has_next"`
	HasPrevious  bool                  `json:"has_previous"`
}

// RequestHistorySummary represents aggregated statistics for request history.
type RequestHistorySummary struct {
	TokenID        string  `json:"token_id,omitempty"`
	Model          string  `json:"model,omitempty"`
	TotalRequests  int     `json:"total_requests"`
	TotalInput     int     `json:"total_input_tokens"`
	TotalOutput    int     `json:"total_output_tokens"`
	TotalTokens    int     `json:"total_tokens"`
	AverageTokens  float64 `json:"average_tokens"`
	FirstRequest   int64   `json:"first_request"`
	LastRequest    int64   `json:"last_request"`
}
```

### 2. Add UsageTracker Interface Methods

**File: `internal/ratelimit/interfaces.go`**

Add the following methods to the `UsageTracker` interface:

```go
	// Request history tracking methods

	// GetRequestHistory retrieves request history with optional filters and pagination.
	// Returns paginated results with total count.
	GetRequestHistory(ctx context.Context, filter *RequestHistoryFilter, page int, pageSize int) (*RequestHistoryResponse, error)

	// GetRequestHistoryByToken retrieves request history for a specific token.
	GetRequestHistoryByToken(ctx context.Context, tokenID string, filter *RequestHistoryFilter, page int, pageSize int) (*RequestHistoryResponse, error)

	// GetRequestHistoryByModel retrieves request history for a specific model.
	GetRequestHistoryByModel(ctx context.Context, model string, filter *RequestHistoryFilter, page int, pageSize int) (*RequestHistoryResponse, error)

	// GetRequestHistorySummary returns aggregated statistics for request history.
	GetRequestHistorySummary(ctx context.Context, tokenID string, model string, startTime int64, endTime int64) (*RequestHistorySummary, error)

	// DeleteRequestHistory deletes request history records matching the filter.
	DeleteRequestHistory(ctx context.Context, filter *RequestHistoryFilter) (int64, error)
```

### 3. Implement UsageTracker Methods

**File: `internal/ratelimit/usage_tracker.go`**

Add the following implementations at the end of the file (before the closing brace):

```go
// GetRequestHistory retrieves request history with optional filters and pagination.
func (ut *usageTrackerImpl) GetRequestHistory(
	ctx context.Context,
	filter *RequestHistoryFilter,
	page int,
	pageSize int,
) (*RequestHistoryResponse, error) {
	// Set default pagination values
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 1000 {
		pageSize = 100
	}
	offset := (page - 1) * pageSize

	// Build query with filters
	query := `
		SELECT id, token_id, model, request_count, input_tokens, output_tokens, timestamp, created_at
		FROM request_history
		WHERE 1=1
	`
	args := []interface{}{}
	argCount := 0

	if filter != nil {
		if filter.TokenID != "" {
			argCount++
			query += fmt.Sprintf(" AND token_id = $%d", argCount)
			args = append(args, filter.TokenID)
		}
		if filter.Model != "" {
			argCount++
			query += fmt.Sprintf(" AND model = $%d", argCount)
			args = append(args, filter.Model)
		}
		if filter.StartTime > 0 {
			argCount++
			query += fmt.Sprintf(" AND timestamp >= $%d", argCount)
			args = append(args, filter.StartTime)
		}
		if filter.EndTime > 0 {
			argCount++
			query += fmt.Sprintf(" AND timestamp <= $%d", argCount)
			args = append(args, filter.EndTime)
		}
		if filter.MinTokens > 0 {
			argCount++
			query += fmt.Sprintf(" AND (input_tokens + output_tokens) >= $%d", argCount)
			args = append(args, filter.MinTokens)
		}
		if filter.MaxTokens > 0 {
			argCount++
			query += fmt.Sprintf(" AND (input_tokens + output_tokens) <= $%d", argCount)
			args = append(args, filter.MaxTokens)
		}
	}

	// Get total count
	countQuery := "SELECT COUNT(*) FROM request_history" + query[31:] // Skip "SELECT id, token_id..."
	var totalCount int
	if err := ut.db.QueryRowContext(ctx, countQuery, args...).Scan(&totalCount); err != nil {
		return nil, fmt.Errorf("failed to count request history: %w", err)
	}

	// Add ordering and pagination
	query += " ORDER BY timestamp DESC"
	argCount++
	query += fmt.Sprintf(" LIMIT $%d", argCount)
	args = append(args, pageSize)
	argCount++
	query += fmt.Sprintf(" OFFSET $%d", argCount)
	args = append(args, offset)

	// Execute query
	rows, err := ut.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query request history: %w", err)
	}
	defer rows.Close()

	var records []RequestHistoryRecord
	for rows.Next() {
		var record RequestHistoryRecord
		if err := rows.Scan(
			&record.ID,
			&record.TokenID,
			&record.Model,
			&record.RequestCount,
			&record.InputTokens,
			&record.OutputTokens,
			&record.Timestamp,
			&record.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan request history record: %w", err)
		}
		record.TotalTokens = record.InputTokens + record.OutputTokens
		records = append(records, record)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating request history: %w", err)
	}

	totalPages := (totalCount + pageSize - 1) / pageSize
	if totalPages == 0 {
		totalPages = 1
	}

	return &RequestHistoryResponse{
		Records:     records,
		TotalCount:  totalCount,
		Page:        page,
		PageSize:    pageSize,
		TotalPages:  totalPages,
		HasNext:     page < totalPages,
		HasPrevious: page > 1,
	}, nil
}

// GetRequestHistoryByToken retrieves request history for a specific token.
func (ut *usageTrackerImpl) GetRequestHistoryByToken(
	ctx context.Context,
	tokenID string,
	filter *RequestHistoryFilter,
	page int,
	pageSize int,
) (*RequestHistoryResponse, error) {
	if filter == nil {
		filter = &RequestHistoryFilter{}
	}
	filter.TokenID = tokenID
	return ut.GetRequestHistory(ctx, filter, page, pageSize)
}

// GetRequestHistoryByModel retrieves request history for a specific model.
func (ut *usageTrackerImpl) GetRequestHistoryByModel(
	ctx context.Context,
	model string,
	filter *RequestHistoryFilter,
	page int,
	pageSize int,
) (*RequestHistoryResponse, error) {
	if filter == nil {
		filter = &RequestHistoryFilter{}
	}
	filter.Model = model
	return ut.GetRequestHistory(ctx, filter, page, pageSize)
}

// GetRequestHistorySummary returns aggregated statistics for request history.
func (ut *usageTrackerImpl) GetRequestHistorySummary(
	ctx context.Context,
	tokenID string,
	model string,
	startTime int64,
	endTime int64,
) (*RequestHistorySummary, error) {
	query := `
		SELECT 
			COALESCE(SUM(request_count), 0) as total_requests,
			COALESCE(SUM(input_tokens), 0) as total_input,
			COALESCE(SUM(output_tokens), 0) as total_output,
			COALESCE(MIN(timestamp), 0) as first_request,
			COALESCE(MAX(timestamp), 0) as last_request
		FROM request_history
		WHERE 1=1
	`
	args := []interface{}{}
	argCount := 0

	if tokenID != "" {
		argCount++
		query += fmt.Sprintf(" AND token_id = $%d", argCount)
		args = append(args, tokenID)
	}
	if model != "" {
		argCount++
		query += fmt.Sprintf(" AND model = $%d", argCount)
		args = append(args, model)
	}
	if startTime > 0 {
		argCount++
		query += fmt.Sprintf(" AND timestamp >= $%d", argCount)
		args = append(args, startTime)
	}
	if endTime > 0 {
		argCount++
		query += fmt.Sprintf(" AND timestamp <= $%d", argCount)
		args = append(args, endTime)
	}

	var summary RequestHistorySummary
	summary.TokenID = tokenID
	summary.Model = model

	if err := ut.db.QueryRowContext(ctx, query, args...).Scan(
		&summary.TotalRequests,
		&summary.TotalInput,
		&summary.TotalOutput,
		&summary.FirstRequest,
		&summary.LastRequest,
	); err != nil {
		return nil, fmt.Errorf("failed to query request history summary: %w", err)
	}

	summary.TotalTokens = summary.TotalInput + summary.TotalOutput
	if summary.TotalRequests > 0 {
		summary.AverageTokens = float64(summary.TotalTokens) / float64(summary.TotalRequests)
	}

	return &summary, nil
}

// DeleteRequestHistory deletes request history records matching the filter.
func (ut *usageTrackerImpl) DeleteRequestHistory(
	ctx context.Context,
	filter *RequestHistoryFilter,
) (int64, error) {
	query := "DELETE FROM request_history WHERE 1=1"
	args := []interface{}{}
	argCount := 0

	if filter != nil {
		if filter.TokenID != "" {
			argCount++
			query += fmt.Sprintf(" AND token_id = $%d", argCount)
			args = append(args, filter.TokenID)
		}
		if filter.Model != "" {
			argCount++
			query += fmt.Sprintf(" AND model = $%d", argCount)
			args = append(args, filter.Model)
		}
		if filter.StartTime > 0 {
			argCount++
			query += fmt.Sprintf(" AND timestamp >= $%d", argCount)
			args = append(args, filter.StartTime)
		}
		if filter.EndTime > 0 {
			argCount++
			query += fmt.Sprintf(" AND timestamp <= $%d", argCount)
			args = append(args, filter.EndTime)
		}
	} else {
		// Safety: don't allow deleting all records without a filter
		return 0, fmt.Errorf("filter is required for delete operation")
	}

	result, err := ut.db.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("failed to delete request history: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	return rowsAffected, nil
}
```

### 4. Add QuotaManager Wrapper Methods

**File: `internal/ratelimit/quota_manager.go`**

Add the following wrapper methods after the existing `ResetTokenErrors` method:

```go
// GetRequestHistory retrieves request history with optional filters and pagination.
func (qm *QuotaManager) GetRequestHistory(ctx context.Context, filter *RequestHistoryFilter, page int, pageSize int) (*RequestHistoryResponse, error) {
	return qm.usageTracker.GetRequestHistory(ctx, filter, page, pageSize)
}

// GetRequestHistoryByToken retrieves request history for a specific token.
func (qm *QuotaManager) GetRequestHistoryByToken(ctx context.Context, tokenID string, filter *RequestHistoryFilter, page int, pageSize int) (*RequestHistoryResponse, error) {
	return qm.usageTracker.GetRequestHistoryByToken(ctx, tokenID, filter, page, pageSize)
}

// GetRequestHistoryByModel retrieves request history for a specific model.
func (qm *QuotaManager) GetRequestHistoryByModel(ctx context.Context, model string, filter *RequestHistoryFilter, page int, pageSize int) (*RequestHistoryResponse, error) {
	return qm.usageTracker.GetRequestHistoryByModel(ctx, model, filter, page, pageSize)
}

// GetRequestHistorySummary returns aggregated statistics for request history.
func (qm *QuotaManager) GetRequestHistorySummary(ctx context.Context, tokenID string, model string, startTime int64, endTime int64) (*RequestHistorySummary, error) {
	return qm.usageTracker.GetRequestHistorySummary(ctx, tokenID, model, startTime, endTime)
}

// DeleteRequestHistory deletes request history records matching the filter.
func (qm *QuotaManager) DeleteRequestHistory(ctx context.Context, filter *RequestHistoryFilter) (int64, error) {
	return qm.usageTracker.DeleteRequestHistory(ctx, filter)
}
```

### 5. Add REST API Endpoints

**File: `internal/restapi/rate_limit_api.go`**

Add the following route registrations to the `RegisterRoutes` method:

```go
	// Request history endpoints
	mux.HandleFunc("/api/ratelimit/request-history", api.handleRequestHistory)
	mux.HandleFunc("/api/ratelimit/request-history/", api.handleRequestHistoryPath)
	mux.HandleFunc("/api/ratelimit/request-history/summary", api.handleRequestHistorySummary)
	mux.HandleFunc("/api/ratelimit/request-history/delete", api.handleDeleteRequestHistory)
```

Add the following handler methods at the end of the file:

```go
// handleRequestHistory handles GET /api/ratelimit/request-history with query parameters.
// Query parameters:
//   - token_id: Filter by token ID
//   - model: Filter by model
//   - start_time: Start timestamp (milliseconds)
//   - end_time: End timestamp (milliseconds)
//   - min_tokens: Minimum total tokens
//   - max_tokens: Maximum total tokens
//   - page: Page number (default: 1)
//   - page_size: Page size (default: 100, max: 1000)
func (api *RateLimitAPI) handleRequestHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed")
		return
	}

	// Parse query parameters
	filter := &ratelimit.RequestHistoryFilter{
		TokenID:   r.URL.Query().Get("token_id"),
		Model:     r.URL.Query().Get("model"),
		StartTime: parseTimestamp(r.URL.Query().Get("start_time")),
		EndTime:   parseTimestamp(r.URL.Query().Get("end_time")),
	}
	
	if minTokens := r.URL.Query().Get("min_tokens"); minTokens != "" {
		if val, err := strconv.Atoi(minTokens); err == nil {
			filter.MinTokens = val
		}
	}
	if maxTokens := r.URL.Query().Get("max_tokens"); maxTokens != "" {
		if val, err := strconv.Atoi(maxTokens); err == nil {
			filter.MaxTokens = val
		}
	}

	// Parse pagination
	page := parsePage(r.URL.Query().Get("page"))
	pageSize := parsePageSize(r.URL.Query().Get("page_size"))

	// Get request history
	response, err := api.quotaManager.GetRequestHistory(r.Context(), filter, page, pageSize)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "query_failed", err.Error())
		return
	}

	WriteJSON(w, http.StatusOK, response)
}

// handleRequestHistoryPath handles GET /api/ratelimit/request-history/:token or /api/ratelimit/request-history/:token/:model.
func (api *RateLimitAPI) handleRequestHistoryPath(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed")
		return
	}

	// Extract path components
	path := strings.TrimPrefix(r.URL.Path, "/api/ratelimit/request-history/")
	if path == "" {
		WriteError(w, http.StatusBadRequest, "invalid_request", "Token ID required")
		return
	}
	path = strings.TrimSuffix(path, "/")

	parts := strings.Split(path, "/")
	if len(parts) == 0 {
		WriteError(w, http.StatusBadRequest, "invalid_request", "Token ID required")
		return
	}

	tokenID := parts[0]
	model := ""
	if len(parts) > 1 && parts[1] != "summary" && parts[1] != "delete" {
		model = parts[1]
	}

	// Parse query parameters for additional filtering
	filter := &ratelimit.RequestHistoryFilter{
		TokenID:   tokenID,
		Model:     model,
		StartTime: parseTimestamp(r.URL.Query().Get("start_time")),
		EndTime:   parseTimestamp(r.URL.Query().Get("end_time")),
	}
	
	if minTokens := r.URL.Query().Get("min_tokens"); minTokens != "" {
		if val, err := strconv.Atoi(minTokens); err == nil {
			filter.MinTokens = val
		}
	}
	if maxTokens := r.URL.Query().Get("max_tokens"); maxTokens != "" {
		if val, err := strconv.Atoi(maxTokens); err == nil {
			filter.MaxTokens = val
		}
	}

	// Parse pagination
	page := parsePage(r.URL.Query().Get("page"))
	pageSize := parsePageSize(r.URL.Query().Get("page_size"))

	// Get request history
	var response *ratelimit.RequestHistoryResponse
	var err error

	if model == "" {
		response, err = api.quotaManager.GetRequestHistoryByToken(r.Context(), tokenID, filter, page, pageSize)
	} else {
		response, err = api.quotaManager.GetRequestHistoryByModel(r.Context(), model, filter, page, pageSize)
	}

	if err != nil {
		WriteError(w, http.StatusInternalServerError, "query_failed", err.Error())
		return
	}

	WriteJSON(w, http.StatusOK, response)
}

// handleRequestHistorySummary handles GET /api/ratelimit/request-history/summary.
// Query parameters:
//   - token_id: Filter by token ID (optional)
//   - model: Filter by model (optional)
//   - start_time: Start timestamp (milliseconds, optional)
//   - end_time: End timestamp (milliseconds, optional)
func (api *RateLimitAPI) handleRequestHistorySummary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed")
		return
	}

	// Parse query parameters
	tokenID := r.URL.Query().Get("token_id")
	model := r.URL.Query().Get("model")
	startTime := parseTimestamp(r.URL.Query().Get("start_time"))
	endTime := parseTimestamp(r.URL.Query().Get("end_time"))

	// Get summary
	summary, err := api.quotaManager.GetRequestHistorySummary(r.Context(), tokenID, model, startTime, endTime)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "query_failed", err.Error())
		return
	}

	WriteJSON(w, http.StatusOK, summary)
}

// handleDeleteRequestHistory handles DELETE /api/ratelimit/request-history/delete.
// Query parameters (at least one required):
//   - token_id: Filter by token ID
//   - model: Filter by model
//   - start_time: Start timestamp (milliseconds)
//   - end_time: End timestamp (milliseconds)
func (api *RateLimitAPI) handleDeleteRequestHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodDelete {
		WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed")
		return
	}

	// Parse query parameters
	filter := &ratelimit.RequestHistoryFilter{
		TokenID:   r.URL.Query().Get("token_id"),
		Model:     r.URL.Query().Get("model"),
		StartTime: parseTimestamp(r.URL.Query().Get("start_time")),
		EndTime:   parseTimestamp(r.URL.Query().Get("end_time")),
	}

	// Validate that at least one filter is provided
	if filter.TokenID == "" && filter.Model == "" && filter.StartTime == 0 && filter.EndTime == 0 {
		WriteError(w, http.StatusBadRequest, "invalid_request", "At least one filter parameter is required")
		return
	}

	// Delete request history
	rowsAffected, err := api.quotaManager.DeleteRequestHistory(r.Context(), filter)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "delete_failed", err.Error())
		return
	}

	// Publish cache invalidation event
	if api.cacheInvalidator != nil && filter.TokenID != "" {
		api.cacheInvalidator.PublishInvalidation(ratelimit.InvalidationToken, filter.TokenID, "api")
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"message":      "request_history_deleted",
		"rows_affected": rowsAffected,
	})
}

// Helper functions for parsing query parameters

func parseTimestamp(value string) int64 {
	if value == "" {
		return 0
	}
	if val, err := strconv.ParseInt(value, 10, 64); err == nil {
		return val
	}
	return 0
}

func parsePage(value string) int {
	if value == "" {
		return 1
	}
	if val, err := strconv.Atoi(value); err == nil && val > 0 {
		return val
	}
	return 1
}

func parsePageSize(value string) int {
	if value == "" {
		return 100
	}
	if val, err := strconv.Atoi(value); err == nil && val > 0 && val <= 1000 {
		return val
	}
	return 100
}
```

Add the missing import at the top of the file:

```go
import (
	"strconv"
	// ... existing imports
)
```

### 6. Update Dashboard Endpoints File

**File: `web/dashboard/js/api/endpoints.js`**

Add the following endpoint definitions to the `ENDPOINTS` object:

```javascript
    // Rate limit management
    GET_RATE_LIMIT_CONFIGS: '/api/ratelimit/config',
    GET_RATE_LIMIT_CONFIG: (providerId) => `/api/ratelimit/config/${providerId}`,
    UPDATE_RATE_LIMIT_CONFIG: (providerId) => `/api/ratelimit/config/${providerId}`,
    
    // Usage tracking
    GET_ALL_USAGE: '/api/ratelimit/usage',
    GET_PROVIDER_USAGE: (providerId) => `/api/ratelimit/usage/${providerId}`,
    GET_TOKEN_USAGE: (providerId, tokenId) => `/api/ratelimit/usage/${providerId}/${tokenId}`,
    RESET_PROVIDER_USAGE: (providerId) => `/api/ratelimit/reset/${providerId}`,
    RESET_TOKEN_USAGE: (providerId, tokenId) => `/api/ratelimit/reset/${providerId}/${tokenId}`,
    
    // Model usage tracking
    GET_ALL_MODEL_USAGE: '/api/ratelimit/model-usage',
    GET_PROVIDER_MODEL_USAGE: (providerId) => `/api/ratelimit/model-usage/${providerId}`,
    GET_TOKEN_MODEL_USAGE: (providerId, tokenId) => `/api/ratelimit/model-usage/${providerId}/${tokenId}`,
    GET_MODEL_USAGE: (providerId, tokenId, model) => `/api/ratelimit/model-usage/${providerId}/${tokenId}/${model}`,
    
    // Error tracking
    GET_PROVIDER_ERRORS: (providerId) => `/api/ratelimit/errors?provider_id=${providerId}`,
    GET_TOKEN_ERRORS: (providerId, tokenId) => `/api/ratelimit/errors/${providerId}/${tokenId}`,
    RESET_TOKEN_ERRORS: (providerId, tokenId) => `/api/ratelimit/errors/reset/${providerId}/${tokenId}`,
    
    // Request history
    GET_REQUEST_HISTORY: '/api/ratelimit/request-history',
    GET_TOKEN_REQUEST_HISTORY: (tokenId) => `/api/ratelimit/request-history/${tokenId}`,
    GET_TOKEN_MODEL_REQUEST_HISTORY: (tokenId, model) => `/api/ratelimit/request-history/${tokenId}/${model}`,
    GET_REQUEST_HISTORY_SUMMARY: '/api/ratelimit/request-history/summary',
    DELETE_REQUEST_HISTORY: '/api/ratelimit/request-history/delete',
```

## API Endpoint Summary

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/ratelimit/request-history` | Get all request history with filters and pagination |
| GET | `/api/ratelimit/request-history/:token` | Get request history for a specific token |
| GET | `/api/ratelimit/request-history/:token/:model` | Get request history for a specific token and model |
| GET | `/api/ratelimit/request-history/summary` | Get aggregated statistics for request history |
| POST/DELETE | `/api/ratelimit/request-history/delete` | Delete request history records |

## Query Parameters

### For Request History Endpoints:
- `token_id` - Filter by token ID
- `model` - Filter by model name
- `start_time` - Start timestamp in milliseconds
- `end_time` - End timestamp in milliseconds
- `min_tokens` - Minimum total tokens
- `max_tokens` - Maximum total tokens
- `page` - Page number (default: 1)
- `page_size` - Page size (default: 100, max: 1000)

### For Summary Endpoint:
- `token_id` - Filter by token ID (optional)
- `model` - Filter by model (optional)
- `start_time` - Start timestamp in milliseconds (optional)
- `end_time` - End timestamp in milliseconds (optional)

### For Delete Endpoint (at least one required):
- `token_id` - Filter by token ID
- `model` - Filter by model
- `start_time` - Start timestamp in milliseconds
- `end_time` - End timestamp in milliseconds

## Response Formats

### Request History Response:
```json
{
  "records": [
    {
      "id": "uuid",
      "token_id": "token-id",
      "model": "gpt-4",
      "request_count": 1,
      "input_tokens": 100,
      "output_tokens": 50,
      "total_tokens": 150,
      "timestamp": 1710720000000,
      "created_at": 1710720000000
    }
  ],
  "total_count": 1000,
  "page": 1,
  "page_size": 100,
  "total_pages": 10,
  "has_next": true,
  "has_previous": false
}
```

### Summary Response:
```json
{
  "token_id": "token-id",
  "model": "gpt-4",
  "total_requests": 1000,
  "total_input_tokens": 100000,
  "total_output_tokens": 50000,
  "total_tokens": 150000,
  "average_tokens": 150.0,
  "first_request": 1710720000000,
  "last_request": 1710806400000
}
```

## Testing Requirements

After implementation, add tests in `internal/ratelimit/usage_tracker_test.go`:
- Test `GetRequestHistory` with various filters
- Test `GetRequestHistoryByToken` pagination
- Test `GetRequestHistoryByModel` filtering
- Test `GetRequestHistorySummary` aggregation
- Test `DeleteRequestHistory` with filters
- Test edge cases (empty results, invalid parameters)

## Implementation Order

1. Add data structures to `interfaces.go`
2. Add interface methods to `UsageTracker`
3. Implement methods in `usage_tracker.go`
4. Add wrapper methods to `quota_manager.go`
5. Add REST API handlers to `rate_limit_api.go`
6. Update dashboard `endpoints.js`
7. Write unit tests
8. Test with curl/Postman
9. Update dashboard UI to use new endpoints
