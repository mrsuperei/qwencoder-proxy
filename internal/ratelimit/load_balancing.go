package ratelimit

import (
	"context"
	"database/sql"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/sunbankio/qwencoder-proxy/internal/logging"
	"github.com/sunbankio/qwencoder-proxy/internal/token"
)

// LoadBalancingStrategy defines the interface for load balancing strategies
type LoadBalancingStrategy interface {
	// SelectToken selects a token based on the strategy
	SelectToken(ctx context.Context, providerID string) (*token.ProviderToken, error)

	// Name returns the strategy name
	Name() string
}

// WeightedRoundRobinSelector implements weighted round-robin load balancing
type WeightedRoundRobinSelector struct {
	currentIndex int
	mu           sync.Mutex
	db           *sql.DB
	quotaManager *QuotaManager
	logger       logging.Logger
}

// NewWeightedRoundRobinSelector creates a new weighted round-robin selector
func NewWeightedRoundRobinSelector(db *sql.DB, quotaManager *QuotaManager, logger logging.Logger) *WeightedRoundRobinSelector {
	return &WeightedRoundRobinSelector{
		currentIndex: 0,
		db:           db,
		quotaManager: quotaManager,
		logger:       logger,
	}
}

// Name returns the strategy name
func (s *WeightedRoundRobinSelector) Name() string {
	return "weighted_round_robin"
}

// SelectToken selects a token using weighted round-robin
func (s *WeightedRoundRobinSelector) SelectToken(ctx context.Context, providerID string) (*token.ProviderToken, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Get all valid tokens with their weights
	tokens, err := s.getWeightedTokens(ctx, providerID)
	if err != nil {
		return nil, err
	}

	if len(tokens) == 0 {
		return nil, token.ErrNoValidTokens
	}

	// Sort tokens by weight (highest first) for weighted selection
	sort.Slice(tokens, func(i, j int) bool {
		return tokens[i].weight > tokens[j].weight
	})

	// Select token based on round-robin within sorted list
	s.currentIndex = (s.currentIndex + 1) % len(tokens)
	selected := tokens[s.currentIndex]

	s.logger.DebugLog("[WeightedRoundRobinSelector] Selected token %s (weight: %d)",
		selected.token.ID, selected.weight)

	return selected.token, nil
}

type weightedToken struct {
	token  *token.ProviderToken
	weight int
}

func (s *WeightedRoundRobinSelector) getWeightedTokens(ctx context.Context, providerID string) ([]weightedToken, error) {
	// Get all valid tokens for the provider
	query := `
		SELECT id, access_token, refresh_token, token_type, expiry_date,
		       email, resource_url, scope, api_key, project_id,
		       healthy, health_score, last_used, created_at, error_count,
		       last_error, proxy_id
		FROM tokens
		WHERE provider_id = ? AND healthy = 1 AND expiry_date > ?
	`

	now := time.Now().UnixMilli()
	rows, err := s.db.QueryContext(ctx, query, providerID, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tokens []token.ProviderToken
	for rows.Next() {
		var t token.ProviderToken
		var proxyID sql.NullString
		var projectID sql.NullString
		var lastError sql.NullString

		err := rows.Scan(
			&t.ID, &t.AccessToken, &t.RefreshToken, &t.TokenType, &t.ExpiryDate,
			&t.Email, &t.ResourceURL, &t.Scope, &t.APIKey, &projectID,
			&t.Healthy, &t.HealthScore, &t.LastUsed, &t.CreatedAt, &t.ErrorCount,
			&lastError, &proxyID,
		)
		if projectID.Valid {
			t.ProjectID = projectID.String
		}
		if lastError.Valid {
			t.LastError = lastError.String
		}
		if err != nil {
			return nil, err
		}
		tokens = append(tokens, t)
	}

	// Calculate weights based on remaining quota
	weightedTokens := make([]weightedToken, 0, len(tokens))
	for _, t := range tokens {
		s.logger.DebugLog("[WeightedRoundRobinSelector] Checking quota status for token: %s", t.ID)
		status, err := s.quotaManager.GetQuotaStatus(ctx, providerID, t.ID)
		if err != nil {
			s.logger.WarnLog("[WeightedRoundRobinSelector] Failed to get quota status for %s: %v",
				t.ID, err)
			continue
		}

		// Calculate weight based on remaining quota
		weight := int(status.RemainingRequestsPerDay) +
			int(status.RemainingRequestsPerMinute) +
			int(status.RemainingTokensPerMinute)

		// Ensure minimum weight of 1
		if weight < 1 {
			weight = 1
		}

		s.logger.DebugLog("[WeightedRoundRobinSelector] Token %s has weight: %d (RPD: %d, RPM: %d, TPM: %d)",
			t.ID, weight, status.RemainingRequestsPerDay, status.RemainingRequestsPerMinute, status.RemainingTokensPerMinute)

		weightedTokens = append(weightedTokens, weightedToken{
			token:  &t,
			weight: weight,
		})
	}

	s.logger.DebugLog("[WeightedRoundRobinSelector] Returning %d weighted tokens", len(weightedTokens))

	return weightedTokens, nil
}

// LeastConnectionsSelector implements least connections load balancing
type LeastConnectionsSelector struct {
	db           *sql.DB
	quotaManager *QuotaManager
	logger       logging.Logger
}

// NewLeastConnectionsSelector creates a new least connections selector
func NewLeastConnectionsSelector(db *sql.DB, quotaManager *QuotaManager, logger logging.Logger) *LeastConnectionsSelector {
	return &LeastConnectionsSelector{
		db:           db,
		quotaManager: quotaManager,
		logger:       logger,
	}
}

// Name returns the strategy name
func (s *LeastConnectionsSelector) Name() string {
	return "least_connections"
}

// SelectToken selects the token with the fewest active connections
func (s *LeastConnectionsSelector) SelectToken(ctx context.Context, providerID string) (*token.ProviderToken, error) {
	// Get tokens with their current connection counts
	query := `
		SELECT t.id, t.access_token, t.refresh_token, t.token_type, t.expiry_date,
		       t.email, t.resource_url, t.scope, t.api_key, t.project_id,
		       t.healthy, t.health_score, t.last_used, t.created_at, t.error_count,
		       t.last_error, t.proxy_id,
		       COALESCE(u.requests_in_minute, 0) as current_connections
		FROM tokens t
		LEFT JOIN token_usage u ON t.id = u.token_id
		WHERE t.provider_id = ? AND t.healthy = 1 AND t.expiry_date > ?
		ORDER BY current_connections ASC
		LIMIT 1
	`

	now := time.Now().UnixMilli()
	var t token.ProviderToken
	var currentConnections int
	var proxyID sql.NullString
	var projectID sql.NullString
	var lastError sql.NullString

	err := s.db.QueryRowContext(ctx, query, providerID, now).Scan(
		&t.ID, &t.AccessToken, &t.RefreshToken, &t.TokenType, &t.ExpiryDate,
		&t.Email, &t.ResourceURL, &t.Scope, &t.APIKey, &projectID,
		&t.Healthy, &t.HealthScore, &t.LastUsed, &t.CreatedAt, &t.ErrorCount,
		&lastError, &proxyID,
		&currentConnections,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, token.ErrNoValidTokens
		}
		return nil, err
	}

	if projectID.Valid {
		t.ProjectID = projectID.String
	}
	if lastError.Valid {
		t.LastError = lastError.String
	}

	s.logger.DebugLog("[LeastConnectionsSelector] Selected token %s (connections: %d)",
		t.ID, currentConnections)

	return &t, nil
}

// AdaptiveSelector implements adaptive load balancing
type AdaptiveSelector struct {
	db           *sql.DB
	quotaManager *QuotaManager
	logger       logging.Logger
}

// NewAdaptiveSelector creates a new adaptive selector
func NewAdaptiveSelector(db *sql.DB, quotaManager *QuotaManager, logger logging.Logger) *AdaptiveSelector {
	return &AdaptiveSelector{
		db:           db,
		quotaManager: quotaManager,
		logger:       logger,
	}
}

// Name returns the strategy name
func (s *AdaptiveSelector) Name() string {
	return "adaptive"
}

// SelectToken selects a token using adaptive multi-factor scoring
func (s *AdaptiveSelector) SelectToken(ctx context.Context, providerID string) (*token.ProviderToken, error) {
	// Combine multiple factors for adaptive selection
	tokens, err := s.getValidTokens(ctx, providerID)
	if err != nil {
		return nil, err
	}

	if len(tokens) == 0 {
		return nil, token.ErrNoValidTokens
	}

	// Calculate adaptive score for each token
	type adaptiveScore struct {
		token   *token.ProviderToken
		score   float64
		factors map[string]float64
	}

	scores := make([]adaptiveScore, 0, len(tokens))
	for _, t := range tokens {
		factors := make(map[string]float64)

		// Factor 1: Rate limit availability (0-1)
		status, err := s.quotaManager.GetQuotaStatus(ctx, providerID, t.ID)
		if err == nil {
			factors["rate_limit"] = s.calculateRateLimitScore(status)
		} else {
			factors["rate_limit"] = 1.0 // Assume full availability
		}

		// Factor 2: Health score (0-1)
		factors["health"] = float64(t.HealthScore)

		// Factor 3: Recency (prefer less recently used)
		timeSinceLastUse := time.Since(time.UnixMilli(t.LastUsed))
		factors["recency"] = math.Min(timeSinceLastUse.Minutes()/60.0, 1.0)

		// Factor 4: Error rate (enhanced with exponential decay)
		if t.ErrorCount > 0 {
			// Exponential decay based on error count
			// More errors = much lower score
			factors["error_rate"] = 1.0 / math.Pow(float64(t.ErrorCount+1), 1.5)
		} else {
			factors["error_rate"] = 1.0
		}

		// Factor 5: Healthy status
		if t.Healthy {
			factors["healthy_status"] = 1.0
		} else {
			// Severely penalize unhealthy tokens
			factors["healthy_status"] = 0.1
		}

		// Calculate weighted score (updated weights)
		score := (factors["rate_limit"] * 0.4) +
			(factors["health"] * 0.15) +
			(factors["recency"] * 0.15) +
			(factors["error_rate"] * 0.25) +
			(factors["healthy_status"] * 0.05)

		scores = append(scores, adaptiveScore{
			token:   &t,
			score:   score,
			factors: factors,
		})
	}

	// Sort by score (highest first)
	sort.Slice(scores, func(i, j int) bool {
		return scores[i].score > scores[j].score
	})

	selected := scores[0]
	s.logger.DebugLog("[AdaptiveSelector] Selected token %s (score: %.2f, factors: %v)",
		selected.token.ID, selected.score, selected.factors)

	return selected.token, nil
}

// calculateRateLimitScore calculates a score based on rate limit availability
func (s *AdaptiveSelector) calculateRateLimitScore(status *QuotaStatus) float64 {
	// Calculate score based on remaining quotas
	rpdScore := math.Min(float64(status.RemainingRequestsPerDay)/1000.0, 1.0)
	rpmScore := math.Min(float64(status.RemainingRequestsPerMinute)/60.0, 1.0)
	tpmScore := math.Min(float64(status.RemainingTokensPerMinute)/10000.0, 1.0)

	return (rpdScore * 0.4) + (rpmScore * 0.3) + (tpmScore * 0.3)
}

// getValidTokens retrieves all valid tokens for a provider
func (s *AdaptiveSelector) getValidTokens(ctx context.Context, providerID string) ([]token.ProviderToken, error) {
	query := `
		SELECT id, access_token, refresh_token, token_type, expiry_date,
		       email, resource_url, scope, api_key, project_id,
		       healthy, health_score, last_used, created_at, error_count,
		       last_error, proxy_id
		FROM tokens
		WHERE provider_id = ? AND healthy = 1 AND expiry_date > ?
	`

	now := time.Now().UnixMilli()
	rows, err := s.db.QueryContext(ctx, query, providerID, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tokens []token.ProviderToken
	for rows.Next() {
		var t token.ProviderToken
		var proxyID sql.NullString
		var projectID sql.NullString
		var lastError sql.NullString

		err := rows.Scan(
			&t.ID, &t.AccessToken, &t.RefreshToken, &t.TokenType, &t.ExpiryDate,
			&t.Email, &t.ResourceURL, &t.Scope, &t.APIKey, &projectID,
			&t.Healthy, &t.HealthScore, &t.LastUsed, &t.CreatedAt, &t.ErrorCount,
			&lastError, &proxyID,
		)
		if err != nil {
			return nil, err
		}

		if projectID.Valid {
			t.ProjectID = projectID.String
		}
		if lastError.Valid {
			t.LastError = lastError.String
		}

		tokens = append(tokens, t)
	}

	return tokens, nil
}
