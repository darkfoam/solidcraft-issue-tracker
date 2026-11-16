package query

import (
	"database/sql"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/nclack/solidcraft-triage/internal/db"
)

type IssueRow struct {
	Repo    string
	Number  int
	State   string
	Title   string
	URL     string
	Tags    []string
	Snippet string
}

type TagRow struct {
	ID          int64
	Slug        string
	Name        string
	Kind        string
	ParentID    sql.NullInt64
	Description string
	Source      string
	Confidence  float64
	Signal      string
	OpenCount   int
	TotalCount  int
}

type CommentRow struct {
	Author    string
	CreatedAt string
	Body      string
	URL       string
}

type RefRow struct {
	TargetRepo   string
	TargetNumber int
	Kind         string
	Context      string
	SourceRepo   string
	SourceNumber int
	SourceTitle  string
	SourceKind   string
}

type IssueDetail struct {
	IssueRow
	Body         string
	Author       string
	AuthorType   string
	CreatedAt    string
	UpdatedAt    string
	ClosedAt     string
	HTMLURL      string
	Comments     []CommentRow
	Tags         []TagRow
	OutgoingRefs []RefRow
	IncomingRefs []RefRow
	ClusterName  string
	ClusterWhy   string
	Labels       []string
}

// Repo resolves a short or full repository name to its canonical slug.
func Repo(d *db.DB, spec string) (string, error) {
	spec = strings.ToLower(strings.TrimSpace(spec))
	if spec == "" {
		return "", nil
	}
	if strings.Contains(spec, "/") {
		var slug string
		err := d.QueryRow(`SELECT slug FROM repos WHERE slug=?`, spec).Scan(&slug)
		if err == sql.ErrNoRows {
			return "", fmt.Errorf("repository %q is not in the database", spec)
		}
		return slug, err
	}
	rows, err := d.Query(`SELECT slug FROM repos WHERE slug=? OR slug LIKE ? ORDER BY slug`, spec, "%/"+spec)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var found []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return "", err
		}
		found = append(found, s)
	}
	switch len(found) {
	case 0:
		return "", fmt.Errorf("repository %q is not in the database", spec)
	case 1:
		return found[0], nil
	default:
		return "", fmt.Errorf("repository %q is ambiguous: %s", spec, strings.Join(found, ", "))
	}
}

// ParseIssueSpec accepts 123, repo#123, or owner/repo#123.
func ParseIssueSpec(spec string) (repo string, number int, err error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return "", 0, fmt.Errorf("missing issue number")
	}
	if i := strings.LastIndexByte(spec, '#'); i >= 0 {
		repo = strings.ToLower(strings.TrimSpace(spec[:i]))
		spec = spec[i+1:]
	}
	number, err = strconv.Atoi(spec)
	if err != nil || number <= 0 {
		return "", 0, fmt.Errorf("invalid issue number %q", spec)
	}
	return repo, number, nil
}

func FindIssue(d *db.DB, repoSpec string, number int) (*IssueDetail, error) {
	var repo string
	var err error
	if repoSpec != "" {
		repo, err = Repo(d, repoSpec)
		if err != nil {
			return nil, err
		}
	}
	q := `SELECT i.id,r.slug,i.number,i.state,i.title,i.body,i.author_login,i.author_type,
		i.created_at,i.updated_at,i.closed_at,i.html_url,i.url
		FROM issues i JOIN repos r ON r.id=i.repo_id
		WHERE i.number=?`
	args := []any{number}
	if repo != "" {
		q += ` AND r.slug=?`
		args = append(args, repo)
	}
	rows, err := d.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type found struct {
		id     int64
		detail IssueDetail
	}
	var matches []found
	for rows.Next() {
		var f found
		var id int64
		if err := rows.Scan(&id, &f.detail.Repo, &f.detail.Number, &f.detail.State,
			&f.detail.Title, &f.detail.Body, &f.detail.Author, &f.detail.AuthorType,
			&f.detail.CreatedAt, &f.detail.UpdatedAt, &f.detail.ClosedAt,
			&f.detail.HTMLURL, &f.detail.URL); err != nil {
			return nil, err
		}
		f.id = id
		matches = append(matches, f)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	if len(matches) == 0 {
		return nil, fmt.Errorf("issue %s not found", FormatIssueSpec(repo, number))
	}
	if len(matches) > 1 {
		var opts []string
		for _, m := range matches {
			opts = append(opts, FormatIssueSpec(m.detail.Repo, m.detail.Number))
		}
		return nil, fmt.Errorf("issue number %d exists in several repositories: %s (use repo#number)", number, strings.Join(opts, ", "))
	}
	m := matches[0]
	m.detail.Tags, _ = TagsForIssue(d, m.id)
	m.detail.Comments, _ = CommentsForIssue(d, m.id)
	m.detail.OutgoingRefs, _ = OutgoingRefs(d, m.id)
	m.detail.IncomingRefs, _ = IncomingRefs(d, m.detail.Repo, m.detail.Number)
	m.detail.ClusterName, m.detail.ClusterWhy = ClusterForIssue(d, m.id)
	m.detail.Labels, _ = LabelsForIssue(d, m.id)
	return &m.detail, nil
}

func FindIssueID(d *db.DB, repoSpec string, number int) (int64, string, error) {
	var repo string
	var err error
	if repoSpec != "" {
		repo, err = Repo(d, repoSpec)
		if err != nil {
			return 0, "", err
		}
	}
	q := `SELECT i.id,r.slug FROM issues i JOIN repos r ON r.id=i.repo_id WHERE i.number=?`
	args := []any{number}
	if repo != "" {
		q += ` AND r.slug=?`
		args = append(args, repo)
	}
	rows, err := d.Query(q, args...)
	if err != nil {
		return 0, "", err
	}
	defer rows.Close()
	var ids []int64
	var repos []string
	for rows.Next() {
		var id int64
		var slug string
		if err := rows.Scan(&id, &slug); err != nil {
			return 0, "", err
		}
		ids = append(ids, id)
		repos = append(repos, slug)
	}
	if err := rows.Err(); err != nil {
		return 0, "", err
	}
	if len(ids) == 0 {
		return 0, "", fmt.Errorf("issue %s not found", FormatIssueSpec(repo, number))
	}
	if len(ids) > 1 {
		var opts []string
		for i := range ids {
			opts = append(opts, FormatIssueSpec(repos[i], number))
		}
		return 0, "", fmt.Errorf("issue number %d exists in several repositories: %s (use repo#number)", number, strings.Join(opts, ", "))
	}
	return ids[0], repos[0], nil
}

func FormatIssueSpec(repo string, number int) string {
	if repo == "" {
		return strconv.Itoa(number)
	}
	return repo + "#" + strconv.Itoa(number)
}

func TagsForIssue(d *db.DB, issueID int64) ([]TagRow, error) {
	rows, err := d.Query(`SELECT t.id,t.slug,t.name,t.kind,t.parent_id,t.description,it.source,it.confidence,it.signal
		FROM issue_tags it JOIN tags t ON t.id=it.tag_id
		WHERE it.issue_id=? ORDER BY t.kind,t.slug`, issueID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TagRow
	for rows.Next() {
		var t TagRow
		if err := rows.Scan(&t.ID, &t.Slug, &t.Name, &t.Kind, &t.ParentID, &t.Description,
			&t.Source, &t.Confidence, &t.Signal); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func CommentsForIssue(d *db.DB, issueID int64) ([]CommentRow, error) {
	rows, err := d.Query(`SELECT author_login,created_at,body,html_url FROM comments
		WHERE issue_id=? ORDER BY created_at,id`, issueID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CommentRow
	for rows.Next() {
		var c CommentRow
		if err := rows.Scan(&c.Author, &c.CreatedAt, &c.Body, &c.URL); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func OutgoingRefs(d *db.DB, issueID int64) ([]RefRow, error) {
	rows, err := d.Query(`SELECT target_repo,target_number,kind,context FROM refs
		WHERE from_issue_id=? ORDER BY target_repo,target_number,kind`, issueID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RefRow
	for rows.Next() {
		var r RefRow
		if err := rows.Scan(&r.TargetRepo, &r.TargetNumber, &r.Kind, &r.Context); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func IncomingRefs(d *db.DB, repo string, number int) ([]RefRow, error) {
	rows, err := d.Query(`
		SELECT ri.slug,i.number,i.title,'issue',x.kind,x.context
		FROM refs x JOIN issues i ON i.id=x.from_issue_id JOIN repos ri ON ri.id=i.repo_id
		WHERE x.target_repo=? AND x.target_number=?
		UNION ALL
		SELECT ri.slug,i.number,i.title,'comment',x.kind,x.context
		FROM refs x JOIN comments c ON c.id=x.from_comment_id
		JOIN issues i ON i.id=c.issue_id JOIN repos ri ON ri.id=i.repo_id
		WHERE x.target_repo=? AND x.target_number=?
		UNION ALL
		SELECT rp.slug,p.number,p.title,'pr',x.kind,x.context
		FROM refs x JOIN pull_requests p ON p.id=x.from_pr_id JOIN repos rp ON rp.id=p.repo_id
		WHERE x.target_repo=? AND x.target_number=?
		ORDER BY 1,2`, repo, number, repo, number, repo, number)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RefRow
	for rows.Next() {
		var r RefRow
		if err := rows.Scan(&r.SourceRepo, &r.SourceNumber, &r.SourceTitle, &r.SourceKind, &r.Kind, &r.Context); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func ClusterForIssue(d *db.DB, issueID int64) (string, string) {
	var name, why string
	err := d.QueryRow(`SELECT c.name,c.rationale FROM clusters c JOIN cluster_issues ci ON ci.cluster_id=c.id
		WHERE ci.issue_id=? ORDER BY c.score DESC LIMIT 1`, issueID).Scan(&name, &why)
	if err != nil {
		return "", ""
	}
	return name, why
}

func LabelsForIssue(d *db.DB, issueID int64) ([]string, error) {
	rows, err := d.Query(`SELECT l.name FROM issue_labels il JOIN labels l ON l.id=il.label_id
		WHERE il.issue_id=? ORDER BY l.name`, issueID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// ResolveTag finds one tag by slug, name or alias. kind may be empty or a tag kind.
func ResolveTag(d *db.DB, name, kind string) (*TagRow, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("missing tag")
	}
	queries := []struct {
		q    string
		args []any
	}{
		{`SELECT id,slug,name,kind,parent_id,description FROM tags WHERE lower(slug)=lower(?)`, []any{name}},
		{`SELECT id,slug,name,kind,parent_id,description FROM tags WHERE lower(name)=lower(?)`, []any{name}},
		{`SELECT t.id,t.slug,t.name,t.kind,t.parent_id,t.description FROM tags t
			JOIN tag_aliases a ON a.tag_id=t.id WHERE lower(a.alias)=lower(?)`, []any{name}},
	}
	for _, item := range queries {
		rows, err := d.Query(item.q, item.args...)
		if err != nil {
			return nil, err
		}
		var matches []TagRow
		for rows.Next() {
			var t TagRow
			if err := rows.Scan(&t.ID, &t.Slug, &t.Name, &t.Kind, &t.ParentID, &t.Description); err != nil {
				rows.Close()
				return nil, err
			}
			matches = append(matches, t)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
		if kind != "" {
			var filtered []TagRow
			for _, t := range matches {
				if t.Kind == kind {
					filtered = append(filtered, t)
				}
			}
			matches = filtered
		}
		if len(matches) == 1 {
			return &matches[0], nil
		}
		if len(matches) > 1 {
			var slugs []string
			for _, t := range matches {
				slugs = append(slugs, t.Kind+":"+t.Slug)
			}
			return nil, fmt.Errorf("tag %q is ambiguous: %s", name, strings.Join(slugs, ", "))
		}
	}
	return nil, fmt.Errorf("unknown tag %q", name)
}

func ResolveOrCreateTag(d *db.DB, name string) (*TagRow, error) {
	t, err := ResolveTag(d, name, "")
	if err == nil {
		return t, nil
	}
	slug := Slugify(name)
	if slug == "" {
		return nil, fmt.Errorf("invalid tag name %q", name)
	}
	var id int64
	err = d.QueryRow(`INSERT INTO tags(slug,name,kind) VALUES(?,?,'custom')
		ON CONFLICT(slug) DO UPDATE SET name=excluded.name RETURNING id`, slug, name).Scan(&id)
	if err != nil {
		return nil, err
	}
	return ResolveTag(d, slug, "")
}

func Slugify(s string) string {
	var b strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

func Areas(d *db.DB) ([]TagRow, error) {
	rows, err := d.Query(`
		SELECT t.id,t.slug,t.name,t.kind,t.parent_id,t.description,
		       COUNT(DISTINCT CASE WHEN i.state='open' THEN it.issue_id END),
		       COUNT(DISTINCT it.issue_id)
		FROM tags t
		LEFT JOIN issue_tags it ON it.tag_id=t.id
		LEFT JOIN issues i ON i.id=it.issue_id
		WHERE t.kind='area'
		GROUP BY t.id
		ORDER BY t.name COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TagRow
	for rows.Next() {
		var t TagRow
		if err := rows.Scan(&t.ID, &t.Slug, &t.Name, &t.Kind, &t.ParentID, &t.Description,
			&t.OpenCount, &t.TotalCount); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func AreaIssues(d *db.DB, tagID int64, state string, typeTagID int64, extraTag string, limit int) ([]IssueRow, error) {
	q := `
		SELECT r.slug,i.id,i.number,i.state,i.title,i.html_url
		FROM issue_tags it JOIN issues i ON i.id=it.issue_id JOIN repos r ON r.id=i.repo_id
		WHERE it.tag_id=?`
	args := []any{tagID}
	if state != "" {
		q += ` AND i.state=?`
		args = append(args, state)
	}
	if typeTagID != 0 {
		q += ` AND EXISTS (SELECT 1 FROM issue_tags it2 WHERE it2.issue_id=i.id AND it2.tag_id=?)`
		args = append(args, typeTagID)
	}
	if extraTag != "" {
		q += ` AND EXISTS (SELECT 1 FROM issue_tags it3 JOIN tags t3 ON t3.id=it3.tag_id WHERE it3.issue_id=i.id AND t3.slug=?)`
		args = append(args, extraTag)
	}
	q += ` ORDER BY CASE i.state WHEN 'open' THEN 0 ELSE 1 END, i.updated_at DESC, i.number DESC LIMIT ?`
	args = append(args, limit)
	rows, err := d.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []IssueRow
	var ids []int64
	for rows.Next() {
		var row IssueRow
		var id int64
		if err := rows.Scan(&row.Repo, &id, &row.Number, &row.State, &row.Title, &row.URL); err != nil {
			return nil, err
		}
		ids = append(ids, id)
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	tagMap, _ := AreaTagsForIssueIDs(d, ids)
	for i := range out {
		out[i].Tags = tagMap[ids[i]]
	}
	return out, nil
}

func AreaTagsForIssueIDs(d *db.DB, ids []int64) (map[int64][]string, error) {
	out := map[int64][]string{}
	if len(ids) == 0 {
		return out, nil
	}
	ph := Placeholders(len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	rows, err := d.Query(`SELECT it.issue_id,t.slug FROM issue_tags it JOIN tags t ON t.id=it.tag_id
		WHERE t.kind='area' AND it.issue_id IN (`+ph+`) ORDER BY t.slug`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var slug string
		if err := rows.Scan(&id, &slug); err != nil {
			return nil, err
		}
		out[id] = append(out[id], slug)
	}
	return out, rows.Err()
}

func Search(d *db.DB, text, repo, state, extraTag string, limit int) ([]IssueRow, error) {
	match := FTSQuery(text)
	var rows *sql.Rows
	var err error
	if match == "" {
		q := `SELECT r.slug,i.id,i.number,i.state,i.title,i.html_url FROM issues i JOIN repos r ON r.id=i.repo_id
			WHERE (i.title LIKE ? OR i.body LIKE ?)`
		args := []any{"%" + text + "%", "%" + text + "%"}
		if repo != "" {
			q += ` AND r.slug=?`
			args = append(args, repo)
		}
		if state != "" {
			q += ` AND i.state=?`
			args = append(args, state)
		}
		if extraTag != "" {
			q += ` AND EXISTS (SELECT 1 FROM issue_tags itx JOIN tags tx ON tx.id=itx.tag_id WHERE itx.issue_id=i.id AND tx.slug=?)`
			args = append(args, extraTag)
		}
		q += ` ORDER BY i.updated_at DESC LIMIT ?`
		args = append(args, limit)
		rows, err = d.Query(q, args...)
	} else {
		q := `SELECT r.slug,i.id,i.number,i.state,i.title,i.html_url,
			snippet(issue_fts,-1,'«','»','…',18)
			FROM issue_fts JOIN issues i ON i.id=issue_fts.rowid JOIN repos r ON r.id=i.repo_id
			WHERE issue_fts MATCH ?`
		args := []any{match}
		if repo != "" {
			q += ` AND r.slug=?`
			args = append(args, repo)
		}
		if state != "" {
			q += ` AND i.state=?`
			args = append(args, state)
		}
		if extraTag != "" {
			q += ` AND EXISTS (SELECT 1 FROM issue_tags itx JOIN tags tx ON tx.id=itx.tag_id WHERE itx.issue_id=i.id AND tx.slug=?)`
			args = append(args, extraTag)
		}
		q += ` ORDER BY rank LIMIT ?`
		args = append(args, limit)
		rows, err = d.Query(q, args...)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []IssueRow
	var ids []int64
	for rows.Next() {
		var row IssueRow
		var id int64
		if match == "" {
			if err := rows.Scan(&row.Repo, &id, &row.Number, &row.State, &row.Title, &row.URL); err != nil {
				return nil, err
			}
		} else {
			if err := rows.Scan(&row.Repo, &id, &row.Number, &row.State, &row.Title, &row.URL, &row.Snippet); err != nil {
				return nil, err
			}
		}
		ids = append(ids, id)
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	tagMap, _ := AreaTagsForIssueIDs(d, ids)
	for i := range out {
		out[i].Tags = tagMap[ids[i]]
	}
	return out, nil
}

// FTSQuery turns user text into a safe FTS5 AND query.
func FTSQuery(text string) string {
	var terms []string
	for _, f := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_')
	}) {
		if len(f) < 2 {
			continue
		}
		terms = append(terms, `"`+strings.ReplaceAll(f, `"`, `""`)+`"`)
	}
	return strings.Join(terms, " AND ")
}

func Related(d *db.DB, detail *IssueDetail) ([]IssueRow, error) {
	seen := map[string]bool{}
	var out []IssueRow
	add := func(repo string, number int) {
		key := repo + "#" + strconv.Itoa(number)
		if seen[key] {
			return
		}
		seen[key] = true
		var row IssueRow
		err := d.QueryRow(`SELECT r.slug,i.number,i.state,i.title,i.html_url FROM issues i JOIN repos r ON r.id=i.repo_id
			WHERE r.slug=? AND i.number=?`, repo, number).Scan(&row.Repo, &row.Number, &row.State, &row.Title, &row.URL)
		if err == nil {
			out = append(out, row)
		}
	}
	for _, r := range detail.OutgoingRefs {
		add(r.TargetRepo, r.TargetNumber)
	}
	for _, r := range detail.IncomingRefs {
		add(r.SourceRepo, r.SourceNumber)
	}
	// Cluster members. Collect them first: the DB has one connection, so a
	// QueryRow inside an open Rows loop would wait on itself.
	rows, err := d.Query(`SELECT r.slug,i.number,i.state,i.title,i.html_url
		FROM cluster_issues ci JOIN issues i ON i.id=ci.issue_id JOIN repos r ON r.id=i.repo_id
		WHERE ci.cluster_id IN (SELECT cluster_id FROM cluster_issues WHERE issue_id=(
			SELECT i2.id FROM issues i2 JOIN repos r2 ON r2.id=i2.repo_id WHERE r2.slug=? AND i2.number=?))
		AND i.id != (SELECT i3.id FROM issues i3 JOIN repos r3 ON r3.id=i3.repo_id WHERE r3.slug=? AND i3.number=?)`,
		detail.Repo, detail.Number, detail.Repo, detail.Number)
	if err != nil {
		return nil, err
	}
	var members []IssueRow
	for rows.Next() {
		var row IssueRow
		if err := rows.Scan(&row.Repo, &row.Number, &row.State, &row.Title, &row.URL); err != nil {
			rows.Close()
			return nil, err
		}
		members = append(members, row)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	for _, row := range members {
		add(row.Repo, row.Number)
	}
	return out, nil
}

func Clusters(d *db.DB, top int) ([]struct {
	ID        int64
	Name      string
	Rationale string
	Score     int
	Members   int
}, error) {
	rows, err := d.Query(`SELECT c.id,c.name,c.rationale,c.score,COUNT(ci.issue_id)
		FROM clusters c LEFT JOIN cluster_issues ci ON ci.cluster_id=c.id
		GROUP BY c.id ORDER BY c.score DESC, COUNT(ci.issue_id) DESC LIMIT ?`, top)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []struct {
		ID        int64
		Name      string
		Rationale string
		Score     int
		Members   int
	}
	for rows.Next() {
		var c struct {
			ID        int64
			Name      string
			Rationale string
			Score     int
			Members   int
		}
		if err := rows.Scan(&c.ID, &c.Name, &c.Rationale, &c.Score, &c.Members); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func Stats(d *db.DB) (string, error) {
	var b strings.Builder
	rows, err := d.Query(`SELECT r.slug,
		COUNT(i.id),
		COALESCE(SUM(CASE WHEN i.state='open' THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN i.state='closed' THEN 1 ELSE 0 END),0)
		FROM repos r LEFT JOIN issues i ON i.repo_id=r.id GROUP BY r.id ORDER BY r.slug`)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	for rows.Next() {
		var slug string
		var total, open, closed int
		if err := rows.Scan(&slug, &total, &open, &closed); err != nil {
			return "", err
		}
		fmt.Fprintf(&b, "%s: %d issues (%d open, %d closed)\n", slug, total, open, closed)
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	rows.Close()
	var comments, refs, prs, tags, taggedAny, taggedArea, untaggedOpenAny, untaggedOpenArea int
	d.QueryRow(`SELECT count(*) FROM comments`).Scan(&comments)
	d.QueryRow(`SELECT count(*) FROM refs`).Scan(&refs)
	d.QueryRow(`SELECT count(*) FROM pull_requests`).Scan(&prs)
	d.QueryRow(`SELECT count(*) FROM tags`).Scan(&tags)
	d.QueryRow(`SELECT count(DISTINCT issue_id) FROM issue_tags`).Scan(&taggedAny)
	d.QueryRow(`SELECT count(DISTINCT it.issue_id) FROM issue_tags it JOIN tags t ON t.id=it.tag_id WHERE t.kind='area'`).Scan(&taggedArea)
	d.QueryRow(`SELECT count(*) FROM issues i WHERE i.state='open' AND NOT EXISTS (SELECT 1 FROM issue_tags it WHERE it.issue_id=i.id)`).Scan(&untaggedOpenAny)
	d.QueryRow(`SELECT count(*) FROM issues i WHERE i.state='open' AND NOT EXISTS (
		SELECT 1 FROM issue_tags it JOIN tags t ON t.id=it.tag_id WHERE it.issue_id=i.id AND t.kind='area')`).Scan(&untaggedOpenArea)
	fmt.Fprintf(&b, "comments: %d\ncross-refs: %d\npull request records: %d\ntags: %d\ntagged issues (any): %d\ntagged issues (area): %d\nuntagged open (no tag): %d\nuntagged open (no area): %d\n",
		comments, refs, prs, tags, taggedAny, taggedArea, untaggedOpenAny, untaggedOpenArea)
	var fts int
	d.QueryRow(`SELECT count(*) FROM issue_fts`).Scan(&fts)
	fmt.Fprintf(&b, "full-text rows: %d\n", fts)
	frows, ferr := d.Query(`SELECT t.slug,count(DISTINCT it.issue_id) FROM issue_tags it JOIN tags t ON t.id=it.tag_id
		WHERE t.slug LIKE 'failure/%' GROUP BY t.slug ORDER BY t.slug`)
	if ferr == nil {
		for frows.Next() {
			var slug string
			var n int
			if frows.Scan(&slug, &n) == nil {
				fmt.Fprintf(&b, "%s: %d\n", slug, n)
			}
		}
		frows.Close()
	}
	rows2, err := d.Query(`SELECT r.slug,s.phase,s.mode,s.last_sync_completed_at,s.last_full_sync_completed_at,s.last_error
		FROM sync_state s JOIN repos r ON r.id=s.repo_id ORDER BY r.slug`)
	if err == nil {
		defer rows2.Close()
		for rows2.Next() {
			var slug, phase, mode, last, full, lastErr string
			if err := rows2.Scan(&slug, &phase, &mode, &last, &full, &lastErr); err != nil {
				return "", err
			}
			fmt.Fprintf(&b, "sync %s: phase=%s mode=%s watermark=%s full=%s", slug, phase, mode, orDash(last), orDash(full))
			if lastErr != "" {
				fmt.Fprintf(&b, " error=%q", lastErr)
			}
			b.WriteByte('\n')
		}
	}
	if st, err := os.Stat(d.Path); err == nil {
		fmt.Fprintf(&b, "database: %s (%d bytes)\n", d.Path, st.Size())
	}
	return b.String(), nil
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func Placeholders(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}
