# DECLARATIVE-GRAMMAR.0 — one grammar artifact for both parser twins

**Status:** In progress · **Started:** 2026-08-05 (maintainer
instruction: "extract a declarative tabnas grammar that both go and ts
should use").

## The contract

`parser/go/grammar.json` is the single source of the boru
grammar's STRUCTURE, loaded by both parsers:

- **Go** embeds it (`go:embed`, `parser/go/declgrammar.go`) and
  applies it in `Parse`'s stage-2 setup. It moved with the parser when
  that became a top-level module — it was `parser/go/grammar.json`
  while the parser was a package inside the kernel.
- **TS** reads the same file relatively
  (`eng/ts/src/parser/declgrammar.ts`, the `SPEC_DIR` precedent) and
  applies it in `makeBoruJsonic`.

The artifact expresses, in registration order (order is load-bearing):

1. **The token table** — fixed tokens with their source text
   (`{"name": "OP", "text": "("}`), matcher-produced tokens by bare
   name (`#TL`, `#ML`, `#XML`). Go registers them via `j.Token`; TS
   pre-declares the fixed set via `options({fixed:{token}})` and
   resolves/mints via `j.token` — same names, same order, so the two
   lexers agree by construction.
2. **Rule-spec edits** — `{rule, op, alts}` where `op` is one of
   `prependOpen / appendOpen / setOpen / prependClose / appendClose /
   setClose` and each alternate carries the tabnas AltSpec DATA:
   `s` (token-name sequences, canonical arrays-of-alternative-groups),
   `p` (push rule), `r` (replace rule), `b` (backtrack count).
3. **Behavior by NAME** — an alternate's action is either the `text`
   shorthand (emit that unquoted Text marker as the node — the
   dominant case) or a named `a` hook; conditions are named `c` hooks.
   Each language binds names from its hook table
   (`valDeclHooks` in `grammar.go` / `grammar.ts`); an unknown name
   panics/throws at parser construction, so the artifact and the hook
   tables are one contract enforced on both engines.

What stays per-language: the custom lex matchers (template literal,
big-number, minilang, XML), the converters, and the hook bodies — the
grammar's IMPLEMENTATION; the artifact owns its SHAPE.

## Migration state

Rule-by-rule, both engines in lockstep, full suites (parser tests,
streamdump, the corpora, the cross-engine differential) green after
every batch:

| Batch | Content | State |
|---|---|---|
| 1 | Token table; `valof` prependOpen marker batch (ML/OP/LA/BT pushes, the 7 marker tokens, arrowTag); `valof` prependClose dot-chain (inPairValue gate) | **landed** |
| next | the remaining `valof` edits, `pair` (8 edits), `elem`, `paren`/`pelem`, `interp`/`ielem`/`iexpr`/`ieval`, `angle`/`aelem`, `arrowfold`/`arrowfoldelem`, `dotchain`, `map` | pending |

Batch 2+ needs three schema additions the paren/interp rules use:
named BO/BC/AC/AO action LISTS per edit, alternate `u` maps (pure
data), and library-builtin token references (`$ZZ` end-of-source,
`$CA` comma) resolved from a per-language builtin table. The number
SUBSCRIBER (`setupNumberSub`) is lexer machinery like the custom
matchers — it stays per-language, outside the artifact.

The legacy TS hand-rolled tokenizer is already deleted (its value
contract predated ADR-012 opacity); tabnas is the parser, everywhere.
