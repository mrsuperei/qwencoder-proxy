package config

import (
	"net/http"
	"time"
)

// HTTPClient is an interface that abstracts HTTP client operations.
// This interface enables dependency inversion, testability, and allows for
// proxy-aware implementations.
//
// The standard library's *http.Client satisfies this interface, making it
// easy to adopt throughout the codebase without breaking existing code.
//
// Example usage:
//
//	client := &http.Client{Timeout: 30 * time.Second}
//	var httpClient HTTPClient = client // *http.Client satisfies HTTPClient
//
// Future implementations may include:
// - StandardHTTPClient: Wraps *http.Client with additional functionality
// - RetryHTTPClient: Adds automatic retry logic for failed requests
// - LoggingHTTPClient: Logs request/response details for debugging
// - MetricsHTTPClient: Tracks request metrics and statistics
type HTTPClient interface {
	// Do sends an HTTP request and returns an HTTP response.
	// This method matches the signature of http.Client.Do().
	Do(req *http.Request) (*http.Response, error)
}

// ServerConfig holds server-related configuration
type ServerConfig struct {
	Port string
}

// HTTPClientConfig holds HTTP client configuration
type HTTPClientConfig struct {
	MaxIdleConns            int
	MaxIdleConnsPerHost     int
	IdleConnTimeoutSeconds  int
	RequestTimeoutSeconds   int
	StreamingTimeoutSeconds int
	ReadTimeoutSeconds      int
}

// LoggingConfig holds logging-related configuration
type LoggingConfig struct {
	IsDebugMode bool
}

// OAuthServerConfig holds OAuth REST API server configuration
type OAuthServerConfig struct {
	Port            string        // OAuth server port (default: 8080)
	CallbackBaseURL string        // Base URL for callbacks (e.g., "http://localhost:8080")
	StateTTL        time.Duration // OAuth state TTL (default: 10m)
	DeviceCodeTTL   time.Duration // Device code TTL (default: 15m)
	EnableCORS      bool          // Enable CORS (default: false)
	AllowedOrigins  []string      // CORS allowed origins (default: ["*"])
}

// StorageConfig holds storage backend configuration
type StorageConfig struct {
	DBPath string // Path to SQLite database
}

// SQLiteRetryConfig holds SQLite retry configuration
type SQLiteRetryConfig struct {
	Enabled     bool // Enable retry logic (default: true)
	MaxRetries  int  // Maximum number of retry attempts (default: 10)
	BaseDelayMs int  // Base delay in milliseconds (default: 50)
	MaxDelayMs  int  // Maximum delay in milliseconds (default: 2000)
}

// DefaultSQLiteRetryConfig returns default SQLite retry configuration
func DefaultSQLiteRetryConfig() *SQLiteRetryConfig {
	return &SQLiteRetryConfig{
		Enabled:     true,
		MaxRetries:  10,
		BaseDelayMs: 50,
		MaxDelayMs:  2000,
	}
}

// Default storage configuration constants
const (
	DefaultStoragePath = ".credentials/tokens.db"
)

// RateLimitConfig holds rate limiting configuration
type RateLimitConfig struct {
	// Async usage recording configuration
	AsyncEnabled     bool          // Enable async recording (default: false)
	AsyncWorkerCount int           // Number of worker goroutines (default: 5)
	AsyncQueueSize   int           // Channel buffer size (default: 1000)
	AsyncRetryLimit  int           // Max retries per job (default: 3)
	AsyncRetryDelay  time.Duration // Delay between retries (default: 100ms)

	// Cache configuration
	EnableCache           bool          // Enable in-memory caching (default: true)
	CacheProviderTTL      time.Duration // Provider metrics TTL (default: 5s)
	CacheTokenTTL         time.Duration // Token metrics TTL (default: 5s)
	CacheMaxProviders     int           // Max provider entries (default: 100)
	CacheMaxTokens        int           // Max token entries (default: 1000)
	CacheRefreshBeforeTTL time.Duration // Refresh before expiry (default: 1s)

	// Database notification configuration
	EnableDBNotification bool          // Enable database change monitoring (default: false)
	DBPollingInterval    time.Duration // Interval between database checks (default: 5s)
}

// DefaultRateLimitConfig returns default rate limit configuration
func DefaultRateLimitConfig() *RateLimitConfig {
	return &RateLimitConfig{
		AsyncEnabled:          false,
		AsyncWorkerCount:      5,
		AsyncQueueSize:        1000,
		AsyncRetryLimit:       3,
		AsyncRetryDelay:       100 * time.Millisecond,
		EnableCache:           true,
		CacheProviderTTL:      5 * time.Second,
		CacheTokenTTL:         5 * time.Second,
		CacheMaxProviders:     100,
		CacheMaxTokens:        1000,
		CacheRefreshBeforeTTL: 1 * time.Second,
		EnableDBNotification:  false,
		DBPollingInterval:     5 * time.Second,
	}
}

// Config holds all configuration for application
type Config struct {
	Server      ServerConfig
	HTTPClient  HTTPClientConfig
	Logging     LoggingConfig
	OAuthServer OAuthServerConfig
	Storage     StorageConfig
	SQLiteRetry SQLiteRetryConfig
	RateLimit   RateLimitConfig
}

// DefaultConfig returns the default configuration
func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Port: "8143",
		},
		HTTPClient: HTTPClientConfig{
			MaxIdleConns:            50,
			MaxIdleConnsPerHost:     50,
			IdleConnTimeoutSeconds:  180,
			RequestTimeoutSeconds:   300,
			StreamingTimeoutSeconds: 900, // Using the value from proxy/client.go as it's longer
			ReadTimeoutSeconds:      45,
		},
		Logging: LoggingConfig{
			IsDebugMode: false,
		},
		OAuthServer: OAuthServerConfig{
			Port:            "8143",
			CallbackBaseURL: "http://localhost:8143",
			StateTTL:        10 * time.Minute,
			DeviceCodeTTL:   15 * time.Minute,
			EnableCORS:      false,
			AllowedOrigins:  []string{"*"},
		},
		Storage: StorageConfig{
			DBPath: DefaultStoragePath,
		},
		SQLiteRetry: *DefaultSQLiteRetryConfig(),
		RateLimit:   *DefaultRateLimitConfig(),
	}
}

// SharedHTTPClient creates and returns a shared HTTP client with the configured settings
func (c *Config) SharedHTTPClient() *http.Client {
	transport := &http.Transport{
		MaxIdleConns:        c.HTTPClient.MaxIdleConns,
		MaxIdleConnsPerHost: c.HTTPClient.MaxIdleConnsPerHost,
		IdleConnTimeout:     time.Duration(c.HTTPClient.IdleConnTimeoutSeconds) * time.Second,
	}

	return &http.Client{
		Timeout:   time.Duration(c.HTTPClient.RequestTimeoutSeconds) * time.Second,
		Transport: transport,
	}
}

// StreamingHTTPClient creates and returns an HTTP client for streaming with the configured settings
func (c *Config) StreamingHTTPClient() *http.Client {
	return &http.Client{
		Timeout: time.Duration(c.HTTPClient.StreamingTimeoutSeconds) * time.Second,
	}
}
