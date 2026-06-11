// Package migrate imports stack metadata from Graphite (the `gt` CLI).
//
// Graphite stores the same concepts stitch does — each branch's parent and the
// parent revision it was based on — so migration is a near-1:1 mapping. Current
// Graphite keeps this in a SQLite database at .git/.graphite_metadata.db (table
// `branch_metadata` with columns branch_name / parent_branch_name /
// parent_branch_revision). Older Graphite kept it as JSON blobs under
// refs/branch-metadata/<branch>. We read whichever is present and write our own
// refs/stitch/<branch> metadata, leaving Graphite's data untouched.
package migrate

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/onancelabs/stitch/internal/gitx"
	"github.com/onancelabs/stitch/internal/stack"
)

type graphiteRow struct {
	Branch    string
	Parent    string
	ParentRev string
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// isHexish reports whether s looks like a git object id (hex, reasonably long).
func isHexish(s string) bool {
	if len(s) < 12 {
		return false
	}
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

// readGraphiteTrunk returns the trunk branch recorded in .graphite_repo_config.
func readGraphiteTrunk() (string, error) {
	p, err := gitx.Path(".graphite_repo_config")
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return "", fmt.Errorf("this does not look like a Graphite repo (no .graphite_repo_config)")
	}
	var cfg struct {
		Trunk  string `json:"trunk"`
		Trunks []struct {
			Name string `json:"name"`
		} `json:"trunks"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return "", fmt.Errorf("could not parse .graphite_repo_config: %v", err)
	}
	if cfg.Trunk != "" {
		return cfg.Trunk, nil
	}
	if len(cfg.Trunks) > 0 && cfg.Trunks[0].Name != "" {
		return cfg.Trunks[0].Name, nil
	}
	return "", fmt.Errorf("no trunk recorded in .graphite_repo_config")
}

// readGraphiteStack reads Graphite's branch metadata. It prefers the current
// SQLite store (authoritative) when the sqlite3 CLI is available, then falls
// back to Graphite's JSON snapshots (which need no external tool), then to the
// legacy refs/branch-metadata blobs.
func readGraphiteStack() ([]graphiteRow, error) {
	dbPath, dberr := gitx.Path(".graphite_metadata.db")
	dbExists := dberr == nil && fileExists(dbPath)
	if dbExists {
		if _, err := exec.LookPath("sqlite3"); err == nil {
			return readGraphiteSQLite(dbPath)
		}
	}
	// Dependency-free fallback: Graphite mirrors the same parent metadata into
	// JSON snapshots under .git/.gt/snapshots, so sqlite3 isn't strictly needed.
	if rows, err := readGraphiteSnapshot(); err == nil && len(rows) > 0 {
		fmt.Println("(Reading Graphite metadata from its JSON snapshot; run 'st log' afterward to confirm.)")
		return rows, nil
	}
	if refs, _ := gitx.Run("for-each-ref", "--format=%(refname)", "refs/branch-metadata/"); refs != "" {
		return readGraphiteRefs()
	}
	if dbExists {
		return nil, fmt.Errorf("Graphite's metadata is in a SQLite database (.git/.graphite_metadata.db) with no readable JSON snapshot; install the 'sqlite3' CLI to import it (%s)", sqliteInstallHint())
	}
	return nil, fmt.Errorf("no Graphite metadata found (looked for .git/.graphite_metadata.db, .git/.gt/snapshots, and refs/branch-metadata/)")
}

// sqliteInstallHint returns an OS-appropriate way to install the sqlite3 CLI.
func sqliteInstallHint() string {
	switch runtime.GOOS {
	case "darwin":
		return "macOS ships sqlite3; if missing, run 'brew install sqlite'"
	case "windows":
		return "install from https://sqlite.org/download.html or 'winget install SQLite.SQLite'"
	default:
		return "e.g. 'sudo apt-get install sqlite3' or 'sudo dnf install sqlite'"
	}
}

// readGraphiteSnapshot reads the newest Graphite JSON snapshot under
// .git/.gt/snapshots — a dependency-free source of the same parent metadata.
func readGraphiteSnapshot() ([]graphiteRow, error) {
	dir, err := gitx.Path(".gt/snapshots")
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("no Graphite snapshots")
	}
	var newest string
	var newestT time.Time
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".snapshot") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if newest == "" || info.ModTime().After(newestT) {
			newest, newestT = filepath.Join(dir, e.Name()), info.ModTime()
		}
	}
	if newest == "" {
		return nil, fmt.Errorf("no Graphite snapshots")
	}
	data, err := os.ReadFile(newest)
	if err != nil {
		return nil, err
	}
	return parseGraphiteSnapshot(data), nil
}

// parseGraphiteSnapshot extracts (branch, parent, parentRevision) rows from a
// Graphite snapshot. The snapshot's "branches" is an array of [name, obj] pairs.
// Pure and unit tested.
func parseGraphiteSnapshot(data []byte) []graphiteRow {
	var snap struct {
		Branches [][2]json.RawMessage `json:"branches"`
	}
	if json.Unmarshal(data, &snap) != nil {
		return nil
	}
	var rows []graphiteRow
	for _, pair := range snap.Branches {
		var name string
		if json.Unmarshal(pair[0], &name) != nil {
			continue
		}
		var obj struct {
			Parent    string `json:"parentBranchName"`
			ParentRev string `json:"parentBranchRevision"`
		}
		if json.Unmarshal(pair[1], &obj) != nil || obj.Parent == "" {
			continue
		}
		rows = append(rows, graphiteRow{Branch: name, Parent: obj.Parent, ParentRev: obj.ParentRev})
	}
	return rows
}

// readGraphiteSQLite reads the branch_metadata table via the sqlite3 CLI, opened
// read-only so Graphite's database is never modified.
func readGraphiteSQLite(dbPath string) ([]graphiteRow, error) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		return nil, fmt.Errorf("'sqlite3' is required to read Graphite's metadata; install it (macOS ships it; Debian/Ubuntu: apt-get install sqlite3)")
	}
	const us = "\x1f" // unit separator: cannot appear in branch names or SHAs
	out, err := exec.Command("sqlite3", "-readonly", "-separator", us, dbPath,
		"SELECT branch_name, IFNULL(parent_branch_name,''), IFNULL(parent_branch_revision,'') "+
			"FROM branch_metadata WHERE parent_branch_name IS NOT NULL AND parent_branch_name <> ''",
	).Output()
	if err != nil {
		return nil, fmt.Errorf("reading %s: %v", dbPath, err)
	}
	var rows []graphiteRow
	for _, line := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
		if line == "" {
			continue
		}
		f := strings.SplitN(line, us, 3)
		if len(f) < 3 {
			continue
		}
		rows = append(rows, graphiteRow{Branch: f[0], Parent: f[1], ParentRev: f[2]})
	}
	return rows, nil
}

// readGraphiteRefs reads the legacy refs/branch-metadata/<branch> JSON blobs.
func readGraphiteRefs() ([]graphiteRow, error) {
	out, err := gitx.Run("for-each-ref", "--format=%(refname)", "refs/branch-metadata/")
	if err != nil {
		return nil, err
	}
	var rows []graphiteRow
	for _, ref := range strings.Split(out, "\n") {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			continue
		}
		blob, err := gitx.Run("cat-file", "-p", ref)
		if err != nil {
			continue
		}
		var m struct {
			Parent    string `json:"parentBranchName"`
			ParentRev string `json:"parentBranchRevision"`
			PrevRef   string `json:"prevRef"` // older field name for the base
		}
		if json.Unmarshal([]byte(blob), &m) != nil || m.Parent == "" {
			continue
		}
		rev := m.ParentRev
		if rev == "" {
			rev = m.PrevRef
		}
		rows = append(rows, graphiteRow{
			Branch:    strings.TrimPrefix(ref, "refs/branch-metadata/"),
			Parent:    m.Parent,
			ParentRev: rev,
		})
	}
	return rows, nil
}

// readGraphitePRNumbers maps branch -> PR number from .graphite_pr_info, if any.
func readGraphitePRNumbers() map[string]int {
	m := map[string]int{}
	p, err := gitx.Path(".graphite_pr_info")
	if err != nil {
		return m
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return m
	}
	var doc struct {
		PRInfos []struct {
			HeadRefName string `json:"headRefName"`
			PRNumber    int    `json:"prNumber"`
		} `json:"prInfos"`
	}
	if json.Unmarshal(data, &doc) != nil {
		return m
	}
	for _, pr := range doc.PRInfos {
		if pr.HeadRefName != "" && pr.PRNumber != 0 {
			m[pr.HeadRefName] = pr.PRNumber
		}
	}
	return m
}

// buildStitchMetaFromGraphite maps Graphite rows to Stitch metadata. Branches
// that no longer exist locally are skipped; a missing/!hex parent revision falls
// back to a live merge-base so the result is always restackable. Pure and unit
// tested.
func buildStitchMetaFromGraphite(
	rows []graphiteRow,
	prs map[string]int,
	exists func(string) bool,
	mergeBaseFn func(a, b string) (string, error),
) (map[string]*stack.BranchMeta, []string) {
	out := map[string]*stack.BranchMeta{}
	var skipped []string
	for _, r := range rows {
		if !exists(r.Branch) {
			skipped = append(skipped, r.Branch)
			continue
		}
		rev := r.ParentRev
		if !isHexish(rev) {
			if mb, err := mergeBaseFn(r.Branch, r.Parent); err == nil {
				rev = mb
			}
		}
		m := &stack.BranchMeta{Parent: r.Parent, ParentRev: rev}
		if n, ok := prs[r.Branch]; ok {
			m.PR = n
		}
		out[r.Branch] = m
	}
	return out, skipped
}

// FromGraphite performs `st init --from-graphite`.
func FromGraphite() error {
	if err := gitx.RequireRepo(); err != nil {
		return err
	}
	trunk, err := readGraphiteTrunk()
	if err != nil {
		return err
	}
	if !gitx.BranchExists(trunk) {
		return fmt.Errorf("Graphite trunk %q is not a local branch", trunk)
	}
	rows, err := readGraphiteStack()
	if err != nil {
		return err
	}
	metas, skipped := buildStitchMetaFromGraphite(rows, readGraphitePRNumbers(), gitx.BranchExists, gitx.MergeBase)
	if len(metas) == 0 {
		return fmt.Errorf("found no migratable Graphite branches (all merged or missing locally)")
	}
	if err := gitx.SetTrunk(trunk); err != nil {
		return err
	}
	names := make([]string, 0, len(metas))
	for b := range metas {
		names = append(names, b)
	}
	sort.Strings(names)
	for _, b := range names {
		if err := stack.WriteMeta(b, metas[b]); err != nil {
			return err
		}
	}

	fmt.Printf("Migrated %d branch(es) from Graphite (trunk: %s):\n", len(metas), trunk)
	for _, b := range names {
		pr := ""
		if metas[b].PR != 0 {
			pr = fmt.Sprintf("  (PR #%d)", metas[b].PR)
		}
		fmt.Printf("  %s  ◂ %s%s\n", b, metas[b].Parent, pr)
	}
	if len(skipped) > 0 {
		fmt.Printf("Skipped %d branch(es) not present locally.\n", len(skipped))
	}
	if _, err := stack.BuildGraph(); err != nil {
		fmt.Printf("Note: %v\n", err)
	}
	fmt.Println("\nGraphite's own metadata was left untouched. Run 'st log' to see your stack.")
	return nil
}
