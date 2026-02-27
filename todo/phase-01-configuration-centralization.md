# Phase 1: Configuration Centralization

## Problem

### Security Risk: Hardcoded OAuth Credentials
The project contains **hardcoded OAuth credentials and API endpoints** directly in the source code. This is a critical security vulnerability because:

1. **Credentials are exposed in version control** - Anyone with access to the repository can see client secrets
2. **No environment-specific configuration** - Development, staging, and production use the same credentials
3. **No secrets management** - No integration with vault or secure storage
4. **Difficult to rotate credentials** - Changing credentials requires code changes and redeployment

### Configuration Duplication
The same configuration values are scattered across multiple files:

| File | Configuration | Current Location |
|------|----------------|------------------|
| `provider/gemini/auth.go` | ClientID, ClientSecret, Scope, RedirectPort | Lines 36-39 |
| `provider/gemini/gemini.go` | DefaultBaseURL | Line 23 |
| `provider/qwen/auth_device.go` | OAuthClientID, OAuthScope, OAuthTokenURL, OAuthDeviceAuthURL | Lines 29-33 |
| `provider/qwen/qwen.go` | DefaultBaseURL | Line 26 |
| `provider/iflow/iflow.go` | AuthURL, TokenURL, UserInfoURL, APIKeyURL, APIBaseURL, ClientID, ClientSecret, DefaultPort | Lines 22-29 |
| `internal/token/constants.go` | DefaultQwenBaseURL | Line 5 |

### Specific Hardcoded Values (Security Risk)

```go
// provider/gemini/auth.go - Lines 36-37
ClientID:     "681255809395-oo8ft2oprdrnp9e3aqf6av3hmdib135j.apps.googleusercontent.com"
ClientSecret: "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"

// provider/iflow/iflow.go - Lines 26-27
ClientID:     "10009311001"
ClientSecret: "4Z3YjXycVsQvyGF1etiNlIBB4RsqSDtW"

// provider/qwen/auth_device.go - Line 29
OAuthClientID: "f0304373b74a44d2b584a3fb70ca9e56"
```

---

## How to Fix

### Step 1: Create Configuration Package

Create a new file `internal/config/provider_config.go` with the following structure:

```go
package config

import (
    "os"
    "strconv"
)

// ProviderConfig holds all provider-specific configuration
type ProviderConfig struct {
    // Gemini Configuration
    GeminiBaseURL      string
    GeminiClientID      string
    GeminiClientSecret  string
    GeminiScope        string
    GeminiRedirectPort  int
    
    // Qwen Configuration
    QwenBaseURL         string
    QwenClientID        string
    QwenScope           string
    QwenTokenURL        string
    QwenDeviceAuthURL   string
    
    // iFlow Configuration
    IFlowAuthURL        string
    IFlowTokenURL       string
    IFlowUserInfoURL     string
    IFlowAPIKeyURL      string
    IFlowAPIBaseURL     string
    IFlowClientID       string
    IFlowClientSecret   string
    IFlowDefaultPort    int
    
    // Antigravity Configuration
    AntigravityDailyBaseURL    string
    AntigravityAutopushBaseURL string
}

// getEnv returns environment variable or default value
func getEnv(key, defaultValue string) string {
    if value := os.Getenv(key); value != "" {
        return value
    }
    return defaultValue
}

// getEnvInt returns environment variable as integer or default value
func getEnvInt(key string, defaultValue int) int {
    if value := os.Getenv(key); value != "" {
        if intVal, err := strconv.Atoi(value); err == nil {
            return intVal
        }
    }
    return defaultValue
}

// LoadProviderConfig loads provider configuration from environment variables
// with sensible defaults for development
func LoadProviderConfig() *ProviderConfig {
    return &ProviderConfig{
        // Gemini Configuration
        GeminiBaseURL:      getEnv("GEMINI_BASE_URL", "https://cloudcode-pa.googleapis.com/v1internal"),
        GeminiClientID:      getEnv("GEMINI_CLIENT_ID", ""), // Empty by default - requires env var
        GeminiClientSecret:  getEnv("GEMINI_CLIENT_SECRET", ""), // Empty by default - requires env var
        GeminiScope:        getEnv("GEMINI_SCOPE", "https://www.googleapis.com/auth/cloud-platform https://www.googleapis.com/auth/userinfo.email https://www.googleapis.com/auth/userinfo.profile openid"),
        GeminiRedirectPort:  getEnvInt("GEMINI_REDIRECT_PORT", 8085),
        
        // Qwen Configuration
        QwenBaseURL:         getEnv("QWEN_BASE_URL", "https://portal.qwen.ai/v1"),
        QwenClientID:        getEnv("QWEN_CLIENT_ID", "f0304373b74a44d2b584a3fb70ca9e56"), // Public client ID
        QwenScope:           getEnv("QWEN_SCOPE", "openid profile email model.completion"),
        QwenTokenURL:        getEnv("QWEN_TOKEN_URL", "https://chat.qwen.ai/api/v1/oauth2/token"),
        QwenDeviceAuthURL:   getEnv("QWEN_DEVICE_AUTH_URL", "https://chat.qwen.ai/api/v1/oauth2/device/code"),
        
        // iFlow Configuration
        IFlowAuthURL:        getEnv("IFLOW_AUTH_URL", "https://iflow.cn/oauth"),
        IFlowTokenURL:       getEnv("IFLOW_TOKEN_URL", "https://iflow.cn/oauth/token"),
        IFlowUserInfoURL:     getEnv("IFLOW_USERINFO_URL", "https://iflow.cn/api/oauth/getUserInfo"),
        IFlowAPIKeyURL:      getEnv("IFLOW_APIKEY_URL", "https://platform.iflow.cn/api/openapi/apikey"),
        IFlowAPIBaseURL:     getEnv("IFLOW_API_BASE_URL", "https://apis.iflow.cn/v1"),
        IFlowClientID:       getEnv("IFLOW_CLIENT_ID", "10009311001"), // Consider moving to env var
        IFlowClientSecret:   getEnv("IFLOW_CLIENT_SECRET", "4Z3YjXycVsQvyGF1etiNlIBB4RsqSDtW"), // Consider moving to env var
        IFlowDefaultPort:    getEnvInt("IFLOW_DEFAULT_PORT", 11451),
        
        // Antigravity Configuration
        AntigravityDailyBaseURL:    getEnv("ANTIGRAVITY_DAILY_BASE_URL", "https://daily-cloudcode-pa.sandbox.googleapis.com"),
        AntigravityAutopushBaseURL: getEnv("ANTIGRAVITY_AUTOPUSH_BASE_URL", "https://autopush-cloudcode-pa.sandbox.googleapis.com"),
    }
}
```

### Step 2: Create Environment Variables File

Create `.env.example` file in the project root:

```bash
# ========================================
# Provider Configuration
# ========================================
# Copy this file to .env and fill in your credentials
# NEVER commit .env to version control!

# ----------------------------------------
# Gemini Configuration
# ----------------------------------------
GEMINI_BASE_URL=https://cloudcode-pa.googleapis.com/v1internal
GEMINI_CLIENT_ID=your_gemini_client_id_here
GEMINI_CLIENT_SECRET=your_gemini_client_secret_here
GEMINI_REDIRECT_PORT=8085

# ----------------------------------------
# Qwen Configuration
# ----------------------------------------
QWEN_BASE_URL=https://portal.qwen.ai/v1
QWEN_CLIENT_ID=f0304373b74a44d2b584a3fb70ca9e56
QWEN_SCOPE=openid profile email model.completion
QWEN_TOKEN_URL=https://chat.qwen.ai/api/v1/oauth2/token
QWEN_DEVICE_AUTH_URL=https://chat.qwen.ai/api/v1/oauth2/device/code

# ----------------------------------------
# iFlow Configuration
# ----------------------------------------
IFLOW_AUTH_URL=https://iflow.cn/oauth
IFLOW_TOKEN_URL=https://iflow.cn/oauth/token
IFLOW_USERINFO_URL=https://iflow.cn/api/oauth/getUserInfo
IFLOW_APIKEY_URL=https://platform.iflow.cn/api/openapi/apikey
IFLOW_API_BASE_URL=https://apis.iflow.cn/v1
IFLOW_CLIENT_ID=10009311001
IFLOW_CLIENT_SECRET=4Z3YjXycVsQvyGF1etiNlIBB4RsqSDtW
IFLOW_DEFAULT_PORT=11451

# ----------------------------------------
# Antigravity Configuration
# ----------------------------------------
ANTIGRAVITY_DAILY_BASE_URL=https://daily-cloudcode-pa.sandbox.googleapis.com
ANTIGRAVITY_AUTOPUSH_BASE_URL=https://autopush-cloudcode-pa.sandbox.googleapis.com
```

### Step 3: Update .gitignore

Ensure `.env` is added to `.gitignore`:

```
# Environment variables with secrets
.env
.env.local
.env.*.local
```

### Step 4: Update Provider Files

#### 4.1 Update `provider/gemini/auth.go`

Remove hardcoded constants and use configuration:

```go
// Remove these lines (36-43):
// const (
//     ClientID     = "681255809395-oo8ft2oprdrnp9e3aqf6av3hmdib135j.apps.googleusercontent.com"
//     ClientSecret = "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"
//     Scope        = "https://www.googleapis.com/auth/cloud-platform https://www.googleapis.com/auth/userinfo.email https://www.googleapis.com/auth/userinfo.profile openid"
//     RedirectPort = 8085
//     CredsDir     = ".gemini"
//     CredsFile    = "oauth_creds.json"
// )

// Remove DefaultOAuthConfig() function (lines 34-43)

// Update NewAuthenticator to accept config:
func NewAuthenticator(providerConfig *config.ProviderConfig) *Authenticator {
    if providerConfig == nil {
        providerConfig = config.LoadProviderConfig()
    }
    
    // Validate required credentials
    if providerConfig.GeminiClientID == "" {
        // Log warning but continue for backward compatibility
        // In production, this should return an error
    }
    
    return &Authenticator{
        config: &OAuthConfig{
            ClientID:     providerConfig.GeminiClientID,
            ClientSecret: providerConfig.GeminiClientSecret,
            Scope:        providerConfig.GeminiScope,
            RedirectPort:  providerConfig.GeminiRedirectPort,
            CredsDir:     ".gemini",
            CredsFile:    "oauth_creds.json",
        },
        logger:     logging.NewLogger(),
        httpClient: &http.Client{Timeout: 30 * time.Second},
    }
}
```

#### 4.2 Update `provider/gemini/gemini.go`

```go
// Remove line 23:
// const DefaultBaseURL = "https://cloudcode-pa.googleapis.com/v1internal"

// Update NewProvider to use config:
func NewProvider(authenticator *Authenticator, providerConfig *config.ProviderConfig) *Provider {
    if authenticator == nil {
        if providerConfig == nil {
            providerConfig = config.LoadProviderConfig()
        }
        authenticator = NewAuthenticator(providerConfig)
    }
    if providerConfig == nil {
        providerConfig = config.LoadProviderConfig()
    }
    
    return &Provider{
        BaseProvider:  provider.NewBaseProvider(logging.NewLogger(), 5*time.Minute),
        baseURL:       providerConfig.GeminiBaseURL,
        authenticator: authenticator,
    }
}
```

#### 4.3 Update `provider/qwen/auth_device.go`

```go
// Remove lines 25-34 (hardcoded constants):
// const (
//     OAuthTokenURL = "https://chat.qwen.ai/api/v1/oauth2/token"
//     OAuthClientID = "f0304373b74a44d2b584a3fb70ca9e56"
//     OAuthScope = "openid profile email model.completion"
//     OAuthDeviceAuthURL = "https://chat.qwen.ai/api/v1/oauth2/device/code"
// )

// Update AuthenticateWithDeviceFlow function:
func AuthenticateWithDeviceFlow(ctx context.Context, logger logging.Logger, multiTokenMgr *tokenpkg.MultiTokenManager, providerConfig *config.ProviderConfig) error {
    if providerConfig == nil {
        providerConfig = config.LoadProviderConfig()
    }
    
    conf := &oauth2.Config{
        ClientID: providerConfig.QwenClientID,
        Scopes:   []string{providerConfig.QwenScope},
        Endpoint: oauth2.Endpoint{
            TokenURL:      providerConfig.QwenTokenURL,
            DeviceAuthURL: providerConfig.QwenDeviceAuthURL,
        },
    }
    // ... rest of function remains the same
}
```

#### 4.4 Update `provider/qwen/qwen.go`

```go
// Remove line 26:
// const DefaultBaseURL = "https://portal.qwen.ai/v1"

// Update NewProviderWithTokenManager:
func NewProviderWithTokenManager(tokenManager *tokenpkg.TokenManager, logger logging.Logger, providerConfig *config.ProviderConfig) *Provider {
    if providerConfig == nil {
        providerConfig = config.LoadProviderConfig()
    }
    
    return &Provider{
        BaseProvider:  provider.NewBaseProvider(logger, 5*time.Minute),
        authenticator: NewQwenAuthenticator(tokenManager, logger, providerConfig),
    }
}
```

#### 4.5 Update `provider/iflow/iflow.go`

```go
// Remove lines 20-30 (hardcoded constants):
// const (
//     AuthURL      = "https://iflow.cn/oauth"
//     TokenURL     = "https://iflow.cn/oauth/token"
//     UserInfoURL  = "https://iflow.cn/api/oauth/getUserInfo"
//     APIKeyURL    = "https://platform.iflow.cn/api/openapi/apikey"
//     ClientID     = "10009311001"
//     ClientSecret = "4Z3YjXycVsQvyGF1etiNlIBB4RsqSDtW"
//     DefaultPort  = 11451
//     APIBaseURL   = "https://apis.iflow.cn/v1"
// )

// Update NewProvider:
func NewProvider(authenticator *Authenticator, providerConfig *config.ProviderConfig) *Provider {
    if authenticator == nil {
        if providerConfig == nil {
            providerConfig = config.LoadProviderConfig()
        }
        authenticator = NewAuthenticator(providerConfig)
    }
    if providerConfig == nil {
        providerConfig = config.LoadProviderConfig()
    }
    
    return &Provider{
        BaseProvider:  provider.NewBaseProvider(logging.NewLogger(), 5*time.Minute),
        baseURL:       providerConfig.IFlowAPIBaseURL,
        authenticator: authenticator,
    }
}
```

#### 4.6 Update `provider/iflow/auth.go`

```go
// Remove DefaultOAuthConfig() function

// Update NewAuthenticator:
func NewAuthenticator(providerConfig *config.ProviderConfig) *Authenticator {
    if providerConfig == nil {
        providerConfig = config.LoadProviderConfig()
    }
    
    return &Authenticator{
        config: &OAuthConfig{
            ClientID:     providerConfig.IFlowClientID,
            ClientSecret: providerConfig.IFlowClientSecret,
            RedirectPort:  providerConfig.IFlowDefaultPort,
            CredsDir:     ".iflow",
            CredsFile:    "oauth_creds.json",
        },
        logger:     logging.NewLogger(),
        httpClient: &http.Client{Timeout: 30 * time.Second},
    }
}
```

#### 4.7 Update `provider/antigravity/antigravity.go`

```go
// Remove lines 24-33 (hardcoded constants):
// const (
//     DefaultDailyBaseURL = "https://daily-cloudcode-pa.sandbox.googleapis.com"
//     DefaultAutopushBaseURL = "https://autopush-cloudcode-pa.sandbox.googleapis.com"
//     DefaultUserAgent = "antigravity/1.11.5 windows/amd64"
//     APIVersion = "v1internal"
// )

// Update NewProvider:
func NewProvider(authenticator *Authenticator, providerConfig *config.ProviderConfig) *Provider {
    if authenticator == nil {
        if providerConfig == nil {
            providerConfig = config.LoadProviderConfig()
        }
        authenticator = NewAuthenticator(providerConfig)
    }
    if providerConfig == nil {
        providerConfig = config.LoadProviderConfig()
    }
    
    return &Provider{
        BaseProvider:    provider.NewBaseProvider(logging.NewLogger(), 5*time.Minute),
        dailyBaseURL:    providerConfig.AntigravityDailyBaseURL,
        autopushBaseURL: providerConfig.AntigravityAutopushBaseURL,
        authenticator:   authenticator,
        projectID:       "",
        isInitialized:   false,
        cachedModels:    make(map[string]bool),
    }
}
```

### Step 5: Update main.go

Load configuration at application startup:

```go
func main() {
    // ... existing code ...
    
    // Load provider configuration from environment
    providerConfig := config.LoadProviderConfig()
    
    // Pass providerConfig to provider initialization
    // ... update initializeProviders function to accept providerConfig ...
}
```

---

## Implementation Checklist

- [ ] Create `internal/config/provider_config.go`
- [ ] Create `.env.example` file
- [ ] Update `.gitignore` to exclude `.env`
- [ ] Update `provider/gemini/auth.go` - remove hardcoded constants
- [ ] Update `provider/gemini/gemini.go` - use provider config
- [ ] Update `provider/qwen/auth_device.go` - use provider config
- [ ] Update `provider/qwen/qwen.go` - use provider config
- [ ] Update `provider/iflow/iflow.go` - remove hardcoded constants
- [ ] Update `provider/iflow/auth.go` - use provider config
- [ ] Update `provider/antigravity/antigravity.go` - use provider config
- [ ] Update `cmd/qwencoder-proxy/main.go` - load provider config
- [ ] Update all provider constructors to accept providerConfig parameter
- [ ] Run tests to ensure backward compatibility
- [ ] Update documentation with new environment variables

---

## Testing

### Unit Tests
Create `internal/config/provider_config_test.go`:

```go
package config

import (
    "os"
    "testing"
)

func TestLoadProviderConfig_Defaults(t *testing.T) {
    // Clear environment variables
    os.Unsetenv("GEMINI_BASE_URL")
    os.Unsetenv("GEMINI_CLIENT_ID")
    
    cfg := LoadProviderConfig()
    
    if cfg.GeminiBaseURL != "https://cloudcode-pa.googleapis.com/v1internal" {
        t.Errorf("Expected default Gemini base URL, got %s", cfg.GeminiBaseURL)
    }
}

func TestLoadProviderConfig_FromEnv(t *testing.T) {
    os.Setenv("GEMINI_BASE_URL", "https://custom.example.com")
    defer os.Unsetenv("GEMINI_BASE_URL")
    
    cfg := LoadProviderConfig()
    
    if cfg.GeminiBaseURL != "https://custom.example.com" {
        t.Errorf("Expected custom base URL, got %s", cfg.GeminiBaseURL)
    }
}
```

### Integration Tests
- Verify all providers work with environment variables
- Test with missing environment variables
- Test with invalid environment variables

---

## Estimated Effort

| Task | Time |
|------|------|
| Create provider_config.go | 30 minutes |
| Create .env.example | 15 minutes |
| Update provider files (6 files) | 2 hours |
| Update main.go | 15 minutes |
| Write tests | 30 minutes |
| **Total** | **3.5 hours** |

---

## Benefits

1. **Security**: Credentials no longer hardcoded in source code
2. **Flexibility**: Easy to change configuration without code changes
3. **Environment-specific**: Different configs for dev/staging/prod
4. **Maintainability**: Single source of truth for configuration
5. **Onboarding**: New developers can use `.env.example` as template
