# `path/filepath` → `boru:filepath`

> **Status: design proposal — not implemented.** A curated, hand-written
> native module wrapping Go's `path/filepath` (OS-aware paths). Read
> [`README.10.md`](README.10.md) first for the shared conventions this
> note assumes.

## 1. Package & status

Go package: `path/filepath` — manipulation of **OS file paths** with the
platform's separator (`/` on Unix, `\` on Windows) and volume names.
This note specifies `boru:filepath` (namespace `FilePath`). It mirrors
`boru:path-util` ([`PATH.10.md`](PATH.10.md)) word-for-word for the shared
ops but is platform-dependent, and adds the filepath-only words `rel`,
`to-slash`, `from-slash`, `volume-name`, and `abs`. Design proposal; no
Go code exists yet.

## 2. Why curated

Same motivation as `boru:path-util`: kebab names, `Split`'s `(dir, file)`
→ a `Map`, `Join`'s variadic → a `List`, `Match`'s `(matched, error)` →
a Boolean. The added value here is being explicit about the **one word
with an environmental dependency** (`abs`, which resolves against the
current working directory) and keeping everything else a pure string
transform — see Policy.

## 3. Import & namespace

```
import "boru:filepath"        # binds the FilePath namespace
```

`FilePath` is not a builtin type and not an existing module namespace, so
the **bare namespace is used (no `-util` suffix)**. Contrast its sibling
`boru:path-util`, which *must* take `-util` because its bare namespace
`Path` collides with the builtin `Path` type. Words: `FilePath.join`,
`FilePath.rel`, …

## 4. API

Signatures are **top-first, sig order** (position 0 = top of stack). All
inner natives use `BarrierPos: -1` so the infix form `a FilePath.word b`
dispatches.

| Go symbol | boru word | signature (top-first) | one-line doc | boru-ish refinement |
|---|---|---|---|---|
| `filepath.Join` | `join` | `[List] -> [String]` | Join elements with the OS separator and clean. | Variadic `...string` → a single `List[String]`. |
| `filepath.Split` | `split` | `[String] -> [Map]` | Split a path into directory and final element. | `(dir, file)` → `Map {dir, file}`. |
| `filepath.Base` | `base` | `[String] -> [String]` | Last element of the path. | Pure `String → String`. |
| `filepath.Dir` | `dir` | `[String] -> [String]` | All but the last element, cleaned. | Pure `String → String`. |
| `filepath.Ext` | `ext` | `[String] -> [String]` | File-name extension including the leading dot. | Pure `String → String`. |
| `filepath.Clean` | `clean` | `[String] -> [String]` | Shortest equivalent path for the OS. | Pure `String → String`. |
| `filepath.IsAbs` | `is-abs` | `[String] -> [Boolean]` | True if the path is absolute for the OS. | `String → Boolean`. |
| `filepath.Rel` | `rel` | `[String, String] -> [String]` | Relative path from a base to a target. | sig is `base, target` (base top-first). `(string, error)` → value-or-error (`rel`). |
| `filepath.ToSlash` | `to-slash` | `[String] -> [String]` | Replace OS separators with `/`. | Pure `String → String`; identity on Unix. |
| `filepath.FromSlash` | `from-slash` | `[String] -> [String]` | Replace `/` with the OS separator. | Pure `String → String`; identity on Unix. |
| `filepath.Match` | `match` | `[String, String] -> [Boolean]` | Match a name against an OS-aware pattern. | sig `pattern, name`. `(matched, error)` → Boolean, malformed pattern → `bad-pattern`. |
| `filepath.VolumeName` | `volume-name` | `[String] -> [String]` | Leading volume name (e.g. `C:` on Windows). | Pure `String → String`; empty on Unix. |
| `filepath.Abs` | `abs` | `[String] -> [String]` | Absolute representation of a path. | `(string, error)` → value-or-error (`abs`). **Environmental** — resolves against the current working directory; see Policy. |

## 5. Types

Scalars / List / Map only. No opaque external handle, no FixedID
allocation.

## 6. Errors

No panics — guard with `AsConcreteString` / `RequireConcreteList`. Failure
via `r.BoruError(code, detail, word)`:

- `rel` — Go `filepath.Rel` error (target not relative to base) → `rel`.
- `match` — `filepath.ErrBadPattern` → `bad-pattern`.
- `abs` — Go `filepath.Abs` error (cwd unavailable) → `abs`.
- Non-String args → `bad-arg`.

The pure words (`join`/`split`/`base`/`dir`/`ext`/`clean`/`is-abs`/
`to-slash`/`from-slash`/`volume-name`) never fail on well-typed String
input.

## 7. Policy / capabilities

**None for the pure string ops** — they run under any policy. The single
exception is **`abs`**: it calls `os.Getwd()` to resolve a relative path,
so it has an **environmental dependency**. It should gate on the
`system-info` / `process` boundary (the same global cap that
`boru:runtime` / `boru:os` process words use, per the README "Policy &
capabilities"), or — preferably — route the cwd through a **host cwd
capability seam** (the same way file reads route through `FileOps` rather
than direct OS calls in `io.go`), so the module stays pure and the host
controls the one impure call. Recommendation: keep the other twelve words
pure and isolate the cwd dependency behind that seam, so `abs` is the only
word a restrictive policy can disable.

## 8. Overlap

- `boru:path-util` (`PathUtil`, [`PATH.10.md`](PATH.10.md)) — the
  always-slash, always-pure sibling. Same word names for the shared ops;
  the dividing line is *separator and purity*: use `boru:path-util` for
  URLs and portable virtual paths, `boru:filepath` for real OS paths.
- `boru:io` (`IO`) — owns all **filesystem access** (`read`, `write`,
  `folder`, …) through the `FileOps` capability. So `filepath.Glob` and
  `filepath.Walk` are **OUT of scope for this module** — they read the
  filesystem and therefore belong to `boru:io`, not `boru:filepath`. This
  module is path-*string* algebra only (plus the cwd-dependent `abs`).

## 9. Examples

All args-before form; never `FilePath.word a b`.

```
import "boru:filepath"

["a" "b" "c.txt"] FilePath.join      # → "a/b/c.txt"  (OS separator)
"a/b/c.txt" FilePath.split           # → {dir:"a/b/" file:"c.txt"}
"a/b/c.txt" FilePath.ext             # → ".txt"

"/a/b" FilePath.rel "/a/b/c/d"       # → "c/d"  (infix form: base, target)
"a/b" FilePath.to-slash              # → "a/b"  (identity on Unix)
"*.txt" FilePath.match "c.txt"       # → true

"x/y" FilePath.abs                   # → "<cwd>/x/y"  (environmental — gated)
```

## 10. Open questions / out of scope

- Out of scope: `filepath.Glob`, `filepath.Walk` / `WalkDir`
  (filesystem access → `boru:io`); `filepath.EvalSymlinks` (filesystem +
  follows links → `boru:io`); `filepath.SplitList` (PATH-env splitting →
  `boru:os` territory).
- Open: whether `abs` belongs in this module at all, or should move to
  `boru:os` / `boru:io` so `boru:filepath` is *entirely* pure. Leaving it
  here with an explicit gate is the current proposal; revisit once the
  cwd capability seam exists.

## 11. Implementation sketch

Wiring checklist (no code). The pure words mirror `math.go`; `abs` (the
gated word) mirrors the capability-backed pattern in `io.go`:

- `lang/go/modules/filepath.go` —
  `BuildFilePathModule(parent) (ModuleDesc, error)`: fresh `subReg`,
  register inner `[]NativeFunc` (each sig `BarrierPos: -1`), wrap as
  `FnDef` exports, return `ModuleDesc{ID: parent.Modules.NextID(),
  Exports: {"FilePath": …}}`. `abs` reaches the cwd via the host
  capability seam / `HostPolicy(reg)` gate rather than calling
  `os.Getwd()` directly.
- Register `BuildFilePathModule` in the `modules` map in
  `lang/go/modules/modules.go`.
- `lang/go/modules/docs_filepath.go` — `registerDocs("boru:filepath",
  {…})` with a one-liner per export.
- `lang/spec/module-filepath.tsv` — rows leading with
  `import "boru:filepath"`; every positive row paired with an
  `ERROR:<substring>` negative sibling (`match` malformed →
  `ERROR:bad-pattern`, `rel` non-relative → `ERROR:rel`, plus a denied-
  policy row for `abs`).
- Boundary conversion via `core.FromNative` / `core.ToNative`.
- No FixedID / external-type entry.
