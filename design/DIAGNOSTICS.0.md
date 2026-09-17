# DIAGNOSTICS.0 — the structured diagnostic framework

Status: LANDED (phases 1–6 on the fn-afn-signatures branch).
Owner surfaces: eng/go (model + renderer + builders), lang/go (arith
taxonomy, key-miss suggestions), cmd/go (check/run/do/REPL/LSP), wpg.

## Goal

Rust/Elm-grade error UX: every failure report should help FIX the
problem — multiple labeled source locations, expected-vs-found
comparisons, per-overload "why this candidate didn't match" verdicts,
did-you-mean suggestions, plain language, and actionable fixes.
Builds on the loud-failures tradition (design/ERRORS.8.md, the DX
field reports).

## The model (eng/go/boru_error.go)

`BoruError` was EXTENDED in place (it is type-asserted everywhere —
stampErrPos, traps, gates):

- `Spans []DiagSpan` — secondary labeled locations. The error's own
  Row/Col/Src stays the primary span (^^^ underline); secondaries
  render `---` underlines with a label, own `--> file:row:col` header,
  and — via DiagSpan.Source/File — a DIFFERENT file's excerpt for
  cross-file spans (a declaration inside an imported module).
- `Notes []string` — freestanding `= note:` lines.
- `Suggestions []DiagSuggestion{Message, Replacement *string}` —
  `= help:` lines; Replacement (nil = none — the no-zero-value-
  overload rule) is the seed for editor code actions.
- `DeclSite{Pos, Source, File}` — where a contract was DECLARED,
  threaded FnSig.Decl → FrameTailSpec.Decl → ReturnCheckInfo.Decl.

`CheckDiagnostic` grew the same additive fields (`Src`, `Notes`,
`Suggestions`, all omitempty — the JSON/LSP contract only grows).

## Rendering (eng/go/diag_render.go)

ONE renderer: `(e *BoruError) Render(RenderOpts{Color}) string`.
`Error()` IS `Render(RenderOpts{})` — the plain rendering, byte-
identical to the historical output when no structured payload is
present, so the 872 spec ERROR rows, logs, `%w` chains, JSON, and the
wasm playground never see ANSI. Interactive surfaces opt in:
`native.ResolveColor(r, w, mode)` implements `--color auto|always|never`
(auto = real terminal only, NO_COLOR honored) — it lives in the
language layer, not here, because resolving it needs the process
environment and a stat() on the destination; the kernel renderer takes
the verdict as a `RenderOpts` field and never sniffs a terminal itself.
Check diagnostics
render their rich block via `RenderCheckDiagnostic` UNDER the stable
`check: r:c: [sev] code: detail` one-liner (a parsing contract).

## Message builders — the one-text-source rule (eng/go/diag_msg.go)

Generalizes the return_check_msg.go precedent: every family's text is
built in one place and shared by the interpreter, the VM's compiled
guards, the check-pass trap, and the checker. The shared builders
(`noMatchDiag`, `buildReturnTypeError`, `buildReturnCountError`,
`buildNotFoundKeyError`) take DATA, not an Engine — so any surface with
the failing values in hand rebuilds the byte-identical error.

`noMatchDetail(name)` is a function of the word name ALONE — every
surface raises the same head, so the Detail-equality half of the
compiled_fullcorpus gate holds trivially. The RICH payload (received
arguments, per-candidate verdicts, spans, suggestions) is engineered to
be byte-identical across engines too — see **Compiled-mode parity**
below; that is the phase-7 contract, enforced by the extended gate.

## Family contracts

- **Dispatch** (`signature_error` / check `no_signature`): head
  ``cannot call `w` — no signature matches the arguments``; notes =
  received-tuple line, ≤3 per-candidate verdicts from the explain pass
  (below), `…and N more signatures`; suggestions = swap probe ("did you
  swap the arguments?"), forward-parens fix, `boru describe` pointer (≥4
  overloads or truncated list). There is deliberately NO tape-snapshot
  ("stack: …") note — it was interpreter-tape-internal and had no
  compiled-mode equivalent (see Compiled-mode parity); the received-tuple
  note carries the same "what was here" information in plain language.
- **Explain pass** (eng/go/dispatch_explain.go): failure-path-only
  positional probe of every non-fallback overload against the failing
  tuple — arity / slot-type / pattern verdicts, Score-ranked
  nearest-first. NEVER runs in matchSignature's hot loop (a +16%
  dispatch regression was caught earlier on this branch; benchmarks
  confirmed zero success-path cost). Positional fidelity is the
  documented contract; an overload the probe cannot blame is skipped,
  never given a fabricated reason.
- **Undefined words** (`undefined_word`): grep-friendly head kept
  (86 code-pinned rows); Elm voice in suggestions — SuggestNames
  (eng/go/didyoumean.go, OSA Damerau-Levenshtein, banded, distance cap
  by length ≤4→1 ≤8→2 else 3, case-only first, ≤3 results) over
  Registry.SuggestionCandidates(); describe pointer when the nearest
  miss is a builtin. Same hook for strict-read key misses (candidates
  = the container's actual keys) and unknown type names. Suggest,
  never auto-correct.
- **Return contracts** (`type_error`): Detail/hint unchanged (shared
  with the VM); two secondary spans — the produced value (got.Pos,
  skipped when positionless or at the error position) and the
  declaration (DeclSite; zero site attaches nothing — locations are
  never guessed).
- **Parse** (`syntax_error`): tabnas/jsonic failures translated into
  boru-voice BoruErrors (eng/go/parser/parse_error.go); the library's
  always-on ANSI palette, `--internal:` block, and docs link are
  disabled at the source.
- **Arith** (`arith_error`): div/mod-by-zero and apd faults raise
  coded BoruErrors (was bare fmt.Errorf), matching the check pass's
  existing taxonomy.

## Surfaces

- `boru run/do/check --color`, REPL auto-color, `boru build` binaries
  auto-resolve over stderr.
- check: rich block under each one-liner (cmd/go/internal/check).
- LSP: notes/suggestions append to Diagnostic.Message as
  `note:`/`help:` lines.
- wasm/serve: always plain (pinned by TestEvalErrorsArePlain).

## Compiled-mode parity (phase 7)

A compiled-mode error is BYTE-IDENTICAL to the interpreted one — same
Detail, notes, suggestions, and secondary spans (the only tolerated
difference is the primary caret position on a return error: the VM
points inside the shared fn unit where the interpreter points at the
call site). The rule was "errors from compiled mode must be exactly the
same." Three mechanisms achieve it, one per way a compiled program can
error:

1. **Runtime param-contract guards** (vm.go `checkParamContract`,
   `checkNativeParamContract`) rebuild the full dispatch diagnostic at
   run time from the failing values via `runtimeNoMatch` → the shared
   `noMatchDiag`. They hold the same concrete runtime values the
   interpreter's `sigError` sees, so the rebuilt error is identical by
   construction.
2. **The compile-time OpTrap** (`tryRecordUnmatchedDispatchTrap`,
   strict-read miss) serialises the interpreter's OWN error — the full
   `BoruError` including spans/notes/suggestions — into the `TrapSpec`
   via `RecordTrapErr`; the VM rebuilds it at `OpTrap`. Baking is valid
   ONLY when the trap's operands are CONCRETE at compile time
   (definiteness), because then check-time values == runtime values. A
   trap whose window contains a CARRIER declines: the carrier is not
   concrete at compile time, so a baked error could not match the
   interpreter's runtime (concrete-value) one. That decline leaves the
   caller's `MarkUncompilable` refusal standing — the program does NOT
   compile, and at run time it is SILENTLY re-run on the interpreter.
   That re-run is scaffolding absorbing the gap, never a fallback this
   design leans on and never a reason a refusal may stand: the refusal
   is an open DEFECT, owed a proof or a mechanism and tracked to
   closure, and the silence makes it worse by hiding the failure
   instead of reporting it. This supersedes the Phase-6 M4
   carrier-disjointness trap, which is removed; what that removal gives
   up is not just the (error-path) compilation of a terminal tail that
   never runs — it is the compiled program itself, for every shape that
   now declines, until the defect is closed.
3. **Return errors** thread the declaration site (`FnSig.Decl` →
   `CompiledFn.Decl`) and share `buildReturnTypeError` /
   `buildReturnCountError`, so the VM's RET raises the same two spans
   (produced value + declaration) as the interpreter's ReturnCheck.

The interpreter was made UNIFORMLY rich to match: a fallback-dispatch
failure now routes through `sigError` (not the barer `fnFallbackSig`
error) unless the fn courtesy-dispatches a 0-arg overload.

Enforcement: `compiled_fullcorpus_test.go` compares aeC vs aeI Detail,
notes, suggestion messages, and span labels+positions — tolerating only
the primary-caret difference and the incidental value-rendering
non-determinism (counter-based IDs, pre/post-eval operand forms) the
result comparison already tolerates.

## Gallery (generated from real runs)

### Undefined word + did-you-mean

```
error: [boru/undefined_word]: undefined word: pritn
  --> 1:1
  1 | pritn "hi"
      ^^^^^ undefined word: pritn
  = help: did you mean `print`?
  = help: see `boru describe print` for its signatures and examples\n```\n\n(before: `[boru/undefined_word]: undefined word: pritn` + position only)\n
### Dispatch failure — received tuple, candidate verdict, swap probe

```
error: [boru/signature_error]: cannot call `wp` — no signature matches the arguments
  --> 2:1
  1 | def wp fn [[policy:String n:Integer] [Integer] [n]]
  2 | wp 3 "collect"
      ^^ cannot call `wp` — no signature matches the arguments
  = note: the arguments were 3 (an Integer) and 'collect' (a ProperString)
  = note: candidate `wp (String, Integer)` — argument 1: expected String, got 3 (an Integer)
  = note: stack: >>>word(wp)<<< 3 'collect'
  = help: no signature matches (Integer, ProperString); one exists for (String, Integer) — did you swap the arguments? expected: wp policy:String n:Integer\n```\n\n(before: `no matching signature for wp` + an `expected:`/`stack:` hint blob)\n
### Return-type violation — three labeled locations

```
error: [boru/type_error]: f: return value 1: expected String, got Integer
  --> 2:4
  1 | def f fn [[n:Integer] String [n]]
  2 | 42 f
         ^ f: return value 1: expected String, got Integer
  --> 2:1
  2 | 42 f
      -- the returned value was produced here
  --> 1:23
  1 | def f fn [[n:Integer] String [n]]
                            ------ the declaration says `f` returns String
  = value: 42\n```\n\n(before: the header + primary caret only — no declaration or value spans)\n
### Parse error — boru voice

```
error: [boru/syntax_error]: this string is never closed: 'abc
  --> 1:7
  1 | def s 'abc
            ^^^^ this string is never closed: 'abc
  = help: add the closing quote\n```\n\n(before: raw jsonic output with a hardcoded ANSI palette, an `--internal:` block, and the library's docs link)\n
### Arithmetic — coded taxonomy

```
error: [boru/arith_error]: division by zero
  --> 1:4
  1 | 20 div 0
         ^^^ division by zero\n```\n\n(before: a bare code-less `division by zero` with no position or excerpt)\n
## Invariants for future changes

1. Detail text a compiled guard bakes must stay name-only
   deterministic; enrich via Notes/Suggestions.
2. `Error()` stays ANSI-free forever; color is caller-resolved.
3. Never guess a location: no position → say so; zero DeclSite → no
   span; a span into another file carries its own Source/File.
4. The explain pass and every suggestion hook run on failure paths
   only — the dispatch hot loop is untouched (benchmark before
   landing anything that changes this).
5. The `check:` one-liner format and error CODES are byte-stable
   contracts; the rich block and suggestion text are free to evolve
   (spec rows pinning phrases migrate with the change).
