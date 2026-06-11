package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/onancelabs/stitch/internal/gitx"
	"github.com/onancelabs/stitch/internal/stack"
)

func newModifyCmd() *cobra.Command {
	var msg string
	var all, newCommit bool
	cmd := &cobra.Command{
		Use:     "modify",
		Aliases: []string{"m"},
		Short:   "Amend the current branch's commit, then restack its children",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := gitx.RequireRepo(); err != nil {
				return err
			}
			if err := ensureNoPausedRestack(); err != nil {
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
				return fmt.Errorf("refusing to modify trunk (%s)", trunk)
			}
			// Guard: amending when the branch has no commit of its own would
			// rewrite the parent's commit. Require an explicit new commit.
			if !newCommit && stack.IsTracked(cur) {
				m, err := stack.ReadMeta(cur)
				if err != nil {
					return err
				}
				headSha, err := gitx.RevParse("HEAD")
				if err != nil {
					return err
				}
				parentTip, err := gitx.RevParse(m.Parent)
				if err != nil {
					return err
				}
				if headSha == parentTip {
					return fmt.Errorf("%s has no commit of its own to amend; use 'st modify -c' to add one", cur)
				}
			}
			if all {
				if _, err := gitx.Run("add", "-A"); err != nil {
					return err
				}
			}
			if newCommit {
				if !gitx.HasStaged() {
					return fmt.Errorf("no staged changes to commit")
				}
				cargs := []string{"commit"}
				if msg != "" {
					cargs = append(cargs, "-m", msg)
				}
				if err := gitx.RunIO(cargs...); err != nil {
					return err
				}
			} else {
				cargs := []string{"commit", "--amend"}
				if msg != "" {
					cargs = append(cargs, "-m", msg)
				} else {
					cargs = append(cargs, "--no-edit")
				}
				if err := gitx.RunIO(cargs...); err != nil {
					return err
				}
			}
			// Restack everything stacked above this branch.
			g, err := stack.BuildGraph()
			if err != nil {
				return err
			}
			desc := g.Descendants(cur)
			if len(desc) == 0 {
				return nil
			}
			clean, err := gitx.IsClean()
			if err != nil {
				return err
			}
			if !clean {
				return fmt.Errorf("commit left the tree dirty; resolve before children can be restacked")
			}
			targets := map[string]bool{}
			for _, b := range desc {
				targets[b] = true
			}
			return stack.ExecuteRestack(stack.BuildPlan(g, targets, cur))
		},
	}
	cmd.Flags().StringVarP(&msg, "message", "m", "", "set/replace the commit message")
	cmd.Flags().BoolVarP(&all, "all", "a", false, "stage all changes first")
	cmd.Flags().BoolVarP(&newCommit, "commit", "c", false, "create a new commit instead of amending")
	return cmd
}
