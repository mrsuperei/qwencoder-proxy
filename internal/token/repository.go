package token

// TokenRepository defines the interface for token storage operations.
// This interface is used by TokenManager and provides all methods needed
// for token management operations.
type TokenRepository interface {
	// Load loads all tokens from storage.
	Load() (map[string]TokenMetadata, error)

	// Save persists all tokens to storage.
	Save(tokens map[string]TokenMetadata) error

	// GetCredentialsPath returns the path to the credentials file/directory.
	GetCredentialsPath() string

	// Clear removes all tokens from storage.
	Clear() error

	// GetToken retrieves a single token by ID.
	GetToken(id string) (*ProviderToken, error)

	// ListTokens returns all tokens from storage.
	ListTokens() []ProviderToken

	// UpdateToken updates a token using the provided update function.
	UpdateToken(id string, update func(*TokenMetadata)) error

	// IsTokenValid checks if a token is valid (not expired and healthy).
	IsTokenValid(token ProviderToken) bool

	// GetValidTokens returns all valid tokens from storage.
	GetValidTokens() []ProviderToken

	// GetTokenCount returns the number of tokens in storage.
	GetTokenCount() int

	// GetValidTokenCount returns the number of valid tokens in storage.
	GetValidTokenCount() int
}

// Ensure SQLiteStore implements TokenRepository at compile time.
var _ TokenRepository = (*SQLiteStore)(nil)

// Ensure MockStore implements TokenRepository at compile time.
var _ TokenRepository = (*MockStore)(nil)
