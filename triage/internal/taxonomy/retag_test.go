package taxonomy

import (
	"path/filepath"
	"testing"

	"github.com/nclack/solidcraft-triage/internal/db"
)

func TestRetagPreservesManualOverrides(t *testing.T) {
	d, err := db.Open(filepath.Join(t.TempDir(), "triage.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	repoID, err := d.EnsureRepo("tangency-labs/solidcraft")
	if err != nil {
		t.Fatal(err)
	}
	var issueID int64
	if err := d.QueryRow(`INSERT INTO issues(repo_id,number,title,body,state,raw_json)
		VALUES(?,?,?,?,'open','{}') RETURNING id`,
		repoID, 1, "COMBINE: operation failed", "boolean Cut: no result").Scan(&issueID); err != nil {
		t.Fatal(err)
	}
	if _, err := Retag(d, false, true, 0); err != nil {
		t.Fatal(err)
	}

	var n int
	if err := d.QueryRow(`SELECT count(*) FROM issue_tags it JOIN tags t ON t.id=it.tag_id
		WHERE it.issue_id=? AND t.slug='kernel/booleans' AND it.source!='manual'`, issueID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("rule tag missing: %d", n)
	}

	// A manual tag and a manual removal must both survive a full re-tag.
	var customID int64
	if err := d.QueryRow(`INSERT INTO tags(slug,name,kind) VALUES('custom/keep','Keep','custom')
		ON CONFLICT(slug) DO UPDATE SET name=excluded.name RETURNING id`).Scan(&customID); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Exec(`INSERT INTO issue_tags(issue_id,tag_id,source,confidence,signal)
		VALUES(?,?,'manual',1.0,'manual')`, issueID, customID); err != nil {
		t.Fatal(err)
	}
	var boolID int64
	if err := d.QueryRow(`SELECT id FROM tags WHERE slug='kernel/booleans'`).Scan(&boolID); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Exec(`DELETE FROM issue_tags WHERE issue_id=? AND tag_id=?`, issueID, boolID); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Exec(`INSERT INTO tag_overrides(issue_id,tag_id,action) VALUES(?,?,'remove')`, issueID, boolID); err != nil {
		t.Fatal(err)
	}

	if _, err := Retag(d, false, true, 0); err != nil {
		t.Fatal(err)
	}
	d.QueryRow(`SELECT count(*) FROM issue_tags WHERE issue_id=? AND tag_id=?`, issueID, customID).Scan(&n)
	if n != 1 {
		t.Fatalf("manual tag was lost")
	}
	d.QueryRow(`SELECT count(*) FROM issue_tags WHERE issue_id=? AND tag_id=?`, issueID, boolID).Scan(&n)
	if n != 0 {
		t.Fatalf("manually removed tag was re-added")
	}
	var action string
	d.QueryRow(`SELECT action FROM tag_overrides WHERE issue_id=? AND tag_id=?`, issueID, boolID).Scan(&action)
	if action != "remove" {
		t.Fatalf("manual removal override was lost: %q", action)
	}
}
