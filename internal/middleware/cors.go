package middleware

import (
	"net/http"
	"strings"
)

// CORSConfig holds configuration for CORS middleware
type CORSConfig struct {
	AllowedOrigins   []string
	AllowedMethods   []string
	AllowedHeaders   []string
	ExposedHeaders   []string
	AllowCredentials bool
	MaxAge           int
}

// DefaultCORSConfig returns a default CORS configuration
func DefaultCORSConfig() *CORSConfig {
	return &CORSConfig{
		AllowedOrigins: []string{"*"},
		AllowedMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS", "PATCH"},
		AllowedHeaders: []string{"Content-Type", "Authorization", "Accept", "Origin", "X-Requested-With"},
		ExposedHeaders: []string{"Content-Length", "Content-Type"},
		MaxAge:         86400, // 24 hours
	}
}

// CORSMiddleware configures CORS for all requests
func CORSMiddleware(config *CORSConfig) func(http.Handler) http.Handler {
	if config == nil {
		config = DefaultCORSConfig()
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")

			// Check if the origin is allowed
			originAllowed := false
			for _, allowedOrigin := range config.AllowedOrigins {
				if allowedOrigin == "*" || allowedOrigin == origin {
					originAllowed = true
					break
				}
			}

			if originAllowed {
				// Set CORS headers
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Methods", strings.Join(config.AllowedMethods, ", "))
				w.Header().Set("Access-Control-Allow-Headers", strings.Join(config.AllowedHeaders, ", "))
				w.Header().Set("Access-Control-Expose-Headers", strings.Join(config.ExposedHeaders, ", "))
				w.Header().Set("Access-Control-Max-Age", string(config.MaxAge))

				if config.AllowCredentials {
					w.Header().Set("Access-Control-Allow-Credentials", "true")
				}

				// Handle preflight requests
				if r.Method == http.MethodOptions {
					w.WriteHeader(http.StatusOK)
					return
				}
			}

			// Call the next handler
			next.ServeHTTP(w, r)
		})
	}
}

// ApplyCORS wraps a handler with CORS headers
func ApplyCORS(handler http.HandlerFunc, config *CORSConfig) http.HandlerFunc {
	if config == nil {
		config = DefaultCORSConfig()
	}

	return func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")

		// Check if the origin is allowed
		originAllowed := false
		for _, allowedOrigin := range config.AllowedOrigins {
			if allowedOrigin == "*" || allowedOrigin == origin {
				originAllowed = true
				break
			}
		}

		if originAllowed {
			// Set CORS headers
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", strings.Join(config.AllowedMethods, ", "))
			w.Header().Set("Access-Control-Allow-Headers", strings.Join(config.AllowedHeaders, ", "))
			w.Header().Set("Access-Control-Expose-Headers", strings.Join(config.ExposedHeaders, ", "))
			w.Header().Set("Access-Control-Max-Age", string(config.MaxAge))

			if config.AllowCredentials {
				w.Header().Set("Access-Control-Allow-Credentials", "true")
			}

			// Handle preflight requests
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusOK)
				return
			}
		}

		// Call the handler
		handler(w, r)
	}
}
