package appproxy

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"
)

const secret = "hush"

var now = time.Unix(1_800_000_000, 0)

func signedQuery(shop string, at time.Time) url.Values {
	query := url.Values{"shop": {shop}, "logged_in_customer_id": {"42"}, "path_prefix": {"/apps/example"}, "timestamp": {itoa(at.Unix())}}
	query.Set("signature", Signature(query, secret))
	return query
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

func serve(t *testing.T, query url.Values) (int, Caller) {
	t.Helper()
	var got Caller
	handler := Verify(VerifyOptions{Secret: secret, Shop: "example-shop.myshopify.com", MaxAge: 5 * time.Minute, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Now: func() time.Time { return now }})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, _ = CallerFrom(r.Context())
	}))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/proxy/customers/tags?"+query.Encode(), nil))
	return recorder.Code, got
}

func TestSignatureMatchesShopifysExamples(t *testing.T) {
	// Shopify's App Proxy docs (authenticate-app-proxies), secret "hush".
	query := url.Values{
		"extra":       {"1", "2"},
		"path_prefix": {"/apps/awesome_reviews"},
		"shop":        {"shop-name.myshopify.com"},
		"timestamp":   {"1317327555"},
	}
	for customerID, want := range map[string]string{
		"":  "e072b6d7e6622d85912a5214b860d3100dc1e73d9bc29f43796ac8c9ff8093cb",
		"1": "4c68c8624d737112c91818c11017d24d334b524cb5c2b8ba08daa056f7395ddb",
	} {
		query.Set("logged_in_customer_id", customerID)
		if got := Signature(query, "hush"); got != want {
			t.Errorf("customer %q: signature = %s, want %s", customerID, got, want)
		}
	}
}

func TestVerifyAcceptsAFreshSignedRequest(t *testing.T) {
	code, caller := serve(t, signedQuery("example-shop.myshopify.com", now.Add(-time.Minute)))
	if code != http.StatusOK || caller.CustomerID != "42" || caller.Shop != "example-shop.myshopify.com" {
		t.Fatalf("code %d caller %+v", code, caller)
	}
}

func TestVerifyRejects(t *testing.T) {
	tampered := signedQuery("example-shop.myshopify.com", now)
	tampered.Set("logged_in_customer_id", "43")
	cases := map[string]url.Values{
		"no signature":   {"shop": {"example-shop.myshopify.com"}},
		"tampered query": tampered,
		"other shop":     signedQuery("evil.myshopify.com", now),
		"stale":          signedQuery("example-shop.myshopify.com", now.Add(-10*time.Minute)),
	}
	for name, query := range cases {
		if code, _ := serve(t, query); code != http.StatusUnauthorized {
			t.Errorf("%s: code %d, want 401", name, code)
		}
	}
}

func TestRateLimitKeyIgnoresThePort(t *testing.T) {
	a := httptest.NewRequest(http.MethodPost, "/", nil)
	a.RemoteAddr = "10.0.0.9:50001"
	b := httptest.NewRequest(http.MethodPost, "/", nil)
	b.RemoteAddr = "10.0.0.9:50002"
	keyA, _ := RateLimitKey(a)
	keyB, _ := RateLimitKey(b)
	if keyA != keyB || keyA != "ip:10.0.0.9" {
		t.Fatalf("keys %q %q", keyA, keyB)
	}
	forwarded := httptest.NewRequest(http.MethodPost, "/", nil)
	forwarded.Header.Set("X-Forwarded-For", "203.0.113.7, 10.0.0.1")
	if key, _ := RateLimitKey(forwarded); key != "ip:203.0.113.7" {
		t.Fatalf("forwarded key %q", key)
	}
}
