package cli

import (
	"bytes"
	"io"
	"strings"
	"testing"

	keyring "github.com/zalando/go-keyring"

	"github.com/onancelabs/stitch/internal/stack"
)

// stOut runs the CLI and returns everything it wrote to stdout.
func stOut(t *testing.T, args ...string) string {
	t.Helper()
	var buf bytes.Buffer
	root := NewRootCmd("test")
	root.SetArgs(args)
	root.SetOut(&buf)
	root.SetErr(io.Discard)
	if err := root.Execute(); err != nil {
		t.Fatalf("st %v: %v", args, err)
	}
	return buf.String()
}

func TestLogShowsPRStatusAndAheadBehind(t *testing.T) {
	keyring.MockInit()
	t.Setenv("GITHUB_TOKEN", "test-token")
	setupRepoWithOrigin(t)
	installFakeForge(t)

	writeFile(t, "f1.txt", "1\n")
	runSt(t, "create", "-a", "-m", "c1", "b1")
	writeFile(t, "f2.txt", "2\n")
	runSt(t, "create", "-a", "-m", "c2", "b2")

	// Before submit: no PR info, no remote refs — log looks like today.
	out := stOut(t, "log")
	if strings.Contains(out, "#") || strings.Contains(out, "↑") || strings.Contains(out, "↓") {
		t.Errorf("pre-submit log should have no PR/sync segments:\n%s", out)
	}

	runSt(t, "submit", "--no-open") // PRs 101 (b1), 102 (b2), drafts

	out = stOut(t, "log")
	if !strings.Contains(out, "b1  #101 draft") || !strings.Contains(out, "b2  #102 draft") {
		t.Errorf("log should show PR number and cached state:\n%s", out)
	}
	if strings.Contains(out, "↑") || strings.Contains(out, "↓") {
		t.Errorf("freshly pushed branches should show no ahead/behind:\n%s", out)
	}

	// One unpushed commit on b2 -> ↑1.
	mustGit(t, "checkout", "b2")
	writeFile(t, "f2.txt", "2-more\n")
	mustGit(t, "commit", "-aqm", "c2-more")
	out = stOut(t, "log")
	if !strings.Contains(out, "b2  #102 draft ↑1") {
		t.Errorf("b2 should be 1 ahead of origin/b2:\n%s", out)
	}

	// Drop b2's local extra commit after pushing it -> ↓1.
	mustGit(t, "push", "-q", "origin", "b2:b2")
	mustGit(t, "reset", "-q", "--hard", "HEAD~1")
	out = stOut(t, "log")
	if !strings.Contains(out, "b2  #102 draft ↓1") {
		t.Errorf("b2 should be 1 behind origin/b2:\n%s", out)
	}

	// Sanity: metadata carries the cached state log reads.
	m1, _ := stack.ReadMeta("b1")
	if m1.PRState != "draft" {
		t.Errorf("b1 cached state = %q, want draft", m1.PRState)
	}
}

func TestLogRemoteRefreshesPRState(t *testing.T) {
	keyring.MockInit()
	t.Setenv("GITHUB_TOKEN", "test-token")
	setupRepoWithOrigin(t)
	fk := installFakeForge(t)

	writeFile(t, "f1.txt", "1\n")
	runSt(t, "create", "-a", "-m", "c1", "b1")
	runSt(t, "submit", "--no-open") // PR 101, draft

	// The PR moved on the server; plain log still shows the cache.
	fk.states[101] = "open"
	if out := stOut(t, "log"); !strings.Contains(out, "#101 draft") {
		t.Errorf("plain log should show cached state:\n%s", out)
	}

	out := stOut(t, "log", "--remote")
	if !strings.Contains(out, "#101 open") {
		t.Errorf("--remote should show refreshed state:\n%s", out)
	}
	m, _ := stack.ReadMeta("b1")
	if m.PRState != "open" {
		t.Errorf("--remote should update the cache, got %q", m.PRState)
	}
}

func TestLogRemoteRequiresOrigin(t *testing.T) {
	setupRepo(t) // no origin remote
	if err := execStitch("log", "--remote"); err == nil {
		t.Error("log --remote without an origin remote should fail")
	}
}

func TestCreateHelpExplainsUncommittedChanges(t *testing.T) {
	out := stOut(t, "create", "--help")
	if !strings.Contains(out, "uncommitted changes") {
		t.Errorf("create --help should explain what happens to uncommitted changes:\n%s", out)
	}
}
