# Handover: the voxgig-boru libraries on main, stopped mid interpreter-entry pass (2026-10-02)

This page is for whoever picks up the downstream work on the nine
voxgig-boru libraries next: getting them to run on boru `main`, measuring
how far they are from full compilation, and recording every compiler defect
they surface in this repo. It covers the state at stop, the measurements,
the interpreter-entry pass that was stopped part-way (its work is saved as
patches beside this file), the defects found and where each is recorded, and
the tools. The compiler rules are in [COMPILABLE-SUBSET.md](COMPILABLE-SUBSET.md);
the answer-divergence ledger is [NUR.md](../NUR.md).

> **Recorded 2026-10-05.** The fourteen defects found at the very end
> (U1–U14 under "Defects found") are now in the ledgers they belong to:
> U1–U6 are [NUR378–NUR383](../NUR.md#nur378) (U1, U2, U4, U5 and U6
> re-verified on both lanes at 64c5ab2 with the lanes probe below; U3 keeps
> its library-scale repro), and U7–U14 with the root shapes R1–R6 are in
> [COMPILABLE-SUBSET.md](COMPILABLE-SUBSET.md) §5 ("Open refusals recorded
> 2026-10-05"). The U-numbers below are kept as the detailed account.

## State at stop

- boru `main` is at **64c5ab2f3** (round 5, #526), unchanged throughout. All
  measurements below are against it.
- Every library has an open migration PR from branch
  `claude/boru-main-migration`. All are pushed, nothing is merged:

| Library | PR | Head | CI | Notes |
|---|---|---|---|---|
| aless (was alice) | voxgig-boru/aless#6 | `c920c64` | **none ran**: the `tests` workflow is `disabled_inactivity` | A repo admin must re-enable it (Actions → tests → Enable workflow); a push then runs it. |
| bloom-filter | voxgig-boru/bloom-filter#26 | `bf4fdc8` | green | |
| cache | voxgig-boru/cache#2 | `0b98f26` | green | design only; placeholder suites |
| decision | voxgig-boru/decision#14 | `dbfb355` | green | |
| graph | voxgig-boru/graph#2 | `d1b2fc4` | green | design only; placeholder suites |
| sort | voxgig-boru/sort#8 | `ef389db` | green | |
| stats | voxgig-boru/stats#9 | `0f8a494` | green | |
| template | voxgig-boru/template#7 | `9cdd836` | green | |
| trie | voxgig-boru/trie#16 | `6f906e3` | green | the stopped pass left uncommitted edits; they are saved as a patch here (below) |
| admin | voxgig-boru/admin#1 | `51681b9` | no CI | the cross-library sweep tools and the 2026-10-02 baseline; the baseline is stale in two places (below) |

- This repo: boru-lang/boru#528 (branch `claude/record-stored-fn-tail-read`)
  carried every record listed under "Defects found" and this page; on
  2026-10-05 it was merged with the round-6 handover (#527) into one
  baseline branch, which also records U1–U14 (above).
- Each library's gate (`test/divergence/run.sh`, decision's
  `test/diverge.sh`) passes at its head: every suite compiles, runs green and
  checks with 0 errors, and every module checks clean.

## How far from full compilation (measured 2026-10-02)

Three levels, each measured on all 58 suites of the nine libraries:

1. **The program compiles.** 58/58 suites run on the only path (check →
   compile → VM): 0 compile refusals, 0 check errors, 0 runtime failures.
   This needed library changes, each a natural rewrite commented with the
   defect it avoids; every library's `dx-report.md` / `DX-REPORT.md` lists
   them.
2. **Every runtime callback compiles.** Callbacks built at run time
   (function values handed to `each`, `fold`, `Tui.run`, `boru:test`'s
   property machinery) are stamped one by one. `boru -compile-report` per
   suite: **0 declined** (89 before this session's generator rewrites, all in
   property-test suites; see "Defects found" for the shapes).
3. **Nothing runs on the interpreter at run time.** Not visible in
   `-compile-report`. An instrumented build (below) counts every
   unattributed, non-check interpreter entry from compiled code — a native
   word running a code body on a pooled interpreter, a fallback island. At
   stop: **415,854 entries in 30 of 58 suites.**

| Library | Entries | Suites with entries | Root shapes (see next table) |
|---|---:|---|---|
| trie | 334,312 | 11/11 | R1 (288,187), R2 (19,913), R6; the rest nested under those |
| sort | 39,002 | 3/5 | R3 (islands; each island counts twice, as its `vm:island*` seam and the `Engine.Run` it performs) |
| template | 32,320 | 8/8 | R5 (one island in the lexer loop; ~27,500 entries are nested inside it) |
| stats | 6,180 | 2/5 | R2 |
| bloom-filter | 2,660 | 5/5 | R2 (1,045), R4 (800 islands, ×2), R1 (15) |
| aless | 1,380 | 1/9 | R2 |
| decision, cache, graph | 0 | 0 | — |

Per suite: [handover/voxgig-interp-wip/census-2026-10-02.txt](handover/voxgig-interp-wip/census-2026-10-02.txt)
(the `unattributed=` column; `attributed=` counts entries a seam owns, such as
the check pass or a sandbox, and is out of scope).

### The root shapes

Fixing a root removes the entries nested inside it (chains with only
`go.(*Engine).*` frames).

| | Shape | Entered via (Go chain, innermost first) | Where | Natural rewrite | Status |
|---|---|---|---|---|---|
| R1 | `do {k: [expr], …}` — native `do` over a Map evaluates each list value on the interpreter | `doEvalDataList < DoEvalMapValue < DoMapHandler < vmContext.run` | trie's node builders and encode payloads (all four modules); bloom-filter test maps; template (inside R5) | a plain map literal, `{k: expr}` / `{k: (expr)}`: same map, 0 entries (probed in a fn body called from `each`) | trie: done in the WIP patch (0 entries, value-identical; gate not run). Others: not started. |
| R2 | `r.list-of [gen] n` (boru:rand) — the native runs its element body on a pooled interpreter, once per element | `runPooledAt < RunPooled < modules.randNativesForState.func6 < vmContext.tryNativeFnApply` | every property suite that generates lists: aless, bloom-filter, stats, trie | a named fn drawing in a loop: `[for n [(r.int 0 999)]]` or `iota n each [ var [[k] (r.int 0 999) ] ]` — the same draws in the same order (seed 7 → `[550, 417, 502, 54, 722]` both ways), 0 entries | trie: done in the WIP patch with a typed count (U9). Others: not started. |
| R3 | sort's comparator application: `xi xj comp/v apply` (the NUR123 workaround) re-steps through an island; trailing calls `fn comp(Any, Any) 5 3` island through `callDynTrailTop` | `runIslandResolved < applyReStep < callDynApply`, `… < islandRun < callDynTrailTop` | sort_prop_spec (~38k), sort_prop_test (1,088), sort_smoke_test (52) | property suites: a 0-arg fn returning the module member (U12); `sort.aql`: none (U3, U8, U11, U13) | prototype for the two property suites, unverified; `sort.aql` blocked upstream |
| R4 | a generator list literal holding `r.int …` / `r.one-of […]` calls | `islandRun < makeListReStep` | bloom_prop_spec rows 108, 119 | not investigated | not started |
| R5 | `if (i gte (toks size)) [do {code:[''] next:[i] …}] [ … ]` in template.aql's lexer/compile loop (~row 393) | `runIslandResolved < liveDeopt < deoptIfFn` | every template suite; most of template's entries are nested inside it | not investigated (R1's rewrite is the first thing to try) | not started |
| R6 | trie radix/tst property specs: fold bodies (`doFold < InvokeBody < RunResolved`), Reach lenses (`each $.1` → `ApplyReach < RunPooledSub`), a property body run through `CallBoru` (`InvokeCallback < checkPropBody < runCheckProp`) | as listed | radix_prop_spec, tst_prop_spec | rename the driver loop variable (U10) | done in the trie WIP; the fold and lens entries were nested and vanish with R1/U10 |

Template's `boru:vm` sandbox runs are *attributed* entries (the sandbox
compiles each generated program and is allowed to interpret) and are out of
scope.

## Work in progress (stopped)

A workflow ran one implementer agent per library (trie, sort, template,
bloom-filter, stats, aless), each to be followed by an independent
adversarial verifier. It was stopped on request while the first two
implementers (trie, sort) were running; the other four never started.
Nothing from it is committed or pushed. What exists is saved in
[handover/voxgig-interp-wip/](handover/voxgig-interp-wip/). The harnesses and
outputs behind the "re-verified" claims below lived in the session's scratch
space and are gone; re-run them before trusting the patches. trie's working
tree was reset to `6f906e3` once its diff was saved here.

### trie — `trie-interp-entries-wip.patch`

9 files, +144/−56; applies to trie#16's head `6f906e3` (it was that
working tree's uncommitted diff at stop). Re-verified after the stop, on
scratch copies, by a second agent:

- **Interpreter entries: all 11 suites, 334,312 → 0** (no islands).
- **Value identity:** a harness over 11 key sets (keys such as `if`, `do`,
  `end`, `kids`, and Unicode) through every Set/Map word, `encode` /
  `decode` and the raw node maps prints byte-identical output on the old and
  new modules (trie 5,480 lines, radix 5,378, tst 6,343, burst 4,558). The
  old `r.list-of` and the new loop generate identical lists for seeds 1, 2,
  7, 42 and 1234 across every generator shape. Every suite's stdout is
  identical.
- `boru check`: 0 errors on all 4 modules and 11 suites; `-compile-report`:
  0 declines; every suite exits 0 and prints `all green`.
- **Not done:** `test/divergence/run.sh` itself was not run on it; the
  DX-REPORT.md section "Run-time interpreter entries" that the patch's
  comments cite (10 times) does not exist yet; BORU-MAIN-VERIFICATION.md is
  not updated; nothing is committed.

What it changes:

1. R1: every node builder and encode payload in the four modules becomes a
   plain map literal (`{end: fin, val: val, kids: kids}`), same fields in the
   same order.
2. R2: the generator helpers draw in a loop — `def n:Integer (r.int lo hi)`
   then `[for n [ (r.string charset (r.int 1 maxlen)) ]]`. The `:Integer` is
   required (U9 below).
3. R6: the four property specs' driver loop variable `s` becomes `spec`.
   That alone removes 21,266 entries (radix) and 3,224 (tst): see U10.
4. radix's `common-len` keeps its `do {…}` steps, with a new comment saying
   a plain literal raises `undefined word: acc`. **That comment is partly
   wrong:** the plain literal failed only because the stale property stamp
   (U10) ran the fold on the interpreter, where U4 applies; with the `spec`
   driver a plain literal is green. Fix the comment, and say U4 can give a
   silently wrong value, not only `undefined word`.

To finish: apply it on trie#16's branch, fix that comment, add the
DX-REPORT section (the before/after table, each rewrite, U4/U9/U10/U14 with
their repros), run the gate, record the retest in BORU-MAIN-VERIFICATION.md,
commit, push.

### sort — `sort-interp-entries-prototype.patch`

Test files only (`sort_prop_spec.aql`, `sort_prop_test.aql`); applies to
sort#8's head `ef389db`. The stopped agent changed nothing in the repo; this
was its scratch prototype, re-measured after the stop:
**sort_prop_spec 37,862 → 0, sort_prop_test 1,088 → 0**; sort_smoke_test
stays at 52 (U11); the unit suites were already 0. `boru check` and
`-compile-report` are clean on it, and the recovery agent's harness found
the PropertyResult maps and printed output identical for the suites' own
seeds. The gate was not run and nothing is committed.

The change: the property bodies call `(by-num)`, a 0-arg fn returning
`Sort.by-number/v`, instead of reading a top-level
`def by-num (Sort.by-number/v)` by `/v` (U12), and `sort_prop_test`'s P6
passes an inline lambda as the key fn. Caveat: the 0-arg trick removes the
islands only when it returns a MODULE member; with a program-defined
comparator it still islands, and a program-level def of a combinator result
read by `/v` in a property body is refused (U7) or fails compiled (U6).

`sort.aql` itself has no natural fix. Its `comp/v apply` must stay: the bare
and forward spellings silently corrupt a later call (U3) or are refused (U8),
and a compare-with helper raises the count (U13). sort_smoke_test's 52
entries are U11 (the native `cmp/v` through a Function param), owed
upstream. Open choice: keep the smoke demo on `cmp/v` (52 entries until
U11 is fixed) or move it to `Sort.by-generic/v` (0, but no longer a native
word value).

To finish: apply, add the DX-REPORT section the patch's comments cite, run
the gate, commit, push.

## Defects found, and where each is recorded

Every defect below was measured on both lanes (`RunInterp` vs
`RunCompiledReason`, a fresh engine per lane) at 64c5ab2, with a minimal
repro except where noted (NUR372 has a library-scale one; the last item has
none).

**Answer divergences — NUR.md (all Pending):**

| Record | Defect | Silent? | Hit in |
|---|---|---|---|
| NUR366 | a declined stored fn breaking its return count raises `internal_error`, not the contract's `type_error` | no | decision |
| NUR367 | a loop-rebound local read bare in an arm after the loop comes back uncalled | **yes** | decision (`unique`) |
| NUR368 | a run-time fn value applied at the main program applies late (`print (41 f) 7` prints 41) | **yes** | decision |
| NUR369 | a re-entered fn-value param called from an `each` callback applies the inner call's fn | **yes** | template nested blocks |
| NUR370 | a fn param called on a body-local def raises `DISPATCH_GENERIC` | no | template |
| NUR371 | a callee's fold-body def clobbers the caller's same-named local | no | template |
| NUR372 | a fold body reading its param raises `undefined_word` on the 5th–8th compile in a process (library-scale repro only) | no | template filters |
| NUR373 | an ungrouped `r.list-of […] n` as a fn's result repeats its first draw | **yes** | aless, trie generators |
| NUR374 | a grouped `r.list-of` generator raises `undefined word: r` | no | aless, bloom, stats, trie |
| NUR375 | a callback binding `r.int` draws with `def` raises "a landed fn value takes arguments (NUR298)" | no | generator rewrites |
| NUR376 | a name read inside `do {k: [expr]}` leaves a later same-named binding undefined | no | stats (`mode`, `ols`, `zscores`) |
| NUR377 | `boru:test` mints its record types from a fresh ID counter (`BuildTestModule` lacks `AdoptSeqFrom`): a type made after it fails its own return contract, and `is` answers false | **yes** (`is`) | bloom-filter, stats (both import the library before `boru:test` as the workaround) |

**Compile refusals — COMPILABLE-SUBSET.md §5, "Open refusals recorded
2026-10-02":** a file-imported fn whose result is a bare gradual read
declines its stamp (decision); calling a run-time fn value (most
spellings); a nested var-binding `each` at top level ("twin regime",
aless); the compile pass's 500,000-step analysis budget, exhausted, is
misreported as a compiler defect (aless); a callback applying a fn-valued
field of its param — every bare `boru:test` generator — declines (81 of the
89); a callback `var` reusing a loop-carried def name of a called word
declines (decision row K); a fn-local `Test.check-prop` whose property
interpolates a template refuses (sort harness); a `Test.prop` with an
interpolated template refuses (template harness).

**U1–U14 — recorded 2026-10-05** (U1–U6 as NUR378–NUR383, U7–U14 in §5;
this is the detailed account):

Found while measuring and re-verified on both lanes after the stop
(standalone repros; the interpreter lane via the lanes probe). Record
these first — the four silent ones (U1–U4) above all.

**Silent answer divergences**

- **U1.** An `each` callback applying its fn's `Function` param reuses the
  FIRST call's fn on later calls:
  ```
  def f fn [[c:Function x:Integer] [List] [ [x] each [ var [[e] (e 3 c) ] ] ]]
  print (f add/v 5)     # both lanes [8]
  print (f sub/v 7)     # interpreted [4]; compiled [10]
  ```
  `boru check` clean, exit 0. The forward `(c 3 e)` is affected too;
  `(e 3 c/v apply)` agrees. Possibly NUR369's root, but NUR369 is
  re-entrant and this is sequential.
- **U2.** A param named after a built-in word (`cmp`, `sub`): the
  interpreter raises `reserved_word` (`undef sub: 'sub' is a built-in word
  and cannot be redefined`) for any such param; `boru check` is silent;
  compiled runs and may apply the BUILT-IN instead of the passed fn:
  ```
  def g fn [[sub:Function] [List] [ [5] each [ var [[e] (e 3 sub/v apply) ] ] ]]
  print (g add/v)       # interpreted reserved_word; compiled [2] (add gives [8])
  ```
  With `[[cmp:Function xs:List] …]` and `sub/v`, compiled gives `[1, -1]`
  where `sub` gives `[2, -2]`. Likely mechanism (read from the VM, not
  proven): the frame renames a native word value to its param name and the
  native fast path looks the native up by that name — the same mechanism as
  U11. The libraries' params are not built-in names.
- **U3.** NUR123's shape, though NUR123 is archived as fixed: a recursive
  helper reading its `Function` param bare (or forward) and by `/v`,
  reached from an `each` body, silently corrupts a LATER, unrelated call's
  loop state. sort's DX-REPORT "workaround 1" has the repro
  (`sd`/`first`/`second`): `print (cmp/v first)` then `print (3 second)`
  gives `0` `4` interpreted and `0` `1` compiled with `def c ((arr get 0)
  (arr get 1) comp)` or the forward `def c (comp (arr get 1) (arr get 0))`;
  `comp/v apply` gives `0` `4` on both. In the suites `Sort.heap` followed
  by `Sort.tim` returned the input unsorted.
- **U4.** A `var`-bound name in a plain map literal inside an `if` arm of a
  fold body: the interpreter resolves it to a same-named top-level def
  (or, without one, raises `undefined word`); compiled reads the `var` —
  here the compiled answer is the intended one:
  ```
  def acc {n: 100}
  def count fn [ [xs:List] [Map] [
    {n: 0} xs [ var [[x acc] if true [ {n: ((acc "n" get) 1 add)} ] [acc] ] ] fold
  ] ]
  print (count [7 8 9])   # interpreted {"n": 101}; compiled {"n": 3}
  ```
  Without the `if`, `boru check` refuses `undefined word: acc`; with it,
  check is clean. Any path that runs such a fold on the interpreter (U10)
  turns this into a wrong value or a failure.

**Loud answer divergences**

- **U5.** A FILE-module fn whose `each` callback forward-applies its
  `Function` param to another param: `def h fn [[f:Function y:Any] [List] [
  [5] each [ var [[e] (f y e) ] ] ]]`, exported `M.h`, then `print (M.h
  sub/v 3)`: interpreted `[2]`; compiled `each: element 0: signature_error:
  cannot call f` (with a user fn: `undefined word: y`). The same fn in one
  file agrees; the trailing `(e y f)` agrees.
- **U6.** A closure made by a module fn, bound by a program-level `def` and
  read by `/v` in a property body: module `def mk fn [[n:Integer]
  [Function] [ [a:Any] => [ a add n ] ]]`, `def use fn [[q:Function
  x:Integer] [Integer] [ (x q) ]]`, exported; then `def c (M.mk 10)` and
  `Test.check-prop "p" [ 5 ] [ var [[x] ((M.use c/v x) eq 15) ] ] 2 1 0`:
  interpreted `ok: true`; compiled `ok: false`, `error(undefined word: q)`.
  `def c (Sort.by-key k/v)` read as `c/v` in a check-prop body fails the
  same way (`undefined word: comp`).

**Compile refusals (§5 kind)**

- **U7.** A top-level `def by-val-key (Sort.by-key val-key/v)` read as
  `by-val-key/v` inside a `check-prop` body refuses the program: "operand of
  unknown provenance or not statically materialisable at test-check-prop";
  the interpreter passes (sort_prop_test's P6 shape).
- **U8.** The forward spelling in the NUR123 recursive shape refuses:
  `def go fn [[comp:Function n:Integer xs:List] [Integer] [ def c (comp (xs
  get 1) (xs get 0)) def _r (if (n gt 0) [ (xs (n sub 1) comp/v go) ] [ 0 ])
  n ]]`, `print ([3 1] 2 cmp/v go)`: interpreted `2`; compiled "fn go:
  unapplied fn-value in body residual (dynamic apply not compiled in a fn
  body)".
- **U9.** `for` over an untyped def bound from a `boru:rand` member call:
  `def f fn [ [lo:Integer hi:Integer r:Map] [List] [ def n (r.int lo hi)
  [for n [ 7 ]] ] ]`, `print (f 2 4 (Rand.with-seed 7))`: interpreted
  `[7, 7, 7]`; compiled "unmatched dispatch recovered at for". In a property
  generator helper the stamp declines silently and the generator runs on the
  interpreter. `def n:Integer (…)` compiles; an untyped def from a plain Map
  field compiles.

**Interpreter entries (answers agree)**

- **U10.** A run-time-stamped property body goes stale on EVERY run when
  the top-level loop variable driving `Test.run-property` / `check-prop`
  shares a name bound inside the property's callees and a Reach lens
  (`each $.1`) is on the call path; after the re-stamp budget
  (`RestampMaxTries = 3`, `compiler/go/stamp_runtime.go`) the remaining runs
  go through `CallBoru`:
  ```
  import "boru:test"
  import "boru:struct-util"
  def pluck fn [ [m:Map] [List] [ (m StructUtil.items) each $.1 ] ]
  def build fn [ [ks:List] [Map] [ {} ks [ var [[k acc] acc set (k) 1 ] ] fold ] ]
  def _ ([1] each [ var [[k] def res (Test.check-prop "x" [ ["ab" "b"] ] [ var [[ks] (ks build) pluck drop true ] ] 20 1 0) print (res) 0 ] ])
  ```
  Both lanes answer `ok: true`, but 16 of the 20 runs go to the
  interpreter (the trace shows 48 `INTERP` lines, three seams per run).
  Renaming the loop variable (`var [[q] …]`), or replacing the lens with a
  `var` each, gives 0. Why callee-local names
  enter the property body's dependency snapshot when a lens is present is
  not determined.
- **U11.** A native word value (`cmp/v`) passed through a FILE-module fn's
  `Function` param is applied on an island, not the native fast path
  (`print (Sort.quick cmp/v [5 3 8 1 9 2 7 4 6 0])`: 52 entries; every
  algorithm with `cmp/v`: 4,440). A diagnostic build shows the param
  rename (`name="comp" nativeWord=true regNative=false`) defeating the
  by-name native lookup. Owed: key the fast path on the value's own native
  identity. One file: 0 entries.
- **U12.** A program-level fn value read inside a property body and applied
  inside a module fn runs on an island (`CallBoru`): a top-level
  `def by-num (Sort.by-number/v)` read as `by-num/v` in a `Test.prop` body
  costs 41,200 entries; a 0-arg fn returning the MODULE member costs 0; a
  program-defined comparator islands either way.
- **U13.** A forward application `(comp y x)` of a `Function` param inside a
  module fn islands via `callDynFrame` (2 islands per call with `cmp/v`).

**Checker false positives**

- **U14.** `def f fn [ [lo:Integer hi:Integer r:Map] [Integer] [ def
  n:Integer (r.int lo hi) n ] ]`, `print (f 2 4 (Rand.with-seed 7))`: the
  check refuses with "expected 1 return value(s), got 3"; both lanes answer
  `3`. The untyped def and `def n ((r.int lo hi))` pass.
- A `[List]`-declared `r.list-of` helper (NUR373's text) and a var-bound `r`
  in an `r.list-of` element body (NUR374's text).

One more, transcript only (no repro survived): a trie harness draft failed
to compile with "twin regime: a bind transition has no stream placement",
on the old and new modules alike.

## Tools (saved in handover/voxgig-interp-wip/)

- `interp-trace-build.patch` — applies to 64c5ab2f3. Prints
  `INTERP\t<seam>\t<Go caller chain>` for every unattributed, non-check
  interpreter entry (`core/go/interp_entry.go` `noteInterp`) and
  `ISLAND\t<site>\t<row:col>\t<first tokens>` for every island
  (`eng/go/vm.go` `islandRun`, `eng/go/vm_defer.go` `runIslandResolved`),
  when `BORU_INTERP_TRACE=1`. Build it in a worktree:
  `git worktree add --detach <dir> 64c5ab2f3 && cd <dir> && git apply …/interp-trace-build.patch && cd cmd/go && go build -o <bin> ./boru`.
- `interp-count.sh` — runs suites on that binary and summarises INTERP /
  ISLAND counts and the top chains: `T=<binary> …/interp-count.sh
  test/<suite>.aql`, from a library's root.
- `lanes_test.go.txt` — a throwaway Go test that runs each file named in
  `LANES_FILES` on the interpreter and compiled, printing both outputs. Copy
  it to `lang/go/zzlanes/lanes_test.go` (untracked; never commit it) and run
  `cd lang/go && LANES_FILES="<abs paths>" go test ./zzlanes/ -run TestZZLanes -count=1 -v`.
  The CLI cannot run the interpreter any more, so this is how a divergence
  is confirmed.
- `census_test.go.txt` — the same idea over whole suites with the
  interpreter-entry and runtime-bail hooks armed (`ArmInterpEntryHook`,
  `ArmRuntimeBailHook`): per suite, ran / green / unattributed / attributed
  / by seam. `CENSUS_ROOT`, `CENSUS_LIBS`, `CENSUS_OUT` select inputs.
- admin's `bench/verify-libs.sh` (correctness sweep) and
  `bench/bench-libs.sh` (timings) — see admin's README.

## Next, in order

0. Done 2026-10-05: U1–U14 recorded — U1–U6 as NUR378–NUR383, U7–U9,
   U10–U13 with R1–R6 (run-time interpreter use, which §6 calls a defect
   "owed a real lowering") and U14 in COMPILABLE-SUBSET.md §5. NUR380 (U3)
   still owes a minimal repro.
1. **trie**: finish and verify the WIP patch (below), then commit and push
   to trie#16.
2. **sort**: decide the comparator shape (below), then the same.
3. **template** (R5), **bloom-filter** (R1 tests, R2, R4), **stats** (R2),
   **aless** (R2): not started. R2's rewrite is mechanical; the generator
   must stay in a named fn whose param is `r` (inline forms decline or hit
   NUR374), and every rewrite needs an old-vs-new value check over several
   seeds — every run, seed and max-shrinks unchanged.
4. Done 2026-10-05: R1–R6 are in COMPILABLE-SUBSET.md §5 beside U10–U13.
5. **Point the libraries' dx-reports at the records.** Several still say
   "not yet recorded upstream" for what is now NUR373–NUR377 or a §5 bullet:
   aless `dx-report.md` (~265–286: NUR374, NUR373), bloom-filter
   `dx-report.md` (M1 → NUR377; ~217–223 → NUR374), stats `dx-report.md`
   (D1 → NUR376; ~277 → NUR374), trie `DX-REPORT.md` (~823–830 → NUR374,
   NUR373), sort `DX-REPORT.md` (~187 → §5), template `dx-report.md`
   (~281–292 → §5). decision is done (`dbfb355`).
6. **admin**: re-run the sweep and update `baselines/BASELINE-2026-10-02.md`
   and the README. It still says 89 runtime callbacks decline (now 0) and
   has no interpreter-entry census.
7. **aless CI**: needs an admin to re-enable the workflow (above).
8. If boru `main` moves past 64c5ab2f3: re-run admin's sweep first; several
   workarounds name defects that may be fixed (each library lists them).

## Lessons that cost time

- `boru -compile-report` reports compile stamps, not run-time interpreter
  use. A suite with 0 declines can still spend most of its time on the
  interpreter (trie: 334,312 entries with every callback stamped). Measure
  with the trace build.
- Confirm every suspected divergence on both lanes with the lanes probe
  before recording it. The CLI runs only the compiled path, and `-no-check`
  is for probing only, never for getting green.
- The check pass has false positives around `r.list-of` (NUR373's text);
  a `[List]`-declared generator helper can be refused while an `[Any]` one
  runs.
- A generator's shape decides whether it stamps: group every member draw
  (`(r.int 0 5)`), and keep a nested generator in a named fn whose param is
  `r` with its body grouped. The ungrouped `r.list-of` result repeats its
  draw silently (NUR373), and every property still passes on the repeats.
- Import a library BEFORE `boru:test` (NUR377).
- Lists of record numbers drift: check NUR.md, open PRs and `git log -S` before
  assigning a NUR number (NUR373–NUR377 were free on main and in open PRs on
  2026-10-02).
- The knowledge graph digests the Go package list: run boru's commit gate in a
  clean worktree when untracked probe packages (`lang/go/zz*`) are present.
