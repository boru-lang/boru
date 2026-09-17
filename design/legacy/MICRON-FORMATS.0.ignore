# MICRON-FORMATS — declarative `$parse` / `$emit` for user Micron kinds

> **Caveat (2026-07): `MiniLang.micron` — the user-kind literal hook this
> proposal builds on — has since been REMOVED.** The `+m` literal grammar
> is fixed to the builtin Micron leaves (Emailon, Urlon, Pathon); the word
> is a tombstone raising `mini_registry_frozen`, and the eng-side extras
> merge (`MicronLiteralSpec` / `MicronGrammarWith`) went with it. User
> Micron TYPES still exist (`def Nameon refine Micron {…}` + `make`);
> custom sources are parsed by parser fn values. Any revival of this
> proposal must first re-argue extensible `+m` dispatch against the frozen
> un-namespaced-literal decision.

**Status:** PROPOSAL (2026-07-15). Nothing here is landed. This note is a
design record: it distinguishes, throughout, what is **verified in the code
today** from what is **proposed**. The executable spec, when this lands, will
be `lang/spec/module-minilang.tsv` / a new `lang/spec/micron-format.tsv`; the
canonical word docs will come from `boru describe`.

Grounding was done against the tree at the time of writing:
`eng/go/micron_grammar.go`, `lang/go/modules/minilang.go`,
`lang/go/modules/parselang.go`, `lang/go/native/format.go`,
`eng/go/engine.go`, `eng/go/core_ref.go`, and `design/EMIT.md` /
`design/MINILANG.5.md`.

---

## 1. Why — the gap

A **Micron** (the `Scalar/Micron` structured-scalar family) is an immutable,
content-equal, orderable, property-readable scalar. The twelve builtin leaves
(Emailon, Urlon, Semveron, Cidron, …) each have a rich **canonical string
form** — `make Semveron '1.2.3-rc.1'` round-trips to `1.2.3-rc.1`.

A **user** Micron kind has no *custom* string form — and that is fine, it is
**not required to**. By default it reads and renders as a **field-map**:

```
def Ticketon refine Micron {id:String}
make Ticketon {id:'123'}      # => {id:'123'}   ← the DEFAULT: a field-map
make Ticketon 'T-123'         # => check failed  ← no CUSTOM string parser
```

The map **is** a perfectly good default: the kind gets the full Micron
*contract* (immutability, `.field` access, ordering) with zero extra work, and
`{id:'123'}` is a legible, round-trippable representation. A custom canonical
string form (`make Xon 'T-123'`, a `T-123` literal) is **optional sugar** for
kinds that want a compact spelling.

So the goal here is **not** to force every kind to have a string form — it is to
let a kind that *wants* one declare it **once** in the type definition (the same
parse+render round-trip the builtin leaves get by hand, instead of two
hand-written Go functions). **A kind that declares neither `$parse` nor `$emit`
keeps the map default, unchanged.** Nothing below is mandatory.

---

## 2. What exists today (verified)

The parse half of this is ~80% built already. Four facts anchor the design.

### 2.1 The `+m` micron minilang and the merged tabnas grammar

The shared `+m` grammar (`eng/go/micron_grammar.go`, `MicronGrammarWith`) merges
**only three** builtin leaves' tabnas grammars (`github.com/tabnas/parser/go`) —
**Emailon, Urlon, Pathon** — in that token order (`#EMAILON`, `#URLON`, extras…,
`#PATHON`). Each recognizes its literal shape and calls its constructor;
`MicronFromString` parses with the merge, and the `+m:…` minilang literal
(boru:minilang, kind `micron` / short form `m`) dispatches on literal *shape*:

```
+m:alice@example.com    => Emailon
+m:https://x.com/a      => Urlon
+m:a/b                  => Pathon        (the whitespace-free catch-all)
```

Precedence is Emailon → Urlon → extras → Pathon; a shape the gate accepts but
the constructor refuses falls back to Pathon.

**The *other* builtin leaves are NOT in this grammar.** Semveron, Cidron,
Macon, … have their own Go string constructors (`semveronFromString`, …) reached
by `make Semveron '…'`, but no tabnas leaf in the merge — so `+m:1.2.3` is a
**Pathon**, not a Semveron. The reusable parse plumbing this proposal builds on
is therefore the *three-leaf merge plus the `MicronLiteralSpec` extension hook*,
not a per-leaf grammar for every builtin.

### 2.2 `MiniLang.micron` — the user-kind literal hook (parse only)

`MiniLang.micron <Kind> <grammar> <fn>` (`lang/go/modules/minilang.go:287`)
registers a literal shape for a **user** Micron kind. It is exactly the
"grammar in a field + builder" idea, already working:

- `<grammar>` is **a whole tabnas grammar as a declarative GrammarSpec map**
  (the same document `Parse.spec` accepts) or a `Parse.grammar` builder value.
  It is **not** a bare string: `MiniLang.micron` rejects a concrete `String`
  (that was the retired regexp form, `minilang.go:855`), and the grammar must
  declare at least one `options.match.token` gate;
- `<fn>` turns the parse result into a `Kind` instance;
- it builds an `eng.MicronLiteralSpec` and merges into the `+m` grammar via
  `eng.MicronGrammarWith`, between the builtin leaves and the Pathon catch-all.

**Today, a merge conflict is an error, not a fallback** (this is the *verified*
behaviour): a token collision is rejected in `micronGrammarFinalize`
(`minilang.go:963`), and any later `MicronGrammarWith` failure surfaces as
`mini_parse_error` from `miniMicronHandlerFor` (`minilang.go:1158`). The
"register the kind under its own minilang name instead" idea is a **proposal**
(§9), not current behaviour — do not rely on it as existing.

Verified end-to-end (runs today):

```
def Ticketon refine Micron {id:String}
MiniLang.micron Ticketon {options:{match:{token:{'#TK':'@/T-[0-9]+/'}}}}
                ([s:String] => [make Ticketon {id:s}])  end

mini m 'T-123'          # typeof => Ticketon        ✓  parse works, joins +m
```

Note the grammar surfaces are distinct: `(*Tabnas).GrammarText` parses a tabnas
**grammar-spec text** into an options/rules map; **ABNF** is a *separate* path —
the `abnf` section of `Parse.spec` (or `Parse.abnf` builder steps,
`lang/go/modules/parse.go:585`), not a `GrammarText` surface. This proposal uses
the **GrammarSpec map** form that `MiniLang.micron` already accepts.

### 2.3 The parse / emit split; tabnas is decode-only

boru already separates the two directions into sibling modules:

```
parse : string → value      (boru:parselang, lang/go/modules/parselang.go)
emit  : value → string      (boru:emitlang,  design/EMIT.md)
```

Crucially, **tabnas grammars do not reverse**. `TabnasFormat` is marked
read-only: `Encode` always returns the read-only sentinel
(`lang/go/native/format.go:88`, `ReadOnly() bool`). That is *why* `emit` is a
**separate walk-based emitter** (`design/EMIT.md`, `lang/go/native/emit.go`)
rather than an inversion of the parsers — a general grammar cannot be run
backwards (see §6.5).

### 2.4 Where the builtin render lives

Each builtin leaf hand-codes render **separately** from its grammar:

| direction | mechanism | example |
|---|---|---|
| parse | string constructor `xFromString` (reached by `make X '…'`; only Emailon/Urlon/Pathon *also* reach theirs through the `+m` tabnas merge) | `semveronFromString` (`micron.go:905`) |
| render | a `case` in `micronRender` | `micronSemveronRender` (`micron.go:1042`) |

So today, even for the builtins, parse and render are two artifacts that are
**not derived from each other**. `MiniLang.micron` supplies only parse + a
builder; a user kind registered that way still renders as its field-map. This
is the roundtrip gap the proposal closes.

---

## 3. The design — `$parse` / `$emit` on `refine Micron`

### 3.1 Reserved `$`-keys

The surface is a reserved metadata key inside the `refine Micron` body:

```
def Ticketon refine Micron {id:String
                            $parse: 'T-${id}'}
```

`$`-prefixed keys carry format metadata; the remaining keys are the field
schema. This matches the existing `$module` / `$name` synthetic-key convention.

**Status:** *not implemented.* Today `refine Micron {… $parse:…}` is **rejected**
(the `$parse` entry is validated as a field-schema entry, whose value must be a
type — verified: `check failed`). Reserving `$`-keys as metadata (and excluding
them from the field schema) is the first concrete gap (§8).

### 3.2 Three surfaces: template | grammar | function

`$parse` and `$emit` are each **polymorphic over one notion — a function**:

```
$emit: 'T-${id}'                            # template  — sugar → an emit function
$emit: ([fields:Map] => [concat 'T-' (fields.id)])   # function — fields → String

$parse: 'T-${id}'                           # template  — invertible → parse + a +m gate
$parse: {options:{match:{token:{…}}}}       # grammar   — the tabnas GrammarSpec (§5, Tier 2)
$parse: ([s:String] => [make Ticketon …])   # function  — string → value, for `make X '…'`
```

The template and the grammar are **sugar that compile to functions** — but two
**adapter constraints** (both real API facts, called out so this stays honest):

- **`$emit` function shape.** `EmitLang.register` (`emitlang.go:344`) requires
  every emitter signature to be `[value:Any opts:Map …] → String`, **not**
  field-parameters. So a Micron `$emit` is *not* a raw `EmitLang.register` fn; it
  is a **fields function** wrapped by a small adapter that projects the Micron
  value to its field map and drops `opts`. The register layer is reused *through
  that adapter*, not directly.
- **`$parse` function needs a gate to join `+m`.** `MiniLang.micron` is
  token-driven: the merged parser must know which literal spans a kind claims, so
  the grammar must declare an `options.match.token` gate (`minilang.go:846`). A
  bare `$parse` **function alone cannot join `+m`** — it serves only the
  `make <Kind> 'string'` path (§8). To participate in `+m:…` dispatch a kind must
  supply a template or grammar (which carry the gate); the function is then the
  post-parse builder, not the gate.

### 3.3 Symmetry

The two keys are mirror images, exactly like `parse` / `emit`:

```
$parse : String → fields      (joins the +m grammar / MicronFromString)
$emit  : fields → String      (installs the kind's canonical render)
```

A kind may give both, or give a single **invertible** `$parse` that serves both
directions (§4).

---

## 4. The invertible template (Tier 1) — roundtrip by construction

The default, and the answer to "solve formatting for simple user microns
first," is a single **invertible template**: a linear sequence of literal
delimiters and typed, single-use `${field}` holes.

```
def Endpointon refine Micron {host:Hoston version:Semveron
                              $parse: '${host} v${version}'}

make Endpointon 'api.x.com:443 v2.1.0'
#   parse : split on the literal " v"  → host-slice 'api.x.com:443', version-slice '2.1.0'
#           → each slice parsed by its OWN field type:  make Hoston 'api.x.com:443', make Semveron '2.1.0'
#   render: render each field via its type, rejoin with the literals
print (make Endpointon 'api.x.com:443 v2.1.0')   # => api.x.com:443 v2.1.0
```

Three properties fall out:

1. **Field extraction is automatic.** The hole *names* are the field names; the
   field *types* come from the `refine Micron {…}` schema. No hand-written
   builder.
2. **It composes with recursive `make`.** The template only splits at the
   literal delimiters; each slice is handed to that field's own `make`, so
   composite Microns nest. (This unifies the earlier "Layer 1 recursive make"
   and "Layer 2 template" into one mechanism.)
3. **It joins `+m`.** The template compiles to a tabnas `GrammarSpec`
   (literals → match tokens, `${field}` → captures) fed through the existing
   `MiniLang.micron` path.

**The roundtrip law** holds *by construction* for the invertible subset. Writing
`render x` for the kind's **canonical string** (the `$emit` output / the value's
`String`), and **not** bare `emit`:

```
make Xon (render x)  ==  x            for every x : Xon
```

because render is substitution and parse is the inverse split on the same
literals. A generated roundtrip test per kind proves it (§8). This is the whole
reason to keep Tier 1 to the invertible class: the law is free, not asserted.

> **Why not bare `emit`.** `emit x` auto-selects the **JSON** encoder for a
> Scalar (`lang/go/native/emit.go:198`), which quotes `v.String()` — so
> `make Xon (emit x)` would feed a JSON string literal `"…"`, not the canonical
> Micron text. The law is about the kind's own canonical render. Making bare
> `emit x` pick a per-kind Micron renderer would need a natural-emitter hook for
> the Micron family (listed in §9); it is not assumed here.

**Invertibility conditions** (what keeps a template in Tier 1): a linear
concatenation of literal chunks and holes; each field appears exactly once; the
literal delimiters are non-empty and unambiguous (a field's rendered form
cannot contain the following delimiter); no alternation / optional / repetition.

---

## 5. Tiers and the invertibility boundary

| Tier | `$parse` | `$emit` | Roundtrip | Use |
|---|---|---|---|---|
| **0** | *(none)* | *(none)* | map ↔ map (already) | **the default** — most kinds; no custom string form wanted |
| **1** | invertible template `'…${f}…'` | *derived from `$parse`* | by construction | simple microns that want a compact literal |
| **2** | full tabnas grammar (ABNF text or GrammarSpec map) | **separate** template / fn | enforced by test | syntax with alternation / optional / repetition |
| **fn** | builder function | render function | enforced by test | full custom |

**Tier 0 is the baseline and needs no opt-in:** a kind that declares neither key
constructs from and renders as its field-map, exactly as today. Tiers 1–3 are
purely additive for kinds that want a custom spelling.

Tier 2 exists because a **full grammar cannot be inverted** (§6.5) — so its
render must be supplied separately, and a generated test enforces the two stay
consistent (mirroring how the builtins hand-code both sides, and how
`parse`/`emit` are separate modules):

```
def Ticketon refine Micron {id:String team:String
  $parse: {options:{match:{token:{'#TK':'@/T-[0-9]+@[a-z]+/'}}} …}   # full grammar
  $emit:  'T-${id}@${team}'}                                          # separate render
```

---

## 6. The abstraction operator (why the template surface is what it is)

The template surface raises a natural question: an interp string
`` `${host} v${version}` `` *looks* like a render template already — why not use
it directly as `$emit`? Answering that pins down a general operator worth
recording.

### 6.1 An interp string is an eager closure over its free variables

In the engine an interp string is `InterpStringPayload{Parts []InterpPart}`,
and each `InterpPart` is either a literal chunk (`Lit`) or an **embedded
expression** (`Expr []Value`) — `eng/go/value.go:2106`. `evalInterpParts`
(`eng/go/engine.go:4152`) runs each hole's `Expr` **in a sub-engine against the
live registry**, stringifies, and concatenates. So denotationally:

```
`${a} x ${b}`   ≡   fn [[a b]] [ concat (str a) ' x ' (str b) ]
```

— a **closure over its free variables** (the hole references), returning a
string. But it is **eager**: boru applies it the instant it is reached. Verified:

```
def x 5
def s `${x} bar`     # s is "5 bar" — captured ONCE, at the point reached
def x 9
print s              # => 5 bar   (not "9 bar")
```

The value you ever hold is the *result* string, never the function.

### 6.2 Why `quote` / `ref` cannot defer it

Neither reification operator catches the closure:

- **`ref`** resolves a **name** to its binding and strips fn-auto-invoke
  (`ResolveRef`, `eng/go/core_ref.go:26`). It needs a bound name — an interp
  *literal* is not one, so `ref ` `` `${x} bar` `` `` fails at `ref`. And the doc
  is explicit that `ref`'s result is **unquoted / still a live call site** —
  `ref` un-invokes a name, it does not defer an expression.
- **`quote`** captures a token as inert data — but an interp string is **not
  inert**; the checker still resolves the holes' free variables under quote
  (`quote ` `` `${zzz} bar` `` `` flags `zzz` — verified).

The reason both miss: the holes are **free variables resolved against the
enclosing scope**, not parameters. An interp string is a **closure** (it
references its environment), not a **λ-abstraction** (nothing introduces its
holes as parameters). There is no binder for `ref` or `quote` to suspend.

### 6.3 The operator, named

To hold an interp string as a function you must **abstract its free variables
into parameters** — a first-class **λ-abstraction over the free variables of an
open term**:

```
`${host} v${version}`   ==>   fn [[host version]] [ concat (str host) ' v' (str version) ]
```

Classical names:

- **Bracket abstraction** (Schönfinkel / Curry): `[x]M` — the term such that
  `([x]M) N = M[x:=N]`; the primitive that converts λ-calculus to SKI
  combinators.
- **Lambda lifting** (Johnsson, 1985): the same operation as a compiler pass —
  eliminate free variables by making them parameters.

One boru-native framing: the operator is **`quote` + auto-parameterize-the-holes**.
`quote` reifies a term as data without abstracting; this operator reifies *and*
binds the free holes as λ-params. They are siblings — which is also why
`quote`/`ref` can't stand in for it (§6.2): they reify or un-invoke, but neither
introduces the binder.

### 6.4 Equivalents in other languages

The surface form — mark holes, auto-abstract into params — is common (it *is*
this operator, restricted to syntactically-marked holes):

| Language | Form | Desugars to |
|---|---|---|
| Scala | `_ + 1` | `x => x + 1` |
| Clojure | `#(+ % 1)` | `(fn [x] (+ x 1))` |
| Mathematica | `# + 1 &` | `Function[x, x+1]` |
| Elixir | `&(&1 + 1)` | `fn x -> x + 1 end` |
| Swift | `{ $0 + 1 }` | `{ x in x + 1 }` |
| Kotlin | `it + 1` | single implicit param |

boru's `${…}`-holes → fn is this family, specialized to string templates with
*named* holes (the field names) instead of positional `_` / `%` / `$0`.

The **general** operator (abstract *all* free vars, not just marked ones) is the
bracket-abstraction / lambda-lifting pair above. The **prerequisite** — code
held as an inspectable open term — is **quasiquotation** (Lisp `` `(,x) ``,
Scheme, Template Haskell `[| … |]` + `$(...)`) and **staged metaprogramming**
(MetaML, MetaOCaml `.<e>.` / `.~e`, whose "open code" values are the closest
thing to a first-class version). An interp string *is* a quasiquote: literal
parts quoted, `${…}` antiquoted.

So the operator is well-known, but almost always appears as **(a)** syntactic
sugar over marked holes, **(b)** a compile-time transform (lambda lifting), or
**(c)** staging brackets — essentially never as a general runtime operator over
arbitrary values, for the reason in §6.5.

### 6.5 Would it generalize to closures? — No

The hard distinction:

- an **open term** = unevaluated code that still *contains* free variables;
- a **closure** = an abstraction that has *already captured* its free variables'
  values.

Abstraction needs free variables to abstract. A closure has **none left** —
they have been resolved into captured values; the term is closed. Abstracting a
closure would mean *un-capturing*, and capture has no inverse the operation can
run: which captured values were "meant to be" parameters is not recoverable,
and the captured environment is opaque anyway (in boru, `inspect` on a fn
surfaces only its **signatures**, never captures or body — verified).

So the operator is fundamentally **term → function**. There is no sound
**closure → function** direction. It lives in the metaprogramming / staging
world (code is an inspectable open term), not the closure world.

This is exactly why interp strings qualify and closures don't: an interp string
is a **reified open term** (its `Parts` still hold unevaluated hole `Expr`s),
*not* a closure — and boru's eager evaluation is the precise moment the open term
collapses to a closed value (`def s ` `` `${x} bar` `` ` → `"5 bar"`). The
operator must act **before** that collapse, on the term.

### 6.6 boru-specific position

boru is *closer* to a general version than a compiled language: fn bodies are
reified token streams internally, and the engine already computes free-variable
captures (`ComputeCaptures`, `eng/go/callable_words.go`). But none of that is
first-class at the surface — closures are opaque (`inspect` = signatures only;
no `abstract` / `capture` / `freevar` word). Generalizing the operator to
arbitrary closures would require exposing reification that does not exist and
re-opening captures that are semantically baked. The clean, sound target is
**open-term abstraction over syntactically-delimited holes** — i.e. the
template case, whose free-variable set is trivially the `${…}` names.

### 6.7 Consequence for the format surface

This is why Tier 1 uses a **plain-string** template `'T-${id}'`, not a backtick
interp `` `T-${id}` ``:

- A plain string is **inert text** — the Micron layer parses the `${…}` itself,
  so it never touches boru's eager interp machinery and never demands the fields
  be in scope. The `${id}` is a *field name to abstract over*, not a *free
  variable to resolve now*.
- It is also **direction-neutral**: render by *filling* the holes, parse by
  *splitting* on the literals — serving both `$parse` and `$emit` from one
  string. A backtick interp commits to eval/render and would need the
  abstraction operator of §6.3 even to be usable.

The **function** form (`$emit: ([host version] => […])`) is just §6.3's
abstraction written explicitly. If boru later grows the abstraction operator as a
first-class thing, `$emit: ` `` `${…}` `` `` becomes a third spelling of the same
function — but it is not required for this design.

---

## 7. How Tier 1 desugars onto existing machinery

No net-new parse plumbing. `$parse:'T-${id}'` compiles to what already runs:

```
# 'T-${id}'  ==  literals ["T-"] + hole [id]   ==>

MiniLang.micron Ticketon
  {options:{match:{token:{'#TK':'@/T-.+/'}}}}                 # template → GrammarSpec (parse, joins +m)
  ([s:String] => [make Ticketon {id: <slice after 'T-'>}])    # auto-derived builder (hole→field)

# + install the kind's canonical render = fill the same template   (render / $emit)
```

So Tier 1 is: one template → (a) the existing `MiniLang.micron` grammar+builder
for parse, plus (b) a template emitter for render. The genuinely new work is
the reserved-key surface and the template↔grammar compiler.

---

## 8. Gaps to implement (Tier 1)

1. **Reserve `$`-keys** in `refine Micron` as metadata, excluded from the field
   schema (today `$parse:` is rejected).
2. **Template → `MicronLiteralSpec`** compiler (literals → match tokens,
   `${field}` → captures) routed through `MiniLang.micron` /
   `MicronGrammarWith`, so the kind joins `+m` / `MicronFromString`.
3. **`make <Kind> 'string'` construction path.** Joining `+m` covers only
   `+m:…` / `mini m '…'`. The headline `make Ticketon 'T-123'` is a *separate*
   path: `makeMicronUser` (`eng/go/micron.go:2219`) currently rejects every
   non-map source. Wire a registry-aware `make <Kind> String` that routes the
   string through the kind's parser, or the advertised `make` form still fails.
4. **Render**: install the kind's canonical String render to fill the template
   (used by `print` / the value's `String`); auto-derive the field-capture
   builder (hole names → schema fields), removing the hand-written fn in the
   common case. Wrap it as an `EmitLang.register` emitter *via a value→fields
   adapter* (§3.2) if `emit <kind>` is also wanted.
5. **Roundtrip test generator**: assert `make Xon (render x) == x` per kind
   (canonical render, not bare `emit` — §4).
6. **Invertibility check**: reject a Tier-1 `$parse` template that is not
   invertible (repeated field, empty/ambiguous delimiter, alternation) with a
   clear error pointing at Tier 2.
7. **(Optional) natural-emit hook** for the Micron family, so bare `emit x`
   picks the per-kind render instead of the JSON default (§4 caveat).

---

## 9. Open questions

- **`$emit` default = the map (nothing is mandatory).** When only an invertible
  `$parse` is given, `$emit` is derived. When only a non-invertible `$parse`
  (Tier 2) is given and no `$emit`, the kind simply **keeps the field-map render**
  — the default from §1. Parsing a compact literal but rendering back to
  `{…}` is asymmetric but not wrong: the map is a legitimate representation, and a
  custom `$emit` is opt-in. (Earlier draft leaned "error"; the map default makes
  that unnecessary.)
- **Delimiter ambiguity at the boundary of recursive fields.** A field whose own
  rendered form can contain the following literal delimiter breaks the split
  (e.g. a `${path}` slice containing the delimiter char). The invertibility
  check (§8.5) must reason about each field type's alphabet, or require an
  explicit escape/quote in the template.
- **A grammar that can't join `+m` gets its OWN minilang name.** A user kind's
  `$parse` grammar normally merges into the shared `+m` grammar (between the
  builtin leaves and the Pathon catch-all). But the merge can genuinely fail — a
  match-token name collision, or a shape that overlaps another kind's — and a
  Micron literal is not required to live under `+m`. When `eng.MicronGrammarWith`
  (the tabnas `(*Tabnas).Merge`) rejects a kind, it is **not forced in**: it is
  registered as its own **named minilang** instead (a distinct `MiniLang.register`
  kind — e.g. `+ticket:T-123` rather than `+m:T-123`), giving the incompatible
  shape its own literal namespace while `+m` stays a clean merge of the compatible
  ones. So `+m` membership is best-effort, not mandatory; the fallback is a
  per-kind minilang, and the `parse`/`emit` behaviour is identical either way.
  This also dissolves the old precedence question: kinds that would collide simply
  don't share a namespace.
- **First-class abstraction operator (§6.3).** Out of scope here, but if built,
  it would let backtick `` `${…}` `` templates serve as `$emit` directly and would
  have uses beyond Microns (format helpers, staged string building). Worth its
  own note.
