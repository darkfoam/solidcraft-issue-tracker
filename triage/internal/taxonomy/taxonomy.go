package taxonomy

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

//go:embed taxonomy.json
var embedded []byte

type Area struct {
	Slug        string   `json:"slug"`
	Name        string   `json:"name"`
	Parent      string   `json:"parent"`
	Kind        string   `json:"kind"`
	Description string   `json:"description"`
	Aliases     []string `json:"aliases"`
	Labels      []string `json:"labels"`
	Prefixes    []string `json:"prefixes"`
	Paths       []string `json:"paths"`
	Symbols     []string `json:"symbols"`
	Patterns    []string `json:"patterns"`
}

type Doc struct {
	Version int    `json:"version"`
	Areas   []Area `json:"areas"`
}

func Load() (*Doc, error) {
	var doc Doc
	if err := json.Unmarshal(embedded, &doc); err != nil {
		return nil, fmt.Errorf("parse taxonomy: %w", err)
	}
	if err := doc.Validate(); err != nil {
		return nil, err
	}
	return &doc, nil
}

func (d *Doc) Validate() error {
	if d.Version <= 0 {
		return fmt.Errorf("taxonomy version must be positive")
	}
	bySlug := map[string]Area{}
	for _, a := range d.Areas {
		if a.Slug == "" || a.Name == "" {
			return fmt.Errorf("area with empty slug or name")
		}
		if _, ok := bySlug[a.Slug]; ok {
			return fmt.Errorf("duplicate area slug %q", a.Slug)
		}
		switch a.Kind {
		case "area", "type", "symptom":
		default:
			return fmt.Errorf("%s: invalid kind %q", a.Slug, a.Kind)
		}
		bySlug[a.Slug] = a
	}
	for _, a := range d.Areas {
		if a.Parent != "" {
			p, ok := bySlug[a.Parent]
			if !ok {
				return fmt.Errorf("%s: parent %q does not exist", a.Slug, a.Parent)
			}
			if p.Kind != "area" {
				return fmt.Errorf("%s: parent %q is not an area", a.Slug, a.Parent)
			}
		}
		for _, p := range a.Patterns {
			if _, err := regexp.Compile("(?i)" + p); err != nil {
				return fmt.Errorf("%s: bad pattern %q: %w", a.Slug, p, err)
			}
		}
	}
	// Aliases are stored with a unique index, so a collision is a hard error.
	seen := map[string]string{}
	for _, a := range d.Areas {
		for _, alias := range a.Aliases {
			k := strings.ToLower(strings.TrimSpace(alias))
			if k == "" {
				return fmt.Errorf("%s: empty alias", a.Slug)
			}
			if old, ok := seen[k]; ok && old != a.Slug {
				return fmt.Errorf("alias %q belongs to both %s and %s", alias, old, a.Slug)
			}
			seen[k] = a.Slug
		}
	}
	return nil
}

func (d *Doc) Area(slug string) (Area, bool) {
	for _, a := range d.Areas {
		if a.Slug == slug {
			return a, true
		}
	}
	return Area{}, false
}
