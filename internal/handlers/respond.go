package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/ChristiantangCC/Shopify-App-Service-Template/internal/shopify"
)

// Admin is the part of the Shopify client the handlers use (a fake in tests).
type Admin interface {
	Do(ctx context.Context, query string, variables map[string]any, out any) error
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

// failShopify answers an Admin API failure: 503 while the store is not
// connected (open the app in the admin), 502 for anything else.
func failShopify(w http.ResponseWriter, logger *slog.Logger, message string, err error) {
	if errors.Is(err, shopify.ErrNotConnected) {
		logger.Error(message+": not connected, open the app in the store's admin", "error", err)
		writeError(w, http.StatusServiceUnavailable, "service not connected")
		return
	}
	logger.Error(message, "error", err)
	writeError(w, http.StatusBadGateway, message)
}
