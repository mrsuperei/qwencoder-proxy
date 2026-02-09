// Package restapi provides REST API endpoints for OAuth2 authentication flows
package restapi

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/sunbankio/qwencoder-proxy/logging"
)

// ErrorResponse represents a standard error response
type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}

// ErrorDetail represents error details
type ErrorDetail struct {
	Code    string                 `json:"code"`
	Message string                 `json:"message"`
	Details map[string]interface{} `json:"details,omitempty"`
}

// CORS middleware adds CORS headers to responses
func CORS(allowedOrigins []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")

			// Check if origin is allowed
			allowed := false
			for _, allowedOrigin := range allowedOrigins {
				if allowedOrigin == "*" || allowedOrigin == origin {
					allowed = true
					break
				}
			}

			if allowed {
				if allowedOrigins[0] == "*" {
					w.Header().Set("Access-Control-Allow-Origin", "*")
				} else {
					w.Header().Set("Access-Control-Allow-Origin", origin)
				}
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
				w.Header().Set("Access-Control-Max-Age", "86400")
			}

			// Handle preflight requests
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusOK)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// Logging middleware logs HTTP requests
func Logging(logger logging.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// Wrap response writer to capture status code
			wrapped := &responseWriter{ResponseWriter: w}

			next.ServeHTTP(wrapped, r)

			duration := time.Since(start)
			logger.InfoLog("[%s] %s %s - Status: %d - Duration: %v",
				r.Method, r.URL.Path, r.RemoteAddr, wrapped.statusCode, duration)
		})
	}
}

// responseWriter wraps http.ResponseWriter to capture status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// WriteJSON writes a JSON response
func WriteJSON(w http.ResponseWriter, status int, data interface{}) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	return json.NewEncoder(w).Encode(data)
}

// WriteError writes an error response in JSON format
func WriteError(w http.ResponseWriter, status int, code, message string) error {
	return WriteErrorWithDetails(w, status, code, message, nil)
}

// WriteErrorWithDetails writes an error response with additional details
func WriteErrorWithDetails(w http.ResponseWriter, status int, code, message string, details map[string]interface{}) error {
	errResp := ErrorResponse{
		Error: ErrorDetail{
			Code:    code,
			Message: message,
			Details: details,
		},
	}
	return WriteJSON(w, status, errResp)
}

// GetQueryParam retrieves a query parameter from the request
func GetQueryParam(r *http.Request, key string) string {
	return r.URL.Query().Get(key)
}

// ParseJSON parses JSON from the request body
func ParseJSON(r *http.Request, v interface{}) error {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(v); err != nil {
		return fmt.Errorf("failed to parse JSON: %w", err)
	}
	defer r.Body.Close()
	return nil
}

// SimpleRequestLogger is a simple logger that uses the standard log package
// Used when no logger is provided
type SimpleRequestLogger struct{}

// InfoLog logs an info message
func (l *SimpleRequestLogger) InfoLog(format string, args ...interface{}) {
	log.Printf("[INFO] "+format, args...)
}

// ErrorLog logs an error message
func (l *SimpleRequestLogger) ErrorLog(format string, args ...interface{}) {
	log.Printf("[ERROR] "+format, args...)
}

// DebugLog logs a debug message
func (l *SimpleRequestLogger) DebugLog(format string, args ...interface{}) {
	log.Printf("[DEBUG] "+format, args...)
}

// WarningLog logs a warning message
func (l *SimpleRequestLogger) WarningLog(format string, args ...interface{}) {
	log.Printf("[WARN] "+format, args...)
}
