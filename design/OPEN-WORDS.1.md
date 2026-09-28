# Open Words rev 2 — ownership-anchored signatures, natural dispatch order

Status: **decided (2026-07-23) — implemented on this branch.**
Revision of [OPEN-WORDS.0.md](OPEN-WORDS.0.md): rev 1's locked-first
ordering theorem (§2.3/§3.3 there) is **replaced** by an
ownership-anchored admission rule, and dispatch ordering becomes
purely type-natural. Everything else in rev 1 — the `def`-merge
model, word clones on the DefTable, scoping, export transplant, the
firewall idiom, sealed words — is unchanged. Discussion artifact per
the ADR rule (no ADR entry without explicit maintainer instruction).

One sentence: **a signature may be added to a word only when it is
anchored by a nominal type its author owns and replaces nothing its
author does not own; dispatch then simply picks the most specific
match.**

## 1. Why rev 1's ordering had to go

Rev 1 made merges safe with a tier: locked (native) signatures sort
strictly before every unlocked (merged) signature, before arity,
before specificity (`CompareSignatures`, signature.go). That bought
the stability theorem — *no previously-valid call changes its
dispatch under a merge* — by ordering alone. But the tier
over-approximates the theorem, and the over-approximation is exactly
the useful part of subtype polymorphism:

- `def M (refine Map)` + a merged `set [Atom Any M]` sig is
  **silently unreachable** — the locked `[Atom Any Map]` sig is
  tried first and matches the M value (a subtype conforms to its
  parent). Verified on the live tree: the user body never runs, the
  kernel handler wins, and the M tag evaporates on its result. No
  error anywhere; `typeof` keeps claiming M.
- The same override is legal and correct when both sigs are locked —
  the kernel's own WeakFlexMap `set` sigs dispatch ahead of the
  FlexMap sigs by per-position specificity. Specialization is a
  privilege of the host tier, not a property of the type lattice.
- FLEX-ATTRS.1 §5.1 therefore had to declare the module `set`
  override for a maintained `SortedFlexMap` "dead on arrival."

The tier also made safety a *global ordering invariant* — invisible
at the def site, dependent on what else is loaded — instead of a
local, checkable property of the signature being added.

## 2. The model — three rules, uniform across every author

Every type records its **owner** at registration (§4). Every
signature's author is the scope adding it: the kernel, a first-party
or user module, or the top-level program. The rules:

- **R1 — anchor.** A signature merged onto a word that carries
  signatures the author does not own MUST include at least one
  position typed by a **nominal** type the author owns. (A word the
  author owns outright — a plain user fn — is unrestricted, as
  today.)
- **R2 — no cross-owner redefinition.** A signature whose
  argument-type tuple equals an existing signature the author does
  not own is refused (`locked_signature`, generalized to all
  owners). Re-merging a tuple the author owns replaces it in scope,
  as today (the REPL iteration pattern).
- **R3 — natural order.** Candidates sort by arity, then
  per-position specificity on the unified type/value lattice, then
  barrier — with **no tier key**. `Locked` survives only as the
  replacement-refusal flag behind R2 and for the sealed words
  (`def`/`make`/`word`), never as an ordering input.

The kernel trivially satisfies R1 (it owns `Integer`, `Map`,
`Number`, …), so its registrations are unchanged. First-party
modules anchor on the types they register (`Date`, `Matrix`,
`Pathon`). User modules anchor on their minted types. The top-level
program anchors on types it mints with `refine`/`class`.

**Re-export carveout (transplant only).** R1 is enforced strictly
where a signature is WRITTEN (the def-merge inside the authoring
scope). At TRANSPLANT time, a source clone may carry re-exported
signatures whose anchors belong to a module deeper in the import
chain (rev 1 §2.4: re-export takes ownership of the *export*). The
transplant admission therefore accepts any **module-minted** nominal
anchor for source clones — reachability still holds, because a
module-minted type cannot predate the import chain that delivered
it — while kernel- and program-owned anchors stay author-only
(`sigHasModuleAnchor`, word_extend.go). Host clones always verify
against their declared `ExtOwner`.

## 3. The safety theorem, restated and proved

**Theorem (reachability).** Under R1–R3, no call whose argument
types all predate a merge changes its dispatch because of that
merge.

*Proof sketch.* Let S be a merged signature anchored at position i
by nominal type T owned by author A. Nominal membership is
tag-carried: `v.Is(T)` requires `v.Parent.ConformsTo(T)` — the value
must have been constructed *as* a T (or a subtype). No value tagged
T exists before T exists, and T is minted by A's scope at merge time
or later (for registered module types: before the module's sigs
transplant, and values of those types are only produced by the
module's own words). Hence any call matching S carries at least one
post-merge value; a call built entirely from pre-existing types
cannot match S at any position ordering. Since R2 forbids replacing
existing tuples, every pre-existing signature is still present and
still matches exactly the calls it matched before; by R3 those calls
see the same most-specific pre-existing signature. ∎

The theorem is the same guarantee rev 1's tier provided —
"previously-valid calls are byte-identical" — but proved by
*provenance* instead of *ordering*, which is precisely what makes
overlap-with-specialization admissible: a `set [Atom Any
SortedFlexMap]` sig overlaps the kernel `[Atom Any Map]` sig only on
values that could not exist before the module loaded.

### 3.1 Nominality is load-bearing (the counterexample)

R1 says *nominal*, not merely *user-defined*, and this is not
pedantry. boru user types split by membership mode (the refine
doctrine):

- **bare refine / class** — tag-carried; a value is a member only if
  constructed as one. These anchor.
- **predicate / member / disjunct / negation / schema / bounded-Type
  / surface bodies** — content-carried; membership is decided by
  looking at the value (or, for a surface, by an `exposes`
  declaration a type can make at ANY time). These MUST NOT anchor.
  *(Since the type-node fusion,
  [legacy/TYPE-REPRESENTATION.1.ignore](legacy/TYPE-REPRESENTATION.1.ignore), the catch-all
  kinds — literal/singleton and other body-carrying declarations
  routed through `BindingBodyUnifier` — belong to this
  content-carried, MUST-NOT-anchor set too.)*
  Counterexample: with `SortedMap` as a content predicate
  (`MintMemberType(TMap, keysAscending)`), the pre-existing literal
  `{a:1 b:2}` *is* a SortedMap — its keys happen to ascend — so a
  sig anchored on it would capture pre-existing `set` calls and the
  theorem is dead. A **surface** is the same trap by a different
  door (Codex-review P1): its conformance set is shared and sees
  *post-hoc* `exposes` updates, so a value that predates the surface
  can start matching it — anchoring on a surface would let a later
  `exposes` retroactively capture old calls. Every such behavior
  carries the `ContentMembership` marker; `IsNominalAnchor` excludes
  the marked set, and `TestContentMembershipInventory` pins it so a
  new content behavior missing the marker fails loudly rather than
  silently becoming anchor-eligible.
- **aliases** (`def MyMap Map`) never mint, so they cannot launder a
  foreign type through the check — unchanged from rev 1.

Consequence for FLEX-ATTRS.1 §5.2: the classification type
(predicate `SortedMap`) and the override anchor (nominal
`SortedFlexMap`) are necessarily *different types*. The predicate
type classifies; the nominal type specializes.

## 4. Ownership — a stamped fact, not a category

`Type` gains an **Owner** stamp, set once at registration:

- kernel builtins (`builtinDecls`) — the kernel sentinel;
- module-registered global types — the registering module's id
  (`boru:time-util` owns `Date`/`Instant`/the durations; matrix owns
  `Tensor`; `boru:io` owns `Pathon`'s module-scoped kinds; …);
- runtime mints (`refine`/`class`/member types) — the minting
  scope's module id, or the program sentinel at top level.

This dissolves rev 1's builtin/user dichotomy (`isUserType`,
`Origin == OriginUserDef`) as the safety discriminator, and with it
the first-party waiver: `NewWordExtensionAnchored` /
`AllowBuiltinAnchor` existed solely because `Date` was "builtin" and
therefore could not anchor time-util's own arithmetic. Under
ownership, `add [Date CalendarDuration]` is a rule application, not
an escape hatch.

**`RegisterExternalBuiltin` is retired.** There is one registration
path for globally-registered types, carrying (path, FixedID, owner,
behavior). FixedIDs and the wire-stability contract are **retained
unchanged** — wire identity is orthogonal to ownership, and
`fixedid_stability_test` continues to pin every ID at its current
value. What disappears is the *category* ("external builtin") and
the API name; what remains is one owner-stamped registration used by
kernel table and modules alike.

## 5. Behavior changes

1. **Top-level non-anchored merges are refused.** `def add fn
   [[a:Boolean b:Boolean] …]` at top level — admissible under rev 1
   because the tier made it harmless — now fails the R1 admission
   check (`extend_owned_type`). Maintainer's call: the pattern is
   non-essential and dangerous (it was also the main §4.1/§4.2
   hazard surface). The fn-scope/closure/undef *machinery* is
   untouched and continues to work for anchored merges.
2. **Module extensions gain reachability.** An anchored `set [Atom
   Any SortedFlexMap]` sig now sorts ahead of the kernel Map/FlexMap
   sigs by specificity (user mints rank above every kernel type in
   their branch — `externalBandFor`) and actually dispatches. This
   un-blocks FLEX-ATTRS.1 D4's maintained sorted type at the word
   level.
3. **First-party extensions are unchanged in effect.** Temporal /
   io / matrix tuples all anchor under ownership; the waiver
   plumbing is deleted.
4. **Dispatch among pre-existing sigs is unchanged.** All-kernel
   candidate lists were already specificity-sorted within the locked
   tier; removing the tier key reorders nothing among them, and
   formerly-unlocked benign merges either become refusals (1) or
   anchored (2).

## 6. Hazards carried forward (unchanged verdicts, new shapes)

- **Collection shift (rev 1 §4.2).** A merged sig still widens
  forward collection — but with R1, only around values of the
  anchor's nominal family, which shrinks the advisory surface to the
  owner's own domain.
- **Cross-module subtype shadowing.** Module B may refine module A's
  nominal type and anchor overrides on the subtype, changing
  behavior for B-tagged values flowing through A-generic code. That
  is ordinary virtual-dispatch fragility (B can break A's invariant
  for B's values), not a stability violation. Allowed; documented.
- **Bypass funnels.** Words that write through the mutable payload
  in Go (the setpath/struct family, adoption, compiled fast paths)
  never dispatch `set` and are untouched by any sig-level mechanism.
  FLEX-ATTRS.1 §5.2's write-mediation capability remains the answer
  underneath an invariant-bearing type.
- **Compiled-path conservatism.** Fold sites must not assume a
  parent-typed receiver dispatches the parent sig when a subtype
  could flow — a discipline the locked tier already required
  (WeakFlexMap vs FlexMap are both locked), now policed for merged
  sigs too by the differential and whole-corpus gates.

## 7. Migration

| Piece | Change |
| --- | --- |
| `CompareSignatures` | delete the Locked ordering key; keep arity → per-position → barrier |
| `word_extend.go` | `requireUserTypedSigs` → owner-verified nominal-anchor rule, applied at every scope (def-merge and transplant); `AllowBuiltinAnchor` / `NewWordExtensionAnchored` deleted |
| `native_definition.go` merge path | top-level/fn-scope merges onto other-owner words run the same admission check |
| `TypeTable` | `RegisterExternalBuiltin` removed; unified owner-carrying registration; `Owner` stamped on every mint path; FixedIDs unchanged |
| first-party callers | temporal / timers / fetch / keyval / module-types / matrix re-registered through the unified path with their owner ids |
| `lang/spec/open-words.tsv` | non-anchored merge rows become admission refusals; anchored twins pin the new dispatch (a minted subtype's sig actually fires); locked-first rows repinned as ownership rows |
| `micron.tsv` §extensions | row 504 (builtin Micron leaf refused) unchanged in outcome, error code updated; 505 (minted Baron anchors) unchanged |
| ratchets | check-accuracy pins for touched files; COMPILED_STATUS refresh; fnmodel golden if sig lists shift |

## 7b. The pure-boru enablers (follow-up; two landed, one OPEN)

Three seams initially kept the FLEX-ATTRS.1 D4 sorted nodes a
Go-module capability. Two are fixed (pinned by
`lang/spec/refine-flex.tsv`):

- **Flex retag.** `def w:S (flex m)` for `S (refine FlexMap)` — the
  flex-literal unify arm now accepts a check-mode carrier tagged at or
  under the flex type (`unifyFlexLiteral`), matching the runtime,
  which already unified. Bare-refine newtypes of mutable containers
  render by payload family (`bareRefineUnifier.formatDelegate`).
- **Container content-predicates** already worked as fn-bodied
  capitalised defs (`def Sorted ([m:Map] => [pred])`); pinned. Since
  NUR099 they are declared with `fnpred` (`def Sorted (fnpred [[m:Map]
  [pred]])`) — a capitalised name over an undeclared fn body is refused.

The third — **base-dispatch delegation for override bodies** — is
now CLOSED by `as` (§9). A `super`-style word (returning the
pristine dispatch as a Function to bind under a fresh name) was
built first and REJECTED by the maintainer: requiring renamed
shadows of core words (`def bset (super set)` and then calling
`bset` in the body) is bad DX. §7b.1–7b.3 keep the problem
statement and the design space that led to the decision.

### 7b.1 The delegation problem, precisely

An anchored override's body needs to PERFORM the operation it
specializes. For `SortedFlexMap`, the `set` override must (a) write
the entry, then (b) restore key order — and both (a) and the
per-key moves in (b) ARE the operation `set` (plus `del`). But
inside the override's scope, the word `set` resolves — like every
boru word, dynamically, at call time, through the def stack — to the
MERGED clone, whose most specific matching signature for a
SortedFlexMap receiver is the override itself. The body's `set`
re-enters the body: unbounded recursion. The name being extended is
the only name the body has for the thing it needs underneath.

This is the classic override-shadow problem every OO language meets
(`super` in Smalltalk/Java/Python, `call-next-method` in CLOS,
`invoke` in Julia). boru's value-binding `def` avoids it for VALUES
by evaluating the body before binding (`def x (x add 1)` reads the
old x) — but fn bodies are DEFERRED code, resolved at dispatch time,
so the same trick does not apply.

### 7b.2 Why Go-side extensions never hit this

The kernel's own subtype specializations (WeakFlexMap's `set` sigs
over FlexMap's) delegate freely because Go operates in a SECOND
namespace below the word layer. The word `set` is only a dispatch
surface; each signature's implementation is a distinct Go symbol
(`setFlexMapHandler`, `setWeakFlexMapHandler`), and those handlers
never "call `set`" — they call payload-level operations
(`OrderedMap.Set`, `WeakFlexMapData.SetValue`). Two structural
facts do the work:

1. **Two layers of naming.** One shared surface name for dispatch;
   unlimited distinct implementation names beneath it. Overriding at
   the surface while delegating at the implementation layer is
   trivial because the layers cannot collide.
2. **Static resolution.** A Go handler's callees are fixed at link
   time. A boru body's words are resolved at call time through a
   stack the merge itself just rewrote — the override shadows the
   very name it needs.

boru source currently has ONLY the surface layer: the word `set` is
both the dispatch surface and the sole vocabulary for the
operation. That is the whole gap.

### 7b.3 Design space for the drawing board

- **(a) Pre-merge lexical resolution.** Inside a merge's body,
  references to the extended word resolve to the PRE-MERGE dispatch
  — the def-body precedent (`def x (x add 1)`) applied to extension
  bodies, implementable as an implicit capture of the base clone at
  def-merge time. DX: the body just says `set k v m` and it means
  "the set that existed when this code was written" — exactly the
  static-resolution intuition Go gets for free. Cost: the body
  cannot re-enter the FULL dispatch (no virtual self-recursion over
  substructures of the same subtype); that call would need the
  surface spelling from outside, or is simply not expressible.
- **(b) A dispatch-continuation word** (CLOS `call-next-method`):
  one uniform word meaning "continue dispatch below the currently
  executing signature." No renamed shadows, full-fidelity semantics,
  and self-recursion stays available through the plain word. Cost: a
  special word with dispatch-stack context, and a compile story for
  it.
- **(c) A primitive layer.** Give boru the implementation vocabulary
  Go enjoys — a small set of payload-level words (`map-put!`,
  `map-del!`, an ordered-insert) that the surface words are defined
  over. Overrides then build on primitives, never delegating to the
  surface at all — the same two-layer structure as the kernel. Cost:
  new vocabulary to design and maintain; the primitives must be
  powerful enough for real invariants without becoming a second API.
- **(d) Call-site signature selection via dispatch ascription**
  (maintainer proposal). Force a call to a SPECIFIC signature by
  widening an argument's type AT MATCH TIME ONLY: `set k v (m as
  FlexMap)` dispatches as if `m` were a plain FlexMap — so the
  anchored override cannot match (its anchor is a nominal subtype
  the widened tag no longer conforms to; the reachability theorem
  run in reverse) and natural specificity lands on the base sig —
  while the VALUE passed to the body keeps its real tag (Julia
  `invoke` semantics: select on declared types, execute on actual
  args). Rule R2 (no redefinition) already guarantees a type
  vector uniquely names a sig, so exact ascription pins
  deterministically. The crucial fork is what kind of cast: a
  RETAGGING cast (reparent the value) would evaporate the subtype
  tag on results and hand outsiders an invariant-bypass; a
  match-time-only, UPCAST-only ascription changes nothing about
  the value and refuses downcasts, so it is sound with no new
  dispatch machinery — it is per-call static-resolution, i.e. a
  devirtualization hint the compiled path can pin statically
  (the strongest compile story of the four options). Costs: a
  pure-boru base body's own internal calls remain virtual (they
  dispatch on the real tag — same property as Julia's `invoke`;
  the kernel natives are the recursion floor today), and the
  operator is publicly available, so outside code can route around
  an override explicitly (visible and greppable, but a policy
  question: allow anywhere vs restrict to extension bodies).
  Surface syntax open: an `as` word vs reusing `:` ascription in
  call position.

(a) matches the maintainer's resolution-should-be-natural instinct
most closely; (c) is the most honest mirror of why Go works; (b) is
the classical multiple-dispatch answer; (d) is the C++ qualified-
call / Julia `invoke` answer — explicit like (b) but needing no
dispatch-stack context, and it keeps full-dispatch re-entry
available by simply not ascribing.

**DECIDED: (d), implemented as the `as` word — see §9.**

## 8. Relation to bounded generics

None needed — confirming the earlier conclusion: `gen [(T extends
Map)]` already covers the parametric half (one implementation, many
types, checker-preserved `T → T`), and this note's rules cover the
ad-hoc half (different implementation per owned subtype). The two
compose: a module's generic utilities accept any Map subtype and
keep it; its anchored sigs specialize behavior for the subtypes it
owns.

## 9. `as` — call-site signature selection (option (d), IMPLEMENTED)

`v as T` is match-time dispatch ascription: the value comes back
UNCHANGED — payload, real Parent tag, rendering, equality — carrying
an ascription (`Value.asc`, a pointer field) that the NEXT signature
match reads as the value's tag. Three rules make it sound:

- **Upcast-only.** `T` must be an ancestor of (or equal to) the
  value's own tag (`asValidate`, error `as_error`) — `as` widens
  dispatch, it never claims a subtype the value does not carry, so
  no runtime representation changes and no invariant can be forged.
  Bare type literals refuse on both slots' misuse: the target must
  be a nominal type literal, the subject must be a value.
- **Match-time only.** The single matcher choke point
  (`sigTypeMatches`, which every dispatch path funnels through —
  the interpreter's matchSignature arms, positionalMatch, the VM's
  guarded-native contract and poly re-match) substitutes a
  reparented VIEW for the test and nothing else. The view is
  strictly non-dynamic: post-validation, dispatch resolves at
  exactly the ascribed type.
- **Consumed at delivery.** Every arg-delivery and storage boundary
  strips the ascription (`StripAscribed`): the interpreter's
  execMatch args loop and execFnDefSig param binding, list/map
  literal assembly (autoEvalList / autoEvalMap), and the VM's
  OpCallNative / poly / OpCallUser(Poly) arg pops, OpStoreLocal /
  OpBindGlobal / OpBindDynScope / OpBindTyped stores, and
  OpMakeList / OpMakeMap assembly. So an ascription selects exactly
  ONE dispatch and can never ride into a handler, a binding, a
  container, or a returned receiver.
- **Never escapes a function return.** A fn/lambda/module call is an
  abstraction boundary with a declared signature: the value it hands
  back is its real self, so an ascription written in the body cannot
  leak into the caller's dispatch. Every fn-return strips it — the
  interpreter frame-collapse (the ReturnCheck arm, via
  `stripTapeAscriptions`), `Registry.CallBoru`, and the VM `OpRet` over
  the frame's produced values. Without this a helper `[(w as
  FlexMap)]` would hand its caller an ascribed value and the caller's
  next dispatch would wrongly select the base overload (the
  Codex-review P1: an escaped ascription). Everything ELSE is inline
  value-routing and stays TRANSPARENT — a paren group, an `if`-branch,
  and a code-body word (`do` / `each` / `fold` / …) all route the
  value to the next dispatch, which consumes the ascription there.
  That is what makes `set … (m as FlexMap)` work, and it is what keeps
  the interpreter in step with the COMPILED path: the compiler folds
  `as` at compile time into the static dispatch selection, so the
  ascription rides the value flow to the consuming dispatch across
  every inline form. A fn return is the one boundary the compiler
  models with a declared type rather than the ascribed carrier, which
  is exactly why stripping there — and only there — keeps the two
  engines in agreement. Pinned by lang/spec/as.tsv §8.

**Why it solves §7b.1.** Inside an anchored override, `set k v (m
as FlexMap)` widens the receiver's dispatch tag to FlexMap; the
override's own nominal anchor (a strict subtype) cannot match the
widened view — the reachability theorem run in reverse — so natural
specificity lands on the base overload, while the base handler still
receives the real subtype-tagged value (the tag survives, in-place
mutation hits the same payload). The body reads in core vocabulary;
no renamed shadows, no retagging, no dispatch-stack context. R2 (no
redefinition) guarantees a type vector uniquely names a signature,
so exact ascription pins deterministically.

**Check/compile story.** `as` is an ordinary native (no RunInCheck):
`asReturns` is its exact static mirror — a provable violation (bad
target, non-ancestor over a known tag) emits the same `as_error` as
a RuntimeMirror diagnostic (`as_error` is registered SeverityError);
a valid ascription returns the input carrier ascribed, so check-mode
dispatch of the consuming word resolves the SAME signature the
runtime match will; a dynamic input the tree lattice cannot refute
passes gradually as a carrier at the target (the runtime handler
re-validates). Acceptance: lang/spec/as.tsv (transparency, subtype
tags, overload selection, one-call consumption, delegation,
refusals) and lang/go/test/sorted_user_test.go (the full pure-boru
SortedFlexMap: anchored override + as-delegation + recursive
resort keeps keys sorted with the tag intact).

**Compiled-path residual.** A DYNAMIC operand dispatching a word
that carries BOTH native and merged user signatures has two live
candidates, and the emitter's dispatch commit is single-candidate
(static CALL_NATIVE, single-sig guard, or native-only/user-only
poly) — so e.g. the resort loop's dynamic list-element key feeding
the merged `set` refuses compilation ("dynamic input at set") and
the acceptance runs interpreted. This is the pre-existing
dynamic-input boundary meeting merged tables, not an `as` limit —
ascribed receivers with strict keys compile (as.tsv §5), and the
VM's poly re-match already honors ascription when it does run.
Widening the commit to guarded multi-candidate dispatch is future
compiler work.
