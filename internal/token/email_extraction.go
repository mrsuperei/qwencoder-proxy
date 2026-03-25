// Package auth provides email extraction implementations for OAuth providers
package token

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/sunbankio/qwencoder-proxy/internal/logging"
)

// EmailExtractor defines the interface for extracting email from OAuth responses
type EmailExtractor interface {
	// ExtractEmail extracts email from the OAuth token response
	ExtractEmail(ctx context.Context, tokenResponse map[string]interface{}, accessToken string) (string, error)
	// ProviderID returns the provider identifier
	ProviderID() string
	// UserInfoURL returns the user info endpoint URL (if applicable)
	UserInfoURL() string
}

// QwenEmailExtractor implements EmailExtractor for Qwen
type QwenEmailExtractor struct {
	userInfoURL string
	httpClient  *http.Client
	logger      logging.Logger
}

// ProviderID returns the provider identifier for Qwen
func (qe *QwenEmailExtractor) ProviderID() string {
	return "qwen"
}

// NewQwenEmailExtractor creates a new QwenEmailExtractor
func NewQwenEmailExtractor(httpClient *http.Client, logger logging.Logger) *QwenEmailExtractor {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &QwenEmailExtractor{
		userInfoURL: QwenUserInfoURL,
		httpClient:  httpClient,
		logger:      logger,
	}
}

// ExtractEmail extracts email from the OAuth token response for Qwen
func (qe *QwenEmailExtractor) ExtractEmail(ctx context.Context, tokenResponse map[string]interface{}, accessToken string) (string, error) {
	// First try to extract email directly from token response
	if email, found := extractEmailFromTokenResponse(tokenResponse); found {
		qe.logger.DebugLog("[QwenEmailExtractor] Email found in token response: %s", normalizeEmail(email))
		return normalizeEmail(email), nil
	}

	// If not in token response, fetch from user info endpoint
	qe.logger.DebugLog("[QwenEmailExtractor] Email not in token response, fetching from user info endpoint")
	email, err := fetchEmailFromUserInfo(ctx, qe.userInfoURL, accessToken, qe.httpClient, qe.logger)
	if err != nil {
		return "", fmt.Errorf("failed to fetch email from user info endpoint: %w", err)
	}

	return normalizeEmail(email), nil
}

// GetRegisteredProviders returns the list of registered providers (for initialization)
// This method is called by MultiTokenManager during initialization
func (eem *EmailExtractionManager) GetRegisteredProviders() []string {
	eem.mu.RLock()
	defer eem.mu.RUnlock()

	var providers []string
	for _, extractor := range eem.extractors {
		// Get provider ID from extractor (all extractors implement EmailExtractor)
		providers = append(providers, extractor.ProviderID())
	}

	eem.logger.DebugLog("[EmailExtractionManager] GetRegisteredProviders: %v", providers)
	return providers
}

// UserInfoURL returns the user info endpoint URL for Qwen
func (qe *QwenEmailExtractor) UserInfoURL() string {
	return qe.userInfoURL
}

// GeminiEmailExtractor implements EmailExtractor for Gemini
type GeminiEmailExtractor struct {
	userInfoURL string
	httpClient  *http.Client
	logger      logging.Logger
}

// NewGeminiEmailExtractor creates a new GeminiEmailExtractor
func NewGeminiEmailExtractor(httpClient *http.Client, logger logging.Logger) *GeminiEmailExtractor {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &GeminiEmailExtractor{
		userInfoURL: GeminiUserInfoURL,
		httpClient:  httpClient,
		logger:      logger,
	}
}

// ExtractEmail extracts email from the OAuth token response for Gemini
func (ge *GeminiEmailExtractor) ExtractEmail(ctx context.Context, tokenResponse map[string]interface{}, accessToken string) (string, error) {
	ge.logger.InfoLog("[GeminiEmailExtractor] Starting email extraction, tokenResponse keys: %v", getMapKeys(tokenResponse))

	// First try to extract email directly from token response
	if email, found := extractEmailFromTokenResponse(tokenResponse); found {
		ge.logger.InfoLog("[GeminiEmailExtractor] PATH 1: Email found in token response: %s", normalizeEmail(email))
		return normalizeEmail(email), nil
	}

	ge.logger.InfoLog("[GeminiEmailExtractor] PATH 2: Email not in token response, falling back to user info endpoint")

	// Check if access token is available
	if accessToken == "" {
		ge.logger.ErrorLog("[GeminiEmailExtractor] No access token provided for user info endpoint call")
		return "", fmt.Errorf("no access token available to fetch email from user info endpoint")
	}

	ge.logger.InfoLog("[GeminiEmailExtractor] Fetching email from user info endpoint: %s", ge.userInfoURL)
	email, err := fetchEmailFromUserInfo(ctx, ge.userInfoURL, accessToken, ge.httpClient, ge.logger)
	if err != nil {
		ge.logger.ErrorLog("[GeminiEmailExtractor] Failed to fetch email from user info endpoint: %v", err)
		return "", fmt.Errorf("failed to fetch email from user info endpoint: %w", err)
	}

	ge.logger.InfoLog("[GeminiEmailExtractor] PATH 2 SUCCESS: Email extracted from user info endpoint: %s", normalizeEmail(email))
	return normalizeEmail(email), nil
}

// getMapKeys returns the keys of a map as a string slice for logging
func getMapKeys(m map[string]interface{}) []string {
	if m == nil {
		return []string{}
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// ProviderID returns the provider identifier for Gemini
func (ge *GeminiEmailExtractor) ProviderID() string {
	return "gemini-cli"
}

// UserInfoURL returns the user info endpoint URL for Gemini
func (ge *GeminiEmailExtractor) UserInfoURL() string {
	return ge.userInfoURL
}

// KiroEmailExtractor implements EmailExtractor for Kiro
type KiroEmailExtractor struct {
	userInfoURL string
	httpClient  *http.Client
	logger      logging.Logger
}

// NewKiroEmailExtractor creates a new KiroEmailExtractor
func NewKiroEmailExtractor(httpClient *http.Client, logger logging.Logger) *KiroEmailExtractor {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &KiroEmailExtractor{
		userInfoURL: KiroSocialUserInfoURL, // Default to social auth endpoint
		httpClient:  httpClient,
		logger:      logger,
	}
}

// ExtractEmail extracts email from the OAuth token response for Kiro
func (ke *KiroEmailExtractor) ExtractEmail(ctx context.Context, tokenResponse map[string]interface{}, accessToken string) (string, error) {
	// First try to extract email directly from token response
	if email, found := extractEmailFromTokenResponse(tokenResponse); found {
		ke.logger.DebugLog("[KiroEmailExtractor] Email found in token response: %s", normalizeEmail(email))
		return normalizeEmail(email), nil
	}

	// Kiro uses AWS SSO which may not have a standard userinfo endpoint
	// Try the social auth endpoint first
	ke.logger.DebugLog("[KiroEmailExtractor] Email not in token response, fetching from social user info endpoint")
	email, err := fetchEmailFromUserInfo(ctx, ke.userInfoURL, accessToken, ke.httpClient, ke.logger)
	if err == nil && email != "" {
		return normalizeEmail(email), nil
	}

	// Try the identity endpoint as fallback
	ke.logger.DebugLog("[KiroEmailExtractor] Social endpoint failed, trying identity endpoint")
	email, err = fetchEmailFromUserInfo(ctx, KiroIdentityUserInfoURL, accessToken, ke.httpClient, ke.logger)
	if err != nil {
		return "", fmt.Errorf("failed to fetch email from Kiro user info endpoints: %w", err)
	}

	return normalizeEmail(email), nil
}

// ProviderID returns the provider identifier for Kiro
func (ke *KiroEmailExtractor) ProviderID() string {
	return "kiro"
}

// UserInfoURL returns the user info endpoint URL for Kiro
func (ke *KiroEmailExtractor) UserInfoURL() string {
	return ke.userInfoURL
}

// IFlowEmailExtractor implements EmailExtractor for iFlow
type IFlowEmailExtractor struct {
	userInfoURL string
	httpClient  *http.Client
	logger      logging.Logger
}

// NewIFlowEmailExtractor creates a new IFlowEmailExtractor
func NewIFlowEmailExtractor(httpClient *http.Client, logger logging.Logger) *IFlowEmailExtractor {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &IFlowEmailExtractor{
		userInfoURL: IFlowUserInfoURL,
		httpClient:  httpClient,
		logger:      logger,
	}
}

// ExtractEmail extracts email from the OAuth token response for iFlow
func (ife *IFlowEmailExtractor) ExtractEmail(ctx context.Context, tokenResponse map[string]interface{}, accessToken string) (string, error) {
	// First try to extract email directly from token response
	if email, found := extractEmailFromTokenResponse(tokenResponse); found {
		ife.logger.DebugLog("[IFlowEmailExtractor] Email found in token response: %s", normalizeEmail(email))
		return normalizeEmail(email), nil
	}

	// Fetch from iFlow user info endpoint
	// iFlow uses a query parameter for the access token
	ife.logger.DebugLog("[IFlowEmailExtractor] Email not in token response, fetching from user info endpoint")
	userInfoURL := fmt.Sprintf("%s?accessToken=%s", ife.userInfoURL, url.QueryEscape(accessToken))

	req, err := http.NewRequestWithContext(ctx, "GET", userInfoURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create user info request: %w", err)
	}

	req.Header.Set("Accept", "application/json")

	resp, err := ife.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send user info request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("user info request failed (status %d): %s", resp.StatusCode, string(body))
	}

	var userInfoResp map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&userInfoResp); err != nil {
		return "", fmt.Errorf("failed to decode user info response: %w", err)
	}

	if success, ok := userInfoResp["success"].(bool); !ok || !success {
		return "", fmt.Errorf("user info request unsuccessful")
	}

	data, ok := userInfoResp["data"].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("no data in user info response")
	}

	// Try email first, then phone as fallback
	var email string
	if e, ok := data["email"].(string); ok && e != "" {
		email = e
	} else if phone, ok := data["phone"].(string); ok && phone != "" {
		email = phone
	}

	if email == "" {
		return "", fmt.Errorf("no email or phone found in user info response")
	}

	ife.logger.DebugLog("[IFlowEmailExtractor] Email extracted from user info: %s", normalizeEmail(email))
	return normalizeEmail(email), nil
}

// ProviderID returns the provider identifier for iFlow
func (ife *IFlowEmailExtractor) ProviderID() string {
	return "iflow"
}

// UserInfoURL returns the user info endpoint URL for iFlow
func (ife *IFlowEmailExtractor) UserInfoURL() string {
	return ife.userInfoURL
}

// EmailExtractionManager manages email extraction for all providers
type EmailExtractionManager struct {
	extractors map[string]EmailExtractor // ProviderID -> EmailExtractor
	mu         sync.RWMutex
	logger     logging.Logger
}

// NewEmailExtractionManager creates a new EmailExtractionManager
func NewEmailExtractionManager(logger logging.Logger) *EmailExtractionManager {
	if logger == nil {
		logger = logging.NewLogger()
	}
	return &EmailExtractionManager{
		extractors: make(map[string]EmailExtractor),
		logger:     logger,
	}
}

// RegisterExtractor registers an email extractor for a provider
func (eem *EmailExtractionManager) RegisterExtractor(extractor EmailExtractor) {
	eem.mu.Lock()
	defer eem.mu.Unlock()

	eem.extractors[extractor.ProviderID()] = extractor
	eem.logger.InfoLog("[EmailExtractionManager] Registered extractor for provider: %s", extractor.ProviderID())
}

// ExtractEmail extracts email for a provider
func (eem *EmailExtractionManager) ExtractEmail(ctx context.Context, providerID string, tokenResponse map[string]interface{}, accessToken string) (string, error) {
	eem.mu.RLock()
	extractor, ok := eem.extractors[providerID]
	eem.mu.RUnlock()

	if !ok {
		return "", fmt.Errorf("no email extractor registered for provider: %s", providerID)
	}

	email, err := extractor.ExtractEmail(ctx, tokenResponse, accessToken)
	if err != nil {
		eem.logger.WarnLog("[EmailExtractionManager] Failed to extract email for %s: %v", providerID, err)
		return "", err
	}

	eem.logger.InfoLog("[EmailExtractionManager] Successfully extracted email for %s: %s", providerID, email)
	return email, nil
}

// GetExtractor returns the extractor for a provider
func (eem *EmailExtractionManager) GetExtractor(providerID string) (EmailExtractor, error) {
	eem.mu.RLock()
	defer eem.mu.RUnlock()

	extractor, ok := eem.extractors[providerID]
	if !ok {
		return nil, fmt.Errorf("no email extractor registered for provider: %s", providerID)
	}

	return extractor, nil
}

// extractEmailFromTokenResponse tries to extract email directly from token response
func extractEmailFromTokenResponse(tokenResponse map[string]interface{}) (string, bool) {
	// Try common field names
	emailFields := []string{"email", "user_email", "userEmail", "user.email"}

	for _, field := range emailFields {
		if email, ok := tokenResponse[field].(string); ok && email != "" {
			return email, true
		}
	}

	// Try nested in "data" field (common in some APIs)
	if data, ok := tokenResponse["data"].(map[string]interface{}); ok {
		for _, field := range emailFields {
			if email, ok := data[field].(string); ok && email != "" {
				return email, true
			}
		}
	}

	// Try nested in "user" field
	if user, ok := tokenResponse["user"].(map[string]interface{}); ok {
		for _, field := range emailFields {
			if email, ok := user[field].(string); ok && email != "" {
				return email, true
			}
		}
	}

	return "", false
}

// fetchEmailFromUserInfo fetches email from a user info endpoint with retry logic
func fetchEmailFromUserInfo(ctx context.Context, userInfoURL string, accessToken string, httpClient *http.Client, logger logging.Logger) (string, error) {
	const maxRetries = 3
	const initialRetryDelay = 500 * time.Millisecond

	var lastErr error

	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff
			delay := initialRetryDelay * time.Duration(1<<(attempt-1))
			logger.InfoLog("[fetchEmailFromUserInfo] Retry attempt %d/%d after %v delay", attempt+1, maxRetries, delay)
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return "", fmt.Errorf("context cancelled during retry delay: %w", ctx.Err())
			}
		}

		logger.InfoLog("[fetchEmailFromUserInfo] Attempt %d/%d - Request URL: %s", attempt+1, maxRetries, userInfoURL)
		logger.DebugLog("[fetchEmailFromUserInfo] Access token (first 20 chars): %s...", truncateString(accessToken, 20))

		req, err := http.NewRequestWithContext(ctx, "GET", userInfoURL, nil)
		if err != nil {
			lastErr = fmt.Errorf("failed to create user info request: %w", err)
			logger.ErrorLog("[fetchEmailFromUserInfo] %v", lastErr)
			continue
		}

		// Set Authorization header with Bearer token
		authHeader := "Bearer " + accessToken
		req.Header.Set("Authorization", authHeader)
		req.Header.Set("Accept", "application/json")

		logger.DebugLog("[fetchEmailFromUserInfo] Request headers - Authorization: %s, Accept: %s",
			truncateString(authHeader, 30), req.Header.Get("Accept"))

		resp, err := httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("failed to send user info request: %w", err)
			logger.ErrorLog("[fetchEmailFromUserInfo] %v", lastErr)
			// Retry on network errors
			continue
		}
		defer resp.Body.Close()

		logger.InfoLog("[fetchEmailFromUserInfo] Response status code: %d", resp.StatusCode)

		// Read response body for logging
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			lastErr = fmt.Errorf("failed to read response body: %w", err)
			logger.ErrorLog("[fetchEmailFromUserInfo] %v", lastErr)
			continue
		}

		bodyStr := string(body)
		logger.DebugLog("[fetchEmailFromUserInfo] Response body: %s", truncateString(bodyStr, 500))

		// Check for non-retryable errors (4xx except 429)
		if resp.StatusCode >= 400 && resp.StatusCode < 500 && resp.StatusCode != http.StatusTooManyRequests {
			lastErr = fmt.Errorf("user info request failed with non-retryable status %d: %s", resp.StatusCode, bodyStr)
			logger.ErrorLog("[fetchEmailFromUserInfo] %v", lastErr)
			return "", lastErr
		}

		// Check for retryable errors (5xx, 429, or network issues)
		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("user info request failed (status %d): %s", resp.StatusCode, bodyStr)
			logger.WarnLog("[fetchEmailFromUserInfo] %v", lastErr)
			continue // Retry for 5xx, 429
		}

		// Parse JSON response
		var userInfoResp map[string]interface{}
		if err := json.Unmarshal(body, &userInfoResp); err != nil {
			lastErr = fmt.Errorf("failed to decode user info response: %w", err)
			logger.ErrorLog("[fetchEmailFromUserInfo] %v", lastErr)
			// Don't retry JSON parsing errors
			return "", lastErr
		}

		logger.DebugLog("[fetchEmailFromUserInfo] Parsed response: %+v", userInfoResp)

		// Try various response structures
		email, found := extractEmailFromTokenResponse(userInfoResp)
		if found {
			logger.InfoLog("[fetchEmailFromUserInfo] Email extracted from response: %s", email)
			return email, nil
		}

		lastErr = fmt.Errorf("no email found in user info response")
		logger.WarnLog("[fetchEmailFromUserInfo] %v", lastErr)
		// Don't retry if we got a valid response but no email
		return "", lastErr
	}

	// All retries exhausted
	return "", fmt.Errorf("failed to fetch email from user info endpoint after %d attempts: %w", maxRetries, lastErr)
}

// truncateString truncates a string to the specified length, adding "..." if truncated
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// normalizeEmail normalizes email address (lowercase, trim)
func normalizeEmail(email string) string {
	if email == "" {
		return ""
	}
	email = strings.TrimSpace(strings.ToLower(email))

	// Basic validation - should not be empty after trim
	if email == "" {
		return ""
	}

	// Basic email format validation - should contain @
	// Note: We allow aliases (user@local format) for providers like Qwen
	// This is intentionally permissive to allow various identifier formats
	if !strings.Contains(email, "@") {
		return "" // Invalid format
	}

	// Split email into local and domain parts
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return "" // Invalid format
	}

	localPart := strings.TrimSpace(parts[0])
	domainPart := strings.TrimSpace(parts[1])

	// Both parts should be non-empty
	if localPart == "" || domainPart == "" {
		return ""
	}

	return email
}
