# Higher-Order Functions in boru — capability audit

**Status: point-in-time report (2026-08-19; re-assessed 2026-08-21).**
A dated empirical snapshot, not a living reference — see
`design/README.md`. Its claims describe the engine as built from this
tree on that date; several of them are pinned to NUR records whose fixes
will change the answers.

**Method:** empirical, against `cmd/go/bin/boru` built from this tree,
cross-read with `core/`, `lang/`, `NUR.md`, `ADR.md` and
`lang/spec/*.tsv`. Every claim below is either a source citation or a
command that was run and whose output is quoted verbatim. Examples are in
the canonical **forward call form** (`AGENTS.md` §"Working in the code")
and use the **fewest square brackets** each signature admits
([`STYLE-GUIDE.md`](../STYLE-GUIDE.md) §S1 / §S2); every one was re-run in
its final spelling — see §4.1 and §4.2 for why those re-runs are not a
formality. Where a `fn` here keeps the bracketed spec-list form, its
signature cannot be reduced: two or more parameters, zero parameters, or
overloads (§4.2).

Companion to [`LISP-ANALYSIS.5.md`](LISP-ANALYSIS.5.md), which graded
higher-order functions **A−** and combinators **B+** from a design
reading. This note tests those grades by *writing the programs*. The
substrate grade holds. What the design reading missed is a cluster of
**call-vs-value** hazards that only surface once a function value flows
through a parameter, and one live **engine disagreement**.

---

## 0. Verdict

| Question | Answer |
|---|---|
| Are functions first-class? | **Yes.** Storable in lists and maps, passable, returnable, closing over locals, recursive, introspectable. |
| Can all combinators be implemented? | **Yes**, since the `/v` change (2026-08-19). S, K, I, B, C, W, U, the full Church basis — booleans *including* `and`/`or`, numerals, pairs — CPS, and a working parser-combinator library were all built and run. The one construction the first pass could not finish now works in its naive spelling (§1.2, §5.2). |
| Is it *pleasant*? | **Much more so.** `/v` is total over binding kinds, so the polymorphic combinators need no escape hatch — the two undocumented idioms (`(args).N` and boxing) are no longer required. |
| Is it *safe*? | **Safer since 2026-08-24.** The interpreter-vs-compiler divergence (§5.7, NUR073) is CLOSED by the BROAD fix. Three silent-wrong-answer classes remain, listed in §5 — and since 2026-08-25 the quietest of them, §5.1, is named by `boru check` as `stranded_type_call` instead of passing without comment. |

**One-line summary:** boru's function substrate is complete — every
combinator in the audit was built and run — and since `/v` became total
over binding kinds the surface has one spelling for "the value, whatever
kind it is". What remains is narrower: a *call-vs-value* ambiguity at the
paren-application and capitalised-name sites. The engine-dependence that
this line originally named (§5.7) is closed as of 2026-08-24: the BROAD
fix makes a paren place its collapsed Function on both lanes, and with it
the paren-application ambiguity itself is gone — application is now
explicit, which is a surface change every §1 program had to absorb.

> **Re-assessed 2026-08-19** after `/r`→`/v` / `ref`→`valof` landed with the
> function-only gate removed. §5.2 is closed and NUR085 retired; §5.1,
> §5.3–§5.9 were each re-run against the new binary and are unchanged.

> **Re-assessed 2026-08-21** after the type-node fusion (PR #394) and
> the valof-flip / NUR095 work (PR #396). Every §1 program, §4 spelling
> and §5 hazard was re-run against a binary built from that tree. The
> substrate verdict stands — every §1 program still produces its quoted
> output on both engines — and §5.2 stays closed (no valof-flip
> regression on non-type bindings). Four claims moved, each corrected in
> place: §5.1's `I/v apply` escape hatch is **gone** (`/v` on a type
> binding now denotes the minted node — `lang/spec/valof.tsv` §11 — so
> lowercase names are the only remedy) and its stranded pair prints
> differently; §2's "`Function` is opaque" now holds for *bare*
> `Function` only (`fnsig` shipped, and fn-shape class members apply on
> both engines — NUR095 retired, NUR096 records the checker lag);
> §1.6's non-tail 100 000-deep row holds only on the compiled lane (the
> interpreter now trips the tape ceiling near depth 40 000); and §5.4's
> fn-body return-arity error lost its `ap:` prefix (it now names the
> enclosing fn). NUR073 (§5.7) was re-run and is live, unchanged, the
> default still siding with the compiler; NUR086–NUR089 all still
> reproduce byte-for-byte. New records NUR091 and NUR096 join §5.1's
> and §5.5's defect classes respectively. Stale citations fixed in
> place.

> **Recommendation 2 closed 2026-08-25.** §5.1's shape — a capitalised
> name bound to a fn body, written in call position — now costs a
> check-mode warning, `stranded_type_call`, carrying the did-you-mean
> treatment the item asked for. `boru do` is unchanged and still silent:
> the program runs and exits 0, so the finding sits in the suspicion
> tier by the repo's own severity discipline, not by omission. §5.1
> records the gate and the four shapes it deliberately stays quiet on.

---

## 1. What works — the evidence

Every program below was written and run against this tree, under both
`-no-compile` (interpreter) and the default (bytecode, with any program
the compiler refuses silently routed to the interpreter — §5.8), and is
quoted here in full — definitions **and** the calls that produced the
quoted output — so it can be re-run from the note itself.

> **Superseded in spelling 2026-08-24 — NUR073's BROAD fix landed.** Every
> §1 program below is written in the paren-application idiom
> (`((f a) b)`), and that idiom no longer exists: a paren PLACES its
> collapsed function value and never re-steps it, so application is
> explicit. The programs themselves all still work — each was re-derived
> and re-run in the new spelling, and the corpus rows carry it — so the
> substrate verdict is unchanged; only the surface moved. The mechanical
> translation is three rules:
>
> - a def-bound WORD still calls: `(toint c3/v)` is unchanged;
> - a param-held fn applies through `apply`, argument beneath:
>   `(f x)` becomes `x f/v apply`;
> - a curried chain stages: `((f a) b)` becomes `b (f a) apply`.
>
> So `def ii ((ss kk/v) kk/v)` is now `def ii (kk/v (ss kk/v) apply)`,
> and `(ii 42)` is `42 ii/v apply` — still `42`, still `check: 0
> error(s)`. The parser-combinator library of §1.5 produces its four
> quoted results byte-for-byte in the new spelling. Read the transcripts
> below as the 2026-08-19 record; read
> `lang/spec/frontier/frontier-hof-audit.tsv` for what runs today.
>
> **Pinned 2026-08-21.** These transcripts are now also a standing gate:
> `lang/spec/frontier/frontier-hof-audit.tsv` re-checks every §1.1–§1.5
> value (plus §1.6's combinator rows) on the interpreter oracle on every
> `make test`, with the compile ledger in
> `test/go/langspec/frontier_spec_test.go` pinning which rows the
> compiler still refuses and why (§5.8's provenance class, §4.3's capture
> family, NUR087's check false positive). The §1.4 divergences — Z, and
> the naive Y that no strict language can run — are deliberately NOT
> pinned: no budget bounds them (see the §1.4 correction), so a test
> would hang the suite. The deterministic §5.4 behaviour implicated in
> Z's compiled-lane divergence is pinned instead (the corpus file's §7
> rows: the statement-level apply and the `/v apply` workaround — the
> 2026-08-24 correction in §5.4 narrows what that rule explains).

### 1.1 The classical combinator bases

`I = S K K`, derived rather than assumed:

```boru
def kk x:Any => [y:Any => [x]]
def ss f:Function => [g:Function => [x:Any => [(f x) (g x)]]]
def ii ((ss kk/v) kk/v)
print (ii 42)        ;# 42
print (ii "hello")   ;# hello
```

`check: 0 error(s)`, correct on both engines. **BCKW** runs correctly on
both engines too, but — unlike SKI — it does **not** check clean:

```boru
def bb f:Function => [g:Function => [x:Any => [f (g x)]]]
def cc f:Function => [x:Any => [y:Any => [(f y) x]]]
def kk x:Any => [y:Any => [x]]
def ww f:Function => [x:Any => [(f x) x]]
def add2 a:Integer => [b:Integer => [add a b]]
print (((bb (n:Integer => [mul n 2])) (n:Integer => [add n 3])) 4)  ;# 14
print (((cc add2/v) 1) 10)                                             ;# 11
print ((kk 7) 99)                                                      ;# 7
print ((ww add2/v) 4)                                                  ;# 8
```

`14 11 7 8` on both engines, but:

```
check: 1:51: [error] no_signature: cannot call `g` — no signature matches
  the arguments; got (Integer); assuming best-fit candidate for analysis
  1 | def bb f:Function => [g:Function => [x:Any => [f (g x)]]]
                                                        ^
check failed: 1 error(s)
```

**The combinator is not what decides this — the CALL SITE is.** The same
`bb`, called with named fn references instead of inline lambdas, checks
clean and returns the same `14`:

```
;# args are inline lambdas       → check failed: 1 error(s), runs to 14
print (((bb (n:Integer => [mul n 2])) (n:Integer => [add n 3])) 4)

;# args are named fn references  → check: 0 error(s),        runs to 14
def d fn n:Integer Integer [mul n 2]
def e fn n:Integer Integer [add n 3]
print (((bb d/v) e/v) 4)
```

And `ss` — which §1.1 shows checking clean — fails identically when
*it* is called with inline lambdas. So this is not S-versus-B and not a
property of the body: **an inline lambda argument loses information a
named `/v` reference keeps**, and only the lambda spelling makes the
analysis mistake `g` for an `Integer`. The SKI block above escapes it
solely because it applies `kk/v`, a reference.

That is a checker defect, not the opacity cost of §2 — an inline lambda
and a named reference to the same function should be equally
checkable. Recorded as **NUR089**.

**A declared function type does not fix it, and it is worth being exact
about that.** Replacing `f:Function` with a declared `f:IntToInt`
(`design/FUNCTION-TYPES.0.md`) leaves the lambda call site still failing
the check — the shape it rejects is real (a `=>` lambda declares no
return, so it is `Integer → Any`, not `Integer → Integer`), but widening
the declared type to `Integer → Any` makes the program *run* while the
check still errors. So NUR089 is orthogonal to opacity and survives the
fn-type work. What function types do buy is stated in §2 and measured in
`FUNCTION-TYPES.0.md`: **sound rejection** of a wrong-shaped argument
that bare `Function` accepts.

### 1.2 Church encodings

Numerals and pairs work unmodified:

```boru
def czero f:Function => [x:Any => [x]]
def csucc n:Function => [f:Function => [x:Any => [f ((n f/v) x)]]]
def cplus m:Function => [n:Function => [f:Function => [x:Any =>
             [((m f/v) ((n f/v) x))]]]]
def cmult m:Function => [n:Function => [f:Function => [(m (n f/v))]]]
def toint n:Function => [((n (k:Integer => [add k 1])) 0)]
def c1 (csucc czero/v)
def c2 (csucc c1/v)
def c3 (csucc c2/v)
print (toint c3/v)                    ;# 3
print (toint ((cplus c2/v) c3/v))     ;# 5
print (toint ((cmult c2/v) c3/v))     ;# 6
```

```boru
def cpair a:Any => [b:Any => [s:Function => [((s a) b)]]]
def cfst  p:Function => [(p (a:Any => [b:Any => [a]]))]
def csnd  p:Function => [(p (a:Any => [b:Any => [b]]))]
def p ((cpair 1) 2)
print (cfst p/v)   ;# 1
print (csnd p/v)   ;# 2
```

Church **booleans** work in their naive spelling. `λt.λf.t` returns a
parameter, and since `/v` is total over binding kinds that is just `t/v`
— no boxing, no `(args).N`:

```boru
def ctrue  t:Any => [f:Any => [t/v]]
def cfalse t:Any => [f:Any => [f/v]]
def cif  p:Function => [t:Any => [e:Any => [((p t) e)]]]
def cnot p:Function => [t:Any => [f:Any => [((p f) t)]]]
def cand p:Function => [q:Function => [((p q/v) cfalse/v)]]
def cor  p:Function => [q:Function => [((p ctrue/v) q/v)]]
print ((((cif ctrue/v)  "T") "F"))
print ((((cif cfalse/v) "T") "F"))
print ((((cif (cnot ctrue/v))  "T") "F"))
print ((((cif (cnot cfalse/v)) "T") "F"))
print ((((cif ((cand ctrue/v)  cfalse/v)) "T") "F"))
print ((((cif ((cand ctrue/v)  ctrue/v))  "T") "F"))
print ((((cif ((cor  ctrue/v)  cfalse/v)) "T") "F"))
print ((((cif ((cor  cfalse/v) cfalse/v)) "T") "F"))
```

```
T  F  F  T  F  T  T  F      (one per line)
```

`check: 0 error(s)`. That is `true`, `false`, `¬true`, `¬false`, `T∧F`,
`T∧T`, `T∨F`, `F∨F` — the full basis.

> **Before `/v` (kept as the record of what changed).** Under the old
> `/r`, this did not work. `[t]` called a function-bound parameter and
> `[t/r]` refused a non-function one, so `ctrue`/`cfalse` needed
> `(args).N` plus a boxing step to reach the enclosing fn's argument —
> and `cand`/`cor`, which must return one Church boolean *as itself*,
> resisted every spelling tried: named booleans tripped
> `lang/spec/fn-value.tsv` §5's loud-dispatch rule ("a failed dispatch
> is LOUD, wherever the value would have gone" — the original text
> mis-cited `valof.tsv` §5 for this; fixed 2026-08-21), and holding them
> anonymously in a list then ran into forward-collection barriers. It
> was the one construction the audit could not finish. Removing the
> function-only gate closed it (§5.2, NUR085 retired).

### 1.3 Continuation-passing style

CPS works cleanly, including the closure-per-frame that CPS implies:

```boru
def factk fn [[n:Integer k:Function][Any][
  if (lte 1 n) [ (k 1) ]
               [ def kk ( fn r:Integer Any [ def m (mul n r) (k m) ] )
                 (factk (sub 1 n) kk/v) ] ]]
print (factk 5  (v:Integer => [v]))          ;# 120
print (factk 10 (v:Integer => [add 0 v]))    ;# 3628800
```

### 1.4 Anonymous recursion

The U-combinator form runs on **both** engines with a clean check:

```boru
def fgen fn s:Function Function [
  ( fn n:Integer Integer [ if (lte 1 n) [1] [ mul n ((s s) (sub 1 n)) ] ] ) ]
def fact (fgen fgen/v)
print (fact 5)       ;# 120
```

The full **Z combinator** (`λf.(λx.f(λv.(x x)v))²`) **diverges**. The
cause is not call-by-value eagerness: lambda bodies were confirmed lazy
(a `raise` inside an unapplied lambda body never fires). On the
compiled lane the cause is §5.4's collect — `((x x) v)` does not
apply there, so the `λv` thunk never actually guards the recursion.

> **Corrected 2026-08-24.** This originally attributed the divergence
> to §5.4 unconditionally ("`((x x) v)` does not parse as an
> application inside a body"). §5.4's same-date correction shows the
> interpreter DOES apply that shape — the collect is the compiled
> lane's — so the attribution above holds for the compiled lane only.
> Why the interpreter's run also diverges is unestablished, and cannot
> be re-established from this page: the Z source was not quoted here,
> the one §1 program that was not.

> **Corrected 2026-08-21.** This originally said "it hangs until the
> step limit". Measured: the step limit never trips — a run under
> `--options steps:200000` was still going at 30 s, and an in-process
> run grows until the OS kills it — so the divergence is bounded by
> nothing the engine counts, only by an outside timeout. That is also
> why Z is not pinned as a test (§1's pinning note): there is no budget
> under which "diverges" terminates into an assertable error.

### 1.5 A real parser-combinator library

The standard stress test for higher-order support — `item`, `sat`, `alt`,
`seq`, `many`, all returning closures over their arguments. A parser is a
`Function: String -> {ok val rest}`:

```boru
def pitem s:String => [
  if (eq 0 (size s)) [ {ok:false rest:s val:None} ]
                     [ {ok:true val:(slice 0 1 s) rest:(slice 1 (size s) s)} ] ]

def psat fn p:Function Function [
  ( fn s:String Map [
      def r (pitem s)
      if (r.ok) [ if (p (r.val)) [ r ] [ {ok:false rest:s val:None} ] ]
                [ r ] ] ) ]

def palt fn [[a:Function b:Function][Function][
  ( fn s:String Map [ def r (a s)  if (r.ok) [ r ] [ (b s) ] ] ) ]]

def pseq fn [[a:Function b:Function][Function][
  ( fn s:String Map [
      def r1 (a s)
      if (r1.ok)
        [ def r2 (b (r1.rest))
          if (r2.ok) [ {ok:true val:[(r1.val) (r2.val)] rest:(r2.rest)} ]
                     [ {ok:false rest:s val:None} ] ]
        [ {ok:false rest:s val:None} ] ] ) ]]

def manyloop fn [[a:Function s:String acc:List][Map][
  def r (a s)
  if (r.ok) [ (manyloop a/v (r.rest) (push (r.val) acc)) ]
            [ {ok:true val:acc rest:s} ] ]]
def pmany fn a:Function Function [
  ( fn s:String Map [ def z [] (manyloop a/v s z) ] ) ]

def isdigit c:String => [ and (gte "0" c) (lte "9" c) ]
def digit  (psat isdigit/v)
def digits (pmany digit/v)
def ab     (palt (psat (c:String => [eq "a" c])) (psat (c:String => [eq "b" c])))
def two    (pseq digit/v digit/v)

print (digits "123ab")
print (ab "bzz")
print (ab "zzz")
print (two "42x")
```

```
{"ok": true, "val": ["1", "2", "3"], "rest": "ab"}
{"ok": true, "val": "b", "rest": "zz"}
{"ok": false, "rest": "zzz", "val": null}
{"ok": true, "val": ["4", "2"], "rest": "x"}
```

Identical on both engines. It does **not** pass `boru check` — see §5.5.

### 1.6 Everything else that held up

| Capability | Verdict |
|---|---|
| Closures capturing fn-locals, escaping their scope | ✅ `def mk fn k:Integer Function [ def loc (mul 10 k) (fn x:Integer Integer [add loc x]) ]`; `(mk 3)` then called with `1` → `31` |
| Two closures from one factory keep distinct captures | ✅ `(mk 1)` → `1`, `(mk 100)` → `100` |
| Functions in a list, retrieved and called | ✅ `((fs get 1) 5)` → `10` |
| Functions in a map; dynamic key dispatch | ✅ `((tbl get k) 5)` → `10` |
| Pipeline over a list of functions | ✅ `fold [apply] fs 5` → `12` |
| Named `compose` returning a `Function` | ✅ `(compose (a:Integer => [add 1 a]) (a:Integer => [mul 2 a]))` applied to `5` → `11` |
| `memoize` over a captured `flex` cache | ✅ MISS/hit/MISS |
| Mutable capture (`flex` mutated through a closure) | ✅ `[1 2 3]` |
| `usurp` (`/u`) wraps a fn and the wrapper is storable | ✅ `(sub2 10 3)` → `7`; `(sub2/u 10 3)` → `-7`; `def g (f2/u)` then `(g 10 3)` → `-7` |
| Tail recursion, 1 000 000 deep | ✅ constant space; bounded by the 10M **step** limit, not the tape |
| Non-tail recursion, 100 000 deep | ✅ compiled lane: `sum 100000` → `5000050000`. **Re-assessed 2026-08-21:** the interpreter (`-no-compile`) now trips the 396 718-entry tape ceiling near depth 40 000, so this depth passes only under the default lane |
| Non-tail recursion, 1 000 000 deep | ✅ *clean* refusal naming the tape ceiling and the remedy |
| `gensym` | ✅ exists (`LISP-ANALYSIS.5.md` grading it **D** is stale) |

---

## 2. Where boru sits among functional languages

Reading across Haskell, OCaml/SML, Scheme, Clojure, JavaScript, and the
concatenative peers (Factor, Joy):

| Capability | Haskell | Scheme | JS | Factor | **boru** |
|---|---|---|---|---|---|
| First-class functions | ✓ | ✓ | ✓ | ✓ (quotations) | **✓** |
| Lexical closures | ✓ | ✓ | ✓ | ✓ | **✓ for fn-locals; non-locals resolve late (§5.6)** |
| Anonymous lambda syntax | `\x ->` | `lambda` | `=>` | `[ ]` | **`x:T => [body]`** |
| Returning a function | ✓ | ✓ | ✓ | ✓ | **✓** |
| Auto-currying | ✓ | ✗ | ✗ | ✗ | **✗** (manual nesting) |
| Partial application | ✓ | SRFI-26 | `.bind` | `curry` | **mechanism ✓, named combinator ✗** — see below |
| Named `compose` / `pipe` | `.` `>>>` | `compose` | — | `compose` | **✗ — user-written** |
| `flip` | ✓ | ✓ | — | `swap` | **≈ `usurp` / `/u`** (2-arg) |
| Function type in the type system | `a -> b` | — | — | stack effects | **✓ where annotated — `fnsig` (see below); bare `Function` opaque** |
| Polymorphic identity `λx.x` | ✓ | ✓ | ✓ | ✓ | **✓ since `/v` — `t:Any => [t/v]` (§5.2)** |
| Dataflow shufflers (`dip`/`keep`/`bi`) | n/a | n/a | n/a | ✓ | **✗** |
| Laziness / infinite structures | ✓ | promises / SRFI-41 | generators | library | **✗ — strict, materialised** |
| Callback uniformity across containers | ✓ | ✓ | ✓ | ✓ | **✗ (§5.3)** |
| Guaranteed TCO | ✓ | ✓ (required) | ✗ | ✓ | **effectively ✓** (1M deep, constant space) |

Three entries deserve emphasis.

**Partial application exists; the *word* does not.** `curryOrStack`
(`core/go/engine.go:7923`) packages an under-supplied forward call — word
plus collected args — into a value that completes when it is later
expanded, and `lang/go/test/basic.tsv` exercises it across the arithmetic
words (`def add5 word [add 5]` then `10 add5` → `15`). So the row is not
"boru cannot partially apply"; it is that there is no first-class
`partial`/`curry` word taking a `Function` and some arguments and
returning a `Function`.

**`Function` is opaque — where left bare.** At the audit's writing this
read "there is no way to write a function from `Integer` to `String`";
`fnsig` has since closed that where you annotate (see the re-assessment
below). `tpartial` is unrelated (it makes record fields optional). The
cost of a *bare* `Function` is unchanged: a call through it carries no
static information at all — the checker passes it, and the mismatch
surfaces only at run time:

```
$ cat sa.boru
def sa fn x:Function Any [(x x)]  def i t:Any => [t/v]  print (sa i/v)
$ boru check sa.boru
check: 0 error(s), 0 warning(s), 0 info
$ boru run sa.boru
error: [boru/signature_error]: x is still waiting for 1 argument(s) when
  `x` begins its own dispatch — a function word is a barrier and never
  feeds forward collection (strict rule); group the call in parens so its
  RESULT becomes the argument: x (x …)
```

A clean check on a program that cannot run is the shape of the cost.
Compare Haskell's `(a -> b) -> [a] -> [b]`, which makes `map` both
checkable and self-documenting. boru's `map`-equivalent can only say
`Function` — or, now, a named `fnsig` type.

> **Re-assessed 2026-08-21.** `fnsig` merged with the same branch that
> merged this audit (2026-08-19) — the prototype
> `design/FUNCTION-TYPES.0.md` described as unmerged shipped with it,
> so this row was stale on arrival. `def IntToStr fnsig Integer String`
> mints an enforceable fn-shape type: a wrong-shaped argument is
> refused by the checker AND by dispatch (verified under `-no-check`,
> contravariant parameters / covariant return), and the `sa` program
> above, rewritten with `x:AA` for `def AA fnsig Any Any`, is caught at
> CHECK time — the exact clean-check-cannot-run failure this paragraph
> records moves to check time where you annotate. Since NUR095's fix
> (2026-08-20) a fn stored through a named class's fnsig-typed field
> applies on both engines (`lang/spec/class.tsv` §fn-members), with the
> checker still modelling that member apply as inert — NUR096. The
> transcript above still reproduces verbatim: a bare `Function`
> parameter stays opaque, but that is now a choice, not a limit.

**boru's real combinator strength is elsewhere.** The
`usurp`/`stack-args`/`forward-args`/`force-arity` family adapts a
function's *calling convention* and composes — closer to Factor's
`dip`/`keep` algebra than to anything in Scheme, and it has no
counterpart in the ML family. That is a genuine differentiator, and it is
under-advertised relative to the missing `compose`/`curry` staples.

---

## 3. The one rule that explains most of the surprises

ADR-011: *"A function value is always the inert, referenceable thing —
**calling is an act of the use site**. … A bare name bound to a function
calls; a value at the pointer dispatches; `/v` takes the reference."*

That rule is coherent, and it is the source of every §5 hazard. In an
applicative language `f` denotes the function and `f(x)` calls it. In
boru `f` *calls*, and `f/v` denotes. Combinator code is precisely the
code that mostly wants to **denote**.

---

## 4. The idiom sheet

What a higher-order programmer needs, in one place. None of this is
currently in `REFERENCE.md` or `HOWTO.md`.

| Intent | Write | Not |
|---|---|---|
| Pass a function as an argument | `hof f/v xs` | `hof f xs` (works today, but NUR078 will remove it) |
| Return a parameter — **any** kind | `[p/v]` | `[p]` — this **calls** `p` when `p` holds a function |
| Read an *enclosing* fn's parameter | `[p/v]` — `/v` closes over like any name | `(args).N` — the *inner* fn's list, and no longer needed here |
| Apply a computed function inside a body | `def h (g 1)` then `v h/v apply` | `((g 1) v)` — yields two values |
| Callback for `each`/`fold`/`scan` over a **list** | `each [f] xs` (quotation) | `each f/v xs` — no such signature |
| Callback for `filter` over either shape | `filter f/v xs` — the callback gets a `{key value}` pair over a list, a `KeyVal` over a map, never the bare element | — (`filter` is the only one of the five with a list `Function` form) |
| Let a *user-defined* word take a quotation | `hof (codequote [body]) xs` | `hof [body] xs` — evaluated at the call site |
| Computed key for the **quoting** accessor `set` | `set (k) v m` | `set k v m` — silently uses the literal `"k"` |
| Computed key for `get` — and, since 2026-08-21, `has` | `m get k` / `m has k` — both **evaluate** the key (`lang/spec/accessor.tsv`) | — no parens needed; a literal name is `'k'` or `k/q` |

`/v` deserves its own line, because one spelling now covers every
binding kind — a fn (call suppressed), a non-fn (identity), and a
parameter whose kind is not known statically:

```boru
def ident t:Any => [t/v]
print (ident 5)                                   ;# 5
print (ident "s")                                 ;# s
def g (ident (z:Integer => [add 1 z]))
print (g 5)                                       ;# 6
```

It closes over an enclosing fn's parameter like any other name, so the
boxing step the first pass needed is gone:

```boru
def mk fn t:Any Function [ ( fn f:Any Any [ t/v ] ) ]
def c (mk 7)
def d (mk (z:Integer => [add 1 z]))
print (c 0)                                       ;# 7
def g (d 0)
print (g 5)                                       ;# 6
```

`(args).N` still reads a parameter positionally and is still the way to
reach an argument that has no name, but it is no longer the *only* total
read, and the two idioms this audit had to invent — `(args).N` for a
polymorphic slot and boxing for an enclosing one — are both retired.

### 4.1 Forward form reverses the operands

Forward call form is canonical for new code except for the two-argument
words convention reads as infix operators, which house style writes infix
(`AGENTS.md`, `STYLE-GUIDE.md` §S2). The two spellings are not
interchangeable: moving an operand across the word moves it to a
different signature position, so for a non-commutative binary word they
compute **different values**, and nothing in the expression says which
you got:

```
$ boru do 'sub 10 3'    →   -7
$ boru do '10 sub 3'    →    7
$ boru do 'lte 5 1'     →   true
$ boru do '5 lte 1'     →   false
```

`lte x y` is `y ≤ x`; `sub a b` is `b − a`. So "n ≤ 1" is `n lte 1`
infix, or `lte 1 n` written all-forward; "n − 1" is `n sub 1` infix, or
`sub 1 n` all-forward. Transcribing a guard from infix habit into
forward form silently inverts it — a factorial written with `lte n 1`
returns `1` for every input instead of recursing, exit 0. That is not hypothetical:
it happened while converting this note's examples, which is why every one
of them was re-run after conversion.

### 4.2 Signature spellings — six ways to write one signature

Every `fn` in this note uses the fewest square brackets that express its
signature ([`STYLE-GUIDE.md`](../STYLE-GUIDE.md) §S1). For the common
one-parameter, one-return case there are **six** valid spellings, and
they are the same *value*, not merely the same answer:

```boru
def s1 fn x:Integer Integer [mul 2 x]        ;# 1 bracket pair  <- least
def s2 fn x:Integer [Integer] [mul 2 x]      ;# 2
def s3 fn [x:Integer Integer [mul 2 x]]      ;# 2
def s4 fn [[x:Integer] Integer [mul 2 x]]    ;# 3
def s5 fn [x:Integer [Integer] [mul 2 x]]    ;# 3
def s6 fn [[x:Integer] [Integer] [mul 2 x]]  ;# 4  <- fully canonical

print (s1 21) print (s2 21) print (s3 21)
print (s4 21) print (s5 21) print (s6 21)
print (deq (canon s1/v) (canon s6/v))
print (deq (canon s2/v) (canon s6/v))
print (deq (canon s3/v) (canon s6/v))
print (deq (canon s4/v) (canon s6/v))
print (deq (canon s5/v) (canon s6/v))
```

```
42 42 42 42 42 42        (one per line)
true true true true true (one per line)
```

`check: 0 error(s)`. The bodies' brackets are not optional — a body is
always a list — so one pair is the floor.

These eleven rows are no longer only a transcript — both halves are
pinned, so the equivalence is re-checked on every `make test` rather
than only on the day this note was written: the six spellings computing
`42` are spec rows at `lang/spec/fn-triple.tsv` §2b, and the five
`canon` equalities are `TestFnSignatureSpellingsAreOneValue` in
`lang/go/test/fn_triple_compiled_test.go`. They are split because
`canon` of a function value still refuses to compile (a Stage 3 gap, and
so an open defect) and the spec corpus holds `refusalCeiling = 0`.

**A bracketed input is not a longer spelling; it is a different form.**

```
$ boru do 'def f fn [x:Integer] [Integer] [mul 2 x] end f 21'
error: [boru/fn_error]: fn: list length must be a non-zero multiple of 3
  (input output body triples); use `fnsig` for the type-only form, or the
  3-arg form `fn input output body` for a single triple with a non-list input
```

`fn` has two signatures — `[(tnot List) Any List]` and `[List]`. A list
in the input position selects the second, so `[x:Integer]` is read as a
spec list of length 1 and the following `[Integer] [mul 2 x]` strand as
separate arguments. `boru describe fn` states it: *"a list input always
selects the spec-list form."*

**What cannot be reduced**, and so is not a style violation:

| shape | why the spec list is required |
|---|---|
| `fn [[a:Integer b:Integer] [Integer] [add a b]]` | 2+ parameters — the input is a list |
| `fn [[] [Integer] [42]]` | 0 parameters — `[]` is a list |
| `fn [[a:Integer] [Integer] […] [s:String] [String] […]]` | overloads need one list of triples |

Overloads whose inputs are each a single parameter also take the flat
triple spelling, which is shorter: `fn [a:Integer Integer [add 1 a]
s:String String [s]]` builds the same two-overload function.

**`boru fmt` implements this rule only from the fully canonical
spelling** — `fn [[x:Integer] [Integer] [mul 2 x]]` becomes
`fn x:Integer Integer [mul 2 x]`, while `s2`–`s5` above pass through it
untouched. So a file can be `fmt`-clean and still carry four spellings of
one signature. Recorded as **NUR088**.

### 4.3 Curried-lambda spellings — three ways to nest an arrow

Every curried combinator in §1 nests one `=>` inside another, and there
are three ways to write the nesting. They are **not** three spellings of
one value:

```boru
def kk x:Any => [y:Any => [x]]    ;# A — inner lambda in a code list
def kk x:Any => (y:Any => [x])    ;# B — inner lambda in a paren group
def kk x:Any => y:Any => [x]      ;# C — chained arrows, no wrapper
```

All three answer `7` for `((kk 7) 99)`, on the interpreter and the
compiler alike, and check identically. The difference is structural, and
`canon` is what sees it:

```
A  fn [[x:Any][Any][({y:word(Any)} sugar(lambda) [word(x)])]]
B  fn [[x:Any][Any][(({y:word(Any)} sugar(lambda) [word(x)]))]]
C  fn [[x:Any][Any][({y:word(Any)} sugar(lambda) [word(x)])]]
```

**At two levels A and C are the same value** — `deq (canon a/v) (canon
c/v)` is `true` — and **B is not**: its explicit parens are one group
more than the arrow already supplies, so its body canons doubled,
`[((…))]` against `[(…)]`.

At three levels even A and C diverge:

```
A  …[({g:…} sugar(lambda) [({x:…} sugar(lambda) [word(f) (g x)])])]…
C  …[({g:…} sugar(lambda)  ({x:…} sugar(lambda) [word(f) (g x)]) )]…
```

**A wraps each nested lambda in its own code list; C makes the inner
lambda the enclosing body directly.** The two-level case coincides only
because the OUTERMOST body is `fn`'s body slot, which auto-wraps a
non-list body into a one-element list (`ParseFnDef`'s abbreviation rule,
`lang/spec/fn-triple.tsv`). Inner lambda bodies get no such wrap — they
keep whatever the source wrote. So the deeper the currying, the more the
three spellings diverge as values while agreeing as programs.

**Which to write.** C is the fewest brackets (§S1b) and, at two levels,
literally the same value as A. But the equality does not survive a third
level, so "A and C are interchangeable" is true only for the shallowest
case — do not generalise it. This note keeps **A** throughout §1,
because a nested combinator basis is exactly where the equality stops
holding and the bracketed form is the one whose structure matches its
indentation.

**Where the difference bites.** Nowhere in evaluation — but `canon` is
the input to content-addressed identity
(`design/CONTENT-ADDRESSING.0.md`), so A, B and C hash as three
different functions. That is the same class as **NUR074** (`canon`
renders parameter names, so alpha-equivalent functions digest
differently): a purely syntactic choice moving a content hash.

---

## 5. Gotchas, ranked by how quietly they fail

### 5.1 A capitalised name bound to a function never calls — ~~silently~~ **DIAGNOSED 2026-08-25**

```
$ boru do 'def I x:Integer => [add 1 x] end I 5'
I 5
$ echo $?
0
```

> **Re-assessed 2026-08-21.** This stranded pair printed
> `fn (Integer) 5` when the audit was written. Since the type-node
> fusion the bare capitalised name evaluates to the minted lattice
> node, which renders as its own name — so the wrong stack now looks
> like an unevaluated echo of the input, which is quieter still.

`def <Capitalised>` installs a **type** binding, not a value binding
(`eng/go/CLAUDE.md:655`, `core/go/core_type.go::InstallType`,
`lang/go/CLAUDE.md:904`). Given a function body this mints a **predicate
type** — a real and useful feature:

```
$ boru do 'def Even fn n:Integer Boolean [eq 0 (mod 2 n)] end 4 is Even'
true
```

The collision is with convention: the combinator literature is `S`, `K`,
`I`, `B`, `C`, `W`, `Y` — all capitals. A reader transcribing them gets
no error, exit 0, and a wrong stack.

**Workaround:** lowercase names — now the only one. The escape hatch
this audit originally recorded, `5 I/v apply` → `6`, no longer exists:
the valof flip pinned `/v` on a type binding to the minted node for
every declaration kind (`lang/spec/valof.tsv` §11), identical to bare
evaluation, so the same spelling now refuses with
`[boru/signature_error]` — "cannot call `apply` … expected Reach, got
I (an I)" — exit 1, on both engines. The recorded fn body stays reachable
only as the node's *content* (`TypeContentOf`), which has no surface
spelling.
**Severity:** silent wrong answer. **Not a bug** — but it cost a
diagnostic, and that diagnostic has now landed (recommendation 2,
2026-08-25). `boru check` reports the shape as **`stranded_type_call`**:

```
$ boru check -e 'def I x:Integer => [add 1 x] end I 5'
check: 1:34: [warning] stranded_type_call: 'I' names a type, not a function: a capitalised def binds a TYPE, so the call never ran and its operands stayed on the stack
  1 | def I x:Integer => [add 1 x] end I 5
                                       ^
  = note: a def whose name is capitalised and whose body is a fn mints a TYPE (`4 is I` is the intended use); the fn body survives only as that type's content
  = help: lowercase the name to bind a callable function instead
          i
```

The finding is **warning** severity, and `boru do` / `boru run` still
print nothing: `I 5` runs to completion with exit 0, so the codebase's
own severity discipline (`checkCodeSeverity`, `core/go/check_state.go` —
Error is for GUARANTEED runtime failures) puts this in the suspicion
tier, and the default pre-flight is quiet unless it is about to abort.
The transcript at the top of this section is therefore still accurate for
`boru do`; what changed is that `boru check`, `boru run --check`, and
every editor on the LSP surface (which publishes every severity) now name
the mistake and its fix. A project that wants the shape to FAIL has the
existing lever: `boru check --pedantic` exits 1 on any warning.

**How narrowly it fires.** The gate is a bare lattice node whose
DECLARED CONTENT is a function value, immediately followed by a
non-type value, on the top-level straight line
(`check/go/check_recovery.go::noteStrandedTypeCall`, seam
`CheckBraid.NoteStrandedTypeCall` at the end-of-`Run` residual). Keying
on the node's fn *content* rather than on predicate-ness is what makes
the multi-argument combinators visible: `def K fn [[a:Any b:Any][Any][a]]
end` mints a plainly `Function`-parented node, not a predicate type, so
a predicate test would have missed exactly the letters this audit is
about. Four shapes stay deliberately quiet, because a false hint costs
more here than a missed one:

| Quiet on | Why |
|---|---|
| `4 is Even` | the deliberate predicate-type use — `is` consumed the node |
| `4 is Even  Even`, `xs Even` | the node is stranded LAST; nothing followed it to be called |
| `Integer 5`, `def Foo Integer  Foo 5`, `F 5` (a `fnsig`), `C 5` (a class) | the node's content is not a function — naming a type beside a value is ordinary `is`/`typeof` code |
| `[I 5]`, `{a:I b:5}` | boru carries types as data; inside a container VALUE the pair is not a call that failed to happen |
| `I 5 drop`, `def r (I 5)`, `size (I 5)` | **not by design** — a recall limit of the residual scan; see "What it still misses" below |

Judging the top-level residual is not as narrow as it sounds: a paren
group runs on the same tape, and an `if` arm, a `for` body, a `do` region
and a fn body all surface their own residual into it, so
`for [1 2] [I 5]` and `def g y:Integer => [I y]` are caught too.

The name in the message follows what was WRITTEN, not what the node calls
itself: `def J I  J 5` blames `J`, where the caret is. A *computed*
placement (`(valof I) 5`) carries no source token, so there — and only
there — the node's own name stands in, with no position to point at. There
is no `Replacement` on the suggestion, deliberately: the fix is a
COORDINATED rename (the declaration and every reference) and the diagnostic
points at one use site, so a single-token code action applied there would
leave `def I …` standing and turn the call into an `undefined_word` — worse
than no code action at all. And one source defect costs one diagnostic: a
body analysed once per call shape (`def g y:Integer => [I y]  g 1  g 2`)
surfaces the same tokens repeatedly, so the emitter dedupes through
`CheckAddUnique`. Positives and negatives are pinned together in
`lang/go/test/stranded_type_call_test.go`.

**What it still misses.** A residual scan cannot see a pair something else
already ate, and that is a real hole rather than a rounding error:

```
def I x:Integer => [add 1 x] end I 5 drop      # → I          (drop took the 5)
def I x:Integer => [add 1 x] end I 5 print     # → 5, then I  (print took it)
def I x:Integer => [add 1 x] end def r (I 5) r # → 5 I        (def bound one of the pair)
def I x:Integer => [add 1 x] end size (I 5)    # → 0 5        (size consumed the NODE)
```

Every one is §5.1 and none is reported. `def r (I 5)` is the sting: binding
a combinator application to a name is the most natural way to *use* one, and
that spelling gets nothing. Closing this means recording the candidate where
the node is PLACED — the step site knows the following SOURCE token, which is
the fact that actually decides it — instead of reading the finished stack. It
was not folded into the change that introduced the diagnostic: that is a core
step-loop seam with its own false-positive surface to re-validate, and the
shape the audit itself records (`boru do 'def I … end I 5'`) is caught today.
The limit is pinned as a fact by `TestStrandedTypeCallMissesConsumedPair`, so
a fix closes it loudly rather than drifting past it.

**A related declaration-time gap — NUR099.** Reviewing this diagnostic, the
maintainer asked why `def <Capitalised>` accepts a function with an
implementation body at all. Most of that reading does not hold: a fn body
under a capitalised name IS the predicate-type declaration form, and a
non-Boolean body is legal under the **None-on-failure** convention, where the
body returns the value for a member and `None` for a non-member
(`lang/spec/record.tsv` §177 pins `def Positive fn [n:Integer Integer [if (n
gt 0) [n] [None]]]`, return type Integer). `def I x:Integer => [add 1 x]` is
therefore a well-formed predicate that admits every Integer.

What does hold is that `def K fn [[a:Any b:Any][Any][a]] end K 1 2` binds a
type nothing can inhabit and reports nothing. The discriminator that suggests
itself is the parameter count, and that is forbidden — ADR-016 bans
exceptions keyed on arity, ruled absolute by the maintainer on 2026-08-25
(the engine's own `RunPredicate` gate breaks the same rule; recorded as
**NUR100**).

The route that does work goes through the ROOT of §5.1, which is that one
spelling carries two jobs. The same fn body means a callable function under a
lowercase name and a membership test under a capitalised one, and the
capitalised form cannot simply be refused because it is the ONLY way to
declare an arbitrary predicate type: no word in the type-constructing
vocabulary does that job (`refine` declines an Integer base outright, and the
comparison predicates have their own door in `def Big (Integer gt 10)`). The
capital is not something predicates want — it is the only entrance available.

So give predicates their own door. **NUR099**'s verdict, ruled 2026-08-25, is
a **`fnpred`** word analogous to `fnsig`: where `boru describe fnsig` reads
*"a function TYPE — a function minus its body"*, `fnpred` is a predicate TYPE
— a function kept FOR its body. Once it exists, `def <Capitalised>
<fn-with-body>` denotes nothing legitimate and can be refused at the
declaration, with no parameter counting anywhere, and the case-keyed meaning
of a fn body retires with it. Until that lands, `stranded_type_call` remains
the only thing reporting the §5.1 shape.

The same silent-stranding class has since been caught at declaration time
too: a malformed `fn` whose output slot is bare (`def f fn List Any [1]`)
strands its operands and binds nothing, exit 0, where its bracketed-output
twin raises — recorded as **NUR091**.

### 5.2 ~~Naming a `Function`-valued parameter calls it~~ — RESOLVED

**This finding is closed.** It was the audit's headline gap: a bare name
CALLED a function-bound parameter, `/r` REFUSED a non-function one, and
the `x is Function` guard could not be written either because naming `x`
started the call — so no spelling through a parameter's bound name was
total over both kinds, and the identity function had none.

The fix (2026-08-19, this branch): `/r` became **`/v`** and `ref` became
**`valof`**, and the function-only gate was removed. `/v` now denotes the
binding's VALUE whatever kind it is — for a fn binding it suppresses the
call, for anything else it is the identity:

```
$ boru do 'def idf t:Any => [t/v] end (idf 5)'
5
$ boru do 'def idf t:Any => [t/v] end (idf "s")'
s
$ boru do 'def idf t:Any => [t/v] end def g (idf (z:Integer => [add 1 z])) end (g 5)'
6
```

One body, three kinds. The guard is writable now too, because `t/v`
takes the value instead of starting a call:

```
$ boru do 'def idf t:Any => [if (t/v is Function) ["fn"] ["not"]] end (idf 5)'
not
$ boru do 'def idf t:Any => [if (t/v is Function) ["fn"] ["not"]] end (idf (z:Integer => [z]))'
fn
```

And the construction §1.2 could not finish — Church `and`/`or` — now
works in the **naive** spelling, with no boxing and no `(args).N`
(§1.2). NUR085 is retired; `(args).N` still works and is no longer
needed for this.

The remaining edge is unchanged and is not this rule's: a bare `[t]`
still calls a function-bound parameter. That is ADR-011 working as
designed ("calling is an act of the use site"); `/v` is how you say you
meant the value.

### 5.3 ~~`each`/`for-each`/`fold`/`scan` take a `Function` callback over a Map but not over a List~~ — **RESOLVED 2026-08-24**

> **Closed 2026-08-24 (NUR086 retired).** All four now register
> `{TFunction, TList}`, and the LIST form hands the callback the
> **element**: `each dbl/v [1 2 3]` is `[2 4 6]`. The fix reused the
> existing handlers unchanged — `eachHandler` already passed the element
> to `InvokeBody`, and a Function value reaches the callback exactly
> where a quotation body would — so this was a missing signature, not
> missing machinery. The record's second half (a matching `filter`
> decision) is ruled rather than deferred: **a per-container Function
> form hands the container's natural unit; `filter`'s single
> cross-container signature hands a position descriptor**, which is its
> contract because the descriptor carries the index a positional
> predicate needs. Pinned as `lang/spec/higher-order.tsv` §6 and stated
> in `REFERENCE.md` §"Higher-order array words". The audit's "callback
> uniformity across containers ✗" row in §2 is now: uniform where a form
> is per-container, deliberately different for `filter`.
>
> **Two open defects, both narrow and both stated rather than hidden.**
> The list Function form reaches its callback through `InvokeBody`, which
> the lowering ISLANDS rather than models, so those rows are ledgered in
> `frontier-hof-audit.tsv` §12 — the fix buys the spelling and leaves the
> rows uncompiled, with graduation to a modelled fn-value callback frame
> still owed. And a LAMBDA over a *gradual-Any* collection now refuses to
> compile where it used to: with two `TFunction` overloads reachable, the
> compiler cannot commit, because the callback gets the ELEMENT over a
> list and a `KeyVal` over a map — so a closure compiled against either
> shape is wrong for the other. Not compiling it is the only containment
> available today, and it is a defect either way — the modelling that
> would tell the two shapes apart is missing. The default lane runs the
> program on the interpreter with the loud warning
> (`lang/go/bytecode_gradual_each_test.go`, the refusesAndFallsBack
> group).

```
$ boru do 'def dbl x:Integer => [mul 2 x] end each dbl/v [1 2 3]'
error: [boru/signature_error]: cannot call `each` — no signature matches the arguments
  = note: candidate `each (Function, Map)` takes 2 arguments, but none were supplied
  = note: candidate `each (Reach, List)`  takes 2 arguments, but none were supplied
```

At source: `each` registers `{TFunction, TMap}` and no `{TFunction,
TList}` (`lang/go/native/native_array.go:326`); same for `for-each`
(`:348`), `fold` (`:393-394`) and `scan` (`:420`). `filter` — alone —
registers `{TFunction, TAny}` and so has a list `Function` form
(`lang/go/native/natives.go:261`).

Neither `Function` form is a plain element callback, though: `each`'s map
form hands the lambda a `KeyVal`, and `filter`'s list form hands it a
`{key, value}` pair (`lang/go/native/filter.go`). So an
`Integer -> Integer` function value cannot be handed directly to *any* of
the five — only a quotation naming it can.

**Workaround:** `each [f] xs`, or `each (kv:Any => [f (kv.v)]) m`.
**Severity:** confusing error; a learnability tax on the most-reached-for
words. Recorded as **NUR086**.

### 5.4 Applying a computed function value depends on the enclosing context

The same expression means two things:

```
$ boru do 'def mk fn a:Integer Function [(fn b:Integer Integer [add a b])] end ((mk 1) 2)'
3
$ boru do 'def mk fn a:Integer Function [(fn b:Integer Integer [add a b])] end print ((mk 1) 2)'
fn (Integer)
2
$ echo $?
0
```

`(mk 1)` *resolves* a function; the **call belongs to whatever encloses
the group** (`lang/spec/fn-value.tsv` §4, NUR035). At statement level the
statement applies it; under `print` the argument window merely collects
both. Inside a fn body the same shape yields a return-arity error:

```
error: [boru/type_error]: g: expected 1 return value(s), got 2 — [fn (Integer) 2]
```

(The prefix names the enclosing fn — `g` here, empty for an anonymous
one — and the message now carries a source span pointing at the group
and the declaration, `core/go/return_check_msg.go`. It read `ap:` when
the audit was written; same class, same payload.)

> **Corrected 2026-08-24 (completeness review).** These transcripts are
> the COMPILED lane's, and always were: under `-no-compile` the
> interpreter re-steps the collapsed fn in every context on this page —
> `print ((mk 1) 2)` answered **3** on the audit-day tree (`e332d15`)
> and answers 3 today — so "the argument window merely collects both"
> recorded a silent engine divergence (NUR073's class, where §0 counted
> one such divergence), not a lane-independent context rule. Since the
> §9g guard (`12c8150`) the print shape FAILS to compile and the
> lanes agree on 3 — `boru run` behind its loud warning, `boru do`
> (the command these transcripts use) routed to the interpreter
> SILENTLY, which hides the failure rather than excusing it,
> `-force-compile` refusing with a `force-compile` error — so the
> quoted `fn (Integer)` / `2` reproduces only on a pre-guard tree.
> The fn-body arity error above is likewise the compiled lane's, and
> that row is LIVE and check-clean today: interpreted `(g 0)` answers
> 3, exit 0, where the checked default raises the quoted error, exit 1
> — recorded as widened evidence on NUR073. Two attributions move with
> this: the "silent wrong answer under `print`" severity belonged to
> §5.7's engines-disagree class, and the Z explanation below holds for
> the compiled lane only — the interpreter applies exactly the
> `((x x) v)` shape, so the interpreter-lane divergence of §1.4's Z
> (whose source this note does not quote — the one §1 program that
> cannot be re-run from the page) has no established cause here.

On the COMPILED lane this is why the Z spelling cannot work: `((x x)
v)` never applies there, so the `λv` guard is inert. The interpreter
applies that shape, so its divergence (§1.4) has a different, still
unestablished cause — see the correction above.

**Workaround:** `def h (mk 1)` then `2 h/v apply` → `3`.
**Forward hazard — now realised (2026-08-24).** NUR073's accepted
**BROAD** verdict removed inline application entirely — *"`(fn Integer [Integer] [10 add]) 7` becomes two
values and the inline-application idiom is removed"*. That idiom
currently answers `17`. Combinator code written against today's top-level
behaviour will change meaning when the fix lands.

> **Re-measured 2026-08-25, post-BROAD and post-#402 — the severity moves
> and one NEW divergence falls out.** Every shape on this page was re-run
> on both lanes against the merged tree.
>
> **The `print` divergence is gone.** `print ((mk 1) 2)` answers
> `fn (Integer)` then `2` on BOTH lanes now, so the recorded severity
> ("silent wrong answer under `print`") and its NUR073 attribution are
> discharged. What remains there is not a defect but ADR-011's rule
> working as written — *calling is an act of the use site*. A placed fn
> dispatches when it is STEPPED at the pointer and not when an argument
> window merely collects it, which partitions cleanly:
>
> | applies | does not apply |
> |---|---|
> | statement level; `def r ((mk 1) 2)`; `((mk 1) 2) add 0`; `2 (mk 1) apply` | `print ((mk 1) 2)`; `[((mk 1) 2)]`; `add 0 ((mk 1) 2)` — which errors loudly |
>
> **The fn-body row is still live**, exactly as the 2026-08-24 note
> records: interpreted `(g 0)` answers 3, the checked default raises
> `type_error: expected 1 return value(s), got 2`.
>
> **And BROAD turns out to be only half implemented — NUR101.** The
> partition above is not a designed use-site rule after all. ADR-011's
> amendment says a paren places its collapsed Function *"reference and
> inline literal alike"*, and it does — but NOT when the group COMPUTES
> one:
>
> ```
> (fn Integer [Integer] [10 add]) 7   → fn (Integer) 7      inline literal: places ✓
> (inc/v) 2                           → fn inc(Integer) 2   reference: places ✓
> (mk 1) 2                            → 3                   COMPUTED: dispatches ✗
> (if true [inc/v] [inc/v]) 2         → 3                   COMPUTED: dispatches ✗
> ```
>
> So the `((mk 1) 2)` → `3` transcripts at the head of this section record
> the UNFIXED behaviour, not a rule: the specified answer is `fn 2`, and
> when the computed half of BROAD lands these transcripts and the
> `def h (mk 1)` / `2 h/v apply` workaround will need re-spelling, exactly
> as every §1 program did when BROAD's first half landed. `def h (mk 1)`
> then `h 2` → `3` stays correct throughout — a bare NAME bound to a
> function calls, by rule.
>
> The symptom that surfaced it: `[((mk 1) 2)]` is `[3]` interpreted and
> `[fn (Integer) 2]` compiled, silently — exit 0 both ways, `boru check`
> clean. The COMPILED answer is the specified one; the interpreter carries
> the placement bug into container evaluation. Fixing the placement rule
> closes the divergence with it, and wants no compiler change.

**Severity:** ~~silent wrong answer under `print`~~ — that lane divergence
is closed (see above). What is left is **NUR101**: BROAD placed a referenced
or literal fn but still dispatches a COMPUTED one, so this section's
`(mk 1) 2` → `3` transcripts record unfixed behaviour rather than a rule,
and the same gap shows as a silent lane divergence one container deeper.

### 5.5 ~~The checker rejects correct higher-order programs~~ — **RESOLVED 2026-08-24**

> **Closed 2026-08-24 (NUR087 retired).** The repro below, and the §1.5
> library in its ORIGINAL spelling, now check clean and run under plain
> `boru run`. Root cause, and it is narrower than this section's framing:
> the `if` was never the trigger. `checkModeParenFnCollapse` — the
> machinery built to close exactly this def-split false-positive class —
> required the call's ARGUMENT to be non-dynamic, so `def r2 (b (r1.rest))`
> (a call through a `Function` param whose argument is an earlier dynamic
> binding) did not collapse, the pending `def` collected nothing, and every
> later read of `r2` raised a false `undefined_word`. A dynamic argument
> makes the result no less knowable than a static one — the window
> collapses to `dynamic(Any)` either way — so the restriction bought no
> soundness. Pinned as `lang/spec/fn-value.tsv`'s chained def-split rows,
> flat and branch-local.

### 5.5 (as recorded) The checker rejects correct higher-order programs, and `boru run` obeys it

Minimal repro — a `def` inside an `if` branch becomes invisible once an
earlier `def` in the same body bound the result of a call **through a
`Function` parameter**:

```boru
def mk fn [[a:Function b:Function][Function][
  ( fn s:Integer Map [
      def r1 (a s)
      if (r1.ok)
        [ def r2 (b (r1.rest))
          if (r2.ok) [ {ok:true val:[(r1.val) (r2.val)] rest:(r2.rest)} ]
                     [ {ok:false rest:s val:None} ] ]
        [ {ok:false rest:s val:None} ] ] ) ]]
def h (mk (z:Integer => [ {ok:true val:1 rest:8} ])
          (z:Integer => [ {ok:true val:2 rest:9} ]))
print (h 1)
```

```
check: 6:15: [error] undefined_word: undefined word: r2
  = help: did you mean `r1`?
```

It runs correctly (`{"ok": true, "val": [1, 2], "rest": 9}`). Remove
`def r1 (a s)` — replace it with a literal — and the check passes. The
parser-combinator library of §1.5 hits this: plain `boru run` on it
**refuses to run it**, `check failed: 6 error(s)`, exit 1. `-no-check`
runs it correctly.

**Workaround:** `-no-check`, or hoist the binding.
**Severity:** a correct program is refused by the default invocation.
Recorded as **NUR087**. Post-audit, the checker-vs-runtime family
gained a member one layer up: NUR095's fix (2026-08-20) made a fn
stored through a fn-shape-typed class member apply in compiled
execution, but the check pass still models that member apply as the
inert pre-fix stack — recorded as **NUR096**.

### 5.6 Non-local names in a closure resolve late, through the def stack

```
$ boru do 'def n 1 end def c x:Integer => [add n x] end def n 100 end (c 5)'
105
$ boru do 'def n 1 end def c x:Integer => [add n x] end def n 100 end undef n end (c 5)'
6
```

fn-**locals** are captured properly and outlive their scope (§1.6). Names
from the enclosing module scope are not captured — they are resolved at
call time against the shadowing stack. This matches JavaScript's
behaviour for a reassigned outer binding, but `def n 100` is a *shadow
push*, not an assignment, so an ML/Haskell reader expecting `6` gets
`105`.

**Severity:** confusing; matters whenever a combinator is defined before
a name it mentions is re-`def`ed. Recorded as **NUR097** (2026-08-21),
with the proposed verdict *Allowed plus an in-file diagnostic* — the
late half is the top-level liveness contract, not a defect. The
freezing idiom is parameter capture:
`def mkc fn nn:Integer Function [(fn x:Integer Integer [add nn x])]`
then `def c (mkc n)` → `6` however `n` is later re-`def`ed. All three
behaviours (105, the post-`undef` 6, and the frozen 6) are pinned as
`lang/spec/frontier/frontier-hof-audit.tsv` §8, and the contract is
documented in `REFERENCE.md` §"Definition and scoping".

### 5.7 The engines disagree — NUR073, ~~live today~~ **RESOLVED 2026-08-24**

> **Closed 2026-08-24.** The BROAD verdict's clause-3 fix landed: every
> paren PLACES its collapsed Function (the reach-group exclusion stays,
> so dot access still dispatches). `((h z/v))` is now the held value on
> BOTH engines, pinned through `canon`/`typeof` in
> `lang/spec/fn-value.tsv` §4b — the bare residual keeps a
> name-rendering difference that belongs to NUR074's class, which is why
> the pin is not on it. The transcript below is the pre-fix record; the
> "choice of engine is a choice of semantics" claim it makes no longer
> holds for this shape, and with it §0's "one live engine disagreement"
> line is discharged.

```
$ cat n73.boru
def z fn [[][Integer][42]]  def h fn f:Function Any [f/v]  ((h z/v))

$ boru run -no-check -no-compile n73.boru     # interpreter
42
$ boru run -no-check -force-compile n73.boru  # compiler
fn z
$ boru run n73.boru                           # the plain default — exit 0
fn z
```

`NUR.md` records the divergence as *"the only such row in the corpus"*;
what its text does not say is that the **default** invocation now sides
with the compiler, so `-no-compile` and the default disagree, silently,
exit 0 both ways. For higher-order code the choice of engine is a choice
of semantics.

**Severity:** highest. Two supported invocations of the same program
print different answers with no diagnostic.

### 5.8 Compilation refuses nearly all curried higher-order code

Every curried combinator in §1.1–1.2 draws the same refusal:

```
warning: bytecode compilation refused, ran on the interpreter (slower):
  fn ss: body result of unknown provenance
```

Other observed reasons: `fn-value application bounded by a paren (dynamic
value precedes args)`, and NUR037's fn-local-fn-as-body-word refusal.

Each of these is a **defect** — an unimplemented or unproven case in the
compiler, owed a fix and tracked to closure — and not a sanctioned
outcome: done is a language that compiles, all valid code, no
exceptions. Unlike §5.7 the failure is at least *announced* on stderr
rather than hidden. But at this density it means higher-order style does
not compile as a rule, not as an exception. Worth stating plainly in the
docs — not as a performance trade-off a user chooses, but as the size of
the hole still owed a fix.

> **Re-measured 2026-08-25 — the headline above is no longer true.**
> Thirteen higher-order shapes were run against the post-#402 tree. NINE
> compile. "Does not compile as a rule" was accurate when written; it is
> not the shape of the boundary now, and the two lines that matter most
> — the closure factory and the curried arrow, the very spellings this
> section quotes as always refusing — are on the compiling side.
>
> | compiles today | |
> |---|---|
> | `each` / `fold` / `filter` with a `/v` callback | `each dbl/v [1 2 3]` |
> | an inline lambda callback | `each ([x:Integer] => [mul 2 x]) [1 2 3]` |
> | `apply` | `5 inc/v apply` |
> | a **closure factory** | `def a5 (mk 5)` … `a5 3` |
> | a **curried arrow** | `def g (f 2)` … `g 3` |
> | a fn held in a map, read and applied | `5 (m.f) apply` |
> | a fn returned from an `if` branch | `5 (if true [i/v] [d/v]) apply` |
>
> What still refuses is narrower and falls in three named classes:
>
> | refuses | reason |
> |---|---|
> | a function-valued operand to a MODULE word — `FnUtil.compose`, `FnUtil.partial` | `function-valued operand at compose (Stage 3)` |
> | self-application (the Y shape) | `body result of unknown provenance` — §5.8's original signature |
> | deep combinator chains (S) | `unmatched dispatch recovered at apply` |
>
> The first is the one that bites: `boru:fn-util` is the module
> recommendation 4 shipped for exactly this style (`compose`, `pipe`,
> `curry`, `partial`, …), and none of it compiles — so the vocabulary the
> audit added to make higher-order code pleasant is also the vocabulary
> guaranteed NOT to compile. That is the highest-value defect left on
> this page.
>
> Corpus-wide the picture is already better: the main spec set compiles
> **7182 of 7182** compilable rows with 0 islanded, and what does not
> compile is isolated in ledgered frontier files. The hole this section
> asked to be stated plainly is therefore much smaller than it was —
> three identifiable shapes rather than higher-order style as such — but
> smaller is not closed, and each of the three is still a defect owed a
> fix.

**Stage 1 landed (2026-08-21) — the def-bound computed-fn read.** The
compile lane's false `undefined_word` on a name def-bound to a computed
fn (`def h (mk 1)  (h 2)` — the strict check's carrier binding installs
no `Defs` entry, so the read looked undefined where the CLI check and
the interpreter both succeed) is resolved: on a compile pass, `stepWord`
consults the per-pass fn-carrier side table (moved down to
`core/go/check_fncarrier.go`) and substitutes the carrier with the same
use + `NoteDefRead` provenance a def-value read records. The §5.6
freeze-idiom row (`def c (mkc n) … (c 5)`) now **compiles natively with
parity** — the first graduation in this family. Three companion
soundness guards shipped with it, each closing a hole the substitution
exposed (each was a would-be miscompile caught by the frontier parity
harness):

- a `/v` read of a table-bound name deliberately KEEPS the diagnostic
  (`stepWordVal` declines the table) — substituting there green-lit a
  lowering that dropped the operand;
- the unmatched-dispatch trap declines a window naming a table-bound
  word (`TryRecordUnmatchedDispatchTrap`): at run time that name IS
  bound, so the static no-match is a modeling artifact, not a definite
  runtime failure (the pmany/pseq rows trapped `signature_error` where
  the interpreter succeeds);
- the leading fn-carrier apply refuses a READ-substituted lead whose
  closure shape is not statically known (`resolveDynamicApply`): the
  interpreter word-dispatches such a read, while `OpCallDynamic`'s
  island runs anonymous-VALUE semantics — for a named Go-impl fn value
  (`FnUtil.const`'s result) those diverge (ADR-016's data-vs-call edge:
  `((FnUtil.const 7) 99)` is 99 in BOTH engines, but
  `def k (FnUtil.const 7)  (k 99)` is 7 interpreted). A compiled-factory
  producer (statically known closure arity) stays lowered.

A fourth guard closes the rematch window the same way
(`RecordDispatchRematchValues` declines a read-substituted fn-carrier
window value: `def q (if c [fn-arm] [fn-arm])  add 1 (q 5)` re-matched
`add` over `[1, fn, 5]` and raised where the interpreter computes 7).
And the escaping-closure factory itself graduated: `def a5 (mk 5)
a5 3` — the §5.4 make-adder, called once through its binding —
compiles natively with parity (repeated reads still refuse at the
fn-value residual nets).

**Every non-graduating row still fails to compile, and the failure stays
hidden.** Refusals are loud by policy (`compile_refused`), but every
program in this family refused behind the SILENT check-diagnostics
sentinel before Stage 1 — so a pass that substituted a carrier read marks
itself (`CheckState.FnCarrierReadSubstituted`), and a refusal from such a
pass keeps routing the program to the interpreter SILENTLY, the precise
reason surfaced only as the CLI's performance warning. That silence is
the worst of it: a failure that hides itself is worse than one that
shouts. The census suites classify this transitional class with the
sentinel; the frontier compile ledger tracks its precise per-row reasons,
and every row it tracks is an open defect.

The remaining rows in the family moved one or two stages later, each to
an emit-land refusal that is still a refusal (`frontierCompileLedger`
records the exact strings): the Stage 3 function-valued-operand gate (the
fn-util combinator rows), the Stage 2 single-result-branch rule (the
U-combinator), capture-bearing `body result of unknown provenance`
(compose, palt), and the guards above.

**Stage 2 (2026-08-21) — the closure-flag split, landed with its
witness.** `fnUnitRec.lambdaUnit` distinguishes a returned lambda's own
unit (word `"fnval"`, a real named-param frame with capture slots) from
a native code-body unit (each/do$body, a CallableSpec-input frame) —
the split the campaign brief identified — and `DynApplyLeadEligible`
now admits a lambda unit's own slots. The end-to-end witness (pinned in
`frontier-hof-audit.tsv` §9): the **apply-the-capture factory**

```
def mkc2 fn [[g:Function][Function][( fn [[v:Integer][Integer][(g v)]] )]]
def h2 (mkc2 (z:Integer => [add 7 z]))
(h2 5)                                # → 12, compiled natively
```

compiles with parity — without the admission the inner `[g, v]`
residual count-refused the fnval probe and the whole factory refused
`body result of unknown provenance`. The probe battery around it:
repeated reads (`(h2 5) (h2 10)`) and multi-instance factories stay
refusals, ledgered as defects (`fn value precedes residual args`); the
0-arg-apply-of-a-1-arg-capture spelling refuses where the interpreter
raises (the interpreter re-run raises the identical error); a `g:Any`
data capture never reaches the admission (not a Function carrier).

**Capture reachability at call sites — landed (2026-08-21, the second
Stage-2 increment).** A CONCRETELY-installed factory closure
(`def h (mkap …)  (h 5)`, the `=>`-inner spelling — the analysis yields
a concrete FnDefInfo whose Captured are construction-scope carriers)
used to refuse `capture g of h unreachable at a call site`: the unit
call re-resolves its captures per call site, where they are
meaningless. The call now lowers as a fn-VALUE apply through the DEF
SITE's recorded operand (`RecordDynApplyName` reads the name's
`evDynBind` event — the factory call's out, a promoted local carrying
the `OpPushClosure` result whose baked captures `invokeClosure`
installs VM-native). Two soundness pieces found by the probe battery:

- the first route tried (`BIND_DYN_SCOPE` + `OpLookupDynScopeData`)
  silently broke — `bindDynScope` → `InstallDef` DECLINES a
  ClosurePayload value (the fn arm requires FnDefInfo), so the bind
  no-opped and the lookup found the stale check-pass binding, whose
  token body islanded without captures (`undefined_word: g` at run
  time). The def-site-operand route avoids the def table entirely;
- the memoised body analysis returns the SAME residual value (same ID)
  for every call of one shape, so per-call outs must FRESHEN or
  `producedBy` overwrites and every residual slot resolves to the last
  apply (`(h 5) (h 10)` compiled `[17 17]` for the interpreter's
  `[12 17]`). `recordFnValueApplyFallback` mints a fresh carrier per
  site and substitutes it into the dispatch result.

The gates, each load-bearing: single arg + single CARRIER out;
anonymous, unquoted, single-own-sig binding; at least one
non-concrete capture (fully-concrete-capture units keep the unit
call). Pinned in `frontier-hof-audit.tsv` §9b: single call, repeated
calls, rebind-between-calls ordering, and two factory instances all
compile with parity; `((kk 7) 99)` (no def binding — no def-site
operand) stays the ledgered §4.3 refusal.

**The event-lead trailing apply — landed for proven arities
(2026-08-21, §9c).** `RecordDynApply`'s event-provenance hard-refusal
("runtime quote state unknown") is retired where the window is proven:
a new op, `OpCallDynTrailKeepQ`, preserves the runtime quote state (no
read-substitution strip — a callee returning `quote (fn …)` stays
inert in BOTH engines, an unquoted anonymous result applies in both),
and the record admits it only when the callee's arity provably equals
the window (`producerReturnedClosureArity`, or a concrete single-sig
proof). `(2 (mk 4))` compiles natively (frontier §9c); the wider
window `(1 2 (mk 4))` — where the interpreter under-applies and the
deeper value survives — keeps the refusal (ledgered), as do quoted
and carrier-lead spellings without an arity proof.

**The Church chain's blocker, located (2026-08-21).** The stage above
named the lever as "an arity channel for param-typed fn carriers". The
probe battery says otherwise, and the correction matters because the
arity work would not have moved these rows. Bisecting the family down
to its smallest member isolates ONE discriminator — the inner lambda's
parameter TYPE:

```
def app g:Function => [x:Integer => [(g x)]]   def h (app …)  (h 5)   # compiles (§9)
def app g:Function => [x:Any     => [(g x)]]   def h (app …)  (h 5)   # refuses
```

Everything else — the factory, the capture, the call site, the lead's
admission through `DynApplyLeadEligible` — is identical and already
lands. What refuses is `parenLeadFnApplyIdx`'s **argument** gate
(`last.Dynamic || IsFnValueResidual(last)`), and it is load-bearing:
the leading and trailing spellings converge only while the argument is
not a function. A FUNCTION-valued argument is never applied by the
interpreter — its leading collection meets a function word, a barrier
that never feeds forward collection, and RAISES — where the trailing
model the window records binds and applies. A gradual (`Any`) argument
cannot be proven non-function, so it is excluded with the static case.

Dropping the gate compiles the whole `x:Any` family, including the
one-level rows above, which reads as a graduation and is a miscompile
waiting on its first function-valued argument. It cannot be repaired at
run time, which is the part worth recording:

- the interpreter's raise is a property of **word dispatch**, not of
  the values. An island over the resolved window `[lead, fnArg]` — the
  faithful-by-construction move everywhere else in this campaign —
  leaves both inert instead (probe: the residual comes back
  `fn (Integer) fn (Integer)`, no apply and no error);
- and the raise has **two** texts — the stranded-forward barrier
  (`g is still waiting for 1 argument(s) when x begins its own
  dispatch`) when the lead parked a forward, the lead's own no-match
  (`cannot call g — no signature matches the arguments`) when no
  overload could — selected by engine-internal collection state the
  compiled window does not carry. A single-sig lead takes the first, a
  multi-overload lead the second, and the lead here is a CARRIER, so
  neither is provable at record time.

The obvious next idea — replay the window at WORD level, since the
recorder does know the argument's name (the unit's slot→name `locals`
table) — was probed too, and it does not close the gap either. Islanding
`[lead, Word("x")]` with `x` bound to a function raises, but blames a
THIRD target (`cannot call x — no signature matches the arguments`: the
argument's own dispatch fires with no arguments), because the raise the
interpreter produces depends on the enclosing frame and the lead's parked
forward state — context no island reconstruction carries. Value island:
inert. Word island: wrong blame. Both are recorded here so the next
attempt does not re-derive them.

So the refusal stays — an unclosed defect rather than a resolution — and
it now stays **pinned** rather than incidental:
`TestS5BParenLeadFnApplyIdxGradualArgDeclines` (core) fails if either
clause is dropped, the gate's comment carries the reasoning, and
`frontier-hof-audit.tsv` §9d ledgers the family so it graduates
automatically when the shape is genuinely solved.

**A Stage 1 regression, found and closed (2026-08-21, §9e).** Probing
the nested-curried-residual stage turned up a SILENT MISCOMPILE in the
default lane — the one class this campaign must never produce:

```
def mk2 fn [[a:Integer][Function][( fn [[b:Integer][Function][( fn
  [[c:Integer][Integer][add a (add b c)]] )]] )]]
def f1 (mk2 1)
def f2 (f1 2)
(f2 3)                  # interpreter 6; compiled `2 fn (Integer) 3`
```

Bisected to `e48e5dd` — Stage 1's own read substitution. Before it, the
`f1` read raised a false `undefined_word` and the program refused;
making the read honest let the analysis reach `def f2 (f1 2)`, which it
cannot model: it returns the CALLEE unchanged, so `f2` binds the very
carrier `f1` denotes. Compiled, both names take one slot
(`BIND_GLOBAL g0` and `g1` over the same `l0`) and the apply's
unconsumed `2` leaks into the top-level residual.

The def site now detects exactly that shape — a bind whose value is
already table-bound under another NAME is a dropped apply, and nothing
else, since a legitimate alias cannot reach it (`def g f1` is a
strict-barrier syntax error; `def g f1/v` resolves through `Defs`
without consulting the table) — and refuses. That removes the
miscompile and leaves a defect in its place: the shape does not compile
at all, and its routing to the interpreter is silent, exactly as it was
on main (main refused this program too, behind the silent
check-diagnostics sentinel). Nothing else was lost: the two-level
chain, the chained spelling, multi-instance factories and the `/v`
alias all still compile, and a six-shape differential sweep plus the
frontier corpus agree across lanes. `frontier-hof-audit.tsv` §9e
ledgers the shape.

The lesson generalises to the rest of this campaign: making a read
honest moves programs from "refused" into "modelled", and every shape
that arrives there needs its model CHECKED, not assumed. Stage 1's four
guards were written against the shapes its probe battery reached; this
one it did not reach, because the chain needs two def-bound levels
before the aliasing becomes visible.

**Three more, from a differential sweep (2026-08-21, §9f).** Acting on
that lesson, a thirty-shape sweep — every def-bound-computed-fn program
the surrounding vocabulary can spell, each run on both engines and
diffed — found three further divergences, all in ONE context: a **code
body**. A code body is re-run by its native through the INTERPRETER,
and neither of this branch's admissions survives that:

| Program | Compiled | Interpreted |
|---|---|---|
| `def f (mk 1)  each [1 2 3] [(f 1)]` | `[3 3]` | `[3]` |
| `def f (mk 1)  do [(f 2)]` | raises `undefined word: f` | `3` |
| `def h (mkg …)  do [(h 1)]` | raises `undefined word: g` | `8` |

The first two are Stage 1's read substitution reaching a body TOKEN
rather than an operand: `each`'s body assembled as the DATA list
`[f, 5]` (an `OpMakeList`), taking each's own input list with it. The
third is Stage 2's lead-apply admission (`3d914ad`) leaving a compiled
`ClosurePayload` where `do`'s re-run must apply it — and a
ClosurePayload is invokable only through the VM's re-entrant runner,
never the interpreter (`payload.go`'s own contract, plan P2), so the
re-run reached the closure's token body with no captures installed.

Three guards, each at the narrowest point that catches its shape
without costing a graduation:

- the substitution declines inside a nested body
  (`CheckState.NestedBodyDepth > 0`) — returning this class to its
  pre-Stage-1 refusal, still SILENT: a miscompile traded for a compile
  failure that hides itself, not for a fix;
- `RecordMakeListInner` refuses a list whose member is a table
  carrier — the corruption's list-assembly twin, which no nesting
  counter sees because `each`'s body analyses at depth 0;
- `recordDispatchOutcome` refuses a code-body argument that READS a
  name dyn-bound to a compiled closure. Scoped to the read, not the
  bind: a blanket refusal at `RecordDynBind` was tried first and
  unwound the whole §9b family, which applies exactly such closures
  from compiled code quite happily.

All four sweeps and the full frontier corpus are clean, and §9b, §9c
and the Stage 1 graduations all still compile. `frontier-hof-audit.tsv`
§9f ledgers the three shapes.

**The widened sweep, and two more (2026-08-21, §9g).** The §9f note
ended by saying the ~30 hand-picked programs were not exhaustive. They
were not. A GENERATED sweep — the cross-product of factory spelling
(verbose `fn`, arrow, capture-taking, gradual-parameter, three-level) ×
binding shape (plain, rebind, two instances, `/v` alias) × 23
consumption contexts (top level, operand, nested paren, def-local, `if`
arms, `case` arms, `for`, `while`, `do`, `each`, `map`, `filter`, list
literal, map value, fn body, user-fn argument, `apply`, `typeof`, …) —
is **690 programs**, and it found **24 divergences** the hand-picked set
missed, in exactly two contexts:

```
def h (mk (z:Integer => [add 7 z]))
typeof (h 5)                # interpreted Integer; compiled `Function 5`
filter [1 2] [gt 0 (h 5)]   # interpreted: body not Boolean
                            # compiled:    cannot order Function and Integer
```

Bisected to `3d914ad` — but unlike §9e/§9f the defect is not *in* that
commit. Before the Stage 2 admission these programs refused outright
(the factory's inner unit did not compile), so the top-level modelling
was never exercised. The admission **unmasked** it. The disassembly is
unambiguous:

```
(h 5)          PUSH_LOCAL h ; PUSH_CONST 5 ; CALL_DYNAMIC /1   ← applies
typeof (h 5)   PUSH_LOCAL h ; CALL_NATIVE typeof ; PUSH_CONST 5
```

The paren never collapsed into an apply. The bare spelling survives only
because `Finalize`'s `resolveDynamicApply` lowers the leftover
program-residual; consume that residual — hand it to a word — and the
apply is simply lost. An `Any`-typed slot then swallows the FUNCTION and
strands the argument behind it, which is why exactly `typeof` and
`filter`'s `gt` surfaced it: a slot that type-checks a Function accepts
the wrong operand silently, where `add`'s numeric slots reject it.

`argIsProducedClosure` refuses a dispatch whose argument is a closure
this pass produced (`producerReturnedClosureArity`). A word with a
genuine `Function` slot is unaffected — its argument is not one of these
produced closures. Re-swept: **690 programs, 0 divergences** (535 of them
running to a value, not merely agreeing on an error), the two earlier
sweeps clean, and the frontier corpus green with no graduation lost.
Ledgered as §9g.

**A sixth, from review (2026-08-21, §9h).** Both P1 findings of the
PR #397 Codex review were real, and both are instances of ONE fact: a
computed fn is not installed in `Defs` — the compiled closure machinery
owns the name — so it lives only in the check-pass carrier table, and
the two binding stores can disagree.

- **`undef` left the table behind.** It popped `Defs` and never dropped
  the carrier, so a later read resolved a binding that was gone.
  `DropCheckFnCarrierBind` fixes it, pinned directly.
- **Shadowing gave the name two meanings**, which is the half the
  reviewer's own repro did not reach and a probe did:

  ```
  def f 1 ;  def f (mk 1) ;  undef f ;  (f 2)
  interpreted 3    compiled `1 2`
  ```

  The compiled program bound only the shadowed `f = 1` (the computed
  `def` installs nothing), so the pop exposed it and stranded the `2`.
  That is not repairable by dropping the table entry — the compiled
  lane never had the closure under that name at all — so a computed fn
  shadowing a live binding now refuses.

Worth recording about the review itself: the reviewer's stated repro
(`def f (mk 1) ; undef f ; (f 2)`) does **not** reproduce, because
`undef` does not remove a fn binding at all — both lanes answer 3. The
FINDING was right and the REPRO was wrong, and taking the repro at face
value would have closed it as unreproducible. Whether `undef` should
remove a fn binding is a separate language question, untouched here;
the fix is correct whichever way it is eventually settled.

The wider point for whoever picks this up: **every admission in this
campaign needs a code-body probe — and a generated sweep, not a
hand-picked one.** All four classes share one signature — an admission
sound where the value is an operand, applied where the value is a token
— and the fourth was found only because the sweep was mechanised. Two of
the four (§9f's `do`-body closure read, §9g) are not bugs the admitting
commit wrote; they are shapes it made REACHABLE. Admitting a shape is
therefore never a local change: it promotes a whole population of
programs from "refused" to "modelled", and that population is what has
to be swept.

Remaining stages, each its own probe-driven increment:

1. **The gradual-argument lead window** (§9d, above) — the Church
   chain's real gate. Needs a way to answer the interpreter's
   word-dispatch question from a compiled window: either a proof at
   record time that the argument slot cannot hold a function (a
   non-`Any` bound, or a whole-program flow fact), or a lowering that
   reproduces the interpreter's FRAME — not merely its tokens: the
   word-level island above shows tokens alone are not enough, because
   the blame target follows the parked forward. The arity channel the
   previous draft proposed is neither, and is not on this path.
2. **The Church chain's inner lead, beyond §9d** — `((b x) y)` inside
   cif's lambda additionally needs the OUTER apply's window to be
   arity-provable; the inner apply's own admission is blocked by (1)
   first, so (1) is the prerequisite and the arity work is only visible
   behind it.
3. `tryReturnedClosure` for nested curried residuals (2-level-plus
   factories), and CPS (the factk rows). Stage 0 prerequisites
   (NUR077's Apply Op, NUR073's BROAD verdict) remain maintainer-ruled.

### 5.9 Smaller edges met while writing the programs

- **Quotations are not closures.** A `codequote`d body does not capture
  fn-locals: `def mkq fn n:Integer List [(codequote [add n])]` then
  `each q [1 2 3]` → `cannot call add`. Only `Function` values close.
- **User-defined words cannot take an unevaluated quotation.**
  `hof [mul 2] xs` evaluates the bracket at the call site;
  `NoEvalArgs` is a native-word privilege. Use `codequote`.
- **A computed key for a *quoting* accessor must be parenthesised.**
  At the audit's writing `cache has k` silently looked up the literal
  `"k"` and returned `false` — NUR040's class, and what made a first
  `memoize` attempt miss on every call while the cache visibly filled.
  **Corrected 2026-08-21:** `has` now evaluates its key exactly as
  `get` does, so that failure mode is gone for `has` — a bound bare
  key computes, and an unbound one raises `undefined_word` loudly.
  `set` remains the quoting member of the family (NUR040, Allowed):
  its computed key still needs `(k)`.
- **`flex` map keys must be Strings.** An `Integer` key is refused, which
  bites every memo table.
- **No comparator-based sort.** `sort` takes no callback;
  `ArrayUtil.sortby` takes a *parallel key list*, not a comparator, so
  ordering by a user function needs a Schwartzian transform.
- **User fn names collide with core words** with a hard-to-read error:
  `def outer fn [[a:Function]…]` → `[boru/extend_owner]: a core word can
  be extended only with at least one NOMINAL argument type the extending
  scope owns`.
- **No laziness.** `size (take 3 (iota 10000000))` answers `3` in 5.3s —
  the ten million elements are built first — and `iota` caps at
  16 777 216 (`[boru/iota_error]: iota: count … exceeds the cap`). No
  infinite sequences, no generators; a `take`-from-an-unbounded-stream
  program has no expression.
- ~~**No `while`.**~~ **Closed 2026-08-21:** `while [cond] [body]` is a
  word — condition re-evaluated per iteration, body values accumulate,
  `break`/`continue` as in `for`, step-budget-bounded (see
  `REFERENCE.md` §"`while` — the condition loop";
  `lang/spec/frontier/frontier-while.tsv`). At the audit's writing,
  `for` was a numeric range with `break` and loops over a condition
  were recursion.

---

## 6. Recommendations, cheapest first

1. ~~**Document the `(args).N` idiom.**~~ **Done differently, and
   better:** `/r`→`/v` with the function-only gate removed makes `/v`
   itself the total read, so there is no folklore idiom left to document
   (§5.2). `boru describe valof` and `lang/spec/valof.tsv` §9 carry the
   rule; `REFERENCE.md`/`HOWTO.md` were swept to the new spelling.
2. ~~**Add a diagnostic for §5.1.**~~ **Done (2026-08-25):**
   `stranded_type_call`, a check-mode **warning** — a bare lattice node
   whose declared content is a FUNCTION, immediately followed by a
   non-type value on the top-level straight line. It carries the
   did-you-mean treatment this item asked for: a `= note:` naming the
   capitalisation rule and a `= help:` whose replacement is the
   lowercased name. Warning, not error, because the program runs and
   exits 0 — so `boru do` is still silent, by the severity discipline
   rather than by omission. The gate keys on the node's fn *content*,
   not on predicate-ness, so the multi-argument combinators (`K`, `B`,
   `C`, `W`) are caught alongside the 1-arg ones; §5.1 tabulates the
   four shapes it deliberately stays quiet on.
3. ~~**Give `each`/`for-each`/`fold`/`scan` a `{TFunction, TList}`
   signature**~~ — **Done (2026-08-24, NUR086 retired).** The four
   signatures landed with an ELEMENT callback, reusing the existing
   handlers. The `filter` question this item raised is ruled rather than
   left open: a per-container form hands the container's natural unit,
   `filter`'s one cross-container form hands a position descriptor (§5.3).
4. ~~**Ship the missing vocabulary as a module**~~ — **Done
   (2026-08-21):** `boru:fn-util` ships `compose`, `pipe`, `curry`,
   `partial`, `const`, `identity`, `flip`, `on`, `memoize` as native
   words next to `boru:type-util` (`lang/go/modules/fn.go`;
   `REFERENCE.md` §"The `boru:fn-util` module"; behaviour rows in
   `lang/spec/frontier/frontier-fn-util.tsv`, ledgered under the
   def-bound-computed-fn refusal until that family graduates).
5. ~~**Fix NUR087**~~ — **Done (2026-08-24).** The checker no longer
   refuses the §1.5 library: the def-split collapse now admits a DYNAMIC
   argument, which is the shape a parser combinator is built from
   (`def r1 (a s)` then `def r2 (b (r1.rest))`). The audit's headline
   complaint — a correct program stopped by its own pre-flight — is
   closed, and the library runs under plain `boru run` in the spelling
   this note quotes.
6. ~~**Land NUR073.**~~ **Done (2026-08-24):** the BROAD fix landed — a
   paren places its collapsed Function on every lane, so `-no-compile`
   and the default are one language for these shapes again. The cost the
   verdict budgeted was paid: the paren-application idiom is gone and
   every §1 program was re-spelled with explicit `apply` (see the §1
   note). NUR073 is discharged, pinned via `canon`/`typeof` in
   `lang/spec/fn-value.tsv` §4b.

---

## 7. Where the LISP analysis needs updating

`LISP-ANALYSIS.5.md` §3 says *"inner fns snapshot enclosing-fn locals"* —
confirmed for locals, but §5.6 shows non-locals are resolved late, which
the grade should reflect. Its §0 grades `gensym` **D**; `gensym` exists
today. Its "Gaps" list (no `compose`/`curry`/`partial`/`const`/
`identity`/`flip`-by-name) is right as a *vocabulary* claim, though
partial application itself does exist as a mechanism (§2). §4's call for
Factor's `dip`/`keep`/`bi` still stands.

The grade this audit would give: **substrate A−, surface B−** (was C+
before `/v`). Nothing is missing from the machine, and the biggest
surface gap — no spelling for "the value, whatever kind it is" — is now
closed: `/v` is it. What is still missing is a uniform callback
convention (§5.3) and the point-free vocabulary that would make
hand-rolling `compose`/`curry`/`partial` unnecessary.
