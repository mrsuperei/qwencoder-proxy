// Package auth provides authentication and token management functionality.
// This file contains unit tests for the proxy connection tester.
package auth

import (
	"context"
	"testing"
	"time"

	"github.com/sunbankio/qwencoder-proxy/logging"
)

func TestNewProxyTester(t *testing.T) {
	logger := &logging.Logger{}
	tester := NewProxyTester(logger)

	if tester == nil {
		t.Fatal("NewProxyTester returned nil")
	}

	if tester.logger != logger {
		t.Error("NewProxyTester did not set logger correctly")
	}
}

func TestTestConnection_NilConfig(t *testing.T) {
	logger := &logging.Logger{}
	tester := NewProxyTester(logger)

	result, err := tester.TestConnection(context.Background(), nil)

	if err == nil {
		t.Error("Expected error for nil config, got nil")
	}

	if result == nil {
		t.Error("Expected result object, got nil")
	}

	if result.Success {
		t.Error("Expected success to be false for nil config")
	}

	if result.Error == "" {
		t.Error("Expected error message for nil config, got empty string")
	}
}

func TestTestConnection_NoneType(t *testing.T) {
	logger := &logging.Logger{}
	tester := NewProxyTester(logger)

	config := &ProxyConfig{
		Type: ProxyTypeNone,
	}

	result, err := tester.TestConnection(context.Background(), config)

	if err == nil {
		t.Error("Expected error for 'none' type, got nil")
	}

	if result.Success {
		t.Error("Expected success to be false for 'none' type")
	}
}

func TestTestConnection_InvalidConfig(t *testing.T) {
	logger := &logging.Logger{}
	tester := NewProxyTester(logger)

	tests := []struct {
		name    string
		config  *ProxyConfig
		wantErr bool
	}{
		{
			name: "empty host",
			config: &ProxyConfig{
				Type: ProxyTypeHTTP,
				Host: "",
				Port: 8080,
			},
			wantErr: true,
		},
		{
			name: "invalid port (0)",
			config: &ProxyConfig{
				Type: ProxyTypeHTTP,
				Host: "proxy.example.com",
				Port: 0,
			},
			wantErr: true,
		},
		{
			name: "invalid port (negative)",
			config: &ProxyConfig{
				Type: ProxyTypeHTTP,
				Host: "proxy.example.com",
				Port: -1,
			},
			wantErr: true,
		},
		{
			name: "invalid port (too large)",
			config: &ProxyConfig{
				Type: ProxyTypeHTTP,
				Host: "proxy.example.com",
				Port: 70000,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tester.TestConnection(context.Background(), tt.config)

			if tt.wantErr && err == nil {
				t.Errorf("Expected error, got nil")
			}

			if result.Success {
				t.Error("Expected success to be false for invalid config")
			}
		})
	}
}

func TestTestConnection_ValidConfigButUnreachable(t *testing.T) {
	logger := &logging.Logger{}
	tester := NewProxyTester(logger)

	// Use an unreachable proxy configuration
	config := &ProxyConfig{
		Type: ProxyTypeHTTP,
		Host: "192.0.2.1", // TEST-NET-1, reserved for documentation (unreachable)
		Port: 8080,
	}

	// Set a short timeout for the test
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	result, err := tester.TestConnection(ctx, config)

	// Should not return an error (the error should be in the result)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	// Should fail to connect
	if result.Success {
		t.Error("Expected connection to fail for unreachable proxy")
	}

	// Should have an error message
	if result.Error == "" {
		t.Error("Expected error message for unreachable proxy, got empty string")
	}

	// Should have a tested timestamp
	if result.TestedAt == 0 {
		t.Error("Expected tested timestamp, got 0")
	}
}

func TestTestConnection_ContextTimeout(t *testing.T) {
	logger := &logging.Logger{}
	tester := NewProxyTester(logger)

	// Use a proxy that will likely timeout
	config := &ProxyConfig{
		Type: ProxyTypeHTTP,
		Host: "10.255.255.1", // Unreachable IP that will cause timeout
		Port: 8080,
	}

	// Set a very short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	result, err := tester.TestConnection(ctx, config)

	// Should not return an error (the error should be in the result)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	// Should fail
	if result.Success {
		t.Error("Expected connection to fail due to timeout")
	}

	// Should have an error message
	if result.Error == "" {
		t.Error("Expected error message for timeout, got empty string")
	}
}

func TestCreateHTTPProxyClient(t *testing.T) {
	logger := &logging.Logger{}
	tester := NewProxyTester(logger)

	tests := []struct {
		name    string
		config  *ProxyConfig
		wantErr bool
	}{
		{
			name: "valid HTTP proxy without auth",
			config: &ProxyConfig{
				Type: ProxyTypeHTTP,
				Host: "proxy.example.com",
				Port: 8080,
			},
			wantErr: false,
		},
		{
			name: "valid HTTPS proxy without auth",
			config: &ProxyConfig{
				Type: ProxyTypeHTTPS,
				Host: "proxy.example.com",
				Port: 8443,
			},
			wantErr: false,
		},
		{
			name: "valid HTTP proxy with auth",
			config: &ProxyConfig{
				Type:     ProxyTypeHTTP,
				Host:     "proxy.example.com",
				Port:     8080,
				Username: "user",
				Password: "pass",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := tester.createProxyClient(tt.config)

			if tt.wantErr && err == nil {
				t.Error("Expected error, got nil")
			}

			if !tt.wantErr {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}

				if client == nil {
					t.Error("Expected client, got nil")
				}
			}
		})
	}
}

func TestCreateSOCKS5Client(t *testing.T) {
	logger := &logging.Logger{}
	tester := NewProxyTester(logger)

	tests := []struct {
		name    string
		config  *ProxyConfig
		wantErr bool
	}{
		{
			name: "valid SOCKS5 proxy without auth",
			config: &ProxyConfig{
				Type: ProxyTypeSOCKS5,
				Host: "proxy.example.com",
				Port: 1080,
			},
			wantErr: false,
		},
		{
			name: "valid SOCKS5 proxy with auth",
			config: &ProxyConfig{
				Type:     ProxyTypeSOCKS5,
				Host:     "proxy.example.com",
				Port:     1080,
				Username: "user",
				Password: "pass",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := tester.createProxyClient(tt.config)

			if tt.wantErr && err == nil {
				t.Error("Expected error, got nil")
			}

			if !tt.wantErr {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}

				if client == nil {
					t.Error("Expected client, got nil")
				}
			}
		})
	}
}

func TestCreateProxyClient_UnsupportedType(t *testing.T) {
	logger := &logging.Logger{}
	tester := NewProxyTester(logger)

	config := &ProxyConfig{
		Type: "unsupported",
		Host: "proxy.example.com",
		Port: 8080,
	}

	client, err := tester.createProxyClient(config)

	if err == nil {
		t.Error("Expected error for unsupported proxy type, got nil")
	}

	if client != nil {
		t.Error("Expected nil client for unsupported proxy type")
	}
}
