# Go Modules (reflection bridge)

> **Status: design proposal.** Nothing in this note is implemented yet.
> It describes how boru *could* expose arbitrary Go packages to boru source
> via a reflection bridge, surfaced through the existing `import` word with
> a `go:` prefix. It is written to be implementable as a later task; the
> "Implementation" and "Open questions" sections mark the work and the
> undecided edges.

## Motivation

boru is **sealed at compile time**. Go reaches the language only through
host-wired native words (`RegisterNativeFunc`) and native modules
(`BuildXxxModule` → `ModuleDesc`, imported as `import "boru:math-util"`,
see [NATIVE-MODULES.10.md](NATIVE-MODULES.10.md)). Every importable name is
gated through a *static map* (`lang/go/modules/modules.go`'s `modules`).
There is no FFI, no runtime `reflect`, no dynamic loading.

The goal here is to let a host expose **any** Go package's functions and
constants to boru *without hand-writing a `NativeFunc` per function*.

Go itself cannot load arbitrary packages at runtime (only the limited
`plugin` package can, with severe constraints — see "Alternatives"). So
"arbitrary" here means **any package the host chooses to wire in** — the
host enumerates the Go symbols once at construction time, and the bridge
exposes them to boru *automatically via reflection* rather than per-function
boilerplate. The host registration list is the trust boundary.

## Surface

A `go:` import sits alongside `boru:`:

```
import "go:net/url"
"https://example.com/a?b=1" Url.parse    # reflected call into url.Parse
```

The namespace is derived from the last path segment of the import name,
title-cased — mirroring the capitalised-namespace convention native modules
use (`MathUtil`, `IO`, see `lang/go/CLAUDE.md` "Module / ModuleExport
instances"):

| Import            | Namespace |
|-------------------|-----------|
| `"go:strings"`    | `Strings` |
| `"go:net/url"`    | `Url`     |
| `"go:math/rand"`  | `Rand`    |

Beware the builtin type-name clashes the `-util` suffix was introduced to
avoid (`String`, `Time`, `Type`, `Array`, `Matrix`). When a derived
namespace would clash, the import-renaming form already supported by the
`import` word (see [IMPORTS.10.md](IMPORTS.10.md) §2) overrides it:

```
import [Strings Str] "go:strings"
"hello" "he" Str.has-prefix              # → true
```

Go identifiers are exported in `CamelCase`; the bridge lower-kebab-cases
the export name (`HasPrefix` → `has-prefix`, `ParseFloat` → `parse-float`)
to read like the rest of boru. The mapping is mechanical and documented per
package via `boru describe` (below).

## Calling convention

Reflected words behave exactly like native-module words (see
[NATIVE-MODULES.10.md](NATIVE-MODULES.10.md) "Calling Convention"): a
`Namespace.word` dot-access is an auto-invoking `FnDef` wrapper that fires
as soon as it finds matching arguments, on the stack or forward. Every
layout dispatches — stack form (`a b Pkg.word`), infix form
(`a Pkg.word b`) and forward form (`Pkg.word a b`) — and these notes
write their examples args-before-dot by convention:

```
import "go:strings"
"hello world" " " Strings.split          # → ["hello" "world"]   (stack form)
"  hi  " Strings.trim-space              # → "hi"
```

> **Note on argument form.** boru's general guidance is to prefer *forward*
> form `f a b c` for words that are not read as operators (see
> `core/go/CLAUDE.md` "Signature Ordering"), and module words are no
> exception: since the structure-first lazy forward-argument resolution
> (`6687638`) they forward-collect like built-ins, so `Pkg.word a b`
> dispatches alongside `a b Pkg.word` and `a Pkg.word b`. Pinned for
> `boru:` modules by `lang/spec/edge-modules-1.tsv` (`MathUtil.min 3 7`
> → `3`, "forward form agrees"; `MathUtil.hypot 3.0 4.0` → `5.0`).
> `go:` words inherit this from the shared module machinery. The
> args-before spelling stays the convention these notes write their
> examples in, because it is the form the wrapper was designed around —
> not because the forward one fails.

A Go function's parameters map to sig positions **in declared order**
(top-first, sig order — the one kernel convention). For a Go
`func F(a A, b B) R`, the stack-form call is `a b Pkg.f` (`a` is sig[0], the
stack top). Inner sigs must use `BarrierPos: -1` — all-forward, so a
position the caller does not fill forward falls back to the stack, which
is what lets the infix form dispatch too (see the caveat under
"Resolution path").

## Host registration (the "arbitrary" surface)

The host enumerates the symbols it wants to expose. Proposed public entry
point on the `lang` package (alongside the existing `lang.New`, `(*Boru).Run`,
`(*Boru).Check`, `(*Boru).Register` in `lang/go/boru.go`):

```go
a, _ := lang.New()

// Each value is a Go func or constant obtained by the host however it
// likes — std lib, third-party, its own packages.
a.RegisterGoPackage("net/url", map[string]any{
    "Parse":      url.Parse,
    "QueryEscape": url.QueryEscape,
})
a.RegisterGoPackage("strings", map[string]any{
    "Split":     strings.Split,
    "TrimSpace": strings.TrimSpace,
    "HasPrefix": strings.HasPrefix,
})

a.Run(`import "go:strings"   "a,b,c" "," Strings.split`)
```

This is deliberate: Go has no portable way to discover or load a package by
name at runtime, and even reflection cannot enumerate a package's exported
identifiers. The host map *is* the package surface. It doubles as the trust
boundary — only registered symbols are reachable from boru.

## Type mapping

Values cross the boundary through an extension of the existing
`core.FromNative` / `core.ToNative` helpers (`core/go/gobridge.go`), which
today cover only scalars, lists, and maps with a *fixed* mapping. The bridge
needs **param-directed** conversion: the target Go type comes from the
function's `reflect.Type`, so a boru Integer becomes whatever width the
parameter declares.

| boru                     | Go (in)                         | Go (out) → boru          |
|-------------------------|---------------------------------|-------------------------|
| `String`                | `string`                        | `string` → `String`     |
| `Integer`               | `int`, `int8..64`, `uint8..64`  | integer kinds → `Integer` |
| `Float`                 | `float32`, `float64`            | float kinds → `Float`/`Integer`¹ |
| `Boolean`               | `bool`                          | `bool` → `Boolean`      |
| `List`                  | `[]T` / `[]any` (elementwise)   | slice/array → `List`    |
| `Map`                   | `map[string]any`; or a `struct` (fields ↔ keys via reflection) | map / struct → `Map` |
| `None`                  | `nil` (pointer/interface/slice/map) | `nil` → `None`      |
| `Go/Object` (see below) | the wrapped Go value, unchanged | opaque value → `Go/Object` |

¹ `floatToValue` already demotes integral floats to `Integer` for compact
output; reused unchanged.

### Opaque values: the `Go/Object` type

Go values that have no boru counterpart (structs the host doesn't want
flattened, pointers, handles like `*os.File`, `*url.URL`) are wrapped in an
`ExtensionPayload{Body: any}` — the kernel's explicit escape hatch
(`core/go/payload.go`, `core.NewExtension`; see `core/go/CLAUDE.md` "Sealed
Payload"). They surface under a single registered external type, `Go/Object`,
and **round-trip unchanged** back into later Go calls (so
`url.Parse` → `*url.URL` → `(*url.URL).String` composes).

`Go/Object` is registered once via `RegisterExternalBuiltin` with a
`FixedID` from the documented `10000+` host/third-party range (see
`core/go/CLAUDE.md` "FixedID Allocation"). Its `TypeBehavior.Format`
delegates to `fmt.Sprint` on the body; `Match`/`Equal` compare identity.
Bodies that want to participate in deep cloning implement the optional
`DeepCloner` capability (`core/go/clone.go`).

### Errors

A trailing `error` return is the universal Go convention. The bridge
**unwraps it**: on non-nil `error` it raises a boru error (`ErrorInfo`,
constructed via `r.BoruError`) instead of pushing the error as a value. So
`func F(...) (T, error)` exposes a single `T` result to boru and faults on
failure, matching how native words signal errors.

## Resolution path (reuse existing module machinery)

The `import` word already dispatches on the reference shape in
`lang/go/native/native_module_module.go`: `isNativeModImport` (`boru:`),
`isDataFile`, `isFilePath`, bare module. The `go:` form adds one parallel
branch.

| Concern            | Native modules (`boru:`)                          | Go modules (`go:`) — proposed |
|--------------------|--------------------------------------------------|-------------------------------|
| Prefix test        | `isNativeModImport` (`native_module_module.go`)  | `isGoModImport`               |
| Resolve            | `resolveNativeMod` → `r.Modules.Resolver`        | `resolveGoMod` → a Go-package resolver |
| Builder            | `BuildXxxModule` (static `Natives` slice)        | reflection-generated exports  |
| Descriptor         | `ModuleDesc` with `Exports` (`core/go/value.go`)  | same `ModuleDesc` shape       |
| Install            | `installExports` / `MarkLoaded`                  | identical                     |
| Introspect-only    | `ResolveAnyModule` (for `describe`)              | extend with the `go:` branch  |

The reflected builder mirrors `BuildMathModule` (`lang/go/modules/math.go`):
create an isolated sub-registry, register one generated word per Go symbol,
wrap each as an `FnDef` carrying the sub-registry, and package them into a
`ModuleDesc`. The only difference from a native module is that the words are
generated from `reflect.Type` instead of a hand-written `[]NativeFunc`.

> **Sub-registry wrapper caveat:** module FnDef wrappers must register their
> inner native with `BarrierPos: -1` so the infix form `a Pkg.f b` dispatches.
> This is a known sharp edge documented in `lang/go/CLAUDE.md` "Module FnDef
> Wrappers — inner sig BarrierPos (CRITICAL)" and pinned by
> `wrapper_dispatch_test.go`. The reflected generator must honour it.

### The reflection bridge core (the new piece)

For each registered symbol that is a func, generate a kernel `Handler`
(`func(args, ctx, stack, r) ([]Value, error)`, `core/go/signature.go`) closed
over the func's `reflect.Value`:

1. Inspect `reflect.Type` `In`/`Out`, including `IsVariadic`.
2. For each parameter, convert `args[i]` → a Go value of the declared param
   type (param-directed `FromNative`); fault with a clear `BoruError` on an
   inconvertible shape.
3. `fn.Call(in)`.
4. Split results: a trailing `error` is unwrapped (raise on non-nil); the
   remaining results convert back via `ToNative`/`Go/Object` wrapping.

Constants (non-func symbols) are exposed as 0-arg words that push the
converted value, matching how `math.pi` / `math.e` work today.

## Static checking & `describe`

Reflected words cannot be expressed precisely in boru's type lattice
(`int`, `*url.URL`, etc. have no lattice node). They are declared with
`TAny` arg and return types (mapping the obvious scalar kinds to their boru
type where 1:1 is safe). Consequently `boru check` treats them as gradual
boundaries — values flowing through a `go:` word carry `Any` forward. This
is the same conservative stance anonymous lambdas take (`Returns=[Any]`,
see `lang/go/CLAUDE.md` "Lambda Syntax").

`describe` integrates through the **live registry path**
(`lang/go/native/describe.go` — `BuildQualifiedFuncInfo` / dynamic help),
not static help entries. The signature line is synthesized from the func's
`reflect.Type` (param/return Go types shown for orientation) plus a generated
one-line summary noting the backing Go symbol:

```
describe "go:strings"            # namespace + reflected exports
describe Strings.split           # synthesized sig + "→ strings.Split"
```

## Security / policy

Two gates, in order:

1. **Host registration** is primary and absolute: a package is reachable
   only if the host called `RegisterGoPackage` for it. boru source cannot
   name an unregistered package, and reflection cannot reach a symbol the
   host did not hand over.
2. **Policy** gates the `go:` import op the same way native-module imports
   are gated today (`lang/go/policy/`, `modules.Resolve` consults
   `HostPolicy`): a policy can disable `go:` imports wholesale or per
   package. Because a `go:` call executes arbitrary host Go, deployments
   that sandbox should keep it off by default.

Threat model, stated plainly: there is **no dynamic code loading**. The
exposed surface is exactly the static set the host compiled in and
registered; behaviour is deterministic across runs.

## Alternatives considered

| Approach            | Why not |
|---------------------|---------|
| Go `plugin` (.so)   | The only mechanism that loads truly *new* code at runtime, but Linux/macOS-only, requires an identical toolchain and dependency graph, cannot be unloaded, and breaks boru's sealed/deterministic model. |
| Subprocess / RPC    | Fully isolated and language-agnostic, but heavyweight, serialization-bound, and an operational burden (process lifecycle, transport). Overkill for in-process Go interop. |
| Build-time codegen  | Type-safe with no runtime `reflect`, but every package must be chosen and generated *before* the binary is built — no host-time flexibility, more build machinery. |
| **Reflection bridge** (chosen) | Host wires in any package at construction time; words are generated automatically; stays in-process, deterministic, and sealed. Cost is runtime `reflect` and `Any`-typed boundaries. |

## Open questions

- **Variadics.** Map a boru `List` tail to the variadic slot, or accept
  trailing forward args? (Lean: a single `List` argument.)
- **Struct methods / pointer receivers.** Expose methods on a `Go/Object`
  (e.g. `urlObj Url.string` → `(*url.URL).String`)? Needs a method-dispatch
  convention distinct from package-function dispatch.
- **Multi-return arity.** Go funcs returning `(A, B)` (no error) — push both
  onto the stack in order, or require the host to wrap? Stack semantics make
  multi-push natural but surprising for readers.
- **`(T, error)` vs `(T, U)` disambiguation.** The "trailing error" rule is
  unambiguous only because `error` is an interface; confirm the bridge keys
  on the concrete `error` type, not arity.
- **Funcs as arguments / closures.** Marshaling a boru `Function` into a Go
  `func(...)` param is possible via reflection but needs a defined calling
  convention; likely out of scope for v1 (reject with a clear error).
- **Generics & channels.** `reflect` cannot instantiate generic functions;
  channels and `context.Context` have no boru representation. All rejected at
  registration time with a clear error rather than failing at call time.

## Implementation checklist (for the later task)

1. `core/go/gobridge.go` — add param-directed conversion (target
   `reflect.Type`), struct↔Map, and `Go/Object` wrapping/unwrapping.
2. Register the `Go/Object` external type (`RegisterExternalBuiltin`,
   FixedID `10000+`) with a format/identity `TypeBehavior`; add it to the
   FixedID stability snapshot (`lang/go/test/fixedid_stability_test.go`).
3. Reflection word generator: `reflect.Value` → kernel `Handler`
   (variadic, error-unwrap, multi-return per the resolved open questions).
4. `lang/go/modules/` — a `go:` resolver that builds a `ModuleDesc` from a
   host-registered symbol map (mirror `BuildMathModule`; inner sigs
   `BarrierPos: -1`).
5. `lang/go/native/native_module_module.go` — `isGoModImport` / `resolveGoMod`
   branches in `import` and `ResolveAnyModule`.
6. `lang/go/boru.go` — `(*Boru).RegisterGoPackage` (or an `Options` field) and
   wiring of the new resolver, parallel to `modules.InstallResolver`.
7. `lang/go/policy/` — a `go`/`go-modules` scope gating the import op.
8. `lang/go/native/describe.go` — synthesize help from `reflect.Type` for
   `go:` namespaces.
9. Tests: positive **and** negative (per `lang/go/CLAUDE.md` "Test discipline")
   — inconvertible args, unregistered package, policy-denied import,
   error-returning funcs, `Go/Object` round-trip; a `lang/spec/*.tsv` suite
   once the surface settles.

## Adding a package (end-user recipe, once implemented)

1. Host calls `a.RegisterGoPackage("<import path>", map[string]any{…})` with
   the funcs/constants to expose.
2. boru source runs `import "go:<import path>"`, optionally renaming the
   namespace (`import [Derived New] "go:…"`).
3. Call the words: `arg1 arg2 Namespace.word-name`.
4. `describe "go:<import path>"` lists what's available.
