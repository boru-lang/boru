# Releasing boru

The `boru` CLI is distributed as a Go module. Once released, install it with:

```bash
go install github.com/boru-lang/boru/cmd/go/boru@latest
```

## How the modules fit together

This repo is a **multi-module** monorepo. Six modules are published:

| Module | Path | Depends on |
|---|---|---|
| kernel | `github.com/boru-lang/boru/core/go` | — |
| parser | `github.com/boru-lang/boru/parser/go` | core/go |
| check pass | `github.com/boru-lang/boru/check/go` | core/go |
| base layer | `github.com/boru-lang/boru/basic/go` | core/go, parser/go |
| language | `github.com/boru-lang/boru/lang/go` | core/go, check/go, parser/go, basic/go |
| CLI | `github.com/boru-lang/boru/cmd/go` | lang/go (and the pieces below it) |

`calc/go`, `wpg`, `test/go`, `test/specfix`, `test/solardemo` and
`tools/vetcopies` are development-only and are not published.

### Local development uses `go.work`

`go.work` (at the repo root) lists every module so a checkout builds against
its **in-tree** sibling source. Because of the workspace, the module `go.mod`
files can pin **real released versions** of their siblings without local
`replace ../…` directives — the workspace supplies the local source on top.

> Before the very first tagged release, the sibling `go.mod`s still carry local
> `replace ../…` directives (which take precedence over the workspace). The
> first `make release` strips them and pins real versions; from then on the
> `go.mod`s are clean and `go.work` is what lets the checkout still build
> locally.

### Why the replaces have to go

`go install <pkg>@<version>` **refuses any module whose `go.mod` contains a
`replace` directive**. So a published module must have **no replaces** and must
pin **resolvable** versions of everything it requires. That is exactly what the
release flow produces.

The external `voxgig/struct` and `voxgig/udk` dependencies used to force
replaces too (their published modules declared mismatched/bare module paths).
Both are now consumed by their canonical paths —
`github.com/voxgig/struct/go` and `github.com/voxgig/udk/go` — with no replace.

## Cutting a release

Releases are cut by the **Release** workflow (`.github/workflows/release.yml`), from
the last commit on `main` that the daily build passed:

```bash
gh workflow run release.yml                    # a dry run: the plan, nothing pushed
gh workflow run release.yml -f dry-run=false   # the release
```

1. **The gate** (`scripts/release-gate.sh`). The commit — by default the head of the
   newest successful daily build on `main`, or the workflow's `commit` input — must
   be on `main`, and both CI (its push run) and the daily build must have passed it:
   nothing ships that only Linux has seen. It must also contain the last release (see
   below). To release something newer, run the daily build on `main` first
   (`gh workflow run daily.yml`) and release once it is green.
2. **The release** (`scripts/release.sh`), on a work branch at that commit. For
   `core/go` → `parser/go` → `check/go` → `basic/go` → `lang/go` → `cmd/go`, in
   dependency order, it bumps the module's patch from its latest `<module>/vX.Y.Z`
   tag (or `v0.0.1` for a first release), strips the local `replace` directives, pins
   the sibling `require`s to the versions being released, runs `go mod tidy`, commits,
   tags `<module>/vX.Y.Z` and pushes the tag. Because each module is tagged **before**
   the next one pins it, every published `go.mod` references already-resolvable
   versions.
3. **The release branch** fast-forwards to the result, and the result is merged back
   into `main` with a merge commit, never a squash, so `main` always contains the last
   release.
4. **Verification.** The workflow installs the new CLI from outside the repository,
   the way a user does (`scripts/release-verify.sh`), then starts CI on `main`, because
   the workflow's own push starts no workflow.

Locally, `make release` (or `scripts/release.sh`) runs the same release from `HEAD`,
gated instead by a full `make test`. `DRY_RUN=1 make release` previews it. Run it where
the whole suite is green; some host-backend and clipboard vault tests only pass on a
configured host.

### The release branch

`release` only ever moves forward, to a commit on `main` plus that release's version
pins: a snapshot of `main`, not a separate line. A fix to a release is a fix on `main`
(a pull request as usual) and a new release, which also ships everything else merged
since. Nothing is cherry-picked onto `release`.

If the merge back into `main` conflicts — a `go.mod` changed on `main` while the
release ran — the release itself is already out: the tags and `release` are pushed.
Merge `release` into `main` by hand to finish. The next release refuses to start until
that is done, because its gate checks that the last release is contained in the commit.

### Version scheme

"Bump patch when publishing anything." Each module advances its own patch
independently (they need not share a number). `cmd/go`'s version is also
stamped into the `boru -version` string.

## After a release

The workflow verifies the install itself (`scripts/release-verify.sh`). By hand, from
outside the repo:

```bash
cd /tmp && GOWORK=off go install github.com/boru-lang/boru/cmd/go/boru@latest
boru -version
```
