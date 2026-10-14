// Command server runs a Shopify app service: a small HTTP service that holds
// one store's Admin API access token and exposes a few endpoints that use it.
// Storefronts call it through the App Proxy (/proxy), servers such as Shopify
// Flow with an API key (/api). See README.md.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/httprate"

	"github.com/ChristiantangCC/Shopify-App-Service-Template/internal/apikey"
	"github.com/ChristiantangCC/Shopify-App-Service-Template/internal/appproxy"
	"github.com/ChristiantangCC/Shopify-App-Service-Template/internal/config"
	"github.com/ChristiantangCC/Shopify-App-Service-Template/internal/handlers"
	"github.com/ChristiantangCC/Shopify-App-Service-Template/internal/logging"
	"github.com/ChristiantangCC/Shopify-App-Service-Template/internal/shopify"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	logger, shutdownLogs, err := logging.New(ctx, logging.Options{Level: cfg.LogLevel, Format: cfg.LogFormat, ServiceNamespace: cfg.ServiceNamespace, OTLPEndpoint: cfg.OTLPEndpoint})
	if err != nil {
		return fmt.Errorf("logging: %w", err)
	}
	slog.SetDefault(logger)

	tokens, err := shopify.NewTokenStore(cfg.DataDir, cfg.ClientSecret)
	if err != nil {
		return err
	}
	if !tokens.Connected(cfg.Shop) {
		logger.Warn("[Server] Not connected to Shopify yet: open the app in the store's admin once", "shop", cfg.Shop)
	}
	admin := shopify.NewClient(cfg.Shop, cfg.APIVersion, tokens)

	router := chi.NewRouter()
	router.Use(middleware.RequestID, middleware.Recoverer, middleware.Timeout(30*time.Second))
	router.Get("/health", handlers.Health)
	// The App URL: opened in the admin, it connects the store (token exchange).
	router.Method(http.MethodGet, "/", handlers.Install{Shop: cfg.Shop, ClientID: cfg.ClientID, ClientSecret: cfg.ClientSecret, Tokens: tokens, Logger: logger})

	// The service's endpoints, mounted under both /proxy and /api below.
	// Example endpoints (read_products, write_products): replace with the
	// service's own, and delete internal/handlers/example_products.go.
	endpoints := func(r chi.Router) {
		r.Method(http.MethodGet, "/example/products", handlers.ListProducts{Admin: admin, Logger: logger})
		r.Method(http.MethodPost, "/example/products", handlers.CreateProduct{Admin: admin, Logger: logger})
	}

	// Storefront and browser callers: only reachable through Shopify's App
	// Proxy, which signs every request. Rate limited per customer, else per IP.
	router.Route("/proxy", func(r chi.Router) {
		r.Use(appproxy.Verify(appproxy.VerifyOptions{Secret: cfg.ClientSecret, Shop: cfg.Shop, MaxAge: cfg.ProxyMaxAge, Logger: logger}))
		r.Use(httprate.Limit(cfg.RateLimitPerMinute, time.Minute, httprate.WithKeyFuncs(appproxy.RateLimitKey)))
		endpoints(r)
	})

	// Server-to-server callers (Shopify Flow, backends): a bearer API key.
	// Off unless API_KEY is set.
	if cfg.APIKey != "" {
		router.Route("/api", func(r chi.Router) {
			r.Use(apikey.Verify(cfg.APIKey, logger))
			endpoints(r)
		})
	}

	server := &http.Server{Addr: fmt.Sprintf(":%d", cfg.Port), Handler: router, ReadHeaderTimeout: 10 * time.Second}
	serverErrors := make(chan error, 1)
	go func() {
		logger.Info("[Server] Listening", "port", cfg.Port, "shop", cfg.Shop)
		serverErrors <- server.ListenAndServe()
	}()

	select {
	case err := <-serverErrors:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
		logger.Info("[Server] Shutting down")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("[Server] Shutdown failed", "error", err)
	}
	_ = shutdownLogs(shutdownCtx)
	return nil
}
