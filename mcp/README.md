# Issue queue MCP server

A local stdio MCP server over the SvelteKit API. It lets an agent list queue
clusters, find ungrouped issues, claim work, group reports, update status and
inspect sync state.

## Run

The SvelteKit app must be running first:

```sh
npm run dev
```

Then start the MCP server:

```sh
npm run mcp:issues
```

Environment:

| Variable         | Default                 | Meaning                                      |
| ---------------- | ----------------------- | -------------------------------------------- |
| `ISSUES_API_URL` | `http://127.0.0.1:5173` | SvelteKit base URL                           |
| `AGENT_TOKEN`    | empty                   | Bearer token for mutations, matching the app |
| `MCP_ALLOW_SYNC` | empty                   | Set to `1` to expose `run_sync`              |

## OpenCode

Add a local MCP server, for example:

```sh
opencode mcp add issues --global -- node \
  /Volumes/Projects/active/personal/solidcraft-issues/mcp/server.mjs
```

Then restart OpenCode so the server inherits the desired `ISSUES_API_URL` and
`AGENT_TOKEN` environment. Do not put a token in the MCP config.

## Tools

Read-only:

- `list_queue`
- `list_unclustered`
- `get_cluster`
- `sync_status`

Write tools:

- `claim_cluster`
- `release_cluster`
- `update_cluster`
- `assign_issues`
- `create_cluster`
- `merge_clusters`
- `run_sync` (disabled unless `MCP_ALLOW_SYNC=1`)

Recommended agent flow:

1. `list_unclustered` for the area/topic being worked.
2. `get_cluster` or `list_queue` to find an existing group.
3. `claim_cluster`.
4. `assign_issues` new reports into the group, or `create_cluster`.
5. `update_cluster` with the next status/layer/priority.
6. `release_cluster` when stopping.

Cluster IDs are stable integer rows in the `clusters` table. The graph reads
the same rows, so agent changes appear in the visualization after the next
graph cache refresh.
