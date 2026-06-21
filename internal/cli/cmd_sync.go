package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/onancelabs/stitch/internal/auth"
	"github.com/onancelabs/stitch/internal/forge"
	"github.com/onancelabs/stitch/internal/gitx"
	"github.com/onancelabs/stitch/internal/stack"
)

func newSyncCmd() *cobra.Command {
	var noFetch bool
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Fetch, fast-forward trunk, drop merged branches, and restack",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := gitx.RequireRepo(); err != nil {
				return err
			}
			if err := ensureNoPausedRestack(); err != nil {
				return err
			}
			clean, err := gitx.IsClean()
			if err != nil {
				return err
			}
			if !clean {
				return fmt.Errorf("working tree is dirty; commit or stash changes first")
			}
			trunk, err := gitx.TrunkName()
			if err != nil {
				return err
			}
			hasOrigin := gitx.OK("remote", "get-url", "origin")
			if !noFetch && hasOrigin {
				fmt.Println("Fetching origin...")
				if err := gitx.RunIO("fetch", "--prune", "origin"); err != nil {
					return err
				}
			}

			// Fast-forward trunk to its upstream, if that is a fast-forward.
			remoteTrunk := "origin/" + trunk
			if hasOrigin && gitx.OK("rev-parse", "--verify", "--quiet", remoteTrunk) {
				cur, _ := gitx.CurrentBranch()
				if cur == trunk {
					if err := gitx.RunIO("merge", "--ff-only", remoteTrunk); err != nil {
						return fmt.Errorf("could not fast-forward %s: %v", trunk, err)
					}
				} else if gitx.OK("merge-base", "--is-ancestor", trunk, remoteTrunk) {
					if _, err := gitx.Run("update-ref", "refs/heads/"+trunk, remoteTrunk); err != nil {
						return err
					}
					fmt.Printf("Fast-forwarded %s to %s.\n", trunk, remoteTrunk)
				} else {
					fmt.Printf("Note: %s diverged from %s; leaving it as-is.\n", trunk, remoteTrunk)
				}
			}

			// Detect and clean up merged PRs (squash-merge aware) when we can
			// reach GitHub; otherwise just do the local restack.
			if hasOrigin {
				if token, err := auth.ResolveToken(auth.DefaultHost); err == nil {
					if originURL, e := gitx.Run("remote", "get-url", "origin"); e == nil {
						if f, e2 := newForge(originURL, token); e2 == nil {
							if err := cleanupMerged(f, trunk); err != nil {
								fmt.Printf("Note: merged-PR cleanup skipped: %v\n", err)
							}
						}
					}
				} else {
					fmt.Println("(No GitHub token; skipping merged-PR cleanup. Run 'st auth' to enable.)")
				}
			}

			cur, err := gitx.CurrentBranch()
			if err != nil {
				return err
			}
			g, err := stack.BuildGraph()
			if err != nil {
				return err
			}
			if len(g.Order) == 0 {
				fmt.Println("No tracked branches to restack.")
				return nil
			}
			// Restack every stack (trunk moved, so all are stale), but isolate
			// conflicts: other stacks skip-and-report, the current stack pauses.
			others, currentOps := g.PartitionForSync(cur)
			failures, err := stack.RestackStacksIsolated(others)
			if err != nil {
				return err
			}
			if len(failures) > 0 {
				reportSkippedStacks(failures)
			}
			if len(currentOps) == 0 {
				// On trunk / untracked: nothing to pause on. Return to start.
				if cur != "" {
					_, _ = gitx.Run("checkout", cur)
				}
				return nil
			}
			return stack.ExecuteRestack(&stack.State{Ops: currentOps, Return: cur})
		},
	}
	cmd.Flags().BoolVar(&noFetch, "no-fetch", false, "skip git fetch")
	return cmd
}

// reportSkippedStacks prints the stacks sync left untouched because they
// conflicted. They are not the current stack, so they never blocked the sync;
// the user resolves each by checking it out and running 'st restack'.
func reportSkippedStacks(failures []stack.StackFailure) {
	fmt.Printf("\n%d stack(s) skipped due to conflicts (your other stacks weren't changed):\n", len(failures))
	for _, f := range failures {
		fmt.Printf("  %s\tconflict on %s\t→ `git checkout %s && st restack`\n", f.Base, f.Branch, f.Branch)
	}
	fmt.Println()
}

// cleanupMerged asks the forge which tracked branches have merged PRs
// (including squash merges) and, for each, re-parents its children onto the
// nearest non-merged ancestor, then deletes the merged branch locally.
func cleanupMerged(f forge.Forge, trunk string) error {
	g, err := stack.BuildGraph()
	if err != nil {
		return err
	}
	mergedSet := map[string]bool{}
	prOf := map[string]int{}
	var merged []string
	for b, m := range g.Meta {
		if m.PR == 0 {
			continue
		}
		state, err := f.PRState(m.PR)
		if err != nil {
			continue // PR not found / transient: leave the branch alone
		}
		if state == "merged" {
			mergedSet[b] = true
			prOf[b] = m.PR
			merged = append(merged, b)
		} else if state != m.PRState {
			m.PRState = state // refresh the cache for surviving branches
			if err := stack.WriteMeta(b, m); err != nil {
				return err
			}
		}
	}
	if len(merged) == 0 {
		return nil
	}

	// Walk up past any merged parents to the first surviving branch (or trunk).
	resolveParent := func(p string) string {
		for mergedSet[p] {
			pm := g.Meta[p]
			if pm == nil {
				break
			}
			p = pm.Parent
		}
		return p
	}
	for b := range mergedSet {
		for _, c := range g.Children[b] {
			if mergedSet[c] {
				continue
			}
			cm := g.Meta[c]
			cm.Parent = resolveParent(cm.Parent)
			if err := stack.WriteMeta(c, cm); err != nil {
				return err
			}
		}
	}

	// Don't delete the branch we're standing on.
	if cur, err := gitx.CurrentBranch(); err == nil && mergedSet[cur] {
		if err := gitx.RunIO("checkout", trunk); err != nil {
			return err
		}
	}
	for _, b := range merged {
		_ = stack.DeleteMeta(b)
		if _, err := gitx.Run("branch", "-D", b); err != nil {
			fmt.Printf("Note: could not delete local branch %s: %v\n", b, err)
			continue
		}
		fmt.Printf("Cleaned up merged branch %s (PR #%d).\n", b, prOf[b])
	}
	return nil
}
