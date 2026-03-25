package ratelimit

import (
	"context"
	"sort"

	"github.com/sunbankio/qwencoder-proxy/internal/logging"
	"github.com/sunbankio/qwencoder-proxy/internal/token"
)

// UsageAwareSelectionStrategy selects tokens based on current usage metrics.
// Implements the token.SelectionStrategy interface.
type UsageAwareSelectionStrategy struct {
	quotaManager *QuotaManager
	logger       logging.Logger
	providerID   string
}

// NewUsageAwareSelectionStrategy creates a new usage-aware selection strategy.
// The providerID is required to track usage per provider.
func NewUsageAwareSelectionStrategy(quotaManager *QuotaManager, logger logging.Logger, providerID string) token.SelectionStrategy {
	return &UsageAwareSelectionStrategy{
		quotaManager: quotaManager,
		logger:       logger,
		providerID:   providerID,
	}
}

// Name returns the strategy name.
func (s *UsageAwareSelectionStrategy) Name() string {
	return "usage_aware"
}

// SelectToken selects a token based on usage metrics.
func (s *UsageAwareSelectionStrategy) SelectToken(tokens []token.ProviderToken) (*token.ProviderToken, error) {
	if len(tokens) == 0 {
		return nil, token.ErrNoTokensAvailable
	}

	ctx := context.Background()

	// Evaluate each token's quota status.
	type tokenScore struct {
		token     *token.ProviderToken
		status    *QuotaStatus
		score     float64
		isLimited bool
	}

	scores := make([]tokenScore, 0, len(tokens))

	for i := range tokens {
		t := &tokens[i]

		// Get quota status for this token.
		// Use strategy's providerID and token's ID as tokenID.
		status, err := s.quotaManager.GetQuotaStatus(ctx, s.providerID, t.ID)
		if err != nil {
			s.logger.WarnLog("[UsageAwareSelectionStrategy] Failed to get quota status for %s/%s: %v",
				s.providerID, t.ID, err)
			continue
		}

		// Calculate score based on remaining quota.
		// Higher score = more available quota.
		score := float64(status.RemainingRequestsPerDay)*0.4 +
			float64(status.RemainingRequestsPerMinute)*0.3 +
			float64(status.RemainingTokensPerMinute)*0.3

		scores = append(scores, tokenScore{
			token:     t,
			status:    status,
			score:     score,
			isLimited: status.IsLimited,
		})
	}

	// Filter out rate-limited tokens.
	available := make([]tokenScore, 0)
	for _, ts := range scores {
		if !ts.isLimited {
			available = append(available, ts)
		}
	}

	// If all tokens are rate-limited, return error.
	if len(available) == 0 {
		s.logger.WarnLog("[UsageAwareSelectionStrategy] All tokens are rate-limited")
		return nil, token.ErrNoValidTokens
	}

	// Sort by score (highest first).
	sort.Slice(available, func(i, j int) bool {
		return available[i].score > available[j].score
	})

	// Select the token with the highest score.
	selected := available[0]

	s.logger.DebugLog("[UsageAwareSelectionStrategy] Selected token %s/%s (score: %.2f, remaining: RPD=%d, RPM=%d, TPM=%d)",
		s.providerID, selected.token.ID, selected.score,
		selected.status.RemainingRequestsPerDay,
		selected.status.RemainingRequestsPerMinute,
		selected.status.RemainingTokensPerMinute)

	return selected.token, nil
}
