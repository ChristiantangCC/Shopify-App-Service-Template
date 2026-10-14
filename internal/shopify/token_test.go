package shopify

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestTokenStoreSurvivesARestartEncrypted(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewTokenStore(dir, "secret")
	if store.Connected("example-shop.myshopify.com") {
		t.Fatal("connected before any token")
	}
	if err := store.Save("example-shop.myshopify.com", "shpat_abc"); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "shopify-token.enc"))
	if bytes.Contains(raw, []byte("shpat_abc")) {
		t.Fatal("token stored in plain text")
	}
	reopened, _ := NewTokenStore(dir, "secret")
	if token, err := reopened.Token("example-shop.myshopify.com"); err != nil || token != "shpat_abc" {
		t.Fatalf("after restart: %q %v", token, err)
	}
	if _, err := reopened.Token("other.myshopify.com"); err != ErrNotConnected {
		t.Fatal("token handed out for another shop")
	}
	if withOtherSecret, _ := NewTokenStore(dir, "rotated"); withOtherSecret.Connected("example-shop.myshopify.com") {
		t.Fatal("read with another secret")
	}
	reopened.Forget()
	if again, _ := NewTokenStore(dir, "secret"); again.Connected("example-shop.myshopify.com") {
		t.Fatal("token kept after Forget")
	}
}
