package cli

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/onancelabs/stitch/internal/gitx"
	"github.com/onancelabs/stitch/internal/stack"
)

func newDownCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "down",
		Aliases: []string{"d"},
		Short:   "Check out the parent branch (toward trunk)",
		Args:    cobra.NoArgs,
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
				fmt.Println("Already at trunk.")
				return nil
			}
			if !stack.IsTracked(cur) {
				return fmt.Errorf("branch %q is not tracked; run 'st track'", cur)
			}
			m, err := stack.ReadMeta(cur)
			if err != nil {
				return err
			}
			return gitx.RunIO("checkout", m.Parent)
		},
	}
}

func newUpCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "up [child]",
		Aliases: []string{"u"},
		Short:   "Check out a child branch (away from trunk)",
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := gitx.RequireRepo(); err != nil {
				return err
			}
			cur, err := gitx.CurrentBranch()
			if err != nil {
				return err
			}
			g, err := stack.BuildGraph()
			if err != nil {
				return err
			}
			kids := g.Children[cur]
			if len(kids) == 0 {
				fmt.Println("Already at the top of the stack.")
				return nil
			}
			var target string
			if len(kids) == 1 {
				target = kids[0]
			} else if len(args) == 1 {
				for _, k := range kids {
					if k == args[0] {
						target = k
					}
				}
				if target == "" {
					return fmt.Errorf("%q is not a child of %s", args[0], cur)
				}
			} else {
				target, err = chooseBranch(cur, kids)
				if err != nil {
					return err
				}
			}
			return gitx.RunIO("checkout", target)
		},
	}
}

func chooseBranch(parent string, kids []string) (string, error) {
	fmt.Printf("%s has multiple children:\n", parent)
	for i, k := range kids {
		fmt.Printf("  [%d] %s\n", i+1, k)
	}
	fmt.Print("Select a branch number: ")
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return "", err
	}
	idx, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil || idx < 1 || idx > len(kids) {
		return "", fmt.Errorf("invalid selection")
	}
	return kids[idx-1], nil
}
