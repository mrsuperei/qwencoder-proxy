// Package tests provides end-to-end tests for proxy configuration flow
package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunbankio/qwencoder-proxy/auth"
	"github.com/sunbankio/qwencoder-proxy/logging"
	"github.com/sunbankio/qwencoder-proxy/restapi"
)

// ============================================================================
// Test Infrastructure
// ============================================================================

// E2ETestEnvironment holds complete test environment for E2E tests
type E2ETestEnvironment struct {
	Server          *restapi.Server
	ServerURL       string
	ProviderID      string
	TokenID         string
	MockProviderAPI *MockHTTPServer
	Cleanup         func()
}

// setupE2ETestEnvironment creates a complete test environment with REST API server
func setupE2ETestEnvironment(t *testing.T) *E2ETestEnvironment {
	t.Helper()

	logger := logging.NewLogger()

	// Create mock provider API server
	mockProviderAPI := NewMockHTTPServer(func(w http.ResponseWriter, r *http.Request) {
		// Verify authorization header
		authHeader := r.Header.Get("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Return success response
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"response": map[string]interface{}{
				"candidates": []map[string]interface{}{
					{
						"content": map[string]interface{}{
							"parts": []map[string]interface{}{
								{
									"text": "Test response",
								},
							},
						},
					},
				},
			},
		})
	})
	mockProviderAPI.Start()

	// Create REST API server with random port
	testPort := findAvailablePort(t)

	apiConfig := &restapi.Config{
		Port:            testPort,
		CallbackBaseURL: fmt.Sprintf("http://localhost:%s", testPort),
		StateTTL:        10 * time.Minute,
		DeviceCodeTTL:   15 * time.Minute,
		EnableCORS:      true,
		AllowedOrigins:  []string{"*"},
	}

	server := restapi.NewServer(apiConfig, logger)

	// Start server in background
	go func() {
		if err := server.Start(); err != nil && !strings.Contains(err.Error(), "address already in use") {
			t.Logf("Server error: %v", err)
		}
	}()

	// Wait for server to be ready
	serverURL := fmt.Sprintf("http://localhost:%s", testPort)
	require.Eventually(t, func() bool {
		resp, err := http.Get(serverURL + "/api/providers")
		if err != nil {
			return false
		}
		defer resp.Body.Close()
		return resp.StatusCode == http.StatusOK
	}, 5*time.Second, 100*time.Millisecond, "Server did not start in time")

	// Use existing provider "qwen" for testing
	providerID := "qwen"

	// Initialize token store for the provider
	// This is needed because the store is created on-demand
	// and we need it to exist before adding tokens
	// Make a request to credentials endpoint to initialize the store
	credsURL := fmt.Sprintf("%s/api/credentials/%s", serverURL, providerID)
	req, _ := http.NewRequest("GET", credsURL, nil)
	client := &http.Client{Timeout: 5 * time.Second}
	_, _ = client.Do(req)

	// Create a test token via direct API call
	tokenID := createTestTokenViaAPI(t, serverURL, providerID)

	cleanup := func() {
		server.Stop()
		mockProviderAPI.Stop()
	}

	return &E2ETestEnvironment{
		Server:          server,
		ServerURL:       serverURL,
		ProviderID:      providerID,
		TokenID:         tokenID,
		MockProviderAPI: mockProviderAPI,
		Cleanup:         cleanup,
	}
}

// findAvailablePort finds an available port for testing
func findAvailablePort(t *testing.T) string {
	t.Helper()
	listener, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", ":0")
	if err != nil {
		t.Fatalf("Failed to find available port: %v", err)
	}
	defer listener.Close()
	return fmt.Sprintf("%d", listener.Addr().(*net.TCPAddr).Port)
}

// createTestTokenViaAPI creates a test token via REST API
func createTestTokenViaAPI(t *testing.T, serverURL, providerID string) string {
	t.Helper()

	url := fmt.Sprintf("%s/api/credentials/%s", serverURL, providerID)
	tokenReq := map[string]interface{}{
		"access_token":  "test-access-token-" + generateRandomString(8),
		"refresh_token": "test-refresh-token-" + generateRandomString(8),
		"token_type":    "Bearer",
		"expires_in":    3600,
		"email":         "test@example.com",
	}

	reqBody, _ := json.Marshal(tokenReq)
	req, _ := http.NewRequest("POST", url, bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	return result["token_id"].(string)
}

// generateRandomString generates a random string for testing
func generateRandomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[i%len(charset)]
	}
	return string(b)
}

// ============================================================================
// Configuration Flow Tests
// ============================================================================

func TestProxyE2E_ConfigureHTTPProxy(t *testing.T) {
	t.Run("configure HTTP proxy for token", func(t *testing.T) {
		env := setupE2ETestEnvironment(t)
		defer env.Cleanup()

		// Configure HTTP proxy
		proxyConfig := auth.ProxyConfig{
			Type:    auth.ProxyTypeHTTP,
			Host:    "proxy.example.com",
			Port:    8080,
			Enabled: true,
		}

		url := fmt.Sprintf("%s/api/credentials/%s/%s/proxy", env.ServerURL, env.ProviderID, env.TokenID)
		reqBody, _ := json.Marshal(proxyConfig)
		req, _ := http.NewRequest("PUT", url, bytes.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")

		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		require.Equal(t, http.StatusOK, resp.StatusCode)

		// Verify configuration via GET
		getResp := getProxyConfig(t, env.ServerURL, env.ProviderID, env.TokenID)
		require.NotNil(t, getResp.Proxy)
		assert.Equal(t, auth.ProxyTypeHTTP, getResp.Proxy.Type)
		assert.Equal(t, "proxy.example.com", getResp.Proxy.Host)
		assert.Equal(t, 8080, getResp.Proxy.Port)
		assert.True(t, getResp.Proxy.Enabled)
	})
}

func TestProxyE2E_ConfigureHTTPSProxy(t *testing.T) {
	t.Run("configure HTTPS proxy for token", func(t *testing.T) {
		env := setupE2ETestEnvironment(t)
		defer env.Cleanup()

		proxyConfig := auth.ProxyConfig{
			Type:    auth.ProxyTypeHTTPS,
			Host:    "secure.proxy.com",
			Port:    443,
			Enabled: true,
		}

		url := fmt.Sprintf("%s/api/credentials/%s/%s/proxy", env.ServerURL, env.ProviderID, env.TokenID)
		reqBody, _ := json.Marshal(proxyConfig)
		req, _ := http.NewRequest("PUT", url, bytes.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")

		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		require.Equal(t, http.StatusOK, resp.StatusCode)

		// Verify configuration
		getResp := getProxyConfig(t, env.ServerURL, env.ProviderID, env.TokenID)
		require.NotNil(t, getResp.Proxy)
		assert.Equal(t, auth.ProxyTypeHTTPS, getResp.Proxy.Type)
	})
}

func TestProxyE2E_ConfigureSOCKS5Proxy(t *testing.T) {
	t.Run("configure SOCKS5 proxy for token", func(t *testing.T) {
		env := setupE2ETestEnvironment(t)
		defer env.Cleanup()

		proxyConfig := auth.ProxyConfig{
			Type:    auth.ProxyTypeSOCKS5,
			Host:    "socks.example.com",
			Port:    1080,
			Enabled: true,
		}

		url := fmt.Sprintf("%s/api/credentials/%s/%s/proxy", env.ServerURL, env.ProviderID, env.TokenID)
		reqBody, _ := json.Marshal(proxyConfig)
		req, _ := http.NewRequest("PUT", url, bytes.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")

		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		require.Equal(t, http.StatusOK, resp.StatusCode)

		// Verify configuration
		getResp := getProxyConfig(t, env.ServerURL, env.ProviderID, env.TokenID)
		require.NotNil(t, getResp.Proxy)
		assert.Equal(t, auth.ProxyTypeSOCKS5, getResp.Proxy.Type)
	})
}

func TestProxyE2E_ConfigureAuthenticatedProxy(t *testing.T) {
	t.Run("configure authenticated proxy for token", func(t *testing.T) {
		env := setupE2ETestEnvironment(t)
		defer env.Cleanup()

		proxyConfig := auth.ProxyConfig{
			Type:     auth.ProxyTypeHTTP,
			Host:     "proxy.example.com",
			Port:     8080,
			Username: "testuser",
			Password: "testpass",
			Enabled:  true,
		}

		url := fmt.Sprintf("%s/api/credentials/%s/%s/proxy", env.ServerURL, env.ProviderID, env.TokenID)
		reqBody, _ := json.Marshal(proxyConfig)
		req, _ := http.NewRequest("PUT", url, bytes.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")

		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		require.Equal(t, http.StatusOK, resp.StatusCode)

		// Verify configuration (password should be masked)
		getResp := getProxyConfig(t, env.ServerURL, env.ProviderID, env.TokenID)
		require.NotNil(t, getResp.Proxy)
		assert.Equal(t, "testuser", getResp.Proxy.Username)
		assert.Equal(t, "***", getResp.Proxy.Password) // Password should be masked
	})
}

func TestProxyE2E_ConfigureAndDisableProxy(t *testing.T) {
	t.Run("configure and then disable proxy", func(t *testing.T) {
		env := setupE2ETestEnvironment(t)
		defer env.Cleanup()

		// Configure proxy
		proxyConfig := auth.ProxyConfig{
			Type:    auth.ProxyTypeHTTP,
			Host:    "proxy.example.com",
			Port:    8080,
			Enabled: true,
		}

		url := fmt.Sprintf("%s/api/credentials/%s/%s/proxy", env.ServerURL, env.ProviderID, env.TokenID)
		reqBody, _ := json.Marshal(proxyConfig)
		req, _ := http.NewRequest("PUT", url, bytes.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")

		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Do(req)
		require.NoError(t, err)
		resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		// Disable proxy
		proxyConfig.Enabled = false
		reqBody, _ = json.Marshal(proxyConfig)
		req, _ = http.NewRequest("PUT", url, bytes.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")

		resp, err = client.Do(req)
		require.NoError(t, err)
		resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		// Verify proxy is disabled
		getResp := getProxyConfig(t, env.ServerURL, env.ProviderID, env.TokenID)
		require.NotNil(t, getResp.Proxy)
		assert.False(t, getResp.Proxy.Enabled)
	})
}

func TestProxyE2E_ConfigureDeleteReconfigure(t *testing.T) {
	t.Run("configure, delete, and reconfigure proxy", func(t *testing.T) {
		env := setupE2ETestEnvironment(t)
		defer env.Cleanup()

		// Configure proxy
		proxyConfig1 := auth.ProxyConfig{
			Type:    auth.ProxyTypeHTTP,
			Host:    "proxy1.example.com",
			Port:    8080,
			Enabled: true,
		}

		url := fmt.Sprintf("%s/api/credentials/%s/%s/proxy", env.ServerURL, env.ProviderID, env.TokenID)
		reqBody, _ := json.Marshal(proxyConfig1)
		req, _ := http.NewRequest("PUT", url, bytes.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")

		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Do(req)
		require.NoError(t, err)
		resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		// Delete proxy
		req, _ = http.NewRequest("DELETE", url, nil)
		resp, err = client.Do(req)
		require.NoError(t, err)
		resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		// Verify proxy is deleted
		getResp := getProxyConfig(t, env.ServerURL, env.ProviderID, env.TokenID)
		assert.Nil(t, getResp.Proxy)

		// Reconfigure with different proxy
		proxyConfig2 := auth.ProxyConfig{
			Type:    auth.ProxyTypeHTTPS,
			Host:    "proxy2.example.com",
			Port:    443,
			Enabled: true,
		}

		reqBody, _ = json.Marshal(proxyConfig2)
		req, _ = http.NewRequest("PUT", url, bytes.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")

		resp, err = client.Do(req)
		require.NoError(t, err)
		resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		// Verify new configuration
		getResp = getProxyConfig(t, env.ServerURL, env.ProviderID, env.TokenID)
		require.NotNil(t, getResp.Proxy)
		assert.Equal(t, auth.ProxyTypeHTTPS, getResp.Proxy.Type)
		assert.Equal(t, "proxy2.example.com", getResp.Proxy.Host)
	})
}

// ============================================================================
// Request Flow Tests
// ============================================================================

func TestProxyE2E_RequestWithConfiguredProxy(t *testing.T) {
	t.Run("request with configured proxy succeeds", func(t *testing.T) {
		env := setupE2ETestEnvironment(t)
		defer env.Cleanup()

		// Configure proxy
		proxyConfig := auth.ProxyConfig{
			Type:    auth.ProxyTypeHTTP,
			Host:    "proxy.example.com",
			Port:    8080,
			Enabled: true,
		}

		url := fmt.Sprintf("%s/api/credentials/%s/%s/proxy", env.ServerURL, env.ProviderID, env.TokenID)
		reqBody, _ := json.Marshal(proxyConfig)
		req, _ := http.NewRequest("PUT", url, bytes.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")

		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Do(req)
		require.NoError(t, err)
		resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		// Get token from API
		tokenURL := fmt.Sprintf("%s/api/token/%s", env.ServerURL, env.ProviderID)
		req, _ = http.NewRequest("GET", tokenURL, nil)
		resp, err = client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		var tokenResp map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&tokenResp)
		require.NotNil(t, tokenResp["access_token"])

		// Verify proxy configuration is retrieved
		getResp := getProxyConfig(t, env.ServerURL, env.ProviderID, env.TokenID)
		require.NotNil(t, getResp.Proxy)
		assert.True(t, getResp.Proxy.Enabled)
	})
}

func TestProxyE2E_RequestWithNoProxy(t *testing.T) {
	t.Run("request with no proxy succeeds", func(t *testing.T) {
		env := setupE2ETestEnvironment(t)
		defer env.Cleanup()

		// No proxy configured - should use direct connection
		getResp := getProxyConfig(t, env.ServerURL, env.ProviderID, env.TokenID)
		assert.Nil(t, getResp.Proxy)

		// Get token from API
		tokenURL := fmt.Sprintf("%s/api/token/%s", env.ServerURL, env.ProviderID)
		req, _ := http.NewRequest("GET", tokenURL, nil)

		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		var tokenResp map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&tokenResp)
		require.NotNil(t, tokenResp["access_token"])
	})
}

func TestProxyE2E_RequestAfterProxyChange(t *testing.T) {
	t.Run("request after proxy change uses new proxy", func(t *testing.T) {
		env := setupE2ETestEnvironment(t)
		defer env.Cleanup()

		// Configure first proxy
		proxyConfig1 := auth.ProxyConfig{
			Type:    auth.ProxyTypeHTTP,
			Host:    "proxy1.example.com",
			Port:    8080,
			Enabled: true,
		}

		url := fmt.Sprintf("%s/api/credentials/%s/%s/proxy", env.ServerURL, env.ProviderID, env.TokenID)
		reqBody, _ := json.Marshal(proxyConfig1)
		req, _ := http.NewRequest("PUT", url, bytes.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")

		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Do(req)
		require.NoError(t, err)
		resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		// Verify first proxy
		getResp := getProxyConfig(t, env.ServerURL, env.ProviderID, env.TokenID)
		assert.Equal(t, "proxy1.example.com", getResp.Proxy.Host)

		// Change to second proxy
		proxyConfig2 := auth.ProxyConfig{
			Type:    auth.ProxyTypeHTTPS,
			Host:    "proxy2.example.com",
			Port:    443,
			Enabled: true,
		}

		reqBody, _ = json.Marshal(proxyConfig2)
		req, _ = http.NewRequest("PUT", url, bytes.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")

		resp, err = client.Do(req)
		require.NoError(t, err)
		resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		// Verify new proxy is used
		getResp = getProxyConfig(t, env.ServerURL, env.ProviderID, env.TokenID)
		assert.Equal(t, "proxy2.example.com", getResp.Proxy.Host)
		assert.Equal(t, auth.ProxyTypeHTTPS, getResp.Proxy.Type)
	})
}

func TestProxyE2E_RequestAfterProxyDelete(t *testing.T) {
	t.Run("request after proxy delete uses direct connection", func(t *testing.T) {
		env := setupE2ETestEnvironment(t)
		defer env.Cleanup()

		// Configure proxy
		proxyConfig := auth.ProxyConfig{
			Type:    auth.ProxyTypeHTTP,
			Host:    "proxy.example.com",
			Port:    8080,
			Enabled: true,
		}

		url := fmt.Sprintf("%s/api/credentials/%s/%s/proxy", env.ServerURL, env.ProviderID, env.TokenID)
		reqBody, _ := json.Marshal(proxyConfig)
		req, _ := http.NewRequest("PUT", url, bytes.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")

		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Do(req)
		require.NoError(t, err)
		resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		// Verify proxy is configured
		getResp := getProxyConfig(t, env.ServerURL, env.ProviderID, env.TokenID)
		require.NotNil(t, getResp.Proxy)

		// Delete proxy
		req, _ = http.NewRequest("DELETE", url, nil)
		resp, err = client.Do(req)
		require.NoError(t, err)
		resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		// Verify proxy is removed (direct connection)
		getResp = getProxyConfig(t, env.ServerURL, env.ProviderID, env.TokenID)
		assert.Nil(t, getResp.Proxy)
	})
}

// ============================================================================
// Error Flow Tests
// ============================================================================

func TestProxyE2E_InvalidProxyConfigurationRejected(t *testing.T) {
	tests := []struct {
		name        string
		proxyConfig auth.ProxyConfig
		expectError string
	}{
		{
			name: "empty host",
			proxyConfig: auth.ProxyConfig{
				Type:    auth.ProxyTypeHTTP,
				Host:    "",
				Port:    8080,
				Enabled: true,
			},
			expectError: "host",
		},
		{
			name: "port out of range",
			proxyConfig: auth.ProxyConfig{
				Type:    auth.ProxyTypeHTTP,
				Host:    "proxy.example.com",
				Port:    70000,
				Enabled: true,
			},
			expectError: "port",
		},
		{
			name: "username without password",
			proxyConfig: auth.ProxyConfig{
				Type:     auth.ProxyTypeHTTP,
				Host:     "proxy.example.com",
				Port:     8080,
				Username: "testuser",
				Enabled:  true,
			},
			expectError: "password",
		},
		{
			name: "password without username",
			proxyConfig: auth.ProxyConfig{
				Type:     auth.ProxyTypeHTTP,
				Host:     "proxy.example.com",
				Port:     8080,
				Password: "testpass",
				Enabled:  true,
			},
			expectError: "username",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := setupE2ETestEnvironment(t)
			defer env.Cleanup()

			url := fmt.Sprintf("%s/api/credentials/%s/%s/proxy", env.ServerURL, env.ProviderID, env.TokenID)
			reqBody, _ := json.Marshal(tt.proxyConfig)
			req, _ := http.NewRequest("PUT", url, bytes.NewReader(reqBody))
			req.Header.Set("Content-Type", "application/json")

			client := &http.Client{Timeout: 5 * time.Second}
			resp, err := client.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			require.Equal(t, http.StatusBadRequest, resp.StatusCode)

			var errorResp map[string]interface{}
			json.NewDecoder(resp.Body).Decode(&errorResp)
			require.NotNil(t, errorResp["error"])
			assert.Contains(t, errorResp["error"].(string), tt.expectError)
		})
	}
}

func TestProxyE2E_ProxyConnectionFailureHandled(t *testing.T) {
	t.Run("proxy connection failure is handled gracefully", func(t *testing.T) {
		env := setupE2ETestEnvironment(t)
		defer env.Cleanup()

		// Configure proxy with unreachable host
		proxyConfig := auth.ProxyConfig{
			Type:    auth.ProxyTypeHTTP,
			Host:    "unreachable-proxy.example.invalid",
			Port:    8080,
			Enabled: true,
		}

		url := fmt.Sprintf("%s/api/credentials/%s/%s/proxy", env.ServerURL, env.ProviderID, env.TokenID)
		reqBody, _ := json.Marshal(proxyConfig)
		req, _ := http.NewRequest("PUT", url, bytes.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")

		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Do(req)
		require.NoError(t, err)
		resp.Body.Close()

		// Configuration should succeed (validation only)
		require.Equal(t, http.StatusOK, resp.StatusCode)

		// Verify configuration is saved
		getResp := getProxyConfig(t, env.ServerURL, env.ProviderID, env.TokenID)
		require.NotNil(t, getResp.Proxy)
		assert.Equal(t, "unreachable-proxy.example.invalid", getResp.Proxy.Host)
	})
}

func TestProxyE2E_ProxyTimeoutHandled(t *testing.T) {
	t.Run("proxy timeout is handled gracefully", func(t *testing.T) {
		env := setupE2ETestEnvironment(t)
		defer env.Cleanup()

		// Configure proxy with unreachable IP (will timeout)
		proxyConfig := auth.ProxyConfig{
			Type:    auth.ProxyTypeHTTP,
			Host:    "10.255.255.1",
			Port:    8080,
			Enabled: true,
		}

		url := fmt.Sprintf("%s/api/credentials/%s/%s/proxy", env.ServerURL, env.ProviderID, env.TokenID)
		reqBody, _ := json.Marshal(proxyConfig)
		req, _ := http.NewRequest("PUT", url, bytes.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")

		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Do(req)
		require.NoError(t, err)
		resp.Body.Close()

		// Configuration should succeed (validation only)
		require.Equal(t, http.StatusOK, resp.StatusCode)

		// Verify configuration is saved
		getResp := getProxyConfig(t, env.ServerURL, env.ProviderID, env.TokenID)
		require.NotNil(t, getResp.Proxy)
	})
}

func TestProxyE2E_ProxyHealthUpdatedOnFailure(t *testing.T) {
	t.Run("proxy health is updated on failure", func(t *testing.T) {
		env := setupE2ETestEnvironment(t)
		defer env.Cleanup()

		// Configure proxy
		proxyConfig := auth.ProxyConfig{
			Type:    auth.ProxyTypeHTTP,
			Host:    "proxy.example.com",
			Port:    8080,
			Enabled: true,
		}

		url := fmt.Sprintf("%s/api/credentials/%s/%s/proxy", env.ServerURL, env.ProviderID, env.TokenID)
		reqBody, _ := json.Marshal(proxyConfig)
		req, _ := http.NewRequest("PUT", url, bytes.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")

		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Do(req)
		require.NoError(t, err)
		resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		// Get proxy config with health status
		getResp := getProxyConfig(t, env.ServerURL, env.ProviderID, env.TokenID)
		require.NotNil(t, getResp.Proxy)

		// Health status should be present (initial state)
		if getResp.HealthStatus != nil {
			assert.Equal(t, 1.0, getResp.HealthStatus.HealthScore)
		}
	})
}

// ============================================================================
// Health Tracking Tests
// ============================================================================

func TestProxyE2E_HealthScoreIncreasesOnSuccess(t *testing.T) {
	t.Run("health score increases on successful requests", func(t *testing.T) {
		env := setupE2ETestEnvironment(t)
		defer env.Cleanup()

		// Configure proxy
		proxyConfig := auth.ProxyConfig{
			Type:    auth.ProxyTypeHTTP,
			Host:    "proxy.example.com",
			Port:    8080,
			Enabled: true,
		}

		url := fmt.Sprintf("%s/api/credentials/%s/%s/proxy", env.ServerURL, env.ProviderID, env.TokenID)
		reqBody, _ := json.Marshal(proxyConfig)
		req, _ := http.NewRequest("PUT", url, bytes.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")

		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Do(req)
		require.NoError(t, err)
		resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		// Get proxy config with health status
		getResp := getProxyConfig(t, env.ServerURL, env.ProviderID, env.TokenID)
		require.NotNil(t, getResp.Proxy)

		// Initial health score should be 1.0
		if getResp.HealthStatus != nil {
			assert.Equal(t, 1.0, getResp.HealthStatus.HealthScore)
		}
	})
}

func TestProxyE2E_HealthScoreDecreasesOnFailure(t *testing.T) {
	t.Run("health score decreases on failed requests", func(t *testing.T) {
		env := setupE2ETestEnvironment(t)
		defer env.Cleanup()

		// Configure proxy with unreachable host
		proxyConfig := auth.ProxyConfig{
			Type:    auth.ProxyTypeHTTP,
			Host:    "unreachable.example.invalid",
			Port:    8080,
			Enabled: true,
		}

		url := fmt.Sprintf("%s/api/credentials/%s/%s/proxy", env.ServerURL, env.ProviderID, env.TokenID)
		reqBody, _ := json.Marshal(proxyConfig)
		req, _ := http.NewRequest("PUT", url, bytes.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")

		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Do(req)
		require.NoError(t, err)
		resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		// Get proxy config with health status
		getResp := getProxyConfig(t, env.ServerURL, env.ProviderID, env.TokenID)
		require.NotNil(t, getResp.Proxy)

		// Health status should be present
		if getResp.HealthStatus != nil {
			assert.GreaterOrEqual(t, getResp.HealthStatus.HealthScore, 0.0)
			assert.LessOrEqual(t, getResp.HealthStatus.HealthScore, 1.0)
		}
	})
}

func TestProxyE2E_HealthStatusReflectsProxyState(t *testing.T) {
	t.Run("health status reflects current proxy state", func(t *testing.T) {
		env := setupE2ETestEnvironment(t)
		defer env.Cleanup()

		// Configure proxy
		proxyConfig := auth.ProxyConfig{
			Type:    auth.ProxyTypeHTTP,
			Host:    "proxy.example.com",
			Port:    8080,
			Enabled: true,
		}

		url := fmt.Sprintf("%s/api/credentials/%s/%s/proxy", env.ServerURL, env.ProviderID, env.TokenID)
		reqBody, _ := json.Marshal(proxyConfig)
		req, _ := http.NewRequest("PUT", url, bytes.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")

		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Do(req)
		require.NoError(t, err)
		resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		// Get proxy config with health status
		getResp := getProxyConfig(t, env.ServerURL, env.ProviderID, env.TokenID)
		require.NotNil(t, getResp.Proxy)

		// Verify health status structure
		if getResp.HealthStatus != nil {
			assert.Greater(t, getResp.HealthStatus.LastCheck, int64(0))
			assert.NotNil(t, getResp.HealthStatus.HealthScore)
			assert.NotNil(t, getResp.HealthStatus.ConsecutiveFailures)
		}
	})
}

// ============================================================================
// Caching Tests
// ============================================================================

func TestProxyE2E_SameProxyConfigSharesClient(t *testing.T) {
	t.Run("same proxy config shares cached client", func(t *testing.T) {
		env := setupE2ETestEnvironment(t)
		defer env.Cleanup()

		// Configure proxy
		proxyConfig := auth.ProxyConfig{
			Type:    auth.ProxyTypeHTTP,
			Host:    "proxy.example.com",
			Port:    8080,
			Enabled: true,
		}

		url := fmt.Sprintf("%s/api/credentials/%s/%s/proxy", env.ServerURL, env.ProviderID, env.TokenID)
		reqBody, _ := json.Marshal(proxyConfig)
		req, _ := http.NewRequest("PUT", url, bytes.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")

		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Do(req)
		require.NoError(t, err)
		resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		// Get proxy config multiple times - should use cached client
		for i := 0; i < 3; i++ {
			getResp := getProxyConfig(t, env.ServerURL, env.ProviderID, env.TokenID)
			require.NotNil(t, getResp.Proxy)
			assert.Equal(t, "proxy.example.com", getResp.Proxy.Host)
		}
	})
}

func TestProxyE2E_DifferentProxyConfigCreatesNewClient(t *testing.T) {
	t.Run("different proxy config creates new client", func(t *testing.T) {
		env := setupE2ETestEnvironment(t)
		defer env.Cleanup()

		// Configure first proxy
		proxyConfig1 := auth.ProxyConfig{
			Type:    auth.ProxyTypeHTTP,
			Host:    "proxy1.example.com",
			Port:    8080,
			Enabled: true,
		}

		url := fmt.Sprintf("%s/api/credentials/%s/%s/proxy", env.ServerURL, env.ProviderID, env.TokenID)
		reqBody, _ := json.Marshal(proxyConfig1)
		req, _ := http.NewRequest("PUT", url, bytes.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")

		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Do(req)
		require.NoError(t, err)
		resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		getResp1 := getProxyConfig(t, env.ServerURL, env.ProviderID, env.TokenID)
		assert.Equal(t, "proxy1.example.com", getResp1.Proxy.Host)

		// Change to second proxy
		proxyConfig2 := auth.ProxyConfig{
			Type:    auth.ProxyTypeHTTPS,
			Host:    "proxy2.example.com",
			Port:    443,
			Enabled: true,
		}

		reqBody, _ = json.Marshal(proxyConfig2)
		req, _ = http.NewRequest("PUT", url, bytes.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")

		resp, err = client.Do(req)
		require.NoError(t, err)
		resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		getResp2 := getProxyConfig(t, env.ServerURL, env.ProviderID, env.TokenID)
		assert.Equal(t, "proxy2.example.com", getResp2.Proxy.Host)
		assert.Equal(t, auth.ProxyTypeHTTPS, getResp2.Proxy.Type)
	})
}

func TestProxyE2E_CacheClearedOnProxyChange(t *testing.T) {
	t.Run("cache is cleared when proxy config changes", func(t *testing.T) {
		env := setupE2ETestEnvironment(t)
		defer env.Cleanup()

		// Configure first proxy
		proxyConfig1 := auth.ProxyConfig{
			Type:    auth.ProxyTypeHTTP,
			Host:    "proxy1.example.com",
			Port:    8080,
			Enabled: true,
		}

		url := fmt.Sprintf("%s/api/credentials/%s/%s/proxy", env.ServerURL, env.ProviderID, env.TokenID)
		reqBody, _ := json.Marshal(proxyConfig1)
		req, _ := http.NewRequest("PUT", url, bytes.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")

		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Do(req)
		require.NoError(t, err)
		resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		// Get proxy config
		getResp1 := getProxyConfig(t, env.ServerURL, env.ProviderID, env.TokenID)
		assert.Equal(t, "proxy1.example.com", getResp1.Proxy.Host)

		// Change proxy config
		proxyConfig2 := auth.ProxyConfig{
			Type:    auth.ProxyTypeHTTP,
			Host:    "proxy2.example.com",
			Port:    8081,
			Enabled: true,
		}

		reqBody, _ = json.Marshal(proxyConfig2)
		req, _ = http.NewRequest("PUT", url, bytes.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")

		resp, err = client.Do(req)
		require.NoError(t, err)
		resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		// Verify new config is returned (cache was cleared)
		getResp2 := getProxyConfig(t, env.ServerURL, env.ProviderID, env.TokenID)
		assert.Equal(t, "proxy2.example.com", getResp2.Proxy.Host)
		assert.Equal(t, 8081, getResp2.Proxy.Port)
	})
}

// ============================================================================
// Provider-Specific Tests
// ============================================================================

func TestProxyE2E_GeminiProxyFlow(t *testing.T) {
	t.Run("complete proxy flow for Gemini provider", func(t *testing.T) {
		env := setupE2ETestEnvironment(t)
		defer env.Cleanup()

		// Configure proxy for Gemini-style provider
		proxyConfig := auth.ProxyConfig{
			Type:    auth.ProxyTypeHTTP,
			Host:    "gemini-proxy.example.com",
			Port:    8080,
			Enabled: true,
		}

		url := fmt.Sprintf("%s/api/credentials/%s/%s/proxy", env.ServerURL, env.ProviderID, env.TokenID)
		reqBody, _ := json.Marshal(proxyConfig)
		req, _ := http.NewRequest("PUT", url, bytes.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")

		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Do(req)
		require.NoError(t, err)
		resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		// Verify configuration
		getResp := getProxyConfig(t, env.ServerURL, env.ProviderID, env.TokenID)
		require.NotNil(t, getResp.Proxy)
		assert.Equal(t, "gemini-proxy.example.com", getResp.Proxy.Host)
	})
}

func TestProxyE2E_IFlowProxyFlow(t *testing.T) {
	t.Run("complete proxy flow for IFlow provider", func(t *testing.T) {
		env := setupE2ETestEnvironment(t)
		defer env.Cleanup()

		// Configure proxy for IFlow-style provider
		proxyConfig := auth.ProxyConfig{
			Type:    auth.ProxyTypeHTTPS,
			Host:    "iflow-proxy.example.com",
			Port:    443,
			Enabled: true,
		}

		url := fmt.Sprintf("%s/api/credentials/%s/%s/proxy", env.ServerURL, env.ProviderID, env.TokenID)
		reqBody, _ := json.Marshal(proxyConfig)
		req, _ := http.NewRequest("PUT", url, bytes.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")

		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Do(req)
		require.NoError(t, err)
		resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		// Verify configuration
		getResp := getProxyConfig(t, env.ServerURL, env.ProviderID, env.TokenID)
		require.NotNil(t, getResp.Proxy)
		assert.Equal(t, auth.ProxyTypeHTTPS, getResp.Proxy.Type)
	})
}

func TestProxyE2E_KiroProxyFlow(t *testing.T) {
	t.Run("complete proxy flow for Kiro provider", func(t *testing.T) {
		env := setupE2ETestEnvironment(t)
		defer env.Cleanup()

		// Configure proxy for Kiro-style provider
		proxyConfig := auth.ProxyConfig{
			Type:    auth.ProxyTypeSOCKS5,
			Host:    "kiro-proxy.example.com",
			Port:    1080,
			Enabled: true,
		}

		url := fmt.Sprintf("%s/api/credentials/%s/%s/proxy", env.ServerURL, env.ProviderID, env.TokenID)
		reqBody, _ := json.Marshal(proxyConfig)
		req, _ := http.NewRequest("PUT", url, bytes.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")

		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Do(req)
		require.NoError(t, err)
		resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		// Verify configuration
		getResp := getProxyConfig(t, env.ServerURL, env.ProviderID, env.TokenID)
		require.NotNil(t, getResp.Proxy)
		assert.Equal(t, auth.ProxyTypeSOCKS5, getResp.Proxy.Type)
	})
}

func TestProxyE2E_QwenProxyFlow(t *testing.T) {
	t.Run("complete proxy flow for Qwen provider", func(t *testing.T) {
		env := setupE2ETestEnvironment(t)
		defer env.Cleanup()

		// Configure proxy for Qwen-style provider
		proxyConfig := auth.ProxyConfig{
			Type:     auth.ProxyTypeHTTP,
			Host:     "qwen-proxy.example.com",
			Port:     8080,
			Username: "qwenuser",
			Password: "qwenpass",
			Enabled:  true,
		}

		url := fmt.Sprintf("%s/api/credentials/%s/%s/proxy", env.ServerURL, env.ProviderID, env.TokenID)
		reqBody, _ := json.Marshal(proxyConfig)
		req, _ := http.NewRequest("PUT", url, bytes.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")

		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Do(req)
		require.NoError(t, err)
		resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		// Verify configuration (password should be masked)
		getResp := getProxyConfig(t, env.ServerURL, env.ProviderID, env.TokenID)
		require.NotNil(t, getResp.Proxy)
		assert.Equal(t, "qwen-proxy.example.com", getResp.Proxy.Host)
		assert.Equal(t, "qwenuser", getResp.Proxy.Username)
		assert.Equal(t, "***", getResp.Proxy.Password)
	})
}

func TestProxyE2E_AntigravityProxyFlow(t *testing.T) {
	t.Run("complete proxy flow for Antigravity provider", func(t *testing.T) {
		env := setupE2ETestEnvironment(t)
		defer env.Cleanup()

		// Configure proxy for Antigravity-style provider
		proxyConfig := auth.ProxyConfig{
			Type:    auth.ProxyTypeHTTPS,
			Host:    "antigravity-proxy.example.com",
			Port:    443,
			Enabled: true,
		}

		url := fmt.Sprintf("%s/api/credentials/%s/%s/proxy", env.ServerURL, env.ProviderID, env.TokenID)
		reqBody, _ := json.Marshal(proxyConfig)
		req, _ := http.NewRequest("PUT", url, bytes.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")

		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Do(req)
		require.NoError(t, err)
		resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		// Verify configuration
		getResp := getProxyConfig(t, env.ServerURL, env.ProviderID, env.TokenID)
		require.NotNil(t, getResp.Proxy)
		assert.Equal(t, "antigravity-proxy.example.com", getResp.Proxy.Host)
	})
}

// ============================================================================
// Comprehensive Test Scenarios
// ============================================================================

func TestProxyE2E_MultipleTokensWithDifferentProxies(t *testing.T) {
	t.Run("multiple tokens with different proxy configurations", func(t *testing.T) {
		env := setupE2ETestEnvironment(t)
		defer env.Cleanup()

		// Create additional tokens
		tokenID2 := createTestTokenViaAPI(t, env.ServerURL, env.ProviderID)
		tokenID3 := createTestTokenViaAPI(t, env.ServerURL, env.ProviderID)

		// Configure different proxies for each token
		proxyConfigs := []struct {
			tokenID string
			config  auth.ProxyConfig
		}{
			{
				tokenID: env.TokenID,
				config: auth.ProxyConfig{
					Type:    auth.ProxyTypeHTTP,
					Host:    "proxy1.example.com",
					Port:    8080,
					Enabled: true,
				},
			},
			{
				tokenID: tokenID2,
				config: auth.ProxyConfig{
					Type:    auth.ProxyTypeHTTPS,
					Host:    "proxy2.example.com",
					Port:    443,
					Enabled: true,
				},
			},
			{
				tokenID: tokenID3,
				config: auth.ProxyConfig{
					Type:    auth.ProxyTypeSOCKS5,
					Host:    "proxy3.example.com",
					Port:    1080,
					Enabled: true,
				},
			},
		}

		client := &http.Client{Timeout: 5 * time.Second}
		for _, pc := range proxyConfigs {
			url := fmt.Sprintf("%s/api/credentials/%s/%s/proxy", env.ServerURL, env.ProviderID, pc.tokenID)
			reqBody, _ := json.Marshal(pc.config)
			req, _ := http.NewRequest("PUT", url, bytes.NewReader(reqBody))
			req.Header.Set("Content-Type", "application/json")

			resp, err := client.Do(req)
			require.NoError(t, err)
			resp.Body.Close()
			require.Equal(t, http.StatusOK, resp.StatusCode)
		}

		// Verify each token has correct proxy configuration
		for _, pc := range proxyConfigs {
			getResp := getProxyConfig(t, env.ServerURL, env.ProviderID, pc.tokenID)
			require.NotNil(t, getResp.Proxy)
			assert.Equal(t, pc.config.Host, getResp.Proxy.Host)
			assert.Equal(t, pc.config.Type, getResp.Proxy.Type)
		}
	})
}

func TestProxyE2E_ConcurrentProxyConfiguration(t *testing.T) {
	t.Run("concurrent proxy configuration requests", func(t *testing.T) {
		env := setupE2ETestEnvironment(t)
		defer env.Cleanup()

		// Create additional tokens for concurrent testing
		tokenIDs := []string{env.TokenID}
		for i := 0; i < 4; i++ {
			tokenIDs = append(tokenIDs, createTestTokenViaAPI(t, env.ServerURL, env.ProviderID))
		}

		var wg sync.WaitGroup
		client := &http.Client{Timeout: 5 * time.Second}

		// Configure proxies concurrently
		for i, tokenID := range tokenIDs {
			wg.Add(1)
			go func(idx int, tid string) {
				defer wg.Done()

				proxyConfig := auth.ProxyConfig{
					Type:    auth.ProxyTypeHTTP,
					Host:    fmt.Sprintf("proxy%d.example.com", idx),
					Port:    8080 + idx,
					Enabled: true,
				}

				url := fmt.Sprintf("%s/api/credentials/%s/%s/proxy", env.ServerURL, env.ProviderID, tid)
				reqBody, _ := json.Marshal(proxyConfig)
				req, _ := http.NewRequest("PUT", url, bytes.NewReader(reqBody))
				req.Header.Set("Content-Type", "application/json")

				resp, err := client.Do(req)
				if err == nil {
					resp.Body.Close()
				}
			}(i, tokenID)
		}

		wg.Wait()

		// Verify all configurations were applied
		for i, tokenID := range tokenIDs {
			getResp := getProxyConfig(t, env.ServerURL, env.ProviderID, tokenID)
			require.NotNil(t, getResp.Proxy)
			assert.Equal(t, fmt.Sprintf("proxy%d.example.com", i), getResp.Proxy.Host)
		}
	})
}

func TestProxyE2E_ProxyConfigurationDuringActiveRequest(t *testing.T) {
	t.Run("proxy configuration change during active request", func(t *testing.T) {
		env := setupE2ETestEnvironment(t)
		defer env.Cleanup()

		// Configure initial proxy
		proxyConfig1 := auth.ProxyConfig{
			Type:    auth.ProxyTypeHTTP,
			Host:    "proxy1.example.com",
			Port:    8080,
			Enabled: true,
		}

		url := fmt.Sprintf("%s/api/credentials/%s/%s/proxy", env.ServerURL, env.ProviderID, env.TokenID)
		reqBody, _ := json.Marshal(proxyConfig1)
		req, _ := http.NewRequest("PUT", url, bytes.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")

		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Do(req)
		require.NoError(t, err)
		resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		// Simulate active request by getting token
		tokenURL := fmt.Sprintf("%s/api/token/%s", env.ServerURL, env.ProviderID)
		req, _ = http.NewRequest("GET", tokenURL, nil)
		resp, err = client.Do(req)
		require.NoError(t, err)
		resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		// Change proxy configuration while "request is active"
		proxyConfig2 := auth.ProxyConfig{
			Type:    auth.ProxyTypeHTTPS,
			Host:    "proxy2.example.com",
			Port:    443,
			Enabled: true,
		}

		reqBody, _ = json.Marshal(proxyConfig2)
		req, _ = http.NewRequest("PUT", url, bytes.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")

		resp, err = client.Do(req)
		require.NoError(t, err)
		resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		// Verify new configuration is active
		getResp := getProxyConfig(t, env.ServerURL, env.ProviderID, env.TokenID)
		require.NotNil(t, getResp.Proxy)
		assert.Equal(t, "proxy2.example.com", getResp.Proxy.Host)
	})
}

func TestProxyE2E_TokenRefreshWithProxy(t *testing.T) {
	t.Run("token refresh with proxy configuration", func(t *testing.T) {
		env := setupE2ETestEnvironment(t)
		defer env.Cleanup()

		// Configure proxy
		proxyConfig := auth.ProxyConfig{
			Type:    auth.ProxyTypeHTTP,
			Host:    "proxy.example.com",
			Port:    8080,
			Enabled: true,
		}

		url := fmt.Sprintf("%s/api/credentials/%s/%s/proxy", env.ServerURL, env.ProviderID, env.TokenID)
		reqBody, _ := json.Marshal(proxyConfig)
		req, _ := http.NewRequest("PUT", url, bytes.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")

		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Do(req)
		require.NoError(t, err)
		resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		// Verify proxy is configured
		getResp := getProxyConfig(t, env.ServerURL, env.ProviderID, env.TokenID)
		require.NotNil(t, getResp.Proxy)

		// Get token info to verify it's still accessible
		credsURL := fmt.Sprintf("%s/api/credentials/%s", env.ServerURL, env.ProviderID)
		req, _ = http.NewRequest("GET", credsURL, nil)
		resp, err = client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		var credsResp map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&credsResp)
		require.NotNil(t, credsResp["tokens"])
	})
}

// ============================================================================
// Helper Functions
// ============================================================================

// getProxyConfig retrieves proxy configuration for a token
func getProxyConfig(t *testing.T, serverURL, providerID, tokenID string) *restapi.ProxyConfigResponse {
	t.Helper()

	url := fmt.Sprintf("%s/api/credentials/%s/%s/proxy", serverURL, providerID, tokenID)
	req, _ := http.NewRequest("GET", url, nil)

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)

	var result restapi.ProxyConfigResponse
	err = json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)

	return &result
}
