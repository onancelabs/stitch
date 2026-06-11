package forge

import (
	"strings"
	"testing"
)

func TestParseRemote(t *testing.T) {
	cases := []struct {
		in, owner, repo string
		wantErr         bool
	}{
		{"git@github.com:owner/repo.git", "owner", "repo", false},
		{"https://github.com/owner/repo.git", "owner", "repo", false},
		{"https://github.com/owner/repo", "owner", "repo", false},
		{"ssh://git@github.com/owner/repo.git", "owner", "repo", false},
		{"git@github.com:Some-Org/My-Repo.git", "Some-Org", "My-Repo", false},
		{"https://github.com/owner/repo/", "owner", "repo", false},
		{"not-a-remote", "", "", true},
	}
	for _, c := range cases {
		owner, repo, err := ParseRemote(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("ParseRemote(%q): expected error", c.in)
			}
			continue
		}
		if err != nil || owner != c.owner || repo != c.repo {
			t.Errorf("ParseRemote(%q) = (%q,%q,%v), want (%q,%q,nil)", c.in, owner, repo, err, c.owner, c.repo)
		}
	}
}

func TestRenderStackBody(t *testing.T) {
	entries := []StackEntry{{"add-ui", 12}, {"add-api", 11}, {"add-model", 10}}

	// Empty body: a fresh managed block.
	out := RenderStackBody("", entries, "add-api")
	if !strings.HasPrefix(out, stackMarkerStart) || !strings.Contains(out, stackMarkerEnd) {
		t.Error("empty body should be wrapped in the managed markers")
	}
	if !strings.Contains(out, "#11 add-api  ← this PR") {
		t.Errorf("current PR not marked:\n%s", out)
	}
	if !strings.Contains(out, "#12 add-ui") || !strings.Contains(out, "#10 add-model") {
		t.Error("not all stack entries rendered")
	}

	// Existing managed block is replaced; surrounding text is preserved.
	existing := "Intro paragraph.\n" + stackMarkerStart + "\nOLD CONTENT\n" + stackMarkerEnd + "\nFooter."
	out = RenderStackBody(existing, entries, "add-ui")
	if !strings.Contains(out, "Intro paragraph.") || !strings.Contains(out, "Footer.") {
		t.Error("surrounding text should be preserved")
	}
	if strings.Contains(out, "OLD CONTENT") {
		t.Error("stale managed block should be replaced")
	}
	if strings.Count(out, stackMarkerStart) != 1 {
		t.Error("should not duplicate the managed block")
	}

	// No markers and non-empty: block is appended after existing text.
	out = RenderStackBody("Hello", entries, "add-ui")
	if strings.Index(out, "Hello") > strings.Index(out, stackMarkerStart) {
		t.Error("existing text should come before the appended block")
	}
}
