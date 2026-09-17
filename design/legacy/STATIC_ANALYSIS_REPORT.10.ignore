# Static analysis — setup and findings

This documents the static-analysis tooling wired into the repo and the
findings from the first run. Three Go modules are covered: `eng/go`
(engine kernel + parser), `lang` (language library), `cmd/go` (CLI tools).

## What was added

| Tool | How it runs | Where |
| --- | --- | --- |
| **golangci-lint** (v2.5.0) | `make lint` — runs `golangci-lint run ./...` in all three modules | new `lint` target in `lang/go/Makefile`, `cmd/go/Makefile`, and the new `eng/go/Makefile`; new step in `.github/workflows/ci.yml` |
| **govulncheck** | `make vuln` — runs `govulncheck ./...` in all three modules | new `vuln` target in the same Makefiles; new (advisory) step in CI |
| `lint-assertions` (pre-existing grep check) | `make lint-assertions` | unchanged, still in `lang/go/Makefile` and CI |

`golangci-lint`'s config lives at the repo root (`.golangci.yml`) and is
found by walking up from each module directory. Local install:

```bash
curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/HEAD/install.sh \
  | sh -s -- -b "$(go env GOPATH)/bin" v2.5.0
go install golang.org/x/vuln/cmd/govulncheck@latest
```

## golangci-lint configuration (`.golangci.yml`)

Deliberately conservative — meant to gate CI without churn:

- **Enabled:** the `standard` preset (`errcheck`, `govet`, `ineffassign`,
  `staticcheck`, `unused`) plus `bodyclose`, `misspell`, `unconvert`. The
  `gofmt` formatter is checked.
- **Tuned:**
  - `staticcheck` keeps golangci-lint's default check set but additionally
    drops the `QF*` quickfix *suggestions* (De-Morgan rewrites,
    tagged-switch hints) — useful in review, too stylistic to gate on.
  - `errcheck.exclude-functions` skips a few calls where ignoring the
    error is the established convention: `(*database/sql.Tx).Rollback`,
    `(net/http.ResponseWriter).Write`, `(*encoding/json.Encoder).Encode`.
  - `_test.go` files are exempt from `errcheck` and `ineffassign` (test
    setup routinely discards errors; spec-runner fixtures intentionally
    discard the always-nil error from already-type-matched `AsX()`).
  - `unused` is suppressed for a handful of files that carry parked /
    work-in-progress dead symbols — `lang/go/native/{native_query,query,
    native_string_helpers,native_temporal_await}.go` (query-builder
    rework, unit-aware string helpers, the await ordering field) — and for
    the `lang/go/native/aliases.go` re-export shim, which intentionally
    mirrors the whole `eng` API. **These exclusions are debt; remove them
    once the parked code lands or is deleted.**
- **Not enabled (opt-in / ad-hoc):** `gosec`, `revive`/`stylecheck`,
  `gocritic`, `errorlint`, `nilerr`, `nilaway`. See the gosec section
  below; `nilerr` was evaluated and left out (its three hits are all
  deliberate error-swallowing — verify, then `//nolint` or fix if you
  want it on).

## golangci-lint findings on first run

All findings on the existing code were either fixed or grandfathered via
config. After this change `golangci-lint run ./...` is clean (0 issues)
in all three modules.

### Fixed

| Where | Linter | Fix |
| --- | --- | --- |
| `eng/go/engine.go` | `unused` | deleted dead `(*Engine).peekForwardValue`, `(*Engine).resolvedStackBefore` |
| `eng/go/parser/parse.go` | `unused` | deleted dead `isWhitespace` |
| `eng/go/carrier.go:442` | `ineffassign` | removed dead `bestHasFn = true` (followed by `break`) |
| `eng/go/engine.go` (stepCloseParen) | `ineffassign` | removed dead `closeIdx--` (overwritten by `findCloseParenAfter` two lines down) |
| `eng/go/engine.go:612` | `errcheck` | `_ = e.stepOpenParen()` — it never returns a non-nil error |
| `eng/go/nativefunc.go:72` | `staticcheck` S1016 | `//nolint:staticcheck` — the explicit `NativeSig`→`Signature` field copy is intentional |
| `lang/go/native/native_array.go:1164`, `lang/go/native/natives.go:554`, `lang/go/native/mutability_test.go:227` | `staticcheck` ST1023 | dropped the redundant explicit type in `var x T = …` |
| `lang/go/native/listops.go:37,73` | `staticcheck` S1009 | dropped the `nil` check before `len()` |
| `lang/go/modules/time.go:233` | `errcheck` | `_, _ = fmt.Sscanf(…)` — best-effort parse of an already-validated digit run |
| `lang/go/native/mutability_test.go:42`, `lang/go/test/object_type_test.go:1522` | `staticcheck` SA4006 | dropped / `_`-ed the dead intermediate `result` assignment |
| `cmd/go/install.go:91` | `errcheck` | check `os.MkdirAll` for the directory-entry case and bail on error |
| `cmd/go/auth.go:588` | `errcheck` | `_ = json.Unmarshal(…)` with a comment — best-effort parse of a 201 body |
| `test/solardemo/main.go` (×4) | `staticcheck` ST1013 | `405` → `http.StatusMethodNotAllowed` |

### Grandfathered via config (debt to triage)

- **`QF1001`/`QF1003`** (~8 spots in `word_name.go`, `native_boolean.go`,
  `native/setpath.go`, `engine/query.go`) — De-Morgan / tagged-switch
  refactor suggestions; suppressed globally, not bugs.

The previously-grandfathered ~31 `unused` symbols in the query-builder
rework (`native_query.go` + `query.go`), the unit-aware string helpers
(`toGraphemes`, `strLen`, `strSlice`), the `order` field in
`native_temporal_await.go`, and 5 re-export aliases in `aliases.go`
have all been deleted; the matching `.golangci.yml` exclusions are
removed and `unused` is enforced on those files again.

### Error-handling refactor (post architecture review, May 2026)

CLAUDE.md names `r.BoruError(code, detail, word)` as the canonical
handler-side error path (it threads the source position the engine
needs for `--> row:col` site extracts and the structured `[boru/<code>]`
prefix). Pre-refactor, native handlers almost universally returned
bare `fmt.Errorf` errors instead, which dropped both pieces of context.

Roughly **140 sites** across 23 files in `lang/go/native/*.go` were
converted in a mechanical pass:

```
return nil, fmt.Errorf("op: detail %d", n)
→
return nil, r.BoruError("op_error", fmt.Sprintf("op: detail %d", n), "op")
```

The opname prefix is kept in the detail so existing substring-match
error tests (`strings.Contains(err.Error(), "op: …")`) keep working.
Handler signatures with `_ *Registry` were upgraded to `r *Registry`
to access the helper.

Remaining `fmt.Errorf` calls (~210) fall into three categories that
are deliberately not converted:

1. `fmt.Errorf("%w", …)` wrappers — preserve the underlying chain
   for `errors.Is`/`errors.As`.
2. Calls inside helper functions that don't have a `*Registry` in
   scope (e.g. parser utilities in `query.go`'s SQL builders).
3. Calls that construct an `&BoruError{…}` literal directly — already
   structured, just bypass the helper.

The remaining sites can be ratcheted down in follow-up PRs by
plumbing the registry through helpers, or by adding a
`MakeBoruError(code, detail, word, src, hint)` variant for the
no-registry path.

### Looked at but left alone

`nilerr` (not enabled) flags three deliberate error-swallows — worth a
second look but defensible as designed:

- `lang/go/native/native_module_module.go` (`loadModuleResources`) — a
  malformed `.boru/boru.json` is treated as "no resources" rather than an
  error. A typo would silently disable a module's resources.
- `lang/go/native/native_type.go:321` — a predicate-evaluation error inside
  `x is SomePredicate` yields `false` rather than propagating.

## govulncheck

`make vuln` currently **fails** (exit 3). All reachable findings are in
the **Go standard library** at `go1.24.7` (the version pinned by the `go`
directive); none are in this repo's first-party code or in a reachable
path through a third-party dependency:

- **~19 reachable stdlib advisories** (`GO-2025-401x` … `GO-2026-49xx`) in
  `crypto/x509`, `crypto/tls`, `net/http`, `net/url`, `net`, `os`,
  `encoding/asn1`. Reached via the HTTP-fetch native word, the decision
  module's TLS use, etc. Fixed in patch releases ranging from `go1.24.8`
  through `go1.25.10`.
- **~14 more** in imported packages / required modules that are *not* on a
  reachable call path (govulncheck does not fail on these).

**Recommendation:** bump the `go` directive (and keep it current) — that
clears the ones fixed on the 1.24.x line; the rest need 1.25.x. Until
then the CI `vuln` step is `continue-on-error: true` (advisory). The scan
is still worth keeping: it's the signal for when to update Go and the
guard against a future *reachable* dependency CVE.

## gosec (ad-hoc — not wired into CI/Makefiles)

`gosec` was run once for this report. It is **not** in `make lint` or CI
(too noisy at default settings — G104 unhandled-error and permission-bit
checks dominate), but a periodic manual `gosec ./...` per module is
worthwhile. Findings by module:

### `cmd/go` — the most security-relevant module

- **G305 (zip-slip)** — `cmd/go/install.go:89`, the module-zip
  extractor. There *is* a guard (`if strings.Contains(f.Name, "..")`),
  and `filepath.Join` cleans the path, so the practical risk is low — but
  the canonical pattern (`filepath.Clean`, then require the result to be
  under `destDir`) is more robust. **Worth tightening.**
- **G703 / G304 (path traversal via taint)** — `install.go`,
  `registry.go`, and `auth.go`/`main.go`: module names / file names from
  HTTP requests and CLI args flow into filesystem paths. Same root cause
  as the zip-slip item; review the path-construction sites together.
- **G114 (no server timeouts)** — `wpg/serve/main.go:75`,
  `test/solardemo/main.go:204`, `cmd/go/registry.go:273` use
  `http.ListenAndServe` with no `ReadTimeout`/`WriteTimeout`. Fine for a
  dev/test server; harden (use a configured `http.Server`) if any of
  these is ever exposed.
- **G107 (variable URL in HTTP request)** — `install.go:53` fetches from
  the user-supplied `-r <registry-url>`. Intended behavior.
- **G705 (XSS via taint)** — `registry.go:47` writes a module zip's bytes
  to the response. False positive for binary content; set
  `Content-Type: application/zip` for clarity.
- **G104 (×18), G301/G306 (perm bits ×12)** — unhandled `w.Write` /
  `json.Encode` / `rc.Close` returns, and `0755` dir / `0644` file modes
  (gosec wants `0750`/`0600`). Conventional; not acted on.

### `eng/go`

- **G404 (weak RNG ×2)** — `eng/go/value.go:513,523`: `math/rand` to
  generate object-type IDs (`"T_" + 12 hex chars`). Not a security token;
  fine as a uniqueness source.
- **G115 (integer-overflow conversion ×7)** — `value.go:523,531–536`: the
  `int64`/`uint64`→`byte` truncations that turn random bytes into hex
  digits. Intentional masking.
- **G602 (×2)** — `core_helpers.go:404,411`: gosec's bounds analysis
  being conservative; false positives.

### `lang`

- **G201 (SQL string formatting ×1)** — `lang/go/native/sqlite.go:122`:
  `fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)", quoteIdent(name),
  joinQuoted(columns), …)`. Identifiers are quoted via `quoteIdent` /
  `joinQuoted` and the values use `?` placeholders, so this is not an
  injection vector — but it's the kind of pattern worth a comment.
- **G304 (×1)** — `lang/go/capabilities/fileops.go:33`: `os.ReadFile`
  on a resolved module path. There is resolution logic upstream; gosec
  flags any variable path. Consider `os.Root`-scoped access (Go 1.24).
- **G104 (×2)** — the `tx.Rollback()` calls in `sqlite.go` (now excluded
  in the golangci-lint config too).

## Complexity, size, duplication

A first-pass design-analysis snapshot. `gocyclo`, `gocognit` and `dupl`
are wired into `make lint` (the linters are now in `.golangci.yml`);
thresholds are pinned just above the current worst so the gate is green
today and only a new catastrophically-complex or duplicated function
would fail it. The full distribution is documented here so the
thresholds can be ratcheted down as the hotspots are refactored.

### Size map — `scc`

| Module | Files | Lines | Code | Comments | `scc` complexity |
| --- | ---: | ---: | ---: | ---: | ---: |
| `eng/go` | 42 | 20,781 | 14,735 | 4,393 | 3,991 |
| `lang` | 201 | 74,923 | 61,645 | 6,038 | 15,753 |
| `cmd/go` | 19 | 6,565 | 5,541 | 312 | 1,254 |
| `util/go` | 1 | 187 | 144 | 30 | 43 |
| **total (Go)** | **263** | **102,456** | **82,065** | **10,773** | **21,041** |

(`scc complexity` is a heuristic score, useful as a relative size signal
rather than a strict metric.)

### Cyclomatic complexity — `gocyclo` (non-test, top 15)

| Cyclo | Function | File |
| ---: | --- | --- |
| 87 | `(*Engine).matchSignature` | `eng/go/match.go:27` |
| 66 | `Value.String` | `eng/go/value.go:1605` |
| 64 | `inferExact` | `lang/go/native/native_help.go:121` |
| 63 | `Unify` | `eng/go/unify.go:20` |
| 49 | `(*Engine).stepWord` | `eng/go/engine.go:693` |
| 42 | `parseStrOpts` | `lang/go/native/native_string_helpers.go:59` |
| 41 | `(*Engine).execFnDefLiteral` | `eng/go/engine.go:1527` |
| 40 | `(*Engine).execMatch` | `eng/go/engine.go:937` |
| 38 | `ParseFnParams` | `eng/go/fn_params.go:57` |
| 37 | `makeHandler` | `eng/go/core_make.go:325` |
| 36 | `execute` | `cmd/go/main.go:31` |
| 36 | `(*Engine).Run` | `eng/go/engine.go:332` |
| 35 | `tokenize` | `lang/go/formatter/format.go:90` |
| 35 | `(*Engine).execFnDefSig` | `eng/go/engine.go:1702` |
| 35 | `(*Engine).preEvalParens` | `eng/go/engine.go:558` |

Distribution (non-test, all three modules):

| Cyclomatic complexity | Functions |
| ---: | ---: |
| `> 15` | 79 |
| `> 20` | 43 |
| `> 30` | 22 |
| `> 40` | 7 |
| `> 50` | 4 |

### Cognitive complexity — `gocognit` (non-test, top 10)

Cognitive complexity penalises nesting and unconventional control flow,
so it diverges from cyclomatic in deeply-nested code.

| Cognit | Function | File |
| ---: | --- | --- |
| **211** | `(*Engine).matchSignature` | `eng/go/match.go:27` |
| 101 | `ParseFnParams` | `eng/go/fn_params.go:57` |
| 93 | `makeHandler` | `eng/go/core_make.go:325` |
| 92 | `(*Engine).execFnDefLiteral` | `eng/go/engine.go:1527` |
| 92 | `InstallDef` | `eng/go/core_helpers.go:31` |
| 91 | `(*Engine).preEvalParens` | `eng/go/engine.go:558` |
| 89 | `InstallFnDef` | `eng/go/core_helpers.go:194` |
| 83 | `Unify` | `eng/go/unify.go:20` |
| 81 | `(*Engine).stepCloseParen` | `eng/go/engine.go:2182` |
| 70 | `Value.String` | `eng/go/value.go:1605` |

Distribution: 123 funcs `> 15`, 49 `> 30`, 33 `> 40`, 22 `> 50`,
15 `> 60`.

`matchSignature` is the clear outlier (cyclo 87, cognit 211) — it's the
unified dispatch algorithm and is intrinsically a big decision tree.
It carries `//nolint:gocyclo,gocognit` with a comment pointing here.
Everything else clears the current gate (cyclo 70 / cognit 200).

> **Correction (2026-08-31).** The tables above are the first-run
> snapshot and their locations pre-date the four-piece module carve
> (`eng/go/match.go` no longer exists — the function is
> `(*Engine).MatchSignature` at `core/go/engine.go`). More importantly
> the 87/211 outlier reading is DEAD: Stage 2's collection-kernel
> re-seat (54d8830) extracted the ~250-line per-candidate scan to
> `CollectCandidateScan`, and the live measure is **cyclo 68 /
> cognit 131** — under the caps — so its `//nolint` is deleted and the
> gate binds on it for real. `gocognit.min-complexity` is ratcheted
> 200 → 170 (worst non-exempt: `check/go BuildFnBodyReturnsFn` 165;
> `core stepCloseParen` 161); `gocyclo` stays 70 (worst non-exempt:
> `basic DefTypedHandler` at exactly 70, which passes under the
> strictly-greater semantics). The one remaining
> `//nolint:gocyclo,gocognit` pair in the tree is `eng vm.run`
> (148/278) — the VM instruction switch, which Stage 4 grows by
> design. The lesson this correction encodes: complexity figures in
> comments and docs go stale silently — measure before planning
> against them (`gocyclo`/`gocognit` standalone, commands below).

### Duplication — `dupl`, clone groups at ≥ 300 tokens

| Where |
| --- |
| `lang/go/test/boolean_test.go:19-97` ↔ `lang/go/test/syntax_test.go:26-107` |
| `lang/go/test/basic_test.go:15-93` ↔ `lang/go/test/options_params_test.go:15-89` |
| `lang/go/test/arg_order_test.go:1-89` ↔ `lang/go/test/options_params_test.go:1-89` |

All three are test-table boilerplate inside `lang/go/test/`. The gate
(`dupl.threshold: 400` tokens) sits above the worst current group, so
they don't trip it; raising the threshold higher catches nothing. New
copy-paste larger than the current worst would fail.

The previously-noted cross-module clone — the spec-runner harness,
which had `eng/go/spec_test.go` and `lang/go/test/spec_runner_test.go`
implementing the same "read `.tsv`, parse tokens, run an engine,
compare output" loop — has been extracted into the new
`github.com/boru-lang/boru/util/go/specrunner` module. Both test files
now call `specrunner.RunDir(t, ..., func(input) ([]eng.Value, error)
{...})` and just supply their own engine setup.

### Running the tools standalone

The numbers above were produced by:

```bash
go install github.com/fzipp/gocyclo/cmd/gocyclo@latest
go install github.com/uudashr/gocognit/cmd/gocognit@latest
go install github.com/mibk/dupl@latest
go install github.com/boyter/scc/v3@latest

gocyclo  -ignore '_test\.go$' -top 25  eng/go lang cmd/go util/go
gocognit -top 25                       eng/go lang cmd/go util/go
dupl     -t 300                        eng/go lang cmd/go util/go
scc      --no-cocomo                   eng/go lang cmd/go util/go
```

`make lint` invokes the golangci-lint-bundled versions of `gocyclo`,
`gocognit` and `dupl` at the gate thresholds.

## Suggested next steps

1. **Bump the `go` directive** (and keep current) to clear the
   1.24.x-fixed stdlib advisories; flip the CI `vuln` step to blocking
   once it's clean.
2. **Triage the parked dead code** in `lang/go/native/native_query.go` /
   `query.go` (finish or delete) and drop the matching `unused`
   exclusions from `.golangci.yml`.
3. **Review the `cmd/go` path-construction sites** (zip extraction,
   registry file serving, module install) — the gosec G305/G703 cluster.
4. **Consider rewriting `lint-assertions`** (the grep for unsafe
   `.Data.(Type)` assertions in `lang/go/native/native_*.go`) as a small
   `go/analysis` analyzer so it runs under `golangci-lint` and matches the
   AST instead of source text.
5. Optionally tighten `.golangci.yml` over time — `errorlint`,
   `bodyclose` settings, a curated `gosec` profile, `gofumpt` instead of
   `gofmt`.
6. **Ratchet the complexity / dupl gates** in `.golangci.yml`
   (`gocyclo.min-complexity`, `gocognit.min-complexity`, `dupl.threshold`)
   downward as the top-of-list hotspots get refactored. Today they're
   pinned just above the current worst (so green but only catches
   catastrophic regression); useful next steps are splitting
   `matchSignature` (the only function carrying both nolints) and the
   2,000-line-class engine helpers.
   *(2026-08-31: the `matchSignature` half of this step happened as a
   side effect of Stage 2's re-seat and the gocognit ratchet landed at
   170 — see the correction above. Further planner decomposition is
   deliberately trigger-gated on Stage 4; the triggers live in
   design/FULL-COMPILATION.0.md §10.)*
