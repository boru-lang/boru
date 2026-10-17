# Pluggable extension modules: coding, micro-format, number systems

> **Caveat (2026-07): the parse/mini/emit registration surfaces this note
> models itself on have since been REMOVED.** The `parse`/`mini`/`emit`
> kind namespaces are frozen (the atom sugar is un-namespaced, so the kind
> sets are fixed built-in lists; the `register` words are tombstones
> raising `<surface>_registry_frozen`), and custom languages are Function
> VALUES built with `NewParseLangFn` / `NewMiniLangFn` / `NewEmitLangFn`
> and bound via `(*Boru).DefineValue` (or a `def`). Three capabilities were
> retired without replacement: boru-side mini compile hooks
> (`register-compiled`), custom-kind member types (`MiniLang.<Name>`), and
> `+kind/src/` sugar for custom kinds. The host-registration RECIPE below
> (validate / store under a capability / gate on policy) is still the
> template for capabilities with their OWN namespaces (log sinks, TUI
> backends, and this note's proposals) — just no longer a description of
> the parse/emit code.

> **Status: design proposal, not implemented.** This note specifies three
> new extensible capabilities — `boru:coding` (escaping / encoding),
> `boru:micro-format` (email, iCal, vCard, …), and **higher-order number
> systems** for the math module (complex, rational, quaternion) — built on
> **one shared host-registration mechanism modelled on the existing
> parse/emit framework**, with per-extension boilerplate **scaffolded by
> [jostraca](https://github.com/jostraca/jostraca) templates**. Read
> [NATIVE-MODULES.10.md](NATIVE-MODULES.10.md) and
> [go-modules/README.10.md](go-modules/README.10.md) first.

## 1. Motivation

boru already has a proven pattern for *pluggable, named handlers*: the
parse/emit framework. `boru:parselang` / `boru:emitlang` keep a registry of
format kinds (`json`, `csv`, `yaml`, …), expose `parse <kind>` /
`emit <kind>`, and let a host add new kinds at runtime via
`RegisterHostParser` / `RegisterHostEmitter` — or boru code add them via a
`register` word (`lang/go/modules/parselang.go`, `emitlang.go`).

Three upcoming needs are the *same shape* — a registry of named codecs
behind a small dispatch surface, extensible without editing core:

1. **`boru:coding`** — escape / unescape / encode / decode for host
   syntaxes (html, url, shell, json-string, regex, quoted-printable, …).
2. **`boru:micro-format`** — parse / emit structured micro-formats (email
   messages, iCal/vCard, HTTP headers, data-URIs, …).
3. **Higher-order number systems** — complex, rational, quaternion as
   first-class numbers that participate in `add`/`mul`/… arithmetic.

Rather than three bespoke designs, this note defines **one extension
mechanism** (§2) reused by all three, **jostraca templates** (§3) that
generate the per-extension Go boilerplate, then the three modules
(§4–§6). The number-systems case needs one genuinely new kernel seam — an
**arithmetic behavior** (§6) — because, unlike codecs, numbers must plug
into the hardcoded arithmetic dispatch.

## 2. The shared extension mechanism (parse/emit-style)

Generalise the parse/emit host-registration pattern into a reusable shape
every extensible module follows. The reference implementation is
`RegisterHostParser` (`lang/go/modules/parselang.go:191`) and
`RegisterHostEmitter` (`emitlang.go:163`).

### 2.1 The pattern (one registry per domain)

Each extensible module owns a registry of named handlers with five parts,
all already demonstrated by parselang/emitlang:

1. **A `Spec` struct** — the registration envelope. parselang's is
   `ParseLangSpec{Name string; Returns []*native.Type; Handler ParseLang}`
   (`ParseLang` is the exported named type of a parse_<lang> function)
   (`parselang.go:147`). Name is a **lowercase, unprefixed** kind atom;
   the framework adds the word prefix (`parse_`, `emit_`, …).
2. **A `RegisterHostX(reg *native.Registry, spec XSpec) error`** entry
   point — validates (lowercase name, non-nil handler, no collision with
   built-ins, no duplicate) and installs.
3. **Registry state on the registry**, stored under a capability key
   (e.g. `"engine.parselang.host"`, `parselang.go:160`) holding pending
   pre-import specs **and** the live module, so registration works both
   **before and after** `import` and is visible to later calls in the
   same session (runtime-pluggable).
4. **A boru-level `register` word** so boru code (not just the host) can
   add a kind — `ParseLang.register` installs a boru fn as a parser.
5. **A handler of the standard shape**
   `func(args []native.Value, named map[string]native.Value, stack []native.Value, r *native.Registry) ([]native.Value, error)`
   converting at the boundary with `eng.FromNative` / `eng.ToNative`,
   erroring via `r.BoruError(code, detail, word)`, never panicking.

### 2.2 What is reused vs new

| Module | Reuses | Adds |
|---|---|---|
| `boru:coding` | the whole pattern (a codec registry) | nothing kernel-level — pure string→string handlers |
| `boru:micro-format` | the **existing** parse/emit registries directly (`RegisterFormatParser`/`RegisterFormatEmitter`, `parselang.go:227`) | a thin micro-format helper layer + typed accessors |
| number systems | the pattern *shape* for the registry | a new kernel **arithmetic-behavior seam** (§6) — numbers must enter `add`/`mul` dispatch |

The first two need **no kernel change** — they are new registries (or
reuse of the parse/emit one). Only number systems touch the kernel.

### 2.3 Policy

Each registry gates through `lang/go/policy`. parse/emit use the
all-or-nothing `formats` scope (`policy.KnownScopes`, gated via
`SetHostFormats`, `lang/go/native/capabilities.go`). `boru:coding` reuses
`formats`; `boru:micro-format` reuses `formats` (it *is* formats);
number-system construction is pure (no gating). Per-kind gating remains a
possible future refinement (today the `formats` gate is module-wide).

## 3. jostraca templates for authoring extensions

[jostraca](https://github.com/jostraca/jostraca) is a code/project
generator that composes a file tree from **React-style components**
(`Project`, `Folder`, `File`, `Content`, `Fragment`, `Slot`, `Inject`,
`Copy`), authored in TypeScript/JS (a **Go port** exists), with `$$path$$`
model-variable substitution and an **`Inject`/`Slot`** mechanism for
updating *existing* files.

Adding any extension above is repetitive Go boilerplate — a `Spec`, a
handler skeleton, a `registerDocs` line, `lang/spec/module-*.tsv` rows,
and a registration call. jostraca templates generate exactly this from a
small model so authors write only the codec body:

- **Templates live in the jostraca project** (the `boru` codec/number-system
  generator), invoked as a dev tool; the repo references them, it does not
  vendor a runtime dependency (generation is build-time, off the hot path
  — consistent with boru staying sealed at runtime,
  [GO-MODULES.10.md](GO-MODULES.10.md)).
- **Model** (per extension): `{ module, kind, namespace, returns, doc,
  examples }`. From it the template emits:
  - a new `File` `lang/go/modules/<module>_<kind>.go` — the `Spec` + a
    `Handler` skeleton with the `r.BoruError` scaffolding and a `TODO` body;
  - `Content` into `docs_<module>.go` (the `registerDocs` line) and
    `lang/spec/module-<module>.tsv` (positive + `ERROR:` rows) via
    **`Inject`/`Slot`** markers, so the existing files are updated in place;
  - an **`Inject`** into the registry's built-in list / the `modules` map
    when the kind ships in-tree.
- **Authoring a new codec** then reduces to: run the generator with the
  model, fill the handler body, `make fmt vet lint test`. The generator
  guarantees the docs/spec/registration stay in lockstep with the export
  (which `TestModuleExportDocs` and the spec runner already enforce).

The same generator templates cover all four registries (parse, emit,
coding, micro-format) and the number-system scaffold, since they share the
§2 shape — one template family, parameterised by domain.

## 4. `boru:coding` — escaping / unescaping / encoding / decoding

A module for **text → text** transforms in host syntaxes: escaping and
its inverse, plus text encodings. Namespace `Coding`.

### 4.1 Surface

A generic, registry-backed dispatch (mirroring `parse <kind>`), plus the
built-in kinds:

| word | signature (top-first) | meaning |
|---|---|---|
| `escape` | `[Atom kind, String s] → String` | Escape `s` for the named syntax. |
| `unescape` | `[Atom kind, String s] → String` | Inverse of `escape` (error `unescape` on malformed). |
| `encode` | `[Atom kind, String s] → String` | Encode `s` in the named text encoding. |
| `decode` | `[Atom kind, String s] → String` | Inverse of `encode` (error `decode`). |
| `kinds` | `→ List[Atom]` | List registered coding kinds (like `ParseLang.kinds`). |
| `register` | `[fn, Atom kind, Atom op] →` | Install a boru fn as a coding kind. |

Built-in **escape** kinds: `html`, `xml`, `url` (percent), `shell`,
`json` (string body), `csv` (field), `regex` (metacharacters), `c`
(backslash escapes), `sql` (single-quote doubling). Built-in **encode**
kinds: `url` (percent-encode), `html-entities`, `quoted-printable`,
`punycode`. Example flat aliases may be provided for the common pair
(`html-escape`/`html-unescape`) — the canonical form is the dispatch.

```
import "boru:coding"

"a<b>" html/q Coding.escape              # "a&lt;b&gt;"
"a b&c" url/q Coding.encode              # "a%20b%26c"
"He said \"hi\"" json/q Coding.escape   # "He said \\\"hi\\\""
"a.b*c" regex/q Coding.escape           # "a\\.b\\*c"
"%zz" url/q Coding.decode                # ERROR:decode
```

### 4.2 Types, errors, policy

String in, String out — scalars only, via `eng.FromNative`/`ToNative`. No
external type. Errors: `unescape` / `decode` (malformed input),
`unknown-coding` (unregistered kind atom). Gates on the `formats` scope
(reuses the parse/emit capability seam).

### 4.3 Overlap (consolidation)

`boru:coding` is the **text** coding hub and overlaps several existing
notes; the dividing lines:

- **`boru:bin-util`** owns **binary** base-N of a `Bytes` value
  (base64/hex/base32/base128/ascii85 of raw bytes;
  [go-modules/BIN-UTIL.10.md](go-modules/BIN-UTIL.10.md)). `boru:coding`
  is string-syntax escaping + text encodings. A `Coding.encode base64` on
  a *String* is a convenience that delegates to `bin-util` on the
  string's UTF-8 bytes.
- **`boru:html`** (escape/unescape) is **subsumed** by `Coding.escape html`
  — recommend folding it in and dropping the standalone `boru:html` note.
- **`boru:url`** keeps URL *structure* (parse/build), but its
  `query-escape`/`path-escape` move to `Coding.escape url` /
  `Coding.encode url`. (Update [go-modules/NET-URL.10.md](go-modules/NET-URL.10.md)
  to delegate.)
- **`boru:strconv`** `quote`/`unquote` (Go-syntax string literals) is
  adjacent; expose it as `Coding.escape c` / `unescape c` or keep both and
  cross-reference. Left as an open question (§7).

## 5. `boru:micro-format` — email, iCal, vCard, …

Structured **micro-formats**: small, well-defined textual formats that
parse to / emit from boru Records. Namespace `MicroFormat`.

### 5.1 Built on the existing parse/emit framework

Each micro-format is registered as a **parse kind and an emit kind** via
the existing bridges `RegisterFormatParser` / `RegisterFormatEmitter`
(`parselang.go:227`, `emitlang.go:193`), so they are reachable two ways:

```
import "boru:parselang"   import "boru:micro-format"

# via the generic parse/emit surface (registered kinds):
parse vcard <text>                       # → Record
emit ical <event-record>                 # → String

# via typed accessors on the MicroFormat namespace:
<text> MicroFormat.email                 # → {from, to, subject, date, body, headers}
<event> MicroFormat.to-ical              # → String
```

Initial kinds: `email` (RFC 5322 message → headers Map + body),
`vcard`, `ical` (VEVENT/VTODO), `http-headers`, `data-uri`,
`query-string` (`a=1&b=2` ↔ Map), `cookie`. Each is a parser
(`String → Record`) + emitter (`Record → String`) pair.

### 5.2 Typed accessors

Beyond the raw `parse`/`emit` kinds, `MicroFormat` exposes a typed word
per format that returns a **named Record** (so fields are addressable and
checkable) rather than a generic map — e.g. `MicroFormat.email` yields a
`refine Record [from:String to:List subject:String date:DateTime
body:String headers:Map]`. The Record type is exported
(`MicroFormat.Email`, …) for `x is MicroFormat.Email` checks.

### 5.3 Extension

New micro-formats register through the **same parse/emit host seam** (§2)
— `RegisterFormatParser(reg, "vcard", vcardFormat)` — or boru code via
`ParseLang.register`. The jostraca template (§3) scaffolds the
parser/emitter pair, the typed-accessor word, the exported Record type,
docs, and spec rows. Types: Records/Maps/Lists/scalars +
`DateTime` (reuses `boru:time-util`). Errors: `microformat-parse`
(malformed input), per format. Gates on `formats`.

### 5.4 Overlap

Distinct from the structural-data formats in `parselang`/`emitlang`
(json/yaml/csv) — micro-formats are *domain* records, not general data
trees. `email`/`http-headers` parsing does not duplicate `boru:net`
(which does HTTP transport, not message parsing) or `boru:mail` (the
niche `net/mail` address-only note in go-modules, which `boru:micro-format`
**supersedes** — recommend dropping `NET-MAIL.10.md`).

## 6. Higher-order number systems (math)

Add **complex**, **rational**, and **quaternion** as first-class numbers
that participate in arithmetic — superseding the Map-based
[go-modules/MATH-CMPLX.10.md](go-modules/MATH-CMPLX.10.md) (a complex as
`{re,im}` could be *transformed* but not added, "an odd half-tool"). The
constructors/accessors live in the math module (today `boru:math-util`,
`MathUtil`; see naming note §6.4); the **types and arithmetic seam** are
kernel-level.

### 6.1 The blocker: arithmetic is hardcoded

Core arithmetic (`add`/`sub`/`mul`/`div`/`mod`/`pow`) dispatches through a
fixed **leaf tower** — `towerOps{intFn, bigFn, decFn, fltFn}` keyed by
`numLeaf(v)` over the four built-in leaves
(`lang/go/native/native_helpers.go:8`,`125`; `native_math.go:101`). There
is **no seam** for a new number type to enter `add`. Comparison, by
contrast, *does* have a seam (the `Comparer` behavior, `core/go/compare.go`)
— numbers need the arithmetic equivalent.

### 6.2 The new seam: an `Arith` behavior + number-system registry

Introduce a kernel **`Arith` behavior** (optional `TypeBehavior`
capability, parallel to `Comparer`):

```
// eng/go (interface only — illustrative)
type Arith interface {
    Add(a, b Value) (Value, error)
    Sub(a, b Value) (Value, error)
    Mul(a, b Value) (Value, error)
    Div(a, b Value) (Value, error)
    Neg(a Value) (Value, error)
    // Pow optional; promotion via the registry (below)
}
```

and a **number-system registry** entry per type:

```
RegisterNumberSystem(reg, NumberSystemSpec{
    Name:    "complex",
    Type:    TComplex,                 // a Scalar/Number/Complex external builtin
    Rank:    rankComplex,              // promotion order vs other number leaves
    Arith:   complexArith{},           // the behavior above
    Promote: func(v Value) (Value, error) { … }, // builtin number → this type
    Parse:   func(s string) (Value, bool),       // literal/string form (optional)
    Format:  func(v Value) string,               // "3+4i"
})
```

Then **one change** to the arithmetic dispatcher
(`numericBinaryHandler`): if either operand's leaf is a registered
higher-order number, pick the **wider** system by `Rank`, `Promote` both
operands into it, and delegate to its `Arith` method — exactly how the
tower already widens `Integer → BigInteger → BigDecimal`. Built-in
leaves keep their fast path untouched; only a non-builtin operand takes
the seam. This is the principled version of "extend the tower" — additive,
no per-call cost for ordinary arithmetic, and open to host-registered
systems.

```
import "boru:math-util"

(MathUtil.complex 3 4) (MathUtil.complex 1 2) add   # 4+6i  (real add dispatch)
(MathUtil.rational 1 2) (MathUtil.rational 1 3) add # 5/6   (exact)
2 (MathUtil.complex 0 1) mul                         # 0+2i  (Integer promoted)
```

### 6.3 The number systems

| System | Type | Payload | order | notes |
|---|---|---|---|---|
| Complex | `Scalar/Number/Complex` | `complex128` (or apd pair for exactness) | partial — equality + magnitude only; `cmp` by (re,im) lexicographic for sort stability, documented | `complex`, `real`, `imag`, `conj`, `abs`, `arg`, `polar` |
| Rational | `Scalar/Number/Rational` | `*big.Rat` | **total** | `rational`, `num`, `den`, exact `div`; auto-reduce |
| Quaternion | `Scalar/Number/Quaternion` | `[4]float64` | none (no `Comparer`) | `quaternion`, components, `conj`, `norm`; non-commutative `mul` |

Each registers as an external builtin (`RegisterExternalBuiltin`,
`core/go/typetable.go`) with `Match`/`Format`/`Equal`, optional `Comparer`,
and the new `Arith`. **FixedID:** allocate a new documented band
**`Scalar/Number` extensions** (proposal: `6000–6099`) in
`core/go/CLAUDE.md` "FixedID Allocation", pinned in
`lang/go/test/fixedid_stability_test.go`.

### 6.4 Naming, module, literals

- The math module is `boru:math-util` (`MathUtil`); there is **no**
  `boru:math` today. The constructors/accessors go in `boru:math-util`
  (keeping one math home), or a dedicated `boru:number` if the math module
  grows too large — open question (§7). The note refers to it as the math
  module either way.
- **Literals** — constructor words (`complex 3 4`, `rational 1 2`) are the
  primary spelling. A literal syntax (`3+4i`, `1/2`) could later be added
  by registering the system's `Parse` with a numeric-literal hook, but is
  out of scope for v1.

## 7. Cross-cutting: errors, policy, FixedIDs

- **Errors** — kebab `r.BoruError` codes, Go errors unwrapped, no panics
  (guard with `AsConcrete*`); per-module codes named above.
- **Policy** — `boru:coding` and `boru:micro-format` gate on the existing
  `formats` scope (reuse `SetHostFormats`/the parse/emit capability seam);
  number-system construction/arithmetic is pure (ungated). Per-kind
  gating is a future refinement.
- **Docs/spec** — every export gets a `registerDocs` line
  (`TestModuleExportDocs`) and `lang/spec/module-<id>.tsv` rows with
  positive + `ERROR:` siblings — all generated by the jostraca template.

## 8. Open questions

- **`strconv.quote` vs `Coding.escape c`** — fold or cross-reference? Lean:
  cross-reference, keep both.
- **Fold `boru:html` / `boru:mail` away** — recommended (subsumed by
  `boru:coding` / `boru:micro-format`); confirm before removing those notes.
  `boru:url` escaping delegates to `boru:coding`.
- **Math home** — number systems in `boru:math-util` vs a new `boru:number`.
- **Complex ordering** — provide a (documented, non-mathematical) total
  order for sort stability, or leave complex un-`Comparer`'d (equality
  only) like quaternion? Lean: lexicographic for sort utility, documented.
- **Arith seam scope** — ship with `add/sub/mul/div/neg`; defer `pow`,
  `mod` (ill-defined for complex/quaternion) to per-system module words.
- **jostraca dependency** — generator is a dev-time tool (Go port), not a
  runtime dep; confirm it stays out of `go.mod` (build-time only).

## 9. Implementation phases

1. **Extension mechanism doc-as-built** — factor the parselang/emitlang
   registration shape into a documented, reusable helper (or just follow
   the pattern per module). No behaviour change.
2. **`boru:coding`** — new module + codec registry (pure string handlers),
   fold in `boru:html`, delegate `boru:url` escaping. (No kernel change.)
3. **`boru:micro-format`** — register the format pairs via
   `RegisterFormatParser`/`Emitter`; typed accessors + Record types;
   supersede `boru:mail`. (No kernel change.)
4. **Number systems** — the kernel `Arith` seam + `RegisterNumberSystem`
   + the dispatcher hook (the one core change), then Complex / Rational /
   Quaternion types and `MathUtil` constructors; FixedID band; supersede
   `MATH-CMPLX.10.md`.
5. **jostraca templates** — the generator family (§3) covering all four
   registries + the number-system scaffold; wire into the dev workflow.

Each phase ships with positive + negative specs (Test discipline,
`lang/go/CLAUDE.md`) and, for the kernel seam, `recover()`-based no-panic
tests.
