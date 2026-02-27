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

// Default storage configuration constants
const (
	DefaultStoragePath = ".credentials/tokens.db"
)

// Config holds all configuration for application
type Config struct {
	Server      ServerConfig
	HTTPClient  HTTPClientConfig
	Logging     LoggingConfig
	OAuthServer OAuthServerConfig
	Storage     StorageConfig
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
