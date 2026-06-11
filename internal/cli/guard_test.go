package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/onancelabs/stitch/internal/gitx"
	"github.com/onancelabs/stitch/internal/stack"
)

// A paused restack (plan file present, even with git's rebase already finished)
// must block history-rewriting commands and funnel the user to continue/abort.
func TestPausedRestackBlocksCommands(t *testing.T) {
	setupRepo(t)
	writeFile(t, "f1.txt", "1\n")
	runSt(t, "create", "-a", "-m", "c1", "b1")

	p, err := gitx.Path("stitch/restack.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, p, `{"ops":[{"branch":"b1","parent":"main","oldBase":"deadbeef"}],"return":"b1"}`)

	for _, args := range [][]string{{"restack"}, {"modify", "-a", "-m", "x"}, {"sync", "--no-fetch"}} {
		err := execStitch(args...)
		if err == nil || !strings.Contains(err.Error(), "st continue") {
			t.Errorf("st %v with a paused restack: want a 'st continue' error, got %v", args, err)
		}
	}

	runSt(t, "abort") // clears the plan
	if s, _ := stack.LoadState(); s != nil {
		t.Fatal("abort should clear the saved plan")
	}
	runSt(t, "restack") // now proceeds normally
}
