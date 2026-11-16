# Scripts

- `sync.mjs` — runs the Go ingester (`triage sync`) and then `triage retag`,
  records a `sync_runs` row, and uses `GITHUB_TOKEN`, `GH_TOKEN`, or
  `gh auth token`.
- The local MCP server lives in `../mcp/server.mjs`.

The graph is built in the SvelteKit server process (`src/lib/server/graph.ts`) so
the site is driven by the live SQLite database.

No generated database or credentials should be committed here.
