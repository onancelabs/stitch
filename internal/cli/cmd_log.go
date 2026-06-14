package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/onancelabs/stitch/internal/auth"
	"github.com/onancelabs/stitch/internal/gitx"
	"github.com/onancelabs/stitch/internal/stack"
)

func newLogCmd() *cobra.Command {
	var remote bool
	cmd := &cobra.Command{
		Use:     "log",
		Aliases: []string{"ls"},
		Short:   "Show the stack as a tree (top of stack first)",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := gitx.RequireRepo(); err != nil {
				return err
			}
			if remote {
				if err := refreshPRStates(cmd.OutOrStdout()); err != nil {
					return err
				}
			}
			g, err := stack.BuildGraph()
			if err != nil {
				return err
			}
			cur, _ := gitx.CurrentBranch()
			printStack(cmd.OutOrStdout(), g, cur)
			return nil
		},
	}
	cmd.Flags().BoolVar(&remote, "remote", false, "fetch origin and refresh PR states from the code host")
	return cmd
}

// refreshPRStates fetches origin (so ahead/behind is current) and re-reads the
// state of every PR in the stack, updating the per-branch cache.
func refreshPRStates(w io.Writer) error {
	originURL, err := gitx.Run("remote", "get-url", "origin")
	if err != nil {
		return fmt.Errorf("no 'origin' remote: %v", err)
	}
	token, err := auth.ResolveToken(auth.DefaultHost)
	if err != nil {
		return err // already hints at 'st auth'
	}
	f, err := newForge(originURL, token)
	if err != nil {
		return err
	}
	if err := gitx.RunIO("fetch", "--prune", "origin"); err != nil {
		return err
	}
	g, err := stack.BuildGraph()
	if err != nil {
		return err
	}
	for _, b := range g.Order {
		m := g.Meta[b]
		if m.PR == 0 {
			continue
		}
		state, err := f.PRState(m.PR)
		if err != nil {
			fmt.Fprintf(w, "Note: could not refresh #%d (%s): %v\n", m.PR, b, err)
			continue
		}
		if state != m.PRState {
			m.PRState = state
			if err := stack.WriteMeta(b, m); err != nil {
				return err
			}
		}
	}
	return nil
}

func printStack(w io.Writer, g *stack.Graph, cur string) {
	var lines []string
	var rec func(node string, depth int)
	rec = func(node string, depth int) {
		kids := g.Children[node]
		// Print children (higher in the stack) above their parent.
		for i := len(kids) - 1; i >= 0; i-- {
			rec(kids[i], depth+1)
		}
		marker := "○"
		if node == cur {
			marker = "●"
		}
		label := node
		if node == g.Trunk {
			label += " (trunk)"
		}
		extra := ""
		m, tracked := g.Meta[node]
		if tracked && m.PR != 0 {
			extra += fmt.Sprintf("  #%d", m.PR)
			if m.PRState != "" {
				extra += " " + m.PRState
			}
		}
		if tracked || node == g.Trunk {
			if s := remoteSync(node); s != "" {
				extra += " " + s
			}
		}
		if tracked && g.NeedsRestack(node) {
			extra += "  (needs restack)"
		}
		lines = append(lines, fmt.Sprintf("%s%s %s%s", strings.Repeat("  ", depth), marker, label, extra))
	}

	// Roots: trunk plus any external (non-tracked) parent referenced by a stack.
	roots := []string{g.Trunk}
	seen := map[string]bool{g.Trunk: true}
	for _, m := range g.Meta {
		if _, ok := g.Meta[m.Parent]; !ok && !seen[m.Parent] {
			roots = append(roots, m.Parent)
			seen[m.Parent] = true
		}
	}
	for _, r := range roots {
		rec(r, 0)
	}
	for _, l := range lines {
		fmt.Fprintln(w, l)
	}
}

// remoteSync renders "↑n ↓m" vs origin/<branch> (empty when in sync or the
// branch was never pushed). Local-only: reflects the last fetch.
func remoteSync(branch string) string {
	if !gitx.OK("rev-parse", "--verify", "--quiet", "refs/remotes/origin/"+branch) {
		return ""
	}
	ahead, behind, err := gitx.AheadBehind("origin/"+branch, branch)
	if err != nil {
		return ""
	}
	s := ""
	if ahead > 0 {
		s += fmt.Sprintf(" ↑%d", ahead)
	}
	if behind > 0 {
		s += fmt.Sprintf(" ↓%d", behind)
	}
	return strings.TrimPrefix(s, " ")
}
