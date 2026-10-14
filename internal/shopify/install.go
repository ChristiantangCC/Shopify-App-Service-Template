package shopify

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// SessionClaims are the parts of Shopify's session token (the `id_token` the
// admin passes when it opens the app) this service checks.
type SessionClaims struct {
	Dest string `json:"dest"`
	Aud  string `json:"aud"`
	Exp  int64  `json:"exp"`
	Nbf  int64  `json:"nbf"`
}

// Shop is the myshopify domain from the token's `dest` (https://<shop>).
func (c SessionClaims) Shop() string {
	return strings.TrimPrefix(c.Dest, "https://")
}

// leeway tolerates small clock differences between Shopify and the box.
const leeway = 10 * time.Second

// VerifySessionToken checks a Shopify session token: HS256 signed with the
// app's client secret, issued for this app (aud = client id), and valid now.
func VerifySessionToken(token, clientID, clientSecret string, now time.Time) (SessionClaims, error) {
	var claims SessionClaims
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return claims, errors.New("session token: malformed")
	}
	header, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil || !strings.Contains(string(header), `"HS256"`) {
		return claims, errors.New("session token: unexpected algorithm")
	}
	mac := hmac.New(sha256.New, []byte(clientSecret))
	mac.Write([]byte(parts[0] + "." + parts[1]))
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || !hmac.Equal(signature, mac.Sum(nil)) {
		return claims, errors.New("session token: bad signature")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || json.Unmarshal(payload, &claims) != nil {
		return claims, errors.New("session token: bad payload")
	}
	switch {
	case claims.Aud != clientID:
		return claims, errors.New("session token: issued for another app")
	case now.After(time.Unix(claims.Exp, 0).Add(leeway)):
		return claims, errors.New("session token: expired")
	case now.Before(time.Unix(claims.Nbf, 0).Add(-leeway)):
		return claims, errors.New("session token: not valid yet")
	case !strings.HasSuffix(claims.Shop(), ".myshopify.com"):
		return claims, errors.New("session token: no shop")
	}
	return claims, nil
}

// ExchangeOptions configures ExchangeForOfflineToken.
type ExchangeOptions struct {
	Shop         string
	ClientID     string
	ClientSecret string
	SessionToken string
	HTTPClient   *http.Client
}

// ExchangeForOfflineToken trades a verified session token for the store's
// offline Admin API access token (Shopify's token exchange). The offline token
// lasts until the app is uninstalled.
func ExchangeForOfflineToken(ctx context.Context, opts ExchangeOptions) (string, error) {
	client := opts.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	body, _ := json.Marshal(map[string]string{
		"client_id":            opts.ClientID,
		"client_secret":        opts.ClientSecret,
		"grant_type":           "urn:ietf:params:oauth:grant-type:token-exchange",
		"subject_token":        opts.SessionToken,
		"subject_token_type":   "urn:ietf:params:oauth:token-type:id_token",
		"requested_token_type": "urn:shopify:params:oauth:token-type:offline-access-token",
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("https://%s/admin/oauth/access_token", opts.Shop), bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	res, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("token exchange: %w", err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 64<<10))
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token exchange: Shopify answered %d", res.StatusCode)
	}
	var parsed struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil || parsed.AccessToken == "" {
		return "", errors.New("token exchange: unexpected response")
	}
	return parsed.AccessToken, nil
}
