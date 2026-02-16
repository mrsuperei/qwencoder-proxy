// Package antigravity provides authentication for Antigravity provider
package antigravity

import (
	"github.com/sunbankio/qwencoder-proxy/provider/gemini"
)

// OAuthConfig holds OAuth configuration for Antigravity
// Antigravity uses Google OAuth with different client credentials
type OAuthConfig struct {
	ClientID     string
	ClientSecret string
	Scope        string
	RedirectPort int
	CredsDir     string
	CredsFile    string
}

// DefaultOAuthConfig returns default Antigravity OAuth configuration
func DefaultOAuthConfig() *OAuthConfig {
	return &OAuthConfig{
		ClientID:     "1071006060591-tmhssin2h21lcre235vtolojh4g403ep.apps.googleusercontent.com",
		ClientSecret: "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",
		Scope:        "https://www.googleapis.com/auth/cloud-platform",
		RedirectPort: 8086,
		CredsDir:     ".antigravity",
		CredsFile:    "oauth_creds.json",
	}
}

// Authenticator wraps the Gemini authenticator for Antigravity
// Antigravity uses the same Google OAuth flow as Gemini CLI
type Authenticator struct {
	*gemini.Authenticator
}

// NewAuthenticator creates a new Antigravity authenticator
func NewAuthenticator(config *OAuthConfig) *Authenticator {
	if config == nil {
		config = DefaultOAuthConfig()
	}

	// Convert Antigravity config to Gemini config
	geminiConfig := &gemini.OAuthConfig{
		ClientID:     config.ClientID,
		ClientSecret: config.ClientSecret,
		Scope:        config.Scope,
		RedirectPort: config.RedirectPort,
		CredsDir:     config.CredsDir,
		CredsFile:    config.CredsFile,
	}

	return &Authenticator{
		Authenticator: gemini.NewAuthenticator(geminiConfig),
	}
}
