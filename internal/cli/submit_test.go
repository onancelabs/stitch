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

	// Each PR gets a stitch-managed comment listing entries top-first, marking itself.
	if got := fk.comments[101]; !strings.Contains(got, "#102 `b2`") || !strings.Contains(got, "#101 `b1`  👈 this PR") {
		t.Errorf("b1's comment wrong:\n%s", got)
	}
	if got := fk.comments[102]; !strings.Contains(got, "#102 `b2`  👈 this PR") {
		t.Errorf("b2's comment should mark itself:\n%s", got)
	}
	id1, id2 := fk.commentID[101], fk.commentID[102]

	// Resubmit is idempotent: same PR numbers via knownPR.
	runSt(t, "submit", "--no-open")
	if got := fk.ensures[len(fk.ensures)-1].knownPR; got != 102 {
		t.Errorf("resubmit should pass the known PR number, got %d", got)
	}
	// ...and the same comments are edited, not recreated.
	if fk.commentID[101] != id1 || fk.commentID[102] != id2 {
		t.Error("resubmit should reuse the same comment IDs")
	}
	if fk.commentCreate[101] != 1 || fk.commentCreate[102] != 1 {
		t.Errorf("resubmit must not create duplicate comments: %+v", fk.commentCreate)
	}
}

func TestSubmitStripsLegacyBodyOnce(t *testing.T) {
	keyring.MockInit()
	t.Setenv("GITHUB_TOKEN", "test-token")
	setupRepoWithOrigin(t)
	fk := installFakeForge(t)

	writeFile(t, "f1.txt", "1\n")
	runSt(t, "create", "-a", "-m", "c1", "b1")

	runSt(t, "submit", "--no-open") // first submit: no cached comment yet
	if len(fk.stripped) != 1 || fk.stripped[0] != 101 {
		t.Fatalf("first submit should strip the legacy body once: %+v", fk.stripped)
	}

	runSt(t, "submit", "--no-open") // comment ID now cached
	if len(fk.stripped) != 1 {
		t.Errorf("resubmit must not strip again (comment cached): %+v", fk.stripped)
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
