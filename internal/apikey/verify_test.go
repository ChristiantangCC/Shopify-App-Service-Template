package apikey

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

var (
	flowKey = strings.Repeat("f", 32)
	erpKey  = strings.Repeat("e", 32)
)

func TestVerify(t *testing.T) {
	var caller string
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		caller, _ = CallerFrom(r.Context())
		w.WriteHeader(http.StatusNoContent)
	})
	keys := []Key{{Name: "flow", Value: flowKey}, {Name: "erp", Value: erpKey}}
	handler := Verify(keys, slog.New(slog.NewTextHandler(io.Discard, nil)))(ok)

	tests := []struct {
		name       string
		header     string
		want       int
		wantCaller string
	}{
		{"first key", "Bearer " + flowKey, http.StatusNoContent, "flow"},
		{"second key", "Bearer " + erpKey, http.StatusNoContent, "erp"},
		{"wrong key", "Bearer " + strings.Repeat("x", 32), http.StatusUnauthorized, ""},
		{"no scheme", flowKey, http.StatusUnauthorized, ""},
		{"missing", "", http.StatusUnauthorized, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			caller = ""
			req := httptest.NewRequest(http.MethodGet, "/api/x", nil)
			if tt.header != "" {
				req.Header.Set("Authorization", tt.header)
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != tt.want || caller != tt.wantCaller {
				t.Fatalf("got %d caller %q, want %d caller %q", rec.Code, caller, tt.want, tt.wantCaller)
			}
		})
	}
}

func TestParse(t *testing.T) {
	keys, err := Parse(flowKey, []string{"erp:" + erpKey, " "})
	if err != nil || len(keys) != 2 || keys[0].Name != "default" || keys[1].Name != "erp" || keys[1].Value != erpKey {
		t.Fatalf("got %+v, %v", keys, err)
	}
	if keys, err := Parse("", nil); err != nil || len(keys) != 0 {
		t.Fatalf("empty config should mean no keys, got %+v, %v", keys, err)
	}

	bad := []struct {
		name   string
		single string
		named  []string
	}{
		{"short key", "short", nil},
		{"no name", "", []string{erpKey}},
		{"empty name", "", []string{":" + erpKey}},
		{"duplicate name", "", []string{"erp:" + erpKey, "erp:" + flowKey}},
	}
	for _, tt := range bad {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Parse(tt.single, tt.named); err == nil {
				t.Fatal("want an error")
			}
		})
	}
}
