// internal/middleware/jwt.go
package middleware

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

const (
	// JWT secret key for signing tokens
	// In production, this should be loaded from environment variables or a secure vault
	jwtSecret = "gatorswamp_secret_key_should_be_loaded_from_env"

	// Token expiration time - 24 hours
	tokenExpiration = 24 * time.Hour
)

// Claims represents the JWT claims for our application
type Claims struct {
	UserID uuid.UUID `json:"user_id"`
	jwt.RegisteredClaims
}

// UnprotectedRoutes defines routes that don't require JWT authentication
var UnprotectedRoutes = map[string]bool{
	"/health":        true,
	"/user/register": true,
	"/user/login":    true,
}

// GenerateToken creates a new JWT token for the given user ID
func GenerateToken(userID uuid.UUID) (string, error) {
	// Create token expiration time
	expirationTime := time.Now().Add(tokenExpiration)

	// Create claims with user ID and standard claims
	claims := &Claims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expirationTime),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
			Issuer:    "gator-swamp-api",
			Subject:   userID.String(),
		},
	}

	// Create token with claims and signing method
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	// Sign token with secret key
	tokenString, err := token.SignedString([]byte(jwtSecret))
	if err != nil {
		return "", err
	}

	return tokenString, nil
}

// ValidateToken validates the provided JWT token
func ValidateToken(tokenString string) (*Claims, error) {
	// Parse token with claims
	token, err := jwt.ParseWithClaims(
		tokenString,
		&Claims{},
		func(token *jwt.Token) (interface{}, error) {
			// Verify signing method
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			return []byte(jwtSecret), nil
		},
	)

	if err != nil {
		return nil, err
	}

	// Validate token and extract claims
	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		return claims, nil
	}

	return nil, errors.New("invalid token")
}

// AuthMiddleware is a middleware function to validate JWT tokens
func AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if the route is protected
		if UnprotectedRoutes[r.URL.Path] {
			next.ServeHTTP(w, r)
			return
		}

		// Extract Authorization header
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "Authorization header required", http.StatusUnauthorized)
			return
		}

		// Check for Bearer token format
		if !strings.HasPrefix(authHeader, "Bearer ") {
			http.Error(w, "Invalid authorization format", http.StatusUnauthorized)
			return
		}

		// Extract token from header
		tokenString := strings.TrimPrefix(authHeader, "Bearer ")

		// Validate token
		claims, err := ValidateToken(tokenString)
		if err != nil {
			http.Error(w, fmt.Sprintf("Invalid token: %v", err), http.StatusUnauthorized)
			return
		}

		// Check if token is expired
		if time.Now().After(claims.ExpiresAt.Time) {
			http.Error(w, "Token expired", http.StatusUnauthorized)
			return
		}

		// Set user ID in request context
		ctx := r.Context()
		ctx = SetUserIDInContext(ctx, claims.UserID)

		// Continue with request
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// ApplyJWTMiddleware wraps a handler function with JWT authentication
func ApplyJWTMiddleware(handler http.HandlerFunc, path string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Skip JWT validation for unprotected routes
		if UnprotectedRoutes[path] {
			handler(w, r)
			return
		}

		// Extract Authorization header
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "Authorization header required", http.StatusUnauthorized)
			return
		}

		// Check for Bearer token format
		if !strings.HasPrefix(authHeader, "Bearer ") {
			http.Error(w, "Invalid authorization format", http.StatusUnauthorized)
			return
		}

		// Extract token from header
		tokenString := strings.TrimPrefix(authHeader, "Bearer ")

		// Validate token
		claims, err := ValidateToken(tokenString)
		if err != nil {
			log.Printf("JWT Error: %v", err)
			http.Error(w, "Invalid token", http.StatusUnauthorized)
			return
		}

		// Check if token is expired
		if time.Now().After(claims.ExpiresAt.Time) {
			http.Error(w, "Token expired", http.StatusUnauthorized)
			return
		}

		// Set user ID in request context
		ctx := r.Context()
		ctx = SetUserIDInContext(ctx, claims.UserID)

		// Continue with handler
		handler(w, r.WithContext(ctx))
	}
}

// Define a custom context key type to avoid collisions
type contextKey string

// UserIDKey is the key used to store the user ID in the context
const UserIDKey contextKey = "user_id"

// SetUserIDInContext saves the user ID in the request context
func SetUserIDInContext(ctx context.Context, userID uuid.UUID) context.Context {
	return context.WithValue(ctx, UserIDKey, userID)
}

// GetUserIDFromContext retrieves the user ID from the context
func GetUserIDFromContext(ctx context.Context) (uuid.UUID, bool) {
	userID, ok := ctx.Value(UserIDKey).(uuid.UUID)
	return userID, ok
}

// NewRouterWithMiddleware creates a new router with JWT and CORS middleware
func NewRouterWithMiddleware() *http.ServeMux {
	mux := http.NewServeMux()
	return mux
}
