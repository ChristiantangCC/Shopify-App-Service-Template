// Package appproxy verifies requests that Shopify forwards through the app's
// App Proxy (https://<store>/apps/<subpath>/... → this service).
//
// Storefront code is public, so the storefront cannot hold a secret. Instead
// Shopify signs every forwarded request: it adds shop, logged_in_customer_id,
// path_prefix and timestamp to the query string, and a `signature` that is the
// HMAC-SHA256 of those parameters with the app's client secret. Only Shopify
// and this service know the secret.
package appproxy

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Caller is what a verified App Proxy request tells about who is calling.
type Caller struct {
	Shop string
	// CustomerID is the logged-in storefront customer's numeric id, "" when anonymous.
	CustomerID string
}

type callerKey struct{}

// CallerFrom returns the verified caller placed in ctx by Verify.
func CallerFrom(ctx context.Context) (Caller, bool) {
	caller, ok := ctx.Value(callerKey{}).(Caller)
	return caller, ok
}

// Signature computes the App Proxy signature of a query string: every
// parameter except `signature`, sorted by key, as "key=value" (multiple values
// joined with ","), concatenated without separator, HMAC-SHA256 with the
// secret, hex encoded.
func Signature(query url.Values, secret string) string {
	keys := make([]string, 0, len(query))
	for key := range query {
		if key != "signature" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	var message strings.Builder
	for _, key := range keys {
		message.WriteString(key)
		message.WriteString("=")
		message.WriteString(strings.Join(query[key], ","))
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(message.String()))
	return hex.EncodeToString(mac.Sum(nil))
}

// VerifyOptions configures Verify.
type VerifyOptions struct {
	Secret string
	// Shop is the only store whose requests are accepted.
	Shop string
	// MaxAge is how old the signed timestamp may be (replay protection).
	MaxAge time.Duration
	Logger *slog.Logger
	// Now is overridable in tests.
	Now func() time.Time
}

// Verify is middleware that lets a request through only when it carries a
// valid, fresh App Proxy signature for the configured shop. Anything else gets
// 401 without detail.
func Verify(opts VerifyOptions) func(http.Handler) http.Handler {
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			query := r.URL.Query()
			reject := func(reason string) {
				opts.Logger.Warn("[AppProxy] Rejected request", "reason", reason, "path", r.URL.Path)
				http.Error(w, "unauthorized", http.StatusUnauthorized)
			}

			given, err := hex.DecodeString(query.Get("signature"))
			if err != nil || len(given) == 0 {
				reject("missing or malformed signature")
				return
			}
			expected, _ := hex.DecodeString(Signature(query, opts.Secret))
			if !hmac.Equal(given, expected) {
				reject("bad signature")
				return
			}
			if query.Get("shop") != opts.Shop {
				reject("other shop")
				return
			}
			timestamp, err := strconv.ParseInt(query.Get("timestamp"), 10, 64)
			if err != nil {
				reject("bad timestamp")
				return
			}
			if age := now().Sub(time.Unix(timestamp, 0)); age > opts.MaxAge || age < -opts.MaxAge {
				reject("stale timestamp")
				return
			}

			caller := Caller{Shop: opts.Shop, CustomerID: query.Get("logged_in_customer_id")}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), callerKey{}, caller)))
		})
	}
}

// RateLimitKey keys rate limiting on the logged-in customer, otherwise the
// visitor's IP. Behind the App Proxy and Traefik, the visitor's IP is the first
// entry of X-Forwarded-For.
func RateLimitKey(r *http.Request) (string, error) {
	if caller, ok := CallerFrom(r.Context()); ok && caller.CustomerID != "" {
		return "customer:" + caller.CustomerID, nil
	}
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		return "ip:" + strings.TrimSpace(strings.Split(forwarded, ",")[0]), nil
	}
	// RemoteAddr is "ip:port", and the port differs per connection.
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return "ip:" + host, nil
	}
	return "ip:" + r.RemoteAddr, nil
}
