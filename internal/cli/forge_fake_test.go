package cli

import (
	"fmt"

	"github.com/onancelabs/stitch/internal/forge"
)

type ensureCall struct {
	branch, base, title, body string
	draft                     bool
	knownPR                   int
}

type retitleCall struct {
	num   int
	title string
}

type bodyCall struct {
	num     int
	entries []forge.StackEntry
	current string
}

// fakeForge records calls and assigns PR numbers 101, 102, ...
type fakeForge struct {
	ensures  []ensureCall
	retitles []retitleCall
	bodies   []bodyCall
	states   map[int]string // PR number -> state served by PRState
	byHead   map[string]int
	nextPR   int
}

func newFakeForge() *fakeForge {
	return &fakeForge{states: map[int]string{}, byHead: map[string]int{}, nextPR: 100}
}

func (f *fakeForge) EnsurePR(branch, base, title, body string, draft bool, knownPR int) (int, string, string, error) {
	f.ensures = append(f.ensures, ensureCall{branch, base, title, body, draft, knownPR})
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
	state := "open"
	if draft {
		state = "draft"
	}
	if s, ok := f.states[num]; ok {
		state = s
	}
	return num, fmt.Sprintf("https://example.test/pr/%d", num), state, nil
}

func (f *fakeForge) SetPRTitle(num int, title string) error {
	f.retitles = append(f.retitles, retitleCall{num, title})
	return nil
}

func (f *fakeForge) UpdatePRBody(num int, entries []forge.StackEntry, current string) error {
	f.bodies = append(f.bodies, bodyCall{num, entries, current})
	return nil
}

func (f *fakeForge) PRState(num int) (string, error) {
	if s, ok := f.states[num]; ok {
		return s, nil
	}
	return "open", nil
}

// installFakeForge swaps the constructor hook for one test.
func installFakeForge(t interface{ Cleanup(func()) }) *fakeForge {
	fk := newFakeForge()
	old := newForge
	newForge = func(remoteURL, token string) (forge.Forge, error) { return fk, nil }
	t.Cleanup(func() { newForge = old })
	return fk
}
