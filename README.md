# Stitch

Stacked branches on top of plain git, in a single self-contained Go binary. You **stitch** your branches into one clean, reviewable chain.

Big changes review better as a series of small, dependent branches — one reviewable step each. Keeping such a chain healthy by hand (rebasing everything above every edit) is the painful part. Stitch automates exactly that, and nothing else: it's plain git underneath, with no server, no account, and all state inside your repo's own `.git`.

- **Stack-aware branching** — `st create` stacks a branch on the current one; `st modify` amends a branch and automatically rebases everything above it.
- **Safe restacks** — each rebase is a precise `git rebase --onto` from a recorded base revision, so commits are never duplicated or dropped; conflicts pause cleanly and resume with `st continue`.
- **Stacked pull requests** — `st submit` pushes the stack and opens one PR per branch, each showing only its own diff, with an auto-maintained stack map in every description.
- **Squash-merge-aware sync** — `st sync` deletes branches whose PRs merged (even squash merges) and re-parents the rest.
- **Drop-in for git** — any command `st` doesn't recognise passes through to git, so `st status`, `st diff`, `st rebase -i` all just work.
- **Graphite import** — already have stacks in Graphite? `st init --from-graphite` brings them along, non-destructively.

## Install

Homebrew (macOS):

```sh
brew install onancelabs/tap/stitch
```

Or with Go 1.21+:

```sh
go install github.com/onancelabs/stitch/cmd/st@latest
# or, from a checkout:
go build -o st ./cmd/st          # produces ./st
sudo mv st /usr/local/bin/       # put it on your PATH
# or, with the Makefile:
make build && sudo mv st /usr/local/bin/
```

git is required at runtime either way. Pre-built binaries for macOS, Linux, and Windows are also attached to each [GitHub release](https://github.com/onancelabs/stitch/releases).

Shell completions come for free via cobra — e.g. `st completion zsh`.

## Quick start

```sh
cd your-repo
st init                          # detects trunk (main/master)

# build a 3-branch stack, one commit each
echo "..." > a.txt
st create add-model -a -m "Add model"
echo "..." > b.txt
st create add-api   -a -m "Add API on top of model"
echo "..." > c.txt
st create add-ui    -a -m "Add UI on top of API"

st log                           # see the stack

# fix something lower in the stack; everything above is rebased automatically
st down                          # move toward trunk
# ...edit files...
st modify -am "fix"              # amends this branch, restacks its children

st submit                        # push the stack, open a PR per branch
st sync                          # later: drop merged branches, restack
```

The branch name in `create` may come before or after the flags — both `st create my-branch -a -m "msg"` and `st create -a -m "msg" my-branch` work, and short flags bundle git-style (`st modify -am "msg"`).

## Commands

| Command                                   | Alias | What it does                                                                                                                                  |
| ----------------------------------------- | ----- | --------------------------------------------------------------------------------------------------------------------------------------------- |
| `init [--trunk <name>] [--from-graphite]` |       | Configure the trunk branch (auto-detects `main`/`master`), or import an existing Graphite stack.                                              |
| `create <branch> [-m msg] [-a]`           | `c`   | Create a branch stacked on the current one, committing staged changes (`-a` stages everything first).                                         |
| `modify [-m msg] [-a] [-c]`               | `m`   | Amend the current branch's commit (or `-c` for a new commit), then restack everything above it.                                               |
| `restack [--all]`                         | `r`   | Rebase the current stack so each branch sits on its parent. `--all` does every tracked branch.                                                |
| `continue`                                |       | Resume a restack after you resolve a conflict.                                                                                                |
| `abort`                                   |       | Cancel an in-progress restack and return to where you started.                                                                                |
| `up [<child>]`                            | `u`   | Check out a child branch (prompts when there are several).                                                                                    |
| `down`                                    | `d`   | Check out the parent branch (toward trunk).                                                                                                   |
| `track [-p <parent>]`                     |       | Start tracking the current (ordinary git) branch; parent defaults to trunk.                                                                   |
| `untrack [<branch>] [--thread] [--all]`   |       | Stop tracking a branch (re-parenting children), the current thread (`--thread`), or every tracked branch (`--all`). Git branches stay intact. |
| `log`                                     | `ls`  | Show the stack as a tree, top of stack first, flagging branches that need a restack.                                                          |
| `sync [--no-fetch]`                       |       | Fetch, fast-forward trunk, delete merged branches (squash-merge aware), and restack the survivors.                                            |
| `submit [-r/--ready]`                     | `s`   | Push the current stack and open/update one PR per branch (base = parent). Drafts by default; `-r` opens for review.                           |
| `auth [--token \| --status \| --logout]`  |       | Sign in to GitHub via the OAuth device flow (default), or store a PAT with `--token`; kept in the OS keychain.                                |

Run `st <command> -h` for per-command flags. Any command not in this table is forwarded straight to git with the same arguments and exit code.

## How it works

A "stack" is a chain (or tree) of branches where each branch is one reviewable change and remembers two things: its **parent branch** and the **parent revision it was based on**. From those two facts everything else — restacking after an edit, navigating up and down, syncing onto a moved trunk — falls out of ordinary git plumbing. (Stitch's own word for a single stack is a _thread_ — you'll see it in commands like `st untrack --thread`.)

When you amend a commit, its SHA changes, so every branch above it is now based on a commit that no longer exists. `restack` walks the stack top-down (parents before children) and, for each branch, runs:

```
git rebase --onto <parent's new tip> <parent's OLD tip> <branch>
```

The "old tip" is the `parentRev` Stitch recorded for that branch. Passing it as the rebase base is what guarantees exactly the branch's own commits are replayed — no parent commits get duplicated, none get dropped. After a branch is replayed, Stitch updates its `parentRev` to the parent's new tip so the next branch up the stack lines up correctly.

### Conflicts

If a rebase hits a conflict, the remaining plan is saved to `.git/stitch/restack.json` and the run stops. Resolve the files, `git add` them, then:

```sh
st continue     # finishes the current branch and resumes the rest
st abort        # or bail out entirely and return to your start branch
```

### Where state lives

Per-branch metadata is stored as a git blob pointed to by the ref `refs/stitch/<branch>`. This is durable (survives `git gc`), invisible to your working tree, never treated as a real branch, and not pushed by default. The trunk name is stored in `git config stitch.trunk`. Removing the tool leaves your branches untouched; to wipe its state:

```sh
git for-each-ref --format='%(refname)' refs/stitch/ | xargs -n1 git update-ref -d
git config --unset stitch.trunk
```

## Submitting stacks (GitHub)

`st submit` turns your local stack into a set of stacked pull requests.

Sign in once — the resulting token is kept in your OS keychain:

```sh
st auth                 # OAuth device flow: copy the one-time code, confirm in your browser
st auth --token <tok>   # or store a personal access token directly
st auth --status        # check it's set;  st auth --logout to remove
```

The device flow needs an OAuth App client id (embedded at release time, or set `STITCH_GITHUB_CLIENT_ID`). A PAT instead needs pull-request and contents write access — fine-grained with _Pull requests: read/write_ and _Contents: read/write_, or a classic token with the `repo` scope. `st` also reads `GITHUB_TOKEN` / `GH_TOKEN` from the environment, which is handy for CI.

Then, from any branch in your stack:

```sh
st submit               # draft PRs;  st submit -r  opens them for review
```

For each branch in the current stack, bottom-up, `submit`:

1. pushes it with `--force-with-lease` (restacks rewrite history, so a plain push would be rejected; the lease keeps the force push safe);
2. opens or updates a PR whose **base is the branch's parent** — so each PR shows only its own diff — recording the PR number in the branch metadata;
3. writes a stack map into each PR description, inside a managed block so re-submitting updates it instead of piling on comments.

When it finishes, `st submit` opens the top PR in your browser (pass `--no-open` to skip).

After PRs merge, `st sync` asks GitHub the state of each one. Merged PRs — **including squash merges, which `git branch --merged` cannot detect** — have their local branch deleted and their children re-parented onto the nearest surviving ancestor before the stack restacks. With no token available, `sync` still does the local half (fetch, fast-forward trunk, restack).

## Migrating from Graphite

If you have existing stacks in Graphite, import them in place:

```sh
st init --from-graphite
```

This reads each branch's parent, base revision, and PR number, and writes Stitch's equivalent (`refs/stitch/…` and `stitch.trunk`), **leaving the original data untouched** so you can switch back at any time. Branches that no longer exist locally are skipped, and a missing base revision falls back to a live `git merge-base`.

It reads whichever store is present, in order:

1. the SQLite database (`.git/.graphite_metadata.db`) via the `sqlite3` CLI — the authoritative, always-current source;
2. if `sqlite3` isn't installed, the JSON snapshot (`.git/.gt/snapshots/`) — **no external tool required**;
3. the legacy `refs/branch-metadata` blobs used by older versions.

So you rarely need anything extra: `sqlite3` ships with macOS, and if it's absent the JSON-snapshot fallback covers it. Only if none of the three are readable does it print an OS-specific hint to install `sqlite3`.

## Safety & security

- **No shell, ever.** git is always invoked via `exec.Command("git", args...)` with a separate argument vector — never `sh -c` and never string interpolation — so branch names, commit messages, and SHAs cannot inject shell commands.
- **Argument-injection guarded.** User-supplied branch names are validated (rejected if empty or starting with `-`) so they can't be misread by git as options. SHAs handed to `rebase --onto` come from `git rev-parse`, not user input.
- **Tokens stay in the OS keychain.** `st auth` stores your GitHub token via the system keychain (or reads `GITHUB_TOKEN`/`GH_TOKEN`); it is never written to the repo or printed back out. All GitHub calls are confined to `internal/forge`.
- **History-rewriting commands are gated.** `restack`/`sync`/`modify` refuse to run with a dirty working tree, while another rebase is in progress, or while a restack is paused; `modify` also refuses to touch trunk or to amend a branch that has no commit of its own; `create` rolls back the new branch if its commit is aborted. Because rewrites go through `git rebase`, the previous tips remain recoverable via `git reflog`.
- **State is confined to `.git`.** The only files written are inside the repo's own git directory (`.git/stitch/`), resolved through `git rev-parse --git-path`; no user-controlled paths are ever written.

## Development

```sh
go test ./...        # the suite drives real throwaway git repositories
go vet ./...
make dist            # cross-compile dist/st-<os>-<arch> for macOS, Linux, Windows
go build -ldflags "-X main.version=1.2.3" -o st ./cmd/st   # version injection
```

Releases are automated: pushing a `vX.Y.Z` tag runs GoReleaser via GitHub Actions, which builds the binaries, attaches them to a GitHub release, and updates the Homebrew tap ([`onancelabs/homebrew-tap`](https://github.com/onancelabs/homebrew-tap)).

The restack engine is exercised end-to-end against real git (tree-shaped stacks, conflict-then-continue, moved trunks). The `submit`/`sync` orchestration is tested offline against a fake forge plus a local bare origin, and the OAuth device flow against a fake GitHub server. The only untested paths are the real GitHub API round-trips behind the `Forge` interface.

Dependencies are three small, well-known libraries: [`cobra`](https://github.com/spf13/cobra)/[`pflag`](https://github.com/spf13/pflag) (CLI), [`go-github`](https://github.com/google/go-github) (PR API), and [`go-keyring`](https://github.com/zalando/go-keyring) (token storage). Everything else is the standard library.

## Roadmap

More forges behind the `Forge` interface (GitLab, Bitbucket); reviewer/label/title management on `submit`; GitHub Enterprise base URLs; an `undo` built on reflog snapshots.

## Acknowledgements

Stacked-branch workflows have a healthy ecosystem of tools — [Graphite](https://graphite.dev), [ghstack](https://github.com/ezyang/ghstack), [git-spr](https://github.com/ejoffe/spr), [av](https://github.com/aviator-co/av), [git-branchless](https://github.com/arxanas/git-branchless), and others — each with its own take. Stitch is an independent, from-scratch implementation of the workflow, built to stay small: plain git underneath, local-only state, one binary.

## License

[Business Source License 1.1](LICENSE). In short: you can use Stitch freely — personally and inside your organization, commercial or not — and read and modify the source. What you can't do is offer Stitch itself to third parties as a commercial product or service. Each version converts to the Apache 2.0 license four years after its release. For other arrangements, see the contact in the LICENSE file.
