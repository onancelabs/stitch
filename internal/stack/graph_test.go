package stack

import "testing"

func TestStackBaseAndPartition(t *testing.T) {
	g := &Graph{
		Trunk: "main",
		Meta: map[string]*BranchMeta{
			"a1": {Parent: "main", ParentRev: "ra"},
			"b1": {Parent: "main", ParentRev: "rb1"},
			"b2": {Parent: "b1", ParentRev: "rb2"},
		},
		Order: []string{"a1", "b1", "b2"},
	}

	if got := g.StackBase("b2"); got != "b1" {
		t.Errorf("StackBase(b2) = %q, want b1", got)
	}
	if got := g.StackBase("a1"); got != "a1" {
		t.Errorf("StackBase(a1) = %q, want a1", got)
	}
	if got := g.StackBase("main"); got != "" {
		t.Errorf("StackBase(main) = %q, want empty", got)
	}
	if got := g.StackBase("feature-x"); got != "" {
		t.Errorf("StackBase of an untracked branch = %q, want empty", got)
	}

	// Current = b2 → the b stack (b1,b2) is current; the a stack is other.
	others, current := g.PartitionForSync("b2")
	if len(current) != 2 || current[0].Branch != "b1" || current[1].Branch != "b2" {
		t.Fatalf("current ops = %+v, want b1 then b2", current)
	}
	if current[0].Parent != "main" || current[0].OldBase != "rb1" {
		t.Errorf("current[0] op fields wrong: %+v", current[0])
	}
	if current[1].Parent != "b1" || current[1].OldBase != "rb2" {
		t.Errorf("current[1] op fields wrong: %+v", current[1])
	}
	if len(others) != 1 || others[0].Base != "a1" ||
		len(others[0].Ops) != 1 || others[0].Ops[0].Branch != "a1" {
		t.Fatalf("others = %+v, want one stack base a1 with op a1", others)
	}

	// On trunk → everything is "other", nothing current, first-seen order.
	others, current = g.PartitionForSync("main")
	if current != nil {
		t.Errorf("current ops on trunk = %+v, want nil", current)
	}
	if len(others) != 2 || others[0].Base != "a1" || others[1].Base != "b1" {
		t.Fatalf("others on trunk = %+v, want stacks a1 then b1", others)
	}
	if len(others[1].Ops) != 2 {
		t.Errorf("b stack should carry both b1 and b2, got %+v", others[1].Ops)
	}
}
