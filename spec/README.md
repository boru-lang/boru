# spec — the Lean specification of the boru kernel

A machine-checked description of **the core** — the interpreter kernel in
[`core/go`](../core/go): values, the type lattice, canonical rendering,
the total order, unification, signature matching and the step loop.
Nothing above the kernel is here: no word library, no parser, no checker.

The specification is three things at once, on purpose:

- **a definition** — each kernel mechanism is a total Lean function, so
  "what does the kernel do with this tape" has one answer you can read;
- **a third implementation** — the model is executable, and is held to
  the same parity corpus ([`core/spec`](../core/spec)) that `core/go` and
  `core/ts` are held to, through a runner of its own;
- **a place to prove things** — the lattice is an order, the argument rule
  is one rule at every arity, the total order is total.

## Layout

| File | What it defines |
|------|-----------------|
| [`Boru/Core/Lattice.lean`](Boru/Core/Lattice.lean) | the type lattice over any declaration table: conformance, chains, common ancestors, bare-name and path resolution |
| [`Boru/Core/TypeTable.lean`](Boru/Core/TypeTable.lean) | the kernel's builtin table — **generated** from `core/go/typetable.go` |
| [`Boru/Core/Number.lean`](Boru/Core/Number.lean) | BigDecimal and binary64, exact, with their source forms |
| [`Boru/Core/Value.lean`](Boru/Core/Value.lean) | the value universe: what a tape holds |
| [`Boru/Core/Canon.lean`](Boru/Core/Canon.lean) | the canonical source form of a value |
| [`Boru/Core/Compare.lean`](Boru/Core/Compare.lean) | the one total order; a disjunct's canonical alternatives |
| [`Boru/Core/Unify.lean`](Boru/Core/Unify.lean) | unification — the meet of two values |
| [`Boru/Core/Truth.lean`](Boru/Core/Truth.lean) | the one truthiness rule |
| [`Boru/Core/Match.lean`](Boru/Core/Match.lean) | the binding store, slot admission, the plan: which signature a call takes and where its arguments come from |
| [`Boru/Core/Engine.lean`](Boru/Core/Engine.lean) | the step loop |
| [`Boru/Core/Theorems/`](Boru/Core/Theorems) | what is proved |
| [`Boru/Corespec/`](Boru/Corespec), [`Corespec.lean`](Corespec.lean) | the corpus notation, the fixture words, and the runner |
| [`COVERAGE.md`](COVERAGE.md) | which kernel mechanisms are specified, and which are not yet |

## How it is checked

```bash
make -C spec check     # verify-typetable + build + corpus
make -C spec fuzz      # differential search against core/go
```

**`build`** type-checks the model and every theorem. There is no `sorry`.

**`corpus`** runs every row of `core/spec/{canon,data,dispatch,unify}.tsv`
on the model and compares the result with the row's `expected` column —
the third runner of the corpus, sharing no code with the other two. It
also reports, for `divergent.tsv` (where `core/go` and `core/ts`
disagree), which engine the specification sides with — `core/go`, on
every row today.

**`verify-typetable`** regenerates `TypeTable.lean` from the Go table and
fails on any difference: the specification's lattice is the kernel's.

**`fuzz`** generates random programs over the fixture vocabulary, runs
each on the model and on `core/go` (through `TestCoreProbe`), and prints
every disagreement. It is a search tool: agreement proves nothing, a
disagreement is a concrete expression where model and kernel part ways.

### What the theorems are, exactly

Two kinds, and the difference matters:

- **General theorems** (`conformsTo_refl`, `conformsTo_any`, …) hold for
  every table and are proved by ordinary proof, checked by Lean's kernel.
- **Finite theorems** — every statement about the builtin table, and
  every statement that runs the machine — quantify over a stated finite
  domain and are discharged by `native_decide`: evaluated by Lean's
  compiler rather than reduced in its kernel. They are exactly as
  trustworthy as the compiler, and they are statements about the stated
  domain only. `arity3_splits` says the four splits of a three-argument
  call agree for every choice of three operands **from its operand
  set** — it is a checked instance family of the rule, not a proof of the
  rule for all tapes.

Making the machine reduce in Lean's kernel (it is blocked today by the
string functions `TypeId` leans on) and proving the argument rule for
all tapes are the next steps on this side; `COVERAGE.md` lists them.

### Where the specification and `core/go` differ

Nowhere that has been compared. The specification states the contract and
`core/go` meets it on every corpus row and every program the search has
drawn. (The one difference there was — `core/go` rendered an open-paren
marker as nothing, which `core/spec/divergent.tsv` had adjudicated against
it — was fixed in the kernel.)

Four kernel defects were first written down here — the model reproduced
them because the kernel had them, and each was recorded in
[NUR.md](../NUR.md) (NUR385–NUR388). All four are fixed in the kernel and
the records closed: the model now states the rule, and the rows that pin
each fix are in `core/spec` under the record's number.

Nine more are open, and the model reproduces them until they are ruled
on: a `/uv` value is not collected by a waiting word as a `/v` value is
(NUR390); a word with more than one definition is not equal to itself
(NUR391); an unmatched `(` is refused at the top level and in a
literal but collected as a value in a loop's body (NUR392); a function
that is a function body's only result is refused as an uncalled call
(NUR396); a word defined in boru is not retried from the stack when
its forward plan fails, where the same function as a literal is (NUR398);
a full-stack word in a counted loop's body takes the loop's mark with the
stack it replaces, and the loop ends there (NUR400); a function value
planned again for a wider overload that does not fill drops the argument
it had collected (NUR401); a counted loop's index wraps past the
Integer range, so an exhausted range starts over from the other end
(NUR402); and the values a function of unnamed parameters may leave
beneath its returns are counted, not identified, so a value its body
computed is dropped in a consumed argument's place (NUR404). Each is
marked where it is modelled and where it is pinned.

Two the model never had, and found by disagreeing with the kernel. A
loop's inert inputs — and a call frame's unnamed arguments — were stepped
when the loop or the call ran inside a group a waiting word evaluates, so
a function among them was called on what the body wrote after it (NUR397).
And a root type — `Any`, `None`, `Never` — standing as a loop's or an
`if`'s condition was reported as "no value", because the kernel asked the
value for its lattice parent and a root has none (NUR399). In both the
model stated the rule and the kernel was fixed to meet it.

Four the model does NOT reproduce, because each is an accident of how
`core/go` holds or compares values rather than a rule to state: typed
containers equal by their constraint alone (NUR393), a disjunct inside
another value compared as it prints (NUR394), an order that is not
transitive between typed and plain lists (NUR395), and an empty list built
over a nil slice read as no list at all (NUR403). There the specification states the
structural rule and disagrees with `core/go`; the differential search does
not draw those shapes, and no corpus row sits on them.

## Toolchain

[`lean-toolchain`](lean-toolchain) pins `leanprover/lean4:v4.36.0`. No
Mathlib and no other dependency — the bundled core library only. With
[`elan`](https://github.com/leanprover/elan) installed, `lake` fetches the
toolchain itself. Where elan's download host is unreachable but GitHub
is, unpack `lean-4.36.0-linux.zip` from
`github.com/leanprover/lean4/releases` and put its `bin/` on `PATH`.
