package gitx

import (
	"fmt"
	"strings"
)

// ValidBranchName rejects names that are empty or could be misread by git as a
// command-line option (a leading dash). Combined with always invoking git via
// exec.Command with a separate argv (never a shell), this closes the
// argument-injection vector for user-supplied branch names. git itself enforces
// the remaining ref-name rules when the branch is actually created.
func ValidBranchName(name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("branch name is empty")
	}
	if strings.HasPrefix(name, "-") {
		return fmt.Errorf("invalid branch name %q (must not start with '-')", name)
	}
	return nil
}

// RequireRepo errors out if the current directory is not inside a git repo.
func RequireRepo() error {
	if !OK("rev-parse", "--git-dir") {
		return fmt.Errorf("not a git repository")
	}
	return nil
}

// Path returns the path to a file inside the git dir, worktree-safe.
func Path(rel string) (string, error) {
	return Run("rev-parse", "--git-path", rel)
}

// TrunkName returns the configured trunk branch, auto-detecting and persisting
// main/master the first time if no config is set.
func TrunkName() (string, error) {
	if v, err := Run("config", "--get", "stitch.trunk"); err == nil && v != "" {
		return v, nil
	}
	for _, c := range []string{"main", "master"} {
		if BranchExists(c) {
			_, _ = Run("config", "stitch.trunk", c) // best effort persist
			return c, nil
		}
	}
	return "", fmt.Errorf("could not detect trunk branch; run 'st init --trunk <name>'")
}

// SetTrunk records the trunk branch in git config after validating it exists.
func SetTrunk(name string) error {
	if !BranchExists(name) {
		return fmt.Errorf("branch %q does not exist", name)
	}
	_, err := Run("config", "stitch.trunk", name)
	return err
}
