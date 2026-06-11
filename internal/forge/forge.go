package forge

// Forge is a code host that can manage pull requests for one repository. It is
// exactly what submit/sync need today — implement it to add a new host.
type Forge interface {
	// EnsurePR finds an open PR for branch (preferring knownPR), creating it
	// with the given base when none exists, or re-basing it if it drifted.
	EnsurePR(branch, base, title string, draft bool, knownPR int) (num int, url string, err error)
	// UpdatePRBody writes the managed stack map into the PR body.
	UpdatePRBody(num int, entries []StackEntry, current string) error
	// PRMerged reports whether the PR was merged (squash-aware).
	PRMerged(num int) (bool, error)
}
