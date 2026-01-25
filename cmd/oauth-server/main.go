// Package main is the entry point for the OAuth REST API server
package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/sunbankio/qwencoder-proxy/config"
	"github.com/sunbankio/qwencoder-proxy/logging"
	"github.com/sunbankio/qwencoder-proxy/restapi"
)

func main() {
	// Parse command-line flags
	port := flag.String("port", "", "Server port (default: from config or 8080)")
	callbackURL := flag.String("callback-url", "", "Callback base URL (default: http://localhost:<port>)")
	enableCORS := flag.Bool("cors", false, "Enable CORS")
	debug := flag.Bool("debug", false, "Enable debug logging")
	flag.Parse()

	// Load configuration
	cfg := config.DefaultConfig()

	// Override with command-line flags
	if *port != "" {
		cfg.OAuthServer.Port = *port
	}
	if *callbackURL != "" {
		cfg.OAuthServer.CallbackBaseURL = *callbackURL
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

	// Create OAuth API server configuration
	apiConfig := &restapi.Config{
		Port:            cfg.OAuthServer.Port,
		CallbackBaseURL: cfg.OAuthServer.CallbackBaseURL,
		StateTTL:        cfg.OAuthServer.StateTTL,
		DeviceCodeTTL:   cfg.OAuthServer.DeviceCodeTTL,
		EnableCORS:      cfg.OAuthServer.EnableCORS,
		AllowedOrigins:  cfg.OAuthServer.AllowedOrigins,
	}

	// Create and start OAuth REST API server
	server := restapi.NewServer(apiConfig, logger)

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Start server in goroutine
	errChan := make(chan error, 1)
	go func() {
		errChan <- server.Start()
	}()

	// Wait for shutdown or error
	select {
	case err := <-errChan:
		if err != nil {
			logger.ErrorLog("Server error: %v", err)
			os.Exit(1)
		}
	case sig := <-sigChan:
		logger.InfoLog("Received signal %v, shutting down...", sig)
		// Note: In a more complete implementation, we would gracefully shutdown the server
		// For now, we just exit
		os.Exit(0)
	}
}

func init() {
	// Print banner
	fmt.Println(`
╔══════════════════════════════════════════════════════════╗
║                                                            ║
║              OAuth2 REST API Server                         ║
║                                                            ║
║  A minimal REST API for OAuth2 authentication flows        ║
║                                                            ║
╚══════════════════════════════════════════════════════════╝
`)
}
