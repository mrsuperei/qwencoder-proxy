package auth

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gofrs/flock"
	"github.com/google/uuid"
	"github.com/sunbankio/qwencoder-proxy/logging"
)

// ProviderToken represents a single stored token with metadata
type ProviderToken struct {
	ID           string  `json:"id"` // Unique token ID (UUID)
	AccessToken  string  `json:"access_token"`
	RefreshToken string  `json:"refresh_token"`
	TokenType    string  `json:"token_type"`
	ExpiryDate   int64   `json:"expiry_date"`
	Email        string  `json:"email"`                  // Extracted from OAuth response
	ResourceURL  string  `json:"resource_url,omitempty"` // For Qwen
	Scope        string  `json:"scope,omitempty"`        // For Gemini
	APIKey       string  `json:"api_key,omitempty"`      // For IFlow
	Healthy      bool    `json:"healthy"`                // Track if token is working
	HealthScore  float64 `json:"health_score"`           // 0.0-1.0 health score
	LastUsed     int64   `json:"last_used"`              // Timestamp of last use
	CreatedAt    int64   `json:"created_at"`             // Token creation timestamp
	ErrorCount   int     `json:"error_count"`            // Number of consecutive errors
	LastError    string  `json:"last_error,omitempty"`   // Last error message
}

// StoreSettings contains provider-specific settings
type StoreSettings struct {
	SelectionStrategy string `json:"selection_strategy"` // "random", "round_robin", "least_used"
	RefreshBufferSec  int    `json:"refresh_buffer_sec"` // Seconds before expiry to refresh
	MaxErrorCount     int    `json:"max_error_count"`    // Max errors before marking unhealthy
}

// MultiTokenStore manages multiple tokens for a provider
type MultiTokenStore struct {
	ProviderID  string          `json:"provider_id"`
	Version     int             `json:"version"` // Schema version
	Tokens      []ProviderToken `json:"tokens"`  // In-memory cache of tokens
	Settings    StoreSettings   `json:"settings"`
	mu          sync.RWMutex
	providerDir string // Provider directory path (e.g., ".credentials/gemini")
	logger      *logging.Logger
}

// NewMultiTokenStore creates a new MultiTokenStore instance
func NewMultiTokenStore(providerID, filePath string, logger *logging.Logger) *MultiTokenStore {
	// Extract provider directory from filePath
	// filePath is like ".credentials/gemini.json"
	// providerDir will be ".credentials/gemini"
	providerDir := filepath.Dir(filePath)
	if providerDir == "." {
		providerDir = ".credentials"
	}
	providerDir = filepath.Join(providerDir, providerID)

	return &MultiTokenStore{
		ProviderID:  providerID,
		Version:     StoreVersion,
		Tokens:      []ProviderToken{},
		providerDir: providerDir,
		Settings: StoreSettings{
			SelectionStrategy: DefaultSelectionStrategy,
			RefreshBufferSec:  DefaultRefreshBufferSec,
			MaxErrorCount:     DefaultMaxErrorCount,
		},
		logger: logger,
	}
}

// Load loads tokens from individual JSON files in the provider directory
func (mts *MultiTokenStore) Load() error {
	mts.mu.Lock()
	defer mts.mu.Unlock()

	// Create the provider directory if it doesn't exist
	if err := mts.ensureProviderDirectory(); err != nil {
		return fmt.Errorf("failed to create provider directory: %w", err)
	}

	// Check if provider directory exists
	if _, err := os.Stat(mts.providerDir); os.IsNotExist(err) {
		// Directory doesn't exist, initialize with empty store
		mts.Tokens = []ProviderToken{}
		mts.Version = StoreVersion
		mts.logger.InfoLog("No existing token store found, initialized empty store for provider: %s", mts.ProviderID)
		return nil
	}

	// Load all JSON files from the provider directory
	return mts.loadInternal()
}

// loadInternal performs the actual load operation (caller must hold lock)
func (mts *MultiTokenStore) loadInternal() error {
	// Read all JSON files from the provider directory
	entries, err := os.ReadDir(mts.providerDir)
	if err != nil {
		return fmt.Errorf("failed to read provider directory: %w", err)
	}

	loadedTokens := make([]ProviderToken, 0)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		// Only process .json files
		if !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		// Skip settings.json and other non-token files
		if entry.Name() == "settings.json" {
			continue
		}

		filePath := filepath.Join(mts.providerDir, entry.Name())
		data, err := os.ReadFile(filePath)
		if err != nil {
			mts.logger.WarningLog("Failed to read token file %s: %v", filePath, err)
			continue
		}

		var token ProviderToken
		if err := json.Unmarshal(data, &token); err != nil {
			mts.logger.WarningLog("Failed to parse token file %s: %v", filePath, err)
			continue
		}

		loadedTokens = append(loadedTokens, token)
	}

	// Try to load settings from settings.json
	settingsPath := filepath.Join(mts.providerDir, "settings.json")
	if data, err := os.ReadFile(settingsPath); err == nil {
		var settings StoreSettings
		if err := json.Unmarshal(data, &settings); err == nil {
			mts.Settings = settings
		}
	}

	mts.Tokens = loadedTokens
	mts.Version = StoreVersion
	mts.logger.InfoLog("Loaded token store for provider: %s with %d tokens", mts.ProviderID, len(mts.Tokens))
	return nil
}

// migrateFromLegacy migrates legacy single-token format to multi-token format
func (mts *MultiTokenStore) migrateFromLegacy(legacyFilePath string) error {
	mts.logger.InfoLog("Attempting to migrate legacy format for provider: %s", mts.ProviderID)

	// Read the legacy file
	data, err := os.ReadFile(legacyFilePath)
	if err != nil {
		return fmt.Errorf("failed to read legacy file: %w", err)
	}

	// Try to parse as legacy OAuthCreds format
	var legacyCreds struct {
		AccessToken  string `json:"access_token"`
		TokenType    string `json:"token_type"`
		RefreshToken string `json:"refresh_token"`
		ResourceURL  string `json:"resource_url,omitempty"`
		ExpiryDate   int64  `json:"expiry_date"`
	}

	if err := json.Unmarshal(data, &legacyCreds); err != nil {
		return fmt.Errorf("failed to parse legacy format: %w", err)
	}

	// Check if this is actually a legacy format (has access_token field)
	if legacyCreds.AccessToken == "" {
		return fmt.Errorf("not a valid legacy format")
	}

	// Create backup of legacy file
	backupPath := legacyFilePath + ".backup"
	if err := os.WriteFile(backupPath, data, CredentialsFileMode); err != nil {
		mts.logger.WarningLog("Failed to create backup of legacy file: %v", err)
	} else {
		mts.logger.InfoLog("Legacy file backed up to: %s", backupPath)
	}

	// Create a new token from legacy data
	token := ProviderToken{
		ID:           GenerateTokenID(),
		AccessToken:  legacyCreds.AccessToken,
		RefreshToken: legacyCreds.RefreshToken,
		TokenType:    legacyCreds.TokenType,
		ExpiryDate:   legacyCreds.ExpiryDate,
		ResourceURL:  legacyCreds.ResourceURL,
		Email:        "", // Email not available in legacy format
		Healthy:      true,
		HealthScore:  1.0,
		LastUsed:     GetCurrentTimestamp(),
		CreatedAt:    GetCurrentTimestamp(),
		ErrorCount:   0,
	}

	// Ensure provider directory exists
	if err := mts.ensureProviderDirectory(); err != nil {
		return fmt.Errorf("failed to create provider directory: %w", err)
	}

	// Save the migrated token to individual file
	if err := mts.saveTokenToFile(token); err != nil {
		return fmt.Errorf("failed to save migrated token: %w", err)
	}

	mts.Tokens = []ProviderToken{token}
	mts.Version = StoreVersion

	// Save settings
	if err := mts.saveSettings(); err != nil {
		mts.logger.WarningLog("Failed to save settings after migration: %v", err)
	}

	mts.logger.InfoLog("Successfully migrated legacy format to multi-token store for provider: %s", mts.ProviderID)
	return nil
}

// Save saves tokens to individual JSON files
func (mts *MultiTokenStore) Save() error {
	mts.mu.Lock()
	defer mts.mu.Unlock()
	return mts.saveInternal()
}

// saveInternal performs the actual save operation (caller must hold lock)
func (mts *MultiTokenStore) saveInternal() error {
	// Ensure provider directory exists
	if err := mts.ensureProviderDirectory(); err != nil {
		return fmt.Errorf("failed to create provider directory: %w", err)
	}

	// Save each token to its own file
	for _, token := range mts.Tokens {
		if err := mts.saveTokenToFile(token); err != nil {
			mts.logger.WarningLog("Failed to save token %s: %v", token.ID, err)
			continue
		}
	}

	// Save settings
	if err := mts.saveSettings(); err != nil {
		mts.logger.WarningLog("Failed to save settings: %v", err)
	}

	mts.logger.DebugLog("Saved token store for provider: %s with %d tokens", mts.ProviderID, len(mts.Tokens))
	return nil
}

// saveSettings saves the store settings to settings.json
func (mts *MultiTokenStore) saveSettings() error {
	settingsPath := filepath.Join(mts.providerDir, "settings.json")
	data, err := json.MarshalIndent(mts.Settings, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal settings: %w", err)
	}

	if err := os.WriteFile(settingsPath, data, CredentialsFileMode); err != nil {
		return fmt.Errorf("failed to write settings file: %w", err)
	}

	return nil
}

// saveTokenToFile saves a single token to its own file
func (mts *MultiTokenStore) saveTokenToFile(token ProviderToken) error {
	filePath := mts.getFilePathForToken(token)

	// Use a file lock to prevent race conditions
	lockPath := filePath + ".lock"
	fileLock := flock.New(lockPath)
	locked, err := fileLock.TryLock()
	if err != nil {
		return fmt.Errorf("failed to acquire file lock: %w", err)
	}
	if !locked {
		return fmt.Errorf("failed to acquire file lock, another process is holding it")
	}
	defer fileLock.Unlock()

	// Write to temp file first
	tmpPath := filePath + ".tmp"
	data, err := json.MarshalIndent(token, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal token: %w", err)
	}

	if err := os.WriteFile(tmpPath, data, CredentialsFileMode); err != nil {
		return fmt.Errorf("failed to write temp file: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tmpPath, filePath); err != nil {
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	return nil
}

// AddToken adds a new token to the store
func (mts *MultiTokenStore) AddToken(token ProviderToken) error {
	mts.mu.Lock()
	defer mts.mu.Unlock()

	// Validate token
	if token.ID == "" {
		token.ID = GenerateTokenID()
	}
	if token.CreatedAt == 0 {
		token.CreatedAt = GetCurrentTimestamp()
	}
	if token.LastUsed == 0 {
		token.LastUsed = GetCurrentTimestamp()
	}
	if token.HealthScore == 0 {
		token.HealthScore = 1.0
	}
	if token.TokenType == "" {
		token.TokenType = "Bearer"
	}

	// Check if token with same ID already exists
	for i, t := range mts.Tokens {
		if t.ID == token.ID {
			// Handle email change - rename file if email changed
			if t.Email != token.Email {
				oldFilePath := mts.getFilePathForToken(t)
				newFilePath := mts.getFilePathForToken(token)
				if oldFilePath != newFilePath {
					// Delete old file if it exists
					if err := os.Remove(oldFilePath); err != nil && !os.IsNotExist(err) {
						mts.logger.WarningLog("Failed to delete old token file %s: %v", oldFilePath, err)
					}
				}
			}
			mts.logger.InfoLog("Updated existing token %s (by ID) for provider: %s", token.ID, mts.ProviderID)
			mts.Tokens[i] = token
			return mts.saveInternal()
		}
	}

	// Check for duplicate by refresh_token (same OAuth account)
	// This prevents multiple records for the same OAuth account
	if token.RefreshToken != "" {
		for i, t := range mts.Tokens {
			if t.RefreshToken == token.RefreshToken {
				// Handle email change - rename file if email changed
				if t.Email != token.Email {
					oldFilePath := mts.getFilePathForToken(t)
					newFilePath := mts.getFilePathForToken(token)
					if oldFilePath != newFilePath {
						// Delete old file if it exists
						if err := os.Remove(oldFilePath); err != nil && !os.IsNotExist(err) {
							mts.logger.WarningLog("Failed to delete old token file %s: %v", oldFilePath, err)
						}
					}
				}
				mts.logger.InfoLog("Found duplicate token by refresh_token for provider: %s - updating existing token %s instead of creating new one",
					mts.ProviderID, t.ID)
				// Preserve the existing token's ID and CreatedAt, but update other fields
				token.ID = t.ID
				token.CreatedAt = t.CreatedAt
				mts.Tokens[i] = token
				return mts.saveInternal()
			}
		}
	}

	// Handle duplicate emails by appending counter
	if token.Email != "" {
		emailCount := 0
		for _, t := range mts.Tokens {
			if t.Email == token.Email {
				emailCount++
			}
		}
		if emailCount > 0 {
			// Append counter to email to make filename unique
			token.Email = fmt.Sprintf("%s-%d", token.Email, emailCount+1)
		}
	}

	// Add new token
	mts.logger.InfoLog("Added new token %s for provider: %s", token.ID, mts.ProviderID)
	mts.Tokens = append(mts.Tokens, token)
	return mts.saveInternal()
}

// RemoveToken removes a token by ID and deletes its file
func (mts *MultiTokenStore) RemoveToken(tokenID string) error {
	mts.mu.Lock()
	defer mts.mu.Unlock()

	for i, token := range mts.Tokens {
		if token.ID == tokenID {
			// Delete the token file
			filePath := mts.getFilePathForToken(token)
			if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
				mts.logger.WarningLog("Failed to delete token file %s: %v", filePath, err)
			}

			mts.Tokens = append(mts.Tokens[:i], mts.Tokens[i+1:]...)
			mts.logger.InfoLog("Removed token %s for provider: %s", tokenID, mts.ProviderID)
			return nil
		}
	}

	return fmt.Errorf("token not found: %s", tokenID)
}

// GetToken gets a specific token by ID
func (mts *MultiTokenStore) GetToken(tokenID string) (*ProviderToken, error) {
	mts.mu.RLock()
	defer mts.mu.RUnlock()

	for i := range mts.Tokens {
		if mts.Tokens[i].ID == tokenID {
			return &mts.Tokens[i], nil
		}
	}

	return nil, fmt.Errorf("token not found: %s", tokenID)
}

// ListTokens lists all tokens
func (mts *MultiTokenStore) ListTokens() []ProviderToken {
	mts.mu.RLock()
	defer mts.mu.RUnlock()

	// Return a copy to prevent external modifications
	tokens := make([]ProviderToken, len(mts.Tokens))
	copy(tokens, mts.Tokens)
	return tokens
}

// UpdateToken updates a token with a function
func (mts *MultiTokenStore) UpdateToken(tokenID string, updateFunc func(*ProviderToken)) error {
	mts.mu.Lock()
	defer mts.mu.Unlock()

	for i := range mts.Tokens {
		if mts.Tokens[i].ID == tokenID {
			oldToken := mts.Tokens[i]
			updateFunc(&mts.Tokens[i])

			// Handle email change - rename file if email changed
			if oldToken.Email != mts.Tokens[i].Email {
				oldFilePath := mts.getFilePathForToken(oldToken)
				newFilePath := mts.getFilePathForToken(mts.Tokens[i])
				if oldFilePath != newFilePath {
					// Delete old file if it exists
					if err := os.Remove(oldFilePath); err != nil && !os.IsNotExist(err) {
						mts.logger.WarningLog("Failed to delete old token file %s: %v", oldFilePath, err)
					}
				}
			}

			mts.logger.DebugLog("Updated token %s for provider: %s", tokenID, mts.ProviderID)
			return mts.saveInternal()
		}
	}

	return fmt.Errorf("token not found: %s", tokenID)
}

// MarkTokenHealthy marks a token as healthy
func (mts *MultiTokenStore) MarkTokenHealthy(tokenID string) error {
	return mts.UpdateToken(tokenID, func(token *ProviderToken) {
		token.Healthy = true
		token.HealthScore = 1.0
		token.ErrorCount = 0
		token.LastError = ""
		token.LastUsed = GetCurrentTimestamp()
	})
}

// MarkTokenUnhealthy marks a token as unhealthy
func (mts *MultiTokenStore) MarkTokenUnhealthy(tokenID string, err error) error {
	return mts.UpdateToken(tokenID, func(token *ProviderToken) {
		token.Healthy = false
		token.ErrorCount++
		token.HealthScore = max(0.0, token.HealthScore-0.2)
		if err != nil {
			token.LastError = err.Error()
		}
	})
}

// IsTokenValid checks if token is valid (not expired and healthy)
func (mts *MultiTokenStore) IsTokenValid(token ProviderToken) bool {
	if !token.Healthy {
		return false
	}
	if token.ExpiryDate == 0 {
		return false
	}
	// Use the refresh buffer from settings
	bufferMs := int64(mts.Settings.RefreshBufferSec) * 1000
	return time.Now().UnixMilli() < token.ExpiryDate-bufferMs
}

// GetValidTokens gets all valid tokens
func (mts *MultiTokenStore) GetValidTokens() []ProviderToken {
	mts.mu.RLock()
	defer mts.mu.RUnlock()

	validTokens := make([]ProviderToken, 0)
	for _, token := range mts.Tokens {
		if mts.IsTokenValid(token) {
			validTokens = append(validTokens, token)
		}
	}
	return validTokens
}

// GetTokenCount gets total token count
func (mts *MultiTokenStore) GetTokenCount() int {
	mts.mu.RLock()
	defer mts.mu.RUnlock()
	return len(mts.Tokens)
}

// GetValidTokenCount gets valid token count
func (mts *MultiTokenStore) GetValidTokenCount() int {
	mts.mu.RLock()
	defer mts.mu.RUnlock()
	count := 0
	for _, token := range mts.Tokens {
		if mts.IsTokenValid(token) {
			count++
		}
	}
	return count
}

// GenerateTokenID generates a unique token ID using UUID
func GenerateTokenID() string {
	return uuid.New().String()
}

// GetCurrentTimestamp gets current Unix timestamp in milliseconds
func GetCurrentTimestamp() int64 {
	return time.Now().UnixMilli()
}

// getFilePathForToken returns the file path for a token
// Format: .credentials/{providerID}/{email}.json
// If email is empty, uses "unknown-{tokenID}.json"
func (mts *MultiTokenStore) getFilePathForToken(token ProviderToken) string {
	filename := mts.sanitizeEmailForFilename(token.Email)
	if filename == "" {
		filename = fmt.Sprintf("unknown-%s", token.ID)
	}
	return filepath.Join(mts.providerDir, filename+".json")
}

// sanitizeEmailForFilename sanitizes an email address for use as a filename
// Replaces @ with _ and removes other invalid characters
func (mts *MultiTokenStore) sanitizeEmailForFilename(email string) string {
	if email == "" {
		return ""
	}

	// Replace @ with _
	sanitized := strings.ReplaceAll(email, "@", "_")

	// Remove other invalid characters for filenames
	// Windows invalid characters: \ / : * ? " < > |
	// Unix invalid characters: /
	invalidChars := []string{"\\", ":", "*", "?", "\"", "<", ">", "|"}
	for _, char := range invalidChars {
		sanitized = strings.ReplaceAll(sanitized, char, "")
	}

	return sanitized
}

// ensureProviderDirectory creates the provider directory if it doesn't exist
func (mts *MultiTokenStore) ensureProviderDirectory() error {
	if err := os.MkdirAll(mts.providerDir, CredentialsDirMode); err != nil {
		return fmt.Errorf("failed to create provider directory %s: %w", mts.providerDir, err)
	}
	return nil
}
