package auth

import (
	"errors"
	"fmt"
	"math/rand"
	"sort"
	"sync"
	"time"

	"github.com/sunbankio/qwencoder-proxy/logging"
)

var (
	// ErrNoValidTokens is returned when no valid tokens are available
	ErrNoValidTokens = errors.New("no valid tokens available")
	// ErrNoTokensAvailable is returned when no tokens are available at all
	ErrNoTokensAvailable = errors.New("no tokens available")
	// ErrUnknownStrategy is returned when an unknown strategy name is requested
	ErrUnknownStrategy = errors.New("unknown selection strategy")
)

// SelectionStrategy defines the interface for token selection strategies
type SelectionStrategy interface {
	// SelectToken selects a token from the provided list
	SelectToken(tokens []ProviderToken) (*ProviderToken, error)
	// Name returns the strategy name
	Name() string
}

// RandomSelectionStrategy selects a random valid token
type RandomSelectionStrategy struct {
	rand *rand.Rand
}

// NewRandomSelectionStrategy creates a new RandomSelectionStrategy
func NewRandomSelectionStrategy() *RandomSelectionStrategy {
	return &RandomSelectionStrategy{
		rand: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// Name returns the strategy name
func (s *RandomSelectionStrategy) Name() string {
	return "random"
}

// SelectToken selects a random valid token from the provided list
func (s *RandomSelectionStrategy) SelectToken(tokens []ProviderToken) (*ProviderToken, error) {
	// Filter valid tokens
	validTokens := filterValidTokens(tokens)
	if len(validTokens) == 0 {
		return nil, ErrNoValidTokens
	}

	// Randomly select one token
	idx := s.rand.Intn(len(validTokens))
	return &validTokens[idx], nil
}

// RoundRobinSelectionStrategy selects tokens in a round-robin fashion
type RoundRobinSelectionStrategy struct {
	currentIndex int
	mu           sync.Mutex
}

// NewRoundRobinSelectionStrategy creates a new RoundRobinSelectionStrategy
func NewRoundRobinSelectionStrategy() *RoundRobinSelectionStrategy {
	return &RoundRobinSelectionStrategy{
		currentIndex: 0,
	}
}

// Name returns the strategy name
func (s *RoundRobinSelectionStrategy) Name() string {
	return "round_robin"
}

// SelectToken selects the next token in round-robin order
func (s *RoundRobinSelectionStrategy) SelectToken(tokens []ProviderToken) (*ProviderToken, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Filter valid tokens
	validTokens := filterValidTokens(tokens)
	if len(validTokens) == 0 {
		return nil, ErrNoValidTokens
	}

	// Select next token in round-robin order
	idx := s.currentIndex % len(validTokens)
	s.currentIndex++
	return &validTokens[idx], nil
}

// LeastUsedSelectionStrategy selects the token that was used least recently
type LeastUsedSelectionStrategy struct{}

// NewLeastUsedSelectionStrategy creates a new LeastUsedSelectionStrategy
func NewLeastUsedSelectionStrategy() *LeastUsedSelectionStrategy {
	return &LeastUsedSelectionStrategy{}
}

// Name returns the strategy name
func (s *LeastUsedSelectionStrategy) Name() string {
	return "least_used"
}

// SelectToken selects the least recently used token
func (s *LeastUsedSelectionStrategy) SelectToken(tokens []ProviderToken) (*ProviderToken, error) {
	// Filter valid tokens
	validTokens := filterValidTokens(tokens)
	if len(validTokens) == 0 {
		return nil, ErrNoValidTokens
	}

	// Sort by LastUsed timestamp (ascending)
	sort.Slice(validTokens, func(i, j int) bool {
		return validTokens[i].LastUsed < validTokens[j].LastUsed
	})

	// Select the least recently used token
	return &validTokens[0], nil
}

// TokenManager manages token selection for a provider
type TokenManager struct {
	store    *MultiTokenStore
	strategy SelectionStrategy
	mu       sync.RWMutex
	logger   *logging.Logger
}

// NewTokenManager creates a new TokenManager
func NewTokenManager(store *MultiTokenStore, strategy SelectionStrategy, logger *logging.Logger) *TokenManager {
	return &TokenManager{
		store:    store,
		strategy: strategy,
		logger:   logger,
	}
}

// SelectToken selects a token using the configured strategy
func (tm *TokenManager) SelectToken() (*ProviderToken, error) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	// Get all tokens from store
	tokens := tm.store.ListTokens()
	if len(tokens) == 0 {
		return nil, ErrNoTokensAvailable
	}

	// Select token using configured strategy
	token, err := tm.strategy.SelectToken(tokens)
	if err != nil {
		return nil, err
	}

	// Update LastUsed timestamp
	if err := tm.store.UpdateToken(token.ID, func(t *ProviderToken) {
		t.LastUsed = GetCurrentTimestamp()
	}); err != nil {
		tm.logger.WarningLog("Failed to update LastUsed timestamp for token %s: %v", token.ID, err)
	}

	tm.logger.DebugLog("Selected token %s using %s strategy", token.ID, tm.strategy.Name())
	return token, nil
}

// SelectTokenByID selects a specific token by ID (for testing or manual selection)
func (tm *TokenManager) SelectTokenByID(tokenID string) (*ProviderToken, error) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	token, err := tm.store.GetToken(tokenID)
	if err != nil {
		return nil, err
	}

	// Check if token is valid
	if !tm.store.IsTokenValid(*token) {
		return nil, fmt.Errorf("token %s is not valid", tokenID)
	}

	// Update LastUsed timestamp
	if err := tm.store.UpdateToken(tokenID, func(t *ProviderToken) {
		t.LastUsed = GetCurrentTimestamp()
	}); err != nil {
		tm.logger.WarningLog("Failed to update LastUsed timestamp for token %s: %v", tokenID, err)
	}

	tm.logger.DebugLog("Selected token %s by ID", tokenID)
	return token, nil
}

// SetStrategy changes the selection strategy
func (tm *TokenManager) SetStrategy(strategy SelectionStrategy) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.strategy = strategy
	tm.logger.InfoLog("Changed selection strategy to: %s", strategy.Name())
}

// GetStrategy returns the current strategy
func (tm *TokenManager) GetStrategy() SelectionStrategy {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return tm.strategy
}

// GetValidTokens returns all valid tokens
func (tm *TokenManager) GetValidTokens() ([]ProviderToken, error) {
	return tm.store.GetValidTokens(), nil
}

// GetTokenCount returns the total number of tokens
func (tm *TokenManager) GetTokenCount() int {
	return tm.store.GetTokenCount()
}

// GetValidTokenCount returns the number of valid tokens
func (tm *TokenManager) GetValidTokenCount() int {
	return tm.store.GetValidTokenCount()
}

// HealthTracker tracks token health and provides recommendations
type HealthTracker struct {
	store  *MultiTokenStore
	mu     sync.RWMutex
	logger *logging.Logger
}

// NewHealthTracker creates a new HealthTracker
func NewHealthTracker(store *MultiTokenStore, logger *logging.Logger) *HealthTracker {
	return &HealthTracker{
		store:  store,
		logger: logger,
	}
}

// ReportSuccess reports a successful token usage
func (ht *HealthTracker) ReportSuccess(tokenID string) error {
	ht.mu.Lock()
	defer ht.mu.Unlock()

	return ht.store.UpdateToken(tokenID, func(token *ProviderToken) {
		token.Healthy = true
		token.ErrorCount = 0
		token.LastError = ""
		// Gradually increase health score, max 1.0
		token.HealthScore = min(1.0, token.HealthScore+0.1)
		token.LastUsed = GetCurrentTimestamp()
	})
}

// ReportFailure reports a failed token usage
func (ht *HealthTracker) ReportFailure(tokenID string, err error) error {
	ht.mu.Lock()
	defer ht.mu.Unlock()

	return ht.store.UpdateToken(tokenID, func(token *ProviderToken) {
		token.ErrorCount++
		token.HealthScore = max(0.0, token.HealthScore-0.2)
		if err != nil {
			token.LastError = err.Error()
		}
		// Mark as unhealthy if error count exceeds max
		if token.ErrorCount >= ht.store.Settings.MaxErrorCount {
			token.Healthy = false
		}
	})
}

// GetHealthiestToken returns the token with the highest health score
func (ht *HealthTracker) GetHealthiestToken() (*ProviderToken, error) {
	ht.mu.RLock()
	defer ht.mu.RUnlock()

	validTokens := ht.store.GetValidTokens()
	if len(validTokens) == 0 {
		return nil, ErrNoValidTokens
	}

	// Find token with highest health score
	healthiest := &validTokens[0]
	for i := 1; i < len(validTokens); i++ {
		if validTokens[i].HealthScore > healthiest.HealthScore {
			healthiest = &validTokens[i]
		}
	}

	return healthiest, nil
}

// GetUnhealthyTokens returns all unhealthy tokens
func (ht *HealthTracker) GetUnhealthyTokens() []ProviderToken {
	ht.mu.RLock()
	defer ht.mu.RUnlock()

	tokens := ht.store.ListTokens()
	unhealthy := make([]ProviderToken, 0)

	for _, token := range tokens {
		if !token.Healthy {
			unhealthy = append(unhealthy, token)
		}
	}

	return unhealthy
}

// CleanupUnhealthyTokens removes tokens that have been unhealthy for too long
// This is a placeholder for future implementation - actual cleanup requires tracking
// when a token became unhealthy
func (ht *HealthTracker) CleanupUnhealthyTokens(maxAge time.Duration) (int, error) {
	ht.mu.Lock()
	defer ht.mu.Unlock()

	// This is a placeholder implementation
	// A full implementation would need to track when each token became unhealthy
	// and only remove tokens that have been unhealthy for longer than maxAge

	unhealthy := ht.GetUnhealthyTokens()
	count := len(unhealthy)

	if count > 0 {
		ht.logger.InfoLog("Found %d unhealthy tokens (cleanup not yet implemented)", count)
	}

	return count, nil
}

// StrategyFactory creates selection strategies by name
type StrategyFactory struct{}

// NewStrategyFactory creates a new StrategyFactory
func NewStrategyFactory() *StrategyFactory {
	return &StrategyFactory{}
}

// CreateStrategy creates a selection strategy by name
func (sf *StrategyFactory) CreateStrategy(name string) (SelectionStrategy, error) {
	switch name {
	case "random":
		return NewRandomSelectionStrategy(), nil
	case "round_robin":
		return NewRoundRobinSelectionStrategy(), nil
	case "least_used":
		return NewLeastUsedSelectionStrategy(), nil
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnknownStrategy, name)
	}
}

// ListStrategies returns a list of available strategy names
func (sf *StrategyFactory) ListStrategies() []string {
	return []string{"random", "round_robin", "least_used"}
}

// filterValidTokens filters tokens to only include valid ones
func filterValidTokens(tokens []ProviderToken) []ProviderToken {
	valid := make([]ProviderToken, 0, len(tokens))

	for _, token := range tokens {
		if isTokenValid(token) {
			valid = append(valid, token)
		}
	}

	return valid
}

// isTokenValid checks if a token is valid (not expired and healthy)
func isTokenValid(token ProviderToken) bool {
	if !token.Healthy {
		return false
	}
	if token.ExpiryDate == 0 {
		return false
	}
	// Use a default buffer of 30 minutes if settings not available
	bufferMs := int64(DefaultRefreshBufferSec) * 1000
	return time.Now().UnixMilli() < token.ExpiryDate-bufferMs
}
