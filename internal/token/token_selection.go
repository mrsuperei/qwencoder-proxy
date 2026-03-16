package token

import (
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/sunbankio/qwencoder-proxy/internal/logging"
)

// ProxyClientFactory is an interface for creating HTTP clients with proxy configuration.
// This interface breaks the circular dependency between auth and config packages.
type ProxyClientFactory interface {
	// GetClient returns an HTTP client for the given proxy configuration.
	// If proxyConfig is nil or has type ProxyTypeNone, a direct connection client is returned.
	GetClient(proxyConfig *ProxyConfig) *http.Client
}

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
	store              TokenRepository
	strategy           SelectionStrategy
	mu                 sync.RWMutex
	logger             logging.Logger
	clientFactory      ProxyClientFactory
	proxyHealthTracker *ProxyHealthTracker
}

// NewTokenManager creates a new TokenManager
func NewTokenManager(store TokenRepository, strategy SelectionStrategy, logger logging.Logger,
	clientFactory ProxyClientFactory, proxyHealthTracker *ProxyHealthTracker) *TokenManager {
	return &TokenManager{
		store:              store,
		strategy:           strategy,
		logger:             logger,
		clientFactory:      clientFactory,
		proxyHealthTracker: proxyHealthTracker,
	}
}

// SelectToken selects a token using the configured strategy
func (tm *TokenManager) SelectToken() (*ProviderToken, error) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	// Get all tokens from store
	tokens := tm.store.ListTokens()
	nowMs := time.Now().UnixMilli()
	tm.logger.DebugLog("[TokenManager] SelectToken called: total tokens=%d, nowMs=%d", len(tokens), nowMs)
	for i, token := range tokens {
		tm.logger.DebugLog("[TokenManager] Token[%d]: ID=%s, Email=%s, Healthy=%v, ExpiryDate=%d, LastUsed=%d",
			i, token.ID, token.Email, token.Healthy, token.ExpiryDate, token.LastUsed)
	}
	if len(tokens) == 0 {
		tm.logger.ErrorLog("[TokenManager] No tokens available in store")
		return nil, ErrNoTokensAvailable
	}

	// Select token using configured strategy
	token, err := tm.strategy.SelectToken(tokens)
	if err != nil {
		return nil, err
	}

	// Update LastUsed timestamp
	if err := tm.store.UpdateToken(token.ID, func(t *TokenMetadata) {
		t.LastUsed = time.Now().UnixMilli()
	}); err != nil {
		tm.logger.WarnLog("Failed to update LastUsed timestamp for token %s: %v", token.ID, err)
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
	if err := tm.store.UpdateToken(tokenID, func(t *TokenMetadata) {
		t.LastUsed = time.Now().UnixMilli()
	}); err != nil {
		tm.logger.WarnLog("Failed to update LastUsed timestamp for token %s: %v", tokenID, err)
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

// SelectTokenWithClient selects a token using the configured strategy and returns
// both the token and its configured HTTP client. This enables providers to use
// token-specific proxy configurations for API requests.
//
// The method:
// 1. Calls existing SelectionStrategy.SelectToken() to get ProviderToken
// 2. Gets ProviderToken from MultiTokenStore with full metadata
// 3. Extracts ProxyConfig from token (may be nil)
// 4. Calls ProxyAwareHTTPClientFactory.GetClient(proxyConfig)
// 5. Updates proxy health metrics via ProxyHealthTracker
// 6. Returns (token, client, nil)
//
// Returns an error if no tokens are available or if token selection fails.
func (tm *TokenManager) SelectTokenWithClient() (*ProviderToken, *http.Client, error) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	// Get all tokens from store
	tokens := tm.store.ListTokens()
	if len(tokens) == 0 {
		return nil, nil, ErrNoTokensAvailable
	}

	// Select token using configured strategy
	token, err := tm.strategy.SelectToken(tokens)
	if err != nil {
		return nil, nil, err
	}

	// Get full ProviderToken from store with all metadata
	fullToken, err := tm.store.GetToken(token.ID)
	if err != nil {
		tm.logger.ErrorLog("Failed to get full token %s from store: %v", token.ID, err)
		return nil, nil, fmt.Errorf("failed to get token from store: %w", err)
	}

	// Extract ProxyConfig from token (may be nil)
	proxyConfig := fullToken.Proxy

	// Get client from factory with proxy config
	if tm.clientFactory == nil {
		tm.logger.WarnLog("Client factory not set for token manager, returning nil client")
		return fullToken, nil, nil
	}

	client := tm.clientFactory.GetClient(proxyConfig)

	// Update proxy health metrics via ProxyHealthTracker
	if tm.proxyHealthTracker != nil {
		healthScore := tm.proxyHealthTracker.GetHealthScore(fullToken.ID)
		tm.logger.DebugLog("Token %s selected with proxy health score: %.2f", fullToken.ID, healthScore)
	}

	// Update LastUsed timestamp
	if err := tm.store.UpdateToken(fullToken.ID, func(t *TokenMetadata) {
		t.LastUsed = time.Now().UnixMilli()
	}); err != nil {
		tm.logger.WarnLog("Failed to update LastUsed timestamp for token %s: %v", fullToken.ID, err)
	}

	proxyType := "direct"
	if proxyConfig != nil {
		proxyType = string(proxyConfig.Type)
	}
	tm.logger.DebugLog("Selected token %s using %s strategy with %s connection", fullToken.ID, tm.strategy.Name(), proxyType)

	return fullToken, client, nil
}

// GetTokenClient returns an HTTP client configured for the specified token's proxy.
// This method allows providers to get a client for a specific token without
// going through the token selection process.
//
// Returns an error if the token is not found or if client factory is not set.
func (tm *TokenManager) GetTokenClient(tokenID string) (*http.Client, error) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	// Get ProviderToken from store
	token, err := tm.store.GetToken(tokenID)
	if err != nil {
		tm.logger.ErrorLog("Failed to get token %s: %v", tokenID, err)
		return nil, fmt.Errorf("failed to get token: %w", err)
	}

	// Check if client factory is set
	if tm.clientFactory == nil {
		tm.logger.WarnLog("Client factory not set for token manager")
		return nil, fmt.Errorf("client factory not set")
	}

	// Extract ProxyConfig from token
	proxyConfig := token.Proxy

	// Return client from factory
	client := tm.clientFactory.GetClient(proxyConfig)

	proxyType := "direct"
	if proxyConfig != nil {
		proxyType = string(proxyConfig.Type)
	}
	tm.logger.DebugLog("Retrieved client for token %s with %s connection", tokenID, proxyType)

	return client, nil
}

// UpdateProxyHealth updates the health status for a token's proxy connection.
// This method:
// - Calls proxyHealthTracker.UpdateHealth()
// - Updates ProviderToken.ProxyHealthScore in store
// - Saves updated token
//
// Parameters:
//   - tokenID: The ID of the token to update
//   - healthy: Whether the proxy connection is healthy
//   - err: Optional error from the proxy connection
//
// Returns an error if the token is not found or if the update fails.
func (tm *TokenManager) UpdateProxyHealth(tokenID string, healthy bool, err error) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	// Check if proxy health tracker is set
	if tm.proxyHealthTracker == nil {
		tm.logger.WarnLog("Proxy health tracker not set for token manager")
		return fmt.Errorf("proxy health tracker not set")
	}

	// Update health in tracker
	healthScore := tm.proxyHealthTracker.UpdateHealth(tokenID, healthy, err)

	// Update ProviderToken.ProxyHealthScore in store
	updateErr := tm.store.UpdateToken(tokenID, func(token *ProviderToken) {
		token.ProxyHealthScore = healthScore
	})
	if updateErr != nil {
		tm.logger.ErrorLog("Failed to update proxy health score for token %s: %v", tokenID, updateErr)
		return fmt.Errorf("failed to update token proxy health: %w", updateErr)
	}

	status := "healthy"
	if !healthy {
		status = "unhealthy"
	}
	tm.logger.DebugLog("Updated proxy health for token %s: %s, score: %.2f", tokenID, status, healthScore)

	return nil
}

// UpdateToken updates a token using the provided update function.
// This is a convenience method that delegates to the store's UpdateToken.
//
// Parameters:
//   - tokenID: The ID of the token to update
//   - updateFunc: A function that modifies the token metadata
//
// Returns an error if the token is not found or if the update fails.
func (tm *TokenManager) UpdateToken(tokenID string, updateFunc func(*TokenMetadata)) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	// Update token in store
	updateErr := tm.store.UpdateToken(tokenID, updateFunc)
	if updateErr != nil {
		tm.logger.ErrorLog("Failed to update token %s: %v", tokenID, updateErr)
		return fmt.Errorf("failed to update token: %w", updateErr)
	}

	tm.logger.DebugLog("Updated token %s", tokenID)
	return nil
}

// HealthTracker tracks token health and provides recommendations
type HealthTracker struct {
	store  *SQLiteStore
	mu     sync.RWMutex
	logger logging.Logger
}

// NewHealthTracker creates a new HealthTracker
func NewHealthTracker(store *SQLiteStore, logger logging.Logger) *HealthTracker {
	return &HealthTracker{
		store:  store,
		logger: logger,
	}
}

// ReportSuccess reports a successful token usage
func (ht *HealthTracker) ReportSuccess(tokenID string) error {
	ht.mu.Lock()
	defer ht.mu.Unlock()

	return ht.store.UpdateToken(tokenID, func(token *TokenMetadata) {
		token.Healthy = true
		token.ErrorCount = 0
		token.LastError = ""
		// Gradually increase health score, max 1.0
		token.HealthScore = min(1.0, token.HealthScore+0.1)
		token.LastUsed = time.Now().UnixMilli()
	})
}

// ReportFailure reports a failed token usage
func (ht *HealthTracker) ReportFailure(tokenID string, err error) error {
	ht.mu.Lock()
	defer ht.mu.Unlock()

	// Get settings to check max error count
	settings, settingsErr := ht.store.GetSettings()
	if settingsErr != nil {
		return settingsErr
	}

	return ht.store.UpdateToken(tokenID, func(token *TokenMetadata) {
		token.ErrorCount++
		token.HealthScore = max(0.0, token.HealthScore-0.2)
		if err != nil {
			token.LastError = err.Error()
		}
		// Mark as unhealthy if error count exceeds max
		if token.ErrorCount >= settings.MaxErrorCount {
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
		// We can't log here because this function is called from strategies
		// But the validity check is: healthy=true AND expiry > now + 30min_buffer
		if isTokenValid(token) {
			valid = append(valid, token)
		} else {
			// DEBUG: Log why token was rejected to help diagnose token expiry issues
			// This will appear in logs when SelectToken is called
			// We can't use logger here because this is a package-level function
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
	nowMs := time.Now().UnixMilli()
	expiryWithBuffer := token.ExpiryDate - bufferMs
	isValid := nowMs < expiryWithBuffer
	// DEBUG: Log validation details to diagnose token expiry issues
	// This helps identify if tokens are being rejected due to incorrect expiry calculation
	// The validity check is: now < (expiry - 30min_buffer)
	// We can't use logging here as this is called from strategies
	return isValid
}
