package cli

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"

	"github.com/onancelabs/stitch/internal/gitx"
	"github.com/onancelabs/stitch/internal/stack"
)

func newTrackCmd() *cobra.Command {
	var parent string
	cmd := &cobra.Command{
		Use:   "track",
		Short: "Start tracking the current branch as part of a stack",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := gitx.RequireRepo(); err != nil {
				return err
			}
			cur, err := gitx.CurrentBranch()
			if err != nil {
				return err
			}
			trunk, err := gitx.TrunkName()
			if err != nil {
				return err
			}
			if cur == trunk {
				return fmt.Errorf("cannot track trunk itself")
			}
			p := parent
			if p == "" {
				p = trunk
			}
			if err := gitx.ValidBranchName(p); err != nil {
				return fmt.Errorf("parent: %v", err)
			}
			if p == cur {
				return fmt.Errorf("a branch cannot be its own parent")
			}
			if !gitx.BranchExists(p) {
				return fmt.Errorf("parent branch %q does not exist", p)
			}
			base, err := gitx.MergeBase(cur, p)
			if err != nil {
				return err
			}
			if err := stack.WriteMeta(cur, &stack.BranchMeta{Parent: p, ParentRev: base}); err != nil {
				return err
			}
			fmt.Printf("Tracking %s (parent: %s).\n", cur, p)
			return nil
		},
	}
	cmd.Flags().StringVarP(&parent, "parent", "p", "", "parent branch (default: trunk)")
	return cmd
}

func newUntrackCmd() *cobra.Command {
	var all, thread bool
	cmd := &cobra.Command{
		Use:   "untrack [branch]",
		Short: "Stop tracking a branch, the current thread (--thread), or all (--all)",
		Long: "Removes Stitch's metadata for one branch (re-parenting its children), " +
			"the whole current thread (--thread), or every tracked branch (--all). " +
			"Your git branches and commits are never touched; re-run 'st track' or " +
			"'st init --from-graphite' to start tracking again.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := gitx.RequireRepo(); err != nil {
				return err
			}
			if all || thread {
				if len(args) > 0 {
					return fmt.Errorf("--all/--thread take no branch argument")
				}
				return untrackBulk(all)
			}
			target := ""
			if len(args) == 1 {
				target = args[0]
			} else {
				c, err := gitx.CurrentBranch()
				if err != nil {
					return err
				}
				target = c
			}
			if !stack.IsTracked(target) {
				return fmt.Errorf("branch %q is not tracked", target)
			}
			g, err := stack.BuildGraph()
			if err != nil {
				return err
			}
			m := g.Meta[target]
			if m == nil {
				// Orphaned metadata (branch no longer exists): just drop it.
				if err := stack.DeleteMeta(target); err != nil {
					return err
				}
				fmt.Printf("Untracked %s.\n", target)
				return nil
			}
			kids := g.Children[target]
			for _, child := range kids {
				cm := g.Meta[child]
				cm.Parent = m.Parent // re-parent to this branch's parent
				if err := stack.WriteMeta(child, cm); err != nil {
					return err
				}
			}
			if err := stack.DeleteMeta(target); err != nil {
				return err
			}
			fmt.Printf("Untracked %s.\n", target)
			if len(kids) > 0 {
				fmt.Printf("Re-parented %d child branch(es) to %s; run 'st restack' to realign.\n", len(kids), m.Parent)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "untrack every tracked branch")
	cmd.Flags().BoolVar(&thread, "thread", false, "untrack the whole current thread (this stack)")
	return cmd
}

// untrackBulk removes Stitch metadata for the whole current thread, or for every
// tracked branch (all). Git branches and commits are left intact.
func untrackBulk(all bool) error {
	var names []string
	if all {
		tracked, err := stack.ListTracked()
		if err != nil {
			return err
		}
		names = tracked
	} else {
		g, err := stack.BuildGraph()
		if err != nil {
			return err
		}
		cur, err := gitx.CurrentBranch()
		if err != nil {
			return err
		}
		set := map[string]bool{}
		for _, b := range g.Ancestors(cur) {
			set[b] = true
		}
		for _, b := range g.Descendants(cur) {
			set[b] = true
		}
		for b := range set {
			names = append(names, b)
		}
	}
	if len(names) == 0 {
		fmt.Println("No tracked branches to untrack.")
		return nil
	}
	sort.Strings(names)
	for _, b := range names {
		if err := stack.DeleteMeta(b); err != nil {
			return err
		}
	}
	scope := "the current thread"
	if all {
		scope = "all threads"
	}
	fmt.Printf("Untracked %d branch(es) from %s. Your git branches and commits are untouched.\n", len(names), scope)
	return nil
}
