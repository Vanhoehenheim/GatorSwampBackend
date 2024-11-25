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
