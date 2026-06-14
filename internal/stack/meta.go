// Package stack is the stitch engine: per-branch metadata stored inside git,
// the in-memory stack graph, and the restack (rebase --onto) machinery.
package stack

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/onancelabs/stitch/internal/gitx"
)

// BranchMeta is the per-branch state stitch tracks. It is stored as a git
// blob pointed to by the ref refs/stitch/<branch>, which is durable (survives
// gc), invisible to the working tree, and never treated as a real branch.
type BranchMeta struct {
	Parent    string `json:"parent"`    // parent branch name
	ParentRev string `json:"parentRev"` // parent tip this branch was last based on (the rebase "old base")
	PR        int    `json:"pr,omitempty"`
	PRState   string `json:"prState,omitempty"` // last seen PR state: draft|open|closed|merged
}

func metaRef(branch string) string { return "refs/stitch/" + branch }

// IsTracked reports whether the branch has stitch metadata.
func IsTracked(branch string) bool {
	return gitx.OK("rev-parse", "--verify", "--quiet", metaRef(branch))
}

func ReadMeta(branch string) (*BranchMeta, error) {
	out, err := gitx.Run("cat-file", "-p", metaRef(branch))
	if err != nil {
		return nil, fmt.Errorf("branch %q is not tracked (run 'st track')", branch)
	}
	var m BranchMeta
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		return nil, fmt.Errorf("corrupt stitch metadata for %q: %v", branch, err)
	}
	return &m, nil
}

func WriteMeta(branch string, m *BranchMeta) error {
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	sha, err := gitx.RunInput(string(b), "hash-object", "-w", "--stdin")
	if err != nil {
		return err
	}
	_, err = gitx.Run("update-ref", metaRef(branch), sha)
	return err
}

func DeleteMeta(branch string) error {
	_, err := gitx.Run("update-ref", "-d", metaRef(branch))
	return err
}

// ListTracked returns the names of all branches that have stitch metadata.
func ListTracked() ([]string, error) {
	out, err := gitx.Run("for-each-ref", "--format=%(refname)", "refs/stitch/")
	if err != nil {
		return nil, err
	}
	var names []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		names = append(names, strings.TrimPrefix(line, "refs/stitch/"))
	}
	return names, nil
}
