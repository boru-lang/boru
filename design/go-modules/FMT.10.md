# `boru:fmt` — Go `fmt`

> **Status: design proposal, not implemented.** A curated, hand-written
> native module wrapping the *string-returning* part of Go's `fmt`. Read
> [README.10.md](README.10.md) first for the shared conventions this note
> assumes.

## 1. Package & status

Go [`fmt`](https://pkg.go.dev/fmt) implements C-style formatted I/O. This
note covers only the **string-producing** functions — `Sprintf`,
`Sprint`, `Sprintln`, and `Errorf` (as a format-then-error helper). The
`Print` / `Fprint` family that writes to `os.Stdout` / an `io.Writer` is
**out of scope**: side-effecting output belongs to `boru:io` (`IO.print`
/ `IO.printstr`, see `lang/go/native/io_module.go`). Nothing is
implemented yet.

## 2. Why curated

Go's `fmt.Sprintf(format string, a ...any) string` is variadic over the
empty interface. The raw `go:` bridge would expose that as an
`Any`-typed varargs blur. The curated surface fixes the shape: a format
String plus a **List** of boru values, with values crossing the boundary
via `core.ToNative` so `%v`/`%d`/`%s`/… see real Go scalars. printf-style
formatting is the power-user complement to boru's idiomatic template
strings (see Overlap).

## 3. Import & namespace

```
import "boru:fmt"            # binds the Fmt namespace
```

`fmt` does not clash with a builtin type or existing module namespace,
so no `-util` suffix (naming rule in `lang/go/CLAUDE.md` "Package
layout"). Words are dot-accessed: `Fmt.format`, `Fmt.sprint`,
`Fmt.sprintln`.

## 4. API

Signatures are **top-first, sig order** (position 0 = top of stack).
Inner native sigs use `BarrierPos: -1` so the infix form dispatches. Go's
variadic `...any` becomes a single **List** argument.

| Go symbol | boru word | signature (top-first) | one-line doc | boru-ish refinement |
|---|---|---|---|---|
| `Sprintf(format,a...) string` | `format` | `[List args, String format] -> String` | Format args against a printf format string. | Variadic `...any` → a `List`; each element bridged via `core.ToNative`. format is the top arg so infix reads `format Fmt.format args`. |
| `Sprint(a...) string` | `sprint` | `[List] -> String` | Concatenate args, spaces only between non-string operands. | Variadic → `List`; no format string. |
| `Sprintln(a...) string` | `sprintln` | `[List] -> String` | Concatenate args with spaces, append a newline. | Variadic → `List`; always spaces between operands, trailing `\n`. |
| `Errorf(format,a...) error` | (folded) | — | — | Not a separate word: an `error` value has no boru counterpart. Use `Fmt.format` to build the message, then raise it via the engine's normal error path (`r.BoruError`) at the call site. |

Common verbs (documented in `docs_fmt.go` / the spec header so users
have a reference without leaving boru):

| verb | meaning |
|---|---|
| `%v` | default format for the value |
| `%+v` | default format, struct field names (Map keys) |
| `%s` | string |
| `%d` | base-10 integer |
| `%f` | decimal float (`%.2f` for fixed precision) |
| `%q` | double-quoted string / single-quoted rune |
| `%t` | boolean |
| `%x` | hex (lowercase) |

boru values map to their Go counterparts through `core.ToNative` before
formatting: String→`string`, Integer→`int64`, Float→`float64`,
Boolean→`bool`, List→`[]any`, Map→`map[string]any`, None→`nil`. A Map
formats under `%v`/`%+v` like a Go map.

## 5. Types

Scalars + List + Map only. No opaque handle type, no
`RegisterExternalBuiltin` / FixedID. The whole boundary is
`core.FromNative` / `core.ToNative` (`core/go/gobridge.go`).

## 6. Errors

`fmt` does not return an `error` from the `Sprint*` family — bad verbs
produce in-band markers (`%!d(string=hi)`, `%!(EXTRA …)`) rather than
failing. The module therefore:

- returns the formatted String including any `%!` marker (mirrors Go;
  does not invent an error where Go produced none), and
- guards its own argument types up front: a non-String format or a
  non-List args raises `format` (via `r.BoruError`). Guard with
  `AsConcreteString` and `RequireConcreteList` before use; never panic
  (`core/go/CLAUDE.md` "Panic Prevention").

| code | raised when |
|---|---|
| `format` | format arg is not a concrete String, or args is not a concrete List. |

(Whether to additionally scan the result for `%!` and promote it to an
error is an open question — see §10.)

## 7. Policy / capabilities

None — pure string production, no I/O. (The output-writing `Print`
family that *would* gate lives in `boru:io`, not here.)

## 8. Overlap

boru already has two formatting facilities, and `Fmt` must not disturb
them — it sits alongside as the printf escape hatch:

- **Template strings** `` `...${x}...` `` (parser-level `InterpString`,
  see `lang/go/CLAUDE.md` "Template string interpolation") are the
  **idiomatic default** for interpolation. Reach for them first.
- The **core `format` word** (`lang/go/native/format.go`) renders boru
  values to canonical text.

**Dividing line:** `Fmt.format` exposes *Go printf semantics* — width,
precision, verb control (`%-10.3f`, `%x`, `%+v`) — for power users who
need that control. It is namespaced (`Fmt.format`) so it never shadows
the bare core `format` word. Template strings remain the recommended,
unqualified way to build strings; `Fmt.format` is opt-in via `import`.

## 9. Examples (args-before form)

```
import "boru:fmt"

[42] "n = %d" Fmt.format                    # "n = 42"
[3.14159] "%.2f" Fmt.format                 # "3.14"
["world"] "hello %s" Fmt.format             # "hello world"
[1 2 3] Fmt.sprint                          # "1 2 3"
["a" "b"] Fmt.sprintln                      # "a b\n"
[42] "%q" Fmt.format                        # "%!q(int64=42)"  (Go in-band marker, no error)
42 Fmt.format "n = %d"                       # ERROR:format    (format ok but args not a List)
```

## 10. Open questions / out of scope

- **`Print` / `Println` / `Fprintf`** — stdout/writer output is out of
  scope; that surface is `boru:io`.
- **`Errorf` / `%w` wrapping** — boru has no first-class `error` value, so
  `Errorf` is folded away (build the message with `Fmt.format`, raise via
  `r.BoruError`). Revisit if an error/diagnostic value type is added.
- **`%!`-marker promotion** — should a result containing a Go formatting
  marker (`%!d(...)`, `%!(EXTRA ...)`) be returned verbatim (current
  proposal, matches Go) or promoted to a `BoruError`? Leaning verbatim
  for transparency.
- **`Scan` / `Sscanf` family** — input parsing is out of scope; scalar
  parsing is `boru:strconv` (see [STRCONV.10.md](STRCONV.10.md)).

## 11. Implementation sketch

Wiring checklist — no Go code. Reference: `lang/go/modules/math.go`
(`BuildMathModule`, the pure-module canonical builder).

- `lang/go/modules/fmt.go` — `BuildFmtModule(parent *native.Registry)
  (native.ModuleDesc, error)`: isolated `native.DefaultRegistry()`
  sub-registry, register a `FmtNatives []native.NativeFunc` slice
  (inner sigs `BarrierPos: -1`), wrap each as an `FnDef` export into an
  `*OrderedMap`, return `ModuleDesc{ID: parent.Modules.NextID(),
  Exports: {"Fmt": …}}`.
- Register `"fmt": BuildFmtModule` in the `modules` map in
  `lang/go/modules/modules.go`.
- `lang/go/modules/docs_fmt.go` — `registerDocs("boru:fmt", …)` with a
  one-line summary per export (`TestModuleExportDocs` enforces it).
- `lang/spec/module-fmt.tsv` — `input⇥expected⇥description`, each row
  leading with `import "boru:fmt"`; each positive row paired with an
  `ERROR:<substring>` negative sibling.
- No FixedID entry, no policy wiring.
