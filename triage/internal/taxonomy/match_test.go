package taxonomy

import "testing"

func TestLoadIsValid(t *testing.T) {
	doc, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Areas) < 20 {
		t.Fatalf("only %d areas loaded", len(doc.Areas))
	}
	if _, ok := doc.Area("kernel/booleans"); !ok {
		t.Fatal("kernel/booleans missing")
	}
}

func TestMatcherFindsOperationAreasAndParents(t *testing.T) {
	doc, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	m, err := NewMatcher(doc)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		title string
		body  string
		want  string
	}{
		{"FILLET: operation failed", "", "kernel/blends"},
		{"COMBINE: operation failed", "boolean Cut: no result", "kernel/booleans"},
		{"EXTRUDE: operation failed", "", "kernel/sweeps"},
		{"SHELL: operation failed", "", "kernel/shell-offset"},
		{"CREATE SKETCH: operation failed", "", "sketch"},
		{"Sketch dimension is locked", "", "sketch/dimensions"},
		{"STEP import loses colours", "", "io/step-iges"},
		{"Render workspace is blank", "", "render/workspace"},
		{"Windows 11 install fails", "", "platform/desktop"},
	}
	for _, tc := range cases {
		got := map[string]bool{}
		for _, m := range m.Match(tc.title, tc.body, nil) {
			got[m.Slug] = true
		}
		if !got[tc.want] {
			t.Errorf("%q: missing %s; got %v", tc.title, tc.want, got)
		}
	}
}

func TestMatcherPropagatesParent(t *testing.T) {
	doc, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	m, err := NewMatcher(doc)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, x := range m.Match("FILLET: operation failed", "", nil) {
		got[x.Slug] = true
	}
	if !got["kernel"] {
		t.Fatalf("kernel parent was not added: %v", got)
	}
}
