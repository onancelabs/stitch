package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/onancelabs/stitch/internal/gitx"
	"github.com/onancelabs/stitch/internal/stack"
)

func newCreateCmd() *cobra.Command {
	var msg string
	var all bool
	cmd := &cobra.Command{
		Use:     "create <branch>",
		Aliases: []string{"c"},
		Short:   "Create a new branch stacked on the current one",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if err := gitx.ValidBranchName(name); err != nil {
				return err
			}
			if err := gitx.RequireRepo(); err != nil {
				return err
			}
			if gitx.BranchExists(name) {
				return fmt.Errorf("branch %q already exists", name)
			}
			parent, err := gitx.CurrentBranch()
			if err != nil {
				return err
			}
			parentRev, err := gitx.RevParse(parent)
			if err != nil {
				return err
			}
			if all {
				if _, err := gitx.Run("add", "-A"); err != nil {
					return err
				}
			}
			if _, err := gitx.Run("checkout", "-b", name); err != nil {
				return err
			}
			if gitx.HasStaged() {
				cargs := []string{"commit"}
				if msg != "" {
					cargs = append(cargs, "-m", msg)
				}
				if err := gitx.RunIO(cargs...); err != nil {
					// Roll back the branch if the commit was aborted.
					_, _ = gitx.Run("checkout", parent)
					_, _ = gitx.Run("branch", "-D", name)
					return fmt.Errorf("commit failed: %v", err)
				}
			} else {
				fmt.Printf("No staged changes; created %q with no commit yet.\n", name)
			}
			if err := stack.WriteMeta(name, &stack.BranchMeta{Parent: parent, ParentRev: parentRev}); err != nil {
				return err
			}
			fmt.Printf("Created %s (stacked on %s).\n", name, parent)
			return nil
		},
	}
	cmd.Flags().StringVarP(&msg, "message", "m", "", "commit message")
	cmd.Flags().BoolVarP(&all, "all", "a", false, "stage all changes before committing")
	return cmd
}
