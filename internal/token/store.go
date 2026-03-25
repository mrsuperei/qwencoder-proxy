package token

// ProviderToken represents a single stored token with metadata
type ProviderToken struct {
	ID               string       `json:"id"` // Unique token ID (UUID)
	AccessToken      string       `json:"access_token"`
	RefreshToken     string       `json:"refresh_token"`
	TokenType        string       `json:"token_type"`
	ExpiryDate       int64        `json:"expiry_date"`
	Email            string       `json:"email"`                        // Extracted from OAuth response
	ResourceURL      string       `json:"resource_url,omitempty"`       // For Qwen
	Scope            string       `json:"scope,omitempty"`              // For Gemini
	APIKey           string       `json:"api_key,omitempty"`            // For IFlow
	ProjectID        string       `json:"project_id,omitempty"`         // For Gemini - Cloud Code Assist project ID
	Healthy          bool         `json:"healthy"`                      // Track if token is working
	HealthScore      float64      `json:"health_score"`                 // 0.0-1.0 health score
	LastUsed         int64        `json:"last_used"`                    // Timestamp of last use
	CreatedAt        int64        `json:"created_at"`                   // Token creation timestamp
	ErrorCount       int          `json:"error_count"`                  // Number of consecutive errors
	LastError        string       `json:"last_error,omitempty"`         // Last error message
	Proxy            *ProxyConfig `json:"proxy,omitempty"`              // Proxy configuration for this token
	ProxyHealthScore float64      `json:"proxy_health_score,omitempty"` // Separate health tracking for proxy (0.0-1.0)
}

// StoreSettings contains provider-specific settings
type StoreSettings struct {
	SelectionStrategy string `json:"selection_strategy"` // "random", "round_robin", "least_used"
	RefreshBufferSec  int    `json:"refresh_buffer_sec"` // Seconds before expiry to refresh
	MaxErrorCount     int    `json:"max_error_count"`    // Max errors before marking unhealthy
	UpdatedAt         int64  `json:"updated_at"`         // Unix timestamp in milliseconds when settings were last updated
}

// TokenMetadata is an alias for ProviderToken, representing token metadata for storage operations.
// This alias enables the TokenStore interface to work with existing ProviderToken implementations
// while providing a cleaner abstraction for storage operations.
type TokenMetadata = ProviderToken

// TokenStore defines the complete interface for token storage operations.
// This interface provides all methods needed for token management including
// CRUD operations, validation, settings management, and health tracking.
type TokenStore interface {
	// ========== Basic Operations ==========

	// Load loads all tokens from storage and returns them mapped by their unique ID.
	Load() (map[string]ProviderToken, error)

	// Save persists all tokens to storage, keyed by their unique ID.
	Save(tokens map[string]ProviderToken) error

	// Clear removes all tokens from storage.
	Clear() error

	// GetCredentialsPath returns the path to the credentials file/directory.
	GetCredentialsPath() string

	// ========== CRUD Operations ==========

	// GetToken retrieves a single token by ID.
	GetToken(id string) (*ProviderToken, error)

	// ListTokens returns all tokens from storage.
	ListTokens() []ProviderToken

	// AddToken adds a new token to storage.
	AddToken(token ProviderToken) error

	// UpdateToken updates a token using the provided update function.
	UpdateToken(id string, update func(*ProviderToken)) error

	// RemoveToken removes a token by ID.
	RemoveToken(id string) error

	// ========== Validation Operations ==========

	// IsTokenValid checks if a token is valid (not expired and healthy).
	IsTokenValid(token ProviderToken) bool

	// GetValidTokens returns all valid tokens from storage.
	GetValidTokens() []ProviderToken

	// ========== Query Operations ==========

	// GetTokenCount returns the total number of tokens in storage.
	GetTokenCount() int

	// GetValidTokenCount returns the number of valid tokens in storage.
	GetValidTokenCount() int

	// ========== Settings Operations ==========

	// GetSettings retrieves provider-specific settings from storage.
	GetSettings() (StoreSettings, error)

	// SaveSettings persists provider-specific settings to storage.
	SaveSettings(settings StoreSettings) error

	// ========== Health Operations ==========

	// MarkTokenHealthy marks a token as healthy.
	MarkTokenHealthy(id string) error

	// MarkTokenUnhealthy marks a token as unhealthy.
	MarkTokenUnhealthy(id string, err error) error
}

// Ensure SQLiteStore implements TokenStore at compile time.
var _ TokenStore = (*SQLiteStore)(nil)

// Ensure MockStore implements TokenStore at compile time.
var _ TokenStore = (*MockStore)(nil)
