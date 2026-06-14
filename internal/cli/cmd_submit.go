package cli

import (
	"fmt"
	"slices"

	"github.com/spf13/cobra"

	"github.com/onancelabs/stitch/internal/auth"
	"github.com/onancelabs/stitch/internal/browser"
	"github.com/onancelabs/stitch/internal/forge"
	"github.com/onancelabs/stitch/internal/gitx"
	"github.com/onancelabs/stitch/internal/stack"
)

func newSubmitCmd() *cobra.Command {
	var ready, noOpen bool
	var title string
	cmd := &cobra.Command{
		Use:     "submit",
		Aliases: []string{"s"},
		Short:   "Push the current stack and open/update a PR per branch",
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
				return fmt.Errorf("check out a branch in your stack, not trunk (%s)", trunk)
			}
			g, err := stack.BuildGraph()
			if err != nil {
				return err
			}
			// The current stack, bottom-up (trunk's child first).
			targets := map[string]bool{}
			for _, b := range g.Ancestors(cur) {
				targets[b] = true
			}
			for _, b := range g.Descendants(cur) {
				targets[b] = true
			}
			var order []string
			for _, b := range g.Order {
				if targets[b] {
					order = append(order, b)
				}
			}
			if len(order) == 0 {
				return fmt.Errorf("no tracked branches in the current stack; use 'st create' or 'st track'")
			}
			if title != "" && !slices.Contains(order, cur) {
				return fmt.Errorf("--title applies to the current branch (%s), which has no PR in this submit", cur)
			}
			for _, b := range order {
				if g.NeedsRestack(b) {
					return fmt.Errorf("%s needs restacking; run 'st restack' first", b)
				}
			}

			originURL, err := gitx.Run("remote", "get-url", "origin")
			if err != nil {
				return fmt.Errorf("no 'origin' remote: %v", err)
			}
			token, err := auth.ResolveToken(auth.DefaultHost)
			if err != nil {
				return err
			}
			f, err := newForge(originURL, token)
			if err != nil {
				return err
			}

			// 1. Push every branch (force-with-lease, since restacks rewrite
			// history) so all PR base branches exist on the remote.
			for _, b := range order {
				fmt.Printf("Pushing %s...\n", b)
				if err := gitx.RunIO("push", "--force-with-lease", "origin", b+":"+b); err != nil {
					return fmt.Errorf("push %s failed: %v", b, err)
				}
			}

			// 2. Ensure a PR per branch, base = its parent branch.
			prNum := map[string]int{}
			prURL := map[string]string{}
			for _, b := range order {
				base := g.Meta[b].Parent
				t, body := gitx.FirstCommit(base, b)
				if b == cur && title != "" {
					t = title
				}
				if t == "" {
					t = b
				}
				hadPR := g.Meta[b].PR != 0
				num, url, state, err := f.EnsurePR(b, base, t, body, !ready, g.Meta[b].PR)
				if err != nil {
					return fmt.Errorf("PR for %s failed: %v", b, err)
				}
				if b == cur && title != "" && hadPR {
					if err := f.SetPRTitle(num, title); err != nil {
						return fmt.Errorf("retitling PR #%d failed: %v", num, err)
					}
				}
				prNum[b], prURL[b] = num, url
				if m := g.Meta[b]; m.PR != num || m.PRState != state {
					m.PR, m.PRState = num, state
					if err := stack.WriteMeta(b, m); err != nil {
						return err
					}
				}
			}

			// 3. Write the stack map into each PR (top of stack first).
			var entries []forge.StackEntry
			for i := len(order) - 1; i >= 0; i-- {
				entries = append(entries, forge.StackEntry{Branch: order[i], PR: prNum[order[i]]})
			}
			for _, b := range order {
				if err := f.UpdatePRBody(prNum[b], entries, b); err != nil {
					return fmt.Errorf("updating PR #%d failed: %v", prNum[b], err)
				}
			}

			fmt.Println("\nSubmitted stack:")
			for i := len(order) - 1; i >= 0; i-- {
				b := order[i]
				fmt.Printf("  #%d  %s  %s\n", prNum[b], b, prURL[b])
			}

			// Open the PR for the top of the stack in a browser.
			if !noOpen {
				top := order[len(order)-1]
				if url := prURL[top]; url != "" {
					fmt.Printf("\nOpening %s\n", url)
					if err := browser.Open(url); err != nil {
						fmt.Printf("(could not open browser: %v)\n", err)
					}
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVarP(&ready, "ready", "r", false, "open PRs for review instead of as drafts")
	cmd.Flags().BoolVar(&noOpen, "no-open", false, "don't open the top PR in a browser")
	cmd.Flags().StringVarP(&title, "title", "t", "", "title for the current branch's PR (created or retitled)")
	return cmd
}
