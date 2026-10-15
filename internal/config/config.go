// Package config reads the service's settings from environment variables.
package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/caarlos0/env/v11"

	"github.com/ChristiantangCC/Shopify-Go-Service-App-Template/internal/apikey"
)

// Config holds everything the service needs at start.
type Config struct {
	// Port the HTTP server listens on.
	Port int `env:"PORT" envDefault:"8080"`

	// Shop is the store's myshopify domain, e.g. "example-shop.myshopify.com".
	Shop string `env:"SHOPIFY_SHOP,required"`
	// ClientID and ClientSecret of the app (Dev Dashboard). The secret verifies
	// the admin's session token and App Proxy signatures, and encrypts the
	// stored access token.
	ClientID     string `env:"SHOPIFY_CLIENT_ID,required"`
	ClientSecret string `env:"SHOPIFY_CLIENT_SECRET,required"`
	// APIVersion of the Admin API.
	APIVersion string `env:"SHOPIFY_API_VERSION" envDefault:"2026-07"`

	// DataDir holds the encrypted access token file (a Docker volume in production).
	DataDir string `env:"DATA_DIR" envDefault:"/data"`
	// ProxyMaxAge is how old an App Proxy request's timestamp may be.
	ProxyMaxAge time.Duration `env:"PROXY_MAX_AGE" envDefault:"5m"`
	// RateLimitPerMinute is the number of proxy requests one caller may make per minute.
	RateLimitPerMinute int `env:"RATE_LIMIT_PER_MINUTE" envDefault:"30"`
	// APIKey and APIKeys enable the /api routes for server-to-server callers
	// (Shopify Flow, backends), sent as `Authorization: Bearer <key>`. API_KEY
	// is one key; API_KEYS is "name:key,name:key" for several callers or a
	// rotation. Both empty = /api off.
	APIKey  string   `env:"API_KEY"`
	APIKeys []string `env:"API_KEYS" envSeparator:","`
	// Keys is APIKey and APIKeys parsed and validated.
	Keys []apikey.Key `env:"-"`

	// ServiceNamespace names the service in logs (OpenTelemetry service.namespace), e.g. "acme-reviews-service".
	ServiceNamespace string `env:"SERVICE_NAMESPACE" envDefault:"shopify-app-service"`
	// OTLPEndpoint enables OpenTelemetry logs (e.g. to Grafana Loki) when set, e.g. http://otel-collector:4318.
	OTLPEndpoint string `env:"OTEL_EXPORTER_OTLP_ENDPOINT"`
	// LogLevel is debug, info, warn or error.
	LogLevel string `env:"LOG_LEVEL" envDefault:"info"`
	// LogFormat is json (production) or text (readable, for local development).
	LogFormat string `env:"LOG_FORMAT" envDefault:"json"`
}

// Load reads and validates the environment.
func Load() (Config, error) {
	var cfg Config
	if err := env.Parse(&cfg); err != nil {
		return cfg, err
	}
	cfg.Shop = strings.ToLower(strings.TrimSpace(cfg.Shop))
	if !strings.HasSuffix(cfg.Shop, ".myshopify.com") {
		return cfg, fmt.Errorf("SHOPIFY_SHOP must be a myshopify domain, got %q", cfg.Shop)
	}
	keys, err := apikey.Parse(cfg.APIKey, cfg.APIKeys)
	if err != nil {
		return cfg, err
	}
	cfg.Keys = keys
	return cfg, nil
}
