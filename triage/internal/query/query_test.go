package query

import "testing"

func TestFTSQueryIsSafe(t *testing.T) {
	cases := map[string]string{
		"boolean":       `"boolean"`,
		"boolean ops":   `"boolean" AND "ops"`,
		"Boolean: ops!": `"boolean" AND "ops"`,
		"a b":           "",
		`he said "hi"`:  `"he" AND "said" AND "hi"`,
		"#123":          `"123"`,
	}
	for in, want := range cases {
		if got := FTSQuery(in); got != want {
			t.Errorf("FTSQuery(%q) = %q, want %q", in, got, want)
		}
	}
}
