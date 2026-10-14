# Shopify App Service Template

A small Go service that holds one store's Admin API access token and exposes a few endpoints of your own. No admin UI, no database, no Redis. About 25 MB image, about 8 MB RAM.

Use it when you just need something like:

- A Shopify Flow workflow that runs a mutation Flow has no action for.
- A storefront form that tags the customer, creates a draft order or saves a metafield.
- An ERP, a cron job or a Zapier-style tool that reads or writes store data.

## How it works

```
Shopify Flow, backend, cron ── Bearer API_KEY ──┐
                                                ├─▶ service ─▶ Admin API
Storefront ─▶ App Proxy (signed by Shopify) ────┘
```

- **`/api/...`**: server-to-server callers send `Authorization: Bearer <API_KEY>`.
- **`/proxy/...`**: storefront calls go through Shopify's App Proxy, which signs every request. No key in theme code.
- **`GET /`**: the App URL. Opening the app in the admin hands the service its access token.

**The access token.** When the app is opened in the admin, the service exchanges the admin's session token for the store's offline access token. It's kept in memory and in one encrypted file (`$DATA_DIR/shopify-token.enc`, AES-256-GCM, keyed by the client secret), so it survives restarts and deploys. Every opening exchanges again, so newly approved scopes apply.

## Quick start

Needs Go 1.26 and [cloudflared](https://developers.cloudflare.com/cloudflare-one/connections/connect-networks/downloads/).

1. **Run it:** `./scripts/dev.sh`. It opens a tunnel, prints the App URL and waits for the credentials. (Already have them? Put them in `.env`, see `.env.example`, and it skips the questions.)
2. **Create the app** in the [Dev Dashboard](https://dev.shopify.com/dashboard): Create app manually, App URL = the printed URL, embedded on, legacy install flow off, scopes (`read_products,write_products` for the example). Create, then Release.
3. **Paste** the client ID and secret from the app's Settings into the terminal.
4. **Install** from the [Partner Dashboard](https://partners.shopify.com): App distribution → your app → Custom distribution → store domain → Generate link → open it → Install.
5. The app opens in the admin and says **Connected**. The tunnel is only needed until here.
6. **Call it:**

```sh
source .env
curl -X POST localhost:8080/api/example/products \
  -H "Authorization: Bearer $API_KEY" -d '{"title": "Demo Hoodie"}'
```

## Your own endpoints

1. Copy `internal/handlers/example_products.go`: validate input, call `h.Admin.Do(ctx, query, vars, &out)`, write JSON.
2. Mount it in `endpoints` in `cmd/server/main.go` (served under `/api` and `/proxy`).
3. Add the scopes to the app, release, open the app once.
4. Delete the example.

From Flow: a **Send HTTP request** action to `https://<domain>/api/<endpoint>` with the `Authorization` header. From the storefront: add an App Proxy (`apps` / `<subpath>` → `https://<domain>/proxy`) and `fetch('/apps/<subpath>/<endpoint>')`.

## Deploy

```sh
docker compose up -d --build
```

Put HTTPS in front, set the App URL to `https://<domain>/` and keep the `data` volume. All settings are in `.env.example`.

## License

[MIT](LICENSE)
