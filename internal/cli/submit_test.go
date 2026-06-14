package cli

import (
	"strings"
	"testing"

	keyring "github.com/zalando/go-keyring"

	"github.com/onancelabs/stitch/internal/stack"
)

// setupRepoWithOrigin gives the test repo a local bare origin so `git push`
// works without a network.
func setupRepoWithOrigin(t *testing.T) string {
	t.Helper()
	setupRepo(t)
	bare := t.TempDir()
	mustGit(t, "init", "--bare", "-q", bare)
	mustGit(t, "remote", "add", "origin", bare)
	return bare
}

func TestSubmitStack(t *testing.T) {
	keyring.MockInit()
	t.Setenv("GITHUB_TOKEN", "test-token")
	setupRepoWithOrigin(t)
	fk := installFakeForge(t)

	writeFile(t, "f1.txt", "1\n")
	runSt(t, "create", "-a", "-m", "c1", "b1")
	writeFile(t, "f2.txt", "2\n")
	runSt(t, "create", "-a", "-m", "c2", "b2")

	runSt(t, "submit", "--no-open")

	// Pushed bottom-up to the bare origin.
	heads := mustGit(t, "ls-remote", "--heads", "origin")
	if !strings.Contains(heads, "refs/heads/b1") || !strings.Contains(heads, "refs/heads/b2") {
		t.Fatalf("both branches should be pushed, got:\n%s", heads)
	}

	// One PR per branch, base = parent, drafts by default, bottom-up order.
	if len(fk.ensures) != 2 ||
		fk.ensures[0].branch != "b1" || fk.ensures[0].base != "main" ||
		fk.ensures[1].branch != "b2" || fk.ensures[1].base != "b1" {
		t.Fatalf("EnsurePR calls wrong: %+v", fk.ensures)
	}
	if !fk.ensures[0].draft {
		t.Error("submit without --ready should open drafts")
	}
	if fk.ensures[0].title != "c1" {
		t.Errorf("PR title should be the commit subject, got %q", fk.ensures[0].title)
	}

	// PR numbers written back into metadata.
	m1, _ := stack.ReadMeta("b1")
	m2, _ := stack.ReadMeta("b2")
	if m1.PR != 101 || m2.PR != 102 {
		t.Errorf("PR numbers not recorded: b1=%d b2=%d", m1.PR, m2.PR)
	}
	if m1.PRState != "draft" || m2.PRState != "draft" {
		t.Errorf("PR states not recorded: b1=%q b2=%q", m1.PRState, m2.PRState)
	}

	// Stack map: every PR body updated, entries top-of-stack first, current marked.
	if len(fk.bodies) != 2 {
		t.Fatalf("want 2 body updates, got %d", len(fk.bodies))
	}
	for _, bc := range fk.bodies {
		if len(bc.entries) != 2 || bc.entries[0].Branch != "b2" || bc.entries[1].Branch != "b1" {
			t.Errorf("entries should list top first: %+v", bc.entries)
		}
	}

	// Resubmit is idempotent: same PR numbers via knownPR.
	runSt(t, "submit", "--no-open")
	if got := fk.ensures[len(fk.ensures)-1].knownPR; got != 102 {
		t.Errorf("resubmit should pass the known PR number, got %d", got)
	}
}

func TestSubmitTitleAndBodyFromFirstCommit(t *testing.T) {
	keyring.MockInit()
	t.Setenv("GITHUB_TOKEN", "test-token")
	setupRepoWithOrigin(t)
	fk := installFakeForge(t)

	writeFile(t, "f1.txt", "1\n")
	runSt(t, "create", "b1")
	mustGit(t, "add", "-A")
	mustGit(t, "commit", "-qm", "Add API", "-m", "Detailed description.")
	writeFile(t, "f1.txt", "1+\n")
	mustGit(t, "commit", "-aqm", "fix tests")

	runSt(t, "submit", "--no-open")

	if fk.ensures[0].title != "Add API" {
		t.Errorf("title should be the FIRST commit subject, got %q", fk.ensures[0].title)
	}
	if fk.ensures[0].body != "Detailed description." {
		t.Errorf("body should be the first commit's message body, got %q", fk.ensures[0].body)
	}
}

func TestSubmitTitleFlag(t *testing.T) {
	keyring.MockInit()
	t.Setenv("GITHUB_TOKEN", "test-token")
	setupRepoWithOrigin(t)
	fk := installFakeForge(t)

	writeFile(t, "f1.txt", "1\n")
	runSt(t, "create", "-a", "-m", "c1", "b1")
	writeFile(t, "f2.txt", "2\n")
	runSt(t, "create", "-a", "-m", "c2", "b2") // current branch

	runSt(t, "submit", "--no-open", "--title", "Custom title")
	if fk.ensures[0].title != "c1" || fk.ensures[1].title != "Custom title" {
		t.Errorf("--title should hit only the current branch's PR: %+v", fk.ensures)
	}
	if len(fk.retitles) != 0 {
		t.Errorf("fresh creation should not retitle, got %+v", fk.retitles)
	}

	runSt(t, "submit", "--no-open", "--title", "Renamed")
	if len(fk.retitles) != 1 || fk.retitles[0].num != 102 || fk.retitles[0].title != "Renamed" {
		t.Errorf("resubmit with --title should retitle the current PR: %+v", fk.retitles)
	}
}
