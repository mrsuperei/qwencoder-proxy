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

// TokenStore defines token storage operations following the Dependency Inversion Principle.
// Enables swapping storage implementations (file, database, cloud, in-memory for tests).
type TokenStore interface {
	// Load loads all tokens from storage and returns them mapped by their unique ID.
	Load() (map[string]TokenMetadata, error)

	// Save persists all tokens to storage, keyed by their unique ID.
	Save(tokens map[string]TokenMetadata) error

	// GetCredentialsPath returns the path to the credentials file/directory.
	GetCredentialsPath() string

	// Clear removes all tokens from storage.
	Clear() error
}
