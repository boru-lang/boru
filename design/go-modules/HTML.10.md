# `boru:html` — Go `html`

> **Status: design proposal, not implemented.** A curated, hand-written
> native module wrapping Go's `html`. Read [README.10.md](README.10.md)
> first for the shared conventions this note assumes. This is a tiny
> two-word module.

## 1. Package & status

Go [`html`](https://pkg.go.dev/html) provides exactly two functions for
escaping and unescaping HTML text: `EscapeString` (replaces the five
special characters `<`, `>`, `&`, `'`, `"` with their entities) and
`UnescapeString` (reverses it, decoding the full entity set). This note
specifies an idiomatic boru surface over that package. Nothing is
implemented yet.

## 2. Why curated

The `go:` reflection bridge would surface the two functions almost
verbatim (`EscapeString(string) string`), so the curated win here is
small — kebab names, namespace grouping, and a consistent String→String
shape. The module exists for completeness and discoverability rather
than reshaping: HTML entity escaping is a common one-liner that
deserves a documented, tested home.

## 3. Import & namespace

```
import "boru:html"          # binds the Html namespace
```

The bare package name `html` does not clash with any builtin type
(`String`, `Path`, `Time`, …) or existing module namespace, so no
`-util` suffix is needed (per the naming rule in `lang/go/CLAUDE.md`
"Package layout"). Words are dot-accessed: `Html.escape`,
`Html.unescape`.

## 4. API

Signatures are **top-first, sig order** (position 0 is the top of the
stack). All inner native sigs use `BarrierPos: -1` so the infix form
dispatches.

| Go symbol | boru word | signature (top-first) | one-line doc | boru-ish refinement |
|---|---|---|---|---|
| `EscapeString(s)` | `escape` | `[String] -> String` | Escape `<`, `>`, `&`, `'`, `"` to HTML entities. | Total, no error; plain String→String rename. |
| `UnescapeString(s)` | `unescape` | `[String] -> String` | Decode HTML entities back to their characters. | Total, no error; decodes the full entity set, not just the five `escape` produces. |

Notes:
- Both functions are total — neither Go function returns an error
  (`UnescapeString` leaves unrecognised `&…;` sequences untouched), so
  neither word can fail. The module has no error codes (§6).
- `escape`/`unescape` are not exact inverses: `unescape` decodes
  numeric and named entities that `escape` never emits, so
  `unescape (escape s)` recovers `s` but `escape (unescape s)` may
  differ for inputs containing other entities — standard `html`
  package behaviour.

## 5. Types

Scalars only — String. No opaque handle type, no
`RegisterExternalBuiltin` / FixedID is needed. Convert at the boundary
with `core.FromNative` / `core.ToNative` (`core/go/gobridge.go`); guard
the arg with `AsConcreteString`.

## 6. Errors

None — both words are total String→String transforms with no failure
mode, so no kebab error codes are defined. (A non-String argument is
rejected by signature dispatch before the handler runs.)

## 7. Policy / capabilities

None — pure value transformation, runs under any policy.

## 8. Overlap

None with an existing module. No boru-user-facing word does HTML entity
escaping today.

**Out of scope — `html/template`.** Go's
[`html/template`](https://pkg.go.dev/html/template) is a *separate,
much larger* surface: it does **contextual, auto-escaping** template
rendering (escaping is chosen per HTML/JS/CSS/URL context, not a single
blanket entity pass). That belongs with the templating work, not here —
cross-reference [TEXT-TEMPLATE.10.md](TEXT-TEMPLATE.10.md) (`boru:template`,
the `text/template` wrapper) for the template surface, and note that
`html/template`'s auto-escaping is explicitly **not** covered by either
note. `Html.escape` is the manual, single-shot entity escaper only.

## 9. Examples (args-before form)

```
import "boru:html"

"<b>hi & bye</b>" Html.escape          # "&lt;b&gt;hi &amp; bye&lt;/b&gt;"
"&lt;a&gt;" Html.unescape              # "<a>"
"a &amp; b" Html.unescape              # "a & b"
"plain text" Html.escape               # "plain text"   (nothing to escape)
"&notanentity;" Html.unescape          # "&notanentity;"   (unrecognised, untouched)
```

(No `ERROR:` row: neither word has a failure mode — see §6. The spec
suite instead pins a type-rejection case, e.g. a non-String argument
failing signature dispatch.)

## 10. Open questions / out of scope

- **`html/template`** — contextual auto-escaping is a separate, larger
  surface and is OUT of scope here (see §8; cross-ref
  [TEXT-TEMPLATE.10.md](TEXT-TEMPLATE.10.md)).
- **Attribute / URL / JS escaping** — context-specific escapers live in
  `html/template`'s internals, not the `html` package; not provided.
- **Negative-test shape** — with no error mode, the Test-discipline
  negative sibling is a type-rejection row (non-String arg) rather than
  an `ERROR:<code>` from the handler.

## 11. Implementation sketch

Wiring checklist — no Go code here. Reference: `lang/go/modules/math.go`
(`BuildMathModule`, the pure-module canonical builder).

- `lang/go/modules/html.go` — `BuildHtmlModule(parent *native.Registry)
  (native.ModuleDesc, error)`: make an isolated `native.DefaultRegistry()`
  sub-registry, register an `HtmlNatives []native.NativeFunc` slice (two
  words, each inner sig `BarrierPos: -1`), wrap each word as an `FnDef`
  export into an `*OrderedMap`, return `ModuleDesc{ID:
  parent.Modules.NextID(), Exports: {"Html": …}}`.
- Register `"html": BuildHtmlModule` in the `modules` map in
  `lang/go/modules/modules.go`.
- `lang/go/modules/docs_html.go` — `registerDocs("boru:html",
  map[string]string{…})` with a one-line summary for `escape` and
  `unescape` (`TestModuleExportDocs` enforces completeness).
- `lang/spec/module-html.tsv` — `input⇥expected⇥description` rows, each
  leading with `import "boru:html"`; pair the positive rows with a
  type-rejection negative sibling (Test discipline,
  `lang/go/CLAUDE.md`).
- No FixedID entry (no external type), no policy wiring.
