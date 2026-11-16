# Triage ingester

This is the Go GitHub ingester imported from `~/solidcraft-triage`. It syncs
SolidCraft and solidcore issues, pull requests, comments, cross-references and
labels into SQLite, and applies the deterministic area taxonomy.

The SvelteKit app reads the same database. The app owns the cluster/work tables;
this binary owns the GitHub source rows and taxonomy tags.

## Build

```sh
cd triage
go build -o bin/triage ./cmd/triage
```

Or from the repository root:

```sh
npm run triage:build
```

## Sync

```sh
TRIAGE_DB=../data/triage.sqlite ./bin/triage sync
TRIAGE_DB=../data/triage.sqlite ./bin/triage retag
```

The wrapper at the repository root reads `GITHUB_TOKEN`, or uses `gh auth token`
when no token is set:

```sh
npm run sync
```

`npm run sync:full` exists for a deliberate full re-fetch. Normal scheduled runs
are incremental.

## Query

```sh
TRIAGE_DB=../data/triage.sqlite ./bin/triage stats
TRIAGE_DB=../data/triage.sqlite ./bin/triage area kernel
TRIAGE_DB=../data/triage.sqlite ./bin/triage search "boolean"
TRIAGE_DB=../data/triage.sqlite ./bin/triage clusters --top 20
```

The database schema includes the source tables plus `cluster_queue`,
`cluster_events` and `sync_runs` used by the graph/work-queue API.
