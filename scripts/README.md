# Scripts

The graph build currently runs inside the SvelteKit server process
(`src/lib/server/graph.ts`) so the site is driven by the live triage database.

Future operator scripts belong here:

- `refresh-graph` — rebuild and persist a layout snapshot.
- `export-graph` — write the current graph to a JSON file for inspection.
- `work-queue` — initialize/migrate the separate write database.

No generated database or credentials should be committed here.
