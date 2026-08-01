package config

import (
	"errors"
	"os"
	"strings"
	"time"
)

type Config struct {
	ListenAddress              string
	DatabaseURL                string
	AppOrigin                  string
	ProxyBaseDomain            string
	SessionTTL                 time.Duration
	Development                bool
	BootstrapAdminUsername     string
	BootstrapAdminPasswordFile string
}

func Load() (Config, error) {
	cfg := Config{
		ListenAddress:              env("TVPN_LISTEN_ADDRESS", ":8080"),
		DatabaseURL:                strings.TrimSpace(os.Getenv("TVPN_DATABASE_URL")),
		AppOrigin:                  env("TVPN_APP_ORIGIN", "http://app.localhost:8080"),
		ProxyBaseDomain:            env("TVPN_PROXY_BASE_DOMAIN", "proxy.localhost:8080"),
		SessionTTL:                 12 * time.Hour,
		Development:                os.Getenv("TVPN_ENV") != "production",
		BootstrapAdminUsername:     strings.TrimSpace(os.Getenv("TVPN_BOOTSTRAP_ADMIN_USERNAME")),
		BootstrapAdminPasswordFile: strings.TrimSpace(os.Getenv("TVPN_BOOTSTRAP_ADMIN_PASSWORD_FILE")),
	}
	if cfg.DatabaseURL == "" {
		return Config{}, errors.New("TVPN_DATABASE_URL is required")
	}
	if cfg.BootstrapAdminUsername != "" && cfg.BootstrapAdminPasswordFile == "" {
		return Config{}, errors.New("TVPN_BOOTSTRAP_ADMIN_PASSWORD_FILE is required when bootstrap admin is configured")
	}
	return cfg, nil
}

func env(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}
