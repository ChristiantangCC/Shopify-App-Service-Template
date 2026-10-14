# Shopify App Service Template

> [!NOTE]
> This is **not** a full Shopify app starter in Go. It's the bare minimum to get a small service running.
>
> It's meant for agencies, developers and e-commerce managers who want one or more quick services up and running: "when X happens, do Y" in the store, without a database or Redis to run.
>
> Building an app that needs webhooks, a database, Redis caching, background jobs and queues, billing, multiple stores or a full admin UI? Start from a full app template instead.

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

## Setup

Needs Go 1.26 and [cloudflared](https://developers.cloudflare.com/cloudflare-one/connections/connect-networks/downloads/). The service must be running and reachable over HTTPS **before** the app is installed: installing opens the app in the admin, and that opening is what hands the service its token.

### 1. Run the service

```sh
./scripts/dev.sh
```

It opens a tunnel, prints the **App URL** and waits for the store, client ID and secret. Leave it running: the tunnel gets a new URL on every start, and the app you create next must point at this one.

Already have an app? Put `SHOPIFY_SHOP`, `SHOPIFY_CLIENT_ID` and `SHOPIFY_CLIENT_SECRET` in `.env` (see `.env.example`) and it skips the questions.

### 2. Create the app (Dev Dashboard)

In the [Dev Dashboard](https://dev.shopify.com/dashboard): **Apps → Create app → Create app manually**.

| Field | Value |
| --- | --- |
| App name | Anything, e.g. "Product Service App" |
| App URL | The URL `dev.sh` printed (later: `https://<your-domain>/`) |
| Embed app in Shopify admin | **On.** The admin then sends the session token the service exchanges for the access token |
| Allowed redirection URL(s) | Empty |
| Use legacy install flow | **Off.** Shopify handles the install; there is no OAuth callback |
| Scopes | What your endpoints need, comma separated. The example needs `read_products,write_products` |

Click **Create app**, then **Release** the version. Changing the URL or scopes later means a new version, released the same way.

### 3. Give the service the credentials

In the app: **Settings → Credentials**. Copy the **Client ID** and **Client secret** into the waiting terminal. `dev.sh` saves them to `.env` (git ignored), generates an `API_KEY` and starts the service.

### 4. Install it on the store (Partner Dashboard)

Custom apps are installed with a link from the [Partner Dashboard](https://partners.shopify.com), not the Dev Dashboard:

1. **App distribution → All apps →** your app.
2. **Choose distribution → Custom distribution** (this can't be undone for that app).
3. Enter the store's domain, e.g. `your-store.myshopify.com`. Leave "Allow multi-store install" off unless it's for a Plus organisation.
4. **Generate link** and open it while logged in to the store's admin (or send it to the store owner).
5. Check the scopes and click **Install**.

### 5. Check it's connected

After **Install** the app opens in the admin and says **"Connected"**, and the service logs `[Install] Connected`. From now on the token works from wherever the service runs; the tunnel can go.

Not connected? The App URL in the released version must match where the service runs, and the service must be up when the app is opened. Open the app again from the admin's Apps menu to retry.

**Adding scopes later:** add them to the app, release, then open the app in the admin and approve. That opening fetches a token with the new scopes.

### 6. Call it

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
