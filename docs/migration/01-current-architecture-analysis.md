# Current Architecture Analysis - qwencoder-proxy

## Executive Summary

This document provides a comprehensive technical analysis of the current OAuth and token storage architecture in the qwencoder-proxy project. It details how tokens and metadata are persisted and accessed when the `/v1/chat/completions` API endpoint is invoked.

---

## Table of Contents

1. [Project Overview](#project-overview)
2. [Component Architecture](#component-architecture)
3. [Token Storage Mechanism](#token-storage-mechanism)
4. [OAuth Token Acquisition Workflow](#oauth-token-acquisition-workflow)
5. [/v1/chat/completions Request Flow](#v1chatcompletions-request-flow)
6. [Token Refresh Workflow](#token-refresh-workflow)
7. [Data Persistence Operations](#data-persistence-operations)
8. [Current Limitations](#current-limitations)
9. [Architecture Flow Diagram](#architecture-flow-diagram)

---

## Project Overview

The qwencoder-proxy is a Go-based proxy server that provides OpenAI-compatible API endpoints for multiple AI providers:
- **Gemini** (Google Cloud Platform)
- **Qwen** (Alibaba Cloud)
- **iFlow** (Custom OAuth2 provider)
- **Kiro** (Custom OAuth2 provider)
- **Antigravity** (Custom OAuth2 provider)

### Key Technologies
- **Language**: Go 1.21+
- **Storage**: File-based JSON storage
- **Authentication**: OAuth2 with PKCE
- **Concurrency**: goroutines with sync.RWMutex
- **HTTP**: Standard library net/http
- **JSON**: encoding/json

---

## Component Architecture

### 1. Main Entry Point ([`cmd/qwencoder-proxy/main.go`](../cmd/qwencoder-proxy/main.go))

**Responsibilities:**
- Initialize [`MultiTokenManager`](../internal/token/multi_token_manager.go) for token management
- Start refresh schedulers for automatic token renewal
- Create and register providers with token manager injection
- Set up HTTP server with REST API and proxy routes

**Key Initialization Flow:**
```
main()
  ├─→ Load configuration from environment
  ├─→ Create MultiTokenManager
  ├─→ Initialize MultiTokenManager
  ├─→ Start MultiTokenManager (refresh schedulers)
  ├─→ Create providers with token manager injection
  ├─→ Create REST API server
  ├─→ Register REST API routes
  ├─→ Register proxy routes
  └─→ Start HTTP server
```

### 2. Token Management Layer ([`internal/token/`](../internal/token/))

#### 2.1 MultiTokenManager ([`multi_token_manager.go`](../internal/token/multi_token_manager.go))

**Purpose:** Central coordinator for all token operations across all providers.

**Components Managed:**
- `stores`: map[string]*MultiTokenStore - Token stores per provider
- `managers`: map[string]*TokenManager - Token managers per provider
- `refreshers`: map[string]ProviderRefresh - Token refresh implementations
- `schedulers`: map[string]*RefreshScheduler - Automatic refresh schedulers
- `extractors`: map[string]EmailExtractor - Email extraction from token responses
- `healthTrackers`: map[string]*HealthTracker - Token health tracking
- `proxyHealthTrackers`: map[string]*ProxyHealthTracker - Proxy health tracking
- `emailManager`: *EmailExtractionManager - Coordinator for email extractors

**Key Methods:**
- `Initialize()` - Sets up email extractors
- `Start()` - Starts all refresh schedulers
- `Stop()` - Stops all schedulers
- `GetTokenStore(providerID)` - Returns or creates token store for provider
- `GetTokenManager(providerID)` - Returns or creates token manager for provider
- `RegisterRefresher(providerID, refresher)` - Registers token refresher
- `RegisterProviderConfig(config)` - Registers provider OAuth configuration

#### 2.2 MultiTokenStore ([`multi_token_store.go`](../internal/token/multi_token_store.go))

**Purpose:** File-based token storage per provider.

**Data Structure:**
```go
type MultiTokenStore struct {
    ProviderID  string          // Provider identifier (e.g., "gemini-cli")
    Version     int             // Schema version
    Tokens      []ProviderToken  // In-memory cache of tokens
    Settings    StoreSettings   // Provider-specific settings
    mu          sync.RWMutex    // Thread safety
    providerDir string          // Directory path (e.g., ".credentials/gemini")
    logger      logging.Logger
}
```

**Storage Location:**
- Directory: `.credentials/{providerID}/`
- Token files: `.credentials/{providerID}/{sanitized_email}.json`
- Settings file: `.credentials/{providerID}/settings.json`

**File Locking:**
- Uses `github.com/gofrs/flock` for file-level locking
- Ensures concurrent access safety

**Key Methods:**
- `Load()` - Loads all tokens from JSON files
- `Save()` - Persists all tokens to JSON files
- `AddToken(token)` - Adds a new token
- `UpdateToken(tokenID, updateFunc)` - Updates existing token
- `RemoveToken(tokenID)` - Removes a token
- `ListTokens()` - Returns all tokens
- `GetSettings()` - Returns provider settings

#### 2.3 TokenManager ([`token_selection.go`](../internal/token/token_selection.go))

**Purpose:** Token selection and management for a provider.

**Data Structure:**
```go
type TokenManager struct {
    store              *MultiTokenStore
    strategy           SelectionStrategy
    mu                 sync.RWMutex
    logger             logging.Logger
    clientFactory      ProxyClientFactory
    proxyHealthTracker *ProxyHealthTracker
}
```

**Selection Strategies:**
1. **RandomSelectionStrategy** - Randomly selects from valid tokens
2. **RoundRobinSelectionStrategy** - Rotates through valid tokens
3. **LeastUsedSelectionStrategy** - Selects least recently used token

**Key Methods:**
- `SelectToken()` - Selects a valid token based on strategy
- `GetToken()` - Gets a specific token by ID
- `UpdateToken()` - Updates token metadata
- `MarkHealthy(tokenID)` - Marks token as healthy
- `MarkUnhealthy(tokenID, error)` - Marks token as unhealthy

#### 2.4 ProviderToken ([`multi_token_store.go:18-36`](../internal/token/multi_token_store.go#L18-L36))

**Purpose:** Represents a single stored OAuth token with metadata.

**Data Structure:**
```go
type ProviderToken struct {
    ID               string       // UUID (unique identifier)
    AccessToken      string       // OAuth access token
    RefreshToken     string       // OAuth refresh token
    TokenType        string       // Token type (usually "Bearer")
    ExpiryDate       int64        // Expiry timestamp (Unix milliseconds)
    Email            string       // User email (extracted from token response)
    ResourceURL      string       // Resource URL (for Qwen)
    Scope            string       // OAuth scope (for Gemini)
    APIKey           string       // API key (for IFlow)
    Healthy          bool         // Token health status
    HealthScore      float64      // Health score (0.0-1.0)
    LastUsed         int64        // Last usage timestamp
    CreatedAt        int64        // Token creation timestamp
    ErrorCount       int          // Consecutive error count
    LastError        string       // Last error message
    Proxy            *ProxyConfig // Proxy configuration
    ProxyHealthScore float64      // Proxy health score (0.0-1.0)
}
```

#### 2.5 StoreSettings ([`multi_token_store.go:38-43`](../internal/token/multi_token_store.go#L38-L43)

**Purpose:** Provider-specific settings.

**Data Structure:**
```go
type StoreSettings struct {
    SelectionStrategy string // "random", "round_robin", "least_used"
    RefreshBufferSec  int    // Seconds before expiry to refresh
    MaxErrorCount     int    // Max errors before marking unhealthy
}
```

### 3. REST API Layer ([`restapi/rest_api.go`](../restapi/rest_api.go))

**Purpose:** Handles OAuth2 authentication flows and token management endpoints.

**Key Endpoints:**

| Endpoint | Method | Purpose |
|----------|--------|---------|
| `/api/auth/start` | POST | Initiates OAuth flow, returns auth URL |
| `/api/callback` | GET | Handles OAuth callback, exchanges code for tokens |
| `/api/token/{provider}` | GET | Gets current access token for provider |
| `/api/token/{provider}` | DELETE | Deletes stored token for provider |
| `/api/token/{provider}/refresh` | POST | Manually refreshes access token |
| `/api/credentials` | GET | Lists all stored credentials |

**OAuth Callback Flow:**
```
/api/callback?code=xxx&state=yyy
  ├─→ Validate state
  ├─→ Exchange code for tokens
  ├─→ Extract email from token response
  ├─→ Create ProviderToken
  ├─→ Save to MultiTokenStore
  └─→ Return success to user
```

### 4. Proxy Handler ([`proxy/openai_handler.go`](../proxy/openai_handler.go))

**Purpose:** Handles OpenAI-compatible API requests and routes them to appropriate providers.

**Key Endpoint:**
- `/v1/chat/completions` - OpenAI-compatible chat completions

**Request Processing Flow:**
```
POST /v1/chat/completions
  ├─→ Parse request body
  ├─→ Determine provider from model
  ├─→ Get provider from factory
  ├─→ Get converter for provider protocol
  ├─→ Convert request to provider format
  ├─→ Get token from provider authenticator
  ├─→ Forward request to provider
  ├─→ Convert response to OpenAI format
  └─→ Return response to client
```

---

## Token Storage Mechanism

### File-Based Storage Structure

```
.credentials/
├── gemini/
│   ├── user1_example_com.json
│   ├── user2_example_com.json
│   └── settings.json
├── qwen/
│   ├── user3_example_com.json
│   └── settings.json
├── iflow/
│   └── settings.json
├── kiro/
│   └── settings.json
└── antigravity/
    └── settings.json
```

### Token File Format

**Location:** `.credentials/{providerID}/{sanitized_email}.json`

**Example:**
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "access_token": "ya29.a0AfH6SMB...",
  "refresh_token": "1//0g...",
  "token_type": "Bearer",
  "expiry_date": 1708768800000,
  "email": "user@example.com",
  "scope": "https://www.googleapis.com/auth/cloud-platform",
  "resource_url": "",
  "api_key": "",
  "healthy": true,
  "health_score": 1.0,
  "last_used": 1708768800000,
  "created_at": 1708768800000,
  "error_count": 0,
  "last_error": "",
  "proxy": {
    "host": "proxy.example.com",
    "port": 8080,
    "type": "http",
    "username": "user",
    "password": "pass"
  },
  "proxy_health_score": 1.0
}
```

### Settings File Format

**Location:** `.credentials/{providerID}/settings.json`

**Example:**
```json
{
  "selection_strategy": "random",
  "refresh_buffer_sec": 1800,
  "max_error_count": 3
}
```

### Storage Operations

#### Load Operation
```go
func (mts *MultiTokenStore) Load() error {
    mts.mu.Lock()
    defer mts.mu.Unlock()

    // Ensure directory exists
    if err := mts.ensureProviderDirectory(); err != nil {
        return err
    }

    // Read all JSON files
    entries, err := os.ReadDir(mts.providerDir)
    if err != nil {
        return err
    }

    // Load each token file
    for _, entry := range entries {
        if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
            continue
        }
        if entry.Name() == "settings.json" {
            continue
        }

        // Read and unmarshal
        data, _ := os.ReadFile(filepath.Join(mts.providerDir, entry.Name()))
        var token ProviderToken
        json.Unmarshal(data, &token)
        mts.Tokens = append(mts.Tokens, token)
    }

    // Load settings
    settingsData, _ := os.ReadFile(filepath.Join(mts.providerDir, "settings.json"))
    json.Unmarshal(settingsData, &mts.Settings)

    return nil
}
```

#### Save Operation
```go
func (mts *MultiTokenStore) Save() error {
    mts.mu.Lock()
    defer mts.mu.Unlock()

    // Ensure directory exists
    mts.ensureProviderDirectory()

    // Save each token
    for _, token := range mts.Tokens {
        // Sanitize email for filename
        filename := sanitizeEmail(token.Email) + ".json"
        filepath := filepath.Join(mts.providerDir, filename)

        // Marshal to JSON
        data, _ := json.MarshalIndent(token, "", "  ")

        // Write with atomic rename
        tempPath := filepath + ".tmp"
        os.WriteFile(tempPath, data, 0600)
        os.Rename(tempPath, filepath)
    }

    // Save settings
    settingsPath := filepath.Join(mts.providerDir, "settings.json")
    settingsData, _ := json.MarshalIndent(mts.Settings, "", "  ")
    os.WriteFile(settingsPath, settingsData, 0600)

    return nil
}
```

---

## OAuth Token Acquisition Workflow

### Detailed Flow Diagram

```
┌─────────────────────────────────────────────────────────────────────────┐
│                      User Dashboard                               │
│                  (Web Interface)                                  │
└────────────────────────────┬────────────────────────────────────────┘
                             │
                             │ 1. POST /api/auth/start
                             │    {provider: "gemini-cli"}
                             ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                  REST API Server                                    │
│                  (restapi.Server)                                    │
├─────────────────────────────────────────────────────────────────────────┤
│ 1. Generate state parameter (random string)                         │
│ 2. Generate PKCE code_verifier (random 128 bytes)                │
│ 3. Generate code_challenge (SHA256 of code_verifier)              │
│ 4. Store state in StateManager with TTL                              │
│ 5. Build OAuth authorization URL with:                                  │
│    - client_id                                                       │
│    - redirect_uri (callback URL)                                       │
│    - scope                                                           │
│    - response_type=code                                             │
│    - state                                                           │
│    - code_challenge                                                   │
│    - code_challenge_method=S256                                        │
└────────────────────────────┬────────────────────────────────────────┘
                             │
                             │ 2. Return auth_url
                             ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                      User Browser                                     │
│                  (OAuth Provider Login)                                │
└────────────────────────────┬────────────────────────────────────────┘
                             │
                             │ 3. User authenticates
                             │    and grants permissions
                             ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                  OAuth Provider                                     │
│         (Google, Qwen, iFlow, Kiro)                              │
├─────────────────────────────────────────────────────────────────────────┤
│ 1. User logs in and consents to permissions                           │
│ 2. Provider generates authorization code                                │
│ 3. Provider redirects to:                                             │
│    {callback_base_url}/api/callback?code=xxx&state=yyy            │
└────────────────────────────┬────────────────────────────────────────┘
                             │
                             │ 4. GET /api/callback?code=xxx&state=yyy
                             ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                  REST API Server                                    │
│                  (restapi.Server)                                    │
├─────────────────────────────────────────────────────────────────────────┤
│ 1. Validate state from StateManager                                 │
│    - Check if state exists                                            │
│    - Check if state is expired                                         │
│    - Delete state after validation                                      │
│ 2. Exchange authorization code for tokens:                               │
│    POST to provider token endpoint                                      │
│    - grant_type=authorization_code                                     │
│    - code=xxx                                                         │
│    - redirect_uri                                                      │
│    - client_id                                                        │
│    - client_secret                                                    │
│    - code_verifier                                                    │
│ 3. Parse token response:                                               │
│    - access_token                                                     │
│    - refresh_token                                                    │
│    - token_type                                                       │
│    - expires_in (convert to expiry_date)                               │
│ 4. Extract email from token response:                                  │
│    - Call EmailExtractionManager.ExtractEmail()                        │
│    - Provider-specific extractor handles extraction                           │
│ 5. Create ProviderToken:                                              │
│    - Generate UUID for ID                                              │
│    - Set all token fields                                             │
│    - Set healthy=true                                                   │
│    - Set health_score=1.0                                             │
│    - Set last_used=0                                                   │
│    - Set created_at=now                                                │
│    - Set error_count=0                                                 │
│ 6. Save to MultiTokenStore:                                            │
│    - Call store.AddToken(token)                                         │
│    - Store writes to .credentials/{providerID}/{email}.json          │
│ 7. Return success response to user                                     │
└────────────────────────────┬────────────────────────────────────────┘
                             │
                             │ 5. Success response
                             ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                      User Dashboard                               │
│                  (Token Added Successfully)                             │
└─────────────────────────────────────────────────────────────────────────┘
```

### Code Flow: OAuth Callback Handler

```go
// restapi/rest_api.go - handleCallback()

func (s *Server) handleCallback(w http.ResponseWriter, r *http.Request) {
    // 1. Extract parameters
    code := r.URL.Query().Get("code")
    state := r.URL.Query().Get("state")
    providerID := r.URL.Query().Get("provider")

    // 2. Validate state
    if !s.stateManager.Validate(state) {
        http.Error(w, "Invalid or expired state", http.StatusBadRequest)
        return
    }

    // 3. Get provider config
    config := s.getProviderConfig(providerID)

    // 4. Exchange code for tokens
    tokenResponse := s.exchangeCodeForTokens(code, config)

    // 5. Extract email
    email := s.multiTokenManager.emailManager.ExtractEmail(
        providerID,
        tokenResponse,
    )

    // 6. Create ProviderToken
    token := tokpkg.ProviderToken{
        ID:          uuid.New().String(),
        AccessToken:  tokenResponse.AccessToken,
        RefreshToken: tokenResponse.RefreshToken,
        TokenType:    tokenResponse.TokenType,
        ExpiryDate:   time.Now().Add(time.Duration(tokenResponse.ExpiresIn) * time.Second).UnixMilli(),
        Email:        email,
        Healthy:      true,
        HealthScore:  1.0,
        LastUsed:     0,
        CreatedAt:    time.Now().UnixMilli(),
        ErrorCount:   0,
    }

    // 7. Save to store
    store, _ := s.multiTokenManager.GetTokenStore(providerID)
    store.AddToken(token)

    // 8. Return success
    json.NewEncoder(w).Encode(map[string]string{"status": "success"})
}
```

---

## /v1/chat/completions Request Flow

### Detailed Flow Diagram

```
┌─────────────────────────────────────────────────────────────────────────┐
│                   Client Application                               │
│              (curl, Python, JavaScript, etc.)                    │
└────────────────────────────┬────────────────────────────────────────┘
                             │
                             │ 1. POST /v1/chat/completions
                             │    {
                             │      "model": "gemini-1.5-pro",
                             │      "messages": [...]
                             │    }
                             ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                  HTTP Server (mux)                                   │
│                  (net/http.ServeMux)                                │
├─────────────────────────────────────────────────────────────────────────┤
│ 1. Apply middleware:                                               │
│    - CORS headers                                                    │
│    - Logging                                                         │
└────────────────────────────┬────────────────────────────────────────┘
                             │
                             │ 2. Route to OpenAIHandler
                             ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                  OpenAIHandler                                     │
│                  (proxy.OpenAIHandler)                                │
├─────────────────────────────────────────────────────────────────────────┤
│ 1. Parse request body:                                             │
│    - Extract model name                                                │
│    - Extract messages                                                  │
│    - Extract stream flag                                               │
│ 2. Determine provider from model:                                      │
│    - Check if model starts with "gemini-" → gemini-cli provider        │
│    - Check if model starts with "qwen-" → qwen provider             │
│    - Check if model starts with "kiro-" → kiro provider             │
│    - Check if model starts with "iflow-" → iflow provider           │
│    - Check if model starts with "antigravity-" → antigravity provider│
│    - Default to first available provider                                 │
└────────────────────────────┬────────────────────────────────────────┘
                             │
                             │ 3. Get provider
                             ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                  Provider Factory                                   │
│                  (provider.Factory)                                    │
├─────────────────────────────────────────────────────────────────────────┤
│ 1. Look up provider by type or model                                │
│ 2. Return provider instance                                           │
└────────────────────────────┬────────────────────────────────────────┘
                             │
                             │ 4. Get provider
                             ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                  Provider Instance                                  │
│         (gemini.Provider, qwen.Provider, etc.)                    │
├─────────────────────────────────────────────────────────────────────────┤
│ 1. Get authenticator from provider                                    │
│ 2. Authenticator.GetToken() is called                                  │
└────────────────────────────┬────────────────────────────────────────┘
                             │
                             │ 5. GetToken()
                             ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                  TokenManager                                      │
│                  (token.TokenManager)                                   │
├─────────────────────────────────────────────────────────────────────────┤
│ 1. Call store.ListTokens() to get all tokens for provider          │
│ 2. Filter valid tokens:                                              │
│    - healthy == true                                                  │
│    - expiry_date > now + refresh_buffer_sec                            │
│    - error_count < max_error_count                                    │
│ 3. Apply selection strategy:                                           │
│    - Random: Pick random valid token                                   │
│    - Round Robin: Pick next valid token in rotation                    │
│    - Least Used: Pick token with lowest last_used timestamp          │
│ 4. Update selected token:                                             │
│    - Set last_used = now                                              │
│    - Call store.UpdateToken()                                           │
│ 5. Return access token                                                │
└────────────────────────────┬────────────────────────────────────────┘
                             │
                             │ 6. Access token returned
                             ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                  Provider Authenticator                             │
│         (gemini.Authenticator, qwen.Authenticator, etc.)             │
├─────────────────────────────────────────────────────────────────────────┤
│ 1. Store access token for use in API request                          │
│ 2. Return token to caller                                             │
└────────────────────────────┬────────────────────────────────────────┘
                             │
                             │ 7. Get converter
                             ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                  Converter Factory                                  │
│                  (converter.Factory)                                    │
├─────────────────────────────────────────────────────────────────────────┤
│ 1. Get converter for provider protocol                                │
│ 2. Return converter instance                                           │
└────────────────────────────┬────────────────────────────────────────┘
                             │
                             │ 8. Convert request
                             ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                  Converter                                         │
│         (gemini.Converter, qwen.Converter, etc.)                   │
├─────────────────────────────────────────────────────────────────────────┤
│ 1. Convert OpenAI request format to provider format                   │
│    - Map message structure                                            │
│    - Map parameters (temperature, max_tokens, etc.)                   │
└────────────────────────────┬────────────────────────────────────────┘
                             │
                             │ 9. Provider request
                             ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                  Provider API                                       │
│         (Google, Qwen, iFlow, Kiro APIs)                        │
├─────────────────────────────────────────────────────────────────────────┤
│ 1. Process chat completion request                                     │
│ 2. Return response in provider format                                   │
└────────────────────────────┬────────────────────────────────────────┘
                             │
                             │ 10. Provider response
                             ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                  Converter                                         │
│         (gemini.Converter, qwen.Converter, etc.)                   │
├─────────────────────────────────────────────────────────────────────────┤
│ 1. Convert provider response to OpenAI format                           │
│    - Map response structure                                          │
│    - Map choices/delta                                             │
│    - Map usage information                                            │
└────────────────────────────┬────────────────────────────────────────┘
                             │
                             │ 11. Update health tracking
                             ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                  HealthTracker                                     │
│         (token.HealthTracker)                                        │
├─────────────────────────────────────────────────────────────────────────┤
│ 1. If request successful:                                            │
│    - Mark token as healthy                                             │
│    - Set health_score = 1.0                                         │
│    - Reset error_count = 0                                            │
│ 2. If request failed:                                                │
│    - Increment error_count                                              │
│    - Set last_error                                                    │
│    - If error_count >= max_error_count:                                  │
│      - Set healthy = false                                               │
│      - Set health_score = 0.0                                         │
│ 3. Call store.UpdateToken() to persist changes                         │
└────────────────────────────┬────────────────────────────────────────┘
                             │
                             │ 12. Return response
                             ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                  HTTP Server (mux)                                   │
│                  (net/http.ServeMux)                                │
├─────────────────────────────────────────────────────────────────────────┤
│ 1. Set Content-Type: application/json                                │
│ 2. Write response body                                               │
└────────────────────────────┬────────────────────────────────────────┘
                             │
                             │ 13. HTTP response
                             ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                   Client Application                               │
│              (curl, Python, JavaScript, etc.)                    │
└─────────────────────────────────────────────────────────────────────────┘
```

### Code Flow: Token Selection

```go
// internal/token/token_selection.go - TokenManager.SelectToken()

func (tm *TokenManager) SelectToken() (*ProviderToken, error) {
    tm.mu.Lock()
    defer tm.mu.Unlock()

    // 1. Get all tokens from store
    tokens := tm.store.ListTokens()

    // 2. Filter valid tokens
    validTokens := filterValidTokens(tokens, tm.store.Settings)
    if len(validTokens) == 0 {
        return nil, ErrNoValidTokens
    }

    // 3. Apply selection strategy
    selectedToken, err := tm.strategy.SelectToken(validTokens)
    if err != nil {
        return nil, err
    }

    // 4. Update last_used timestamp
    now := time.Now().UnixMilli()
    selectedToken.LastUsed = now

    // 5. Persist update
    if err := tm.store.UpdateToken(selectedToken.ID, func(t *ProviderToken) {
        t.LastUsed = now
    }); err != nil {
        tm.logger.WarnLog("Failed to update token last_used: %v", err)
    }

    return selectedToken, nil
}

func filterValidTokens(tokens []ProviderToken, settings StoreSettings) []ProviderToken {
    var valid []ProviderToken
    now := time.Now().UnixMilli()
    expiryThreshold := now + int64(settings.RefreshBufferSec*1000)

    for _, token := range tokens {
        // Check health
        if !token.Healthy {
            continue
        }

        // Check expiry
        if token.ExpiryDate < expiryThreshold {
            continue
        }

        // Check error count
        if token.ErrorCount >= settings.MaxErrorCount {
            continue
        }

        valid = append(valid, token)
    }

    return valid
}
```

---

## Token Refresh Workflow

### Detailed Flow Diagram

```
┌─────────────────────────────────────────────────────────────────────────┐
│                  RefreshScheduler                                   │
│         (token.RefreshScheduler)                                    │
├─────────────────────────────────────────────────────────────────────────┤
│ 1. Started by MultiTokenManager.Start()                              │
│ 2. Runs periodic check every 30 seconds                                │
└────────────────────────────┬────────────────────────────────────────┘
                             │
                             │ 1. Periodic check
                             ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                  RefreshScheduler Check                              │
├─────────────────────────────────────────────────────────────────────────┤
│ 1. For each token in store:                                          │
│    a. Calculate time until expiry:                                       │
│       time_to_expiry = expiry_date - now                                 │
│    b. If time_to_expiry < refresh_buffer_sec:                            │
│       - Schedule refresh for this token                                    │
│ 2. For each scheduled refresh:                                         │
│    a. Add to request queue                                              │
└────────────────────────────┬────────────────────────────────────────┘
                             │
                             │ 2. Refresh request queued
                             ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                  RefreshCoordinator                                │
│         (token.RefreshCoordinator)                                   │
├─────────────────────────────────────────────────────────────────────────┤
│ 1. Manages request queue (channel)                                   │
│ 2. Worker goroutines pick up requests:                                  │
│    - Max 3 concurrent workers                                          │
│    - Each worker:                                                       │
│      a. Receive request from queue                                        │
│      b. Call ProviderRefresh.RefreshToken()                              │
│      c. Update token in store                                           │
│      d. Mark token as healthy                                           │
└────────────────────────────┬────────────────────────────────────────┘
                             │
                             │ 3. RefreshToken() called
                             ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                  ProviderRefresh                                    │
│         (provider-specific refresh implementations)                    │
├─────────────────────────────────────────────────────────────────────────┤
│ 1. Build refresh request:                                             │
│    - grant_type=refresh_token                                         │
│    - refresh_token=xxx                                                 │
│    - client_id                                                        │
│    - client_secret                                                    │
│ 2. POST to provider token endpoint                                    │
│ 3. Parse response:                                                    │
│    - access_token                                                       │
│    - refresh_token (may be new)                                         │
│    - expires_in (convert to expiry_date)                               │
│ 4. Return updated token data                                           │
└────────────────────────────┬────────────────────────────────────────┘
                             │
                             │ 4. Updated token returned
                             ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                  MultiTokenStore                                   │
│         (token.MultiTokenStore)                                    │
├─────────────────────────────────────────────────────────────────────────┤
│ 1. UpdateToken(tokenID, updateFunc):                                   │
│    a. Get current token from in-memory cache                            │
│    b. Apply update function to token                                    │
│    c. Marshal token to JSON                                             │
│    d. Write to temp file                                               │
│    e. Atomic rename to final path                                        │
│    f. Use file lock for concurrency safety                               │
│ 2. Update in-memory cache                                              │
└────────────────────────────┬────────────────────────────────────────┘
                             │
                             │ 5. Token updated
                             ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                  HealthTracker                                     │
│         (token.HealthTracker)                                        │
├─────────────────────────────────────────────────────────────────────────┤
│ 1. Mark token as healthy                                               │
│    - Set healthy = true                                                │
│    - Set health_score = 1.0                                            │
│    - Reset error_count = 0                                               │
│ 2. Persist changes to store                                           │
└─────────────────────────────────────────────────────────────────────────┘
```

### Code Flow: Token Refresh

```go
// internal/token/token_refresh.go - RefreshScheduler

func (rs *RefreshScheduler) checkAndScheduleRefreshes() {
    now := time.Now().UnixMilli()

    // Get all tokens
    tokens := rs.store.ListTokens()
    settings := rs.store.GetSettings()
    bufferMs := int64(settings.RefreshBufferSec * 1000)

    for _, token := range tokens {
        // Calculate time until expiry
        timeToExpiry := token.ExpiryDate - now

        // Schedule refresh if expiring soon
        if timeToExpiry < bufferMs {
            rs.scheduleRefresh(token.ID)
        }
    }
}

// Worker goroutine
func (rs *RefreshScheduler) worker() {
    for {
        select {
        case <-rs.ctx.Done():
            return
        case req := <-rs.requestQueue:
            // Refresh token
            newToken, err := rs.refresher.RefreshToken(req.tokenID)
            if err != nil {
                // Mark unhealthy
                rs.healthTracker.MarkUnhealthy(req.tokenID, err.Error())
                continue
            }

            // Update token in store
            rs.store.UpdateToken(req.tokenID, func(t *ProviderToken) {
                t.AccessToken = newToken.AccessToken
                t.RefreshToken = newToken.RefreshToken
                t.ExpiryDate = newToken.ExpiryDate
            })

            // Mark healthy
            rs.healthTracker.MarkHealthy(req.tokenID)
        }
    }
}
```

---

## Data Persistence Operations

### Load Operation Detail

```
MultiTokenStore.Load()
  │
  ├─→ Acquire write lock (sync.RWMutex.Lock())
  │
  ├─→ Ensure provider directory exists
  │    - Create .credentials/{providerID}/ if not exists
  │    - Set permissions to 0700
  │
  ├─→ Check if directory exists
  │    ├─→ If NO: Initialize empty store
  │    │    - Set Tokens = []ProviderToken{}
  │    │    - Set default Settings
  │    │    - Release lock
  │    │    - Return nil
  │    │
  │    └─→ If YES: Continue to load
  │
  ├─→ Read directory entries
  │    - os.ReadDir(mts.providerDir)
  │
  ├─→ For each entry:
  │    ├─→ Skip directories
  │    ├─→ Skip non-.json files
  │    ├─→ Skip settings.json
  │    │
  │    ├─→ Read file: os.ReadFile(filepath)
  │    │
  │    ├─→ Unmarshal JSON: json.Unmarshal(data, &token)
  │    │
  │    └─→ Append to Tokens slice
  │
  ├─→ Load settings.json
  │    - Read file
  │    - Unmarshal to StoreSettings
  │
  ├─→ Set Version = StoreVersion
  │
  ├─→ Release lock
  │
  └─→ Return nil
```

### Save Operation Detail

```
MultiTokenStore.Save()
  │
  ├─→ Acquire write lock (sync.RWMutex.Lock())
  │
  ├─→ Ensure provider directory exists
  │
  ├─→ For each token in Tokens:
  │    │
  │    ├─→ Sanitize email for filename
  │    │    - Replace @ with _
  │    │    - Replace . with _
  │    │
  │    ├─→ Marshal token to JSON
  │    │    - json.MarshalIndent(token, "", "  ")
  │    │
  │    ├─→ Create temp file path
  │    │    - filepath + ".tmp"
  │    │
  │    ├─→ Write to temp file
  │    │    - os.WriteFile(tempPath, data, 0600)
  │    │
  │    ├─→ Acquire file lock (flock)
  │    │    - lock := flock.New(filepath)
  │    │    - lock.Lock()
  │    │
  │    ├─→ Atomic rename
  │    │    - os.Rename(tempPath, filepath)
  │    │
  │    └─→ Release file lock
  │        - lock.Unlock()
  │
  ├─→ Save settings.json
  │    - Marshal StoreSettings
  │    - Write to file
  │
  ├─→ Release lock
  │
  └─→ Return nil
```

### File Locking Mechanism

```go
// Using github.com/gofrs/flock for file-level locking

func (mts *MultiTokenStore) saveTokenToFile(token ProviderToken) error {
    filename := sanitizeEmail(token.Email) + ".json"
    filepath := filepath.Join(mts.providerDir, filename)

    // Create file lock
    lock := flock.New(filepath)
    if err := lock.Lock(); err != nil {
        return fmt.Errorf("failed to acquire lock: %w", err)
    }
    defer lock.Unlock()

    // Write to temp file
    tempPath := filepath + ".tmp"
    data, _ := json.MarshalIndent(token, "", "  ")
    if err := os.WriteFile(tempPath, data, 0600); err != nil {
        return err
    }

    // Atomic rename
    return os.Rename(tempPath, filepath)
}
```

---

## Current Limitations

### 1. Performance Issues

| Issue | Impact |
|-------|--------|
| **File I/O overhead** | Each token operation requires reading/writing JSON files |
| **No caching** | Tokens are re-read from disk on every request |
| **No prepared statements** | JSON marshaling/unmarshaling on every operation |
| **Synchronous I/O** | Blocking file operations |

### 2. Concurrency Issues

| Issue | Impact |
|-------|--------|
| **File locking required** | Concurrent access must wait for file locks |
| **Global lock** | All operations on a provider share same lock |
| **No connection pooling** | Each operation opens/closes files |

### 3. Scalability Issues

| Issue | Impact |
|-------|--------|
| **File system limits** | Limited by OS file descriptor limits |
| **Directory scanning** | Loading all tokens requires scanning directory |
| **No indexing** | Cannot efficiently query/filter tokens |

### 4. Data Integrity Issues

| Issue | Impact |
|-------|--------|
| **No transactions** | Multi-token operations are not atomic |
| **Partial writes possible** | System crash during write can corrupt data |
| **No rollback** | Failed operations cannot be undone |

### 5. Operational Issues

| Issue | Impact |
|-------|--------|
| **Complex backup** | Must backup entire directory structure |
| **Difficult migration** | Moving tokens between environments is complex |
| **No query capabilities** | Cannot run complex queries on tokens |
| **No analytics** | Cannot analyze token usage patterns |

---

## Architecture Flow Diagram

### Complete System Architecture

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                           Client Application                               │
│                    (Dashboard / API Consumer)                               │
└────────────────────────────┬────────────────────────────────────────────────┘
                             │
                             │ HTTP Request
                             ▼
┌─────────────────────────────────────────────────────────────────────────────────┐
│                         HTTP Server (mux)                                   │
│  ┌──────────────────────────────────────────────────────────────────────────┐  │
│  │                      Middleware Layer                                  │  │
│  │  - CORS                                                             │  │
│  │  - Logging                                                          │  │
│  └──────────────────────────────────────────────────────────────────────────┘  │
│                                                                          │
│  ┌──────────────────────┐  ┌──────────────────────────────────────────┐   │
│  │  REST API Routes    │  │  Proxy Routes (OpenAI-compatible)      │   │
│  │  /api/auth/start    │  │  /v1/chat/completions                │   │
│  │  /api/callback      │  │  /v1/models                          │   │
│  │  /api/token/*       │  │  /gemini/v1/chat/completions         │   │
│  │  /api/credentials   │  │  /qwen/v1/chat/completions           │   │
│  └──────────┬───────────┘  └──────────────────────┬───────────────────┘   │
└─────────────┼───────────────────────────────────┼───────────────────────────┘
              │                                   │
              │                                   │
              ▼                                   ▼
┌─────────────────────────┐         ┌─────────────────────────────────────────┐
│  REST API Server       │         │  OpenAIHandler                       │
│  (restapi.Server)     │         │  (proxy.OpenAIHandler)              │
├─────────────────────────┤         ├─────────────────────────────────────────┤
│ - handleAuthStart()    │         │ - handleChatCompletions()           │
│ - handleCallback()     │         │ - handleListModels()                │
│ - handleToken()       │         │ - Determine provider from model       │
│ - handleCredentials()  │         │ - Get provider from factory          │
└──────────┬────────────┘         └──────────────┬────────────────────┘
           │                                    │
           │                                    │
           ▼                                    ▼
┌─────────────────────────────────────────────────────────────────────────────────┐
│                     MultiTokenManager                                      │
│  ┌────────────────────────────────────────────────────────────────────────┐  │
│  │  Token Stores (per provider)                                       │  │
│  │  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐  │  │
│  │  │ gemini store     │  │ qwen store      │  │ iflow store     │  │  │
│  │  │ MultiTokenStore │  │ MultiTokenStore │  │ MultiTokenStore │  │  │
│  │  └────────┬────────┘  └────────┬────────┘  └────────┬────────┘  │  │
│  └───────────┼───────────────────┼───────────────────┼────────────────┘  │
│              │                   │                   │                    │
│  ┌───────────┼───────────────────┼───────────────────┼────────────────┐  │
│  │ Token     │                   │                   │                    │  │
│  │ Managers │                   │                   │                    │  │
│  │ ┌─────────▼────────┐ ┌──────▼─────────┐ ┌──────▼─────────┐   │  │
│  │ │ gemini manager   │ │ qwen manager   │ │ iflow manager   │   │  │
│  │ │ TokenManager     │ │ TokenManager   │ │ TokenManager   │   │  │
│  │ └─────────────────┘ └───────────────┘ └───────────────┘   │  │
│  └──────────────────────────────────────────────────────────────────┘  │
│                                                                          │
│  ┌────────────────────────────────────────────────────────────────────────┐  │
│  │  Refresh Schedulers (per provider)                                   │  │
│  │  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐  │  │
│  │  │ gemini scheduler│  │ qwen scheduler │  │ iflow scheduler│  │  │
│  │  │ RefreshScheduler│  │ RefreshScheduler│  │ RefreshScheduler│  │  │
│  │  └─────────────────┘  └─────────────────┘  └─────────────────┘  │  │
│  └────────────────────────────────────────────────────────────────────────┘  │
│                                                                          │
│  ┌────────────────────────────────────────────────────────────────────────┐  │
│  │  Health Trackers (per provider)                                     │  │
│  │  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐  │  │
│  │  │ gemini health   │  │ qwen health    │  │ iflow health   │  │  │
│  │  │ HealthTracker   │  │ HealthTracker   │  │ HealthTracker   │  │  │
│  │  └─────────────────┘  └─────────────────┘  └─────────────────┘  │  │
│  └────────────────────────────────────────────────────────────────────────┘  │
│                                                                          │
│  ┌────────────────────────────────────────────────────────────────────────┐  │
│  │  Email Extraction Manager                                           │  │
│  │  - GeminiEmailExtractor                                            │  │
│  │  - QwenEmailExtractor                                              │  │
│  │  - KiroEmailExtractor                                              │  │
│  │  - IFlowEmailExtractor                                             │  │
│  └────────────────────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────────────────────┘
                                      │
                                      │
                                      ▼
┌─────────────────────────────────────────────────────────────────────────────────┐
│                    File-Based Storage (.credentials/)                         │
│                                                                          │
│  .credentials/                                                            │
│  ├── gemini/                                                             │
│  │   ├── user1_example_com.json  ──────┐                                │
│  │   ├── user2_example_com.json        │                                │
│  │   └── settings.json                │                                │
│  ├── qwen/                              │                                │
│  │   ├── user3_example_com.json        │                                │
│  │   └── settings.json                │                                │
│  └── iflow/                             │                                │
│      └── settings.json                  │                                │
│                                         │                                │
│                                         │ JSON Marshal/Unmarshal            │
│                                         │ File I/O                         │
│                                         │ File Locking (flock)             │
│                                         │                                │
└─────────────────────────────────────────────┼────────────────────────────────┘
                                          │
                                          │
                                          ▼
┌─────────────────────────────────────────────────────────────────────────────────┐
│                    OAuth Providers                                          │
│  - Google OAuth2 (Gemini)                                               │
│  - Qwen OAuth2                                                           │
│  - iFlow OAuth2                                                          │
│  - Kiro OAuth2                                                           │
└─────────────────────────────────────────────────────────────────────────────────┘
```

---

## Summary

The current qwencoder-proxy architecture uses a file-based token storage system with the following characteristics:

### Strengths
- Simple implementation
- Easy to debug (human-readable JSON files)
- No external dependencies for storage
- Works well for small token counts

### Weaknesses
- Performance limitations due to file I/O
- Concurrency bottlenecks from file locking
- No transaction support
- Limited query capabilities
- Scalability concerns with large token counts

### Key Components
1. **MultiTokenManager** - Central coordinator
2. **MultiTokenStore** - File-based storage per provider
3. **TokenManager** - Token selection and management
4. **RefreshScheduler** - Automatic token refresh
5. **HealthTracker** - Token health monitoring
6. **EmailExtractionManager** - Email extraction from tokens

### Data Flow
1. OAuth tokens are acquired via `/api/callback`
2. Tokens are stored as individual JSON files
3. `/v1/chat/completions` requests trigger token selection
4. Tokens are refreshed automatically by scheduler
5. Health is tracked and updated on each API call

This architecture analysis provides the foundation for designing the SQLite migration strategy.
