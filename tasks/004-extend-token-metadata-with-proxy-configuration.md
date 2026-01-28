# Task 004: Extend Token Metadata with Proxy Configuration

## Description
Extend the ProviderToken structure to include proxy configuration fields. This enables storing proxy settings alongside token metadata while maintaining backward compatibility with existing token files.

## Technical Context
This task integrates proxy configuration into the token storage system:

- **Backward Compatibility**: New fields must be optional (omitempty in JSON tags)
- **Data Model Extension**: Following proxy-per-token architecture design
- **File-Based Storage**: Existing tokens load without proxy fields (default to "none")

**Current ProviderToken Structure** ([`auth/multi_token_store.go:18`](auth/multi_token_store.go:18)):
```go
type ProviderToken struct {
    ID             string
    AccessToken    string
    RefreshToken   string
    TokenType      string
    ExpiryDate     int64
    Email          string
    Healthy        bool
    HealthScore    float64
    LastUsed       int64
    CreatedAt      int64
    ErrorCount     int
}
```

**Extensions Required**:
- `Proxy *ProxyConfig` - Proxy configuration for this token
- `ProxyHealthScore float64` - Separate health tracking for proxy (0.0-1.0)

**JSON Storage Format** (backward compatible):
```json
{
  "id": "uuid",
  "access_token": "...",
  // ... existing fields ...
  "proxy": {
    "type": "socks5",
    "host": "proxy.example.com",
    "port": 1080,
    "username": "user",
    "password": "encrypted_password",
    "enabled": true
  },
  "proxy_health_score": 0.95
}
```

**Architectural Principles**:
- **Open/Closed Principle**: Extend existing structure without breaking changes
- **Single Responsibility**: Token storage handles both token and proxy metadata
- **Backward Compatibility**: Existing tokens without proxy fields work correctly

## Subtasks
1. Modify [`auth/multi_token_store.go`](auth/multi_token_store.go) ProviderToken struct:
   - Add `Proxy *ProxyConfig` field with `json:"proxy,omitempty"` tag
   - Add `ProxyHealthScore float64` field with `json:"proxy_health_score,omitempty"` tag
2. Update JSON marshaling to handle nil Proxy field gracefully
3. Update JSON unmarshaling to handle missing proxy fields:
   - Default to nil Proxy (no proxy configured)
   - Default ProxyHealthScore to 1.0 (healthy)
4. Update SaveToken() method to persist new proxy fields
5. Update LoadToken() method to load proxy fields from file
6. Add validation in AddToken() to ensure proxy config is valid if provided
7. Add unit tests for:
   - Loading existing tokens without proxy config (backward compatibility)
   - Saving tokens with proxy config
   - Loading tokens with proxy config
   - JSON marshaling/unmarshaling
   - ProxyHealthScore default values
8. Ensure proxy password encryption is handled (credential masking in logs)

## Dependencies
- Task 001: Create Proxy Configuration Data Structures (uses ProxyConfig)

## Files to Modify
- `auth/multi_token_store.go` (modify ProviderToken struct and methods)
- `auth/multi_token_store_test.go` (add tests for proxy fields)

## Related Tasks
- Task 005: Add Proxy Health Tracking to Token Metadata (uses ProxyHealthScore)
- Task 008: Update all authenticator implementations (may need proxy awareness)
