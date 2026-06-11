package cli

import (
	"fmt"

	"github.com/onancelabs/stitch/internal/stack"
)

// ensureNoPausedRestack refuses to start history-rewriting work while a rebase
// is live or a saved restack plan exists. The plan can outlive git's own rebase
// state when the user runs `git rebase --continue` themselves; `st continue`
// recovers that case correctly, so send them there.
func ensureNoPausedRestack() error {
	if stack.RebaseInProgress() {
		return fmt.Errorf("a rebase is in progress; run 'st continue' or 'st abort'")
	}
	if s, err := stack.LoadState(); err == nil && s != nil && len(s.Ops) > 0 {
		return fmt.Errorf("a restack is paused; run 'st continue' or 'st abort'")
	}
	return nil
}
