// Package main is the entry point for qwencoder-proxy server
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sunbankio/qwencoder-proxy/internal/config"
	"github.com/sunbankio/qwencoder-proxy/internal/converter"
	"github.com/sunbankio/qwencoder-proxy/internal/logging"
	"github.com/sunbankio/qwencoder-proxy/internal/provider"
	"github.com/sunbankio/qwencoder-proxy/internal/provider/antigravity"
	"github.com/sunbankio/qwencoder-proxy/internal/provider/gemini"
	"github.com/sunbankio/qwencoder-proxy/internal/provider/iflow"
	"github.com/sunbankio/qwencoder-proxy/internal/provider/kiro"
	"github.com/sunbankio/qwencoder-proxy/internal/provider/qwen"
	"github.com/sunbankio/qwencoder-proxy/internal/ratelimit"
	"github.com/sunbankio/qwencoder-proxy/internal/restapi"
	"github.com/sunbankio/qwencoder-proxy/internal/token"

	_ "modernc.org/sqlite"
)

func main() {
	// Parse command-line flags
	port := flag.String("port", "", "Server port (default: 8143)")
	debug := flag.Bool("debug", false, "Enable debug logging")
	migrateCmd := flag.Bool("migrate-ratelimit", false, "Migrate rate limiting data to main database")
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

	// Handle migration command
	if *migrateCmd {
		logger.InfoLog("Starting rate limiting data migration...")

		// Create backup
		backupPath, err := ratelimit.BackupDatabase(cfg.Storage.DBPath)
		if err != nil {
			logger.ErrorLog("Failed to create backup: %v", err)
			os.Exit(1)
		}
		logger.InfoLog("Backup created successfully: %s", backupPath)

		// Migrate data
		if err := ratelimit.MigrateRateLimitingToTokensDB(cfg.Storage.DBPath+".ratelimit", cfg.Storage.DBPath); err != nil {
			logger.ErrorLog("Migration failed: %v", err)
			os.Exit(1)
		}

		logger.InfoLog("Migration completed successfully")
		logger.InfoLog("You can now safely delete the old rate limiting database: %s", cfg.Storage.DBPath+".ratelimit")
		return
	}

	logger.InfoLog("Starting qwencoder-proxy server on port %s", cfg.Server.Port)

	// Create multi-token manager
	multiTokenMgr := token.NewMultiTokenManager(logger)
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

	// Set storage configuration
	multiTokenMgr.SetStorageConfig(cfg.Storage.DBPath)
	logger.InfoLog("DB path: %s", cfg.Storage.DBPath)

	// Create proxy client factory for proxy-aware HTTP clients
	proxyClientFactory := config.NewProxyAwareHTTPClientFactory(
		cfg.HTTPClient,
		logger,
		50, // max cache size
	)

	// Set client factory on multi-token manager
	multiTokenMgr.SetClientFactory(proxyClientFactory)

	// Open SHARED database connection for both token storage AND rate limiting
	db, err := sql.Open("sqlite", cfg.Storage.DBPath)
	if err != nil {
		logger.ErrorLog("Failed to open database: %v", err)
		os.Exit(1)
	}
	// Configure connection pool for SQLite (single connection to prevent locks)
	db.SetMaxOpenConns(1) // Only one connection for SQLite
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(5 * time.Minute)
	db.SetConnMaxIdleTime(1 * time.Minute)
	logger.InfoLog("Configured database connection pool for SQLite")

	// NOTE: Database connection is shared between rate limiting and token storage.
	// It will be closed during graceful shutdown.

	// Verify database has required tables
	logger.InfoLog("Verifying rate limiting database tables...")
	if err := verifyRateLimitTables(db, logger); err != nil {
		logger.ErrorLog("Database verification failed: %v", err)
		os.Exit(1)
	}
	logger.InfoLog("Rate limiting database tables verified")

	// Initialize integrated rate limit system with SHARED database connection
	logger.InfoLog("Initializing integrated rate limit system with database: %s", cfg.Storage.DBPath)

	// Create system configuration from app config
	rateLimitConfig := &ratelimit.SystemConfig{
		AsyncRecording: &ratelimit.AsyncUsageRecorderConfig{
			Enabled:     cfg.RateLimit.AsyncEnabled,
			WorkerCount: cfg.RateLimit.AsyncWorkerCount,
			QueueSize:   cfg.RateLimit.AsyncQueueSize,
			RetryLimit:  cfg.RateLimit.AsyncRetryLimit,
			RetryDelay:  cfg.RateLimit.AsyncRetryDelay,
		},
		Caching: &ratelimit.CacheConfig{
			ProviderMetricsTTL:  cfg.RateLimit.CacheProviderTTL,
			TokenMetricsTTL:     cfg.RateLimit.CacheTokenTTL,
			MaxProviderEntries:  cfg.RateLimit.CacheMaxProviders,
			MaxTokenEntries:     cfg.RateLimit.CacheMaxTokens,
			RefreshBeforeExpiry: cfg.RateLimit.CacheRefreshBeforeTTL,
		},
		DBNotification: &ratelimit.DBNotificationConfig{
			Enabled:         cfg.RateLimit.EnableDBNotification,
			PollingInterval: cfg.RateLimit.DBPollingInterval,
		},
	}

	rateLimitSystem, err := ratelimit.NewIntegratedRateLimitSystem(db, logger, rateLimitConfig)
	if err != nil {
		logger.ErrorLog("Failed to initialize integrated rate limit system: %v", err)
		os.Exit(1)
	}
	logger.InfoLog("Integrated rate limit system initialized successfully")

	// Start the rate limit system
	if err := rateLimitSystem.Start(); err != nil {
		logger.ErrorLog("Failed to start rate limit system: %v", err)
		os.Exit(1)
	}
	logger.InfoLog("Rate limit system started")

	// Get components from integrated system
	quotaManager := rateLimitSystem.GetQuotaManager()
	cacheInvalidator := rateLimitSystem.GetCacheInvalidator()
	logger.InfoLog("Rate limiting components ready")

	// Create provider factory
	providerFactory := provider.NewFactory(logger)

	// Create converter factory
	converterFactory := converter.NewFactory()

	// Initialize providers with token manager injection
	if err := initializeProviders(providerFactory, multiTokenMgr, logger); err != nil {
		logger.ErrorLog("Failed to initialize providers: %v", err)
		os.Exit(1)
	}

	// Create HTTP mux and register routes
	mux := http.NewServeMux()

	// Initialize token stores with SHARED database connection
	// This prevents database locking issues with rate limiting system
	if err := multiTokenMgr.InitializeStoresWithDB(db, logger); err != nil {
		logger.ErrorLog("Failed to initialize token stores with shared DB: %v", err)
		os.Exit(1)
	}
	logger.InfoLog("Initialized token stores with shared database connection")

	// Create REST API server for OAuth flows
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

	// Inject rate limit manager into REST API server
	restAPIServer.SetRateLimitManager(quotaManager)

	// Inject cache invalidator into REST API server
	restAPIServer.SetCacheInvalidator(cacheInvalidator)

	// Create dashboard handler
	dashboardHandler, err := restapi.NewDashboardHandler(logger)
	if err != nil {
		logger.ErrorLog("Failed to create dashboard handler: %v", err)
	} else {
		// Register dashboard routes
		mux.HandleFunc("/", dashboardHandler.ServeIndex)
		mux.HandleFunc("/tokens", dashboardHandler.ServeTokens)
		mux.HandleFunc("/proxies", dashboardHandler.ServeProxies)
		mux.HandleFunc("/css/", dashboardHandler.ServeCSS)
		mux.HandleFunc("/js/", dashboardHandler.ServeJS)
		logger.InfoLog("Dashboard routes registered")
	}

	// Register REST API routes (providers, credentials, etc.)
	restAPIServer.RegisterRoutes(mux)

	// Register OpenAI-compatible routes with token manager integration
	restAPIServer.RegisterProxyRoutes(mux, providerFactory, converterFactory)

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
		quotaManager.Close()
		os.Exit(1)
	case sig := <-sigChan:
		logger.InfoLog("Received signal %v, shutting down...", sig)
		// Stop rate limit system first (includes all rate limiting components)
		if err := rateLimitSystem.Stop(); err != nil {
			logger.ErrorLog("Failed to stop rate limit system: %v", err)
		}
		// Stop multi-token manager (closes token stores)
		multiTokenMgr.Stop()
		// Close shared database connection
		if err := db.Close(); err != nil {
			logger.ErrorLog("Failed to close database: %v", err)
		}
		// Stop REST API server
		restAPIServer.Stop()
		logger.InfoLog("Server stopped gracefully")
		os.Exit(0)
	}
}

// initializeProviders creates and registers all providers with token manager injection
func initializeProviders(
	factory *provider.Factory,
	multiTokenMgr *token.MultiTokenManager,
	logger logging.Logger,
) error {
	logger.InfoLog("Initializing providers with token manager injection...")

	// Register provider configurations with multi-token manager
	if err := registerProviderConfigs(multiTokenMgr); err != nil {
		return fmt.Errorf("failed to register provider configs: %w", err)
	}

	// Create and register Gemini provider
	geminiTokenMgr, err := multiTokenMgr.GetTokenManager("gemini-cli")
	if err != nil {
		logger.WarnLog("Failed to get token manager for gemini-cli: %v", err)
	} else {
		geminiProvider := gemini.NewProviderWithTokenManager(nil, geminiTokenMgr, logger)
		factory.RegisterWithTokenManager(geminiProvider, geminiTokenMgr)
		logger.InfoLog("Registered gemini-cli provider with token manager")
	}

	// Create and register iFlow provider
	iflowAuth := iflow.NewAuthenticator(nil)
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
	kiroAuth := kiro.NewAuthenticator(nil)
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
	antigravityAuth := antigravity.NewAuthenticator(nil)
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
func registerProviderConfigs(multiTokenMgr *token.MultiTokenManager) error {
	// Register Gemini provider config
	multiTokenMgr.RegisterProvider("gemini-cli", token.ProviderConfig{
		ID:           "gemini-cli",
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
	multiTokenMgr.RegisterProvider("iflow", token.ProviderConfig{
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
	multiTokenMgr.RegisterProvider("kiro", token.ProviderConfig{
		ID:           "kiro",
		ClientID:     "10009311001",
		ClientSecret: "4Z3YjXycVsQvyGF1etiNlIBB4RsqSDtW",
		AuthURL:      "https://oidc.{{region}}.amazonaws.com/token",
		TokenURL:     "https://codewhisperer.{{region}}.amazonaws.com",
		Flow:         "manual",
		Scopes:       []string{},
	})

	// Register Qwen provider config
	multiTokenMgr.RegisterProvider("qwen", token.ProviderConfig{
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
	multiTokenMgr.RegisterProvider("antigravity", token.ProviderConfig{
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

// verifyRateLimitTables verifies that all required rate limiting tables exist
func verifyRateLimitTables(db *sql.DB, logger logging.Logger) error {
	requiredTables := []string{
		"provider_usage",
		"token_usage",
		"request_history",
	}

	// Query SQLite master table to get all tables
	rows, err := db.Query(`
		SELECT name FROM sqlite_master
		WHERE type='table'
		AND name IN (?, ?, ?)
		ORDER BY name
	`, requiredTables[0], requiredTables[1], requiredTables[2])
	if err != nil {
		return fmt.Errorf("failed to query database tables: %w", err)
	}
	defer rows.Close()

	var existingTables []string
	for rows.Next() {
		var tableName string
		if err := rows.Scan(&tableName); err != nil {
			return fmt.Errorf("failed to scan table name: %w", err)
		}
		existingTables = append(existingTables, tableName)
	}

	// Check for missing tables
	missingTables := []string{}
	for _, required := range requiredTables {
		found := false
		for _, existing := range existingTables {
			if required == existing {
				found = true
				break
			}
		}
		if !found {
			missingTables = append(missingTables, required)
		}
	}

	if len(missingTables) > 0 {
		return fmt.Errorf("missing required tables: %v", missingTables)
	}

	logger.InfoLog("All required rate limiting tables exist: %v", existingTables)
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
