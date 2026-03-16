// Package config provides configuration management for the application.
package config

import (
	"net/http"
	"testing"
	"time"
)

// TestHTTPClientInterfaceSatisfaction verifies that *http.Client satisfies the HTTPClient interface.
func TestHTTPClientInterfaceSatisfaction(t *testing.T) {
	// Create a standard http.Client
	stdClient := &http.Client{
		Timeout: 30 * time.Second,
	}

	// Assign to HTTPClient interface - this will fail at compile time if *http.Client doesn't satisfy the interface
	var client HTTPClient = stdClient

	// Verify the client is not nil
	if client == nil {
		t.Error("Expected client to not be nil")
	}
}

// TestHTTPClientConfigDefaults verifies that DefaultConfig returns expected HTTPClientConfig values.
func TestHTTPClientConfigDefaults(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.HTTPClient.MaxIdleConns != 50 {
		t.Errorf("Expected MaxIdleConns to be 50, got %d", cfg.HTTPClient.MaxIdleConns)
	}

	if cfg.HTTPClient.MaxIdleConnsPerHost != 50 {
		t.Errorf("Expected MaxIdleConnsPerHost to be 50, got %d", cfg.HTTPClient.MaxIdleConnsPerHost)
	}

	if cfg.HTTPClient.IdleConnTimeoutSeconds != 180 {
		t.Errorf("Expected IdleConnTimeoutSeconds to be 180, got %d", cfg.HTTPClient.IdleConnTimeoutSeconds)
	}

	if cfg.HTTPClient.RequestTimeoutSeconds != 300 {
		t.Errorf("Expected RequestTimeoutSeconds to be 300, got %d", cfg.HTTPClient.RequestTimeoutSeconds)
	}

	if cfg.HTTPClient.StreamingTimeoutSeconds != 900 {
		t.Errorf("Expected StreamingTimeoutSeconds to be 900, got %d", cfg.HTTPClient.StreamingTimeoutSeconds)
	}

	if cfg.HTTPClient.ReadTimeoutSeconds != 45 {
		t.Errorf("Expected ReadTimeoutSeconds to be 45, got %d", cfg.HTTPClient.ReadTimeoutSeconds)
	}
}

// TestSharedHTTPClient verifies that SharedHTTPClient creates a client with proper configuration.
func TestSharedHTTPClient(t *testing.T) {
	cfg := DefaultConfig()
	client := cfg.SharedHTTPClient()

	if client == nil {
		t.Fatal("Expected client to not be nil")
	}

	expectedTimeout := time.Duration(cfg.HTTPClient.RequestTimeoutSeconds) * time.Second
	if client.Timeout != expectedTimeout {
		t.Errorf("Expected timeout to be %v, got %v", expectedTimeout, client.Timeout)
	}

	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatal("Expected transport to be *http.Transport")
	}

	if transport.MaxIdleConns != cfg.HTTPClient.MaxIdleConns {
		t.Errorf("Expected MaxIdleConns to be %d, got %d", cfg.HTTPClient.MaxIdleConns, transport.MaxIdleConns)
	}

	if transport.MaxIdleConnsPerHost != cfg.HTTPClient.MaxIdleConnsPerHost {
		t.Errorf("Expected MaxIdleConnsPerHost to be %d, got %d", cfg.HTTPClient.MaxIdleConnsPerHost, transport.MaxIdleConnsPerHost)
	}

	expectedIdleTimeout := time.Duration(cfg.HTTPClient.IdleConnTimeoutSeconds) * time.Second
	if transport.IdleConnTimeout != expectedIdleTimeout {
		t.Errorf("Expected IdleConnTimeout to be %v, got %v", expectedIdleTimeout, transport.IdleConnTimeout)
	}
}

// TestStreamingHTTPClient verifies that StreamingHTTPClient creates a client with proper configuration.
func TestStreamingHTTPClient(t *testing.T) {
	cfg := DefaultConfig()
	client := cfg.StreamingHTTPClient()

	if client == nil {
		t.Fatal("Expected client to not be nil")
	}

	expectedTimeout := time.Duration(cfg.HTTPClient.StreamingTimeoutSeconds) * time.Second
	if client.Timeout != expectedTimeout {
		t.Errorf("Expected timeout to be %v, got %v", expectedTimeout, client.Timeout)
	}
}

// TestHTTPClientInterfaceTypeAssertion verifies that we can type assert from HTTPClient to *http.Client.
func TestHTTPClientInterfaceTypeAssertion(t *testing.T) {
	stdClient := &http.Client{
		Timeout: 30 * time.Second,
	}

	var client HTTPClient = stdClient

	// Type assert back to *http.Client
	assertedClient, ok := client.(*http.Client)
	if !ok {
		t.Error("Failed to type assert HTTPClient to *http.Client")
	}

	if assertedClient != stdClient {
		t.Error("Type assertion returned a different client than the original")
	}
}

// TestHTTPClientConfigCustomValues verifies that custom HTTPClientConfig values are preserved.
func TestHTTPClientConfigCustomValues(t *testing.T) {
	cfg := &Config{
		HTTPClient: HTTPClientConfig{
			MaxIdleConns:            100,
			MaxIdleConnsPerHost:     20,
			IdleConnTimeoutSeconds:  60,
			RequestTimeoutSeconds:   120,
			StreamingTimeoutSeconds: 600,
			ReadTimeoutSeconds:      30,
		},
	}

	client := cfg.SharedHTTPClient()

	if client == nil {
		t.Fatal("Expected client to not be nil")
	}

	expectedTimeout := 120 * time.Second
	if client.Timeout != expectedTimeout {
		t.Errorf("Expected timeout to be %v, got %v", expectedTimeout, client.Timeout)
	}

	transport := client.Transport.(*http.Transport)
	if transport.MaxIdleConns != 100 {
		t.Errorf("Expected MaxIdleConns to be 100, got %d", transport.MaxIdleConns)
	}

	if transport.MaxIdleConnsPerHost != 20 {
		t.Errorf("Expected MaxIdleConnsPerHost to be 20, got %d", transport.MaxIdleConnsPerHost)
	}

	expectedIdleTimeout := 60 * time.Second
	if transport.IdleConnTimeout != expectedIdleTimeout {
		t.Errorf("Expected IdleConnTimeout to be %v, got %v", expectedIdleTimeout, transport.IdleConnTimeout)
	}
}

// TestHTTPClientNilTransport verifies that a client with nil transport still satisfies the interface.
func TestHTTPClientNilTransport(t *testing.T) {
	client := &http.Client{
		Timeout: 30 * time.Second,
		// Transport is nil, will use http.DefaultTransport
	}

	var httpClient HTTPClient = client

	if httpClient == nil {
		t.Error("Expected httpClient to not be nil")
	}
}
