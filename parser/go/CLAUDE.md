# parser/go — the boru parser CLAUDE.md

The `parser` module turns boru **source text** into `[]core.Value`. It is
the front end and nothing else: no evaluation, no analysis, no compilation.
Cut out of `eng/go/parser` (design/legacy/ENG-FOUR-PIECE.0.ignore's split, extended
below the kernel rather than across it).

Stage 3 of parsing lives here in full — the tabnas/jsonic lexer setup, the
declarative grammar artifact (`grammar.json`, `go:embed`ed), the imperative
rule extensions, and the conversion walk that turns jsonic nodes into kernel
values.

## The one rule

**parser depends on `core` and NOTHING else in the repo.** Not eng, not
check, not compiler. It uses 109 core symbols (`core.Value`, the `New*`
constructors, the `Is*`/`As*` predicates, `core.BoruError`, the `T*` type
nodes) plus `tabnas/jsonic` and `apd`. Nothing in the repo depends on the
parser except the layers above it, so it is a **leaf over core** — which is
why its coverage gate sits at 100% from day one instead of on a ratchet:
there is no other module's suite that could be covering it, and no seam
whose far side lives elsewhere.

If you find yourself wanting an eng symbol here, the dependency is pointing
the wrong way. Either the symbol belongs in core, or the behaviour belongs
in the caller.

## Names are never invented

`nameless_gate_test.go` is a source-scanning gate, not a behaviour test: it
greps this package for `core.NewWord("literal")` / `core.NewAtom("literal")`
and for `jsonic.Text{Str: "…"}` substitutions, and fails on anything not in
its allowlist. The parser must never mint a name the user did not write —
syntax that has no name parses to a structural marker (`core.NewSugar`) that
the engine lowers, so the name stays the user's.

The gate's own regex is part of the contract: when this package moved out of
`eng/`, the pattern still matched `eng\.` and would have silently stopped
catching anything. It is self-tested (`TestParserNeverInventsNamesCatches`)
precisely so that cannot happen quietly.

## LexTokens is a seam, not an internal

`LexTokens` is the trivia-preserving token stream a formatter front end
consumes (`lang/go/formatter`). Its consumer is in another module, so it must
be tested HERE — before the cut it was covered incidentally from lang and had
no test of its own, which the standalone gate immediately exposed.

Note its bool result means "the LEXER reported no error", which is narrower
than "well formed": an unterminated backtick scans clean, because the
backtick is an ordinary operator token and the template literal is assembled
by a grammar rule the bare `Next` loop never runs.

## Coverage

`make cover-gate-parser` gates this module by its own suite at **100%**, on
top of the repo-wide merged ADR-008 gate. The three `//covergate:allow`
exclusions are the embedded artifact's decode/schema guards (compile-time
constant JSON, proven valid by `TestDeclGrammarLoads`) and one converter arm
whose sole caller pre-converts every item.
