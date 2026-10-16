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

```bash
make release            # full test → auto patch-bump → tag & push, in order
DRY_RUN=1 make release   # preview: prints every action, tags/pushes nothing
```

`scripts/release.sh`:

1. **Gates on the full suite** — runs `make test`; nothing is tagged unless it
   passes. (Run releases where the whole suite is green — e.g. Linux CI; some
   host-backend/clipboard vault tests only pass on a configured host.)
2. **Auto-bumps the patch** of each module from its latest `<module>/vX.Y.Z`
   tag (or `v0.0.1` for a module's first release).
3. For `core/go` → `parser/go` → `check/go` → `basic/go` → `lang/go` → `cmd/go`, in dependency order: strips the local
   `replace` directives, pins the sibling `require`s to the versions being
   released, `go mod tidy`, commits, tags `<module>/vX.Y.Z`, and pushes.

Because each module is tagged **before** the next one pins it, every published
`go.mod` references already-resolvable versions.

### Version scheme

"Bump patch when publishing anything." Each module advances its own patch
independently (they need not share a number). `cmd/go`'s version is also
stamped into the `boru -version` string.

## After a release

Verify from outside the repo:

```bash
cd /tmp && GOWORK=off go install github.com/boru-lang/boru/cmd/go/boru@latest
boru -version
```
