// Package main is the entry point for qwencoder-proxy server
package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sunbankio/qwencoder-proxy/auth"
	"github.com/sunbankio/qwencoder-proxy/config"
	"github.com/sunbankio/qwencoder-proxy/converter"
	"github.com/sunbankio/qwencoder-proxy/logging"
	"github.com/sunbankio/qwencoder-proxy/provider"
	"github.com/sunbankio/qwencoder-proxy/provider/antigravity"
	"github.com/sunbankio/qwencoder-proxy/provider/gemini"
	"github.com/sunbankio/qwencoder-proxy/provider/iflow"
	"github.com/sunbankio/qwencoder-proxy/provider/kiro"
	"github.com/sunbankio/qwencoder-proxy/provider/qwen"
	"github.com/sunbankio/qwencoder-proxy/proxy"
	"github.com/sunbankio/qwencoder-proxy/restapi"
)

func main() {
	// Parse command-line flags
	port := flag.String("port", "", "Server port (default: 8143)")
	debug := flag.Bool("debug", false, "Enable debug logging")
	flag.Parse()

	// Load configuration from environment
	cfg := config.DefaultConfig()

	// Override with command-line flags
	if *port != "" {
		cfg.Server.Port = *port
	}
	if *debug {
		cfg.Logging.IsDebugMode = true
		logging.IsDebugMode = true
	}

	// Initialize logger
	logger := logging.NewLogger()
	if cfg.Logging.IsDebugMode {
		logger.DebugLog("Debug mode enabled")
	}

	logger.InfoLog("Starting qwencoder-proxy server on port %s", cfg.Server.Port)

	// Create multi-token manager
	multiTokenMgr := auth.NewMultiTokenManager(logger)
	if err := multiTokenMgr.Initialize(); err != nil {
		logger.ErrorLog("Failed to initialize multi-token manager: %v", err)
		os.Exit(1)
	}

	// Start refresh schedulers
	if err := multiTokenMgr.Start(); err != nil {
		logger.ErrorLog("Failed to start multi-token manager: %v", err)
		os.Exit(1)
	}

	// Set credentials directory (can be overridden by environment)
	if credsDir := os.Getenv("CREDENTIALS_DIR"); credsDir != "" {
		multiTokenMgr.SetCredentialsDir(credsDir)
		logger.InfoLog("Using credentials directory: %s", credsDir)
	}

	// Create proxy client factory for proxy-aware HTTP clients
	proxyClientFactory := config.NewProxyAwareHTTPClientFactory(
		cfg.HTTPClient,
		logger,
		50, // max cache size
	)

	// Set client factory on multi-token manager
	multiTokenMgr.SetClientFactory(proxyClientFactory)

	// Create provider factory
	providerFactory := provider.NewFactory()

	// Create converter factory
	converterFactory := converter.NewFactory()

	// Initialize providers with token manager injection
	if err := initializeProviders(providerFactory, multiTokenMgr, logger); err != nil {
		logger.ErrorLog("Failed to initialize providers: %v", err)
		os.Exit(1)
	}

	// Create HTTP mux and register routes
	mux := http.NewServeMux()

	// Create REST API server for dashboard and OAuth flows
	apiConfig := &restapi.Config{
		Port:            cfg.Server.Port,
		CallbackBaseURL: "http://localhost:" + cfg.Server.Port,
		StateTTL:        10 * time.Minute,
		DeviceCodeTTL:   15 * time.Minute,
		EnableCORS:      true,
		AllowedOrigins:  []string{"*"},
	}
	restAPIServer := restapi.NewServer(apiConfig, logger)

	// Inject the main application's multi-token manager into the REST API server
	// This ensures both the proxy server and REST API server share the same token state
	restAPIServer.SetMultiTokenManager(multiTokenMgr)

	// Register REST API routes (dashboard, providers, credentials, etc.)
	restAPIServer.RegisterRoutes(mux)

	// Register OpenAI-compatible routes
	proxy.RegisterOpenAIRoutes(mux, providerFactory, converterFactory)

	// Register provider-specific routes
	proxy.RegisterProviderSpecificRoutes(mux, providerFactory, converterFactory)

	// Apply middleware (CORS, logging)
	handler := applyMiddleware(mux, logger, cfg)

	// Create server address
	addr := ":" + cfg.Server.Port

	// Setup graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Start server in goroutine
	errChan := make(chan error, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.ErrorLog("Panic recovered: %v", r)
			}
		}()
		logger.InfoLog("Server listening on %s", addr)
		errChan <- http.ListenAndServe(addr, handler)
	}()

	// Wait for shutdown or error
	select {
	case err := <-errChan:
		logger.ErrorLog("Server error: %v", err)
		multiTokenMgr.Stop()
		os.Exit(1)
	case sig := <-sigChan:
		logger.InfoLog("Received signal %v, shutting down...", sig)
		// Stop multi-token manager
		multiTokenMgr.Stop()
		// Stop REST API server
		restAPIServer.Stop()
		logger.InfoLog("Server stopped gracefully")
		os.Exit(0)
	}
}

// initializeProviders creates and registers all providers with token manager injection
func initializeProviders(
	factory *provider.Factory,
	multiTokenMgr *auth.MultiTokenManager,
	logger logging.Logger,
) error {
	logger.InfoLog("Initializing providers with token manager injection...")

	// Register provider configurations with multi-token manager
	if err := registerProviderConfigs(multiTokenMgr); err != nil {
		return fmt.Errorf("failed to register provider configs: %w", err)
	}

	// Create and register Gemini provider
	geminiAuth := auth.NewGeminiAuthenticator(nil)
	geminiAuth.SetMultiTokenManager(multiTokenMgr)
	geminiProvider := gemini.NewProvider(geminiAuth)
	geminiTokenMgr, err := multiTokenMgr.GetTokenManager("gemini")
	if err != nil {
		logger.WarnLog("Failed to get token manager for gemini: %v", err)
	} else {
		geminiAuth.SetTokenManager(geminiTokenMgr)
		factory.RegisterWithTokenManager(geminiProvider, geminiTokenMgr)
		logger.InfoLog("Registered gemini provider with token manager")
	}

	// Create and register iFlow provider
	iflowAuth := auth.NewIFlowAuthenticator(nil)
	iflowAuth.SetMultiTokenManager(multiTokenMgr)
	iflowProvider := iflow.NewProvider(iflowAuth)
	iflowTokenMgr, err := multiTokenMgr.GetTokenManager("iflow")
	if err != nil {
		logger.WarnLog("Failed to get token manager for iflow: %v", err)
	} else {
		iflowAuth.SetTokenManager(iflowTokenMgr)
		factory.RegisterWithTokenManager(iflowProvider, iflowTokenMgr)
		logger.InfoLog("Registered iflow provider with token manager")
	}

	// Create and register Kiro provider
	kiroAuth := auth.NewKiroAuthenticator(nil)
	kiroAuth.SetMultiTokenManager(multiTokenMgr)
	kiroProvider := kiro.NewProvider(kiroAuth)
	kiroTokenMgr, err := multiTokenMgr.GetTokenManager("kiro")
	if err != nil {
		logger.WarnLog("Failed to get token manager for kiro: %v", err)
	} else {
		kiroAuth.SetTokenManager(kiroTokenMgr)
		factory.RegisterWithTokenManager(kiroProvider, kiroTokenMgr)
		logger.InfoLog("Registered kiro provider with token manager")
	}

	// Create and register Qwen provider
	qwenTokenMgr, err := multiTokenMgr.GetTokenManager("qwen")
	if err != nil {
		logger.WarnLog("Failed to get token manager for qwen: %v", err)
	} else {
		qwenProvider := qwen.NewProviderWithTokenManager(qwenTokenMgr, logger)
		factory.RegisterWithTokenManager(qwenProvider, qwenTokenMgr)
		logger.InfoLog("Registered qwen provider with token manager")
	}

	// Create and register Antigravity provider
	antigravityAuth := auth.NewGeminiAuthenticator(nil)
	antigravityAuth.SetMultiTokenManager(multiTokenMgr)
	antigravityProvider := antigravity.NewProvider(antigravityAuth)
	antigravityTokenMgr, err := multiTokenMgr.GetTokenManager("antigravity")
	if err != nil {
		logger.WarnLog("Failed to get token manager for antigravity: %v", err)
	} else {
		antigravityAuth.SetTokenManager(antigravityTokenMgr)
		factory.RegisterWithTokenManager(antigravityProvider, antigravityTokenMgr)
		logger.InfoLog("Registered antigravity provider with token manager")
	}

	// Populate model-to-provider mappings
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := factory.PopulateModelProviders(ctx); err != nil {
		logger.WarnLog("Failed to populate model providers: %v", err)
		// Continue anyway - providers have hardcoded models
	}

	logger.InfoLog("All providers initialized successfully")
	return nil
}

// registerProviderConfigs registers provider configurations with multi-token manager
func registerProviderConfigs(multiTokenMgr *auth.MultiTokenManager) error {
	// Register Gemini provider config
	multiTokenMgr.RegisterProvider("gemini", auth.ProviderConfig{
		ID:           "gemini",
		ClientID:     "681255809395-oo8ft2oprdrnp9e3aqf6av3hmdib135j.apps.googleusercontent.com",
		ClientSecret: "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",
		AuthURL:      "https://accounts.google.com/o/oauth2/v2/auth",
		TokenURL:     "https://oauth2.googleapis.com/token",
		Flow:         "device_code",
		Scopes: []string{
			"https://www.googleapis.com/auth/cloud-platform",
			"https://www.googleapis.com/auth/userinfo.email",
			"https://www.googleapis.com/auth/userinfo.profile",
			"openid",
		},
	})

	// Register iFlow provider config
	multiTokenMgr.RegisterProvider("iflow", auth.ProviderConfig{
		ID:           "iflow",
		ClientID:     "10009311001",
		ClientSecret: "4Z3YjXycVsQvyGF1etiNlIBB4RsqSDtW",
		AuthURL:      "https://iflow.cn/oauth",
		TokenURL:     "https://iflow.cn/oauth/token",
		Flow:         "device_code",
		Scopes: []string{
			"openid",
			"email",
			"profile",
		},
	})

	// Register Kiro provider config
	multiTokenMgr.RegisterProvider("kiro", auth.ProviderConfig{
		ID:           "kiro",
		ClientID:     "10009311001",
		ClientSecret: "4Z3YjXycVsQvyGF1etiNlIBB4RsqSDtW",
		AuthURL:      "https://oidc.{{region}}.amazonaws.com/token",
		TokenURL:     "https://codewhisperer.{{region}}.amazonaws.com",
		Flow:         "manual",
		Scopes:       []string{},
	})

	// Register Qwen provider config
	multiTokenMgr.RegisterProvider("qwen", auth.ProviderConfig{
		ID:           "qwen",
		ClientID:     "10009311001",
		ClientSecret: "4Z3YjXycVsQvyGF1etiNlIBB4RsqSDtW",
		AuthURL:      "https://iflow.cn/oauth",
		TokenURL:     "https://iflow.cn/oauth/token",
		Flow:         "device_code",
		Scopes: []string{
			"openid",
			"email",
			"profile",
		},
	})

	// Register Antigravity provider config
	multiTokenMgr.RegisterProvider("antigravity", auth.ProviderConfig{
		ID:           "antigravity",
		ClientID:     "1071006060591-tmhssin2h21lcre235vtolojh4g403ep.apps.googleusercontent.com",
		ClientSecret: "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",
		AuthURL:      "https://accounts.google.com/o/oauth2/v2/auth",
		TokenURL:     "https://oauth2.googleapis.com/token",
		Flow:         "device_code",
		Scopes: []string{
			"https://www.googleapis.com/auth/cloud-platform",
			"https://www.googleapis.com/auth/userinfo.email",
			"https://www.googleapis.com/auth/userinfo.profile",
			"openid",
		},
	})

	return nil
}

// applyMiddleware adds middleware to the HTTP handler
func applyMiddleware(handler http.Handler, logger logging.Logger, cfg *config.Config) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Log request
		logger.InfoLog("%s %s", r.Method, r.URL.Path)

		// Set CORS headers
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		// Handle OPTIONS preflight
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		// Call next handler
		handler.ServeHTTP(w, r)
	})
}
