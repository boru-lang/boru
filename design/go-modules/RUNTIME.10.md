# `runtime` → `boru:runtime` (`Runtime`)

> **Status: design proposal — not implemented.** This note specifies the
> curated boru surface for Go's `runtime` package. No Go code exists yet;
> the note exists so the proposed surface — and its **policy gating** — is
> auditable before any handler is written. Read
> [`README.10.md`](README.10.md) first for the shared conventions this
> note assumes.

## 1. Package & status

Go [`runtime`](https://pkg.go.dev/runtime) is the program's view of the
**Go runtime and the machine it runs on**: the target OS/architecture,
the CPU count, the scheduler's goroutine and `GOMAXPROCS` settings, and
the toolchain version. `boru:runtime` curates the small, read-only,
constant-style slice of that surface that a query/script might
legitimately want — for capability probing, logging, or
platform-conditional behaviour. The scheduler-tuning, GC, profiling, and
stack-introspection halves of `runtime` are out of scope (§10).

## 2. Why curated

The raw `go:` bridge would expose these as Go-named functions with `Any`
boundaries (`runtime.GOMAXPROCS(int) int` even takes a mutating arg). The
curated module:

- presents each as a **zero-arg, constant-style word** with a real
  lattice return type (`goos`→String, `num-cpu`→Integer, …);
- exposes `GOOS`/`GOARCH` (Go *constants*) and the function readers
  (`NumCPU`, `Version`, …) uniformly as words, hiding the const-vs-func
  distinction;
- reads `GOMAXPROCS` **without mutating** it (Go's `GOMAXPROCS(-1)`
  query convention is wrapped so the boru word can never change the
  setting — there is no setter); and
- gates the whole module on `system-info` (§7), which the bridge cannot.

## 3. Import & namespace

```
import "boru:runtime"     # binds the Runtime namespace
```

Namespace is the plain capitalized package name, **`Runtime`** — no
`-util` suffix (it is not a pure helper library and clashes with no
builtin type). Words are reached via dot access; being zero-arg, they are
simply `Runtime.goos`, `Runtime.num-cpu`, etc.

## 4. API

All words are **zero-arg constants** — they read nothing from the stack
and return a single scalar. Their inner native sigs declare
**`BarrierPos: 0`** (constant-style, exactly like `math-pi`/`math-e` in
`math.go`). At zero args the spelling is byte-identical to `-1` after
registration (`-1` normalizes to `TotalArgs()` = 0 — NUR023), so this
is an idiom choice, not a divergence from ADR-004's forward-eligible
default.

| Go symbol | boru word | signature (top-first) | one-line doc | boru-ish refinement |
|---|---|---|---|---|
| `runtime.GOOS` | `goos` | `→ String` | The target operating system (`"linux"`, `"darwin"`, …). | Go build *constant* exposed as a zero-arg word. |
| `runtime.GOARCH` | `goarch` | `→ String` | The target architecture (`"amd64"`, `"arm64"`, …). | Go build constant exposed as a zero-arg word. |
| `runtime.NumCPU` | `num-cpu` | `→ Integer` | The number of logical CPUs usable by this process. | `func() int` → zero-arg Integer word. |
| `runtime.NumGoroutine` | `num-goroutine` | `→ Integer` | The number of goroutines currently existing. | `func() int` → zero-arg Integer word. |
| `runtime.GOMAXPROCS` | `max-procs` | `→ Integer` | The max OS threads executing Go code simultaneously. | **Read-only**: wraps `GOMAXPROCS(-1)` (the query form) so the word reports the value and can never set it. No setter is exposed. |
| `runtime.Version` | `version` | `→ String` | The Go runtime/toolchain version (e.g. `"go1.24"`). | `func() string` → zero-arg String word. |

### Refinement notes

- **No setter for `max-procs`.** Go's `runtime.GOMAXPROCS(n)` both reads
  *and* sets (returning the previous value); passing `-1` is the
  documented "query without changing" idiom. The boru word wraps the
  `-1` query form only, so `boru:runtime` cannot retune the host
  scheduler. (Mutating the scheduler would be a `process`-class effect,
  not `system-info`; it is deliberately omitted — see §10.)
- **No `(value, ok)` / `(value, error)` to collapse.** Every covered
  symbol returns a plain value, so there is no None/error refinement
  here — just the rename and the const-style packaging.

## 5. Types

Scalars only — `String` (`goos`, `goarch`, `version`) and `Integer`
(`num-cpu`, `num-goroutine`, `max-procs`) — bridged via `core.ToNative`
(`core/go/gobridge.go`). **No opaque external handle**, so no `FixedID`
from the `10000+` range and no entry in
`lang/go/test/fixedid_stability_test.go`.

## 6. Errors

These words cannot fail at the Go level — every covered symbol is a
constant or an infallible reader. So there are **no module-specific
error codes** for the happy path. The only error surface is the policy
gate (§7), which returns either the host policy's `*policy.Denied`
verbatim or a `capability_not_installed`-style stub error when
`system-info` is uninstalled; surface its `Code`. Handlers still take no
unguarded action that could panic (`core/go/CLAUDE.md` "Panic
Prevention"). Every positive `.tsv` row still pairs with an
`ERROR:<substring>` sibling — here the negatives are the
**deny-policy** rows and the wrong-arity rows (a zero-arg word handed an
argument), per `lang/go/CLAUDE.md` "Test discipline".

## 7. Policy / capabilities

Every word here **leaks host fingerprint** — OS, architecture, CPU
count, toolchain version, live goroutine count all reveal the execution
environment and vary by host and build. So the whole module gates on the
**`system-info`** global cap (`policy.GlobalOps`), the same cap
`boru:os`'s `hostname`/`home-dir`/`temp-dir` use.

There is **no dedicated capability scope** in `policy.KnownScopes` for
runtime info (no `runtime` scope exists, and none is proposed). Instead,
each handler consults the global cap directly via
`HostPolicy(r).CheckGlobal("system-info")` (the `Policy.CheckGlobal`
method, `policy/policy.go`) — the same single-hard-cap check the
capability wrappers use. On allow, the handler returns the value; on
deny it returns the `*policy.Denied`.

| word(s) | global cap (`GlobalOps`) | default posture |
|---|---|---|
| `goos`, `goarch`, `num-cpu`, `num-goroutine`, `max-procs`, `version` | `system-info` | on when `system-info` allowed; deny/stub for determinism |

**Determinism concern (call this out).** Because these values vary by
host and build, a sandbox that wants **reproducible** runs (specs,
golden tests, content-addressed evaluation) should **deny or stub**
`system-info`. Mirroring the `FixedClock` precedent
(`lang/go/capabilities/capabilities.go`), a host can install a small
fake "runtime info" provider returning canonical values (`goos="linux"`,
`num-cpu=1`, `version="go-test"`) so output is stable. Even though these
are read-only constants, they must still route through the capability
seam — **not** direct `runtime.*` calls in the handler — so a sandbox can
substitute the fake (the same seam discipline `FileOps`/`HostEnv`
follow). `num-goroutine` is the least deterministic (it depends on live
scheduler state) and the least useful to a guest; consider denying it
even when the rest of `system-info` is allowed.

**No policy installed ⇒ allow-everything.** `HostPolicy(r) == nil` is the
default opt-in posture (`capabilities.go` `HostPolicy` doc): the
OS-backed defaults run ungated. Sandboxes opt *in* to restriction by
installing a policy that denies `system-info`.

## 8. Overlap

- **`boru:os` (`Os`).** Shares the `system-info` cap and the
  fingerprint/determinism concern, but reports the **OS process**
  (hostname, pid, cwd, env) rather than the **Go runtime**
  (GOOS/GOARCH/NumCPU/version). No word overlap; the two are
  complementary host-introspection modules and a host typically allows
  or denies `system-info` for both together.
- No other module touches this domain.

## 9. Examples (args-before form)

Zero-arg words need no preceding stack value:

```
import "boru:runtime"

Runtime.goos                          # → "linux"
Runtime.goarch                        # → "amd64"
Runtime.num-cpu                       # → 8
Runtime.version                       # → "go1.24"

# platform-conditional behaviour
(Runtime.goos eq "windows") if [".\\bin"] ["./bin"]

# sizing work to the machine
Runtime.num-cpu mul 2                 # → 16  (worker pool hint)

# under a deny-system-info policy, any of these returns the policy error
Runtime.num-goroutine                 # → <policy denied> when system-info denied
```

## 10. Open questions / out of scope

- **Out of scope:** the mutating/tuning half of `runtime` —
  `GOMAXPROCS(n)` *as a setter*, `GC`, `SetFinalizer`, `Gosched`,
  `LockOSThread`, the `MemStats` / `ReadMemStats` block, `Stack`,
  `Callers`, `runtime/debug`, `runtime/pprof`. These are either
  effectful (a `process`-class gate, not `system-info`) or deep
  introspection with no clean boru value shape.
- **Out of scope here:** `runtime.NumCgoCall`, `runtime.Compiler`,
  `runtime.GOROOT` (deprecated) — niche; can be added later if a
  use-case appears (each would also gate on `system-info`).
- **Open:** should `num-goroutine` ship at all? It exposes live
  scheduler state, is rarely actionable from a query, and is the most
  non-deterministic word. Proposal: ship it but document that a
  determinism sandbox should deny it; revisit if unused.
- **Open:** memory stats (`MemStats.Alloc`, etc.) — a future
  `boru:runtime` addition or a separate diagnostics module? Deferred;
  the shape (a big options Map) and the gating both need their own
  design.

## 11. Implementation sketch (wiring checklist — no code)

These are pure-ish constant readers, but because of the `system-info`
gate they still touch the capability/policy seam — so the wiring sits
between `math.go` (pure) and `io.go` (capability-backed):

1. **Seam (light).** Either add a tiny `HostSysInfo` provider to
   `lang/go/capabilities/capabilities.go` (a `FixedClock`-style fake for
   determinism) returning `goos`/`goarch`/`num-cpu`/`num-goroutine`/
   `max-procs`/`version`, with an OS-backed default that delegates to
   `runtime.*`; or, minimally, have each handler call
   `HostPolicy(r).CheckGlobal("system-info")` and then `runtime.*`. The
   provider form is preferred because it makes the determinism stub
   possible (recommended).
2. **Policy.** No new scope: `system-info` is already in
   `policy.GlobalOps`. No `policy.GlobalsFor` change is needed because
   the check is a direct `CheckGlobal("system-info")`, not a
   scope/op pair.
3. **Module.** `BuildRuntimeModule(parent) (native.ModuleDesc, error)`
   in `lang/go/modules/runtime.go`: fresh
   `subReg := native.DefaultRegistry()`, register the inner
   `RuntimeNatives` (each handler: `CheckGlobal("system-info")` then
   read via the provider), wrap each as an `FnDef` into an
   `*OrderedMap` keyed by word, export under `"Runtime"`,
   `ID: parent.Modules.NextID()`. Inner sigs: **`BarrierPos: 0`**
   (zero-arg constants — match `math-pi`).
4. Register `BuildRuntimeModule` in the `modules` map in
   `lang/go/modules/modules.go`.
5. **Docs.** `lang/go/modules/docs_runtime.go` →
   `registerDocs("boru:runtime", {…})`, one line per export
   (`TestModuleExportDocs` enforces).
6. **Spec.** `lang/spec/module-runtime.tsv` — rows lead with
   `import "boru:runtime"`; use a **fixed `system-info` provider** so
   `goos`/`num-cpu`/`version` are reproducible, and pair every positive
   row with an `ERROR:<substring>` sibling (deny-policy row + wrong-arity
   row). `describe` surfaces the module live via `stampExportProvenance`.
