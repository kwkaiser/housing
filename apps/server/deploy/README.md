# Deploying housing

The server (`housing server`) is a single binary that serves the JSON API on a loopback port. The UI is the separate web app described below. The server handles sign-in itself through an OIDC provider (see [Sign-in](#sign-in)), so the reverse proxy in front only needs to terminate TLS.

## Build

```sh
task server:build
(cd apps/server && GOOS=linux GOARCH=amd64 go build -o bin/housing-linux-amd64 ./cmd/housing)
```

The binary is all you need to copy. Install it as `/usr/local/bin/housing`.

## Service user and data

```sh
sudo useradd --system --home-dir /var/lib/housing --shell /usr/sbin/nologin housing
```

systemd creates `/var/lib/housing` (`StateDirectory=housing`), owned by the `housing` user. The unit runs with that as its working directory and passes `--data-dir /var/lib/housing/data`, so the SQLite database and images live under `/var/lib/housing/data`. The process can write nowhere else (`ProtectSystem=strict` with `ReadWritePaths=/var/lib/housing`).

Only the app writes the SQLite database. To move existing data over, stop the service, copy `data/` into `/var/lib/housing/data`, and `chown -R housing:housing /var/lib/housing`.

## Commands

Everything day to day happens in the web UI: profiles, collections, runs and jobs. The binary also has a small admin CLI. Run it as the `housing` user with `--data-dir /var/lib/housing/data`:

- `housing server`: the API server, also running queued jobs and the daily scheduled runs.
- `housing migrate`: apply pending database migrations and print the schema version. The server migrates on start too; run this before a deploy to check the new binary against the database.
- `housing import [--profiles-dir profiles] [--collections-dir collections]`: copy profiles and collections from the old files into the database. Safe to re-run.
- `housing jobs`, `housing jobs show <id>`, `housing jobs run <collection>`: list jobs, show one with its progress log, or queue a collection run for the server to pick up.
- `housing run <collection>`: run a collection in the foreground, printing progress, for debugging. It takes the data directory's lock, so it fails fast while the server is running; stop the service first, or use `housing jobs run` instead.
- `housing version`.

## API keys

Keys are only needed when jobs fetch or assess listings, so the server starts without them. Put them in an env file readable only by root:

```sh
sudo install -d -m 0755 /etc/housing
sudo install -m 0600 /dev/null /etc/housing/env
```

```
APIFY_TOKEN=...
OPENROUTER_API_KEY=...
CARTO_API_KEY=...
TRANSITLAND_API_KEY=...
```

`CARTO_API_KEY` is optional and enables the Positron base map on the map page. The server fetches Positron tiles from CARTO itself and caches them under `data/tiles` for 30 days, so the key never reaches the browser. Without it, the map falls back to OpenStreetMap tiles. A free non-commercial key is available from https://carto.com/basemaps/apikey.

`TRANSITLAND_API_KEY` is optional and enables the bus and rail route overlays on the map page, with route names, official colors and frequencies on hover. The server fetches Transitland route tiles itself and caches them under `data/tiles/transitland` for 7 days; the key never reaches the browser. Without it, rail falls back to OpenRailwayMap tiles and there is no bus overlay. A free key is available from https://www.transit.land/plans-pricing/. Named bike paths come from OpenFreeMap, which needs no key, cached under `data/tiles/openfreemap`.

## API

The server also serves a JSON API under `/api/v1` for scripts and agents. Each client authenticates with an access token issued by the OIDC provider to the server's client, sent as a bearer token, such as one from a service account using the client credentials grant.

```sh
curl -s -H "Authorization: Bearer $TOKEN" http://127.0.0.1:8080/api/v1/collections
```

It lists collections, a collection's ranked listings for a day, listing detail with per-criterion grades, profiles, jobs and collage images, and can queue collection runs. The OpenAPI 3.1 spec is at `/api/openapi.json` (or `.yaml`), with a 3.0 copy at `/api/openapi-3.0.json` for client generators that do not support 3.1 yet, and browsable docs at `/api/docs`. The spec and docs need no token.

The same tokens work for the MCP server at `/api/mcp` (streamable HTTP), which gives agents tools to list collections and ranked listings, read a listing with its grades and photo collages, read profiles, and queue and check runs. To add it to Claude Code:

```sh
claude mcp add --transport http housing https://housing.example.com/api/mcp --header "Authorization: Bearer $TOKEN"
```

## Sign-in

Browsers sign in with the OIDC authorization code flow. The server is a confidential client: it keeps the session in the database and gives the browser only an HttpOnly session cookie, valid for 30 days. Set:

- `HOUSING_OIDC_ISSUER`: the provider's issuer URL, exactly as its discovery document gives it. For authentik that is `https://authentik.example.com/application/o/<slug>/`.
- `HOUSING_OIDC_CLIENT_ID`, `HOUSING_OIDC_CLIENT_SECRET`: the client's credentials.
- `HOUSING_PUBLIC_URL`: the URL people open, such as `https://housing.example.com`. The provider must allow `<HOUSING_PUBLIC_URL>/auth/callback` as a redirect URI.

The server reads the provider's discovery document on start and will not start if it cannot. Who may sign in is up to the provider; in authentik, bind a policy or group to the application. Signing out ends the server's session and then the provider's, through its end-session endpoint.

`--no-auth` turns all of this off: every request is treated as signed in and the variables above are not needed. It is meant for local development only.

## systemd

```sh
sudo cp apps/server/deploy/housing.service /etc/systemd/system/housing.service
sudo systemctl daemon-reload
sudo systemctl enable --now housing
journalctl -u housing -f
curl -s http://127.0.0.1:8080/healthz
```

On stop, the server stops accepting connections and lets in-flight requests finish for up to 10 seconds. `TimeoutStopSec=30` leaves room for that. Restarts happen only on failure.

## Web app

`apps/web` builds into its own image, `git.kwkaiser.io/kwkaiser/housing-web`, which runs Caddy on port 8080. Caddy serves the app and proxies `/api/` and `/auth/` to the server, keeping the `Host` header so the server's same-origin checks pass. `HOUSING_API_URL` sets where the server listens and defaults to `http://housing:8080`.

For local development, `task dev` runs the server with `--no-auth` alongside the Vite dev server, which proxies `/api` and `/auth` to `HOUSING_API_URL` (default `http://127.0.0.1:8080`). To try real sign-in locally, set the OIDC variables in `.env` with `HOUSING_PUBLIC_URL=http://localhost:5173`, allow `http://localhost:5173/auth/callback` on the provider, and run `task server:server` and `task web:dev`.
