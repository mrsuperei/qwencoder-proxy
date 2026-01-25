# Security Architecture Validation Report
## OAuth Management Dashboard

**Date:** 2026-01-23  
**Document:** `plan/oauth-dashboard-architecture.md`  
**Sections Validated:** Section 6 (Security Architecture) and Section 11 (Security Checklist)

---

## Executive Summary

This report provides a comprehensive validation of the security architecture for the OAuth Management Dashboard. While the architecture demonstrates a strong foundation with proper client secrets protection, CSRF protection, and rate limiting, several critical security improvements are needed to ensure production-grade security.

**Overall Assessment:** ⚠️ **NEEDS IMPROVEMENT** - Good foundation but requires additional security measures before production deployment.

---

## 1. Client Secrets Protection (Section 6.1, Lines 493-525)

### Current Implementation
- ✅ Uses `json:"-"` tags to prevent secrets from being exposed in API responses
- ✅ Server-side token exchange keeps secrets away from client
- ✅ Proper flow diagram showing secrets staying server-side
- ✅ Secrets stored in [`providers.json`](#42-providersjson-schema)

### Issues Identified

| Severity | Issue | Impact |
|----------|-------|--------|
| 🔴 **HIGH** | No file permissions specified for credential storage | Secrets readable by other users on multi-user systems |
| 🟡 **MEDIUM** | No encryption at rest for stored secrets | Secrets exposed if filesystem is compromised |
| 🟡 **MEDIUM** | No secret rotation strategy | Long-lived secrets increase attack surface |
| 🟢 **LOW** | No OS keyring integration | Less secure than platform-native credential storage |

### Required Improvements

```go
// Add to storage layer implementation
const (
    // File permissions for secure credential storage
    ConfigFilePermissions = 0600  // Owner read/write only
    ConfigDirPermissions  = 0700  // Owner execute/read/write only
)

// When creating directories (line 44 in existing auth/oauth.go pattern)
os.MkdirAll(dir, ConfigDirPermissions)

// When creating files
file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, ConfigFilePermissions)
```

### Additional Recommendations

1. **Encryption at Rest**: Consider using AES-256-GCM to encrypt secrets before storage
2. **OS Keyring**: For production, integrate with platform keyring (Windows Credential Manager, macOS Keychain, Linux Secret Service)
3. **Secret Rotation**: Implement automatic secret rotation with versioning
4. **Audit Logging**: Log all secret access events (without logging the secrets themselves)

---

## 2. CSRF Protection (Section 6.2, Lines 527-546)

### Current Implementation
- ✅ Uses `gorilla/csrf` middleware
- ✅ HTTPS-only mode (`csrf.Secure(true)`)
- ✅ SameSite strict mode
- ✅ Scoped to `/api/` path

### Issues Identified

| Severity | Issue | Impact |
|----------|-------|--------|
| 🔴 **HIGH** | CSRF key hardcoded in middleware | Key exposed in source code, not rotatable |
| 🟡 **MEDIUM** | No token expiration mechanism | Long-lived CSRF tokens increase risk |
| 🟢 **LOW** | No double-submit cookie pattern | Single layer of CSRF protection |

### Required Improvements

```go
// dashboard/middleware/csrf.go
package middleware

import (
    "github.com/gorilla/csrf"
    "github.com/sunbankio/qwencoder-proxy/config"
    "net/http"
)

// CSRFMiddleware provides CSRF protection with configurable key
func CSRFMiddleware(cfg *config.Config) func(http.Handler) http.Handler {
    // CSRF key should come from environment variable or config
    csrfKey := []byte(cfg.Dashboard.CSRFKey)
    if len(csrfKey) < 32 {
        // If not configured, generate a warning but still protect
        // In production, this should fail or use a secure default
    }
    
    return csrf.Protect(
        csrfKey,
        csrf.Secure(cfg.Dashboard.SecureCookies), // Configurable for dev/prod
        csrf.SameSite(csrf.SameSiteStrictMode),
        csrf.Path("/api/"),
        csrf.MaxAge(3600), // Add 1-hour expiration
    )
}
```

### Additional Recommendations

1. **Key Rotation**: Implement CSRF key rotation without breaking active sessions
2. **Token Storage**: Store CSRF tokens in session instead of cookie for additional security
3. **Per-Request Tokens**: Consider using per-request tokens for high-risk operations

---

## 3. Rate Limiting (Section 6.3, Lines 548-597)

### Current Implementation
- ✅ Uses `golang.org/x/time/rate`
- ✅ Per-IP rate limiting
- ✅ Configurable RPS and burst

### Issues Identified

| Severity | Issue | Impact |
|----------|-------|--------|
| 🟡 **MEDIUM** | No cleanup mechanism for old limiters | Memory leak over time |
| 🟡 **MEDIUM** | No endpoint-specific rate limits | OAuth callbacks should have different limits |
| 🟢 **LOW** | No rate limit headers in responses | Poor user experience |
| 🟢 **LOW** | No whitelist for trusted IPs | Admin accounts may be blocked |

### Required Improvements

```go
// dashboard/middleware/ratelimit.go
package middleware

import (
    "golang.org/x/time/rate"
    "net/http"
    "sync"
    "time"
)

// RateLimiter implements rate limiting per IP with cleanup
type RateLimiter struct {
    limiters map[string]*rate.Limiter
    mu       sync.RWMutex
    rate     rate.Limit
    burst    int
    lastAccess map[string]time.Time  // Track last access for cleanup
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(rps int, burst int) *RateLimiter {
    rl := &RateLimiter{
        limiters: make(map[string]*rate.Limiter),
        lastAccess: make(map[string]time.Time),
        rate:     rate.Limit(rps),
        burst:    burst,
    }
    // Start cleanup goroutine
    go rl.cleanupOldLimiters()
    return rl
}

// cleanupOldLimiters removes limiters not accessed in the last hour
func (rl *RateLimiter) cleanupOldLimiters() {
    ticker := time.NewTicker(5 * time.Minute)
    defer ticker.Stop()
    
    for range ticker.C {
        rl.mu.Lock()
        cutoff := time.Now().Add(-1 * time.Hour)
        for ip, lastAccess := range rl.lastAccess {
            if lastAccess.Before(cutoff) {
                delete(rl.limiters, ip)
                delete(rl.lastAccess, ip)
            }
        }
        rl.mu.Unlock()
    }
}

// Middleware returns the rate limiting middleware
func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        ip := getClientIP(r)
        
        rl.mu.Lock()
        limiter, exists := rl.limiters[ip]
        if !exists {
            limiter = rate.NewLimiter(rl.rate, rl.burst)
            rl.limiters[ip] = limiter
        }
        rl.lastAccess[ip] = time.Now()  // Update access time
        rl.mu.Unlock()
        
        if !limiter.Allow() {
            // Add rate limit headers
            w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", int(rl.rate)))
            w.Header().Set("X-RateLimit-Remaining", "0")
            w.Header().Set("X-RateLimit-Reset", time.Now().Add(time.Second).Format(time.RFC1123))
            http.Error(w, "Too many requests", http.StatusTooManyRequests)
            return
        }
        
        next.ServeHTTP(w, r)
    })
}

// Endpoint-specific rate limiters
type EndpointRateLimit struct {
    PathPattern string
    RPS         int
    Burst       int
}

var endpointLimits = []EndpointRateLimit{
    {PathPattern: "/api/oauth/callback", RPS: 30, Burst: 50},  // Higher for OAuth callbacks
    {PathPattern: "/api/providers", RPS: 10, Burst: 20},       // Standard for CRUD
    {PathPattern: "/api/dashboard", RPS: 20, Burst: 40},       // Higher for dashboard reads
}
```

### Additional Recommendations

1. **Rate Limit Headers**: Include `X-RateLimit-Limit`, `X-RateLimit-Remaining`, `X-RateLimit-Reset`
2. **Trusted IP Whitelist**: Allow configuration of trusted IPs that bypass rate limiting
3. **Distributed Rate Limiting**: For multi-instance deployments, use Redis or similar
4. **Adaptive Rate Limiting**: Adjust limits based on user authentication status

---

## 4. CORS Configuration (Section 6.4, Lines 599-626)

### Current Implementation
- ✅ CORS middleware implemented
- ✅ Allows common methods and headers
- ✅ Handles OPTIONS preflight

### Issues Identified

| Severity | Issue | Impact |
|----------|-------|--------|
| 🔴 **CRITICAL** | Wildcard origin (`*`) with credentials enabled | **Invalid per CORS spec**, security vulnerability |
| 🔴 **HIGH** | No origin validation | Any origin can access the API |
| 🟡 **MEDIUM** | No `Vary: Origin` header | Caching issues with CORS |
| 🟢 **LOW** | No configurable origins | Hard to deploy to different domains |

### Required Improvements

```go
// dashboard/middleware/cors.go
package middleware

import (
    "net/http"
    "strings"
)

// CORSMiddleware handles CORS headers with proper origin validation
func CORSMiddleware(allowedOrigins []string, allowCredentials bool) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            origin := r.Header.Get("Origin")
            
            // Validate origin against allowlist
            allowedOrigin := ""
            if origin != "" {
                for _, allowed := range allowedOrigins {
                    if allowed == "*" || strings.EqualFold(origin, allowed) {
                        allowedOrigin = allowed
                        break
                    }
                }
            }
            
            // Only set CORS headers if origin is valid
            if allowedOrigin != "" {
                // CRITICAL: If credentials are allowed, origin must be specific, not wildcard
                if allowCredentials {
                    w.Header().Set("Access-Control-Allow-Origin", origin)
                } else {
                    w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
                }
                w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
                w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-CSRF-Token, Authorization")
                w.Header().Set("Access-Control-Allow-Credentials", fmt.Sprintf("%t", allowCredentials))
                w.Header().Set("Vary", "Origin")
            }
            
            if r.Method == "OPTIONS" {
                w.WriteHeader(http.StatusOK)
                return
            }
            
            next.ServeHTTP(w, r)
        })
    }
}
```

### Configuration Example

```go
// dashboard/config.go
type DashboardConfig struct {
    // ... existing fields ...
    AllowedOrigins   []string `json:"allowed_origins"`   // e.g., ["https://dashboard.example.com"]
    AllowCredentials bool     `json:"allow_credentials"` // true for same-origin, false for cross-origin
}

// Default configuration
func DefaultConfig() *DashboardConfig {
    return &DashboardConfig{
        // ... existing config ...
        AllowedOrigins:   []string{"http://localhost:8143"}, // Dev default
        AllowCredentials: true,  // Safe for same-origin
    }
}
```

### Additional Recommendations

1. **Origin Validation**: Always validate origin against allowlist
2. **Environment-Specific Origins**: Use different origins for dev/staging/prod
3. **CORS Preflight Caching**: Set `Access-Control-Max-Age` for performance
4. **Expose Headers**: Add `Access-Control-Expose-Headers` for custom headers

---

## 5. Session Security (Section 6.5, Lines 628-656)

### Current Implementation
- ✅ Uses `crypto/rand` for token generation
- ✅ Uses `base64.URLEncoding`
- ✅ Configurable session duration

### Issues Identified

| Severity | Issue | Impact |
|----------|-------|--------|
| 🔴 **HIGH** | No `HttpOnly` flag mentioned | Cookies accessible to JavaScript (XSS risk) |
| 🔴 **HIGH** | No session fixation protection | Attackers can set session IDs |
| 🟡 **MEDIUM** | No cookie encryption | Session tokens readable if intercepted |
| 🟡 **MEDIUM** | No IP binding for sessions | Session hijacking possible |
| 🟢 **LOW** | No concurrent session limits | User can have unlimited sessions |

### Required Improvements

```go
// dashboard/session/session.go
package session

import (
    "crypto/rand"
    "encoding/base64"
    "net/http"
    "time"
)

// SessionConfig holds session configuration
type SessionConfig struct {
    CookieName     string
    Duration       time.Duration
    Secure         bool  // HTTPS only
    HttpOnly       bool  // Not accessible via JavaScript
    SameSite       http.SameSite
    MaxAge         int   // Cookie max age in seconds
}

// DefaultSessionConfig returns secure defaults
func DefaultSessionConfig() *SessionConfig {
    return &SessionConfig{
        CookieName: "qwencoder_session",
        Duration:   24 * time.Hour,
        Secure:     true,  // Always true in production
        HttpOnly:   true,  // CRITICAL: Prevent XSS from stealing cookies
        SameSite:   http.SameSiteStrictMode,
        MaxAge:     86400, // 24 hours
    }
}

// GenerateToken generates a secure random session token
func GenerateToken() (string, error) {
    b := make([]byte, 32)  // 256 bits of entropy
    if _, err := rand.Read(b); err != nil {
        return "", err
    }
    return base64.URLEncoding.EncodeToString(b), nil
}

// SetSessionCookie sets a secure session cookie
func SetSessionCookie(w http.ResponseWriter, token string, cfg *SessionConfig) {
    http.SetCookie(w, &http.Cookie{
        Name:     cfg.CookieName,
        Value:    token,
        Path:     "/",
        MaxAge:   cfg.MaxAge,
        Secure:   cfg.Secure,
        HttpOnly: cfg.HttpOnly,  // CRITICAL: Prevents XSS attacks
        SameSite: cfg.SameSite,
    })
}

// Session represents a user session with additional security
type Session struct {
    ID        string    `json:"id"`
    Token     string    `json:"token"`
    CreatedAt time.Time `json:"created_at"`
    ExpiresAt time.Time `json:"expires_at"`
    ClientIP  string    `json:"client_ip"`   // For IP binding
    UserAgent string    `json:"user_agent"` // For additional validation
    Data      map[string]string `json:"data"`
}

// ValidateSession checks if session is valid and matches expected context
func (s *Session) Validate(expectedIP, expectedUA string) bool {
    if time.Now().After(s.ExpiresAt) {
        return false
    }
    
    // IP binding - optional but recommended
    if expectedIP != "" && s.ClientIP != expectedIP {
        return false
    }
    
    // User agent validation - optional but recommended
    if expectedUA != "" && s.UserAgent != expectedUA {
        return false
    }
    
    return true
}
```

### Session Fixation Protection

```go
// handlers/auth.go
// After successful authentication, regenerate session
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
    // ... authenticate user ...
    
    // Invalidate old session if exists
    oldSessionID := getSessionID(r)
    if oldSessionID != "" {
        h.sessionStore.Delete(oldSessionID)
    }
    
    // Create new session
    newToken, _ := session.GenerateToken()
    newSession := &session.Session{
        ID:        generateID(),
        Token:     newToken,
        CreatedAt: time.Now(),
        ExpiresAt: time.Now().Add(sessionCfg.Duration),
        ClientIP:  getClientIP(r),
        UserAgent: r.UserAgent(),
    }
    
    h.sessionStore.Save(newSession)
    session.SetSessionCookie(w, newToken, sessionCfg)
}
```

### Additional Recommendations

1. **Cookie Encryption**: Encrypt session cookie values for additional security
2. **Concurrent Session Limits**: Limit number of active sessions per user
3. **Session Invalidation**: Invalidate all sessions on password change or suspicious activity
4. **Session Rotation**: Rotate session IDs periodically

---

## 6. Token Refresh Strategy (Section 6.6, Lines 658-676)

### Current Implementation
- ✅ Uses 30-minute buffer (matches existing [`TokenRefreshBufferMs`](../auth/constants.go:6))
- ✅ Auto-refresh on API access
- ✅ Manual refresh button
- ✅ Failed refresh marks provider as "needs re-authentication"

### Issues Identified

| Severity | Issue | Impact |
|----------|-------|--------|
| 🟡 **MEDIUM** | No refresh token rotation | Stolen refresh tokens remain valid |
| 🟡 **MEDIUM** | No token revocation on logout | Tokens remain valid after logout |
| 🟡 **MEDIUM** | No handling of refresh token expiration | Poor error handling |
| 🟢 **LOW** | No exponential backoff for failed refresh | Can overwhelm provider API |

### Required Improvements

```go
// dashboard/services/oauth.go
package services

import (
    "context"
    "fmt"
    "time"
    
    "github.com/sunbankio/qwencoder-proxy/dashboard/models"
    "golang.org/x/oauth2"
)

// RefreshToken refreshes an access token with rotation
func (s *OAuthService) RefreshToken(ctx context.Context, providerID string) error {
    provider, err := s.providerStorage.Get(providerID)
    if err != nil {
        return fmt.Errorf("provider not found: %w", err)
    }
    
    token, err := s.tokenStorage.Get(providerID)
    if err != nil {
        return fmt.Errorf("no tokens found: %w", err)
    }
    
    if token.RefreshToken == "" {
        return fmt.Errorf("no refresh token available")
    }
    
    // Check if refresh token is expired (if provider returns expiry)
    if token.RefreshExpiry != nil && time.Now().After(*token.RefreshExpiry) {
        return fmt.Errorf("refresh token expired, user must re-authenticate")
    }
    
    // Build OAuth2 config
    conf := &oauth2.Config{
        ClientID:     provider.ClientID,
        ClientSecret: provider.ClientSecret,
        RedirectURL:  provider.RedirectURL,
        Endpoint: oauth2.Endpoint{
            TokenURL: provider.TokenURL,
        },
    }
    
    // Create token source
    ts := conf.TokenSource(ctx, &oauth2.Token{
        RefreshToken: token.RefreshToken,
    })
    
    // Get new token with retry logic
    var newToken *oauth2.Token
    var refreshErr error
    
    // Implement exponential backoff for failed refresh attempts
    maxRetries := 3
    for attempt := 0; attempt < maxRetries; attempt++ {
        newToken, refreshErr = ts.Token()
        if refreshErr == nil {
            break
        }
        
        if attempt < maxRetries-1 {
            // Exponential backoff: 1s, 2s, 4s
            backoff := time.Duration(1<<uint(attempt)) * time.Second
            time.Sleep(backoff)
        }
    }
    
    if refreshErr != nil {
        // Mark provider as needing re-authentication
        provider.Connected = false
        provider.Error = "Token refresh failed: re-authentication required"
        s.providerStorage.Save(provider)
        return fmt.Errorf("failed to refresh token after %d attempts: %w", maxRetries, refreshErr)
    }
    
    // REFRESH TOKEN ROTATION: Use new refresh token if provided
    // This is a security best practice (RFC 6749 Section 1.5)
    oldRefreshToken := token.RefreshToken
    token.AccessToken = newToken.AccessToken
    if newToken.RefreshToken != "" {
        token.RefreshToken = newToken.RefreshToken
    }
    token.ExpiresAt = newToken.Expiry
    token.LastRefreshed = time.Now()
    
    // If refresh token rotation is supported, invalidate old token
    if newToken.RefreshToken != "" && newToken.RefreshToken != oldRefreshToken {
        // Some providers support token rotation via revocation endpoint
        // This would be provider-specific
        s.logTokenRotation(providerID, oldRefreshToken)
    }
    
    return s.tokenStorage.Save(token)
}

// RevokeToken revokes stored tokens and invalidates refresh token
func (s *OAuthService) RevokeToken(ctx context.Context, providerID string) error {
    provider, err := s.providerStorage.Get(providerID)
    if err != nil {
        return fmt.Errorf("provider not found: %w", err)
    }
    
    token, err := s.tokenStorage.Get(providerID)
    if err != nil {
        return fmt.Errorf("no tokens found: %w", err)
    }
    
    // Attempt to revoke tokens with provider if revocation endpoint exists
    if provider.RevocationURL != "" {
        err := s.revokeWithProvider(ctx, provider, token)
        if err != nil {
            // Log but continue with local deletion
            // Provider may not support revocation
        }
    }
    
    // Delete tokens from local storage
    if err := s.tokenStorage.Delete(providerID); err != nil {
        return err
    }
    
    // Update provider status
    provider.Connected = false
    provider.Error = ""
    provider.UpdatedAt = time.Now()
    return s.providerStorage.Save(provider)
}

// StoredToken extended to support refresh token expiry
type StoredToken struct {
    ProviderID      string     `json:"provider_id"`
    AccessToken     string     `json:"access_token"`
    RefreshToken    string     `json:"refresh_token,omitempty"`
    RefreshExpiry   *time.Time `json:"refresh_expiry,omitempty"`  // Add this
    TokenType       string     `json:"token_type"`
    ExpiresAt       time.Time  `json:"expires_at"`
    Scope           string     `json:"scope,omitempty"`
    
    // Metadata
    CreatedAt       time.Time  `json:"created_at"`
    LastRefreshed   time.Time  `json:"last_refreshed"`
    UserID          string     `json:"user_id,omitempty"`
}
```

### Additional Recommendations

1. **Refresh Token Rotation**: Always rotate refresh tokens when supported by provider
2. **Token Revocation**: Implement provider-specific revocation endpoints
3. **Exponential Backoff**: Implement retry logic with backoff for failed refreshes
4. **Refresh Token Expiry**: Track and handle refresh token expiration
5. **Audit Logging**: Log all token refresh and revocation events

---

## 7. Security Checklist Validation (Section 11, Lines 2288-2300)

### Current Checklist
```
- [x] Client secrets never exposed to client (server-side only)
- [x] CSRF protection on all state-changing endpoints
- [x] Rate limiting per IP address
- [x] Secure session cookies (HttpOnly, Secure, SameSite)
- [x] State parameter validation in OAuth flow
- [x] File locking for concurrent storage access
- [x] Input validation and sanitization
- [x] Proper error messages (no sensitive data leakage)
- [x] HTTPS enforcement in production
- [x] Token refresh with expiry buffer
```

### Issues with Current Checklist

| Item | Status | Issue |
|------|--------|-------|
| Client secrets never exposed | ⚠️ PARTIAL | Missing file permissions and encryption |
| CSRF protection | ⚠️ PARTIAL | Missing token expiration and key rotation |
| Rate limiting | ⚠️ PARTIAL | Missing cleanup and endpoint-specific limits |
| Secure session cookies | ❌ INCOMPLETE | HttpOnly flag not explicitly mentioned |
| Input validation | ❌ NOT IMPLEMENTED | No validation framework specified |
| HTTPS enforcement | ⚠️ PARTIAL | No HSTS or redirect enforcement |

### Updated Security Checklist

```markdown
## 11. Security Checklist

### Client Secrets Protection
- [x] Client secrets never exposed to client (server-side only)
- [ ] Client secrets stored with secure file permissions (0600 for files, 0700 for directories)
- [ ] Client secrets encrypted at rest (AES-256-GCM or equivalent)
- [ ] Secret rotation strategy implemented
- [ ] OS keyring integration for production deployments

### CSRF Protection
- [x] CSRF protection on all state-changing endpoints
- [ ] CSRF token expiration (recommended: 1 hour)
- [ ] CSRF key rotation mechanism
- [ ] Double-submit cookie pattern for high-risk operations
- [ ] CSRF key sourced from environment variable or config

### Rate Limiting
- [x] Rate limiting per IP address
- [ ] Rate limiter cleanup mechanism (prevent memory leaks)
- [ ] Endpoint-specific rate limits (OAuth callbacks, CRUD operations)
- [ ] Rate limit headers in responses (X-RateLimit-* headers)
- [ ] Trusted IP whitelist for admin accounts
- [ ] Distributed rate limiting for multi-instance deployments

### CORS Configuration
- [ ] CORS origin validation (no wildcard with credentials)
- [ ] Environment-specific origin allowlist
- [ ] Vary: Origin header set
- [ ] Preflight caching configuration
- [ ] Configurable allowed methods and headers

### Session Security
- [x] Secure session cookies (HttpOnly, Secure, SameSite)
- [ ] Session fixation protection (regenerate session on login)
- [ ] Cookie encryption for additional security
- [ ] IP binding for sessions (optional but recommended)
- [ ] Concurrent session limits per user
- [ ] Session invalidation on password change or suspicious activity
- [ ] Session rotation mechanism

### Token Management
- [x] Token refresh with expiry buffer (30 minutes)
- [ ] Refresh token rotation on each use
- [ ] Token revocation on logout
- [ ] Refresh token expiration handling
- [ ] Exponential backoff for failed refresh attempts
- [ ] Provider-specific token revocation endpoints
- [ ] Audit logging for token operations

### Input Validation
- [ ] Input validation framework implemented
- [ ] URL validation for OAuth endpoints
- [ ] Client ID/Secret format validation
- [ ] JSON schema validation for API requests
- [ ] SQL injection prevention (if database added later)
- [ ] XSS prevention in all user-facing content

### Error Handling
- [x] Proper error messages (no sensitive data leakage)
- [ ] Structured error codes for client handling
- [ ] Error logging for security events
- [ ] Generic error messages for authentication failures
- [ ] Stack traces only in debug mode

### Transport Security
- [x] HTTPS enforcement in production
- [ ] HSTS header configuration
- [ ] HTTP to HTTPS redirect
- [ ] TLS 1.2+ requirement
- [ ] Secure cipher suite configuration

### Storage Security
- [x] File locking for concurrent storage access
- [ ] Secure file permissions (0600/0700)
- [ ] Directory permission validation
- [ ] Atomic file writes
- [ ] Backup encryption (if implemented)

### OAuth Flow Security
- [x] State parameter validation in OAuth flow
- [ ] State parameter expiration (10 minutes)
- [ ] PKCE (Proof Key for Code Exchange) for public clients
- [ ] Redirect URI validation
- [ ] Nonce parameter for OpenID Connect

### Logging & Monitoring
- [ ] Security event logging (authentication, authorization failures)
- [ ] Audit trail for sensitive operations
- [ ] Rate limit violation logging
- [ ] CSRF token validation failures logged
- [ ] Token refresh/revocation logging
- [ ] Anomaly detection for suspicious activity

### Compliance & Best Practices
- [ ] OWASP Top 10 vulnerabilities addressed
- [ ] OAuth 2.0 Security Best Current Practice (RFC 6819)
- [ ] Regular security audits
- [ ] Dependency vulnerability scanning
- [ ] Security testing in CI/CD pipeline
```

---

## 8. Additional Security Components Needed

### 8.1 Input Validation Framework

```go
// dashboard/validation/validator.go
package validation

import (
    "net/url"
    "regexp"
    "strings"
)

// Validator provides input validation
type Validator struct {
    // Predefined patterns
    urlPattern    *regexp.Regexp
    clientIDPattern *regexp.Regexp
}

func NewValidator() *Validator {
    return &Validator{
        urlPattern:    regexp.MustCompile(`^https?://[a-zA-Z0-9\-._~:/?#\[\]@!$&'()*+,;=]+$`),
        clientIDPattern: regexp.MustCompile(`^[a-zA-Z0-9\-._~]+$`),
    }
}

// ValidateURL validates an OAuth endpoint URL
func (v *Validator) ValidateURL(endpoint string) error {
    parsed, err := url.Parse(endpoint)
    if err != nil {
        return fmt.Errorf("invalid URL format: %w", err)
    }
    
    if parsed.Scheme != "https" && parsed.Scheme != "http" {
        return fmt.Errorf("URL must use http or https scheme")
    }
    
    if !v.urlPattern.MatchString(endpoint) {
        return fmt.Errorf("URL contains invalid characters")
    }
    
    return nil
}

// ValidateClientID validates OAuth client ID format
func (v *Validator) ValidateClientID(clientID string) error {
    if clientID == "" {
        return fmt.Errorf("client ID cannot be empty")
    }
    
    if len(clientID) > 255 {
        return fmt.Errorf("client ID too long (max 255 characters)")
    }
    
    if !v.clientIDPattern.MatchString(clientID) {
        return fmt.Errorf("client ID contains invalid characters")
    }
    
    return nil
}

// ValidateScopes validates OAuth scopes
func (v *Validator) ValidateScopes(scopes []string) error {
    if len(scopes) == 0 {
        return fmt.Errorf("at least one scope is required")
    }
    
    for _, scope := range scopes {
        if scope == "" {
            return fmt.Errorf("scope cannot be empty")
        }
        
        if len(scope) > 255 {
            return fmt.Errorf("scope too long (max 255 characters)")
        }
        
        // Scope should only contain alphanumeric, hyphens, underscores, dots, and colons
        scopePattern := regexp.MustCompile(`^[a-zA-Z0-9\-._:/]+$`)
        if !scopePattern.MatchString(scope) {
            return fmt.Errorf("scope '%s' contains invalid characters", scope)
        }
    }
    
    return nil
}
```

### 8.2 Security Middleware Stack

```go
// dashboard/middleware/security.go
package middleware

import (
    "net/http"
    "strings"
)

// SecurityMiddleware applies multiple security headers
func SecurityMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Security headers
        w.Header().Set("X-Content-Type-Options", "nosniff")
        w.Header().Set("X-Frame-Options", "DENY")
        w.Header().Set("X-XSS-Protection", "1; mode=block")
        w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
        w.Header().Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")
        
        // HSTS (HTTP Strict Transport Security)
        // Only set if already using HTTPS
        if r.URL.Scheme == "https" {
            w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
        }
        
        // Content Security Policy (basic version)
        // Adjust based on your needs
        w.Header().Set("Content-Security-Policy", 
            "default-src 'self'; " +
            "script-src 'self' 'unsafe-inline'; " +
            "style-src 'self' 'unsafe-inline'; " +
            "img-src 'self' data:; " +
            "font-src 'self';")
        
        next.ServeHTTP(w, r)
    })
}

// HTTPSRedirect redirects HTTP to HTTPS
func HTTPSRedirect(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if r.URL.Scheme != "https" && !strings.HasPrefix(r.Host, "localhost") {
            httpsURL := "https://" + r.Host + r.URL.RequestURI()
            http.Redirect(w, r, httpsURL, http.StatusPermanentRedirect)
            return
        }
        next.ServeHTTP(w, r)
    })
}
```

### 8.3 Audit Logging

```go
// dashboard/audit/logger.go
package audit

import (
    "encoding/json"
    "time"
)

// EventType represents the type of audit event
type EventType string

const (
    EventAuthSuccess      EventType = "auth_success"
    EventAuthFailure      EventType = "auth_failure"
    EventTokenRefresh     EventType = "token_refresh"
    EventTokenRevoke      EventType = "token_revoke"
    EventProviderCreated  EventType = "provider_created"
    EventProviderUpdated  EventType = "provider_updated"
    EventProviderDeleted  EventType = "provider_deleted"
    EventCSRFViolation    EventType = "csrf_violation"
    EventRateLimitExceed  EventType = "rate_limit_exceed"
)

// AuditEvent represents a security audit event
type AuditEvent struct {
    Timestamp   time.Time   `json:"timestamp"`
    EventType   EventType   `json:"event_type"`
    UserID      string      `json:"user_id,omitempty"`
    IPAddress   string      `json:"ip_address"`
    UserAgent   string      `json:"user_agent,omitempty"`
    Resource    string      `json:"resource,omitempty"`
    Action      string      `json:"action,omitempty"`
    Success     bool        `json:"success"`
    Details     interface{} `json:"details,omitempty"`
}

// Logger handles audit logging
type Logger struct {
    logger *logging.Logger
}

func NewAuditLogger(logger *logging.Logger) *Logger {
    return &Logger{logger: logger}
}

// LogEvent logs an audit event
func (l *Logger) LogEvent(event *AuditEvent) {
    // Don't log sensitive details
    if event.Details != nil {
        // Sanitize details if needed
        event.Details = sanitizeDetails(event.Details)
    }
    
    data, _ := json.Marshal(event)
    l.logger.InfoLog("AUDIT: %s", string(data))
}

// sanitizeDetails removes sensitive information from audit details
func sanitizeDetails(details interface{}) interface{} {
    // Implement sanitization logic
    // Remove passwords, tokens, secrets, etc.
    return details
}
```

---

## 9. Integration with Existing Codebase

### 9.1 Consistent with Existing Patterns

| Pattern | Existing Codebase | Dashboard Architecture | Status |
|---------|------------------|------------------------|--------|
| File Locking | [`flock.New()`](../auth/oauth.go:50) | ✅ Planned | ✅ Consistent |
| Token Refresh Buffer | [`TokenRefreshBufferMs`](../auth/constants.go:6) | ✅ 30-minute buffer | ✅ Consistent |
| File Permissions | [`0700`](../auth/oauth.go:45) | ❌ Not specified | ⚠️ Needs addition |
| OAuth2 Library | [`golang.org/x/oauth2`](../auth/oauth.go:13) | ✅ Planned | ✅ Consistent |
| HTTP Client | Custom timeout config | ✅ Planned | ✅ Consistent |

### 9.2 Recommended Integration Points

```go
// dashboard/storage/lock.go
// Reuse existing file locking pattern from auth/oauth.go

package storage

import (
    "github.com/gofrs/flock"
    "os"
    "path/filepath"
)

// FileLock wraps the existing flock pattern
type FileLock struct {
    lock *flock.Flock
    file *os.File
}

// AcquireLock acquires a file lock with proper permissions
func AcquireLock(basePath, name string) (*FileLock, error) {
    lockPath := filepath.Join(basePath, name+".lock")
    
    // Ensure directory exists with secure permissions
    dir := filepath.Dir(lockPath)
    if err := os.MkdirAll(dir, 0700); err != nil {
        return nil, err
    }
    
    lock := flock.New(lockPath)
    locked, err := lock.TryLock()
    if err != nil {
        return nil, err
    }
    if !locked {
        return nil, fmt.Errorf("failed to acquire lock")
    }
    
    return &FileLock{lock: lock}, nil
}

// Release releases the lock
func (fl *FileLock) Release() error {
    if fl.lock != nil {
        return fl.lock.Unlock()
    }
    return nil
}
```

---

## 10. Summary of Findings

### Critical Issues (Must Fix Before Production)

1. **CORS Configuration**: Wildcard origin with credentials enabled is invalid and insecure
2. **Session Cookies**: HttpOnly flag not explicitly set, exposing to XSS attacks
3. **File Permissions**: No secure file permissions specified for credential storage
4. **CSRF Key**: Hardcoded in middleware, should be configurable
5. **Input Validation**: No validation framework specified

### High Priority Issues

1. **Rate Limiter Memory Leak**: No cleanup mechanism for old limiters
2. **Session Fixation**: No session regeneration on login
3. **Refresh Token Rotation**: Not implemented
4. **Token Revocation**: No logout token revocation
5. **Encryption at Rest**: Secrets stored in plaintext

### Medium Priority Issues

1. **IP Binding**: No session IP binding
2. **Concurrent Sessions**: No limit on active sessions
3. **Endpoint-Specific Rate Limits**: One-size-fits-all rate limiting
4. **CSRF Token Expiration**: No expiration mechanism
5. **Audit Logging**: No security event logging

### Low Priority Issues

1. **Rate Limit Headers**: Not included in responses
2. **Trusted IP Whitelist**: No admin bypass
3. **Double-Submit CSRF**: Single layer of protection
4. **Cookie Encryption**: Additional security layer
5. **Exponential Backoff**: For failed token refresh

---

## 11. Recommendations

### Immediate Actions (Before Production)

1. **Fix CORS Configuration**: Implement proper origin validation
2. **Add HttpOnly to Cookies**: Explicitly set HttpOnly flag
3. **Specify File Permissions**: Use 0600 for files, 0700 for directories
4. **Implement Input Validation**: Create validation framework
5. **Configure CSRF Key**: Make CSRF key configurable

### Short-Term (Within First Sprint)

1. **Add Rate Limiter Cleanup**: Prevent memory leaks
2. **Implement Session Fixation Protection**: Regenerate sessions on login
3. **Add Refresh Token Rotation**: Rotate tokens on refresh
4. **Implement Token Revocation**: Revoke tokens on logout
5. **Add Audit Logging**: Log security events

### Medium-Term (Within First Month)

1. **Add Encryption at Rest**: Encrypt stored secrets
2. **Implement IP Binding**: Optional session IP validation
3. **Add Concurrent Session Limits**: Limit active sessions per user
4. **Endpoint-Specific Rate Limits**: Different limits for different endpoints
5. **Add Security Headers**: Implement comprehensive security headers

### Long-Term (Ongoing)

1. **OS Keyring Integration**: Platform-native credential storage
2. **Distributed Rate Limiting**: For multi-instance deployments
3. **Advanced Threat Detection**: Anomaly detection for suspicious activity
4. **Regular Security Audits**: Periodic security reviews
5. **Dependency Scanning**: Automated vulnerability scanning

---

## 12. Conclusion

The OAuth Management Dashboard security architecture demonstrates a strong foundation with proper client secrets protection, CSRF protection, and rate limiting. However, several critical security improvements are needed before production deployment:

**Strengths:**
- ✅ Client secrets properly protected from client exposure
- ✅ CSRF protection implemented with gorilla/csrf
- ✅ Rate limiting per IP address
- ✅ Token refresh with proper buffer
- ✅ Integration with existing codebase patterns

**Critical Gaps:**
- ❌ CORS configuration is invalid (wildcard with credentials)
- ❌ Session cookies missing HttpOnly flag
- ❌ No secure file permissions specified
- ❌ CSRF key hardcoded
- ❌ No input validation framework

**Overall Recommendation:** 
The architecture is **NOT ready for production** without addressing the critical issues identified in this report. Implement the immediate actions and short-term improvements before deploying to production.

---

## Appendix A: Security Best Practices References

1. **OAuth 2.0 Security Best Current Practice** (RFC 6819)
2. **OWASP Top 10 Web Application Security Risks**
3. **OWASP OAuth 2.0 Cheat Sheet**
4. **MDN Web Security Guide**
5. **Google OAuth 2.0 Security Guidelines**
6. **Microsoft Identity Platform Security Best Practices**

---

**Report Generated:** 2026-01-23  
**Reviewed By:** Security Architecture Validation  
**Next Review Date:** Before production deployment
