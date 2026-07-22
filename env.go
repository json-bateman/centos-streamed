// Package streamed holds process-wide configuration for the centos-streamed
// web server. It mirrors the layout of the basicauth reference project but
// carries no authentication settings.
package streamed

import (
	"log/slog"
	"os"

	"github.com/joho/godotenv"
	"github.com/kelseyhightower/envconfig"
)

type Config struct {
	Port   int    `envconfig:"PORT" default:"8080"`
	DBPath string `envconfig:"DB_PATH" default:"./data/streamed.db"`
}

// The host facts injected by the platform CLI (SERVER_NAME, SERVER_OS,
// SERVER_KERNEL) are read directly in the web package rather than through
// envconfig, since they are set without the STREAMED_ prefix.

var Env Config

// LoadSettings loads a .env file if present, then fills Env from the
// environment. There are no required secrets, so a missing .env is harmless.
func LoadSettings() *Config {
	if err := godotenv.Load(); err != nil {
		slog.Warn(".env file not found, using system environment variables instead")
	}

	if err := envconfig.Process("streamed", &Env); err != nil {
		slog.Error("Failed to load environment configuration", "error", err)
		os.Exit(1)
	}

	return &Env
}
