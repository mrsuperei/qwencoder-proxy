// Package main is the entry point for the unified qwencoder-proxy server
// This server combines proxy functionality with OAuth REST API and dashboard
package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

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
	port := flag.String("port", "", "Server port (default: from config or 8143)")
	callbackURL := flag.String("callback-url", "", "Callback base URL (default: http://localhost:<port>)")
	enableCORS := flag.Bool("cors", false, "Enable CORS")
	debug := flag.Bool("debug", false, "Enable debug logging")
	flag.Parse()

	// Load configuration
	cfg := config.DefaultConfig()

	// Override with command-line flags
	if *port != "" {
		cfg.Server.Port = *port
		cfg.OAuthServer.Port = *port
	}
	if *callbackURL != "" {
		cfg.OAuthServer.CallbackBaseURL = *callbackURL
	} else {
		// Default callback URL to match server port
		cfg.OAuthServer.CallbackBaseURL = "http://localhost:" + cfg.Server.Port
	}
	if *enableCORS {
		cfg.OAuthServer.EnableCORS = true
	}
	if *debug {
		cfg.Logging.IsDebugMode = true
	}

	// Initialize logger
	logger := logging.NewLogger()
	if cfg.Logging.IsDebugMode {
		logger.DebugLog("Debug mode enabled")
	}

	// Initialize provider factory
	factory := provider.NewFactory()

	// Register all providers
	registerProviders(factory)

	// Populate model-to-provider mapping
	ctx := context.Background()
	if err := factory.PopulateModelProviders(ctx); err != nil {
		logger.ErrorLog("Failed to populate model providers: %v", err)
	}

	// Initialize converter factory
	convFactory := converter.NewFactory()

	// Create HTTP multiplexer
	mux := http.NewServeMux()

	// Register proxy routes
	proxy.RegisterOpenAIRoutes(mux, factory, convFactory)
	proxy.RegisterProviderSpecificRoutes(mux, factory, convFactory)
	if err := proxy.RegisterGeminiRoutes(mux, factory); err != nil {
		logger.ErrorLog("Failed to register Gemini routes: %v", err)
	}
	if err := proxy.RegisterAnthropicRoutes(mux, factory); err != nil {
		logger.ErrorLog("Failed to register Anthropic routes: %v", err)
	}

	// Create OAuth REST API server
	oauthConfig := &restapi.Config{
		Port:            cfg.OAuthServer.Port,
		CallbackBaseURL: cfg.OAuthServer.CallbackBaseURL,
		StateTTL:        cfg.OAuthServer.StateTTL,
		DeviceCodeTTL:   cfg.OAuthServer.DeviceCodeTTL,
		EnableCORS:      cfg.OAuthServer.EnableCORS,
		AllowedOrigins:  cfg.OAuthServer.AllowedOrigins,
	}
	oauthServer := restapi.NewServer(oauthConfig, logger)

	// Register OAuth routes
	oauthServer.RegisterRoutes(mux)

	// Apply middleware
	var handler http.Handler = mux
	if cfg.OAuthServer.EnableCORS {
		handler = restapi.CORS(cfg.OAuthServer.AllowedOrigins)(handler)
	}
	handler = restapi.Logging(logger)(handler)

	// Start server
	addr := ":" + cfg.Server.Port
	logger.InfoLog("Starting qwencoder-proxy on %s", addr)
	logger.InfoLog("Dashboard available at http://localhost:%s", cfg.Server.Port)
	logger.InfoLog("OAuth API available at http://localhost:%s/api", cfg.Server.Port)
	logger.InfoLog("Proxy API available at http://localhost:%s/v1", cfg.Server.Port)

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		logger.InfoLog("Shutting down...")
		os.Exit(0)
	}()

	if err := http.ListenAndServe(addr, handler); err != nil {
		logger.ErrorLog("Server error: %v", err)
		os.Exit(1)
	}
}

// registerProviders registers all available providers with the factory
func registerProviders(factory *provider.Factory) {
	// Register Qwen provider (no authenticator needed)
	qwenProvider := qwen.NewProvider()
	factory.Register(qwenProvider)

	// Register Gemini CLI provider
	geminiProvider := gemini.NewProvider(nil)
	factory.Register(geminiProvider)

	// Register Kiro provider (Claude-compatible)
	kiroProvider := kiro.NewProvider(nil)
	factory.Register(kiroProvider)

	// Register Antigravity provider
	antigravityProvider := antigravity.NewProvider(nil)
	factory.Register(antigravityProvider)

	// Register iFlow provider
	iflowProvider := iflow.NewProvider(nil)
	factory.Register(iflowProvider)
}

func init() {
	// Print banner
	fmt.Println(`
╔════════════════════════════════════════════════════════╗
║                                                            ║
║              QWENCODER-PROXY SERVER                         ║
║                                                            ║
║  Unified proxy server with OAuth REST API and dashboard      ║
║                                                            ║
╚════════════════════════════════════════════════════════╝
`)
}
