# Coverage allowlist — provably-unreachable defensive guards

`make cover-gate` (ADR-008) enforces **100% unit-test coverage of every
*reachable* Go statement**. A small, reviewed set of exclusions covers the
blocks that are **provably unreachable through any test** while remaining
**valuable defensive code**. Each exclusion is an inline
`//covergate:allow <reason>` comment on the guard's own opening line — the
proof travels WITH the guard, so a refactor that shifts lines never
invalidates it. This note explains what may be excluded, why, and the
discipline that keeps it honest.

## The mechanism — an inline pragma

An exclusion is a line comment whose token IS the marker, placed on the
guard's opening line:

```go
if unit < 0 || unit >= len(es.fnRecs) { //covergate:allow bounds guard — callers pass a StartFnCompile unit
	return
}
```

`covergate` (run with `-root <repo>`) parses every source file referenced by
the merged profile with `go/parser` and collects the lines whose trailing
comment token is `//covergate:allow` — detection is comment-exact, so the
marker's appearance in prose (this document, covergate's own doc comment) or
in a string literal is ignored. The reason after the marker is **mandatory**
(covergate rejects a bare marker): it is the proof of unreachability.

A pragma excludes the **uncovered** coverage block(s) opening on its line.
Go's cover tool often splits an `if guard {` line into a *reaching* block
(covered — control gets there) and the *guard-body* block (uncovered — the
guard never fires); the pragma excludes only the uncovered body and counts
the covered reaching block normally, so line granularity never over-excludes
real coverage.

This replaced the historical line-keyed `test/go/covergate/allowlist.tsv`,
whose `<file>:<startLine>.<col>,<endLine>.<col>` keys went stale on every
refactor that inserted or deleted a line above a guard — a large,
signal-free churn (a single diagnostics refactor re-pointed ~80 entries at
once). The pragma carries no absolute coordinate, so that churn is gone.

## Why an allowlist rather than "literally 100%"

Two roads lead to a green 100% gate for a block that no input can execute:

1. **Delete the guard.** Correct *only* when the code has no defensive
   value — a tautological comparison, a shadowed branch, a `return` after
   a loop that always returns. Those are removed at source under ADR-005
   ("no deliberate dead code"), not allowlisted.
2. **Keep the guard, exclude it.** Correct when the block is an
   **error-propagation or safety arm** whose call cannot fail *today* but
   defends against a future change, an external library, or data
   corruption. Deleting it to win a coverage number would silently drop a
   real error if that call ever started failing — a net regression. These
   are allowlisted.

The whole 2026-07 quality program pushed coverage from 78.5% to 100% of
reachable statements precisely by making the hard edges reachable with
**mocking seams** (design/TEST-SEAMS.10.md). The allowlist is the residue:
the arms where a seam would either sit on a hot dispatch path (measurable
allocation regression — see the reverted `screenResults` method-value
seam) or cannot force the failure at all because the guarded call is
total for every constructible input.

## What is excluded (categories)

Each exclusion is a `//covergate:allow <reason>` comment; the reason names
the category. The categories:

- **§engine** — interpreter step/dispatch defensive index and error arms
  in `core/go/engine.go`. Tautological index comparisons (a backward scan
  makes `fwdIdx < endIdx` a theorem), `stepEnd` error arms (`stepEnd`
  only ever returns nil), and `TopTypeBody`/`Lookup` arms shadowed by a
  preceding `Defs.Top`. Reachable only through the lang layer or a
  pathological program, which the cross-package census already runs.
- **§compiler** — bytecode compiler / VM defensive arms
  (`callable_words`, `emit`, `vm`, `lower`, `method_shape`, `user_poly`):
  the belt-and-braces result screens and match-drift refusals that a
  compiled-reachable handler cannot trigger (the emitter refuses the
  words that could produce a tape-coupled token; a real island re-steps
  one rather than returning it).
- **§kernel** — shared-assertion and gate-guaranteed kernel guards: an
  `As<T>` error arm sitting under an `Is<T>` guard that keys on the
  *identical* type assertion, or a read that its enclosing `containsFlex`
  / `containsSharedMutable` gate already proved will succeed.
- **§native** — native word-handler defensive error-propagation and
  same-assertion guards (the `anyToValue` convert-back arms that
  `valueToAny`'s JSON-shaped output never trips, `Is*`/`As*` pairs).
- **§modules** — module provably-invariant and grammar-defensive guards
  (a beam-trim whose queue is invariant at length 1, an emit-kind lookup
  whose input is always registered, re-validation shadowed upstream).
- **§formatter** — the pretty-printer's inline/tokenizer guards (a
  word-scan that always advances, single-line returns unreachable once
  the multi-line branch was taken).
- **§crypto** — vault crypto **defense-in-depth**: hex/base64 decodes of
  values the vault itself just minted with the inverse encoder, and a
  reserved-key check shadowed by an earlier alias validation. Kept
  deliberately — these defend stored-secret integrity against a future
  format change, and are the category we are *least* willing to delete.
- **§misc** — the lone `exec.NewServer` error arm (that constructor has a
  single `return s, nil`).

The per-block proofs live in the wave-6…9 coverage-agent reports; the
category reason on each pragma is the durable summary.

## Discipline — the exclusions cannot rot

`covergate` treats the pragmas as **live**, not fire-and-forget:

- If every block on a pragma's line is **actually covered** by some suite,
  the gate **fails** and names the line: the guard became reachable, so
  cover it and delete the pragma ("graduate" it).
- If a pragma line opens **no** profiled block (a refactor moved the guard
  off the line, or deleted it), the gate **fails** as stale.

So the exclusions can only shrink or move deliberately, and — because the
pragma rides on the guard's line — a line-shifting refactor moves the pragma
with the guard rather than dangling a stale coordinate. Adding one is a
reviewed act: it needs a proof that no test can reach the block and an
argument that the guard earns its keep as defence. When in doubt, prefer a
seam (make it reachable) or deletion (if it is truly dead) over a pragma.
