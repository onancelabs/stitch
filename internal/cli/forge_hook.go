package cli

import "github.com/onancelabs/stitch/internal/forge"

// newForge is swapped by tests to inject a fake code host.
var newForge = forge.New
