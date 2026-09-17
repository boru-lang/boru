# boru Code Review Report

**Date:** 2026-04-05  
**Scope:** `boru/` (architecture, performance, maintainability, gotchas, non-standard patterns, and duplication)  
**Method:** static review of core runtime, parser, query engine, native word registration, and supporting types.

---

## Executive Summary

The codebase is ambitious and feature-rich, with good test density and clear intent comments. The main risks are **structural complexity concentration** (very large core files), **runtime safety inconsistencies** (panic-prone type assertions despite a no-panic design goal), and **duplication-driven drift risk** (repeated type registries and boilerplate native registration patterns).

The highest-priority improvements are:

1. Reduce panic surface by replacing direct type assertions in handlers with safe accessors and explicit errors.
2. Break up parser and engine "god files" into subpackages/modules with smaller bounded responsibilities.
3. Centralize duplicated type-name registries and native-word registration patterns.
4. Reduce avoidable allocation and chain-walk hotspots in context and schema merging logic.

---

## Key Findings

## 1) Architecture: Core logic is concentrated in very large files (high change-risk)

- `internal/engine/engine.go` is ~2249 lines.
- `internal/engine/query.go` is ~1875 lines.
- `internal/parser/parse.go` is ~1189 lines.

These files each blend multiple concerns (parsing grammar config + semantic conversion, query DSL + SQL generation + execution lifecycle, stack mechanics + dispatch + tracing + error shaping). This drives merge conflicts, slows onboarding, and increases regression risk.

**Recommendation**
- Split each into focused units:
  - `engine`: dispatch, stack ops, control-flow, error helpers.
  - `query`: builder state, SQL emitters, materialization lifecycle, clause parsers.
  - `parser`: token setup, grammar rules, conversion phases.
- Add package-private interfaces between these units to reduce internal coupling.

---

## 2) Safety gotcha: direct `Data.(type)` assertions remain in hot paths

There are still direct type assertions in builtin handlers, e.g.:

- `native_string_upper.go`: `args[0].Data.(string)`
- `native_string_lower.go`: `args[0].Data.(string)`
- `native_definition_undef.go`: `args[1].Data.(FnUndefInfo)`

The codebase explicitly states a no-panic policy and acknowledges type-literal/nil-data pitfalls, but these assertions can panic if assumptions are violated by parser/runtime evolution or future signature changes.

**Recommendation**
- Replace direct assertions with `AsX()` helpers or guarded `v, ok := ...` style.
- Add a focused panic-guard test suite for every word still using assertion-style extraction.
- Consider a lint rule (or CI grep gate) forbidding `\.Data\.\(` in runtime handlers except explicitly allowed locations.

---

## 3) Duplication: type-name registries are repeated across layers

There is a `typeNames` mapping in both:

- `internal/engine/engine.go`
- `internal/parser/parse.go`

These lists are not identical in purpose or contents over time and are vulnerable to drift (new type added in one place, forgotten in the other).

**Recommendation**
- Create one canonical exported/internal provider (e.g. `engine.TypeNameTable()`), used by parser and engine.
- Add a unit test that validates consistency between parse-time and run-time type resolution behavior.

---

## 4) Duplication/complexity: native word registration has heavy boilerplate

There are many small `native_*.go` files (dozens), often repeating the same registration scaffold and near-identical handler shape.

This style is readable per-word but scales poorly:
- difficult bulk changes (signature behavior updates, tracing hooks, safety checks)
- easy for subtle inconsistency in error wording/arg extraction
- repetitive tests needed for similar behavior classes

**Recommendation**
- Introduce small helper constructors for common unary/binary families (string transforms, boolean ops, math ops).
- Keep one-file-per-domain if preferred, but reduce per-word custom scaffolding.

---

## 5) Performance: `ObjectTypeInfo.AllFields()` reconstructs inherited map on every call

`AllFields()` recursively rebuilds parent fields and then overlays own fields each call. In inheritance-heavy usage this can become a repeated allocation hotspot.

**Recommendation**
- Cache merged field maps in `ObjectTypeInfo` with invalidation only when the type definition changes.
- If immutability of type definitions is guaranteed post-registration, precompute once.

---

## 6) Performance & complexity: context chain update walks entire stack/prototype graph

`Registry.UpdateCtxStoreChain()` iterates every context stack entry and may walk each prototype chain to patch copy-on-write roots.

This is logically correct but can become O(stack_depth × chain_depth), and complexity grows as nested evaluation/module scopes increase.

**Recommendation**
- Maintain parent/root identity metadata to short-circuit scans.
- Consider storing explicit root pointers so only affected branches are touched.

---

## 7) Encapsulation gotcha: mutable internal slices are returned directly

- `OrderedMap.Keys()` returns `m.keys` directly (not a copy).

Callers can mutate returned slices and accidentally corrupt internal state ordering assumptions.

**Recommendation**
- Return a defensive copy from `Keys()`.
- Audit similar APIs for direct exposure of internal mutable storage.

---

## 8) Inconsistency vs policy: panic-oriented API docs exist in core containers

`ReadList.Get(i)` documents “Panics if out of bounds.” That conflicts with the global “panics must never occur” posture unless strictly internal and always bounds-guarded.

**Recommendation**
- Add `GetOk(i) (Value, bool)` or return an error in external-facing contexts.
- Restrict panic-capable helpers to truly internal, proven-safe call paths and annotate accordingly.

---

## 9) Query subsystem has tight coupling of parse/build/execute responsibilities

`query.go` currently contains:
- DSL word registration
- condition parsing
- SQL string generation
- source materialization into SQLite temp tables
- set op orchestration and cleanup behavior

This makes behavioral changes risky (e.g., SQL generation tweaks can unintentionally affect lifecycle and temp-table handling).

**Recommendation**
- Separate into:
  - `query_words.go` (boru word handlers)
  - `query_ast.go` / `query_clause.go` (parsed clause model)
  - `query_sql.go` (SQL emitter)
  - `query_exec.go` (materialization/cleanup)

---

## 10) Non-standard maintainability pattern: parser builds grammar inline in one function

`Parse()` dynamically defines tokens, matchers, and grammar modifications inline with conversion logic nearby. This is powerful but difficult to reason about incrementally.

**Recommendation**
- Extract named builder functions per feature family:
  - base tokens
  - interpolation grammar
  - paren grammar
  - pair/optional-field grammar
- Keep parse pipeline staged and explicit (lex setup → grammar setup → conversion).

---

## 11) Potential SQL lifecycle overhead in chained query operations

`QueryBuilder.Materialize()` ensures sources/joins/set-op branches by loading temp tables and dropping them with deferred cleanup. For long query pipelines or nested subqueries, repeated temp-table creation/drop may become expensive.

**Recommendation**
- Add optional temp-table reuse for the duration of a pipeline/materialization context.
- Instrument with lightweight profiling counters (tables created, rows copied, query duration).

---

## 12) Positive note: test coverage breadth is strong

The repository has broad tests across parser, engine, native modules, query paths, and integration scenarios. This is a major strength and reduces the risk of incremental refactoring.

**Recommendation**
- Preserve this strength by adding characterization tests before structural refactors of parser/engine/query subsystems.

---

## Prioritized Action Plan

### P0 (Safety)
1. Remove/guard remaining direct `Data.(type)` assertions in runtime handlers.
2. Add panic-regression tests for all builtin words still using unsafe assertions.

### P1 (Architecture)
1. Split `engine.go`, `query.go`, `parse.go` into focused files/modules.
2. Centralize type-name registry shared by parser/runtime.

### P2 (Performance)
1. Cache `AllFields()` results for object types.
2. Optimize context chain rewrite strategy.
3. Measure and reduce temp-table churn in query materialization.

### P3 (Maintainability)
1. Add native-word registration helpers to reduce boilerplate.
2. Return defensive copies from APIs exposing mutable internals.

---

## Closing

The project already has strong domain depth and extensive tests. Addressing the structural concentration and panic-safety inconsistencies would materially improve long-term velocity and runtime robustness without changing boru semantics.
