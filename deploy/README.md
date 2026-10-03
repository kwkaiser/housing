# Deploying housing

The web app (`housing server`) is a single binary that serves HTML on a loopback port. It has no login of its own, so run it behind a reverse proxy that handles TLS and authentication.

## Build

```sh
task build
GOOS=linux GOARCH=amd64 go build -o bin/housing-linux-amd64 ./cmd/housing
```

Templates and static assets are embedded, so the binary is all you need to copy. Install it as `/usr/local/bin/housing`.

## Service user and data

```sh
sudo useradd --system --home-dir /var/lib/housing --shell /usr/sbin/nologin housing
```

systemd creates `/var/lib/housing` (`StateDirectory=housing`), owned by the `housing` user. The unit runs with that as its working directory and passes `--data-dir /var/lib/housing/data`, so the SQLite database and images live under `/var/lib/housing/data`. The process can write nowhere else (`ProtectSystem=strict` with `ReadWritePaths=/var/lib/housing`).

Only the app writes the SQLite database. To move existing data over, stop the service, copy `data/` into `/var/lib/housing/data`, and `chown -R housing:housing /var/lib/housing`.

## Commands

Everything day to day happens in the web UI: profiles, collections, runs and jobs. The binary also has a small admin CLI. Run it as the `housing` user with `--data-dir /var/lib/housing/data`:

- `housing server`: the web app, also running queued jobs and the daily scheduled runs.
- `housing migrate`: apply pending database migrations and print the schema version. The server migrates on start too; run this before a deploy to check the new binary against the database.
- `housing import [--profiles-dir profiles] [--collections-dir collections]`: copy profiles and collections from the old files into the database. Safe to re-run.
- `housing jobs`, `housing jobs show <id>`, `housing jobs run <collection>`: list jobs, show one with its progress log, or queue a collection run for the server to pick up.
- `housing run <collection>`: run a collection in the foreground, printing progress, for debugging. It takes the data directory's lock, so it fails fast while the server is running; stop the service first, or use `housing jobs run` instead.
- `housing apikeys`, `housing apikeys create <name>`, `housing apikeys delete <id>`: list API keys, create one and print it, or delete one. Safe to run while the server is up.
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
```

## API

The server also serves a JSON API under `/api/v1` for scripts and agents. Each client authenticates with its own API key, sent as a bearer token. Create and delete keys on the API keys page (`/keys`) or with `housing apikeys`. A key is shown once when it is created; the database keeps only its hash.

```sh
curl -s -H "Authorization: Bearer hk_..." http://127.0.0.1:8080/api/v1/collections
```

It lists collections, a collection's ranked listings for a day, listing detail with per-criterion grades, profiles, jobs and collage images, and can queue collection runs. The OpenAPI 3.1 spec is at `/api/openapi.json` (or `.yaml`), with a 3.0 copy at `/api/openapi-3.0.json` for client generators that do not support 3.1 yet, and browsable docs at `/api/docs`. The spec and docs need no key.

API keys are separate from the reverse proxy's login, so let `/api/` through the proxy without it if API clients cannot log in there. Keep everything else, including `/keys`, behind the proxy: anyone who can reach the web UI can create keys.

## systemd

```sh
sudo cp deploy/housing.service /etc/systemd/system/housing.service
sudo systemctl daemon-reload
sudo systemctl enable --now housing
journalctl -u housing -f
curl -s http://127.0.0.1:8080/healthz
```

On stop, the server stops accepting connections and lets in-flight requests finish for up to 10 seconds. `TimeoutStopSec=30` leaves room for that. Restarts happen only on failure.
