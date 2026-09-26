# CONTENT-ADDRESSING.0 — identity by hash, and what it would actually take

> **Status: discovery / design note — not implemented.** It consolidates
> two documents into one design: `legacy/unison-in-boru-report.0.ignore` (the
> applicability study — *should* boru do this?) and
> `legacy/unison-hash-identity-probe.0.ignore` (the proof of concept — *can* it,
> today?). Read this one first; the other two are the argument and the
> measurements behind it.
>
> Nothing here is an ADR. §4.1 identifies one rule that would need to
> become one (**canonicity**), and per `ADR.md` that happens only on
> explicit maintainer instruction.
>
> Reproduce every measurement quoted below with
> `scripts/hash-identity-probe.sh`.

## 1. The thesis

Three costs boru is currently paying share a single root cause, and it is
not a bug in any of them. It is that **a definition's identity is its
name.**

| Cost | Where |
|---|---|
| Every invoke of a runtime-stamped callback pays a pull-based `DepsFresh` walk — ~2–10 Go map lookups before the VM executes one opcode. "Compiled code gets no slower" is violated *before reload exists*. | `RELOAD-INVALIDATION.0.md` §1, §2.2 |
| The AOT codec must invent "a stable symbolic identity a fresh process re-resolves" for every pointer in a `*Program`, and its table is mostly **refusals** — user-poly calls, interpreter islands, exotic const payloads. | `AOT-COMPILE.0.md` §3.2 |
| The pre-1.0 rename tax: `test.test` → `Test.test`, every utility module gaining `-util`, word families relocated between modules — each breaking every downstream program for **zero** semantic change. | `README.md` §Upgrade notes |

Each is being solved separately: generation counters and poisoning for the
first, a per-pool whitelist for the second, a migration note for the
third. Content addressing is the observation that they are one problem.
If identity were derived from *what a definition is* rather than *what it
is called*, then staleness is not a runtime property to validate (it is a
cache miss), a cross-process reference needs no re-resolution scheme (the
identity travels), and a rename touches no code (only a name map).

> **Not on this list: F1.** An earlier draft of this note counted
> `RELOAD-INVALIDATION.0.md` §3's confirmed divergence (`6 105 12`
> interpreted, `12 12 12` compiled) as a fourth symptom. That was wrong,
> and PR #376's review caught it. F1's mechanism is **phase ordering, not
> identity**: the rebinds *do* correctly poison the stored ref, but
> module-scope `def` sites execute only during the **compile pass**, so by
> VM time the def table already holds the final `bonus` and there is no
> runtime rebind left for any cache key to miss on. Capturing a hash
> cannot restore `6 105 12`; that needs runtime bind lowering or a
> conservative refusal. Hashing is **complementary** — once binds are
> lowered to runtime, a hash-keyed cache misses correctly on each one —
> but it is not the fix, and this note no longer claims it is.

That is the thesis. The rest of this note is about how far boru can
actually go, which is further than nothing and considerably less than
Unison.

## 2. What boru already has

boru is unusually close to the prerequisites, for reasons unrelated to
hashing:

- **Code is data.** The parser emits `[]core.Value`; quotations,
  `quote`/`unquote`, and hygienic macros are all ordinary values.
- **A rendering contract.** ADR-015: `canon v` renders source that
  re-parses to a value `deq` to `v`, with **no exempt kinds** — the
  awkward kinds were deliberately not carved out, because "canon is the
  serialisation boundary."
- **Structural equality.** `deq`, with the recorded fn/host reflexivity
  gap (NUR031).
- **A digest, already.** `BinUtil.fnv64` (`lang/go/modules/binary.go:501-521`),
  explicitly non-cryptographic; `boru:crypto` is designed but unbuilt.
- **Version coexistence, accidentally.** Nested `.boru/<dep>/.boru/<dep>`
  install trees already isolate two versions of a dependency.

What is missing is not machinery. It is that nothing consumes the
rendering as an identity.

## 3. What the proof of concept measured

The PoC is `BinUtil.fnv64 (canon v)` — the whole proposal, with no engine
change. Ten properties that content-addressed identity requires:
**3 pass, 8 fail, 1 suspected hazard cleared.** Two results change the
design; the rest scope it.

**Result 1 — faithfulness is not canonicity.** `{a:1 b:2} deq {b:2 a:1}`
is true, their canons differ, their digests differ. ADR-015 states the
round-trip in one direction only (`parse(canon v) deq v`), and
`CANON-ROUNDTRIP.0.md` §1 records that the fixpoint reading was
*deliberately rejected*. Identity needs `deq x y` ⟹ `hash x == hash y`,
which is a **new** rule, not a pending one.

**Result 2 — a text hash is not a meaning hash.** `usedbl` calls `dbl`;
rebinding `dbl` moves `usedbl 5` from `10` to `500` while the digest
stands still at `4947418992068046316`, because canon renders a callee as
its *name*:

```
fn usedbl[[n:Number][Number][word(dbl) word(n)]]
```

This is the finding that reshapes everything. A digest with this property
used as a compiled-unit cache key does not fix F1 — **it reproduces F1**,
serving a unit built against the old `dbl`. Macros compound it: they are
not expanded in canon, so a macro edit is invisible to every caller's
digest.

The supporting results: a fn's canon carries its binding name (NUR031) and
its parameter names (**new** — NUR031 covers only the former); a fn's
canon does not re-parse at all; a `Store`'s canon embeds live pointer
addresses, so its digest differs *between runs of the same program*; and
cycles terminate only because canon does not follow references — the same
property that causes Result 2, so fixing it buys the cycle problem.

One hazard was investigated and **cleared**: Go and TS canon agree byte
for byte on twelve float cases including max double, the smallest
subnormal, and `-0.0`. Cross-port identity agreement is in better shape
than assumed; the parity discipline (ADR-014) is doing its job.

## 4. The design

### 4.1 Two digests, deliberately not one

The single most important design decision is to **stop using one word for
two things.** Everything cheap lives on one side of this line and
everything hard on the other, and conflating them is what made the
original report over-promise.

| | **Artifact digest** | **Definition digest** |
|---|---|---|
| Hashes | file bytes | a definition's *meaning* |
| Construction | Merkle over sorted relative paths + bytes + modes | normalised AST with referents replaced by their digests |
| Semantics needed | none | canonicity, referent resolution, macro expansion, cycles (NOT alpha-normalisation: a boru parameter's name is behaviour — §4.2 step 3, NUR074) |
| Buys | dependency pinning, reproducibility, signing, capability/code binding | compiled-unit caching, free renames, test caching |
| Status | **specified already** (`boru-vendor.0.md` §5); recommended already (`MODULE-SECURITY.0.md` §9.3) | a programme (§4.3) |
| Blocked by | nothing | P1–P8 |

The artifact digest is available now and needs no language decision. It is
Phase 1 and it should not wait for anything in this note.

### 4.2 The normalisation pipeline a definition digest needs

In order, each step with the probe result that demands it:

1. **Canonicity** — `deq` values must render identically (P1). *New rule;
   would need an ADR.*
2. **Strip the binding name** (P3) — NUR031's verdict already requires it.
3. ~~**De-name parameters**, to positional references (P4)~~ — **withdrawn
   (NUR074, resolved 2026-09-26).** It would be UNSOUND in boru. A parameter
   is a frame binding on the def stack, and a free name resolves at call
   time through that stack, so a parameter's name is visible to every
   function the body reaches: `def x 1  def g fn [[] [Any] [x]]` then
   `def f fn [[x:Any] [Any] [g]]  f 5` answers 5, and the same `f` with its
   parameter named `y` answers 1. Unison can de-name because its variables
   are lexical; boru's parameters are not, so two definitions differing only
   in a parameter name are two behaviours, and a digest must keep the name
   (as canon and `deq` do).
4. **Expand macros** before hashing (P6), or a macro edit is invisible.
5. **Substitute referents** with their digests (P5) — §4.3, the crux.
6. **Cycle components** — hash a strongly-connected group as a unit,
   address members by index, because step 5 reintroduces the cycle that
   canon's name-rendering currently avoids.
7. **Renderers for the unspellable kinds** — fn (P2) and Store (P8);
   ADR-015 §5 explicitly leaves their source form undecided.

Plus one operational precondition: the compiler **refuses** any program in
which a function value reaches `canon` ("Stage 3"), so computing an
identity currently drops the program to the interpreter.

### 4.3 The crux: referent substitution under call-time binding

Step 5 is where Unison's design meets a boru rule it cannot simply adopt.
Unison resolves a name to exactly one definition at hash time. boru
resolves a word at *call* time, and resolves it to a **set** of signatures
selected by argument type — so "the referent of `dbl`" is not even a
well-defined single thing. Three options:

**(a) Digest the resolved world.** A definition's digest incorporates the
digests of its referents as resolved at digest time. A rebind therefore
mints a new digest for the callee *and every dependent* — which is exactly
Unison's `update` propagation. Buys the full payoff (free renames, test
caching, cross-process references). Costs a propagation pass, a resolution
step that must pick one referent per call site, and a direct collision
with call-time binding: `usedbl`'s meaning would be frozen at digest time,
which is a **language change**, not an optimisation.

**(b) Digest the text, key on (text digest, world digest).** The
definition digest covers the normalised body with referents left as names.
A second digest covers the **assumption set** — the `name → digest` map
for exactly the names the body references. A compiled unit is keyed on the
pair. Call-time binding is untouched: a rebind changes the world digest,
the compound key misses, and the unit is recompiled or falls back.

**(c) Inline the closure.** Monomorphise dependencies into the body before
hashing. Rejected: it destroys late binding, which is a documented
language property, and it makes every dependent recompile on any change.

**Recommendation: (b) for the compiled-unit key; (a) only if the workflow
payoffs are ever wanted, and only as an explicit language decision.**

Option (b) deserves emphasis because it collapses the **hot path**: what
`depNames` + `Gen`/`Depth` does per invoke — a walk over the dep set,
two map lookups per dep (`RELOAD-INVALIDATION.0.md` §2.2) — becomes a
single equality check on a precomputed digest. That is constraint 3
("compiled code gets no slower") satisfied without a language change.

**What (b) does not buy, corrected after PR #376 review.** Two earlier
claims here were too strong:

- **It does not avoid the reverse index.** Something has to update a
  unit's world digest when a name in its assumption set rebinds, and
  finding those units *is* a reverse dependency index — the machinery of
  `RELOAD-INVALIDATION.0.md` §5.2. The only way to skip it is a single
  global world counter bumped on any rebind, which is one integer compare
  on the hot path but **over-invalidates**: every compiled unit misses on
  every unrelated rebind. So (b) is a choice between a reverse index for
  precision and a global counter for simplicity, not an escape from both.
  What (b) genuinely changes is the *hot path* cost, which becomes O(1)
  either way; the rebind-event cost is the same work §5.2 already
  describes.
- **It does not by itself retire the user-poly refusal.**
  `AOT-COMPILE.0.md` §3.2 refuses `UserPolyRef` for **two independent**
  reasons: live `Impls` pointer identities, *and* stored-mode `Sigs` — a
  body-local overload table "a live name `Lookup` could never resolve"
  and which cannot be re-derived in a fresh process. A digest over the
  impl set replaces the first. The second is untouched, and any design
  that wants this refusal lifted must separately specify how stored-mode
  `Sigs` are serialized or reconstructed.

### 4.4 Phasing

Each phase is independently shippable and ends green.

- **Phase 0 — decide canonicity.** Open a record for `deq x y` ⟹
  `canon x == canon y`, and settle the question it forces: map key order
  is preserved by `canon` and insignificant to `deq`, so canonicity means
  either sorting keys in the canonical form (canon stops reproducing
  source order) or narrowing `deq` (a wider notion of inequality). No
  code. **Everything else depends on this.**
- **Phase 1 — artifact digest on the registry path.** `deps` records a
  content hash; `install` verifies it; reuse `boru-vendor.0.md` §5's
  Merkle rule. Independent of Phases 0 and 2+. Closes
  `MODULE-SECURITY.0.md` §9.3, which is the precondition for its own
  capability-manifest work: a manifest is advisory unless manifest and
  bytes are pinned together.
- **Phase 2 — a `hash` word, scoped to data.** Ship the digest for
  scalars and lists, documented as **not** applying to functions or
  handles until §4.2 is closed. Deterministic across processes today
  (P9), and it makes the canon contract testable by a property rather
  than by inspection.

  **Maps are excluded until Phase 0.** An earlier draft listed maps here
  and recommended the digest for dedup; PR #376's review caught the
  contradiction with this note's own P1 result. `{a:1 b:2}` and
  `{b:2 a:1}` are `deq` and digest differently, so for maps the digest is
  a **byte-serialization checksum, not content identity** — sound for
  "did these bytes change?", unsound for "are these the same value?", and
  dedup is the second question. Either gate map hashing on Phase 0
  canonicity, or ship it for maps with that limitation stated in the word's
  own documentation. It must not be described as content identity before
  canonicity lands.
- **Phase 3 — world-digest keying for compiled units** (option b).
  Targets the `DepsFresh` hot-path cost and the user-poly refusal. Needs
  Phase 0 and the Stage 3 refusal lifted; needs *none* of steps 3–6.
- **Phase 4 — definition digests** (option a), with the full §4.2
  pipeline. Only if free renames, test caching, or code mobility are
  wanted badly enough to reopen call-time binding.

Phases 1 and 2 are small and unblocked. Phase 3 is the one with a
measurable payoff against a recorded defect. Phase 4 is a language
programme and should not be entered by accident.

## 5. Open questions

1. **Canonicity vs. map order** (Phase 0). Sort keys, or narrow `deq`?
2. **What does `def` mean once definitions are addressable?** F1 is
   currently an argument about which binding a call sees; under Phase 4 it
   becomes an argument about whether `def` rebinds a name or mints a
   definition.
3. **What is a `Store`'s source form?** ADR-015 §5 leaves this open and
   notes it may surface a language question — what does re-parsing a live
   socket handle even mean? A digest needs an answer; "refuse to hash
   handles" is a legitimate one.
4. **Which referent, under type-directed dispatch?** Step 5 presumes a
   call site has *a* referent. With signature-set dispatch it has a set,
   and the digest must cover the set — or the scheme must restrict itself
   to option (b), where names are never resolved.
5. **Does the digest need to be collision-resistant?** For the artifact
   digest, yes — it is a supply-chain control. For a compiled-unit cache
   key the earlier draft said a fast non-cryptographic digest "is
   sufficient"; PR #376's review corrected that, and it is worth stating
   sharply: **a collision in a compiled-unit key serves code compiled for
   a different definition** — a miscompile, not a stale read.
   `BinUtil.fnv64` clears the sign bit (`lang/go/modules/binary.go`
   `sign63Mask`), so it supplies **63 bits**, and FNV is not collision
   resistant at any width. A fast digest is therefore admissible only as
   an **index**, with the lookup verifying the full key — the normalised
   canon and assumption set — on hit, or else the key must be a
   collision-resistant digest from `boru:crypto` (designed, unbuilt).
   Specify the verification; do not treat the digest alone as the key.
   These are different requirements on the same word, which is another
   reason §4.1's split has to hold.

## 6. Rejected

- **The codebase as a database (UCM, scratch files, `add`/`update`).**
  boru's delivery model is a single binary over text files, with `boru
  fmt`, an LSP, `editors/`, a wasm playground, git review — and a
  knowledge graph whose structural evidence is read *from the file tree*
  (`go.work`, every `go.mod`). Moving definitions into a database
  invalidates that evidence model. Adopt the identity, keep the files;
  nothing in §4 requires otherwise.
- **Hash-based type identity.** Unison hashes types, which is why it needs
  a `unique` modifier — and `unique` is its default, an admission that
  structural type identity is usually wrong. boru has already chosen
  nominal: ADR-010, ADR-012, FixedID stability the AOT codec depends on,
  and sealed nominal classes. Hashing *terms* must not drag type identity
  along with it.
- **Immutable definitions as a language rule.** Incompatible with mutable
  `Store`/`Array`/class instances and a live-rebinding `def` in a REPL.
  Available as an *internal* invariant for compiled units (Phase 3), not
  as a surface rule.
- **Inlining the closure** (§4.3 option c) — destroys late binding.
- **Abilities / algebraic effects.** Out of scope here; analysed in
  `legacy/effect-oriented-programming-in-boru-report.0.ignore`, whose idea #1 (effect
  rows inferred by the carrier checker) is the same design Unison ships,
  and whose recorded ceiling — no multi-shot continuations under the tape
  model — is unaffected by anything in this note. Phase 4's test-caching
  payoff does depend on that work, because caching a result asserts
  determinism.

## 7. Evidence index

- `legacy/unison-in-boru-report.0.ignore` — the applicability study, corrected in
  place by the probe.
- `legacy/unison-hash-identity-probe.0.ignore` — the measurements (P1–P10).
- `scripts/hash-identity-probe.boru`, `scripts/hash-identity-probe.sh` —
  the runnable probe.
- `RELOAD-INVALIDATION.0.md` §1, §2.2, §3 (F1), §4, §5 — the hot-path
  cost, the divergence, the prior-art survey.
- `AOT-COMPILE.0.md` §3.1–3.3 — the symbolic-reference problem and the
  refusal whitelist.
- `CANON-ROUNDTRIP.0.md` + ADR-015 — the rendering contract and what it
  deliberately does not say.
- `MODULE-SECURITY.0.md` §9.3 — "no signing, no lockfile, no content
  hash", and why the manifest depends on fixing it.
- `boru-vendor.0.md` §5 — the Merkle `tree` hash already specified for the
  vendor axis.
- `HOT-CODE-LOADING.0.md` — reload as a protocol, the consumer of Phase 4.
