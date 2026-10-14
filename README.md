<div align="center">

# Shopify App Service Template

**A Shopify app that is just a few endpoints.**

One small Go service that holds a store's Admin API access token and exposes the endpoints you write. Shopify Flow, a backend or the storefront calls them; the service makes the authenticated Admin API call. No admin UI, no database, no Redis, no workers.

[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
![Go 1.26](https://img.shields.io/badge/go-1.26-00ADD8)
![Image ~25 MB](https://img.shields.io/badge/image-~25%20MB-lightgrey)
![RAM ~8 MB](https://img.shields.io/badge/RAM-~8%20MB-lightgrey)

</div>

## Why

Plenty of store requests come down to "when X happens, change something in Shopify":

- A Shopify Flow workflow needs a mutation Flow has no action for.
- A storefront form should tag the customer, create a draft order or save a metafield.
- An ERP, a cron job or a Zapier-style tool needs to read or write store data.

All of these need an **Admin API access token**, and getting one means creating an app. A full app template (Remix, Next.js, Rails) brings an admin UI, sessions, a database, OAuth routes, webhooks and background jobs. That is the right tool for an app merchants click around in. It is a lot to run and maintain for three endpoints.

This template is the other end: a reverse proxy of sorts that has an Admin API token. You write the endpoints; it handles the token, verifies who is calling, retries throttled calls and logs.

| | Full app template | This template |
| --- | --- | --- |
| Admin UI | Yes (Polaris pages) | One "Connected" page |
| Storage | Database for sessions | One encrypted file |
| Moving parts | Web, DB, Redis, workers | One 25 MB container |
| Memory | Hundreds of MB | About 8 MB |
| Stores | Many | One per deployment |
| Good for | Apps merchants use | Endpoints other systems call |

## How it works

```
Shopify Flow, backend, cron ──── Authorization: Bearer <API_KEY> ───┐
                                                                    ├──▶ this service ──▶ Shopify Admin API
Storefront (theme JS, forms) ──▶ App Proxy (signed by Shopify) ─────┘      (offline access token)
```

- **`/api/...`** is for server-to-server callers. They send `Authorization: Bearer <API_KEY>`. Shopify Flow's "Send HTTP request" action can set that header.
- **`/proxy/...`** is for the storefront. Browser code is public, so it can't hold a key. Instead the storefront calls `https://<store>/apps/<subpath>/...`, and Shopify's App Proxy forwards it here with an HMAC signature made with the app's client secret. The service checks the signature, that it's fresh (5 minutes) and that it's for your store. It also knows the logged-in customer, if any. Rate limited per customer, else per IP.
- **`GET /`** is the App URL. Shopify opens it inside the admin when someone opens the app, and that is how the service gets its access token (below).
- **`GET /health`** for uptime checks.

The same endpoint can sit behind both: write it once, mount it under `/api` and `/proxy` (see `cmd/server/main.go`).

## The access token

There is no OAuth callback and no database. The token comes from Shopify's [token exchange](https://shopify.dev/docs/apps/build/authentication-authorization/access-tokens/token-exchange):

1. A staff member opens the app in the store's admin. Shopify loads the App URL (`GET /`) with a session token (`id_token`, a JWT signed with the app's client secret).
2. The service verifies it: signature, audience (this app's client ID), expiry, and that it's for the configured store.
3. It trades the session token for the store's **offline access token**.
4. The token is kept **in memory** for every API call, and **encrypted on disk** so it survives restarts and deploys.

**Storage.** One file, `$DATA_DIR/shopify-token.enc` (a Docker volume in production):

- AES-256-GCM, with a key derived from the client secret (SHA-256). Without the secret the file is useless, and the secret only lives in the environment.
- Written atomically (temp file, then rename) with mode `0600`.
- Read once at start; after that every request uses the in-memory copy, so there's no disk I/O per call.

**Lifetime.**

- Offline tokens for custom apps don't expire; the token lasts until the app is uninstalled.
- Every time the app is opened in the admin, the service exchanges again. A token keeps the scopes it was issued with, so after adding scopes you approve them and open the app once, and the new scopes apply.
- If Shopify ever answers 401 (uninstalled, token revoked), the service drops the token and answers `503 service not connected` until the app is opened again.
- Rotating the client secret makes the stored file unreadable. Open the app once afterwards to store a new token.

Why not the client credentials grant (client ID + secret only)? Shopify only allows it when the app and the store belong to the same organisation. A custom-distribution install on a client's store doesn't, and the grant answers `shop_not_permitted`.

## Quick start

You need [Go 1.26](https://go.dev/dl/), [cloudflared](https://developers.cloudflare.com/cloudflare-one/connections/connect-networks/downloads/) (for the first install only) and a [Shopify Partner](https://www.shopify.com/partners) account.

### 1. Run the service

```sh
git clone https://github.com/ChristiantangCC/Shopify-App-Service-Template my-service
cd my-service
./scripts/dev.sh
```

It opens a tunnel, prints the **App URL**, and waits for the store, client ID and secret. Leave it running.

It asks because the free tunnel gets a new URL on every start: the app needs that URL before it exists, and the credentials only exist after the app is created, so `dev.sh` keeps the tunnel open while you create the app. Already have an app? Put `SHOPIFY_SHOP`, `SHOPIFY_CLIENT_ID` and `SHOPIFY_CLIENT_SECRET` in `.env` (see `.env.example`) and `dev.sh` skips the questions.

### 2. Create the app

In the [Dev Dashboard](https://dev.shopify.com/dashboard): **Apps → Create app → Create app manually**.

- **App name:** anything, e.g. "Product Service App".
- **App URL:** the URL `dev.sh` printed.
- **Embed app in Shopify admin:** keep it ticked. The admin then passes the session token the service exchanges.
- **Use legacy install flow:** off.
- **Scopes:** what your endpoints need. The example endpoints need `read_products,write_products`.
- **Create app**, then **Release**.

### 3. Paste the credentials

**Settings → Credentials**: copy the **Client ID** and **Client secret** into the waiting terminal. `dev.sh` saves them to `.env` (git-ignored), generates an `API_KEY`, and starts the service.

### 4. Install it on the store

In the [Partner Dashboard](https://partners.shopify.com) (not the Dev Dashboard):

1. **App distribution → All apps →** your app **→ Choose distribution → Custom distribution**.
2. Enter the store's domain (`your-store.myshopify.com`) and **Generate link**.
3. Open the link, check the scopes and click **Install**.

### 5. Approve the scopes

Installing opens the app in the admin: the page says **"Connected"** and the terminal logs `[Install] Connected`. The service now has the token. The tunnel is only needed until this point; the token works from wherever the service runs.

### 6. Call it

```sh
source .env
curl -s -X POST localhost:8080/api/example/products \
  -H "Authorization: Bearer $API_KEY" \
  -d '{"title": "Demo Hoodie"}'
```

```json
{"product":{"id":"gid://shopify/Product/10344372175176","title":"Demo Hoodie","handle":"demo-hoodie","status":"DRAFT"}}
```

## Writing your own endpoints

The example in `internal/handlers/example_products.go` is the pattern to copy:

```go
// CreateProduct handles POST /example/products with {"title": "..."}.
func (h CreateProduct) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// 1. validate the input
	// 2. h.Admin.Do(ctx, mutation, variables, &out)
	// 3. writeJSON(w, http.StatusCreated, ...)
}
```

1. Add a handler in `internal/handlers/`. `Admin.Do` sends the GraphQL request with the stored token, waits and retries when Shopify throttles, and returns `shopify.ErrNotConnected` before the first install.
2. Mount it in the `endpoints` function in `cmd/server/main.go`. It's then served under `/api` and `/proxy`.
3. For storefront calls, `appproxy.CallerFrom(r.Context())` gives the shop and the logged-in customer's ID.
4. Add the scopes to the app, release, and open the app once in the admin.
5. Delete the example file and its two routes.

### Calling it from Shopify Flow

Add a **Send HTTP request** action:

- **Method:** POST
- **URL:** `https://<your-domain>/api/<your-endpoint>`
- **Headers:** `Authorization: Bearer <API_KEY>`, `Content-Type: application/json`
- **Body:** the JSON your endpoint expects, using Flow variables.

### Calling it from the storefront

In the app's version settings, add an **App Proxy**: prefix `apps`, a subpath (e.g. `service`), URL `https://<your-domain>/proxy`. Then from theme code:

```js
await fetch('/apps/service/example/products?first=5');
```

Never put the `API_KEY` in theme code or anything else a browser loads.

## Deploy

Any host that runs a container and gives it HTTPS works:

```sh
cp .env.example .env    # fill it in, or reuse the one dev.sh wrote
docker compose up -d --build
```

- Put HTTPS in front (Caddy, Traefik, a load balancer) and set the app's **App URL** to `https://<your-domain>/`.
- Keep the `data` volume: it holds the encrypted token.
- Open the app once in the admin after the first deploy if the service doesn't have a token yet.

Image: about 25 MB (distroless, static binary, runs as nonroot). Memory: about 8 MB in use; the compose file caps it at 128 MB.

## Configuration

| Variable | Default | |
| --- | --- | --- |
| `SHOPIFY_SHOP` | required | The store's `*.myshopify.com` domain |
| `SHOPIFY_CLIENT_ID` | required | From the app's settings |
| `SHOPIFY_CLIENT_SECRET` | required | Verifies session tokens and App Proxy signatures, encrypts the token file |
| `API_KEY` | empty (off) | Enables `/api`; at least 32 characters |
| `SHOPIFY_API_VERSION` | `2026-07` | Admin API version |
| `DATA_DIR` | `/data` | Where the encrypted token is kept |
| `PROXY_MAX_AGE` | `5m` | Oldest accepted App Proxy timestamp |
| `RATE_LIMIT_PER_MINUTE` | `30` | App Proxy requests per customer (or IP) |
| `PORT` | `8080` | |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `LOG_FORMAT` | `json` | `text` for local development |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | empty | Also send logs to an OpenTelemetry collector |
| `SERVICE_NAMESPACE` | `shopify-app-service` | Service name in OpenTelemetry logs |

## Layout

```
cmd/server/           main: config, router, graceful shutdown
internal/handlers/    your endpoints (+ the example), install page, health
internal/shopify/     Admin API client, token exchange, encrypted token store
internal/appproxy/    App Proxy signature check, rate limit key
internal/apikey/      bearer API key check for /api
internal/config/      environment variables
internal/logging/     slog JSON/text, optional OpenTelemetry
scripts/dev.sh        local development: tunnel + prompts + run
```

Libraries: [chi](https://github.com/go-chi/chi) (router), [httprate](https://github.com/go-chi/httprate) (rate limit), [env](https://github.com/caarlos0/env) (config), the OpenTelemetry log bridge. Everything else is the standard library.

## Develop

```sh
go test ./...
go vet ./...
docker build -t shopify-app-service .
```

## Limits

- **One store per deployment.** A service for another store is another deployment with its own app. For many stores, use a full app template.
- **No webhooks.** Add a handler and verify `X-Shopify-Hmac-Sha256` if you need them, or use Flow to call `/api`.
- **The first install needs the service reachable over HTTPS**, because Shopify loads the App URL in the admin. Locally, `dev.sh`'s tunnel covers that.

## License

[MIT](LICENSE)
