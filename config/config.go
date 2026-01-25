package config

import (
	"net/http"
	"time"
)

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

// Config holds all configuration for application
type Config struct {
	Server      ServerConfig
	HTTPClient  HTTPClientConfig
	Logging     LoggingConfig
	OAuthServer OAuthServerConfig
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
