// Package gitx wraps the git CLI. Every invocation goes through exec.Command
// with a separate argv — never a shell — which closes the argument-injection
// vector for user-supplied input.
package gitx

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Run runs git with the given args and returns trimmed stdout. On failure the
// error includes git's stderr so messages are actionable.
func Run(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(errb.String())
		if msg == "" {
			msg = err.Error()
		}
		return strings.TrimSpace(out.String()), fmt.Errorf("git %s: %s", strings.Join(args, " "), msg)
	}
	return strings.TrimSpace(out.String()), nil
}

// RunIO runs git inheriting the parent's stdio. Used for commands that may open
// an editor or show progress/conflicts (commit, rebase, checkout, fetch).
func RunIO(args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

// RunInput runs git feeding the given string on stdin and returns stdout.
func RunInput(stdin string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Stdin = strings.NewReader(stdin)
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), strings.TrimSpace(errb.String()))
	}
	return strings.TrimSpace(out.String()), nil
}

// OK runs git purely for its exit status (used for predicate queries).
func OK(args ...string) bool {
	return exec.Command("git", args...).Run() == nil
}

// CurrentBranch returns the checked-out branch name, or an error if HEAD is
// detached.
func CurrentBranch() (string, error) {
	b, err := Run("symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil || b == "" {
		return "", fmt.Errorf("HEAD is detached; check out a branch first")
	}
	return b, nil
}

// RevParse resolves a ref to its object id.
func RevParse(ref string) (string, error) {
	return Run("rev-parse", "--verify", ref)
}

func BranchExists(name string) bool {
	return OK("show-ref", "--verify", "--quiet", "refs/heads/"+name)
}

func MergeBase(a, b string) (string, error) {
	return Run("merge-base", a, b)
}

// IsClean reports whether the working tree and index have no changes.
func IsClean() (bool, error) {
	out, err := Run("status", "--porcelain")
	if err != nil {
		return false, err
	}
	return out == "", nil
}

// HasStaged reports whether there are staged (indexed) changes ready to commit.
func HasStaged() bool {
	// `git diff --cached --quiet` exits non-zero when there are staged changes.
	return !OK("diff", "--cached", "--quiet")
}

// CommitSubject returns the subject line of a branch's most recent commit.
func CommitSubject(branch string) string {
	s, _ := Run("log", "-1", "--format=%s", branch)
	return s
}
