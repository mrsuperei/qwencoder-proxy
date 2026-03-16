// Package auth provides multi-token manager for managing OAuth tokens across providers
package token

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/sunbankio/qwencoder-proxy/internal/logging"
)

// ProviderConfig holds provider-specific configuration
type ProviderConfig struct {
	ID           string
	ClientID     string
	ClientSecret string
	AuthURL      string
	TokenURL     string
	Flow         string
	Scopes       []string
}

// MultiTokenManager manages all multi-token components
type MultiTokenManager struct {
	stores              map[string]*SQLiteStore
	managers            map[string]*TokenManager
	refreshers          map[string]ProviderRefresh
	schedulers          map[string]*RefreshScheduler
	extractors          map[string]EmailExtractor
	emailManager        *EmailExtractionManager
	healthTrackers      map[string]*HealthTracker
	proxyHealthTrackers map[string]*ProxyHealthTracker
	strategyFactory     *StrategyFactory
	mu                  sync.RWMutex
	logger              logging.Logger
	httpClient          *http.Client
	clientFactory       ProxyClientFactory
	credentialsDir      string
	dbPath              string // SQLite database path
	initialized         bool
}

// NewMultiTokenManager creates a new MultiTokenManager
func NewMultiTokenManager(logger logging.Logger) *MultiTokenManager {
	if logger == nil {
		logger = logging.NewLogger()
	}

	return &MultiTokenManager{
		stores:              make(map[string]*SQLiteStore),
		managers:            make(map[string]*TokenManager),
		refreshers:          make(map[string]ProviderRefresh),
		schedulers:          make(map[string]*RefreshScheduler),
		extractors:          make(map[string]EmailExtractor),
		healthTrackers:      make(map[string]*HealthTracker),
		proxyHealthTrackers: make(map[string]*ProxyHealthTracker),
		strategyFactory:     NewStrategyFactory(),
		logger:              logger,
		httpClient:          &http.Client{Timeout: 30 * time.Second},
		credentialsDir:      ".credentials",
		dbPath:              filepath.Join(".credentials", "tokens.db"),
	}
}

// SetCredentialsDir sets the credentials directory
func (mtm *MultiTokenManager) SetCredentialsDir(dir string) {
	mtm.mu.Lock()
	defer mtm.mu.Unlock()
	mtm.credentialsDir = dir
}

// SetHTTPClient sets the HTTP client for email extraction
func (mtm *MultiTokenManager) SetHTTPClient(client *http.Client) {
	mtm.mu.Lock()
	defer mtm.mu.Unlock()
	mtm.httpClient = client
}

// SetClientFactory sets the proxy-aware HTTP client factory
func (mtm *MultiTokenManager) SetClientFactory(clientFactory ProxyClientFactory) {
	mtm.mu.Lock()
	defer mtm.mu.Unlock()
	mtm.clientFactory = clientFactory
}

// SetStorageConfig sets the storage backend configuration
func (mtm *MultiTokenManager) SetStorageConfig(dbPath string) {
	mtm.mu.Lock()
	defer mtm.mu.Unlock()
	mtm.dbPath = dbPath
	mtm.logger.InfoLog("[MultiTokenManager] Storage config updated: dbPath=%s", dbPath)
}

// Initialize initializes all components
func (mtm *MultiTokenManager) Initialize() error {
	mtm.mu.Lock()
	defer mtm.mu.Unlock()

	if mtm.initialized {
		return nil
	}

	mtm.logger.InfoLog("[MultiTokenManager] Initializing multi-token manager...")

	// Create email extraction manager
	mtm.emailManager = NewEmailExtractionManager(mtm.logger)

	// Register email extractors
	mtm.emailManager.RegisterExtractor(NewQwenEmailExtractor(mtm.httpClient, mtm.logger))
	mtm.emailManager.RegisterExtractor(NewGeminiEmailExtractor(mtm.httpClient, mtm.logger))
	mtm.emailManager.RegisterExtractor(NewKiroEmailExtractor(mtm.httpClient, mtm.logger))
	mtm.emailManager.RegisterExtractor(NewIFlowEmailExtractor(mtm.httpClient, mtm.logger))

	mtm.initialized = true
	mtm.logger.InfoLog("[MultiTokenManager] Initialization complete")
	return nil
}

// Start starts the refresh schedulers for all registered providers
func (mtm *MultiTokenManager) Start() error {
	mtm.mu.Lock()
	defer mtm.mu.Unlock()

	if !mtm.initialized {
		return fmt.Errorf("multi-token manager not initialized")
	}

	mtm.logger.InfoLog("[MultiTokenManager] Starting refresh schedulers...")

	// Start schedulers for all registered providers
	for providerID, scheduler := range mtm.schedulers {
		scheduler.Start()
		mtm.logger.InfoLog("[MultiTokenManager] Started scheduler for provider: %s", providerID)
	}

	return nil
}

// Stop stops all components
func (mtm *MultiTokenManager) Stop() {
	mtm.mu.Lock()
	defer mtm.mu.Unlock()

	mtm.logger.InfoLog("[MultiTokenManager] Stopping multi-token manager...")

	// Stop all schedulers
	for providerID, scheduler := range mtm.schedulers {
		scheduler.Stop()
		mtm.logger.InfoLog("[MultiTokenManager] Stopped scheduler for provider: %s", providerID)
	}

	mtm.logger.InfoLog("[MultiTokenManager] Stopped")
}

// GetTokenStore returns the token store for a provider
func (mtm *MultiTokenManager) GetTokenStore(providerID string) (*SQLiteStore, error) {
	mtm.mu.RLock()
	store, ok := mtm.stores[providerID]
	mtm.mu.RUnlock()

	if ok {
		mtm.logger.DebugLog("[MultiTokenManager] Returning existing token store for provider: %s", providerID)
		return store, nil
	}

	// Create new store
	mtm.mu.Lock()
	defer mtm.mu.Unlock()

	// Check again in case another goroutine created it
	if store, ok := mtm.stores[providerID]; ok {
		mtm.logger.DebugLog("[MultiTokenManager] Returning existing token store for provider: %s (after re-check)", providerID)
		return store, nil
	}

	mtm.logger.InfoLog("[MultiTokenManager] Creating new token store for provider: %s", providerID)

	// Ensure database directory exists
	dbDir := filepath.Dir(mtm.dbPath)
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory %s: %w", dbDir, err)
	}

	// Create SQLite store
	sqliteStore, err := NewSQLiteStore(mtm.dbPath, providerID, mtm.logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create SQLite store: %w", err)
	}

	mtm.stores[providerID] = sqliteStore

	// Create health tracker for this provider
	mtm.healthTrackers[providerID] = NewHealthTracker(sqliteStore, mtm.logger)

	// Create proxy health tracker for this provider
	mtm.proxyHealthTrackers[providerID] = NewProxyHealthTracker(mtm.logger, 5, 5*time.Minute)

	mtm.logger.InfoLog("[MultiTokenManager] Created token store for provider: %s", providerID)
	return sqliteStore, nil
}

// GetTokenManager returns the token manager for a provider
func (mtm *MultiTokenManager) GetTokenManager(providerID string) (*TokenManager, error) {
	mtm.mu.RLock()
	manager, ok := mtm.managers[providerID]
	mtm.mu.RUnlock()

	if ok {
		mtm.logger.DebugLog("[MultiTokenManager] Returning existing token manager for provider: %s", providerID)
		return manager, nil
	}

	mtm.logger.InfoLog("[MultiTokenManager] Creating new token manager for provider: %s", providerID)

	// Get or create store
	store, err := mtm.GetTokenStore(providerID)
	if err != nil {
		return nil, err
	}

	mtm.logger.DebugLog("[MultiTokenManager] Store for %s has %d tokens", providerID, store.GetTokenCount())

	// Create strategy from store settings
	settings, err := store.GetSettings()
	if err != nil {
		return nil, fmt.Errorf("failed to get store settings: %w", err)
	}
	strategy, err := mtm.strategyFactory.CreateStrategy(settings.SelectionStrategy)
	if err != nil {
		return nil, fmt.Errorf("failed to create selection strategy: %w", err)
	}

	mtm.mu.Lock()
	defer mtm.mu.Unlock()

	// Check again in case another goroutine created it
	if manager, ok := mtm.managers[providerID]; ok {
		return manager, nil
	}

	// Get proxy health tracker for this provider (already created by GetTokenStore)
	proxyHealthTracker, ok := mtm.proxyHealthTrackers[providerID]
	if !ok {
		proxyHealthTracker = NewProxyHealthTracker(mtm.logger, 5, 5*time.Minute)
		mtm.proxyHealthTrackers[providerID] = proxyHealthTracker
	}

	manager = NewTokenManager(store, strategy, mtm.logger, mtm.clientFactory, proxyHealthTracker)
	mtm.managers[providerID] = manager

	mtm.logger.InfoLog("[MultiTokenManager] Created token manager for provider: %s", providerID)
	return manager, nil
}

// GetHealthTracker returns the health tracker for a provider
func (mtm *MultiTokenManager) GetHealthTracker(providerID string) (*HealthTracker, error) {
	mtm.mu.RLock()
	tracker, ok := mtm.healthTrackers[providerID]
	mtm.mu.RUnlock()

	if ok {
		return tracker, nil
	}

	// Get or create store
	store, err := mtm.GetTokenStore(providerID)
	if err != nil {
		return nil, err
	}

	mtm.mu.Lock()
	defer mtm.mu.Unlock()

	// Check again in case another goroutine created it
	if tracker, ok := mtm.healthTrackers[providerID]; ok {
		return tracker, nil
	}

	tracker = NewHealthTracker(store, mtm.logger)
	mtm.healthTrackers[providerID] = tracker

	return tracker, nil
}

// GetProxyHealthTracker returns the proxy health tracker for a provider
func (mtm *MultiTokenManager) GetProxyHealthTracker(providerID string) (*ProxyHealthTracker, error) {
	mtm.mu.RLock()
	tracker, ok := mtm.proxyHealthTrackers[providerID]
	mtm.mu.RUnlock()

	if ok {
		return tracker, nil
	}

	mtm.mu.Lock()
	defer mtm.mu.Unlock()

	// Check again in case another goroutine created it
	if tracker, ok := mtm.proxyHealthTrackers[providerID]; ok {
		return tracker, nil
	}

	tracker = NewProxyHealthTracker(mtm.logger, 5, 5*time.Minute)
	mtm.proxyHealthTrackers[providerID] = tracker

	return tracker, nil
}

// GetEmailExtractionManager returns the email extraction manager
func (mtm *MultiTokenManager) GetEmailExtractionManager() *EmailExtractionManager {
	mtm.mu.RLock()
	defer mtm.mu.RUnlock()
	return mtm.emailManager
}

// RegisterProvider registers a provider with the multi-token system
func (mtm *MultiTokenManager) RegisterProvider(providerID string, config ProviderConfig) error {
	mtm.logger.InfoLog("[MultiTokenManager] Registering provider: %s", providerID)

	// Create token store for provider
	_, err := mtm.GetTokenStore(providerID)
	if err != nil {
		return fmt.Errorf("failed to create token store for provider %s: %w", providerID, err)
	}

	// Create token manager for provider
	_, err = mtm.GetTokenManager(providerID)
	if err != nil {
		return fmt.Errorf("failed to create token manager for provider %s: %w", providerID, err)
	}

	return nil
}

// RegisterRefresher registers a provider-specific refresher and creates refresh infrastructure
func (mtm *MultiTokenManager) RegisterRefresher(providerID string, refresher ProviderRefresh) error {
	mtm.mu.Lock()
	defer mtm.mu.Unlock()

	mtm.logger.InfoLog("[MultiTokenManager] Registering refresher for provider: %s", providerID)

	// Get or create store
	store, ok := mtm.stores[providerID]
	if !ok {
		return fmt.Errorf("provider store not found: %s", providerID)
	}

	// Create refresh coordinator for this provider with client factory
	coordinator := NewRefreshCoordinator(store, 3, mtm.logger, mtm.clientFactory)
	coordinator.RegisterRefresher(refresher)

	// Create refresh scheduler for this provider
	scheduler := NewRefreshScheduler(coordinator, store, 5*time.Minute, mtm.logger)

	mtm.refreshers[providerID] = refresher
	mtm.schedulers[providerID] = scheduler

	mtm.logger.InfoLog("[MultiTokenManager] Registered refresher and created scheduler for provider: %s", providerID)
	return nil
}

// SaveToken saves a token for a provider (used by OAuth callbacks)
func (mtm *MultiTokenManager) SaveToken(providerID string, token ProviderToken) error {
	mtm.logger.InfoLog("[MultiTokenManager] Saving token for provider: %s, email: %s", providerID, token.Email)

	store, err := mtm.GetTokenStore(providerID)
	if err != nil {
		return fmt.Errorf("failed to get token store: %w", err)
	}

	if err := store.AddToken(token); err != nil {
		return fmt.Errorf("failed to add token to store: %w", err)
	}

	// Register for refresh if token has refresh token
	if token.RefreshToken != "" {
		// Note: Token refresh is handled by the RefreshCoordinator/RefreshScheduler
		// which are created when RegisterRefresher is called
		mtm.logger.DebugLog("[MultiTokenManager] Token has refresh capability")
	}

	return nil
}

// SelectToken selects a token for a provider
func (mtm *MultiTokenManager) SelectToken(providerID string) (*ProviderToken, error) {
	manager, err := mtm.GetTokenManager(providerID)
	if err != nil {
		return nil, fmt.Errorf("failed to get token manager: %w", err)
	}

	return manager.SelectToken()
}

// ExtractEmail extracts email for a provider using the email extraction manager
func (mtm *MultiTokenManager) ExtractEmail(ctx context.Context, providerID string, tokenResponse map[string]interface{}, accessToken string) (string, error) {
	if mtm.emailManager == nil {
		return "", fmt.Errorf("email extraction manager not initialized")
	}

	return mtm.emailManager.ExtractEmail(ctx, providerID, tokenResponse, accessToken)
}

// ListProviders returns all registered provider IDs
func (mtm *MultiTokenManager) ListProviders() []string {
	mtm.mu.RLock()
	defer mtm.mu.RUnlock()

	providers := make([]string, 0, len(mtm.stores))
	for providerID := range mtm.stores {
		providers = append(providers, providerID)
	}
	return providers
}

// GetProviderTokenCount returns the number of tokens for a provider
func (mtm *MultiTokenManager) GetProviderTokenCount(providerID string) (int, error) {
	store, err := mtm.GetTokenStore(providerID)
	if err != nil {
		return 0, err
	}
	return store.GetTokenCount(), nil
}

// RemoveToken removes a token from a provider
func (mtm *MultiTokenManager) RemoveToken(providerID, tokenID string) error {
	store, err := mtm.GetTokenStore(providerID)
	if err != nil {
		return fmt.Errorf("failed to get token store: %w", err)
	}

	return store.RemoveToken(tokenID)
}

// ClearProviderTokens clears all tokens for a provider
func (mtm *MultiTokenManager) ClearProviderTokens(providerID string) error {
	mtm.mu.Lock()
	defer mtm.mu.Unlock()

	store, ok := mtm.stores[providerID]
	if !ok {
		return fmt.Errorf("provider not found: %s", providerID)
	}

	// Clear all tokens
	if err := store.Clear(); err != nil {
		return fmt.Errorf("failed to clear store: %w", err)
	}

	// Remove from managers
	delete(mtm.managers, providerID)

	mtm.logger.InfoLog("[MultiTokenManager] Cleared all tokens for provider: %s", providerID)
	return nil
}

// ReportTokenSuccess reports a successful token usage
func (mtm *MultiTokenManager) ReportTokenSuccess(providerID, tokenID string) error {
	tracker, err := mtm.GetHealthTracker(providerID)
	if err != nil {
		return err
	}

	return tracker.ReportSuccess(tokenID)
}

// ReportTokenFailure reports a failed token usage
func (mtm *MultiTokenManager) ReportTokenFailure(providerID, tokenID string, err error) error {
	tracker, err := mtm.GetHealthTracker(providerID)
	if err != nil {
		return err
	}

	return tracker.ReportFailure(tokenID, err)
}
