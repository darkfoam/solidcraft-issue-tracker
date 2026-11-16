package taxonomy

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/nclack/solidcraft-triage/internal/db"
)

type Count struct {
	Open  int
	Total int
}

type Report struct {
	Version       int
	DryRun        bool
	Considered    int
	Tagged        int
	Untagged      int
	OpenUntagged  int
	Areas         map[string]Count
	Types         map[string]int
	Symptoms      map[string]int
	ScannedIssues int
}

func (r *Report) Format(doc *Doc) string {
	var b strings.Builder
	mode := "rules"
	if r.DryRun {
		mode += " (dry run; no issue tags written)"
	}
	fmt.Fprintf(&b, "retag %s v%d\n", mode, r.Version)
	fmt.Fprintf(&b, "considered: %d\n", r.Considered)
	pct := 0.0
	if r.Considered > 0 {
		pct = 100 * float64(r.Tagged) / float64(r.Considered)
	}
	fmt.Fprintf(&b, "tagged: %d (%.1f%%)\nuntagged: %d\nuntagged open: %d\n\n", r.Tagged, pct, r.Untagged, r.OpenUntagged)
	b.WriteString("areas:\n")
	roots, children := areaTree(doc)
	var walk func(a Area, depth int)
	walk = func(a Area, depth int) {
		c := r.Areas[a.Slug]
		count := fmt.Sprintf("%d", c.Open)
		if c.Total != c.Open {
			count = fmt.Sprintf("%d/%d", c.Open, c.Total)
		}
		fmt.Fprintf(&b, "%s%s (%s) [%s]\n", strings.Repeat("  ", depth), a.Name, count, a.Slug)
		for _, ch := range children[a.Slug] {
			walk(ch, depth+1)
		}
	}
	for _, a := range roots {
		walk(a, 0)
	}
	b.WriteString("\ntypes:\n")
	for _, a := range typed(doc, "type") {
		fmt.Fprintf(&b, "  %s (%d) [%s]\n", a.Name, r.Types[a.Slug], a.Slug)
	}
	b.WriteString("\nsymptoms:\n")
	for _, a := range typed(doc, "symptom") {
		fmt.Fprintf(&b, "  %s (%d) [%s]\n", a.Name, r.Symptoms[a.Slug], a.Slug)
	}
	return b.String()
}

func areaTree(doc *Doc) ([]Area, map[string][]Area) {
	bySlug := map[string]Area{}
	for _, a := range doc.Areas {
		bySlug[a.Slug] = a
	}
	children := map[string][]Area{}
	var roots []Area
	for _, a := range doc.Areas {
		if a.Kind != "area" {
			continue
		}
		if a.Parent != "" {
			if _, ok := bySlug[a.Parent]; ok {
				children[a.Parent] = append(children[a.Parent], a)
				continue
			}
		}
		roots = append(roots, a)
	}
	return roots, children
}

func typed(doc *Doc, kind string) []Area {
	var out []Area
	for _, a := range doc.Areas {
		if a.Kind == kind {
			out = append(out, a)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

type issueText struct {
	ID       int64
	Title    string
	Body     string
	State    string
	Repo     string
	Number   int
	Comments string
	Labels   string
}

// Retag applies deterministic taxonomy rules. DryRun computes the same matches
// but writes only the taxonomy itself, never issue_tags.
func Retag(d *db.DB, dryRun, all bool, limit int) (*Report, error) {
	doc, ids, err := Seed(d)
	if err != nil {
		return nil, err
	}
	matcher, err := NewMatcher(doc)
	if err != nil {
		return nil, err
	}
	report := &Report{Version: doc.Version, DryRun: dryRun,
		Areas: map[string]Count{}, Types: map[string]int{}, Symptoms: map[string]int{}}

	if dryRun {
		if err := retagDryRun(d, matcher, doc, report, all, limit); err != nil {
			return nil, err
		}
		return report, nil
	}

	if err := retagWrite(d, matcher, ids, doc, report, all, limit); err != nil {
		return nil, err
	}
	if err := fillReportFromDB(d, doc, report); err != nil {
		return nil, err
	}
	return report, nil
}

func retagDryRun(d *db.DB, matcher *Matcher, doc *Doc, report *Report, all bool, limit int) error {
	last := int64(0)
	scanned := 0
	openUntagged := 0
	for {
		page, err := loadIssuePage(d, doc, last, all, pageSize(limit, scanned))
		if err != nil {
			return err
		}
		if len(page) == 0 {
			break
		}
		for _, issue := range page {
			last = issue.ID
			scanned++
			matches := matcher.Match(issue.Title, issue.Body+"\n"+issue.Comments, strings.Split(issue.Labels, "\t"))
			tagged := false
			for _, m := range matches {
				if m.Confidence < 0.80 {
					continue
				}
				switch matcher.Kind(m.Slug) {
				case "area":
					tagged = true
					c := report.Areas[m.Slug]
					c.Total++
					if issue.State == "open" {
						c.Open++
					}
					report.Areas[m.Slug] = c
				case "type":
					report.Types[m.Slug]++
				case "symptom":
					report.Symptoms[m.Slug]++
				}
			}
			if tagged {
				report.Tagged++
			} else if issue.State == "open" {
				openUntagged++
			}
			if limit > 0 && scanned >= limit {
				break
			}
		}
		if limit > 0 && scanned >= limit {
			break
		}
	}
	report.Considered = scanned
	report.ScannedIssues = scanned
	report.Untagged = report.Considered - report.Tagged
	report.OpenUntagged = openUntagged
	return nil
}

func pageSize(limit, scanned int) int {
	if limit <= 0 {
		return 400
	}
	n := limit - scanned
	if n > 400 {
		n = 400
	}
	if n < 1 {
		return 1
	}
	return n
}

func retagWrite(d *db.DB, matcher *Matcher, ids map[string]int64, doc *Doc, report *Report, all bool, limit int) error {
	last := int64(0)
	scanned := 0
	for {
		page, err := loadIssuePage(d, doc, last, all, pageSize(limit, scanned))
		if err != nil {
			return err
		}
		if len(page) == 0 {
			break
		}
		tx, err := d.Begin()
		if err != nil {
			return err
		}
		deleteStmt, err := tx.Prepare(`DELETE FROM issue_tags WHERE issue_id=? AND source!='manual'`)
		if err != nil {
			tx.Rollback()
			return err
		}
		overrideStmt, err := tx.Prepare(`SELECT action FROM tag_overrides WHERE issue_id=? AND tag_id=?`)
		if err != nil {
			tx.Rollback()
			return err
		}
		insertStmt, err := tx.Prepare(`INSERT INTO issue_tags(issue_id,tag_id,source,confidence,signal,created_at,updated_at)
			VALUES(?,?,?,?,?,?,?) ON CONFLICT(issue_id,tag_id) DO NOTHING`)
		if err != nil {
			tx.Rollback()
			return err
		}
		manualStmt, err := tx.Prepare(`INSERT INTO issue_tags(issue_id,tag_id,source,confidence,signal,created_at,updated_at)
			VALUES(?,?,'manual',1.0,'manual',?,?) ON CONFLICT(issue_id,tag_id) DO NOTHING`)
		if err != nil {
			tx.Rollback()
			return err
		}
		checkStmt, err := tx.Prepare(`INSERT INTO issue_checks(issue_id,method,version,processed_at,note)
			VALUES(?,'rules',?,?, '') ON CONFLICT(issue_id,method,version) DO UPDATE SET processed_at=excluded.processed_at`)
		if err != nil {
			tx.Rollback()
			return err
		}
		now := time.Now().UTC().Format(time.RFC3339)
		for _, issue := range page {
			last = issue.ID
			scanned++
			if _, err := deleteStmt.Exec(issue.ID); err != nil {
				tx.Rollback()
				return err
			}
			matches := matcher.Match(issue.Title, issue.Body+"\n"+issue.Comments, strings.Split(issue.Labels, "\t"))
			for _, m := range matches {
				if m.Confidence < 0.80 {
					continue
				}
				id, ok := ids[m.Slug]
				if !ok {
					continue
				}
				var action string
				err := overrideStmt.QueryRow(issue.ID, id).Scan(&action)
				if err == nil && action == "remove" {
					continue
				}
				if err != nil && err != sql.ErrNoRows {
					tx.Rollback()
					return err
				}
				if action == "add" {
					if _, err := manualStmt.Exec(issue.ID, id, now, now); err != nil {
						tx.Rollback()
						return err
					}
				}
				if _, err := insertStmt.Exec(issue.ID, id, m.Source, m.Confidence, m.Signal, now, now); err != nil {
					tx.Rollback()
					return err
				}
			}
			if _, err := checkStmt.Exec(issue.ID, doc.Version, now); err != nil {
				tx.Rollback()
				return err
			}
			if limit > 0 && scanned >= limit {
				break
			}
		}
		deleteStmt.Close()
		overrideStmt.Close()
		insertStmt.Close()
		manualStmt.Close()
		checkStmt.Close()
		if err := tx.Commit(); err != nil {
			return err
		}
		if limit > 0 && scanned >= limit {
			break
		}
	}
	report.ScannedIssues = scanned
	return nil
}

func loadIssuePage(d *db.DB, doc *Doc, lastID int64, all bool, pageSize int) ([]issueText, error) {
	if pageSize <= 0 {
		return nil, nil
	}
	q := `
		SELECT i.id,i.title,i.body,i.state,r.slug,i.number,
		       COALESCE((SELECT group_concat(c.body,char(10)) FROM comments c WHERE c.issue_id=i.id),''),
		       COALESCE((SELECT group_concat(l.name,char(9)) FROM issue_labels il JOIN labels l ON l.id=il.label_id WHERE il.issue_id=i.id),''),
		       COALESCE(ic.processed_at,'')
		FROM issues i
		JOIN repos r ON r.id=i.repo_id
		LEFT JOIN issue_checks ic ON ic.issue_id=i.id AND ic.method='rules' AND ic.version=?
		WHERE i.id > ?`
	args := []any{doc.Version, lastID}
	if !all {
		q += ` AND (ic.issue_id IS NULL OR i.updated_at > ic.processed_at)`
	}
	q += ` ORDER BY i.id LIMIT ?`
	args = append(args, pageSize)
	rows, err := d.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []issueText
	for rows.Next() {
		var it issueText
		var processed string
		if err := rows.Scan(&it.ID, &it.Title, &it.Body, &it.State, &it.Repo, &it.Number, &it.Comments, &it.Labels, &processed); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

func fillReportFromDB(d *db.DB, doc *Doc, report *Report) error {
	var total, tagged, openUntagged int
	if err := d.QueryRow(`SELECT count(*) FROM issues`).Scan(&total); err != nil {
		return err
	}
	if err := d.QueryRow(`SELECT count(DISTINCT it.issue_id) FROM issue_tags it JOIN tags t ON t.id=it.tag_id WHERE t.kind='area'`).Scan(&tagged); err != nil {
		return err
	}
	if err := d.QueryRow(`SELECT count(*) FROM issues i WHERE i.state='open' AND NOT EXISTS (
		SELECT 1 FROM issue_tags it JOIN tags t ON t.id=it.tag_id WHERE it.issue_id=i.id AND t.kind='area')`).Scan(&openUntagged); err != nil {
		return err
	}
	report.Considered = total
	report.Tagged = tagged
	report.Untagged = total - tagged
	report.OpenUntagged = openUntagged

	rows, err := d.Query(`SELECT t.slug, count(DISTINCT it.issue_id),
		COALESCE(SUM(CASE WHEN i.state='open' THEN 1 ELSE 0 END),0)
		FROM tags t JOIN issue_tags it ON it.tag_id=t.id JOIN issues i ON i.id=it.issue_id
		WHERE t.kind='area' GROUP BY t.id`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var slug string
		var c Count
		if err := rows.Scan(&slug, &c.Total, &c.Open); err != nil {
			return err
		}
		report.Areas[slug] = c
	}
	if err := rows.Err(); err != nil {
		return err
	}
	rows.Close()

	rows2, err := d.Query(`SELECT t.kind,t.slug,count(DISTINCT it.issue_id)
		FROM tags t JOIN issue_tags it ON it.tag_id=t.id
		WHERE t.kind IN ('type','symptom') GROUP BY t.kind,t.id`)
	if err != nil {
		return err
	}
	defer rows2.Close()
	for rows2.Next() {
		var kind, slug string
		var n int
		if err := rows2.Scan(&kind, &slug, &n); err != nil {
			return err
		}
		if kind == "type" {
			report.Types[slug] = n
		} else {
			report.Symptoms[slug] = n
		}
	}
	return rows2.Err()
}
