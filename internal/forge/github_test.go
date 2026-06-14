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

func TestRenderStackComment(t *testing.T) {
	entries := []StackEntry{{"add-ui", 12}, {"add-api", 11}, {"add-model", 10}}
	out := RenderStackComment(entries, "add-api", "main")

	if !strings.HasPrefix(out, stackMarkerStart) || !strings.Contains(out, stackMarkerEnd) {
		t.Error("comment should be wrapped in the managed markers")
	}
	if !strings.Contains(out, "[!NOTE]") || !strings.Contains(out, "🧵 **Stitch thread**") {
		t.Errorf("missing the thread callout header:\n%s", out)
	}
	if !strings.Contains(out, "#11 `add-api`  👈 this PR") {
		t.Errorf("current PR not marked:\n%s", out)
	}
	if strings.Contains(out, "#12 `add-ui`  👈") || strings.Contains(out, "#10 `add-model`  👈") {
		t.Errorf("only the current PR should be marked:\n%s", out)
	}
	if !strings.Contains(out, "- `main`") {
		t.Errorf("trunk row missing:\n%s", out)
	}
	if !strings.Contains(out, "Managed by [stitch](https://github.com/onancelabs/stitch)") {
		t.Errorf("footer missing:\n%s", out)
	}
}

func TestStripStackBlock(t *testing.T) {
	body := "Intro paragraph.\n\n" + stackMarkerStart + "\nOLD STACK\n" + stackMarkerEnd + "\n\nFooter."
	cleaned, changed := stripStackBlock(body)
	if !changed {
		t.Fatal("a body containing the block should report changed=true")
	}
	if strings.Contains(cleaned, "OLD STACK") || strings.Contains(cleaned, stackMarkerStart) {
		t.Errorf("block not removed:\n%s", cleaned)
	}
	if !strings.Contains(cleaned, "Intro paragraph.") || !strings.Contains(cleaned, "Footer.") {
		t.Errorf("surrounding text should be preserved:\n%s", cleaned)
	}

	plain := "Just a description, no block."
	out, changed := stripStackBlock(plain)
	if changed || out != plain {
		t.Errorf("a body without the block should be unchanged, got changed=%v %q", changed, out)
	}
}
