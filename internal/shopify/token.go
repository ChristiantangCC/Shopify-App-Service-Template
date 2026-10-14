// Package shopify talks to the Shopify Admin API with the store's offline
// access token, obtained once when the app is first opened in the admin.
package shopify

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ErrNotConnected means no token is stored yet: the app has not been opened
// in the store's admin since it was installed (or Shopify rejected the token).
var ErrNotConnected = errors.New("shopify: not connected, open the app in the store's admin once")

// TokenStore keeps the store's offline access token in memory and in an
// encrypted file (AES-GCM, key derived from the client secret), so it survives
// restarts and deploys without a database.
type TokenStore struct {
	path string
	key  [32]byte

	mu    sync.RWMutex
	shop  string
	token string
}

type storedToken struct {
	Shop    string    `json:"shop"`
	Token   string    `json:"token"`
	SavedAt time.Time `json:"savedAt"`
}

// NewTokenStore opens the token file in dir; a missing or unreadable file (a
// changed client secret) means "not connected yet".
func NewTokenStore(dir, clientSecret string) (*TokenStore, error) {
	store := &TokenStore{path: filepath.Join(dir, "shopify-token.enc"), key: sha256.Sum256([]byte("shopify-app-service-token:" + clientSecret))}
	raw, err := os.ReadFile(store.path)
	if errors.Is(err, os.ErrNotExist) {
		return store, nil
	}
	if err != nil {
		return nil, fmt.Errorf("token store: %w", err)
	}
	if saved, err := store.decrypt(raw); err == nil {
		store.shop, store.token = saved.Shop, saved.Token
	}
	return store, nil
}

// Token returns the stored token for shop.
func (s *TokenStore) Token(shop string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.token == "" || s.shop != shop {
		return "", ErrNotConnected
	}
	return s.token, nil
}

// Connected reports whether a token is stored for shop.
func (s *TokenStore) Connected(shop string) bool {
	_, err := s.Token(shop)
	return err == nil
}

// Save stores a new token for shop (file first, then memory).
func (s *TokenStore) Save(shop, token string) error {
	raw, err := s.encrypt(storedToken{Shop: shop, Token: token, SavedAt: time.Now().UTC()})
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("token store: %w", err)
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return fmt.Errorf("token store: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return fmt.Errorf("token store: %w", err)
	}
	s.mu.Lock()
	s.shop, s.token = shop, token
	s.mu.Unlock()
	return nil
}

// Forget drops the token after Shopify rejected it (app uninstalled). The
// next opening of the app in the admin connects again.
func (s *TokenStore) Forget() {
	s.mu.Lock()
	s.token = ""
	s.mu.Unlock()
	_ = os.Remove(s.path)
}

func (s *TokenStore) encrypt(value storedToken) ([]byte, error) {
	plain, _ := json.Marshal(value)
	gcm, err := s.aead()
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, plain, nil), nil
}

func (s *TokenStore) decrypt(raw []byte) (storedToken, error) {
	var value storedToken
	gcm, err := s.aead()
	if err != nil {
		return value, err
	}
	if len(raw) < gcm.NonceSize() {
		return value, errors.New("token store: file too short")
	}
	plain, err := gcm.Open(nil, raw[:gcm.NonceSize()], raw[gcm.NonceSize():], nil)
	if err != nil {
		return value, err
	}
	return value, json.Unmarshal(plain, &value)
}

func (s *TokenStore) aead() (cipher.AEAD, error) {
	block, err := aes.NewCipher(s.key[:])
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}
