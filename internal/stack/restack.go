package stack

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/onancelabs/stitch/internal/gitx"
)

// Op is a single branch rebase in a restack plan.
//
// The rebase performed is:  git rebase --onto <NewBase> <OldBase> <Branch>
// where OldBase is the parent revision the branch was previously based on
// (captured up front from metadata) and NewBase is the parent's tip at the
// moment we rebase (computed live, so chained restacks see the updated parent).
type Op struct {
	Branch  string `json:"branch"`
	Parent  string `json:"parent"`
	OldBase string `json:"oldBase"`
	NewBase string `json:"newBase,omitempty"`
}

// State is persisted to .git/stitch/restack.json so a restack
// interrupted by a conflict can be resumed with `st continue`.
type State struct {
	Ops    []Op   `json:"ops"`    // ops[0] is current; remaining follow
	Return string `json:"return"` // branch to check out when finished
}

func statePath() (string, error) {
	return gitx.Path("stitch/restack.json")
}

func saveState(s *State) error {
	p, err := statePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, b, 0o644)
}

func LoadState() (*State, error) {
	p, err := statePath()
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var s State
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func clearState() error {
	p, err := statePath()
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// RebaseInProgress reports whether git has a paused rebase (e.g. a conflict).
func RebaseInProgress() bool {
	for _, d := range []string{"rebase-merge", "rebase-apply"} {
		if p, err := gitx.Path(d); err == nil {
			if fi, err := os.Stat(p); err == nil && fi.IsDir() {
				return true
			}
		}
	}
	return false
}

// BuildPlan creates an ordered restack plan for the given target branches,
// using the global topological order so parents are always restacked first.
func BuildPlan(g *Graph, targets map[string]bool, returnBranch string) *State {
	var ops []Op
	for _, b := range g.Order {
		if !targets[b] {
			continue
		}
		m := g.Meta[b]
		ops = append(ops, Op{Branch: b, Parent: m.Parent, OldBase: m.ParentRev})
	}
	return &State{Ops: ops, Return: returnBranch}
}

// ExecuteRestack runs the plan, persisting progress before each risky step so a
// conflict can be resumed. On conflict it returns a helpful error.
func ExecuteRestack(s *State) error {
	if err := saveState(s); err != nil {
		return err
	}
	for len(s.Ops) > 0 {
		o := &s.Ops[0]
		newBase, err := gitx.RevParse(o.Parent)
		if err != nil {
			return err
		}
		if newBase == o.OldBase {
			// Parent hasn't moved relative to the stored base: nothing to replay.
			if err := finalize(o.Branch, o.OldBase); err != nil {
				return err
			}
			s.Ops = s.Ops[1:]
			if err := saveState(s); err != nil {
				return err
			}
			continue
		}
		fmt.Printf("Restacking %s onto %s...\n", o.Branch, o.Parent)
		o.NewBase = newBase
		if err := saveState(s); err != nil { // persist NewBase before the rebase
			return err
		}
		if rerr := gitx.RunIO("rebase", "--onto", newBase, o.OldBase, o.Branch); rerr != nil {
			if RebaseInProgress() {
				return conflictError(o.Branch)
			}
			return fmt.Errorf("failed to restack %s: %v", o.Branch, rerr)
		}
		if err := finalize(o.Branch, newBase); err != nil {
			return err
		}
		s.Ops = s.Ops[1:]
		if err := saveState(s); err != nil {
			return err
		}
	}
	ret := s.Return
	if err := clearState(); err != nil {
		return err
	}
	if ret != "" {
		_, _ = gitx.Run("checkout", ret) // best effort: return to where we started
	}
	fmt.Println("Restack complete.")
	return nil
}

// finalize records that a branch is now based on newBase.
func finalize(branch, newBase string) error {
	m, err := ReadMeta(branch)
	if err != nil {
		return err
	}
	m.ParentRev = newBase
	return WriteMeta(branch, m)
}

func conflictError(branch string) error {
	return fmt.Errorf(`restack hit a conflict while replaying %s.
Resolve the conflicted files, run 'git add' on them, then:
    st continue
To cancel the entire restack and return to where you started:
    st abort`, branch)
}

// ContinueRestack resumes after the user resolves a conflict.
func ContinueRestack() error {
	s, err := LoadState()
	if err != nil {
		return err
	}
	if s == nil || len(s.Ops) == 0 {
		return fmt.Errorf("no restack in progress")
	}
	if RebaseInProgress() {
		if err := gitx.RunIO("rebase", "--continue"); err != nil {
			if RebaseInProgress() {
				return fmt.Errorf("still conflicted; resolve, 'git add', then run 'st continue'")
			}
			return err
		}
	}
	// The current op's rebase has finished; record its new base.
	o := s.Ops[0]
	nb := o.NewBase
	if nb == "" {
		if nb, err = gitx.RevParse(o.Parent); err != nil {
			return err
		}
	}
	if err := finalize(o.Branch, nb); err != nil {
		return err
	}
	s.Ops = s.Ops[1:]
	return ExecuteRestack(s)
}

// AbortRestack cancels an in-progress restack and returns to the start branch.
func AbortRestack() error {
	if RebaseInProgress() {
		_ = gitx.RunIO("rebase", "--abort")
	}
	s, _ := LoadState()
	ret := ""
	if s != nil {
		ret = s.Return
	}
	if err := clearState(); err != nil {
		return err
	}
	if ret != "" {
		_, _ = gitx.Run("checkout", ret)
	}
	fmt.Println("Restack aborted.")
	return nil
}
