package refs

import "testing"

func TestExtractKindsAndRepos(t *testing.T) {
	text := `Fixes #12. Duplicate of #34.
Fixes https://github.com/other/repo/issues/11.
See https://github.com/tangency-labs/solidcraft/pull/56 and other/repo#78.
A plain mention #90 and a self reference #7.`
	got := Extract(text, "tangency-labs/solidcraft", 7)
	byKey := map[string]string{}
	for _, r := range got {
		byKey[r.TargetRepo+"#"+itoa(r.TargetNumber)] = r.Kind
	}
	cases := map[string]string{
		"tangency-labs/solidcraft#12": "fixes",
		"tangency-labs/solidcraft#34": "duplicate_of",
		"tangency-labs/solidcraft#56": "linked_pr",
		"other/repo#11":               "fixes",
		"other/repo#78":               "mention",
		"tangency-labs/solidcraft#90": "mention",
	}
	for key, want := range cases {
		if byKey[key] != want {
			t.Errorf("%s: got kind %q, want %q", key, byKey[key], want)
		}
	}
	if _, ok := byKey["tangency-labs/solidcraft#7"]; ok {
		t.Errorf("self reference should be skipped")
	}
}

func TestExtractDoesNotTreatCrossRepoAsTwoRefs(t *testing.T) {
	got := Extract("see other/repo#9", "mine/repo", 1)
	if len(got) != 1 {
		t.Fatalf("got %d refs, want 1: %+v", len(got), got)
	}
	if got[0].TargetRepo != "other/repo" || got[0].TargetNumber != 9 {
		t.Fatalf("got %+v", got[0])
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
