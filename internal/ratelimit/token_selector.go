package ratelimit

import (
	"context"
	"database/sql"

	"github.com/sunbankio/qwencoder-proxy/internal/logging"
	"github.com/sunbankio/qwencoder-proxy/internal/token"
)

// RateAwareTokenSelector selects tokens based on a configurable load balancing strategy
type RateAwareTokenSelector struct {
	db       *sql.DB
	logger   logging.Logger
	strategy LoadBalancingStrategy
}

// NewRateAwareTokenSelector creates a new rate-aware token selector with the specified strategy
func NewRateAwareTokenSelector(db *sql.DB, quotaManager *QuotaManager, logger logging.Logger, strategy LoadBalancingStrategy) *RateAwareTokenSelector {
	return &RateAwareTokenSelector{
		db:       db,
		logger:   logger,
		strategy: strategy,
	}
}

// SelectToken delegates to the configured load balancing strategy
func (s *RateAwareTokenSelector) SelectToken(ctx context.Context, providerID string) (*token.ProviderToken, error) {
	s.logger.DebugLog("[RateAwareTokenSelector] Using strategy: %s", s.strategy.Name())
	return s.strategy.SelectToken(ctx, providerID)
}

// SetStrategy changes the load balancing strategy
func (s *RateAwareTokenSelector) SetStrategy(strategy LoadBalancingStrategy) {
	s.logger.InfoLog("[RateAwareTokenSelector] Strategy changed to: %s", strategy.Name())
	s.strategy = strategy
}

// GetStrategy returns the current load balancing strategy name
func (s *RateAwareTokenSelector) GetStrategy() string {
	return s.strategy.Name()
}
