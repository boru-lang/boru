# `boru:csv` — Go `encoding/csv`

> **Status: design proposal, not implemented. OPTIONAL** — its core
> happy-path overlaps the existing `parse csv` / `emit csv` words (see
> §8 and §10). A curated, hand-written native module wrapping Go's
> `encoding/csv`. Read [README.10.md](README.10.md) first for the shared
> conventions this note assumes.

## 1. Package & status

Go [`encoding/csv`](https://pkg.go.dev/encoding/csv) reads and writes
RFC-4180 comma-separated values with configurable knobs:
`Reader.ReadAll` decodes all rows, `Writer.WriteAll` encodes them, and
`Comma` (delimiter), `Comment`, `FieldsPerRecord` (row-width policy),
`LazyQuotes`, and `TrimLeadingSpace` tune non-standard dialects. This
note specifies an idiomatic boru surface over that package. Nothing is
implemented yet.

## 2. Why curated

The raw `go:` reflection bridge would surface the stateful
`*csv.Reader` / `*csv.Writer` objects, `io.Reader`/`io.Writer`
constructors, the mutable knob fields, and a `([][]string, error)`
boundary — none of which read well in boru. The curated surface hides
the Reader/Writer objects entirely (whole-String in, whole-value out),
turns the knobs into an options Map, and collapses `(value, error)`
into value-or-error via `r.BoruError`. The value it adds over
`parse csv` is **dialect control** (custom delimiter, comment char,
lazy quotes) — see §8.

## 3. Import & namespace

```
import "boru:csv"           # binds the Csv namespace
```

The bare package name `csv` does not clash with any builtin type
(`String`, `Path`, `Time`, …) or existing module namespace, so no
`-util` suffix is needed (per the naming rule in `lang/go/CLAUDE.md`
"Package layout"). Words are dot-accessed: `Csv.parse`,
`Csv.parse-records`, `Csv.format`.

## 4. API

Signatures are **top-first, sig order** (position 0 is the top of the
stack). All inner native sigs use `BarrierPos: -1` so the infix form
dispatches. The options Map is the **top** arg so the infix form reads
`source Csv.parse opts`; passing no opts uses RFC-4180 defaults.

| Go symbol | boru word | signature (top-first) | one-line doc | boru-ish refinement |
|---|---|---|---|---|
| `Reader.ReadAll() ([][]string,err)` | `parse` | `[Map opts, String src] -> List[List[String]]` | Parse CSV text to a list of rows, each a list of String fields. | Reader object hidden; knobs → `opts` Map (§ below); fields are always String (no numeric coercion); `([][]string,err)` → value-or-error `parse`. |
| `Reader.ReadAll()` + header zip | `parse-records` | `[Map opts, String src] -> List[Map]` | Parse CSV using the first row as headers, yielding one Map per data row. | No Go equivalent — a curated convenience that zips the header row against each data row; ragged rows error `parse`. |
| `Writer.WriteAll(rows) err` | `format` | `[Map opts, List[List[String]] rows] -> String` | Format a list of String rows as RFC-4180 CSV text. | Writer object hidden; output returned as a String rather than written to an `io.Writer`; non-String cells error `format`. |

**Options Map** (all keys optional; absent → Go's RFC-4180 default):

| key | type | Go field | meaning |
|---|---|---|---|
| `delimiter` | String (single char) | `Comma` | field separator (default `,`); first rune taken. |
| `comment` | String (single char) | `Comment` | lines starting with this rune are skipped (default none). |
| `lazy-quotes` | Boolean | `LazyQuotes` | allow bare `"` in unquoted fields and non-doubled quotes (default false). |
| `trim-leading-space` | Boolean | `TrimLeadingSpace` | trim leading white-space in each field (default false). |

Notes:
- `FieldsPerRecord` is not a free-form knob: `parse` uses Go's default
  (the first row sets the required width, ragged rows error). A future
  `{fields-per-record:N}` / `{ragged:true}` option is left to Open
  questions.
- `format` always uses `\r\n` line terminators (Go's
  `csv.Writer` default) and only honours `delimiter` from the opts Map
  (the other knobs are read-only properties).

## 5. Types

Scalars, List, Map only — String fields, `List[List[String]]` tables,
`List[Map]` records, an options Map. The Go `*csv.Reader` / `*csv.Writer`
are constructed, used, and discarded inside each word, so **no opaque
handle type** is surfaced — no `RegisterExternalBuiltin` / FixedID is
needed. Convert at the boundary with `core.FromNative` / `core.ToNative`
(`core/go/gobridge.go`); guard the list/map args with
`RequireConcreteList` / `RequireConcreteMap`.

## 6. Errors

Go `error` returns unwrap to a `BoruError` via `r.BoruError(code,
detail, word)` with a kebab-case code:

| code | raised when |
|---|---|
| `parse` | `ReadAll` fails — malformed quoting, wrong field count, or (for `parse-records`) a row wider/narrower than the header. |
| `format` | a cell is not a String, the row list is not a `List[List[String]]`, or `WriteAll` errors. |

A bad option (e.g. a multi-rune `delimiter`) errors under the calling
word's code with a clear detail. Guard args with `AsConcreteString` /
`RequireConcreteList` / `RequireConcreteMap` before use; never panic
(`core/go/CLAUDE.md` "Panic Prevention").

## 7. Policy / capabilities

None — pure in-memory string transformation, runs under any policy.

## 8. Overlap

**This is the load-bearing section — be candid.** CSV is already
exposed twice in the tree:

- `parse csv '<text>'` (the `boru:parselang` `parse_csv` export,
  `docs_parselang.go`) — decodes CSV to a List of rows, each a List of
  fields, with **numeric fields coerced to numbers**, zero config.
- `emit csv <table>` (the `boru:emitlang` `emit_csv` export,
  `docs_emitlang.go`) — RFC-4180 CSV from a Table or list of records,
  with a single `{separation:sep}` knob.

**The dividing line:** `parse`/`emit csv` are the **zero-config
convenience** for the standard comma dialect (and `emit` even infers
CSV as a Table's natural format via `emit_auto`). `boru:csv` is the
**low-level reader/writer with dialect control** — custom `delimiter`,
`comment` lines, `lazy-quotes`, `trim-leading-space`, strict field-count
enforcement, and all-String fields (no numeric coercion) for callers who
need faithful round-tripping of non-standard CSV/TSV/PSV. `boru:csv` does
not move or change the existing words.

If a project never touches a non-comma dialect or quirky quoting,
`boru:csv` buys nothing over `parse`/`emit csv` — hence the **OPTIONAL**
flag (§10). Promote it only if real dialect-control demand appears.

## 9. Examples (args-before form)

```
import "boru:csv"

"a,b\n1,2" Csv.parse {}                         # [["a","b"],["1","2"]]
"a;b\n1;2" Csv.parse {delimiter:";"}            # [["a","b"],["1","2"]]  (infix form: src word opts)
"name,age\nAda,36" Csv.parse-records {}         # [{name:"Ada", age:"36"}]
[["a","b"],["1","2"]] Csv.format {}             # "a,b\r\n1,2\r\n"
"\"unterminated" Csv.parse {}                   # ERROR:parse
[[1,2]] Csv.format {}                           # ERROR:format   (cells must be String)
```

## 10. Open questions / out of scope

- **OPTIONAL / redundancy.** Per §8 the comma happy-path duplicates
  `parse`/`emit csv`. Decide before implementing: ship `boru:csv` only if
  dialect control (custom delimiter/comment/lazy-quotes) is genuinely
  wanted; otherwise drop this module and let `parselang`/`emitlang`
  own CSV. Listed last in the roster's encoding block for that reason.
- **`FieldsPerRecord` / ragged rows** — only Go's default (first row
  sets width) is exposed; a `{ragged:true}` opt to allow variable-width
  rows is deferred.
- **Numeric coercion** — `boru:csv` deliberately keeps fields as String
  (faithful round-trip); the numeric-coercion behaviour stays the
  exclusive province of `parse csv`.
- **Streaming row-at-a-time** — Go's incremental `Reader.Read()` /
  `Writer.Write()` are out of scope; the whole-String words cover the
  in-memory case, and streaming belongs with `boru:io`.

## 11. Implementation sketch

Wiring checklist — no Go code here. Reference: `lang/go/modules/math.go`
(`BuildMathModule`, the pure-module canonical builder).

- `lang/go/modules/csv.go` — `BuildCsvModule(parent *native.Registry)
  (native.ModuleDesc, error)`: make an isolated `native.DefaultRegistry()`
  sub-registry, register a `CsvNatives []native.NativeFunc` slice (each
  inner sig `BarrierPos: -1`), wrap each word as an `FnDef` export into
  an `*OrderedMap`, return `ModuleDesc{ID: parent.Modules.NextID(),
  Exports: {"Csv": …}}`.
- Register `"csv": BuildCsvModule` in the `modules` map in
  `lang/go/modules/modules.go`.
- `lang/go/modules/docs_csv.go` — `registerDocs("boru:csv",
  map[string]string{…})` with a one-line summary per export
  (`TestModuleExportDocs` enforces completeness).
- `lang/spec/module-csv.tsv` — `input⇥expected⇥description` rows, each
  leading with `import "boru:csv"`; every positive row paired with an
  `ERROR:<substring>` negative sibling (Test discipline,
  `lang/go/CLAUDE.md`).
- No FixedID entry (no external type), no policy wiring.
