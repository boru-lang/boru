# boru:model vs. voxgig/struct test model — aontu nested-import defect (RESOLVED)

Status: **Resolved, June 2026.** A `github.com/rjrodger/aontu` (Go) defect
once blocked `boru:model` from reproducing the canonical `voxgig/struct`
test model. Upstream aontu fixed it (tagged `v0.1.4`, with
`github.com/tabnas/multisource` v0.3.1); after bumping the dependency,
`boru:model` builds the model and its content matches the committed
reference exactly (semantically / canonically). The history below is kept
as a record of the validation. Tracked as a design note (not an ADR).

## What was validated

`voxgig/struct` ships a model at `build/test/test.jsonic` that builds the
struct test specs by `@`-importing nine sibling `.jsonic` files under a
`struct:` tree, plus an inline `primary:` tree. The committed reference
output is `build/test/test.json` (~390 KB).

The goal was to confirm `boru:model` (which wraps the Go
`github.com/voxgig/model` → Go aontu) can build that model and match the
reference exactly.

## Result (after the upstream fix)

`boru:model` builds the model (`Model.run` → `ok:true`) and the output is
**390,826 bytes** whose content is **canonically identical** to the
390,606-byte reference: parsing both and re-serialising with sorted keys
and normalised separators yields byte-for-byte equal strings. So the
**model content matches exactly**.

The two files are not byte-identical *as written*, for serialisation
reasons only — not data:

1. **Key order.** voxgig/model's Go writer marshals `map[string]any` with
   alphabetically sorted keys (`encoding/json`); the reference is in
   TypeScript insertion order. (voxgig/model's README documents this as a
   known, accepted cross-language difference.)
2. **HTML escaping.** Go's `encoding/json` escapes `<`, `>`, `&` to
   `<` / `>` / `&` by default, which the TS encoder emits
   raw — accounting for the small size delta. Parsing normalises these
   away, so the data is unchanged.

A semantic (parse-then-compare) check is therefore the correct validation,
and it passes.

### Before the fix (historical)

With aontu `v0.1.4-0.20260622151248-c74b91f166cb` (the earlier pseudo-
version), the same build produced **705 bytes**: the entire `@`-imported
`struct` tree was missing and only the inline `primary:` tree survived,
because of the nested-import defect described below.

## Root cause (the pre-fix Go aontu pseudo-version)

A **colon-chain (nested-path) key whose value is a bare `@"file"` import**
silently resolved to `{}` instead of loading the file. With `minor.jsonic`
present in the base dir:

```
aontu.NewWithBase(dir).Generate(`x: @"minor.jsonic"`)              -> loads (24,968 B)   OK
aontu.NewWithBase(dir).Generate(`struct: { minor: @"minor.jsonic" }`) -> loads (24,983 B)   OK
aontu.NewWithBase(dir).Generate(`struct: minor: @"minor.jsonic"`)     -> {}                 BUG
aontu.NewWithBase(dir).Generate(`s: m: @"minor.jsonic"`)              -> {}                 BUG
```

`struct: minor: 1` and `struct: minor: {a:1}` (nested-path with non-import
values) resolve fine, and `key()` works on its own — so the trigger is
specifically **nested-path key + bare `@import` value**. The struct test
model uses exactly that shape (`struct: minor: @"minor.jsonic"`, …) for
every spec import, so the Go engine drops the whole `struct` branch.

The defect reproduces identically at three layers, so it is **not** in the
`boru:model` wrapper:

| Layer | `struct` resolved? |
| --- | --- |
| `boru:model` `Model.run` | no (705 B) |
| voxgig `model.New(...).Run()` | no (223 B model) |
| `aontu.NewWithBase(...).Generate(...)` | no (223 B) |

The canonical TypeScript aontu (which generated the reference) resolved
the nested-path import form correctly, which is why the reference was full
while the Go build was truncated.

**Fix:** upstream aontu corrected the nested-path import resolution
(tagged `v0.1.4`). Bumping `github.com/rjrodger/aontu/go` to it (and the
transitive `github.com/tabnas/multisource/go` to v0.3.1) makes the Go
build resolve the `struct` tree in full.

## Consequences for boru:model

- The `boru:model` module is sound: it drives a real multi-file model build
  end-to-end on disk and on an in-memory FS (see
  `lang/go/test/model_test.go`), and now reproduces the `voxgig/struct`
  test model's content exactly.
- When validating model output against a reference produced by the
  TypeScript implementation, compare **semantically** (parse then compare),
  not byte-for-byte: the Go writer's sorted keys and HTML escaping are
  serialisation-only differences.
