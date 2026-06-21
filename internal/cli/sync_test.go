package cli

import (
	"io"
	"os"
	"strings"
	"testing"

	keyring "github.com/zalando/go-keyring"

	"github.com/onancelabs/stitch/internal/gitx"
	"github.com/onancelabs/stitch/internal/stack"
)

func TestSyncCleansUpMergedBranch(t *testing.T) {
	keyring.MockInit()
	t.Setenv("GITHUB_TOKEN", "test-token")
	setupRepoWithOrigin(t)
	fk := installFakeForge(t)

	writeFile(t, "f1.txt", "1\n")
	runSt(t, "create", "-a", "-m", "c1", "b1")
	writeFile(t, "f2.txt", "2\n")
	runSt(t, "create", "-a", "-m", "c2", "b2")
	runSt(t, "submit", "--no-open") // assigns PRs 101 (b1), 102 (b2)

	fk.states[101] = "merged" // pretend b1's PR got squash-merged
	mustGit(t, "checkout", "main")
	runSt(t, "sync", "--no-fetch")

	if gitx.BranchExists("b1") {
		t.Error("merged branch b1 should be deleted")
	}
	if stack.IsTracked("b1") {
		t.Error("merged branch b1 should be untracked")
	}
	m2, err := stack.ReadMeta("b2")
	if err != nil || m2.Parent != "main" {
		t.Fatalf("b2 should be re-parented onto main, got %+v err=%v", m2, err)
	}
	// b2 was restacked onto main and carries only its own commit.
	if n := mustGit(t, "rev-list", "--count", "main..b2"); n != "1" {
		t.Errorf("b2 should have exactly 1 commit over main, got %s", n)
	}
	g, err := stack.BuildGraph()
	if err != nil {
		t.Fatal(err)
	}
	if g.NeedsRestack("b2") {
		t.Error("b2 should be fully restacked after sync")
	}
}

// captureStdout redirects os.Stdout for the duration of fn and returns what was
// written. The sync command reports via fmt.Print* (real stdout), not the cobra
// out writer, so tests capture it here.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	done := make(chan string, 1)
	go func() {
		b, err := io.ReadAll(r)
		if err != nil {
			t.Errorf("captureStdout: read error: %v", err)
		}
		done <- string(b)
	}()
	// Restore stdout and close the writer no matter how fn() exits, so a
	// panicking/t.Fatal-ing fn() can't leak the reader goroutine.
	defer func() { os.Stdout = old }()
	defer func() { _ = w.Close() }()
	fn()
	_ = w.Close() // normal path: unblock the reader before we wait on done
	return <-done
}

// buildConflictingStacks creates two independent stacks off main, then advances
// main so that restacking the "b" stack conflicts (b1 adds shared.txt, which the
// new trunk also adds) while the "a" stack (a1 adds a.txt) restacks cleanly.
// Leaves HEAD on main. Both branches were based on the pre-advance main.
func buildConflictingStacks(t *testing.T) {
	t.Helper()
	writeFile(t, "a.txt", "a\n")
	runSt(t, "create", "-a", "-m", "ca", "a1")
	mustGit(t, "checkout", "main")
	writeFile(t, "shared.txt", "b-version\n")
	runSt(t, "create", "-a", "-m", "cb", "b1")
	// Advance trunk with a conflicting shared.txt.
	mustGit(t, "checkout", "main")
	writeFile(t, "shared.txt", "main-version\n")
	mustGit(t, "add", "-A")
	mustGit(t, "commit", "-qm", "trunk-advance")
}

func TestSyncIsolatesOtherStackConflict(t *testing.T) {
	setupRepo(t)
	buildConflictingStacks(t)

	// Current stack = a (clean). The b stack must skip-and-report, not block.
	mustGit(t, "checkout", "a1")
	var syncErr error
	out := captureStdout(t, func() { syncErr = execStitch("sync", "--no-fetch") })

	if syncErr != nil {
		t.Fatalf("sync should succeed (exit 0) when only OTHER stacks conflict, got %v", syncErr)
	}
	if stack.RebaseInProgress() {
		t.Fatal("sync must not leave a paused rebase for an other-stack conflict")
	}
	if s, _ := stack.LoadState(); s != nil {
		t.Error("no restack state should remain after an other-stack skip")
	}
	clean, _ := gitx.IsClean()
	if !clean {
		t.Error("working tree should be clean after the aborted rebase")
	}
	g, err := stack.BuildGraph()
	if err != nil {
		t.Fatal(err)
	}
	if g.NeedsRestack("a1") {
		t.Error("current stack a1 should be fully restacked")
	}
	if !g.NeedsRestack("b1") {
		t.Error("conflicting other stack b1 should be left UNrestacked")
	}
	if cur, _ := gitx.CurrentBranch(); cur != "a1" {
		t.Errorf("should return to a1 after sync, got %q", cur)
	}
	if !strings.Contains(out, "skipped") || !strings.Contains(out, "b1") {
		t.Errorf("report should mention the skipped stack and b1, got:\n%s", out)
	}
}

func TestSyncCurrentStackConflictPauses(t *testing.T) {
	setupRepo(t)
	// Two independent stacks that BOTH conflict with an advancing trunk:
	//   - e1 (other stack): adds shared.txt
	//   - b1 (current stack): adds shared.txt
	// Old single-plan sync paused on whichever came first in g.Order and left
	// the other queued in restack state. The new sync must pause on the CURRENT
	// stack (b1) while skip-and-reporting the other (e1), and `st continue` must
	// finish with ONLY b1 — never re-touching e1.
	writeFile(t, "shared.txt", "e-version\n")
	runSt(t, "create", "-a", "-m", "ce", "e1")
	mustGit(t, "checkout", "main")
	writeFile(t, "shared.txt", "b-version\n")
	runSt(t, "create", "-a", "-m", "cb", "b1")
	// Advance trunk with a conflicting shared.txt.
	mustGit(t, "checkout", "main")
	writeFile(t, "shared.txt", "main-version\n")
	mustGit(t, "add", "-A")
	mustGit(t, "commit", "-qm", "trunk-advance")

	// Current stack = b1 (conflicts). It must pause; e1 must skip-and-report.
	mustGit(t, "checkout", "b1")
	var syncErr error
	out := captureStdout(t, func() { syncErr = execStitch("sync", "--no-fetch") })
	if syncErr == nil {
		t.Fatal("sync should return an error (paused) when the CURRENT stack conflicts")
	}
	if !stack.RebaseInProgress() {
		t.Fatal("current-stack conflict should leave a paused rebase")
	}
	// The other conflicting stack was skipped and reported, not paused on.
	if !strings.Contains(out, "skipped") || !strings.Contains(out, "e1") {
		t.Errorf("other conflicting stack e1 should be skip-reported, got:\n%s", out)
	}
	// The paused restack state holds ONLY the current stack — the old single-plan
	// code would still have e1 queued here.
	s, _ := stack.LoadState()
	if s == nil {
		t.Fatal("expected paused restack state for the current stack")
	}
	for _, op := range s.Ops {
		if op.Branch == "e1" {
			t.Errorf("paused state must not contain the skipped other stack e1: %+v", s.Ops)
		}
	}

	// Resolve b1 and continue: it must COMPLETE (only b1 in phase 2), never
	// re-touching e1.
	writeFile(t, "shared.txt", "resolved\n")
	mustGit(t, "add", "shared.txt")
	if err := stack.ContinueRestack(); err != nil {
		t.Fatalf("continue after resolving the current stack should complete, got: %v", err)
	}
	if st, _ := stack.LoadState(); st != nil {
		t.Error("restack state should be cleared after continue completes")
	}
	// e1 was never restacked; it stays skipped for the user to handle later.
	g, err := stack.BuildGraph()
	if err != nil {
		t.Fatal(err)
	}
	if !g.NeedsRestack("e1") {
		t.Error("skipped other stack e1 should remain UNrestacked after the current stack resolves")
	}
}

func TestSyncOnTrunkReportsAllConflicts(t *testing.T) {
	setupRepo(t)
	buildConflictingStacks(t) // leaves HEAD on main

	var syncErr error
	out := captureStdout(t, func() { syncErr = execStitch("sync", "--no-fetch") })
	if syncErr != nil {
		t.Fatalf("sync on trunk should exit 0 even with a conflicting stack, got %v", syncErr)
	}
	if stack.RebaseInProgress() {
		t.Fatal("nothing should pause when sync runs on trunk")
	}
	g, _ := stack.BuildGraph()
	if g.NeedsRestack("a1") {
		t.Error("clean stack a1 should be restacked")
	}
	if !g.NeedsRestack("b1") {
		t.Error("conflicting stack b1 should be skipped and left UNrestacked")
	}
	if !strings.Contains(out, "skipped") || !strings.Contains(out, "b1") {
		t.Errorf("report should list the skipped b stack, got:\n%s", out)
	}
}

func TestSyncSkippedStackKeepsPartialProgress(t *testing.T) {
	setupRepo(t)
	// Three-branch stack: c1 (clean) -> c2 (adds shared.txt, will conflict) -> c3.
	writeFile(t, "c1.txt", "c1\n")
	runSt(t, "create", "-a", "-m", "cc1", "c1")
	writeFile(t, "shared.txt", "c-version\n")
	runSt(t, "create", "-a", "-m", "cc2", "c2")
	writeFile(t, "c3.txt", "c3\n")
	runSt(t, "create", "-a", "-m", "cc3", "c3")
	// Advance trunk so c2's shared.txt conflicts; c1 still restacks cleanly.
	mustGit(t, "checkout", "main")
	writeFile(t, "shared.txt", "main-version\n")
	mustGit(t, "add", "-A")
	mustGit(t, "commit", "-qm", "trunk-advance")

	// On trunk → the whole c stack is an "other" stack and skips on c2.
	var syncErr error
	out := captureStdout(t, func() { syncErr = execStitch("sync", "--no-fetch") })
	if syncErr != nil {
		t.Fatalf("sync should exit 0, got %v", syncErr)
	}
	if stack.RebaseInProgress() {
		t.Fatal("the c stack should have been aborted, not left paused")
	}
	g, _ := stack.BuildGraph()
	if g.NeedsRestack("c1") {
		t.Error("c1 restacked cleanly before the conflict and should be kept (partial progress)")
	}
	if !g.NeedsRestack("c2") {
		t.Error("c2 conflicted and should be left UNrestacked")
	}
	if !strings.Contains(out, "c2") {
		t.Errorf("report should name the conflict branch c2, got:\n%s", out)
	}
}
