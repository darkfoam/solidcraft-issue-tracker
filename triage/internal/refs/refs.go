package refs

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

// Ref is one cross-reference found in issue, comment or pull-request text.
type Ref struct {
	TargetRepo   string
	TargetNumber int
	Kind         string
	Context      string
	Raw          string
}

var (
	duplicateRe = regexp.MustCompile(`(?i)\bduplicate\s+of\s*(?:#|https?://github\.com/([A-Za-z0-9_.-]+)/([A-Za-z0-9_.-]+)/(?:issues|pull)/)(\d+)`)
	keywordRe   = regexp.MustCompile(`(?i)\b(fix(?:es|ed|s)?|clos(?:es|ed|e)?|resolv(?:es|ed|e)?)\s*:?\s*(?:#|https?://github\.com/([A-Za-z0-9_.-]+)/([A-Za-z0-9_.-]+)/(?:issues|pull)/)(\d+)`)
	urlRe       = regexp.MustCompile(`https?://github\.com/([A-Za-z0-9_.-]+)/([A-Za-z0-9_.-]+)/(issues|pull)/(\d+)`)
	crossRepoRe = regexp.MustCompile(`\b([A-Za-z0-9_.-]+)/([A-Za-z0-9_.-]+)#(\d+)\b`)
	hashRe      = regexp.MustCompile(`(^|[^A-Za-z0-9_/])#(\d+)\b`)
)

// Extract finds cross-references in text. defaultRepo is owner/name; self is the
// issue or PR number the text belongs to and is skipped as a self-reference.
func Extract(text, defaultRepo string, self int) []Ref {
	defaultRepo = normalizeRepo(defaultRepo)
	byTarget := map[string]Ref{}

	add := func(r Ref) {
		if r.TargetNumber <= 0 {
			return
		}
		if r.TargetRepo == "" {
			r.TargetRepo = defaultRepo
		}
		r.TargetRepo = normalizeRepo(r.TargetRepo)
		if r.TargetRepo == defaultRepo && r.TargetNumber == self {
			return
		}
		key := fmt.Sprintf("%s#%d", r.TargetRepo, r.TargetNumber)
		if old, ok := byTarget[key]; ok && kindRank(old.Kind) >= kindRank(r.Kind) {
			return
		}
		byTarget[key] = r
	}

	for _, m := range duplicateRe.FindAllStringSubmatchIndex(text, -1) {
		target, raw, ctx := matchTarget(text, m, 1, 2, defaultRepo)
		add(Ref{TargetRepo: target, TargetNumber: atoi(text, m[6], m[7]), Kind: "duplicate_of", Context: ctx, Raw: raw})
	}
	for _, m := range keywordRe.FindAllStringSubmatchIndex(text, -1) {
		verb := strings.ToLower(text[m[2]:m[3]])
		kind := "fixes"
		switch {
		case strings.HasPrefix(verb, "clos"):
			kind = "closes"
		case strings.HasPrefix(verb, "resolv"):
			kind = "resolves"
		}
		target, raw, ctx := matchTarget(text, m, 2, 3, defaultRepo)
		add(Ref{TargetRepo: target, TargetNumber: atoi(text, m[8], m[9]), Kind: kind, Context: ctx, Raw: raw})
	}
	for _, m := range urlRe.FindAllStringSubmatchIndex(text, -1) {
		owner := text[m[2]:m[3]]
		repo := text[m[4]:m[5]]
		kind := "mention"
		if text[m[6]:m[7]] == "pull" {
			kind = "linked_pr"
		}
		add(Ref{TargetRepo: owner + "/" + repo, TargetNumber: atoi(text, m[8], m[9]), Kind: kind, Context: contextAround(text, m[0], m[1]), Raw: text[m[0]:m[1]]})
	}
	for _, m := range crossRepoRe.FindAllStringSubmatchIndex(text, -1) {
		owner := text[m[2]:m[3]]
		repo := text[m[4]:m[5]]
		add(Ref{TargetRepo: owner + "/" + repo, TargetNumber: atoi(text, m[6], m[7]), Kind: "mention", Context: contextAround(text, m[0], m[1]), Raw: text[m[0]:m[1]]})
	}
	for _, m := range hashRe.FindAllStringSubmatchIndex(text, -1) {
		add(Ref{TargetRepo: defaultRepo, TargetNumber: atoi(text, m[4], m[5]), Kind: "mention", Context: contextAround(text, m[0], m[1]), Raw: text[m[0]:m[1]]})
	}

	out := make([]Ref, 0, len(byTarget))
	for _, r := range byTarget {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].TargetRepo != out[j].TargetRepo {
			return out[i].TargetRepo < out[j].TargetRepo
		}
		return out[i].TargetNumber < out[j].TargetNumber
	})
	return out
}

func normalizeRepo(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

func kindRank(kind string) int {
	switch kind {
	case "duplicate_of":
		return 5
	case "fixes", "closes", "resolves":
		return 4
	case "linked_pr":
		return 3
	case "mention":
		return 1
	}
	return 0
}

func atoi(s string, start, end int) int {
	n := 0
	for _, c := range s[start:end] {
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int(c-'0')
		if n > 1<<31 {
			return 0
		}
	}
	return n
}

// matchTarget pulls owner/repo from optional groups, falling back to defaultRepo.
func matchTarget(text string, m []int, ownerGroup, repoGroup int, defaultRepo string) (string, string, string) {
	repo := defaultRepo
	oi, ri := 2*ownerGroup, 2*repoGroup
	if oi+1 < len(m) && ri+1 < len(m) && m[oi] >= 0 && m[ri] >= 0 {
		repo = text[m[oi]:m[oi+1]] + "/" + text[m[ri]:m[ri+1]]
	}
	return repo, text[m[0]:m[1]], contextAround(text, m[0], m[1])
}

func contextAround(text string, start, end int) string {
	const width = 90
	if start < 0 || end > len(text) || start >= end {
		return ""
	}
	runes := []rune(text)
	startRune := utf8.RuneCountInString(text[:start])
	endRune := utf8.RuneCountInString(text[:end])
	lo := max(0, startRune-width)
	hi := min(len(runes), endRune+width)
	s := strings.TrimSpace(string(runes[lo:hi]))
	s = strings.Join(strings.Fields(s), " ")
	if lo > 0 {
		s = "…" + s
	}
	if hi < len(runes) {
		s += "…"
	}
	return s
}
