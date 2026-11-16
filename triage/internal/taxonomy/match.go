package taxonomy

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

type Match struct {
	Slug       string
	Source     string
	Signal     string
	Confidence float64
}

type compiledArea struct {
	area     Area
	patterns []*regexp.Regexp
	paths    []*regexp.Regexp
	symbols  []*regexp.Regexp
	labels   map[string]bool
	prefixes []string
}

type Matcher struct {
	doc    *Doc
	areas  []compiledArea
	parent map[string]string
	kind   map[string]string
}

func NewMatcher(doc *Doc) (*Matcher, error) {
	m := &Matcher{doc: doc, parent: map[string]string{}, kind: map[string]string{}}
	for _, a := range doc.Areas {
		m.parent[a.Slug] = a.Parent
		m.kind[a.Slug] = a.Kind
		ca := compiledArea{area: a, labels: map[string]bool{}}
		for _, label := range a.Labels {
			ca.labels[strings.ToLower(label)] = true
		}
		for _, p := range a.Prefixes {
			ca.prefixes = append(ca.prefixes, strings.ToUpper(strings.TrimSpace(p)))
		}
		for _, p := range a.Patterns {
			re, err := regexp.Compile("(?i)" + p)
			if err != nil {
				return nil, fmt.Errorf("%s: pattern %q: %w", a.Slug, p, err)
			}
			ca.patterns = append(ca.patterns, re)
		}
		for _, p := range a.Paths {
			re, err := regexp.Compile("(?i)" + globToRegex(p))
			if err != nil {
				return nil, fmt.Errorf("%s: path %q: %w", a.Slug, p, err)
			}
			ca.paths = append(ca.paths, re)
		}
		for _, s := range a.Symbols {
			re, err := regexp.Compile("(?i)" + symbolRegex(s))
			if err != nil {
				return nil, fmt.Errorf("%s: symbol %q: %w", a.Slug, s, err)
			}
			ca.symbols = append(ca.symbols, re)
		}
		m.areas = append(m.areas, ca)
	}
	return m, nil
}

// Match returns the best area tag for each matched slug.
func (m *Matcher) Match(title, body string, labels []string) []Match {
	text := title + "\n" + body
	labelSet := map[string]bool{}
	for _, l := range labels {
		for _, part := range strings.Split(l, "\t") {
			labelSet[strings.ToLower(strings.TrimSpace(part))] = true
		}
	}
	best := map[string]Match{}
	add := func(slug, source, signal string, confidence float64) {
		old, ok := best[slug]
		if !ok || confidence > old.Confidence {
			best[slug] = Match{Slug: slug, Source: source, Signal: signal, Confidence: confidence}
		}
	}
	prefix := titlePrefix(title)
	for _, ca := range m.areas {
		a := ca.area
		for _, label := range a.Labels {
			if labelSet[strings.ToLower(label)] {
				add(a.Slug, "label", "label:"+label, 0.95)
			}
		}
		if prefix != "" {
			for _, p := range ca.prefixes {
				if prefix == p || strings.HasPrefix(prefix, p+" ") || strings.HasPrefix(prefix, p+":") {
					add(a.Slug, "rule", "prefix:"+p, 0.95)
				}
			}
		}
		for i, re := range ca.paths {
			if re.MatchString(text) {
				add(a.Slug, "path", "path:"+a.Paths[i], 0.85)
			}
		}
		for i, re := range ca.symbols {
			if re.MatchString(text) {
				add(a.Slug, "symbol", "symbol:"+a.Symbols[i], 0.85)
			}
		}
		for _, re := range ca.patterns {
			if re.MatchString(title) {
				add(a.Slug, "rule", "title:"+re.String(), 0.90)
			} else if re.MatchString(body) {
				add(a.Slug, "keyword", "keyword:"+re.String(), 0.65)
			}
		}
	}

	// A child area also tags its parents, so "kernel" is a useful umbrella.
	// Weak body-only matches do not propagate: otherwise one incidental keyword
	// would tag every parent in the hierarchy.
	for slug, match := range best {
		if match.Confidence < 0.80 {
			continue
		}
		for p := m.parent[slug]; p != ""; p = m.parent[p] {
			if old, ok := best[p]; !ok || old.Confidence < 0.80 {
				best[p] = Match{Slug: p, Source: "rule", Signal: "child:" + slug, Confidence: 0.80}
			}
		}
	}

	out := make([]Match, 0, len(best))
	for _, v := range best {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Confidence != out[j].Confidence {
			return out[i].Confidence > out[j].Confidence
		}
		return out[i].Slug < out[j].Slug
	})
	return out
}

func (m *Matcher) Kind(slug string) string { return m.kind[slug] }

func titlePrefix(title string) string {
	t := strings.TrimSpace(title)
	if i := strings.IndexByte(t, ':'); i > 0 {
		t = t[:i]
	} else {
		return ""
	}
	t = strings.TrimSpace(t)
	t = strings.ToUpper(t)
	if len(t) > 60 {
		return ""
	}
	return t
}

func globToRegex(glob string) string {
	var b strings.Builder
	for _, r := range glob {
		switch r {
		case '*':
			b.WriteString(".*")
		case '?':
			b.WriteByte('.')
		default:
			if unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune("_-./", r) {
				b.WriteRune(r)
			} else {
				b.WriteString(regexp.QuoteMeta(string(r)))
			}
		}
	}
	return b.String()
}

func symbolRegex(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return `(?!)`
	}
	esc := regexp.QuoteMeta(s)
	if isWordEdge(s[0]) {
		esc = `\b` + esc
	}
	if isWordEdge(s[len(s)-1]) {
		esc += `\b`
	}
	return esc
}

func isWordEdge(b byte) bool {
	return b == '_' || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9'
}
