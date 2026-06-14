// Package forge talks to code-hosting services. Today: GitHub. ALL go-github
// usage is confined to this package.
package forge

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/go-github/v66/github"
)

// ghClient is an alias so the rest of the package can hold a GitHub client
// without spelling out the go-github type everywhere.
type ghClient = *github.Client

// StackEntry is one branch/PR pair used to render the stack map in PR bodies.
type StackEntry struct {
	Branch string
	PR     int
}

func sp(s string) *string { return &s }
func bp(b bool) *bool     { return &b }

// ParseRemote extracts owner and repo from a git remote URL. It accepts the
// common SSH and HTTPS forms (with or without a ".git" suffix):
//
//	git@github.com:owner/repo.git
//	ssh://git@github.com/owner/repo.git
//	https://github.com/owner/repo
func ParseRemote(raw string) (owner, repo string, err error) {
	s := strings.TrimSpace(raw)
	s = strings.TrimSuffix(s, ".git")
	if i := strings.Index(s, "://"); i >= 0 { // strip scheme
		s = s[i+3:]
	}
	if i := strings.Index(s, "@"); i >= 0 { // strip userinfo (git@)
		s = s[i+1:]
	}
	s = strings.Replace(s, ":", "/", 1) // host:owner/repo -> host/owner/repo
	var parts []string
	for _, p := range strings.Split(s, "/") {
		if p != "" {
			parts = append(parts, p)
		}
	}
	if len(parts) < 3 {
		return "", "", fmt.Errorf("could not parse owner/repo from remote %q", raw)
	}
	return parts[len(parts)-2], parts[len(parts)-1], nil
}

// Client bundles a GitHub API client with the repository it talks to.
type Client struct {
	gh    ghClient
	Owner string
	Repo  string
}

// New parses the remote URL and returns a ready GitHub-backed Forge using
// token.
func New(remoteURL, token string) (Forge, error) {
	owner, repo, err := ParseRemote(remoteURL)
	if err != nil {
		return nil, err
	}
	return &Client{gh: github.NewClient(nil).WithAuthToken(token), Owner: owner, Repo: repo}, nil
}

var _ Forge = (*Client)(nil)

// EnsurePR finds an open PR for branch (preferring a known PR number), creating
// one with base=base if none exists, or updating its base if it drifted. It
// returns the PR number, HTML URL, and current state. body applies only when
// the PR is created.
func (c *Client) EnsurePR(branch, base, title, body string, draft bool, knownPR int) (int, string, string, error) {
	return ensurePR(c.gh, c.Owner, c.Repo, branch, base, title, body, draft, knownPR)
}

// SetPRTitle replaces the PR's title.
func (c *Client) SetPRTitle(num int, title string) error {
	_, _, err := c.gh.PullRequests.Edit(context.Background(), c.Owner, c.Repo, num, &github.PullRequest{Title: sp(title)})
	return err
}

// UpdatePRBody writes the stack map into a PR's body, replacing any existing
// managed block, and only issues an edit when the body actually changes.
func (c *Client) UpdatePRBody(num int, entries []StackEntry, current string) error {
	return updateStackBody(c.gh, c.Owner, c.Repo, num, entries, current)
}

// PRState reports the PR's state ("draft", "open", "closed", "merged" — the
// latter true for squash merges too, which `git branch --merged` cannot detect).
func (c *Client) PRState(num int) (string, error) {
	p, _, err := c.gh.PullRequests.Get(context.Background(), c.Owner, c.Repo, num)
	if err != nil {
		return "", err
	}
	return prStateOf(p), nil
}

// prStateOf collapses GitHub's merged/state/draft fields into one word.
func prStateOf(p *github.PullRequest) string {
	switch {
	case p.GetMerged():
		return "merged"
	case p.GetState() == "closed":
		return "closed"
	case p.GetDraft():
		return "draft"
	default:
		return "open"
	}
}

func ensurePR(client ghClient, owner, repo, branch, base, title, body string, draft bool, knownPR int) (int, string, string, error) {
	ctx := context.Background()
	var pr *github.PullRequest
	if knownPR != 0 {
		if p, _, err := client.PullRequests.Get(ctx, owner, repo, knownPR); err == nil && p.GetState() == "open" {
			pr = p
		}
	}
	if pr == nil {
		prs, _, err := client.PullRequests.List(ctx, owner, repo, &github.PullRequestListOptions{
			Head:  owner + ":" + branch,
			State: "open",
		})
		if err != nil {
			return 0, "", "", err
		}
		if len(prs) > 0 {
			pr = prs[0]
		}
	}
	if pr == nil {
		created, _, err := client.PullRequests.Create(ctx, owner, repo, &github.NewPullRequest{
			Title: sp(title),
			Head:  sp(branch),
			Base:  sp(base),
			Body:  sp(body),
			Draft: bp(draft),
		})
		if err != nil {
			return 0, "", "", err
		}
		return created.GetNumber(), created.GetHTMLURL(), prStateOf(created), nil
	}
	if pr.GetBase().GetRef() != base {
		if _, _, err := client.PullRequests.Edit(ctx, owner, repo, pr.GetNumber(), &github.PullRequest{
			Base: &github.PullRequestBranch{Ref: sp(base)},
		}); err != nil {
			return 0, "", "", err
		}
	}
	return pr.GetNumber(), pr.GetHTMLURL(), prStateOf(pr), nil
}

func updateStackBody(client ghClient, owner, repo string, num int, entries []StackEntry, current string) error {
	ctx := context.Background()
	p, _, err := client.PullRequests.Get(ctx, owner, repo, num)
	if err != nil {
		return err
	}
	nb := RenderStackBody(p.GetBody(), entries, current)
	if nb == p.GetBody() {
		return nil
	}
	_, _, err = client.PullRequests.Edit(ctx, owner, repo, num, &github.PullRequest{Body: sp(nb)})
	return err
}

const (
	stackMarkerStart = "<!-- stitch:start -->"
	stackMarkerEnd   = "<!-- stitch:end -->"
)

// RenderStackBody returns existing with the stitch-managed block inserted or
// replaced. entries are listed top of stack first. This is pure and unit tested.
func RenderStackBody(existing string, entries []StackEntry, current string) string {
	var b strings.Builder
	b.WriteString(stackMarkerStart + "\n")
	b.WriteString("**Stack** (top to bottom, managed by stitch):\n")
	for _, e := range entries {
		line := fmt.Sprintf("- #%d `%s`", e.PR, e.Branch)
		if e.Branch == current {
			line += "  👈 this PR"
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	b.WriteString(stackMarkerEnd)
	block := b.String()

	if i := strings.Index(existing, stackMarkerStart); i >= 0 {
		if j := strings.Index(existing, stackMarkerEnd); j > i {
			return existing[:i] + block + existing[j+len(stackMarkerEnd):]
		}
	}
	if strings.TrimSpace(existing) == "" {
		return block + "\n"
	}
	return existing + "\n\n" + block + "\n"
}
