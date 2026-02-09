package auth

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
