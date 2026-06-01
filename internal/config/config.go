package config

import (
	"fmt"
	"time"

	"github.com/kelseyhightower/envconfig"
)

// Holds all configurable values for the application.
type Config struct {
	ServerAddr        string        `envconfig:"DEPLOYCRANE_SERVER_ADDR" default:"localhost"`
	ListenPort        string        `envconfig:"DEPLOYCRANE_SERVER_PORT" default:"8080"`
	DBPath            string        `envconfig:"DEPLOYCRANE_DB_PATH" default:"deploycrane.db"`
	CloneBasePath     string        `envconfig:"DEPLOYCRANE_CLONE_BASE_PATH" default:"./clones"`
	ShutdownTimeout   time.Duration `envconfig:"DEPLOYCRANE_SHUTDOWN_TIMEOUT" default:"30s"`
	ReadTimeout       time.Duration `envconfig:"DEPLOYCRANE_HTTP_READ_TIMEOUT" default:"10s"`
	WriteTimeout      time.Duration `envconfig:"DEPLOYCRANE_HTTP_WRITE_TIMEOUT" default:"30m"`
	IdleTimeout       time.Duration `envconfig:"DEPLOYCRANE_HTTP_IDLE_TIMEOUT" default:"60s"`
	ImagePrefix       string        `envconfig:"DEPLOYCRANE_IMAGE_PREFIX" default:"deploycrane"`
	ContainerPort     int           `envconfig:"DEPLOYCRANE_CONTAINER_PORT" default:"8080"`
	PortAllocationMin int           `envconfig:"DEPLOYCRANE_PORT_ALLOCATION_Min" default:"8100"`
	PortAllocationMax int           `envconfig:"DEPLOYCRANE_PORT_ALLOCATION_Max" default:"9000"`
	CORSOrigins       string        `envconfig:"DEPLOYCRANE_CORS_ORIGINS" default:"http://localhost:3000"`
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
