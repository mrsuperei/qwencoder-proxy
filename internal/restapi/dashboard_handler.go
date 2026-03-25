// Package restapi provides REST API endpoints for OAuth2 authentication flows
package restapi

import (
	"embed"
	"fmt"
	"io/fs"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/sunbankio/qwencoder-proxy/internal/logging"
)

//go:embed web
var webFS embed.FS

// DashboardHandler handles HTTP requests for the dashboard
type DashboardHandler struct {
	logger      logging.Logger
	webFS       fs.FS
	cssFS       fs.FS
	jsFS        fs.FS
	indexHTML   []byte
	tokensHTML  []byte
	proxiesHTML []byte
}

// NewDashboardHandler creates a new dashboard handler
func NewDashboardHandler(logger logging.Logger) (*DashboardHandler, error) {
	// Create subdirectory filesystems
	cssFS, err := fs.Sub(webFS, "web/css")
	if err != nil {
		return nil, fmt.Errorf("failed to create CSS subdirectory: %w", err)
	}

	jsFS, err := fs.Sub(webFS, "web/js")
	if err != nil {
		return nil, fmt.Errorf("failed to create JS subdirectory: %w", err)
	}

	// Read index.html into memory for faster serving
	indexHTML, err := fs.ReadFile(webFS, "web/index.html")
	if err != nil {
		return nil, fmt.Errorf("failed to read index.html: %w", err)
	}

	// Read tokens.html into memory for faster serving
	tokensHTML, err := fs.ReadFile(webFS, "web/tokens.html")
	if err != nil {
		return nil, fmt.Errorf("failed to read tokens.html: %w", err)
	}

	// Read proxies.html into memory for faster serving
	proxiesHTML, err := fs.ReadFile(webFS, "web/proxies.html")
	if err != nil {
		return nil, fmt.Errorf("failed to read proxies.html: %w", err)
	}

	return &DashboardHandler{
		logger:      logger,
		webFS:       webFS,
		cssFS:       cssFS,
		jsFS:        jsFS,
		indexHTML:   indexHTML,
		tokensHTML:  tokensHTML,
		proxiesHTML: proxiesHTML,
	}, nil
}

// ServeIndex serves the dashboard index page
func (h *DashboardHandler) ServeIndex(w http.ResponseWriter, r *http.Request) {
	// Only serve index.html for root path
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write(h.indexHTML)
}

// ServeTokens serves the tokens management page
func (h *DashboardHandler) ServeTokens(w http.ResponseWriter, r *http.Request) {
	// Only serve tokens.html for /tokens path
	if r.URL.Path != "/tokens" {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write(h.tokensHTML)
}

// ServeProxies serves the proxy management page
func (h *DashboardHandler) ServeProxies(w http.ResponseWriter, r *http.Request) {
	// Only serve proxies.html for /proxies path
	if r.URL.Path != "/proxies" {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write(h.proxiesHTML)
}

// ServeCSS serves CSS files
func (h *DashboardHandler) ServeCSS(w http.ResponseWriter, r *http.Request) {
	// Remove /css/ prefix from path
	path := strings.TrimPrefix(r.URL.Path, "/css/")
	if path == "" || path == "/" {
		http.NotFound(w, r)
		return
	}

	// Clean the path to prevent directory traversal
	path = filepath.Clean(path)

	// Read file from embedded filesystem
	content, err := fs.ReadFile(h.cssFS, path)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	// Set content type
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write(content)
}

// ServeJS serves JavaScript files
func (h *DashboardHandler) ServeJS(w http.ResponseWriter, r *http.Request) {
	// Remove /js/ prefix from path
	path := strings.TrimPrefix(r.URL.Path, "/js/")
	if path == "" || path == "/" {
		http.NotFound(w, r)
		return
	}

	// Clean the path to prevent directory traversal
	path = filepath.Clean(path)

	// Read file from embedded filesystem
	content, err := fs.ReadFile(h.jsFS, path)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	// Set content type for ES6 modules
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write(content)
}
