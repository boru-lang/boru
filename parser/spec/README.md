# parser/spec — the parser-level parity corpus

The declarative contract `parser/go` and `parser/ts` are both held to. One set
of files, two runners (`parser/go/parserspec_test.go`,
`parser/ts/src/parserspec.test.ts`), no shared code between them.

## Why it exists

`parse-battery.test.ts` opened by declaring "The Go parser is the REFERENCE:
every construct converts to the same value stream." Nothing checked that.
When it was checked for the first time, **15 of 254 rows encoded a render
`parser/go` does not produce** (design/TS-PARITY-AUDIT.0.md) — the battery
had been pinning TypeScript's own behaviour, and the twins could drift with
no gate noticing.

The other twin pairs already had a shared corpus — `eng/spec` for the
engines, `core/spec` for the cores. The parser was the only pair without one.

A parser-level oracle *was* designed: `parser/go/streamdump_test.go` and
`parser/ts/src/streamdump.ts` both dump the value stream for every `eng/spec`
row. Nothing ever ran the comparison — no target, no CI step — so with
`STREAMDUMP_FILE` unset the Go side dumps to a temp dir and discards it.
`make crossdiff` does compare the two *engines*, but hard-fails only when
both produce a value and the values differ, so a parser-level difference that
still evaluates alike is invisible to it.

## Two runners, no shared code

Each runner reads the files and implements the escape decoding and the render
independently, in about forty lines. That is the same discipline `core/spec`
uses, for the same reason: shared scaffolding can hide one bug from both
engines (design/CORE-GO-TS-DEFECTS.0.md, blind spot 9), and a shared reader
would hide exactly the class of defect this corpus was built to catch.

## Format

Tab-separated. A line is a **comment** only when it starts with `#` *and*
contains no tab — `#` is boru's own comment marker, so sources begin with it,
and treating every `#` line as a comment silently drops those rows. Blank
lines are ignored; a whitespace-only source is a real row, not a blank one.

Every column is escaped (`\n`, `\t`, `\\`) so one row is exactly one line. A
render can itself contain a newline — XML text spanning lines does — and an
unescaped one splits the row and truncates it silently. Both failure modes
above were live bugs during this corpus's construction, which is why both are
called out here.

### `parse.tsv` — the contract

```
src	expected
```

`expected` is the canon of the parsed value stream (each value rendered,
space-joined), or `ERR ` and the first line of the error text. **Both engines
must produce it.**

### `divergent.tsv` — the parity debt

```
src	go	ts	note
```

Sources the two engines render differently. Each runner asserts its **own**
column, so both suites stay green while the difference stays pinned: if either
engine moves, its column fails and the row must be re-examined rather than
silently re-baselined. Each runner also **fails a row whose `go` and `ts`
columns are equal** — a fixed divergence must move to `parse.tsv`, or this
file stops being an honest debt list.

The file is **shrink-only**. Adding a row needs the justification a
`//covergate:allow` does (design/COVERAGE-ALLOWLIST.10.md): a reviewed reason
the difference is not simply a bug to fix.

## The current debt

**Nine rows, in two classes, both properties of the ENGINES rather than of
either port.**

| rows | what | why it is not a port bug |
|---:|---|---|
| 1 | nesting depth: Go guards at 10,000 levels, TS refuses at 500 | the tabnas TS rule engine recurses per level and blows the JS call stack near 900, before any converter counter can fire. Measured: 600 parses, 1,000 overflows. Closing it means making the TS parser iterative, not raising a constant. |
| 8 | `[Map<]`, `[(1]`, `{a: (1}` and kin: Go names the offending token, TS cannot | the TS rule engine STOPS at its maximum-iteration bound and returns the partial root without erroring. `parse()` watches the step count and raises, so both sides now give **the same code** and differ only in TEXT. |

The second class is worth reading as a success rather than a debt entry:
before the step-cap guard, TS **accepted** all eight — `[Map<]` parsed to an
empty value stream and `Map<]` to a bare `word(Map)`. A silent wrong answer
became a wrong message.

Zero is still the goal state, and both runners assert the invariant that
keeps it honest: a row whose two columns are EQUAL fails, so a fixed
divergence has to move to `parse.tsv` rather than sit here looking like
debt.

## History

The ledger held **15 of 254 rows** when it was first checked, and was driven
to zero on 2026-08-08. Measuring what had never been measured then added
three BigDecimal rows, and those were closed too — by giving `core/ts` a
real arbitrary-precision decimal (`core/ts/src/decimal.ts`, a scaled bigint
on apd's model) in place of the binary64 payload that could not represent
what Go represented. What the original fifteen turned out to be:

| class | rows | resolution |
|---|---:|---|
| big-decimal canon | 8 | `core/ts` had `TBigInteger`/`TBigDecimal` as types with **no constructor and no render arm** — every big number lost its `0d` marker |
| typed-container canon | 3 | **both** engines wrong: Go dropped the element-type tag, TS leaked `word(...)`. `REFERENCE.md:228`/`:1224` settle it as `[:Integer]` |
| marker canon | 1 | **TS was right**: `REFERENCE.md:415` makes `end` the word and `;` its synonym; Go rendered it empty, so `1 ;` reparsed as a bare `1` |
| error text | 1 | the two jsonic ports disagree about whether `1_` lexes as a number; TS's fallback now reproduces Go's classification |
| behavioural | 2 | `1e400` → TS had the `float_overflow` refusal but only on a path `1e400` never took |

| BigDecimal range/scale | 3 | found later by measurement; `core/ts` had a binary64 payload, so `0d1e400` overflowed to Infinity, `0d1e-400` underflowed to **zero**, and `0d0.30` lost its scale |

Worth keeping in view: the `go` column was **not** the reference in two of
the five original classes. The header's warning that it is "the reference by
convention, not by proof" was load-bearing.

Two of the eighteen were invisible to the 1765-row `parser-crossdiff`,
because both engines were erroring-by-rendering rather than disagreeing on a
value. `scripts/parity-probe.sh` is what found them.

The corpus grew from 370 rows to 535 while driving `parser/ts` from 93.97%
to 100% line coverage, and the growth found three more defects the crossdiff
could not:

- an EMPTY `${}` interpolation in an XML **attribute** folded to `""` in Go
  and stayed a runtime hole in TS — Go's nil/empty conflation, mirrored
  anyway because parity with the shipped language is the contract;
- the eight rule-step-cap shapes above, where TS silently accepted programs
  Go rejects;
- an error raised while CONVERTING `${…}` rendered its two halves in the
  opposite order (`interpolation expression error:` before or after
  `[boru/float_overflow]:`).

Each was found by sweeping the regions the coverage report called
uncovered, which is the general lesson: **an uncovered branch in one port is
where a divergence hides**, because nothing has ever compared the two there.
