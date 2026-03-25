# Break Down MultiTokenManager into Focused Components

**Priority:** MEDIUM  
**Estimated Time:** 3 days  
**Complexity:** High  
**Files to Create:** 6  
**Files to Modify:** 3  
**Files to Delete:** 1

---

## Problem Description

The `MultiTokenManager` in [`internal/token/multi_token_manager.go`](qwencoder-proxy/internal/token/multi_token_manager.go) is a god object that manages too many responsibilities, making it difficult to:
- Test individual components
- Maintain and extend functionality
- Understand the codebase
- Isolate and fix bugs
- Add new features without affecting existing code

### Current State

**Responsibilities of MultiTokenManager:**

1. **Token Storage Management:**
   - Creating and managing SQLite stores
   - Loading and saving tokens
   - CRUD operations on tokens

2. **Token Manager Management:**
   - Creating token managers per provider
   - Managing token lifecycle
   - Token selection strategies

3. **Token Refresh Scheduling:**
   - Scheduling automatic token refresh
   - Managing refresh intervals
   - Handling refresh failures

4. **Health Tracking:**
   - Tracking token health status
   - Managing health scores
   - Error counting and recovery

5. **Proxy Health Tracking:**
   - Tracking proxy connection health
   - Managing proxy health scores
   - Testing proxy connections

6. **Email Extraction:**
   - Managing email extractors
   - Extracting emails from tokens
   - Registering extractors per provider

7. **Strategy Factory:**
   - Creating selection strategies
   - Managing strategy configuration

**Problems:**
- 7 distinct responsibilities in one struct
- 500+ lines of code in single file
- Difficult to test individual components
- Tight coupling between unrelated features
- Violates Single Responsibility Principle

---

## Solution Architecture

### Separation of Concerns

Split `MultiTokenManager` into focused, testable components:

```
internal/token/
├── manager/
│   ├── token_manager.go         # Core token management
│   ├── manager_factory.go       # Factory for creating managers
│   └── manager_config.go       # Manager configuration
├── storage/
│   ├── store.go                # Storage interface
│   ├── sqlite_store.go          # SQLite implementation
│   └── store_factory.go         # Factory for creating stores
├── refresh/
│   ├── scheduler.go             # Refresh scheduling
│   ├── scheduler_config.go       # Scheduler configuration
│   └── refresh_task.go          # Individual refresh task
├── health/
│   ├── tracker.go               # Health tracking
│   ├── tracker_config.go         # Tracker configuration
│   └── health_state.go          # Health state
├── proxy/
│   ├── health_tracker.go         # Proxy health tracking
│   ├── health_tracker_config.go  # Tracker configuration
│   └── proxy_tester.go          # Proxy connection testing
├── extraction/
│   ├── email_extractor.go        # Email extraction interface
│   ├── extractor_manager.go     # Extractor management
│   └── extractors/
│       ├── qwen.go
│       ├── gemini.go
│       ├── kiro.go
│       └── iflow.go
└── selection/
    ├── strategy.go              # Selection strategies
    ├── strategy_factory.go      # Strategy factory
    └── selector.go             # Token selector
```

---

## Implementation Plan

### Phase 1: Create Manager Package (Day 1)

#### Step 1.1: Create Token Manager

**New File:** `internal/token/manager/token_manager.go`

```go
package manager

import (
	"context"
	"fmt"
	"sync"
	"time"
	"github.com/sunbankio/qwencoder-proxy/internal/logging"
	"github.com/sunbankio/qwencoder-proxy/internal/token"
)

// TokenManager manages tokens for a specific provider
type TokenManager struct {
	store     token.TokenStore
	strategy  token.SelectionStrategy
	logger    logging.Logger
	mu        sync.RWMutex
}

// NewTokenManager creates a new token manager
func NewTokenManager(store token.TokenStore, strategy token.SelectionStrategy, logger logging.Logger) *TokenManager {
	return &TokenManager{
		store:    store,
		strategy: strategy,
		logger:    logger,
	}
}

// SelectToken selects a token using the configured strategy
func (tm *TokenManager) SelectToken() (*token.ProviderToken, error) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	
	// Load all tokens
	tokensMap, err := tm.store.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load tokens: %w", err)
	}
	
	// Convert to slice
	tokens := make([]*token.ProviderToken, 0, len(tokensMap))
	for _, t := range tokensMap {
		tokens = append(tokens, &t)
	}
	
	// Select using strategy
	selected, err := tm.strategy.Select(tokens)
	if err != nil {
		return nil, fmt.Errorf("failed to select token: %w", err)
	}
	
	tm.logger.DebugLog("[TokenManager] Selected token %s for provider", selected.ID)
	return selected, nil
}

// UpdateToken updates a token in storage
func (tm *TokenManager) UpdateToken(token *token.ProviderToken) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	
	tokensMap, err := tm.store.Load()
	if err != nil {
		return fmt.Errorf("failed to load tokens: %w", err)
	}
	
	tokensMap[token.ID] = *token
	
	if err := tm.store.Save(tokensMap); err != nil {
		return fmt.Errorf("failed to save token: %w", err)
	}
	
	tm.logger.DebugLog("[TokenManager] Updated token %s", token.ID)
	return nil
}

// GetTokens returns all tokens
func (tm *TokenManager) GetTokens() ([]token.ProviderToken, error) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	
	tokensMap, err := tm.store.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load tokens: %w", err)
	}
	
	tokens := make([]token.ProviderToken, 0, len(tokensMap))
	for _, t := range tokensMap {
		tokens = append(tokens, t)
	}
	
	return tokens, nil
}

// GetValidTokens returns all valid tokens
func (tm *TokenManager) GetValidTokens() ([]token.ProviderToken, error) {
	tokens, err := tm.GetTokens()
	if err != nil {
		return nil, err
	}
	
	validTokens := make([]token.ProviderToken, 0)
	for _, t := range tokens {
		if tm.store.IsTokenValid(t) {
			validTokens = append(validTokens, t)
		}
	}
	
	return validTokens, nil
}
```

#### Step 1.2: Create Manager Factory

**New File:** `internal/token/manager/manager_factory.go`

```go
package manager

import (
	"fmt"
	"github.com/sunbankio/qwencoder-proxy/internal/logging"
	"github.com/sunbankio/qwencoder-proxy/internal/token"
	"github.com/sunbankio/qwencoder-proxy/internal/token/selection"
)

// ManagerFactory creates token managers for providers
type ManagerFactory struct {
	storeFactory token.StoreFactory
	strategyFactory *selection.StrategyFactory
	logger        logging.Logger
}

// NewManagerFactory creates a new manager factory
func NewManagerFactory(storeFactory token.StoreFactory, logger logging.Logger) *ManagerFactory {
	return &ManagerFactory{
		storeFactory:    storeFactory,
		strategyFactory: selection.NewStrategyFactory(),
		logger:          logger,
	}
}

// GetManager returns a token manager for the specified provider
func (mf *ManagerFactory) GetManager(providerID string) (*TokenManager, error) {
	// Get store for provider
	store, err := mf.storeFactory.GetStore(providerID)
	if err != nil {
		return nil, fmt.Errorf("failed to get store for %s: %w", providerID, err)
	}
	
	// Get settings for strategy
	settings, err := store.GetSettings()
	if err != nil {
		return nil, fmt.Errorf("failed to get settings for %s: %w", providerID, err)
	}
	
	// Create strategy
	strategy, err := mf.strategyFactory.GetStrategy(settings.SelectionStrategy)
	if err != nil {
		return nil, fmt.Errorf("failed to get strategy for %s: %w", providerID, err)
	}
	
	// Create manager
	manager := NewTokenManager(store, strategy, mf.logger)
	
	mf.logger.InfoLog("[ManagerFactory] Created manager for provider %s", providerID)
	return manager, nil
}
```

#### Step 1.3: Create Manager Config

**New File:** `internal/token/manager/manager_config.go`

```go
package manager

// ManagerConfig holds configuration for token managers
type ManagerConfig struct {
	// ProviderID is the identifier for the provider
	ProviderID string
	
	// AutoRefresh enables automatic token refresh
	AutoRefresh bool
	
	// RefreshInterval is the interval between refresh checks
	RefreshInterval time.Duration
	
	// HealthCheckInterval is the interval between health checks
	HealthCheckInterval time.Duration
	
	// MaxRetries is the maximum number of retry attempts
	MaxRetries int
}

// DefaultManagerConfig returns sensible defaults
func DefaultManagerConfig() ManagerConfig {
	return ManagerConfig{
		AutoRefresh:         true,
		RefreshInterval:     5 * time.Minute,
		HealthCheckInterval: 1 * time.Minute,
		MaxRetries:          3,
	}
}
```

### Phase 2: Create Refresh Package (Day 1)

#### Step 2.1: Create Refresh Scheduler

**New File:** `internal/token/refresh/scheduler.go`

```go
package refresh

import (
	"context"
	"fmt"
	"sync"
	"time"
	"github.com/sunbankio/qwencoder-proxy/internal/logging"
	"github.com/sunbankio/qwencoder-proxy/internal/token"
)

// Scheduler manages token refresh tasks
type Scheduler struct {
	tasks    map[string]*RefreshTask
	mu        sync.RWMutex
	logger    logging.Logger
	ctx       context.Context
	cancel    context.CancelFunc
}

// NewScheduler creates a new refresh scheduler
func NewScheduler(logger logging.Logger) *Scheduler {
	return &Scheduler{
		tasks: make(map[string]*RefreshTask),
		logger: logger,
	}
}

// Start starts the refresh scheduler
func (s *Scheduler) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	s.ctx, s.cancel = context.WithCancel(ctx)
	
	s.logger.InfoLog("[Scheduler] Starting refresh scheduler")
	
	// Start goroutine to process tasks
	go s.processTasks()
	
	return nil
}

// Stop stops the refresh scheduler
func (s *Scheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	if s.cancel != nil {
		s.cancel()
	}
	
	s.logger.InfoLog("[Scheduler] Stopping refresh scheduler")
}

// ScheduleTask schedules a refresh task
func (s *Scheduler) ScheduleTask(tokenID string, task *RefreshTask) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	s.tasks[tokenID] = task
	
	s.logger.InfoLog("[Scheduler] Scheduled refresh task for token %s", tokenID)
	return nil
}

// processTasks processes refresh tasks
func (s *Scheduler) processTasks() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			s.checkAndRunTasks()
		case <-s.ctx.Done():
			return
		}
	}
}

// checkAndRunTasks checks and runs tasks that are due
func (s *Scheduler) checkAndRunTasks() {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	now := time.Now()
	
	for tokenID, task := range s.tasks {
		if task.ShouldRun(now) {
			s.logger.DebugLog("[Scheduler] Running refresh task for token %s", tokenID)
			
			if err := task.Run(); err != nil {
				s.logger.ErrorLog("[Scheduler] Refresh task failed for token %s: %v", tokenID, err)
			}
		}
	}
}
```

#### Step 2.2: Create Refresh Task

**New File:** `internal/token/refresh/refresh_task.go`

```go
package refresh

import (
	"context"
	"fmt"
	"time"
	"github.com/sunbankio/qwencoder-proxy/internal/logging"
	"github.com/sunbankio/qwencoder-proxy/internal/token"
)

// RefreshTask represents a token refresh task
type RefreshTask struct {
	TokenID        string
	ProviderID     string
	Refresher      token.ProviderRefresh
	LastRun        time.Time
	NextRun        time.Time
	Interval       time.Duration
	logger         logging.Logger
}

// NewRefreshTask creates a new refresh task
func NewRefreshTask(tokenID, providerID string, refresher token.ProviderRefresh, interval time.Duration, logger logging.Logger) *RefreshTask {
	return &RefreshTask{
		TokenID:    tokenID,
		ProviderID: providerID,
		Refresher:  refresher,
		LastRun:    time.Time{},
		NextRun:    time.Now().Add(interval),
		Interval:   interval,
		logger:     logger,
	}
}

// ShouldRun checks if the task should run now
func (rt *RefreshTask) ShouldRun(now time.Time) bool {
	return now.After(rt.NextRun) || rt.LastRun.IsZero()
}

// Run executes the refresh task
func (rt *RefreshTask) Run() error {
	rt.logger.InfoLog("[RefreshTask] Refreshing token %s for provider %s", rt.TokenID, rt.ProviderID)
	
	// Execute refresh
	updatedToken, err := rt.Refresher.RefreshToken(context.Background(), rt.TokenID)
	if err != nil {
		return fmt.Errorf("failed to refresh token %s: %w", rt.TokenID, err)
	}
	
	// Update last run time
	rt.LastRun = time.Now()
	rt.NextRun = time.Now().Add(rt.Interval)
	
	rt.logger.InfoLog("[RefreshTask] Successfully refreshed token %s", rt.TokenID)
	return nil
}
```

#### Step 2.3: Create Scheduler Config

**New File:** `internal/token/refresh/scheduler_config.go`

```go
package refresh

import "time"

// SchedulerConfig holds configuration for the refresh scheduler
type SchedulerConfig struct {
	// CheckInterval is how often to check for tasks to run
	CheckInterval time.Duration
	
	// MaxConcurrentTasks is the maximum number of concurrent refresh tasks
	MaxConcurrentTasks int
	
	// TaskTimeout is the maximum time a task can run
	TaskTimeout time.Duration
}

// DefaultSchedulerConfig returns sensible defaults
func DefaultSchedulerConfig() SchedulerConfig {
	return SchedulerConfig{
		CheckInterval:       30 * time.Second,
		MaxConcurrentTasks: 5,
		TaskTimeout:         5 * time.Minute,
	}
}
```

### Phase 3: Create Health Package (Day 2)

#### Step 3.1: Create Health Tracker

**New File:** `internal/token/health/tracker.go`

```go
package health

import (
	"context"
	"fmt"
	"sync"
	"time"
	"github.com/sunbankio/qwencoder-proxy/internal/logging"
	"github.com/sunbankio/qwencoder-proxy/internal/token"
)

// Tracker tracks health status of tokens
type Tracker struct {
	store   token.TokenStore
	logger  logging.Logger
	mu      sync.RWMutex
	config  TrackerConfig
}

// NewTracker creates a new health tracker
func NewTracker(store token.TokenStore, logger logging.Logger, config TrackerConfig) *Tracker {
	return &Tracker{
		store:  store,
		logger: logger,
		config: config,
	}
}

// Start starts the health tracker
func (ht *Tracker) Start(ctx context.Context) error {
	ht.logger.InfoLog("[HealthTracker] Starting health tracker")
	
	// Start periodic health checks
	go ht.runHealthChecks(ctx)
	
	return nil
}

// Stop stops the health tracker
func (ht *Tracker) Stop() {
	ht.logger.InfoLog("[HealthTracker] Stopping health tracker")
}

// MarkHealthy marks a token as healthy
func (ht *Tracker) MarkHealthy(tokenID string) error {
	ht.mu.Lock()
	defer ht.mu.Unlock()
	
	if err := ht.store.MarkTokenHealthy(tokenID); err != nil {
		return fmt.Errorf("failed to mark token %s as healthy: %w", tokenID, err)
	}
	
	ht.logger.DebugLog("[HealthTracker] Marked token %s as healthy", tokenID)
	return nil
}

// MarkUnhealthy marks a token as unhealthy
func (ht *Tracker) MarkUnhealthy(tokenID string, err error) error {
	ht.mu.Lock()
	defer ht.mu.Unlock()
	
	if storeErr := ht.store.MarkTokenUnhealthy(tokenID, err); storeErr != nil {
		return fmt.Errorf("failed to mark token %s as unhealthy: %w", tokenID, storeErr)
	}
	
	ht.logger.WarnLog("[HealthTracker] Marked token %s as unhealthy: %v", tokenID, err)
	return nil
}

// runHealthChecks runs periodic health checks
func (ht *Tracker) runHealthChecks(ctx context.Context) {
	ticker := time.NewTicker(ht.config.CheckInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			ht.checkTokenHealth()
		case <-ctx.Done():
			return
		}
	}
}

// checkTokenHealth checks the health of all tokens
func (ht *Tracker) checkTokenHealth() {
	tokensMap, err := ht.store.Load()
	if err != nil {
		ht.logger.ErrorLog("[HealthTracker] Failed to load tokens: %v", err)
		return
	}
	
	for _, token := range tokensMap {
		// Check if token needs health check
		if time.Since(time.UnixMilli(token.LastUsed)) > ht.config.HealthCheckInterval {
			// Perform health check
			ht.checkToken(token.ID)
		}
	}
}

// checkToken checks the health of a specific token
func (ht *Tracker) checkToken(tokenID string) {
	// Implementation would call provider health check
	// This is a placeholder for the actual implementation
	ht.logger.DebugLog("[HealthTracker] Checking health of token %s", tokenID)
}
```

#### Step 3.2: Create Tracker Config

**New File:** `internal/token/health/tracker_config.go`

```go
package health

import "time"

// TrackerConfig holds configuration for the health tracker
type TrackerConfig struct {
	// CheckInterval is how often to check token health
	CheckInterval time.Duration
	
	// HealthCheckTimeout is the timeout for health checks
	HealthCheckTimeout time.Duration
	
	// MaxErrorCount is the maximum errors before marking unhealthy
	MaxErrorCount int
	
	// HealthScoreDecay is how much to decay health score over time
	HealthScoreDecay float64
}

// DefaultTrackerConfig returns sensible defaults
func DefaultTrackerConfig() TrackerConfig {
	return TrackerConfig{
		CheckInterval:       1 * time.Minute,
		HealthCheckTimeout:  30 * time.Second,
		MaxErrorCount:      3,
		HealthScoreDecay:    0.1,
	}
}
```

### Phase 4: Create Proxy Health Package (Day 2)

#### Step 4.1: Create Proxy Health Tracker

**New File:** `internal/token/proxy/health_tracker.go`

```go
package proxy

import (
	"context"
	"fmt"
	"sync"
	"time"
	"github.com/sunbankio/qwencoder-proxy/internal/logging"
	"github.com/sunbankio/qwencoder-proxy/internal/token"
)

// HealthTracker tracks the health of proxy connections
type HealthTracker struct {
	store  token.TokenStore
	logger logging.Logger
	mu     sync.RWMutex
	config HealthTrackerConfig
}

// NewHealthTracker creates a new proxy health tracker
func NewHealthTracker(store token.TokenStore, logger logging.Logger, config HealthTrackerConfig) *HealthTracker {
	return &HealthTracker{
		store:  store,
		logger: logger,
		config: config,
	}
}

// TestProxy tests a proxy connection
func (pht *HealthTracker) TestProxy(ctx context.Context, tokenID string) (time.Duration, error) {
	pht.mu.RLock()
	defer pht.mu.RUnlock()
	
	// Get token
	token, err := pht.store.GetToken(tokenID)
	if err != nil {
		return 0, fmt.Errorf("failed to get token %s: %w", tokenID, err)
	}
	
	// Get proxy config
	proxy := token.Proxy
	if proxy == nil {
		return 0, nil // No proxy configured
	}
	
	// Test proxy connection
	start := time.Now()
	
	// This is a placeholder - actual implementation would test the proxy
	latency := time.Since(start)
	
	pht.logger.DebugLog("[ProxyHealthTracker] Tested proxy for token %s: latency=%v", tokenID, latency)
	
	return latency, nil
}

// UpdateProxyHealth updates the health score of a proxy
func (pht *HealthTracker) UpdateProxyHealth(tokenID string, latency time.Duration, success bool) error {
	pht.mu.Lock()
	defer pht.mu.Unlock()
	
	// Get token
	token, err := pht.store.GetToken(tokenID)
	if err != nil {
		return fmt.Errorf("failed to get token %s: %w", tokenID, err)
	}
	
	// Update proxy health score
	newScore := pht.calculateHealthScore(token.ProxyHealthScore, latency, success)
	
	// Update token
	token.ProxyHealthScore = newScore
	
	// Save token
	if err := pht.store.UpdateToken(token.ID, func(t *token.ProviderToken) {
		*t = *token
	}); err != nil {
		return fmt.Errorf("failed to update token %s: %w", tokenID, err)
	}
	
	pht.logger.DebugLog("[ProxyHealthTracker] Updated proxy health for token %s: score=%v", tokenID, newScore)
	return nil
}

// calculateHealthScore calculates a new health score
func (pht *HealthTracker) calculateHealthScore(currentScore float64, latency time.Duration, success bool) float64 {
	if !success {
		// Penalize failures
		return currentScore * 0.5
	}
	
	// Adjust based on latency
	latencyMs := latency.Milliseconds()
	if latencyMs < 1000 {
		// Fast connection - increase score
		return currentScore + 0.1
	} else if latencyMs < 3000 {
		// Moderate latency - maintain score
		return currentScore
	} else {
		// Slow connection - decrease score
		return currentScore - 0.1
	}
}
```

#### Step 4.2: Create Proxy Tester

**New File:** `internal/token/proxy/proxy_tester.go`

```go
package proxy

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"
	"github.com/sunbankio/qwencoder-proxy/internal/logging"
	"github.com/sunbankio/qwencoder-proxy/internal/token"
)

// ProxyTester tests proxy connections
type ProxyTester struct {
	logger logging.Logger
	client *http.Client
}

// NewProxyTester creates a new proxy tester
func NewProxyTester(logger logging.Logger) *ProxyTester {
	return &ProxyTester{
		logger: logger,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Test tests a proxy connection
func (pt *ProxyTester) Test(ctx context.Context, proxy *token.ProxyConfig) (time.Duration, error) {
	if proxy == nil {
		return 0, nil
	}
	
	// Create proxy URL
	proxyURL := fmt.Sprintf("%s://%s:%d", proxy.Type, proxy.Host, proxy.Port)
	
	// Test connection
	start := time.Now()
	
	// Create request through proxy
	req, err := http.NewRequestWithContext(ctx, "GET", "https://www.google.com", nil)
	if err != nil {
		return 0, fmt.Errorf("failed to create request: %w", err)
	}
	
	// Set proxy
	transport := &http.Transport{
		Proxy: http.ProxyURL(proxyURL),
	}
	pt.client.Transport = transport
	
	// Execute request
	resp, err := pt.client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("proxy connection failed: %w", err)
	}
	defer resp.Body.Close()
	
	latency := time.Since(start)
	pt.logger.DebugLog("[ProxyTester] Proxy test completed: latency=%v", latency)
	
	return latency, nil
}
```

### Phase 5: Update MultiTokenManager (Day 3)

#### Step 5.1: Simplify MultiTokenManager

**Modify File:** `internal/token/multi_token_manager.go`

**Location:** Replace entire file with simplified version

```go
package token

import (
	"context"
	"fmt"
	"github.com/sunbankio/qwencoder-proxy/internal/logging"
	"github.com/sunbankio/qwencoder-proxy/internal/token/manager"
	"github.com/sunbankio/qwencoder-proxy/internal/token/refresh"
	"github.com/sunbankio/qwencoder-proxy/internal/token/health"
	"github.com/sunbankio/qwencoder-proxy/internal/token/proxy"
	"github.com/sunbankio/qwencoder-proxy/internal/token/extraction"
)

// MultiTokenManager coordinates all token management components
type MultiTokenManager struct {
	managerFactory    *manager.ManagerFactory
	refreshScheduler  *refresh.Scheduler
	healthTracker     *health.Tracker
	proxyHealthTracker *proxy.HealthTracker
	emailManager     *extraction.EmailExtractionManager
	logger           logging.Logger
	initialized       bool
}

// NewMultiTokenManager creates a new multi-token manager
func NewMultiTokenManager(logger logging.Logger) *MultiTokenManager {
	return &MultiTokenManager{
		logger: logger,
	}
}

// Initialize initializes all components
func (mtm *MultiTokenManager) Initialize() error {
	mtm.logger.InfoLog("[MultiTokenManager] Initializing...")
	
	// Create manager factory
	storeFactory := NewStoreFactory(mtm.logger)
	mtm.managerFactory = manager.NewManagerFactory(storeFactory, mtm.logger)
	
	// Create email extraction manager
	mtm.emailManager = extraction.NewEmailExtractionManager(mtm.logger)
	
	// Register email extractors
	mtm.emailManager.RegisterExtractor(extraction.NewQwenEmailExtractor(nil, mtm.logger))
	mtm.emailManager.RegisterExtractor(extraction.NewGeminiEmailExtractor(nil, mtm.logger))
	mtm.emailManager.RegisterExtractor(extraction.NewKiroEmailExtractor(nil, mtm.logger))
	mtm.emailManager.RegisterExtractor(extraction.NewIFlowEmailExtractor(nil, mtm.logger))
	
	mtm.initialized = true
	mtm.logger.InfoLog("[MultiTokenManager] Initialization complete")
	return nil
}

// Start starts all components
func (mtm *MultiTokenManager) Start() error {
	if !mtm.initialized {
		return fmt.Errorf("multi-token manager not initialized")
	}
	
	mtm.logger.InfoLog("[MultiTokenManager] Starting components...")
	
	// Start refresh scheduler
	if err := mtm.refreshScheduler.Start(context.Background()); err != nil {
		return fmt.Errorf("failed to start refresh scheduler: %w", err)
	}
	
	// Start health tracker
	if err := mtm.healthTracker.Start(context.Background()); err != nil {
		return fmt.Errorf("failed to start health tracker: %w", err)
	}
	
	mtm.logger.InfoLog("[MultiTokenManager] All components started")
	return nil
}

// Stop stops all components
func (mtm *MultiTokenManager) Stop() {
	mtm.logger.InfoLog("[MultiTokenManager] Stopping components...")
	
	// Stop refresh scheduler
	mtm.refreshScheduler.Stop()
	
	// Stop health tracker
	mtm.healthTracker.Stop()
	
	mtm.logger.InfoLog("[MultiTokenManager] All components stopped")
}

// GetManager returns a token manager for a provider
func (mtm *MultiTokenManager) GetManager(providerID string) (*manager.TokenManager, error) {
	return mtm.managerFactory.GetManager(providerID)
}

// GetTokenStore returns a token store for a provider
func (mtm *MultiTokenManager) GetTokenStore(providerID string) (TokenStore, error) {
	storeFactory := NewStoreFactory(mtm.logger)
	return storeFactory.GetStore(providerID)
}
```

---

## Testing

### Unit Tests

**New File:** `internal/token/manager/token_manager_test.go`

```go
package manager

import (
	"testing"
	"github.com/stretchr/testify/assert"
	"github.com/sunbankio/qwencoder-proxy/internal/token"
)

func TestNewTokenManager(t *testing.T) {
	store := &MockStore{}
	strategy := &RandomStrategy{}
	logger := logging.NewLogger()
	
	manager := NewTokenManager(store, strategy, logger)
	
	assert.NotNil(t, manager)
	assert.Equal(t, store, manager.store)
	assert.Equal(t, strategy, manager.strategy)
}

func TestSelectToken(t *testing.T) {
	store := &MockStore{
		tokens: map[string]token.ProviderToken{
			"token1": {ID: "token1", Email: "test1@example.com", Healthy: true},
			"token2": {ID: "token2", Email: "test2@example.com", Healthy: true},
		},
	}
	strategy := &RandomStrategy{}
	manager := NewTokenManager(store, strategy, nil)
	
	token, err := manager.SelectToken()
	
	assert.NoError(t, err)
	assert.NotNil(t, token)
	assert.Contains(t, []string{"token1", "token2"}, token.ID)
}

func TestUpdateToken(t *testing.T) {
	store := &MockStore{}
	strategy := &RandomStrategy{}
	manager := NewTokenManager(store, strategy, nil)
	
	token := &token.ProviderToken{ID: "token1", Email: "updated@example.com"}
	err := manager.UpdateToken(token)
	
	assert.NoError(t, err)
	
	// Verify token was updated
	updated, _ := store.GetToken("token1")
	assert.Equal(t, "updated@example.com", updated.Email)
}
```

---

## Verification Checklist

### Phase 1: Manager Package
- [ ] `internal/token/manager/token_manager.go` created
- [ ] `internal/token/manager/manager_factory.go` created
- [ ] `internal/token/manager/manager_config.go` created
- [ ] Unit tests pass

### Phase 2: Refresh Package
- [ ] `internal/token/refresh/scheduler.go` created
- [ ] `internal/token/refresh/refresh_task.go` created
- [ ] `internal/token/refresh/scheduler_config.go` created
- [ ] Unit tests pass

### Phase 3: Health Package
- [ ] `internal/token/health/tracker.go` created
- [ ] `internal/token/health/tracker_config.go` created
- [ ] Unit tests pass

### Phase 4: Proxy Health Package
- [ ] `internal/token/proxy/health_tracker.go` created
- [ ] `internal/token/proxy/proxy_tester.go` created
- [ ] Unit tests pass

### Phase 5: Integration
- [ ] `multi_token_manager.go` simplified
- [ ] All imports updated
- [ ] All components integrated
- [ ] Integration tests pass

---

## Impact

**Positive:**
- Separation of concerns - each component has single responsibility
- Easier to test individual components
- Better maintainability
- Reduced coupling between components
- Easier to add new features
- Clearer code organization

**Code Volume Changes:**
- Removed: ~500 lines from `multi_token_manager.go`
- Added: ~800 lines across 6 new packages
- Net increase: ~300 lines (but with much better organization)

**Risk:**
- Medium - requires careful integration testing
- Breaking change to `MultiTokenManager` API
- Need to update all callers

**Side Effects:**
- New package structure
- Changes to import paths throughout codebase
- Need to update all references to `MultiTokenManager`
- Better test coverage possible

---

## Future Enhancements

1. **Component Lifecycle:**
   - Better initialization and shutdown
   - Component health checks
   - Dependency injection

2. **Event System:**
   - Component events (start, stop, error)
   - Event bus for communication
   - Async event processing

3. **Metrics Collection:**
   - Per-component metrics
   - Performance tracking
   - Resource usage monitoring

4. **Configuration Management:**
   - Dynamic configuration updates
   - Hot reloading of config
   - Configuration validation

5. **Observability:**
   - Structured logging
   - Distributed tracing
   - Component status dashboards
