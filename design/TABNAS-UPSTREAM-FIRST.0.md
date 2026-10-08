# TABNAS-UPSTREAM-FIRST.0 — tabnas parser defects go upstream, not into a shim

**Status:** ACCEPTED as [ADR-014](../ADR.md#adr-014) · **Recorded:** 2026-08-10
(maintainer instruction: "no workarounds for issues in tabnas parsers —
these always require upstream fixes")

boru's two parsers are thin layers over a pair of dependency twins:
`github.com/tabnas/jsonic/go` over `github.com/tabnas/parser/go`, and
`@tabnas/jsonic` over `@tabnas/parser`. The dependency is where the lexer
lives. When the twins disagree with each other — or one of them is simply
wrong — the defect is upstream's, and this note records why it must be
fixed there rather than papered over in `parser/go` / `parser/ts`.

## The case that settled it

On 2026-08-09 a review found that boru's DATA-decode seam
(`SafeParseData`) had lost jsonic's lenient superset: `{v: 1.2.3}` raised,
and `1.x` silently decoded to the LIST `[1 '.x']`. The cause was boru's
own language-grammar decimal matcher installed into a plain data parser.

The first fix was a boru-side shim: a DATA mode that DECLINES a run it
cannot claim, handing it back to the stock scanner. That is textbook
workaround reasoning — "let the dependency deal with it" — and it was
**wrong in a way the author could not see from one port**. Declining is
only safe where the two stock scanners AGREE, and for dot-adjacent
base-prefixed runs they did not:

```
[0xFF.5]      Go: ["0xFF.5"]      TS: [[], "xFF.5"]   ← phantom element, '0' eaten
[-0xFF.5, 9]  Go: ["-0xFF.5", 9]  TS: [null, null, "xFF.5", 9]
{a:0xFF.5}    Go: {a:"0xFF.5"}    TS: throws
```

An adversarial verification pass caught it. The **second** workaround
layered on the first: claim the whole prose run as one `#TX` token in both
ports, so neither reaches the disagreeing stock path. It worked — and it
left two divergent matcher code paths, ten spec rows, and a comment
explaining a dependency bug, all inside boru.

Two workarounds deep, the actual defect was still in the dependency, still
unreported, and still shipping to every other tabnas consumer.

## What upstream-first bought instead

The defects were written up against the **bare** dependency — no boru
matchers, no boru seams, so each reproduction stood on its own — and fixed
upstream. Four issues, all resolved in `jsonic v0.6.0` / `parser v0.8.0`:

| # | Defect | Resolution |
|---|---|---|
| 1 | TS mangled dot-adjacent base-prefixed runs (fabricated elements, eaten characters) | TS declines the whole run; both ports agree |
| 2 | TS could not represent map insertion order for integer-like keys | opt-in `map:{ordered:true}` + `keyOrder()` side-channel |
| 3 | Go read `+_1` as the number 1, silently swallowing `+_` | both ports treat it as lenient text |
| 4 | `Make` bumped a package-global id counter unsynchronized (a data race) | `atomic.Int64`, documented concurrent-safe |

Every one of those had a boru-side cost that now goes away: a matcher arm,
a process-wide construction mutex, the `GuardMake` wrapper whose whole
signature existed to hold that mutex, and two documented seam asymmetries
that no shared spec row could express — the ordered-map fix turned the
second into ten `data.tsv` rows.

A FIFTH shim fell out of the same upgrade without being reported: the TS
rule engine used to bound its main loop and, on reaching the bound, simply
STOP — no throw, partial root returned — so `[Map<]` parsed to an empty
value stream where Go reported `unexpected \`]\``. boru carried
`watchRuleSteps`/`ruleStepCap` to detect that by counting rule iterations
and re-deriving the library's own cap formula. v0.8.0 throws instead, and
`errors.ts` translates it to the byte-identical diagnostic — notes and
position included, which is why 928 tests pass with the guard deleted. The
shim's own doc had predicted its death: *"If the library ever changes the
formula this guard stops firing rather than firing early — the fail-open
direction."* It stopped firing, silently, and only the 100%-coverage gate
noticed. A shim that reads the dependency's internals (`j.internal()`,
`config.rule.maxmul`) cannot help but rot like this.

## The rule, and its boundary

**Upstream.** A defect in dependency behaviour — lexing, token
boundaries, value construction, concurrency — is reproduced against the
bare dependency, reported, fixed, and consumed as a version bump.
`scripts/parity-probe.sh` is the instrument: it runs a source through both
ports and prints AGREE or DIFFER, which is what turns "TS looks wrong"
into a defect report with a table.

**Boru's own.** A divergence produced by boru's *grammar layer* — the
arrowfold, dotchains, its custom matchers, its recovery diagnostics — is
boru's to fix, recorded in `NUR.md` and pinned in
`parser/spec/divergent.tsv`. The 2026-08-09 sweep's 55 divergences across
nine classes are all of this kind; upgrading the dependency left them
byte-identical, which is the evidence that the two categories really are
distinct.

**The tell.** If you are about to write a comment that explains a
dependency's bug in order to justify boru code, stop: that comment belongs
in an upstream issue. A shim that survives the fix becomes dead weight
nobody dares delete, because its comment no longer describes reality.

## Closed upstream — and the lesson in how it was nearly missed

**Resolved 2026-08-11 in tabnas parser v0.8.3.** Everything below described
a live divergence until then; it is kept because the *method* is the point,
and because the retirement it enabled is the ADR's full lifecycle running
once end to end: shim → report → upstream fix → version bump → shim
deleted → NUR closed. `matchBasePrefixRun` is gone from both ports.

v0.8.3 fixed two divergences, not one. The dot boundary described below,
and a second found while writing the report up: the TS regex accepted only
lowercase marker letters where Go accepted either case, so `0XFF` was
`#NR(255)` in Go and `#TX` in TS. **One shim was masking both** — which is
its own argument for reporting rather than shimming, since a shim hides
whatever else shares its shape.

Retirement was proven by comparing the TWO ports: 25 shapes AGREE under
`scripts/parity-probe.sh`, `parser-crossdiff` IDENTICAL over 1765 rows, and
the `lex.tsv` §base-prefixed boundaries rows still passing — those rows pin
the previous behaviour, so passing means the renders did not move, and
because they cover the shapes that actually diverged their silence is
evidence rather than the absence of it. See `NUR.md` §NUR061.

The base-prefix **overflow** divergence reported after v0.8.0 is FIXED in
jsonic v0.6.1 / parser v0.8.1: both ports now round the true value, so
`0xFFFFFFFFFFFFFFFF` is 1.8446744073709552e19 in both — including the
trap the report flagged, where naively keeping `ParseInt`'s clamped value
would have agreed on `0x8000000000000000` by coincidence and been off by
2× one literal later.

`matchBasePrefixRun` still could not be retired, for a **different**
divergence at the dot boundary. With `.` registered as a token — as boru's
grammar does and the bare dependency does not — the stock scanners
disagree on the run BEFORE the dot: Go calls `0xFF` in `0xFF.5` a number,
TS calls it text. Deleting the arm splits three shapes that agree today:
`0xFF.5` renders Go's *"a number has no members to access with `.`"*
against TS's `255.5`, and `[0xFF.5]` against `[255.5]`.

**The disagreement is conditional, and that matters more than it looks.**
TS classifies the run before the dot by looking PAST the dot, so only some
follow characters diverge: `0xFF.5`, `0xFF.0`, `0xFF.` and `0xFF.e5` split
(as do `0o17.5` and `0b101.5`), while `0xFF.a`, `0xFF.F`, `0xFF.x`,
`0xFF.z` and `0xFF.5x` agree in both ports with no arm at all. A sweep
that probes an agreeing shape and concludes the arm is dead repeats the
mistake below by another route — which is why `lex.tsv` now pins one of
each, labelled. General text/fixed-token boundaries agree throughout
(`abc.def`, `x0.5`, `0z.5`, `00.5`, `0x.5`), so the divergence is narrow
rather than a general text-matcher defect.

**How that was nearly missed is the point.** The arm was deleted in both
ports, and both suites passed — 928 TS tests and the whole Go suite, clean
— because no corpus row covered the shape. Go's token stream was
byte-identical before and after, which made the retirement look proven; the
divergence only surfaced on comparing the TWO ports' token streams, and
then loudly under `scripts/parity-probe.sh`. A shim's own passing suite is
not evidence that the shim is dead. `lex.tsv`'s §base-prefixed boundaries
rows now pin the classification — including one deliberately AGREEING
shape, labelled as such — so the next sweep fails instead of looking safe,
and cannot mistake an agreeing probe for proof.

Held open by `NUR.md` §NUR061 — the auditable owner ADR-014 asks for. The
report is written and measured in
[legacy/TABNAS-DOT-BOUNDARY-REPORT.0.ignore](legacy/TABNAS-DOT-BOUNDARY-REPORT.0.ignore), ready
to file verbatim; it is **not yet submitted**, so no issue URL exists to
link. When one does, it belongs in NUR061 and in both matcher comments.
The arm goes when the boundary is fixed upstream.

## Cost, honestly

Upstream-first is slower when you need the fix now, and it does not work
at all if upstream is unresponsive. The escape hatch is explicit rather
than silent: a temporary shim is permitted only with the upstream issue
linked in its comment and a `NUR.md` record holding it open, so the shim's
removal is somebody's job rather than nobody's.
