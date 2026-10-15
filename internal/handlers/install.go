package handlers

import (
	"fmt"
	"html"
	"log/slog"
	"net/http"
	"time"

	"github.com/ChristiantangCC/Shopify-Go-Service-App-Template/internal/shopify"
)

// Install handles GET /, the App URL Shopify loads inside the admin when the
// app is opened. Every opening trades the admin's session token (`id_token`)
// for the store's offline access token and stores it: the first opening
// connects the store, and later ones pick up newly granted scopes (a token
// keeps the scopes it was issued with). That is the only reason the app has an
// admin page.
type Install struct {
	Shop         string
	ClientID     string
	ClientSecret string
	Tokens       *shopify.TokenStore
	Logger       *slog.Logger
}

func (h Install) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// The admin embeds this page; allow only the store and the admin to frame it.
	w.Header().Set("Content-Security-Policy", fmt.Sprintf("frame-ancestors https://%s https://admin.shopify.com", h.Shop))

	idToken := r.URL.Query().Get("id_token")
	if idToken == "" {
		h.page(w, http.StatusBadRequest, "Open this app from the Shopify admin.")
		return
	}
	claims, err := shopify.VerifySessionToken(idToken, h.ClientID, h.ClientSecret, time.Now())
	if err != nil {
		h.Logger.Warn("[Install] Rejected session token", "error", err)
		h.page(w, http.StatusUnauthorized, "This link is not valid. Open the app from the Shopify admin again.")
		return
	}
	if claims.Shop() != h.Shop {
		h.Logger.Warn("[Install] Opened from another store", "shop", claims.Shop())
		h.page(w, http.StatusForbidden, "This app is set up for another store.")
		return
	}

	token, err := shopify.ExchangeForOfflineToken(r.Context(), shopify.ExchangeOptions{Shop: h.Shop, ClientID: h.ClientID, ClientSecret: h.ClientSecret, SessionToken: idToken})
	if err != nil {
		h.Logger.Error("[Install] Token exchange failed", "error", err)
		h.page(w, http.StatusBadGateway, "Could not connect to Shopify. Try opening the app again.")
		return
	}
	if err := h.Tokens.Save(h.Shop, token); err != nil {
		h.Logger.Error("[Install] Could not store the token", "error", err)
		h.page(w, http.StatusInternalServerError, "Could not store the connection. Check the service's data volume.")
		return
	}
	h.Logger.Info("[Install] Connected", "shop", h.Shop)
	h.page(w, http.StatusOK, "Connected. This app works in the background; nothing to set up here.")
}

func (h Install) page(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	fmt.Fprintf(w, `<!doctype html><html><head><meta charset="utf-8"><title>Connected</title><style>body{font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;margin:40px;color:#303030}</style></head><body><p>%s</p></body></html>`, html.EscapeString(message))
}
