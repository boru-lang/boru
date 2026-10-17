# `path` → `boru:path-util`

> **Status: design proposal — not implemented.** A curated, hand-written
> native module wrapping Go's `path` (slash-separated paths). Read
> [`README.10.md`](README.10.md) first for the shared conventions this
> note assumes.

## 1. Package & status

Go package: `path` — manipulation of **slash-separated** paths (URL
paths, archive entries, virtual hierarchies — *not* OS file paths, which
are `path/filepath`, see [`PATH-FILEPATH.10.md`](PATH-FILEPATH.10.md)).
This note specifies `boru:path-util` (namespace `PathUtil`). Design
proposal; no Go code exists yet.

## 2. Why curated

The raw `go:path` bridge would expose Go names and the two-return
`(dir, file)` shape of `Split` as a positional pair the user has to
destructure. The curated surface kebab-renames the words, refines
`Split` into a `Map {dir, file}`, makes `Join`'s variadic a single `List`
argument, and collapses `Match`'s `(matched, error)` into a Boolean (with
a `bad-pattern` error). Pure string algebra, idiomatic shape.

## 3. Import & namespace

```
import "boru:path-util"        # binds the PathUtil namespace
```

The `-util` id + `*Util` namespace is **REQUIRED here**: the bare
namespace would be `Path`, which **collides with the builtin `Path`
type** in the kernel. Per the README naming rule ("the `-util` id +
`*Util` namespace is used **only** when the bare namespace would collide
with a builtin type … `Path` …"), this package takes `boru:path-util` /
`PathUtil`. Words: `PathUtil.join`, `PathUtil.base`, … The `-util` suffix
also reads correctly — this *is* a utility library of pure helpers.

## 4. API

Signatures are **top-first, sig order** (position 0 = top of stack). All
inner natives use `BarrierPos: -1` so the infix form `a PathUtil.word b`
dispatches.

| Go symbol | boru word | signature (top-first) | one-line doc | boru-ish refinement |
|---|---|---|---|---|
| `path.Join` | `join` | `[List] -> [String]` | Join path elements with `/` and clean the result. | Variadic `...string` → a single `List[String]` argument (one List value, idiomatic). Empty / empty-string elements skipped, as in Go. |
| `path.Split` | `split` | `[String] -> [Map]` | Split a path into its directory and final element. | Two returns `(dir, file)` → a `Map {dir, file}` so the caller reads by name rather than destructuring a pair. |
| `path.Base` | `base` | `[String] -> [String]` | Last element of the path. | Pure `String → String`. Empty path → `"."`; trailing slashes stripped (Go semantics). |
| `path.Dir` | `dir` | `[String] -> [String]` | All but the last element, cleaned. | Pure `String → String`. |
| `path.Ext` | `ext` | `[String] -> [String]` | File-name extension including the leading dot. | Pure `String → String`; empty when none. |
| `path.Clean` | `clean` | `[String] -> [String]` | Shortest equivalent path (resolves `.` / `..`). | Pure `String → String`. |
| `path.IsAbs` | `is-abs` | `[String] -> [Boolean]` | True if the path is absolute (begins with `/`). | `String → Boolean`. |
| `path.Match` | `match` | `[String, String] -> [Boolean]` | Match a name against a shell-style pattern. | sig is `pattern, name` (pattern top-first). `(matched, error)` → Boolean, with a malformed pattern raising `bad-pattern` rather than a third return. |

## 5. Types

Scalars / List / Map only. No opaque external handle, no FixedID
allocation.

## 6. Errors

No panics — guard with `AsConcreteString` / `RequireConcreteList` before
use. Failure via `r.BoruError(code, detail, word)`:

- `match` — Go `path.ErrBadPattern` → `bad-pattern`.
- A non-String element inside the `join` List, or a non-String arg
  anywhere → `bad-arg`.

`split` / `base` / `dir` / `ext` / `clean` / `is-abs` never fail on a
well-typed String input (Go returns defined results for `""`).

## 7. Policy / capabilities

**None — pure.** Pure string manipulation; no filesystem, network, env,
or clock access. Runs under any policy. (Note: `path` does **not** touch
the filesystem at all — even `path/filepath`'s `abs` is absent here
because slash paths have no cwd.)

## 8. Overlap

- `boru:filepath` (`FilePath`) — the **OS-aware** sibling
  ([`PATH-FILEPATH.10.md`](PATH-FILEPATH.10.md)). Same word names
  (`join`, `split`, `base`, …) but `path/filepath` uses the
  OS separator and adds `rel` / `to-slash` / `from-slash` /
  `volume-name` / `abs`. Dividing line: `boru:path-util` is *always*
  slash-based and *always* pure (good for URLs and portable virtual
  paths); `boru:filepath` is platform-dependent. They do not share code or
  move each other's words.
- The builtin `Path` **type** is unrelated kernel machinery; this module
  operates on plain `String` values, which is why the `-util` rename is
  needed.

## 9. Examples

All args-before form; never `PathUtil.word a b`.

```
import "boru:path-util"

["a" "b" "c"] PathUtil.join          # → "a/b/c"
["a/" "/b"] PathUtil.join            # → "a/b"  (cleaned)

"a/b/c.txt" PathUtil.split           # → {dir:"a/b/" file:"c.txt"}
"a/b/c.txt" PathUtil.base            # → "c.txt"
"a/b/c.txt" PathUtil.dir             # → "a/b"
"a/b/c.txt" PathUtil.ext             # → ".txt"
"a/./b/../c" PathUtil.clean          # → "a/c"
"/a/b" PathUtil.is-abs               # → true

"*.txt" PathUtil.match "c.txt"       # → true   (infix form: pattern, name)
```

## 10. Open questions / out of scope

- Out of scope: nothing material — `path` is small and fully covered.
  (`path` has no `Rel` / `VolumeName` / `Abs`; those are filepath-only.)
- Open: whether `join` should also accept the variadic-on-stack spelling
  in addition to a single List (the math module keeps variadics as a
  List; we follow that for consistency).

## 11. Implementation sketch

Wiring checklist (no code), mirroring `math.go` (pure module):

- `lang/go/modules/path_util.go` —
  `BuildPathUtilModule(parent) (ModuleDesc, error)`: fresh `subReg`,
  register inner `[]NativeFunc` (each sig `BarrierPos: -1`), wrap as
  `FnDef` exports, return `ModuleDesc{ID: parent.Modules.NextID(),
  Exports: {"PathUtil": …}}`.
- Register `BuildPathUtilModule` in the `modules` map in
  `lang/go/modules/modules.go`.
- `lang/go/modules/docs_path_util.go` — `registerDocs("boru:path-util",
  {…})` with a one-liner per export.
- `lang/spec/module-path-util.tsv` — rows leading with
  `import "boru:path-util"`; every positive row paired with an
  `ERROR:<substring>` negative sibling (notably `match` with a malformed
  pattern → `ERROR:bad-pattern`).
- Boundary conversion via `core.FromNative` / `core.ToNative`
  (String↔`string`, List↔slice, Map↔`map[string]any`).
- No FixedID / external-type entry.
