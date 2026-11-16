# SolidCraft Issue Graph

A local, server-backed SvelteKit app that visualizes the SolidCraft issue corpus as an
Obsidian-style graph. It reads the triage SQLite database directly, groups GitHub
issues into distinct problems, and lays the hierarchy out as:

```text
area (Kernel)
└── topic (Booleans)
    └── distinct issue (a bug-sized cluster)
        └── GitHub issue (#123, #456, …)
```

The site is read-only today. The data and API shape are designed so an agent work
queue and MCP server can be added without rebuilding the visualization.

![SolidCraft Issue Graph](docs/screenshot.png)

## What is in the graph

- **Area** — a top-level area from the triage taxonomy, such as Kernel, UI, I/O or
  Infra.
- **Topic** — a child area, such as Booleans, Blends, Shell & offset or STEP & IGES.
- **Distinct issue** — a cluster of reports that appear to describe the same problem.
  Clusters are inferred from `duplicate_of` links plus a normalized error signature or
  title. This is a triage aid, not a hand-curated truth.
- **GitHub issue** — every issue in both `tangency-labs/solidcraft` and
  `tangency-labs/solidcore`.

Every GitHub issue is present in the API response and can be shown as a leaf node.
The overview turns off leaf issue nodes and single-report groups by default so the
topic structure stays readable; both are toggles in the sidebar.

## Stack

- SvelteKit 3 + Svelte 5 (runes), TypeScript
- `@sveltejs/adapter-node`
- `node:sqlite` (Node 24+) against the read-only triage database
- `d3-force` for the one-time server-side layout
- `sigma` + `graphology` for the WebGL canvas view
- Prettier and ESLint

## Requirements

- Node.js 24 or later (the server uses the built-in `node:sqlite` module)
- The triage database, by default at:
  `~/solidcraft-triage/analysis/triage.sqlite`
- No database server is required. SQLite is opened read-only.

## Set up

```sh
cd /Volumes/Projects/active/personal/solidcraft-issues
npm install
cp .env.example .env
```

Edit `.env` if the database is somewhere else:

```sh
TRIAGE_DB_PATH=/absolute/path/to/solidcraft-triage/analysis/triage.sqlite
```

Start the development server on the default port:

```sh
npm run dev
```

Open <http://localhost:5173/>.

## Build and run

```sh
npm run verify
TRIAGE_DB_PATH=/absolute/path/to/triage.sqlite npm start
```

`npm start` runs the adapter-node server. The default port is `3000`; set `PORT` to
change it.

The graph is built on the first request, then cached in the Node process. At the time
of writing the first build takes roughly 4–6 seconds and produces about 17 MB of
uncompressed JSON. Subsequent requests reuse the cache until
`GRAPH_CACHE_TTL_MS` expires (60 seconds by default) and the database mtime changes.

For remote or shared deployment:

- Put the app behind a reverse proxy with gzip/Brotli enabled.
- Keep `triage.sqlite` read-only for the app user.
- Give the process enough memory for the graph (the in-memory graph and JSON response
  are both large).
- Do not expose a writable triage database to this app.

## Using the graph

- **Scroll** to zoom, **drag** to pan, **click** a node to select it.
- **Fit graph** resets the camera.
- **Search** matches issue numbers, titles, cluster signatures, and labels. Press
  Enter or click a result to select and focus it.
- **Repository / State / Topic / Subtopic** filter the visible issues.
- **Show GitHub issue nodes** reveals every leaf issue node.
- **Show single-report groups** reveals clusters that contain only one report.
- **Show cross-reference links** draws issue-to-issue mention edges.
- The right panel shows a selected node's counts, member issues, labels, and the
  GitHub link.

Node size follows connectivity: hierarchy degree and issue counts make hubs larger;
statistically more connected issue nodes are larger than isolated ones.

## Data model and grouping

The input tables are:

| Table                    | Use                                          |
| ------------------------ | -------------------------------------------- |
| `repos`                  | Repository identity                          |
| `issues`                 | Issue number, title, body, state, dates, URL |
| `tags`, `issue_tags`     | Area hierarchy and type/symptom tags         |
| `labels`, `issue_labels` | GitHub labels                                |
| `refs`                   | `duplicate_of`, `mention` and PR references  |

The server uses the area tags (`kind = 'area'`) as the top two levels. It chooses the
deepest matching area tag as the primary path, then groups issues within that topic:

1. Union issues connected by `duplicate_of` references.
2. For generic in-app titles such as `EXTRUDE: operation failed`, extract the first
   meaningful quoted error from the body and normalize numbers, URLs, and IDs.
3. Otherwise group by normalized title.
4. Add issue-to-issue mention edges as optional cross-reference links.

This means a “distinct issue” is an inferred cluster, not a canonical GitHub issue.
The UI exposes the cluster signature and member reports so a human or agent can split,
merge, or confirm it in the next work-queue phase.

## API

The first read-only endpoint is:

```text
GET /api/graph
GET /api/graph?repo=tangency-labs/solidcraft&state=open
GET /api/graph?area=kernel&topic=booleans
```

It returns `{ meta, nodes, edges }`. See [`docs/api-and-mcp.md`](docs/api-and-mcp.md)
for the node/edge schema, filter semantics, and the proposed work-queue and MCP
contract.

## Project layout

```text
src/
  lib/
    components/GraphView.svelte   Sigma canvas and reducers
    graph/types.ts                Shared API types
    server/graph.ts               SQLite read, clustering, layout, cache
  routes/
    +page.svelte                  Graph shell, filters, detail panel
    api/graph/+server.ts          Read-only graph API
docs/api-and-mcp.md               API and MCP roadmap
scripts/                          Reserved for data/ops scripts
static/                           Static assets
```

## Current limitations

- The graph is a heuristic grouping. Error signatures are not a substitute for a
  maintainer's duplicate decision.
- The API sends the full graph in one response. The queue phase should add paginated
  cluster and issue endpoints.
- The layout is precomputed on the server; changing the database triggers a rebuild
  only after cache expiry.
- The app does not write to GitHub, the triage database, or the ticket server.
- There is no authentication yet. Keep the app local or behind an authenticated
  proxy until the work-queue API exists.

## Roadmap

1. Add a paginated work-queue API backed by a separate `work.sqlite` database.
2. Add cluster claim/release, status, priority, layer, labels, and audit events.
3. Add an MCP server over the API with read-only tools first, then scoped write tools.
4. Add explicit split/merge actions with a record of the decision.
5. Add saved views and deep links for a selected cluster or issue.
6. Add a queue health dashboard (unclaimed, stale claims, blocked, no-buy-in).
