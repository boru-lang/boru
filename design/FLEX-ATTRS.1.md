# FLEX-ATTRS.1 — insertion order universal; WeakFlexMap; sorted types

> **Implementation note (2026-07-24):** D1 is IMPLEMENTED on this
> branch. Mechanism: the **grammar-BC channel** (§3.2 route 1), not the
> upstream jsonic bump — tabnas/jsonic v0.2.0 is pinned and reached
> read-only from the module cache, so `pairval` can't be edited here; the
> BC route is designed against a swappable `Meta["ko"]` contract so an
> upstream bump can replace it later. The `boru_ck` leak (§2.6) is fixed
> first (a `defer delete(r.K, "boru_ck")` in the computed-key BC), then a
> `ko` order channel records each pair's final union key in source order
> (first-position/last-value dedup, base-name normalization for
> shorthand/optional, verbatim quoted/computed keys — the five §3.2
> special cases). `convertMapData` and `convertTypedMap` iterate the new
> `orderedKeys(union, ko)` (source order, sorted tail only as the
> determinism fallback for maps that reach the path off the pair
> grammar); `sortedKeys` and its three tests are gone (ADR-008). The
> renderers were already `Keys()`-based, so no Go render change was
> needed; the **TS engine** flips its three render sites
> (`canon.ts` map + options, `value.ts` toString) from `sortedKeys()` to
> `keys()` — `sortedKeys()` stays only for order-insensitive map equality
> (`coretype.ts`). Migration (§3.3): old-vs-new differential over the full
> corpus, every differing row machine-verified as a pure key permutation
> before its expected column flipped — 14 exact-canon rows (map,
> edge-containers-3/4, refine-flex, bytecode-migrated, basic, behave,
> syntax, frontier-do-catch, structure ×2), 1 Go assertion
> (behave_reify_wave3), 3 doc examples (REFERENCE/TUTORIAL/HOWTO), the
> nodes.tsv representation-vs-canonical doctrine (§3.4), the interp/
> structure prose, and FORMAL-SPEC.md open question #3 (now closed). The
> §3.5 normative decisions are pinned: duplicate-key first-position/
> last-value, source-order value-eval (first-raise-wins), the
> order-insensitive deq/cmp half of the equality matrix, and record/class
> order-identity (reordered bodies non-unifiable; same-content instances
> stay deq-equal). D3 (below) was IMPLEMENTED earlier.

> **Implementation note (2026-07-22):** D3 is IMPLEMENTED on this
> branch, extended per maintainer instruction to all three flex kinds
> — `WeakFlexMap` (FixedID 123), `WeakFlexList` (124), `WeakFlexXml`
> (125). Collected list/xml elements SPLICE-COMPACT at observation
> points (survivor order preserved — a hole would violate the
> no-sparse-lists invariant). The §4.4 refusal domain, snapshot-at-
> funnel representation, `weak_value_error` diagnostics, and the §4.7
> propagation matrix (adopt=share, `node`=strong Map, `flex`=strong
> FlexMap, `clone`=weak) are as designed; spec rows live in
> `lang/spec/weak-flex.tsv` (live-binding discipline, zero vanish
> rows), collection is covered by GC unit tests in
> `eng/go/weak_flex_test.go`. The 2026-07-23 review round added:
> `cmp` sees the swept CONTENT view (mode-blind ordering, not the
> `(make WeakFlex…)` canon), the check-time mirrors are live
> (`weak_value_error` via `weakValueMirror`; typed writes via
> `d2CheckWrite` — `make`'s check residual now carries the element
> tag for flex AND weak targets), and typed weak containers enforce
> `{:T}`/`[:T]` on set/append exactly like the flex column. D1
> (universal insertion order) is now implemented too (see the
> 2026-07-24 note above); D4 (sorted types) remains
> unimplemented — but ALL THREE of D4's blocking seams are now GONE:
> open-words rev 2 (design/OPEN-WORDS.1.md) makes anchored subtype
> overrides dispatch, its §7b follow-ups fixed the flex retag and
> pinned container predicates (lang/spec/refine-flex.tsv), and the
> third seam — base-dispatch delegation for override bodies — is
> closed by the `as` dispatch-ascription word (OPEN-WORDS.1 §9;
> lang/spec/as.tsv; the full pure-boru SortedFlexMap acceptance runs
> in lang/go/test/sorted_user_test.go). A super-style word was built
> first and rejected on DX grounds (§7b.1-3 keep that history).

Status: **design under examination — decisions taken; D3 implemented
on this branch** (2026-07-22). Revision of
[legacy/FLEX-ATTRS.0.ignore](legacy/FLEX-ATTRS.0.ignore): after reviewing that note's
analysis, the maintainer took four decisions (§1) that replace the
instance-attribute mechanism. This note records the decisions, designs
each piece, and re-runs the same four-perspective examination —
implementation, type system, gotchas, language design — ending in open
questions. It distinguishes throughout what is **verified on the live
tree / binary** from what is proposed. Not an ADR (no ADR entry without
explicit maintainer instruction). No code changes accompany this note.

Grounding — verified against the live tree and a fresh binary
(2026-07-21/22, HEAD c2b4c9f):

- `eng/go/parser/parse.go:159,862-876,1106`, jsonic v0.2.0
  `grammar.go:15-42,273-292` (the literal sort and the ListPair channel)
- `eng/go/equal.go:15-32,163,191-192`, `eng/go/util.go:106-140`,
  `eng/go/shape.go:94-114`, `eng/go/canon.go:145-191` (the exact-match
  flex predicates)
- `eng/go/signature.go:553-592`, `eng/go/word_extend.go:36-40,125,299`
  (locked-first dispatch, module word extension)
- `eng/go/typetable.go:642-796`, `lang/go/test/fixedid_stability_test.go`
  (FixedID space), `lang/go/native/native_keyval.go` (KeyVal — the
  tree's one existing nominal Map child, used below as a live probe)
- `design/FLEX-NODES.10.md`, `design/legacy/FLEX-ATTRS.0.ignore` (prior designs)


## 1. The decisions

Superseding the FLEX-ATTRS.0 `attr` proposal:

- **D1** — all Nodes are **insertion-ordered in all cases**: the
  definitive default. Map literals preserve source key order; the
  parser's literal key-sort goes away.
- **D2** — the instance-**attributes idea is scrapped**.
- **D3** — a **`WeakFlexMap`** type is added as a **subtype of
  FlexMap** (values weak w.r.t. garbage collection).
- **D4** — a **`sorted`** utility word in a new **`boru:node-util`**
  module returns **`SortedMap` / `SortedFlexMap`** — module-exported
  subtypes of Map / FlexMap — as clones with keys re-sorted, with set
  and key-iteration overrides that maintain sorted key order.

D1 adopts exactly the "cheap endpoint" FLEX-ATTRS.0 §6.4 recommended
(the JS/Python convergence). D3 adopts the "weakness is a separate
type" half of §7's recommendation, but as a fully-inheriting subtype
(the Java `WeakHashMap` shape — more precisely Python's
`WeakValueDictionary`, since values are weak) rather than the
restricted opaque type (the JS `WeakMap` shape). D4 puts sorted maps
in a library, which is where Python/JS/Java/Clojure all keep them.


## 2. What exists today (verified — new findings beyond FLEX-ATTRS.0 §2)

1. **Why fn params keep declared order while `{a:1 b:2}` literals
   don't — solved.** It is jsonic's **ListPair mode**
   (`parse.go:159`): in LIST context each `k:v` pair becomes its own
   single-entry `map[string]any` and the enclosing list carries the
   order (`[zz:1 aa:2]` → `[{zz:1} {aa:2}]`, verified). The sort at
   `parse.go:876` never bites a one-key map. Explicit `{…}` literals
   instead accumulate into ONE Go `map[string]any` at the jsonic
   boundary — where iteration is RANDOMIZED, so the parser's sort is a
   **determinism fix, not a style choice**: it cannot be deleted, only
   replaced by an order source. Records via list-pair bodies
   (`refine Record [x:… y:…]`) already render declared order today;
   the sort is confined to explicit `{…}` conversion.
2. **A FlexMap subtype half-works today** — probed both with
   `refine FlexMap` and with the kernel's own `KeyVal`
   (`Node/Map/KeyVal`, FixedID 5002, the one existing nominal Map
   child). Dispatch inherits (`set`/`get`/`keys` all fire via
   ConformsTo), the tag survives flex `set` — but `x is FlexMap` →
   false, `deq` against an equal-content flexmap → **false** (falls to
   the `%v` default = payload-pointer compare, `equal.go:191-192`),
   `node x` → **identity on the live container** (no snapshot), and
   the value renders as `SFM({0xc0005867b0})` — a raw Go pointer.
   Root cause, one pattern: the flex predicates are deliberately exact
   (`IsFlexMap` = `Parent.Equal(TFlexMap)` + MapPayload probe,
   `util.go:106-115`; `nodeFamily` exact "so Inspect/Args keep their
   own identity", `equal.go:19-32`; Shape's map family exact,
   `shape.go:94-96`; canon's branches exact, `canon.go:145-171`).
3. **The locked-first theorem.** `CompareSignatures` sorts Locked
   (native) sigs strictly first (`signature.go:553-566`); every module
   sig transplanted onto a core word is force-unlocked
   (`word_extend.go:125`). Since any FlexMap-subtype value also
   matches the earlier locked `[.., TFlexMap]` sig, a module's
   specialized `set` sig for a FlexMap subtype is **unreachable by
   theorem** — it installs silently dead. (Module extension of core
   words is real and works for DISJOINT types — the matrix/temporal
   precedent; it fails precisely for subtypes.) Verified via KeyVal:
   `kv set x 9` is claimed by the kernel Map sig and returns a plain
   `Map` — tag gone.
4. **Two JSON serializers with opposite policies ship today**:
   `StructUtil.jsonify` sorts (encoding/json; the `$class`-leads
   contract depends on `'$' < letters`, `jsonify.go:34-36`) while
   `IO.write {fmt:"json"}` preserves insertion order. The kg pipeline
   content-addresses assertion ids on jsonify's sorted output
   (`canon-of = StructUtil.jsonify`, `kg/identifiers.boru:36-71`).
5. **`Debug.gc` exists** (`boru:debug` → `runtime.GC()`;
   module-debug.tsv pins only `typeof` because the numbers are
   nondeterministic). Exactly one GC-dependent Go test exists
   (`bytecode_allocguard_test.go:26`).
6. A pre-existing parser bug surfaced during verification: the
   computed-key `boru_ck` K-flag **leaks to subsequent pairs** —
   `def k 'zz' def aa 5 {[k]:1 aa:2}` → `{5:2 zz:1}` (the bare key
   `aa` is wrongly evaluated as computed). The D1 order channel sits
   directly on this machinery.


## 3. D1 — insertion order universal

### 3.1 Assessment

D1 is the **removal of an incoherence**, not a feature: fn params,
record list-pair bodies, and XML literal attributes are all
source-ordered today; incrementally-built flexmaps and their `node`
snapshots already expose insertion order; only explicit `{…}` literals
lose order, and only at the jsonic map boundary. That is the strongest
possible position for a breaking change, and the audit confirms the
blast radius is small because spec authors already wrote multi-key
literals in sorted source order defensively.

### 3.2 Mechanism

Two viable routes for the order source (a third — re-typing `{…}` onto
the ListPair channel — is rejected: it changes the node shape every
existing grammar extension writes into and loses duplicate handling):

1. **Grammar-BC channel** (`Meta["ko"]`, modeled on the ordered
   `Meta["sh"]` list, `grammar.go:992-1006`): works against pinned
   jsonic v0.2.0 but carries five special cases across four grammar
   extension paths — the `{foo?:v}` double-fire hazard, the live
   `boru_ck` leak (§2.6, must be fixed first), base-name normalization
   for `{foo/r}`/`{foo?}`, verbatim qk/ck keys, and duplicate dedup.
2. **Upstream jsonic write** — `pairval` (jsonic `grammar.go:15-42`)
   has the key and the MapRef in hand at exactly ONE site; appending
   to `MapRef.Meta["keyorder"]` there eliminates every subtlety, and —
   decisive — it is the only route that also reaches `SafeParse`, the
   json5/jsonc plugin kinds, and the `parse`-word boundary, which the
   BC channel cannot. tabnas/jsonic is in-house. **Leaning: bump
   jsonic**; if the BC shim ships first, design `convertMapData`
   against the Meta contract so the shim is swappable.

Consumer side: `convertMapData` iterates the ko order (interleaving
synthesized shorthand/optional keys at their source positions),
`convertTypedMap` (`parse.go:1106`) likewise; the `sortedKeys` helper
and its three tests die with it (ADR-008). Downstream is already
ready: `autoEvalMap`, the compiled `RecordMakeMap`/`vmMakeMap` replay,
and every renderer iterate `Keys()` — grep-clean of sorts.

### 3.3 Blast radius (audited mechanically, 538 raw hits triaged)

**14 exact-canon spec rows** (10 lang-side, 2 shared eng/spec rows × 2
engines; 4 are unify-output rows), **1 Go assertion**
(`behave_reify_wave3_test.go:140-142`), **3 runnable doc examples**
(REFERENCE.md:307, TUTORIAL.md:282, HOWTO.md:228), **~11 prose sites**
(including the normative "emitted in sorted order" sentence at
`eng/spec/nodes.tsv:9-11`, and FORMAL-SPEC.md:915's open question #3,
which D1 closes), and the **TS engine's three render call-sites**
(`canon.ts:185,190`, `value.ts:365` — TS sorts at render, a different
architecture; it must flip to insertion `keys()` in the SAME change or
the shared-corpus cross-engine gates fail). **Zero in-tree semantic
flips** — but four contracts change and need normative rows (§3.5).

Migration discipline: build old and new binaries, run the corpus under
both, and machine-verify every differing row is a pure key permutation
(multiset of `k:v` pairs unchanged) before patching expected columns —
never blind-regenerate, or D1 launders real regressions. Everything
lands atomically (parser + rows + both engines): specrunner is exact.

### 3.4 Scope doctrine (the maximal-reading trap)

"In all cases" must be pinned to **Node construction, mutation,
iteration, and render**. The order-destroying boundaries stay
sorted-canonical, and this is a necessity, not a dodge:

- `StructUtil.jsonify` sorts — kg assertion ids HASH that output
  (§2.4); insertion-order jsonify makes ids construction-order-
  dependent and breaks the collision detector's premise (equal
  content ⇒ equal id). The `$class`-leads contract also rides the
  sort.
- StructUtil rebuilds, tabnas decoders, `convert Map` of Store: the
  upstream representation is an unordered Go map — sorted output is
  the determinism fallback where source order is unrecoverable.

The doctrine, borrowed from the precedents (Python/JS emit insertion
order; sorted JSON is opt-in canonicalization, RFC-8785-style):
**representation order** (Keys(), render, canon, iteration, eval
order) vs **canonical order** (jsonify, decode boundaries, Ideal
projections) — two named functions, not one inconsistency. Write this
into the nodes.tsv prose FIRST so no one "helpfully" sweeps the
serializers later. (`IO.write` emit = representation-faithful;
`jsonify` = canonicalizing; document jsonify as such.)

One asymmetry becomes visible and must be documented: the same text
parses ordered as a boru literal but sorted through `StructUtil.parse`
(until the jsonic hook reaches it — the upstream route in §3.2 closes
this too; decoded json/yaml/toml stays sorted regardless, upstream
maps are unordered).

### 3.5 New normative decisions D1 imports (each needs a spec row)

1. **Duplicate literal keys**: `{zz:1 aa:2 zz:3}` — leaning
   **first-position/last-value** → `{zz:3 aa:2}` (matches
   `OrderedMap.Set` re-set semantics and JS/Python literals; today's
   sorted output hides the question). Verified today: last value wins,
   no deep merge. A check-mode duplicate-key diagnostic is worth
   considering separately (the JS strict-mode precedent: duplicates
   are almost always bugs).
2. **Literal value-evaluation order** becomes source order
   (`autoEvalMap` walks `Keys()`) — side effects and first-raise-wins
   follow key order. This makes key order an OPERATIONAL property, not
   a rendering one; today effectful entries run in alphabetical key
   order, which no author expects. Pin under both interpreter and VM.
3. **The equality matrix** (stated as a table, not discovered row by
   row): canon/render — order-sensitive; `deq`/`cmp` —
   order-insensitive (`mapsEqual` key-lookup; `compareMapEntries` own
   sorted copies — unchanged); unify — order-STRICT for record/class
   type bodies, order-insensitive for options. Monotonicity holds one
   way (canon-equal ⇒ deq) and the converse breaks by design
   (deq-equal values may render differently) — canon's contract is
   restated as *representation-faithful round-trip form*
   (`parse(canon(v)) deq v`), not a content fingerprint.
4. **Record/class order-identity**: `class {b:… a:…}` and
   `class {a:… b:…}` become distinct, non-unifiable types (they join
   `refine Record`'s existing order-strictness — verified:
   `unify-fail` on reordered record bodies today). For records this is
   FORCED by positional-make soundness (`make R [7 8]` fills schema
   order); classes inherit it for uniformity. Corpus scan: zero
   in-tree collisions. Instances of order-variant types remain
   deq-equal — type identity finer than value equality; pin it so
   nobody "fixes" it. Also newly visible: instance render/inspect/
   table-column order becomes declaration order everywhere (mostly
   the point), and a child-class override keeps the PARENT's field
   position (`AllFields` set-on-existing, `value.go:915-931`) — a
   subtle surprise worth one row.

D1 side benefit: post-D1, a byte-order `sorted` word (§5) exactly
reproduces today's literal renders — the recovery path for anyone who
depended on sorted output, and D4's migration story.


## 4. D3 — WeakFlexMap, a nominal kernel subtype

### 4.1 Type addition (forced path)

Kernel `builtinDecls` row — `{Path: "Node/Map/FlexMap/WeakFlexMap",
FixedID: 123, Rank: 30_221_000_000}` (123 is the next kernel FixedID;
depth-3 children take +1e6 spacing, precedent `Ideal/Resource/Entity`)
plus the fixedid_stability snapshot line. `RegisterExternalBuiltin` is
mechanically possible (KeyVal precedent) but wrong here: D3's
semantics require kernel branches (nodeFamily, Shape, canon, clone,
MakeNodeHandler, adoption) that cannot name a lang-registered type —
the eng/go/CLAUDE.md kernel-residency rule applies exactly. KeyVal
survives as an external Map child only because nothing in eng
discriminates it.

### 4.2 The exact-match seam bill — the real cost, and a prerequisite

§2.2's probes prove every seam fires today. ~14 kernel + ~4 lang sites
conflate two relations under one predicate shape: "is in the flex-map
FAMILY" (semantic — should be subtype-closed) vs "is exactly the
FlexMap REPRESENTATION" (exact — AsMutableMap/payload sites). The bill
is paid once, as a **behavior-neutral family-widening refactor landed
BEFORE any container subtype exists** (while only TFlexMap inhabits
the family — zero behavior change, reviewable in isolation):

- Widen via a payload-blind `isFlexMapFamily(t)` (descends-from-
  TFlexMap — never blanket `ConformsTo(TMap)`, preserving the
  documented Inspect/Args identity carve-out): `nodeFamily`, Shape's
  map family, `IsFlexNode`/`containsFlex`, the dynScope-rescue gate
  (`emit.go:1585`), unify's shape routing, and the const-intern
  no-dedupe gate (`emit.go:6193` — exact TList/TMap today; it already
  misses FlexMap, a latent shared-pointer miscompile this fixes).
- Representation-touching sites get explicit arms when the type
  lands: canon branch, clone (the default arm SHARES pointers — an
  aliasing bug if left), FlexDeepCopy/NodeDeepCopy, MakeNodeHandler's
  target switch, FreshenDefault re-establishment, unify's typed-map
  remint, AdoptIntoFlex.

Ship condition: **atomicity**. A partial ship leaves silent
pointer-identity `deq` and canon-colliding renders in a release —
the ADR-005 failure mode in its worst, silent form.

### 4.3 Representation: dedicated payload, snapshot-at-funnel

MapPayload reuse founders on the funnel problem: `AsMap` returns the
raw `*OrderedMap` and dozens of sites iterate `Keys()`+`Get()`
directly, several ignoring the ok flag — any weak box or zero Value
leaking through corrupts the stack, and iterate-then-get is unsound
under ANY in-map representation (a key alive at `Keys()` can be dead
by `Get(k)`). The sound design:

- A sealed `WeakFlexMapData{keys []string; slots map[string]weakSlot}`
  payload. The `AsMap` arm **sweeps** (`ptr.Value()==nil` → splice)
  and **materializes a strong snapshot OrderedMap** — one operation
  observes one consistent world. This is CPython's
  `WeakValueDictionary` iteration-guard design; 20 years of
  production precedent.
- `AsMutableMap` REFUSES the payload → every unported mutating
  handler fails LOUDLY ("not a map payload"), never silently
  mis-shares — the correct ADR-005 posture. This forces the one new
  write surface: a dedicated `set [String/Atom, Any, WeakFlexMap]`
  sig pair beside the FlexMap sigs (`set` is the whole map-column
  write surface today — no `del` exists).
- Reaping is **lazy sweep at the read funnels only**.
  `runtime.AddCleanup` is disqualified: cleanups run concurrently and
  OrderedMap/WeakFlexMapData are unsynchronized — it degenerates into
  lazy sweep plus a second nondeterminism source.
- Weak slots: `weak.Pointer[T]` needs concrete T — a five-arm sum
  over the pointer-backed identities (`*OrderedMap` for flex/class
  fields, `*FlexListData`, `*FlexXmlData`, `*StoreInstanceInfo`) with
  per-kind revival templates (ClassInstanceInfo is by-value around
  its Fields pointer). Budget for it.
- Pleasant side effect: the distinct payload is automatically
  excluded from `isInertConst`'s MapPayload-shape admission — a weak
  map can never bake as an immortal const (which would pin its
  contents forever — the opposite of weak).

### 4.4 The weak domain — DECIDED (maintainer, 2026-07-22): Python-style refusal

The value domain follows Python's weakref rule (weakness requires
reference identity; the rest is refused, not silently reinterpreted):

| Stored value | Fate |
|---|---|
| Scalar branch (incl. BigInt/Decimal, Time, Microns) | accepted, **strong** — no GC identity, nothing to track |
| Mutables: flex nodes (incl. WeakFlexMap), Store, Resource, class instances | accepted, **weak** — dropping the handle is the eviction event |
| Immutable Nodes (Map, List, Xml) | **refused** — value-semantic, no coherent referent |
| fn values, type literals, Error, ExtensionPayload | **refused** (v1; fn values revisited if observer registries demand, Extension via per-host opt-in) |

This DISSOLVES the dead-on-arrival axis: `AdoptIntoFlex` never runs
at the weak funnel — no adopted copy can enter, every container value
in a weak map is a shared live handle. Construction
(`make WeakFlexMap {…}`) applies the same classification. The recipe:
*cache it evictably → `flex` it and hold the handle; keep it until
`del` → scalar, or an ordinary FlexMap.*

**Enforcement is check-time, with one wrinkle**: a narrow ACCEPTING
sig alone cannot refuse — a plain-Map value that fails it falls
through to the inherited `[.., TFlexMap]` sig (the weak map conforms
at the receiver slot) and dies in `AsMutableMap` with a baffling
message. So the kernel ships explicit REFUSAL sigs
(`set [Key, Map|List|Xml, WeakFlexMap]` raising the teachable error —
the Micron erroring-`set` precedent, native_storage.go:131-148).
Per-position specificity orders the ladder for free (flex-value
accepts above Map-refusals above the classifying Any-fallback), and
the static half costs nothing new: a matched sig that unconditionally
raises is exactly what the check-mode guaranteed-error mirror
machinery reports (`CheckDiagnostic.RuntimeMirror`) — flagged at
check time whenever the value's type is statically known, the same
message at runtime otherwise. Export the accept-union as a named type
(`Weakable`) so params and `describe set` can state the domain in one
word. Bonus: the refusal rows are deterministically spec-able —
proper positive+negative TSV pairs at the domain boundary.

**The refusal error is fully customisable** — the diagnostic
machinery is multi-channel (`BoruError.Notes` → `= note:` lines,
`Suggestions` → `= help:` fixes, primary caret + secondary labeled
spans; `eng/go/boru_error.go:18-53`, `diag_render.go`), with live
precedents of the teachable-refusal pattern at the FlexList bounds
error ("use append to grow a FlexList; sparse FlexLists are an
error", `native_storage.go:608-612`, via `r.BoruErrorHint`) and the
Micron erroring sigs. Proposed rendering, pinned by a negative spec
row per refused kind:

```
error: [boru/weak_value_error]: set: cannot store an immutable Map in a WeakFlexMap
  --> 2:11
  2 | set cfg/q {mode:'fast'} w
    |           ^^^^^^^^^^^^^ this Map is a value, not a handle
  = note: weak entries live only while the program keeps another
          handle to the stored value alive
  = note: an immutable Map is a value — it has no independent
          identity the collector could track, so a weak entry for
          it could never be kept alive by anything you hold
  = help: store a mutable handle: set cfg/q (flex {mode:'fast'}) w
  = help: or keep it in an ordinary FlexMap if it should live until
          deleted — scalars (42, 'text') are stored strongly and
          are fine here
```

The check-mode mirror carries the same rendering statically, and the
Any-slot fallback sig classifies-and-raises, so EVERY refused kind
(Map, List, Xml, fn values, type literals, Error, Extension) gets its
tailored variant — nothing falls through to a bare `no_signature`
candidate dump.

Residual asymmetry, to be stated in bold in the docs: in a mixed
cache, handle entries evict and scalar entries are immortal-until-
`del` — the one cell where lifetime still depends on stored kind.
(Full-Python — refusing scalars too — was considered and set aside:
scalars cannot dangle, and refusing them only forces boxing.)

### 4.5 What is spec-able (and what never will be)

**No positive vanish row can exist, ever.** Even with `Debug.gc`,
whether a dropped binding's entry survives depends on interpreter
internals (shadowed-binding retention in DefTable backing arrays,
resolved-arg slices, VM registers and const pool) — the same row can
pass interpreted and fail compiled: exactly the differential the
bytecode parity suites forbid. What the nominal type DOES make
spec-able — and this is the half of FLEX-ATTRS.0 §7's rejection that
genuinely dissolves — is everything else: typeof/is/make rows,
set/get/size/keys/render/canon/deq rows where the row keeps a live
binding to every stored handle (live bindings pin deterministically in
every mode), adoption/materialization rows, refusal rows. Collection
behavior itself is Go-unit-test-only: weak.Pointer clearing is
deterministic after `runtime.GC()` once the referent is truly
unreachable (one cycle suffices — no resurrection for weak pointers;
allocate in a helper so the test frame holds no strong copy;
`bytecode_allocguard_test.go` precedent), satisfying ADR-008 without
covergate entries.

Check mode is the decisive win: the construction ReturnsFn can branch
on `ConformsTo(TWeakFlexMap)` and record per-key `T ∪ None` joins
(preferred over blanket `ValsPoisoned` — "if present, the written
bound holds" survives collection, since collection removes entries
rather than corrupting them). FLEX-ATTRS.0 proved the attr design
could never surface weakness statically; the type does it for free.
And the LSP objection is smaller than it looks in boru specifically:
map `get` is already None-partial (absent keys read as None), so the
static contract of `get` is unchanged — what weakens is the checker's
frame property, which is exactly the subsystem that can now see the
type. (In a total-read language the subtype WOULD be an LSP
violation; record this reasoning, it is load-bearing.)

### 4.6 The five FLEX-ATTRS.0 §7 axes, re-examined under the subtype

| Axis | Status under D3 |
|---|---|
| 1. No referent for scalars | Unchanged in substance; now LEGISLABLE — one deterministic decision point (the weak `set`) and a nominal name under which to document the carve-out |
| 2. Adoption makes plain stores dead-on-arrival | **Cured by the §4.4 refusal decision** — plain containers never enter; no adopted copy can exist in a weak slot |
| 3. Reachability = interpreter internals; run/check/compiled divergence | Unchanged; bounded (lazy sweep: entries vanish only at observation points) but not cured — contract must say "MAY be absent; neither removal nor retention guaranteed" |
| 4. No deterministic spec row | Unchanged for collection (no vanish rows, ever); CURED for the entire non-collection surface (§4.5) |
| 5. deq time-dependence | Mid-comparison corruption CURED (snapshot funnel); between-call instability remains — Java-equals semantics, documented |
| (Checker blindness — §4.1 of .0) | **Fully cured** — the load-bearing half of the original rejection |

### 4.7 The recorded push-back, and the acceptance contract

The examination's honest verdict: the subtype buys iteration, size,
render, content-deq, and flex-graph adoption — **precisely the
operations whose weak semantics are nondeterministic** — at the price
of the §4.2 bill. The alternative FLEX-ATTRS.0 leaned to, an opaque
Ideal registry (`make WeakRegistry`; `put/get/del/has`; no
size/keys/each/render; identity deq; Store-style opaque canon),
serves the classic use cases (caches, interning, observer registries)
with near-zero kernel churn, no snapshot machinery, no silent-failure
modes, and the identical spec limitation (collection is equally
untestable in both designs — this axis does not discriminate). JS
refused weak iteration deliberately; Java/Python allow it and their
docs are standing warning labels; boru's values (exact-canon specs,
compile==interpret parity, loud failures) point toward refusing.
**If** live-cache introspection via the Map vocabulary is a hard
requirement, WeakFlexMap is shippable under this contract: the §4.2
atomicity condition; the §4.4 domain table; normative
"operational-nondeterminism" prose (the WeakHashMap warning label as
spec text — a new category for the spec, created deliberately);
canon declared a **stable snapshot render, not a semantic
round-trip** (the Node column's first, stated as such — Store/class
already canon descriptively, but in the Ideal column); no vanish
rows; `T ∪ None` shape joins; `node w` = strong immutable
materialization as the escape hatch. Also confirm **weak VALUES
only** (keys are OrderedMap strings — weak keys would be a different
container entirely), and note the TS engine cannot force GC: eng/ts
either ships the type strong-with-a-tag (a documented
quality-of-implementation fork) or D3 rows stay out of the shared
corpus.

Propagation matrix (each cell one decision + a paired spec row):
`flex w` → strong mutable FlexMap snapshot (demotion, loud in docs);
`node w` → strong immutable Map (requires the §4.2 widening — today
it returns the live container); `clone w` → weak clone re-boxing live
slots; unify `{:T}` remint and typed-param adoption → carry weakness
or refuse, never silent demotion; storing `w` into another flex tree
→ pointer-share (it IS a reference cell). Sweep-vs-report granularity
(does `size` splice dead slots or report the unswept view) must be
picked; leaning: sweep (Python behavior).


## 5. D4 — `sorted` and the sorted types

### 5.1 As worded, blocked twice (verified)

1. **The module `set` override is dead on arrival** — the
   locked-first theorem (§2.3). The transplanted
   `[String Any SortedFlexMap]` sig is provably unreachable; bare
   `set` on a SortedFlexMap dispatches the locked kernel FlexMap sig
   and **appends**, silently breaking the type's one advertised
   invariant while `typeof` still says SortedFlexMap. A `NodeUtil.set`
   twin would create a split-brain write surface (two spellings of
   `set`, opposite semantics on the same value) — reject.
2. **The immutable SortedMap tag evaporates on first use** — plain-Map
   words are copy-returning and mint fresh `NewMap`s (verified via
   KeyVal: `set` returns plain Map). A nominal subtype of a
   structural kernel type survives exactly dup/def/clone/reads. The
   kernel's structural algebra is parent-forgetting by design.

Also: "key iteration overrides" are the wrong half — iteration words
call `OrderedMap.Keys()` directly with no Behavior hook, so read-time
overrides would need a new capability consulted at every iteration
site. **Maintain-on-write wins decisively**: if the keys slice is kept
sorted at the write funnel, every reader (render, keys, each, fold,
canon) is correct for free.

### 5.2 The coherent recast

The type-theoretic reading: D4 conflates a **refinement type** (values
satisfying P now; membership by predicate; no maintenance claim) with
an **invariant type** (all values satisfy P always; requires write
mediation) — and only the first is expressible at the module layer.
A type whose invariant depends on which imports are in scope is a
convention, not a type. So:

- **`sorted`** — a pure clone-resort word in `boru:node-util` (a new
  module; full registration checklist applies): Map → re-sorted plain
  clone; FlexMap → FlexDeepCopy + resort. **Byte-order comparator
  only** (verified: byte order exactly reproduces pre-D1 literal
  renders AND agrees with string `cmp` — `'Z' cmp 'a'` → -1). A
  comparator-fn parameter is rejected for the typed form: it can ride
  neither canon nor decidable membership (the same unexpressibility
  that killed attrs). A comparator-taking one-shot reorder returning
  a plain map could be a later, separate word.
- **`SortedMap`** — a **content-predicate member type**
  (`MintMemberType(TMap, keysAscending)` — the boru:io StreamKind /
  minilang pattern): `x is NodeUtil.SortedMap` decidable by content,
  sig-slot usable in module words, canon-safe (values canon as plain
  maps), module-absent-safe, equality-neutral. Subset semantics — a
  coincidentally-sorted map inhabits it — is a FEATURE for a property
  type. No maintenance claim, honestly.
- **`SortedFlexMap` (maintained)** — if wanted at all, it is a
  **kernel feature** whose constructor happens to live in a module.
  Two escapes, both demoting D4-as-worded: (i) a **KeyOrderer
  write-mediation capability** on TypeBehavior, consulted at
  `setFlexMapHandler`'s single `m.Set` call site (+the future `del`)
  — one seam, module supplies policy, kernel keeps enforcement,
  compiled path inherits for free; closest to "the type owns its
  write path" (TreeMap/Clojure's actual shape). Leaning: this. (ii)
  locked kernel-registered sigs over a kernel-known type (the KeyVal
  route) — works, but hard-codes one subtype into the word table.
  Either way the type must be kernel-known (the §4.2 seam list
  applies to it identically — `node sortedFm` returning the same
  mutable container is a flex-invariant violation today), and
  "module-exported" honestly reduces to: the module exports the
  constructor word and the type literal.
- **Composition**: `Sorted∘Weak` cannot exist (sibling nominal
  subtypes, single-parent tree). Pin `sorted w` on a WeakFlexMap:
  strong SortedFlexMap snapshot (documented strengthening) or loud
  refusal — never silent.

### 5.3 Naming

`sort` (existing, maps: by VALUE — verified) vs `sorted` (proposed: by
KEY) is one letter apart with opposite axes, and a Python reader
expects `sorted(dict)` to return a list. ADR-001's "same value type,
different meaning" confusability clause names exactly this hazard; the
`NodeUtil.` prefix mitigates, an axis-explicit name (`sortkeys` /
`bykeys`) removes it. Flag for decision. `WeakFlexMap`: a Java reader
assumes weak KEYS (WeakHashMap); D3 means weak VALUES
(WeakValueDictionary) — docs must lead with "values are weak" (or
consider naming the axis). `SortedMap`-as-predicate makes the name
true by construction; `SortedMap`-as-evaporating-tag would be a name
for a guarantee the system cannot keep.


## 6. The minting-layer rule (falls out of D3 + D4 jointly)

> A type lives in the **kernel** iff kernel-owned machinery must
> discriminate it — write funnels, equality/canon/shape, adoption,
> const gates, lifecycle. **Modules** may export predicates (views),
> constructors, and vocabulary over kernel containers — **never
> invariants**. The middle box — nominal, representation-identical,
> invariant-bearing subtypes of mutable kernel containers — is
> unsupported and banned by rule (both enforcement channels provably
> fail there: locked-first kills dispatch specialization;
> parent-forgetting words kill the tag).

This rule correctly predicts every case in evidence: WeakFlexMap →
kernel (payload, equality, adoption, lifecycle all diverge);
SortedMap-as-predicate → module (representation-identical, decidable
content predicate); SortedFlexMap-as-worded → the banned box; KeyVal →
legal external child precisely because nothing in eng discriminates it
and it claims no invariant. Note the earlier "deterministic → module,
GC → kernel" framing is falsified by maintained-SortedFlexMap (fully
deterministic, still needs the kernel); the rule above is the right
one. Recommend adding it, plus the family-vs-representation predicate
discipline (§4.2), to eng/go/CLAUDE.md when implementation starts.


## 7. Phasing (leaning)

1. **P1 — D1 atomic unit**: jsonic order channel (preferably
   upstream) + convertMapData/convertTypedMap + the three TS render
   call-sites + the 14 rows/1 test/3 docs/~11 prose edits + the
   duplicate-policy and eval-order rows + the scope-doctrine and
   equality-matrix prose in nodes.tsv, with the `boru_ck` leak fix
   folded in. Ship the plain `sorted` word + predicate SortedMap in
   the same release (P1.5) — it is D1's migration story and is
   regime-invariant (a sorted map's insertion order IS sorted).
2. **P2 — family-widening refactor** (behavior-neutral, while only
   TFlexMap inhabits the family): `isFlexMapFamily` through
   nodeFamily/Shape/IsFlexNode/canon-dispatch/the emit gates. The
   shared prerequisite for BOTH new types; closes the pre-existing
   emit.go:6193 FlexMap seam on its own merits.
3. **P3 — D3** WeakFlexMap (payload, FixedID 123, explicit arms, weak
   `set` sigs, shape joins, GC unit tests) — under the §4.7
   acceptance contract, or the Ideal WeakRegistry if the use-case
   answer (§8 Q1) permits.
4. **P4 — SortedFlexMap** maintained variant via the KeyOrderer
   funnel (kernel-known type; module keeps the constructor) — only if
   the predicate form proves insufficient in practice.


## 8. Open questions

D1 — insertion order:

1. **Mechanism** — bump tabnas/jsonic so `pairval` writes
   `Meta["keyorder"]` at its one site (also fixing SafeParse, the
   json5/jsonc kinds, and the `parse`-word boundary), or build the
   boru-side grammar-BC channel against pinned v0.2.0 as a swappable
   shim? Leaning: upstream. Is the `boru_ck` K-flag leak fixed in the
   same change?
2. **Scope pin** — confirm the doctrine: Node
   construction/render/iteration = insertion order; jsonify,
   StructUtil rebuilds, tabnas decode, Ideal→Node projections =
   sorted canonicalization (kg ids hash jsonify's sort; the
   `$class`-leads contract rides it). Written into nodes.tsv BEFORE
   implementation?
3. **Duplicates** — first-position/last-value (`{zz:1 aa:2 zz:3}` →
   `{zz:3 aa:2}`) confirmed, with a pinned row? And should check mode
   diagnose duplicate literal keys (JS strict-mode precedent)?
4. **Order-identity** — intended that `class {a b}` / `class {b a}`
   become distinct, non-unifiable types (joining records' existing
   strictness), pinned together with the equality matrix and the
   eval-order rows? And is the defensive sorted fallback in
   convertMapData (keys absent from the order channel) meant to be
   reachable (needs a test) or provably unreachable (needs a
   covergate proof)? ADR-008 forces one answer.

D3 — WeakFlexMap:

5. **The use case** — caches, interning, observer registries, or
   live-cache introspection? If nothing requires iterating/sizing the
   weak collection, the opaque Ideal WeakRegistry (put/get/del/has)
   serves at a fraction of the kernel bill; iteration is the one fact
   that justifies the subtype. Is a two-step path acceptable
   (registry now, subtype only if programs demonstrate the need)?
6. **Weak values only** — confirmed? (Keys are OrderedMap strings;
   weak keys would be a different container.)
7. **Adoption policy** at the weak `set` — RESOLVED (2026-07-22):
   Python-style refusal, per §4.4 — scalars strong, mutable handles
   weak, immutable Nodes and other value-like data refused via
   erroring sigs mirrored at check time.
7b. **`del` leniency (new, from the operation walkthrough)** —
   collected ≡ absent at every surface, so `del` of a collected key
   and of a never-present key must behave identically; a strict
   missing-key error would raise or not depending on GC timing (a
   nondeterministic error path). WeakFlexMap therefore forces
   `del`-on-missing to be a silent no-op; does the whole map column
   adopt lenient `del` for uniformity (leaning — with a strict
   variant possible later), or is a documented weak-only asymmetry
   accepted?
8. **Atomicity** — accept that the type lands only together with the
   full family-widening bill (§4.2), given a partial ship leaves
   silent pointer-identity `deq` in a release?
9. **Contract prose** — accept a normative "operational
   nondeterminism" category (the warning label), snapshot canon
   (non-round-trippable, a Node-column first), no vanish rows, and
   the per-boundary propagation matrix (§4.7)? And which TS fork:
   strong-with-a-tag, or D3 rows confined to lang/spec?
10. **Sweep granularity** — do size/keys/render splice dead slots
    (Python behavior; leaning) or report the unswept snapshot?

D4 — sorted:

11. **The write path** — is maintained sortedness a hard guarantee?
    If yes: KeyOrderer write-funnel capability (leaning) or
    kernel-locked sigs — and what does bare `set` do on a
    SortedFlexMap: insert-in-order, or loud error? (Never silent
    append.) If no: does the predicate SortedMap + pure `sorted`
    word cover the need?
12. **SortedMap nature** — confirm content-predicate member type
    (subset semantics accepted) over the evaporating nominal tag?
13. **Naming** — does `sorted` survive next to `sort`-by-value
    (ADR-001 confusability), or take an axis-explicit name
    (`sortkeys`/`bykeys`)? Byte-order comparator pinned by that name
    in the spec?
14. **Composition** — `sorted w` on a WeakFlexMap: strong
    SortedFlexMap snapshot, or refusal?

Cross-cutting:

15. **The minting-layer rule** (§6) — adopt normatively (with the
    family-vs-representation predicate discipline) into
    eng/go/CLAUDE.md when implementation starts?
