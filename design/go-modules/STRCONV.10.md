# `boru:strconv` — Go `strconv`

> **Status: design proposal, not implemented.** A curated, hand-written
> native module wrapping Go's `strconv`. Read
> [README.10.md](README.10.md) first for the shared conventions this note
> assumes.

## 1. Package & status

Go [`strconv`](https://pkg.go.dev/strconv) converts between primitive
values and their textual representations: parse a string to an int /
uint / float / bool, format a number / bool back to a string, and
add/strip Go-syntax quoting. This note specifies an idiomatic boru
surface over that package. Nothing is implemented yet.

## 2. Why curated

The raw `go:` reflection bridge would surface `strconv.ParseInt(s
string, base int, bitSize int) (int64, error)` verbatim — three
arguments, a `(value, error)` pair, and a `bitSize` knob that is
meaningless in boru (a boru Integer **is** an `int64`, so `bitSize` is
always 64). The curated surface drops `bitSize`, splits the
base-10-vs-explicit-base cases into two readable words, and collapses
every `(value, error)` into value-or-error via `r.BoruError`. Numeric
parsing/formatting is common enough in data wrangling to earn a
first-class, documented, tested surface.

## 3. Import & namespace

```
import "boru:strconv"        # binds the Strconv namespace
```

The bare package name `strconv` does not clash with any builtin type
(`String`, `Path`, `Time`, …) or existing module namespace, so no
`-util` suffix is needed (per the naming rule in `lang/go/CLAUDE.md`
"Package layout"). Words are dot-accessed: `Strconv.parse-int`,
`Strconv.format-float`, etc.

## 4. API

Signatures are **top-first, sig order** (position 0 is the top of the
stack). All inner native sigs use `BarrierPos: -1` so the infix form
dispatches.

| Go symbol | boru word | signature (top-first) | one-line doc | boru-ish refinement |
|---|---|---|---|---|
| `Atoi(s) (int,err)` | `parse-int` | `[String] -> Integer` | Parse a base-10 integer string. | `Atoi` ≡ `ParseInt(s,10,64)`; `(int,err)` → value-or-error `parse-int`. |
| `ParseInt(s,base,bitSize) (int64,err)` | `parse-int-base` | `[Integer base, String s] -> Integer` | Parse a signed integer in the given base. | Dropped `bitSize` (boru Integer is int64). base is the top arg so infix reads `s parse-int-base base`; `0` base means infer from prefix. `(int64,err)` → value-or-error. |
| `ParseUint(s,base,bitSize) (uint64,err)` | `parse-uint` | `[Integer base, String s] -> Integer` | Parse an unsigned integer in the given base. | Dropped `bitSize`. Result returned as Integer; overflow past int64 max errors `parse-uint`. |
| `ParseFloat(s,bitSize) (float64,err)` | `parse-float` | `[String] -> Float` | Parse a floating-point string. | Dropped `bitSize` (boru Float is float64). `(float64,err)` → value-or-error `parse-float`. |
| `ParseBool(s) (bool,err)` | `parse-bool` | `[String] -> Boolean` | Parse a boolean ("1","t","true","0","f","false",…). | `(bool,err)` → value-or-error `parse-bool`. |
| `FormatInt(i,base) string` | `format-int` | `[Integer] -> String` | Format an integer in base 10. | Specialised to base 10 (the common case); total, no error. |
| `FormatInt(i,base) string` | `format-int-base` | `[Integer base, Integer i] -> String` | Format an integer in the given base (2–36). | base is the top arg; out-of-range base errors `format-int-base`. |
| `FormatFloat(f,fmt,prec,bitSize) string` | `format-float` | `[Integer prec, Atom fmt, Float f] -> String` | Format a float with a verb and precision. | Dropped `bitSize`. `fmt` byte → single-char Atom (`'f'`,`'e'`,`'g'`,…); `prec` = -1 for shortest; unknown verb errors `format-float`. |
| `FormatBool(b) string` | `format-bool` | `[Boolean] -> String` | Format a boolean as "true"/"false". | Total, no error. |
| `Quote(s) string` | `quote` | `[String] -> String` | Wrap a string in a Go-syntax double-quoted literal. | Total, no error. |
| `Unquote(s) (string,err)` | `unquote` | `[String] -> String` | Strip Go-syntax quoting from a quoted literal. | `(string,err)` → value-or-error `unquote` on malformed input. |

Notes:
- `Itoa` / `Atoi` are the base-10 shortcuts for `FormatInt` / `ParseInt`;
  they are folded into `format-int` / `parse-int` rather than given
  separate words.
- `fmt` for `format-float` is a single-character Atom so `'f'`/`'e'`/`'g'`
  read naturally at the call site; the handler takes the first rune as
  the format byte.

## 5. Types

Scalars only — String, Integer, Float, Boolean, Atom. No opaque handle
type, no `RegisterExternalBuiltin` / FixedID is needed. Convert at the
boundary with `core.FromNative` / `core.ToNative` (`core/go/gobridge.go`).

## 6. Errors

Go `error` returns unwrap to a `BoruError` via `r.BoruError(code,
detail, word)` with a kebab-case code matching the word:

| code | raised when |
|---|---|
| `parse-int` | `Atoi` fails (non-numeric / overflow). |
| `parse-int-base` | `ParseInt` fails, or base out of 2–36 (and ≠ 0). |
| `parse-uint` | `ParseUint` fails, or the value exceeds int64 max. |
| `parse-float` | `ParseFloat` fails. |
| `parse-bool` | `ParseBool` does not recognise the string. |
| `format-int-base` | base out of range 2–36. |
| `format-float` | unknown format verb, or empty `fmt` Atom. |
| `unquote` | `Unquote` rejects a malformed quoted literal. |

Guard each arg with `AsConcreteString` / `AsConcreteInteger` /
`AsConcreteFloat` / `AsConcreteBoolean` before use; never panic
(`core/go/CLAUDE.md` "Panic Prevention").

## 7. Policy / capabilities

None — pure value conversion, runs under any policy.

## 8. Overlap

None with an existing module. The core engine has literal parsing in
the parser, but no boru-user-facing word converts a runtime String to an
Integer/Float; `boru:strconv` fills that gap. `Fmt.format` (see
[FMT.10.md](FMT.10.md)) does printf-style formatting of arbitrary
values — `Strconv.format-*` is the scalar-specific, base-aware path.

## 9. Examples (args-before form)

```
import "boru:strconv"

"42" Strconv.parse-int                 # 42
"ff" Strconv.parse-int-base 16         # 255   (infix form: s word base)
"3.14" Strconv.parse-float             # 3.14
"true" Strconv.parse-bool              # true
255 Strconv.format-int                 # "255"
255 Strconv.format-int-base 16         # "ff"
3.14159 Strconv.format-float 'f' 2     # "3.14"   (f, args-before prec/verb)
"he\tllo" Strconv.quote                # "\"he\\tllo\""
"\"hi\"" Strconv.unquote               # "hi"
"abc" Strconv.parse-int                # ERROR:parse-int
```

## 10. Open questions / out of scope

- **AppendInt / AppendQuote family** — append-to-byte-slice variants are
  out of scope; boru has no Bytes type (see [BYTES.10.md](BYTES.10.md))
  and string building is already idiomatic via templates / `concat`.
- **QuoteRune / QuoteToASCII / AppendQuoteRuneToGraphic** — niche quoting
  variants deferred until a real need appears; `quote` covers the common
  case.
- **Base inference (`base 0`)** — `parse-int-base` accepts base 0 to mean
  "infer from `0x`/`0o`/`0b`/`0` prefix" (Go semantics). Whether to also
  give that a named convenience word (`parse-int-auto`) is left open.

## 11. Implementation sketch

Wiring checklist — no Go code here. Reference: `lang/go/modules/math.go`
(`BuildMathModule`, the pure-module canonical builder).

- `lang/go/modules/strconv.go` — `BuildStrconvModule(parent *native.Registry)
  (native.ModuleDesc, error)`: make an isolated `native.DefaultRegistry()`
  sub-registry, register a `StrconvNatives []native.NativeFunc` slice
  (each inner sig `BarrierPos: -1`, zero-arg constants would use `0` —
  none here), wrap each word as an `FnDef` export into an
  `*OrderedMap`, return `ModuleDesc{ID: parent.Modules.NextID(),
  Exports: {"Strconv": …}}`.
- Register `"strconv": BuildStrconvModule` in the `modules` map in
  `lang/go/modules/modules.go`.
- `lang/go/modules/docs_strconv.go` — `registerDocs("boru:strconv",
  map[string]string{…})` with a one-line summary per export
  (`TestModuleExportDocs` enforces completeness).
- `lang/spec/module-strconv.tsv` — `input⇥expected⇥description` rows,
  each leading with `import "boru:strconv"`; every positive row paired
  with an `ERROR:<substring>` negative sibling (Test discipline,
  `lang/go/CLAUDE.md`).
- No FixedID entry (no external type), no policy wiring.
