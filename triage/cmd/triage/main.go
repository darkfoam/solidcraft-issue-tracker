package main

import (
	"bufio"
	"context"
	"database/sql"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/nclack/solidcraft-triage/internal/db"
	gh "github.com/nclack/solidcraft-triage/internal/github"
	"github.com/nclack/solidcraft-triage/internal/query"
	"github.com/nclack/solidcraft-triage/internal/sync"
	"github.com/nclack/solidcraft-triage/internal/taxonomy"
)

const version = "0.1.0"

var defaultRepos = []string{"tangency-labs/solidcraft", "tangency-labs/solidcore"}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(0)
	}
	cmd, args := os.Args[1], os.Args[2:]
	switch cmd {
	case "sync":
		cmdSync(args)
	case "areas":
		cmdAreas(args)
	case "area":
		cmdArea(args)
	case "search":
		cmdSearch(args)
	case "issue":
		cmdIssue(args)
	case "related":
		cmdRelated(args)
	case "clusters":
		cmdClusters(args)
	case "tag":
		cmdTag(args, true)
	case "untag":
		cmdTag(args, false)
	case "stats":
		cmdStats(args)
	case "sql":
		cmdSQL(args)
	case "retag":
		cmdRetag(args)
	case "apply-tags":
		cmdApplyTags(args)
	case "help", "-h", "--help":
		usage()
	case "version", "--version":
		fmt.Println("triage", version)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", cmd)
		usage()
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `triage - local SolidCraft/Core issue index

  triage sync [--full] [--repo owner/name] [--limit N]
  triage areas
  triage area <name-or-alias> [--state open|closed] [--type bug] [--limit N]
  triage search "<text>" [--state open|closed] [--repo name] [--limit N]
  triage issue <number|repo#number> [--repo name]
  triage related <number|repo#number> [--repo name]
  triage clusters [--top N]
  triage tag <number|repo#number> <tag>
  triage untag <number|repo#number> <tag>
  triage retag [--dry-run] [--all] [--limit N]
  triage apply-tags [--source rule] [--prefix failure/] < tags.tsv
  triage stats

The database lives on the solidcraft-triage Podman volume at /data/triage.sqlite.
Set TRIAGE_DB to use another path. Sync reads GITHUB_TOKEN or GH_TOKEN.
`)
}

func openDB() *db.DB {
	d, err := db.Open(os.Getenv("TRIAGE_DB"))
	if err != nil {
		die("%v", err)
	}
	return d
}

func die(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "triage: "+format+"\n", args...)
	os.Exit(1)
}

func flags(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	return fs
}

func parseFlags(fs *flag.FlagSet, args []string) {
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
}

func validState(s string) string {
	switch s {
	case "", "all":
		return ""
	case "open", "closed":
		return s
	default:
		die("--state must be open, closed or all")
		return ""
	}
}

func canonicalRepo(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if strings.Contains(s, "/") {
		return s
	}
	for _, known := range defaultRepos {
		if strings.HasSuffix(known, "/"+s) {
			return known
		}
	}
	return s
}

func repoOrEmpty(d *db.DB, spec string) string {
	if spec == "" {
		return ""
	}
	slug, err := query.Repo(d, spec)
	if err != nil {
		die("%v", err)
	}
	return slug
}

func cmdSync(args []string) {
	fs := flags("sync")
	full := fs.Bool("full", false, "discard cursors and fetch everything again")
	repoFlag := fs.String("repo", "", "only this repository (owner/name or short name); default is both")
	limit := fs.Int("limit", 0, "stop after this many issues (debug; rerun to resume)")
	parseFlags(fs, args)
	if *limit < 0 {
		die("--limit must be zero or positive")
	}

	slugs := append([]string(nil), defaultRepos...)
	if strings.TrimSpace(*repoFlag) != "" {
		slugs = nil
		for _, s := range strings.Split(*repoFlag, ",") {
			s = strings.TrimSpace(s)
			if s != "" {
				slugs = append(slugs, canonicalRepo(s))
			}
		}
	}

	d := openDB()
	defer d.Close()

	for _, slug := range slugs {
		if _, err := d.EnsureRepo(slug); err != nil {
			die("%v", err)
		}
	}

	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		token = os.Getenv("GH_TOKEN")
	}
	if token == "" {
		die("no GitHub token. Run through the wrapper (it uses `gh auth token`) or set GITHUB_TOKEN")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	logLine := func(format string, a ...any) {
		fmt.Fprintf(os.Stderr, "[%s] %s\n", time.Now().Format("15:04:05"), fmt.Sprintf(format, a...))
	}
	client := gh.New(token)
	client.Logf = logLine
	syncer := &sync.Syncer{DB: d, GH: client, Logf: logLine}

	failed := false
	for _, slug := range slugs {
		fmt.Printf("== %s ==\n", slug)
		st, err := syncer.Run(ctx, slug, *full, *limit)
		if err != nil {
			fmt.Fprintf(os.Stderr, "triage: %s: %v\n", slug, err)
			failed = true
			continue
		}
		status := st.Mode + " sync"
		if st.Partial {
			status += " (partial; run sync again to resume)"
		}
		fmt.Printf("%s: %s, %d issues, %d pull requests, %d comments, %d cross-refs\n",
			slug, status, st.Issues, st.PRs, st.Comments, st.Refs)
		if !st.Finished.IsZero() && !st.Started.IsZero() {
			fmt.Printf("  %.1fs\n", st.Finished.Sub(st.Started).Seconds())
		}
	}
	if failed {
		os.Exit(1)
	}
	printTotals(d)
}

func printTotals(d *db.DB) {
	rows, err := d.Query(`SELECT r.slug, COUNT(i.id),
		COALESCE(SUM(CASE WHEN i.state='open' THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN i.state='closed' THEN 1 ELSE 0 END),0),
		(SELECT COUNT(*) FROM comments c WHERE c.repo_id=r.id),
		(SELECT COUNT(*) FROM refs x WHERE x.repo_id=r.id)
		FROM repos r LEFT JOIN issues i ON i.repo_id=r.id
		GROUP BY r.id ORDER BY r.slug`)
	if err != nil {
		return
	}
	defer rows.Close()
	fmt.Println("\nTotals:")
	for rows.Next() {
		var slug string
		var total, open, closed, comments, refs int
		if err := rows.Scan(&slug, &total, &open, &closed, &comments, &refs); err != nil {
			return
		}
		fmt.Printf("  %s: %d issues (%d open, %d closed), %d comments, %d cross-refs\n", slug, total, open, closed, comments, refs)
	}
}

func cmdAreas(args []string) {
	fs := flags("areas")
	parseFlags(fs, args)
	d := openDB()
	defer d.Close()
	areas, err := query.Areas(d)
	if err != nil {
		die("%v", err)
	}
	if len(areas) == 0 {
		fmt.Println("No areas defined yet. Phase 2 proposes the taxonomy.")
		return
	}
	byID := map[int64]query.TagRow{}
	for _, a := range areas {
		byID[a.ID] = a
	}
	children := map[int64][]query.TagRow{}
	var roots []query.TagRow
	for _, a := range areas {
		if a.ParentID.Valid {
			if _, ok := byID[a.ParentID.Int64]; ok {
				children[a.ParentID.Int64] = append(children[a.ParentID.Int64], a)
				continue
			}
		}
		roots = append(roots, a)
	}
	sort.Slice(roots, func(i, j int) bool { return strings.ToLower(roots[i].Name) < strings.ToLower(roots[j].Name) })
	for k := range children {
		sort.Slice(children[k], func(i, j int) bool {
			return strings.ToLower(children[k][i].Name) < strings.ToLower(children[k][j].Name)
		})
	}
	var walk func(a query.TagRow, depth int, seen map[int64]bool)
	walk = func(a query.TagRow, depth int, seen map[int64]bool) {
		if seen[a.ID] {
			return
		}
		seen[a.ID] = true
		count := fmt.Sprintf("%d", a.OpenCount)
		if a.TotalCount != a.OpenCount {
			count = fmt.Sprintf("%d/%d", a.OpenCount, a.TotalCount)
		}
		fmt.Printf("%s%s (%s) [%s]\n", strings.Repeat("  ", depth), a.Name, count, a.Slug)
		for _, c := range children[a.ID] {
			walk(c, depth+1, seen)
		}
	}
	seen := map[int64]bool{}
	for _, a := range roots {
		walk(a, 0, seen)
	}
}

func cmdArea(args []string) {
	if len(args) == 0 {
		die("usage: triage area <name-or-alias> [--state open|closed] [--type bug] [--limit N]")
	}
	name := args[0]
	fs := flags("area")
	state := fs.String("state", "", "open, closed or all")
	typ := fs.String("type", "", "type tag, such as bug or crash")
	extraTag := fs.String("tag", "", "also require this tag slug, such as failure/nonfatal-operation")
	limit := fs.Int("limit", 50, "maximum issues")
	parseFlags(fs, args[1:])
	*state = validState(*state)

	d := openDB()
	defer d.Close()
	tag, err := query.ResolveTag(d, name, "area")
	if err != nil {
		die("%v", err)
	}
	var typeID int64
	if strings.TrimSpace(*typ) != "" {
		t, err := query.ResolveTag(d, *typ, "type")
		if err != nil {
			die("%v", err)
		}
		typeID = t.ID
	}
	rows, err := query.AreaIssues(d, tag.ID, *state, typeID, *extraTag, *limit)
	if err != nil {
		die("%v", err)
	}
	if len(rows) == 0 {
		fmt.Printf("No issues for %s.\n", tag.Slug)
		return
	}
	printIssues(rows, false)
}

func cmdSearch(args []string) {
	var words []string
	i := 0
	for i < len(args) && !strings.HasPrefix(args[i], "-") {
		words = append(words, args[i])
		i++
	}
	if len(words) == 0 {
		die("usage: triage search \"<text>\" [--state open|closed] [--repo name] [--limit N]")
	}
	text := strings.Join(words, " ")
	fs := flags("search")
	state := fs.String("state", "", "open, closed or all")
	repoFlag := fs.String("repo", "", "repository short name or owner/name")
	extraTag := fs.String("tag", "", "also require this tag slug")
	limit := fs.Int("limit", 50, "maximum issues")
	parseFlags(fs, args[i:])
	*state = validState(*state)

	d := openDB()
	defer d.Close()
	repo := repoOrEmpty(d, *repoFlag)
	rows, err := query.Search(d, text, repo, *state, *extraTag, *limit)
	if err != nil {
		die("%v", err)
	}
	if len(rows) == 0 {
		fmt.Println("No matches.")
		return
	}
	printIssues(rows, true)
}

func cmdIssue(args []string) {
	if len(args) == 0 {
		die("usage: triage issue <number|repo#number> [--repo name]")
	}
	spec := args[0]
	fs := flags("issue")
	repoFlag := fs.String("repo", "", "repository short name or owner/name")
	parseFlags(fs, args[1:])

	repo, number, err := query.ParseIssueSpec(spec)
	if err != nil {
		die("%v", err)
	}
	if *repoFlag != "" {
		repo = *repoFlag
	}
	d := openDB()
	defer d.Close()
	detail, err := query.FindIssue(d, repo, number)
	if err != nil {
		die("%v", err)
	}
	printDetail(detail)
}

func cmdRelated(args []string) {
	if len(args) == 0 {
		die("usage: triage related <number|repo#number> [--repo name]")
	}
	spec := args[0]
	fs := flags("related")
	repoFlag := fs.String("repo", "", "repository short name or owner/name")
	parseFlags(fs, args[1:])

	repo, number, err := query.ParseIssueSpec(spec)
	if err != nil {
		die("%v", err)
	}
	if *repoFlag != "" {
		repo = *repoFlag
	}
	d := openDB()
	defer d.Close()
	detail, err := query.FindIssue(d, repo, number)
	if err != nil {
		die("%v", err)
	}
	rows, err := query.Related(d, detail)
	if err != nil {
		die("%v", err)
	}
	fmt.Printf("Related to %s\n", query.FormatIssueSpec(detail.Repo, detail.Number))
	if detail.ClusterName != "" {
		fmt.Printf("Cluster: %s — %s\n", detail.ClusterName, detail.ClusterWhy)
	}
	if len(rows) == 0 {
		fmt.Println("No cross-references or cluster members yet.")
		return
	}
	printIssues(rows, false)
}

func cmdClusters(args []string) {
	fs := flags("clusters")
	top := fs.Int("top", 20, "maximum clusters")
	parseFlags(fs, args)
	d := openDB()
	defer d.Close()
	rows, err := query.Clusters(d, *top)
	if err != nil {
		die("%v", err)
	}
	if len(rows) == 0 {
		fmt.Println("No clusters yet. Phase 3 builds them.")
		return
	}
	for _, c := range rows {
		fmt.Printf("%s (%d issues, score %d)\n", c.Name, c.Members, c.Score)
		if c.Rationale != "" {
			fmt.Printf("  %s\n", oneLine(c.Rationale, 300))
		}
	}
}

func cmdTag(args []string, add bool) {
	if len(args) < 2 {
		if add {
			die("usage: triage tag <number|repo#number> <tag>")
		}
		die("usage: triage untag <number|repo#number> <tag>")
	}
	spec, tagName := args[0], args[1]
	repoSpec, number, err := query.ParseIssueSpec(spec)
	if err != nil {
		die("%v", err)
	}
	d := openDB()
	defer d.Close()
	issueID, repo, err := query.FindIssueID(d, repoSpec, number)
	if err != nil {
		die("%v", err)
	}
	if add {
		tag, err := query.ResolveOrCreateTag(d, tagName)
		if err != nil {
			die("%v", err)
		}
		if _, err := d.Exec(`INSERT INTO issue_tags(issue_id,tag_id,source,confidence,signal)
			VALUES(?,?,'manual',1.0,'manual')
			ON CONFLICT(issue_id,tag_id) DO UPDATE SET source='manual',confidence=1.0,signal='manual',updated_at=datetime('now')`,
			issueID, tag.ID); err != nil {
			die("%v", err)
		}
		if _, err := d.Exec(`INSERT INTO tag_overrides(issue_id,tag_id,action) VALUES(?,?,'add')
			ON CONFLICT(issue_id,tag_id) DO UPDATE SET action='add'`, issueID, tag.ID); err != nil {
			die("%v", err)
		}
		fmt.Printf("tagged %s with %s (%s, manual)\n", query.FormatIssueSpec(repo, number), tag.Slug, tag.Kind)
		return
	}
	tag, err := query.ResolveTag(d, tagName, "")
	if err != nil {
		die("%v", err)
	}
	if _, err := d.Exec(`DELETE FROM issue_tags WHERE issue_id=? AND tag_id=?`, issueID, tag.ID); err != nil {
		die("%v", err)
	}
	if _, err := d.Exec(`INSERT INTO tag_overrides(issue_id,tag_id,action) VALUES(?,?,'remove')
		ON CONFLICT(issue_id,tag_id) DO UPDATE SET action='remove'`, issueID, tag.ID); err != nil {
		die("%v", err)
	}
	fmt.Printf("untagged %s: %s (manual removal survives later tagging)\n", query.FormatIssueSpec(repo, number), tag.Slug)
}

// cmdSQL is a local, read-only escape hatch for analysis. It is not part of the
// agent query surface: query_only is set before the statement runs.
func cmdSQL(args []string) {
	query := strings.Join(args, " ")
	if strings.TrimSpace(query) == "" {
		die("usage: triage sql <select-or-with-statement>")
	}
	d := openDB()
	defer d.Close()
	if _, err := d.Exec(`PRAGMA query_only=ON`); err != nil {
		die("%v", err)
	}
	rows, err := d.Query(query)
	if err != nil {
		die("%v", err)
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		die("%v", err)
	}
	fmt.Println(strings.Join(cols, "\t"))
	vals := make([]any, len(cols))
	ptrs := make([]any, len(cols))
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	for rows.Next() {
		if err := rows.Scan(ptrs...); err != nil {
			die("%v", err)
		}
		out := make([]string, len(vals))
		for i, v := range vals {
			out[i] = sqlCell(v)
		}
		fmt.Println(strings.Join(out, "\t"))
	}
	if err := rows.Err(); err != nil {
		die("%v", err)
	}
}

func sqlCell(v any) string {
	var s string
	switch x := v.(type) {
	case nil:
		s = ""
	case []byte:
		s = string(x)
	case string:
		s = x
	default:
		s = fmt.Sprint(x)
	}
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\t", "\\t")
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\r", "")
	return s
}

func cmdRetag(args []string) {
	fs := flags("retag")
	dry := fs.Bool("dry-run", false, "compute tags and coverage without writing issue tags")
	all := fs.Bool("all", false, "reprocess every issue, not only new or changed ones")
	limit := fs.Int("limit", 0, "debug: stop after N issues")
	parseFlags(fs, args)
	if *limit < 0 {
		die("--limit must be zero or positive")
	}
	d := openDB()
	defer d.Close()
	report, err := taxonomy.Retag(d, *dry, *all, *limit)
	if err != nil {
		die("%v", err)
	}
	doc, err := taxonomy.Load()
	if err != nil {
		die("%v", err)
	}
	fmt.Print(report.Format(doc))
}

// cmdApplyTags reads issue<TAB>tag<TAB>confidence<TAB>signal from stdin. It is used for
// bulk classification passes (failure_kind) without exposing a general SQL write path.
func cmdApplyTags(args []string) {
	fs := flags("apply-tags")
	source := fs.String("source", "rule", "issue_tags.source")
	prefix := fs.String("prefix", "", "replace auto tags whose slug starts with this prefix")
	parseFlags(fs, args)
	d := openDB()
	defer d.Close()
	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	tx, err := d.Begin()
	if err != nil {
		die("%v", err)
	}
	insert, err := tx.Prepare(`INSERT INTO issue_tags(issue_id,tag_id,source,confidence,signal,created_at,updated_at)
		VALUES(?,?,?,?,?,?,?) ON CONFLICT(issue_id,tag_id) DO UPDATE SET source=excluded.source,confidence=excluded.confidence,signal=excluded.signal,updated_at=excluded.updated_at`)
	if err != nil {
		die("%v", err)
	}
	created := 0
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "issue\t") {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 4 {
			die("bad apply-tags line: %q", line)
		}
		repo, num, err := query.ParseIssueSpec(parts[0])
		if err != nil {
			die("%v", err)
		}
		issueID, err := findIssueTx(tx, repo, num)
		if err != nil {
			die("%v", err)
		}
		slug := parts[1]
		var conf float64
		if _, err := fmt.Sscanf(parts[2], "%f", &conf); err != nil {
			die("bad confidence %q", parts[2])
		}
		var tagID int64
		err = tx.QueryRow(`INSERT INTO tags(slug,name,kind) VALUES(?,?, 'custom')
			ON CONFLICT(slug) DO UPDATE SET name=excluded.name RETURNING id`, slug, tagName(slug)).Scan(&tagID)
		if err != nil {
			die("%v", err)
		}
		if *prefix != "" {
			if _, err := tx.Exec(`DELETE FROM issue_tags WHERE issue_id=? AND source!='manual' AND tag_id IN (
				SELECT id FROM tags WHERE slug LIKE ?)`, issueID, *prefix+"%"); err != nil {
				die("%v", err)
			}
		}
		now := db.Now()
		if _, err := insert.Exec(issueID, tagID, *source, conf, parts[3], now, now); err != nil {
			die("%v", err)
		}
		created++
	}
	if err := sc.Err(); err != nil {
		die("%v", err)
	}
	insert.Close()
	if err := tx.Commit(); err != nil {
		die("%v", err)
	}
	fmt.Printf("applied %d tags\n", created)
}

func findIssueTx(tx *sql.Tx, repo string, number int) (int64, error) {
	var id int64
	err := tx.QueryRow(`SELECT i.id FROM issues i JOIN repos r ON r.id=i.repo_id
		WHERE r.slug=? AND i.number=?`, repo, number).Scan(&id)
	return id, err
}

func tagName(slug string) string {
	if i := strings.LastIndexByte(slug, '/'); i >= 0 {
		slug = slug[i+1:]
	}
	slug = strings.ReplaceAll(slug, "-", " ")
	slug = strings.ReplaceAll(slug, "_", " ")
	return strings.Title(slug)
}

func cmdStats(args []string) {
	fs := flags("stats")
	parseFlags(fs, args)
	d := openDB()
	defer d.Close()
	out, err := query.Stats(d)
	if err != nil {
		die("%v", err)
	}
	fmt.Print(out)
}

func printIssues(rows []query.IssueRow, withSnippet bool) {
	for _, r := range rows {
		fmt.Printf("%s [%s] %s", query.FormatIssueSpec(r.Repo, r.Number), r.State, oneLine(r.Title, 180))
		if len(r.Tags) > 0 {
			fmt.Printf("  {%s}", strings.Join(r.Tags, ","))
		}
		if withSnippet && strings.TrimSpace(r.Snippet) != "" {
			fmt.Printf("  — %s", oneLine(r.Snippet, 220))
		}
		fmt.Println()
	}
}

func printDetail(d *query.IssueDetail) {
	fmt.Printf("%s [%s] %s\n", query.FormatIssueSpec(d.Repo, d.Number), d.State, d.Title)
	if d.HTMLURL != "" {
		fmt.Printf("URL: %s\n", d.HTMLURL)
	}
	fmt.Printf("Author: %s (%s)\n", d.Author, d.AuthorType)
	fmt.Printf("Created: %s\nUpdated: %s\n", d.CreatedAt, d.UpdatedAt)
	if d.ClosedAt != "" {
		fmt.Printf("Closed: %s\n", d.ClosedAt)
	}
	if len(d.Labels) > 0 {
		fmt.Printf("GitHub labels: %s\n", strings.Join(d.Labels, ", "))
	}
	printTags(d.Tags)
	if d.ClusterName != "" {
		fmt.Printf("Cluster: %s — %s\n", d.ClusterName, d.ClusterWhy)
	}
	fmt.Println("\nBody:")
	if strings.TrimSpace(d.Body) == "" {
		fmt.Println("(empty)")
	} else {
		fmt.Println(d.Body)
	}
	fmt.Printf("\nComments (%d):\n", len(d.Comments))
	for _, c := range d.Comments {
		fmt.Printf("\n--- %s at %s\n%s\n", c.Author, c.CreatedAt, c.Body)
	}
	if len(d.OutgoingRefs) > 0 {
		fmt.Println("\nReferences out:")
		for _, r := range d.OutgoingRefs {
			fmt.Printf("  -> %s %s: %s\n", query.FormatIssueSpec(r.TargetRepo, r.TargetNumber), r.Kind, oneLine(r.Context, 180))
		}
	}
	if len(d.IncomingRefs) > 0 {
		fmt.Println("\nReferences in:")
		for _, r := range d.IncomingRefs {
			fmt.Printf("  <- %s (%s) %s: %s\n", query.FormatIssueSpec(r.SourceRepo, r.SourceNumber), r.SourceKind, r.Kind, oneLine(r.Context, 180))
		}
	}
}

func printTags(tags []query.TagRow) {
	if len(tags) == 0 {
		fmt.Println("Tags: (none)")
		return
	}
	byKind := map[string][]string{}
	for _, t := range tags {
		label := t.Slug
		if t.Source != "manual" {
			label += "=" + t.Source
		}
		byKind[t.Kind] = append(byKind[t.Kind], label)
	}
	var kinds []string
	for k := range byKind {
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)
	for _, k := range kinds {
		sort.Strings(byKind[k])
		fmt.Printf("%s tags: %s\n", k, strings.Join(byKind[k], ", "))
	}
}

func oneLine(s string, maxLen int) string {
	s = strings.Join(strings.Fields(strings.ReplaceAll(strings.ReplaceAll(s, "\r", " "), "\n", " ")), " ")
	if maxLen > 0 && len(s) > maxLen {
		s = s[:maxLen] + "…"
	}
	return s
}
