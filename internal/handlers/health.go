package handlers

import "net/http"

// Health answers GET /health for load balancers, reverse proxies and uptime checks. It does not
// call Shopify: a Shopify hiccup must not mark the service itself as down.
func Health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
