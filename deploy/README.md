# Deployment

Host: Debian Trixie, Docker Compose, host-level Caddy.

## Layout

```text
/home/nclack/solidcraft-issue-tracker/
  compose.yaml
  data/triage.sqlite
  backups/
  .env.sync                 # mode 0600, not committed
```

The app and MCP bind only to loopback. Caddy is the public entry point.

## Caddy

Append [`Caddyfile.snippet`](Caddyfile.snippet) to the host Caddyfile, then reload:

```sh
sudo systemctl reload caddy
```

The snippet proxies:

- `/` to `127.0.0.1:3100`
- `/mcp` and `/mcp/*` to `127.0.0.1:3101`

The existing Caddy global `acme_dns cloudflare` configuration issues the
certificate.

## Start

```sh
cd /home/nclack/solidcraft-issue-tracker
docker compose build
docker compose up -d app mcp
```

The app database directory must be writable by UID 1000 so SQLite can manage WAL
files. The public app opens the DB read-only; the internal sync job is the writer.

## Initial data

Copy a consistent database backup before starting the app:

```sh
sqlite3 data/triage.sqlite ".backup /tmp/triage.sqlite"
scp /tmp/triage.sqlite orion:/home/nclack/solidcraft-issue-tracker/data/triage.sqlite
```

## Sync

Create `/home/nclack/solidcraft-issue-tracker/.env.sync` with a read-only GitHub token,
mode `0600`:

```sh
GITHUB_TOKEN=...
```

Run manually:

```sh
docker compose --profile sync run --rm sync
```

The sync job writes issues, comments, refs, tags, PR statuses and `sync_runs`.
The app shows the last successful sync time.

## MCP

Public read-only endpoint:

```text
https://solidcraft.neilclack.com/mcp
```

No token is required. The MCP service calls the app over the Compose network.

## Deployment runner

The repository includes `.github/workflows/deploy.yml`. The `orion-solidcraft`
self-hosted runner is installed under `/home/nclack/actions-runner-solidcraft`
with labels `self-hosted`, `linux`, `x64`. It restarts on boot through cron and
runs the deploy workflow on pushes to `main`.

Manual equivalent:

```sh
cd /home/nclack/solidcraft-issue-tracker
git pull --ff-only
docker compose build
docker compose up -d app mcp
```

Before every deploy, back up `data/triage.sqlite`; the app and MCP containers do
not contain the database.
