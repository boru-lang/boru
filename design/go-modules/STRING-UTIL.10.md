# `boru:string-util` (expanded) — Unicode classification

> **Status: design proposal, not implemented.** This note specifies an
> expansion of the existing `boru:string-util` module
> (`lang/go/modules/string.go`, namespace `StringUtil`) to **own all
> Unicode functionality**: per-rune classification predicates and case
> mapping. It supersedes the standalone `boru:unicode` note (now removed).
> Read [README.10.md](README.10.md) first for the shared conventions.

## 1. Why fold Unicode into string-util

`string-util` already owns text work and delegates to Go's `strings`,
`regexp`, **and `unicode`** (it uses `unicode` today for `normalize` /
NFC). Rune classification (`IsLetter`, `IsDigit`, …) is the same domain at
a finer grain. Rather than a separate `boru:unicode` module the user's
decision is to **concentrate all Unicode in `string-util`**, so a single
import (`import "boru:string-util"`) covers casing, normalization, search,
*and* character classification, discoverable together under `describe
StringUtil`.

## 2. Import & namespace

```
import "boru:string-util"     # binds the StringUtil namespace (unchanged)
```

Unchanged existing module. New words dot-access flat: `StringUtil.is-alpha`,
`StringUtil.is-digit`, etc.

## 3. Design choice: whole-string predicates (the key refinement)

The standalone `boru:unicode` note classified the **first rune** of a
String (`"abc" Unicode.is-letter` → tested `'a'`). `string-util` is
**whole-string oriented**, so the folded surface adopts the idiomatic
whole-string contract (the Python `str.isalpha()` / `isdigit()` model):

> A classification predicate is true iff the string is **non-empty** and
> **every** rune satisfies the class.

This **subsumes the single-character case** — for a 1-char String it
answers exactly the old per-rune question (`"a" StringUtil.is-alpha` →
true) — while also answering the more useful whole-string question
(`"abc" StringUtil.is-alpha` → true, `"ab1" StringUtil.is-alpha` →
false). An **empty string is false** for every predicate (no rune to
satisfy the class), so the words are **total** — no `expected-single-char`
error, unlike the per-rune draft.

(If a strict per-character contract is ever wanted, classify a sliced
single char: `0 1 StringUtil.substr ... StringUtil.is-alpha` — but the
whole-string form is the primary, and recommended, surface.)

## 4. API — new classification predicates

`String -> Boolean`, true iff non-empty and every rune is in the class.
Top-first sig order; inner native sigs `BarrierPos: -1`; invoked
args-before-dot (single-arg words dispatch in forward and stack form).

| Go per-rune symbol | boru word | one-line doc | refinement |
|---|---|---|---|
| `unicode.IsLetter` | `is-alpha` | True if every rune is a letter (Unicode L). | whole-string "all"; empty → false. |
| `unicode.IsDigit` | `is-digit` | True if every rune is a decimal digit 0–9. | whole-string. |
| `unicode.IsNumber` | `is-numeric` | True if every rune is a number (Unicode N, incl. ⅗, Ⅻ). | broader than `is-digit`. |
| `IsLetter`+`IsDigit` | `is-alnum` | True if every rune is a letter or digit. | composite (Python `isalnum`). |
| `unicode.IsSpace` | `is-space` | True if every rune is whitespace. | whole-string. |
| `unicode.IsUpper` | `is-upper` | True if there is a cased rune and all cased runes are upper. | ignores uncased runes (Python `isupper`). |
| `unicode.IsLower` | `is-lower` | True if there is a cased rune and all cased runes are lower. | ignores uncased runes. |
| `unicode.IsTitle` | `is-title` | True if the string is title-cased. | word-initial caps. |
| `unicode.IsPunct` | `is-punct` | True if every rune is punctuation (Unicode P). | whole-string. |
| `unicode.IsControl` | `is-control` | True if every rune is a control character. | whole-string. |
| `unicode.IsPrint` | `is-printable` | True if every rune is printable. | whole-string (Python `isprintable`). |
| `r < 0x80` | `is-ascii` | True if every rune is ASCII (< 128). | convenience; common gate. |

## 5. Case mapping (already present + one addition)

Whole-string case mapping already exists and works on single characters
too — **no per-rune `to-upper`/`to-lower` words are added** (they would
duplicate these):

- `StringUtil.upper` / `StringUtil.lower` — existing
  (`lang/go/modules/docs_string.go`); `"a" StringUtil.upper` → `"A"`.
- `StringUtil.changecase` — existing; recases whole strings
  (lower/upper/camel/snake/…).

Add for completeness if not already a `changecase` mode:

| word | signature | doc |
|---|---|---|
| `title` | `[String] -> String` | Title-case the string (Unicode `ToTitle` per word). |

## 6. Types

Scalars only — String in, Boolean (predicates) or String (`title`) out.
No opaque handle, no `RegisterExternalBuiltin`/FixedID. The boundary is a
rune walk over the String in the handler (`for _, r := range s`), not a
slice copy.

## 7. Errors

The classification predicates are **total** (empty → false, any content
classified rune-by-rune), so they raise no errors. `title` is total.
Guard inputs with `AsConcreteString`; never panic (`core/go/CLAUDE.md`
"Panic Prevention").

## 8. Overlap

This is the consolidation target, so there is no cross-module overlap to
reconcile — the Unicode words now live alongside the existing
`StringUtil` surface:

- `normalize` (NFC + whitespace tidy) and `changecase` already use Go
  `unicode`; the new predicates extend that same delegation.
- No existing word is moved or renamed; the change is purely additive
  (new predicates + optional `title`).
- Related: `boru:bin-util` has `ord` (String→Integer codepoint) / `chr`
  (Integer→String); code-point access stays there, classification here.

## 9. Examples (args-before form)

```
import "boru:string-util"

"abc" StringUtil.is-alpha               # true
"ab1" StringUtil.is-alpha               # false
"a" StringUtil.is-alpha                 # true   (1-char case still works)
"12345" StringUtil.is-digit             # true
"Ⅻ" StringUtil.is-numeric               # true   (Roman numeral: Number, not Digit)
"HELLO" StringUtil.is-upper             # true
"Hello World" StringUtil.is-title       # true
"  " StringUtil.is-space                # true
"" StringUtil.is-alpha                  # false  (empty → false, no error)
"café" StringUtil.is-ascii              # false
```

## 10. Open questions / out of scope

- **Strict per-character mode** — not provided; the whole-string form
  subsumes single-char use. Revisit only if a "exactly one rune" contract
  is requested.
- **Script / category tables** (`unicode.Latin`, `unicode.Han`, `Is`/`In`)
  — out of scope for the first cut; revisit if script detection is needed.
- **`SimpleFold`** — case-insensitive fold iteration; deferred.
- **`is-empty` / `is-blank`** — trivial helpers; `is-space` plus a length
  check covers blank. Left out unless requested.

## 11. Implementation sketch

Wiring checklist — no Go code. Reference: the existing string module
(`lang/go/modules/string.go`, `BuildStringModule`) and `math.go`'s
table-driven `classifierNative` for the predicate generator.

- `lang/go/modules/string.go` — extend `BuildStringModule`: register the
  new predicate natives (generate from a `(name, func(rune) bool)` table
  applied "all runes, non-empty"; the composite/`is-ascii`/`is-upper`
  ignore-uncased rules are small custom handlers) and the `title` word
  into the existing sub-registry; export `FnDef` wrappers (inner sigs
  `BarrierPos: -1`). The `unicode` import is already present.
- `lang/go/modules/docs_string.go` — add a one-line `registerDocs`
  entry per new export (`TestModuleExportDocs` enforces completeness).
- `lang/spec/module-string.tsv` — positive rows leading with `import
  "boru:string-util"` plus an `ERROR:`/negative sibling where expressible
  (e.g. `is-alpha` on mixed/empty inputs returning `false`), per Test
  discipline (`lang/go/CLAUDE.md`).
- No FixedID entry, no policy wiring (pure).
