# Triage ingester

The embedded Go GitHub ingester. It syncs issues, pull requests, comments,
cross-references and labels into SQLite, and applies the deterministic issue
taxonomy.

The public SvelteKit app only reads this database. Sync and organization jobs are
internal.

## Build

```sh
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

From the repository root:

```sh
npm run sync
npm run sync -- --full
```

The sync wrapper reads `GITHUB_TOKEN` or `GH_TOKEN`, or uses `gh auth token`.

## Query

```sh
TRIAGE_DB=../data/triage.sqlite ./bin/triage stats
TRIAGE_DB=../data/triage.sqlite ./bin/triage area kernel
TRIAGE_DB=../data/triage.sqlite ./bin/triage search "boolean"
TRIAGE_DB=../data/triage.sqlite ./bin/triage clusters --top 20
```

The database schema includes source tables, cluster tables, pull-request status,
`sync_runs` and sync state. The public app is read-only.
