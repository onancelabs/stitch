// Package cli is the st command tree: one file per command, built on cobra.
// A "stack" is a chain (or tree) of branches where each branch is one
// reviewable change and remembers its parent branch plus the parent revision
// it was based on. Everything else is git plumbing.
package cli

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
)

// NewRootCmd builds the full command tree. It is also used by tests so each run
// starts from a clean set of flag values.
func NewRootCmd(version string) *cobra.Command {
	root := &cobra.Command{
		Use:           "st",
		Short:         "Stitch — stacked branches on top of git",
		Version:       version,
		SilenceUsage:  true, // don't dump usage text on ordinary errors
		SilenceErrors: true, // Run prints errors itself, with a "st:" prefix
	}
	root.AddCommand(
		newInitCmd(),
		newCreateCmd(),
		newModifyCmd(),
		newRestackCmd(),
		newContinueCmd(),
		newAbortCmd(),
		newUpCmd(),
		newDownCmd(),
		newTrackCmd(),
		newUntrackCmd(),
		newLogCmd(),
		newSyncCmd(),
		newSubmitCmd(),
		newAuthCmd(),
	)
	return root
}

// Run dispatches a command line: recognised subcommands go to cobra; anything
// else is forwarded to git, so `st rebase`, `st status`, etc. behave exactly
// like the underlying git command.
func Run(version string, args []string) int {
	root := NewRootCmd(version)
	if len(args) > 0 && !isKnownCommand(root, args[0]) {
		return gitPassthrough(args)
	}
	root.SetArgs(args)
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "st: "+err.Error())
		return 1
	}
	return 0
}

// isKnownCommand reports whether name is one of st's own subcommands (or a
// flag / cobra built-in that cobra should handle), as opposed to something we
// forward to git.
func isKnownCommand(root *cobra.Command, name string) bool {
	// Flags (--help, --version), and cobra's completion machinery (__complete…).
	if strings.HasPrefix(name, "-") || strings.HasPrefix(name, "__") {
		return true
	}
	if name == "help" || name == "completion" {
		return true
	}
	for _, c := range root.Commands() {
		if c.Name() == name {
			return true
		}
		for _, a := range c.Aliases {
			if a == name {
				return true
			}
		}
	}
	return false
}

// gitPassthrough runs `git <args...>` inheriting stdio and returns git's exit
// code.
func gitPassthrough(args []string) int {
	cmd := exec.Command("git", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return ee.ExitCode()
		}
		fmt.Fprintln(os.Stderr, "st: "+err.Error())
		return 1
	}
	return 0
}
