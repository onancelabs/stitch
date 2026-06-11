package cli

import (
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

	fk.merged[101] = true // pretend b1's PR got squash-merged
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
