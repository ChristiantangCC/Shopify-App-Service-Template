package apikey

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestVerify(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	handler := Verify("s3cret-key", slog.New(slog.NewTextHandler(io.Discard, nil)))(ok)

	tests := []struct {
		name   string
		header string
		want   int
	}{
		{"right key", "Bearer s3cret-key", http.StatusNoContent},
		{"wrong key", "Bearer s3cret-kex", http.StatusUnauthorized},
		{"no scheme", "s3cret-key", http.StatusUnauthorized},
		{"missing", "", http.StatusUnauthorized},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/x", nil)
			if tt.header != "" {
				req.Header.Set("Authorization", tt.header)
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != tt.want {
				t.Fatalf("got %d, want %d", rec.Code, tt.want)
			}
		})
	}
}
