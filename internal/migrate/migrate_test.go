package migrate

import (
	"strings"
	"testing"
)

func TestBuildStitchMetaFromGraphite(t *testing.T) {
	rows := []graphiteRow{
		{Branch: "b1", Parent: "main", ParentRev: strings.Repeat("a", 40)},
		{Branch: "b2", Parent: "b1", ParentRev: ""},                          // no rev -> merge-base
		{Branch: "b3", Parent: "b2", ParentRev: "not-a-sha"},                 // junk rev -> merge-base
		{Branch: "gone", Parent: "main", ParentRev: strings.Repeat("b", 40)}, // not local -> skipped
	}
	exists := func(b string) bool { return b == "b1" || b == "b2" || b == "b3" }
	mb := func(a, p string) (string, error) { return "MB_" + a, nil }
	prs := map[string]int{"b1": 143}

	metas, skipped := buildStitchMetaFromGraphite(rows, prs, exists, mb)

	if len(metas) != 3 {
		t.Fatalf("want 3 migrated, got %d", len(metas))
	}
	if m := metas["b1"]; m.Parent != "main" || m.ParentRev != strings.Repeat("a", 40) || m.PR != 143 {
		t.Errorf("b1 mapped wrong: %+v", m)
	}
	if metas["b2"].ParentRev != "MB_b2" {
		t.Errorf("b2 empty rev should fall back to merge-base, got %q", metas["b2"].ParentRev)
	}
	if metas["b3"].ParentRev != "MB_b3" {
		t.Errorf("b3 junk rev should fall back to merge-base, got %q", metas["b3"].ParentRev)
	}
	if len(skipped) != 1 || skipped[0] != "gone" {
		t.Errorf("expected to skip [gone], got %v", skipped)
	}
}

func TestParseGraphiteSnapshot(t *testing.T) {
	// Shape mirrors a real Graphite .gt/snapshots/*.snapshot file.
	data := []byte(`{"branchesHash":"x","currentBranchName":"s2","branches":[
	  ["main",{"branchRevision":"aaa","children":["s1"]}],
	  ["s1",{"parentBranchName":"main","parentBranchRevision":"` + strings.Repeat("a", 40) + `","children":["s2"]}],
	  ["s2",{"parentBranchName":"s1","parentBranchRevision":"` + strings.Repeat("b", 40) + `","children":[]}],
	  ["stale",{"validationResult":"BAD_PARENT_NAME","branchRevision":"ccc","children":[]}]
	]}`)
	rows := parseGraphiteSnapshot(data)
	// main (no parent) and stale (no parentBranchName) are excluded.
	if len(rows) != 2 {
		t.Fatalf("want 2 rows, got %d: %+v", len(rows), rows)
	}
	got := map[string]graphiteRow{}
	for _, r := range rows {
		got[r.Branch] = r
	}
	if got["s1"].Parent != "main" || got["s2"].Parent != "s1" {
		t.Errorf("parents parsed wrong: %+v", got)
	}
	if got["s1"].ParentRev != strings.Repeat("a", 40) {
		t.Errorf("s1 parentRev wrong: %q", got["s1"].ParentRev)
	}
}
