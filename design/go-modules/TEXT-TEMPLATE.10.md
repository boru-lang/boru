# `text/template` → `boru:template`

> **Status: design proposal — not implemented.** A curated, hand-written
> native module wrapping Go's `text/template` with an idiomatic
> ("boru-ish") surface. Read [`README.10.md`](README.10.md) first for the
> shared conventions this note assumes.

## 1. Package & status

Go package: [`text/template`](https://pkg.go.dev/text/template) — Go's
data-driven text templating engine: a template string with `{{...}}`
actions (field access, `range` loops, `if`/`else` conditionals, pipelines)
is parsed, then executed against a data value to produce text. This note
specifies `boru:template` (namespace `Template`), exposing both a one-shot
`render` and a reusable `compile`/`exec` pair. Design proposal; no Go code
exists yet.

## 2. Why curated

The raw `go:text/template` bridge would surface the full builder protocol:
`template.New(name)` → `(*Template).Parse(text)` → `(*Template).Execute(io.Writer, data)`,
with the writer, the `*Template` pointer, and the parse/execute split all
exposed as `Any`-typed plumbing. For a query/data language the common need
is "fill this template with this Map, give me the String" — one call. The
curated surface provides that as `render` (no handle at all), supplies an
`io.Writer`/buffer internally and returns the result String, and offers a
separate `compile`/`exec` pair only for the hot-loop case where re-parsing
per render is wasteful. Errors split cleanly into a parse error and an
execution error.

## 3. Import & namespace

```
import "boru:template"        # binds the Template namespace
```

The bare capitalized package name `Template` is free (not a builtin type —
note `Type`, `Time`, `Path` *are* builtins and so take `-util`, but
`Template` is not — and not an existing module namespace), so **no `-util`
suffix** is needed (per the naming rule in `lang/go/CLAUDE.md` "Package
layout"). Words are dot-accessed: `Template.render`, `Template.compile`, …

## 4. API

Signatures are **top-first, sig order** (position 0 = top of stack), per
the README "Argument order & dispatch" rule. All inner natives use
`BarrierPos: -1` so the infix form `a Template.word b` dispatches (this is
the dispatch requirement pinned by `wrapper_dispatch_test.go`).

| Go symbol | boru word | signature (top-first) | one-line doc | boru-ish refinement |
|---|---|---|---|---|
| `template.New(name).Parse(text)` + `(*Template).Execute` | `render` | `[Map data, String template] -> String` | Parse a template and execute it against data in one call. | **PRIMARY — recommended.** Parses + executes in a single call; **no opaque handle**, fully idiomatic. `data` is the top arg so infix form reads `template Template.render data`. Internally writes to a `bytes.Buffer` and returns its String. A parse failure errors `parse`; an execution failure errors `exec`. |
| `template.New(name).Parse(text)` | `compile` | `[String] -> Template` | Parse a template string into a reusable compiled handle. | **REUSABLE — for hot loops.** Returns an opaque `Template` external-type handle holding the parsed `*template.Template`. Parse failure errors `parse`. |
| `(*Template).Execute` | `exec` | `[Map data, Template tmpl] -> String` | Execute a compiled template against data, returning the result String. | Pairs with `compile`: `data` top arg so infix reads `tmpl Template.exec data`. Re-uses the parsed template, avoiding re-parse per render. Execution failure errors `exec`. |

### One-shot vs reusable — the key design choice

`render` is the recommended primary surface: most call sites render a
template once (or rarely), and a single `template data Template.render`
call with no handle to manage is the idiomatic boru shape — the same
"value in, value out, no protocol" choice this whole roster makes.

`compile` + `exec` exist only for the **hot-loop** case: rendering the
*same* template against *many* data Maps, where re-parsing the template
text on every render is measurable waste. `compile` once, `exec` in the
loop. The cost is that `compile` must hand back an opaque handle (§5), so
reach for it only when the reuse actually matters.

### `data` as a Map

The template's data context is modeled as a boru **Map**: `core.FromNative`
turns it into a `map[string]any`, which `text/template` addresses with
`{{.key}}`. Nested Maps and Lists work through the same bridge
(`{{range .items}}`, `{{.user.name}}`). A non-Map data value (e.g. a bare
String or List as the whole context) can be accepted too — the bridge
hands `text/template` whatever the converted Go value is, and `{{.}}` is
the top-level dot — but Map is the documented common shape.

## 5. Types

Mostly scalars / Map. The one exception is `compile`'s return value: a
parsed `*template.Template` has **no boru counterpart**, so it is held in
an `ExtensionPayload` and surfaced as a registered external type, the
`Template` handle.

This follows the in-tree precedent of **`IO.StreamKind`** (`io.go`):
`MintStreamKind(subReg)` mints a module-owned type into the sub-registry
per import and exports it as a type literal via `NewTypeLiteral`, so
`x is IO.StreamKind` works after import. `boru:template` mirrors that:

- Register the type with **`RegisterExternalBuiltin`** using a **`FixedID`
  from the documented `10000+` host/third-party range**
  (`core/go/CLAUDE.md` "FixedID Allocation"). Unlike `StreamKind` (minted
  fresh per import), a parsed-template handle is a concrete payload type
  whose identity must be **stable** so equality / `is` checks and the
  `fixedid_stability_test.go` guard hold across imports — hence a fixed,
  reserved ID rather than a per-import mint.
- **FixedID-stability obligation:** once allocated, the ID is permanent;
  add it to `lang/go/test/fixedid_stability_test.go` so a future
  renumber is caught. Pick the next free `10000+` slot and record it
  there.
- Export the type literal (`Template.Kind`, say) so `x is Template.Kind`
  discriminates a compiled handle from other values, exactly as
  `IO.StreamKind` does for stream handles.

The handle is **opaque**: it carries the `*template.Template` for `exec`
to use and is not introspectable as data. Everything else (`render`
in/out, `data`) is plain String / Map and needs no external type.

## 6. Errors

No panics (`core/go/CLAUDE.md` "Panic Prevention"; guard with
`AsConcreteString` / `RequireConcreteMap` before use). Go `error` returns
unwrap to a `BoruError` via `r.BoruError(code, detail, word)` with
kebab-case codes:

| code | raised when |
|---|---|
| `parse` | `Parse` rejects the template text (bad `{{...}}` syntax, unclosed action, unknown function). Raised by `render` and `compile`. |
| `exec` | `Execute` fails (missing field with strict option, type error in a pipeline, a `range` over a non-iterable). Raised by `render` and `exec`. |
| `bad-arg` | non-String template, non-Map data, or `exec` handed a value that is not a `Template` handle. |

Splitting `parse` from `exec` lets callers tell "the template is
malformed" from "the data did not fit the template".

## 7. Policy / capabilities

**None — pure.** Parsing and executing a template are in-memory string
operations writing to an internal buffer; nothing touches disk, network,
env, or clock. Runs under any policy. (Loading template *files* would be
I/O — out of scope, see §10; read the file with `IO.read` and pass the
String.)

## 8. Overlap

- **`html/template`** (contextual auto-escaping for safe HTML output) is
  **out of scope** here — it is a different package with a security
  contract of its own; see the cross-referenced `boru:html`
  ([HTML.10.md](HTML.10.md)). `boru:template` is the *text* engine with no
  escaping.
- **boru's own template strings** (the backtick `` `...${expr}...` ``
  interpolation built into the parser — see `lang/go/CLAUDE.md` "Template
  string interpolation") cover *simple value interpolation* and are the
  right tool for that. `boru:template` is for **Go-template-syntax logic**:
  `{{range}}` loops, `{{if}}`/`{{else}}` conditionals, pipelines, and
  named sub-templates — control flow that backtick interpolation does not
  express. The dividing line: reach for backtick strings to splice a few
  values, reach for `Template` when the output shape depends on iterating
  or branching over the data.

## 9. Examples (args-before form)

All args-before form (`template data Template.render` /
`template Template.render data`); never pure forward.

```
import "boru:template"

# one-shot render (recommended)
"Hello {{.name}}!" {name:"Ada"} Template.render
# → "Hello Ada!"

# conditionals / ranges — the reason this module exists
"{{range .xs}}[{{.}}]{{end}}" {xs:[1 2 3]} Template.render
# → "[1][2][3]"

# infix form also dispatches
{name:"Ada"} Template.render "Hi {{.name}}"        # → "Hi Ada"

# reusable: compile once, exec per row (hot loop)
def t ("{{.n}}: {{.v}}" Template.compile)
t Template.exec {n:"a" v:1}                          # → "a: 1"
t Template.exec {n:"b" v:2}                          # → "b: 2"

# errors
"{{.name" {name:"x"} Template.render               # ERROR:parse  (unclosed action)
"{{.missing.deep}}" {} Template.render             # ERROR:exec   (nil field walk)
```

## 10. Open questions / out of scope

- **Custom funcs (`Funcs(FuncMap)`)** — letting boru register helper
  functions callable from inside a template is powerful but needs a bridge
  from boru `Function` values into a Go `FuncMap`. Deferred; open question
  whether the common cases (a few string/format helpers) justify it or
  whether boru pre-processing of the data Map is enough.
- **Named / associated templates and `{{template "x"}}`** — multi-template
  sets (`ParseFiles`, `{{define}}`/`{{template}}`) are out of scope for
  the first cut; `render` / `compile` handle a single template body.
- **Loading from files** — out of scope (would be I/O, breaking the
  "pure" property); read via `IO.read` and pass the String.
- **`Option("missingkey=error")` and other execution options** — start
  with the default (missing map keys render as `<no value>`); whether to
  expose a strict-missing-key option is open. If added, it would be an
  options Map arg, not a separate word.
- **`html/template` as a sibling module** — explicitly punted to
  [HTML.10.md](HTML.10.md) / a future `boru:html-template`; not this note.

## 11. Implementation sketch

Wiring checklist — no Go code here. Reference: `lang/go/modules/io.go`
(`BuildIOModule` — the module-owned external-type pattern via
`MintStreamKind` / `NewTypeLiteral`), with `math.go` for the plain pure
words.

- `lang/go/modules/template.go` — `BuildTemplateModule(parent *native.Registry)
  (native.ModuleDesc, error)`: isolated `native.DefaultRegistry()`
  sub-registry; register the `Template` external type via
  `RegisterExternalBuiltin` with a reserved `FixedID` in the `10000+`
  range; register a `TemplateNatives []native.NativeFunc` slice (each
  inner sig `BarrierPos: -1`); wrap each word as an `FnDef` export into an
  `*OrderedMap`; also export the type literal (`exports.Set("Kind",
  native.NewTypeLiteral(templateKind))`, mirroring io.go's
  `StreamKind` export); return `ModuleDesc{ID: parent.Modules.NextID(),
  Exports: {"Template": …}}`.
- `render` / `exec` execute to an internal `bytes.Buffer` and return its
  `String()`; `compile` wraps the parsed `*template.Template` in an
  `ExtensionPayload` of the registered external type; `exec` guards that
  its handle arg is that type (else `bad-arg`).
- Register `"template": BuildTemplateModule` in the `modules` map in
  `lang/go/modules/modules.go`.
- **FixedID stability:** add the allocated `10000+` ID to
  `lang/go/test/fixedid_stability_test.go` (`core/go/CLAUDE.md` "FixedID
  Allocation").
- `lang/go/modules/docs_template.go` — `registerDocs("boru:template",
  map[string]string{…})` with a one-liner per export (else
  `TestModuleExportDocs` fails); document `Kind` too.
- `lang/spec/module-template.tsv` — `input⇥expected⇥description` rows
  leading with `import "boru:template"`; cover `render`, the `range`/`if`
  cases, `compile`+`exec`, and pair every positive row with an
  `ERROR:<substring>` sibling (`parse` and `exec` both) per Test
  discipline (`lang/go/CLAUDE.md`).
- Boundary conversion via `core.FromNative` / `core.ToNative`
  (String↔`string`, Map↔`map[string]any`, List↔slice).
