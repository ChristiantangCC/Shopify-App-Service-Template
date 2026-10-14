package handlers

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ChristiantangCC/Shopify-App-Service-Template/internal/shopify"
)

// fakeAdmin answers every call with a fixed JSON body (or error) and records
// the variables it was called with.
type fakeAdmin struct {
	response string
	err      error
	vars     map[string]any
}

func (f *fakeAdmin) Do(_ context.Context, _ string, vars map[string]any, out any) error {
	f.vars = vars
	if f.err != nil {
		return f.err
	}
	return json.Unmarshal([]byte(f.response), out)
}

var discard = slog.New(slog.NewTextHandler(io.Discard, nil))

func TestListProducts(t *testing.T) {
	admin := &fakeAdmin{response: `{"products":{"nodes":[{"id":"gid://shopify/Product/1","title":"Shirt","handle":"shirt","tags":["a"]}]}}`}
	rec := httptest.NewRecorder()
	ListProducts{Admin: admin, Logger: discard}.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/proxy/example/products?first=500", nil))

	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"title":"Shirt"`) {
		t.Fatalf("got %d %s", rec.Code, rec.Body)
	}
	if admin.vars["first"] != 5 {
		t.Fatalf("out-of-range first should fall back to 5, got %v", admin.vars["first"])
	}
}

func TestListProductsNotConnected(t *testing.T) {
	rec := httptest.NewRecorder()
	ListProducts{Admin: &fakeAdmin{err: shopify.ErrNotConnected}, Logger: discard}.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("got %d, want 503", rec.Code)
	}
}

func TestCreateProduct(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		response string
		want     int
	}{
		{"creates a draft", `{"title":"  Hello  "}`, `{"productCreate":{"product":{"id":"gid://shopify/Product/1","title":"Hello","handle":"hello","status":"DRAFT"},"userErrors":[]}}`, http.StatusCreated},
		{"bad json", `{`, "", http.StatusBadRequest},
		{"empty title", `{"title":" "}`, "", http.StatusBadRequest},
		{"user errors", `{"title":"x"}`, `{"productCreate":{"product":null,"userErrors":[{"field":["title"],"message":"Title is invalid"}]}}`, http.StatusUnprocessableEntity},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			admin := &fakeAdmin{response: tt.response}
			rec := httptest.NewRecorder()
			CreateProduct{Admin: admin, Logger: discard}.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tt.body)))
			if rec.Code != tt.want {
				t.Fatalf("got %d %s, want %d", rec.Code, rec.Body, tt.want)
			}
			if tt.want == http.StatusCreated {
				input := admin.vars["product"].(map[string]any)
				if input["title"] != "Hello" || input["status"] != "DRAFT" {
					t.Fatalf("sent %v, want trimmed title as a draft", input)
				}
			}
		})
	}
}
