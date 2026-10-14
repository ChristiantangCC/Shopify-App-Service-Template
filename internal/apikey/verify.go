// Package apikey guards the /api routes for server-to-server callers: Shopify
// Flow's "Send HTTP request" action, a backend, a cron job. They can keep a
// secret, so they send it as `Authorization: Bearer <API_KEY>`.
//
// Never use /api from storefront or browser code: anything shipped to a
// browser is public, key included. Browsers go through the App Proxy (/proxy).
package apikey

import (
	"crypto/sha256"
	"crypto/subtle"
	"log/slog"
	"net/http"
	"strings"
)

// Verify rejects requests whose bearer token is not key, with 401.
func Verify(key string, logger *slog.Logger) func(http.Handler) http.Handler {
	// Hashing both sides makes the comparison constant time regardless of the
	// given token's length.
	want := sha256.Sum256([]byte(key))
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			given, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
			got := sha256.Sum256([]byte(strings.TrimSpace(given)))
			if !ok || subtle.ConstantTimeCompare(got[:], want[:]) != 1 {
				logger.Warn("[APIKey] Rejected request", "path", r.URL.Path)
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
