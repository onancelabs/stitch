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

// fakeForge records calls and assigns PR numbers 101, 102, ...
type fakeForge struct {
	ensures       []ensureCall
	retitles      []retitleCall
	comments      map[int]string // PR num -> last rendered comment body
	commentID     map[int]int64  // PR num -> assigned comment ID
	commentCreate map[int]int    // PR num -> number of creates (should stay 1)
	stripped      []int          // PR nums StripStackBody was called on
	states        map[int]string // PR number -> state served by PRState
	byHead        map[string]int
	nextPR        int
	nextComment   int64
}

func newFakeForge() *fakeForge {
	return &fakeForge{
		comments:      map[int]string{},
		commentID:     map[int]int64{},
		commentCreate: map[int]int{},
		states:        map[int]string{},
		byHead:        map[string]int{},
		nextPR:        100,
	}
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

func (f *fakeForge) UpsertStackComment(num int, knownID int64, body string) (int64, error) {
	f.comments[num] = body
	if id, ok := f.commentID[num]; ok {
		return id, nil // edit existing
	}
	f.nextComment++
	id := 5000 + f.nextComment
	f.commentID[num] = id
	f.commentCreate[num]++
	return id, nil
}

func (f *fakeForge) StripStackBody(num int) error {
	f.stripped = append(f.stripped, num)
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
