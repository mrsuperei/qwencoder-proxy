// Package auth provides authentication and token management functionality.
// This file contains proxy configuration data structures for per-token proxy support.
package auth

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ProxyType represents the type of proxy protocol to use.
type ProxyType string

const (
	// ProxyTypeNone indicates no proxy should be used (direct connection).
	ProxyTypeNone ProxyType = "none"
	// ProxyTypeHTTP indicates an HTTP proxy.
	ProxyTypeHTTP ProxyType = "http"
	// ProxyTypeHTTPS indicates an HTTPS proxy.
	ProxyTypeHTTPS ProxyType = "https"
	// ProxyTypeSOCKS5 indicates a SOCKS5 proxy.
	ProxyTypeSOCKS5 ProxyType = "socks5"
)

// Validate checks if the ProxyType is valid.
func (pt ProxyType) Validate() error {
	switch pt {
	case ProxyTypeNone, ProxyTypeHTTP, ProxyTypeHTTPS, ProxyTypeSOCKS5:
		return nil
	default:
		return fmt.Errorf("invalid proxy type: %s", pt)
	}
}

// String returns the string representation of ProxyType.
func (pt ProxyType) String() string {
	return string(pt)
}

// ProxyConfig represents the configuration for a proxy server.
// This is a value object that is immutable after creation.
type ProxyConfig struct {
	Type     ProxyType `json:"type"`               // Type of proxy (none, http, https, socks5)
	Host     string    `json:"host"`               // Proxy server hostname or IP address
	Port     int       `json:"port"`               // Proxy server port number
	Username string    `json:"username,omitempty"` // Optional username for authentication
	Password string    `json:"password,omitempty"` // Optional password for authentication
	Enabled  bool      `json:"enabled"`            // Whether this proxy configuration is active
}

// Validate checks if the ProxyConfig is valid and ready to use.
func (pc *ProxyConfig) Validate() error {
	if pc == nil {
		return nil // nil config is valid (no proxy)
	}

	// Validate proxy type
	if err := pc.Type.Validate(); err != nil {
		return err
	}

	// For "none" type, other fields can be empty
	if pc.Type == ProxyTypeNone {
		return nil
	}

	// Validate host is not empty
	if pc.Host == "" {
		return fmt.Errorf("proxy host cannot be empty for type %s", pc.Type)
	}

	// Validate port is in valid range
	if pc.Port <= 0 || pc.Port > 65535 {
		return fmt.Errorf("proxy port must be between 1 and 65535, got: %d", pc.Port)
	}

	// Validate that if username is provided, password should also be provided (and vice versa)
	if (pc.Username != "" && pc.Password == "") || (pc.Username == "" && pc.Password != "") {
		return fmt.Errorf("both username and password must be provided together, or neither")
	}

	return nil
}

// String returns a masked string representation of ProxyConfig for logging.
// Passwords are masked for security.
func (pc *ProxyConfig) String() string {
	if pc == nil {
		return "ProxyConfig(nil)"
	}

	var password string
	if pc.Password != "" {
		password = "****"
	}

	return fmt.Sprintf("ProxyConfig{Type: %s, Host: %s, Port: %d, Username: %s, Password: %s, Enabled: %t}",
		pc.Type, pc.Host, pc.Port, pc.Username, password, pc.Enabled)
}

// MaskedPassword returns the password masked with asterisks, or empty string if no password.
func (pc *ProxyConfig) MaskedPassword() string {
	if pc == nil || pc.Password == "" {
		return ""
	}
	return strings.Repeat("*", len(pc.Password))
}

// ProxyHealth represents the health status of a proxy connection.
type ProxyHealth struct {
	LastCheck           int64   `json:"last_check"`           // Timestamp of last health check
	IsHealthy           bool    `json:"is_healthy"`           // Current health status
	LastError           string  `json:"last_error,omitempty"` // Optional error message from last failure
	ConsecutiveFailures int     `json:"consecutive_failures"` // Count of consecutive failures
	AverageLatencyMs    int     `json:"average_latency_ms"`   // Average latency in milliseconds
	HealthScore         float64 `json:"health_score"`         // Health score (0.0-1.0)
}

// ProxyConfigKey is used as a cache key for proxy-configured HTTP clients.
// The key excludes credentials for security reasons - clients with the same
// proxy server configuration but different credentials will share the same client.
type ProxyConfigKey struct {
	Type ProxyType
	Host string
	Port int
}

// String returns the string representation of ProxyConfigKey for use as a map key.
func (pck ProxyConfigKey) String() string {
	return fmt.Sprintf("%s:%s:%d", pck.Type, pck.Host, pck.Port)
}

// MarshalJSON implements custom JSON marshaling for ProxyConfigKey.
func (pck ProxyConfigKey) MarshalJSON() ([]byte, error) {
	return json.Marshal(pck.String())
}

// UnmarshalJSON implements custom JSON unmarshaling for ProxyConfigKey.
func (pck *ProxyConfigKey) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}

	// Parse the string in format "type:host:port"
	parts := strings.Split(s, ":")
	if len(parts) != 3 {
		return fmt.Errorf("invalid ProxyConfigKey format: %s", s)
	}

	pck.Type = ProxyType(parts[0])
	pck.Host = parts[1]
	if _, err := fmt.Sscanf(parts[2], "%d", &pck.Port); err != nil {
		return fmt.Errorf("invalid port in ProxyConfigKey: %s", parts[2])
	}

	return nil
}

// NewProxyConfigKey creates a new ProxyConfigKey from a ProxyConfig.
// Returns nil if ProxyConfig is nil or has type "none".
func NewProxyConfigKey(pc *ProxyConfig) *ProxyConfigKey {
	if pc == nil || pc.Type == ProxyTypeNone {
		return nil
	}

	return &ProxyConfigKey{
		Type: pc.Type,
		Host: pc.Host,
		Port: pc.Port,
	}
}
