package token

import (
	"errors"
	"sync"
	"time"

	"github.com/sunbankio/qwencoder-proxy/internal/logging"
)

// MockStore is an in-memory implementation of token storage for testing.
// It provides all methods needed by TokenManager and TokenStore interface.
type MockStore struct {
	mu         sync.RWMutex
	tokens     map[string]ProviderToken
	providerID string
	logger     logging.Logger
	cleared    bool
}

// NewMockStore creates a new in-memory mock token store for testing.
func NewMockStore(providerID string, logger logging.Logger) *MockStore {
	if logger == nil {
		logger = logging.NewLogger()
	}
	return &MockStore{
		tokens:     make(map[string]ProviderToken),
		providerID: providerID,
		logger:     logger,
	}
}

// Load returns all tokens from the mock store.
func (m *MockStore) Load() (map[string]ProviderToken, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Return a copy of the tokens map
	result := make(map[string]ProviderToken, len(m.tokens))
	for k, v := range m.tokens {
		result[k] = v
	}
	return result, nil
}

// Save persists all tokens to the mock store.
func (m *MockStore) Save(tokens map[string]ProviderToken) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Make a copy of the tokens map
	m.tokens = make(map[string]ProviderToken, len(tokens))
	for k, v := range tokens {
		m.tokens[k] = v
	}
	m.cleared = false
	return nil
}

// GetCredentialsPath returns a mock credentials path.
func (m *MockStore) GetCredentialsPath() string {
	return ".credentials/mock.db"
}

// Clear removes all tokens from the mock store.
func (m *MockStore) Clear() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.tokens = make(map[string]ProviderToken)
	m.cleared = true
	return nil
}

// AddToken adds a single token to the mock store.
func (m *MockStore) AddToken(token ProviderToken) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.tokens[token.ID] = token
	return nil
}

// GetToken retrieves a single token by ID.
func (m *MockStore) GetToken(id string) (*ProviderToken, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	token, ok := m.tokens[id]
	if !ok {
		return nil, ErrTokenNotFound
	}
	return &token, nil
}

// ListTokens returns all tokens from the mock store.
func (m *MockStore) ListTokens() []ProviderToken {
	m.mu.RLock()
	defer m.mu.RUnlock()

	tokens := make([]ProviderToken, 0, len(m.tokens))
	for _, token := range m.tokens {
		tokens = append(tokens, token)
	}
	return tokens
}

// UpdateToken updates a token using the provided update function.
func (m *MockStore) UpdateToken(id string, update func(*ProviderToken)) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	token, ok := m.tokens[id]
	if !ok {
		return ErrTokenNotFound
	}

	// Convert to pointer for update
	tokenPtr := &token
	update(tokenPtr)
	m.tokens[id] = *tokenPtr
	return nil
}

// RemoveToken removes a token by ID.
func (m *MockStore) RemoveToken(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.tokens[id]; !ok {
		return ErrTokenNotFound
	}
	delete(m.tokens, id)
	return nil
}

// GetSettings retrieves provider-specific settings from storage.
func (m *MockStore) GetSettings() (StoreSettings, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Return default settings for mock store
	return StoreSettings{
		SelectionStrategy: "random",
		RefreshBufferSec:  300,
		MaxErrorCount:     5,
		UpdatedAt:         time.Now().UnixMilli(),
	}, nil
}

// SaveSettings persists provider-specific settings to storage.
func (m *MockStore) SaveSettings(settings StoreSettings) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// For mock store, we just log the settings save
	m.logger.InfoLog("[MockStore] Settings saved: %+v", settings)
	return nil
}

// MarkTokenHealthy marks a token as healthy.
func (m *MockStore) MarkTokenHealthy(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	token, ok := m.tokens[id]
	if !ok {
		return ErrTokenNotFound
	}

	token.Healthy = true
	token.HealthScore = 1.0
	token.ErrorCount = 0
	token.LastError = ""
	m.tokens[id] = token
	return nil
}

// MarkTokenUnhealthy marks a token as unhealthy.
func (m *MockStore) MarkTokenUnhealthy(id string, err error) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	token, ok := m.tokens[id]
	if !ok {
		return ErrTokenNotFound
	}

	token.Healthy = false
	token.HealthScore = 0.0
	token.ErrorCount++
	if err != nil {
		token.LastError = err.Error()
	}
	m.tokens[id] = token
	return nil
}

// IsTokenValid checks if a token is valid (not expired and healthy).
func (m *MockStore) IsTokenValid(token ProviderToken) bool {
	now := time.Now().UnixMilli()
	return token.Healthy && token.ExpiryDate > now
}

// GetValidTokens returns all valid tokens from the mock store.
func (m *MockStore) GetValidTokens() []ProviderToken {
	m.mu.RLock()
	defer m.mu.RUnlock()

	valid := make([]ProviderToken, 0)
	for _, token := range m.tokens {
		if m.IsTokenValid(token) {
			valid = append(valid, token)
		}
	}
	return valid
}

// TokenCount returns the number of tokens in the store.
func (m *MockStore) TokenCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return len(m.tokens)
}

// GetTokenCount returns the number of tokens in the store.
func (m *MockStore) GetTokenCount() int {
	return m.TokenCount()
}

// GetValidTokenCount returns the number of valid tokens in the store.
func (m *MockStore) GetValidTokenCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	count := 0
	for _, token := range m.tokens {
		if m.IsTokenValid(token) {
			count++
		}
	}
	return count
}

// WasCleared returns true if Clear() was called.
func (m *MockStore) WasCleared() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.cleared
}

// ErrTokenNotFound is returned when a token is not found in the mock store.
var ErrTokenNotFound = errors.New("token not found")
