// Package auth provides authentication and token management functionality.
// This file contains proxy connection testing functionality.
package token

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/sunbankio/qwencoder-proxy/internal/logging"
	"golang.org/x/net/proxy"
)

// ProxyTester handles testing proxy connections.
type ProxyTester struct {
	logger logging.Logger
}

// NewProxyTester creates a new ProxyTester.
func NewProxyTester(logger logging.Logger) *ProxyTester {
	return &ProxyTester{logger: logger}
}

// TestResult represents the result of a proxy connection test.
type TestResult struct {
	Success   bool   `json:"success"`
	LatencyMs int    `json:"latency_ms"`
	Error     string `json:"error,omitempty"`
	TestedAt  int64  `json:"tested_at"`
}

// TestConnection tests if a proxy connection works.
// It makes a simple HTTP request through the proxy to verify connectivity.
func (pt *ProxyTester) TestConnection(ctx context.Context, proxyConfig *ProxyConfig) (*TestResult, error) {
	result := &TestResult{
		TestedAt: time.Now().Unix(),
	}

	// Validate proxy configuration
	if proxyConfig == nil || proxyConfig.Type == ProxyTypeNone {
		return result, fmt.Errorf("no proxy configuration provided")
	}

	if err := proxyConfig.Validate(); err != nil {
		return result, fmt.Errorf("invalid proxy configuration: %w", err)
	}

	// Create HTTP client with proxy configuration
	client, err := pt.createProxyClient(proxyConfig)
	if err != nil {
		result.Error = fmt.Sprintf("failed to create proxy client: %v", err)
		return result, nil
	}

	// Set timeout for the test (30 seconds for proxy connections)
	testCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// Use a reliable test endpoint
	testURL := "https://www.google.com"

	// Measure latency
	startTime := time.Now()
	req, err := http.NewRequestWithContext(testCtx, "GET", testURL, nil)
	if err != nil {
		result.Error = fmt.Sprintf("failed to create request: %v", err)
		return result, nil
	}

	// Set timeout for the request (same as context timeout)
	client.Timeout = 30 * time.Second

	pt.logger.InfoLog("[ProxyTester] Testing connection - Type: %s, Host: %s:%d, URL: %s",
		proxyConfig.Type, proxyConfig.Host, proxyConfig.Port, testURL)

	resp, err := client.Do(req)
	if err != nil {
		// Provide more helpful error messages
		errStr := err.Error()
		result.Error = formatProxyError(errStr)
		pt.logger.WarnLog("[ProxyTester] Connection failed - %s:%d - %s",
			proxyConfig.Host, proxyConfig.Port, result.Error)
		return result, nil
	}
	defer resp.Body.Close()

	latency := time.Since(startTime)
	result.LatencyMs = int(latency.Milliseconds())

	// Check if we got a successful response
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		result.Success = true
		pt.logger.InfoLog("[ProxyTester] Proxy connection test successful - Type: %s, Host: %s:%d, Latency: %dms",
			proxyConfig.Type, proxyConfig.Host, proxyConfig.Port, result.LatencyMs)
	} else {
		result.Error = fmt.Sprintf("unexpected status code: %d", resp.StatusCode)
	}

	return result, nil
}

// formatProxyError provides more helpful error messages for common proxy connection issues.
func formatProxyError(errStr string) string {
	// Check for common error patterns
	lowerErr := strings.ToLower(errStr)
	switch {
	case strings.Contains(lowerErr, "timeout") || strings.Contains(lowerErr, "deadline exceeded"):
		return "Connection timed out - The proxy is not responding or is unreachable. Check if the proxy address and port are correct, and ensure the proxy server is running."
	case strings.Contains(lowerErr, "connection refused"):
		return "Connection refused - The proxy server is not running or is blocking connections. Verify the proxy is online and accessible."
	case strings.Contains(lowerErr, "no such host") || strings.Contains(lowerErr, "no address associated with hostname"):
		return "DNS resolution failed - The proxy hostname cannot be found. Check if the proxy address is correct."
	case strings.Contains(lowerErr, "proxy authentication") || strings.Contains(lowerErr, "407"):
		return "Authentication failed - The proxy requires authentication. Check if username and password are correct."
	case strings.Contains(lowerErr, "certificate") || strings.Contains(lowerErr, "tls") || strings.Contains(lowerErr, "x509"):
		return "TLS/SSL error - There may be an issue with the proxy's SSL certificate or HTTPS configuration."
	default:
		return errStr
	}
}

// createProxyClient creates an HTTP client configured with the given proxy settings.
func (pt *ProxyTester) createProxyClient(proxyConfig *ProxyConfig) (*http.Client, error) {
	switch proxyConfig.Type {
	case ProxyTypeHTTP, ProxyTypeHTTPS:
		return pt.createHTTPProxyClient(proxyConfig)
	case ProxyTypeSOCKS5:
		return pt.createSOCKS5Client(proxyConfig)
	default:
		return nil, fmt.Errorf("unsupported proxy type: %s", proxyConfig.Type)
	}
}

// createHTTPProxyClient creates an HTTP client configured for HTTP/HTTPS proxy.
func (pt *ProxyTester) createHTTPProxyClient(proxyConfig *ProxyConfig) (*http.Client, error) {
	proxyURL, err := url.Parse(fmt.Sprintf("%s://%s:%d",
		proxyConfig.Type, proxyConfig.Host, proxyConfig.Port))
	if err != nil {
		return nil, fmt.Errorf("failed to parse proxy URL: %w", err)
	}

	// Add credentials if provided
	if proxyConfig.Username != "" && proxyConfig.Password != "" {
		proxyURL.User = url.UserPassword(proxyConfig.Username, proxyConfig.Password)
	}

	return &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyURL(proxyURL),
		},
	}, nil
}

// createSOCKS5Client creates an HTTP client configured for SOCKS5 proxy.
func (pt *ProxyTester) createSOCKS5Client(proxyConfig *ProxyConfig) (*http.Client, error) {
	auth := &proxy.Auth{}
	if proxyConfig.Username != "" && proxyConfig.Password != "" {
		auth.User = proxyConfig.Username
		auth.Password = proxyConfig.Password
	}

	proxyAddr := fmt.Sprintf("%s:%d", proxyConfig.Host, proxyConfig.Port)
	dialer, err := proxy.SOCKS5("tcp", proxyAddr, auth, proxy.Direct)
	if err != nil {
		return nil, fmt.Errorf("failed to create SOCKS5 dialer: %w", err)
	}

	return &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return dialer.Dial(network, addr)
			},
		},
	}, nil
}
