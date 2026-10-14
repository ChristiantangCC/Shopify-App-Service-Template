package shopify

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"
)

var issued = time.Unix(1_800_000_000, 0)

func sessionToken(t *testing.T, secret string, claims map[string]any) string {
	t.Helper()
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	payload, _ := json.Marshal(claims)
	body := header + "." + base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(body))
	return body + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func validClaims() map[string]any {
	return map[string]any{"dest": "https://example-shop.myshopify.com", "aud": "client-id", "exp": issued.Add(time.Minute).Unix(), "nbf": issued.Add(-time.Second).Unix()}
}

func TestVerifySessionTokenAcceptsShopifysToken(t *testing.T) {
	claims, err := VerifySessionToken(sessionToken(t, "secret", validClaims()), "client-id", "secret", issued)
	if err != nil || claims.Shop() != "example-shop.myshopify.com" {
		t.Fatalf("claims %+v err %v", claims, err)
	}
}

func TestVerifySessionTokenRejects(t *testing.T) {
	otherApp := validClaims()
	otherApp["aud"] = "other"
	expired := validClaims()
	expired["exp"] = issued.Add(-time.Minute).Unix()
	cases := map[string]string{
		"wrong secret": sessionToken(t, "not-the-secret", validClaims()),
		"other app":    sessionToken(t, "secret", otherApp),
		"expired":      sessionToken(t, "secret", expired),
		"garbage":      "a.b",
	}
	for name, token := range cases {
		if _, err := VerifySessionToken(token, "client-id", "secret", issued); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}
