package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/sunbankio/qwencoder-proxy/logging"
	"golang.org/x/oauth2"
)

// RefreshRequest represents a token refresh request
type RefreshRequest struct {
	TokenID     string
	ProviderID  string
	Priority    int // Lower value = higher priority
	ScheduledAt time.Time
	Token       ProviderToken
}

// RefreshResult represents the result of a token refresh
type RefreshResult struct {
	TokenID  string
	Success  bool
	NewToken *ProviderToken
	Error    error
	Duration time.Duration
}

// ProviderRefresh defines the interface for provider-specific refresh logic
type ProviderRefresh interface {
	// RefreshToken refreshes a token and returns the updated token
	RefreshToken(ctx context.Context, token ProviderToken) (ProviderToken, error)
	// ProviderID returns the provider identifier
	ProviderID() string
}

// RefreshCoordinator manages token refresh operations with a worker pool
type RefreshCoordinator struct {
	store         *MultiTokenStore
	refreshers    map[string]ProviderRefresh // ProviderID -> ProviderRefresh
	requestQueue  chan RefreshRequest
	resultQueue   chan RefreshResult
	workers       int
	ctx           context.Context
	cancel        context.CancelFunc
	wg            sync.WaitGroup
	mu            sync.RWMutex
	logger        logging.Logger
	workerStatus  map[int]bool // Worker ID -> Active status
	clientFactory ProxyClientFactory
}

// NewRefreshCoordinator creates a new RefreshCoordinator
func NewRefreshCoordinator(store *MultiTokenStore, workers int, logger logging.Logger, clientFactory ProxyClientFactory) *RefreshCoordinator {
	if workers <= 0 {
		workers = 3 // Default worker count
	}
	if logger == nil {
		logger = logging.NewLogger()
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &RefreshCoordinator{
		store:         store,
		refreshers:    make(map[string]ProviderRefresh),
		requestQueue:  make(chan RefreshRequest, 100), // Buffered queue
		resultQueue:   make(chan RefreshResult, 100),
		workers:       workers,
		ctx:           ctx,
		cancel:        cancel,
		logger:        logger,
		workerStatus:  make(map[int]bool),
		clientFactory: clientFactory,
	}
}

// Start starts the refresh coordinator workers
func (rc *RefreshCoordinator) Start() error {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	for i := 0; i < rc.workers; i++ {
		rc.wg.Add(1)
		rc.workerStatus[i] = true
		go rc.worker(i)
	}

	rc.logger.InfoLog("[RefreshCoordinator] Started %d workers", rc.workers)
	return nil
}

// Stop gracefully stops the refresh coordinator
func (rc *RefreshCoordinator) Stop() {
	rc.mu.Lock()
	// Cancel context first to signal workers to stop
	rc.cancel()

	// Close queues to signal workers to stop
	close(rc.requestQueue)
	close(rc.resultQueue)
	rc.mu.Unlock()

	// Wait for workers to finish (must be done without holding the lock)
	rc.wg.Wait()

	rc.logger.InfoLog("[RefreshCoordinator] Stopped all workers")
}

// worker processes refresh requests from the queue
func (rc *RefreshCoordinator) worker(workerID int) {
	defer rc.wg.Done()
	defer func() {
		rc.mu.Lock()
		rc.workerStatus[workerID] = false
		rc.mu.Unlock()
	}()

	for {
		select {
		case <-rc.ctx.Done():
			rc.logger.DebugLog("[RefreshCoordinator] Worker %d shutting down", workerID)
			return

		case request, ok := <-rc.requestQueue:
			if !ok {
				// Queue closed
				return
			}

			rc.processRequest(request, workerID)
		}
	}
}

// processRequest handles a single refresh request
func (rc *RefreshCoordinator) processRequest(request RefreshRequest, workerID int) {
	startTime := time.Now()
	result := RefreshResult{
		TokenID: request.TokenID,
	}

	rc.logger.DebugLog("[RefreshCoordinator] Worker %d processing refresh for token %s (provider: %s)",
		workerID, request.TokenID, request.ProviderID)

	// Find appropriate refresher
	rc.mu.RLock()
	refresher, ok := rc.refreshers[request.ProviderID]
	rc.mu.RUnlock()

	if !ok {
		result.Success = false
		result.Error = fmt.Errorf("no refresher registered for provider: %s", request.ProviderID)
		rc.logger.ErrorLog("[RefreshCoordinator] %v", result.Error)
		rc.sendResult(result)
		return
	}

	// Perform refresh
	newToken, err := refresher.RefreshToken(rc.ctx, request.Token)
	result.Duration = time.Since(startTime)

	if err != nil {
		result.Success = false
		result.Error = err
		rc.logger.ErrorLog("[RefreshCoordinator] Failed to refresh token %s: %v", request.TokenID, err)

		// Mark token as unhealthy in store
		if updateErr := rc.store.MarkTokenUnhealthy(request.TokenID, err); updateErr != nil {
			rc.logger.ErrorLog("[RefreshCoordinator] Failed to mark token unhealthy: %v", updateErr)
		}
	} else {
		result.Success = true
		result.NewToken = &newToken
		rc.logger.InfoLog("[RefreshCoordinator] Successfully refreshed token %s (took %v)",
			request.TokenID, result.Duration)

		// Update token in store
		if updateErr := rc.store.UpdateToken(request.TokenID, func(token *ProviderToken) {
			*token = newToken
			token.LastUsed = GetCurrentTimestamp()
			token.Healthy = true
			token.ErrorCount = 0
			token.LastError = ""
		}); updateErr != nil {
			rc.logger.ErrorLog("[RefreshCoordinator] Failed to update token in store: %v", updateErr)
		}
	}

	rc.sendResult(result)
}

// sendResult sends a result to the result queue (non-blocking)
func (rc *RefreshCoordinator) sendResult(result RefreshResult) {
	select {
	case rc.resultQueue <- result:
		// Result sent successfully
	default:
		// Queue full, log warning but don't block
		rc.logger.WarnLog("[RefreshCoordinator] Result queue full, dropping result for token %s", result.TokenID)
	}
}

// ScheduleRefresh schedules a token refresh with optional priority
func (rc *RefreshCoordinator) ScheduleRefresh(tokenID string, priority int) error {
	// Get token from store
	token, err := rc.store.GetToken(tokenID)
	if err != nil {
		return fmt.Errorf("failed to get token %s: %w", tokenID, err)
	}

	request := RefreshRequest{
		TokenID:     tokenID,
		ProviderID:  rc.store.ProviderID,
		Priority:    priority,
		ScheduledAt: time.Now(),
		Token:       *token,
	}

	select {
	case rc.requestQueue <- request:
		rc.logger.DebugLog("[RefreshCoordinator] Scheduled refresh for token %s (priority: %d)", tokenID, priority)
		return nil

	case <-time.After(5 * time.Second):
		return fmt.Errorf("failed to schedule refresh: queue full or timeout")
	}
}

// RefreshToken immediately refreshes a token (synchronous)
func (rc *RefreshCoordinator) RefreshToken(ctx context.Context, tokenID string) (*ProviderToken, error) {
	// Get token from store
	token, err := rc.store.GetToken(tokenID)
	if err != nil {
		return nil, fmt.Errorf("failed to get token %s: %w", tokenID, err)
	}

	// Find appropriate refresher
	rc.mu.RLock()
	refresher, ok := rc.refreshers[rc.store.ProviderID]
	rc.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("no refresher registered for provider: %s", rc.store.ProviderID)
	}

	// Perform refresh
	newToken, err := refresher.RefreshToken(ctx, *token)
	if err != nil {
		return nil, fmt.Errorf("failed to refresh token: %w", err)
	}

	// Update token in store
	if err := rc.store.UpdateToken(tokenID, func(token *ProviderToken) {
		*token = newToken
		token.LastUsed = GetCurrentTimestamp()
		token.Healthy = true
		token.ErrorCount = 0
		token.LastError = ""
	}); err != nil {
		return nil, fmt.Errorf("failed to update token in store: %w", err)
	}

	return &newToken, nil
}

// RegisterRefresher registers a provider-specific refresher
func (rc *RefreshCoordinator) RegisterRefresher(refresher ProviderRefresh) {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	rc.refreshers[refresher.ProviderID()] = refresher
	rc.logger.InfoLog("[RefreshCoordinator] Registered refresher for provider: %s", refresher.ProviderID())
}

// GetQueueSize returns the current queue size
func (rc *RefreshCoordinator) GetQueueSize() int {
	return len(rc.requestQueue)
}

// GetWorkerStatus returns the status of all workers
func (rc *RefreshCoordinator) GetWorkerStatus() map[int]bool {
	rc.mu.RLock()
	defer rc.mu.RUnlock()

	status := make(map[int]bool)
	for k, v := range rc.workerStatus {
		status[k] = v
	}
	return status
}

// RefreshScheduler periodically checks for tokens that need refreshing
type RefreshScheduler struct {
	coordinator   *RefreshCoordinator
	store         *MultiTokenStore
	checkInterval time.Duration
	ctx           context.Context
	cancel        context.CancelFunc
	wg            sync.WaitGroup
	mu            sync.RWMutex
	logger        logging.Logger
}

// NewRefreshScheduler creates a new RefreshScheduler
func NewRefreshScheduler(coordinator *RefreshCoordinator, store *MultiTokenStore, checkInterval time.Duration, logger logging.Logger) *RefreshScheduler {
	if checkInterval <= 0 {
		checkInterval = 5 * time.Minute // Default check interval
	}
	if logger == nil {
		logger = logging.NewLogger()
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &RefreshScheduler{
		coordinator:   coordinator,
		store:         store,
		checkInterval: checkInterval,
		ctx:           ctx,
		cancel:        cancel,
		logger:        logger,
	}
}

// Start starts the refresh scheduler
func (rs *RefreshScheduler) Start() {
	rs.wg.Add(1)
	go rs.run()

	rs.logger.InfoLog("[RefreshScheduler] Started with check interval: %v", rs.checkInterval)
}

// Stop stops the refresh scheduler
func (rs *RefreshScheduler) Stop() {
	rs.cancel()
	rs.wg.Wait()
	rs.logger.InfoLog("[RefreshScheduler] Stopped")
}

// run is the main scheduler loop
func (rs *RefreshScheduler) run() {
	defer rs.wg.Done()

	ticker := time.NewTicker(rs.checkInterval)
	defer ticker.Stop()

	// Do an initial check
	rs.CheckAndSchedule()

	for {
		select {
		case <-rs.ctx.Done():
			rs.logger.DebugLog("[RefreshScheduler] Shutting down")
			return

		case <-ticker.C:
			rs.CheckAndSchedule()
		}
	}
}

// CheckAndSchedule checks all tokens and schedules refreshes for expiring ones
func (rs *RefreshScheduler) CheckAndSchedule() {
	tokens := rs.store.ListTokens()
	bufferSeconds := rs.store.Settings.RefreshBufferSec

	rs.logger.DebugLog("[RefreshScheduler] Checking %d tokens for refresh", len(tokens))

	for _, token := range tokens {
		if !token.Healthy {
			// Skip unhealthy tokens
			continue
		}

		if isTokenExpiringSoon(token, bufferSeconds) {
			priority := calculateRefreshPriority(token, bufferSeconds)
			if err := rs.coordinator.ScheduleRefresh(token.ID, priority); err != nil {
				rs.logger.ErrorLog("[RefreshScheduler] Failed to schedule refresh for token %s: %v", token.ID, err)
			}
		}
	}
}

// QwenRefresher implements ProviderRefresh for Qwen
type QwenRefresher struct {
	httpClient    *http.Client
	logger        logging.Logger
	clientFactory ProxyClientFactory
	tokenManager  *TokenManager // For proxy health tracking
}

// NewQwenRefresher creates a new QwenRefresher
func NewQwenRefresher(httpClient *http.Client, logger logging.Logger, clientFactory ProxyClientFactory, tokenManager *TokenManager) *QwenRefresher {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	if logger == nil {
		logger = logging.NewLogger()
	}
	return &QwenRefresher{
		httpClient:    httpClient,
		logger:        logger,
		clientFactory: clientFactory,
		tokenManager:  tokenManager,
	}
}

// RefreshToken refreshes a Qwen token using the existing RefreshAccessToken function
func (qr *QwenRefresher) RefreshToken(ctx context.Context, token ProviderToken) (ProviderToken, error) {
	if token.RefreshToken == "" {
		return ProviderToken{}, fmt.Errorf("no refresh token available")
	}

	// Log token refresh with proxy info
	proxyType := "direct"
	if token.Proxy != nil {
		proxyType = string(token.Proxy.Type)
	}
	qr.logger.InfoLog("[QwenRefresher] Refreshing token %s using %s connection", token.ID, proxyType)

	// Create OAuthCreds from ProviderToken
	creds := OAuthCreds{
		AccessToken:  token.AccessToken,
		TokenType:    token.TokenType,
		RefreshToken: token.RefreshToken,
		ResourceURL:  token.ResourceURL,
		ExpiryDate:   token.ExpiryDate,
	}

	// Use the existing RefreshAccessToken function
	refreshedCreds, err := RefreshAccessToken(creds)
	if err != nil {
		// Check if this is a proxy error and update health
		if isProxyError(err) && qr.tokenManager != nil {
			qr.logger.ErrorLog("[QwenRefresher] Proxy error during token refresh for %s: %v", token.ID, err)
			if updateErr := qr.tokenManager.UpdateProxyHealth(token.ID, false, err); updateErr != nil {
				qr.logger.ErrorLog("[QwenRefresher] Failed to update proxy health: %v", updateErr)
			}
		}
		return ProviderToken{}, fmt.Errorf("failed to refresh Qwen token: %w", err)
	}

	// Convert back to ProviderToken
	newToken := token
	newToken.AccessToken = refreshedCreds.AccessToken
	newToken.TokenType = refreshedCreds.TokenType
	newToken.RefreshToken = refreshedCreds.RefreshToken
	newToken.ExpiryDate = refreshedCreds.ExpiryDate
	newToken.ResourceURL = refreshedCreds.ResourceURL

	// Update proxy health on success
	if qr.tokenManager != nil {
		if updateErr := qr.tokenManager.UpdateProxyHealth(token.ID, true, nil); updateErr != nil {
			qr.logger.ErrorLog("[QwenRefresher] Failed to update proxy health: %v", updateErr)
		}
	}

	qr.logger.InfoLog("[QwenRefresher] Successfully refreshed token %s", token.ID)
	return newToken, nil
}

// ProviderID returns the provider identifier
func (qr *QwenRefresher) ProviderID() string {
	return "qwen"
}

// GeminiRefresher implements ProviderRefresh for Gemini
type GeminiRefresher struct {
	config        *GeminiOAuthConfig
	httpClient    *http.Client
	logger        logging.Logger
	clientFactory ProxyClientFactory
	tokenManager  *TokenManager // For proxy health tracking
}

// NewGeminiRefresher creates a new GeminiRefresher
func NewGeminiRefresher(config *GeminiOAuthConfig, httpClient *http.Client, logger logging.Logger, clientFactory ProxyClientFactory, tokenManager *TokenManager) *GeminiRefresher {
	if config == nil {
		config = DefaultGeminiOAuthConfig()
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	if logger == nil {
		logger = logging.NewLogger()
	}
	return &GeminiRefresher{
		config:        config,
		httpClient:    httpClient,
		logger:        logger,
		clientFactory: clientFactory,
		tokenManager:  tokenManager,
	}
}

// RefreshToken refreshes a Gemini token using the existing GeminiAuthenticator refresh logic
func (gr *GeminiRefresher) RefreshToken(ctx context.Context, token ProviderToken) (ProviderToken, error) {
	if token.RefreshToken == "" {
		return ProviderToken{}, fmt.Errorf("no refresh token available")
	}

	// Log token refresh with proxy info
	proxyType := "direct"
	if token.Proxy != nil {
		proxyType = string(token.Proxy.Type)
	}
	gr.logger.InfoLog("[GeminiRefresher] Refreshing token %s using %s connection", token.ID, proxyType)

	// Get proxy-aware client if factory is available
	httpClient := gr.httpClient
	if gr.clientFactory != nil {
		httpClient = gr.clientFactory.GetClient(token.Proxy)
		gr.logger.DebugLog("[GeminiRefresher] Using proxy-aware client for token %s", token.ID)
	}

	// Setup OAuth2 config
	conf := &oauth2.Config{
		ClientID:     gr.config.ClientID,
		ClientSecret: gr.config.ClientSecret,
		Scopes:       []string{gr.config.Scope},
		Endpoint: oauth2.Endpoint{
			AuthURL:  "https://accounts.google.com/o/oauth2/v2/auth",
			TokenURL: "https://oauth2.googleapis.com/token",
		},
	}

	// Create oauth2.Token from ProviderToken
	oauthToken := &oauth2.Token{
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		TokenType:    token.TokenType,
		Expiry:       time.UnixMilli(token.ExpiryDate),
	}

	// Create a context with the custom HTTP client
	oauthCtx := context.WithValue(ctx, oauth2.HTTPClient, httpClient)

	// Create TokenSource and get new token
	ts := conf.TokenSource(oauthCtx, oauthToken)
	newToken, err := ts.Token()
	if err != nil {
		// Check if this is a proxy error and update health
		if isProxyError(err) && gr.tokenManager != nil {
			gr.logger.ErrorLog("[GeminiRefresher] Proxy error during token refresh for %s: %v", token.ID, err)
			if updateErr := gr.tokenManager.UpdateProxyHealth(token.ID, false, err); updateErr != nil {
				gr.logger.ErrorLog("[GeminiRefresher] Failed to update proxy health: %v", updateErr)
			}
		}
		return ProviderToken{}, fmt.Errorf("failed to refresh Gemini token: %w", err)
	}

	// Convert back to ProviderToken
	newProviderToken := token
	newProviderToken.AccessToken = newToken.AccessToken
	newProviderToken.TokenType = newToken.TokenType
	if newToken.RefreshToken != "" {
		newProviderToken.RefreshToken = newToken.RefreshToken
	}
	newProviderToken.ExpiryDate = newToken.Expiry.UnixMilli()

	// Update scope if provided
	if scope, ok := newToken.Extra("scope").(string); ok && scope != "" {
		newProviderToken.Scope = scope
	}

	// Update proxy health on success
	if gr.tokenManager != nil {
		if updateErr := gr.tokenManager.UpdateProxyHealth(token.ID, true, nil); updateErr != nil {
			gr.logger.ErrorLog("[GeminiRefresher] Failed to update proxy health: %v", updateErr)
		}
	}

	gr.logger.InfoLog("[GeminiRefresher] Successfully refreshed token %s", token.ID)
	return newProviderToken, nil
}

// ProviderID returns the provider identifier
func (gr *GeminiRefresher) ProviderID() string {
	return "gemini"
}

// KiroRefresher implements ProviderRefresh for Kiro
type KiroRefresher struct {
	config        *KiroOAuthConfig
	httpClient    *http.Client
	logger        logging.Logger
	clientFactory ProxyClientFactory
	tokenManager  *TokenManager // For proxy health tracking
}

// NewKiroRefresher creates a new KiroRefresher
func NewKiroRefresher(config *KiroOAuthConfig, httpClient *http.Client, logger logging.Logger, clientFactory ProxyClientFactory, tokenManager *TokenManager) *KiroRefresher {
	if config == nil {
		config = DefaultKiroOAuthConfig()
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	if logger == nil {
		logger = logging.NewLogger()
	}
	return &KiroRefresher{
		config:        config,
		httpClient:    httpClient,
		logger:        logger,
		clientFactory: clientFactory,
		tokenManager:  tokenManager,
	}
}

// RefreshToken refreshes a Kiro token using the existing KiroAuthenticator refresh logic
func (kr *KiroRefresher) RefreshToken(ctx context.Context, token ProviderToken) (ProviderToken, error) {
	if token.RefreshToken == "" {
		return ProviderToken{}, fmt.Errorf("no refresh token available")
	}

	// Log token refresh with proxy info
	proxyType := "direct"
	if token.Proxy != nil {
		proxyType = string(token.Proxy.Type)
	}
	kr.logger.InfoLog("[KiroRefresher] Refreshing token %s using %s connection", token.ID, proxyType)

	// Get proxy-aware client if factory is available
	httpClient := kr.httpClient
	if kr.clientFactory != nil {
		httpClient = kr.clientFactory.GetClient(token.Proxy)
		kr.logger.DebugLog("[KiroRefresher] Using proxy-aware client for token %s", token.ID)
	}

	// Determine refresh URL based on auth method
	var refreshURL string
	region := kr.config.Region
	authMethod := "social" // Default to social

	// Try to extract auth method from token (stored as additional metadata)
	// For now, we'll use the default social auth method
	if authMethod == "social" {
		refreshURL = strings.ReplaceAll(kr.config.RefreshURL, "{{region}}", region)
	} else {
		refreshURL = strings.ReplaceAll(kr.config.RefreshIDCURL, "{{region}}", region)
	}

	// Build request body
	requestBody := map[string]string{
		"refreshToken": token.RefreshToken,
	}

	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		return ProviderToken{}, fmt.Errorf("failed to marshal refresh request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", refreshURL, strings.NewReader(string(bodyBytes)))
	if err != nil {
		return ProviderToken{}, fmt.Errorf("failed to create refresh request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		// Check if this is a proxy error and update health
		if isProxyError(err) && kr.tokenManager != nil {
			kr.logger.ErrorLog("[KiroRefresher] Proxy error during token refresh for %s: %v", token.ID, err)
			if updateErr := kr.tokenManager.UpdateProxyHealth(token.ID, false, err); updateErr != nil {
				kr.logger.ErrorLog("[KiroRefresher] Failed to update proxy health: %v", updateErr)
			}
		}
		return ProviderToken{}, fmt.Errorf("failed to send refresh request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return ProviderToken{}, fmt.Errorf("token refresh failed with status: %d", resp.StatusCode)
	}

	var tokenResp struct {
		AccessToken  string `json:"accessToken"`
		ExpiresIn    int64  `json:"expiresIn"`
		RefreshToken string `json:"refreshToken,omitempty"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return ProviderToken{}, fmt.Errorf("failed to decode refresh response: %w", err)
	}

	// Convert back to ProviderToken
	newToken := token
	newToken.AccessToken = tokenResp.AccessToken
	newToken.ExpiryDate = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second).UnixMilli()

	// Preserve refresh token if not provided in response
	if tokenResp.RefreshToken != "" {
		newToken.RefreshToken = tokenResp.RefreshToken
	}

	// Update proxy health on success
	if kr.tokenManager != nil {
		if updateErr := kr.tokenManager.UpdateProxyHealth(token.ID, true, nil); updateErr != nil {
			kr.logger.ErrorLog("[KiroRefresher] Failed to update proxy health: %v", updateErr)
		}
	}

	kr.logger.InfoLog("[KiroRefresher] Successfully refreshed token %s", token.ID)
	return newToken, nil
}

// ProviderID returns the provider identifier
func (kr *KiroRefresher) ProviderID() string {
	return "kiro"
}

// IFlowRefresher implements ProviderRefresh for iFlow
type IFlowRefresher struct {
	config        *IFlowOAuthConfig
	httpClient    *http.Client
	logger        logging.Logger
	clientFactory ProxyClientFactory
	tokenManager  *TokenManager // For proxy health tracking
}

// NewIFlowRefresher creates a new IFlowRefresher
func NewIFlowRefresher(config *IFlowOAuthConfig, httpClient *http.Client, logger logging.Logger, clientFactory ProxyClientFactory, tokenManager *TokenManager) *IFlowRefresher {
	if config == nil {
		config = DefaultIFlowOAuthConfig()
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	if logger == nil {
		logger = logging.NewLogger()
	}
	return &IFlowRefresher{
		config:        config,
		httpClient:    httpClient,
		logger:        logger,
		clientFactory: clientFactory,
		tokenManager:  tokenManager,
	}
}

// RefreshToken refreshes an iFlow token using the existing IFlowAuthenticator refresh logic
func (ifr *IFlowRefresher) RefreshToken(ctx context.Context, token ProviderToken) (ProviderToken, error) {
	if token.RefreshToken == "" {
		return ProviderToken{}, fmt.Errorf("no refresh token available")
	}

	// Log token refresh with proxy info
	proxyType := "direct"
	if token.Proxy != nil {
		proxyType = string(token.Proxy.Type)
	}
	ifr.logger.InfoLog("[IFlowRefresher] Refreshing token %s using %s connection", token.ID, proxyType)

	// Get proxy-aware client if factory is available
	httpClient := ifr.httpClient
	if ifr.clientFactory != nil {
		httpClient = ifr.clientFactory.GetClient(token.Proxy)
		ifr.logger.DebugLog("[IFlowRefresher] Using proxy-aware client for token %s", token.ID)
	}

	// Setup OAuth2 config
	conf := &oauth2.Config{
		ClientID:     ifr.config.ClientID,
		ClientSecret: ifr.config.ClientSecret,
		Endpoint: oauth2.Endpoint{
			AuthURL:  IFlowAuthURL,
			TokenURL: IFlowTokenURL,
		},
	}

	// Create oauth2.Token from ProviderToken
	oauthToken := &oauth2.Token{
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		TokenType:    token.TokenType,
		Expiry:       time.UnixMilli(token.ExpiryDate),
	}

	// Create a context with the custom HTTP client
	oauthCtx := context.WithValue(ctx, oauth2.HTTPClient, httpClient)

	// Create TokenSource and get new token
	ts := conf.TokenSource(oauthCtx, oauthToken)
	newToken, err := ts.Token()
	if err != nil {
		// Check if this is a proxy error and update health
		if isProxyError(err) && ifr.tokenManager != nil {
			ifr.logger.ErrorLog("[IFlowRefresher] Proxy error during token refresh for %s: %v", token.ID, err)
			if updateErr := ifr.tokenManager.UpdateProxyHealth(token.ID, false, err); updateErr != nil {
				ifr.logger.ErrorLog("[IFlowRefresher] Failed to update proxy health: %v", updateErr)
			}
		}
		return ProviderToken{}, fmt.Errorf("failed to refresh iFlow token: %w", err)
	}

	// Convert back to ProviderToken
	newProviderToken := token
	newProviderToken.AccessToken = newToken.AccessToken
	newProviderToken.TokenType = newToken.TokenType
	if newToken.RefreshToken != "" {
		newProviderToken.RefreshToken = newToken.RefreshToken
	}
	newProviderToken.ExpiryDate = newToken.Expiry.UnixMilli()

	// Update scope if provided
	if scope, ok := newToken.Extra("scope").(string); ok && scope != "" {
		newProviderToken.Scope = scope
	}

	// Update proxy health on success
	if ifr.tokenManager != nil {
		if updateErr := ifr.tokenManager.UpdateProxyHealth(token.ID, true, nil); updateErr != nil {
			ifr.logger.ErrorLog("[IFlowRefresher] Failed to update proxy health: %v", updateErr)
		}
	}

	ifr.logger.InfoLog("[IFlowRefresher] Successfully refreshed token %s", token.ID)
	return newProviderToken, nil
}

// ProviderID returns the provider identifier
func (ifr *IFlowRefresher) ProviderID() string {
	return "iflow"
}

// isProxyError detects if an error is a proxy-related error
func isProxyError(err error) bool {
	if err == nil {
		return false
	}

	// Check for common proxy-related error patterns
	errStr := err.Error()

	// Connection refused / timeout
	if strings.Contains(errStr, "connection refused") ||
		strings.Contains(errStr, "connect: connection refused") {
		return true
	}

	// Timeout errors
	if strings.Contains(errStr, "timeout") ||
		strings.Contains(errStr, "i/o timeout") ||
		strings.Contains(errStr, "deadline exceeded") {
		return true
	}

	// DNS errors
	if strings.Contains(errStr, "no such host") ||
		strings.Contains(errStr, "lookup") ||
		strings.Contains(errStr, "dns") {
		return true
	}

	// Proxy-specific errors
	if strings.Contains(errStr, "proxy") ||
		strings.Contains(errStr, "socks") ||
		strings.Contains(errStr, "tunnel") {
		return true
	}

	// Check for net.OpError with specific types
	var netErr net.Error
	if errors.As(err, &netErr) {
		if netErr.Timeout() {
			return true
		}
	}

	return false
}

// classifyProxyError returns the type of proxy error for health tracking
func classifyProxyError(err error) string {
	if err == nil {
		return "none"
	}

	errStr := err.Error()

	// Connection errors
	if strings.Contains(errStr, "connection refused") ||
		strings.Contains(errStr, "connect: connection refused") {
		return "connection_refused"
	}

	// Timeout errors
	if strings.Contains(errStr, "timeout") ||
		strings.Contains(errStr, "i/o timeout") ||
		strings.Contains(errStr, "deadline exceeded") {
		return "timeout"
	}

	// DNS errors
	if strings.Contains(errStr, "no such host") ||
		strings.Contains(errStr, "lookup") ||
		strings.Contains(errStr, "dns") {
		return "dns"
	}

	// Authentication errors
	if strings.Contains(errStr, "authentication") ||
		strings.Contains(errStr, "unauthorized") ||
		strings.Contains(errStr, "401") ||
		strings.Contains(errStr, "407") {
		return "authentication"
	}

	// Generic proxy error
	if strings.Contains(errStr, "proxy") ||
		strings.Contains(errStr, "socks") ||
		strings.Contains(errStr, "tunnel") {
		return "proxy_error"
	}

	return "unknown"
}

// calculateRefreshPriority calculates refresh priority based on expiry time
// Lower value = higher priority
func calculateRefreshPriority(token ProviderToken, bufferSeconds int) int {
	remaining := getExpiryTimeRemaining(token)
	bufferDuration := time.Duration(bufferSeconds) * time.Second

	// Critical: already expired or within 5 minutes of buffer
	if remaining < bufferDuration-(5*time.Minute) {
		return 0
	}

	// High: within 15 minutes of buffer
	if remaining < bufferDuration-(15*time.Minute) {
		return 10
	}

	// Medium: within 30 minutes of buffer
	if remaining < bufferDuration-(30*time.Minute) {
		return 20
	}

	// Low: more than 30 minutes before buffer
	return 30
}

// isTokenExpiringSoon checks if a token is expiring soon
func isTokenExpiringSoon(token ProviderToken, bufferSeconds int) bool {
	if token.ExpiryDate == 0 {
		return false // No expiry date set
	}

	remaining := getExpiryTimeRemaining(token)
	bufferDuration := time.Duration(bufferSeconds) * time.Second

	return remaining < bufferDuration
}

// getExpiryTimeRemaining returns the time remaining until token expiry
func getExpiryTimeRemaining(token ProviderToken) time.Duration {
	if token.ExpiryDate == 0 {
		return 0
	}

	expiry := time.UnixMilli(token.ExpiryDate)
	remaining := time.Until(expiry)

	if remaining < 0 {
		return 0
	}

	return remaining
}
