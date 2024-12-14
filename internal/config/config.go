// internal/config/config.go
package config

import (
	"fmt"
	"os"
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
	// Load .env file if it exists
	err := godotenv.Load()
	if err != nil {
		// Don't return error if .env doesn't exist
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("error loading .env file: %v", err)
		}
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
