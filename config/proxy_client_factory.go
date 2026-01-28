// Package config provides configuration management for the application.
// This file contains the proxy-aware HTTP client factory.
package config

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"golang.org/x/net/proxy"

	"github.com/sunbankio/qwencoder-proxy/auth"
	"github.com/sunbankio/qwencoder-proxy/logging"
)

// ProxyAwareHTTPClientFactory creates and caches HTTP clients configured with proxy settings.
// This factory implements the factory pattern and client caching to optimize resource usage.
//
// The factory maintains a cache of HTTP clients keyed by proxy configuration.
// Tokens with identical proxy configurations share HTTP clients, reducing resource usage.
// Credentials are excluded from the cache key for security reasons.
type ProxyAwareHTTPClientFactory struct {
	baseConfig  HTTPClientConfig
	proxyCache  map[auth.ProxyConfigKey]*http.Client
	cacheLock   sync.RWMutex
	logger      *logging.Logger
	maxSize     int
	accessOrder []auth.ProxyConfigKey // Track access order for LRU eviction
}

// NewProxyAwareHTTPClientFactory creates a new ProxyAwareHTTPClientFactory.
//
// Parameters:
//   - baseConfig: Base HTTP client configuration for all created clients
//   - logger: Logger for factory operations
//   - maxSize: Maximum number of cached clients (default: 50 if <= 0)
func NewProxyAwareHTTPClientFactory(baseConfig HTTPClientConfig, logger *logging.Logger, maxSize int) *ProxyAwareHTTPClientFactory {
	if maxSize <= 0 {
		maxSize = 50
	}

	return &ProxyAwareHTTPClientFactory{
		baseConfig:  baseConfig,
		proxyCache:  make(map[auth.ProxyConfigKey]*http.Client),
		logger:      logger,
		maxSize:     maxSize,
		accessOrder: make([]auth.ProxyConfigKey, 0),
	}
}

// GetClient returns an HTTP client for the given proxy configuration.
// If a client for this configuration exists in the cache, it is returned.
// Otherwise, a new client is created, cached, and returned.
//
// The cache key excludes credentials for security - clients with the same
// proxy server but different credentials will share the same client.
func (f *ProxyAwareHTTPClientFactory) GetClient(proxyConfig *auth.ProxyConfig) *http.Client {
	if proxyConfig == nil || proxyConfig.Type == auth.ProxyTypeNone {
		return f.createDirectClient()
	}

	key := auth.NewProxyConfigKey(proxyConfig)
	if key == nil {
		return f.createDirectClient()
	}

	f.cacheLock.RLock()
	client, exists := f.proxyCache[*key]
	f.cacheLock.RUnlock()

	if exists {
		f.logger.DebugLog("Cache hit for proxy: %s", key.String())
		f.updateAccessOrder(*key)
		return client
	}

	f.logger.DebugLog("Cache miss for proxy: %s, creating new client", key.String())

	f.cacheLock.Lock()
	defer f.cacheLock.Unlock()

	// Double-check after acquiring write lock
	if client, exists := f.proxyCache[*key]; exists {
		return client
	}

	client = f.CreateClientWithProxy(proxyConfig)
	f.proxyCache[*key] = client
	f.accessOrder = append(f.accessOrder, *key)

	// Apply LRU eviction if cache is full
	if len(f.proxyCache) > f.maxSize {
		f.evictLRU()
	}

	f.logger.DebugLog("Created and cached new client for proxy: %s (cache size: %d/%d)",
		key.String(), len(f.proxyCache), f.maxSize)

	return client
}

// CreateClientWithProxy creates a new HTTP client configured with the given proxy settings.
// This method does not use the cache - it always creates a new client.
func (f *ProxyAwareHTTPClientFactory) CreateClientWithProxy(proxyConfig *auth.ProxyConfig) *http.Client {
	if proxyConfig == nil || proxyConfig.Type == auth.ProxyTypeNone {
		return f.createDirectClient()
	}

	var transport *http.Transport
	var err error

	switch proxyConfig.Type {
	case auth.ProxyTypeHTTP, auth.ProxyTypeHTTPS:
		transport, err = f.createHTTPProxyTransport(proxyConfig)
	case auth.ProxyTypeSOCKS5:
		transport, err = f.createSOCKS5Transport(proxyConfig)
	default:
		f.logger.WarningLog("Unknown proxy type: %s, using direct connection", proxyConfig.Type)
		return f.createDirectClient()
	}

	if err != nil {
		f.logger.ErrorLog("Failed to create proxy transport for %s: %v, using direct connection", proxyConfig.Type, err)
		return f.createDirectClient()
	}

	return &http.Client{
		Timeout:   time.Duration(f.baseConfig.RequestTimeoutSeconds) * time.Second,
		Transport: transport,
	}
}

// ClearCache removes all cached HTTP clients from the factory.
// This is useful when proxy configurations change or when resources need to be freed.
func (f *ProxyAwareHTTPClientFactory) ClearCache() {
	f.cacheLock.Lock()
	defer f.cacheLock.Unlock()

	for key, client := range f.proxyCache {
		// Close idle connections to free resources
		if transport, ok := client.Transport.(*http.Transport); ok {
			transport.CloseIdleConnections()
		}
		delete(f.proxyCache, key)
	}

	f.accessOrder = f.accessOrder[:0]
	f.logger.DebugLog("Cleared proxy client cache")
}

// GetCacheSize returns the current number of cached HTTP clients.
func (f *ProxyAwareHTTPClientFactory) GetCacheSize() int {
	f.cacheLock.RLock()
	defer f.cacheLock.RUnlock()
	return len(f.proxyCache)
}

// createDirectClient creates an HTTP client without proxy configuration.
func (f *ProxyAwareHTTPClientFactory) createDirectClient() *http.Client {
	return &http.Client{
		Timeout: time.Duration(f.baseConfig.RequestTimeoutSeconds) * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        f.baseConfig.MaxIdleConns,
			MaxIdleConnsPerHost: f.baseConfig.MaxIdleConnsPerHost,
			MaxConnsPerHost:     0, // No limit
			IdleConnTimeout:     time.Duration(f.baseConfig.IdleConnTimeoutSeconds) * time.Second,
		},
	}
}

// createHTTPProxyTransport creates an HTTP transport configured for HTTP/HTTPS proxy.
func (f *ProxyAwareHTTPClientFactory) createHTTPProxyTransport(proxyConfig *auth.ProxyConfig) (*http.Transport, error) {
	proxyURL, err := url.Parse(fmt.Sprintf("%s://%s:%d",
		proxyConfig.Type, proxyConfig.Host, proxyConfig.Port))
	if err != nil {
		return nil, fmt.Errorf("failed to parse proxy URL: %w", err)
	}

	// Add credentials if provided
	if proxyConfig.Username != "" && proxyConfig.Password != "" {
		proxyURL.User = url.UserPassword(proxyConfig.Username, proxyConfig.Password)
	}

	return &http.Transport{
		Proxy: http.ProxyURL(proxyURL),
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:        f.baseConfig.MaxIdleConns,
		MaxIdleConnsPerHost: f.baseConfig.MaxIdleConnsPerHost,
		MaxConnsPerHost:     0,
		IdleConnTimeout:     time.Duration(f.baseConfig.IdleConnTimeoutSeconds) * time.Second,
	}, nil
}

// createSOCKS5Transport creates an HTTP transport configured for SOCKS5 proxy.
func (f *ProxyAwareHTTPClientFactory) createSOCKS5Transport(proxyConfig *auth.ProxyConfig) (*http.Transport, error) {
	var auth *proxy.Auth
	if proxyConfig.Username != "" && proxyConfig.Password != "" {
		auth = &proxy.Auth{
			User:     proxyConfig.Username,
			Password: proxyConfig.Password,
		}
	}

	dialer, err := proxy.SOCKS5("tcp",
		fmt.Sprintf("%s:%d", proxyConfig.Host, proxyConfig.Port),
		auth,
		&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create SOCKS5 dialer: %w", err)
	}

	return &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return dialer.Dial(network, addr)
		},
		MaxIdleConns:        f.baseConfig.MaxIdleConns,
		MaxIdleConnsPerHost: f.baseConfig.MaxIdleConnsPerHost,
		MaxConnsPerHost:     0,
		IdleConnTimeout:     time.Duration(f.baseConfig.IdleConnTimeoutSeconds) * time.Second,
	}, nil
}

// updateAccessOrder updates the access order for LRU tracking.
// The key is moved to the end of the access order slice (most recently used).
func (f *ProxyAwareHTTPClientFactory) updateAccessOrder(key auth.ProxyConfigKey) {
	f.cacheLock.Lock()
	defer f.cacheLock.Unlock()

	// Find and remove the key from its current position
	for i, k := range f.accessOrder {
		if k == key {
			f.accessOrder = append(f.accessOrder[:i], f.accessOrder[i+1:]...)
			break
		}
	}

	// Add to the end (most recently used)
	f.accessOrder = append(f.accessOrder, key)
}

// evictLRU removes the least recently used client from the cache.
// This method must be called while holding the cacheLock.
func (f *ProxyAwareHTTPClientFactory) evictLRU() {
	if len(f.accessOrder) == 0 {
		return
	}

	// Remove the first element (least recently used)
	lruKey := f.accessOrder[0]
	f.accessOrder = f.accessOrder[1:]

	if client, exists := f.proxyCache[lruKey]; exists {
		// Close idle connections to free resources
		if transport, ok := client.Transport.(*http.Transport); ok {
			transport.CloseIdleConnections()
		}
		delete(f.proxyCache, lruKey)
		f.logger.DebugLog("Evicted LRU client for proxy: %s", lruKey.String())
	}
}
