# Scripts

- `sync.mjs` — internal GitHub sync, taxonomy retag, and linked-PR status refresh.
  It reads `GITHUB_TOKEN`, `GH_TOKEN`, or `gh auth token`.
- `pr-status.mjs` — internal GraphQL refresh of open linked PR review/check state.

These are operator jobs, not public API endpoints. The public app and MCP are
read-only.

No generated database or credentials should be committed here.
