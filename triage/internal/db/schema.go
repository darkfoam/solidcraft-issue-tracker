package db

const schemaSQL = `
CREATE TABLE IF NOT EXISTS meta (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS repos (
  id         INTEGER PRIMARY KEY,
  owner      TEXT NOT NULL,
  name       TEXT NOT NULL,
  slug       TEXT NOT NULL UNIQUE,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE TABLE IF NOT EXISTS issues (
  id             INTEGER PRIMARY KEY,
  repo_id        INTEGER NOT NULL REFERENCES repos(id) ON DELETE CASCADE,
  number         INTEGER NOT NULL,
  title          TEXT NOT NULL DEFAULT '',
  body           TEXT NOT NULL DEFAULT '',
  state          TEXT NOT NULL DEFAULT 'open',
  state_reason   TEXT NOT NULL DEFAULT '',
  author_login   TEXT NOT NULL DEFAULT '',
  author_type    TEXT NOT NULL DEFAULT '',
  created_at     TEXT NOT NULL DEFAULT '',
  updated_at     TEXT NOT NULL DEFAULT '',
  closed_at      TEXT NOT NULL DEFAULT '',
  url            TEXT NOT NULL DEFAULT '',
  html_url       TEXT NOT NULL DEFAULT '',
  comments_count INTEGER NOT NULL DEFAULT 0,
  is_locked      INTEGER NOT NULL DEFAULT 0,
  raw_json       TEXT NOT NULL DEFAULT '{}',
  first_synced_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  last_synced_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  UNIQUE (repo_id, number)
);
CREATE INDEX IF NOT EXISTS issues_repo_state ON issues(repo_id, state);
CREATE INDEX IF NOT EXISTS issues_updated ON issues(updated_at);
CREATE INDEX IF NOT EXISTS issues_repo_number ON issues(repo_id, number);

CREATE TABLE IF NOT EXISTS comments (
  id             INTEGER PRIMARY KEY,
  repo_id        INTEGER NOT NULL REFERENCES repos(id) ON DELETE CASCADE,
  github_id      INTEGER NOT NULL,
  issue_id       INTEGER NOT NULL REFERENCES issues(id) ON DELETE CASCADE,
  author_login   TEXT NOT NULL DEFAULT '',
  author_type    TEXT NOT NULL DEFAULT '',
  body           TEXT NOT NULL DEFAULT '',
  created_at     TEXT NOT NULL DEFAULT '',
  updated_at     TEXT NOT NULL DEFAULT '',
  url            TEXT NOT NULL DEFAULT '',
  html_url       TEXT NOT NULL DEFAULT '',
  raw_json       TEXT NOT NULL DEFAULT '{}',
  first_synced_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  last_synced_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  UNIQUE (repo_id, github_id)
);
CREATE INDEX IF NOT EXISTS comments_issue ON comments(issue_id);
CREATE INDEX IF NOT EXISTS comments_updated ON comments(updated_at);

CREATE TABLE IF NOT EXISTS labels (
  id          INTEGER PRIMARY KEY,
  repo_id     INTEGER NOT NULL REFERENCES repos(id) ON DELETE CASCADE,
  name        TEXT NOT NULL,
  color       TEXT NOT NULL DEFAULT '',
  description TEXT NOT NULL DEFAULT '',
  raw_json    TEXT NOT NULL DEFAULT '{}',
  UNIQUE (repo_id, name)
);

CREATE TABLE IF NOT EXISTS issue_labels (
  issue_id INTEGER NOT NULL REFERENCES issues(id) ON DELETE CASCADE,
  label_id INTEGER NOT NULL REFERENCES labels(id) ON DELETE CASCADE,
  PRIMARY KEY (issue_id, label_id)
);

CREATE TABLE IF NOT EXISTS pull_requests (
  id           INTEGER PRIMARY KEY,
  repo_id      INTEGER NOT NULL REFERENCES repos(id) ON DELETE CASCADE,
  number       INTEGER NOT NULL,
  title        TEXT NOT NULL DEFAULT '',
  body         TEXT NOT NULL DEFAULT '',
  state        TEXT NOT NULL DEFAULT '',
  merged_at    TEXT NOT NULL DEFAULT '',
  author_login TEXT NOT NULL DEFAULT '',
  created_at   TEXT NOT NULL DEFAULT '',
  updated_at   TEXT NOT NULL DEFAULT '',
  closed_at    TEXT NOT NULL DEFAULT '',
  url          TEXT NOT NULL DEFAULT '',
  html_url     TEXT NOT NULL DEFAULT '',
  raw_json     TEXT NOT NULL DEFAULT '{}',
  first_synced_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  last_synced_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  UNIQUE (repo_id, number)
);
CREATE INDEX IF NOT EXISTS pull_requests_repo_number ON pull_requests(repo_id, number);

CREATE TABLE IF NOT EXISTS refs (
  id              INTEGER PRIMARY KEY,
  repo_id         INTEGER NOT NULL REFERENCES repos(id) ON DELETE CASCADE,
  from_issue_id   INTEGER REFERENCES issues(id) ON DELETE CASCADE,
  from_comment_id INTEGER REFERENCES comments(id) ON DELETE CASCADE,
  from_pr_id      INTEGER REFERENCES pull_requests(id) ON DELETE CASCADE,
  target_repo     TEXT NOT NULL DEFAULT '',
  target_number   INTEGER NOT NULL,
  kind            TEXT NOT NULL DEFAULT 'mention',
  context         TEXT NOT NULL DEFAULT '',
  raw             TEXT NOT NULL DEFAULT '',
  created_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX IF NOT EXISTS refs_from_issue ON refs(from_issue_id);
CREATE INDEX IF NOT EXISTS refs_from_comment ON refs(from_comment_id);
CREATE INDEX IF NOT EXISTS refs_from_pr ON refs(from_pr_id);
CREATE INDEX IF NOT EXISTS refs_target ON refs(target_repo, target_number);

CREATE TABLE IF NOT EXISTS tags (
  id          INTEGER PRIMARY KEY,
  slug        TEXT NOT NULL UNIQUE,
  name        TEXT NOT NULL,
  kind        TEXT NOT NULL DEFAULT 'area' CHECK (kind IN ('area','type','symptom','custom')),
  parent_id   INTEGER REFERENCES tags(id) ON DELETE SET NULL,
  description TEXT NOT NULL DEFAULT '',
  created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX IF NOT EXISTS tags_kind ON tags(kind);
CREATE INDEX IF NOT EXISTS tags_parent ON tags(parent_id);

CREATE TABLE IF NOT EXISTS tag_aliases (
  id     INTEGER PRIMARY KEY,
  tag_id INTEGER NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
  alias  TEXT NOT NULL COLLATE NOCASE,
  UNIQUE (alias)
);

CREATE TABLE IF NOT EXISTS tag_paths (
  id     INTEGER PRIMARY KEY,
  tag_id INTEGER NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
  glob   TEXT NOT NULL,
  UNIQUE (tag_id, glob)
);

CREATE TABLE IF NOT EXISTS tag_symbols (
  id     INTEGER PRIMARY KEY,
  tag_id INTEGER NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
  symbol TEXT NOT NULL,
  UNIQUE (tag_id, symbol)
);

CREATE TABLE IF NOT EXISTS issue_tags (
  issue_id   INTEGER NOT NULL REFERENCES issues(id) ON DELETE CASCADE,
  tag_id     INTEGER NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
  source     TEXT NOT NULL DEFAULT 'manual' CHECK (source IN ('manual','rule','path','symbol','keyword','label','llm','embedding')),
  confidence REAL NOT NULL DEFAULT 1.0,
  signal     TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  PRIMARY KEY (issue_id, tag_id)
);
CREATE INDEX IF NOT EXISTS issue_tags_tag ON issue_tags(tag_id);

CREATE TABLE IF NOT EXISTS issue_checks (
  issue_id     INTEGER NOT NULL REFERENCES issues(id) ON DELETE CASCADE,
  method       TEXT NOT NULL,
  version      TEXT NOT NULL DEFAULT '',
  processed_at TEXT NOT NULL,
  note         TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (issue_id, method, version)
);

CREATE TABLE IF NOT EXISTS tag_overrides (
  issue_id INTEGER NOT NULL REFERENCES issues(id) ON DELETE CASCADE,
  tag_id   INTEGER NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
  action   TEXT NOT NULL CHECK (action IN ('add','remove')),
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  PRIMARY KEY (issue_id, tag_id)
);

CREATE TABLE IF NOT EXISTS clusters (
  id           INTEGER PRIMARY KEY,
  name         TEXT NOT NULL,
  rationale    TEXT NOT NULL DEFAULT '',
  root_tag_id  INTEGER REFERENCES tags(id) ON DELETE SET NULL,
  score        INTEGER NOT NULL DEFAULT 0,
  created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE TABLE IF NOT EXISTS cluster_issues (
  cluster_id INTEGER NOT NULL REFERENCES clusters(id) ON DELETE CASCADE,
  issue_id   INTEGER NOT NULL REFERENCES issues(id) ON DELETE CASCADE,
  role       TEXT NOT NULL DEFAULT 'member',
  PRIMARY KEY (cluster_id, issue_id)
);

CREATE UNIQUE INDEX IF NOT EXISTS cluster_issues_one_per_issue ON cluster_issues(issue_id);
CREATE INDEX IF NOT EXISTS cluster_issues_cluster ON cluster_issues(cluster_id);

-- Agent-facing triage state. One row per materialized cluster.
CREATE TABLE IF NOT EXISTS cluster_queue (
  cluster_id       INTEGER PRIMARY KEY REFERENCES clusters(id) ON DELETE CASCADE,
  status           TEXT NOT NULL DEFAULT 'new'
                   CHECK (status IN ('new','triaged','claimed','in_progress','blocked','done','wont_fix')),
  priority         TEXT NOT NULL DEFAULT 'none'
                   CHECK (priority IN ('none','p0','p1','p2','p3')),
  layer            TEXT NOT NULL DEFAULT 'unknown'
                   CHECK (layer IN ('app','ops','kernel','unknown')),
  assignee         TEXT NOT NULL DEFAULT '',
  note             TEXT NOT NULL DEFAULT '',
  version          INTEGER NOT NULL DEFAULT 0,
  claimed_at       TEXT NOT NULL DEFAULT '',
  claim_expires_at TEXT NOT NULL DEFAULT '',
  updated_at       TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE TABLE IF NOT EXISTS cluster_events (
  id         INTEGER PRIMARY KEY,
  cluster_id INTEGER REFERENCES clusters(id) ON DELETE CASCADE,
  actor      TEXT NOT NULL DEFAULT '',
  action     TEXT NOT NULL,
  payload    TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX IF NOT EXISTS cluster_events_cluster ON cluster_events(cluster_id, created_at);

CREATE TABLE IF NOT EXISTS sync_runs (
  id          INTEGER PRIMARY KEY,
  repo        TEXT NOT NULL DEFAULT '',
  started_at  TEXT NOT NULL DEFAULT '',
  finished_at TEXT NOT NULL DEFAULT '',
  status      TEXT NOT NULL DEFAULT 'running',
  mode        TEXT NOT NULL DEFAULT '',
  issues      INTEGER NOT NULL DEFAULT 0,
  comments    INTEGER NOT NULL DEFAULT 0,
  refs        INTEGER NOT NULL DEFAULT 0,
  error       TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS pull_request_status (
  pr_id             INTEGER PRIMARY KEY REFERENCES pull_requests(id) ON DELETE CASCADE,
  draft             INTEGER NOT NULL DEFAULT 0,
  review_decision   TEXT NOT NULL DEFAULT '',
  mergeable         TEXT NOT NULL DEFAULT '',
  mergeable_state   TEXT NOT NULL DEFAULT '',
  checks_state      TEXT NOT NULL DEFAULT '',
  head_sha          TEXT NOT NULL DEFAULT '',
  updated_at        TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE TABLE IF NOT EXISTS sync_state (
  repo_id                   INTEGER PRIMARY KEY REFERENCES repos(id) ON DELETE CASCADE,
  phase                     TEXT NOT NULL DEFAULT 'done',
  mode                      TEXT NOT NULL DEFAULT '',
  since                     TEXT NOT NULL DEFAULT '',
  cursor                    TEXT NOT NULL DEFAULT '',
  sync_started_at           TEXT NOT NULL DEFAULT '',
  last_sync_completed_at    TEXT NOT NULL DEFAULT '',
  last_full_sync_completed_at TEXT NOT NULL DEFAULT '',
  full_in_progress          INTEGER NOT NULL DEFAULT 0,
  last_error                TEXT NOT NULL DEFAULT '',
  updated_at                TEXT NOT NULL DEFAULT ''
);

CREATE VIRTUAL TABLE IF NOT EXISTS issue_fts USING fts5(
  title,
  body,
  comments,
  repo_slug UNINDEXED,
  number UNINDEXED,
  tokenize = 'porter unicode61'
);
`

const schemaVersion = "3"
