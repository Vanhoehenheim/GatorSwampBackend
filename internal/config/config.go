package config

// ServerConfig holds all server-related settings
type ServerConfig struct {
	Port           int
	Host           string
	MetricsEnabled bool
}

// DefaultConfig provides default server settings
func DefaultConfig() *ServerConfig {
	return &ServerConfig{
		Port:           8080,
		Host:           "localhost",
		MetricsEnabled: true,
	}
}

type Config struct {
	Host string
	Port int
	// Add these fields
	AllowedOrigins []string
	Debug          bool
}
