package cli

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/onancelabs/stitch/internal/gitx"
	"github.com/onancelabs/stitch/internal/stack"
)

// TestMigrateFromGraphiteEndToEnd fabricates a repo in the exact current-Graphite
// format (SQLite branch_metadata + .graphite_repo_config + .graphite_pr_info)
// and runs the real `st init --from-graphite`. Requires the sqlite3 CLI.
func TestMigrateFromGraphiteEndToEnd(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 not installed; skipping Graphite migration e2e")
	}
	setupRepo(t)
	// Build the stack with PLAIN git (no stitch metadata yet): main <- s1 <- s2.
	mustGit(t, "checkout", "-b", "s1")
	writeFile(t, "a.txt", "a\n")
	mustGit(t, "add", "-A")
	mustGit(t, "commit", "-qm", "a")
	mustGit(t, "checkout", "-b", "s2")
	writeFile(t, "b.txt", "b\n")
	mustGit(t, "add", "-A")
	mustGit(t, "commit", "-qm", "b")

	mainSHA := mustGit(t, "rev-parse", "main")
	s1SHA := mustGit(t, "rev-parse", "s1")

	// Fabricate Graphite's current metadata files.
	sql := `CREATE TABLE "branch_metadata" ("branch_name" text not null primary key, "parent_branch_name" text, "parent_branch_revision" text, "last_submitted_version" text, "state" text, "children" text, "branch_revision" text, "validation_result" text, "parent_head_revision" text);
INSERT INTO branch_metadata(branch_name,parent_branch_name,parent_branch_revision) VALUES ('main',NULL,NULL),('s1','main','` + mainSHA + `'),('s2','s1','` + s1SHA + `'),('stale','main','` + strings.Repeat("d", 40) + `');`
	if out, err := exec.Command("sqlite3", ".git/.graphite_metadata.db", sql).CombinedOutput(); err != nil {
		t.Fatalf("sqlite3 create failed: %v: %s", err, out)
	}
	writeFile(t, ".git/.graphite_repo_config", `{"trunk":"main","trunks":[{"name":"main"}]}`)
	writeFile(t, ".git/.graphite_pr_info", `{"prInfos":[{"headRefName":"s1","prNumber":143}]}`)

	runSt(t, "init", "--from-graphite")

	m1, err := stack.ReadMeta("s1")
	if err != nil || m1.Parent != "main" || m1.ParentRev != mainSHA || m1.PR != 143 {
		t.Errorf("s1 migrated wrong: %+v err=%v", m1, err)
	}
	m2, err := stack.ReadMeta("s2")
	if err != nil || m2.Parent != "s1" || m2.ParentRev != s1SHA {
		t.Errorf("s2 migrated wrong: %+v err=%v", m2, err)
	}
	if stack.IsTracked("stale") {
		t.Error("stale branch (no local ref) should not have been migrated")
	}
	if tr, _ := gitx.TrunkName(); tr != "main" {
		t.Errorf("trunk should be main, got %q", tr)
	}
	if _, err := os.Stat(".git/.graphite_metadata.db"); err != nil {
		t.Error("migration must not delete Graphite's database")
	}
}
