// Package restapi provides REST API endpoints for OAuth2 authentication flows
package restapi

import (
	"fmt"
	"os"
	"path/filepath"
)

// ProviderConfig holds the configuration for an OAuth provider
type ProviderConfig struct {
	ID            string   // Provider identifier (e.g., "qwen", "gemini")
	Name          string   // Display name
	Flow          string   // OAuth flow type: "device_code", "authorization_code", "external"
	ClientID      string   // OAuth client ID
	ClientSecret  string   // OAuth client secret (for authorization_code flow)
	AuthURL       string   // OAuth authorization URL
	TokenURL      string   // OAuth token URL
	DeviceAuthURL string   // Device code authorization URL (for device_code flow)
	Scopes        []string // OAuth scopes
	CredsDir      string   // Directory name for credentials (e.g., ".qwen")
	CredsFile     string   // Credentials filename
	Description   string   // Optional description for external providers
}

// ProviderInfo is a simplified representation of provider info for API responses
type ProviderInfo struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Flow        string   `json:"flow"`
	AuthURL     string   `json:"auth_url,omitempty"`
	TokenURL    string   `json:"token_url,omitempty"`
	Scopes      []string `json:"scopes,omitempty"`
	Description string   `json:"description,omitempty"`
}

// ProviderRegistry manages the registry of OAuth providers
type ProviderRegistry struct {
	providers map[string]*ProviderConfig
}

// NewProviderRegistry creates a new provider registry with default providers
func NewProviderRegistry() *ProviderRegistry {
	registry := &ProviderRegistry{
		providers: make(map[string]*ProviderConfig),
	}
	registry.registerDefaultProviders()
	return registry
}

// registerDefaultProviders registers all supported OAuth providers
func (pr *ProviderRegistry) registerDefaultProviders() {
	// Qwen - Device Code Flow
	pr.RegisterProvider(&ProviderConfig{
		ID:            "qwen",
		Name:          "Qwen",
		Flow:          "device_code",
		ClientID:      "f0304373b74a44d2b584a3fb70ca9e56",
		TokenURL:      "https://chat.qwen.ai/api/v1/oauth2/token",
		DeviceAuthURL: "https://chat.qwen.ai/api/v1/oauth2/device/code",
		Scopes:        []string{"openid", "profile", "email", "model.completion"},
		CredsDir:      ".qwen",
		CredsFile:     "qwenproxy_creds.json",
	})

	// Gemini - Authorization Code Flow
	pr.RegisterProvider(&ProviderConfig{
		ID:           "gemini",
		Name:         "Gemini (Google)",
		Flow:         "authorization_code",
		ClientID:     "681255809395-oo8ft2oprdrnp9e3aqf6av3hmdib135j.apps.googleusercontent.com",
		ClientSecret: "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",
		AuthURL:      "https://accounts.google.com/o/oauth2/v2/auth",
		TokenURL:     "https://oauth2.googleapis.com/token",
		Scopes:       []string{"https://www.googleapis.com/auth/cloud-platform"},
		CredsDir:     ".gemini",
		CredsFile:    "oauth_creds.json",
	})

	// iFlow - Authorization Code Flow
	pr.RegisterProvider(&ProviderConfig{
		ID:           "iflow",
		Name:         "iFlow",
		Flow:         "authorization_code",
		ClientID:     "10009311001",
		ClientSecret: "4Z3YjXycVsQvyGF1etiNlIBB4RsqSDtW",
		AuthURL:      "https://iflow.cn/oauth",
		TokenURL:     "https://iflow.cn/oauth/token",
		Scopes:       []string{"openid", "email", "profile", "offline_access"},
		CredsDir:     ".iflow",
		CredsFile:    "oauth_creds.json",
	})

	// Kiro - External (uses pre-existing AWS SSO credentials)
	pr.RegisterProvider(&ProviderConfig{
		ID:          "kiro",
		Name:        "Kiro (AWS SSO)",
		Flow:        "external",
		Description: "Uses pre-existing AWS SSO credentials from Kiro IDE",
		CredsDir:    ".aws/sso/cache",
		CredsFile:   "kiro-auth-token.json",
	})
}

// RegisterProvider adds a new provider to the registry
func (pr *ProviderRegistry) RegisterProvider(config *ProviderConfig) {
	pr.providers[config.ID] = config
}

// GetConfig retrieves a provider configuration by ID
func (pr *ProviderRegistry) GetConfig(providerID string) (*ProviderConfig, error) {
	config, ok := pr.providers[providerID]
	if !ok {
		return nil, fmt.Errorf("unknown provider: %s", providerID)
	}
	return config, nil
}

// ListProviders returns a list of all available providers
func (pr *ProviderRegistry) ListProviders() []ProviderInfo {
	providers := make([]ProviderInfo, 0, len(pr.providers))
	for _, config := range pr.providers {
		info := ProviderInfo{
			ID:     config.ID,
			Name:   config.Name,
			Flow:   config.Flow,
			Scopes: config.Scopes,
		}

		if config.AuthURL != "" {
			info.AuthURL = config.AuthURL
		}
		if config.TokenURL != "" {
			info.TokenURL = config.TokenURL
		}
		if config.Description != "" {
			info.Description = config.Description
		}

		providers = append(providers, info)
	}
	return providers
}

// GetCredentialsPath returns the full path to the credentials file for a provider
func (pr *ProviderRegistry) GetCredentialsPath(providerID string) (string, error) {
	config, err := pr.GetConfig(providerID)
	if err != nil {
		return "", err
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	return filepath.Join(homeDir, config.CredsDir, config.CredsFile), nil
}

// HasProvider checks if a provider exists in the registry
func (pr *ProviderRegistry) HasProvider(providerID string) bool {
	_, ok := pr.providers[providerID]
	return ok
}
