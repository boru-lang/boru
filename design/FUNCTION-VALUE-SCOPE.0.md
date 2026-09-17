# FUNCTION VALUE SCOPE

Where a function value's **free words** resolve — the module that *defined*
it, or the module that *runs* it — and why boru answered both, depending on
which engine executed the program.

**Status (2026-08-15): rule 1 is IMPLEMENTED; rules 2 and 3 are not.**
The divergence this document opens with — a **compile ≠ interpret
disagreement that returned a silently wrong number**, verified first-hand
against `boru 0.1.0-dev (git 96bf5f9e)` — is fixed: a function value now
resolves its free words in its defining module on both engines, whether it
is applied, bound to a name, or passed through a native callback. See
**§12** for exactly what landed, what did not, and why. The file keeps its
`.0` suffix because §11's other two clauses are still design-only, and
because a dozen code comments cite this path.

The defect is not recorded anywhere else in `design/` — the closest prior
mentions (`FN-VALUE-DISPATCH.0.md` §, `VOXGIG-COMPILE-LEAVES.1.md`) are
compile-time emitter findings of the same *shape* but a different problem.
This is the first statement of the runtime rule.

## 1. Summary

A function value that crosses a module boundary should keep access to the
module that defined it. In boru it does — **on the bytecode compiler**.
On the interpreter it does not: the body's free words are resolved in the
*running* module, so a module-private helper is either not found, or —
worse — silently bound to a **same-named word in the calling module**.

```boru
# mod-a.aql — `pub` calls A's private `secret` (x + 1)
# mod-b.aql — B has its OWN `secret` (x * 100), and applies a Function param
print (B.apply1 A.pub 5) end
```

| Engine | Result | |
|---|---|---|
| `boru --no-compile t3.aql` | **`500`** | ran **B**'s `secret` — silently wrong |
| `boru --force-compile t3.aql` | **`6`** | ran **A**'s `secret` — correct |
| `boru t3.aql` (default) | `6` | whichever path the program happens to take |
| `boru check t3.aql` | **0 errors, 0 warnings** | statically invisible |

The default mode compiles when it can and, when it cannot, *silently
routes the program to the interpreter* — scaffolding that absorbs a
compile refusal without ever reporting it — so **the same source can
produce either answer depending on whether an unrelated part of the
program happens to be compilable.** No diagnostic fires in either
direction.

This is the outcome the repository's own tests call the cardinal
forbidden one (`lang/go/bytecode_stored_handler_freeze_test.go`: "a
compile ≠ interpret MISCOMPILE (the cardinal forbidden outcome)"), and it
is the same class as the stored-handler defect fixed in
`RELOAD-INVALIDATION.0.md` §3 — with one aggravating difference: that one
produced a *wrong-but-explicable* value from hoisted state, while this
one can run **an entirely different function than the one written**.

**The mechanism for the correct behaviour already exists and is already
set.** `FnDefInfo.Registry` holds the defining module's registry, and its
doc comment states the intent verbatim (`core/go/value.go:697-700`):

> If Registry is non-nil, the function was defined in a module and should
> execute in that registry's context (closure semantics).

The defect is that only *some* dispatch paths read it. This is therefore
a **wiring gap, not a design flaw** — which is what makes it worth
fixing rather than documenting.

## 2. The rule, as measured

Three dispatch classes, three different behaviours:

| Class | How the value is invoked | Interpreter | Bytecode |
|---|---|---|---|
| **Value dispatch** | `A.pub 5`; an *unnamed* `Function` param applied directly | ✅ defining module | ✅ defining module |
| **Name dispatch** | bound to a name — `def g (A.pub/r)`, or a **named** `f:Function` param | ❌ **running module** | ✅ defining module |
| **Native callback — group 1** (6 sites) | `filter`, map-lambda `each`/`fold`, `StructUtil.walk`, core `walk`, `IO.mount`, `boru:parse` | ❌ **running module** | ❌ **running module** |
| **Native callback — group 2** (8 sites) | `RunPredicate`, service handlers ×2, codecs, `serve-raw`, `Model`, `Tui.run` update/view | ❌ **running module** | ✅ defining module |

> **Correction (§10 audit).** An earlier revision of this table claimed *all*
> native-callback sites were wrong on both engines. That holds only for
> group 1. Group 2 already diverges like name dispatch — and two of them
> (`net_codec.go:100`, `native_service.go:419,423`) are **verified
> miscompiles today, check-clean**. `RunPredicate` is the worst single
> site in the tree: `native_type.go:825-829` converts its failure into a
> silent `false` rather than an error, so a `refine` predicate that
> cannot resolve its own helper quietly rejects every value.

Two consequences worth stating separately, because they have different
severities:

- **Name dispatch diverges between engines** (§1). Silent, and mode-dependent.
- **The native-callback seam is wrong on *both* engines.** It is not a
  divergence — it is uniformly the *wrong* scope. This is the class that
  matters most for the library ecosystem, because every callback API in
  boru goes through it: comparators, predicates, service handlers,
  codecs, TUI update/view, `RunPredicate`.

Within a class the behaviour is uniform: invocation shape (`each`,
`fold`, `do`, nested fn) does not change the outcome, and the failure is
a catchable runtime error, not a static one.

**The blast radius on the interpreter is total.** A function value
invoked by name across a module boundary loses its module's private
helpers, its module-level values, **its module's other exports**, and any
namespace its module imported. Only built-ins survive.

**The leak is one-directional**, which is the one piece of good news: a
caller never gains the callee module's privates, and a caller's
same-named definitions never corrupt a library's own *by-name* internal
calls. Only values that cross the boundary are affected.

## 3. Reproduction

```bash
cd $(mktemp -d)
cat > mod-a.aql <<'EOF'
def secret fn [[x:Integer] [Integer] [ x add 1 ]]
def pub    fn [[x:Integer] [Integer] [ secret x ]]
export "A" { pub: pub/r }
EOF
cat > mod-b.aql <<'EOF'
def secret fn [[x:Integer] [Integer] [ x mul 100 ]]
def apply1 fn [[f:Function n:Integer] [Integer] [ (f n) ]]
export "B" { apply1: apply1/r }
EOF
cat > t3.aql <<'EOF'
import "./mod-a.aql" end
import "./mod-b.aql" end
print (B.apply1 A.pub 5) end
EOF

boru --no-compile   t3.aql   # => 500   ← B's secret. WRONG, no error
boru --force-compile t3.aql  # => 6     ← A's secret. correct
boru check          t3.aql   # => 0 error(s), 0 warning(s)
```

A second, louder shape — rebinding an export under a new name — fails at
**check** time on both engines, which at least is honest:

```bash
cat > t9.aql <<'EOF'
import "./mod-a.aql" end
def g (A.pub/r)
print (g 5) end
EOF
boru t9.aql   # => check: 2:37: [error] undefined_word: undefined word: secret
```

## 4. Mechanism

### 4.1 Where `Registry` is set

Only at module-export resolution — `resolveModuleExport`
(`lang/go/native/native_module_module.go:800-802`, and the same three
branches at `:830`, `:841`), with a local mirror in
`lang/go/modules/test.go:226-253`. The stated intent (`:792-795`):

> A function value — typically produced by `name/r` in the export map …
> must carry the module registry so it executes in module scope
> (resolving module-private words) when called after import.

Everything else merely propagates it by struct copy.

### 4.2 Which paths honour it

| Path | File:line | Honours `fnDef.Registry`? |
|---|---|---|
| `execFnDefLiteral` (value at the pointer) | `core/go/engine.go:5003` | **yes** — foreign-registry branch `:5277-5352` |
| `ExecFnDefSigStackMatch` | `core/go/engine.go:5474` | **yes** (pass-through) |
| `execFnDefSig` | `core/go/engine.go:5754` | **yes** — via its `capturedReg` parameter |
| trivial-delegation short-circuit | `core/go/engine.go:5250` | **yes** |
| VM `tryNativeFnApply` | `eng/go/vm.go:1113` | **yes** — and *declines* a foreign boru body so the island honours it |
| **`CallBoru` / `CallBoruNamed`** | `core/go/registry.go:1535` | **no — structurally cannot.** A method on a Registry; no `FnDefInfo` parameter exists |
| **`InvokeCallback`** | `core/go/invoke.go:65` | **no — structurally cannot.** Takes `(r, sig, args, captures)`; no `FnDefInfo` |
| **`InstallFnDef`'s handler** | `core/go/core_helpers.go:678` | **no** — closes over the *install-time* registry |
| VM `CALL_USER` | `eng/go/vm.go:1815` | different channel (`CompiledFn.Reg` = the analysis-pass registry) |

The `InstallFnDef` handler documents the drop as deliberate
(`core/go/core_helpers.go:235-238`):

> The handler closes over the install-time registry `r`: an FnDef
> registered into a name dispatches in the registry it was installed in.
> (The registry passed to the Handler at call time is intentionally
> ignored here — this mirrors the historical InstallFnDef closure.)

That comment is the whole bug in four lines. It was written about
*name registration*, and it is correct for a fn defined in the running
module — but it also swallows the case where the value arrived from
somewhere else carrying its own `Registry`.

### 4.3 Why binding to a name loses it

`A.pub 5` puts the export value at the pointer, where `execFnDefLiteral`
sees `fnDef.Registry` and routes to the foreign branch. But *binding* it
to a name — including binding it to a **named `Function` parameter** —
funnels through `installDef` → `compileFnSigs` → `buildFnBodyHandler(r,
…)`, which rebuilds the dispatch handler closed over the **installing**
registry. `entry.Registry` survives as a *field*, but the handler is what
`execMatch` actually calls, so the field is never consulted again.

The asymmetry is therefore: **applying a value keeps its scope; naming it
loses it.** A named `f:Function` parameter is the single most common way
a library receives a callback.

### 4.4 Why closure capture cannot fix it

`Captured` holds enclosing-fn locals only. `ComputeCaptures`
(`core/go/fn_capture.go:301-365`) skips any name whose
`Defs.Depth(name) <= baseline` — which is *precisely* the module-level
category:

| Reference resolves to… | Captured? |
|---|---|
| enclosing-fn param / body-local | **yes** — snapshotted |
| **module-level or global `def`** | **no** — dynamic lookup at call time |
| native word | no (never in `r.Defs`) |
| unbound name (forward ref, recursion) | no |

A module-private helper is exactly what `Captured` refuses to hold.
Only `fnDef.Registry` can carry it — which is why the fix has to be
wiring that field through, not extending capture (§7 option (b)).

### 4.5 Module registries are disjoint

`RunModuleBody` builds each module a **brand-new independent
`*Registry`** (`newSubRegistry`), inheriting host capabilities and I/O
plumbing by an explicit list — but **not `Defs`**. There is no parent
chain: `lookupUncached` reads `r.Defs` and nothing else. The two things
that look like a chain are not — `SetDebugParent` carries only the debug
hook, and `Contexts.PushExisting` shares only the `ctx-*` store.

So a lookup from the wrong registry cannot reach the right definitions
*at all*; it either misses, or hits a same-named word and returns the
wrong answer.

## 5. Not this bug — a separate, adjacent fault

While characterising this I checked a claim I had made elsewhere: that
the `sort` library's combinator failure shares this root cause. **It does
not**, and the distinction matters because the two need different fixes:

| | combinator fault | scope fault (this doc) |
|---|---|---|
| Symptom | `uncalled_function: call to 'by-number' matched no signature` | `undefined_word: <name>`, or a wrong value |
| Phase | **check / static**, exit 1, never runs | **runtime**, catchable |
| Mode-dependent | **standalone: no** — the check error is identical on both engines. **In a suite: yes** — `sort_unit_test.aql` is `ok` compiled and `FAIL` interpreted (corrected 2026-08-15; see §12.4) | **yes** (name dispatch) |
| Fix | add `/r` → works | `/r` is irrelevant |

`(Sort.by-number Sort.reverse)` fails because the bare namespace word is
**auto-invoked with zero args during forward collection** inside the
combinator call; `(Sort.by-number/r Sort.reverse)` works. That is an
arity/dispatch issue and belongs in its own record. It is noted here only
so the two are never again conflated.

## 6. The cost today

The workaround is "put everything in one module", and the ecosystem is
already paying for it:

- **`sort`** is a single 1,490-line file *by necessity*. Its header states
  the rule: "boru resolves a function value's free words in the module
  that runs it, so a comparator that calls a private helper must live in
  the same module as the algorithms that invoke it." Splitting the
  comparators into their own file made `Sort.natural` raise
  `undefined_word: natural-go`. A planned `then`-combinator was **dropped**
  because it would capture two functions, one of them cross-module.
- **`graph`** (the new pathfinding library) has had to specify that
  user-supplied **heuristics must be self-contained** — parameters and
  builtins only — which caps how much abstraction a caller can build
  around an A\* query.
- Generalised: **every callback API in boru inherits the constraint** —
  comparators, predicates, visitors, codecs, service handlers, TUI
  update/view. The library author cannot offer "pass me a function"
  without also saying "…but it may not call your own helpers."

That is a language-level tax on the exact abstraction — higher-order
functions across module boundaries — that first-class function values
exist to provide.

## 7. The fix

### 7.1 The option space

| Option | Mechanism | Verdict |
|---|---|---|
| **(a) evaluate in the defining module's registry** — wire `fnDef.Registry` through every path | Python `__globals__`; Lua `_ENV`; OCaml closures | **Ship this.** The field exists, is already set, and two of three classes already honour it |
| (b) capture module-level free words at construction | eager snapshot | **Reject.** Freezes the binding — kills redefinition, forward refs and hot reload; and a captured fn whose own body has free words just relocates the problem |
| (c) resolve word tokens to word objects at parse time | Factor / Forth XT / CL symbol cell | **Best endgame**, after (a). Name early-bound, definition late-bound; removes a per-call lookup |
| (d) caller-first, defining-module fallback | two-stage lookup | **Reject as semantics.** Preserves the capture bug and converts errors into silent wrong answers. Viable only as a temporary warn-and-continue instrument |
| (e) do nothing; export the helpers | documentation | **Reject.** Makes private helpers public — i.e. deletes the module boundary |

### 7.2 The semantic rule to adopt

> **A function value's free words resolve in the module that defined it,
> at the time it is called.**

Two halves, both load-bearing:

- **Lexical *module*** — *which* namespace is fixed at definition. This is
  what makes cross-module higher-order code work, and it is what the
  compiler already does.
- **Dynamic *within* that module** — *which definition* is found is a live
  lookup, not a snapshot. This preserves the documented module-level
  dynamic semantics (`lang/go/CLAUDE.md`: `def x 1; def f …; def x 2; f 0`
  → `2`), recursion via forward reference, and — critically —
  **hot reloading** (`HOT-CODE-LOADING.0.md`), which depends on a
  re-imported module's new definitions being seen by existing values.

Python is the exact precedent: a function object holds `__globals__`, a
reference to its **defining** module's dict, and `LOAD_GLOBAL` is a
lookup *in that dict at call time*. `importlib.reload` re-executes the
module into the *same* dict, so pre-existing function values see the new
helpers. That last detail is worth copying exactly — a reload that
*replaced* the registry would orphan every function value that pointed at
the old one, which is a constraint this design places on the `reload`
word proposed in `HOT-CODE-LOADING.0.md` §5.

The resulting asymmetry must be stated in the spec, not left implicit:
**module fns are lexically scoped; a fn defined in the running scope
continues to see that scope.** `t3.aql` is the discriminating case, and
the rule says the answer is `6`.

### 7.3 What changes

1. **`InvokeCallback` must take the `FnDefInfo`** (or the fn `Value`),
   not bare `(sig, captures)` — `core/go/invoke.go:65`. Its own doc
   already argues it is the right seam: *"the single seam every native
   callback word … dispatches through, so retiring the interpreter for
   reducible callback bodies is one routing decision rather than an edit
   per word."* Fixing it fixes the whole native-callback class at once:
   `filter.go:154`, `native_map_iter.go:110`, `walk.go:83`,
   `walk_core.go:165`, `io_mount.go:73`, `native_service.go:419,423`,
   `net_codec.go:100`, `net_socket.go:599`, `model.go:403`,
   `tui_run.go:375,418`, `registry.go:1895`.
2. **`buildFnBodyHandler` must prefer `fnDefCopy.Registry`** when it is
   non-nil and differs from the install registry
   (`core/go/core_helpers.go:678`). The narrower alternative is to widen
   `installDef`'s existing foreign-registry branch (`:80-115`) beyond
   trivial delegations, so a foreign boru-bodied fn keeps its *original*
   signatures — whose handlers are already closed over the right
   registry.
3. **`analysisFnConstructionPass` must be routed to the same registry**
   (`core/go/core_helpers.go:626`), or check keeps reporting
   `undefined_word` for code that now runs correctly — the `t9.aql`
   error above is exactly that pass.
4. **The compiler needs no separate edit** for name dispatch:
   `CompiledFn.Reg` is stamped from the registry the analysis pass ran
   the body in, so the compiled path follows automatically.

**Two further changes the audit found are required, which this plan
originally missed** (§10.4). Threading `FnDefInfo` is necessary but not
sufficient:

5. **`InvokeCompiled`'s freshness check must be re-anchored to the
   defining registry.** `core/go/invoke.go:79` validates
   `CompiledFnRef.depsFresh` against the *invoking* registry, which is
   why adding an unrelated `def` in another module can flip the answer.
   Threading `fnDef` fixes the interpreter path but leaves this
   check meaningless unless it moves too.
6. **The two competing stamp registries must agree.**
   `serviceAddHandler` (`native_service.go:249`) and `resolveCodec`
   (`net_codec.go:161-162`) stamp on the **running** registry, while
   `native_module_module.go:369` stamps on the **module** registry.
   After the fix these disagree, and the same source yields two scopes
   depending on which stamp won.

**Cost, measured:** 14 mechanical call-site edits. The audit checked the
warning below and **it does not materialise** — every one of the 14 sites
already type-asserts the `FnDefInfo` to reach `.Captured` and simply
discards the rest, so no site is blocked on having only a `*Signature`.

### 7.4 Hazards

- **This is a scoping-model change, not a local bug fix.** `installDef`'s
  comment states the current model plainly: *"A name bound as a local
  inside fn F is visible — via boru's dynamic scoping — to any fn F
  reaches on the call stack."* The change makes that false across module
  boundaries. It needs a spec row and a `lang/spec/*.tsv` battery, not
  just a patch.
- **Programs may depend on the current behaviour**, deliberately or
  accidentally. A caller that shadows a library's helper to influence it
  is relying on today's semantics; after the fix it silently stops
  working. §8 phases for this.
- **Check-mode and compiler invariants key on one `DefTable`** —
  `NoteFrozenRead`, `NotifyNameRebound`, and `CompiledFnRef` dep
  snapshots all snapshot a single registry's `Defs`. Option (d)'s
  two-registry cascade would make the freshness test unsound; option (a)
  does not, because each value still has exactly one resolution scope.
- **Performance.** Option (a) adds a pointer per value and a frame field
  per call; per-name lookup cost is unchanged. Option (b) would instead
  make every call O(module size) — another reason to reject it.

## 8. Phasing

Modelled on the Emacs lexical-binding migration, which is the closest
real precedent for flipping a scoping default in a live ecosystem:

- **Phase 0** — no semantic change. Make the divergence *visible*: a
  differential spec battery pinning `t3.aql`-shaped programs across
  interpreter / check / compiler, so the current disagreement is a
  recorded failure rather than an invisible one.
- **Phase 1** — fix the **native-callback seam** (§7.3 items 1, 5, 6)
  first: it is the class the library ecosystem actually hits, and the
  audit measured its migration exposure at **zero**. It is *not*,
  however, the "pure bug fix with nothing to migrate" this plan
  originally assumed — group 2 already diverges (§2), so the seam fix
  must land **together with** the `depsFresh` re-anchoring and the
  stamp-registry reconciliation, or it will repair the interpreter
  path while leaving the VM path arriving at the right answer for
  the wrong reason.
- **Phase 2** — fix **name dispatch** (items 2–3). This is the one that
  changes interpreter semantics; land it with the spec rows and a
  `boru check` diagnostic that reports every free word which *would*
  resolve differently under the new rule. That diagnostic is the entire
  migration tool.
- **Phase 3** — if a deliberate dynamic-evaluation idiom turns out to be
  needed, give it a *name* (`with-registry`-shaped) before the accidental
  one becomes an error. Do not leave dynamic evaluation reachable only by
  accident.
- **Phase 4 (optional)** — given (a), parse-time word resolution (c) is a
  representation change rather than a semantic one: take it for the
  speedup and for parse-time unresolved-name errors.

Two invariants throughout: **capture bindings, never values** (this is
what keeps redefinition and hot reload working), and **make the
deliberate-dynamic case visible before making the accidental-dynamic case
an error**.

## 9. Open questions

1. **Should a fn defined at top level and passed *into* a module get the
   top-level scope?** The rule in §7.2 says yes (its defining module is
   the main program), which fixes the `sort` comparator case — a user
   comparator would reach the user's own helpers. Confirm this is
   intended, because it is the change users will notice most.
2. **What is a "module" for a `Vm.run` sub-engine or a spawned process
   fork?** Both create registries; is the defining scope the fork or its
   parent? (Leaning: the fork inherits the parent's `Defs` clone, so the
   value's `Registry` pointer stays valid — but this needs a spec row.)
3. **Does the `reload` word (`HOT-CODE-LOADING.0.md` §5.1) mutate the
   module registry in place, or replace it?** §7.2 says it must mutate:
   replacing orphans every function value already handed out. This
   constrains that design and should be recorded there too.
4. ~~**Is the current behaviour load-bearing anywhere in-tree?**~~
   **ANSWERED — no.** Measured tree-wide in §10: **0 migration sites and
   0 silent-change sites** across the language repo and all seven library
   repos, against **224+ sites that the fix repairs**. The migration cost
   is nil; the reason it is nil is that the ecosystem already worked
   around the defect, in writing (§10.2).

## 10. Migration audit

Tree-wide, over `/home/user/boru` and the seven sibling library repos.
Each site classified: **A** the fix repairs it · **B** the fix breaks it
(migration) · **C** the fix silently changes its value · **D**
unaffected.

### 10.1 The numbers

| | A (repaired) | B (migration) | C (silent change) | unauditable |
|---|---|---|---|---|
| Library repos (7) | **224** | **0** | **0** | 16 files + 2 probe shapes |
| boru repo | 3 + 12 worked-around | **0** | **0** | — |

**The migration cost is zero.** No function value anywhere in the tree
resolves a free word that exists only in the module that runs it, and no
module pair that exchanges function values shares a module-level name.

Two caveats keep this honest. **B and C are not unreachable** — both were
reproduced against `sort`'s shipped, documented comparator API. The class-C
repro is the one to look at: with `by-string` defined in both the caller
and `sort.aql`, the same program returns

```
--no-compile    → ["Apple", "fig", "pear"]     (ascending)
--force-compile → ["pear", "fig", "Apple"]     (descending)
```

— opposite orderings, exit 0, no diagnostic on either engine. And the
**collision scan found one near-miss**: `trie.aql`/`radix.aql`/`tst.aql`/
`burst.aql` share 28–33 module-level names pairwise. They are class D only
because the variants exchange Lists, never function values. Add one
function-valued word to that API and the tree acquires its first class-C
site.

### 10.2 Why the cost is zero: the ecosystem already paid it

The zero is not evidence the defect is harmless. It is evidence that
every library author hit it and worked around it, and three of them wrote
down why:

- **`boru/utils/*.boru` — 12 CLI tools, and not one calls `Cli.main`.**
  Each unrolls it into `Cli.dispatch` plus three statements, naming this
  defect. `cat.boru:209-217` also prices it: routing through `Cli.main`
  forces interpreter mode, costing **12× on the hot path** (8.6 s vs
  0.72 s over 8,000 lines, measured).
- **`aless-app.aql:432-435`** — the `Aless.feed` wrapper exists solely
  because "an `/r`-parked update fn called from another module cannot
  resolve this module's private words; a module wrapper can."
- **`sort.aql` is one 1,490-line file because of this**, with 34
  `Function` params and 11 exports needing privates. `CLAUDE.md` states
  it as a structural rule. After the fix, all 34 seams become safe and
  **the file can finally be split** — the single largest win in the
  corpus.

Every comparator example in every `AGENTS.md` is written with a
builtin-only body. That is not style; it is the shape that dodges the
defect.

### 10.3 The guards never could have caught it

Seven of nine repos ship a divergence runner whose job is exactly
interpreter-vs-compiler comparison. **All of them are structurally
blind**, for one reason:

```sh
interp="$("$AQL" "$s" 2>&1)"      # sort/test/divergence/run.sh:76 — and 5 siblings
```

No flag means the **default** mode, which prefers the bytecode compiler.
The column labelled `INTERPRETER` is running the compiler, so the gate
compares bytecode against bytecode. Proof on the one file that actually
diverges:

```
boru              test/sort_unit_test.aql  → all green   exit 0
boru --no-compile test/sort_unit_test.aql  → FAIL reverse …  exit 1
```

The runner prints `INTERPRETER ok` for a file the interpreter fails.
**Fix: add `--no-compile` to the interpreter leg — one line per repo.**
This should land regardless of anything else in this document.

Two further blind spots bound the audit's reach: the **checker implements
the old rule**, so `--force-compile` refuses precisely the programs that
trip the defect (16 files unauditable that way); and `lang/spec/*.tsv`
has **zero rows** exercising a cross-module function value, a boru codec,
a cross-module service handler, `IO.mount` across modules, `Tui.run`'s
update/view, or `Model.*` at all. `make verify-bytecode` could not have
caught this because the corpus never poses the question.

### 10.4 Corrections to this document

The audit falsified three claims made in earlier sections, now fixed
in place: the native-callback classification (§2 — group 2 diverges
rather than being uniformly wrong), the "hard ones" warning in §7.3 (no
site is blocked on a bare `*Signature`; the fix is 14 mechanical edits),
and the characterisation of phase 1 as a pure bug fix (§8 — it must land
with two further changes).

### 10.5 Unrelated breakage found in passing

**`/home/user/aless` does not run at all.** All 14 modules import the
retired `aql:` namespace (`import "aql:io"` → `module "aql:io" not
found`); the modules now live under `boru:`. A `sed 's/"aql:/"boru:/g'`
port runs clean, so it is a pure rename. Not this defect, but it means
the repo has been dead since the namespace change and nothing reported it.

## 11. Target semantics

The rule this document recommends is one clause of a three-clause model.
Stating all three together matters, because users do not experience them
separately — they experience one expectation, which every mainstream
language meets:

> **A function value means the same thing wherever it goes.**

1. **Free names resolve where the function was written** — lexically by
   module, dynamically in time. This document. boru already has Python's
   model (local → captured → module globals → builtins) and differs in
   one respect: its "module globals" is whichever module is running.
   `FnDefInfo.Registry` is the missing `__globals__` pointer, already
   set. This is completing a design, not replacing one.
2. **Captures should be bindings, not values.** Today `Captured` holds a
   shallow snapshot, so the universal counter idiom silently does not
   work — the closure sees the value at construction forever, unless the
   payload happens to be pointer-backed, which makes the rule invisible
   and type-dependent. Python, JS, Lua, Scheme and Ruby all capture the
   *cell*. This is also the invariant hot reload needs
   (`HOT-CODE-LOADING.0.md`): capture bindings and redefinition stays
   visible.
3. **A function value is data until something applies it.** The `sort`
   combinator fault (§5) is this rule breaking: `Sort.by-number` is a
   value in one argument position and an application in another. In a
   concatenative language a bare word *is* application, so `/r` is right
   and worth keeping — the defect is the inconsistency. A parameter slot
   typed `Function` should never auto-apply its argument.

The spec sentence:

> A function value carries the scope it was written in. Its free names
> are looked up in its **defining** module, at **call** time; names it
> captured from an enclosing function are shared **bindings**, not
> copies; and a function value is **data** until something applies it.

Rules 1 and 3 are live defects with reproductions. Rule 2 is latent —
nobody has filed it because the `flex`-cell workaround is discoverable —
and it will surface the first time someone ports a closure-heavy design.

## Appendix — how other languages bind free names in a function value

| Language | Representation | Free-name lookup | Redefinition-friendly |
|---|---|---|---|
| **Python** | code + `__globals__` + `__closure__` | local → enclosing cell → **defining module dict** → builtins | yes — the dict is mutable and shared with `reload` |
| **Lua ≥ 5.2** | proto + upvalues incl. `_ENV` | `x` compiles to `_ENV.x`; `_ENV` is a lexically captured upvalue | yes |
| **Scheme / OCaml / Haskell** | closure = code + heap environment | lexical, by construction | n/a (immutable bindings) |
| **Common Lisp** | code + lexical env; named fns via the symbol's **function cell** | lexical for variables; global cell for functions | yes — cell is mutable |
| **Erlang** | fun + captured environment | lexical; module-local funs tie to the module version | via the two-generation module rule |
| **Factor / Forth** | quotation of **word objects** | resolved at parse time to a word owning a mutable definition slot | yes — name early-bound, definition late-bound |
| **boru today** | `FnDefInfo` + `Registry` + `Captured` | **defining module on some paths, running module on others** | — |

The unifying observation is Moses's (1970): a function value is
meaningless without its environment, so the environment must be *part of
the value*. boru already made that decision — `FnDefInfo.Registry` is the
environment pointer. What remains is to consult it everywhere.

> **Superseded in part, 2026-08-17.** §12.4 and §12.6's open-work
> conclusions were re-measured against `8732662` and **three of their
> figures were wrong** — the clause-3 row count, the clause-2 bare-name
> count, and the claim that narrow and broad are inseparable (they are
> separable, by `FnDefInfo.Name`). The current account of what remains,
> with corrected numbers and a per-item blocker, is
> [FN-VALUE-OPEN-WORK.0.md](FN-VALUE-OPEN-WORK.0.md). This section stays
> as the record of how the landed work was reasoned about; read the newer
> note before starting anything from it.

## 12. Implementation log — 2026-08-15

Rule 1 of §11 ("free names resolve where the function was written") is
implemented across every path §4.2 listed as not honouring
`fnDef.Registry`. Rules 2 and 3 are **not** implemented; §12.4 records
where each one actually lives, so the next pass starts from a located
mechanism rather than re-deriving one.

### 12.1 The seam

Three functions in `core/go/invoke.go` now answer the scope question once
for every caller:

- **`FnHome(r, fnDef) (*Registry, []CapturedBinding)`** — the whole rule
  in three lines: the defining registry when the value carries one, the
  caller's otherwise. `Registry` is set only at module-export resolution
  (§4.1), so a fn defined in the running scope takes the same path it
  always did and the common case is byte-identical.
- **`InvokeCallbackFn(r, fnDef, sig, args)`** — `InvokeCallback` with the
  definition in hand. Also re-anchors `InvokeCompiled`'s `depsFresh`
  check (§7.3 item 5), which was previously evaluated against a registry
  nobody had asked about.
- ~~**`CallBoruFn(r, fnDef, sig, args)`** — the interpreter-only
  sibling.~~ **RETIRED 2026-08-28.** It carried the same
  defining-registry routing but kept the call on `CallBoru`, because six
  words (`filter`, the map-lambda `each`/`fold` bodies, core `walk`,
  `StructUtil.walk`, `IO.mount`, `boru:parse`) already choose between a
  compiled closure and an interpreter `FnDefInfo` themselves, and routing
  their `FnDefInfo` half through `InvokeCallback` would silently move
  those bodies onto the VM: *fixing where free words resolve must not
  also change which engine resolves them.* That was the right fence for
  THIS document's change and the wrong one for full compilation, which
  wants exactly that move. All seven sites (those six plus the fn-util
  words) now call `InvokeCallbackFn`, and the function is deleted rather
  than left as a hatch — a second dispatch path is a divergence source in
  its own right (`design/FN-VALUE-OPEN-WORK.0.md` records two it caused).
  The routing rule this document establishes is unchanged; only the
  engine choice moved.

### 12.2 What changed, by class

| Class | Sites | Change |
|---|---|---|
| Native callbacks (§7.3 item 1) | `filter.go`, `native_map_iter.go`, `walk.go`, `walk_core.go`, `io_mount.go`, `parse.go`, `native_service.go` ×2, `net_codec.go`, `model.go`, `tui_run.go` ×2, `registry.go` (`RunPredicate`) | route through the seam |
| `serve-raw` (§7.3 item 1, deferred as a concurrency hazard) | `net_socket.go` | the acceptor forks the **defining** registry; per-connection `ForkConcurrent` keeps the goroutines isolated, so the module is never shared. Writers stay the caller's. |
| Name dispatch (§7.3 items 2–3) | `core/go/core_helpers.go` — `compileFnSigs`, `InstallFnDef` | the body handler and the construction-time analysis pass are both built against the defining registry, so `def g A.pub` answers exactly as `A.pub` |
| Stamp reconciliation (§7.3 item 6) | `compiler/go/stamp_runtime.go` — `StampFnValue`, `StampFnValueInPlace` | a stored handler is stamped where it is **stored** but runs where it was **written**; the detached compile now uses the defining registry, so the VM unit and the interpreter path cannot disagree |
| Closure lowering (**not** in the original plan) | `compiler/go/callable_words.go` — `foreignFnHome` | see §12.3 |

### 12.3 One change the plan missed, found by testing

§7.3 item 4 claimed *"the compiler needs no separate edit."* It does.

The compiler lowers a higher-order word's fn operand to a **closure unit
compiled against the calling module** (`tryRecordLambdaClosure`). So with
the seam fixed and nothing else, `filter A.big xs` returned the defining
module's answer interpreted and the calling module's answer compiled —
trading a wrong value for a *mode-dependent* one, which is worse.

`foreignFnHome` declines the lowering when the fn value carries a foreign
`Registry`. The decline falls through to the runtime callback path, which
now runs the body on its own registry, so the engines agree. Declining
costs the closure fast path on a cross-module callback and nothing at all
on the same-module one.

**That decline is a defect, not a resting place.** A cross-module callback
is valid boru, so it is owed a compile; the decline is an unimplemented
case, tracked to closure immediately below, and it is not a sanctioned
outcome of this design. Under `--force-compile` (the strict mode) the gap
is at least visible instead of a silent miscompile: `walk` with a
cross-module hook now says *"function value reaches walk (Stage 3)"*
instead of returning the wrong answer. In default mode the runtime
*silently* routes the refused body to the interpreter: machinery that
absorbs the defect and hands back the right value while nothing in the run
says a compile was refused. That measures the debt, it does not discharge
it, and the silence is why the debt goes unnoticed.

**The decline has a measured cost, and it is on the record.** `filter` with
a cross-module predicate now compiles with one interpreter ISLAND
(`FALLBACK b0 ; filter`) instead of a native closure. The main corpus
ratchets islands to zero (`compiled_coverage_test.go`'s `islandGate`), so
the row lives in `lang/spec/frontier/frontier-fnvalue-scope.tsv` — the
frontier corpus, which `computeCensus` skips structurally because it is a
subdirectory — with its graduation criterion pinned in
`frontier_spec_test.go`'s ledger under `failsWith: "islanded"`. Its
SEMANTICS stay pinned on both engines by
`lang/go/function_value_scope_test.go`; only its compiled *disposition*
moved out of the ratchet.

**What graduating it actually takes.** An earlier draft of this section
called the real fix "its own const pool and stamp registry". That was
imprecise on both counts, and the correction is worth recording because it
makes the work smaller than it sounded:

- The **const pool is per-`Program`, not per-registry** (`emit.go:714`), so
  no split is needed; the **type** pool is already resolved `curReg`-first
  at run time (`vm.go:1583`).
- The **runtime half already ships**: `CompiledFn.Reg` (`bytecode.go:970`)
  plus `enterUnit`'s `if p.Fns[u].Reg != nil { curReg = p.Fns[u].Reg }`
  (`vm.go:1403`) already give a closure unit its own dispatch registry, and
  `StartFnCompile`'s parameter is already named `fnReg` (`emit.go:3314`).
- Of the four compile-side roles `r` plays in `recordClosureDispatch`,
  three are **solved patterns the named foreign-fn path already uses**:
  `shareCheckStateFrom` (`check/go/check_recovery.go:527`) for the
  `CheckState`, and `fd.Registry.AnalysisScopeID()` for the memo key —
  exactly what `check/go/check_fnbody.go:312,513` does.
- The **one unsolved role is capture operands.**
  `recordClosureDispatch` resolves them in the CALLER's emit tables
  (`callable_words.go:474`), and `dynScopeRescue`'s fallback re-resolves
  them at run time against the caller's `curReg` (`vm.go:1902`), so a
  foreign module-scope capture has no operand home. Closing it needs a
  registry-tagged dyn-scope operand so `OpLookupDynScope` can name
  `fd.Registry`; declining foreign closures that carry non-lexical
  captures only moves the defect into the ledger, so it is a stopgap and
  not a close. A second, smaller asymmetry rides along: `enterBodyUnit`
  brackets contexts on the CALLING registry (`vm.go:294`) while `curReg`
  would be `fd.Registry`.

Note this also revises §7.4's blanket hazard. The freeze/rebind ledger
keying on one `DefTable` is a real constraint, but `shareCheckStateFrom`
is the existing answer to it — the named foreign-fn path has been living
with it since the M1 wave.

### 12.4 Not implemented

- **§11 rule 2 — captures as bindings, not values.** Still a shallow
  snapshot, and the representation swap is necessary but **nowhere near
  sufficient**. Two blockers sit upstream of it, both verified in source,
  and neither was visible when rule 2 was written:

  **(a) `def` is a push, not a mutation, so a cell has nothing to
  observe.** `DefTable.Push` *appends* a `DefEntry`
  (`core/go/deftable.go:96-102`); `def` never overwrites, and frame
  teardown pops by depth. A `*Cell` bound to the entry live at
  construction would therefore still miss a later `def n 2`, which
  pushes a *new* entry the cell does not point at. Cells buy nothing
  until `def` of an already-frame-bound name becomes write-through —
  which collides with `InstallFrameBinding`'s deliberate shadowing
  (`core/go/core_helpers.go:37-39`), whose reason is a fixed correctness
  bug (`design/ACCESSOR-SPLIT-AND-CLEANUP-BUG.md`).

  **(b) The counter idiom's closure never captures the name at all.**
  Rule 2 justifies itself with "the universal counter idiom silently does
  not work". But in `def n 0` + `([] => [def n (n add 1) n])`, the body
  `def`s `n`, so `CollectBodyLocalDefs` marks it a body-local
  (`core/go/fn_capture.go:373-380`) and `ComputeCaptures` skips it
  (`:334-337`). **`n` is not in `Captured` under any representation.**
  Making that idiom work needs a `nonlocal`/`set!` distinction in the
  surface language — new syntax, a spec change, and a checker story —
  before the capture representation matters at all.

  So landing only the swap is a pure-cost change: it breaks three
  cross-module seam signatures (`core/go/emit_recorder.go`,
  `dispatch_slots.go`, `analysis_hooks.go`, each with an `inactive*` stub
  that becomes a fresh ADR-008 obligation) and the compiled trailing-slot
  path (`CompiledFn.NCaptures`, `OpPushClosure`'s `copy`,
  `bindUnitLocals`), falsifies three explicit soundness comments that
  assert the snapshot rule, and unsettles `FnAnalysisKey`'s memo, which
  keys on the capture's *content type* — while moving zero user-visible
  behaviour. That is the definition of a half-change, and it is why this
  stays design-only.
- **§11 rule 3 — a `Function`-typed slot must not auto-apply.**
  **Retraction: an earlier revision of this section described the wrong
  mechanism, and rule 3 as *worded* is already satisfied.**

  It said "forward collection has no channel telling the stepper that the
  slot it is filling is `Function`-typed". There is one:
  `hasPendingForwardExpectingFunction` (`core/go/engine.go`) walks back to
  the pending forward and reads `SigArgType(fwd.Sig, nextIdx)`, and
  `stepWord`'s `TFunction` intercept turns the word into a reference
  instead of executing it. A `f:Function` parameter binds its argument as
  a value, correctly, on both engines.

  **The live defect is one level in: reading that bound param inside the
  body where no argument follows it.** Measured:

  | body shape | `--no-compile` | `--force-compile` | `boru check` |
  |---|---|---|---|
  | arity 0, no args — `[f]` | `Integer` (**applies**, → 7) | `Function` | 0 errors |
  | arity ≥1, args present — `[f n]` | `10` | `10` | 0 errors |
  | arity ≥1, no args — `[c]` | raises `signature_error` | `Function` | 0 errors |

  Rows 1 and 3 are **check-clean compile ≠ interpret divergences**, and
  row 1 is a silent wrong *value* in default mode — the class
  `bytecode_stored_handler_freeze_test.go` gates as MISCOMPILE. My
  original repro (`typeof (grab nought)` → `Integer`) is row 1; I had
  attributed it to the slot, and it is the deref.

  **Neither engine is simply wrong**, which is why this is not a bug fix.
  The interpreter's model is *a bare name is a call* — spec'd by
  `path-modifier.tsv` ("a bare fn name remains a call/barrier") and
  `fn-value.tsv` ("with no argument the 0-arg signature still answers"),
  and conceded by rule 3 itself: "in a concatenative language a bare word
  *is* application, so `/r` is right and worth keeping". The compiler's
  model is *a param is a value slot* — lowered to a unit local
  (`RegisterLocal`), never a dispatchable name. Row 2 is where the two
  models agree, and it is pinned green five times over
  (`fn-value.tsv:135-137`, `module-fnvalue-boundary.tsv:51`,
  `path-modifier.tsv:62-67`, `frontier-chained-apply.tsv:25`,
  `module-parselang.tsv:120`) — so any fix must preserve it.

  Both rows GRADUATED 2026-09-05 (NUR123: the compiled lane re-steps a bare
  read of a fn-valued frame binding as the WORD, `CompiledFn.DynFrameWords`)
  and live in `lang/spec/fn-value.tsv` §7; the frontier file is retired.

  **RULED, 2026-08-15 (maintainer): a bare name is a CALL; `/r` is how you
  ask for the value. Arity discrimination is explicitly rejected**, and
  the general form is now **ADR-016** — arity and origin never change how
  a function behaves. That
  settles both divergent rows without a new rule, and it makes the
  interpreter the correct engine in each:

  | shape | correct behaviour | interpreter | compiler |
  |---|---|---|---|
  | `[f]`, arity 0 | apply | ✅ `Integer` | ❌ `Function` |
  | `[c]`, arity ≥1 | raise `signature_error` | ✅ raises | ❌ `Function` |
  | `[c/r]`, arity ≥1 | the value | ✅ `Function` | ✅ `Function` |
  | `[f/r]`, arity 0 | the value | ❌ `Integer` | ✅ `Function` |

  So this is not a semantic choice at all — it is **two ordinary bugs**,
  one per engine, and the last row is the one that matters most, because
  it is the prescribed workaround failing:

  1. **The compiler must treat a bare Function-bound name as a call.** It
     lowers the param to a unit local (`RegisterLocal`) and reads it as a
     value, so it neither applies nor raises. Rows 1 and 2 above.
  2. **`/r` must park a 0-arg fn bound to a PARAM.** Row 4. The scope is
     narrow and worth stating exactly, because the obvious general claim
     is wrong — measured:

     | shape | interp | compiled |
     |---|---|---|
     | `typeof nought/r` (module level) | `Function` | `Function` |
     | `[zero/r]` (list, REFERENCE.md's own example) | `[fn zero]` | `[fn zero]` |
     | `[[f/r]]` — 0-arg param inside a list | `[fn f]` | `[fn zero]` |
     | `[c/r]` — arity ≥1 param, body residual | `Function` | `Function` |
     | **`[f/r]` — 0-arg param, body residual** | **`7`** | `Function` |

     So `/r` parks correctly at module level, inside a list, and at arity
     ≥1. It fails in exactly one place: a **0-arg** Function param that is
     the body's **residual**. The likely mechanism is the dispatch-mod
     marker being dropped at the body tail before it can park the value
     (`stepLiteral` drops an unconsumed marker as a no-op) — but that is a
     hypothesis, not a measurement, and should be confirmed before fixing.

     `REFERENCE.md:2239` already promises the behaviour this breaks: *"The
     reference holds at any arity and in any position … `[zero/r]` is
     `[<function>]` even for a 0-arg `zero` (it is **not** fired)."* So
     this is a documented-contract violation as well as an ADR-016 one.

  Bug 2 is the sharp one: the documented answer to "how do I pass a
  function as a value" silently does the opposite in one shape, and it
  fails *quietly* — `f/r` yields `7` where the author asked for the
  function.

  **Surveyed 2026-08-16, and it is a family rather than one shape.** The
  full matrix (interpreter | compiled):

  | shape | interp | compiled | |
  |---|---|---|---|
  | `typeof nought/r` (module, 0-arg) | `Function` | `Function` | ok |
  | `typeof one/r` (module, 1-arg) | `Function` | `Function` | ok |
  | `[nought/r]` (list literal) | `[fn nought]` | `[fn nought]` | ok |
  | `typeof N.z/r` (namespace, 0-arg) | `Function` | `Function` | ok |
  | `typeof N.o/r` (namespace, 1-arg) | `Function` | `Function` | ok |
  | `[c/r]` (body residual, 1-arg param) | `Function` | `Function` | ok |
  | `{k: one/r}` → `(m.k)` (1-arg) | `Function` | `Function` | ok |
  | **`[f/r]`** (body residual, **0-arg** param) | **`7`** | `Function` | interp wrong |
  | **`{k: nought/r}` → `(m.k)`** (**0-arg**) | **`7`** | *refuses* | interp wrong |
  | `[[f/r]]` (0-arg param in a list) | `[fn f]` | `[fn nought]` | **name diverges** |

  **RULED (maintainer, 2026-08-16), superseding an earlier reading of this
  section: `/r` IS NOT STICKY.**

  > `name/r` is sugar for `ref name`. It deactivates exactly **one** use —
  > the one it is attached to. Any **subsequent** use of the value, in any
  > context, is function **invocation**. To deactivate again you write
  > `/r` (or `ref`) again. **The same rule applies at every arity.**

  This is a deliberate boru difference from languages where a bare name is
  the reference and parentheses mean invocation. boru has no
  parens-for-invocation, so a bare name **is** invocation and `/r` is the
  opt-out. An earlier revision of this section recorded a "store-time `/r`
  persists" reading; that is withdrawn. It would have made every
  `export "M" {w: w/r}` inert and stranded the argument in `M.inc 5` — 81
  spec rows use that idiom — and it contradicted `lang/spec/ref.tsv:8-15`,
  which already states the rule correctly:

  > "The SAME value dispatches normally when it is re-stepped elsewhere:
  > unwrapped from a paren, retrieved from a map slot, or handed to
  > `apply`."

  Under the rule each break has a **different** correct engine, which is
  why they are two fixes rather than one:

  | break | shape | correct | to fix |
  |---|---|---|---|
  | 1 | `[f/r]` body residual | **compiled** (`Function`) — the `/r` is at the use, so h returns the value | **interpreter** |
  | 2 | `{k: z/r}` → `(m.k)` | **interpreter** (`7`) — the `/r` parked the *store*; the read is a fresh use, so it invokes | **compiler** |

  For break 1 the `/r` is doing its job: measured, `[f/r ; 5]` does **not**
  fire at the word step where `[f ; 5]` does. The call comes later, at the
  **fn-frame collapse**, which re-steps the body's residual after the
  marker is gone. A fn body's residual is a *return value*, not a fresh
  use, so that re-step must not invoke it — while a **user** paren re-step
  must still fire (`lang/spec/ref.tsv:26` pins `(z/r)` → `42`).

  For break 2 the compiler refuses (*"fn value read from a container
  auto-dispatches (Stage 3)"*) where the interpreter correctly invokes.
  Closing that refusal is the fix.

  **RULED in full (maintainer, 2026-08-16).** Four clauses, which together
  redefine how a function value is held and applied. Each was measured
  against the engine before being written down here.

  1. **`/r` is sugar for `ref name` and deactivates ONE use.** Any
     subsequent use, in any context, is invocation. Same at every arity.
  2. **Passing a function as an argument requires `/r`.** It is what stops
     the function firing in place.
  3. **Parens do not re-step.** They place a value (or values) on the
     forward stack. So `(ref z)` ≡ `ref z` and `(z/r)` ≡ `z/r`.
  4. **`inc/r 5` is TWO VALUES**, not a call. Applying a held reference is
     what `apply` is for.

  ### What that changes, measured

  Clause 3 is narrower than it sounds — an ordinary paren is untouched
  (`(1 add 2)` → `3`). Only a paren whose *result* is a Function moves,
  because only that result was being re-dispatched:

  | input | today | ruled |
  |---|---|---|
  | `z/r` | `fn z` | unchanged |
  | `(z/r)` | **`42`** | `fn z` |
  | `ref z` | `fn z` | unchanged |
  | `(ref z)` | **`42`** | `fn z` |
  | `inc/r 5` | `fn inc(Integer) 5` | unchanged |
  | `(inc/r) 5` | **`6`** | `fn inc(Integer) 5` |

  **`lang/spec/ref.tsv` §2 is void.** Its heading — *"The held value
  dispatches when RE-STEPPED (paren unwrap)"* — is the thing being
  withdrawn. Rows 25-30 and row 43 change; the header contract at lines
  11-12 loses the clause "unwrapped from a paren".

  The rest of that file **survives**, which is the check that the rulings
  are consistent rather than merely decisive:

  - **§3** (map slots, rows 33-34) — a fresh use invokes. Matches the
    break-2 ruling exactly.
  - **§4** (`apply`, 37-39) — explicit application, and by clause 4 it
    becomes the *only* way to apply a held reference.
  - **§5** lambdas, **§6** negatives — unaffected.
  - **§7** (58-59) — pins `typeof inc/r` ≡ `typeof (inc/r)`, both
    `Function`. **That is clause 3 already holding in one corner**, and
    it is the strongest existing evidence for the ruling.

  ### ADR-011 needs an amendment — flagged, not made

  > **Made, 2026-08-17.** The maintainer instructed the amendment: the
  > fourth clause is struck in `ADR.md`, and clause 2 is to be
  > implemented — with all four engine sites retiring together (the
  > NUR038 call-head barrier included, re-opening that question inside
  > the implementing PR). The same rulings set clause 3 to **broad** and
  > NUR077 to a **dedicated Apply Op** — see
  > `design/FN-VALUE-OPEN-WORK.0.md` §1.1, the current account.

  Clause 2 supersedes ADR-011's final sentence:

  > "…`/r` takes the reference and is no collection barrier; **a bare fn
  > name before a `Function`-typed slot resolves as a reference.**"

  That clause is why `h zero` and `h zero/r` are indistinguishable today
  — measured: with a `Function`-typed slot both collect `zero` as a
  value, while with an `Any` slot the bare name **invokes**
  (*"h is still waiting for 2 argument(s) when `zero` begins its own
  dispatch"*). So the slot type, not `/r`, decides — which is precisely
  what clause 2 rejects.

  ADR entries are added and amended only on explicit maintainer
  instruction, so this is recorded here rather than edited into `ADR.md`.

  ### Blast radius

  - `lang/spec/ref.tsv`: 7 rows (§2 plus row 43) + the header contract.
  - `:Function` params across the whole spec corpus: **20 rows, 9 of which
    pass a bare name** and would need `/r` under clause 2.
  - Ecosystem: every callback API. `sort`'s `AGENTS.md` already tells users
    to write `/r` (*"A bare own-word comparator auto-invokes; `/r` passes
    it as a value"*), so the documentation is already aligned.
  - The `unused_def` fix landed in `7e98aeb` hooks the `TFunction`
    intercept clause 2 retires, so it needs rework alongside.

  The pattern is exactly what ADR-016 forbids: a **0-arg** parked fn fires
  where a **1-arg** one parks. Worth testing before fixing whether the
  arity-≥1 park is the marker being *honoured* or merely dispatch failing
  for want of arguments — if the latter, `/r` is not honoured in these
  positions at all and the fix is larger than a 0-arg special case.

  **A third divergence, found in the same survey and previously unrecorded:
  `canon` renders a different NAME per engine.** `canon` of a `/r`-parked
  param gives `[fn f[[][Integer][7]]]` interpreted (the *binding* name) and
  `[fn nought[[][Integer][7]]]` compiled (the *defining* name) — the same
  value, different user-visible text, which ADR-015 (canon always
  round-trips) has an interest in.

  **RULED (maintainer, 2026-08-16): `canon` renders fn values with NO NAME.**
  That subsumes the divergence rather than picking a winner: with no name
  on either engine there is nothing to disagree about. The load-bearing
  check before implementing is that a nameless rendering still
  **re-parses to a `deq`-equal value**, since ADR-015 admits no exempt
  kinds.

  **This ruling is not new work — it is NUR031 becoming actionable, and
  this document's own §12 work is what unblocked it.** NUR031's refined
  verdict (2026-08-15) already states the same target and the same root
  cause:

  > the halves are one root cause (the binding name in `FnDefInfo.Name`
  > drives `eq`, canon and `tcmp` alike), so: BOX `FnDefInfo` for `eq`
  > (reference) and compare signatures+body for `deq` (NUR011 as written),
  > **canon renders the ANONYMOUS fn literal**, and **PR #366's
  > native-callback seam lands first**. Design-unblocked, sequenced

  PR #366 landed as `7e98aeb`. So NUR031's stated prerequisite is
  satisfied and its sequence says this is next. Two things follow that a
  reader should not have to rediscover:

  - **The name divergence measured above is a *symptom* of NUR031**, not a
    separate defect. `FnDefInfo.Name` driving the rendering is exactly the
    root cause NUR031 names; the interpreter shows the binding name and the
    compiler the defining name because they disagree about which `Name` the
    value carries.
  - **`eq`, `deq` and `tcmp` move with canon.** NUR031 treats them as one
    change for one reason — the same field feeds all four — so a canon-only
    fix would leave `eq` still keyed on the binding name and the record
    still open.

  **ADR-016 names a third, which this note had not caught.**
  `execFnDefLiteral` (`core/go/engine.go:5271`) gates on
  `(fnDef.Anonymous || fnDef.Macro) && fwdCount == 0 && len(positions) == 0`
  → treat as data. That keys on **origin as well as arity**: a 0-arg
  *anonymous* value alone on the stack is data, where a 0-arg *named* one
  dispatches. It is load-bearing — the comment records that it is what
  makes `def f ([] => [body])` bind the Function rather than the body's
  result — so the replacement must keep that binding working without
  consulting `Anonymous`. `lang/go/CLAUDE.md`'s "Sharp edge: 0-arg
  lambdas as values vs as calls" documents it as a design and is now
  annotated as describing a defect.

  Two earlier readings of this section were wrong and are retracted above
  and here: the slot does not auto-apply (it is the body deref), and this
  is not a maintainer decision about semantics (the rule was already
  `/r`; the engines just fail to honour it).

  One contained, decision-free bug fell out of this and **is** fixed here:
  the `TFunction` intercept resolved the name without recording a use, so
  `boru check` reported `unused_def` for every fn handed bare to a
  callback API — `Sort.quick mycmp xs`, the commonest way a boru library
  takes a function. The sibling `ResolveRef` path has recorded the use
  since the `export "X" { f: impl/r }` form was fixed; the intercept now
  matches it, noted only on a successful fn lookup so a genuinely unused
  def still warns (`lang/go/fnslot_unused_def_test.go`, both directions).

  **It is not only a toy repro — it is costing the `sort` library a green
  suite today, and §5 needs a correction because of it.** §5 called the
  combinator fault "mode-independent". That holds for the *standalone*
  shape: `[3 9 1 5] Sort.quick (Sort.by-number Sort.reverse) end` raises
  `[boru/uncalled_function]: call to 'by-number' matched no signature` at
  CHECK time, identically under `--no-compile`, `--compile` and
  `--force-compile`. But inside the test framework the same combinator is
  a **runtime divergence**: `sort_unit_test.aql` is `ok` compiled and
  `FAIL` interpreted. So "mode-independent" is true of the check-time
  error and false of the in-suite behaviour, and rule 3 owns both.

  That divergence had been invisible because every divergence runner in
  the ecosystem ran `$AQL "$s"` — the DEFAULT compile-preferring mode —
  in its column labelled INTERPRETER, and so compared bytecode against
  bytecode. Fixed in the five library repos by passing `--no-compile`;
  `sort`'s gate correctly goes red as a result. A/B'd against boru before
  and after rule 1 (`8c0df3e`/`1a4600c`): byte-identical in all three
  modes, so the failure is pre-existing and rule 1 is not implicated.

### 12.5 Evidence

- `lang/spec/module-fnvalue-boundary.tsv` §4 — four rows: applying,
  naming, a named `f:Function` parameter, and the negative (a
  module-private word stays private).
- `lang/spec/frontier/frontier-fnvalue-scope.tsv` — the fifth, the native
  callback seam, out in the frontier corpus because it islands (§12.3).
- `lang/go/function_value_scope_test.go` — the five callback classes on
  **both** engines, plus a check-clean pass (the defect was invisible to
  `boru check`; the fix must not become a false positive there).
- `lang/go/modules/net_serveraw_scope_test.go` — `serve-raw` over real
  loopback sockets, two sequential connections, verified to fail without
  the fix.
- `core/go/invoke_fnhome_test.go` — `FnHome`'s three arms.

**No pre-existing compiled row's disposition moved**, which is the
load-bearing negative result for a change that touches core dispatch. It
was measured rather than assumed: the corpus is 7366 rows / 7000 compiled
/ 0 islanded / 0 refused before these spec rows, and 7370 / 7003 / 0 / 0
after — every gate at its ratchet. An intermediate revision put all five
rows in the main corpus and read 7371 / 7004 / **1** islanded, which is
how the closure-lowering cost was found at all; moving that one row to the
frontier is what §12.3 records. `make verify-bytecode` — the whole-corpus
interpreter/compiler differential plus the race and args-aliasing gates —
passes.

The lesson worth keeping: the census, differential, refusal-ceiling and
check-accuracy gates all passed while `TestCompiledCoverage` and
`TestOnlyMetaFallsBack` did not. Running a subset of `test/go/langspec`
and generalising to "the gates" is not a verification.

### 12.6 Break 1 is fixed; clause 3 is measured and still forked

**Break 1 (`[f/r]` body residual) is implemented.** One condition and one
pointer bump in `stepCloseParen` (`core/go/engine.go`): a fn **frame**
whose collapse leaves exactly one unquoted `Function` value is delivering
a *return*, so the rewind steps past it instead of re-stepping it into a
call.

The mechanism is `ParkResult` — `ref`'s own — and the reason it is the
right one is that it is **positional**: inertness is a property of the
pointer at one index at one moment, and nothing is stamped on the value.
That is exactly clause 1 ("`/r` is not sticky"). The two value-borne
markers nearby are both wrong here: `Quoted` travels with the value and
would make it permanently inert, and `ReachGroup` is a barrier hint of
the opposite polarity.

A **user** paren still re-steps, because the two collapses are told apart
by `FrameOpenInfo` (`core/go/fn_frame.go`), which is machine-generated
only — `NewFrameOpen` / `NewFrameOpenSpan` are the sole constructors, so
no source text can forge it.

Pinned as `lang/spec/ref.tsv` §8, three rows: the 0-arg param case, a
module-scope name (proving it is not param-specific), and a 1-arg case
(ADR-016). Arguments are handed over with `/r` per clause 2. Measured on
the full corpus: `TestSpecProd` green, `TestSpecCompiledDifferential`
6219 rows compiled / **0** mismatches — no pre-existing row moved.

#### The clause-3 fork, now measured rather than posed

A fourth row was drafted — `((h z/r))` → `42`, to pin that the park is
positional and the returned reference still dispatches — and **dropped**,
because it is the one row in 6219 that diverges: interpreter `42`,
compiled `fn z`. The compiler does not re-step a paren-collapsed
`Function`; the interpreter does. That divergence is pre-existing and
unexercised anywhere else in the corpus, and it is precisely what clause
3 turns on, so pinning either answer now would pre-empt the ruling.

It has no home in either corpus, which is the point: `lang/spec/*.tsv`
asserts one outcome, and `lang/spec/frontier/*.tsv` takes the
**interpreter** as its semantics oracle (`TestFrontierSpecInterp` requires
every frontier row to pass interpreted) while ledgering only the compile
status — and here the oracle is the question, since under clause 3 the
COMPILED answer is the correct one. So it is recorded as **NUR073**
(Pending), which is what that register is for: a divergence that is never
lost or silently baselined while its verdict is open. The `ref.tsv` §8
header points at it and says not to add the row.

Implementing clause 3 was attempted and measured. **The narrow and broad
readings are not separable by any positional mechanism**, which was not
known when the fork was first posed:

- Dropping the frame-only condition (any paren collapsing to a `Function`
  parks) breaks **every namespace call** — `MathUtil.sqrt 0` — because
  dot access is lowered to `( MathUtil dot sqrt )` and its re-step *is*
  the dispatch. Excluding `ReachGroup` fixes that class entirely.
- With that exclusion the corpus reads **31 failing rows across 9 files**.
  Seven are `ref.tsv` §2 + row 43, which clause 3 voids by design. The
  rest are not: `(fn Integer [Integer] [10 add]) 7` and
  `([n:Integer] => [n add 1]) 5` — **inline application of a function
  literal** — plus the `(sub2/r) 10 3` family in `modifiers` and `usurp`.

The finding: an inline fn **literal** inside a paren and a `/r`
**reference** inside a paren reach the close paren in the *same* state —
a stepped-past `Function` at `openIdx+1`. `/r` (`stepWordRef` →
`stepLiteral`) and `ref` (`ParkResult`) both just advance the pointer.
So "only a `/r`/`ref` reference survives its paren" cannot be expressed
positionally; it needs a transient marker **on the value**, cleared by
the collapse that consumes it — which is the value-borne mechanism clause
1 rejects, and which would leak the moment the value is stored.

That leaves two implementable readings, and the choice is a language
decision, not a defect:

| | `(z/r)` / `(ref z)` | `(inc/r) 5` | `(fn …) 7`, `([n] => […]) 5` |
|---|---|---|---|
| today | `42` | `6` | applies |
| **broad** — no paren re-steps a `Function` | `fn z` ✅ | two values ✅ | **two values** (idiom removed) |
| **narrow** — only a held reference survives | `fn z` ✅ | two values ✅ | applies |

Broad is what clause 3 says literally and costs the inline-application
idiom (24 of the 31 rows, spec-covered across `fn-triple`, `modifiers`,
`usurp`, `recursion`, `apply`, `corpus-core`, `corpus-structures`,
`module-fmt`). Narrow keeps it but needs the rejected mechanism. Neither
is a bug fix, so neither is taken here.

**Clause 2 is blocked for a different reason.** It supersedes ADR-011's
final sentence (§12.4 records the measurement), and ADRs are amended only
on explicit maintainer instruction — so implementing it would leave an
Accepted record stating the opposite of the engine. The `unused_def` fix
in `7e98aeb` hooks the same `TFunction` intercept and reworks with it.

**Break 2** (the compiler's *"fn value read from a container
auto-dispatches (Stage 3)"* refusal) is untouched and independent of all
of the above.
