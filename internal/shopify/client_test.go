package shopify

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// testClient returns a connected client that talks to a server answering with
// the given status codes in turn (200 with an empty data object at the end).
func testClient(t *testing.T, statuses ...int) (*Client, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := int(calls.Add(1)) - 1
		if n < len(statuses) && statuses[n] != http.StatusOK {
			w.WriteHeader(statuses[n])
			return
		}
		_, _ = w.Write([]byte(`{"data":{}}`))
	}))
	t.Cleanup(server.Close)

	tokens, err := NewTokenStore(t.TempDir(), "secret")
	if err != nil {
		t.Fatal(err)
	}
	if err := tokens.Save("example-shop.myshopify.com", "shpat_abc"); err != nil {
		t.Fatal(err)
	}
	client := NewClient("example-shop.myshopify.com", "2026-07", tokens)
	client.endpoint = server.URL
	client.backoff = func(int) time.Duration { return time.Millisecond }
	return client, &calls
}

func TestDoRetries(t *testing.T) {
	const query = `query { shop { name } }`
	const mutation = `mutation create($p: ProductCreateInput!) { productCreate(product: $p) { product { id } } }`
	const idempotent = `mutation set($key: String!) { inventorySetQuantities(input: {}) @idempotent(key: $key) { userErrors { message } } }`

	tests := []struct {
		name      string
		document  string
		statuses  []int
		wantErr   bool
		wantCalls int32
	}{
		{"query retried after 503", query, []int{503, 502}, false, 3},
		{"query gives up after 3 retries", query, []int{500, 500, 500, 500}, true, 4},
		{"mutation not retried after 503", mutation, []int{503}, true, 1},
		{"mutation retried after 429", mutation, []int{429}, false, 2},
		{"idempotent mutation retried after 503", idempotent, []int{503}, false, 2},
		{"commented mutation still treated as one", "# create it\n" + mutation, []int{503}, true, 1},
		{"400 not retried", query, []int{400}, true, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, calls := testClient(t, tt.statuses...)
			err := client.Do(context.Background(), tt.document, nil, nil)
			if (err != nil) != tt.wantErr || calls.Load() != tt.wantCalls {
				t.Fatalf("err %v after %d calls, want error %v after %d", err, calls.Load(), tt.wantErr, tt.wantCalls)
			}
		})
	}
}

func TestDoUnauthorizedForgetsToken(t *testing.T) {
	client, _ := testClient(t, 401)
	if err := client.Do(context.Background(), `query { shop { name } }`, nil, nil); err != ErrNotConnected {
		t.Fatalf("got %v, want ErrNotConnected", err)
	}
	if client.tokens.Connected("example-shop.myshopify.com") {
		t.Fatal("token should be dropped after a 401")
	}
}
