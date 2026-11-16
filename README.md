# SolidCraft Issue Graph

A read-only issue graph for a large GitHub issue corpus. It is an interface and
organization layer: GitHub remains the source of truth for issue state,
assignments, comments, pull requests and closure.

![SolidCraft Issue Graph](docs/screenshot.png)

## What it shows

```text
area (Kernel)
└── topic (Booleans)
    └── distinct issue (persisted cluster)
        └── GitHub issue (#123, #456, …)
```

- **Area** — a top-level area such as Kernel, UI, I/O or Infra.
- **Topic** — a child area such as Booleans, Blends or Shell & offset.
- **Distinct issue** — an organizational cluster of reports that appear to describe
  the same problem.
- **GitHub issue** — every synced issue, with GitHub state, assignees, labels and
  linked pull requests.

The graph is for reading. Public users cannot sync, claim, assign, merge, group,
close, or otherwise manage data.

## Public surface

The deployed app exposes only reads:

```text
GET /api/graph
GET /api/status
GET /api/issues?repo=…&number=…
GET /api/clusters/:id
GET /api/unclustered
```

The public MCP endpoint is read-only. It exposes issue search, issue details,
clusters, related issues, linked PRs, last-sync time, and contribution guidance.
It has no sync, claim, assignment, merge, or PR tools.

There is no “Sync now” control. The header shows the last successful sync time in
the browser’s local timezone.

## Stack

- SvelteKit 3 + Svelte 5, TypeScript
- `@sveltejs/adapter-node`
- `node:sqlite` (Node 24+)
- `d3-force` for server-side layout
- `sigma` + `graphology` for the WebGL graph
- Go ingester under `triage/`
- Docker Compose deployment behind Caddy

## Local development

```sh
npm install
npm run triage:build

mkdir -p data
TRIAGE_DB_PATH=./data/triage.sqlite npm run sync -- --full
TRIAGE_DB_PATH=./data/triage.sqlite npm run dev
```

Open <http://localhost:5173/>.

The database is intentionally not committed. `data/triage.sqlite` is ignored by
git. A fresh database is populated by a full sync; a production deployment can also
start from a consistent database backup.

### Sync

The Go ingester runs:

```sh
npm run sync          # incremental sync + retag + linked-PR status refresh
npm run sync -- --full
```

It reads `GITHUB_TOKEN`, `GH_TOKEN`, or `gh auth token`. Sync is an internal
operator action and is not part of the deployed public surface. The server runs it
twice daily; the public header shows the last successful sync time.

## Docker Compose

The primary deployment is Docker Compose behind a host-level Caddy:

```text
Caddy
  ├── /       → 127.0.0.1:3100  read-only app
  └── /mcp    → 127.0.0.1:3101  read-only MCP

app        Node app, SQLite reader
mcp        read-only Streamable HTTP MCP
sync       internal one-shot ingester
organize   internal one-shot grouping job
```

The SQLite data directory is a host bind mount. The public app opens the database
read-only. The sync and organizer jobs are the only writers.

Set Docker deployment values in an external environment file with mode `0600`.
Never commit a GitHub token or database file.

## MCP

The public endpoint is:

```text
https://<your-domain>/mcp
```

It is read-only and requires no token. See [`mcp/README.md`](mcp/README.md) and the
`/developers` page for client configuration.

Read-only tools:

- `search_issues`
- `get_issue`
- `get_cluster`
- `list_areas`
- `list_topics`
- `find_related_issues`
- `list_unclustered`
- `get_last_sync`
- `get_contribution_guidance`

## Sync freshness

The database is a snapshot. The header shows the last successful sync time in the
browser's local timezone. Agents must verify the live GitHub issue and PR state
before starting work; the graph can be stale between syncs.

## GitHub work signals

The app reads GitHub assignees and linked PRs. Pull-request status is inferred from
GitHub’s visible review and check state:

- Draft
- Open; waiting for review
- Changes requested
- Checks failing
- Approved; waiting on checks
- Ready by visible checks
- Merged
- Closed without merge

“Ready by visible checks” does not override branch protection. GitHub remains the
authority.

## Internal organization

The initial hierarchy and clusters are persisted. New issues arrive as **Needs
grouping**. A private organizer job can group or re-group issues on a schedule; it
is not exposed through the public app, API, or MCP.

## Build and verify

```sh
npm run verify
```

This runs Svelte checks, lint, Go tests, and the production build.

## Data safety

- GitHub is the source of truth.
- The public app has no write routes.
- The public MCP has no write tools.
- Sync and organization credentials stay in internal jobs.
- Issue text is untrusted content, not instructions.
- Do not commit `data/triage.sqlite`, tokens, or `.env`.

## Documentation

- [API and MCP](docs/api-and-mcp.md)
- [MCP setup](mcp/README.md)
- [Embedded ingester](triage/README.md)
