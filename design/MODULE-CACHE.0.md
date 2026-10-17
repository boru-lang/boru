# MODULE-CACHE.0 — File-Module Caching & the Singleton Question

This document captures the feasibility findings for adding a
`require.cache`-style per-registry **file-module cache** to boru, and the
one semantic decision that gates it.

It is a sibling to:

- **IMPORTS.10** — the surface behaviour of the `import` word.
- **NATIVE-MODULES.10** — the `boru:<name>` native-module path.

Status: **analysis only — not implemented.** The current behaviour
(every file `import` re-runs the module body) is unchanged. This doc
records the design so the work can be picked up deliberately rather than
slipped in, because a cache is *not* a transparent optimization — it
changes observable semantics (see §3).

## 1. Background — what happens today

There are two import dedup behaviours, and they diverge:

- **Native modules** (`boru:math`, …) are loaded **at most once per
  registry**. `resolveNativeMod` (`lang/go/native/native_module_module.go`)
  checks `r.Modules.IsLoaded(name)` and short-circuits; the load set
  lives on `ModuleRegistry.loaded` (`core/go/modules.go`).
- **File modules** (`./lib.boru`, bare names, …) are **never cached**.
  `loadFileModule` (`native_module_module.go:248`) re-reads, re-parses,
  and re-runs the file body on **every** `import`. Two imports of the
  same file run its body twice and produce independent exports.

Consequence: a diamond import — `a` and `b` both `import "./util.boru"`,
both imported by `main` — evaluates `util` twice. There is also **no
circular-import support**: with no in-flight registry to hand back, an
`a → b → a` cycle re-evaluates rather than resolving to a partial export
set (contrast Node, which returns the partially-populated `exports`).

## 2. Proposed design

The cache fits in the channel built for the native-module resolver fix
(see the `InheritConfig` work on `core/go/modules.go`).

### 2.1 Storage

Add a field to `ModuleRegistry` (`core/go/modules.go`), next to the
existing `loaded` set and `seq` counter:

```go
type ModuleRegistry struct {
    loaded      map[string]bool
    seq         int
    fileModules map[string]ModuleDesc // resolved-abs-path → cached desc
    InitFunc    func(*Registry)
    Resolver    func(name string, r *Registry) (ModuleDesc, error)
}
```

### 2.2 Cache key

The **resolved absolute path** already computed inside `loadFileModule`:

```go
resolved := resolveImportPath(parent, path)   // native_module_module.go:253
```

The resolved path (not the source string) is the correct key: the same
relative path `./util.boru` resolves differently from different
`BaseDir`s, and bare-name / `boru.json`-`main` / `index.boru` resolution
all collapse to one concrete path here.

### 2.3 Lookup / store

In `loadFileModule`, after computing `resolved`:

1. If `parent.Modules.fileModules[resolved]` is present, return it.
2. Otherwise run the body as today, store the resulting `ModuleDesc`
   under `resolved`, and return it.

### 2.4 Graph-wide sharing — rides on `InheritConfig`

`fileModules` is a `map` (reference type). Adding it to
`ModuleRegistry.InheritConfig` means **every module sub-registry in the
import graph shares the same cache by reference**, with no per-call-site
plumbing — the exact "a new config field is inherited everywhere by
default" property `InheritConfig` was introduced to provide. `a` and `b`
importing `./util.boru` then hit one shared map automatically.

This is the same propagation channel as the sub-import resolver fix: the
bug fix and this feature reuse one mechanism.

## 3. The decision: a cache *is* singleton semantics

This is not an internal optimization that can be added invisibly. You
cannot have both "the same instance is returned to every importer" and
"the body re-runs with fresh state." A cache necessarily switches boru to
**Node-style singleton modules**, with two observable changes:

### 3.1 Module-private state becomes shared

A module that keeps private mutable state and exports closures over it:

```boru
def m {count: 0}
def bump fn [[] [Integer] [m set 'count' ((m get 'count') add 1)]]
export "Counter" {bump: bump}
```

Today each importer gets a fresh `m`. With a cache, all importers share
one `m`. This is mechanically guaranteed: export maps are `*OrderedMap`
pointers, and exported `FnDef` values carry `Registry: modReg`
(`resolveModuleExport`, `native_module_module.go:397`) — on a cache hit
every importer reuses the one `modReg` and its export maps. This is
*exactly* Node's `require.cache` behaviour and is usually what people
want, but it is a behaviour change that existing boru programs can
observe.

### 3.2 Load-time parent context freezes

`RunModuleBody` pushes the **parent's** context at load
(`PushExisting(parentCtx)`, `native_module_module.go:67`). On a cache
hit the body does not re-run, so the module retains the **first**
importer's context view. Minor and documentable (module bodies rarely
read parent context at load time), but real.

## 4. Edge cases to handle

1. **Check-mode vs run-mode passes.** `import` runs in check mode too
   (every `import` signature is `RunInCheckMode: true`,
   `native_misc.go:187`). A cache on a long-lived registry must not let
   a `ModuleDesc` produced during the check pass leak into the run pass.
   Mitigation: key/scope the cache per pass, or clear it between passes.

2. **Long-lived registries.** A registry is reused across inputs in the
   REPL (`cmd/go/internal/repl`) and in the wasm playground, which
   constructs one `instance` and calls `Run` in a loop
   (`wpg/wasm/main.go:18,35`). A registry-lifetime cache would not pick
   up file edits between REPL lines — identical to Node's persistent
   `require.cache`, but a deliberate choice (see §5).

3. **Circular imports (bonus).** Once a cache exists, inserting an
   in-flight **placeholder** `ModuleDesc` *before* running the body
   gives Node's partial-exports cycle-breaking almost for free, turning
   today's "no cycle support" into real support. Optional phase 2.

## 5. Options (as presented)

| Option | Behaviour | Trade-off |
|---|---|---|
| **Per-run singleton cache** (recommended) | Cache lives for one top-level `Run` and its whole import graph; cleared at the end of each `Run`. | Diamond imports dedupe; module state shared within one execution; REPL/wasm re-read edited files on the next line. Node-style singletons without the stale-file surprise. |
| **Registry-lifetime cache** | Cache persists for the registry's whole lifetime. | Matches Node's `require.cache` exactly; fastest; a long-lived REPL/wasm session won't see file edits until restart. |
| **Per-run cache + circular-import support** | Per-run cache plus an in-flight placeholder. | Genuine import cycles resolve via partial exports instead of re-evaluating. |
| **Status quo** | Re-run per import. | No semantic change; redundant re-evaluation; no cycle support. |

## 6. Recommendation

If pursued, **per-run singleton cache** (optionally with circular-import
support) is the sweet spot: it brings file modules to parity with the
already-cached native-module path, deduplicates diamond imports, and
adopts Node-style sharing **within a single program execution** without
the "edited file isn't picked up" surprise that a registry-lifetime
cache imposes on the REPL and the web playground.

The work is small and localized — one field on `ModuleRegistry`, one
line in `InheritConfig`, a lookup/store wrapper in `loadFileModule`, and
a clear between passes — but it should ship only with an explicit
decision to adopt singleton semantics (§3), since that is observable to
existing programs.
