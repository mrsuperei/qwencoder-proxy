package token

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/sunbankio/qwencoder-proxy/logging"
)

// RefreshRequest represents a token refresh request.
type RefreshRequest struct {
	TokenID     string
	ProviderID  string
	Priority    int // Lower value = higher priority.
	ScheduledAt time.Time
	Token       ProviderToken
}

// RefreshResult represents the result of a token refresh.
type RefreshResult struct {
	TokenID  string
	Success  bool
	NewToken *ProviderToken
	Error    error
	Duration time.Duration
}

// ProviderRefresh defines the interface for provider-specific refresh logic.
type ProviderRefresh interface {
	// RefreshToken refreshes a token and returns the updated token.
	RefreshToken(ctx context.Context, token ProviderToken) (ProviderToken, error)
	// ProviderID returns the provider identifier.
	ProviderID() string
}

// RefreshCoordinator manages token refresh operations with a worker pool.
type RefreshCoordinator struct {
	store        *SQLiteStore
	refreshers   map[string]ProviderRefresh
	requestQueue chan RefreshRequest
	resultQueue  chan RefreshResult
	workers      int
	ctx          context.Context
	cancel       context.CancelFunc
	wg           sync.WaitGroup
	mu           sync.RWMutex
	logger       logging.Logger
}

// NewRefreshCoordinator creates a new RefreshCoordinator.
func NewRefreshCoordinator(store *SQLiteStore, workers int, logger logging.Logger, _ ProxyClientFactory) *RefreshCoordinator {
	if workers <= 0 {
		workers = 3
	}
	if logger == nil {
		logger = logging.NewLogger()
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &RefreshCoordinator{
		store:        store,
		refreshers:   make(map[string]ProviderRefresh),
		requestQueue: make(chan RefreshRequest, 100),
		resultQueue:  make(chan RefreshResult, 100),
		workers:      workers,
		ctx:          ctx,
		cancel:       cancel,
		logger:       logger,
	}
}

// Start starts coordinator workers.
func (rc *RefreshCoordinator) Start() error {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	for i := 0; i < rc.workers; i++ {
		rc.wg.Add(1)
		go rc.worker()
	}

	rc.logger.InfoLog("[RefreshCoordinator] Started %d workers", rc.workers)
	return nil
}

// Stop gracefully stops all workers.
func (rc *RefreshCoordinator) Stop() {
	rc.cancel()
	rc.wg.Wait()
	rc.logger.InfoLog("[RefreshCoordinator] Stopped all workers")
}

func (rc *RefreshCoordinator) worker() {
	defer rc.wg.Done()

	for {
		select {
		case <-rc.ctx.Done():
			return
		case request := <-rc.requestQueue:
			rc.processRequest(request)
		}
	}
}

func (rc *RefreshCoordinator) processRequest(request RefreshRequest) {
	startTime := time.Now()
	result := RefreshResult{TokenID: request.TokenID}

	rc.mu.RLock()
	refresher, ok := rc.refreshers[request.ProviderID]
	rc.mu.RUnlock()

	if !ok {
		result.Success = false
		result.Error = fmt.Errorf("no refresher registered for provider: %s", request.ProviderID)
		result.Duration = time.Since(startTime)
		rc.sendResult(result)
		return
	}

	newToken, err := refresher.RefreshToken(rc.ctx, request.Token)
	result.Duration = time.Since(startTime)
	if err != nil {
		result.Success = false
		result.Error = err
		_ = rc.store.MarkTokenUnhealthy(request.TokenID, err)
		rc.sendResult(result)
		return
	}

	_ = rc.store.UpdateToken(request.TokenID, func(token *ProviderToken) {
		*token = newToken
		token.LastUsed = time.Now().UnixMilli()
		token.Healthy = true
		token.ErrorCount = 0
		token.LastError = ""
	})

	result.Success = true
	result.NewToken = &newToken
	rc.sendResult(result)
}

func (rc *RefreshCoordinator) sendResult(result RefreshResult) {
	select {
	case rc.resultQueue <- result:
	default:
		rc.logger.WarnLog("[RefreshCoordinator] Result queue full, dropping result for token %s", result.TokenID)
	}
}

// ScheduleRefresh schedules a token refresh.
func (rc *RefreshCoordinator) ScheduleRefresh(tokenID string, priority int) error {
	token, err := rc.store.GetToken(tokenID)
	if err != nil {
		return fmt.Errorf("failed to get token %s: %w", tokenID, err)
	}

	request := RefreshRequest{
		TokenID:     tokenID,
		ProviderID:  rc.store.providerID,
		Priority:    priority,
		ScheduledAt: time.Now(),
		Token:       *token,
	}

	select {
	case rc.requestQueue <- request:
		return nil
	case <-time.After(5 * time.Second):
		return fmt.Errorf("failed to schedule refresh: queue full or timeout")
	}
}

// RegisterRefresher registers a provider-specific refresher.
func (rc *RefreshCoordinator) RegisterRefresher(refresher ProviderRefresh) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	rc.refreshers[refresher.ProviderID()] = refresher
}

// RefreshScheduler periodically checks for tokens that need refreshing.
type RefreshScheduler struct {
	coordinator   *RefreshCoordinator
	store         *SQLiteStore
	checkInterval time.Duration
	ctx           context.Context
	cancel        context.CancelFunc
	wg            sync.WaitGroup
	logger        logging.Logger
}

// NewRefreshScheduler creates a new RefreshScheduler.
func NewRefreshScheduler(coordinator *RefreshCoordinator, store *SQLiteStore, checkInterval time.Duration, logger logging.Logger) *RefreshScheduler {
	if checkInterval <= 0 {
		checkInterval = 5 * time.Minute
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

// Start starts the refresh scheduler.
func (rs *RefreshScheduler) Start() {
	rs.wg.Add(1)
	go rs.run()
}

// Stop stops the refresh scheduler.
func (rs *RefreshScheduler) Stop() {
	rs.cancel()
	rs.wg.Wait()
}

func (rs *RefreshScheduler) run() {
	defer rs.wg.Done()

	ticker := time.NewTicker(rs.checkInterval)
	defer ticker.Stop()

	rs.CheckAndSchedule()

	for {
		select {
		case <-rs.ctx.Done():
			return
		case <-ticker.C:
			rs.CheckAndSchedule()
		}
	}
}

// CheckAndSchedule checks tokens and schedules refreshes for expiring ones.
func (rs *RefreshScheduler) CheckAndSchedule() {
	tokens := rs.store.ListTokens()
	settings, err := rs.store.GetSettings()
	if err != nil {
		rs.logger.ErrorLog("[RefreshScheduler] Failed to get settings: %v", err)
		return
	}
	bufferSeconds := settings.RefreshBufferSec

	for _, token := range tokens {
		if !token.Healthy {
			continue
		}
		if isTokenExpiringSoon(token, bufferSeconds) {
			priority := calculateRefreshPriority(token, bufferSeconds)
			_ = rs.coordinator.ScheduleRefresh(token.ID, priority)
		}
	}
}

func calculateRefreshPriority(token ProviderToken, bufferSeconds int) int {
	remaining := getExpiryTimeRemaining(token)
	bufferDuration := time.Duration(bufferSeconds) * time.Second

	if remaining < bufferDuration-(5*time.Minute) {
		return 0
	}
	if remaining < bufferDuration-(15*time.Minute) {
		return 10
	}
	if remaining < bufferDuration-(30*time.Minute) {
		return 20
	}
	return 30
}

func isTokenExpiringSoon(token ProviderToken, bufferSeconds int) bool {
	if token.ExpiryDate == 0 {
		return false
	}
	remaining := getExpiryTimeRemaining(token)
	bufferDuration := time.Duration(bufferSeconds) * time.Second
	return remaining < bufferDuration
}

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
