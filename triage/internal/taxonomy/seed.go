package taxonomy

import (
	"database/sql"
	"fmt"

	"github.com/nclack/solidcraft-triage/internal/db"
)

// Seed stores the embedded taxonomy in the database and returns the slug -> tag id map.
func Seed(d *db.DB) (*Doc, map[string]int64, error) {
	doc, err := Load()
	if err != nil {
		return nil, nil, err
	}
	tx, err := d.Begin()
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback()

	ids := map[string]int64{}
	for _, a := range doc.Areas {
		var id int64
		if err := tx.QueryRow(`
			INSERT INTO tags(slug,name,kind,description,updated_at)
			VALUES(?,?,?,?,?)
			ON CONFLICT(slug) DO UPDATE SET
				name=excluded.name, kind=excluded.kind,
				description=excluded.description, updated_at=excluded.updated_at
			RETURNING id`,
			a.Slug, a.Name, a.Kind, a.Description, db.Now()).Scan(&id); err != nil {
			return nil, nil, fmt.Errorf("seed tag %s: %w", a.Slug, err)
		}
		ids[a.Slug] = id

		if _, err := tx.Exec(`DELETE FROM tag_aliases WHERE tag_id=?`, id); err != nil {
			return nil, nil, err
		}
		for _, alias := range a.Aliases {
			if _, err := tx.Exec(`INSERT INTO tag_aliases(tag_id,alias) VALUES(?,?)
				ON CONFLICT(alias) DO NOTHING`, id, alias); err != nil {
				return nil, nil, fmt.Errorf("seed alias %q: %w", alias, err)
			}
		}

		if _, err := tx.Exec(`DELETE FROM tag_paths WHERE tag_id=?`, id); err != nil {
			return nil, nil, err
		}
		for _, p := range a.Paths {
			if _, err := tx.Exec(`INSERT INTO tag_paths(tag_id,glob) VALUES(?,?)
				ON CONFLICT(tag_id,glob) DO NOTHING`, id, p); err != nil {
				return nil, nil, fmt.Errorf("seed path %q: %w", p, err)
			}
		}

		if _, err := tx.Exec(`DELETE FROM tag_symbols WHERE tag_id=?`, id); err != nil {
			return nil, nil, err
		}
		for _, s := range a.Symbols {
			if _, err := tx.Exec(`INSERT INTO tag_symbols(tag_id,symbol) VALUES(?,?)
				ON CONFLICT(tag_id,symbol) DO NOTHING`, id, s); err != nil {
				return nil, nil, fmt.Errorf("seed symbol %q: %w", s, err)
			}
		}
	}

	for _, a := range doc.Areas {
		if a.Parent == "" {
			if _, err := tx.Exec(`UPDATE tags SET parent_id=NULL WHERE slug=?`, a.Slug); err != nil {
				return nil, nil, err
			}
			continue
		}
		parentID, ok := ids[a.Parent]
		if !ok {
			return nil, nil, fmt.Errorf("%s: parent %s was not seeded", a.Slug, a.Parent)
		}
		if _, err := tx.Exec(`UPDATE tags SET parent_id=? WHERE id=?`, parentID, ids[a.Slug]); err != nil {
			return nil, nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, nil, err
	}
	return doc, ids, nil
}

func tagID(tx *sql.Tx, slug string) (int64, error) {
	var id int64
	err := tx.QueryRow(`SELECT id FROM tags WHERE slug=?`, slug).Scan(&id)
	return id, err
}
