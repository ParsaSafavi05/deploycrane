package config

import (
	"fmt"
	"time"

	"github.com/kelseyhightower/envconfig"
)

// Holds all configurable values for the application.
type Config struct {
	ServerAddr        string        `envconfig:"SERVER_ADDR" default:"0.0.0.0"`
	ListenPort        string        `envconfig:"SERVER_PORT" default:"8080"`
	DBPath            string        `envconfig:"DB_PATH" default:"deploycrane.db"`
	CloneBasePath     string        `envconfig:"CLONE_BASE_PATH" default:"./clones"`
	ShutdownTimeout   time.Duration `envconfig:"SHUTDOWN_TIMEOUT" default:"30s"`
	ReadTimeout       time.Duration `envconfig:"HTTP_READ_TIMEOUT" default:"10s"`
	WriteTimeout      time.Duration `envconfig:"HTTP_WRITE_TIMEOUT" default:"30m"`
	IdleTimeout       time.Duration `envconfig:"HTTP_IDLE_TIMEOUT" default:"60s"`
	ImagePrefix       string        `envconfig:"IMAGE_PREFIX" default:"deploycrane"`
	ContainerPort     int           `envconfig:"CONTAINER_PORT" default:"8080"`
	PortAllocationMin int           `envconfig:"PORT_ALLOCATION_Min" default:"8100"`
	PortAllocationMax int           `envconfig:"PORT_ALLOCATION_Max" default:"9000"`
	CORSOrigins       string        `envconfig:"CORS_ORIGINS" default:"*"`
}

// Load reads the configuration from environment variables and returns a populated Config.
func Load() (*Config, error) {
	var cfg Config
	err := envconfig.Process("", &cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}
	return &cfg, nil
}
