package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/onancelabs/stitch/internal/gitx"
	"github.com/onancelabs/stitch/internal/stack"
)

func newLogCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "log",
		Aliases: []string{"ls"},
		Short:   "Show the stack as a tree (top of stack first)",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := gitx.RequireRepo(); err != nil {
				return err
			}
			g, err := stack.BuildGraph()
			if err != nil {
				return err
			}
			cur, _ := gitx.CurrentBranch()
			printStack(g, cur)
			return nil
		},
	}
}

func printStack(g *stack.Graph, cur string) {
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
		if _, tracked := g.Meta[node]; tracked && g.NeedsRestack(node) {
			extra = "  (needs restack)"
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
		fmt.Println(l)
	}
}
