// Package apikey guards the /api routes for server-to-server callers: Shopify
// Flow's "Send HTTP request" action, a backend, a cron job. They can keep a
// secret, so they send it as `Authorization: Bearer <key>`.
//
// Never use /api from storefront or browser code: anything shipped to a
// browser is public, key included. Browsers go through the App Proxy (/proxy).
package apikey

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
)

// Key is one accepted API key. Name tells callers apart in the logs (e.g.
// "flow", "erp"), so each caller can get its own key and keys can be rotated
// one at a time.
type Key struct {
	Name  string
	Value string
}

// MinLength is the shortest key accepted.
const MinLength = 32

// Parse reads API_KEY (one unnamed key, named "default") and API_KEYS
// ("name:key,name:key") into a list. Both may be empty: /api is then off.
func Parse(single string, named []string) ([]Key, error) {
	var keys []Key
	if single = strings.TrimSpace(single); single != "" {
		keys = append(keys, Key{Name: "default", Value: single})
	}
	for _, entry := range named {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		name, value, ok := strings.Cut(entry, ":")
		if !ok || strings.TrimSpace(name) == "" {
			return nil, fmt.Errorf("API_KEYS entries must look like name:key, got one without a name")
		}
		keys = append(keys, Key{Name: strings.TrimSpace(name), Value: strings.TrimSpace(value)})
	}
	seen := map[string]bool{}
	for _, key := range keys {
		if len(key.Value) < MinLength {
			return nil, fmt.Errorf("API key %q must be at least %d characters (try: openssl rand -hex 32)", key.Name, MinLength)
		}
		if seen[key.Name] {
			return nil, fmt.Errorf("API key name %q is used twice", key.Name)
		}
		seen[key.Name] = true
	}
	return keys, nil
}

type callerKey struct{}

// CallerFrom returns the name of the key the request was made with.
func CallerFrom(ctx context.Context) (string, bool) {
	name, ok := ctx.Value(callerKey{}).(string)
	return name, ok
}

// Verify rejects requests whose bearer token is none of keys, with 401, and
// logs which key every accepted request used.
func Verify(keys []Key, logger *slog.Logger) func(http.Handler) http.Handler {
	// Hashing both sides makes each comparison constant time regardless of the
	// given token's length.
	hashes := make([][32]byte, len(keys))
	for i, key := range keys {
		hashes[i] = sha256.Sum256([]byte(key.Value))
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			given, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
			got := sha256.Sum256([]byte(strings.TrimSpace(given)))
			match := -1
			// Compare against every key, so the time taken doesn't reveal
			// which one matched.
			for i := range hashes {
				if subtle.ConstantTimeCompare(got[:], hashes[i][:]) == 1 {
					match = i
				}
			}
			if !ok || match < 0 {
				logger.Warn("[APIKey] Rejected request", "path", r.URL.Path)
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			caller := keys[match].Name
			logger.Info("[APIKey] Call", "caller", caller, "method", r.Method, "path", r.URL.Path)
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), callerKey{}, caller)))
		})
	}
}
