package cli

import (
	"fmt"

	"github.com/onancelabs/stitch/internal/forge"
)

type ensureCall struct {
	branch, base, title string
	draft               bool
	knownPR             int
}

type bodyCall struct {
	num     int
	entries []forge.StackEntry
	current string
}

// fakeForge records calls and assigns PR numbers 101, 102, ...
type fakeForge struct {
	ensures []ensureCall
	bodies  []bodyCall
	merged  map[int]bool
	byHead  map[string]int
	nextPR  int
}

func newFakeForge() *fakeForge {
	return &fakeForge{merged: map[int]bool{}, byHead: map[string]int{}, nextPR: 100}
}

func (f *fakeForge) EnsurePR(branch, base, title string, draft bool, knownPR int) (int, string, error) {
	f.ensures = append(f.ensures, ensureCall{branch, base, title, draft, knownPR})
	num := knownPR
	if num == 0 {
		if n, ok := f.byHead[branch]; ok {
			num = n
		} else {
			f.nextPR++
			num = f.nextPR
			f.byHead[branch] = num
		}
	}
	return num, fmt.Sprintf("https://example.test/pr/%d", num), nil
}

func (f *fakeForge) UpdatePRBody(num int, entries []forge.StackEntry, current string) error {
	f.bodies = append(f.bodies, bodyCall{num, entries, current})
	return nil
}

func (f *fakeForge) PRMerged(num int) (bool, error) { return f.merged[num], nil }

// installFakeForge swaps the constructor hook for one test.
func installFakeForge(t interface{ Cleanup(func()) }) *fakeForge {
	fk := newFakeForge()
	old := newForge
	newForge = func(remoteURL, token string) (forge.Forge, error) { return fk, nil }
	t.Cleanup(func() { newForge = old })
	return fk
}
