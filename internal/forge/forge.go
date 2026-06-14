package forge

// Forge is a code host that can manage pull requests for one repository. It is
// exactly what submit/sync/log need today — implement it to add a new host.
type Forge interface {
	// EnsurePR finds an open PR for branch (preferring knownPR), creating it
	// with the given base, title, and body when none exists, or re-basing it
	// if it drifted. body is used only at creation; existing descriptions are
	// never touched here. state is one of "draft", "open", "closed", "merged".
	EnsurePR(branch, base, title, body string, draft bool, knownPR int) (num int, url, state string, err error)
	// SetPRTitle replaces the PR's title.
	SetPRTitle(num int, title string) error
	// UpdatePRBody writes the managed stack map into the PR body.
	UpdatePRBody(num int, entries []StackEntry, current string) error
	// PRState reports the PR's state: "draft", "open", "closed", or "merged"
	// (squash-aware, unlike `git branch --merged`).
	PRState(num int) (string, error)
}
