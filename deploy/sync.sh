#!/bin/sh
set -eu

APP_DIR="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
cd "$APP_DIR"

# Keep one run at a time. The lock is intentionally outside the repository.
LOCK="${SYNC_LOCK:-/tmp/solidcraft-issue-tracker-sync.lock}"
exec 9>"$LOCK"
if ! flock -n 9; then
	echo "sync already running" >&2
	exit 0
fi

mkdir -p data/backups
find data/backups -type f -name 'triage-*.sqlite' -mtime +14 -delete
docker compose --profile backup run --rm -T backup || echo "backup failed; continuing with sync" >&2
docker compose --profile sync run --rm -T sync
