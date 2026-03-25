# Extract OAuth Configuration

**Priority:** LOW  
**Estimated Time:** 1 day  
**Complexity:** Low  
**Files to Create:** 1  
**Files to Modify:** 2

---

## Problem Description

OAuth credentials (client IDs, secrets, URLs) are hardcoded throughout the codebase. This creates:
- Security risk (credentials in source code)
- Difficult to rotate credentials
- Hard to manage multiple environments
- Configuration drift between environments
- No way to override without code changes

### Current State

**Hardcoded Credentials in `internal/restapi/rest_api.go`:**

```go
// Line 141-186: Hardcoded OAuth credentials
geminiRefresher := gemini.NewGeminiTokenRefresher(
	"681255809395-oo8ft2oprdrnp9e3aqf6av3hmdib135j.apps.googleusercontent.com",  // ❌ Hardcoded
	"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",                   // ❌ Hardcoded
	"https://oauth2.googleapis.com/token",                           // ❌ Hardcoded
	s.logger,
)

qwenRefresher := qwen.NewQwenTokenRefresher(s.logger)
// No credentials passed - uses internal defaults

iflowRefresher := iflow.NewIFlowTokenRefresher(
	"10009311001",        // ❌ Hardcoded
	"4Z3YjXycVsQvyGF1etiNlIBB4RsqSDtW",  // ❌ Hardcoded
	"https://iflow.cn/oauth/token",                               // ❌ Hardcoded
	s.logger,
)
```

**Problems:**
- Credentials exposed in source code
- Can't rotate without code changes
- Can't use different credentials per environment
- Security vulnerability if code is leaked
- No audit trail for credential changes

---

## Solution Architecture

### Configuration-Based OAuth Credentials

Move all OAuth credentials to configuration (environment variables or config file) and load them at runtime.

**Benefits:**
- Credentials not in source code
- Easy to rotate credentials
- Support for multiple environments
- Better security (credentials in secrets management)
- Configuration audit trail
- No code changes for credential updates

---

## Implementation Plan

### Phase 1: Create OAuth Configuration (Day 1)

#### Step 1.1: Create OAuth Config Structure

**New File:** `internal/config/oauth_config.go`

```go
package config

import (
	"fmt"
	"os"
)

// OAuthConfig holds OAuth configuration for providers
type OAuthConfig struct {
	// Provider configurations
	Gemini     ProviderOAuthConfig `mapstructure:"gemini"`
	Qwen       ProviderOAuthConfig `mapstructure:"qwen"`
	Kiro        ProviderOAuthConfig `mapstructure:"kiro"`
	Antigravity ProviderOAuthConfig `mapstructure:"antigravity"`
	IFlow       ProviderOAuthConfig `mapstructure:"iflow"`
}

// ProviderOAuthConfig holds OAuth configuration for a single provider
type ProviderOAuthConfig struct {
	// OAuth endpoints
	ClientID     string `mapstructure:"client_id"`
	ClientSecret string `mapstructure:"client_secret"`
	AuthURL      string `mapstructure:"auth_url"`
	TokenURL     string `mapstructure:"token_url"`
	
	// OAuth settings
	Scopes       []string `mapstructure:"scopes"`
	RedirectURI   string `mapstructure:"redirect_uri"`
	
	// PKCE settings
	UsePKCE      bool   `mapstructure:"use_pkce"`
	CodeChallenge string `mapstructure:"code_challenge_method"`
}

// LoadOAuthConfig loads OAuth configuration from environment
func LoadOAuthConfig() (*OAuthConfig, error) {
	config := &OAuthConfig{
		Gemini: ProviderOAuthConfig{
			ClientID:     getEnvWithDefault("GEMINI_CLIENT_ID", ""),
			ClientSecret: getEnvWithDefault("GEMINI_CLIENT_SECRET", ""),
			AuthURL:      getEnvWithDefault("GEMINI_AUTH_URL", "https://accounts.google.com/o/oauth2/v2/auth"),
			TokenURL:     getEnvWithDefault("GEMINI_TOKEN_URL", "https://oauth2.googleapis.com/token"),
			Scopes:       []string{"openid", "email", "profile"},
			RedirectURI:   getEnvWithDefault("GEMINI_REDIRECT_URI", "http://localhost:8080/api/callback"),
			UsePKCE:      true,
		},
		Qwen: ProviderOAuthConfig{
			ClientID:     getEnvWithDefault("QWEN_CLIENT_ID", ""),
			ClientSecret: getEnvWithDefault("QWEN_CLIENT_SECRET", ""),
			AuthURL:      getEnvWithDefault("QWEN_AUTH_URL", ""),
			TokenURL:     getEnvWithDefault("QWEN_TOKEN_URL", ""),
			Scopes:       []string{"openid", "email", "profile", "offline_access"},
			RedirectURI:   getEnvWithDefault("QWEN_REDIRECT_URI", "http://localhost:8080/api/callback"),
			UsePKCE:      true,
		},
		Kiro: ProviderOAuthConfig{
			ClientID:     getEnvWithDefault("KIRO_CLIENT_ID", ""),
			ClientSecret: getEnvWithDefault("KIRO_CLIENT_SECRET", ""),
			AuthURL:      getEnvWithDefault("KIRO_AUTH_URL", ""),
			TokenURL:     getEnvWithDefault("KIRO_TOKEN_URL", ""),
			Scopes:       []string{},
			RedirectURI:   getEnvWithDefault("KIRO_REDIRECT_URI", "http://localhost:8080/api/callback"),
			UsePKCE:      false,
		},
		Antigravity: ProviderOAuthConfig{
			ClientID:     getEnvWithDefault("ANTIGRAVITY_CLIENT_ID", ""),
			ClientSecret: getEnvWithDefault("ANTIGRAVITY_CLIENT_SECRET", ""),
			AuthURL:      getEnvWithDefault("ANTIGRAVITY_AUTH_URL", "https://accounts.google.com/o/oauth2/v2/auth"),
			TokenURL:     getEnvWithDefault("ANTIGRAVITY_TOKEN_URL", "https://oauth2.googleapis.com/token"),
			Scopes:       []string{"openid", "email", "profile"},
			RedirectURI:   getEnvWithDefault("ANTIGRAVITY_REDIRECT_URI", "http://localhost:8080/api/callback"),
			UsePKCE:      true,
		},
		IFlow: ProviderOAuthConfig{
			ClientID:     getEnvWithDefault("IFLOW_CLIENT_ID", ""),
			ClientSecret: getEnvWithDefault("IFLOW_CLIENT_SECRET", ""),
			AuthURL:      getEnvWithDefault("IFLOW_AUTH_URL", ""),
			TokenURL:     getEnvWithDefault("IFLOW_TOKEN_URL", ""),
			Scopes:       []string{},
			RedirectURI:   getEnvWithDefault("IFLOW_REDIRECT_URI", "http://localhost:8080/api/callback"),
			UsePKCE:      false,
		},
	}
	
	// Validate required fields
	if config.Gemini.ClientID == "" {
		return nil, fmt.Errorf("GEMINI_CLIENT_ID is required")
	}
	if config.Gemini.ClientSecret == "" {
		return nil, fmt.Errorf("GEMINI_CLIENT_SECRET is required")
	}
	
	return config, nil
}

// getEnvWithDefault gets an environment variable with a default value
func getEnvWithDefault(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}

// ValidateOAuthConfig validates OAuth configuration
func ValidateOAuthConfig(config *OAuthConfig) error {
	if config == nil {
		return fmt.Errorf("OAuth config is nil")
	}
	
	// Validate Gemini config
	if config.Gemini.ClientID == "" {
		return fmt.Errorf("GEMINI_CLIENT_ID is required")
	}
	if config.Gemini.ClientSecret == "" {
		return fmt.Errorf("GEMINI_CLIENT_SECRET is required")
	}
	
	// Validate iFlow config
	if config.IFlow.ClientID == "" {
		return fmt.Errorf("IFLOW_CLIENT_ID is required")
	}
	if config.IFlow.ClientSecret == "" {
		return fmt.Errorf("IFLOW_CLIENT_SECRET is required")
	}
	
	return nil
}
```

#### Step 1.2: Update Config Package

**Modify File:** `internal/config/config.go`

**Location:** Add OAuth config to main Config struct

```go
// Add to Config struct
type Config struct {
	// ... existing fields ...
	
	// OAuth configuration
	OAuth OAuthConfig `mapstructure:"oauth"`
}

// Add to DefaultConfig()
func DefaultConfig() *Config {
	return &Config{
		// ... existing defaults ...
		OAuth: OAuthConfig{
			Gemini: ProviderOAuthConfig{
				ClientID:     "",
				ClientSecret: "",
				AuthURL:      "https://accounts.google.com/o/oauth2/v2/auth",
				TokenURL:     "https://oauth2.googleapis.com/token",
				Scopes:       []string{"openid", "email", "profile"},
				RedirectURI:   "http://localhost:8080/api/callback",
				UsePKCE:      true,
			},
			Qwen: ProviderOAuthConfig{
				ClientID:     "",
				ClientSecret: "",
				AuthURL:      "",
				TokenURL:     "",
				Scopes:       []string{"openid", "email", "profile", "offline_access"},
				RedirectURI:   "http://localhost:8080/api/callback",
				UsePKCE:      true,
			},
			Kiro: ProviderOAuthConfig{
				ClientID:     "",
				ClientSecret: "",
				AuthURL:      "",
				TokenURL:     "",
				Scopes:       []string{},
				RedirectURI:   "http://localhost:8080/api/callback",
				UsePKCE:      false,
			},
			Antigravity: ProviderOAuthConfig{
				ClientID:     "",
				ClientSecret: "",
				AuthURL:      "https://accounts.google.com/o/oauth2/v2/auth",
				TokenURL:     "https://oauth2.googleapis.com/token",
				Scopes:       []string{"openid", "email", "profile"},
				RedirectURI:   "http://localhost:8080/api/callback",
				UsePKCE:      true,
			},
			IFlow: ProviderOAuthConfig{
				ClientID:     "",
				ClientSecret: "",
				AuthURL:      "",
				TokenURL:     "",
				Scopes:       []string{},
				RedirectURI:   "http://localhost:8080/api/callback",
				UsePKCE:      false,
			},
		},
	}
}
```

### Phase 2: Update REST API (Day 1)

#### Step 2.1: Use OAuth Config in REST API

**Modify File:** `internal/restapi/rest_api.go`

**Location:** Update `registerProviderRefreshers()` method (around line 132)

```go
// BEFORE: Hardcoded credentials
func (s *Server) registerProviderRefreshers() error {
	// ...
	geminiRefresher := gemini.NewGeminiTokenRefresher(
		"681255809395-oo8ft2oprdrnp9e3aqf6av3hmdib135j.apps.googleusercontent.com",
		"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",
		"https://oauth2.googleapis.com/token",
		s.logger,
	)
	// ...
}

// AFTER: Use configuration
func (s *Server) registerProviderRefreshers() error {
	if s.multiTokenManager == nil {
		return fmt.Errorf("multi-token manager not initialized")
	}
	
	s.logger.InfoLog("[Server] Registering provider refreshers...")
	
	// Get OAuth config from server config
	oauthConfig := s.config.OAuth
	
	// Register Gemini refresher
	geminiRefresher := gemini.NewGeminiTokenRefresher(
		oauthConfig.Gemini.ClientID,
		oauthConfig.Gemini.ClientSecret,
		oauthConfig.Gemini.TokenURL,
		s.logger,
	)
	if err := s.multiTokenManager.RegisterRefresher("gemini-cli", geminiRefresher); err != nil {
		s.logger.ErrorLog("[Server] Failed to register Gemini refresher: %v", err)
		return fmt.Errorf("failed to register Gemini refresher: %w", err)
	}
	s.logger.InfoLog("[Server] Registered Gemini refresher")
	
	// Register Qwen refresher
	qwenRefresher := qwen.NewQwenTokenRefresher(s.logger)
	if err := s.multiTokenManager.RegisterRefresher("qwen", qwenRefresher); err != nil {
		s.logger.ErrorLog("[Server] Failed to register Qwen refresher: %v", err)
		return fmt.Errorf("failed to register Qwen refresher: %w", err)
	}
	s.logger.InfoLog("[Server] Registered Qwen refresher")
	
	// Register iFlow refresher
	iflowRefresher := iflow.NewIFlowTokenRefresher(
		oauthConfig.IFlow.ClientID,
		oauthConfig.IFlow.ClientSecret,
		oauthConfig.IFlow.TokenURL,
		s.logger,
	)
	if err := s.multiTokenManager.RegisterRefresher("iflow", iflowRefresher); err != nil {
		s.logger.ErrorLog("[Server] Failed to register iFlow refresher: %v", err)
		return fmt.Errorf("failed to register iFlow refresher: %w", err)
	}
	s.logger.InfoLog("[Server] Registered iFlow refresher")
	
	// Register Antigravity refresher
	antigravityRefresher := antigravity.NewAntigravityTokenRefresher(
		oauthConfig.Antigravity.ClientID,
		oauthConfig.Antigravity.ClientSecret,
		oauthConfig.Antigravity.TokenURL,
		s.logger,
	)
	if err := s.multiTokenManager.RegisterRefresher("antigravity", antigravityRefresher); err != nil {
		s.logger.ErrorLog("[Server] Failed to register Antigravity refresher: %v", err)
		return fmt.Errorf("failed to register Antigravity refresher: %w", err)
	}
	s.logger.InfoLog("[Server] Registered Antigravity refresher")
	
	s.logger.InfoLog("[Server] All provider refreshers registered successfully")
	return nil
}
```

#### Step 2.2: Update Server Config

**Modify File:** `internal/restapi/rest_api.go`

**Location:** Add OAuth config to Server struct

```go
// Add to Server struct
type Server struct {
	config            *Config
	registry          *ProviderRegistry
	stateManager      *StateManager
	logger            logging.Logger
	httpClient        *http.Client
	tokenStores       map[string]tokpkg.TokenStore
	tokenManagers     map[string]*tokpkg.TokenManager
	multiTokenManager *tokpkg.MultiTokenManager
	oauthConfig       *config.OAuthConfig  // Add OAuth config
}
```

#### Step 2.3: Update Server Constructor

**Modify File:** `internal/restapi/rest_api.go`

**Location:** Update `NewServer()` method (around line 68)

```go
// BEFORE: No OAuth config
func NewServer(config *Config, logger logging.Logger) *Server {
	return &Server{
		config:            config,
		registry:          NewProviderRegistry(),
		stateManager:      NewStateManager(logger),
		logger:            logger,
		httpClient:        &http.Client{Timeout: 30 * time.Second},
		tokenStores:       make(map[string]tokpkg.TokenStore),
		tokenManagers:     make(map[string]*tokpkg.TokenManager),
		multiTokenManager: nil,
	}
}

// AFTER: Include OAuth config
func NewServer(config *Config, logger logging.Logger) *Server {
	return &Server{
		config:            config,
		registry:          NewProviderRegistry(),
		stateManager:      NewStateManager(logger),
		logger:            logger,
		httpClient:        &http.Client{Timeout: 30 * time.Second},
		tokenStores:       make(map[string]tokpkg.TokenStore),
		tokenManagers:     make(map[string]*tokpkg.TokenManager),
		multiTokenManager: nil,
		oauthConfig:       &config.OAuth,  // Add OAuth config
	}
}
```

### Phase 3: Update Main Package (Day 1)

#### Step 3.1: Load OAuth Config

**Modify File:** `cmd/main.go`

**Location:** Add OAuth config loading (around line 34)

```go
// BEFORE: No OAuth config loading
cfg := config.DefaultConfig()

// AFTER: Load OAuth config
cfg := config.DefaultConfig()

// Load OAuth configuration
oauthConfig, err := config.LoadOAuthConfig()
if err != nil {
	logger.ErrorLog("Failed to load OAuth config: %v", err)
	os.Exit(1)
}

// Validate OAuth configuration
if err := config.ValidateOAuthConfig(oauthConfig); err != nil {
	logger.ErrorLog("Invalid OAuth config: %v", err)
	os.Exit(1)
}

// Set OAuth config in main config
cfg.OAuth = oauthConfig
```

---

## Testing

### Unit Tests

**New File:** `internal/config/oauth_config_test.go`

```go
package config

import (
	"os"
	"testing"
	"github.com/stretchr/testify/assert"
)

func TestLoadOAuthConfig(t *testing.T) {
	// Set environment variables
	os.Setenv("GEMINI_CLIENT_ID", "test_client_id")
	os.Setenv("GEMINI_CLIENT_SECRET", "test_secret")
	os.Setenv("IFLOW_CLIENT_ID", "iflow_client_id")
	os.Setenv("IFLOW_CLIENT_SECRET", "iflow_secret")
	
	config, err := LoadOAuthConfig()
	
	assert.NoError(t, err)
	assert.Equal(t, "test_client_id", config.Gemini.ClientID)
	assert.Equal(t, "test_secret", config.Gemini.ClientSecret)
	assert.Equal(t, "iflow_client_id", config.IFlow.ClientID)
	assert.Equal(t, "iflow_secret", config.IFlow.ClientSecret)
}

func TestLoadOAuthConfigWithDefaults(t *testing.T) {
	// Clear environment variables
	os.Unsetenv("GEMINI_CLIENT_ID")
	os.Unsetenv("GEMINI_CLIENT_SECRET")
	
	config, err := LoadOAuthConfig()
	
	assert.NoError(t, err)
	// Should use defaults for missing env vars
	assert.Equal(t, "https://accounts.google.com/o/oauth2/v2/auth", config.Gemini.AuthURL)
	assert.Equal(t, "https://oauth2.googleapis.com/token", config.Gemini.TokenURL)
}

func TestValidateOAuthConfig(t *testing.T) {
	// Test valid config
	validConfig := &OAuthConfig{
		Gemini: ProviderOAuthConfig{
			ClientID:     "test_id",
			ClientSecret: "test_secret",
		},
	}
	
	err := ValidateOAuthConfig(validConfig)
	assert.NoError(t, err)
	
	// Test invalid config (missing client ID)
	invalidConfig := &OAuthConfig{
		Gemini: ProviderOAuthConfig{
			ClientID:     "",
			ClientSecret: "test_secret",
		},
	}
	
	err = ValidateOAuthConfig(invalidConfig)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "GEMINI_CLIENT_ID is required")
}
```

---

## Configuration

### Environment Variables

Add to `.env` or environment:

```bash
# Gemini OAuth Configuration
GEMINI_CLIENT_ID=your_gemini_client_id
GEMINI_CLIENT_SECRET=your_gemini_client_secret
GEMINI_AUTH_URL=https://accounts.google.com/o/oauth2/v2/auth
GEMINI_TOKEN_URL=https://oauth2.googleapis.com/token
GEMINI_REDIRECT_URI=http://localhost:8080/api/callback

# Qwen OAuth Configuration
QWEN_CLIENT_ID=your_qwen_client_id
QWEN_CLIENT_SECRET=your_qwen_client_secret
QWEN_AUTH_URL=
QWEN_TOKEN_URL=
QWEN_REDIRECT_URI=http://localhost:8080/api/callback

# iFlow OAuth Configuration
IFLOW_CLIENT_ID=your_iflow_client_id
IFLOW_CLIENT_SECRET=your_iflow_client_secret
IFLOW_AUTH_URL=
IFLOW_TOKEN_URL=https://iflow.cn/oauth/token
IFLOW_REDIRECT_URI=http://localhost:8080/api/callback

# Antigravity OAuth Configuration
ANTIGRAVITY_CLIENT_ID=your_antigravity_client_id
ANTIGRAVITY_CLIENT_SECRET=your_antigravity_client_secret
ANTIGRAVITY_AUTH_URL=https://accounts.google.com/o/oauth2/v2/auth
ANTIGRAVITY_TOKEN_URL=https://oauth2.googleapis.com/token
ANTIGRAVITY_REDIRECT_URI=http://localhost:8080/api/callback
```

### Configuration File

**New File:** `.env.example`

```bash
# OAuth Configuration
# Copy this file to .env and fill in your credentials

# Gemini OAuth
GEMINI_CLIENT_ID=
GEMINI_CLIENT_SECRET=
GEMINI_AUTH_URL=https://accounts.google.com/o/oauth2/v2/auth
GEMINI_TOKEN_URL=https://oauth2.googleapis.com/token
GEMINI_REDIRECT_URI=http://localhost:8080/api/callback

# Qwen OAuth
QWEN_CLIENT_ID=
QWEN_CLIENT_SECRET=
QWEN_AUTH_URL=
QWEN_TOKEN_URL=
QWEN_REDIRECT_URI=http://localhost:8080/api/callback

# iFlow OAuth
IFLOW_CLIENT_ID=
IFLOW_CLIENT_SECRET=
IFLOW_AUTH_URL=
IFLOW_TOKEN_URL=https://iflow.cn/oauth/token
IFLOW_REDIRECT_URI=http://localhost:8080/api/callback

# Antigravity OAuth
ANTIGRAVITY_CLIENT_ID=
ANTIGRAVITY_CLIENT_SECRET=
ANTIGRAVITY_AUTH_URL=https://accounts.google.com/o/oauth2/v2/auth
ANTIGRAVITY_TOKEN_URL=https://oauth2.googleapis.com/token
ANTIGRAVITY_REDIRECT_URI=http://localhost:8080/api/callback
```

---

## Verification Checklist

### Phase 1: OAuth Config Package
- [ ] `internal/config/oauth_config.go` created
- [ ] `OAuthConfig` struct defined
- [ ] `ProviderOAuthConfig` struct defined
- [ ] `LoadOAuthConfig()` implemented
- [ ] `ValidateOAuthConfig()` implemented
- [ ] Unit tests pass

### Phase 2: REST API Updates
- [ ] `rest_api.go` imports config package
- [ ] `registerProviderRefreshers()` uses OAuth config
- [ ] Server struct updated with OAuth config
- [ ] `NewServer()` uses OAuth config
- [ ] All tests pass

### Phase 3: Main Package Updates
- [ ] `main.go` loads OAuth config
- [ ] OAuth config validated
- [ ] OAuth config set in main config
- [ ] All tests pass

### Phase 4: Configuration
- [ ] `.env.example` created
- [ ] Environment variables documented
- [ ] Default values documented
- [ ] All tests pass

---

## Impact

**Positive:**
- Credentials not in source code
- Better security (credentials in environment)
- Easy to rotate credentials
- Support for multiple environments
- Configuration audit trail
- No code changes for credential updates

**Code Volume Changes:**
- Added: ~200 lines in `oauth_config.go`
- Added: ~50 lines in `config.go`
- Removed: ~50 lines of hardcoded credentials
- Net addition: ~200 lines (but with better security)

**Risk:**
- Low - configuration is a well-established pattern
- Backward compatible (defaults provided)
- No breaking changes to existing functionality

**Side Effects:**
- Environment variables required for OAuth
- Configuration file can be used instead
- Better security practices
- Easier deployment across environments

---

## Security Considerations

### Credential Management

1. **Never Commit Credentials:**
   - Add `.env` to `.gitignore`
   - Use environment variables in production
   - Use secrets management systems (AWS Secrets Manager, Vault)

2. **Credential Rotation:**
   - Rotate credentials regularly
   - Use short-lived credentials
   - Implement credential rotation automation

3. **Access Control:**
   - Restrict access to configuration
   - Use least privilege principle
   - Audit credential access

4. **Credential Storage:**
   - Store credentials securely
   - Encrypt credentials at rest
   - Use secure key management

---

## Future Enhancements

1. **Multiple OAuth Configs:**
   - Support for multiple OAuth configurations
   - Config profiles (dev, staging, prod)
   - Dynamic config reloading

2. **Credential Validation:**
   - Validate credentials against provider
   - Test credential validity on startup
   - Credential health checks

3. **Secrets Management:**
   - Integration with secrets managers
   - Automatic credential rotation
   - Credential leak detection

4. **Configuration Encryption:**
   - Encrypt sensitive config values
   - Secure config file storage
   - Runtime decryption

5. **OAuth 2.0 Support:**
   - Support for OAuth 2.0 features
   - PKCE enhancements
   - JWT token validation
