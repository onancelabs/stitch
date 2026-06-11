package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/onancelabs/stitch/internal/auth"
	"github.com/onancelabs/stitch/internal/browser"
)

func newAuthCmd() *cobra.Command {
	var token string
	var logout, status bool
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Sign in to GitHub (OAuth device flow) or store a token",
		Long: "Authorizes stitch with GitHub for 'st submit' and 'st sync'. By default " +
			"this runs GitHub's device flow: you get a one-time code, confirm it in " +
			"your browser, and the resulting token is stored in your OS keychain.\n\n" +
			"To use a personal access token instead, pass --token (the token needs " +
			"Pull requests: read/write and Contents: read/write, or the classic " +
			"'repo' scope). GITHUB_TOKEN / GH_TOKEN environment variables are also " +
			"honored at use time without storing anything.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if logout {
				if err := auth.Delete(auth.DefaultHost); err != nil {
					return fmt.Errorf("could not remove token: %v", err)
				}
				fmt.Println("Removed the stored GitHub token.")
				return nil
			}
			if status {
				if t, err := auth.ResolveToken(auth.DefaultHost); err == nil && t != "" {
					fmt.Println("A GitHub token is configured.")
				} else {
					fmt.Println("No GitHub token configured.")
				}
				return nil
			}
			if token != "" {
				if err := auth.Store(auth.DefaultHost, token); err != nil {
					return fmt.Errorf("could not store token in keychain: %v", err)
				}
				fmt.Println("Token stored in your OS keychain.")
				return nil
			}
			cid := strings.TrimSpace(os.Getenv("STITCH_GITHUB_CLIENT_ID"))
			if cid == "" {
				cid = auth.GitHubClientID
			}
			if cid == "" {
				return fmt.Errorf("no OAuth client id is configured for the device flow.\n" +
					"Either set STITCH_GITHUB_CLIENT_ID (a GitHub OAuth App with Device Flow enabled),\n" +
					"or store a personal access token directly:  st auth --token <token>")
			}
			tok, err := auth.DeviceFlow(cid, browser.Open, cmd.InOrStdin(), cmd.OutOrStdout())
			if err != nil {
				return fmt.Errorf("%v\n(You can always use a PAT instead:  st auth --token <token>)", err)
			}
			if err := auth.Store(auth.DefaultHost, tok); err != nil {
				return fmt.Errorf("could not store token in keychain: %v", err)
			}
			fmt.Println("Token stored in your OS keychain.")
			return nil
		},
	}
	cmd.Flags().StringVar(&token, "token", "", "store this personal access token instead of running the device flow")
	cmd.Flags().BoolVar(&logout, "logout", false, "remove the stored token")
	cmd.Flags().BoolVar(&status, "status", false, "report whether a token is configured")
	return cmd
}
