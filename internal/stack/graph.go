package stack

import (
	"fmt"
	"sort"

	"github.com/onancelabs/stitch/internal/gitx"
)

// Graph is the in-memory stack model built from branch metadata.
type Graph struct {
	Trunk    string
	Meta     map[string]*BranchMeta // tracked branch -> metadata
	Children map[string][]string    // branch -> sorted child branches
	Order    []string               // tracked branches, parents before children
}

func BuildGraph() (*Graph, error) {
	trunk, err := gitx.TrunkName()
	if err != nil {
		return nil, err
	}
	tracked, err := ListTracked()
	if err != nil {
		return nil, err
	}
	g := &Graph{Trunk: trunk, Meta: map[string]*BranchMeta{}, Children: map[string][]string{}}
	for _, b := range tracked {
		m, err := ReadMeta(b)
		if err != nil {
			return nil, err
		}
		g.Meta[b] = m
	}
	// Drop orphaned metadata whose underlying git branch no longer exists, so a
	// branch deleted with plain git doesn't wedge every command.
	for b := range g.Meta {
		if !gitx.BranchExists(b) {
			delete(g.Meta, b)
		}
	}
	for b, m := range g.Meta {
		g.Children[m.Parent] = append(g.Children[m.Parent], b)
	}
	for k := range g.Children {
		sort.Strings(g.Children[k])
	}
	order, err := g.topo()
	if err != nil {
		return nil, err
	}
	g.Order = order
	return g, nil
}

// topo returns tracked branches in an order where every parent precedes its
// children, and reports cycles. Roots are trunk plus any parent that is not
// itself a tracked branch (so stacks rooted off a non-trunk branch still work).
func (g *Graph) topo() ([]string, error) {
	rootset := map[string]bool{g.Trunk: true}
	for _, m := range g.Meta {
		if _, ok := g.Meta[m.Parent]; !ok {
			rootset[m.Parent] = true
		}
	}
	var roots []string
	for r := range rootset {
		roots = append(roots, r)
	}
	sort.Strings(roots)

	visited := map[string]bool{}
	var order []string
	queue := append([]string{}, roots...)
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		for _, c := range g.Children[n] {
			if visited[c] {
				return nil, fmt.Errorf("cycle detected in stack metadata at %q", c)
			}
			visited[c] = true
			order = append(order, c)
			queue = append(queue, c)
		}
	}
	for b := range g.Meta {
		if !visited[b] {
			return nil, fmt.Errorf("cycle detected in stack metadata involving %q", b)
		}
	}
	return order, nil
}

// Ancestors returns branch plus its tracked ancestors, nearest first, stopping
// at the first non-tracked parent (trunk or an external root).
func (g *Graph) Ancestors(branch string) []string {
	var out []string
	cur := branch
	for {
		m, ok := g.Meta[cur]
		if !ok {
			break
		}
		out = append(out, cur)
		cur = m.Parent
	}
	return out
}

// Descendants returns all branches stacked above branch (pre-order).
func (g *Graph) Descendants(branch string) []string {
	var out []string
	var walk func(n string)
	walk = func(n string) {
		for _, c := range g.Children[n] {
			out = append(out, c)
			walk(c)
		}
	}
	walk(branch)
	return out
}

// NeedsRestack reports whether a tracked branch is no longer sitting on its
// parent's current tip.
func (g *Graph) NeedsRestack(branch string) bool {
	m, ok := g.Meta[branch]
	if !ok {
		return false
	}
	tip, err := gitx.RevParse(m.Parent)
	if err != nil {
		return false
	}
	return tip != m.ParentRev
}
