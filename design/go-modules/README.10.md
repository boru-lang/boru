# Curated Go-stdlib boru modules

> **Status: design proposal.** Nothing in this folder is implemented yet.
> Each `<PKG>.10.md` note specifies a *curated, hand-written* native boru
> module that wraps one Go standard-library package with an **idiomatic
> ("boru-ish") API** — not a mechanical 1:1 mirror of the Go signatures.
> The docs exist so the proposed surface is auditable before any Go code
> is written.

This folder is the index + shared conventions for the family. Read this
file first, then the per-package note for the package you care about.

## Relationship to the reflection bridge

[`../GO-MODULES.10.md`](../GO-MODULES.10.md) proposes a *generic reflection
bridge* (`import "go:net/url"`) that auto-exposes any host-registered Go
package with raw, `Any`-typed, machine-derived signatures. This folder is
the **complementary** path:

| | Reflection bridge (`go:`) | Curated modules (`boru:`) — this folder |
|---|---|---|
| Surface | every registered Go symbol, mechanically | a chosen, renamed, reshaped subset |
| Signatures | `reflect.Type`, `Any` boundaries | real lattice types, refined arg order |
| Idioms | Go names, Go shapes | kebab words, value-or-error, options maps |
| Cost | runtime `reflect`, `Any` everywhere | hand-written `NativeFunc` per word |
| Audience | host wires arbitrary packages ad hoc | first-class, documented, tested stdlib words |

The two can coexist: the bridge is the escape hatch; a curated module is
the promotion of a package that has earned a polished surface. Both reuse
the same module machinery ([`../NATIVE-MODULES.10.md`](../NATIVE-MODULES.10.md)),
the same `core.FromNative`/`core.ToNative` bridge, and the same opaque-handle
mechanism (below).

## Scope of this family — *new packages only*

These notes cover Go packages that have **no boru-user-facing surface
today**. The packages already exposed by an existing module are out of
scope and are NOT to be re-specified or disturbed:

- `math`, `math/big` → `boru:math-util` (`MathUtil`)
- `strings`, `regexp`, `unicode` → `boru:string-util` (`StringUtil`) — **now expanded** with Unicode classification, see below
- `time` (+ clock/async) → `boru:time-util` (`TimeUtil`)
- `math/bits`, `hash/fnv` → `boru:bin-util` (`BinUtil`) — **now expanded**, see below
- `math/rand` → `boru:rand` (`Rand`)
- `net/http` → `boru:net` (`Net`)
- `io`, file ops → `boru:io` (`IO`)
- `sort` → core `sort` + `boru:array-util` (`grade`/`sortby`)

> **Consolidation (deviates from strict 1:1).** All *binary-adjacent*
> functionality — cryptographic hashes, HMAC, secure random, CRC, hex /
> base32 / base64 / base128 / ascii85 encoding, and GUIDs — is
> **concentrated into the existing `boru:bin-util` module** rather than
> split into one module per Go package. It builds on a new first-class
> [`Bytes`](BYTES.10.md) type. See [BIN-UTIL.10.md](BIN-UTIL.10.md) and
> [BYTES.10.md](BYTES.10.md). This superseded and removed the earlier
> standalone CRYPTO-*, HASH-CRC*, and ENCODING-BASE64/HEX notes.

> **Consolidation (deviates from strict 1:1).** All *Unicode*
> functionality — per-string rune classification (`is-alpha`,
> `is-digit`, `is-space`, …) and case mapping — is **folded into the
> existing `boru:string-util` module** rather than split into a separate
> `boru:unicode` module. See [STRING-UTIL.10.md](STRING-UTIL.10.md). This
> superseded and removed the earlier standalone UNICODE note.

Where a new module's domain *touches* one of these (e.g. `boru:csv` vs the
`parselang`/`emitlang` CSV words), the per-package note calls out the
overlap and the dividing line in its **Overlap** section. It does not
move or change the existing words.

## The roster

One boru module per Go package. Namespaces are the plain capitalized
package name; the `-util` id + `*Util` namespace is used **only** when the
bare namespace would collide with a builtin type (`Path`, `String`,
`Time`, `Type`, `Array`, `Matrix`, …) or an existing module namespace
(`Rand`, `Net`, `IO`). See the naming rule in `lang/go/CLAUDE.md`
("Package layout").

| Go package | doc | import id | namespace | policy gate |
|---|---|---|---|---|
| `strconv` | [STRCONV](STRCONV.10.md) | `boru:strconv` | `Strconv` | none |
| `fmt` | [FMT](FMT.10.md) | `boru:fmt` | `Fmt` | none |
| `net/url` | [NET-URL](NET-URL.10.md) | `boru:url` | `Url` | none |
| `path` | [PATH](PATH.10.md) | `boru:path-util` | `PathUtil` | none |
| `path/filepath` | [PATH-FILEPATH](PATH-FILEPATH.10.md) | `boru:filepath` | `FilePath` | none (pure string ops) |
| `os` | [OS](OS.10.md) | `boru:os` | `Os` | `env`, `process`, `system-info` |
| `os/exec` | [EXEC](EXEC.10.md) | `boru:exec` | `Exec` | `process` |
| `runtime` | [RUNTIME](RUNTIME.10.md) | `boru:runtime` | `Runtime` | `system-info` |
| `encoding/csv` | [ENCODING-CSV](ENCODING-CSV.10.md) | `boru:csv` | `Csv` | none |
| `text/template` | [TEXT-TEMPLATE](TEXT-TEMPLATE.10.md) | `boru:template` | `Template` | none |
| `html` | [HTML](HTML.10.md) | `boru:html` | `Html` | none |
| `net/mail` | [NET-MAIL](NET-MAIL.10.md) | `boru:mail` | `Mail` | none |
| `math/cmplx` | [MATH-CMPLX](MATH-CMPLX.10.md) | `boru:cmplx` | `Cmplx` | none |

`Mail` and `Cmplx` are flagged niche.

### Binary-adjacent (consolidated into `boru:bin-util`)

Not 1:1 modules — these fold into the one expanded module plus a new type:

| Area | doc | home | policy gate |
|---|---|---|---|
| crypto hashes, HMAC, secure random, CRC, hex/base32/base64/base128/ascii85, GUIDs | [BIN-UTIL](BIN-UTIL.10.md) | `boru:bin-util` (`BinUtil`) | none, except `random`/`uuid` (entropy) |
| first-class binary leaf type | [BYTES](BYTES.10.md) | `Bytes` type (global builtin) | none |

Covers the Go packages `crypto/{sha256,sha512,sha1,md5,hmac,rand}`,
`hash/{crc32,crc64}`, `encoding/{hex,base32,base64,ascii85}`, a custom
base128 codec, and `github.com/google/uuid`.

### Text / Unicode (consolidated into `boru:string-util`)

Not a 1:1 module — Go `unicode` per-rune classification + case folds into
the existing whole-string utility module:

| Area | doc | home | policy gate |
|---|---|---|---|
| Unicode classification (`is-alpha`/`is-digit`/`is-space`/…) + case mapping | [STRING-UTIL](STRING-UTIL.10.md) | `boru:string-util` (`StringUtil`) | none |

### Filesystem (all in `boru:io`)

`boru:io` owns **all** filesystem functionality — content I/O, tree
mutation, directory listing, `stat`, the read-only existence/type
predicates (`exists`/`is-file`/`is-dir`/`is-symlink`), and the
location getters (`cwd`/`home-dir`/`temp-dir`). `boru:os`
([OS](OS.10.md)) keeps only the **non-filesystem** remainder of the Go
`os` module (env, args, identity, exit). Both share the one `FileOps`
capability + `fileops` policy scope.

| Area | doc | home | policy gate |
|---|---|---|---|
| filesystem content / tree / listing / stat / existence predicates | [IO](IO.10.md) | `boru:io` (`IO`) | `fileops` (`disk.read`/`disk.write`) |

## Shared conventions (every note assumes these)

These are the kernel/module rules the per-package notes do **not**
re-derive. They are cited to the source of truth; read those before
implementing.

### Module construction

A module is a `BuildXxxModule(parent *native.Registry) (native.ModuleDesc, error)`
that (1) creates an isolated sub-registry (`native.DefaultRegistry()`),
(2) registers its inner `[]native.NativeFunc` there, (3) wraps each as an
`FnDef` export into an `*OrderedMap` keyed by word name, (4) returns a
`ModuleDesc{ID: parent.Modules.NextID(), Exports: {"<Namespace>": …}}`.
`lang/go/modules/math.go` (`BuildMathModule`) and `io.go`
(`BuildIOModule`, the capability-backed variant) are the canonical
references. Register the builder in the `modules` map in
`lang/go/modules/modules.go`.

### Argument order & dispatch (CRITICAL)

- Inner native sigs MUST use `BarrierPos: -1` — all-forward, so every
  position is forward-eligible and any position the caller does not fill
  forward falls back to the value stack. That fallback is what lets the
  infix form `a Ns.word b` dispatch; a `BarrierPos: 0` inner sig demands
  every arg from the stack and refuses it. This is the sharp edge in
  `lang/go/CLAUDE.md` "Module FnDef Wrappers — inner sig BarrierPos",
  pinned by `wrapper_dispatch_test.go`. For ZERO-ARG constants the two
  spellings are byte-identical after registration (`-1` normalizes to
  `TotalArgs()`, which is 0 at zero args — NUR023): `-1` is the ADR-004
  uniform default, `0` an accepted constant-style idiom; existing
  registrations use both and neither is wrong.
- `FnSig.Params` and `NativeSig.Args` are **top-first, sig order**:
  position 0 is the top of the stack. Document signatures in this order.
- Every layout dispatches: `a b Ns.word` (stack form), `a Ns.word b`
  (infix form) and pure forward `Ns.word a b` alike — module words
  forward-collect like built-ins since `6687638`, pinned by
  `lang/spec/edge-modules-1.tsv`. Every example in these notes is
  written **args-before-dot** by convention, matching
  `../NATIVE-MODULES.10.md` "Calling Convention"; that is a house
  choice for these notes, not a dispatch constraint.

### Type mapping & the value bridge

- Convert at the boundary with `core.FromNative` / `core.ToNative`
  (`core/go/gobridge.go`): String↔`string`, Integer↔int kinds,
  Float↔float kinds, Boolean↔`bool`, List↔slice, Map↔`map[string]any`,
  None↔`nil`.
- A Go value with no boru counterpart (`*url.URL`, a parsed
  `*template.Template`, a `*csv.Reader`) is held in an
  `ExtensionPayload` and surfaced as a **registered external type**
  via `RegisterExternalBuiltin` with a `FixedID` from the documented
  `10000+` host/third-party range (`core/go/CLAUDE.md` "FixedID
  Allocation"; add it to `lang/go/test/fixedid_stability_test.go`). The
  module-owned `IO.StreamKind` (minted per import, `io.go`) is the
  in-tree reference for an opaque, module-scoped handle type. A note
  introduces such a type only when a value genuinely cannot be a plain
  Map/String — most of these packages need none.

### Errors (no panics)

- Never panic (`core/go/CLAUDE.md` "Panic Prevention"). Guard args with
  `AsConcreteString`/`RequireConcreteList`/etc. before use.
- Signal failure with `r.BoruError(code, detail, word)` using a
  kebab-case `code`. A Go `error` return is unwrapped to a `BoruError`;
  a Go `(value, ok)` pair collapses to either value-or-`None` or
  value-or-error, whichever reads better (the note states which).

### Policy & capabilities

- Pure packages (most of this roster) need **no gating** and run under
  any policy.
- Side-effecting packages gate through `lang/go/policy`: the scope/op
  is checked via `HostPolicy(reg)`. Recognized scopes
  (`policy.KnownScopes`): `fileops`, `network`, `sqlite`, `formats`,
  `env`, `process`, `clock`; coarse caps (`policy.GlobalOps`):
  `disk.read`, `disk.write`, `network`, `process`, `env`, `clock`,
  `system-info`, `mutate`. So `boru:os` env words gate on `env`, process
  words on `process`, and `boru:runtime`/host-info on the `system-info`
  global cap. Host-backed effects (entropy for `crypto/rand`, any file
  read) go through a capability seam like `FileOps`, not direct OS calls
  — see `io.go`.

### Docs, spec, and naming plumbing

- Every export gets a one-line summary in a `docs_<id>.go` file via
  `registerDocs("boru:<id>", map[string]string{…})` (`docs.go`);
  `TestModuleExportDocs` fails if any export lacks one.
- Behaviour is pinned by a `lang/spec/module-<id>.tsv` suite
  (`input⇥expected⇥description`, rows lead with `import "boru:<id>"`).
  Per **Test discipline** (`lang/go/CLAUDE.md`) every positive row needs
  a negative sibling (`ERROR:<substring>`).
- Exported names must be capitalized. Words are kebab-case.
- `describe` surfaces the module live via provenance stamped in
  `stampExportProvenance` (`modules.go`) — no static help wiring needed.

## Per-package note template

Each `<PKG>.10.md` follows this structure:

1. **Package & status** — the Go package, one-line purpose, "design
   proposal, not implemented".
2. **Why curated** — one or two lines: what the refined surface buys
   over the raw `go:` bridge for this package.
3. **Import & namespace** — `import "boru:<id>"` → `<Namespace>`; the
   clash rationale if `-util` is used.
4. **API** — the core table: Go symbol → boru word (kebab) → signature
   (top-first) → one-line doc → **boru-ish refinement** (what changed and
   why: `(v, ok)`→value-or-None/error, variadic→List, flags→options Map,
   dropped/renamed/merged words, type overloads).
5. **Types** — kernel types used; any opaque external handle (rare) with
   its FixedID plan; otherwise "scalars/List/Map only".
6. **Errors** — the kebab error codes and how Go errors map.
7. **Policy / capabilities** — the gate (or "none — pure").
8. **Overlap** — the dividing line vs any existing module that touches
   the domain (or "none").
9. **Examples** — 3–5 args-before snippets.
10. **Open questions / out of scope**.
11. **Implementation sketch** — builder + `docs_<id>.go` + `modules` map
    entry + `lang/spec/module-<id>.tsv`, pointing at `math.go` (pure) or
    `io.go` (capability-backed) as the reference. No code, just the
    wiring checklist.
