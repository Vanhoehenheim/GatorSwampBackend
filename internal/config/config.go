// internal/config/config.go
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/joho/godotenv"
)

// ServerConfig holds all server-related settings
type ServerConfig struct {
	Port           int
	Host           string
	MetricsEnabled bool
}

// Config holds the complete application configuration
type Config struct {
	Server         *ServerConfig
	MongoDBURI     string
	AllowedOrigins []string
	Debug          bool
}

// DefaultConfig provides default server settings
func DefaultConfig() *ServerConfig {
	return &ServerConfig{
		Port:           8080,
		Host:           "localhost",
		MetricsEnabled: true,
	}
}

// LoadConfig loads configuration from environment variables and applies defaults
func LoadConfig() (*Config, error) {
	// Try to load .env file from multiple possible locations
	envLocations := []string{
		".env",          // Current directory
		"../../.env",    // Project root when running from cmd/engine
		"../../../.env", // Even higher directory
		filepath.Join(os.Getenv("GOPATH"), "src/gator-swamp/.env"), // GOPATH location
	}

	envLoaded := false
	for _, location := range envLocations {
		if err := godotenv.Load(location); err == nil {
			envLoaded = true
			break
		}
	}

	if !envLoaded {
		// If we couldn't find a .env file, try loading without a path
		// This is a silent failure if no .env exists, which is fine
		_ = godotenv.Load()
	}

	// Start with default server config
	serverConfig := DefaultConfig()

	// Override server settings from environment if provided
	if portStr := os.Getenv("PORT"); portStr != "" {
		if port, err := strconv.Atoi(portStr); err == nil {
			serverConfig.Port = port
		}
	}

	if host := os.Getenv("HOST"); host != "" {
		serverConfig.Host = host
	}

	if metricsEnabled := os.Getenv("METRICS_ENABLED"); metricsEnabled != "" {
		serverConfig.MetricsEnabled = metricsEnabled == "true"
	}

	// Get MongoDB URI from environment variable
	mongoURI := os.Getenv("MONGODB_URI")
	if mongoURI == "" {
		return nil, fmt.Errorf("MONGODB_URI environment variable is required")
	}

	// Initialize complete config
	config := &Config{
		Server:         serverConfig,
		MongoDBURI:     mongoURI,
		AllowedOrigins: []string{"*"}, // Default to allow all origins
		Debug:          false,
	}

	// Override remaining settings from environment if provided
	if origins := os.Getenv("ALLOWED_ORIGINS"); origins != "" {
		config.AllowedOrigins = []string{origins}
	}

	if debug := os.Getenv("DEBUG"); debug == "true" {
		config.Debug = true
	}

	return config, nil
}
