package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/onancelabs/stitch/internal/gitx"
	"github.com/onancelabs/stitch/internal/stack"
)

func newRestackCmd() *cobra.Command {
	var all bool
	cmd := &cobra.Command{
		Use:     "restack",
		Aliases: []string{"r"},
		Short:   "Rebase the current stack so each branch sits on its parent",
		Args:    cobra.NoArgs,
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
			cur, err := gitx.CurrentBranch()
			if err != nil {
				return err
			}
			g, err := stack.BuildGraph()
			if err != nil {
				return err
			}
			targets := map[string]bool{}
			if all {
				for _, b := range g.Order {
					targets[b] = true
				}
			} else {
				for _, b := range g.Ancestors(cur) {
					targets[b] = true
				}
				for _, b := range g.Descendants(cur) {
					targets[b] = true
				}
			}
			if len(targets) == 0 {
				fmt.Println("Nothing to restack.")
				return nil
			}
			return stack.ExecuteRestack(stack.BuildPlan(g, targets, cur))
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "restack every tracked branch, not just the current stack")
	return cmd
}

func newContinueCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "continue",
		Short: "Resume a restack after resolving conflicts",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := gitx.RequireRepo(); err != nil {
				return err
			}
			return stack.ContinueRestack()
		},
	}
}

func newAbortCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "abort",
		Short: "Abort an in-progress restack and return to the start branch",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := gitx.RequireRepo(); err != nil {
				return err
			}
			return stack.AbortRestack()
		},
	}
}
