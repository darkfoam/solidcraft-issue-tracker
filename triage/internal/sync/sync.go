package sync

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/nclack/solidcraft-triage/internal/db"
	gh "github.com/nclack/solidcraft-triage/internal/github"
	"github.com/nclack/solidcraft-triage/internal/refs"
)

type Syncer struct {
	DB   *db.DB
	GH   *gh.Client
	Logf func(format string, args ...any)
}

type Stats struct {
	Repo     string
	Mode     string
	Issues   int
	PRs      int
	Comments int
	Refs     int
	Skipped  int
	Partial  bool
	Started  time.Time
	Finished time.Time
}

type state struct {
	Phase                   string
	Mode                    string
	Since                   string
	Cursor                  string
	SyncStartedAt           string
	Watermark               string
	LastSyncCompletedAt     string
	LastFullSyncCompletedAt string
	FullInProgress          bool
	LastError               string
}

func (s *Syncer) Run(ctx context.Context, slug string, full bool, limit int) (Stats, error) {
	if strings.TrimSpace(s.GH.Token) == "" {
		return Stats{}, fmt.Errorf("no GitHub token: run the wrapper (it uses `gh auth token`) or set GITHUB_TOKEN")
	}
	repoID, err := s.DB.EnsureRepo(slug)
	if err != nil {
		return Stats{}, err
	}
	st, err := s.loadState(repoID)
	if err != nil {
		return Stats{}, err
	}
	stats := Stats{Repo: slug, Started: time.Now()}
	now := time.Now().UTC().Format(time.RFC3339)

	switch {
	case full:
		st = state{Phase: "issues", Mode: "full", SyncStartedAt: now, FullInProgress: true}
		s.log("starting a fresh full sync")
	case st.Phase == "issues" || st.Phase == "comments":
		s.log("resuming %s sync (%s, cursor %s)", st.Mode, st.Phase, shortTime(st.Cursor))
	case st.LastFullSyncCompletedAt == "" || st.Watermark == "":
		st = state{Phase: "issues", Mode: "full", SyncStartedAt: now, FullInProgress: true}
		s.log("no completed full sync yet; starting one")
	default:
		watermark := st.Watermark
		fullDone := st.LastFullSyncCompletedAt
		st = state{Phase: "issues", Mode: "incremental", Since: watermark, SyncStartedAt: now,
			Watermark: watermark, LastFullSyncCompletedAt: fullDone}
		s.log("starting incremental sync since %s", shortTime(st.Since))
	}
	stats.Mode = st.Mode
	if err := s.saveState(repoID, st); err != nil {
		return stats, err
	}

	if st.Phase == "issues" {
		done, err := s.syncIssues(ctx, repoID, slug, &st, limit, &stats)
		if err != nil {
			s.fail(repoID, st, err)
			stats.Finished = time.Now()
			return stats, err
		}
		if !done {
			stats.Partial = true
			stats.Finished = time.Now()
			return stats, nil
		}
		st.Phase = "comments"
		st.Cursor = ""
		if err := s.saveState(repoID, st); err != nil {
			return stats, err
		}
	}

	if st.Phase == "comments" {
		if err := s.syncComments(ctx, repoID, slug, &st, &stats); err != nil {
			s.fail(repoID, st, err)
			stats.Finished = time.Now()
			return stats, err
		}
	}

	// The watermark is when this run started, not when it ended, so anything
	// updated while the sync was running is fetched by the next run.
	st.Phase = "done"
	st.Mode = ""
	st.Since = ""
	st.Cursor = ""
	st.Watermark = st.SyncStartedAt
	st.LastSyncCompletedAt = db.Now()
	if st.FullInProgress {
		st.LastFullSyncCompletedAt = db.Now()
		st.FullInProgress = false
	}
	st.LastError = ""
	if err := s.saveState(repoID, st); err != nil {
		return stats, err
	}
	stats.Finished = time.Now()
	return stats, nil
}

func (s *Syncer) syncIssues(ctx context.Context, repoID int64, slug string, st *state, limit int, stats *Stats) (bool, error) {
	next := ""
	page := 0
	fetched := 0
	for {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		page++
		var (
			body []byte
			hdr  http.Header
			err  error
		)
		if next != "" {
			body, hdr, err = s.GH.GetURL(ctx, next)
		} else {
			q := url.Values{}
			q.Set("state", "all")
			q.Set("sort", "updated")
			q.Set("direction", "asc")
			q.Set("per_page", "100")
			if since := effectiveSince(st); since != "" {
				q.Set("since", since)
			}
			body, hdr, err = s.GH.Get(ctx, "/repos/"+slug+"/issues", q)
		}
		if err != nil {
			return false, err
		}
		items, err := gh.Page(body)
		if err != nil {
			return false, fmt.Errorf("decode issues page %d: %w", page, err)
		}
		if len(items) == 0 {
			s.log("issues: %d pages, %d fetched, done", page-1, fetched)
			return true, nil
		}

		tx, err := s.DB.Begin()
		if err != nil {
			return false, err
		}
		lastUpdated := ""
		affected := make([]int64, 0, len(items))
		pageIssues, pagePRs, pageRefs := 0, 0, 0
		reachedLimit := false
		for _, raw := range items {
			var issue gh.Issue
			if err := json.Unmarshal(raw, &issue); err != nil {
				tx.Rollback()
				return false, fmt.Errorf("decode issue on page %d: %w", page, err)
			}
			if !issue.UpdatedAt.IsZero() {
				lastUpdated = issue.UpdatedAt.UTC().Format(time.RFC3339)
			}
			if issue.PullRequest != nil {
				nrefs, err := s.upsertPR(tx, repoID, slug, &issue, raw)
				if err != nil {
					tx.Rollback()
					return false, err
				}
				pagePRs++
				stats.PRs++
				stats.Refs += nrefs
				continue
			}
			id, nrefs, err := s.upsertIssue(tx, repoID, slug, &issue, raw)
			if err != nil {
				tx.Rollback()
				return false, err
			}
			pageIssues++
			pageRefs += nrefs
			stats.Issues++
			stats.Refs += nrefs
			fetched++
			affected = append(affected, id)
			if limit > 0 && fetched >= limit {
				reachedLimit = true
				break
			}
		}
		if err := tx.Commit(); err != nil {
			return false, fmt.Errorf("commit issues page %d: %w", page, err)
		}
		for _, id := range affected {
			if err := s.DB.RefreshIssueFTS(id); err != nil {
				return false, err
			}
		}
		if lastUpdated != "" {
			st.Cursor = lastUpdated
			if err := s.saveState(repoID, *st); err != nil {
				return false, err
			}
		}
		s.log("issues: page %d, %d issues, %d PRs, %d refs (total %d issues)", page, pageIssues, pagePRs, pageRefs, stats.Issues)
		if reachedLimit {
			s.log("stopped at --limit %d; run sync again to resume", limit)
			return false, nil
		}
		next = gh.NextLink(hdr)
		if next == "" {
			s.log("issues: %d pages, %d fetched, done", page, fetched)
			return true, nil
		}
	}
}

func (s *Syncer) syncComments(ctx context.Context, repoID int64, slug string, st *state, stats *Stats) error {
	next := ""
	page := 0
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		page++
		var (
			body []byte
			hdr  http.Header
			err  error
		)
		if next != "" {
			body, hdr, err = s.GH.GetURL(ctx, next)
		} else {
			q := url.Values{}
			q.Set("sort", "updated")
			q.Set("direction", "asc")
			q.Set("per_page", "100")
			if since := effectiveSince(st); since != "" {
				q.Set("since", since)
			}
			body, hdr, err = s.GH.Get(ctx, "/repos/"+slug+"/issues/comments", q)
		}
		if err != nil {
			return err
		}
		items, err := gh.Page(body)
		if err != nil {
			return fmt.Errorf("decode comments page %d: %w", page, err)
		}
		if len(items) == 0 {
			s.log("comments: %d pages, %d stored, done", page-1, stats.Comments)
			return nil
		}

		tx, err := s.DB.Begin()
		if err != nil {
			return err
		}
		lastUpdated := ""
		affectedSet := map[int64]bool{}
		pageComments, pageRefs := 0, 0
		for _, raw := range items {
			var comment gh.Comment
			if err := json.Unmarshal(raw, &comment); err != nil {
				tx.Rollback()
				return fmt.Errorf("decode comment on page %d: %w", page, err)
			}
			if !comment.UpdatedAt.IsZero() {
				lastUpdated = comment.UpdatedAt.UTC().Format(time.RFC3339)
			}
			number, ok := issueNumberFromURL(comment.IssueURL)
			if !ok {
				stats.Skipped++
				continue
			}
			var issueID int64
			err := tx.QueryRow(`SELECT id FROM issues WHERE repo_id=? AND number=?`, repoID, number).Scan(&issueID)
			if err == sql.ErrNoRows {
				// A comment on a pull request (PRs are not stored as issues).
				stats.Skipped++
				continue
			}
			if err != nil {
				tx.Rollback()
				return err
			}
			nrefs, err := s.upsertComment(tx, repoID, slug, issueID, &comment, raw)
			if err != nil {
				tx.Rollback()
				return err
			}
			pageComments++
			pageRefs += nrefs
			stats.Comments++
			stats.Refs += nrefs
			affectedSet[issueID] = true
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit comments page %d: %w", page, err)
		}
		for id := range affectedSet {
			if err := s.DB.RefreshIssueFTS(id); err != nil {
				return err
			}
		}
		if lastUpdated != "" {
			st.Cursor = lastUpdated
			if err := s.saveState(repoID, *st); err != nil {
				return err
			}
		}
		s.log("comments: page %d, %d stored, %d refs (total %d comments, %d skipped)", page, pageComments, pageRefs, stats.Comments, stats.Skipped)
		next = gh.NextLink(hdr)
		if next == "" {
			s.log("comments: %d pages, %d stored, done", page, stats.Comments)
			return nil
		}
	}
}

func (s *Syncer) upsertIssue(tx *sql.Tx, repoID int64, slug string, issue *gh.Issue, raw json.RawMessage) (int64, int, error) {
	var id int64
	err := tx.QueryRow(`
		INSERT INTO issues(repo_id,number,title,body,state,state_reason,author_login,author_type,
			created_at,updated_at,closed_at,url,html_url,comments_count,is_locked,raw_json,last_synced_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(repo_id,number) DO UPDATE SET
			title=excluded.title, body=excluded.body, state=excluded.state,
			state_reason=excluded.state_reason, author_login=excluded.author_login,
			author_type=excluded.author_type, created_at=excluded.created_at,
			updated_at=excluded.updated_at, closed_at=excluded.closed_at,
			url=excluded.url, html_url=excluded.html_url, comments_count=excluded.comments_count,
			is_locked=excluded.is_locked, raw_json=excluded.raw_json, last_synced_at=excluded.last_synced_at
		RETURNING id`,
		repoID, issue.Number, issue.Title, issue.Body, issue.State, issue.StateReason,
		issue.User.LoginOrEmpty(), issue.User.TypeOrEmpty(),
		ts(issue.CreatedAt), ts(issue.UpdatedAt), tsPtr(issue.ClosedAt),
		issue.URL, issue.HTMLURL, issue.Comments, db.Bool(issue.Locked), string(raw), db.Now(),
	).Scan(&id)
	if err != nil {
		return 0, 0, fmt.Errorf("upsert issue #%d: %w", issue.Number, err)
	}

	if _, err := tx.Exec(`DELETE FROM issue_labels WHERE issue_id=?`, id); err != nil {
		return 0, 0, err
	}
	for _, label := range issue.Labels {
		var labelID int64
		if err := tx.QueryRow(`
			INSERT INTO labels(repo_id,name,color,description,raw_json) VALUES(?,?,?,?,?)
			ON CONFLICT(repo_id,name) DO UPDATE SET color=excluded.color, description=excluded.description, raw_json=excluded.raw_json
			RETURNING id`, repoID, label.Name, label.Color, label.Description, marshal(label)).Scan(&labelID); err != nil {
			return 0, 0, err
		}
		if _, err := tx.Exec(`INSERT INTO issue_labels(issue_id,label_id) VALUES(?,?) ON CONFLICT DO NOTHING`, id, labelID); err != nil {
			return 0, 0, err
		}
	}

	if _, err := tx.Exec(`DELETE FROM refs WHERE from_issue_id=?`, id); err != nil {
		return 0, 0, err
	}
	nrefs, err := insertRefs(tx, repoID, id, 0, 0, slug, issue.Number, issueBody(issue))
	if err != nil {
		return 0, 0, err
	}
	return id, nrefs, nil
}

func (s *Syncer) upsertPR(tx *sql.Tx, repoID int64, slug string, issue *gh.Issue, raw json.RawMessage) (int, error) {
	mergedAt := ""
	if issue.PullRequest != nil {
		mergedAt = tsPtr(issue.PullRequest.MergedAt)
	}
	var id int64
	err := tx.QueryRow(`
		INSERT INTO pull_requests(repo_id,number,title,body,state,merged_at,author_login,
			created_at,updated_at,closed_at,url,html_url,raw_json,last_synced_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(repo_id,number) DO UPDATE SET
			title=excluded.title, body=excluded.body, state=excluded.state,
			merged_at=excluded.merged_at, author_login=excluded.author_login,
			created_at=excluded.created_at, updated_at=excluded.updated_at,
			closed_at=excluded.closed_at, url=excluded.url, html_url=excluded.html_url,
			raw_json=excluded.raw_json, last_synced_at=excluded.last_synced_at
		RETURNING id`,
		repoID, issue.Number, issue.Title, issue.Body, issue.State, mergedAt,
		issue.User.LoginOrEmpty(), ts(issue.CreatedAt), ts(issue.UpdatedAt), tsPtr(issue.ClosedAt),
		issue.URL, issue.HTMLURL, string(raw), db.Now(),
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("upsert PR #%d: %w", issue.Number, err)
	}
	if _, err := tx.Exec(`DELETE FROM refs WHERE from_pr_id=?`, id); err != nil {
		return 0, err
	}
	return insertRefs(tx, repoID, 0, 0, id, slug, issue.Number, issue.Body)
}

func (s *Syncer) upsertComment(tx *sql.Tx, repoID int64, slug string, issueID int64, comment *gh.Comment, raw json.RawMessage) (int, error) {
	var id int64
	err := tx.QueryRow(`
		INSERT INTO comments(repo_id,github_id,issue_id,author_login,author_type,body,created_at,
			updated_at,url,html_url,raw_json,last_synced_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(repo_id,github_id) DO UPDATE SET
			issue_id=excluded.issue_id, author_login=excluded.author_login,
			author_type=excluded.author_type, body=excluded.body, created_at=excluded.created_at,
			updated_at=excluded.updated_at, url=excluded.url, html_url=excluded.html_url,
			raw_json=excluded.raw_json, last_synced_at=excluded.last_synced_at
		RETURNING id`,
		repoID, comment.ID, issueID, comment.User.LoginOrEmpty(), comment.User.TypeOrEmpty(),
		comment.Body, ts(comment.CreatedAt), ts(comment.UpdatedAt), comment.URL, comment.HTMLURL,
		string(raw), db.Now(),
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("upsert comment %d: %w", comment.ID, err)
	}
	if _, err := tx.Exec(`DELETE FROM refs WHERE from_comment_id=?`, id); err != nil {
		return 0, err
	}
	number, _ := issueNumberFromURL(comment.IssueURL)
	return insertRefs(tx, repoID, 0, id, 0, slug, number, comment.Body)
}

func insertRefs(tx *sql.Tx, repoID, fromIssue, fromComment, fromPR int64, defaultRepo string, self int, text string) (int, error) {
	found := refs.Extract(text, defaultRepo, self)
	for _, r := range found {
		if _, err := tx.Exec(`INSERT INTO refs(repo_id,from_issue_id,from_comment_id,from_pr_id,
			target_repo,target_number,kind,context,raw) VALUES(?,?,?,?,?,?,?,?,?)`,
			repoID, nullID(fromIssue), nullID(fromComment), nullID(fromPR),
			r.TargetRepo, r.TargetNumber, r.Kind, r.Context, r.Raw); err != nil {
			return 0, err
		}
	}
	return len(found), nil
}

func (s *Syncer) loadState(repoID int64) (state, error) {
	var st state
	var full int
	err := s.DB.QueryRow(`SELECT phase,mode,since,cursor,sync_started_at,
		last_sync_completed_at,last_sync_completed_at,last_full_sync_completed_at,full_in_progress,last_error
		FROM sync_state WHERE repo_id=?`, repoID).Scan(
		&st.Phase, &st.Mode, &st.Since, &st.Cursor, &st.SyncStartedAt, &st.Watermark,
		&st.LastSyncCompletedAt, &st.LastFullSyncCompletedAt, &full, &st.LastError)
	if err == sql.ErrNoRows {
		return state{Phase: "done"}, nil
	}
	if err != nil {
		return st, err
	}
	st.FullInProgress = full != 0
	return st, nil
}

func (s *Syncer) saveState(repoID int64, st state) error {
	_, err := s.DB.Exec(`INSERT INTO sync_state(repo_id,phase,mode,since,cursor,sync_started_at,
		last_sync_completed_at,last_full_sync_completed_at,full_in_progress,last_error,updated_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(repo_id) DO UPDATE SET
			phase=excluded.phase, mode=excluded.mode, since=excluded.since,
			cursor=excluded.cursor, sync_started_at=excluded.sync_started_at,
			last_sync_completed_at=excluded.last_sync_completed_at,
			last_full_sync_completed_at=excluded.last_full_sync_completed_at,
			full_in_progress=excluded.full_in_progress, last_error=excluded.last_error,
			updated_at=excluded.updated_at`,
		repoID, st.Phase, st.Mode, st.Since, st.Cursor, st.SyncStartedAt,
		st.Watermark, st.LastFullSyncCompletedAt, db.Bool(st.FullInProgress), st.LastError, db.Now())
	return err
}

func (s *Syncer) fail(repoID int64, st state, err error) {
	st.LastError = err.Error()
	_ = s.saveState(repoID, st)
}

func (s *Syncer) log(format string, args ...any) {
	if s.Logf != nil {
		s.Logf(format, args...)
	}
}

func effectiveSince(st *state) string {
	since := st.Since
	if st.Cursor != "" && (since == "" || st.Cursor > since) {
		since = st.Cursor
	}
	return since
}

func issueBody(issue *gh.Issue) string {
	if issue.Title == "" {
		return issue.Body
	}
	return issue.Title + "\n" + issue.Body
}

func issueNumberFromURL(u string) (int, bool) {
	i := strings.LastIndex(u, "/issues/")
	if i < 0 {
		return 0, false
	}
	n, err := strconv.Atoi(strings.TrimSuffix(u[i+len("/issues/"):], "/"))
	if err != nil || n <= 0 {
		return 0, false
	}
	return n, true
}

func ts(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func tsPtr(t *time.Time) string {
	if t == nil || t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func marshal(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func nullID(id int64) any {
	if id == 0 {
		return nil
	}
	return id
}

func shortTime(s string) string {
	if s == "" {
		return "(start)"
	}
	return s
}
