// Package restapi provides REST API endpoints for OAuth2 authentication flows
package restapi

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"sync"
	"time"

	"github.com/sunbankio/qwencoder-proxy/auth"
)

// OAuthState represents the state for an authorization code flow
type OAuthState struct {
	StateID      string    // Unique state identifier
	Provider     string    // Provider ID
	CodeVerifier string    // PKCE code verifier
	RedirectURI  string    // Redirect URI for callback
	ExpiresAt    time.Time // State expiration time
	CreatedAt    time.Time // Creation time
}

// DevicePollState represents the state for a device code flow
type DevicePollState struct {
	PollID       string           // Unique poll identifier
	Provider     string           // Provider ID
	DeviceCode   string           // Device code from OAuth provider
	ExpiresAt    time.Time        // Poll expiration time
	Interval     time.Duration    // Polling interval
	Status       string           // Current status: "pending", "authorized", "error"
	Tokens       *auth.OAuthCreds // OAuth tokens when authorized
	ErrorCode    string           // Error code if status is "error"
	ErrorMessage string           // Error message if status is "error"
	LastPolledAt time.Time        // Last time this poll was queried
	CreatedAt    time.Time        // Creation time
}

// StateManager manages in-memory state for OAuth flows
type StateManager struct {
	states         map[string]*OAuthState
	polls          map[string]*DevicePollState
	processedCodes map[string]time.Time // Tracks processed OAuth codes for idempotency (code -> processedAt)
	mu             sync.RWMutex
}

// NewStateManager creates a new state manager
func NewStateManager() *StateManager {
	sm := &StateManager{
		states:         make(map[string]*OAuthState),
		polls:          make(map[string]*DevicePollState),
		processedCodes: make(map[string]time.Time),
	}
	// Start cleanup routine
	go sm.StartCleanupRoutine()
	return sm
}

// CreateState creates a new OAuth state for authorization code flow
func (sm *StateManager) CreateState(provider, codeVerifier, redirectURI string, ttl time.Duration) (string, error) {
	stateID, err := generateRandomString(16)
	if err != nil {
		return "", fmt.Errorf("failed to generate state ID: %w", err)
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()

	now := time.Now()
	sm.states[stateID] = &OAuthState{
		StateID:      stateID,
		Provider:     provider,
		CodeVerifier: codeVerifier,
		RedirectURI:  redirectURI,
		ExpiresAt:    now.Add(ttl),
		CreatedAt:    now,
	}

	fmt.Printf("[StateManager] Created state: %s for provider: %s, redirectURI: %s, expires at: %v\n",
		stateID, provider, redirectURI, now.Add(ttl))

	return stateID, nil
}

// ValidateAndConsume validates a state and removes it from the store (one-time use)
func (sm *StateManager) ValidateAndConsume(stateID string) (*OAuthState, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	fmt.Printf("[StateManager] Validating state: %s, total states: %d\n",
		stateID, len(sm.states))

	state, ok := sm.states[stateID]
	if !ok {
		fmt.Printf("[StateManager] State not found: %s. Available states: %v\n",
			stateID, sm.getStateKeys())
		return nil, fmt.Errorf("invalid state: state not found")
	}

	if time.Now().After(state.ExpiresAt) {
		delete(sm.states, stateID)
		fmt.Printf("[StateManager] State expired: %s (expired at: %v)\n",
			stateID, state.ExpiresAt)
		return nil, fmt.Errorf("invalid state: state expired")
	}

	// Remove state after validation (one-time use)
	delete(sm.states, stateID)

	fmt.Printf("[StateManager] State validated and consumed: %s for provider: %s\n",
		stateID, state.Provider)

	return state, nil
}

// CreatePoll creates a new device code poll state
func (sm *StateManager) CreatePoll(provider, deviceCode string, interval time.Duration, ttl time.Duration) (string, error) {
	pollID, err := generateUUID()
	if err != nil {
		return "", fmt.Errorf("failed to generate poll ID: %w", err)
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()

	now := time.Now()
	sm.polls[pollID] = &DevicePollState{
		PollID:       pollID,
		Provider:     provider,
		DeviceCode:   deviceCode,
		ExpiresAt:    now.Add(ttl),
		Interval:     interval,
		Status:       "pending",
		LastPolledAt: now,
		CreatedAt:    now,
	}

	return pollID, nil
}

// UpdatePoll updates a device poll state with new status and optional tokens
func (sm *StateManager) UpdatePoll(pollID, status string, tokens *auth.OAuthCreds) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	poll, ok := sm.polls[pollID]
	if !ok {
		return fmt.Errorf("poll not found: %s", pollID)
	}

	poll.Status = status
	poll.Tokens = tokens
	poll.LastPolledAt = time.Now()

	return nil
}

// UpdatePollError updates a device poll state with error information
func (sm *StateManager) UpdatePollError(pollID, errorCode, errorMessage string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	poll, ok := sm.polls[pollID]
	if !ok {
		return fmt.Errorf("poll not found: %s", pollID)
	}

	poll.Status = "error"
	poll.ErrorCode = errorCode
	poll.ErrorMessage = errorMessage
	poll.LastPolledAt = time.Now()

	return nil
}

// GetPoll retrieves a device poll state
func (sm *StateManager) GetPoll(pollID string) (*DevicePollState, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	poll, ok := sm.polls[pollID]
	if !ok {
		return nil, fmt.Errorf("poll not found: %s", pollID)
	}

	// Check if expired
	if time.Now().After(poll.ExpiresAt) {
		return nil, fmt.Errorf("poll expired: %s", pollID)
	}

	// Update last polled time
	poll.LastPolledAt = time.Now()

	return poll, nil
}

// IsCodeProcessed checks if an OAuth authorization code has already been processed
func (sm *StateManager) IsCodeProcessed(code string) bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	_, exists := sm.processedCodes[code]
	return exists
}

// MarkCodeProcessed marks an OAuth authorization code as processed
func (sm *StateManager) MarkCodeProcessed(code string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.processedCodes[code] = time.Now()
}

// CleanupExpired removes expired states, polls, and processed codes
func (sm *StateManager) CleanupExpired() {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	now := time.Now()

	// Clean up expired states
	for id, state := range sm.states {
		if now.After(state.ExpiresAt) {
			delete(sm.states, id)
		}
	}

	// Clean up expired polls
	for id, poll := range sm.polls {
		if now.After(poll.ExpiresAt) {
			delete(sm.polls, id)
		}
	}

	// Clean up processed codes older than 1 hour (OAuth codes are typically short-lived)
	codeExpiry := 1 * time.Hour
	for code, processedAt := range sm.processedCodes {
		if now.Sub(processedAt) > codeExpiry {
			delete(sm.processedCodes, code)
		}
	}
}

// StartCleanupRoutine starts a background goroutine to clean up expired states
func (sm *StateManager) StartCleanupRoutine() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		sm.CleanupExpired()
	}
}

// generateRandomString generates a random string of the specified length
func generateRandomString(length int) (string, error) {
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// generateUUID generates a UUID-like string for poll IDs
func generateUUID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

// getStateKeys returns a list of all state keys for logging purposes
func (sm *StateManager) getStateKeys() []string {
	keys := make([]string, 0, len(sm.states))
	for k := range sm.states {
		keys = append(keys, k)
	}
	return keys
}
