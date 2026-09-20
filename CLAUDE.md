# CLAUDE.md

The agent guide for this repository is **[AGENTS.md](AGENTS.md)** — read it
first. It is a top-level router: it points you at the right documentation
for the task at hand and explains how to discover the language and the CLI
straight from the tool with **`boru describe`** (words, categories, modules)
and **`boru help`** (the CLI's subcommands).

## Argument order — one rule, no exceptions

Read this before writing a line of boru. A call binds its arguments **in
signature order**: matching fills positions from the **forward stack**
(the tokens written after the word, in written order) up to that
signature's barrier, then fills every remaining position from the **value
stack** in reverse — top of stack first, then next-deeper.

That is the whole rule, at every arity. **Two-argument words are not a
special case, and there is no "swap form."** Both phrasings are legacy
misunderstandings; if you meet one in a doc, a comment, or a review, fix
it. A call form only chooses where the split falls.

Two consequences follow, and they are the ones that surprise people:

- Put every operand on the value stack and you get **Forth order** —
  `10 3 sub` is `7`.
- Write every operand after the word and you get **written order**, which
  for a non-commutative word reads backwards — `sub 1 3` is `2`, because
  `sub` computes `args[1] - args[0]`.

Surface style ([STYLE-GUIDE.md](STYLE-GUIDE.md) §S2): **infix** for the
two-argument words convention reads as operators — `add`, `sub`, `mul`,
`div`, `mod`, `pow`, `and`, `or`, `lt`, `lte`, `gt`, `gte`, `eq`, `neq`
(`1 add 2`, `10 sub 3`, `n lte 1`) — and **forward form `f a b c`** for
everything else.

For a fast, structured orientation, the repository also ships a
**project knowledge graph** — modules, packages, docs, tools, and
concepts with evidence-backed relations. **Read
[kg/out/graph.md](kg/out/graph.md)**: it is the short outline, with the
module dependency view (what each Go module depends on and what depends
on it, read from `go.work` and every `go.mod`) first. The full bundle is
[kg/out/graph.json](kg/out/graph.json) — the machine contract, and far
too large to read whole. Guide: [kg/README.md](kg/README.md); check the
graph against the tree with `make -C kg verify`.

Module-specific deep guides — read the relevant one **before** changing
that module:

- Engine kernel (types, values, matching, parser): [eng/go/CLAUDE.md](eng/go/CLAUDE.md)
- Interpreter core (the standalone module cut from eng; kernel conventions apply): [core/go/CLAUDE.md](core/go/CLAUDE.md)
- Parser (the standalone front end: source text → `[]core.Value`; a leaf over core, gated at 100%): [parser/go/CLAUDE.md](parser/go/CLAUDE.md)
- Type checker (the check module: analysis pass, carriers, diagnostics; kernel conventions apply): [check/go/CLAUDE.md](check/go/CLAUDE.md)
- Compiler (the compiler module: recorder, lowering, bytecode emitter; kernel conventions apply): [compiler/go/CLAUDE.md](compiler/go/CLAUDE.md)
- Base language layer (fundamental words, predefined content types): [basic/go/CLAUDE.md](basic/go/CLAUDE.md)
- Language layer (native words, modules, registry): [lang/go/CLAUDE.md](lang/go/CLAUDE.md)
- Knowledge-graph pipeline (schema, ids, resolution, validation): [kg/README.md](kg/README.md)

Before committing, run the **commit gate** from the repo root — three
minutes or less, on what the change touched:

```bash
make commit-gate
```

It runs gofmt, vet and golangci-lint on the touched modules (in parallel),
the touched modules' unit tests (the changed packages of `lang/go` and
`cmd/go`), the langspec gates over a smoke corpus plus every spec file the
change touched, and the knowledge graph when docs or tooling changed
(`scripts/commit-gate.sh` is the definition). Before a push, `make
ci-local` runs exactly the steps CI runs (`scripts/ci-steps.sh` is the one
definition both share); CI runs the same steps as parallel jobs, each
under the same three-minute ceiling. Two switches make iteration fast:
`BORU_SPEC_FILES=callbacks.tsv,fold-*.tsv` restricts every corpus walk in
`test/go/langspec` to the named spec files (the ten gates over one family
run in seconds; the per-file compile-failure ledger
`test/go/langspec/compile_failures.tsv` still asserts on every selected
file, so a compile regression in that family fails the filtered run), and
`BORU_DIRECTION_GATES=1` arms the direction lane —
the gates against their END STATE, red by design until full compilation
is done (`make test-direction`; the default lane asserts only the
regression ceilings and is what blocks). `make gate-status` prints every
gate's live value against both numbers; CI renders the same table into
every run's summary from the langspec shards.

`make cover-gate` enforces **ADR-008**: 100% unit-test coverage of every
reachable Go statement (the sole exclusions are provably-unreachable guards
marked with a proof-carrying `//covergate:allow <reason>` comment on the
guard's opening line — see `design/COVERAGE-ALLOWLIST.10.md`). It is
cached per module and takes about half an hour cold, so it runs nightly
(`cover-gate.yml`) and before a merge, not on every commit.

If your change touches the repository's structure, tooling, or
documentation set, also keep the project knowledge graph current:
update [kg/project/boru-project.jsonic](kg/project/boru-project.jsonic)
and rebuild the committed bundle with `make -C kg graph` (see
[kg/README.md](kg/README.md)); `make -C kg check test` verifies the
pipeline itself.

> **The kg gate is DEACTIVATED (2026-09-20) and `make -C kg graph` does
> not currently run.** It dies at `pc=8` with `DISPATCH_GENERIC at ev` —
> the generic lane's evaluating host, one of the three unbuilt cores
> ([design/FULL-COMPILATION-REVIEW.0.md](design/FULL-COMPILATION-REVIEW.0.md)
> §2), verified identical on a clean worktree at `ba64e11`. The
> `kg-verify` step in `scripts/ci-steps.sh` and the matching commit-gate
> lane are off until it lands; both carry the re-activation instruction.
> So a doc change is NOT blocked on the graph today. If you need the
> graph rebuilt meanwhile, run the generator on the REFERENCE ENGINE
> (`a.RunInterp` over `kg/main.boru`, working directory `kg/`) from a
> throwaway test in an EXISTING package — a new package changes the
> go-tree digest the graph hashes, so the graph goes stale the moment you
> delete it.
