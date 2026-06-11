package cli

import (
	"io"
	"os"
	"testing"

	"github.com/onancelabs/stitch/internal/gitx"
	"github.com/onancelabs/stitch/internal/stack"
)

// These tests drive the real cobra command tree against throwaway git
// repositories. They exercise the parts that can corrupt history if wrong:
// restack ordering, rebase --onto with the stored old base, tree-shaped stacks,
// navigation, conflict-then-continue, and the pflag flag bundling (-am).

func mustGit(t *testing.T, args ...string) string {
	t.Helper()
	out, err := gitx.Run(args...)
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return out
}

func writeFile(t *testing.T, name, content string) {
	t.Helper()
	if err := os.WriteFile(name, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// execStitch runs the CLI exactly as a user would, starting from a fresh
// command tree so flag values don't leak between calls.
func execStitch(args ...string) error {
	root := NewRootCmd("test")
	root.SetArgs(args)
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	return root.Execute()
}

func runSt(t *testing.T, args ...string) {
	t.Helper()
	if err := execStitch(args...); err != nil {
		t.Fatalf("st %v: %v", args, err)
	}
}

func setupRepo(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	old, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(old) })
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	mustGit(t, "-c", "init.defaultBranch=main", "init", "-q")
	mustGit(t, "config", "user.email", "t@example.com")
	mustGit(t, "config", "user.name", "Test")
	mustGit(t, "config", "core.editor", "true")
	mustGit(t, "config", "commit.gpgsign", "false")
	writeFile(t, "base.txt", "base\n")
	mustGit(t, "add", "-A")
	mustGit(t, "commit", "-qm", "base")
}

func TestLinearStackModifyRestack(t *testing.T) {
	setupRepo(t)
	writeFile(t, "f1.txt", "one\n")
	runSt(t, "create", "-a", "-m", "c1", "b1")
	writeFile(t, "f2.txt", "two\n")
	runSt(t, "create", "-a", "-m", "c2", "b2")
	writeFile(t, "f3.txt", "three\n")
	runSt(t, "create", "-a", "-m", "c3", "b3")

	// Modify the bottom of the stack; children must restack cleanly.
	mustGit(t, "checkout", "b1")
	writeFile(t, "f1.txt", "one-modified\n")
	runSt(t, "modify", "-a", "-m", "c1-mod")

	if cur, _ := gitx.CurrentBranch(); cur != "b1" {
		t.Fatalf("expected to return to b1 after modify, got %q", cur)
	}

	mustGit(t, "checkout", "b3")
	if got := readFile(t, "f1.txt"); got != "one-modified\n" {
		t.Errorf("b3 should see modified bottom commit, got %q", got)
	}
	if got := readFile(t, "f2.txt"); got != "two\n" {
		t.Errorf("b3 lost middle commit: %q", got)
	}
	if got := readFile(t, "f3.txt"); got != "three\n" {
		t.Errorf("b3 lost its own commit: %q", got)
	}
	if n := mustGit(t, "rev-list", "--count", "main..b3"); n != "3" {
		t.Errorf("b3 should have exactly 3 commits over main (no dup/drop), got %s", n)
	}
	if mustGit(t, "rev-parse", "b3~1") != mustGit(t, "rev-parse", "b2") {
		t.Error("b3's parent is not b2's tip")
	}
	if mustGit(t, "rev-parse", "b2~1") != mustGit(t, "rev-parse", "b1") {
		t.Error("b2's parent is not b1's tip")
	}
	m2, _ := stack.ReadMeta("b2")
	if tip := mustGit(t, "rev-parse", "b1"); m2.ParentRev != tip {
		t.Errorf("b2 parentRev not updated: %s != %s", m2.ParentRev, tip)
	}
}

func TestTreeStack(t *testing.T) {
	setupRepo(t)
	writeFile(t, "fa.txt", "a\n")
	runSt(t, "create", "-a", "-m", "cA", "A")
	writeFile(t, "fb.txt", "b\n")
	runSt(t, "create", "-a", "-m", "cB", "B")
	mustGit(t, "checkout", "A")
	writeFile(t, "fc.txt", "c\n")
	runSt(t, "create", "-a", "-m", "cC", "C")

	mustGit(t, "checkout", "A")
	writeFile(t, "fa.txt", "a-modified\n")
	runSt(t, "modify", "-a", "-m", "cA-mod")

	mustGit(t, "checkout", "B")
	if readFile(t, "fa.txt") != "a-modified\n" || readFile(t, "fb.txt") != "b\n" {
		t.Error("B incorrect after restack")
	}
	mustGit(t, "checkout", "C")
	if readFile(t, "fa.txt") != "a-modified\n" || readFile(t, "fc.txt") != "c\n" {
		t.Error("C incorrect after restack")
	}
	if _, err := os.Stat("fb.txt"); err == nil {
		t.Error("C leaked sibling B's file")
	}
}

func TestUpDown(t *testing.T) {
	setupRepo(t)
	writeFile(t, "f1.txt", "one\n")
	runSt(t, "create", "-a", "-m", "c1", "b1")
	writeFile(t, "f2.txt", "two\n")
	runSt(t, "create", "-a", "-m", "c2", "b2")

	mustGit(t, "checkout", "b1")
	runSt(t, "up")
	if cur, _ := gitx.CurrentBranch(); cur != "b2" {
		t.Fatalf("up should move to b2, got %q", cur)
	}
	runSt(t, "down")
	if cur, _ := gitx.CurrentBranch(); cur != "b1" {
		t.Fatalf("down should move to b1, got %q", cur)
	}
	runSt(t, "down")
	if cur, _ := gitx.CurrentBranch(); cur != "main" {
		t.Fatalf("down from b1 should move to main, got %q", cur)
	}
}

func TestTrackAndRestackOntoMovedTrunk(t *testing.T) {
	setupRepo(t)
	mustGit(t, "checkout", "-b", "feature")
	writeFile(t, "z.txt", "z1\n")
	mustGit(t, "add", "-A")
	mustGit(t, "commit", "-qm", "z1")

	runSt(t, "track") // parent defaults to trunk; parentRev = merge-base
	m, err := stack.ReadMeta("feature")
	if err != nil {
		t.Fatal(err)
	}
	if m.Parent != "main" {
		t.Errorf("expected parent main, got %q", m.Parent)
	}

	mustGit(t, "checkout", "main")
	writeFile(t, "more.txt", "more\n")
	mustGit(t, "add", "-A")
	mustGit(t, "commit", "-qm", "trunk-move")
	mustGit(t, "checkout", "feature")
	runSt(t, "restack")
	if n := mustGit(t, "rev-list", "--count", "main..feature"); n != "1" {
		t.Errorf("feature should keep exactly its 1 commit, got %s", n)
	}
}

func TestConflictThenContinue(t *testing.T) {
	setupRepo(t)
	writeFile(t, "shared.txt", "v1\n")
	runSt(t, "create", "-a", "-m", "x1", "x1")
	writeFile(t, "shared.txt", "v2\n")
	runSt(t, "create", "-a", "-m", "x2", "x2") // touches the same file -> will conflict

	mustGit(t, "checkout", "x1")
	writeFile(t, "shared.txt", "v1-modified\n")
	if err := execStitch("modify", "-a", "-m", "x1-mod"); err == nil {
		t.Fatal("expected a restack conflict, got nil error")
	}
	if !stack.RebaseInProgress() {
		t.Fatal("expected a paused rebase after conflict")
	}

	writeFile(t, "shared.txt", "v2-resolved\n")
	mustGit(t, "add", "shared.txt")
	if err := stack.ContinueRestack(); err != nil {
		t.Fatalf("continue failed: %v", err)
	}
	mustGit(t, "checkout", "x2")
	if got := readFile(t, "shared.txt"); got != "v2-resolved\n" {
		t.Errorf("resolved content not preserved, got %q", got)
	}
	m, _ := stack.ReadMeta("x2")
	if tip := mustGit(t, "rev-parse", "x1"); m.ParentRev != tip {
		t.Errorf("x2 parentRev not updated after continue: %s != %s", m.ParentRev, tip)
	}
	if s, _ := stack.LoadState(); s != nil {
		t.Error("restack state should be cleared after completion")
	}
}

func TestValidBranchName(t *testing.T) {
	if gitx.ValidBranchName("-x") == nil {
		t.Error("a name starting with '-' must be rejected (git option injection)")
	}
	if gitx.ValidBranchName("--upload-pack=evil") == nil {
		t.Error("an option-looking name must be rejected")
	}
	if gitx.ValidBranchName("") == nil {
		t.Error("an empty name must be rejected")
	}
	if err := gitx.ValidBranchName("feature/foo-bar"); err != nil {
		t.Errorf("a normal name should be allowed, got %v", err)
	}
}

// TestModifyBundledFlags proves pflag handles git-style combined short flags
// (-am) end-to-end through the real command.
func TestModifyBundledFlags(t *testing.T) {
	setupRepo(t)
	writeFile(t, "f1.txt", "one\n")
	runSt(t, "create", "-a", "-m", "c1", "b1")

	writeFile(t, "f1.txt", "one-edited\n")
	runSt(t, "modify", "-am", "edited f1") // -a and -m bundled, git-style

	if subj := mustGit(t, "log", "-1", "--format=%s", "b1"); subj != "edited f1" {
		t.Errorf("bundled -am did not set the message, got %q", subj)
	}
	if got := readFile(t, "f1.txt"); got != "one-edited\n" {
		t.Errorf("bundled -a did not stage the change, got %q", got)
	}
}

func TestKnownCommandRouting(t *testing.T) {
	root := NewRootCmd("test")
	for _, name := range []string{"create", "modify", "submit", "c", "m", "s", "--help", "completion", "__complete"} {
		if !isKnownCommand(root, name) {
			t.Errorf("%q should be treated as a known/builtin command", name)
		}
	}
	for _, name := range []string{"rebase", "status", "diff", "cherry-pick"} {
		if isKnownCommand(root, name) {
			t.Errorf("%q should pass through to git, not be treated as known", name)
		}
	}
}

func TestGitPassthrough(t *testing.T) {
	setupRepo(t)
	if code := Run("test", []string{"status"}); code != 0 {
		t.Errorf("`st status` should pass through to git and succeed, got exit %d", code)
	}
	if code := Run("test", []string{"rev-parse", "--git-dir"}); code != 0 {
		t.Errorf("`st rev-parse` passthrough failed, exit %d", code)
	}
}

func TestUntrackAll(t *testing.T) {
	setupRepo(t)
	writeFile(t, "f1.txt", "1\n")
	runSt(t, "create", "-a", "-m", "c1", "b1")
	writeFile(t, "f2.txt", "2\n")
	runSt(t, "create", "-a", "-m", "c2", "b2")
	if !stack.IsTracked("b1") || !stack.IsTracked("b2") {
		t.Fatal("setup: both branches should be tracked")
	}
	runSt(t, "untrack", "--all")
	if stack.IsTracked("b1") || stack.IsTracked("b2") {
		t.Error("--all should untrack every branch")
	}
	if !gitx.BranchExists("b1") || !gitx.BranchExists("b2") {
		t.Error("untrack must leave git branches intact")
	}
}

func TestUntrackThread(t *testing.T) {
	setupRepo(t)
	writeFile(t, "f1.txt", "1\n")
	runSt(t, "create", "-a", "-m", "c1", "b1")
	writeFile(t, "f2.txt", "2\n")
	runSt(t, "create", "-a", "-m", "c2", "b2")
	mustGit(t, "checkout", "b1")
	runSt(t, "untrack", "--thread")
	if stack.IsTracked("b1") || stack.IsTracked("b2") {
		t.Error("--thread should untrack the whole current thread")
	}
	if !gitx.BranchExists("b1") || !gitx.BranchExists("b2") {
		t.Error("untrack must leave git branches intact")
	}
}
