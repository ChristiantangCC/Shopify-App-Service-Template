#!/usr/bin/env bash
# Local development: opens a cloudflared tunnel, asks for whatever .env is
# missing (store, client ID + secret), then runs the service with readable logs.
#
# The tunnel is only needed until the app has been opened once in the store's
# admin: that hands the service its access token. After that the token works
# from anywhere, tunnel or not.
set -euo pipefail
cd "$(dirname "$0")/.."

for tool in go cloudflared; do
  command -v "$tool" >/dev/null || { echo "Needs $tool (brew install $tool)"; exit 1; }
done

touch .env
set -a; . ./.env; set +a
PORT="${PORT:-8080}"

log="$(mktemp)"
cloudflared tunnel --url "http://localhost:$PORT" --no-autoupdate >"$log" 2>&1 &
tunnel=$!
trap 'kill $tunnel 2>/dev/null; rm -f "$log"' EXIT

printf 'Opening a tunnel'
url=""
for _ in $(seq 1 30); do
  url="$(grep -o 'https://[a-z0-9-]*\.trycloudflare\.com' "$log" | head -1 || true)"
  [ -n "$url" ] && break
  printf '.'; sleep 1
done
[ -n "$url" ] || { echo; cat "$log"; exit 1; }
printf '\n\n  App URL:  \033[1;36m%s\033[0m\n\n' "$url"

save() { echo "$1=$2" >>.env; export "$1=$2"; }
[ -n "${SHOPIFY_SHOP:-}" ] || { read -rp "Store (example.myshopify.com): " v; save SHOPIFY_SHOP "$v"; }
[ -n "${SHOPIFY_CLIENT_ID:-}" ] || { read -rp "Client ID: " v; save SHOPIFY_CLIENT_ID "$v"; }
[ -n "${SHOPIFY_CLIENT_SECRET:-}" ] || { read -rsp "Client secret (hidden): " v; echo; save SHOPIFY_CLIENT_SECRET "$v"; }
[ -n "${API_KEY:-}" ] || save API_KEY "$(openssl rand -hex 32)"

echo
DATA_DIR="${DATA_DIR:-./data}" LOG_FORMAT="${LOG_FORMAT:-text}" PORT="$PORT" go run ./cmd/server
