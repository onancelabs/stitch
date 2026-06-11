package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/onancelabs/stitch/internal/gitx"
	"github.com/onancelabs/stitch/internal/migrate"
)

func newInitCmd() *cobra.Command {
	var trunk string
	var fromGraphite bool
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Configure the trunk branch for this repository",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if fromGraphite {
				return migrate.FromGraphite()
			}
			if err := gitx.RequireRepo(); err != nil {
				return err
			}
			name := trunk
			if name == "" {
				for _, c := range []string{"main", "master"} {
					if gitx.BranchExists(c) {
						name = c
						break
					}
				}
				if name == "" {
					c, err := gitx.CurrentBranch()
					if err != nil {
						return err
					}
					name = c
				}
			}
			if err := gitx.SetTrunk(name); err != nil {
				return err
			}
			fmt.Printf("Stitch initialized; trunk = %s\n", name)
			return nil
		},
	}
	cmd.Flags().StringVar(&trunk, "trunk", "", "trunk branch name (default: detect main/master)")
	cmd.Flags().BoolVar(&fromGraphite, "from-graphite", false, "import an existing Graphite stack (parents, base revisions, PR numbers) into Stitch")
	return cmd
}
