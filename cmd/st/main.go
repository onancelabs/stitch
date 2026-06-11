// Command st (the Stitch project) is a small CLI for working with stacked
// branches on top of plain git. See internal/cli for the command tree;
// subcommands st doesn't recognise are passed straight through to git.
package main

import (
	"os"

	"github.com/onancelabs/stitch/internal/cli"
)

// version is overridden at build time via -ldflags "-X main.version=...".
var version = "0.1.0"

func main() {
	os.Exit(cli.Run(version, os.Args[1:]))
}
